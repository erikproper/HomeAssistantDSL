/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: Liveness
 *
 * PROJECT.md 25c (design: claude-notes/plans/gateway-liveness.md, agreed with Erik 2026-11-01): one
 * generic liveness check for anything that talks MQTT -- a bridged Home Assistant instance, a
 * Zigbee2MQTT, a Z-Wave JS UI -- with no gateway-specific code here; a TLivenessSpec says per
 * gateway:
 *
 *   - Traffic: the topics whose messages are signs of life (a subscription filter), minus Ignore:
 *     filters for what others send there (commands from main, the coordinator's own inquiries and
 *     shadows). In Ignore, "#" may also stand in the middle, for any number of levels:
 *     "zigbee2mqtt/#/set" is every command, however deep the device's name ("a/b/c");
 *   - Quiet: how long without them before asking;
 *   - Ask/AskPayload: the question;
 *   - Answer and AnswerPayload or AnswerField: what counts as a healthy answer (the exact payload,
 *     or a JSON field that is true), within Within;
 *   - Verdict, Online, Offline: the topic the entities' availability reads, and its payloads;
 *   - Misses: how many unanswered questions in a row (each Within apart) make it offline.
 *
 * Nothing is sent while the gateway talks anyway: only after a quiet spell does the coordinator
 * ask. A gateway with a Last Will writes the verdict itself when it stops; this adds the hang
 * (connected, but not working): it writes Offline to that same topic, retained like the Last Will,
 * and Online on the next sign of life -- which a recovering gateway overwrites itself anyway. Not
 * signs of life: retained messages (they say nothing about now), the coordinator's own question,
 * Ignore, the verdict topic (also written by us, by the Last Will and by the gateway when it
 * connects -- whose live Online or Offline we publish retained ourselves, as a gateway that
 * doesn't retain would otherwise leave our earlier verdict there, found 2026-11-07 on a Shelly
 * plug), and an unhealthy answer. Ignore matters: an
 * HA command on zigbee2mqtt/<device>/set would otherwise count as life while Zigbee2MQTT hangs --
 * and ha2mqtt's original ping-pong counted the coordinator's own inquiry every minute, so it never
 * got quiet and a hang there went unnoticed (found 2026-11-01). At start
 * the coordinator asks at once, so the state is known right away.
 *
 * Generalized from instance_liveness.go (ha2mqtt's ping-pong, PROJECT.md 26, 2026-10-28), which is
 * now one spec of this.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 07.11.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gopkg.in/yaml.v3"
)

// TLivenessSpec is one gateway's liveness check (see the file comment).
type TLivenessSpec struct {
	Name          string
	Traffic       string
	Ignore        []string
	Quiet         time.Duration
	Ask           string
	AskPayload    string
	Answer        string
	AnswerPayload string // "" when AnswerField decides
	AnswerField   string // dot path into the answer's JSON, e.g. "data.healthy"
	Within        time.Duration
	Verdict       string
	Online        string
	Offline       string
	Misses        int
}

// TLiveness tracks one gateway.
type TLiveness struct {
	mu       sync.Mutex
	client   mqtt.Client
	spec     TLivenessSpec
	status   string // the verdict we last published, "" before any
	quiet    *time.Timer
	waiting  *time.Timer
	unanswer int // unanswered questions in a row
}

func newLiveness(client mqtt.Client, spec TLivenessSpec) *TLiveness {
	if spec.Misses < 1 {
		spec.Misses = 1
	}
	return &TLiveness{client: client, spec: spec}
}

func (l *TLiveness) start() error {
	filters := []string{l.spec.Traffic}
	if l.spec.Answer != "" && !topicMatches(l.spec.Traffic, l.spec.Answer) {
		filters = append(filters, l.spec.Answer)
	}
	for _, filter := range filters {
		token := subscribe(l.client, filter, 0, func(_ mqtt.Client, msg mqtt.Message) {
			l.observe(msg.Topic(), string(msg.Payload()), msg.Retained())
		})
		if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
			return fmt.Errorf("subscribing to %s: %v", filter, token.Error())
		}
	}
	l.ask()
	fmt.Printf("[liveness] tracking %s\n", l.spec.Name)
	return nil
}

// observe handles one message on the gateway's topics.
func (l *TLiveness) observe(topic, payload string, retained bool) {
	if retained {
		return
	}
	if topic == l.spec.Verdict {
		l.mu.Lock()
		if payload == l.spec.Online || payload == l.spec.Offline {
			// The gateway's own word (its connect message, or its Last Will), kept retained: a
			// client subscribing later must read the same, also when the gateway doesn't retain.
			l.setStatus(payload)
		} else {
			l.status = payload // the next sign of life republishes Online
		}
		l.mu.Unlock()
		return
	}
	for _, filter := range l.spec.Ignore {
		if ignoreMatches(filter, topic) {
			return
		}
	}
	if topic == l.spec.Answer && !(topic == l.spec.Ask && payload == l.spec.AskPayload) {
		if l.healthy(payload) {
			l.alive()
		}
		return
	}
	if topic == l.spec.Ask && payload == l.spec.AskPayload {
		return
	}
	if topicMatches(l.spec.Traffic, topic) {
		l.alive()
	}
}

// healthy reports whether payload is a healthy answer.
func (l *TLiveness) healthy(payload string) bool {
	if l.spec.AnswerField == "" {
		return payload == l.spec.AnswerPayload
	}
	var value interface{}
	if json.Unmarshal([]byte(payload), &value) != nil {
		return false
	}
	for _, key := range strings.Split(l.spec.AnswerField, ".") {
		object, ok := value.(map[string]interface{})
		if !ok {
			return false
		}
		value = object[key]
	}
	return value == true
}

func (l *TLiveness) alive() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.waiting != nil {
		l.waiting.Stop()
		l.waiting = nil
	}
	l.unanswer = 0
	l.setStatus(l.spec.Online)
	l.resetQuiet()
}

// resetQuiet (re)arms the quiet-spell timer; l.mu held.
func (l *TLiveness) resetQuiet() {
	if l.quiet != nil {
		l.quiet.Stop()
	}
	l.quiet = time.AfterFunc(l.spec.Quiet, l.ask)
}

// ask sends the question; no healthy answer within Within counts as a miss, and Misses of them in a
// row make the gateway offline -- until then it asks again right away.
func (l *TLiveness) ask() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.waiting != nil {
		return // already waiting
	}
	l.client.Publish(l.spec.Ask, 0, false, l.spec.AskPayload)
	l.waiting = time.AfterFunc(l.spec.Within, func() {
		l.mu.Lock()
		l.waiting = nil
		l.unanswer++
		if l.unanswer < l.spec.Misses {
			l.mu.Unlock()
			l.ask()
			return
		}
		defer l.mu.Unlock()
		l.setStatus(l.spec.Offline)
		l.resetQuiet() // ask again after the next quiet spell
	})
}

// setStatus publishes the verdict, retained, when it changed; l.mu held.
func (l *TLiveness) setStatus(status string) {
	if l.status == status {
		return
	}
	l.status = status
	fmt.Printf("[liveness] %s: %s\n", l.spec.Name, status)
	l.client.Publish(l.spec.Verdict, 1, true, status)
}

// ignoreMatches is topicMatches, but with "#" allowed anywhere, matching zero or more levels.
func ignoreMatches(filter, topic string) bool {
	return levelsMatch(strings.Split(filter, "/"), strings.Split(topic, "/"))
}

func levelsMatch(f, t []string) bool {
	if len(f) == 0 {
		return len(t) == 0
	}
	if f[0] == "#" {
		for i := 0; i <= len(t); i++ {
			if levelsMatch(f[1:], t[i:]) {
				return true
			}
		}
		return false
	}
	if len(t) == 0 || (f[0] != "+" && f[0] != t[0]) {
		return false
	}
	return levelsMatch(f[1:], t[1:])
}

// topicMatches reports whether topic matches the MQTT subscription filter (with + and #).
func topicMatches(filter, topic string) bool {
	f := strings.Split(filter, "/")
	t := strings.Split(topic, "/")
	for i, part := range f {
		if part == "#" {
			return true
		}
		if i >= len(t) || (part != "+" && part != t[i]) {
			return false
		}
	}
	return len(f) == len(t)
}

// TLivenessFile is coordinator/liveness.yaml, written by the generator from the gateways'
// "liveness with:" blocks in Physical.def (homeassistant/physical_liveness.go).
type TLivenessFile struct {
	Gateways []struct {
		Device        string   `yaml:"device"`
		Traffic       string   `yaml:"traffic"`
		Ignore        []string `yaml:"ignore"`
		Quiet         string   `yaml:"quiet"`
		Ask           string   `yaml:"ask"`
		AskPayload    string   `yaml:"ask_payload"`
		Answer        string   `yaml:"answer"`
		AnswerPayload string   `yaml:"answer_payload"`
		AnswerField   string   `yaml:"answer_field"`
		Within        string   `yaml:"within"`
		Verdict       string   `yaml:"verdict"`
		Online        string   `yaml:"online"`
		Offline       string   `yaml:"offline"`
		Misses        int      `yaml:"misses"`
	} `yaml:"gateways"`
}

// loadLivenessFile reads path; a missing file means no gateways to check.
func loadLivenessFile(path string) (TLivenessFile, error) {
	var file TLivenessFile
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return file, nil
	}
	if err != nil {
		return file, err
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return file, fmt.Errorf("%s: %w", path, err)
	}
	return file, nil
}

// livenessDuration parses an "HH:MM:SS" duration.
func livenessDuration(hms string) (time.Duration, error) {
	var h, m, s int
	if n, err := fmt.Sscanf(hms, "%d:%d:%d", &h, &m, &s); err != nil || n != 3 {
		return 0, fmt.Errorf("duration %q is no HH:MM:SS", hms)
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second, nil
}

// livenessSpecs turns the file's entries into specs; an entry with a bad duration is reported and
// skipped.
func livenessSpecs(file TLivenessFile) []TLivenessSpec {
	var specs []TLivenessSpec
	for _, g := range file.Gateways {
		quiet, err1 := livenessDuration(g.Quiet)
		within, err2 := livenessDuration(g.Within)
		if err1 != nil || err2 != nil || quiet <= 0 || within <= 0 {
			fmt.Printf("[liveness] %s: skipped -- quiet %q, within %q\n", g.Device, g.Quiet, g.Within)
			continue
		}
		specs = append(specs, TLivenessSpec{
			Name: g.Device, Traffic: g.Traffic, Ignore: g.Ignore, Quiet: quiet,
			Ask: g.Ask, AskPayload: g.AskPayload, Answer: g.Answer, AnswerPayload: g.AnswerPayload, AnswerField: g.AnswerField,
			Within: within, Verdict: g.Verdict, Online: g.Online, Offline: g.Offline, Misses: g.Misses,
		})
	}
	return specs
}

// startGatewayLiveness starts a check per declared gateway.
func startGatewayLiveness(client mqtt.Client, file TLivenessFile) error {
	for _, spec := range livenessSpecs(file) {
		if err := newLiveness(client, spec).start(); err != nil {
			return err
		}
	}
	return nil
}
