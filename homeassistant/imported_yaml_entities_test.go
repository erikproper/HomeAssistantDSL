package main

import "testing"

// TestResolveDerivedImport: a plain capability is its own relayed topic with "value"; a chain of
// derived capabilities composes their templates over the ultimate atomic ancestor's topic; a
// dangling reference doesn't resolve (moved from the coordinator's own tests, PROJECT.md 19h.4).
func TestResolveDerivedImport(t *testing.T) {
	link := TDeviceConceptualLink{AttributeEntityIDs: map[string]TDeviceAttributeLink{
		"battery_level": {EntityID: "sensor.infrastructural_terrace_netatmo_battery_level"},
	}}
	device := TImportedDevice{Capabilities: map[string]TImportedCapability{
		"battery_level":         {},
		"battery_alert":         {Domain: "binary_sensor", DerivedFromCapability: "battery_level", DerivedViaTemplate: "( $ | int(0) < 20 )"},
		"battery_alert_delayed": {Domain: "binary_sensor", DerivedFromCapability: "battery_alert", DerivedViaTemplate: "not ($)"},
		"dangling":              {Domain: "binary_sensor", DerivedFromCapability: "missing", DerivedViaTemplate: "$"},
	}}
	baseTopic := "imports/sensor.infrastructural_terrace_netatmo_battery_level/state"
	if topic, template, ok := resolveDerivedImport(device, link, "battery_level", map[string]bool{}); !ok || topic != baseTopic || template != "value" {
		t.Errorf("atomic: %q %q %v", topic, template, ok)
	}
	if topic, template, ok := resolveDerivedImport(device, link, "battery_alert_delayed", map[string]bool{}); !ok || topic != baseTopic || template != "not ((( (value) | int(0) < 20 )))" {
		t.Errorf("chain: %q %q %v", topic, template, ok)
	}
	if _, _, ok := resolveDerivedImport(device, link, "dangling", map[string]bool{}); ok {
		t.Errorf("a dangling reference resolved")
	}
}
