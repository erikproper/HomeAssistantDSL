package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGeneratePhysicalIntegrationOutputsWritesDevicesFileWithNoHostsBlock is the regression test
// for a real bug found live 2026-08-30 standing up Vienna's coordinator: a house with no
// "integration hosts" block at all (e.g. a coordinator/cloud-broker setup stood up before any
// local hosts devices exist, PROJECT.md 1.2c/1.2d) never got a coordinator/devices.yaml written
// at all, even though Installation/MQTTDiscoveryConceptualPrefix -- read from there by every
// other coordinator subscription -- have nothing else to come from. Confirmed live: the
// coordinator logged "a cloud broker is configured but devices.yaml has no installation set" and
// refused to publish/clean up cloud-side topics.
func TestGeneratePhysicalIntegrationOutputsWritesDevicesFileWithNoHostsBlock(t *testing.T) {
	definitionDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(definitionDir, "Settings.def"), []byte(`${installation} = "vienna";`), 0o644); err != nil {
		t.Fatalf("writing Settings.def: %v", err)
	}
	physicalDef := `physical layer with:
  home_assistant main: vienna https://home-vienna.erikproper.eu;
end;
`
	if err := os.WriteFile(filepath.Join(definitionDir, "Physical.def"), []byte(physicalDef), 0o644); err != nil {
		t.Fatalf("writing Physical.def: %v", err)
	}

	outputRoot := t.TempDir()
	admin := newAdministrationState()
	if err := generatePhysicalIntegrationOutputs(definitionDir, outputRoot, t.TempDir(), admin); err != nil {
		t.Fatalf("generatePhysicalIntegrationOutputs error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outputRoot, "coordinator", "devices.yaml"))
	if err != nil {
		t.Fatalf("expected coordinator/devices.yaml to be written even with no \"integration hosts\" block: %v", err)
	}
	if !strings.Contains(string(data), `installation: "vienna"`) {
		t.Errorf("generated devices.yaml = %s, want it to carry installation: \"vienna\"", data)
	}
}
