/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Physical-layer hard-enforcement rules
 *
 * PROJECT.md item 0: warns when the same DeviceID is declared under more than one integration
 * kind's own Physical.def block -- a real incident found live 2026-09-10 (Junglinster's own
 * "host.frame" declared twice) that went undetected until its second declaration's own
 * capabilities silently went missing. Within-kind duplicates are each collector's own concern
 * (integration_hosts_storage.go et al., each now warns on its own first-wins drop); this file
 * covers the cross-kind case, which no single collector can see on its own.
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

// validateNoConflictingDeviceNames warns when the same DeviceID appears in more than one of the
// six device-collection maps. Two legitimate exceptions, both exactly a 2-kind pairing (a THIRD
// kind sharing the id alongside either is still flagged -- neither design was ever meant to cover
// that):
//   - a "commandline" device deliberately sharing its identity with an already-positioned "hosts"
//     device of the SAME id (registerCommandlineDevicePositioning's own doc comment explains why,
//     e.g. host.frame's own picture-frame daemon sharing identity with its own host entry).
//   - a "logical" device deliberately sharing its identity with an existing PHYSICAL-layer device
//     of any other single kind -- integration_logical_storage.go's own "dependency on" overlay
//     mechanism (e.g. node.vienna_livingroom, both an "import" device and a "logical" one): the
//     logical declaration EXTENDS the physical device, it isn't a second competing one.
func validateNoConflictingDeviceNames(hostDevicesByID map[string]THostDevice, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, commandlineDevicesByID map[string]TCommandlineDevice, localDevicesByID map[string]TLocalDevice, logicalDevicesByID map[string]TLogicalDevice) []string {
	kindsByDeviceID := map[string][]string{}
	for id := range hostDevicesByID {
		kindsByDeviceID[id] = append(kindsByDeviceID[id], "hosts")
	}
	for id := range discoveryGatewaysByID {
		kindsByDeviceID[id] = append(kindsByDeviceID[id], "discovery")
	}
	for id := range hassBridgeDevicesByID {
		kindsByDeviceID[id] = append(kindsByDeviceID[id], "home_assistant")
	}
	for id := range importedDevicesByID {
		kindsByDeviceID[id] = append(kindsByDeviceID[id], "import")
	}
	for id := range commandlineDevicesByID {
		kindsByDeviceID[id] = append(kindsByDeviceID[id], "commandline")
	}
	for id := range localDevicesByID {
		kindsByDeviceID[id] = append(kindsByDeviceID[id], "local")
	}
	for id := range logicalDevicesByID {
		kindsByDeviceID[id] = append(kindsByDeviceID[id], "logical")
	}

	deviceIDs := make([]string, 0, len(kindsByDeviceID))
	for id := range kindsByDeviceID {
		deviceIDs = append(deviceIDs, id)
	}
	sort.Strings(deviceIDs)

	var warnings []string
	for _, id := range deviceIDs {
		kinds := kindsByDeviceID[id]
		if len(kinds) < 2 {
			continue
		}
		if len(kinds) == 2 && containsString(kinds, "hosts") && containsString(kinds, "commandline") {
			continue // the one deliberate hosts+commandline exception -- see this function's own doc comment
		}
		if len(kinds) == 2 && containsString(kinds, "logical") {
			continue // the deliberate logical-overlay exception -- see this function's own doc comment
		}
		sort.Strings(kinds)
		warnings = append(warnings, fmt.Sprintf("device %q is declared under more than one integration kind (%v) -- if this isn't the deliberate hosts+commandline or logical-overlay identity-sharing case, one of these is very likely a mistake", id, kinds))
	}
	return warnings
}
