/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Checks tests
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 15.10.2026
 *
 */

package main

import (
	"strings"
	"testing"
)

// TestRuleReferencingSphereLevelAggregateEntityDoesNotWarn is a regression test for a real gap
// found live 2026-10-15 on Vienna: generator.go's generateTemplateLights/generateTemplateSwitches
// synthesize sphere-level aggregate entities (light.<sphere>, switch.<sphere>/space,
// switch.<sphere>/media) directly from SpaceOnByName["root"]/SpaceOffByName["root"] in their own
// later generation pass -- those were never added to EntityRecordsBySpace via the normal
// deriveEntity path every other derived entity goes through, so a rule referencing one by its
// absolute path (e.g. Vienna's real "social:/leaving" rule's "turn switch.social:/space off;")
// always warned "undeclared", even though the entity is real, correctly generated, and live.
func TestRuleReferencingSphereLevelAggregateEntityDoesNotWarn(t *testing.T) {
	const dsl = `space social:apartment with:
  space social:hallway as area with:
    entity light.social:main;
    light on: social:main;

    rule social:/leaving with:
      when event.physical:door is 1_single
      then turn switch.social:/space off;
    end;
  end;
end;`

	var report strings.Builder
	output := captureStdout(t, func() {
		_, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
	})
	if strings.Contains(output, "switch.social/space") {
		t.Errorf("expected no \"undeclared entity\" warning for the sphere-level aggregate switch, got:\n%s", output)
	}
}

// TestRuleReferencingTimeWindowDayTimeSensorDoesNotWarn is a regression test for the same class
// of gap as above, for generateTimeWindowBinarySensors' own synthesized "is_<domain>_day_time"
// binary sensors (real case: Vienna's vacuum/retry_charging rule's "if
// binary_sensor.social:/is_vacuuming_day_time is on and ...;").
func TestRuleReferencingTimeWindowDayTimeSensorDoesNotWarn(t *testing.T) {
	const dsl = `space social:apartment with:
  entity vacuum.social:x;

  rule social:/vacuum/retry_charging with:
    when vacuum.social:x turns error
    if binary_sensor.social:/is_vacuuming_day_time is on
    then turn vacuum.social:x on;
  end;
end;`

	var report strings.Builder
	output := captureStdout(t, func() {
		_, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
	})
	if strings.Contains(output, "is_vacuuming_day_time") {
		t.Errorf("expected no \"undeclared entity\" warning for the is_vacuuming_day_time sensor, got:\n%s", output)
	}
}
