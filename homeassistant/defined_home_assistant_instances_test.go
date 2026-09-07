package main

import "testing"

func TestCollectHomeAssistantInstancesParsesMainAndNamed(t *testing.T) {
	content := "physical layer with:\n  home_assistant main: junglinster;\n  home_assistant protocols-server-2: protocols-server-2 ${protocols_server_2_url};\nend;\n"

	got := collectHomeAssistantInstances(content)
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 instances", got)
	}
	if got["main"] != (THomeAssistantInstance{Name: "junglinster"}) {
		t.Errorf("main = %+v, want {Name: junglinster, URL: \"\"}", got["main"])
	}
	if got["protocols-server-2"] != (THomeAssistantInstance{Name: "protocols-server-2", URL: "${protocols_server_2_url}"}) {
		t.Errorf("protocols-server-2 = %+v, want Name+URL both set", got["protocols-server-2"])
	}
}

func TestParseHomeAssistantMainDirectiveURLOptional(t *testing.T) {
	name, url := parseHomeAssistantMainDirective("home_assistant main: junglinster;")
	if name != "junglinster" || url != "" {
		t.Errorf("got (%q, %q), want (\"junglinster\", \"\") -- url should be optional", name, url)
	}
}

func TestParseHomeAssistantMainDirectiveWithURL(t *testing.T) {
	name, url := parseHomeAssistantMainDirective("home_assistant main: junglinster ${main_url};")
	if name != "junglinster" || url != "${main_url}" {
		t.Errorf("got (%q, %q), want (\"junglinster\", \"${main_url}\")", name, url)
	}
}

func TestParseHomeAssistantMainDirectiveIgnoresNamedInstances(t *testing.T) {
	name, url := parseHomeAssistantMainDirective("home_assistant protocols-server-2: protocols-server-2;")
	if name != "" || url != "" {
		t.Errorf("got (%q, %q), want (\"\", \"\") -- only \"main\" should match", name, url)
	}
}
