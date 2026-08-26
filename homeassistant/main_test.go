/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Main/Test
 *
 * This component provides smoke tests for interpretation, expansion, and availability checking.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 24.03.2026
 *
 */

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHomeAssistantEntityIDMapping(t *testing.T) {
	testCases := []struct {
		fullName string
		expected string
	}{
		{fullName: "sun.[sun]", expected: "sun.sun"},
		{fullName: "binary_sensor.social/entrance/front_door/ding", expected: "binary_sensor.social_entrance_front_door_ding"},
		{fullName: "sensor.infrastructural/house/server_room/xanadu/temperature", expected: "sensor.infrastructural_house_server_room_xanadu_temperature"},
	}

	for _, testCase := range testCases {
		actual := toHomeAssistantEntityID(testCase.fullName)
		if actual != testCase.expected {
			t.Fatalf("unexpected entity id mapping for %q: got %q, expected %q", testCase.fullName, actual, testCase.expected)
		}
	}
}

func TestExtractEntityIDsFromStatesPayload(t *testing.T) {
	payload := []byte(`[
  {"entity_id":"sun.sun","state":"above_horizon"},
  {"entity_id":"sensor.outdoor_temperature","state":"17"}
]`)

	entityIDs, err := extractEntityIDsFromStatesPayload(payload)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if !entityIDs["sun.sun"] {
		t.Fatalf("expected sun.sun to be present")
	}
	if !entityIDs["sensor.outdoor_temperature"] {
		t.Fatalf("expected sensor.outdoor_temperature to be present")
	}
}

func TestParseSpaceHeaderRecognizesVirtualSpace(t *testing.T) {
	kind, name, isArea, ok := parseSpaceHeader("virtual space social:extension with:")
	if !ok {
		t.Fatalf("expected virtual space header to parse")
	}
	if kind != SpaceKindVirtual {
		t.Fatalf("expected kind %q, got %q", SpaceKindVirtual, kind)
	}
	if name != "social:extension" {
		t.Fatalf("expected virtual space name, got %q", name)
	}
	if isArea {
		t.Fatalf("expected isArea = false for a plain space header")
	}

	kind, name, isArea, ok = parseSpaceHeader("space social:house with:")
	if !ok {
		t.Fatalf("expected regular space header to parse")
	}
	if kind != SpaceKindRegular {
		t.Fatalf("expected kind %q, got %q", SpaceKindRegular, kind)
	}
	if name != "social:house" {
		t.Fatalf("expected space name, got %q", name)
	}
	if isArea {
		t.Fatalf("expected isArea = false for a plain space header")
	}
}

func TestParseSpaceHeaderRecognizesAsArea(t *testing.T) {
	kind, name, isArea, ok := parseSpaceHeader("space infrastructural:rack as area with:")
	if !ok {
		t.Fatalf("expected \"as area\" space header to parse")
	}
	if kind != SpaceKindRegular {
		t.Fatalf("expected kind %q, got %q", SpaceKindRegular, kind)
	}
	if name != "infrastructural:rack" {
		t.Fatalf("expected \"as area\" marker stripped from name, got %q", name)
	}
	if !isArea {
		t.Fatalf("expected isArea = true for a \"space ... as area with:\" header")
	}
}

func TestParseListPatternsWildcardLeaf(t *testing.T) {
	patterns := parseListPatterns("sensor.*/cpu/*")
	if len(patterns) != 1 {
		t.Fatalf("got %d patterns, want 1: %+v", len(patterns), patterns)
	}
	p := patterns[0]
	if p.domain != "sensor" || p.sphere != "" || p.pathSuffix != "cpu" || !p.wildcardLeaf {
		t.Fatalf("parseListPatterns(sensor.*/cpu/*) = %+v, want {domain:sensor sphere:\"\" pathSuffix:cpu wildcardLeaf:true}", p)
	}
}

func TestMatchesAnyListPatternWildcardLeaf(t *testing.T) {
	patterns := parseListPatterns("sensor.*/cpu/*")

	cases := []struct {
		path string
		want bool
	}{
		{"house/laundry_kitchen/rack/protocols-server-2/cpu/load", true},
		{"house/laundry_kitchen/rack/protocols-server-2/cpu/temperature", true},
		{"cpu/load", true},                             // prefix at the very start of the path
		{"house/laundry_kitchen/rack/cpu", false},      // no leaf segment after "cpu"
		{"house/laundry_kitchen/rack/cpu_load", false}, // "cpu" must be its own segment, not a substring
	}
	for _, c := range cases {
		rec := TEntityRecord{Identity: TEntityIdentity{Domain: "sensor", Sphere: "infrastructural", Path: c.path}}
		if got := matchesAnyListPattern(rec, patterns); got != c.want {
			t.Errorf("matchesAnyListPattern(path=%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestParseListDeclarationsAsCardsWithDetailLevel(t *testing.T) {
	src := `list "Compute nodes" all sensor.*/cpu/load sensor.*/cpu/temperature as cards with:
  clean_prefix infrastructural;
  detail_level 3;
end;
`
	decls := parseListDeclarations(src)
	if len(decls) != 1 {
		t.Fatalf("got %d declarations, want 1: %+v", len(decls), decls)
	}
	d := decls[0]
	if d.title != "Compute nodes" {
		t.Errorf("title = %q, want %q", d.title, "Compute nodes")
	}
	if !d.asCards {
		t.Errorf("asCards = false, want true")
	}
	if d.detailLevel != 3 {
		t.Errorf("detailLevel = %d, want 3", d.detailLevel)
	}
	if len(d.patterns) != 2 {
		t.Fatalf("got %d patterns, want 2 (the \"as cards\" marker must not leak into pattern parsing): %+v", len(d.patterns), d.patterns)
	}
	if d.patterns[0].pathSuffix != "cpu/load" || d.patterns[1].pathSuffix != "cpu/temperature" {
		t.Errorf("patterns = %+v, want pathSuffix cpu/load and cpu/temperature", d.patterns)
	}
}

func TestParseListDeclarationsDefaultsDetailLevelWhenOmitted(t *testing.T) {
	src := `list "Compute nodes" all sensor.*/cpu/* as cards with:
  clean_prefix infrastructural;
end;
`
	decls := parseListDeclarations(src)
	if len(decls) != 1 {
		t.Fatalf("got %d declarations, want 1: %+v", len(decls), decls)
	}
	if decls[0].detailLevel != 2 {
		t.Errorf("detailLevel = %d, want default 2", decls[0].detailLevel)
	}
}

func TestBuildSensorGraphListFileYAML(t *testing.T) {
	entries := []TListEntry{
		{entityID: "sensor.infrastructural_house_laundry_kitchen_rack_junglinster_cpu_load", displayName: "house/laundry_kitchen/rack/junglinster/cpu/load"},
	}
	got := buildSensorGraphListFileYAML(entries, 2)
	want := "cards:\n" +
		"  - detail: 2\n" +
		"    entity: sensor.infrastructural_house_laundry_kitchen_rack_junglinster_cpu_load\n" +
		"    graph: line\n" +
		"    name: 'house/laundry_kitchen/rack/junglinster/cpu/load'\n" +
		"    type: sensor\n" +
		"type: vertical-stack\n"
	if got != want {
		t.Errorf("buildSensorGraphListFileYAML =\n%s\nwant:\n%s", got, want)
	}
}

func TestImpliedAggregatesForVirtualSpaceUseVirtualPolicy(t *testing.T) {
	spaceOrder := []string{"social/extension"}
	entityRecordsBySpace := map[string][]TEntityRecord{
		"social/extension": {
			{Identity: TEntityIdentity{Domain: "light", Path: "extension/main"}},
			{Identity: TEntityIdentity{Domain: "sensor", Path: "extension/temperature"}},
		},
	}

	virtualAggregates := impliedAggregatesForSpace("social/extension", SpaceKindVirtual, spaceOrder, entityRecordsBySpace)
	if len(virtualAggregates) != 1 || virtualAggregates[0] != "light.social/extension" {
		t.Fatalf("unexpected virtual-space aggregates: %v", virtualAggregates)
	}

	regularAggregates := impliedAggregatesForSpace("social/extension", SpaceKindRegular, spaceOrder, entityRecordsBySpace)
	if len(regularAggregates) != 2 {
		t.Fatalf("expected regular space to include sensor aggregate as well, got %v", regularAggregates)
	}
	if !strings.Contains(strings.Join(regularAggregates, "\n"), "sensor.social/extension/temperature") {
		t.Fatalf("expected temperature aggregate for regular space, got %v", regularAggregates)
	}
}

func TestValidateInvocationParametersAcceptsSnakeCaseForCamelCaseMacroParameter(t *testing.T) {
	ctx := &TMacroExpansionContext{Config: TExpanderConfig{CheckTypes: true}}
	macro := &TParsedCreationMacro{
		Name: "battery_alert",
		Parameters: []TMacroParameter{
			{Name: "${alert_level}", Kind: ParamInt, Optional: true},
		},
	}
	invocation := &TMacroInvocation{
		Name:       "battery_alert",
		Target:     "roborock",
		Parameters: map[string]string{"alert_level": "15"},
	}

	errors := ctx.ValidateInvocationParameters(invocation, macro)
	if len(errors) != 0 {
		t.Fatalf("expected no validation errors, got %v", errors)
	}
}

func TestExpandMacroSubstitutesSnakeCaseInvocationIntoCamelCaseMacroBody(t *testing.T) {
	ctx := &TMacroExpansionContext{
		Macros: map[string]*TParsedCreationMacro{
			"battery_alert": {
				Name: "battery_alert",
				Parameters: []TMacroParameter{
					{Name: "${alert_level}", Kind: ParamInt, Optional: true},
				},
				Body: []string{`condition sensor.infrastructural:${entity}:battery_level "($ | int(0)) < ${alert_level}";`},
			},
		},
		Config: TExpanderConfig{CheckTypes: true},
	}
	invocation := &TMacroInvocation{
		Name:       "battery_alert",
		Target:     "roborock",
		Parameters: map[string]string{"alert_level": "15"},
	}

	expanded, _, err := ctx.ExpandMacro(invocation, nil)
	if err != nil {
		t.Fatalf("expected macro expansion to succeed, got %v", err)
	}
	if len(expanded) != 1 {
		t.Fatalf("expected one expanded line, got %v", expanded)
	}
	if !strings.Contains(expanded[0], `< 15";`) {
		t.Fatalf("expected expanded body to substitute alert level, got %q", expanded[0])
	}
}

func TestParseParameterTreatsOptionAsImplicitlyOptional(t *testing.T) {
	param, err := parseParameter("${no_collect} option")
	if err != nil {
		t.Fatalf("expected option parameter to parse, got %v", err)
	}
	if param.Kind != ParamOption {
		t.Fatalf("expected option kind, got %v", param.Kind)
	}
	if !param.Optional {
		t.Fatalf("expected option parameter to be implicitly optional")
	}
}

func TestValidateInvocationParametersRejectsUnknownParameter(t *testing.T) {
	ctx := &TMacroExpansionContext{Config: TExpanderConfig{CheckTypes: true}}
	macro := &TParsedCreationMacro{
		Name: "battery_alert",
		Parameters: []TMacroParameter{
			{Name: "${alert_level}", Kind: ParamInt, Optional: true},
		},
	}
	invocation := &TMacroInvocation{
		Name:       "battery_alert",
		Target:     "social/living_room",
		Parameters: map[string]string{"alertlevel": "15", "typo_param": "bad"},
	}

	errors := ctx.ValidateInvocationParameters(invocation, macro)
	if len(errors) == 0 {
		t.Fatalf("expected validation errors for unknown parameters, got none")
	}
	found := false
	for _, e := range errors {
		if strings.Contains(e, "unknown parameter") && strings.Contains(e, "typo_param") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'unknown parameter typo_param' error, got %v", errors)
	}
}

func TestValidateInvocationParametersRejectsMissingRequiredParameter(t *testing.T) {
	ctx := &TMacroExpansionContext{Config: TExpanderConfig{CheckTypes: true}}
	macro := &TParsedCreationMacro{
		Name: "battery_alert",
		Parameters: []TMacroParameter{
			{Name: "${alert_level}", Kind: ParamInt, Optional: false},
		},
	}
	invocation := &TMacroInvocation{
		Name:       "battery_alert",
		Target:     "social/living_room",
		Parameters: map[string]string{},
	}

	errors := ctx.ValidateInvocationParameters(invocation, macro)
	if len(errors) == 0 {
		t.Fatalf("expected validation error for missing required parameter, got none")
	}
	found := false
	for _, e := range errors {
		if strings.Contains(e, "missing required parameter") && strings.Contains(e, "alert_level") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'missing required parameter alert_level' error, got %v", errors)
	}
}

func TestValidateParameterTypeEntityRejectsNonEntitySpec(t *testing.T) {
	if err := validateParameterType("roborock", ParamEntity); err == "" {
		t.Fatalf("expected validation error for bare name without domain, got none")
	}
	if err := validateParameterType("sensor.physical:living_room", ParamEntity); err != "" {
		t.Fatalf("expected no error for valid entity spec, got %q", err)
	}
	if err := validateParameterType("sensor.physical/apartment/living_room", ParamEntity); err != "" {
		t.Fatalf("expected no error for extensional entity spec, got %q", err)
	}
}

func TestParseParameterKindRecognisesSetOfInt(t *testing.T) {
	if kind := parseParameterKind("set of int"); kind != ParamSetOfInt {
		t.Fatalf("expected ParamSetOfInt for 'set of int', got %v", kind)
	}
}

func TestValidateParameterTypeSetOfIntRejectsNonIntegers(t *testing.T) {
	if err := validateParameterType("{1, 2, 3}", ParamSetOfInt); err != "" {
		t.Fatalf("expected no error for valid set of int, got %q", err)
	}
	if err := validateParameterType("{1, two, 3}", ParamSetOfInt); err == "" {
		t.Fatalf("expected validation error for non-integer set member, got none")
	}
	if err := validateParameterType("{}", ParamSetOfInt); err == "" {
		t.Fatalf("expected validation error for empty set, got none")
	}
}

func TestListPatternMatchesSegmentBoundary(t *testing.T) {
	patterns := parseListPatterns("all binary_sensor.*/node")

	match := func(domain, sphere, path string) bool {
		return matchesAnyListPattern(TEntityRecord{
			Identity: TEntityIdentity{Domain: domain, Sphere: sphere, Path: path},
		}, patterns)
	}

	// "binary_sensor.x/y/node" must match.
	if !match("binary_sensor", "x", "y/node") {
		t.Fatalf("expected match for path y/node")
	}
	// "binary_sensor.x/y_node" must NOT match (underscore, not slash).
	if match("binary_sensor", "x", "y_node") {
		t.Fatalf("expected no match for path y_node")
	}
	// "binary_sensor.x/y/node/sub" must NOT match (node is not the tail).
	if match("binary_sensor", "x", "y/node/sub") {
		t.Fatalf("expected no match for path y/node/sub")
	}
	// Different domain must not match.
	if match("sensor", "x", "y/node") {
		t.Fatalf("expected no match for domain sensor")
	}
	// "all" keyword in the pattern source must be silently ignored (no extra pattern).
	if len(patterns) != 1 {
		t.Fatalf("expected exactly 1 pattern, got %d", len(patterns))
	}
}

func TestListPatternMatchesAllForWildcardDomain(t *testing.T) {
	patterns := parseListPatterns("climate.*")

	if !matchesAnyListPattern(TEntityRecord{
		Identity: TEntityIdentity{Domain: "climate", Sphere: "physical", Path: "house/kitchen/radiator"},
	}, patterns) {
		t.Fatalf("expected climate.* to match any climate entity")
	}
	if matchesAnyListPattern(TEntityRecord{
		Identity: TEntityIdentity{Domain: "sensor", Sphere: "physical", Path: "house/kitchen/temperature"},
	}, patterns) {
		t.Fatalf("expected climate.* to not match a sensor entity")
	}
}

func TestListPatternSphereFilter(t *testing.T) {
	patterns := parseListPatterns("all binary_sensor.social/*/door binary_sensor.social/*/window")
	if len(patterns) != 2 {
		t.Fatalf("expected 2 patterns, got %d", len(patterns))
	}
	for _, p := range patterns {
		if p.sphere != "social" {
			t.Fatalf("expected sphere=social, got %q", p.sphere)
		}
	}

	match := func(domain, sphere, path string) bool {
		return matchesAnyListPattern(TEntityRecord{
			Identity: TEntityIdentity{Domain: domain, Sphere: sphere, Path: path},
		}, patterns)
	}

	// Social door must match.
	if !match("binary_sensor", "social", "apartment/hallway/door") {
		t.Fatal("expected social door entity to match")
	}
	// Physical door must NOT match a social sphere filter.
	if match("binary_sensor", "physical", "apartment/bedroom/door") {
		t.Fatal("expected physical door entity to not match social sphere filter")
	}
	// Social window must match.
	if !match("binary_sensor", "social", "house/bathroom/window") {
		t.Fatal("expected social window entity to match")
	}
	// Wrong domain must not match.
	if match("sensor", "social", "apartment/hallway/door") {
		t.Fatal("expected wrong domain to not match")
	}
}

func TestNormalizeEntityFullNameLightSocialInVirtualExtensionContext(t *testing.T) {
	spacePath := []string{"social:house", "social:extension"}
	got := normalizeEntityFullName("light.social", spacePath)
	want := "light.social/house/extension"
	if got != want {
		t.Fatalf("normalizeEntityFullName(%q, %v) = %q, want %q", "light.social", spacePath, got, want)
	}
}

func TestSpaceOnInVirtualSpacePopulatesSwitchOnByName(t *testing.T) {
	const miniDSL = `space social:house with:
  space social:corridor with:
    entity light.social:main;
    light on: @all;
    space off: @all;
  end;
  virtual space social:extension with:
    member social/house/corridor;
    space off: @all;
    space on: light.social;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration

	switchOn := admin.SpaceSwitchOnByName["social/house/extension"]
	if len(switchOn) == 0 {
		t.Fatalf("SpaceSwitchOnByName[social/house/extension] is empty, expected light.social/house/extension")
	}
	if switchOn[0] != "light.social/house/extension" {
		t.Fatalf("SpaceSwitchOnByName[social/house/extension] = %v, want [light.social/house/extension]", switchOn)
	}
}

func TestSourceToJinja2IsAvailableSugar(t *testing.T) {
	got := sourceToJinja2("sensor.processor_use is available")
	want := "'ON' if (states('sensor.processor_use') not in ['unavailable', 'unknown']) else 'OFF'"
	if got != want {
		t.Errorf("sourceToJinja2 = %q, want %q", got, want)
	}
}

func TestSourceToJinja2PlainAndAttributeStillWork(t *testing.T) {
	if got := sourceToJinja2("sensor.foo"); got != "states('sensor.foo')" {
		t.Errorf("plain source: got %q", got)
	}
	if got := sourceToJinja2("sensor.foo!bar"); got != "state_attr('sensor.foo', 'bar')" {
		t.Errorf("attribute source: got %q", got)
	}
}

func TestParseConditionDirectiveIsAvailableSugar(t *testing.T) {
	sources, expr := parseConditionDirective("condition sensor.processor_use is available;", nil)
	if len(sources) != 1 || sources[0] != "sensor.processor_use" {
		t.Fatalf("sources = %v, want [sensor.processor_use]", sources)
	}
	wantExpr := "$1 not in ['unavailable', 'unknown']"
	if expr != wantExpr {
		t.Errorf("expr = %q, want %q", expr, wantExpr)
	}
	// Confirm it produces the exact same generated expression as writing the boilerplate by hand.
	got := buildConditionStateExpr(sources, expr)
	want := "states('sensor.processor_use') not in ['unavailable', 'unknown']"
	if got != want {
		t.Errorf("buildConditionStateExpr = %q, want %q", got, want)
	}
}

func TestParseConditionDirectiveQuotedFormStillWorks(t *testing.T) {
	sources, expr := parseConditionDirective(`condition sensor.foo "$1 == 'on'";`, nil)
	if len(sources) != 1 || sources[0] != "sensor.foo" {
		t.Fatalf("sources = %v, want [sensor.foo]", sources)
	}
	if expr != "$1 == 'on'" {
		t.Errorf("expr = %q, want \"$1 == 'on'\"", expr)
	}
}

func TestNormalizeEntityFullNameEmptySphereBeforeColonDefaultsSphere(t *testing.T) {
	spacePath := []string{"social:garage"}
	got := normalizeEntityFullName("device.:air-4", spacePath)
	want := "device.infrastructural/garage/air-4"
	if got != want {
		t.Fatalf("normalizeEntityFullName(%q, %v) = %q, want %q (empty pre-colon sphere must default, not produce a malformed \"type./path\")", "device.:air-4", spacePath, got, want)
	}
}

func TestNormalizeEntityFullNameLeadingSlashIsSphereAbsolute(t *testing.T) {
	spacePath := []string{"social:garage"}
	got := normalizeEntityFullName("binary_sensor.infrastructural:/smarty/node", spacePath)
	want := "binary_sensor.infrastructural/smarty/node"
	if got != want {
		t.Fatalf("normalizeEntityFullName with leading '/' = %q, want %q (space context must NOT be prepended)", got, want)
	}
}

func TestDeviceEntityImpliesDiscoveryEntitiesWithoutSpacePrefix(t *testing.T) {
	const miniDSL = `space social:garage with:
  entity device.infrastructural:/smarty from host.smarty with: all entities;
end;`

	hostDevicesByID := map[string]THostDevice{
		"host.smarty": {DeviceID: "host.smarty", HostName: "smarty", IntegrationType: "cpu"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, hostDevicesByID, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration

	records := admin.EntityRecordsBySpace["social/garage"]
	byName := map[string]TEntityRecord{}
	for _, rec := range records {
		byName[rec.Name] = rec
	}

	for _, name := range []string{
		"binary_sensor.infrastructural/smarty/node",
		"sensor.infrastructural/smarty/cpu/load",
		"sensor.infrastructural/smarty/cpu/temperature",
	} {
		rec, ok := byName[name]
		if !ok {
			t.Fatalf("expected implied entity %q not found in social/garage; got %v", name, byName)
		}
		if !rec.DiscoveryImplied {
			t.Errorf("entity %q: DiscoveryImplied = false, want true", name)
		}
		if rec.HasDefinitionOrImport {
			t.Errorf("entity %q: HasDefinitionOrImport = true, want false", name)
		}
	}

	// Regression guard: must NOT have inherited the enclosing space's "garage" segment.
	if _, wrong := byName["binary_sensor.infrastructural/garage/smarty/node"]; wrong {
		t.Fatalf("implied node entity incorrectly inherited the enclosing space path (garage/ prefix)")
	}

	link, ok := admin.DeviceConceptualLinks["host.smarty"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[host.smarty] to be populated")
	}
	if link.NodeEntityID != "binary_sensor.infrastructural_smarty_node" {
		t.Errorf("link.NodeEntityID = %q, want %q", link.NodeEntityID, "binary_sensor.infrastructural_smarty_node")
	}
	if link.DisplayName != "infrastructural/garage/smarty" {
		t.Errorf("link.DisplayName = %q, want %q", link.DisplayName, "infrastructural/garage/smarty")
	}
	if got := link.ConstantAttributes["model"]; got != (TDeviceAttributeConstant{Value: "Compute host"}) {
		t.Errorf("link.ConstantAttributes[model] = %+v, want %+v", got, TDeviceAttributeConstant{Value: "Compute host"})
	}
	wantAttrs := map[string]TDeviceAttributeLink{
		"load":        {EntityID: "sensor.infrastructural_smarty_cpu_load", Unit: "%", StateClass: "measurement", Icon: "mdi:cpu-64-bit"},
		// No Icon: cpuAttributeSpecs["temperature"] sets none, and this test passes no
		// capabilityDefaults (nil) -- "mdi:thermometer" now comes only from Defaults.def's
		// "defaults: for sensor.*/temperature: ...;" rule, not a Go-level fallback.
		"temperature": {EntityID: "sensor.infrastructural_smarty_cpu_temperature", DeviceClass: "temperature", Unit: "°C", StateClass: "measurement"},
	}
	for attr, want := range wantAttrs {
		if got := link.AttributeEntityIDs[attr]; got != want {
			t.Errorf("link.AttributeEntityIDs[%q] = %+v, want %+v", attr, got, want)
		}
	}
}

func TestDeviceEntityBareSpecDefaultsToInfrastructuralSphereAndSpaceRelativePath(t *testing.T) {
	// No explicit sphere/absolute marker: "device" defaults to the infrastructural sphere
	// (taxonomy.go's SphereOf), but the path still follows the normal space-relative
	// convention -- same as any other entity declared without a leading "/".
	const miniDSL = `space social:garage with:
  entity device.smarty from host.smarty with: all entities;
end;`

	hostDevicesByID := map[string]THostDevice{
		"host.smarty": {DeviceID: "host.smarty", HostName: "smarty", IntegrationType: "cpu"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, hostDevicesByID, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration

	records := admin.EntityRecordsBySpace["social/garage"]
	byName := map[string]bool{}
	for _, rec := range records {
		byName[rec.Name] = true
	}

	if !byName["binary_sensor.infrastructural/garage/smarty/node"] {
		t.Fatalf("expected space-relative node entity binary_sensor.infrastructural/garage/smarty/node; got %v", records)
	}

	// Regression guard: a space-relative spec's DisplayName must not double the space's own
	// location segments -- normalizeEntityFullName already merges them into the entity's
	// structural path, so deviceDisplayName must derive its own location prefix from the
	// spec's bare leaf path (deviceSpecLeafPath), not from the already-merged path.
	link, ok := admin.DeviceConceptualLinks["host.smarty"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[host.smarty] to be populated")
	}
	if link.DisplayName != "infrastructural/garage/smarty" {
		t.Errorf("link.DisplayName = %q, want %q", link.DisplayName, "infrastructural/garage/smarty")
	}
}

func TestGenerateReportingAutomationsSplitsNumericAndDeviceInfoCapabilities(t *testing.T) {
	haOutputDir := t.TempDir()
	devices := []THostDevice{
		{
			DeviceID:        "host.junglinster",
			HostName:        "junglinster",
			IntegrationType: "home_assistant",
			Capabilities: map[string]string{
				"cpu/load":        "sensor.processor_use",
				"cpu/temperature": "sensor.processor_temperature",
				"sw_version":      "update.home_assistant_operating_system_update!installed_version",
			},
			CapabilityLiteralPrefixes: map[string]string{
				"sw_version": "Home Assistant Operating System ",
			},
		},
	}

	if err := generateReportingAutomations(haOutputDir, devices, "junglinster"); err != nil {
		t.Fatalf("generateReportingAutomations error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(haOutputDir, "automation", "infrastructural", "automation.reporting_device_host.junglinster.yaml"))
	if err != nil {
		t.Fatalf("expected automation file to be written: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "topic: hosts/junglinster/cpu/state") {
		t.Errorf("expected numeric action topic, got:\n%s", content)
	}
	if !strings.Contains(content, "'load': states('sensor.processor_use') | float(0)") {
		t.Errorf("expected load to be float-coerced, got:\n%s", content)
	}
	if !strings.Contains(content, "topic: hosts/junglinster/device/state") {
		t.Errorf("expected device-info action topic, got:\n%s", content)
	}
	if !strings.Contains(content, "'sw_version': 'Home Assistant Operating System ' ~ state_attr('update.home_assistant_operating_system_update', 'installed_version')") {
		t.Errorf("expected sw_version to concatenate its literal prefix with the \"!attribute\" state_attr() lookup, got:\n%s", content)
	}
	if strings.Contains(content, "installed_version') | float(0)") {
		t.Errorf("sw_version must NOT be float(0)-coerced, got:\n%s", content)
	}
	if !strings.Contains(content, "'via_device': 'junglinster'") {
		t.Errorf("expected via_device literal, got:\n%s", content)
	}
	if !strings.Contains(content, "'name': 'junglinster'") {
		t.Errorf("expected name literal, got:\n%s", content)
	}
}

func TestGenerateCustomizationFilesSkipsDiscoveryImpliedEntities(t *testing.T) {
	const miniDSL = `space social:garage with:
  entity device.infrastructural:/smarty from host.smarty with: all entities;
  entity switch.social:test_switch;
end;`

	hostDevicesByID := map[string]THostDevice{
		"host.smarty": {DeviceID: "host.smarty", HostName: "smarty", IntegrationType: "cpu"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, hostDevicesByID, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	outputDir := t.TempDir()
	if err := generateCustomizationFiles(outputDir, result.Administration); err != nil {
		t.Fatalf("generateCustomizationFiles error: %v", err)
	}

	nodePath := filepath.Join(outputDir, "customization", "binary_sensor", "infrastructural", "binary_sensor.infrastructural_smarty_node.yaml")
	if _, err := os.Stat(nodePath); err == nil {
		t.Errorf("expected no customization file for the discovery-implied node entity, but found %s", nodePath)
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected error checking %s: %v", nodePath, err)
	}

	switchPath := filepath.Join(outputDir, "customization", "switch", "social", "switch.social_garage_test_switch.yaml")
	if _, err := os.Stat(switchPath); err != nil {
		t.Errorf("expected a customization file for the ordinary (non-discovery-implied) switch entity, got error: %v", err)
	}
}

func TestRegisterDeviceImpliedEntitiesNoWarningForPingType(t *testing.T) {
	// "ping" type has no AttributeDomain at all -- liveness-only by design, so registering the
	// node entity only is the expected, permanent outcome, not a warning-worthy edge case.
	admin := newAdministrationState()
	decl := TDeviceEntityDeclaration{DeviceSpec: "infrastructural:appletv-office", DeviceID: "host.appletv-office", Flags: []string{"all entities"}}
	hostDevicesByID := map[string]THostDevice{
		"host.appletv-office": {DeviceID: "host.appletv-office", HostName: "appletv-office", IntegrationType: "ping"},
	}

	warnings := registerDeviceImpliedEntities(admin, decl, hostDevicesByID, nil, "Spaces.def", 898)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for a ping-type device, got %v", warnings)
	}
}

func TestRegisterDeviceImpliedEntitiesStillWarnsForHomeAssistantTypeWithNoCapabilities(t *testing.T) {
	// "home_assistant" type DOES have the structural capacity for variable attributes
	// (AttributeDomain set) -- an empty set here usually means the device forgot to declare
	// capabilities, so this case should still warn.
	admin := newAdministrationState()
	decl := TDeviceEntityDeclaration{DeviceSpec: "infrastructural:junglinster", DeviceID: "host.junglinster", Flags: []string{"all entities"}}
	hostDevicesByID := map[string]THostDevice{
		"host.junglinster": {DeviceID: "host.junglinster", HostName: "junglinster", IntegrationType: "home_assistant"},
	}

	warnings := registerDeviceImpliedEntities(admin, decl, hostDevicesByID, nil, "Spaces.def", 42)
	if len(warnings) == 0 {
		t.Errorf("expected a warning for a home_assistant-type device with no declared capabilities")
	}
}

func TestSpaceAsAreaAssignsSuggestedAreaToDevices(t *testing.T) {
	const miniDSL = `space social:house with:
  space social:laundry_kitchen with:
    space infrastructural:rack as area with:
      entity device.infrastructural:junglinster from host.junglinster with: all entities;
    end;
  end;
end;`

	hostDevicesByID := map[string]THostDevice{
		"host.junglinster": {DeviceID: "host.junglinster", HostName: "junglinster", IntegrationType: "cpu"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, hostDevicesByID, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	link, ok := result.Administration.DeviceConceptualLinks["host.junglinster"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[host.junglinster] to be populated")
	}
	want := TDeviceAttributeConstant{Value: "infrastructural/house/laundry_kitchen/rack"}
	if got := link.ConstantAttributes["suggested_area"]; got != want {
		t.Errorf("link.ConstantAttributes[suggested_area] = %+v, want %+v", got, want)
	}
}

func TestSpaceAsAreaShadowingAndExplicitOverride(t *testing.T) {
	// smarty sits in the outer "as area" space (garage) with no area of its own -- inherits it.
	// junglinster sits in a nested "as area" space (rack) -- gets the more specific one, not
	// the outer garage area. protocols-server-1 has its own explicit suggested_area override,
	// which must survive even though it's positioned inside the "rack" area space.
	const miniDSL = `space infrastructural:garage as area with:
  entity device.infrastructural:smarty from host.smarty with: all entities;
  space infrastructural:rack as area with:
    entity device.infrastructural:junglinster from host.junglinster with: all entities;
    entity device.infrastructural:protocols-server-1 from host.protocols-server-1 with: all entities;
  end;
end;`

	hostDevicesByID := map[string]THostDevice{
		"host.smarty":      {DeviceID: "host.smarty", HostName: "smarty", IntegrationType: "cpu"},
		"host.junglinster": {DeviceID: "host.junglinster", HostName: "junglinster", IntegrationType: "cpu"},
		"host.protocols-server-1": {
			DeviceID: "host.protocols-server-1", HostName: "protocols-server-1", IntegrationType: "cpu",
			ConstantAttributes: map[string]THostConstantAttribute{
				"suggested_area": {Value: "infrastructural/basement"},
			},
		},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, hostDevicesByID, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration

	cases := map[string]string{
		"host.smarty":             "infrastructural/garage",
		"host.junglinster":        "infrastructural/garage/rack",
		"host.protocols-server-1": "infrastructural/basement",
	}
	for deviceID, want := range cases {
		link, ok := admin.DeviceConceptualLinks[deviceID]
		if !ok {
			t.Fatalf("expected DeviceConceptualLinks[%s] to be populated", deviceID)
		}
		got := link.ConstantAttributes["suggested_area"]
		if got.Value != want {
			t.Errorf("%s: suggested_area = %q, want %q", deviceID, got.Value, want)
		}
	}
}

func TestParseHostsIntegrationBodyConstantDeviceAttributes(t *testing.T) {
	body := []string{
		"device host.junglinster junglinster home_assistant with:",
		"  load:        sensor.processor_use;",
		"  temperature: sensor.processor_temperature;",
		`  model: "Cool raspi"; # Default value`,
		`  hw_version: "1928930" forced; # The "forced" means we do not allow for updates`,
		"end;",
	}

	devices, warnings := parseHostsIntegrationBody(body)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1: %+v", len(devices), devices)
	}
	d := devices[0]

	if got, want := d.Capabilities["load"], "sensor.processor_use"; got != want {
		t.Errorf("Capabilities[load] = %q, want %q", got, want)
	}
	if got, want := d.ConstantAttributes["model"], (THostConstantAttribute{Value: "Cool raspi"}); got != want {
		t.Errorf("ConstantAttributes[model] = %+v, want %+v", got, want)
	}
	if got, want := d.ConstantAttributes["hw_version"], (THostConstantAttribute{Value: "1928930", Forced: true}); got != want {
		t.Errorf("ConstantAttributes[hw_version] = %+v, want %+v", got, want)
	}

	mat, known := MaterializationForIntegrationType(d.IntegrationType)
	if !known {
		t.Fatalf("no materialization for integration type %q", d.IntegrationType)
	}
	merged, mergeWarnings := mergedConstantAttributes(mat, d)
	if len(mergeWarnings) != 0 {
		t.Fatalf("unexpected merge warnings: %v", mergeWarnings)
	}
	// The per-device "model" override must win over the "home_assistant" type default
	// ("Home Assistant instance"), and "hw_version" (no type default at all) must come
	// through with Forced still set.
	if got, want := merged["model"], (THostConstantAttribute{Value: "Cool raspi"}); got != want {
		t.Errorf("merged[model] = %+v, want %+v (device override should win over the type default)", got, want)
	}
	if got, want := merged["hw_version"], (THostConstantAttribute{Value: "1928930", Forced: true}); got != want {
		t.Errorf("merged[hw_version] = %+v, want %+v", got, want)
	}
}

func TestParseHostsIntegrationBodyCapabilityWithLiteralPrefixAndAttribute(t *testing.T) {
	body := []string{
		"device host.junglinster junglinster home_assistant with:",
		`  sw_version: "Home Assistant Operating System " update.home_assistant_operating_system_update!installed_version;`,
		"  load: sensor.processor_use;",
		"end;",
	}

	devices, warnings := parseHostsIntegrationBody(body)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1: %+v", len(devices), devices)
	}
	d := devices[0]

	if got, want := d.Capabilities["sw_version"], "update.home_assistant_operating_system_update!installed_version"; got != want {
		t.Errorf("Capabilities[sw_version] = %q, want %q", got, want)
	}
	if got, want := d.CapabilityLiteralPrefixes["sw_version"], "Home Assistant Operating System "; got != want {
		t.Errorf("CapabilityLiteralPrefixes[sw_version] = %q, want %q", got, want)
	}
	// A capability with no literal prefix must not get a spurious entry.
	if _, hasPrefix := d.CapabilityLiteralPrefixes["load"]; hasPrefix {
		t.Errorf("CapabilityLiteralPrefixes[load] should be absent, got %q", d.CapabilityLiteralPrefixes["load"])
	}
	if got, want := d.Capabilities["load"], "sensor.processor_use"; got != want {
		t.Errorf("Capabilities[load] = %q, want %q", got, want)
	}
}

func TestParseHostsIntegrationBodyGroupedCapabilityName(t *testing.T) {
	body := []string{
		"device host.junglinster junglinster home_assistant with:",
		"  cpu/load:        sensor.processor_use;",
		"  cpu/temperature: sensor.processor_temperature;",
		`  sw_version: "Home Assistant Operating System " update.home_assistant_operating_system_update!installed_version;`,
		`  model:       "Home Assistant Green";`,
		"end;",
	}

	devices, warnings := parseHostsIntegrationBody(body)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1: %+v", len(devices), devices)
	}
	d := devices[0]

	if got, want := d.Capabilities["cpu/load"], "sensor.processor_use"; got != want {
		t.Errorf("Capabilities[cpu/load] = %q, want %q", got, want)
	}
	if got, want := d.Capabilities["cpu/temperature"], "sensor.processor_temperature"; got != want {
		t.Errorf("Capabilities[cpu/temperature] = %q, want %q", got, want)
	}
	if got, want := d.ConstantAttributes["model"], (THostConstantAttribute{Value: "Home Assistant Green"}); got != want {
		t.Errorf("ConstantAttributes[model] = %+v, want %+v", got, want)
	}

	mat, known := MaterializationForIntegrationType(d.IntegrationType)
	if !known {
		t.Fatalf("no materialization for integration type %q", d.IntegrationType)
	}
	attrNames := mat.AttributeNames(d)
	want := []string{"load", "temperature"}
	if len(attrNames) != len(want) || attrNames[0] != want[0] || attrNames[1] != want[1] {
		t.Fatalf("AttributeNames = %v, want %v", attrNames, want)
	}
	if group := mat.AttributeGroup(d, "load"); group != "cpu" {
		t.Errorf("AttributeGroup(load) = %q, want %q", group, "cpu")
	}
	if group := mat.AttributeGroup(d, "temperature"); group != "cpu" {
		t.Errorf("AttributeGroup(temperature) = %q, want %q", group, "cpu")
	}
}

func TestParseDiscoveryIntegrationBody(t *testing.T) {
	body := []string{
		"device discovery.ems_esp with:",
		`  identifiers "ems-esp-extra";`,
		"  device discovery.ems_esp_boiler with:",
		"    sensor.outdoor_temperature: sensor.boiler_outdoortemp;",
		"  end;",
		"end;",
	}

	devices, warnings := parseDiscoveryIntegrationBody(body)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2 (root + nested boiler): %+v", len(devices), devices)
	}

	byID := map[string]TDiscoveryGatewayDevice{}
	for _, d := range devices {
		byID[d.DeviceID] = d
	}

	root, ok := byID["discovery.ems_esp"]
	if !ok {
		t.Fatalf("expected discovery.ems_esp, got %v", byID)
	}
	if root.ParentDeviceID != "" {
		t.Errorf("root ParentDeviceID = %q, want \"\"", root.ParentDeviceID)
	}
	wantIdentifiers := []string{"ems-esp-extra", "ems-esp"} // explicit line, then the inferred default
	if len(root.Identifiers) != len(wantIdentifiers) || root.Identifiers[0] != wantIdentifiers[0] || root.Identifiers[1] != wantIdentifiers[1] {
		t.Errorf("root Identifiers = %v, want %v", root.Identifiers, wantIdentifiers)
	}

	boiler, ok := byID["discovery.ems_esp_boiler"]
	if !ok {
		t.Fatalf("expected discovery.ems_esp_boiler, got %v", byID)
	}
	if boiler.ParentDeviceID != "discovery.ems_esp" {
		t.Errorf("boiler ParentDeviceID = %q, want %q", boiler.ParentDeviceID, "discovery.ems_esp")
	}
	if len(boiler.Identifiers) != 1 || boiler.Identifiers[0] != "ems-esp-boiler" {
		t.Errorf("boiler Identifiers = %v, want the inferred default [\"ems-esp-boiler\"]", boiler.Identifiers)
	}
	cap, ok := boiler.Capabilities["outdoor_temperature"]
	if !ok || cap.Domain != "sensor" || cap.Leaf != "boiler_outdoortemp" {
		t.Errorf("boiler Capabilities[outdoor_temperature] = %+v, ok=%v, want Domain sensor, Leaf boiler_outdoortemp (domain prefix stripped)", cap, ok)
	}
}

func TestExtractDiscoveryEntityDeclarationLastDotIsLeafBoundary(t *testing.T) {
	decl, ok := extractDiscoveryEntityDeclaration("entity sensor.social:garage_door/temperature from discovery.ems_esp.boiler_outdoortemp;")
	if !ok {
		t.Fatalf("expected line to parse")
	}
	if decl.EntitySpec != "sensor.social:garage_door/temperature" {
		t.Errorf("EntitySpec = %q, want %q", decl.EntitySpec, "sensor.social:garage_door/temperature")
	}
	if decl.GatewayDeviceID != "discovery.ems_esp" {
		t.Errorf("GatewayDeviceID = %q, want %q (must split on the LAST dot, not the first)", decl.GatewayDeviceID, "discovery.ems_esp")
	}
	if decl.Leaf != "boiler_outdoortemp" {
		t.Errorf("Leaf = %q, want %q", decl.Leaf, "boiler_outdoortemp")
	}

	// Must not be confused with the unrelated "entity device.<spec> from <device-id> with:
	// ...;" construct (Conceptual_DeviceEntities.go), which always has a trailing "with:" clause.
	if _, ok := extractDiscoveryEntityDeclaration("entity device.infrastructural:junglinster from host.junglinster with: all entities;"); ok {
		t.Errorf("must not match a device.<spec> from <device-id> with: ...; line")
	}
}

func TestRegisterDiscoveryEntityLinkViaParse(t *testing.T) {
	// Declared at the top level (no enclosing space) so the space-relative spec's own path
	// ("garage_door/temperature") isn't merged with any outer space context -- avoids the same
	// double-counting trap normalizeEntityFullName has for any relative spec (see
	// deviceSpecLeafPath's doc comment, Conceptual_DeviceEntities.go).
	const miniDSL = `entity sensor.social:garage_door/temperature from discovery.ems_esp_boiler.outdoor_temperature;`

	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"discovery.ems_esp_boiler": {
			DeviceID: "discovery.ems_esp_boiler", ParentDeviceID: "discovery.ems_esp", Identifiers: []string{"ems-esp-boiler"},
			Capabilities: map[string]TDiscoveryCapability{"outdoor_temperature": {Domain: "sensor", Leaf: "boiler_outdoortemp"}},
		},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, discoveryGatewaysByID, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration

	link, ok := admin.DiscoveryEntityLinks["sensor.social_garage_door_temperature"]
	if !ok {
		t.Fatalf("expected DiscoveryEntityLinks[sensor.social_garage_door_temperature] to be populated, got %+v", admin.DiscoveryEntityLinks)
	}
	if link.GatewayDeviceID != "discovery.ems_esp_boiler" || link.Leaf != "boiler_outdoortemp" {
		t.Errorf("link = %+v, want gateway discovery.ems_esp_boiler, leaf boiler_outdoortemp (resolved from the local name \"outdoor_temperature\")", link)
	}

	records := admin.EntityRecordsBySpace["root"]
	found := false
	for _, rec := range records {
		if rec.Name == "sensor.social/garage_door/temperature" {
			found = true
			if !rec.DiscoveryImplied {
				t.Errorf("expected DiscoveryImplied = true, so this entity is excluded from customization/aggregates like device.<spec> entities already are")
			}
		}
	}
	if !found {
		t.Fatalf("expected sensor.social/garage_door/temperature to be registered, got %v", records)
	}
}

func TestRegisterDiscoveryEntityLinkWarnsOnUnknownGateway(t *testing.T) {
	const miniDSL = `space social:garage_door with:
  entity sensor.social:garage_door/temperature from discovery.unknown_gateway.leaf;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, map[string]TDiscoveryGatewayDevice{}, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if _, ok := result.Administration.DiscoveryEntityLinks["sensor.social_garage_door_temperature"]; ok {
		t.Errorf("expected no link registered for an unknown gateway device id")
	}
}

func TestAttributeNamesExcludesDeviceInfoCapabilities(t *testing.T) {
	mat, known := MaterializationForIntegrationType("home_assistant")
	if !known {
		t.Fatalf("no materialization for integration type %q", "home_assistant")
	}
	device := THostDevice{
		DeviceID: "host.junglinster",
		Capabilities: map[string]string{
			"cpu/load":        "sensor.processor_use",
			"cpu/temperature": "sensor.processor_temperature",
			"sw_version":      "update.home_assistant_operating_system_update!installed_version",
		},
	}

	got := mat.AttributeNames(device)
	want := []string{"load", "temperature"}
	if len(got) != len(want) {
		t.Fatalf("AttributeNames = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("AttributeNames = %v, want %v", got, want)
		}
	}
	if group := mat.AttributeGroup(device, "load"); group != "cpu" {
		t.Errorf("AttributeGroup(load) = %q, want %q", group, "cpu")
	}
	// Regression guard: sw_version has no group prefix, so it must never become a "variable
	// device attribute" -- it's a device-info capability, destined for the discovery "device:"
	// block via generateReportingAutomations' device/state topic, not a standalone
	// "sensor..../cpu/sw_version" entity.
	for _, name := range got {
		if name == "sw_version" {
			t.Fatalf("AttributeNames must not include sw_version: %v", got)
		}
	}
}

func TestHomeAssistantCapabilityEntityIDsStripsAttributeSuffix(t *testing.T) {
	definitionDir := t.TempDir()
	physicalContent := `physical layer with:
  integration hosts with:
    device host.junglinster junglinster home_assistant with:
      load: sensor.processor_use;
      sw_version: "Home Assistant Operating System " update.home_assistant_operating_system_update!installed_version;
    end;
  end;
end;
`
	if err := os.WriteFile(filepath.Join(definitionDir, "Physical.def"), []byte(physicalContent), 0o644); err != nil {
		t.Fatalf("failed to write Physical.def: %v", err)
	}

	ids := homeAssistantCapabilityEntityIDs(definitionDir)

	if label, ok := ids["sensor.processor_use"]; !ok || label != "host.junglinster/load" {
		t.Errorf("ids[sensor.processor_use] = (%q, %v), want (\"host.junglinster/load\", true)", label, ok)
	}
	// The "!installed_version" attribute suffix must be stripped -- checking presence of the
	// literal "id!attribute" string would never match a real entity id from /api/states.
	if label, ok := ids["update.home_assistant_operating_system_update"]; !ok || label != "host.junglinster/sw_version" {
		t.Errorf("ids[update.home_assistant_operating_system_update] = (%q, %v), want (\"host.junglinster/sw_version\", true)", label, ok)
	}
	for id := range ids {
		if strings.Contains(id, "!") {
			t.Errorf("entity id %q must not contain an unstripped \"!attribute\" suffix", id)
		}
	}
}

func TestMergedConstantAttributesWarnsOnUnknownName(t *testing.T) {
	device := THostDevice{
		DeviceID:        "host.smarty",
		IntegrationType: "cpu",
		ConstantAttributes: map[string]THostConstantAttribute{
			"typo_field": {Value: "oops"},
		},
	}
	mat, _ := MaterializationForIntegrationType("cpu")
	merged, warnings := mergedConstantAttributes(mat, device)
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
	if _, present := merged["typo_field"]; present {
		t.Errorf("unrecognised attribute %q should not be merged in", "typo_field")
	}
}
