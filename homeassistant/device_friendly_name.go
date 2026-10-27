/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: DeviceFriendlyName
 *
 * PROJECT.md item 8 (2026-10-06): a device's own MQTT discovery "device": {"name": ...} block
 * should describe where this project's own conceptual layer says the device IS, not whatever a
 * gateway's own native metadata happens to carry (a Z-Wave relay's own zwavejs2mqtt Location+Name
 * describes where the RELAY MODULE is installed, e.g. a garage fuse cabinet -- not the room
 * whose light it actually switches, and a relay commonly switches more than one room's own light
 * from the exact same physical position, so there's no single "room" answer anyway).
 *
 * The user's own rule, confirmed against a real live case (Junglinster's
 * switches.garage_fuse_cabinet_zwave_042, a Fibaro double-relay): if the device's own "node"
 * capability is positioned in Conceptual.def, the device's friendly name is THAT entity's own
 * conceptual position (sphere + full path), with its own trailing "node" segment dropped --
 * confirmed resolving to "infrastructural/garage/fuse_cabinet/zwave/042" for that real device,
 * correctly describing the relay's own actual install location, independent of which room(s) it
 * happens to switch. If "node" is NOT positioned, fall back to the device's own raw DSL id and
 * flag it -- a device with ANY positioned entity but no positioned node is exactly PROJECT.md
 * item 7's own "every device needs a node" concern, just caught at POSITIONING time rather than
 * DECLARATION time (physical_rule_node_capability.go's Rule 1 and logical_rule_node_capability.go
 * both only check Physical.def/Logical.def's own Capabilities map, never whether Conceptual.def
 * actually positions it).
 *
 * Finding the node entity's own resolved identity needs TWO different lookup paths, confirmed by
 * reading resolveLogicalSiblingSourceEntityID's own established dispatch
 * (Conceptual_LogicalEntities.go) rather than assuming one: a "hosts"/"hassbridge"/"import"-kind
 * device's node lives on DeviceConceptualLinks[deviceID] (NodeEntityID or
 * AttributeEntityIDs["node"]), but a DISCOVERY-kind device's "node" is an ordinary discovery
 * capability -- resolved via (GatewayDeviceID, Leaf) matching against admin.DiscoveryEntityLinks,
 * the exact same lookup autoAssignDiscoveryStableIDs already builds
 * (physical_auto_assign_stable_ids.go) -- DeviceConceptualLinks is never populated for it at all
 * unless something else (an absorb/switched-by cross-reference) separately causes it to be. A
 * first version of this file checked only the DeviceConceptualLinks path and silently flagged
 * EVERY discovery-kind device as unpositioned (61/157 false positives across the two houses,
 * caught by running it against real data before trusting it, not by code review).
 *
 * Even with that fix, real Conceptual.def content for both houses still had devices with no "node"
 * positioned at all. The user's own refinement (2026-10-06): a device used in EXACTLY one space has
 * only one possible answer for "where is this device," so IMPLY its "node" positioning there
 * automatically instead of demanding the author spell out the single obvious case by hand --
 * applyImpliedNodePositioning (since removed) did this. The user's own follow-up (2026-10-08, a
 * two-step plan): once backfill_explicit_node_positioning.go has given every device that mechanism
 * could ever successfully imply an explicit "node" entity of its own, drop the implication entirely
 * and make validateDevicesHaveFriendlyNames (below) a HARD rule instead of a warning -- a device
 * positioned in more than one space, or never positioned via a "device <id> with:" block at all, is
 * a genuine authoring gap, not something to guess at silently.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 06.10.2026
 *
 */

package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// conceptualProvenancePattern parses a TEntityRecord.Provenance string back into its own source
// line number and original spec text -- every Conceptual-layer provenance is built as
// "<file>:<lineNum> → <spec> ...". Used by deriveDeviceOwnLeafFromSiblings (below) to recover a
// sibling's own declared leaf text.
var conceptualProvenancePattern = regexp.MustCompile(`^(\S+):(\d+) → (\S+)`)

// spaceContextPath recovers, from a space name alone (an EntityRecordsBySpace map key, e.g.
// "social/house/living_room/living"), the sphere-less "contextPath" portion
// normalizeSpaceContext (expander.go) would have computed from the live administration.SpacePath
// at the moment an entity was registered there. formatNestedSpaceName (expander.go) builds every
// such space name as exactly "<spaceType>/<contextPath>" (or bare spaceType/contextPath alone when
// the other is empty) -- so recovering contextPath is just dropping that leading spaceType
// segment and rejoining the rest. "" (root, or a bare spaceType with no contextPath at all) has no
// context to strip at all.
func spaceContextPath(spaceName string) string {
	if spaceName == "" || spaceName == "root" {
		return ""
	}
	segments := strings.Split(spaceName, "/")
	if len(segments) <= 1 {
		return ""
	}
	return strings.Join(segments[1:], "/")
}

// entityIdentityByID finds entityID's own stored TEntityIdentity -- the same linear scan over
// EntityRecordsBySpace already used elsewhere in this codebase (e.g. administration.go's own
// RegisteredExternalEntityReferences bookkeeping) to go from a flattened entity_id back to its
// pre-flattening Sphere/Path.
func entityIdentityByID(admin *TAdministrationState, entityID string) (TEntityIdentity, bool) {
	for _, records := range admin.EntityRecordsBySpace {
		for _, rec := range records {
			if toHomeAssistantEntityID(rec.Name) == entityID {
				return rec.Identity, true
			}
		}
	}
	return TEntityIdentity{}, false
}

// stripLastPathSegment drops identity's own trailing path segment (the node capability's bare
// label itself, e.g. "garage/fuse_cabinet/zwave/042/node" -> ".../042") to get the DEVICE's own
// position from its node ENTITY's position.
func stripLastPathSegment(identity TEntityIdentity) TEntityIdentity {
	path := identity.Path
	if lastSlash := strings.LastIndex(path, "/"); lastSlash >= 0 {
		path = path[:lastSlash]
	} else {
		path = ""
	}
	return TEntityIdentity{Sphere: identity.Sphere, Path: path}
}

// resolveDeviceFriendlyName computes deviceID's own device-level friendly name. positioned is
// false when the device's own node capability isn't positioned in Conceptual.def at all -- the
// caller should fall back to the device's own raw id and the validation below warns.
func resolveDeviceFriendlyName(deviceID string, admin *TAdministrationState, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice) (name string, positioned bool) {
	// Every kind's "node" capability, however it was actually positioned under the hood, is
	// recorded uniformly in DeviceNodeEntityID by registerDeviceCapabilityEntityLinkAllowingHidden
	// itself -- see that map's own doc comment (administration.go) for why this single choke point,
	// rather than re-deriving which of several different internal storage shapes a given kind uses,
	// is the only reliable way to find it (a raw discovery leaf, a "derived ... is available"
	// discovery capability, a Logical.def override, and hosts/import/commandline/local's own
	// AttributeEntityIDs["node"] are four genuinely different storage shapes for the same thing).
	// Checked BEFORE HostIdentity (2026-10-07, reordered from the original): a "hosts" device that
	// EXPLICITLY declares its own "entity binary_sensor.infrastructural:<leaf>/node;" line (e.g.
	// node.mini/node.air_4, which also have cpu attributes needing explicit positioning) gets
	// recorded here too, via the exact same choke point -- and this entity's own identity is
	// already correctly resolved from its own explicit spec text, independent of whether the
	// device header still carries an "as <leaf>" clause at all (confirmed live, 2026-10-07:
	// Vienna's node.air_4 with its header's "as air-4" stripped and its own entities rewritten
	// explicit, e.g. "entity binary_sensor.infrastructural:air-4/node;", still resolves "air-4"
	// correctly here). HostIdentity, built once at positioning time straight from the device
	// header's own raw "as <leaf>" text, is used only as the fallback for a "hosts" device with NO
	// explicit "node" entity of its own at all (a plain ping-only device like netatmo/candy, whose
	// liveness check IS its node, never spelled out as its own Conceptual.def entity line).
	if entityID, hasNode := admin.DeviceNodeEntityID[deviceID]; hasNode {
		if identity, found := entityIdentityByID(admin, entityID); found {
			return friendlyEntityName(stripLastPathSegment(identity)), true
		}
	}

	if link, hasLink := admin.DeviceConceptualLinks[deviceID]; hasLink && link.HostIdentity != (TEntityIdentity{}) {
		return friendlyEntityName(link.HostIdentity), true
	}

	return "", false
}

// fixUpDisplayNameFromNode re-derives TDeviceConceptualLink.DisplayName (the coordinator-facing
// "display_name"/device-grouping text, built once at positioning time by deviceDisplayName,
// Conceptual_DeviceEntities.go) for every device whose header carried NO "as <leaf>" clause at
// all -- deviceDisplayName's own displayLeaf parameter came from that same raw header text
// (registerDevicePositioning's decl.Spec), so a device with none got a DisplayName missing its
// own distinguishing leaf entirely, even once every entity Spaces.def declares under it was
// written out fully explicit (the user's own real case, 2026-10-07: Vienna's node.air_4,
// "device node.air_4 with: entity binary_sensor.infrastructural:air-4/node; ...;", lost
// "display_name: air-4" from coordinator/imported.yaml even though its own entities resolved
// correctly).
//
// Reuses resolveDeviceFriendlyName (above) -- the exact same "derive the device's own position
// from its own node entity" computation PROJECT.md item 8 already built for the device's own
// discovery "device": {"name": ...} block -- rather than re-deriving a second, parallel
// leaf-from-node computation. The two outputs differ only in the leading sphere token
// (friendlyEntityName's own "sphere/path" vs deviceDisplayName's bare, sphere-less path, per the
// user's own 2026-09-24 "a device has no sphere of its own" ruling), stripped here with a single
// SplitN.
//
// Deliberately restricted to devices with a single occurrence and an empty DeviceNamePath -- the
// SAME devices applyImpliedNodePositioning (below) already limits itself to, for the identical
// reason (a device positioned more than once, or whose single occurrence DID carry a leaf, is
// left exactly as deviceDisplayName already computed it, zero behaviour change). Must run AFTER
// applyImpliedNodePositioning/applyColocatedPositioning, so every node this device could possibly
// have -- including an implied one -- is already resolved by the time this reads
// DeviceNodeEntityID.
func fixUpDisplayNameFromNode(admin *TAdministrationState, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice) {
	for deviceID, occurrences := range admin.DevicePositioningOccurrences {
		if len(occurrences) != 1 || occurrences[0].DeviceNamePath != "" {
			continue
		}
		link, hasLink := admin.DeviceConceptualLinks[deviceID]
		if !hasLink {
			continue
		}
		name, positioned := resolveDeviceFriendlyName(deviceID, admin, discoveryGatewaysByID)
		if !positioned {
			continue
		}
		if _, rest, found := strings.Cut(name, "/"); found {
			link.DisplayName = rest
		} else {
			link.DisplayName = ""
		}
		admin.DeviceConceptualLinks[deviceID] = link
	}
}

// findSharedDevicePositions groups every device by the exact (space, leaf) it was positioned at
// (admin.DevicePositioningOccurrences), and returns one warning per group with 2+ members that
// each currently have NO positioned "node" of their own (resolveDeviceFriendlyName) -- i.e. would
// each, if given the bare implied node, collide on the exact same ".../node" entity. A group with
// only ONE such member is NOT a clash risk: its own sibling(s) already claim a DIFFERENT,
// distinguishing leaf for their own node, so the lone remaining member can safely take the bare
// one. "Weaving" onto a shared conceptual position is itself a real, legitimate authoring pattern
// (e.g. two Zigbee devices physically bundled in the same enclosure) -- only the bare-node
// collision itself is the problem this flags. Read-only reporting -- callers decide which suffix
// to use, this only finds the groups that actually need one.
func findSharedDevicePositions(admin *TAdministrationState, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice) []string {
	type positionKey struct {
		space string
		leaf  string
	}
	groups := map[positionKey]map[string]bool{}

	deviceIDs := make([]string, 0, len(admin.DevicePositioningOccurrences))
	for id := range admin.DevicePositioningOccurrences {
		deviceIDs = append(deviceIDs, id)
	}
	sort.Strings(deviceIDs)

	for _, deviceID := range deviceIDs {
		if admin.EventOnlyDeviceIDs[deviceID] {
			continue
		}
		for _, occ := range admin.DevicePositioningOccurrences[deviceID] {
			key := positionKey{space: strings.Join(occ.SpacePath, "/"), leaf: occ.DeviceNamePath}
			if groups[key] == nil {
				groups[key] = map[string]bool{}
			}
			groups[key][deviceID] = true
		}
	}

	keys := make([]positionKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].space != keys[j].space {
			return keys[i].space < keys[j].space
		}
		return keys[i].leaf < keys[j].leaf
	})

	var warnings []string
	for _, key := range keys {
		if len(groups[key]) < 2 {
			continue
		}
		var atRisk []string
		for id := range groups[key] {
			if _, positioned := resolveDeviceFriendlyName(id, admin, discoveryGatewaysByID); !positioned {
				atRisk = append(atRisk, id)
			}
		}
		if len(atRisk) < 2 {
			continue
		}
		sort.Strings(atRisk)
		warnings = append(warnings, fmt.Sprintf("devices %v share conceptual position %q/%q and none has its own node positioned yet -- each needs its own distinguishing leaf, or they'll collide on the same bare \".../node\" entity (PROJECT.md item 8)", atRisk, key.space, key.leaf))
	}
	return warnings
}

// applyColocatedPositioning resolves every Logical.def "colocated <device-id> as <leaf>;" fact
// (TLogicalDevice.Colocated -- see its own doc comment) into a real positioning for the named
// device: whatever space+leaf the ANCHOR device (the one declaring "colocated ...;") was itself
// positioned at in Conceptual.def, plus "/<leaf>" appended. The concrete trigger: Vienna's
// switch.living_room_tv (a Z-Wave relay switching the TV's own power) has no conceptual position
// of its own in Conceptual.def at all, but obviously lives wherever appliance.living_room_tv (the
// TV it switches, positioned "as tv") lives -- "colocated switch.living_room_tv as switch;" on
// appliance.living_room_tv's own Logical.def entry resolves it to "tv/switch", without the author
// having to duplicate the TV's own space/leaf by hand or keep the two in sync across edits.
//
// Gives the colocated target its own "node" capability entity directly (registerDeviceCapability-
// EntityLink, same call applyImpliedNodePositioning used to make for it, now inlined here instead
// -- PROJECT.md's own 2026-10-08 two-step plan drops that mechanism's implied-inference sweep
// entirely, so every positioned device, colocated or not, must come away from THIS pass already
// self-sufficient).
//
// The anchor itself must be positioned in EXACTLY one space (same "there must be only one
// possible answer" rule applyImpliedNodePositioning already applies) -- an anchor positioned
// nowhere, or in more than one space, makes "the same position, plus a suffix" ambiguous, and is
// reported instead of guessed. A colocated target that already has its OWN real position (a
// genuine authoring conflict, not the use case this exists for) is also reported, not silently
// double-registered.
func applyColocatedPositioning(admin *TAdministrationState, logicalDevicesByID map[string]TLogicalDevice, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, hostDevicesByID map[string]THostDevice, commandlineDevicesByID map[string]TCommandlineDevice, localDevicesByID map[string]TLocalDevice, entitiesPath string) []string {
	var warnings []string

	anchorIDs := make([]string, 0, len(logicalDevicesByID))
	for id := range logicalDevicesByID {
		anchorIDs = append(anchorIDs, id)
	}
	sort.Strings(anchorIDs)

	for _, anchorID := range anchorIDs {
		anchor := logicalDevicesByID[anchorID]
		if len(anchor.Colocated) == 0 {
			continue
		}

		anchorOccurrences := admin.DevicePositioningOccurrences[anchorID]
		if len(anchorOccurrences) != 1 {
			warnings = append(warnings, fmt.Sprintf("device %q declares \"colocated ...;\" but is itself positioned in %d space(s) (need exactly 1) -- cannot derive a colocated position for it", anchorID, len(anchorOccurrences)))
			continue
		}
		occ := anchorOccurrences[0]

		for _, fact := range anchor.Colocated {
			if existing := admin.DevicePositioningOccurrences[fact.DeviceID]; len(existing) > 0 {
				warnings = append(warnings, fmt.Sprintf("device %q is declared \"colocated\" with %q, but already has its own positioning in Conceptual.def -- remove one or the other", fact.DeviceID, anchorID))
				continue
			}

			// occ.DeviceNamePath is the anchor's own raw "as <leaf>" header text -- empty once
			// that clause is gone, even when the anchor's own entities (including its "node")
			// were rewritten fully explicit. Prefer deriving the anchor's own leaf from its
			// already-resolved "node" entity instead (the same DeviceNodeEntityID choke point
			// resolveDeviceFriendlyName above already trusts), stripping both the trailing
			// "/node" segment and the enclosing space's own context prefix -- falling back to the
			// raw header text only when the anchor's own node isn't resolved yet (e.g. it is
			// ITSELF only implied later, after this runs).
			anchorLeaf := occ.DeviceNamePath
			if entityID, hasNode := admin.DeviceNodeEntityID[anchorID]; hasNode {
				if identity, found := entityIdentityByID(admin, entityID); found {
					_, contextPath := normalizeSpaceContext(occ.SpacePath)
					leafPath := stripLastPathSegment(identity).Path
					if contextPath != "" {
						if leafPath == contextPath {
							leafPath = ""
						} else {
							leafPath = strings.TrimPrefix(leafPath, contextPath+"/")
						}
					}
					anchorLeaf = leafPath
				}
			}

			newLeaf := fact.LeafSuffix
			if anchorLeaf != "" {
				newLeaf = anchorLeaf + "/" + fact.LeafSuffix
			}

			savedSpacePath := admin.SpacePath
			admin.SpacePath = occ.SpacePath
			positioningWarnings := registerDevicePositioning(admin, TDevicePositioningDeclaration{DeviceID: fact.DeviceID, Spec: newLeaf}, discoveryGatewaysByID, hostDevicesByID, hassBridgeDevicesByID, importedDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID, entitiesPath, 0)
			// Self-sufficiently give the colocated target its own "node" entity here too, rather
			// than counting on applyImpliedNodePositioning's later single-occurrence sweep to do it
			// -- that sweep is going away (PROJECT.md's own 2026-10-08 two-step plan: every used
			// device must position its own node explicitly, no implied inference left at all). A
			// colocated target has no Conceptual.def presence of its own to make this "explicit" in
			// the usual sense, but the "colocated" fact itself (plus the anchor's own real position)
			// already IS the author's explicit statement of where it is -- this just turns that into
			// a real node capability entity, the same way the fact's own positioning line above does.
			//
			// But skip it if the target's own "node" is ALREADY resolved via some other path --
			// real case found live: Vienna's node.candy is BOTH a colocated target (anchor
			// appliance.washing_machine) AND literally referenced by name inside another device's
			// own "derived binary_sensor.node with: condition node.candy ...;" clause, which already
			// resolves (and registers) node.candy's own "node" capability as a side effect, earlier
			// in the parse -- via autoMaterializePhysicalCapability's "local kind" branch
			// (Conceptual_LogicalEntities.go), which populates link.AttributeEntityIDs["node"], NOT
			// admin.DeviceNodeEntityID -- checking DeviceNodeEntityID alone missed this and
			// collided ("already used as a different conceptual entity"); resolveDeviceFriendlyName
			// is the one existing helper that already checks BOTH lookup paths correctly (the exact
			// same check applyImpliedNodePositioning's own sweep relies on for this).
			if _, alreadyPositioned := resolveDeviceFriendlyName(fact.DeviceID, admin, discoveryGatewaysByID); !alreadyPositioned {
				decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "binary_sensor.infrastructural:node", DeviceID: fact.DeviceID, Capability: "node"}
				implyWarnings, deferred := registerDeviceCapabilityEntityLink(admin, decl, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID, entitiesPath, 0, true, newLeaf, true)
				if deferred || len(implyWarnings) > 0 {
					warnings = append(warnings, fmt.Sprintf("colocated device %q: could not give it its own \"node\" capability (%v)", fact.DeviceID, implyWarnings))
				}
			}
			admin.SpacePath = savedSpacePath

			warnings = append(warnings, positioningWarnings...)
		}
	}

	return warnings
}

// triggerLogicalOwnNodeCapabilities gives every device whose Logical.def entry declares its own
// "node" capability (absorbed, defined, or derived -- e.g. Vienna's appliance.picture_frame/
// buttons.apartment_living_room_kitchen_bj, each "derived binary_sensor.node with: condition
// ...;") a chance to actually register it, when nothing has yet. This is NOT guessing: the
// capability is already genuinely, explicitly declared -- it just has no literal Conceptual.def
// "entity ...;" line of its own to trigger registerDeviceCapabilityEntityLink's ordinary per-line
// dispatch from (backfill_explicit_node_positioning.go deliberately excludes these devices from
// ever being given one, since that collides with this exact mechanism -- see its own doc comment).
// applyImpliedNodePositioning (removed, PROJECT.md's own 2026-10-08 two-step plan) used to trigger
// this as an incidental side effect of its own single-occurrence GUESSING sweep for devices with NO
// declared node capability at all; dropping that sweep entirely also silently dropped this
// legitimate trigger for devices that DO have one declared, a real regression caught live (Vienna:
// both devices above newly failed validateDevicesHaveFriendlyNames's hard rule, below, once the
// guessing sweep was gone) -- this function is the narrow, guess-free replacement for just that
// part. Deliberately silent on anything it can't resolve (ambiguous/missing positioning, a
// genuinely unresolvable sibling reference) -- validateDevicesHaveFriendlyNames reports the real
// problem either way.
func triggerLogicalOwnNodeCapabilities(admin *TAdministrationState, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, hostDevicesByID map[string]THostDevice, commandlineDevicesByID map[string]TCommandlineDevice, localDevicesByID map[string]TLocalDevice, logicalDevicesByID map[string]TLogicalDevice, entitiesPath string) {
	ids := make([]string, 0, len(logicalDevicesByID))
	for id, dev := range logicalDevicesByID {
		if _, hasNode := dev.Capabilities["node"]; hasNode {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	for _, id := range ids {
		if _, positioned := resolveDeviceFriendlyName(id, admin, discoveryGatewaysByID); positioned {
			continue
		}
		occurrences := admin.DevicePositioningOccurrences[id]
		if len(occurrences) != 1 {
			continue
		}
		occ := occurrences[0]
		// occ.DeviceNamePath is the device's own header "as <leaf>" text -- empty once
		// flatten_conceptual_positions.go has stripped it, even though the device's own OTHER
		// entities (siblings under the same header) still carry that leaf explicitly in their own
		// rewritten specs. Falling back to deriveDeviceOwnLeafFromSiblings in that case makes this
		// registration "as"-independent, the same way every other capability kind (node/discovery/
		// absorbed) already is -- real case found live: Vienna's appliance.picture_frame/
		// buttons.apartment_living_room_kitchen_bj/appliance.washing_machine, each with no node
		// entity line of its own (only a Logical.def "derived binary_sensor.node" capability), lost
		// their own leaf entirely once their header's "as" clause was gone.
		deviceNamePath := occ.DeviceNamePath
		if deviceNamePath == "" {
			deviceNamePath = deriveDeviceOwnLeafFromSiblings(admin, id)
		}
		savedSpacePath := admin.SpacePath
		admin.SpacePath = occ.SpacePath
		decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "binary_sensor.infrastructural:node", DeviceID: id, Capability: "node"}
		registerDeviceCapabilityEntityLink(admin, decl, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID, entitiesPath, 0, true, deviceNamePath, true)
		admin.SpacePath = savedSpacePath
	}
}

// deriveDeviceOwnLeafFromSiblings recovers deviceID's own leaf (the text that USED to come from
// its header's "as <leaf>" clause) from one of its OWN sibling entities' already-resolved
// Identity, for when that header carries none any more. Every sibling's Identity.Path is already
// fully resolved by the time this runs (the main Conceptual.def parse is complete), independent of
// whether the SOURCE spec text was an implicit fold or already fully explicit -- so this works
// identically whether or not flatten_conceptual_positions.go has touched the file yet. Mirrors
// backfill_explicit_node_positioning.go's own sibling-fallback passes (same reasoning, simplified
// to the two cases that matter here: a bare sibling's remainder IS the device's own leaf outright;
// otherwise strip the sibling's own known capability tail from its remainder).
func deriveDeviceOwnLeafFromSiblings(admin *TAdministrationState, deviceID string) string {
	provenanceDeviceID := regexp.MustCompile(` from (\S+)`)
	// capabilityLabelPattern extracts a sibling's own BARE capability label from provenance's own
	// trailing token -- NOT deviceSpecLeafPath(pm[3]) (the spec TEXT), which is unreliable here:
	// once flatten_conceptual_positions.go has already run, every sibling's own spec text already
	// has the device's leaf baked directly into it (e.g. "sensor.infrastructural:bj/linkquality",
	// not bare "linkquality" any more), so subtracting ITS OWN "bareSpec" from Identity.Path's
	// remainder is a no-op (they're now the same string) -- real case found live: Vienna's
	// buttons.apartment_living_room_kitchen_bj's own "linkquality" sibling, post-flatten, silently
	// returned "" instead of "bj". The capability LABEL (decl.Capability, resolved via longest-
	// suffix matching against the device's own KNOWN Physical.def capabilities) never absorbs the
	// device leaf this way regardless of flattening, same choke point
	// backfill_explicit_node_positioning.go's own backfillCapabilityLabelPattern already relies on.
	// "(?:with )?" handles a bare no-rename-shorthand capability's own provenance shape (e.g.
	// "... from buttons.apartment_living_room_kitchen_bj with linkquality"), confirmed live --
	// without it, this simply fails to match at all (capLabel stays empty), silently routing a
	// perfectly good "linkquality" sibling into the "bare" bucket instead, where its own FULL
	// remainder ("bj/linkquality") got used as the device's leaf verbatim.
	capabilityLabelPattern := regexp.MustCompile(`from \S+ (?:with )?(\S+)$`)
	spaceNameForRecord := map[string]string{}
	for spaceName, records := range admin.EntityRecordsBySpace {
		for _, rec := range records {
			spaceNameForRecord[rec.Provenance] = spaceName
		}
	}

	type candidate struct {
		remainder string
		capLabel  string
		sphere    string
	}
	var bare, withTail []candidate
	// atSpace holds the domains of the siblings positioned at their space's own name (e.g. a bare
	// "light.social:"): the last resort to name the node after (see the end of this function).
	var atSpace []string
	for _, records := range admin.EntityRecordsBySpace {
		for _, rec := range records {
			// A "physical" sphere entity (e.g. "entity event.physical: from event.pressed;")
			// excludes autoMaterializePhysicalCapability's own synthetic shadow entities (always
			// sphere "physical", Path == some OTHER device's own id verbatim, e.g. "node.candy") --
			// those carry a DIFFERENT device's own identity under THIS device's provenance text
			// (see flatten_conceptual_positions.go's own matching exclusion, same root cause and
			// same "Path contains a dot" discriminator: every device id has the dotted
			// "domain.identifier" form, which an ordinary space path never does). A hand-authored
			// "physical" sphere sibling (e.g. kitchen_bj's own "event.physical: from event.pressed;",
			// Identity.Path "apartment/living_room/kitchen/bj" -- a normal, dot-free space path) is
			// NOT excluded by this check and remains a candidate, same as any other sphere.
			if rec.Identity.Sphere == "physical" && strings.Contains(rec.Identity.Path, ".") {
				continue
			}
			if m := provenanceDeviceID.FindStringSubmatch(rec.Provenance); m == nil || m[1] != deviceID {
				continue
			}
			pm := conceptualProvenancePattern.FindStringSubmatch(rec.Provenance)
			if pm == nil {
				continue
			}
			contextPath := spaceContextPath(spaceNameForRecord[rec.Provenance])
			remainder := rec.Identity.Path
			if contextPath != "" {
				if remainder == contextPath {
					remainder = ""
				} else {
					remainder = strings.TrimPrefix(remainder, contextPath+"/")
				}
			}
			if remainder == "" {
				atSpace = addIfAbsent(atSpace, rec.Identity.Domain)
				continue
			}
			// pm[3] (conceptualProvenancePattern's own \S+ capture) is ALWAYS just decl.LocalSpec's
			// own single token -- it never includes a trailing " from <source>" at all (that's a
			// SEPARATE part of provenance's own construction, "from <deviceID> <Capability>", not
			// part of the captured spec). So declaredLeaf below is this entity's own FULL declared
			// leaf text regardless of "from"-shape.
			declaredLeaf := strings.TrimPrefix(deviceSpecLeafPath(pm[3]), ":")
			capLabel := ""
			if cm := capabilityLabelPattern.FindStringSubmatch(rec.Provenance); cm != nil {
				capLabel = cm[1]
			}
			// The real discriminator: does declaredLeaf's own LAST PATH SEGMENT have a genuine
			// suffix relationship with capLabel (either direction)? For the bare-shorthand
			// convention (e.g. kitchen_bj's "linkquality"), capLabel was inferred FROM that exact
			// trailing text via longest-suffix matching, so it always relates this way --
			// declaredLeaf = device-leaf + "/" + capLabel. The SAME relationship holds even when
			// capLabel is a LONGER, more specific sibling name than declaredLeaf's own trailing
			// segment -- real case found live: Junglinster's utility.heater ("sensor.physical:
			// garage_door/temperature from sensor.outdoor_temperature": trailing segment
			// "temperature" IS a suffix of capLabel "outdoor_temperature", even though capLabel
			// itself is no suffix of the full declaredLeaf -- confirming "garage_door" alone, not
			// "garage_door/temperature", is this device's own true leaf). For a genuine "from
			// <source>" cross-reference with NO such relationship at all (e.g. picture_frame's
			// "switch.slideshow", a tap device's "pressed" -- neither shares any suffix with
			// "picture_frame" or "tap"), declaredLeaf (== remainder here) is already this entity's
			// own complete, self-contained contribution.
			lastSegment := declaredLeaf
			if idx := strings.LastIndex(declaredLeaf, "/"); idx >= 0 {
				lastSegment = declaredLeaf[idx+1:]
			}
			related := capLabel != "" && (lastSegment == capLabel || strings.HasSuffix(capLabel, lastSegment) || strings.HasSuffix(lastSegment, capLabel))
			switch {
			case declaredLeaf == "":
				bare = append(bare, candidate{remainder: remainder, sphere: rec.Identity.Sphere})
			case related:
				if remainder == lastSegment || strings.HasSuffix(remainder, "/"+lastSegment) {
					withTail = append(withTail, candidate{remainder: remainder, capLabel: lastSegment})
				}
			default:
				if remainder == declaredLeaf {
					bare = append(bare, candidate{remainder: remainder, sphere: rec.Identity.Sphere})
				}
			}
		}
	}
	// Deterministic, same preference order as backfill_explicit_node_positioning.go's own
	// sibling-fallback passes: a bare sibling (nothing of its own folded into the leaf) is
	// unambiguous, preferring sphere "infrastructural"; otherwise any bare sibling; otherwise
	// strip a sibling's own known capability tail from its remainder.
	sort.Slice(bare, func(i, j int) bool { return bare[i].remainder < bare[j].remainder })
	for _, c := range bare {
		if c.sphere == "infrastructural" {
			return c.remainder
		}
	}
	if len(bare) > 0 {
		return bare[0].remainder
	}
	sort.Slice(withTail, func(i, j int) bool { return withTail[i].remainder < withTail[j].remainder })
	for _, c := range withTail {
		leaf := strings.TrimSuffix(strings.TrimSuffix(c.remainder, c.capLabel), "/")
		if leaf != "" {
			return leaf
		}
	}
	// Every sibling stands for its space itself (PROJECT.md 17g, 2026-10-26): name the node after
	// what that sibling is -- its own leaf ("temperature" for "sensor.social:temperature"), else
	// its domain ("light" for a bare "light.social:") -- instead of leaving it at the space's own
	// name, where it wouldn't say what it is the node of.
	if len(withTail) > 0 {
		return withTail[0].capLabel
	}
	if len(atSpace) > 0 {
		sort.Strings(atSpace)
		return atSpace[0]
	}
	return ""
}

// validateDevicesHaveFriendlyNames reports every positioned device (any kind, any
// DeviceConceptualLinks entry, or any discovery gateway with at least one live capability) whose
// own node capability isn't positioned -- resolveDeviceFriendlyName has nothing to build a real
// friendly name from, and the fallback (the device's own raw id) is exactly what this item exists
// to stop showing to the user. A HARD rule (generateFromPaths aborts if this returns anything at
// all) since PROJECT.md's own 2026-10-08 two-step plan: every used device must position its own
// node explicitly, no implied inference left to paper over a gap silently -- applyImpliedNode-
// Positioning (removed, used to run right before this) used to auto-supply the single-occurrence
// case; every device it could ever successfully supply now has an explicit "node" entity of its
// own instead (backfill_explicit_node_positioning.go, run once per house), so this should trigger
// only for a genuinely new, real gap from here on.
func validateDevicesHaveFriendlyNames(admin *TAdministrationState, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice) []string {
	var warnings []string

	candidates := map[string]bool{}
	for id := range admin.DeviceConceptualLinks {
		candidates[id] = true
	}
	for _, link := range admin.DiscoveryEntityLinks {
		candidates[link.GatewayDeviceID] = true
	}

	deviceIDs := make([]string, 0, len(candidates))
	for id := range candidates {
		deviceIDs = append(deviceIDs, id)
	}
	sort.Strings(deviceIDs)
	for _, id := range deviceIDs {
		if admin.EventOnlyDeviceIDs[id] {
			continue
		}
		if _, positioned := resolveDeviceFriendlyName(id, admin, discoveryGatewaysByID); !positioned {
			warnings = append(warnings, fmt.Sprintf("device %q has no positioned \"node\" capability -- its own device friendly name falls back to its raw id instead of its real conceptual position; position its node (PROJECT.md item 8)", id))
		}
	}
	return warnings
}
