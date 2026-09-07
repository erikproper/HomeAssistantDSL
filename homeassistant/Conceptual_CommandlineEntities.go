/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualCommandlineEntities
 *
 * Registers the conceptual layer's "entity <local-spec> from <device-id> entity <capability>;"
 * construct (Conceptual_DeviceCapabilityEntities.go) for a "commandline" integration device's own
 * switch/sensor/button capability -- lets a script-backed entity get a real conceptual position
 * (e.g. "switch.social:picture_frame") instead of the coordinator's own auto-derived
 * "switch.<host>_<name>" fallback (house_event_bus_coordinator/discoverycommandline.go), the same
 * positioning freedom every other device kind's capabilities already have.
 *
 * Unlike registerHostAttributeEntity/registerDeviceSourceEntityLink, there's no per-attribute
 * typing metadata to resolve here (a commandline capability declares no device_class/unit/
 * state_class anywhere in Physical.def) -- this only ever resolves the entity id itself, which
 * generateCommandlineCoordinatorFile (integration_commandline_generator.go) reads back out of
 * DeviceConceptualLinks to pass through to the coordinator as the capability's desired entity_id/
 * display name.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 07.09.2026
 *
 */

package main

import "fmt"

// registerCommandlineCapabilityEntityLink resolves localSpec (e.g. "switch.social:picture_frame")
// against a commandline device's own already-declared capability, and records the resulting entity
// id (plus a display name derived the same way registerHostAttributeEntity's does) into the
// device's shared DeviceConceptualLink -- the same link a "hosts"/"home_assistant" declaration of
// the SAME deviceID already established via its own "device <spec> from <device-id> [with: ...];"
// positioning line (deliberately reused, not re-created, for a device that shares its identity
// across kinds -- see discoverycommandline.go's own doc comment for why host.frame does this).
//
// A domain mismatch between localSpec's own domain (e.g. "binary_sensor") and capability.Kind
// (e.g. "switch") is rejected outright, not coerced: unlike a hassbridge capability (whose Source
// really is a distinct, independently-typed upstream entity a domain coercion can genuinely make
// sense for), a commandline capability's Kind IS the entity's real domain as the coordinator will
// actually publish it -- a mismatched local spec would silently produce a broken reference no
// coercion could fix.
func registerCommandlineCapabilityEntityLink(administration *TAdministrationState, localSpec, deviceID, bareName string, capability TCommandlineCapability, provenance string, finalAttempt bool) (warnings []string, deferred bool) {
	link, hasLink := administration.DeviceConceptualLinks[deviceID]
	if !hasLink {
		if !finalAttempt {
			return nil, true
		}
		return []string{fmt.Sprintf("%s: device %q has no \"device.<spec> from %s;\" positioning yet -- add one (any space) before referencing one of its entities directly", provenance, deviceID, deviceID)}, true
	}

	fullName := normalizeEntityFullName(localSpec, administration.SpacePath)
	identity := extractEntityIdentity(fullName)
	if identity.Domain == "" {
		return []string{fmt.Sprintf("%s: could not resolve a domain from %q; skipping", provenance, localSpec)}, false
	}
	if identity.Domain != capability.Kind {
		return []string{fmt.Sprintf("%s: capability %q is a %q, but %q declares domain %q; they must match", provenance, bareName, capability.Kind, localSpec, identity.Domain)}, false
	}

	entityID := toHomeAssistantEntityID(fullName)
	if entityID == "" {
		return []string{fmt.Sprintf("%s: could not resolve a Home Assistant entity id from %q; skipping", provenance, localSpec)}, false
	}

	spaceName := administration.CurrentSpaceName()
	administration.RegisterDiscoveryImpliedEntity(spaceName, fullName, provenance, deviceID+"!"+bareName)

	if link.AttributeEntityIDs == nil {
		link.AttributeEntityIDs = map[string]TDeviceAttributeLink{}
	}
	link.AttributeEntityIDs[bareName] = TDeviceAttributeLink{EntityID: entityID, Identity: identity}
	administration.DeviceConceptualLinks[deviceID] = link
	return nil, false
}
