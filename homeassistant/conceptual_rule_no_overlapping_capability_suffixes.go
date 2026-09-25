/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Conceptual-layer hard-enforcement rules
 *
 * Real bug found live 2026-09-25 (Vienna): the no-rename shorthand's own longest-"/"-suffix
 * capability matching (registerDeviceCapabilityEntityLinkAllowingHidden's own
 * resolveLongestSuffixCapability, Conceptual_DeviceCapabilityEntities.go) means a device
 * declaring BOTH a capability "mm" and a capability "ll/mm" is inherently ambiguous bookkeeping,
 * even though the matcher itself always resolves deterministically (longest match wins) -- the
 * user's own words: "It does make sense to warn against a logical/physical device having both a
 * capability mm and ll/mm. That should be forbidden."
 *
 * validateNoOverlappingCapabilitySuffixes closes that gap: run once, after Physical.def/Logical.def
 * parsing (every kind's own Capabilities map is fully populated by then), checking each device's
 * OWN capability set for any pair where one name is a whole "/"-delimited trailing-segment SUFFIX
 * of the other -- "node" and "aqara/node" collide this way, "l/mm" and "kk/ll/mm" do NOT (a partial
 * segment match, "l" vs "ll", is never a real suffix). Scoped to discovery/hassbridge/
 * imported/commandline/local/logical kinds, the only ones with an author-declared Capabilities map
 * at all -- "hosts" kind capabilities come from a fixed, hardcoded materialization table
 * (MaterializationForIntegrationType's own AttributeNames), never author input, so this ambiguity
 * can never arise there.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 25.09.2026
 *
 */

package main

import (
	"fmt"
	"sort"
	"strings"
)

// isCapabilitySuffixOf reports whether short's own "/"-delimited segments are a whole trailing
// suffix of long's own segments -- short=="node", long=="aqara/node" -> true; short=="l",
// long=="kk/ll/mm" -> false (not a whole segment of long at all); short==long -> false (identical
// names are a duplicate map key, impossible within one device's own Capabilities map, not this
// rule's concern).
func isCapabilitySuffixOf(short, long string) bool {
	if short == long {
		return false
	}
	shortSegments := strings.Split(short, "/")
	longSegments := strings.Split(long, "/")
	if len(shortSegments) >= len(longSegments) {
		return false
	}
	tail := longSegments[len(longSegments)-len(shortSegments):]
	return strings.Join(tail, "/") == short
}

// validateNoOverlappingCapabilitySuffixes checks every device's own capability-name set (across
// every kind with an author-declared Capabilities map) for a pair where one name is a "/"-suffix of
// the other, warning once per colliding pair. deviceID/capability names are both attacker-free,
// generator-internal strings (never used in a template literally), so simple %q formatting is safe.
func validateNoOverlappingCapabilitySuffixes(discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, commandlineDevicesByID map[string]TCommandlineDevice, localDevicesByID map[string]TLocalDevice, logicalDevicesByID map[string]TLogicalDevice) []string {
	capsByDevice := map[string][]string{}
	for id, device := range discoveryGatewaysByID {
		for name := range device.Capabilities {
			capsByDevice[id] = append(capsByDevice[id], name)
		}
	}
	for id, device := range hassBridgeDevicesByID {
		for name := range device.Capabilities {
			capsByDevice[id] = append(capsByDevice[id], name)
		}
	}
	for id, device := range importedDevicesByID {
		for name := range device.Capabilities {
			capsByDevice[id] = append(capsByDevice[id], name)
		}
	}
	for id, device := range commandlineDevicesByID {
		for name := range device.Capabilities {
			capsByDevice[id] = append(capsByDevice[id], name)
		}
	}
	for id, device := range localDevicesByID {
		for name := range device.Capabilities {
			capsByDevice[id] = append(capsByDevice[id], name)
		}
	}
	for id, device := range logicalDevicesByID {
		for name := range device.Capabilities {
			capsByDevice[id] = append(capsByDevice[id], name)
		}
	}

	deviceIDs := make([]string, 0, len(capsByDevice))
	for id := range capsByDevice {
		deviceIDs = append(deviceIDs, id)
	}
	sort.Strings(deviceIDs)

	var warnings []string
	for _, deviceID := range deviceIDs {
		names := capsByDevice[deviceID]
		sort.Strings(names)
		for i, a := range names {
			for _, b := range names[i+1:] {
				if isCapabilitySuffixOf(a, b) || isCapabilitySuffixOf(b, a) {
					warnings = append(warnings, fmt.Sprintf("device %q declares both capability %q and %q -- one is a \"/\"-suffix of the other, which is ambiguous for the no-rename shorthand's own longest-suffix matching; rename one of them", deviceID, a, b))
				}
			}
		}
	}
	return warnings
}
