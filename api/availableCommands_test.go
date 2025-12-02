package api

import (
	"testing"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		// Test parsing by name
		{"Open command by name", "open", AvailableCommands.Open, false},
		{"Close command by name", "close", AvailableCommands.Close, false},
		{"Stop command by name", "stop", AvailableCommands.Stop, false},
		{"Light on by name", "light_on", AvailableCommands.LightOn, false},
		{"Light off by name", "light_off", AvailableCommands.LightOff, false},
		{"Open 50% by name", "open_percent_50", AvailableCommands.OpenPercent50, false},
		{"Aux on by name", "aux_on", AvailableCommands.AuxOn, false},
		{"Phone lockout on", "phone_lockout_on", AvailableCommands.PhoneLockoutOn, false},

		// Test parsing by integer string
		{"Open command by number", "2", AvailableCommands.Open, false},
		{"Close command by number", "4", AvailableCommands.Close, false},
		{"Stop command by number", "3", AvailableCommands.Stop, false},
		{"Light on by number", "16", AvailableCommands.LightOn, false},

		// Test error cases
		{"Invalid command name", "invalid_command", 0, true},
		{"Empty string", "", 0, true},
		{"Random string", "foobar", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCommand(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCommand(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseCommand(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestAvailableCommands_Values(t *testing.T) {
	// Test that all command values are properly set
	tests := []struct {
		name  string
		value int
		want  int
	}{
		{"Open", AvailableCommands.Open, 2},
		{"Close", AvailableCommands.Close, 4},
		{"Stop", AvailableCommands.Stop, 3},
		{"PartOpen1", AvailableCommands.PartOpen1, 5},
		{"PartOpen2", AvailableCommands.PartOpen2, 6},
		{"PartOpen3", AvailableCommands.PartOpen3, 7},
		{"LightOn", AvailableCommands.LightOn, 16},
		{"LightOff", AvailableCommands.LightOff, 17},
		{"AuxOn", AvailableCommands.AuxOn, 18},
		{"AuxOff", AvailableCommands.AuxOff, 19},
		{"OpenPercent05", AvailableCommands.OpenPercent05, 32},
		{"OpenPercent50", AvailableCommands.OpenPercent50, 41},
		{"OpenPercent95", AvailableCommands.OpenPercent95, 50},
		{"PhoneLockoutOn", AvailableCommands.PhoneLockoutOn, 258},
		{"PhoneLockoutOff", AvailableCommands.PhoneLockoutOff, 257},
		{"RemoteControlLockoutOn", AvailableCommands.RemoteControlLockoutOn, 20},
		{"RemoteControlLockoutOff", AvailableCommands.RemoteControlLockoutOff, 21},
		{"EnableCycleTest", AvailableCommands.EnableCycleTest, 321},
		{"DisableCycleTest", AvailableCommands.DisableCycleTest, 322},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.want {
				t.Errorf("AvailableCommands.%s = %d, want %d", tt.name, tt.value, tt.want)
			}
		})
	}
}

func TestAvailableCommandsMap_Consistency(t *testing.T) {
	// Verify that all commands in the map match the struct values
	mapTests := map[string]int{
		"open":          AvailableCommands.Open,
		"close":         AvailableCommands.Close,
		"stop":          AvailableCommands.Stop,
		"light_on":      AvailableCommands.LightOn,
		"light_off":     AvailableCommands.LightOff,
		"open_percent_50": AvailableCommands.OpenPercent50,
	}

	for key, expectedValue := range mapTests {
		mapValue, exists := AvailableCommandsMap[key]
		if !exists {
			t.Errorf("AvailableCommandsMap missing key: %q", key)
			continue
		}
		if mapValue != expectedValue {
			t.Errorf("AvailableCommandsMap[%q] = %d, want %d", key, mapValue, expectedValue)
		}
	}
}
