/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: DiscoveryPassthroughDevices
 *
 * The missing half of PROJECT.md 1.8's suggestion mechanism for PROJECT.md item 7's passthrough
 * relay (discoverybridge.go): discovery_existence.go's own suggestion report only ever covers
 * undeclared LEAVES of an ALREADY-declared "discovery" gateway (a gatewayID is required just to
 * key its tracking), so a device passthrough is relaying byte-for-byte -- one no declared gateway
 * claims at all yet -- was entirely invisible to the operator, defeating a good chunk of the
 * point of a *gradual* migration (found live 2026-09-14: real Zigbee2MQTT traffic was flowing
 * through passthrough but never showed up in suggestions/discovery.txt).
 *
 * Keyed by the device's own real identifier (the raw payload's device.identifiers, not a
 * coordinator-invented slug -- Zigbee2MQTT already provides a genuinely stable one per device), so
 * the generator's suggestion report can emit a ready-to-paste "device discovery.<slug> with:
 * identifiers "<real-id>"; ...; end;" block per still-undeclared device. A device is forgotten the
 * moment it starts matching a declared gateway (discoverybridge.go's own passthrough branch calls
 * Forget alongside its existing raw-topic retirement), so a migrated device stops being suggested
 * for migration it has already undergone.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 14.09.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// TPassthroughLeaf is one observed capability under an undeclared passthrough device -- domain is
// the real HA component, taken directly from the discovery topic (unlike discovery_existence.go's
// own leaf tracking, passthrough always knows the true domain, never needs to guess it).
type TPassthroughLeaf struct {
	Domain string `json:"domain"`
	Leaf   string `json:"leaf"` // the gateway's own unique_id
}

// tPassthroughDeviceEntry is one undeclared device's own tracked state.
type tPassthroughDeviceEntry struct {
	Name   string                      `json:"name"` // the device's own human-readable name, if reported
	Leaves map[string]TPassthroughLeaf `json:"leaves"`
}

// TPassthroughNameCollision records that two different devices' passthrough-relayed discovery
// payloads both claim the same "default_entity_id" -- see ClaimName's own doc comment for how this
// arises and why it matters. ClaimedBy keeps its relay going; BlockedID's relay is suppressed.
type TPassthroughNameCollision struct {
	ClaimedBy string `json:"claimed_by"`
	BlockedID string `json:"blocked_id"`
}

// TPassthroughNameClaim is one default_entity_id's current owner -- Consistent records whether
// THAT claim's own default_entity_id looked self-consistent with its own state_topic at the time
// it was made (discoverybridge.go's discoveryNameLooksConsistent), so a later, more consistent
// claimant can displace a stale one regardless of arrival order -- see ClaimName's own doc comment.
// Topic is the exact conceptual-prefix topic THIS claim was actually relayed under -- needed so a
// displacement can retire it (a different device's passthrough topic is a different string
// entirely, not derivable from defaultEntityID/deviceIdentifier alone).
type TPassthroughNameClaim struct {
	Owner      string `json:"owner"`
	Consistent bool   `json:"consistent"`
	Topic      string `json:"topic"`
}

// TPassthroughDeviceTracker tracks every device discoverybridge.go's passthrough relay has
// observed but that no declared "discovery" gateway claims yet, plus (2026-09-20) which
// default_entity_id each relayed device has claimed, so a same-name clash between two different
// devices can be caught and flagged rather than silently relayed twice.
type TPassthroughDeviceTracker struct {
	mu         sync.Mutex
	path       string // "" disables persistence entirely (tests)
	devices    map[string]*tPassthroughDeviceEntry
	claims     map[string]TPassthroughNameClaim     // default_entity_id -> current owner
	collisions map[string]TPassthroughNameCollision // default_entity_id -> the clash, if any
	// topicDevice remembers which deviceIdentifier a passthrough discovery config topic currently
	// belongs to, so a later empty (retracted) payload on that same topic -- Zigbee2MQTT's own
	// device-removal convention -- can be resolved back to the device to forget. Mirrors
	// TDiscoveryExistenceTracker.topicIdentity exactly, and for the same reason: an empty payload
	// carries no device identifier of its own to decode. Deliberately in-memory only, never
	// persisted -- discovery config topics are retained, so a coordinator restart's own subscribe
	// naturally replays every still-live device's current (non-empty) config before any new
	// retraction could arrive, making this self-healing without needing to survive a restart on
	// disk (same reasoning as topicIdentity's own doc comment).
	topicDevice   map[string]string
	debounceTimer *time.Timer // see ScheduleStatusPublish's own doc comment
	persistTimer  *time.Timer // see SchedulePersist's own doc comment
}

type passthroughDevicesPersistedFile struct {
	Devices    map[string]*tPassthroughDeviceEntry  `json:"devices"`
	Claims     map[string]TPassthroughNameClaim     `json:"claims"`
	Collisions map[string]TPassthroughNameCollision `json:"collisions"`
}

// newPassthroughDeviceTracker loads path's previously persisted state, if any -- mirrors
// newDiscoveryExistenceTracker exactly (never fails, missing/incompatible falls back to empty).
func newPassthroughDeviceTracker(path string) *TPassthroughDeviceTracker {
	t := &TPassthroughDeviceTracker{
		path:        path,
		devices:     map[string]*tPassthroughDeviceEntry{},
		claims:      map[string]TPassthroughNameClaim{},
		collisions:  map[string]TPassthroughNameCollision{},
		topicDevice: map[string]string{},
	}
	t.loadPersisted()
	return t
}

func (t *TPassthroughDeviceTracker) loadPersisted() {
	if t.path == "" {
		return
	}
	data, err := os.ReadFile(t.path)
	if err != nil {
		return
	}
	var persisted passthroughDevicesPersistedFile
	if err := json.Unmarshal(data, &persisted); err != nil {
		return
	}
	if persisted.Devices != nil {
		t.devices = persisted.Devices
	}
	if persisted.Claims != nil {
		t.claims = persisted.Claims
	}
	if persisted.Collisions != nil {
		t.collisions = persisted.Collisions
	}
}

func (t *TPassthroughDeviceTracker) persist() {
	if t.path == "" {
		return
	}
	data, err := json.MarshalIndent(passthroughDevicesPersistedFile{Devices: t.devices, Claims: t.claims, Collisions: t.collisions}, "", "  ")
	if err != nil {
		fmt.Printf("[discovery-passthrough] marshalling persisted state: %v\n", err)
		return
	}
	if err := os.WriteFile(t.path, data, 0o644); err != nil {
		fmt.Printf("[discovery-passthrough] persisting state to %s: %v\n", t.path, err)
	}
}

// ClaimName records that deviceIdentifier's relayed discovery payload claims defaultEntityID (the
// raw payload's own "default_entity_id" field) as its suggested HA entity id -- the exact field
// Home Assistant uses to auto-assign an entity_id the first time it sees a discovery config.
// Passthrough relays a not-yet-declared device's payload byte-for-byte (discoverybridge.go), so
// two different physical devices whose payloads happen to claim the SAME default_entity_id would
// otherwise both get relayed into HA, which silently disambiguates by suffixing one of them --
// exactly the kind of "Entity not found" surprise a stable migration is supposed to prevent. Real
// case found live 2026-09-20: Junglinster's "xanadu" smart plug was renamed in Zigbee2MQTT to
// "storage_room/disks_1", but its OLD retained discovery payload (still claiming
// "switch.house/server_room/xanadu" as its default_entity_id) never got refreshed, colliding with
// a DIFFERENT physical plug that has since taken over the "xanadu" name.
//
// consistent is discoverybridge.go's own discoveryNameLooksConsistent(payload.DefaultEntityID,
// payload.StateTopic) for THIS message -- whether its own claimed name actually matches its own
// current state_topic, rather than being a stale leftover from before a rename. A consistent
// claimant always displaces an inconsistent current owner, regardless of which arrived first --
// this is what makes ownership immune to retained-backlog REPLAY ORDER. Real incident, found live
// minutes after this mechanism's own first deploy (2026-09-20): plain first-claim-wins let the
// STALE device win the race on that particular coordinator restart, which then suppressed the
// CURRENTLY CORRECT device's relay instead of the stale one's -- exactly backwards. Two claims of
// equal consistency (both true or both false) keep the original first-come-wins behavior, since
// there's no signal to prefer one over the other.
//
// Every later message from a DIFFERENT, less-or-equally-consistent device is recorded as a
// collision (surfaced in suggestions/discovery.txt, see buildPassthroughCollisionReport) rather
// than relayed. Returns ok=true (caller should relay) when defaultEntityID is unclaimed, already
// claimed by THIS SAME device (the overwhelmingly common case, including every retained-backlog
// replay), this claim displaces a less consistent one, or either argument is empty. Returns
// ok=false (caller must NOT relay this message) otherwise.
//
// evictedTopic is non-empty exactly when a displacement just happened AND the displaced owner had
// already been relayed under a different topic than this claim's own -- the caller must retire
// that topic so the now-displaced device's stale entity doesn't linger in HA alongside the new
// owner's (passthrough relays two different physical devices under two different raw topics even
// when they collide on the same suggested name, so simply switching ownership here doesn't by
// itself un-publish what was already sent).
func (t *TPassthroughDeviceTracker) ClaimName(defaultEntityID, deviceIdentifier, topic string, consistent bool) (ok bool, evictedTopic string) {
	if defaultEntityID == "" || deviceIdentifier == "" {
		return true, ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	// Deliberately does NOT persist() on the two paths below (a first-ever claim, or the same
	// device merely reaffirming its own name) -- real incident, found live MINUTES after this
	// mechanism's first deploy (2026-09-20): claims are established for nearly every one of
	// Junglinster's ~983 passthrough devices during a single retained-backlog replay at startup,
	// and a synchronous disk write (persist marshals the WHOLE devices+claims+collisions state)
	// on every single one of those, stacked on top of Record's own already-substantial persist
	// cadence, pushed startup well past the coordinator's systemd watchdog timeout -- a genuine
	// crash loop, never reaching steady state. Ordinary claims need not survive a restart for
	// correctness anyway: discoveryNameLooksConsistent is recomputed fresh from each LIVE payload
	// on every replay, so ownership re-derives itself correctly from scratch every time regardless
	// of what (if anything) was persisted. Persisting stays on the two genuinely rare, interesting
	// paths below (an actual collision, or a displacement) -- both orders of magnitude less
	// frequent, and the only ones suggestions/discovery.txt actually needs to survive a restart.
	claim, claimed := t.claims[defaultEntityID]
	if !claimed {
		t.claims[defaultEntityID] = TPassthroughNameClaim{Owner: deviceIdentifier, Consistent: consistent, Topic: topic}
		return true, ""
	}
	if claim.Owner == deviceIdentifier {
		if claim.Consistent != consistent || claim.Topic != topic {
			t.claims[defaultEntityID] = TPassthroughNameClaim{Owner: deviceIdentifier, Consistent: consistent, Topic: topic}
		}
		return true, ""
	}
	if consistent && !claim.Consistent {
		t.claims[defaultEntityID] = TPassthroughNameClaim{Owner: deviceIdentifier, Consistent: true, Topic: topic}
		delete(t.collisions, defaultEntityID)
		t.persist()
		if claim.Topic != "" && claim.Topic != topic {
			return true, claim.Topic
		}
		return true, ""
	}
	if existing, seen := t.collisions[defaultEntityID]; !seen || existing.BlockedID != deviceIdentifier {
		t.collisions[defaultEntityID] = TPassthroughNameCollision{ClaimedBy: claim.Owner, BlockedID: deviceIdentifier}
		t.persist()
	}
	return false, ""
}

// Record notes deviceIdentifier (the raw device's own first identifier) has leaf (component +
// its own unique_id) -- called for every passthrough-relayed, non-empty message. A "" identifier
// or leaf is a no-op: nothing stable to key on. name, when non-empty, updates the device's own
// last-known human-readable name. Returns true the first time this exact fact is recorded, so
// callers can skip a redundant publish on every retained-message replay at startup.
//
// Deliberately does NOT persist synchronously -- real incident, 2026-09-22 (Junglinster): this
// used to call t.persist() directly here, exactly the same disease ClaimName's own doc comment
// (above) already diagnosed and fixed for ITS persist calls on 2026-09-20 (a synchronous disk
// write, marshalling the WHOLE growing devices+claims+collisions state, on nearly every one of
// Junglinster's ~983 passthrough devices during a single retained-backlog replay at startup) --
// Record's own call site just wasn't covered by that fix. Callers debounce the actual write via
// SchedulePersist instead (discoverybridge.go already does, right alongside its existing
// ScheduleStatusPublish call for the same Record result).
func (t *TPassthroughDeviceTracker) Record(deviceIdentifier, name, domain, leaf string) bool {
	if deviceIdentifier == "" || leaf == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	entry, ok := t.devices[deviceIdentifier]
	if !ok {
		entry = &tPassthroughDeviceEntry{Leaves: map[string]TPassthroughLeaf{}}
		t.devices[deviceIdentifier] = entry
	}
	changed := false
	if name != "" && entry.Name != name {
		entry.Name = name
		changed = true
	}
	if existing, seen := entry.Leaves[leaf]; !seen || existing.Domain != domain {
		entry.Leaves[leaf] = TPassthroughLeaf{Domain: domain, Leaf: leaf}
		changed = true
	}
	return changed
}

// Forget removes deviceIdentifier entirely -- called the moment a device starts matching a
// declared gateway, so a migrated device is never suggested again. Returns false (a no-op) when
// deviceIdentifier was never tracked to begin with -- the overwhelmingly common case for ordinary
// declared-gateway traffic unrelated to passthrough at all -- so callers can skip a redundant
// status publish.
func (t *TPassthroughDeviceTracker) Forget(deviceIdentifier string) bool {
	if deviceIdentifier == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.devices[deviceIdentifier]; !ok {
		return false
	}
	delete(t.devices, deviceIdentifier)
	// Release any name claim/collision this device held -- once declared, it's positioned
	// independently of passthrough naming entirely, and a stale claim here would otherwise block
	// any OTHER still-undeclared device from ever claiming that name, forever.
	for name, claim := range t.claims {
		if claim.Owner == deviceIdentifier {
			delete(t.claims, name)
		}
	}
	for name, collision := range t.collisions {
		if collision.ClaimedBy == deviceIdentifier || collision.BlockedID == deviceIdentifier {
			delete(t.collisions, name)
		}
	}
	// Also release every topic->identity mapping this device held (RecordTopicIdentity) -- a stale
	// entry here is harmless (ForgetByTopic on it later would just no-op, the device is already
	// gone), but leaving it around forever would grow this map unboundedly across a coordinator's
	// whole lifetime as devices get migrated or removed.
	for topic, owner := range t.topicDevice {
		if owner == deviceIdentifier {
			delete(t.topicDevice, topic)
		}
	}
	t.persist()
	return true
}

// RecordTopicIdentity remembers that topic currently belongs to deviceIdentifier -- called on every
// successfully-decoded, non-empty passthrough discovery payload from a still-undeclared device,
// before it's known whether that device is newly-observed or already-tracked, so a later retraction
// (an empty payload) on the same topic always has an identity to resolve against. Mirrors
// TDiscoveryExistenceTracker.RecordTopicIdentity exactly.
func (t *TPassthroughDeviceTracker) RecordTopicIdentity(topic, deviceIdentifier string) {
	if topic == "" || deviceIdentifier == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.topicDevice[topic] = deviceIdentifier
}

// ForgetByTopic resolves topic's last-recorded device identity (RecordTopicIdentity) and forgets
// that device entirely (Forget) -- called when an empty (retracted) payload arrives on a topic this
// tracker has seen a real payload on before.
//
// Real gap found live 2026-09-21 (Vienna): a device deleted outright from Zigbee2MQTT (not renamed
// or migrated into a real Physical.def declaration, genuinely removed from the network) publishes
// empty payloads on all its own discovery config topics -- Zigbee2MQTT's own device-removal
// convention, exactly what discoverybridge.go's empty-payload branch already handles for a
// DECLARED gateway via TDiscoveryExistenceTracker.MarkRetracted. But Forget was only ever called
// from the OTHER branch, when a still-undeclared device starts matching a declared gateway (i.e.
// gets properly migrated) -- nothing ever removed a deleted-but-never-migrated device's own
// passthrough suggestion entry, leaving it suggested in suggestions/discovery.txt forever (the
// user's own report: "discovery.apartment_living_room_rack_aqara_multi" stayed suggested well
// after the underlying Zigbee device was deleted from the gateway).
//
// deviceIdentifier is returned even when ok is false only in the sense that "" is returned too --
// ok is false when topic's identity was never recorded (nothing to forget: this topic never
// belonged to a passthrough-tracked device, e.g. it's a declared gateway's own topic instead) or the
// device was already forgotten (e.g. a second config topic for the same already-removed device).
func (t *TPassthroughDeviceTracker) ForgetByTopic(topic string) (deviceIdentifier string, ok bool) {
	t.mu.Lock()
	deviceIdentifier, found := t.topicDevice[topic]
	t.mu.Unlock()
	if !found {
		return "", false
	}
	return deviceIdentifier, t.Forget(deviceIdentifier)
}

// snapshot returns a deep-enough copy of the current device set for publishing.
func (t *TPassthroughDeviceTracker) snapshot() map[string]tPassthroughDeviceEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]tPassthroughDeviceEntry, len(t.devices))
	for id, entry := range t.devices {
		leaves := make(map[string]TPassthroughLeaf, len(entry.Leaves))
		for k, v := range entry.Leaves {
			leaves[k] = v
		}
		out[id] = tPassthroughDeviceEntry{Name: entry.Name, Leaves: leaves}
	}
	return out
}

// collisionsSnapshot returns a copy of the current name-collision set for publishing.
func (t *TPassthroughDeviceTracker) collisionsSnapshot() map[string]TPassthroughNameCollision {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]TPassthroughNameCollision, len(t.collisions))
	for name, collision := range t.collisions {
		out[name] = collision
	}
	return out
}

// passthroughDevicesStatusTopic is the retained topic this tracker's full snapshot is published
// to -- homeassistant/discovery_passthrough_suggestions.go's own copy of this same literal string
// is what the generator subscribes to at generate time (mirrors discoveryExistenceStatusTopic's
// own cross-package duplication -- the two packages never share Go types).
const passthroughDevicesStatusTopic = "discovery_passthrough/devices/state"

// tPassthroughStatusSnapshot is the full retained payload on passthroughDevicesStatusTopic --
// wraps both undeclared-device tracking and same-name collision tracking so the generator only
// needs to subscribe to one topic for both (homeassistant/discovery_passthrough_suggestions.go
// keeps its own mirrored copy, same cross-package convention as tPassthroughDeviceEntry/
// tPassthroughDeviceSuggestion).
type tPassthroughStatusSnapshot struct {
	Devices    map[string]tPassthroughDeviceEntry   `json:"devices"`
	Collisions map[string]TPassthroughNameCollision `json:"collisions"`
}

// PublishStatus publishes the tracker's current full device set and name-collision set, retained,
// to mainClient and -- when cloudClient is non-nil -- also to cloudClient, prefixed with
// ownInstallation (same collision-avoidance reasoning as discovery_existence.go's own
// publishStatus -- unrelated to the entity-naming collisions tracked here).
func (t *TPassthroughDeviceTracker) PublishStatus(mainClient, cloudClient mqtt.Client, ownInstallation string) error {
	data, err := json.Marshal(tPassthroughStatusSnapshot{Devices: t.snapshot(), Collisions: t.collisionsSnapshot()})
	if err != nil {
		return fmt.Errorf("marshalling passthrough device status: %w", err)
	}
	if err := publishRetained(mainClient, passthroughDevicesStatusTopic, data); err != nil {
		return fmt.Errorf("publishing %s (local): %w", passthroughDevicesStatusTopic, err)
	}
	if cloudClient != nil {
		cloudTopic := ownInstallation + "/" + passthroughDevicesStatusTopic
		if err := publishRetained(cloudClient, cloudTopic, data); err != nil {
			return fmt.Errorf("publishing %s (cloud): %w", cloudTopic, err)
		}
	}
	return nil
}

// PassthroughStatusDebounceDelay is production's own delay argument for ScheduleStatusPublish --
// how long it waits after the LAST call before actually publishing. Long enough to coalesce an
// entire startup retained-backlog replay burst (the 2026-09-14 incident's exact shape: hundreds of
// "first time seen" Record calls in a tight loop, discoverybridge.go) into a single publish once
// the burst quiets down, short enough that a genuine new arrival during ordinary operation still
// surfaces to suggestions/discovery.txt well within the same session rather than needing a
// coordinator restart/reconnect to notice it (found live 2026-09-19: Junglinster's own Z-Wave
// devices sat untracked-in-suggestions for over a day of normal operation because Record's own
// "changed" return value was simply discarded at its call site, exactly the "callers can skip a
// redundant publish" case that return value's own doc comment anticipated but PublishStatus was
// never safely wired up to actually use). An explicit call argument rather than a package-level
// const, deliberately -- same reasoning as newDiscoveryRelayQueue's own throttle parameter
// (discovery_relay_queue.go): lets a test exercise the real debounce/coalesce logic on a short
// delay instead of either hardcoding a slow real-time wait into every test or bypassing the
// timing logic entirely.
const PassthroughStatusDebounceDelay = 30 * time.Second

// PassthroughStatusPeriodicInterval is production's own interval argument for
// StartPeriodicStatusPublish -- the baseline safety-net cadence, catching anything the debounce
// above might miss (in particular a leaf-only update on an ALREADY-known device, which Record
// still reports as "changed" but this component's own "new arrival" framing doesn't need to react
// to instantly) without ever needing a coordinator restart/reconnect. Deliberately low-frequency,
// matching this whole mechanism's own established "a generate-time suggestion feed, nothing live
// depends on it" tolerance for staleness. Also an explicit call argument, same reasoning as above.
const PassthroughStatusPeriodicInterval = 10 * time.Minute

// SchedulePersist debounces a persist() call by delay -- same coalescing shape as
// ScheduleStatusPublish (its own doc comment covers the general reasoning), a genuinely separate
// timer since persisting to disk and publishing the MQTT status are independent concerns Record's
// own call site schedules side by side, not sequentially. Safe to call directly from the MQTT
// message-dispatch callback: the actual write always happens later, on this timer's own goroutine.
func (t *TPassthroughDeviceTracker) SchedulePersist(delay time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.persistTimer != nil {
		t.persistTimer.Reset(delay)
		return
	}
	t.persistTimer = time.AfterFunc(delay, func() {
		t.mu.Lock()
		t.persistTimer = nil
		t.persist()
		t.mu.Unlock()
	})
}

// ScheduleStatusPublish debounces a full PublishStatus call by delay -- see
// PassthroughStatusDebounceDelay's own doc comment for why a plain per-call publish isn't safe
// here, and for that production delay's own value. Safe to call directly from the MQTT
// message-dispatch callback (discoverybridge.go): the actual publish always happens later, on this
// timer's own goroutine, never synchronously on the caller's. Repeated calls within delay of each
// other keep resetting the same pending timer rather than scheduling additional ones, so an
// arbitrarily long burst still only ever produces one publish, shortly after the burst's own last
// event -- no separate max-wait cap is needed on top of that, since StartPeriodicStatusPublish's
// own independent ticker is the backstop for the (implausible, but not impossible) case of a burst
// so continuous it never actually quiets down.
func (t *TPassthroughDeviceTracker) ScheduleStatusPublish(mainClient, cloudClient mqtt.Client, ownInstallation string, delay time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.debounceTimer != nil {
		t.debounceTimer.Reset(delay)
		return
	}
	t.debounceTimer = time.AfterFunc(delay, func() {
		t.mu.Lock()
		t.debounceTimer = nil
		t.mu.Unlock()
		if err := t.PublishStatus(mainClient, cloudClient, ownInstallation); err != nil {
			fmt.Printf("[discovery-passthrough] debounced status publish: %v\n", err)
		}
	})
}

// StartPeriodicStatusPublish starts a background goroutine publishing t's full status on a fixed
// interval cadence, for the lifetime of the process (never stopped, matching every other
// background worker in this codebase, e.g. newDiscoveryRelayQueue's own). Purely a safety net
// alongside ScheduleStatusPublish -- see that pair's own doc comments.
func (t *TPassthroughDeviceTracker) StartPeriodicStatusPublish(mainClient, cloudClient mqtt.Client, ownInstallation string, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			if err := t.PublishStatus(mainClient, cloudClient, ownInstallation); err != nil {
				fmt.Printf("[discovery-passthrough] periodic status publish: %v\n", err)
			}
		}
	}()
}
