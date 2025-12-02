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
