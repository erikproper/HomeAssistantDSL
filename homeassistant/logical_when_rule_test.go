package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseRuleTurnsOn confirms the basic "rule <spec> with: when <event> turns on; then turn
// <action> off; end;" block shape populates a TRule correctly (PROJECT.md item 13 Phase 1 --
// the new three-layer CDL1-style parser, replacing the old ad hoc one-liner TWhenRule it
// mechanically matches in scope: one trigger, one action, no conditions).
func TestParseRuleTurnsOn(t *testing.T) {
	const dsl = `space social:wc as area with:
  rule social:done_removing_smells with:
    when binary_sensor.social:done_removing turns on
    then turn fan.social off;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration
	if len(admin.Rules) != 1 {
		t.Fatalf("got %d Rules, want 1", len(admin.Rules))
	}
	rule := admin.Rules[0]
	if rule.Name != "social/wc/done_removing_smells" {
		t.Errorf("Name = %q, want %q", rule.Name, "social/wc/done_removing_smells")
	}
	if len(rule.Triggers) != 1 {
		t.Fatalf("got %d Triggers, want 1", len(rule.Triggers))
	}
	if rule.Triggers[0].EntitySpec != "binary_sensor.social/wc/done_removing" {
		t.Errorf("Triggers[0].EntitySpec = %q, want %q", rule.Triggers[0].EntitySpec, "binary_sensor.social/wc/done_removing")
	}
	if rule.Triggers[0].State != "on" {
		t.Errorf("Triggers[0].State = %q, want \"on\"", rule.Triggers[0].State)
	}
	if len(rule.Actions) != 1 {
		t.Fatalf("got %d Actions, want 1", len(rule.Actions))
	}
	if rule.Actions[0].EntitySpec != "fan.social/wc" {
		t.Errorf("Actions[0].EntitySpec = %q, want %q", rule.Actions[0].EntitySpec, "fan.social/wc")
	}
	if rule.Actions[0].State != "off" {
		t.Errorf("Actions[0].State = %q, want \"off\"", rule.Actions[0].State)
	}
}

// TestParseRuleMalformedStatementsAreSkipped confirms a handful of near-miss "when"/"then"
// statement shapes inside a rule block are rejected (via the parser's own warnings), not
// silently turned into a bogus trigger/action.
func TestParseRuleMalformedStatementsAreSkipped(t *testing.T) {
	tests := []string{
		"when fan.social;",            // missing "turns <state>" or "is <event-type>"
		"then frobnicate fan.social;", // not a recognised action verb
		"then turn fan.social;",       // missing state
	}
	for _, line := range tests {
		t.Run(line, func(t *testing.T) {
			dsl := "space social:wc as area with:\n  rule social:x with:\n    " + line + "\n  end;\nend;"
			var report strings.Builder
			result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if len(result.Administration.Rules) != 1 {
				t.Fatalf("got %d Rules, want 1 (the rule itself still closes cleanly)", len(result.Administration.Rules))
			}
			rule := result.Administration.Rules[0]
			if len(rule.Triggers) != 0 || len(rule.Actions) != 0 {
				t.Errorf("malformed statement %q was still recorded: %+v", line, rule)
			}
		})
	}
}

// TestParseRuleOutsideBlockWarns confirms a bare "when ...;"/"then ...;" outside any "rule <spec>
// with: ...; end;" block is rejected with an actionable warning rather than silently dropped.
func TestParseRuleOutsideBlockWarns(t *testing.T) {
	const dsl = `space social:wc as area with:
  when binary_sensor.social:done_removing turns on;
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
	if len(result.Administration.Rules) != 0 {
		t.Errorf("got %d Rules for a stray when-statement, want 0", len(result.Administration.Rules))
	}
	if !strings.Contains(output, "outside a") || !strings.Contains(output, "rule <spec> with:") {
		t.Errorf("expected a warning naming the fix, got: %q", output)
	}
}

// TestParseRuleDuplicateNameWarns confirms two rules resolving to the same Name trigger a
// warning (RuleNamesSeen) -- both still get recorded (last-writer-wins is the generator's own
// concern, e.g. writeAutomationFile overwriting the same path), but the collision itself must be
// surfaced, not silently accepted.
func TestParseRuleDuplicateNameWarns(t *testing.T) {
	const dsl = `space social:wc as area with:
  rule social:done_removing_smells with:
    when binary_sensor.social:done_removing turns on
    then turn fan.social off;
  end;
  rule social:done_removing_smells with:
    when binary_sensor.social:done_removing turns off
    then turn fan.social on;
  end;
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
	if len(result.Administration.Rules) != 2 {
		t.Fatalf("got %d Rules, want 2 (both still recorded, just warned about)", len(result.Administration.Rules))
	}
	if !strings.Contains(output, "[WARNING]") || !strings.Contains(output, "social/wc/done_removing_smells") || !strings.Contains(output, "more than once") {
		t.Errorf("expected a duplicate rule-name warning naming the repeated name, got: %q", output)
	}
}

// TestGenerateRuleAutomationsWritesExpectedYAML confirms the generated automation's alias/id
// (the rule's own explicit Name) and trigger/action shape match what a state-transition rule
// needs -- unchanged output shape from the old TWhenRule construct (buildStateTriggerAutomationYAML
// is still shared, unmodified), just produced from the new TRule/CDL1-parsed data model.
func TestGenerateRuleAutomationsWritesExpectedYAML(t *testing.T) {
	const dsl = `space social:wc as area with:
  rule social:done_removing_smells with:
    when binary_sensor.social:done_removing turns on
    then turn fan.social off;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	outDir := t.TempDir()
	if err := generateRuleAutomations(outDir, result.Administration); err != nil {
		t.Fatalf("generateRuleAutomations: %v", err)
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
// generateFollowsAutomations refactor (2026-09-29): it delegates its actual YAML-building to
// buildStateTriggerAutomationYAML, the same helper generateRuleAutomations uses -- this confirms
// that refactor (and the later TWhenRule -> TRule/CDL1-parser swap) didn't change follows' own
// output (still exactly 2 automations, still the same alias/trigger/action shape it always had)
// even though the code is shared.
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
