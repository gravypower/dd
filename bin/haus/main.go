package main

import (
	"context"
	"flag"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/looplab/fsm"
	"github.com/samthor/dd"
	ddapi "github.com/samthor/dd/api"
	"github.com/samthor/dd/helper"
	"github.com/sirupsen/logrus"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var (
	logger              = logrus.New()
	flagCredentialsPath = flag.String("creds", "creds.json", "path to credentials file")
	flagHost            = flag.String("host", "", "host to connect to")
	flagMqtt            = flag.String("mqtt", "", "mqtt server")
	flagMqttPort        = flag.Int("mqttPort", 1883, "mqtt port")
	flagMqttUser        = flag.String("mqttUser", "", "mqtt user")
	flagMqttPassword    = flag.String("mqttPassword", "", "mqtt password")
	flagMqttPrefix      = flag.String("mqttPrefix", "dd-door", "prefix for mqtt")
	flagRemoveEntity    = flag.String("removeEntity", "", "remove entity")
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

	creds, err := helper.LoadCreds(*flagCredentialsPath)
	if err != nil {
		logger.Fatalf("can't open credentials file: %v %v", *flagCredentialsPath, err)
	}

	ddfsm := fsm.NewFSM(
		"offline",
		fsm.Events{
			{Name: "go_online", Src: []string{"offline"}, Dst: "online"},
			{Name: "go_offline", Src: []string{"online", "opening", "closing", "open", "closed"}, Dst: "offline"},
			{Name: "open", Src: []string{"closed"}, Dst: "opening"},
			{Name: "close", Src: []string{"open"}, Dst: "closing"},
			{Name: "opened", Src: []string{"opening"}, Dst: "open"},
			{Name: "closed", Src: []string{"closing"}, Dst: "closed"},
		},
		fsm.Callbacks{
			"enter_online": func(_ context.Context, e *fsm.Event) {
				fmt.Println("Cover is online")
			},
			"enter_offline": func(_ context.Context, e *fsm.Event) {
				fmt.Println("Cover is offline")
			},
			"enter_opening": func(_ context.Context, e *fsm.Event) {
				fmt.Println("Cover is opening")
			},
			"enter_closing": func(_ context.Context, e *fsm.Event) {
				fmt.Println("Cover is closing")
			},
			"enter_open": func(_ context.Context, e *fsm.Event) {
				fmt.Println("Cover is fully open")
			},
			"enter_closed": func(_ context.Context, e *fsm.Event) {
				fmt.Println("Cover is fully closed")
			},
		},
	)

	fmt.Println(ddfsm.Current())

	conn := dd.Conn{Host: *flagHost, Debug: *flagDebug}
	err = conn.Connect(creds.Credential)
	if err != nil {
		logger.Fatalf("failed to connect: %v", err)
	}

	basicInfo := fetchBasicInfo(&conn)
	logger.WithField("basicInfo", basicInfo).Info("Fetched basic information about the connection")

	mqttClient := connectToMQTT(*flagMqtt, *flagMqttUser, *flagMqttPassword, *flagMqttPort)
	defer mqttClient.Disconnect(250)

	mqttHandler := ddapi.NewMQTTHandler(mqttClient, logger)

	if *flagRemoveEntity != "" {
		ddapi.RemoveEntity(mqttHandler, *flagRemoveEntity)
		return
	}

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-stopCh
		logger.Info("Shutting down gracefully.")
		err = ddfsm.Event(context.Background(), "go_offline")
		if err != nil {
			fmt.Println("Error:", err)
		}
		ddapi.MarkAllOffline(mqttHandler, *flagMqttPrefix)
		os.Exit(0)
	}()

	statusCh := make(chan ddapi.DoorStatus)
	go handleStatusUpdates(&conn, statusCh)

	subscribeToMQTT(mqttHandler, *flagMqttPrefix, &conn, statusCh)

	logger.Info("Waiting on status updates.")
	for status := range statusCh {
		logger.WithField("status", status).Info("Announcing status")
		for _, device := range status.Devices {
			if !ddapi.ConfiguredDevices[device.ID] {
				ddapi.ConfigureDevice(mqttHandler, *flagMqttPrefix, device, basicInfo)
			} else {
				logger.WithField("deviceID", device.ID).Info("Device already configured")
			}
			ddapi.MarkOnline(mqttHandler, *flagMqttPrefix, device.ID)

			if ddfsm.Event(context.Background(), "go_online") != nil {
				fmt.Println("Error:", err)
			}

			ddapi.PublishDoorState(mqttHandler, *flagMqttPrefix, device.ID, device.Device.Position)
		}
	}
}

func fetchBasicInfo(conn *dd.Conn) ddapi.BasicInfo {
	var info ddapi.BasicInfo
	err := conn.SimpleRequest(dd.SimpleRequest{
		Path:   "/sdk/info",
		Target: dd.SDKTarget,
		Output: &info,
	})
	if err != nil {
		logger.WithError(err).Fatalf("could not get basic info")
	}
	return info
}

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
		logger.WithError(token.Error()).Fatalf("failed to connect to mqtt")
	}
	return client
}

func subscribeToMQTT(handler *ddapi.MQTTHandler, prefix string, conn *dd.Conn, statusCh chan ddapi.DoorStatus) {
	subscribeTopic := fmt.Sprintf("%s/#", prefix)
	err := handler.Subscribe(subscribeTopic, 0, func(c mqtt.Client, m mqtt.Message) {
		if m.Retained() {
			return
		}
		handleMQTTMessage(handler, m, conn, statusCh)
	})
	if err != nil {
		logger.WithError(err).WithField("subscribeTopic", subscribeTopic).Fatalf("couldn't subscribe to topic")
	}
}

func handleMQTTMessage(handler *ddapi.MQTTHandler, message mqtt.Message, conn *dd.Conn, statusCh chan ddapi.DoorStatus) {
	parts := strings.Split(message.Topic(), "/")
	if len(parts) != 3 || parts[2] == "" {
		return
	}
	deviceID, cmd := parts[1], parts[2]
	logger.WithFields(logrus.Fields{
		"topic":    message.Topic(),
		"deviceID": deviceID,
		"command":  cmd,
	}).Info("Received MQTT request")

	switch cmd {
	case "command":
		handleCommand(conn, deviceID, message.Payload())
	case "state":
		logger.WithField("message", message).Debug("handleMQTTMessage safeFetchStatus")
		status := safeFetchStatus(conn)
		device := status.Get(deviceID)
		if device == nil {
			logger.WithField("deviceID", deviceID).Warn("Received request for unknown device")
			return
		}
		ddapi.PublishDoorState(handler, *flagMqttPrefix, device.ID, device.Device.Position)
	case "availability":
		ddapi.MarkOnline(handler, *flagMqttPrefix, deviceID)
	default:
		logger.WithFields(logrus.Fields{
			"command": cmd,
			"Payload": message.Payload(),
		}).Warn("Unknown command received. Expected commands are 'set', 'state', or 'availability'")
	}
}

func handleCommand(conn *dd.Conn, deviceID string, payload []byte) {
	payloadStr := strings.TrimSpace(string(payload)) // Convert to string and trim whitespace
	logger.WithField("payload", payloadStr).Debug("Processing payload")

	switch payloadStr {
	case "OPEN":
		logger.WithField("deviceID", deviceID).Info("Opening device")
		safeCommand(conn, deviceID, ddapi.AvailableCommands.Open)
	case "CLOSE":
		logger.WithField("deviceID", deviceID).Info("Closing device")
		safeCommand(conn, deviceID, ddapi.AvailableCommands.Close)
	case "STOP":
		logger.WithField("deviceID", deviceID).Info("Stopping device")
		safeCommand(conn, deviceID, ddapi.AvailableCommands.Stop)
	default:
		logger.WithField("payload", payloadStr).Warn("Unknown command")
	}
}

func handleStatusUpdates(conn *dd.Conn, statusCh chan ddapi.DoorStatus) {
	status := safeFetchStatus(conn)
	statusCh <- status
	if err := helper.LoopMessages(context.Background(), conn, statusCh); err != nil {
		logger.WithField("error", err).Fatal("Error reading messages")
	}
}

func safeFetchStatus(conn *dd.Conn) ddapi.DoorStatus {
	var status ddapi.DoorStatus
	err := conn.RPC(dd.RPC{
		Path:   "/app/res/devices/fetch",
		Output: &status,
	})
	if err != nil {
		logger.WithField("error", err).Fatal("Could not fetch status")
	}
	return status
}

func safeCommand(conn *dd.Conn, deviceID string, command int) {
	var commandInput ddapi.CommandInput
	commandInput.DeviceId = deviceID
	commandInput.Action.Command = command
	err := conn.RPC(dd.RPC{
		Path:  "/app/res/action",
		Input: commandInput,
	})
	if err != nil {
		logger.WithFields(logrus.Fields{
			"commandInput": commandInput,
			"error":        err,
		}).Fatal("Could not perform action")
	}
}

type Payload struct {
	FromController bool `json:"from_controller,omitempty"`
	Position       *int `json:"position"`
	Stop           bool `json:"stop,omitempty"`
	Active         bool `json:"active,omitempty"`
}
