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

func TestInterpretationOperationalForKnownHouses(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	prev := DebugEnabled
	DebugEnabled = true
	defer func() { DebugEnabled = prev }()

	if err := runInterpretation(root, HouseNames); err != nil {
		t.Fatalf("interpretation failed: %v", err)
	}

	for _, houseName := range HouseNames {
		interpretationPath := debugReportPath(root, houseName, DebugReportInterpretation)
		content, readErr := os.ReadFile(interpretationPath)
		if readErr != nil {
			t.Fatalf("failed to read %s: %v", interpretationPath, readErr)
		}
		text := string(content)
		if strings.TrimSpace(text) == "" {
			t.Fatalf("interpretation file is empty: %s", interpretationPath)
		}
		if !strings.Contains(text, "Interpretation:") {
			t.Fatalf("interpretation marker missing in %s", interpretationPath)
		}
		if !strings.Contains(text, "BLOCK ") {
			t.Fatalf("no parsed block entries found in %s", interpretationPath)
		}
		if strings.Contains(text, "Status: errors") {
			t.Fatalf("parser reported errors in %s", interpretationPath)
		}
	}
}

func TestExpansionOperationalForKnownHouses(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	prev := DebugEnabled
	DebugEnabled = true
	defer func() { DebugEnabled = prev }()

	if err := runExpansion(root, HouseNames); err != nil {
		t.Fatalf("expansion failed: %v", err)
	}

	for _, houseName := range HouseNames {
		expansionPath := debugReportPath(root, houseName, DebugReportExpansion)
		content, readErr := os.ReadFile(expansionPath)
		if readErr != nil {
			t.Fatalf("failed to read %s: %v", expansionPath, readErr)
		}
		text := string(content)
		if strings.TrimSpace(text) == "" {
			t.Fatalf("expansion file is empty: %s", expansionPath)
		}
		if !strings.Contains(text, "=== MACRO EXPANSION REPORT ===") {
			t.Fatalf("expansion marker missing in %s", expansionPath)
		}
		if strings.Contains(text, "Status: ERROR") {
			t.Fatalf("unexpected error marker in %s", expansionPath)
		}
	}
}

func TestExpansionListsVirtualSpaces(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	prev := DebugEnabled
	DebugEnabled = true
	defer func() { DebugEnabled = prev }()

	if err := runExpansion(root, []string{"Junglinster"}); err != nil {
		t.Fatalf("expansion failed: %v", err)
	}

	expansionPath := debugReportPath(root, "Junglinster", DebugReportExpansion)
	content, readErr := os.ReadFile(expansionPath)
	if readErr != nil {
		t.Fatalf("failed to read %s: %v", expansionPath, readErr)
	}
	text := string(content)
	if !strings.Contains(text, `Virtual space "social/house/extension":`) {
		t.Fatalf("expected virtual extension space in %s", expansionPath)
	}
	if !strings.Contains(text, `Virtual space "social/house/laundry_corridor":`) {
		t.Fatalf("expected virtual laundry_corridor space in %s", expansionPath)
	}
}

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

func TestResolveHomeAssistantTargetPrefersDefinitionsOverEnvironment(t *testing.T) {
	root := t.TempDir()
	definitionDir := filepath.Join(root, "Definitions")
	if err := os.MkdirAll(definitionDir, 0o755); err != nil {
		t.Fatalf("failed to create definition dir: %v", err)
	}

	serverContent := "${main_instance} = \"https://from-definitions.example\";\nmain vienna ${main_instance};\n"
	secretsContent := "secrets:\n  ${main_api_token} = \"token-from-definitions\";\n  ${main_api_tls_insecure} = true;\nend;\n"
	if err := os.WriteFile(filepath.Join(definitionDir, "Server.def"), []byte(serverContent), 0o644); err != nil {
		t.Fatalf("failed to write Server.def: %v", err)
	}
	if err := os.WriteFile(filepath.Join(definitionDir, "Secrets.def"), []byte(secretsContent), 0o644); err != nil {
		t.Fatalf("failed to write Secrets.def: %v", err)
	}

	t.Setenv("HASS_BASE_URL", "https://from-env.example")
	t.Setenv("HASS_TOKEN", "token-from-env")
	t.Setenv("HASS_INSECURE_SKIP_TLS_VERIFY", "false")

	target, err := resolveHomeAssistantTarget(definitionDir)
	if err != nil {
		t.Fatalf("unexpected target resolution error: %v", err)
	}

	if target.BaseURL != "https://from-definitions.example" {
		t.Fatalf("expected definition URL to win, got %q", target.BaseURL)
	}
	if target.Token != "token-from-definitions" {
		t.Fatalf("expected definition token to win, got %q", target.Token)
	}
	if !target.InsecureSkipTLS {
		t.Fatalf("expected definition insecure TLS flag to win")
	}
}

func TestParseServerAssignmentsWithDefinedSelectsMatchingBranch(t *testing.T) {
	serverContent := strings.Join([]string{
		`if is up "vienna.fritz.box" then`,
		`  ${main_instance} = "http://vienna.fritz.box:8123";`,
		`elif is up "junglinster.fritz.box" then`,
		`  ${main_instance} = "https://junglinster.homelinux.org";`,
		`else`,
		`  ${main_instance} = "https://fallback.example";`,
		`end;`,
		`main vienna ${main_instance};`,
	}, "\n")

	assignments := parseServerAssignmentsWithDefined(serverContent, func(host string) bool {
		return host == "junglinster.fritz.box"
	})
	if assignments["main_instance"] != "https://junglinster.homelinux.org" {
		t.Fatalf("expected elif branch assignment, got %q", assignments["main_instance"])
	}
}

func TestParseServerAssignmentsWithDefinedFallsBackToElse(t *testing.T) {
	serverContent := strings.Join([]string{
		`if is up "vienna.fritz.box" then`,
		`  ${main_instance} = "http://vienna.fritz.box:8123";`,
		`elif is up "junglinster.fritz.box" then`,
		`  ${main_instance} = "https://junglinster.homelinux.org";`,
		`else`,
		`  ${main_instance} = "https://fallback.example";`,
		`end;`,
		`main vienna ${main_instance};`,
	}, "\n")

	assignments := parseServerAssignmentsWithDefined(serverContent, func(string) bool { return false })
	if assignments["main_instance"] != "https://fallback.example" {
		t.Fatalf("expected else branch assignment, got %q", assignments["main_instance"])
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

func TestSettingsSharedFileContainsIconVariables(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	// Icon variables live in the shared Settings.def; local files only hold per-house overrides.
	sharedSettingsPath := filepath.Join(root, "New", "Shared", "Definitions", "Settings.def")
	content, readErr := os.ReadFile(sharedSettingsPath)
	if readErr != nil {
		t.Fatalf("failed to read %s: %v", sharedSettingsPath, readErr)
	}
	text := string(content)
	for _, expectedLine := range []string{
		"${consumes_icon} = \"mdi:flash\";",
		"${media_switch_icon} = \"mdi:monitor-speaker\";",
		"${water_icon} = \"mdi:water-off\";",
	} {
		if !strings.Contains(text, expectedLine) {
			t.Fatalf("expected %q in %s", expectedLine, sharedSettingsPath)
		}
	}

	// Local settings files must only contain house-specific overrides (${workdays}).
	workdaysByHouse := map[string]string{
		"Vienna":      `"AT"`,
		"Junglinster": `"LU"`,
	}
	for _, houseName := range HouseNames {
		localSettingsPath := filepath.Join(root, "New", houseName, "Definitions", "Settings.def")
		localContent, localReadErr := os.ReadFile(localSettingsPath)
		if localReadErr != nil {
			t.Fatalf("failed to read %s: %v", localSettingsPath, localReadErr)
		}
		localText := string(localContent)
		if !strings.Contains(localText, workdaysByHouse[houseName]) {
			t.Fatalf("expected ${workdays} %s in %s", workdaysByHouse[houseName], localSettingsPath)
		}
	}
}






func TestParseSpaceHeaderRecognizesVirtualSpace(t *testing.T) {
	kind, name, ok := parseSpaceHeader("virtual space social:extension with:")
	if !ok {
		t.Fatalf("expected virtual space header to parse")
	}
	if kind != SpaceKindVirtual {
		t.Fatalf("expected kind %q, got %q", SpaceKindVirtual, kind)
	}
	if name != "social:extension" {
		t.Fatalf("expected virtual space name, got %q", name)
	}

	kind, name, ok = parseSpaceHeader("space social:house with:")
	if !ok {
		t.Fatalf("expected regular space header to parse")
	}
	if kind != SpaceKindRegular {
		t.Fatalf("expected kind %q, got %q", SpaceKindRegular, kind)
	}
	if name != "social:house" {
		t.Fatalf("expected space name, got %q", name)
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

func TestInterpretationRespectsMainIncludeOrder(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	prev := DebugEnabled
	DebugEnabled = true
	defer func() { DebugEnabled = prev }()

	if err := runInterpretation(root, []string{"Vienna"}); err != nil {
		t.Fatalf("interpretation failed: %v", err)
	}

	interpretationPath := debugReportPath(root, "Vienna", DebugReportInterpretation)
	content, readErr := os.ReadFile(interpretationPath)
	if readErr != nil {
		t.Fatalf("failed to read %s: %v", interpretationPath, readErr)
	}
	text := string(content)

	mainIndex := strings.Index(text, "File: Main.def")
	macrosIndex := strings.Index(text, "File: Macros.def")
	entitiesIndex := strings.Index(text, "File: Entities.def")
	if mainIndex < 0 || macrosIndex < 0 || entitiesIndex < 0 {
		t.Fatalf("expected Main.def, Macros.def and Entities.def sections in %s", interpretationPath)
	}
	if !(mainIndex < macrosIndex && macrosIndex < entitiesIndex) {
		t.Fatalf("expected Main.def -> Macros.def -> Entities.def ordering in %s", interpretationPath)
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
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), "test.def", &TMacroExpansionContext{}, &report)
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
