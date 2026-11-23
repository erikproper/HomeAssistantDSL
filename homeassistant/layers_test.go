/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Layers tests
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 23.11.2026
 *
 */

package main

import "testing"

// TestParseLayerBlocksRuleWithIfElseAction is a regression test for a real bug found live
// 2026-10-13 migrating Junglinster's sarah_bedroom cover rules: genericBodyElement
// (defined.go) treated a rule body's "else:" marker as an independent nested block needing
// its OWN closing "end;", when it actually shares the single "end;" that already closes its
// enclosing "then if <entity-ref> is <state>:" action. That starved the outer "rule ...:"/
// "conceptual layer with:" blocks of their own closing "end;", failing the whole layer
// extraction with a misleading "missing its closing end;" error anchored at the outermost
// block, even though the rule body itself was perfectly well-formed (confirmed separately by
// ParseEntitiesAndFillAdministration, which parses rule bodies line-by-line and has no such
// issue).
func TestParseLayerBlocksRuleWithIfElseAction(t *testing.T) {
	const content = `conceptual layer with:
  space social:house/sarah_bedroom as area with:
    rule social:house/sarah_bedroom/cover/close with:
      when event.physical:house/sarah_bedroom/door_tap is press_2
      then if cover.social:house/sarah_bedroom is closing:
        stop cover.social:house/sarah_bedroom;
      else:
        set cover.social:house/sarah_bedroom to jinja "100 - (states('number.social_house_sarah_bedroom_my_position') | int)";
      end;
    end;
  end;
end;
`

	blocks, warnings := parseLayerBlocks(content, "test.def")
	for _, w := range warnings {
		t.Errorf("unexpected warning: %s", w)
	}
	if len(blocks) != 1 {
		t.Fatalf("got %d layer blocks, want 1 (layer extraction must not be starved of its own closing \"end;\" by the rule's \"else:\" marker)", len(blocks))
	}
	if blocks[0].Layer != LayerConceptual {
		t.Errorf("Layer = %q, want %q", blocks[0].Layer, LayerConceptual)
	}
}

// TestParseLayerBlocksUnclosedBlock checks that a missing "end;" is reported at the block that
// misses it, found by the indentation, not at the layer's own header on line 1.
func TestParseLayerBlocksUnclosedBlock(t *testing.T) {
	const content = `conceptual layer with:
  space social:apartment as area with:
    space social:kitchen with:
      device lights.kitchen with:
        entity light.social:main from light;
    end;
    space social:bedroom with:
      entity light.social:main;
    end;
  end;
end;
`
	_, warnings := parseLayerBlocks(content, "Conceptual.def")
	want := `Conceptual.def:4: "device lights.kitchen with:" has no "end;" -- probably just before line 6 ("end;"), which is no longer indented inside it`
	if len(warnings) != 1 || warnings[0] != want {
		t.Fatalf("warnings = %q, want [%q]", warnings, want)
	}
}

// TestParseLayerBlocksUnclosedBlockAtEnd checks the report when no line is indented back: the
// innermost block still open at the end.
func TestParseLayerBlocksUnclosedBlockAtEnd(t *testing.T) {
	const content = `conceptual layer with:
  space social:apartment as area with:
    entity light.social:main;
  end;
`
	_, warnings := parseLayerBlocks(content, "Conceptual.def")
	want := `Conceptual.def:1: "conceptual layer with:" has no "end;" before the end of the file`
	if len(warnings) != 1 || warnings[0] != want {
		t.Fatalf("warnings = %q, want [%q]", warnings, want)
	}
}

// TestParseLayerBlocksStatementSwallowsEnd checks the report for a statement without its ";",
// which swallows the "end;" after it: at that statement, on its own line (Vienna, 2026-11-23).
func TestParseLayerBlocksStatementSwallowsEnd(t *testing.T) {
	const content = `conceptual layer with:
  space social:living_room with:
    device buttons.living_room_kitchen_door_moes_2
      as leaf kitchen/door/moes/2
    with:
      entity binary_sensor.infrastructural:node;
    end;
    space off:
      switch.social:picture_frame;
      switch.social:tv/enabler
    end;
    light on social:main;
  end;
end;
`
	_, warnings := parseLayerBlocks(normalizeDefinitionWhitespace(content), "Conceptual.def")
	want := `Conceptual.def:10: "switch.social:tv/enabler" has no ";", so the "end;" after it was read as part of it`
	if len(warnings) != 1 || warnings[0] != want {
		t.Fatalf("warnings = %q, want [%q]", warnings, want)
	}
}
