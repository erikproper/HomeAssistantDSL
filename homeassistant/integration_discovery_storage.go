/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationDiscoveryStorage
 *
 * The data model for the "discovery" integration's parsed gateway devices: externally
 * discovered MQTT devices (e.g. EMS-ESP) that publish their own native Home Assistant MQTT
 * Discovery, bundled under DSL-level device ids so Spaces.def entities can reference a specific
 * auto-discovered leaf via "entity <spec> from <gateway-id>.<local-name>;"
 * (Conceptual_DiscoveryEntities.go) without this generator having to know or hand-declare the
 * gateway's own entity set.
 *
 * A gateway is commonly not one HA device but several, chained via via_device to a root (e.g.
 * EMS-ESP's "ems-esp" root plus "ems-esp-boiler"/"ems-esp-thermostat"/"ems-esp-mixer", each
 * via_device: "ems-esp") -- TDiscoveryGatewayDevice.ParentDeviceID models that real hierarchy
 * directly via nested "device <id> with: ... end;" declarations, rather than flattening it into
 * one bulk "identifiers" list the way this used to work (2026-08-26 redesign).
 *
 * Each device may declare its own "<entity_type>.<local-name>: <source-leaf>;" capabilities --
 * Domain/Leaf, mirroring THassBridgeCapability's own Domain/Source shape. Leaf is the raw,
 * gateway-native unique_id (e.g. "boiler_outdoortemp") -- unstable, owned by the gateway, not by
 * us. local-name is the DSL-chosen, stable name Spaces.def actually references; resolving
 * local-name -> raw leaf happens once, generator-side (registerDiscoveryEntityLink), so a leaf
 * being renamed on the wire never ripples into every Spaces.def declaration that uses it -- the
 * same naming-stability principle the "hosts"/"home_assistant" device grammar already has.
 *
 * Parsing (integration_discovery_parser.go) produces TDiscoveryGatewayDevice values; this file
 * doesn't parse anything itself.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 26.08.2026
 *
 */

package main

import (
	"fmt"
	"strings"
)

// TDiscoveryCapability is one named leaf a discovery gateway (sub-)device exposes.
type TDiscoveryCapability struct {
	Domain string
	Leaf   string // raw gateway leaf id -- empty when AvailabilityOf is set instead

	// SourceDomain (2026-09-15), when non-empty, is an optional "<raw-domain>:" qualifier on Leaf
	// (e.g. "event" in "event.core: event:0x..._action_zigbee2mqtt;") -- disambiguates a leaf id
	// the gateway publishes under more than one raw MQTT discovery domain for the same underlying
	// property (real case: a Moes scene remote's "action", published as both a "sensor" text
	// mirror and a richer "event" entity, identical unique_id). Propagated to
	// TDiscoveryEntityLink/discovery.yaml so the coordinator's own relay matching
	// (subscribeDiscoveryBridge, discoverybridge.go) can require the incoming raw message's own
	// topic domain to match, not just (gateway, leaf). Empty (the default, unaffected by this)
	// matches whichever raw domain publishes the leaf, exactly as before -- required for the
	// existing, deliberate cross-domain pattern (e.g. "fan.core: 0x..._switch_zigbee2mqtt;",
	// relaying a switch-domain-published leaf under a different declared conceptual domain).
	SourceDomain string

	// AvailabilityOf/AvailabilityOfDomain, when AvailabilityOf is non-empty, name a SIBLING
	// capability (this same device's own local label, e.g. "core", plus the domain it was
	// declared under, e.g. "light") this one tracks the availability of -- from a capability
	// line's own "<domain>.<label> is available;" sugar (e.g. "binary_sensor.node:
	// light.core is available;"), rather than a raw gateway leaf. The domain is required
	// (2026-09-15), not inferred, matching "derived DDD.NNN from EEE.MMM via TTT;"'s own
	// always-domain-qualified "from" side -- self-documenting, and guards against a future
	// same-named-different-domain capability being referenced ambiguously; resolution
	// (registerDiscoveryAvailabilityEntityLink) checks the sibling's own Domain matches.
	// Mutually exclusive with Leaf and DerivedFromCapability.
	AvailabilityOf       string
	AvailabilityOfDomain string

	// DerivedFromCapability/DerivedViaTemplate (2026-09-15): "derived DDD.NNN from EEE.MMM via
	// TTT;" declares this capability's value as a function of a SIBLING capability of the same
	// gateway device, exactly the same grammar/semantics as home_assistant/hassbridge's own
	// "derived" (Physical_DerivedCapability.go) -- DerivedFromCapability holds EEE.MMM's own map
	// key (MMM), DerivedViaTemplate is TTT ("$" stands for the sibling's own resolved value).
	// Mutually exclusive with Leaf and AvailabilityOf. Resolved into a real entity the same way
	// AvailabilityOf is -- a local condition/template entity (registerDiscoveryDerivedEntityLink,
	// Conceptual_DiscoveryEntities.go), never a raw MQTT relay, since the value has to be computed
	// locally from a sibling rather than read off the wire.
	DerivedFromCapability string
	DerivedViaTemplate    string

	// Hidden (2026-09-15), from an optional leading "hidden " qualifier on the capability line,
	// marks an internal-only intermediate value -- fully usable as another capability's own
	// "derived ... from ...;" source (a raw Capabilities map lookup, unaffected by this flag), but
	// refused if a Spaces.def "entity ... from <label>;" line tries to position it directly (the
	// conceptual/logical layers should never see it). The motivating case: a sensor's raw,
	// unadjusted reading that only exists to feed a "derived" capability applying an offset/scale.
	Hidden bool
}

// TDiscoveryGatewayDevice is one "device <id> with: ... end;" declaration inside "integration
// discovery with: ... end;", optionally nested inside a parent device (ParentDeviceID != "") to
// model a real gateway's own device hierarchy.
//
// Identifiers matches an incoming discovery payload's own device identifiers OR its via_device
// (one hop) -- always includes an inferred default (DeviceID's own leaf segment, "_" -> "-", e.g.
// "discovery.ems_esp_boiler" -> "ems-esp-boiler") alongside any explicit "identifiers "<value>";"
// lines, so a device whose real HA identifier matches that convention needs no explicit
// declaration at all.
type TDiscoveryGatewayDevice struct {
	DeviceID       string
	ParentDeviceID string
	Identifiers    []string
	Capabilities   map[string]TDiscoveryCapability

	// IgnoreOtherCapabilities (2026-09-21), from an "ignore other capabilities;" body line, tells
	// the coordinator's own matchingGateway (house_event_bus_coordinator/discoverybridge.go) to
	// match THIS device by its own declared Identifiers only, never via another device's
	// via_device pointing at one of them (the "one-hop" rule this whole struct's own doc comment
	// describes, built for e.g. EMS-ESP's "ems-esp-thermostat" via_device: "ems-esp"). That rule
	// silently breaks for a device that's genuinely the ROOT of an entire network rather than one
	// narrow multi-facet gateway: real bug found live 2026-09-21 (Vienna) -- declaring
	// "node.zigbee2mqtt_bridge" with the bridge's own identifier made EVERY other Zigbee2MQTT
	// device match it too (Zigbee2MQTT sets every single device's own via_device to the bridge),
	// so every other device's undeclared leaves started appearing lumped under this one gateway in
	// suggestions/discovery.txt. Scoped to suggestion/existence-tracking's own gateway attribution
	// only -- this device's OWN declared capabilities still resolve normally either way, since
	// those already match via direct Identifiers, never needing the one-hop rule at all.
	IgnoreOtherCapabilities bool
}

// inferredDiscoveryIdentifier derives a device's default HA device identifier from its own DSL
// id: everything after the first "." (dropping the "discovery." qualifier), "_" replaced with
// "-" (e.g. "discovery.ems_esp_boiler" -> "ems-esp-boiler").
func inferredDiscoveryIdentifier(deviceID string) string {
	name := deviceID
	if idx := strings.Index(name, "."); idx >= 0 {
		name = name[idx+1:]
	}
	return strings.ReplaceAll(name, "_", "-")
}

// collectDiscoveryGatewaysByID re-parses Physical.def's "discovery" integration block and
// returns every declared gateway device (at any nesting depth, flattened) keyed by DeviceID, for
// callers that need lookup outside the Physical.def-native generation pipeline (Spaces.def
// parsing via Conceptual_DiscoveryEntities.go). First declaration wins for a duplicated id,
// matching collectHostsDevicesByID's policy.
func collectDiscoveryGatewaysByID(definitionDir string) (map[string]TDiscoveryGatewayDevice, []string) {
	physicalContent, mergedLineNos, warnings := collectLayerContent(definitionDir, []string{"Physical.def"}, LayerPhysical)
	physicalContent = resolveDerivedConditionOneLiners(physicalContent)
	var jinjaWarnings []string
	physicalContent, jinjaWarnings = resolveJinjaTemplateCallsInDerivedLines(physicalContent, loadJinjaTemplateDefinitions(definitionDir))
	warnings = append(warnings, jinjaWarnings...)
	blocks, blockWarnings := parseIntegrationBlocks(physicalContent, mergedLineNos)
	warnings = append(warnings, blockWarnings...)

	byID := map[string]TDiscoveryGatewayDevice{}
	for _, block := range blocks {
		if block.Name != "discovery" {
			continue
		}
		devices, bodyWarnings := parseDiscoveryIntegrationBody(block.BodyLines)
		warnings = append(warnings, bodyWarnings...)
		for _, d := range devices {
			if _, exists := byID[d.DeviceID]; !exists {
				byID[d.DeviceID] = d
			} else {
				// See integration_hosts_storage.go's own "declared more than once" warning for
				// the real incident (2026-09-10) that prompted adding this across every
				// integration kind's own collector, not just hassbridge's (which already had it).
				warnings = append(warnings, fmt.Sprintf("Physical.def: device %q declared more than once within the \"discovery\" integration; keeping the first declaration", d.DeviceID))
			}
		}
	}
	return byID, warnings
}
