package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateImportedHassBridgeFile(t *testing.T) {
	devices := []TImportedDevice{
		{
			DeviceID: "hass.vienna_terrace", RemoteInstallation: "junglinster", RemoteDeviceID: "hass.vienna_terrace",
			Capabilities: map[string]TImportedCapability{
				"temperature": {},
			},
		},
	}
	admin := newAdministrationState()
	admin.DeviceConceptualLinks["hass.vienna_terrace"] = TDeviceConceptualLink{
		DisplayName: "terrace/netatmo",
		AttributeEntityIDs: map[string]TDeviceAttributeLink{
			"temperature": {EntityID: "sensor.infrastructural_terrace_netatmo_temperature"},
		},
	}

	outputRoot := t.TempDir()
	if err := generateImportedDeviceFile(outputRoot, devices, admin); err != nil {
		t.Fatalf("generateImportedDeviceFile error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outputRoot, "coordinator", "imported.yaml"))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	content := string(data)
	for _, want := range []string{
		"hass.vienna_terrace:",
		"remote_installation: \"junglinster\"",
		"remote_device_id: \"hass.vienna_terrace\"",
		"display_name: terrace/netatmo",
		"local_entity: sensor.infrastructural_terrace_netatmo_temperature",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("generated file = %s, want it to contain %q", content, want)
		}
	}
	if strings.Contains(content, "remote_entity_ref") {
		t.Errorf("generated file = %s, must never contain \"remote_entity_ref\" -- the explicit form was removed 2026-09-07", content)
	}
}

// TestGenerateImportedHassBridgeFileWritesDerivedCapabilityFields is a focused test for
// plans/derived-capability-mechanism.md Phase 2's import/kind-4 runtime path: a "derived"
// capability's Domain/DerivedFromCapability/DerivedViaTemplate must pass through into
// imported.yaml verbatim, alongside its own local_entity like any other capability -- an ordinary
// (non-derived) capability must NOT gain these fields.
func TestGenerateImportedHassBridgeFileWritesDerivedCapabilityFields(t *testing.T) {
	devices := []TImportedDevice{
		{
			DeviceID: "import.vienna_terrace", RemoteInstallation: "junglinster", RemoteDeviceID: "hass.vienna_terrace",
			Capabilities: map[string]TImportedCapability{
				"battery_level": {},
				"battery_alert": {Domain: "binary_sensor", DerivedFromCapability: "battery_level", DerivedViaTemplate: "( $ | int(0) < 20 )"},
			},
		},
	}
	admin := newAdministrationState()
	admin.DeviceConceptualLinks["import.vienna_terrace"] = TDeviceConceptualLink{
		DisplayName: "terrace/netatmo",
		AttributeEntityIDs: map[string]TDeviceAttributeLink{
			"battery_level": {EntityID: "sensor.infrastructural_terrace_netatmo_battery_level"},
			"battery_alert": {EntityID: "binary_sensor.infrastructural_terrace_netatmo_battery_alert"},
		},
	}

	outputRoot := t.TempDir()
	if err := generateImportedDeviceFile(outputRoot, devices, admin); err != nil {
		t.Fatalf("generateImportedDeviceFile error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outputRoot, "coordinator", "imported.yaml"))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	content := string(data)
	for _, want := range []string{
		"local_entity: binary_sensor.infrastructural_terrace_netatmo_battery_alert",
		"domain: binary_sensor",
		"derived_from: battery_level",
		"derived_via: \"( $ | int(0) < 20 )\"",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("generated file = %s, want it to contain %q", content, want)
		}
	}

	// The ordinary battery_level capability must not gain any of the three derived-only fields.
	// Capabilities are written in sorted order ("battery_alert" < "battery_level"), so
	// battery_level's own block runs from its header to the end of the generated file -- the
	// three derived-only strings above only ever appear inside battery_alert's earlier block.
	batteryLevelIdx := strings.Index(content, "battery_level:")
	if batteryLevelIdx < 0 {
		t.Fatalf("expected battery_level in generated file: %s", content)
	}
	batteryLevelBlock := content[batteryLevelIdx:]
	if strings.Contains(batteryLevelBlock, "domain:") || strings.Contains(batteryLevelBlock, "derived_from:") || strings.Contains(batteryLevelBlock, "derived_via:") {
		t.Errorf("ordinary capability block = %q, must not contain any derived-only field", batteryLevelBlock)
	}
}

// TestGenerateImportedHassBridgeFileSkipsUnpositionedDevice covers the "hasLink" gate: a device
// declared in Physical.def but never positioned in Spaces.def (no DeviceConceptualLinks entry)
// must be silently skipped, same as generateHassBridgeFile's own export-side gate.
func TestGenerateImportedHassBridgeFileSkipsUnpositionedDevice(t *testing.T) {
	devices := []TImportedDevice{
		{
			DeviceID: "hass.vienna_terrace", RemoteInstallation: "junglinster", RemoteDeviceID: "hass.vienna_terrace",
			Capabilities: map[string]TImportedCapability{
				"temperature": {},
			},
		},
	}
	admin := newAdministrationState()

	outputRoot := t.TempDir()
	if err := generateImportedDeviceFile(outputRoot, devices, admin); err != nil {
		t.Fatalf("generateImportedDeviceFile error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputRoot, "coordinator", "imported.yaml")); !os.IsNotExist(err) {
		t.Errorf("expected no file written when the device was never positioned in Spaces.def, got err=%v", err)
	}
}

// TestGenerateImportedHassBridgeFileSkipsUnusedCapability covers the per-capability half of the
// same gate: a positioned device with an UNUSED declared capability (never referenced via
// "for <device-id>: entity ... from <capability>;") must only emit the capabilities that
// were actually used.
func TestGenerateImportedHassBridgeFileSkipsUnusedCapability(t *testing.T) {
	devices := []TImportedDevice{
		{
			DeviceID: "hass.vienna_terrace", RemoteInstallation: "junglinster", RemoteDeviceID: "hass.vienna_terrace",
			Capabilities: map[string]TImportedCapability{
				"temperature": {},
				"humidity":    {},
			},
		},
	}
	admin := newAdministrationState()
	admin.DeviceConceptualLinks["hass.vienna_terrace"] = TDeviceConceptualLink{
		AttributeEntityIDs: map[string]TDeviceAttributeLink{
			"temperature": {EntityID: "sensor.infrastructural_terrace_netatmo_temperature"},
		},
	}

	outputRoot := t.TempDir()
	if err := generateImportedDeviceFile(outputRoot, devices, admin); err != nil {
		t.Fatalf("generateImportedDeviceFile error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outputRoot, "coordinator", "imported.yaml"))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	if strings.Contains(string(data), "humidity") {
		t.Errorf("generated file = %s, want no \"humidity\" entry -- it was never used in Spaces.def", data)
	}
}

// TestGenerateImportedHassBridgeFileOmitsDisplayNameWhenUnset covers a positioned device whose
// DisplayName is empty (e.g. a "device from" positioning that predates this field) -- the line
// must be omitted rather than written blank, leaving discoveryimport.go's own LocalDeviceID
// fallback to apply.
func TestGenerateImportedHassBridgeFileOmitsDisplayNameWhenUnset(t *testing.T) {
	devices := []TImportedDevice{
		{
			DeviceID: "hass.vienna_terrace", RemoteInstallation: "junglinster", RemoteDeviceID: "hass.vienna_terrace",
			Capabilities: map[string]TImportedCapability{
				"temperature": {},
			},
		},
	}
	admin := newAdministrationState()
	admin.DeviceConceptualLinks["hass.vienna_terrace"] = TDeviceConceptualLink{
		AttributeEntityIDs: map[string]TDeviceAttributeLink{
			"temperature": {EntityID: "sensor.infrastructural_terrace_netatmo_temperature"},
		},
	}

	outputRoot := t.TempDir()
	if err := generateImportedDeviceFile(outputRoot, devices, admin); err != nil {
		t.Fatalf("generateImportedDeviceFile error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outputRoot, "coordinator", "imported.yaml"))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	if strings.Contains(string(data), "display_name") {
		t.Errorf("generated file = %s, want no \"display_name\" line when DisplayName is unset", data)
	}
}

func TestGenerateImportedHassBridgeFileNoDevicesWritesNoFile(t *testing.T) {
	outputRoot := t.TempDir()
	admin := newAdministrationState()
	if err := generateImportedDeviceFile(outputRoot, nil, admin); err != nil {
		t.Fatalf("generateImportedDeviceFile error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputRoot, "coordinator", "imported.yaml")); !os.IsNotExist(err) {
		t.Errorf("expected no file written when nothing is imported, got err=%v", err)
	}
}
