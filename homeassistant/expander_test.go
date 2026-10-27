package main

import "testing"

// TestNormalizeSpaceListDirective covers the generic list-directive shape (2026-10-21): the
// one-item form without a colon and the block form both normalize to the original
// "<keyword>: <items>;" line; that original form, and any other line, is left alone.
func TestNormalizeSpaceListDirective(t *testing.T) {
	noLine := func(int) int { return 0 }
	cases := []struct {
		name   string
		lines  []string
		want   string
		wantI  int
		wantOK bool
	}{
		{"one item, no colon", []string{"light on social:main;"}, "light on: social:main;", 0, true},
		{"original form untouched", []string{"space off: @light @media;"}, "", 0, false},
		{"block", []string{
			"heating leak:",
			"  binary_sensor.social:living/door;",
			"  # a comment",
			"",
			"  binary_sensor.social:kitchen/terrace/door binary_sensor.social/apartment/bedroom/door;",
			"end;",
			"light on: social:main;",
		}, "heating leak: binary_sensor.social:living/door binary_sensor.social:kitchen/terrace/door binary_sensor.social/apartment/bedroom/door;", 5, true},
		{"unrelated line", []string{"lights motion guarded;"}, "", 0, false},
		{"keyword prefix of another word", []string{"space offset: x;"}, "", 0, false},
	}
	for _, c := range cases {
		got, gotI, ok := normalizeSpaceListDirective(c.lines, 0, c.lines[0], "Conceptual.def", noLine)
		if ok != c.wantOK || got != c.want || (ok && gotI != c.wantI) {
			t.Errorf("%s: got (%q, %d, %v), want (%q, %d, %v)", c.name, got, gotI, ok, c.want, c.wantI, c.wantOK)
		}
	}
}
