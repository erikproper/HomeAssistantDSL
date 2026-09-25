package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seedPassthroughDevicesCache(t *testing.T, definitionDir string, devices map[string]tPassthroughDeviceSuggestion) {
	t.Helper()
	seedPassthroughStatusCache(t, definitionDir, tPassthroughStatusSuggestion{Devices: devices})
}

func seedPassthroughStatusCache(t *testing.T, definitionDir string, status tPassthroughStatusSuggestion) {
	t.Helper()
	cachePath := passthroughDevicesCachePath(definitionDir)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(cachePath, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestBuildPassthroughSuggestionReportEmitsIdentifiersAndCapabilities(t *testing.T) {
	devices := map[string]tPassthroughDeviceSuggestion{
		"zigbee2mqtt_0xaabbcc": {
			Name: "aqara_multi",
			Leaves: map[string]tPassthroughLeafSuggestion{
				"0xaabbcc_temperature": {Domain: "sensor", Leaf: "0xaabbcc_temperature"},
			},
		},
	}
	report := buildPassthroughSuggestionReport(devices, nil)
	if !strings.Contains(report, "device discovery.aqara_multi with:") {
		t.Errorf("report = %q, want a \"device discovery.aqara_multi with:\" header", report)
	}
	if !strings.Contains(report, `identifiers "zigbee2mqtt_0xaabbcc";`) {
		t.Errorf("report = %q, want an explicit identifiers line (the slug has no relation to the real device id)", report)
	}
	// recognizeDiscoveryCapability recognizes "temperature" in the leaf name -- LHS uses the
	// recognized suffix, RHS is always the raw leaf itself (recognizeDiscoveryCapability's own
	// existing behavior, shared with the gateway-leaf report).
	if !strings.Contains(report, "sensor.temperature sensor.0xaabbcc_temperature;") {
		t.Errorf("report = %q, want the leaf suggested as a capability line", report)
	}
}

func TestBuildPassthroughSuggestionReportFallsBackToIdentifierWhenNameEmpty(t *testing.T) {
	devices := map[string]tPassthroughDeviceSuggestion{
		"zigbee2mqtt_0xaabbcc": {
			Leaves: map[string]tPassthroughLeafSuggestion{"leaf": {Domain: "sensor", Leaf: "leaf"}},
		},
	}
	report := buildPassthroughSuggestionReport(devices, nil)
	if !strings.Contains(report, "device discovery.zigbee2mqtt_0xaabbcc with:") {
		t.Errorf("report = %q, want the slug to fall back to the sanitized raw identifier", report)
	}
}

func TestBuildPassthroughSuggestionReportExcludesAlreadyDeclaredDevices(t *testing.T) {
	devices := map[string]tPassthroughDeviceSuggestion{
		"zigbee2mqtt_0xaabbcc": {
			Name:   "aqara_multi",
			Leaves: map[string]tPassthroughLeafSuggestion{"leaf": {Domain: "sensor", Leaf: "leaf"}},
		},
	}
	report := buildPassthroughSuggestionReport(devices, map[string]bool{"zigbee2mqtt_0xaabbcc": true})
	if strings.TrimSpace(report) != "" {
		t.Errorf("expected an already-declared device to be excluded, got:\n%s", report)
	}
}

func TestBuildPassthroughSuggestionReportSkipsLeaflessDevices(t *testing.T) {
	devices := map[string]tPassthroughDeviceSuggestion{
		"zigbee2mqtt_0xaabbcc": {Name: "aqara_multi", Leaves: map[string]tPassthroughLeafSuggestion{}},
	}
	report := buildPassthroughSuggestionReport(devices, nil)
	if strings.TrimSpace(report) != "" {
		t.Errorf("expected a device with no leaves to produce nothing, got:\n%s", report)
	}
}

// TestBuildPassthroughCollisionReportWarnsAboutBothDevices is the suggestion-side counterpart to
// the coordinator's ClaimName (house_event_bus_coordinator/discovery_passthrough_devices.go) --
// confirms the warning names BOTH the winning and the suppressed device, plus the actual
// default_entity_id in question, so the operator knows exactly what to go fix in Zigbee2MQTT.
func TestBuildPassthroughCollisionReportWarnsAboutBothDevices(t *testing.T) {
	collisions := map[string]tPassthroughNameCollisionSuggestion{
		"switch.house/server_room/xanadu": {ClaimedBy: "zigbee2mqtt_0xa4c138a4716e0c02", BlockedID: "zigbee2mqtt_0xc4988600000f73e3"},
	}
	report := buildPassthroughCollisionReport(collisions)
	if !strings.Contains(report, `"switch.house/server_room/xanadu"`) {
		t.Errorf("report = %q, want the colliding default_entity_id named", report)
	}
	if !strings.Contains(report, "zigbee2mqtt_0xa4c138a4716e0c02") || !strings.Contains(report, "zigbee2mqtt_0xc4988600000f73e3") {
		t.Errorf("report = %q, want both the claimed-by and blocked device identifiers named", report)
	}
}

func TestBuildPassthroughCollisionReportEmptyWhenNoCollisions(t *testing.T) {
	if report := buildPassthroughCollisionReport(nil); strings.TrimSpace(report) != "" {
		t.Errorf("expected no collisions to produce nothing, got:\n%s", report)
	}
}

// TestGenerateDiscoverySuggestionsIncludesPassthroughCollisions is the end-to-end regression test
// for the real bug found live 2026-09-20 (Junglinster's "xanadu" smart plug, renamed in
// Zigbee2MQTT, left a stale retained discovery payload colliding with the device that took over
// its name) -- a collision recorded by the coordinator must reach suggestions/discovery.txt.
func TestGenerateDiscoverySuggestionsIncludesPassthroughCollisions(t *testing.T) {
	definitionDir := t.TempDir()
	outputRoot := t.TempDir()
	seedPassthroughStatusCache(t, definitionDir, tPassthroughStatusSuggestion{
		Collisions: map[string]tPassthroughNameCollisionSuggestion{
			"switch.house/server_room/xanadu": {ClaimedBy: "zigbee2mqtt_0xa4c138a4716e0c02", BlockedID: "zigbee2mqtt_0xc4988600000f73e3"},
		},
	})

	rules := []TDiscoveryPassthroughRule{{TopicPrefix: "zigbee2mqtt/", SourcePrefix: "homeassistant.physical"}}
	ctx := TPhysicalGenerationContext{HasMQTTSecrets: true, MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}

	if err := generateDiscoverySuggestions(definitionDir, outputRoot, map[string]TDiscoveryGatewayDevice{}, nil, rules, ctx); err != nil {
		t.Fatalf("generateDiscoverySuggestions error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outputRoot, "suggestions", "discovery.txt"))
	if err != nil {
		t.Fatalf("expected suggestions/discovery.txt to be written: %v", err)
	}
	if !strings.Contains(string(data), "switch.house/server_room/xanadu") {
		t.Errorf("suggestions file = %s, want the collision warning", data)
	}
}

func TestAlreadyDeclaredIdentifiersCollectsAcrossAllGateways(t *testing.T) {
	gateways := map[string]TDiscoveryGatewayDevice{
		"discovery.a": {Identifiers: []string{"id-1", "id-2"}},
		"discovery.b": {Identifiers: []string{"id-3"}},
	}
	claimed := alreadyDeclaredIdentifiers(gateways)
	for _, id := range []string{"id-1", "id-2", "id-3"} {
		if !claimed[id] {
			t.Errorf("expected %q to be claimed, got %+v", id, claimed)
		}
	}
	if claimed["id-4"] {
		t.Errorf("expected an undeclared identifier to not be claimed")
	}
}

// TestGenerateDiscoverySuggestionsIncludesPassthroughDevices confirms the real gap found live
// 2026-09-14 is actually fixed end to end: a house with a declared gateway (EMS-ESP-style) AND
// passthrough traffic must see BOTH in the same suggestions/discovery.txt.
func TestGenerateDiscoverySuggestionsIncludesPassthroughDevices(t *testing.T) {
	definitionDir := t.TempDir()
	outputRoot := t.TempDir()
	seedDiscoveryExistenceAggregateCache(t, definitionDir, TDiscoveryExistenceAggregatePayload{
		"discovery.ems_esp_boiler": {"boiler_outdoortemp": "known-to-exist"},
	})
	seedPassthroughDevicesCache(t, definitionDir, map[string]tPassthroughDeviceSuggestion{
		"zigbee2mqtt_0xaabbcc": {
			Name:   "aqara_multi",
			Leaves: map[string]tPassthroughLeafSuggestion{"0xaabbcc_temperature": {Domain: "sensor", Leaf: "0xaabbcc_temperature"}},
		},
	})

	gateways := map[string]TDiscoveryGatewayDevice{
		"discovery.ems_esp_boiler": {DeviceID: "discovery.ems_esp_boiler", Capabilities: map[string]TDiscoveryCapability{}},
	}
	rules := []TDiscoveryPassthroughRule{{TopicPrefix: "zigbee2mqtt/", SourcePrefix: "homeassistant.physical"}}
	ctx := TPhysicalGenerationContext{
		HasMQTTSecrets:              true,
		MQTTSecrets:                 TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"},
		DiscoveryExistenceAggregate: &tDiscoveryExistenceFetchResult{},
	}

	if err := generateDiscoverySuggestions(definitionDir, outputRoot, gateways, nil, rules, ctx); err != nil {
		t.Fatalf("generateDiscoverySuggestions error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outputRoot, "suggestions", "discovery.txt"))
	if err != nil {
		t.Fatalf("expected suggestions/discovery.txt to be written: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "boiler_outdoortemp") {
		t.Errorf("suggestions file = %s, want the existing gateway-leaf suggestion still present", content)
	}
	if !strings.Contains(content, "device discovery.aqara_multi with:") {
		t.Errorf("suggestions file = %s, want the passthrough device suggestion too", content)
	}
}

// TestGenerateDiscoverySuggestionsIncludesUndeclaredDevicesWithoutPassthroughRules is the
// regression test for a real bug found live 2026-09-19: fetchPassthroughDevices used to be gated
// on "len(passthroughRules) > 0", so a house with real discovery gateways declared but NO
// "discovery_passthrough ...;" rule at all (a perfectly normal state -- passthrough is an opt-in
// gradual-migration aid, not a requirement) never saw suggestions for undeclared devices, even
// though the coordinator's own tracker (discoverybridge.go, fixed the same day) was recording them
// regardless of passthrough rules. Confirms the fetch now always runs once the function's own
// top-of-function guard is satisfied.
func TestGenerateDiscoverySuggestionsIncludesUndeclaredDevicesWithoutPassthroughRules(t *testing.T) {
	definitionDir := t.TempDir()
	outputRoot := t.TempDir()
	seedDiscoveryExistenceAggregateCache(t, definitionDir, TDiscoveryExistenceAggregatePayload{
		"discovery.ems_esp_boiler": {"boiler_outdoortemp": "known-to-exist"},
	})
	seedPassthroughDevicesCache(t, definitionDir, map[string]tPassthroughDeviceSuggestion{
		"zigbee2mqtt_0xaabbcc": {
			Name:   "aqara_multi",
			Leaves: map[string]tPassthroughLeafSuggestion{"0xaabbcc_temperature": {Domain: "sensor", Leaf: "0xaabbcc_temperature"}},
		},
	})

	gateways := map[string]TDiscoveryGatewayDevice{
		"discovery.ems_esp_boiler": {DeviceID: "discovery.ems_esp_boiler", Capabilities: map[string]TDiscoveryCapability{}},
	}
	ctx := TPhysicalGenerationContext{
		HasMQTTSecrets:              true,
		MQTTSecrets:                 TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"},
		DiscoveryExistenceAggregate: &tDiscoveryExistenceFetchResult{},
	}

	// No passthrough rules at all -- this is the exact condition that used to suppress the fetch.
	if err := generateDiscoverySuggestions(definitionDir, outputRoot, gateways, nil, nil, ctx); err != nil {
		t.Fatalf("generateDiscoverySuggestions error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outputRoot, "suggestions", "discovery.txt"))
	if err != nil {
		t.Fatalf("expected suggestions/discovery.txt to be written: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "device discovery.aqara_multi with:") {
		t.Errorf("suggestions file = %s, want the undeclared device suggestion even with zero passthrough rules", content)
	}
}

// TestGenerateDiscoverySuggestionsPassthroughOnlyStillRuns confirms a house with NO declared
// gateways at all -- the common starting point for a gradual migration -- still gets passthrough
// suggestions, rather than the old "no gateways declared = no-op" short-circuit silently hiding
// them.
func TestGenerateDiscoverySuggestionsPassthroughOnlyStillRuns(t *testing.T) {
	definitionDir := t.TempDir()
	outputRoot := t.TempDir()
	seedPassthroughDevicesCache(t, definitionDir, map[string]tPassthroughDeviceSuggestion{
		"zigbee2mqtt_0xaabbcc": {
			Name:   "aqara_multi",
			Leaves: map[string]tPassthroughLeafSuggestion{"0xaabbcc_temperature": {Domain: "sensor", Leaf: "0xaabbcc_temperature"}},
		},
	})

	rules := []TDiscoveryPassthroughRule{{TopicPrefix: "zigbee2mqtt/", SourcePrefix: "homeassistant.physical"}}
	ctx := TPhysicalGenerationContext{HasMQTTSecrets: true, MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}

	if err := generateDiscoverySuggestions(definitionDir, outputRoot, map[string]TDiscoveryGatewayDevice{}, nil, rules, ctx); err != nil {
		t.Fatalf("generateDiscoverySuggestions error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outputRoot, "suggestions", "discovery.txt"))
	if err != nil {
		t.Fatalf("expected suggestions/discovery.txt to be written even with zero declared gateways: %v", err)
	}
	if !strings.Contains(string(data), "device discovery.aqara_multi with:") {
		t.Errorf("suggestions file = %s, want the passthrough device suggestion", data)
	}
}
