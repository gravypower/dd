package api

import (
	"github.com/samthor/dd"
)

// Door command constants - these map to SmartDoor device command codes
const (
	// CMD_OPEN fully opens the door (position 100)
	CMD_OPEN = 2
	// CMD_CLOSE fully closes the door (position 0)
	CMD_CLOSE = 4
	// CMD_PET_OPEN opens the door to pet height (position ~20)
	CMD_PET_OPEN = 6
	// CMD_PARCEL_OPEN opens the door to parcel height (position ~68)
	CMD_PARCEL_OPEN = 7
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

// GetCommandForPosition maps a position percentage (0-100) to the appropriate device command.
// Uses granular percentage commands (5% increments) when available.
func GetCommandForPosition(position int) int {
	// Clamp position to valid range
	if position < 0 {
		position = 0
	}
	if position > 100 {
		position = 100
	}

	switch {
	case position == 0:
		return AvailableCommands.Close
	case position <= 5:
		return AvailableCommands.OpenPercent05
	case position <= 10:
		return AvailableCommands.OpenPercent10
	case position <= 15:
		return AvailableCommands.OpenPercent15
	case position <= 20:
		return AvailableCommands.OpenPercent20
	case position <= 25:
		return AvailableCommands.OpenPercent25
	case position <= 30:
		return AvailableCommands.OpenPercent30
	case position <= 35:
		return AvailableCommands.OpenPercent35
	case position <= 40:
		return AvailableCommands.OpenPercent40
	case position <= 45:
		return AvailableCommands.OpenPercent45
	case position <= 50:
		return AvailableCommands.OpenPercent50
	case position <= 55:
		return AvailableCommands.OpenPercent55
	case position <= 60:
		return AvailableCommands.OpenPercent60
	case position <= 65:
		return AvailableCommands.OpenPercent65
	case position <= 70:
		return AvailableCommands.OpenPercent70
	case position <= 75:
		return AvailableCommands.OpenPercent75
	case position <= 80:
		return AvailableCommands.OpenPercent80
	case position <= 85:
		return AvailableCommands.OpenPercent85
	case position <= 90:
		return AvailableCommands.OpenPercent90
	case position <= 95:
		return AvailableCommands.OpenPercent95
	default: // 96-100
		return AvailableCommands.Open
	}
}

// SafeFetchStatus fetches the door status and returns an error if it fails.
// This function no longer calls Fatal() to allow graceful error handling.
func SafeFetchStatus(conn *dd.Conn) (*DoorStatus, error) {
	var status DoorStatus
	err := conn.RPC(dd.RPC{
		Path:   "/app/res/devices/fetch",
		Output: &status,
	})
	if err != nil {
		logger.WithField("error", err).Error("Could not fetch door status")
		return nil, err
	}
	return &status, nil
}
