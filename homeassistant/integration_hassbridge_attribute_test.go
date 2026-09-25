package main

import "testing"

// TestParseHassBridgeAttributeBothForms confirms Physical.def's new "attribute <name>: <source>;"
// grammar parses correctly in both shapes it's allowed: nested inside a capability's own "with:
// ... end;" block, and as a standalone "<path> attribute <name>: ...;" trailing line -- real
// motivating case (2026-09-21): vacuum.roomba's "error"/"error_code" attributes going unreported
// during a live fault ("stuck near a cliff"), since only its bare state was ever bridged. Source is
// unquoted -- an entity reference, not an arbitrary value (fixed same day, matching Sources' own
// unquoted convention).
func TestParseHassBridgeAttributeBothForms(t *testing.T) {
	body := []string{
		"device appliance.vacuum with:",
		`  vacuum.roomba vacuum.roomba with:`,
		`    map: "home" "docked";`,
		`    attribute error: vacuum.roomba!error;`,
		"  end;",
		`  roomba attribute error_code: vacuum.roomba!error_code;`,
		"end;",
	}

	devices, warnings := parseHassBridgeIntegrationBody(body, "protocols-server-2")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}

	cap, ok := devices[0].Capabilities["roomba"]
	if !ok {
		t.Fatalf("capability %q not found in %v", "roomba", devices[0].Capabilities)
	}
	if got := cap.Attributes["error"]["protocols-server-2"]; got != "vacuum.roomba!error" {
		t.Errorf(`Attributes["error"]["protocols-server-2"] = %q, want "vacuum.roomba!error"`, got)
	}
	if got := cap.Attributes["error_code"]["protocols-server-2"]; got != "vacuum.roomba!error_code" {
		t.Errorf(`Attributes["error_code"]["protocols-server-2"] = %q, want "vacuum.roomba!error_code"`, got)
	}
	// The bare state map: from the same block must still land in ValueMap, untouched by the new
	// attribute declarations sharing the same "with:" block.
	if cap.ValueMap["home"] != "docked" {
		t.Errorf(`ValueMap["home"] = %q, want "docked"`, cap.ValueMap["home"])
	}
}

// TestParseHassBridgeAttributeValueMapDisambiguatedFromStateMap confirms the qualified "map <name>:
// "<from>" "<to>";" form lands in AttributeValueMaps, keyed by attribute name, while the bare
// "map: "<from>" "<to>";" form (no name) keeps landing in ValueMap (state-only) -- the two must
// never collide regardless of declaration order within the same block.
func TestParseHassBridgeAttributeValueMapDisambiguatedFromStateMap(t *testing.T) {
	body := []string{
		"device appliance.vacuum with:",
		`  vacuum.roomba vacuum.roomba with:`,
		`    map error: "Stuck near a cliff" "stuck_cliff";`,
		`    map: "home" "docked";`,
		`    attribute error: vacuum.roomba!error;`,
		"  end;",
		"end;",
	}

	devices, warnings := parseHassBridgeIntegrationBody(body, "protocols-server-2")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	cap := devices[0].Capabilities["roomba"]

	if got := cap.AttributeValueMaps["error"]["Stuck near a cliff"]; got != "stuck_cliff" {
		t.Errorf(`AttributeValueMaps["error"]["Stuck near a cliff"] = %q, want "stuck_cliff"`, got)
	}
	if _, leaked := cap.ValueMap["Stuck near a cliff"]; leaked {
		t.Errorf("ValueMap = %v, the attribute-scoped map entry must not also land in the state ValueMap", cap.ValueMap)
	}
	if got := cap.ValueMap["home"]; got != "docked" {
		t.Errorf(`ValueMap["home"] = %q, want "docked" (bare map: must still work unmodified)`, got)
	}
	if _, leaked := cap.AttributeValueMaps[""]; leaked {
		t.Errorf("AttributeValueMaps = %v, the bare state map: entry must not land under an empty attribute name", cap.AttributeValueMaps)
	}
}

// TestParseHassBridgeAttributeStandaloneUnknownCapabilityWarns mirrors
// capabilityValueMapPattern's own "unknown capability" warning behaviour for the new standalone
// "attribute"/"map <name>:" trailing-line forms.
func TestParseHassBridgeAttributeStandaloneUnknownCapabilityWarns(t *testing.T) {
	body := []string{
		"device appliance.vacuum with:",
		`  ghost attribute error: vacuum.roomba!error;`,
		"end;",
	}

	devices, warnings := parseHassBridgeIntegrationBody(body, "protocols-server-2")
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one", warnings)
	}
	if len(devices) != 1 || len(devices[0].Capabilities) != 0 {
		t.Errorf("devices = %+v, want no capability created for the unknown path", devices)
	}
}
