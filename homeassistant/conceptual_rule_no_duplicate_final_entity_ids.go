/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Conceptual-layer hard-enforcement rules
 *
 * Real bug found live 2026-09-16 (Vienna): "node.fritz_box" (a "home_assistant" bridge device,
 * Physical.def) and "node.fritz.box" (an unrelated "hosts"/ping device for the SAME physical
 * router) are two entirely different Physical.def declarations, positioned independently in
 * Conceptual.def ("device infrastructural:fritz_box from node.fritz_box with: ...;" and "device
 * infrastructural:fritz.box from node.fritz.box;") -- their own DSL-level entity names differ by a
 * single character ("fritz_box" vs "fritz.box"), so neither AppendEntityRecord's own
 * same-space/same-name dedup (administration.go) nor validateNoConflictingDeviceNames
 * (physical_rule_no_conflicting_device_names.go, which only compares DEVICE ids, never their
 * downstream entity names) ever saw them as related at all. But toHomeAssistantEntityID
 * (defined.go) sanitizes BOTH "." and "_" down to the same "_" -- so the two entity NAMES resolve
 * to the exact same final HA entity_id, and HA's own MQTT discovery silently gives the SECOND one
 * ever created a "_2"-suffixed duplicate instead, permanently, with no error or warning from this
 * generator at all. The user's own words: "That should have resulted in an error/warning."
 *
 * validateNoDuplicateFinalEntityIDs closes that gap: run once, after Conceptual.def parsing is
 * fully complete (every kind's own positioning/capability registration has already happened), over
 * the FINAL, fully-resolved entity set -- the one property no earlier, kind-specific check can see
 * on its own, since the collision here is a property of the SANITIZED ENTITY_ID, not of anything
 * either declaration knows about itself.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 16.09.2026
 *
 */

package main

import (
	"fmt"
	"sort"
)

// validateNoDuplicateFinalEntityIDs scans every entity record this generation run produced
// (admin.EntityRecordsBySpace, already fully populated by the time this runs) and warns whenever
// more than one entity record sanitizes to the identical final HA entity_id -- regardless of
// which space they're in, which kind produced them, or whether their own DSL-level names (rec.Name)
// happen to differ (the fritz_box/fritz.box case) or be identical (AppendEntityRecord's own
// same-space/same-name dedup already refuses a second identical name WITHIN one space, but two
// spaces each declaring the exact same entity name independently is a real, equally silent
// collision it can't see either -- an entity's own full name is meant to be globally unique across
// the whole house, so two occurrences of it, anywhere, always indicates a mistake). Deliberately
// keyed on the SANITIZED id, not the raw name, since that's the one thing no earlier check compares.
func validateNoDuplicateFinalEntityIDs(admin *TAdministrationState) []string {
	type occurrence struct {
		name       string
		provenance string
	}
	bySanitizedID := map[string][]occurrence{}

	for _, records := range admin.EntityRecordsBySpace {
		for _, rec := range records {
			id := toHomeAssistantEntityID(rec.Name)
			if id == "" {
				continue
			}
			bySanitizedID[id] = append(bySanitizedID[id], occurrence{name: rec.Name, provenance: rec.Provenance})
		}
	}

	sanitizedIDs := make([]string, 0, len(bySanitizedID))
	for id := range bySanitizedID {
		sanitizedIDs = append(sanitizedIDs, id)
	}
	sort.Strings(sanitizedIDs)

	var warnings []string
	for _, id := range sanitizedIDs {
		occurrences := bySanitizedID[id]
		if len(occurrences) < 2 {
			continue
		}
		var detail string
		for _, occ := range occurrences {
			detail += fmt.Sprintf("\n  %s (%s)", occ.name, provenanceLabel(occ.provenance))
		}
		warnings = append(warnings, fmt.Sprintf("entity id %q is produced by %d separate declarations -- HA will silently give all but one of them a \"_2\"-suffixed duplicate:%s", id, len(occurrences), detail))
	}
	return warnings
}
