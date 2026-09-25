package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEntityCommandAutomationBody(t *testing.T) {
	identity := TEntityIdentity{Domain: "vacuum", Sphere: "social", Path: "roomba"}
	command := commandSpec{Name: "start", Service: "vacuum.start", Payload: "start", DiscoveryKey: "payload_start"}
	body := entityCommandAutomationBody(identity, command, "vacuum.roomba", "homeassistant_instances/protocols-server-2/bridge/vacuum.social_roomba/command")

	if !strings.Contains(body, `alias: "command/vacuum/social/roomba/start"`) {
		t.Errorf("body = %s, want the slash-joined alias with the command name appended", body)
	}
	if !strings.Contains(body, "platform: mqtt") {
		t.Errorf("body = %s, want an mqtt trigger", body)
	}
	if !strings.Contains(body, `topic: "homeassistant_instances/protocols-server-2/bridge/vacuum.social_roomba/command"`) {
		t.Errorf("body = %s, want the given shared command topic", body)
	}
	if !strings.Contains(body, `payload: "start"`) {
		t.Errorf("body = %s, want an exact-match payload filter on the trigger", body)
	}
	if !strings.Contains(body, "service: vacuum.start") {
		t.Errorf("body = %s, want the command's own hardcoded service call", body)
	}
	if !strings.Contains(body, "entity_id: vacuum.roomba") {
		t.Errorf("body = %s, want the action targeting the remote instance's own real entity_id", body)
	}
}

// TestEntityCommandAutomationBodyValueTrigger is the regression test for "number"'s own
// set_value command (2026-09-25, the Overkiz/Somfy migration): its automation must have NO
// "payload:" filter on the trigger (any message is a genuine new value, not one of several
// discriminated commands) and must pass that value through as the service call's own "value" data
// field, not a bare no-data call.
func TestEntityCommandAutomationBodyValueTrigger(t *testing.T) {
	identity := TEntityIdentity{Domain: "number", Sphere: "social", Path: "front/my_position"}
	command := commandSpec{Name: "set_value", Service: "number.set_value", ValueTrigger: true}
	body := entityCommandAutomationBody(identity, command, "number.living_room_front_my_position", "homeassistant_instances/protocols-server-2/bridge/number.social_front_my_position/command")

	if strings.Contains(body, "payload:") {
		t.Errorf("body = %s, want no payload filter at all on a ValueTrigger command", body)
	}
	if !strings.Contains(body, "service: number.set_value") {
		t.Errorf("body = %s, want the command's own hardcoded service call", body)
	}
	if !strings.Contains(body, "entity_id: number.living_room_front_my_position") {
		t.Errorf("body = %s, want the action targeting the remote instance's own real entity_id", body)
	}
	if !strings.Contains(body, `value: "{{ trigger.payload }}"`) {
		t.Errorf("body = %s, want the triggered MQTT payload passed through as the service's own \"value\" data field", body)
	}
}

// TestEntityCommandAutomationBodyCustomDataKey is the regression test for cover's own
// set_position command (2026-09-25): its own commandSpec.DataKey ("position") must be used as the
// service call's own data field name, not "value" (number.set_value's own default) -- a plain
// hardcoded "value" would call cover.set_cover_position with the wrong parameter entirely.
func TestEntityCommandAutomationBodyCustomDataKey(t *testing.T) {
	identity := TEntityIdentity{Domain: "cover", Sphere: "social", Path: "front"}
	command := commandSpec{Name: "set_position", Service: "cover.set_cover_position", ValueTrigger: true, DataKey: "position", TopicKey: "set_position_topic"}
	body := entityCommandAutomationBody(identity, command, "cover.living_room_front", "homeassistant_instances/ha2mqtt/bridge/cover.social_front/command/set_position")

	if !strings.Contains(body, `position: "{{ trigger.payload }}"`) {
		t.Errorf("body = %s, want the triggered payload passed through as the command's own \"position\" data field", body)
	}
	if strings.Contains(body, "value:") {
		t.Errorf("body = %s, want no \"value\" data field at all -- DataKey overrides the default", body)
	}
}

// TestHassBridgeEntityCommandTopicFor is the regression test for the topic-selection rule itself
// (2026-09-25, added alongside cover's own set_position): a fixed-payload command always gets the
// shared base topic; a ValueTrigger command always gets its own dedicated "<base>/<name>" topic,
// regardless of domain -- so a fixed-payload sibling's own "OPEN"/"CLOSE"/"STOP" literal payload
// can never also match a ValueTrigger command's own "any payload" trigger on the same topic.
func TestHassBridgeEntityCommandTopicFor(t *testing.T) {
	fixed := commandSpec{Name: "open", Payload: "OPEN"}
	if got, want := hassBridgeEntityCommandTopicFor("ha2mqtt", "cover.social_front", fixed), "homeassistant_instances/ha2mqtt/bridge/cover.social_front/command"; got != want {
		t.Errorf("fixed-payload topic = %q, want the shared base topic %q", got, want)
	}

	valueTrigger := commandSpec{Name: "set_position", ValueTrigger: true}
	if got, want := hassBridgeEntityCommandTopicFor("ha2mqtt", "cover.social_front", valueTrigger), "homeassistant_instances/ha2mqtt/bridge/cover.social_front/command/set_position"; got != want {
		t.Errorf("ValueTrigger topic = %q, want its own dedicated topic %q", got, want)
	}
}

func TestGenerateHassBridgeEntityCommandAutomationsOneFilePerCommand(t *testing.T) {
	const miniDSL = `space social:apartment with:
  device hass.protocols_server_2 as roomba with:
  end;
  entity vacuum.social:roomba from hass.protocols_server_2 vacuum.roomba;
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.protocols_server_2": {
			DeviceID:  "hass.protocols_server_2",
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"roomba": {Domain: "vacuum", Sources: map[string]string{"protocols-server-2": "vacuum.roomba"}},
			},
		},
	}

	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &strings.Builder{}, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	outputDir := t.TempDir()
	if err := generateHassBridgeEntityCommandAutomations(outputDir, "protocols-server-2", hassBridgeDevicesByID, result.Administration); err != nil {
		t.Fatalf("generateHassBridgeEntityCommandAutomations error: %v", err)
	}

	for _, command := range domainCommands["vacuum"] {
		path := filepath.Join(outputDir, "automation", "infrastructural", "automation.command_vacuum_social_apartment_roomba_"+command.Name+".yaml")
		body, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("expected %s to exist: %v", path, err)
			continue
		}
		if !strings.Contains(string(body), "service: "+command.Service) {
			t.Errorf("%s = %q, want it to call %s", path, body, command.Service)
		}
		if !strings.Contains(string(body), `payload: "`+command.Payload+`"`) {
			t.Errorf("%s = %q, want an exact-match filter on payload %q", path, body, command.Payload)
		}
	}
}

// TestGenerateHassBridgeEntityCommandAutomationsSkipsReadOnlyDomain confirms a capability whose
// domain has no domainCommands entry (e.g. plain "sensor") generates no command automations at
// all -- read-only domains must stay entirely unaffected by this mechanism.
func TestGenerateHassBridgeEntityCommandAutomationsSkipsReadOnlyDomain(t *testing.T) {
	const miniDSL = `space social:david_bedroom with:
  device hass.davids_bedroom as netatmo with:
  end;
  entity sensor.physical:netatmo/co2 from hass.davids_bedroom sensor.co2;
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.davids_bedroom": {
			DeviceID:  "hass.davids_bedroom",
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"co2": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.davids_bedroom_carbon_dioxide"}},
			},
		},
	}

	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &strings.Builder{}, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	outputDir := t.TempDir()
	if err := generateHassBridgeEntityCommandAutomations(outputDir, "protocols-server-2", hassBridgeDevicesByID, result.Administration); err != nil {
		t.Fatalf("generateHassBridgeEntityCommandAutomations error: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(outputDir, "automation", "infrastructural"))
	if err == nil && len(entries) > 0 {
		t.Errorf("expected no command automations for a read-only (sensor) capability, got %v", entries)
	}
}
