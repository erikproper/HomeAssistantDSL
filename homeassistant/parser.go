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
	// forDeviceNamePath mirrors forDeviceID: the positioned device's own leaf path
	// (deviceSpecLeafPath of the with-block header's own spec, e.g. "netatmo"), set only while
	// inside the block -- PROJECT.md item 5, "device declaration acts like a space". Passed to
	// registerDeviceCapabilityEntityLink below so a capability line inside the block can write
	// "sensor.physical:co2" instead of repeating "sensor.physical:netatmo/co2"; see
	// registerDeviceSourceEntityLink's doc comment (Conceptual_DeviceSourceEntities.go) for why this
	// is a separate value from administration.SpacePath rather than pushed onto it.
	forDeviceNamePath := ""

	for i := 0; i < len(entityLines); i++ {
		trimmed := strings.TrimSpace(entityLines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		capabilityIsInferred := false
		if forDeviceID != "" {
			if trimmed == EndToken+StatementEndToken {
				forDeviceID = ""
				forDeviceNamePath = ""
				continue
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
		} else if withBlockDecl, ok := extractDeviceWithBlockHeader(trimmed); ok {
			administration.EnsureSpaceRegistered(administration.SpacePath, SpaceKindRegular)
			warnings := registerDevicePositioning(administration, *withBlockDecl, discoveryGatewaysByID, hostDevicesByID, hassBridgeDevicesByID, importedDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID, entitiesPath, sourceLine(i))
			for _, w := range warnings {
				fmt.Fprintf(os.Stderr, "[WARNING] %s\n", w)
			}
			forDeviceID = withBlockDecl.DeviceID
			forDeviceNamePath = deviceSpecLeafPath(withBlockDecl.Spec)
			continue
		}

		if oneLinerDecl, embeddedEntity, ok := extractDeviceWithOneLiner(trimmed); ok {
			administration.EnsureSpaceRegistered(administration.SpacePath, SpaceKindRegular)
			warnings := registerDevicePositioning(administration, *oneLinerDecl, discoveryGatewaysByID, hostDevicesByID, hassBridgeDevicesByID, importedDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID, entitiesPath, sourceLine(i))
			for _, w := range warnings {
				fmt.Fprintf(os.Stderr, "[WARNING] %s\n", w)
			}
			oneLinerDeviceNamePath := deviceSpecLeafPath(oneLinerDecl.Spec)
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

		// Handle "rule <spec> when <event-entity> turns on|off then turn <action-entity> on|off;"
		// (2026-09-29) -- records a TWhenRule. See TWhenRule's own doc comment for the deliberately
		// minimal scope of this first cut (single bare event atom, no "if" gate clause, no
		// composition, no multi-action block -- none of that is needed by either real site this was
		// built for) and for why the leading "rule <spec>" name is mandatory (a future multi-clause
		// rule body can't mechanically derive a name from its own trigger/action any more). <spec>
		// resolves exactly like a domain-less entity leaf (normalizeEntityFullName's own
		// "sphere:path" branch) -- reused as-is, not a new mechanism. Malformed input is silently
		// skipped, matching "follows"/"limits" immediately above -- this construct is new but
		// structurally identical to them, so it follows their own local precedent rather than
		// introducing a general warnings path this function doesn't otherwise have; the one
		// exception is a duplicate rule Name, which IS warned about (see WhenRuleNamesSeen's own
		// doc comment) since a silent collision would silently overwrite one rule's own generated
		// automation with another's.
		if strings.HasPrefix(trimmed, "rule ") {
			spaceName := administration.CurrentSpaceName()
			body := strings.TrimPrefix(trimmed, "rule ")
			whenIdx := strings.Index(body, " when ")
			if whenIdx > 0 {
				nameSpec := strings.TrimSpace(body[:whenIdx])
				content := strings.TrimSuffix(strings.TrimSpace(body[whenIdx+len(" when "):]), ";")
				thenIdx := strings.Index(content, " then ")
				if thenIdx > 0 {
					eventFields := strings.Fields(content[:thenIdx])
					actionFields := strings.Fields(content[thenIdx+len(" then "):])
					if nameSpec != "" && len(eventFields) == 3 && eventFields[1] == "turns" &&
						(eventFields[2] == "on" || eventFields[2] == "off") &&
						len(actionFields) == 3 && actionFields[0] == "turn" &&
						(actionFields[2] == "on" || actionFields[2] == "off") {
						eventEntity := normalizeEntityFullName(eventFields[0], administration.SpacePath)
						actionEntity := normalizeEntityFullName(actionFields[1], administration.SpacePath)
						ruleName := normalizeEntityFullName(nameSpec, administration.SpacePath)
						if !strings.ContainsAny(eventEntity+actionEntity+ruleName, "$:{}[]*") {
							if administration.WhenRuleNamesSeen[ruleName] {
								fmt.Fprintf(os.Stderr, "[WARNING] Conceptual.def: rule %q is declared more than once -- each \"rule <spec> when ...;\" must resolve to a unique name\n", ruleName)
							}
							administration.WhenRuleNamesSeen[ruleName] = true
							administration.WhenRules = append(administration.WhenRules, TWhenRule{
								SpaceName:    spaceName,
								Name:         ruleName,
								EventEntity:  eventEntity,
								EventState:   eventFields[2],
								ActionEntity: actionEntity,
								ActionState:  actionFields[2],
							})
						}
					}
				}
			}
			continue
		}

		// A bare "when ...;" with no leading "rule <spec>" is the RETIRED form (2026-09-29,
		// same day it first shipped) -- warn with the exact fix rather than silently dropping the
		// rule, matching this codebase's own established practice for a just-retired keyword
		// (e.g. "enabler <id>;" -> "... enabled by ...;").
		if strings.HasPrefix(trimmed, "when ") {
			fmt.Fprintf(os.Stderr, "[WARNING] Conceptual.def: bare \"when ...;\" is retired -- prefix it with a rule name, e.g. \"rule <sphere:leaf> when ...;\"\n")
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
