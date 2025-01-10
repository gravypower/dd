package api

import (
	"github.com/samthor/dd"
)

// Constants for door commands
const (
	CMD_CLOSE       = 4
	CMD_PET_OPEN    = 6
	CMD_PARCEL_OPEN = 7
	CMD_OPEN        = 2
)

// DoorStatusDevice represents the status of a single device.
type DoorStatusDevice struct {
	ID           string `json:"deviceId"`
	ScreenFormat int    `json:"screenFormat"`
	Time         int64  `json:"time"`
	Hash         int    `json:"hash"`
	Name         string `json:"name"`

	Buttons []DoorStatusButton `json:"buttons"`
	Aux     []DoorStatusButton `json:"aux"`

	Device struct {
		Position int `json:"position"` // 0-100
	} `json:"device"`

	Log struct {
		ID    int64  `json:"logId"`
		Alert int    `json:"alert"`
		Text  string `json:"text"`
		Time  int64  `json:"time"`
	} `json:"log"`
}

// DoorStatusButton represents a button displayed in the UI.
type DoorStatusButton struct {
	Action struct {
		Base    int `json:"base"`
		Command int `json:"cmd"`
	} `json:"action"`

	Title string `json:"title"`
	Icon  string `json:"icon"`
	Hide  int    `json:"hide"`
	Row   int    `json:"row"`
	Col   int    `json:"col"`
}

// DoorStatusUsers represents a user in the environment.
type DoorStatusUsers struct {
	Enabled  bool   `json:"enabled"`
	Username string `json:"userName"`
}

// DoorStatus represents the top-level status structure for all devices.
type DoorStatus struct {
	DeviceOrder []string           `json:"deviceOrder"`
	Devices     []DoorStatusDevice `json:"devices"`

	Users []DoorStatusUsers `json:"users"`
}

// IsAdmin returns whether this is an admin-only payload.
func (ds *DoorStatus) IsAdmin() bool {
	return len(ds.DeviceOrder) == 0 && len(ds.Users) > 0
}

// Get returns a DoorStatusDevice by ID.
func (ds *DoorStatus) Get(id string) *DoorStatusDevice {
	for i := range ds.Devices {
		if ds.Devices[i].ID == id {
			return &ds.Devices[i]
		}
	}
	return nil
}

// CommandForRatio returns the door command for the given position.
func CommandForRatio(position int) int {
	switch {
	case position <= 0:
		return CMD_CLOSE
	case position <= 20:
		return CMD_PET_OPEN
	case position <= 68:
		return CMD_PARCEL_OPEN
	default:
		return CMD_OPEN
	}
}

// SafeFetchStatus fetches the door status safely.
func SafeFetchStatus(conn *dd.Conn) *DoorStatus {
	var status DoorStatus
	err := conn.RPC(dd.RPC{
		Path:   "/app/res/devices/fetch",
		Output: &status,
	})
	if err != nil {
		logger.WithField("error", err).Fatal("Could not fetch door status")
	}
	return &status
}
