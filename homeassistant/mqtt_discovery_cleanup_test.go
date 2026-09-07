package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectMQTTDiscoveryCleanTopicsParsesDedupesAndSorts(t *testing.T) {
	content := `
mqtt_discovery_clean zigbee2mqtt;
mqtt_discovery_clean ems-esp;
mqtt_discovery_clean ems-esp;
# mqtt_discovery_clean commented-out;
not a clean statement at all
`
	got := collectMQTTDiscoveryCleanTopics(content)
	want := []string{"ems-esp", "zigbee2mqtt"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCollectMQTTDiscoveryCleanTopicsIgnoresLinesInsideIntegrationBlocks(t *testing.T) {
	// mqtt_discovery_clean is a bare top-level statement -- confirm it's still recognised
	// regardless of surrounding "integration ... with: ... end;" blocks elsewhere in the file
	// (scanGroupClauseBlocks silently skips non-header lines, so this independent pass must not
	// be tripped up by them either).
	content := `
integration hosts with:
  device host.smarty smarty cpu;
end;
mqtt_discovery_clean ems-esp;
`
	got := collectMQTTDiscoveryCleanTopics(content)
	if len(got) != 1 || got[0] != "ems-esp" {
		t.Errorf("got %v, want [ems-esp]", got)
	}
}

func TestGenerateDiscoveryCleanupFileWritesDeclaredTopics(t *testing.T) {
	outputRoot := t.TempDir()
	if err := generateDiscoveryCleanupFile(outputRoot, []string{"ems-esp", "zigbee2mqtt"}); err != nil {
		t.Fatalf("generateDiscoveryCleanupFile error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outputRoot, "coordinator", "discovery_cleanup.yaml"))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "clean_topics:\n  - ems-esp\n  - zigbee2mqtt\n") {
		t.Errorf("generated content = %q, want it to contain the clean_topics list", content)
	}
}

func TestGenerateDiscoveryCleanupFileSkipsWhenEmpty(t *testing.T) {
	outputRoot := t.TempDir()
	if err := generateDiscoveryCleanupFile(outputRoot, nil); err != nil {
		t.Fatalf("generateDiscoveryCleanupFile error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputRoot, "coordinator", "discovery_cleanup.yaml")); !os.IsNotExist(err) {
		t.Errorf("expected no file to be written when there are no declared topics")
	}
}

func TestGenerateDiscoveryCleanupFileRejectsCoordinatorNodeID(t *testing.T) {
	outputRoot := t.TempDir()
	if err := generateDiscoveryCleanupFile(outputRoot, []string{"coordinator", "ems-esp"}); err != nil {
		t.Fatalf("generateDiscoveryCleanupFile error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outputRoot, "coordinator", "discovery_cleanup.yaml"))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "coordinator") {
		t.Errorf("generated content = %q, must not include the rejected \"coordinator\" node_id", content)
	}
	if !strings.Contains(content, "ems-esp") {
		t.Errorf("generated content = %q, want the other, legitimate topic still written", content)
	}
}
