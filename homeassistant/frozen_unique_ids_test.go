package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStripAndCollectDerivedFrozenIDs: a derived line's "as <id>" is hidden from Physical.def's own
// capability parsers, and collected per device.
func TestStripAndCollectDerivedFrozenIDs(t *testing.T) {
	content := "device sensors.a with:\n" +
		"  derived capability binary_sensor.node as binary_sensor.infra_a_node with condition sensor.x is available;\n" +
		"  derived capability binary_sensor.battery_alert as binary_sensor.infra_a_battery_alert from sensor.battery_level via \"$ < 5\";\n" +
		"end;\n" +
		"device sensors.b with:\n" +
		"  derived capability binary_sensor.node with condition sensor.y is available;\n" +
		"end;"
	stripped := stripDerivedAssignedIDs(content)
	want := "device sensors.a with:\n" +
		"  derived capability binary_sensor.node with condition sensor.x is available;\n" +
		"  derived capability binary_sensor.battery_alert from sensor.battery_level via \"$ < 5\";\n" +
		"end;\n" +
		"device sensors.b with:\n" +
		"  derived capability binary_sensor.node with condition sensor.y is available;\n" +
		"end;"
	if stripped != want {
		t.Errorf("stripped =\n%s\nwant\n%s", stripped, want)
	}
	ids := collectDerivedFrozenIDs(content)
	if len(ids) != 2 || ids["sensors.a!node"] != "binary_sensor.infra_a_node" || ids["sensors.a!battery_alert"] != "binary_sensor.infra_a_battery_alert" {
		t.Errorf("ids = %v", ids)
	}
}

// TestParseLogicalLayerBodyFrozenIDs: both Logical.def derived forms carry an "as <id>" before
// "with" -- the one-liner here with its condition on the next line.
func TestParseLogicalLayerBodyFrozenIDs(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device appliance.tv with:",
		"  derived capability binary_sensor.node as binary_sensor.infra_tv_node with",
		"    condition physical appliance.tv and node.tv;",
		"  derived capability binary_sensor.windy as binary_sensor.social_windy with:",
		"    condition node.tv;",
		"    delay_off 00:01:00;",
		"  end;",
		"end;",
	}, nil)
	if len(warnings) != 0 || len(devices) != 1 {
		t.Fatalf("devices %+v, warnings %v", devices, warnings)
	}
	caps := devices[0].Capabilities
	if caps["node"].AssignedID != "binary_sensor.infra_tv_node" || caps["windy"].AssignedID != "binary_sensor.social_windy" {
		t.Errorf("AssignedIDs = %q, %q", caps["node"].AssignedID, caps["windy"].AssignedID)
	}
	if caps["node"].DerivedCondition == nil || caps["windy"].DelayOff != "00:01:00" {
		t.Errorf("rest of the capabilities lost: %+v", caps)
	}
}

// TestWriteFrozenUniqueIDs: the ID lands right after a derived capability's name (also when the
// statement spans lines, or is a "with:" block header), and only within the right device.
func TestWriteFrozenUniqueIDs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Logical.def")
	content := "logical layer with:\n" +
		"  device appliance.other with:\n" +
		"    derived capability binary_sensor.node with condition node.other;\n" +
		"  end;\n" +
		"  device appliance.tv with:\n" +
		"    derived capability binary_sensor.node with condition physical appliance.tv and\n" +
		"      node.tv;\n" +
		"    derived capability binary_sensor.windy with:\n" +
		"      condition node.tv;\n" +
		"    end;\n" +
		"  end;\n" +
		"end;"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	reports := writeFrozenUniqueIDs(path, map[string]string{
		"appliance.tv!node":  "binary_sensor.infra_tv_node",
		"appliance.tv!windy": "binary_sensor.social_windy",
		"appliance.tv!other": "binary_sensor.not_derived", // no derived line: silently left alone
	})
	if len(reports) != 2 {
		t.Errorf("reports = %v, want 2 assignments", reports)
	}
	got, _ := os.ReadFile(path)
	want := "logical layer with:\n" +
		"  device appliance.other with:\n" +
		"    derived capability binary_sensor.node with condition node.other;\n" +
		"  end;\n" +
		"  device appliance.tv with:\n" +
		"    derived capability binary_sensor.node as binary_sensor.infra_tv_node with condition physical appliance.tv and\n" +
		"      node.tv;\n" +
		"    derived capability binary_sensor.windy as binary_sensor.social_windy with:\n" +
		"      condition node.tv;\n" +
		"    end;\n" +
		"  end;\n" +
		"end;"
	if string(got) != want {
		t.Errorf("file =\n%s\nwant\n%s", got, want)
	}
}

// TestUniqueIDFor: a frozen ID wins over the entity_id; without one, the entity_id stays.
func TestUniqueIDFor(t *testing.T) {
	admin := newAdministrationState()
	admin.AppendEntityRecord("root", TEntityRecord{Name: "binary_sensor.infrastructural/tv/node", SourceKey: "logical:appliance.tv!node"})
	admin.AppendEntityRecord("root", TEntityRecord{Name: "binary_sensor.infrastructural/tv/other", SourceKey: "logical:appliance.tv!other"})
	admin.FrozenUniqueIDs["logical:appliance.tv!node"] = "binary_sensor.old_name_node"
	if got := admin.UniqueIDFor("binary_sensor.infrastructural_tv_node"); got != "binary_sensor.old_name_node" {
		t.Errorf("frozen: got %q", got)
	}
	if got := admin.UniqueIDFor("binary_sensor.infrastructural_tv_other"); got != "binary_sensor.infrastructural_tv_other" {
		t.Errorf("not frozen: got %q", got)
	}
}

// TestUniqueIDForSharedSourceKey: a capability feeding two entities freezes nothing for either --
// one def line can't hold two IDs (the Junglinster fan.house_wc/zwave_058 "node" case, 2026-10-21).
func TestUniqueIDForSharedSourceKey(t *testing.T) {
	admin := newAdministrationState()
	admin.AppendEntityRecord("a", TEntityRecord{Name: "binary_sensor.infrastructural/fuse_cabinet/zwave_058/node", SourceKey: "physical:switches.zwave_058!node"})
	admin.AppendEntityRecord("b", TEntityRecord{Name: "binary_sensor.infrastructural/house/wc/node", SourceKey: "physical:switches.zwave_058!node"})
	admin.FrozenUniqueIDs["physical:switches.zwave_058!node"] = "binary_sensor.infrastructural_house_wc_node"
	for _, entityID := range []string{"binary_sensor.infrastructural_fuse_cabinet_zwave_058_node", "binary_sensor.infrastructural_house_wc_node"} {
		if got := admin.UniqueIDFor(entityID); got != entityID {
			t.Errorf("UniqueIDFor(%q) = %q, want the entity_id itself", entityID, got)
		}
	}
	if reports := autoAssignFrozenUniqueIDs(t.TempDir(), admin); len(reports) != 1 {
		t.Errorf("reports = %v, want exactly one overlap report", reports)
	}
}

// TestBridgedFrozenIDsInBlockHeader: a bridged capability written as a "with:" block carries its
// frozen ID in the header -- lifted off for the parsers, collected like any other.
func TestBridgedFrozenIDsInBlockHeader(t *testing.T) {
	content := "integration home_assistant ha2mqtt with:\n" +
		"  device appliance.vacuum with:\n" +
		"    vacuum.core vacuum.roomba as hassbridge_vacuum_x with:\n" +
		"      icon: \"mdi:robot-vacuum\";\n" +
		"    end;\n" +
		"    capability sensor.battery_level sensor.roomba_battery as hassbridge_sensor_y with icon: \"mdi:battery\";\n" +
		"  end;\n" +
		"end;"
	stripped := stripDerivedAssignedIDs(content)
	if strings.Contains(stripped, " as ") || !strings.Contains(stripped, "vacuum.core vacuum.roomba with:") {
		t.Errorf("stripped =\n%s", stripped)
	}
	ids := collectDerivedFrozenIDs(content)
	if ids["appliance.vacuum!core"] != "hassbridge_vacuum_x" || ids["appliance.vacuum!battery_level"] != "hassbridge_sensor_y" {
		t.Errorf("ids = %v", ids)
	}
}
