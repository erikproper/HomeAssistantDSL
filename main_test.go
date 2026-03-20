/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: MainTest
 *
 * This component provides a smoke test that keeps migration operational while legacy source files evolve.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 18.03.2026
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

	definitionNames := []string{"Settings.def", "Secrets.def", "Server.def", "Bridges.def", "Entities.def", "Lists.def", "Macros.def"}
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
		interpretationPath := filepath.Join(root, "New", houseName, "interpretation.txt")
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
			"  ConsumesIcon \"mdi:flash\";",
			"  MediaSwitchIcon \"mdi:monitor-speaker\";",
			"  WaterIcon \"mdi:water-off\";",
		} {
			if !strings.Contains(text, expectedLine) {
				t.Fatalf("expected %q in %s", expectedLine, settingsPath)
			}
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
	if !strings.Contains(text, "condition sun.[sun]!elevation \"$ > 4\";") {
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
