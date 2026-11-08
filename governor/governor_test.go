package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// tRecorder records what the governor publishes.
type tRecorder struct{ messages []string }

func (r *tRecorder) Publish(topic, payload string, retained bool) {
	mark := ""
	if retained {
		mark = " (r)"
	}
	r.messages = append(r.messages, fmt.Sprintf("%s %s%s", topic, payload, mark))
}

func (r *tRecorder) take() []string {
	messages := r.messages
	r.messages = nil
	return messages
}

const testDevices = `
status: governed/status
entities:
  - device: lights.test_switch
    label: switch
    kind: switch
    topic: governed/lights.test_switch/switch
    default: "off"
    enforce_every: "00:05:00"
    channel: fs20
    paths:
      - set: fhem/fs20/radio.test_switch/set
        received: fhem/fs20/radio.test_switch
        encoding: fs20_switch
        copies: 2
  - device: lights.other
    label: switch
    kind: switch
    topic: governed/lights.other/switch
    enforce_every: "00:05:00"
    channel: fs20
    paths:
      - set: fhem/fs20/radio.other/set
        received: fhem/fs20/radio.other
        encoding: fs20_switch
        copies: 1
`

func newTestGovernor(t *testing.T) (*TGovernor, *tRecorder) {
	var devices TDevicesFile
	if err := yaml.Unmarshal([]byte(testDevices), &devices); err != nil {
		t.Fatal(err)
	}
	if problems := checkDevicesFile(devices); len(problems) != 0 {
		t.Fatal(problems)
	}
	r := &tRecorder{}
	return newGovernor(devices, r, func(string, ...any) {}), r
}

var t0 = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func equal(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("%s:\n got %q\nwant %q", what, got, want)
	}
}

// TestStart: a retained state is kept, a missing one becomes the default; the states are
// published, and the first enforcement goes out at once, the other's half a period later.
func TestStart(t *testing.T) {
	g, r := newTestGovernor(t)
	g.RetainedState("governed/lights.test_switch/switch", "on")
	g.Start(t0)
	equal(t, "start", r.take(), []string{"governed/lights.test_switch/switch on (r)", "governed/lights.other/switch off (r)"})
	g.Tick(t0)
	equal(t, "first tick", r.take(), []string{"fhem/fs20/radio.test_switch/set on"})
	g.Tick(t0.Add(2*time.Minute + 30*time.Second))
	equal(t, "half a period", r.take(), []string{"fhem/fs20/radio.other/set off"})
	g.Tick(t0.Add(5 * time.Minute))
	equal(t, "a period", r.take(), []string{"fhem/fs20/radio.test_switch/set on"})
}

// TestCommand: a command changes the desired state, which is published and sent at once.
func TestCommand(t *testing.T) {
	g, r := newTestGovernor(t)
	g.Start(t0)
	g.Tick(t0)
	r.take()
	g.Command("governed/lights.test_switch/switch/set", "ON", t0.Add(time.Second))
	g.Command("governed/lights.test_switch/switch/set", "dim", t0.Add(time.Second))
	g.Tick(t0.Add(time.Second))
	equal(t, "command", r.take(), []string{"governed/lights.test_switch/switch on (r)", "fhem/fs20/radio.test_switch/set on"})
}

// TestReceived: the echo of its own send is ignored; a remote's change becomes the desired state,
// once, however many radios heard it; a dim-down is off.
func TestReceived(t *testing.T) {
	g, r := newTestGovernor(t)
	g.Start(t0)
	g.Tick(t0)
	r.take()
	g.Received("fhem/fs20/radio.test_switch", "off", t0.Add(time.Second))
	g.Received("fhem/fs20/radio.test_switch", "off", t0.Add(time.Second))
	equal(t, "echo", r.take(), nil)
	at := t0.Add(time.Minute)
	g.Received("fhem/fs20/radio.test_switch", "on", at)
	g.Received("fhem/fs20/radio.test_switch", "on", at.Add(100*time.Millisecond))
	g.Tick(at.Add(time.Second))
	equal(t, "remote on", r.take(), []string{"governed/lights.test_switch/switch on (r)"})
	g.Received("fhem/fs20/radio.test_switch", "dimdown", at.Add(time.Minute))
	equal(t, "remote dim down", r.take(), []string{"governed/lights.test_switch/switch off (r)"})
}

// TestThrottle: the radio channel's budget lets a burst through, then one command per frame's
// worth of duty cycle; real commands go before enforcements.
func TestThrottle(t *testing.T) {
	g, r := newTestGovernor(t)
	g.Start(t0)
	r.take()
	g.buckets["fs20"].tokens = 0.44 // exactly test_switch's two frames
	g.Tick(t0)
	equal(t, "first", r.take(), []string{"fhem/fs20/radio.test_switch/set off"})
	g.Command("governed/lights.other/switch/set", "on", t0)
	g.Tick(t0)
	equal(t, "no budget", r.take(), []string{"governed/lights.other/switch on (r)"})
	g.Tick(t0.Add(22 * time.Second)) // 0.22 s of air time back
	equal(t, "budget back", r.take(), []string{"fhem/fs20/radio.other/set on"})
}

// TestDuration reads and writes HH:MM:SS.
func TestDuration(t *testing.T) {
	var d struct{ D Duration }
	if err := yaml.Unmarshal([]byte("d: \"01:02:03\""), &d); err != nil || d.D.Duration != time.Hour+2*time.Minute+3*time.Second {
		t.Fatalf("%v %v", d.D, err)
	}
	out, _ := yaml.Marshal(d)
	if strings.TrimSpace(string(out)) != `d: "01:02:03"` {
		t.Errorf("%s", out)
	}
}
