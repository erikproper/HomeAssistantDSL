/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationDiscoveryParser
 *
 * Parses the "device <id> with: ... end;" syntax inside Physical.def's "integration discovery
 * with: ... end;" block (already extracted by Physical_Parser.go's generic wrapper parser) into
 * TDiscoveryGatewayDevice records (integration_discovery_storage.go). A device may nest further
 * "device <id> with: ... end;" declarations of its own (modelling a real gateway's own device
 * hierarchy, e.g. EMS-ESP's boiler/thermostat/mixer under its "ems-esp" root) to any depth, so
 * this is a real recursive-descent parse (via TLineCursor, defined.go) rather than the flat,
 * single-level state machine most other "device ... with: ... end;" bodies use elsewhere in this
 * codebase.
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

var discoveryDevicePattern = regexp.MustCompile(`^device\s+(\S+)\s+with:\s*$`)
var discoveryIdentifiersPattern = regexp.MustCompile(`^identifiers\s+"([^"]*)"\s*;\s*$`)

// Domain is mandatory and explicit, same shape and rationale as the "home_assistant" integration
// grammar's own capability lines (integration_hassbridge_parser.go): no implicit "always sensor"
// default. Source captured greedily so a leaf name containing "/" or other punctuation still
// parses.
var discoveryCapabilityPattern = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_/]*):\s*(.+?)\s*;\s*$`)

// bareDiscoveryLeaf strips a capability line's source value down to the bare, raw gateway leaf
// id decodeDiscoveryPayload matches live traffic against (payload.UniqueID, e.g.
// "boiler_outdoortemp") -- a source written domain-prefixed ("sensor.boiler_outdoortemp", the
// same shape as the gateway's own reported default_entity_id) has that prefix dropped; a source
// with no dot at all (the bare leaf id directly) passes through unchanged.
func bareDiscoveryLeaf(source string) string {
	source = strings.TrimSpace(source)
	if idx := strings.Index(source, "."); idx >= 0 {
		return source[idx+1:]
	}
	return source
}

// parseDiscoveryIntegrationBody parses the body lines of an "integration discovery with: ...
// end;" block into every declared gateway device, at any nesting depth, flattened into one
// slice. Lines that don't parse cleanly are reported as warnings rather than aborting the parse.
func parseDiscoveryIntegrationBody(bodyLines []string) ([]TDiscoveryGatewayDevice, []string) {
	cur := newLineCursor(bodyLines)
	var devices []TDiscoveryGatewayDevice
	var warnings []string

	for !cur.AtEnd() {
		line := cur.ThisLine()
		if matches := discoveryDevicePattern.FindStringSubmatch(line); matches != nil {
			cur.Advance()
			parsed, w := parseDiscoveryDeviceBody(cur, matches[1], "")
			devices = append(devices, parsed...)
			warnings = append(warnings, w...)
			continue
		}
		warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line in \"integration discovery\" block (body line %d): %q", cur.ThisLineNumber(), line))
		cur.Advance()
	}

	return devices, warnings
}

// parseDiscoveryDeviceBody parses one "device <id> with: ... end;" block's body (cur positioned
// just past its header line), recursing into any further nested "device ... with: ... end;"
// declarations. Returns deviceID's own record first, followed by every nested device found
// (flattened, depth-first) -- collectDiscoveryGatewaysByID doesn't care about order, only that
// every device ends up keyed by its own DeviceID.
func parseDiscoveryDeviceBody(cur *TLineCursor, deviceID, parentDeviceID string) ([]TDiscoveryGatewayDevice, []string) {
	current := TDiscoveryGatewayDevice{DeviceID: deviceID, ParentDeviceID: parentDeviceID, Capabilities: map[string]TDiscoveryCapability{}}
	var nested []TDiscoveryGatewayDevice
	var warnings []string

	finish := func() []TDiscoveryGatewayDevice {
		current.Identifiers = append(current.Identifiers, inferredDiscoveryIdentifier(deviceID))
		return append([]TDiscoveryGatewayDevice{current}, nested...)
	}

	for !cur.AtEnd() {
		line := cur.ThisLine()
		if line == "end;" {
			cur.Advance()
			return finish(), warnings
		}
		if matches := discoveryDevicePattern.FindStringSubmatch(line); matches != nil {
			cur.Advance()
			childDevices, w := parseDiscoveryDeviceBody(cur, matches[1], deviceID)
			nested = append(nested, childDevices...)
			warnings = append(warnings, w...)
			continue
		}
		if matches := discoveryIdentifiersPattern.FindStringSubmatch(line); matches != nil {
			current.Identifiers = append(current.Identifiers, matches[1])
			cur.Advance()
			continue
		}
		if matches := discoveryCapabilityPattern.FindStringSubmatch(line); matches != nil {
			current.Capabilities[matches[2]] = TDiscoveryCapability{Domain: matches[1], Leaf: bareDiscoveryLeaf(matches[3])}
			cur.Advance()
			continue
		}
		warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line inside device %q (body line %d): %q", deviceID, cur.ThisLineNumber(), line))
		cur.Advance()
	}

	warnings = append(warnings, fmt.Sprintf("Physical.def: device %q is missing its closing \"end;\"", deviceID))
	return finish(), warnings
}
