package main

import "testing"

// TestParseHassBridgeCapabilityOneLinerPosition is the regression test for the concrete trigger
// (2026-09-25, cover.cover's own "derived value ...;" position declaration): a capability whose
// own "with: ... end;" block carries exactly ONE statement can be written as a single line,
// "<domain>.<path> <source> with <single-statement>;", instead of the full block form -- must
// parse identically either way.
func TestParseHassBridgeCapabilityOneLinerPosition(t *testing.T) {
	block := []string{
		"device utility.somfy_living_room_rear with:",
		"  cover.cover cover.living_room_rear with derived value cover.living_room_rear!current_position;",
		"end;",
	}
	devices, warnings := parseHassBridgeIntegrationBody(block, "ha2mqtt")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	cap, ok := devices[0].Capabilities["cover"]
	if !ok {
		t.Fatalf("capability %q not found in %v", "cover", devices[0].Capabilities)
	}
	if cap.Domain != "cover" || cap.Sources["ha2mqtt"] != "cover.living_room_rear" {
		t.Errorf("cap = %+v, want Domain=cover, Sources[ha2mqtt]=cover.living_room_rear", cap)
	}
	if cap.Position["ha2mqtt"] != "cover.living_room_rear!current_position" {
		t.Errorf(`Position["ha2mqtt"] = %q, want "cover.living_room_rear!current_position"`, cap.Position["ha2mqtt"])
	}
}

// TestParseHassBridgeCapabilityOneLinerIcon covers the OTHER real live shape (envoy's four
// "... with: unit: "kWh"; end;" blocks, washing_machine's per-sensor "icon: ...;" ones) --
// confirms the shorthand isn't special-cased to "derived value" alone.
func TestParseHassBridgeCapabilityOneLinerIcon(t *testing.T) {
	block := []string{
		"device utility.envoy with:",
		`  sensor.production/today/energy sensor.envoy_today_energy with unit: "kWh";`,
		"end;",
	}
	devices, warnings := parseHassBridgeIntegrationBody(block, "main")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	cap, ok := devices[0].Capabilities["production/today/energy"]
	if !ok {
		t.Fatalf("capability not found in %v", devices[0].Capabilities)
	}
	if cap.Unit != "kWh" {
		t.Errorf("Unit = %q, want kWh", cap.Unit)
	}
}

// TestParseHassBridgeCapabilityOneLinerAndBlockFormCoexist confirms both shapes can appear in the
// same device, and that a MULTI-statement block (e.g. vacuum's map+icon+2 attributes) is left as
// the block form -- there is no one-liner shape for more than one statement.
func TestParseHassBridgeCapabilityOneLinerAndBlockFormCoexist(t *testing.T) {
	block := []string{
		"device appliance.vacuum with:",
		"  vacuum.core vacuum.roomba with:",
		`    map: "home" "docked";`,
		`    icon: "mdi:robot-vacuum";`,
		"  end;",
		`  binary_sensor.charging binary_sensor.roomba_charging with icon: "mdi:battery-charging";`,
		"end;",
	}
	devices, warnings := parseHassBridgeIntegrationBody(block, "main")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	vacuumCap := devices[0].Capabilities["core"]
	if vacuumCap.ValueMap["home"] != "docked" || vacuumCap.Icon != "mdi:robot-vacuum" {
		t.Errorf("vacuum.core capability = %+v", vacuumCap)
	}
	chargingCap := devices[0].Capabilities["charging"]
	if chargingCap.Icon != "mdi:battery-charging" {
		t.Errorf("charging capability = %+v, want icon mdi:battery-charging", chargingCap)
	}
}

// TestParseHassBridgeCapabilityOneLinerWarnsOnUnrecognisedStatement confirms a one-liner whose
// single statement isn't one of the recognised "with:" body-line forms warns, rather than silently
// dropping the modifier or corrupting the capability's own Source (the exact failure mode the
// earlier domain-qualified "position:" parsing bug this session had -- capabilityPattern's own
// free-text group swallowing the whole line).
func TestParseHassBridgeCapabilityOneLinerWarnsOnUnrecognisedStatement(t *testing.T) {
	block := []string{
		"device utility.thing with:",
		"  cover.cover cover.living_room_rear with nonsense keyword here;",
		"end;",
	}
	devices, warnings := parseHassBridgeIntegrationBody(block, "ha2mqtt")
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
	if _, ok := devices[0].Capabilities["cover"]; ok {
		t.Errorf("capability %q should not have been registered from a rejected one-liner", "cover")
	}
}
