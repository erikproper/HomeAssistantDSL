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

// commandlineCapabilityPattern matches "<kind>.<name> <quoted-scripts>;" -- Kind/name mirror the
// "home_assistant"/"discovery" integrations' own mandatory-explicit-domain capability lines
// (e.g. integration_discovery_parser.go's discoveryCapabilityPattern); scripts are captured
// greedily as one raw string, split into individual quoted values by commandlineScriptPattern
// below. Whitespace-separated, no colon between <name> and <quoted-scripts> (2026-09-19 -- a
// colon-optional trial ran first, then both houses' Physical.def/Logical.def were rewritten to
// the colon-less form and verified byte-identical on regenerate, so the colon alternative was
// dropped here outright) -- see logicalCapabilityPattern's own identical change for the full
// rationale. Requires the explicit "capability" keyword (PROJECT.md item 11b, 2026-10-11) -- see
// discoveryCapabilityPattern's own doc comment for the general rationale.
// An optional frozen "as <id>" (PROJECT.md item 48/16) follows the name: the capability's
// unique_id, kept when the capability is renamed.
var commandlineCapabilityPattern = regexp.MustCompile(`^capability\s+(switch|sensor|button)\.([A-Za-z_][A-Za-z0-9_]*)(?:\s+as\s+([A-Za-z_][A-Za-z0-9_.]*))?\s+(.+?)\s*;\s*$`)
var commandlineScriptPattern = regexp.MustCompile(`"([^"]*)"`)

// commandlineDependencyOnPattern is "dependency on <device-id>;" inside a commandline device
// block -- repeatable, mirrors Logical.def's own logicalDependencyOnPattern exactly (2026-10-07:
// commandline is a physical-layer integration, so this dependency is declared at the physical
// layer directly rather than through a separate Logical.def entry -- the user's own explicit
// choice, unlike hassbridge/import which only get this via Logical.def). Requires the whole
// remainder before ";" to be ONE bare token, same safety discipline as the Logical.def pattern.
var commandlineDependencyOnPattern = regexp.MustCompile(`^dependency\s+on\s+(\S+);$`)

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
			if matches := commandlineDependencyOnPattern.FindStringSubmatch(line); matches != nil {
				current.DependsOn = append(current.DependsOn, matches[1])
				continue
			}
			if matches := commandlineCapabilityPattern.FindStringSubmatch(line); matches != nil {
				kind, name, assignedID, rawScripts := matches[1], matches[2], matches[3], matches[4]
				scripts := commandlineScriptPattern.FindAllStringSubmatch(rawScripts, -1)
				wantCount := commandlineScriptCountByKind[kind]
				if len(scripts) != wantCount {
					warnings = append(warnings, fmt.Sprintf("Physical.def: device %q: %q capability %q wants %d quoted script(s), got %d; ignored", current.DeviceID, kind, name, wantCount, len(scripts)))
					continue
				}
				cap := TCommandlineCapability{Kind: kind, AssignedID: assignedID}
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
