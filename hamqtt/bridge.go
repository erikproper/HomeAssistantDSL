/*
 *
 * Module:    HomeAssistant
 * Package:   HAMQTT
 * Component: Bridge
 *
 * A "home_assistant" bridge entity's conceptual config (PROJECT.md 19h.3, 2026-10-27): an entity of
 * another Home Assistant instance (ha2mqtt, or main itself), reported over MQTT by the generator's
 * reporting automations on homeassistant_instances/<instance>/bridge/<entity>/..., as the main
 * instance gets it. Shared by the coordinator's relay (its discovery, and its cloud copy for an
 * exported device) and the generator's YAML entities, so the two can never differ. The rules, and
 * the live bugs behind them, are documented at house_event_bus_coordinator/discoveryhassbridge.go's
 * buildHassBridgeEntityDiscoveryBody.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 27.10.2026
 *
 */

package hamqtt

import (
	"fmt"
	"strings"
)

// Payloads of the bridge's topics: a node/liveness topic carries "true"/"false" (hosts,
// commandline) or a proxied entity's own "on"/"off"; a proxied entity's state may be "unavailable".
const (
	StateOn          = "on"
	StateOff         = "off"
	StateAvailable   = "available"
	StateUnavailable = "unavailable"
	BoolTrue         = "true"
)

// BridgeStateTopic and its siblings are a bridge entity's topics, as the reporting automations
// publish them and the commands are received.
func BridgeStateTopic(instance, localEntity string) string {
	return "homeassistant_instances/" + instance + "/bridge/" + localEntity + "/state"
}

func BridgeAttributesTopic(instance, localEntity string) string {
	return "homeassistant_instances/" + instance + "/bridge/" + localEntity + "/attributes"
}

func BridgeCommandTopic(instance, localEntity string) string {
	return "homeassistant_instances/" + instance + "/bridge/" + localEntity + "/command"
}

// Command is one command a bridge entity accepts (homeassistant/hassbridge_commands.go).
type Command struct {
	Payload      string
	DiscoveryKey string
	ValueTrigger bool
	DataKey      string
	TopicKey     string
}

// TopicKey is the config key the command's topic goes under.
func (c Command) Key() string {
	if c.TopicKey != "" {
		return c.TopicKey
	}
	return "command_topic"
}

// BridgeCommandTopicFor is name's command topic: its own sub-topic for a value-carrying command.
func BridgeCommandTopicFor(instance, localEntity, name string, command Command) string {
	base := BridgeCommandTopic(instance, localEntity)
	if command.ValueTrigger {
		return base + "/" + name
	}
	return base
}

// BridgeUniqueID is a bridge entity's unique_id: its frozen stable id, else the derived one.
func BridgeUniqueID(localEntity, stableID string) string {
	if stableID != "" {
		return stableID
	}
	return "hassbridge_" + strings.ReplaceAll(localEntity, ".", "_")
}

// NodeAvailabilityEntries carries the rules of the coordinator's former nodeAvailabilityEntries
// (moved here 2026-10-27); its documentation, with the live bug behind it, follows unchanged.
//
// nodeAvailabilityEntries builds one TAvailabilityEntry per non-empty nodeTopic -- a device's own
// connectivity signal, or (PROJECT.md's logical-layer work) another device's own connectivity
// signal this one was declared to "depend on" (Logical.def "dependency on <other-id>;", resolved
// generator-side, already flattened across the full transitive dependency chain -- the coordinator
// never walks that graph itself, it just ANDs whatever flat topic list it's handed). Empty strings
// are skipped, so a caller can pass its own possibly-"" node topic (already gated via
// availabilityTopicFor -- "" for the node capability itself, or a device with no declared node
// capability) straight through without an extra branch at the call site.
//
// value_template normalises BOTH conventions a node topic can carry, rather than assuming one --
// a hassbridge-kind node relays a real HA entity's own lowercase mqttStateOn/Off
// (applyProxiedBinarySensorPayload), but a hosts-kind (or commandline-kind, 2026-10-06) node topic
// publishes the coordinator's own mqttBoolPayloadTrue/False (discovery.go's
// TBinarySensorDiscoveryPayload{PayloadOn: mqttBoolPayloadTrue, ...}, unconditional for every hosts/
// commandline device). Shared by the hassbridge entity-availability path, the cross-house import
// path (an import can wrap EITHER kind, kind-agnostic since 2026-09-05, so hardcoding one literal
// here silently broke availability for an imported hosts-kind device's own node topic specifically
// -- real bug found live 2026-09-08), AND commandline's own "dependency on" support -- a
// hosts-kind dependency is the only kind resolved today, always "true"/"false".
//
// NodeAvailabilityEntries is one availability entry per non-empty node topic -- a device's own
// liveness, or a "dependency on" device's -- accepting both "on"/"off" and "true"/"false".
func NodeAvailabilityEntries(nodeTopics ...string) []map[string]interface{} {
	var entries []map[string]interface{}
	for _, nodeTopic := range nodeTopics {
		if nodeTopic == "" {
			continue
		}
		entries = append(entries, map[string]interface{}{
			"topic":                 nodeTopic,
			"value_template":        fmt.Sprintf("{{ '%s' if value in ['%s', '%s'] else '%s' }}", StateOn, StateOn, BoolTrue, StateOff),
			"payload_available":     StateOn,
			"payload_not_available": StateOff,
		})
	}
	return entries
}

// ProxiedAvailability carries the rules of the coordinator's former buildAvailabilityFields (moved here 2026-10-27); its
// documentation, with the live bugs behind each rule, follows unchanged.
//
// buildAvailabilityFields sets body's HA MQTT discovery "availability"/"availability_mode"
// fields for an entity proxying a real, independently-fallible upstream HA entity (hassbridge and
// import kinds -- not used by hosts, whose self-reported values are tied to the same liveness as
// their own node signal, so this two-factor distinction doesn't apply there). Refines the earlier
// node-only gating live 2026-08-31: node being reachable does NOT guarantee every entity it
// reports is itself valid right now -- e.g. one Netatmo module's own reading can drop out
// (upstream integration reports it "unavailable") while the module's own connectivity stays fine.
// Availability is therefore the AND (availability_mode "all", the same mechanism Zigbee2MQTT's own
// discovery configs already use to AND bridge-level + device-level availability) of:
//  1. stateTopic itself, always checked -- if the entity's own last-reported payload is literally
//     "unavailable" (what a proxied HA entity's own state naturally becomes when its upstream
//     integration can't read it, independent of device connectivity), it's unavailable.
//  2. every one of nodeTopics (variadic, since 2026-09-16 -- see below), each treated identically:
//     the device's own connectivity signal, or (PROJECT.md's logical-layer work) another device's
//     own connectivity signal this one was declared to "depend on" (Logical.def, resolved
//     generator-side into TImportedDevice.DependsOnAvailabilityTopics, already flattened across
//     the full transitive dependency chain -- the coordinator never walks that graph itself, it
//     just ANDs whatever flat topic list it's handed). Empty strings are skipped, so a caller can
//     pass its own possibly-"" node topic (already gated via availabilityTopicFor -- "" for the
//     node capability itself, or a device with no declared node capability) straight through
//     without an extra branch at the call site.
//
// A further factor -- mixing in the availability of the compute node/host an entity's own
// integration runs on (e.g. protocols-server-2 itself) -- is intentionally not built here; that's
// a separate, later step per the user's own explicit sequencing.
//
// ProxiedAvailability is a proxied entity's availability: its own state isn't "unavailable", AND
// every node topic says it's up.
func ProxiedAvailability(stateTopic string, nodeTopics ...string) []map[string]interface{} {
	return append([]map[string]interface{}{{
		"topic":                 stateTopic,
		"value_template":        fmt.Sprintf("{{ '%s' if value == '%s' else '%s' }}", StateUnavailable, StateUnavailable, StateAvailable),
		"payload_available":     StateAvailable,
		"payload_not_available": StateUnavailable,
	}}, NodeAvailabilityEntries(nodeTopics...)...)
}

// BridgeEntity is what a bridge entity's config is built from: the declaration (the generator's
// homeassistant_bridge.yaml), and what the source entity itself reports (Live*).
type BridgeEntity struct {
	UniqueID      string
	LocalEntity   string
	Capability    string
	DisplaySuffix string
	DeviceClass   string
	Unit          string
	StateClass    string
	Icon          string
	// LiveUnit/LiveDeviceClass/LiveIcon: the source entity's own, learned by the coordinator's
	// existence inquiries -- used where the declaration has none.
	LiveUnit        string
	LiveDeviceClass string
	LiveIcon        string
	Commands        map[string]Command
	DiscoveryExtra  map[string]interface{}
	HasAttributes   bool
	HasPosition     bool
	Instance        string
	StateTopic      string
	AttributesTopic string
	// NodeAvailabilityTopics: the device's own node (when it has one, and this isn't it) and every
	// "dependency on" node topic.
	NodeAvailabilityTopics []string
	// CommandsEnabled: false for a cloud copy, which another house only reads.
	CommandsEnabled bool
	Device          interface{}
}

// BridgeEntityConfig returns e's conceptual config. The caller adds what is its own (the
// coordinator its origin).
func BridgeEntityConfig(e BridgeEntity) map[string]interface{} {
	var name interface{}
	if e.DisplaySuffix != "" {
		name = e.DisplaySuffix
	}
	body := map[string]interface{}{
		"unique_id":         e.UniqueID,
		"default_entity_id": e.LocalEntity,
		"name":              name,
		"device":            e.Device,
	}
	// Not for a domain without a state (a button): its topic still gates the availability below.
	if domain := e.LocalEntity[:strings.Index(e.LocalEntity, ".")+1]; Accepts(strings.TrimSuffix(domain, "."), body, "state_topic") {
		body["state_topic"] = e.StateTopic
	}
	if deviceClass := FirstNonEmpty(e.DeviceClass, e.LiveDeviceClass); deviceClass != "" {
		body["device_class"] = deviceClass
	}
	if unit := FirstNonEmpty(e.Unit, e.LiveUnit); unit != "" {
		body["unit_of_measurement"] = unit
	}
	if e.StateClass != "" {
		body["state_class"] = e.StateClass
	}
	icon := e.Icon
	if icon == "" && e.Capability != "node" {
		icon = e.LiveIcon
	}
	if icon != "" {
		body["icon"] = icon
	}
	if e.CommandsEnabled && len(e.Commands) > 0 {
		for name, command := range e.Commands {
			body[command.Key()] = BridgeCommandTopicFor(e.Instance, e.LocalEntity, name, command)
			if command.DiscoveryKey != "" {
				body[command.DiscoveryKey] = command.Payload
			}
		}
		for key, value := range e.DiscoveryExtra {
			body[key] = value
		}
	}
	if e.HasAttributes && e.AttributesTopic != "" {
		body["json_attributes_topic"] = e.AttributesTopic
	}
	if e.HasPosition && e.StateTopic != "" {
		body["position_topic"] = e.StateTopic
		body["position_template"] = "{{ value_json.position }}"
		body["value_template"] = "{{ value_json.state }}"
	}
	// This carries the rules of the coordinator's former applyProxiedBinarySensorPayload (moved here 2026-10-27); its
	// documentation, with the live bugs behind each rule, follows unchanged.
	//
	// applyProxiedBinarySensorPayload sets body's "payload_on"/"payload_off" to HA's own native
	// binary_sensor state convention ("on"/"off", lowercase) when localEntity's domain is
	// "binary_sensor" -- a no-op for every other domain. Real bug found live 2026-08-31: hassbridge
	// and import discovery configs never set these fields at all, so HA's MQTT binary_sensor platform
	// fell back to its OWN default ("ON"/"OFF", uppercase) -- which never matches a proxied entity's
	// actual lowercase state, leaving it stuck at "unknown" even though the upstream source was
	// reporting a perfectly good "on". Hosts devices don't need this: their own node entity already
	// sets PayloadOn/PayloadOff explicitly (TBinarySensorDiscoveryPayload, "true"/"false" -- a
	// coordinator-chosen convention, not a proxied HA entity's own state).
	// A proxied binary_sensor's state is Home Assistant's own lowercase "on"/"off".
	if strings.HasPrefix(e.LocalEntity, "binary_sensor.") {
		body["payload_on"] = StateOn
		body["payload_off"] = StateOff
	}
	body["availability"] = ProxiedAvailability(e.StateTopic, e.NodeAvailabilityTopics...)
	body["availability_mode"] = "all"
	return body
}
