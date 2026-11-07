package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestParseRuleEventTriggerWithTemplateConditionAndDataField mirrors a real Vienna automation
// (manual:social/apartment/living_room/main/light/turn_on) -- an event-received trigger, a
// template condition against the firing trigger's own context, and a "turn ... with ..." data
// field.
func TestParseRuleEventTriggerWithTemplateConditionAndDataField(t *testing.T) {
	const dsl = `space social:apartment/living_room as area with:
  rule social:apartment/living_room/main/light/turn_on with:
    when event.physical:apartment/living_room/signify_dimmer is press
    if trigger.button is "on"
    then turn light.social:apartment/living_room/main on with brightness_pct 70;
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

	if len(rule.Triggers) != 1 || rule.Triggers[0].Kind != "event" || rule.Triggers[0].EventType != "press" {
		t.Fatalf("Triggers = %+v, want one event trigger with EventType \"press\"", rule.Triggers)
	}
	if rule.Condition == nil || rule.Condition.Kind != "template" || rule.Condition.TriggerAttr != "button" || rule.Condition.Value != "on" {
		t.Fatalf("Condition = %+v, want one template condition on trigger.button == \"on\"", rule.Condition)
	}
	if len(rule.Actions) != 1 || rule.Actions[0].Verb != "turn" || rule.Actions[0].State != "on" || len(rule.Actions[0].Data) != 1 || rule.Actions[0].Data[0].Name != "brightness_pct" || rule.Actions[0].Data[0].Value != "70" {
		t.Fatalf("Actions = %+v, want one turn-on action with brightness_pct 70", rule.Actions)
	}

	outDir := t.TempDir()
	if err := generateRuleAutomations(outDir, admin); err != nil {
		t.Fatalf("generateRuleAutomations: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(outDir, "automation", "*", "automation.*.yaml"))
	if len(matches) != 1 {
		t.Fatalf("got %d automation files, want 1: %v", len(matches), matches)
	}
	content, _ := os.ReadFile(matches[0])
	got := string(content)
	for _, want := range []string{
		"  - trigger: event.received\n",
		"      event_type:\n      - 'press'\n",
		"  - condition: template\n",
		"    value_template: '{{ trigger.to_state.attributes.button == ''on'' }}'\n",
		"  - service: light.turn_on\n",
		"    data:\n      brightness_pct: 70\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated automation missing %q, got:\n%s", want, got)
		}
	}
}

// TestParseRuleMultipleActions mirrors a real Vienna automation (manual:social/leaving) --
// several ordered actions, no "if" clause.
func TestParseRuleMultipleActions(t *testing.T) {
	const dsl = `space social:apartment as area with:
  rule social:leaving with:
    when event.physical:apartment/hallway/door is 1_single
    then:
      turn input_boolean.social:occupation/around on;
      turn switch.social:space off;
    end;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	rule := result.Administration.Rules[0]
	if len(rule.Actions) != 2 {
		t.Fatalf("got %d Actions, want 2: %+v", len(rule.Actions), rule.Actions)
	}
	if rule.Actions[0].Verb != "turn" || rule.Actions[0].State != "on" {
		t.Errorf("Actions[0] = %+v, want turn-on", rule.Actions[0])
	}
	if rule.Actions[1].Verb != "turn" || rule.Actions[1].State != "off" {
		t.Errorf("Actions[1] = %+v, want turn-off", rule.Actions[1])
	}
}

// TestParseRuleIfThenElseAction mirrors a real Junglinster automation
// (manual:social/house/sarah_bedroom/cover/close) -- a "then if ...: ...; else: ...; end;"
// branching action with a Jinja-computed "set" value in the else branch.
func TestParseRuleIfThenElseAction(t *testing.T) {
	const dsl = `space social:house/sarah_bedroom as area with:
  rule social:house/sarah_bedroom/cover/close with:
    when event.physical:house/sarah_bedroom/door_tap is press_2
    then if cover.social:house/sarah_bedroom is closing:
      stop cover.social:house/sarah_bedroom;
    else:
      set cover.social:house/sarah_bedroom to jinja "100 - (states('number.social_house_sarah_bedroom_my_position') | int)";
    end;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(result.Administration.Rules) != 1 {
		t.Fatalf("got %d Rules, want 1", len(result.Administration.Rules))
	}
	rule := result.Administration.Rules[0]
	if len(rule.Actions) != 1 || rule.Actions[0].Verb != "if" {
		t.Fatalf("Actions = %+v, want one \"if\" action", rule.Actions)
	}
	ifAction := rule.Actions[0]
	if ifAction.CondState != "closing" {
		t.Errorf("CondState = %q, want \"closing\"", ifAction.CondState)
	}
	if len(ifAction.ThenActions) != 1 || ifAction.ThenActions[0].Verb != "stop" {
		t.Fatalf("ThenActions = %+v, want one \"stop\" action", ifAction.ThenActions)
	}
	if len(ifAction.ElseActions) != 1 || ifAction.ElseActions[0].Verb != "set" || !ifAction.ElseActions[0].ValueIsJinja {
		t.Fatalf("ElseActions = %+v, want one jinja \"set\" action", ifAction.ElseActions)
	}

	outDir := t.TempDir()
	if err := generateRuleAutomations(outDir, result.Administration); err != nil {
		t.Fatalf("generateRuleAutomations: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(outDir, "automation", "*", "automation.*.yaml"))
	if len(matches) != 1 {
		t.Fatalf("got %d automation files, want 1: %v", len(matches), matches)
	}
	content, _ := os.ReadFile(matches[0])
	got := string(content)
	// Real bug found live 2026-10-13, deploying this exact rule to Junglinster: an
	// unescaped "'" inside the jinja pass-through text broke the single-quoted YAML
	// scalar wrapping it, which passed this test's own string-matching checks below
	// (they don't care about YAML structure) but put the live house's HA core into
	// recovery mode on deploy. Parsing the generated file as YAML is the only check that
	// would have caught it.
	var parsed any
	if err := yaml.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("generated automation is not valid YAML: %v\ncontent:\n%s", err, got)
	}
	for _, want := range []string{
		"  - if:\n",
		"    state: 'closing'\n",
		"    then:\n",
		"    - service: cover.stop_cover\n",
		"    else:\n",
		"    - service: cover.set_cover_position\n",
		"      data:\n        position: '{{ 100 - (states(''number.social_house_sarah_bedroom_my_position'') | int) }}'\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated automation missing %q, got:\n%s", want, got)
		}
	}
}

// TestParseRuleIfAndOrNotExpression confirms the "if" clause's full boolean expression grammar
// (PROJECT.md item 13, 2026-10-13: a rule allows at most one "if" clause; multiple conditions
// combine via and/or/not and parentheses INSIDE that one clause, like any other boolean
// expression, rather than a second top-level clause or a separate block construct) -- "A and (B
// or not C);" with standard precedence.
func TestParseRuleIfAndOrNotExpression(t *testing.T) {
	const dsl = `space social:wc as area with:
  rule social:x with:
    when binary_sensor.social:a turns on
    if binary_sensor.social:b is on and
       (binary_sensor.social:c is on or not binary_sensor.social:d is on)
    then turn fan.social:x on;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(result.Administration.Rules) != 1 {
		t.Fatalf("got %d Rules, want 1", len(result.Administration.Rules))
	}
	rule := result.Administration.Rules[0]
	cond := rule.Condition
	if cond == nil || cond.Kind != "and" || len(cond.Operands) != 2 {
		t.Fatalf("Condition = %+v, want a top-level \"and\" with 2 operands", cond)
	}
	if cond.Operands[0].Kind != "state" || cond.Operands[0].EntitySpec != "binary_sensor.social/wc/b" {
		t.Errorf("Operands[0] = %+v, want state binary_sensor.social/wc/b is on", cond.Operands[0])
	}
	orNode := cond.Operands[1]
	if orNode.Kind != "or" || len(orNode.Operands) != 2 {
		t.Fatalf("Operands[1] = %+v, want a nested \"or\" with 2 operands", orNode)
	}
	if orNode.Operands[0].EntitySpec != "binary_sensor.social/wc/c" {
		t.Errorf("or.Operands[0] = %+v, want binary_sensor.social/wc/c", orNode.Operands[0])
	}
	notNode := orNode.Operands[1]
	if notNode.Kind != "not" || len(notNode.Operands) != 1 || notNode.Operands[0].EntitySpec != "binary_sensor.social/wc/d" {
		t.Errorf("or.Operands[1] = %+v, want not binary_sensor.social/wc/d", notNode)
	}

	outDir := t.TempDir()
	if err := generateRuleAutomations(outDir, result.Administration); err != nil {
		t.Fatalf("generateRuleAutomations: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(outDir, "automation", "*", "automation.*.yaml"))
	if len(matches) != 1 {
		t.Fatalf("got %d automation files, want 1: %v", len(matches), matches)
	}
	content, _ := os.ReadFile(matches[0])
	got := string(content)
	for _, want := range []string{
		"  condition:\n",
		"  - condition: state\n    entity_id: binary_sensor.social_wc_b\n    state: 'on'\n",
		"  - condition: or\n    conditions:\n",
		"    - condition: state\n      entity_id: binary_sensor.social_wc_c\n      state: 'on'\n",
		"    - condition: not\n      conditions:\n",
		"      - condition: state\n        entity_id: binary_sensor.social_wc_d\n        state: 'on'\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated automation missing %q, got:\n%s", want, got)
		}
	}
}

// TestParseRuleTimeConditionWeekday confirms "if time weekday <day>[, <day>]*;" parses into
// TRuleCondition.Weekdays and renders as HA's own "weekday:" list condition field (2026-10-13
// addition, needed for Junglinster's Saturday-only thermostat-calibration migration).
func TestParseRuleTimeConditionWeekday(t *testing.T) {
	const dsl = `space social:wc as area with:
  rule social:x with:
    when time is 09:00:00
    if time weekday sat, sun
    then press button.social:x;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(result.Administration.Rules) != 1 {
		t.Fatalf("got %d Rules, want 1", len(result.Administration.Rules))
	}
	cond := result.Administration.Rules[0].Condition
	if cond == nil || cond.Kind != "time" || len(cond.Weekdays) != 2 || cond.Weekdays[0] != "sat" || cond.Weekdays[1] != "sun" {
		t.Fatalf("Condition = %+v, want time condition with Weekdays [sat sun]", cond)
	}

	outDir := t.TempDir()
	if err := generateRuleAutomations(outDir, result.Administration); err != nil {
		t.Fatalf("generateRuleAutomations: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(outDir, "automation", "*", "automation.*.yaml"))
	if len(matches) != 1 {
		t.Fatalf("got %d automation files, want 1: %v", len(matches), matches)
	}
	content, _ := os.ReadFile(matches[0])
	got := string(content)
	if !strings.Contains(got, "    weekday:\n    - sat\n    - sun\n") {
		t.Errorf("generated automation missing weekday list, got:\n%s", got)
	}
}

// TestParseRuleEventTriggerQuotesYAMLBooleanishEventType is a regression test for a real bug
// found live 2026-10-14: Vienna's IKEA shortcut buttons report event_type "on" (and "off" for
// others), which renderRuleTriggerYAML wrote unquoted into the generated automation YAML --
// YAML 1.1's "Norway problem" then parses the bare word as the boolean true, which can never
// equal the real string Zigbee2MQTT reports, so the trigger silently never fired despite the
// MQTT message arriving correctly on the bus. Confirms the rendered line is actually valid YAML
// (via yaml.Unmarshal) AND that the event_type comes back as the Go string "on", not a bool.
func TestParseRuleEventTriggerQuotesYAMLBooleanishEventType(t *testing.T) {
	const dsl = `space social:wc as area with:
  rule social:x with:
    when event.physical:shortcut is on
    then toggle light.social:x;
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
	matches, _ := filepath.Glob(filepath.Join(outDir, "automation", "*", "automation.*.yaml"))
	if len(matches) != 1 {
		t.Fatalf("got %d automation files, want 1: %v", len(matches), matches)
	}
	content, _ := os.ReadFile(matches[0])

	var parsed []map[string]any
	if err := yaml.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("generated automation is not valid YAML: %v\ncontent:\n%s", err, content)
	}
	triggers, _ := parsed[0]["trigger"].([]any)
	if len(triggers) != 1 {
		t.Fatalf("got %d triggers, want 1: %+v", len(triggers), parsed[0]["trigger"])
	}
	options, _ := triggers[0].(map[string]any)["options"].(map[string]any)
	eventTypes, _ := options["event_type"].([]any)
	if len(eventTypes) != 1 {
		t.Fatalf("got %d event_type values, want 1: %+v", len(eventTypes), options["event_type"])
	}
	if s, ok := eventTypes[0].(string); !ok || s != "on" {
		t.Errorf("event_type[0] = %#v (type %T), want the string \"on\" -- a bool here means YAML\n"+
			"parsed it as a boolean instead of a string, exactly the bug this test guards against.\n"+
			"content:\n%s", eventTypes[0], eventTypes[0], content)
	}
}

// TestParseRuleStateConditionFor confirms "if <entity-ref> is <state> for <duration>;" parses
// into TRuleCondition.For and renders as HA's own nested state-condition "for:" field (2026-10-13
// addition, needed for Junglinster's Nespresso auto-off debounce migration).
func TestParseRuleStateConditionFor(t *testing.T) {
	const dsl = `space social:wc as area with:
  rule social:x with:
    when every minute
    if switch.social:x is on for 00:05:00
    then turn switch.social:x off;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(result.Administration.Rules) != 1 {
		t.Fatalf("got %d Rules, want 1", len(result.Administration.Rules))
	}
	cond := result.Administration.Rules[0].Condition
	if cond == nil || cond.Kind != "state" || cond.For != "00:05:00" {
		t.Fatalf("Condition = %+v, want state condition with For \"00:05:00\"", cond)
	}

	outDir := t.TempDir()
	if err := generateRuleAutomations(outDir, result.Administration); err != nil {
		t.Fatalf("generateRuleAutomations: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(outDir, "automation", "*", "automation.*.yaml"))
	if len(matches) != 1 {
		t.Fatalf("got %d automation files, want 1: %v", len(matches), matches)
	}
	content, _ := os.ReadFile(matches[0])
	got := string(content)
	if !strings.Contains(got, "for:\n") || !strings.Contains(got, "hours: 00\n") || !strings.Contains(got, "minutes: 05\n") {
		t.Errorf("generated automation missing state condition for:, got:\n%s", got)
	}
}

// TestParseRuleTooManyClausesWarns confirms a second "when"/"if"/"then" clause in the same rule
// is rejected with an actionable warning and ignored, not silently accepted (PROJECT.md item 13,
// 2026-10-13: exactly one "when", at most one "if", exactly one "then" per rule).
func TestParseRuleTooManyClausesWarns(t *testing.T) {
	const dsl = `space social:wc as area with:
  rule social:x with:
    when binary_sensor.social:a turns on
    when binary_sensor.social:b turns on
    if binary_sensor.social:c is on
    if binary_sensor.social:d is on
    then turn fan.social:x on;
    then turn fan.social:x off;
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
	if len(result.Administration.Rules) != 1 {
		t.Fatalf("got %d Rules, want 1", len(result.Administration.Rules))
	}
	rule := result.Administration.Rules[0]
	if len(rule.Triggers) != 1 || rule.Triggers[0].EntitySpec != "binary_sensor.social/wc/a" {
		t.Errorf("Triggers = %+v, want only the first \"when\" clause kept", rule.Triggers)
	}
	if rule.Condition == nil || rule.Condition.EntitySpec != "binary_sensor.social/wc/c" {
		t.Errorf("Condition = %+v, want only the first \"if\" clause kept", rule.Condition)
	}
	if len(rule.Actions) != 1 || rule.Actions[0].State != "on" {
		t.Errorf("Actions = %+v, want only the first \"then\" clause kept", rule.Actions)
	}
	for _, want := range []string{
		`exactly one "when" clause`,
		`at most one "if" clause`,
		`exactly one "then" clause`,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected a warning containing %s, got: %q", want, output)
		}
	}
}

// TestParseRuleWhenOrChain confirms "when <clause> or <clause>;" parses into multiple triggers
// from a single statement (2026-10-13 readability addition) -- functionally identical to writing
// one "when ...;" line per clause, just more compact when they share one action.
func TestParseRuleWhenOrChain(t *testing.T) {
	const dsl = `space social:apartment/hallway as area with:
  rule social:main/light/toggle with:
    when event.physical:door is 2_single or
         event.physical:bedroom/door is 2_single
    then toggle light.social:/apartment/hallway/main;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(result.Administration.Rules) != 1 {
		t.Fatalf("got %d Rules, want 1", len(result.Administration.Rules))
	}
	rule := result.Administration.Rules[0]
	if len(rule.Triggers) != 2 {
		t.Fatalf("got %d Triggers, want 2: %+v", len(rule.Triggers), rule.Triggers)
	}
	if rule.Triggers[0].EntitySpec != "event.physical/apartment/hallway/door" || rule.Triggers[0].EventType != "2_single" {
		t.Errorf("Triggers[0] = %+v, want physical/apartment/hallway/door is 2_single", rule.Triggers[0])
	}
	if rule.Triggers[1].EntitySpec != "event.physical/apartment/hallway/bedroom/door" || rule.Triggers[1].EventType != "2_single" {
		t.Errorf("Triggers[1] = %+v, want physical/apartment/hallway/bedroom/door is 2_single", rule.Triggers[1])
	}
}

// TestParseRuleThenBlock confirms "then: <action>; <action>; end;" parses multiple actions
// without repeating "then" on each line (2026-10-13 readability addition).
func TestParseRuleThenBlock(t *testing.T) {
	const dsl = `space social:apartment as area with:
  rule social:/leaving with:
    when event.physical:/apartment/hallway/door is 1_single
    then:
      turn input_boolean.social:/occupation/around on;
      turn switch.social:/space off;
    end;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(result.Administration.Rules) != 1 {
		t.Fatalf("got %d Rules, want 1", len(result.Administration.Rules))
	}
	rule := result.Administration.Rules[0]
	if len(rule.Actions) != 2 {
		t.Fatalf("got %d Actions, want 2: %+v", len(rule.Actions), rule.Actions)
	}
	if rule.Actions[0].Verb != "turn" || rule.Actions[0].State != "on" || rule.Actions[0].EntitySpec != "input_boolean.social/occupation/around" {
		t.Errorf("Actions[0] = %+v, want turn-on input_boolean.social/occupation/around", rule.Actions[0])
	}
	if rule.Actions[1].Verb != "turn" || rule.Actions[1].State != "off" || rule.Actions[1].EntitySpec != "switch.social/space" {
		t.Errorf("Actions[1] = %+v, want turn-off switch.social/space", rule.Actions[1])
	}
}

// TestDimAndBrightenActionsRenderAsTurnOnWithBrightnessStep confirms "dim ... with <N>%;" /
// "brighten ... with <N>%;" are parsed as a "turn ... on" with a signed, bounded step (2026-10-15;
// bounded 2026-10-23): rendered as a brightness_pct computed from the light's own brightness,
// never below dimBrightnessFloorPct and never above 100 -- so dimming never switches a light off.
func TestDimAndBrightenActionsRenderAsTurnOnWithBrightnessStep(t *testing.T) {
	const dsl = `space social:wc as area with:
  entity light.social:main;

  rule social:x with:
    when every minute
    then:
      dim light.social:main with 10%;
      brighten light.social:main with 15%;
    end;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(result.Administration.Rules) != 1 {
		t.Fatalf("got %d Rules, want 1", len(result.Administration.Rules))
	}
	rule := result.Administration.Rules[0]
	if len(rule.Actions) != 2 {
		t.Fatalf("got %d Actions, want 2: %+v", len(rule.Actions), rule.Actions)
	}
	if rule.Actions[0].Verb != "turn" || rule.Actions[0].State != "on" || len(rule.Actions[0].Data) != 1 || rule.Actions[0].Data[0].Name != brightnessBoundedStepField || rule.Actions[0].Data[0].Value != "-10" {
		t.Errorf("Actions[0] = %+v, want turn-on with a bounded step of -10", rule.Actions[0])
	}
	if rule.Actions[1].Verb != "turn" || rule.Actions[1].State != "on" || len(rule.Actions[1].Data) != 1 || rule.Actions[1].Data[0].Name != brightnessBoundedStepField || rule.Actions[1].Data[0].Value != "15" {
		t.Errorf("Actions[1] = %+v, want turn-on with a bounded step of 15", rule.Actions[1])
	}
	var sb strings.Builder
	renderRuleActionYAML(&sb, "  ", rule.Actions[0])
	want := "brightness_pct: \"{{ [[((state_attr('light.social_wc_main', 'brightness') or 0) / 2.55) | round | int + (-10), 5] | max, 100] | min }}\"\n"
	if !strings.Contains(sb.String(), want) || strings.Contains(sb.String(), "brightness_step_pct") {
		t.Errorf("rendered dim action:\n%s\nwant it to contain %q", sb.String(), want)
	}
}

// TestDefaultBrightnessDataFieldSugar confirms "with [default] brightness <N>%;" is sugar for
// "with brightness_pct <N>;" (2026-10-15) -- "default" and "%" are both optional and cosmetic.
func TestDefaultBrightnessDataFieldSugar(t *testing.T) {
	const dsl = `space social:wc as area with:
  entity light.social:main;

  rule social:x with:
    when every minute
    then turn light.social:main on with default brightness 70%;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	rule := result.Administration.Rules[0]
	if len(rule.Actions) != 1 || len(rule.Actions[0].Data) != 1 || rule.Actions[0].Data[0].Name != "brightness_pct" || rule.Actions[0].Data[0].Value != "70" {
		t.Fatalf("Actions = %+v, want one turn-on with brightness_pct 70", rule.Actions)
	}
}
