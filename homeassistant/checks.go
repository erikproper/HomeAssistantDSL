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

// buildDeclaredEntitySet collects every entity name registered across all spaces into a lookup set.
func buildDeclaredEntitySet(administration *TAdministrationState) map[string]bool {
	declared := map[string]bool{}
	for _, records := range administration.EntityRecordsBySpace {
		for _, rec := range records {
			declared[rec.Name] = true
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
}
