package api

type RegisterRequest struct {
	RemoteRegistrationCode string `json:"remoteRegistrationCode"`
	UserPassword           string `json:"userPassword"`
	PhoneModel             string `json:"phoneModel"`
	PhoneName              string `json:"phoneName"` // can be renamed by user in app later
}

type RegisterResponse struct {
	BaseStation   string `json:"bsid"`
	IsAdmin       bool   `json:"isAdmin"`
	Name          string `json:"name"`
	PhoneId       string `json:"phoneId"`
	PhonePassword string `json:"phonePassword"`
	PhoneSecret   string `json:"phoneSecret"`
	UserId        string `json:"userId"`
	UserName      string `json:"userName"`
}
