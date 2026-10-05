package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseWhenRuleTurnsOn confirms the basic "rule <spec> when <event> turns on then turn
// <action> on;" shape populates a TWhenRule correctly, including the new mandatory rule Name
// (2026-09-29) resolved exactly like a domain-less entity leaf.
func TestParseWhenRuleTurnsOn(t *testing.T) {
	const dsl = `space social:wc as area with:
  rule social:done_removing_smells when binary_sensor.social:done_removing turns on then turn fan.social off;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration
	if len(admin.WhenRules) != 1 {
		t.Fatalf("got %d WhenRules, want 1", len(admin.WhenRules))
	}
	rule := admin.WhenRules[0]
	if rule.Name != "social/wc/done_removing_smells" {
		t.Errorf("Name = %q, want %q", rule.Name, "social/wc/done_removing_smells")
	}
	if rule.EventEntity != "binary_sensor.social/wc/done_removing" {
		t.Errorf("EventEntity = %q, want %q", rule.EventEntity, "binary_sensor.social/wc/done_removing")
	}
	if rule.EventState != "on" {
		t.Errorf("EventState = %q, want \"on\"", rule.EventState)
	}
	if rule.ActionEntity != "fan.social/wc" {
		t.Errorf("ActionEntity = %q, want %q", rule.ActionEntity, "fan.social/wc")
	}
	if rule.ActionState != "off" {
		t.Errorf("ActionState = %q, want \"off\"", rule.ActionState)
	}
}

// TestParseWhenRuleMalformedLinesAreSkipped confirms a handful of near-miss shapes are silently
// ignored (matching "follows"/"limits" own established local precedent) rather than crashing or
// producing a bogus WhenRule.
func TestParseWhenRuleMalformedLinesAreSkipped(t *testing.T) {
	tests := []string{
		`rule social:x when fan.social turns sideways then turn fan.social on;`,
		`rule social:x when fan.social then turn fan.social on;`,
		`rule social:x when fan.social turns on then toggle fan.social;`,
		`rule social:x when fan.social turns on turn fan.social on;`, // missing "then"
		`rule when fan.social turns on then turn fan.social on;`,    // missing rule name
	}
	for _, line := range tests {
		t.Run(line, func(t *testing.T) {
			dsl := "space social:wc as area with:\n  " + line + "\nend;"
			var report strings.Builder
			result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if len(result.Administration.WhenRules) != 0 {
				t.Errorf("got %d WhenRules for malformed line %q, want 0", len(result.Administration.WhenRules), line)
			}
		})
	}
}

// TestParseWhenRuleBareFormRetiredWarns confirms a bare "when ...;" (no leading "rule <spec>",
// the form this construct shipped with for one day before the mandatory name landed) is rejected
// with an actionable warning rather than silently dropped.
func TestParseWhenRuleBareFormRetiredWarns(t *testing.T) {
	const dsl = `space social:wc as area with:
  when binary_sensor.social:done_removing turns on then turn fan.social off;
end;`

	var report strings.Builder
	var result TExpansionParseResult
	var err error
	output := captureStderr(t, func() {
		result, err = ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
	})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(result.Administration.WhenRules) != 0 {
		t.Errorf("got %d WhenRules for a bare (unnamed) when-rule, want 0", len(result.Administration.WhenRules))
	}
	if !strings.Contains(output, "retired") || !strings.Contains(output, "rule <sphere:leaf> when") {
		t.Errorf("expected a retired-form warning naming the fix, got: %q", output)
	}
}

// TestParseWhenRuleDuplicateNameWarns confirms two rules resolving to the same Name trigger a
// warning (WhenRuleNamesSeen) -- both still get recorded (last-writer-wins is the generator's own
// concern, e.g. writeAutomationFile overwriting the same path), but the collision itself must be
// surfaced, not silently accepted.
func TestParseWhenRuleDuplicateNameWarns(t *testing.T) {
	const dsl = `space social:wc as area with:
  rule social:done_removing_smells when binary_sensor.social:done_removing turns on then turn fan.social off;
  rule social:done_removing_smells when binary_sensor.social:done_removing turns off then turn fan.social on;
end;`

	var report strings.Builder
	var result TExpansionParseResult
	var err error
	output := captureStderr(t, func() {
		result, err = ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
	})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(result.Administration.WhenRules) != 2 {
		t.Fatalf("got %d WhenRules, want 2 (both still recorded, just warned about)", len(result.Administration.WhenRules))
	}
	if !strings.Contains(output, "[WARNING]") || !strings.Contains(output, "social/wc/done_removing_smells") || !strings.Contains(output, "more than once") {
		t.Errorf("expected a duplicate rule-name warning naming the repeated name, got: %q", output)
	}
}

// TestGenerateWhenRuleAutomationsWritesExpectedYAML confirms the generated automation's
// alias/id (now the rule's own explicit Name, 2026-09-29) and trigger/action shape match what a
// state-transition "when" rule needs.
func TestGenerateWhenRuleAutomationsWritesExpectedYAML(t *testing.T) {
	const dsl = `space social:wc as area with:
  rule social:done_removing_smells when binary_sensor.social:done_removing turns on then turn fan.social off;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	outDir := t.TempDir()
	if err := generateWhenRuleAutomations(outDir, result.Administration); err != nil {
		t.Fatalf("generateWhenRuleAutomations: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(outDir, "automation", "*", "automation.*.yaml"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("got %d automation files, want 1: %v", len(matches), matches)
	}
	content, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(content)
	wantEventID := "binary_sensor.social_wc_done_removing"
	wantActionID := "fan.social_wc"
	for _, want := range []string{
		"- alias: social/wc/done_removing_smells\n",
		"id: automation.social_wc_done_removing_smells\n",
		"trigger:\n  - platform: state\n    entity_id: " + wantEventID + "\n    to: 'on'\n",
		"action:\n  - service: fan.turn_off\n    entity_id: " + wantActionID + "\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated automation missing %q, got:\n%s", want, got)
		}
	}
}

// TestFollowsStillProducesTwoAutomationsViaSharedBuilder is the regression test for the
// generateFollowsAutomations refactor (2026-09-29): it now delegates its actual YAML-building to
// buildStateTriggerAutomationYAML, the same helper generateWhenRuleAutomations uses -- this
// confirms that refactor didn't change follows' own output (still exactly 2 automations, still
// the same alias/trigger/action shape it always had) even though the code is now shared.
func TestFollowsStillProducesTwoAutomationsViaSharedBuilder(t *testing.T) {
	const dsl = `space social:corridor as area with:
  follows light.social:corb light.social:main;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	outDir := t.TempDir()
	if err := generateFollowsAutomations(outDir, result.Administration); err != nil {
		t.Fatalf("generateFollowsAutomations: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(outDir, "automation", "*", "automation.*.yaml"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("got %d automation files, want 2 (turn_on + turn_off): %v", len(matches), matches)
	}
	var sawOn, sawOff bool
	for _, m := range matches {
		content, err := os.ReadFile(m)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		got := string(content)
		if strings.Contains(got, "to: 'on'\n") && strings.Contains(got, "service: light.turn_on\n") {
			sawOn = true
		}
		if strings.Contains(got, "to: 'off'\n") && strings.Contains(got, "service: light.turn_off\n") {
			sawOff = true
		}
	}
	if !sawOn || !sawOff {
		t.Errorf("expected one turn_on and one turn_off automation, sawOn=%v sawOff=%v", sawOn, sawOff)
	}
}
