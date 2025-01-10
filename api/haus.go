package api

import (
	"context"
	"encoding/json"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/looplab/fsm"
	"github.com/samthor/dd"
	"github.com/sirupsen/logrus"
	"os"
	"sync"
	"time"
)

const (
	CommandTopicTemplate             = "%s/%s/command"
	StateTopicTemplate               = "%s/%s/state"
	PositionTopicTemplate            = "%s/%s/position"
	SetPositionTopicTemplate         = "%s/%s/set_position"
	AvailabilityTopicTemplate        = "%s/%s/availability"
	HomeAssistantConfigTopicTemplate = "homeassistant/cover/%s/config"
)

var (
	// Map to store FSMs for devices
	DeviceFSMs = make(map[string]*DeviceFSM)
	logger     = logrus.New()
)

func init() {
	logger.SetOutput(os.Stdout)
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})
	logger.SetReportCaller(true)
	logger.SetLevel(logrus.InfoLevel)
}

// MQTTHandler centralizes MQTT operations
type MQTTHandler struct {
	Client mqtt.Client
	Mutex  sync.Mutex
	Logger *logrus.Logger
}

// DeviceFSM encapsulates a state machine for a device
type DeviceFSM struct {
	ID          string
	MQTTPrefix  string
	FSM         *fsm.FSM
	Conn        *dd.Conn
	mqttHandler *MQTTHandler
	State       string // Current state for additional tracking if needed

}

// NewMQTTHandler creates a new MQTTHandler instance
func NewMQTTHandler(client mqtt.Client, logger *logrus.Logger) *MQTTHandler {
	return &MQTTHandler{
		Client: client,
		Logger: logger,
	}
}

// Publish safely publishes a message to MQTT with a timeout
func (h *MQTTHandler) Publish(topic string, qos byte, retained bool, payload interface{}) error {
	h.Mutex.Lock()
	defer h.Mutex.Unlock()

	tok := h.Client.Publish(topic, qos, retained, payload)
	if ok := tok.WaitTimeout(3 * time.Second); !ok {
		err := tok.Error()
		h.Logger.WithFields(logrus.Fields{
			"topic":   topic,
			"payload": payload,
			"error":   err,
		}).Error("Publish timed out")
		return err
	}
	if err := tok.Error(); err != nil {
		h.Logger.WithFields(logrus.Fields{
			"topic":   topic,
			"payload": payload,
			"error":   err,
		}).Error("Failed to publish")
		return err
	}
	h.Logger.WithFields(logrus.Fields{
		"topic":   topic,
		"payload": payload,
	}).Info("Message published successfully")
	return nil
}

// Subscribe subscribes to a topic with the provided callback and ensures thread safety
func (h *MQTTHandler) Subscribe(topic string, qos byte, callback mqtt.MessageHandler) error {
	h.Mutex.Lock()
	defer h.Mutex.Unlock()

	tok := h.Client.Subscribe(topic, qos, callback)
	if ok := tok.WaitTimeout(3 * time.Second); !ok {
		h.Logger.WithFields(logrus.Fields{
			"topic": topic,
		}).Error("Subscription timed out")
		return tok.Error()
	}
	if err := tok.Error(); err != nil {
		h.Logger.WithFields(logrus.Fields{
			"topic": topic,
			"error": err,
		}).Error("Failed to subscribe")
		return err
	}

	h.Logger.WithFields(logrus.Fields{
		"topic": topic,
	}).Info("Successfully subscribed to topic")
	return nil
}

// ConfigureDevice publishes the Home Assistant MQTT cover configuration
func ConfigureDevice(handler *MQTTHandler, conn *dd.Conn, mqttPrefix string, device DoorStatusDevice, basicInfo BasicInfo) *DeviceFSM {
	configTopic := fmt.Sprintf(HomeAssistantConfigTopicTemplate, device.ID)
	configPayload := map[string]interface{}{
		"name":               device.Name,
		"command_topic":      fmt.Sprintf(CommandTopicTemplate, mqttPrefix, device.ID),
		"state_topic":        fmt.Sprintf(StateTopicTemplate, mqttPrefix, device.ID),
		"availability_topic": fmt.Sprintf(AvailabilityTopicTemplate, mqttPrefix, device.ID),
		"availability_mode":  "all",
		"device_class":       "garage",
		"unique_id":          fmt.Sprintf("cover_%s", device.ID),
		"scan_interval":      10,
		"device": map[string]interface{}{
			"identifiers":  []string{fmt.Sprintf("garage_door_%s", device.ID)},
			"name":         basicInfo.Name,
			"manufacturer": "dd",
		},
	}

	logger.WithFields(logrus.Fields{
		"configTopic":   configTopic,
		"configPayload": configPayload,
	}).Debug("configuring device")

	bytes, err := json.Marshal(configPayload)
	if err != nil {
		logger.WithField("err", err).Error("couldn't encode config payload")
		return nil
	}

	if err := handler.Publish(configTopic, 0, true, bytes); err != nil {
		logger.WithField("err", err).Fatal("Couldn't publish config")
	}
	NewDeviceFSM(device.ID, mqttPrefix, conn, handler)
	DeviceFSMs[device.ID] = NewDeviceFSM(device.ID, mqttPrefix, conn, handler)

	return DeviceFSMs[device.ID]
}

// RemoveEntity removes the Home Assistant entity for the device
func RemoveEntity(handler *MQTTHandler, deviceID string) {
	discoveryTopic := fmt.Sprintf(HomeAssistantConfigTopicTemplate, deviceID)
	if err := handler.Publish(discoveryTopic, 1, true, ""); err != nil {
		logger.WithFields(logrus.Fields{
			"deviceID": deviceID,
			"error":    err,
		}).Error("Failed to remove entity for device")
	} else {
		logger.WithField("deviceID", deviceID).Info("Removed entity for device")
	}
	delete(DeviceFSMs, deviceID)
}

// MarkOffline marks all configured devices as offline
func MarkOffline(handler *MQTTHandler, prefix string, deviceID string) {
	logger.WithField("deviceID", deviceID).Info("Marking device as offline")
	availabilityTopic := fmt.Sprintf(AvailabilityTopicTemplate, prefix, deviceID)
	if err := handler.Publish(availabilityTopic, 0, false, "offline"); err != nil {
		logger.WithFields(logrus.Fields{
			"deviceID": deviceID,
			"error":    err,
		}).Error("Failed to mark device offline")
	} else {
		logger.WithFields(logrus.Fields{
			"deviceID":          deviceID,
			"availabilityTopic": availabilityTopic,
		}).Info("Marked device as offline")
	}

}

// MarkOnline marks a specific device as online
func MarkOnline(handler *MQTTHandler, prefix, deviceID string) {
	availabilityTopic := fmt.Sprintf(AvailabilityTopicTemplate, prefix, deviceID)
	if err := handler.Publish(availabilityTopic, 0, false, "online"); err != nil {
		logger.WithFields(logrus.Fields{
			"deviceID": deviceID,
			"error":    err,
		}).Error("Failed to mark device online")
	} else {
		logger.WithFields(logrus.Fields{
			"deviceID":          deviceID,
			"availabilityTopic": availabilityTopic,
		}).Info("Marked device as online")
	}
}

const (
	CLOSE = 0
	OPEN  = 100
)

// PublishDoorState publishes the door state to the MQTT broker
func PublishDoorState(handler *MQTTHandler, prefix string, deviceID string, doorState int) {
	var haState string

	switch doorState {
	case OPEN:
		haState = "open"
	case CLOSE:
		haState = "closed"
	default:
		haState = "unknown"
	}

	stateTopic := fmt.Sprintf(StateTopicTemplate, prefix, deviceID)
	if err := handler.Publish(stateTopic, 0, false, haState); err != nil {
		logger.WithField("error", err).Fatal("Couldn't publish door state")
	}
}

// NewDeviceFSM initializes the FSM for a specific device
func NewDeviceFSM(deviceID string, mqttPrefix string, conn *dd.Conn, mqttHandler *MQTTHandler) *DeviceFSM {
	return &DeviceFSM{
		ID:          deviceID,
		MQTTPrefix:  mqttPrefix,
		Conn:        conn,
		mqttHandler: mqttHandler,
		FSM: fsm.NewFSM(
			"offline",
			fsm.Events{
				{Name: "go_online", Src: []string{"offline"}, Dst: "online"},
				{Name: "go_offline", Src: []string{"online", "opening", "closing", "open", "closed"}, Dst: "offline"},
				{Name: "open", Src: []string{"closed"}, Dst: "opening"},
				{Name: "close", Src: []string{"open"}, Dst: "closing"},
				{Name: "opened", Src: []string{"opening"}, Dst: "open"},
				{Name: "closed", Src: []string{"closing"}, Dst: "closed"},
				{Name: "stop", Src: []string{"opening", "closing"}, Dst: "stopped"},
			},
			fsm.Callbacks{
				"enter_online": func(ctx context.Context, e *fsm.Event) {
					logger.WithField("deviceID", deviceID).Info("Device is online")
					MarkOnline(mqttHandler, mqttPrefix, deviceID)
				},
				"enter_offline": func(ctx context.Context, e *fsm.Event) {
					MarkOffline(mqttHandler, mqttPrefix, deviceID)
					logger.WithField("deviceID", deviceID).Info("Device is offline")
				},
				"enter_opening": func(ctx context.Context, e *fsm.Event) {
					logger.WithField("deviceID", deviceID).Info("Device is opening")
					SafeCommand(conn, deviceID, AvailableCommands.Open)
				},
				"enter_closing": func(ctx context.Context, e *fsm.Event) {
					logger.WithField("deviceID", deviceID).Info("Device is closing")
					SafeCommand(conn, deviceID, AvailableCommands.Close)
				},
				"enter_stopped": func(ctx context.Context, e *fsm.Event) {
					logger.WithField("deviceID", deviceID).Info("Device is stopping")
					SafeCommand(conn, deviceID, AvailableCommands.Stop)
				},
				"enter_open": func(ctx context.Context, e *fsm.Event) {
					logger.WithField("deviceID", deviceID).Info("Device is fully open")
				},
				"enter_closed": func(ctx context.Context, e *fsm.Event) {
					logger.WithField("deviceID", deviceID).Info("Device is fully closed")
				},
			},
		),
	}
}
