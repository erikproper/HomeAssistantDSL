package main

import (
	"strings"
	"testing"
)

func TestExtractDeviceWithBlockHeader(t *testing.T) {
	decl, ok := extractDeviceWithBlockHeader("device infrastructural:xanadu from host.xanadu with:")
	if !ok {
		t.Fatalf("expected the header to match")
	}
	if decl.Spec != "infrastructural:xanadu" || decl.DeviceID != "host.xanadu" {
		t.Fatalf("got %+v", decl)
	}
	if _, ok := extractDeviceWithBlockHeader("device infrastructural:xanadu from host.xanadu;"); ok {
		t.Errorf("expected no match for the bare (non-with:) positioning form")
	}
}

// TestDeviceWithBlockMatchesTwoStatementFormForHosts confirms "device <spec> from <device-id>
// with: ... end;" produces an IDENTICAL DeviceConceptualLinks entry to writing the same thing out
// longhand -- a bare "device <spec> from <device-id>;" positioning statement followed by one fully
// spelled-out "entity <spec> from <device-id> entity <capability>;" line per attribute (no "for:"
// abbreviation, which the with-block's body lines expand into internally, PROJECT.md unification
// plan 2026-09-01) -- for a "hosts" device.
func TestDeviceWithBlockMatchesTwoStatementFormForHosts(t *testing.T) {
	const twoStatementDSL = `device infrastructural:xanadu from host.xanadu;
entity sensor.infrastructural:xanadu/cpu/load        from host.xanadu entity load;
entity sensor.infrastructural:xanadu/cpu/temperature from host.xanadu entity temperature;`
	const mergedDSL = `device infrastructural:xanadu from host.xanadu with:
  entity sensor.infrastructural:xanadu/cpu/load        from entity load;
  entity sensor.infrastructural:xanadu/cpu/temperature from entity temperature;
end;`

	hostDevicesByID := map[string]THostDevice{
		"host.xanadu": {DeviceID: "host.xanadu", HostName: "xanadu", IntegrationType: "cpu"},
	}

	var reportA, reportB strings.Builder
	resultA, errA := ParseEntitiesAndFillAdministration(strings.Split(twoStatementDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &reportA, hostDevicesByID, nil, nil, nil, nil, nil)
	if errA != nil {
		t.Fatalf("two-statement parse error: %v", errA)
	}
	resultB, errB := ParseEntitiesAndFillAdministration(strings.Split(mergedDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &reportB, hostDevicesByID, nil, nil, nil, nil, nil)
	if errB != nil {
		t.Fatalf("merged-form parse error: %v", errB)
	}

	linkA := resultA.Administration.DeviceConceptualLinks["host.xanadu"]
	linkB := resultB.Administration.DeviceConceptualLinks["host.xanadu"]
	if linkA.NodeEntityID != linkB.NodeEntityID {
		t.Errorf("NodeEntityID: two-statement %q vs merged %q, want identical", linkA.NodeEntityID, linkB.NodeEntityID)
	}
	if len(linkA.AttributeEntityIDs) != len(linkB.AttributeEntityIDs) {
		t.Fatalf("AttributeEntityIDs count: two-statement %v vs merged %v, want identical", linkA.AttributeEntityIDs, linkB.AttributeEntityIDs)
	}
	for attr, wantLink := range linkA.AttributeEntityIDs {
		gotLink, ok := linkB.AttributeEntityIDs[attr]
		if !ok || gotLink != wantLink {
			t.Errorf("AttributeEntityIDs[%q]: two-statement %+v vs merged %+v, want identical", attr, wantLink, gotLink)
		}
	}
}

// TestDeviceWithBlockMatchesTwoStatementFormForHassBridge is the same equivalence check as
// TestDeviceWithBlockMatchesTwoStatementFormForHosts, for a native "home_assistant" bridge device,
// confirming the merged construct works identically for both device kinds it supports.
func TestDeviceWithBlockMatchesTwoStatementFormForHassBridge(t *testing.T) {
	const twoStatementDSL = `device infrastructural:laserjet from hass.laserjet;
entity sensor.status   from hass.laserjet entity sensor.status;
entity sensor.cardrige from hass.laserjet entity sensor.cardrige;`
	const mergedDSL = `device infrastructural:laserjet from hass.laserjet with:
  entity sensor.status   from entity sensor.status;
  entity sensor.cardrige from entity sensor.cardrige;
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.laserjet": {DeviceID: "hass.laserjet", Instances: []string{"protocols-server-2"}, Capabilities: map[string]THassBridgeCapability{
			"node":     {Domain: "binary_sensor", Sources: map[string]string{"protocols-server-2": "sensor.hewlett_packard_hp_laserjet_professional_p1102w is available"}},
			"status":   {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.hewlett_packard_hp_laserjet_professional_p1102w"}},
			"cardrige": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.hewlett_packard_hp_laserjet_professional_p1102w_black_cartridge_hp_ce285a"}},
		}},
	}

	var reportA, reportB strings.Builder
	resultA, errA := ParseEntitiesAndFillAdministration(strings.Split(twoStatementDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &reportA, nil, nil, hassBridgeDevicesByID, nil, nil, nil)
	if errA != nil {
		t.Fatalf("two-statement parse error: %v", errA)
	}
	resultB, errB := ParseEntitiesAndFillAdministration(strings.Split(mergedDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &reportB, nil, nil, hassBridgeDevicesByID, nil, nil, nil)
	if errB != nil {
		t.Fatalf("merged-form parse error: %v", errB)
	}

	linkA := resultA.Administration.DeviceConceptualLinks["hass.laserjet"]
	linkB := resultB.Administration.DeviceConceptualLinks["hass.laserjet"]
	if len(linkA.AttributeEntityIDs) != len(linkB.AttributeEntityIDs) {
		t.Fatalf("AttributeEntityIDs count: two-statement %v vs merged %v, want identical", linkA.AttributeEntityIDs, linkB.AttributeEntityIDs)
	}
	for attr, wantLink := range linkA.AttributeEntityIDs {
		gotLink, ok := linkB.AttributeEntityIDs[attr]
		if !ok || gotLink != wantLink {
			t.Errorf("AttributeEntityIDs[%q]: two-statement %+v vs merged %+v, want identical", attr, wantLink, gotLink)
		}
	}
}

func TestExtractDevicePositioningDeclaration(t *testing.T) {
	decl, ok := extractDevicePositioningDeclaration("device infrastructural:netatmo from hass.davids_bedroom;")
	if !ok {
		t.Fatalf("expected the line to match")
	}
	if decl.Spec != "infrastructural:netatmo" || decl.DeviceID != "hass.davids_bedroom" {
		t.Fatalf("got %+v", decl)
	}
}

// TestDevicePositioningWithNodeCapabilityDoesNotDropRestOfSpace is a regression test for a real
// bug hit live 2026-08-27: collectDevicePositioningDeclarations runs as a file-wide pre-pass,
// before the main parse loop reaches the space's own "with:" header. When the positioned device
// declares a "node" capability, registerDevicePositioning auto-registers that node entity via
// RegisterDiscoveryImpliedEntity, which writes straight into realAdmin.EntitiesBySpace for the
// space -- before the space has ever gone through EnsureSpaceRegistered. EnsureSpaceRegistered's
// own idempotency check (administration.go) keys off exactly that map's presence, so when the
// main loop later opens the space for real, it wrongly believes the space was already registered
// and never appends it to SpaceOrder. Every pass that walks SpaceOrder (entity/customization file
// generation, DeriveBinarySensorSubdomainAggregates) then silently skips the *entire* space --
// not just the node entity -- with no warning anywhere. Confirmed live: an entire bedroom's worth
// of entities (cover, climate, window, light group) vanished from generated output the moment
// this construct was used on a device with a "node" capability, traced down to this exact
// bookkeeping gap.
func TestDevicePositioningWithNodeCapabilityDoesNotDropRestOfSpace(t *testing.T) {
	const miniDSL = `space social:david_bedroom with:
  device infrastructural:netatmo from hass.davids_bedroom;
  entity binary_sensor.physical:windows/aqara_magnet/window;
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.davids_bedroom": {
			DeviceID:  "hass.davids_bedroom",
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"node": {Domain: "binary_sensor", Sources: map[string]string{"protocols-server-2": "sensor.davids_bedroom_connectivity"}},
			},
		},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration

	foundInSpaceOrder := false
	for _, name := range admin.SpaceOrder {
		if name == "social/david_bedroom" {
			foundInSpaceOrder = true
			break
		}
	}
	if !foundInSpaceOrder {
		t.Fatalf("SpaceOrder = %v, want it to contain %q -- a space touched by collectDevicePositioningDeclarations must still be registered when the main loop's OpenSpace reaches it", admin.SpaceOrder, "social/david_bedroom")
	}

	records := admin.EntityRecordsBySpace["social/david_bedroom"]
	foundWindow := false
	for _, rec := range records {
		if strings.Contains(rec.Name, "windows/aqara_magnet/window") {
			foundWindow = true
			break
		}
	}
	if !foundWindow {
		t.Errorf("EntityRecordsBySpace[%q] = %+v, want it to include the window entity declared after the device-positioning line", "social/david_bedroom", records)
	}
}

// TestRegisterDevicePositioningWarnsWhenNoNodeCapabilityDeclared is the regression test for a real
// gap caught live 2026-08-29: "device <spec> from <device-id>;" auto-registers that device's own
// "node" (liveness) entity when one is declared, but used to silently do nothing at all when none
// was -- several real home_assistant bridge devices had no "node" capability and nothing ever
// flagged it. Must now warn, naming the device.
func TestRegisterDevicePositioningWarnsWhenNoNodeCapabilityDeclared(t *testing.T) {
	administration := newAdministrationState()
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.no_node": {
			DeviceID:     "hass.no_node",
			Instances:    []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{"temperature": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.no_node_temperature"}}},
		},
	}
	decl := TDevicePositioningDeclaration{Spec: "infrastructural:no_node", DeviceID: "hass.no_node"}

	warnings := registerDevicePositioning(administration, decl, nil, hassBridgeDevicesByID, nil, "test.def", 1)
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "hass.no_node") || !strings.Contains(warnings[0], "no \"node\" capability declared") {
		t.Errorf("warning = %q, want it to name the device and explain the missing \"node\" capability", warnings[0])
	}

	// The device's own conceptual link (DisplayName/ConstantAttributes) must still register --
	// only the node auto-registration is skipped, not the whole positioning.
	if _, ok := administration.DeviceConceptualLinks["hass.no_node"]; !ok {
		t.Errorf("expected DeviceConceptualLinks[hass.no_node] to still be populated despite the missing node capability")
	}
}

// TestRegisterDevicePositioningNoWarningWhenNodeCapabilityDeclared confirms the warning is scoped
// precisely to the missing-node case, not raised for a normal, correctly-declared device.
func TestRegisterDevicePositioningNoWarningWhenNodeCapabilityDeclared(t *testing.T) {
	administration := newAdministrationState()
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.davids_bedroom": {
			DeviceID:  "hass.davids_bedroom",
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"node": {Domain: "binary_sensor", Sources: map[string]string{"protocols-server-2": "sensor.davids_bedroom_temperature is available"}},
			},
		},
	}
	decl := TDevicePositioningDeclaration{Spec: "infrastructural:netatmo", DeviceID: "hass.davids_bedroom"}

	warnings := registerDevicePositioning(administration, decl, nil, hassBridgeDevicesByID, nil, "test.def", 1)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings when a \"node\" capability is declared, got %v", warnings)
	}
}
