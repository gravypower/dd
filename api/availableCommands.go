package api

import (
	"errors"
	"strconv"
)

// AvailableCommands contains all SmartDoor device command codes.
// These integer codes are sent to the device to trigger specific actions.
// Command code ranges:
//   - 2-7: Basic door operations (open, close, partial opens)
//   - 16-21: Light and auxiliary controls
//   - 32-50: Percentage-based door positions (5% to 95%)
//   - 20-21, 257-258: Lockout controls
//   - 321-322: Cycle testing
//   - 352-355: Camera alarm controls
var AvailableCommands = struct {
	AuxOff                   int
	AuxOn                    int
	Close                    int
	Open                     int
	Stop                     int
	LightOn                  int
	LightOff                 int
	OpenPercent05            int
	OpenPercent10            int
	OpenPercent15            int
	OpenPercent20            int
	OpenPercent25            int
	OpenPercent30            int
	OpenPercent35            int
	OpenPercent40            int
	OpenPercent45            int
	OpenPercent50            int
	OpenPercent55            int
	OpenPercent60            int
	OpenPercent65            int
	OpenPercent70            int
	OpenPercent75            int
	OpenPercent80            int
	OpenPercent85            int
	OpenPercent90            int
	OpenPercent95            int
	PartOpen1                int
	PartOpen2                int
	PartOpen3                int
	PhoneLockoutOff          int
	PhoneLockoutOn           int
	RemoteControlLockoutOff  int
	RemoteControlLockoutOn   int
	CameraAudioAlarmDisable  int
	CameraAudioAlarmEnable   int
	CameraMotionAlarmDisable int
	CameraMotionAlarmEnable  int
	DisableCycleTest         int
	EnableCycleTest          int
}{
	AuxOff:                   19,
	AuxOn:                    18,
	Close:                    4,
	Open:                     2,
	Stop:                     3,
	LightOn:                  16,
	LightOff:                 17,
	OpenPercent05:            32,
	OpenPercent10:            33,
	OpenPercent15:            34,
	OpenPercent20:            35,
	OpenPercent25:            36,
	OpenPercent30:            37,
	OpenPercent35:            38,
	OpenPercent40:            39,
	OpenPercent45:            40,
	OpenPercent50:            41,
	OpenPercent55:            42,
	OpenPercent60:            43,
	OpenPercent65:            44,
	OpenPercent70:            45,
	OpenPercent75:            46,
	OpenPercent80:            47,
	OpenPercent85:            48,
	OpenPercent90:            49,
	OpenPercent95:            50,
	PartOpen1:                5,
	PartOpen2:                6,
	PartOpen3:                7,
	PhoneLockoutOff:          257,
	PhoneLockoutOn:           258,
	RemoteControlLockoutOff:  21,
	RemoteControlLockoutOn:   20,
	CameraAudioAlarmDisable:  355,
	CameraAudioAlarmEnable:   354,
	CameraMotionAlarmDisable: 353,
	CameraMotionAlarmEnable:  352,
	DisableCycleTest:         322,
	EnableCycleTest:          321,
}

var AvailableCommandsMap = map[string]int{
	"aux_off":                     AvailableCommands.AuxOff,
	"aux_on":                      AvailableCommands.AuxOn,
	"close":                       AvailableCommands.Close,
	"open":                        AvailableCommands.Open,
	"stop":                        AvailableCommands.Stop,
	"light_on":                    AvailableCommands.LightOn,
	"light_off":                   AvailableCommands.LightOff,
	"open_percent_05":             AvailableCommands.OpenPercent05,
	"open_percent_10":             AvailableCommands.OpenPercent10,
	"open_percent_15":             AvailableCommands.OpenPercent15,
	"open_percent_20":             AvailableCommands.OpenPercent20,
	"open_percent_25":             AvailableCommands.OpenPercent25,
	"open_percent_30":             AvailableCommands.OpenPercent30,
	"open_percent_35":             AvailableCommands.OpenPercent35,
	"open_percent_40":             AvailableCommands.OpenPercent40,
	"open_percent_45":             AvailableCommands.OpenPercent45,
	"open_percent_50":             AvailableCommands.OpenPercent50,
	"open_percent_55":             AvailableCommands.OpenPercent55,
	"open_percent_60":             AvailableCommands.OpenPercent60,
	"open_percent_65":             AvailableCommands.OpenPercent65,
	"open_percent_70":             AvailableCommands.OpenPercent70,
	"open_percent_75":             AvailableCommands.OpenPercent75,
	"open_percent_80":             AvailableCommands.OpenPercent80,
	"open_percent_85":             AvailableCommands.OpenPercent85,
	"open_percent_90":             AvailableCommands.OpenPercent90,
	"open_percent_95":             AvailableCommands.OpenPercent95,
	"part_open_1":                 AvailableCommands.PartOpen1,
	"part_open_2":                 AvailableCommands.PartOpen2,
	"part_open_3":                 AvailableCommands.PartOpen3,
	"phone_lockout_off":           AvailableCommands.PhoneLockoutOff,
	"phone_lockout_on":            AvailableCommands.PhoneLockoutOn,
	"remote_control_lockout_off":  AvailableCommands.RemoteControlLockoutOff,
	"remote_control_lockout_on":   AvailableCommands.RemoteControlLockoutOn,
	"camera_audio_alarm_disable":  AvailableCommands.CameraAudioAlarmDisable,
	"camera_audio_alarm_enable":   AvailableCommands.CameraAudioAlarmEnable,
	"camera_motion_alarm_disable": AvailableCommands.CameraMotionAlarmDisable,
	"camera_motion_alarm_enable":  AvailableCommands.CameraMotionAlarmEnable,
	"disable_cycle_test":          AvailableCommands.DisableCycleTest,
	"enable_cycle_test":           AvailableCommands.EnableCycleTest,
}

// ParseCommand converts a string command to its integer value.
func ParseCommand(command string) (int, error) {

	// Try to parse the input as an integer directly
	if value, err := strconv.Atoi(command); err == nil {
		return value, nil
	}

	if value, exists := AvailableCommandsMap[command]; exists {
		return value, nil
	}
	return 0, errors.New("command not found")
}
