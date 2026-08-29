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

// extractRelativeProvidingTarget extracts the target path from a "call providing :Y;" or
// "call providing /Y;" invocation (plain statement or "with: ..." block header). A ':'-prefixed
// target has its marker stripped — the caller re-attaches its own ':' when building the node
// entity's reference, so the space-relative resolution in normalizeEntityFullName still applies.
// A '/'-prefixed (absolute) target is returned with its leading '/' intact: appended after the
// caller's "sphere:" prefix, "sphere:/path" is exactly the marker normalizeEntityFullName needs
// to treat the path as absolute rather than relative to the calling space. Returns "" for
// unmarked (bare) targets — see warnOnUnqualifiedEntitySpacePathTarget for that case — or for
// invocations that aren't "providing" calls at all.
func extractRelativeProvidingTarget(invText string) string {
	invText = strings.TrimSpace(invText)
	if !strings.HasPrefix(invText, "call providing ") {
		return ""
	}
	rest := strings.TrimPrefix(invText, "call providing ")
	if idx := strings.Index(rest, " with"); idx > 0 {
		rest = rest[:idx]
	}
	rest = strings.TrimSuffix(rest, ";")
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, ":") {
		return strings.TrimPrefix(rest, ":")
	}
	if strings.HasPrefix(rest, "/") {
		return rest
	}
	return ""
}

// warnOnUnqualifiedEntitySpacePathTarget warns when a top-level (Entities.def-authored) macro
// invocation passes its positional entity_space_path argument without a ':' (relative) or '/'
// (absolute) marker — e.g. "call providing ring;" instead of "call providing :ring;". Such a
// value is used verbatim by the macro body with no space-context resolution, which usually isn't
// what the author intended and can silently collapse several distinct spaces' entities into one
// shared, wrongly-named entity. Only top-level invocations are checked (this function is called
// from processInvocation, never from macro-internal expansion), so every value seen here is
// literal author-typed text, not an already-resolved value passed between nested macro calls —
// the ambiguity that makes blanket auto-resolution unsafe deeper in the expansion engine doesn't
// apply here.
func warnOnUnqualifiedEntitySpacePathTarget(ctx *TMacroExpansionContext, invocation *TMacroInvocation, entitiesPath string, lineNum int) {
	macroDef, ok := ctx.Macros[invocation.Name]
	if !ok {
		return
	}
	for _, p := range macroDef.Parameters {
		if !p.Positional {
			continue
		}
		if p.Kind == ParamEntitySpacePath && isUnqualifiedEntitySpacePath(invocation.Target) {
			fmt.Fprintf(os.Stderr, "[WARNING] %s line %d: %q passed to %s's entity_space_path parameter without a ':' or '/' marker — did you mean %q (relative) or %q (absolute)?\n",
				entitiesPath, lineNum, invocation.Target, invocation.Name, ":"+invocation.Target, "/"+invocation.Target)
		}
		return
	}
}

// isUnqualifiedEntitySpacePath reports whether value is neither ':'-relative, '/'-absolute, nor a
// dotted entity_name (which castParamValue can cast to entity_space_path) — i.e. a bare fragment
// with no resolution marker at all.
func isUnqualifiedEntitySpacePath(value string) bool {
	return value != "" && !strings.HasPrefix(value, ":") && !strings.HasPrefix(value, "/") && !strings.Contains(value, ".")
}

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
func ParseEntitiesAndFillAdministration(entityLines []string, lineNos []int, entitiesPath string, ctx *TMacroExpansionContext, report *strings.Builder, hostDevicesByID map[string]THostDevice, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, capabilityDefaults []TCapabilityDefaultRule) (TExpansionParseResult, error) {
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

	// pendingCapabilityLink/pendingSourceLink hold "entity ... from <device-id> entity ...;" /
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
	type pendingCapabilityLink struct {
		decl    TDeviceCapabilityEntityDeclaration
		lineNum int
	}
	type pendingSourceLink struct {
		decl    TDeviceSourceEntityDeclaration
		lineNum int
	}
	var pendingCapabilityLinks []pendingCapabilityLink
	var pendingSourceLinks []pendingSourceLink

	// processInvocation parses, validates, expands, and registers one macro invocation text.
	// It is used both for top-level invocations and for inline "entity … with <invocation>;" bodies.
	processInvocation := func(invText string, lineNum int) error {
		invocationCount++
		report.WriteString(fmt.Sprintf("Line %d (in %s):\n", lineNum, formatSpacePath(administration.SpacePath)))
		report.WriteString(fmt.Sprintf("  Invocation: %s\n", invText))
		invocation, parseErr := ctx.ParseMacroInvocation(invText)
		if parseErr != nil {
			return fmt.Errorf("strict macro validation failed in %s at line %d: failed to parse invocation %q: %w", entitiesPath, lineNum, invText, parseErr)
		}
		if strictErr := ctx.ValidateInvocationStrict(invocation); strictErr != nil {
			return fmt.Errorf("strict macro validation failed in %s at line %d for invocation %q: %w", entitiesPath, lineNum, invText, strictErr)
		}
		warnOnUnqualifiedEntitySpacePathTarget(ctx, invocation, entitiesPath, lineNum)
		validInvocations++
		report.WriteString("  Status: OK (all parameters valid)\n")
		callChain := fmt.Sprintf("%s:%d → %s %s", filepath.Base(entitiesPath), lineNum, invocation.Name, invocation.Target)
		expandedRecords, expandErr := collectExpandedEntityRecords(ctx, invocation, administration.SpacePath, false, callChain)
		if expandErr != nil {
			return fmt.Errorf("strict macro validation failed in %s at line %d while expanding invocation %q: %w", entitiesPath, lineNum, invText, expandErr)
		}
		for _, expandedRecord := range expandedRecords {
			administration.EnsureSpaceRegistered(expandedRecord.SpacePath, SpaceKindRegular)
			expandedSpaceName := formatNestedSpaceName(expandedRecord.SpacePath)
			isExternal := !expandedRecord.Record.HasDefinitionOrImport
			externalEntry := ""
			if isExternal {
				externalEntry = expandedRecord.Record.Name
			}
			administration.RegisterEntityClosure(TPendingEntityCollection{
				SpaceName:      expandedSpaceName,
				Entry:          expandedRecord.Record.Name,
				ExternalEntry:  externalEntry,
				Record:         expandedRecord.Record,
				HasExternalRef: isExternal,
			})
		}
		if len(invocation.Parameters) > 0 {
			report.WriteString("  Parameters:\n")
			for pkey, pval := range invocation.Parameters {
				report.WriteString(fmt.Sprintf("    %s = %q\n", pkey, pval))
			}
		}
		report.WriteString("\n")
		return nil
	}

	// forDeviceID is "" outside a "for <device-id>: ... end;" block (Conceptual_DeviceCapabilityEntities.go's
	// device-capability-entity shorthand); a flat state flag, not a stack -- nesting isn't supported,
	// there's no use case for it.
	forDeviceID := ""

	for i := 0; i < len(entityLines); i++ {
		trimmed := strings.TrimSpace(entityLines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if forDeviceID != "" {
			if trimmed == EndToken+StatementEndToken {
				forDeviceID = ""
				continue
			}
			expanded, ok := expandForDeviceShorthandLine(trimmed, forDeviceID)
			if !ok {
				fmt.Fprintf(os.Stderr, "[WARNING] %s line %d: inside \"for %s:\" block, expected \"entity <spec> from entity <capability>;\", got %q; ignored\n", entitiesPath, sourceLine(i), forDeviceID, trimmed)
				continue
			}
			trimmed = expanded
		} else if deviceID, ok := extractForDeviceHeader(trimmed); ok {
			forDeviceID = deviceID
			continue
		}

		if candidateMacroName, isCreateInvocation := extractCallInvocationMacroName(trimmed); isCreateInvocation {
			if _, exists := ctx.Macros[candidateMacroName]; !exists {
				return TExpansionParseResult{}, fmt.Errorf("strict macro validation failed in %s at line %d: unknown macro %q in invocation %q", entitiesPath, sourceLine(i), candidateMacroName, trimmed)
			}
		}

		if positioningDecl, ok := extractDevicePositioningDeclaration(trimmed); ok {
			administration.EnsureSpaceRegistered(administration.SpacePath, SpaceKindRegular)
			for _, w := range registerDevicePositioning(administration, *positioningDecl, hassBridgeDevicesByID, entitiesPath, sourceLine(i)) {
				fmt.Fprintf(os.Stderr, "[WARNING] %s\n", w)
			}
			continue
		}

		if deviceDecl, ok := extractDeviceEntityDeclaration(trimmed); ok {
			administration.EnsureSpaceRegistered(administration.SpacePath, SpaceKindRegular)
			for _, w := range registerDeviceImpliedEntities(administration, *deviceDecl, hostDevicesByID, hassBridgeDevicesByID, entitiesPath, sourceLine(i)) {
				fmt.Fprintf(os.Stderr, "[WARNING] %s\n", w)
			}
			continue
		}

		if capabilityDecl, ok := extractDeviceCapabilityEntityDeclaration(trimmed); ok {
			administration.EnsureSpaceRegistered(administration.SpacePath, SpaceKindRegular)
			warnings, deferred := registerDeviceCapabilityEntityLink(administration, *capabilityDecl, discoveryGatewaysByID, hassBridgeDevicesByID, hostDevicesByID, entitiesPath, sourceLine(i), false)
			if deferred {
				pendingCapabilityLinks = append(pendingCapabilityLinks, pendingCapabilityLink{decl: *capabilityDecl, lineNum: sourceLine(i)})
			} else {
				for _, w := range warnings {
					fmt.Fprintf(os.Stderr, "[WARNING] %s\n", w)
				}
			}
			continue
		}

		if discoveryDecl, ok := extractDiscoveryEntityDeclaration(trimmed); ok {
			administration.EnsureSpaceRegistered(administration.SpacePath, SpaceKindRegular)
			for _, w := range registerDiscoveryEntityLink(administration, *discoveryDecl, discoveryGatewaysByID, entitiesPath, sourceLine(i)) {
				fmt.Fprintf(os.Stderr, "[WARNING] %s\n", w)
			}
			continue
		}

		if sourceDecl, ok := extractDeviceSourceEntityDeclaration(trimmed); ok {
			administration.EnsureSpaceRegistered(administration.SpacePath, SpaceKindRegular)
			warnings, deferred := registerDeviceSourceEntityLink(administration, *sourceDecl, hassBridgeDevicesByID, entitiesPath, sourceLine(i), false, "")
			if deferred {
				pendingSourceLinks = append(pendingSourceLinks, pendingSourceLink{decl: *sourceDecl, lineNum: sourceLine(i)})
			} else {
				for _, w := range warnings {
					fmt.Fprintf(os.Stderr, "[WARNING] %s\n", w)
				}
			}
			continue
		}

		if entityDecl, ok := extractEntityDeclaration(trimmed); ok {
			// A second ':' in an entity spec is legacy sub-domain notation; '/' must be used instead.
			if hasSecondColonSeparator(entityDecl.Specification) {
				fmt.Fprintf(os.Stderr, "[WARNING] %s line %d: entity specification %q uses a second ':' sub-domain separator (legacy); use '/' instead\n",
					entitiesPath, sourceLine(i), entityDecl.Specification)
			}

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

			// Extract inline "with adjustment <offset> <scale>;" properties.
			if withIdx := strings.Index(trimmed, " with adjustment "); withIdx >= 0 {
				adjPart := strings.TrimSuffix(strings.TrimSpace(trimmed[withIdx+len(" with adjustment "):]), ";")
				adjFields := strings.Fields(adjPart)
				if len(adjFields) >= 2 {
					record.AdjustmentOffset = adjFields[0]
					record.AdjustmentScale = adjFields[1]
				}
			}
			// Extract inline "with condition <sources> <expr>;" properties.
			if withIdx := strings.Index(trimmed, " with condition "); withIdx >= 0 {
				condPart := strings.TrimSpace(trimmed[withIdx+len(" with condition "):])
				record.ConditionSources, record.ConditionExpr = parseConditionDirective("condition "+condPart, administration.SpacePath)
			}
			// Extract inline "with icon <value>;" for any entity type.
			if withIdx := strings.Index(trimmed, " with icon "); withIdx >= 0 {
				iconPart := strings.TrimSuffix(strings.TrimSpace(trimmed[withIdx+len(" with icon "):]), ";")
				iconPart = strings.Trim(iconPart, "\"")
				resolved := resolveSettingsVar(iconPart, ctx.Settings)
				record.EntityIcon = resolved
				if record.Identity.Domain == "input_boolean" {
					record.InputBooleanIcon = resolved
				}
			}
			// Extract inline "with definition as flipped <source>;" — the declaring switch
			// mirrors <source>, inverted.
			if withIdx := strings.Index(trimmed, " with definition as flipped "); withIdx >= 0 {
				sourcePart := strings.TrimSuffix(strings.TrimSpace(trimmed[withIdx+len(" with definition as flipped "):]), ";")
				source := normalizeEntityFullName(sourcePart, administration.SpacePath)
				if !strings.ContainsAny(source, "$:{}[]*") {
					administration.FlippedRelations = append(administration.FlippedRelations, TFlippedRelation{
						SelfEntity: fullName,
						Source:     source,
					})
				}
			}
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
				// "entity … with <invocation>;" — process the inline body as a macro invocation.
				if withIdx := strings.Index(trimmed, " with "); withIdx >= 0 {
					inlineStmt := strings.TrimSpace(trimmed[withIdx+len(" with "):])
					if isMacroInvocation(inlineStmt, ctx.Macros) {
						if target := extractRelativeProvidingTarget(inlineStmt); target != "" {
							nodeEntityName := normalizeEntityFullName("binary_sensor.infrastructural:"+target+"/node", administration.SpacePath)
							administration.NodeRepresentativeByEntityID[nodeEntityName] = toHomeAssistantEntityID(fullName)
						}
						if err := processInvocation(inlineStmt, sourceLine(i)); err != nil {
							return TExpansionParseResult{}, err
						}
					}
				}
			}
		}

		// Record "imported rest <bridge> <remote_id> <interval> [<value_expr>];" directives.
		// These appear inside entity "with:" bodies; the enclosing entity is the last pending collection.
		if strings.HasPrefix(trimmed, "imported rest ") && len(administration.PendingEntityCollections) > 0 {
			fields := strings.Fields(strings.TrimSuffix(trimmed, ";"))
			if len(fields) >= 5 {
				bridgeName := fields[2]
				remoteID := fields[3]
				scanEvery, scanErr := strconv.Atoi(fields[4])
				if scanErr == nil && scanEvery > 0 {
					valueExpr := ""
					if len(fields) >= 6 {
						raw := strings.Join(fields[5:], " ")
						raw = strings.Trim(raw, "\"")
						valueExpr = raw
					}
					localName := administration.PendingEntityCollections[len(administration.PendingEntityCollections)-1].Record.Name
					administration.RestImports = append(administration.RestImports, TRestImportRecord{
						LocalEntityName: localName,
						BridgeName:      bridgeName,
						RemoteEntityID:  remoteID,
						ScanInterval:    scanEvery,
						ValueExpr:       valueExpr,
					})
				}
			}
			continue
		}

		// Record "cli_sensor <alias> <fqdn> <script>;" directives inside entity bodies.
		if strings.HasPrefix(trimmed, "cli_sensor ") && len(administration.PendingEntityCollections) > 0 {
			fields := strings.Fields(strings.TrimSuffix(trimmed, ";"))
			if len(fields) >= 4 {
				localName := administration.PendingEntityCollections[len(administration.PendingEntityCollections)-1].Record.Name
				administration.CliSensors = append(administration.CliSensors, TCliSensorRecord{
					LocalEntityName: localName,
					UserAlias:       fields[1],
					HostFQDN:        fields[2],
					ScriptPath:      fields[3],
				})
			}
			continue
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

		// Record "value <entity_spec>!<attribute>;" directives inside entity bodies.
		if strings.HasPrefix(trimmed, "value ") && len(administration.PendingEntityCollections) > 0 {
			raw := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "value "), ";"))
			exclamIdx := strings.Index(raw, "!")
			var normalised string
			if exclamIdx > 0 {
				entitySpec := raw[:exclamIdx]
				attribute := raw[exclamIdx+1:]
				normalised = normalizeEntityFullName(entitySpec, administration.SpacePath) + "!" + attribute
			} else {
				normalised = normalizeEntityFullName(raw, administration.SpacePath)
			}
			lastIdx := len(administration.PendingEntityCollections) - 1
			pending := &administration.PendingEntityCollections[lastIdx]
			pending.Record.ValueExpr = normalised
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
						val = resolveSettingsVar(val, ctx.Settings)
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
					pending.Record.InputNumberIcon = resolveSettingsVar(val, ctx.Settings)
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
			resolved := resolveSettingsVar(iconVal, ctx.Settings)
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

		if isMacroInvocation(trimmed, ctx.Macros) {
			// Collect multi-line "with:" body into a single invocation text if needed.
			invocationText := trimmed
			if strings.HasSuffix(trimmed, "with:") {
				j := i + 1
				for ; j < len(entityLines); j++ {
					nextTrimmed := strings.TrimSpace(entityLines[j])
					invocationText += "\n" + nextTrimmed
					if nextTrimmed == "end;" {
						break
					}
				}
				if j < len(entityLines) {
					i = j
				}
			}
			if target := extractRelativeProvidingTarget(invocationText); target != "" {
				nodeEntityName := normalizeEntityFullName("binary_sensor.infrastructural:"+target+"/node", administration.SpacePath)
				if len(administration.PendingEntityCollections) > 0 {
					hostName := administration.PendingEntityCollections[len(administration.PendingEntityCollections)-1].Record.Name
					administration.NodeRepresentativeByEntityID[nodeEntityName] = toHomeAssistantEntityID(hostName)
				}
			}
			if err := processInvocation(invocationText, sourceLine(i)); err != nil {
				return TExpansionParseResult{}, err
			}
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

		// Handle "lights_motion_guarded [with delay N];" — auto-registers a no_motion/ignore input_boolean
		// for the current space, which is then picked up by generateNoMotionAutomations.
		if strings.HasPrefix(trimmed, "lights_motion_guarded") {
			spaceName := administration.CurrentSpaceName()
			entityName := "input_boolean." + spaceName + "/no_motion/ignore"
			administration.EnsureSpaceRegistered(administration.SpacePath, SpaceKindRegular)
			administration.RegisterEntityClosure(TPendingEntityCollection{
				SpaceName: spaceName,
				Entry:     entityName + " (lights_motion_guarded)",
				Record: TEntityRecord{
					Name:                  entityName,
					Identity:              extractEntityIdentity(entityName),
					NoCollect:             true,
					HasDefinitionOrImport: true,
					InputBooleanIcon:      "mdi:motion-sensor-off",
					Provenance:            "lights_motion_guarded:" + spaceName,
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

		// Handle "member <spaceName>;" — records a member space for virtual-space @all expansion.
		if strings.HasPrefix(trimmed, "member ") {
			memberRef := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(trimmed, "member ")), ";")
			if memberRef != "" {
				spaceName := administration.CurrentSpaceName()
				administration.SpaceMembersByName[spaceName] = append(
					administration.SpaceMembersByName[spaceName], memberRef)
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
	for _, pending := range pendingCapabilityLinks {
		warnings, _ := registerDeviceCapabilityEntityLink(administration, pending.decl, discoveryGatewaysByID, hassBridgeDevicesByID, hostDevicesByID, entitiesPath, pending.lineNum, true)
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "[WARNING] %s\n", w)
		}
	}
	for _, pending := range pendingSourceLinks {
		warnings, _ := registerDeviceSourceEntityLink(administration, pending.decl, hassBridgeDevicesByID, entitiesPath, pending.lineNum, true, "")
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "[WARNING] %s\n", w)
		}
	}

	// For every space with a climate entity or a physical heating switch, auto-register a
	// leakage_evidence sensor — unconditionally, matching the legacy generator, since a space
	// without an explicit "heating leak:" still gets the sensor (aggregating to always-off via
	// binary_sensor.infrastructural_off, see generateBinarySensorGroups). The entity lives in the
	// physical domain at the same path as the social space.
	for _, spaceName := range heatingCapableSocialSpaceNames(administration) {
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
