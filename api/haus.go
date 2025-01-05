package api

import (
	"encoding/json"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"log"
)

const (
	CommandTopicTemplate             = "%s/%s/command"
	StateTopicTemplate               = "%s/%s/state"
	PositionTopicTemplate            = "%s/%s/position"
	SetPositionTopicTemplate         = "%s/%s/set_position"
	AvailabilityTopicTemplate        = "%s/%s/availability"
	HomeAssistantConfigTopicTemplate = "homeassistant/cover/%s/config"
)

var ConfiguredDevices = make(map[string]bool)

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
		"device": map[string]interface{}{
			"identifiers":  []string{fmt.Sprintf("garage_door_%s", device.ID)},
			"name":         basicInfo.Name,
			"manufacturer": "dd",
		},
	}
	log.Printf("configTopic: %s", configTopic)
	log.Printf("configPayload: %s", configPayload)
	bytes, err := json.Marshal(configPayload)
	if err != nil {
		log.Fatalf("couldn't encode config payload: %v", err)
	}
	tok := client.Publish(configTopic, 0, true, bytes)
	<-tok.Done()
	if tok.Error() != nil {
		log.Fatalf("couldn't publish config: %v", tok.Error())
	}
}

func RemoveEntity(mqttClient mqtt.Client, deviceID string) {
	discoveryTopic := fmt.Sprintf(HomeAssistantConfigTopicTemplate, deviceID)
	tok := mqttClient.Publish(discoveryTopic, 0, true, "")
	tok.Wait()
	if tok.Error() != nil {
		log.Printf("Failed to remove entity for device %s: %v", deviceID, tok.Error())
	} else {
		log.Printf("Removed entity for device %s.", deviceID)
	}
	delete(ConfiguredDevices, deviceID)
}

func MarkAllOffline(mqttClient mqtt.Client, prefix string) {
	for deviceID := range ConfiguredDevices {
		availabilityTopic := fmt.Sprintf(AvailabilityTopicTemplate, prefix, deviceID)
		tok := mqttClient.Publish(availabilityTopic, 0, true, "offline")
		tok.Wait()
		if tok.Error() != nil {
			log.Printf("Failed to mark device %s offline: %v", deviceID, tok.Error())
		}
	}
}

func MarkOnline(mqttClient mqtt.Client, prefix, deviceID string) {
	availabilityTopic := fmt.Sprintf(AvailabilityTopicTemplate, prefix, deviceID)
	tok := mqttClient.Publish(availabilityTopic, 0, true, "online")
	tok.Wait()
	if tok.Error() != nil {
		log.Printf("Failed to mark device %s online: %v", deviceID, tok.Error())
	} else {
		log.Printf("Marked device %s as online.", deviceID)
	}
}
