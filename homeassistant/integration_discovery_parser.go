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

// discoveryMetadataPattern is a device's own metadata line (PROJECT.md 19b, 2026-10-26),
// "<field>: \"<value>\" [forced];", optionally followed by the generator's own "# discovery:
// ..." note -- the same shape a hosts device's constant attributes have. The generator writes
// these itself from the discovered metadata; a forced value is the author's and is kept.
var discoveryMetadataPattern = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*):\s*"([^"]*)"(?:\s+(forced))?\s*;\s*(?:#.*)?$`)

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
//
// Requires the explicit "capability" keyword (PROJECT.md item 11b, 2026-10-11, both houses
// rewritten wholesale and verified byte-identical on regenerate) -- every capability-declaring
// line in every integration kind now carries it, so a line that's merely referencing/modifying an
// existing capability (a device-level fact, a refinement, a block opener) is visually
// distinguishable on sight from one that's declaring a brand new one.
var discoveryCapabilityPattern = regexp.MustCompile(`^capability\s+([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_/]*)\s+(.+?)\s*;\s*$`)

// discoveryAvailabilitySuffix is the " is available" sugar on a capability line's source,
// marking it as a reference to a SIBLING capability's own eventual entity (e.g. "binary_sensor.node:
// light.core is available;") rather than a raw gateway leaf id -- mirrors parseConditionDirective's
// identically-named sugar (expander.go), applied here to a capability declaration instead of an
// entity's own "condition" property. The sibling reference itself is domain-qualified
// ("<domain>.<label>", e.g. "light.core") -- required, not inferred, matching "derived
// DDD.NNN from EEE.MMM via TTT;"'s own "from" side (Physical_DerivedCapability.go).
const discoveryAvailabilitySuffix = " is available"

var discoveryAvailabilitySiblingPattern = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_/]*)$`)

// discoveryAssignedIDPattern matches a plain raw-leaf capability's own trailing " as <id>" suffix
// (PROJECT.md item 7's stable coordinator<->HA entity identity design, memory/project_stable_
// entity_identity_design.md) -- "<id>" is whatever the generator or a one-time bootstrap pass
// already froze, carried through verbatim from here on; this grammar never interprets it, only
// stores and re-emits it. Greedy on the leading group so a multi-word match prefers the LAST
// " as <token>" in the line (irrelevant in practice -- no real raw leaf id contains " as " -- but
// correct regardless).
var discoveryAssignedIDPattern = regexp.MustCompile(`^(.+)\s+as\s+(\S+)$`)

// cutDiscoveryAssignedID splits a capability line's raw source text into its own prefix and a
// trailing " as <id>" suffix, if present. Called BEFORE the "is available"/"<raw-domain>:<leaf>"
// dispatch below, so an assigned id composes with either -- except "is available" itself, which
// parseDiscoveryDeviceBody's own dispatch rejects it on (AvailabilityOf capabilities never flow
// through relayedDiscoveryUniqueID, so freezing an id for one would be a silent no-op).
func cutDiscoveryAssignedID(source string) (rest, assignedID string, ok bool) {
	m := discoveryAssignedIDPattern.FindStringSubmatch(source)
	if m == nil {
		return source, "", false
	}
	return m[1], m[2], true
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
		if matches := discoveryMetadataPattern.FindStringSubmatch(line); matches != nil && knownConstantDeviceAttributes[matches[1]] {
			if current.Metadata == nil {
				current.Metadata = map[string]THostConstantAttribute{}
			}
			current.Metadata[matches[1]] = THostConstantAttribute{Value: matches[2], Forced: matches[3] != ""}
			cur.Advance()
			continue
		}
		if line == "entities via yaml;" {
			// PROJECT.md 19c (2026-10-27): Home Assistant gets this device's entities from the
			// generator's own YAML "mqtt:" config, not from the coordinator's relay.
			current.EntitiesViaYAML = true
			cur.Advance()
			continue
		}
		if line == "ignore other capabilities;" {
			current.IgnoreOtherCapabilities = true
			cur.Advance()
			continue
		}
		// "hidden "/"inverted " are optional leading qualifiers on a capability line (either shape
		// below), stackable in either order -- stripped here, once, so the two patterns below never
		// need to know about them. A hidden capability still fully exists for internal use (a
		// "derived ... from ...;" elsewhere on the SAME device may still reference it, via the raw
		// Capabilities map lookup, never the entity-registration dispatch this marks), but
		// registerDiscoveryEntityLink/registerDeviceCapabilityEntityLink's discovery branch refuse
		// to position it directly as a Spaces.def entity -- it's an internal-only intermediate value
		// (e.g. a sensor's raw, unadjusted reading that only a "derived" capability should ever
		// read), not something the conceptual (or logical) layer should see. "inverted" marks a
		// switch/binary_sensor whose raw on/off payload means the opposite of what it should
		// report/command (TDiscoveryCapability.Inverted's own doc comment has the full rationale).
		capabilityLine, hidden := strings.CutPrefix(line, "hidden ")
		capabilityLine, inverted := strings.CutPrefix(capabilityLine, "inverted ")
		if !hidden {
			// Also accept the opposite order, "inverted hidden ...;".
			if strippedHidden, ok := strings.CutPrefix(capabilityLine, "hidden "); ok {
				capabilityLine, hidden = strippedHidden, true
			}
		}

		// Checked first, before discoveryCapabilityPattern: "derived" is a distinct leading
		// keyword, unambiguous against "<domain>.<path>: ...;" -- same ordering rationale as
		// integration_hassbridge_parser.go's own dispatch (Physical_DerivedCapability.go).
		if decl, ok := parseDerivedCapabilityLine(capabilityLine); ok {
			current.Capabilities[decl.Label] = TDiscoveryCapability{
				Domain:                decl.Domain,
				DerivedFromCapability: decl.FromLabel,
				DerivedViaTemplate:    decl.Template,
				Hidden:                hidden,
				Inverted:              inverted,
			}
			cur.Advance()
			continue
		}
		if matches := discoveryCapabilityPattern.FindStringSubmatch(capabilityLine); matches != nil {
			rawSource, assignedID, hasAssignedID := cutDiscoveryAssignedID(matches[3])
			if !hasAssignedID {
				rawSource = matches[3]
			}
			if sibling, ok := strings.CutSuffix(rawSource, discoveryAvailabilitySuffix); ok {
				if hasAssignedID {
					warnings = append(warnings, fmt.Sprintf("Physical.def: device %q's %q capability has an \"as <id>;\" suffix, but it's an \"... is available;\" capability -- stable ids only apply to a plain raw-leaf capability; ignoring", deviceID, matches[2]))
				}
				sibling = strings.TrimSpace(sibling)
				siblingMatch := discoveryAvailabilitySiblingPattern.FindStringSubmatch(sibling)
				if siblingMatch == nil {
					warnings = append(warnings, fmt.Sprintf("Physical.def: device %q's %q capability references %q, which isn't domain-qualified -- write \"<domain>.%s is available;\" (e.g. \"light.%s is available;\")", deviceID, matches[2], sibling, sibling, sibling))
					cur.Advance()
					continue
				}
				current.Capabilities[matches[2]] = TDiscoveryCapability{Domain: matches[1], AvailabilityOf: siblingMatch[2], AvailabilityOfDomain: siblingMatch[1], Hidden: hidden, Inverted: inverted}
			} else if sourceDomain, leaf, ok := strings.Cut(rawSource, ":"); ok {
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
				current.Capabilities[matches[2]] = TDiscoveryCapability{Domain: matches[1], Leaf: leaf, SourceDomain: sourceDomain, Hidden: hidden, Inverted: inverted, AssignedID: assignedID}
			} else {
				current.Capabilities[matches[2]] = TDiscoveryCapability{Domain: matches[1], Leaf: rawSource, Hidden: hidden, Inverted: inverted, AssignedID: assignedID}
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
