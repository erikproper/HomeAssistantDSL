package main

import (
	"strings"
	"testing"
)

// TestParseDiscoveryIntegrationBodyAssignedID covers PROJECT.md item 7's "as <id>;" suffix on a
// plain raw-leaf capability line (memory/project_stable_entity_identity_design.md): once present,
// it's captured into TDiscoveryCapability.AssignedID, leaving Leaf as the raw leaf alone.
func TestParseDiscoveryIntegrationBodyAssignedID(t *testing.T) {
	devices, warnings := parseDiscoveryIntegrationBody([]string{
		"device switches.garage_fuse_cabinet_zwave_058 with:",
		"  identifiers \"zwavejs2mqtt_0xd0204fb2_node58\";",
		"  capability fan.fan zwavejs2mqtt_0xd0204fb2_58-37-1-currentValue as switches_garage_fuse_cabinet_zwave_058_zwavejs2mqtt_0xd0204fb2_58-37-1-currentValue;",
		"end;",
	})
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	cap, ok := devices[0].Capabilities["fan"]
	if !ok {
		t.Fatalf("capability %q not found", "fan")
	}
	if cap.Leaf != "zwavejs2mqtt_0xd0204fb2_58-37-1-currentValue" {
		t.Errorf("Leaf = %q, want the raw leaf with no trailing \"as\" clause", cap.Leaf)
	}
	if cap.AssignedID != "switches_garage_fuse_cabinet_zwave_058_zwavejs2mqtt_0xd0204fb2_58-37-1-currentValue" {
		t.Errorf("AssignedID = %q, want the frozen id", cap.AssignedID)
	}
}

// TestParseDiscoveryIntegrationBodyAssignedIDWithSourceDomain covers "as <id>;" composing with
// the "<raw-domain>:<leaf>" disambiguation qualifier -- both are orthogonal, so both should
// survive together on the same line.
func TestParseDiscoveryIntegrationBodyAssignedIDWithSourceDomain(t *testing.T) {
	devices, warnings := parseDiscoveryIntegrationBody([]string{
		"device buttons.example with:",
		"  capability event.pressed event:0x1234_action_zigbee2mqtt as buttons_example_event_0x1234_action_zigbee2mqtt;",
		"end;",
	})
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	cap := devices[0].Capabilities["pressed"]
	if cap.SourceDomain != "event" || cap.Leaf != "0x1234_action_zigbee2mqtt" {
		t.Errorf("SourceDomain/Leaf = %q/%q, want event/0x1234_action_zigbee2mqtt", cap.SourceDomain, cap.Leaf)
	}
	if cap.AssignedID != "buttons_example_event_0x1234_action_zigbee2mqtt" {
		t.Errorf("AssignedID = %q, want the frozen id", cap.AssignedID)
	}
}

// TestParseDiscoveryIntegrationBodyAssignedIDRejectedOnAvailability covers the rejection case:
// "as <id>;" on an "... is available;" capability is meaningless (that capability never flows
// through relayedDiscoveryUniqueID) -- warned and ignored, not silently accepted.
func TestParseDiscoveryIntegrationBodyAssignedIDRejectedOnAvailability(t *testing.T) {
	devices, warnings := parseDiscoveryIntegrationBody([]string{
		"device sensors.example with:",
		"  capability binary_sensor.node light.core is available as should_be_ignored;",
		"end;",
	})
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "is available") {
		t.Errorf("warning doesn't mention the real cause: %v", warnings)
	}
	cap := devices[0].Capabilities["node"]
	if cap.AssignedID != "" {
		t.Errorf("AssignedID = %q, want empty (rejected)", cap.AssignedID)
	}
	if cap.AvailabilityOf != "core" || cap.AvailabilityOfDomain != "light" {
		t.Errorf("AvailabilityOf/AvailabilityOfDomain = %q/%q, want core/light (still resolved despite the rejected suffix)", cap.AvailabilityOf, cap.AvailabilityOfDomain)
	}
}

// TestParseDiscoveryIntegrationBodyNoAssignedID confirms a plain capability line with no "as"
// clause at all leaves AssignedID empty -- the common, not-yet-bootstrapped case.
func TestParseDiscoveryIntegrationBodyNoAssignedID(t *testing.T) {
	devices, warnings := parseDiscoveryIntegrationBody([]string{
		"device sensors.example with:",
		"  capability sensor.battery_level 0x1234_battery_zigbee2mqtt;",
		"end;",
	})
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	cap := devices[0].Capabilities["battery_level"]
	if cap.AssignedID != "" {
		t.Errorf("AssignedID = %q, want empty", cap.AssignedID)
	}
	if cap.Leaf != "0x1234_battery_zigbee2mqtt" {
		t.Errorf("Leaf = %q, want the raw leaf unchanged", cap.Leaf)
	}
}
