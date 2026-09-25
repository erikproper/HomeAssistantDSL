/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationLocalParser
 *
 * Parses the "device <id> with: <domain>.<label>; ...; end;" syntax inside Physical.def's
 * "integration local with: ... end;" block (already extracted by Physical_Parser.go's generic
 * wrapper parser) into TLocalDevice records (integration_local_storage.go).
 *
 * Two body-line shapes, both mirroring integration_discovery_parser.go's own capability-line
 * conventions exactly (same charset, same " is available" sibling sugar) since a "local" device's
 * eventual Conceptual.def positioning is deliberately meant to feel identical to a discovery
 * device's:
 *
 *   <domain>.<label>;
 *   <domain>.<label> <sibling-domain>.<sibling-label> is available;
 *
 * Unlike discovery, a "local" capability never has a raw source/leaf at all -- the first shape is
 * bare (nothing to relay, just an acknowledgement that the entity may exist on main), and the
 * second's only "source" is a same-device sibling reference.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 24.09.2026
 *
 */

package main

import (
	"fmt"
	"regexp"
	"strings"
)

var localWithDevicePattern = regexp.MustCompile(`^device\s+(\S+)\s+with:\s*$`)

// localCapabilityPattern matches "<domain>.<label>;" (bare) or "<domain>.<label> <rest>;" (the
// "is available" shape, <rest> checked against localAvailabilitySiblingPattern below) -- same
// two-group-plus-greedy-rest shape as discoveryCapabilityPattern, except <rest> is optional here
// (a bare capability has nothing after its own "<domain>.<label>").
var localCapabilityPattern = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)(?:\s+(.+?))?\s*;\s*$`)

// localAvailabilitySuffix mirrors discoveryAvailabilitySuffix exactly.
const localAvailabilitySuffix = " is available"

var localAvailabilitySiblingPattern = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)$`)

// parseLocalIntegrationBody parses the body lines of an "integration local with: ... end;" block
// into every declared device. Lines that don't parse cleanly are reported as warnings rather than
// aborting the parse.
func parseLocalIntegrationBody(bodyLines []string) ([]TLocalDevice, []string) {
	var devices []TLocalDevice
	var warnings []string

	inDeviceCapabilities := false
	var current TLocalDevice

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
			if matches := localCapabilityPattern.FindStringSubmatch(line); matches != nil {
				domain, label, rest := matches[1], matches[2], strings.TrimSpace(matches[3])
				if rest == "" {
					current.Capabilities[label] = TLocalCapability{Domain: domain}
					continue
				}
				sibling, ok := strings.CutSuffix(rest, localAvailabilitySuffix)
				if !ok {
					warnings = append(warnings, fmt.Sprintf("Physical.def: device %q's %q capability has unrecognised trailing text %q -- only \"<domain>.<label> is available\" is supported after a capability's own name", current.DeviceID, label, rest))
					continue
				}
				sibling = strings.TrimSpace(sibling)
				siblingMatch := localAvailabilitySiblingPattern.FindStringSubmatch(sibling)
				if siblingMatch == nil {
					warnings = append(warnings, fmt.Sprintf("Physical.def: device %q's %q capability references %q, which isn't domain-qualified -- write \"<domain>.%s is available;\" (e.g. \"camera.%s is available;\")", current.DeviceID, label, sibling, sibling, sibling))
					continue
				}
				current.Capabilities[label] = TLocalCapability{Domain: domain, AvailabilityOf: siblingMatch[2], AvailabilityOfDomain: siblingMatch[1]}
				continue
			}
			warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line inside device %q (body line %d): %q", current.DeviceID, lineIdx+1, line))
			continue
		}

		if matches := localWithDevicePattern.FindStringSubmatch(line); matches != nil {
			current = TLocalDevice{DeviceID: matches[1], Capabilities: map[string]TLocalCapability{}}
			inDeviceCapabilities = true
			continue
		}

		warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line in \"integration local\" block (body line %d): %q", lineIdx+1, line))
	}

	return devices, warnings
}
