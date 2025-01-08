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
	"github.com/looplab/fsm"
	"github.com/samthor/dd"
	ddapi "github.com/samthor/dd/api"
	"github.com/samthor/dd/helper"
	"github.com/sirupsen/logrus"
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

// DeviceFSM encapsulates a state machine for a device
type DeviceFSM struct {
	ID          string
	FSM         *fsm.FSM
	Conn        *dd.Conn
	mqttHandler *ddapi.MQTTHandler
	State       string // Current state for additional tracking if needed
}

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

	if *flagRemoveEntity != "" {
		ddapi.RemoveEntity(mqttHandler, *flagRemoveEntity)
		return
	}

	conn := dd.Conn{Host: *flagHost, Debug: *flagDebug}
	err = conn.Connect(credentials.Credential)
	if err != nil {
		logger.WithError(err).Fatal("failed to connect to dd")
	}

	basicInfo := ddapi.FetchBasicInfo(&conn)
	logger.WithField("basicInfo", basicInfo).Info("Fetched basic information about the connection")

	// Map to store FSMs for devices
	deviceFSMs := make(map[string]*DeviceFSM)

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	// Ensure resources are cleaned up on shutdown
	defer func() {
		logger.Info("Shutting down gracefully")
		for deviceID, fsm := range deviceFSMs {
			logger.Infof("Shutting down device: %s", deviceID)
			err := fsm.FSM.Event(context.Background(), "go_offline")
			if err != nil {
				logger.WithError(err).Errorf("Failed to set device %s to offline", deviceID)
			} else {
				logger.Infof("Device %s successfully set to offline", deviceID)
			}
		}
		mqttClient.Disconnect(250)
	}()

	statusCh := make(chan ddapi.DoorStatus)
	go handleStatusUpdates(&conn, statusCh)

	// Subscribe to MQTT
	subscribeToMQTT(mqttHandler, *flagMqttPrefix, deviceFSMs, &conn)

	logger.Info("Waiting for MQTT messages...")
	logger.Info("Waiting on status updates...")

	for status := range statusCh {
		logger.WithField("status", status).Info("Announcing status")
		for _, device := range status.Devices {
			if !ddapi.ConfiguredDevices[device.ID] {
				ddapi.ConfigureDevice(mqttHandler, *flagMqttPrefix, device, basicInfo)
			} else {
				logger.WithField("deviceID", device.ID).Info("Device already configured")
			}
			ddapi.MarkOnline(mqttHandler, *flagMqttPrefix, device.ID)

			// Retrieve or create the FSM for the device
			deviceFSM, exists := deviceFSMs[device.ID]
			if !exists {
				deviceFSM = NewDeviceFSM(device.ID, &conn, mqttHandler)
				deviceFSMs[device.ID] = deviceFSM
			}

			ddapi.PublishDoorState(mqttHandler, *flagMqttPrefix, device.ID, device.Device.Position)
		}
	}

}

// NewDeviceFSM initializes the FSM for a specific device
func NewDeviceFSM(deviceID string, conn *dd.Conn, mqttHandler *ddapi.MQTTHandler) *DeviceFSM {
	return &DeviceFSM{
		ID:          deviceID,
		Conn:        conn,
		mqttHandler: mqttHandler,
		FSM: fsm.NewFSM(
			"online",
			fsm.Events{
				{Name: "go_online", Src: []string{"offline"}, Dst: "online"},
				{Name: "go_offline", Src: []string{"online", "opening", "closing", "open", "closed"}, Dst: "offline"},
				{Name: "open", Src: []string{"closed"}, Dst: "opening"},
				{Name: "close", Src: []string{"open"}, Dst: "closing"},
				{Name: "opened", Src: []string{"opening"}, Dst: "open"},
				{Name: "closed", Src: []string{"closing"}, Dst: "closed"},
				{Name: "stop", Src: []string{"opening", "closing"}, Dst: "stopped"},
			},
			fsm.Callbacks{
				"enter_online": func(ctx context.Context, e *fsm.Event) {
					logger.WithField("deviceID", deviceID).Info("Device is online")
					ddapi.MarkOnline(mqttHandler, *flagMqttPrefix, deviceID)
				},
				"enter_offline": func(ctx context.Context, e *fsm.Event) {
					ddapi.MarkOffline(mqttHandler, *flagMqttPrefix, deviceID)
					logger.WithField("deviceID", deviceID).Info("Device is offline")
				},
				"enter_opening": func(ctx context.Context, e *fsm.Event) {
					logger.WithField("deviceID", deviceID).Info("Device is opening")
					ddapi.SafeCommand(conn, deviceID, ddapi.AvailableCommands.Open)
				},
				"enter_closing": func(ctx context.Context, e *fsm.Event) {
					logger.WithField("deviceID", deviceID).Info("Device is closing")
					ddapi.SafeCommand(conn, deviceID, ddapi.AvailableCommands.Close)
				},
				"enter_stopped": func(ctx context.Context, e *fsm.Event) {
					logger.WithField("deviceID", deviceID).Info("Device is stopping")
					ddapi.SafeCommand(conn, deviceID, ddapi.AvailableCommands.Stop)
				},
				"enter_open": func(ctx context.Context, e *fsm.Event) {
					logger.WithField("deviceID", deviceID).Info("Device is fully open")
				},
				"enter_closed": func(ctx context.Context, e *fsm.Event) {
					logger.WithField("deviceID", deviceID).Info("Device is fully closed")
				},
			},
		),
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
func subscribeToMQTT(mqttHandler *ddapi.MQTTHandler, prefix string, deviceFSMs map[string]*DeviceFSM, conn *dd.Conn) {
	topic := fmt.Sprintf("%s/#", prefix)
	token := mqttHandler.Client.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
		handleMQTTMessage(deviceFSMs, msg.Topic(), msg.Payload(), conn)
	})
	if token.Wait() && token.Error() != nil {
		logger.WithError(token.Error()).Fatal("Failed to subscribe to MQTT topic")
	}
	logger.Infof("Subscribed to topic: %s", topic)
}

// Handle incoming MQTT messages
func handleMQTTMessage(deviceFSMs map[string]*DeviceFSM, topic string, payload []byte, conn *dd.Conn) {
	parts := strings.Split(topic, "/")
	if len(parts) < 3 {
		logger.WithField("topic", topic).Warn("Invalid topic format")
		return
	}

	command := strings.ToUpper(string(payload))

	deviceID := parts[1]
	deviceFSM, exists := deviceFSMs[deviceID]

	if !exists {
		logger.WithField("device", deviceID).Warn("Device not exists")
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
		logger.Warnf("Unknown command for device %s: %s", deviceID, command)
	}
}

func handleStatusUpdates(conn *dd.Conn, statusCh chan ddapi.DoorStatus) {
	status := ddapi.SafeFetchStatus(conn)
	statusCh <- status
	if err := helper.LoopMessages(context.Background(), conn, statusCh); err != nil {
		logger.WithField("error", err).Fatal("Error reading messages")
	}
}
