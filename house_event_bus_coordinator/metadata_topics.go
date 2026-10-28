/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: MetadataTopics
 *
 * PROJECT.md 22d (2026-10-28): the coordinator isn't aware of each integration's peculiarities, so
 * the generator tells it, per device, where that device's metadata is: coordinator/
 * metadata_topics.yaml, from Physical.def's 'metadata <field> from topic "<topic>"[ value "<key>"];'
 * lines (an "integration mqtt" device's -- e.g. EMS-ESP's sw_version from ems-esp/info, field
 * "version"). The coordinator subscribes to each topic, takes the field (a JSON field, or the whole
 * message) and records it like any discovered metadata (discovery_device_info/state), which
 * generate captures into Physical.def.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 28.10.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gopkg.in/yaml.v3"
)

// TMetadataTopic is one device metadata field and where to read it.
type TMetadataTopic struct {
	Field string `yaml:"field"`
	Topic string `yaml:"topic"`
	Key   string `yaml:"value"`
}

// TMetadataTopicsFile is coordinator/metadata_topics.yaml.
type TMetadataTopicsFile struct {
	Devices map[string][]TMetadataTopic `yaml:"devices"`
}

// loadMetadataTopicsFile reads path; a missing file means none.
func loadMetadataTopicsFile(path string) (TMetadataTopicsFile, error) {
	var file TMetadataTopicsFile
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return file, nil
	}
	if err != nil {
		return file, err
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return file, fmt.Errorf("%s: %w", path, err)
	}
	return file, nil
}

// metadataValue is what a message on a metadata topic says about the field: its JSON field key,
// or (key "") the whole message; ok is false when there is nothing usable.
func metadataValue(payload []byte, key string) (string, bool) {
	if key == "" {
		value := strings.TrimSpace(string(payload))
		return value, value != ""
	}
	var body map[string]interface{}
	if err := json.Unmarshal(payload, &body); err != nil {
		return "", false
	}
	switch value := body[key].(type) {
	case string:
		return value, value != ""
	case float64, bool:
		return fmt.Sprint(value), true
	}
	return "", false
}

// startMetadataTopics subscribes to every device's metadata topics.
func startMetadataTopics(client, cloudClient mqtt.Client, ownInstallation string, file TMetadataTopicsFile) error {
	byTopic := map[string][]struct{ deviceID, field, key string }{}
	for deviceID, topics := range file.Devices {
		for _, t := range topics {
			byTopic[t.Topic] = append(byTopic[t.Topic], struct{ deviceID, field, key string }{deviceID, t.Field, t.Key})
		}
	}
	for topic, readers := range byTopic {
		readers := readers
		token := subscribe(client, topic, 0, func(_ mqtt.Client, msg mqtt.Message) {
			changed := false
			for _, r := range readers {
				if value, ok := metadataValue(msg.Payload(), r.key); ok {
					if discoveryDeviceInfo.RecordFields(r.deviceID, map[string]string{r.field: value}) {
						changed = true
					}
				}
			}
			if changed {
				discoveryDeviceInfo.SchedulePublish(client, cloudClient, ownInstallation)
			}
		})
		if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
			return fmt.Errorf("subscribing to %s: %v", topic, token.Error())
		}
	}
	if len(byTopic) > 0 {
		fmt.Printf("[metadata-topics] reading %d topic(s)\n", len(byTopic))
	}
	return nil
}
