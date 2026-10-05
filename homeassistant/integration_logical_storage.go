/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationLogicalStorage
 *
 * The data model and resolution helpers for Logical.def's "logical layer with: ... end;" block
 * (PROJECT.md's logical-layer work, started 2026-09-16 -- see memory/project_logical_layer_
 * introduction.md for the overall on-demand strategy). A logical device declared under the SAME
 * id as an existing physical-layer device (hosts/hassbridge/discovery/import) is an EXTENSION of
 * it, not a competing second device -- when Conceptual.def positions that id, the physical
 * device's own capabilities resolve exactly as before, plus whatever this file's own
 * TLogicalDevice adds. A logical device can declare "dependency on <device-id>;" (repeatable): the
 * device's own availability (its "node" capability, and every other capability) must additionally
 * go unavailable whenever any declared dependency's own node capability does -- see
 * resolveDependencyAvailabilityTopics' own doc comment for exactly how that gets threaded through
 * to the coordinator.
 *
 * A logical device can ALSO declare its own capabilities directly (2026-09-16, the
 * media_player_device migration) -- unlike "dependency on," this is how a BRAND NEW device id
 * (one with no existing physical-layer counterpart at all, e.g. "utility.apple_tv") gets
 * positioned at the conceptual layer just like a physical device would. Four capability shapes
 * exist today, all generating plain HA-native YAML (never MQTT/coordinator-involved -- a logical
 * device's whole point is deriving a value from entities HA already knows about): absorbed
 * (TLogicalCapability.AbsorbedFromDeviceID), defined input_number helpers (IsDefinedInputNumber),
 * and derived conditions/values (IsDerivedCondition/IsDerivedValue) -- see each field's own doc
 * comment below. (Two earlier shapes, a plain "<value> is available [with: enabler ...;];"
 * condition and a standalone "<media-player-entity> [with: no_play_input ...;];" switch coercion,
 * were retired 2026-10-05 once both were fully superseded -- the first by the "enabled by"/
 * "dependency on" sugar below, the second by Physical.def's own "local"-kind switch.media
 * capability -- confirmed zero remaining usages in either house before removal.)
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 16.09.2026
 *
 */

package main

import "fmt"

// TLogicalCapability is one "<domain>.<label> <value>;" declaration inside a TLogicalDevice's own
// body (integration_logical_parser.go). Fields below serve several unrelated capability shapes
// (see this file's own header comment) -- whichever doesn't apply to a given capability is simply
// left zero-valued, the same way TDiscoveryCapability already mixes several orthogonal shapes
// (AvailabilityOf vs DerivedFromCapability vs Leaf) in one struct.
type TLogicalCapability struct {
	Domain string
	// AbsorbedFromDeviceID/AbsorbedFromCapability (2026-09-18, the "absorb" operation --
	// memory/project_logical_layer_device_combining.md, deferred since 2026-08-22 until
	// Zigbee2MQTT migration was underway) are set together, from one "<domain>.<label>:
	// <bare-capability>;" line inside a TLogicalDevice's own "capabilities from <device-id>: ...
	// end;" block. Mutually exclusive with the defined/derived shapes below: this capability's
	// value isn't computed by THIS device at all, it's another (absorbed) device's own named
	// capability, resolved via THAT device's normal registration path
	// (registerLogicalAbsorbedCapability, Conceptual_LogicalEntities.go, delegates straight to
	// registerDeviceCapabilityEntityLinkAllowingHidden targeting AbsorbedFromDeviceID) but
	// attributed to and positioned under THIS device's own leaf/naming. An absorbed device's own
	// switch acting as an enabler of the HOST device's overall availability (e.g.
	// appliance.washing_machine's own plug) is expressed via the "enabled by"/"dependency on"
	// device-level sugar instead (logical_enabler_dependency_sugar.go) -- a dedicated absorb-block
	// keyword never existed for this.
	AbsorbedFromDeviceID   string
	AbsorbedFromCapability string
	// IsDefinedInputNumber/DefinedMinimum/DefinedMaximum/DefinedStep/DefinedIcon/DefinedUnits
	// (2026-09-18) are set together, from a "defined <domain>.<label> with: minimum <val>; maximum
	// <val>; step <val>; icon <val>; units <val>; end;" block -- declares an HA input_number HELPER
	// entity (a pure, user-adjustable value with no device/MQTT backing at all), reusing the exact
	// same TEntityRecord.InputNumberMin/Max/Step/Unit/Icon fields the plain Conceptual-layer
	// "entity input_number.<spec> with: minimum ...; end;" construct already populates (see
	// registerLogicalDefinedInputNumberCapability, Conceptual_LogicalEntities.go). First real case:
	// the "windy" threshold, replacing Macros.def's own "windy"/"sunny" macros' identical
	// "${x} = entity input_number.social/x_threshold with: ...;" shape.
	IsDefinedInputNumber bool
	DefinedMinimum       string
	DefinedMaximum       string
	DefinedStep          string
	DefinedIcon          string
	DefinedUnits         string
	// IsDerivedCondition/DerivedCondition/DerivedDeviceClass/DelayOn/DelayOff (2026-09-18, tree
	// shape since 2026-09-24) are set together, from a "derived <domain>.<label> with: condition
	// <bool-expr>; device_class <val>; delay_on <time>; delay_off <time>; end;" block -- a
	// MULTI-SOURCE boolean condition (and/or/not/parens over bare sibling refs and "jinja ... {
	// ... }" sub-clauses, logical_condition_expr.go). DerivedCondition's leaves are unresolved raw
	// tokens (sibling capability labels, or literal "domain.path[!attribute]" entity references);
	// each is resolved by resolveLogicalSiblingSourceEntityID (Conceptual_LogicalEntities.go) --
	// deliberately kind-agnostic: a sibling may be an ordinary "defined" capability, or one absorbed
	// via "capabilities from ...;" (registerLogicalAbsorbedCapability records a LogicalEntityLinks
	// entry under THIS device's own id/label specifically so this lookup sees it too, per the
	// user's own explicit design: this resolution must see the COMPLETE logical device). Same-device
	// only for now -- no "from <device-id>" qualifier per reference yet, on-demand discipline (see
	// memory/project_flat_entity_form_ban.md's sibling note on this same pattern); a sibling not yet
	// positioned defers exactly like any other reverse-lookup-based capability
	// (registerLogicalDerivedConditionCapability). First real case: "windy" itself, replacing
	// Macros.def's own "windy" macro's "condition ${entity} ${windy_threshold} ...;" verbatim.
	//
	// IsDerivedValue/DerivedValue (2026-09-24) is the SAME tree type, restricted at parse time
	// (logical_condition_expr.go's parseValueExpr) to a single non-boolean-composable atom (a bare
	// ref or one "jinja ... { ... }" clause, never and/or/not/parens) -- the correct keyword for a
	// non-"binary_sensor" derivation (e.g. sensor.pressure), replacing the pre-2026-09-24 misuse of
	// "condition" for a value that was never actually a boolean (registerLogicalDerivedConditionCapability
	// now rejects a domain/keyword mismatch either way).
	IsDerivedCondition bool
	DerivedCondition   *TConditionExpr
	IsDerivedValue     bool
	DerivedValue       *TConditionExpr
	DerivedDeviceClass string
	DelayOn            string
	DelayOff           string
}

// TLogicalSwitchedByFact is one "<domain>.<label> is switched by <domain>.<label> from
// <device-id>;" line inside a TLogicalDevice's own body (2026-09-27, the "switched_device"
// migration -- see logical_switched_by.go's own header comment for the full design/rationale).
// Unlike TLogicalCapability's own shapes, this is metadata ATTACHED to a capability the owning
// device ALREADY declares at the physical layer (Physical.def/local-kind) -- it never replaces or
// redefines that capability's own registration, only additionally drives
// administration.SwitchedDeviceRelations once the annotated capability gets positioned anywhere
// outside the "physical" sphere (see maybeRegisterSwitchedByRelation).
type TLogicalSwitchedByFact struct {
	OwnDomain      string // the annotated capability's own domain (e.g. "light") -- must match its real Physical.def-declared domain
	SourceDomain   string // the referenced sibling device's own capability domain (e.g. "light", even when the underlying hardware is really a switch)
	SourceLabel    string // the referenced sibling device's own capability label (e.g. "light_2")
	SourceDeviceID string // the referenced sibling device's own DSL device-id (e.g. "switches.house_server_room_fuse_box_zwave_048")
}

// TLogicalEnabledByFact holds a device-level "<domain>.<label> enabled by <domain>.<label>
// from <device-id>;" annotation (2026-09-28) -- the fuller, LHS-validated generalization that
// REPLACES the plain "enabler <device-id>;" shorthand outright (confirmed with the user: not kept
// alongside it). At most one per device (there is only ever one "node" capability to build), so a
// pointer field on TLogicalDevice, not a map like TLogicalSwitchedByFact's own per-capability
// shape (which genuinely combines several DIFFERENT capabilities across two devices; this
// annotation only ever refines ONE thing -- the device's own liveness self-check).
type TLogicalEnabledByFact struct {
	OwnDomain      string // the LHS capability's own domain (e.g. "media_player") -- must match a capability ALREADY declared on this device at the physical layer (validated at resolution time via the normal same-device sibling-ref lookup, not a separate pre-check)
	OwnLabel       string // the LHS capability's own bare label (e.g. "core")
	SourceDomain   string // the enabler switch's own capability domain (e.g. "switch")
	SourceLabel    string // the enabler switch's own capability label (e.g. "core")
	SourceDeviceID string // the enabler switch's own DSL device-id (e.g. "switch.house_living_room_tv")
}

// TLogicalDevice is one "device <id> with: ... end;" declaration inside Logical.def's "logical
// layer with: ... end;" block (integration_logical_parser.go).
type TLogicalDevice struct {
	DeviceID     string
	DependsOn    []string // other device ids (any kind, including another logical-only id)
	Capabilities map[string]TLogicalCapability
	// EnabledBy (2026-09-28) is this device's own "<domain>.<label> enabled by <domain>.<label>
	// from <device-id>;" fact, if declared -- sugar for the recurring "absorb the enabler switch's
	// own capability as switch.enabler, then let the enabler being 'off' stand in for this device's
	// own unavailability" pattern (a deliberately powered-down TV/appliance is not a fault), same
	// spirit as the OLD plain "enabler <device-id>;" line this REPLACES, but with the base
	// self-check built from the named LHS capability's own "is available" state instead of the
	// whole device's "physical <id>" liveness -- more precise when a device has several capabilities
	// and only ONE of them is what actually signals "the appliance is on". Desugared by
	// applyLogicalEnablerDependencySugar (logical_enabler_dependency_sugar.go) into the SAME
	// Capabilities["enabler"]/Capabilities["node"] entries the old hand-written longhand already
	// produced -- see that function's own doc comment for the full desugaring. nil means none
	// declared.
	EnabledBy *TLogicalEnabledByFact
	// SwitchedBy (2026-09-27) holds every "<domain>.<label> is switched by <domain>.<label> from
	// <device-id>;" fact this device declares, keyed by the annotated capability's OWN bare label
	// (e.g. "core") -- see TLogicalSwitchedByFact's own doc comment.
	SwitchedBy map[string]TLogicalSwitchedByFact
	// NodeDelayOff (2026-09-28) is this device's own top-level "delay_off <time>;" line, if declared
	// -- carried into the "enabler"/"enabled by" sugar's own synthesized "node" derived-condition
	// capability (applyLogicalEnablerDependencySugar), matching what a hand-written "derived
	// binary_sensor.node with: ... delay_off <time>; end;" block could already express. Added for
	// Vienna's own appliance.tv (a real, deployed 1-minute debounce before the TV's own liveness
	// flips off) -- without this field, migrating a device using delay_off to the sugar would
	// silently drop that debounce, a real behavioural regression, not a cosmetic one. "" means none
	// declared (the sugar's own synthesized node capability then has no delay).
	NodeDelayOff string
}

// resolveTransitiveDependencies walks deviceID's own DependsOn edges (and, transitively, every
// dependency's own DependsOn) to the full closure a dependent device's availability must AND in --
// A depends on B depends on C means A's own availability needs B's AND C's, not just B's (the
// coordinator never re-walks this itself; the whole flattened, cycle-checked list is resolved
// once, generator-side, and handed to it already flat). cycle is non-empty (the offending path,
// for a clear warning) if deviceID's own dependency graph loops back on itself -- chain is always
// returned too in that case (everything found before the cycle was detected), but callers should
// treat a non-empty cycle as "don't trust this list" and skip applying it rather than use a
// partial result.
func resolveTransitiveDependencies(deviceID string, logicalByID map[string]TLogicalDevice) (chain []string, cycle []string) {
	seen := map[string]bool{}   // already fully resolved, anywhere in the walk -- dedupes chain
	onPath := map[string]bool{} // on the CURRENT dfs path -- revisiting one of these is a cycle
	path := []string{deviceID}

	var walk func(id string) []string
	walk = func(id string) []string {
		onPath[id] = true
		defer delete(onPath, id)

		logical, ok := logicalByID[id]
		if !ok {
			return nil
		}
		for _, depID := range logical.DependsOn {
			if onPath[depID] {
				return append(append([]string{}, path...), depID)
			}
			path = append(path, depID)
			if found := walk(depID); found != nil {
				return found
			}
			path = path[:len(path)-1]

			if !seen[depID] {
				seen[depID] = true
				chain = append(chain, depID)
			}
		}
		return nil
	}

	cycle = walk(deviceID)
	if cycle != nil {
		return chain, cycle
	}
	return chain, nil
}

// resolveDependencyAvailabilityTopics turns deviceID's own fully-resolved dependency chain (see
// resolveTransitiveDependencies) into the raw MQTT topics the coordinator should AND into that
// device's own availability (house_event_bus_coordinator's buildAvailabilityFields, now variadic
// over exactly this kind of list). Every dependency target must resolve to SOME kind's own
// "node"/liveness topic -- today that's only implemented for the "hosts" integration (every
// IntegrationType shares THostsEntityMaterialization.NodeTopic, e.g. "hosts/netatmo/node/state"),
// since that's the only kind a real Physical.def dependency target exists for yet. A dependency
// naming a hassbridge/discovery/import device, or an id that resolves to nothing at all, is
// reported as a warning and simply excluded from the returned list (a missing dependency topic
// should never be silently treated as "always available," but nor should it block generation --
// same warn-and-continue discipline every other Physical.def authoring mistake in this codebase
// gets) -- extend this dispatch the next time a real dependency on one of those kinds is needed.
func resolveDependencyAvailabilityTopics(deviceID string, logicalByID map[string]TLogicalDevice, hostDevicesByID map[string]THostDevice) ([]string, []string) {
	logical, ok := logicalByID[deviceID]
	if !ok || len(logical.DependsOn) == 0 {
		return nil, nil
	}

	chain, cycle := resolveTransitiveDependencies(deviceID, logicalByID)
	if cycle != nil {
		return nil, []string{fmt.Sprintf("Logical.def: device %q's dependency chain cycles: %v -- ignoring its \"dependency on\" declarations entirely", deviceID, cycle)}
	}

	var topics []string
	var warnings []string
	for _, depID := range chain {
		host, ok := hostDevicesByID[depID]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("Logical.def: device %q depends on %q, which isn't a \"hosts\" integration device (dependency resolution isn't built for any other kind yet) -- ignoring this dependency", deviceID, depID))
			continue
		}
		mat, ok := MaterializationForIntegrationType(host.IntegrationType)
		if !ok {
			continue // can't happen for a real THostDevice (IntegrationType is validated at parse time), but never worth a panic over
		}
		if topic := mat.NodeTopic(host.HostName); topic != "" {
			topics = append(topics, topic)
		}
	}
	return topics, warnings
}

// applyResolvedDependencyTopics copies admin.DependsOnAvailabilityTopics (resolved once,
// generator-side, in parseAdministrationFromPaths -- see TAdministrationState's own field
// comment) onto a freshly (and independently, per this codebase's own established convention)
// re-parsed imported-device slice, keyed by DeviceID. Never re-resolves or re-walks the
// dependency graph itself -- that already happened once, admin is just being consulted here, so
// Physical_Generator.go's own re-parse of Physical.def's "import" integration (right before
// coordinator/imported.yaml is written) doesn't have to re-collect Logical.def or re-run
// resolveDependencyAvailabilityTopics a second time.
func applyResolvedDependencyTopics(importedDevices []TImportedDevice, admin *TAdministrationState) {
	for i := range importedDevices {
		if topics, ok := admin.DependsOnAvailabilityTopics[importedDevices[i].DeviceID]; ok {
			importedDevices[i].DependsOnAvailabilityTopics = topics
		}
	}
}

// applyResolvedDependencyTopicsToHassBridge is applyResolvedDependencyTopics' own mirror for the
// "home_assistant" bridge kind (2026-09-16, folding "node.fritz.box" -- a "hosts"/ping device for
// Vienna's router -- into "node.fritz_box" -- the SAME router's "home_assistant" bridge device --
// exactly the way "node.vienna_livingroom depends on host.netatmo" was done first). Map-based
// rather than slice-based, since hassBridgeDevicesByID (Physical_Generator.go's own collected map,
// keyed by DeviceID already) never gets independently re-parsed as a slice the way imported
// devices do.
func applyResolvedDependencyTopicsToHassBridge(hassBridgeDevicesByID map[string]THassBridgeDevice, admin *TAdministrationState) {
	for id, device := range hassBridgeDevicesByID {
		if topics, ok := admin.DependsOnAvailabilityTopics[id]; ok {
			device.DependsOnAvailabilityTopics = topics
			hassBridgeDevicesByID[id] = device
		}
	}
}
