package main

import "testing"

// TestCoordinatorYAMLEntityBodies: a reload and a restart button for every instance, the
// installation and "all", the "Discover entity" text and the "missing declared entities"
// indicator -- the coordinator's former discovery configs (PROJECT.md 19h.2); without an
// installation no buttons at all.
func TestCoordinatorYAMLEntityBodies(t *testing.T) {
	entities := coordinatorYAMLEntityBodies([]string{"main", "protocols-server-1"}, "vienna")
	byID := map[string]map[string]interface{}{}
	for _, entity := range entities {
		byID[entity.EntityID] = entity.Body
	}
	if len(entities) != 2*4+2 {
		t.Fatalf("%d entities, want 10 (2 actions x 4 scopes, discover, missing)", len(entities))
	}
	button := byID["button.infrastructural_restart_protocols_server_1"]
	if button == nil || button["unique_id"] != "meta_restart_protocols-server-1" || button["command_topic"] != "meta/restart/protocols-server-1/request" || button["payload_press"] != "PRESS" || button["icon"] != "mdi:restart" {
		t.Errorf("restart button for protocols-server-1 = %v", button)
	}
	if all := byID["button.infrastructural_reload_all"]; all == nil || all["command_topic"] != "meta/reload/all/request" {
		t.Errorf("reload-all button = %v", all)
	}
	if text := byID["text.infrastructural_discover_entity"]; text == nil || text["unique_id"] != "meta_discover_entity" || text["command_topic"] != "meta/discover_entity/set" || text["optimistic"] != true {
		t.Errorf("discover-entity text = %v", text)
	}
	if missing := byID["binary_sensor.missing_declared_entities"]; missing == nil || missing["state_topic"] != "coordinator/missing_declared_entities/state" || missing["json_attributes_topic"] != "coordinator/missing_declared_entities/attributes" || missing["payload_on"] != "true" {
		t.Errorf("missing-declared-entities indicator = %v", missing)
	}
	if without := coordinatorYAMLEntityBodies([]string{"main"}, ""); len(without) != 2 {
		t.Errorf("without an installation: %d entities, want 2 (no buttons)", len(without))
	}
}
