package main

import (
	"strings"
	"testing"
)

func TestExtractDeviceSourceEntityDeclaration(t *testing.T) {
	decl, ok := extractDeviceSourceEntityDeclaration("entity binary_sensor.infrastructural:laserjet/status as switch.yyy from hass.laserjet;")
	if !ok {
		t.Fatalf("expected the line to match")
	}
	if decl.LocalSpec != "binary_sensor.infrastructural:laserjet/status" || decl.Source != "switch.yyy" || decl.DeviceID != "hass.laserjet" {
		t.Fatalf("got %+v", decl)
	}
}

func TestExtractDeviceSourceEntityDeclarationDoesNotMatchDiscoveryShape(t *testing.T) {
	// "entity <spec> from <gateway>.<leaf>;" (Conceptual_DiscoveryEntities.go) has no "as" clause
	// -- must not be swallowed by this construct's pattern.
	if _, ok := extractDeviceSourceEntityDeclaration("entity sensor.physical:garage_door/temperature from discovery.ems_esp.boiler_outdoortemp;"); ok {
		t.Fatalf("expected no match for the discovery-gateway shape")
	}
}

func TestDeviceSourceEntityRegistersAndFeedsHassBridgeFile(t *testing.T) {
	const miniDSL = `device infrastructural:laserjet from hass.laserjet;
entity binary_sensor.infrastructural:laserjet/jammed as switch.laserjet_jam_sensor from hass.laserjet;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.laserjet": {DeviceID: "hass.laserjet", Instances: []string{"protocols-server-2"}, Capabilities: map[string]THassBridgeCapability{
			"status": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.hewlett_packard_hp_laserjet_professional_p1102w"}},
		}},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration

	link, ok := admin.DeviceConceptualLinks["hass.laserjet"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[hass.laserjet] to be populated")
	}

	wantEntityID := "binary_sensor.infrastructural_laserjet_jammed"
	attr, ok := link.AttributeEntityIDs[wantEntityID]
	if !ok {
		t.Fatalf("expected AttributeEntityIDs keyed by the local entity_id %q, got %v", wantEntityID, link.AttributeEntityIDs)
	}
	if attr.EntityID != wantEntityID {
		t.Errorf("EntityID = %q, want %q", attr.EntityID, wantEntityID)
	}

	// device.Capabilities was mutated in place -- generateHassBridgeFile reads it directly, no
	// changes needed there for this to feed through to coordinator/homeassistant_bridge.yaml.
	cap, ok := hassBridgeDevicesByID["hass.laserjet"].Capabilities[wantEntityID]
	if !ok || cap.Domain != "binary_sensor" || cap.Sources["protocols-server-2"] != "switch.laserjet_jam_sensor" {
		t.Errorf("Capabilities[%q] = (%+v, %v), want (Domain \"binary_sensor\", Sources[\"protocols-server-2\"] \"switch.laserjet_jam_sensor\", true) -- the coerced local domain, the source entity, and the original \"status\" capability preserved alongside it", wantEntityID, cap, ok)
	}
	if got := hassBridgeDevicesByID["hass.laserjet"].Capabilities["status"]; got.Sources["protocols-server-2"] != "sensor.hewlett_packard_hp_laserjet_professional_p1102w" {
		t.Errorf("original Physical.def-declared \"status\" capability was lost, got %+v", got)
	}
}

// TestDeviceCapabilityEntityResolvesTypingDefaults is the regression test for a real bug caught
// live 2026-08-29: registerDeviceSourceEntityLink (reached here via registerDeviceCapabilityEntityLink,
// the "entity ... from <device-id> entity <capability>;" construct, including its "for <device-id>:
// ...;"/merged-block shorthand) never resolved device_class/unit/state_class/icon at all -- every
// capability positioned this way silently lost its typing in HA, even though the sibling
// registerHassBridgeAttributeEntity path always has.
func TestDeviceCapabilityEntityResolvesTypingDefaults(t *testing.T) {
	const miniDSL = `device infrastructural:davids_bedroom from hass.davids_bedroom;
entity sensor.physical:netatmo/co2 from hass.davids_bedroom entity sensor.co2;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.davids_bedroom": {DeviceID: "hass.davids_bedroom", Instances: []string{"protocols-server-2"}, Capabilities: map[string]THassBridgeCapability{
			"co2": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.davids_bedroom_carbon_dioxide"}},
		}},
	}
	capabilityDefaults := []TCapabilityDefaultRule{
		{Domain: "sensor", PatternPath: "co2", DeviceClass: "carbon_dioxide", Unit: "ppm", StateClass: "measurement"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, hassBridgeDevicesByID, nil, nil, capabilityDefaults)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	link, ok := result.Administration.DeviceConceptualLinks["hass.davids_bedroom"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[hass.davids_bedroom] to be populated")
	}
	attr, ok := link.AttributeEntityIDs["co2"]
	if !ok {
		t.Fatalf("expected AttributeEntityIDs[co2] to be populated, got %v", link.AttributeEntityIDs)
	}
	if attr.DeviceClass != "carbon_dioxide" || attr.Unit != "ppm" || attr.StateClass != "measurement" {
		t.Errorf("AttributeEntityIDs[co2] = %+v, want DeviceClass \"carbon_dioxide\", Unit \"ppm\", StateClass \"measurement\" resolved from capabilityDefaults", attr)
	}

	// device.Capabilities (what generateHassBridgeFile/generateHassBridgeFile's writer actually
	// reads) must carry the same resolved typing, not just the in-memory link.
	cap := hassBridgeDevicesByID["hass.davids_bedroom"].Capabilities["co2"]
	if cap.DeviceClass != "carbon_dioxide" || cap.Unit != "ppm" || cap.StateClass != "measurement" {
		t.Errorf("Capabilities[co2] = %+v, want the same resolved typing", cap)
	}
}

// TestDeviceCapabilityEntityExplicitTypingWinsOverDefaults confirms an explicit device_class/unit/
// state_class/icon Physical.def already declared directly on the capability (before this
// construct runs) is preserved, never overwritten by a conflicting capabilityDefaults rule -- same
// precedence registerHassBridgeAttributeEntity's own path already guarantees.
func TestDeviceCapabilityEntityExplicitTypingWinsOverDefaults(t *testing.T) {
	const miniDSL = `device infrastructural:davids_bedroom from hass.davids_bedroom;
entity sensor.physical:netatmo/co2 from hass.davids_bedroom entity sensor.co2;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.davids_bedroom": {DeviceID: "hass.davids_bedroom", Instances: []string{"protocols-server-2"}, Capabilities: map[string]THassBridgeCapability{
			"co2": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.davids_bedroom_carbon_dioxide"}, DeviceClass: "explicit_class", Unit: "explicit_unit"},
		}},
	}
	capabilityDefaults := []TCapabilityDefaultRule{
		{Domain: "sensor", PatternPath: "co2", DeviceClass: "carbon_dioxide", Unit: "ppm", StateClass: "measurement"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, hassBridgeDevicesByID, nil, nil, capabilityDefaults)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	attr := result.Administration.DeviceConceptualLinks["hass.davids_bedroom"].AttributeEntityIDs["co2"]
	if attr.DeviceClass != "explicit_class" || attr.Unit != "explicit_unit" {
		t.Errorf("AttributeEntityIDs[co2] = %+v, want the explicit device_class/unit preserved, not overwritten by capabilityDefaults", attr)
	}
	// StateClass had no explicit value -- the default rule should still gap-fill it.
	if attr.StateClass != "measurement" {
		t.Errorf("AttributeEntityIDs[co2].StateClass = %q, want the default-resolved \"measurement\" (only unset fields should be gap-filled)", attr.StateClass)
	}
}

func TestDeviceSourceEntityWarnsWithoutPriorDevicePositioning(t *testing.T) {
	const miniDSL = `entity binary_sensor.infrastructural:laserjet/jammed as switch.laserjet_jam_sensor from hass.laserjet;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.laserjet": {DeviceID: "hass.laserjet", Instances: []string{"protocols-server-2"}, Capabilities: map[string]THassBridgeCapability{}},
	}

	var report strings.Builder
	_, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	// registerDeviceSourceEntityLink's warning goes to stderr via the parse loop, not returned
	// here -- exercised directly instead for a deterministic assertion. finalAttempt=true: this
	// device is never positioned anywhere in miniDSL, so it should warn even on a "final" check.
	admin := newAdministrationState()
	decl := TDeviceSourceEntityDeclaration{LocalSpec: "binary_sensor.infrastructural:laserjet/jammed", Source: "switch.laserjet_jam_sensor", DeviceID: "hass.laserjet"}
	warnings, deferred := registerDeviceSourceEntityLink(admin, decl, hassBridgeDevicesByID, nil, "Spaces.def", 42, true, "")
	if len(warnings) != 1 || !strings.Contains(warnings[0], "no \"device.<spec> from hass.laserjet with: ...;\" positioning yet") {
		t.Errorf("warnings = %v, want a single warning about missing device positioning", warnings)
	}
	if !deferred {
		t.Errorf("deferred = false, want true for a device that was never positioned")
	}
}

func TestDeviceSourceEntityDefersRatherThanWarnsOnFirstAttempt(t *testing.T) {
	// finalAttempt=false: the device isn't positioned yet, but might still be positioned later in
	// the same file -- must come back empty-handed and deferred, not a premature warning.
	admin := newAdministrationState()
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.laserjet": {DeviceID: "hass.laserjet", Instances: []string{"protocols-server-2"}, Capabilities: map[string]THassBridgeCapability{}},
	}
	decl := TDeviceSourceEntityDeclaration{LocalSpec: "binary_sensor.infrastructural:laserjet/jammed", Source: "switch.laserjet_jam_sensor", DeviceID: "hass.laserjet"}
	warnings, deferred := registerDeviceSourceEntityLink(admin, decl, hassBridgeDevicesByID, nil, "Spaces.def", 42, false, "")
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none on a non-final attempt", warnings)
	}
	if !deferred {
		t.Errorf("deferred = false, want true when the device isn't positioned yet")
	}
}

func TestDeviceSourceEntityWarnsOnUnknownDevice(t *testing.T) {
	admin := newAdministrationState()
	decl := TDeviceSourceEntityDeclaration{LocalSpec: "binary_sensor.infrastructural:laserjet/jammed", Source: "switch.laserjet_jam_sensor", DeviceID: "hass.unknown"}
	warnings, deferred := registerDeviceSourceEntityLink(admin, decl, map[string]THassBridgeDevice{}, nil, "Spaces.def", 42, false, "")
	if len(warnings) != 1 || !strings.Contains(warnings[0], `device "hass.unknown" not found`) {
		t.Errorf("warnings = %v, want a single \"device not found\" warning", warnings)
	}
	if deferred {
		t.Errorf("deferred = true, want false -- an unknown device can never resolve later, so this must be final immediately")
	}
}

// TestDeviceCapabilityEntityBeforePositioningStillResolves is the order-independence regression
// test for the single-pass redesign (2026-08-27): a U1 reference reached before its device's U2
// positioning line must still resolve, via the parser's pending-list retry -- not by re-scanning
// the file, and not by requiring positioning-before-usage source order.
func TestDeviceCapabilityEntityBeforePositioningStillResolves(t *testing.T) {
	const miniDSL = `space social:david_bedroom with:
  entity sensor.physical:netatmo/co2 from hass.davids_bedroom entity sensor.co2;
  device infrastructural:netatmo from hass.davids_bedroom;
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.davids_bedroom": {
			DeviceID:  "hass.davids_bedroom",
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"co2": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.davids_bedroom_carbon_dioxide"}},
			},
		},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration

	link, ok := admin.DeviceConceptualLinks["hass.davids_bedroom"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[hass.davids_bedroom] to be populated")
	}
	found := false
	for entityID := range link.AttributeEntityIDs {
		if strings.Contains(entityID, "co2") {
			found = true
		}
	}
	if !found {
		t.Errorf("AttributeEntityIDs = %v, want a co2 entry -- the U1 reference declared before its device's U2 positioning line should still resolve", link.AttributeEntityIDs)
	}
}
