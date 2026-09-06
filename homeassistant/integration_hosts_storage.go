/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationHostsStorage
 *
 * The data model and analysis helpers for the "hosts" integration's parsed devices:
 * THostDevice itself, plus the collection-level operations (deduplication, duplicate
 * detection) that generation (integration_hosts_generator.go) relies on. Parsing
 * (integration_hosts_parser.go) produces THostDevice values; this file doesn't parse or
 * generate anything itself.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 19.08.2026
 *
 */

package main

import (
	"fmt"
	"sort"
	"strings"
)

// THostConstantAttribute is one per-device override for a constant HA discovery device-map
// field (e.g. "model", "hw_version"), declared as `<field>: "<value>" [forced];` inside a
// device's "with:" block. Forced means the value must never be overwritten by live data the
// device itself later reports (once that mechanism exists -- see Architecture.md §13) --
// distinct from a bare default, which a future live-report merge is free to replace.
type THostConstantAttribute struct {
	Value  string
	Forced bool
}

// knownConstantDeviceAttributes are the HA MQTT discovery "device:" map field names this
// generator understands (see e.g. https://www.home-assistant.io/integrations/binary_sensor.mqtt/,
// "device (map)"). Deliberately excludes identifiers/connections/name/via_device -- those are
// structural (computed, not simple per-device string overrides).
var knownConstantDeviceAttributes = map[string]bool{
	"manufacturer":      true,
	"model":             true,
	"model_id":          true,
	"hw_version":        true,
	"sw_version":        true,
	"serial_number":     true,
	"configuration_url": true,
	"suggested_area":    true,
}

// THostDevice is one "device <id> <host> <type> [with: <capability>: [\"<literal>\"]
// <entity>[!<attribute>]; <constant device attribute>: "<value>" [forced]; ... end;]"
// declaration from the "integration hosts" block.
type THostDevice struct {
	DeviceID        string
	HostName        string
	IntegrationType string            // "home_assistant", "cpu", or "ping"
	Capabilities    map[string]string // capability name -> Home Assistant entity reference (home_assistant type only), e.g. "sensor.processor_use" or "update.home_assistant_operating_system_update!installed_version" (the same "<entity>!<attribute>" convention Spaces.def entity bodies use, e.g. "value sensor.X!state_message;"). A name may carry an explicit "<group>/<leaf>" prefix (e.g. "cpu/load") marking it as a variable attribute in that group -- see splitCapabilityName/AttributeNames. A name with no group prefix is a device-info capability instead (feeds the discovery "device:" block only, never gets its own sensor entity).
	// CapabilityLiteralPrefixes holds an optional literal string prefix for a capability,
	// declared as `<capability>: "<literal>" <entity>[!<attribute>];` -- e.g. `sw_version:
	// "Home Assistant Operating System " update.home_assistant_operating_system_update!installed_version;`
	// renders as 'Home Assistant Operating System ' ~ state_attr(...) in the generated Jinja
	// (generateReportingAutomations). Absent for a capability with no literal prefix.
	CapabilityLiteralPrefixes map[string]string
	ConstantAttributes        map[string]THostConstantAttribute // per-device overrides for HA device-map fields, e.g. "model" -> {"Cool raspi", false}
	// Cloud is the "cloud" routing keyword (parseRoutingKeywords) trailing a device declaration --
	// see its own doc comment for the full semantics. Means this device communicates
	// (reports/is discovered) via the house's cloud broker profiles ("cloud_client"/
	// "cloud_coordinator" in Physical.def) instead of "main". Not set (the default) means this
	// device is purely local, today's existing behaviour.
	Cloud bool
	// selfImport is parseRoutingKeywords' "import" keyword, parsed alongside Cloud but NOT
	// resolved to ImportedFrom here -- integration_hosts_parser.go has no installation name
	// available to resolve it against. Unexported: pure parse-to-resolve plumbing, never
	// serialized into devices.yaml or read by anything outside this package. Both places that
	// construct a THostDevice from "integration hosts" (collectHostsDevicesByID below,
	// generateHostsIntegrationOutputs/integration_hosts_generator.go) already have -- or cheaply
	// resolve -- this installation's own name, and must immediately turn selfImport into
	// ImportedFrom set to that name right after parsing, mirroring exactly what an ordinary
	// "integration import with: device <id> <this-installation> <host> <type>;" declaration
	// would have produced, had the DSL author written it out by hand pointing at themselves.
	selfImport bool
	// ImportedFrom is the remote installation name (e.g. "junglinster") this device was declared
	// via "integration import with: device <local-id> <remote-installation> <remote-host>
	// <type>; ...;" (integration_import_parser.go) instead of "integration hosts", OR the
	// resolved self-import case above (this installation's own name, from a "cloud import"
	// declaration in "integration hosts") -- "" for a device that's neither. HostName holds the
	// *remote* host name for a real cross-house import, reused as-is for local topic construction
	// too (see mqtt_relay.go, coordinator side) -- no separate "local alias" concept, to keep this
	// from growing yet another name to track.
	ImportedFrom string
}

// dedupedHostNames returns every device's HostName, deduplicated and sorted, regardless
// of integration type -- home_assistant, cpu, and ping devices are all pingable hosts.
func dedupedHostNames(devices []THostDevice) []string {
	seen := map[string]bool{}
	var names []string
	for _, d := range devices {
		if d.HostName == "" || seen[d.HostName] {
			continue
		}
		seen[d.HostName] = true
		names = append(names, d.HostName)
	}
	sort.Strings(names)
	return names
}

// collectHostsDevicesByID re-parses Physical.def's "hosts" integration block and returns
// every declared device keyed by DeviceID, for callers that need device lookup outside the
// Physical.def-native generation pipeline (Spaces.def parsing via
// Conceptual_DeviceEntities.go, presence checks). First declaration wins for a duplicated id
// (matching warnDuplicateDeviceIDs' policy), but this helper doesn't itself warn -- callers
// that care about duplicates already get that from generateHostsIntegrationOutputs' own pass.
// Resolves each device's own selfImport flag (parseRoutingKeywords' "import" keyword) into
// ImportedFrom set to this installation's own resolved name, mirroring generateHostsIntegrationOutputs'
// own resolution -- see THostDevice.selfImport's doc comment for why this has to happen here
// rather than inside the lower-level parser.
func collectHostsDevicesByID(definitionDir string) (map[string]THostDevice, []string) {
	physicalContent, mergedLineNos, warnings := collectLayerContent(definitionDir, []string{"Physical.def"}, LayerPhysical)
	blocks, blockWarnings := parseIntegrationBlocks(physicalContent, mergedLineNos)
	warnings = append(warnings, blockWarnings...)
	installation := resolveInstallationName(definitionDir)

	byID := map[string]THostDevice{}
	for _, block := range blocks {
		if block.Name != "hosts" {
			continue
		}
		devices, bodyWarnings := parseHostsIntegrationBody(block.BodyLines)
		warnings = append(warnings, bodyWarnings...)
		for _, d := range devices {
			// "import" without "cloud" is meaningless (generateHostsIntegrationOutputs warns
			// about it once; not repeated here to avoid double warnings for the same file).
			if d.selfImport && d.Cloud {
				if installation == "" {
					warnings = append(warnings, fmt.Sprintf("device %q: \"import\" declared but ${installation} is not set; cannot self-qualify -- ignored", d.DeviceID))
				} else {
					d.ImportedFrom = installation
				}
			}
			if _, exists := byID[d.DeviceID]; !exists {
				byID[d.DeviceID] = d
			}
		}
	}
	return byID, warnings
}

// homeAssistantCapabilityEntityIDs returns every entity ID referenced by a
// home_assistant-type device's capabilities (e.g. sensor.processor_use for the "load"
// capability), mapped to a "<device id>/<capability>" label. These entities aren't declared
// in Spaces.def -- there's no space/entity linkage for physical-layer devices yet -- so
// callers checking assumed-from-HA entities (presence.go) need to pull them in separately.
// A capability value may carry a "!<attribute>" suffix (see THostDevice.Capabilities) -- that
// part is stripped before checking presence, since it's the *entity* that must exist on the
// instance, not a literal "id!attribute" string, which would never match a real entity id.
//
// Assumes every home_assistant-type device lives on "the main instance" -- true today
// since Physical.def can only declare one home_assistant target per house. Once a second,
// named HA instance can be declared (e.g. the protocols-server-2 integration-adapter role,
// Architecture.md §7/§9.2), this needs to become per-instance, grouping devices by which
// instance hosts them and checking each independently.
func homeAssistantCapabilityEntityIDs(definitionDir string) map[string]string {
	devicesByID, _ := collectHostsDevicesByID(definitionDir)

	ids := map[string]string{}
	for _, d := range devicesByID {
		if d.IntegrationType != "home_assistant" {
			continue
		}
		for capability, entityID := range d.Capabilities {
			if bangIdx := strings.Index(entityID, "!"); bangIdx > 0 {
				entityID = entityID[:bangIdx]
			}
			ids[entityID] = d.DeviceID + "/" + capability
		}
	}
	return ids
}

// THostsAttributeSpec is the fixed HA typing metadata for one variable attribute -- what kind
// of sensor it is, per the same "the integration should provide/determine the entity type and
// unit" principle AttributeSpecs itself already follows. Empty fields are omitted from
// generated output/discovery payloads (HA treats an absent device_class/unit/state_class as
// "generic numeric sensor," which is correct for attributes with no more specific type, e.g.
// "load").
type THostsAttributeSpec struct {
	DeviceClass string // HA device_class, e.g. "temperature" -- "" if none applies
	Unit        string // HA unit_of_measurement, e.g. "°C" -- "" if unitless
	StateClass  string // HA state_class, e.g. "measurement"
	Icon        string // HA icon override, e.g. "mdi:cpu-64-bit" -- "" to let device_class pick HA's default
}

// THostsEntityMaterialization describes how one "hosts" integration device type's
// "entity device.<spec> from <device-id> ...;" declaration (Conceptual_DeviceEntities.go)
// materializes into concrete HA entities: the node (availability) entity's domain and path
// suffix, the attribute entities' domain/path suffix/per-attribute typing, and default
// constant device info (Manufacturer/Model -- "defaults for" real hardware metadata that
// isn't derivable from anything reported today, per the user's own framing; deliberately not
// per-device, just per integration type). This is deliberately "hosts" integration-owned
// knowledge rather than something the generic conceptual-layer code decides: a future
// integration (e.g. a weather device) would materialize its own device.<spec> declarations
// differently -- perhaps as a sensor with no "/node" concept at all -- and would own that
// decision in its own storage file the same way this one does.
type THostsEntityMaterialization struct {
	NodeDomain      string // e.g. "binary_sensor"
	NodeSuffix      string // e.g. "node" -- appended as ".../<device path>/<suffix>"
	AttributeDomain string // e.g. "sensor" -- empty if this type has no variable attributes
	AttributeSuffix string // e.g. "cpu" -- appended as ".../<device path>/<suffix>/<attribute name>"
	AttributeSpecs  map[string]THostsAttributeSpec

	// StateTopicTemplate is the MQTT topic variable-attribute values are reported on, as a
	// fmt template taking the device's HostName (e.g. "hosts/%s/cpu/state"), or "" if this
	// type reports no state topic (e.g. ping, liveness-only). This is the single source of
	// truth for the topic convention: both generateReportingAutomations (the HA-side publisher
	// for home_assistant-type devices) and generateCoordinatorDevicesFile (so the coordinator
	// doesn't have to independently guess or hardcode it) read it from here, rather than each
	// hardcoding the topic themselves. The deployed Integrations/cpu/report script implements
	// this same convention independently, outside this repo -- if this template changes, that
	// script needs updating to match (the user's own responsibility, not generated).
	StateTopicTemplate string

	// NodeTopicTemplate is the MQTT topic this type's liveness/availability is reported on, as
	// a fmt template taking the device's HostName (e.g. "hosts/%s/node/state"). Every hosts
	// device is ping-checked, so this is set for all three known types. The deployed
	// Integrations/ping/report script implements this convention independently, outside this
	// repo.
	NodeTopicTemplate string

	// DeviceInfoTopicTemplate is the MQTT topic live-reported device metadata (manufacturer,
	// model, hw_version, sw_version, serial_number, model_id -- ungrouped capability names,
	// see splitCapabilityName) is posted on, as a fmt template taking the device's HostName
	// (e.g. "hosts/%s/device/state").
	// Set for "cpu" and "home_assistant" only -- "ping" devices have no live-reporting
	// mechanism. The deployed Integrations/cpu/report script implements this convention
	// independently, outside this repo, same caveat as StateTopicTemplate.
	DeviceInfoTopicTemplate string

	// DefaultConstantAttributes are integration-type-level defaults for HA discovery
	// device-map fields (knownConstantDeviceAttributes), overridable per device via a
	// device's own "with:" block (THostDevice.ConstantAttributes -- see mergedConstantAttributes).
	// Only "model" is set anywhere today, as a coarse, honest category ("what kind of thing is
	// this"), not a real product model -- still useful in HA's device list, still clearly a
	// default. No manufacturer default anywhere -- no real manufacturer data exists for any of
	// these device types, and a fabricated value would be misleading rather than helpful.
	DefaultConstantAttributes map[string]string
}

// StateTopic resolves m's StateTopicTemplate for a specific host, or "" if this
// materialization has none.
func (m THostsEntityMaterialization) StateTopic(hostName string) string {
	if m.StateTopicTemplate == "" {
		return ""
	}
	return fmt.Sprintf(m.StateTopicTemplate, hostName)
}

// NodeTopic resolves m's NodeTopicTemplate for a specific host, or "" if this materialization
// has none.
func (m THostsEntityMaterialization) NodeTopic(hostName string) string {
	if m.NodeTopicTemplate == "" {
		return ""
	}
	return fmt.Sprintf(m.NodeTopicTemplate, hostName)
}

// DeviceInfoTopic resolves m's DeviceInfoTopicTemplate for a specific host, or "" if this
// materialization has none.
func (m THostsEntityMaterialization) DeviceInfoTopic(hostName string) string {
	if m.DeviceInfoTopicTemplate == "" {
		return ""
	}
	return fmt.Sprintf(m.DeviceInfoTopicTemplate, hostName)
}

// AttributeNames returns the variable-attribute names device implies, each in full
// "<group>/<leaf>" form (e.g. "cpu/load") -- consistent with how a "home_assistant" bridge
// device's own capabilities are named (Physical.def's "sensor.cpu/load: ...;" lines), so a
// Spaces.def author references a device's attributes the same way regardless of which
// integration kind it belongs to (PROJECT.md, 2026-09-01: was inconsistent -- hosts devices
// used to expose bare leaf names like "load" here, home_assistant bridge devices always used
// the full "cpu/load" form). Two sources: m's own hardwired AttributeSpecs names if m has any
// (e.g. "cpu" type, where every device of that type reports the same names regardless of what
// it declares) prefixed with m.AttributeSuffix, otherwise device's own declared Capabilities
// names that already carry an explicit "<group>/<leaf>" prefix (e.g. "cpu/load") -- see
// splitCapabilityName. A capability with no group prefix is a device-info capability, not a
// variable attribute (generateReportingAutomations posts it to DeviceInfoTopic instead, the
// coordinator's TLiveDeviceInfoStore merges it into ConstantAttributes), and must NOT get a
// standalone sensor entity -- omitting the group is exactly what excludes it here. Sorted for
// deterministic output ordering.
func (m THostsEntityMaterialization) AttributeNames(device THostDevice) []string {
	if len(m.AttributeSpecs) > 0 {
		names := make([]string, 0, len(m.AttributeSpecs))
		for name := range m.AttributeSpecs {
			if m.AttributeSuffix != "" {
				name = m.AttributeSuffix + "/" + name
			}
			names = append(names, name)
		}
		sort.Strings(names)
		return names
	}
	names := make([]string, 0, len(device.Capabilities))
	for declared := range device.Capabilities {
		group, _ := splitCapabilityName(declared)
		if group == "" {
			continue
		}
		names = append(names, declared)
	}
	sort.Strings(names)
	return names
}

// splitCapabilityName splits a declared capability name into its optional group prefix and
// leaf name -- "cpu/load" -> ("cpu", "load"); "load" (no group) -> ("", "load"). The group,
// when present, is this attribute's path-suffix segment (in place of the integration type's
// own hardwired AttributeSuffix, e.g. "cpu" for the "cpu" integration type -- see
// registerHostAttributeEntity, Conceptual_DeviceEntities.go) and marks it as a variable
// attribute rather than a device-info capability (see AttributeNames).
func splitCapabilityName(declared string) (group, leaf string) {
	if idx := strings.Index(declared, "/"); idx >= 0 {
		return declared[:idx], declared[idx+1:]
	}
	return "", declared
}

// AttributeSpec resolves attr's HA typing metadata: attr is the full "<group>/<leaf>" name
// AttributeNames returns (e.g. "cpu/load") -- only the leaf half (splitCapabilityName) is ever
// used to key the typing tables, since cpuAttributeSpecs/m.AttributeSpecs are keyed by leaf name
// alone (they predate the group-prefixed DSL-facing convention and have no need to duplicate
// it). Tries m's own AttributeSpecs entry first, else cpuAttributeSpecs -- the latter covers
// "load"/"temperature" for home_assistant-type devices, whose Capabilities names aren't
// hardwired in any THostsEntityMaterialization's own AttributeSpecs. A name neither table knows
// about resolves to the zero value (HA treats an absent device_class/unit/state_class as a
// generic numeric sensor).
func (m THostsEntityMaterialization) AttributeSpec(attr string) THostsAttributeSpec {
	_, leaf := splitCapabilityName(attr)
	if spec, ok := m.AttributeSpecs[leaf]; ok {
		return spec
	}
	return cpuAttributeSpecs[leaf]
}

// hostsEntityMaterializationByType maps a "hosts" integration device's IntegrationType to how
// it materializes. All three known types currently share the same node shape (every hosts
// device is liveness-checked via ping, so "binary_sensor.../node" applies uniformly, and all
// three get a NodeTopicTemplate) -- only the variable attributes and state topic differ. "cpu"
// and "home_assistant" both report on the same hosts/<host>/cpu/state MQTT topic family and
// share the same AttributeDomain/AttributeSuffix ("sensor"/"cpu"), so both name their attribute
// entities identically; "cpu" always reports load+temperature there (AttributeSpecs hardwires
// both names for every device of that type), while home_assistant-type devices leave
// AttributeSpecs unset -- their variable attributes are already explicit via Capabilities, a
// differently shaped (entity-mapped, not name-only) mechanism, so AttributeNames
// (integration_hosts_storage.go, below) falls back to each device's own Capabilities names, typed
// via cpuAttributeSpecs, when AttributeSpecs is empty.
// cpuAttributeSpecs is the HA typing metadata for the "cpu"-topic-family attribute names
// ("load", "temperature"), shared between "cpu" type (where AttributeSpecs hardwires that
// every device of that type reports both) and "home_assistant" type (where a device's own
// Capabilities names, not the type, decide which of these apply -- see AttributeNames's own
// fallback below). Both types name
// the resulting entities identically (AttributeSuffix "cpu"), since both report on the same
// hosts/<host>/cpu/state topic family and a "load"/"temperature" reading means the same thing
// regardless of which mechanism populated it.
var cpuAttributeSpecs = map[string]THostsAttributeSpec{
	// mdi:cpu-64-bit matches the icon HA's own system_monitor "Processor use" sensor uses --
	// kept consistent since host.junglinster's "load" capability maps directly to that sensor.
	"load":        {StateClass: "measurement", Unit: "%", Icon: "mdi:cpu-64-bit"},
	"temperature": {DeviceClass: "temperature", Unit: "°C", StateClass: "measurement"},
}

var hostsEntityMaterializationByType = map[string]THostsEntityMaterialization{
	"cpu": {
		NodeDomain: "binary_sensor", NodeSuffix: "node",
		AttributeDomain: "sensor", AttributeSuffix: "cpu",
		AttributeSpecs:            cpuAttributeSpecs,
		StateTopicTemplate:        "hosts/%s/cpu/state",
		NodeTopicTemplate:         "hosts/%s/node/state",
		DeviceInfoTopicTemplate:   "hosts/%s/device/state",
		DefaultConstantAttributes: map[string]string{"model": "Compute host"},
	},
	"ping": {
		NodeDomain:                "binary_sensor",
		NodeSuffix:                "node",
		NodeTopicTemplate:         "hosts/%s/node/state",
		DefaultConstantAttributes: map[string]string{"model": "Network host"},
	},
	"home_assistant": {
		NodeDomain: "binary_sensor", NodeSuffix: "node",
		// AttributeDomain/AttributeSuffix match "cpu" type (same topic family, same naming)
		// even though AttributeSpecs is deliberately left unset here -- which attribute names
		// apply is per-device (THostDevice.Capabilities), not hardwired per type.
		AttributeDomain:           "sensor",
		AttributeSuffix:           "cpu",
		StateTopicTemplate:        "hosts/%s/cpu/state",
		NodeTopicTemplate:         "hosts/%s/node/state",
		DeviceInfoTopicTemplate:   "hosts/%s/device/state",
		DefaultConstantAttributes: map[string]string{"model": "Home Assistant instance"},
	},
}

// mergedConstantAttributes combines mat's integration-type-level defaults with device's own
// per-device overrides (device wins on a shared key, keeping its own Forced flag -- a type
// default is never "forced," it's just a fallback). Warns (via the returned warnings) about
// any device-declared attribute name not in knownConstantDeviceAttributes, since an unknown
// name is almost certainly a typo -- HA would simply ignore it, silently, if published as-is.
func mergedConstantAttributes(mat THostsEntityMaterialization, device THostDevice) (map[string]THostConstantAttribute, []string) {
	merged := map[string]THostConstantAttribute{}
	for name, value := range mat.DefaultConstantAttributes {
		merged[name] = THostConstantAttribute{Value: value}
	}
	var warnings []string
	for name, attr := range device.ConstantAttributes {
		if !knownConstantDeviceAttributes[name] {
			warnings = append(warnings, fmt.Sprintf("device %q: unrecognised constant device attribute %q; ignored", device.DeviceID, name))
			continue
		}
		merged[name] = attr
	}
	return merged, warnings
}

// MaterializationForIntegrationType returns how a "hosts" integration device type's
// device.<spec> declaration materializes, or false if the type is unrecognised -- callers
// should treat that as "nothing to register," not an error.
func MaterializationForIntegrationType(integrationType string) (THostsEntityMaterialization, bool) {
	m, ok := hostsEntityMaterializationByType[integrationType]
	return m, ok
}

// warnDuplicateDeviceIDs reports device ids declared more than once (e.g. copy-paste
// leftovers when Physical.def's device list was assembled from Integrations.def's
// earlier draft) -- the first declaration wins for the coordinator devices file, but the
// duplication itself is worth surfacing rather than silently resolving.
func warnDuplicateDeviceIDs(devices []THostDevice) []string {
	seen := map[string]bool{}
	var warnings []string
	for _, d := range devices {
		if seen[d.DeviceID] {
			warnings = append(warnings, fmt.Sprintf("device id %q is declared more than once; keeping the first declaration", d.DeviceID))
			continue
		}
		seen[d.DeviceID] = true
	}
	return warnings
}
