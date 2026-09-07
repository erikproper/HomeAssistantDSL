/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualDiscoveryEntities
 *
 * Parses and registers the conceptual layer's "entity <spec> from <gateway-id>.<leaf>;"
 * construct: a link from a Spaces.def-declared entity to a specific auto-discovered leaf entity
 * of a Physical.def "discovery" integration gateway device (e.g. EMS-ESP) -- a device that
 * publishes its own complete, correct Home Assistant MQTT Discovery independently, so unlike the
 * "hosts" integration's device.<spec> construct (Conceptual_DeviceEntities.go), there's no
 * entity-materialization decision to make here: the gateway already decided domain/device_class/
 * unit/etc. itself. All this construct does is say "this conceptual entity id should be the one
 * HA actually sees for that gateway leaf" -- the coordinator's discovery bridge
 * (house_event_bus_coordinator) does the actual relaying, once it's observed that leaf's own
 * discovery payload on the wire (which can't be checked at ./configure-time -- the gateway's
 * entity set is genuinely unknown until then).
 *
 * Like device.<spec> entities, none of these are generator-authored YAML -- they're registered
 * discovery-implied (TEntityRecord.DiscoveryImplied), reusing RegisterDiscoveryImpliedEntity
 * exactly as device.<spec> entities already do (so they're correctly excluded from
 * generateCustomizationFiles/space-level aggregates too, with no changes needed there).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 22.08.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// TDiscoveryEntityDeclaration is one parsed "entity <spec> from <gateway-id>.<leaf>;" line.
type TDiscoveryEntityDeclaration struct {
	EntitySpec      string // "sensor.social:garage_door/temperature" -- an ordinary entity spec, resolved the same way any other entity's is
	GatewayDeviceID string // "discovery.ems_esp" -- everything in the from-clause before its LAST "."
	Leaf            string // "boiler_outdoortemp" -- the from-clause's final segment
}

// discoveryEntityPattern requires the from-clause to end directly in ";" with no "with:" clause
// -- this is what distinguishes it from "entity device.<spec> from <device-id> with: ...;"
// (Conceptual_DeviceEntities.go), which must be checked first in the parse loop since its own
// from-clause could otherwise be mistaken for this simpler shape's target.
var discoveryEntityPattern = regexp.MustCompile(`^entity (\S+) from (\S+);$`)

// extractDiscoveryEntityDeclaration recognises the "entity <spec> from <gateway-id>.<leaf>;"
// shape. The from-clause's target is split on its LAST "." -- not its first -- since the gateway
// device id itself may contain a dot (e.g. "discovery.ems_esp", per Note 1: the "discovery."
// prefix is just a naming convention, not special syntax).
func extractDiscoveryEntityDeclaration(line string) (*TDiscoveryEntityDeclaration, bool) {
	matches := discoveryEntityPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil, false
	}
	target := matches[2]
	lastDot := strings.LastIndex(target, ".")
	if lastDot <= 0 || lastDot >= len(target)-1 {
		return nil, false
	}
	return &TDiscoveryEntityDeclaration{
		EntitySpec:      matches[1],
		GatewayDeviceID: target[:lastDot],
		Leaf:            target[lastDot+1:],
	}, true
}

// registerDiscoveryEntityLink resolves decl.EntitySpec through the normal entity-naming
// machinery (whatever domain/sphere/path the spec itself declares -- unlike device.<spec>, this
// construct doesn't default to any particular sphere, since the entity could be any kind of
// thing: a sensor, a climate, a binary_sensor), resolves decl.GatewayDeviceID against
// discoveryGatewaysByID, and records the resulting TDiscoveryEntityLink on
// administration.DiscoveryEntityLinks for generateDiscoveryIntegrationOutputs to pick up.
// Returns warnings; never aborts parsing.
func registerDiscoveryEntityLink(administration *TAdministrationState, decl TDiscoveryEntityDeclaration, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, entitiesPath string, lineNum int) []string {
	provenance := fmt.Sprintf("%s:%d → %s from %s.%s", filepath.Base(entitiesPath), lineNum, decl.EntitySpec, decl.GatewayDeviceID, decl.Leaf)

	gateway, found := discoveryGatewaysByID[decl.GatewayDeviceID]
	if !found {
		return []string{fmt.Sprintf("%s: gateway device %q not found in Physical.def's \"discovery\" integration", provenance, decl.GatewayDeviceID)}
	}
	// decl.Leaf is the DSL-chosen, stable local name declared on that device's own
	// "<type>.<local-name>: <source-leaf>;" capability line -- never the raw, gateway-native
	// leaf id directly (that would reintroduce the naming-stability problem this indirection
	// exists to avoid). Resolve it here, once, generator-side.
	capability, capabilityFound := gateway.Capabilities[decl.Leaf]
	if !capabilityFound {
		return []string{fmt.Sprintf("%s: device %q declares no %q capability; add a \"<type>.%s: <source-leaf>;\" line to its Physical.def declaration", provenance, decl.GatewayDeviceID, decl.Leaf, decl.Leaf)}
	}

	fullName := normalizeEntityFullName(decl.EntitySpec, administration.SpacePath)
	identity := extractEntityIdentity(fullName)
	if identity.Domain == "" {
		return []string{fmt.Sprintf("%s: could not resolve a domain from %q; skipping", provenance, decl.EntitySpec)}
	}

	entityID := toHomeAssistantEntityID(fullName)
	if entityID == "" {
		return []string{fmt.Sprintf("%s: could not resolve a Home Assistant entity id from %q; skipping", provenance, decl.EntitySpec)}
	}

	spaceName := administration.CurrentSpaceName()
	administration.RegisterDiscoveryImpliedEntity(spaceName, fullName, provenance, decl.GatewayDeviceID+"!"+decl.Leaf)
	deviceClass, unit, stateClass, icon := resolveCapabilityDefaults(administration.CapabilityDefaults, identity.Domain, identity.Path)
	administration.DiscoveryEntityLinks[entityID] = TDiscoveryEntityLink{
		EntityID:        entityID,
		GatewayDeviceID: decl.GatewayDeviceID,
		Leaf:            capability.Leaf,
		DeviceClass:     deviceClass,
		Unit:            unit,
		StateClass:      stateClass,
		Icon:            icon,
	}
	return nil
}
