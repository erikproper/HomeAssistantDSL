package main

import (
	"strings"
	"testing"
)

func TestExtractDeviceWithBlockHeader(t *testing.T) {
	decl, ok := extractDeviceWithBlockHeader("device host.xanadu as xanadu with:")
	if !ok {
		t.Fatalf("expected the header to match")
	}
	if decl.Spec != "xanadu" || decl.DeviceID != "host.xanadu" {
		t.Fatalf("got %+v", decl)
	}
	if _, ok := extractDeviceWithBlockHeader("device host.xanadu as xanadu;"); ok {
		t.Errorf("expected no match for a line with no \"with:\" at all")
	}
}

// TestExtractDeviceWithBlockHeaderNoAs confirms "as" is optional -- Spec is empty, matching a
// device positioned with no compound name of its own.
func TestExtractDeviceWithBlockHeaderNoAs(t *testing.T) {
	decl, ok := extractDeviceWithBlockHeader("device switches.server_room_zwave_054 with:")
	if !ok {
		t.Fatalf("expected the header to match")
	}
	if decl.Spec != "" || decl.DeviceID != "switches.server_room_zwave_054" {
		t.Fatalf("got %+v, want empty Spec and the bare device-id", decl)
	}
}

// TestExtractDeviceWithOneLiner covers the "with <entity-statement>;" sugar (2026-09-24), including
// the compound "as" leaf path from the user's own real example (node 67's own infrastructural
// identity).
func TestExtractDeviceWithOneLiner(t *testing.T) {
	decl, entity, ok := extractDeviceWithOneLiner("device switches.garage_fuse_cabinet_zwave_067 as fuse_cabinet/zwave/067 with entity binary_sensor.infrastructural:node;")
	if !ok {
		t.Fatalf("expected the one-liner to match")
	}
	if decl.DeviceID != "switches.garage_fuse_cabinet_zwave_067" || decl.Spec != "fuse_cabinet/zwave/067" {
		t.Fatalf("got decl=%+v", decl)
	}
	if entity != "entity binary_sensor.infrastructural:node;" {
		t.Errorf("entity = %q, want the embedded entity statement verbatim", entity)
	}
}

// TestExtractDeviceWithOneLinerNoAs covers the shortest real example from the user's own request.
func TestExtractDeviceWithOneLinerNoAs(t *testing.T) {
	decl, entity, ok := extractDeviceWithOneLiner("device switches.garage_fuse_cabinet_zwave_067 with entity binary_sensor.infrastructural:fuse_cabinet/zwave/067/node;")
	if !ok {
		t.Fatalf("expected the one-liner to match")
	}
	if decl.DeviceID != "switches.garage_fuse_cabinet_zwave_067" || decl.Spec != "" {
		t.Fatalf("got decl=%+v", decl)
	}
	if entity != "entity binary_sensor.infrastructural:fuse_cabinet/zwave/067/node;" {
		t.Errorf("entity = %q", entity)
	}
	if _, _, ok := extractDeviceWithOneLiner("device switches.X with:"); ok {
		t.Errorf("expected no match for a plain block header (no embedded entity statement)")
	}
}

// TestRegisterDevicePositioningIsIdempotentAcrossThreeOccurrences is the core regression test for
// the 2026-09-24 register-once-merge-repeatedly redesign: the SAME device-id positioned via THREE
// separate "device <id> [as ...] with:" blocks (two contributing entities in different spaces, one
// establishing the device's own infrastructural identity) -- mirroring the real Z-Wave node
// 54/55 shape this session's own migrations needed. No "already positioned" warning, and the FIRST
// occurrence's own "as" leaf (not the later ones') determines DisplayName.
func TestRegisterDevicePositioningIsIdempotentAcrossThreeOccurrences(t *testing.T) {
	const dsl = `space social:front as area with:
  device switches.zwave_054 as ring with:
    entity switch.social:main from core_1;
  end;
end;

space social:rear with:
  device switches.zwave_054 with:
    entity switch.social:rear from core_2;
  end;
end;

device switches.zwave_054 as fuse_cabinet/zwave/054 with entity binary_sensor.infrastructural:node;`

	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"switches.zwave_054": {
			DeviceID:    "switches.zwave_054",
			Identifiers: []string{"zwavejs2mqtt_054"},
			Capabilities: map[string]TDiscoveryCapability{
				"core_1": {Domain: "switch", Leaf: "054-37-1-currentValue"},
				"core_2": {Domain: "switch", Leaf: "054-37-2-currentValue"},
				"node":   {Domain: "binary_sensor", Leaf: "054-37-1-currentValue"},
			},
		},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, discoveryGatewaysByID, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if report.Len() != 0 {
		t.Logf("report (not necessarily an error): %s", report.String())
	}
	admin := result.Administration

	link, ok := admin.DeviceConceptualLinks["switches.zwave_054"]
	if !ok {
		t.Fatalf("expected a DeviceConceptualLinks entry for switches.zwave_054")
	}
	// DisplayName comes from the FIRST occurrence's own "as ring" (space "front", "as area"), not
	// the third occurrence's "as fuse_cabinet/zwave/054".
	if link.DisplayName != "front/ring" {
		t.Errorf("DisplayName = %q, want %q (from the FIRST occurrence only)", link.DisplayName, "front/ring")
	}
	// Discovery-kind capabilities never populate DeviceConceptualLinks.AttributeEntityIDs (that's a
	// hosts/hassbridge/import-only mechanism, see TestDiscoveryDeviceKeepsOwnPhysicalPositioningWhenGainingLogicalCapabilities's
	// own comment) -- they resolve via the separate DiscoveryEntityLinks map instead, keyed by
	// entity_id. Confirm all three blocks' own capabilities resolved against the SAME gateway
	// device, which is what "register-once-merge-repeatedly" actually needs to guarantee here.
	wantLeafByGatewayEntity := map[string]string{
		"switch.social_front_ring_main":                             "054-37-1-currentValue",
		"switch.social_rear_rear":                                   "054-37-2-currentValue",
		"binary_sensor.infrastructural_fuse_cabinet_zwave_054_node": "054-37-1-currentValue",
	}
	for entityID, wantLeaf := range wantLeafByGatewayEntity {
		discLink, ok := admin.DiscoveryEntityLinks[entityID]
		if !ok {
			t.Errorf("DiscoveryEntityLinks missing %q, want it resolved via switches.zwave_054", entityID)
			continue
		}
		if discLink.GatewayDeviceID != "switches.zwave_054" {
			t.Errorf("DiscoveryEntityLinks[%q].GatewayDeviceID = %q, want switches.zwave_054", entityID, discLink.GatewayDeviceID)
		}
		if discLink.Leaf != wantLeaf {
			t.Errorf("DiscoveryEntityLinks[%q].Leaf = %q, want %q", entityID, discLink.Leaf, wantLeaf)
		}
	}
	if len(admin.DiscoveryEntityLinks) != 3 {
		t.Errorf("DiscoveryEntityLinks = %v, want exactly 3 entries (core_1, core_2, node all merged from three separate blocks)", admin.DiscoveryEntityLinks)
	}
}

// TestRegisterDevicePositioningOrderIndependent mirrors the pre-2026-09-24
// TestDeviceFromOnlyBlockIsOrderIndependent: declaration order within the file must never matter,
// including when the block carrying the device's own "as" leaf comes LAST.
func TestRegisterDevicePositioningOrderIndependent(t *testing.T) {
	const dsl = `device hass.multiswitch with:
  entity light.social::carport from core_1;
end;

device hass.multiswitch with:
  entity light.social::main from core_2;
end;

device hass.multiswitch as multiswitch with entity binary_sensor.infrastructural:node;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.multiswitch": {DeviceID: "hass.multiswitch", Instances: []string{"main"}, Capabilities: map[string]THassBridgeCapability{
			"core_1": {Domain: "light", Sources: map[string]string{"main": "switch.raw_core_1"}},
			"core_2": {Domain: "light", Sources: map[string]string{"main": "switch.raw_core_2"}},
			"node":   {Domain: "binary_sensor", Sources: map[string]string{"main": "switch.raw_node"}},
		}},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	link := result.Administration.DeviceConceptualLinks["hass.multiswitch"]
	if len(link.AttributeEntityIDs) != 3 {
		t.Fatalf("AttributeEntityIDs: got %d entries (%v), want 3 (core_1, core_2, node all resolved)", len(link.AttributeEntityIDs), link.AttributeEntityIDs)
	}
	for _, want := range []string{"core_1", "core_2", "node"} {
		if _, ok := link.AttributeEntityIDs[want]; !ok {
			t.Errorf("AttributeEntityIDs missing %q -- a block before the \"as\"-carrying one never resolved", want)
		}
	}
}

// TestRegisterDevicePositioningForStandaloneCommandlineDevice is a regression test for a real gap
// found live 2026-09-10: a "commandline" integration device with no sibling "hosts"/"home_assistant"
// device sharing its DeviceID had no way to ever get positioned at all.
func TestRegisterDevicePositioningForStandaloneCommandlineDevice(t *testing.T) {
	const dsl = `device appliance.picture_frame as picture_frame with:
  entity switch.social:picture_frame from slideshow;
end;`

	commandlineDevicesByID := map[string]TCommandlineDevice{
		"appliance.picture_frame": {
			DeviceID: "appliance.picture_frame",
			Host:     "protocols-server-2",
			Capabilities: map[string]TCommandlineCapability{
				"slideshow": {Kind: "switch", StatusScript: "check", OnScript: "start", OffScript: "stop"},
			},
		},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, nil, nil, commandlineDevicesByID, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if report.Len() != 0 {
		t.Errorf("unexpected warnings: %s", report.String())
	}

	link, ok := result.Administration.DeviceConceptualLinks["appliance.picture_frame"]
	if !ok {
		t.Fatalf("expected a DeviceConceptualLinks entry for appliance.picture_frame")
	}
	attr, ok := link.AttributeEntityIDs["slideshow"]
	if !ok {
		t.Fatalf("expected the \"slideshow\" capability to resolve, got %+v", link.AttributeEntityIDs)
	}
	if attr.EntityID == "" || attr.Identity.Domain != "switch" {
		t.Errorf("attr = %+v, want a resolved non-empty switch entity_id", attr)
	}
}

// TestDeviceWithBlockMatchesTwoStatementFormForHosts confirms the merged "with:" block form
// produces an IDENTICAL DeviceConceptualLinks entry to writing each capability out as its own
// separate "device ... with entity ...;" one-liner, for a "hosts" device -- including "node"
// itself, now an explicit reference like any other capability (2026-09-24).
func TestDeviceWithBlockMatchesTwoStatementFormForHosts(t *testing.T) {
	const separateLinesDSL = `device host.xanadu as xanadu with entity binary_sensor.infrastructural:node;
entity sensor.infrastructural:xanadu/cpu/load        from host.xanadu load;
entity sensor.infrastructural:xanadu/cpu/temperature from host.xanadu temperature;`
	const mergedDSL = `device host.xanadu as xanadu with:
  entity binary_sensor.infrastructural:node;
  entity sensor.infrastructural:xanadu/cpu/load        from load;
  entity sensor.infrastructural:xanadu/cpu/temperature from temperature;
end;`

	hostDevicesByID := map[string]THostDevice{
		"host.xanadu": {DeviceID: "host.xanadu", HostName: "xanadu", IntegrationType: "cpu"},
	}

	var reportA, reportB strings.Builder
	resultA, errA := ParseEntitiesAndFillAdministration(strings.Split(separateLinesDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &reportA, hostDevicesByID, nil, nil, nil, nil, nil, nil, nil)
	if errA != nil {
		t.Fatalf("separate-lines parse error: %v", errA)
	}
	resultB, errB := ParseEntitiesAndFillAdministration(strings.Split(mergedDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &reportB, hostDevicesByID, nil, nil, nil, nil, nil, nil, nil)
	if errB != nil {
		t.Fatalf("merged-form parse error: %v", errB)
	}

	linkA := resultA.Administration.DeviceConceptualLinks["host.xanadu"]
	linkB := resultB.Administration.DeviceConceptualLinks["host.xanadu"]
	if linkA.NodeEntityID != linkB.NodeEntityID {
		t.Errorf("NodeEntityID: separate-lines %q vs merged %q, want identical", linkA.NodeEntityID, linkB.NodeEntityID)
	}
	if len(linkA.AttributeEntityIDs) != len(linkB.AttributeEntityIDs) {
		t.Fatalf("AttributeEntityIDs count: separate-lines %v vs merged %v, want identical", linkA.AttributeEntityIDs, linkB.AttributeEntityIDs)
	}
	for attr, wantLink := range linkA.AttributeEntityIDs {
		gotLink, ok := linkB.AttributeEntityIDs[attr]
		if !ok || gotLink != wantLink {
			t.Errorf("AttributeEntityIDs[%q]: separate-lines %+v vs merged %+v, want identical", attr, wantLink, gotLink)
		}
	}
}

// TestRegisterDevicePositioningInheritsEnclosingAreaFromNestedSpace is a regression test for a real
// gap found live 2026-09-08: a hassbridge device positioned inside a space nested under an "as
// area" ancestor never picked up the enclosing area as its own suggested_area.
func TestRegisterDevicePositioningInheritsEnclosingAreaFromNestedSpace(t *testing.T) {
	administration := newAdministrationState()
	administration.OpenSpace(SpaceKindRegular, "social:terrace", true) // "as area"
	wantArea := administration.CurrentArea()
	if wantArea == "" {
		t.Fatalf("CurrentArea() empty right after entering an \"as area\" space -- test fixture is broken")
	}
	administration.OpenSpace(SpaceKindRegular, "social:front", false) // nested, not its own area
	if got := administration.CurrentArea(); got != wantArea {
		t.Fatalf("CurrentArea() = %q after entering a non-area nested space, want it to still inherit %q", got, wantArea)
	}

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.living_room_terrace": {
			DeviceID:  "hass.living_room_terrace",
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"node": {Domain: "binary_sensor", Sources: map[string]string{"protocols-server-2": "sensor.terrace_netatmo_temperature is available"}},
			},
		},
	}
	decl := TDevicePositioningDeclaration{Spec: "netatmo", DeviceID: "hass.living_room_terrace"}

	warnings := registerDevicePositioning(administration, decl, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil, "test.def", 1)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	link := administration.DeviceConceptualLinks["hass.living_room_terrace"]
	attr, ok := link.ConstantAttributes["suggested_area"]
	if !ok || attr.Value != wantArea {
		t.Errorf("suggested_area = %+v (ok=%v), want %q inherited from the enclosing \"as area\" space", attr, ok, wantArea)
	}
}

// TestRegisterDevicePositioningExplicitSuggestedAreaWinsOverEnclosingArea confirms a device's own
// explicit suggested_area declaration still wins over an enclosing "as area" space.
func TestRegisterDevicePositioningExplicitSuggestedAreaWinsOverEnclosingArea(t *testing.T) {
	administration := newAdministrationState()
	administration.OpenSpace(SpaceKindRegular, "social:terrace", true) // "as area"

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.living_room_terrace": {
			DeviceID: "hass.living_room_terrace",
			ConstantAttributes: map[string]THostConstantAttribute{
				"suggested_area": {Value: "Explicit Area"},
			},
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"node": {Domain: "binary_sensor", Sources: map[string]string{"protocols-server-2": "sensor.terrace_netatmo_temperature is available"}},
			},
		},
	}
	decl := TDevicePositioningDeclaration{Spec: "netatmo", DeviceID: "hass.living_room_terrace"}

	warnings := registerDevicePositioning(administration, decl, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil, "test.def", 1)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	link := administration.DeviceConceptualLinks["hass.living_room_terrace"]
	if got := link.ConstantAttributes["suggested_area"].Value; got != "Explicit Area" {
		t.Errorf("suggested_area = %q, want the device's own explicit \"Explicit Area\" to win over the enclosing \"as area\" space", got)
	}
}

// TestRegisterDevicePositioningWarnsOnEntityCollisionBetweenTwoDeviceIDs is the regression test for
// a real bug found live 2026-09-24 (Junglinster): "node.laserjet" and "utility.laserjet" -- two
// entirely different device ids -- were both positioned "as laserjet", silently producing two
// colliding "node" entities with no warning at all. Originally fixed with a position-level check
// (DeviceIdentityOwner) that hard-rejected any two devices sharing a position outright; that turned
// out to be a real false positive (2026-09-25, Vienna: two Aqara sensors legitimately "weaving" onto
// the same conceptual position with no actual entity overlap) and was replaced with this precise,
// entity-level check in RegisterDiscoveryImpliedEntity -- sharing a position is fine, only an actual
// colliding final entity_id is warned about, and the warning names that entity plus both devices.
func TestRegisterDevicePositioningWarnsOnEntityCollisionBetweenTwoDeviceIDs(t *testing.T) {
	const dsl = `device node.laserjet as laserjet with entity binary_sensor.infrastructural:node;
device utility.laserjet as laserjet with entity binary_sensor.infrastructural:node;`

	hostDevicesByID := map[string]THostDevice{
		"node.laserjet": {DeviceID: "node.laserjet", HostName: "laserjet", IntegrationType: "ping"},
	}
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"utility.laserjet": {DeviceID: "utility.laserjet", Instances: []string{"main"}, Capabilities: map[string]THassBridgeCapability{
			"node": {Domain: "binary_sensor", Sources: map[string]string{"main": "sensor.laserjet_connectivity"}},
		}},
	}

	var report strings.Builder
	var result TExpansionParseResult
	var err error
	stderr := captureStderr(t, func() {
		result, err = ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, hostDevicesByID, nil, hassBridgeDevicesByID, nil, nil, nil, nil, nil)
	})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if !strings.Contains(stderr, `entity`) || !strings.Contains(stderr, `"node.laserjet"`) || !strings.Contains(stderr, `"utility.laserjet"`) || !strings.Contains(stderr, "laserjet/node") {
		t.Errorf("expected a warning naming the actual colliding entity and both devices, got stderr: %s", stderr)
	}
	// Weaving two devices onto the same position is legal on its own -- BOTH still get their own
	// identity; only the specific colliding entity is flagged, not the whole positioning.
	if _, ok := result.Administration.DeviceConceptualLinks["node.laserjet"]; !ok {
		t.Errorf("expected the FIRST device (node.laserjet) to register DeviceConceptualLinks")
	}
	if _, ok := result.Administration.DeviceConceptualLinks["utility.laserjet"]; !ok {
		t.Errorf("expected the SECOND device (utility.laserjet) to also register DeviceConceptualLinks -- sharing a position is legal")
	}
}

// TestRegisterDevicePositioningUnknownDeviceWarns confirms a device-id matching no known kind
// (Physical.def or Logical.def) is reported, not silently ignored.
func TestRegisterDevicePositioningUnknownDeviceWarns(t *testing.T) {
	administration := newAdministrationState()
	decl := TDevicePositioningDeclaration{Spec: "ghost", DeviceID: "hass.ghost"}

	warnings := registerDevicePositioning(administration, decl, nil, nil, nil, nil, nil, nil, nil, "test.def", 1)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "hass.ghost") {
		t.Fatalf("warnings = %v, want exactly one naming the unknown device", warnings)
	}
}

// TestRegisterDevicePositioningRejectsLoneInfrastructuralSpherePrefix is the regression test for
// the 2026-09-24 grammar restriction: a SINGLE "as infrastructural:<leaf>" clause is redundant (a
// device has no sphere of its own in the single-sphere case) and must be rejected, forcing the bare
// "as <leaf>" form instead -- distinct from the multi-sphere form (tested below), which IS accepted.
func TestRegisterDevicePositioningRejectsLoneInfrastructuralSpherePrefix(t *testing.T) {
	administration := newAdministrationState()
	hostDevicesByID := map[string]THostDevice{"host.x": {DeviceID: "host.x", HostName: "x", IntegrationType: "cpu"}}
	decl := TDevicePositioningDeclaration{Spec: "infrastructural:x", DeviceID: "host.x"}

	warnings := registerDevicePositioning(administration, decl, nil, hostDevicesByID, nil, nil, nil, nil, nil, "test.def", 1)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "no longer accepts a sphere prefix on its own") {
		t.Fatalf("warnings = %v, want exactly one rejecting the lone infrastructural prefix", warnings)
	}
}

// TestRegisterDevicePositioningMultiSphereLeafPerEntitySphere covers the 2026-09-24 multi-sphere
// "as" clause (isMultiSphereDeviceNamePath/resolveDeviceNamePathForSphere,
// Conceptual_DeviceEntities.go): a device's own leaf can differ PER ENTITY SPHERE, motivated by the
// real "frient" precedent -- a brand-qualifying suffix useful for "infrastructural" grouping but
// irrelevant (and undesirable) in a "physical"/"social" reading's own name. A discovery-kind device
// (the real-world case) with "as physical:front infrastructural:front/frient": the physical-sphere
// "motion"/"illuminance" capabilities must resolve under the plain "front" leaf, while the
// infrastructural-sphere "battery_level" capability keeps the full "front/frient" leaf.
func TestRegisterDevicePositioningMultiSphereLeafPerEntitySphere(t *testing.T) {
	const miniDSL = `space social:terrace with:
  device sensors.terrace_frient as physical:front infrastructural:front/frient with:
    entity binary_sensor.physical:motion from core;
    entity sensor.physical:illuminance from illuminance;
    entity sensor.infrastructural:battery_level from battery_level;
  end;
end;`

	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"sensors.terrace_frient": {
			DeviceID:    "sensors.terrace_frient",
			Identifiers: []string{"zigbee2mqtt_0xdeadbeef"},
			Capabilities: map[string]TDiscoveryCapability{
				"core":          {Domain: "binary_sensor", Leaf: "0xdeadbeef_occupancy_zigbee2mqtt"},
				"illuminance":   {Domain: "sensor", Leaf: "0xdeadbeef_illuminance_zigbee2mqtt"},
				"battery_level": {Domain: "sensor", Leaf: "0xdeadbeef_battery_zigbee2mqtt"},
			},
		},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, discoveryGatewaysByID, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration

	for label, wantEntityID := range map[string]string{
		"motion":        "binary_sensor.physical_terrace_front_motion",
		"illuminance":   "sensor.physical_terrace_front_illuminance",
		"battery_level": "sensor.infrastructural_terrace_front_frient_battery_level",
	} {
		if _, ok := admin.DiscoveryEntityLinks[wantEntityID]; !ok {
			t.Errorf("%s: expected DiscoveryEntityLinks[%q] to exist, got keys %v", label, wantEntityID, discoveryEntityLinkKeys(admin))
		}
	}

	// The device's own identity (DisplayName/collision key) must come from the "infrastructural"
	// entry, not the "physical" one.
	link, ok := admin.DeviceConceptualLinks["sensors.terrace_frient"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[sensors.terrace_frient] to be populated")
	}
	if link.DisplayName != "terrace/front/frient" {
		t.Errorf("DisplayName = %q, want %q", link.DisplayName, "terrace/front/frient")
	}
}

func discoveryEntityLinkKeys(admin *TAdministrationState) []string {
	keys := make([]string, 0, len(admin.DiscoveryEntityLinks))
	for k := range admin.DiscoveryEntityLinks {
		keys = append(keys, k)
	}
	return keys
}
