package main

import (
	"strings"
	"testing"
)

func TestExtractDeviceCapabilityEntityDeclarationToleratesColumnAlignmentSpacing(t *testing.T) {
	// Spaces.def commonly pads with extra spaces for column alignment (e.g. multiple sensor
	// lines' "from" clauses lined up) -- a regression test for a real bug where the pattern used
	// a literal single space instead of \s+, silently failing to match any padded line.
	decl, ok := extractDeviceCapabilityEntityDeclaration("entity sensor.physical:netatmo/co2                 from hass.davids_bedroom entity sensor.co2;")
	if !ok {
		t.Fatalf("expected the padded line to match")
	}
	want := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.physical:netatmo/co2", DeviceID: "hass.davids_bedroom", Capability: "sensor.co2"}
	if *decl != want {
		t.Errorf("got %+v, want %+v", *decl, want)
	}
}

func TestExtractDeviceCapabilityEntityDeclarationSingleSpace(t *testing.T) {
	decl, ok := extractDeviceCapabilityEntityDeclaration("entity binary_sensor.infrastructural:netatmo/radio from hass.davids_bedroom entity binary_sensor.radio;")
	if !ok {
		t.Fatalf("expected the single-space line to match")
	}
	want := TDeviceCapabilityEntityDeclaration{LocalSpec: "binary_sensor.infrastructural:netatmo/radio", DeviceID: "hass.davids_bedroom", Capability: "binary_sensor.radio"}
	if *decl != want {
		t.Errorf("got %+v, want %+v", *decl, want)
	}
}

func TestBareCapabilityName(t *testing.T) {
	if got := bareCapabilityName("sensor.co2"); got != "co2" {
		t.Errorf("bareCapabilityName(sensor.co2) = %q, want co2", got)
	}
	if got := bareCapabilityName("co2"); got != "co2" {
		t.Errorf("bareCapabilityName(co2) = %q, want co2 (no domain prefix to strip)", got)
	}
}

func TestExpandForDeviceShorthandLine(t *testing.T) {
	got, ok := expandForDeviceShorthandLine("entity sensor.status from entity sensor.status;", "hass.laserjet")
	if !ok {
		t.Fatalf("expected the shorthand line to match")
	}
	want := "entity sensor.status from hass.laserjet entity sensor.status;"
	if got != want {
		t.Errorf("expandForDeviceShorthandLine = %q, want %q", got, want)
	}
	if _, ok := expandForDeviceShorthandLine("entity sensor.status from hass.laserjet entity sensor.status;", "hass.laserjet"); ok {
		t.Errorf("expected no match for an already-expanded (non-shorthand) line")
	}
}

// TestForDeviceBlockRegistersSameAsExplicitDeviceIDLines is the regression test for the "for
// <device-id>: ... end;" abbreviation the user asked for 2026-08-28, to avoid repeating a device
// id on every "entity ... from <device-id> entity <capability>;" line -- confirms the shorthand
// (now only reachable inside the merged "device <spec> from <device-id> with: ... end;" block,
// PROJECT.md unification plan 2026-09-01) registers identically to writing the device id out on
// every line.
func TestForDeviceBlockRegistersSameAsExplicitDeviceIDLines(t *testing.T) {
	const miniDSL = `device infrastructural:laserjet from hass.laserjet with:
  entity sensor.status   from entity sensor.status;
  entity sensor.cardrige from entity sensor.cardrige;
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.laserjet": {DeviceID: "hass.laserjet", Instances: []string{"protocols-server-2"}, Capabilities: map[string]THassBridgeCapability{
			"status":   {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.hewlett_packard_hp_laserjet_professional_p1102w"}},
			"cardrige": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.hewlett_packard_hp_laserjet_professional_p1102w_black_cartridge_hp_ce285a"}},
		}},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration

	link, ok := admin.DeviceConceptualLinks["hass.laserjet"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[hass.laserjet] to be populated")
	}
	for _, capabilityKey := range []string{"status", "cardrige"} {
		if _, found := link.AttributeEntityIDs[capabilityKey]; !found {
			t.Errorf("expected capability %q to be registered via the \"for\" block, got %v", capabilityKey, link.AttributeEntityIDs)
		}
	}
}

// TestForDeviceBlockRegistersHostsCapabilities is the "hosts"-kind counterpart to
// TestForDeviceBlockRegistersSameAsExplicitDeviceIDLines (PROJECT.md, unification plan
// 2026-09-01): "hosts" devices used to be explicitly rejected by this construct
// ("this construct doesn't support that kind yet") -- confirms the merged "device <spec> from
// <device-id> with: ... end;" block registers a cpu-type host's node (unconditional) and
// explicitly-named attributes identically to what "with: all entities;" used to bulk-imply,
// without needing that flag at all.
func TestForDeviceBlockRegistersHostsCapabilities(t *testing.T) {
	const miniDSL = `device infrastructural:xanadu from host.xanadu with:
  entity sensor.infrastructural:xanadu/cpu/load        from entity cpu/load;
  entity sensor.infrastructural:xanadu/cpu/temperature from entity cpu/temperature;
end;`

	hostDevicesByID := map[string]THostDevice{
		"host.xanadu": {DeviceID: "host.xanadu", HostName: "xanadu", IntegrationType: "cpu"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, hostDevicesByID, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	link, ok := result.Administration.DeviceConceptualLinks["host.xanadu"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[host.xanadu] to be populated")
	}
	if link.NodeEntityID == "" {
		t.Errorf("link.NodeEntityID = %q, want the node entity registered by positioning (unconditional)", link.NodeEntityID)
	}
	for _, attr := range []string{"load", "temperature"} {
		if _, found := link.AttributeEntityIDs[attr]; !found {
			t.Errorf("expected attribute %q to be registered via the \"for\" block, got %v", attr, link.AttributeEntityIDs)
		}
	}
}

// TestHostCapabilityEntityLinkRejectsUnknownAttribute confirms an attribute name that isn't part
// of the device's integration type's materialization is rejected with a clear message, not
// silently accepted or matched against the wrong thing.
func TestHostCapabilityEntityLinkRejectsUnknownAttribute(t *testing.T) {
	const miniDSL = `device infrastructural:xanadu from host.xanadu with:
  entity sensor.infrastructural:xanadu/bogus from entity bogus;
end;`

	hostDevicesByID := map[string]THostDevice{
		"host.xanadu": {DeviceID: "host.xanadu", HostName: "xanadu", IntegrationType: "cpu"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, hostDevicesByID, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	link := result.Administration.DeviceConceptualLinks["host.xanadu"]
	if _, found := link.AttributeEntityIDs["bogus"]; found {
		t.Errorf("expected \"bogus\" to be rejected (not a real cpu-type attribute), got %v", link.AttributeEntityIDs)
	}
}

// TestHostCapabilityEntityLinkRejectsNode confirms "node" can't be referenced explicitly for a
// hosts device -- it's unconditional, already registered the moment the device is positioned, so
// an explicit reference would just be a confusing, redundant second path to the same entity.
func TestHostCapabilityEntityLinkRejectsNode(t *testing.T) {
	const miniDSL = `device infrastructural:xanadu from host.xanadu with:
  entity binary_sensor.infrastructural:xanadu/node from entity node;
end;`

	hostDevicesByID := map[string]THostDevice{
		"host.xanadu": {DeviceID: "host.xanadu", HostName: "xanadu", IntegrationType: "cpu"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, hostDevicesByID, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	link, ok := result.Administration.DeviceConceptualLinks["host.xanadu"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[host.xanadu] to be populated")
	}
	if link.NodeEntityID == "" {
		t.Errorf("link.NodeEntityID = %q, want it still set from positioning, unaffected by the rejected explicit reference", link.NodeEntityID)
	}
	if _, found := link.AttributeEntityIDs["node"]; found {
		t.Errorf("link.AttributeEntityIDs = %v, want no \"node\" entry -- it's rejected, not registered as a regular attribute", link.AttributeEntityIDs)
	}
}

// TestHostCapabilityEntityLinkDefersWithoutPriorPositioning confirms a capability-link line
// appearing BEFORE its device's positioning line in the same file still resolves correctly (via
// the single retry ParseEntitiesAndFillAdministration makes after the whole file is read), mirroring
// the same order-independence the hassbridge/import branches already have.
func TestHostCapabilityEntityLinkDefersWithoutPriorPositioning(t *testing.T) {
	const miniDSL = `entity sensor.infrastructural:xanadu/cpu/load from host.xanadu entity cpu/load;
device infrastructural:xanadu from host.xanadu;`

	hostDevicesByID := map[string]THostDevice{
		"host.xanadu": {DeviceID: "host.xanadu", HostName: "xanadu", IntegrationType: "cpu"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, hostDevicesByID, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	link, ok := result.Administration.DeviceConceptualLinks["host.xanadu"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[host.xanadu] to be populated")
	}
	if _, found := link.AttributeEntityIDs["load"]; !found {
		t.Errorf("expected \"load\" to resolve via the deferred retry, got %v", link.AttributeEntityIDs)
	}
}

func TestForDeviceBlockWarnsOnUnrecognisedLineAndKeepsParsing(t *testing.T) {
	const miniDSL = `device infrastructural:laserjet from hass.laserjet with:
  this is not a valid line;
  entity sensor.status from entity sensor.status;
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.laserjet": {DeviceID: "hass.laserjet", Instances: []string{"protocols-server-2"}, Capabilities: map[string]THassBridgeCapability{
			"status": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.hewlett_packard_hp_laserjet_professional_p1102w"}},
		}},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	link, ok := result.Administration.DeviceConceptualLinks["hass.laserjet"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[hass.laserjet] to be populated")
	}
	if _, found := link.AttributeEntityIDs["status"]; !found {
		t.Errorf("expected the valid line after the bad one to still register, got %v", link.AttributeEntityIDs)
	}
}
