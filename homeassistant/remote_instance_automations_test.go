package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateInstanceAutomationTreesWritesMainAndRemoteTrees(t *testing.T) {
	admin := newAdministrationState()
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.laserjet": {DeviceID: "hass.laserjet", Instances: []string{"protocols-server-2"}, Capabilities: map[string]THassBridgeCapability{
			"status": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.hewlett_packard_hp_laserjet_professional_p1102w"}},
		}},
	}
	positioningDecl := TDevicePositioningDeclaration{Spec: "infrastructural:laserjet", DeviceID: "hass.laserjet"}
	registerDevicePositioning(admin, positioningDecl, nil, nil, hassBridgeDevicesByID, nil, nil, nil, "Spaces.def", 370)
	capabilityDecl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.infrastructural:laserjet/status", DeviceID: "hass.laserjet", Capability: "status"}
	if warnings, deferred := registerDeviceCapabilityEntityLink(admin, capabilityDecl, nil, hassBridgeDevicesByID, nil, nil, nil, nil, "Spaces.def", 370, true, ""); len(warnings) != 0 || deferred {
		t.Fatalf("unexpected warnings/deferred positioning the fixture device: warnings=%v deferred=%v", warnings, deferred)
	}

	instances := map[string]THomeAssistantInstance{
		"main":               {Name: "junglinster"},
		"protocols-server-2": {Name: "protocols-server-2"},
	}

	root := t.TempDir()
	haOutputDir := filepath.Join(root, "hass", "junglinster")

	if err := generateInstanceAutomationTrees(haOutputDir, instances, hassBridgeDevicesByID, admin); err != nil {
		t.Fatalf("generateInstanceAutomationTrees error: %v", err)
	}

	// coordinator_bootstrap generation is disabled (2026-08-27, remote_instance_automations.go) --
	// its "report entities detailed" automation hung "main" outright when forced; PROJECT.md 1.1
	// replaces it with a passive statestream + interrogation-queue design. Assert absence, not
	// content, until that lands and this test is updated to match the new automation.
	if _, err := os.Stat(filepath.Join(haOutputDir, "automation", "infrastructural", "automation.coordinator_bootstrap.yaml")); !os.IsNotExist(err) {
		t.Errorf("expected no coordinator_bootstrap automation to be written while it's disabled")
	}
	if _, err := os.Stat(filepath.Join(haOutputDir, "automation", "infrastructural", "automation.coordinator_bridge_report.yaml")); !os.IsNotExist(err) {
		t.Errorf("expected no bridge-reporting automation for \"main\" (it isn't a bridge source instance)")
	}

	remoteInstanceDir := filepath.Join(root, "hass", "protocols-server-2")
	remoteConfig, err := os.ReadFile(filepath.Join(remoteInstanceDir, "configuration.yaml"))
	if err != nil {
		t.Fatalf("expected protocols-server-2 to get its own configuration.yaml: %v", err)
	}
	if strings.Contains(string(remoteConfig), "customize") {
		t.Errorf("configuration.yaml = %q, want no \"customize:\" line -- no customization/ directory is generated for a remote instance", remoteConfig)
	}
	if !strings.Contains(string(remoteConfig), "packages: !include_dir_named integrations") {
		t.Errorf("configuration.yaml = %q, want the packages include directive", remoteConfig)
	}
	remoteAutomationInclude, err := os.ReadFile(filepath.Join(remoteInstanceDir, "integrations", "automation.yaml"))
	if err != nil {
		t.Fatalf("expected protocols-server-2 to get integrations/automation.yaml: %v", err)
	}
	if !strings.Contains(string(remoteAutomationInclude), "!include_dir_merge_list ../automation") {
		t.Errorf("integrations/automation.yaml = %q, want the automation include-dir directive", remoteAutomationInclude)
	}
	if _, err := os.Stat(filepath.Join(remoteInstanceDir, "integrations", "sensor.yaml")); !os.IsNotExist(err) {
		t.Errorf("expected no other domain's integrations/*.yaml -- this generator produces nothing else for a remote instance")
	}

	remoteRoot := filepath.Join(remoteInstanceDir, "automation", "infrastructural")
	if _, err := os.Stat(filepath.Join(remoteRoot, "automation.coordinator_bootstrap.yaml")); !os.IsNotExist(err) {
		t.Errorf("expected no coordinator_bootstrap automation to be written while it's disabled")
	}

	// hass.laserjet has a capability but no DeviceInfoCapabilities -- the combined device-info
	// automation has nothing to report and shouldn't be written at all.
	if _, err := os.Stat(filepath.Join(remoteRoot, "automation.coordinator_bridge_report.yaml")); !os.IsNotExist(err) {
		t.Errorf("expected no device-info automation for a device with no DeviceInfoCapabilities")
	}

	// The "status" capability instead gets its own per-entity reporting automation (PROJECT.md
	// 1.1) -- filename/alias built from its fully qualified name (sensor/infrastructural/laserjet/status).
	remoteReporting, err := os.ReadFile(filepath.Join(remoteRoot, "automation.reporting_sensor_infrastructural_laserjet_status.yaml"))
	if err != nil {
		t.Fatalf("expected protocols-server-2's per-entity reporting automation to be written: %v", err)
	}
	if !strings.Contains(string(remoteReporting), `alias: "reporting/sensor/infrastructural/laserjet/status"`) {
		t.Errorf("reporting automation = %q, want the slash-joined alias", remoteReporting)
	}
	if !strings.Contains(string(remoteReporting), "sensor.hewlett_packard_hp_laserjet_professional_p1102w") {
		t.Errorf("reporting automation = %q, want it to reference the laserjet's source entity", remoteReporting)
	}
	if !strings.Contains(string(remoteReporting), `topic: "homeassistant_instances/protocols-server-2/bridge/sensor.infrastructural_laserjet_status/state"`) {
		t.Errorf("reporting automation = %q, want a fixed topic keyed by the local entity_id", remoteReporting)
	}
	if !strings.Contains(string(remoteReporting), "event: start") {
		t.Errorf("reporting automation = %q, want a homeassistant-start trigger so a fresh restart republishes state", remoteReporting)
	}
}

// TestGenerateInstanceAutomationTreesUsesIncarnationValueForOutputDirNotQualifier covers the real
// bug found live 2026-09-21 (Vienna's "frame"->"ha2mqtt" instance rename): the output directory for
// a non-"main" instance must be named after its own "home_assistant <qualifier>: <name>;" RIGHT-hand
// value (matching resolveMainIncarnationName's own convention for "main" -- hass/vienna/, not
// hass/main/), while the MQTT topics inside that tree must still use the LEFT-hand qualifier (what
// the coordinator and homeassistant_bridge.yaml key by). This only surfaces once the two differ --
// the existing fixture above uses "protocols-server-2" for both, which would pass even with the bug.
func TestGenerateInstanceAutomationTreesUsesIncarnationValueForOutputDirNotQualifier(t *testing.T) {
	admin := newAdministrationState()
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.laserjet": {DeviceID: "hass.laserjet", Instances: []string{"ha2mqtt"}, Capabilities: map[string]THassBridgeCapability{
			"status": {Domain: "sensor", Sources: map[string]string{"ha2mqtt": "sensor.laserjet_status"}},
		}},
	}
	positioningDecl := TDevicePositioningDeclaration{Spec: "infrastructural:laserjet", DeviceID: "hass.laserjet"}
	registerDevicePositioning(admin, positioningDecl, nil, nil, hassBridgeDevicesByID, nil, nil, nil, "Spaces.def", 1)
	capabilityDecl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.infrastructural:laserjet/status", DeviceID: "hass.laserjet", Capability: "status"}
	if warnings, deferred := registerDeviceCapabilityEntityLink(admin, capabilityDecl, nil, hassBridgeDevicesByID, nil, nil, nil, nil, "Spaces.def", 1, true, ""); len(warnings) != 0 || deferred {
		t.Fatalf("unexpected warnings/deferred positioning the fixture device: warnings=%v deferred=%v", warnings, deferred)
	}

	instances := map[string]THomeAssistantInstance{
		"main":    {Name: "vienna"},
		"ha2mqtt": {Name: "protocols-server-1"},
	}

	root := t.TempDir()
	haOutputDir := filepath.Join(root, "hass", "vienna")

	if err := generateInstanceAutomationTrees(haOutputDir, instances, hassBridgeDevicesByID, admin); err != nil {
		t.Fatalf("generateInstanceAutomationTrees error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "hass", "ha2mqtt")); !os.IsNotExist(err) {
		t.Errorf("expected no hass/ha2mqtt/ output directory -- the qualifier must not be used as the output directory name")
	}

	reporting, err := os.ReadFile(filepath.Join(root, "hass", "protocols-server-1", "automation", "infrastructural", "automation.reporting_sensor_infrastructural_laserjet_status.yaml"))
	if err != nil {
		t.Fatalf("expected the output tree at hass/protocols-server-1/ (the qualifier's own incarnation value), not hass/ha2mqtt/: %v", err)
	}
	if !strings.Contains(string(reporting), `topic: "homeassistant_instances/ha2mqtt/bridge/sensor.infrastructural_laserjet_status/state"`) {
		t.Errorf("reporting automation = %q, want the topic to still use the qualifier \"ha2mqtt\", not the incarnation value \"protocols-server-1\"", reporting)
	}
}

func TestHassBridgeReportingAutomationBodyDeviceInfo(t *testing.T) {
	deviceInfoReports := []THassBridgeDeviceInfoReport{
		{
			DeviceID: "host.junglinster",
			Fields: map[string]string{
				"model":      "sensor.foo",
				"sw_version": "update.home_assistant_operating_system_update!installed_version",
			},
			LiteralPrefixes: map[string]string{
				"sw_version": "Home Assistant Operating System ",
			},
		},
	}
	body := hassBridgeReportingAutomationBody("main", deviceInfoReports)

	if !strings.Contains(body, `topic: "homeassistant_instances/main/bridge/device/host.junglinster/state"`) {
		t.Errorf("missing the device-info topic, got: %s", body)
	}
	wantPayload := `payload: "{{ {'model': states('sensor.foo'), 'sw_version': 'Home Assistant Operating System ' ~ state_attr('update.home_assistant_operating_system_update', 'installed_version')} | tojson }}"`
	if !strings.Contains(body, wantPayload) {
		t.Errorf("device-info payload = %s, want it to contain %q", body, wantPayload)
	}
	// Both fields' bare entities must be in the trigger list too.
	if !strings.Contains(body, "    - sensor.foo\n") || !strings.Contains(body, "    - update.home_assistant_operating_system_update\n") {
		t.Errorf("trigger entity_id list missing device-info fields' bare entities, got: %s", body)
	}
}

func TestGenerateInstanceAutomationTreesNoInstancesIsNoop(t *testing.T) {
	root := t.TempDir()
	haOutputDir := filepath.Join(root, "hass", "junglinster")

	if err := generateInstanceAutomationTrees(haOutputDir, nil, nil, newAdministrationState()); err != nil {
		t.Fatalf("generateInstanceAutomationTrees error: %v", err)
	}
	if _, err := os.Stat(haOutputDir); !os.IsNotExist(err) {
		t.Errorf("expected no output at all when no instance is declared")
	}
}
