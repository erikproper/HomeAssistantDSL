package main

import (
	"strings"
	"testing"
)

// TestValidateHassBridgeAvailabilitySources is a regression test for a real bug found live
// 2026-09-15: a typo'd entity_id inside an "... is available;" hassbridge capability source went
// completely undetected (no build/generate error, just a permanently-unavailable condition).
func TestValidateHassBridgeAvailabilitySources(t *testing.T) {
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		// Matches one of its own OTHER capabilities' sources -- the real, working convention
		// (e.g. node.fritz_box, node.vienna) -- must not be flagged.
		"node.compliant": {DeviceID: "node.compliant", Capabilities: map[string]THassBridgeCapability{
			"node":     {Domain: "binary_sensor", Sources: map[string]string{"vienna": "sensor.processor_use is available"}},
			"cpu/load": {Domain: "sensor", Sources: map[string]string{"vienna": "sensor.processor_use"}},
		}},
		// Typo: "sensor.fritz_box_7590_ax_gb_recieved" (transposed) doesn't match the real source
		// "sensor.fritz_box_7590_ax_gb_received" declared on the sibling "gb_received" capability.
		"node.typo": {DeviceID: "node.typo", Capabilities: map[string]THassBridgeCapability{
			"node":        {Domain: "binary_sensor", Sources: map[string]string{"vienna": "sensor.fritz_box_7590_ax_gb_recieved is available"}},
			"gb_received": {Domain: "sensor", Sources: map[string]string{"vienna": "sensor.fritz_box_7590_ax_gb_received"}},
		}},
		// No "is available" capability at all -- nothing to check, must not be flagged.
		"node.plain": {DeviceID: "node.plain", Capabilities: map[string]THassBridgeCapability{
			"status": {Domain: "sensor", Sources: map[string]string{"vienna": "sensor.something"}},
		}},
	}

	warnings := validateHassBridgeAvailabilitySources(hassBridgeDevicesByID)
	joined := strings.Join(warnings, "\n")

	if !strings.Contains(joined, `"node.typo"`) {
		t.Errorf("expected a warning for node.typo's mismatched \"is available\" source, got: %v", warnings)
	}
	if !strings.Contains(joined, "fritz_box_7590_ax_gb_recieved") {
		t.Errorf("expected the warning to name the typo'd entity_id, got: %v", warnings)
	}
	if strings.Contains(joined, `"node.compliant"`) {
		t.Errorf("node.compliant's \"is available\" source matches a sibling, should not be flagged: %v", warnings)
	}
	if strings.Contains(joined, `"node.plain"`) {
		t.Errorf("node.plain has no \"is available\" capability, should not be flagged: %v", warnings)
	}
	if len(warnings) != 1 {
		t.Errorf("expected exactly 1 warning, got %d: %v", len(warnings), warnings)
	}
}

// TestResolveHassBridgeAvailabilityLabelReferences is the regression test for the user's own
// explicit request, 2026-09-25 (utility.somfy_living_room_front's own "binary_sensor.node cover is
// available;", where "cover" is the sibling "cover.cover" capability's own LABEL, not a raw
// entity_id): a bare label reference must be rewritten in place to the sibling's own real source,
// with the " is available" suffix preserved, so every downstream consumer sees a fully-resolved
// entity_id exactly as if it had been hand-typed. An already-resolved raw entity_id (containing a
// ".") must be left untouched, matching every existing device's own convention unchanged.
func TestResolveHassBridgeAvailabilityLabelReferences(t *testing.T) {
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"utility.somfy_living_room_front": {DeviceID: "utility.somfy_living_room_front", Capabilities: map[string]THassBridgeCapability{
			"node":       {Domain: "binary_sensor", Sources: map[string]string{"ha2mqtt": "cover is available"}},
			"cover":      {Domain: "cover", Sources: map[string]string{"ha2mqtt": "cover.living_room_front"}},
			"cover_slow": {Domain: "cover", Sources: map[string]string{"ha2mqtt": "cover.living_room_front_low_speed"}},
		}},
		// Already-resolved raw entity_id -- must stay untouched (every pre-existing device's own
		// convention, e.g. node.fritz_box/node.vienna).
		"node.compliant": {DeviceID: "node.compliant", Capabilities: map[string]THassBridgeCapability{
			"node":     {Domain: "binary_sensor", Sources: map[string]string{"vienna": "sensor.processor_use is available"}},
			"cpu/load": {Domain: "sensor", Sources: map[string]string{"vienna": "sensor.processor_use"}},
		}},
		// Unknown label ("typo") -- left unresolved, still surfaces as validateHassBridgeAvailabilitySources'
		// own "likely a typo" warning afterward.
		"node.unknown_label": {DeviceID: "node.unknown_label", Capabilities: map[string]THassBridgeCapability{
			"node":   {Domain: "binary_sensor", Sources: map[string]string{"vienna": "typo is available"}},
			"status": {Domain: "sensor", Sources: map[string]string{"vienna": "sensor.something"}},
		}},
	}

	resolveHassBridgeAvailabilityLabelReferences(hassBridgeDevicesByID)

	if got := hassBridgeDevicesByID["utility.somfy_living_room_front"].Capabilities["node"].Sources["ha2mqtt"]; got != "cover.living_room_front is available" {
		t.Errorf("resolved source = %q, want the sibling \"cover\" capability's own real entity_id substituted in", got)
	}
	if got := hassBridgeDevicesByID["node.compliant"].Capabilities["node"].Sources["vienna"]; got != "sensor.processor_use is available" {
		t.Errorf("already-resolved source = %q, want it left completely untouched", got)
	}
	if got := hassBridgeDevicesByID["node.unknown_label"].Capabilities["node"].Sources["vienna"]; got != "typo is available" {
		t.Errorf("unknown-label source = %q, want it left unresolved (surfaces as the validator's own warning)", got)
	}
}
