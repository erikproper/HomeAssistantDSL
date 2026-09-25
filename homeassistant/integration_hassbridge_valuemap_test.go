package main

import (
	"strings"
	"testing"
)

// TestParseHassBridgeValueMapBothForms confirms Physical.def's "map: "<from>" "<to>";" grammar
// parses correctly in both shapes it's allowed: nested inside a capability's own "with: ...
// end;" block, and as a standalone "<path> map: ...;" trailing line -- real motivating case
// (2026-09-09): a Roomba's native vocabulary ("home", "run") translated to HA's own MQTT vacuum
// activity strings ("docked", "cleaning").
func TestParseHassBridgeValueMapBothForms(t *testing.T) {
	body := []string{
		"device appliance.vacuum with:",
		`  vacuum.roomba vacuum.roomba with:`,
		`    map: "home" "docked";`,
		`    map: "run" "cleaning";`,
		"  end;",
		`  roomba map: "stop" "idle";`,
		"end;",
	}

	devices, warnings := parseHassBridgeIntegrationBody(body, "protocols-server-2")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}

	cap, ok := devices[0].Capabilities["roomba"]
	if !ok {
		t.Fatalf("capability %q not found in %v", "roomba", devices[0].Capabilities)
	}
	want := map[string]string{"home": "docked", "run": "cleaning", "stop": "idle"}
	if len(cap.ValueMap) != len(want) {
		t.Fatalf("ValueMap = %v, want %v", cap.ValueMap, want)
	}
	for from, to := range want {
		if cap.ValueMap[from] != to {
			t.Errorf("ValueMap[%q] = %q, want %q", from, cap.ValueMap[from], to)
		}
	}
}

// TestParseHassBridgeCapabilityRejectsOldColonForm is the regression test for the 2026-09-19
// grammar restriction: capabilityPattern/capabilityWithPattern briefly accepted an OPTIONAL ":"
// between <path> and <source> as a trial (both houses' Physical.def/Logical.def were then
// rewritten to the colon-less form and verified byte-identical on regenerate) -- now that the
// migration is complete, the old "sensor.status: sensor.laserjet_status;" colon form must be
// REJECTED outright, not silently tolerated.
func TestParseHassBridgeCapabilityRejectsOldColonForm(t *testing.T) {
	body := []string{
		"device appliance.vacuum with:",
		`  sensor.status: sensor.laserjet_status;`,
		"end;",
	}

	devices, warnings := parseHassBridgeIntegrationBody(body, "protocols-server-2")
	if len(warnings) != 1 || !strings.Contains(warnings[0], "unrecognised line") {
		t.Fatalf("expected exactly one \"unrecognised line\" warning for the old colon form, got: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	if _, ok := devices[0].Capabilities["status"]; ok {
		t.Errorf("Capabilities = %+v, want the old colon-form line rejected, not silently parsed", devices[0].Capabilities)
	}
}

// TestParseHassBridgeIntegrationBodyIgnoreOtherCapabilities is the regression test for a real bug
// found live 2026-09-21 (Vienna): the FRITZ!Box's own "utility.fritz_box" hassbridge device kept
// getting an auto-generated diagnostic "firmware_update" button entity suggested in
// suggestions/home_assistant_main.txt (kind-3) despite an existing "fritz_os" capability already
// covering essentially the same functionality -- known noise, not a genuine future capability
// worth positioning. "ignore other capabilities;" opts a hassbridge device out of that suggestion
// entirely (see THassBridgeDevice.IgnoreOtherCapabilities' own doc comment).
func TestParseHassBridgeIntegrationBodyIgnoreOtherCapabilities(t *testing.T) {
	body := []string{
		"device utility.fritz_box with:",
		"  ignore other capabilities;",
		"  button.fritz_os button.fritz_box_7590_ax_firmware_update;",
		"end;",
	}

	devices, warnings := parseHassBridgeIntegrationBody(body, "vienna")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 || !devices[0].IgnoreOtherCapabilities {
		t.Errorf("expected IgnoreOtherCapabilities to be set, got %+v", devices)
	}
	if _, ok := devices[0].Capabilities["fritz_os"]; !ok {
		t.Errorf("expected the ordinary capability line right after the directive to still parse, got %+v", devices[0].Capabilities)
	}
}
