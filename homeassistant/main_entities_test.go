package main

import "testing"

// TestCollectMainEntityIDs confirms a bare entity (no definition, not discovery-implied) is
// collected and converted via toHomeAssistantEntityID, while an explicitly-defined entity and a
// discovery-implied one (both HasDefinitionOrImport-adjacent, but for different reasons -- see
// main_entities.go's own header comment) are both excluded.
func TestCollectMainEntityIDs(t *testing.T) {
	admin := newAdministrationState()
	admin.EntityRecordsBySpace["root"] = []TEntityRecord{
		{Name: "sensor.physical:door/aqara_multi/temperature", HasDefinitionOrImport: false, DiscoveryImplied: false},
		{Name: "sensor.social:total_power", HasDefinitionOrImport: true, DiscoveryImplied: false},
		{Name: "sensor.infrastructural:smarty/cpu/load", HasDefinitionOrImport: false, DiscoveryImplied: true},
	}

	got := collectMainEntityIDs(admin)
	if len(got) != 1 || got[0] != "sensor.physical_door_aqara_multi_temperature" {
		t.Errorf("collectMainEntityIDs = %v, want exactly [sensor.physical_door_aqara_multi_temperature]", got)
	}
}

// TestCollectMainEntityIDsDedupesAndSorts confirms the same bare entity declared under two spaces
// (shouldn't normally happen, but defensive) is deduplicated, and output is sorted for determinism.
func TestCollectMainEntityIDsDedupesAndSorts(t *testing.T) {
	admin := newAdministrationState()
	admin.EntityRecordsBySpace["a"] = []TEntityRecord{
		{Name: "sensor.social:zebra", HasDefinitionOrImport: false},
	}
	admin.EntityRecordsBySpace["b"] = []TEntityRecord{
		{Name: "sensor.social:aardvark", HasDefinitionOrImport: false},
		{Name: "sensor.social:zebra", HasDefinitionOrImport: false},
	}

	got := collectMainEntityIDs(admin)
	want := []string{"sensor.social_aardvark", "sensor.social_zebra"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("collectMainEntityIDs = %v, want %v", got, want)
	}
}

// TestCollectDiscoveryImpliedEntityIDs is the exact inverse selection of TestCollectMainEntityIDs --
// see collectDiscoveryImpliedEntityIDs' own doc comment for the real bug this exists to prevent
// (found live 2026-09-20: every discovery-declared device's own entities kept reappearing in
// suggestions/home_assistant_main.txt as unclaimed "hass.discovered_..." blocks).
func TestCollectDiscoveryImpliedEntityIDs(t *testing.T) {
	admin := newAdministrationState()
	admin.EntityRecordsBySpace["root"] = []TEntityRecord{
		{Name: "sensor.physical:door/aqara_multi/temperature", HasDefinitionOrImport: false, DiscoveryImplied: false},
		{Name: "sensor.social:total_power", HasDefinitionOrImport: true, DiscoveryImplied: false},
		{Name: "sensor.infrastructural:smarty/cpu/load", HasDefinitionOrImport: false, DiscoveryImplied: true},
	}

	got := collectDiscoveryImpliedEntityIDs(admin)
	if len(got) != 1 || got[0] != "sensor.infrastructural_smarty_cpu_load" {
		t.Errorf("collectDiscoveryImpliedEntityIDs = %v, want exactly [sensor.infrastructural_smarty_cpu_load]", got)
	}
}

// TestCollectDiscoveryImpliedEntityIDsDedupesAndSorts mirrors
// TestCollectMainEntityIDsDedupesAndSorts for the discovery-implied side.
func TestCollectDiscoveryImpliedEntityIDsDedupesAndSorts(t *testing.T) {
	admin := newAdministrationState()
	admin.EntityRecordsBySpace["a"] = []TEntityRecord{
		{Name: "sensor.social:zebra", DiscoveryImplied: true},
	}
	admin.EntityRecordsBySpace["b"] = []TEntityRecord{
		{Name: "sensor.social:aardvark", DiscoveryImplied: true},
		{Name: "sensor.social:zebra", DiscoveryImplied: true},
	}

	got := collectDiscoveryImpliedEntityIDs(admin)
	want := []string{"sensor.social_aardvark", "sensor.social_zebra"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("collectDiscoveryImpliedEntityIDs = %v, want %v", got, want)
	}
}
