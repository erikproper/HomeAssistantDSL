/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: GeneratorMediaSwitchTest
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 30.09.2026
 *
 */

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateMediaSwitchesHandlesMultiSegmentPlayerPath is the regression test for a real bug
// found live 2026-09-30 (Junglinster's "appliance.sonos_headphones", a local-kind media device
// positioned two segments deep under its space, "sonos/headphones", unlike every other media
// player in either house which sits directly under its own space with a single-segment leaf, e.g.
// "garage/sonos"): the house-level aggregate media switch's own "turn everything off" list derived
// each leaf player's own shortname by taking only the LAST path segment after stripping the
// space's own prefix -- correct by coincidence for a single-segment leaf (last segment == the
// whole remaining sub-path), but silently wrong for a multi-segment one, where it dropped every
// segment except the last ("headphones"), producing a switch name
// ("switch.house/headphones/media") that was never actually generated anywhere, instead of the
// real one ("switch.house/sonos/headphones/media"). Confirmed live via presence.go's own
// referential-integrity check flagging exactly this bogus reference.
func TestGenerateMediaSwitchesHandlesMultiSegmentPlayerPath(t *testing.T) {
	const miniDSL = `conceptual layer with:
  space social:house with:
    device appliance.sonos_headphones with:
      entity media_player.social:sonos/headphones from media_player.core;
      entity switch.social:sonos/headphones/media;
    end;
  end;
end;`

	localDevicesByID := map[string]TLocalDevice{
		"appliance.sonos_headphones": {
			DeviceID: "appliance.sonos_headphones",
			Capabilities: map[string]TLocalCapability{
				"core":  {Domain: "media_player"},
				"media": {Domain: "switch", MediaPlayerEntity: "media_player.social_house_sonos_headphones"},
			},
		},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, localDevicesByID, nil, nil)
	if err != nil {
		t.Fatalf("ParseEntitiesAndFillAdministration: %v", err)
	}

	outDir := t.TempDir()
	if err := generateMediaSwitches(outDir, result.Administration); err != nil {
		t.Fatalf("generateMediaSwitches: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(outDir, "entities", "template", "switch", "social", "switch.social_house_media.yaml"))
	if err != nil {
		t.Fatalf("ReadFile switch.social_house_media.yaml: %v", err)
	}
	got := string(content)

	const want = "switch.social_house_sonos_headphones_media"
	if !strings.Contains(got, want) {
		t.Errorf("switch.social_house_media.yaml does not reference %q (the real, multi-segment leaf switch), got:\n%s", want, got)
	}
	const wrong = "switch.social_house_headphones_media"
	if strings.Contains(got, wrong) {
		t.Errorf("switch.social_house_media.yaml wrongly references %q (the bogus, truncated-to-last-segment name), got:\n%s", wrong, got)
	}
}
