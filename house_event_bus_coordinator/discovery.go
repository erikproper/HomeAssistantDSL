/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: Discovery
 *
 * Builds Home Assistant MQTT Discovery topics/payloads (§6.1 role 2, discovery refinement)
 * for devices with a "conceptual:" link in devices.yaml -- devices without one (most, today)
 * produce no discovery output, matching the generator's own "positioning is opt-in per
 * device" design. Standard HA discovery topic shape:
 * "homeassistant/<component>/<node_id>/<object_id>/config", retained. node_id is the device
 * id with "." -> "_" (HA restricts node_id/object_id to [a-zA-Z0-9_-]+); object_id is the
 * *local part* of the entity id Spaces.def already resolved (stripping the domain prefix),
 * so HA computes entity_id = <domain>.<object_id> equal to that id exactly, rather than
 * guessing from a name.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 21.08.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// TDiscoveryDevice is the shared HA discovery "device:" block grouping a device's node and
// attribute entities under one HA device (Architecture.md §6.6). Name is the location-aware
// TDeviceConceptual.DisplayName when available ("infrastructural/garage/smarty"), falling
// back to the bare physical HostName otherwise -- never the live-reported hostname, which stays
// informational only (buildDiscoveryConfigs deliberately never reads live["name"]). The
// remaining fields are populated from TDeviceConceptual.ConstantAttributes merged with live-
// reported data (resolveConstantAttribute) via applyConstantAttribute. ViaDevice is resolved
// separately, from a live-reported "via_device" hostname matched against another device's own
// identifier (buildDiscoveryConfigs' caller, mqtt.go) -- "" if unresolved, omitted from the
// payload.
type TDiscoveryDevice struct {
	Identifiers      []string `json:"identifiers"`
	Name             string   `json:"name"`
	Manufacturer     string   `json:"manufacturer,omitempty"`
	Model            string   `json:"model,omitempty"`
	ModelID          string   `json:"model_id,omitempty"`
	HwVersion        string   `json:"hw_version,omitempty"`
	SwVersion        string   `json:"sw_version,omitempty"`
	SerialNumber     string   `json:"serial_number,omitempty"`
	ConfigurationURL string   `json:"configuration_url,omitempty"`
	SuggestedArea    string   `json:"suggested_area,omitempty"`
	ViaDevice        string   `json:"via_device,omitempty"`
}

// applyConstantAttribute sets dev's field for one HA device-map attribute name, matching
// integration_hosts_storage.go's knownConstantDeviceAttributes exactly. Unrecognised names are
// silently ignored here -- the generator already validates and warns on these at generation
// time (mergedConstantAttributes); the coordinator is a thin consumer, not a second authority.
func applyConstantAttribute(dev *TDiscoveryDevice, name, value string) {
	switch name {
	case "manufacturer":
		dev.Manufacturer = value
	case "model":
		dev.Model = value
	case "model_id":
		dev.ModelID = value
	case "hw_version":
		dev.HwVersion = value
	case "sw_version":
		dev.SwVersion = value
	case "serial_number":
		dev.SerialNumber = value
	case "configuration_url":
		dev.ConfigurationURL = value
	case "suggested_area":
		dev.SuggestedArea = value
	}
}

// knownLiveDeviceInfoFields mirrors integration_hosts_storage.go's constant of the same name --
// duplicated rather than imported since the coordinator and generator are separate `package
// main` binaries that can't import each other (see main.go's TConceptualAttribute, which
// already independently mirrors the generator's shape the same way).
var knownLiveDeviceInfoFields = map[string]bool{
	"manufacturer":  true,
	"model":         true,
	"model_id":      true,
	"hw_version":    true,
	"sw_version":    true,
	"serial_number": true,
}

// resolveConstantAttribute resolves one HA device-map field's value with precedence forced-DSL
// > live-reported > default-DSL: a device's own "forced" Physical.def override always wins; a
// non-forced DSL value (integration-type default or a non-forced per-device override) is used
// until live data arrives, then live data takes over. Returns "" if neither source has it.
func resolveConstantAttribute(name string, dsl map[string]TConceptualConstant, live map[string]string) string {
	if c, ok := dsl[name]; ok && c.Forced {
		return c.Value
	}
	if v, ok := live[name]; ok && v != "" {
		return v
	}
	if c, ok := dsl[name]; ok {
		return c.Value
	}
	return ""
}

// TBinarySensorDiscoveryPayload is the HA MQTT Discovery config for a device's node
// (liveness/availability) entity. "object_id" and "has_entity_name" are NOT real MQTT discovery
// keys (verified against home-assistant/core's mqtt/schemas.py and mqtt/const.py, and the
// binary_sensor.mqtt docs) -- HA silently ignores both and falls back to combining device.name +
// entity name for any device-grouped MQTT entity, which is what produced the doubled
// name/entity_id bug this struct now fixes. DefaultEntityID (the *full* entity_id, e.g.
// "binary_sensor.foobar") is the real, documented mechanism for controlling entity_id on first
// creation -- like the fields it replaces, only honoured the first time a given unique_id is
// registered. Name is *string, not string: nil marshals to JSON null, which is what
// "device.name alone, no combination" actually requires ("Can be set to null if only the device
// name is relevant" -- binary_sensor.mqtt docs); an absent/empty-string name does NOT mean the
// same thing to HA. DeviceClass/Icon both come from devices.yaml's conceptual.node_device_class/
// node_icon (generator-resolved from Defaults.def, see TDeviceConceptual) -- Icon is usually
// empty in practice since a device_class like "connectivity" already gives HA a sensible default
// icon on its own, but the field exists for whenever a "defaults: for ...;" rule does set one.
type TBinarySensorDiscoveryPayload struct {
	UniqueID        string           `json:"unique_id"`
	DefaultEntityID string           `json:"default_entity_id"`
	Name            *string          `json:"name"`
	StateTopic      string           `json:"state_topic"`
	PayloadOn       string           `json:"payload_on"`
	PayloadOff      string           `json:"payload_off"`
	DeviceClass     string           `json:"device_class,omitempty"`
	Icon            string           `json:"icon,omitempty"`
	Device          TDiscoveryDevice `json:"device"`
}

// TSensorDiscoveryPayload is the HA MQTT Discovery config for one of a device's variable
// attributes -- same DefaultEntityID/Name rationale as TBinarySensorDiscoveryPayload's doc
// comment. Name is always a short leaf label (e.g. "Load"), never nil: HA combines it with
// device.name automatically for display ("Load" + "infrastructural/garage/smarty" ->
// "infrastructural/garage/smarty Load"), which is the whole point -- unlike the node entity,
// an attribute needs its own distinguishing name. AvailabilityTopic is the device's node topic --
// the attribute goes unavailable exactly when the node does, per the design's own stated intent
// (liveness gates attribute availability). DeviceClass/UnitOfMeasurement/StateClass/Icon come
// from the attribute's TConceptualAttribute -- hardwired per integration type
// (THostsAttributeSpec), not guessed.
type TSensorDiscoveryPayload struct {
	UniqueID            string           `json:"unique_id"`
	DefaultEntityID     string           `json:"default_entity_id"`
	Name                string           `json:"name"`
	StateTopic          string           `json:"state_topic"`
	ValueTemplate       string           `json:"value_template"`
	AvailabilityTopic   string           `json:"availability_topic,omitempty"`
	PayloadAvailable    string           `json:"payload_available,omitempty"`
	PayloadNotAvailable string           `json:"payload_not_available,omitempty"`
	DeviceClass         string           `json:"device_class,omitempty"`
	UnitOfMeasurement   string           `json:"unit_of_measurement,omitempty"`
	StateClass          string           `json:"state_class,omitempty"`
	Icon                string           `json:"icon,omitempty"`
	Device              TDiscoveryDevice `json:"device"`
}

// TDiscoveryConfig bundles one discovery config topic with its (not-yet-serialised) payload.
// Component/Capability (added 2026-09-07) let a caller rebuild an INDEPENDENT cloud-side topic
// for this same config, keyed by exportStableID(ownInstallation, deviceID, Capability) rather than
// Topic's own deviceID-derived naming -- mqtt.go's publishDeviceDiscovery/discoverycleanup.go's
// expectedCloudHostsPayloads both need this so a hosts-kind capability's cloud catalogue entry can
// be matched by cross-house import shorthand (discoveryimport.go's matchImportedCapabilityByStableID)
// the same way a hassbridge capability's already can (discoveryhassbridge.go), without touching
// Topic itself -- which stays exactly as before for the LOCAL discovery config, never renamed.
// Component is the HA MQTT discovery component ("binary_sensor"/"sensor"); Capability is the bare
// key within the device ("node", or an attribute name like "cpu/load").
type TDiscoveryConfig struct {
	Topic      string
	Payload    interface{}
	Component  string
	Capability string
}

// discoveryTopic builds the standard HA MQTT Discovery config topic for one component/entity
// under prefix (${mqtt_discovery_conceptual}, TDevicesFile.conceptualPrefix -- "homeassistant" by
// default, the prefix HA itself subscribes to), keyed by stableID -- identity-based (already the
// same value used as the payload's own unique_id: deviceID+"_node"/"_"+attr for hosts devices,
// gatewayID+"_"+leaf for discovery-bridge relays), never the entity's readable name/position. The
// "coordinator" node_id segment marks every topic this process publishes, so it can be recognised
// (isCoordinatorOwnedTopic, discoverycleanup.go) regardless of which device or attribute it
// belongs to. Deliberately decoupled from the payload's own "default_entity_id"/"name" fields,
// which still force entity_id/display name from the readable path exactly as before -- renaming
// or repositioning a device now only ever changes an existing topic's payload, never the topic
// itself, so it can never orphan a retained message the way the topic previously being
// name-derived did.
func discoveryTopic(prefix, component, stableID string) string {
	return prefix + "/" + component + "/coordinator/" + sanitizeTopicSegment(stableID) + "/config"
}

// sanitizeTopicSegment replaces "." with "_", since HA restricts MQTT discovery topic segments to
// [a-zA-Z0-9_-]+ and deviceID/gatewayID values ("host.xanadu", "discovery.ems_esp") contain dots.
func sanitizeTopicSegment(s string) string {
	return strings.ReplaceAll(s, ".", "_")
}

// availabilityTopicFor is the shared "a device's node/connectivity entity gates every other
// capability's availability" rule -- every discovery-config builder with a device-level node
// entity follows it: hosts devices (buildDiscoveryConfigs' own attribute loop, below), hassbridge
// (hassBridgeAvailabilityTopic, discoveryhassbridge.go), and imports (importedAvailabilityTopic,
// discoveryimport.go). Generalised live 2026-08-31 from what started as hosts-only, then got
// duplicated near-identically into the other two kinds -- one shared rule instead of three copies.
// capability's own config must never reference itself -- "node" (the capability node's own
// discovery config is built from) returns "" to avoid a circular definition -- and a device with
// no node entity/topic at all has nothing to gate on. Callers each resolve nodeStateTopic their
// own device-kind-specific way (already-known device.NodeTopic for hosts; a computed bridge/relay
// state topic for hassbridge/imports) -- this function only owns the gating rule itself, not
// topic resolution, since that genuinely differs per kind.
func availabilityTopicFor(nodeStateTopic, capability string) string {
	if capability == "node" || nodeStateTopic == "" {
		return ""
	}
	return nodeStateTopic
}

// buildAvailabilityFields sets body's HA MQTT discovery "availability"/"availability_mode"
// fields for an entity proxying a real, independently-fallible upstream HA entity (hassbridge and
// import kinds -- not used by hosts, whose self-reported values are tied to the same liveness as
// their own node signal, so this two-factor distinction doesn't apply there). Refines the earlier
// node-only gating live 2026-08-31: node being reachable does NOT guarantee every entity it
// reports is itself valid right now -- e.g. one Netatmo module's own reading can drop out
// (upstream integration reports it "unavailable") while the module's own connectivity stays fine.
// Availability is therefore the AND (availability_mode "all", the same mechanism Zigbee2MQTT's own
// discovery configs already use to AND bridge-level + device-level availability) of up to two
// factors:
//  1. stateTopic itself, always checked -- if the entity's own last-reported payload is literally
//     "unavailable" (what a proxied HA entity's own state naturally becomes when its upstream
//     integration can't read it, independent of device connectivity), it's unavailable.
//  2. nodeTopic, when non-"" (the device's own connectivity signal, already gated via
//     availabilityTopicFor -- "" for the node capability itself, or a device with no declared node
//     capability, meaning only factor 1 applies).
//
// A further factor -- mixing in the availability of the compute node/host an entity's own
// integration runs on (e.g. protocols-server-2 itself) -- is intentionally not built here; that's
// a separate, later step per the user's own explicit sequencing.
func buildAvailabilityFields(body map[string]interface{}, stateTopic, nodeTopic string) {
	entries := []map[string]interface{}{
		{
			"topic":                 stateTopic,
			"value_template":        "{{ 'unavailable' if value == 'unavailable' else 'available' }}",
			"payload_available":     "available",
			"payload_not_available": "unavailable",
		},
	}
	if nodeTopic != "" {
		entries = append(entries, map[string]interface{}{
			"topic":                 nodeTopic,
			"payload_available":     "on",
			"payload_not_available": "off",
		})
	}
	body["availability"] = entries
	body["availability_mode"] = "all"
}

// applyProxiedBinarySensorPayload sets body's "payload_on"/"payload_off" to HA's own native
// binary_sensor state convention ("on"/"off", lowercase) when localEntity's domain is
// "binary_sensor" -- a no-op for every other domain. Real bug found live 2026-08-31: hassbridge
// and import discovery configs never set these fields at all, so HA's MQTT binary_sensor platform
// fell back to its OWN default ("ON"/"OFF", uppercase) -- which never matches a proxied entity's
// actual lowercase state, leaving it stuck at "unknown" even though the upstream source was
// reporting a perfectly good "on". Hosts devices don't need this: their own node entity already
// sets PayloadOn/PayloadOff explicitly (TBinarySensorDiscoveryPayload, "true"/"false" -- a
// coordinator-chosen convention, not a proxied HA entity's own state).
func applyProxiedBinarySensorPayload(body map[string]interface{}, localEntity string) {
	if !strings.HasPrefix(localEntity, "binary_sensor.") {
		return
	}
	body["payload_on"] = "on"
	body["payload_off"] = "off"
}

// buildDiscoveryConfigs returns every HA MQTT Discovery config implied by one devices.yaml
// device -- nil if the device has no "conceptual:" link (nothing to discover yet). live is that
// device's current live-reported snapshot (TLiveDeviceInfoStore.Snapshot; nil/empty is fine --
// every known constant field just falls back to whatever Physical.def declared, or nothing).
// viaDeviceID is the already-resolved via_device target (another device's own identifier), ""
// if the reported broker hostname didn't resolve to one or nothing was reported yet. prefix is
// TDevicesFile.conceptualPrefix() -- ${mqtt_discovery_conceptual}, "homeassistant" by default.
func buildDiscoveryConfigs(deviceID string, device TDevice, live map[string]string, viaDeviceID string, prefix string) []TDiscoveryConfig {
	if device.Conceptual == nil {
		return nil
	}

	name := device.Conceptual.DisplayName
	if name == "" {
		name = device.Host
	}
	devBlock := TDiscoveryDevice{Identifiers: []string{deviceID}, Name: name, ViaDevice: viaDeviceID}

	fieldNames := make(map[string]bool, len(device.Conceptual.ConstantAttributes))
	for attrName := range device.Conceptual.ConstantAttributes {
		fieldNames[attrName] = true
	}
	for attrName := range live {
		if knownLiveDeviceInfoFields[attrName] {
			fieldNames[attrName] = true
		}
	}
	for attrName := range fieldNames {
		if value := resolveConstantAttribute(attrName, device.Conceptual.ConstantAttributes, live); value != "" {
			applyConstantAttribute(&devBlock, attrName, value)
			if attrName == "serial_number" {
				devBlock.Identifiers = append(devBlock.Identifiers, "serial:"+value)
			}
		}
	}
	var configs []TDiscoveryConfig

	if device.Conceptual.NodeEntity != "" {
		uniqueID := deviceID + "_node"
		configs = append(configs, TDiscoveryConfig{
			Topic:      discoveryTopic(prefix, "binary_sensor", uniqueID),
			Component:  "binary_sensor",
			Capability: "node",
			Payload: TBinarySensorDiscoveryPayload{
				UniqueID:        uniqueID,
				DefaultEntityID: device.Conceptual.NodeEntity,
				Name:            nil, // device.name alone -- the node entity IS the device, no distinguishing name needed
				StateTopic:      device.NodeTopic,
				PayloadOn:       "true",
				PayloadOff:      "false",
				DeviceClass:     device.Conceptual.NodeDeviceClass,
				Icon:            device.Conceptual.NodeIcon,
				Device:          devBlock,
			},
		})
	}

	attrNames := make([]string, 0, len(device.Conceptual.AttributeEntities))
	for attr := range device.Conceptual.AttributeEntities {
		attrNames = append(attrNames, attr)
	}
	sort.Strings(attrNames)
	for _, attr := range attrNames {
		link := device.Conceptual.AttributeEntities[attr]
		uniqueID := deviceID + "_" + attr
		// Capability is the DSL-wide group-prefixed name ("cpu/load") when devices.yaml carries
		// one (TConceptualAttribute.Capability's own doc comment explains why it must differ from
		// attr, the bare leaf used everywhere else in this function) -- falls back to the bare
		// leaf for an attribute with no group, or a devices.yaml generated before this field
		// existed.
		capability := link.Capability
		if capability == "" {
			capability = attr
		}
		configs = append(configs, TDiscoveryConfig{
			Topic:      discoveryTopic(prefix, "sensor", uniqueID),
			Component:  "sensor",
			Capability: capability,
			Payload: TSensorDiscoveryPayload{
				UniqueID:            uniqueID,
				DefaultEntityID:     link.Entity,
				Name:                name + "/" + attr,
				StateTopic:          device.Topic,
				ValueTemplate:       "{{ value_json." + attr + " }}",
				AvailabilityTopic:   availabilityTopicFor(device.NodeTopic, attr),
				PayloadAvailable:    "true",
				PayloadNotAvailable: "false",
				DeviceClass:         link.DeviceClass,
				UnitOfMeasurement:   link.Unit,
				StateClass:          link.StateClass,
				Icon:                link.Icon,
				Device:              devBlock,
			},
		})
	}

	return configs
}

// printDiscoveryDryRun prints every device's discovery topic + JSON payload to stdout instead
// of connecting/publishing -- lets the payload shape be checked without a live broker, and
// doubles as an operational debugging tool. Shows the static Physical.def baseline only (no
// live map, no via_device) since there's no broker connection to have received anything from.
func printDiscoveryDryRun(devicesFile TDevicesFile) {
	deviceIDs := make([]string, 0, len(devicesFile.Devices))
	for id := range devicesFile.Devices {
		deviceIDs = append(deviceIDs, id)
	}
	sort.Strings(deviceIDs)

	for _, id := range deviceIDs {
		configs := buildDiscoveryConfigs(id, devicesFile.Devices[id], nil, "", devicesFile.conceptualPrefix())
		if len(configs) == 0 {
			continue
		}
		for _, cfg := range configs {
			data, err := json.MarshalIndent(cfg.Payload, "  ", "  ")
			if err != nil {
				fmt.Printf("%s: error marshalling payload: %v\n", cfg.Topic, err)
				continue
			}
			fmt.Printf("%s\n  %s\n", cfg.Topic, string(data))
		}
	}
}
