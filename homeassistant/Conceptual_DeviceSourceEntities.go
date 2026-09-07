/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualDeviceSourceEntities
 *
 * Parses and registers the conceptual layer's "entity <local-spec> as <source> from <device-id>;"
 * construct: positions ONE specific entity a "home_assistant" bridge device exposes (its <source>
 * is the remote instance's own raw entity reference, e.g. "sensor.hewlett_packard_..." or
 * "switch.yyy!some_attribute" -- the same "entity!attribute" convention hosts/hassbridge
 * capability values already use) under a local name/type declared directly in Spaces.def --
 * without needing a matching Physical.def "device hass.X with: <capability>: <entity>; end;"
 * capability line at all. The local <type> may deliberately differ from <source>'s own domain
 * (e.g. "binary_sensor.xxx as switch.yyy from zzz") -- a type *coercion*, not a mismatch: the
 * coordinator's relay already keys its discovery topic/domain off the LOCAL entity_id
 * (hassBridgeDiscoveryTopic, house_event_bus_coordinator/discoveryhassbridge.go), so this needs no
 * extra machinery here; it's the DSL author's job to only coerce between genuinely
 * state-value-compatible domains (switch/binary_sensor's "on"/"off" being the obvious case).
 *
 * Scope: "home_assistant" bridge devices only for now -- "hosts" devices don't have a comparable
 * "capability value is a raw foreign entity reference" story without also reaching back into
 * Physical.def-stage reporting-automation generation (which runs before Spaces.def is even parsed
 * today), a separate, not-yet-designed piece of work (see the plan's own build order).
 *
 * A device.<spec> from <device-id> with: attributes;/end; positioning (U2, Conceptual_DeviceEntities.go)
 * must already exist for <device-id> before this construct (U1) is used -- warned, not silently
 * auto-created, if missing; it doesn't have to be in the same space. The synthetic capability/
 * attribute-link key used to tie this declaration's Capabilities entry (integration_hassbridge_storage.go's
 * THassBridgeDevice, mutated in place here) to its AttributeEntityIDs entry is the resolved local
 * entity_id itself -- always unique, avoids colliding with a real Physical.def-declared capability
 * name. NOTE: if this construct's device.<spec> U2 positioning also uses plain "with: attributes;"
 * (not yet built: "unused attributes") and is declared AFTER this line in Spaces.def, its
 * blanket expansion will also pick up this synthetic capability and register a second, redundant
 * local entity for the same source -- acceptable today (the DSL author asked for "everything"),
 * to be resolved once "unused attributes" (out of scope here) lands.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 24.08.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"regexp"
)

// TDeviceSourceEntityDeclaration is one parsed "entity <local-spec> as <source> from <device-id>;"
// line.
type TDeviceSourceEntityDeclaration struct {
	LocalSpec string // "binary_sensor.infrastructural:laserjet/status" -- an ordinary entity spec, resolved the same way any other entity's is
	Source    string // "sensor.hewlett_packard_...!some_attribute" -- the remote instance's own raw entity reference, "!attribute" optional
	DeviceID  string // "hass.laserjet" from "from hass.laserjet"
}

// deviceSourceEntityPattern requires an explicit " as " clause -- this is what distinguishes it
// from "entity <spec> from <gateway-id>.<leaf>;" (Conceptual_DiscoveryEntities.go), which has no
// "as" clause at all, so there's no ambiguity between the two shapes.
var deviceSourceEntityPattern = regexp.MustCompile(`^entity (\S+) as (\S+) from (\S+);$`)

func extractDeviceSourceEntityDeclaration(line string) (*TDeviceSourceEntityDeclaration, bool) {
	matches := deviceSourceEntityPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil, false
	}
	return &TDeviceSourceEntityDeclaration{
		LocalSpec: matches[1],
		Source:    matches[2],
		DeviceID:  matches[3],
	}, true
}

// registerDeviceSourceEntityLink resolves decl.LocalSpec through the normal entity-naming
// machinery (its own domain drives coercion, same as any other entity spec), requires decl.DeviceID
// to already be both a known "home_assistant" bridge device AND already positioned via a prior
// device.<spec> from <device-id> with: ...; (U2) declaration, then records the source/local
// mapping for generateHassBridgeFile (integration_hassbridge_generator.go) to pick up exactly as
// it already does for Physical.def-declared capabilities -- no changes needed there.
//
// capabilityKey is the map key used for BOTH device.Capabilities and link.AttributeEntityIDs.
// Empty means "use the resolved entity_id" -- this construct's own default, for the "as" form:
// there's no pre-existing Physical.def capability name to reuse, so the always-unique entity_id is
// the only sensible key (see this file's header comment). A caller that already resolved a real
// Physical.def capability label (registerDeviceCapabilityEntityLink, for the
// "entity ... from <device-id> entity <capability>;" construct, and registerDevicePositioning, for
// the "node" capability it auto-registers) MUST pass that bare label instead -- device.Capabilities
// already has an entry under that exact key (that's how the caller found capability.Source in the
// first place), and AttributeEntityIDs has to match it. This was a real, live bug (2026-08-28):
// passing "" from that path meant a "co2"-keyed device.Capabilities entry could never be found via
// an entity_id-keyed AttributeEntityIDs, so generateHassBridgeFile silently skipped every
// capability positioned via that construct -- confirmed against real Junglinster output
// (hass.davids_bedroom's Netatmo capabilities were entirely absent from
// coordinator/homeassistant_bridge.yaml).
//
// The parser reads the file exactly once, in source order, so a U1 line can be reached before its
// device's U2 positioning line appears later in the file. finalAttempt distinguishes the two times
// this can be called for the same declaration: false during the single main pass -- if the device
// isn't positioned yet, deferred=true and no warning is produced, since the positioning might
// still arrive later in the same file; true for the one retry ParseEntitiesAndFillAdministration
// makes over its small pending-list (already-parsed declarations, not a second scan of the file)
// once the whole file has been read -- only then does "still not positioned" become a real,
// reported warning. Every other return path is unaffected by finalAttempt: those failures (unknown
// device, unresolvable domain/entity id) can never be fixed by something appearing later in the
// file, so they're always final.
func registerDeviceSourceEntityLink(administration *TAdministrationState, decl TDeviceSourceEntityDeclaration, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, entitiesPath string, lineNum int, finalAttempt bool, capabilityKey string) (warnings []string, deferred bool) {
	provenance := fmt.Sprintf("%s:%d → %s as %s from %s", filepath.Base(entitiesPath), lineNum, decl.LocalSpec, decl.Source, decl.DeviceID)

	if importedDevice, found := importedDevicesByID[decl.DeviceID]; found {
		return registerImportedDeviceSourceEntityLink(administration, decl, importedDevice, entitiesPath, lineNum, finalAttempt, capabilityKey, provenance)
	}

	device, found := hassBridgeDevicesByID[decl.DeviceID]
	if !found {
		return []string{fmt.Sprintf("%s: device %q not found in Physical.def's \"home_assistant\" integration or as a \"hassbridge\"-form import", provenance, decl.DeviceID)}, false
	}

	link, hasLink := administration.DeviceConceptualLinks[decl.DeviceID]
	if !hasLink {
		if !finalAttempt {
			return nil, true
		}
		return []string{fmt.Sprintf("%s: device %q has no \"device.<spec> from %s with: ...;\" positioning yet -- add one (any space) before referencing one of its entities directly", provenance, decl.DeviceID, decl.DeviceID)}, true
	}

	fullName := normalizeEntityFullName(decl.LocalSpec, administration.SpacePath)
	identity := extractEntityIdentity(fullName)
	if identity.Domain == "" {
		return []string{fmt.Sprintf("%s: could not resolve a domain from %q; skipping", provenance, decl.LocalSpec)}, false
	}

	entityID := toHomeAssistantEntityID(fullName)
	if entityID == "" {
		return []string{fmt.Sprintf("%s: could not resolve a Home Assistant entity id from %q; skipping", provenance, decl.LocalSpec)}, false
	}

	spaceName := administration.CurrentSpaceName()
	key := capabilityKey
	if key == "" {
		key = entityID
	}
	sourceCapability := capabilityKey
	if sourceCapability == "" {
		// Bare "entity <spec> as <source> from <device-id>;" form: decl.Source (the raw remote
		// reference, e.g. "sensor.some_integration_entity") is the only stable capability identity
		// available -- entityID would be tautologically unique per conceptual entity and could never
		// detect reuse.
		sourceCapability = decl.Source
	}
	administration.RegisterDiscoveryImpliedEntity(spaceName, fullName, provenance, decl.DeviceID+"!"+sourceCapability)
	if device.Capabilities == nil {
		device.Capabilities = map[string]THassBridgeCapability{}
		hassBridgeDevicesByID[decl.DeviceID] = device
	}
	// Seed any of device_class/unit/state_class/icon the DSL left unset -- same precedence as
	// registerHassBridgeAttributeEntity's own path: an explicit device_class/unit/state_class/icon
	// Physical.def already declared on this exact capability (existing, preserved below rather
	// than overwritten) always wins; Defaults.def/code-level postfix (resolveCapabilityDefaults)
	// only fills what's left unset. This construct never resolved typing metadata at all before --
	// a real bug, confirmed live 2026-08-29: every capability positioned via "entity ... from
	// <device-id> entity <capability>;" (including its "for <device-id>: ...;"/merged-block
	// shorthand) silently lost its unit/device_class/state_class/icon in HA, even though the
	// sibling bulk-import path never had this gap.
	existing := device.Capabilities[key]
	deviceClass, unit, stateClass, icon := existing.DeviceClass, existing.Unit, existing.StateClass, existing.Icon
	defaultDeviceClass, defaultUnit, defaultStateClass, defaultIcon := resolveCapabilityDefaults(administration.CapabilityDefaults, identity.Domain, key)
	if deviceClass == "" {
		deviceClass = defaultDeviceClass
	}
	if unit == "" {
		unit = defaultUnit
	}
	if stateClass == "" {
		stateClass = defaultStateClass
	}
	if icon == "" {
		icon = defaultIcon
	}
	sources := existing.Sources
	if capabilityKey == "" {
		// Bare "entity <spec> as <source> from <device-id>;" form -- decl.Source is DSL-author-
		// supplied directly in Spaces.def, with no per-instance context available here (unlike a
		// Physical.def capability line, which always knows its own declaring instance). Applied
		// identically across every one of the device's currently known instances -- this form
		// predates roaming's per-instance sources and was never meant to distinguish them.
		sources = map[string]string{}
		for _, instance := range device.Instances {
			sources[instance] = decl.Source
		}
	}
	device.Capabilities[key] = THassBridgeCapability{
		Domain: identity.Domain, Sources: sources,
		DeviceClass: deviceClass, Unit: unit, StateClass: stateClass, Icon: icon,
	}
	link.AttributeEntityIDs[key] = TDeviceAttributeLink{
		EntityID: entityID, DeviceClass: deviceClass, Unit: unit, StateClass: stateClass, Icon: icon,
		Identity: identity,
	}
	administration.DeviceConceptualLinks[decl.DeviceID] = link

	return nil, false
}

// registerImportedDeviceSourceEntityLink is registerDeviceSourceEntityLink's counterpart for a
// "hassbridge"-form import (PROJECT.md 1.2d, integration_import_hassbridge_storage.go) --
// deliberately much simpler than the native path: there's no per-device Capabilities map to seed
// or mutate (device.Capabilities/THassBridgeCapability is a native-hassbridge-only concept), and
// no typing metadata to resolve here at all -- an imported entity's device_class/unit/state_class/
// icon come from the exporting installation's own already-resolved discovery payload, learned live
// by the coordinator (house_event_bus_coordinator/discoveryimport.go), never known at generate
// time on this side. This function's only job is to compute the LOCAL entity id Spaces.def's own
// positioning implies and record it in link.AttributeEntityIDs -- exactly what
// generateImportedDeviceFile (integration_import_hassbridge_generator.go) reads to tell the
// coordinator which local entity id to publish each capability under, instead of the coordinator
// inventing its own generic naming.
//
// The bare "entity <spec> as <source> from <device-id>;" form (capabilityKey == "", decl.Source
// DSL-author-supplied) makes no sense for an import -- there's no "raw remote entity string" the
// DSL author can meaningfully supply here, only an already-Physical.def-declared capability name
// (via the "for <device-id>: entity <spec> from entity <capability>;" shorthand,
// Conceptual_DeviceCapabilityEntities.go, which always passes a non-empty capabilityKey) -- so
// that form is rejected with a clear message rather than silently accepted and never actually
// wired to anything.
func registerImportedDeviceSourceEntityLink(administration *TAdministrationState, decl TDeviceSourceEntityDeclaration, importedDevice TImportedDevice, entitiesPath string, lineNum int, finalAttempt bool, capabilityKey, provenance string) (warnings []string, deferred bool) {
	if capabilityKey == "" {
		return []string{fmt.Sprintf("%s: device %q is an import -- the \"entity ... as <source> from ...;\" form isn't supported for imports (there's no local raw entity reference to give); use \"for %s: entity <spec> from entity <capability>;\" referencing an already-declared Physical.def capability instead", provenance, decl.DeviceID, decl.DeviceID)}, false
	}
	if _, declared := importedDevice.Capabilities[capabilityKey]; !declared {
		return []string{fmt.Sprintf("%s: imported device %q declares no %q capability; add a \"%s: <remote-local-entity>;\" line to its Physical.def import declaration", provenance, decl.DeviceID, capabilityKey, capabilityKey)}, false
	}

	link, hasLink := administration.DeviceConceptualLinks[decl.DeviceID]
	if !hasLink {
		if !finalAttempt {
			return nil, true
		}
		return []string{fmt.Sprintf("%s: device %q has no \"device.<spec> from %s with: ...;\" positioning yet -- add one (any space) before referencing one of its entities directly", provenance, decl.DeviceID, decl.DeviceID)}, true
	}

	fullName := normalizeEntityFullName(decl.LocalSpec, administration.SpacePath)
	identity := extractEntityIdentity(fullName)
	if identity.Domain == "" {
		return []string{fmt.Sprintf("%s: could not resolve a domain from %q; skipping", provenance, decl.LocalSpec)}, false
	}

	entityID := toHomeAssistantEntityID(fullName)
	if entityID == "" {
		return []string{fmt.Sprintf("%s: could not resolve a Home Assistant entity id from %q; skipping", provenance, decl.LocalSpec)}, false
	}

	spaceName := administration.CurrentSpaceName()
	administration.RegisterDiscoveryImpliedEntity(spaceName, fullName, provenance, decl.DeviceID+"!"+capabilityKey)

	link.AttributeEntityIDs[capabilityKey] = TDeviceAttributeLink{EntityID: entityID, Identity: identity}
	administration.DeviceConceptualLinks[decl.DeviceID] = link

	return nil, false
}
