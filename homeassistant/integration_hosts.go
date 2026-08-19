/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationHosts
 *
 * Integration-specific parser and output generation for Physical.def's
 * "integration hosts with: ... end;" block: local "device <id> <host> <type>
 * [with: <capability>: <entity>; ... end;]" syntax, generating the ping integration's
 * host list and a devices.yaml input file for the coordinator. See physical.go for the
 * generic "integration <name> with: ... end;" wrapper this plugs into.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 18.08.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// THostDevice is one "device <id> <host> <type> [with: <capability>: <entity>; ... end;]"
// declaration from the "integration hosts" block.
type THostDevice struct {
	DeviceID        string
	HostName        string
	IntegrationType string // "home_assistant", "cpu", or "ping"
	Capabilities    map[string]string // capability name -> Home Assistant entity id (home_assistant type only)
}

// parseHostsIntegrationBody parses the body lines of an "integration hosts with: ...
// end;" block (already extracted by physical.go's generic wrapper parser) into device
// declarations. Lines that don't parse cleanly (e.g. a malformed "device ...;" line) are
// reported as warnings rather than aborting the parse.
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

// dedupedHostNames returns every device's HostName, deduplicated and sorted, regardless
// of integration type -- home_assistant, cpu, and ping devices are all pingable hosts.
func dedupedHostNames(devices []THostDevice) []string {
	seen := map[string]bool{}
	var names []string
	for _, d := range devices {
		if d.HostName == "" || seen[d.HostName] {
			continue
		}
		seen[d.HostName] = true
		names = append(names, d.HostName)
	}
	sort.Strings(names)
	return names
}

// generatePingHostsFile writes <outputRoot>/ping/hosts: one host name per line, matching
// the plain-text format the existing ping/report script already expects (it globs
// ~/ping/hosts*). Deployment from this generated file to the actual Integrations/ping/
// location is handled outside the generator.
func generatePingHostsFile(outputRoot string, devices []THostDevice) error {
	hosts := dedupedHostNames(devices)
	var sb strings.Builder
	for _, host := range hosts {
		sb.WriteString(host + "\n")
	}
	dir := filepath.Join(outputRoot, "ping")
	return writeYAMLFile(filepath.Join(dir, "hosts"), sb.String())
}

// generateCoordinatorDevicesFile writes <outputRoot>/coordinator/devices.yaml: the raw
// physical-layer device/capability list the coordinator needs to build discovery
// messages. This intentionally carries no space/conceptual positioning -- that mapping
// doesn't exist yet (Entities.def hasn't been updated to connect these devices to
// spaces), so this stays a pure physical-layer (PSM) artifact for now.
func generateCoordinatorDevicesFile(outputRoot string, devices []THostDevice) error {
	seen := map[string]bool{}
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("devices:\n")
	for _, d := range devices {
		if seen[d.DeviceID] {
			continue
		}
		seen[d.DeviceID] = true
		sb.WriteString("  " + d.DeviceID + ":\n")
		sb.WriteString("    host: " + d.HostName + "\n")
		sb.WriteString("    integration: " + d.IntegrationType + "\n")
		if len(d.Capabilities) == 0 {
			sb.WriteString("    capabilities: {}\n")
			continue
		}
		sb.WriteString("    capabilities:\n")
		capNames := make([]string, 0, len(d.Capabilities))
		for name := range d.Capabilities {
			capNames = append(capNames, name)
		}
		sort.Strings(capNames)
		for _, name := range capNames {
			sb.WriteString("      " + name + ":\n")
			sb.WriteString("        entity: " + d.Capabilities[name] + "\n")
		}
	}
	dir := filepath.Join(outputRoot, "coordinator")
	return writeYAMLFile(filepath.Join(dir, "devices.yaml"), sb.String())
}

// warnDuplicateDeviceIDs reports device ids declared more than once (e.g. copy-paste
// leftovers when Physical.def's device list was assembled from Integrations.def's
// earlier draft) -- the first declaration wins for the coordinator devices file, but the
// duplication itself is worth surfacing rather than silently resolving.
func warnDuplicateDeviceIDs(devices []THostDevice) []string {
	seen := map[string]bool{}
	var warnings []string
	for _, d := range devices {
		if seen[d.DeviceID] {
			warnings = append(warnings, fmt.Sprintf("device id %q is declared more than once; keeping the first declaration", d.DeviceID))
			continue
		}
		seen[d.DeviceID] = true
	}
	return warnings
}

// generateHostsIntegrationOutputs is the entry point registered in physical.go's
// integrationBodyParsers for the "hosts" integration: parses the body then writes both
// the ping hosts file and the coordinator devices file.
func generateHostsIntegrationOutputs(bodyLines []string, outputRoot string) error {
	devices, warnings := parseHostsIntegrationBody(bodyLines)
	for _, w := range warnings {
		fmt.Printf("[integration hosts] %s\n", w)
	}
	if len(devices) == 0 {
		return nil
	}

	for _, w := range warnDuplicateDeviceIDs(devices) {
		fmt.Printf("[integration hosts] %s\n", w)
	}

	if err := generatePingHostsFile(outputRoot, devices); err != nil {
		return err
	}
	return generateCoordinatorDevicesFile(outputRoot, devices)
}
