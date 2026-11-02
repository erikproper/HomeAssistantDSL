/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: RuleNodeCapability
 *
 * PROJECT.md item 7's "Rule 1", made a hard error for every device (PROJECT.md 26, Erik
 * 2026-11-02): every device has a "node" capability, declared in Physical.def or added in
 * Logical.def (declared, derived or absorbed). Before, it was a warning for home_assistant and
 * import devices and for a device only Logical.def declares; discovery and local devices weren't
 * checked at all -- by then they all had a node anyway, most from Logical.def (a Hue Tap's
 * "derived capability binary_sensor.node with condition node from integration.zigbee2mqtt").
 *
 * hosts and commandline devices are exempt: their node comes with their kind (a ping, a Last
 * Will). So is a request, a Logical.def device that only raises events (PROJECT.md item 49): it has
 * no state to be available or not. A Logical.def device without capabilities of its own (the plain
 * "device X with dependency on Y;") extends another declaration and is checked there.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 02.11.2026
 *
 */

package main

import (
	"fmt"
	"sort"
)

// validateEveryDeviceHasNode returns one problem per device without a "node" capability, sorted.
func validateEveryDeviceHasNode(
	logicalDevicesByID map[string]TLogicalDevice,
	hostDevicesByID map[string]THostDevice,
	discoveryGatewaysByID map[string]TDiscoveryGatewayDevice,
	hassBridgeDevicesByID map[string]THassBridgeDevice,
	importedDevicesByID map[string]TImportedDevice,
	commandlineDevicesByID map[string]TCommandlineDevice,
	localDevicesByID map[string]TLocalDevice,
) []string {
	hasNode := map[string]bool{} // device id -> its node, false while none is found
	for id, device := range hassBridgeDevicesByID {
		_, has := device.Capabilities["node"]
		hasNode[id] = hasNode[id] || has
	}
	for id, device := range importedDevicesByID {
		_, has := device.Capabilities["node"]
		hasNode[id] = hasNode[id] || has
	}
	for id, gateway := range discoveryGatewaysByID {
		_, has := gateway.Capabilities["node"]
		hasNode[id] = hasNode[id] || has
	}
	for id, device := range localDevicesByID {
		_, has := device.Capabilities["node"]
		hasNode[id] = hasNode[id] || has
	}
	for id, device := range logicalDevicesByID {
		if _, has := device.Capabilities["node"]; has {
			hasNode[id] = true
			continue
		}
		if _, declared := hasNode[id]; declared {
			continue
		}
		_, isHost := hostDevicesByID[id]
		_, isCommandline := commandlineDevicesByID[id]
		if isHost || isCommandline || len(device.Capabilities) == 0 || onlyDerivesEvents(device) {
			continue
		}
		hasNode[id] = false
	}
	var problems []string
	for id, has := range hasNode {
		if !has {
			problems = append(problems, fmt.Sprintf("device %q has no \"node\" capability, in Physical.def or Logical.def -- declare, derive or absorb one (e.g. \"derived capability binary_sensor.node with condition ...;\" in Logical.def)", id))
		}
	}
	sort.Strings(problems)
	return problems
}

// onlyDerivesEvents reports whether every capability of device is a derived "event" (PROJECT.md
// item 49) -- a request such as a light toggle, raised by other devices. It has no state of its own,
// so nothing to be available or not: Rule 1's "node" doesn't apply.
func onlyDerivesEvents(device TLogicalDevice) bool {
	for _, capability := range device.Capabilities {
		if capability.Domain != "event" || !capability.IsDerivedCondition {
			return false
		}
	}
	return true
}
