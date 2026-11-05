package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// namingTestAdmin is a device "sensors.aqara" whose node is positioned at
// infrastructural/apartment/hallway/door/aqara/node, with a battery level under it, a social
// humidity elsewhere, and an update at the device's own position.
func namingTestAdmin() *TAdministrationState {
	admin := newAdministrationState()
	records := []struct{ name, sphere, path, domain string }{
		{"binary_sensor.infrastructural/apartment/hallway/door/aqara/node", "infrastructural", "apartment/hallway/door/aqara/node", "binary_sensor"},
		{"sensor.infrastructural/apartment/hallway/door/aqara/battery_level", "infrastructural", "apartment/hallway/door/aqara/battery_level", "sensor"},
		{"sensor.social/apartment/hallway/humidity", "social", "apartment/hallway/humidity", "sensor"},
		{"update.infrastructural/apartment/hallway/door/aqara", "infrastructural", "apartment/hallway/door/aqara", "update"},
	}
	for _, r := range records {
		admin.EntityRecordsBySpace["social/apartment/hallway"] = append(admin.EntityRecordsBySpace["social/apartment/hallway"], TEntityRecord{
			Name: r.name, Identity: TEntityIdentity{Domain: r.domain, Sphere: r.sphere, Path: r.path},
		})
	}
	admin.DeviceNodeEntityID["sensors.aqara"] = "binary_sensor.infrastructural_apartment_hallway_door_aqara_node"
	return admin
}

func namingTestBody() map[string]interface{} {
	return map[string]interface{}{
		"unique_id": "uid",
		"name":      "full/path",
		"device":    map[string]interface{}{"identifiers": []interface{}{"zigbee2mqtt_0x1"}, "name": "apartment/hallway/door/aqara"},
	}
}

func TestNameEntityRelativeToMaterialDevice(t *testing.T) {
	admin := namingTestAdmin()
	for entityID, want := range map[string]string{
		"sensor.infrastructural_apartment_hallway_door_aqara_battery_level": "battery_level",
		"update.infrastructural_apartment_hallway_door_aqara":               "update",
	} {
		body := namingTestBody()
		if !nameEntity(admin, body, entityID, "sensors.aqara", nil, nil) {
			t.Fatalf("%s: not named", entityID)
		}
		if body["name"] != want {
			t.Errorf("%s: name %v, want %q", entityID, body["name"], want)
		}
		if got := body["device"].(map[string]interface{})["name"]; got != "material/apartment/hallway/door/aqara" {
			t.Errorf("%s: device name %v", entityID, got)
		}
	}
}

func TestNameEntitySocialBecomesSubDevice(t *testing.T) {
	body := namingTestBody()
	if !nameEntity(namingTestAdmin(), body, "sensor.social_apartment_hallway_humidity", "sensors.aqara", map[string]string{"sensor.social_apartment_hallway_humidity": "apartment/hallway", "binary_sensor.infrastructural_apartment_hallway_door_aqara_node": "apartment/hallway/door"}, nil) {
		t.Fatal("not named")
	}
	if body["name"] != nil {
		t.Errorf("name %v, want null", body["name"])
	}
	sub := body["device"].(map[string]interface{})
	if sub["name"] != "social/apartment/hallway/humidity" || sub["via_device"] != "zigbee2mqtt_0x1" || sub["suggested_area"] != "apartment/hallway" {
		t.Errorf("sub-device %v", sub)
	}
	if ids := sub["identifiers"].([]string); len(ids) != 1 || ids[0] != "uid" {
		t.Errorf("identifiers %v, want the entity's unique_id", ids)
	}
}

func TestNameEntityWithoutPositionedNodeKeepsName(t *testing.T) {
	body := namingTestBody()
	if nameEntity(namingTestAdmin(), body, "sensor.social_apartment_hallway_humidity", "sensors.other", nil, nil) {
		t.Fatal("named without a positioned node")
	}
	if body["name"] != "full/path" {
		t.Errorf("name changed to %v", body["name"])
	}
}

func TestYAMLNodeBodyReadsOwnAvailability(t *testing.T) {
	sibling := map[string]interface{}{
		"device": map[string]interface{}{"identifiers": []interface{}{"zigbee2mqtt_0x1"}},
		"availability": []interface{}{
			map[string]interface{}{"topic": "zigbee2mqtt/bridge/state", "value_template": "{{ value_json.state }}"},
			map[string]interface{}{"topic": "zigbee2mqtt/aqara/availability", "value_template": "{{ value_json.state }}"},
		},
	}
	shared := map[string]bool{"zigbee2mqtt/bridge/state": true}
	body, ok := yamlNodeBody(sibling, "binary_sensor.x_node", "uid", "", shared)
	if !ok {
		t.Fatal("no body")
	}
	if body["state_topic"] != "zigbee2mqtt/aqara/availability" {
		t.Errorf("state_topic %v", body["state_topic"])
	}
	if body["value_template"] != "{{ 'on' if (value_json.state) == 'online' else 'off' }}" {
		t.Errorf("value_template %v", body["value_template"])
	}
	rest := body["availability"].([]interface{})
	if len(rest) != 1 || rest[0].(map[string]interface{})["topic"] != "zigbee2mqtt/bridge/state" {
		t.Errorf("availability %v", rest)
	}
	if body["device_class"] != "connectivity" || body["payload_on"] != "on" || body["unique_id"] != "uid" {
		t.Errorf("body %v", body)
	}
}

func TestYAMLNodeBodyWithoutAvailability(t *testing.T) {
	if _, ok := yamlNodeBody(map[string]interface{}{}, "binary_sensor.x_node", "uid", "", nil); ok {
		t.Error("a body without any availability to read")
	}
}

func TestNameEntitySharedPositionNamedByDomain(t *testing.T) {
	admin := namingTestAdmin()
	shared := sharedPositions(admin, []string{"sensor.social_apartment_hallway_humidity", "sensor.social_apartment_hallway_humidity"})
	body := namingTestBody()
	if !nameEntity(admin, body, "sensor.social_apartment_hallway_humidity", "sensors.aqara", nil, shared) {
		t.Fatal("not named")
	}
	if body["name"] != "sensor" {
		t.Errorf("name %v, want its domain", body["name"])
	}
}

func TestDropFriendlyNamesKeepsIconsAndNatives(t *testing.T) {
	dir := t.TempDir()
	write := func(path, content string) {
		if err := writeYAMLFile(filepath.Join(dir, path), content); err != nil {
			t.Fatal(err)
		}
	}
	write("entities/mqtt/sensor/sensor.own.yaml", "- sensor: {}\n")
	write("entities/mqtt/sensor/sensor.own_icon.yaml", "- sensor: {}\n")
	write("customization/sensor/social/sensor.own.yaml", "sensor.own:\n  friendly_name: a/b\n")
	write("customization/sensor/social/sensor.own_icon.yaml", "sensor.own_icon:\n  icon: mdi:x\n  friendly_name: a/c\n")
	write("customization/sensor/social/sensor.native.yaml", "sensor.native:\n  friendly_name: a/d\n")
	dropped, err := dropFriendlyNames(dir)
	if err != nil || dropped != 2 {
		t.Fatalf("dropped %d, %v; want 2", dropped, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "customization/sensor/social/sensor.own.yaml")); !os.IsNotExist(err) {
		t.Error("an emptied file is still there")
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "customization/sensor/social/sensor.own_icon.yaml")); strings.Contains(string(data), "friendly_name") || !strings.Contains(string(data), "icon: mdi:x") {
		t.Errorf("icon file: %s", data)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "customization/sensor/social/sensor.native.yaml")); !strings.Contains(string(data), "friendly_name: a/d") {
		t.Errorf("a native entity lost its friendly_name: %s", data)
	}
}
