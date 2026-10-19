/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualNamedTriggerEntities
 *
 * Parses and registers the "trigger <name> when <capability> is '<value>'[,or]in
 * ['<v1>', '<v2>', ...]];" construct inside a "device <id> with: ... end;" block
 * (Conceptual.def) -- Step 1 of the device-level trigger-abstraction cleanup (2026-10-15,
 * PROJECT.md's own "physical sphere" item): a rule's own "when" clause should never need to know
 * a raw, device-level protocol code ("2_single") -- that detail belongs here, at the device, not
 * at the rule. See TAdministrationState.NamedTriggers' own doc comment (administration.go) for
 * the registry this feeds, and rule_parser.go's own "when [a] <name> is triggered" grammar for
 * the rule-side half.
 *
 * The underlying raw entity this construct implies (what an "entity ... from <capability>;" line
 * would otherwise have declared explicitly) is auto-derived, not author-named: same domain as
 * <capability>, "physical" sphere (today's own existing convention for a raw device-level event
 * entity -- unchanged by this step; eliminating "physical" as a sphere entirely is the separate,
 * later goal this is a prerequisite for), path = the device id's own bare label. Resolution
 * itself is NOT reimplemented -- it goes through registerDeviceCapabilityEntityLink, the exact
 * same choke point an ordinary "entity ... from <capability>;" line already uses, so this
 * construct can never drift from what that path does for hidden/switched-by/deferred capabilities.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 15.10.2026
 *
 */

package main

import (
	"fmt"
	"regexp"
	"strings"
)

// TNamedTriggerDeclaration is one parsed "trigger <name> when <capability> is '<value>'
// [in ['<v1>', '<v2>', ...]][ and if trigger.<attr> is '<value>'];" line.
type TNamedTriggerDeclaration struct {
	Name       string   // bare "sphere:path", no domain prefix -- the NamedTriggers key
	Capability string   // e.g. "event.pressed"
	EventTypes []string // one or more raw event_type values that mean this trigger fired
	// TriggerAttr/TriggerAttrValues (Step 2, 2026-10-15; list-value extended 2026-10-19 -- a
	// real Moes multi-gang button regression found live in Vienna, see
	// namedTriggerSourceCondition's own doc comment, generator.go) are "" unless this line
	// carried the optional "and if trigger.<attr> is '<value>'[in ['<v1>', ...]]" suffix --
	// TNamedTriggerSource's own doc comment (administration.go) explains the motivating case.
	TriggerAttr       string
	TriggerAttrValues []string
}

// namedTriggerAttrSuffixPattern is the optional "and if trigger.<attr> is '<value>'[or in
// ['<v1>', ...]]" tail shared by both value-shapes below -- captured once, appended to each
// pattern's own required prefix, so the attr-condition stays in exactly one place to maintain.
// Two alternatives for the value itself (single-quoted literal, or a bracketed list), mirroring
// the base capability-value pattern pair below exactly -- extracted group indices: (4)=attr,
// (5)=single value (if that alternative matched), (6)=raw list text (if that one did).
const namedTriggerAttrSuffixPattern = `(?:\s+and\s+if\s+trigger\.(\S+)\s+is\s+(?:'([^']*)'|in\s*\[\s*([^\]]+?)\s*\]))?\s*;$`

var namedTriggerSingleValuePattern = regexp.MustCompile(`^trigger\s+(\S+)\s+when\s+(\S+)\s+is\s+'([^']*)'` + namedTriggerAttrSuffixPattern)
var namedTriggerListValuePattern = regexp.MustCompile(`^trigger\s+(\S+)\s+when\s+(\S+)\s+is\s+in\s*\[\s*(.+?)\s*\]` + namedTriggerAttrSuffixPattern)

// splitQuotedListText parses a bracketed list's raw inner text ("'a', 'b'") into its unquoted
// elements -- shared by both the base capability-value list shape and the attr-condition's own
// list shape, so the two never drift in how they strip quotes/whitespace.
func splitQuotedListText(text string) []string {
	var values []string
	for _, part := range strings.Split(text, ",") {
		part = strings.Trim(strings.TrimSpace(part), "'")
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

// extractNamedTriggerDeclaration recognises either value-shape of the "trigger ...;" line, and
// either value-shape of its own optional attr-condition suffix (group 5: single value, group
// 6: bracketed list -- namedTriggerAttrSuffixPattern's own doc comment).
func extractNamedTriggerDeclaration(line string) (*TNamedTriggerDeclaration, bool) {
	if m := namedTriggerSingleValuePattern.FindStringSubmatch(line); m != nil && m[3] != "" {
		return &TNamedTriggerDeclaration{Name: m[1], Capability: m[2], EventTypes: []string{m[3]}, TriggerAttr: m[4], TriggerAttrValues: namedTriggerAttrValues(m)}, true
	}
	if m := namedTriggerListValuePattern.FindStringSubmatch(line); m != nil {
		values := splitQuotedListText(m[3])
		if len(values) == 0 {
			return nil, false
		}
		return &TNamedTriggerDeclaration{Name: m[1], Capability: m[2], EventTypes: values, TriggerAttr: m[4], TriggerAttrValues: namedTriggerAttrValues(m)}, true
	}
	return nil, false
}

// namedTriggerAttrValues extracts the attr-condition's own value(s) from a match against either
// top-level pattern (both share namedTriggerAttrSuffixPattern's group layout): m[5] for the
// single-value alternative, m[6] for the bracketed-list one. nil if the whole attr suffix is
// absent (m[4] == "").
func namedTriggerAttrValues(m []string) []string {
	if m[4] == "" {
		return nil
	}
	if m[5] != "" {
		return []string{m[5]}
	}
	return splitQuotedListText(m[6])
}

// registerNamedTrigger resolves decl's own capability for deviceID -- via
// registerDeviceCapabilityEntityLink, exactly as an ordinary "entity ... from <capability>;" line
// would, using an auto-derived LocalSpec -- atLeaf (from the device's own "at <leaf> with:"
// header, "" if it used plain "with:") when given, otherwise the device id's own bare label
// (this function's original 2026-10-15 fallback; kept for a device with no live entity_id to
// preserve, e.g. a brand-new one never declared any other way). Then records the resulting
// entity_id + decl's own EventTypes into administration.NamedTriggers, keyed by decl.Name
// resolved space-relative (normalizeEntityFullName, same as EntitySpec elsewhere) -- NOT taken
// literally, so the same relative name written in two different spaces (e.g. two different
// "social:main/light/toggle_request") resolves to two DIFFERENT fully-qualified keys, matching
// how every other DSL construct already works (a real bug found live 2026-10-15 migrating
// Vienna: two unrelated rules' own relative names collided before this fix). Warns (doesn't fail
// the whole parse) if the capability doesn't resolve, or if this exact device already declared
// this exact named trigger (a real authoring mistake, not a legitimate repeat -- unlike a
// capability shared across kinds, one device has no reason to declare the same name twice).
func registerNamedTrigger(administration *TAdministrationState, decl TNamedTriggerDeclaration, deviceID, atLeaf string, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, hostDevicesByID map[string]THostDevice, commandlineDevicesByID map[string]TCommandlineDevice, localDevicesByID map[string]TLocalDevice, logicalDevicesByID map[string]TLogicalDevice, entitiesPath string, lineNum int) []string {
	dotIdx := strings.Index(decl.Capability, ".")
	if dotIdx <= 0 {
		return []string{fmt.Sprintf("%s line %d: trigger %q: capability %q needs a domain prefix (e.g. \"event.pressed\")", entitiesPath, lineNum, decl.Name, decl.Capability)}
	}
	domain := decl.Capability[:dotIdx]
	bareCapability := bareCapabilityName(decl.Capability)
	leaf := atLeaf
	if leaf == "" {
		leaf = bareCapabilityName(deviceID)
	}
	localSpec := domain + ".physical:" + leaf

	// Several "trigger ...;" lines on the same device commonly share one capability (one raw
	// value each) -- resolve it only once, reusing the entity_id an earlier line already
	// established, rather than calling registerDeviceCapabilityEntityLink again (which would
	// otherwise flag this as a copy-paste "declared twice" duplicate).
	deviceCapabilityKey := deviceID + "!" + bareCapability
	entityID, alreadyResolved := administration.NamedTriggerEntityIDByDeviceCapability[deviceCapabilityKey]
	if !alreadyResolved {
		capDecl := TDeviceCapabilityEntityDeclaration{LocalSpec: localSpec, DeviceID: deviceID, Capability: decl.Capability}
		warnings, deferred := registerDeviceCapabilityEntityLink(administration, capDecl, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID, entitiesPath, lineNum, true, "", false)
		if deferred || len(warnings) > 0 {
			return warnings
		}
		entityID = toHomeAssistantEntityID(resolveDeviceEntityFullName(localSpec, administration.SpacePath, ""))
		administration.NamedTriggerEntityIDByDeviceCapability[deviceCapabilityKey] = entityID
	}

	resolvedName := normalizeEntityFullName(decl.Name, administration.SpacePath)
	for _, existing := range administration.NamedTriggers[resolvedName] {
		if existing.EntityID == entityID {
			return []string{fmt.Sprintf("%s line %d: trigger %q: device %q declares this named trigger more than once", entitiesPath, lineNum, decl.Name, deviceID)}
		}
	}
	administration.NamedTriggers[resolvedName] = append(administration.NamedTriggers[resolvedName], TNamedTriggerSource{
		EntityID:          entityID,
		EventTypes:        decl.EventTypes,
		TriggerAttr:       decl.TriggerAttr,
		TriggerAttrValues: decl.TriggerAttrValues,
	})
	return nil
}
