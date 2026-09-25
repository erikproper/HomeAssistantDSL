package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveDerivedConditionOneLiners is the regression test for the 2026-09-25 full-sweep
// rewording: "derived DDD.NNN with condition VVV;" is a pure syntactic alias for the plain
// "DDD.NNN VVV;" capability line every Physical.def parser already understands -- see
// resolveDerivedConditionOneLiners's own doc comment for why this is a text rewrite rather than a
// new parser construct.
func TestResolveDerivedConditionOneLiners(t *testing.T) {
	content := "identifiers \"somfy_front\";\n" +
		"derived binary_sensor.node with condition cover.living_room_front is available;\n" +
		"cover.cover cover.living_room_front;\n"

	got := resolveDerivedConditionOneLiners(content)
	gotLines := strings.Split(got, "\n")
	wantLines := strings.Split(content, "\n")
	if len(gotLines) != len(wantLines) {
		t.Fatalf("got %d lines, want %d (line count must never change)", len(gotLines), len(wantLines))
	}
	wantRewritten := "binary_sensor.node cover.living_room_front is available;"
	if gotLines[1] != wantRewritten {
		t.Errorf("rewritten line = %q, want %q", gotLines[1], wantRewritten)
	}
	if gotLines[0] != wantLines[0] || gotLines[2] != wantLines[2] {
		t.Errorf("non-\"derived ... with condition\" lines must be left untouched, got %v", gotLines)
	}
}

// TestResolveDerivedConditionOneLinersLeavesOrdinaryLinesUntouched confirms the rewrite is a no-op
// on content that never uses the new wording -- both houses' Physical.def, pre-sweep, must
// regenerate byte-identical once this function is wired into every entry point.
func TestResolveDerivedConditionOneLinersLeavesOrdinaryLinesUntouched(t *testing.T) {
	content := "binary_sensor.node cover.living_room_front is available;\ncover.cover cover.living_room_front;"
	got := resolveDerivedConditionOneLiners(content)
	if got != content {
		t.Errorf("got %q, want content left byte-identical", got)
	}
}

// TestParseDerivedCapabilityLine is a focused unit test for plans/derived-capability-mechanism.md
// Phase 2's grammar: "derived DDD.NNN from EEE.MMM via TTT;".
func TestParseDerivedCapabilityLine(t *testing.T) {
	decl, ok := parseDerivedCapabilityLine(`derived binary_sensor.battery_alert from sensor.battery_level via "( $ | int(0) < 20 )";`)
	if !ok {
		t.Fatalf("expected a match")
	}
	want := TDerivedCapabilityDeclaration{
		Domain: "binary_sensor", Label: "battery_alert",
		FromDomain: "sensor", FromLabel: "battery_level",
		Template: "( $ | int(0) < 20 )",
	}
	if decl != want {
		t.Errorf("parseDerivedCapabilityLine = %+v, want %+v", decl, want)
	}

	if _, ok := parseDerivedCapabilityLine(`sensor.battery_level: sensor.battery_level;`); ok {
		t.Errorf("ordinary capability line must not match")
	}
	if _, ok := parseDerivedCapabilityLine(`map: "home" "docked";`); ok {
		t.Errorf("map line must not match")
	}
}

// TestParseDerivedCapabilityLineRejectsOldEntityKeyword confirms the pre-2026-09-14 "derived
// entity ..." shape (before the redundant "entity" keyword was dropped) no longer matches -- no
// back-compat was kept, since every real declaration was migrated in the same change.
func TestParseDerivedCapabilityLineRejectsOldEntityKeyword(t *testing.T) {
	if _, ok := parseDerivedCapabilityLine(`derived entity binary_sensor.battery_alert from sensor.battery_level via "( $ | int(0) < 20 )";`); ok {
		t.Errorf("the old \"derived entity ...\" shape must no longer match")
	}
}

// TestParseJinjaTemplateDefinitions is a focused unit test for Settings.def's "jinja
// ${name}(params) = "template";" declaration (2026-09-15).
func TestParseJinjaTemplateDefinitions(t *testing.T) {
	content := `
# comment line, ignored
${some_other_setting} = "value";
jinja ${int_less_then}(i) = "'on' if (($ | int(0)) < ${i}) else 'off'";
jinja ${add_float_value}(a) = "($ | float(0)) + ${a}";
jinja ${adjust_float_value}(a, s) = "(($ | float(0)) + ${a}) * ${s}";
`
	defs := parseJinjaTemplateDefinitions(content)
	if len(defs) != 3 {
		t.Fatalf("got %d definitions, want 3: %+v", len(defs), defs)
	}
	want := map[string]TJinjaTemplateDefinition{
		"int_less_then":      {Params: []string{"i"}, Template: "'on' if (($ | int(0)) < ${i}) else 'off'"},
		"add_float_value":    {Params: []string{"a"}, Template: "($ | float(0)) + ${a}"},
		"adjust_float_value": {Params: []string{"a", "s"}, Template: "(($ | float(0)) + ${a}) * ${s}"},
	}
	for name, wantDef := range want {
		gotDef, ok := defs[name]
		if !ok {
			t.Errorf("missing definition %q", name)
			continue
		}
		if gotDef.Template != wantDef.Template || len(gotDef.Params) != len(wantDef.Params) {
			t.Errorf("defs[%q] = %+v, want %+v", name, gotDef, wantDef)
			continue
		}
		for i := range wantDef.Params {
			if gotDef.Params[i] != wantDef.Params[i] {
				t.Errorf("defs[%q].Params = %v, want %v", name, gotDef.Params, wantDef.Params)
			}
		}
	}
}

// TestResolveJinjaTemplateCallsInDerivedLines proves a "derived ... via jinja ${name}(args);"
// line rewrites into the plain "derived ... via "<resolved>";" shape
// parseDerivedCapabilityLine already understands, leaving every other line untouched (same line
// count, so a caller's own line-number bookkeeping stays valid).
func TestResolveJinjaTemplateCallsInDerivedLines(t *testing.T) {
	jinjaTemplates := map[string]TJinjaTemplateDefinition{
		"int_less_then": {Params: []string{"i"}, Template: "'on' if (($ | int(0)) < ${i}) else 'off'"},
	}
	content := "identifiers \"zigbee2mqtt_0x00158d0003d4e964\";\n" +
		"sensor.battery_level 0x00158d0003d4e964_battery_zigbee2mqtt;\n" +
		"derived binary_sensor.battery_alert from sensor.battery_level via jinja ${int_less_then}(10);\n"

	got, warnings := resolveJinjaTemplateCallsInDerivedLines(content, jinjaTemplates)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	gotLines := strings.Split(got, "\n")
	wantLines := strings.Split(content, "\n")
	if len(gotLines) != len(wantLines) {
		t.Fatalf("got %d lines, want %d (line count must never change)", len(gotLines), len(wantLines))
	}
	wantDerivedLine := `derived binary_sensor.battery_alert from sensor.battery_level via "'on' if (($ | int(0)) < 10) else 'off'";`
	if gotLines[2] != wantDerivedLine {
		t.Errorf("rewritten line = %q, want %q", gotLines[2], wantDerivedLine)
	}
	if gotLines[0] != wantLines[0] || gotLines[1] != wantLines[1] {
		t.Errorf("non-\"via jinja\" lines must be left untouched, got %v", gotLines)
	}

	decl, ok := parseDerivedCapabilityLine(strings.TrimSpace(gotLines[2]))
	if !ok {
		t.Fatalf("expected the rewritten line to parse as an ordinary \"derived ... via \"...\";\" line")
	}
	if decl.Template != "'on' if (($ | int(0)) < 10) else 'off'" {
		t.Errorf("decl.Template = %q, want the resolved jinja template", decl.Template)
	}
}

// TestResolveJinjaTemplateCallsInDerivedLinesWarnsOnUndeclaredTemplate proves an undeclared jinja
// template name is reported, and the offending line is left unrewritten (so it falls through to
// parseDerivedCapabilityLine's own "unrecognised line" warning too, rather than silently vanishing).
func TestResolveJinjaTemplateCallsInDerivedLinesWarnsOnUndeclaredTemplate(t *testing.T) {
	content := `derived binary_sensor.battery_alert from sensor.battery_level via jinja ${no_such_template}(10);`
	got, warnings := resolveJinjaTemplateCallsInDerivedLines(content, map[string]TJinjaTemplateDefinition{})
	if len(warnings) != 1 || !strings.Contains(warnings[0], "no_such_template") {
		t.Fatalf("warnings = %v, want exactly one mentioning \"no_such_template\"", warnings)
	}
	if got != content {
		t.Errorf("content = %q, want unchanged when the jinja template is undeclared", got)
	}
}

// TestResolveJinjaTemplateCallsInDerivedLinesWarnsOnArgumentCountMismatch proves a jinja call with
// the wrong number of arguments is reported, not silently mis-substituted.
func TestResolveJinjaTemplateCallsInDerivedLinesWarnsOnArgumentCountMismatch(t *testing.T) {
	jinjaTemplates := map[string]TJinjaTemplateDefinition{
		"adjust_float_value": {Params: []string{"a", "s"}, Template: "(($ | float(0)) + ${a}) * ${s}"},
	}
	content := `derived sensor.pressure from sensor.pressure_raw via jinja ${adjust_float_value}(20);`
	_, warnings := resolveJinjaTemplateCallsInDerivedLines(content, jinjaTemplates)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "adjust_float_value") {
		t.Fatalf("warnings = %v, want exactly one mentioning \"adjust_float_value\"", warnings)
	}
}

// TestCollectDiscoveryGatewaysByIDResolvesJinjaDerivedCapability is an end-to-end test: a real
// discovery gateway device declared with a "derived ... via jinja ${name}(args);" line resolves
// all the way through to a fully-formed DerivedViaTemplate, exactly as if the DSL author had
// written the expanded string out by hand.
func TestCollectDiscoveryGatewaysByIDResolvesJinjaDerivedCapability(t *testing.T) {
	definitionDir := t.TempDir()
	physicalDef := `physical layer with:
  integration discovery with:
    device discovery.hallway_aqara_windoor with:
      identifiers "zigbee2mqtt_0x00158d0003d4e964";
      sensor.battery_level 0x00158d0003d4e964_battery_zigbee2mqtt;
      derived binary_sensor.battery_alert from sensor.battery_level via jinja ${int_less_then}(10);
    end;
  end;
end;
`
	settingsDef := `jinja ${int_less_then}(i) = "'on' if (($ | int(0)) < ${i}) else 'off'";`
	if err := os.WriteFile(filepath.Join(definitionDir, "Physical.def"), []byte(physicalDef), 0o644); err != nil {
		t.Fatalf("WriteFile Physical.def: %v", err)
	}
	if err := os.WriteFile(filepath.Join(definitionDir, "Settings.def"), []byte(settingsDef), 0o644); err != nil {
		t.Fatalf("WriteFile Settings.def: %v", err)
	}

	byID, warnings := collectDiscoveryGatewaysByID(definitionDir)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	device, ok := byID["discovery.hallway_aqara_windoor"]
	if !ok {
		t.Fatalf("expected device discovery.hallway_aqara_windoor, got %+v", byID)
	}
	cap, ok := device.Capabilities["battery_alert"]
	if !ok {
		t.Fatalf("expected capability battery_alert, got %+v", device.Capabilities)
	}
	if cap.DerivedFromCapability != "battery_level" {
		t.Errorf("DerivedFromCapability = %q, want battery_level", cap.DerivedFromCapability)
	}
	wantTemplate := "'on' if (($ | int(0)) < 10) else 'off'"
	if cap.DerivedViaTemplate != wantTemplate {
		t.Errorf("DerivedViaTemplate = %q, want %q", cap.DerivedViaTemplate, wantTemplate)
	}
}
