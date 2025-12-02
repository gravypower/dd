package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/samthor/dd"
	ddapi "github.com/samthor/dd/api"
	"github.com/samthor/dd/helper"
	"github.com/sirupsen/logrus"
)

// Door position constants (0-100 scale)
const (
	// CLOSE represents a fully closed door position
	CLOSE = 0
	// OPEN represents a fully open door position
	OPEN = 100
)

// Logger setup
var logger = logrus.New()

// Flags
var (
	flagCredentialsPath = flag.String("credentials", "dd-credentials.json", "path to credentials file")
	flagHost            = flag.String("host", "", "host to connect to")
	flagMqtt            = flag.String("mqtt", "", "mqtt server")
	flagMqttPort        = flag.Int("mqttPort", 1883, "mqtt port")
	flagMqttUser        = flag.String("mqttUser", "", "mqtt user")
	flagMqttPassword    = flag.String("mqttPassword", "", "mqtt password")
	flagMqttPrefix      = flag.String("mqttPrefix", "dd-door", "prefix for mqtt")
	flagRemoveEntity    = flag.String("removeEntity", "", "entity to remove from haus")
	flagDebug           = flag.Bool("debug", false, "debug mode")
)

func init() {
	logger.SetOutput(os.Stdout)
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})
	logger.SetLevel(logrus.InfoLevel)
}

func main() {
	flag.Parse()

	credentials, err := helper.LoadCreds(*flagCredentialsPath)
	if err != nil {
		logger.WithField("*flagCredentialsPath", *flagCredentialsPath).WithError(err).Fatal("can't open credentials file")
	}

	// MQTT connection setup
	mqttClient := connectToMQTT(*flagMqtt, *flagMqttUser, *flagMqttPassword, *flagMqttPort)
	mqttHandler := ddapi.NewMQTTHandler(mqttClient, logger)

	// Wait for MQTT to be available before proceeding to init state machine (bounded)
	maxWait := 60 * time.Second
	deadline := time.Now().Add(maxWait)
	for !mqttClient.IsConnected() {
		if time.Now().After(deadline) {
			logger.Error("MQTT did not connect within 60s. Check broker address, port, and credentials (username/password). Exiting.")
			os.Exit(1)
		}
		logger.Warn("MQTT not available yet; waiting before initializing state machine...")
		time.Sleep(5 * time.Second)
	}
	logger.Info("MQTT is connected; proceeding with initialization")

	if *flagRemoveEntity != "" {
		err := mqttHandler.RemoveEntity(*flagRemoveEntity)
		if err != nil {
			logger.WithField("*flagRemoveEntity", *flagRemoveEntity).WithError(err).Fatal("can't remove entity")
		}
		return
	}

	ddConn := dd.Conn{Host: *flagHost, Debug: *flagDebug}
	err = ddConn.Connect(credentials.Credential)
	if err != nil {
		logger.WithError(err).Fatal("failed to connect to dd")
	}

	basicInfo, err := ddapi.FetchBasicInfo(&ddConn)
	if err != nil {
		logger.WithError(err).Fatal("failed to fetch basic device info")
	}
	logger.WithField("basicInfo", basicInfo).Debug("Fetched basic information about the connection")

	// Context for background goroutines
	ctx, cancel := context.WithCancel(context.Background())

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	// Wait for the termination signal
	go func() {
		<-stopCh
		logger.Info("Termination signal received")
		// Ensure resources are cleaned up
		logger.Info("Shutting down gracefully")
		// Cancel the background status loop first
		cancel()
		// Use thread-safe helper to get all devices
		allDevices := ddapi.GetAllDeviceFSMs()
		for deviceID, fsm := range allDevices {
			logger.Infof("Shutting down device: %s", deviceID)
			err := fsm.Trigger(context.Background(), "go_offline")
			if err != nil {
				logger.WithField("deviceID", deviceID).WithError(err).Error("Failed to set device to offline")
			} else {
				logger.WithField("deviceID", deviceID).Info("Device successfully set to offline")
			}
		}
		mqttClient.Disconnect(250)
		os.Exit(0)
	}()

	statusCh := make(chan ddapi.DoorStatus)
	go handleStatusUpdates(ctx, &ddConn, statusCh)

	for status := range statusCh {
		for _, device := range status.Devices {
			logger.WithField("Position", device.Device.Position).Info("Announcing Position")

			// Ensure thread-safe access to DeviceFSMs using helper functions
			deviceFSM, exists := ddapi.GetDeviceFSM(device.ID)
			if !exists {
				deviceFSM = ddapi.ConfigureDevice(mqttHandler, &ddConn, *flagMqttPrefix, device, *basicInfo)
				// Subscriptions are handled in MQTT OnConnect handler
				logger.Info("Waiting on status updates...")
				err := deviceFSM.Trigger(context.Background(), "go_online")
				if err != nil {
					logger.WithError(err).Error("Failed to process 'go_online' event")
				}
			} else {
				logger.WithField("deviceID", device.ID).Info("Device already configured")
			}

			// Determine the desired FSM state
			var haState string
			switch device.Device.Position {
			case OPEN:
				haState = "go_opened"
			case CLOSE:
				haState = "go_closed"
			default:
				logger.WithField("Position", device.Device.Position).Warn("Ignoring intermediate or unknown position")
				continue // Skip this device
			}

			currentState := deviceFSM.Current()
			// Skip redundant transitions to the same final state (idempotent)
			if (currentState == "closed" && haState == "go_closed") ||
				(currentState == "open" && haState == "go_opened") {
				logger.WithFields(logrus.Fields{
					"currentState": currentState,
					"haState":      haState,
					"deviceID":     device.ID,
				}).Debug("Ignoring redundant transition to the same state")
				continue
			}

			if (currentState == "opening" && haState == "go_closed") ||
				(currentState == "closing" && haState == "go_opened") {
				logger.WithFields(logrus.Fields{
					"currentState": currentState,
					"haState":      haState,
					"deviceID":     device.ID,
				}).Debug("Ignoring invalid state transition while opening or closing")
				continue
			}

			// Process the state transition
			err := deviceFSM.Trigger(context.Background(), haState)
			if err != nil {
				logger.WithError(err).
					WithField("haState", haState).
					WithField("currentState", deviceFSM.Current()).
					Error("Failed to process event")
			}
		}
	}

}

// Connect to MQTT broker
func connectToMQTT(broker, user, password string, port int) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", broker, port))
	// Use a stable client ID for a persistent session
	opts.SetClientID("dd_haus")

	// Networking and timeouts
	opts.SetConnectTimeout(5 * time.Second)
	opts.SetWriteTimeout(5 * time.Second)

	// Make MQTT connection resilient
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)
	// Enable persistent session and automatic resubscription
	opts.SetCleanSession(false)
	opts.SetResumeSubs(true)
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		logger.Info("Connected to MQTT broker")
		// Subscribe (or resubscribe) on every (re)connect
		subscribeToMQTTCommandTopics(ddapi.NewMQTTHandler(c, logger), *flagMqttPrefix)
	})
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		logger.WithError(err).Warn("MQTT connection lost; will retry")
	})

	if user != "" {
		opts.SetUsername(user)
	}

	if password != "" {
		opts.SetPassword(password)
	}

	client := mqtt.NewClient(opts)
	if token := client.Connect(); !token.WaitTimeout(3 * time.Second) {
		logger.Warn("Initial MQTT connect timed out; auto-reconnect will continue in background")
	} else if err := token.Error(); err != nil {
		// Detect common authentication/authorization failures and fail fast
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "not authorized") || strings.Contains(errStr, "not authorised") || strings.Contains(errStr, "bad user name or password") || strings.Contains(errStr, "unauthor") {
			logger.WithError(err).Error("MQTT authentication failed. Check username/password and broker ACLs.")
			os.Exit(1)
		}
		logger.WithError(err).Warn("Initial MQTT connect failed; will keep retrying in background")
	}

	return client
}

// Subscribe to MQTT topics
func subscribeToMQTTCommandTopics(mqttHandler *ddapi.MQTTHandler, prefix string) {
	commandTopics := fmt.Sprintf(ddapi.CommandTopicTemplate, prefix, "+")

	// If not connected, skip subscribing; OnConnect will invoke us again
	if !mqttHandler.Client.IsConnected() {
		logger.WithField("topic", commandTopics).Warn("Skipping subscribe: MQTT not connected")
		return
	}

	token := mqttHandler.Client.Subscribe(commandTopics, 0, func(client mqtt.Client, msg mqtt.Message) {
		payload := strings.ToUpper(string(msg.Payload()))
		logger.WithField("payload", payload).WithField("commandTopics", commandTopics).Info("processing mqtt message")
		handleCommand(msg.Topic(), payload)
	})
	if !token.WaitTimeout(3 * time.Second) {
		logger.WithField("topic", commandTopics).Warn("Subscribe timed out; will retry on next reconnect")
		return
	}
	if err := token.Error(); err != nil {
		logger.WithError(err).WithField("topic", commandTopics).Warn("Subscribe failed; will retry on next reconnect")
		return
	}
	logger.WithField("commandTopics", commandTopics).Info("Subscribed to topic")
}

// Handle incoming MQTT messages
func handleCommand(topic string, command string) {
	parts := strings.Split(topic, "/")
	if len(parts) < 3 {
		logger.WithField("topic", topic).Warn("Invalid topic format")
		return
	}

	deviceID := parts[1]
	// Use thread-safe helper to access DeviceFSMs
	deviceFSM, exists := ddapi.GetDeviceFSM(deviceID)

	if !exists {
		logger.WithField("device", deviceID).Error("Device does not exist")
		return
	}

	switch command {
	case "ONLINE":
		err := deviceFSM.Trigger(context.Background(), "go_online")
		if err != nil {
			logger.WithError(err).Error("Failed to process 'go_online' event")
		}
	case "OFFLINE":
		err := deviceFSM.Trigger(context.Background(), "go_offline")
		if err != nil {
			logger.WithError(err).Error("Failed to process 'go_offline' event")
		}
	case "GO_OPEN":
		err := deviceFSM.Trigger(context.Background(), "go_open")
		if err != nil {
			logger.WithError(err).Error("Failed to process 'open' event")
		}
	case "GO_CLOSE":
		err := deviceFSM.Trigger(context.Background(), "go_close")
		if err != nil {
			logger.WithError(err).Error("Failed to process 'close' event")
		}
	case "STOP":
		err := deviceFSM.Trigger(context.Background(), "go_stop")
		if err != nil {
			logger.WithError(err).Error("Failed to process 'stop' event")
		}
	default:
		logger.WithFields(logrus.Fields{
			"deviceID": deviceID,
			"command":  command}).Warn("Unknown command for device")
	}
}

func handleStatusUpdates(ctx context.Context, conn *dd.Conn, statusCh chan ddapi.DoorStatus) {
	status, err := ddapi.SafeFetchStatus(conn)
	if err != nil {
		logger.WithError(err).Error("Failed to fetch initial status")
		// Continue even if initial fetch fails - messages loop may recover
	} else {
		statusCh <- *status
	}

	if err := helper.LoopMessages(ctx, conn, statusCh); err != nil {
		logger.WithError(err).Error("Error reading messages - connection may be lost")
		// Allow graceful shutdown instead of Fatal
		close(statusCh)
	}
}
