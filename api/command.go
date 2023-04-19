package api

type CommandInput struct {
	Action struct {
		Command int `json:"cmd"`
	} `json:"action"`
	DeviceId string `json:"deviceId"`
}

type CommandOutput struct {
	Value string `json:"value"`
}
