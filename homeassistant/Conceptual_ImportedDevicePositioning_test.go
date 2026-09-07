package main

import (
	"strings"
	"testing"
)

// TestImportedDevicePositioningEndToEnd covers PROJECT.md 1.2d's Spaces.def integration: the SAME
// merged "device <spec> from <device-id> with: entity ... from entity <capability>; ...; end;"
// construct already used for native hassbridge devices must also work, unmodified, for a
// "hassbridge"-form import -- Spaces.def never needs to know or care which integration kind a
// device-id resolves against (the user's own explicit design confirmation, 2026-08-30).
func TestImportedDevicePositioningEndToEnd(t *testing.T) {
	const miniDSL = `space social:shower_room with:
  device infrastructural:netatmo from hass.vienna_shower_room with:
    entity sensor.physical:netatmo/temperature from entity temperature;
    entity sensor.physical:netatmo/co2         from entity co2;
  end;
end;`

	importedDevicesByID := map[string]TImportedDevice{
		"hass.vienna_shower_room": {
			DeviceID: "hass.vienna_shower_room", RemoteInstallation: "junglinster", RemoteDeviceID: "hass.vienna_shower_room",
			Capabilities: map[string]TImportedCapability{
				"node":        {},
				"temperature": {},
				"co2":         {},
			},
		},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, nil, importedDevicesByID, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration

	link, ok := admin.DeviceConceptualLinks["hass.vienna_shower_room"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[hass.vienna_shower_room] to be populated")
	}

	node, ok := link.AttributeEntityIDs["node"]
	if !ok || node.EntityID != "binary_sensor.infrastructural_shower_room_netatmo_node" {
		t.Errorf("node auto-registration = %+v, ok=%v, want EntityID binary_sensor.infrastructural_shower_room_netatmo_node", node, ok)
	}
	temperature, ok := link.AttributeEntityIDs["temperature"]
	if !ok || temperature.EntityID != "sensor.physical_shower_room_netatmo_temperature" {
		t.Errorf("temperature = %+v, ok=%v, want EntityID sensor.physical_shower_room_netatmo_temperature", temperature, ok)
	}
	co2, ok := link.AttributeEntityIDs["co2"]
	if !ok || co2.EntityID != "sensor.physical_shower_room_netatmo_co2" {
		t.Errorf("co2 = %+v, ok=%v, want EntityID sensor.physical_shower_room_netatmo_co2", co2, ok)
	}
}

// TestRegisterDeviceCapabilityEntityLinkWarnsOnUndeclaredImportedCapability confirms referencing
// a capability the Physical.def import never declared is a clear, reported warning, not a silent
// no-op -- via registerDeviceCapabilityEntityLink directly (the "for <device-id>: entity ... from
// entity <capability>;" shorthand's own dispatch function), since warnings from the full parse
// pipeline go to stderr rather than the report buffer.
func TestRegisterDeviceCapabilityEntityLinkWarnsOnUndeclaredImportedCapability(t *testing.T) {
	administration := newAdministrationState()
	importedDevicesByID := map[string]TImportedDevice{
		"hass.vienna_shower_room": {
			DeviceID: "hass.vienna_shower_room", RemoteInstallation: "junglinster", RemoteDeviceID: "hass.vienna_shower_room",
			Capabilities: map[string]TImportedCapability{
				"node": {},
			},
		},
	}
	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.physical:netatmo/pressure", DeviceID: "hass.vienna_shower_room", Capability: "pressure"}

	warnings, deferred := registerDeviceCapabilityEntityLink(administration, decl, nil, nil, importedDevicesByID, nil, nil, "test.def", 1, true)
	if deferred {
		t.Fatalf("expected deferred=false")
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "pressure") {
		t.Errorf("warnings = %v, want exactly one warning naming the undeclared \"pressure\" capability", warnings)
	}
}

// TestRegisterImportedDevicePositioningWarnsWhenNoNodeCapabilityDeclared mirrors the native
// hassbridge test of the same shape (Conceptual_DevicePositioning_test.go) for the import path.
func TestRegisterImportedDevicePositioningWarnsWhenNoNodeCapabilityDeclared(t *testing.T) {
	administration := newAdministrationState()
	importedDevicesByID := map[string]TImportedDevice{
		"hass.no_node": {
			DeviceID: "hass.no_node", RemoteInstallation: "junglinster", RemoteDeviceID: "hass.no_node",
			Capabilities: map[string]TImportedCapability{
				"temperature": {},
			},
		},
	}
	decl := TDevicePositioningDeclaration{Spec: "infrastructural:no_node", DeviceID: "hass.no_node"}

	warnings := registerDevicePositioning(administration, decl, nil, nil, importedDevicesByID, "test.def", 1)
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "hass.no_node") || !strings.Contains(warnings[0], "declares no \"node\" capability") {
		t.Errorf("warning = %q, want it to name the device and explain the missing \"node\" capability", warnings[0])
	}
	if _, ok := administration.DeviceConceptualLinks["hass.no_node"]; !ok {
		t.Errorf("expected DeviceConceptualLinks[hass.no_node] to still be populated despite the missing node capability")
	}
}

// TestRegisterDeviceSourceEntityLinkRejectsAsFormForImports confirms the bare "entity ... as
// <source> from <device-id>;" form is explicitly rejected for an import (there's no meaningful
// local "source" to give -- only an already-declared Physical.def capability makes sense).
func TestRegisterDeviceSourceEntityLinkRejectsAsFormForImports(t *testing.T) {
	administration := newAdministrationState()
	administration.DeviceConceptualLinks["hass.vienna_shower_room"] = TDeviceConceptualLink{AttributeEntityIDs: map[string]TDeviceAttributeLink{}}
	importedDevicesByID := map[string]TImportedDevice{
		"hass.vienna_shower_room": {DeviceID: "hass.vienna_shower_room", Capabilities: map[string]TImportedCapability{}},
	}
	decl := TDeviceSourceEntityDeclaration{LocalSpec: "sensor.physical:netatmo/temperature", Source: "sensor.something", DeviceID: "hass.vienna_shower_room"}

	warnings, deferred := registerDeviceSourceEntityLink(administration, decl, nil, importedDevicesByID, "test.def", 1, true, "")
	if deferred {
		t.Fatalf("expected deferred=false")
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "isn't supported for imports") {
		t.Errorf("warnings = %v, want exactly one warning explaining the \"as\" form isn't supported for imports", warnings)
	}
}
