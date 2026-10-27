package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestAutoMaterializedFullNameOmitsCore covers the "core"-omission rule (2026-09-27): a bare
// "core" label produces no trailing segment at all, matching what an explicit "entity ...: from
// core;" positioning would already omit elsewhere.
func TestAutoMaterializedFullNameOmitsCore(t *testing.T) {
	got := autoMaterializedFullName("light", "light", TEntityIdentity{Sphere: "physical", Path: "sensors.server_room_main_spots"}, "core")
	want := "light.physical/sensors.server_room_main_spots"
	if got != want {
		t.Errorf("autoMaterializedFullName(...) = %q, want %q", got, want)
	}
}

// TestAutoMaterializedFullNameShortensDomainMatchingTail covers the domain-tail-shortening rule
// (2026-09-27): a label whose own leading word duplicates the domain (e.g. "light_2" under domain
// "light") shortens to just the remainder.
func TestAutoMaterializedFullNameShortensDomainMatchingTail(t *testing.T) {
	got := autoMaterializedFullName("light", "light", TEntityIdentity{Sphere: "physical", Path: "switches.zwave_048"}, "light_2")
	want := "light.physical/switches.zwave_048/2"
	if got != want {
		t.Errorf("autoMaterializedFullName(...) = %q, want %q", got, want)
	}
}

// TestAutoMaterializedFullNameKeepsUnrelatedLabel covers the ordinary case: a label with no
// relationship to the domain is kept verbatim (e.g. node 67's own "core_2"/"carport"-style labels
// that don't happen to start with the domain word).
func TestAutoMaterializedFullNameKeepsUnrelatedLabel(t *testing.T) {
	got := autoMaterializedFullName("light", "light", TEntityIdentity{Sphere: "physical", Path: "switches.zwave_067"}, "core_2")
	want := "light.physical/switches.zwave_067/core_2"
	if got != want {
		t.Errorf("autoMaterializedFullName(...) = %q, want %q", got, want)
	}
}

// TestSwitchedByEndToEndServerRoomCorridor is the core regression test for the whole "is switched
// by" mechanism, modelled directly on the real server_room/corridor pilot (node 48): a device
// (the Zigbee2MQTT light group, XX) with a Logical.def "core is switched by light.light_2 from
// <zwave-device>" fact, positioned in Conceptual.def ONLY in the "social" sphere (never an
// explicit "physical" sphere line) -- the entire point of this migration is that neither XX's own
// nor YY's own raw "physical" entity needs a hand-written Conceptual.def line any more.
func TestSwitchedByEndToEndServerRoomCorridor(t *testing.T) {
	const dsl = `space social:server_room as area with:
  device sensors.server_room_main_spots with:
    entity light.social:main from light.core;
  end;
end;`

	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"sensors.server_room_main_spots": {
			DeviceID:    "sensors.server_room_main_spots",
			Identifiers: []string{"zigbee2mqtt_group_1"},
			Capabilities: map[string]TDiscoveryCapability{
				"core": {Domain: "light", Leaf: "1_light_zigbee2mqtt"},
			},
		},
		"switches.house_server_room_fuse_box_zwave_048": {
			DeviceID:    "switches.house_server_room_fuse_box_zwave_048",
			Identifiers: []string{"zwavejs2mqtt_0xd0204fb2_node48"},
			Capabilities: map[string]TDiscoveryCapability{
				"light_1": {Domain: "light", Leaf: "48-37-1-currentValue"},
				"light_2": {Domain: "light", Leaf: "48-37-2-currentValue"},
			},
		},
	}

	logicalDevicesByID := map[string]TLogicalDevice{
		"sensors.server_room_main_spots": {
			DeviceID:     "sensors.server_room_main_spots",
			Capabilities: map[string]TLogicalCapability{},
			SwitchedBy: map[string]TLogicalSwitchedByFact{
				"core": {OwnDomain: "light", SourceDomain: "light", SourceLabel: "light_2", SourceDeviceID: "switches.house_server_room_fuse_box_zwave_048"},
			},
		},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, discoveryGatewaysByID, nil, nil, nil, nil, logicalDevicesByID, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration

	if len(admin.SwitchedDeviceRelations) != 1 {
		t.Fatalf("got %d SwitchedDeviceRelations, want 1 (%v)", len(admin.SwitchedDeviceRelations), admin.SwitchedDeviceRelations)
	}
	rel := admin.SwitchedDeviceRelations[0]

	selfID := toHomeAssistantEntityID(rel.SelfEntity)
	deviceID := toHomeAssistantEntityID(rel.Device)
	mainID := toHomeAssistantEntityID(rel.MainEntity)

	if !strings.Contains(selfID, "server_room_main") {
		t.Errorf("SelfEntity resolved to %q, want it to contain \"server_room_main\"", selfID)
	}
	if deviceID == selfID {
		t.Fatalf("Device (%q) must NOT equal SelfEntity (%q) -- a self-referential group", deviceID, selfID)
	}
	if mainID == deviceID {
		t.Fatalf("MainEntity (%q) must NOT equal Device (%q) -- they're two distinct physical entities", mainID, deviceID)
	}
	if strings.HasSuffix(deviceID, "_core") {
		t.Errorf("Device = %q, want the trailing \"_core\" omitted (the \"core\"-omission rule)", deviceID)
	}
	if !strings.HasSuffix(mainID, "_2") || strings.Contains(mainID, "light_2") {
		t.Errorf("MainEntity = %q, want it to end in \"_2\" with \"light_2\" shortened away", mainID)
	}
	if !strings.HasPrefix(deviceID, "light.physical_") || !strings.HasPrefix(mainID, "light.physical_") {
		t.Errorf("Device/MainEntity = %q / %q, want both auto-materialized under the \"physical\" sphere", deviceID, mainID)
	}
}

// TestSwitchedBySelfEntityFeedsSpaceLightAggregate is the regression test for a real bug found
// live 2026-09-28 (Junglinster's kitchen-workplace migration, node 58): a switched-by SelfEntity
// registered into administration.SpaceLightsByName too LATE to feed that space's own room-level
// light aggregate (generateTemplateLights, generator.go -- only writes one when SpaceOnByName ends
// up non-empty, which falls back to SpaceLightsByName absent an explicit "light on:"/"light off:"
// override). The space's own "apply defaults" snapshot (HandleEndToken, administration.go) runs
// SYNCHRONOUSLY the moment that space's "end;" is parsed, on the ordinary first pass -- but
// resolveSwitchedByEndpoint's own deferral (added earlier this same session, for the "prefer an
// explicit physical positioning" fix) routinely pushes Device/MainEntity resolution to the LATER
// finalAttempt-only retry pass whenever neither side has an explicit physical positioning yet (the
// overwhelmingly common case) -- and the OLD code only registered the SelfEntity's own existence
// into EntityRecordsBySpace/SpaceLightsByName AFTER that later resolution succeeded, missing the
// space's already-run defaults snapshot entirely. Every switched-by site migrated before this fix
// happened to have an explicit "light on:" override already in place (server_room/corridor,
// living/dining), masking the bug until kitchen workplace -- the first site without one. Confirmed
// fixed by asserting SpaceLightsByName contains the SelfEntity's own name using the EXACT SAME
// DSL/fixtures as TestSwitchedByEndToEndServerRoomCorridor (no "light on:" line there either).
func TestSwitchedBySelfEntityFeedsSpaceLightAggregate(t *testing.T) {
	const dsl = `space social:server_room as area with:
  device sensors.server_room_main_spots with:
    entity light.social:main from light.core;
  end;
end;`

	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"sensors.server_room_main_spots": {
			DeviceID:    "sensors.server_room_main_spots",
			Identifiers: []string{"zigbee2mqtt_group_1"},
			Capabilities: map[string]TDiscoveryCapability{
				"core": {Domain: "light", Leaf: "1_light_zigbee2mqtt"},
			},
		},
		"switches.house_server_room_fuse_box_zwave_048": {
			DeviceID:    "switches.house_server_room_fuse_box_zwave_048",
			Identifiers: []string{"zwavejs2mqtt_0xd0204fb2_node48"},
			Capabilities: map[string]TDiscoveryCapability{
				"light_1": {Domain: "light", Leaf: "48-37-1-currentValue"},
				"light_2": {Domain: "light", Leaf: "48-37-2-currentValue"},
			},
		},
	}

	logicalDevicesByID := map[string]TLogicalDevice{
		"sensors.server_room_main_spots": {
			DeviceID:     "sensors.server_room_main_spots",
			Capabilities: map[string]TLogicalCapability{},
			SwitchedBy: map[string]TLogicalSwitchedByFact{
				"core": {OwnDomain: "light", SourceDomain: "light", SourceLabel: "light_2", SourceDeviceID: "switches.house_server_room_fuse_box_zwave_048"},
			},
		},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, discoveryGatewaysByID, nil, nil, nil, nil, logicalDevicesByID, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration

	if len(admin.SwitchedDeviceRelations) != 1 {
		t.Fatalf("got %d SwitchedDeviceRelations, want 1 (%v)", len(admin.SwitchedDeviceRelations), admin.SwitchedDeviceRelations)
	}
	selfID := toHomeAssistantEntityID(admin.SwitchedDeviceRelations[0].SelfEntity)

	var found bool
	for _, lights := range admin.SpaceLightsByName {
		for _, name := range lights {
			if toHomeAssistantEntityID(name) == selfID {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("SelfEntity %q not found in any SpaceLightsByName entry -- the room-level light aggregate for its space will never be generated", selfID)
	}
}

// TestSwitchedByReusesExplicitPhysicalPositioning is the regression test for a real bug found live
// 2026-09-28, right after deploying and restarting Junglinster with the corridor/server_room pilot:
// corridor's switch (YY, switches.house_server_room_fuse_box_zwave_048's own "light_1" capability)
// keeps a genuine, deliberately-authored explicit "physical" sphere positioning (Conceptual.def,
// "device switches.house_server_room_fuse_box_zwave_048 with entity light.physical:main from
// light.light_1 with no_collect;") for an unrelated pre-existing "follows" automation reference.
// Before this fix, the switched-by mechanism unconditionally ALSO auto-materialized its own,
// separately-named synthetic entity for the SAME (deviceID, label) -- and since the coordinator's
// own MQTT discovery topic identity is keyed by (gatewayID, leaf) alone (discovery.go's
// discoveryTopic), not by which Conceptual.def positioning declared it, the two publishes collided
// into ONE MQTT topic; whichever config happened to publish last silently won, and the OTHER
// positioning's own entity simply never existed live -- corridor's switched-light group ended up
// "unavailable" because its helper's own member entity was missing. The user's own correction: an
// explicit positioning always takes precedence over an auto-materialized "hidden" one for the same
// capability -- confirmed here by asserting MainEntity resolves to the EXPLICIT positioning's own
// entity_id (not a "_1"/"switches_..." shaped auto-materialized one), and that exactly ONE
// DiscoveryEntityLinks entry exists for the shared (gateway, leaf) pair, not two competing ones.
//
// Line order matters here, deliberately matching the REAL file's own order (Conceptual.def: XX's
// "social:main" positioning at line 1084, YY's explicit "physical:main" positioning LATER at line
// 1092) -- an earlier version of this test put the explicit positioning FIRST, which happened to
// mask the real bug entirely (ConceptualUseBySource already had it by the time XX's line ran, no
// deferral ever exercised). With XX's line first, resolveSwitchedByEndpoint must actually defer
// (not immediately auto-materialize) until the deferred-retry pass has seen YY's own line too.
func TestSwitchedByReusesExplicitPhysicalPositioning(t *testing.T) {
	const dsl = `space social:corridor as area with:
  device sensors.corridor_main_spots with:
    entity light.social:main from light.core;
  end;

  device switches.house_server_room_fuse_box_zwave_048 with entity light.physical:main from light.light_1 with no_collect;
end;`

	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"sensors.corridor_main_spots": {
			DeviceID:    "sensors.corridor_main_spots",
			Identifiers: []string{"zigbee2mqtt_group_4"},
			Capabilities: map[string]TDiscoveryCapability{
				"core": {Domain: "light", Leaf: "4_light_zigbee2mqtt"},
			},
		},
		"switches.house_server_room_fuse_box_zwave_048": {
			DeviceID:    "switches.house_server_room_fuse_box_zwave_048",
			Identifiers: []string{"zwavejs2mqtt_0xd0204fb2_node48"},
			Capabilities: map[string]TDiscoveryCapability{
				"light_1": {Domain: "light", Leaf: "48-37-1-currentValue"},
			},
		},
	}

	logicalDevicesByID := map[string]TLogicalDevice{
		"sensors.corridor_main_spots": {
			DeviceID:     "sensors.corridor_main_spots",
			Capabilities: map[string]TLogicalCapability{},
			SwitchedBy: map[string]TLogicalSwitchedByFact{
				"core": {OwnDomain: "light", SourceDomain: "light", SourceLabel: "light_1", SourceDeviceID: "switches.house_server_room_fuse_box_zwave_048"},
			},
		},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, discoveryGatewaysByID, nil, nil, nil, nil, logicalDevicesByID, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration

	if len(admin.SwitchedDeviceRelations) != 1 {
		t.Fatalf("got %d SwitchedDeviceRelations, want 1 (%v)", len(admin.SwitchedDeviceRelations), admin.SwitchedDeviceRelations)
	}
	mainID := toHomeAssistantEntityID(admin.SwitchedDeviceRelations[0].MainEntity)

	const wantMainID = "light.physical_corridor_main"
	if mainID != wantMainID {
		t.Errorf("MainEntity = %q, want %q -- the EXPLICIT positioning's own entity must be reused, not a separately auto-materialized one", mainID, wantMainID)
	}

	var matches int
	for _, link := range admin.DiscoveryEntityLinks {
		if link.GatewayDeviceID == "switches.house_server_room_fuse_box_zwave_048" && link.Leaf == "48-37-1-currentValue" {
			matches++
		}
	}
	if matches != 1 {
		t.Errorf("got %d DiscoveryEntityLinks for the shared (gateway, leaf) pair, want exactly 1 -- the explicit positioning and the switched-by mechanism must resolve to the SAME entity, never two competing ones", matches)
	}
}

// TestSwitchedByGeneratedAutomationIDHasNoEmbeddedDot is the regression test for a real bug found
// live regenerating Junglinster's server_room/corridor pilot: TSwitchedDeviceRelation.Device/
// .MainEntity DELIBERATELY keep a raw DSL device-id's own dot intact (e.g.
// "switches.house_server_room_fuse_box_zwave_048") -- matching autoMaterializePhysicalCapability's
// own synthetic-identity convention exactly, since RunPostParseChecks (checks.go) compares these
// fields verbatim against whatever got registered as that entity's own Name; sanitizing the dot
// only in the relation would silently break that lookup even though the FINAL entity_id still
// matches. The real fix belongs in generateSwitchedDeviceAutomations (generator.go) itself, which
// used to build its own automation id via a naive slash-only replacement that let an embedded dot
// leak straight into automation.<id> verbatim -- this test exercises that generator function
// directly (not just the relation) to confirm the actual generated id is clean.
func TestSwitchedByGeneratedAutomationIDHasNoEmbeddedDot(t *testing.T) {
	const dsl = `space social:server_room as area with:
  device sensors.server_room_main_spots with:
    entity light.social:main from light.core;
  end;
end;`

	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"sensors.server_room_main_spots": {
			DeviceID:     "sensors.server_room_main_spots",
			Identifiers:  []string{"zigbee2mqtt_group_1"},
			Capabilities: map[string]TDiscoveryCapability{"core": {Domain: "light", Leaf: "1_light_zigbee2mqtt"}},
		},
		"switches.house_server_room_fuse_box_zwave_048": {
			DeviceID:     "switches.house_server_room_fuse_box_zwave_048",
			Identifiers:  []string{"zwavejs2mqtt_0xd0204fb2_node48"},
			Capabilities: map[string]TDiscoveryCapability{"light_2": {Domain: "light", Leaf: "48-37-2-currentValue"}},
		},
	}
	logicalDevicesByID := map[string]TLogicalDevice{
		"sensors.server_room_main_spots": {
			DeviceID:     "sensors.server_room_main_spots",
			Capabilities: map[string]TLogicalCapability{},
			SwitchedBy: map[string]TLogicalSwitchedByFact{
				"core": {OwnDomain: "light", SourceDomain: "light", SourceLabel: "light_2", SourceDeviceID: "switches.house_server_room_fuse_box_zwave_048"},
			},
		},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, discoveryGatewaysByID, nil, nil, nil, nil, logicalDevicesByID, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(result.Administration.SwitchedDeviceRelations) != 1 {
		t.Fatalf("got %d SwitchedDeviceRelations, want 1", len(result.Administration.SwitchedDeviceRelations))
	}
	// The relation itself DELIBERATELY keeps the raw device-id's own dot (see this test's own doc
	// comment) -- only the GENERATED automation id must come out clean.
	outDir := t.TempDir()
	if err := generateSwitchedDeviceAutomations(outDir, result.Administration); err != nil {
		t.Fatalf("generateSwitchedDeviceAutomations: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(outDir, "automation", "*", "automation.*switched_on.yaml"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("got %d switched_on automation file(s), want 1: %v", len(matches), matches)
	}
	base := filepath.Base(matches[0])
	if strings.Count(base, ".") != 2 { // "automation" . "<id>" . "yaml" -- exactly two dots total
		t.Errorf("generated automation filename = %q, want no embedded dot in <id> (exactly the 2 structural dots)", base)
	}
}

// TestSwitchedBySuppressesCompetingDiscoveryEntity is the regression test for a real bug found live
// 2026-09-28 (Junglinster's server_room/corridor deploy): the SAME "entity light.social:main from
// light.core;" positioning that triggers the "is switched by" SelfEntity relation used to ALSO,
// independently, register an ordinary raw discovery entity under that exact same target name
// (registerDiscoveryEntityLink, via registerDeviceCapabilityEntityLinkAllowingHiddenCore's ordinary
// discovery branch) -- a genuine collision against generateSwitchedDeviceEntities' own generated
// "platform: group" entity for the identical entity_id. Live, the raw discovery entity won the
// name, HA bumped it to a "_2" suffix, and the intended switched-light group never registered under
// its real name at all. switchedBySuppressesEntity (logical_switched_by.go) now short-circuits
// Core's own discovery/local branches for exactly this positioning -- confirmed here by asserting
// DiscoveryEntityLinks holds NO entry for the SelfEntity's own resolved entity_id, while the
// SwitchedDeviceRelations entry (the thing that's actually supposed to own this name) still exists.
func TestSwitchedBySuppressesCompetingDiscoveryEntity(t *testing.T) {
	const dsl = `space social:server_room as area with:
  device sensors.server_room_main_spots with:
    entity light.social:main from light.core;
  end;
end;`

	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"sensors.server_room_main_spots": {
			DeviceID:    "sensors.server_room_main_spots",
			Identifiers: []string{"zigbee2mqtt_group_1"},
			Capabilities: map[string]TDiscoveryCapability{
				"core": {Domain: "light", Leaf: "1_light_zigbee2mqtt"},
			},
		},
		"switches.house_server_room_fuse_box_zwave_048": {
			DeviceID:    "switches.house_server_room_fuse_box_zwave_048",
			Identifiers: []string{"zwavejs2mqtt_0xd0204fb2_node48"},
			Capabilities: map[string]TDiscoveryCapability{
				"light_2": {Domain: "light", Leaf: "48-37-2-currentValue"},
			},
		},
	}

	logicalDevicesByID := map[string]TLogicalDevice{
		"sensors.server_room_main_spots": {
			DeviceID:     "sensors.server_room_main_spots",
			Capabilities: map[string]TLogicalCapability{},
			SwitchedBy: map[string]TLogicalSwitchedByFact{
				"core": {OwnDomain: "light", SourceDomain: "light", SourceLabel: "light_2", SourceDeviceID: "switches.house_server_room_fuse_box_zwave_048"},
			},
		},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, discoveryGatewaysByID, nil, nil, nil, nil, logicalDevicesByID, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration

	if len(admin.SwitchedDeviceRelations) != 1 {
		t.Fatalf("got %d SwitchedDeviceRelations, want 1 (%v)", len(admin.SwitchedDeviceRelations), admin.SwitchedDeviceRelations)
	}
	selfID := toHomeAssistantEntityID(admin.SwitchedDeviceRelations[0].SelfEntity)

	if _, collided := admin.DiscoveryEntityLinks[selfID]; collided {
		t.Errorf("DiscoveryEntityLinks has a competing raw entry for %q -- the SelfEntity positioning's own discovery-entity generation should have been suppressed in favour of the switched-by group", selfID)
	}
}

// TestSwitchedByDeviceWithBothSelfEntityAndExplicitPhysicalPositioning is the regression test for a
// real bug found live 2026-09-28 (Junglinster's node 15 living/dining migration): XX's own dimmer
// capability (light.core) gets BOTH the switched-by SelfEntity trigger (light.social:main) AND,
// separately, a deliberate explicit "physical" sphere positioning of the SAME capability
// (light.physical:main, kept stable for two pre-existing non-DSL "dim up/down" automations that
// target it directly) -- a genuinely new scenario the server_room/corridor pilot never exercised
// (there, only YY -- the switch source -- ever got a kept explicit positioning, never XX itself).
// Before the fix, maybeRegisterSwitchedByRelation's own bookkeeping registration
// (RegisterDiscoveryImpliedEntity, called to satisfy "follows"/"light on:" declared-entity checks)
// used the bare "deviceID!capability" sourceKey -- identical to what the explicit "physical"
// positioning's own ordinary registration uses -- so the two collided, firing a false-positive
// "already used as a different conceptual entity" warning even though both entities are
// legitimate and intentional. Confirmed fixed by asserting that warning does NOT fire.
func TestSwitchedByDeviceWithBothSelfEntityAndExplicitPhysicalPositioning(t *testing.T) {
	const dsl = `space social:living as area with:
  device sensors.living_room_living_main_dimmer with:
    entity light.social:main from light.core;
    entity light.physical:main from light.core;
  end;
end;`

	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"sensors.living_room_living_main_dimmer": {
			DeviceID:    "sensors.living_room_living_main_dimmer",
			Identifiers: []string{"zigbee2mqtt_0x0c2a6ffffe5bf9c2"},
			Capabilities: map[string]TDiscoveryCapability{
				"core": {Domain: "light", Leaf: "0x0c2a6ffffe5bf9c2_light_zigbee2mqtt"},
			},
		},
		"switches.house_living_room_hallway_door_switch_box_zwave_015": {
			DeviceID:    "switches.house_living_room_hallway_door_switch_box_zwave_015",
			Identifiers: []string{"zwavejs2mqtt_0xd0204fb2_node15"},
			Capabilities: map[string]TDiscoveryCapability{
				"light_1": {Domain: "light", Leaf: "15-37-1-currentValue"},
			},
		},
	}

	logicalDevicesByID := map[string]TLogicalDevice{
		"sensors.living_room_living_main_dimmer": {
			DeviceID:     "sensors.living_room_living_main_dimmer",
			Capabilities: map[string]TLogicalCapability{},
			SwitchedBy: map[string]TLogicalSwitchedByFact{
				"core": {OwnDomain: "light", SourceDomain: "light", SourceLabel: "light_1", SourceDeviceID: "switches.house_living_room_hallway_door_switch_box_zwave_015"},
			},
		},
	}

	var report strings.Builder
	var err error
	output := captureStderr(t, func() {
		_, err = ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, discoveryGatewaysByID, nil, nil, nil, nil, logicalDevicesByID, nil)
	})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if strings.Contains(output, "already used as a different conceptual entity") {
		t.Errorf("false-positive duplicate-conceptual-entity warning fired:\n%s", output)
	}
}

// TestSwitchedByNoOpWithoutFact covers the cheap-no-op path: an ordinary capability registration
// for a device with NO SwitchedBy fact at all must not touch SwitchedDeviceRelations.
func TestSwitchedByNoOpWithoutFact(t *testing.T) {
	const dsl = `space social:living_room as area with:
  device sensors.living_room_indirect with:
    entity light.social:indirect from light.core;
  end;
end;`

	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"sensors.living_room_indirect": {
			DeviceID:    "sensors.living_room_indirect",
			Identifiers: []string{"zigbee2mqtt_0xabc"},
			Capabilities: map[string]TDiscoveryCapability{
				"core": {Domain: "light", Leaf: "0xabc_light_zigbee2mqtt"},
			},
		},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, discoveryGatewaysByID, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(result.Administration.SwitchedDeviceRelations) != 0 {
		t.Errorf("expected no SwitchedDeviceRelations for a device with no \"is switched by\" fact, got %v", result.Administration.SwitchedDeviceRelations)
	}
}

// TestSwitchCoercedIntoLight: a "switch" capability switching a light gets the light role, and its
// own "switch_" label prefix is still shortened -- the name is the one a "light_2" label gave.
func TestSwitchCoercedIntoLight(t *testing.T) {
	if got := coercedDomain("switch", "light"); got != "light" {
		t.Errorf("coercedDomain(switch, light) = %q", got)
	}
	if got := coercedDomain("switch", "climate"); got != "switch" {
		t.Errorf("coercedDomain(switch, climate) = %q, want switch (no such role)", got)
	}
	if got := coercedDomain("sensor", "light"); got != "sensor" {
		t.Errorf("coercedDomain(sensor, light) = %q, want sensor (only a switch is coerced)", got)
	}
	got := autoMaterializedFullName(coercedDomain("switch", "light"), "switch", TEntityIdentity{Sphere: "physical", Path: "switches.zwave_048"}, "switch_2")
	if want := "light.physical/switches.zwave_048/2"; got != want {
		t.Errorf("autoMaterializedFullName(...) = %q, want %q", got, want)
	}
}

// TestDomainNamedCapability: "from DD" with DD the entity's own domain means DD.DD; any other bare
// capability stays bare.
func TestDomainNamedCapability(t *testing.T) {
	for _, c := range []struct{ spec, capability, want string }{
		{"cover.social:front", "cover", "cover.cover"},
		{"cover.social:", "cover", "cover.cover"},
		{"light.social:main", "core", "core"},
		{"light.social:main", "switch.switch_1", "switch.switch_1"},
	} {
		if got := domainNamedCapability(c.spec, c.capability); got != c.want {
			t.Errorf("domainNamedCapability(%q, %q) = %q, want %q", c.spec, c.capability, got, c.want)
		}
	}
}
