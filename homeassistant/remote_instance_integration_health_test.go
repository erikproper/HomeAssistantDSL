package main

import (
	"strings"
	"testing"
)

// TestIntegrationHealth: every instance reports its config entries that aren't loaded on its own
// topic, and main notifies per instance through the declared service (PROJECT.md 29).
func TestIntegrationHealth(t *testing.T) {
	if got := collectProblemsNotifyService("  home_assistant main: vienna;\n  notify problems via notify.mobile_app_phone; # Erik\n"); got != "notify.mobile_app_phone" {
		t.Errorf("notify service = %q", got)
	}
	if got := collectProblemsNotifyService("  home_assistant main: vienna;\n"); got != "" {
		t.Errorf("notify service without the line = %q", got)
	}
	health := integrationHealthAutomationBody("ha2mqtt")
	for _, want := range []string{"id: automation.coordinator_integration_health_ha2mqtt", "platform: time_pattern", `topic: "homeassistant_instances/ha2mqtt/integrations"`, "config_entry_attr(id, 'state')", "disabled_by", `topic: "homeassistant_instances/ha2mqtt/updates"`, "states.update", "!= 'mqtt'"} {
		if !strings.Contains(health, want) {
			t.Errorf("health automation lacks %q:\n%s", want, health)
		}
	}
	if hosts := containerHostsOf(map[string]THomeAssistantInstance{"main": {Name: "vienna"}, "ha2mqtt": {Name: "protocols-server-1"}}); len(hosts) != 1 || hosts[0] != "protocols-server-1" {
		t.Errorf("container hosts = %v", hosts)
	}
	notify := integrationProblemsNotifyAutomationBody([]string{"ha2mqtt", "main"}, "notify.mobile_app_phone", "vienna")
	for _, want := range []string{"- binary_sensor.infrastructural_integrations_ha2mqtt", "- binary_sensor.infrastructural_integrations_main", "attribute: items", `for: "00:15:00"`, "attributes.get('items', [])", "service: notify.mobile_app_phone", "tag: \"integrations_vienna_{{ instance }}\""} {
		if !strings.Contains(notify, want) {
			t.Errorf("notify automation lacks %q:\n%s", want, notify)
		}
	}
}
