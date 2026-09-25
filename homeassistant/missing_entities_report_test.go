package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingEntitiesReportAddSkipsEmptySections(t *testing.T) {
	report := &TMissingEntitiesReport{}
	report.Add("kind-2", nil)
	report.Add("kind-3", []string{})
	if !report.Empty() {
		t.Errorf("expected the report to stay empty after adding only empty sections, got %+v", report.Sections)
	}
}

func TestMissingEntitiesReportRenderGroupsByKind(t *testing.T) {
	report := &TMissingEntitiesReport{}
	report.Add("discovery gateway leaves (kind-2)", []string{"sensor.foo: confirmed not to exist"})
	report.Add("home_assistant bridge capabilities (kind-3)", []string{"co2 (device hass.bar): confirmed not to exist"})

	if report.Empty() {
		t.Fatalf("expected a non-empty report")
	}
	rendered := report.Render()
	if !strings.Contains(rendered, "## discovery gateway leaves (kind-2)") {
		t.Errorf("rendered report missing kind-2 header:\n%s", rendered)
	}
	if !strings.Contains(rendered, "- sensor.foo: confirmed not to exist") {
		t.Errorf("rendered report missing kind-2 item:\n%s", rendered)
	}
	if !strings.Contains(rendered, "## home_assistant bridge capabilities (kind-3)") {
		t.Errorf("rendered report missing kind-3 header:\n%s", rendered)
	}
	if !strings.Contains(rendered, "- co2 (device hass.bar): confirmed not to exist") {
		t.Errorf("rendered report missing kind-3 item:\n%s", rendered)
	}
}

func TestGenerateMissingEntitiesReportWritesFileWhenNonEmpty(t *testing.T) {
	outputRoot := t.TempDir()
	report := &TMissingEntitiesReport{}
	report.Add("kind-2", []string{"sensor.foo: confirmed not to exist"})

	if err := generateMissingEntitiesReport(outputRoot, report); err != nil {
		t.Fatalf("generateMissingEntitiesReport error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outputRoot, "suggestions", "missing.txt"))
	if err != nil {
		t.Fatalf("expected suggestions/missing.txt to be written: %v", err)
	}
	if !strings.Contains(string(data), "sensor.foo") {
		t.Errorf("suggestions/missing.txt = %s, want it to mention sensor.foo", data)
	}
}

// TestGenerateMissingEntitiesReportRemovesStaleFileWhenEmpty confirms an empty report is treated
// as an authoritative "nothing missing right now" -- same convention every other suggestions file
// already follows (e.g. generateDiscoverySuggestions): a stale file from an earlier run, when a
// problem has since been resolved, must not linger looking current.
func TestGenerateMissingEntitiesReportRemovesStaleFileWhenEmpty(t *testing.T) {
	outputRoot := t.TempDir()
	stalePath := filepath.Join(outputRoot, "suggestions", "missing.txt")
	if err := os.MkdirAll(filepath.Dir(stalePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(stalePath, []byte("stale content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := generateMissingEntitiesReport(outputRoot, &TMissingEntitiesReport{}); err != nil {
		t.Fatalf("generateMissingEntitiesReport error: %v", err)
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Errorf("expected the stale suggestions/missing.txt to be removed, got err=%v", err)
	}
}

func TestGenerateMissingEntitiesReportNoFileWhenNeverExisted(t *testing.T) {
	outputRoot := t.TempDir()
	if err := generateMissingEntitiesReport(outputRoot, &TMissingEntitiesReport{}); err != nil {
		t.Fatalf("generateMissingEntitiesReport error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputRoot, "suggestions", "missing.txt")); !os.IsNotExist(err) {
		t.Errorf("expected no suggestions/missing.txt to be written")
	}
}
