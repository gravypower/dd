package api

import (
	"github.com/samthor/dd"
	"github.com/sirupsen/logrus"
)

type CommandInput struct {
	Action struct {
		Command int `json:"cmd"`
	} `json:"action"`
	DeviceId string `json:"deviceId"`
}

type CommandOutput struct {
	Value string `json:"value"`
}

func SafeCommand(conn *dd.Conn, deviceID string, command int) {
	var commandInput CommandInput
	commandInput.DeviceId = deviceID
	commandInput.Action.Command = command
	err := conn.RPC(dd.RPC{
		Path:  "/app/res/action",
		Input: commandInput,
	})
	if err != nil {
		logger.WithFields(logrus.Fields{
			"commandInput": commandInput,
			"error":        err,
		}).Fatal("Could not perform action")
	}
}
