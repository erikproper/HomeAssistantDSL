/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationLogicalParser
 *
 * Parses Logical.def's "logical layer with: ... end;" block body -- a flat sequence of
 *
 *   "device <device-id> with:
 *      dependency on <device-id>;
 *      ...
 *    end;"
 *
 * declarations, no outer "integration X with:" wrapper (unlike Physical.def's discovery/hosts/
 * hassbridge/import kinds): the logical layer isn't tied to any external protocol/integration, so
 * there's nothing to nest under -- collectLayerContent already strips the "logical layer with: ...
 * end;" wrapper itself, handing this file only the body. See integration_logical_storage.go's own
 * header comment for what a logical device actually means and how "dependency on" resolves.
 *
 * "dependency on <device-id>;" and "<domain>.<label>: <value> [is available] [with: enabler
 * <entity-ref>; delay_off <time>; no_play_input "<value>"; end;];" are the two capability shapes
 * built so far (2026-09-16). Two more dependency forms are planned next, once a real Vienna case
 * needs them: "dependency on entity <entity-id>;" and "dependency on availability of entity
 * <entity-id>;", both naming a raw, already-existing HA entity on "main" directly rather than
 * another Physical.def device id (requiring a new generator-authored automation on main to publish
 * that entity's own state/availability to MQTT in the first place, plus adding it to kind-5's
 * existence-check list) -- deliberately not built ahead of a concrete need, same "on demand"
 * discipline as the rest of this layer.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 16.09.2026
 *
 */

package main

import (
	"fmt"
	"regexp"
	"strings"
)

var logicalDevicePattern = regexp.MustCompile(`^device\s+(\S+)\s+with:\s*$`)

// logicalDeviceOneLinerPattern is the single-statement shorthand for logicalDevicePattern's own
// block form -- "device <id> with <single-statement>;" instead of "device <id> with:
// <single-statement>; end;" -- for the common case of a device declaring exactly one body line
// (2026-09-25, the user's own explicit "all with clauses should follow this logic" instruction --
// picture_frame's single "dependency on ...;" was the concrete trigger). Distinguishable from
// logicalDevicePattern's own block form by construction: the block form's line ends in a bare
// ":" (nothing else on the line), this one requires content plus a trailing ";" -- no line can
// match both. Scoped to the two body-line shapes that are themselves already single-line-complete
// (a "dependency on <id>;" or a bare "<domain>.<label> <value> [is available];" capability) --
// tried against logicalDependencyOnPattern then logicalCapabilityPattern by
// parseLogicalLayerBody itself, not here (this is only the header/body-text split). A
// block-opening body line (absorb/defined/derived/capability-with-block) inherently needs more
// than one line anyway, so there is no one-liner shape for those to collapse into.
var logicalDeviceOneLinerPattern = regexp.MustCompile(`^device\s+(\S+)\s+with\s+(.+?)\s*;\s*$`)

// logicalDependencyOnPattern intentionally requires the whole remainder before ";" to be ONE
// bare token -- "dependency on entity binary_sensor.KKK;"/"dependency on availability of entity
// DDD.KKK;" (two/four tokens) fall through to the generic "unrecognised line" warning rather than
// being misread as a device-id of "entity"/"availability" -- exactly the safety this shape needs
// until those two forms are actually built (see this file's own header comment).
var logicalDependencyOnPattern = regexp.MustCompile(`^dependency\s+on\s+(\S+)\s*;\s*$`)

// logicalCapabilityPattern is a plain, self-contained "<domain>.<label> <value>;" line -- mirrors
// discoveryCapabilityPattern's own shape exactly (integration_discovery_parser.go).
//
// Whitespace-separated, no colon between <label> and <value> (2026-09-19, per the user's own
// request -- a colon-optional trial ran first, then both houses' Physical.def/Logical.def were
// rewritten to the colon-less form and verified byte-identical on regenerate, so the colon
// alternative was dropped here outright). Motivation: a capability's own declared name (e.g.
// "sensor.power", "switch.core") reads exactly like a Conceptual.def entity spec, and a colon
// immediately after it would visually clash with that OTHER, unrelated "sphere:path" colon
// (normalizeEntityFullName/namingSpacePath) -- confirmed harmless everywhere the old colon form
// was purely vestigial (Vienna's vacuum device block had it trailing an already-complete path for
// no semantic reason at all, silently absorbed by the path-trimming logic). The old
// "domain.label: value;" form is no longer accepted at all.
var logicalCapabilityPattern = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_/]*)\s+(.+?)\s*;\s*$`)

// logicalCapabilityWithBlockPattern is the same capability line, opening a nested "with: ...
// end;" block of its own (enabler/delay_off/no_play_input -- logicalCapabilityBodyPattern below).
// Same colon-dropped grammar as logicalCapabilityPattern just above.
var logicalCapabilityWithBlockPattern = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_/]*)\s+(.+?)\s+with:\s*$`)

// logicalIsAvailableSuffix mirrors discoveryAvailabilitySuffix's own sugar (integration_discovery_
// parser.go) -- here the reference is always a raw, already-existing HA entity ("main"), never a
// same-device sibling capability label, so no domain-qualification is needed on this side (unlike
// discovery's own "<domain>.<label> is available;").
const logicalIsAvailableSuffix = " is available"

// logicalCapabilityBodyPattern matches any ONE of a capability's own nested "with: ... end;" body
// lines -- exactly the three refinements built so far, any subset, any order (matching this DSL's
// general "maybe X" convention of independent, order-insensitive directives). "dependency on" is
// handled separately below (logicalDependencyOnPattern, repeatable, unlike these three), not folded
// into this alternation.
var logicalCapabilityBodyPattern = regexp.MustCompile(`^(enabler|delay_off)\s+(\S+)\s*;\s*$`)
var logicalNoPlayInputPattern = regexp.MustCompile(`^no_play_input\s+"([^"]*)"\s*;\s*$`)

// logicalAbsorbBlockPattern opens a device's own "capabilities from <device-id>: ... end;" block
// (2026-09-18, the "absorb" operation -- memory/project_logical_layer_device_combining.md) -- a
// SECOND kind of nested block inside a "device <id> with: ... end;" declaration, alongside a
// capability's own "with:" block (logicalCapabilityWithBlockPattern), never nested inside one
// another. Body lines are "<domain>.<label>: <bare-capability>;" (logicalCapabilityPattern's own
// shape reused verbatim -- same "<domain>.<label>: <value>;" text shape as an ordinary capability
// line, just interpreted differently: the RHS is a bare capability reference on <device-id>, not a
// literal entity_id). No "enabler" line here -- an absorbed device's own switch acting as an
// enabler of the HOST device's overall availability is expressed as a separate, ordinary
// IsAvailable capability instead (see TLogicalCapability.AbsorbedFromDeviceID's own doc comment
// for why).
var logicalAbsorbBlockPattern = regexp.MustCompile(`^capabilities\s+from\s+(\S+):\s*$`)

// logicalAbsorbBareCapabilityPattern is a shorthand for the absorb block's own
// "<domain>.<label>: <bare-capability>;" line (2026-09-19): "<domain>.<label>;" with no ":<value>"
// at all, when the absorbed device's own capability name is IDENTICAL to <label> -- e.g.
// "switch.core;" instead of "switch.core: switch.core;". A rename (label differs from the
// absorbed device's own capability name) still requires the explicit ":<value>" form; this
// pattern doesn't match a line that has one, so there's no ambiguity between the two shapes.
var logicalAbsorbBareCapabilityPattern = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_/]*)\s*;\s*$`)

// logicalDefinedBlockPattern opens a device's own "defined <domain>.<label> with: minimum <val>;
// maximum <val>; step <val>; icon <val>; units <val>; end;" block (2026-09-18) -- declares an HA
// input_number HELPER entity (see TLogicalCapability.IsDefinedInputNumber's own doc comment). A
// THIRD kind of nested block inside "device <id> with: ... end;", alongside a capability's own
// "with:" block and an absorb block, never nested inside either.
var logicalDefinedBlockPattern = regexp.MustCompile(`^defined\s+([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_/]*)\s+with:\s*$`)

// logicalDefinedFieldPattern matches one of a "defined" block's five body lines -- all five are
// required by every real case so far, but parsed independently (any subset, any order), same "maybe
// X" convention as logicalCapabilityBodyPattern above.
var logicalDefinedFieldPattern = regexp.MustCompile(`^(minimum|maximum|step|icon|units)\s+(.+?)\s*;\s*$`)

// logicalDerivedBlockPattern opens a device's own "derived <domain>.<label> with: condition
// <bool-expr>; device_class <val>; delay_on <time>; delay_off <time>; end;" (or "value <atom>;"
// for a non-"binary_sensor" domain) block (2026-09-18, grammar redesigned 2026-09-24) -- see
// TLogicalCapability.IsDerivedCondition/IsDerivedValue's own doc comment. A FOURTH kind of nested
// block inside "device <id> with: ... end;".
var logicalDerivedBlockPattern = regexp.MustCompile(`^derived\s+([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_/]*)\s+with:\s*$`)

// logicalDerivedFieldPattern matches one of a "derived" block's OWN non-"condition"/"value" body
// lines -- device_class/delay_on/delay_off, any subset, any order.
var logicalDerivedFieldPattern = regexp.MustCompile(`^(device_class|delay_on|delay_off)\s+(.+?)\s*;\s*$`)

// findTopLevelSemicolon returns the byte index of the first ";" in s that is outside a quoted
// string and at bracket/paren depth 0, or -1 if s doesn't (yet) contain one -- used to accumulate
// a "derived" block's own "condition"/"value" expression across possibly-several physical lines
// (the user's own preferred layout breaks a long boolean expression across lines, e.g. a line
// ending in "and") before handing the complete raw text to parseConditionExpr/parseValueExpr
// (logical_condition_expr.go), which do the real tokenising/recursive-descent parsing -- this is
// plain depth counting to find where to CUT the accumulated text, not a grammar of its own, so it
// doesn't run against memory/feedback_no_new_regexp_parsers.md's own concern.
func findTopLevelSemicolon(s string) int {
	depth := 0
	inString := false
	for i, r := range s {
		if inString {
			if r == '"' {
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
		case '(', '{':
			depth++
		case ')', '}':
			depth--
		case ';':
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// parseLogicalCapabilityValue strips logicalIsAvailableSuffix from raw if present.
func parseLogicalCapabilityValue(raw string) (entity string, isAvailable bool) {
	if entity, ok := strings.CutSuffix(raw, logicalIsAvailableSuffix); ok {
		return strings.TrimSpace(entity), true
	}
	return raw, false
}

// parseLogicalLayerBody parses Logical.def's own layer body into every declared logical device.
// Lines that don't parse cleanly are reported as warnings rather than aborting the parse, matching
// every other integration's own body parser policy (e.g. parseDiscoveryIntegrationBody).
func parseLogicalLayerBody(bodyLines []string, jinjaTemplates map[string]TJinjaTemplateDefinition) ([]TLogicalDevice, []string) {
	var devices []TLogicalDevice
	var warnings []string

	inDevice := false
	inCapability := false
	inAbsorb := false
	inDefined := false
	inDerived := false
	inDerivedExpr := false
	var current TLogicalDevice
	var currentCapLabel string
	var currentCap TLogicalCapability
	var currentAbsorbDeviceID string
	var currentDefinedLabel, currentDefinedDomain string
	var currentDefinedMin, currentDefinedMax, currentDefinedStep, currentDefinedIcon, currentDefinedUnits string
	var currentDerivedLabel, currentDerivedDomain string
	var currentDerivedCondition, currentDerivedValue *TConditionExpr
	var currentDerivedDeviceClass, currentDerivedDelayOn, currentDerivedDelayOff string
	var derivedExprKind, derivedExprBuffer string

	// tryCloseDerivedExpr checks whether derivedExprBuffer now contains a complete, balanced
	// "condition"/"value" expression (a top-level ";") -- if so, hands the raw text to
	// parseConditionExpr/parseValueExpr (logical_condition_expr.go) and leaves inDerivedExpr.
	// Called both right after the head line ("condition "/"value " prefix, possibly already
	// complete on one line) and after each further accumulated line, so a multi-line expression
	// (the user's own preferred layout, breaking a long boolean expression across lines) closes
	// the moment it balances, regardless of how many lines it took.
	tryCloseDerivedExpr := func(lineIdx int) {
		idx := findTopLevelSemicolon(derivedExprBuffer)
		if idx < 0 {
			return
		}
		raw := derivedExprBuffer[:idx]
		trailing := strings.TrimSpace(derivedExprBuffer[idx+1:])
		if trailing != "" {
			warnings = append(warnings, fmt.Sprintf("Logical.def: unexpected content after device %q's %q derived %q expression (body line %d): %q", current.DeviceID, currentDerivedLabel, derivedExprKind, lineIdx+1, trailing))
		}
		sourceLabel := fmt.Sprintf("Logical.def: device %q's %q derived %q", current.DeviceID, currentDerivedLabel, derivedExprKind)
		if derivedExprKind == "condition" {
			tree, exprWarnings := parseConditionExpr(raw, jinjaTemplates, sourceLabel)
			warnings = append(warnings, exprWarnings...)
			currentDerivedCondition = tree
		} else {
			tree, exprWarnings := parseValueExpr(raw, jinjaTemplates, sourceLabel)
			warnings = append(warnings, exprWarnings...)
			currentDerivedValue = tree
		}
		inDerivedExpr = false
	}

	for lineIdx, rawLine := range bodyLines {
		line := strings.TrimSpace(rawLine)
		if commentIdx := strings.Index(line, "#"); commentIdx >= 0 {
			line = strings.TrimSpace(line[:commentIdx])
		}
		if line == "" {
			continue
		}

		if inDerivedExpr {
			if derivedExprBuffer != "" {
				derivedExprBuffer += " "
			}
			derivedExprBuffer += line
			tryCloseDerivedExpr(lineIdx)
			continue
		}

		if inDerived {
			if line == "end;" {
				current.Capabilities[currentDerivedLabel] = TLogicalCapability{
					Domain:             currentDerivedDomain,
					IsDerivedCondition: currentDerivedCondition != nil,
					DerivedCondition:   currentDerivedCondition,
					IsDerivedValue:     currentDerivedValue != nil,
					DerivedValue:       currentDerivedValue,
					DerivedDeviceClass: currentDerivedDeviceClass,
					DelayOn:            currentDerivedDelayOn,
					DelayOff:           currentDerivedDelayOff,
				}
				inDerived = false
				continue
			}
			// "condition"/"value" may open either inline ("condition <expr>;", possibly still
			// spanning further lines) or bare on its own line (the user's own preferred layout for
			// a long boolean expression, e.g. "condition\n  (a and b) or\n  c;") -- both forms hand
			// whatever follows to tryCloseDerivedExpr's own accumulation loop identically; only the
			// STARTING buffer differs (empty vs. the same-line remainder).
			if line == "condition" || strings.HasPrefix(line, "condition ") {
				inDerivedExpr = true
				derivedExprKind = "condition"
				derivedExprBuffer = strings.TrimPrefix(line, "condition")
				tryCloseDerivedExpr(lineIdx)
				continue
			}
			if line == "value" || strings.HasPrefix(line, "value ") {
				inDerivedExpr = true
				derivedExprKind = "value"
				derivedExprBuffer = strings.TrimPrefix(line, "value")
				tryCloseDerivedExpr(lineIdx)
				continue
			}
			if matches := logicalDerivedFieldPattern.FindStringSubmatch(line); matches != nil {
				switch matches[1] {
				case "device_class":
					currentDerivedDeviceClass = matches[2]
				case "delay_on":
					currentDerivedDelayOn = matches[2]
				case "delay_off":
					currentDerivedDelayOff = matches[2]
				}
				continue
			}
			warnings = append(warnings, fmt.Sprintf("Logical.def: unrecognised line inside device %q's %q derived capability (body line %d): %q", current.DeviceID, currentDerivedLabel, lineIdx+1, line))
			continue
		}

		if inDefined {
			if line == "end;" {
				current.Capabilities[currentDefinedLabel] = TLogicalCapability{
					Domain:               currentDefinedDomain,
					IsDefinedInputNumber: true,
					DefinedMinimum:       currentDefinedMin,
					DefinedMaximum:       currentDefinedMax,
					DefinedStep:          currentDefinedStep,
					DefinedIcon:          currentDefinedIcon,
					DefinedUnits:         currentDefinedUnits,
				}
				inDefined = false
				continue
			}
			if matches := logicalDefinedFieldPattern.FindStringSubmatch(line); matches != nil {
				switch matches[1] {
				case "minimum":
					currentDefinedMin = matches[2]
				case "maximum":
					currentDefinedMax = matches[2]
				case "step":
					currentDefinedStep = matches[2]
				case "icon":
					currentDefinedIcon = matches[2]
				case "units":
					currentDefinedUnits = matches[2]
				}
				continue
			}
			warnings = append(warnings, fmt.Sprintf("Logical.def: unrecognised line inside device %q's %q defined capability (body line %d): %q", current.DeviceID, currentDefinedLabel, lineIdx+1, line))
			continue
		}

		if inAbsorb {
			if line == "end;" {
				inAbsorb = false
				continue
			}
			if matches := logicalAbsorbBareCapabilityPattern.FindStringSubmatch(line); matches != nil {
				current.Capabilities[matches[2]] = TLogicalCapability{
					Domain:                 matches[1],
					AbsorbedFromDeviceID:   currentAbsorbDeviceID,
					AbsorbedFromCapability: matches[2],
				}
				continue
			}
			if matches := logicalCapabilityPattern.FindStringSubmatch(line); matches != nil {
				current.Capabilities[matches[2]] = TLogicalCapability{
					Domain:                 matches[1],
					AbsorbedFromDeviceID:   currentAbsorbDeviceID,
					AbsorbedFromCapability: bareCapabilityName(matches[3]),
				}
				continue
			}
			warnings = append(warnings, fmt.Sprintf("Logical.def: unrecognised line inside device %q's \"capabilities from %q\" block (body line %d): %q", current.DeviceID, currentAbsorbDeviceID, lineIdx+1, line))
			continue
		}

		if inCapability {
			if line == "end;" {
				current.Capabilities[currentCapLabel] = currentCap
				inCapability = false
				continue
			}
			if matches := logicalCapabilityBodyPattern.FindStringSubmatch(line); matches != nil {
				switch matches[1] {
				case "enabler":
					currentCap.EnablerEntity = matches[2]
				case "delay_off":
					currentCap.DelayOff = matches[2]
				}
				continue
			}
			if matches := logicalNoPlayInputPattern.FindStringSubmatch(line); matches != nil {
				currentCap.NoPlayInput = matches[1]
				continue
			}
			// 2026-09-16: a capability's own "dependency on <device-id>;" (repeatable), scoped to
			// THIS capability alone -- see TLogicalCapability.DependsOn's own doc comment for why
			// this is deliberately separate from the device-level "dependency on" handled below
			// (inDevice branch), which feeds resolveDependencyAvailabilityTopics/MQTT
			// depends_on_availability instead.
			if matches := logicalDependencyOnPattern.FindStringSubmatch(line); matches != nil {
				currentCap.DependsOn = append(currentCap.DependsOn, matches[1])
				continue
			}
			warnings = append(warnings, fmt.Sprintf("Logical.def: unrecognised line inside device %q's %q capability (body line %d): %q", current.DeviceID, currentCapLabel, lineIdx+1, line))
			continue
		}

		if inDevice {
			if line == "end;" {
				devices = append(devices, current)
				inDevice = false
				continue
			}
			if matches := logicalDependencyOnPattern.FindStringSubmatch(line); matches != nil {
				current.DependsOn = append(current.DependsOn, matches[1])
				continue
			}
			if matches := logicalAbsorbBlockPattern.FindStringSubmatch(line); matches != nil {
				currentAbsorbDeviceID = matches[1]
				inAbsorb = true
				continue
			}
			if matches := logicalDefinedBlockPattern.FindStringSubmatch(line); matches != nil {
				currentDefinedDomain = matches[1]
				currentDefinedLabel = matches[2]
				currentDefinedMin, currentDefinedMax, currentDefinedStep, currentDefinedIcon, currentDefinedUnits = "", "", "", "", ""
				inDefined = true
				continue
			}
			if matches := logicalDerivedBlockPattern.FindStringSubmatch(line); matches != nil {
				currentDerivedDomain = matches[1]
				currentDerivedLabel = matches[2]
				currentDerivedCondition, currentDerivedValue = nil, nil
				currentDerivedDeviceClass, currentDerivedDelayOn, currentDerivedDelayOff = "", "", ""
				inDerived = true
				inDerivedExpr = false
				derivedExprBuffer, derivedExprKind = "", ""
				continue
			}
			if matches := logicalCapabilityWithBlockPattern.FindStringSubmatch(line); matches != nil {
				entity, isAvailable := parseLogicalCapabilityValue(matches[3])
				currentCapLabel = matches[2]
				currentCap = TLogicalCapability{Domain: matches[1], Entity: entity, IsAvailable: isAvailable}
				inCapability = true
				continue
			}
			if matches := logicalCapabilityPattern.FindStringSubmatch(line); matches != nil {
				entity, isAvailable := parseLogicalCapabilityValue(matches[3])
				current.Capabilities[matches[2]] = TLogicalCapability{Domain: matches[1], Entity: entity, IsAvailable: isAvailable}
				continue
			}
			warnings = append(warnings, fmt.Sprintf("Logical.def: unrecognised line inside device %q (body line %d): %q", current.DeviceID, lineIdx+1, line))
			continue
		}

		if matches := logicalDevicePattern.FindStringSubmatch(line); matches != nil {
			current = TLogicalDevice{DeviceID: matches[1], Capabilities: map[string]TLogicalCapability{}}
			inDevice = true
			continue
		}

		if matches := logicalDeviceOneLinerPattern.FindStringSubmatch(line); matches != nil {
			deviceID, body := matches[1], strings.TrimSpace(matches[2])+";"
			dev := TLogicalDevice{DeviceID: deviceID, Capabilities: map[string]TLogicalCapability{}}
			switch {
			case logicalDependencyOnPattern.MatchString(body):
				dev.DependsOn = append(dev.DependsOn, logicalDependencyOnPattern.FindStringSubmatch(body)[1])
			case logicalCapabilityPattern.MatchString(body):
				capMatches := logicalCapabilityPattern.FindStringSubmatch(body)
				entity, isAvailable := parseLogicalCapabilityValue(capMatches[3])
				dev.Capabilities[capMatches[2]] = TLogicalCapability{Domain: capMatches[1], Entity: entity, IsAvailable: isAvailable}
			default:
				warnings = append(warnings, fmt.Sprintf("Logical.def: device %q's one-line \"with %s\" body isn't a recognised single-statement form -- only \"dependency on <id>;\" or a bare capability line may be shortened this way (body line %d)", deviceID, body, lineIdx+1))
				continue
			}
			devices = append(devices, dev)
			continue
		}

		warnings = append(warnings, fmt.Sprintf("Logical.def: unrecognised line in \"logical layer\" block (body line %d): %q", lineIdx+1, line))
	}

	return devices, warnings
}

// collectLogicalDevicesByID reads Logical.def's own "logical layer with: ... end;" block, keyed
// by DeviceID -- mirrors collectDiscoveryGatewaysByID/collectHostsDevicesByID's own shape. A
// device declared more than once keeps its first declaration, same warn-and-continue convention
// as every other kind's own collector.
//
// Unlike every other layer, having NO logical layer at all (no Logical.def, or an empty one) is
// the normal, unremarkable default -- most houses have declared nothing here yet (the logical
// layer is being built out on demand, one real candidate at a time, see memory/project_logical_
// layer_introduction.md). collectLayerContent's own "no %q layer ... found" warning is meant for
// layers that are always expected to exist (conceptual/physical); it's suppressed here so a house
// with no Logical.def doesn't get a spurious warning on every generate.
func collectLogicalDevicesByID(definitionDir string, settings map[string]string) (map[string]TLogicalDevice, []string) {
	logicalContent, _, layerWarnings := collectLayerContent(definitionDir, []string{"Logical.def"}, LayerLogical)
	var warnings []string
	for _, w := range layerWarnings {
		if strings.Contains(w, fmt.Sprintf("no %q layer", LayerLogical)) {
			continue
		}
		warnings = append(warnings, w)
	}
	if strings.TrimSpace(logicalContent) == "" {
		return map[string]TLogicalDevice{}, warnings
	}

	// Unlike Conceptual.def (where "${name}" resolution is a handful of targeted call sites,
	// e.g. icon/input_number fields in parser.go, each invoking resolveSettingsVar itself) and
	// Macros.def (where "${name}" is resolved as part of ordinary macro-argument substitution
	// during expansion), Logical.def is never macro-expanded and has no per-field resolution of
	// its own -- so every "${name}" reference (e.g. "windy"'s own minimum/maximum/icon/device_class
	// fields, 2026-09-18) is substituted here, once, over the whole raw body, before parsing. A
	// jinja template call's own "${name}" (e.g. "jinja ${int_less_then}(10)") is a DIFFERENT
	// namespace (parseJinjaTemplateDefinitions, not parseDefinitionAssignments) and is resolved
	// later, inside parseConditionExpr/parseValueExpr itself (logical_condition_expr.go) -- safe to
	// run this pass first regardless, since resolveSettingsVar leaves an unknown "${name}" (never a
	// real Settings.def variable) untouched rather than erroring (its own doc comment).
	logicalContent = substituteSettingsVariables(logicalContent, settings)

	devices, bodyWarnings := parseLogicalLayerBody(strings.Split(logicalContent, "\n"), loadJinjaTemplateDefinitions(definitionDir))
	warnings = append(warnings, bodyWarnings...)

	byID := map[string]TLogicalDevice{}
	for _, d := range devices {
		if _, exists := byID[d.DeviceID]; !exists {
			byID[d.DeviceID] = d
		} else {
			warnings = append(warnings, fmt.Sprintf("Logical.def: device %q declared more than once within the \"logical\" layer; keeping the first declaration", d.DeviceID))
		}
	}
	return byID, warnings
}
