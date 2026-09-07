/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: EntityExistence
 *
 * Three-state entity-existence tracking and the paced inquiry loop -- PROJECT.md 1.1 (memory:
 * project_entity_existence_inquiry_design). Every source entity a positioned hassbridge capability
 * references (coordinator/homeassistant_bridge.yaml, already loaded by main.go) starts out
 * not-known-to-exist; the loop asks the owning instance about one entity at a time (via
 * homeassistant/remote_instance_entity_existence.go's inquiry automation -- a single states[...]
 * dict lookup, never a full-instance scan) and records the reply. Applies uniformly to every named
 * instance, including "main" -- there is no scale-dependent shortcut here, that's the whole point
 * of asking about one entity at a time instead of dumping every state at once.
 *
 * Persisted to entity_existence.json (coordinator-owned runtime state, mirroring
 * discoverycleanup.go's discovery_topics.json pattern) so a coordinator restart doesn't forget
 * everything previously learned -- newEntityExistenceTracker loads it up front, and every mutation
 * (Seed/Record/DiscoverSiblings/SeedManualEntity) re-persists. Seed's own "already tracked? skip"
 * check (unchanged) is what makes this safe: re-seeding from homeassistant_bridge.yaml after a
 * restart never resets an already-resolved status, it only adds genuinely new capabilities.
 *
 * Entities with no owning device (DeviceID == "") -- today only ever produced by
 * discover_entity_input.go's manual "Discover entity" bootstrap before its first
 * DiscoverSiblings round resolves a device for it -- are inquired exactly like any other tracked
 * entity; nextToInquire's single flat priority order never distinguished device-linked from
 * orphan entities to begin with, so there was never a separate pass to build for them.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 28.08.2026
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
)

// TEntityExistenceStatus is one entity's current existence status, from the coordinator's own
// inquiry plus the involved integration's own reply -- PROJECT.md 1.1's own three values, exact
// strings (these are also what gets published to the generator, mqtt_entity_existence.go must
// stay in sync).
type TEntityExistenceStatus string

const (
	StatusNotKnownToExist TEntityExistenceStatus = "not-known-to-exist"
	StatusKnownToExist    TEntityExistenceStatus = "known-to-exist"
	StatusKnownNotToExist TEntityExistenceStatus = "known-not-to-exist"
	existenceInquiryTick                         = 10 * time.Second // how often the loop wakes to check every instance
	existenceMinInterval                         = 1 * time.Minute  // "at most one inquiry per minute" -- per instance
)

// TEntityExistenceEntry is one tracked source entity's current status.
type TEntityExistenceEntry struct {
	DeviceID string // "" if this entity has no owning device
	// RemoteDeviceID is the remote instance's own HA device_id DeviceID was minted from, when
	// DeviceID is a coordinator-synthesized "hass.discovered_..." id (DiscoverSiblings) -- "" for
	// entries with no device, and for a real Physical.def-declared device id (those are never
	// ambiguous, no remote id needed to disambiguate them). Exists solely so a second,
	// differently-remote-device_id anchor that happens to share the exact same human-readable
	// device name (confirmed live 2026-08-29: two separate Netatmo modules both auto-named
	// "Vienna Terrace") never collapses into the first one's synthetic grouping.
	RemoteDeviceID string
	Status         TEntityExistenceStatus
	State          string // last-known state value, only meaningful when Status == StatusKnownToExist
	// Unit/DeviceClass are the remote entity's own reported unit_of_measurement/device_class
	// (added 2026-09-06), piggybacked off the same inquiry reply that already answers "does this
	// exist" -- only meaningful when Status == StatusKnownToExist, "" otherwise. Used by
	// discoveryhassbridge.go as a fallback typing source (behind Physical.def's own explicit
	// declaration and Defaults.def/code-level rules) when neither of those set anything for a
	// bridged capability, rather than leaving it with no unit/icon at all even though the remote
	// entity genuinely has both.
	Unit        string
	DeviceClass string
}

// TEntityExistenceTracker holds, per named HA instance, every hassbridge-referenced source
// entity's current status, and drives the paced inquiry loop.
type TEntityExistenceTracker struct {
	mu       sync.Mutex
	path     string                                       // persistPath's own record; "" disables persistence entirely (tests)
	entries  map[string]map[string]*TEntityExistenceEntry // instance -> sourceEntity -> entry
	order    map[string][]string                          // instance -> stable registration order (round-robin base)
	cursor   map[string]int                               // instance -> next round-robin index for the "recheck known" pass
	lastSent map[string]time.Time                         // instance -> last inquiry send time
}

// newEntityExistenceTracker loads path's previously persisted state, if any (loadPersisted --
// never fails, missing/incompatible falls back to empty, exactly like discoverycleanup.go's
// loadTopicManifest) so a coordinator restart resumes from what was already known instead of
// forgetting it. path == "" disables persistence outright (no load, no writes) -- used by tests
// that have no reason to touch disk.
func newEntityExistenceTracker(path string) *TEntityExistenceTracker {
	t := &TEntityExistenceTracker{
		path:     path,
		entries:  map[string]map[string]*TEntityExistenceEntry{},
		order:    map[string][]string{},
		cursor:   map[string]int{},
		lastSent: map[string]time.Time{},
	}
	t.loadPersisted()
	return t
}

// entityExistencePersistedInstance/entityExistencePersistedFile are entity_existence.json's own
// on-disk shape -- one entry per instance, preserving registration order (nextToInquire's
// round-robin base) alongside each entity's last-known entry.
type entityExistencePersistedInstance struct {
	Order   []string                         `json:"order"`
	Entries map[string]TEntityExistenceEntry `json:"entries"`
}

type entityExistencePersistedFile struct {
	Instances map[string]entityExistencePersistedInstance `json:"instances"`
}

// loadPersisted populates t.entries/t.order from t.path, if set and readable. A missing file, an
// empty path, or content in an incompatible shape all fall back to starting empty -- this is a
// coordinator-owned runtime cache, always safe to rebuild from nothing (the ordinary Seed/inquiry
// loop repopulates it from scratch either way, just more slowly).
func (t *TEntityExistenceTracker) loadPersisted() {
	if t.path == "" {
		return
	}
	data, err := os.ReadFile(t.path)
	if err != nil {
		return
	}
	var persisted entityExistencePersistedFile
	if err := json.Unmarshal(data, &persisted); err != nil {
		return
	}
	for instance, saved := range persisted.Instances {
		entries := make(map[string]*TEntityExistenceEntry, len(saved.Entries))
		for entity, entry := range saved.Entries {
			entryCopy := entry
			entries[entity] = &entryCopy
		}
		t.entries[instance] = entries
		t.order[instance] = saved.Order
	}
}

// persist writes t's current entries/order to t.path -- a no-op if persistence is disabled
// (t.path == ""). Must be called with t.mu already held (every caller is already inside a
// Lock/defer Unlock pair), mirroring discoverycleanup.go's TDiscoveryPublisher.persist.
func (t *TEntityExistenceTracker) persist() {
	if t.path == "" {
		return
	}
	persisted := entityExistencePersistedFile{Instances: make(map[string]entityExistencePersistedInstance, len(t.entries))}
	for instance, entries := range t.entries {
		saved := entityExistencePersistedInstance{Order: t.order[instance], Entries: make(map[string]TEntityExistenceEntry, len(entries))}
		for entity, entry := range entries {
			saved.Entries[entity] = *entry
		}
		persisted.Instances[instance] = saved
	}
	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		fmt.Printf("[existence] marshalling persisted state: %v\n", err)
		return
	}
	if err := os.WriteFile(t.path, data, 0o644); err != nil {
		fmt.Printf("[existence] persisting state to %s: %v\n", t.path, err)
	}
}

// Seed registers every hassbridge capability's source entity as not-known-to-exist, grouped by
// its owning device -- the generator's own "assumed to exist" list. homeassistant_bridge.yaml
// already *is* that list (every capability declared there is exactly what Physical.def/Spaces.def
// assume exists), so no separate generator output is needed to carry it -- this coordinator reads
// what it already loads.
//
// Also prunes confirmed-dead ghost entries (added 2026-09-05): a source that's both (a)
// known-not-to-exist and (b) no longer declared anywhere in bridgeFile is deleted outright, rather
// than sitting in persisted state forever. Without this, a fixed-or-removed Physical.def typo (e.g.
// sensor.living_room__battery, the double-underscore mistake found live the same day) permanently
// occupies a "recheck known" round-robin slot even after nothing references it any more -- states[]
// already told us definitively it doesn't exist (exists=false, not just an unresolved "unknown"),
// so once it also drops out of the declared set there is nothing left to ever resolve. Scoped
// tightly to known-not-to-exist only: a not-yet-resolved or known-to-exist entry -- including a
// DiscoverSiblings-discovered sibling or an in-progress manual "Discover entity" seed, neither of
// which bridgeFile ever declares -- is never touched here, only a source that's both confirmed
// absent AND no longer anyone's concern gets removed.
func (t *TEntityExistenceTracker) Seed(bridgeFile THassBridgeFile) {
	t.mu.Lock()
	defer t.mu.Unlock()
	declared := map[string]map[string]bool{} // instance -> entity -> still declared this round
	for deviceID, device := range bridgeFile.Devices {
		// Each instance seeds its OWN declared source entity, never a shared one -- a roaming
		// device's own local entity_id for the same capability can genuinely differ across
		// instances (real incident, found live 2026-09-05: Vienna's own registry initially lacked
		// hass.eriks_iphone's battery_level entity under the identical name Junglinster's
		// instances used). A capability an instance never declared a source for is skipped for
		// that instance -- not every instance necessarily reports every capability.
		for _, instance := range device.Instances {
			if _, ok := t.entries[instance]; !ok {
				t.entries[instance] = map[string]*TEntityExistenceEntry{}
			}
			if declared[instance] == nil {
				declared[instance] = map[string]bool{}
			}
			for _, cap := range device.Capabilities {
				source, declaredHere := cap.SourceEntities[instance]
				if source == "" || !declaredHere {
					continue
				}
				entity := bareEntityFromSource(source)
				declared[instance][entity] = true
				if _, seen := t.entries[instance][entity]; seen {
					continue
				}
				t.entries[instance][entity] = &TEntityExistenceEntry{
					DeviceID: deviceID,
					Status:   StatusNotKnownToExist,
				}
				t.order[instance] = append(t.order[instance], entity)
			}
		}
	}

	for instance, entities := range t.entries {
		for entity, entry := range entities {
			if entry.Status != StatusKnownNotToExist || declared[instance][entity] {
				continue
			}
			delete(t.entries[instance], entity)
			t.order[instance] = removeString(t.order[instance], entity)
			fmt.Printf("[existence] %s: pruned %q -- confirmed not-to-exist and no longer declared\n", instance, entity)
		}
	}

	t.persist()
}

// removeString returns list with every occurrence of want removed, preserving order.
func removeString(list []string, want string) []string {
	out := list[:0]
	for _, item := range list {
		if item != want {
			out = append(out, item)
		}
	}
	return out
}

// bareEntityFromSource strips a source specification's "!attribute" or " is available" suffix
// (homeassistant/remote_instance_automations.go's sourceToJinja2's own two sugars) -- states[...]
// only ever accepts a plain entity_id, not a compound expression, so a source carrying either
// sugar must be reduced to the bare entity_id before it's usable as an inquiry target. Confirmed
// live 2026-08-28: without this, "sensor.processor_use is available" (a "node" capability's raw
// source) was being inquired about literally, could never resolve (not a real entity_id), and
// permanently monopolized "main"'s not-known-to-exist priority slot every tick -- starving
// sensor.processor_temperature, which never got reached.
func bareEntityFromSource(src string) string {
	if entityID, ok := strings.CutSuffix(src, " is available"); ok {
		return entityID
	}
	if bangIdx := strings.Index(src, "!"); bangIdx > 0 {
		return src[:bangIdx]
	}
	return src
}

// nextToInquire picks the next source entity to ask instance about: any not-known-to-exist entity
// first, in registration order (the backlog of unresolved/newly-seeded ones); once every entity
// has a resolved status, round-robin through them as a refresh pass. This applies uniformly
// whether or not the entity has a known owning device -- PROJECT.md 1.1's original design sketched
// a separate third pass just for device-less ("orphan") entities, but registration order and the
// round-robin refresh already cover them exactly like any other entity, so no such separate pass
// was ever needed. Returns "" if instance has nothing tracked.
func (t *TEntityExistenceTracker) nextToInquire(instance string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	byEntity := t.entries[instance]
	order := t.order[instance]
	if len(order) == 0 {
		return ""
	}
	for _, entity := range order {
		if byEntity[entity].Status == StatusNotKnownToExist {
			return entity
		}
	}
	idx := t.cursor[instance] % len(order)
	t.cursor[instance] = idx + 1
	return order[idx]
}

// Record applies one inquiry reply to instance's tracked status for sourceEntity. A reply about an
// entity this tracker never asked about (stale/unexpected) is ignored, not an error.
func (t *TEntityExistenceTracker) Record(instance, sourceEntity string, exists bool, state, unit, deviceClass string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	byEntity, ok := t.entries[instance]
	if !ok {
		return
	}
	entry, ok := byEntity[sourceEntity]
	if !ok {
		return
	}
	if exists {
		entry.Status = StatusKnownToExist
		entry.State = state
		entry.Unit = unit
		entry.DeviceClass = deviceClass
	} else {
		entry.Status = StatusKnownNotToExist
		entry.State = ""
		entry.Unit = ""
		entry.DeviceClass = ""
	}
	t.persist()
}

// LiveTyping returns instance's own last-reported unit_of_measurement/device_class for
// sourceEntity, "" for either (or both) if never resolved to known-to-exist, or unknown to this
// tracker entirely -- discoveryhassbridge.go's own fallback typing source, behind Physical.def's
// explicit declaration and Defaults.def/code-level rules.
func (t *TEntityExistenceTracker) LiveTyping(instance, sourceEntity string) (unit, deviceClass string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry, ok := t.entries[instance][sourceEntity]
	if !ok {
		return "", ""
	}
	return entry.Unit, entry.DeviceClass
}

// DiscoverSiblings folds newly-seen entities into instance's tracked set, attributed to the same
// DSL device as anchorEntity (an entity already tracked -- the one whose inquiry reply carried
// these siblings via device_entities(), homeassistant/remote_instance_entity_existence.go). This
// is the "which entities they provide" half of PROJECT.md 1.1's design: a sibling HA's own device
// registry reports for a known device, that this tracker didn't already know about, is added as
// known-to-exist immediately -- device_entities() only ever returns entities that genuinely exist,
// so no separate confirmation round is needed for these. A no-op if anchorEntity isn't tracked
// (nothing to attribute the discovery to) or already-known siblings, which just fall through.
//
// If anchor itself has no owning DSL device yet (DeviceID == "" -- true for anything seeded via
// discover_entity_input.go's manual bootstrap, which never knows a device up front), a synthetic
// grouping id is minted from deviceName (the remote instance's own device_attr(did, 'name'),
// homeassistant/remote_instance_entity_existence.go) and assigned to *both* the anchor and its
// newly-discovered siblings, so a manually-discovered device still lands in the suggestion
// report's usual "device X with: ...;" grouping instead of permanently sitting in the "no known
// device grouping" section. Once assigned, that synthetic id sticks (re-discovery on a later
// inquiry reuses it via anchor.DeviceID being non-empty already), so it stays stable across ticks.
func (t *TEntityExistenceTracker) DiscoverSiblings(instance, anchorEntity, remoteDeviceID, deviceName string, siblings []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	byEntity, ok := t.entries[instance]
	if !ok {
		return
	}
	anchor, ok := byEntity[anchorEntity]
	if !ok {
		return
	}
	if anchor.DeviceID == "" && len(siblings) > 0 {
		anchor.DeviceID = resolveSyntheticDeviceID(byEntity, remoteDeviceID, deviceName, anchorEntity)
		anchor.RemoteDeviceID = remoteDeviceID
		fmt.Printf("[existence] %s: %s has no known device, grouping it and its siblings under synthetic device %q\n", instance, anchorEntity, anchor.DeviceID)
	}
	for _, sibling := range siblings {
		if sibling == "" || sibling == anchorEntity {
			continue
		}
		if _, seen := byEntity[sibling]; seen {
			continue
		}
		byEntity[sibling] = &TEntityExistenceEntry{DeviceID: anchor.DeviceID, RemoteDeviceID: anchor.RemoteDeviceID, Status: StatusKnownToExist}
		t.order[instance] = append(t.order[instance], sibling)
		fmt.Printf("[existence] %s: discovered new sibling %s (via %s, device %q)\n", instance, sibling, anchorEntity, anchor.DeviceID)
	}
	t.persist()
}

// syntheticDeviceID mints a stable, obviously-a-placeholder "hass.<slug>" grouping id for a device
// the DSL never declared (see DiscoverSiblings) -- preferring the remote instance's own
// human-readable device name, falling back to the anchor entity_id itself when the instance
// reports no name at all (device_attr(did, 'name') can legitimately return none).
func syntheticDeviceID(deviceName, fallback string) string {
	source := deviceName
	if source == "" {
		source = fallback
	}
	return "hass.discovered_" + slugify(source)
}

// resolveSyntheticDeviceID returns the synthetic local device id for remoteDeviceID within
// byEntity's own tracked entries -- reusing an already-minted id for the *same* remote device_id
// if one exists (e.g. a later DiscoverSiblings round reaching the same device via a different
// anchor entity), and minting + disambiguating a new one otherwise. Two genuinely different remote
// devices that happen to share the exact same human-readable name must never collapse into one
// synthetic grouping (confirmed live 2026-08-29: two separate Netatmo modules both auto-named
// "Vienna Terrace") -- disambiguated with a numeric suffix, mirroring how the remote instance's
// own entity naming already disambiguates same-named siblings ("_2").
func resolveSyntheticDeviceID(byEntity map[string]*TEntityExistenceEntry, remoteDeviceID, deviceName, fallback string) string {
	if remoteDeviceID != "" {
		for _, entry := range byEntity {
			if entry.RemoteDeviceID == remoteDeviceID {
				return entry.DeviceID
			}
		}
	}
	base := syntheticDeviceID(deviceName, fallback)
	id := base
	for n := 2; deviceIDClaimedByDifferentRemote(byEntity, id, remoteDeviceID); n++ {
		id = fmt.Sprintf("%s_%d", base, n)
	}
	return id
}

// deviceIDClaimedByDifferentRemote reports whether id is already in use as some entry's DeviceID,
// attributed to a remote device_id other than remoteDeviceID -- the collision
// resolveSyntheticDeviceID's disambiguation loop exists to detect.
func deviceIDClaimedByDifferentRemote(byEntity map[string]*TEntityExistenceEntry, id, remoteDeviceID string) bool {
	for _, entry := range byEntity {
		if entry.DeviceID == id && entry.RemoteDeviceID != remoteDeviceID {
			return true
		}
	}
	return false
}

// slugify reduces s to a Physical.def-style identifier fragment: lowercase, runs of anything
// other than [a-z0-9] collapsed to a single "_", leading/trailing "_" trimmed.
func slugify(s string) string {
	var sb strings.Builder
	prevUnderscore := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			sb.WriteRune(r)
			prevUnderscore = false
		case !prevUnderscore:
			sb.WriteByte('_')
			prevUnderscore = true
		}
	}
	return strings.Trim(sb.String(), "_")
}

// snapshotByDevice returns instance's current status, grouped by owning device id, for publishing.
func (t *TEntityExistenceTracker) snapshotByDevice(instance string) map[string]map[string]TEntityExistenceEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	byDevice := map[string]map[string]TEntityExistenceEntry{}
	for entity, entry := range t.entries[instance] {
		if byDevice[entry.DeviceID] == nil {
			byDevice[entry.DeviceID] = map[string]TEntityExistenceEntry{}
		}
		byDevice[entry.DeviceID][entity] = *entry
	}
	return byDevice
}

// dueForInquiry reports whether instance's existenceMinInterval has elapsed since its last
// inquiry (or none was ever sent), and -- if so -- reserves this tick by updating lastSent
// immediately, so two near-simultaneous ticks can never both fire for the same instance.
func (t *TEntityExistenceTracker) dueForInquiry(instance string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	last, ok := t.lastSent[instance]
	if ok && now.Sub(last) < existenceMinInterval {
		return false
	}
	t.lastSent[instance] = now
	return true
}

// existenceInquiryReply is homeassistant/remote_instance_entity_existence.go's own JSON reply
// shape -- keep the two in sync. SiblingEntities is device_entities(DeviceID) as reported by the
// instance itself -- every entity HA's own registry already associates with EntityID's device,
// which may include entities this tracker has never seen before (DiscoverSiblings). DeviceName is
// device_attr(DeviceID, 'name') -- only used by DiscoverSiblings, to name a synthetic device
// grouping for an anchor the DSL never declared a device for.
type existenceInquiryReply struct {
	EntityID        string   `json:"entity_id"`
	Exists          bool     `json:"exists"`
	State           string   `json:"state"`
	Unit            string   `json:"unit_of_measurement"`
	DeviceClass     string   `json:"device_class"`
	DeviceID        string   `json:"device_id"`
	DeviceName      string   `json:"device_name"`
	SiblingEntities []string `json:"sibling_entities"`
}

// existenceStatusDevicePayload/publishStatus are the JSON shape published to
// "homeassistant_instances/<name>/existence/state" -- grouped per device (device id "" holds any
// entity with no owning device), generator-friendly, mirroring the manifest topic's own
// "transformer-friendly JSON, no unnecessary parsing" convention.
type existenceStatusDevicePayload struct {
	Entities map[string]existenceStatusEntityPayload `json:"entities"`
}

type existenceStatusEntityPayload struct {
	Status string `json:"status"`
	State  string `json:"state,omitempty"`
}

// publishStatus publishes instance's current status snapshot to mainClient (bare topic) and --
// when cloudClient is non-nil -- also to cloudClient, unconditionally (not "prefer one"), retained.
// Mirrors entity_catalogue.go's subscribeEntityCatalogue relay policy exactly: the coordinator
// always relays to both when a cloud broker is configured, so the generator (which may run from
// anywhere, not just on the house LAN) can read via whichever is reachable, preferring cloud
// (mqtt_entity_existence.go's fetchEntityExistencePreferringCloud, generator side).
//
// The CLOUD copy's topic is additionally prefixed with ownInstallation -- e.g.
// "junglinster/homeassistant_instances/main/existence/state" -- unlike the local (bare) copy. Real
// collision found live 2026-09-05: every house's own primary HA instance is conventionally named
// "main", so two houses sharing one cloud broker were overwriting each other's retained status
// under the same bare topic before this qualification existed. Local topics never need this: each
// house only ever reads its own local broker's copy of its own instances' status.
func (t *TEntityExistenceTracker) publishStatus(mainClient, cloudClient mqtt.Client, ownInstallation, instance string) error {
	byDevice := t.snapshotByDevice(instance)
	payload := make(map[string]existenceStatusDevicePayload, len(byDevice))
	for deviceID, entities := range byDevice {
		entityPayloads := make(map[string]existenceStatusEntityPayload, len(entities))
		for entity, entry := range entities {
			entityPayloads[entity] = existenceStatusEntityPayload{Status: string(entry.Status), State: entry.State}
		}
		payload[deviceID] = existenceStatusDevicePayload{Entities: entityPayloads}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling existence status for %s: %w", instance, err)
	}
	topic := existenceStatusTopic(instance)
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

func existenceInquiryTopic(instance string) string {
	return "homeassistant_instances/" + instance + "/inquire"
}
func existenceReplyTopic(instance string) string { return existenceInquiryTopic(instance) + "/reply" }
func existenceStatusTopic(instance string) string {
	return "homeassistant_instances/" + instance + "/existence/state"
}

// subscribeExistenceReply subscribes to instance's own inquiry-reply topic (local broker only --
// that's where the instance's own inquiry automation actually publishes), recording each reply and
// republishing the updated status snapshot to both mainClient and cloudClient (publishStatus).
func (t *TEntityExistenceTracker) subscribeExistenceReply(mainClient, cloudClient mqtt.Client, ownInstallation, instance string) error {
	handler := func(_ mqtt.Client, msg mqtt.Message) {
		var reply existenceInquiryReply
		if err := json.Unmarshal(msg.Payload(), &reply); err != nil {
			fmt.Printf("[existence] %s: cannot parse inquiry reply: %v\n", instance, err)
			return
		}
		t.Record(instance, reply.EntityID, reply.Exists, reply.State, reply.Unit, reply.DeviceClass)
		fmt.Printf("[existence] %s: %s -> exists=%v\n", instance, reply.EntityID, reply.Exists)
		if reply.Exists && len(reply.SiblingEntities) > 0 {
			t.DiscoverSiblings(instance, reply.EntityID, reply.DeviceID, reply.DeviceName, reply.SiblingEntities)
		}
		if err := t.publishStatus(mainClient, cloudClient, ownInstallation, instance); err != nil {
			fmt.Printf("[existence] %v\n", err)
		}
	}
	topic := existenceReplyTopic(instance)
	token := mainClient.Subscribe(topic, 0, handler)
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		if err := token.Error(); err != nil {
			return fmt.Errorf("subscribing to %s: %w", topic, err)
		}
		return fmt.Errorf("subscribing to %s: timed out", topic)
	}
	return nil
}

// maybeInquire sends at most one inquiry for instance, only if existenceMinInterval has elapsed
// since its last one -- the actual per-instance ("per integration") pacing PROJECT.md 1.1 asks for.
func (t *TEntityExistenceTracker) maybeInquire(client mqtt.Client, instance string) {
	if !t.dueForInquiry(instance, time.Now()) {
		return
	}
	entity := t.nextToInquire(instance)
	if entity == "" {
		return
	}
	t.publishInquiry(client, instance, entity)
}

// InquireNow immediately publishes an inquiry for entityID on instance, bypassing the normal
// per-instance pacing (dueForInquiry) -- for the rare, explicit, human-triggered "discover this
// specific entity" request (discover_entity_input.go), not the automatic loop the pacing exists to
// protect the instance from. Still updates lastSent, so the automatic loop's own next tick for
// this instance waits its normal interval from here rather than from whenever it last ran.
func (t *TEntityExistenceTracker) InquireNow(client mqtt.Client, instance, entityID string) {
	t.mu.Lock()
	t.lastSent[instance] = time.Now()
	t.mu.Unlock()
	t.publishInquiry(client, instance, entityID)
}

// publishInquiry is the raw "send this one inquiry" primitive maybeInquire and InquireNow both
// build on -- kept as one place so the topic/logging/error-handling can't drift between the two.
func (t *TEntityExistenceTracker) publishInquiry(client mqtt.Client, instance, entity string) {
	token := client.Publish(existenceInquiryTopic(instance), 0, false, []byte(entity))
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		if err := token.Error(); err != nil {
			fmt.Printf("[existence] inquiring about %s on %s: %v\n", entity, instance, err)
		} else {
			fmt.Printf("[existence] inquiring about %s on %s: timed out\n", entity, instance)
		}
		return
	}
	fmt.Printf("[existence] %s: inquired about %s\n", instance, entity)
}

// SeedManualEntity registers entityID as a tracked, not-known-to-exist entry for instance if it
// isn't already tracked -- the human-triggered counterpart to Seed (discover_entity_input.go). A
// no-op if entityID is already tracked, whatever its current status, so re-requesting something
// already known doesn't reset its state. DeviceID starts "" (unknown) -- if this entity turns out
// to belong to a device, that only becomes known once its own inquiry reply arrives.
func (t *TEntityExistenceTracker) SeedManualEntity(instance, entityID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.entries[instance]; !ok {
		t.entries[instance] = map[string]*TEntityExistenceEntry{}
	}
	if _, seen := t.entries[instance][entityID]; seen {
		return
	}
	t.entries[instance][entityID] = &TEntityExistenceEntry{Status: StatusNotKnownToExist}
	t.order[instance] = append(t.order[instance], entityID)
	t.persist()
}

// StartEntityExistenceInquiries subscribes to every instance's own reply topic (mainClient --
// where each instance's own inquiry automation actually lives), publishes each instance's current
// status snapshot immediately (see below), then starts the paced inquiry loop in its own goroutine
// -- one tick every existenceInquiryTick, each tick considering every instance independently (each
// instance's own existenceMinInterval gates whether that particular instance actually gets an
// inquiry this tick). Resulting status snapshots are relayed to cloudClient too (publishStatus),
// when non-nil -- inquiry commands/replies themselves stay local-only, only the aggregated status is
// cloud-relayed, mirroring entity_catalogue.go's own local-vs-relayed split. No-op (returns nil
// immediately) if instances is empty.
//
// The immediate publish (added 2026-09-05) matters because publishStatus is otherwise only called
// on an actual reply/transition: a coordinator restart re-seeds already-known status in memory (from
// persisted entity_existence.json) without re-publishing it, so the retained MQTT topic itself can
// go stale or missing -- e.g. after a broker-side retained message is cleared, or (the case that
// exposed this) a topic-naming change like §12's cloud installation-qualification fix, which left
// the newly-qualified cloud topic with no retained value at all until the next real inquiry reply,
// causing every ./generate run's cloud fetch to time out in the meantime.
func (t *TEntityExistenceTracker) StartEntityExistenceInquiries(mainClient, cloudClient mqtt.Client, ownInstallation string, instances []string) error {
	if len(instances) == 0 {
		return nil
	}
	for _, instance := range instances {
		if err := t.subscribeExistenceReply(mainClient, cloudClient, ownInstallation, instance); err != nil {
			return err
		}
	}
	for _, instance := range instances {
		if err := t.publishStatus(mainClient, cloudClient, ownInstallation, instance); err != nil {
			fmt.Printf("[existence] %v\n", err)
		}
	}
	go func() {
		ticker := time.NewTicker(existenceInquiryTick)
		defer ticker.Stop()
		for range ticker.C {
			for _, instance := range instances {
				t.maybeInquire(mainClient, instance)
			}
		}
	}()
	return nil
}
