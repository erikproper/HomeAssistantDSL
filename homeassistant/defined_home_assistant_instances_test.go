package main

import "testing"

func TestCollectHomeAssistantInstancesParsesMainAndNamed(t *testing.T) {
	content := "physical layer with:\n  home_assistant main: junglinster;\n  home_assistant protocols-server-2: protocols-server-2;\nend;\n"

	got := collectHomeAssistantInstances(content)
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 instances", got)
	}
	if got["main"] != (THomeAssistantInstance{Name: "junglinster"}) {
		t.Errorf("main = %+v, want {Name: junglinster}", got["main"])
	}
	if got["protocols-server-2"] != (THomeAssistantInstance{Name: "protocols-server-2"}) {
		t.Errorf("protocols-server-2 = %+v, want {Name: protocols-server-2}", got["protocols-server-2"])
	}
}

func TestParseHomeAssistantMainDirective(t *testing.T) {
	name := parseHomeAssistantMainDirective("home_assistant main: junglinster;")
	if name != "junglinster" {
		t.Errorf("got %q, want %q", name, "junglinster")
	}
}

func TestParseHomeAssistantMainDirectiveIgnoresNamedInstances(t *testing.T) {
	name := parseHomeAssistantMainDirective("home_assistant protocols-server-2: protocols-server-2;")
	if name != "" {
		t.Errorf("got %q, want \"\" -- only \"main\" should match", name)
	}
}

// TestParseHomeAssistantMainDirectiveIgnoresStrayExtraToken is the regression test for the real
// bug found live 2026-09-14: Vienna's own declaration carried a now-removed trailing "<url>"
// token (dead weight from a design superseded by MQTT-based instance routing) plus a trailing "#"
// comment, and the comment alone was enough to break the line's own terminating ";" match
// entirely -- silently emptying resolveMainIncarnationName's result, which Vienna's own deploy
// script depends on to pick the right output directory (hass/vienna/). A stray extra token (with
// no trailing comment) must still resolve the name correctly, just warn about the extra token,
// rather than fail silently the way the full line previously did.
func TestParseHomeAssistantMainDirectiveIgnoresStrayExtraToken(t *testing.T) {
	name := parseHomeAssistantMainDirective("home_assistant main: junglinster stray-leftover-token;")
	if name != "junglinster" {
		t.Errorf("got %q, want %q -- the name must still resolve despite the stray extra token", name, "junglinster")
	}
}
