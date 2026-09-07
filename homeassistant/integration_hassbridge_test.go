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

// TestCollectHassBridgeDevicesByIDMergesRoamingInstances covers a "roaming" device declared
// identically in two "integration home_assistant <qualifier> with:" blocks within the same house
// (e.g. an HA companion-app phone reachable via either of two local HA instances) -- the two
// declarations must merge into ONE record with both instances listed, not silently collide
// (first-wins), since each real instance still needs its own reporting automation generated.
func TestCollectHassBridgeDevicesByIDMergesRoamingInstances(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
  home_assistant main: main;
  home_assistant protocols-server-2: protocols-server-2;

  integration home_assistant main with:
    device hass.eriks_iphone roaming import with:
      sensor.battery_level: sensor.eriks_iphone_15_pro_battery_level;
    end;
  end;

  integration home_assistant protocols-server-2 with:
    device hass.eriks_iphone roaming import with:
      sensor.battery_level: sensor.eriks_iphone_15_pro_battery_level;
    end;
  end;
end;
`)
	if err := os.WriteFile(filepath.Join(definitionDir, "Settings.def"), []byte(`${installation} = "junglinster";`), 0o644); err != nil {
		t.Fatalf("failed to write Settings.def: %v", err)
	}

	byID, warnings := collectHassBridgeDevicesByID(definitionDir)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	device, ok := byID["hass.eriks_iphone"]
	if !ok {
		t.Fatalf("expected hass.eriks_iphone to be collected; got %v", byID)
	}
	if len(device.Instances) != 2 || !containsString(device.Instances, "main") || !containsString(device.Instances, "protocols-server-2") {
		t.Errorf("Instances = %v, want [main protocols-server-2] in some order", device.Instances)
	}
	if device.ExportAs != "roaming" {
		t.Errorf("ExportAs = %q, want %q", device.ExportAs, "roaming")
	}
	if device.SelfImportFrom != "junglinster" {
		t.Errorf("SelfImportFrom = %q, want %q", device.SelfImportFrom, "junglinster")
	}
}

// TestCollectHassBridgeDevicesByIDMergesRoamingInstancesWithoutInstallation covers the same
// merge, but with ${installation} unresolved -- SelfImportFrom stays unresolved (with its own
// warning per declaration), while the Instances merge itself must still succeed independently.
func TestCollectHassBridgeDevicesByIDMergesRoamingInstancesWithoutInstallation(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
  home_assistant main: main;
  home_assistant protocols-server-2: protocols-server-2;

  integration home_assistant main with:
    device hass.eriks_iphone roaming import with:
      sensor.battery_level: sensor.eriks_iphone_15_pro_battery_level;
    end;
  end;

  integration home_assistant protocols-server-2 with:
    device hass.eriks_iphone roaming import with:
      sensor.battery_level: sensor.eriks_iphone_15_pro_battery_level;
    end;
  end;
end;
`)

	byID, warnings := collectHassBridgeDevicesByID(definitionDir)
	if len(warnings) != 2 {
		t.Fatalf("got %d warnings, want 2 (one per declaration, \"import\" unresolvable): %v", len(warnings), warnings)
	}
	device, ok := byID["hass.eriks_iphone"]
	if !ok {
		t.Fatalf("expected hass.eriks_iphone to be collected; got %v", byID)
	}
	if len(device.Instances) != 2 || !containsString(device.Instances, "main") || !containsString(device.Instances, "protocols-server-2") {
		t.Errorf("Instances = %v, want [main protocols-server-2] in some order (merge must succeed independently of SelfImportFrom resolution)", device.Instances)
	}
	if device.ExportAs != "roaming" {
		t.Errorf("ExportAs = %q, want %q", device.ExportAs, "roaming")
	}
	if device.SelfImportFrom != "" {
		t.Errorf("SelfImportFrom = %q, want \"\" (no ${installation} set)", device.SelfImportFrom)
	}
}

// TestCollectHassBridgeDevicesByIDWarnsOnNonRoamingDuplicate confirms a genuine duplicate
// DeviceID across two qualifier blocks -- neither declared "roaming" -- stays a reported warning
// (almost certainly a Physical.def authoring mistake), not a silent merge.
func TestCollectHassBridgeDevicesByIDWarnsOnNonRoamingDuplicate(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
  home_assistant main: main;
  home_assistant protocols-server-2: protocols-server-2;

  integration home_assistant main with:
    device hass.oops with:
      sensor.status: sensor.one;
    end;
  end;

  integration home_assistant protocols-server-2 with:
    device hass.oops with:
      sensor.status: sensor.two;
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
      sensor.status: sensor.hewlett_packard_hp_laserjet_professional_p1102w;
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
      sensor.humidity: sensor.vienna_terrace_humidity;
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

// TestCollectHassBridgeDevicesByIDParsesRoamingAndImportKeywords covers the 2026-09-02 addition:
// "roaming" (an alternative to "export" that cross-posts under the fixed virtual installation
// name "roaming" instead of this installation's own real name) and "import" (self-import, same
// meaning as the "hosts" side's "cloud"+"import" -- resolves to this installation's own name once
// ${installation} is set).
func TestCollectHassBridgeDevicesByIDParsesRoamingAndImportKeywords(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
  home_assistant protocols-server-2: protocols-server-2;

  integration home_assistant protocols-server-2 with:
    device hass.eriks_iphone roaming import with:
      sensor.battery_level: sensor.eriks_iphone_battery_level;
    end;
  end;
end;
`)
	if err := os.WriteFile(filepath.Join(definitionDir, "Settings.def"), []byte(`${installation} = "protocols-server-2";`), 0o644); err != nil {
		t.Fatalf("writing Settings.def: %v", err)
	}

	byID, warnings := collectHassBridgeDevicesByID(definitionDir)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	device, ok := byID["hass.eriks_iphone"]
	if !ok {
		t.Fatalf("expected hass.eriks_iphone to be collected; got %v", byID)
	}
	if !device.Export || device.ExportAs != "roaming" {
		t.Errorf("Export/ExportAs = %v/%q, want true/\"roaming\"", device.Export, device.ExportAs)
	}
	if device.SelfImportFrom != "protocols-server-2" {
		t.Errorf("SelfImportFrom = %q, want %q", device.SelfImportFrom, "protocols-server-2")
	}
}

// TestCollectHassBridgeDevicesByIDWarnsWhenBothExportAndRoamingDeclared confirms declaring both
// keywords on the same device header is flagged, not silently resolved one way or the other.
func TestCollectHassBridgeDevicesByIDWarnsWhenBothExportAndRoamingDeclared(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
  home_assistant protocols-server-2: protocols-server-2;

  integration home_assistant protocols-server-2 with:
    device hass.eriks_iphone export roaming with:
      sensor.battery_level: sensor.eriks_iphone_battery_level;
    end;
  end;
end;
`)

	byID, warnings := collectHassBridgeDevicesByID(definitionDir)
	device, ok := byID["hass.eriks_iphone"]
	if !ok {
		t.Fatalf("expected hass.eriks_iphone to still be collected despite the conflicting keywords")
	}
	if device.ExportAs != "roaming" {
		t.Errorf("ExportAs = %q, want \"roaming\" to win over bare \"export\"", device.ExportAs)
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "hass.eriks_iphone") && strings.Contains(w, "export") && strings.Contains(w, "roaming") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a warning naming both conflicting keywords, got %v", warnings)
	}
}

// TestCollectHassBridgeDevicesByIDWarnsWhenImportDeclaredWithoutInstallation confirms "import" is
// safely ignored (not silently resolved to a wrong/empty qualifier) when ${installation} isn't set.
func TestCollectHassBridgeDevicesByIDWarnsWhenImportDeclaredWithoutInstallation(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
  home_assistant protocols-server-2: protocols-server-2;

  integration home_assistant protocols-server-2 with:
    device hass.eriks_iphone roaming import with:
      sensor.battery_level: sensor.eriks_iphone_battery_level;
    end;
  end;
end;
`)
	// Deliberately no Settings.def -- ${installation} resolves to "".

	byID, warnings := collectHassBridgeDevicesByID(definitionDir)
	device := byID["hass.eriks_iphone"]
	if device.SelfImportFrom != "" {
		t.Errorf("SelfImportFrom = %q, want \"\" -- installation is unresolved", device.SelfImportFrom)
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "hass.eriks_iphone") && strings.Contains(w, "installation") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a warning about the unresolved installation, got %v", warnings)
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
	positioningDecl := TDevicePositioningDeclaration{Spec: "infrastructural:vienna_terrace", DeviceID: "hass.vienna_terrace"}
	if warnings := registerDevicePositioning(admin, positioningDecl, nil, hassBridgeDevicesByID, nil, "Spaces.def", 1); len(warnings) != 0 {
		t.Fatalf("unexpected warnings positioning the fixture device: %v", warnings)
	}
	capabilityDecl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.infrastructural:vienna_terrace/temperature", DeviceID: "hass.vienna_terrace", Capability: "temperature"}
	if warnings, deferred := registerDeviceCapabilityEntityLink(admin, capabilityDecl, nil, hassBridgeDevicesByID, nil, nil, nil, "Spaces.def", 1, true); len(warnings) != 0 || deferred {
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

// TestGenerateHassBridgeFileWritesExportAsAndSelfImportFrom covers the 2026-09-02 "roaming"
// addition's generator half: ExportAs/SelfImportFrom must carry through into
// coordinator/homeassistant_bridge.yaml, for the coordinator to resolve a per-device cross-post
// qualifier and know when to relay "roaming" data back onto this instance's own local topic.
func TestGenerateHassBridgeFileWritesExportAsAndSelfImportFrom(t *testing.T) {
	admin := newAdministrationState()
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.eriks_iphone": {
			DeviceID: "hass.eriks_iphone", Instances: []string{"protocols-server-2"},
			Export: true, ExportAs: "roaming", SelfImportFrom: "protocols-server-2",
			Capabilities: map[string]THassBridgeCapability{
				"battery_level": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.eriks_iphone_battery_level"}},
			},
		},
	}
	positioningDecl := TDevicePositioningDeclaration{Spec: "infrastructural:eriks_iphone", DeviceID: "hass.eriks_iphone"}
	registerDevicePositioning(admin, positioningDecl, nil, hassBridgeDevicesByID, nil, "Spaces.def", 1)
	capabilityDecl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.infrastructural:eriks_iphone/battery_level", DeviceID: "hass.eriks_iphone", Capability: "battery_level"}
	if warnings, deferred := registerDeviceCapabilityEntityLink(admin, capabilityDecl, nil, hassBridgeDevicesByID, nil, nil, nil, "Spaces.def", 1, true); len(warnings) != 0 || deferred {
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
	if !strings.Contains(string(data), `export_as: "roaming"`) {
		t.Errorf("generated homeassistant_bridge.yaml = %s, want it to contain export_as: \"roaming\"", data)
	}
	if !strings.Contains(string(data), `self_import_from: "protocols-server-2"`) {
		t.Errorf("generated homeassistant_bridge.yaml = %s, want it to contain self_import_from: \"protocols-server-2\"", data)
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
	positioningDecl := TDevicePositioningDeclaration{Spec: "infrastructural:laserjet", DeviceID: "hass.laserjet"}
	if warnings := registerDevicePositioning(admin, positioningDecl, nil, hassBridgeDevicesByID, nil, "Spaces.def", 1); len(warnings) != 0 {
		t.Fatalf("unexpected warnings positioning the fixture device: %v", warnings)
	}
	capabilityDecl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.infrastructural:laserjet/status", DeviceID: "hass.laserjet", Capability: "status"}
	if warnings, deferred := registerDeviceCapabilityEntityLink(admin, capabilityDecl, nil, hassBridgeDevicesByID, nil, nil, nil, "Spaces.def", 1, true); len(warnings) != 0 || deferred {
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
      sensor.status: sensor.hewlett_packard_hp_laserjet_professional_p1102w;
    end;
  end;

  device hass.office_garden with:
    binary_sensor.node: binary_sensor.office_garden_connectivity;
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
      binary_sensor.node: sensor.processor_use is available;
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
      sensor.status: sensor.hewlett_packard_hp_laserjet_professional_p1102w;
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
      sensor.status: sensor.hewlett_packard_hp_laserjet_professional_p1102w;
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
	positioningDecl := TDevicePositioningDeclaration{Spec: "infrastructural:laserjet", DeviceID: "hass.laserjet"}
	registerDevicePositioning(admin, positioningDecl, nil, hassBridgeDevicesByID, nil, "Spaces.def", 370)
	capabilityDecl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.infrastructural:laserjet/status", DeviceID: "hass.laserjet", Capability: "status"}
	if warnings, deferred := registerDeviceCapabilityEntityLink(admin, capabilityDecl, nil, hassBridgeDevicesByID, nil, nil, nil, "Spaces.def", 370, true); len(warnings) != 0 || deferred {
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
      binary_sensor.node: sensor.processor_use is available with:
        device_class: "connectivity";
      end;
      sensor.cpu/load: sensor.processor_use with:
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

	positioningDecl := TDevicePositioningDeclaration{Spec: "infrastructural:junglinster", DeviceID: "host.junglinster"}
	if warnings := registerDevicePositioning(admin, positioningDecl, nil, hassBridgeDevicesByID, nil, "Spaces.def", 42); len(warnings) != 0 {
		t.Fatalf("unexpected warnings positioning the fixture device: %v", warnings)
	}
	capabilityDecl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.infrastructural:junglinster/radio", DeviceID: "host.junglinster", Capability: "radio"}
	if warnings, deferred := registerDeviceCapabilityEntityLink(admin, capabilityDecl, nil, hassBridgeDevicesByID, nil, nil, nil, "Spaces.def", 42, true); len(warnings) != 0 || deferred {
		t.Fatalf("unexpected warnings/deferred: warnings=%v deferred=%v", warnings, deferred)
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
