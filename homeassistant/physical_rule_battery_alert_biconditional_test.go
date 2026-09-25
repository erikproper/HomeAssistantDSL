package main

import (
	"strings"
	"testing"
)

func TestValidateBatteryLevelAlertBiconditional(t *testing.T) {
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

	warnings := validateBatteryLevelAlertBiconditional(hassBridgeDevicesByID, importedDevicesByID, discoveryGatewaysByID)
	joined := strings.Join(warnings, "\n")

	if len(warnings) != 4 {
		t.Fatalf("got %d warnings, want 4: %v", len(warnings), warnings)
	}
	for _, want := range []string{`"hass.level_only"`, `"hass.alert_only"`, `"import.level_only"`, `"discovery.level_only"`} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings = %v, want one mentioning %s", warnings, want)
		}
	}
	for _, mustNotAppear := range []string{`"hass.both"`, `"hass.derived_pair"`, `"hass.neither"`, `"discovery.derived_pair"`} {
		if strings.Contains(joined, mustNotAppear) {
			t.Errorf("warnings = %v, must not mention %s", warnings, mustNotAppear)
		}
	}
}
