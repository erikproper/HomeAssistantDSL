/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationHostsParser
 *
 * Parses the local "device <id> <host> <type> [with: <capability>: <entity>; <constant
 * device attribute>: "<value>" [forced]; ... end;]" syntax inside Physical.def's
 * "integration hosts with: ... end;" block (already extracted by Physical_Parser.go's
 * generic wrapper parser) into THostDevice records (integration_hosts_storage.go).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 26.08.2026
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
	// Group 1 is the capability name, optionally "<group>/<leaf>" (e.g. "cpu/load") -- see
	// splitCapabilityName (integration_hosts_storage.go). Group 2 (optional) is a literal
	// string prefix -- see THostDevice.CapabilityLiteralPrefixes -- e.g. `sw_version: "Home
	// Assistant Operating System " update.x!installed_version;`.
	capabilityPattern := regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*(?:/[A-Za-z_][A-Za-z0-9_]*)?):\s*(?:"([^"]*)"\s+)?(\S+)\s*;\s*$`)
	// Constant device attribute lines are distinguished from capability lines purely by shape:
	// a quoted string value (optionally followed by "forced"), vs. capabilityPattern's bare
	// entity-id-shaped token. E.g. `model: "Cool raspi";` or `hw_version: "1928930" forced;`.
	constantAttributePattern := regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*):\s*"([^"]*)"(?:\s+(forced))?\s*;\s*$`)

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
			if matches := constantAttributePattern.FindStringSubmatch(line); matches != nil {
				current.ConstantAttributes[matches[1]] = THostConstantAttribute{Value: matches[2], Forced: matches[3] == "forced"}
				continue
			}
			if matches := capabilityPattern.FindStringSubmatch(line); matches != nil {
				current.Capabilities[matches[1]] = matches[3]
				if matches[2] != "" {
					current.CapabilityLiteralPrefixes[matches[1]] = matches[2]
				}
				continue
			}
			warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line inside device %q capability block (body line %d): %q", current.DeviceID, lineIdx+1, line))
			continue
		}

		strippedLine, cloud, selfImport := parseRoutingKeywords(line)

		if matches := withDevicePattern.FindStringSubmatch(strippedLine); matches != nil {
			current = THostDevice{
				DeviceID: matches[1], HostName: matches[2], IntegrationType: matches[3],
				Capabilities:              map[string]string{},
				CapabilityLiteralPrefixes: map[string]string{},
				ConstantAttributes:        map[string]THostConstantAttribute{},
				Cloud:                     cloud, selfImport: selfImport,
			}
			inDeviceCapabilities = true
			continue
		}

		if matches := bareDevicePattern.FindStringSubmatch(strippedLine); matches != nil {
			devices = append(devices, THostDevice{DeviceID: matches[1], HostName: matches[2], IntegrationType: matches[3], Cloud: cloud, selfImport: selfImport})
			continue
		}

		warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line in \"integration hosts\" block (body line %d): %q", lineIdx+1, line))
	}

	return devices, warnings
}
