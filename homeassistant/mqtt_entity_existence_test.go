package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecognizeCapability(t *testing.T) {
	cases := []struct {
		entityID   string
		wantDomain string
		wantSuffix string
	}{
		{"binary_sensor.davids_bedroom_connectivity", "binary_sensor", "node"},
		{"sensor.davids_bedroom_carbon_dioxide", "sensor", "co2"},
		{"sensor.davids_bedroom_atmospheric_pressure", "sensor", "pressure"},
		{"sensor.davids_bedroom_humidity", "sensor", "humidity"},
		{"sensor.davids_bedroom_noise", "sensor", "noise"},
		{"sensor.davids_bedroom_temperature", "sensor", "temperature"},
		{"sensor.davids_bedroom_wi_fi_strength", "sensor", "radio"},
		{"sensor.office_garden_rf_strength", "sensor", "radio"},
		{"sensor.hewlett_packard_hp_laserjet_professional_p1102w_uptime", "sensor", "uptime"},
		{"sensor.davids_bedroom_health_index", "sensor", ""}, // not recognised
	}
	for _, c := range cases {
		domain, suffix := recognizeCapability(c.entityID)
		if domain != c.wantDomain || suffix != c.wantSuffix {
			t.Errorf("recognizeCapability(%q) = (%q, %q), want (%q, %q)", c.entityID, domain, suffix, c.wantDomain, c.wantSuffix)
		}
	}
}

func TestUsedHassBridgeEntityIDsFiltersByInstanceAndStripsAttributeSuffix(t *testing.T) {
	devicesByID := map[string]THassBridgeDevice{
		"hass.davids_bedroom": {
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"co2":  {Sources: map[string]string{"protocols-server-2": "sensor.davids_bedroom_carbon_dioxide"}},
				"node": {Sources: map[string]string{"protocols-server-2": "sensor.davids_bedroom_connectivity is available"}},
			},
			DeviceInfoCapabilities: map[string]string{
				"sw_version": "update.foo!installed_version",
			},
		},
		"hass.other_instance_device": {
			Instances: []string{"some_other_instance"},
			Capabilities: map[string]THassBridgeCapability{
				"x": {Sources: map[string]string{"some_other_instance": "sensor.should_not_appear"}},
			},
		},
	}

	used := usedHassBridgeEntityIDs(devicesByID, "protocols-server-2")

	want := map[string]bool{
		"sensor.davids_bedroom_carbon_dioxide": true,
		"sensor.davids_bedroom_connectivity":   true,
		"update.foo":                           true,
	}
	if len(used) != len(want) {
		t.Fatalf("got %v, want %v", used, want)
	}
	for id := range want {
		if !used[id] {
			t.Errorf("expected %q to be marked used, got %v", id, used)
		}
	}
	if used["sensor.should_not_appear"] {
		t.Errorf("device belonging to a different instance leaked into the used set: %v", used)
	}
}

func TestBuildSuggestionReportFromExistenceExcludesUsedAndUnresolved(t *testing.T) {
	status := TEntityExistenceStatusPayload{
		"hass.davids_bedroom": {Entities: map[string]TEntityExistenceStatusEntry{
			"sensor.davids_bedroom_carbon_dioxide": {Status: existenceStatusKnownToExist},
			"sensor.davids_bedroom_health_index":   {Status: existenceStatusKnownToExist},
			"sensor.already_used":                  {Status: existenceStatusKnownToExist},
			"sensor.still_unresolved":              {Status: "not-known-to-exist"},
			"sensor.confirmed_gone":                {Status: existenceStatusKnownNotToExist},
		}},
		"": {Entities: map[string]TEntityExistenceStatusEntry{
			"sensor.standalone_thing": {Status: existenceStatusKnownToExist},
		}},
	}
	used := map[string]bool{"sensor.already_used": true}

	report := buildSuggestionReportFromExistence(status, used, nil)

	if !containsAll(report,
		"device hass.davids_bedroom with:",
		"sensor.co2:",
		"sensor.davids_bedroom_carbon_dioxide;",
		"sensor.:",
		"sensor.davids_bedroom_health_index;",
		"# not recognized",
		"end;",
		"# entities with no known device grouping:",
		"# sensor.standalone_thing",
	) {
		t.Errorf("report missing expected content:\n%s", report)
	}
	for _, unwanted := range []string{"sensor.already_used", "sensor.still_unresolved", "sensor.confirmed_gone"} {
		if strings.Contains(report, unwanted) {
			t.Errorf("report must exclude %q (used/unresolved/confirmed-gone), got:\n%s", unwanted, report)
		}
	}
}

func TestBuildSuggestionReportFromExistenceEmptyWhenNothingToSuggest(t *testing.T) {
	status := TEntityExistenceStatusPayload{
		"hass.davids_bedroom": {Entities: map[string]TEntityExistenceStatusEntry{
			"sensor.davids_bedroom_carbon_dioxide": {Status: "not-known-to-exist"},
		}},
	}
	if report := buildSuggestionReportFromExistence(status, nil, nil); strings.TrimSpace(report) != "" {
		t.Errorf("expected an empty report when nothing is known-to-exist, got:\n%s", report)
	}
}

// TestBuildSuggestionReportFromExistenceOverridesSyntheticDeviceIDWithDeclaredName is the
// regression test for the gap the user hit live 2026-08-28: after declaring
// "device hass.office_garden with: ...; end;" in Physical.def for 3 of a coordinator-discovered
// device's 5 entities, the still-undeclared 2 must be suggested under "hass.office_garden" too --
// not the coordinator's synthetic "hass.discovered_office_garden" -- since Physical.def is the
// naming authority, not the coordinator (mqtt_entity_existence.go's own doc comment).
func TestBuildSuggestionReportFromExistenceOverridesSyntheticDeviceIDWithDeclaredName(t *testing.T) {
	status := TEntityExistenceStatusPayload{
		"hass.discovered_office_garden": {Entities: map[string]TEntityExistenceStatusEntry{
			"binary_sensor.office_garden_connectivity": {Status: existenceStatusKnownToExist},
			"sensor.office_garden_battery":             {Status: existenceStatusKnownToExist},
			"sensor.office_garden_humidity":            {Status: existenceStatusKnownToExist},
			"sensor.office_garden_rf_strength":         {Status: existenceStatusKnownToExist},
			"sensor.office_garden_temperature":         {Status: existenceStatusKnownToExist},
		}},
	}
	used := map[string]bool{
		"binary_sensor.office_garden_connectivity": true,
		"sensor.office_garden_battery":             true,
		"sensor.office_garden_humidity":            true,
	}
	declared := map[string]string{
		"binary_sensor.office_garden_connectivity": "hass.office_garden",
		"sensor.office_garden_battery":             "hass.office_garden",
		"sensor.office_garden_humidity":            "hass.office_garden",
	}

	report := buildSuggestionReportFromExistence(status, used, declared)

	if !containsAll(report,
		"device hass.office_garden with:",
		"sensor.office_garden_rf_strength;",
		"sensor.temperature:",
		"sensor.office_garden_temperature;",
	) {
		t.Errorf("report missing expected content:\n%s", report)
	}
	if strings.Contains(report, "hass.discovered_office_garden") {
		t.Errorf("report still uses the synthetic device id even though a real name is declared:\n%s", report)
	}
	for _, unwanted := range []string{"binary_sensor.office_garden_connectivity;", "sensor.office_garden_battery;", "sensor.office_garden_humidity;"} {
		if strings.Contains(report, unwanted) {
			t.Errorf("report must exclude already-declared %q, got:\n%s", unwanted, report)
		}
	}
}

func TestExistenceStatusTopic(t *testing.T) {
	got := existenceStatusTopic("protocols-server-2")
	want := "homeassistant_instances/protocols-server-2/existence/state"
	if got != want {
		t.Errorf("existenceStatusTopic() = %q, want %q", got, want)
	}
}

func TestFetchEntityExistenceFallsBackToLocalCacheOnFetchFailure(t *testing.T) {
	definitionDir := t.TempDir()
	cachePath := existenceCachePath(definitionDir, "protocols-server-2")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	cached := TEntityExistenceStatusPayload{
		"hass.davids_bedroom": {Entities: map[string]TEntityExistenceStatusEntry{
			"sensor.davids_bedroom_carbon_dioxide": {Status: "known-to-exist", State: "412.3"},
		}},
	}
	data, _ := json.Marshal(cached)
	if err := os.WriteFile(cachePath, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx := TPhysicalGenerationContext{MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}
	status, err := fetchEntityExistence(definitionDir, ctx, "protocols-server-2")
	if err != nil {
		t.Fatalf("fetchEntityExistence error: %v -- want it to fall back to the cache instead", err)
	}
	entry := status["hass.davids_bedroom"].Entities["sensor.davids_bedroom_carbon_dioxide"]
	if entry.Status != "known-to-exist" || entry.State != "412.3" {
		t.Errorf("got %+v, want the cached entry", entry)
	}
}

func TestFetchEntityExistenceFailsWhenNeitherFetchNorCacheAvailable(t *testing.T) {
	definitionDir := t.TempDir()
	ctx := TPhysicalGenerationContext{MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}
	if _, err := fetchEntityExistence(definitionDir, ctx, "protocols-server-2"); err == nil {
		t.Errorf("expected an error when neither a live fetch nor a cache file is available")
	}
}

func seedExistenceCache(t *testing.T, definitionDir, instanceName string, payload TEntityExistenceStatusPayload) {
	t.Helper()
	cachePath := existenceCachePath(definitionDir, instanceName)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(cachePath, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestCheckKnownNotToExistErrorsFlagsConfirmedAbsence(t *testing.T) {
	definitionDir := t.TempDir()
	seedExistenceCache(t, definitionDir, "protocols-server-2", TEntityExistenceStatusPayload{
		"hass.davids_bedroom": {Entities: map[string]TEntityExistenceStatusEntry{
			"sensor.davids_bedroom_carbon_dioxide": {Status: "known-not-to-exist"},
			"sensor.davids_bedroom_humidity":       {Status: "known-to-exist", State: "55"},
		}},
	})

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

	ctx := TPhysicalGenerationContext{MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}
	err := checkKnownNotToExistErrors(definitionDir, hassBridgeDevicesByID, ctx)
	if err == nil {
		t.Fatalf("expected an error for the confirmed-absent co2 capability")
	}
	if !strings.Contains(err.Error(), "co2") || !strings.Contains(err.Error(), "sensor.davids_bedroom_carbon_dioxide") {
		t.Errorf("error = %v, want it to name the co2 capability and its source entity", err)
	}
	if strings.Contains(err.Error(), "humidity") {
		t.Errorf("error = %v, want it to NOT flag humidity -- it's known-to-exist", err)
	}
}

// TestCheckMainEntityKnownNotToExistErrorsFlagsConfirmedAbsence confirms a bare main-instance
// entity (kind-5, PROJECT.md item 1) is flagged only when the coordinator has actually confirmed
// it known-not-to-exist -- scanning every device bucket in the payload (including the "" no-device
// one), since a bare entity has no device grouping known ahead of time, unlike a hassbridge
// capability's own declared device id.
func TestCheckMainEntityKnownNotToExistErrorsFlagsConfirmedAbsence(t *testing.T) {
	definitionDir := t.TempDir()
	seedExistenceCache(t, definitionDir, "main", TEntityExistenceStatusPayload{
		"": {Entities: map[string]TEntityExistenceStatusEntry{
			"sensor.physical_door_aqara_multi_temperature": {Status: "known-not-to-exist"},
		}},
		"hass.discovered_door": {Entities: map[string]TEntityExistenceStatusEntry{
			"sensor.physical_door_aqara_multi_humidity": {Status: "known-to-exist", State: "42"},
		}},
	})

	mainEntityIDs := []string{"sensor.physical_door_aqara_multi_temperature", "sensor.physical_door_aqara_multi_humidity"}
	ctx := TPhysicalGenerationContext{MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}
	err := checkMainEntityKnownNotToExistErrors(definitionDir, mainEntityIDs, ctx)
	if err == nil {
		t.Fatalf("expected an error for the confirmed-absent temperature entity")
	}
	if !strings.Contains(err.Error(), "sensor.physical_door_aqara_multi_temperature") {
		t.Errorf("error = %v, want it to name the confirmed-absent entity", err)
	}
	if strings.Contains(err.Error(), "humidity") {
		t.Errorf("error = %v, want it to NOT flag humidity -- it's known-to-exist", err)
	}
}

func TestCheckMainEntityKnownNotToExistErrorsOptimisticWhenUnresolved(t *testing.T) {
	definitionDir := t.TempDir()
	seedExistenceCache(t, definitionDir, "main", TEntityExistenceStatusPayload{
		"": {Entities: map[string]TEntityExistenceStatusEntry{
			// No entry at all for this entity -- the coordinator hasn't inquired about it yet.
		}},
	})

	ctx := TPhysicalGenerationContext{MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}
	err := checkMainEntityKnownNotToExistErrors(definitionDir, []string{"sensor.physical_door_aqara_multi_temperature"}, ctx)
	if err != nil {
		t.Errorf("unresolved status must not block generation, got: %v", err)
	}
}

func TestCheckMainEntityKnownNotToExistErrorsNoEntitiesIsNoop(t *testing.T) {
	definitionDir := t.TempDir()
	ctx := TPhysicalGenerationContext{MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}
	if err := checkMainEntityKnownNotToExistErrors(definitionDir, nil, ctx); err != nil {
		t.Errorf("no main entities at all must be a no-op (no fetch attempted), got: %v", err)
	}
}

func TestCheckKnownNotToExistErrorsOptimisticWhenUnresolved(t *testing.T) {
	definitionDir := t.TempDir()
	seedExistenceCache(t, definitionDir, "protocols-server-2", TEntityExistenceStatusPayload{
		"hass.davids_bedroom": {Entities: map[string]TEntityExistenceStatusEntry{
			// No entry at all for co2 -- the coordinator hasn't inquired about it yet.
		}},
	})

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.davids_bedroom": {
			DeviceID:  "hass.davids_bedroom",
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"co2": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.davids_bedroom_carbon_dioxide"}},
			},
		},
	}

	ctx := TPhysicalGenerationContext{MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}
	if err := checkKnownNotToExistErrors(definitionDir, hassBridgeDevicesByID, ctx); err != nil {
		t.Errorf("unresolved status must not block generation, got: %v", err)
	}
}

// TestCheckKnownNotToExistErrorsSkipsRoamingDevices is a regression test for a real incident found
// live 2026-09-05: Vienna's own hass.eriks_iphone declaration (a roaming device, ExportAs
// "roaming", Instances: ["main"] -- Vienna's OWN local "main" instance only) started failing
// generation the moment Vienna's coordinator began reliably publishing fresh existence status,
// because Vienna's own "main" instance genuinely has no local pairing for this capability -- even
// though the SAME roaming device is confirmed known-to-exist on Junglinster's own instances, which
// Vienna's generator has no visibility into at all (each house only ever loads its own
// Physical.def). A roaming device's existence can never be authoritatively judged from one house's
// own local instance(s) alone, so it must never hard-fail generation this way.
func TestCheckKnownNotToExistErrorsSkipsRoamingDevices(t *testing.T) {
	definitionDir := t.TempDir()
	seedExistenceCache(t, definitionDir, "main", TEntityExistenceStatusPayload{
		"hass.eriks_iphone": {Entities: map[string]TEntityExistenceStatusEntry{
			"sensor.eriks_iphone_15_pro_battery_level": {Status: "known-not-to-exist"},
		}},
	})

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.eriks_iphone": {
			DeviceID:  "hass.eriks_iphone",
			Instances: []string{"main"},
			ExportAs:  "roaming",
			Capabilities: map[string]THassBridgeCapability{
				"battery_level": {Domain: "sensor", Sources: map[string]string{"main": "sensor.eriks_iphone_15_pro_battery_level"}},
			},
		},
	}

	ctx := TPhysicalGenerationContext{MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}
	if err := checkKnownNotToExistErrors(definitionDir, hassBridgeDevicesByID, ctx); err != nil {
		t.Errorf("a roaming device's own known-not-to-exist status (from THIS house's local instances alone) must never block generation, got: %v", err)
	}
}

func TestCheckKnownNotToExistErrorsNoInstancesIsNoop(t *testing.T) {
	definitionDir := t.TempDir()
	ctx := TPhysicalGenerationContext{MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}
	if err := checkKnownNotToExistErrors(definitionDir, map[string]THassBridgeDevice{}, ctx); err != nil {
		t.Errorf("no hassbridge devices at all must be a no-op, got: %v", err)
	}
}
