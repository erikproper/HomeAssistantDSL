package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNormalizeDefinitionWhitespace is the regression test for a real, user-reported gap (raised
// more than once): a literal TAB character, or a non-"\n" line ending, wasn't being treated as
// "just another space" -- the many hand-rolled literal `strings.Index(line, " with X ")`-style
// single-space substring searches scattered across this codebase's parsers only ever matched a
// genuine ASCII space, so a tab silently broke the match (no warning, just a quietly wrong parse)
// instead of "just working" the way any other run of whitespace already did.
func TestNormalizeDefinitionWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"tab between words becomes a space", "device X\twith:", "device X with:"},
		{"multiple tabs each become their own space", "a\t\tb", "a  b"},
		{"CRLF line ending becomes a bare LF (each line already terminated, so no join triggers)", "line one;\r\nline two;", "line one;\nline two;"},
		{"lone CR line ending (old Mac style) becomes a bare LF (each line already terminated)", "line one;\rline two;", "line one;\nline two;"},
		{"repeated real spaces are left alone", "a    b", "a    b"},
		{"leading tab indentation becomes a leading space", "\t\tentity foo;", "  entity foo;"},
		{"mixed tabs and CRLF in the same content", "device X\twith:\r\n\tentity foo;\r\n", "device X with:\n entity foo;\n"},
		{"an unterminated line (no trailing ; or :) joins into the next, CRLF included", "line one\r\nline two;", "line one line two;"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeDefinitionWhitespace(tc.input); got != tc.want {
				t.Errorf("normalizeDefinitionWhitespace(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestJoinWrappedStatementContinuations is the regression test for the real bug found live
// 2026-09-29 (Junglinster's Conceptual.def:517): an editor-wrapped one-liner "device X with
// <statement>;" declaration, broken right after the bare word "with," silently failed to parse as
// either the one-liner or block form. A line ending in the standalone word "with" (not "with:")
// must be joined, single-space, to whatever follows, until a real terminator is reached.
func TestJoinWrappedStatementContinuations(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"wrapped one-liner joins onto the next line",
			"device switches.foo with\n   entity light.social:main from light.light_2;\nend;",
			"device switches.foo with entity light.social:main from light.light_2;\nend;",
		},
		{
			"a real block header ending in \"with:\" is left alone",
			"device X with:\n  entity foo;\nend;",
			"device X with:\n  entity foo;\nend;",
		},
		{
			"a word merely ending in the letters \"with\" is not treated as the keyword",
			"# note: filled_with\nentity foo;",
			"# note: filled_with\nentity foo;",
		},
		{
			"a blank line breaks the join rather than being swallowed",
			"device switches.foo with\n\nentity light.social:main;",
			"device switches.foo with\n\nentity light.social:main;",
		},
		{
			"joins across more than one continuation line until a terminator is found",
			"device switches.foo with\n   entity\n   light.social:main from light.light_2;",
			"device switches.foo with entity light.social:main from light.light_2;",
		},
		{
			"a comment line ending in the word \"with\" never triggers a join",
			"# :aqara_magnet with alert_level 10;\" + \"call battery_level_device :philips_dimmer with\n# alert_level 20;\ndevice sensors.foo with:\n  entity bar;\nend;",
			"# :aqara_magnet with alert_level 10;\" + \"call battery_level_device :philips_dimmer with\n# alert_level 20;\ndevice sensors.foo with:\n  entity bar;\nend;",
		},
		{
			"a comment line is never swallowed as a continuation either",
			"device switches.foo with\n# a stray remark\nentity bar;",
			"device switches.foo with\n# a stray remark\nentity bar;",
		},
		{
			"general fix (2026-09-29): a wrapped \"is switched by\" one-liner, the real live bug that " +
				"proved the narrower \"with\"-only fix was the wrong shape -- wraps right after \"by\", " +
				"not \"with\" at all",
			"device sensors.house_kitchen_workplace_dimmer with light.core is switched by\n  light.light from switches.house_kitchen_hallway_door_switch_box_zwave_058;",
			"device sensors.house_kitchen_workplace_dimmer with light.core is switched by light.light from switches.house_kitchen_hallway_door_switch_box_zwave_058;",
		},
		{
			"general fix: a multi-line boolean condition (already a supported, deliberate DSL author " +
				"layout) is unaffected -- joins to the same single line derivedExprBuffer's own " +
				"accumulation would already produce, so no double-join/behaviour change",
			"condition\n  physical appliance.sonos_playbar_livingroom and\n  node.sonos_playbar_livingroom and\n  appliance.sonos_subwoofer_livingroom;",
			"condition physical appliance.sonos_playbar_livingroom and node.sonos_playbar_livingroom and appliance.sonos_subwoofer_livingroom;",
		},
		{
			"general fix: a genuine mid-line semicolon inside a quoted jinja string never fools the " +
				"trailing-terminator check into treating the line as complete early",
			"condition jinja \"$1 == 'a;b'\"\n  { sensor.x };",
			"condition jinja \"$1 == 'a;b'\" { sensor.x };",
		},
		{
			"real bug found live 2026-09-29, minutes after this function was first generalized: a " +
				"trailing inline comment with an ODD count of '\"' (routine in this codebase's own " +
				"prose, e.g. quoting a phrase without a matching closing quote in the same comment) " +
				"must never leave quote-tracking stuck \"open\" and swallow everything that follows",
			"entity foo; # the old \"flipped wrapper\ndevice bar with entity baz;",
			"entity foo; # the old \"flipped wrapper\ndevice bar with entity baz;",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeDefinitionWhitespace(tc.input); got != tc.want {
				t.Errorf("normalizeDefinitionWhitespace(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestReadOptionalFileNormalisesTabsAndLineEndings is the end-to-end regression test: a real file
// on disk containing a tab where a space was expected, and a CRLF line ending, must come back from
// readOptionalFile already normalised -- every downstream parser sees only plain spaces and "\n",
// with no per-call-site changes needed anywhere else.
func TestReadOptionalFileNormalisesTabsAndLineEndings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Test.def")
	raw := "device appliance.thing with\tdependency on utility.other;\r\nend;\r\n"
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := readOptionalFile(path)
	if err != nil {
		t.Fatalf("readOptionalFile error: %v", err)
	}
	want := "device appliance.thing with dependency on utility.other;\nend;\n"
	if got != want {
		t.Errorf("readOptionalFile content = %q, want %q", got, want)
	}
}

// TestReadOptionalFileMissingFileStillReturnsEmpty confirms normalizeDefinitionWhitespace's own
// addition didn't disturb readOptionalFile's existing "missing file -> empty string, no error"
// contract (collectLayerContent and others rely on this for optional .def files).
func TestReadOptionalFileMissingFileStillReturnsEmpty(t *testing.T) {
	got, err := readOptionalFile(filepath.Join(t.TempDir(), "DoesNotExist.def"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("content = %q, want empty string for a missing file", got)
	}
}
