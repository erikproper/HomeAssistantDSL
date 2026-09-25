package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateCoverAwayEntitiesNeverDeclaresInitial is the regression test for a real bug found
// live 2026-09-21 (both houses, present since 2026-08-19): the three cover-away helper entities
// (closed_position_when_away, opened_position_when_away, does_when_away) each hardcoded an
// "initial:" value. Per Home Assistant's own input_number/input_select source
// (async_added_to_hass only attempts RestoreEntity when the value/option is still None going in,
// and __init__ sets it immediately from config["initial"] when present), a configured "initial:"
// permanently disables restore -- not just on first setup -- so a user's own choice was silently
// wiped back to the hardcoded default on every restart AND every homeassistant.reload_all (which
// this project's own meta-reload mechanism calls on every deploy). None of the three must ever
// emit "initial:" again.
func TestGenerateCoverAwayEntitiesNeverDeclaresInitial(t *testing.T) {
	admin := newAdministrationState()
	admin.EntityRecordsBySpace["root"] = []TEntityRecord{
		{Name: "cover.social:apartment/living_room/living", Identity: TEntityIdentity{Domain: "cover", Sphere: "social", Path: "apartment/living_room/living"}},
	}

	outputDir := t.TempDir()
	if err := generateCoverAwayEntities(outputDir, admin); err != nil {
		t.Fatalf("generateCoverAwayEntities error: %v", err)
	}

	files := []string{
		filepath.Join(outputDir, "entities", "input_number", "social", "input_number.social_cover_apartment_living_room_living_closed_position_when_away.yaml"),
		filepath.Join(outputDir, "entities", "input_number", "social", "input_number.social_cover_apartment_living_room_living_opened_position_when_away.yaml"),
		filepath.Join(outputDir, "entities", "input_select", "social", "input_select.social_cover_apartment_living_room_living_does_when_away.yaml"),
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", f, err)
		}
		if strings.Contains(string(data), "initial:") {
			t.Errorf("%s contains \"initial:\" -- this must be dropped so HA's own RestoreEntity mechanism can restore a user's previous choice across restarts/reloads:\n%s", f, data)
		}
	}
}

// TestGenerateCoverAwayEntitiesDoesWhenAwayDefaultsToIgnore confirms the does_when_away options
// list still starts with "Ignore" -- the safe do-nothing default a genuinely new cover (never
// seen before, nothing to restore) falls back to via HA's own "options[0] when no initial/no
// restored state" behavior, now that "initial:" itself is gone.
func TestGenerateCoverAwayEntitiesDoesWhenAwayDefaultsToIgnore(t *testing.T) {
	admin := newAdministrationState()
	admin.EntityRecordsBySpace["root"] = []TEntityRecord{
		{Name: "cover.social:terrace/awning", Identity: TEntityIdentity{Domain: "cover", Sphere: "social", Path: "terrace/awning"}},
	}

	outputDir := t.TempDir()
	if err := generateCoverAwayEntities(outputDir, admin); err != nil {
		t.Fatalf("generateCoverAwayEntities error: %v", err)
	}

	selectFile := filepath.Join(outputDir, "entities", "input_select", "social", "input_select.social_cover_terrace_awning_does_when_away.yaml")
	data, err := os.ReadFile(selectFile)
	if err != nil {
		t.Fatalf("reading %s: %v", selectFile, err)
	}
	body := string(data)
	optionsIdx := strings.Index(body, "options:")
	if optionsIdx < 0 {
		t.Fatalf("no \"options:\" block found in %s:\n%s", selectFile, body)
	}
	firstOptionIdx := strings.Index(body[optionsIdx:], "- ")
	if firstOptionIdx < 0 || !strings.HasPrefix(strings.TrimSpace(body[optionsIdx+firstOptionIdx+2:]), "Ignore") {
		t.Errorf("expected \"Ignore\" to be the first listed option (HA's own fallback default when no initial/restored state exists), got:\n%s", body)
	}
}
