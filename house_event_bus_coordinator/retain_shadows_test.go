package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestShadowedTopics: deduplicated and sorted, never a shadow of a shadow.
func TestShadowedTopics(t *testing.T) {
	got := shadowedTopics([]string{"z2m/b", "z2m/a", "z2m/b", "", "z2m/a/retain"})
	if want := []string{"z2m/a", "z2m/b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("shadowedTopics = %v, want %v", got, want)
	}
}

// TestStaleRetainShadows: only what the previous manifest has and the current list hasn't.
func TestStaleRetainShadows(t *testing.T) {
	got := staleRetainShadows([]string{"z2m/old", "z2m/kept", "z2m/gone"}, []string{"z2m/kept", "z2m/new"})
	if want := []string{"z2m/gone", "z2m/old"}; !reflect.DeepEqual(got, want) {
		t.Errorf("staleRetainShadows = %v, want %v", got, want)
	}
	if got := staleRetainShadows(nil, []string{"z2m/new"}); len(got) != 0 {
		t.Errorf("no previous manifest: want nothing stale, got %v", got)
	}
}

// TestStartRetainShadows: the stale shadow is cleared, the new manifest published, and a listed
// topic's retained message posted on its shadow, unchanged and only for listed topics.
func TestStartRetainShadows(t *testing.T) {
	client := &fakeClient{retained: []fakeMessage{
		{topic: retainShadowsManifestTopic, payload: []byte(`["z2m/plug","z2m/old"]`)},
		{topic: "z2m/plug", payload: []byte(`{"power":220}`)},
		{topic: "z2m/unlisted", payload: []byte(`{"power":1}`)},
	}}
	if err := startRetainShadows(client, TRetainShadowsFile{Topics: []string{"z2m/plug", "z2m/new"}}); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, p := range client.publishedSnapshot() {
		got[p.topic] = string(p.payload)
	}
	want := map[string]string{
		"z2m/old/retain":           "",
		retainShadowsManifestTopic: `["z2m/new","z2m/plug"]`,
		"z2m/plug/retain":          `{"power":220}`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("published = %v, want %v", got, want)
	}
}

// TestLoadRetainShadowsFile: a missing file means no shadows.
func TestLoadRetainShadowsFile(t *testing.T) {
	dir := t.TempDir()
	file, err := loadRetainShadowsFile(filepath.Join(dir, "retain_shadows.yaml"))
	if err != nil || len(file.Topics) != 0 {
		t.Errorf("missing file: got %v, %v", file, err)
	}
	path := filepath.Join(dir, "retain_shadows.yaml")
	if err := os.WriteFile(path, []byte("topics:\n  - z2m/plug\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if file, err := loadRetainShadowsFile(path); err != nil || !reflect.DeepEqual(file.Topics, []string{"z2m/plug"}) {
		t.Errorf("got %v, %v", file, err)
	}
}
