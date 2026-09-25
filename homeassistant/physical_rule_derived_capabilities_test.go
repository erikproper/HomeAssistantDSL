package main

import (
	"strings"
	"testing"
)

// TestValidateDerivedCapabilitiesAcceptsWellFormedChain covers a two-step derivation (a capability
// derived from another derived capability), which must collapse fine with no warnings.
func TestValidateDerivedCapabilitiesAcceptsWellFormedChain(t *testing.T) {
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.roomba": {DeviceID: "hass.roomba", Capabilities: map[string]THassBridgeCapability{
			"battery_level": {Domain: "sensor"},
			"battery_alert": {Domain: "binary_sensor", DerivedFromCapability: "battery_level"},
			"battery_alert_delayed": {Domain: "binary_sensor", DerivedFromCapability: "battery_alert"},
		}},
	}
	warnings := validateDerivedCapabilities(hassBridgeDevicesByID, nil, nil)
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings for a well-formed chain: %v", warnings)
	}
}

// TestValidateDerivedCapabilitiesFlagsDanglingReference covers "derived from" a label the device
// doesn't declare at all.
func TestValidateDerivedCapabilitiesFlagsDanglingReference(t *testing.T) {
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.roomba": {DeviceID: "hass.roomba", Capabilities: map[string]THassBridgeCapability{
			"battery_alert": {Domain: "binary_sensor", DerivedFromCapability: "battery_level"},
		}},
	}
	warnings := validateDerivedCapabilities(hassBridgeDevicesByID, nil, nil)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "not a capability declared on the same device") {
		t.Errorf("warnings = %v, want exactly one dangling-reference warning", warnings)
	}
}

// TestValidateDerivedCapabilitiesFlagsDirectSelfDerivation covers a capability declared derived
// from itself.
func TestValidateDerivedCapabilitiesFlagsDirectSelfDerivation(t *testing.T) {
	importedDevicesByID := map[string]TImportedDevice{
		"import.x": {DeviceID: "import.x", Capabilities: map[string]TImportedCapability{
			"battery_alert": {Domain: "binary_sensor", DerivedFromCapability: "battery_alert"},
		}},
	}
	warnings := validateDerivedCapabilities(nil, importedDevicesByID, nil)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "loops back on itself") {
		t.Errorf("warnings = %v, want exactly one self-loop warning", warnings)
	}
}

// TestValidateDerivedCapabilitiesFlagsIndirectCycle covers a longer cycle (A from B, B from A).
func TestValidateDerivedCapabilitiesFlagsIndirectCycle(t *testing.T) {
	importedDevicesByID := map[string]TImportedDevice{
		"import.x": {DeviceID: "import.x", Capabilities: map[string]TImportedCapability{
			"a": {Domain: "sensor", DerivedFromCapability: "b"},
			"b": {Domain: "sensor", DerivedFromCapability: "a"},
		}},
	}
	warnings := validateDerivedCapabilities(nil, importedDevicesByID, nil)
	if len(warnings) != 2 {
		t.Fatalf("warnings = %v, want 2 (one per side of the cycle, each is its own top-level derived capability)", warnings)
	}
	for _, w := range warnings {
		if !strings.Contains(w, "loops back on itself") {
			t.Errorf("warning %q does not mention a loop", w)
		}
	}
}
