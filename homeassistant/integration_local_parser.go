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
 * Body-line shapes, mostly mirroring integration_discovery_parser.go's own capability-line
 * conventions exactly (same charset, same " is available" sibling sugar) since a "local" device's
 * eventual Conceptual.def positioning is deliberately meant to feel identical to a discovery
 * device's:
 *
 *   <domain>.<label>;
 *   <domain>.<label> <sibling-domain>.<sibling-label> is available;
 *   derived <domain>.<label> from <sibling-domain>.<sibling-label> via <template>;
 *
 * Unlike discovery, a "local" capability never has a raw source/leaf at all -- the first shape is
 * bare (nothing to relay, just an acknowledgement that the entity may exist on main), and the
 * second's only "source" is a same-device sibling reference. The third ("derived", 2026-09-30,
 * applyLocalCapabilityLine's own first check -- shares Physical_DerivedCapability.go's parser with
 * discovery/hassbridge verbatim) is the one real exception to "a local capability is never a
 * computed value": it lets a local device express e.g. battery_alert as a genuine Physical.def
 * capability, closing the gap physical_rule_battery_alert_biconditional.go's own header documents.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 30.09.2026
 *
 */

package main

import (
	"fmt"
	"regexp"
	"strings"
)

var localWithDevicePattern = regexp.MustCompile(`^device\s+(\S+)\s+with:\s*$`)

// localWithDeviceOneLinerPattern is localWithDevicePattern's own single-statement shorthand --
// "device <id> with <single-capability-statement>;" instead of "device <id> with: <statement>;
// end;" (2026-09-27, the generic with-clause dispatcher, with_clause.go) -- for a device
// declaring exactly one capability (e.g. the Ring doorbell's own bare "camera.core;" shape).
var localWithDeviceOneLinerPattern = regexp.MustCompile(`^device\s+(\S+)\s+with\s+(.+?)\s*;\s*$`)

// localCapabilityPattern matches "<domain>.<label>;" (bare) or "<domain>.<label> <rest>;" (the
// "is available" shape, <rest> checked against localAvailabilitySiblingPattern below) -- same
// two-group-plus-greedy-rest shape as discoveryCapabilityPattern, except <rest> is optional here
// (a bare capability has nothing after its own "<domain>.<label>"). Requires the explicit
// "capability" keyword (PROJECT.md item 11b, 2026-10-11) -- see discoveryCapabilityPattern's own
// doc comment for the general rationale.
var localCapabilityPattern = regexp.MustCompile(`^capability\s+([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)(?:\s+(.+?))?\s*;\s*$`)

// localAvailabilitySuffix mirrors discoveryAvailabilitySuffix exactly.
const localAvailabilitySuffix = " is available"

var localAvailabilitySiblingPattern = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)$`)

// localCapabilityWithPattern opens a "switch.<label> <media-player-entity> with: ... end;" block
// (2026-09-27, the media_player->switch coercion relocated here from Logical.def -- see
// TLocalCapability's own doc comment) -- the ONLY local capability shape that ever needs a nested
// block, for the optional "no_play_input" refinement. Checked before localCapabilityPattern, same
// mutually-exclusive-by-trailing-token ordering every other integration kind's own
// "<domain>.<label> <value> with:" pattern already uses.
var localCapabilityWithPattern = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)\s+(.+?)\s*with:\s*$`)

// localNoPlayInputPattern mirrors Logical.def's own identically-named directive exactly (same
// field, same grammar, just a new home) -- see TLocalCapability.NoPlayInput's own doc comment.
var localNoPlayInputPattern = regexp.MustCompile(`^no_play_input\s+"([^"]*)"\s*;\s*$`)

// localMediaSwitchCapabilityStatements is the shared (2026-09-27, the generic with-clause
// dispatcher, with_clause.go) single-entry statement list for a media-switch capability's own
// "with: ... end;" block -- used identically by both the multi-line block form (the
// pendingCapabilityLabel branch below) and localCapabilityWithOneLinerPattern's folded one-liner.
var localMediaSwitchCapabilityStatements = []TWithClauseStatement[TLocalCapability]{
	{
		Pattern: localNoPlayInputPattern,
		Apply: func(cap TLocalCapability, m []string) TLocalCapability {
			cap.NoPlayInput = m[1]
			return cap
		},
	},
}

// localCapabilityWithOneLinerPattern is localCapabilityWithPattern's own single-statement
// shorthand -- "switch.<label> <media-player-entity> with <single-statement>;" instead of
// "switch.<label> <media-player-entity> with: <single-statement>; end;".
// Requires the explicit "capability" keyword at the very front (PROJECT.md item 11b, 2026-10-11)
// -- same rationale as integration_hassbridge_parser.go's own identical capabilityWithOneLinerPattern
// change.
var localCapabilityWithOneLinerPattern = regexp.MustCompile(`^capability\s+([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)\s+(.+?)\s+with\s+(.+?)\s*;\s*$`)

// applyLocalCapabilityLine applies ONE "<domain>.<label>;"/"<domain>.<label> ... is available;"/
// "<domain>.<label> <media-player-entity>;" line (localCapabilityPattern's own shape) to dev,
// returning (warning, true) if the line matched this shape at all (warning is "" on success),
// or ("", false) if it didn't match localCapabilityPattern and the caller should try something
// else. Shared (2026-09-27, the generic with-clause dispatcher's own spirit -- a single complex
// pattern with internal branching doesn't reduce to with_clause.go's flat statement-list model,
// but the "block form and one-liner fold call the SAME logic" principle still applies) by both
// the block form's own loop below and localWithDeviceOneLinerPattern's folded one-liner.
func applyLocalCapabilityLine(dev *TLocalDevice, deviceID, line string) (warning string, matched bool) {
	// Checked first, before localCapabilityPattern: "derived" is a distinct leading keyword,
	// unambiguous against "<domain>.<label>...;" -- same ordering rationale as
	// integration_discovery_parser.go/integration_hassbridge_parser.go's own dispatch
	// (Physical_DerivedCapability.go).
	if decl, ok := parseDerivedCapabilityLine(line); ok {
		dev.Capabilities[decl.Label] = TLocalCapability{
			Domain:                decl.Domain,
			DerivedFromCapability: decl.FromLabel,
			DerivedViaTemplate:    decl.Template,
		}
		return "", true
	}

	matches := localCapabilityPattern.FindStringSubmatch(line)
	if matches == nil {
		return "", false
	}
	domain, label, rest := matches[1], matches[2], strings.TrimSpace(matches[3])
	if rest == "" {
		dev.Capabilities[label] = TLocalCapability{Domain: domain}
		return "", true
	}
	if sibling, ok := strings.CutSuffix(rest, localAvailabilitySuffix); ok {
		sibling = strings.TrimSpace(sibling)
		siblingMatch := localAvailabilitySiblingPattern.FindStringSubmatch(sibling)
		if siblingMatch == nil {
			return fmt.Sprintf("device %q's %q capability references %q, which isn't domain-qualified -- write \"<domain>.%s is available;\" (e.g. \"camera.%s is available;\")", deviceID, label, sibling, sibling, sibling), true
		}
		dev.Capabilities[label] = TLocalCapability{Domain: domain, AvailabilityOf: siblingMatch[2], AvailabilityOfDomain: siblingMatch[1]}
		return "", true
	}
	if domain != "switch" {
		return fmt.Sprintf("device %q's %q capability has unrecognised trailing text %q -- only \"<domain>.<label> is available;\" or (domain \"switch\" only) a literal media_player entity is supported after a capability's own name", deviceID, label, rest), true
	}
	dev.Capabilities[label] = TLocalCapability{Domain: domain, MediaPlayerEntity: rest}
	return "", true
}

// parseLocalIntegrationBody parses the body lines of an "integration local with: ... end;" block
// into every declared device. Lines that don't parse cleanly are reported as warnings rather than
// aborting the parse.
func parseLocalIntegrationBody(bodyLines []string) ([]TLocalDevice, []string) {
	var devices []TLocalDevice
	var warnings []string

	inDeviceCapabilities := false
	var current TLocalDevice
	pendingCapabilityLabel := "" // "" when not inside a media-switch capability's own "with: ... end;" block

	for lineIdx, rawLine := range bodyLines {
		line := stripLineComment(rawLine)
		if line == "" {
			continue
		}

		if inDeviceCapabilities && pendingCapabilityLabel != "" {
			if line == "end;" {
				pendingCapabilityLabel = ""
				continue
			}
			cap, exists := current.Capabilities[pendingCapabilityLabel]
			if !exists {
				// The block's own header was already rejected (wrong domain, warned about there) and
				// never created a capability entry to attach this to -- still consume its body lines
				// silently (up to the "end;" handled above), rather than letting them fall through as
				// further, misleadingly-unrelated "unrecognised line" warnings on top of the one
				// already reported for the header.
				continue
			}
			if updated, ok := ApplyWithClauseBodyLine(cap, line, localMediaSwitchCapabilityStatements); ok {
				current.Capabilities[pendingCapabilityLabel] = updated
				continue
			}
			warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line inside capability %q's \"with:\" block (body line %d): %q", pendingCapabilityLabel, lineIdx+1, line))
			continue
		}

		if inDeviceCapabilities {
			if line == "end;" {
				devices = append(devices, current)
				inDeviceCapabilities = false
				continue
			}
			if matches := localCapabilityWithPattern.FindStringSubmatch(line); matches != nil {
				domain, label, source := matches[1], matches[2], matches[3]
				if domain != "switch" {
					warnings = append(warnings, fmt.Sprintf("Physical.def: device %q's %q capability opens a \"with:\" block, but only \"switch.<label> <media-player-entity> with: ...;\" (the media_player->switch coercion) supports one -- domain %q isn't", current.DeviceID, label, domain))
					// Still consume the block's own body lines (up to its matching "end;") silently,
					// rather than letting "no_play_input"/"end;" fall through as further,
					// misleadingly-unrelated "unrecognised line" warnings on top of the one above.
					pendingCapabilityLabel = label
					continue
				}
				current.Capabilities[label] = TLocalCapability{Domain: domain, MediaPlayerEntity: source}
				pendingCapabilityLabel = label
				continue
			}
			if matches := localCapabilityWithOneLinerPattern.FindStringSubmatch(line); matches != nil {
				domain, label, source := matches[1], matches[2], matches[3]
				if domain != "switch" {
					warnings = append(warnings, fmt.Sprintf("Physical.def: device %q's %q capability's one-line \"with\" refinement isn't supported for domain %q -- only \"switch.<label> <media-player-entity> with ...;\" (the media_player->switch coercion) does", current.DeviceID, label, domain))
					continue
				}
				cap := TLocalCapability{Domain: domain, MediaPlayerEntity: source}
				body := strings.TrimSpace(matches[4]) + ";"
				if updated, ok := ApplyWithClauseBodyLine(cap, body, localMediaSwitchCapabilityStatements); ok {
					current.Capabilities[label] = updated
					continue
				}
				warnings = append(warnings, fmt.Sprintf("Physical.def: device %q's %q capability one-line \"with %s\" body isn't a recognised single-statement form -- only \"no_play_input \\\"<value>\\\";\" may be shortened this way (body line %d)", current.DeviceID, label, body, lineIdx+1))
				continue
			}
			if warning, matched := applyLocalCapabilityLine(&current, current.DeviceID, line); matched {
				if warning != "" {
					warnings = append(warnings, "Physical.def: "+warning)
				}
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

		if matches := localWithDeviceOneLinerPattern.FindStringSubmatch(line); matches != nil {
			deviceID := matches[1]
			body := strings.TrimSpace(matches[2]) + ";"
			dev := TLocalDevice{DeviceID: deviceID, Capabilities: map[string]TLocalCapability{}}
			if warning, matched := applyLocalCapabilityLine(&dev, deviceID, body); matched {
				if warning != "" {
					warnings = append(warnings, "Physical.def: "+warning)
				}
				// The device is still added even if its one-and-only capability line had a content
				// problem (warned above) -- matching the block form's own parity: a device is
				// positioned regardless of whether every one of its capability lines individually
				// succeeded.
				devices = append(devices, dev)
				continue
			}
			warnings = append(warnings, fmt.Sprintf("Physical.def: device %q's one-line \"with %s\" body isn't a recognised single-statement form (body line %d)", deviceID, body, lineIdx+1))
			continue
		}

		warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line in \"integration local\" block (body line %d): %q", lineIdx+1, line))
	}

	return devices, warnings
}
