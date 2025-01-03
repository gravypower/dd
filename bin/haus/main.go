package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/samthor/dd"
	ddapi "github.com/samthor/dd/api"
	"github.com/samthor/dd/helper"
)

var (
	flagCredentialsPath = flag.String("creds", "creds.json", "path to credentials file")
	flagHost            = flag.String("host", "", "host to connect to")
	flagMqtt            = flag.String("mqtt", "", "mqtt server")
	flagMqttPort        = flag.Int("mqttPort", 1883, "mqtt port")
	flagMqttUser        = flag.String("mqttUser", "", "mqtt user")
	flagMqttPassword    = flag.String("mqttPassword", "", "mqtt password")
	flagMqttPrefix      = flag.String("mqttPrefix", "dd-door", "prefix for mqtt")
	flagDebug           = flag.Bool("debug", false, "debug mode")
)

var configuredDevices = make(map[string]bool)

func main() {
	flag.Parse()

	creds, err := helper.LoadCreds(*flagCredentialsPath)
	if err != nil {
		log.Fatalf("can't open credentials file: %v %v", *flagCredentialsPath, err)
	}

	conn := dd.Conn{Host: *flagHost, Debug: *flagDebug}
	err = conn.Connect(creds.Credential)
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}

	// Fetch basic info from SDK endpoint.
	var info ddapi.BasicInfo
	err = conn.SimpleRequest(dd.SimpleRequest{
		Path:   "/sdk/info",
		Target: dd.SDKTarget,
		Output: &info,
	})
	if err != nil {
		log.Fatalf("could not get basic info: %v", err)
	}
	log.Printf("basic info: %+v", info)

	// Connect to MQTT and do/send things.
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", *flagMqtt, *flagMqttPort))
	opts.SetClientID("go_mqtt_client")

	if *flagMqttUser != "" {
		opts.SetUsername(*flagMqttUser)
	}

	if *flagMqttPassword != "" {
		opts.SetPassword(*flagMqttPassword)
	}

	mqttClient := mqtt.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("failed to connect to mqtt: %s/%d %v", *flagMqtt, *flagMqttPort, err)
	}

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-stopCh
		log.Println("Shutting down gracefully...")
		markAllOffline(mqttClient, *flagMqttPrefix)
		os.Exit(0)
	}()

	// Recieve status updates forever.
	statusCh := make(chan ddapi.DoorStatus)
	go func() {
		status := safeFetchStatus(&conn)
		statusCh <- status // force initial status to be treated inline
		err := helper.LoopMessages(context.Background(), &conn, statusCh)
		if err != nil {
			log.Fatalf("err reading messages: %v", err)
		}
	}()

	subscribeTopic := fmt.Sprintf("%s/#", *flagMqttPrefix)
	tok := mqttClient.Subscribe(subscribeTopic, 0, func(c mqtt.Client, m mqtt.Message) {

		if m.Retained() {
			return // ignore retained
		}

		parts := strings.Split(m.Topic(), "/")
		if len(parts) != 3 || parts[2] == "" {
			return // not useful
		}

		deviceId := parts[1]
		cmd := parts[2]
		log.Printf("got mqtt request: deviceId=%v cmd=%v", deviceId, cmd)

		switch cmd {
		case "set":
			bytes := m.Payload()
			var payload Payload
			err := json.Unmarshal(bytes, &payload)
			if err != nil {
				log.Printf("got invalid payload in set: %v", err)
				return
			}
			if payload.Stop {
				log.Printf("[%v]: stopping", deviceId)
				// TODO: what
				safeCommand(&conn, deviceId, ddapi.AvailableCommands.Stop)
			} else if payload.Position != nil && *payload.Position > 0 {
				log.Printf("[%v]: cowardly opening all the way for position=%v", deviceId, *payload.Position)
				safeCommand(&conn, deviceId, ddapi.AvailableCommands.Open)
			} else if payload.Position != nil {
				log.Printf("[%v]: closing", deviceId)
				safeCommand(&conn, deviceId, ddapi.AvailableCommands.Close)
			} else {
				log.Printf("[%v]: got misunderstood payload, ignoring: %+v", deviceId, payload)
			}

		case "get":
			status := safeFetchStatus(&conn)
			d := status.Get(deviceId)
			if d == nil {
				log.Printf("Got request for unknown device: %s", deviceId)
				return
			}
			safePublish(mqttClient, *d)

		default:
			return
		}

	})
	<-tok.Done() // wait for sub to start
	if tok.Error() != nil {
		log.Fatalf("couldn't subscribe to topic=%s, err=%v", subscribeTopic, tok.Error())
	}

	log.Printf("waiting on status...")
	for status := range statusCh {

		log.Printf("announcing status: %+v", status)
		for _, device := range status.Devices {
			// Configure the entity only if needed
			if !configuredDevices[device.ID] {
				configureEntity(mqttClient, *flagMqttPrefix, device)
			}
			markOnline(mqttClient, *flagMqttPrefix, device.ID)
			safePublish(mqttClient, device)
		}
	}
}

// safePublish sends a JSON-encoded payload to MQTT for the given device and its status, as a general announcement.
// Aborts if this fails.
func safePublish(c mqtt.Client, d ddapi.DoorStatusDevice) {
	topic := fmt.Sprintf("%s/%s/state", *flagMqttPrefix, d.ID)

	payload := Payload{Position: &d.Device.Position, FromController: true}
	bytes, err := json.Marshal(payload)
	if err != nil {
		log.Fatalf("couldn't encode payload for mqtt: %v", err)
	}

	log.Printf("publishing topic=%v payload=%+v", topic, payload)
	tok := c.Publish(topic, 0, false, bytes)
	<-tok.Done()
	if tok.Error() != nil {
		log.Fatalf("couldn't publish to topic=%s, err=%v", topic, tok.Error())
	}
}

// Fetches the current status from the device. Crashes on fail.
func safeFetchStatus(conn *dd.Conn) ddapi.DoorStatus {
	var status ddapi.DoorStatus
	err := conn.RPC(dd.RPC{
		Path:   "/app/res/devices/fetch",
		Output: &status,
	})
	if err != nil {
		log.Fatalf("could not fetch status: %v", err)
	}
	return status
}

// Performs the given command. Crashes on fail.
func safeCommand(conn *dd.Conn, deviceId string, command int) {
	var commandInput ddapi.CommandInput
	commandInput.DeviceId = deviceId
	commandInput.Action.Command = command
	err := conn.RPC(dd.RPC{
		Path:  "/app/res/action",
		Input: commandInput,
	})
	if err != nil {
		log.Fatalf("Could not perform action: %+v err=%v", commandInput, err)
	}
}

func configureEntity(mqttClient mqtt.Client, prefix string, device ddapi.DoorStatusDevice) {
	discoveryTopic := fmt.Sprintf("homeassistant/cover/%s/config", device.ID)

	configPayload := map[string]interface{}{
		"name":               device.Name, // Use device name dynamically
		"command_topic":      fmt.Sprintf("%s/%s/command", prefix, device.ID),
		"state_topic":        fmt.Sprintf("%s/%s/state", prefix, device.ID),
		"position_topic":     fmt.Sprintf("%s/%s/position", prefix, device.ID),
		"set_position_topic": fmt.Sprintf("%s/%s/set_position", prefix, device.ID),
		"availability_topic": fmt.Sprintf("%s/%s/availability", prefix, device.ID),
		"payload_open":       "open",
		"payload_close":      "close",
		"payload_stop":       "stop",
		"state_open":         "open",
		"state_closed":       "closed",
		"state_stopped":      "stopped",
		"device_class":       "garage",
		"unique_id":          fmt.Sprintf("garage_door_%s", device.ID),
	}

	payload, err := json.Marshal(configPayload)
	if err != nil {
		log.Fatalf("Failed to encode discovery payload: %v", err)
	}

	tok := mqttClient.Publish(discoveryTopic, 0, true, payload)
	tok.Wait()
	if tok.Error() != nil {
		log.Fatalf("Failed to publish discovery payload: %v", tok.Error())
	}

	// Mark the device as online
	markOnline(mqttClient, prefix, device.ID)

	configuredDevices[device.ID] = true
	log.Printf("Device '%s' (ID: %s) has been configured.", device.Name, device.ID)
}

func removeEntity(mqttClient mqtt.Client, prefix, deviceID string) {
	discoveryTopic := fmt.Sprintf("homeassistant/cover/%s/config", deviceID)
	tok := mqttClient.Publish(discoveryTopic, 0, true, "")
	tok.Wait()
	if tok.Error() != nil {
		log.Printf("Failed to remove entity for device %s: %v", deviceID, tok.Error())
	} else {
		log.Printf("Removed entity for device %s.", deviceID)
	}
	delete(configuredDevices, deviceID)
}

func markAllOffline(mqttClient mqtt.Client, prefix string) {
	for deviceID := range configuredDevices {
		availabilityTopic := fmt.Sprintf("%s/%s/availability", prefix, deviceID)
		tok := mqttClient.Publish(availabilityTopic, 0, true, "offline")
		tok.Wait()
		if tok.Error() != nil {
			log.Printf("Failed to mark device %s offline: %v", deviceID, tok.Error())
		}
	}
}

func markOnline(mqttClient mqtt.Client, prefix, deviceID string) {
	availabilityTopic := fmt.Sprintf("%s/%s/availability", prefix, deviceID)
	tok := mqttClient.Publish(availabilityTopic, 0, true, "online")
	tok.Wait()
	if tok.Error() != nil {
		log.Printf("Failed to mark device %s online: %v", deviceID, tok.Error())
	} else {
		log.Printf("Marked device %s as online.", deviceID)
	}
}

type Payload struct {
	FromController bool `json:"from_controller,omitempty"` // stub to force change
	Position       *int `json:"position"`                  // reported by device
	Stop           bool `json:"stop,omitempty"`            // can be triggered by caller
	Active         bool `json:"active,omitempty"`          // whether it might be active (moving)
}
