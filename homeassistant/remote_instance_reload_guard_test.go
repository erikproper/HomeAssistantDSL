package main

import (
	"strings"
	"testing"
)

// TestReloadGuard: the line is parsed into a guard; the automation triggers on the capability's
// source entity being unavailable for the given time, reloads its integration, and pauses.
func TestReloadGuard(t *testing.T) {
	devices, warnings := parseHassBridgeIntegrationBody([]string{
		"device sensors.netatmo_office with:",
		"reload integration when sensor.co2 is unavailable for 00:05:00;",
		"capability sensor.co2 sensor.office_carbon_dioxide;",
		"end;",
	}, "ha2mqtt")
	if len(warnings) != 0 || len(devices) != 1 || len(devices[0].ReloadGuards) != 1 {
		t.Fatalf("devices %+v, warnings %v", devices, warnings)
	}
	guards, problems := bridgeReloadGuards("ha2mqtt", map[string]THassBridgeDevice{"sensors.netatmo_office": devices[0]})
	body, ok := reloadGuardAutomationBody("ha2mqtt", guards)
	if !ok || len(problems) != 0 {
		t.Fatalf("ok %v, problems %v", ok, problems)
	}
	for _, want := range []string{"entity_id: sensor.office_carbon_dioxide", `for: "00:05:00"`, "homeassistant.reload_config_entry", `delay: "00:15:00"`, "mode: single"} {
		if !strings.Contains(body, want) {
			t.Errorf("automation lacks %q:\n%s", want, body)
		}
	}
	if mainGuards, _ := bridgeReloadGuards("main", map[string]THassBridgeDevice{"sensors.netatmo_office": devices[0]}); len(mainGuards) != 0 {
		t.Errorf("a guard of ha2mqtt resolved for main")
	}
}

// TestLocalReloadGuardWhileUp: a local device's guard runs on main, its "while <device> is up"
// becoming a condition on that device's node.
func TestLocalReloadGuardWhileUp(t *testing.T) {
	devices, warnings := parseLocalIntegrationBody([]string{
		"device appliance.sonos_headphones with:",
		"reload integration when media_player.media_player is unavailable for 00:05:00 while node.sonos_headphones is up;",
		"capability media_player.media_player;",
		"end;",
	})
	if len(warnings) != 0 || len(devices) != 1 || len(devices[0].ReloadGuards) != 1 {
		t.Fatalf("devices %+v, warnings %v", devices, warnings)
	}
	admin := newAdministrationState()
	admin.DeviceConceptualLinks["appliance.sonos_headphones"] = TDeviceConceptualLink{AttributeEntityIDs: map[string]TDeviceAttributeLink{"media_player": {EntityID: "media_player.social_house_sonos_headphones"}}}
	admin.DeviceConceptualLinks["node.sonos_headphones"] = TDeviceConceptualLink{NodeEntityID: "binary_sensor.infrastructural_sonos_headphones_ping_node"}
	guards, problems := localReloadGuards(map[string]TLocalDevice{"appliance.sonos_headphones": devices[0]}, admin)
	if len(problems) != 0 || len(guards) != 1 {
		t.Fatalf("guards %v, problems %v", guards, problems)
	}
	body, _ := reloadGuardAutomationBody("main", guards)
	for _, want := range []string{"entity_id: media_player.social_house_sonos_headphones", "id: guard_0", "'guard_0': 'binary_sensor.infrastructural_sonos_headphones_ping_node'"} {
		if !strings.Contains(body, want) {
			t.Errorf("automation lacks %q:\n%s", want, body)
		}
	}
}
