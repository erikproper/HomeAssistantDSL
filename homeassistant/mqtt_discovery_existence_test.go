package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoveryGatewayExistenceStatusTopic(t *testing.T) {
	got := discoveryGatewayExistenceStatusTopic("discovery.ems_esp")
	want := "discovery_gateways/discovery.ems_esp/existence/state"
	if got != want {
		t.Errorf("discoveryGatewayExistenceStatusTopic() = %q, want %q", got, want)
	}
}

func seedDiscoveryExistenceCache(t *testing.T, definitionDir, gatewayID string, payload TDiscoveryExistenceStatusPayload) {
	t.Helper()
	cachePath := discoveryExistenceCachePath(definitionDir, gatewayID)
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

func TestFetchDiscoveryExistenceFallsBackToLocalCacheOnFetchFailure(t *testing.T) {
	definitionDir := t.TempDir()
	seedDiscoveryExistenceCache(t, definitionDir, "discovery.ems_esp", TDiscoveryExistenceStatusPayload{
		"boiler_outdoortemp": "known-to-exist",
	})

	ctx := TPhysicalGenerationContext{MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}
	status, err := fetchDiscoveryExistence(definitionDir, ctx, "discovery.ems_esp")
	if err != nil {
		t.Fatalf("fetchDiscoveryExistence error: %v -- want it to fall back to the cache instead", err)
	}
	if status["boiler_outdoortemp"] != "known-to-exist" {
		t.Errorf("got %+v, want the cached entry", status)
	}
}

func TestFetchDiscoveryExistenceFailsWhenNeitherFetchNorCacheAvailable(t *testing.T) {
	definitionDir := t.TempDir()
	ctx := TPhysicalGenerationContext{MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}
	if _, err := fetchDiscoveryExistence(definitionDir, ctx, "discovery.ems_esp"); err == nil {
		t.Errorf("expected an error when neither a live fetch nor a cache file is available")
	}
}

func TestCheckDiscoveryKnownNotToExistErrorsFlagsConfirmedAbsence(t *testing.T) {
	definitionDir := t.TempDir()
	seedDiscoveryExistenceCache(t, definitionDir, "discovery.ems_esp", TDiscoveryExistenceStatusPayload{
		"boiler_outdoortemp":  "known-not-to-exist",
		"thermostat_lastcode": "known-to-exist",
	})

	discoveryEntityLinks := map[string]TDiscoveryEntityLink{
		"sensor.physical:garage_door/temperature": {GatewayDeviceID: "discovery.ems_esp", Leaf: "boiler_outdoortemp"},
		"sensor.physical:garage_door/lastcode":    {GatewayDeviceID: "discovery.ems_esp", Leaf: "thermostat_lastcode"},
	}

	ctx := TPhysicalGenerationContext{MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}
	err := checkDiscoveryKnownNotToExistErrors(definitionDir, discoveryEntityLinks, ctx)
	if err == nil {
		t.Fatalf("expected an error for the confirmed-absent boiler_outdoortemp leaf")
	}
	if !strings.Contains(err.Error(), "boiler_outdoortemp") {
		t.Errorf("error = %v, want it to name the boiler_outdoortemp leaf", err)
	}
	if strings.Contains(err.Error(), "thermostat_lastcode") {
		t.Errorf("error = %v, want it to NOT flag thermostat_lastcode -- it's known-to-exist", err)
	}
}

func TestCheckDiscoveryKnownNotToExistErrorsOptimisticWhenUnresolved(t *testing.T) {
	definitionDir := t.TempDir()
	seedDiscoveryExistenceCache(t, definitionDir, "discovery.ems_esp", TDiscoveryExistenceStatusPayload{
		// No entry at all for boiler_outdoortemp -- the coordinator hasn't observed it yet.
	})

	discoveryEntityLinks := map[string]TDiscoveryEntityLink{
		"sensor.physical:garage_door/temperature": {GatewayDeviceID: "discovery.ems_esp", Leaf: "boiler_outdoortemp"},
	}

	ctx := TPhysicalGenerationContext{MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}
	if err := checkDiscoveryKnownNotToExistErrors(definitionDir, discoveryEntityLinks, ctx); err != nil {
		t.Errorf("unresolved status must not block generation, got: %v", err)
	}
}

func TestCheckDiscoveryKnownNotToExistErrorsNoLinksIsNoop(t *testing.T) {
	definitionDir := t.TempDir()
	ctx := TPhysicalGenerationContext{MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}
	if err := checkDiscoveryKnownNotToExistErrors(definitionDir, map[string]TDiscoveryEntityLink{}, ctx); err != nil {
		t.Errorf("no discovery entity links at all must be a no-op, got: %v", err)
	}
}

func TestRecognizeDiscoveryCapability(t *testing.T) {
	cases := []struct {
		leaf       string
		wantDomain string
		wantSuffix string
	}{
		{"office_garden_humidity", "sensor", "humidity"}, // keyword table still applies when it matches
		{"boiler_outdoortemp", "sensor", ""},             // no keyword match -- falls back to "sensor", not the bare leaf
		{"thermostat_hc1_switchprogmode", "sensor", ""},
	}
	for _, c := range cases {
		domain, suffix := recognizeDiscoveryCapability(c.leaf)
		if domain != c.wantDomain || suffix != c.wantSuffix {
			t.Errorf("recognizeDiscoveryCapability(%q) = (%q, %q), want (%q, %q)", c.leaf, domain, suffix, c.wantDomain, c.wantSuffix)
		}
	}
}

func TestUsedDiscoveryLeavesFiltersByGateway(t *testing.T) {
	links := map[string]TDiscoveryEntityLink{
		"sensor.physical:garage_door/temperature": {GatewayDeviceID: "discovery.ems_esp_boiler", Leaf: "boiler_outdoortemp"},
		"sensor.physical:garage_door/other":       {GatewayDeviceID: "discovery.ems_esp_thermostat", Leaf: "thermostat_hc1"},
	}
	used := usedDiscoveryLeaves(links, "discovery.ems_esp_boiler")
	if !used["boiler_outdoortemp"] {
		t.Errorf("expected boiler_outdoortemp marked used, got %v", used)
	}
	if used["thermostat_hc1"] {
		t.Errorf("expected thermostat_hc1 (a different gateway) NOT marked used, got %v", used)
	}
}

func TestBuildDiscoverySuggestionReportExcludesUsedAndUnresolved(t *testing.T) {
	statusByGateway := map[string]TDiscoveryExistenceStatusPayload{
		"discovery.ems_esp_boiler": {
			"boiler_outdoortemp": "known-to-exist", // already used -- excluded
			"boiler_heatingpump": "known-to-exist", // not yet claimed -- suggested
			"boiler_lastcode":    "not-known-to-exist",
			"boiler_servicecode": "known-not-to-exist",
		},
	}
	links := map[string]TDiscoveryEntityLink{
		"sensor.physical:garage_door/temperature": {GatewayDeviceID: "discovery.ems_esp_boiler", Leaf: "boiler_outdoortemp"},
	}

	report := buildDiscoverySuggestionReport(statusByGateway, links)
	if !strings.Contains(report, "device discovery.ems_esp_boiler with:") {
		t.Errorf("report missing device block:\n%s", report)
	}
	if !strings.Contains(report, "boiler_heatingpump;") {
		t.Errorf("report missing the not-yet-claimed leaf:\n%s", report)
	}
	for _, unwanted := range []string{"boiler_outdoortemp;", "boiler_lastcode", "boiler_servicecode"} {
		if strings.Contains(report, unwanted) {
			t.Errorf("report must exclude %q (used/unresolved/confirmed-gone), got:\n%s", unwanted, report)
		}
	}
}

func TestBuildDiscoverySuggestionReportEmptyWhenNothingToSuggest(t *testing.T) {
	statusByGateway := map[string]TDiscoveryExistenceStatusPayload{
		"discovery.ems_esp_boiler": {"boiler_outdoortemp": "not-known-to-exist"},
	}
	if report := buildDiscoverySuggestionReport(statusByGateway, nil); strings.TrimSpace(report) != "" {
		t.Errorf("expected an empty report when nothing is known-to-exist, got:\n%s", report)
	}
}

func TestGenerateDiscoverySuggestionsWritesFileFromCache(t *testing.T) {
	definitionDir := t.TempDir()
	outputRoot := t.TempDir()
	seedDiscoveryExistenceCache(t, definitionDir, "discovery.ems_esp_boiler", TDiscoveryExistenceStatusPayload{
		"boiler_outdoortemp": "known-to-exist",
	})

	gateways := map[string]TDiscoveryGatewayDevice{
		"discovery.ems_esp_boiler": {DeviceID: "discovery.ems_esp_boiler", Capabilities: map[string]TDiscoveryCapability{}},
	}
	ctx := TPhysicalGenerationContext{HasMQTTSecrets: true, MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}

	if err := generateDiscoverySuggestions(definitionDir, outputRoot, gateways, nil, ctx); err != nil {
		t.Fatalf("generateDiscoverySuggestions error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outputRoot, "suggestions", "discovery.txt"))
	if err != nil {
		t.Fatalf("expected suggestions/discovery.txt to be written: %v", err)
	}
	if !strings.Contains(string(data), "boiler_outdoortemp") {
		t.Errorf("suggestions file = %s, want it to mention boiler_outdoortemp", data)
	}
}

func TestGenerateDiscoverySuggestionsNoGatewaysIsNoop(t *testing.T) {
	definitionDir := t.TempDir()
	outputRoot := t.TempDir()
	ctx := TPhysicalGenerationContext{HasMQTTSecrets: true, MQTTSecrets: TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"}}
	if err := generateDiscoverySuggestions(definitionDir, outputRoot, map[string]TDiscoveryGatewayDevice{}, nil, ctx); err != nil {
		t.Errorf("no declared gateways at all must be a no-op, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputRoot, "suggestions", "discovery.txt")); !os.IsNotExist(err) {
		t.Errorf("expected no suggestions file to be written")
	}
}
