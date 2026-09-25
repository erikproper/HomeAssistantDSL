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
 * positioned at the conceptual layer just like a physical device would. Two capability shapes
 * exist today, both generating plain HA-native YAML (never MQTT/coordinator-involved -- a logical
 * device's whole point is deriving a value from entities HA already knows about):
 *   - "<domain>.<label>: <entity> is available [with: enabler <entity-ref>; delay_off <time>;
 *     end;];" -- a condition-based entity mirroring Macros.def's own "providing" macro exactly
 *     (enabler makes it available whenever the enabler is off; delay_off debounces the flip to
 *     unavailable) -- see Conceptual_LogicalEntities.go's registerLogicalIsAvailableCapability.
 *   - "<domain>.<label>: <media-player-entity> [with: no_play_input "<value>"; end;];" where
 *     Domain=="switch" -- a media_player->switch coercion, the same one Macros.def's own
 *     "media_switch" directive already performs (generator.go's buildLeafMediaSwitchYAML,
 *     extended with this capability's own sibling "node" as its availability) -- see
 *     Conceptual_LogicalEntities.go's registerLogicalMediaSwitchCapability. Any other domain
 *     mismatch (a coercion this codebase has no real case for yet) is rejected with a clear
 *     warning rather than guessed at, same "on demand" discipline as everything else here.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 16.09.2026
 *
 */

package main

import "fmt"

// TLogicalCapability is one "<domain>.<label>: <value> [is available] [with: ...];" declaration
// inside a TLogicalDevice's own body (integration_logical_parser.go). Fields below serve two
// unrelated capability shapes (see this file's own header comment) -- whichever doesn't apply to
// a given capability is simply left zero-valued, the same way TDiscoveryCapability already mixes
// several orthogonal shapes (AvailabilityOf vs DerivedFromCapability vs Leaf) in one struct.
type TLogicalCapability struct {
	Domain string
	// Entity is the literal raw HA entity_id this capability's value/condition is computed from
	// (e.g. "media_player.social_apartment_living_room_apple_tv") -- always a plain, already-
	// existing entity on "main," never a same-device sibling capability label (logical devices
	// have no such registry to look one up in).
	Entity string
	// IsAvailable is true for "<value> is available [with: ...];" -- Entity's own reachability
	// becomes this capability's own boolean value (only "node" uses this today, but not
	// restricted to that label).
	IsAvailable bool
	// EnablerEntity/DelayOff refine an IsAvailable capability's own "with:" block, mirroring the
	// OLD "providing" macro's own enabler/delay_off exactly. Both optional, independently.
	EnablerEntity string
	DelayOff      string
	// DependsOn (2026-09-16) is this SPECIFIC capability's own "dependency on <device-id>;" lines
	// (repeatable), nested inside its own "with:" block alongside enabler/delay_off -- the user's
	// own explicit design, retrofitting appliance.apple_tv/appliance.tv with a dependency on
	// node.appletv/node.tv's own ping-liveness. Deliberately scoped to the CAPABILITY, not the
	// owning TLogicalDevice, unlike the OTHER "dependency on" consumer (resolveDependencyAvailabilityTopics,
	// for import/hassbridge kinds' own MQTT depends_on_availability) -- an IsAvailable capability's
	// own condition is the only thing that ever needs this, so there's no reason for it to affect
	// every OTHER capability the same logical device might declare (e.g. switch.media, which
	// already derives its own availability via its sibling "node" capability's own reverse lookup
	// instead). Resolved by resolveLogicalDependencyNodeEntities (integration_logical_storage.go)
	// into each dependency's own HA node entity_id -- a flat, one-hop list (no transitive/cycle
	// walking, unlike the device-level DependsOn below): every real case so far only ever names a
	// plain "hosts" ping device with no further dependencies of its own.
	DependsOn []string
	// NoPlayInput refines a media-switch-coercion capability's own "with:" block (meaningful only
	// when Domain=="switch" and Entity's own domain is "media_player").
	NoPlayInput string
	// AbsorbedFromDeviceID/AbsorbedFromCapability (2026-09-18, the "absorb" operation --
	// memory/project_logical_layer_device_combining.md, deferred since 2026-08-22 until
	// Zigbee2MQTT migration was underway) are set together, from one "<domain>.<label>:
	// <bare-capability>;" line inside a TLogicalDevice's own "capabilities from <device-id>: ...
	// end;" block. Mutually exclusive with Entity/IsAvailable/the switch-coercion shape above:
	// this capability's value isn't computed by THIS device at all, it's another (absorbed)
	// device's own named capability, resolved via THAT device's normal registration path
	// (registerLogicalAbsorbedCapability, Conceptual_LogicalEntities.go, delegates straight to
	// registerDeviceCapabilityEntityLinkAllowingHidden targeting AbsorbedFromDeviceID) but
	// attributed to and positioned under THIS device's own leaf/naming. An absorbed device's own
	// switch acting as an enabler of the HOST device's overall availability (e.g.
	// appliance.washing_machine's own plug) is expressed as an ordinary, separate IsAvailable
	// capability on the host device, with EnablerEntity a literal, already-known entity_id --
	// same style Logical.def already uses for appliance.tv's own enabler -- rather than a
	// dedicated absorb-block keyword: the host's OWN raw node lives on a different HA instance
	// than a Zigbee2MQTT enabler switch (hassbridge relays are computed by a Jinja template that
	// runs on the REMOTE source instance, never locally), so the two can't be folded into one
	// expression the way a purely local enabler can -- a wrapping capability referencing the
	// host's own already-registered raw node is the only way to compose them.
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
	// IsDerivedCondition/DerivedCondition/DerivedDeviceClass/DelayOn (2026-09-18, tree shape since
	// 2026-09-24) are set together, from a "derived <domain>.<label> with: condition <bool-expr>;
	// device_class <val>; delay_on <time>; delay_off <time>; end;" block -- a MULTI-SOURCE boolean
	// condition (and/or/not/parens over bare sibling refs and "jinja ... { ... }" sub-clauses,
	// logical_condition_expr.go) generalizing IsAvailable's own fixed single-$1
	// "$1 not in [...]" shape. DerivedCondition's leaves are unresolved raw tokens (sibling
	// capability labels, or literal "domain.path[!attribute]" entity references); each is resolved
	// by resolveLogicalSiblingSourceEntityID (Conceptual_LogicalEntities.go) -- deliberately kind-
	// agnostic: a sibling may be an ordinary IsAvailable/switch-coercion capability, another
	// "defined" one, or one absorbed via "capabilities from ...;" (registerLogicalAbsorbedCapability
	// records a LogicalEntityLinks entry under THIS device's own id/label specifically so this
	// lookup sees it too, per the user's own explicit design: this resolution must see the COMPLETE
	// logical device). Same-device only for now -- no "from <device-id>" qualifier per reference
	// yet, on-demand discipline (see memory/project_flat_entity_form_ban.md's sibling note on this
	// same pattern); a sibling not yet positioned defers exactly like any other reverse-lookup-based
	// capability (registerLogicalDerivedConditionCapability). DelayOn is separate from the existing
	// IsAvailable-only DelayOff field above -- IsAvailable has never needed delay_on, so it stayed a
	// single field; this capability needs both. First real case: "windy" itself, replacing
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
}

// TLogicalDevice is one "device <id> with: ... end;" declaration inside Logical.def's "logical
// layer with: ... end;" block (integration_logical_parser.go).
type TLogicalDevice struct {
	DeviceID     string
	DependsOn    []string // other device ids (any kind, including another logical-only id)
	Capabilities map[string]TLogicalCapability
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

// resolveLogicalDependencyNodeEntities is resolveDependencyAvailabilityTopics' own counterpart for
// a logical device's "is available" Jinja condition (registerLogicalIsAvailableCapability,
// Conceptual_LogicalEntities.go) rather than an MQTT-published capability's own
// depends_on_availability -- the two mechanisms can't share one resolution, since a logical "is
// available" capability's own condition is a generator-authored HA-native template (never routed
// through the coordinator at all), so it needs each dependency's own HA ENTITY_ID to write an
// is_state() check against, not a raw MQTT topic string. See Architecture.md §6.7's own note on
// this split (2026-09-16): entity domains hard-wired straight into the main instance (media_player,
// weather, ...) have no MQTT availability topic to depend on at all.
//
// A dependency already positioned in Conceptual.def (administration.DeviceConceptualLinks has a
// NodeEntityID for it -- e.g. Vienna's "device infrastructural:appletv from node.appletv;") reuses
// that entity_id directly. One that ISN'T positioned anywhere gets a synthetic positioning of its
// own instead, under a default "physical/<raw-device-id>/node" name (e.g.
// "binary_sensor.physical_node_appletv_node") -- purely functional/internal, never meant to be
// looked at directly on a dashboard, but a REAL entity all the same, generated the exact same way
// registerHostDevicePositioning would for an explicit "device <spec> from <device-id>;" line.
//
// Real bug found live 2026-09-17: this used to call RegisterDiscoveryImpliedEntity directly and
// stop there -- bookkeeping-only (it satisfies checkEntityReferences and skips a customization
// file), but never actually gives generateHostsIntegrationOutputs a DeviceConceptualLinks entry to
// find for this device. That function only ever writes a "conceptual: node_entity: ...;" block into
// coordinator/devices.yaml when admin.DeviceConceptualLinks[deviceID] exists -- without it, the
// coordinator has no idea an HA entity should exist for this device's own liveness at all, so it
// never publishes discovery for it. The referencing is_state() check was therefore ALWAYS false
// (an unknown entity's state can never match 'on'), silently breaking three already-deployed
// devices' own "is available" conditions (tv, apple_tv, the living_room sonos pair) -- confirmed
// live: binary_sensor.infrastructural_apartment_living_room_apple_tv_node stuck permanently off.
// Fixed by actually performing the positioning (registerHostNodeEntity, the same helper
// registerHostDevicePositioning itself calls) and storing the result into
// administration.DeviceConceptualLinks[depID], exactly as if a real "device ... from
// <device-id>;" line had positioned it -- same entity_id as before (nothing downstream needs to
// change), just now a real, live one.
//
// dependsOn is TLogicalCapability.DependsOn -- a flat, ALREADY-SCOPED-TO-THIS-CAPABILITY list, not
// a device-level one -- so unlike resolveDependencyAvailabilityTopics this never walks a
// transitive/cyclic chain: every real case so far only ever names a plain "hosts" ping device with
// no further dependencies of its own, and a flat one-hop resolution is enough until that changes.
// Scope otherwise identical to resolveDependencyAvailabilityTopics: hosts-kind dependency targets
// only, same warn-and-continue discipline for anything else.
func resolveLogicalDependencyNodeEntities(dependsOn []string, hostDevicesByID map[string]THostDevice, administration *TAdministrationState, provenance string) ([]string, []string) {
	var entityIDs []string
	var warnings []string
	for _, depID := range dependsOn {
		if link, ok := administration.DeviceConceptualLinks[depID]; ok && link.NodeEntityID != "" {
			entityIDs = append(entityIDs, link.NodeEntityID)
			continue
		}
		host, ok := hostDevicesByID[depID]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("%s: depends on %q, which isn't a \"hosts\" integration device (dependency resolution isn't built for any other kind yet) -- ignoring this dependency", provenance, depID))
			continue
		}
		mat, ok := MaterializationForIntegrationType(host.IntegrationType)
		if !ok {
			continue // can't happen for a real THostDevice (IntegrationType is validated at parse time), but never worth a panic over
		}
		syntheticIdentity := TEntityIdentity{Sphere: "physical", Path: depID}
		// "auto-registered dependency node: " prefix (2026-09-21) lets resolveListEntries
		// (generator.go) exclude this fallback registration from External.def's own "nodes"-style
		// declarations, the same way it already excludes "derived" entities -- this entity exists
		// purely to make "dependency on <device-id>;" work for a device that was never itself
		// explicitly positioned (an ugly synthetic name, "physical/<device-id>/node"), not because
		// the DSL author positioned a genuine, standalone node worth a dashboard's own attention.
		// Real case found live: appliance.washing_machine's "dependency on node.candy;" auto-
		// registered "binary_sensor.physical_node_candy_node" purely as availability plumbing, and
		// it kept surfacing in list.nodes as if it were a real, deliberately positioned device.
		dependencyProvenance := "auto-registered dependency node: " + provenance
		link, linkWarnings := registerHostDeviceIdentity(administration, mat, host, syntheticIdentity, depID)
		warnings = append(warnings, linkWarnings...)
		nodeFullName := fmt.Sprintf("%s.%s/%s/%s", mat.NodeDomain, syntheticIdentity.Sphere, syntheticIdentity.Path, mat.NodeSuffix)
		link.NodeEntityID, link.NodeDeviceClass, link.NodeIcon = registerHostNodeAttribute(administration, mat, host, nodeFullName, "root", dependencyProvenance)
		administration.DeviceConceptualLinks[depID] = link
		if link.NodeEntityID != "" {
			entityIDs = append(entityIDs, link.NodeEntityID)
		}
	}
	return entityIDs, warnings
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
