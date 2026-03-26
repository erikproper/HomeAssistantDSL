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

	if err := runInterpretation(root, THouseNames); err != nil {
		t.Fatalf("interpretation failed: %v", err)
	}

	for _, houseName := range THouseNames {
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

	if err := runExpansion(root, THouseNames); err != nil {
		t.Fatalf("expansion failed: %v", err)
	}

	for _, houseName := range THouseNames {
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

func TestParseServerAssignmentsWithAvailabilitySelectsMatchingBranch(t *testing.T) {
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

	assignments := parseServerAssignmentsWithAvailability(serverContent, func(host string) bool {
		return host == "junglinster.fritz.box"
	})
	if assignments["main_instance"] != "https://junglinster.homelinux.org" {
		t.Fatalf("expected elif branch assignment, got %q", assignments["main_instance"])
	}
}

func TestParseServerAssignmentsWithAvailabilityFallsBackToElse(t *testing.T) {
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

	assignments := parseServerAssignmentsWithAvailability(serverContent, func(string) bool { return false })
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
	for _, houseName := range THouseNames {
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











func TestInterpretationRespectsMainIncludeOrder(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

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
