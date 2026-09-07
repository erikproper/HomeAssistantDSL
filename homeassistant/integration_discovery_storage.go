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

import "strings"

// TDiscoveryCapability is one named leaf a discovery gateway (sub-)device exposes.
type TDiscoveryCapability struct {
	Domain string
	Leaf   string
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
			}
		}
	}
	return byID, warnings
}
