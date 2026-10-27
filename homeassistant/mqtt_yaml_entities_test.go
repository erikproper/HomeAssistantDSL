package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"homeassistant/hamqtt"
)

// TestDiscoveredMQTTConfig: what Home Assistant's discovery takes from a payload -- abbreviations
// expanded, "~" substituted, fields the domain's schema doesn't know dropped (PROJECT.md 19c).
func TestDiscoveredMQTTConfig(t *testing.T) {
	sensor := map[string]interface{}{
		"~": "ems-esp", "stat_t": "~/boiler_data", "val_tpl": "{{ value_json.temp }}", "uniq_id": "boiler_temp",
		"dev":    map[string]interface{}{"ids": []interface{}{"ems-esp"}, "mf": "EMS-ESP"},
		"avty":   []interface{}{map[string]interface{}{"t": "~/status"}},
		"origin": map[string]interface{}{"name": "EMS-ESP"}, "object_id": "x", "unknown_field": 1,
	}
	got := hamqtt.DiscoveredConfig(sensor, "sensor")
	want := map[string]interface{}{
		"state_topic": "ems-esp/boiler_data", "value_template": "{{ value_json.temp }}", "unique_id": "boiler_temp",
		"device":       map[string]interface{}{"identifiers": []interface{}{"ems-esp"}, "manufacturer": "EMS-ESP"},
		"availability": []interface{}{map[string]interface{}{"topic": "ems-esp/status"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sensor:\n got %v\nwant %v", got, want)
	}

	binary := hamqtt.DiscoveredConfig(map[string]interface{}{"state_topic": "z2m/x", "state_value_template": "{{ value_json.s }}"}, "binary_sensor")
	if _, kept := binary["state_value_template"]; kept {
		t.Errorf("binary_sensor kept state_value_template, which its schema doesn't know")
	}
	light := hamqtt.DiscoveredConfig(map[string]interface{}{"schema": "json", "command_topic": "z2m/l/set", "state_value_template": "x", "brightness": true}, "light")
	if light["brightness"] != true || light["command_topic"] != "z2m/l/set" {
		t.Errorf("json light lost a field of its own schema: %v", light)
	}
}

// TestValidateYAMLMQTTEntities: a field the domain doesn't know is reported (the live case: a
// bridge button's state_topic), a valid entity isn't.
func TestValidateYAMLMQTTEntities(t *testing.T) {
	dir := t.TempDir()
	write := func(domain, name, content string) {
		path := filepath.Join(dir, "entities", "mqtt", domain, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("button", "button.ok.yaml", "- button:\n    unique_id: a\n    command_topic: x/press\n")
	write("button", "button.bad.yaml", "- button:\n    unique_id: b\n    command_topic: x/press\n    state_topic: x/state\n")
	problems := validateYAMLMQTTEntities(dir)
	if len(problems) != 1 || !strings.Contains(problems[0], "button.bad.yaml") || !strings.Contains(problems[0], "state_topic") {
		t.Errorf("problems = %q, want exactly button.bad.yaml's state_topic", problems)
	}
}

// TestRawConfigForSourceDomain: a leaf a gateway publishes under two raw domains (a Moes remote's
// "action": a "sensor" mirror and an "event" entity) is told apart by the declared source domain;
// without one, the entity's own domain decides, and an ambiguous leaf isn't guessed (moved from
// the coordinator's relay test, PROJECT.md 19).
func TestRawConfigForSourceDomain(t *testing.T) {
	configs := TDiscoveryRawConfigs{"buttons.moes": {
		"sensor/0x70_action_zigbee2mqtt": {"uniq_id": "0x70_action_zigbee2mqtt", "name": "sensor mirror"},
		"event/0x70_action_zigbee2mqtt":  {"uniq_id": "0x70_action_zigbee2mqtt", "name": "event"},
	}}
	link := TDiscoveryEntityLink{GatewayDeviceID: "buttons.moes", Leaf: "0x70_action_zigbee2mqtt", SourceDomain: "event"}
	if raw, domain, ok := rawConfigFor(configs, "event.pressed", link); !ok || domain != "event" || raw["name"] != "event" {
		t.Errorf("with source domain event: %v %q %v", raw, domain, ok)
	}
	link.SourceDomain = ""
	if _, domain, ok := rawConfigFor(configs, "sensor.action", link); !ok || domain != "sensor" {
		t.Errorf("without a source domain, the entity's own domain: %q %v", domain, ok)
	}
	if _, _, ok := rawConfigFor(configs, "binary_sensor.pressed", link); ok {
		t.Errorf("an ambiguous leaf under a third domain is guessed")
	}
}

// TestDroppedFieldsNote: counted per domain and field, nothing when nothing drops.
func TestDroppedFieldsNote(t *testing.T) {
	entities := []THostYAMLEntity{
		discoveredYAMLEntity("button", "button.a", map[string]interface{}{"command_topic": "a", "state_topic": "a"}),
		discoveredYAMLEntity("button", "button.b", map[string]interface{}{"command_topic": "b", "state_topic": "b"}),
		discoveredYAMLEntity("sensor", "sensor.c", map[string]interface{}{"state_topic": "c"}),
	}
	if got, want := droppedFieldsNote(entities), "; dropped, unknown to Home Assistant: button state_topic (2)"; got != want {
		t.Errorf("droppedFieldsNote = %q, want %q", got, want)
	}
	if got := droppedFieldsNote(entities[2:]); got != "" {
		t.Errorf("droppedFieldsNote(sensor) = %q, want empty", got)
	}
}
