package main

import "testing"

// TestDropShadowedDerivedAggregates: a space's derived media switch yields to the one player's
// wrapper switch positioned at the space's own name (PROJECT.md 17g), so the space has a single
// "switch.social/house/kitchen/media" -- not a second declaration of it.
func TestDropShadowedDerivedAggregates(t *testing.T) {
	admin := newAdministrationState()
	admin.EntityRecordsBySpace["social/house/kitchen"] = []TEntityRecord{
		{Name: "switch.social/house/kitchen/media", Provenance: "derived: media social/house/kitchen"},
		{Name: "switch.social/house/kitchen/media", Provenance: "Conceptual.def:721 → switch.social:media from appliance.sonos_kitchen media", MediaSwitchPlayerName: "media_player.social_house_kitchen_sonos"},
		{Name: "switch.social/house/kitchen/space", Provenance: "derived: space off social/house/kitchen"},
	}
	dropShadowedDerivedAggregates(admin)
	records := admin.EntityRecordsBySpace["social/house/kitchen"]
	if len(records) != 2 || records[0].MediaSwitchPlayerName == "" || records[1].Name != "switch.social/house/kitchen/space" {
		t.Errorf("records = %+v, want the positioned media switch and the (unshadowed) space switch", records)
	}
}
