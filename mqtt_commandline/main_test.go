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

type fakeClient struct {
	mqtt.Client
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
	c.published = append(c.published, recordedPublish{topic: topic, payload: data})
	return fakeToken{}
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

// TestHandleSwitchCommandPublishesTargetStateOnSuccess confirms the new (2026-09-07) optimistic
// reporting behaviour: on a successful on/off script, the COMMANDED payload is published
// immediately to the state topic, without re-running status_script first -- avoids waiting on a
// potentially slow status script (e.g. check_slideshow's own "sudo xrandr" call) just to confirm
// what was just commanded. Any real discrepancy is left for the next periodic poll to correct.
func TestHandleSwitchCommandPublishesTargetStateOnSuccess(t *testing.T) {
	client := &fakeClient{}
	cfg := TSwitchConfig{OnScript: "true", OffScript: "true", StatusScript: "echo should-not-run"}
	handler := handleSwitchCommand(client, "frame", "slideshow", cfg)

	handler(client, fakeMessage{payload: []byte("1")})

	if len(client.published) != 1 {
		t.Fatalf("published = %+v, want exactly one publish", client.published)
	}
	if client.published[0].topic != "commandline/frame/slideshow/state" {
		t.Errorf("topic = %q, want the slideshow state topic", client.published[0].topic)
	}
	if string(client.published[0].payload) != "1" {
		t.Errorf("payload = %q, want the commanded target \"1\", not a re-run of status_script", client.published[0].payload)
	}
}

// TestHandleSwitchCommandPublishesNothingOnFailure confirms a failing on/off script publishes
// nothing at all -- the retained state is left as-is, for the next periodic poll to resolve,
// rather than optimistically reporting a target state that was never actually reached.
func TestHandleSwitchCommandPublishesNothingOnFailure(t *testing.T) {
	client := &fakeClient{}
	cfg := TSwitchConfig{OnScript: "false", OffScript: "false", StatusScript: "echo 0"}
	handler := handleSwitchCommand(client, "frame", "slideshow", cfg)

	handler(client, fakeMessage{payload: []byte("1")})

	if len(client.published) != 0 {
		t.Fatalf("published = %+v, want no publish for a failing command script", client.published)
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
