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
	DeviceFSMs = make(map[string]*DeviceFSM)
	logger     = logrus.New()
)

func init() {
	logger.SetOutput(os.Stdout)
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})
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
	State       string
}

// NewMQTTHandler creates a new MQTTHandler instance
func NewMQTTHandler(client mqtt.Client, logger *logrus.Logger) *MQTTHandler {
	return &MQTTHandler{
		Client: client,
		Logger: logger,
	}
}

// publishToMQTT is a helper method to centralize MQTT publish logic
func (h *MQTTHandler) publishToMQTT(topic string, qos byte, retained bool, payload interface{}) error {
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
	}).Debug("Message published successfully")
	return nil
}

// PublishStatus publishes a device's status to the appropriate topic
func (h *MQTTHandler) PublishStatus(prefix, deviceID, status string) error {
	topic := fmt.Sprintf(StateTopicTemplate, prefix, deviceID)
	return h.publishToMQTT(topic, 0, false, status)
}

// PublishStatus publishes a device's status to the appropriate topic
func (h *MQTTHandler) PublishAvailability(prefix, deviceID, availability string) error {
	topic := fmt.Sprintf(AvailabilityTopicTemplate, prefix, deviceID)
	return h.publishToMQTT(topic, 0, false, availability)
}

// RemoveEntity removes the Home Assistant entity for the device
func (h *MQTTHandler) RemoveEntity(deviceID string) error {
	discoveryTopic := fmt.Sprintf(HomeAssistantConfigTopicTemplate, deviceID)
	err := h.publishToMQTT(discoveryTopic, 0, false, "")
	if err != nil {
		h.Logger.WithFields(logrus.Fields{
			"deviceID": deviceID,
			"error":    err,
		}).Error("Failed to remove entity for device")
		return err
	}
	h.Logger.WithField("deviceID", deviceID).Info("Removed entity for device")
	return nil
}

// ConfigureDevice publishes the Home Assistant MQTT cover configuration
func ConfigureDevice(handler *MQTTHandler, conn *dd.Conn, mqttPrefix string, device DoorStatusDevice, basicInfo BasicInfo) *DeviceFSM {
	configTopic := fmt.Sprintf(HomeAssistantConfigTopicTemplate, device.ID)
	configPayload := map[string]interface{}{
		"name":                  device.Name,
		"command_topic":         fmt.Sprintf(CommandTopicTemplate, mqttPrefix, device.ID),
		"state_topic":           fmt.Sprintf(StateTopicTemplate, mqttPrefix, device.ID),
		"availability_topic":    fmt.Sprintf(AvailabilityTopicTemplate, mqttPrefix, device.ID),
		"availability_mode":     "latest",
		"payload_open":          "go_open",
		"payload_close":         "go_close",
		"state_open":            "open",
		"state_closed":          "closed",
		"state_opening":         "opening",
		"state_closing":         "closing",
		"state_stopping":        "stopping",
		"payload_available":     "online",
		"payload_not_available": "offline",
		"optimistic":            false,
		"retain":                false,
		"device_class":          "garage",
		"expire_after":          60,
		"unique_id":             fmt.Sprintf("cover_%s", device.ID),
		"scan_interval":         10,
		"device": map[string]interface{}{
			"identifiers":  []string{fmt.Sprintf("garage_door_%s", device.ID)},
			"name":         basicInfo.Name,
			"manufacturer": "dd",
		},
		"icon": "mdi:garage",
	}

	bytes, err := json.Marshal(configPayload)
	if err != nil {
		logger.WithField("err", err).Error("Couldn't encode config payload")
		return nil
	}

	if err := handler.publishToMQTT(configTopic, 0, false, bytes); err != nil {
		logger.WithField("err", err).Fatal("Couldn't publish config")
	}

	DeviceFSMs[device.ID] = NewDeviceFSM(device.ID, mqttPrefix, conn, handler)
	return DeviceFSMs[device.ID]
}

// NewDeviceFSM initializes the FSM for a specific device
func NewDeviceFSM(deviceID string, mqttPrefix string, conn *dd.Conn, mqttHandler *MQTTHandler) *DeviceFSM {
	return &DeviceFSM{
		ID:          deviceID,
		MQTTPrefix:  mqttPrefix,
		Conn:        conn,
		mqttHandler: mqttHandler,
		FSM: fsm.NewFSM(
			"initial",
			fsm.Events{
				{Name: "go_online", Src: []string{"offline", "initial"}, Dst: "online"},
				{Name: "go_offline", Src: []string{"online", "opening", "closing", "open", "closed", "stopping", "stopped"}, Dst: "offline"},
				{Name: "go_open", Src: []string{"closed", "stopped"}, Dst: "opening"},
				{Name: "go_close", Src: []string{"open", "stopped"}, Dst: "closing"},
				{Name: "go_opened", Src: []string{"online", "opening", "open", "closing", "closed", "stopping", "stopped"}, Dst: "open"},
				{Name: "go_closed", Src: []string{"online", "opening", "open", "closing", "closed", "stopping", "stopped"}, Dst: "closed"},
				{Name: "go_stop", Src: []string{"online", "opening", "open", "closing", "closed"}, Dst: "stopping"},
				{Name: "go_stopped", Src: []string{"stopping"}, Dst: "stopped"},
			},
			fsm.Callbacks{
				"enter_online": func(ctx context.Context, e *fsm.Event) {
					err := mqttHandler.PublishAvailability(mqttPrefix, deviceID, "online")
					if err != nil {
						logger.WithError(err).WithField("deviceID", deviceID).Error("Error setting Device online")
						return
					}
					logger.WithField("deviceID", deviceID).Info("Device is online")
				},
				"enter_offline": func(ctx context.Context, e *fsm.Event) {
					err := mqttHandler.PublishAvailability(mqttPrefix, deviceID, "offline")
					if err != nil {
						logger.WithError(err).WithField("deviceID", deviceID).Error("Error setting Device offline")
						return
					}
					logger.WithField("deviceID", deviceID).Info("Device is offline")
				},
				"enter_opening": func(ctx context.Context, e *fsm.Event) {
					err := mqttHandler.PublishStatus(mqttPrefix, deviceID, "opening")
					SafeCommand(conn, deviceID, AvailableCommands.Open)
					if err != nil {
						logger.WithError(err).WithField("deviceID", deviceID).Error("Error setting Device to opening")
						return
					}
					logger.WithField("deviceID", deviceID).Info("Device is Opening")
				},
				"enter_closing": func(ctx context.Context, e *fsm.Event) {
					err := mqttHandler.PublishStatus(mqttPrefix, deviceID, "closing")
					SafeCommand(conn, deviceID, AvailableCommands.Close)
					if err != nil {
						logger.WithError(err).WithField("deviceID", deviceID).Error("Error setting Device to closing")
						return
					}
					logger.WithField("deviceID", deviceID).Info("Device is Closing")
				},
				"enter_stopping": func(ctx context.Context, e *fsm.Event) {
					logger.WithField("deviceID", deviceID).Info("Device is Stopping")
					err := mqttHandler.PublishStatus(mqttPrefix, deviceID, "stopping")
					SafeCommand(conn, deviceID, AvailableCommands.Stop)
					if err != nil {
						logger.WithError(err).WithField("deviceID", deviceID).Error("Error setting Device to stopping")
						return
					}

				},
				"enter_open": func(ctx context.Context, e *fsm.Event) {
					err := mqttHandler.PublishStatus(mqttPrefix, deviceID, "open")
					if err != nil {
						logger.WithError(err).WithField("deviceID", deviceID).Error("Error setting Device to opened")
						return
					}
					logger.WithField("deviceID", deviceID).Info("Device is fully Opened")
				},
				"enter_closed": func(ctx context.Context, e *fsm.Event) {
					err := mqttHandler.PublishStatus(mqttPrefix, deviceID, "closed")
					if err != nil {
						logger.WithError(err).WithField("deviceID", deviceID).Error("Error setting Device to closed")
						return
					}
					logger.WithField("deviceID", deviceID).Info("Device is fully Closed")
				},
			},
		),
	}
}
