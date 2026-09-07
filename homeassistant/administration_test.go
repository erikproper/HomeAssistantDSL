/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Administration tests
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 06.09.2026
 *
 */

package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// captureStderr redirects os.Stderr for the duration of fn and returns everything written to it --
// used to observe RegisterDiscoveryImpliedEntity's direct [WARNING] print (administration.go),
// which mirrors AppendEntityRecord's own same-space duplicate-registration warning style rather than
// threading a return value back through callers.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = original }()

	fn()

	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("io.ReadAll: %v", err)
	}
	return string(out)
}

// TestDeviceSourceEntityWarnsOnCrossSpaceConceptualReuse is the regression test for PROJECT.md item
// 0b, "each entity (of a device) can only be used once at the conceptual layer": the same hassbridge
// device capability, referenced via the bare "as <source>" form (registerDeviceSourceEntityLink,
// capabilityKey == "") from two different spaces, must warn -- even though nothing else in the
// pipeline would otherwise notice, since each space's own EntityRecordSeenBySpace tracking is scoped
// per-space and would see two entirely distinct entity names.
func TestDeviceSourceEntityWarnsOnCrossSpaceConceptualReuse(t *testing.T) {
	const miniDSL = `device infrastructural:laserjet from hass.laserjet;
space social:living_room with:
  entity sensor.social:living_room/battery as sensor.laserjet_battery from hass.laserjet;
end;
space social:kitchen with:
  entity sensor.social:kitchen/battery as sensor.laserjet_battery from hass.laserjet;
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.laserjet": {DeviceID: "hass.laserjet", Instances: []string{"protocols-server-2"}, Capabilities: map[string]THassBridgeCapability{}},
	}

	var report strings.Builder
	var result TExpansionParseResult
	var err error
	output := captureStderr(t, func() {
		result, err = ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil)
	})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_ = result

	if !strings.Contains(output, "[WARNING]") || !strings.Contains(output, "hass.laserjet!sensor.laserjet_battery") {
		t.Fatalf("expected a cross-space conceptual reuse warning naming the shared source, got: %q", output)
	}
}

// TestDeviceSourceEntityNoWarningForDistinctSources is the companion guard test: two DIFFERENT
// capabilities of the same device, positioned under two different entities in two different spaces,
// is exactly the legitimate case item 0b's rule must never flag.
func TestDeviceSourceEntityNoWarningForDistinctSources(t *testing.T) {
	const miniDSL = `device infrastructural:laserjet from hass.laserjet;
space social:living_room with:
  entity sensor.social:living_room/battery as sensor.laserjet_battery from hass.laserjet;
end;
space social:kitchen with:
  entity sensor.social:kitchen/status as sensor.laserjet_status from hass.laserjet;
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.laserjet": {DeviceID: "hass.laserjet", Instances: []string{"protocols-server-2"}, Capabilities: map[string]THassBridgeCapability{}},
	}

	var report strings.Builder
	var err error
	output := captureStderr(t, func() {
		_, err = ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil)
	})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if strings.Contains(output, "already used as a different conceptual entity") {
		t.Fatalf("expected no cross-space reuse warning for two distinct sources, got: %q", output)
	}
}
