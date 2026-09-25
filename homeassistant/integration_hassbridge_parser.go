/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationHassBridgeParser
 *
 * Parses Physical.def's "integration home_assistant <qualifier> with: device <id> with:
 * <capability>: <entity>; ... end; end;" blocks into THassBridgeDevice records
 * (integration_hassbridge_storage.go). This header shape ("integration home_assistant
 * <qualifier> with:", an extra qualifier token with no "on" keyword) doesn't match
 * Physical_Parser.go's shared integrationHeaderPattern ("integration <name> [on <host>] with:"),
 * so -- unlike "hosts"/"discovery" -- this integration scans Physical.def independently, with its
 * own header pattern, the same way mqtt_discovery_cleanup.go's bare top-level statement does.
 * scanGroupClauseBlocks (defined.go) already handles nested "with:"/"end;" depth-tracking
 * generically regardless of which header pattern found the outer block, so this doesn't collide
 * with or duplicate what parseIntegrationBlocks does for "hosts"/"discovery" over the same text.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 23.08.2026
 *
 */

package main

import (
	"fmt"
	"regexp"
	"strings"
)

var hassBridgeIntegrationHeaderPattern = regexp.MustCompile(`^integration\s+home_assistant\s+(\S+)\s+with:\s*$`)

// parseHassBridgeDeviceHeaderKeywords strips trailing "export"/"roaming"/"import" tokens (any
// order) from a "device <id> [<keywords>] with:" header line, mirroring parseRoutingKeywords'
// own pop-from-the-end approach (mqtt_routing_keywords.go) rather than growing withDevicePattern
// into a combinatorial regex for every keyword-order permutation. "export" and "roaming" are
// mutually sensible alternatives (both select this device's cross-post qualifier -- this
// installation's own real name, or the fixed virtual name "roaming" respectively; declaring both
// is a DSL-author mistake, left for the caller to warn about, not rejected here), "import" is
// independent (same self-import meaning "cloud"+"import" already has for "hosts" devices,
// mqtt_relay.go) -- see THassBridgeDevice.ExportAs/selfImport's own doc comments for the full
// semantics. Returns line unchanged if it doesn't end in "with:".
func parseHassBridgeDeviceHeaderKeywords(line string) (rest string, export, roaming, selfImport bool) {
	if !strings.HasSuffix(line, "with:") {
		return line, false, false, false
	}
	body := strings.TrimSpace(strings.TrimSuffix(line, "with:"))
	tokens := strings.Fields(body)
	for len(tokens) > 0 {
		switch tokens[len(tokens)-1] {
		case "export":
			export = true
			tokens = tokens[:len(tokens)-1]
		case "roaming":
			roaming = true
			tokens = tokens[:len(tokens)-1]
		case "import":
			selfImport = true
			tokens = tokens[:len(tokens)-1]
		default:
			return strings.Join(tokens, " ") + " with:", export, roaming, selfImport
		}
	}
	return strings.Join(tokens, " ") + " with:", export, roaming, selfImport
}

// collectHassBridgeDevicesByID scans Physical.def for every "integration home_assistant
// <qualifier> with: ... end;" block, parses its device declarations, and warns if a referenced
// qualifier has no matching "home_assistant <qualifier>: <name>;" instance declaration
// (collectHomeAssistantInstances, defined.go). Resolves each device's own selfImport flag
// (parseHassBridgeDeviceHeaderKeywords' "import" keyword) into SelfImportFrom set to this
// installation's own resolved name, mirroring generateHassBridgeFile's own resolution -- see
// THassBridgeDevice.SelfImportFrom's doc comment for why this has to happen here rather than
// inside the lower-level parser.
func collectHassBridgeDevicesByID(definitionDir string) (map[string]THassBridgeDevice, []string) {
	physicalContent, mergedLineNos, warnings := collectLayerContent(definitionDir, []string{"Physical.def"}, LayerPhysical)
	physicalContent = resolveDerivedConditionOneLiners(physicalContent)
	var jinjaWarnings []string
	physicalContent, jinjaWarnings = resolveJinjaTemplateCallsInDerivedLines(physicalContent, loadJinjaTemplateDefinitions(definitionDir))
	warnings = append(warnings, jinjaWarnings...)
	blocks, blockWarnings := scanGroupClauseBlocks(splitLines(physicalContent), "Physical.def", hassBridgeIntegrationHeaderPattern)
	warnings = append(warnings, blockWarnings...)

	instances := collectHomeAssistantInstances(physicalContent)
	installation := resolveInstallationName(definitionDir)

	byID := map[string]THassBridgeDevice{}
	for _, block := range blocks {
		instanceQualifier := block.HeaderMatch[1]
		startLine := translateContentLineNo(mergedLineNos, block.StartLine)
		if _, known := instances[instanceQualifier]; !known {
			warnings = append(warnings, fmt.Sprintf("Physical.def:%d: \"integration home_assistant %s with:\" references instance %q, which has no matching \"home_assistant %s: ...;\" declaration", startLine, instanceQualifier, instanceQualifier, instanceQualifier))
		}
		devices, bodyWarnings := parseHassBridgeIntegrationBody(block.BodyLines, instanceQualifier)
		warnings = append(warnings, bodyWarnings...)
		for _, d := range devices {
			if d.selfImport {
				if installation == "" {
					warnings = append(warnings, fmt.Sprintf("device %q: \"import\" declared but ${installation} is not set; cannot self-qualify -- ignored", d.DeviceID))
				} else {
					d.SelfImportFrom = installation
				}
			}
			existing, exists := byID[d.DeviceID]
			if !exists {
				byID[d.DeviceID] = d
				continue
			}
			// A duplicate DeviceID across two "integration home_assistant <qualifier> with:"
			// blocks in the same house is almost always a Physical.def authoring mistake
			// (first-wins, warned) -- EXCEPT for a "roaming" device (ExportAs "roaming"), which is
			// deliberately declared once per real local instance it might be actively connected to
			// (e.g. an HA companion-app phone reachable via either of two local HA instances): each
			// such declaration contributes its own Instance to the SAME merged record, since each
			// real instance still needs its own reporting automation generated
			// (generateHassBridgeEntityReportingAutomations), even though they all publish under
			// the one shared "roaming" identity rather than their own real instance names.
			if existing.ExportAs == "roaming" && d.ExportAs == "roaming" {
				merged := existing
				if !containsString(merged.Instances, instanceQualifier) {
					merged.Instances = append(merged.Instances, instanceQualifier)
				}
				// Union each capability's per-instance Sources rather than discarding d's own
				// declaration wholesale -- real bug, found live 2026-09-05: a naive "keep
				// existing, drop d" merge here silently threw away every instance's own source
				// past the first, so every instance's reporting automation was generated against
				// whichever declaration happened to be seen first, even when a later instance's
				// local entity_id for the same capability genuinely differed (confirmed live:
				// Vienna's own HA companion-app registration for hass.eriks_iphone did not, at
				// first, use the identical entity_id Junglinster's instances did). Typing metadata
				// (Domain/Unit/Icon/DeviceClass/StateClass) is assumed identical across instances
				// -- it describes the one shared local/coordinator-side entity, not the remote
				// source -- so the first declaration's copy of it wins; only Sources is unioned.
				if merged.Capabilities == nil {
					merged.Capabilities = map[string]THassBridgeCapability{}
				}
				for capName, dCap := range d.Capabilities {
					mergedCap, known := merged.Capabilities[capName]
					if !known {
						merged.Capabilities[capName] = dCap
						continue
					}
					if mergedCap.Sources == nil {
						mergedCap.Sources = map[string]string{}
					}
					for instance, source := range dCap.Sources {
						mergedCap.Sources[instance] = source
					}
					merged.Capabilities[capName] = mergedCap
				}
				byID[d.DeviceID] = merged
				continue
			}
			warnings = append(warnings, fmt.Sprintf("Physical.def: device %q declared more than once across \"integration home_assistant\" blocks; keeping the first declaration (instance %q) -- if it should roam across instances, declare \"roaming\" consistently on every one", d.DeviceID, existing.Instances))
		}
	}
	warnings = append(warnings, warnAboutOrphanedHassBridgeDeviceBlocks(physicalContent, mergedLineNos, byID)...)
	// Runs here, once, rather than at each of this function's own several call sites (real bug
	// found live 2026-09-25: an earlier version only ran it at generator.go's own call site, right
	// before validateHassBridgeAvailabilitySources -- Physical_Generator.go's own SEPARATE call
	// into this same function, which is what actually builds homeassistant_bridge.yaml, re-parses
	// Physical.def completely fresh and never saw that mutation, so the validator's own warning
	// went away while the generated YAML still carried the unresolved bare label) -- see
	// resolveHassBridgeAvailabilityLabelReferences's own doc comment for what this rewrites.
	resolveHassBridgeAvailabilityLabelReferences(byID)
	return byID, warnings
}

// containsString reports whether want appears anywhere in list.
func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

var strayHassBridgeDevicePattern = regexp.MustCompile(`^device\s+(hass\.\S+)(?:\s+(?:export|roaming|import)){0,2}\s+with:\s*$`)

// warnAboutOrphanedHassBridgeDeviceBlocks finds every "device hass.<id> with:" header line
// anywhere in physicalContent whose id never made it into byID -- almost always because the block
// sits outside any "integration home_assistant <qualifier> with:" block, which is the only place
// this parser ever looks for device declarations; anywhere else, the whole block is silently
// dropped with no feedback otherwise. Confirmed live 2026-08-28: a device pasted right after an
// integration block's closing "end;" instead of before it vanished from hassBridgeDevicesByID
// entirely -- no warning, no error, just missing capability positioning and a suggestion report
// that never stopped suggesting its already-declared entities. A device id that *is* in byID
// (declared correctly, however many times its header text happens to appear -- e.g. inside its own
// already-parsed block) is never warned about; this only ever flags an id that never resolved.
func warnAboutOrphanedHassBridgeDeviceBlocks(physicalContent string, mergedLineNos []int, byID map[string]THassBridgeDevice) []string {
	var warnings []string
	for i, line := range splitLines(physicalContent) {
		matches := strayHassBridgeDevicePattern.FindStringSubmatch(strings.TrimSpace(line))
		if matches == nil {
			continue
		}
		if _, found := byID[matches[1]]; found {
			continue
		}
		lineNo := translateContentLineNo(mergedLineNos, i+1)
		warnings = append(warnings, fmt.Sprintf("Physical.def:%d: \"device %s with:\" isn't nested inside any \"integration home_assistant <qualifier> with:\" block, so it's being silently ignored -- move it inside one", lineNo, matches[1]))
	}
	return warnings
}

// parseHassBridgeIntegrationBody parses one "integration home_assistant <qualifier> with:"
// block's body lines into device declarations. Lines that don't parse cleanly are reported as
// warnings rather than aborting the parse.
func parseHassBridgeIntegrationBody(bodyLines []string, instance string) ([]THassBridgeDevice, []string) {
	var devices []THassBridgeDevice
	var warnings []string

	// Trailing keywords ("export"/"roaming"/"import") are stripped by parseHassBridgeDeviceHeaderKeywords
	// before this pattern ever sees the line -- see that function's own doc comment.
	withDevicePattern := regexp.MustCompile(`^device\s+(\S+)\s+with:\s*$`)
	// Domain is mandatory and explicit ("<entity_type>.<path>: <source>;") -- no group-prefix,
	// unlike hosts capabilities: every capability is a variable attribute for this device kind
	// (see integration_hassbridge_storage.go's doc comment), and no implicit "always sensor"
	// default either (superseded design decision -- see the plan's §D). Source is captured
	// greedily up to the trailing ";" (not \S+) so it can carry a multi-word "<entity> is
	// available" suffix (sourceToJinja2's sugar for the standard liveness-check boilerplate),
	// not just a single bare entity reference.
	// "<path>" and "<source>" are whitespace-separated, no colon between them (2026-09-19 -- a
	// colon-optional trial ran first, then both houses' Physical.def/Logical.def were rewritten
	// to the colon-less form and verified byte-identical on regenerate, so the colon alternative
	// was dropped here outright) -- see logicalCapabilityPattern's own identical change for the
	// full rationale.
	capabilityPattern := regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_/]*)\s+(.+?)\s*;\s*$`)
	// Same capability line, but opening a nested "with: ... end;" block of its own typing
	// metadata instead of terminating with ";" -- an alternative to the bare
	// "<path> <keyword>: ...;" trailing-line form (capabilityMetadataPattern) that keeps a
	// capability's overrides grouped with its declaration instead of repeating the path prefix
	// once per field. Checked before capabilityPattern since the two are mutually exclusive by
	// trailing token ("with:" vs ";") but both start the same way. Same colon-dropped grammar as
	// capabilityPattern just above.
	capabilityWithPattern := regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_/]*)\s+(.+?)\s*with:\s*$`)
	// Single-statement shorthand for capabilityWithPattern's own block form -- "<domain>.<path>
	// <source> with <single-statement>;" instead of "<domain>.<path> <source> with:
	// <single-statement>; end;" -- mirrors logicalDeviceOneLinerPattern's identical reasoning
	// (2026-09-25, the user's own "if there is only one line in the with clause, fold it"
	// instruction, generalised from Logical.def's device header to every "with:" block in this
	// DSL -- cover.cover's own "derived value ...;" position declaration was the concrete
	// trigger). Source is a bare token (\S+), matching every real "with"-modified capability's
	// source in both houses' Physical.def today (unlike capabilityPattern's own free-text group,
	// which also has to carry the multi-word "<entity> is available" suffix -- a capability with
	// that suffix is never itself further "with"-modified in this codebase). Checked before
	// capabilityPattern for the same reason capabilityWithPattern already is: a generic free-text
	// match would otherwise swallow the whole "<source> with <statement>" as its own Source,
	// corrupting it exactly like the earlier "position:" domain-qualified parsing bug this session
	// already hit once (see capabilityWithPositionPattern's own doc comment).
	capabilityWithOneLinerPattern := regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_/]*)\s+(\S+)\s+with\s+(.+?)\s*;\s*$`)
	// A bare typing-metadata line inside a capability's own "with: ... end;" block -- same four
	// keywords as capabilityMetadataPattern, just without the leading path (implied by nesting).
	capabilityWithMetadataPattern := regexp.MustCompile(`^(unit|icon|device_class|state_class):\s*"([^"]*)"\s*;\s*$`)
	// "map: "<source-value>" "<target-value>";" -- repeatable (unlike the single-valued keywords
	// above), inside a capability's own "with: ... end;" block. Translates the source's own raw
	// reported value before it's published -- see THassBridgeCapability.ValueMap's own doc
	// comment for why (Roomba's "home"/"run"/... vs. HA's "docked"/"cleaning"/...).
	capabilityWithValueMapPattern := regexp.MustCompile(`^map:\s*"([^"]*)"\s+"([^"]*)"\s*;\s*$`)
	// Bare (non-domain-prefixed) metadata lines -- mirrors integration_hosts_parser.go's own
	// two patterns exactly, tried in the same order (constant-attribute first: a quoted value
	// with nothing else after it but optional "forced" is unambiguous only if checked before the
	// looser device-info pattern, which would otherwise also match it).
	constantAttributePattern := regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*):\s*"([^"]*)"(?:\s+(forced))?\s*;\s*$`)
	deviceInfoCapabilityPattern := regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*):\s*(?:"([^"]*)"\s+)?(\S+)\s*;\s*$`)
	// Per-capability typing metadata: "<path> unit|icon|device_class|state_class: "<value>";" --
	// a two-token LHS (path, then the metadata keyword), so it can't collide with
	// deviceInfoCapabilityPattern's single-token-then-colon shape. Targets an already-declared
	// capability by its bare path (the same key Capabilities is keyed by, no domain prefix
	// needed since the capability line already established that).
	capabilityMetadataPattern := regexp.MustCompile(`^(\S+)\s+(unit|icon|device_class|state_class):\s*"([^"]*)"\s*;\s*$`)
	// Standalone-trailing-line sibling of capabilityWithValueMapPattern -- "<path> map: "<from>"
	// "<to>";", targeting an already-declared capability by its bare path, same shape
	// capabilityMetadataPattern uses for the single-valued keywords.
	capabilityValueMapPattern := regexp.MustCompile(`^(\S+)\s+map:\s*"([^"]*)"\s+"([^"]*)"\s*;\s*$`)
	// "attribute <name>: <source>;" -- repeatable, inside a capability's own "with: ... end;"
	// block. Declares an extra named attribute (source is the same "<entity>[!<attribute>]"
	// convention Sources uses) to expose on this capability's discovery config via
	// json_attributes_topic, alongside its own state -- see THassBridgeCapability.Attributes' own
	// doc comment (2026-09-21, the vacuum "stuck near a cliff" incident: the bridge only ever
	// relayed vacuum.roomba's bare state, never its "error"/"error_code" attributes). Source is
	// UNQUOTED (2026-09-21, fixed same day it was first added) -- it's an entity reference, the
	// same convention Sources/over:-entries use everywhere else in this DSL (unquoted), not an
	// arbitrary translation value the way map:'s own quoted "<from>" "<to>" strings are.
	capabilityWithAttributePattern := regexp.MustCompile(`^attribute\s+([A-Za-z_][A-Za-z0-9_]*):\s*(\S+)\s*;\s*$`)
	// Standalone-trailing-line sibling of capabilityWithAttributePattern -- "<path> attribute
	// <name>: <source>;", same shape capabilityValueMapPattern uses.
	capabilityAttributePattern := regexp.MustCompile(`^(\S+)\s+attribute\s+([A-Za-z_][A-Za-z0-9_]*):\s*(\S+)\s*;\s*$`)
	// "map <name>: "<from>" "<to>";" -- the ATTRIBUTE-scoped sibling of the bare (state-only)
	// map:/capabilityValueMapPattern above, disambiguated purely by the mandatory identifier
	// between "map" and ":" (bare "map:" has no token there at all, so the two can never collide).
	// Translates one declared attribute's own raw reported value, independently of whatever
	// bare map: does for this capability's own state -- see THassBridgeCapability.AttributeValueMaps.
	capabilityWithAttributeValueMapPattern := regexp.MustCompile(`^map\s+([A-Za-z_][A-Za-z0-9_]*):\s*"([^"]*)"\s+"([^"]*)"\s*;\s*$`)
	// Standalone-trailing-line sibling of capabilityWithAttributeValueMapPattern.
	capabilityAttributeValueMapPattern := regexp.MustCompile(`^(\S+)\s+map\s+([A-Za-z_][A-Za-z0-9_]*):\s*"([^"]*)"\s+"([^"]*)"\s*;\s*$`)
	// "derived value <source>;" -- inside a capability's own "with: ... end;" block, same source
	// convention as attribute/Sources -- see THassBridgeCapability.Position's own doc comment
	// (2026-09-25, cover's own current-position feature; keyword changed from the original bare
	// "position:" the same day, to echo Logical.def's own "derived <domain>.<label> with: value
	// <expr>; end;" vocabulary -- the user's explicit request, "follow the syntax used on the
	// logical level." NOT the same mechanism as Physical_DerivedCapability.go's entity-creating
	// "derived DDD.NNN from EEE.MMM via TTT;" line, deliberately: that one spawns a brand new
	// sibling capability/entity, which would break the cover's own draggable-slider requirement
	// (HA's MQTT cover schema needs position_topic on the SAME entity as state_topic, not a
	// separate one) -- so this stays a per-capability modifier, just reusing "derived"/"value" as
	// vocabulary, not the capability-creating grammar itself. No colon after "value" -- caught by
	// the user same day: Logical.def's own "value <expr>;" keyword inside a "derived ... with:"
	// block has no colon either (2026-09-19's "capability colon dropped from grammar" convention),
	// so this needed to match that, not capabilityWithMetadataPattern's own "<keyword>: <value>;"
	// shape. No name token, unlike "attribute" (there's only ever one position per capability).
	capabilityWithPositionPattern := regexp.MustCompile(`^derived\s+value\s+(\S+)\s*;\s*$`)
	// Standalone-trailing-line sibling of capabilityWithPositionPattern -- "<path> derived value <source>;".
	capabilityPositionPattern := regexp.MustCompile(`^(\S+)\s+derived\s+value\s+(\S+)\s*;\s*$`)

	// applyCapabilityWithBodyLine applies ONE body line from a capability's own "with: ... end;"
	// block to cap, returning ok=false for anything it doesn't recognise. Shared by both the
	// multi-line block form (pendingCapabilityPath loop below) and capabilityWithOneLinerPattern's
	// folded one-line equivalent, so the two stay in lock-step by construction -- a new modifier
	// keyword added to one automatically works in the other.
	applyCapabilityWithBodyLine := func(cap THassBridgeCapability, line string) (THassBridgeCapability, bool) {
		if matches := capabilityWithMetadataPattern.FindStringSubmatch(line); matches != nil {
			switch matches[1] {
			case "unit":
				cap.Unit = matches[2]
			case "icon":
				cap.Icon = matches[2]
			case "device_class":
				cap.DeviceClass = matches[2]
			case "state_class":
				cap.StateClass = matches[2]
			}
			return cap, true
		}
		if matches := capabilityWithValueMapPattern.FindStringSubmatch(line); matches != nil {
			if cap.ValueMap == nil {
				cap.ValueMap = map[string]string{}
			}
			cap.ValueMap[matches[1]] = matches[2]
			return cap, true
		}
		if matches := capabilityWithAttributePattern.FindStringSubmatch(line); matches != nil {
			if cap.Attributes == nil {
				cap.Attributes = map[string]map[string]string{}
			}
			if cap.Attributes[matches[1]] == nil {
				cap.Attributes[matches[1]] = map[string]string{}
			}
			cap.Attributes[matches[1]][instance] = matches[2]
			return cap, true
		}
		if matches := capabilityWithAttributeValueMapPattern.FindStringSubmatch(line); matches != nil {
			if cap.AttributeValueMaps == nil {
				cap.AttributeValueMaps = map[string]map[string]string{}
			}
			if cap.AttributeValueMaps[matches[1]] == nil {
				cap.AttributeValueMaps[matches[1]] = map[string]string{}
			}
			cap.AttributeValueMaps[matches[1]][matches[2]] = matches[3]
			return cap, true
		}
		if matches := capabilityWithPositionPattern.FindStringSubmatch(line); matches != nil {
			if cap.Position == nil {
				cap.Position = map[string]string{}
			}
			cap.Position[instance] = matches[1]
			return cap, true
		}
		return cap, false
	}

	inDeviceCapabilities := false
	var current THassBridgeDevice
	pendingCapabilityPath := "" // "" when not inside a capability's own "with: ... end;" block

	for lineIdx, rawLine := range bodyLines {
		line := strings.TrimSpace(rawLine)
		if commentIdx := strings.Index(line, "#"); commentIdx >= 0 {
			line = strings.TrimSpace(line[:commentIdx])
		}
		if line == "" {
			continue
		}

		if inDeviceCapabilities && pendingCapabilityPath != "" {
			if line == "end;" {
				pendingCapabilityPath = ""
				continue
			}
			if cap, ok := applyCapabilityWithBodyLine(current.Capabilities[pendingCapabilityPath], line); ok {
				current.Capabilities[pendingCapabilityPath] = cap
				continue
			}
			warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line inside capability %q's \"with:\" block (body line %d): %q", pendingCapabilityPath, lineIdx+1, line))
			continue
		}

		if inDeviceCapabilities {
			if line == "end;" {
				devices = append(devices, current)
				inDeviceCapabilities = false
				continue
			}
			if line == "ignore other capabilities;" {
				current.IgnoreOtherCapabilities = true
				continue
			}
			// Checked first, before any domain-prefixed capability pattern: "derived" is a
			// distinct leading keyword, unambiguous against "<domain>.<path>: ...;" -- see
			// Physical_DerivedCapability.go's own header comment.
			if decl, ok := parseDerivedCapabilityLine(line); ok {
				current.Capabilities[decl.Label] = THassBridgeCapability{
					Domain:                decl.Domain,
					DerivedFromCapability: decl.FromLabel,
					DerivedViaTemplate:    decl.Template,
				}
				continue
			}
			if matches := capabilityWithPattern.FindStringSubmatch(line); matches != nil {
				current.Capabilities[matches[2]] = THassBridgeCapability{Domain: matches[1], Sources: map[string]string{instance: matches[3]}}
				pendingCapabilityPath = matches[2]
				continue
			}
			if matches := capabilityWithOneLinerPattern.FindStringSubmatch(line); matches != nil {
				cap := THassBridgeCapability{Domain: matches[1], Sources: map[string]string{instance: matches[3]}}
				body := strings.TrimSpace(matches[4]) + ";"
				if applied, ok := applyCapabilityWithBodyLine(cap, body); ok {
					current.Capabilities[matches[2]] = applied
					continue
				}
				warnings = append(warnings, fmt.Sprintf("Physical.def: capability %q's one-line \"with %s\" body isn't a recognised single-statement form (body line %d)", matches[2], body, lineIdx+1))
				continue
			}
			if matches := capabilityPattern.FindStringSubmatch(line); matches != nil {
				current.Capabilities[matches[2]] = THassBridgeCapability{Domain: matches[1], Sources: map[string]string{instance: matches[3]}}
				continue
			}
			if matches := constantAttributePattern.FindStringSubmatch(line); matches != nil {
				if !knownConstantDeviceAttributes[matches[1]] {
					warnings = append(warnings, fmt.Sprintf("Physical.def: device %q: unrecognised constant device attribute %q; ignored", current.DeviceID, matches[1]))
					continue
				}
				current.ConstantAttributes[matches[1]] = THostConstantAttribute{Value: matches[2], Forced: matches[3] == "forced"}
				continue
			}
			if matches := capabilityMetadataPattern.FindStringSubmatch(line); matches != nil {
				path, key, value := matches[1], matches[2], matches[3]
				cap, known := current.Capabilities[path]
				if !known {
					warnings = append(warnings, fmt.Sprintf("Physical.def: device %q: %q metadata declared for unknown capability %q; ignored", current.DeviceID, key, path))
					continue
				}
				switch key {
				case "unit":
					cap.Unit = value
				case "icon":
					cap.Icon = value
				case "device_class":
					cap.DeviceClass = value
				case "state_class":
					cap.StateClass = value
				}
				current.Capabilities[path] = cap
				continue
			}
			if matches := capabilityValueMapPattern.FindStringSubmatch(line); matches != nil {
				path, from, to := matches[1], matches[2], matches[3]
				cap, known := current.Capabilities[path]
				if !known {
					warnings = append(warnings, fmt.Sprintf("Physical.def: device %q: \"map\" declared for unknown capability %q; ignored", current.DeviceID, path))
					continue
				}
				if cap.ValueMap == nil {
					cap.ValueMap = map[string]string{}
				}
				cap.ValueMap[from] = to
				current.Capabilities[path] = cap
				continue
			}
			if matches := capabilityAttributePattern.FindStringSubmatch(line); matches != nil {
				path, name, source := matches[1], matches[2], matches[3]
				cap, known := current.Capabilities[path]
				if !known {
					warnings = append(warnings, fmt.Sprintf("Physical.def: device %q: \"attribute\" declared for unknown capability %q; ignored", current.DeviceID, path))
					continue
				}
				if cap.Attributes == nil {
					cap.Attributes = map[string]map[string]string{}
				}
				if cap.Attributes[name] == nil {
					cap.Attributes[name] = map[string]string{}
				}
				cap.Attributes[name][instance] = source
				current.Capabilities[path] = cap
				continue
			}
			if matches := capabilityAttributeValueMapPattern.FindStringSubmatch(line); matches != nil {
				path, name, from, to := matches[1], matches[2], matches[3], matches[4]
				cap, known := current.Capabilities[path]
				if !known {
					warnings = append(warnings, fmt.Sprintf("Physical.def: device %q: \"map %s\" declared for unknown capability %q; ignored", current.DeviceID, name, path))
					continue
				}
				if cap.AttributeValueMaps == nil {
					cap.AttributeValueMaps = map[string]map[string]string{}
				}
				if cap.AttributeValueMaps[name] == nil {
					cap.AttributeValueMaps[name] = map[string]string{}
				}
				cap.AttributeValueMaps[name][from] = to
				current.Capabilities[path] = cap
				continue
			}
			if matches := capabilityPositionPattern.FindStringSubmatch(line); matches != nil {
				path, source := matches[1], matches[2]
				cap, known := current.Capabilities[path]
				if !known {
					warnings = append(warnings, fmt.Sprintf("Physical.def: device %q: \"position\" declared for unknown capability %q; ignored", current.DeviceID, path))
					continue
				}
				if cap.Position == nil {
					cap.Position = map[string]string{}
				}
				cap.Position[instance] = source
				current.Capabilities[path] = cap
				continue
			}
			if matches := deviceInfoCapabilityPattern.FindStringSubmatch(line); matches != nil {
				if !knownConstantDeviceAttributes[matches[1]] {
					warnings = append(warnings, fmt.Sprintf("Physical.def: device %q: unrecognised device attribute %q; ignored", current.DeviceID, matches[1]))
					continue
				}
				current.DeviceInfoCapabilities[matches[1]] = matches[3]
				if matches[2] != "" {
					current.DeviceInfoLiteralPrefixes[matches[1]] = matches[2]
				}
				continue
			}
			warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line inside device %q capability block (body line %d): %q", current.DeviceID, lineIdx+1, line))
			continue
		}

		strippedLine, export, roaming, selfImport := parseHassBridgeDeviceHeaderKeywords(line)
		if matches := withDevicePattern.FindStringSubmatch(strippedLine); matches != nil {
			exportAs := ""
			if roaming {
				exportAs = "roaming"
			}
			if export && roaming {
				warnings = append(warnings, fmt.Sprintf("Physical.def: device %q: both \"export\" and \"roaming\" declared; \"roaming\" wins", matches[1]))
			}
			current = THassBridgeDevice{
				DeviceID: matches[1], Instances: []string{instance},
				Capabilities:              map[string]THassBridgeCapability{},
				ConstantAttributes:        map[string]THostConstantAttribute{},
				DeviceInfoCapabilities:    map[string]string{},
				DeviceInfoLiteralPrefixes: map[string]string{},
				Export:                    export || roaming,
				ExportAs:                  exportAs,
				selfImport:                selfImport,
			}
			inDeviceCapabilities = true
			continue
		}

		warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line in \"integration home_assistant %s\" block (body line %d): %q", instance, lineIdx+1, line))
	}

	return devices, warnings
}
