/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualDeviceCapabilityEntities
 *
 * Parses and registers the conceptual layer's "entity <local-spec> from <device-id> entity
 * <capability>;" construct -- a unified alternative to the discovery integration's dot-joined
 * "entity <spec> from <gateway-id>.<leaf>;" (Conceptual_DiscoveryEntities.go) and the
 * home_assistant bridge's "entity <spec> as <source> from <device-id>;"
 * (Conceptual_DeviceSourceEntities.go) forms, referencing a device's own already-declared
 * capability by its local label (the LHS of its "<type>.<label>: <source>;" Physical.def line,
 * e.g. "co2" from "sensor.co2: sensor.davids_bedroom_carbon_dioxide;") regardless of which
 * integration kind the device belongs to -- one syntax for "position this specific entity of a
 * device I've already declared," instead of a different keyword per device kind. <capability> may
 * carry a domain prefix ("sensor.co2") for readability at the call site; it's stripped before
 * lookup since neither the discovery gateway's nor the hassbridge device's own Capabilities map
 * is keyed by domain-prefixed names (both are keyed by the bare local label alone).
 *
 * This file owns only the parsing + dispatch; the actual registration work is delegated entirely
 * to each device kind's own existing, unchanged function (registerDiscoveryEntityLink /
 * registerDeviceSourceEntityLink) once the device id resolves and the bare capability name is
 * looked up -- so this construct can never drift from what those two already-relied-upon paths do.
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
	"slices"
	"strings"
)

// TDeviceCapabilityEntityDeclaration is one parsed "entity <local-spec> from <device-id> entity
// <capability>;" line.
type TDeviceCapabilityEntityDeclaration struct {
	LocalSpec  string
	DeviceID   string
	Capability string // e.g. "sensor.co2" -- domain prefix optional, stripped before lookup (bareCapabilityName)
}

var deviceCapabilityEntityPattern = regexp.MustCompile(`^entity\s+(\S+)\s+from\s+(\S+)\s+entity\s+(\S+);$`)

// extractDeviceCapabilityEntityDeclaration recognises the "entity <spec> from <device-id> entity
// <capability>;" shape -- distinguished from extractDiscoveryEntityDeclaration's dot-joined
// "entity <spec> from <gateway-id>.<leaf>;" (exactly one token after "from") by the extra literal
// "entity" keyword and capability token.
func extractDeviceCapabilityEntityDeclaration(line string) (*TDeviceCapabilityEntityDeclaration, bool) {
	matches := deviceCapabilityEntityPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil, false
	}
	return &TDeviceCapabilityEntityDeclaration{
		LocalSpec:  matches[1],
		DeviceID:   matches[2],
		Capability: matches[3],
	}, true
}

// forDeviceShorthandPattern is the body-line shape inside a "device <spec> from <device-id>
// with: ... end;" block (Conceptual_DevicePositioning.go's deviceWithBlockHeaderPattern) -- sugar
// for a run of "entity <spec> from <device-id> entity <capability>;" lines that all repeat the
// same device-id. Inside the block, each line drops the repeated device-id ("entity <spec> from
// entity <capability>;"); parser.go's own main loop tracks "currently inside a with-block, for
// which device-id" and expands each such line back to deviceCapabilityEntityPattern's full shape
// before handing it to the ordinary dispatch chain -- so this is purely textual sugar, expanded at
// the point of parsing, with no new registration path of its own to drift from
// registerDeviceCapabilityEntityLink. (Formerly also the body shape of a standalone "for
// <device-id>: ... end;" block; that standalone form and its header recognizer were retired
// 2026-09-01 in favour of the merged construct -- this expansion function is the only part that
// survived, now reused by the merged construct's own body-line handling.)
var forDeviceShorthandPattern = regexp.MustCompile(`^entity\s+(\S+)\s+from\s+entity\s+(\S+);$`)

// expandForDeviceShorthandLine rewrites one "entity <spec> from entity <capability>;" line (found
// inside a "device <spec> from <deviceID> with: ... end;" block) back to the full "entity <spec>
// from <deviceID> entity <capability>;" shape deviceCapabilityEntityPattern expects.
func expandForDeviceShorthandLine(line, deviceID string) (string, bool) {
	matches := forDeviceShorthandPattern.FindStringSubmatch(line)
	if matches == nil {
		return "", false
	}
	return fmt.Sprintf("entity %s from %s entity %s;", matches[1], deviceID, matches[2]), true
}

// bareCapabilityName strips an optional "<domain>." prefix from ref (e.g. "sensor.co2" -> "co2"),
// matching the bare-name convention both discoveryGatewaysByID's and hassBridgeDevicesByID's own
// Capabilities maps are keyed by (each map key is a device's own declared local label, never
// domain-prefixed -- see integration_discovery_parser.go / integration_hassbridge_parser.go).
func bareCapabilityName(ref string) string {
	if idx := strings.Index(ref, "."); idx >= 0 {
		return ref[idx+1:]
	}
	return ref
}

// registerDeviceCapabilityEntityLink dispatches decl.DeviceID to whichever integration kind it
// resolves against -- a "discovery" gateway device (delegates to registerDiscoveryEntityLink,
// Conceptual_DiscoveryEntities.go, entirely unchanged), a "home_assistant" bridge device (looks up
// the named capability's own Source, then delegates to registerDeviceSourceEntityLink,
// Conceptual_DeviceSourceEntities.go, entirely unchanged), or -- since 2026-09-01 -- a "hosts"
// device (delegates to registerHostCapabilityEntityLink, below; structurally different from the
// hassbridge/import branches since a hosts capability has no per-attribute Source to resolve, see
// that function's own doc comment).
//
// finalAttempt is threaded straight through to registerDeviceSourceEntityLink for the
// "home_assistant" branch -- see that function's own doc comment: the parser reads the file once,
// so a U1 line can arrive before its device's U2 positioning does; false lets that branch defer
// silently (deferred=true, no warning) rather than report a false "not positioned" failure, true
// is the one retry after the whole file has been read, where "still not positioned" is final. The
// discovery/hosts/unknown-device branches never depend on positioning, so they always return
// deferred=false regardless of finalAttempt.
func registerDeviceCapabilityEntityLink(administration *TAdministrationState, decl TDeviceCapabilityEntityDeclaration, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, hostDevicesByID map[string]THostDevice, commandlineDevicesByID map[string]TCommandlineDevice, entitiesPath string, lineNum int, finalAttempt bool) (warnings []string, deferred bool) {
	provenance := fmt.Sprintf("%s:%d → %s from %s entity %s", filepath.Base(entitiesPath), lineNum, decl.LocalSpec, decl.DeviceID, decl.Capability)
	bareName := bareCapabilityName(decl.Capability)

	// Checked first, but only actually claims the declaration when this SPECIFIC capability name is
	// one of this device's own commandline capabilities -- a commandline device commonly shares its
	// DeviceID with a "hosts"-kind declaration of the same physical machine (e.g. host.frame is both
	// a "hosts" cpu device and a "commandline" device, deliberately sharing one HA device identity,
	// discoverycommandline.go's own doc comment), so falling through to the other kinds below when
	// the capability name isn't recognised here (rather than erroring immediately, unlike every
	// other branch) is required for that dual-kind case to keep working for its own non-commandline
	// capabilities (e.g. "load"/"temperature").
	if device, found := commandlineDevicesByID[decl.DeviceID]; found {
		if capability, capFound := device.Capabilities[bareName]; capFound {
			return registerCommandlineCapabilityEntityLink(administration, decl.LocalSpec, decl.DeviceID, bareName, capability, provenance, finalAttempt)
		}
	}

	if _, found := discoveryGatewaysByID[decl.DeviceID]; found {
		return registerDiscoveryEntityLink(administration, TDiscoveryEntityDeclaration{
			EntitySpec:      decl.LocalSpec,
			GatewayDeviceID: decl.DeviceID,
			Leaf:            bareName,
		}, discoveryGatewaysByID, entitiesPath, lineNum), false
	}

	if device, found := hassBridgeDevicesByID[decl.DeviceID]; found {
		capability, capFound := device.Capabilities[bareName]
		if !capFound {
			return []string{fmt.Sprintf("%s: device %q declares no %q capability; add a \"<type>.%s: <source>;\" line to its Physical.def declaration", provenance, decl.DeviceID, bareName, bareName)}, false
		}
		return registerDeviceSourceEntityLink(administration, TDeviceSourceEntityDeclaration{
			LocalSpec: decl.LocalSpec,
			// capabilityKey (bareName) is non-empty below, so registerDeviceSourceEntityLink
			// preserves capability's own already-per-instance Sources map unchanged -- this Source
			// value is display-only (the provenance string on a rare error path).
			Source:   representativeSource(capability.Sources),
			DeviceID: decl.DeviceID,
		}, hassBridgeDevicesByID, importedDevicesByID, entitiesPath, lineNum, finalAttempt, bareName)
	}

	if importedDevice, found := importedDevicesByID[decl.DeviceID]; found {
		if _, capFound := importedDevice.Capabilities[bareName]; !capFound {
			return []string{fmt.Sprintf("%s: imported device %q declares no %q capability; add a \"%s: <remote-local-entity>;\" line to its Physical.def import declaration", provenance, decl.DeviceID, bareName, bareName)}, false
		}
		return registerDeviceSourceEntityLink(administration, TDeviceSourceEntityDeclaration{
			LocalSpec: decl.LocalSpec,
			DeviceID:  decl.DeviceID,
		}, hassBridgeDevicesByID, importedDevicesByID, entitiesPath, lineNum, finalAttempt, bareName)
	}

	if device, found := hostDevicesByID[decl.DeviceID]; found {
		return registerHostCapabilityEntityLink(administration, device, decl.DeviceID, bareName, provenance, finalAttempt)
	}

	return []string{fmt.Sprintf("%s: device %q not found in Physical.def's \"discovery\" or \"home_assistant\" integrations, or as a \"hassbridge\"-form import", provenance, decl.DeviceID)}, false
}

// registerHostCapabilityEntityLink is registerDeviceCapabilityEntityLink's "hosts"-kind branch
// (PROJECT.md, unification plan 2026-09-01) -- added so `entity <spec> from <device-id> entity
// <capability>;` (and its `for <device-id>: ... end;` shorthand) work for "hosts" devices exactly
// as they already do for "home_assistant" bridge/import devices, letting a device that doesn't
// actually have every attribute its integration type hardwires (e.g. host.mqtt, a cloud VM with
// no real "temperature" sensor) name only the ones it genuinely has, instead of the "with: all
// entities;" bulk form pulling in the full hardwired set.
//
// Structurally different from the hassbridge/import branches above: a hosts capability has no
// per-attribute `Source` string to resolve (integration_hosts_storage.go's
// MaterializationForIntegrationType hardwires domain/naming from the device's own position plus
// its integration type, never a free-form reference), so this reuses registerHostAttributeEntity
// (Conceptual_DeviceEntities.go) -- the same function "with: all entities;"/its own single-flag
// form already call -- rather than registerDeviceSourceEntityLink. "node" is deliberately rejected
// here: a hosts device's node is unconditional, already registered the moment it's positioned
// (registerHostNodeEntity, called from registerHostDevicePositioning), so there's nothing for an
// explicit reference to add -- accepting it would just be a confusing,
// redundant second path to the exact same entity.
//
// finalAttempt/deferred follow registerDeviceSourceEntityLink's own established convention: the
// parser reads the file once, so a capability-link line can arrive before its device's positioning
// does; deferred=true with no warning lets the single retry (after the whole file is read) resolve
// it once positioning has actually landed, wherever in the file it appears.
func registerHostCapabilityEntityLink(administration *TAdministrationState, device THostDevice, deviceID, bareName, provenance string, finalAttempt bool) (warnings []string, deferred bool) {
	if bareName == "node" {
		return []string{fmt.Sprintf("%s: %q's node entity is registered automatically when the device is positioned (\"device.<spec> from %s;\") -- no explicit reference needed or supported", provenance, deviceID, deviceID)}, false
	}

	mat, known := MaterializationForIntegrationType(device.IntegrationType)
	if !known {
		return []string{fmt.Sprintf("%s: integration type %q has no known entity materialization; skipping", provenance, device.IntegrationType)}, false
	}
	if !slices.Contains(mat.AttributeNames(device), bareName) {
		return []string{fmt.Sprintf("%s: device %q (integration type %q) has no %q attribute", provenance, deviceID, device.IntegrationType, bareName)}, false
	}

	link, hasLink := administration.DeviceConceptualLinks[deviceID]
	if !hasLink {
		if !finalAttempt {
			return nil, true
		}
		return []string{fmt.Sprintf("%s: device %q has no \"device.<spec> from %s;\" positioning yet -- add one (any space) before referencing one of its entities directly", provenance, deviceID, deviceID)}, true
	}

	spaceName := administration.CurrentSpaceName()
	registerHostAttributeEntity(administration, mat, device, link.HostIdentity, spaceName, provenance, link, bareName)
	administration.DeviceConceptualLinks[deviceID] = link
	return nil, false
}
