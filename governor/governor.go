/*
 *
 * Module:    Governor
 * Package:   Main
 * Component: Governor
 *
 * The governor's logic (PROJECT.md 37, claude-notes/plans/governed-integration.md), apart from MQTT
 * (mqtt.go) so it can be tested with a clock of its own. Per governed entity:
 *
 *   - its desired state is what was last asked for, kept retained on its topic -- read back at the
 *     start, a default when the broker has none (the definition's, else the kind's);
 *   - a command (Home Assistant's, on <topic>/set) changes it and is sent at once;
 *   - every "enforce every" the desired state is sent again; the first ones are staggered;
 *   - sending goes through one pending slot per entity (a newer command replaces one not yet sent,
 *     a real command goes before an enforcement) and the radio channel's budget, a token bucket
 *     of air time refilled at the channel's duty cycle; a command costs a frame per radio sending
 *     it;
 *   - what a radio received for the device is a radio-requested change -- someone used a remote,
 *     or another radio sent -- and becomes the desired state (it is not a confirmation, and is not
 *     sent back); the echo of the governor's own send, within echoWindow, is ignored, and the same
 *     value from several radios within dedupWindow counts once.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 08.11.2026
 *
 */

package main

import (
	"strings"
	"time"
)

const (
	echoWindow  = 5 * time.Second // a received value the governor sent this recently is its own echo
	dedupWindow = 2 * time.Second // the same value received again this soon is the same press
)

// TKind is a kind of governed entity: its hard-wired default and what a command may say.
type TKind struct {
	Default string
	Parse   func(payload string) (string, bool)
}

// kinds are the kinds of governed entity.
var kinds = map[string]TKind{
	"switch": {Default: "off", Parse: parseOnOff},
}

// parseOnOff reads "on" or "off", in any case.
func parseOnOff(payload string) (string, bool) {
	switch value := strings.ToLower(strings.TrimSpace(payload)); value {
	case "on", "off":
		return value, true
	}
	return "", false
}

// TEncoding is how a path writes a value, and reads what its radio received.
type TEncoding struct {
	Encode func(value string) string
	Decode func(payload string) (string, bool)
}

// encodings are the paths' ways of writing values.
var encodings = map[string]TEncoding{
	"fs20_switch": {Encode: func(value string) string { return value }, Decode: decodeFS20Switch},
}

// decodeFS20Switch reads what FHEM reports for an FS20 switch: on, off, a remote's dim up or
// down (on, off), or a dim level (on unless 0).
func decodeFS20Switch(payload string) (string, bool) {
	switch payload = strings.TrimSpace(payload); {
	case payload == "on" || payload == "dimup" || strings.HasPrefix(payload, "on-"):
		return "on", true
	case payload == "off" || payload == "dimdown":
		return "off", true
	case strings.HasPrefix(payload, "dim") && strings.HasSuffix(payload, "%"):
		if payload == "dim00%" {
			return "off", true
		}
		return "on", true
	}
	return "", false
}

// TPublisher publishes a message.
type TPublisher interface {
	Publish(topic, payload string, retained bool)
}

// tPending is a value waiting to be sent.
type tPending struct {
	value       string
	enforcement bool
	since       time.Time
}

// tSent is the last value sent on a path.
type tSent struct {
	value string
	at    time.Time
}

// tEntity is a governed entity at run time.
type tEntity struct {
	def            TGovernedEntityDef
	kind           TKind
	desired        string
	known          bool
	pending        *tPending
	lastSent       time.Time
	sent           []tSent
	lastReceived   string
	lastReceivedAt time.Time
	cost           float64
}

// tPathRef is a path of an entity.
type tPathRef struct {
	entity *tEntity
	path   int
}

// tBucket is a radio channel's budget of air time, in seconds.
type tBucket struct {
	channel TChannel
	tokens  float64
	at      time.Time
}

// take spends cost when the bucket has it, refilling it first.
func (b *tBucket) take(cost float64, now time.Time) bool {
	if !b.at.IsZero() {
		b.tokens += now.Sub(b.at).Seconds() * b.channel.DutyCycle
	}
	b.at = now
	if b.tokens > b.channel.BurstSeconds {
		b.tokens = b.channel.BurstSeconds
	}
	if b.tokens < cost {
		return false
	}
	b.tokens -= cost
	return true
}

// TGovernor governs the entities of one devices.yaml.
type TGovernor struct {
	publisher  TPublisher
	logf       func(format string, args ...any)
	entities   []*tEntity
	byState    map[string]*tEntity
	byCommand  map[string]*tEntity
	byReceived map[string][]tPathRef
	buckets    map[string]*tBucket
	started    bool
}

// newGovernor returns the governor of devices, publishing through publisher.
func newGovernor(devices TDevicesFile, publisher TPublisher, logf func(string, ...any)) *TGovernor {
	g := &TGovernor{publisher: publisher, logf: logf, byState: map[string]*tEntity{}, byCommand: map[string]*tEntity{}, byReceived: map[string][]tPathRef{}, buckets: map[string]*tBucket{}}
	for _, def := range devices.Entities {
		channel, _ := channelOf(devices, def.Channel)
		if g.buckets[def.Channel] == nil {
			g.buckets[def.Channel] = &tBucket{channel: channel, tokens: channel.BurstSeconds}
		}
		e := &tEntity{def: def, kind: kinds[def.Kind], sent: make([]tSent, len(def.Paths))}
		for i, p := range def.Paths {
			e.cost += channel.FrameSeconds * float64(max(p.Copies, 1))
			if p.Received != "" {
				g.byReceived[p.Received] = append(g.byReceived[p.Received], tPathRef{e, i})
			}
		}
		g.entities = append(g.entities, e)
		g.byState[def.Topic] = e
		g.byCommand[def.Topic+"/set"] = e
	}
	return g
}

// StateTopics are the topics the governor keeps its desired states on.
func (g *TGovernor) StateTopics() []string {
	var topics []string
	for _, e := range g.entities {
		topics = append(topics, e.def.Topic)
	}
	return topics
}

// CommandTopics are the topics commands come in on.
func (g *TGovernor) CommandTopics() []string {
	var topics []string
	for _, e := range g.entities {
		topics = append(topics, e.def.Topic+"/set")
	}
	return topics
}

// ReceivedTopics are the topics the radios report on.
func (g *TGovernor) ReceivedTopics() []string {
	var topics []string
	for topic := range g.byReceived {
		topics = append(topics, topic)
	}
	return topics
}

// RetainedState takes a desired state the broker kept, before the start.
func (g *TGovernor) RetainedState(topic, payload string) {
	e := g.byState[topic]
	if e == nil || g.started {
		return
	}
	if value, ok := e.kind.Parse(payload); ok {
		e.desired, e.known = value, true
	}
}

// Start begins governing: an entity without a retained state gets its default, every state is
// published, and the first enforcements are staggered over the enforcement period.
func (g *TGovernor) Start(now time.Time) {
	for i, e := range g.entities {
		if !e.known {
			e.desired = e.def.Default
			if e.desired == "" {
				e.desired = e.kind.Default
			}
			e.known = true
			g.logf("%s %s: no retained state, default %s", e.def.Device, e.def.Label, e.desired)
		}
		g.publishState(e)
		offset := e.def.EnforceEvery.Duration * time.Duration(i) / time.Duration(len(g.entities))
		e.lastSent = now.Add(-e.def.EnforceEvery.Duration).Add(offset)
	}
	g.started = true
}

// Command takes a command for an entity.
func (g *TGovernor) Command(topic, payload string, now time.Time) {
	e := g.byCommand[topic]
	if e == nil || !g.started {
		return
	}
	value, ok := e.kind.Parse(payload)
	if !ok {
		g.logf("%s %s: ignored command %q", e.def.Device, e.def.Label, payload)
		return
	}
	e.desired = value
	g.publishState(e)
	e.pending = &tPending{value: value, since: now}
}

// Received takes what a radio received for a path's device.
func (g *TGovernor) Received(topic, payload string, now time.Time) {
	for _, ref := range g.byReceived[topic] {
		e := ref.entity
		value, ok := encodings[e.def.Paths[ref.path].Encoding].Decode(payload)
		if !ok || !g.started {
			continue
		}
		if sent := e.sent[ref.path]; sent.value == value && now.Sub(sent.at) < echoWindow {
			continue // the echo of its own send
		}
		if e.lastReceived == value && now.Sub(e.lastReceivedAt) < dedupWindow {
			continue // the same press, heard by another radio
		}
		e.lastReceived, e.lastReceivedAt = value, now
		if value != e.desired {
			g.logf("%s %s: radio-requested %s", e.def.Device, e.def.Label, value)
			e.desired = value
			e.pending = nil // the device has it already
			e.lastSent = now
			g.publishState(e)
		}
	}
}

// Tick enforces what is due and sends what the radio channels' budgets allow: real commands
// first, then enforcements, each the oldest first.
func (g *TGovernor) Tick(now time.Time) {
	if !g.started {
		return
	}
	for _, e := range g.entities {
		if e.pending == nil && e.def.EnforceEvery.Duration > 0 && now.Sub(e.lastSent) >= e.def.EnforceEvery.Duration {
			e.pending = &tPending{value: e.desired, enforcement: true, since: now}
		}
	}
	for _, enforcement := range []bool{false, true} {
		for {
			e := g.oldestPending(enforcement)
			if e == nil || !g.buckets[e.def.Channel].take(e.cost, now) {
				break
			}
			g.send(e, now)
		}
	}
}

// oldestPending returns the entity whose pending value of this sort waits longest.
func (g *TGovernor) oldestPending(enforcement bool) *tEntity {
	var oldest *tEntity
	for _, e := range g.entities {
		if e.pending != nil && e.pending.enforcement == enforcement && (oldest == nil || e.pending.since.Before(oldest.pending.since)) {
			oldest = e
		}
	}
	return oldest
}

// send sends e's pending value on every path.
func (g *TGovernor) send(e *tEntity, now time.Time) {
	value := e.pending.value
	for i, p := range e.def.Paths {
		g.publisher.Publish(p.Set, encodings[p.Encoding].Encode(value), false)
		e.sent[i] = tSent{value: value, at: now}
	}
	if !e.pending.enforcement {
		g.logf("%s %s: sent %s", e.def.Device, e.def.Label, value)
	}
	e.pending, e.lastSent = nil, now
}

// publishState publishes e's desired state, retained.
func (g *TGovernor) publishState(e *tEntity) {
	g.publisher.Publish(e.def.Topic, e.desired, true)
}
