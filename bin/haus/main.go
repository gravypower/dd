package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/samthor/dd"
	ddapi "github.com/samthor/dd/api"
	"github.com/samthor/dd/helper"
)

var (
	flagCredentialsPath = flag.String("creds", "", "path to credentials file")
	flagHost            = flag.String("host", "192.168.3.205", "host to connect to")
	flagMqtt            = flag.String("mqtt", "mqtt.haus.samthor.au", "mqtt server")
	flagMqttPort        = flag.Int("mqttPort", 1883, "mqtt port")
	flagMqttPrefix      = flag.String("mqttPrefix", "dd-door", "prefix for mqtt")
	flagDebug           = flag.Bool("debug", false, "debug mode")
)

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

	// Fetch initial list of devices.
	status, err := fetchStatus(&conn)
	if err != nil {
		log.Fatalf("Could not fetch initial statust: %v", err)
	}

	// Connect to MQTT and do/send things.
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", *flagMqtt, *flagMqttPort))
	opts.SetClientID("go_mqtt_client")

	mqttClient := mqtt.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("failed to connect to mqtt: %s/%d %v", *flagMqtt, *flagMqttPort, err)
	}

	// Recieve status updates forever.
	statusCh := make(chan ddapi.DoorStatus)
	go func() {
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
			log.Printf("TODO: set payload %+v", payload)

			var commandInput ddapi.CommandInput
			commandInput.DeviceId = deviceId
			commandInput.Action.Command = ddapi.CommandForRatio(payload.Position)
			err = conn.RPC(dd.RPC{
				Path:  "/app/res/action",
				Input: commandInput,
			})
			if err != nil {
				log.Fatalf("Could not perform action: %+v err=%v", commandInput, err)
			}

		case "get":
			status, err := fetchStatus(&conn)
			if err != nil {
				log.Fatalf("Could not fetch initial statust: %v", err)
			}
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

	for status := range statusCh {
		for _, x := range status.Devices {
			safePublish(mqttClient, x)
		}
	}
}

// safePublish sends a JSON-encoded payload for the given device and its status.
func safePublish(c mqtt.Client, d ddapi.DoorStatusDevice) {
	topic := fmt.Sprintf("%s/%s", *flagMqttPrefix, d.ID)
	payload := Payload{Position: d.Device.Position}
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

func fetchStatus(conn *dd.Conn) (ddapi.DoorStatus, error) {
	var status ddapi.DoorStatus
	err := conn.RPC(dd.RPC{
		Path:   "/app/res/devices/fetch",
		Output: &status,
	})
	return status, err
}

type Payload struct {
	Position int `json:"position"`
}
