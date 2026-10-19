/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Layers tests
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 13.10.2026
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
