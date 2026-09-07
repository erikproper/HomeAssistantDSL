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
// "for <device-id>: entity ... from entity <capability>;") must only emit the capabilities that
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
