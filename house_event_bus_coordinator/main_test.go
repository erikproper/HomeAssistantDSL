package main

import (
	"path/filepath"
	"testing"
)

// TestLoadDevicesFileMissingReturnsZeroValue is the regression test for a real bug found live
// 2026-08-30 standing up Vienna's coordinator on frame: a house with no "hosts" integration
// devices declared at all never gets a devices.yaml written (same as every other
// integration-specific output file), but loadDevicesFile used to treat that as a hard error and
// abort startup -- unlike loadHassBridgeFile/loadDiscoveryFile, which already tolerated a missing
// file. Confirmed the fix by getting the coordinator running against a real devices.yaml-less
// coordinator/ directory.
func TestLoadDevicesFileMissingReturnsZeroValue(t *testing.T) {
	got, err := loadDevicesFile(filepath.Join(t.TempDir(), "does_not_exist.yaml"))
	if err != nil {
		t.Fatalf("loadDevicesFile error: %v", err)
	}
	if len(got.Devices) != 0 {
		t.Errorf("got %v, want a zero-value file for a missing path", got)
	}
}
