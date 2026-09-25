/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Physical-layer hard-enforcement rules
 *
 * PROJECT.md item 7 / plans/derived-capability-mechanism.md's Rule 3: a device declaring "power"
 * must also declare "consumes" (atomic or derived) -- one-directional, unlike Rule 2's
 * battery_level/battery_alert biconditional: "consumes" is inherently a threshold read off "power"
 * (see Settings.def's "${int_more_then}(i)" jinja macro, the migration-default threshold), but the
 * reverse isn't asserted here -- a device could plausibly have a "consumes"-shaped capability
 * computed some other way without this generator needing to know about it.
 *
 * Introduced 2026-09-17 alongside the dishwasher's own migration off a hand-written, standalone
 * Conceptual.def "condition" entity (Macros.def's old "power_switch" macro's own "consumes"
 * binary_sensor) onto a real Physical.def "derived binary_sensor.consumes from sensor.power via
 * jinja ${int_more_then}(1);" capability -- this rule exists so the NEXT power-capable device
 * migrated doesn't silently forget to bring "consumes" along too.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 17.09.2026
 *
 */

package main

import (
	"fmt"
	"sort"
)

// validatePowerImpliesConsumes warns whenever a home_assistant/hassbridge, import, or discovery
// device declares "power" without also declaring "consumes" -- either side may be atomic or
// derived, only presence of the capability KEY matters here, not how it was declared (mirrors
// validateBatteryLevelAlertBiconditional's own convention exactly, minus its reverse check).
func validatePowerImpliesConsumes(hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice) []string {
	var warnings []string

	hassBridgeIDs := make([]string, 0, len(hassBridgeDevicesByID))
	for id := range hassBridgeDevicesByID {
		hassBridgeIDs = append(hassBridgeIDs, id)
	}
	sort.Strings(hassBridgeIDs)
	for _, id := range hassBridgeIDs {
		device := hassBridgeDevicesByID[id]
		_, hasPower := device.Capabilities["power"]
		_, hasConsumes := device.Capabilities["consumes"]
		if w := powerImpliesConsumesWarning(id, hasPower, hasConsumes); w != "" {
			warnings = append(warnings, w)
		}
	}

	importedIDs := make([]string, 0, len(importedDevicesByID))
	for id := range importedDevicesByID {
		importedIDs = append(importedIDs, id)
	}
	sort.Strings(importedIDs)
	for _, id := range importedIDs {
		device := importedDevicesByID[id]
		_, hasPower := device.Capabilities["power"]
		_, hasConsumes := device.Capabilities["consumes"]
		if w := powerImpliesConsumesWarning(id, hasPower, hasConsumes); w != "" {
			warnings = append(warnings, w)
		}
	}

	discoveryIDs := make([]string, 0, len(discoveryGatewaysByID))
	for id := range discoveryGatewaysByID {
		discoveryIDs = append(discoveryIDs, id)
	}
	sort.Strings(discoveryIDs)
	for _, id := range discoveryIDs {
		device := discoveryGatewaysByID[id]
		_, hasPower := device.Capabilities["power"]
		_, hasConsumes := device.Capabilities["consumes"]
		if w := powerImpliesConsumesWarning(id, hasPower, hasConsumes); w != "" {
			warnings = append(warnings, w)
		}
	}

	return warnings
}

func powerImpliesConsumesWarning(deviceID string, hasPower, hasConsumes bool) string {
	if hasPower && !hasConsumes {
		return fmt.Sprintf("device %q declares \"power\" but no \"consumes\" (atomic or derived) -- Rule 3 requires a power-capable device to also expose a consumption threshold", deviceID)
	}
	return ""
}
