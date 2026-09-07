/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: CapabilityDefaults
 *
 * Parses Physical.def's "defaults: for <domain>.<pattern>: <unit|icon|device_class|
 * state_class>: "<value>"; ... end; ... end;" block: user-declared default typing metadata for
 * capabilities matched by domain and path pattern, so a device declaration doesn't need to
 * repeat "node device_class: ...;"/"cpu/temperature unit: ...;" style lines for shapes that
 * recur across devices. A capability's own explicit metadata -- whether the per-capability
 * "with: ... end;" clause or the older bare "<path> <keyword>: ...;" trailing line
 * (integration_hassbridge_parser.go) -- always takes precedence; these rules only fill fields a
 * capability leaves unset (see capabilityDefaultsFor, consulted from
 * registerHassBridgeAttributeEntity in Conceptual_DeviceEntities.go).
 *
 * PatternPath may be a literal path ("node", "cpu/load") or carry a leading wildcard segment
 * (star, slash) that absorbs zero or more of the capability's own leading path segments,
 * matching by suffix (e.g. a "temperature" suffix pattern matches both "temperature" and
 * "cpu/temperature") -- see matchesCapabilityPattern.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 25.08.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// TCapabilityDefaultRule is one "for <domain>.<pattern>: ...; end;" rule from a "defaults: ...
// end;" block.
type TCapabilityDefaultRule struct {
	Domain      string
	PatternPath string
	DeviceClass string
	Unit        string
	StateClass  string
	Icon        string
}

var capabilityDefaultsHeaderPattern = regexp.MustCompile(`^defaults:\s*$`)

// A "for" line names one or more space-separated "<domain>.<pattern>" targets sharing the same
// body, e.g. "for sensor.*/total_power sensor.*/current_power:" -- captured whole here, then
// split/validated per-target by capabilityDefaultsTargetPattern below.
var capabilityDefaultsForPattern = regexp.MustCompile(`^for\s+(.+):\s*$`)
var capabilityDefaultsTargetPattern = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\.(\S+)$`)
var capabilityDefaultsMetadataPattern = regexp.MustCompile(`^(unit|icon|device_class|state_class):\s*"([^"]*)"\s*;\s*$`)

// collectCapabilityDefaults scans for every "defaults: ... end;" block and parses their
// "for <domain>.<pattern>: ...; end;" rules -- both sharedDefinitionDir's own Defaults.def
// (a flat, wrapper-less file, same convention as the shared Settings.def: its whole content is
// implicitly rules, no "physical layer with:" wrapper needed) and, for a house that wants
// local additions/overrides, any "defaults: ... end;" block still declared inside its own
// Physical.def. There is normally only one block per source, but nothing requires that.
func collectCapabilityDefaults(definitionDir, sharedDefinitionDir string) ([]TCapabilityDefaultRule, []string) {
	var rules []TCapabilityDefaultRule
	var warnings []string

	collect := func(content, sourceFile string) {
		blocks, blockWarnings := scanGroupClauseBlocks(splitLines(content), sourceFile, capabilityDefaultsHeaderPattern)
		warnings = append(warnings, blockWarnings...)
		for _, block := range blocks {
			blockRules, bodyWarnings := parseCapabilityDefaultsBody(block.BodyLines)
			rules = append(rules, blockRules...)
			warnings = append(warnings, bodyWarnings...)
		}
	}

	if sharedContent, err := readOptionalFile(filepath.Join(sharedDefinitionDir, "Defaults.def")); err == nil {
		collect(sharedContent, "Defaults.def")
	}

	physicalContent, _, physicalWarnings := collectLayerContent(definitionDir, []string{"Physical.def"}, LayerPhysical)
	warnings = append(warnings, physicalWarnings...)
	collect(physicalContent, "Physical.def")

	return rules, warnings
}

// parseCapabilityDefaultsBody parses one "defaults: ... end;" block's body lines into rules.
// Lines that don't parse cleanly are reported as warnings rather than aborting the parse.
func parseCapabilityDefaultsBody(bodyLines []string) ([]TCapabilityDefaultRule, []string) {
	var rules []TCapabilityDefaultRule
	var warnings []string

	type target struct{ Domain, PatternPath string }

	inFor := false
	var currentTargets []target
	var currentMetadata TCapabilityDefaultRule
	var currentLabel string // for warning messages, e.g. "sensor.*/total_power, sensor.*/current_power"

	for _, rawLine := range bodyLines {
		line := strings.TrimSpace(rawLine)
		if commentIdx := strings.Index(line, "#"); commentIdx >= 0 {
			line = strings.TrimSpace(line[:commentIdx])
		}
		if line == "" {
			continue
		}

		if inFor {
			if line == "end;" {
				for _, t := range currentTargets {
					rules = append(rules, TCapabilityDefaultRule{
						Domain: t.Domain, PatternPath: t.PatternPath,
						DeviceClass: currentMetadata.DeviceClass, Unit: currentMetadata.Unit,
						StateClass: currentMetadata.StateClass, Icon: currentMetadata.Icon,
					})
				}
				inFor = false
				continue
			}
			if matches := capabilityDefaultsMetadataPattern.FindStringSubmatch(line); matches != nil {
				switch matches[1] {
				case "unit":
					currentMetadata.Unit = matches[2]
				case "icon":
					currentMetadata.Icon = matches[2]
				case "device_class":
					currentMetadata.DeviceClass = matches[2]
				case "state_class":
					currentMetadata.StateClass = matches[2]
				}
				continue
			}
			warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line inside \"for %s\" defaults rule: %q", currentLabel, line))
			continue
		}

		if matches := capabilityDefaultsForPattern.FindStringSubmatch(line); matches != nil {
			currentTargets = nil
			currentMetadata = TCapabilityDefaultRule{}
			currentLabel = matches[1]
			for _, token := range strings.Fields(matches[1]) {
				targetMatches := capabilityDefaultsTargetPattern.FindStringSubmatch(token)
				if targetMatches == nil {
					warnings = append(warnings, fmt.Sprintf("Physical.def: \"for %s\": %q is not a valid \"<domain>.<pattern>\" target; skipped", currentLabel, token))
					continue
				}
				currentTargets = append(currentTargets, target{Domain: targetMatches[1], PatternPath: targetMatches[2]})
			}
			inFor = true
			continue
		}

		warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line inside \"defaults:\" block: %q", line))
	}

	return rules, warnings
}

// matchesCapabilityPattern reports whether a capability's own domain/path is matched by a
// defaults rule. domain must match ruleDomain exactly. patternPath "*" matches any path; a
// leading "*/" absorbs zero or more of path's own leading segments, matching everything after it
// as a required suffix; with no such prefix, path must equal patternPath exactly.
func matchesCapabilityPattern(domain, path, ruleDomain, patternPath string) bool {
	if domain != ruleDomain {
		return false
	}
	if patternPath == "*" {
		return true
	}
	suffix, wildcard := strings.CutPrefix(patternPath, "*/")
	if !wildcard {
		return path == patternPath
	}
	return path == suffix || strings.HasSuffix(path, "/"+suffix)
}

// capabilityDefaultsFor returns the device_class/unit/state_class/icon the given rules seed for
// a capability, first match per field wins. Called only to fill a field the capability itself
// (and, below that, the code-level postfix tables -- postfixCapabilityDefaults) leaves unset.
func capabilityDefaultsFor(rules []TCapabilityDefaultRule, domain, path string) (deviceClass, unit, stateClass, icon string) {
	for _, rule := range rules {
		if !matchesCapabilityPattern(domain, path, rule.Domain, rule.PatternPath) {
			continue
		}
		if deviceClass == "" {
			deviceClass = rule.DeviceClass
		}
		if unit == "" {
			unit = rule.Unit
		}
		if stateClass == "" {
			stateClass = rule.StateClass
		}
		if icon == "" {
			icon = rule.Icon
		}
	}
	return
}

// resolveCapabilityDefaults returns the device_class/unit/state_class/icon for a domain/path,
// preferring a matching "defaults: for ...;" rule (capabilityDefaultsFor -- user-declared, via
// Defaults.def or a house's own Physical.def) over the code-level postfix tables
// (postfixCapabilityDefaults). This is the one shared entry point every generator path that
// seeds typing metadata from an entity's postfix should call, rather than indexing the
// code-level tables directly, so Defaults.def is a single, consistent override point site-wide.
func resolveCapabilityDefaults(rules []TCapabilityDefaultRule, domain, path string) (deviceClass, unit, stateClass, icon string) {
	deviceClass, unit, stateClass, icon = capabilityDefaultsFor(rules, domain, path)
	fallbackDeviceClass, fallbackUnit, fallbackStateClass, fallbackIcon := postfixCapabilityDefaults(domain, path)
	if deviceClass == "" {
		deviceClass = fallbackDeviceClass
	}
	if unit == "" {
		unit = fallbackUnit
	}
	if stateClass == "" {
		stateClass = fallbackStateClass
	}
	if icon == "" {
		icon = fallbackIcon
	}
	return
}
