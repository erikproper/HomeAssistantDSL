package main

import "testing"

// TestDomainCommandsCompleteness guards against a copy-paste gap in the table: every FIXED-payload
// command (ValueTrigger false) must have all four fields set, and every DiscoveryKey/Payload pair
// must be unique within its domain (HA's discovery config would otherwise silently let one
// command's field clobber another's). A ValueTrigger command (e.g. "number"'s set_value) is the
// deliberate exception -- Payload/DiscoveryKey are BOTH expected empty for it (commandSpec's own
// doc comment), so it's checked only for Name/Service and skipped from the uniqueness checks
// entirely (an empty Payload/DiscoveryKey isn't "clobbering" anything).
func TestDomainCommandsCompleteness(t *testing.T) {
	for domain, commands := range domainCommands {
		seenKeys := map[string]bool{}
		seenPayloads := map[string]bool{}
		for _, command := range commands {
			if command.Name == "" || command.Service == "" {
				t.Errorf("domain %q: command %+v has an empty Name/Service", domain, command)
			}
			if command.ValueTrigger {
				if command.Payload != "" || command.DiscoveryKey != "" {
					t.Errorf("domain %q: ValueTrigger command %+v must leave Payload/DiscoveryKey both empty", domain, command)
				}
				continue
			}
			if command.Payload == "" || command.DiscoveryKey == "" {
				t.Errorf("domain %q: command %+v has an empty field", domain, command)
			}
			if seenKeys[command.DiscoveryKey] {
				t.Errorf("domain %q: DiscoveryKey %q used by more than one command", domain, command.DiscoveryKey)
			}
			seenKeys[command.DiscoveryKey] = true
			if seenPayloads[command.Payload] {
				t.Errorf("domain %q: Payload %q used by more than one command -- commands on a shared command_topic are discriminated by payload, so this pair would be ambiguous", domain, command.Payload)
			}
			seenPayloads[command.Payload] = true
		}
	}
}

// TestApplyStatePayloadTemplateIdentityForUndeclaredDomain confirms every domain not in
// domainStatePayloadTemplate keeps today's atomic (bare-string) reporting behaviour unchanged --
// in particular "switch", commandable but not JSON-shaped.
func TestApplyStatePayloadTemplateIdentityForUndeclaredDomain(t *testing.T) {
	got := applyStatePayloadTemplate("switch", "states('switch.foo')")
	want := "states('switch.foo')"
	if got != want {
		t.Errorf("applyStatePayloadTemplate(switch, ...) = %q, want %q (identity)", got, want)
	}
}

// TestApplyStatePayloadTemplateWrapsVacuumState is a regression test for the real gap found live
// 2026-09-09: MQTT vacuum's state_topic payload must already be a JSON object carrying a "state"
// key (homeassistant/components/mqtt/vacuum.py's own _state_message_received), unlike every other
// bridged domain's bare-value payload.
func TestApplyStatePayloadTemplateWrapsVacuumState(t *testing.T) {
	got := applyStatePayloadTemplate("vacuum", "states('vacuum.roomba')")
	want := "{'state': (states('vacuum.roomba'))} | tojson"
	if got != want {
		t.Errorf("applyStatePayloadTemplate(vacuum, ...) = %q, want %q", got, want)
	}
}

// TestBuildHassBridgeStatePayloadNoPositionIsIdentity confirms a capability with no Position
// declared keeps today's behaviour completely unchanged -- just applyStatePayloadTemplate's own
// result, one trigger entity.
func TestBuildHassBridgeStatePayloadNoPositionIsIdentity(t *testing.T) {
	cap := THassBridgeCapability{Domain: "switch"}
	payload, triggers := buildHassBridgeStatePayload(cap, "ha2mqtt", "states('switch.foo')", "switch.foo")
	if payload != "states('switch.foo')" {
		t.Errorf("payload = %q, want the atomic expression unchanged", payload)
	}
	if len(triggers) != 1 || triggers[0] != "switch.foo" {
		t.Errorf("triggers = %v, want just [switch.foo]", triggers)
	}
}

// TestBuildHassBridgeStatePayloadFoldsPositionIntoAtomicDomain is the regression test for cover's
// own set_position/current_position feature (2026-09-25): an atomic-payload domain (cover has no
// domainStatePayloadTemplate entry) with Position declared must produce a combined
// {'state': ..., 'position': ...} JSON dict, and both the state's own and the position's own
// trigger entities (when they differ) must be watched.
func TestBuildHassBridgeStatePayloadFoldsPositionIntoAtomicDomain(t *testing.T) {
	cap := THassBridgeCapability{
		Domain:   "cover",
		Position: map[string]string{"ha2mqtt": "cover.living_room_front!current_position"},
	}
	payload, triggers := buildHassBridgeStatePayload(cap, "ha2mqtt", "states('cover.living_room_front')", "cover.living_room_front")

	want := "{'state': (states('cover.living_room_front')), 'position': (state_attr('cover.living_room_front', 'current_position'))} | tojson"
	if payload != want {
		t.Errorf("payload = %q, want %q", payload, want)
	}
	// Same entity for both state and position (the common case) -- must NOT be duplicated.
	if len(triggers) != 1 || triggers[0] != "cover.living_room_front" {
		t.Errorf("triggers = %v, want just [cover.living_room_front] (deduplicated, not two entries for the same entity)", triggers)
	}
}

// TestBuildHassBridgeStatePayloadPositionOnDifferentEntity confirms a position source on a
// genuinely DIFFERENT entity than the capability's own state adds a second, distinct trigger --
// not assumed to always collapse to one, even though every real Somfy case today does.
func TestBuildHassBridgeStatePayloadPositionOnDifferentEntity(t *testing.T) {
	cap := THassBridgeCapability{
		Domain:   "cover",
		Position: map[string]string{"ha2mqtt": "sensor.living_room_front_target_closure"},
	}
	_, triggers := buildHassBridgeStatePayload(cap, "ha2mqtt", "states('cover.living_room_front')", "cover.living_room_front")

	if len(triggers) != 2 {
		t.Fatalf("triggers = %v, want both the state's own and the position's own distinct entities", triggers)
	}
	if triggers[0] != "cover.living_room_front" || triggers[1] != "sensor.living_room_front_target_closure" {
		t.Errorf("triggers = %v, want [cover.living_room_front, sensor.living_room_front_target_closure]", triggers)
	}
}

// TestBuildHassBridgeStatePayloadNoPositionForThisInstance confirms a capability with Position
// declared for a DIFFERENT instance only (a roaming-style declaration this instance never
// contributed to) reports plain state, matching every other per-instance Sources/Attributes lookup
// in this codebase.
func TestBuildHassBridgeStatePayloadNoPositionForThisInstance(t *testing.T) {
	cap := THassBridgeCapability{
		Domain:   "cover",
		Position: map[string]string{"other-instance": "cover.other!current_position"},
	}
	payload, _ := buildHassBridgeStatePayload(cap, "ha2mqtt", "states('cover.living_room_front')", "cover.living_room_front")
	if payload != "states('cover.living_room_front')" {
		t.Errorf("payload = %q, want plain state -- no Position declared for instance %q", payload, "ha2mqtt")
	}
}
