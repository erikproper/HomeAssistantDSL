/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationMQTT
 *
 * PROJECT.md 39 (raw MQTT), first step (2026-10-28): Physical.def's "integration mqtt with: ...
 * end;" block declares devices whose MQTT topics we don't control and for which nothing publishes
 * discovery -- the start being the gateways' own status topics, as integration devices (PROJECT.md
 * 26): Z-Wave JS UI's zwave/driver/status, EMS-ESP's ems-esp/status. A capability names its topic:
 *
 *   device integration.zwave2mqtt with:
 *     capability binary_sensor.node topic "zwave/driver/status" on "true";
 *   end;
 *
 * -- a binary_sensor, "on" for that value and "off" for any other. Such a device rides on the
 * discovery machinery: it is a gateway device (Raw) whose "raw discovery config" the generator
 * writes itself (rawMQTTConfigs) instead of the coordinator collecting one, so its positioning,
 * frozen IDs, YAML entity and retain decision are exactly a discovered device's. Retain follows the
 * same default (Architecture.md §6.16): not retained, so a shadow, unless "is retained;" or one of
 * the installation's "retained topics" filters (which may be written in this block too) says so.
 * The coordinator gets nothing about it: there is nothing to collect or relay.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 28.10.2026
 *
 */

package main

import (
	"fmt"
	"regexp"
	"strings"
)

var mqttTopicCapabilityPattern = regexp.MustCompile(`^capability\s+(binary_sensor)\.([A-Za-z_][A-Za-z0-9_/]*)\s+topic\s+"([^"]+)"\s+on\s+"([^"]*)"(?:\s+as\s+([A-Za-z_][A-Za-z0-9_.]*))?\s*;\s*$`)

// mqttMetadataTopicPattern matches 'metadata <field> from topic "<topic>"[ value "<key>"];' (PROJECT.md
// 22d, Erik, 2026-10-28): where the coordinator reads a raw device's metadata field -- <key> the
// JSON field, none for the whole message.
var mqttMetadataTopicPattern = regexp.MustCompile(`^metadata\s+([a-z_]+)\s+from\s+topic\s+"([^"]+)"(?:\s+value\s+"([^"]*)")?\s*;\s*$`)

// TRawMetadataTopic is one "metadata ... from topic ..." line of an integration mqtt device.
type TRawMetadataTopic struct {
	Field string `yaml:"field"`
	Topic string `yaml:"topic"`
	Key   string `yaml:"value,omitempty"`
}

// TRawTopicCapability is one "capability ... topic "<topic>" on "<value>";" of an integration mqtt
// device.
type TRawTopicCapability struct {
	Topic string
	On    string
	// Implied: the topic isn't written in the capability's line (an instance's node).
	Implied bool
}

// rawMQTTLeaf is the leaf (raw config unique_id) the generator gives a raw capability.
func rawMQTTLeaf(deviceID, label string) string {
	return strings.NewReplacer(".", "_", "/", "_").Replace(deviceID + "_" + label)
}

// parseMQTTIntegrationBody parses an "integration mqtt" block's body into gateway devices (Raw).
func parseMQTTIntegrationBody(bodyLines []string) ([]TDiscoveryGatewayDevice, []string) {
	cur := newLineCursor(bodyLines)
	var devices []TDiscoveryGatewayDevice
	var warnings []string
	var current *TDiscoveryGatewayDevice
	for ; !cur.AtEnd(); cur.Advance() {
		line := cur.ThisLine()
		if current == nil {
			if m := discoveryDevicePattern.FindStringSubmatch(line); m != nil {
				current = &TDiscoveryGatewayDevice{DeviceID: m[1], Identifiers: []string{m[1]}, Capabilities: map[string]TDiscoveryCapability{}, Raw: true, RawTopics: map[string]TRawTopicCapability{}}
				continue
			}
			if _, _, isRetainedTopics := parseRetainedTopicsLine(line); isRetainedTopics {
				continue // read by parseDiscoveryRetainedTopics
			}
			warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line in \"integration mqtt\" block (body line %d): %q", cur.ThisLineNumber(), line))
			continue
		}
		switch {
		case line == "end;":
			devices = append(devices, *current)
			current = nil
		case line == "is retained;":
			current.Retain = retainDeclared
		case line == "is not retained;":
			current.Retain = retainNotDeclared
		case mqttMetadataTopicPattern.MatchString(line):
			m := mqttMetadataTopicPattern.FindStringSubmatch(line)
			if !knownConstantDeviceAttributes[m[1]] {
				warnings = append(warnings, fmt.Sprintf("Physical.def: mqtt device %q: %q is no metadata field (manufacturer, model, model_id, hw_version, sw_version, serial_number, ...)", current.DeviceID, m[1]))
				continue
			}
			current.RawMetadata = append(current.RawMetadata, TRawMetadataTopic{Field: m[1], Topic: m[2], Key: m[3]})
		case discoveryMetadataPattern.MatchString(quoteDeviceReference(line)):
			// The value generate captured (writeDiscoveryMetadata), or a forced one.
			m := discoveryMetadataPattern.FindStringSubmatch(quoteDeviceReference(line))
			if current.Metadata == nil {
				current.Metadata = map[string]THostConstantAttribute{}
			}
			current.Metadata[m[1]] = THostConstantAttribute{Value: m[2], Forced: m[3] != ""}
		default:
			m := mqttTopicCapabilityPattern.FindStringSubmatch(line)
			if m == nil {
				warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line inside mqtt device %q (body line %d): %q -- a capability here is \"capability binary_sensor.<label> topic \\\"<topic>\\\" on \\\"<value>\\\";\"", current.DeviceID, cur.ThisLineNumber(), line))
				continue
			}
			current.Capabilities[m[2]] = TDiscoveryCapability{Domain: m[1], Leaf: rawMQTTLeaf(current.DeviceID, m[2]), AssignedID: m[5]}
			current.RawTopics[m[2]] = TRawTopicCapability{Topic: m[3], On: m[4]}
		}
	}
	if current != nil {
		warnings = append(warnings, fmt.Sprintf("Physical.def: mqtt device %q is missing its closing \"end;\"", current.DeviceID))
		devices = append(devices, *current)
	}
	return devices, warnings
}

// rawMQTTConfigs returns the "raw discovery configs" the generator writes itself for every raw
// gateway device's capabilities, keyed like the coordinator's ("<domain>/<unique_id>"), so the YAML
// writer builds their entities as it does a discovered one's.
func rawMQTTConfigs(gateway TDiscoveryGatewayDevice) map[string]map[string]interface{} {
	configs := map[string]map[string]interface{}{}
	for label, raw := range gateway.RawTopics {
		capability := gateway.Capabilities[label]
		config := map[string]interface{}{
			"unique_id":      capability.Leaf,
			"state_topic":    raw.Topic,
			"value_template": fmt.Sprintf("{{ 'ON' if value == %q else 'OFF' }}", raw.On),
			"payload_on":     "ON",
			"payload_off":    "OFF",
			"device":         map[string]interface{}{"identifiers": []interface{}{gateway.DeviceID}, "name": gateway.DeviceID},
		}
		if label == "node" {
			config["device_class"] = "connectivity"
			config["entity_category"] = "diagnostic"
		}
		configs[capability.Domain+"/"+capability.Leaf] = config
	}
	return configs
}

// generateMQTTIntegrationOutputs is "integration mqtt"'s entry in integrationBodyParsers: it only
// reports the block's problems -- its devices' entities come from generateYAMLMQTTEntities, and
// the coordinator gets nothing.
func generateMQTTIntegrationOutputs(bodyLines []string, ctx TPhysicalGenerationContext) error {
	_, warnings := parseMQTTIntegrationBody(bodyLines)
	for _, w := range warnings {
		fmt.Printf("[integration mqtt] %s\n", w)
	}
	return nil
}

// instanceStatusTopic is where the coordinator keeps a bridged Home Assistant instance's liveness,
// retained ("online"/"offline", from its ping-pong -- house_event_bus_coordinator/
// instance_liveness.go): a topic we control, so the instance's node is declared inside its own
// integration ("integration home_assistant <instance> with: device integration.<instance> with:
// capability binary_sensor.node; end;"), not in "integration mqtt" (Erik, 2026-10-28).
func instanceStatusTopic(instance string) string {
	return "homeassistant_instances/" + instance + "/status"
}

// impliedRawTopic marks a raw capability whose topic is implied by convention, not written in its
// line (an instance's node) -- for the stable-id assignment's line search.
const impliedRawTopic = "\x00implied"

var (
	instanceDeviceHeaderPattern   = regexp.MustCompile(`^device\s+(integration\.\S+)\s+with:\s*$`)
	instanceNodeCapabilityPattern = regexp.MustCompile(`^capability\s+binary_sensor\.node(?:\s+as\s+([A-Za-z_][A-Za-z0-9_.]*))?\s*;\s*$`)
)

// splitInstanceIntegrationDevices takes the "device integration.<instance> with: ... end;" blocks
// out of a "home_assistant <instance>" integration's body: the bridge parser gets the rest, and each
// such block becomes a raw gateway device whose node reads instanceStatusTopic.
func splitInstanceIntegrationDevices(bodyLines []string, instance string) (rest []string, devices []TDiscoveryGatewayDevice, warnings []string) {
	var current *TDiscoveryGatewayDevice
	for _, raw := range bodyLines {
		line := stripLineComment(raw)
		if current == nil {
			if m := instanceDeviceHeaderPattern.FindStringSubmatch(line); m != nil {
				current = &TDiscoveryGatewayDevice{DeviceID: m[1], Identifiers: []string{m[1]}, Capabilities: map[string]TDiscoveryCapability{}, Raw: true, RawTopics: map[string]TRawTopicCapability{}, Retain: retainDeclared, CoordinatorStatus: true}
				continue
			}
			rest = append(rest, raw)
			continue
		}
		switch {
		case line == "":
		case line == "end;":
			devices = append(devices, *current)
			current = nil
		default:
			m := instanceNodeCapabilityPattern.FindStringSubmatch(line)
			if m == nil {
				warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line inside %q (only \"capability binary_sensor.node;\" so far): %q", current.DeviceID, line))
				continue
			}
			current.Capabilities["node"] = TDiscoveryCapability{Domain: "binary_sensor", Leaf: rawMQTTLeaf(current.DeviceID, "node"), AssignedID: m[1]}
			current.RawTopics["node"] = TRawTopicCapability{Topic: instanceStatusTopic(instance), On: "online", Implied: true}
		}
	}
	if current != nil {
		devices = append(devices, *current)
	}
	return rest, devices, warnings
}

// TMetadataTopicsFile is coordinator/metadata_topics.yaml (PROJECT.md 22d): per device, the topics
// its metadata fields are read from -- the coordinator subscribes to each, and feeds what it reads
// into discovery_device_info/state, which generate captures into Physical.def.
type TMetadataTopicsFile struct {
	Devices map[string][]TRawMetadataTopic `yaml:"devices"`
}
