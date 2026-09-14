package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyValueMapIdentityWhenUndeclared(t *testing.T) {
	got := applyValueMap(nil, "states('vacuum.roomba')")
	want := "states('vacuum.roomba')"
	if got != want {
		t.Errorf("applyValueMap(nil, ...) = %q, want %q (identity)", got, want)
	}
}

// TestApplyValueMapTranslatesAndPassesThroughUnknown is a regression test for the real gap found
// live 2026-09-09: a Roomba's own native vocabulary ("home", "run", ...) doesn't match HA's MQTT
// vacuum activity strings at all. Also confirms this is a translation table, not an enum
// whitelist -- an unmapped value must pass through unchanged, never error.
func TestApplyValueMapTranslatesAndPassesThroughUnknown(t *testing.T) {
	valueMap := map[string]string{"home": "docked", "run": "cleaning"}
	got := applyValueMap(valueMap, "states('vacuum.roomba')")
	want := "{'home': 'docked', 'run': 'cleaning'}.get(states('vacuum.roomba'), states('vacuum.roomba'))"
	if got != want {
		t.Errorf("applyValueMap(...) = %q, want %q", got, want)
	}
}

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
  entity sensor.physical:netatmo/co2      from hass.davids_bedroom sensor.co2;
  entity sensor.physical:netatmo/humidity from hass.davids_bedroom sensor.humidity;
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

// TestResolveHassBridgeCapabilityExprAtomic covers the ordinary (non-derived) case, unchanged
// behaviour from before resolveHassBridgeCapabilityExpr existed: ValueMap applied, source turned
// into a states()/state_attr() Jinja read, bareEntityFromSource as the trigger.
func TestResolveHassBridgeCapabilityExprAtomic(t *testing.T) {
	device := THassBridgeDevice{Capabilities: map[string]THassBridgeCapability{
		"status": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.roomba_status"}, ValueMap: map[string]string{"home": "docked"}},
	}}
	expr, trigger, ok := resolveHassBridgeCapabilityExpr(device, "status", "protocols-server-2", map[string]bool{})
	if !ok {
		t.Fatalf("expected a resolved expression")
	}
	if trigger != "sensor.roomba_status" {
		t.Errorf("trigger = %q, want sensor.roomba_status", trigger)
	}
	if !strings.Contains(expr, "states('sensor.roomba_status')") || !strings.Contains(expr, "'docked'") {
		t.Errorf("expr = %q, want it to read the source and apply the value map", expr)
	}
}

// TestResolveHassBridgeCapabilityExprDerived is a focused test for
// plans/derived-capability-mechanism.md Phase 2's runtime half for hassbridge (kind-3) devices:
// a "derived" capability's value is composed from its sibling's own resolved expression, and its
// trigger entity is the sibling's real HA entity (not itself, since it has none).
func TestResolveHassBridgeCapabilityExprDerived(t *testing.T) {
	device := THassBridgeDevice{Capabilities: map[string]THassBridgeCapability{
		"battery_level": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.roomba_battery_level"}},
		"battery_alert": {Domain: "binary_sensor", DerivedFromCapability: "battery_level", DerivedViaTemplate: "( $ | int(0) < 20 )"},
	}}
	expr, trigger, ok := resolveHassBridgeCapabilityExpr(device, "battery_alert", "protocols-server-2", map[string]bool{})
	if !ok {
		t.Fatalf("expected a resolved expression")
	}
	if trigger != "sensor.roomba_battery_level" {
		t.Errorf("trigger = %q, want the ultimate atomic base's own entity (sensor.roomba_battery_level)", trigger)
	}
	want := "( (states('sensor.roomba_battery_level')) | int(0) < 20 )"
	if expr != want {
		t.Errorf("expr = %q, want %q", expr, want)
	}
}

// TestResolveHassBridgeCapabilityExprDerivedMissingInstanceSource covers a derived capability
// whose atomic base was never declared for the instance being resolved against -- must fail
// cleanly (ok=false), matching how an ordinary capability with no Source for this instance is
// skipped, not crash.
func TestResolveHassBridgeCapabilityExprDerivedMissingInstanceSource(t *testing.T) {
	device := THassBridgeDevice{Capabilities: map[string]THassBridgeCapability{
		"battery_level": {Domain: "sensor", Sources: map[string]string{"main": "sensor.roomba_battery_level"}},
		"battery_alert": {Domain: "binary_sensor", DerivedFromCapability: "battery_level", DerivedViaTemplate: "( $ | int(0) < 20 )"},
	}}
	if _, _, ok := resolveHassBridgeCapabilityExpr(device, "battery_alert", "protocols-server-2", map[string]bool{}); ok {
		t.Errorf("expected ok=false: battery_level has no Source for instance \"protocols-server-2\"")
	}
}

// TestGenerateHassBridgeEntityReportingAutomationsForDerivedCapability is the end-to-end
// counterpart: a positioned derived capability gets its own reporting automation, composing its
// value from its sibling's source and triggering on the sibling's real entity.
func TestGenerateHassBridgeEntityReportingAutomationsForDerivedCapability(t *testing.T) {
	const miniDSL = `space social:living_room with:
  device infrastructural:vacuum from hass.roomba;
  entity sensor.physical:vacuum/battery_level from hass.roomba sensor.battery_level;
  entity binary_sensor.physical:vacuum/battery_alert from hass.roomba binary_sensor.battery_alert;
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.roomba": {
			DeviceID:  "hass.roomba",
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"battery_level": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.roomba_battery_level"}},
				"battery_alert": {Domain: "binary_sensor", DerivedFromCapability: "battery_level", DerivedViaTemplate: "( $ | int(0) < 20 )"},
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

	alertFile := filepath.Join(outputDir, "automation", "infrastructural", "automation.reporting_binary_sensor_physical_living_room_vacuum_battery_alert.yaml")
	body, err := os.ReadFile(alertFile)
	if err != nil {
		t.Fatalf("reading derived-capability automation file: %v", err)
	}
	if !strings.Contains(string(body), "sensor.roomba_battery_level") {
		t.Errorf("derived automation = %q, want it to reference its sibling's own source entity as the trigger", body)
	}
	if !strings.Contains(string(body), "int(0) < 20") {
		t.Errorf("derived automation = %q, want the derived template composed into the payload expression", body)
	}
}
