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

// captureStdout is captureStderr's own mirror for os.Stdout -- used to observe the plain
// fmt.Printf("[physical] ...") diagnostics scattered through the "physical" generation pass
// (mqtt_discovery_existence.go and siblings), which print directly rather than returning warnings.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = original }()

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
		result, err = ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil)
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
		_, err = ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil)
	})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if strings.Contains(output, "already used as a different conceptual entity") {
		t.Fatalf("expected no cross-space reuse warning for two distinct sources, got: %q", output)
	}
}

// TestDeriveBinarySensorSubdomainAggregatesOnlyTracksBackedSubdomains is the regression test for a
// real bug found live 2026-09-18 (Vienna's "windy" migration): DeriveBinarySensorSubdomainAggregates
// used to derive a placeholder aggregate EntityRecord for EVERY binary_sensor subdomain it saw (any
// entity's own last path segment), not just the four (binarySensorSubdomains, generator.go)
// generateBinarySensorSubdomainGroups actually backs with a real group entity. Every other
// subdomain (e.g. "battery_alert", "windy") still got a customization file written
// (generateCustomizationFiles iterates every EntityRecord unconditionally) for an entity_id no
// generation step ever creates -- confirmed live: binary_sensor.social_terrace_windy got a
// customize.yaml stanza but was correctly absent from HA's own entity registry. Fixed by scoping
// the derived-aggregate source subdomains to the same tracked list.
func TestDeriveBinarySensorSubdomainAggregatesOnlyTracksBackedSubdomains(t *testing.T) {
	state := newAdministrationState()
	state.SpaceOrder = []string{"social/terrace"}
	state.EntityRecordsBySpace["social/terrace"] = []TEntityRecord{
		{Name: "binary_sensor.social/terrace/door", Identity: extractEntityIdentity("binary_sensor.social/terrace/door")},
		{Name: "binary_sensor.social/terrace/netatmo_windmeter/windy", Identity: extractEntityIdentity("binary_sensor.social/terrace/netatmo_windmeter/windy")},
	}

	state.DeriveBinarySensorSubdomainAggregates()

	var gotDoor, gotWindy bool
	for _, rec := range state.EntityRecordsBySpace["social/terrace"] {
		switch rec.Name {
		case "binary_sensor.social/terrace/door":
			gotDoor = true
		case "binary_sensor.social/terrace/windy":
			gotWindy = true
		}
	}
	if !gotDoor {
		t.Errorf("expected a derived aggregate for tracked subdomain %q (backed by generateBinarySensorSubdomainGroups)", "door")
	}
	if gotWindy {
		t.Errorf("did not expect a derived aggregate for untracked subdomain %q -- no generation step backs it with a real entity", "windy")
	}
}

// TestRegisterExternalEntityReferenceRecordsAttributeSuffix covers the "!attribute" tracking added
// 2026-09-21 (buildUndeclaredExternalEntityAttributesSuggestions' own data source): registering
// "weather.forecast!pressure" must record "pressure" against "weather.forecast" in
// ExternalEntityReferencedAttributes, while a bare reference (no "!") records nothing.
func TestRegisterExternalEntityReferenceRecordsAttributeSuffix(t *testing.T) {
	state := newAdministrationState()

	state.RegisterExternalEntityReference("social", "weather.forecast!pressure", "test")
	state.RegisterExternalEntityReference("social", "weather.forecast", "test")

	got := state.ExternalEntityReferencedAttributes["weather.forecast"]
	if len(got) != 1 || !got["pressure"] {
		t.Errorf("ExternalEntityReferencedAttributes[weather.forecast] = %v, want exactly {pressure: true}", got)
	}
}

// TestRegisterExternalEntityReferenceAccumulatesMultipleAttributes covers the real live case
// (environment.weather's own "node" and "pressure" capabilities both referencing weather.forecast,
// one bare and one with "!pressure") extended one step further: two DIFFERENT attribute suffixes
// referenced for the same entity across separate calls must both accumulate, not overwrite.
func TestRegisterExternalEntityReferenceAccumulatesMultipleAttributes(t *testing.T) {
	state := newAdministrationState()

	state.RegisterExternalEntityReference("social", "weather.forecast!pressure", "test")
	state.RegisterExternalEntityReference("social", "weather.forecast!humidity", "test")

	got := state.ExternalEntityReferencedAttributes["weather.forecast"]
	if len(got) != 2 || !got["pressure"] || !got["humidity"] {
		t.Errorf("ExternalEntityReferencedAttributes[weather.forecast] = %v, want {pressure: true, humidity: true}", got)
	}
}
