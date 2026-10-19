/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: LogicalRuleNodeCapability
 *
 * PROJECT.md item 7 / physical_rule_node_capability.go's "Rule 1," re-enforced at the LOGICAL
 * layer (2026-10-05, found live: Junglinster's own "fan.house_wc" declared no "node" capability
 * at all -- absorbing "fan.fan" from switches.garage_fuse_cabinet_zwave_058 but not that same
 * device's own "node" -- and generation succeeded silently, no warning anywhere). Rule 1 itself
 * only ever scans hassBridgeDevicesByID/importedDevicesByID (PHYSICAL-layer maps), so a
 * Logical.def-only device -- one with no existing physical-layer counterpart under the same id at
 * all, i.e. a genuinely NEW device identity this layer invents (TLogicalDevice's own header
 * comment: "a BRAND NEW device id... gets positioned at the conceptual layer just like a physical
 * device would") -- was never checked by anything.
 *
 * Deliberately NOT a blanket "every logical device needs its own node capability" rule: most
 * Logical.def devices (see this file's own 2026-10-05 audit, both houses, zero other violations
 * found) share their id with an EXISTING physical-layer device (hosts/discovery/hassbridge/
 * import/commandline/local) and are an EXTENSION of it (integration_logical_storage.go's own
 * header comment) -- that physical device already owns its own liveness story (Rule 1's explicit
 * check, a hosts/commandline device's structural exemption, or a discovery-kind device's own
 * "derived binary_sensor.node" in Physical.def), so the logical extension adding, say, one more
 * derived capability on top needs no node of its own. This check only fires for a logical device
 * id that exists in NONE of those physical-layer maps -- the brand-new-identity case Rule 1 can't
 * see. A logical device with an empty Capabilities map (the plain "device X with dependency on
 * Y;" sugar, nothing else) is skipped too -- it owns no entity of its own for "node" to gate.
 *
 * Warning-only, same precedent as Rule 1 and every other hard-enforcement rule in this codebase
 * (physical_rule_node_capability.go, logical_rule_battery_alert_biconditional.go) -- printed under
 * "[logical]" by generator.go, not "[physical]", so the DSL author is pointed at Logical.def.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 05.10.2026
 *
 */

package main

import (
	"fmt"
	"sort"
)

// validateLogicalNodeCapabilityRequired warns whenever a Logical.def device (a) declares at
// least one capability of its own, (b) has no "node" capability among them (declared, absorbed,
// or derived -- only presence of the Capabilities["node"] key matters, matching
// validateLogicalBatteryLevelAlertBiconditional's own "only presence of the capability KEY
// matters here" precedent), and (c) names an id that no physical-layer collection (hosts/
// discovery/hassbridge/import/commandline/local) already declares -- i.e. this is a genuinely new
// device identity, not an extension of one that already has its own liveness story.
func validateLogicalNodeCapabilityRequired(
	logicalDevicesByID map[string]TLogicalDevice,
	hostDevicesByID map[string]THostDevice,
	discoveryGatewaysByID map[string]TDiscoveryGatewayDevice,
	hassBridgeDevicesByID map[string]THassBridgeDevice,
	importedDevicesByID map[string]TImportedDevice,
	commandlineDevicesByID map[string]TCommandlineDevice,
	localDevicesByID map[string]TLocalDevice,
) []string {
	var warnings []string

	logicalIDs := make([]string, 0, len(logicalDevicesByID))
	for id := range logicalDevicesByID {
		logicalIDs = append(logicalIDs, id)
	}
	sort.Strings(logicalIDs)

	for _, id := range logicalIDs {
		device := logicalDevicesByID[id]
		if len(device.Capabilities) == 0 {
			continue
		}
		if _, hasNode := device.Capabilities["node"]; hasNode {
			continue
		}
		if _, ok := hostDevicesByID[id]; ok {
			continue
		}
		if _, ok := discoveryGatewaysByID[id]; ok {
			continue
		}
		if _, ok := hassBridgeDevicesByID[id]; ok {
			continue
		}
		if _, ok := importedDevicesByID[id]; ok {
			continue
		}
		if _, ok := commandlineDevicesByID[id]; ok {
			continue
		}
		if _, ok := localDevicesByID[id]; ok {
			continue
		}
		warnings = append(warnings, fmt.Sprintf("device %q declares no \"node\" capability and has no existing physical-layer counterpart to inherit one from (PROJECT.md item 7's Rule 1, re-enforced at the logical layer) -- either declare/absorb/derive its own \"node\", or absorb the capability from whichever device it's built from, e.g. \"absorb from <device-id>: capability binary_sensor.node; ...\"", id))
	}

	return warnings
}
