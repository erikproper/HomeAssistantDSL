/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: SharedAvailability
 *
 * PROJECT.md 22b (2026-10-28): a discovered device's entities get their availability from its
 * gateway's discovery configs, which mix two kinds of entries: the gateway's own, shared by all
 * its devices (Zigbee2MQTT's zigbee2mqtt/bridge/state, Z-Wave JS UI's zwave/driver/status and
 * zwave/_CLIENTS/<client>/status, EMS-ESP's ems-esp/status), and the device's own (Zigbee2MQTT's
 * zigbee2mqtt/<device>/availability, a Z-Wave node's status). A device that only sends when used
 * -- a battery-less Hue Tap -- is marked offline by its gateway after a while without messages,
 * though it works fine when pressed. Its declaration then says "ignore own availability;", and its
 * entities keep only the shared entries: available exactly as long as the gateway is.
 *
 * Which entries are shared is read from the data, not from gateway names: an availability topic
 * that the configs of more than one device carry is the gateway's; one only this device carries is
 * its own (Architecture.md §6.16). The same rule will let integration devices (PROJECT.md 26)
 * suggest which devices belong to them.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 28.10.2026
 *
 */

package main

import (
	"strings"

	"homeassistant/hamqtt"
)

// sharedAvailabilityTopics returns every availability topic that the raw configs of more than one
// device carry (a device being its first "identifiers" value), as Home Assistant reads them
// (abbreviations expanded, "~" substituted).
func sharedAvailabilityTopics(configs TDiscoveryRawConfigs) map[string]bool {
	devicesByTopic := map[string]map[string]bool{}
	for _, gatewayConfigs := range configs {
		for key, raw := range gatewayConfigs {
			domain, _, _ := strings.Cut(key, "/")
			config := hamqtt.DiscoveredConfig(copyConfig(raw), domain)
			deviceID := firstIdentifier(config["device"])
			if deviceID == "" {
				continue
			}
			for _, topic := range availabilityTopicsOf(config) {
				if devicesByTopic[topic] == nil {
					devicesByTopic[topic] = map[string]bool{}
				}
				devicesByTopic[topic][deviceID] = true
			}
		}
	}
	shared := map[string]bool{}
	for topic, devices := range devicesByTopic {
		if len(devices) > 1 {
			shared[topic] = true
		}
	}
	return shared
}

// keepSharedAvailability drops from body's availability every entry whose topic isn't in shared,
// and returns the topics it dropped. When no shared entry would remain, body is left as it is
// (and kept reports why): an entity must not end up without any availability.
func keepSharedAvailability(body map[string]interface{}, shared map[string]bool) (dropped []string, kept bool) {
	list, isList := body["availability"].([]interface{})
	if !isList {
		if topic, single := body["availability_topic"].(string); single && topic != "" {
			if shared[topic] {
				return nil, true
			}
			return nil, false
		}
		return nil, true
	}
	var keep []interface{}
	for _, item := range list {
		entry, _ := item.(map[string]interface{})
		topic, _ := entry["topic"].(string)
		if shared[topic] {
			keep = append(keep, item)
		} else {
			dropped = append(dropped, topic)
		}
	}
	if len(keep) == 0 {
		return nil, false
	}
	body["availability"] = keep
	return dropped, true
}

// availabilityTopicsOf returns config's availability topics, list or single form.
func availabilityTopicsOf(config map[string]interface{}) []string {
	var topics []string
	if list, isList := config["availability"].([]interface{}); isList {
		for _, item := range list {
			if entry, isMap := item.(map[string]interface{}); isMap {
				if topic, ok := entry["topic"].(string); ok && topic != "" {
					topics = append(topics, topic)
				}
			}
		}
	}
	if topic, ok := config["availability_topic"].(string); ok && topic != "" {
		topics = append(topics, topic)
	}
	return topics
}

func firstIdentifier(device interface{}) string {
	block, _ := device.(map[string]interface{})
	switch ids := block["identifiers"].(type) {
	case []interface{}:
		if len(ids) > 0 {
			if id, ok := ids[0].(string); ok {
				return id
			}
		}
	case string:
		return ids
	}
	return ""
}

// copyConfig is a shallow copy of raw, with its availability list copied too: DiscoveredConfig
// writes the expanded entries back into that list.
func copyConfig(raw map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(raw))
	for key, value := range raw {
		if list, isList := value.([]interface{}); isList {
			value = append([]interface{}{}, list...)
		}
		out[key] = value
	}
	return out
}
