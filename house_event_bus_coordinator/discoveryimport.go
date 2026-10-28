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
 *   - "hosts"-kind topics are bare (e.g. "hosts/<host>/cpu/state") and used as-is -- retired
 *     2026-09-08's installation-qualified segment (mqtt_relay.go's own former qualifyHostsTopic),
 *     since every cloud-reporting host now has a globally unique hostname across the whole fleet,
 *     removing the need to disambiguate installations on the shared cloud broker at all.
 *     "home_assistant"-kind topics were always self-qualifying regardless
 *     ("homeassistant_instances/<instance>/bridge/<entity>/state" -- the instance name is baked in
 *     at generation time on the exporting side) and are unaffected by this change.
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

	"homeassistant/hamqtt"

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
// Domain/DerivedFromCapability/DerivedViaTemplate (added 2026-09-10, plans/
// derived-capability-mechanism.md Phase 2) are only ever set for a "derived DDD.NNN from
// EEE.MMM via TTT;" capability (homeassistant/Physical_DerivedCapability.go) -- passed through
// verbatim from Physical.def by integration_import_generator.go, never resolved there (unlike an
// ordinary imported capability, a derived one has no exporter payload to match against at all, so
// this side has to resolve its own discovery config -- resolveImportedDerivedCapability walks the
// same-device derivation chain these three fields describe). All three empty for an ordinary
// imported capability -- unchanged behaviour from before this feature existed.
type TImportedCapability struct {
	LocalEntity           string `yaml:"local_entity"`
	Domain                string `yaml:"domain"`
	DerivedFromCapability string `yaml:"derived_from"`
	DerivedViaTemplate    string `yaml:"derived_via"`
	// StableID is the capability's frozen universal ID (PROJECT.md item 48, Physical.def's
	// "capability ... as <id>;"), its unique_id -- "" until assigned, falling back to the
	// name-based importedUniqueID formula.
	StableID string `yaml:"stable_id"`
	// RemoteID (PROJECT.md 22, 2026-10-28) is the exporter's persistent identity for this
	// capability (Physical.def's "capability ... from <remote-id>"), matched against a cloud
	// config's "export_id" -- so renaming the exporter's device or capability doesn't break the
	// import. Empty: matched by the name-based exportStableID, as before.
	RemoteID string `yaml:"remote_id,omitempty"`
}

// TImportedDevice is one declared import from coordinator/imported.yaml.
// DisplayName is THIS house's own space-positioning-derived name for the device
// (deviceDisplayName, generator-side) -- never the exporting installation's own name for it, same
// principle as every other device kind's own "device:" block name.
// ConstantAttributes (added 2026-09-08) only ever carries "suggested_area" today -- a purely
// local Spaces.def positioning concept (which area THIS house's own "as area" space put the
// device under), never something the exporting installation should dictate. Unlike
// manufacturer/model/..., which genuinely belong to the remote device and are instead learned
// live from the exporter's own cloud-crossed discovery config (buildImportedDiscoveryBody's own
// payload.Device fields) -- reuses the same shared TConceptualConstant shape every other device
// kind's own constant attributes already use, not a hassbridge-specific concept.
type TImportedDevice struct {
	RemoteInstallation string                         `yaml:"remote_installation"`
	RemoteDeviceID     string                         `yaml:"remote_device_id"`
	DisplayName        string                         `yaml:"display_name"`
	Capabilities       map[string]TImportedCapability `yaml:"capabilities"`
	ConstantAttributes map[string]TConceptualConstant `yaml:"constant_attributes"`
	// DependsOnAvailability (2026-09-16, PROJECT.md's logical-layer work) is every extra raw MQTT
	// topic this device's own capabilities' availability must additionally AND in, resolved
	// generator-side from a Logical.def "dependency on <other-id>;" declaration sharing this same
	// device's own id (homeassistant/integration_logical_storage.go's
	// resolveDependencyAvailabilityTopics) -- already flattened across the full transitive
	// dependency chain and cycle-checked there, so this coordinator never walks that graph itself.
	// Passed straight through to buildAvailabilityFields alongside the device's own node topic.
	// Empty for a device with no matching Logical.def declaration -- unchanged behaviour from
	// before this field existed.
	DependsOnAvailability []string `yaml:"depends_on_availability"`
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

func importedStateTopic(localEntity string) string {
	return "imports/" + localEntity + "/state"
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
	// ExportID: the exporter's persistent identity for the capability (PROJECT.md 22).
	ExportID        string `json:"export_id"`
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
		// ViaDevice (added 2026-09-14, PROJECT.md item 3/plans/via-device-inference.md) is the
		// EXPORTING house's own local DeviceID for whatever device the exported device is
		// "connected via" -- discoveryhassbridge.go's buildHassBridgeDeviceBlock already sets this
		// standard HA field on the exporter's own local discovery config (Phase 1), and
		// crossPostHassBridgeToCloud forwards the same JSON bytes verbatim, so it just arrives
		// here for free. Expressed in the EXPORTER's own device-id namespace, not this house's --
		// buildImportedDiscoveryBody must translate it via importViaDeviceIndex before using it.
		ViaDevice string `json:"via_device"`
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
	StableID      string
	DisplayName   string
	// SuggestedArea is TImportedDevice.ConstantAttributes' own "suggested_area" value, if declared
	// -- see TImportedDevice's own doc comment for why this is the one constant attribute an
	// import carries locally, rather than learning it from the exporter like manufacturer/model.
	SuggestedArea string
	// ExistenceKey: importExistenceKey's, for the import existence tracker.
	ExistenceKey string
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
//
// A capability with a RemoteID (PROJECT.md 22, 2026-10-28) matches on the payload's "export_id"
// instead: the exporter's persistent identity, which a rename of its device or capability doesn't
// change (the topic's segment does).
func matchImportedCapabilityByStableID(importFile TImportedFile, remoteInstallation, topic, exportID string) (importCapabilityMatch, bool) {
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
			if cap.RemoteID != "" {
				if cap.RemoteID != exportID {
					continue
				}
			} else if exportStableID(device.RemoteInstallation, device.RemoteDeviceID, capability) != stableID {
				continue
			}
			return importCapabilityMatch{LocalDeviceID: localDeviceID, Capability: capability, LocalEntity: cap.LocalEntity, StableID: cap.StableID, DisplayName: device.DisplayName, SuggestedArea: device.ConstantAttributes["suggested_area"].Value, ExistenceKey: importExistenceKey(device, capability)}, true
		}
	}
	return importCapabilityMatch{}, false
}

// importExistenceKey is how the import existence tracker knows one imported capability: its
// RemoteID when it has one, else the name-based exportStableID.
func importExistenceKey(device TImportedDevice, capability string) string {
	if remoteID := device.Capabilities[capability].RemoteID; remoteID != "" {
		return remoteID
	}
	return exportStableID(device.RemoteInstallation, device.RemoteDeviceID, capability)
}

// importRemoteDeviceKey identifies one remote device by (exporting installation, that
// installation's own local DeviceID for it) -- the same pair TImportedDevice's own
// RemoteInstallation/RemoteDeviceID fields already carry, exactly what a "device import.X from
// <house> <remote-id> with:" Physical.def declaration states. Added 2026-09-14 (PROJECT.md item
// 3/plans/via-device-inference.md).
type importRemoteDeviceKey struct {
	RemoteInstallation string
	RemoteDeviceID     string
}

// buildImportViaDeviceIndex builds a (RemoteInstallation, RemoteDeviceID) -> local DeviceID index
// from every device importFile already declares -- pure local data, fully known from Physical.def
// alone, no coordinator runtime state needed. buildImportedDiscoveryBody uses it to translate an
// exporter's own via_device (expressed in ITS device-id namespace) into whichever of this house's
// own imports, if any, corresponds to that same remote device.
func buildImportViaDeviceIndex(importFile TImportedFile) map[importRemoteDeviceKey]string {
	index := make(map[importRemoteDeviceKey]string, len(importFile.Devices))
	for localDeviceID, device := range importFile.Devices {
		if device.RemoteInstallation == "" || device.RemoteDeviceID == "" {
			continue
		}
		index[importRemoteDeviceKey{RemoteInstallation: device.RemoteInstallation, RemoteDeviceID: device.RemoteDeviceID}] = localDeviceID
	}
	return index
}

// importedRemote is what payload -- the exporting house's cloud catalogue entry -- says about the
// entity, its via_device resolved to the local device importing that one.
func importedRemote(payload importedDiscoveryPayload, remoteInstallation string, viaDeviceIndex map[importRemoteDeviceKey]string) hamqtt.ImportedRemote {
	remote := hamqtt.ImportedRemote{
		Manufacturer: payload.Device.Manufacturer, Model: payload.Device.Model, ModelID: payload.Device.ModelID,
		HwVersion: payload.Device.HwVersion, SwVersion: payload.Device.SwVersion, SerialNumber: payload.Device.SerialNumber,
		DeviceClass: payload.DeviceClass, Unit: payload.Unit, StateClass: payload.StateClass, Icon: payload.Icon,
		PayloadOn: payload.PayloadOn, PayloadOff: payload.PayloadOff,
		ExportID: payload.ExportID,
	}
	if len(payload.Device.Identifiers) > 0 {
		remote.RemoteDeviceID = payload.Device.Identifiers[0]
	}
	if payload.Device.ViaDevice != "" {
		remote.ViaDevice = viaDeviceIndex[importRemoteDeviceKey{RemoteInstallation: remoteInstallation, RemoteDeviceID: payload.Device.ViaDevice}]
	}
	return remote
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

// TImportedHostsRelayIndex maps a "hosts"-kind capability's bare cloud source topic
// (e.g. "hosts/mqtt/cpu/state") to every locally-declared
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
	viaDeviceIndex := buildImportViaDeviceIndex(importFile)

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
			match, matched := matchImportedCapabilityByStableID(importFile, remoteInstallation, msg.Topic(), payload.ExportID)
			if !matched {
				return
			}
			if existenceTracker != nil {
				if existenceTracker.MarkKnown(remoteInstallation, match.ExistenceKey) {
					fmt.Printf("[import-existence] %s: %s -> known-to-exist\n", remoteInstallation, match.ExistenceKey)
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
				// A "hosts"-kind capability's raw topic is bare and published as-is on the cloud
				// broker too (retired 2026-09-08's installation-qualified segment -- every
				// cloud-reporting host has a globally unique hostname across the whole fleet
				// instead, so no two remote installations can ever collide on the same bare
				// topic). Learn it, and, if this attribute shares a JSON-blob topic with others
				// (value_template set), which field to pull out of it, so the state relay below
				// (registered once, not per capability) knows what to do with a message on it.
				hostsRelayIndex.register(payload.StateTopic, TImportedHostsRelay{LocalEntity: match.LocalEntity, ExtractionField: extractionField})
			case payload.StateTopic != "":
				// A "home_assistant"-kind capability's own state topic is already self-qualifying
				// (unlike "hosts"-kind's bare one above) -- registering it here lets stateHandler
				// relay it the same lazy, topic-learned way, the only mechanism that ever learns
				// this mapping at all, since a declaration never states the remote's own local
				// entity name up front.
				hostsRelayIndex.register(payload.StateTopic, TImportedHostsRelay{LocalEntity: match.LocalEntity, ExtractionField: extractionField})
			}

			if importCatalogue.Record(match.LocalEntity, importedRemote(payload, remoteInstallation, viaDeviceIndex)) {
				importCatalogue.SchedulePublish(mainClient, cloudClient, ownInstallation)
			}
			// Home Assistant gets the entity from the generator's YAML (PROJECT.md 19h.4), built
			// from what the catalogue entry recorded above says.
		}

		configFilter := remoteInstallation + "/" + conceptualPrefix + "/+/+/+/config"
		token := subscribe(cloudClient, configFilter, 0, configHandler)
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
		token = subscribe(cloudClient, stateFilter, 0, stateHandler)
		if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
			if err := token.Error(); err != nil {
				return fmt.Errorf("subscribing to %s: %w", stateFilter, err)
			}
			return fmt.Errorf("subscribing to %s: timed out", stateFilter)
		}

	}

	// "hosts"-kind's own bare shape: "hosts/<host>/<suffix>/state" (retired 2026-09-08's
	// installation-qualified segment -- every cloud-reporting host has a globally unique hostname
	// across the whole fleet instead, so this one wildcard is shared across every remote
	// installation rather than needing its own per-installation subscription). hostsRelayIndex is
	// populated lazily by each remote installation's own configHandler above as its capabilities'
	// discovery configs arrive, so a message on a topic nothing has registered yet is simply a
	// safe no-op (lookup returns nothing to relay).
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

	hostsStateFilter := "hosts/+/+/state"
	token := subscribe(cloudClient, hostsStateFilter, 0, hostsStateHandler)
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		if err := token.Error(); err != nil {
			return fmt.Errorf("subscribing to %s: %w", hostsStateFilter, err)
		}
		return fmt.Errorf("subscribing to %s: timed out", hostsStateFilter)
	}

	return nil
}
