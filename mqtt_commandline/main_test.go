/*
 *
 * Module:    MqttCommandline
 * Package:   Main
 * Component: MainTest
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 06.09.2026
 *
 */

package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// --- minimal mqtt.Client/Token/Message fakes, just enough to test handleSwitchCommand's own
// publish behaviour -- no real broker. ---

type fakeToken struct{}

func (fakeToken) Wait() bool                     { return true }
func (fakeToken) WaitTimeout(time.Duration) bool { return true }
func (fakeToken) Done() <-chan struct{}          { ch := make(chan struct{}); close(ch); return ch }
func (fakeToken) Error() error                   { return nil }

type recordedPublish struct {
	topic   string
	payload []byte
}

type fakeMessage struct{ payload []byte }

func (m fakeMessage) Duplicate() bool   { return false }
func (m fakeMessage) Qos() byte         { return 0 }
func (m fakeMessage) Retained() bool    { return false }
func (m fakeMessage) Topic() string     { return "" }
func (m fakeMessage) MessageID() uint16 { return 0 }
func (m fakeMessage) Payload() []byte   { return m.payload }
func (m fakeMessage) Ack()              {}

// fakeClient's Publish is called both from the test's own goroutine (handleSwitchCommand's
// synchronous, immediate optimistic publish) and from the background goroutine it spawns (the
// follow-up status_script re-check, 2026-09-21) -- mu guards published against that real
// concurrent access, not just a hypothetical one.
type fakeClient struct {
	mqtt.Client
	mu        sync.Mutex
	published []recordedPublish
}

func (c *fakeClient) Publish(topic string, _ byte, _ bool, payload interface{}) mqtt.Token {
	var data []byte
	switch p := payload.(type) {
	case []byte:
		data = p
	case string:
		data = []byte(p)
	}
	c.mu.Lock()
	c.published = append(c.published, recordedPublish{topic: topic, payload: data})
	c.mu.Unlock()
	return fakeToken{}
}

func (c *fakeClient) publishedSnapshot() []recordedPublish {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]recordedPublish, len(c.published))
	copy(out, c.published)
	return out
}

// waitForPublishCount polls until client has recorded at least n publishes or timeout elapses --
// handleSwitchCommand's background goroutine (the post-script status_script re-check) completes on
// its own schedule, not synchronously with the handler call.
func waitForPublishCount(t *testing.T, client *fakeClient, n int, timeout time.Duration) []recordedPublish {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		got := client.publishedSnapshot()
		if len(got) >= n {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("published = %+v, want at least %d publish(es) within %s", got, n, timeout)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	return path
}

func TestLoadConfigParsesAllThreeKinds(t *testing.T) {
	path := writeTempFile(t, `host: "frame"
switches:
  slideshow:
    status_script: "check_slideshow"
    on_script: "start_slideshow"
    off_script: "stop_slideshow"
buttons:
  reboot:
    press_script: "reboot_frame"
`)
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Host != "frame" {
		t.Errorf("Host = %q, want \"frame\"", cfg.Host)
	}
	sw, ok := cfg.Switches["slideshow"]
	if !ok || sw.StatusScript != "check_slideshow" || sw.OnScript != "start_slideshow" || sw.OffScript != "stop_slideshow" {
		t.Errorf("Switches[slideshow] = %+v, unexpected", sw)
	}
	b, ok := cfg.Buttons["reboot"]
	if !ok || b.PressScript != "reboot_frame" {
		t.Errorf("Buttons[reboot] = %+v, unexpected", b)
	}
}

func TestLoadConfigRejectsMissingHost(t *testing.T) {
	path := writeTempFile(t, "switches: {}\n")
	if _, err := loadConfig(path); err == nil {
		t.Fatalf("expected an error for a config with no \"host:\" set")
	}
}

func TestLoadSecretsParsesShellFormat(t *testing.T) {
	path := writeTempFile(t, "mqtt_server=vienna\nmqtt_login=carvalhoproper\nmqtt_password=hunter2\nmqtt_port=1883\nmqtt_tls=0\n")
	secrets, err := loadSecrets(path)
	if err != nil {
		t.Fatalf("loadSecrets: %v", err)
	}
	if secrets.Server != "vienna" || secrets.Login != "carvalhoproper" || secrets.Password != "hunter2" || secrets.Port != "1883" || secrets.TLS {
		t.Errorf("secrets = %+v, unexpected", secrets)
	}
}

func TestLoadSecretsRejectsMissingServer(t *testing.T) {
	path := writeTempFile(t, "mqtt_login=x\n")
	if _, err := loadSecrets(path); err == nil {
		t.Fatalf("expected an error for secrets with no mqtt_server/mqtt_port set")
	}
}

func TestRunStatusScriptTrimsStdout(t *testing.T) {
	out, err := runStatusScript("echo '  1  '")
	if err != nil {
		t.Fatalf("runStatusScript: %v", err)
	}
	if out != "1" {
		t.Errorf("runStatusScript output = %q, want \"1\" (trimmed)", out)
	}
}

// TestRunStatusScriptWordSplitsArguments confirms scripts are invoked via ordinary shell
// word-splitting (PROJECT.md item 1's own design), not passed as one single opaque argument -- a
// script declared with its own arguments (e.g. "echo foo") must actually see them split.
func TestRunStatusScriptWordSplitsArguments(t *testing.T) {
	out, err := runStatusScript("echo foo bar")
	if err != nil {
		t.Fatalf("runStatusScript: %v", err)
	}
	if out != "foo bar" {
		t.Errorf("runStatusScript output = %q, want \"foo bar\"", out)
	}
}

// TestRunStatusScriptTreatsNonZeroExitAsMeaningfulNotAsFailure is the regression test for a real
// bug found live 2026-09-06: check_slideshow (already deployed on frame.vienna) prints "1" AND
// exits non-zero to signal "not running" -- a predicate-script convention, not an execution
// failure. runStatusScript must still return that stdout, with no error, so the daemon actually
// publishes "1" (off) instead of silently dropping the state update.
func TestRunStatusScriptTreatsNonZeroExitAsMeaningfulNotAsFailure(t *testing.T) {
	out, err := runStatusScript("echo 1; exit 1")
	if err != nil {
		t.Fatalf("runStatusScript returned an error for a non-zero exit with real stdout: %v", err)
	}
	if out != "1" {
		t.Errorf("runStatusScript output = %q, want \"1\"", out)
	}
}

// TestRunCommandScriptTreatsNonZeroExitAsFailure is the regression test for a real bug found live
// 2026-09-07: runCommandScript must NOT share runStatusScript's own leniency -- an on/off/press
// script's non-zero exit is a genuine failure signal (no known equivalent to check_slideshow's own
// "0/1 on stdout AND via exit code" convention exists for command scripts), and handleSwitchCommand
// relies on this to decide whether to optimistically report the commanded target state.
func TestRunCommandScriptTreatsNonZeroExitAsFailure(t *testing.T) {
	if err := runCommandScript("exit 1"); err == nil {
		t.Fatalf("expected an error for a command script that exits non-zero")
	}
}

func TestRunCommandScriptSucceedsOnZeroExit(t *testing.T) {
	if err := runCommandScript("true"); err != nil {
		t.Fatalf("runCommandScript: %v", err)
	}
}

// TestHandleSwitchCommandPublishesTargetStateBeforeRunningScript confirms the (2026-09-21) fix for
// a real bug: the COMMANDED payload must be published to the state topic BEFORE the on/off script
// runs, not after it completes -- an earlier version published only after a successful script run,
// which meant HA's own (non-optimistic, since state_topic is set) switch UI stayed showing the OLD
// state for however long the script took (observed live: ~4s for start_slideshow/stop_slideshow's
// own "sudo xrandr"/X round-trip), visibly "bouncing" before settling. The script itself now runs in
// a background goroutine, which then re-runs status_script and republishes once the script is done
// (a second, later publish) -- this test only asserts the FIRST, immediate/synchronous publish, by
// requiring exactly one at the moment the handler call returns, before that goroutine has had any
// chance to run.
func TestHandleSwitchCommandPublishesTargetStateBeforeRunningScript(t *testing.T) {
	client := &fakeClient{}
	cfg := TSwitchConfig{OnScript: "true", OffScript: "true", StatusScript: "echo should-not-run-yet"}
	handler := handleSwitchCommand(client, "frame", "slideshow", cfg)

	handler(client, fakeMessage{payload: []byte("1")})
	published := client.publishedSnapshot()

	if len(published) != 1 {
		t.Fatalf("published = %+v, want exactly one publish synchronously", published)
	}
	if published[0].topic != "commandline/frame/slideshow/state" {
		t.Errorf("topic = %q, want the slideshow state topic", published[0].topic)
	}
	if string(published[0].payload) != "1" {
		t.Errorf("payload = %q, want the commanded target \"1\", not a re-run of status_script", published[0].payload)
	}
}

// TestHandleSwitchCommandRepublishesRealStateOnceScriptFinishes confirms the (2026-09-21) tightened
// self-correction: once the background on/off script completes (success or failure), status_script
// is re-run and its real output republished -- catching a wrong optimistic guess within about as
// long as the command script itself takes, rather than waiting for statusPollInterval's own up-to-
// 60s periodic poll. Covers both outcomes: a succeeding script whose real status agrees with what
// was optimistically published, and a failing script whose real status contradicts it.
func TestHandleSwitchCommandRepublishesRealStateOnceScriptFinishes(t *testing.T) {
	cases := []struct {
		name       string
		script     string
		realStatus string
	}{
		{name: "succeeding script, status agrees", script: "true", realStatus: "1"},
		{name: "failing script, status contradicts the optimistic guess", script: "false", realStatus: "0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeClient{}
			cfg := TSwitchConfig{OnScript: tc.script, OffScript: tc.script, StatusScript: "echo " + tc.realStatus}
			handler := handleSwitchCommand(client, "frame", "slideshow", cfg)

			handler(client, fakeMessage{payload: []byte("1")})
			published := waitForPublishCount(t, client, 2, time.Second)

			if published[0].topic != "commandline/frame/slideshow/state" || string(published[0].payload) != "1" {
				t.Fatalf("first (optimistic) publish = %+v, want target \"1\" on the state topic", published[0])
			}
			if published[1].topic != "commandline/frame/slideshow/state" || string(published[1].payload) != tc.realStatus {
				t.Errorf("second (status re-check) publish = %+v, want %q on the state topic", published[1], tc.realStatus)
			}
		})
	}
}

// TestHandleSwitchCommandIgnoresUnrecognisedPayload confirms a payload that's neither "1" nor "0"
// runs nothing and publishes nothing.
func TestHandleSwitchCommandIgnoresUnrecognisedPayload(t *testing.T) {
	client := &fakeClient{}
	cfg := TSwitchConfig{OnScript: "true", OffScript: "true", StatusScript: "echo 0"}
	handler := handleSwitchCommand(client, "frame", "slideshow", cfg)

	handler(client, fakeMessage{payload: []byte("toggle")})

	if len(client.published) != 0 {
		t.Fatalf("published = %+v, want no publish for an unrecognised payload", client.published)
	}
}

func TestRunStatusScriptReturnsErrorWhenShellItselfCannotRun(t *testing.T) {
	orig := shellPath
	shellPath = "/nonexistent/sh"
	defer func() { shellPath = orig }()
	if _, err := runStatusScript("echo hi"); err == nil {
		t.Fatalf("expected an error when the shell itself cannot be executed")
	}
}

func TestRunCommandScriptReturnsErrorWhenShellItselfCannotRun(t *testing.T) {
	orig := shellPath
	shellPath = "/nonexistent/sh"
	defer func() { shellPath = orig }()
	if err := runCommandScript("echo hi"); err == nil {
		t.Fatalf("expected an error when the shell itself cannot be executed")
	}
}
