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
 * explicit "entity binary_sensor.<spec>/node from <device-id> entity binary_sensor.node;"
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
// "entity binary_sensor.<spec>/node from <device-id> entity binary_sensor.node;" line had been
// written. Called from the single main parse loop at the exact point the positioning line is
// reached, with administration.SpacePath already reflecting the real, current nesting -- so the
// node sub-call always finds DeviceConceptualLinks[decl.DeviceID] just-set two lines up and can
// never itself need to defer; its finalAttempt argument is fixed true for exactly that reason.
// Returns warnings; never aborts parsing.
func registerDevicePositioning(administration *TAdministrationState, decl TDevicePositioningDeclaration, hostDevicesByID map[string]THostDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, entitiesPath string, lineNum int) []string {
	provenance := fmt.Sprintf("%s:%d → device %s from %s", filepath.Base(entitiesPath), lineNum, decl.Spec, decl.DeviceID)

	deviceIdentity := extractEntityIdentity(normalizeEntityFullName("device."+decl.Spec, administration.SpacePath))
	if deviceIdentity.Sphere == "" || deviceIdentity.Path == "" {
		return []string{fmt.Sprintf("%s: could not resolve a sphere/path from %q; skipping", provenance, decl.Spec)}
	}
	spaceName := administration.CurrentSpaceName()
	displayName := deviceDisplayName(spaceName, deviceIdentity.Sphere, deviceSpecLeafPath(decl.Spec))

	if hostDevice, found := hostDevicesByID[decl.DeviceID]; found {
		return registerHostDevicePositioning(administration, decl, hostDevice, deviceIdentity, spaceName, displayName, provenance)
	}

	if importedDevice, found := importedDevicesByID[decl.DeviceID]; found {
		return registerImportedDevicePositioning(administration, decl, importedDevice, deviceIdentity, displayName, entitiesPath, lineNum, provenance)
	}

	device, found := hassBridgeDevicesByID[decl.DeviceID]
	if !found {
		return []string{fmt.Sprintf("%s: device %q not found in Physical.def's \"hosts\"/\"home_assistant\" integrations or as a \"hassbridge\"-form import", provenance, decl.DeviceID)}
	}

	constantAttrs := make(map[string]TDeviceAttributeConstant, len(device.ConstantAttributes))
	for name, attr := range device.ConstantAttributes {
		constantAttrs[name] = TDeviceAttributeConstant{Value: attr.Value, Forced: attr.Forced}
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
		return []string{fmt.Sprintf("%s: device %q has no \"node\" capability declared -- its own liveness/connectivity entity won't be registered; add one (e.g. \"binary_sensor.node: <some already-reported entity> is available;\") if that's not intentional", provenance, decl.DeviceID)}
	}
	nodeSpec := "binary_sensor." + deviceIdentity.Sphere + ":" + deviceSpecLeafPath(decl.Spec) + "/node"
	warnings, _ := registerDeviceSourceEntityLink(administration, TDeviceSourceEntityDeclaration{
		LocalSpec: nodeSpec,
		// capabilityKey "node" below is non-empty, so registerDeviceSourceEntityLink preserves
		// nodeCapability's own already-per-instance Sources map unchanged -- this Source value is
		// display-only (the provenance string on a rare error path).
		Source:   representativeSource(nodeCapability.Sources),
		DeviceID: decl.DeviceID,
	}, hassBridgeDevicesByID, importedDevicesByID, entitiesPath, lineNum, true, "node")
	return warnings
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

// registerImportedDevicePositioning is registerDevicePositioning's counterpart for a
// "hassbridge"-form import (PROJECT.md 1.2d) -- same shape as the native path (shared "device:"
// block + auto-registered "node" liveness entity, if declared), except ConstantAttributes is
// always empty (an import carries no Physical.def-declared device-info fields of its own; the
// coordinator learns them live from the exporting installation's own report, same principle as
// registerImportedDeviceSourceEntityLink's typing-metadata deferral) and node resolution goes
// through that function (capabilityKey "node") instead of the native path's inline
// registerDeviceSourceEntityLink call.
func registerImportedDevicePositioning(administration *TAdministrationState, decl TDevicePositioningDeclaration, importedDevice TImportedDevice, deviceIdentity TEntityIdentity, displayName, entitiesPath string, lineNum int, provenance string) []string {
	administration.DeviceConceptualLinks[decl.DeviceID] = TDeviceConceptualLink{
		DisplayName:        displayName,
		AttributeEntityIDs: map[string]TDeviceAttributeLink{},
	}

	if _, hasNode := importedDevice.Capabilities["node"]; !hasNode {
		return []string{fmt.Sprintf("%s: imported device %q declares no \"node\" capability -- its own liveness/connectivity entity won't be registered; add one (e.g. \"node: binary_sensor.<some-already-exported-entity>;\") to its Physical.def import declaration if that's not intentional", provenance, decl.DeviceID)}
	}
	nodeSpec := "binary_sensor." + deviceIdentity.Sphere + ":" + deviceSpecLeafPath(decl.Spec) + "/node"
	warnings, _ := registerDeviceSourceEntityLink(administration, TDeviceSourceEntityDeclaration{
		LocalSpec: nodeSpec,
		DeviceID:  decl.DeviceID,
	}, nil, map[string]TImportedDevice{decl.DeviceID: importedDevice}, entitiesPath, lineNum, true, "node")
	return warnings
}
