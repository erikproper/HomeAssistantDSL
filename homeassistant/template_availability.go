/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: TemplateAvailability
 *
 * PROJECT.md item 17b (2026-10-25): a generated template entity gets an "availability:" from its
 * sources, as HA's own groups already do. A derived value (a template sensor, or a binary_sensor
 * derived from values) is available only while all its sources have a value -- no misleading 0
 * from "| float(0)". A template aggregate (space light, space/media switch) is available while any
 * member is ("all" is a later choice). An expression that tests availability itself (a node, an
 * "is available" condition, a battery alert) gets none: it has to say "off" exactly when a source
 * is gone.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 25.10.2026
 *
 */

package main

import (
	"regexp"
	"slices"
	"strings"
)

// templateSourcePattern finds the entity_ids a rendered Jinja expression reads.
var templateSourcePattern = regexp.MustCompile(`\b(?:states|is_state|is_state_attr|state_attr|has_value)\('([a-z_]+\.[a-z0-9_]+)'`)

// templateSourceIDs returns the entity_ids expr reads, each once, in order of appearance.
func templateSourceIDs(expr string) []string {
	var ids []string
	for _, m := range templateSourcePattern.FindAllStringSubmatch(expr, -1) {
		if !slices.Contains(ids, m[1]) {
			ids = append(ids, m[1])
		}
	}
	return ids
}

// expressesAvailability reports whether expr tests availability itself.
func expressesAvailability(expr string) bool {
	return strings.Contains(expr, "'unavailable'") || strings.Contains(expr, "'unknown'") || strings.Contains(expr, "has_value(")
}

// derivedValueAvailability is the availability of an entity derived from expr's values: all
// sources need a value. "" when expr reads no entity or tests availability itself.
func derivedValueAvailability(expr string) string {
	ids := templateSourceIDs(expr)
	if len(ids) == 0 || expressesAvailability(expr) {
		return ""
	}
	tests := make([]string, len(ids))
	for i, id := range ids {
		tests[i] = "has_value('" + id + "')"
	}
	return "{{ " + strings.Join(tests, " and ") + " }}"
}

// aggregateAvailability is the availability of a template aggregate of memberIDs: any member
// available, or with all, every member.
func aggregateAvailability(memberIDs []string, all bool) string {
	if len(memberIDs) == 0 {
		return ""
	}
	quoted := make([]string, len(memberIDs))
	for i, id := range memberIDs {
		quoted[i] = "'" + id + "'"
	}
	list := "[" + strings.Join(quoted, ", ") + "]"
	if all {
		return "{{ " + list + " | reject('has_value') | list | count == 0 }}"
	}
	return "{{ " + list + " | select('has_value') | list | count > 0 }}"
}

// appendAvailability writes availability (if any) as the entity's "availability:" line.
func appendAvailability(sb *strings.Builder, availability string) {
	if availability != "" {
		sb.WriteString("  availability: \"" + availability + "\"\n")
	}
}
