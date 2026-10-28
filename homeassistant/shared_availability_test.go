package main

import (
	"reflect"
	"testing"
)

// TestSharedAvailabilityTopics: a topic more than one device's configs carry is shared (the
// gateway's); one only a single device carries is its own -- also through "~" and abbreviations.
func TestSharedAvailabilityTopics(t *testing.T) {
	configs := TDiscoveryRawConfigs{
		"buttons.tap": {
			"event/tap_action": {"~": "zigbee2mqtt/garage/door/tap", "uniq_id": "tap_action",
				"avty": []interface{}{map[string]interface{}{"t": "zigbee2mqtt/bridge/state"}, map[string]interface{}{"t": "~/availability"}},
				"dev":  map[string]interface{}{"ids": []interface{}{"zigbee2mqtt_0x01"}}},
		},
		"sensors.plug": {
			"sensor/plug_power": {"unique_id": "plug_power",
				"availability": []interface{}{map[string]interface{}{"topic": "zigbee2mqtt/bridge/state"}, map[string]interface{}{"topic": "zigbee2mqtt/plug/availability"}},
				"device":       map[string]interface{}{"identifiers": []interface{}{"zigbee2mqtt_0x02"}}},
		},
	}
	got := sharedAvailabilityTopics(configs)
	if want := map[string]bool{"zigbee2mqtt/bridge/state": true}; !reflect.DeepEqual(got, want) {
		t.Errorf("shared = %v, want %v", got, want)
	}
}

// TestKeepSharedAvailability: own entries go, shared ones stay; nothing shared left: unchanged.
func TestKeepSharedAvailability(t *testing.T) {
	shared := map[string]bool{"zigbee2mqtt/bridge/state": true}
	body := map[string]interface{}{"availability": []interface{}{
		map[string]interface{}{"topic": "zigbee2mqtt/bridge/state"},
		map[string]interface{}{"topic": "zigbee2mqtt/garage/door/tap/availability"},
	}}
	dropped, kept := keepSharedAvailability(body, shared)
	if !kept || !reflect.DeepEqual(dropped, []string{"zigbee2mqtt/garage/door/tap/availability"}) || len(body["availability"].([]interface{})) != 1 {
		t.Errorf("dropped %v, kept %v, body %v", dropped, kept, body)
	}
	lonely := map[string]interface{}{"availability": []interface{}{map[string]interface{}{"topic": "x/availability"}}}
	if _, kept := keepSharedAvailability(lonely, shared); kept || len(lonely["availability"].([]interface{})) != 1 {
		t.Errorf("nothing shared: want unchanged and reported, got %v", lonely)
	}
}
