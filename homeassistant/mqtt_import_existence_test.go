/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: MQTTImportExistenceTest
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 07.09.2026
 *
 */

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportExistenceGeneratorStatusTopic(t *testing.T) {
	got := importExistenceGeneratorStatusTopic("junglinster")
	want := "import_existence/junglinster/existence/state"
	if got != want {
		t.Errorf("importExistenceGeneratorStatusTopic() = %q, want %q", got, want)
	}
}

func TestExportStableIDGenMatchesCoordinatorFormula(t *testing.T) {
	// The user's own worked example (see house_event_bus_coordinator's
	// TestExportStableIDIndependentOfLocalNaming) -- the two packages can't share the function
	// itself (separate modules), so this pins the generator's own copy to the identical formula.
	got := exportStableIDGen("junglinster", "hass.vienna_livingroom", "node")
	want := "junglinster_hass_vienna_livingroom_node"
	if got != want {
		t.Errorf("exportStableIDGen() = %q, want %q", got, want)
	}
}

func seedImportExistenceCache(t *testing.T, definitionDir, remoteInstallation string, payload TImportExistenceStatusPayload) {
	t.Helper()
	cachePath := importExistenceCachePath(definitionDir, remoteInstallation)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(cachePath, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestFetchImportExistenceFallsBackToLocalCacheOnFetchFailure(t *testing.T) {
	definitionDir := t.TempDir()
	stableID := exportStableIDGen("junglinster", "hass.vienna_livingroom", "co2")
	seedImportExistenceCache(t, definitionDir, "junglinster", TImportExistenceStatusPayload{
		stableID: "known-to-exist",
	})

	ctx := TPhysicalGenerationContext{MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}
	status, err := fetchImportExistence(definitionDir, ctx, "junglinster")
	if err != nil {
		t.Fatalf("fetchImportExistence error: %v -- want it to fall back to the cache instead", err)
	}
	if status[stableID] != "known-to-exist" {
		t.Errorf("got %+v, want the cached entry", status)
	}
}

func TestFetchImportExistenceFailsWhenNeitherFetchNorCacheAvailable(t *testing.T) {
	definitionDir := t.TempDir()
	ctx := TPhysicalGenerationContext{MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}
	if _, err := fetchImportExistence(definitionDir, ctx, "junglinster"); err == nil {
		t.Errorf("expected an error when neither a live fetch nor a cache file is available")
	}
}

func TestCheckImportKnownNotToExistErrorsFlagsConfirmedAbsence(t *testing.T) {
	definitionDir := t.TempDir()
	confirmedGoneID := exportStableIDGen("junglinster", "hass.vienna_livingroom", "co2")
	knownID := exportStableIDGen("junglinster", "hass.vienna_livingroom", "node")
	seedImportExistenceCache(t, definitionDir, "junglinster", TImportExistenceStatusPayload{
		confirmedGoneID: "known-not-to-exist",
		knownID:         "known-to-exist",
	})

	importedDevices := []TImportedDevice{
		{
			DeviceID:           "import.vienna_livingroom",
			RemoteInstallation: "junglinster",
			RemoteDeviceID:     "hass.vienna_livingroom",
			Capabilities: map[string]TImportedCapability{
				"co2":  {}, // shorthand -- RemoteEntityRef == ""
				"node": {}, // shorthand, known-to-exist -- must not be flagged
			},
		},
	}

	ctx := TPhysicalGenerationContext{MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}
	err := checkImportKnownNotToExistErrors(definitionDir, importedDevices, ctx)
	if err == nil {
		t.Fatalf("expected an error for the confirmed-absent co2 capability")
	}
	if !strings.Contains(err.Error(), "co2") {
		t.Errorf("error = %v, want it to name the co2 capability", err)
	}
	if strings.Contains(err.Error(), `"node"`) {
		t.Errorf("error = %v, want it to NOT flag node -- it's known-to-exist", err)
	}
}

func TestCheckImportKnownNotToExistErrorsOptimisticWhenUnresolved(t *testing.T) {
	definitionDir := t.TempDir()
	seedImportExistenceCache(t, definitionDir, "junglinster", TImportExistenceStatusPayload{
		// No entry at all for the co2 capability's stable id -- the remote coordinator hasn't been
		// observed yet.
	})

	importedDevices := []TImportedDevice{
		{
			DeviceID:           "import.vienna_livingroom",
			RemoteInstallation: "junglinster",
			RemoteDeviceID:     "hass.vienna_livingroom",
			Capabilities: map[string]TImportedCapability{
				"co2": {},
			},
		},
	}

	ctx := TPhysicalGenerationContext{MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}
	if err := checkImportKnownNotToExistErrors(definitionDir, importedDevices, ctx); err != nil {
		t.Errorf("unresolved status must not block generation, got: %v", err)
	}
}

func TestCheckImportKnownNotToExistErrorsIgnoresExplicitRefCapabilities(t *testing.T) {
	definitionDir := t.TempDir()
	// Deliberately no cache seeded at all -- an explicit-ref capability must never even attempt a
	// fetch, let alone be flagged, since it predates this mechanism entirely.
	importedDevices := []TImportedDevice{
		{
			DeviceID:           "import.vienna_terrace",
			RemoteInstallation: "junglinster",
			RemoteDeviceID:     "hass.vienna_terrace",
			Capabilities: map[string]TImportedCapability{
				"temperature": {RemoteEntityRef: "sensor.infrastructural_vienna_terrace_temperature"},
			},
		},
	}

	ctx := TPhysicalGenerationContext{MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}
	if err := checkImportKnownNotToExistErrors(definitionDir, importedDevices, ctx); err != nil {
		t.Errorf("an explicit-ref-only device must be a no-op (no fetch attempted at all), got: %v", err)
	}
}

func TestCheckImportKnownNotToExistErrorsNoDevicesIsNoop(t *testing.T) {
	definitionDir := t.TempDir()
	ctx := TPhysicalGenerationContext{MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}
	if err := checkImportKnownNotToExistErrors(definitionDir, nil, ctx); err != nil {
		t.Errorf("no imported devices at all must be a no-op, got: %v", err)
	}
}
