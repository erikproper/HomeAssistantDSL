package main

import (
	"strings"
	"testing"
)

// TestValidateNodeCapabilityRequired is a focused unit test for Rule 1
// (plans/derived-capability-mechanism.md's "Two hard-enforcement rules"): a hassbridge or import
// device with no "node" capability gets flagged; one that declares "node" doesn't.
func TestValidateNodeCapabilityRequired(t *testing.T) {
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.compliant": {DeviceID: "hass.compliant", Capabilities: map[string]THassBridgeCapability{
			"node": {Domain: "binary_sensor"},
		}},
		"hass.missing": {DeviceID: "hass.missing", Capabilities: map[string]THassBridgeCapability{
			"co2": {Domain: "sensor"},
		}},
	}
	importedDevicesByID := map[string]TImportedDevice{
		"import.compliant": {DeviceID: "import.compliant", Capabilities: map[string]TImportedCapability{
			"node": {},
		}},
		"import.missing": {DeviceID: "import.missing", Capabilities: map[string]TImportedCapability{
			"battery_level": {},
		}},
	}

	warnings := validateNodeCapabilityRequired(hassBridgeDevicesByID, importedDevicesByID)

	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, `"hass.missing"`) {
		t.Errorf("expected a warning for hass.missing, got: %v", warnings)
	}
	if !strings.Contains(joined, `"import.missing"`) {
		t.Errorf("expected a warning for import.missing, got: %v", warnings)
	}
	if strings.Contains(joined, `"hass.compliant"`) {
		t.Errorf("hass.compliant declares \"node\", should not be flagged: %v", warnings)
	}
	if strings.Contains(joined, `"import.compliant"`) {
		t.Errorf("import.compliant declares \"node\", should not be flagged: %v", warnings)
	}
	if len(warnings) != 2 {
		t.Errorf("expected exactly 2 warnings, got %d: %v", len(warnings), warnings)
	}
}
