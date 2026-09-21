/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualDevicePositioning
 *
 * Parses and registers the conceptual layer's "device <spec> from <device-id>;" construct --
 * historically a lighter-weight alternative to the "entity device.<spec> from <device-id> with:
 * all entities;" bulk form (retired 2026-09-01, see this file's own note below), now the only way
 * to position a device. Intentionally doesn't bulk-register every capability -- it only:
 * (a) establishes the device's shared conceptual identity (DisplayName/ConstantAttributes, used
 * for the HA discovery "device:" block every one of its entities shares), and (b) auto-registers
 * its "node" (liveness)
 * entity -- for a "home_assistant" bridge device, only if it has declared one (a hassbridge
 * device's node is an optional capability, same as any other); for a "hosts" device, always (its
 * node is unconditional, registerHostNodeEntity, Conceptual_DeviceEntities.go) -- the same way an
 * explicit "entity binary_sensor.<spec>/node from <device-id> binary_sensor.node;"
 * (Conceptual_DeviceCapabilityEntities.go) would. Every other entity is expected to be
 * individually positioned via that same per-capability construct.
 *
 * Registered the moment ParseEntitiesAndFillAdministration's single main loop (parser.go) reaches
 * this line -- no separate pre-pass or second scan of the file. A "device <spec> from <device-id>;"
 * line appearing *after* something that references it (an "entity ... from <device-id> entity
 * ...;" / "entity ... as ... from <device-id>;" line, Conceptual_DeviceCapabilityEntities.go /
 * Conceptual_DeviceSourceEntities.go) still works: registerDeviceSourceEntityLink's own
 * "not positioned yet" check reports deferred=true rather than a warning when reached too early,
 * and the parser retries that small in-memory list once, after the whole file has been read --
 * order-independent without ever re-scanning raw text (see registerDeviceSourceEntityLink's doc
 * comment, and PROJECT.md/feedback memory: a second scan of the same file caused a real bug here,
 * 2026-08-27 -- EnsureSpaceRegistered's idempotency check got fooled when a scratch state machine
 * wrote into the real administration state ahead of the real OpenSpace call for the same space).
 *
 * The bulk "all entities" form (and the standalone "for <device-id>: ... end;" shorthand it
 * coexisted with) was retired 2026-09-01, once every caller (Junglinster and Vienna's real
 * Spaces.def included) had migrated to this file's own bare positioning plus the merged
 * "device <spec> from <device-id> with: <entity-spec>; ...; end;" block
 * (deviceWithBlockHeaderPattern below + Conceptual_DeviceCapabilityEntities.go's
 * expandForDeviceShorthandLine) -- see PROJECT.md's unification plan. Conceptual_DeviceEntities.go
 * now holds only the shared registration engine (registerHostAttributeEntity/
 * registerHostNodeEntity/registerHassBridgeAttributeEntity), no parsing/dispatch of its own.
 *
 * Scope: "home_assistant" bridge devices (native + hassbridge-import) and, since 2026-09-01,
 * "hosts" devices too (registerHostDevicePositioning, reusing registerHostNodeEntity --
 * Conceptual_DeviceEntities.go's own materialization machinery, not a separate implementation).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 27.08.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"regexp"
)

// TDevicePositioningDeclaration is one parsed "device <spec> from <device-id>;" line.
type TDevicePositioningDeclaration struct {
	Spec     string // "infrastructural:netatmo" -- an ordinary <sphere>:<path> spec, same shape device.<spec> has minus the "device." domain prefix
	DeviceID string // "hass.davids_bedroom" from "from hass.davids_bedroom"
}

var devicePositioningPattern = regexp.MustCompile(`^device\s+(\S+)\s+from\s+(\S+);$`)

// extractDevicePositioningDeclaration recognises the "device <spec> from <device-id>;" shape --
// distinguished from every "entity ..."-prefixed construct by having no "entity" keyword at all.
func extractDevicePositioningDeclaration(line string) (*TDevicePositioningDeclaration, bool) {
	matches := devicePositioningPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil, false
	}
	return &TDevicePositioningDeclaration{Spec: matches[1], DeviceID: matches[2]}, true
}

// deviceWithBlockHeaderPattern is "device <spec> from <device-id> with:" -- PROJECT.md's
// 2026-09-01 unification: merges this file's own light positioning with
// Conceptual_DeviceCapabilityEntities.go's "for <device-id>: ... end;" shorthand into one block,
// so a device's positioning and its explicit capability list live in a single statement instead of
// two separate ones repeating the device-id. Distinguished from devicePositioningPattern by ending
// in "with:" (a block header, parser.go's main loop tracks "currently inside this block, for which
// device-id") rather than ";" (a complete statement).
var deviceWithBlockHeaderPattern = regexp.MustCompile(`^device\s+(\S+)\s+from\s+(\S+)\s+with:$`)

// extractDeviceWithBlockHeader recognises a "device <spec> from <device-id> with:" block header.
// Returns a TDevicePositioningDeclaration -- the header is registered exactly like a bare
// positioning statement (registerDevicePositioning), since it's semantically identical; only the
// body (each line expanded via expandForDeviceShorthandLine, Conceptual_DeviceCapabilityEntities.go,
// then dispatched through the ordinary extractDeviceCapabilityEntityDeclaration path) is new.
func extractDeviceWithBlockHeader(line string) (*TDevicePositioningDeclaration, bool) {
	matches := deviceWithBlockHeaderPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil, false
	}
	return &TDevicePositioningDeclaration{Spec: matches[1], DeviceID: matches[2]}, true
}

// registerDevicePositioning resolves decl.Spec through the normal entity-naming machinery
// (prepending "device." so it gets the same default-sphere/space-relative-path handling
// device.<spec>'s own bulk form already has -- deviceSpecLeafPath/deviceDisplayName,
// Conceptual_DeviceEntities.go), builds the device's shared conceptual link (DisplayName from
// the current space + resolved sphere/path, ConstantAttributes from Physical.def's device-level
// fields), and auto-registers its "node" capability, if declared, via
// registerDeviceSourceEntityLink -- exactly as if an explicit
// "entity binary_sensor.<spec>/node from <device-id> binary_sensor.node;" line had been
// written. Called from the single main parse loop at the exact point the positioning line is
// reached, with administration.SpacePath already reflecting the real, current nesting -- so the
// node sub-call always finds DeviceConceptualLinks[decl.DeviceID] just-set two lines up and can
// never itself need to defer; its finalAttempt argument is fixed true for exactly that reason.
// Returns warnings; never aborts parsing.
//
// discoveryGatewaysByID (PROJECT.md item 7, 2026-09-15) is checked first, ahead of every other
// kind. Unlike hosts (always) or hassbridge (only if declared, but never DEFERRED -- its "node"
// source is already a fully-resolved remote reference straight from Physical.def), a discovery
// gateway's "node" capability, when declared, tracks a SIBLING capability's own resolved LOCAL
// entity_id (registerDiscoveryAvailabilityEntityLink/registerDiscoveryDerivedEntityLink) -- that
// sibling is virtually always positioned later in the same "with:" block's own body, so the
// auto-registration attempted here almost always defers; tDeferredAutoCapabilityLink carries
// enough (decl + deviceNamePath) for the caller (parser.go) to retry it exactly like any other
// deferred capability line, once the whole file has been read. A gateway device with NO "node"
// capability declared is silently skipped, no warning -- unlike hosts/hassbridge, MQTT discovery
// gateways are NOT uniformly controlled (Zigbee2MQTT's own availability conventions can't be
// assumed to generalise to every future discovery source), so declaring no liveness capability at
// all is a legitimate, unremarkable choice here, not an oversight worth flagging.
func registerDevicePositioning(administration *TAdministrationState, decl TDevicePositioningDeclaration, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hostDevicesByID map[string]THostDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, commandlineDevicesByID map[string]TCommandlineDevice, logicalDevicesByID map[string]TLogicalDevice, entitiesPath string, lineNum int) ([]string, []tDeferredAutoCapabilityLink) {
	provenance := fmt.Sprintf("%s:%d → device %s from %s", filepath.Base(entitiesPath), lineNum, decl.Spec, decl.DeviceID)

	// No legitimate DSL construct positions the same device id twice -- a "dependency on"-only
	// Logical.def entry sharing a physical device's id is wired through DependsOnAvailabilityTopics
	// entirely separately (generator.go), never through a second call to this function. Real bug
	// found live 2026-09-16: Vienna's Conceptual.def declared "device infrastructural:sonos/left
	// from appliance.sonos-left;" twice, verbatim, with no warning at all (see
	// TAdministrationState.DevicePositioningProvenance's own doc comment for why the usual
	// entity-level dedup checks can't catch this).
	if firstProvenance, seen := administration.DevicePositioningProvenance[decl.DeviceID]; seen {
		return []string{fmt.Sprintf("%s: device %q is already positioned in Conceptual.def -- ignoring this duplicate positioning\n  first:  %s\n  second: %s", provenance, decl.DeviceID, provenanceLabel(firstProvenance), provenanceLabel(provenance))}, nil
	}
	administration.DevicePositioningProvenance[decl.DeviceID] = provenance

	// Checked first, ahead of every physical kind: a logical device's own id is always brand new
	// (never shared with a physical kind's own id -- the "dependency on" overlay case shares an id
	// but never has its OWN Capabilities, so it's never positioned through this branch at all; see
	// integration_logical_storage.go's own header comment for the distinction). Same auto-"node"
	// treatment as discovery below, just dispatched through registerLogicalIsAvailableCapability
	// instead once the label resolves.
	//
	// Real bug found live 2026-09-16: this branch used to fire for ANY id present in
	// logicalDevicesByID, including a "dependency on"-only entry (empty Capabilities) sharing an id
	// with an existing physical device -- e.g. node.vienna_livingroom (import-kind, Logical.def only
	// declares "dependency on host.netatmo;"). Since the branch always returned unconditionally, the
	// device's OWN physical-kind registration below (the "importedDevicesByID"/"hostDevicesByID"/...
	// dispatch, which is what auto-implies an import/hosts device's own "node" capability) was never
	// reached at all -- silently dropping node.vienna_livingroom's own connectivity entity entirely
	// (confirmed live: no "node:" key in coordinator/imported.yaml, missing from the aggregate node
	// list), even though Physical.def still declares "binary_sensor.node;" for it. The doc comment
	// above already asserted this "never happens" -- the code just never actually enforced it. Fixed
	// by requiring logical.Capabilities to be non-empty before intercepting here; a dependency-only
	// logical device now falls through to its real physical-kind branch below, same as if it had no
	// Logical.def entry at all (DependsOnAvailabilityTopics is resolved entirely separately, in
	// generator.go's parseAdministrationFromPaths, so this guard doesn't affect that at all).
	//
	// hasPhysicalPresence (2026-09-18, the "absorb" operation) extends this same guard one step
	// further: a device id can now have BOTH a real physical presence (e.g. hassbridge) AND non-
	// empty Logical.def capabilities of its own (an "absorb" block, plus a wrapping enabler-aware
	// capability referencing that SAME device's own raw node -- see Conceptual_DeviceCapabilityEntities.go's
	// dispatchLogicalCapability and Logical.def's own appliance.washing_machine entry). Such a device
	// must still go through its OWN physical-kind branch below for POSITIONING (shared "device:"
	// block, constant attributes, suggested_area, raw node auto-registration) -- only its individual
	// CAPABILITY lines (dispatched via registerDeviceCapabilityEntityLinkAllowingHidden, not this
	// function) reach the logical overlay. Without this, appliance.washing_machine's real hassbridge
	// positioning (constantAttrs/suggested_area/its own genuine "node" capability) would be silently
	// skipped the moment its Logical.def entry gained any real capability -- the same class of bug
	// as the 2026-09-16 one this comment already describes, just on the non-empty-Capabilities side.
	_, hasHassBridgePresence := hassBridgeDevicesByID[decl.DeviceID]
	_, hasDiscoveryPresence := discoveryGatewaysByID[decl.DeviceID]
	_, hasHostPresence := hostDevicesByID[decl.DeviceID]
	_, hasImportedPresence := importedDevicesByID[decl.DeviceID]
	_, hasCommandlinePresence := commandlineDevicesByID[decl.DeviceID]
	hasPhysicalPresence := hasHassBridgePresence || hasDiscoveryPresence || hasHostPresence || hasImportedPresence || hasCommandlinePresence
	if logical, found := logicalDevicesByID[decl.DeviceID]; found && len(logical.Capabilities) > 0 && !hasPhysicalPresence {
		deviceIdentity := extractEntityIdentity(normalizeEntityFullName("device."+decl.Spec, administration.SpacePath))
		if deviceIdentity.Sphere == "" || deviceIdentity.Path == "" {
			return []string{fmt.Sprintf("%s: could not resolve a sphere/path from %q; skipping", provenance, decl.Spec)}, nil
		}
		deviceNamePath := deviceSpecLeafPath(decl.Spec)
		displayName := deviceDisplayName(administration.CurrentSpaceName(), deviceIdentity.Sphere, deviceNamePath)
		administration.DeviceConceptualLinks[decl.DeviceID] = TDeviceConceptualLink{
			DisplayName:        displayName,
			AttributeEntityIDs: map[string]TDeviceAttributeLink{},
		}

		var warnings []string
		if nodeCapability, hasNode := logical.Capabilities["node"]; hasNode {
			autoDecl := TDeviceCapabilityEntityDeclaration{
				LocalSpec:  nodeCapability.Domain + "." + deviceIdentity.Sphere + ":" + "node",
				DeviceID:   decl.DeviceID,
				Capability: "node",
			}
			// No deferral possible here (registerLogicalIsAvailableCapability never defers -- its
			// Entity/EnablerEntity are always literal, already-existing entity_ids, nothing to wait
			// on), unlike discovery's own auto-node above.
			warnings, _ = registerDeviceCapabilityEntityLinkAllowingHidden(administration, autoDecl, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, logicalDevicesByID, entitiesPath, lineNum, false, deviceNamePath, true)
		}
		return warnings, nil
	}

	if gateway, found := discoveryGatewaysByID[decl.DeviceID]; found {
		deviceIdentity := extractEntityIdentity(normalizeEntityFullName("device."+decl.Spec, administration.SpacePath))
		if deviceIdentity.Sphere == "" || deviceIdentity.Path == "" {
			return []string{fmt.Sprintf("%s: could not resolve a sphere/path from %q; skipping", provenance, decl.Spec)}, nil
		}
		deviceNamePath := deviceSpecLeafPath(decl.Spec)
		displayName := deviceDisplayName(administration.CurrentSpaceName(), deviceIdentity.Sphere, deviceNamePath)
		administration.DeviceConceptualLinks[decl.DeviceID] = TDeviceConceptualLink{
			DisplayName:        displayName,
			AttributeEntityIDs: map[string]TDeviceAttributeLink{},
		}

		// Auto-registered here, from the positioning header alone, never from an explicit
		// Spaces.def body line: "node" -- exactly like hosts/hassbridge's own auto-node,
		// generalised to discovery 2026-09-15. A "hidden" capability (2026-09-15) does NOT need
		// this treatment: its only sanctioned use is as a "derived ... from ...;" sibling's own
		// source, and registerDiscoveryDerivedEntityLink resolves that directly off Physical.def's
		// own gateway.Capabilities map (the raw gateway+leaf), never through a materialized
		// DiscoveryEntityLinks entry of its own -- so a hidden raw leaf never needs (and never
		// gets) its own positioned entity at all.
		var warnings []string
		var deferred []tDeferredAutoCapabilityLink
		var autoLabels []string
		if _, hasNode := gateway.Capabilities["node"]; hasNode {
			autoLabels = append(autoLabels, "node")
		}
		for _, label := range autoLabels {
			autoDecl := TDeviceCapabilityEntityDeclaration{
				LocalSpec:  gateway.Capabilities[label].Domain + "." + deviceIdentity.Sphere + ":" + label,
				DeviceID:   decl.DeviceID,
				Capability: label,
			}
			w, isDeferred := registerDeviceCapabilityEntityLinkAllowingHidden(administration, autoDecl, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, logicalDevicesByID, entitiesPath, lineNum, false, deviceNamePath, true)
			warnings = append(warnings, w...)
			if isDeferred {
				deferred = append(deferred, tDeferredAutoCapabilityLink{decl: autoDecl, deviceNamePath: deviceNamePath})
			}
		}
		return warnings, deferred
	}

	deviceIdentity := extractEntityIdentity(normalizeEntityFullName("device."+decl.Spec, administration.SpacePath))
	if deviceIdentity.Sphere == "" || deviceIdentity.Path == "" {
		return []string{fmt.Sprintf("%s: could not resolve a sphere/path from %q; skipping", provenance, decl.Spec)}, nil
	}
	spaceName := administration.CurrentSpaceName()
	displayName := deviceDisplayName(spaceName, deviceIdentity.Sphere, deviceSpecLeafPath(decl.Spec))

	if hostDevice, found := hostDevicesByID[decl.DeviceID]; found {
		return registerHostDevicePositioning(administration, decl, hostDevice, deviceIdentity, spaceName, displayName, provenance), nil
	}

	if importedDevice, found := importedDevicesByID[decl.DeviceID]; found {
		return registerImportedDevicePositioning(administration, decl, importedDevice, deviceIdentity, displayName, entitiesPath, lineNum, provenance), nil
	}

	if _, found := commandlineDevicesByID[decl.DeviceID]; found {
		return registerCommandlineDevicePositioning(administration, decl, displayName), nil
	}

	device, found := hassBridgeDevicesByID[decl.DeviceID]
	if !found {
		return []string{fmt.Sprintf("%s: device %q not found in Physical.def's \"hosts\"/\"home_assistant\"/\"commandline\" integrations or as a \"hassbridge\"-form import", provenance, decl.DeviceID)}, nil
	}

	constantAttrs := make(map[string]TDeviceAttributeConstant, len(device.ConstantAttributes))
	for name, attr := range device.ConstantAttributes {
		constantAttrs[name] = TDeviceAttributeConstant{Value: attr.Value, Forced: attr.Forced}
	}
	// A device positioned inside an "as area" space gets that area as its suggested_area default --
	// unless the device already has its own explicit override (device always wins). Mirrors
	// registerHostNodeEntity's identical precedence (Conceptual_DeviceEntities.go) -- real gap found
	// live 2026-09-08: this hassbridge path never called CurrentArea() at all, so a hassbridge
	// device nested under a "space ... as area with:" (e.g. Junglinster's "social:front" nested
	// under "social:terrace as area") never picked up the enclosing area, unlike a "hosts" device in
	// the exact same position.
	if _, hasExplicit := constantAttrs["suggested_area"]; !hasExplicit {
		if area := administration.CurrentArea(); area != "" {
			constantAttrs["suggested_area"] = TDeviceAttributeConstant{Value: area}
		}
	}
	administration.DeviceConceptualLinks[decl.DeviceID] = TDeviceConceptualLink{
		DisplayName:        displayName,
		ConstantAttributes: constantAttrs,
		AttributeEntityIDs: map[string]TDeviceAttributeLink{},
	}

	nodeCapability, hasNode := device.Capabilities["node"]
	if !hasNode {
		// This positioning form's whole second half (beyond the shared "device:" block) is
		// auto-registering the device's own "node" (liveness) entity -- silently skipping that
		// when none is declared used to give no feedback at all, easy to miss (confirmed live
		// 2026-08-29: several home_assistant bridge devices had no "node" capability declared and
		// nothing ever flagged it). Warn, don't error: a device genuinely not wanting a liveness
		// entity is a legitimate, if rare, choice -- this positioning form must still work for it.
		return []string{fmt.Sprintf("%s: device %q has no \"node\" capability declared -- its own liveness/connectivity entity won't be registered; add one (e.g. \"binary_sensor.node: <some already-reported entity> is available;\") if that's not intentional", provenance, decl.DeviceID)}, nil
	}
	nodeSpec := "binary_sensor." + deviceIdentity.Sphere + ":" + deviceSpecLeafPath(decl.Spec) + "/node"
	warnings, _ := registerDeviceSourceEntityLink(administration, TDeviceSourceEntityDeclaration{
		LocalSpec: nodeSpec,
		// capabilityKey "node" below is non-empty, so registerDeviceSourceEntityLink preserves
		// nodeCapability's own already-per-instance Sources map unchanged -- this Source value is
		// display-only (the provenance string on a rare error path).
		Source:   representativeSource(nodeCapability.Sources),
		DeviceID: decl.DeviceID,
	}, hassBridgeDevicesByID, importedDevicesByID, entitiesPath, lineNum, true, "node", "")
	return warnings, nil
}

// tDeferredAutoCapabilityLink carries a discovery gateway's auto-implied "node" capability
// registration (registerDevicePositioning) that couldn't resolve yet -- almost always the case,
// since the sibling capability it tracks is virtually always positioned later in the same "with:"
// block's own body. The caller (parser.go) queues this exactly like any other deferred capability
// line and retries once, after the whole file has been read.
type tDeferredAutoCapabilityLink struct {
	decl           TDeviceCapabilityEntityDeclaration
	deviceNamePath string
}

// registerHostDevicePositioning is registerDevicePositioning's counterpart for a "hosts"
// integration device (PROJECT.md, unification plan 2026-09-01) -- unlike the "home_assistant"
// bridge path, a hosts device's node entity is unconditional (every hosts device gets one,
// regardless of what attributes get requested later), so there's no "has no node capability
// declared" warning case here -- registerHostNodeEntity (Conceptual_DeviceEntities.go) already
// registers it as part of building the base link. Every other attribute (e.g. "load"/"temperature"
// for a cpu-type host) is expected to be individually requested afterward via
// registerDeviceCapabilityEntityLink's own now-hosts-aware branch (the "entity ... from
// <device-id> entity <capability>;" construct, or its "for <device-id>: ... end;" shorthand) --
// this positioning form only ever seeds the shared "device:" block plus the node, same division of
// responsibility the "home_assistant" path already has.
func registerHostDevicePositioning(administration *TAdministrationState, decl TDevicePositioningDeclaration, device THostDevice, deviceIdentity TEntityIdentity, spaceName, displayName, provenance string) []string {
	mat, known := MaterializationForIntegrationType(device.IntegrationType)
	if !known {
		return []string{fmt.Sprintf("%s: integration type %q has no known entity materialization; skipping", provenance, device.IntegrationType)}
	}
	link, warnings := registerHostNodeEntity(administration, mat, device, deviceIdentity, spaceName, displayName, provenance)
	administration.DeviceConceptualLinks[decl.DeviceID] = link
	return warnings
}

// registerCommandlineDevicePositioning is registerDevicePositioning's counterpart for a
// "commandline" integration device (real gap found live 2026-09-10: a commandline device was
// only ever reachable via registerCommandlineCapabilityEntityLink, which requires
// DeviceConceptualLinks[deviceID] to already exist -- by design, for a device sharing its
// identity with an already-positioned "hosts"/"home_assistant" device of the SAME id (e.g.
// host.frame, discoverycommandline.go's own doc comment explains why). A commandline device with
// no such sibling -- its own standalone DeviceID, e.g. appliance.picture_frame -- had no way to
// ever get positioned at all).
//
// Deliberately minimal, unlike the hosts/hassbridge/import paths: TCommandlineDevice carries no
// ConstantAttributes (no device-info fields exist anywhere in this integration kind's own
// Physical.def grammar) and no "node" capability concept -- a commandline device's liveness
// entity is always the coordinator's own unconditional, auto-named
// binary_sensor.<host>_commandline_node (discoverycommandline.go's buildCommandlineDiscoveryConfigs),
// never influenced by Spaces.def positioning, so there is nothing else to auto-register here
// beyond the shared "device:" block itself.
func registerCommandlineDevicePositioning(administration *TAdministrationState, decl TDevicePositioningDeclaration, displayName string) []string {
	administration.DeviceConceptualLinks[decl.DeviceID] = TDeviceConceptualLink{
		DisplayName:        displayName,
		AttributeEntityIDs: map[string]TDeviceAttributeLink{},
	}
	return nil
}

// registerImportedDevicePositioning is registerDevicePositioning's counterpart for a
// "hassbridge"-form import (PROJECT.md 1.2d) -- same shape as the native path (shared "device:"
// block + auto-registered "node" liveness entity, if declared), except device-info fields
// (manufacturer/model/...) are always left empty (an import carries no Physical.def-declared
// device-info fields of its own; the coordinator learns them live from the exporting
// installation's own report, same principle as registerImportedDeviceSourceEntityLink's typing-
// metadata deferral) and node resolution goes through that function (capabilityKey "node") instead
// of the native path's inline registerDeviceSourceEntityLink call.
//
// suggested_area is the one exception to "device-info fields stay empty" -- it's a purely local
// Spaces.def positioning concept (which area a device sits in on THIS house's own conceptual
// layer), unrelated to the remote device's own hardware info the exporter reports live. Mirrors
// registerHostNodeEntity/the native hassbridge path's identical "as area" precedence -- real gap
// found live 2026-09-08 alongside the native path's own identical bug.
func registerImportedDevicePositioning(administration *TAdministrationState, decl TDevicePositioningDeclaration, importedDevice TImportedDevice, deviceIdentity TEntityIdentity, displayName, entitiesPath string, lineNum int, provenance string) []string {
	link := TDeviceConceptualLink{
		DisplayName:        displayName,
		AttributeEntityIDs: map[string]TDeviceAttributeLink{},
	}
	if area := administration.CurrentArea(); area != "" {
		link.ConstantAttributes = map[string]TDeviceAttributeConstant{"suggested_area": {Value: area}}
	}
	administration.DeviceConceptualLinks[decl.DeviceID] = link

	if _, hasNode := importedDevice.Capabilities["node"]; !hasNode {
		return []string{fmt.Sprintf("%s: imported device %q declares no \"node\" capability -- its own liveness/connectivity entity won't be registered; add one (e.g. \"node: binary_sensor.<some-already-exported-entity>;\") to its Physical.def import declaration if that's not intentional", provenance, decl.DeviceID)}
	}
	nodeSpec := "binary_sensor." + deviceIdentity.Sphere + ":" + deviceSpecLeafPath(decl.Spec) + "/node"
	warnings, _ := registerDeviceSourceEntityLink(administration, TDeviceSourceEntityDeclaration{
		LocalSpec: nodeSpec,
		DeviceID:  decl.DeviceID,
	}, nil, map[string]TImportedDevice{decl.DeviceID: importedDevice}, entitiesPath, lineNum, true, "node", "")
	return warnings
}
