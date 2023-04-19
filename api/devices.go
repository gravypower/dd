package api

// DoorStatusDevice is the status of a single device.
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
		Time  int    `json:"time"`
	}
}

// DoorStatusButton is a button displayed in the UI.
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

// DoorStatusUsers lists the users available to the environment.
type DoorStatusUsers struct {
	Enabled  bool   `json:"enabled"`
	Username string `json:"userName"`
}

// DoorStatus is the top-level status structure for all devices.
// This is emitted regularly without a ProcessID, and is the response type for "/app/res/devices/fetch".
type DoorStatus struct {
	DeviceOrder []string           `json:"deviceOrder"`
	Devices     []DoorStatusDevice `json:"devices"`

	// we might also _just_ see Users (the "admin update" payload)
	Users []DoorStatusUsers `json:"users"`
}

// IsAdmin returns whether this is an admin-only payload.
func (ds *DoorStatus) IsAdmin() bool {
	return len(ds.DeviceOrder) == 0 && len(ds.Users) > 0
}

// Get returns a DoorStatus by ID.
func (ds *DoorStatus) Get(id string) *DoorStatusDevice {
	for i := range ds.Devices {
		if ds.Devices[i].ID == id {
			return &ds.Devices[i]
		}
	}
	return nil
}

// Returns the door command for the given position.
// This might not be the same if you changed it. :shrug:
func CommandForRatio(position int) int {
	if position <= 0 {
		return 4 // close
	} else if position <= 20 {
		return 6 // pet: 10%
	} else if position <= 68 {
		return 7 // parcel: 34%
	} else {
		return 2 // open
	}
}
