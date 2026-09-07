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

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gopkg.in/yaml.v3"
)

// THassBridgeCapability is one declared capability's source (remote) and local entity ids, plus
// its optional typing metadata, from coordinator/homeassistant_bridge.yaml.
type THassBridgeCapability struct {
	// SourceEntities holds this capability's remote source entity, keyed by declaring instance --
	// genuinely per-instance, not a single shared value: the same physical roaming device can
	// register under a DIFFERENT local entity_id on each HA instance it connects to (real
	// incident, found live 2026-09-05 -- Vienna's own registry initially lacked
	// hass.eriks_iphone's battery_level entity under the identical name Junglinster's instances
	// used). Kept for logging/context only here -- the source instance's own reporting automation
	// already resolved it -- but Seed (entity_existence.go) DOES functionally depend on this being
	// correct per-instance, to inquire about the right entity_id on each instance.
	SourceEntities map[string]string `yaml:"source_entities"`
	LocalEntity    string            `yaml:"local_entity"`
	DeviceClass    string            `yaml:"device_class"`
	Unit           string            `yaml:"unit"`
	StateClass     string            `yaml:"state_class"`
	Icon           string            `yaml:"icon"`
}

// THassBridgeDevice is one bridged device's instance qualifier, declared capabilities, and
// static device metadata. DeviceInfoCapabilities (homeassistant_bridge.yaml's own dynamic
// device-info section) is intentionally not unmarshalled here -- the coordinator only cares
// about the live-*reported* value (arriving via MQTT, subscribeHassBridgeDeviceInfo), never the
// source specification used to produce it, which is purely the reporting automation's concern.
type THassBridgeDevice struct {
	// Instances is every "home_assistant <qualifier>" this device bridges from -- almost always
	// exactly one, but a "roaming" device (ExportAs "roaming") may be declared identically across
	// more than one qualifier block within the same house (e.g. an HA companion-app phone reachable
	// via either of two local HA instances), merged into one record generator-side
	// (homeassistant/integration_hassbridge_parser.go's collectHassBridgeDevicesByID) rather than
	// colliding.
	Instances          []string                         `yaml:"instances"`
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
	// ExportAs overrides the installation name Export's cross-post is qualified under -- "" means
	// this coordinator's own real installation name (today's only behaviour); a non-empty value
	// (currently only ever "roaming", the "roaming" keyword replacing "export" on the device
	// header) means several real installations can each export their own local copy of a device
	// under one shared, stable cloud-side identity instead of three separate real-name-qualified
	// ones. See exportQualifier, below, and mqtt_relay.go's doc comment for the "hosts"-kind
	// precedent this generalizes. Added 2026-09-02.
	ExportAs string `yaml:"export_as,omitempty"`
	// SelfImportFrom is "" unless the "import" keyword was also declared, in which case it's this
	// installation's own resolved name -- means this coordinator should additionally relay
	// whatever's published under ExportAs (or, absent that, its own real name) back onto its own
	// already-existing local bridge topic, feeding the SAME discovery entity Spaces.def
	// positioning already creates for this device rather than a second one. See
	// subscribeHassBridgeSelfImport, below -- deliberately NOT routed through
	// TImportedDevice/discoveryimport.go, which would create a structurally distinct,
	// duplicate discovery entity instead of feeding the existing one. Added 2026-09-02.
	SelfImportFrom string `yaml:"self_import_from,omitempty"`
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
func buildHassBridgeDeviceBlock(deviceID string, device THassBridgeDevice, live map[string]string) TDiscoveryDevice {
	name := device.DisplayName
	if name == "" {
		name = deviceID
	}
	devBlock := TDiscoveryDevice{Identifiers: []string{deviceID}, Name: name}

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
func hassBridgeUniqueID(localEntity string) string {
	return "hassbridge_" + strings.ReplaceAll(localEntity, ".", "_")
}

// hassBridgeEntityStateTopic is the same topic convention hassBridgeReportingAutomationBody
// (homeassistant/remote_instance_automations.go) builds on the generator side -- shared here so
// subscribeHassBridgeDeviceInfo's republish-on-device-info-update path can reconstruct a
// capability's own state_topic without having to have observed a message on it first.
func hassBridgeEntityStateTopic(instance, localEntity string) string {
	return "homeassistant_instances/" + instance + "/bridge/" + localEntity + "/state"
}

func hassBridgeDiscoveryTopic(localEntity, prefix string) (topic string, ok bool) {
	dotIdx := strings.Index(localEntity, ".")
	if dotIdx < 0 {
		return "", false
	}
	domain := localEntity[:dotIdx]
	return discoveryTopic(prefix, domain, hassBridgeUniqueID(localEntity)), true
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

// hassBridgeCloudDiscoveryTopic is exportStableID's own topic counterpart -- the discovery CONFIG
// topic an exported capability's cloud copy is published under, keyed by domain (extracted from
// localEntity, since the capability's declared Domain always matches it) and the stable id above.
// Distinct from the capability's own cloud STATE topic (where its actual value lives, still
// canonicalizeRoamingBridgeTopic-derived, unaffected by this) -- this is only the discovery
// config's own address.
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
// A multi-instance ("roaming") device has no single node topic to gate on -- hassBridgeEntityStateTopic
// bakes the reporting instance's own name into the topic string, and different instances' own
// copies of "node" live at genuinely different topics, only one of which is actually live at a
// given moment. Rather than guess which, node-gating is skipped entirely for a device declared on
// more than one instance -- its other capabilities' availability then depends only on their own
// last-reported value never being the literal string "unavailable" (buildAvailabilityFields' other
// half), still functional, just without the extra node cross-check.
// hassBridgeDefaultInstance picks a deterministic single instance out of device.Instances for
// contexts that need to reconstruct a state_topic without having observed a live message on it
// (subscribeHassBridgeDeviceInfo's own republish-on-device-info-update path) -- "" if none
// declared. For a "roaming" device with several, this is only ever a reasonable starting point,
// not necessarily the currently-live one: subscribeHassBridge's own wildcard state handler
// self-heals it the moment any instance's real traffic next arrives, republishing discovery with
// state_topic pointing at whichever topic actually just fired.
func hassBridgeDefaultInstance(device THassBridgeDevice) string {
	if len(device.Instances) == 0 {
		return ""
	}
	return device.Instances[0]
}

func hassBridgeAvailabilityTopic(device THassBridgeDevice, capability string) string {
	nodeTopic := ""
	if len(device.Instances) == 1 {
		if node, ok := device.Capabilities["node"]; ok && node.LocalEntity != "" {
			nodeTopic = hassBridgeEntityStateTopic(device.Instances[0], node.LocalEntity)
		}
	}
	return availabilityTopicFor(nodeTopic, capability)
}

// buildHassBridgeEntityDiscoveryBody builds one capability's discovery config payload -- shared by
// subscribeHassBridge's own handler and subscribeHassBridgeDeviceInfo's republish-on-update path,
// so the two can't drift on which fields a hassbridge entity's config carries. Typing metadata
// fields (device_class/unit_of_measurement/state_class/icon) are omitted when empty -- HA treats
// an absent one as "generic," same convention TSensorDiscoveryPayload's own omitempty tags use.
// nodeAvailabilityTopic is "" for the node capability itself, or when its device declares no
// "node" capability -- see hassBridgeAvailabilityTopic. Availability is always set (via the shared
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
// liveUnit/liveDeviceClass (added 2026-09-06) are the remote source entity's own last-reported
// unit_of_measurement/device_class (entity_existence.go's LiveTyping) -- used only when
// found.Unit/found.DeviceClass (Physical.def's own explicit declaration, or a Defaults.def/
// code-level rule already folded in at generate time) left nothing, so a bridged capability with
// no explicit typing anywhere still picks up whatever its remote entity's own upstream
// integration already resolved (e.g. a Fritz!Box's own gb_received sensor), instead of showing up
// with no unit/icon at all. Real gap found live 2026-09-06.
func buildHassBridgeEntityDiscoveryBody(stableID, localEntity string, found hassBridgeMatch, stateTopic string, devBlock TDiscoveryDevice, nodeAvailabilityTopic, liveUnit, liveDeviceClass string) map[string]interface{} {
	body := map[string]interface{}{
		"unique_id":         stableID,
		"default_entity_id": localEntity,
		"name":              devBlock.Name + "/" + found.Capability,
		"state_topic":       stateTopic,
		"device":            devBlock,
	}
	deviceClass := found.DeviceClass
	if deviceClass == "" {
		deviceClass = liveDeviceClass
	}
	unit := found.Unit
	if unit == "" {
		unit = liveUnit
	}
	if deviceClass != "" {
		body["device_class"] = deviceClass
	}
	if unit != "" {
		body["unit_of_measurement"] = unit
	}
	if found.StateClass != "" {
		body["state_class"] = found.StateClass
	}
	if found.Icon != "" {
		body["icon"] = found.Icon
	}
	applyProxiedBinarySensorPayload(body, localEntity)
	buildAvailabilityFields(body, stateTopic, nodeAvailabilityTopic)
	return body
}

// expectedHassBridgeTopics computes every discovery topic bridgeFile's declared capabilities
// imply -- known statically, no live gateway/automation report needed, the same "topic existence
// doesn't depend on live data" property expectedDiscoveryBridgeTopics already has.
func expectedHassBridgeTopics(bridgeFile THassBridgeFile, prefix string) map[string]bool {
	expected := map[string]bool{}
	for _, device := range bridgeFile.Devices {
		for _, cap := range device.Capabilities {
			if topic, ok := hassBridgeDiscoveryTopic(cap.LocalEntity, prefix); ok {
				expected[topic] = true
			}
		}
	}
	return expected
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

// canonicalRoamingInstance is the fixed instance segment substituted into a roaming
// (device.ExportAs != "") device's CLOUD-published bridge topics, in place of whichever real local
// instance (device.Instances) actually produced this particular message. Arbitrary but stable --
// unrelated to whether any real instance happens to be named "main" -- chosen purely so an importer
// never needs to watch more than one topic per capability, or a wildcard, to see a roaming device's
// current value.
const canonicalRoamingInstance = "main"

// canonicalizeRoamingBridgeTopic rewrites bareTopic's "homeassistant_instances/<instance>/..."
// segment to canonicalRoamingInstance when device is a roaming device (ExportAs != "") -- applied
// only when building the CLOUD-bound copy of a topic, never the local one (local traffic still
// needs its real instance segment, e.g. for hassBridgeAvailabilityTopic's per-instance node lookup
// and for subscribeHassBridgeSelfImport's own relay). No-op (returns bareTopic unchanged) for a
// non-roaming device. Fixes the surprise found live 2026-09-05: before this, an iPhone reachable via
// two HA instances cross-posted to TWO different cloud topics
// ("roaming/homeassistant_instances/main/bridge/.../state" and ".../protocols-server-2/bridge/...")
// depending on which instance happened to report most recently, forcing an importer to watch both
// (or wildcard) instead of one stable topic.
func canonicalizeRoamingBridgeTopic(bareTopic string, device THassBridgeDevice) string {
	if device.ExportAs == "" {
		return bareTopic
	}
	parts := strings.SplitN(bareTopic, "/", 3)
	if len(parts) < 2 || parts[0] != "homeassistant_instances" {
		return bareTopic
	}
	parts[1] = canonicalRoamingInstance
	return strings.Join(parts, "/")
}

// exportQualifier resolves the installation name device's Export cross-post is qualified under on
// the cloud broker -- device.ExportAs if set (currently only ever "roaming"), else ownInstallation
// (today's only behaviour, unchanged for every device that doesn't declare "roaming"). Added
// 2026-09-02 alongside "import"'s self-import unification -- see THassBridgeDevice.ExportAs' own
// doc comment and mqtt_relay.go's doc comment for the "hosts"-kind precedent this generalizes.
func exportQualifier(device THassBridgeDevice, ownInstallation string) string {
	if device.ExportAs != "" {
		return device.ExportAs
	}
	return ownInstallation
}

// expectedHassBridgeCloudTopics is expectedHassBridgeTopics' cloud-broker counterpart: every
// "export"-flagged device's capabilities get a qualified (exportQualifier) discovery topic on the
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
		qualifier := exportQualifier(device, ownInstallation)
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
// to the qualified topic directly and get real values, not just a catalogue listing. qualifier is
// already-resolved (exportQualifier) by the caller -- this function stays dumb, no per-device
// logic of its own. No-op if cloudClient is nil (no cloud broker configured).
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
//
// For a roaming device (device.ExportAs != ""), the cloud-bound topic's own instance segment is
// additionally rewritten to a fixed canonicalRoamingInstance (canonicalizeRoamingBridgeTopic),
// regardless of which real declared instance (parts[1]) actually reported this message -- fixes the
// surprise found live 2026-09-05 where the SAME roaming device cross-posted to two different cloud
// topics depending on which of its instances last reported, forcing an importer to watch both.
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

		topic, ok := hassBridgeDiscoveryTopic(localEntity, conceptualPrefix)
		if !ok {
			return
		}

		device := bridgeFile.Devices[found.DeviceID]
		devBlock := buildHassBridgeDeviceBlock(found.DeviceID, device, store.Snapshot(found.DeviceID))
		availabilityTopic := hassBridgeAvailabilityTopic(device, found.Capability)
		liveUnit, liveDeviceClass := "", ""
		if existenceTracker != nil {
			if source, ok := found.SourceEntities[parts[1]]; ok {
				liveUnit, liveDeviceClass = existenceTracker.LiveTyping(parts[1], source)
			}
		}
		body := buildHassBridgeEntityDiscoveryBody(hassBridgeUniqueID(localEntity), localEntity, found, msg.Topic(), devBlock, availabilityTopic, liveUnit, liveDeviceClass)
		data, err := json.Marshal(body)
		if err != nil {
			fmt.Printf("[hass-bridge] marshalling discovery payload for %s: %v\n", localEntity, err)
			return
		}
		if err := publisher.Publish(client, "main", topic, data); err != nil {
			fmt.Printf("[hass-bridge] publishing %s: %v\n", topic, err)
		}

		// Only cross-post traffic that genuinely originated from one of this device's OWN
		// declared real instances (parts[1], e.g. "main"/"protocols-server-2") -- not a message
		// subscribeHassBridgeSelfImport itself just relayed onto this same wildcard from a
		// sibling installation's cloud-published "roaming" data. Real bug found live 2026-09-05,
		// first time "roaming"+"import" was ever exercised for a real device: that relay
		// republishes onto a SYNTHETIC local topic named after ownInstallation (never a real
		// declared instance), which this same wildcard subscription would otherwise treat as
		// fresh local traffic and cross-post right back to cloud under "roaming" -- which the
		// self-import relay is ALSO subscribed to, republishing it locally again, in an unbounded
		// loop that flooded the shared cloud broker within seconds.
		if cloudClient != nil && device.Export && containsString(device.Instances, parts[1]) {
			qualifier := exportQualifier(device, ownInstallation)
			cloudBareTopic := canonicalizeRoamingBridgeTopic(msg.Topic(), device)
			crossPostHassBridgeToCloud(cloudClient, qualifier, cloudBareTopic, msg.Payload())
			cloudAvailabilityTopic := ""
			if availabilityTopic != "" {
				cloudAvailabilityTopic = qualifier + "/" + canonicalizeRoamingBridgeTopic(availabilityTopic, device)
			}
			stableID := exportStableID(qualifier, found.DeviceID, found.Capability)
			cloudBody := buildHassBridgeEntityDiscoveryBody(stableID, localEntity, found, qualifier+"/"+cloudBareTopic, devBlock, cloudAvailabilityTopic, liveUnit, liveDeviceClass)
			cloudBody["installation"] = qualifier
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
	token := client.Subscribe(filter, 0, handler)
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
		if !known {
			return
		}

		var fields map[string]string
		if err := json.Unmarshal(msg.Payload(), &fields); err != nil {
			fmt.Printf("[hass-bridge] %s: cannot parse device-info payload: %v\n", msg.Topic(), err)
			return
		}
		store.Update(deviceID, fields)

		// Same self-import feedback-loop guard as subscribeHassBridge's own handler -- see its
		// doc comment for the full incident. parts[1] here is the reporting instance segment
		// ("main"/"protocols-server-2"/...), never ownInstallation itself.
		exportToCloud := cloudClient != nil && device.Export && containsString(device.Instances, parts[1])
		qualifier := exportQualifier(device, ownInstallation)

		devBlock := buildHassBridgeDeviceBlock(deviceID, device, store.Snapshot(deviceID))
		for capability, cap := range device.Capabilities {
			topic, ok := hassBridgeDiscoveryTopic(cap.LocalEntity, conceptualPrefix)
			if !ok {
				continue
			}
			found := hassBridgeMatch{Capability: capability, DeviceID: deviceID, THassBridgeCapability: cap}
			availabilityTopic := hassBridgeAvailabilityTopic(device, capability)
			liveUnit, liveDeviceClass := "", ""
			if existenceTracker != nil {
				if source, ok := cap.SourceEntities[hassBridgeDefaultInstance(device)]; ok {
					liveUnit, liveDeviceClass = existenceTracker.LiveTyping(hassBridgeDefaultInstance(device), source)
				}
			}
			body := buildHassBridgeEntityDiscoveryBody(hassBridgeUniqueID(cap.LocalEntity), cap.LocalEntity, found, hassBridgeEntityStateTopic(hassBridgeDefaultInstance(device), cap.LocalEntity), devBlock, availabilityTopic, liveUnit, liveDeviceClass)
			data, err := json.Marshal(body)
			if err != nil {
				fmt.Printf("[hass-bridge] marshalling discovery payload for %s: %v\n", cap.LocalEntity, err)
				continue
			}
			if err := publisher.Publish(client, "main", topic, data); err != nil {
				fmt.Printf("[hass-bridge] publishing %s: %v\n", topic, err)
			}
			if exportToCloud {
				cloudStateTopic := qualifier + "/" + canonicalizeRoamingBridgeTopic(hassBridgeEntityStateTopic(hassBridgeDefaultInstance(device), cap.LocalEntity), device)
				cloudAvailabilityTopic := ""
				if availabilityTopic != "" {
					cloudAvailabilityTopic = qualifier + "/" + canonicalizeRoamingBridgeTopic(availabilityTopic, device)
				}
				stableID := exportStableID(qualifier, deviceID, capability)
				cloudBody := buildHassBridgeEntityDiscoveryBody(stableID, cap.LocalEntity, found, cloudStateTopic, devBlock, cloudAvailabilityTopic, liveUnit, liveDeviceClass)
				cloudBody["installation"] = qualifier
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

		if exportToCloud {
			crossPostHassBridgeToCloud(cloudClient, qualifier, canonicalizeRoamingBridgeTopic(msg.Topic(), device), msg.Payload())
		}
	}

	const filter = "homeassistant_instances/+/bridge/device/+/state"
	token := client.Subscribe(filter, 0, handler)
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		if err := token.Error(); err != nil {
			return fmt.Errorf("subscribing to %s: %w", filter, err)
		}
		return fmt.Errorf("subscribing to %s: timed out", filter)
	}
	return nil
}

// subscribeHassBridgeSelfImport relays cloud-broker traffic published under a device's own
// export qualifier (exportQualifier) back onto this instance's own bare local bridge topic --
// modeled directly on relayCloudDevice (mqtt_relay.go, the "hosts"-side precedent this
// generalizes), deliberately NOT on TImportedDevice/discoveryimport.go, which would
// create a second, structurally distinct discovery entity (different unique_id/topics) instead of
// feeding the ONE entity Spaces.def positioning already creates for this device via its own local
// "export"/"roaming" declaration. Only devices with SelfImportFrom set (the "import" keyword,
// homeassistant/integration_hassbridge_parser.go) are relayed -- letting this coordinator see its
// own exported data, or a SIBLING installation's, whichever is currently live, without needing to
// know or care which. Neither handler checks the arriving topic's own instance segment (parts[1]
// for state, parts[4] for device-info) against anything -- subscribeHassBridge/
// subscribeHassBridgeDeviceInfo's own LOCAL handlers already don't either (see their own topic
// parsing), so a relayed message is picked up identically to a genuinely local one regardless of
// which real instance actually published it.
//
// Operational precondition, not enforced here: every real installation sharing a "roaming" device
// must declare it under the identical literal DeviceID in its own Physical.def, or a sibling's
// relayed traffic will never resolve to a capability this coordinator's own bridgeFile knows
// about (byLocal/bridgeFile.Devices lookups below simply won't match, and the message is silently
// dropped).
//
// No-op if cloudClient is nil, or no device declares SelfImportFrom.
func subscribeHassBridgeSelfImport(cloudClient, mainClient mqtt.Client, ownInstallation string, bridgeFile THassBridgeFile) error {
	if cloudClient == nil {
		return nil
	}
	byLocal := hassBridgeCapabilitiesByLocalEntity(bridgeFile)
	qualifiers := map[string]bool{}
	for _, device := range bridgeFile.Devices {
		if device.SelfImportFrom != "" {
			qualifiers[exportQualifier(device, ownInstallation)] = true
		}
	}
	if len(qualifiers) == 0 {
		return nil
	}

	stateHandler := func(_ mqtt.Client, msg mqtt.Message) {
		// <qualifier>/homeassistant_instances/<source-instance>/bridge/<local_entity>/state
		parts := strings.Split(msg.Topic(), "/")
		if len(parts) != 6 || parts[1] != "homeassistant_instances" || parts[3] != "bridge" || parts[5] != "state" {
			return
		}
		found, known := byLocal[parts[4]]
		if !known || bridgeFile.Devices[found.DeviceID].SelfImportFrom == "" {
			return
		}
		target := hassBridgeEntityStateTopic(ownInstallation, parts[4])
		token := mainClient.Publish(target, 0, true, msg.Payload())
		if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
			if err := token.Error(); err != nil {
				fmt.Printf("[hass-bridge:self-import] relaying %s -> %s: %v\n", msg.Topic(), target, err)
			}
		}
	}

	deviceInfoHandler := func(_ mqtt.Client, msg mqtt.Message) {
		// <qualifier>/homeassistant_instances/<source-instance>/bridge/device/<deviceID>/state
		parts := strings.Split(msg.Topic(), "/")
		if len(parts) != 7 || parts[1] != "homeassistant_instances" || parts[3] != "bridge" || parts[4] != "device" || parts[6] != "state" {
			return
		}
		deviceID := parts[5]
		if bridgeFile.Devices[deviceID].SelfImportFrom == "" {
			return
		}
		target := "homeassistant_instances/" + ownInstallation + "/bridge/device/" + deviceID + "/state"
		token := mainClient.Publish(target, 0, true, msg.Payload())
		if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
			if err := token.Error(); err != nil {
				fmt.Printf("[hass-bridge:self-import] relaying %s -> %s: %v\n", msg.Topic(), target, err)
			}
		}
	}

	for qualifier := range qualifiers {
		for filter, handler := range map[string]mqtt.MessageHandler{
			qualifier + "/homeassistant_instances/+/bridge/+/state":        stateHandler,
			qualifier + "/homeassistant_instances/+/bridge/device/+/state": deviceInfoHandler,
		} {
			tok := cloudClient.Subscribe(filter, 0, handler)
			if !tok.WaitTimeout(10*time.Second) || tok.Error() != nil {
				if err := tok.Error(); err != nil {
					return fmt.Errorf("subscribing to %s: %w", filter, err)
				}
				return fmt.Errorf("subscribing to %s: timed out", filter)
			}
		}
	}
	return nil
}
