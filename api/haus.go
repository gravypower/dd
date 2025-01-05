package api

import (
	"encoding/json"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/sirupsen/logrus"
	"os"
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
	ConfiguredDevices = make(map[string]bool)
	logger            = logrus.New()
)

func init() {
	logger.SetOutput(os.Stdout)
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})
	logger.SetLevel(logrus.InfoLevel)
}

func ConfigureDevice(client mqtt.Client, prefix string, device DoorStatusDevice, basicInfo BasicInfo) {
	configTopic := fmt.Sprintf("homeassistant/cover/%s/config", device.ID)
	configPayload := map[string]interface{}{
		"name":                  device.Name,
		"command_topic":         fmt.Sprintf(CommandTopicTemplate, prefix, device.ID),
		"state_topic":           fmt.Sprintf(StateTopicTemplate, prefix, device.ID),
		"availability_topic":    fmt.Sprintf(AvailabilityTopicTemplate, prefix, device.ID),
		"payload_available":     "online",  // Define payload for "online" status
		"payload_not_available": "offline", // Define payload for "offline" status
		"device_class":          "garage",
		"unique_id":             fmt.Sprintf("cover_%s", device.ID),
		"retain":                true, // Ensure Home Assistant retains the latest state
		"qos":                   1,    // Set Quality of Service level (0, 1, or 2)
		"scan_interval":         10,
		"device": map[string]interface{}{
			"identifiers":  []string{fmt.Sprintf("garage_door_%s", device.ID)},
			"name":         basicInfo.Name,
			"manufacturer": "dd",
		},
	}

	logger.WithFields(logrus.Fields{
		"configTopic":   configTopic,
		"configPayload": configPayload,
	}).Debug("configTopic")

	bytes, err := json.Marshal(configPayload)
	if err != nil {
		logger.WithField("err", err).Error("couldn't encode config payload")
	}
	tok := client.Publish(configTopic, 0, true, bytes)
	<-tok.Done()
	if tok.Error() != nil {
		logger.WithField("err", tok.Error()).Fatal("couldn't publish config")
	}
}

func RemoveEntity(mqttClient mqtt.Client, deviceID string) {
	discoveryTopic := fmt.Sprintf(HomeAssistantConfigTopicTemplate, deviceID)
	tok := mqttClient.Publish(discoveryTopic, 0, true, "")
	tok.Wait()
	if tok.Error() != nil {
		logger.WithFields(logrus.Fields{
			"deviceID": deviceID,
			"err":      tok.Error(),
		}).Error("Failed to remove entity for device")
	} else {
		logger.WithField("deviceID", deviceID).Info("Removed entity for device")
	}
	delete(ConfiguredDevices, deviceID)
}

func MarkAllOffline(mqttClient mqtt.Client, prefix string) {
	for deviceID := range ConfiguredDevices {
		availabilityTopic := fmt.Sprintf(AvailabilityTopicTemplate, prefix, deviceID)
		tok := mqttClient.Publish(availabilityTopic, 0, true, "offline")
		tok.Wait()
		if tok.Error() != nil {
			logger.WithFields(logrus.Fields{
				"deviceID": deviceID,
				"err":      tok.Error(),
			}).Error("Failed to mark device offline")
		} else {
			logger.WithField("deviceID", deviceID).Info("Marked device as offline")
		}
	}
}

func MarkOnline(mqttClient mqtt.Client, prefix, deviceID string) {
	availabilityTopic := fmt.Sprintf(AvailabilityTopicTemplate, prefix, deviceID)
	tok := mqttClient.Publish(availabilityTopic, 0, true, "online")
	tok.Wait()
	if tok.Error() != nil {
		logger.WithFields(logrus.Fields{
			"deviceID": deviceID,
			"err":      tok.Error(),
		}).Error("Failed to mark device online")
	} else {
		logger.WithField("deviceID", deviceID).Info("Marked device as online")
	}
}
