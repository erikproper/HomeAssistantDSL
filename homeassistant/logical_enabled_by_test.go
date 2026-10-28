package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestApplyEnabledByImpliesNode covers PROJECT.md 17k's two Junglinster shapes: a server built from
// its cpu host (no Physical.def node of its own), and a TV with a node of its own plus a pinged
// host and a delay. Each implied node must equal the condition both used to spell out by hand.
func TestApplyEnabledByImpliesNode(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device server.xanadu with:",
		"  enabled by switch.switch from switch.xanadu as binary_sensor.frozen_xanadu_node;",
		"  colocated switch.xanadu as switch;",
		"  absorb from node.xanadu:",
		"    capability sensor.cpu/load;",
		"  end;",
		"end;",
		"device appliance.samsungtv with:",
		"  enabled by switch.switch from switch.cycling with delay_off 00:01:00;",
		"  dependency on node.samsungtv;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected parse warnings: %v", warnings)
	}
	logicalByID := map[string]TLogicalDevice{}
	for _, device := range devices {
		logicalByID[device.DeviceID] = device
	}
	localDevicesByID := map[string]TLocalDevice{
		"appliance.samsungtv": {DeviceID: "appliance.samsungtv", Capabilities: map[string]TLocalCapability{"node": {Domain: "binary_sensor"}}},
	}
	if warnings := applyEnabledBy(logicalByID, localDevicesByID, nil, nil); len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	for _, tc := range []struct {
		deviceID, handWritten, delayOff, assignedID string
	}{
		{"server.xanadu", `node.xanadu or jinja "$[switch.enabler] == 'off'"`, "", "binary_sensor.frozen_xanadu_node"},
		{"appliance.samsungtv", `(physical appliance.samsungtv and node.samsungtv) or jinja "$[switch.enabler] == 'off'"`, "00:01:00", ""},
	} {
		device := logicalByID[tc.deviceID]
		want, _ := parseConditionExpr(tc.handWritten, nil, "test")
		node := device.Capabilities["node"]
		if !reflect.DeepEqual(node.DerivedCondition, want) {
			t.Errorf("%s: implied node condition differs from the hand-written %q", tc.deviceID, tc.handWritten)
		}
		if node.DelayOff != tc.delayOff || node.AssignedID != tc.assignedID {
			t.Errorf("%s: node delay_off %q, as %q; want %q, %q", tc.deviceID, node.DelayOff, node.AssignedID, tc.delayOff, tc.assignedID)
		}
		if enabler := device.Capabilities["enabler"]; enabler.Domain != "switch" || enabler.AbsorbedFromCapability != "switch" || enabler.AbsorbedFromDeviceID == "" {
			t.Errorf("%s: enabler capability = %+v, want the switch absorbed from its enabling device", tc.deviceID, enabler)
		}
	}
}

// TestApplyEnabledByKeepsDeclaredNode: a device declaring its own node keeps it, with "or the
// enabler is off" added rather than a rebuilt base (the item 11a lesson).
func TestApplyEnabledByKeepsDeclaredNode(t *testing.T) {
	devices, _ := parseLogicalLayerBody([]string{
		"device appliance.tv with:",
		"  enabled by switch.switch from switch.tv;",
		"  derived capability binary_sensor.node with condition physical appliance.tv and node.tv and node.receiver;",
		"end;",
	}, nil)
	logicalByID := map[string]TLogicalDevice{devices[0].DeviceID: devices[0]}
	if warnings := applyEnabledBy(logicalByID, nil, nil, nil); len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	want, _ := parseConditionExpr(`(physical appliance.tv and node.tv and node.receiver) or jinja "$[switch.enabler] == 'off'"`, nil, "test")
	if !reflect.DeepEqual(logicalByID["appliance.tv"].Capabilities["node"].DerivedCondition, want) {
		t.Errorf("declared node not kept with \"or the enabler is off\" added")
	}
}

// TestWriteFrozenUniqueIDsOnEnabledByLine: an implied node's frozen unique_id is written onto its
// device's "enabled by" line, before a "with delay_off".
func TestWriteFrozenUniqueIDsOnEnabledByLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Logical.def")
	content := "logical layer with:\n" +
		"  device server.imac with:\n" +
		"    enabled by switch.switch from switch.imac;\n" +
		"  end;\n" +
		"  device appliance.tv with:\n" +
		"    enabled by switch.switch from switch.tv with delay_off 00:01:00;\n" +
		"  end;\n" +
		"end;"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFrozenUniqueIDs(path, map[string]string{
		"server.imac!node":  "binary_sensor.imac_node",
		"appliance.tv!node": "binary_sensor.tv_node",
	})
	got, _ := os.ReadFile(path)
	for _, want := range []string{
		"    enabled by switch.switch from switch.imac as binary_sensor.imac_node;\n",
		"    enabled by switch.switch from switch.tv as binary_sensor.tv_node with delay_off 00:01:00;\n",
	} {
		if !strings.Contains(defStatements(string(got)), strings.TrimSpace(want)) {
			t.Errorf("Logical.def lacks %q:\n%s", want, got)
		}
	}
}
