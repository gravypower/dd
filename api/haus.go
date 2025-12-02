package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/looplab/fsm"
	"github.com/samthor/dd"
	"github.com/sirupsen/logrus"
)

const (
	CommandTopicTemplate                           = "%s/%s/command"
	StateTopicTemplate                             = "%s/%s/state"
	PositionTopicTemplate                          = "%s/%s/position"
	SetPositionTopicTemplate                       = "%s/%s/set_position"
	AvailabilityTopicTemplate                      = "%s/%s/availability"
	HomeAssistantConfigTopicTemplate               = "homeassistant/cover/%s/config"
	publishTimeout                   time.Duration = 10 * time.Second
)

var (
	DeviceFSMs = make(map[string]*DeviceFSM)
	// deviceFSMsMutex protects concurrent access to DeviceFSMs map
	deviceFSMsMutex sync.RWMutex
	logger          = logrus.New()
)

func init() {
	logger.SetOutput(os.Stdout)
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})
	logger.SetLevel(logrus.InfoLevel)
}

// GetDeviceFSM safely retrieves a device FSM by ID
func GetDeviceFSM(deviceID string) (*DeviceFSM, bool) {
	deviceFSMsMutex.RLock()
	defer deviceFSMsMutex.RUnlock()
	fsm, exists := DeviceFSMs[deviceID]
	return fsm, exists
}

// SetDeviceFSM safely sets a device FSM
func SetDeviceFSM(deviceID string, fsm *DeviceFSM) {
	deviceFSMsMutex.Lock()
	defer deviceFSMsMutex.Unlock()
	DeviceFSMs[deviceID] = fsm
}

// GetAllDeviceFSMs safely returns all device FSMs (used for shutdown)
func GetAllDeviceFSMs() map[string]*DeviceFSM {
	deviceFSMsMutex.RLock()
	defer deviceFSMsMutex.RUnlock()
	// Return a copy to avoid holding the lock
	devices := make(map[string]*DeviceFSM, len(DeviceFSMs))
	for k, v := range DeviceFSMs {
		devices[k] = v
	}
	return devices
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
	mu          sync.Mutex
}

// Trigger triggers an event on the device FSM.
// Note: Do not hold d.mu while invoking FSM.Event, as callbacks (e.g., enter_state)
// also acquire d.mu and would deadlock. The FSM itself handles its internal concurrency.
func (d *DeviceFSM) Trigger(ctx context.Context, event string) error {
	return d.FSM.Event(ctx, event)
}

// Current returns the current state in a thread-safe way
func (d *DeviceFSM) Current() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.FSM == nil {
		return ""
	}
	return d.FSM.Current()
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

	if !h.Client.IsConnected() {
		err := fmt.Errorf("mqtt not connected; cannot publish to %s", topic)
		h.Logger.WithFields(logrus.Fields{
			"topic":   topic,
			"payload": payload,
			"error":   err,
		}).Error("Publish skipped: not connected")
		return err
	}

	tok := h.Client.Publish(topic, qos, retained, payload)
	if ok := tok.WaitTimeout(publishTimeout); !ok {
		err := fmt.Errorf("mqtt publish to %s timed out after %s", topic, publishTimeout)
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

// PublishAvailability publishes a device's availability to the appropriate topic
func (h *MQTTHandler) PublishAvailability(prefix, deviceID, availability string) error {
	topic := fmt.Sprintf(AvailabilityTopicTemplate, prefix, deviceID)
	return h.publishToMQTT(topic, 0, true, availability)
}

// RemoveEntity removes the Home Assistant entity for the device
func (h *MQTTHandler) RemoveEntity(deviceID string) error {
	discoveryTopic := fmt.Sprintf(HomeAssistantConfigTopicTemplate, deviceID)
	err := h.publishToMQTT(discoveryTopic, 0, true, "")
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

	if err := handler.publishToMQTT(configTopic, 0, true, bytes); err != nil {
		logger.WithField("err", err).Error("Couldn't publish config; will retry in background")
		// Retry in background without killing the process, as broker/network may be slow on startup
		go func() {
			for attempt := 1; attempt <= 5; attempt++ {
				delay := time.Duration(attempt) * 5 * time.Second
				time.Sleep(delay)
				if err := handler.publishToMQTT(configTopic, 0, true, bytes); err == nil {
					logger.WithFields(logrus.Fields{"attempt": attempt}).Info("Published config successfully after retry")
					return
				}
				logger.WithFields(logrus.Fields{"attempt": attempt}).Warn("Retry to publish config failed; will retry again if attempts remain")
			}
		}()
	}

	deviceFSM := NewDeviceFSM(device.ID, mqttPrefix, conn, handler)
	SetDeviceFSM(device.ID, deviceFSM)
	return deviceFSM
}

// NewDeviceFSM initializes the FSM for a specific device
func NewDeviceFSM(deviceID string, mqttPrefix string, conn *dd.Conn, mqttHandler *MQTTHandler) *DeviceFSM {
	df := &DeviceFSM{
		ID:          deviceID,
		MQTTPrefix:  mqttPrefix,
		Conn:        conn,
		mqttHandler: mqttHandler,
	}

	f := fsm.NewFSM(
		"initial",
		fsm.Events{
			{Name: "go_online", Src: []string{"offline", "initial"}, Dst: "online"},
			{Name: "go_offline", Src: []string{"online", "opening", "closing", "open", "closed", "stopping", "stopped"}, Dst: "offline"},
			{Name: "go_open", Src: []string{"online", "closed", "stopped"}, Dst: "opening"},
			{Name: "go_close", Src: []string{"online", "open", "stopped"}, Dst: "closing"},
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
				if err != nil {
					logger.WithError(err).WithField("deviceID", deviceID).Error("Error setting Device to opening")
					return
				}
				err = SafeCommand(conn, deviceID, AvailableCommands.Open)
				if err != nil {
					logger.WithError(err).WithField("deviceID", deviceID).Error("Error sending open command")
					return
				}
				logger.WithField("deviceID", deviceID).Info("Device is Opening")
			},
			"enter_closing": func(ctx context.Context, e *fsm.Event) {
				err := mqttHandler.PublishStatus(mqttPrefix, deviceID, "closing")
				if err != nil {
					logger.WithError(err).WithField("deviceID", deviceID).Error("Error setting Device to closing")
					return
				}
				err = SafeCommand(conn, deviceID, AvailableCommands.Close)
				if err != nil {
					logger.WithError(err).WithField("deviceID", deviceID).Error("Error sending close command")
					return
				}
				logger.WithField("deviceID", deviceID).Info("Device is Closing")
			},
			"enter_stopping": func(ctx context.Context, e *fsm.Event) {
				logger.WithField("deviceID", deviceID).Info("Device is Stopping")
				err := mqttHandler.PublishStatus(mqttPrefix, deviceID, "stopping")
				if err != nil {
					logger.WithError(err).WithField("deviceID", deviceID).Error("Error setting Device to stopping")
					return
				}
				err = SafeCommand(conn, deviceID, AvailableCommands.Stop)
				if err != nil {
					logger.WithError(err).WithField("deviceID", deviceID).Error("Error sending stop command")
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
			"enter_state": func(ctx context.Context, e *fsm.Event) {
				// keep an internal copy of the current state
				df.mu.Lock()
				df.State = e.Dst
				df.mu.Unlock()
			},
			"after_event": func(ctx context.Context, e *fsm.Event) {
				logger.WithFields(logrus.Fields{
					"deviceID": deviceID,
					"event":    e.Event,
					"src":      e.Src,
					"dst":      e.Dst,
				}).Debug("FSM transition complete")
			},
			"error": func(ctx context.Context, e *fsm.Event) {
				// log and ignore invalid transitions
				logger.WithFields(logrus.Fields{
					"deviceID": deviceID,
					"event":    e.Event,
					"src":      e.Src,
					"err":      e.Err,
				}).Warn("FSM transition error")
			},
		},
	)

	df.FSM = f
	return df
}
