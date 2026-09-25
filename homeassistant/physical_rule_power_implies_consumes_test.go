package main

import (
	"strings"
	"testing"
)

func TestValidatePowerImpliesConsumes(t *testing.T) {
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.both":          {Capabilities: map[string]THassBridgeCapability{"power": {}, "consumes": {}}},
		"hass.derived_pair":  {Capabilities: map[string]THassBridgeCapability{"power": {}, "consumes": {DerivedFromCapability: "power"}}},
		"hass.power_only":    {Capabilities: map[string]THassBridgeCapability{"power": {}}},
		"hass.consumes_only": {Capabilities: map[string]THassBridgeCapability{"consumes": {}}},
		"hass.neither":       {Capabilities: map[string]THassBridgeCapability{"co2": {}}},
	}
	importedDevicesByID := map[string]TImportedDevice{
		"import.power_only": {Capabilities: map[string]TImportedCapability{"power": {}}},
	}
	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"discovery.derived_pair": {Capabilities: map[string]TDiscoveryCapability{"power": {}, "consumes": {DerivedFromCapability: "power"}}},
		"discovery.power_only":   {Capabilities: map[string]TDiscoveryCapability{"power": {}}},
	}

	warnings := validatePowerImpliesConsumes(hassBridgeDevicesByID, importedDevicesByID, discoveryGatewaysByID)
	joined := strings.Join(warnings, "\n")

	if len(warnings) != 3 {
		t.Fatalf("got %d warnings, want 3: %v", len(warnings), warnings)
	}
	for _, want := range []string{`"hass.power_only"`, `"import.power_only"`, `"discovery.power_only"`} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings = %v, want one mentioning %s", warnings, want)
		}
	}
	// "consumes" without "power" is NOT flagged -- Rule 3 is one-directional, unlike Rule 2's
	// battery_level/battery_alert biconditional.
	for _, mustNotAppear := range []string{`"hass.both"`, `"hass.derived_pair"`, `"hass.consumes_only"`, `"hass.neither"`, `"discovery.derived_pair"`} {
		if strings.Contains(joined, mustNotAppear) {
			t.Errorf("warnings = %v, must not mention %s", warnings, mustNotAppear)
		}
	}
}
