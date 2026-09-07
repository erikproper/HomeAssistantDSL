/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationHassBridgeStorage
 *
 * The data model for the "home_assistant" integration's parsed bridge devices: entities that
 * already exist as full HA entities on a *named remote instance* (protocols-server-2's printer,
 * weather, sun, ...), bridged into the main instance's conceptual layer -- distinct from, and
 * unrelated to, the pre-existing same-spelled "home_assistant" *device type* inside "integration
 * hosts with: device <id> <host> home_assistant with: ...;" (a local host whose capabilities
 * reference entities already on the main instance, no bridging involved).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 23.08.2026
 *
 */

package main

import (
	"sort"
	"strings"
)

// THassBridgeCapability is one declared entity a "home_assistant" bridge device exposes -- an
// EXPLICIT domain (mandatory, never inferred/defaulted -- per the unified device-grammar design,
// Architecture.md/the "intensional device→space linkage" plan's §D) plus each declaring instance's
// own raw entity reference ("<entity>" or "<entity>!<attribute>", the same convention hosts
// capabilities use). The declared Domain need not match a Source's own HA domain -- a deliberate
// *coercion* (e.g. Domain "binary_sensor" with a Source that's a "switch.*" entity, since their
// on/off state values are compatible) -- the DSL author's responsibility to only coerce between
// genuinely compatible domains; not validated here (no compatibility table defined yet).
type THassBridgeCapability struct {
	Domain string
	// Sources holds this capability's remote source entity, keyed by the declaring instance's own
	// name -- genuinely per-instance, not a single shared value: the same physical roaming device
	// can register under a DIFFERENT local entity_id on each HA instance it connects to (real
	// incident, found live 2026-09-05 -- Vienna's own registry initially lacked
	// hass.eriks_iphone's battery_level entity under the identical name Junglinster's instances
	// used, until manually aligned there; before this fix, collectHassBridgeDevicesByID's roaming
	// merge silently discarded every instance's own declaration but the first, so every instance's
	// reporting automation was generated against whichever source happened to win the merge). A
	// non-roaming device still populates this with exactly one entry (its own sole instance) --
	// every reader must key off the specific instance it cares about, never assume a shared value.
	Sources map[string]string
	// Typing metadata, all optional -- declared via a separate "<path> unit: "<value>";"-shaped
	// line per field (integration_hassbridge_parser.go's capabilityMetadataPattern), not
	// hardwired per integration type the way hosts' cpuAttributeSpecs is: a hassbridge
	// capability's path is a flat string ("cpu/load"), not a hosts-style bare attribute name
	// ("load") a shared table could key off, so the DSL author states it explicitly instead.
	Unit        string
	Icon        string
	DeviceClass string
	StateClass  string
}

// THassBridgeDevice is one "device <id> with: ... end;" declaration inside Physical.def's
// "integration home_assistant <qualifier> with: ... end;" block. Capabilities (domain-prefixed
// lines, "<entity_type>.<path>: <source>;") is keyed by the device-relative path ("status",
// "node", "cpu/load", ...) -- every one becomes a variable attribute entity; there's no
// group-prefix/device-info split the way "hosts" capabilities have, since domain is always
// explicit here (see THassBridgeCapability). Bare (non-domain-prefixed) lines are metadata
// instead, split the same way "hosts" devices already split theirs:
// ConstantAttributes for a purely static `<name>: "<value>" [forced];` (no live source, reuses
// hosts' own THostConstantAttribute), DeviceInfoCapabilities for a dynamic
// `<name>: ["<literal>"] <source>[!<attribute>];` that has to be read live and reported (same
// "<entity>[!<attribute>]" convention hosts device-info capabilities already use) -- both name
// sets validated against knownConstantDeviceAttributes (integration_hosts_storage.go), the
// generalized-to-all-kinds "device (map)" MQTT field allowlist.
type THassBridgeDevice struct {
	DeviceID string
	// Instances is every "home_assistant <qualifier>" this device's declaration has been merged
	// from -- almost always exactly one, but a "roaming" device (ExportAs "roaming") may be
	// declared identically in more than one qualifier block within the same house (e.g. an HA
	// companion-app phone that can be actively connected to either of two local HA instances) --
	// collectHassBridgeDevicesByID merges those into one record instead of the usual
	// first-wins-on-collision rule, since each real instance still needs its own reporting
	// automation generated for it (generateHassBridgeEntityReportingAutomations) even though they
	// all publish under the shared "roaming" identity, not their own real instance names.
	Instances                 []string
	Capabilities              map[string]THassBridgeCapability
	ConstantAttributes        map[string]THostConstantAttribute
	DeviceInfoCapabilities    map[string]string // name -> source ("<entity>[!<attribute>]")
	DeviceInfoLiteralPrefixes map[string]string // name -> literal prefix, if declared
	// Export is set by the optional "device <id> export with: ...; end;" form (PROJECT.md 1.2a).
	// Propagated to coordinator/homeassistant_bridge.yaml (integration_hassbridge_generator.go)
	// as "export: true" -- the coordinator's cue (house_event_bus_coordinator/
	// discoveryhassbridge.go) to cross-post this device's metadata/state traffic onto the cloud
	// broker too, alongside "main".
	Export bool
	// ExportAs overrides the installation name a device's export is qualified under on the cloud
	// broker -- "" (the default) means "export under this installation's own real name," exactly
	// today's Export-alone behaviour; a non-empty value (currently only ever the fixed virtual
	// name "roaming", set by the "roaming" keyword replacing "export" on the device header) means
	// "export under THIS name instead," so several real installations can each independently
	// export their own local copy of a device (e.g. the HA companion app's own sensors, live on
	// whichever real HA instance the phone happens to be connected to) under one shared, stable
	// cloud-side identity, rather than three separate real-name-qualified identities that could
	// never be picked up as one. Added 2026-09-02 alongside "import"'s self-import unification --
	// see mqtt_relay.go's doc comment for the "hosts"-kind precedent this generalizes.
	ExportAs string
	// selfImport is parseHassBridgeDeviceHeaderKeywords' "import" keyword on the same device
	// header, parsed alongside Export/ExportAs by parseHassBridgeIntegrationBody but NOT resolved
	// there -- that function has no installation name available to resolve it against. Unexported:
	// pure parse-to-resolve plumbing, mirrors THostDevice.selfImport in spirit, though hassbridge
	// devices have only the one construction path (collectHassBridgeDevicesByID resolves it
	// immediately after parsing, unlike "hosts" which has two independent call sites to keep in
	// sync).
	selfImport bool
	// SelfImportFrom is "" unless selfImport was set, in which case it's this installation's own
	// resolved name -- makes this installation additionally relay whatever's published under
	// ExportAs (or, if ExportAs is "", this installation's own real name) back onto its own
	// already-existing local bridge topic, feeding the SAME entity Spaces.def positioning already
	// creates rather than a second one (see the coordinator-side relay this drives,
	// discoveryhassbridge.go -- deliberately NOT routed through TImportedDevice/
	// discoveryimport.go, which would create a structurally distinct, duplicate discovery
	// entity). Propagated to coordinator/homeassistant_bridge.yaml as "self_import_from:
	// \"<installation>\"".
	SelfImportFrom string
}

// formatInstanceSources renders a capability's per-instance sources as "<instance>=<source>" pairs
// (only for the instances named in instances, in that order) -- used for error messages that need
// to name exactly which source was checked on which instance, since a roaming capability has no
// single value to report.
func formatInstanceSources(sources map[string]string, instances []string) string {
	parts := make([]string, 0, len(instances))
	for _, instance := range instances {
		if source, ok := sources[instance]; ok {
			parts = append(parts, instance+"="+source)
		}
	}
	return strings.Join(parts, ", ")
}

// representativeSource picks one deterministic value out of a capability's per-instance Sources
// map -- for display/logging purposes only (e.g. an error message naming "the source" a capability
// resolves to), never for anything that actually routes or checks data, since a genuinely
// multi-instance capability has no single canonical source to report. "" if sources is empty.
func representativeSource(sources map[string]string) string {
	if len(sources) == 0 {
		return ""
	}
	instances := make([]string, 0, len(sources))
	for instance := range sources {
		instances = append(instances, instance)
	}
	sort.Strings(instances)
	return sources[instances[0]]
}
