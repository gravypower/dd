package main

import (
	"context"
	"encoding/json"
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

	conn := dd.Conn{Host: *flagHost, Debug: *flagDebug}
	err = conn.Connect(creds.Credential)
	if err != nil {
		logger.Fatalf("failed to connect: %v", err)
	}

	basicInfo := fetchBasicInfo(&conn)
	logger.WithField("basicInfo", basicInfo).Info("Fetched basic information about the connection")

	mqttClient := connectToMQTT(*flagMqtt, *flagMqttUser, *flagMqttPassword, *flagMqttPort)
	defer mqttClient.Disconnect(250)

	if *flagRemoveEntity != "" {
		ddapi.RemoveEntity(mqttClient, *flagRemoveEntity)
		return
	}

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-stopCh
		logger.Info("Shutting down gracefully...")
		ddapi.MarkAllOffline(mqttClient, *flagMqttPrefix)
		os.Exit(0)
	}()

	statusCh := make(chan ddapi.DoorStatus)
	go handleStatusUpdates(&conn, statusCh)

	subscribeToMQTT(mqttClient, *flagMqttPrefix, &conn, statusCh)

	logger.Info("Waiting on status updates...")
	for status := range statusCh {
		logger.WithField("status", status).Info("Announcing status")
		for _, device := range status.Devices {
			if !ddapi.ConfiguredDevices[device.ID] {
				ddapi.ConfigureDevice(mqttClient, *flagMqttPrefix, device, basicInfo)
			}
			ddapi.MarkOnline(mqttClient, *flagMqttPrefix, device.ID)
			safePublish(mqttClient, device)
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
		logger.Fatalf("could not get basic info: %v", err)
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
		logger.Fatalf("failed to connect to mqtt: %v", token.Error())
	}
	return client
}

func subscribeToMQTT(client mqtt.Client, prefix string, conn *dd.Conn, statusCh chan ddapi.DoorStatus) {
	subscribeTopic := fmt.Sprintf("%s/#", prefix)
	token := client.Subscribe(subscribeTopic, 0, func(c mqtt.Client, m mqtt.Message) {
		if m.Retained() {
			return
		}
		handleMQTTMessage(c, m, conn, statusCh)
	})
	<-token.Done()
	if token.Error() != nil {
		logger.Fatalf("couldn't subscribe to topic=%s, err=%v", subscribeTopic, token.Error())
	}
}

func handleMQTTMessage(client mqtt.Client, message mqtt.Message, conn *dd.Conn, statusCh chan ddapi.DoorStatus) {
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
	case "set":
		handleSetCommand(conn, deviceID, message.Payload())
	case "state":
		status := safeFetchStatus(conn)
		device := status.Get(deviceID)
		if device == nil {
			logger.WithField("deviceID", deviceID).Warn("Received request for unknown device")
			return
		}
		safePublish(client, *device)
	case "availability":
		ddapi.MarkOnline(client, *flagMqttPrefix, deviceID)
	default:
		logger.WithField("command", cmd).Warn("Unknown command received. Expected commands are 'set', 'state', or 'availability'")
	}
}

func handleSetCommand(conn *dd.Conn, deviceID string, payload []byte) {
	var p Payload
	if err := json.Unmarshal(payload, &p); err != nil {
		logger.WithField("error", err).Warn("Invalid payload in set command")
		return
	}
	if p.Stop {
		logger.WithFields(logrus.Fields{
			"deviceID":  deviceID,
			"timestamp": time.Now().Format(time.RFC3339),
		}).Info("Stopping device")
		safeCommand(conn, deviceID, ddapi.AvailableCommands.Stop)
	} else if p.Position != nil && *p.Position > 0 {
		logger.WithFields(logrus.Fields{
			"deviceID": deviceID,
			"position": *p.Position,
		}).Info("Opening device to position")
		safeCommand(conn, deviceID, ddapi.AvailableCommands.Open)
	} else {
		logger.WithField("deviceID", deviceID).Info("Closing device")
		safeCommand(conn, deviceID, ddapi.AvailableCommands.Close)
	}
}

func handleStatusUpdates(conn *dd.Conn, statusCh chan ddapi.DoorStatus) {
	status := safeFetchStatus(conn)
	statusCh <- status
	if err := helper.LoopMessages(context.Background(), conn, statusCh); err != nil {
		logger.WithField("error", err).Fatal("Error reading messages")
	}
}

func safePublish(client mqtt.Client, device ddapi.DoorStatusDevice) {
	topic := fmt.Sprintf(ddapi.StateTopicTemplate, *flagMqttPrefix, device.ID)
	payload := Payload{Position: &device.Device.Position, FromController: true}
	bytes, err := json.Marshal(payload)
	if err != nil {
		logger.WithField("error", err).Fatal("Couldn't encode payload")
	}
	tok := client.Publish(topic, 0, false, bytes)
	<-tok.Done()
	if tok.Error() != nil {
		logger.WithField("error", tok.Error()).Fatal("Couldn't publish payload")
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
