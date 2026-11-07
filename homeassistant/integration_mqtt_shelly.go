/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationMQTTShelly
 *
 * The Shelly layer of "integration mqtt" (PROJECT.md 39, claude-notes/plans/shelly-layer.md): a
 * device's "shelly "<id>";" statement implies, from the family its id begins with, the topics of
 * its capabilities -- still listed by label, without topic ("capability switch.switch;") -- its
 * manufacturer, model, the topics its model_id and sw_version are read from, and its liveness.
 * Anything written on the device takes precedence: a capability with its own topic, a metadata
 * line, a "liveness with:" block. Retain is never implied: it is a setting of the device.
 *
 * Only Gen1 families a real device has shown are in the table; Gen2/Plus devices have another
 * tree.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 07.11.2026
 *
 */

package main

import (
	"fmt"
	"sort"
	"strings"
)

// shellyCapability is one capability a Shelly family has: its domain and its raw topic, the
// topic relative to "shellies/<id>/".
type shellyCapability struct {
	Domain string
	Topic  TRawTopicCapability
}

// shellyPlugCapabilities are what every Shelly Plug has.
var shellyPlugCapabilities = map[string]shellyCapability{
	"node":   {"binary_sensor", TRawTopicCapability{Topic: "online", On: "true"}},
	"switch": {"switch", TRawTopicCapability{Topic: "relay/0", Command: "relay/0/command"}},
	"power":  {"sensor", TRawTopicCapability{Topic: "relay/0/power", Unit: "W"}},
	"energy": {"sensor", TRawTopicCapability{Topic: "relay/0/energy", Unit: "Wh", DividedBy: 60}}, // watt-minutes
}

// shellyFamilies are the families by the prefix of a Shelly's id ("shellyplug-6CC3D8").
var shellyFamilies = map[string]map[string]shellyCapability{
	"shellyplug": shellyPlugCapabilities,
	"shellyplug-s": withShellyCapabilities(shellyPlugCapabilities, map[string]shellyCapability{
		"temperature": {"sensor", TRawTopicCapability{Topic: "temperature", Unit: "°C"}},
		"overheated":  {"binary_sensor", TRawTopicCapability{Topic: "overtemperature", On: "1"}},
	}),
	// The i3's three inputs, and their events -- sent only by an input set to "momentary".
	"shellyix3": {
		"node":    {"binary_sensor", TRawTopicCapability{Topic: "online", On: "true"}},
		"input_1": {"binary_sensor", TRawTopicCapability{Topic: "input/0", On: "1"}},
		"input_2": {"binary_sensor", TRawTopicCapability{Topic: "input/1", On: "1"}},
		"input_3": {"binary_sensor", TRawTopicCapability{Topic: "input/2", On: "1"}},
		"event_1": {"event", TRawTopicCapability{Topic: "input_event/0", Field: "event", Types: shellyInputEvents}},
		"event_2": {"event", TRawTopicCapability{Topic: "input_event/1", Field: "event", Types: shellyInputEvents}},
		"event_3": {"event", TRawTopicCapability{Topic: "input_event/2", Field: "event", Types: shellyInputEvents}},
	},
}

// shellyInputEvents are a Gen1 input's events: short, long, double and triple short, short then
// long, long then short.
var shellyInputEvents = []string{"S", "L", "SS", "SSS", "SL", "LS"}

// shellyModels are the product names of the model codes a Shelly announces ("model").
var shellyModels = map[string]string{
	"SHPLG-1":  "Plug",
	"SHPLG2-1": "Plug E",
	"SHPLG-S":  "Plug S",
	"SHPLG-U1": "Plug US",
	"SHIX3-1":  "i3",
}

// The liveness every Shelly gets unless its device has a block of its own: after three missed
// reports (30 s apart by default), ask for its status.
const (
	shellyLivenessQuiet  = "00:01:30"
	shellyLivenessWithin = "00:00:30"
)

// withShellyCapabilities returns base with extra added.
func withShellyCapabilities(base, extra map[string]shellyCapability) map[string]shellyCapability {
	all := map[string]shellyCapability{}
	for label, capability := range base {
		all[label] = capability
	}
	for label, capability := range extra {
		all[label] = capability
	}
	return all
}

// shellyFamily returns the family of id: all of it but its last "-" part.
func shellyFamily(id string) (string, map[string]shellyCapability, bool) {
	cut := strings.LastIndex(id, "-")
	if cut <= 0 {
		return "", nil, false
	}
	family, known := shellyFamilies[id[:cut]]
	return id[:cut], family, known
}

// completeShellyDevice fills in what device's "shelly" statement implies.
func completeShellyDevice(device *TDiscoveryGatewayDevice) []string {
	id := device.RawShelly
	name, family, known := shellyFamily(id)
	if !known {
		return []string{fmt.Sprintf("Physical.def: mqtt device %q: %q is no Shelly family known here (%s)", device.DeviceID, name, strings.Join(sortedShellyFamilies(), ", "))}
	}
	base := "shellies/" + id + "/"
	var warnings []string
	for label, raw := range device.RawTopics {
		if !raw.Implied {
			continue
		}
		implied, has := family[label]
		switch {
		case !has:
			warnings = append(warnings, fmt.Sprintf("Physical.def: mqtt device %q: a %s has no capability %q", device.DeviceID, name, label))
		case implied.Domain != device.Capabilities[label].Domain:
			warnings = append(warnings, fmt.Sprintf("Physical.def: mqtt device %q: a %s's %q is a %s", device.DeviceID, name, label, implied.Domain))
		default:
			topic := implied.Topic
			topic.Topic = base + topic.Topic
			if topic.Command != "" {
				topic.Command = base + topic.Command
			}
			topic.Implied = true
			device.RawTopics[label] = topic
		}
	}

	if device.Metadata == nil {
		device.Metadata = map[string]THostConstantAttribute{}
	}
	if _, has := device.Metadata["manufacturer"]; !has {
		device.Metadata["manufacturer"] = THostConstantAttribute{Value: "Shelly"}
	}
	if model := shellyModels[device.Metadata["model_id"].Value]; model != "" {
		if _, has := device.Metadata["model"]; !has {
			device.Metadata["model"] = THostConstantAttribute{Value: model}
		}
	}
	for _, field := range []TRawMetadataTopic{{Field: "model_id", Key: "model"}, {Field: "sw_version", Key: "fw_ver"}} {
		if !hasRawMetadataField(device.RawMetadata, field.Field) {
			field.Topic = base + "announce"
			device.RawMetadata = append(device.RawMetadata, field)
		}
	}

	if device.Liveness == nil {
		device.Liveness = &TLivenessDecl{
			Device: device.DeviceID, Traffic: base + "#", Quiet: shellyLivenessQuiet,
			Ignore: []string{base + "command", base + "relay/0/command", base + "#/retain"},
			Ask:    base + "command", AskPayload: "announce",
			Answer: base + "info", AnswerField: "wifi_sta.connected", Within: shellyLivenessWithin,
			Verdict: base + "online", Online: "true", Offline: "false",
			Misses: livenessDefaultMisses,
		}
	}
	return warnings
}

// hasRawMetadataField reports whether topics already say where field is read from.
func hasRawMetadataField(topics []TRawMetadataTopic, field string) bool {
	for _, topic := range topics {
		if topic.Field == field {
			return true
		}
	}
	return false
}

// sortedShellyFamilies are the known families, for a message.
func sortedShellyFamilies() []string {
	var names []string
	for name := range shellyFamilies {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
