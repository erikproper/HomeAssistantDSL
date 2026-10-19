/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationLocalTest
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 24.09.2026
 *
 */

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLocalIntegrationBodyBareAndAvailabilityCapabilities(t *testing.T) {
	devices, warnings := parseLocalIntegrationBody(strings.Split(`device appliance.front_door_ring with:
  capability binary_sensor.node camera.core is available;
  capability camera.core;
  capability binary_sensor.ding;
end;`, "\n"))
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 || devices[0].DeviceID != "appliance.front_door_ring" {
		t.Fatalf("devices = %+v, want exactly one appliance.front_door_ring", devices)
	}
	device := devices[0]

	core, ok := device.Capabilities["core"]
	if !ok || core.Domain != "camera" || core.AvailabilityOf != "" {
		t.Errorf("Capabilities[core] = %+v, want a bare camera capability", core)
	}
	ding, ok := device.Capabilities["ding"]
	if !ok || ding.Domain != "binary_sensor" || ding.AvailabilityOf != "" {
		t.Errorf("Capabilities[ding] = %+v, want a bare binary_sensor capability", ding)
	}
	node, ok := device.Capabilities["node"]
	if !ok || node.Domain != "binary_sensor" || node.AvailabilityOf != "core" || node.AvailabilityOfDomain != "camera" {
		t.Errorf("Capabilities[node] = %+v, want Domain=binary_sensor AvailabilityOf=core AvailabilityOfDomain=camera", node)
	}
}

// TestParseLocalIntegrationBodyDeviceOneLiner covers the new "device <id> with <single-statement>;"
// shorthand (2026-09-27, the generic with-clause dispatcher, with_clause.go) for a device
// declaring exactly one capability -- e.g. a bare "camera.core;" shape, collapsing what would
// otherwise need a full "with: ... end;" block for one line.
func TestParseLocalIntegrationBodyDeviceOneLiner(t *testing.T) {
	devices, warnings := parseLocalIntegrationBody(strings.Split(`device appliance.one_shot with capability camera.core;`, "\n"))
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 || devices[0].DeviceID != "appliance.one_shot" {
		t.Fatalf("devices = %+v, want exactly one appliance.one_shot", devices)
	}
	core, ok := devices[0].Capabilities["core"]
	if !ok || core.Domain != "camera" {
		t.Errorf("Capabilities[core] = %+v, want a bare camera capability", core)
	}
}

// TestParseLocalIntegrationBodyDeviceOneLinerRejectsBadContent covers the one-liner's own error
// path: the single statement matches localCapabilityPattern's shape but fails content validation
// (a non-domain-qualified "is available" sibling) -- still warns, and (matching the block form's
// own parity) the device is still positioned, just with no capability for the bad line.
func TestParseLocalIntegrationBodyDeviceOneLinerRejectsBadContent(t *testing.T) {
	devices, warnings := parseLocalIntegrationBody(strings.Split(`device appliance.one_shot with capability binary_sensor.node core is available;`, "\n"))
	if len(warnings) != 1 || !strings.Contains(warnings[0], "isn't domain-qualified") {
		t.Fatalf("warnings = %v, want exactly one \"isn't domain-qualified\" warning", warnings)
	}
	if len(devices) != 1 || devices[0].DeviceID != "appliance.one_shot" {
		t.Fatalf("devices = %+v, want the device still positioned despite the bad capability", devices)
	}
	if _, ok := devices[0].Capabilities["node"]; ok {
		t.Errorf("expected no Capabilities[node] entry for the rejected one-liner content")
	}
}

// TestParseLocalIntegrationBodyMediaSwitchOneLiner is the regression test for the media_player->
// switch coercion's relocation from Logical.def to the "local" kind (2026-09-27) -- the bare
// one-line form, no no_play_input refinement.
func TestParseLocalIntegrationBodyMediaSwitchOneLiner(t *testing.T) {
	devices, warnings := parseLocalIntegrationBody(strings.Split(`device appliance.tv_living_room with:
  capability media_player.core;
  capability switch.media media_player.social_house_living_room_tv;
end;`, "\n"))
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("devices = %+v, want exactly one", devices)
	}
	media, ok := devices[0].Capabilities["media"]
	if !ok || media.Domain != "switch" || media.MediaPlayerEntity != "media_player.social_house_living_room_tv" || media.NoPlayInput != "" {
		t.Errorf("Capabilities[media] = %+v, want Domain=switch MediaPlayerEntity=media_player.social_house_living_room_tv NoPlayInput=\"\"", media)
	}
}

// TestParseLocalIntegrationBodyMediaSwitchWithNoPlayInputOneLiner covers the new one-liner
// shorthand for the media-switch capability's own "no_play_input" refinement (2026-09-27, the
// generic with-clause dispatcher, with_clause.go) -- "switch.media X with no_play_input \"Y\";"
// instead of the full "with: no_play_input \"Y\"; end;" block.
func TestParseLocalIntegrationBodyMediaSwitchWithNoPlayInputOneLiner(t *testing.T) {
	devices, warnings := parseLocalIntegrationBody(strings.Split(`device appliance.sonos_playbar_livingroom with:
  capability switch.media media_player.social_house_living_room_sonos with no_play_input "TV";
end;`, "\n"))
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	media, ok := devices[0].Capabilities["media"]
	if !ok || media.Domain != "switch" || media.MediaPlayerEntity != "media_player.social_house_living_room_sonos" || media.NoPlayInput != "TV" {
		t.Errorf("Capabilities[media] = %+v, want Domain=switch MediaPlayerEntity=media_player.social_house_living_room_sonos NoPlayInput=TV", media)
	}
}

// TestParseLocalIntegrationBodyMediaSwitchWithNoPlayInput covers the "with: no_play_input
// \"...\"; end;" block form (needed for the Sonos playbar's own relocation).
func TestParseLocalIntegrationBodyMediaSwitchWithNoPlayInput(t *testing.T) {
	devices, warnings := parseLocalIntegrationBody(strings.Split(`device appliance.sonos_playbar_livingroom with:
  switch.media media_player.social_house_living_room_sonos with:
    no_play_input "TV";
  end;
end;`, "\n"))
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	media, ok := devices[0].Capabilities["media"]
	if !ok || media.MediaPlayerEntity != "media_player.social_house_living_room_sonos" || media.NoPlayInput != "TV" {
		t.Errorf("Capabilities[media] = %+v, want MediaPlayerEntity=media_player.social_house_living_room_sonos NoPlayInput=TV", media)
	}
}

// TestParseLocalIntegrationBodyMediaSwitchWithBlockRejectsNonSwitchDomain confirms the "with:"
// block form is refused for any domain other than "switch" -- it exists solely to host the
// media_player->switch coercion's own no_play_input refinement.
func TestParseLocalIntegrationBodyMediaSwitchWithBlockRejectsNonSwitchDomain(t *testing.T) {
	_, warnings := parseLocalIntegrationBody(strings.Split(`device appliance.tv_living_room with:
  camera.core media_player.foo with:
    no_play_input "TV";
  end;
end;`, "\n"))
	if len(warnings) != 1 || !strings.Contains(warnings[0], "domain") {
		t.Fatalf("expected exactly one domain-rejection warning, got: %v", warnings)
	}
}

// TestRegisterLocalMediaSwitchCapability covers the media_player->switch coercion's own
// registration path once relocated to the "local" kind (2026-09-27) -- mirrors
// TestRegisterLogicalMediaSwitchCapability (Conceptual_LogicalEntities_test.go) exactly, since
// this must produce a byte-identical TEntityRecord regardless of which kind declared it.
func TestRegisterLocalMediaSwitchCapability(t *testing.T) {
	admin := newAdministrationState()
	admin.DeviceConceptualLinks["appliance.tv_living_room"] = TDeviceConceptualLink{
		AttributeEntityIDs: map[string]TDeviceAttributeLink{
			"node": {EntityID: "binary_sensor.infrastructural_house_living_room_tv_node"},
		},
	}

	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "switch.social:media", DeviceID: "appliance.tv_living_room", Capability: "switch.media"}
	capability := TLocalCapability{Domain: "switch", MediaPlayerEntity: "media_player.social_house_living_room_tv"}

	warnings, deferred := registerLocalCapabilityEntityLink(admin, decl, capability, "media", "tv_living_room", "Physical.def", 1, true, true)
	if len(warnings) != 0 || deferred {
		t.Fatalf("unexpected warnings/deferred: %v / %v", warnings, deferred)
	}

	rec, found := findEntityRecordByName(admin, "switch.social/tv_living_room/media")
	if !found {
		t.Fatalf("no entity record registered for switch.social/tv_living_room/media")
	}
	if rec.MediaSwitchPlayerName != capability.MediaPlayerEntity {
		t.Errorf("MediaSwitchPlayerName = %q, want %q", rec.MediaSwitchPlayerName, capability.MediaPlayerEntity)
	}
	if rec.MediaSwitchAvailabilityEntityID != "binary_sensor.infrastructural_house_living_room_tv_node" {
		t.Errorf("MediaSwitchAvailabilityEntityID = %q, want the sibling node's own entity_id", rec.MediaSwitchAvailabilityEntityID)
	}
}

// TestRegisterLocalMediaSwitchCapabilityDefersUntilNodeResolved mirrors
// TestRegisterLogicalMediaSwitchCapabilityDefersUntilNodeResolved -- the same real bug category
// (a "node" sibling that hasn't resolved yet on the first pass) applies here too.
func TestRegisterLocalMediaSwitchCapabilityDefersUntilNodeResolved(t *testing.T) {
	admin := newAdministrationState()
	admin.DeviceConceptualLinks["appliance.tv_living_room"] = TDeviceConceptualLink{}

	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "switch.social:media", DeviceID: "appliance.tv_living_room", Capability: "switch.media"}
	capability := TLocalCapability{Domain: "switch", MediaPlayerEntity: "media_player.social_house_living_room_tv"}

	warnings, deferred := registerLocalCapabilityEntityLink(admin, decl, capability, "media", "tv_living_room", "Physical.def", 1, true, false)
	if len(warnings) != 0 || !deferred {
		t.Fatalf("warnings/deferred = %v/%v, want no warnings and deferred=true", warnings, deferred)
	}
	if _, found := findEntityRecordByName(admin, "switch.social/tv_living_room/media"); found {
		t.Fatalf("expected no entity record registered on the deferred first attempt")
	}

	link := admin.DeviceConceptualLinks["appliance.tv_living_room"]
	link.AttributeEntityIDs = map[string]TDeviceAttributeLink{"node": {EntityID: "binary_sensor.infrastructural_house_living_room_tv_node"}}
	admin.DeviceConceptualLinks["appliance.tv_living_room"] = link

	warnings, deferred = registerLocalCapabilityEntityLink(admin, decl, capability, "media", "tv_living_room", "Physical.def", 1, true, true)
	if len(warnings) != 0 || deferred {
		t.Fatalf("retry warnings/deferred = %v/%v, want success", warnings, deferred)
	}
	rec, found := findEntityRecordByName(admin, "switch.social/tv_living_room/media")
	if !found {
		t.Fatalf("no entity record registered after the retry")
	}
	if rec.MediaSwitchAvailabilityEntityID != "binary_sensor.infrastructural_house_living_room_tv_node" {
		t.Errorf("MediaSwitchAvailabilityEntityID = %q, want the now-resolved sibling node's own entity_id", rec.MediaSwitchAvailabilityEntityID)
	}
}

// TestRegisterLocalMediaSwitchCapabilityRejectsUnsupportedCoercion mirrors
// TestRegisterLogicalMediaSwitchCapabilityRejectsUnsupportedCoercion -- only switch<-media_player
// is a supported coercion.
func TestRegisterLocalMediaSwitchCapabilityRejectsUnsupportedCoercion(t *testing.T) {
	admin := newAdministrationState()
	admin.DeviceConceptualLinks["appliance.x"] = TDeviceConceptualLink{}

	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "fan.social:media", DeviceID: "appliance.x", Capability: "fan.media"}
	capability := TLocalCapability{Domain: "fan", MediaPlayerEntity: "media_player.social_x"}

	warnings, deferred := registerLocalCapabilityEntityLink(admin, decl, capability, "media", "x", "Physical.def", 1, false, true)
	if deferred {
		t.Errorf("did not expect deferral for an unsupported coercion")
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "isn't a supported coercion") {
		t.Errorf("warnings = %v, want exactly one \"isn't a supported coercion\" warning", warnings)
	}
	if _, found := findEntityRecordByName(admin, "fan.social/x/media"); found {
		t.Errorf("expected no entity record registered for the unsupported coercion")
	}
}

// TestParseLocalIntegrationBodyRejectsNonDomainQualifiedSibling mirrors
// integration_discovery_parser.go's own identical guard: the "is available" sibling reference must
// be domain-qualified, not a bare label.
func TestParseLocalIntegrationBodyRejectsNonDomainQualifiedSibling(t *testing.T) {
	_, warnings := parseLocalIntegrationBody(strings.Split(`device appliance.front_door_ring with:
  capability binary_sensor.node core is available;
end;`, "\n"))
	if len(warnings) != 1 || !strings.Contains(warnings[0], "isn't domain-qualified") {
		t.Fatalf("expected exactly one \"isn't domain-qualified\" warning, got: %v", warnings)
	}
}

func TestCollectLocalDevicesByIDWarnsOnDuplicateDevice(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
  integration local with:
    device appliance.front_door_ring with:
      capability camera.core;
    end;
    device appliance.front_door_ring with:
      capability camera.core;
    end;
  end;
end;
`)

	byID, warnings := collectLocalDevicesByID(definitionDir)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "declared more than once") {
		t.Fatalf("expected exactly one \"declared more than once\" warning, got: %v", warnings)
	}
	if _, ok := byID["appliance.front_door_ring"]; !ok {
		t.Fatalf("expected the first declaration to still be kept")
	}
}

// TestCollectLocalDevicesByIDResolvesJinjaDerivedCapability is an end-to-end test (2026-09-30,
// closing the gap PROJECT.md 1d's battery_level-IFF-battery_alert rule found live): a "local"
// device declared with a "derived ... via jinja ${name}(args);" line resolves all the way through
// to a fully-formed DerivedViaTemplate, exactly like collectDiscoveryGatewaysByID's own identical
// test (Physical_DerivedCapability_test.go) -- confirming the jinja-call resolver is wired into
// the "local" integration's own collector too, not just discovery/hassbridge.
func TestCollectLocalDevicesByIDResolvesJinjaDerivedCapability(t *testing.T) {
	definitionDir := t.TempDir()
	physicalDef := `physical layer with:
  integration local with:
    device appliance.front_door_ring with:
      capability camera.core;
      capability sensor.battery_level;
      derived capability binary_sensor.battery_alert from sensor.battery_level via jinja ${int_less_then}(20);
    end;
  end;
end;
`
	settingsDef := `jinja ${int_less_then}(i) = "'on' if (($ | int(0)) < ${i}) else 'off'";`
	if err := os.WriteFile(filepath.Join(definitionDir, "Physical.def"), []byte(physicalDef), 0o644); err != nil {
		t.Fatalf("WriteFile Physical.def: %v", err)
	}
	if err := os.WriteFile(filepath.Join(definitionDir, "Settings.def"), []byte(settingsDef), 0o644); err != nil {
		t.Fatalf("WriteFile Settings.def: %v", err)
	}

	byID, warnings := collectLocalDevicesByID(definitionDir)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	device, ok := byID["appliance.front_door_ring"]
	if !ok {
		t.Fatalf("expected device appliance.front_door_ring, got %+v", byID)
	}
	cap, ok := device.Capabilities["battery_alert"]
	if !ok {
		t.Fatalf("expected capability battery_alert, got %+v", device.Capabilities)
	}
	if cap.Domain != "binary_sensor" {
		t.Errorf("Domain = %q, want binary_sensor", cap.Domain)
	}
	if cap.DerivedFromCapability != "battery_level" {
		t.Errorf("DerivedFromCapability = %q, want battery_level", cap.DerivedFromCapability)
	}
	wantTemplate := "'on' if (($ | int(0)) < 20) else 'off'"
	if cap.DerivedViaTemplate != wantTemplate {
		t.Errorf("DerivedViaTemplate = %q, want %q", cap.DerivedViaTemplate, wantTemplate)
	}
}
