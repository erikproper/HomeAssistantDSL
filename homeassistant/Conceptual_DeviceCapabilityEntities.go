/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualDeviceCapabilityEntities
 *
 * Parses and registers the conceptual layer's "entity <local-spec> from <device-id> <capability>;"
 * construct -- a unified alternative to the discovery integration's dot-joined "entity <spec> from
 * <gateway-id>.<leaf>;" (Conceptual_DiscoveryEntities.go) and the home_assistant bridge's "entity
 * <spec> as <source> from <device-id>;" (Conceptual_DeviceSourceEntities.go) forms, referencing a
 * device's own already-declared capability by its local label (the LHS of its "<type>.<label>:
 * <source>;" Physical.def line, e.g. "co2" from "sensor.co2: sensor.davids_bedroom_carbon_dioxide;")
 * regardless of which integration kind the device belongs to -- one syntax for "position this
 * specific entity of a device I've already declared," instead of a different keyword per device
 * kind. <capability> may carry a domain prefix ("sensor.co2") for readability at the call site;
 * it's stripped before lookup since neither the discovery gateway's nor the hassbridge device's own
 * Capabilities map is keyed by domain-prefixed names (both are keyed by the bare local label alone).
 *
 * PROJECT.md item 7 (2026-09-09): the RHS used to require a redundant literal "entity" keyword
 * before <capability> ("entity <spec> from <device-id> <capability>;") -- dropped outright
 * (no dual-syntax transition period, matching every other grammar retirement this project has done)
 * once it became clear nothing else could ever occupy that slot: the only other "entity ... from
 * ...;" shape with a single token after "from" is the discovery gateway's dot-joined form, which is
 * unambiguous against this construct's two-or-three-token shape regardless of the "entity" keyword.
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

// TDeviceCapabilityEntityDeclaration is one parsed "entity <local-spec> from <device-id>
// <capability>;" line.
type TDeviceCapabilityEntityDeclaration struct {
	LocalSpec  string
	DeviceID   string
	Capability string // e.g. "sensor.co2" -- domain prefix optional, stripped before lookup (bareCapabilityName)
	// NoCollect excludes this entity from its space's domain-specific aggregate collections
	// (mirrors the plain-entity "no_collect" suffix, TEntityRecord.NoCollect) -- e.g. a smart
	// plug's own PCB temperature reading, which shares a "/temperature" subdomain with real room
	// climate sensors but must stay out of the room's aggregated group sensor. Written as a
	// trailing "with no_collect;" -- the same single-property "with <X>;" shorthand a bare "entity
	// <spec> with <X>;" declaration already uses (analyzeEntityDefinitionContext) -- rather than a
	// bare trailing word, so "no_collect" is never mistaken for a second capability token.
	NoCollect bool
}

var deviceCapabilityEntityPattern = regexp.MustCompile(`^entity\s+(\S+)\s+from\s+(\S+)\s+(\S+?)(?:\s+with\s+no_collect)?;$`)

// extractDeviceCapabilityEntityDeclaration recognises the "entity <spec> from <device-id>
// <capability> [with no_collect];" shape -- distinguished from extractDiscoveryEntityDeclaration's
// dot-joined "entity <spec> from <gateway-id>.<leaf>;" (exactly one token after "from") by the
// extra capability token.
func extractDeviceCapabilityEntityDeclaration(line string) (*TDeviceCapabilityEntityDeclaration, bool) {
	matches := deviceCapabilityEntityPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil, false
	}
	return &TDeviceCapabilityEntityDeclaration{
		LocalSpec:  matches[1],
		DeviceID:   matches[2],
		Capability: matches[3],
		NoCollect:  strings.HasSuffix(line, "with no_collect;"),
	}, true
}

// forDeviceShorthandPattern is the body-line shape inside a "device <spec> from <device-id>
// with: ... end;" block (Conceptual_DevicePositioning.go's deviceWithBlockHeaderPattern) -- sugar
// for a run of "entity <spec> from <device-id> <capability>;" lines that all repeat the same
// device-id. Inside the block, each line drops the repeated device-id ("entity <spec> from
// <capability>;"); parser.go's own main loop tracks "currently inside a with-block, for which
// device-id" and expands each such line back to deviceCapabilityEntityPattern's full shape before
// handing it to the ordinary dispatch chain -- so this is purely textual sugar, expanded at the
// point of parsing, with no new registration path of its own to drift from
// registerDeviceCapabilityEntityLink. (Formerly also the body shape of a standalone "for
// <device-id>: ... end;" block; that standalone form and its header recognizer were retired
// 2026-09-01 in favour of the merged construct -- this expansion function is the only part that
// survived, now reused by the merged construct's own body-line handling.)
//
// No collision risk against the discovery gateway's own dot-joined "entity <spec> from
// <gateway-id>.<leaf>;" (PROJECT.md item 7, 2026-09-09, dropping this shape's own former "entity"
// keyword before <capability>): a "device ... with:" block can only ever be opened for a
// hosts/imported/hassbridge target (registerDevicePositioning, Conceptual_DevicePositioning.go),
// never a "discovery" gateway one, and parser.go's main loop only ever attempts this expansion
// while inside such a block (tracked via forDeviceID) -- the discovery pattern is never even
// consulted for a line reached this way.
var forDeviceShorthandPattern = regexp.MustCompile(`^entity\s+(\S+)\s+from\s+(\S+?)(?:\s+with\s+no_collect)?;$`)

// expandForDeviceShorthandLine rewrites one "entity <spec> from <capability> [with no_collect];"
// line (found inside a "device <spec> from <deviceID> with: ... end;" block) back to the full
// "entity <spec> from <deviceID> <capability> [with no_collect];" shape
// deviceCapabilityEntityPattern expects.
func expandForDeviceShorthandLine(line, deviceID string) (string, bool) {
	matches := forDeviceShorthandPattern.FindStringSubmatch(line)
	if matches == nil {
		return "", false
	}
	suffix := ""
	if strings.HasSuffix(line, "with no_collect;") {
		suffix = " with no_collect"
	}
	return fmt.Sprintf("entity %s from %s %s%s;", matches[1], deviceID, matches[2], suffix), true
}

// forDeviceBareEntityPattern is a SECOND, even shorter body-line shape inside the same "device
// <spec> from <device-id> with: ... end;" block (2026-09-19) -- "entity <spec> [with no_collect];"
// with no "from <capability>" clause at all. Only a shorthand for the ALREADY-shorthand
// forDeviceShorthandPattern above, not a separate mechanism: expandForDeviceBareEntityLine
// auto-infers the capability as <spec>'s own explicit path (the text after the sphere's ":",
// deviceSpecLeafPath) and re-expands through the exact same full "entity <spec> from <device-id>
// <capability> [with no_collect];" shape -- so this can only ever fire when the desired local path
// and the device's own capability name are IDENTICAL text. A rename (local path differs from the
// capability, e.g. "main" positioned from capability "core") still requires the explicit
// "entity <spec> from <capability>;" form -- this pattern doesn't even try to match a line that
// already has a "from" clause, so there's no ambiguity between the two shapes.
var forDeviceBareEntityPattern = regexp.MustCompile(`^entity\s+(\S+?)(?:\s+with\s+no_collect)?;$`)

// expandForDeviceBareEntityLine rewrites one "entity <spec> [with no_collect];" line (no "from"
// clause) into the full "entity <spec> from <deviceID> <capability> [with no_collect];" shape,
// inferring <capability> as deviceSpecLeafPath(spec) -- see forDeviceBareEntityPattern's own doc
// comment. Returns false (no match) when spec has no explicit path at all (e.g. "switch.social:",
// deviceSpecLeafPath returns "") -- there is nothing to infer a capability from in that case, so
// the caller must keep using the explicit "from <capability>;" form.
//
// Only the LAST ":"-separated segment of the leaf path is used as the inferred capability
// (2026-09-19 fix, generalised 2026-09-21): deviceSpecLeafPath only ever strips up to the FIRST
// colon, so a spec using either "sphere::path" (empty leaf-override, hasDeviceLeafOverride,
// Conceptual_DeviceEntities.go) -- e.g. "binary_sensor.social::daylight" -- or "sphere:leaf:path"
// (explicit replacement leaf) -- e.g. "sensor.social:terrace:pressure" -- would otherwise infer a
// capability name still carrying the leftover colon-separated segment(s) (":daylight" or
// "terrace:pressure"), which can never match a real Physical.def/Logical.def capability name (a
// bare label, never colon- or slash-containing). The reconstructed "entity <spec> from ...;" line
// still passes the ORIGINAL spec (with its colons intact) through unchanged, so the naming
// resolution itself is unaffected -- only the inferred capability name needed the fix. Real bugs
// found live: 2026-09-19 (Vienna's daylight entity, double-colon form), 2026-09-21
// (environment.weather's pressure capability, explicit-leaf-override form).
func expandForDeviceBareEntityLine(line, deviceID string) (string, bool) {
	matches := forDeviceBareEntityPattern.FindStringSubmatch(line)
	if matches == nil {
		return "", false
	}
	spec := matches[1]
	leaf := deviceSpecLeafPath(spec)
	leafSegments := strings.Split(leaf, ":")
	capability := leafSegments[len(leafSegments)-1]
	if capability == "" {
		return "", false
	}
	suffix := ""
	if strings.HasSuffix(line, "with no_collect;") {
		suffix = " with no_collect"
	}
	return fmt.Sprintf("entity %s from %s %s%s;", spec, deviceID, capability, suffix), true
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
//
// deviceNamePath is non-empty only when decl came from inside a "device <spec> from <device-id>
// with: ... end;" block's body (parser.go tracks this alongside forDeviceID) -- PROJECT.md item 5,
// see registerDeviceSourceEntityLink's doc comment for the full rationale. Threaded through to
// whichever branch below resolves decl.LocalSpec via a free-form spec (commandline, hassbridge,
// imported); the discovery/hosts branches ignore it, since neither derives its naming from
// decl.LocalSpec's own text (a discovery gateway's naming comes from the dot-joined gateway
// reference itself, and a "hosts" device's from its own positioning-time deviceIdentity, already
// fully resolved when it was positioned).
func registerDeviceCapabilityEntityLink(administration *TAdministrationState, decl TDeviceCapabilityEntityDeclaration, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, hostDevicesByID map[string]THostDevice, commandlineDevicesByID map[string]TCommandlineDevice, logicalDevicesByID map[string]TLogicalDevice, entitiesPath string, lineNum int, finalAttempt bool, deviceNamePath string) (warnings []string, deferred bool) {
	return registerDeviceCapabilityEntityLinkAllowingHidden(administration, decl, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, logicalDevicesByID, entitiesPath, lineNum, finalAttempt, deviceNamePath, false)
}

// registerDeviceCapabilityEntityLinkAllowingHidden is registerDeviceCapabilityEntityLink's real
// body, with one extra parameter: allowHidden. Every ordinary DSL-authored "entity ... from
// <device-id> <capability>;" line (parser.go's main dispatch loop and its deferred retry pass) goes
// through the public wrapper above with allowHidden always false -- a hidden discovery capability
// (integration_discovery_storage.go) may never be positioned directly by Spaces.def. The one
// exception is registerDevicePositioning's own auto-implied registration of a hidden capability's
// underlying entity (Conceptual_DevicePositioning.go) -- that registration is what MAKES the
// capability's value readable at all (a "derived" sibling's own reverse lookup needs a real
// DiscoveryEntityLinks entry to find), so it calls this allowing-hidden entry point directly.
func registerDeviceCapabilityEntityLinkAllowingHidden(administration *TAdministrationState, decl TDeviceCapabilityEntityDeclaration, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, hostDevicesByID map[string]THostDevice, commandlineDevicesByID map[string]TCommandlineDevice, logicalDevicesByID map[string]TLogicalDevice, entitiesPath string, lineNum int, finalAttempt bool, deviceNamePath string, allowHidden bool) (warnings []string, deferred bool) {
	provenance := fmt.Sprintf("%s:%d → %s from %s %s", filepath.Base(entitiesPath), lineNum, decl.LocalSpec, decl.DeviceID, decl.Capability)
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
			return registerCommandlineCapabilityEntityLink(administration, decl.LocalSpec, decl.DeviceID, bareName, capability, provenance, finalAttempt, deviceNamePath)
		}
	}

	if gateway, found := discoveryGatewaysByID[decl.DeviceID]; found {
		discoveryDecl := TDiscoveryEntityDeclaration{
			EntitySpec:      decl.LocalSpec,
			GatewayDeviceID: decl.DeviceID,
			Leaf:            bareName,
			NoCollect:       decl.NoCollect,
		}
		// Fall through to this SAME device id's Logical.def overlay before erroring -- mirrors the
		// hassbridge/import branches' own identical fallback (2026-09-18: a discovery-kind device
		// id, e.g. sensors.terrace_motion, can now ALSO carry real Logical.def capabilities --
		// first real case: "sunny_threshold"/"sunny" alongside the plain discovery-relayed
		// illuminance/temperature/motion/battery_level capabilities).
		if _, capFound := gateway.Capabilities[bareName]; !capFound {
			if logicalDevice, logicalFound := logicalDevicesByID[decl.DeviceID]; logicalFound {
				if _, logicalCapFound := logicalDevice.Capabilities[bareName]; logicalCapFound {
					return dispatchLogicalCapability(administration, decl, logicalDevice, bareName, deviceNamePath, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, logicalDevicesByID, entitiesPath, lineNum, finalAttempt, provenance)
				}
			}
		}
		// A capability whose own source is "... is available;" (capability.AvailabilityOf set)
		// tracks a SIBLING capability's availability rather than a raw gateway leaf -- needs
		// registerDiscoveryAvailabilityEntityLink's own deferred-capable resolution (the sibling
		// may not be positioned yet), not registerDiscoveryEntityLink's raw-leaf path. Unknown
		// capability names fall through to registerDiscoveryEntityLink unchanged, which already
		// reports the "declares no ... capability" warning itself.
		if capability, capFound := gateway.Capabilities[bareName]; capFound && capability.AvailabilityOf != "" {
			return registerDiscoveryAvailabilityEntityLink(administration, discoveryDecl, capability, discoveryGatewaysByID, deviceNamePath, entitiesPath, lineNum, finalAttempt)
		}
		// A "derived DDD.NNN from EEE.MMM via TTT;" capability (DerivedFromCapability set) computes
		// its own value from a SIBLING capability rather than a raw gateway leaf. Two shapes:
		// (1) the sibling is a "hidden" plain raw leaf of the SAME gateway -- relay it directly
		// under this capability's own entity, TTT folded into the MQTT value_template
		// (registerDiscoveryDerivedFromRawLeafEntityLink), no deferral possible or needed.
		// (2) anything else (a real, independently-positioned/materialized capability, or the
		// sibling is itself AvailabilityOf/Derived) -- the sibling's own resolved HA entity_id has
		// to be read via states(...), needing registerDiscoveryDerivedEntityLink's own
		// deferred-capable reverse lookup instead.
		if capability, capFound := gateway.Capabilities[bareName]; capFound && capability.DerivedFromCapability != "" {
			siblingCapability, siblingFound := gateway.Capabilities[capability.DerivedFromCapability]
			if siblingFound && siblingCapability.Hidden && siblingCapability.Leaf != "" && siblingCapability.AvailabilityOf == "" && siblingCapability.DerivedFromCapability == "" {
				return registerDiscoveryDerivedFromRawLeafEntityLink(administration, discoveryDecl, capability, siblingCapability, deviceNamePath, entitiesPath, lineNum), false
			}
			return registerDiscoveryDerivedEntityLink(administration, discoveryDecl, capability, discoveryGatewaysByID, deviceNamePath, entitiesPath, lineNum, finalAttempt)
		}
		return registerDiscoveryEntityLink(administration, discoveryDecl, discoveryGatewaysByID, entitiesPath, lineNum, deviceNamePath, allowHidden), false
	}

	if device, found := hassBridgeDevicesByID[decl.DeviceID]; found {
		capability, capFound := device.Capabilities[bareName]
		if !capFound {
			// Fall through to this SAME device id's Logical.def overlay before erroring -- since
			// the "absorb" operation (2026-09-18) a hassbridge device id can ALSO carry its own
			// Logical.def entry (e.g. appliance.washing_machine's own absorbed "power"/"energy"/
			// "consumes"/"switch.core" from a separate ROBB plug's discovery gateway, plus a
			// wrapping enabler-aware "available" capability referencing this SAME device's own
			// already-positioned raw node) -- dispatchLogicalCapability (below) is the exact same
			// switch the pure-logical branch uses. The imported-device and discovery-gateway
			// branches below have the identical fallback (sensors.vienna_terrace_wind's own
			// "windy_threshold"/"windy", sensors.terrace_motion's own "sunny_threshold"/"sunny",
			// both 2026-09-18); hosts doesn't need it yet (no real case), matching commandline's
			// own pre-existing fall-through above for its own dual-kind case.
			if logicalDevice, logicalFound := logicalDevicesByID[decl.DeviceID]; logicalFound {
				if _, logicalCapFound := logicalDevice.Capabilities[bareName]; logicalCapFound {
					return dispatchLogicalCapability(administration, decl, logicalDevice, bareName, deviceNamePath, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, logicalDevicesByID, entitiesPath, lineNum, finalAttempt, provenance)
				}
			}
			return []string{fmt.Sprintf("%s: device %q declares no %q capability; add a \"<type>.%s: <source>;\" line to its Physical.def declaration", provenance, decl.DeviceID, bareName, bareName)}, false
		}
		return registerDeviceSourceEntityLink(administration, TDeviceSourceEntityDeclaration{
			LocalSpec: decl.LocalSpec,
			// capabilityKey (bareName) is non-empty below, so registerDeviceSourceEntityLink
			// preserves capability's own already-per-instance Sources map unchanged -- this Source
			// value is display-only (the provenance string on a rare error path).
			Source:   representativeSource(capability.Sources),
			DeviceID: decl.DeviceID,
		}, hassBridgeDevicesByID, importedDevicesByID, entitiesPath, lineNum, finalAttempt, bareName, deviceNamePath)
	}

	if importedDevice, found := importedDevicesByID[decl.DeviceID]; found {
		if _, capFound := importedDevice.Capabilities[bareName]; !capFound {
			// Fall through to this SAME device id's Logical.def overlay before erroring -- mirrors
			// the hassbridge branch's own identical fallback above (2026-09-18: an imported device
			// id, e.g. sensors.vienna_terrace_wind, can now ALSO carry real Logical.def capabilities
			// -- first real case: "windy_threshold"/"windy" alongside the plain imported radio/
			// battery_level/wind_direction/wind_speed capabilities).
			if logicalDevice, logicalFound := logicalDevicesByID[decl.DeviceID]; logicalFound {
				if _, logicalCapFound := logicalDevice.Capabilities[bareName]; logicalCapFound {
					return dispatchLogicalCapability(administration, decl, logicalDevice, bareName, deviceNamePath, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, logicalDevicesByID, entitiesPath, lineNum, finalAttempt, provenance)
				}
			}
			return []string{fmt.Sprintf("%s: imported device %q declares no %q capability; add a \"%s: <remote-local-entity>;\" line to its Physical.def import declaration", provenance, decl.DeviceID, bareName, bareName)}, false
		}
		return registerDeviceSourceEntityLink(administration, TDeviceSourceEntityDeclaration{
			LocalSpec: decl.LocalSpec,
			DeviceID:  decl.DeviceID,
		}, hassBridgeDevicesByID, importedDevicesByID, entitiesPath, lineNum, finalAttempt, bareName, deviceNamePath)
	}

	if device, found := hostDevicesByID[decl.DeviceID]; found {
		return registerHostCapabilityEntityLink(administration, device, decl.DeviceID, bareName, provenance, finalAttempt)
	}

	if device, found := logicalDevicesByID[decl.DeviceID]; found {
		return dispatchLogicalCapability(administration, decl, device, bareName, deviceNamePath, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, logicalDevicesByID, entitiesPath, lineNum, finalAttempt, provenance)
	}

	return []string{fmt.Sprintf("%s: device %q not found in Physical.def's \"discovery\" or \"home_assistant\" integrations, Logical.def, or as a \"hassbridge\"-form import", provenance, decl.DeviceID)}, false
}

// dispatchLogicalCapability is the Logical.def device's own capability-shape switch, shared by two
// callers: registerDeviceCapabilityEntityLinkAllowingHidden's own "logicalDevicesByID" branch above
// (a purely logical device id, e.g. appliance.tv), and the hassbridge branch's fallback just above
// (a hassbridge device id that ALSO carries a Logical.def entry -- the "absorb" operation,
// 2026-09-18). Factored out rather than duplicated so both entry points stay byte-identical in
// behaviour as new capability shapes get added here.
func dispatchLogicalCapability(administration *TAdministrationState, decl TDeviceCapabilityEntityDeclaration, device TLogicalDevice, bareName, deviceNamePath string, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, hostDevicesByID map[string]THostDevice, commandlineDevicesByID map[string]TCommandlineDevice, logicalDevicesByID map[string]TLogicalDevice, entitiesPath string, lineNum int, finalAttempt bool, provenance string) (warnings []string, deferred bool) {
	capability, capFound := device.Capabilities[bareName]
	if !capFound {
		return []string{fmt.Sprintf("%s: logical device %q declares no %q capability; add a \"<type>.%s: <value>;\" line to its Logical.def declaration", provenance, decl.DeviceID, bareName, bareName)}, false
	}
	switch {
	case capability.AbsorbedFromDeviceID != "":
		return registerLogicalAbsorbedCapability(administration, decl, capability, bareName, deviceNamePath, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, logicalDevicesByID, entitiesPath, lineNum, finalAttempt)
	case capability.IsDefinedInputNumber:
		return registerLogicalDefinedInputNumberCapability(administration, decl, capability, bareName, deviceNamePath, entitiesPath, lineNum), false
	case capability.IsDerivedCondition:
		return registerLogicalDerivedConditionCapability(administration, decl, capability, bareName, deviceNamePath, entitiesPath, lineNum, finalAttempt, discoveryGatewaysByID)
	case capability.IsAvailable:
		return registerLogicalIsAvailableCapability(administration, decl, capability, bareName, deviceNamePath, entitiesPath, lineNum, hostDevicesByID), false
	case capability.Domain == "switch":
		return registerLogicalMediaSwitchCapability(administration, decl, capability, bareName, deviceNamePath, entitiesPath, lineNum), false
	default:
		return []string{fmt.Sprintf("%s: logical device %q's %q capability isn't a supported shape yet (only \"is available\" conditions, switch<-media_player coercions, absorbed capabilities, defined input_numbers, and derived conditions are built)", provenance, decl.DeviceID, bareName)}, false
	}
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
