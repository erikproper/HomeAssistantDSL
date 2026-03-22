/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Main/Test
 *
 * This component provides a smoke test that keeps migration operational while legacy source files evolve.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 21.03.2026
 *
 */

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationOperationalForKnownHouses(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	if err := runMigration(root, THouseNames); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	definitionNames := []string{"Main.def", "Settings.def", "Secrets.def", "Server.def", "Bridges.def", "Entities.def", "Lists.def", "Macros.def"}
	for _, houseName := range THouseNames {
		for _, definitionName := range definitionNames {
			definitionPath := filepath.Join(root, "New", houseName, "Definitions", definitionName)
			content, readErr := os.ReadFile(definitionPath)
			if readErr != nil {
				t.Fatalf("failed to read %s: %v", definitionPath, readErr)
			}
			if strings.TrimSpace(string(content)) == "" {
				t.Fatalf("generated file is empty: %s", definitionPath)
			}
		}
	}
}

func TestInterpretationOperationalForKnownHouses(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	if err := runMigration(root, THouseNames); err != nil {
		t.Fatalf("migration failed before interpretation: %v", err)
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

	if err := runMigration(root, THouseNames); err != nil {
		t.Fatalf("migration failed before expansion: %v", err)
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

	if err := runMigration(root, []string{"Junglinster"}); err != nil {
		t.Fatalf("migration failed: %v", err)
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

	serverContent := "$MainInstance = \"https://from-definitions.example\";\nmain vienna $MainInstance;\n"
	secretsContent := "secrets:\n  $MainAPIToken = \"token-from-definitions\";\n  $MainAPITLSInsecure = true;\nend;\n"
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
		`  $MainInstance = "http://vienna.fritz.box:8123";`,
		`elif is up "junglinster.fritz.box" then`,
		`  $MainInstance = "https://junglinster.homelinux.org";`,
		`else`,
		`  $MainInstance = "https://fallback.example";`,
		`end;`,
		`main vienna $MainInstance;`,
	}, "\n")

	assignments := parseServerAssignmentsWithAvailability(serverContent, func(host string) bool {
		return host == "junglinster.fritz.box"
	})
	if assignments["MainInstance"] != "https://junglinster.homelinux.org" {
		t.Fatalf("expected elif branch assignment, got %q", assignments["MainInstance"])
	}
}

func TestParseServerAssignmentsWithAvailabilityFallsBackToElse(t *testing.T) {
	serverContent := strings.Join([]string{
		`if is up "vienna.fritz.box" then`,
		`  $MainInstance = "http://vienna.fritz.box:8123";`,
		`elif is up "junglinster.fritz.box" then`,
		`  $MainInstance = "https://junglinster.homelinux.org";`,
		`else`,
		`  $MainInstance = "https://fallback.example";`,
		`end;`,
		`main vienna $MainInstance;`,
	}, "\n")

	assignments := parseServerAssignmentsWithAvailability(serverContent, func(string) bool { return false })
	if assignments["MainInstance"] != "https://fallback.example" {
		t.Fatalf("expected else branch assignment, got %q", assignments["MainInstance"])
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

func TestMigrationIncludesLegacyIconSettings(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	if err := runMigration(root, THouseNames); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	for _, houseName := range THouseNames {
		settingsPath := filepath.Join(root, "New", houseName, "Definitions", "Settings.def")
		content, readErr := os.ReadFile(settingsPath)
		if readErr != nil {
			t.Fatalf("failed to read %s: %v", settingsPath, readErr)
		}
		text := string(content)
		for _, expectedLine := range []string{
			"$ConsumesIcon = \"mdi:flash\";",
			"$MediaSwitchIcon = \"mdi:monitor-speaker\";",
			"$WaterIcon = \"mdi:water-off\";",
		} {
			if !strings.Contains(text, expectedLine) {
				t.Fatalf("expected %q in %s", expectedLine, settingsPath)
			}
		}
		if strings.Contains(text, "settings:") {
			t.Fatalf("unexpected legacy settings: block in %s", settingsPath)
		}
		if strings.Contains(text, "\nend;\n") {
			t.Fatalf("unexpected settings block terminator in %s", settingsPath)
		}
	}
}

func TestMigrationSecretsKeepMainTokenAndDropObsoleteLegacyKeys(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	if err := runMigration(root, THouseNames); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	for _, houseName := range THouseNames {
		secretsPath := filepath.Join(root, "New", houseName, "Definitions", "Secrets.def")
		content, readErr := os.ReadFile(secretsPath)
		if readErr != nil {
			t.Fatalf("failed to read %s: %v", secretsPath, readErr)
		}
		text := string(content)

		if !strings.Contains(text, "$MainAPIToken") {
			t.Fatalf("expected MainAPIToken in %s", secretsPath)
		}

		if houseName == "Vienna" && !strings.Contains(text, "$JunglinsterAPIToken") {
			t.Fatalf("expected JunglinsterAPIToken in %s", secretsPath)
		}

		for _, obsoleteSecret := range []string{
			"$rest_authorization_xanadu",
			"$smarty_key",
			"$telnet_password",
			"$telnet_port",
			"$volvo_login",
			"$volvo_password",
			"$xiaomi_token",
			"$zigbee_deconz_key",
			"$zigbee_importer_key",
			"$zwave_deconz_home_id",
			"$zwave_zwave_home_id",
			"$junglinster_authorization",
		} {
			if strings.Contains(text, obsoleteSecret) {
				t.Fatalf("unexpected obsolete secret %s in %s", obsoleteSecret, secretsPath)
			}
		}
	}

	viennaBridgesPath := filepath.Join(root, "New", "Vienna", "Definitions", "Bridges.def")
	viennaBridgesContent, readBridgesErr := os.ReadFile(viennaBridgesPath)
	if readBridgesErr != nil {
		t.Fatalf("failed to read %s: %v", viennaBridgesPath, readBridgesErr)
	}
	viennaBridgesText := string(viennaBridgesContent)
	if !strings.Contains(viennaBridgesText, "bridge rest junglinster $JunglinsterInstance/api/states authorization $JunglinsterAPIToken;") {
		t.Fatalf("expected JunglinsterAPIToken bridge authorization in %s", viennaBridgesPath)
	}
}

func TestMigrationServerUsesInlinedIfFormat(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	if err := runMigration(root, THouseNames); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	junglinsterServerPath := filepath.Join(root, "New", "Junglinster", "Definitions", "Server.def")
	junglinsterServerContent, readErr := os.ReadFile(junglinsterServerPath)
	if readErr != nil {
		t.Fatalf("failed to read %s: %v", junglinsterServerPath, readErr)
	}
	junglinsterText := string(junglinsterServerContent)
	for _, expected := range []string{
		"if is up \"junglinster.fritz.box\" then",
		"main junglinster $MainInstance;",
	} {
		if !strings.Contains(junglinsterText, expected) {
			t.Fatalf("expected %q in %s", expected, junglinsterServerPath)
		}
	}
	if strings.Contains(junglinsterText, "servers:") {
		t.Fatalf("unexpected legacy servers: block in %s", junglinsterServerPath)
	}

	viennaServerPath := filepath.Join(root, "New", "Vienna", "Definitions", "Server.def")
	viennaServerContent, readErr := os.ReadFile(viennaServerPath)
	if readErr != nil {
		t.Fatalf("failed to read %s: %v", viennaServerPath, readErr)
	}
	viennaText := string(viennaServerContent)
	for _, expected := range []string{
		"if is up \"vienna.fritz.box\" then",
		"elif is up \"junglinster.fritz.box\" then",
		"$MainInstance = \"http://vienna.fritz.box:8123\";",
		"$MainInstance = \"https://vienna.homelinux.org\";",
		"$JunglinsterInstance = \"http://junglinster.fritz.box:8123\";",
		"main vienna $MainInstance;",
	} {
		if !strings.Contains(viennaText, expected) {
			t.Fatalf("expected %q in %s", expected, viennaServerPath)
		}
	}
	if strings.Contains(viennaText, "servers:") {
		t.Fatalf("unexpected legacy servers: block in %s", viennaServerPath)
	}

	viennaBridgesPath := filepath.Join(root, "New", "Vienna", "Definitions", "Bridges.def")
	viennaBridgesContent, readErr := os.ReadFile(viennaBridgesPath)
	if readErr != nil {
		t.Fatalf("failed to read %s: %v", viennaBridgesPath, readErr)
	}
	viennaBridgesText := string(viennaBridgesContent)
	if strings.Contains(viennaBridgesText, "main vienna") {
		t.Fatalf("unexpected 'main vienna' in %s — it belongs in Server.def", viennaBridgesPath)
	}
}

func TestMigrationNormalizesListsToWithBlocks(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	if err := runMigration(root, THouseNames); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	for _, houseName := range THouseNames {
		definitionPath := filepath.Join(root, "New", houseName, "Definitions", "Lists.def")
		content, readErr := os.ReadFile(definitionPath)
		if readErr != nil {
			t.Fatalf("failed to read %s: %v", definitionPath, readErr)
		}
		text := string(content)
		if !strings.Contains(text, "list \"Windoors\" all binary_sensor.*:*:door binary_sensor.*:*:window with:") {
			t.Fatalf("expected normalized list header in %s", definitionPath)
		}
		if !strings.Contains(text, "\n  clean_prefix  social;\n") {
			t.Fatalf("expected two-space list body indentation in %s", definitionPath)
		}
		if !strings.Contains(text, "\nend;\n") {
			t.Fatalf("expected list end; aligned with list header in %s", definitionPath)
		}
		if strings.Contains(text, "\n  begin\n") {
			t.Fatalf("unexpected legacy begin block in %s", definitionPath)
		}
	}
}

func TestMigrationNormalizesPowerSwitchCreateBlocks(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	if err := runMigration(root, THouseNames); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	definitionPath := filepath.Join(root, "New", "Vienna", "Definitions", "Entities.def")
	content, readErr := os.ReadFile(definitionPath)
	if readErr != nil {
		t.Fatalf("failed to read %s: %v", definitionPath, readErr)
	}
	text := string(content)
	for _, expectedLine := range []string{
		"power_switch switch.social:dishwasher with:",
		"  node robb;",
		"  threshold 1;",
	} {
		if !strings.Contains(text, expectedLine) {
			t.Fatalf("expected %q in %s", expectedLine, definitionPath)
		}
	}
	if strings.Contains(text, "create power_switch social dishwasher robb 1 with:") {
		t.Fatalf("unexpected legacy power_switch form still present in %s", definitionPath)
	}
}

func TestNormalizeMacroBodyRemovesLegacyBeginBlocks(t *testing.T) {
	body := []string{
		`if [ "$1" != "" ] ; then`,
		`create thing one`,
		`else`,
		`create thing two`,
		`fi`,
		`begin space social:test`,
		`create thing three`,
		`end space`,
	}

	normalized := strings.Join(normalizeMacroBody(body, []string{"entity"}), "\n")

	for _, unexpected := range []string{"\nbegin\n", "\n  begin\n", "\n    begin\n"} {
		if strings.Contains(normalized, unexpected) {
			t.Fatalf("unexpected legacy begin block in normalized macro body:\n%s", normalized)
		}
	}

	for _, expected := range []string{
		"if \"$entity\" != \"\" then",
		"else",
		"end;",
		"space social:test with:",
	} {
		if !strings.Contains(normalized, expected) {
			t.Fatalf("expected %q in normalized macro body:\n%s", expected, normalized)
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
			{Name: "$alertLevel", Kind: ParamInt, Optional: true},
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
					{Name: "$alertLevel", Kind: ParamInt, Optional: true},
				},
				Body: []string{`definition as condition sensor.infrastructural:$entity:battery_level "($ | int(0)) < $alertLevel";`},
			},
		},
		Config: TExpanderConfig{CheckTypes: true},
	}
	invocation := &TMacroInvocation{
		Name:       "battery_alert",
		Target:     "roborock",
		Parameters: map[string]string{"alert_level": "15"},
	}

	expanded, _, err := ctx.ExpandMacro(invocation)
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
	param, err := parseParameter("$no_collect option")
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

func TestMigrationNormalizesSunDeclarationAsRawEntity(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	if err := runMigration(root, THouseNames); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	for _, houseName := range THouseNames {
		definitionPath := filepath.Join(root, "New", houseName, "Definitions", "Entities.def")
		content, readErr := os.ReadFile(definitionPath)
		if readErr != nil {
			t.Fatalf("failed to read %s: %v", definitionPath, readErr)
		}
		text := string(content)
		if !strings.Contains(text, "entity sun.[sun];") {
			t.Fatalf("expected 'entity sun.[sun];' in %s", definitionPath)
		}
		if strings.Contains(text, "entity sun.sun;") {
			t.Fatalf("unexpected 'entity sun.sun;' in %s", definitionPath)
		}
		if strings.Contains(text, "declare entity ") {
			t.Fatalf("unexpected 'declare entity' in %s — should be rewritten to 'entity'", definitionPath)
		}
	}
}

func TestMigrationNormalizesSunAttributeReference(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	if err := runMigration(root, THouseNames); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	definitionPath := filepath.Join(root, "New", "Vienna", "Definitions", "Entities.def")
	content, readErr := os.ReadFile(definitionPath)
	if readErr != nil {
		t.Fatalf("failed to read %s: %v", definitionPath, readErr)
	}
	text := string(content)
	if !strings.Contains(text, "definition as condition sun.[sun]!elevation \"$ > 4\";") {
		t.Fatalf("expected normalized sun attribute reference in %s", definitionPath)
	}
	if strings.Contains(text, "condition sun.sun:/!elevation") {
		t.Fatalf("unexpected legacy sun attribute reference in %s", definitionPath)
	}
}

func TestMigrationNormalizesLightsMotionGuardedAsInlineWith(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	if err := runMigration(root, THouseNames); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	definitionPath := filepath.Join(root, "New", "Vienna", "Definitions", "Entities.def")
	content, readErr := os.ReadFile(definitionPath)
	if readErr != nil {
		t.Fatalf("failed to read %s: %v", definitionPath, readErr)
	}
	text := string(content)
	if !strings.Contains(text, "lights_motion_guarded with delay 15;") {
		t.Fatalf("expected normalized lights_motion_guarded inline-with form in %s", definitionPath)
	}
	if strings.Contains(text, "lights_motion_guarded 15;") {
		t.Fatalf("unexpected positional lights_motion_guarded form in %s", definitionPath)
	}
}

func TestMigrationNormalizesSunnyAsBlock(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	if err := runMigration(root, THouseNames); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	definitionPath := filepath.Join(root, "New", "Vienna", "Definitions", "Entities.def")
	content, readErr := os.ReadFile(definitionPath)
	if readErr != nil {
		t.Fatalf("failed to read %s: %v", definitionPath, readErr)
	}
	text := string(content)
	if !strings.Contains(text, "sunny physical:signify_motion:illuminance with:\n    delay_on 00:05:00;\n    delay_off 00:05:00;\n  end;") {
		t.Fatalf("expected normalized sunny block in %s", definitionPath)
	}
	if !strings.Contains(text, "windy social:wind_speed with:\n    delay_on 00:01:00;\n    delay_off 00:10:00;\n  end;") {
		t.Fatalf("expected normalized windy block in %s", definitionPath)
	}
}

func TestMigrationNormalizesLightDeviceSphereAndName(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	if err := runMigration(root, THouseNames); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	definitionPath := filepath.Join(root, "New", "Vienna", "Definitions", "Entities.def")
	content, readErr := os.ReadFile(definitionPath)
	if readErr != nil {
		t.Fatalf("failed to read %s: %v", definitionPath, readErr)
	}
	text := string(content)
	if !strings.Contains(text, "light_device physical:left;") {
		t.Fatalf("expected normalized light_device physical:left form in %s", definitionPath)
	}
	if !strings.Contains(text, "light_device physical:right;") {
		t.Fatalf("expected normalized light_device physical:right form in %s", definitionPath)
	}
	if strings.Contains(text, "light_device physical left;") {
		t.Fatalf("unexpected positional light_device form with separate sphere and name in %s", definitionPath)
	}
	if strings.Contains(text, "light_device physical right;") {
		t.Fatalf("unexpected positional light_device form with separate sphere and name in %s", definitionPath)
	}
}

func TestMigrationNormalizesMediaPlayerAsBlock(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	if err := runMigration(root, THouseNames); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	definitionPath := filepath.Join(root, "New", "Vienna", "Definitions", "Entities.def")
	content, readErr := os.ReadFile(definitionPath)
	if readErr != nil {
		t.Fatalf("failed to read %s: %v", definitionPath, readErr)
	}
	text := string(content)
	if !strings.Contains(text, "media_player tv with:") {
		t.Fatalf("expected media_player tv with: block form in %s", definitionPath)
	}
	if !strings.Contains(text, "enabler switch.social:tv;") {
		t.Fatalf("expected enabler switch.social:tv; in media_player block in %s", definitionPath)
	}
	if !strings.Contains(text, "delay_off 00:01:00;") {
		t.Fatalf("expected delay_off 00:01:00; in media_player block in %s", definitionPath)
	}
	if strings.Contains(text, "media_player tv switch.social:tv") {
		t.Fatalf("unexpected positional media_player form in %s", definitionPath)
	}
}

func TestMigrationNormalizesMediaPlayerSpecialCases(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	if err := runMigration(root, []string{"Junglinster"}); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	definitionPath := filepath.Join(root, "New", "Junglinster", "Definitions", "Entities.def")
	content, readErr := os.ReadFile(definitionPath)
	if readErr != nil {
		t.Fatalf("failed to read %s: %v", definitionPath, readErr)
	}
	text := string(content)
	if !strings.Contains(text, "media_player sonos with no_play_input \"TV\";") {
		t.Fatalf("expected normalized no_play media_player block in %s", definitionPath)
	}
	if !strings.Contains(text, "media_player apple_tv with:\n      no_collect;\n      enabler switch.social:cycling;\n    end;") && !strings.Contains(text, "media_player apple_tv with no_collect enabler switch.social:cycling;") {
		t.Fatalf("expected normalized no_collect media_player block in %s", definitionPath)
	}
	if !strings.Contains(text, "media_player tv with:\n      no_collect;\n      enabler switch.social:cycling;\n    end;") && !strings.Contains(text, "media_player tv with no_collect enabler switch.social:cycling;") {
		t.Fatalf("expected normalized no_collect tv media_player block in %s", definitionPath)
	}
	if strings.Contains(text, "media_player sonos with:\n      enabler no_play;") {
		t.Fatalf("unexpected legacy no_play normalization in %s", definitionPath)
	}
}

func TestMigrationCollapesSingleStatementWithBlocks(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	if err := runMigration(root, THouseNames); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	definitionPath := filepath.Join(root, "New", "Vienna", "Definitions", "Entities.def")
	content, readErr := os.ReadFile(definitionPath)
	if readErr != nil {
		t.Fatalf("failed to read %s: %v", definitionPath, readErr)
	}
	text := string(content)
	if !strings.Contains(text, "battery_level_device bed/moes with alert_level 20;") {
		t.Fatalf("expected collapsed battery_level_device inline-with form in %s", definitionPath)
	}
	if !strings.Contains(text, "battery_alert roborock with alert_level 15;") {
		t.Fatalf("expected collapsed battery_alert inline-with form in %s", definitionPath)
	}
	if !strings.Contains(text, "zigbee_group light.social:main with group { kitchen, middle, living };") {
		t.Fatalf("expected collapsed zigbee_group inline-with form in %s", definitionPath)
	}
	if strings.Contains(text, "battery_level_device bed/moes with:\n") {
		t.Fatalf("unexpected multi-line battery_level_device block form in %s", definitionPath)
	}
}

func TestMigrationKeepsCuratedMacrosDefinition(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	macrosPath := filepath.Join(root, "New", "Junglinster", "Definitions", "Macros.def")
	originalContent, readErr := os.ReadFile(macrosPath)
	if readErr != nil {
		t.Fatalf("failed to read %s: %v", macrosPath, readErr)
	}

	curatedMarker := "\n# curated-macros-marker\n"
	updatedContent := string(originalContent)
	if !strings.Contains(updatedContent, curatedMarker) {
		updatedContent += curatedMarker
	}
	if writeErr := os.WriteFile(macrosPath, []byte(updatedContent), 0o644); writeErr != nil {
		t.Fatalf("failed to write %s: %v", macrosPath, writeErr)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(macrosPath, originalContent, 0o644)
	})

	if err := runMigration(root, []string{"Junglinster"}); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	contentAfterMigration, readErr := os.ReadFile(macrosPath)
	if readErr != nil {
		t.Fatalf("failed to re-read %s after migration: %v", macrosPath, readErr)
	}
	if !strings.Contains(string(contentAfterMigration), curatedMarker) {
		t.Fatalf("expected curated macros file to be preserved, marker missing in %s", macrosPath)
	}
}

func TestMigrationGeneratesMainDefinitionWithLogicalIncludeOrder(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	if err := runMigration(root, THouseNames); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	for _, houseName := range THouseNames {
		mainPath := filepath.Join(root, "New", houseName, "Definitions", "Main.def")
		content, readErr := os.ReadFile(mainPath)
		if readErr != nil {
			t.Fatalf("failed to read %s: %v", mainPath, readErr)
		}
		text := string(content)
		expectedOrder := []string{
			"include Macros.def;",
			"include Secrets.def;",
			"include Settings.def;",
			"include Server.def;",
			"include Bridges.def;",
			"include Entities.def;",
			"include Lists.def;",
		}
		lastIndex := -1
		for _, expectedInclude := range expectedOrder {
			currentIndex := strings.Index(text, expectedInclude)
			if currentIndex < 0 {
				t.Fatalf("expected %q in %s", expectedInclude, mainPath)
			}
			if currentIndex <= lastIndex {
				t.Fatalf("expected include order %v in %s", expectedOrder, mainPath)
			}
			lastIndex = currentIndex
		}
	}
}

func TestInterpretationRespectsMainIncludeOrder(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to determine working directory: %v", err)
	}

	if err := runMigration(root, []string{"Vienna"}); err != nil {
		t.Fatalf("migration failed: %v", err)
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
