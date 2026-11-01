package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestVolumeSync: the block is parsed with both member spellings (DSL name and final entity id),
// and generates the sync automation (mode single, settle delay, reconcile) and the start one
// (average within 5..15).
func TestVolumeSync(t *testing.T) {
	const dsl = `space social:house as area with:
  space social:living_room as area with:
    volume sync of:
      media_player.social_house_living_room_sonos;
      media_player.social:tv;   # the Bravia
    end;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	syncs := result.Administration.VolumeSyncs
	if len(syncs) != 1 || len(syncs[0].Members) != 2 || syncs[0].Members[0] != "media_player.social_house_living_room_sonos" {
		t.Fatalf("volume syncs %+v", syncs)
	}
	outDir := t.TempDir()
	if err := generateVolumeSyncAutomations(outDir, result.Administration); err != nil {
		t.Fatalf("generateVolumeSyncAutomations: %v", err)
	}
	read := func(name string) string {
		content, err := os.ReadFile(filepath.Join(outDir, "automation", "social", name))
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		return string(content)
	}
	sync := read("automation.social_house_living_room_volume_sync.yaml")
	for _, want := range []string{
		"- media_player.social_house_living_room_sonos\n", "- media_player.social_house_living_room_tv\n",
		"attribute: volume_level\n", "mode: single\n", "max_exceeded: silent\n",
		"service: media_player.volume_set\n", `timeout: "00:00:05"`, "until: ",
	} {
		if !strings.Contains(sync, want) {
			t.Errorf("sync automation lacks %q:\n%s", want, sync)
		}
	}
	if strings.Contains(sync, "delay:") {
		t.Errorf("sync must not ignore changes for a fixed time (dropped Erik's own changes, 2026-11-01):\n%s", sync)
	}
	start := read("automation.social_house_living_room_volume_sync_start.yaml")
	for _, want := range []string{"event: start\n", "0.05] | max, 0.15] | min", `timeout: "00:05:00"`} {
		if !strings.Contains(start, want) {
			t.Errorf("start automation lacks %q:\n%s", want, start)
		}
	}
	if strings.Contains(start, "automation_reloaded") {
		t.Errorf("start automation must not run on a deploy:\n%s", start)
	}
}
