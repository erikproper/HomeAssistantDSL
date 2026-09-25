package main

import (
	"strings"
	"testing"
)

func TestIsCapabilitySuffixOf(t *testing.T) {
	cases := []struct {
		short, long string
		want        bool
	}{
		{"node", "aqara/node", true},
		{"mm", "ll/mm", true},
		{"mm", "kk/ll/mm", true},
		{"ll/mm", "kk/ll/mm", true},
		{"l/mm", "kk/ll/mm", false}, // "l" is not a whole segment of "kk/ll/mm" ("ll" is)
		{"node", "node", false},     // identical names, not this rule's concern
		{"aqara/node", "node", false},
	}
	for _, c := range cases {
		if got := isCapabilitySuffixOf(c.short, c.long); got != c.want {
			t.Errorf("isCapabilitySuffixOf(%q, %q) = %v, want %v", c.short, c.long, got, c.want)
		}
	}
}

// TestValidateNoOverlappingCapabilitySuffixes is the regression test for the user's own explicit
// request (2026-09-25): a device declaring both "mm"-shaped and "ll/mm"-shaped capabilities is
// forbidden outright, since the no-rename shorthand's own longest-suffix matching makes the two
// ambiguous bookkeeping even though resolution itself is deterministic.
func TestValidateNoOverlappingCapabilitySuffixes(t *testing.T) {
	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"sensors.hallway_aqara_windoor": {
			DeviceID: "sensors.hallway_aqara_windoor",
			Capabilities: map[string]TDiscoveryCapability{
				"node":       {Domain: "binary_sensor"},
				"aqara/node": {Domain: "binary_sensor"},
			},
		},
		"sensors.clean_device": {
			DeviceID: "sensors.clean_device",
			Capabilities: map[string]TDiscoveryCapability{
				"battery_level": {Domain: "sensor"},
				"humidity":      {Domain: "sensor"},
			},
		},
	}

	warnings := validateNoOverlappingCapabilitySuffixes(discoveryGatewaysByID, nil, nil, nil, nil, nil)
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly 1", warnings)
	}
	if !strings.Contains(warnings[0], "sensors.hallway_aqara_windoor") || !strings.Contains(warnings[0], "node") {
		t.Errorf("warnings[0] = %q, want it naming the colliding device and capability", warnings[0])
	}
}

func TestValidateNoOverlappingCapabilitySuffixesCleanDeviceProducesNoWarning(t *testing.T) {
	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"sensors.clean_device": {
			DeviceID: "sensors.clean_device",
			Capabilities: map[string]TDiscoveryCapability{
				"core":          {Domain: "binary_sensor"},
				"battery_level": {Domain: "sensor"},
			},
		},
	}
	if warnings := validateNoOverlappingCapabilitySuffixes(discoveryGatewaysByID, nil, nil, nil, nil, nil); len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
}
