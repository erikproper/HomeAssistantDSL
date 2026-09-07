package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEntityExistenceInquiryAutomationBody(t *testing.T) {
	body := entityExistenceInquiryAutomationBody("protocols-server-2")

	if !strings.Contains(body, `topic: "homeassistant_instances/protocols-server-2/inquire"`) {
		t.Errorf("body = %s, want the inquiry command trigger topic", body)
	}
	if !strings.Contains(body, `topic: "homeassistant_instances/protocols-server-2/inquire/reply"`) {
		t.Errorf("body = %s, want the reply topic", body)
	}
	if !strings.Contains(body, "trigger.payload") {
		t.Errorf("body = %s, want the entity_id to check to come from the trigger payload, not a fixed entity list", body)
	}
	if !strings.Contains(body, "states[trigger.payload]") || !strings.Contains(body, "is not none") {
		t.Errorf("body = %s, want a single dict-lookup existence check, not a loop over states", body)
	}
	if strings.Contains(body, "for s in states") || strings.Contains(body, "for ") {
		t.Errorf("body = %s, must never iterate every state -- that's exactly what hung \"main\" before", body)
	}
	if !strings.Contains(body, "device_id(trigger.payload)") {
		t.Errorf("body = %s, want the device_id lookup for the queried entity", body)
	}
	if !strings.Contains(body, "device_entities(did)") {
		t.Errorf("body = %s, want device_entities() to surface sibling entities on the same device", body)
	}
	if !strings.Contains(body, "'sibling_entities'") {
		t.Errorf("body = %s, want sibling_entities in the reply payload", body)
	}
	if !strings.Contains(body, "device_attr(did, 'name')") || !strings.Contains(body, "'device_name'") {
		t.Errorf("body = %s, want device_name (device_attr lookup) in the reply payload, for grouping a discovered device's suggestion entry", body)
	}
	// Regression test, 2026-08-29: HA's automation default (mode: single) silently drops a new
	// trigger arriving while a previous run is still in progress -- confirmed live, some of a
	// "Discover entity" batch's own rapid-fire inquiries never got a reply at all under that
	// default. Must declare mode: queued (with a raised max) so no inquiry is ever silently lost.
	if !strings.Contains(body, "mode: queued") {
		t.Errorf("body = %s, want \"mode: queued\" so rapid-fire inquiries (e.g. a Discover entity batch) are never silently dropped", body)
	}
	if !strings.Contains(body, "max: 50") {
		t.Errorf("body = %s, want an explicit max raised above HA's own queued-mode default (10), so a big batch can't hit it either", body)
	}
	// Regression test, 2026-09-06: unit_of_measurement/device_class piggyback on the same
	// states[] lookup this automation already does, so the coordinator can use a bridged
	// entity's own remote-reported typing as a fallback when Physical.def/Defaults.def set
	// nothing -- a real gap found live where a bridged capability with no explicit typing showed
	// up with no unit/icon at all, even though the remote entity genuinely has both.
	if !strings.Contains(body, "s.attributes.get('unit_of_measurement')") || !strings.Contains(body, "'unit_of_measurement':") {
		t.Errorf("body = %s, want unit_of_measurement (from s.attributes) in the reply payload", body)
	}
	if !strings.Contains(body, "s.attributes.get('device_class')") || !strings.Contains(body, "'device_class':") {
		t.Errorf("body = %s, want device_class (from s.attributes) in the reply payload", body)
	}
}

func TestGenerateInstanceAutomationTreesWritesInquiryAutomationForEveryInstance(t *testing.T) {
	admin := newAdministrationState()
	instances := map[string]THomeAssistantInstance{
		"main":               {Name: "junglinster"},
		"protocols-server-2": {Name: "protocols-server-2"},
	}

	root := t.TempDir()
	haOutputDir := filepath.Join(root, "hass", "junglinster")

	if err := generateInstanceAutomationTrees(haOutputDir, instances, map[string]THassBridgeDevice{}, admin); err != nil {
		t.Fatalf("generateInstanceAutomationTrees error: %v", err)
	}

	mainInquire := filepath.Join(haOutputDir, "automation", "infrastructural", "automation.coordinator_bridge_inquire.yaml")
	if _, err := os.Stat(mainInquire); err != nil {
		t.Errorf("expected an inquiry automation for \"main\" even with no hassbridge devices positioned yet: %v", err)
	}
	body, err := os.ReadFile(mainInquire)
	if err != nil {
		t.Fatalf("reading main's inquiry automation: %v", err)
	}
	if !strings.Contains(string(body), "homeassistant_instances/main/inquire") {
		t.Errorf("main's inquiry automation = %q, want it scoped to main's own topic", body)
	}

	remoteInquire := filepath.Join(root, "hass", "protocols-server-2", "automation", "infrastructural", "automation.coordinator_bridge_inquire.yaml")
	if _, err := os.Stat(remoteInquire); err != nil {
		t.Errorf("expected an inquiry automation for protocols-server-2 too: %v", err)
	}
}
