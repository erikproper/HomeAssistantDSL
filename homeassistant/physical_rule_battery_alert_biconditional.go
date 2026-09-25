/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Physical-layer hard-enforcement rules
 *
 * PROJECT.md item 7 / plans/derived-capability-mechanism.md's Rule 2: a device has
 * "battery_level" if and only if it has "battery_alert" -- a real biconditional, not just
 * implication. Either side may be atomic (the integration's own native capability) or "derived".
 * Enabled 2026-09-11 once both houses' real macro-driven battery_alert usages were migrated to
 * "derived" (see the migration helper, derived_migration_helper.go) -- zero real violations
 * confirmed live in both houses before this check was written.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 11.09.2026
 *
 */

package main

import (
	"fmt"
	"sort"
)

// validateBatteryLevelAlertBiconditional warns whenever a home_assistant/hassbridge, import, or
// discovery device declares exactly one of "battery_level"/"battery_alert" -- either side may be
// atomic or derived, only presence of the capability KEY matters here, not how it was declared.
func validateBatteryLevelAlertBiconditional(hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice) []string {
	var warnings []string

	hassBridgeIDs := make([]string, 0, len(hassBridgeDevicesByID))
	for id := range hassBridgeDevicesByID {
		hassBridgeIDs = append(hassBridgeIDs, id)
	}
	sort.Strings(hassBridgeIDs)
	for _, id := range hassBridgeIDs {
		device := hassBridgeDevicesByID[id]
		_, hasLevel := device.Capabilities["battery_level"]
		_, hasAlert := device.Capabilities["battery_alert"]
		if w := batteryBiconditionalWarning(id, hasLevel, hasAlert); w != "" {
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
		_, hasLevel := device.Capabilities["battery_level"]
		_, hasAlert := device.Capabilities["battery_alert"]
		if w := batteryBiconditionalWarning(id, hasLevel, hasAlert); w != "" {
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
		_, hasLevel := device.Capabilities["battery_level"]
		_, hasAlert := device.Capabilities["battery_alert"]
		if w := batteryBiconditionalWarning(id, hasLevel, hasAlert); w != "" {
			warnings = append(warnings, w)
		}
	}

	return warnings
}

func batteryBiconditionalWarning(deviceID string, hasLevel, hasAlert bool) string {
	switch {
	case hasLevel && !hasAlert:
		return fmt.Sprintf("device %q declares \"battery_level\" but no \"battery_alert\" (atomic or derived) -- Rule 2 requires both or neither", deviceID)
	case hasAlert && !hasLevel:
		return fmt.Sprintf("device %q declares \"battery_alert\" but no \"battery_level\" (atomic or derived) -- Rule 2 requires both or neither", deviceID)
	default:
		return ""
	}
}
