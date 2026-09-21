package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoveryExistenceAggregateStatusTopic(t *testing.T) {
	got := discoveryExistenceAggregateStatusTopic()
	want := "discovery_existence/state"
	if got != want {
		t.Errorf("discoveryExistenceAggregateStatusTopic() = %q, want %q", got, want)
	}
}

func seedDiscoveryExistenceAggregateCache(t *testing.T, definitionDir string, payload TDiscoveryExistenceAggregatePayload) {
	t.Helper()
	cachePath := discoveryExistenceAggregateCachePath(definitionDir)
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

func TestFetchDiscoveryExistenceAggregateFallsBackToLocalCacheOnFetchFailure(t *testing.T) {
	definitionDir := t.TempDir()
	seedDiscoveryExistenceAggregateCache(t, definitionDir, TDiscoveryExistenceAggregatePayload{
		"discovery.ems_esp": {"boiler_outdoortemp": "known-to-exist"},
	})

	ctx := TPhysicalGenerationContext{
		MQTTSecrets:                 TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"},
		DiscoveryExistenceAggregate: &tDiscoveryExistenceFetchResult{},
	}
	status, err := fetchDiscoveryExistenceAggregate(definitionDir, ctx)
	if err != nil {
		t.Fatalf("fetchDiscoveryExistenceAggregate error: %v -- want it to fall back to the cache instead", err)
	}
	if status["discovery.ems_esp"]["boiler_outdoortemp"] != "known-to-exist" {
		t.Errorf("got %+v, want the cached entry", status)
	}
}

func TestFetchDiscoveryExistenceAggregateFailsWhenNeitherFetchNorCacheAvailable(t *testing.T) {
	definitionDir := t.TempDir()
	ctx := TPhysicalGenerationContext{
		MQTTSecrets:                 TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"},
		DiscoveryExistenceAggregate: &tDiscoveryExistenceFetchResult{},
	}
	if _, err := fetchDiscoveryExistenceAggregate(definitionDir, ctx); err == nil {
		t.Errorf("expected an error when neither a live fetch nor a cache file is available")
	}
}

func TestFetchDiscoveryExistenceAggregateMemoizesWithinOneContext(t *testing.T) {
	definitionDir := t.TempDir()
	seedDiscoveryExistenceAggregateCache(t, definitionDir, TDiscoveryExistenceAggregatePayload{
		"discovery.ems_esp": {"boiler_outdoortemp": "known-to-exist"},
	})

	shared := &tDiscoveryExistenceFetchResult{}
	ctx := TPhysicalGenerationContext{
		MQTTSecrets:                 TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"},
		DiscoveryExistenceAggregate: shared,
	}
	if _, err := fetchDiscoveryExistenceAggregate(definitionDir, ctx); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if !shared.fetched {
		t.Fatalf("expected the shared result to be marked fetched after the first call")
	}

	// Removing the cache file after the first (successful) fetch proves the second call reuses the
	// memoized result rather than fetching again -- a second real fetch would fail with no cache and
	// no reachable broker.
	if err := os.Remove(discoveryExistenceAggregateCachePath(definitionDir)); err != nil {
		t.Fatalf("removing cache: %v", err)
	}
	status, err := fetchDiscoveryExistenceAggregate(definitionDir, ctx)
	if err != nil {
		t.Fatalf("second (memoized) fetch: %v", err)
	}
	if status["discovery.ems_esp"]["boiler_outdoortemp"] != "known-to-exist" {
		t.Errorf("got %+v, want the memoized entry", status)
	}
}

func TestCheckDiscoveryKnownNotToExistErrorsFlagsConfirmedAbsence(t *testing.T) {
	definitionDir := t.TempDir()
	seedDiscoveryExistenceAggregateCache(t, definitionDir, TDiscoveryExistenceAggregatePayload{
		"discovery.ems_esp": {
			"boiler_outdoortemp":  "known-not-to-exist",
			"thermostat_lastcode": "known-to-exist",
		},
	})

	discoveryEntityLinks := map[string]TDiscoveryEntityLink{
		"sensor.physical:garage_door/temperature": {GatewayDeviceID: "discovery.ems_esp", Leaf: "boiler_outdoortemp"},
		"sensor.physical:garage_door/lastcode":    {GatewayDeviceID: "discovery.ems_esp", Leaf: "thermostat_lastcode"},
	}

	ctx := TPhysicalGenerationContext{
		MQTTSecrets:                 TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"},
		DiscoveryExistenceAggregate: &tDiscoveryExistenceFetchResult{},
	}
	problems := checkDiscoveryKnownNotToExistErrors(definitionDir, discoveryEntityLinks, ctx)
	if len(problems) == 0 {
		t.Fatalf("expected a problem for the confirmed-absent boiler_outdoortemp leaf")
	}
	joined := strings.Join(problems, "\n")
	if !strings.Contains(joined, "boiler_outdoortemp") {
		t.Errorf("problems = %v, want it to name the boiler_outdoortemp leaf", problems)
	}
	if strings.Contains(joined, "thermostat_lastcode") {
		t.Errorf("problems = %v, want it to NOT flag thermostat_lastcode -- it's known-to-exist", problems)
	}
}

func TestCheckDiscoveryKnownNotToExistErrorsOptimisticWhenUnresolved(t *testing.T) {
	definitionDir := t.TempDir()
	seedDiscoveryExistenceAggregateCache(t, definitionDir, TDiscoveryExistenceAggregatePayload{
		// No entry at all for boiler_outdoortemp -- the coordinator hasn't observed it yet.
		"discovery.ems_esp": {},
	})

	discoveryEntityLinks := map[string]TDiscoveryEntityLink{
		"sensor.physical:garage_door/temperature": {GatewayDeviceID: "discovery.ems_esp", Leaf: "boiler_outdoortemp"},
	}

	ctx := TPhysicalGenerationContext{
		MQTTSecrets:                 TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"},
		DiscoveryExistenceAggregate: &tDiscoveryExistenceFetchResult{},
	}
	if problems := checkDiscoveryKnownNotToExistErrors(definitionDir, discoveryEntityLinks, ctx); len(problems) != 0 {
		t.Errorf("unresolved status must not be reported as a problem, got: %v", problems)
	}
}

func TestCheckDiscoveryKnownNotToExistErrorsNoLinksIsNoop(t *testing.T) {
	definitionDir := t.TempDir()
	ctx := TPhysicalGenerationContext{
		MQTTSecrets:                 TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"},
		DiscoveryExistenceAggregate: &tDiscoveryExistenceFetchResult{},
	}
	if problems := checkDiscoveryKnownNotToExistErrors(definitionDir, map[string]TDiscoveryEntityLink{}, ctx); len(problems) != 0 {
		t.Errorf("no discovery entity links at all must be a no-op, got: %v", problems)
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

	report := buildDiscoverySuggestionReport(statusByGateway, links, nil)
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

// TestBuildDiscoverySuggestionReportRHSIsBareLeaf is a regression test for a real bug found live
// 2026-09-16: the RHS used to be domain-prefixed ("sensor.boiler_heatingpump;"), but a real
// Physical.def capability line's own RHS is always the bare leaf value with no domain prefix at
// all (e.g. "switch.core 0xc4988600000fbf8e_switch_zigbee2mqtt;") -- copy-pasting a suggestion
// verbatim produced a syntactically broken line.
func TestBuildDiscoverySuggestionReportRHSIsBareLeaf(t *testing.T) {
	statusByGateway := map[string]TDiscoveryExistenceStatusPayload{
		"discovery.ems_esp_boiler": {"boiler_heatingpump": "known-to-exist"},
	}
	report := buildDiscoverySuggestionReport(statusByGateway, nil, nil)
	if !strings.Contains(report, " boiler_heatingpump;") {
		t.Errorf("expected the bare leaf as RHS with no domain prefix, got:\n%s", report)
	}
	if strings.Contains(report, "sensor.boiler_heatingpump") {
		t.Errorf("RHS must not be domain-prefixed, got:\n%s", report)
	}
}

// TestBuildDiscoverySuggestionReportExcludesDeclaredButUnpositionedLeaf is the regression test for
// a real bug found live 2026-09-17: a leaf already given a real, hand-chosen Physical.def label
// (e.g. "sensor.color_options 0x..._color_options_zigbee2mqtt;") but deliberately left
// unpositioned at the conceptual layer (a diagnostic-only capability, the desktop/nespresso
// precedent) kept reappearing in this report forever, always as "sensor. <leaf>; # not
// recognized" -- confusingly suggesting it had never been labelled at all, when it had.
func TestBuildDiscoverySuggestionReportExcludesDeclaredButUnpositionedLeaf(t *testing.T) {
	statusByGateway := map[string]TDiscoveryExistenceStatusPayload{
		"discovery.vidja_left_1": {"0x84b4dbfffefbb43a_color_options_zigbee2mqtt": "known-to-exist"},
	}
	gateways := map[string]TDiscoveryGatewayDevice{
		"discovery.vidja_left_1": {
			DeviceID: "discovery.vidja_left_1",
			Capabilities: map[string]TDiscoveryCapability{
				"color_options": {Domain: "sensor", Leaf: "0x84b4dbfffefbb43a_color_options_zigbee2mqtt"},
			},
		},
	}
	// No discoveryEntityLinks at all -- the capability is declared but never positioned.
	report := buildDiscoverySuggestionReport(statusByGateway, nil, gateways)
	if strings.TrimSpace(report) != "" {
		t.Errorf("expected an already-declared (even if unpositioned) leaf to be excluded, got:\n%s", report)
	}
}

func TestBuildDiscoverySuggestionReportEmptyWhenNothingToSuggest(t *testing.T) {
	statusByGateway := map[string]TDiscoveryExistenceStatusPayload{
		"discovery.ems_esp_boiler": {"boiler_outdoortemp": "not-known-to-exist"},
	}
	if report := buildDiscoverySuggestionReport(statusByGateway, nil, nil); strings.TrimSpace(report) != "" {
		t.Errorf("expected an empty report when nothing is known-to-exist, got:\n%s", report)
	}
}

func TestGenerateDiscoverySuggestionsWritesFileFromCache(t *testing.T) {
	definitionDir := t.TempDir()
	outputRoot := t.TempDir()
	seedDiscoveryExistenceAggregateCache(t, definitionDir, TDiscoveryExistenceAggregatePayload{
		"discovery.ems_esp_boiler": {"boiler_outdoortemp": "known-to-exist"},
	})

	gateways := map[string]TDiscoveryGatewayDevice{
		"discovery.ems_esp_boiler": {DeviceID: "discovery.ems_esp_boiler", Capabilities: map[string]TDiscoveryCapability{}},
	}
	ctx := TPhysicalGenerationContext{
		HasMQTTSecrets:              true,
		MQTTSecrets:                 TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"},
		DiscoveryExistenceAggregate: &tDiscoveryExistenceFetchResult{},
	}

	if err := generateDiscoverySuggestions(definitionDir, outputRoot, gateways, nil, nil, ctx); err != nil {
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
	ctx := TPhysicalGenerationContext{
		HasMQTTSecrets:              true,
		MQTTSecrets:                 TMQTTBrokerSecrets{Server: "127.0.0.1", Port: "1"},
		DiscoveryExistenceAggregate: &tDiscoveryExistenceFetchResult{},
	}
	if err := generateDiscoverySuggestions(definitionDir, outputRoot, map[string]TDiscoveryGatewayDevice{}, nil, nil, ctx); err != nil {
		t.Errorf("no declared gateways at all must be a no-op, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputRoot, "suggestions", "discovery.txt")); !os.IsNotExist(err) {
		t.Errorf("expected no suggestions file to be written")
	}
}
