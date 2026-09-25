/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Physical-layer hard-enforcement rules
 *
 * plans/derived-capability-mechanism.md Phase 2: validates every "derived DDD.NNN from
 * EEE.MMM via TTT;" capability (Physical_DerivedCapability.go) -- EEE.MMM must resolve to an
 * already-declared SIBLING capability of the SAME device (same-device-sibling-only, by design),
 * and the derivation chain must be finite.
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

// validateDerivedCapabilities checks every derived capability declared on any home_assistant/
// hassbridge, import, or discovery device. Warning-only, run once Physical.def collection for all
// device maps is complete (generator.go's parseAdministrationFromPaths) -- nothing downstream yet
// consumes DerivedFromCapability/DerivedViaTemplate (the runtime half -- generator/coordinator
// wiring -- is still to be built), so a bad declaration here has no functional consequence today,
// only a diagnostic one; this check exists so Physical.def authoring mistakes surface immediately
// rather than silently once that runtime half lands.
func validateDerivedCapabilities(hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice) []string {
	var warnings []string

	hassBridgeIDs := make([]string, 0, len(hassBridgeDevicesByID))
	for deviceID := range hassBridgeDevicesByID {
		hassBridgeIDs = append(hassBridgeIDs, deviceID)
	}
	sort.Strings(hassBridgeIDs)
	for _, deviceID := range hassBridgeIDs {
		derivedFrom := map[string]string{}
		for label, cap := range hassBridgeDevicesByID[deviceID].Capabilities {
			derivedFrom[label] = cap.DerivedFromCapability
		}
		warnings = append(warnings, validateDerivedCapabilityChains(deviceID, derivedFrom)...)
	}

	importedIDs := make([]string, 0, len(importedDevicesByID))
	for deviceID := range importedDevicesByID {
		importedIDs = append(importedIDs, deviceID)
	}
	sort.Strings(importedIDs)
	for _, deviceID := range importedIDs {
		derivedFrom := map[string]string{}
		for label, cap := range importedDevicesByID[deviceID].Capabilities {
			derivedFrom[label] = cap.DerivedFromCapability
		}
		warnings = append(warnings, validateDerivedCapabilityChains(deviceID, derivedFrom)...)
	}

	discoveryIDs := make([]string, 0, len(discoveryGatewaysByID))
	for deviceID := range discoveryGatewaysByID {
		discoveryIDs = append(discoveryIDs, deviceID)
	}
	sort.Strings(discoveryIDs)
	for _, deviceID := range discoveryIDs {
		derivedFrom := map[string]string{}
		for label, cap := range discoveryGatewaysByID[deviceID].Capabilities {
			derivedFrom[label] = cap.DerivedFromCapability
		}
		warnings = append(warnings, validateDerivedCapabilityChains(deviceID, derivedFrom)...)
	}

	return warnings
}

// validateDerivedCapabilityChains walks one device's own derivation graph, given derivedFrom
// (every declared capability's own label mapped to its DerivedFromCapability -- "" for an
// atomic, non-derived capability). For each derived capability, walks its chain back toward an
// atomic base, flagging a reference to a label the device doesn't declare at all (dangling) or a
// chain that loops back on itself (directly or transitively) before ever reaching one.
func validateDerivedCapabilityChains(deviceID string, derivedFrom map[string]string) []string {
	var warnings []string

	labels := make([]string, 0, len(derivedFrom))
	for label := range derivedFrom {
		labels = append(labels, label)
	}
	sort.Strings(labels)

	for _, label := range labels {
		from := derivedFrom[label]
		if from == "" {
			continue // atomic capability, nothing to validate
		}

		visited := map[string]bool{label: true}
		cur := from
		for {
			curFrom, known := derivedFrom[cur]
			if !known {
				warnings = append(warnings, fmt.Sprintf("device %q: capability %q is \"derived from\" %q, which is not a capability declared on the same device -- ignored", deviceID, label, cur))
				break
			}
			if visited[cur] {
				warnings = append(warnings, fmt.Sprintf("device %q: capability %q's \"derived from\" chain loops back on itself (via %q) -- ignored", deviceID, label, cur))
				break
			}
			visited[cur] = true
			if curFrom == "" {
				break // reached an atomic base capability -- chain is well-formed
			}
			cur = curFrom
		}
	}

	return warnings
}
