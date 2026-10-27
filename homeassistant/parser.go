/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Parser
 *
 * ParseEntitiesAndFillAdministration parses entity lines, applies strict macro validation,
 * performs aggressive macro expansion, and records open/close entity/space events into
 * administration -- the semantic analysis path used by the real generation pipeline.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 25.03.2026
 *
 */

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// EndToken/StatementEndToken: the literal "end;" statement terminator, checked when
// scanning a nested block's lines for its close.
const (
	EndToken          = "end"
	StatementEndToken = ";"
)

// --- semantic types and processing (unchanged) ---

type TExpansionParseResult struct {
	Administration   *TAdministrationState
	InvocationCount  int
	ValidInvocations int
	TypeErrors       int
}

// ParseEntitiesAndFillAdministration parses entity lines, applies strict macro validation,
// performs aggressive macro expansion, and records open/close entity/space events into administration.
// lineNos runs parallel to entityLines: lineNos[i] is entityLines[i]'s original 1-indexed line
// number in entitiesPath. It must be used for every reported line number instead of i+1 --
// entityLines is layer-extracted content with blank/comment lines already stripped, so the two
// no longer coincide. Pass nil to fall back to i+1 (entityLines is the raw, unstripped file).
func ParseEntitiesAndFillAdministration(entityLines []string, lineNos []int, entitiesPath string, settings map[string]string, report *strings.Builder, hostDevicesByID map[string]THostDevice, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, commandlineDevicesByID map[string]TCommandlineDevice, localDevicesByID map[string]TLocalDevice, logicalDevicesByID map[string]TLogicalDevice, capabilityDefaults []TCapabilityDefaultRule) (TExpansionParseResult, error) {
	sourceLine := func(i int) int {
		if lineNos != nil && i >= 0 && i < len(lineNos) {
			return lineNos[i]
		}
		return i + 1
	}
	administration := newAdministrationState()
	administration.CapabilityDefaults = capabilityDefaults
	onSpaceClosed := func(_ string) {
		// Space-close hooks are centralized in administration; aggregate derivation stays a separate pass.
	}

	invocationCount := 0
	validInvocations := 0
	typeErrors := 0

	// pendingCapabilityLink/pendingSourceLink hold "entity ... from <device-id> ...;" /
	// "entity ... as ... from <device-id>;" declarations reached before their device's own
	// "device <spec> from <device-id>;" (U2) positioning -- registerDeviceCapabilityEntityLink /
	// registerDeviceSourceEntityLink report deferred=true rather than warning in that case (the
	// positioning may still appear later in the same file), and the main loop below queues them
	// here instead of registering immediately. Retried once, after the whole file has been read
	// (below, past the main loop) -- an in-memory retry over already-parsed declarations, not a
	// second scan of entityLines. This is what keeps positioning order-independent without the
	// separate scratch-state pre-pass this file used to have (removed 2026-08-27 after it caused a
	// real bug: the pre-pass wrote into the real administration state ahead of the real OpenSpace
	// call for the same space, fooling EnsureSpaceRegistered's idempotency check into never
	// registering that space at all -- see Conceptual_DevicePositioning.go's doc comment).
	//
	// spacePath is a snapshot (own backing array, never aliasing administration.SpacePath) of
	// administration.SpacePath at the moment the line was first read -- real bug found live
	// 2026-09-07, PROJECT.md item 6: without this, the retry below resolved a deferred line against
	// whatever space administration.SpacePath happened to be at AFTER the whole file was read
	// (typically back at "root"), not the space the line actually sits in. deviceNamePath
	// (pendingCapabilityLink only -- pendingSourceLink's construct is never used inside a device-with
	// block, see registerDeviceSourceEntityLink's doc comment) is the same per-line snapshot for
	// PROJECT.md item 5's device-name-path sugar.
	type pendingCapabilityLink struct {
		decl           TDeviceCapabilityEntityDeclaration
		lineNum        int
		spacePath      []string
		deviceNamePath string
		// allowHidden is true only for a hidden discovery capability's own auto-implied
		// registration -- never for an ordinary DSL-authored "entity ... from <device-id>
		// <capability>;" line, which must still be refused if it names one.
		allowHidden bool
		// capabilityIsInferred is true only when decl.Capability was auto-inferred by
		// expandForDeviceBareEntityLine (the no-rename shorthand, no "from" clause at all) rather
		// than written explicitly -- see registerDeviceCapabilityEntityLinkAllowingHidden's own
		// longest-suffix-fallback note (2026-09-25) for why the distinction matters on retry too.
		capabilityIsInferred bool
	}
	type pendingSourceLink struct {
		decl      TDeviceSourceEntityDeclaration
		lineNum   int
		spacePath []string
	}
	var pendingCapabilityLinks []pendingCapabilityLink
	var pendingSourceLinks []pendingSourceLink

	// forDeviceID is "" outside a device-capability-shorthand block -- either the older standalone
	// "for <device-id>: ... end;" form or the merged "device <spec> from <device-id> with: ...
	// end;" form (Conceptual_DeviceCapabilityEntities.go / Conceptual_DevicePositioning.go); a flat
	// state flag, not a stack -- nesting isn't supported, there's no use case for it.
	forDeviceID := ""
	// forDeviceNamePath mirrors forDeviceID: the positioned device's own leaf path, set only while
	// inside the block -- PROJECT.md item 5, "device declaration acts like a space". Always ""
	// from THIS source since PROJECT.md item 10 (2026-10-11) dropped the "device <id> as <leaf>
	// with:" clause from the grammar outright -- the header can no longer author a non-empty leaf
	// at all. Threaded through unchanged (rather than deleted) because registerDeviceCapabilityEntityLink
	// and its own downstream chain still take a deviceNamePath parameter for a genuinely different,
	// still-live purpose: device_friendly_name.go's applyColocatedPositioning/
	// triggerLogicalOwnNodeCapabilities feed it a COMPUTED leaf (never author-written) for a
	// colocated Logical.def target or a sibling-derived "node" leaf.
	forDeviceNamePath := ""

	// forDeviceAtLeaf is "" unless the current device block was opened via the NEW (2026-10-15)
	// "device <id> at <leaf> with:" header -- deliberately SEPARATE from forDeviceNamePath above
	// (not reusing namingSpacePath's own blanket fold, which applies to every explicit-sphere-path
	// line unconditionally): this leaf only ever folds into a line that opts in via "::"
	// (expandDeviceAtLeafDoubleColon, below).
	forDeviceAtLeaf := ""

	// currentRule is nil outside a "rule <spec> with: ...; end;" body -- a flat state flag, same
	// convention as forDeviceID immediately above (no nesting support, no use case for it). Each
	// "when"/"then" statement line inside is parsed by the real three-layer CDL1-style parser
	// (rule_character_stream.go/rule_tokeniser.go/rule_parser.go, PROJECT.md item 13) rather than
	// more ad hoc line matching -- see TRule's own doc comment for why. currentRuleClauses tracks
	// how many of each top-level clause currentRule has seen so far, reset every time a new rule
	// opens -- see ruleClauseTracker's own doc comment for the exactly-one/at-most-one/exactly-one
	// shape this enforces (confirmed with the user 2026-10-13).
	var currentRule *TRule
	var currentRuleClauses ruleClauseTracker

	for i := 0; i < len(entityLines); i++ {
		trimmed := strings.TrimSpace(entityLines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if currentRule != nil {
			// Join wrapped continuation lines (ruleBodyLineNeedsContinuation, below) before any of
			// the checks below run, so a "when <clause> or\n <clause>" inline-OR trigger chain
			// (2026-10-13) can span multiple physical lines.
			for ruleBodyLineNeedsContinuation(trimmed) && i+1 < len(entityLines) {
				next := strings.TrimSpace(entityLines[i+1])
				if next == "" || strings.HasPrefix(next, "#") {
					break
				}
				i++
				trimmed += " " + next
			}
			if trimmed == EndToken+StatementEndToken {
				if administration.RuleNamesSeen[currentRule.Name] {
					fmt.Fprintf(os.Stderr, "[WARNING] Conceptual.def: rule %q is declared more than once -- each \"rule <spec> with: ...;\" must resolve to a unique name\n", currentRule.Name)
				}
				currentRuleClauses.checkComplete(currentRule.Name)
				administration.RuleNamesSeen[currentRule.Name] = true
				administration.Rules = append(administration.Rules, *currentRule)
				currentRule = nil
				continue
			}
			// "then if <entity-ref> is <state>:" is a multi-line sub-block (PROJECT.md item 13
			// Phase 2) -- the outer scanner's own job is only to find its own matching "end;"
			// (same convention as every other "... with:"/"end;" block boundary in this
			// function), not to parse its content; the real grammar work happens in
			// rule_parser.go via TRuleParser.ActionStatement, called once per line inside.
			if strings.HasPrefix(trimmed, "then if ") && strings.HasSuffix(trimmed, ":") {
				action, newI := parseIfActionBlock(administration, currentRule, entityLines, i, trimmed)
				i = newI
				if action != nil && currentRuleClauses.claimThen(currentRule.Name) {
					currentRule.Actions = append(currentRule.Actions, *action)
				}
				continue
			}
			// "then:" is a multi-action block (2026-10-13) -- several actions without repeating
			// "then" once per line, closed by "end;". Bare action lines inside (no leading
			// "then") reuse TRuleParser.ActionStatement directly, same as "then if"'s own
			// branches just above.
			if trimmed == "then:" {
				actions, newI := parseThenBlock(administration, currentRule, entityLines, i)
				i = newI
				if currentRuleClauses.claimThen(currentRule.Name) {
					currentRule.Actions = append(currentRule.Actions, actions...)
				}
				continue
			}
			parseRuleStatementLine(administration, currentRule, trimmed, &currentRuleClauses)
			continue
		}

		if ruleName, ok := strings.CutSuffix(strings.TrimPrefix(trimmed, "rule "), " with:"); ok && strings.HasPrefix(trimmed, "rule ") {
			currentRule = &TRule{
				SpaceName: administration.CurrentSpaceName(),
				Name:      normalizeEntityFullName(strings.TrimSpace(ruleName), administration.SpacePath),
			}
			currentRuleClauses = ruleClauseTracker{}
			continue
		}

		capabilityIsInferred := false
		if forDeviceID != "" {
			if trimmed == EndToken+StatementEndToken {
				forDeviceID = ""
				forDeviceNamePath = ""
				forDeviceAtLeaf = ""
				continue
			}
			if forDeviceAtLeaf != "" {
				trimmed = expandDeviceAtLeafDoubleColon(trimmed, forDeviceAtLeaf)
			}
			expanded, ok := expandForDeviceShorthandLine(trimmed, forDeviceID)
			if !ok {
				// 2026-09-19: try the even-shorter "entity <spec>;" shape (no "from" at all) --
				// only matches when it wasn't a "from"-having line to begin with, so there's no
				// ambiguity between the two; see expandForDeviceBareEntityLine's own doc comment.
				expanded, ok = expandForDeviceBareEntityLine(trimmed, forDeviceID)
				capabilityIsInferred = ok
			}
			if !ok {
				fmt.Fprintf(os.Stderr, "[WARNING] %s line %d: inside \"for %s:\" block, expected \"entity <spec> from <capability>;\", got %q; ignored\n", entitiesPath, sourceLine(i), forDeviceID, trimmed)
				continue
			}
			trimmed = expanded
		} else if deviceID, atLeaf, ok := extractDeviceAtLeafWithBlockHeader(trimmed); ok {
			administration.EnsureSpaceRegistered(administration.SpacePath, SpaceKindRegular)
			warnings := registerDevicePositioning(administration, TDevicePositioningDeclaration{DeviceID: deviceID}, discoveryGatewaysByID, hostDevicesByID, hassBridgeDevicesByID, importedDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID, entitiesPath, sourceLine(i))
			for _, w := range warnings {
				fmt.Fprintf(os.Stderr, "[WARNING] %s\n", w)
			}
			forDeviceID = deviceID
			forDeviceNamePath = ""
			forDeviceAtLeaf = atLeaf
			continue
		} else if withBlockDecl, ok := extractDeviceWithBlockHeader(trimmed); ok {
			administration.EnsureSpaceRegistered(administration.SpacePath, SpaceKindRegular)
			warnings := registerDevicePositioning(administration, *withBlockDecl, discoveryGatewaysByID, hostDevicesByID, hassBridgeDevicesByID, importedDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID, entitiesPath, sourceLine(i))
			for _, w := range warnings {
				fmt.Fprintf(os.Stderr, "[WARNING] %s\n", w)
			}
			forDeviceID = withBlockDecl.DeviceID
			forDeviceNamePath = ""
			continue
		} else if strings.HasPrefix(trimmed, "device ") && strings.HasSuffix(trimmed, " with:") {
			// A device header no form above recognises used to be skipped silently, leaving its
			// block's entities without their device (2026-10-26: "device utility.xc40 as area with:").
			fmt.Fprintf(os.Stderr, "[WARNING] %s line %d: unrecognised device header %q -- write \"device <id> with:\" or \"device <id> as leaf <leaf> with:\"; an area is declared on a space (\"space <sphere>:<name> as area with:\")\n", entitiesPath, sourceLine(i), trimmed)
			continue
		}

		// The space-level list directives ("space off", "light on", ...) share one generic shape --
		// see normalizeSpaceListDirective. Rewritten here into their original one-line form, so the
		// per-keyword handlers further down stay exactly as they were.
		if normalized, newI, ok := normalizeSpaceListDirective(entityLines, i, trimmed, entitiesPath, sourceLine); ok {
			trimmed, i = normalized, newI
		}

		if oneLinerDecl, embeddedEntity, ok := extractDeviceWithOneLiner(trimmed); ok {
			administration.EnsureSpaceRegistered(administration.SpacePath, SpaceKindRegular)
			warnings := registerDevicePositioning(administration, *oneLinerDecl, discoveryGatewaysByID, hostDevicesByID, hassBridgeDevicesByID, importedDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID, entitiesPath, sourceLine(i))
			for _, w := range warnings {
				fmt.Fprintf(os.Stderr, "[WARNING] %s\n", w)
			}
			oneLinerDeviceNamePath := "" // the grammar can no longer author a non-empty leaf here either -- see forDeviceNamePath's own doc comment above
			oneLinerCapabilityIsInferred := false
			expanded, expandOK := expandForDeviceShorthandLine(embeddedEntity, oneLinerDecl.DeviceID)
			if !expandOK {
				expanded, expandOK = expandForDeviceBareEntityLine(embeddedEntity, oneLinerDecl.DeviceID)
				oneLinerCapabilityIsInferred = expandOK
			}
			if !expandOK {
				fmt.Fprintf(os.Stderr, "[WARNING] %s line %d: \"device %s ... with %s\", expected \"entity <spec> [from <capability>];\", got %q; ignored\n", entitiesPath, sourceLine(i), oneLinerDecl.DeviceID, embeddedEntity, embeddedEntity)
				continue
			}
			if capabilityDecl, ok := extractDeviceCapabilityEntityDeclaration(expanded); ok {
				warnings, deferred := registerDeviceCapabilityEntityLink(administration, *capabilityDecl, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID, entitiesPath, sourceLine(i), false, oneLinerDeviceNamePath, oneLinerCapabilityIsInferred)
				if deferred {
					pendingCapabilityLinks = append(pendingCapabilityLinks, pendingCapabilityLink{
						decl:                 *capabilityDecl,
						lineNum:              sourceLine(i),
						spacePath:            append([]string{}, administration.SpacePath...),
						deviceNamePath:       oneLinerDeviceNamePath,
						capabilityIsInferred: oneLinerCapabilityIsInferred,
					})
				} else {
					for _, w := range warnings {
						fmt.Fprintf(os.Stderr, "[WARNING] %s\n", w)
					}
				}
			}
			continue
		}

		if capabilityDecl, ok := extractDeviceCapabilityEntityDeclaration(trimmed); ok {
			administration.EnsureSpaceRegistered(administration.SpacePath, SpaceKindRegular)
			warnings, deferred := registerDeviceCapabilityEntityLink(administration, *capabilityDecl, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID, entitiesPath, sourceLine(i), false, forDeviceNamePath, capabilityIsInferred)
			if deferred {
				pendingCapabilityLinks = append(pendingCapabilityLinks, pendingCapabilityLink{
					decl:                 *capabilityDecl,
					lineNum:              sourceLine(i),
					spacePath:            append([]string{}, administration.SpacePath...),
					deviceNamePath:       forDeviceNamePath,
					capabilityIsInferred: capabilityIsInferred,
				})
			} else {
				for _, w := range warnings {
					fmt.Fprintf(os.Stderr, "[WARNING] %s\n", w)
				}
			}
			continue
		}

		if discoveryDecl, ok := extractDiscoveryEntityDeclaration(trimmed); ok {
			administration.EnsureSpaceRegistered(administration.SpacePath, SpaceKindRegular)
			for _, w := range registerDiscoveryEntityLink(administration, *discoveryDecl, discoveryGatewaysByID, entitiesPath, sourceLine(i), forDeviceNamePath, false) {
				fmt.Fprintf(os.Stderr, "[WARNING] %s\n", w)
			}
			continue
		}

		if sourceDecl, ok := extractDeviceSourceEntityDeclaration(trimmed); ok {
			administration.EnsureSpaceRegistered(administration.SpacePath, SpaceKindRegular)
			warnings, deferred := registerDeviceSourceEntityLink(administration, *sourceDecl, hassBridgeDevicesByID, importedDevicesByID, entitiesPath, sourceLine(i), false, "", "")
			if deferred {
				pendingSourceLinks = append(pendingSourceLinks, pendingSourceLink{
					decl:      *sourceDecl,
					lineNum:   sourceLine(i),
					spacePath: append([]string{}, administration.SpacePath...),
				})
			} else {
				for _, w := range warnings {
					fmt.Fprintf(os.Stderr, "[WARNING] %s\n", w)
				}
			}
			continue
		}

		if entityDecl, ok := extractEntityDeclaration(trimmed); ok {

			administration.EnsureSpaceRegistered(administration.SpacePath, SpaceKindRegular)

			spaceName := administration.CurrentSpaceName()
			fullName := normalizeEntityFullName(entityDecl.Specification, administration.SpacePath)
			hasDefOrImport, optionKeys := analyzeEntityDefinitionContext(entityLines, i)
			hasConfigOptions := len(optionKeys) > 0
			// "no_collect" may be given as an inline suffix on the "entity ..." line, or as its
			// own bare statement in the entity's body — both forms exclude the entity from
			// space-level aggregation.
			noCollect := entityDecl.NoCollect
			for _, k := range optionKeys {
				if k == "no_collect" {
					noCollect = true
					break
				}
			}
			entry := fmt.Sprintf("%s (line %d)", fullName, sourceLine(i))
			if noCollect {
				entry += " [no_collect]"
			}
			externalEntry := ""
			hasExternalRef := false
			if !hasDefOrImport {
				hasExternalRef = true
				externalEntry = fmt.Sprintf("%s (line %d)", fullName, sourceLine(i))
				if hasConfigOptions {
					externalEntry += fmt.Sprintf(" [config options: %s]", strings.Join(optionKeys, ", "))
				}
			}

			record := TEntityRecord{
				Name:                  fullName,
				Identity:              extractEntityIdentity(fullName),
				NoCollect:             noCollect,
				HasDefinitionOrImport: hasDefOrImport,
				Provenance:            fmt.Sprintf("%s:%d", filepath.Base(entitiesPath), sourceLine(i)),
			}

			// Extract inline "with icon <value>;" for any entity type.
			if withIdx := strings.Index(trimmed, " with icon "); withIdx >= 0 {
				iconPart := strings.TrimSuffix(strings.TrimSpace(trimmed[withIdx+len(" with icon "):]), ";")
				iconPart = strings.Trim(iconPart, "\"")
				resolved := resolveSettingsVar(iconPart, settings)
				record.EntityIcon = resolved
				if record.Identity.Domain == "input_boolean" {
					record.InputBooleanIcon = resolved
				}
			}
			// "with definition as flipped <source>;" retired 2026-10-05 -- superseded by
			// Logical.def's "<domain>.<label> flipped <capability>;" absorb qualifier
			// (Conceptual_LogicalEntities.go's registerLogicalFlippedAbsorbedCapability), which
			// also stops the source needing its own free, separately-declared Conceptual.def
			// entity at all. No warning added for a leftover use of the old phrase: the
			// enclosing "entity ...;" line still registers normally (just without the flip
			// behaviour), and zero real .def sites used this by the time it was removed.
			// Extract inline "with definition as timer \"<duration>\";" — the declaring timer
			// gets that fixed duration.
			if withIdx := strings.Index(trimmed, " with definition as timer "); withIdx >= 0 {
				durationPart := strings.TrimSuffix(strings.TrimSpace(trimmed[withIdx+len(" with definition as timer "):]), ";")
				durationPart = strings.Trim(durationPart, "\"")
				administration.TimerDefRelations = append(administration.TimerDefRelations, TTimerDefRelation{
					SelfEntity: fullName,
					Duration:   durationPart,
				})
			}
			if strings.HasSuffix(trimmed, " with:") {
				administration.PendingEntityCollections = append(administration.PendingEntityCollections, TPendingEntityCollection{
					SpaceName:      spaceName,
					Entry:          entry,
					ExternalEntry:  externalEntry,
					Record:         record,
					ExpectedDepth:  len(administration.OpenBlocks) + 1,
					HasExternalRef: hasExternalRef,
				})
			} else {
				administration.RegisterEntityClosure(TPendingEntityCollection{
					SpaceName:      spaceName,
					Entry:          entry,
					ExternalEntry:  externalEntry,
					Record:         record,
					HasExternalRef: hasExternalRef,
				})
			}
		}

		// Record "cli_switch <alias> <fqdn> <on> <off> <state>;" directives inside entity bodies.
		if strings.HasPrefix(trimmed, "cli_switch ") && len(administration.PendingEntityCollections) > 0 {
			fields := strings.Fields(strings.TrimSuffix(trimmed, ";"))
			if len(fields) >= 6 {
				localName := administration.PendingEntityCollections[len(administration.PendingEntityCollections)-1].Record.Name
				administration.CliSwitches = append(administration.CliSwitches, TCliSwitchRecord{
					LocalEntityName: localName,
					UserAlias:       fields[1],
					HostFQDN:        fields[2],
					OnScript:        fields[3],
					OffScript:       fields[4],
					StateScript:     fields[5],
				})
			}
			continue
		}

		// Record "minimum/maximum/step/unit/icon" directives inside input_number entity bodies.
		if len(administration.PendingEntityCollections) > 0 {
			lastIdx := len(administration.PendingEntityCollections) - 1
			pending := &administration.PendingEntityCollections[lastIdx]
			if pending.Record.Identity.Domain == "input_number" {
				for _, prefix := range []struct{ pfx, field string }{
					{"minimum ", "min"}, {"maximum ", "max"}, {"step ", "step"},
					{"unit ", "unit"}, {"unit_of_measurement ", "unit"},
				} {
					if strings.HasPrefix(trimmed, prefix.pfx) {
						val := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(trimmed, prefix.pfx)), ";")
						val = strings.Trim(val, "\"")
						val = resolveSettingsVar(val, settings)
						switch prefix.field {
						case "min":
							pending.Record.InputNumberMin = val
						case "max":
							pending.Record.InputNumberMax = val
						case "step":
							pending.Record.InputNumberStep = val
						case "unit":
							pending.Record.InputNumberUnit = val
						}
						continue
					}
				}
				if strings.HasPrefix(trimmed, "icon ") {
					val := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(trimmed, "icon ")), ";")
					val = strings.Trim(val, "\"")
					pending.Record.InputNumberIcon = resolveSettingsVar(val, settings)
					continue
				}
			}
		}

		// Record "icon <value>;" directives inside entity bodies.
		if strings.HasPrefix(trimmed, "icon ") && len(administration.PendingEntityCollections) > 0 {
			lastIdx := len(administration.PendingEntityCollections) - 1
			pending := &administration.PendingEntityCollections[lastIdx]
			iconVal := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(trimmed, "icon ")), ";")
			iconVal = strings.Trim(iconVal, "\"")
			resolved := resolveSettingsVar(iconVal, settings)
			pending.Record.EntityIcon = resolved
			if pending.Record.Identity.Domain == "input_boolean" {
				pending.Record.InputBooleanIcon = resolved
			}
			continue
		}

		// Record "has_time;" / "has_date;" directives inside input_datetime entity bodies.
		if (trimmed == "has_time;" || trimmed == "has_date;") && len(administration.PendingEntityCollections) > 0 {
			lastIdx := len(administration.PendingEntityCollections) - 1
			pending := &administration.PendingEntityCollections[lastIdx]
			if pending.Record.Identity.Domain == "input_datetime" {
				if trimmed == "has_time;" {
					pending.Record.InputDatetimeHasTime = true
				} else {
					pending.Record.InputDatetimeHasDate = true
				}
				continue
			}
		}

		// Record "condition <sources> <expr>;" directives inside entity bodies.
		// Record "definition as switched_device <device> <main>;" — used to generate on/off follower automations.
		if strings.HasPrefix(trimmed, "definition as switched_device ") && len(administration.PendingEntityCollections) > 0 {
			spaceName := administration.CurrentSpaceName()
			selfEntity := administration.PendingEntityCollections[len(administration.PendingEntityCollections)-1].Record.Name
			parts := strings.Fields(strings.TrimSuffix(trimmed, ";"))
			// parts: [definition, as, switched_device, <device>, <main>]
			if len(parts) == 5 {
				device := normalizeEntityFullName(parts[3], administration.SpacePath)
				mainEnt := normalizeEntityFullName(parts[4], administration.SpacePath)
				if !strings.ContainsAny(device+mainEnt, "$:{}[]*") {
					administration.SwitchedDeviceRelations = append(administration.SwitchedDeviceRelations, TSwitchedDeviceRelation{
						SpaceName:  spaceName,
						SelfEntity: selfEntity,
						Device:     device,
						MainEntity: mainEnt,
					})
				}
			}
			// fall through: still mark entity as having a definition
		}

		// Record "definition as has_state <source> <state> [delay_on] [delay_off];" — the
		// declaring binary_sensor mirrors whether <source> is in <state>.
		if strings.HasPrefix(trimmed, "definition as has_state ") && len(administration.PendingEntityCollections) > 0 {
			selfEntity := administration.PendingEntityCollections[len(administration.PendingEntityCollections)-1].Record.Name
			fields := strings.Fields(strings.TrimSuffix(trimmed, ";"))
			// fields: [definition, as, has_state, <source>, <state>, [delay_on], [delay_off]]
			for i, f := range fields {
				fields[i] = strings.Trim(f, "\"")
			}
			if len(fields) >= 5 {
				source := normalizeEntityFullName(fields[3], administration.SpacePath)
				state := fields[4]
				delayOn, delayOff := "", ""
				if len(fields) >= 6 {
					delayOn = fields[5]
				}
				if len(fields) >= 7 {
					delayOff = fields[6]
				}
				if !strings.ContainsAny(source, "$:{}[]*") {
					administration.HasStateRelations = append(administration.HasStateRelations, THasStateRelation{
						SelfEntity: selfEntity,
						Source:     source,
						State:      state,
						DelayOn:    delayOn,
						DelayOff:   delayOff,
					})
				}
			}
			// fall through: still mark entity as having a definition
		}

		if strings.HasPrefix(trimmed, "condition ") && len(administration.PendingEntityCollections) > 0 {
			lastIdx := len(administration.PendingEntityCollections) - 1
			pending := &administration.PendingEntityCollections[lastIdx]
			// A directly-declared "/node" or "/battery_alert" entity (its own explicit
			// "condition ...;" clause, not produced by the "providing"/battery_alert macros)
			// gets its ConditionExpr captured like any other entity, so generateConditionEntities
			// renders it — the hardcoded default in generateTemplateBinarySensors only applies
			// when there is no explicit definition at all (see resolveNodeRepresentative).
			pending.Record.ConditionSources, pending.Record.ConditionExpr = parseConditionDirective(trimmed, administration.SpacePath)
			continue
		}

		if spaceKind, spaceName, isArea, ok := parseSpaceHeader(trimmed); ok {
			administration.OpenSpace(spaceKind, spaceName, isArea)
			continue
		}

		if strings.HasSuffix(trimmed, "with:") {
			administration.OpenOtherBlock()
			continue
		}

		// Handle "space off: ..." statements
		if strings.HasPrefix(trimmed, "space off:") {
			spaceName := administration.CurrentSpaceName()
			itemsStr := strings.TrimPrefix(trimmed, "space off:")
			itemsStr = strings.TrimSpace(itemsStr)
			itemsStr = strings.TrimSuffix(itemsStr, ";")
			items := parseSpaceCollectionItems(itemsStr)
			extensionalItems := []string{}
			for _, item := range items {
				// Aggregation tokens (@light, @media, @all) are stored as-is for later expansion.
				if strings.HasPrefix(item, "@") {
					extensionalItems = append(extensionalItems, item)
					continue
				}
				normalized := normalizeEntityFullName(item, strings.Split(spaceName, "/"))
				if strings.ContainsAny(normalized, "$:{}[]*") || normalized == "" {
					fmt.Fprintf(os.Stderr, "[SERIOUS WARNING] Space %q: intensional or unresolved entity reference %q (normalized as %q) in space off; only extensional references are allowed and will be stored.\n", spaceName, item, normalized)
					continue
				}
				extensionalItems = append(extensionalItems, normalized)
			}
			administration.RecordSpaceOff(spaceName, extensionalItems)
			continue
		}

		// Handle "light on: ..." statements
		if strings.HasPrefix(trimmed, "light on:") {
			// Only record SpaceOnByName for the current space block, not for nested/child spaces
			// This prevents parent 'light on:' from propagating to children
			spaceName := administration.CurrentSpaceName()
			lightsStr := strings.TrimPrefix(trimmed, "light on:")
			lightsStr = strings.TrimSpace(lightsStr)
			lightsStr = strings.TrimSuffix(lightsStr, ";")
			lights := parseSpaceCollectionItems(lightsStr)
			extensionalLights := []string{}
			for _, light := range lights {
				lightRef := strings.TrimSpace(light)
				// Aggregation tokens (@light, @media, @all) are stored as-is for later expansion.
				if strings.HasPrefix(lightRef, "@") {
					extensionalLights = append(extensionalLights, lightRef)
					continue
				}
				// Prepend "light." when the reference has no domain prefix.
				if !strings.Contains(lightRef, ".") {
					lightRef = "light." + lightRef
				}
				normalized := normalizeEntityFullName(lightRef, strings.Split(spaceName, "/"))
				if strings.ContainsAny(normalized, "$:{}[]*") || normalized == "" {
					fmt.Fprintf(os.Stderr, "[SERIOUS WARNING] Space %q: intensional or unresolved entity reference %q (normalized as %q) in light on; only extensional references are allowed and will be stored.\n", spaceName, light, normalized)
					continue
				}
				extensionalLights = append(extensionalLights, normalized)
			}
			// Only set SpaceOnByName if inside a direct space block (not inherited)
			if len(administration.SpacePath) > 0 {
				administration.RecordSpaceOn(spaceName, extensionalLights)
			}
			continue
		}

		// Handle "light off: ..." statements — records which lights to turn off for the aggregate entity.
		if strings.HasPrefix(trimmed, "light off:") {
			spaceName := administration.CurrentSpaceName()
			lightsStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "light off:"))
			lightsStr = strings.TrimSuffix(lightsStr, ";")
			lights := parseSpaceCollectionItems(lightsStr)
			offLights := []string{}
			for _, light := range lights {
				lightRef := strings.TrimSpace(light)
				if strings.HasPrefix(lightRef, "@") {
					offLights = append(offLights, lightRef)
					continue
				}
				if !strings.Contains(lightRef, ".") {
					lightRef = "light." + lightRef
				}
				normalized := normalizeEntityFullName(lightRef, strings.Split(spaceName, "/"))
				if strings.ContainsAny(normalized, "$:{}[]*") || normalized == "" {
					continue
				}
				offLights = append(offLights, normalized)
			}
			if len(administration.SpacePath) > 0 {
				administration.RecordLightOff(spaceName, offLights)
			}
			continue
		}

		// "availability all;" (PROJECT.md item 17b): this space's template aggregates (light, space
		// and media switch) are available only while every member is, instead of any member.
		if trimmed == "availability all;" {
			administration.SpaceAvailabilityAll[administration.CurrentSpaceName()] = true
			continue
		}

		// Handle "heating leak: ..." statements — records which entities (doors/windows) must be
		// closed before a radiator can safely run.  Generates a leakage_evidence sensor per space.
		if strings.HasPrefix(trimmed, "heating leak:") {
			spaceName := administration.CurrentSpaceName()
			refsStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "heating leak:"))
			refsStr = strings.TrimSuffix(refsStr, ";")
			normalizedRefs := []string{}
			for _, ref := range parseSpaceCollectionItems(refsStr) {
				ref = strings.TrimSpace(ref)
				if ref == "" {
					continue
				}
				normalized := normalizeEntityFullName(ref, strings.Split(spaceName, "/"))
				if strings.ContainsAny(normalized, "$:{}[]*") || normalized == "" {
					fmt.Fprintf(os.Stderr, "[WARNING] Space %q: unresolvable heating leak reference %q\n", spaceName, ref)
					continue
				}
				normalizedRefs = append(normalizedRefs, normalized)
			}
			administration.RecordHeatingLeak(spaceName, normalizedRefs)
			continue
		}

		// Handle "lights motion guarded [with delay N];" — auto-registers a no_motion/ignore
		// input_boolean for the current space, which is then picked up by
		// generateNoMotionAutomations.
		if strings.HasPrefix(trimmed, "lights motion guarded") {
			spaceName := administration.CurrentSpaceName()
			entityName := "input_boolean." + spaceName + "/no_motion/ignore"
			administration.EnsureSpaceRegistered(administration.SpacePath, SpaceKindRegular)
			administration.RegisterEntityClosure(TPendingEntityCollection{
				SpaceName: spaceName,
				Entry:     entityName + " (lights motion guarded)",
				Record: TEntityRecord{
					Name:                  entityName,
					Identity:              extractEntityIdentity(entityName),
					NoCollect:             true,
					HasDefinitionOrImport: true,
					InputBooleanIcon:      "mdi:motion-sensor-off",
					Provenance:            "lights motion guarded:" + spaceName,
				},
				HasExternalRef: false,
			})
			// Parse optional "with delay N" suffix.
			bare := strings.TrimSuffix(trimmed, ";")
			if idx := strings.Index(bare, "with delay "); idx >= 0 {
				if n, err := strconv.Atoi(strings.TrimSpace(bare[idx+len("with delay "):])); err == nil && n > 0 {
					administration.SpaceNoMotionDelayByName[spaceName] = n
				}
			}
			continue
		}

		// Handle "follows <follower> <leader>;" — records a follower-light relation.
		if strings.HasPrefix(trimmed, "follows ") {
			spaceName := administration.CurrentSpaceName()
			parts := strings.Fields(strings.TrimSuffix(trimmed, ";"))
			if len(parts) == 3 {
				follower := normalizeEntityFullName(parts[1], administration.SpacePath)
				leader := normalizeEntityFullName(parts[2], administration.SpacePath)
				if !strings.ContainsAny(follower+leader, "$:{}[]*") {
					administration.FollowsRelations = append(administration.FollowsRelations, TFollowsRelation{
						SpaceName: spaceName,
						Follower:  follower,
						Leader:    leader,
					})
				}
			}
			continue
		}

		// Handle "limits <timer> <entity>: off on;" — records a timer-limits relation for removing-smell automations.
		// Use LastIndex to find the colon separating entity refs from the direction tokens,
		// since the timer ref itself may contain a colon (intensional form).
		if strings.HasPrefix(trimmed, "limits ") {
			spaceName := administration.CurrentSpaceName()
			content := strings.TrimSuffix(strings.TrimPrefix(trimmed, "limits "), ";")
			colonIdx := strings.LastIndex(content, ":")
			if colonIdx > 0 {
				refs := strings.Fields(content[:colonIdx])
				if len(refs) == 2 {
					timerEntity := normalizeEntityFullName(refs[0], administration.SpacePath)
					boundEntity := normalizeEntityFullName(refs[1], administration.SpacePath)
					if !strings.ContainsAny(timerEntity+boundEntity, "$:{}[]*") {
						administration.TimerLimitsRelations = append(administration.TimerLimitsRelations, TTimerLimitsRelation{
							SpaceName:   spaceName,
							TimerEntity: timerEntity,
							BoundEntity: boundEntity,
						})
					}
				}
			}
			continue
		}

		// A bare "when ...;"/"then ...;" outside any "rule <spec> with: ...; end;" block (handled
		// above, right after currentRule's own nil check) means the author forgot the wrapper --
		// warn with the exact fix rather than silently dropping the statement, matching this
		// codebase's own established practice for a construct that only makes sense inside its own
		// block (e.g. a bare "enabler <id>;" -> "... enabled by ...;" retirement warning).
		if strings.HasPrefix(trimmed, "when ") || strings.HasPrefix(trimmed, "then ") {
			fmt.Fprintf(os.Stderr, "[WARNING] Conceptual.def: %q outside a \"rule <spec> with: ...; end;\" block -- see PROJECT.md item 13\n", trimmed)
			continue
		}

		// Handle "space on: <items>;" — records the explicit turn-on items for the space switch.
		// Distinct from "light on:" which drives the template light entity.
		if strings.HasPrefix(trimmed, "space on:") {
			spaceName := administration.CurrentSpaceName()
			itemsStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "space on:"))
			itemsStr = strings.TrimSuffix(itemsStr, ";")
			extensionalItems := []string{}
			for _, item := range parseSpaceCollectionItems(itemsStr) {
				item = strings.TrimSpace(item)
				if item == "" {
					continue
				}
				if !strings.Contains(item, ".") {
					item = "light." + item
				}
				normalized := normalizeEntityFullName(item, administration.SpacePath)
				if strings.ContainsAny(normalized, "$:{}[]*") || normalized == "" {
					continue
				}
				extensionalItems = append(extensionalItems, normalized)
			}
			if len(administration.SpacePath) > 0 {
				administration.RecordSpaceSwitchOn(spaceName, extensionalItems)
			}
			continue
		}

		// Handle "member space <spaceName>;" (a whole member space, its own light/media/switch
		// aggregates pulled into virtual-space @all expansion) and "member entity <entity-ref>;"
		// (2026-10-04: a single literal entity, any domain, included directly) -- the two keywords
		// disambiguating what used to be one bare "member <ref>;" form (always a space).
		if strings.HasPrefix(trimmed, "member ") {
			rest := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(trimmed, "member ")), ";")
			switch {
			case strings.HasPrefix(rest, "space "):
				memberRef := strings.TrimSpace(strings.TrimPrefix(rest, "space "))
				if memberRef != "" {
					spaceName := administration.CurrentSpaceName()
					administration.SpaceMembersByName[spaceName] = append(
						administration.SpaceMembersByName[spaceName], memberRef)
				}
			case strings.HasPrefix(rest, "entity "):
				item := strings.TrimSpace(strings.TrimPrefix(rest, "entity "))
				if item != "" {
					if !strings.Contains(item, ".") {
						item = "light." + item
					}
					if normalized := normalizeEntityFullName(item, administration.SpacePath); normalized != "" && !strings.ContainsAny(normalized, "$:{}[]*") {
						spaceName := administration.CurrentSpaceName()
						administration.SpaceMemberEntitiesByName[spaceName] = append(
							administration.SpaceMemberEntitiesByName[spaceName], normalized)
					}
				}
			default:
				fmt.Fprintf(os.Stderr, "[WARNING] Conceptual.def: bare \"member %s;\" is retired -- write \"member space %s;\" (or \"member entity ...;\" for a single literal entity)\n", rest, rest)
			}
			continue
		}

		if trimmed == EndToken+StatementEndToken {
			administration.HandleEndToken(onSpaceClosed)
		}
	}

	// One retry over the small pending list built above -- not a second scan of entityLines, just
	// re-registering declarations already parsed during the single main loop, now that every
	// "device <spec> from <device-id>;" positioning in the whole file has been seen. Anything still
	// unresolved here was never positioned anywhere in the file, so finalAttempt=true turns that
	// into a real, reported warning this time.
	//
	// administration.SpacePath is swapped to each pending line's own captured snapshot for the
	// duration of its retry call, then restored -- both registerDeviceCapabilityEntityLink and
	// registerDeviceSourceEntityLink read administration.SpacePath directly (for entity-name
	// resolution and for the space a resolved entity gets filed under), so without this a deferred
	// line would resolve against whatever space parsing happened to end up in, not the space it was
	// actually declared in (PROJECT.md item 6's real bug, found live 2026-09-07).
	origSpacePath := administration.SpacePath
	for _, pending := range pendingCapabilityLinks {
		administration.SpacePath = pending.spacePath
		// The generic entry point re-checks the "node" override on every call, including this retry
		// (Conceptual_DeviceCapabilityEntities.go's own top-of-function check) -- no separate
		// "via override" retry routing needed any more.
		warnings, _ := registerDeviceCapabilityEntityLinkAllowingHidden(administration, pending.decl, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID, entitiesPath, pending.lineNum, true, pending.deviceNamePath, pending.allowHidden, pending.capabilityIsInferred)
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "[WARNING] %s\n", w)
		}
	}
	for _, pending := range pendingSourceLinks {
		administration.SpacePath = pending.spacePath
		warnings, _ := registerDeviceSourceEntityLink(administration, pending.decl, hassBridgeDevicesByID, importedDevicesByID, entitiesPath, pending.lineNum, true, "", "")
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "[WARNING] %s\n", w)
		}
	}
	administration.SpacePath = origSpacePath

	// For every space with a climate entity or a physical heating switch and doors/windows to watch
	// ("heating leak:"), auto-register a leakage_evidence sensor. A space without them has no
	// leakage at all (2026-10-26: no more always-off constant standing in). The entity lives in the
	// physical domain at the same path as the social space.
	for _, spaceName := range heatingCapableSocialSpaceNames(administration) {
		if len(administration.HeatingLeaksByName[spaceName]) == 0 {
			continue
		}
		// Derive the path component (strip the leading sphere name, e.g. "social/kitchen" → "kitchen").
		contextPath := spaceName
		if idx := strings.Index(spaceName, "/"); idx >= 0 {
			contextPath = spaceName[idx+1:]
		} else if isKnownSphere(spaceName) {
			contextPath = ""
		}
		var entityName string
		if contextPath == "" {
			entityName = "binary_sensor.physical/heating/leakage/evidence"
		} else {
			entityName = fmt.Sprintf("binary_sensor.physical/%s/heating/leakage/evidence", contextPath)
		}
		administration.EnsureSpaceRegistered(administration.SpacePath, SpaceKindRegular)
		administration.RegisterEntityClosure(TPendingEntityCollection{
			SpaceName: spaceName,
			Entry:     fmt.Sprintf("%s (heating leak)", entityName),
			Record: TEntityRecord{
				Name:                  entityName,
				Identity:              extractEntityIdentity(entityName),
				NoCollect:             false,
				HasDefinitionOrImport: true,
				Provenance:            "heating_leak:" + spaceName,
			},
			HasExternalRef: false,
		})
	}

	administration.DeriveImpliedHeatingEntities()
	administration.DeriveBinarySensorSubdomainAggregates()
	RunPostParseChecks(administration)

	return TExpansionParseResult{
		Administration:   administration,
		InvocationCount:  invocationCount,
		ValidInvocations: validInvocations,
		TypeErrors:       typeErrors,
	}, nil
}

// ruleBodyLineNeedsContinuation reports whether a rule-body line is still incomplete and needs
// the next physical line appended -- delegates to lineIsCompleteStatement (defined.go), which
// this file's own universal whitespace-normalisation pass (normalizeDefinitionWhitespace) already
// uses to join a "when"/"if" clause's own continuation lines BEFORE this function ever sees a
// line; this is the second, independent pass over the (already mostly-joined) rule-body lines
// that handles "then:"/"then if ...:" multi-line blocks and "end;" -- both passes must agree on
// where a "when"/"if" clause ends, or one undoes the other's correct join.
func ruleBodyLineNeedsContinuation(trimmed string) bool {
	return !lineIsCompleteStatement(trimmed)
}

// parseRuleStatementLine parses one line of a "rule <spec> with: ...; end;" body (a "when
// <entity-ref> turns on|off;" trigger or a "then turn <entity-ref> on|off;" action) via the real
// three-layer CDL1-style parser (rule_character_stream.go/rule_tokeniser.go/rule_parser.go) and
// appends the result to rule. EntitySpec is resolved to a fully-qualified entity name here
// (normalizeEntityFullName), exactly like every other Conceptual.def relation construct resolves
// its own entity references at parse time (FollowsRelations/TimerLimitsRelations/
// SwitchedDeviceRelations) -- not deferred to generation time. Any parse failure is reported via
// the parser's own accumulated warnings; the line is otherwise skipped, matching "follows"/
// "limits"' own local precedent of silently skipping malformed input elsewhere in this function.
func parseRuleStatementLine(administration *TAdministrationState, rule *TRule, line string, clauses *ruleClauseTracker) {
	p := newRuleParser(line, rule.Name)

	var triggers []TRuleTrigger
	if p.WhenStatement(&triggers) {
		if clauses.claimWhen(rule.Name) {
			for i := range triggers {
				triggers[i].EntitySpec = normalizeEntityFullName(triggers[i].EntitySpec, administration.SpacePath)
				if triggers[i].Kind == "named_trigger" {
					// Space-relative, same as EntitySpec above: the same relative name in two
					// spaces names two different events.
					triggers[i].Name = normalizeEntityFullName(triggers[i].Name, administration.SpacePath)
				}
			}
			rule.Triggers = append(rule.Triggers, triggers...)
		}
		return
	}

	var cond TRuleCondition
	if p.IfStatement(&cond) {
		if clauses.claimCondition(rule.Name) {
			resolveConditionEntitySpecs(administration, &cond)
			rule.Condition = &cond
		}
		return
	}

	var action TRuleAction
	if p.ThenStatement(&action) {
		if clauses.claimThen(rule.Name) {
			resolveActionEntitySpec(administration, &action)
			rule.Actions = append(rule.Actions, action)
		}
		return
	}

	for _, w := range p.Warnings {
		fmt.Fprintf(os.Stderr, "[WARNING] Conceptual.def: %s\n", w)
	}
	fmt.Fprintf(os.Stderr, "[WARNING] Conceptual.def: rule %q: unrecognised statement %q\n", rule.Name, line)
}

// resolveConditionEntitySpecs resolves every "state"-kind leaf's EntitySpec in a TRuleCondition
// tree in place, recursing into "and"/"or"/"not" Operands -- a condition can nest arbitrarily
// deep, unlike a trigger or action.
func resolveConditionEntitySpecs(administration *TAdministrationState, cond *TRuleCondition) {
	if cond.Kind == "state" {
		cond.EntitySpec = normalizeEntityFullName(cond.EntitySpec, administration.SpacePath)
	}
	for i := range cond.Operands {
		resolveConditionEntitySpecs(administration, &cond.Operands[i])
	}
}

// ruleClauseTracker enforces PROJECT.md item 13's own structural rule for a rule body
// (2026-10-13, confirmed with the user): exactly one "when" clause, at most one "if" clause,
// exactly one "then" clause (which may itself be a single action, a "then: ...; end;" block, or
// a "then if ...: ...; end;" branching action -- any one of those satisfies the one "then" slot).
// Reset fresh each time a new "rule <spec> with:" block opens.
type ruleClauseTracker struct {
	seenWhen, seenCondition, seenThen bool
}

// claimWhen reports whether this is the rule's first "when" clause, warning and returning false
// otherwise (the caller discards what it just parsed rather than appending it).
func (c *ruleClauseTracker) claimWhen(ruleName string) bool {
	if c.seenWhen {
		fmt.Fprintf(os.Stderr, "[WARNING] Conceptual.def: rule %q: exactly one \"when\" clause is allowed per rule -- ignoring this extra one\n", ruleName)
		return false
	}
	c.seenWhen = true
	return true
}

// claimCondition reports whether this is the rule's first "if" clause, warning and returning
// false otherwise.
func (c *ruleClauseTracker) claimCondition(ruleName string) bool {
	if c.seenCondition {
		fmt.Fprintf(os.Stderr, "[WARNING] Conceptual.def: rule %q: at most one \"if\" clause is allowed per rule -- combine conditions with and/or/not instead of repeating \"if\"; ignoring this extra one\n", ruleName)
		return false
	}
	c.seenCondition = true
	return true
}

// claimThen reports whether this is the rule's first "then" clause, warning and returning false
// otherwise.
func (c *ruleClauseTracker) claimThen(ruleName string) bool {
	if c.seenThen {
		fmt.Fprintf(os.Stderr, "[WARNING] Conceptual.def: rule %q: exactly one \"then\" clause is allowed per rule -- use \"then: ...; end;\" for multiple actions instead of repeating \"then\"; ignoring this extra one\n", ruleName)
		return false
	}
	c.seenThen = true
	return true
}

// checkComplete warns if the rule closed ("end;") without ever seeing its required "when"/"then"
// clause -- "if" is optional (at most one), so its absence is never warned about.
func (c *ruleClauseTracker) checkComplete(ruleName string) {
	if !c.seenWhen {
		fmt.Fprintf(os.Stderr, "[WARNING] Conceptual.def: rule %q: requires exactly one \"when\" clause, found none\n", ruleName)
	}
	if !c.seenThen {
		fmt.Fprintf(os.Stderr, "[WARNING] Conceptual.def: rule %q: requires exactly one \"then\" clause, found none\n", ruleName)
	}
}

// resolveActionEntitySpec resolves action.EntitySpec in place via normalizeEntityFullName --
// a no-op for "delay" (EntitySpec is already "", and normalizeEntityFullName's own early-return
// guard leaves an empty spec unchanged).
func resolveActionEntitySpec(administration *TAdministrationState, action *TRuleAction) {
	action.EntitySpec = normalizeEntityFullName(action.EntitySpec, administration.SpacePath)
}

// parseIfActionBlock parses a "then if <entity-ref> is <state>: <action>...; [else:
// <action>...;] end;" multi-line action block (PROJECT.md item 13 Phase 2's "if" action verb),
// starting at entityLines[headerIdx] == header (already confirmed by the caller to start with
// "then if " and end with ":"). Returns the parsed action (nil if the header itself failed to
// parse) and the index of the block's own "end;" line, so the caller's own loop index naturally
// advances past it on its next i++.
func parseIfActionBlock(administration *TAdministrationState, rule *TRule, entityLines []string, headerIdx int, header string) (*TRuleAction, int) {
	headerParser := newRuleParser(strings.TrimSuffix(header, ":"), rule.Name)
	var action TRuleAction
	headerOK := headerParser.IfActionHeader(&action)
	for _, w := range headerParser.Warnings {
		fmt.Fprintf(os.Stderr, "[WARNING] Conceptual.def: %s\n", w)
	}
	if headerOK {
		action.CondEntitySpec = normalizeEntityFullName(action.CondEntitySpec, administration.SpacePath)
	}

	inElse := false
	i := headerIdx + 1
	for ; i < len(entityLines); i++ {
		line := strings.TrimSpace(entityLines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == EndToken+StatementEndToken {
			break
		}
		if line == "else:" {
			inElse = true
			continue
		}
		branchParser := newRuleParser(line, rule.Name)
		var branchAction TRuleAction
		if branchParser.ActionStatement(&branchAction) {
			resolveActionEntitySpec(administration, &branchAction)
			if inElse {
				action.ElseActions = append(action.ElseActions, branchAction)
			} else {
				action.ThenActions = append(action.ThenActions, branchAction)
			}
			continue
		}
		for _, w := range branchParser.Warnings {
			fmt.Fprintf(os.Stderr, "[WARNING] Conceptual.def: %s\n", w)
		}
		fmt.Fprintf(os.Stderr, "[WARNING] Conceptual.def: rule %q: unrecognised statement %q\n", rule.Name, line)
	}

	if !headerOK {
		return nil, i
	}
	return &action, i
}

// parseThenBlock parses a "then: <action>; <action>; ...; end;" multi-action block (PROJECT.md
// item 13, 2026-10-13), starting right after entityLines[headerIdx] == "then:". Each line inside
// is a bare action statement (TRuleParser.ActionStatement, no leading "then" -- same grammar "if"
// actions' own then/else branches already reuse). Returns the index of the block's own "end;"
// line, so the caller's own loop index naturally advances past it.
func parseThenBlock(administration *TAdministrationState, rule *TRule, entityLines []string, headerIdx int) ([]TRuleAction, int) {
	var actions []TRuleAction
	i := headerIdx + 1
	for ; i < len(entityLines); i++ {
		line := strings.TrimSpace(entityLines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == EndToken+StatementEndToken {
			break
		}
		actionParser := newRuleParser(line, rule.Name)
		var action TRuleAction
		if actionParser.ActionStatement(&action) {
			resolveActionEntitySpec(administration, &action)
			actions = append(actions, action)
			continue
		}
		for _, w := range actionParser.Warnings {
			fmt.Fprintf(os.Stderr, "[WARNING] Conceptual.def: %s\n", w)
		}
		fmt.Fprintf(os.Stderr, "[WARNING] Conceptual.def: rule %q: unrecognised statement %q\n", rule.Name, line)
	}
	return actions, i
}
