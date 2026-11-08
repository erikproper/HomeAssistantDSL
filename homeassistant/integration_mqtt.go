/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationMQTT
 *
 * PROJECT.md 39 (raw MQTT), first step (2026-10-28): Physical.def's "integration mqtt with: ...
 * end;" block declares devices whose MQTT topics we don't control and for which nothing publishes
 * discovery -- the gateways' own status topics, as integration devices (PROJECT.md 26), and
 * devices such as Shelly's. A capability names its topic (parser_mqtt_capability.go):
 *
 *   device switches.test_shelly_plug with:
 *     is retained;
 *     capability binary_sensor.node topic "shellies/shellyplug-6CC3D8/online" on "true";
 *     capability switch.switch topic "shellies/shellyplug-6CC3D8/relay/0"
 *       command "shellies/shellyplug-6CC3D8/relay/0/command";
 *     capability sensor.power topic "shellies/shellyplug-6CC3D8/relay/0/power" unit "W";
 *     capability sensor.energy topic "shellies/shellyplug-6CC3D8/relay/0/energy" unit "Wh" divided by 60;
 *   end;
 *
 * A device's node says whether it is up, and the availability of its other capabilities. A
 * "liveness with:" block makes the coordinator keep that node's topic honest when the device goes
 * silent (physical_liveness.go).
 *
 * Such a device rides on the discovery machinery: it is a gateway device (Raw) whose "raw discovery
 * config" the generator writes itself (rawMQTTConfigs) instead of the coordinator collecting one,
 * so its positioning, frozen IDs, YAML entity and retain decision are exactly a discovered device's.
 * Retain follows the same default (Architecture.md §6.16): not retained, so a shadow, unless "is
 * retained;" or one of the installation's "retained topics" filters (which may be written in this
 * block too) says so. The coordinator gets nothing about its entities: there is nothing to collect
 * or relay.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 07.11.2026
 *
 */

package main

import (
	"fmt"
	"regexp"
	"strings"

	"homeassistant/hamqtt"
)

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

// TRawTopicCapability is one capability statement of an integration mqtt device.
type TRawTopicCapability struct {
	Topic string
	On    string // binary_sensor only
	// Command is the topic a switch takes "on" and "off" on.
	Command string
	// Unit and DividedBy: a sensor's unit, and what its value is divided by to get there (0: as it is).
	Unit      string
	DividedBy int
	// Field and Types: the JSON field an event's type is in, and the types there are (event only).
	Field string
	Types []string
	// Implied: the topic isn't written in the capability's line (an instance's node).
	Implied bool
	// Governed: a governed device's entity (integration_governed.go), its line "capability <d>.<l>
	// by doing ...;".
	Governed bool
}

// rawMQTTLeaf is the leaf (raw config unique_id) the generator gives a raw capability.
func rawMQTTLeaf(deviceID, label string) string {
	return strings.NewReplacer(".", "_", "/", "_").Replace(deviceID + "_" + label)
}

// parseMQTTIntegrationBody parses an "integration mqtt" block's body into gateway devices (Raw).
func parseMQTTIntegrationBody(bodyLines []string) ([]TDiscoveryGatewayDevice, []string) {
	bodyLines, icons := cutCapabilityIcons(bodyLines)
	devices, warnings := parseMQTTIntegrationBodyLines(bodyLines)
	for _, d := range devices {
		for label, capability := range d.Capabilities {
			if icon := icons[d.DeviceID+"!"+label]; icon != "" {
				capability.Icon = icon
				d.Capabilities[label] = capability
			}
		}
	}
	return devices, warnings
}

// parseMQTTIntegrationBodyLines is parseMQTTIntegrationBody on lines without their capabilities' "with icon: ..." (capability_icon.go).
func parseMQTTIntegrationBodyLines(bodyLines []string) ([]TDiscoveryGatewayDevice, []string) {
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
			warnings = append(warnings, completeRawDevice(current)...)
			devices = append(devices, *current)
			current = nil
		case strings.HasPrefix(line, "shelly "):
			p, ok := parseShellyLine(line)
			for _, w := range p.Warnings {
				warnings = append(warnings, fmt.Sprintf("Physical.def: mqtt device %q: %s", current.DeviceID, w))
			}
			if ok {
				current.RawShelly = p.ShellyID
			}
		case line == "is retained;":
			current.Retain = retainDeclared
		case line == "is not retained;":
			current.Retain = retainNotDeclared
		case line == livenessBlockHeader:
			cur.Advance()
			decl, w := parseLivenessBlock(cur, current.DeviceID)
			warnings = append(warnings, w...)
			if decl != nil {
				current.Liveness = decl
			}
			cur.pos-- // past the block's "end;"; the loop advances
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
			// "derived capability ...": from a sibling, as on a discovered device.
			if decl, isDerived := parseDerivedCapabilityLine(line); isDerived {
				current.Capabilities[decl.Label] = TDiscoveryCapability{Domain: decl.Domain, DerivedFromCapability: decl.FromLabel, DerivedViaTemplate: decl.Template}
				continue
			}
			p, ok := parseMQTTCapabilityLine(line)
			for _, w := range p.Warnings {
				warnings = append(warnings, fmt.Sprintf("Physical.def: mqtt device %q: %s", current.DeviceID, w))
			}
			if !ok {
				continue
			}
			current.Capabilities[p.Label] = TDiscoveryCapability{Domain: p.Domain, Leaf: rawMQTTLeaf(current.DeviceID, p.Label), AssignedID: p.AssignedID}
			current.RawTopics[p.Label] = p.Topic
		}
	}
	if current != nil {
		warnings = append(warnings, fmt.Sprintf("Physical.def: mqtt device %q is missing its closing \"end;\"", current.DeviceID))
		warnings = append(warnings, completeRawDevice(current)...)
		devices = append(devices, *current)
	}
	return devices, warnings
}

// completeRawDevice fills in what device's layer implies (a Shelly's), and reports a capability
// whose topic nothing implies.
func completeRawDevice(device *TDiscoveryGatewayDevice) []string {
	var warnings []string
	if device.RawShelly != "" {
		warnings = append(warnings, completeShellyDevice(device)...)
	}
	for label, raw := range device.RawTopics {
		if raw.Implied && raw.Topic == "" {
			warnings = append(warnings, fmt.Sprintf("Physical.def: mqtt device %q: capability %q has no topic, and no \"shelly ...;\" implies one", device.DeviceID, label))
			delete(device.RawTopics, label)
			delete(device.Capabilities, label)
		}
	}
	return warnings
}

// rawMQTTConfigs returns the "raw discovery configs" the generator writes itself for every raw
// gateway device's capabilities, keyed like the coordinator's ("<domain>/<unique_id>"), so the YAML
// writer builds their entities as it does a discovered one's.
func rawMQTTConfigs(gateway TDiscoveryGatewayDevice) map[string]map[string]interface{} {
	configs := map[string]map[string]interface{}{}
	node, hasNode := gateway.RawTopics["node"]
	for label, raw := range gateway.RawTopics {
		capability := gateway.Capabilities[label]
		config := map[string]interface{}{
			"unique_id":   capability.Leaf,
			"state_topic": raw.Topic,
			"device":      map[string]interface{}{"identifiers": []interface{}{gateway.DeviceID}, "name": gateway.DeviceID},
		}
		switch capability.Domain {
		// "ON" for its value, "OFF" for any other.
		case "binary_sensor":
			config["value_template"] = fmt.Sprintf("{{ 'ON' if value == %q else 'OFF' }}", raw.On)
			config["payload_on"] = "ON"
			config["payload_off"] = "OFF"

		// The device's own "on" and "off", as state and as command.
		case "switch":
			config["command_topic"] = raw.Command
			config["payload_on"] = hamqtt.StateOn
			config["payload_off"] = hamqtt.StateOff

		// The value, with what its unit says about it.
		case "sensor":
			addRawSensorFields(config, raw)

		// The type from its JSON field; an empty one (a cleared event) is no event.
		case "event":
			var types []interface{}
			for _, eventType := range raw.Types {
				types = append(types, eventType)
			}
			config["event_types"] = types
			config["value_template"] = fmt.Sprintf(`{%% if value_json.%[1]s %%}{{ {"event_type": value_json.%[1]s} | to_json }}{%% else %%}{}{%% endif %%}`, raw.Field)
		}
		if label == "node" {
			config["device_class"] = "connectivity"
			config["entity_category"] = "diagnostic"
		} else if hasNode && node.Topic != "" {
			config["availability"] = rawNodeAvailability(node.Topic)
		}
		configs[capability.Domain+"/"+capability.Leaf] = config
	}
	return configs
}

// addRawSensorFields gives a sensor's config its unit, the device and state class that unit
// implies, and the template converting its value.
func addRawSensorFields(config map[string]interface{}, raw TRawTopicCapability) {
	if raw.Unit != "" {
		config["unit_of_measurement"] = raw.Unit
	}
	switch raw.Unit {
	case "W":
		config["device_class"] = "power"
		config["state_class"] = "measurement"
	case "Wh", "kWh":
		config["device_class"] = "energy"
		config["state_class"] = "total_increasing"
	case "°C":
		config["device_class"] = "temperature"
		config["state_class"] = "measurement"
	}
	if raw.DividedBy > 0 {
		config["value_template"] = fmt.Sprintf("{{ (value | float / %d) | round(3) }}", raw.DividedBy)
	}
}

// rawNodeAvailability is the availability of a raw device's capabilities: its node.
func rawNodeAvailability(nodeTopic string) []interface{} {
	var entries []interface{}
	for _, entry := range hamqtt.NodeAvailabilityEntries(nodeTopic) {
		entries = append(entries, entry)
	}
	return entries
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
	return "homeassistant.instances/" + instance + "/status"
}

// governedRawTopic marks a governed device's capability, whose line says "by doing ..." -- for the
// stable-id assignment's line search.
const governedRawTopic = "\x00governed"

// impliedRawTopic marks a raw capability whose topic is implied by convention, not written in its
// line (an instance's node) -- for the stable-id assignment's line search.
const impliedRawTopic = "\x00implied"

var (
	instanceDeviceHeaderPattern   = regexp.MustCompile(`^device\s+(integration\.\S+)\s+with:\s*$`)
	instanceNodeCapabilityPattern = regexp.MustCompile(`^capability\s+binary_sensor\.node(?:\s+as\s+unique_id\s+([A-Za-z_][A-Za-z0-9_.]*))?\s*;\s*$`)
)

// splitInstanceIntegrationDevices takes the "device integration.<instance> with: ... end;" block
// out of a "home_assistant <instance>" integration's body: the bridge parser gets the rest, and each
// such block becomes a raw gateway device whose node reads instanceStatusTopic.
func splitInstanceIntegrationDevices(bodyLines []string, instance string) (rest []string, devices []TDiscoveryGatewayDevice, warnings []string) {
	var current *TDiscoveryGatewayDevice
	for _, raw := range bodyLines {
		line := stripLineComment(raw)
		if current == nil {
			// Only integration.<instance> itself; another integration device on the instance (e.g.
			// integration.overkiz, PROJECT.md 25) is an ordinary bridged device.
			if m := instanceDeviceHeaderPattern.FindStringSubmatch(line); m != nil && m[1] == "integration."+instance {
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
			line, icon := cutCapabilityIcon(line)
			m := instanceNodeCapabilityPattern.FindStringSubmatch(line)
			if m == nil {
				warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line inside %q (only \"capability binary_sensor.node;\" so far): %q", current.DeviceID, line))
				continue
			}
			current.Capabilities["node"] = TDiscoveryCapability{Domain: "binary_sensor", Leaf: rawMQTTLeaf(current.DeviceID, "node"), AssignedID: m[1], Icon: icon}
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
