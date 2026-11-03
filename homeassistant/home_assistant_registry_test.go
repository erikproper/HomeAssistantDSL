package main

import (
	"encoding/json"
	"testing"
)

func testManifest(t *testing.T, text string) tRegistryManifest {
	t.Helper()
	var m tRegistryManifest
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// TestPlanRegistryRepair: the port of ha_registry_sync.py's plan -- a rename of the generator's own
// entry, a removal of a YAML entry no longer generated, a stale MQTT entry removed only once it
// isn't live, a rename onto a name held by someone else skipped, and a native entity's first
// rename from its seed.
func TestPlanRegistryRepair(t *testing.T) {
	registry := []registryEntry{
		{EntityID: "sensor.old_name", UniqueID: "u1", Platform: "template"},
		{EntityID: "sensor.gone", UniqueID: "u2", Platform: "template"},
		{EntityID: "binary_sensor.stale_live", UniqueID: "m1", Platform: "mqtt"},
		{EntityID: "binary_sensor.stale_dead", UniqueID: "m2", Platform: "mqtt"},
		{EntityID: "light.want_taken", UniqueID: "u3", Platform: "template"},
		{EntityID: "light.taken", UniqueID: "zz", Platform: "hue", ConfigEntryID: "c1"},
		{EntityID: "media_player.office", UniqueID: "n1", Platform: "sonos", ConfigEntryID: "c2"},
	}
	manifest := testManifest(t, `{"yaml": [{"unique_id": "u1", "entity_id": "sensor.new_name"}, {"unique_id": "u3", "entity_id": "light.taken"}],
		"mqtt": [], "native": [{"source": "appliance.sonos!media_player", "entity_id": "media_player.social_office", "seed": "media_player.office"}]}`)
	previous := testManifest(t, `{"yaml": [{"unique_id": "u1", "entity_id": "sensor.old_name"}, {"unique_id": "u2", "entity_id": "sensor.gone"}],
		"mqtt": [{"unique_id": "m1", "entity_id": "binary_sensor.stale_live"}, {"unique_id": "m2", "entity_id": "binary_sensor.stale_dead"}]}`)
	live := map[string]bool{"binary_sensor.stale_live": true}

	renames, removals, reports := planRegistryRepair(registry, manifest, previous, live)
	got := map[string]string{}
	for _, r := range renames {
		got[r.Entry.EntityID] = r.Target
	}
	if len(got) != 2 || got["sensor.old_name"] != "sensor.new_name" || got["media_player.office"] != "media_player.social_office" {
		t.Errorf("renames = %v", got)
	}
	removed := map[string]bool{}
	for _, e := range removals {
		removed[e.EntityID] = true
	}
	if len(removed) != 2 || !removed["sensor.gone"] || !removed["binary_sensor.stale_dead"] {
		t.Errorf("removals = %v", removed)
	}
	if len(reports) != 1 {
		t.Errorf("reports = %v, want the held target only", reports)
	}
	pending := pendingRemovals(registry, manifest, previous, live)
	if len(pending) != 1 || pending[0].EntityID != "binary_sensor.stale_live" {
		t.Errorf("pending = %v", pending)
	}
}

// TestPlanRegistryRepairTradesNames: two entries trading names are both renamed (the targets are
// held by entries that themselves move).
func TestPlanRegistryRepairTradesNames(t *testing.T) {
	registry := []registryEntry{{EntityID: "sensor.a", UniqueID: "ua", Platform: "template"}, {EntityID: "sensor.b", UniqueID: "ub", Platform: "template"}}
	manifest := testManifest(t, `{"yaml": [{"unique_id": "ua", "entity_id": "sensor.b"}, {"unique_id": "ub", "entity_id": "sensor.a"}]}`)
	renames, _, reports := planRegistryRepair(registry, manifest, tRegistryManifest{}, nil)
	if len(renames) != 2 || len(reports) != 0 {
		t.Errorf("renames %v, reports %v", renames, reports)
	}
}
