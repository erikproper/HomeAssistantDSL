/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: LogicalRuleBatteryAlertBiconditional
 *
 * PROJECT.md item 7 / plans/derived-capability-mechanism.md's Rule 2, re-enforced at the LOGICAL
 * layer (2026-10-04, the user's own explicit follow-up once physical_rule_battery_alert_biconditional.go
 * was extended to cover every PHYSICAL integration kind including "local": "the battery_alert IFF
 * battery_level rule should also be re-enforced at the logical level"). A Logical.def device can
 * absorb or derive its own "battery_level"/"battery_alert" capabilities (e.g. combining several
 * physical devices' own readings into one logical one) -- exactly the same biconditional applies
 * there, independently of whatever the physical layer does: this check only ever looks at a
 * Logical.def device's own Capabilities map, the logical-layer counterpart to the physical
 * check's own "only ever looks at each device's own PHYSICAL capabilities map" scoping.
 *
 * Deliberately a SEPARATE function/file/warning-prefix from the physical check, not a shared loop
 * added to it -- the physical check's own warnings are printed under "[physical]" by generator.go,
 * and conflating a Logical.def-sourced violation under that prefix would misdirect the DSL author
 * to the wrong file. Reuses batteryBiconditionalWarning (physical_rule_battery_alert_biconditional.go)
 * verbatim -- same package, same two-states-in/one-message-out shape, no reason to duplicate it.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 04.10.2026
 *
 */

package main

import "sort"

// validateLogicalBatteryLevelAlertBiconditional warns whenever a Logical.def device declares
// exactly one of "battery_level"/"battery_alert" among its own Capabilities -- either side may be
// absorbed or derived, only presence of the capability KEY matters here, not how it was declared.
func validateLogicalBatteryLevelAlertBiconditional(logicalDevicesByID map[string]TLogicalDevice) []string {
	var warnings []string

	logicalIDs := make([]string, 0, len(logicalDevicesByID))
	for id := range logicalDevicesByID {
		logicalIDs = append(logicalIDs, id)
	}
	sort.Strings(logicalIDs)
	for _, id := range logicalIDs {
		device := logicalDevicesByID[id]
		_, hasLevel := device.Capabilities["battery_level"]
		_, hasAlert := device.Capabilities["battery_alert"]
		if w := batteryBiconditionalWarning(id, hasLevel, hasAlert); w != "" {
			warnings = append(warnings, w)
		}
	}

	return warnings
}
