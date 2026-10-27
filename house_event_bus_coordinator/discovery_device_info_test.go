package main

import (
	"reflect"
	"testing"
)

// TestDiscoveryDeviceInfoRecord: a gateway's metadata is taken from its device block, full or
// abbreviated keys alike; a block with identifiers only adds nothing (PROJECT.md 19b).
func TestDiscoveryDeviceInfoRecord(t *testing.T) {
	tracker := &TDiscoveryDeviceInfoTracker{byGateway: map[string]map[string]string{}}
	z2m := map[string]interface{}{"device": map[string]interface{}{"identifiers": []interface{}{"zigbee2mqtt_0x01"}, "manufacturer": "Develco", "model": "Motion sensor", "model_id": "MOSZB-140", "sw_version": "4.0.8", "name": "apartment/bedroom/frient"}}
	ems := map[string]interface{}{"dev": map[string]interface{}{"ids": []interface{}{"ems-esp"}, "mf": "EMS-ESP", "mdl": "EMS-ESP", "sw": "v3.8.4", "cu": "http://ems-esp"}}
	sub := map[string]interface{}{"dev": map[string]interface{}{"ids": []interface{}{"ems-esp-boiler"}}}
	if !tracker.Record("sensors.bedroom_frient", z2m) || !tracker.Record("discovery.ems_esp", ems) {
		t.Fatalf("expected both to record something")
	}
	if tracker.Record("discovery.ems_esp", sub) || tracker.Record("sensors.bedroom_frient", z2m) {
		t.Errorf("an identifiers-only block or an unchanged one must not count as a change")
	}
	want := map[string]map[string]string{
		"sensors.bedroom_frient": {"manufacturer": "Develco", "model": "Motion sensor", "model_id": "MOSZB-140", "sw_version": "4.0.8"},
		"discovery.ems_esp":      {"manufacturer": "EMS-ESP", "model": "EMS-ESP", "sw_version": "v3.8.4", "configuration_url": "http://ems-esp"},
	}
	if !reflect.DeepEqual(tracker.byGateway, want) {
		t.Errorf("byGateway = %v, want %v", tracker.byGateway, want)
	}
}
