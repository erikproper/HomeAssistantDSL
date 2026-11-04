package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePhysicalDef(t *testing.T, content string) string {
	t.Helper()
	definitionDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(definitionDir, "Physical.def"), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write Physical.def: %v", err)
	}
	return definitionDir
}

// TestCollectHassBridgeDevicesByIDWarnsOnDuplicate confirms a duplicate DeviceID across two
// qualifier blocks stays a reported warning (almost certainly a Physical.def authoring mistake),
// not a silent merge.
func TestCollectHassBridgeDevicesByIDWarnsOnDuplicate(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
  home_assistant main: main;
  home_assistant protocols-server-2: protocols-server-2;

  integration home_assistant main with:
    device hass.oops with:
      capability sensor.status sensor.one;
    end;
  end;

  integration home_assistant protocols-server-2 with:
    device hass.oops with:
      capability sensor.status sensor.two;
    end;
  end;
end;
`)

	byID, warnings := collectHassBridgeDevicesByID(definitionDir)
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
	device, ok := byID["hass.oops"]
	if !ok {
		t.Fatalf("expected hass.oops to be collected; got %v", byID)
	}
	if len(device.Instances) != 1 || device.Instances[0] != "main" {
		t.Errorf("Instances = %v, want [main] (first declaration wins)", device.Instances)
	}
}

func TestCollectHassBridgeDevicesByIDParsesCapabilities(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
  home_assistant protocols-server-2: protocols-server-2;

  integration home_assistant protocols-server-2 with:
    device hass.laserjet with:
      capability sensor.status sensor.hewlett_packard_hp_laserjet_professional_p1102w;
    end;
  end;
end;
`)

	byID, warnings := collectHassBridgeDevicesByID(definitionDir)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	device, ok := byID["hass.laserjet"]
	if !ok {
		t.Fatalf("expected hass.laserjet to be collected; got %v", byID)
	}
	if len(device.Instances) != 1 || device.Instances[0] != "protocols-server-2" {
		t.Errorf("Instances = %v, want [protocols-server-2]", device.Instances)
	}
	if got := device.Capabilities["status"]; got.Domain != "sensor" || got.Sources["protocols-server-2"] != "sensor.hewlett_packard_hp_laserjet_professional_p1102w" {
		t.Errorf("Capabilities[status] = %+v, want Domain \"sensor\" and the printer's status entity as Sources[\"protocols-server-2\"]", got)
	}
	if device.Export {
		t.Errorf("Export = true, want false -- this device never declared the \"export\" keyword")
	}
}

// TestCollectHassBridgeDevicesByIDParsesDerivedCapability is a focused test for
// plans/derived-capability-mechanism.md Phase 2's "derived DDD.NNN from EEE.MMM via TTT;"
// line, inside a "home_assistant" bridge device's own capability block.
func TestCollectHassBridgeDevicesByIDParsesDerivedCapability(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
  home_assistant protocols-server-2: protocols-server-2;

  integration home_assistant protocols-server-2 with:
    device hass.roomba with:
      capability sensor.battery_level sensor.roomba_battery_level;
      derived capability binary_sensor.battery_alert from sensor.battery_level via "( $ | int(0) < 20 )";
    end;
  end;
end;
`)

	byID, warnings := collectHassBridgeDevicesByID(definitionDir)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	device, ok := byID["hass.roomba"]
	if !ok {
		t.Fatalf("expected hass.roomba to be collected; got %v", byID)
	}
	got, ok := device.Capabilities["battery_alert"]
	if !ok {
		t.Fatalf("expected a \"battery_alert\" capability, got %v", device.Capabilities)
	}
	if got.Domain != "binary_sensor" || got.DerivedFromCapability != "battery_level" || got.DerivedViaTemplate != "( $ | int(0) < 20 )" {
		t.Errorf("Capabilities[battery_alert] = %+v, want Domain=binary_sensor DerivedFromCapability=battery_level DerivedViaTemplate=\"( $ | int(0) < 20 )\"", got)
	}
	if len(got.Sources) != 0 {
		t.Errorf("Sources = %v, want empty for a derived capability", got.Sources)
	}
}

// TestCollectHassBridgeDevicesByIDParsesExportKeyword covers PROJECT.md 1.2a's parsing half:
// "device <id> export with: ...; end;" must parse identically to the bare form, just with Export
// set true -- generateHassBridgeFile propagates it to "export: true" in homeassistant_bridge.yaml
// (integration_hassbridge_generator.go), and the coordinator cross-posts to the cloud broker from
// there (house_event_bus_coordinator/discoveryhassbridge.go).
func TestCollectHassBridgeDevicesByIDParsesExportKeyword(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
  home_assistant protocols-server-2: protocols-server-2;

  integration home_assistant protocols-server-2 with:
    device hass.discovered_vienna_terrace_2 export with:
      capability sensor.humidity sensor.vienna_terrace_humidity;
    end;
  end;
end;
`)

	byID, warnings := collectHassBridgeDevicesByID(definitionDir)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	device, ok := byID["hass.discovered_vienna_terrace_2"]
	if !ok {
		t.Fatalf("expected hass.discovered_vienna_terrace_2 to be collected; got %v", byID)
	}
	if !device.Export {
		t.Errorf("Export = false, want true -- the device declared the \"export\" keyword")
	}
	if got := device.Capabilities["humidity"]; got.Domain != "sensor" || got.Sources["protocols-server-2"] != "sensor.vienna_terrace_humidity" {
		t.Errorf("Capabilities[humidity] = %+v, want it parsed normally alongside \"export\"", got)
	}
}

// TestGenerateHassBridgeFileWritesExportFlag covers PROJECT.md 1.2a's generator half: an
// Export=true device must carry "export: true" in the generated coordinator/homeassistant_bridge.yaml
// so the coordinator (house_event_bus_coordinator/discoveryhassbridge.go) knows to cross-post it to
// the cloud broker; a non-exported device must carry no "export" key at all (omitempty).
func TestGenerateHassBridgeFileWritesExportFlag(t *testing.T) {
	admin := newAdministrationState()
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.vienna_terrace": {
			DeviceID: "hass.vienna_terrace", Instances: []string{"protocols-server-2"}, Export: true,
			Capabilities: map[string]THassBridgeCapability{
				"node":        {Domain: "binary_sensor", Sources: map[string]string{"protocols-server-2": "sensor.vienna_terrace_temperature is available"}},
				"temperature": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.vienna_terrace_temperature"}},
			},
		},
	}
	positioningDecl := TDevicePositioningDeclaration{Spec: "vienna_terrace", DeviceID: "hass.vienna_terrace"}
	if warnings := registerDevicePositioning(admin, positioningDecl, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil, "Spaces.def", 1); len(warnings) != 0 {
		t.Fatalf("unexpected warnings positioning the fixture device: %v", warnings)
	}
	capabilityDecl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.infrastructural:vienna_terrace/temperature", DeviceID: "hass.vienna_terrace", Capability: "sensor.temperature"}
	if warnings, deferred := registerDeviceCapabilityEntityLink(admin, capabilityDecl, nil, hassBridgeDevicesByID, nil, nil, nil, nil, nil, "Spaces.def", 1, true, "", false); len(warnings) != 0 || deferred {
		t.Fatalf("unexpected warnings/deferred: warnings=%v deferred=%v", warnings, deferred)
	}

	outputRoot := t.TempDir()
	if err := generateHassBridgeFile(outputRoot, hassBridgeDevicesByID, admin); err != nil {
		t.Fatalf("generateHassBridgeFile error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outputRoot, "coordinator", "homeassistant_bridge.yaml"))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	if !strings.Contains(string(data), "export: true") {
		t.Errorf("generated homeassistant_bridge.yaml = %s, want it to contain \"export: true\"", data)
	}
}

// TestGenerateHassBridgeFileWritesSuggestedAreaFromEnclosingSpace is a regression test for a real
// bug found live 2026-09-08: registerDevicePositioning's own "as area" suggested_area default
// (Conceptual_DevicePositioning.go) is only ever injected into administration's own
// DeviceConceptualLinks copy, never written back into hassBridgeDevicesByID itself --
// generateHassBridgeFile used to read device.ConstantAttributes (the raw, unmerged struct) instead
// of link.ConstantAttributes (the administration-computed one), silently dropping it from the
// generated YAML even though an explicit Physical.def-declared attribute (e.g. "model:") happened
// to still show up correctly, since that one genuinely lives on both. Confirmed live: Junglinster's
// own "hass.living_room_terrace" (nested under "space social:terrace as area with: ... space
// social:front with: device infrastructural:netatmo from hass.living_room_terrace with: ...;")
// never got a suggested_area at all until this was fixed.
func TestGenerateHassBridgeFileWritesSuggestedAreaFromEnclosingSpace(t *testing.T) {
	admin := newAdministrationState()
	admin.OpenSpace(SpaceKindRegular, "social:terrace", true) // "as area"
	admin.OpenSpace(SpaceKindRegular, "social:front", false)  // nested, not its own area

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.living_room_terrace": {
			DeviceID: "hass.living_room_terrace", Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"node":     {Domain: "binary_sensor", Sources: map[string]string{"protocols-server-2": "sensor.terrace_temperature is available"}},
				"humidity": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.terrace_humidity"}},
			},
		},
	}
	positioningDecl := TDevicePositioningDeclaration{Spec: "netatmo", DeviceID: "hass.living_room_terrace"}
	if warnings := registerDevicePositioning(admin, positioningDecl, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil, "Spaces.def", 1); len(warnings) != 0 {
		t.Fatalf("unexpected warnings positioning the fixture device: %v", warnings)
	}
	capabilityDecl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.physical:netatmo/humidity", DeviceID: "hass.living_room_terrace", Capability: "sensor.humidity"}
	if warnings, deferred := registerDeviceCapabilityEntityLink(admin, capabilityDecl, nil, hassBridgeDevicesByID, nil, nil, nil, nil, nil, "Spaces.def", 1, true, "", false); len(warnings) != 0 || deferred {
		t.Fatalf("unexpected warnings/deferred: warnings=%v deferred=%v", warnings, deferred)
	}

	outputRoot := t.TempDir()
	if err := generateHassBridgeFile(outputRoot, hassBridgeDevicesByID, admin); err != nil {
		t.Fatalf("generateHassBridgeFile error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outputRoot, "coordinator", "homeassistant_bridge.yaml"))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	if !strings.Contains(string(data), "suggested_area") {
		t.Errorf("generated homeassistant_bridge.yaml = %s, want it to contain \"suggested_area\" inherited from the enclosing \"as area\" space", data)
	}
}

// TestGenerateHassBridgeFileOmitsExportFlagWhenNotDeclared is the counterpart to
// TestGenerateHassBridgeFileWritesExportFlag -- a device that never declared "export" must not
// have the key at all (the coordinator's yaml.v3 unmarshal into a bool defaults to false either
// way, but an absent key keeps the generated file's diff minimal and matches every other
// omitempty field's convention here).
func TestGenerateHassBridgeFileOmitsExportFlagWhenNotDeclared(t *testing.T) {
	admin := newAdministrationState()
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.laserjet": {
			DeviceID: "hass.laserjet", Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"node":   {Domain: "binary_sensor", Sources: map[string]string{"protocols-server-2": "sensor.hewlett_packard_hp_laserjet_professional_p1102w is available"}},
				"status": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.hewlett_packard_hp_laserjet_professional_p1102w"}},
			},
		},
	}
	positioningDecl := TDevicePositioningDeclaration{Spec: "laserjet", DeviceID: "hass.laserjet"}
	if warnings := registerDevicePositioning(admin, positioningDecl, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil, "Spaces.def", 1); len(warnings) != 0 {
		t.Fatalf("unexpected warnings positioning the fixture device: %v", warnings)
	}
	capabilityDecl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.infrastructural:laserjet/status", DeviceID: "hass.laserjet", Capability: "sensor.status"}
	if warnings, deferred := registerDeviceCapabilityEntityLink(admin, capabilityDecl, nil, hassBridgeDevicesByID, nil, nil, nil, nil, nil, "Spaces.def", 1, true, "", false); len(warnings) != 0 || deferred {
		t.Fatalf("unexpected warnings/deferred: warnings=%v deferred=%v", warnings, deferred)
	}

	outputRoot := t.TempDir()
	if err := generateHassBridgeFile(outputRoot, hassBridgeDevicesByID, admin); err != nil {
		t.Fatalf("generateHassBridgeFile error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outputRoot, "coordinator", "homeassistant_bridge.yaml"))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	if strings.Contains(string(data), "export") {
		t.Errorf("generated homeassistant_bridge.yaml = %s, want no \"export\" key for a non-exported device", data)
	}
}

// TestCollectHassBridgeDevicesByIDWarnsOnDeviceOutsideIntegrationBlock is the regression test for
// a real trap hit live 2026-08-28: a "device hass.<id> with: ...; end;" block pasted right after
// (rather than before) an "integration home_assistant <qualifier> with:" block's own closing
// "end;" is outside that block entirely -- this parser only ever looks for device declarations
// nested inside one, so the whole thing used to vanish from hassBridgeDevicesByID with zero
// feedback. Must now warn, naming the stray device id and a line number.
func TestCollectHassBridgeDevicesByIDWarnsOnDeviceOutsideIntegrationBlock(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
  home_assistant protocols-server-2: protocols-server-2;

  integration home_assistant protocols-server-2 with:
    device hass.laserjet with:
      capability sensor.status sensor.hewlett_packard_hp_laserjet_professional_p1102w;
    end;
  end;

  device hass.office_garden with:
    capability binary_sensor.node binary_sensor.office_garden_connectivity;
  end;
end;
`)

	byID, warnings := collectHassBridgeDevicesByID(definitionDir)
	if _, found := byID["hass.office_garden"]; found {
		t.Fatalf("hass.office_garden shouldn't have parsed at all (it's outside the integration block), got %+v", byID["hass.office_garden"])
	}
	if _, found := byID["hass.laserjet"]; !found {
		t.Fatalf("hass.laserjet (correctly nested) should still parse, got %v", byID)
	}

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "hass.office_garden") && strings.Contains(w, "isn't nested inside") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a warning naming the orphaned hass.office_garden block, got %v", warnings)
	}
}

// TestStrayHassBridgeDevicePatternMatchesExportKeywordToo confirms the orphan-block warning still
// recognises (and thus can warn about) a stray "device hass.<id> export with:" header, not just
// the bare form.
func TestStrayHassBridgeDevicePatternMatchesExportKeywordToo(t *testing.T) {
	if !strayHassBridgeDevicePattern.MatchString("device hass.discovered_vienna_terrace_2 export with:") {
		t.Errorf("expected strayHassBridgeDevicePattern to match a \"device hass.<id> export with:\" header too")
	}
}

func TestCollectHassBridgeDevicesByIDParsesIsAvailableCapability(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
  home_assistant main: junglinster;

  integration home_assistant main with:
    device host.junglinster with:
      capability binary_sensor.node sensor.processor_use is available;
    end;
  end;
end;
`)

	byID, warnings := collectHassBridgeDevicesByID(definitionDir)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	cap, ok := byID["host.junglinster"].Capabilities["node"]
	if !ok {
		t.Fatalf("expected a \"node\" capability, got %v", byID["host.junglinster"].Capabilities)
	}
	if cap.Domain != "binary_sensor" || cap.Sources["main"] != "sensor.processor_use is available" {
		t.Errorf("Capabilities[node] = %+v, want Domain \"binary_sensor\", Sources[\"main\"] \"sensor.processor_use is available\"", cap)
	}
}

func TestCollectHassBridgeDevicesByIDWarnsOnUnknownInstance(t *testing.T) {
	// No "home_assistant protocols-server-2: ...;" declaration at all.
	definitionDir := writePhysicalDef(t, `physical layer with:
  integration home_assistant protocols-server-2 with:
    device hass.laserjet with:
      capability sensor.status sensor.hewlett_packard_hp_laserjet_professional_p1102w;
    end;
  end;
end;
`)

	_, warnings := collectHassBridgeDevicesByID(definitionDir)
	found := false
	for _, w := range warnings {
		if strings.Contains(w, `no matching "home_assistant protocols-server-2: ...;" declaration`) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an unknown-instance warning, got %v", warnings)
	}
}

// TestCollectHassBridgeDevicesByIDReportsCorrectLineNumber is a regression test: the outer
// "physical layer with:" wrapper collectLayerContent already strips means a line number computed
// by re-splitting and re-scanning its *output* (as collectHassBridgeDevicesByID does) doesn't
// match the real Physical.def line unless translated back through collectLayerContent's own
// mergedLineNos (translateContentLineNo, defined.go) -- confirmed against a real, reported bug
// this session, where a missing-instance warning claimed line 9 for content actually on line 13.
func TestCollectHassBridgeDevicesByIDReportsCorrectLineNumber(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
  mqtt main:
    server   ${main_mqtt_server};
  end;

  mqtt_discovery_clean ems-esp;

  home_assistant main: junglinster;

  integration home_assistant protocols-server-2 with:
    device hass.laserjet with:
      capability sensor.status sensor.hewlett_packard_hp_laserjet_professional_p1102w;
    end;
  end;
end;
`)

	_, warnings := collectHassBridgeDevicesByID(definitionDir)
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
	if !strings.HasPrefix(warnings[0], "Physical.def:10:") {
		t.Errorf("warning = %q, want it to report the real Physical.def line (10), not a position within the wrapper-stripped content", warnings[0])
	}
}

func TestRegisterHassBridgeDeviceImpliedEntitiesHasNoNodeEntity(t *testing.T) {
	admin := newAdministrationState()
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.laserjet": {DeviceID: "hass.laserjet", Instances: []string{"protocols-server-2"}, Capabilities: map[string]THassBridgeCapability{
			"status": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.hewlett_packard_hp_laserjet_professional_p1102w"}},
		}},
	}

	// Positioning warns since this fixture declares no "node" capability -- that warning is
	// expected here and covered by its own dedicated test
	// (TestRegisterDevicePositioningWarnsWhenNoNodeCapabilityDeclared); this test only cares that
	// the capability link itself registers cleanly.
	positioningDecl := TDevicePositioningDeclaration{Spec: "laserjet", DeviceID: "hass.laserjet"}
	registerDevicePositioning(admin, positioningDecl, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil, "Spaces.def", 370)
	capabilityDecl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.infrastructural:laserjet/status", DeviceID: "hass.laserjet", Capability: "sensor.status"}
	if warnings, deferred := registerDeviceCapabilityEntityLink(admin, capabilityDecl, nil, hassBridgeDevicesByID, nil, nil, nil, nil, nil, "Spaces.def", 370, true, "", false); len(warnings) != 0 || deferred {
		t.Fatalf("unexpected warnings/deferred: warnings=%v deferred=%v", warnings, deferred)
	}

	link, ok := admin.DeviceConceptualLinks["hass.laserjet"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[hass.laserjet] to be populated")
	}
	if link.NodeEntityID != "" {
		t.Errorf("NodeEntityID = %q, want \"\" -- this device kind has no liveness entity", link.NodeEntityID)
	}
	attr, ok := link.AttributeEntityIDs["status"]
	if !ok {
		t.Fatalf("expected a \"status\" attribute entity, got %v", link.AttributeEntityIDs)
	}
	if attr.EntityID != "sensor.infrastructural_laserjet_status" {
		t.Errorf("status EntityID = %q, want %q", attr.EntityID, "sensor.infrastructural_laserjet_status")
	}
}

func TestCollectHassBridgeDevicesByIDParsesCapabilityWithClause(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
  home_assistant main: junglinster;

  integration home_assistant main with:
    device host.junglinster with:
      capability binary_sensor.node sensor.processor_use is available with:
        device_class: "connectivity";
      end;
      capability sensor.cpu/load sensor.processor_use with:
        unit: "%";
        icon: "mdi:cpu-64-bit";
        state_class: "measurement";
      end;
    end;
  end;
end;
`)

	byID, warnings := collectHassBridgeDevicesByID(definitionDir)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	device, ok := byID["host.junglinster"]
	if !ok {
		t.Fatalf("expected host.junglinster to be parsed, got %v", byID)
	}

	node, ok := device.Capabilities["node"]
	if !ok || node.Sources["main"] != "sensor.processor_use is available" {
		t.Fatalf("node capability = %+v, ok=%v", node, ok)
	}
	if node.DeviceClass != "connectivity" {
		t.Errorf("node's DeviceClass = %q, want \"connectivity\"", node.DeviceClass)
	}

	load, ok := device.Capabilities["cpu/load"]
	if !ok {
		t.Fatalf("expected a \"cpu/load\" capability, got %v", device.Capabilities)
	}
	if load.Unit != "%" || load.Icon != "mdi:cpu-64-bit" || load.StateClass != "measurement" {
		t.Errorf("cpu/load metadata = %+v, want unit=%%, icon=mdi:cpu-64-bit, state_class=measurement", load)
	}
}

// TestRegisterHassBridgeDeviceImpliedEntitiesFallsBackToRemainingGoIconTable is the lowest
// tier: with no explicit capability metadata and no "defaults: for ...;" rule at all
// (admin.CapabilityDefaults is empty), device_class/unit/state_class have no code-level
// fallback left -- every domain-settled subdomain's typing now lives in Defaults.def instead
// (see TestRegisterHassBridgeAttributeEntityUsesCapabilityDefaultsRules). Only icon still
// has a small Go-level fallback (subdomainIcons, generator.go) for domain-agnostic subdomains
// Defaults.def doesn't cover, e.g. "radio".
func TestRegisterHassBridgeDeviceImpliedEntitiesFallsBackToRemainingGoIconTable(t *testing.T) {
	admin := newAdministrationState()
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"host.junglinster": {DeviceID: "host.junglinster", Instances: []string{"main"}, Capabilities: map[string]THassBridgeCapability{
			"radio": {Domain: "sensor", Sources: map[string]string{"main": "sensor.some_radio_signal"}},
			"node":  {Domain: "binary_sensor", Sources: map[string]string{"main": "sensor.processor_use is available"}},
		}},
	}

	positioningDecl := TDevicePositioningDeclaration{Spec: "junglinster", DeviceID: "host.junglinster"}
	if warnings := registerDevicePositioning(admin, positioningDecl, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil, "Spaces.def", 42); len(warnings) != 0 {
		t.Fatalf("unexpected warnings positioning the fixture device: %v", warnings)
	}
	capabilityDecl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.infrastructural:junglinster/radio", DeviceID: "host.junglinster", Capability: "sensor.radio"}
	if warnings, deferred := registerDeviceCapabilityEntityLink(admin, capabilityDecl, nil, hassBridgeDevicesByID, nil, nil, nil, nil, nil, "Spaces.def", 42, true, "", false); len(warnings) != 0 || deferred {
		t.Fatalf("unexpected warnings/deferred: warnings=%v deferred=%v", warnings, deferred)
	}
	// 2026-09-24: "node" is no longer auto-registered at positioning time for any kind -- an
	// ordinary explicit capability reference, exactly like "radio" above.
	nodeDecl := TDeviceCapabilityEntityDeclaration{LocalSpec: "binary_sensor.infrastructural:junglinster/node", DeviceID: "host.junglinster", Capability: "binary_sensor.node"}
	if warnings, deferred := registerDeviceCapabilityEntityLink(admin, nodeDecl, nil, hassBridgeDevicesByID, nil, nil, nil, nil, nil, "Spaces.def", 42, true, "", false); len(warnings) != 0 || deferred {
		t.Fatalf("unexpected warnings/deferred registering node: warnings=%v deferred=%v", warnings, deferred)
	}

	link := admin.DeviceConceptualLinks["host.junglinster"]

	radio, ok := link.AttributeEntityIDs["radio"]
	if !ok {
		t.Fatalf("expected a \"radio\" attribute entity, got %v", link.AttributeEntityIDs)
	}
	if radio.Icon != "mdi:signal" {
		t.Errorf("radio's seeded Icon = %q, want \"mdi:signal\" from the remaining Go-level subdomainIcons fallback", radio.Icon)
	}

	node, ok := link.AttributeEntityIDs["node"]
	if !ok {
		t.Fatalf("expected a \"node\" attribute entity, got %v", link.AttributeEntityIDs)
	}
	if node.DeviceClass != "" || node.Icon != "" {
		t.Errorf("node's DeviceClass/Icon = %q/%q, want both empty -- \"node\" typing now only comes from a \"defaults: for ...;\" rule, and none is set here", node.DeviceClass, node.Icon)
	}
}
