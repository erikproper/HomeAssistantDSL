/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Physical-layer hard-enforcement rules
 *
 * PROJECT.md item 7 / plans/derived-capability-mechanism.md's "Rule 1": every home_assistant/
 * hassbridge and import-kind device must declare a "node" capability.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 10.09.2026
 *
 */

package main

import (
	"fmt"
	"sort"
)

// validateNodeCapabilityRequired implements Rule 1: every home_assistant/hassbridge and
// import-kind device must declare a "node" capability. hosts/commandline/discovery-kind devices
// are structurally exempt -- a hosts device's liveness entity is unconditional
// (registerHostDevicePositioning), a commandline device's is LWT-based
// (registerCommandlineDevicePositioning), and a discovery-kind gateway device relays its upstream
// bridge's own discovery payload verbatim, availability field included (discoverybridge.go) -- none
// of the three have a DSL-level "node" capability concept to check in the first place.
//
// Runs once Physical.def collection is complete (parseAdministrationFromPaths, right after the
// hassBridgeDevicesByID/importedDevicesByID collection calls), independent of whether a device
// ever gets positioned in Spaces.def at all -- registerDevicePositioning's own "node" warning
// (Conceptual_DevicePositioning.go:170) only fires for devices that a Spaces.def "device ...
// from ...;" line actually positions, so it can't see a declared-but-never-positioned device
// missing "node". This is the only check that covers that case.
//
// Warning-only, matching this codebase's own precedent for this exact check
// (Conceptual_DevicePositioning.go:170) -- confirmed by the user 2026-09-10 that Rule 1 has zero
// real violations in either house today.
func validateNodeCapabilityRequired(hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice) []string {
	var warnings []string

	hassBridgeIDs := make([]string, 0, len(hassBridgeDevicesByID))
	for deviceID := range hassBridgeDevicesByID {
		hassBridgeIDs = append(hassBridgeIDs, deviceID)
	}
	sort.Strings(hassBridgeIDs)
	for _, deviceID := range hassBridgeIDs {
		if _, hasNode := hassBridgeDevicesByID[deviceID].Capabilities["node"]; !hasNode {
			warnings = append(warnings, fmt.Sprintf("device %q declares no \"node\" capability (PROJECT.md item 7's Rule 1)", deviceID))
		}
	}

	importedIDs := make([]string, 0, len(importedDevicesByID))
	for deviceID := range importedDevicesByID {
		importedIDs = append(importedIDs, deviceID)
	}
	sort.Strings(importedIDs)
	for _, deviceID := range importedIDs {
		if _, hasNode := importedDevicesByID[deviceID].Capabilities["node"]; !hasNode {
			warnings = append(warnings, fmt.Sprintf("device %q declares no \"node\" capability (PROJECT.md item 7's Rule 1)", deviceID))
		}
	}

	return warnings
}
