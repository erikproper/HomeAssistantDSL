package main

import (
	"strings"
	"testing"
)

// TestValidateNoDuplicateFinalEntityIDsCatchesDotUnderscoreCollision is a regression test for a
// real bug found live 2026-09-16 (Vienna's "node.fritz_box"/"node.fritz.box" collision): two
// entirely different DSL entity names that sanitize to the identical HA entity_id must be caught,
// even though every earlier device-name check (validateNoConflictingDeviceNames,
// AppendEntityRecord's own same-space/same-name dedup) is blind to it.
func TestValidateNoDuplicateFinalEntityIDsCatchesDotUnderscoreCollision(t *testing.T) {
	admin := newAdministrationState()
	admin.AppendEntityRecord("root", TEntityRecord{Name: "binary_sensor.infrastructural/apartment/living_room/fritz_box/node", Provenance: "Conceptual.def:151 -> node.fritz_box"})
	admin.AppendEntityRecord("root", TEntityRecord{Name: "binary_sensor.infrastructural/apartment/living_room/fritz.box/node", Provenance: "Conceptual.def:318 -> node.fritz.box"})

	warnings := validateNoDuplicateFinalEntityIDs(admin)
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "binary_sensor.infrastructural_apartment_living_room_fritz_box_node") {
		t.Errorf("warning = %q, want it to name the colliding entity_id", warnings[0])
	}
	if !strings.Contains(warnings[0], "fritz_box/node") || !strings.Contains(warnings[0], "fritz.box/node") {
		t.Errorf("warning = %q, want it to name both original declarations", warnings[0])
	}
}

// TestValidateNoDuplicateFinalEntityIDsNoFalsePositiveForGenuineSingleEntity confirms an ordinary,
// uncontested entity produces no warning.
func TestValidateNoDuplicateFinalEntityIDsNoFalsePositiveForGenuineSingleEntity(t *testing.T) {
	admin := newAdministrationState()
	admin.AppendEntityRecord("root", TEntityRecord{Name: "binary_sensor.infrastructural/apartment/living_room/apple_tv/node"})

	if warnings := validateNoDuplicateFinalEntityIDs(admin); len(warnings) != 0 {
		t.Errorf("unexpected warnings for a single, uncontested entity: %v", warnings)
	}
}

// TestValidateNoDuplicateFinalEntityIDsCatchesIdenticalNameAcrossSpaces covers the OTHER half of
// this gap: AppendEntityRecord's own dedup only compares names WITHIN one space, so the exact same
// entity name declared independently in two different spaces was ALSO silently uncaught before
// this rule existed -- an entity's own full name is meant to be globally unique, so two
// occurrences anywhere is always a mistake.
func TestValidateNoDuplicateFinalEntityIDsCatchesIdenticalNameAcrossSpaces(t *testing.T) {
	admin := newAdministrationState()
	admin.EntityRecordsBySpace["a"] = append(admin.EntityRecordsBySpace["a"], TEntityRecord{Name: "sensor.social/x", Provenance: "Conceptual.def:10"})
	admin.EntityRecordsBySpace["b"] = append(admin.EntityRecordsBySpace["b"], TEntityRecord{Name: "sensor.social/x", Provenance: "Conceptual.def:99"})

	warnings := validateNoDuplicateFinalEntityIDs(admin)
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "sensor.social_x") {
		t.Errorf("warning = %q, want it to name the colliding entity_id", warnings[0])
	}
}
