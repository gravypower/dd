package main

import (
	"flag"
	"log"

	"github.com/samthor/dd"
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

	var info BasicInfo
	err := conn.SimpleRequest(dd.SimpleRequest{
		Path:   "/sdk/info",
		Target: dd.SDKTarget,
		Output: &info,
	})
	if err != nil {
		log.Fatalf("could not get basic info: %v", err)
	}
	log.Printf("got basic info: %+v", info)

	// remoteRegister(shareCode, sharePassword)

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

	var empty struct{}
	res, err := conn.Request("app/res/menu", empty)
	if err != nil {
		log.Fatalf("Could not do request: %v", err)
	}

	log.Printf("Got res: %+v", res)

	messages, err := conn.Messages(true)
	if err != nil {
		log.Fatalf("Could not read messages: %v", err)
	}
	for _, m := range messages {
		var out = make(map[string]interface{})
		m.Decode(&out)
		log.Printf("got message: %+v", out)
	}
}

type BasicInfo struct {
	BaseStation string `json:"bsid"`
	Mono        int64  `json:"mono"`
	Clock       int64  `json:"clock"`
	Name        string `json:"name"`
	Version     int    `json:"version"`
}

func remoteRegister(conn *dd.Conn, code string, password string) {
	type RegisterRequest struct {
		RemoteRegistrationCode string `json:"remoteRegistrationCode"`
		UserPassword           string `json:"userPassword"`
		PhoneModel             string `json:"phoneModel"`
		PhoneName              string `json:"phoneName"` // can be renamed by user in app later
	}

	req := RegisterRequest{
		RemoteRegistrationCode: code,
		UserPassword:           password,
		PhoneName:              "lol, hi nerds",
		PhoneModel:             "lol, hi nerds",
	}
	out := make(map[string]interface{})

	err := conn.SimpleRequest(dd.SimpleRequest{
		Path:   "/app/remoteregister",
		Target: dd.RemoteTarget,
		Input:  req,
		Output: &out,
	})
	log.Printf("Got out from register: %+v", out)
	log.Printf("got err from register: %+v", err)
}
