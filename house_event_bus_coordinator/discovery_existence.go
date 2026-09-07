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
	"sync"

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
	defer t.mu.Unlock()
	if _, ok := t.entries[gatewayID]; !ok {
		t.entries[gatewayID] = map[string]TEntityExistenceStatus{}
	}
	if t.entries[gatewayID][leaf] == StatusKnownToExist {
		return false
	}
	t.entries[gatewayID][leaf] = StatusKnownToExist
	t.persist()
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
	defer t.mu.Unlock()
	identity, found := t.topicIdentity[topic]
	if !found {
		return "", "", false
	}
	if _, exists := t.entries[identity.gatewayID]; !exists {
		t.entries[identity.gatewayID] = map[string]TEntityExistenceStatus{}
	}
	if t.entries[identity.gatewayID][identity.leaf] == StatusKnownNotToExist {
		return identity.gatewayID, identity.leaf, false
	}
	t.entries[identity.gatewayID][identity.leaf] = StatusKnownNotToExist
	t.persist()
	return identity.gatewayID, identity.leaf, true
}

// PublishAll immediately (re-)publishes every gatewayID declared in discoveryFile.Gateways --
// called once right after Seed, mirroring entity_existence.go's own StartEntityExistenceInquiries
// immediate-publish fix (2026-09-05, see its own doc comment for the full reasoning): unlike kind-3,
// kind-2 has no periodic inquiry loop to eventually self-heal a stale/missing retained topic --
// publishStatus here only ever fires on an actual known/retracted transition, which a gateway with
// entirely stable, already-known leaves may not produce again for a very long time. Without this,
// a coordinator restart (or a topic-naming migration like §12's cloud installation-qualification
// fix, which exposed this gap live) leaves the retained topic missing or stale until the next real
// transition, and every ./generate run's cloud fetch times out in the meantime.
func (t *TDiscoveryExistenceTracker) PublishAll(mainClient, cloudClient mqtt.Client, ownInstallation string, discoveryFile TDiscoveryFile) {
	for gatewayID := range discoveryFile.Gateways {
		if err := t.publishStatus(mainClient, cloudClient, ownInstallation, gatewayID); err != nil {
			fmt.Printf("[discovery-existence] %v\n", err)
		}
	}
}

// snapshot returns gatewayID's current status map, for publishing.
func (t *TDiscoveryExistenceTracker) snapshot(gatewayID string) map[string]TEntityExistenceStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]TEntityExistenceStatus, len(t.entries[gatewayID]))
	for leaf, status := range t.entries[gatewayID] {
		out[leaf] = status
	}
	return out
}

func discoveryExistenceStatusTopic(gatewayID string) string {
	return "discovery_gateways/" + gatewayID + "/existence/state"
}

// publishStatus publishes gatewayID's current status snapshot to mainClient (bare topic) and --
// when cloudClient is non-nil -- also to cloudClient unconditionally, retained. Mirrors
// entity_existence.go's own publishStatus policy exactly, so the generator can read via whichever
// broker is reachable.
//
// The CLOUD copy's topic is additionally prefixed with ownInstallation, same reasoning and same
// live-collision incident (2026-09-05) as entity_existence.go's own publishStatus -- a gatewayID
// like "discovery.ems_esp" is just as likely to repeat across houses as an instance name like
// "main" is.
func (t *TDiscoveryExistenceTracker) publishStatus(mainClient, cloudClient mqtt.Client, ownInstallation, gatewayID string) error {
	snapshot := t.snapshot(gatewayID)
	payload := make(map[string]string, len(snapshot))
	for leaf, status := range snapshot {
		payload[leaf] = string(status)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling discovery existence status for %s: %w", gatewayID, err)
	}
	topic := discoveryExistenceStatusTopic(gatewayID)
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
