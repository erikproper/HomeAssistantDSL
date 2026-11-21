package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateHomekitManifest: an exposed entity is listed by its entity_id, an unknown one is a
// warning, and a bridge exposing nothing is left out (PROJECT.md 62).
func TestGenerateHomekitManifest(t *testing.T) {
	admin := newAdministrationState()
	const fan = "fan.social/apartment/living_room/cycling"
	admin.AppendEntityRecord("social/apartment/living_room", TEntityRecord{Name: fan, Identity: extractEntityIdentity(fan)})
	dir := t.TempDir()
	bridges := []THomekitBridge{
		{Name: "HASS Bridge 6Z", Port: 21065, Exposed: []THomekitExposure{{Name: fan}, {Name: "light.social/nowhere"}}},
		{Name: "Empty", Port: 21066, Exposed: []THomekitExposure{{Name: "light.social/nowhere"}}},
	}
	warnings, err := generateHomekitManifest(dir, bridges, admin)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 3 || !strings.Contains(warnings[2], "exposes nothing") {
		t.Errorf("warnings %v, want two unknown entities and the empty bridge", warnings)
	}
	data, err := os.ReadFile(filepath.Join(dir, homekitManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	var manifest []THomekitManifestBridge
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 1 || manifest[0].Name != "HASS Bridge 6Z" || manifest[0].Port != 21065 ||
		len(manifest[0].Entities) != 1 || manifest[0].Entities[0] != "fan.social_apartment_living_room_cycling" {
		t.Errorf("manifest %+v", manifest)
	}
}

// TestHomekitFilterUnchanged: the bridge's current filter, as the options flow shows it, against
// the wanted entities.
func TestHomekitFilterUnchanged(t *testing.T) {
	want := []string{"switch.b", "fan.a"}
	for _, c := range []struct {
		mode              string
		domains, entities []string
		unchanged         bool
	}{
		{"include", []string{"switch", "fan"}, []string{"fan.a", "switch.b"}, true},
		{"include", []string{"switch", "fan", "light"}, []string{"fan.a", "switch.b"}, false}, // all lights too
		{"include", []string{"switch", "fan"}, []string{"fan.a"}, false},
		{"exclude", []string{"switch", "fan"}, []string{"fan.a", "switch.b"}, false},
	} {
		if got := homekitFilterUnchanged(c.mode, c.domains, c.entities, want); got != c.unchanged {
			t.Errorf("%+v: unchanged %v", c, got)
		}
	}
}
