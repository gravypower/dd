package helper

import (
	"encoding/json"
	"os"

	ddapi "github.com/gravypower/dd/api"
)

// LoadCreds loads a RegisterResponse from disk.
func LoadCreds(p string) (*ddapi.RegisterResponse, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var creds ddapi.RegisterResponse
	err = json.NewDecoder(f).Decode(&creds)
	return &creds, err
}
