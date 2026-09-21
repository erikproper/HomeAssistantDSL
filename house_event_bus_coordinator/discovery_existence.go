/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: DiscoveryExistence
 *
 * Kind-2 (discovery) half of PROJECT.md 1.8 (memory: project_entity_existence_inquiry_design) --
 * the passive counterpart to entity_existence.go's kind-3 mechanism. A gateway (Zigbee2MQTT,
 * Z-Wave, EMS-ESP) self-announces via its own native HA MQTT discovery, so no active inquiry is
 * needed the way kind-3 needs it (a remote HA instance never tells the coordinator anything
 * unprompted) -- but the coordinator still needs to track and report each declared source leaf's
 * three-state status to the generator, exactly as it already does for kind-3, just populated
 * passively as a byproduct of discoverybridge.go's existing discovery-payload subscription instead
 * of by asking. Reuses entity_existence.go's own TEntityExistenceStatus/Status* constants (same
 * package, same three values) rather than redefining them.
 *
 * Retraction (known-not-to-exist) is deliberately not built yet -- an empty retained discovery
 * payload carries no unique_id of its own to decode, so recognising which (gatewayID, leaf) a
 * retraction topic belongs to needs a topic->identity map this file doesn't keep. Every tracked
 * leaf starts not-known-to-exist and can only ever move to known-to-exist today.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 29.08.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// TDiscoveryExistenceTracker holds, per gateway, every declared/observed source leaf's current
// status.
type TDiscoveryExistenceTracker struct {
	mu      sync.Mutex
	path    string                                       // "" disables persistence entirely (tests)
	entries map[string]map[string]TEntityExistenceStatus // gatewayID -> leaf -> status

	// topicIdentity remembers which (gatewayID, leaf) a discovery config topic currently belongs
	// to, so a later retraction (an empty payload on that same topic -- HA's own MQTT discovery
	// removal convention) can be attributed back to the right entry -- RecordTopicIdentity/
	// MarkRetracted. Deliberately in-memory only, never persisted: discovery config topics are
	// retained, so a coordinator restart's own subscribe naturally replays every gateway's current
	// (non-retracted) config before any new retraction could arrive, making this self-healing
	// without needing to survive a restart on disk.
	topicIdentity map[string]discoveryTopicIdentity

	debounceTimer *time.Timer // see ScheduleAggregateStatusPublish's own doc comment

	// onChange (2026-09-21, PROJECT.md item 1a) fires, unconditionally with no arguments, whenever
	// any leaf transitions into or out of StatusKnownNotToExist -- lets main.go wire up
	// TMissingDeclaredEntitiesPublisher.Schedule without this tracker needing to know anything
	// about that publisher itself. Deliberately narrower than
	// ScheduleAggregateStatusPublish's own debounce (which fires on every MarkKnown, even an
	// ordinary first-sighting): a startup replay burst would otherwise schedule a missing-entities
	// republish for every single leaf, most of which never touch StatusKnownNotToExist at all.
	// Mirrors TLiveDeviceInfoStore.onChange's own locking discipline (liveinfo.go) -- read/cleared
	// while locked, always invoked after an explicit unlock, never under the mutex.
	onChange func()
}

// SetOnChange (re)sets the tracker's onChange callback -- see that field's own doc comment.
func (t *TDiscoveryExistenceTracker) SetOnChange(onChange func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onChange = onChange
}

// KnownNotToExistItems returns a sorted "<gatewayID>/<leaf>" snapshot of every leaf currently
// StatusKnownNotToExist -- feeds TMissingDeclaredEntitiesPublisher's own live aggregate indicator.
func (t *TDiscoveryExistenceTracker) KnownNotToExistItems() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	var items []string
	for gatewayID, leaves := range t.entries {
		for leaf, status := range leaves {
			if status == StatusKnownNotToExist {
				items = append(items, gatewayID+"/"+leaf)
			}
		}
	}
	sort.Strings(items)
	return items
}

// discoveryTopicIdentity is one MQTT discovery config topic's currently-known (gatewayID, leaf).
type discoveryTopicIdentity struct {
	gatewayID string
	leaf      string
}

// newDiscoveryExistenceTracker loads path's previously persisted state, if any -- mirrors
// entity_existence.go's newEntityExistenceTracker exactly (never fails, missing/incompatible falls
// back to empty; path == "" disables persistence outright).
func newDiscoveryExistenceTracker(path string) *TDiscoveryExistenceTracker {
	t := &TDiscoveryExistenceTracker{
		path:          path,
		entries:       map[string]map[string]TEntityExistenceStatus{},
		topicIdentity: map[string]discoveryTopicIdentity{},
	}
	t.loadPersisted()
	return t
}

// discoveryExistencePersistedFile is discovery_existence.json's own on-disk shape.
type discoveryExistencePersistedFile struct {
	Gateways map[string]map[string]TEntityExistenceStatus `json:"gateways"`
}

func (t *TDiscoveryExistenceTracker) loadPersisted() {
	if t.path == "" {
		return
	}
	data, err := os.ReadFile(t.path)
	if err != nil {
		return
	}
	var persisted discoveryExistencePersistedFile
	if err := json.Unmarshal(data, &persisted); err != nil {
		return
	}
	for gatewayID, leaves := range persisted.Gateways {
		t.entries[gatewayID] = leaves
	}
}

func (t *TDiscoveryExistenceTracker) persist() {
	if t.path == "" {
		return
	}
	data, err := json.MarshalIndent(discoveryExistencePersistedFile{Gateways: t.entries}, "", "  ")
	if err != nil {
		fmt.Printf("[discovery-existence] marshalling persisted state: %v\n", err)
		return
	}
	if err := os.WriteFile(t.path, data, 0o644); err != nil {
		fmt.Printf("[discovery-existence] persisting state to %s: %v\n", t.path, err)
	}
}

// Seed registers every discovery.yaml EntityLink's own (gateway, leaf) as not-known-to-exist -- the
// generator's own "assumed to exist" list for kind-2, exactly like entity_existence.go's Seed reads
// homeassistant_bridge.yaml for kind-3. Already-tracked (gateway, leaf) pairs are left untouched
// (status preserved as-is), so a coordinator restart never resets an already-observed leaf back to
// unknown.
func (t *TDiscoveryExistenceTracker) Seed(discoveryFile TDiscoveryFile) {
	t.mu.Lock()
	defer t.mu.Unlock()
	declared := map[string]map[string]bool{} // gatewayID -> leaf -> still declared this round
	for _, link := range discoveryFile.EntityLinks {
		if link.Gateway == "" || link.Leaf == "" {
			continue
		}
		if _, ok := t.entries[link.Gateway]; !ok {
			t.entries[link.Gateway] = map[string]TEntityExistenceStatus{}
		}
		if declared[link.Gateway] == nil {
			declared[link.Gateway] = map[string]bool{}
		}
		declared[link.Gateway][link.Leaf] = true
		if _, seen := t.entries[link.Gateway][link.Leaf]; seen {
			continue
		}
		t.entries[link.Gateway][link.Leaf] = StatusNotKnownToExist
	}

	// Prune confirmed-dead ghost leaves no longer declared anywhere -- mirrors
	// entity_existence.go's own Seed pruning (2026-09-05) exactly, same reasoning and same
	// safety property: a retracted-and-no-longer-declared leaf would otherwise sit here forever
	// (MarkRetracted only ever flips status, never removes the entry). Scoped identically tightly
	// to known-not-to-exist only -- a not-yet-resolved or known-to-exist leaf is never touched,
	// so a still-declared leaf keeps being flagged by checkDiscoveryKnownNotToExistErrors for as
	// long as the DSL genuinely still claims it should exist.
	for gatewayID, leaves := range t.entries {
		for leaf, status := range leaves {
			if status != StatusKnownNotToExist || declared[gatewayID][leaf] {
				continue
			}
			delete(t.entries[gatewayID], leaf)
			fmt.Printf("[discovery-existence] %s: pruned %q -- confirmed not-to-exist and no longer declared\n", gatewayID, leaf)
		}
	}

	t.persist()
}

// MarkKnown records leaf as known-to-exist on gatewayID -- called by discoverybridge.go's existing
// discovery-payload subscription handler the moment any payload for that leaf arrives, whether or
// not a devices.yaml EntityLink already claims it (a leaf can genuinely exist on a gateway before
// the DSL ever positions it). Returns false (a no-op past the first observation) if leaf is already
// known-to-exist on gatewayID, so callers can skip a redundant publish on every retained-message
// replay at startup.
func (t *TDiscoveryExistenceTracker) MarkKnown(gatewayID, leaf string) bool {
	t.mu.Lock()
	if _, ok := t.entries[gatewayID]; !ok {
		t.entries[gatewayID] = map[string]TEntityExistenceStatus{}
	}
	if t.entries[gatewayID][leaf] == StatusKnownToExist {
		t.mu.Unlock()
		return false
	}
	wasMissing := t.entries[gatewayID][leaf] == StatusKnownNotToExist
	t.entries[gatewayID][leaf] = StatusKnownToExist
	t.persist()
	onChange := t.onChange
	t.mu.Unlock()
	// Only a genuine "was confirmed gone, now resolved" transition needs to fire onChange -- an
	// ordinary first-sighting (StatusNotKnownToExist -> StatusKnownToExist) never touched
	// StatusKnownNotToExist at all, so TMissingDeclaredEntitiesPublisher's own aggregate has
	// nothing to update.
	if wasMissing && onChange != nil {
		onChange()
	}
	return true
}

// RecordTopicIdentity remembers that topic currently belongs to (gatewayID, leaf) -- called on
// every successfully-decoded, non-empty discovery payload, before it's known whether that leaf is
// newly-observed or already-tracked, so a later retraction on the same topic always has an identity
// to resolve against.
func (t *TDiscoveryExistenceTracker) RecordTopicIdentity(topic, gatewayID, leaf string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.topicIdentity[topic] = discoveryTopicIdentity{gatewayID: gatewayID, leaf: leaf}
}

// MarkRetracted records the (gatewayID, leaf) topic was last associated with (RecordTopicIdentity)
// as known-not-to-exist -- called when an empty/retracted payload arrives on topic. ok is false if
// topic's identity was never recorded (nothing to retract -- e.g. a topic already empty before this
// coordinator process ever saw a real payload on it) or the leaf was already known-not-to-exist (a
// no-op repeat), true otherwise.
func (t *TDiscoveryExistenceTracker) MarkRetracted(topic string) (gatewayID, leaf string, ok bool) {
	t.mu.Lock()
	identity, found := t.topicIdentity[topic]
	if !found {
		t.mu.Unlock()
		return "", "", false
	}
	if _, exists := t.entries[identity.gatewayID]; !exists {
		t.entries[identity.gatewayID] = map[string]TEntityExistenceStatus{}
	}
	if t.entries[identity.gatewayID][identity.leaf] == StatusKnownNotToExist {
		t.mu.Unlock()
		return identity.gatewayID, identity.leaf, false
	}
	t.entries[identity.gatewayID][identity.leaf] = StatusKnownNotToExist
	t.persist()
	onChange := t.onChange
	t.mu.Unlock()
	if onChange != nil {
		onChange()
	}
	return identity.gatewayID, identity.leaf, true
}

// PublishAll immediately (re-)publishes the full aggregate existence status -- called once right
// after Seed, mirroring entity_existence.go's own StartEntityExistenceInquiries immediate-publish
// fix (2026-09-05, see its own doc comment for the full reasoning): unlike kind-3, kind-2 has no
// periodic inquiry loop to eventually self-heal a stale/missing retained topic -- the debounced
// publish MarkKnown/MarkRetracted schedule only ever fires on an actual known/retracted transition,
// which a gateway with entirely stable, already-known leaves may not produce again for a very long
// time. Without this, a coordinator restart (or a topic-naming migration, or a device rename --
// exactly the gap found live 2026-09-20: Vienna's own device-id rename left every renamed gateway's
// existence status unpublished under its new name until the next real transition) leaves the
// retained topic missing or stale, and every ./generate run's fetch times out waiting for it in the
// meantime.
func (t *TDiscoveryExistenceTracker) PublishAll(mainClient, cloudClient mqtt.Client, ownInstallation string) {
	if err := t.PublishAggregateStatus(mainClient, cloudClient, ownInstallation); err != nil {
		fmt.Printf("[discovery-existence] %v\n", err)
	}
}

// AggregateSnapshot returns a deep copy of every gateway's own current status map -- the exact
// shape PublishAggregateStatus publishes, and the exact shape the generator's own aggregate fetch
// (mqtt_discovery_existence.go) expects back.
func (t *TDiscoveryExistenceTracker) AggregateSnapshot() map[string]map[string]TEntityExistenceStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]map[string]TEntityExistenceStatus, len(t.entries))
	for gatewayID, leaves := range t.entries {
		leafCopy := make(map[string]TEntityExistenceStatus, len(leaves))
		for leaf, status := range leaves {
			leafCopy[leaf] = status
		}
		out[gatewayID] = leafCopy
	}
	return out
}

// discoveryExistenceAggregateStatusTopic is the ONE retained topic every declared discovery
// gateway's own existence status is published to together.
//
// Real incident, found live 2026-09-20: the earlier design published one topic PER gateway
// ("discovery_gateways/<gatewayID>/existence/state"). At Vienna's current scale (70+ declared
// gateways) that meant 70+ separate MQTT round trips on every single ./generate run -- and most of
// those round trips paid a FULL connect-timeout each, for gateways the coordinator simply hadn't
// reported on yet (e.g. right after a device-id rename, every renamed gateway's old topic goes
// stale and the new one doesn't exist until the coordinator itself republishes). A user-facing
// generate that should take seconds instead took over ten minutes. This single combined payload
// stays trivially small even at 10x today's scale -- roughly 30-35KB for Vienna's current ~350
// (gateway, leaf) pairs, well under 150KB even for a much larger installation -- MQTT routinely
// carries multi-MB payloads without strain, so a retained message this size is a non-issue for any
// broker, including a Pi-hosted Mosquitto instance. If this ever genuinely became a real problem,
// the documented fallback is sharding by a hash of gatewayID across a handful of numbered topics,
// with the generator collecting every fragment before use -- deliberately NOT built, since nothing
// today is remotely close to needing it.
func discoveryExistenceAggregateStatusTopic() string {
	return "discovery_existence/state"
}

// PublishAggregateStatus publishes every gateway's own current status snapshot, together, to
// mainClient (bare topic) and -- when cloudClient is non-nil -- also to cloudClient, retained. The
// cloud copy's topic is additionally prefixed with ownInstallation, same collision-avoidance
// reasoning the old per-gateway publishStatus established (2026-09-05) -- an installation name like
// "main" is just as likely to repeat across houses as any gatewayID was.
func (t *TDiscoveryExistenceTracker) PublishAggregateStatus(mainClient, cloudClient mqtt.Client, ownInstallation string) error {
	snapshot := t.AggregateSnapshot()
	payload := make(map[string]map[string]string, len(snapshot))
	for gatewayID, leaves := range snapshot {
		leafPayload := make(map[string]string, len(leaves))
		for leaf, status := range leaves {
			leafPayload[leaf] = string(status)
		}
		payload[gatewayID] = leafPayload
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling aggregate discovery existence status: %w", err)
	}
	topic := discoveryExistenceAggregateStatusTopic()
	if err := publishRetained(mainClient, topic, data); err != nil {
		return fmt.Errorf("publishing %s (local): %w", topic, err)
	}
	if cloudClient != nil {
		cloudTopic := ownInstallation + "/" + topic
		if err := publishRetained(cloudClient, cloudTopic, data); err != nil {
			return fmt.Errorf("publishing %s (cloud): %w", cloudTopic, err)
		}
	}
	return nil
}

// DiscoveryExistenceStatusDebounceDelay/DiscoveryExistenceStatusPeriodicInterval mirror
// PassthroughStatusDebounceDelay/PassthroughStatusPeriodicInterval exactly
// (discovery_passthrough_devices.go) -- same reasoning: MarkKnown/MarkRetracted fire once per leaf
// during a retained-backlog replay at startup (hundreds of calls in a tight loop for a house with
// many gateways), and a synchronous full-aggregate publish per call would repeat the exact
// 2026-09-14 crash-loop incident this codebase already learned from once (see that file's own
// header comment for the full story). Explicit call arguments, not package-level consts, for the
// same testability reasoning as that precedent.
const (
	DiscoveryExistenceStatusDebounceDelay    = 30 * time.Second
	DiscoveryExistenceStatusPeriodicInterval = 10 * time.Minute
)

// ScheduleAggregateStatusPublish debounces a PublishAggregateStatus call by delay -- see
// DiscoveryExistenceStatusDebounceDelay's own doc comment for why a plain per-call publish isn't
// safe here. Safe to call directly from the MQTT message-dispatch callback (discoverybridge.go):
// the actual publish always happens later, on this timer's own goroutine, never synchronously on
// the caller's. Repeated calls within delay of each other keep resetting the same pending timer.
func (t *TDiscoveryExistenceTracker) ScheduleAggregateStatusPublish(mainClient, cloudClient mqtt.Client, ownInstallation string, delay time.Duration) {
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
		if err := t.PublishAggregateStatus(mainClient, cloudClient, ownInstallation); err != nil {
			fmt.Printf("[discovery-existence] debounced status publish: %v\n", err)
		}
	})
}

// StartPeriodicAggregateStatusPublish starts a background goroutine publishing t's full aggregate
// status on a fixed interval cadence, for the lifetime of the process -- purely a safety net
// alongside ScheduleAggregateStatusPublish, same reasoning as passthrough's own pair.
func (t *TDiscoveryExistenceTracker) StartPeriodicAggregateStatusPublish(mainClient, cloudClient mqtt.Client, ownInstallation string, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			if err := t.PublishAggregateStatus(mainClient, cloudClient, ownInstallation); err != nil {
				fmt.Printf("[discovery-existence] periodic status publish: %v\n", err)
			}
		}
	}()
}
