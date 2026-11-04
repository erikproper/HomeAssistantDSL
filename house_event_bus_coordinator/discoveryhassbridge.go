/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: DiscoveryHassBridge
 *
 * Bridges entities from a named remote HA instance (protocols-server-2's printer, weather, sun,
 * ...) into the main instance's discovery, per coordinator/homeassistant_bridge.yaml (generator
 * output, homeassistant/integration_hassbridge_generator.go). A reporting automation on the
 * source instance -- generated content, homeassistant/remote_instance_automations.go, written to
 * that instance's own hass/<qualifier>/automation/ tree and copied onto it by hand, since this
 * generator has no deploy path to a remote instance -- publishes each declared capability's
 * already-computed value, retained, to "homeassistant_instances/<name>/bridge/<local_entity>/state"
 * -- keyed by the LOCAL entity_id, not the (possibly compound, e.g. "sensor.x is available" or
 * "sensor.x!attr") source specification, since only the local entity_id is guaranteed to be a
 * clean, space-free MQTT topic segment. This file's only job is to point a normal sensor discovery
 * config's state_topic directly at that already-published topic -- pure discovery-config relay, no
 * value republishing (the automation already resolved the source expression on its own end), same
 * "point at the source's own topic" pattern discoverybridge.go already uses for EMS-ESP-style
 * gateways. Every entity's discovery config also carries a "device:" block, built the same way
 * hosts devices' already is (resolveConstantAttribute/applyConstantAttribute, discovery.go):
 * static Physical.def-declared ConstantAttributes merged with whatever a device's dynamic
 * device-info fields report live, at "homeassistant_instances/<name>/bridge/device/<device-id>/state"
 * (subscribeHassBridgeDeviceInfo) -- one JSON object per device, mirroring hosts' own
 * hosts/<host>/device/state convention, fed into the SAME TLiveDeviceInfoStore hosts devices use
 * (deviceID-keyed, kind-agnostic).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 23.08.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"homeassistant/hamqtt"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gopkg.in/yaml.v3"
)

// THassBridgeCapability is one declared capability's source (remote) and local entity ids, plus
// its optional typing metadata, from coordinator/homeassistant_bridge.yaml.
type THassBridgeCapability struct {
	// SourceEntity is this capability's remote source entity on the device's instance ("" for a
	// derived one) -- Seed (entity_existence.go) inquires about it. (Keyed per instance until
	// PROJECT.md 31, 2026-11-04, for a roaming device.)
	SourceEntity string `yaml:"source_entity"`
	LocalEntity  string `yaml:"local_entity"`
	DeviceClass  string `yaml:"device_class"`
	Unit         string `yaml:"unit"`
	StateClass   string `yaml:"state_class"`
	Icon         string `yaml:"icon"`
	// DisplaySuffix is the word buildHassBridgeEntityDiscoveryBody appends after the device's own
	// display name to build a discovery config's "name" field -- resolved by the generator
	// (homeassistant/Conceptual_DeviceSourceEntities.go's TDeviceAttributeLink.DisplaySuffix, see
	// its own doc comment), never computed here: this coordinator has no knowledge of Spaces.def
	// positioning at all, only of whatever this file already says.
	DisplaySuffix string `yaml:"display_suffix"`
	// StableID is the capability's frozen universal ID (PROJECT.md item 48, Physical.def's
	// "capability ... as <id>;"): its unique_id, and with it its discovery topic, so a rename of
	// the local entity keeps the same HA registry entry. "" falls back to the name-based formula.
	StableID string `yaml:"stable_id"`
	// Commands/DiscoveryExtra (added 2026-09-09) are fully resolved by the generator, off this
	// capability's own domain (homeassistant/hassbridge_commands.go's domainCommands/
	// domainDiscoveryExtras) -- this coordinator never learns a domain name or an HA service
	// string, it only ever merges whatever these two already say into a discovery config
	// (buildHassBridgeEntityDiscoveryBody), keeping the coordinator itself domain-agnostic. A
	// capability with no commands (most of them -- sensor/binary_sensor stay read-only) simply has
	// an empty/absent Commands map.
	Commands map[string]TCapabilityCommand `yaml:"commands"`
	// DiscoveryExtra carries fixed, non-payload discovery fields a commandable domain needs
	// beyond command_topic/payload_* (e.g. vacuum's "supported_features") -- merged into the
	// discovery body verbatim, opaque to this coordinator.
	DiscoveryExtra map[string]interface{} `yaml:"discovery_extra"`
	// HasAttributes (added 2026-09-21): true when this capability declared >=1
	// "attribute <name>: ...;" (homeassistant/integration_hassbridge_parser.go). This coordinator
	// never needs the attribute names/values themselves -- fully resolved generator-side into the
	// sibling "attributes" reporting automation's own Jinja payload
	// (homeassistant/remote_instance_entity_reporting.go), exactly like ValueMap already is -- only
	// this yes/no signal, to decide whether to set json_attributes_topic on this capability's
	// discovery config. The TOPIC itself is computed here (hassBridgeEntityAttributesTopic,
	// mirroring hassBridgeEntityStateTopic exactly), not read from this file, since it's a pure
	// function of instance+local-entity this coordinator already knows how to build. Deliberately
	// NOT routed through DiscoveryExtra above: that field is only merged when the entity is
	// commandable (buildHassBridgeEntityDiscoveryBody's own command-gated block) -- irrelevant to
	// json_attributes_topic, which has nothing to do with commands.
	HasAttributes bool `yaml:"has_attributes,omitempty"`
	// HasPosition (added 2026-09-25, cover's own current-position feature -- matching the
	// discovery-kind Z-Wave awnings cover's existing full position-control feature set): same
	// yes/no-signal shape as HasAttributes above -- true when this capability declared >=1
	// "position: ...;". The generator already folds the resolved position INTO this capability's
	// own state_topic payload as a 'position' JSON key (homeassistant/hassbridge_commands.go's
	// buildHassBridgeStatePayload), so this coordinator only needs to know whether to point
	// position_topic/position_template at that SAME state_topic -- no separate topic at all.
	HasPosition bool `yaml:"has_position,omitempty"`
}

// TCapabilityCommand is one command a capability accepts, from homeassistant_bridge.yaml's own
// "commands: <name>: {payload, discovery_key}" shape -- Payload is the literal string this
// command is recognised by on the capability's single shared command_topic; DiscoveryKey is the
// discovery-config field Payload gets written into (e.g. "payload_on", "payload_start").
// ValueTrigger/DataKey/TopicKey (2026-09-25, added alongside cover's own set_position) mirror
// homeassistant/hassbridge_commands.go's own commandSpec fields exactly -- see that type's doc
// comment. TopicKey defaults to "command_topic" when empty (every command before set_position).
type TCapabilityCommand struct {
	Payload      string `yaml:"payload"`
	DiscoveryKey string `yaml:"discovery_key"`
	ValueTrigger bool   `yaml:"value_trigger"`
	DataKey      string `yaml:"data_key"`
	TopicKey     string `yaml:"topic_key"`
}

// THassBridgeDevice is one bridged device's instance qualifier, declared capabilities, and
// static device metadata. DeviceInfoCapabilities (homeassistant_bridge.yaml's own dynamic
// device-info section) is intentionally not unmarshalled here -- the coordinator only cares
// about the live-*reported* value (arriving via MQTT, subscribeHassBridgeDeviceInfo), never the
// source specification used to produce it, which is purely the reporting automation's concern.
type THassBridgeDevice struct {
	// Instance is the "home_assistant <qualifier>" this device bridges from (a list until
	// PROJECT.md 31, 2026-11-04, for a roaming device declared under several).
	Instance           string                           `yaml:"instance"`
	DisplayName        string                           `yaml:"display_name"`
	Capabilities       map[string]THassBridgeCapability `yaml:"capabilities"`
	ConstantAttributes map[string]TConceptualConstant   `yaml:"constant_attributes"`
	// Export is the "device <id> export with: ...;" flag (PROJECT.md 1.2a,
	// homeassistant/integration_hassbridge_parser.go/_storage.go). An exported device's
	// metadata/state topics are additionally cross-posted onto the cloud broker
	// (installation-qualified, same catalogue-entry-plus-raw-relay shape mqtt_relay.go already
	// uses for a "cloud"+"import" self-imported hosts device) -- see subscribeHassBridge/
	// subscribeHassBridgeDeviceInfo's own doc comments. Command topics don't exist for hassbridge
	// entities yet (PROJECT.md 1.6, still undesigned), so there is nothing to cross-post there
	// yet -- add it alongside state/device-info once commands land, not before.
	Export bool `yaml:"export,omitempty"`
	// DependsOnAvailability (2026-09-16, PROJECT.md's logical-layer work) mirrors
	// TImportedDevice's own identically-named field (discoveryimport.go) exactly -- every extra raw
	// MQTT topic this device's own capabilities' availability must additionally AND in, resolved
	// generator-side from a Logical.def "dependency on <other-id>;" declaration sharing this same
	// device's own id, already flattened across the full transitive dependency chain and
	// cycle-checked there. Passed straight through to buildAvailabilityFields alongside the
	// device's own node topic (hassBridgeAvailabilityTopics). Empty for a device with no matching
	// Logical.def declaration -- unchanged behaviour from before this field existed.
	DependsOnAvailability []string `yaml:"depends_on_availability"`
}

// THassBridgeFile is the top-level shape of a generated coordinator/homeassistant_bridge.yaml
// file.
type THassBridgeFile struct {
	Devices map[string]THassBridgeDevice `yaml:"devices"`
}

// loadHassBridgeFile reads and parses a generator-produced coordinator/homeassistant_bridge.yaml
// file, if one exists -- mirrors loadDiscoveryFile's pattern: a missing file returns a zero
// value, not an error, since most houses have no "home_assistant" bridge devices declared.
func loadHassBridgeFile(path string) (THassBridgeFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return THassBridgeFile{}, nil
		}
		return THassBridgeFile{}, fmt.Errorf("cannot read %s: %w", path, err)
	}

	var bridgeFile THassBridgeFile
	if err := yaml.Unmarshal(data, &bridgeFile); err != nil {
		return THassBridgeFile{}, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	return bridgeFile, nil
}

// hassBridgeMatch is one declared capability's name, typing metadata, and owning device id,
// indexed by its local entity_id (hassBridgeCapabilitiesByLocalEntity). DeviceID is what lets the
// discovery-publish handler find that device's ConstantAttributes and live device-info snapshot
// to build a shared "device:" block.
type hassBridgeMatch struct {
	Capability string
	DeviceID   string
	THassBridgeCapability
}

// hassBridgeCapabilitiesByLocalEntity indexes bridgeFile by local_entity, for looking up which
// capability (and its typing metadata, plus source entity_id kept for logging/context only -- the
// automation already resolved it) a given incoming bridge topic's local entity_id corresponds to.
func hassBridgeCapabilitiesByLocalEntity(bridgeFile THassBridgeFile) map[string]hassBridgeMatch {
	byLocal := map[string]hassBridgeMatch{}
	for deviceID, device := range bridgeFile.Devices {
		for capability, cap := range device.Capabilities {
			byLocal[cap.LocalEntity] = hassBridgeMatch{Capability: capability, DeviceID: deviceID, THassBridgeCapability: cap}
		}
	}
	return byLocal
}

// buildHassBridgeDeviceBlock builds one hassbridge device's shared "device:" discovery block,
// merging its static (Physical.def-declared) ConstantAttributes with whatever live device-info
// has been reported (store.Snapshot(deviceID)) via the same forced-DSL > live > non-forced-DSL
// precedence hosts devices already use (resolveConstantAttribute, discovery.go) -- there's no
// per-integration-type default tier here, unlike hosts, since every hassbridge device is bespoke.
// viaDeviceID (added 2026-09-14, PROJECT.md item 3/plans/via-device-inference.md) is the
// already-resolved via_device target -- another device's own identifier -- exactly mirroring
// discovery.go's buildDiscoveryConfigs' own viaDeviceID parameter for the "hosts" kind: the
// caller resolves it (TEntityExistenceTracker.ResolveViaDevice) before this function runs, since
// it needs a cross-device lookup, not a per-device live-value merge like the fields below. "" if
// unresolved -- omitted from the device block entirely, not an error.
func buildHassBridgeDeviceBlock(deviceID string, device THassBridgeDevice, live map[string]string, viaDeviceID string) TDiscoveryDevice {
	name := device.DisplayName
	if name == "" {
		name = deviceID
	}
	devBlock := TDiscoveryDevice{Identifiers: []string{deviceID}, Name: name, ViaDevice: viaDeviceID}

	fieldNames := make(map[string]bool, len(device.ConstantAttributes))
	for name := range device.ConstantAttributes {
		fieldNames[name] = true
	}
	for name := range live {
		if knownLiveDeviceInfoFields[name] {
			fieldNames[name] = true
		}
	}
	for name := range fieldNames {
		if value := resolveConstantAttribute(name, device.ConstantAttributes, live); value != "" {
			applyConstantAttribute(&devBlock, name, value)
		}
	}
	return devBlock
}

// hassBridgeUniqueID and hassBridgeDiscoveryTopic are shared by subscribeHassBridge's own handler
// and expectedHassBridgeTopics, so the two can't drift apart on what topic a given capability's
// discovery config belongs at. Keyed by the local entity_id -- always a clean, space-free HA
// entity_id, unlike the source specification (which may be compound, e.g. "sensor.x is available").
func hassBridgeUniqueID(localEntity, stableID string) string {
	if stableID != "" {
		return stableID
	}
	return "hassbridge_" + strings.ReplaceAll(localEntity, ".", "_")
}

// hassBridgeEntityStateTopic is the same topic convention hassBridgeReportingAutomationBody
// (homeassistant/remote_instance_automations.go) builds on the generator side -- shared here so
// subscribeHassBridgeDeviceInfo's republish-on-device-info-update path can reconstruct a
// capability's own state_topic without having to have observed a message on it first.
func hassBridgeEntityStateTopic(instance, localEntity string) string {
	return "homeassistant_instances/" + instance + "/bridge/" + localEntity + "/state"
}

// hassBridgeEntityAttributesTopic is hassBridgeEntityStateTopic's sibling for the
// json_attributes_topic feature (2026-09-21) -- the same convention the generator's own sibling
// "attributes" reporting automation (homeassistant/remote_instance_entity_reporting.go) publishes
// to when a capability declares >=1 "attribute <name>: ...;".
func hassBridgeEntityAttributesTopic(instance, localEntity string) string {
	return "homeassistant_instances/" + instance + "/bridge/" + localEntity + "/attributes"
}

func hassBridgeDiscoveryTopic(localEntity, stableID, prefix string) (topic string, ok bool) {
	dotIdx := strings.Index(localEntity, ".")
	if dotIdx < 0 {
		return "", false
	}
	domain := localEntity[:dotIdx]
	return discoveryTopic(prefix, domain, hassBridgeUniqueID(localEntity, stableID)), true
}

// exportStableID is the cloud-side stable identity for one exported capability --
// "<qualifier>_<deviceID-with-dots-as-underscores>_<capability>" -- deliberately INDEPENDENT of
// whatever this house's own local Spaces.def positioning happens to name the local entity as. A
// real robustness gap this fixes, found live 2026-09-07: the cloud unique_id/topic used to be
// hassBridgeUniqueID(localEntity) -- the SAME local-entity-derived id the LOCAL discovery config
// uses -- meaning an importer's own subscription would silently break the moment the exporting
// house repositioned the device locally, entirely unrelated to anything the importer itself
// declared. Decoupling the cloud identity from local naming means it can never drift due to the
// exporter's own repositioning; it depends only on (installation, deviceID, capability), all of
// which are exactly what an importer's own declaration already states.
//
// capability is sanitized ("/" -> "_") here, separately from sanitizeTopicSegment(deviceID) --
// a hosts-kind capability's own name is group-prefixed (e.g. "cpu/load", TDiscoveryConfig's own
// Capability field, discovery.go), and this whole string must stay exactly ONE MQTT topic segment
// (discoveryTopic embeds it as "<prefix>/<component>/coordinator/<stableID>/config") for
// matchImportedCapabilityByStableID's segment-counting to work. Deliberately NOT folded into
// sanitizeTopicSegment itself -- that helper also sanitizes a hosts-kind capability's LOCAL
// discovery topic (buildDiscoveryConfigs' own uniqueID), which must keep its "/" untouched;
// changing it there would rename every such topic already live in production.
func exportStableID(qualifier, deviceID, capability string) string {
	return qualifier + "_" + sanitizeTopicSegment(deviceID) + "_" + strings.ReplaceAll(capability, "/", "_")
}

// hassBridgeExportID is an exported bridge capability's persistent identity, the cloud config's
// "export_id" (PROJECT.md 22, 2026-10-28): its frozen universal ID, which renaming the device or
// capability doesn't change -- else, while it has none yet, the name-based stableID. An importer
// matches on it once its declaration names it ("capability ... from <export-id>").
func hassBridgeExportID(frozenID, stableID string) string {
	if frozenID != "" {
		return frozenID
	}
	return stableID
}

// hostExportID is an exported host capability's persistent identity (PROJECT.md 22): the machine's
// own host name -- already in all its topics, independent of its device id -- and the attribute.
func hostExportID(host, capability string) string {
	return "host_" + host + "_" + strings.ReplaceAll(capability, "/", "_")
}

// hassBridgeCloudDiscoveryTopic is exportStableID's own topic counterpart -- the discovery CONFIG
// topic an exported capability's cloud copy is published under, keyed by domain (extracted from
// localEntity, since the capability's declared Domain always matches it) and the stable id above.
// Distinct from the capability's own cloud STATE topic (where its actual value lives) -- this is
// only the discovery config's own address.
func hassBridgeCloudDiscoveryTopic(localEntity, stableID, prefix string) (topic string, ok bool) {
	dotIdx := strings.Index(localEntity, ".")
	if dotIdx < 0 {
		return "", false
	}
	domain := localEntity[:dotIdx]
	return discoveryTopic(prefix, domain, stableID), true
}

// hassBridgeAvailabilityTopic resolves device's own "node" (connectivity) capability's LOCAL
// bridge state topic, then applies the shared availabilityTopicFor gate (discovery.go) so every
// OTHER capability's discovery config points its availability_topic there -- it goes unavailable
// exactly when the device itself does. Real gap found live 2026-08-31: a hassbridge device's own
// upstream integration (e.g. protocols-server-2's Netatmo integration) may correctly track
// connectivity on its OWN native entity while leaving every OTHER entity it reports simply frozen
// at its last value with no unavailable signal at all -- this coordinator can't fix that upstream
// behaviour, but it CAN make its own published entities honest about it.
//
// hassBridgeOwnSourceEntities returns every source entity_id device itself declares,
// across all its capabilities -- the authoritative "is this entity mine" list ResolveViaDevice
// needs (see its own doc comment for why this must come from Physical.def's own declaration
// rather than the tracker's own DeviceID attribution, which can't represent a source entity two
// devices legitimately share).
func hassBridgeOwnSourceEntities(device THassBridgeDevice) []string {
	entities := make([]string, 0, len(device.Capabilities))
	for _, cap := range device.Capabilities {
		if cap.SourceEntity != "" {
			entities = append(entities, bareEntityFromSource(cap.SourceEntity))
		}
	}
	return entities
}

// hassBridgeAvailabilityTopics (renamed from the singular hassBridgeAvailabilityTopic, 2026-09-16)
// additionally appends device.DependsOnAvailability -- a Logical.def "dependency on <other-id>;"
// declaration sharing this device's own id (e.g. Vienna's "node.fritz_box" depending on
// "node.fritz.box," a "hosts"/ping device for the same physical router) -- mirrors
// importedAvailabilityTopics' own identical reasoning (discoveryimport.go), one file over.
func hassBridgeAvailabilityTopics(device THassBridgeDevice, capability string) []string {
	nodeTopic := ""
	if node, ok := device.Capabilities["node"]; ok && node.LocalEntity != "" && device.Instance != "" {
		nodeTopic = hassBridgeEntityStateTopic(device.Instance, node.LocalEntity)
	}
	var topics []string
	if own := availabilityTopicFor(nodeTopic, capability); own != "" {
		topics = append(topics, own)
	}
	return append(topics, device.DependsOnAvailability...)
}

// buildHassBridgeEntityDiscoveryBody builds one capability's discovery config payload -- shared by
// subscribeHassBridge's own handler and subscribeHassBridgeDeviceInfo's republish-on-update path,
// so the two can't drift on which fields a hassbridge entity's config carries. Typing metadata
// fields (device_class/unit_of_measurement/state_class/icon) are omitted when empty -- HA treats
// an absent one as "generic," same convention TSensorDiscoveryPayload's own omitempty tags use.
// nodeAvailabilityTopics is empty for the node capability itself when its device declares no
// "node" capability and no Logical.def dependency -- see hassBridgeAvailabilityTopics. Availability is always set (via the shared
// buildAvailabilityFields, discovery.go) -- refined live 2026-08-31 to AND the device's own node
// connectivity (when applicable) with the capability's own last-reported value never being the
// literal string "unavailable": node being reachable doesn't guarantee this specific reading is
// currently valid.
//
// "name" bakes in devBlock's own location (devBlock.Name, e.g. "social/roof/solar_panels") ahead
// of the bare capability -- fixed live 2026-09-02: MQTT discovery entities can't use
// "has_entity_name" (not a real key for this platform, see this file's other doc comments), so a
// bare "name": "Production/current/power" left every such entity showing only that, with no
// location context at all in HA's UI, however the entity ended up displayed there. Matches
// buildDiscoveryConfigs' identical fix for "hosts" devices (discovery.go).
// liveUnit/liveDeviceClass/liveIcon (unit/deviceClass added 2026-09-06, icon added 2026-09-09) are
// the remote source entity's own last-reported unit_of_measurement/device_class/icon
// (entity_existence.go's LiveTyping) -- used only when found.Unit/found.DeviceClass/found.Icon
// (Physical.def's own explicit declaration, or a Defaults.def/code-level rule already folded in at
// generate time) left nothing, so a bridged capability with no explicit typing anywhere still
// picks up whatever its remote entity's own upstream integration already resolved (e.g. a
// Fritz!Box's own gb_received sensor), instead of showing up with no unit/icon at all. Real gap
// found live 2026-09-06 (unit/device_class); icon itself was named in that same fix's doc comments
// but never actually wired through until found live again 2026-09-09 (Vienna's washing_machine
// bridge).
//
// commandsEnabled (added 2026-09-09, instance/commandsEnabled replaced a single precomputed
// commandTopic string 2026-09-25 to let each command resolve its own topic -- see
// hassBridgeEntityCommandTopicFor) is false to omit command support entirely -- used for the
// cloud-exported body until PROJECT.md's cross-house command relay (Phase 2, deferred) exists, so
// an importing house never gets a command topic it can't actually reach yet. For the local body,
// true is passed whenever found.Commands is non-empty; when both are set, this function fills in
// each command's own topicKey() field plus every found.Commands[*].DiscoveryKey: Payload pair and
// every found.DiscoveryExtra field verbatim -- all fully resolved by the generator
// (hassbridge_commands.go), never domain-specific logic here.
func buildHassBridgeEntityDiscoveryBody(stableID, localEntity string, found hassBridgeMatch, stateTopic, attributesTopic string, devBlock TDiscoveryDevice, nodeAvailabilityTopics []string, liveUnit, liveDeviceClass, liveIcon, instance string, commandsEnabled bool, installation string) map[string]interface{} {
	commands := make(map[string]hamqtt.Command, len(found.Commands))
	for name, command := range found.Commands {
		commands[name] = hamqtt.Command{Payload: command.Payload, DiscoveryKey: command.DiscoveryKey, ValueTrigger: command.ValueTrigger, DataKey: command.DataKey, TopicKey: command.TopicKey}
	}
	body := hamqtt.BridgeEntityConfig(hamqtt.BridgeEntity{
		UniqueID: stableID, LocalEntity: localEntity, Capability: found.Capability, DisplaySuffix: found.DisplaySuffix,
		DeviceClass: found.DeviceClass, Unit: found.Unit, StateClass: found.StateClass, Icon: found.Icon,
		LiveUnit: liveUnit, LiveDeviceClass: liveDeviceClass, LiveIcon: liveIcon,
		Commands: commands, DiscoveryExtra: found.DiscoveryExtra, HasAttributes: found.HasAttributes, HasPosition: found.HasPosition,
		Instance: instance, StateTopic: stateTopic, AttributesTopic: attributesTopic,
		NodeAvailabilityTopics: nodeAvailabilityTopics, CommandsEnabled: commandsEnabled, Device: devBlock,
	})
	body["availability"] = availabilityEntriesFrom(body["availability"])
	body["origin"] = coordinatorOriginMap(installation)
	return body
}

// availabilityEntriesFrom turns hamqtt's availability list into this package's own type, as the
// discovery configs carried it before the builders moved there.
func availabilityEntriesFrom(list interface{}) []TAvailabilityEntry {
	var availability []TAvailabilityEntry
	entries, _ := list.([]map[string]interface{})
	for _, entry := range entries {
		availability = append(availability, TAvailabilityEntry{
			Topic: entry["topic"].(string), ValueTemplate: entry["value_template"].(string),
			PayloadAvailable: entry["payload_available"].(string), PayloadNotAvailable: entry["payload_not_available"].(string),
		})
	}
	return availability
}

// hassBridgeAcceptsInstance reports whether a bridge message whose instance segment is instance
// may speak for device: one of its own declared instances. Anything else is a retained leftover of
// an instance that no longer exists. Real incident, 2026-10-21 (Junglinster): 184 retained bridge
// states under "protocols-server-2" survived ha2mqtt's move/rename to protocols-server-1 on
// 2026-10-15; every coordinator start then saw each entity under BOTH the dead and the live
// instance, flipping its discovery state_topic between the two (~300 retire+republish cycles per
// restart) and leaving whichever entity happened to be processed last pointed at the frozen dead
// topic.
func hassBridgeAcceptsInstance(device THassBridgeDevice, instance string) bool {
	return device.Instance == instance
}

// containsString reports whether want appears anywhere in list.
func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// expectedHassBridgeCloudTopics is expectedHassBridgeTopics' cloud-broker counterpart: every
// "export"-flagged device's capabilities get an installation-qualified discovery topic on the
// cloud broker too (subscribeHassBridge/subscribeHassBridgeDeviceInfo's own Export handling).
// Bug found and fixed live 2026-08-30: without this, main.go's watchForOrphanedDiscoveryTopics
// (subscribed on the cloud broker with its own expected-topics set) saw these topics as orphaned
// the moment they were first published -- not in *that* set at all -- and retired them within
// seconds, even though they're entirely legitimate. Mirrors expectedCloudHostsPayloads' role for
// hosts devices, except -- like expectedHassBridgeTopics above -- topic-only, no payload content:
// a hassbridge entity's discovery body depends on live device-info (store.Snapshot), not knowable
// at coordinator startup before any report has arrived, so content-staleness checking is skipped
// for these topics exactly as it already is for the local-broker copy.
func expectedHassBridgeCloudTopics(bridgeFile THassBridgeFile, ownInstallation, prefix string) map[string]bool {
	expected := map[string]bool{}
	for deviceID, device := range bridgeFile.Devices {
		if !device.Export {
			continue
		}
		qualifier := ownInstallation
		if qualifier == "" {
			continue
		}
		for capability, cap := range device.Capabilities {
			stableID := exportStableID(qualifier, deviceID, capability)
			if topic, ok := hassBridgeCloudDiscoveryTopic(cap.LocalEntity, stableID, prefix); ok {
				expected[qualifier+"/"+topic] = true
			}
		}
	}
	return expected
}

// crossPostHassBridgeToCloud forward-publishes bareTopic's already-computed payload (retained)
// onto cloudClient at qualifier+"/"+bareTopic -- the coordinator itself is the publisher here
// (unlike a "cloud"-routed hosts device, whose own report script publishes on the cloud broker
// directly; a hassbridge entity's reporting automation only ever publishes locally), so this is a
// genuine, functional relay rather than the hosts mechanism's non-functional discovery catalogue
// entry -- a future "integration import" consumer (PROJECT.md 1.2a, not built yet) can subscribe
// to the qualified topic directly and get real values, not just a catalogue listing. qualifier is this
// installation's own name. No-op if cloudClient is nil (no cloud broker configured).
func crossPostHassBridgeToCloud(cloudClient mqtt.Client, qualifier, bareTopic string, payload []byte) {
	if cloudClient == nil {
		return
	}
	qualified := qualifier + "/" + bareTopic
	token := cloudClient.Publish(qualified, 0, true, payload)
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		if err := token.Error(); err != nil {
			fmt.Printf("[hass-bridge:export] relaying %s -> %s: %v\n", bareTopic, qualified, err)
		}
	}
}

// subscribeHassBridge subscribes to "homeassistant_instances/+/bridge/+/state" and, for every
// message whose 4th topic segment (the local entity_id) matches a declared capability, publishes
// (through publisher, content-aware/self-healing like everything else) a sensor discovery config
// for that capability's local_entity -- including a "device:" block built from that capability's
// owning device's ConstantAttributes + store's current live snapshot -- with state_topic pointing
// directly at the message's own topic -- no value republishing, the automation's own retained
// publish already carries the current, already-computed value. No-op (returns nil immediately)
// when bridgeFile has no devices declared. Six-segment device-info topics
// ("homeassistant_instances/+/bridge/device/+/state") never match this filter's five segments,
// so there's no risk of this handler misreading one as an entity-state message.
//
// An "export"-flagged device (THassBridgeDevice.Export, PROJECT.md 1.2a) additionally gets a
// SEPARATE discovery config published to the cloud broker (installation-qualified topic, same
// node_id-collision-avoidance reasoning as mqtt_relay.go's hosts-side cloud discovery) -- separate,
// not the same payload republished, because its state_topic must point at the cross-posted cloud
// topic (crossPostHassBridgeToCloud), not the bare local one main's copy uses. Unlike the hosts
// mechanism's cloud discovery (a non-functional catalogue entry, mqtt_relay.go's own doc comment),
// this one is genuinely functional: a real HA-MQTT-discovery-shaped payload whose device/entity/
// typing fields and state_topic are all directly usable by a future importing coordinator (no
// import mechanism exists yet, PROJECT.md 1.2a) the same way it already consumes a kind-2
// gateway's own native discovery payload (discoverybridge.go) -- exported kind-3 entities are
// meant to look, wire-format-wise, indistinguishable from a kind-2 gateway's self-description.
// Also carries a top-level "installation" field (this house's own ${installation}) alongside the
// usual "device" block, since device.Identifiers itself is only the bare local device id (e.g.
// "hass.office_garden") and could collide with another exporting house's identically-named device
// on the same shared cloud broker -- the importer needs "installation" to disambiguate/qualify.
// Both no-ops when cloudClient is nil.
func subscribeHassBridge(client, cloudClient mqtt.Client, ownInstallation string, bridgeFile THassBridgeFile, store *TLiveDeviceInfoStore, publisher *TDiscoveryPublisher, conceptualPrefix string, existenceTracker *TEntityExistenceTracker) error {
	if len(bridgeFile.Devices) == 0 {
		return nil
	}

	byLocal := hassBridgeCapabilitiesByLocalEntity(bridgeFile)

	handler := func(_ mqtt.Client, msg mqtt.Message) {
		parts := strings.Split(msg.Topic(), "/")
		if len(parts) != 5 || parts[2] != "bridge" || parts[4] != "state" {
			return
		}
		localEntity := parts[3]
		found, known := byLocal[localEntity]
		if !known {
			return
		}

		if !strings.Contains(localEntity, ".") {
			return
		}

		device := bridgeFile.Devices[found.DeviceID]
		// An empty payload is a retained message being cleared, not a report -- republishing
		// discovery for it would point state_topic at a topic that no longer holds anything.
		if len(msg.Payload()) == 0 || !hassBridgeAcceptsInstance(device, parts[1]) {
			return
		}
		viaDeviceID := ""
		liveUnit, liveDeviceClass, liveIcon := "", "", ""
		if existenceTracker != nil {
			viaDeviceID, _ = existenceTracker.ResolveViaDevice(parts[1], found.DeviceID, hassBridgeOwnSourceEntities(device))
			if found.SourceEntity != "" {
				liveUnit, liveDeviceClass, liveIcon = existenceTracker.LiveTyping(parts[1], found.SourceEntity)
			}
		}
		devBlock := buildHassBridgeDeviceBlock(found.DeviceID, device, store.Snapshot(found.DeviceID), viaDeviceID)
		if discoveryDeviceInfo.RecordBridgeDevice(found.DeviceID, store.Snapshot(found.DeviceID), viaDeviceID) {
			discoveryDeviceInfo.SchedulePublish(client, cloudClient, ownInstallation)
		}
		availabilityTopics := hassBridgeAvailabilityTopics(device, found.Capability)
		// Home Assistant gets the entity from the generator's YAML (PROJECT.md 19h.3); what the
		// source entity reports of its typing is collected for it.
		hassBridgeTyping.Record(localEntity, liveUnit, liveDeviceClass, liveIcon, client, cloudClient, ownInstallation)

		if cloudClient != nil && device.Export {
			qualifier := ownInstallation
			cloudBareTopic := msg.Topic()
			crossPostHassBridgeToCloud(cloudClient, qualifier, cloudBareTopic, msg.Payload())
			var cloudAvailabilityTopics []string
			for _, topic := range availabilityTopics {
				cloudAvailabilityTopics = append(cloudAvailabilityTopics, qualifier+"/"+topic)
			}
			stableID := exportStableID(qualifier, found.DeviceID, found.Capability)
			cloudAttributesBareTopic := hassBridgeEntityAttributesTopic(parts[1], localEntity)
			// devBlock.withoutSuggestedArea(): which area a device sits in is always this house's
			// own local Spaces.def positioning, never a fact for an importing house to inherit --
			// see TDiscoveryDevice.withoutSuggestedArea's own doc comment.
			cloudBody := buildHassBridgeEntityDiscoveryBody(stableID, localEntity, found, qualifier+"/"+cloudBareTopic, qualifier+"/"+cloudAttributesBareTopic, devBlock.withoutSuggestedArea(), cloudAvailabilityTopics, liveUnit, liveDeviceClass, liveIcon, "", false, ownInstallation)
			cloudBody["installation"] = qualifier
			cloudBody["export_id"] = hassBridgeExportID(found.StableID, stableID)
			cloudData, err := json.Marshal(cloudBody)
			if err != nil {
				fmt.Printf("[hass-bridge:export] marshalling cloud discovery payload for %s: %v\n", localEntity, err)
				return
			}
			cloudTopic, ok := hassBridgeCloudDiscoveryTopic(localEntity, stableID, conceptualPrefix)
			if !ok {
				return
			}
			if err := publisher.Publish(cloudClient, "cloud_coordinator", qualifier+"/"+cloudTopic, cloudData); err != nil {
				fmt.Printf("[hass-bridge:export] publishing %s: %v\n", cloudTopic, err)
			}
		}
	}

	const filter = "homeassistant_instances/+/bridge/+/state"
	token := subscribe(client, filter, 0, handler)
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		if err := token.Error(); err != nil {
			return fmt.Errorf("subscribing to %s: %w", filter, err)
		}
		return fmt.Errorf("subscribing to %s: timed out", filter)
	}
	return nil
}

// subscribeHassBridgeDeviceInfo subscribes to
// "homeassistant_instances/+/bridge/device/+/state" (the 5th segment being the device id) and,
// for every message, updates store with that device's live-reported fields, then republishes
// discovery for every one of that device's already-known capabilities so their "device:" blocks
// pick up the fresh data -- mirrors subscribeDeviceInfo's own update-then-republish pattern
// (mqtt.go) for hosts devices exactly.
//
// An "export"-flagged device (THassBridgeDevice.Export) also gets its device-info payload
// cross-posted to the cloud broker (crossPostHassBridgeToCloud) and each capability's refreshed
// discovery config republished there too, as a separate cloud-qualified-state_topic payload plus
// an "installation" field -- same reasoning as subscribeHassBridge's own Export handling. No-op
// when cloudClient is nil.
// republishHassBridgeDeviceCapabilities rebuilds and republishes discovery configs for every one
// of device's known capabilities, picking up store's CURRENT snapshot for deviceID -- the
// "change in store => change in config" half of the device-info propagation contract (liveinfo.go's
// own doc comment), shared by subscribeHassBridgeDeviceInfo's own device-info topic handler and,
// via store's onChange callback (main.go), any other caller of store.Update for a hassbridge
// device -- entity_existence.go's subscribeExistenceReply, today, for manufacturer/model learned
// from an inquiry reply rather than a dedicated device-info report.
func republishHassBridgeDeviceCapabilities(client, cloudClient mqtt.Client, ownInstallation, deviceID string, device THassBridgeDevice, store *TLiveDeviceInfoStore, publisher *TDiscoveryPublisher, conceptualPrefix string, existenceTracker *TEntityExistenceTracker) {
	exportToCloud := cloudClient != nil && device.Export
	qualifier := ownInstallation

	viaDeviceID := ""
	if existenceTracker != nil {
		viaDeviceID, _ = existenceTracker.ResolveViaDevice(device.Instance, deviceID, hassBridgeOwnSourceEntities(device))
	}
	devBlock := buildHassBridgeDeviceBlock(deviceID, device, store.Snapshot(deviceID), viaDeviceID)
	// What the bridge reports about the device goes into discovery_device_info/state too, for
	// generate to capture into Physical.def (PROJECT.md 19h.3) -- via_device as resolved here.
	if discoveryDeviceInfo.RecordBridgeDevice(deviceID, store.Snapshot(deviceID), viaDeviceID) {
		discoveryDeviceInfo.SchedulePublish(client, cloudClient, ownInstallation)
	}
	for capability, cap := range device.Capabilities {
		if !strings.Contains(cap.LocalEntity, ".") {
			continue
		}
		found := hassBridgeMatch{Capability: capability, DeviceID: deviceID, THassBridgeCapability: cap}
		availabilityTopics := hassBridgeAvailabilityTopics(device, capability)
		liveUnit, liveDeviceClass, liveIcon := "", "", ""
		if existenceTracker != nil {
			if cap.SourceEntity != "" {
				liveUnit, liveDeviceClass, liveIcon = existenceTracker.LiveTyping(device.Instance, cap.SourceEntity)
			}
		}
		hassBridgeTyping.Record(cap.LocalEntity, liveUnit, liveDeviceClass, liveIcon, client, cloudClient, ownInstallation)
		if exportToCloud {
			cloudStateTopic := qualifier + "/" + hassBridgeEntityStateTopic(device.Instance, cap.LocalEntity)
			cloudAttributesTopic := qualifier + "/" + hassBridgeEntityAttributesTopic(device.Instance, cap.LocalEntity)
			var cloudAvailabilityTopics []string
			for _, topic := range availabilityTopics {
				cloudAvailabilityTopics = append(cloudAvailabilityTopics, qualifier+"/"+topic)
			}
			stableID := exportStableID(qualifier, deviceID, capability)
			// devBlock.withoutSuggestedArea(): see the identical comment in subscribeHassBridge's
			// own handler above.
			cloudBody := buildHassBridgeEntityDiscoveryBody(stableID, cap.LocalEntity, found, cloudStateTopic, cloudAttributesTopic, devBlock.withoutSuggestedArea(), cloudAvailabilityTopics, liveUnit, liveDeviceClass, liveIcon, "", false, ownInstallation)
			cloudBody["installation"] = qualifier
			cloudBody["export_id"] = hassBridgeExportID(found.StableID, stableID)
			cloudData, err := json.Marshal(cloudBody)
			if err != nil {
				fmt.Printf("[hass-bridge:export] marshalling cloud discovery payload for %s: %v\n", cap.LocalEntity, err)
				continue
			}
			cloudTopic, ok := hassBridgeCloudDiscoveryTopic(cap.LocalEntity, stableID, conceptualPrefix)
			if !ok {
				continue
			}
			if err := publisher.Publish(cloudClient, "cloud_coordinator", qualifier+"/"+cloudTopic, cloudData); err != nil {
				fmt.Printf("[hass-bridge:export] publishing %s: %v\n", cloudTopic, err)
			}
		}
	}
}

func subscribeHassBridgeDeviceInfo(client, cloudClient mqtt.Client, ownInstallation string, bridgeFile THassBridgeFile, store *TLiveDeviceInfoStore, publisher *TDiscoveryPublisher, conceptualPrefix string, existenceTracker *TEntityExistenceTracker) error {
	if len(bridgeFile.Devices) == 0 {
		return nil
	}

	handler := func(_ mqtt.Client, msg mqtt.Message) {
		parts := strings.Split(msg.Topic(), "/")
		if len(parts) != 6 || parts[2] != "bridge" || parts[3] != "device" || parts[5] != "state" {
			return
		}
		deviceID := parts[4]
		device, known := bridgeFile.Devices[deviceID]
		if !known || len(msg.Payload()) == 0 || !hassBridgeAcceptsInstance(device, parts[1]) {
			return
		}

		var fields map[string]string
		if err := json.Unmarshal(msg.Payload(), &fields); err != nil {
			fmt.Printf("[hass-bridge] %s: cannot parse device-info payload: %v\n", msg.Topic(), err)
			return
		}
		// store.Update's own onChange callback (liveinfo.go, wired in main.go) already republishes
		// this device's capabilities on any actual change -- republishHassBridgeDeviceCapabilities
		// below runs unconditionally anyway (matching this handler's own pre-2026-09-08 behaviour,
		// before the store became reactive), since a device-info report can also mean "still the
		// same values, still alive," which is worth a republish in its own right here (unlike a
		// bare inquiry reply, this handler exists specifically because something was reported).
		store.Update(deviceID, fields)

		exportToCloud := cloudClient != nil && device.Export
		qualifier := ownInstallation

		republishHassBridgeDeviceCapabilities(client, cloudClient, ownInstallation, deviceID, device, store, publisher, conceptualPrefix, existenceTracker)

		if exportToCloud {
			crossPostHassBridgeToCloud(cloudClient, qualifier, msg.Topic(), msg.Payload())
		}
	}

	const filter = "homeassistant_instances/+/bridge/device/+/state"
	token := subscribe(client, filter, 0, handler)
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		if err := token.Error(); err != nil {
			return fmt.Errorf("subscribing to %s: %w", filter, err)
		}
		return fmt.Errorf("subscribing to %s: timed out", filter)
	}
	return nil
}
