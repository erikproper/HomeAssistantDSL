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
	g.buckets["fs20"].tokens = 0.22 // exactly one frame
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

const testLight = `
status: governed/status
entities:
  - device: lights.test_dimmer
    label: light
    kind: light
    topic: governed/lights.test_dimmer/light
    default: "off"
    on_level: 60
    enforce_every: "00:05:00"
    channel: fs20
    paths:
      - set: fhem/fs20/radio.test_dimmer/set
        received: fhem/fs20/radio.test_dimmer
        encoding: fs20_dimmer
`

func newTestLight(t *testing.T) (*TGovernor, *tRecorder) {
	var devices TDevicesFile
	if err := yaml.Unmarshal([]byte(testLight), &devices); err != nil {
		t.Fatal(err)
	}
	if problems := checkDevicesFile(devices); len(problems) != 0 {
		t.Fatal(problems)
	}
	r := &tRecorder{}
	g := newGovernor(devices, r, func(string, ...any) {})
	g.Start(t0)
	g.Tick(t0)
	r.take()
	return g, r
}

// TestLight: Home Assistant's JSON in and out; "on" without a brightness is the on-level; the
// echo, rounded to the nearest dim level, is no change; a remote's level is.
func TestLight(t *testing.T) {
	g, r := newTestLight(t)
	at := t0.Add(time.Minute)
	g.Command("governed/lights.test_dimmer/light/set", `{"state":"ON"}`, at)
	g.Tick(at)
	equal(t, "on", r.take(), []string{`governed/lights.test_dimmer/light {"state":"ON","brightness":60} (r)`, "fhem/fs20/radio.test_dimmer/set 60"})
	g.Received("fhem/fs20/radio.test_dimmer", "dim62%", at.Add(200*time.Millisecond))
	equal(t, "echo", r.take(), nil)
	g.Command("governed/lights.test_dimmer/light/set", `{"state":"ON","brightness":40}`, at.Add(time.Minute))
	g.Tick(at.Add(time.Minute))
	equal(t, "brightness", r.take(), []string{`governed/lights.test_dimmer/light {"state":"ON","brightness":40} (r)`, "fhem/fs20/radio.test_dimmer/set 40"})
	g.Received("fhem/fs20/radio.test_dimmer", "dim75%", at.Add(2*time.Minute))
	equal(t, "remote", r.take(), []string{`governed/lights.test_dimmer/light {"state":"ON","brightness":75} (r)`})
	g.Received("fhem/fs20/radio.test_dimmer", "dimup", at.Add(3*time.Minute))
	g.Command("governed/lights.test_dimmer/light/set", `{"state":"OFF"}`, at.Add(4*time.Minute))
	g.Tick(at.Add(4 * time.Minute))
	equal(t, "off", r.take(), []string{`governed/lights.test_dimmer/light {"state":"OFF"} (r)`, "fhem/fs20/radio.test_dimmer/set off"})
}

// TestLightRetained: a light's state kept on the broker is read back.
func TestLightRetained(t *testing.T) {
	var devices TDevicesFile
	_ = yaml.Unmarshal([]byte(testLight), &devices)
	r := &tRecorder{}
	g := newGovernor(devices, r, func(string, ...any) {})
	g.RetainedState("governed/lights.test_dimmer/light", `{"state":"ON","brightness":31}`)
	g.Start(t0)
	g.Tick(t0)
	equal(t, "retained", r.take(), []string{`governed/lights.test_dimmer/light {"state":"ON","brightness":31} (r)`, "fhem/fs20/radio.test_dimmer/set 31"})
}

// TestNormaliseFS20Dim: a level becomes the nearest dim level, "off" below 3.
func TestNormaliseFS20Dim(t *testing.T) {
	for value, want := range map[string]string{"off": "off", "2": "off", "3": "6", "40": "37", "60": "62", "97": "100"} {
		if got := normaliseFS20Dim(value); got != want {
			t.Errorf("%s: got %s, want %s", value, got, want)
		}
	}
}

// TestCopiesWithin: a path's interval for copies is its radio channel's (FS20: 500 ms), unless
// the device has its own.
func TestCopiesWithin(t *testing.T) {
	for within, want := range map[string]time.Duration{"": 500 * time.Millisecond, "\n        copies_within_ms: 50": 50 * time.Millisecond} {
		var devices TDevicesFile
		if err := yaml.Unmarshal([]byte(strings.Replace(testDevices, "encoding: fs20_switch", "encoding: fs20_switch"+within, 1)), &devices); err != nil {
			t.Fatal(err)
		}
		g := newGovernor(devices, &tRecorder{}, func(string, ...any) {})
		if got := g.entities[0].copiesWithin[0]; got != want {
			t.Errorf("copies within %q: %v, want %v", within, got, want)
		}
	}
}
