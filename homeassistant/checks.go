/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Checks
 *
 * Post-parse validation: verifies that entity references in directive lists
 * (heating leak:, space off:, light on:, light off:) are subsets of the
 * declared entities in the administration state.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 25.03.2026
 *
 */

package main

import (
	"sort"
	"strings"
)

// --- post-parse subset checks ---

// buildDeclaredEntitySet collects every entity name registered across all spaces into a lookup
// set, plus the sphere-level aggregate entities (light.<sphere>, switch.<sphere>/space,
// switch.<sphere>/media) that generator.go's own generateTemplateLights/generateTemplateSwitches/
// generateTemplateMediaSwitches synthesize directly from SpaceOnByName["root"]/
// SpaceOffByName["root"] at YAML-write time -- those were never added to EntityRecordsBySpace at
// all (a separate, later generation pass, not the normal per-space deriveEntity path every other
// derived entity goes through), so a rule referencing one (e.g. Vienna's "social:/leaving" ->
// switch.social:/space) always warned "undeclared", even though the entity is real and correctly
// generated. Real gap found live 2026-10-15 -- mirrors generator.go's own grouping conditions
// exactly (same three conditions, kept in sync by hand since the two can't easily share one
// computation: this runs before generation, that's generation itself).
func buildDeclaredEntitySet(administration *TAdministrationState) map[string]bool {
	declared := map[string]bool{}
	for _, records := range administration.EntityRecordsBySpace {
		for _, rec := range records {
			declared[rec.Name] = true
		}
	}

	for _, entry := range administration.SpaceOnByName["root"] {
		if sphere := extractEntityIdentity(entry).Sphere; sphere != "" {
			declared["light."+sphere] = true
		}
	}
	for _, entry := range administration.SpaceOffByName["root"] {
		sphere := extractEntityIdentity(entry).Sphere
		if sphere == "" {
			continue
		}
		if strings.HasSuffix(entry, "/space") {
			declared["switch."+sphere+"/space"] = true
		}
		if strings.HasSuffix(entry, "/media") {
			declared["switch."+sphere+"/media"] = true
		}
	}

	// Same gap, same reason, for the "is_<domain>_day_time"/"is_coverage_time" binary sensors
	// generateTimeWindowBinarySensors (generator.go) synthesizes from any "<domain>_workday/
	// start_day" input_datetime pair it finds -- mirrors that function's own domain-discovery
	// scan exactly. Real gap found live 2026-10-15 (Vienna's vacuum/retry_charging rule's own
	// "if binary_sensor.social:/is_vacuuming_day_time is on ...;").
	for _, spaceName := range administration.SpaceOrder {
		for _, rec := range administration.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain != "input_datetime" || !strings.HasSuffix(rec.Identity.Path, "_workday/start_day") {
				continue
			}
			domain := strings.TrimSuffix(rec.Identity.Path, "_workday/start_day")
			if domain == "" || rec.Identity.Sphere == "" {
				continue
			}
			if domain == "coverage" {
				declared["binary_sensor."+rec.Identity.Sphere+"/is_coverage_time"] = true
			} else {
				declared["binary_sensor."+rec.Identity.Sphere+"/is_"+domain+"_day_time"] = true
			}
		}
	}

	return declared
}

// checkDirectiveEntityRefs reports any entity reference in listsBySpace that is not present
// in the declared set. Aggregation tokens starting with '@' are skipped.
func checkDirectiveEntityRefs(directiveName string, listsBySpace map[string][]string, declared map[string]bool) {
	// Sort space names for deterministic output order.
	spaces := make([]string, 0, len(listsBySpace))
	for spaceName := range listsBySpace {
		spaces = append(spaces, spaceName)
	}
	sort.Strings(spaces)

	for _, spaceName := range spaces {
		for _, ref := range listsBySpace[spaceName] {
			if strings.HasPrefix(ref, "@") {
				continue // aggregation tokens (@light, @media, @all) are expanded, not checked here
			}
			if !declared[ref] {
				Reporter.Warn("Space %q: %s references undeclared entity %q",
					spaceName, directiveName, ref)
			}
		}
	}
}

// checkRelationEntityRef reports a warning if the entity name is not in the declared set.
func checkRelationEntityRef(directive, spaceName, entityName string, declared map[string]bool) {
	if !declared[entityName] {
		Reporter.Warn("Space %q: %s references undeclared entity %q", spaceName, directive, entityName)
	}
}

// RunPostParseChecks validates that directive lists and relation records only reference
// entities that have been declared in the administration state.
// Called after all parsing and entity registration is done.
func RunPostParseChecks(administration *TAdministrationState) {
	declared := buildDeclaredEntitySet(administration)

	checkDirectiveEntityRefs("heating leak:", administration.HeatingLeaksByName, declared)
	checkDirectiveEntityRefs("space off:", administration.SpaceOffByName, declared)
	checkDirectiveEntityRefs("light on:", administration.SpaceOnByName, declared)
	checkDirectiveEntityRefs("light off:", administration.SpaceLightOffByName, declared)

	for _, rel := range administration.FollowsRelations {
		checkRelationEntityRef("follows (follower)", rel.SpaceName, rel.Follower, declared)
		checkRelationEntityRef("follows (leader)", rel.SpaceName, rel.Leader, declared)
	}

	for _, rel := range administration.SwitchedDeviceRelations {
		checkRelationEntityRef("switched_device (device)", rel.SpaceName, rel.Device, declared)
		checkRelationEntityRef("switched_device (main)", rel.SpaceName, rel.MainEntity, declared)
	}

	for _, rel := range administration.TimerLimitsRelations {
		checkRelationEntityRef("timer limits (timer)", rel.SpaceName, rel.TimerEntity, declared)
		checkRelationEntityRef("timer limits (bound)", rel.SpaceName, rel.BoundEntity, declared)
	}

	checkNodeSpheres(declared)

	for _, rule := range administration.Rules {
		for _, trigger := range rule.Triggers {
			if trigger.Kind == "named_trigger" {
				if len(administration.NamedTriggers[trigger.Name]) == 0 {
					Reporter.Warn("Space %q: rule (when) references event %q, which nothing raises",
						rule.SpaceName, trigger.Name)
				}
				continue
			}
			if trigger.EntitySpec == "" {
				continue // "time"/"time_pattern" triggers name no entity at all
			}
			checkRelationEntityRef("rule (when)", rule.SpaceName, trigger.EntitySpec, declared)
		}
		if rule.Condition != nil {
			checkRuleConditionEntityRef(rule.SpaceName, *rule.Condition, declared)
		}
		for _, action := range rule.Actions {
			checkRuleActionEntityRef(rule.SpaceName, action, declared)
		}
	}
}

// checkNodeSpheres warns about every node outside the "infrastructural" sphere (2026-10-26, Erik):
// a node is always about the house's own platform, never about the house itself ("social") or a
// raw building block of the logical layer ("physical").
func checkNodeSpheres(declared map[string]bool) {
	var names []string
	for name := range declared {
		identity := extractEntityIdentity(name)
		if identity.Domain == "binary_sensor" && identity.Sphere != "infrastructural" && (identity.Path == "node" || strings.HasSuffix(identity.Path, "/node")) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		Reporter.Warn("node %q is not in the \"infrastructural\" sphere", name)
	}
}

// checkRuleConditionEntityRef checks a TRuleCondition tree's own entity references -- a
// "template"/"time" condition names no real entity at all, so only "state" leaves are checked;
// "and"/"or"/"not" nodes recurse into their own Operands.
func checkRuleConditionEntityRef(spaceName string, cond TRuleCondition, declared map[string]bool) {
	if cond.Kind == "state" {
		checkRelationEntityRef("rule (if)", spaceName, cond.EntitySpec, declared)
	}
	for _, operand := range cond.Operands {
		checkRuleConditionEntityRef(spaceName, operand, declared)
	}
}

// checkRuleActionEntityRef checks a TRuleAction's own entity reference -- "delay" names no
// entity at all, and "if" names its condition's entity instead (CondEntitySpec) and recurses
// into its own ThenActions/ElseActions, same as this function's own top-level caller does for a
// rule's flat Actions list.
func checkRuleActionEntityRef(spaceName string, action TRuleAction, declared map[string]bool) {
	switch action.Verb {
	case "delay":
		return
	case "if":
		checkRelationEntityRef("rule (then if)", spaceName, action.CondEntitySpec, declared)
		for _, sub := range action.ThenActions {
			checkRuleActionEntityRef(spaceName, sub, declared)
		}
		for _, sub := range action.ElseActions {
			checkRuleActionEntityRef(spaceName, sub, declared)
		}
	default:
		checkRelationEntityRef("rule (then)", spaceName, action.EntitySpec, declared)
	}
}
