/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationCommandlineParser
 *
 * Parses the "device <id> <host> with: <kind>.<name>: <quoted-scripts>; ...; end;" syntax inside
 * Physical.def's "integration commandline with: ... end;" block (already extracted by
 * Physical_Parser.go's generic wrapper parser) into TCommandlineDevice records
 * (integration_commandline_storage.go).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 06.09.2026
 *
 */

package main

import (
	"fmt"
	"regexp"
	"strings"
)

var commandlineWithDevicePattern = regexp.MustCompile(`^device\s+(\S+)\s+(\S+)\s+with:\s*$`)

// commandlineCapabilityPattern matches "<kind>.<name>: <quoted-scripts>;" -- Kind/name mirror the
// "home_assistant"/"discovery" integrations' own mandatory-explicit-domain capability lines
// (e.g. integration_discovery_parser.go's discoveryCapabilityPattern); scripts are captured
// greedily as one raw string, split into individual quoted values by commandlineScriptPattern
// below.
var commandlineCapabilityPattern = regexp.MustCompile(`^(switch|sensor|button)\.([A-Za-z_][A-Za-z0-9_]*):\s*(.+?)\s*;\s*$`)
var commandlineScriptPattern = regexp.MustCompile(`"([^"]*)"`)

// commandlineScriptCountByKind is how many quoted scripts each capability kind requires, and in
// which order they map onto TCommandlineCapability's own fields -- see that type's doc comment.
var commandlineScriptCountByKind = map[string]int{
	"sensor": 1,
	"switch": 3,
	"button": 1,
}

// parseCommandlineIntegrationBody parses the body lines of an "integration commandline with:
// ... end;" block into every declared device. Lines that don't parse cleanly are reported as
// warnings rather than aborting the parse.
func parseCommandlineIntegrationBody(bodyLines []string) ([]TCommandlineDevice, []string) {
	var devices []TCommandlineDevice
	var warnings []string

	inDeviceCapabilities := false
	var current TCommandlineDevice

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
			if matches := commandlineCapabilityPattern.FindStringSubmatch(line); matches != nil {
				kind, name, rawScripts := matches[1], matches[2], matches[3]
				scripts := commandlineScriptPattern.FindAllStringSubmatch(rawScripts, -1)
				wantCount := commandlineScriptCountByKind[kind]
				if len(scripts) != wantCount {
					warnings = append(warnings, fmt.Sprintf("Physical.def: device %q: %q capability %q wants %d quoted script(s), got %d; ignored", current.DeviceID, kind, name, wantCount, len(scripts)))
					continue
				}
				cap := TCommandlineCapability{Kind: kind}
				switch kind {
				case "sensor":
					cap.StatusScript = scripts[0][1]
				case "switch":
					cap.StatusScript, cap.OnScript, cap.OffScript = scripts[0][1], scripts[1][1], scripts[2][1]
				case "button":
					cap.PressScript = scripts[0][1]
				}
				current.Capabilities[name] = cap
				continue
			}
			warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line inside device %q (body line %d): %q", current.DeviceID, lineIdx+1, line))
			continue
		}

		if matches := commandlineWithDevicePattern.FindStringSubmatch(line); matches != nil {
			current = TCommandlineDevice{DeviceID: matches[1], Host: matches[2], Capabilities: map[string]TCommandlineCapability{}}
			inDeviceCapabilities = true
			continue
		}

		warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line in \"integration commandline\" block (body line %d): %q", lineIdx+1, line))
	}

	return devices, warnings
}
