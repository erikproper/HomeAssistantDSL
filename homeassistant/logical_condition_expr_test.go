package main

import (
	"strings"
	"testing"
)

func TestParseConditionExprBareRef(t *testing.T) {
	tree, warnings := parseConditionExpr("binary_sensor.x", nil, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if tree.Kind != CondRef || tree.Ref != "binary_sensor.x" || tree.IsAvailable {
		t.Errorf("tree = %+v, want CondRef{Ref: binary_sensor.x, IsAvailable: false}", tree)
	}
}

// TestParseConditionExprFromDeviceID covers the 2026-09-24 cross-device reference clause: "<label>
// from <device-id>" targets an explicitly OTHER device's own capability instead of a same-device
// sibling label.
func TestParseConditionExprFromDeviceID(t *testing.T) {
	tree, warnings := parseConditionExpr("switch.core from switch.washing_machine", nil, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if tree.Kind != CondRef || tree.Ref != "switch.core" || tree.FromDeviceID != "switch.washing_machine" || tree.ForcePhysical {
		t.Errorf("tree = %+v, want CondRef{Ref: switch.core, FromDeviceID: switch.washing_machine, ForcePhysical: false}", tree)
	}
}

// TestParseConditionExprFromPhysicalDeviceID covers the "from physical <device-id>" force -- the
// escape hatch a device's own override condition needs to reach its own un-overridden physical
// value (e.g. appliance.washing_machine's own "node from physical appliance.washing_machine").
func TestParseConditionExprFromPhysicalDeviceID(t *testing.T) {
	tree, warnings := parseConditionExpr("node from physical appliance.washing_machine", nil, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if tree.Kind != CondRef || tree.Ref != "node" || tree.FromDeviceID != "appliance.washing_machine" || !tree.ForcePhysical {
		t.Errorf("tree = %+v, want CondRef{Ref: node, FromDeviceID: appliance.washing_machine, ForcePhysical: true}", tree)
	}
}

// TestParseConditionExprBarePhysicalShorthand covers the 2026-09-24 "physical <device-id>"
// shorthand -- sugar for "node from physical <device-id>", the single most common shape of the
// "from" clause, letting an override condition reach its own device's raw un-overridden "node"
// capability without spelling out "node from".
func TestParseConditionExprBarePhysicalShorthand(t *testing.T) {
	tree, warnings := parseConditionExpr("physical appliance.front_door_ring", nil, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	want, warnings := parseConditionExpr("node from physical appliance.front_door_ring", nil, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings for the long form: %v", warnings)
	}
	if tree.Kind != want.Kind || tree.Ref != want.Ref || tree.FromDeviceID != want.FromDeviceID || tree.ForcePhysical != want.ForcePhysical || tree.IsAvailable != want.IsAvailable {
		t.Errorf("tree = %+v, want the same shape as the long form %+v", tree, want)
	}
	if tree.Ref != "node" || tree.FromDeviceID != "appliance.front_door_ring" || !tree.ForcePhysical {
		t.Errorf("tree = %+v, want CondRef{Ref: node, FromDeviceID: appliance.front_door_ring, ForcePhysical: true}", tree)
	}
}

// TestParseConditionExprBarePhysicalShorthandThenIsAvailable confirms the shorthand also accepts a
// trailing "is available", same as the long form.
func TestParseConditionExprBarePhysicalShorthandThenIsAvailable(t *testing.T) {
	tree, warnings := parseConditionExpr("physical appliance.front_door_ring is available", nil, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if tree.Ref != "node" || tree.FromDeviceID != "appliance.front_door_ring" || !tree.ForcePhysical || !tree.IsAvailable {
		t.Errorf("tree = %+v, want CondRef{Ref: node, FromDeviceID: appliance.front_door_ring, ForcePhysical: true, IsAvailable: true}", tree)
	}
}

// TestParseConditionExprFromDeviceIDThenIsAvailable confirms "from [physical] <device-id>" and "is
// available" compose on the same ref (order: ref, then from-clause, then is-available suffix).
func TestParseConditionExprFromDeviceIDThenIsAvailable(t *testing.T) {
	tree, warnings := parseConditionExpr("node from physical appliance.washing_machine is available", nil, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if tree.FromDeviceID != "appliance.washing_machine" || !tree.ForcePhysical || !tree.IsAvailable {
		t.Errorf("tree = %+v, want FromDeviceID=appliance.washing_machine ForcePhysical=true IsAvailable=true", tree)
	}
}

// TestParseConditionExprFullWashingMachineExample is the user's own worked example end to end
// (parser layer only -- resolution is covered separately in Conceptual_LogicalEntities_test.go).
func TestParseConditionExprFullWashingMachineExample(t *testing.T) {
	tree, warnings := parseConditionExpr(
		`(node from physical appliance.washing_machine and node.candy) or not switch.core from switch.washing_machine`, nil, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if tree.Kind != CondOr {
		t.Fatalf("tree.Kind = %v, want CondOr", tree.Kind)
	}
	and := tree.Left
	if and.Kind != CondAnd {
		t.Fatalf("tree.Left.Kind = %v, want CondAnd", and.Kind)
	}
	if and.Left.Ref != "node" || and.Left.FromDeviceID != "appliance.washing_machine" || !and.Left.ForcePhysical {
		t.Errorf("and.Left = %+v, want the \"node from physical appliance.washing_machine\" ref", and.Left)
	}
	if and.Right.Ref != "node.candy" || and.Right.FromDeviceID != "" {
		t.Errorf("and.Right = %+v, want a bare \"node.candy\" ref with no from-clause", and.Right)
	}
	not := tree.Right
	if not.Kind != CondNot {
		t.Fatalf("tree.Right.Kind = %v, want CondNot", not.Kind)
	}
	if not.Child.Ref != "switch.core" || not.Child.FromDeviceID != "switch.washing_machine" {
		t.Errorf("not.Child = %+v, want \"switch.core from switch.washing_machine\"", not.Child)
	}
}

func TestParseConditionExprIsAvailableSuffix(t *testing.T) {
	tree, warnings := parseConditionExpr("weather.forecast is available", nil, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if tree.Kind != CondRef || tree.Ref != "weather.forecast" || !tree.IsAvailable {
		t.Errorf("tree = %+v, want CondRef{Ref: weather.forecast, IsAvailable: true}", tree)
	}
}

func TestParseConditionExprJinjaClause(t *testing.T) {
	tree, warnings := parseConditionExpr(`jinja "$1 < 10" { sensor.s }`, nil, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if tree.Kind != CondJinja || tree.JinjaExpr != "$1 < 10" || len(tree.JinjaSources) != 1 || tree.JinjaSources[0] != "sensor.s" {
		t.Errorf("tree = %+v, want CondJinja{JinjaExpr: \"$1 < 10\", JinjaSources: [sensor.s]}", tree)
	}
}

func TestParseConditionExprJinjaClauseMultipleSources(t *testing.T) {
	tree, warnings := parseConditionExpr(`jinja "($1 | int) > ($2 | int)" { illuminance, sunny_threshold }`, nil, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	want := []string{"illuminance", "sunny_threshold"}
	if tree.Kind != CondJinja || len(tree.JinjaSources) != 2 || tree.JinjaSources[0] != want[0] || tree.JinjaSources[1] != want[1] {
		t.Errorf("tree = %+v, want JinjaSources=%v", tree, want)
	}
}

func TestParseConditionExprJinjaNamedTemplateCall(t *testing.T) {
	jinjaTemplates := map[string]TJinjaTemplateDefinition{
		"int_less_then": {Params: []string{"i"}, Template: "'on' if (($ | int(0)) < ${i}) else 'off'"},
	}
	tree, warnings := parseConditionExpr("jinja ${int_less_then}(10) { sensor.battery_level }", jinjaTemplates, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if tree.Kind != CondJinja || tree.JinjaExpr != "'on' if (($1 | int(0)) < 10) else 'off'" {
		t.Errorf("tree = %+v, want the resolved named template", tree)
	}
}

func TestParseConditionExprJinjaNamedTemplateCallUndeclared(t *testing.T) {
	_, warnings := parseConditionExpr("jinja ${no_such}(10) { sensor.x }", map[string]TJinjaTemplateDefinition{}, "test")
	if len(warnings) != 1 || !strings.Contains(warnings[0], "no_such") {
		t.Fatalf("warnings = %v, want exactly one mentioning \"no_such\"", warnings)
	}
}

func TestParseConditionExprJinjaNamedTemplateCallWrongArgCount(t *testing.T) {
	jinjaTemplates := map[string]TJinjaTemplateDefinition{
		"int_less_then": {Params: []string{"i"}, Template: "'on' if (($ | int(0)) < ${i}) else 'off'"},
	}
	_, warnings := parseConditionExpr("jinja ${int_less_then}(10, 20) { sensor.x }", jinjaTemplates, "test")
	if len(warnings) != 1 || !strings.Contains(warnings[0], "2 argument") {
		t.Fatalf("warnings = %v, want exactly one warning naming the wrong argument count", warnings)
	}
}

func TestParseConditionExprBooleanComposition(t *testing.T) {
	tree, warnings := parseConditionExpr(
		`binary_sensor.x and ( not c or jinja "$1 < 10" { sensor.s } )`, nil, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if tree.Kind != CondAnd {
		t.Fatalf("tree.Kind = %v, want CondAnd", tree.Kind)
	}
	if tree.Left.Kind != CondRef || tree.Left.Ref != "binary_sensor.x" {
		t.Errorf("tree.Left = %+v, want CondRef{Ref: binary_sensor.x}", tree.Left)
	}
	or := tree.Right
	if or.Kind != CondOr {
		t.Fatalf("tree.Right.Kind = %v, want CondOr", or.Kind)
	}
	if or.Left.Kind != CondNot || or.Left.Child.Kind != CondRef || or.Left.Child.Ref != "c" {
		t.Errorf("or.Left = %+v, want CondNot{Child: CondRef{Ref: c}}", or.Left)
	}
	if or.Right.Kind != CondJinja || or.Right.JinjaExpr != "$1 < 10" {
		t.Errorf("or.Right = %+v, want CondJinja{JinjaExpr: \"$1 < 10\"}", or.Right)
	}
}

// TestParseConditionExprPrecedence confirms "not" binds tighter than "and", which binds tighter
// than "or" -- "a or b and c" must parse as Or(a, And(b, c)), not And(Or(a, b), c).
func TestParseConditionExprPrecedence(t *testing.T) {
	tree, warnings := parseConditionExpr("a or b and c", nil, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if tree.Kind != CondOr || tree.Left.Ref != "a" {
		t.Fatalf("tree = %+v, want Or(a, ...)", tree)
	}
	if tree.Right.Kind != CondAnd || tree.Right.Left.Ref != "b" || tree.Right.Right.Ref != "c" {
		t.Errorf("tree.Right = %+v, want And(b, c)", tree.Right)
	}
}

func TestParseConditionExprTrailingGarbageWarns(t *testing.T) {
	_, warnings := parseConditionExpr("binary_sensor.x extra_junk", nil, "test")
	if len(warnings) != 1 || !strings.Contains(warnings[0], "unexpected trailing input") {
		t.Fatalf("warnings = %v, want exactly one \"unexpected trailing input\" warning", warnings)
	}
}

func TestParseValueExprBareRef(t *testing.T) {
	tree, warnings := parseValueExpr("weather.forecast_junglinster!pressure", nil, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if tree.Kind != CondRef || tree.Ref != "weather.forecast_junglinster!pressure" {
		t.Errorf("tree = %+v, want CondRef{Ref: weather.forecast_junglinster!pressure}", tree)
	}
}

func TestParseValueExprJinjaClause(t *testing.T) {
	tree, warnings := parseValueExpr(`jinja "$1" { weather.forecast_junglinster!pressure }`, nil, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if tree.Kind != CondJinja || tree.JinjaExpr != "$1" || len(tree.JinjaSources) != 1 {
		t.Errorf("tree = %+v, want a single CondJinja source", tree)
	}
}

// TestParseValueExprRejectsBooleanComposition confirms "value" never accepts and/or/not/parens --
// a sensor value isn't boolean-composable, so ParseAtom (not ParseOrExpr) is the entry point.
func TestParseValueExprRejectsBooleanComposition(t *testing.T) {
	_, warnings := parseValueExpr("binary_sensor.x and binary_sensor.y", nil, "test")
	if len(warnings) != 1 || !strings.Contains(warnings[0], "unexpected trailing input") {
		t.Fatalf("warnings = %v, want exactly one \"unexpected trailing input\" warning (a lone ref, then \"and ...\" left over)", warnings)
	}
}

// TestRegisterLogicalDerivedConditionCapabilityRejectsConditionOnNonBinarySensor is the concrete
// regression test for the bug that motivated this whole redesign: both houses' Logical.def used
// "condition" for a "sensor" domain derivation (sensor.pressure) -- this must now be rejected with
// a clear warning telling the author to use "value" instead.
func TestRegisterLogicalDerivedConditionCapabilityRejectsConditionOnNonBinarySensor(t *testing.T) {
	admin := newAdministrationState()
	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.infrastructural:pressure", DeviceID: "environment.weather", Capability: "sensor.pressure"}
	capability := TLogicalCapability{
		Domain:             "sensor",
		IsDerivedCondition: true,
		DerivedCondition:   testRefCondition("weather.forecast!pressure", false),
	}
	warnings, deferred := registerLogicalDerivedConditionCapability(admin, decl, capability, "pressure", "weather", "Logical.def", 1, true, nil, nil, nil, nil, nil, nil)
	if deferred {
		t.Fatalf("did not expect deferral -- domain validation happens before any sibling resolution")
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "value") {
		t.Fatalf("warnings = %v, want exactly one warning telling the author to use \"value\"", warnings)
	}
}

// TestRegisterLogicalDerivedConditionCapabilityRejectsValueOnBinarySensor is the inverse check --
// "value" may never be used for a "binary_sensor" domain (that's what "condition" is for).
func TestRegisterLogicalDerivedConditionCapabilityRejectsValueOnBinarySensor(t *testing.T) {
	admin := newAdministrationState()
	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "binary_sensor.infrastructural:node", DeviceID: "environment.weather", Capability: "binary_sensor.node"}
	capability := TLogicalCapability{
		Domain:         "binary_sensor",
		IsDerivedValue: true,
		DerivedValue:   testRefCondition("weather.forecast", true),
	}
	warnings, deferred := registerLogicalDerivedConditionCapability(admin, decl, capability, "node", "weather", "Logical.def", 1, true, nil, nil, nil, nil, nil, nil)
	if deferred {
		t.Fatalf("did not expect deferral -- domain validation happens before any sibling resolution")
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "condition") {
		t.Fatalf("warnings = %v, want exactly one warning telling the author to use \"condition\"", warnings)
	}
}

// TestRegisterLogicalDerivedConditionCapabilityPerClauseLocalDollarScoping is the key semantic
// regression test for the new grammar: two independent "jinja ... { ... }" clauses composed in one
// "condition" must each resolve their OWN "$1"/"$2" placeholders from their OWN "{ ... }" list --
// never a single global list, which is what makes composing multiple jinja clauses in one
// expression unambiguous (the user's own explicit design).
func TestRegisterLogicalDerivedConditionCapabilityPerClauseLocalDollarScoping(t *testing.T) {
	admin := newAdministrationState()
	admin.LogicalEntityLinks["sensor.social_a"] = TLogicalEntityLink{DeviceID: "thing.x", Label: "a"}
	admin.LogicalEntityLinks["sensor.social_b"] = TLogicalEntityLink{DeviceID: "thing.x", Label: "b"}
	admin.LogicalEntityLinks["sensor.social_c"] = TLogicalEntityLink{DeviceID: "thing.x", Label: "c"}

	tree, warnings := parseConditionExpr(`jinja "$1 < 10" { a } and jinja "($1|int) > ($2|int)" { b, c }`, nil, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected parse warnings: %v", warnings)
	}

	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "binary_sensor.social:combined", DeviceID: "thing.x", Capability: "binary_sensor.combined"}
	capability := TLogicalCapability{Domain: "binary_sensor", IsDerivedCondition: true, DerivedCondition: tree}
	regWarnings, deferred := registerLogicalDerivedConditionCapability(admin, decl, capability, "combined", "x", "Logical.def", 1, true, nil, nil, nil, nil, nil, nil)
	if deferred {
		t.Fatalf("did not expect deferral -- all three siblings are already positioned")
	}
	if len(regWarnings) != 0 {
		t.Fatalf("unexpected warnings: %v", regWarnings)
	}

	rec, found := findEntityRecordByName(admin, "binary_sensor.social/x/combined")
	if !found {
		t.Fatalf("no entity record registered for binary_sensor.social/x/combined")
	}
	// "a" ($1 of the first clause) must be substituted into "$1 < 10"; "b"/"c" ($1/$2 of the SECOND
	// clause) must be substituted into "($1|int) > ($2|int)" -- NOT "a" leaking into the second
	// clause's own $1, which a naive global-numbering scheme would produce.
	wantFirst := "states('sensor.social_a') < 10"
	wantSecond := "(states('sensor.social_b')|int) > (states('sensor.social_c')|int)"
	if !strings.Contains(rec.ConditionExpr, wantFirst) {
		t.Errorf("ConditionExpr = %q, want it to contain %q (first clause's own local $1)", rec.ConditionExpr, wantFirst)
	}
	if !strings.Contains(rec.ConditionExpr, wantSecond) {
		t.Errorf("ConditionExpr = %q, want it to contain %q (second clause's own local $1/$2)", rec.ConditionExpr, wantSecond)
	}
}
