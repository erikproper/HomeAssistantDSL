package main

import (
	"strings"
	"testing"
)

// TestParseExternalDefinition: the layer, lists with and without options, "as cards", and a
// pattern of any domain under a path segment.
func TestParseExternalDefinition(t *testing.T) {
	declarations, warnings := parseExternalDefinition(`external layer with:
  list "Nodes" all binary_sensor.*/node with:   # a comment
    clean_prefix infrastructural;
    clean_postfix no_motion.ignore;
  end;
  list "Tests" all *.* under test_*;
  list "Compute" all sensor.*/cpu/* as cards with:
    detail_level 3;
  end;
end;
`)
	if len(warnings) != 0 || len(declarations) != 3 {
		t.Fatalf("declarations %+v, warnings %v", declarations, warnings)
	}
	if ops := declarations[0].cleanOps; len(ops) != 2 || ops[1] != (TListCleanOp{"postfix", "no_motion.ignore"}) {
		t.Errorf("clean ops %+v", ops)
	}
	tests := declarations[1].patterns[0]
	if tests.domain != "*" || tests.under != "test_*" {
		t.Errorf("tests pattern %+v", tests)
	}
	for path, want := range map[string]bool{"house/test_plug_1": true, "house/test_plug_1/shelly_plug/node": true, "house/contest": false} {
		rec := TEntityRecord{Identity: TEntityIdentity{Domain: "switch", Sphere: "social", Path: path}}
		if matchesAnyListPattern(rec, declarations[1].patterns) != want {
			t.Errorf("%q: want %v", path, want)
		}
	}
	if d := declarations[2]; !d.asCards || d.detailLevel != 3 || !d.patterns[0].wildcardLeaf {
		t.Errorf("compute %+v", d)
	}
}

// TestParseExternalDefinitionErrors: a mistake is reported at the word, and the next list is
// still read.
func TestParseExternalDefinitionErrors(t *testing.T) {
	declarations, warnings := parseExternalDefinition(`list "Bad" all switch.* with:
  clean_prefx social;
end;
list "Good" all switch.*;
`)
	if len(warnings) != 1 || !strings.Contains(warnings[0], `expected clean_prefix, clean_postfix, detail_level or "end", got "clean_prefx"`) {
		t.Errorf("warnings %q", warnings)
	}
	if len(declarations) != 1 || declarations[0].title != "Good" {
		t.Errorf("declarations %+v", declarations)
	}
}

// TestParseExternalHomekitBridge: a HomeKit bridge with its name, port and exposures, next to a
// list (PROJECT.md 62).
func TestParseExternalHomekitBridge(t *testing.T) {
	declarations, bridges, warnings := parseExternalLayer(`external layer with:
  homekit bridge "HASS Bridge 6Z" on port 21065 with:
    expose fan.social/apartment/living_room/cycling;   # the fan
    expose switch.social/apartment/terrace;
  end;
  list "Nodes" all binary_sensor.*/node;
end;
`)
	if len(warnings) != 0 || len(declarations) != 1 || len(bridges) != 1 {
		t.Fatalf("declarations %+v, bridges %+v, warnings %v", declarations, bridges, warnings)
	}
	want := THomekitBridge{Name: "HASS Bridge 6Z", Port: 21065, Exposed: []THomekitExposure{
		{Name: "fan.social/apartment/living_room/cycling"},
		{Name: "switch.social/apartment/terrace"},
	}}
	if got := bridges[0]; got.Name != want.Name || got.Port != want.Port || len(got.Exposed) != 2 ||
		got.Exposed[0] != want.Exposed[0] || got.Exposed[1] != want.Exposed[1] {
		t.Errorf("bridge %+v, want %+v", got, want)
	}
}

// TestParseExternalHomekitBridgeErrors: a missing port, and a statement that isn't "expose", are
// reported, and the list after them is still read.
func TestParseExternalHomekitBridgeErrors(t *testing.T) {
	for _, src := range []string{
		`homekit bridge "HASS Bridge" with: expose fan.social/cycling; end; list "Nodes" all binary_sensor.*/node;`,
		`homekit bridge "HASS Bridge" on port 21064 with: show fan.social/cycling; end; list "Nodes" all binary_sensor.*/node;`,
	} {
		declarations, bridges, warnings := parseExternalLayer(src)
		if len(warnings) == 0 || len(bridges) != 0 || len(declarations) != 1 {
			t.Errorf("%s: declarations %+v, bridges %+v, warnings %v", src, declarations, bridges, warnings)
		}
	}
}
