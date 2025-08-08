package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/samthor/dd"
	ddapi "github.com/samthor/dd/api"
	"github.com/samthor/dd/helper"
	"github.com/sirupsen/logrus"
)

const (
	CLOSE = 0
	OPEN  = 100
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

	// Wait for MQTT to be available before proceeding to init state machine
	for !mqttClient.IsConnectionOpen() && !mqttClient.IsConnected() {
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

	basicInfo := ddapi.FetchBasicInfo(&ddConn)
	logger.WithField("basicInfo", basicInfo).Debug("Fetched basic information about the connection")

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	// Wait for the termination signal
	go func() {
		<-stopCh
		logger.Info("Termination signal received")
		// Ensure resources are cleaned up
		logger.Info("Shutting down gracefully")
		for deviceID, fsm := range ddapi.DeviceFSMs {
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
	go handleStatusUpdates(&ddConn, statusCh)

	var deviceFSMsMutex sync.Mutex
	for status := range statusCh {
		for _, device := range status.Devices {
			logger.WithField("Position", device.Device.Position).Info("Announcing Position")

			// Ensure thread-safe access to DeviceFSMs
			deviceFSMsMutex.Lock()
			deviceFSM, exists := ddapi.DeviceFSMs[device.ID]
			if !exists {
				deviceFSM = ddapi.ConfigureDevice(mqttHandler, &ddConn, *flagMqttPrefix, device, basicInfo)
				ddapi.DeviceFSMs[device.ID] = deviceFSM
				// Subscribe to MQTT
				subscribeToMQTTCommandTopics(mqttHandler, *flagMqttPrefix)
				logger.Info("Waiting on status updates...")
				err := deviceFSM.Trigger(context.Background(), "go_online")
				if err != nil {
					logger.WithError(err).Error("Failed to process 'go_online' event")
				}
			} else {
				logger.WithField("deviceID", device.ID).Info("Device already configured")
			}
			deviceFSMsMutex.Unlock()

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
	opts.SetClientID("go_mqtt_client")

	// Make MQTT connection resilient
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		logger.Info("Connected to MQTT broker")
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
	if token := client.Connect(); token.WaitTimeout(3*time.Second) && token.Error() != nil {
		logger.WithError(token.Error()).Warn("Initial MQTT connect failed; will keep retrying in background")
	}

	return client
}

// Subscribe to MQTT topics
func subscribeToMQTTCommandTopics(mqttHandler *ddapi.MQTTHandler, prefix string) {
	commandTopics := fmt.Sprintf(ddapi.CommandTopicTemplate, prefix, "+")

	token := mqttHandler.Client.Subscribe(commandTopics, 0, func(client mqtt.Client, msg mqtt.Message) {
		payload := strings.ToUpper(string(msg.Payload()))
		logger.WithField("payload", payload).WithField("commandTopics", commandTopics).Info("processing mqtt message")
		handleCommand(msg.Topic(), payload)
	})
	if token.Wait() && token.Error() != nil {
		logger.WithError(token.Error()).WithField("topic", commandTopics).Fatal("Failed to subscribe to MQTT topic")
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
	deviceFSM, exists := ddapi.DeviceFSMs[deviceID]

	if !exists {
		logger.WithField(
			"device",
			deviceID).WithField(
			"DeviceFSMs",
			ddapi.DeviceFSMs).Fatal(
			"Device not exists")
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

func handleStatusUpdates(conn *dd.Conn, statusCh chan ddapi.DoorStatus) {
	status := ddapi.SafeFetchStatus(conn)
	statusCh <- *status
	if err := helper.LoopMessages(context.Background(), conn, statusCh); err != nil {
		logger.WithField("error", err).Fatal("Error reading messages")
	}
}
