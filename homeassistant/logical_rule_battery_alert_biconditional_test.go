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
	}

	warnings := validateLogicalBatteryLevelAlertBiconditional(logicalDevicesByID)
	joined := strings.Join(warnings, "\n")

	if len(warnings) != 2 {
		t.Fatalf("got %d warnings, want 2: %v", len(warnings), warnings)
	}
	for _, want := range []string{`"appliance.level_only"`, `"appliance.alert_only"`} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings = %v, want one mentioning %s", warnings, want)
		}
	}
	for _, mustNotAppear := range []string{`"appliance.both"`, `"appliance.neither"`} {
		if strings.Contains(joined, mustNotAppear) {
			t.Errorf("warnings = %v, must not mention %s", warnings, mustNotAppear)
		}
	}
}
