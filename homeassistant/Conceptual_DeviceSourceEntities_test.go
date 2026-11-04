package main

import (
	"strings"
	"testing"
)

// TestDeviceCapabilityEntityResolvesTypingDefaults is the regression test for a real bug caught
// live 2026-08-29: registerDeviceSourceEntityLink (reached here via registerDeviceCapabilityEntityLink,
// the "entity ... from <device-id> <capability>;" construct, including its "for <device-id>:
// ...;"/merged-block shorthand) never resolved device_class/unit/state_class/icon at all -- every
// capability positioned this way silently lost its typing in HA, even though the sibling
// registerHassBridgeAttributeEntity path always has.
func TestDeviceCapabilityEntityResolvesTypingDefaults(t *testing.T) {
	const miniDSL = `device hass.davids_bedroom with:
  entity sensor.physical:netatmo/co2 from sensor.co2;
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.davids_bedroom": {DeviceID: "hass.davids_bedroom", Instance: "protocols-server-2", Capabilities: map[string]THassBridgeCapability{
			"co2": {Domain: "sensor", Source: "sensor.davids_bedroom_carbon_dioxide"},
		}},
	}
	capabilityDefaults := []TCapabilityDefaultRule{
		{Domain: "sensor", PatternPath: "co2", DeviceClass: "carbon_dioxide", Unit: "ppm", StateClass: "measurement"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", nil, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil, capabilityDefaults)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	link, ok := result.Administration.DeviceConceptualLinks["hass.davids_bedroom"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[hass.davids_bedroom] to be populated")
	}
	attr, ok := link.AttributeEntityIDs["co2"]
	if !ok {
		t.Fatalf("expected AttributeEntityIDs[co2] to be populated, got %v", link.AttributeEntityIDs)
	}
	if attr.DeviceClass != "carbon_dioxide" || attr.Unit != "ppm" || attr.StateClass != "measurement" {
		t.Errorf("AttributeEntityIDs[co2] = %+v, want DeviceClass \"carbon_dioxide\", Unit \"ppm\", StateClass \"measurement\" resolved from capabilityDefaults", attr)
	}

	// device.Capabilities (what generateHassBridgeFile/generateHassBridgeFile's writer actually
	// reads) must carry the same resolved typing, not just the in-memory link.
	cap := hassBridgeDevicesByID["hass.davids_bedroom"].Capabilities["co2"]
	if cap.DeviceClass != "carbon_dioxide" || cap.Unit != "ppm" || cap.StateClass != "measurement" {
		t.Errorf("Capabilities[co2] = %+v, want the same resolved typing", cap)
	}
}

// TestDeviceCapabilityEntityPreservesValueMapAndDerivedFields is a regression test for a real bug
// found live 2026-09-10 while wiring the "derived" capability mechanism
// (plans/derived-capability-mechanism.md): registerDeviceSourceEntityLink used to rebuild
// device.Capabilities[key] from a fresh THassBridgeCapability{} literal, silently dropping
// ValueMap (and, once added, DerivedFromCapability/DerivedViaTemplate) every time an already-
// declared Physical.def capability was referenced via "entity <spec> from <device-id>
// <capability>;" -- the ordinary way Spaces.def positions a capability. Confirmed this doesn't
// just affect the new "derived" fields: a device already using "map:" (ValueMap) would have hit
// this silently too, had any real Spaces.def happened to position it this way.
func TestDeviceCapabilityEntityPreservesValueMapAndDerivedFields(t *testing.T) {
	const miniDSL = `device hass.roomba with:
  entity sensor.physical:vacuum/status from sensor.status;
  entity binary_sensor.physical:vacuum/battery_alert from binary_sensor.battery_alert;
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.roomba": {DeviceID: "hass.roomba", Instance: "protocols-server-2", Capabilities: map[string]THassBridgeCapability{
			"status":        {Domain: "sensor", Source: "sensor.roomba_status", ValueMap: map[string]string{"home": "docked"}},
			"battery_level": {Domain: "sensor", Source: "sensor.roomba_battery_level"},
			"battery_alert": {Domain: "binary_sensor", DerivedFromCapability: "battery_level", DerivedViaTemplate: "( $ | int(0) < 20 )"},
		}},
	}

	var report strings.Builder
	_, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", nil, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	status := hassBridgeDevicesByID["hass.roomba"].Capabilities["status"]
	if status.ValueMap["home"] != "docked" {
		t.Errorf("Capabilities[status].ValueMap = %v, want \"home\"->\"docked\" preserved", status.ValueMap)
	}

	alert := hassBridgeDevicesByID["hass.roomba"].Capabilities["battery_alert"]
	if alert.DerivedFromCapability != "battery_level" || alert.DerivedViaTemplate != "( $ | int(0) < 20 )" {
		t.Errorf("Capabilities[battery_alert] = %+v, want DerivedFromCapability/DerivedViaTemplate preserved", alert)
	}
}

// TestDeviceCapabilityEntityExplicitTypingWinsOverDefaults confirms an explicit device_class/unit/
// state_class/icon Physical.def already declared directly on the capability (before this
// construct runs) is preserved, never overwritten by a conflicting capabilityDefaults rule -- same
// precedence registerHassBridgeAttributeEntity's own path already guarantees.
func TestDeviceCapabilityEntityExplicitTypingWinsOverDefaults(t *testing.T) {
	const miniDSL = `device hass.davids_bedroom with:
  entity sensor.physical:netatmo/co2 from sensor.co2;
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.davids_bedroom": {DeviceID: "hass.davids_bedroom", Instance: "protocols-server-2", Capabilities: map[string]THassBridgeCapability{
			"co2": {Domain: "sensor", Source: "sensor.davids_bedroom_carbon_dioxide", DeviceClass: "explicit_class", Unit: "explicit_unit"},
		}},
	}
	capabilityDefaults := []TCapabilityDefaultRule{
		{Domain: "sensor", PatternPath: "co2", DeviceClass: "carbon_dioxide", Unit: "ppm", StateClass: "measurement"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", nil, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil, capabilityDefaults)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	attr := result.Administration.DeviceConceptualLinks["hass.davids_bedroom"].AttributeEntityIDs["co2"]
	if attr.DeviceClass != "explicit_class" || attr.Unit != "explicit_unit" {
		t.Errorf("AttributeEntityIDs[co2] = %+v, want the explicit device_class/unit preserved, not overwritten by capabilityDefaults", attr)
	}
	// StateClass had no explicit value -- the default rule should still gap-fill it.
	if attr.StateClass != "measurement" {
		t.Errorf("AttributeEntityIDs[co2].StateClass = %q, want the default-resolved \"measurement\" (only unset fields should be gap-filled)", attr.StateClass)
	}
}

func TestDeviceSourceEntityWarnsWithoutPriorDevicePositioning(t *testing.T) {
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.laserjet": {DeviceID: "hass.laserjet", Instance: "protocols-server-2", Capabilities: map[string]THassBridgeCapability{}},
	}

	// finalAttempt=true: this device is never positioned, so it should warn on the final check.
	admin := newAdministrationState()
	decl := TDeviceSourceEntityDeclaration{LocalSpec: "binary_sensor.infrastructural:laserjet/jammed", DeviceID: "hass.laserjet"}
	warnings, deferred := registerDeviceSourceEntityLink(admin, decl, hassBridgeDevicesByID, nil, "Spaces.def", 42, true, "jammed", "")
	if len(warnings) != 1 || !strings.Contains(warnings[0], "no \"device.<spec> from hass.laserjet with: ...;\" positioning yet") {
		t.Errorf("warnings = %v, want a single warning about missing device positioning", warnings)
	}
	if !deferred {
		t.Errorf("deferred = false, want true for a device that was never positioned")
	}
}

func TestDeviceSourceEntityDefersRatherThanWarnsOnFirstAttempt(t *testing.T) {
	// finalAttempt=false: the device isn't positioned yet, but might still be positioned later in
	// the same file -- must come back empty-handed and deferred, not a premature warning.
	admin := newAdministrationState()
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.laserjet": {DeviceID: "hass.laserjet", Instance: "protocols-server-2", Capabilities: map[string]THassBridgeCapability{}},
	}
	decl := TDeviceSourceEntityDeclaration{LocalSpec: "binary_sensor.infrastructural:laserjet/jammed", DeviceID: "hass.laserjet"}
	warnings, deferred := registerDeviceSourceEntityLink(admin, decl, hassBridgeDevicesByID, nil, "Spaces.def", 42, false, "jammed", "")
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none on a non-final attempt", warnings)
	}
	if !deferred {
		t.Errorf("deferred = false, want true when the device isn't positioned yet")
	}
}

func TestDeviceSourceEntityWarnsOnUnknownDevice(t *testing.T) {
	admin := newAdministrationState()
	decl := TDeviceSourceEntityDeclaration{LocalSpec: "binary_sensor.infrastructural:laserjet/jammed", DeviceID: "hass.unknown"}
	warnings, deferred := registerDeviceSourceEntityLink(admin, decl, map[string]THassBridgeDevice{}, nil, "Spaces.def", 42, false, "jammed", "")
	if len(warnings) != 1 || !strings.Contains(warnings[0], `device "hass.unknown" not found`) {
		t.Errorf("warnings = %v, want a single \"device not found\" warning", warnings)
	}
	if deferred {
		t.Errorf("deferred = true, want false -- an unknown device can never resolve later, so this must be final immediately")
	}
}
