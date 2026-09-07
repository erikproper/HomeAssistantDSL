package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFullyQualifiedEntityNameForms(t *testing.T) {
	identity := TEntityIdentity{Domain: "sensor", Sphere: "infrastructural", Path: "netatmo/co2"}
	if got := fullyQualifiedEntityNameUnderscored(identity); got != "sensor_infrastructural_netatmo_co2" {
		t.Errorf("fullyQualifiedEntityNameUnderscored = %q, want %q", got, "sensor_infrastructural_netatmo_co2")
	}
	if got := fullyQualifiedEntityNameSlashed(identity); got != "sensor/infrastructural/netatmo/co2" {
		t.Errorf("fullyQualifiedEntityNameSlashed = %q, want %q", got, "sensor/infrastructural/netatmo/co2")
	}
}

func TestFullyQualifiedEntityNameSingleSegmentPath(t *testing.T) {
	identity := TEntityIdentity{Domain: "binary_sensor", Sphere: "infrastructural", Path: "laserjet"}
	if got := fullyQualifiedEntityNameUnderscored(identity); got != "binary_sensor_infrastructural_laserjet" {
		t.Errorf("fullyQualifiedEntityNameUnderscored = %q, want %q", got, "binary_sensor_infrastructural_laserjet")
	}
}

func TestEntityReportingAutomationBody(t *testing.T) {
	identity := TEntityIdentity{Domain: "sensor", Sphere: "infrastructural", Path: "netatmo/co2"}
	body := entityReportingAutomationBody(identity, "sensor.davids_bedroom_carbon_dioxide", "homeassistant_instances/protocols-server-2/bridge/sensor.infrastructural_netatmo_co2/state", "states('sensor.davids_bedroom_carbon_dioxide')")

	if !strings.Contains(body, `alias: "reporting/sensor/infrastructural/netatmo/co2"`) {
		t.Errorf("body = %s, want the slash-joined alias", body)
	}
	if !strings.Contains(body, "entity_id: sensor.davids_bedroom_carbon_dioxide") {
		t.Errorf("body = %s, want a state trigger on the bare source entity", body)
	}
	if !strings.Contains(body, "platform: homeassistant") || !strings.Contains(body, "event: start") {
		t.Errorf("body = %s, want a homeassistant-start trigger", body)
	}
	// Regression test, 2026-08-29: without a reload trigger, a reporting automation only
	// republishes on a full HA restart or the next incidental source state-change -- confirmed
	// live, a freshly-redeployed reporting automation sat on "unknown" with no reload-triggered
	// republish. HA's own "homeassistant" trigger platform has no "reload" event (only
	// start/shutdown); the automation integration fires a separate "automation_reloaded" event on
	// the generic event bus instead.
	if !strings.Contains(body, "platform: event") || !strings.Contains(body, "event_type: automation_reloaded") {
		t.Errorf("body = %s, want an \"event: automation_reloaded\" trigger so a plain automations reload also republishes current state", body)
	}
	if !strings.Contains(body, `topic: "homeassistant_instances/protocols-server-2/bridge/sensor.infrastructural_netatmo_co2/state"`) {
		t.Errorf("body = %s, want the given topic", body)
	}
	if !strings.Contains(body, `payload: "{{ states('sensor.davids_bedroom_carbon_dioxide') }}"`) {
		t.Errorf("body = %s, want the given payload expression wrapped in Jinja braces", body)
	}
}

func TestGenerateHassBridgeEntityReportingAutomationsOneFilePerCapability(t *testing.T) {
	const miniDSL = `space social:david_bedroom with:
  device infrastructural:netatmo from hass.davids_bedroom;
  entity sensor.physical:netatmo/co2      from hass.davids_bedroom entity sensor.co2;
  entity sensor.physical:netatmo/humidity from hass.davids_bedroom entity sensor.humidity;
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.davids_bedroom": {
			DeviceID:  "hass.davids_bedroom",
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"co2":      {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.davids_bedroom_carbon_dioxide"}},
				"humidity": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.davids_bedroom_humidity"}},
			},
		},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	outputDir := t.TempDir()
	if err := generateHassBridgeEntityReportingAutomations(outputDir, "protocols-server-2", hassBridgeDevicesByID, result.Administration); err != nil {
		t.Fatalf("generateHassBridgeEntityReportingAutomations error: %v", err)
	}

	// The entity spec is space-relative ("netatmo/co2" inside "space social:david_bedroom"), so
	// the enclosing space's own leaf path folds into the filename -- but the *directory* is always
	// "infrastructural" regardless of the entity's own sphere (this is physical-layer plumbing,
	// not a conceptual placement).
	co2File := filepath.Join(outputDir, "automation", "infrastructural", "automation.reporting_sensor_physical_david_bedroom_netatmo_co2.yaml")
	humidityFile := filepath.Join(outputDir, "automation", "infrastructural", "automation.reporting_sensor_physical_david_bedroom_netatmo_humidity.yaml")

	if _, err := os.Stat(co2File); err != nil {
		t.Errorf("expected %s to exist: %v", co2File, err)
	}
	if _, err := os.Stat(humidityFile); err != nil {
		t.Errorf("expected %s to exist: %v", humidityFile, err)
	}

	co2Body, err := os.ReadFile(co2File)
	if err != nil {
		t.Fatalf("reading co2 file: %v", err)
	}
	if !strings.Contains(string(co2Body), "sensor.davids_bedroom_carbon_dioxide") {
		t.Errorf("co2 automation = %q, want it to reference its own source entity, not humidity's", co2Body)
	}
	if strings.Contains(string(co2Body), "humidity") {
		t.Errorf("co2 automation = %q, want it to be entirely independent of the humidity capability -- one file per entity", co2Body)
	}
}
