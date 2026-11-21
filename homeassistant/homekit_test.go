package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateHomekitConfiguration: an exposed entity becomes an include_entities line (with its
// spoken name as entity_config), an unknown one a warning, a bridge exposing nothing is left out,
// and configuration.yaml includes the file (PROJECT.md 62).
func TestGenerateHomekitConfiguration(t *testing.T) {
	admin := newAdministrationState()
	const fan = "fan.social/apartment/living_room/cycling"
	admin.AppendEntityRecord("social/apartment/living_room", TEntityRecord{Name: fan, Identity: extractEntityIdentity(fan)})
	dir := t.TempDir()
	bridges := []THomekitBridge{
		{Name: "Vienna", Port: 21065, Exposed: []THomekitExposure{{Name: fan, Spoken: "cycling fan"}, {Name: "light.social/nowhere"}}},
		{Name: "Empty", Port: 21066, Exposed: []THomekitExposure{{Name: "light.social/nowhere"}}},
	}
	warnings, err := generateHomekitConfiguration(dir, bridges, admin)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 3 || !strings.Contains(warnings[2], "exposes nothing") {
		t.Errorf("warnings %v, want two unknown entities and the empty bridge", warnings)
	}
	data, err := os.ReadFile(filepath.Join(dir, "integrations", homekitFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"name: Vienna", "port: 21065", "- fan.social_apartment_living_room_cycling", "name: cycling fan"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("homekit.yaml lacks %q:\n%s", want, data)
		}
	}
	if strings.Contains(string(data), "Empty") {
		t.Errorf("the empty bridge was written:\n%s", data)
	}
	config, err := os.ReadFile(filepath.Join(dir, "configuration.yaml"))
	if err != nil || !strings.Contains(string(config), "homekit: !include integrations/homekit.yaml") {
		t.Errorf("configuration.yaml (err=%v):\n%s", err, config)
	}
}

// TestGenerateHomekitConfigurationNone: without a bridge, configuration.yaml has no "homekit:".
func TestGenerateHomekitConfigurationNone(t *testing.T) {
	dir := t.TempDir()
	if err := generateConfigurationFile(dir, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := generateHomekitConfiguration(dir, nil, newAdministrationState()); err != nil {
		t.Fatal(err)
	}
	config, _ := os.ReadFile(filepath.Join(dir, "configuration.yaml"))
	if strings.Contains(string(config), "homekit") {
		t.Errorf("configuration.yaml:\n%s", config)
	}
}
