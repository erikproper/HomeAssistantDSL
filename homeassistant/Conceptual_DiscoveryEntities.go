/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualDiscoveryEntities
 *
 * Parses and registers the conceptual layer's "entity <spec> from <gateway-id> with <leaf>;"
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

// TDiscoveryEntityDeclaration is one parsed "entity <spec> from <gateway-id> with <leaf>;" line.
type TDiscoveryEntityDeclaration struct {
	EntitySpec      string // "sensor.social:garage_door/temperature" -- an ordinary entity spec, resolved the same way any other entity's is
	GatewayDeviceID string // "discovery.ems_esp"
	Leaf            string // "boiler_outdoortemp"
	// NoCollect excludes this entity from its space's domain-specific aggregate collections
	// (mirrors the plain-entity "no_collect" suffix, TEntityRecord.NoCollect) -- needed for a raw
	// gateway leaf that happens to share a subdomain (e.g. "/temperature") with real room-climate
	// sensors but isn't one (a smart plug's own PCB temperature reading, say).
	NoCollect bool
}

// discoveryEntityPattern requires the line to end directly in ";" (optionally preceded by a
// trailing "with no_collect" -- same single-property shorthand a bare "entity <spec> with <X>;"
// declaration already uses, analyzeEntityDefinitionContext) with no "with: ... end;" block -- this
// is what distinguishes it from "entity device.<spec> from <device-id> with: ...;"
// (Conceptual_DeviceEntities.go), which must be checked first in the parse loop since its own
// from-clause could otherwise be mistaken for this simpler shape's target. The gateway and leaf are
// separated by the literal word "with" rather than a "." -- gateway device ids are themselves
// routinely dotted (e.g. "discovery.ems_esp", per Note 1: the "discovery." prefix is just a naming
// convention, not special syntax), so splitting on a dot was ambiguous/fragile; an explicit keyword
// removes the guesswork entirely. Real syntax change, 2026-09-14 -- no back-compat kept for the old
// "from <gateway-id>.<leaf>;" dotted form.
var discoveryEntityPattern = regexp.MustCompile(`^entity\s+(\S+)\s+from\s+(\S+)\s+with\s+(\S+?)(?:\s+with\s+no_collect)?;$`)

// extractDiscoveryEntityDeclaration recognises the "entity <spec> from <gateway-id> with <leaf>
// [with no_collect];" shape.
func extractDiscoveryEntityDeclaration(line string) (*TDiscoveryEntityDeclaration, bool) {
	matches := discoveryEntityPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil, false
	}
	return &TDiscoveryEntityDeclaration{
		EntitySpec:      matches[1],
		GatewayDeviceID: matches[2],
		Leaf:            matches[3],
		NoCollect:       strings.HasSuffix(line, "with no_collect;"),
	}, true
}

// registerDiscoveryEntityLink resolves decl.EntitySpec through the normal entity-naming
// machinery (whatever domain/sphere/path the spec itself declares -- unlike device.<spec>, this
// construct doesn't default to any particular sphere, since the entity could be any kind of
// thing: a sensor, a climate, a binary_sensor), resolves decl.GatewayDeviceID against
// discoveryGatewaysByID, and records the resulting TDiscoveryEntityLink on
// administration.DiscoveryEntityLinks for generateDiscoveryIntegrationOutputs to pick up.
// Returns warnings; never aborts parsing.
//
// deviceNamePath (PROJECT.md item 7/item 5, 2026-09-15) is non-empty only when decl came from
// inside a "device <spec> from discovery.<gateway> with: ... end;" block body -- the positioned
// device's own leaf path, appended as one extra naming segment via namingSpacePath exactly as the
// hosts/hassbridge/imported paths already do (registerDeviceSourceEntityLink), so e.g.
// "entity light.physical: from core;" inside such a block resolves using the block's own leaf
// (e.g. "left/1") instead of needing it spelled out on every line. Empty for the flat top-level
// "entity <spec> from <gateway> with <leaf>;" form, which has no enclosing device block to borrow
// a leaf from.
//
// capability.AvailabilityOf capabilities (the "<domain>.<label>: <sibling> is available;" sugar)
// are rejected here with a warning directing the author to registerDiscoveryAvailabilityEntityLink
// instead -- this function only ever builds a raw-MQTT-sourced link, never a condition entity.
// allowHidden is true only for registerDevicePositioning's own auto-implied registration of a
// hidden capability's underlying entity -- every ordinary DSL-authored line (both the flat
// "entity ... from <gateway-id> with <leaf>;" form and the device-block form's own fallthrough)
// passes false, so a hidden capability is refused wherever a DSL author could have written it.
func registerDiscoveryEntityLink(administration *TAdministrationState, decl TDiscoveryEntityDeclaration, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, entitiesPath string, lineNum int, deviceNamePath string, allowHidden bool) []string {
	provenance := fmt.Sprintf("%s:%d → %s from %s with %s", filepath.Base(entitiesPath), lineNum, decl.EntitySpec, decl.GatewayDeviceID, decl.Leaf)

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
	if capability.AvailabilityOf != "" {
		return []string{fmt.Sprintf("%s: %q is an \"... is available;\" capability, not a raw leaf -- use it from inside a \"device ... from %s with:\" block so its sibling reference can resolve", provenance, decl.Leaf, decl.GatewayDeviceID)}
	}
	if capability.DerivedFromCapability != "" {
		return []string{fmt.Sprintf("%s: %q is a \"derived\" capability, not a raw leaf -- use it from inside a \"device ... from %s with:\" block so its sibling reference can resolve", provenance, decl.Leaf, decl.GatewayDeviceID)}
	}
	if capability.Hidden && !allowHidden {
		return []string{fmt.Sprintf("%s: %q is a \"hidden\" capability -- it may only feed another capability's own \"derived ... from ...;\" declaration in Physical.def, not be positioned directly", provenance, decl.Leaf)}
	}

	fullName := resolveDeviceEntityFullName(decl.EntitySpec, administration.SpacePath, deviceNamePath)
	identity := extractEntityIdentity(fullName)
	if identity.Domain == "" {
		return []string{fmt.Sprintf("%s: could not resolve a domain from %q; skipping", provenance, decl.EntitySpec)}
	}

	entityID := toHomeAssistantEntityID(fullName)
	if entityID == "" {
		return []string{fmt.Sprintf("%s: could not resolve a Home Assistant entity id from %q; skipping", provenance, decl.EntitySpec)}
	}

	spaceName := administration.CurrentSpaceName()
	administration.RegisterDiscoveryImpliedEntity(spaceName, fullName, provenance, decl.GatewayDeviceID+"!"+decl.Leaf, decl.NoCollect)
	deviceClass, unit, stateClass, icon := resolveCapabilityDefaults(administration.CapabilityDefaults, identity.Domain, identity.Path)
	administration.DiscoveryEntityLinks[entityID] = TDiscoveryEntityLink{
		EntityID:        entityID,
		GatewayDeviceID: decl.GatewayDeviceID,
		Leaf:            capability.Leaf,
		SourceDomain:    capability.SourceDomain,
		DeviceClass:     deviceClass,
		Unit:            unit,
		StateClass:      stateClass,
		Icon:            icon,
	}
	return nil
}

// registerDiscoveryDerivedFromRawLeafEntityLink resolves a "derived DDD.NNN from EEE.MMM via
// TTT;" capability whose sibling EEE.MMM (siblingCapability) is itself a "hidden" plain raw-leaf
// capability of the SAME gateway -- rather than materializing EEE.MMM as its own local HA entity
// AND a separate condition/template entity computing off it (registerDiscoveryDerivedEntityLink's
// approach, needed when the sibling is a real, independently-visible entity), this relays the
// sibling's OWN raw gateway+leaf directly under THIS capability's entity, with TTT folded into the
// relayed MQTT config's own value_template (TDiscoveryEntityLink.ValueTemplateWrap,
// buildRelayedDiscoveryConfig in the coordinator) -- one MQTT-discovered entity, computed at the
// source, instead of two entities (a hidden materialized raw one plus a locally-computed one).
//
// No deferral needed at all: unlike registerDiscoveryAvailabilityEntityLink/
// registerDiscoveryDerivedEntityLink (which reverse-lookup a sibling's own Spaces.def-resolved
// entity_id, dependent on positioning order), this reads the sibling's raw leaf directly off
// Physical.def's own gateway.Capabilities map -- already fully known the moment Physical.def
// itself was parsed, independent of anything Spaces.def does.
func registerDiscoveryDerivedFromRawLeafEntityLink(administration *TAdministrationState, decl TDiscoveryEntityDeclaration, capability, siblingCapability TDiscoveryCapability, deviceNamePath, entitiesPath string, lineNum int) []string {
	provenance := fmt.Sprintf("%s:%d → %s from %s with %s", filepath.Base(entitiesPath), lineNum, decl.EntitySpec, decl.GatewayDeviceID, decl.Leaf)

	fullName := resolveDeviceEntityFullName(decl.EntitySpec, administration.SpacePath, deviceNamePath)
	identity := extractEntityIdentity(fullName)
	if identity.Domain == "" {
		return []string{fmt.Sprintf("%s: could not resolve a domain from %q; skipping", provenance, decl.EntitySpec)}
	}
	entityID := toHomeAssistantEntityID(fullName)
	if entityID == "" {
		return []string{fmt.Sprintf("%s: could not resolve a Home Assistant entity id from %q; skipping", provenance, decl.EntitySpec)}
	}

	spaceName := administration.CurrentSpaceName()
	administration.RegisterDiscoveryImpliedEntity(spaceName, fullName, provenance, decl.GatewayDeviceID+"!"+decl.Leaf, decl.NoCollect)
	deviceClass, unit, stateClass, icon := resolveCapabilityDefaults(administration.CapabilityDefaults, identity.Domain, identity.Path)
	administration.DiscoveryEntityLinks[entityID] = TDiscoveryEntityLink{
		EntityID:          entityID,
		GatewayDeviceID:   decl.GatewayDeviceID,
		Leaf:              siblingCapability.Leaf,
		SourceDomain:      siblingCapability.SourceDomain,
		DeviceClass:       deviceClass,
		Unit:              unit,
		StateClass:        stateClass,
		Icon:              icon,
		ValueTemplateWrap: capability.DerivedViaTemplate,
	}
	return nil
}

// registerDiscoveryAvailabilityEntityLink resolves an "<domain>.<label>: <sibling> is available;"
// capability (capability.AvailabilityOf names a sibling capability of the SAME gateway device) into
// a plain condition-based entity -- "$1 not in ['unavailable', 'unknown']" over whatever entity_id
// the sibling capability itself resolved to, exactly the same shape "condition <source> is
// available;" (expander.go's own sugar) already produces for a hand-written Macros.def condition.
//
// Deliberately NOT a raw-MQTT-sourced discovery link: MQTT discovery gateways aren't uniformly
// controlled the way hosts/hassbridge are (PROJECT.md item 7, 2026-09-15) -- Zigbee2MQTT's own
// availability conventions can't be assumed to generalise, so "node" (or any other
// availability-tracking capability) is always an explicit condition over a specific sibling
// capability's own resolved entity, never an auto-registered default.
//
// The sibling might not have been positioned yet (its own "entity ... from <leaf>;" line can
// appear later in the same file, e.g. after this "node" line) -- deferred=true with no warning
// lets the caller retry once after the whole file has been read, exactly like
// registerDeviceSourceEntityLink's own hassbridge/imported deferral.
func registerDiscoveryAvailabilityEntityLink(administration *TAdministrationState, decl TDiscoveryEntityDeclaration, capability TDiscoveryCapability, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, deviceNamePath, entitiesPath string, lineNum int, finalAttempt bool) (warnings []string, deferred bool) {
	provenance := fmt.Sprintf("%s:%d → %s from %s with %s", filepath.Base(entitiesPath), lineNum, decl.EntitySpec, decl.GatewayDeviceID, decl.Leaf)

	gateway, found := discoveryGatewaysByID[decl.GatewayDeviceID]
	if !found {
		return []string{fmt.Sprintf("%s: gateway device %q not found in Physical.def's \"discovery\" integration", provenance, decl.GatewayDeviceID)}, false
	}
	siblingCapability, siblingFound := gateway.Capabilities[capability.AvailabilityOf]
	if !siblingFound {
		return []string{fmt.Sprintf("%s: device %q declares no %q capability to track the availability of", provenance, decl.GatewayDeviceID, capability.AvailabilityOf)}, false
	}
	if siblingCapability.Domain != capability.AvailabilityOfDomain {
		return []string{fmt.Sprintf("%s: device %q's %q capability is domain %q, not %q as referenced (\"%s.%s is available;\")", provenance, decl.GatewayDeviceID, capability.AvailabilityOf, siblingCapability.Domain, capability.AvailabilityOfDomain, capability.AvailabilityOfDomain, capability.AvailabilityOf)}, false
	}

	// Reverse lookup: which entity_id did the sibling capability's own "entity ... from <leaf>;"
	// line (if it has run yet) resolve to? DiscoveryEntityLinks is keyed by entity_id, so this is a
	// scan -- fine at DSL-generation scale (a handful of capabilities per device, at most a few
	// hundred discovery entities house-wide).
	var siblingEntityID string
	for entityID, link := range administration.DiscoveryEntityLinks {
		if link.GatewayDeviceID == decl.GatewayDeviceID && link.Leaf == siblingCapability.Leaf {
			siblingEntityID = entityID
			break
		}
	}
	if siblingEntityID == "" {
		if !finalAttempt {
			return nil, true
		}
		return []string{fmt.Sprintf("%s: sibling capability %q has no \"entity ... from %s with %s;\" positioning yet -- add one (any space) before referencing its availability", provenance, capability.AvailabilityOf, decl.GatewayDeviceID, capability.AvailabilityOf)}, true
	}

	fullName := resolveDeviceEntityFullName(decl.EntitySpec, administration.SpacePath, deviceNamePath)
	identity := extractEntityIdentity(fullName)
	if identity.Domain == "" {
		return []string{fmt.Sprintf("%s: could not resolve a domain from %q; skipping", provenance, decl.EntitySpec)}, false
	}
	entityID := toHomeAssistantEntityID(fullName)
	if entityID == "" {
		return []string{fmt.Sprintf("%s: could not resolve a Home Assistant entity id from %q; skipping", provenance, decl.EntitySpec)}, false
	}

	sources, expr := parseConditionDirective("condition "+siblingEntityID+" is available", nil)
	if expr == "" {
		return []string{fmt.Sprintf("%s: could not build a condition from sibling entity %q", provenance, siblingEntityID)}, false
	}

	spaceName := administration.CurrentSpaceName()
	deviceClass, _, _, icon := resolveCapabilityDefaults(administration.CapabilityDefaults, identity.Domain, identity.Path)
	administration.AppendEntityRecord(spaceName, TEntityRecord{
		Name:              fullName,
		Identity:          identity,
		Provenance:        provenance,
		ConditionSources:  sources,
		ConditionExpr:     expr,
		ConditionDevClass: deviceClass,
		EntityIcon:        icon,
		// This is a generator-authored condition entity, never something assumed to already exist
		// on "main" -- without this, collectMainEntityIDs (main_entities.go) wrongly sweeps it into
		// kind-5's "assumed to exist" list, and the coordinator's existence check fails it the
		// moment it's positioned for the first time (real bug found live 2026-09-24, node 67's own
		// "node" capability -- the exact same class of bug already fixed 2026-09-21 for the
		// Logical-layer sibling, registerLogicalIsAvailableCapability, just never applied here).
		HasDefinitionOrImport: true,
	})
	return nil, false
}

// registerDiscoveryDerivedEntityLink resolves a "derived DDD.NNN from EEE.MMM via TTT;" capability
// (capability.DerivedFromCapability names a sibling capability of the SAME gateway device) into a
// condition-based entity computing TTT ("$" substituted for "$1") over whatever entity_id the
// sibling capability itself resolved to -- same mechanism registerDiscoveryAvailabilityEntityLink
// already uses for the fixed "is available" case, generalised to an arbitrary author-supplied
// template. "$1" (not a parenthesised substitution) matches the existing hand-written condition
// convention (e.g. Macros.def's battery_alert: "($1 | int(0)) < ${alert_level}") -- the DSL author
// is expected to parenthesize "$" themselves in the "via" template wherever precedence requires it,
// exactly as they already do for a hand-written condition.
//
// The sibling might not have been positioned yet -- deferred=true with no warning lets the caller
// retry once after the whole file has been read, same as registerDiscoveryAvailabilityEntityLink.
func registerDiscoveryDerivedEntityLink(administration *TAdministrationState, decl TDiscoveryEntityDeclaration, capability TDiscoveryCapability, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, deviceNamePath, entitiesPath string, lineNum int, finalAttempt bool) (warnings []string, deferred bool) {
	provenance := fmt.Sprintf("%s:%d → %s from %s with %s", filepath.Base(entitiesPath), lineNum, decl.EntitySpec, decl.GatewayDeviceID, decl.Leaf)

	gateway, found := discoveryGatewaysByID[decl.GatewayDeviceID]
	if !found {
		return []string{fmt.Sprintf("%s: gateway device %q not found in Physical.def's \"discovery\" integration", provenance, decl.GatewayDeviceID)}, false
	}
	siblingCapability, siblingFound := gateway.Capabilities[capability.DerivedFromCapability]
	if !siblingFound {
		return []string{fmt.Sprintf("%s: device %q declares no %q capability to derive from", provenance, decl.GatewayDeviceID, capability.DerivedFromCapability)}, false
	}

	// Reverse lookup: which entity_id did the sibling capability's own "entity ... from <leaf>;"
	// line (if it has run yet) resolve to? Same linear scan as registerDiscoveryAvailabilityEntityLink.
	var siblingEntityID string
	for entityID, link := range administration.DiscoveryEntityLinks {
		if link.GatewayDeviceID == decl.GatewayDeviceID && link.Leaf == siblingCapability.Leaf {
			siblingEntityID = entityID
			break
		}
	}
	if siblingEntityID == "" {
		if !finalAttempt {
			return nil, true
		}
		return []string{fmt.Sprintf("%s: sibling capability %q has no \"entity ... from %s with %s;\" positioning yet -- add one (any space) before deriving from it", provenance, capability.DerivedFromCapability, decl.GatewayDeviceID, capability.DerivedFromCapability)}, true
	}

	fullName := resolveDeviceEntityFullName(decl.EntitySpec, administration.SpacePath, deviceNamePath)
	identity := extractEntityIdentity(fullName)
	if identity.Domain == "" {
		return []string{fmt.Sprintf("%s: could not resolve a domain from %q; skipping", provenance, decl.EntitySpec)}, false
	}
	entityID := toHomeAssistantEntityID(fullName)
	if entityID == "" {
		return []string{fmt.Sprintf("%s: could not resolve a Home Assistant entity id from %q; skipping", provenance, decl.EntitySpec)}, false
	}

	expr := strings.ReplaceAll(capability.DerivedViaTemplate, "$", "$1")

	spaceName := administration.CurrentSpaceName()
	deviceClass, _, _, icon := resolveCapabilityDefaults(administration.CapabilityDefaults, identity.Domain, identity.Path)
	administration.AppendEntityRecord(spaceName, TEntityRecord{
		Name:              fullName,
		Identity:          identity,
		Provenance:        provenance,
		ConditionSources:  []string{siblingEntityID},
		ConditionExpr:     expr,
		ConditionDevClass: deviceClass,
		EntityIcon:        icon,
		// Same fix as registerDiscoveryAvailabilityEntityLink's own identical gap, just above --
		// this is ALSO a generator-authored condition entity, never assumed to exist on "main".
		HasDefinitionOrImport: true,
	})
	return nil, false
}
