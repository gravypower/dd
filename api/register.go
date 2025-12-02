package api

import (
	"github.com/gravypower/dd"
)

type RegisterRequest struct {
	RemoteRegistrationCode string `json:"remoteRegistrationCode"`
	UserPassword           string `json:"userPassword"`
	PhoneModel             string `json:"phoneModel"`
	PhoneName              string `json:"phoneName"` // can be renamed by user in app later
}

type RegisterResponse struct {
	dd.Credential        // this includes UserPassword, not actually part of response
	IsAdmin       bool   `json:"isAdmin,omitempty"`
	Name          string `json:"name,omitempty"`
	UserId        string `json:"userId,omitempty"`
	UserName      string `json:"userName,omitempty"`
}
