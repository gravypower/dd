package main

import (
	"flag"
	"log"

	"github.com/samthor/dd"
	ddapi "github.com/samthor/dd/api"
	"github.com/samthor/dd/helper"
)

var (
	flagCredentialsPath = flag.String("creds", "", "path to credentials file")
	flagHost            = flag.String("host", "192.168.3.205", "host to connect to")
	flagCommand         = flag.Int("command", 0, "command to send")
)

func main() {
	flag.Parse()

	creds, err := helper.LoadCreds(*flagCredentialsPath)
	if err != nil {
		log.Fatalf("can't open credentials file: %v %v", *flagCredentialsPath, err)
	}

	conn := dd.Conn{Host: *flagHost}
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

	// Fetch list of devices and control 1st.
	var devices ddapi.DoorStatus
	err = conn.RPC(dd.RPC{
		Path:   "/app/res/devices/fetch",
		Output: &devices,
	})
	if err != nil {
		log.Fatalf("Could not do request: %v", err)
	}
	log.Printf("Got devices: %+v", devices)
	if len(devices.DeviceOrder) == 0 {
		log.Fatalf("No devices to control")
	}
	deviceId := devices.DeviceOrder[0]

	// Send the requested command.
	var commandInput ddapi.CommandInput
	commandInput.DeviceId = deviceId
	commandInput.Action.Command = *flagCommand
	var commandOutput ddapi.CommandOutput
	err = conn.RPC(dd.RPC{
		Path:   "/app/res/action",
		Input:  &commandInput,
		Output: &commandOutput,
	})
	if err != nil {
		log.Fatalf("Could not do request: %v", err)
	}
	log.Printf("Got command response: %+v", commandOutput)
}
