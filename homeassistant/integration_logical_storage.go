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
 * capability -- confirmed zero remaining usages in either house before removal. That "enabled by"/
 * device-level "delay_off" sugar was itself retired 2026-10-11, PROJECT.md item 11a: it silently
 * replaced a device's own real Physical.def "node" condition instead of ANDing onto it whenever the
 * device also declared "enabled by" -- fixed by making every such device spell out its own
 * absorbed "switch.enabler" capability plus an explicit "derived binary_sensor.node with: ...;
 * end;" block directly, the same style already used by every hand-written case in both houses, then
 * deleting the sugar entirely once nothing used it any more.)
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
	// Zigbee2MQTT migration was underway) are set together, from one "capability <domain>.<label>:
	// <bare-capability>;" line inside a TLogicalDevice's own "absorb from <device-id>: ... end;"
	// block (renamed from "capabilities from", PROJECT.md item 11b, 2026-10-11, to match this
	// mechanism's own internal name). Mutually exclusive with the defined/derived shapes below:
	// this capability's value isn't computed by THIS device at all, it's another (absorbed)
	// device's own named capability, resolved via THAT device's normal registration path
	// (registerLogicalAbsorbedCapability, Conceptual_LogicalEntities.go, delegates straight to
	// registerDeviceCapabilityEntityLinkAllowingHidden targeting AbsorbedFromDeviceID) but
	// attributed to and positioned under THIS device's own leaf/naming. An absorbed device's own
	// switch acting as an enabler of the HOST device's overall availability (e.g.
	// appliance.washing_machine's own plug) is just an ordinary absorbed "capability
	// switch.enabler <capability>;" line, paired with an explicit "derived capability
	// binary_sensor.node with: condition ... or jinja \"$[switch.enabler] == 'off'\"; end;" block
	// declared alongside it -- PROJECT.md item 11a, 2026-10-11: an earlier "enabled by"/device-level
	// "delay_off" sugar used to synthesize both of these automatically, removed once both houses'
	// real Logical.def spelled every such device out explicitly instead.
	AbsorbedFromDeviceID   string
	AbsorbedFromCapability string
	// IsDefinedInputNumber/DefinedMinimum/DefinedMaximum/DefinedStep/DefinedIcon/DefinedUnits
	// (2026-09-18) are set together, from a "defined capability <domain>.<label> with: minimum <val>; maximum
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
	// shape since 2026-09-24) are set together, from a "derived capability <domain>.<label> with: condition
	// <bool-expr>; device_class <val>; delay_on <time>; delay_off <time>; end;" block -- a
	// MULTI-SOURCE boolean condition (and/or/not/parens over bare sibling refs and "jinja ... {
	// ... }" sub-clauses, logical_condition_expr.go). DerivedCondition's leaves are unresolved raw
	// tokens (sibling capability labels, or literal "domain.path[!attribute]" entity references);
	// each is resolved by resolveLogicalSiblingSourceEntityID (Conceptual_LogicalEntities.go) --
	// deliberately kind-agnostic: a sibling may be an ordinary "defined" capability, or one absorbed
	// via "absorb from ...;" (registerLogicalAbsorbedCapability records a LogicalEntityLinks
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
	// AssignedID is a derived capability's frozen "as <id>" (PROJECT.md item 48) -- its universal
	// ID, the unique_id its generated template entity is registered under. "" until assigned.
	AssignedID string
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

// TLogicalDevice is one "device <id> with: ... end;" declaration inside Logical.def's "logical
// layer with: ... end;" block (integration_logical_parser.go).
type TLogicalDevice struct {
	DeviceID     string
	DependsOn    []string // other device ids (any kind, including another logical-only id)
	Capabilities map[string]TLogicalCapability
	// SwitchedBy (2026-09-27) holds every "<domain>.<label> is switched by <domain>.<label> from
	// <device-id>;" fact this device declares, keyed by the annotated capability's OWN bare label
	// (e.g. "core") -- see TLogicalSwitchedByFact's own doc comment.
	SwitchedBy map[string]TLogicalSwitchedByFact

	// Colocated (2026-10-07) is every "colocated <device-id> as <leaf>;" line this device
	// declares -- repeatable, each naming an otherwise-independently-positioned device (one with
	// no conceptual location of its own, e.g. a bare relay switch that just happens to physically
	// sit at this device's own location) that should be auto-positioned at THIS device's own
	// resolved conceptual position, with "/<leaf>" appended (leaf always explicit, never
	// auto-derived -- confirmed against the user's own real Vienna edit, "colocated
	// switch.living_room_tv as switch;"). The concrete trigger: switch.living_room_tv (a Z-Wave
	// relay controlling Vienna's TV's power) has no conceptual position of its own, but obviously
	// lives everywhere appliance.living_room_tv (the TV device it switches) lives --
	// applyColocatedPositioning (device_friendly_name.go) resolves this, generator-side, once
	// Conceptual.def parsing completes; see that function's own doc comment for the full mechanics.
	Colocated []TLogicalColocatedFact
}

// TLogicalColocatedFact is one "colocated <device-id> as <leaf>;" line -- see
// TLogicalDevice.Colocated's own doc comment.
type TLogicalColocatedFact struct {
	DeviceID   string
	LeafSuffix string
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
