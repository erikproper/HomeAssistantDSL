package main

import (
	"strings"
	"testing"
)

// TestValidateLogicalNodeCapabilityRequired covers the 2026-10-05 fix: a brand-new logical-only
// device (no physical-layer counterpart under the same id) with capabilities of its own but no
// "node" among them gets flagged -- the real live case being "fan.house_wc" absorbing "fan.fan"
// from a physical device without also absorbing that device's own "node". A logical device that
// EXTENDS an existing physical-layer device (any kind) is never flagged, regardless of whether it
// declares "node" itself, since the physical layer already owns that device's own liveness story.
func TestValidateLogicalNodeCapabilityRequired(t *testing.T) {
	logicalDevicesByID := map[string]TLogicalDevice{
		"fan.missing": {DeviceID: "fan.missing", Capabilities: map[string]TLogicalCapability{
			"fan": {Domain: "fan"},
		}},
		"fan.compliant": {DeviceID: "fan.compliant", Capabilities: map[string]TLogicalCapability{
			"fan":  {Domain: "fan"},
			"node": {Domain: "binary_sensor"},
		}},
		"switches.extends_discovery": {DeviceID: "switches.extends_discovery", Capabilities: map[string]TLogicalCapability{
			"motion": {Domain: "binary_sensor"},
		}},
		"utility.dependency_only": {DeviceID: "utility.dependency_only", DependsOn: []string{"node.somewhere"}},
	}
	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"switches.extends_discovery": {DeviceID: "switches.extends_discovery"},
	}

	warnings := validateLogicalNodeCapabilityRequired(
		logicalDevicesByID,
		map[string]THostDevice{},
		discoveryGatewaysByID,
		map[string]THassBridgeDevice{},
		map[string]TImportedDevice{},
		map[string]TCommandlineDevice{},
		map[string]TLocalDevice{},
	)

	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, `"fan.missing"`) {
		t.Errorf("expected a warning for fan.missing, got: %v", warnings)
	}
	if strings.Contains(joined, `"fan.compliant"`) {
		t.Errorf("fan.compliant declares \"node\", should not be flagged: %v", warnings)
	}
	if strings.Contains(joined, `"switches.extends_discovery"`) {
		t.Errorf("switches.extends_discovery extends an existing discovery-kind device, should not be flagged: %v", warnings)
	}
	if strings.Contains(joined, `"utility.dependency_only"`) {
		t.Errorf("utility.dependency_only declares no capabilities of its own, should not be flagged: %v", warnings)
	}
	if len(warnings) != 1 {
		t.Errorf("expected exactly 1 warning, got %d: %v", len(warnings), warnings)
	}
}
