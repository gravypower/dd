package api

import (
	"testing"
)

func TestCommandForRatio(t *testing.T) {
	tests := []struct {
		name     string
		position int
		want     int
	}{
		{"Fully closed", 0, CMD_CLOSE},
		{"Below zero", -5, CMD_CLOSE},
		{"Pet height lower bound", 1, CMD_PET_OPEN},
		{"Pet height upper bound", 20, CMD_PET_OPEN},
		{"Parcel height lower bound", 21, CMD_PARCEL_OPEN},
		{"Parcel height upper bound", 68, CMD_PARCEL_OPEN},
		{"Fully open lower bound", 69, CMD_OPEN},
		{"Fully open", 100, CMD_OPEN},
		{"Above 100", 150, CMD_OPEN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CommandForRatio(tt.position); got != tt.want {
				t.Errorf("CommandForRatio(%d) = %d, want %d", tt.position, got, tt.want)
			}
		})
	}
}

func TestDoorStatus_IsAdmin(t *testing.T) {
	tests := []struct {
		name   string
		status DoorStatus
		want   bool
	}{
		{
			name: "Admin payload - no devices, has users",
			status: DoorStatus{
				DeviceOrder: []string{},
				Users:       []DoorStatusUsers{{Username: "admin", Enabled: true}},
			},
			want: true,
		},
		{
			name: "Non-admin payload - has devices",
			status: DoorStatus{
				DeviceOrder: []string{"device1"},
				Users:       []DoorStatusUsers{{Username: "user", Enabled: true}},
			},
			want: false,
		},
		{
			name: "Empty payload",
			status: DoorStatus{
				DeviceOrder: []string{},
				Users:       []DoorStatusUsers{},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsAdmin(); got != tt.want {
				t.Errorf("DoorStatus.IsAdmin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDoorStatus_Get(t *testing.T) {
	device1 := DoorStatusDevice{ID: "device1", Name: "Front Door"}
	device2 := DoorStatusDevice{ID: "device2", Name: "Back Door"}

	status := DoorStatus{
		Devices: []DoorStatusDevice{device1, device2},
	}

	tests := []struct {
		name     string
		deviceID string
		want     *DoorStatusDevice
	}{
		{"Find first device", "device1", &device1},
		{"Find second device", "device2", &device2},
		{"Device not found", "device3", nil},
		{"Empty ID", "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := status.Get(tt.deviceID)
			if got == nil && tt.want == nil {
				return
			}
			if got == nil || tt.want == nil {
				t.Errorf("DoorStatus.Get(%s) = %v, want %v", tt.deviceID, got, tt.want)
				return
			}
			if got.ID != tt.want.ID {
				t.Errorf("DoorStatus.Get(%s).ID = %s, want %s", tt.deviceID, got.ID, tt.want.ID)
			}
		})
	}
}

func TestGetCommandForPosition(t *testing.T) {
	tests := []struct {
		name     string
		position int
		want     int
	}{
		// Boundary tests
		{"Negative position (clamped to 0)", -10, AvailableCommands.Close},
		{"Zero position", 0, AvailableCommands.Close},
		{"Position 100", 100, AvailableCommands.Open},
		{"Over 100 (clamped)", 150, AvailableCommands.Open},

		// Percentage commands (5% increments)
		{"Position 1-5", 3, AvailableCommands.OpenPercent05},
		{"Position 5 exact", 5, AvailableCommands.OpenPercent05},
		{"Position 6-10", 8, AvailableCommands.OpenPercent10},
		{"Position 10 exact", 10, AvailableCommands.OpenPercent10},
		{"Position 15 exact", 15, AvailableCommands.OpenPercent15},
		{"Position 20 exact", 20, AvailableCommands.OpenPercent20},
		{"Position 25 exact", 25, AvailableCommands.OpenPercent25},
		{"Position 30 exact", 30, AvailableCommands.OpenPercent30},
		{"Position 35 exact", 35, AvailableCommands.OpenPercent35},
		{"Position 40 exact", 40, AvailableCommands.OpenPercent40},
		{"Position 45 exact", 45, AvailableCommands.OpenPercent45},
		{"Position 50 exact", 50, AvailableCommands.OpenPercent50},
		{"Position 55 exact", 55, AvailableCommands.OpenPercent55},
		{"Position 60 exact", 60, AvailableCommands.OpenPercent60},
		{"Position 65 exact", 65, AvailableCommands.OpenPercent65},
		{"Position 70 exact", 70, AvailableCommands.OpenPercent70},
		{"Position 75 exact", 75, AvailableCommands.OpenPercent75},
		{"Position 80 exact", 80, AvailableCommands.OpenPercent80},
		{"Position 85 exact", 85, AvailableCommands.OpenPercent85},
		{"Position 90 exact", 90, AvailableCommands.OpenPercent90},
		{"Position 95 exact", 95, AvailableCommands.OpenPercent95},

		// In-between values (should round to next 5%)
		{"Position 12 (rounds to 15%)", 12, AvailableCommands.OpenPercent15},
		{"Position 33 (rounds to 35%)", 33, AvailableCommands.OpenPercent35},
		{"Position 47 (rounds to 50%)", 47, AvailableCommands.OpenPercent50},
		{"Position 72 (rounds to 75%)", 72, AvailableCommands.OpenPercent75},

		// High positions
		{"Position 96 (opens fully)", 96, AvailableCommands.Open},
		{"Position 99 (opens fully)", 99, AvailableCommands.Open},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetCommandForPosition(tt.position)
			if got != tt.want {
				t.Errorf("GetCommandForPosition(%d) = %d, want %d", tt.position, got, tt.want)
			}
		})
	}
}

func TestGetCommandForPosition_AllPercentages(t *testing.T) {
	// Test all position values from 0-100 to ensure no panic and valid command
	for pos := 0; pos <= 100; pos++ {
		cmd := GetCommandForPosition(pos)
		if cmd < 2 || cmd > 500 {
			t.Errorf("GetCommandForPosition(%d) returned invalid command: %d", pos, cmd)
		}
	}
}
