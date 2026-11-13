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
 *     of air time refilled at the channel's duty cycle; a command costs a frame (each radio sending
 *     it has a duty cycle of its own);
 *   - what a radio received for the device is a radio-requested change -- someone used a remote,
 *     or another radio sent -- and becomes the desired state (it is not a confirmation, and is not
 *     sent back); the echo of any of the governor's own sends within echoWindow is ignored -- not
 *     only the last one's: flipping fast, an older send's echo may come after a newer send (Erik,
 *     2026-11-13) -- and the same
 *     message from several radios within the copies interval counts once -- the radio channel's
 *     hard-wired one, or the device's own; compared per message, so a held button's repeated
 *     dim steps, further apart, stay presses of their own.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 13.11.2026
 *
 */

package main

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	echoWindow = 5 * time.Second // a received value the governor sent this recently is its own echo
)

// TKind is a kind of governed entity: its hard-wired default, what a command (or a state kept on
// the broker) may say, and how its state is written for Home Assistant. A value is "on"/"off" for
// a switch, "off" or a level 1..100 for a light.
type TKind struct {
	Default     string
	Parse       func(payload string, onLevel int) (string, bool)
	FormatState func(value string) string
}

// kinds are the kinds of governed entity.
var kinds = map[string]TKind{
	"switch": {Default: "off", Parse: func(payload string, _ int) (string, bool) { return parseOnOff(payload) }, FormatState: func(value string) string { return value }},
	"light":  {Default: "off", Parse: parseLight, FormatState: formatLight},
}

// tLightJSON is Home Assistant's JSON light payload, brightness 0..100 (brightness_scale 100).
type tLightJSON struct {
	State      string `json:"state"`
	Brightness *int   `json:"brightness,omitempty"`
}

// parseLight reads a light's command or state: Home Assistant's JSON ("ON" without a brightness
// is onLevel), or plainly "on", "off" or a level.
func parseLight(payload string, onLevel int) (string, bool) {
	var light tLightJSON
	if err := json.Unmarshal([]byte(payload), &light); err == nil && light.State != "" {
		switch strings.ToLower(light.State) {
		case "off":
			return "off", true
		case "on":
			if light.Brightness == nil {
				return strconv.Itoa(onLevel), true
			}
			return lightLevel(*light.Brightness), true
		}
		return "", false
	}
	switch value := strings.ToLower(strings.TrimSpace(payload)); value {
	case "off":
		return "off", true
	case "on":
		return strconv.Itoa(onLevel), true
	default:
		if level, err := strconv.Atoi(value); err == nil {
			return lightLevel(level), true
		}
	}
	return "", false
}

// lightLevel is level as a light's value: "off" at 0 or below, at most 100.
func lightLevel(level int) string {
	if level <= 0 {
		return "off"
	}
	return strconv.Itoa(min(level, 100))
}

// formatLight writes a light's value as Home Assistant's JSON.
func formatLight(value string) string {
	light := tLightJSON{State: "OFF"}
	if level, err := strconv.Atoi(value); err == nil {
		light.State, light.Brightness = "ON", &level
	}
	data, _ := json.Marshal(light)
	return string(data)
}

// parseOnOff reads "on" or "off", in any case.
func parseOnOff(payload string) (string, bool) {
	switch value := strings.ToLower(strings.TrimSpace(payload)); value {
	case "on", "off":
		return value, true
	}
	return "", false
}

// TEncoding is how a path writes a value, reads what its radio received, and what a value
// becomes on the device (an FS20 dimmer's nearest dim level) -- what an echo is compared with.
type TEncoding struct {
	Encode    func(value string) string
	Decode    func(payload string, onLevel int) (string, bool)
	Normalise func(value string) string
}

// encodings are the paths' ways of writing values.
var encodings = map[string]TEncoding{
	"fs20_switch": {Encode: identity, Decode: func(payload string, _ int) (string, bool) { return decodeFS20Switch(payload) }, Normalise: identity},
	"fs20_dimmer": {Encode: identity, Decode: decodeFS20Dimmer, Normalise: normaliseFS20Dim},
}

// identity returns value.
func identity(value string) string { return value }

// fs20DimLevels are the levels an FS20 dimmer knows (FHEM's dim06% .. dim100%); FHEM's
// configuration (integration_fhem.go) rounds a level to the nearest, "off" below 3.
var fs20DimLevels = []int{6, 12, 18, 25, 31, 37, 43, 50, 56, 62, 68, 75, 81, 87, 93, 100}

// normaliseFS20Dim is the dim level a light's value becomes on an FS20 dimmer.
func normaliseFS20Dim(value string) string {
	level, err := strconv.Atoi(value)
	if err != nil {
		return value
	}
	if level < fs20DimLevels[0]/2 {
		return "off"
	}
	nearest := fs20DimLevels[0]
	for _, l := range fs20DimLevels {
		if abs(l-level) < abs(nearest-level) {
			nearest = l
		}
	}
	return strconv.Itoa(nearest)
}

// abs is |n|.
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// decodeFS20Dimmer reads what FHEM reports for an FS20 dimmer: off, a dim level, or on (onLevel).
// A remote's relative dim up or down says no level, and is not taken.
func decodeFS20Dimmer(payload string, onLevel int) (string, bool) {
	switch payload = strings.TrimSpace(payload); {
	case payload == "off" || payload == "dim00%":
		return "off", true
	case payload == "on" || strings.HasPrefix(payload, "on-"):
		return normaliseFS20Dim(strconv.Itoa(onLevel)), true
	case strings.HasPrefix(payload, "dim") && strings.HasSuffix(payload, "%"):
		if level, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(payload, "dim"), "%")); err == nil {
			return lightLevel(level), true
		}
	}
	return "", false
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

// tSent is a value sent on a path.
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
	sent           [][]tSent // per path, its sends within echoWindow
	lastReceived   string
	lastReceivedAt time.Time
	cost           float64
	onLevel        int
	copiesWithin   []time.Duration // per path
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
		e := &tEntity{def: def, kind: kinds[def.Kind], sent: make([][]tSent, len(def.Paths)), onLevel: def.OnLevel, cost: channel.FrameSeconds}
		if e.onLevel == 0 {
			e.onLevel = 100
		}
		for i, p := range def.Paths {
			within := channel.CopiesWithinMS
			if p.CopiesWithinMS > 0 {
				within = p.CopiesWithinMS
			}
			e.copiesWithin = append(e.copiesWithin, time.Duration(within)*time.Millisecond)
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
	if value, ok := e.kind.Parse(payload, e.onLevel); ok {
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
	value, ok := e.kind.Parse(payload, e.onLevel)
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
		encoding := encodings[e.def.Paths[ref.path].Encoding]
		value, ok := encoding.Decode(payload, e.onLevel)
		if !ok || !g.started {
			continue
		}
		if isEcho(e.sent[ref.path], value, encoding, now) {
			continue
		}
		if e.lastReceived == value && now.Sub(e.lastReceivedAt) < e.copiesWithin[ref.path] {
			continue // the same press, heard by another radio
		}
		e.lastReceived, e.lastReceivedAt = value, now
		if value != encoding.Normalise(e.desired) {
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
		recent := slices.DeleteFunc(e.sent[i], func(s tSent) bool { return now.Sub(s.at) >= echoWindow })
		e.sent[i] = append(recent, tSent{value: value, at: now})
	}
	if !e.pending.enforcement {
		g.logf("%s %s: sent %s", e.def.Device, e.def.Label, value)
	}
	e.pending, e.lastSent = nil, now
}

// isEcho tells whether value, received now, is the echo of one of sent.
func isEcho(sent []tSent, value string, encoding TEncoding, now time.Time) bool {
	for _, s := range sent {
		if encoding.Normalise(s.value) == value && now.Sub(s.at) < echoWindow {
			return true
		}
	}
	return false
}

// publishState publishes e's desired state, retained.
func (g *TGovernor) publishState(e *tEntity) {
	g.publisher.Publish(e.def.Topic, e.kind.FormatState(e.desired), true)
}
