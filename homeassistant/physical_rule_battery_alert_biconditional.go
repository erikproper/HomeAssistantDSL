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
 * Extended 2026-09-30 to cover every integration kind, not just home_assistant/hassbridge,
 * import, and discovery -- a real gap found live by the user: several "local"-kind devices
 * (appliance.front_door_ring, appliance.ring_camera, appliance.sonos_kitchen) declare
 * "sensor.battery_level" in Physical.def but their own "battery_alert" only as a Logical.def
 * "derived" capability on the same device id. This check deliberately does NOT cross-reference
 * Logical.def at all -- it only ever looked at each device's own PHYSICAL capabilities map, by
 * design (Rule 2's own biconditional is a physical-layer requirement) -- so these three devices
 * were silently missed simply because "local" wasn't one of the three kinds this function used
 * to scan, not because the devices were exempt from the rule. The right fix for each such
 * violation is to move its "battery_alert" declaration down into Physical.def (matching every
 * other device's own convention), not to make this check tolerate Logical.def as a substitute.
 * "hosts" and "commandline" are included too for full "any integration" coverage, even though
 * neither currently has a real battery_level/battery_alert pair -- same map[string]X capability
 * shape, same check, cheap to include.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 30.09.2026
 *
 */

package main

import (
	"fmt"
	"sort"
)

// validateBatteryLevelAlertBiconditional warns whenever ANY integration kind's device declares
// exactly one of "battery_level"/"battery_alert" at the PHYSICAL layer -- either side may be
// atomic or derived, only presence of the capability KEY matters here, not how it was declared,
// and a Logical.def "derived" capability of the same name on the same device id does NOT count
// as satisfying this -- see this file's own header comment for why.
func validateBatteryLevelAlertBiconditional(hostDevicesByID map[string]THostDevice, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, commandlineDevicesByID map[string]TCommandlineDevice, localDevicesByID map[string]TLocalDevice) []string {
	var warnings []string

	hostIDs := make([]string, 0, len(hostDevicesByID))
	for id := range hostDevicesByID {
		hostIDs = append(hostIDs, id)
	}
	sort.Strings(hostIDs)
	for _, id := range hostIDs {
		device := hostDevicesByID[id]
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

	commandlineIDs := make([]string, 0, len(commandlineDevicesByID))
	for id := range commandlineDevicesByID {
		commandlineIDs = append(commandlineIDs, id)
	}
	sort.Strings(commandlineIDs)
	for _, id := range commandlineIDs {
		device := commandlineDevicesByID[id]
		_, hasLevel := device.Capabilities["battery_level"]
		_, hasAlert := device.Capabilities["battery_alert"]
		if w := batteryBiconditionalWarning(id, hasLevel, hasAlert); w != "" {
			warnings = append(warnings, w)
		}
	}

	localIDs := make([]string, 0, len(localDevicesByID))
	for id := range localDevicesByID {
		localIDs = append(localIDs, id)
	}
	sort.Strings(localIDs)
	for _, id := range localIDs {
		device := localDevicesByID[id]
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
