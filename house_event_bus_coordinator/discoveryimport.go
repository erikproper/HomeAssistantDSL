/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: DiscoveryImport
 *
 * Consumes another installation's exported devices (either "hosts"-kind or "home_assistant"-kind,
 * see homeassistant/integration_import_storage.go's own header comment for the 2026-09-05
 * grammar unification that made this kind-agnostic) off the shared cloud broker, per Physical.def's
 * unified import declarations (coordinator/imported.yaml). Deliberately reuses discoverybridge.go's
 * own shape -- subscribe to a "<prefix>/+/+/+/config" filter, decode each incoming discovery-shaped
 * payload, match it against a declared capability (matchImportedCapabilityByStableID -- purely from
 * the topic's own stable-id segment, exportStableID, discoveryhassbridge.go; never the payload's own
 * content), republish our own local discovery config pointing at a local relay topic -- since both
 * exporting kinds already publish this shape to the cloud broker as their own "cloud catalogue"
 * entry (house_event_bus_coordinator/mqtt.go's publishDeviceDiscovery), indistinguishable, at the
 * discovery-config layer, from a kind-2 gateway's own native self-description (see
 * project_native_export_cloud_crosspost.md). The one structural difference: discoverybridge.go's
 * gateway is on the LOCAL broker; an import's source device is on the CLOUD broker, under the
 * exporting installation's own qualified topic prefix.
 *
 * Kind-agnostic resolution (2026-09-05): the importing side never knows or asks which integration
 * kind the exporter used. It resolves two independent, structural facts purely from fields already
 * present on the cloud discovery payload:
 *   - Does the raw state topic need cross-house qualification? -- "hosts"-kind topics are bare
 *     (e.g. "hosts/<host>/cpu/state", not installation-scoped by construction, per
 *     mqtt_relay.go's qualifyHostsTopic), while "home_assistant"-kind topics are already
 *     self-qualifying ("homeassistant_instances/<instance>/bridge/<entity>/state" -- the instance
 *     name is baked in at generation time on the exporting side). A bare "hosts/..." prefix is the
 *     one and only shape that ever needs qualifyHostsTopic applied; everything else is used as-is.
 *   - Does the raw payload need JSON-blob extraction? -- a "hosts"-kind attribute's discovery
 *     config carries a "value_template" field ("{{ value_json.<attr> }}", the one shape
 *     homeassistant/discovery.go ever generates) because several attributes share one JSON-blob
 *     topic; a "home_assistant"-kind capability's own topic carries its value directly, no
 *     value_template. Presence of that field is what decides whether this side must itself parse
 *     the incoming JSON and pull one field out before relaying, rather than forwarding verbatim.
 * Neither check is a heuristic: today's system only ever produces these two raw-topic/payload
 * shapes into a cloud discovery payload, so a plain prefix/field-presence check is exact.
 *
 * Each declared capability's LOCAL entity id comes straight from coordinator/imported.yaml's
 * "local_entity" field -- resolved generator-side from Spaces.def's own positioning
 * (Conceptual_DevicePositioning.go/Conceptual_DeviceSourceEntities.go's registerImportedDevice*
 * functions), never invented here. Per Architecture.md's "Physical.def/Spaces.def stay the naming
 * authority" principle -- see project_vienna_coordinator_on_frame.md's own writeup for why this
 * mattered enough to redo (the first draft had this file invent its own "imported_<device>_
 * <capability>" naming, which orphaned "windy"/"battery_alert"-style Spaces.def references to a
 * device's entities and left no positioning/naming continuity with the rest of the DSL). This is
 * also what makes expectedImportedTopics fully precomputable ahead of time (every
 * declared capability's local_entity is already resolved by generate time), avoiding the exact
 * class of orphan-topic-cleanup bug the export side hit live 2026-08-30 (see this file's own
 * sibling incident, discoveryhassbridge.go/discoverycleanup.go) -- an import's local discovery
 * configs belong in main.go's ordinary "main" broker expected-topics set, same as any other
 * locally-known device.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 05.09.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gopkg.in/yaml.v3"
)

// TImportedCapability is one declared capability's resolved LOCAL entity id, the one Spaces.def's
// own positioning gave it (LocalEntity, generator-resolved -- empty would mean "never actually
// used," but generateImportedDeviceFile already omits any capability that never resolved, so every
// entry that reaches this file always has one). The capability's own map key, combined with its
// owning TImportedDevice's (RemoteInstallation, RemoteDeviceID), is everything
// matchImportedCapabilityByStableID needs to recognise which incoming cloud discovery payload it
// corresponds to (exportStableID, discoveryhassbridge.go) -- there was an earlier, more verbose
// explicit form carrying a RemoteEntityRef string field (the exporter's own local entity_id,
// matched against each incoming payload's "default_entity_id"), removed 2026-09-07 once no real
// Physical.def still used it.
type TImportedCapability struct {
	LocalEntity string `yaml:"local_entity"`
}

// TImportedDevice is one declared import from coordinator/imported.yaml.
// DisplayName is THIS house's own space-positioning-derived name for the device
// (deviceDisplayName, generator-side) -- never the exporting installation's own name for it, same
// principle as every other device kind's own "device:" block name.
type TImportedDevice struct {
	RemoteInstallation string                         `yaml:"remote_installation"`
	RemoteDeviceID     string                         `yaml:"remote_device_id"`
	DisplayName        string                         `yaml:"display_name"`
	Capabilities       map[string]TImportedCapability `yaml:"capabilities"`
}

// TImportedFile is the top-level shape of a generated coordinator/imported.yaml.
type TImportedFile struct {
	Devices map[string]TImportedDevice `yaml:"devices"`
}

// loadImportedFile mirrors loadHassBridgeFile/loadDiscoveryFile's own "missing file ->
// zero value, not an error" pattern -- most houses import nothing.
func loadImportedFile(path string) (TImportedFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return TImportedFile{}, nil
		}
		return TImportedFile{}, fmt.Errorf("cannot read %s: %w", path, err)
	}
	var importFile TImportedFile
	if err := yaml.Unmarshal(data, &importFile); err != nil {
		return TImportedFile{}, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	return importFile, nil
}

// importedUniqueID and importedStateTopic are the two computed identifiers built from a
// capability's already-resolved LocalEntity -- shared by expectedImportedTopics and
// subscribeImportedDevices' own handler so the two can never drift apart on naming, same
// discipline hassBridgeUniqueID/hassBridgeDiscoveryTopic already follow for the export side.
func importedUniqueID(localEntity string) string {
	return "imported_" + strings.ReplaceAll(localEntity, ".", "_")
}

func importedStateTopic(localEntity string) string {
	return "imports/" + localEntity + "/state"
}

// expectedImportedTopics computes every local discovery topic importFile's declared
// (and, per generateImportedDeviceFile's own gate, already Spaces.def-positioned) capabilities
// imply -- fully known ahead of time from the generated file alone (this file's own header
// comment explains why), for merging into main.go's ordinary "main"-broker expected-topics set.
func expectedImportedTopics(importFile TImportedFile, prefix string) map[string]bool {
	expected := map[string]bool{}
	for _, device := range importFile.Devices {
		for _, cap := range device.Capabilities {
			if cap.LocalEntity == "" {
				continue
			}
			dotIdx := strings.Index(cap.LocalEntity, ".")
			if dotIdx < 0 {
				continue
			}
			domain := cap.LocalEntity[:dotIdx]
			expected[discoveryTopic(prefix, domain, importedUniqueID(cap.LocalEntity))] = true
		}
	}
	return expected
}

// importedDiscoveryPayload decodes an exported discovery payload -- the full-key-name shape both
// buildDiscoveryConfigs (homeassistant/discovery.go, "hosts"-kind) and
// buildHassBridgeEntityDiscoveryBody (discoveryhassbridge.go, "home_assistant"-kind) always write,
// never HA's own abbreviated MQTT discovery keys (which only ever appear on genuinely
// gateway-authored payloads, discoverybridge.go's own concern) -- so this intentionally does NOT
// reuse decodeDiscoveryPayload/rawDiscoveryPayload from that file. ValueTemplate is only ever set
// on a "hosts"-kind attribute (this file's own header comment explains why); the Device sub-fields
// are redundantly present on every capability's own payload, letting an imported device's local
// "device:" block pick up live metadata the same way a real cross-house "hosts" import already did
// before the 2026-09-05 unification (a gap this generalization closes for "home_assistant"-kind
// imports too).
type importedDiscoveryPayload struct {
	DefaultEntityID string `json:"default_entity_id"`
	StateTopic      string `json:"state_topic"`
	ValueTemplate   string `json:"value_template"`
	DeviceClass     string `json:"device_class"`
	Unit            string `json:"unit_of_measurement"`
	StateClass      string `json:"state_class"`
	Icon            string `json:"icon"`
	// PayloadOn/PayloadOff carry a hosts-kind node capability's own coordinator-chosen
	// "true"/"false" convention (discovery.go's TBinarySensorDiscoveryPayload) through the cloud
	// discovery payload verbatim, so buildImportedDiscoveryBody can preserve them instead of
	// assuming every binary_sensor import used a real HA entity's own native "on"/"off" (only true
	// for a hassbridge-kind import). Both empty for anything else -- a hassbridge-sourced
	// discovery config never sets these itself.
	PayloadOn  string `json:"payload_on"`
	PayloadOff string `json:"payload_off"`
	Device     struct {
		Identifiers  []string `json:"identifiers"`
		Manufacturer string   `json:"manufacturer"`
		Model        string   `json:"model"`
		ModelID      string   `json:"model_id"`
		HwVersion    string   `json:"hw_version"`
		SwVersion    string   `json:"sw_version"`
		SerialNumber string   `json:"serial_number"`
	} `json:"device"`
}

// extractValueTemplateField parses the one value_template shape this codebase ever generates --
// "{{ value_json.<field> }}" (homeassistant/discovery.go's buildDiscoveryConfigs, unconditional,
// for every "hosts"-kind attribute) -- into the bare JSON field name, or ok=false if it doesn't
// match that exact shape. Deliberately not a general Jinja parser: an importer should never guess
// at an arbitrary template, only recognise the one fixed form this system's own exporters produce.
func extractValueTemplateField(valueTemplate string) (field string, ok bool) {
	const prefix = "{{ value_json."
	const suffix = " }}"
	if !strings.HasPrefix(valueTemplate, prefix) || !strings.HasSuffix(valueTemplate, suffix) {
		return "", false
	}
	return valueTemplate[len(prefix) : len(valueTemplate)-len(suffix)], true
}

// importCapabilityMatch is one resolved (declared import, capability) pair a decoded payload
// matched against, plus its already-resolved local identity.
type importCapabilityMatch struct {
	LocalDeviceID string
	Capability    string
	LocalEntity   string
	DisplayName   string
}

// matchImportedCapabilityByStableID finds which declared import capability (if any) a topic
// belongs to: it recomputes the EXPORT side's own stable id (exportStableID,
// discoveryhassbridge.go) from each declared capability's (RemoteInstallation, RemoteDeviceID,
// capability-name) and compares it against topic's own stable-id segment --
// "<installation>/<prefix...>/<domain>/coordinator/<stableID>/config", indexed from the end so an
// arbitrarily-shaped (possibly multi-segment) prefix never throws off the offsets. This never
// inspects the payload's own domain or default_entity_id at all -- deliberate coercion: whatever
// domain the exporter actually used is irrelevant, only the stable id (independent of any naming
// on either side) has to match. Skips a capability with no resolved LocalEntity (shouldn't happen
// -- generateImportedDeviceFile already omits those -- but defensive rather than publishing
// something with no real local identity).
//
// An earlier version matched an explicit-form capability's declared RemoteEntityRef against the
// payload's own default_entity_id instead (matchImportedCapability, content-based) -- removed
// 2026-09-07 once the explicit form itself was removed (no real Physical.def still used it; see
// homeassistant/integration_import_parser.go's own header comment).
func matchImportedCapabilityByStableID(importFile TImportedFile, remoteInstallation, topic string) (importCapabilityMatch, bool) {
	parts := strings.Split(topic, "/")
	if len(parts) < 3 || parts[len(parts)-1] != "config" || parts[len(parts)-3] != "coordinator" {
		return importCapabilityMatch{}, false
	}
	stableID := parts[len(parts)-2]
	for localDeviceID, device := range importFile.Devices {
		if device.RemoteInstallation != remoteInstallation {
			continue
		}
		for capability, cap := range device.Capabilities {
			if cap.LocalEntity == "" {
				continue
			}
			if exportStableID(device.RemoteInstallation, device.RemoteDeviceID, capability) != stableID {
				continue
			}
			return importCapabilityMatch{LocalDeviceID: localDeviceID, Capability: capability, LocalEntity: cap.LocalEntity, DisplayName: device.DisplayName}, true
		}
	}
	return importCapabilityMatch{}, false
}

// importedAvailabilityTopic resolves match's own device's "node" (connectivity) capability's
// LOCAL relay state topic, then applies the shared availabilityTopicFor gate (discovery.go) --
// mirrors hassBridgeAvailabilityTopic's own reasoning (discoveryhassbridge.go) one hop further
// downstream: an imported device's other entities go unavailable exactly when its imported "node"
// entity does.
func importedAvailabilityTopic(importFile TImportedFile, match importCapabilityMatch) string {
	nodeTopic := ""
	if device, ok := importFile.Devices[match.LocalDeviceID]; ok {
		if node, ok := device.Capabilities["node"]; ok && node.LocalEntity != "" {
			nodeTopic = importedStateTopic(node.LocalEntity)
		}
	}
	return availabilityTopicFor(nodeTopic, match.Capability)
}

// buildImportedDiscoveryBody builds the local discovery config for one matched capability --
// device_class/unit/state_class/icon copied verbatim from the exporting installation's own
// payload (it already resolved them, including any Defaults.def gap-filling on its own side;
// there is nothing left for this side to add), state_topic pointing at the LOCAL relay topic
// (importedStateTopic) rather than the remote's own cloud-qualified one -- the raw value still
// needs relaying onto that local topic (subscribeImportedDevices' own relay handler) for
// it to ever carry data, same non-negotiable requirement export's own cloud-side discovery config
// has. default_entity_id/name/unique_id/state_topic are all derived from match.LocalEntity --
// Spaces.def's own resolved naming, never invented here. The device block's own "name" is
// match.DisplayName (this house's own space-positioning-derived name, e.g.
// "apartment/bedroom/netatmo") when set -- NEVER payload.Device.Name (the exporting
// installation's own name for it, e.g. bare "vienna_bedroom") -- a real bug found live
// 2026-08-30: falling back to the remote's own name is exactly the same mistake as taking a
// hosts device's display name from anywhere but its own local positioning. nodeAvailabilityTopic
// is "" for the node capability itself, or when its device declares no "node" capability -- see
// importedAvailabilityTopic. Availability is always set (via the shared buildAvailabilityFields,
// discovery.go) -- refined live 2026-08-31 to AND the device's own node connectivity (when
// applicable) with the capability's own last-reported value never being the literal string
// "unavailable": node being reachable doesn't guarantee this specific reading is currently valid.
// The entity's own "name" bakes in that same deviceName ahead of the bare capability -- fixed
// live 2026-09-02 alongside the identical fix in discoveryhassbridge.go/discovery.go, for the same
// reason: MQTT discovery entities can't use "has_entity_name", so a bare capability name left
// every such entity showing no location context at all in HA's UI.
func buildImportedDiscoveryBody(match importCapabilityMatch, payload importedDiscoveryPayload, nodeAvailabilityTopic string) map[string]interface{} {
	deviceName := match.DisplayName
	if deviceName == "" {
		deviceName = match.LocalDeviceID
	}
	stateTopic := importedStateTopic(match.LocalEntity)
	deviceBlock := map[string]interface{}{
		"identifiers": []string{match.LocalDeviceID},
		"name":        deviceName,
	}
	// The remote's own already-resolved device metadata (manufacturer/model/...) is redundantly
	// present on every capability's own payload -- picked up here the same way a real cross-house
	// "hosts" import already did before the 2026-09-05 grammar unification (relayCloudDevice's
	// separate device-info relay, mqtt_relay.go), now for every import kind alike.
	if payload.Device.Manufacturer != "" {
		deviceBlock["manufacturer"] = payload.Device.Manufacturer
	}
	if payload.Device.Model != "" {
		deviceBlock["model"] = payload.Device.Model
	}
	if payload.Device.ModelID != "" {
		deviceBlock["model_id"] = payload.Device.ModelID
	}
	if payload.Device.HwVersion != "" {
		deviceBlock["hw_version"] = payload.Device.HwVersion
	}
	if payload.Device.SwVersion != "" {
		deviceBlock["sw_version"] = payload.Device.SwVersion
	}
	if payload.Device.SerialNumber != "" {
		deviceBlock["serial_number"] = payload.Device.SerialNumber
	}
	body := map[string]interface{}{
		"unique_id":         importedUniqueID(match.LocalEntity),
		"default_entity_id": match.LocalEntity,
		"name":              deviceName + "/" + match.Capability,
		"state_topic":       stateTopic,
		"device":            deviceBlock,
	}
	if payload.DeviceClass != "" {
		body["device_class"] = payload.DeviceClass
	}
	if payload.Unit != "" {
		body["unit_of_measurement"] = payload.Unit
	}
	if payload.StateClass != "" {
		body["state_class"] = payload.StateClass
	}
	if payload.Icon != "" {
		body["icon"] = payload.Icon
	}
	// A hosts-kind node capability's own remote discovery payload already carries its
	// coordinator-chosen "true"/"false" convention (payload.PayloadOn/PayloadOff, set by
	// discovery.go's TBinarySensorDiscoveryPayload) -- preserved verbatim here rather than
	// overwritten. Real bug found live 2026-09-05: applyProxiedBinarySensorPayload's "on"/"off"
	// default is only correct for a hassbridge-sourced binary_sensor (a real HA entity reporting
	// its own native lowercase state); blindly applying it to every imported binary_sensor
	// regardless of origin left every imported hosts-kind node entity stuck at "unknown" forever,
	// since its actual state topic payload ("true"/"false") never matched HA's expected "on"/"off".
	if match.LocalEntity != "" && strings.HasPrefix(match.LocalEntity, "binary_sensor.") && payload.PayloadOn != "" && payload.PayloadOff != "" {
		body["payload_on"] = payload.PayloadOn
		body["payload_off"] = payload.PayloadOff
	} else {
		applyProxiedBinarySensorPayload(body, match.LocalEntity)
	}
	buildAvailabilityFields(body, stateTopic, nodeAvailabilityTopic)
	return body
}

// TImportedHostsRelay is one capability's resolved relay target -- LocalEntity is where to
// republish, ExtractionField is the JSON key to pull out of the raw incoming payload first (""
// means republish the raw payload verbatim, e.g. a "node" liveness boolean, which is never
// JSON-wrapped -- see mqtt_relay.go's own doc comment on why a real value reaches this topic at
// all: the exporting installation's own TCloudLivenessTracker cross-posts its computed liveness to
// the cloud broker, so this side never needs to synthesize anything of its own). A capability's
// raw source topic (and, for a "hosts"-kind one sharing a JSON-blob topic with others, its
// extraction key) is only knowable once the exporting installation's own discovery CONFIG payload
// (StateTopic/ValueTemplate) has actually arrived -- see this file's own header comment --
// registered into TImportedHostsRelayIndex the moment configHandler sees it.
type TImportedHostsRelay struct {
	LocalEntity     string
	ExtractionField string
}

// TImportedHostsRelayIndex maps a "hosts"-kind capability's fully-qualified cloud source topic
// (e.g. "hosts/mqtt/junglinster/cpu/state", after qualifyHostsTopic) to every locally-declared
// capability relayed from it, learned lazily as each capability's own discovery CONFIG message
// arrives. Several capabilities (e.g. "load" and "temperature") share one such topic, since
// "hosts"-kind attributes are published as one JSON blob per device -- this index is what lets one
// incoming message fan out to every locally-declared capability it carries. Guarded by a mutex
// since the config and state MQTT subscriptions run their handlers concurrently.
type TImportedHostsRelayIndex struct {
	mu      sync.Mutex
	byTopic map[string][]TImportedHostsRelay
}

func newImportedHostsRelayIndex() *TImportedHostsRelayIndex {
	return &TImportedHostsRelayIndex{byTopic: map[string][]TImportedHostsRelay{}}
}

func (idx *TImportedHostsRelayIndex) register(topic string, relay TImportedHostsRelay) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	for _, existing := range idx.byTopic[topic] {
		if existing.LocalEntity == relay.LocalEntity {
			return
		}
	}
	idx.byTopic[topic] = append(idx.byTopic[topic], relay)
}

func (idx *TImportedHostsRelayIndex) lookup(topic string) []TImportedHostsRelay {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return append([]TImportedHostsRelay(nil), idx.byTopic[topic]...)
}

// subscribeImportedDevices subscribes on cloudClient to every distinct declared
// RemoteInstallation's own "<installation>/<prefix>/+/+/+/config" filter, matches each incoming
// exported discovery payload against importFile (matchImportedCapabilityByStableID), and publishes
// a local discovery config (through publisher, to "main") whose state_topic is a new local relay
// topic.
// State itself is relayed through two SEPARATE static wildcard subscribes registered once per
// remote installation (never per capability -- see the concurrency note below): one for
// "home_assistant"-kind's self-qualifying topic shape
// ("<installation>/homeassistant_instances/+/bridge/+/state", mirroring
// crossPostHassBridgeToCloud's own shape in reverse), one for "hosts"-kind's bare, cross-house
// qualified shape ("hosts/+/<installation>/+/state", resolved via TImportedHostsRelayIndex once
// the matching capability's own discovery config has been seen). No-op if cloudClient is nil or
// importFile has no devices declared.
//
// A "hosts"-kind device's own "node" capability carries no real reported value of its own (the
// deployed report script never publishes one) -- but this side needs no special handling for
// that: the EXPORTING installation's own TCloudLivenessTracker (mqtt_relay.go's "Availability
// inversion") computes it and cross-posts the result to the cloud broker, qualified exactly like
// any other attribute, so it arrives here as an ordinary "hosts"-kind message and gets relayed
// through hostsStateHandler like any other capability -- no separate liveness synthesis needed on
// the importing side at all (an earlier version of this file did synthesize it here; removed
// 2026-09-05 once the exporter-side fix made it redundant).
//
// The state relay used to be a dynamically-spawned per-capability subscribe, set up (via its own
// goroutine, to avoid blocking the config handler's own SUBACK processing) the first time each
// capability's discovery CONFIG message was seen. Real bug found live 2026-08-31: at startup,
// every declared capability's retained CONFIG message arrives in one burst -- Vienna's own 25+
// capabilities meant 25+ concurrent cloudClient.Subscribe calls fired nearly simultaneously from
// separate goroutines on the same client. Some of those subscriptions were silently lost (paho's
// own internal bookkeeping under concurrent Subscribe calls, not fully root-caused), leaving the
// affected capabilities' state relay permanently dead -- and, unlike Junglinster's own single
// static subscribe (discoveryhassbridge.go's subscribeHassBridge, fixed live the same day by a
// plain restart), a full coordinator restart did NOT reliably recover it, since the very same
// concurrent-burst pattern replayed identically on every restart. One static subscribe per remote
// installation, registered once, removes the concurrent-spawn entirely -- the same "wildcard
// covers every capability" shape subscribeHassBridge's own local-side subscribe already uses.
// existenceTracker may be nil (tests, or a coordinator run before kind-4 tracking existed) -- when
// set, every matched capability's discovery config marks its own stable id known-to-exist
// (import_existence.go's TImportExistenceTracker), republishing that installation's status
// snapshot (qualified with ownInstallation on the cloud copy, same as every other kind's own
// publishStatus) to both brokers on each new observation.
func subscribeImportedDevices(mainClient, cloudClient mqtt.Client, ownInstallation string, importFile TImportedFile, publisher *TDiscoveryPublisher, conceptualPrefix string, existenceTracker *TImportExistenceTracker) error {
	if cloudClient == nil || len(importFile.Devices) == 0 {
		return nil
	}

	remoteInstallations := map[string]bool{}
	for _, device := range importFile.Devices {
		remoteInstallations[device.RemoteInstallation] = true
	}

	hostsRelayIndex := newImportedHostsRelayIndex()

	for remoteInstallation := range remoteInstallations {
		remoteInstallation := remoteInstallation
		configHandler := func(_ mqtt.Client, msg mqtt.Message) {
			if len(msg.Payload()) == 0 {
				return // retraction handling: not built yet, see PROJECT.md 1.2d follow-ups.
			}
			var payload importedDiscoveryPayload
			if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
				fmt.Printf("[discovery-import] %s: cannot parse payload: %v\n", msg.Topic(), err)
				return
			}
			match, matched := matchImportedCapabilityByStableID(importFile, remoteInstallation, msg.Topic())
			if !matched {
				return
			}
			if existenceTracker != nil {
				parts := strings.Split(msg.Topic(), "/")
				stableID := parts[len(parts)-2]
				if existenceTracker.MarkKnown(remoteInstallation, stableID) {
					fmt.Printf("[import-existence] %s: %s -> known-to-exist\n", remoteInstallation, stableID)
					if err := existenceTracker.publishStatus(mainClient, cloudClient, ownInstallation, remoteInstallation); err != nil {
						fmt.Printf("[import-existence] %v\n", err)
					}
				}
			}

			extractionField := ""
			if payload.ValueTemplate != "" {
				if field, ok := extractValueTemplateField(payload.ValueTemplate); ok {
					extractionField = field
				}
			}
			switch {
			case strings.HasPrefix(payload.StateTopic, "hosts/"):
				// A "hosts"-kind capability's raw topic is bare and not self-qualifying (this
				// file's own header comment) -- learn its real cross-house topic and, if this
				// attribute shares a JSON-blob topic with others (value_template set), which field
				// to pull out of it, so the state relay below (registered once, not per
				// capability) knows what to do with a message on that topic.
				qualifiedTopic := qualifyHostsTopic(payload.StateTopic, remoteInstallation)
				hostsRelayIndex.register(qualifiedTopic, TImportedHostsRelay{LocalEntity: match.LocalEntity, ExtractionField: extractionField})
			case payload.StateTopic != "":
				// A "home_assistant"-kind capability's own state topic is already self-qualifying
				// (unlike "hosts"-kind's bare one above) -- registering it here lets stateHandler
				// relay it the same lazy, topic-learned way, the only mechanism that ever learns
				// this mapping at all, since a declaration never states the remote's own local
				// entity name up front.
				hostsRelayIndex.register(payload.StateTopic, TImportedHostsRelay{LocalEntity: match.LocalEntity, ExtractionField: extractionField})
			}

			body := buildImportedDiscoveryBody(match, payload, importedAvailabilityTopic(importFile, match))
			dotIdx := strings.Index(match.LocalEntity, ".")
			if dotIdx < 0 {
				return
			}
			topic := discoveryTopic(conceptualPrefix, match.LocalEntity[:dotIdx], importedUniqueID(match.LocalEntity))
			data, err := json.Marshal(body)
			if err != nil {
				fmt.Printf("[discovery-import] marshalling %s: %v\n", match.LocalEntity, err)
				return
			}
			if err := publisher.Publish(mainClient, "main", topic, data); err != nil {
				fmt.Printf("[discovery-import] publishing %s: %v\n", topic, err)
			}
		}

		configFilter := remoteInstallation + "/" + conceptualPrefix + "/+/+/+/config"
		token := cloudClient.Subscribe(configFilter, 0, configHandler)
		if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
			if err := token.Error(); err != nil {
				return fmt.Errorf("subscribing to %s: %w", configFilter, err)
			}
			return fmt.Errorf("subscribing to %s: timed out", configFilter)
		}

		stateHandler := func(_ mqtt.Client, msg mqtt.Message) {
			// "<installation>/homeassistant_instances/<instance>/bridge/<remoteLocalEntity>/state"
			// -- the exact shape crossPostHassBridgeToCloud qualifies hassBridgeEntityStateTopic
			// with, six segments. Relay target is resolved from whatever configHandler's own
			// lazy-learned index resolved this exact topic to (registered the moment that
			// capability's discovery config was first seen) -- a topic nothing has registered yet
			// (or ever will) is a safe no-op, same as hostsStateHandler's own lookup below.
			parts := strings.Split(msg.Topic(), "/")
			if len(parts) != 6 || parts[1] != "homeassistant_instances" || parts[3] != "bridge" || parts[5] != "state" {
				return
			}
			for _, relay := range hostsRelayIndex.lookup(msg.Topic()) {
				localStateTopic := importedStateTopic(relay.LocalEntity)
				pubToken := mainClient.Publish(localStateTopic, 0, true, msg.Payload())
				if !pubToken.WaitTimeout(10*time.Second) || pubToken.Error() != nil {
					if err := pubToken.Error(); err != nil {
						fmt.Printf("[discovery-import] relaying %s -> %s: %v\n", msg.Topic(), localStateTopic, err)
					}
				}
			}
		}

		stateFilter := remoteInstallation + "/homeassistant_instances/+/bridge/+/state"
		token = cloudClient.Subscribe(stateFilter, 0, stateHandler)
		if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
			if err := token.Error(); err != nil {
				return fmt.Errorf("subscribing to %s: %w", stateFilter, err)
			}
			return fmt.Errorf("subscribing to %s: timed out", stateFilter)
		}

		// "hosts"-kind's own bare, cross-house-qualified shape: "hosts/<host>/<installation>/
		// <suffix>/state" (5 segments -- qualifyHostsTopic inserts the installation right after
		// the host segment). One static wildcard per remote installation, same non-concurrent-
		// spawn discipline as stateFilter above: hostsRelayIndex is populated lazily by
		// configHandler as each capability's own discovery config arrives, so a message on a topic
		// nothing has registered yet is simply a safe no-op (lookup returns nothing to relay).
		hostsStateHandler := func(_ mqtt.Client, msg mqtt.Message) {
			relays := hostsRelayIndex.lookup(msg.Topic())
			for _, relay := range relays {
				payload := msg.Payload()
				if relay.ExtractionField != "" {
					var fields map[string]json.RawMessage
					if err := json.Unmarshal(msg.Payload(), &fields); err != nil {
						fmt.Printf("[discovery-import] %s: cannot parse JSON payload for extraction: %v\n", msg.Topic(), err)
						continue
					}
					raw, found := fields[relay.ExtractionField]
					if !found {
						continue
					}
					payload = raw
				}
				localStateTopic := importedStateTopic(relay.LocalEntity)
				pubToken := mainClient.Publish(localStateTopic, 0, true, payload)
				if !pubToken.WaitTimeout(10*time.Second) || pubToken.Error() != nil {
					if err := pubToken.Error(); err != nil {
						fmt.Printf("[discovery-import] relaying %s -> %s: %v\n", msg.Topic(), localStateTopic, err)
					}
				}
			}
		}

		hostsStateFilter := "hosts/+/" + remoteInstallation + "/+/state"
		token = cloudClient.Subscribe(hostsStateFilter, 0, hostsStateHandler)
		if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
			if err := token.Error(); err != nil {
				return fmt.Errorf("subscribing to %s: %w", hostsStateFilter, err)
			}
			return fmt.Errorf("subscribing to %s: timed out", hostsStateFilter)
		}
	}

	return nil
}
