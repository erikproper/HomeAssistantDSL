package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestImportedDevicePositioningEndToEnd covers PROJECT.md 1.2d's Spaces.def integration: the SAME
// merged "device <spec> from <device-id> with: entity ... from <capability>; ...; end;"
// construct already used for native hassbridge devices must also work, unmodified, for a
// "hassbridge"-form import -- Spaces.def never needs to know or care which integration kind a
// device-id resolves against (the user's own explicit design confirmation, 2026-08-30). Also
// exercises PROJECT.md item 5's "device declaration acts like a space" sugar: the capability specs
// below deliberately omit the device's own leaf path ("netatmo/") -- this is the exact real-world
// example item 5 itself was written against (Vienna's shower_room Netatmo device) -- and still
// resolve to the same entity ids the old, fully-spelled-out "sensor.physical:netatmo/temperature"
// form produced.
func TestImportedDevicePositioningEndToEnd(t *testing.T) {
	const miniDSL = `space social:shower_room with:
  device infrastructural:netatmo from hass.vienna_shower_room with:
    entity sensor.physical:temperature from temperature;
    entity sensor.physical:co2         from co2;
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

// TestImportedDevicePositioningInheritsEnclosingAreaFromNestedSpace is a regression test for a
// real gap found live 2026-09-08, mirroring the native hassbridge path's identical bug
// (Conceptual_DevicePositioning_test.go's TestRegisterDevicePositioningInheritsEnclosingAreaFromNestedSpace):
// an imported device positioned inside a space nested under an "as area" ancestor -- Junglinster's
// own "space social:front with: ...;" nested under "space social:terrace as area with: ...;" --
// never picked up the enclosing area as its own suggested_area.
func TestImportedDevicePositioningInheritsEnclosingAreaFromNestedSpace(t *testing.T) {
	const miniDSL = `space social:terrace as area with:
  space social:front with:
    device infrastructural:netatmo from hass.vienna_shower_room with:
      entity sensor.physical:temperature from temperature;
    end;
  end;
end;`

	importedDevicesByID := map[string]TImportedDevice{
		"hass.vienna_shower_room": {
			DeviceID: "hass.vienna_shower_room", RemoteInstallation: "junglinster", RemoteDeviceID: "hass.vienna_shower_room",
			Capabilities: map[string]TImportedCapability{
				"node":        {},
				"temperature": {},
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
	attr, ok := link.ConstantAttributes["suggested_area"]
	if !ok || attr.Value == "" {
		t.Errorf("suggested_area = %+v (ok=%v), want it inherited from the enclosing \"social:terrace as area\" space", attr, ok)
	}

	// Administration state alone isn't enough -- confirm it actually reaches the generated file
	// the coordinator reads. Real bug found live 2026-09-08: this exact value was correctly
	// computed above, but generateImportedDeviceFile never serialized ConstantAttributes at all,
	// so it silently never left the generator.
	devices := []TImportedDevice{{DeviceID: "hass.vienna_shower_room", RemoteInstallation: "junglinster", RemoteDeviceID: "hass.vienna_shower_room", Capabilities: importedDevicesByID["hass.vienna_shower_room"].Capabilities}}
	outputRoot := t.TempDir()
	if err := generateImportedDeviceFile(outputRoot, devices, admin); err != nil {
		t.Fatalf("generateImportedDeviceFile error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outputRoot, "coordinator", "imported.yaml"))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	if !strings.Contains(string(data), "suggested_area") {
		t.Errorf("generated imported.yaml = %s, want it to contain \"suggested_area\"", data)
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

	warnings, deferred := registerDeviceCapabilityEntityLink(administration, decl, nil, nil, importedDevicesByID, nil, nil, "test.def", 1, true, "")
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

	warnings := registerDevicePositioning(administration, decl, nil, nil, importedDevicesByID, nil, "test.def", 1)
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

	warnings, deferred := registerDeviceSourceEntityLink(administration, decl, nil, importedDevicesByID, "test.def", 1, true, "", "")
	if deferred {
		t.Fatalf("expected deferred=false")
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "isn't supported for imports") {
		t.Errorf("warnings = %v, want exactly one warning explaining the \"as\" form isn't supported for imports", warnings)
	}
}
