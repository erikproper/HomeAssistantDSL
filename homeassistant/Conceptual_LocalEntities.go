/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualLocalEntities
 *
 * Registers a "local" device's own capabilities (integration_local_storage.go's TLocalCapability)
 * once Conceptual.def positions/references them -- the "local" kind's own counterpart to
 * registerHostCapabilityEntityLink/registerHassBridgeAttributeEntity. Two capability shapes (see
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
 *
 * Both shapes populate DeviceConceptualLinks[deviceID].AttributeEntityIDs[label] on success, so
 * Logical.def's existing same-device/cross-device sibling resolution
 * (resolveLogicalSiblingSourceEntityID, Conceptual_LogicalEntities.go) picks up a "local" device's
 * capabilities for free -- no changes needed on that side at all, including for "node" specifically
 * (resolveLogicalSiblingSourceEntityID's own tier-2 check falls through to AttributeEntityIDs["node"]
 * whenever DeviceConceptualLinks.NodeEntityID is empty, exactly as hassbridge's own "node" capability
 * already relies on).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 24.09.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
)

// registerLocalCapabilityEntityLink is registerDeviceCapabilityEntityLinkAllowingHidden's "local"-
// kind branch. finalAttempt/deferred follow the same convention as every other deferred-capable
// branch in this codebase: the parser reads the file once, so a capability line (or this device's
// own auto-registered "node") can be reached before either its own positioning or (for the "is
// available" shape) its sibling's positioning has happened yet; deferred=true with no warning lets
// the single retry (after the whole file is read) resolve it once everything has actually landed.
func registerLocalCapabilityEntityLink(administration *TAdministrationState, decl TDeviceCapabilityEntityDeclaration, capability TLocalCapability, bareName, deviceNamePath, entitiesPath string, lineNum int, finalAttempt bool) (warnings []string, deferred bool) {
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
			Identity:          identity,
			Provenance:        provenance,
			ConditionSources:  sources,
			ConditionExpr:     expr,
			ConditionDevClass: deviceClass,
			EntityIcon:        icon,
			// Generator-authored condition entity -- see this file's own header comment.
			HasDefinitionOrImport: true,
		})
	} else {
		// Bare capability -- see this file's own header comment: registered exactly like a plain
		// bare "entity <spec>;" declaration, HasDefinitionOrImport left false (the zero value).
		administration.AppendEntityRecord(spaceName, TEntityRecord{
			Name:       fullName,
			Identity:   identity,
			Provenance: provenance,
		})
	}

	link.AttributeEntityIDs[bareName] = TDeviceAttributeLink{EntityID: entityID, Identity: identity}
	administration.DeviceConceptualLinks[decl.DeviceID] = link
	return nil, false
}
