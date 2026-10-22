/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualLocalEntities
 *
 * Registers a "local" device's own capabilities (integration_local_storage.go's TLocalCapability)
 * once Conceptual.def positions/references them -- the "local" kind's own counterpart to
 * registerHostCapabilityEntityLink/registerHassBridgeAttributeEntity. Capability shapes (see
 * integration_local_parser.go's own header comment for the full grammar):
 *
 *   - A bare capability ("camera.core;") is nothing more than an acknowledgement that such an
 *     entity may exist on HA main -- registered exactly like a plain bare "entity <spec>;"
 *     declaration (HasDefinitionOrImport left false, the zero value), so kind-5's existence
 *     checking (main_entities.go's collectMainEntityIDs) confirms it's genuinely there, the same
 *     discipline the Ring doorbell's own scattered bare entities already had before this
 *     migration.
 *   - An "is available" capability ("binary_sensor.node camera.core is available;") tracks a
 *     SIBLING capability's own resolved entity_id -- reuses registerDiscoveryAvailabilityEntityLink's
 *     exact condition-building trick (parseConditionDirective("condition "+sibling+" is
 *     available", nil)), but looks the sibling up via DeviceConceptualLinks[deviceID].
 *     AttributeEntityIDs (the same per-device map hassbridge/hosts capabilities already populate),
 *     not a discovery gateway's own Capabilities/DiscoveryEntityLinks bookkeeping -- a "local"
 *     device has no raw MQTT leaves to reverse-lookup against. This is a generator-authored
 *     condition entity, so HasDefinitionOrImport: true (learned directly from the bug found and
 *     fixed 2026-09-24 in registerDiscoveryAvailabilityEntityLink/registerDiscoveryDerivedEntityLink
 *     for the exact same omission -- see Conceptual_DiscoveryEntities.go's own header comment).
 *   - A "derived" capability ("derived binary_sensor.battery_alert from sensor.battery_level via
 *     jinja ${int_less_then}(20);", TLocalCapability.DerivedFromCapability/DerivedViaTemplate,
 *     2026-09-30) is the SAME mechanics as "is available" just above -- a condition over a sibling's
 *     own AttributeEntityIDs-resolved entity_id -- generalised from the fixed "is available" check
 *     to an arbitrary author-supplied template, mirroring registerDiscoveryDerivedEntityLink exactly
 *     (ConditionSources/ConditionExpr built directly, "$" substituted for "$1"). Added to close a
 *     real gap: a "local" device's battery_alert could previously only ever be expressed as a
 *     Logical.def "derived" capability, which physical_rule_battery_alert_biconditional.go's Rule 2
 *     deliberately never looks at (see that file's own header).
 *
 * All three shapes populate DeviceConceptualLinks[deviceID].AttributeEntityIDs[label] on success,
 * so Logical.def's existing same-device/cross-device sibling resolution
 * (resolveLogicalSiblingSourceEntityID, Conceptual_LogicalEntities.go) picks up a "local" device's
 * capabilities for free -- no changes needed on that side at all, including for "node" specifically
 * (resolveLogicalSiblingSourceEntityID's own tier-2 check falls through to AttributeEntityIDs["node"]
 * whenever DeviceConceptualLinks.NodeEntityID is empty, exactly as hassbridge's own "node" capability
 * already relies on).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 30.09.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// registerLocalCapabilityEntityLink is registerDeviceCapabilityEntityLinkAllowingHidden's "local"-
// kind branch. finalAttempt/deferred follow the same convention as every other deferred-capable
// branch in this codebase: the parser reads the file once, so a capability line (or this device's
// own auto-registered "node") can be reached before either its own positioning or (for the "is
// available" shape) its sibling's positioning has happened yet; deferred=true with no warning lets
// the single retry (after the whole file is read) resolve it once everything has actually landed.
func registerLocalCapabilityEntityLink(administration *TAdministrationState, decl TDeviceCapabilityEntityDeclaration, capability TLocalCapability, bareName, deviceNamePath, entitiesPath string, lineNum int, hasNodeCapability, finalAttempt bool) (warnings []string, deferred bool) {
	provenance := fmt.Sprintf("%s:%d → %s from %s %s", filepath.Base(entitiesPath), lineNum, decl.LocalSpec, decl.DeviceID, decl.Capability)

	fullName := resolveDeviceEntityFullName(decl.LocalSpec, administration.SpacePath, deviceNamePath)
	identity := extractEntityIdentity(fullName)
	if identity.Domain == "" {
		return []string{fmt.Sprintf("%s: could not resolve a domain from %q; skipping", provenance, decl.LocalSpec)}, false
	}
	entityID := toHomeAssistantEntityID(fullName)
	if entityID == "" {
		return []string{fmt.Sprintf("%s: could not resolve a Home Assistant entity id from %q; skipping", provenance, decl.LocalSpec)}, false
	}

	if capability.MediaPlayerEntity != "" {
		return registerLocalMediaSwitchCapability(administration, decl, capability, fullName, identity, entityID, bareName, provenance, hasNodeCapability, finalAttempt)
	}

	link, hasLink := administration.DeviceConceptualLinks[decl.DeviceID]
	if !hasLink {
		if !finalAttempt {
			return nil, true
		}
		return []string{fmt.Sprintf("%s: device %q has no \"device.<spec> from %s;\" positioning yet -- add one (any space) before referencing one of its entities directly", provenance, decl.DeviceID, decl.DeviceID)}, true
	}
	if link.AttributeEntityIDs == nil {
		link.AttributeEntityIDs = map[string]TDeviceAttributeLink{}
	}
	spaceName := administration.CurrentSpaceName()

	if capability.AvailabilityOf != "" {
		siblingAttr, hasSibling := link.AttributeEntityIDs[capability.AvailabilityOf]
		if !hasSibling {
			if !finalAttempt {
				return nil, true
			}
			return []string{fmt.Sprintf("%s: sibling capability %q has no positioning yet -- add one before referencing its availability", provenance, capability.AvailabilityOf)}, true
		}
		sources, expr := parseConditionDirective("condition "+siblingAttr.EntityID+" is available", nil)
		if expr == "" {
			return []string{fmt.Sprintf("%s: could not build a condition from sibling entity %q", provenance, siblingAttr.EntityID)}, false
		}
		deviceClass, _, _, icon := resolveCapabilityDefaults(administration.CapabilityDefaults, identity.Domain, identity.Path)
		administration.AppendEntityRecord(spaceName, TEntityRecord{
			Name:              fullName,
			SourceKey:         "physical:" + decl.DeviceID + "!" + bareName,
			Identity:          identity,
			Provenance:        provenance,
			ConditionSources:  sources,
			ConditionExpr:     expr,
			ConditionDevClass: deviceClass,
			EntityIcon:        icon,
			NoCollect:         decl.NoCollect,
			// Generator-authored condition entity -- see this file's own header comment.
			HasDefinitionOrImport: true,
		})
	} else if capability.DerivedFromCapability != "" {
		// "derived DDD.NNN from EEE.MMM via TTT;" (2026-09-30, see this file's own TLocalCapability
		// reference and integration_local_storage.go's header comment) -- the exact same condition-
		// over-a-sibling's-own-resolved-entity mechanics as registerDiscoveryDerivedEntityLink
		// (Conceptual_DiscoveryEntities.go: ConditionSources/ConditionExpr built directly, "$"
		// substituted for "$1"), just resolving the sibling via this device's own AttributeEntityIDs
		// (a "local" device has no raw MQTT leaves to reverse-lookup against, unlike a discovery
		// gateway).
		siblingAttr, hasSibling := link.AttributeEntityIDs[capability.DerivedFromCapability]
		if !hasSibling {
			if !finalAttempt {
				return nil, true
			}
			return []string{fmt.Sprintf("%s: sibling capability %q has no positioning yet -- add one before deriving from it", provenance, capability.DerivedFromCapability)}, true
		}
		expr := strings.ReplaceAll(capability.DerivedViaTemplate, "$", "$1")
		deviceClass, _, _, icon := resolveCapabilityDefaults(administration.CapabilityDefaults, identity.Domain, identity.Path)
		administration.AppendEntityRecord(spaceName, TEntityRecord{
			Name:              fullName,
			SourceKey:         "physical:" + decl.DeviceID + "!" + bareName,
			Identity:          identity,
			Provenance:        provenance,
			ConditionSources:  []string{siblingAttr.EntityID},
			ConditionExpr:     expr,
			ConditionDevClass: deviceClass,
			EntityIcon:        icon,
			NoCollect:         decl.NoCollect,
			// Generator-authored condition entity -- see this file's own header comment.
			HasDefinitionOrImport: true,
		})
	} else {
		// Bare capability -- see this file's own header comment: registered exactly like a plain
		// bare "entity <spec>;" declaration, HasDefinitionOrImport left false (the zero value).
		administration.AppendEntityRecord(spaceName, TEntityRecord{
			Name:       fullName,
			SourceKey:  "physical:" + decl.DeviceID + "!" + bareName,
			Identity:   identity,
			Provenance: provenance,
			NoCollect:  decl.NoCollect,
		})
	}

	link.AttributeEntityIDs[bareName] = TDeviceAttributeLink{EntityID: entityID, Identity: identity}
	administration.DeviceConceptualLinks[decl.DeviceID] = link
	return nil, false
}

// registerLocalMediaSwitchCapability is registerLocalCapabilityEntityLink's own dispatch for a
// "switch.<label> <media-player-entity> [with: no_play_input \"<value>\"; end;];" capability --
// the "local" kind's counterpart to registerLogicalMediaSwitchCapability
// (Conceptual_LogicalEntities.go), relocated here 2026-09-27 (PROJECT.md: the Logical layer should
// only ever define genuinely derived entities; a bare domain relabel of one already-existing
// literal local entity isn't that). Reuses generateMediaSwitches' own rendering wholesale, exactly
// like the Logical.def-sourced version already did -- this only ever needs to build the same plain
// TEntityRecord shape, never a bespoke YAML writer of its own.
//
// hasNodeCapability/finalAttempt mirror registerLogicalMediaSwitchCapability's own identically-
// reasoned deferral exactly: the sibling "node" capability (this SAME local device's own, declared
// earlier or later in the same "with: ... end;" block) may not have resolved into
// DeviceConceptualLinks[deviceID].AttributeEntityIDs["node"] yet on the first pass -- defer (no
// warning) while the device declares one and it hasn't resolved, exactly once, via the standard
// end-of-file retry.
func registerLocalMediaSwitchCapability(administration *TAdministrationState, decl TDeviceCapabilityEntityDeclaration, capability TLocalCapability, fullName string, identity TEntityIdentity, entityID, bareName, provenance string, hasNodeCapability, finalAttempt bool) (warnings []string, deferred bool) {
	sourceDomain, _, _ := strings.Cut(capability.MediaPlayerEntity, ".")
	if capability.Domain != "switch" || sourceDomain != "media_player" {
		return []string{fmt.Sprintf("%s: local capability %q coerces domain %q from %q, which isn't a supported coercion yet (only switch<-media_player, the \"media_switch\" shape, is built) -- skipping", provenance, bareName, capability.Domain, capability.MediaPlayerEntity)}, false
	}

	link, hasLink := administration.DeviceConceptualLinks[decl.DeviceID]
	if !hasLink {
		if !finalAttempt {
			return nil, true
		}
		return []string{fmt.Sprintf("%s: device %q has no \"device.<spec> from %s;\" positioning yet -- add one (any space) before referencing one of its entities directly", provenance, decl.DeviceID, decl.DeviceID)}, true
	}

	// Reverse lookup: which entity_id did this SAME device's own "node" capability resolve to?
	// LogicalEntityLinks is checked FIRST, matching registerLogicalMediaSwitchCapability's own
	// identical lookup exactly: a device whose "node" is a Logical.def override (the "node
	// capability lending/override" special-case, Conceptual_DeviceCapabilityEntities.go) resolves
	// there, not here -- real bug found live 2026-09-27 migrating appliance.tv_living_room's own
	// switch.media: checking ONLY AttributeEntityIDs picked up autoMaterializePhysicalCapability's
	// own "physical <id>" shadow node entity instead (a real entity, just the WRONG one -- this
	// device's own condition tree references "physical appliance.tv_living_room" too, so that
	// shadow entity genuinely exists, it's just never the one Conceptual.def's own "node"-named
	// positioning resolves to once the override applies). AttributeEntityIDs stays the fallback
	// for a "local" device with NO Logical.def override at all (e.g. the Ring doorbell), matching
	// this file's own header comment ("resolveLogicalSiblingSourceEntityID's own tier-2 check
	// falls through ... exactly as hassbridge's own 'node' capability already relies on").
	// See this function's own doc comment for why it may not have resolved yet.
	// Collects EVERY match rather than breaking on the first (map iteration order is randomised
	// per Go process, which would otherwise make this non-deterministic across regenerates
	// whenever more than one entry genuinely matches -- a real, pre-existing bug found live
	// 2026-09-27 in registerLogicalMediaSwitchCapability's own identical loop, confirmed by two
	// consecutive ./generate runs producing different output for an unrelated device with no
	// code change in between). Picking the lexicographically-first match is an arbitrary but
	// STABLE tie-break -- correctness of which match is "right" when there's more than one is a
	// separate, pre-existing question this function doesn't attempt to resolve.
	var nodeEntityIDCandidates []string
	for id, entityLink := range administration.LogicalEntityLinks {
		if entityLink.DeviceID == decl.DeviceID && entityLink.Label == "node" {
			nodeEntityIDCandidates = append(nodeEntityIDCandidates, id)
		}
	}
	sort.Strings(nodeEntityIDCandidates)
	var nodeEntityID string
	if len(nodeEntityIDCandidates) > 0 {
		nodeEntityID = nodeEntityIDCandidates[0]
	} else {
		nodeEntityID = link.AttributeEntityIDs["node"].EntityID
	}
	if hasNodeCapability && nodeEntityID == "" && !finalAttempt {
		return nil, true
	}

	spaceName := administration.CurrentSpaceName()
	administration.AppendEntityRecord(spaceName, TEntityRecord{
		Name:                            fullName,
		SourceKey:                       "physical:" + decl.DeviceID + "!" + bareName,
		Identity:                        identity,
		Provenance:                      provenance,
		MediaSwitchPlayerName:           capability.MediaPlayerEntity,
		MediaSwitchNoPlayInput:          capability.NoPlayInput,
		MediaSwitchAvailabilityEntityID: nodeEntityID,
		NoCollect:                       decl.NoCollect,
		HasDefinitionOrImport:           true,
	})

	if link.AttributeEntityIDs == nil {
		link.AttributeEntityIDs = map[string]TDeviceAttributeLink{}
	}
	link.AttributeEntityIDs[bareName] = TDeviceAttributeLink{EntityID: entityID, Identity: identity}
	administration.DeviceConceptualLinks[decl.DeviceID] = link
	return nil, false
}
