/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualDeviceSourceEntities
 *
 * Registers one capability of a "home_assistant" bridge device (or of a "hassbridge"-form import)
 * under the local name a Spaces.def line gives it: "entity <spec> from <device-id> <capability>;",
 * its "for <device-id>:" and "device ... with:" shorthands, and the "node" capability device
 * positioning registers (Conceptual_DeviceCapabilityEntities.go). The capability's label is the
 * key tying its device.Capabilities entry (integration_hassbridge_storage.go's THassBridgeDevice)
 * to its AttributeEntityIDs entry.
 *
 * The device must already be positioned ("device <spec> from <device-id> with: ... end;",
 * Conceptual_DevicePositioning.go), in any space; a reference reached before that is deferred,
 * and warned about only if the positioning never comes.
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
	"strings"
)

// TDeviceSourceEntityDeclaration is one capability to register under a local entity spec.
type TDeviceSourceEntityDeclaration struct {
	LocalSpec string // "binary_sensor.infrastructural:laserjet/status" -- an ordinary entity spec, resolved the same way any other entity's is
	DeviceID  string // "hass.laserjet" from "from hass.laserjet"
	NoCollect bool   // "not aggregated": kept out of the space's aggregates
}

// registerDeviceSourceEntityLink resolves decl.LocalSpec through the normal entity-naming
// machinery (its own domain drives coercion, same as any other entity spec), requires decl.DeviceID
// to already be both a known "home_assistant" bridge device AND already positioned via a prior
// device.<spec> from <device-id> with: ...; (U2) declaration, then records the source/local
// mapping for generateHassBridgeFile (integration_hassbridge_generator.go) to pick up exactly as
// it already does for Physical.def-declared capabilities -- no changes needed there.
//
// capabilityKey is the capability's Physical.def label, the map key used for BOTH
// device.Capabilities and link.AttributeEntityIDs -- device.Capabilities already has an entry under
// that exact key (that's how the caller found capability.Source in the first place), and
// AttributeEntityIDs has to match it. This was a real, live bug (2026-08-28): keying by entity_id
// from that path meant a "co2"-keyed device.Capabilities entry could never be found via
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
//
// deviceNamePath is PROJECT.md item 5's "device declaration acts like a space" sugar: non-empty
// only when decl came from inside a "device <spec> from <device-id> with: ... end;" block
// (Conceptual_DevicePositioning.go), it's the positioned device's own leaf path (deviceSpecLeafPath
// of that block's own spec, e.g. "netatmo") -- appended as one extra path segment when resolving
// decl.LocalSpec, so a capability line inside the block can write "sensor.physical:co2" instead of
// repeating "sensor.physical:netatmo/co2" every time, exactly as a nested "space netatmo with:"
// block would let entities inside it drop its own name too. Deliberately affects ONLY the naming
// call (fullName below) -- administration.CurrentSpaceName() a few lines down (used for space
// bucketing/RegisterDiscoveryImpliedEntity) must keep reading the real, unmodified
// administration.SpacePath, since this device's capabilities still belong to the space the
// positioning line itself sits in, not to a synthetic "netatmo" pseudo-space that was never opened
// via OpenSpace/EnsureSpaceRegistered.
func registerDeviceSourceEntityLink(administration *TAdministrationState, decl TDeviceSourceEntityDeclaration, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, entitiesPath string, lineNum int, finalAttempt bool, capabilityKey string, deviceNamePath string) (warnings []string, deferred bool) {
	provenance := fmt.Sprintf("%s:%d → %s from %s %s", filepath.Base(entitiesPath), lineNum, decl.LocalSpec, decl.DeviceID, capabilityKey)

	if importedDevice, found := importedDevicesByID[decl.DeviceID]; found {
		return registerImportedDeviceSourceEntityLink(administration, decl, importedDevice, entitiesPath, lineNum, finalAttempt, capabilityKey, provenance, deviceNamePath)
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

	fullName := resolveDeviceEntityFullName(decl.LocalSpec, administration.SpacePath, deviceNamePath)
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
	administration.RegisterDiscoveryImpliedEntity(spaceName, fullName, provenance, decl.DeviceID+"!"+key, decl.NoCollect)
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
	// Start from existing (already read above for its own DeviceClass/Unit/StateClass/Icon) rather
	// than a fresh struct literal -- real bug, found live 2026-09-10 while wiring the "derived"
	// capability mechanism (plans/derived-capability-mechanism.md): a fresh literal here silently
	// dropped ValueMap and (once added) DerivedFromCapability/DerivedViaTemplate every time this
	// construct referenced an existing Physical.def-declared capability (capabilityKey != ""),
	// since none of those fields were ever carried over. Only the fields this function actually
	// resolves (Domain/Source/typing) are reassigned below; everything else -- including any field
	// added to THassBridgeCapability in the future -- passes through unchanged.
	updated := existing
	updated.Domain = identity.Domain
	updated.DeviceClass, updated.Unit, updated.StateClass, updated.Icon = deviceClass, unit, stateClass, icon
	device.Capabilities[key] = updated
	// See TDeviceAttributeLink.DisplaySuffix's own doc comment: an empty trailing path in the DSL
	// author's own local spec (e.g. "vacuum.social:") means "nothing more specific than the device
	// itself" -- fall back to the domain word (never key, the raw Physical.def capability label,
	// e.g. "roomba") rather than leaking a physical-layer detail into the conceptual layer's own
	// friendly name. But when the device's OWN leaf already equals the domain (e.g. "device
	// infrastructural:vacuum from appliance.vacuum" -- devBlock.Name already ends in "vacuum"),
	// even the domain word would be a redundant repeat, so omit the suffix entirely.
	//
	// deviceNamePath alone (the device header's own raw "as <leaf>" text) stops being a reliable
	// signal once that clause is gone -- it reads as "no leaf at all" even when the device's own
	// OVERALL identity still carries one via a DIFFERENT entity's own explicit text (typically its
	// "node"), e.g. Junglinster's utility.somfy_david_bedroom ("entity
	// binary_sensor.infrastructural:somfy/node;", no "as" clause ever) still has display_name
	// "house/david_bedroom/somfy" (fixUpDisplayNameFromNode, device_friendly_name.go) even though
	// THIS "cover" capability's own deviceNamePath/LocalSpec contribute nothing -- and "cover" is
	// still the correct, distinguishing display_suffix there (confirmed live against the real,
	// unmodified deployment), not "" -- only Vienna's appliance.vacuum (no "as" clause AND no
	// other entity contributing a leaf at all, since its own leaf WAS "vacuum", matching the
	// domain, before being fully absorbed) should suppress it. Falling back to the device's own
	// node-derived leaf (DeviceNodeEntityID, the SAME choke point resolveDeviceFriendlyName uses)
	// when deviceNamePath is empty distinguishes the two: "vacuum" (fully absorbed into the
	// domain) suppresses, "somfy" (a real, different leaf) doesn't. If the device's own "node"
	// isn't resolved yet (an ordering risk this doesn't fully rule out), falls back to the
	// pre-2026-10-07 behaviour (show the domain word) rather than guessing "" -- the safe default
	// is to show a suffix that might be redundant, not to silently drop a real distinguishing one.
	deviceLeaf := deviceNamePath
	if deviceLeaf == "" {
		if entityID, hasNode := administration.DeviceNodeEntityID[decl.DeviceID]; hasNode {
			if nodeIdentity, found := entityIdentityByID(administration, entityID); found {
				deviceLeaf = stripLastPathSegment(nodeIdentity).Path
				_, contextPath := normalizeSpaceContext(administration.SpacePath)
				if contextPath != "" {
					deviceLeaf = strings.TrimPrefix(strings.TrimPrefix(deviceLeaf, contextPath), "/")
				}
			}
		}
	}
	displaySuffix := key
	if _, isShadow := strings.CutPrefix(key, "slow/"); isShadow {
		displaySuffix = "slow" // a slow-speed shadow (Physical.def "slow speed ...;", 2026-10-23)
	}
	// "vacuum.social:vacuum" (a capability labelled after its own domain, PROJECT.md item 16) names
	// the same entity as "vacuum.social:" -- its trailing domain segment is stripped -- so it gets
	// the same suffix too.
	if deviceSpecLeafPath(stripEntityOwnTrailingDomainSegment(decl.LocalSpec)) == "" {
		if deviceLeaf == identity.Domain {
			displaySuffix = ""
		} else {
			displaySuffix = identity.Domain
		}
	}
	link.AttributeEntityIDs[key] = TDeviceAttributeLink{
		EntityID: entityID, DeviceClass: deviceClass, Unit: unit, StateClass: stateClass, Icon: icon,
		Identity: identity, DisplaySuffix: displaySuffix,
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

func registerImportedDeviceSourceEntityLink(administration *TAdministrationState, decl TDeviceSourceEntityDeclaration, importedDevice TImportedDevice, entitiesPath string, lineNum int, finalAttempt bool, capabilityKey, provenance, deviceNamePath string) (warnings []string, deferred bool) {
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

	fullName := resolveDeviceEntityFullName(decl.LocalSpec, administration.SpacePath, deviceNamePath)
	identity := extractEntityIdentity(fullName)
	if identity.Domain == "" {
		return []string{fmt.Sprintf("%s: could not resolve a domain from %q; skipping", provenance, decl.LocalSpec)}, false
	}

	entityID := toHomeAssistantEntityID(fullName)
	if entityID == "" {
		return []string{fmt.Sprintf("%s: could not resolve a Home Assistant entity id from %q; skipping", provenance, decl.LocalSpec)}, false
	}

	spaceName := administration.CurrentSpaceName()
	administration.RegisterDiscoveryImpliedEntity(spaceName, fullName, provenance, decl.DeviceID+"!"+capabilityKey, decl.NoCollect)

	link.AttributeEntityIDs[capabilityKey] = TDeviceAttributeLink{EntityID: entityID, Identity: identity}
	administration.DeviceConceptualLinks[decl.DeviceID] = link

	return nil, false
}
