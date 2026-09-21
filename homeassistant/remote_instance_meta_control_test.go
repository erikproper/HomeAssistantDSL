package main

import (
	"strings"
	"testing"
)

func TestMetaReloadRequestTopic(t *testing.T) {
	if got := metaReloadRequestTopic("main"); got != "meta/reload/main/request" {
		t.Errorf("metaReloadRequestTopic(main) = %q, want %q", got, "meta/reload/main/request")
	}
}

func TestMetaRestartRequestTopic(t *testing.T) {
	if got := metaRestartRequestTopic("protocols-server-2"); got != "meta/restart/protocols-server-2/request" {
		t.Errorf("metaRestartRequestTopic(protocols-server-2) = %q, want %q", got, "meta/restart/protocols-server-2/request")
	}
}

// TestMetaReloadAutomationBodyTriggersOnOwnTopicAndCallsReloadAll is the regression test for the
// 2026-09-17 simplification: this used to be several independent automations hand-listing every
// "<domain>.reload" service (to work around HA's script engine re-raising ServiceNotFound
// regardless of continue_on_error, plus automation.reload's own self-cancellation needing a
// delayed, isolated automation) -- replaced with a single call to homeassistant.reload_all, HA's
// own first-party equivalent of the UI's "Reload all YAML configuration" quick action (confirmed
// present live on Vienna, HA Core 2026.9.2), which sidesteps that whole problem class since it's
// HA's own internal orchestration, not a hand-rolled reimplementation of it.
func TestMetaReloadAutomationBodyTriggersOnOwnTopicAndCallsReloadAll(t *testing.T) {
	body := metaReloadAutomationBody("main")
	if !strings.Contains(body, `topic: "meta/reload/main/request"`) {
		t.Errorf("body = %s, want it to trigger on meta/reload/main/request", body)
	}
	if !strings.Contains(body, "service: homeassistant.reload_all") {
		t.Errorf("body = %s, want a homeassistant.reload_all call", body)
	}
	if strings.Count(body, "- alias:") != 1 {
		t.Errorf("body = %s, want exactly one automation (no per-domain isolation needed anymore)", body)
	}
}

func TestMetaRestartAutomationBodyTriggersOnOwnTopicAndCallsRestart(t *testing.T) {
	body := metaRestartAutomationBody("protocols-server-2")
	if !strings.Contains(body, `topic: "meta/restart/protocols-server-2/request"`) {
		t.Errorf("body = %s, want it to trigger on meta/restart/protocols-server-2/request", body)
	}
	if !strings.Contains(body, "service: homeassistant.restart") {
		t.Errorf("body = %s, want a homeassistant.restart call", body)
	}
}
