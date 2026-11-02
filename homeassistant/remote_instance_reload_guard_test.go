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
	for _, want := range []string{"entity_id: sensor.office_carbon_dioxide", `for: "00:05:00"`, "homeassistant.reload_config_entry", "total_seconds() >= 900 }}", "mode: single",
		// Reloaded by config entry, each entry once after a start (2026-11-01).
		`entry_id: "{{ config_entry_id(trigger.entity_id) }}"`, `entry_id: "{{ repeat.item }}"`, "id not in ns.ids",
		// The start companion: waits, then asks the guard by an event.
		"id: automation.coordinator_reload_guard_ha2mqtt_start", "event: coordinator_reload_guard\n", "event_type: coordinator_reload_guard\n",
		// The retry (2026-11-02): every few minutes, the entries down for at least their guard time.
		"id: automation.coordinator_reload_guard_ha2mqtt_retry", "platform: time_pattern", "'for': 300", ".last_changed).total_seconds() >= w.for",
		`reason: "still unavailable after the pause"`, "{{ trigger.event.data.reason }}"} {
		if !strings.Contains(body, want) {
			t.Errorf("automation lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "target:") || strings.Contains(body, `delay: "00:15:00"`) {
		t.Errorf("automation still reloads by target (deprecated) or pauses by a delay:\n%s", body)
	}
	if mainGuards, _ := bridgeReloadGuards("main", map[string]THassBridgeDevice{"sensors.netatmo_office": devices[0]}); len(mainGuards) != 0 {
		t.Errorf("a guard of ha2mqtt resolved for main")
	}
}

// TestReloadGuardPauses: "at most every" sets a guard's pause; guards with another pause than the
// shortest get their own automations, so the plain names stay with the shortest.
func TestReloadGuardPauses(t *testing.T) {
	devices, warnings := parseHassBridgeIntegrationBody([]string{
		"device sensors.netatmo_office with:",
		"reload integration when sensor.co2 is unavailable for 00:05:00 at most every 24:00:00;",
		"capability sensor.co2 sensor.office_carbon_dioxide;",
		"end;",
		"device sensors.ring with:",
		"reload integration when camera.camera is unavailable for 00:05:00;",
		"capability camera.camera camera.front_door;",
		"end;",
	}, "ha2mqtt")
	if len(warnings) != 0 || len(devices) != 2 || devices[0].ReloadGuards[0].Pause != "24:00:00" {
		t.Fatalf("devices %+v, warnings %v", devices, warnings)
	}
	byID := map[string]THassBridgeDevice{}
	for _, d := range devices {
		byID[d.DeviceID] = d
	}
	guards, _ := bridgeReloadGuards("ha2mqtt", byID)
	body, _ := reloadGuardAutomationBody("ha2mqtt", guards)
	for _, want := range []string{
		"id: automation.coordinator_reload_guard_ha2mqtt\n", "total_seconds() >= 900 }}",
		"id: automation.coordinator_reload_guard_ha2mqtt_every_86400s\n", "total_seconds() >= 86400 }}",
		"id: automation.coordinator_reload_guard_ha2mqtt_every_86400s_start\n", "event_type: coordinator_reload_guard_every_86400s\n",
		"id: automation.coordinator_reload_guard_ha2mqtt_every_86400s_retry\n",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("automations lack %q:\n%s", want, body)
		}
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
