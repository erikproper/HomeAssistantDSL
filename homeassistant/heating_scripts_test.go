package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateHeatingScriptsIncludesSwitchHeatedSpaces: a house's set_heating_to_<preset> script
// calls the scripts of both its thermostat-heated and its switch-heated rooms, and leaves the
// switch-heated room's own script to generateSwitchControlledHeatingContent (2026-11-20, the
// office, bathroom, corridor and storage room in Junglinster).
func TestGenerateHeatingScriptsIncludesSwitchHeatedSpaces(t *testing.T) {
	admin := newAdministrationState()
	admin.SpaceOrder = []string{"social/house", "social/house/kitchen", "social/house/office"}
	for space, name := range map[string]string{
		"social/house/kitchen": "input_number.social/house/kitchen/temperature_desired",
		"social/house/office":  "switch.social/house/office/heating",
	} {
		admin.AppendEntityRecord(space, TEntityRecord{Name: name, Identity: extractEntityIdentity(name)})
	}
	dir := t.TempDir()
	if err := generateHeatingScripts(dir, admin); err != nil {
		t.Fatal(err)
	}
	house, err := os.ReadFile(filepath.Join(dir, "script", "social", "script.social_house_set_heating_to_day.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, child := range []string{"script.social_house_kitchen_set_heating_to_day", "script.social_house_office_set_heating_to_day"} {
		if !strings.Contains(string(house), child) {
			t.Errorf("house script doesn't call %s:\n%s", child, house)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "script", "social", "script.social_house_office_set_heating_to_day.yaml")); !os.IsNotExist(err) {
		t.Errorf("the switch-heated office's own script was written here (err=%v), want it left to the switch generator", err)
	}
}
