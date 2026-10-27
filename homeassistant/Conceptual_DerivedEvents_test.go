/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualDerivedEventsTest
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 22.10.2026
 *
 */

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// buttonDevices returns local button devices with an "event.pressed" capability.
func buttonDevices(ids ...string) map[string]TLocalDevice {
	devices := map[string]TLocalDevice{}
	for _, id := range ids {
		devices[id] = TLocalDevice{DeviceID: id, Capabilities: map[string]TLocalCapability{"pressed": {Domain: "event"}}}
	}
	return devices
}

// parseRequest parses one Logical.def request device (logicalLines) and the Conceptual.def text
// dsl positioning it, its buttons and a rule using it.
func parseRequest(t *testing.T, logicalLines []string, dsl string, localDevicesByID map[string]TLocalDevice) *TAdministrationState {
	t.Helper()
	logicalDevices, warnings := parseLogicalLayerBody(logicalLines, nil)
	if len(warnings) != 0 || len(logicalDevices) != 1 {
		t.Fatalf("Logical.def: devices %+v, warnings %v", logicalDevices, warnings)
	}
	logicalDevicesByID := map[string]TLogicalDevice{logicalDevices[0].DeviceID: logicalDevices[0]}
	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, localDevicesByID, logicalDevicesByID, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return result.Administration
}

// requestDSL positions request.wc_light_toggle as event.social:main/light/toggle in space wc,
// the given buttons (each "at" its own bare label), and a rule toggling the light on it.
func requestDSL(buttons ...string) string {
	dsl := "space social:wc as area with:\n  entity light.social:main;\n\n" +
		"  device request.wc_light_toggle with:\n    entity event.social:main/light/toggle;\n  end;\n\n"
	for _, button := range buttons {
		dsl += "  device " + button + " as leaf " + bareCapabilityLabel(button) + " with:\n    entity event.physical:: from event.pressed;\n  end;\n\n"
	}
	return dsl + "  rule social:main/light/toggle with:\n    when event.social:main/light/toggle is raised\n    then toggle light.social:main;\n  end;\nend;"
}

// TestRequestSingleButton: one button's raw event code lives in the request's condition, never in
// the rule -- the event entity listens to the button, the rule to the event entity.
func TestRequestSingleButton(t *testing.T) {
	admin := parseRequest(t, []string{
		"device request.wc_light_toggle with:",
		"  derived capability event.toggle with condition event.pressed from buttons.one is '2_single';",
		"end;",
	}, requestDSL("buttons.one"), buttonDevices("buttons.one"))
	_, got := generateNamedTriggerFiles(t, admin)
	for _, want := range []string{
		"      entity_id:\n      - event.physical_wc_one\n",
		"      event_type:\n      - '2_single'\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated event entity missing %q, got:\n%s", want, got)
		}
	}
}

// TestRequestSeveralButtonsOred: every term of the condition is one more trigger of the same
// event entity.
func TestRequestSeveralButtonsOred(t *testing.T) {
	admin := parseRequest(t, []string{
		"device request.wc_light_toggle with:",
		"  derived capability event.toggle with condition",
		"    event.pressed from buttons.one is '2_single' or",
		"    event.pressed from buttons.two is in ['1_single', '2_single'];",
		"end;",
	}, requestDSL("buttons.one", "buttons.two"), buttonDevices("buttons.one", "buttons.two"))
	_, got := generateNamedTriggerFiles(t, admin)
	if strings.Count(got, "trigger: event.received") != 2 {
		t.Errorf("want 2 event.received triggers (one per button), got:\n%s", got)
	}
	for _, want := range []string{"      - event.physical_wc_one\n", "      - event.physical_wc_two\n", "      - '1_single'\n", "      - '2_single'\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("generated event entity missing %q, got:\n%s", want, got)
		}
	}
}

// TestRequestSharedAttributeFilter: a "trigger.<attribute>" filter the terms agree on becomes the
// event entity's one shared condition (the Signify dimmer's press/hold + button-direction case);
// the rule itself carries no condition.
func TestRequestSharedAttributeFilter(t *testing.T) {
	dsl := strings.Replace(requestDSL("buttons.dimmer"), "then toggle light.social:main;", "then turn light.social:main on with brightness_step_pct -10;", 1)
	admin := parseRequest(t, []string{
		"device request.wc_light_toggle with:",
		"  derived capability event.toggle with condition",
		"    event.pressed from buttons.dimmer is in ['press', 'hold'] and if trigger.button from buttons.dimmer is 'down';",
		"end;",
	}, dsl, buttonDevices("buttons.dimmer"))
	automation, got := generateNamedTriggerFiles(t, admin)
	for _, want := range []string{
		"conditions:\n  - condition: template\n",
		"value_template: '{{ trigger.to_state.attributes.button == ''down'' }}'\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated event entity missing %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "    id: 'nt") {
		t.Errorf("one shared filter needs no per-trigger ids, got:\n%s", got)
	}
	if !strings.Contains(automation, "    data:\n      brightness_step_pct: -10\n") || strings.Contains(automation, "condition:") {
		t.Errorf("automation should carry the action and no condition, got:\n%s", automation)
	}
}

// TestRequestDisagreeingFiltersScopedPerTrigger is the real 2026-10-19 Vienna case: several
// buttons feed the same request, each with its own button-number filter. Each trigger gets its own
// id, and the condition is an "or" over (this trigger fired and its own filter holds) -- HA's
// template context only ever shows the one entity that fired.
func TestRequestDisagreeingFiltersScopedPerTrigger(t *testing.T) {
	admin := parseRequest(t, []string{
		"device request.wc_light_toggle with:",
		"  derived capability event.toggle with condition",
		"    event.pressed from buttons.moes_a is 'single' and if trigger.button from buttons.moes_a is '1' or",
		"    event.pressed from buttons.moes_b is 'single' and if trigger.button from buttons.moes_b is in ['2', '3'];",
		"end;",
	}, requestDSL("buttons.moes_a", "buttons.moes_b"), buttonDevices("buttons.moes_a", "buttons.moes_b"))
	_, got := generateNamedTriggerFiles(t, admin)
	for _, want := range []string{
		"    id: 'nt0'\n",
		"    id: 'nt1'\n",
		"value_template: '{{ trigger.id == ''nt0'' }}'\n",
		"value_template: '{{ trigger.id == ''nt1'' }}'\n",
		"value_template: '{{ trigger.to_state.attributes.button == ''1'' }}'\n",
		"value_template: '{{ trigger.to_state.attributes.button == ''2'' }}'\n",
		"value_template: '{{ trigger.to_state.attributes.button == ''3'' }}'\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated event entity missing %q, got:\n%s", want, got)
		}
	}
	if strings.Count(got, "condition: template") != 5 { // 2 id checks + 1 single-value + 2-way OR for the list
		t.Errorf("expected 5 template conditions (2 id + 1 single attr + 2 list-expanded attr), got:\n%s", got)
	}
}

// TestRaisedEventUndeclaredWarns: a rule on an event nothing raises is reported.
func TestRaisedEventUndeclaredWarns(t *testing.T) {
	const dsl = `space social:wc as area with:
  entity light.social:main;

  rule social:main/light/toggle with:
    when event.social:main/light/toggle is raised
    then toggle light.social:main;
  end;
end;`

	var report strings.Builder
	output := captureStdout(t, func() {
		_, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
	})
	if !strings.Contains(output, "social/wc/main/light/toggle") || !strings.Contains(output, "nothing raises") {
		t.Errorf("expected a warning naming the event nothing raises, got: %q", output)
	}
}

// generateNamedTriggerFiles runs the named-trigger event and automation steps for a parse
// with exactly one rule and one named trigger, returning (automation, event entity) YAML -- and
// checks the automation listens to just that event entity's own "raised" event (PROJECT.md item
// 49: the device expansion and gating now live in the event entity).
func generateNamedTriggerFiles(t *testing.T, admin *TAdministrationState) (string, string) {
	t.Helper()
	outDir := t.TempDir()
	if err := generateNamedTriggerEvents(outDir, admin); err != nil {
		t.Fatalf("generateNamedTriggerEvents: %v", err)
	}
	if err := generateRuleAutomations(outDir, admin); err != nil {
		t.Fatalf("generateRuleAutomations: %v", err)
	}
	read := func(pattern string) (string, string) {
		matches, _ := filepath.Glob(filepath.Join(outDir, pattern))
		if len(matches) != 1 {
			t.Fatalf("got %d files for %s, want 1: %v", len(matches), pattern, matches)
		}
		content, _ := os.ReadFile(matches[0])
		return filepath.Base(matches[0]), string(content)
	}
	_, automation := read(filepath.Join("automation", "*", "automation.*.yaml"))
	eventFile, event := read(filepath.Join("entities", "template", "event", "*", "event.*.yaml"))
	eventID := strings.TrimSuffix(eventFile, ".yaml")
	want := "  - trigger: event.received\n    target:\n      entity_id:\n      - " + eventID + "\n    options:\n      event_type:\n      - 'raised'\n"
	if !strings.Contains(automation, want) || strings.Count(automation, "trigger: event.received") != 1 {
		t.Errorf("automation should trigger on %s's 'raised' only, got:\n%s", eventID, automation)
	}
	for _, want := range []string{"event:\n- name: ", "  unique_id: " + eventID + "\n", "  event_type: raised\n", "  event_types: \"{{ ['raised'] }}\"\n"} {
		if !strings.Contains(event, want) {
			t.Errorf("event entity missing %q, got:\n%s", want, event)
		}
	}
	return automation, event
}

// TestLogicalEventCapabilityRaisesNamedEvent: a Logical.def "derived capability event.<label> as
// <id> with condition ..." device (PROJECT.md item 49) is positioned in Conceptual.def; its
// sources are the buttons' raw events, which Conceptual.def never positions -- the logical layer
// auto-materializes them as "physical" plumbing entities -- each with its own button filter, and
// the entity carries the frozen unique_id.
func TestLogicalEventCapabilityRaisesNamedEvent(t *testing.T) {
	localDevicesByID := map[string]TLocalDevice{
		"buttons.door": {DeviceID: "buttons.door", Capabilities: map[string]TLocalCapability{"pressed": {Domain: "event"}}},
		"buttons.bed":  {DeviceID: "buttons.bed", Capabilities: map[string]TLocalCapability{"pressed": {Domain: "event"}}},
	}
	logicalDevices, warnings := parseLogicalLayerBody([]string{
		"device request.wc_light_toggle with:",
		"  derived capability event.toggle as frozen_toggle_id with",
		"    condition",
		"      event.pressed from buttons.door is 'single' and if",
		"      trigger.button from buttons.door is '1'",
		"    or",
		"      event.pressed from buttons.bed is in ['on', 'toggle'];",
		"end;",
	}, nil)
	if len(warnings) != 0 || len(logicalDevices) != 1 {
		t.Fatalf("devices %+v, warnings %v", logicalDevices, warnings)
	}
	if got := logicalDevices[0].Capabilities["toggle"].AssignedID; got != "frozen_toggle_id" {
		t.Errorf("AssignedID = %q", got)
	}
	logicalDevicesByID := map[string]TLogicalDevice{"request.wc_light_toggle": logicalDevices[0]}

	const dsl = `space social:wc as area with:
  entity light.social:main;

  device request.wc_light_toggle with:
    entity event.social:main/light/toggle;
  end;

  rule social:main/light/toggle with:
    when event.social:main/light/toggle is raised
    then toggle light.social:main;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, localDevicesByID, logicalDevicesByID, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration
	want := []TNamedTriggerSource{
		{EntityID: "event.physical_buttons_door_pressed", EventTypes: []string{"single"}, TriggerAttr: "button", TriggerAttrValues: []string{"1"}},
		{EntityID: "event.physical_buttons_bed_pressed", EventTypes: []string{"on", "toggle"}},
	}
	if got := admin.NamedTriggers["social/wc/main/light/toggle"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("NamedTriggers = %+v, want %+v (report: %s)", got, want, report.String())
	}
	if !admin.EventOnlyDeviceIDs["request.wc_light_toggle"] {
		t.Errorf("request.wc_light_toggle not recorded as event-only")
	}

	admin.FrozenUniqueIDs["logical:request.wc_light_toggle!toggle"] = "frozen_toggle_id"
	saved := generatedUniqueID
	generatedUniqueID = admin.UniqueIDFor
	defer func() { generatedUniqueID = saved }()
	outDir := t.TempDir()
	if err := generateNamedTriggerEvents(outDir, admin); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(filepath.Join(outDir, "entities", "template", "event", "social", "event.social_wc_main_light_toggle.yaml"))
	for _, want := range []string{
		"  unique_id: frozen_toggle_id\n",
		"    id: 'nt0'\n",
		"      - event.physical_buttons_door_pressed\n",
		"      - event.physical_buttons_bed_pressed\n",
		"value_template: '{{ trigger.to_state.attributes.button == ''1'' }}'\n",
	} {
		if !strings.Contains(string(content), want) {
			t.Errorf("event entity missing %q, got:\n%s", want, content)
		}
	}
}

// TestConditionExprValueMatches: "<ref> is '<value>'", "<ref> is in [...]" and "and if" parse, and
// render as state checks outside an event capability.
func TestConditionExprValueMatches(t *testing.T) {
	tree, warnings := parseConditionExpr("sensor.mode from sensors.x is 'eco' and if sensor.level is in ['1', \"2\"]", nil, "test")
	if len(warnings) != 0 || tree == nil || tree.Kind != CondAnd {
		t.Fatalf("tree %+v, warnings %v", tree, warnings)
	}
	if !reflect.DeepEqual(tree.Left.Values, []string{"eco"}) || tree.Left.FromDeviceID != "sensors.x" || tree.Left.IsAvailable {
		t.Errorf("left = %+v", tree.Left)
	}
	if !reflect.DeepEqual(tree.Right.Values, []string{"1", "2"}) {
		t.Errorf("right = %+v", tree.Right)
	}
	if _, warnings := parseConditionExpr("sensor.mode is in ['eco'", nil, "test"); len(warnings) == 0 {
		t.Errorf("unterminated list accepted")
	}
}
