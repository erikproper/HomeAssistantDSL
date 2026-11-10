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

// THassBridgeCapability is one declared entity a "home_assistant" bridge device exposes -- an
// EXPLICIT domain (mandatory, never inferred/defaulted -- per the unified device-grammar design,
// Architecture.md/the "intensional device→space linkage" plan's §D) plus its instance's own raw
// entity reference ("<entity>" or "<entity>!<attribute>", the same convention hosts
// capabilities use). The declared Domain need not match a Source's own HA domain -- a deliberate
// *coercion* (e.g. Domain "binary_sensor" with a Source that's a "switch.*" entity, since their
// on/off state values are compatible) -- the DSL author's responsibility to only coerce between
// genuinely compatible domains; not validated here (no compatibility table defined yet).
type THassBridgeCapability struct {
	Domain string
	// Source is this capability's remote source entity, "<entity>" or "<entity>!<attribute>" --
	// "" for a derived capability. (Keyed per instance until PROJECT.md 31, 2026-11-04, for a
	// roaming device declared under several.)
	Source string
	// Typing metadata, all optional -- declared via a separate "<path> unit: "<value>";"-shaped
	// line per field (integration_hassbridge_parser.go's capabilityMetadataPattern), not
	// hardwired per integration type the way hosts' cpuAttributeSpecs is: a hassbridge
	// capability's path is a flat string ("cpu/load"), not a hosts-style bare attribute name
	// ("load") a shared table could key off, so the DSL author states it explicitly instead.
	Unit        string
	Icon        string
	DeviceClass string
	StateClass  string
	// ValueMap (added 2026-09-09) translates the source's own raw reported value before it's ever
	// published -- "map: "<source-value>" "<target-value>";" (repeatable), declared inside the
	// capability's own "with: ... end;" block or as a bare trailing "<path> map: ...;" line, same
	// two places Unit/Icon/DeviceClass/StateClass can go. Real motivating case: a Roomba's own
	// native vocabulary ("home", "run", "stop", ...) doesn't match HA's own MQTT vacuum activity
	// strings ("docked", "cleaning", "idle", ...) at all -- applied at generate time
	// (remote_instance_entity_reporting.go's applyValueMap), purely within the generator, never
	// reaching the coordinator (which only ever relays an already-resolved value). Empty/absent
	// means every raw value passes through unchanged, exactly as before this field existed. A
	// value with no entry here is left completely unchanged (never an error) -- this is a
	// translation table, not an enum whitelist.
	ValueMap map[string]string
	// DerivedFromCapability/DerivedViaTemplate (added 2026-09-10, plans/
	// derived-capability-mechanism.md Phase 2): "derived DDD.NNN from EEE.MMM via TTT;"
	// declares this capability's value as a function of a SIBLING capability of the SAME device
	// (DerivedFromCapability holds EEE.MMM's own map key, MMM) rather than a literal Sources
	// entry -- Sources is empty for a derived capability. DerivedViaTemplate is TTT, "$" standing
	// for the sibling's own resolved value. Both empty for an ordinary, atomically-declared
	// capability. Populated by parseDerivedCapabilityLine (Physical_DerivedCapability.go),
	// validated (sibling presence, cycle-freedom) by validateDerivedCapabilities
	// (physical_rule_derived_capabilities.go).
	DerivedFromCapability string
	DerivedViaTemplate    string
	// Attributes (added 2026-09-21) declares extra named attributes this capability exposes
	// alongside its own state, via "attribute <name>: <source>;" (repeatable, same two places
	// map:/unit:/... can go, source unquoted -- an entity reference, not an arbitrary value) --
	// source is the same "<entity>[!<attribute>]" convention Source
	// uses. Keyed attribute-name -> source. Feeds a sibling "attributes"
	// MQTT-publish reporting automation (remote_instance_entity_reporting.go) and the coordinator's
	// json_attributes_topic (house_event_bus_coordinator/discoveryhassbridge.go) -- real motivating
	// case: vacuum.roomba going into a real fault ("stuck near a cliff") that never reached main
	// HA's frontend, since only its bare state was ever bridged, never its "error"/"error_code"
	// attributes. Empty/nil means this capability exposes no attributes beyond its own state --
	// unchanged behaviour for every capability declared before this field existed.
	Attributes map[string]string
	// AttributeValueMaps is ValueMap's per-attribute sibling -- "map <attribute-name>: "<from>"
	// "<to>";" (repeatable), keyed by attribute name, disambiguated from the bare (state-only)
	// "map: "<from>" "<to>";" purely by the mandatory name token between "map" and ":"
	// (integration_hassbridge_parser.go's capabilityWithAttributeValueMapPattern/
	// capabilityAttributeValueMapPattern). ValueMap itself stays reserved for this capability's own
	// STATE only, exactly as before this field existed -- no behaviour change for any existing
	// declaration. An entry naming an attribute this capability never actually declared via
	// Attributes is silently inert (never applied, never an error), the same "translation table,
	// not an enum whitelist" tolerance ValueMap already has -- avoids a parse-order dependency
	// between "attribute NAME: ...;" and "map NAME: ...;" lines appearing in either order.
	AttributeValueMaps map[string]map[string]string
	// Position (added 2026-09-25, PROJECT.md item 8's Overkiz/Somfy migration -- matching the
	// discovery-kind Z-Wave awnings cover's own existing full position-control feature set)
	// declares this capability's own current-position source via "derived value <source>;" (same one
	// place map:/unit:/... goes, source unquoted, same "<entity>[!<attribute>]" convention as
	// Source -- almost always "!current_position", e.g. "cover.living_room_front!
	// current_position").
	// Feeds a "position" key riding alongside "state" in this capability's own JSON state payload
	// (remote_instance_entity_reporting.go, generalising the vacuum-only domainStatePayloadTemplate
	// mechanism to a per-capability one) and the coordinator's position_topic/position_template
	// (house_event_bus_coordinator/discoveryhassbridge.go), read off that SAME state_topic rather
	// than a separate one. Empty/nil means this capability reports no position at all -- unchanged
	// behaviour for every capability declared before this field existed.
	Position string
	// Speed (Home Assistant 2026.11): a slow-speed shadow without a source of its own ("slow
	// speed;") commands its cover's own entity, its open, close and set position with this
	// "speed" ("slow") -- Overkiz's separate low-speed entity is deprecated (gone in 2027.5).
	Speed string
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
	// Instance is the "home_assistant <qualifier>" this device is declared under (a list until
	// PROJECT.md 31, 2026-11-04, for a roaming device declared under several).
	Instance                  string
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
	// DependsOnAvailabilityTopics (2026-09-16, PROJECT.md's logical-layer work) mirrors
	// TImportedDevice's own identically-named field exactly -- resolved generator-side
	// (integration_logical_storage.go's resolveDependencyAvailabilityTopics) from a Logical.def
	// declaration sharing this same DeviceID's own "dependency on <other-id>;" lines, already
	// flattened across the full transitive chain and cycle-checked. Threaded into
	// coordinator/homeassistant_bridge.yaml the same way, for the exact same reason: a
	// "home_assistant" bridge device (e.g. node.fritz_box) can depend on a "hosts"-kind ping
	// device's own liveness (e.g. node.fritz.box) too, not just an "import"-kind one.
	DependsOnAvailabilityTopics []string

	// IgnoreOtherCapabilities (2026-09-21), from an "ignore other capabilities;" body line, tells
	// generateEntityCatalogueSuggestions (mqtt_entity_existence.go) to never suggest any further
	// unclaimed entity for this device in suggestions/home_assistant_<instance>.txt -- for a device
	// whose remaining live-but-undeclared entities are known noise (e.g. a router's own auto-
	// generated diagnostic image/firmware-update entities) rather than genuine future capabilities
	// worth positioning. Mirrors TDiscoveryGatewayDevice's own identically-named field/directive
	// (added the same day, same underlying user request) -- that one opts a discovery gateway out
	// of the coordinator's one-hop via_device absorption; this one opts a hassbridge device out of
	// kind-3's own "still unclaimed" suggestion listing. Two different mechanisms (this is purely
	// generator-side report filtering, nothing coordinator-side), same directive text and same
	// underlying idea: "I already know what else is here; stop suggesting it."
	IgnoreOtherCapabilities bool
	// ReloadGuards (PROJECT.md 23): "reload integration when ... is unavailable for ...;" lines.
	ReloadGuards []TReloadGuard
}

// TReloadGuard is one "reload integration when <domain>.<label> is unavailable for <duration>;" of a
// bridged device (PROJECT.md 23): when that capability's source entity on the instance stays
// unavailable that long, the instance reloads the entity's integration (its config entry).
type TReloadGuard struct {
	Capability string
	For        string
	// Pause: "at most every <duration>" -- "" for reloadGuardPause.
	Pause    string
	Instance string
	// WhileUp: "while <device-id> is up" -- only while that device's node is on.
	WhileUp string
}
