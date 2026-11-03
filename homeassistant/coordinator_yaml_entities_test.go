package main

import "testing"

// TestCoordinatorYAMLEntityBodies: a reload and a restart button for every instance, the
// installation and "all", the "Discover entity" text and the "missing declared entities"
// indicator -- the coordinator's former discovery configs (PROJECT.md 19h.2) -- and per instance
// its integrations and updates, per container host its containers' updates (PROJECT.md 29); without an installation no buttons at all.
func TestCoordinatorYAMLEntityBodies(t *testing.T) {
	entities := coordinatorYAMLEntityBodies([]string{"main", "protocols-server-1"}, []string{"protocols-server-1"}, "vienna")
	byID := map[string]map[string]interface{}{}
	for _, entity := range entities {
		byID[entity.EntityID] = entity.Body
	}
	if len(entities) != 2*4+2+2*2+1 {
		t.Fatalf("%d entities, want 15 (2 actions x 4 scopes, discover, missing, 2 x integrations and updates, container updates)", len(entities))
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
	if integrations := byID["binary_sensor.infrastructural_integrations_protocols_server_1"]; integrations == nil || integrations["state_topic"] != "homeassistant_instances/protocols-server-1/integrations" || integrations["json_attributes_topic"] != "homeassistant_instances/protocols-server-1/integrations" || integrations["device_class"] != "problem" || integrations["expire_after"] != 900 {
		t.Errorf("integrations indicator for protocols-server-1 = %v", integrations)
	}
	if containers := byID["binary_sensor.infrastructural_container_updates_protocols_server_1"]; containers == nil || containers["state_topic"] != "hosts/protocols-server-1/containers/updates" || containers["device_class"] != "update" {
		t.Errorf("container updates for protocols-server-1 = %v", containers)
	}
	if without := coordinatorYAMLEntityBodies([]string{"main"}, nil, ""); len(without) != 4 {
		t.Errorf("without an installation: %d entities, want 4 (no buttons)", len(without))
	}
}
