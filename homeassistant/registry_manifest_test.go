package main

import (
	"strings"
	"testing"
)

// TestRegistryDisplayOptions: a defaults rule's precision and unit become a sensor's display
// options; a sensor no rule matches gets none (PROJECT.md 24).
func TestRegistryDisplayOptions(t *testing.T) {
	rules, warnings := parseCapabilityDefaultsBody([]string{
		"for sensor.*/pressure:", `unit: "hPa";`, "precision: 0;", "end;",
		"for sensor.*/temperature:", "precision: 1;", "end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("warnings %v", warnings)
	}
	admin := newAdministrationState()
	admin.CapabilityDefaults = rules
	admin.SpaceOrder = []string{"social/terrace"}
	for _, name := range []string{"sensor.social/terrace/pressure", "sensor.social/terrace/temperature", "sensor.social/terrace/wind_direction"} {
		path := strings.TrimPrefix(name, "sensor.social/")
		admin.EntityRecordsBySpace["social/terrace"] = append(admin.EntityRecordsBySpace["social/terrace"], TEntityRecord{Name: name, Identity: TEntityIdentity{Domain: "sensor", Sphere: "social", Path: path}})
	}
	display := registryDisplayOptions(admin)
	if len(display) != 2 || display[0].EntityID != "sensor.social_terrace_pressure" || *display[0].Precision != 0 || display[0].Unit != "hPa" || *display[1].Precision != 1 || display[1].Unit != "" {
		t.Errorf("display = %+v", display)
	}
}
