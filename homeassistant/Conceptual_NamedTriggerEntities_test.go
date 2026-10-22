/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualNamedTriggerEntitiesTest
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 15.10.2026
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

// TestNamedTriggerSingleDeviceMatchesExplicitEventTrigger confirms a device-level "trigger <name>
// when <capability> is '<value>';" declaration plus a rule's own "when <name> is triggered"
// reference renders the exact same event.received trigger YAML an equivalent explicit "when
// <entity-ref> is '<value>';" rule already would -- the whole point of this construct (PROJECT.md's
// device-level trigger-abstraction step, 2026-10-15) is that the two must be equivalent, just with
// the raw value hidden in the device declaration instead of the rule.
func TestNamedTriggerSingleDeviceMatchesExplicitEventTrigger(t *testing.T) {
	localDevicesByID := map[string]TLocalDevice{
		"buttons.test_button_1": {
			DeviceID:     "buttons.test_button_1",
			Capabilities: map[string]TLocalCapability{"pressed": {Domain: "event"}},
		},
	}

	const dsl = `space social:wc as area with:
  entity light.social:main;

  device buttons.test_button_1 with:
    trigger social:main/light/toggle_request when event.pressed is '2_single';
  end;

  rule social:main/light/toggle with:
    when trigger.social:main/light/toggle_request is triggered
    then toggle light.social:main;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, localDevicesByID, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	sources := result.Administration.NamedTriggers["social/wc/main/light/toggle_request"]
	if len(sources) != 1 {
		t.Fatalf("got %d NamedTriggers sources, want 1: %+v", len(sources), sources)
	}
	if len(sources[0].EventTypes) != 1 || sources[0].EventTypes[0] != "2_single" {
		t.Errorf("sources[0].EventTypes = %+v, want [\"2_single\"]", sources[0].EventTypes)
	}
	if len(result.Administration.Rules) != 1 {
		t.Fatalf("got %d Rules, want 1", len(result.Administration.Rules))
	}
	rule := result.Administration.Rules[0]
	if len(rule.Triggers) != 1 || rule.Triggers[0].Kind != "named_trigger" || rule.Triggers[0].Name != "social/wc/main/light/toggle_request" {
		t.Fatalf("Triggers = %+v, want one named_trigger on social:main/light/toggle_request", rule.Triggers)
	}

	_, got := generateNamedTriggerFiles(t, result.Administration)
	for _, want := range []string{
		"  - trigger: event.received\n",
		"      entity_id:\n      - event.physical_wc_test_button_1\n",
		"      event_type:\n      - '2_single'\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated event entity missing %q, got:\n%s", want, got)
		}
	}
}

// TestNamedTriggerMultipleDevicesOred confirms TWO different devices declaring the SAME named
// trigger both end up OR'd into the one rule's trigger list (one event.received trigger entry
// per contributing device) -- the implicit disjunction across devices this construct exists for.
func TestNamedTriggerMultipleDevicesOred(t *testing.T) {
	localDevicesByID := map[string]TLocalDevice{
		"buttons.test_button_1": {
			DeviceID:     "buttons.test_button_1",
			Capabilities: map[string]TLocalCapability{"pressed": {Domain: "event"}},
		},
		"buttons.test_button_2": {
			DeviceID:     "buttons.test_button_2",
			Capabilities: map[string]TLocalCapability{"pressed": {Domain: "event"}},
		},
	}

	const dsl = `space social:wc as area with:
  entity light.social:main;

  device buttons.test_button_1 with:
    trigger social:main/light/toggle_request when event.pressed is '2_single';
  end;

  device buttons.test_button_2 with:
    trigger social:main/light/toggle_request when event.pressed is in ['1_single', '2_single'];
  end;

  rule social:main/light/toggle with:
    when trigger.social:main/light/toggle_request is triggered
    then toggle light.social:main;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, localDevicesByID, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	sources := result.Administration.NamedTriggers["social/wc/main/light/toggle_request"]
	if len(sources) != 2 {
		t.Fatalf("got %d NamedTriggers sources, want 2: %+v", len(sources), sources)
	}

	_, got := generateNamedTriggerFiles(t, result.Administration)
	if strings.Count(got, "trigger: event.received") != 2 {
		t.Errorf("want 2 event.received trigger entries (one OR'd per device), got:\n%s", got)
	}
	for _, want := range []string{
		"      entity_id:\n      - event.physical_wc_test_button_1\n",
		"      entity_id:\n      - event.physical_wc_test_button_2\n",
		"      - '1_single'\n",
		"      - '2_single'\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated event entity missing %q, got:\n%s", want, got)
		}
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

// TestNamedTriggerUndeclaredWarns confirms a rule referencing a named trigger that no device
// declares is warned about (checks.go), not silently accepted.
func TestNamedTriggerUndeclaredWarns(t *testing.T) {
	const dsl = `space social:wc as area with:
  entity light.social:main;

  rule social:main/light/toggle with:
    when trigger.social:main/light/toggle_request is triggered
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
	if !strings.Contains(output, "named trigger") || !strings.Contains(output, "social/wc/main/light/toggle_request") {
		t.Errorf("expected a warning naming the undeclared named trigger, got: %q", output)
	}
}

// TestNamedTriggerDuplicateFromSameDeviceWarns confirms the SAME device declaring the SAME named
// trigger twice is warned about (a real authoring mistake), not silently duplicated.
func TestNamedTriggerDuplicateFromSameDeviceWarns(t *testing.T) {
	localDevicesByID := map[string]TLocalDevice{
		"buttons.test_button_1": {
			DeviceID:     "buttons.test_button_1",
			Capabilities: map[string]TLocalCapability{"pressed": {Domain: "event"}},
		},
	}

	const dsl = `space social:wc as area with:
  device buttons.test_button_1 with:
    trigger social:main/light/toggle_request when event.pressed is '1_single';
    trigger social:main/light/toggle_request when event.pressed is '2_single';
  end;
end;`

	var report strings.Builder
	output := captureStderr(t, func() {
		_, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, localDevicesByID, nil, nil)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
	})
	if !strings.Contains(output, "more than once") {
		t.Errorf("expected a duplicate-declaration warning, got: %q", output)
	}
}

// TestDeviceAtLeafWithDoubleColonFoldsIntoHiddenTriggerEntity confirms "device <id> at <leaf>
// with:" + "::" (2026-10-15) lets a "trigger ...;" line's own hidden entity reuse a SPECIFIC,
// author-chosen leaf (matching a pre-existing live entity_id when migrating a real device)
// instead of deriving one from the device id -- and that plain ":" lines in the SAME block are
// completely unaffected (only "::" opts in).
func TestDeviceAtLeafWithDoubleColonFoldsIntoHiddenTriggerEntity(t *testing.T) {
	localDevicesByID := map[string]TLocalDevice{
		"buttons.test_button_1": {
			DeviceID: "buttons.test_button_1",
			Capabilities: map[string]TLocalCapability{
				"pressed":           {Domain: "event"},
				"pressed_as_sensor": {Domain: "sensor"},
			},
		},
	}

	const dsl = `space social:wc as area with:
  entity light.social:main;

  device buttons.test_button_1 at door with:
    trigger social:main/light/toggle_request when event.pressed is '2_single';
    entity sensor.infrastructural::battery_level from sensor.pressed_as_sensor;
  end;

  rule social:main/light/toggle with:
    when trigger.social:main/light/toggle_request is triggered
    then toggle light.social:main;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, localDevicesByID, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	sources := result.Administration.NamedTriggers["social/wc/main/light/toggle_request"]
	if len(sources) != 1 {
		t.Fatalf("got %d NamedTriggers sources, want 1: %+v", len(sources), sources)
	}
	if sources[0].EntityID != "event.physical_wc_door" {
		t.Errorf("sources[0].EntityID = %q, want %q (the \"at door\" leaf, not a device-id-derived one)", sources[0].EntityID, "event.physical_wc_door")
	}

	entities := result.Administration.EntityRecordsBySpace["social/wc"]
	var sawBatteryLevel bool
	for _, rec := range entities {
		if rec.Name == "sensor.infrastructural/wc/door/battery_level" {
			sawBatteryLevel = true
		}
	}
	if !sawBatteryLevel {
		t.Errorf("expected \"::\" to fold the \"door\" leaf into the plain entity line too, got entities: %+v", entities)
	}
}

// TestNamedTriggerEmbeddedAttrConditionAppliesAtGeneration confirms a "trigger ...; and if
// trigger.<attr> is '<value>';" declaration (Step 2, 2026-10-15) folds into the generated
// automation's own gating condition -- the Signify dimmer press/hold + button-direction case --
// without the rule itself ever naming the raw button attribute value.
func TestNamedTriggerEmbeddedAttrConditionAppliesAtGeneration(t *testing.T) {
	localDevicesByID := map[string]TLocalDevice{
		"buttons.test_dimmer": {
			DeviceID:     "buttons.test_dimmer",
			Capabilities: map[string]TLocalCapability{"pressed": {Domain: "event"}},
		},
	}

	const dsl = `space social:living_room as area with:
  entity light.social:main;

  device buttons.test_dimmer at dimmer with:
    trigger social:main/light/dim_request when event.pressed is in ['press', 'hold'] and if trigger.button is 'down';
  end;

  rule social:main/light/dim_down with:
    when trigger.social:main/light/dim_request is triggered
    then turn light.social:main on with brightness_step_pct -10;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, localDevicesByID, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	sources := result.Administration.NamedTriggers["social/living_room/main/light/dim_request"]
	if len(sources) != 1 {
		t.Fatalf("got %d NamedTriggers sources, want 1: %+v", len(sources), sources)
	}
	if sources[0].TriggerAttr != "button" || len(sources[0].TriggerAttrValues) != 1 || sources[0].TriggerAttrValues[0] != "down" {
		t.Errorf("sources[0] attr/values = %q/%v, want \"button\"/[down]", sources[0].TriggerAttr, sources[0].TriggerAttrValues)
	}
	if len(sources[0].EventTypes) != 2 || sources[0].EventTypes[0] != "press" || sources[0].EventTypes[1] != "hold" {
		t.Errorf("sources[0].EventTypes = %+v, want [press hold]", sources[0].EventTypes)
	}

	automation, got := generateNamedTriggerFiles(t, result.Administration)
	for _, want := range []string{
		"conditions:\n  - condition: template\n",
		"value_template: '{{ trigger.to_state.attributes.button == ''down'' }}'\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated event entity missing %q, got:\n%s", want, got)
		}
	}
	for _, want := range []string{
		"  - service: light.turn_on\n",
		"    data:\n      brightness_step_pct: -10\n",
	} {
		if !strings.Contains(automation, want) {
			t.Errorf("generated automation missing %q, got:\n%s", want, automation)
		}
	}
	if strings.Contains(automation, "condition:") {
		t.Errorf("the attr gate belongs in the event entity, not the automation, got:\n%s", automation)
	}
}

// TestNamedTriggerDisagreeingSourcesGetPerSourceScopedConditions is the real regression found
// live in Vienna 2026-10-19: several different Moes button devices all feed the SAME named
// trigger, each needing its OWN, DIFFERENT button-number filter (one device's own numbering
// means "button 1", another's means "button 2 or 3") -- a single shared condition can't express
// that, since HA's automation template context only ever reflects whichever ONE entity actually
// fired. Confirms each contributing trigger entry gets its own "id", and the generated condition
// is an OR, across sources, of (trigger.id == this source's id) AND its own attr check -- the
// list-valued source's own attr check further expands into an inner OR of its individual values.
func TestNamedTriggerDisagreeingSourcesGetPerSourceScopedConditions(t *testing.T) {
	localDevicesByID := map[string]TLocalDevice{
		"buttons.test_moes_a": {
			DeviceID:     "buttons.test_moes_a",
			Capabilities: map[string]TLocalCapability{"pressed": {Domain: "event"}},
		},
		"buttons.test_moes_b": {
			DeviceID:     "buttons.test_moes_b",
			Capabilities: map[string]TLocalCapability{"pressed": {Domain: "event"}},
		},
	}

	const dsl = `space social:living_room as area with:
  entity light.social:main;

  device buttons.test_moes_a at moes_a with:
    trigger social:main/light/toggle_request when event.pressed is 'single' and if trigger.button is '1';
  end;

  device buttons.test_moes_b at moes_b with:
    trigger social:main/light/toggle_request when event.pressed is 'single' and if trigger.button is in ['2', '3'];
  end;

  rule social:main/light/toggle with:
    when trigger.social:main/light/toggle_request is triggered
    then toggle light.social:main;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, localDevicesByID, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	sources := result.Administration.NamedTriggers["social/living_room/main/light/toggle_request"]
	if len(sources) != 2 {
		t.Fatalf("got %d NamedTriggers sources, want 2: %+v", len(sources), sources)
	}

	_, got := generateNamedTriggerFiles(t, result.Administration)
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
	// Both sources matched on the SAME raw event_type ('single') -- the per-source id/attr
	// condition is what must tell them apart, not the event_type list, so a naive reader could
	// mistake this for the "sources agree" shared-condition path if the ids were missing.
	if strings.Count(got, "condition: template") != 5 { // 2 id checks + 1 single-value + 2-way OR for the list
		t.Errorf("expected 5 template conditions (2 id + 1 single attr + 2 list-expanded attr), got:\n%s", got)
	}
}

// TestDimAndBrightenActionsRenderAsTurnOnWithBrightnessStep confirms "dim ... with <N>%;" /
// "brighten ... with <N>%;" are parsed as sugar for "turn ... on with brightness_step_pct
// -N/+N;" (2026-10-15) -- no raw negative-number/field-name breach of abstraction in rule text.
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
	if rule.Actions[0].Verb != "turn" || rule.Actions[0].State != "on" || len(rule.Actions[0].Data) != 1 || rule.Actions[0].Data[0].Name != "brightness_step_pct" || rule.Actions[0].Data[0].Value != "-10" {
		t.Errorf("Actions[0] = %+v, want turn-on with brightness_step_pct -10", rule.Actions[0])
	}
	if rule.Actions[1].Verb != "turn" || rule.Actions[1].State != "on" || len(rule.Actions[1].Data) != 1 || rule.Actions[1].Data[0].Name != "brightness_step_pct" || rule.Actions[1].Data[0].Value != "15" {
		t.Errorf("Actions[1] = %+v, want turn-on with brightness_step_pct 15", rule.Actions[1])
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

// TestNormalizeAggregatedTrigger covers the 2026-10-21 notation: both the one-liner and the block
// form rewrite into the original "trigger <name> when <expr>;" declarations, one per clause.
func TestNormalizeAggregatedTrigger(t *testing.T) {
	identity := func(line string) string { return line }

	got, newI, warnings, ok := normalizeAggregatedTrigger(nil, 0, "aggregated trigger.social:main/light/toggle with trigger event.pressed is 'single' and if trigger.button is '2';", identity)
	want := []string{"trigger social:main/light/toggle when event.pressed is 'single' and if trigger.button is '2';"}
	if !ok || newI != 0 || len(warnings) != 0 || !reflect.DeepEqual(got, want) {
		t.Errorf("one-liner: got (%q, %d, %v, %v), want (%q, 0, none, true)", got, newI, warnings, ok, want)
	}

	block := []string{
		"aggregated trigger.social:main/light/toggle with:",
		"  trigger event.pressed is 'single' and if trigger.button is '2';",
		"  # a comment",
		"  trigger event.pressed is 'double';",
		"  something else;",
		"end;",
		"entity light.social:main;",
	}
	got, newI, warnings, ok = normalizeAggregatedTrigger(block, 0, block[0], identity)
	want = []string{
		"trigger social:main/light/toggle when event.pressed is 'single' and if trigger.button is '2';",
		"trigger social:main/light/toggle when event.pressed is 'double';",
	}
	if !ok || newI != 5 || len(warnings) != 1 || !reflect.DeepEqual(got, want) {
		t.Errorf("block: got (%q, %d, %v, %v), want (%q, 5, one warning, true)", got, newI, warnings, ok, want)
	}
	for _, declaration := range got {
		if _, parses := extractNamedTriggerDeclaration(declaration); !parses {
			t.Errorf("normalized declaration %q not understood by extractNamedTriggerDeclaration", declaration)
		}
	}

	if _, _, _, ok := normalizeAggregatedTrigger(nil, 0, "trigger social:x when event.pressed is 'single';", identity); ok {
		t.Errorf("the original form must be left to extractNamedTriggerDeclaration itself")
	}
}

// TestNormalizeAggregatedEvent covers the 2026-10-22 notation "aggregated event.<name> is raised
// when <expr>;" (and its block form): same declarations as the "aggregated trigger." notation.
func TestNormalizeAggregatedEvent(t *testing.T) {
	identity := func(line string) string { return line }
	got, _, _, ok := normalizeAggregatedTrigger(nil, 0, "aggregated event.social:light/main/toggle is raised when event.pressed is 'single' and if trigger.button is '1';", identity)
	want := []string{"trigger social:light/main/toggle when event.pressed is 'single' and if trigger.button is '1';"}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Errorf("one-liner: got (%q, %v), want %q", got, ok, want)
	}
	block := []string{"aggregated event.social:light/main/toggle is raised when:", "  event.pressed is 'single';", "  event.pressed is 'double';", "end;"}
	got, newI, warnings, ok := normalizeAggregatedTrigger(block, 0, block[0], identity)
	want = []string{"trigger social:light/main/toggle when event.pressed is 'single';", "trigger social:light/main/toggle when event.pressed is 'double';"}
	if !ok || newI != 3 || len(warnings) != 0 || !reflect.DeepEqual(got, want) {
		t.Errorf("block: got (%q, %d, %v, %v), want %q", got, newI, warnings, ok, want)
	}
}
