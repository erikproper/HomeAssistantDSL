/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationHostsParser
 *
 * Parses the local "device <id> <host> <type> [with: <capability>: <entity>; ... end;]"
 * syntax inside Physical.def's "integration hosts with: ... end;" block (already
 * extracted by Physical_Parser.go's generic wrapper parser) into THostDevice records
 * (integration_hosts_storage.go).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 19.08.2026
 *
 */

package main

import (
	"fmt"
	"regexp"
	"strings"
)

// parseHostsIntegrationBody parses the body lines of an "integration hosts with: ...
// end;" block into device declarations. Lines that don't parse cleanly (e.g. a malformed
// "device ...;" line) are reported as warnings rather than aborting the parse.
func parseHostsIntegrationBody(bodyLines []string) ([]THostDevice, []string) {
	var devices []THostDevice
	var warnings []string

	bareDevicePattern := regexp.MustCompile(`^device\s+(\S+)\s+(\S+)\s+(\S+)\s*;\s*$`)
	withDevicePattern := regexp.MustCompile(`^device\s+(\S+)\s+(\S+)\s+(\S+)\s+with:\s*$`)
	capabilityPattern := regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*):\s*(\S+)\s*;\s*$`)

	inDeviceCapabilities := false
	var current THostDevice

	for lineIdx, rawLine := range bodyLines {
		line := strings.TrimSpace(rawLine)
		if commentIdx := strings.Index(line, "#"); commentIdx >= 0 {
			line = strings.TrimSpace(line[:commentIdx])
		}
		if line == "" {
			continue
		}

		if inDeviceCapabilities {
			if line == "end;" {
				devices = append(devices, current)
				inDeviceCapabilities = false
				continue
			}
			if matches := capabilityPattern.FindStringSubmatch(line); matches != nil {
				current.Capabilities[matches[1]] = matches[2]
				continue
			}
			warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line inside device %q capability block (body line %d): %q", current.DeviceID, lineIdx+1, line))
			continue
		}

		if matches := withDevicePattern.FindStringSubmatch(line); matches != nil {
			current = THostDevice{DeviceID: matches[1], HostName: matches[2], IntegrationType: matches[3], Capabilities: map[string]string{}}
			inDeviceCapabilities = true
			continue
		}

		if matches := bareDevicePattern.FindStringSubmatch(line); matches != nil {
			devices = append(devices, THostDevice{DeviceID: matches[1], HostName: matches[2], IntegrationType: matches[3]})
			continue
		}

		warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line in \"integration hosts\" block (body line %d): %q", lineIdx+1, line))
	}

	return devices, warnings
}
