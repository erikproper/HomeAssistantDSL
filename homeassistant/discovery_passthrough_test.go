package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectDiscoveryPassthroughRulesParsesDedupesAndSorts(t *testing.T) {
	content := `
discovery_passthrough "zigbee2mqtt/" from "homeassistant.physical";
discovery_passthrough "zwave/" from "homeassistant.physical";
discovery_passthrough "zigbee2mqtt/" from "homeassistant.physical";
# discovery_passthrough "commented/" from "homeassistant.physical";
not a passthrough statement at all
`
	got := collectDiscoveryPassthroughRules(content, t.TempDir())
	want := []TDiscoveryPassthroughRule{
		{TopicPrefix: "zigbee2mqtt/", SourcePrefix: "homeassistant.physical"},
		{TopicPrefix: "zwave/", SourcePrefix: "homeassistant.physical"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestCollectDiscoveryPassthroughRulesIgnoresLinesInsideIntegrationBlocks(t *testing.T) {
	content := `
integration hosts with:
  device host.smarty smarty cpu;
end;
discovery_passthrough "zigbee2mqtt/" from "homeassistant.physical";
`
	got := collectDiscoveryPassthroughRules(content, t.TempDir())
	want := TDiscoveryPassthroughRule{TopicPrefix: "zigbee2mqtt/", SourcePrefix: "homeassistant.physical"}
	if len(got) != 1 || got[0] != want {
		t.Errorf("got %+v, want [%+v]", got, want)
	}
}

// TestCollectDiscoveryPassthroughRulesResolvesSettingsReference is the regression test for the
// real gap found live 2026-09-14: Vienna's own declaration used
// `from "${mqtt_discovery_physical}"` (a Settings.def reference, the natural way to write it,
// matching every other Settings.def-backed field in this DSL) rather than the literal resolved
// string -- silently captured as the literal text "${mqtt_discovery_physical}" before this fix,
// which could never match discoveryFile.PhysicalPrefix, so the coordinator relayed nothing at all.
func TestCollectDiscoveryPassthroughRulesResolvesSettingsReference(t *testing.T) {
	definitionDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(definitionDir, "Settings.def"), []byte(`${mqtt_discovery_physical} = "homeassistant.physical";`), 0o644); err != nil {
		t.Fatalf("writing Settings.def: %v", err)
	}
	content := `discovery_passthrough "zigbee2mqtt/" from "${mqtt_discovery_physical}";`

	got := collectDiscoveryPassthroughRules(content, definitionDir)
	want := TDiscoveryPassthroughRule{TopicPrefix: "zigbee2mqtt/", SourcePrefix: "homeassistant.physical"}
	if len(got) != 1 || got[0] != want {
		t.Errorf("got %+v, want [%+v] -- \"${mqtt_discovery_physical}\" must resolve, not be taken literally", got, want)
	}
}

func TestGenerateDiscoveryPassthroughFileWritesDeclaredRules(t *testing.T) {
	outputRoot := t.TempDir()
	rules := []TDiscoveryPassthroughRule{
		{TopicPrefix: "zigbee2mqtt/", SourcePrefix: "homeassistant.physical"},
	}
	if err := generateDiscoveryPassthroughFile(outputRoot, rules, "homeassistant.physical"); err != nil {
		t.Fatalf("generateDiscoveryPassthroughFile error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outputRoot, "coordinator", "discovery_passthrough.yaml"))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "passthrough:\n  - topic_prefix: \"zigbee2mqtt/\"\n    source_prefix: \"homeassistant.physical\"\n") {
		t.Errorf("generated content = %q, want it to contain the declared rule", content)
	}
}

func TestGenerateDiscoveryPassthroughFileSkipsWhenEmpty(t *testing.T) {
	outputRoot := t.TempDir()
	if err := generateDiscoveryPassthroughFile(outputRoot, nil, "homeassistant.physical"); err != nil {
		t.Fatalf("generateDiscoveryPassthroughFile error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputRoot, "coordinator", "discovery_passthrough.yaml")); !os.IsNotExist(err) {
		t.Errorf("expected no file to be written when there are no declared rules")
	}
}

// TestGenerateDiscoveryPassthroughFileRemovesStaleFileWhenRulesGoEmpty is the regression test for
// a real bug found live 2026-09-19: an absent discovery_passthrough.yaml must be AUTHORITATIVE
// ("coordinator, relay nothing"), not just "the generator didn't bother writing this run" -- a
// house that HAD passthrough rules declared and then removed the declaration (e.g. finishing an
// item-4 migration) used to leave the earlier run's file in place forever, silently keeping the
// coordinator relaying traffic it was explicitly told to stop. Mirrors
// generateDiscoverySuggestions' own identical "empty report removes the stale file" fix.
func TestGenerateDiscoveryPassthroughFileRemovesStaleFileWhenRulesGoEmpty(t *testing.T) {
	outputRoot := t.TempDir()
	rules := []TDiscoveryPassthroughRule{
		{TopicPrefix: "zigbee2mqtt/", SourcePrefix: "homeassistant.physical"},
	}
	if err := generateDiscoveryPassthroughFile(outputRoot, rules, "homeassistant.physical"); err != nil {
		t.Fatalf("generateDiscoveryPassthroughFile error (seeding): %v", err)
	}
	passthroughPath := filepath.Join(outputRoot, "coordinator", "discovery_passthrough.yaml")
	if _, err := os.Stat(passthroughPath); err != nil {
		t.Fatalf("precondition failed: expected the file to exist after seeding: %v", err)
	}

	if err := generateDiscoveryPassthroughFile(outputRoot, nil, "homeassistant.physical"); err != nil {
		t.Fatalf("generateDiscoveryPassthroughFile error (clearing): %v", err)
	}
	if _, err := os.Stat(passthroughPath); !os.IsNotExist(err) {
		t.Errorf("expected the stale file to be removed once rules go empty, got err=%v", err)
	}
}

// TestGenerateDiscoveryPassthroughFileStillWritesMismatchedSource is a regression guard: a rule
// whose declared source prefix doesn't match ${mqtt_discovery_physical} must still be written
// through (with a warning, not silently dropped) -- the coordinator's own subscription scope is
// what actually decides whether it can ever match live traffic, not the generator.
func TestGenerateDiscoveryPassthroughFileStillWritesMismatchedSource(t *testing.T) {
	outputRoot := t.TempDir()
	rules := []TDiscoveryPassthroughRule{
		{TopicPrefix: "zigbee2mqtt/", SourcePrefix: "some.other.prefix"},
	}
	if err := generateDiscoveryPassthroughFile(outputRoot, rules, "homeassistant.physical"); err != nil {
		t.Fatalf("generateDiscoveryPassthroughFile error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outputRoot, "coordinator", "discovery_passthrough.yaml"))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	if !strings.Contains(string(data), "some.other.prefix") {
		t.Errorf("generated content = %q, want the mismatched-source rule still written", string(data))
	}
}
