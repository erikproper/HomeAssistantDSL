/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: DeviceFriendlyNameTest
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 07.10.2026
 *
 */

package main

import "testing"

// TestApplyColocatedPositioningDerivesLeafFromAnchor is the concrete real-world case this
// mechanism was built for: Vienna's switch.living_room_tv (a bare Z-Wave relay, no conceptual
// position of its own) "colocated ... as switch;" with appliance.living_room_tv (positioned "as
// tv"). The colocated device should end up positioned at the anchor's own leaf ("tv") plus the
// declared suffix ("switch") -- "tv/switch".
func TestApplyColocatedPositioningDerivesLeafFromAnchor(t *testing.T) {
	admin := newAdministrationState()
	localDevicesByID := map[string]TLocalDevice{
		// Real Vienna Physical.def declares "derived binary_sensor.node with condition switch.core
		// is available;" for this device -- the fixture needs a "node" capability present too, now
		// that applyColocatedPositioning gives the colocated target its own node entity directly
		// (PROJECT.md's own 2026-10-08 two-step plan) rather than leaning on
		// applyImpliedNodePositioning's later sweep to do it.
		"switch.living_room_tv": {DeviceID: "switch.living_room_tv", Capabilities: map[string]TLocalCapability{"node": {Domain: "binary_sensor"}}},
	}
	logicalByID := map[string]TLogicalDevice{
		"appliance.living_room_tv": {
			DeviceID:  "appliance.living_room_tv",
			Colocated: []TLogicalColocatedFact{{DeviceID: "switch.living_room_tv", LeafSuffix: "switch"}},
		},
	}

	// Simulate Conceptual.def having already positioned the anchor ("device
	// appliance.living_room_tv as tv with: ...; end;") before this post-pass runs -- real
	// Conceptual.def parsing registers the anchor itself via registerLocalDevicePositioning or
	// similar; here it's simplest to just position it the same way the local kind would be.
	localDevicesByID["appliance.living_room_tv"] = TLocalDevice{DeviceID: "appliance.living_room_tv", Capabilities: map[string]TLocalCapability{}}
	warnings := registerDevicePositioning(admin, TDevicePositioningDeclaration{DeviceID: "appliance.living_room_tv", Spec: "tv"}, nil, nil, nil, nil, nil, localDevicesByID, logicalByID, "Conceptual.def", 1)
	if len(warnings) != 0 {
		t.Fatalf("positioning the anchor itself: unexpected warnings: %v", warnings)
	}

	colocatedWarnings := applyColocatedPositioning(admin, logicalByID, nil, nil, nil, nil, nil, localDevicesByID, "Conceptual.def")
	if len(colocatedWarnings) != 0 {
		t.Fatalf("applyColocatedPositioning: unexpected warnings: %v", colocatedWarnings)
	}

	link, ok := admin.DeviceConceptualLinks["switch.living_room_tv"]
	if !ok {
		t.Fatalf("switch.living_room_tv was never positioned; DeviceConceptualLinks = %+v", admin.DeviceConceptualLinks)
	}
	if link.DisplayName != "tv/switch" {
		t.Errorf("DisplayName = %q, want %q", link.DisplayName, "tv/switch")
	}

	occ := admin.DevicePositioningOccurrences["switch.living_room_tv"]
	if len(occ) != 1 || occ[0].DeviceNamePath != "tv/switch" {
		t.Errorf("DevicePositioningOccurrences[switch.living_room_tv] = %+v, want one entry with DeviceNamePath %q", occ, "tv/switch")
	}
}

// TestApplyColocatedPositioningWarnsOnAmbiguousAnchor confirms an anchor positioned in more than
// one space (or never positioned at all) is reported, not guessed at -- same discipline
// applyImpliedNodePositioning already applies to its own single-space rule.
func TestApplyColocatedPositioningWarnsOnAmbiguousAnchor(t *testing.T) {
	admin := newAdministrationState()
	logicalByID := map[string]TLogicalDevice{
		"appliance.living_room_tv": {
			DeviceID:  "appliance.living_room_tv",
			Colocated: []TLogicalColocatedFact{{DeviceID: "switch.living_room_tv", LeafSuffix: "switch"}},
		},
	}

	// Anchor never positioned at all (0 occurrences).
	warnings := applyColocatedPositioning(admin, logicalByID, nil, nil, nil, nil, nil, nil, "Conceptual.def")
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1 for an unpositioned anchor: %v", len(warnings), warnings)
	}
	if _, positioned := admin.DeviceConceptualLinks["switch.living_room_tv"]; positioned {
		t.Errorf("switch.living_room_tv should not have been positioned when the anchor itself is ambiguous")
	}
}
