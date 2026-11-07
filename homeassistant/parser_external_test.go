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
