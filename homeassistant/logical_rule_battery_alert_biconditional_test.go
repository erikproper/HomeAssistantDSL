package main

import (
	"strings"
	"testing"
)

func TestValidateLogicalBatteryLevelAlertBiconditional(t *testing.T) {
	logicalDevicesByID := map[string]TLogicalDevice{
		"appliance.both":       {Capabilities: map[string]TLogicalCapability{"battery_level": {}, "battery_alert": {}}},
		"appliance.level_only": {Capabilities: map[string]TLogicalCapability{"battery_level": {}}},
		"appliance.alert_only": {Capabilities: map[string]TLogicalCapability{"battery_alert": {}}},
		"appliance.neither":    {Capabilities: map[string]TLogicalCapability{"node": {}}},
		// Overrides only the alert; its level is physical (PROJECT.md 31a).
		"sensors.physical_level": {Capabilities: map[string]TLogicalCapability{"battery_alert": {}}},
	}
	physicalHas := func(deviceID, label string) bool {
		return deviceID == "sensors.physical_level" && label == "battery_level"
	}

	warnings := validateLogicalBatteryLevelAlertBiconditional(logicalDevicesByID, physicalHas)
	joined := strings.Join(warnings, "\n")

	if len(warnings) != 2 {
		t.Fatalf("got %d warnings, want 2: %v", len(warnings), warnings)
	}
	for _, want := range []string{`"appliance.level_only"`, `"appliance.alert_only"`} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings = %v, want one mentioning %s", warnings, want)
		}
	}
	for _, mustNotAppear := range []string{`"appliance.both"`, `"appliance.neither"`, `"sensors.physical_level"`} {
		if strings.Contains(joined, mustNotAppear) {
			t.Errorf("warnings = %v, must not mention %s", warnings, mustNotAppear)
		}
	}
}
