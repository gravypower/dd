package main

import (
	"flag"
	"log"
	"time"

	"github.com/samthor/dd"
	ddapi "github.com/samthor/dd/api"
)

var (
	flagDebug   = flag.Bool("debug", false, "whether to be in debug mode")
	flagAddress = flag.String("ip", "192.168.3.205", "address to connect to")
)

const (
	versionUse = "2.21.1"
)

func main() {
	flag.Parse()

	conn := dd.Conn{
		Version:            versionUse,
		Host:               *flagAddress,
		InsecureSkipVerify: true,
		Debug:              *flagDebug,
	}

	var info ddapi.BasicInfo
	err := conn.SimpleRequest(dd.SimpleRequest{
		Path:   "/sdk/info",
		Target: dd.SDKTarget,
		Output: &info,
	})
	if err != nil {
		log.Fatalf("could not get basic info: %v", err)
	}
	log.Printf("got basic info: %+v", info)

	cred := dd.Credential{
		BaseStation:   info.BaseStation,
		PhoneSecret:   "gSFVYjhgNFdHs8hq",
		Phone:         "NFu65QB5jBr0YQhV",
		UserPassword:  sharePassword,
		PhonePassword: "QsgEAJF3vQQi2AMB",
	}
	err = conn.Connect(cred)
	if err != nil {
		log.Fatalf("Could not connect: %v", err)
	}

	go func() {
		for {
			messages, err := conn.Messages()
			if err != nil {
				log.Fatalf("Err fetching background messages: %v", err)
			}
			// TODO: This is probably a status change (button change).
			// Just decodes payload right now, no ID of what it is/was?
			for _, m := range messages {
				var out ddapi.DoorStatus
				m.Decode(&out)
				log.Printf("Got status message: %+v", out)
			}
			time.Sleep(time.Second * 4)
		}
	}()

	var deviceId string

	for range []int{1, 2, 3} {
		var devices ddapi.DoorStatus
		err = conn.RPC(dd.RPC{
			Path:   "/app/res/devices/fetch",
			Output: &devices,
		})
		if err != nil {
			log.Fatalf("Could not do request: %v", err)
		}
		log.Printf("Got devices: %+v", devices)

		deviceId = devices.DeviceOrder[0]
	}

	var commandInput ddapi.CommandInput
	commandInput.DeviceId = deviceId
	commandInput.Action.Command = ddapi.CommandForRatio(0)
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

	time.Sleep(time.Hour)
}

func remoteRegister(conn *dd.Conn, code string, password string) {
	req := ddapi.RegisterRequest{
		RemoteRegistrationCode: code,
		UserPassword:           password,
		PhoneName:              "lol, hi nerds",
		PhoneModel:             "lol, hi nerds",
	}
	out := ddapi.RegisterResponse{}

	err := conn.SimpleRequest(dd.SimpleRequest{
		Path:   "/app/remoteregister",
		Target: dd.RemoteTarget,
		Input:  req,
		Output: &out,
	})
	log.Printf("Got out from register: %+v", out)
	log.Printf("got err from register: %+v", err)
}
