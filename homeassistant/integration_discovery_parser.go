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
//
// "<label>" and "<source>" are whitespace-separated, no colon between them (2026-09-19 -- a
// colon-optional trial ran first, then both houses' Physical.def/Logical.def were rewritten to
// the colon-less form and verified byte-identical on regenerate, so the colon alternative was
// dropped here outright). The old "domain.label: source;" form is no longer accepted at all --
// see logicalCapabilityPattern's own identical change for the full rationale (the colon visually
// clashed with Conceptual.def entity specs' own unrelated "sphere:path" colon).
var discoveryCapabilityPattern = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_/]*)\s+(.+?)\s*;\s*$`)

// discoveryAvailabilitySuffix is the " is available" sugar on a capability line's source,
// marking it as a reference to a SIBLING capability's own eventual entity (e.g. "binary_sensor.node:
// light.core is available;") rather than a raw gateway leaf id -- mirrors parseConditionDirective's
// identically-named sugar (expander.go), applied here to a capability declaration instead of an
// entity's own "condition" property. The sibling reference itself is domain-qualified
// ("<domain>.<label>", e.g. "light.core") -- required, not inferred, matching "derived
// DDD.NNN from EEE.MMM via TTT;"'s own "from" side (Physical_DerivedCapability.go).
const discoveryAvailabilitySuffix = " is available"

var discoveryAvailabilitySiblingPattern = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_/]*)$`)

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
		if line == "ignore other capabilities;" {
			current.IgnoreOtherCapabilities = true
			cur.Advance()
			continue
		}
		// "hidden " is an optional leading qualifier on a capability line (either shape below) --
		// stripped here, once, so the two patterns below never need to know about it. A hidden
		// capability still fully exists for internal use (a "derived ... from ...;" elsewhere on
		// the SAME device may still reference it, via the raw Capabilities map lookup, never the
		// entity-registration dispatch this marks), but registerDiscoveryEntityLink/
		// registerDeviceCapabilityEntityLink's discovery branch refuse to position it directly as
		// a Spaces.def entity -- it's an internal-only intermediate value (e.g. a sensor's raw,
		// unadjusted reading that only a "derived" capability should ever read), not something the
		// conceptual (or logical) layer should see.
		capabilityLine, hidden := strings.CutPrefix(line, "hidden ")

		// Checked first, before discoveryCapabilityPattern: "derived" is a distinct leading
		// keyword, unambiguous against "<domain>.<path>: ...;" -- same ordering rationale as
		// integration_hassbridge_parser.go's own dispatch (Physical_DerivedCapability.go).
		if decl, ok := parseDerivedCapabilityLine(capabilityLine); ok {
			current.Capabilities[decl.Label] = TDiscoveryCapability{
				Domain:                decl.Domain,
				DerivedFromCapability: decl.FromLabel,
				DerivedViaTemplate:    decl.Template,
				Hidden:                hidden,
			}
			cur.Advance()
			continue
		}
		if matches := discoveryCapabilityPattern.FindStringSubmatch(capabilityLine); matches != nil {
			if sibling, ok := strings.CutSuffix(matches[3], discoveryAvailabilitySuffix); ok {
				sibling = strings.TrimSpace(sibling)
				siblingMatch := discoveryAvailabilitySiblingPattern.FindStringSubmatch(sibling)
				if siblingMatch == nil {
					warnings = append(warnings, fmt.Sprintf("Physical.def: device %q's %q capability references %q, which isn't domain-qualified -- write \"<domain>.%s is available;\" (e.g. \"light.%s is available;\")", deviceID, matches[2], sibling, sibling, sibling))
					cur.Advance()
					continue
				}
				current.Capabilities[matches[2]] = TDiscoveryCapability{Domain: matches[1], AvailabilityOf: siblingMatch[2], AvailabilityOfDomain: siblingMatch[1], Hidden: hidden}
			} else if sourceDomain, leaf, ok := strings.Cut(matches[3], ":"); ok {
				// "<raw-domain>:<leaf>" (2026-09-15) -- an optional qualifier disambiguating a
				// leaf id Zigbee2MQTT (or any gateway) publishes under more than one raw MQTT
				// discovery domain for the SAME underlying property (real case: a Moes scene
				// remote's "action" gets both a legacy "sensor" text mirror and a richer "event"
				// entity, identical unique_id -- without this, the coordinator's own (gateway,
				// leaf) matching can't tell which raw message this capability means, and could
				// relay either one non-deterministically). Absent (the common case, no ':' in the
				// leaf) matches whichever raw domain publishes it, unchanged from before -- this is
				// what lets e.g. "fan.core: 0x..._switch_zigbee2mqtt;" deliberately relay a
				// switch-domain-published leaf under a different declared conceptual domain.
				current.Capabilities[matches[2]] = TDiscoveryCapability{Domain: matches[1], Leaf: leaf, SourceDomain: sourceDomain, Hidden: hidden}
			} else {
				current.Capabilities[matches[2]] = TDiscoveryCapability{Domain: matches[1], Leaf: matches[3], Hidden: hidden}
			}
			cur.Advance()
			continue
		}
		warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line inside device %q (body line %d): %q", deviceID, cur.ThisLineNumber(), line))
		cur.Advance()
	}

	warnings = append(warnings, fmt.Sprintf("Physical.def: device %q is missing its closing \"end;\"", deviceID))
	return finish(), warnings
}
