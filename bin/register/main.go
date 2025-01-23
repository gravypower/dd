package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/samthor/dd"
	ddapi "github.com/samthor/dd/api"
)

var (
	flagCredentialsPath = flag.String("credentials", "dd-credentials.json", "path to credentials file")
	flagShareCode       = flag.String("code", "", "share code")
	flagPassword        = flag.String("password", "", "password")
	flagPhoneInfo       = flag.String("phone", "API", "phone info to report")
)

func main() {
	flag.Parse()

	if *flagShareCode == "" || *flagPassword == "" {
		log.Fatalf("must specify -code and -password")
	}

	f, err := os.Create(*flagCredentialsPath)
	if err != nil {
		log.Fatalf("can't create credentials file: %v %v", *flagCredentialsPath, err)
	}

	req := ddapi.RegisterRequest{
		RemoteRegistrationCode: *flagShareCode,
		UserPassword:           *flagPassword,
		PhoneName:              *flagPhoneInfo,
		PhoneModel:             *flagPhoneInfo,
	}
	out := ddapi.RegisterResponse{}

	conn := dd.Conn{}
	err = conn.SimpleRequest(dd.SimpleRequest{
		Path:   "/app/remoteregister",
		Target: dd.RemoteTarget,
		Input:  req,
		Output: &out,
	})
	if err != nil {
		log.Fatalf("can't remoteregister: %+v %v", req, err)
	}

	out.UserPassword = *flagPassword

	err = json.NewEncoder(f).Encode(out)
	if err != nil {
		log.Fatalf("can't encode response: %+v %v", out, err)
	}

	log.Printf("Ok! Saved at: %v", *flagCredentialsPath)
}
