/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualCommandlineEntitiesTest
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 07.09.2026
 *
 */

package main

import (
	"strings"
	"testing"
)

func frameCommandlineDevicesByID() map[string]TCommandlineDevice {
	return map[string]TCommandlineDevice{
		"host.frame": {
			DeviceID: "host.frame",
			Host:     "frame",
			Capabilities: map[string]TCommandlineCapability{
				"slideshow": {Kind: "switch", StatusScript: "/home/pi/bin/check_slideshow", OnScript: "/home/pi/bin/start_slideshow", OffScript: "/home/pi/bin/stop_slideshow"},
			},
		},
	}
}

// TestCommandlineCapabilityEntityLinkResolvesOnDualKindDevice is the regression test for a real
// case found live 2026-09-06: host.frame is declared BOTH as a "hosts" cpu device (ping/cpu) AND a
// "commandline" device (the picture-frame switch), sharing one DeviceID. Positioning it once via
// the hosts branch ("device infrastructural:frame from host.frame;") must still let a SEPARATE
// "entity switch.social:picture_frame from host.frame entity slideshow;" line resolve the
// commandline capability, reusing the SAME DeviceConceptualLink rather than needing its own
// positioning statement.
func TestCommandlineCapabilityEntityLinkResolvesOnDualKindDevice(t *testing.T) {
	const miniDSL = `device infrastructural:apartment/living_room/rack/frame from host.frame;
entity switch.social:apartment/living_room/picture_frame from host.frame entity slideshow;`

	hostDevicesByID := map[string]THostDevice{
		"host.frame": {DeviceID: "host.frame", HostName: "frame", IntegrationType: "cpu"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, hostDevicesByID, nil, nil, nil, frameCommandlineDevicesByID(), nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration

	link, ok := admin.DeviceConceptualLinks["host.frame"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[host.frame] to be populated")
	}
	attr, ok := link.AttributeEntityIDs["slideshow"]
	if !ok {
		t.Fatalf("expected AttributeEntityIDs[slideshow] to be set, got %+v", link.AttributeEntityIDs)
	}
	wantEntityID := "switch.social_apartment_living_room_picture_frame"
	if attr.EntityID != wantEntityID {
		t.Errorf("EntityID = %q, want %q", attr.EntityID, wantEntityID)
	}

	// host.frame's own hosts-kind node entity must be unaffected -- confirms the shared link wasn't
	// clobbered, only added to.
	if link.NodeEntityID == "" {
		t.Errorf("NodeEntityID is empty, want the hosts-kind node entity registerHostDevicePositioning already set")
	}
}

// TestCommandlineCapabilityEntityLinkRejectsDomainMismatch confirms a local spec whose domain
// doesn't match the capability's own Kind is rejected outright, not silently coerced -- unlike a
// hassbridge capability, a commandline capability's Kind IS the entity's real HA domain as the
// coordinator will actually publish it (discoverycommandline.go); a mismatch would produce a
// reference to an entity that will never actually exist under that domain.
func TestCommandlineCapabilityEntityLinkRejectsDomainMismatch(t *testing.T) {
	const miniDSL = `device infrastructural:apartment/living_room/rack/frame from host.frame;
entity binary_sensor.social:apartment/living_room/picture_frame from host.frame entity slideshow;`

	hostDevicesByID := map[string]THostDevice{
		"host.frame": {DeviceID: "host.frame", HostName: "frame", IntegrationType: "cpu"},
	}

	var report strings.Builder
	output := captureStderr(t, func() {
		_, _ = ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, hostDevicesByID, nil, nil, nil, frameCommandlineDevicesByID(), nil)
	})
	if !strings.Contains(output, "must match") {
		t.Fatalf("expected a domain-mismatch warning, got: %q", output)
	}
}
