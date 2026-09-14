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

func TestMetaReloadAutomationBodyTriggersOnOwnTopicAndCallsEveryReloadService(t *testing.T) {
	body := metaReloadAutomationBody("main")
	if !strings.Contains(body, `topic: "meta/reload/main/request"`) {
		t.Errorf("body = %s, want it to trigger on meta/reload/main/request", body)
	}
	for _, service := range append(append([]string{}, metaReloadServices...), "homeassistant.reload_core_config", "automation.reload") {
		if !strings.Contains(body, "service: "+service) {
			t.Errorf("body missing a plain static call to %q: %s", service, body)
		}
	}
	if !strings.Contains(body, "continue_on_error: true") {
		t.Errorf("body = %s, want every reload step to continue_on_error so one missing domain doesn't abort the rest", body)
	}
}

// TestMetaReloadAutomationBodyCallsAutomationReloadLast is a regression test for a real bug found
// live 2026-09-08 (protocols-server-2): automation.reload reloads and recreates every automation
// entity, including this one, mid-run -- HA cancels its own currently-executing script as a direct
// consequence (asyncio.CancelledError, then InvalidStateError from the entity's own removal-future
// being resolved twice), and any action listed AFTER automation.reload silently never runs once
// that hits. When automation.reload was the FIRST action, this meant script.reload and every
// optional domain reload plus homeassistant.reload_core_config never executed at all -- the whole
// point of "reload," gone, with no obviously visible error. automation.reload must be the very
// last action, so every other reload has already completed by the time it (unavoidably)
// cancels itself.
func TestMetaReloadAutomationBodyCallsAutomationReloadLast(t *testing.T) {
	body := metaReloadAutomationBody("main")
	lines := strings.Split(body, "\n")
	var serviceLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- service:") {
			serviceLines = append(serviceLines, trimmed)
		}
	}
	if len(serviceLines) == 0 {
		t.Fatalf("body has no service calls at all: %s", body)
	}
	last := serviceLines[len(serviceLines)-1]
	if last != "- service: automation.reload" {
		t.Errorf("last service call = %q, want \"- service: automation.reload\" (must be dead last -- it cancels its own currently-executing script, so everything else must already be done): %s", last, body)
	}
	for _, line := range serviceLines[:len(serviceLines)-1] {
		if line == "- service: automation.reload" {
			t.Errorf("automation.reload appears before the final action -- everything after it would silently never run: %s", body)
		}
	}
}

// TestMetaReloadAutomationBodyTemplatesOptionalServiceNames is a regression test for a real bug
// found live 2026-09-06: HA statically validates a literal "service: template.reload" at
// automation-LOAD time, so an instance with no YAML template entities configured got a persistent
// "unknown action" Repair issue -- continue_on_error can't help with that, since it only guards a
// RUNTIME failure, never a load-time validation error. Every optional/config-dependent domain's
// reload call must instead use a Jinja-templated service name, which HA can't statically
// validate and so defers checking to execution time.
func TestMetaReloadAutomationBodyTemplatesOptionalServiceNames(t *testing.T) {
	body := metaReloadAutomationBody("main")
	for _, service := range metaOptionalReloadServices {
		want := `service: "{{ '` + service + `' }}"`
		if !strings.Contains(body, want) {
			t.Errorf("body missing templated call %q (would otherwise be statically validated and risk a persistent Repair issue on an instance without that domain configured): %s", want, body)
		}
		if strings.Contains(body, "service: "+service+"\n") {
			t.Errorf("body = %s, want %q called only via a templated service name, never a plain static one", body, service)
		}
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
