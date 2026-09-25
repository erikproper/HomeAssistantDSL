package main

import "testing"

func TestFindBatteryLevelWithoutAlertDevices(t *testing.T) {
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.compliant": {Capabilities: map[string]THassBridgeCapability{
			"battery_level": {}, "battery_alert": {},
		}},
		"hass.violator": {Capabilities: map[string]THassBridgeCapability{
			"battery_level": {},
		}},
		"hass.no_battery": {Capabilities: map[string]THassBridgeCapability{
			"co2": {},
		}},
	}
	importedDevicesByID := map[string]TImportedDevice{
		"import.compliant": {Capabilities: map[string]TImportedCapability{
			"battery_level": {}, "battery_alert": {},
		}},
		"import.violator": {Capabilities: map[string]TImportedCapability{
			"battery_level": {},
		}},
	}

	got := findBatteryLevelWithoutAlertDevices(hassBridgeDevicesByID, importedDevicesByID)
	want := []string{"hass.violator", "import.violator"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d] = %q, want %q (got %v)", i, got[i], w, got)
		}
	}
}

// TestFindBatteryAlertMigrationCandidatesMatchesRealNamingConvention is a regression test for a
// real bug found live 2026-09-10: the first implementation matched against
// TEntityRecord.ConditionSources, which is never populated for a "/battery_alert"-suffixed record
// at all (expander.go explicitly skips the generic condition scan for those, scanning only
// BatteryAlertLevel). The real correlation is a pure naming convention -- confirmed against
// generateTemplateBinarySensors' own derivation (generator.go) -- reproduced here.
func TestFindBatteryAlertMigrationCandidatesMatchesRealNamingConvention(t *testing.T) {
	importedDevicesByID := map[string]TImportedDevice{
		"import.vienna_bedroom": {Capabilities: map[string]TImportedCapability{
			"battery_level": {}, "co2": {},
		}},
	}
	admin := newAdministrationState()
	admin.DeviceConceptualLinks["import.vienna_bedroom"] = TDeviceConceptualLink{
		AttributeEntityIDs: map[string]TDeviceAttributeLink{
			"battery_level": {EntityID: "sensor.infrastructural_apartment_bedroom_netatmo_battery_level"},
		},
	}
	admin.EntityRecordsBySpace["apartment"] = []TEntityRecord{
		{
			Name:              "binary_sensor.infrastructural/apartment/bedroom/netatmo/battery_alert",
			Identity:          TEntityIdentity{Domain: "binary_sensor", Sphere: "infrastructural", Path: "apartment/bedroom/netatmo/battery_alert"},
			BatteryAlertLevel: 20,
			Provenance:        "Spaces.def:203 → battery_alert :netatmo",
		},
	}

	hassBridgeDevicesByID := map[string]THassBridgeDevice{}
	got := findBatteryAlertMigrationCandidates(admin, hassBridgeDevicesByID, importedDevicesByID)
	if len(got) != 1 {
		t.Fatalf("got %d candidates, want 1: %+v", len(got), got)
	}
	c := got[0]
	if c.DeviceID != "import.vienna_bedroom" {
		t.Errorf("DeviceID = %q, want import.vienna_bedroom", c.DeviceID)
	}
	if c.BatteryLevelEntityID != "sensor.infrastructural_apartment_bedroom_netatmo_battery_level" {
		t.Errorf("BatteryLevelEntityID = %q, want the device's own resolved battery_level entity", c.BatteryLevelEntityID)
	}
	if c.AlertLevel != 20 {
		t.Errorf("AlertLevel = %d, want 20", c.AlertLevel)
	}
	if c.Provenance != "Spaces.def:203 → battery_alert :netatmo" {
		t.Errorf("Provenance = %q, want the macro's own call-chain string", c.Provenance)
	}
}

// TestFindBatteryAlertMigrationCandidatesExcludesBareEntities confirms a battery_alert whose
// naming-convention-derived battery_level entity does NOT match any Physical.def device's own
// resolved battery_level (e.g. a raw Zigbee2MQTT passthrough entity with no Physical.def device at
// all) is correctly excluded -- exactly the ~30 non-candidate Zigbee macro calls found live in
// Vienna's real Spaces.def alongside the 4 real ones.
func TestFindBatteryAlertMigrationCandidatesExcludesBareEntities(t *testing.T) {
	admin := newAdministrationState()
	admin.EntityRecordsBySpace["apartment"] = []TEntityRecord{
		{
			Name:              "binary_sensor.infrastructural/apartment/living_room/aqara_windoor/battery_alert",
			Identity:          TEntityIdentity{Domain: "binary_sensor", Sphere: "infrastructural", Path: "apartment/living_room/aqara_windoor/battery_alert"},
			BatteryAlertLevel: 10,
			Provenance:        "Spaces.def:30 → battery_level_device :aqara_windoor",
		},
	}
	got := findBatteryAlertMigrationCandidates(admin, map[string]THassBridgeDevice{}, map[string]TImportedDevice{})
	if len(got) != 0 {
		t.Errorf("got %v, want no candidates -- this battery_alert has no matching Physical.def device", got)
	}
}

func TestRecommendedDerivedDeclaration(t *testing.T) {
	got := recommendedDerivedDeclaration(TBatteryAlertMigrationCandidate{AlertLevel: 20})
	want := `derived binary_sensor.battery_alert from sensor.battery_level via "'on' if (($ | int(0)) < 20) else 'off'";`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
