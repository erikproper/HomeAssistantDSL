package main

import (
	"strings"
	"testing"
)

func TestValidateBatteryLevelAlertBiconditional(t *testing.T) {
	hostDevicesByID := map[string]THostDevice{
		"host.level_only": {Capabilities: map[string]string{"battery_level": "sensor.x"}},
	}
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.both":         {Capabilities: map[string]THassBridgeCapability{"battery_level": {}, "battery_alert": {}}},
		"hass.derived_pair": {Capabilities: map[string]THassBridgeCapability{"battery_level": {}, "battery_alert": {DerivedFromCapability: "battery_level"}}},
		"hass.level_only":   {Capabilities: map[string]THassBridgeCapability{"battery_level": {}}},
		"hass.alert_only":   {Capabilities: map[string]THassBridgeCapability{"battery_alert": {}}},
		"hass.neither":      {Capabilities: map[string]THassBridgeCapability{"co2": {}}},
	}
	importedDevicesByID := map[string]TImportedDevice{
		"import.level_only": {Capabilities: map[string]TImportedCapability{"battery_level": {}}},
	}
	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"discovery.derived_pair": {Capabilities: map[string]TDiscoveryCapability{"battery_level": {}, "battery_alert": {DerivedFromCapability: "battery_level"}}},
		"discovery.level_only":   {Capabilities: map[string]TDiscoveryCapability{"battery_level": {}}},
	}
	commandlineDevicesByID := map[string]TCommandlineDevice{
		"commandline.level_only": {Capabilities: map[string]TCommandlineCapability{"battery_level": {}}},
	}
	// Regression case (found live 2026-09-30): a "local"-kind device declaring "battery_level" in
	// Physical.def with its own "battery_alert" only ever declared as a Logical.def "derived"
	// capability on the same device id -- this check must still flag it, since Rule 2 is a
	// physical-layer requirement and deliberately never cross-references Logical.def.
	localDevicesByID := map[string]TLocalDevice{
		"local.both":                     {Capabilities: map[string]TLocalCapability{"battery_level": {}, "battery_alert": {}}},
		"local.level_only":               {Capabilities: map[string]TLocalCapability{"battery_level": {}}},
		"local.level_only_logical_alert": {Capabilities: map[string]TLocalCapability{"battery_level": {}}},
	}

	warnings := validateBatteryLevelAlertBiconditional(hostDevicesByID, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, commandlineDevicesByID, localDevicesByID)
	joined := strings.Join(warnings, "\n")

	if len(warnings) != 8 {
		t.Fatalf("got %d warnings, want 8: %v", len(warnings), warnings)
	}
	for _, want := range []string{
		`"host.level_only"`, `"hass.level_only"`, `"hass.alert_only"`, `"import.level_only"`,
		`"discovery.level_only"`, `"commandline.level_only"`, `"local.level_only"`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings = %v, want one mentioning %s", warnings, want)
		}
	}
	for _, mustNotAppear := range []string{`"hass.both"`, `"hass.derived_pair"`, `"hass.neither"`, `"discovery.derived_pair"`, `"local.both"`} {
		if strings.Contains(joined, mustNotAppear) {
			t.Errorf("warnings = %v, must not mention %s", warnings, mustNotAppear)
		}
	}
	// "local.level_only_logical_alert" has NO Physical.def "battery_alert" of its own in this
	// fixture (Logical.def-level derivation is out of scope for this check by design) -- it must
	// warn exactly like "local.level_only", not be silently treated as satisfied.
	if !strings.Contains(joined, `"local.level_only_logical_alert"`) {
		t.Errorf("warnings = %v, want a warning for local.level_only_logical_alert (its Logical.def-only battery_alert must not satisfy the physical-layer check)", warnings)
	}
}
