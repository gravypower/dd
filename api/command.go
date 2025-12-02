package api

import (
	"github.com/gravypower/dd"
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

// SafeCommand sends a command to a device and returns an error if it fails.
// This function no longer calls Fatal() to allow graceful error handling.
func SafeCommand(conn *dd.Conn, deviceID string, command int) error {

	logger.WithField("deviceID", deviceID).
		WithField("command", command).
		Info("sending command")

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
		}).Error("Could not perform RPC action")
		return err
	}
	return nil
}
