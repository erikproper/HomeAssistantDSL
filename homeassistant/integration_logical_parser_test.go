package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseLogicalLayerBody(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device node.vienna_livingroom with:",
		"  dependency on host.netatmo;",
		"end;",
		"",
		"# a comment",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	d := devices[0]
	if d.DeviceID != "node.vienna_livingroom" {
		t.Errorf("DeviceID = %q, want node.vienna_livingroom", d.DeviceID)
	}
	if len(d.DependsOn) != 1 || d.DependsOn[0] != "host.netatmo" {
		t.Errorf("DependsOn = %v, want [host.netatmo]", d.DependsOn)
	}
}

// TestParseLogicalLayerBodyMultipleDependencies covers a device depending on more than one other
// device -- "dependency on <id>;" is repeatable, one per line, no comma-list syntax.
func TestParseLogicalLayerBodyMultipleDependencies(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device node.a with:",
		"  dependency on host.b;",
		"  dependency on host.c;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	want := []string{"host.b", "host.c"}
	got := devices[0].DependsOn
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("DependsOn = %v, want %v", got, want)
	}
}

// TestParseLogicalLayerBodyOneLinerDependencyOn is the regression test for the picture_frame case
// found live 2026-09-25: a device declaring exactly one "dependency on <id>;" body line can now be
// written as a single-line shorthand, "device <id> with dependency on <id>;", instead of the full
// "with: ... end;" block -- must parse identically to the block form.
func TestParseLogicalLayerBodyOneLinerDependencyOn(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device appliance.picture_frame with dependency on node.picture_frame;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	d := devices[0]
	if d.DeviceID != "appliance.picture_frame" {
		t.Errorf("DeviceID = %q, want appliance.picture_frame", d.DeviceID)
	}
	if len(d.DependsOn) != 1 || d.DependsOn[0] != "node.picture_frame" {
		t.Errorf("DependsOn = %v, want [node.picture_frame]", d.DependsOn)
	}
}

// TestParseLogicalLayerBodyOneLinerBareCapability covers the other single-line-complete body
// shape -- a bare capability line -- shortened the same way.
// TestParseLogicalLayerBodyOneLinerRejectsBlockOpeningStatement confirms a block-opening body
// shape (e.g. "defined ... with: ...; end;") is NOT accepted as a one-liner's single statement --
// there is no one-line form for those, so this must warn rather than silently mis-parse.
func TestParseLogicalLayerBodyOneLinerRejectsBlockOpeningStatement(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device utility.thing with defined capability sensor.threshold with: minimum 0; end;",
	}, nil)
	if len(devices) != 0 {
		t.Errorf("got %d devices, want 0 (rejected one-liner shouldn't register a device)", len(devices))
	}
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "utility.thing") || !strings.Contains(warnings[0], "one-line") {
		t.Errorf("warning = %q, want it to name the device and call out the one-line restriction", warnings[0])
	}
}

// TestParseLogicalLayerBodyBlockFormStillWorksAlongsideOneLiner confirms the pre-existing
// multi-line "with: ... end;" block form is unaffected by the new one-liner pattern -- both can
// appear in the same file.
func TestParseLogicalLayerBodyBlockFormStillWorksAlongsideOneLiner(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device utility.fritz_box with:",
		"  dependency on node.fritz_box;",
		"end;",
		"device appliance.picture_frame with dependency on node.picture_frame;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2", len(devices))
	}
	if devices[0].DeviceID != "utility.fritz_box" || devices[1].DeviceID != "appliance.picture_frame" {
		t.Errorf("devices = %+v", devices)
	}
}

func TestParseLogicalLayerBodyWarnsOnMalformedLine(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device node.vienna_livingroom with:",
		"  depends on host.netatmo;", // old keyword, no longer recognised
		"end;",
	}, nil)
	if len(devices) != 1 || len(devices[0].DependsOn) != 0 {
		t.Errorf("got devices=%+v, want 1 device with no dependencies (the malformed line should be skipped, not silently accepted)", devices)
	}
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
}

// TestParseLogicalLayerBodyRejectsEntityDependencyForNow documents the current, deliberate scope
// limit: "dependency on entity <id>;"/"dependency on availability of entity <id>;" are planned
// (see this file's own header comment) but not built yet -- a two/four-token remainder must fall
// through to the generic warning rather than being misread as a device-id of "entity"/
// "availability".
func TestParseLogicalLayerBodyRejectsEntityDependencyForNow(t *testing.T) {
	_, warnings := parseLogicalLayerBody([]string{
		"device node.x with:",
		"  dependency on entity binary_sensor.kkk;",
		"end;",
	}, nil)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "unrecognised line") {
		t.Errorf("warnings = %v, want exactly one \"unrecognised line\" warning", warnings)
	}
}

// TestParseLogicalLayerBodyAbsorbBlock covers the "absorb" operation's own grammar (2026-09-18):
// a device's "capabilities from <device-id>: ... end;" block and its repeatable
// "<domain>.<label>: <bare-capability>;" lines -- the concrete appliance.washing_machine/
// discovery.washing_machine_switch shape from the approved plan.
func TestParseLogicalLayerBodyAbsorbBlock(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device appliance.washing_machine with:",
		"  absorb from discovery.washing_machine_switch:",
		"    capability switch.core            switch.core;",
		"    capability sensor.power           sensor.power;",
		"    capability sensor.energy          sensor.energy;",
		"    capability sensor.current         sensor.current;",
		"    capability binary_sensor.consumes binary_sensor.consumes;",
		"  end;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	d := devices[0]
	wantLabels := map[string]struct{ domain, capability string }{
		"core":     {"switch", "core"},
		"power":    {"sensor", "power"},
		"energy":   {"sensor", "energy"},
		"current":  {"sensor", "current"},
		"consumes": {"binary_sensor", "consumes"},
	}
	if len(d.Capabilities) != len(wantLabels) {
		t.Fatalf("Capabilities = %+v, want %d entries", d.Capabilities, len(wantLabels))
	}
	for label, want := range wantLabels {
		capEntry, ok := d.Capabilities[label]
		if !ok {
			t.Fatalf("Capabilities missing label %q: %+v", label, d.Capabilities)
		}
		if capEntry.Domain != want.domain {
			t.Errorf("Capabilities[%q].Domain = %q, want %q", label, capEntry.Domain, want.domain)
		}
		if capEntry.AbsorbedFromDeviceID != "discovery.washing_machine_switch" {
			t.Errorf("Capabilities[%q].AbsorbedFromDeviceID = %q, want discovery.washing_machine_switch", label, capEntry.AbsorbedFromDeviceID)
		}
		if capEntry.AbsorbedFromCapability != want.capability {
			t.Errorf("Capabilities[%q].AbsorbedFromCapability = %q, want %q", label, capEntry.AbsorbedFromCapability, want.capability)
		}
	}
}

// TestParseLogicalLayerBodyAbsorbBlockBareShorthand covers the 2026-09-19 "<domain>.<label>;"
// shorthand (no ":<value>" at all) -- registers identically to
// TestParseLogicalLayerBodyAbsorbBlock's explicit "<domain>.<label>: <value>;" form, valid because
// every label here is textually identical to the absorbed capability it names.
func TestParseLogicalLayerBodyAbsorbBlockBareShorthand(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device appliance.washing_machine with:",
		"  absorb from discovery.washing_machine_switch:",
		"    capability switch.core;",
		"    capability sensor.power;",
		"    capability binary_sensor.consumes;",
		"  end;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	wantLabels := map[string]struct{ domain, capability string }{
		"core":     {"switch", "core"},
		"power":    {"sensor", "power"},
		"consumes": {"binary_sensor", "consumes"},
	}
	d := devices[0]
	if len(d.Capabilities) != len(wantLabels) {
		t.Fatalf("Capabilities = %+v, want %d entries", d.Capabilities, len(wantLabels))
	}
	for label, want := range wantLabels {
		capEntry, ok := d.Capabilities[label]
		if !ok {
			t.Fatalf("Capabilities missing label %q: %+v", label, d.Capabilities)
		}
		if capEntry.Domain != want.domain || capEntry.AbsorbedFromDeviceID != "discovery.washing_machine_switch" || capEntry.AbsorbedFromCapability != want.capability {
			t.Errorf("Capabilities[%q] = %+v, want Domain=%q AbsorbedFromDeviceID=discovery.washing_machine_switch AbsorbedFromCapability=%q", label, capEntry, want.domain, want.capability)
		}
	}
}

// TestParseLogicalLayerBodyRejectsOldColonForm is the regression test for the 2026-09-19 grammar
// restriction: logicalCapabilityPattern briefly accepted an OPTIONAL ":" between <label> and
// <value> as a trial (both houses' Physical.def/Logical.def were then rewritten to the colon-less
// form and verified byte-identical on regenerate) -- now that the migration is complete, the old
// "switch.media: media_player.social:sonos;" colon form must be REJECTED outright, not silently
// tolerated.
func TestParseLogicalLayerBodyRejectsOldColonForm(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device appliance.sonos_complement with:",
		"  switch.media: media_player.social:sonos;",
		"end;",
	}, nil)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "unrecognised line") {
		t.Fatalf("expected exactly one \"unrecognised line\" warning for the old colon form, got: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	if _, ok := devices[0].Capabilities["media"]; ok {
		t.Errorf("Capabilities = %+v, want the old colon-form line rejected, not silently parsed", devices[0].Capabilities)
	}
}

// TestParseLogicalLayerBodyWarnsOnUnrecognisedAbsorbBodyLine covers a malformed line inside an
// absorb block (neither "enabler ...;" nor "<domain>.<label>: <bare-capability>;").
func TestParseLogicalLayerBodyWarnsOnUnrecognisedAbsorbBodyLine(t *testing.T) {
	_, warnings := parseLogicalLayerBody([]string{
		"device appliance.washing_machine with:",
		"  absorb from discovery.washing_machine_switch:",
		"    bogus_field 42;",
		"  end;",
		"end;",
	}, nil)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "absorb from") {
		t.Errorf("warnings = %v, want exactly one warning naming the absorb block", warnings)
	}
}

// TestParseLogicalLayerBodyDefinedInputNumber covers the "defined <domain>.<label> with: ...;
// end;" block (2026-09-18) -- the "windy" threshold's own shape, replacing Macros.def's "windy"
// macro's "${x} = entity input_number.social/x_threshold with: ...;" line.
func TestParseLogicalLayerBodyDefinedInputNumber(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device sensors.vienna_terrace_wind with:",
		"  defined capability input_number.windy_threshold with:",
		"    minimum 0;",
		"    maximum 30;",
		"    step 1;",
		"    icon mdi:weather-windy;",
		"    units km/h;",
		"  end;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	capEntry, ok := devices[0].Capabilities["windy_threshold"]
	if !ok {
		t.Fatalf("Capabilities = %+v, want a %q entry", devices[0].Capabilities, "windy_threshold")
	}
	if !capEntry.IsDefinedInputNumber || capEntry.Domain != "input_number" {
		t.Errorf("capEntry = %+v, want IsDefinedInputNumber=true Domain=input_number", capEntry)
	}
	if capEntry.DefinedMinimum != "0" || capEntry.DefinedMaximum != "30" || capEntry.DefinedStep != "1" || capEntry.DefinedIcon != "mdi:weather-windy" || capEntry.DefinedUnits != "km/h" {
		t.Errorf("capEntry = %+v, want all five defined fields populated", capEntry)
	}
}

// TestParseLogicalLayerBodyDefinedOneLiner covers the new one-liner shorthand for a "defined"
// block with a single field (2026-09-27, the generic with-clause dispatcher, with_clause.go) --
// rarely useful given every real case so far needs all five fields, but free once the shared
// statement list exists, and consistent with every other with-clause consumer gaining one.
func TestParseLogicalLayerBodyDefinedOneLiner(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device sensors.x with:",
		"  defined capability input_number.threshold with minimum 5;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	capEntry, ok := devices[0].Capabilities["threshold"]
	if !ok {
		t.Fatalf("Capabilities = %+v, want a %q entry", devices[0].Capabilities, "threshold")
	}
	if !capEntry.IsDefinedInputNumber || capEntry.Domain != "input_number" || capEntry.DefinedMinimum != "5" {
		t.Errorf("capEntry = %+v, want IsDefinedInputNumber=true Domain=input_number DefinedMinimum=5", capEntry)
	}
}

// TestParseLogicalLayerBodyDerivedCondition covers the "derived <domain>.<label> with: condition
// jinja "<expr-with-$[domain.label]-placeholders>"; device_class ...; delay_on ...; delay_off ...;
// end;" block (2026-09-18, grammar redesigned 2026-09-24, named placeholders 2026-10-05,
// PROJECT.md 1h) -- the "windy" capability's own shape, replacing Macros.def's "windy" macro's
// "condition ${entity} ${windy_threshold} ...;" line.
func TestParseLogicalLayerBodyDerivedCondition(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device sensors.vienna_terrace_wind with:",
		`  derived capability binary_sensor.windy with:`,
		`    condition jinja "($[sensor.wind_speed] in ['unknown', 'unavailable']) or (($[sensor.wind_speed] | int) > ($[input_number.windy_threshold] | int))";`,
		"    device_class wind;",
		"    delay_on  00:01:00;",
		"    delay_off 00:10:00;",
		"  end;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	capEntry, ok := devices[0].Capabilities["windy"]
	if !ok {
		t.Fatalf("Capabilities = %+v, want a %q entry", devices[0].Capabilities, "windy")
	}
	if !capEntry.IsDerivedCondition || capEntry.Domain != "binary_sensor" {
		t.Errorf("capEntry = %+v, want IsDerivedCondition=true Domain=binary_sensor", capEntry)
	}
	if capEntry.DerivedCondition == nil || capEntry.DerivedCondition.Kind != CondJinja {
		t.Fatalf("DerivedCondition = %+v, want a single CondJinja node", capEntry.DerivedCondition)
	}
	wantExpr := "($[sensor.wind_speed] in ['unknown', 'unavailable']) or (($[sensor.wind_speed] | int) > ($[input_number.windy_threshold] | int))"
	if capEntry.DerivedCondition.JinjaExpr != wantExpr {
		t.Errorf("JinjaExpr = %q, want %q", capEntry.DerivedCondition.JinjaExpr, wantExpr)
	}
	if len(capEntry.DerivedCondition.JinjaSources) != 0 {
		t.Errorf("JinjaSources = %v, want nil/empty (a literal-string head embeds its own sources)", capEntry.DerivedCondition.JinjaSources)
	}
	if capEntry.DerivedDeviceClass != "wind" || capEntry.DelayOn != "00:01:00" || capEntry.DelayOff != "00:10:00" {
		t.Errorf("capEntry = %+v, want DerivedDeviceClass=wind DelayOn=00:01:00 DelayOff=00:10:00", capEntry)
	}
}

// TestParseLogicalLayerBodyDefinedAndDerivedCoexist covers the full real "windy" device shape:
// both a "defined" and a "derived" block on the same device, alongside each other.
func TestParseLogicalLayerBodyDefinedAndDerivedCoexist(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device sensors.vienna_terrace_wind with:",
		"  defined capability input_number.windy_threshold with:",
		"    minimum 0;",
		"    maximum 30;",
		"    step 1;",
		"    icon mdi:weather-windy;",
		"    units km/h;",
		"  end;",
		`  derived capability binary_sensor.windy with:`,
		`    condition jinja "($[sensor.wind_speed]|int) > ($[input_number.windy_threshold]|int)";`,
		"    device_class wind;",
		"    delay_on  00:01:00;",
		"    delay_off 00:10:00;",
		"  end;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 || len(devices[0].Capabilities) != 2 {
		t.Fatalf("got %+v, want 1 device with 2 capabilities", devices)
	}
}

// TestSubstituteSettingsVariables covers Logical.def's own "${name}" resolution (2026-09-18) --
// unlike Conceptual.def/Macros.def, Logical.def has no per-field or macro-argument resolution of
// its own, so collectLogicalDevicesByID substitutes over the whole raw body once, before parsing.
func TestSubstituteSettingsVariables(t *testing.T) {
	settings := map[string]string{"wind_speed_min": "0", "windy_device_class": `"safety"`}
	got := substituteSettingsVariables(`minimum ${wind_speed_min}; device_class ${windy_device_class};`, settings)
	want := `minimum 0; device_class safety;`
	if got != want {
		t.Errorf("substituteSettingsVariables = %q, want %q", got, want)
	}
}

// TestSubstituteSettingsVariablesLeavesUnknownNameUnchanged mirrors resolveSettingsVar's own
// silent-passthrough convention -- an unresolvable "${name}" stays literal rather than erroring.
func TestSubstituteSettingsVariablesLeavesUnknownNameUnchanged(t *testing.T) {
	got := substituteSettingsVariables(`icon ${bogus_unknown_name};`, map[string]string{})
	want := `icon ${bogus_unknown_name};`
	if got != want {
		t.Errorf("substituteSettingsVariables = %q, want %q (unchanged)", got, want)
	}
}

// TestParseLogicalLayerBodyDerivedConditionJinjaNamedTemplateCall covers the "condition jinja
// ${name}(args) { ... };" alternative head (2026-09-19, appliance.sonos_roam's own battery_alert;
// folded into logical_condition_expr.go's own grammar 2026-09-24) -- reuses the SAME Settings.def
// named jinja templates Physical.def's own "derived ... via jinja ${name}(args);" already uses,
// rewriting the template's own "$" placeholder to "$1" (this form's "{ ... }" list is always
// exactly one entry).
func TestParseLogicalLayerBodyDerivedConditionJinjaNamedTemplateCall(t *testing.T) {
	jinjaTemplates := map[string]TJinjaTemplateDefinition{
		"int_less_then": {Params: []string{"i"}, Template: "'on' if (($ | int(0)) < ${i}) else 'off'"},
	}
	devices, warnings := parseLogicalLayerBody([]string{
		"device appliance.sonos_roam with:",
		"  derived capability binary_sensor.battery_alert with:",
		"    condition jinja (sensor.infrastructural_apartment_bedroom_sonos_battery_level) ${int_less_then}(10);",
		"  end;",
		"end;",
	}, jinjaTemplates)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	capEntry, ok := devices[0].Capabilities["battery_alert"]
	if !ok || !capEntry.IsDerivedCondition {
		t.Fatalf("Capabilities = %+v, want a derived-condition \"battery_alert\" entry", devices[0].Capabilities)
	}
	if capEntry.DerivedCondition == nil || capEntry.DerivedCondition.Kind != CondJinja {
		t.Fatalf("DerivedCondition = %+v, want a single CondJinja node", capEntry.DerivedCondition)
	}
	if capEntry.DerivedCondition.JinjaExpr != "'on' if (($1 | int(0)) < 10) else 'off'" {
		t.Errorf("JinjaExpr = %q, want the resolved named jinja template", capEntry.DerivedCondition.JinjaExpr)
	}
	wantOver := []string{"sensor.infrastructural_apartment_bedroom_sonos_battery_level"}
	gotOver := capEntry.DerivedCondition.JinjaSources
	if len(gotOver) != 1 || gotOver[0] != wantOver[0] {
		t.Errorf("JinjaSources = %v, want %v", gotOver, wantOver)
	}
}

// TestParseLogicalLayerBodyDerivedConditionJinjaNamedTemplateCallWarnsOnUndeclaredTemplate mirrors
// the Physical.def sibling's own behaviour: an undeclared jinja template name is reported, and the
// capability is left unregistered rather than silently guessed at.
func TestParseLogicalLayerBodyDerivedConditionJinjaNamedTemplateCallWarnsOnUndeclaredTemplate(t *testing.T) {
	_, warnings := parseLogicalLayerBody([]string{
		"device appliance.sonos_roam with:",
		"  derived capability binary_sensor.battery_alert with:",
		"    condition jinja (sensor.battery_level) ${no_such_template}(10);",
		"  end;",
		"end;",
	}, map[string]TJinjaTemplateDefinition{})
	if len(warnings) != 1 || !strings.Contains(warnings[0], "no_such_template") {
		t.Fatalf("warnings = %v, want exactly one mentioning \"no_such_template\"", warnings)
	}
}

// TestRegisterLogicalDerivedConditionCapabilityResolvesLiteralEntityInOver is a regression test
// for appliance.sonos_roam's own battery_alert (2026-09-19): an "over:" entry that doesn't resolve
// as a sibling capability label in any of the three lookups, but looks like an already-qualified
// entity_id (contains a "."), is used verbatim -- covering a bare, independently-existing entity
// like a media_player's own battery_level attribute, which is not a capability of this device.
func TestRegisterLogicalDerivedConditionCapabilityResolvesLiteralEntityInOver(t *testing.T) {
	admin := newAdministrationState()
	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "binary_sensor.infrastructural:battery_alert", DeviceID: "appliance.sonos_roam", Capability: "binary_sensor.battery_alert"}
	capability := TLogicalCapability{
		Domain:             "binary_sensor",
		IsDerivedCondition: true,
		DerivedCondition:   testJinjaCondition("'on' if (($1 | int(0)) < 10) else 'off'", "sensor.infrastructural_apartment_bedroom_sonos_battery_level"),
	}

	warnings, deferred := registerLogicalDerivedConditionCapability(admin, decl, capability, "battery_alert", "sonos", "Logical.def", 1, true, nil, nil, nil, nil, nil, nil)
	if deferred {
		t.Fatalf("did not expect deferral -- a literal entity_id never needs to wait on anything")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	rec, found := findEntityRecordByName(admin, "binary_sensor.infrastructural/sonos/battery_alert")
	if !found {
		t.Fatalf("no entity record registered for binary_sensor.infrastructural/sonos/battery_alert")
	}
	if !strings.Contains(rec.ConditionExpr, "sensor.infrastructural_apartment_bedroom_sonos_battery_level") {
		t.Errorf("ConditionExpr = %q, want it to reference the literal entity_id used verbatim", rec.ConditionExpr)
	}
}

// TestParseLogicalLayerBodyValueAcceptsEntityAttributeLiteral is the regression test for the
// 2026-09-21 fix (the "!" charset addition, now on the new grammar's bare-ref token, 2026-09-24):
// environment.weather's own "pressure" capability, sourced from weather.forecast's own "pressure"
// attribute -- a literal, already-existing main-instance entity!attribute reference, not a sibling
// capability of this device. "pressure" is a "sensor" domain, so it uses "value", not "condition"
// (the exact bug this whole grammar redesign fixes).
func TestParseLogicalLayerBodyValueAcceptsEntityAttributeLiteral(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device environment.weather with:",
		`  derived capability sensor.pressure with:`,
		`    value weather.forecast!pressure;`,
		"  end;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	capEntry, ok := devices[0].Capabilities["pressure"]
	if !ok {
		t.Fatalf("Capabilities = %+v, want a %q entry", devices[0].Capabilities, "pressure")
	}
	if !capEntry.IsDerivedValue || capEntry.Domain != "sensor" {
		t.Errorf("capEntry = %+v, want IsDerivedValue=true Domain=sensor", capEntry)
	}
	if capEntry.DerivedValue == nil || capEntry.DerivedValue.Kind != CondRef || capEntry.DerivedValue.Ref != "weather.forecast!pressure" {
		t.Errorf("DerivedValue = %+v, want a CondRef leaf for weather.forecast!pressure", capEntry.DerivedValue)
	}
}

// TestRegisterLogicalDerivedConditionCapabilityResolvesLiteralEntityAttributeInOver mirrors
// TestRegisterLogicalDerivedConditionCapabilityResolvesLiteralEntityInOver for the "!attribute"
// form -- confirms the literal-entity_id fallback (registerLogicalDerivedConditionCapability,
// Conceptual_LogicalEntities.go) passes an "entity!attribute" token through verbatim into the
// rendered expression, unmodified except for generator.go's own sourceToJinja2 turning it into
// state_attr(...).
func TestRegisterLogicalDerivedConditionCapabilityResolvesLiteralEntityAttributeInOver(t *testing.T) {
	admin := newAdministrationState()
	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.infrastructural:pressure", DeviceID: "environment.weather", Capability: "sensor.pressure"}
	capability := TLogicalCapability{
		Domain:         "sensor",
		IsDerivedValue: true,
		DerivedValue:   testRefCondition("weather.forecast!pressure", false),
	}

	warnings, deferred := registerLogicalDerivedConditionCapability(admin, decl, capability, "pressure", "weather", "Logical.def", 1, true, nil, nil, nil, nil, nil, nil)
	if deferred {
		t.Fatalf("did not expect deferral -- a literal entity_id!attribute never needs to wait on anything")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	rec, found := findEntityRecordByName(admin, "sensor.infrastructural/weather/pressure")
	if !found {
		t.Fatalf("no entity record registered for sensor.infrastructural/weather/pressure")
	}
	if rec.ConditionExpr != "state_attr('weather.forecast', 'pressure')" {
		t.Errorf("ConditionExpr = %q, want state_attr('weather.forecast', 'pressure')", rec.ConditionExpr)
	}
}

// TestParseLogicalLayerBodySwitchedBy covers the block-form "<domain>.<label> is switched by
// <domain>.<label> from <device-id>;" line (2026-09-27, the "switched_device" migration).
func TestParseLogicalLayerBodySwitchedBy(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device sensors.server_room_main_spots with:",
		"  light.core is switched by light.light_2 from switches.house_server_room_fuse_box_zwave_048;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	d := devices[0]
	fact, ok := d.SwitchedBy["core"]
	if !ok {
		t.Fatalf("SwitchedBy[%q] missing, got %v", "core", d.SwitchedBy)
	}
	want := TLogicalSwitchedByFact{
		OwnDomain:      "light",
		SourceDomain:   "light",
		SourceLabel:    "light_2",
		SourceDeviceID: "switches.house_server_room_fuse_box_zwave_048",
	}
	if fact != want {
		t.Errorf("SwitchedBy[core] = %+v, want %+v", fact, want)
	}
}

// TestParseLogicalLayerBodyOneLinerSwitchedBy covers the single-statement "with <domain>.<label>
// is switched by <domain>.<label> from <device-id>;" shorthand.
func TestParseLogicalLayerBodyOneLinerSwitchedBy(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device sensors.corridor_main_spots with light.core is switched by light.light_1 from switches.house_server_room_fuse_box_zwave_048;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	fact, ok := devices[0].SwitchedBy["core"]
	if !ok {
		t.Fatalf("SwitchedBy[%q] missing, got %v", "core", devices[0].SwitchedBy)
	}
	want := TLogicalSwitchedByFact{
		OwnDomain:      "light",
		SourceDomain:   "light",
		SourceLabel:    "light_1",
		SourceDeviceID: "switches.house_server_room_fuse_box_zwave_048",
	}
	if fact != want {
		t.Errorf("SwitchedBy[core] = %+v, want %+v", fact, want)
	}
}

func TestCollectLogicalDevicesByIDDuplicateWarns(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device node.a with:",
		"  dependency on host.b;",
		"end;",
		"device node.a with:",
		"  dependency on host.c;",
		"end;",
	}, nil)
	if len(devices) != 2 {
		t.Fatalf("parseLogicalLayerBody itself doesn't dedupe -- got %d devices, want 2 (collectLogicalDevicesByID does the deduping)", len(devices))
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings from the parser itself: %v", warnings)
	}
}

// TestParseLogicalLayerBodyDerivedOneLinerMatchesBlockForm covers the 2026-10-21 one-liner
// "derived capability <domain>.<label> with condition <expr>;" -- it must yield exactly the
// capability the equivalent "with: condition <expr>; end;" block does, also when its expression
// continues on a following line, and leave the rest of the device body parsed as before.
func TestParseLogicalLayerBodyDerivedOneLinerMatchesBlockForm(t *testing.T) {
	parse := func(lines ...string) TLogicalDevice {
		t.Helper()
		devices, warnings := parseLogicalLayerBody(lines, nil)
		if len(warnings) != 0 || len(devices) != 1 {
			t.Fatalf("got %d devices, warnings %v; want 1 device, no warnings", len(devices), warnings)
		}
		return devices[0]
	}
	block := parse(
		"device appliance.sonos with:",
		"  derived capability binary_sensor.node with:",
		"    condition physical appliance.sonos and node.sonos_left and node.sonos_right;",
		"  end;",
		"  dependency on node.sonos_left;",
		"end;",
	)
	oneLiner := parse(
		"device appliance.sonos with:",
		"  derived capability binary_sensor.node with condition physical appliance.sonos and node.sonos_left and node.sonos_right;",
		"  dependency on node.sonos_left;",
		"end;",
	)
	continued := parse(
		"device appliance.sonos with:",
		"  derived capability binary_sensor.node with condition physical appliance.sonos and",
		"    node.sonos_left and node.sonos_right;",
		"  dependency on node.sonos_left;",
		"end;",
	)
	if !reflect.DeepEqual(oneLiner, block) {
		t.Errorf("one-liner = %+v, want the block form's %+v", oneLiner, block)
	}
	if !reflect.DeepEqual(continued, block) {
		t.Errorf("continued one-liner = %+v, want the block form's %+v", continued, block)
	}
}

// TestParseLogicalLayerBodyDerivedOneLinerRejectsOtherFields: only "condition"/"value" fit the
// one-liner; anything else (device_class, delays) needs the block form.
func TestParseLogicalLayerBodyDerivedOneLinerRejectsOtherFields(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device appliance.sonos with:",
		"  derived capability binary_sensor.node with delay_off 00:01:00;",
		"end;",
	}, nil)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "block form") {
		t.Errorf("warnings = %v, want one pointing at the block form", warnings)
	}
	if len(devices) != 1 || len(devices[0].Capabilities) != 0 {
		t.Errorf("devices = %+v, want the device without any capability", devices)
	}
}
