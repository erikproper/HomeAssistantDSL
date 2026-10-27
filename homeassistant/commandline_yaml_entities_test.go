package main

import "testing"

// TestCommandlineYAMLEntityBodies: the daemon's liveness binary_sensor and one entity per
// capability, as the coordinator's discovery published them (moved from its commandline tests,
// PROJECT.md 19): the node's unique_id doesn't collide with a hosts device's "<id>_node", the
// device identity is shared with a hosts declaration of the same id, a switch keeps the 0/1
// convention, a sensor has no command topic, every entity ANDs the daemon's node into its
// availability, and positioned names win over the bare host.
func TestCommandlineYAMLEntityBodies(t *testing.T) {
	device := TCommandlineDevice{DeviceID: "host.frame", Host: "frame", Capabilities: map[string]TCommandlineCapability{
		"slideshow": {Kind: "switch"}, "reboot": {Kind: "button"}, "uptime": {Kind: "sensor"},
	}}
	admin := &TAdministrationState{DeviceConceptualLinks: map[string]TDeviceConceptualLink{}}
	byID := map[string]THostYAMLEntity{}
	for _, entity := range commandlineYAMLEntityBodies(device, admin) {
		byID[entity.EntityID] = entity
	}
	if len(byID) != 4 {
		t.Fatalf("%d entities, want 4 (node, switch, button, sensor): %v", len(byID), byID)
	}
	node := byID["binary_sensor.frame_commandline_node"].Body
	if node["unique_id"] != "host.frame_commandline_node" || node["state_topic"] != "commandline/frame/node/state" || node["payload_on"] != "true" || node["device_class"] != "connectivity" || node["name"] != "frame/commandline" {
		t.Errorf("node = %v", node)
	}
	if identifiers := node["device"].(map[string]interface{})["identifiers"].([]string); len(identifiers) != 1 || identifiers[0] != "host.frame" {
		t.Errorf("device identifiers = %v, want exactly [host.frame], shared with a hosts declaration", identifiers)
	}
	sw := byID["switch.frame_slideshow"].Body
	if sw["command_topic"] != "commandline/frame/slideshow/set" || sw["payload_on"] != "1" || sw["state_off"] != "0" || sw["availability"].([]map[string]interface{})[0]["topic"] != "commandline/frame/node/state" {
		t.Errorf("switch = %v", sw)
	}
	if button := byID["button.frame_reboot"].Body; button["command_topic"] != "commandline/frame/reboot/press" || button["payload_press"] != "PRESS" {
		t.Errorf("button = %v", button)
	}
	if sensor := byID["sensor.frame_uptime"].Body; sensor["state_topic"] != "commandline/frame/uptime/state" || sensor["command_topic"] != nil {
		t.Errorf("sensor = %v", sensor)
	}

	// Positioned: the device's display name, and a capability's own entity_id and name.
	admin.DeviceConceptualLinks["host.frame"] = TDeviceConceptualLink{
		DisplayName: "apartment/living_room/picture_frame",
		AttributeEntityIDs: map[string]TDeviceAttributeLink{"slideshow": {
			EntityID: "switch.social_apartment_living_room_picture_frame",
			Identity: TEntityIdentity{Sphere: "social", Path: "apartment/living_room/picture_frame"},
		}},
	}
	byID = map[string]THostYAMLEntity{}
	for _, entity := range commandlineYAMLEntityBodies(device, admin) {
		byID[entity.EntityID] = entity
	}
	if node := byID["binary_sensor.frame_commandline_node"].Body; node["name"] != "apartment/living_room/picture_frame/commandline" {
		t.Errorf("positioned node name = %v", node["name"])
	}
	if sw := byID["switch.social_apartment_living_room_picture_frame"].Body; sw == nil || sw["name"] != "social/apartment/living_room/picture_frame" {
		t.Errorf("positioned switch = %v", sw)
	}
}
