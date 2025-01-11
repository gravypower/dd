package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

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
	flagCredentialsPath = flag.String("creds", "creds.json", "path to credentials file")
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
	logger.SetReportCaller(true)
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

	if *flagRemoveEntity != "" {
		ddapi.RemoveEntity(mqttHandler, *flagRemoveEntity)
		return
	}

	ddConn := dd.Conn{Host: *flagHost, Debug: *flagDebug}
	err = ddConn.Connect(credentials.Credential)
	if err != nil {
		logger.WithError(err).Fatal("failed to connect to dd")
	}

	basicInfo := ddapi.FetchBasicInfo(&ddConn)
	logger.WithField("basicInfo", basicInfo).Info("Fetched basic information about the connection")

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
			err := fsm.FSM.Event(context.Background(), "go_offline")
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

	logger.Info("Waiting for MQTT messages...")

	for status := range statusCh {
		for _, device := range status.Devices {
			logger.WithField("Position", device.Device.Position).Info("Announcing Position")
			deviceFSM, exists := ddapi.DeviceFSMs[device.ID]
			if !exists {
				deviceFSM = ddapi.ConfigureDevice(mqttHandler, &ddConn, *flagMqttPrefix, device, basicInfo)
				// Subscribe to MQTT
				subscribeToMQTT(mqttHandler, *flagMqttPrefix)
				logger.Info("Waiting on status updates...")
				err := deviceFSM.FSM.Event(context.Background(), "go_online")
				if err != nil {
					logger.WithError(err).Error("Failed to process 'go_online' event")
				}
			} else {
				logger.WithField("deviceID", device.ID).Info("Device already configured")
			}

			var haState string
			switch device.Device.Position {
			case OPEN:
				haState = "open"
			case CLOSE:
				haState = "closed"
			default:
				haState = "unknown"
			}

			err = deviceFSM.FSM.Event(context.Background(), haState)
			if err != nil {
				logger.WithError(err).WithField("haState", haState).Error("Failed to process event")
			}

		}
	}

}

// Connect to MQTT broker
func connectToMQTT(broker, user, password string, port int) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", broker, port))
	opts.SetClientID("go_mqtt_client")

	if user != "" {
		opts.SetUsername(user)
	}

	if password != "" {
		opts.SetPassword(password)
	}

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		logger.WithError(token.Error()).Fatal("Failed to connect to MQTT broker")
	}

	return client
}

// Subscribe to MQTT topics
func subscribeToMQTT(mqttHandler *ddapi.MQTTHandler, prefix string) {
	topic := fmt.Sprintf("%s/#", prefix)
	token := mqttHandler.Client.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
		handleMQTTMessage(msg.Topic(), msg.Payload())
	})
	if token.Wait() && token.Error() != nil {
		logger.WithError(token.Error()).Fatal("Failed to subscribe to MQTT topic")
	}
	logger.Infof("Subscribed to topic: %s", topic)
}

// Handle incoming MQTT messages
func handleMQTTMessage(topic string, payload []byte) {
	parts := strings.Split(topic, "/")
	if len(parts) < 3 {
		logger.WithField("topic", topic).Warn("Invalid topic format")
		return
	}

	command := strings.ToUpper(string(payload))

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
		err := deviceFSM.FSM.Event(context.Background(), "go_online")
		if err != nil {
			logger.WithError(err).Error("Failed to process 'go_online' event")
		}
	case "OFFLINE":
		err := deviceFSM.FSM.Event(context.Background(), "go_offline")
		if err != nil {
			logger.WithError(err).Error("Failed to process 'go_offline' event")
		}
	case "OPEN":
		err := deviceFSM.FSM.Event(context.Background(), "open")
		if err != nil {
			logger.WithError(err).Error("Failed to process 'open' event")
		}
	case "CLOSE":
		err := deviceFSM.FSM.Event(context.Background(), "close")
		if err != nil {
			logger.WithError(err).Error("Failed to process 'close' event")
		}
	case "STOP":
		err := deviceFSM.FSM.Event(context.Background(), "stop")
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
