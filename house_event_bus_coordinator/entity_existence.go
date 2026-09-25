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
	"regexp"
	"sort"
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
	// RemoteViaDeviceID is the remote instance's own device registry "via_device_id" for
	// RemoteDeviceID -- another remote device id this one is "connected via" (e.g. a Netatmo radio
	// module via its own base station), added 2026-09-14 (PROJECT.md item 3/
	// plans/via-device-inference.md). "" if the remote reported none, or hasn't been inquired yet.
	// ResolveViaDevice matches this against another tracked entry's own RemoteDeviceID, within the
	// same instance, one hop only -- see that method's own doc comment.
	RemoteViaDeviceID string
	Status            TEntityExistenceStatus
	State             string // last-known state value, only meaningful when Status == StatusKnownToExist
	// Unit/DeviceClass are the remote entity's own reported unit_of_measurement/device_class
	// (added 2026-09-06), piggybacked off the same inquiry reply that already answers "does this
	// exist" -- only meaningful when Status == StatusKnownToExist, "" otherwise. Used by
	// discoveryhassbridge.go as a fallback typing source (behind Physical.def's own explicit
	// declaration and Defaults.def/code-level rules) when neither of those set anything for a
	// bridged capability, rather than leaving it with no unit/icon at all even though the remote
	// entity genuinely has both.
	Unit        string
	DeviceClass string
	// Icon is the remote entity's own reported "icon" attribute (added 2026-09-09 -- real gap
	// found live on Vienna's washing_machine bridge: the 2026-09-06 unit/device_class fallback's
	// own doc comments already named icon as part of the same problem, but the fix never actually
	// carried it through). Same fallback tier as Unit/DeviceClass: only used when neither
	// Physical.def's own explicit declaration nor Defaults.def/code-level rules set one.
	Icon string
	// AttributeKeys (added 2026-09-21) is the remote entity's own live-reported HA attribute key
	// set -- feeds the generator's "undeclared live attributes" suggestion check
	// (homeassistant/mqtt_entity_existence.go's buildUndeclaredAttributesSuggestions), only
	// meaningful when Status == StatusKnownToExist. Deliberately NOT threaded through Record's own
	// signature (that already has 9 positional args and ~35 existing test call sites across this
	// package) -- set via the separate, additive RecordAttributeKeys instead.
	AttributeKeys []string
}

// TEntityExistenceTracker holds, per named HA instance, every hassbridge-referenced source
// entity's current status, and drives the paced inquiry loop.
type TEntityExistenceTracker struct {
	mu      sync.Mutex
	path    string                                       // persistPath's own record; "" disables persistence entirely (tests)
	entries map[string]map[string]*TEntityExistenceEntry // instance -> sourceEntity -> entry
	order   map[string][]string                          // instance -> stable registration order (round-robin base)
	cursor  map[string]int                               // instance -> next round-robin index for the "recheck known" pass
	// backlogCursor is nextToInquire's own rotation position within the not-known-to-exist
	// backlog specifically (as opposed to cursor's round-robin over EVERY entry, used only once
	// the whole backlog is empty). Real bug found live 2026-09-07: without this, nextToInquire
	// always rescanned the backlog from index 0 and returned the FIRST still-unresolved entry --
	// if that one entity's reply never arrived (a dropped MQTT message, an HA-side automation
	// queue overflow, or simply a typo'd/never-existing entity_id whose inquiry silently never
	// got recorded), it stayed the head of the backlog forever, permanently starving every OTHER
	// entity registered after it in order. Confirmed live: a bare main-instance entity (kind-5)
	// with a typo in its name sat unresolved for 3+ hours across multiple ./generate runs,
	// blocking everything alphabetically after it. See nextToInquire's own doc comment.
	backlogCursor map[string]int
	lastSent      map[string]time.Time // instance -> last inquiry send time
	// mainEntities is kind-5's own protected set (PROJECT.md item 1, 2026-09-07) -- every
	// entity SeedMainEntities currently declares for instance "main". In-memory only, never
	// persisted (no JSON schema change): Seed's own pruning loop consults it to avoid deleting a
	// confirmed-dead main-instance entity that bridgeFile never declared and so would otherwise
	// look, to Seed alone, exactly like an orphan. See SeedMainEntities' own doc comment for the
	// full mutual-protection contract and the call-order requirement it depends on.
	mainEntities map[string]bool

	// onChange (2026-09-21, PROJECT.md item 1a) fires, unconditionally with no arguments, whenever
	// any tracked entity (kind-3 hassbridge OR kind-5 main-instance, both share this one tracker)
	// transitions into or out of StatusKnownNotToExist -- mirrors
	// TDiscoveryExistenceTracker.onChange exactly (discovery_existence.go), same locking
	// discipline (read/cleared while locked, invoked only after an explicit unlock).
	onChange func()
}

// SetOnChange (re)sets the tracker's onChange callback -- see that field's own doc comment.
func (t *TEntityExistenceTracker) SetOnChange(onChange func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onChange = onChange
}

// KnownNotToExistItems returns a sorted "<instance>/<sourceEntity>" snapshot of every entry
// currently StatusKnownNotToExist -- feeds TMissingDeclaredEntitiesPublisher's own live aggregate
// indicator, covering kind-3 and kind-5 both (whichever instances/entities this tracker holds).
func (t *TEntityExistenceTracker) KnownNotToExistItems() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	var items []string
	for instance, byEntity := range t.entries {
		for sourceEntity, entry := range byEntity {
			if entry.Status == StatusKnownNotToExist {
				items = append(items, instance+"/"+sourceEntity)
			}
		}
	}
	sort.Strings(items)
	return items
}

// newEntityExistenceTracker loads path's previously persisted state, if any (loadPersisted --
// never fails, missing/incompatible falls back to empty, exactly like discoverycleanup.go's
// loadTopicManifest) so a coordinator restart resumes from what was already known instead of
// forgetting it. path == "" disables persistence outright (no load, no writes) -- used by tests
// that have no reason to touch disk.
func newEntityExistenceTracker(path string) *TEntityExistenceTracker {
	t := &TEntityExistenceTracker{
		path:          path,
		entries:       map[string]map[string]*TEntityExistenceEntry{},
		order:         map[string][]string{},
		cursor:        map[string]int{},
		backlogCursor: map[string]int{},
		lastSent:      map[string]time.Time{},
		mainEntities:  map[string]bool{},
	}
	t.loadPersisted()
	return t
}

// entityExistencePersistedInstance/entityExistencePersistedFile are entity_existence.json's own
// on-disk shape -- one entry per instance, preserving registration order (nextToInquire's
// round-robin base) alongside each entity's last-known entry.
//
// Cursor/BacklogCursor (added 2026-09-11) persist nextToInquire's own two round-robin positions --
// real bug found live: without these, every coordinator restart reset both cursors to 0, so the
// "recheck already-known entities" pass (cursor) always restarted from the front of order. An
// instance with many tracked entities and frequent restarts (redeploys) could then go a very long
// time -- in the worst case, indefinitely -- without ever reaching entities near the tail of its
// own order list, even though the remote instance's own inquiry-reply automation was fully capable
// of returning fresh typing/device-info the whole time. Confirmed live on Junglinster
// 2026-09-11: several of Vienna's own imported Netatmo devices (hass.vienna_bedroom and others)
// were missing manufacturer/model in coordinator/live_device_info.json despite a fresh manual
// inquiry immediately returning it correctly -- the automatic loop had simply never gotten back
// around to them since the tracker was last restarted.
type entityExistencePersistedInstance struct {
	Order         []string                         `json:"order"`
	Entries       map[string]TEntityExistenceEntry `json:"entries"`
	Cursor        int                              `json:"cursor,omitempty"`
	BacklogCursor int                              `json:"backlog_cursor,omitempty"`
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
		t.cursor[instance] = saved.Cursor
		t.backlogCursor[instance] = saved.BacklogCursor
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
		saved := entityExistencePersistedInstance{
			Order: t.order[instance], Entries: make(map[string]TEntityExistenceEntry, len(entries)),
			Cursor: t.cursor[instance], BacklogCursor: t.backlogCursor[instance],
		}
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
	// remoteDeviceIDToDevice: instance -> a declared entity's own RemoteDeviceID -> its current,
	// just-corrected DeviceID -- populated alongside the main correction loop below, used by the
	// SECOND pass (after that loop) to also correct DiscoverSiblings-discovered, UNDECLARED sibling
	// entries sharing the same real remote device. Real incident, 2026-09-22 (Junglinster/Vienna
	// Netatmo rename): Seed's own doc comment already explains why it deliberately never touches a
	// sibling entry (bridgeFile has no visibility into it at all) -- but that leaves such a sibling's
	// DeviceID stuck at whatever it was attributed to before a rename, FOREVER (same persistence
	// argument as the "real-but-STALE DeviceID" correction below). ResolveViaDevice's own second
	// loop (a plain, unsorted map range over every tracked entry sharing a via-target's
	// RemoteDeviceID) then non-deterministically picks EITHER a freshly-corrected declared entry OR
	// this stale sibling on any given call -- found live via a coordinator restart producing a
	// CORRECT via_device for one capability ("node") and a STALE one for another ("temperature") of
	// the exact same device, purely by which map iteration order Go happened to pick that call.
	// Filled in only from entries that were ALREADY tracked (so already carry a real RemoteDeviceID
	// from a past inquiry reply) -- populated in the same sorted-deviceIDs order as the correction
	// loop itself, so a RemoteDeviceID shared by two current devices (should never happen in
	// practice, but the existing shared-entity case above shows Physical.def has no way to forbid
	// it) resolves deterministically the same "last sorted device wins" way already established
	// there, not randomly.
	remoteDeviceIDToDevice := map[string]map[string]string{}
	// Sorted, not range-order: two devices can legitimately declare the SAME source entity as
	// their own capability (found live 2026-09-14 -- node.vienna_bedroom and node.vienna_livingroom
	// both declare "binary_sensor.vienna_bedroom_connectivity" as their own "node" capability, a
	// deliberate Physical.def workaround for a flaky dedicated living-room connectivity sensor).
	// Go's randomized map iteration then made the correction below (whichever DEVICE is processed
	// LAST in this loop wins the shared entity) flip non-deterministically between coordinator
	// restarts. Sorting doesn't resolve the underlying modeling ambiguity -- Physical.def itself
	// has no way to say which of two devices is the "real" owner of a shared entity -- but at
	// least makes the outcome stable and reproducible instead of random. ResolveViaDevice itself
	// no longer depends on this attribution being "correct" for the shared entity's own device
	// (see its own doc comment), so this is a belt-and-braces stability fix, not the primary one.
	deviceIDs := make([]string, 0, len(bridgeFile.Devices))
	for deviceID := range bridgeFile.Devices {
		deviceIDs = append(deviceIDs, deviceID)
	}
	sort.Strings(deviceIDs)
	for _, deviceID := range deviceIDs {
		device := bridgeFile.Devices[deviceID]
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
				if existing, seen := t.entries[instance][entity]; seen {
					// The currently-declared owner always wins over whatever was persisted before,
					// full stop -- Physical.def is always the naming authority, not the coordinator
					// (buildSuggestionReportFromExistence's own doc comment states this same
					// principle). Originally this only corrected a synthetic (or missing) DeviceID
					// (real bug found live 2026-09-08: DiscoverSiblings only mints a synthetic
					// "hass.discovered_..." grouping when an anchor's DeviceID is still "" at that
					// moment, which can happen for an entity that's genuinely declared here but
					// whose very first sighting -- a manual "Discover entity" request, or a
					// coordinator restart racing a Seed call -- happened before Seed ever got to
					// it; once wrongly synthetic, it was persisted that way forever after, since the
					// old "if seen, skip" check meant a later, correctly-ordered Seed call could
					// never self-heal it). Broadened 2026-09-14 (PROJECT.md item 3/
					// plans/via-device-inference.md, found live: Vienna never showed a resolved
					// via_device for a Netatmo module despite every underlying signal being
					// correct) -- a real-but-STALE DeviceID from BEFORE a device rename is just as
					// wrong as a synthetic one, and entity_existence.json's deliberate persistence
					// across every coordinator redeploy (Architecture.md §6.11) means it never gets
					// a chance to self-heal on its own: ResolveViaDevice's own lookup keys off the
					// device id THIS Seed call already knows is current, so a stale label there
					// silently breaks the match forever, with no error anywhere in the chain.
					// Status/State/Unit/DeviceClass/Icon are deliberately left untouched -- only
					// DeviceID itself changes here. RemoteDeviceID/RemoteViaDeviceID are
					// deliberately PRESERVED across this correction (changed 2026-09-14, alongside
					// broadening this check): they describe which REMOTE device this entry maps
					// to, a fact that's entirely independent of whatever LOCAL Physical.def label
					// is currently in use for it -- a pure rename doesn't change the real device on
					// the other end, so clearing already-correct data here would just force an
					// unnecessary fresh inquiry (real gap found live 2026-09-14: this correction
					// used to also clear RemoteDeviceID, meaning every coordinator restart after a
					// rename re-forced a fresh wait even on the SECOND and later restarts, not just
					// the first one following the rename itself).
					if existing.DeviceID != deviceID {
						fmt.Printf("[existence] %s: %s was grouped under %q, correcting to its currently-declared device %q\n", instance, entity, existing.DeviceID, deviceID)
						existing.DeviceID = deviceID
					}
					if existing.RemoteDeviceID != "" {
						if remoteDeviceIDToDevice[instance] == nil {
							remoteDeviceIDToDevice[instance] = map[string]string{}
						}
						remoteDeviceIDToDevice[instance][existing.RemoteDeviceID] = deviceID
					}
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

	// Second pass: correct DiscoverSiblings-discovered, UNDECLARED sibling entries too -- see
	// remoteDeviceIDToDevice's own doc comment above for the real incident this closes. Every
	// tracked entry (declared or not) whose RemoteDeviceID matches a device we just confirmed
	// current above gets its DeviceID corrected to match -- a plain map range here is safe (unlike
	// ResolveViaDevice's own, which this fixes the symptom of): each (instance, RemoteDeviceID) pair
	// maps to exactly one deterministically-chosen DeviceID already, so iteration order over entries
	// can't produce a different outcome for the same entry.
	for instance, entities := range t.entries {
		byRemote := remoteDeviceIDToDevice[instance]
		if len(byRemote) == 0 {
			continue
		}
		for entity, entry := range entities {
			if entry.RemoteDeviceID == "" {
				continue
			}
			if target, ok := byRemote[entry.RemoteDeviceID]; ok && entry.DeviceID != target {
				fmt.Printf("[existence] %s: sibling %s was grouped under %q, correcting to its currently-declared device %q\n", instance, entity, entry.DeviceID, target)
				entry.DeviceID = target
			}
		}
	}

	for instance, entities := range t.entries {
		for entity, entry := range entities {
			if entry.Status != StatusKnownNotToExist || declared[instance][entity] {
				continue
			}
			// Kind-5's own protected set (PROJECT.md item 1, 2026-09-07) -- a main-instance bare
			// entity SeedMainEntities currently declares. bridgeFile has no visibility into these
			// at all, so without this check every confirmed-dead one would look like an orphan to
			// THIS seed and get silently pruned on every coordinator restart, re-opening the
			// not-yet-checked window for hours. See SeedMainEntities' own doc comment.
			if instance == "main" && t.mainEntities[entity] {
				continue
			}
			delete(t.entries[instance], entity)
			t.order[instance] = removeString(t.order[instance], entity)
			fmt.Printf("[existence] %s: pruned %q -- confirmed not-to-exist and no longer declared\n", instance, entity)
		}
	}

	// Self-heal any already-persisted malformed entity id (see DiscoverSiblings' own guard, added
	// 2026-09-14 -- this handles ids that got tracked BEFORE that guard existed, or that lost their
	// current declaration since, e.g. a device rename leaving a stale malformed sibling behind).
	// Unlike the StatusKnownNotToExist prune above, this ignores Status entirely: a malformed id can
	// never resolve either way (HA's states[...]/device_id(...) error out instead of answering), so
	// it would otherwise sit in "not-known-to-exist" and monopolize nextToInquire's backlog slot
	// forever. Still deferring to a current declaration, per Seed's usual "Physical.def is the naming
	// authority" principle -- pruning here only ever removes an orphan, never something re-declared.
	for instance, entities := range t.entries {
		for entity := range entities {
			if declared[instance][entity] || isWellFormedEntityID(entity) {
				continue
			}
			delete(t.entries[instance], entity)
			t.order[instance] = removeString(t.order[instance], entity)
			fmt.Printf("[existence] %s: pruned malformed entity id %q -- can never resolve, no longer declared\n", instance, entity)
		}
	}

	t.persist()
}

// SeedMainEntities registers every declared "bare" main-instance entity (kind-5, PROJECT.md item 1,
// 2026-09-07 -- coordinator/main_entities.yaml, generator-side collectMainEntityIDs) as a
// not-known-to-exist entry for instance "main", unless already tracked -- same "already tracked?
// leave it alone" rule as SeedManualEntity. Also prunes ITS OWN confirmed-dead ghost entries no
// longer declared, mirroring Seed's own pruning exactly but scoped to entries this method owns
// (t.mainEntities) -- never touches a hassbridge-declared entry, the symmetric counterpart to Seed's
// own new exclusion just above.
//
// Populates t.mainEntities as a side effect, which Seed's own pruning loop consults to avoid
// deleting a confirmed-dead main entity bridgeFile never declared in the first place. Call this
// BEFORE Seed(bridgeFile) on every coordinator startup so t.mainEntities is populated before Seed's
// own pruning pass runs that same startup -- ordering is load-bearing, not stylistic.
func (t *TEntityExistenceTracker) SeedMainEntities(entityIDs []string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	declared := map[string]bool{}
	for _, id := range entityIDs {
		declared[id] = true
	}

	if _, ok := t.entries["main"]; !ok {
		t.entries["main"] = map[string]*TEntityExistenceEntry{}
	}
	for _, id := range entityIDs {
		if _, seen := t.entries["main"][id]; seen {
			continue
		}
		t.entries["main"][id] = &TEntityExistenceEntry{Status: StatusNotKnownToExist}
		t.order["main"] = append(t.order["main"], id)
	}

	for entity := range t.mainEntities {
		if declared[entity] {
			continue
		}
		entry, tracked := t.entries["main"][entity]
		if !tracked || entry.Status != StatusKnownNotToExist {
			continue
		}
		delete(t.entries["main"], entity)
		t.order["main"] = removeString(t.order["main"], entity)
		fmt.Printf("[existence] main: pruned %q -- confirmed not-to-exist and no longer declared\n", entity)
	}

	t.mainEntities = declared
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

// validEntityIDPattern mirrors Home Assistant's own homeassistant.core.valid_entity_id() --
// notably its `(?!.+__)` guard rejecting a double underscore anywhere in the id, which HA's
// template `states[...]`/`device_id(...)` lookups enforce too: given a malformed id, they raise
// TemplateError: Invalid entity ID instead of returning None the way a well-formed-but-unknown id
// would. A tracked entity that can never pass this check can therefore never be resolved by
// inquiry at all -- every attempt errors the inquiry automation out before it gets anywhere near
// building a reply -- so it would otherwise monopolize nextToInquire's not-known-to-exist backlog
// slot forever, exactly like the unreduced-source-expression bug bareEntityFromSource already
// fixed (2026-08-28).
var validEntityIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:_[a-z0-9]+)*\.[a-z0-9]+(?:_[a-z0-9]+)*$`)

func isWellFormedEntityID(id string) bool {
	return validEntityIDPattern.MatchString(id)
}

// nextToInquire picks the next source entity to ask instance about: any not-known-to-exist entity
// first, ROTATING through the backlog rather than always starting from the front (backlogCursor --
// see its own doc comment for the real bug this fixes, 2026-09-07); once every entity has a
// resolved status, round-robin through them as a refresh pass (cursor). This applies uniformly
// whether or not the entity has a known owning device -- PROJECT.md 1.1's original design sketched
// a separate third pass just for device-less ("orphan") entities, but registration order and the
// round-robin refresh already cover them exactly like any other entity, so no such separate pass
// was ever needed. Returns "" if instance has nothing tracked.
func (t *TEntityExistenceTracker) nextToInquire(instance string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	byEntity := t.entries[instance]
	order := t.order[instance]
	n := len(order)
	if n == 0 {
		return ""
	}
	start := t.backlogCursor[instance] % n
	for i := 0; i < n; i++ {
		idx := (start + i) % n
		if byEntity[order[idx]].Status == StatusNotKnownToExist {
			t.backlogCursor[instance] = idx + 1
			return order[idx]
		}
	}
	idx := t.cursor[instance] % n
	t.cursor[instance] = idx + 1
	return order[idx]
}

// Record applies one inquiry reply to instance's tracked status for sourceEntity, and returns the
// entry's own owning DeviceID ("" if untracked or the entity has no owning device) so the caller
// (subscribeExistenceReply) can feed reply's own device-info fields into TLiveDeviceInfoStore under
// the right key without a second, separately-locked lookup. A reply about an entity this tracker
// never asked about (stale/unexpected) is ignored, not an error.
//
// remoteDeviceID/remoteViaDeviceID (added 2026-09-14, PROJECT.md item 3/
// plans/via-device-inference.md) record the remote instance's own device_id/via_device_id for
// EVERY tracked entry, not just DiscoverSiblings-synthesized ones as before -- ResolveViaDevice
// needs a real Physical.def-declared device's own RemoteDeviceID populated too, to match another
// device's reported via_device_id against it.
func (t *TEntityExistenceTracker) Record(instance, sourceEntity string, exists bool, state, unit, deviceClass, icon, remoteDeviceID, remoteViaDeviceID string) (deviceID string) {
	t.mu.Lock()
	byEntity, ok := t.entries[instance]
	if !ok {
		t.mu.Unlock()
		return ""
	}
	entry, ok := byEntity[sourceEntity]
	if !ok {
		t.mu.Unlock()
		return ""
	}
	wasMissing := entry.Status == StatusKnownNotToExist
	if exists {
		entry.Status = StatusKnownToExist
		entry.State = state
		entry.Unit = unit
		entry.DeviceClass = deviceClass
		entry.Icon = icon
		if remoteDeviceID != "" {
			entry.RemoteDeviceID = remoteDeviceID
		}
		entry.RemoteViaDeviceID = remoteViaDeviceID
	} else {
		entry.Status = StatusKnownNotToExist
		entry.State = ""
		entry.Unit = ""
		entry.DeviceClass = ""
		entry.Icon = ""
		entry.RemoteViaDeviceID = ""
	}
	t.persist()
	nowMissing := entry.Status == StatusKnownNotToExist
	result := entry.DeviceID
	onChange := t.onChange
	t.mu.Unlock()
	// Only a genuine transition needs to fire onChange -- most Record calls are ordinary
	// known-to-exist confirmations/refreshes that never touched StatusKnownNotToExist at all, and
	// TMissingDeclaredEntitiesPublisher's own aggregate has nothing to update for those.
	if wasMissing != nowMissing && onChange != nil {
		onChange()
	}
	return result
}

// RecordAttributeKeys stores sourceEntity's live-reported HA attribute key set (added 2026-09-21).
// Kept separate from Record deliberately, not a new Record parameter -- Record already has 9
// positional args and ~35 existing test call sites across this package's own tests; a 10th
// parameter would force a purely mechanical edit to every one of them for a field only the
// generator's undeclared-attributes suggestion check (homeassistant/mqtt_entity_existence.go) ever
// reads. Pure live-data cache, same tier as Unit/DeviceClass/Icon -- no persist(), no onChange()
// (this never affects StatusKnownNotToExist, the only thing either of those cares about). A no-op
// (same "unknown instance/entity, ignore" tolerance Record itself has) when instance/sourceEntity
// isn't already tracked.
func (t *TEntityExistenceTracker) RecordAttributeKeys(instance, sourceEntity string, keys []string) {
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
	entry.AttributeKeys = keys
}

// LiveTyping returns instance's own last-reported unit_of_measurement/device_class/icon for
// sourceEntity, "" for any not resolved to known-to-exist, or unknown to this tracker entirely --
// discoveryhassbridge.go's own fallback typing source, behind Physical.def's explicit declaration
// and Defaults.def/code-level rules.
func (t *TEntityExistenceTracker) LiveTyping(instance, sourceEntity string) (unit, deviceClass, icon string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry, ok := t.entries[instance][sourceEntity]
	if !ok {
		return "", "", ""
	}
	return entry.Unit, entry.DeviceClass, entry.Icon
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
		if !isWellFormedEntityID(sibling) {
			// Real bug found live 2026-09-14 (PROJECT.md item 3): HA's own device_entities()
			// reply carried a malformed double-underscore id for this device at some point in
			// the past (root incident lost -- entity_existence.json is deliberately persisted
			// across restarts, see this file's own header comment); tracking it would monopolize
			// nextToInquire's backlog slot forever, since HA's states[...]/device_id(...) reject
			// it outright rather than returning "doesn't exist".
			fmt.Printf("[existence] %s: ignoring malformed sibling entity id %q (via %s, device %q)\n", instance, sibling, anchorEntity, anchor.DeviceID)
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

// ResolveViaDevice reports the local DeviceID deviceID's own via_device should point at, within
// instance -- PROJECT.md item 3/plans/via-device-inference.md's same-house inference: deviceID
// depends on another device this same instance ALSO advertises, purely from data the paced
// inquiry loop already collected (no new Physical.def syntax). Deliberately one hop only, matching
// discoverybridge.go's own documented kind-2 scope -- a target that is itself "via" a third device
// is not chased further.
//
// Two lookups, both plain scans over instance's own tracked entries (small, instance-scoped maps
// -- same style resolveSyntheticDeviceID already uses for its own disambiguation check):
//  1. find deviceID's own reported RemoteViaDeviceID (any of its entries -- they all belong to the
//     same device, so the first non-empty hit is enough, same reasoning snapshotByDevice's own
//     grouping relies on).
//  2. find another entry, anywhere in this instance, whose own RemoteDeviceID equals that --
//     that entry's DeviceID is the resolved target.
//
// ok is false if deviceID reported no via_device_id, or if the target it named isn't (yet, or
// ever) itself a tracked device in this same instance -- a correct, unremarkable outcome (nothing
// to link to), not an error.
// ownSourceEntities is deviceID's own declared source entity_ids for instance (any capability's
// SourceEntities[instance], e.g. from THassBridgeDevice.Capabilities) -- looked up directly by
// entity_id rather than via the tracker's own DeviceID attribution, deliberately. Found live
// 2026-09-14: two devices can legitimately declare the SAME source entity as their own capability
// (node.vienna_bedroom and node.vienna_livingroom both declare "binary_sensor.vienna_bedroom_
// connectivity" as their own "node", a deliberate Physical.def workaround for a flaky dedicated
// living-room connectivity sensor) -- the tracker's own entry.DeviceID can only ever hold ONE
// owner for a shared entity (Seed's own correction, sorted for determinism but still an arbitrary
// pick between two equally-legitimate claimants), so keying this lookup off entry.DeviceID would
// silently and non-deterministically fail for whichever device didn't "win" the shared entity that
// restart. Physical.def's own declaration is the authoritative source for "is this entity mine",
// so use it directly instead.
func (t *TEntityExistenceTracker) ResolveViaDevice(instance, deviceID string, ownSourceEntities []string) (targetDeviceID string, ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	byEntity, exists := t.entries[instance]
	if !exists {
		return "", false
	}

	remoteViaDeviceID := ""
	for _, sourceEntity := range ownSourceEntities {
		if entry, tracked := byEntity[sourceEntity]; tracked && entry.RemoteViaDeviceID != "" {
			remoteViaDeviceID = entry.RemoteViaDeviceID
			break
		}
	}
	if remoteViaDeviceID == "" {
		return "", false
	}

	for _, entry := range byEntity {
		if entry.DeviceID != "" && entry.DeviceID != deviceID && entry.RemoteDeviceID == remoteViaDeviceID {
			return entry.DeviceID, true
		}
	}
	return "", false
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
	Icon            string   `json:"icon"`
	DeviceID        string   `json:"device_id"`
	DeviceName      string   `json:"device_name"`
	Manufacturer    string   `json:"manufacturer"`
	Model           string   `json:"model"`
	ModelID         string   `json:"model_id"`
	SwVersion       string   `json:"sw_version"`
	HwVersion       string   `json:"hw_version"`
	SerialNumber    string   `json:"serial_number"`
	ViaDeviceID     string   `json:"via_device_id"`
	SiblingEntities []string `json:"sibling_entities"`
	// AttributeKeys (added 2026-09-21) is the source entity's own live-reported HA attribute key
	// set -- see TEntityExistenceEntry.AttributeKeys' own doc comment.
	AttributeKeys []string `json:"attribute_keys"`
}

// deviceInfoFields returns reply's own manufacturer/model/... fields as a knownLiveDeviceInfoFields-
// keyed map (discovery.go), omitting anything empty -- the shape TLiveDeviceInfoStore.Update and
// buildHassBridgeDeviceBlock's forced-DSL > live > DSL precedence already expect. Returns an empty,
// non-nil map if reply carries none of these (a remote entity with no owning device, or an older
// remote automation that predates this reply shape).
func (reply existenceInquiryReply) deviceInfoFields() map[string]string {
	fields := map[string]string{}
	for name, value := range map[string]string{
		"manufacturer":  reply.Manufacturer,
		"model":         reply.Model,
		"model_id":      reply.ModelID,
		"sw_version":    reply.SwVersion,
		"hw_version":    reply.HwVersion,
		"serial_number": reply.SerialNumber,
	} {
		if value != "" {
			fields[name] = value
		}
	}
	return fields
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
	// AttributeKeys (added 2026-09-21) is the source entity's own live-reported HA attribute key
	// set -- see TEntityExistenceEntry.AttributeKeys' own doc comment. Omitted when empty, same
	// convention State already follows.
	AttributeKeys []string `json:"attribute_keys,omitempty"`
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
			entityPayloads[entity] = existenceStatusEntityPayload{Status: string(entry.Status), State: entry.State, AttributeKeys: entry.AttributeKeys}
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
// store is nil-safe (tests exercising this without a store) -- a confirmed-existing reply carrying
// any manufacturer/model/... fields feeds them into store under the entity's own tracked DeviceID,
// so discoveryhassbridge.go's buildHassBridgeDeviceBlock picks them up through the same
// forced-DSL > live > DSL precedence a hosts device's own dynamic fields already use.
func (t *TEntityExistenceTracker) subscribeExistenceReply(mainClient, cloudClient mqtt.Client, ownInstallation, instance string, store *TLiveDeviceInfoStore) error {
	handler := func(_ mqtt.Client, msg mqtt.Message) {
		var reply existenceInquiryReply
		if err := json.Unmarshal(msg.Payload(), &reply); err != nil {
			fmt.Printf("[existence] %s: cannot parse inquiry reply: %v\n", instance, err)
			return
		}
		deviceID := t.Record(instance, reply.EntityID, reply.Exists, reply.State, reply.Unit, reply.DeviceClass, reply.Icon, reply.DeviceID, reply.ViaDeviceID)
		if reply.Exists {
			t.RecordAttributeKeys(instance, reply.EntityID, reply.AttributeKeys)
		}
		fmt.Printf("[existence] %s: %s -> exists=%v\n", instance, reply.EntityID, reply.Exists)
		if reply.Exists && len(reply.SiblingEntities) > 0 {
			t.DiscoverSiblings(instance, reply.EntityID, reply.DeviceID, reply.DeviceName, reply.SiblingEntities)
		}
		if store != nil && deviceID != "" {
			if fields := reply.deviceInfoFields(); len(fields) > 0 {
				store.Update(deviceID, fields)
			}
			// via_device is deliberately never stored in TLiveDeviceInfoStore (a cross-device
			// lookup, not a per-device live value -- ResolveViaDevice's own doc comment), but a
			// newly-resolved target still needs the exact same "republish this device's discovery
			// config" reaction store.Update's onChange already gives manufacturer/model -- Notify
			// gives it that without storing anything. This handler has no THassBridgeDevice in
			// scope to call ResolveViaDevice itself (it's generic across every tracked kind, not
			// hassbridge-specific) -- notifying whenever this reply reports EITHER half of a
			// via_device relationship (its own device_id, which some other device's via_device_id
			// might target, or its own via_device_id, which might now resolve) is a deliberately
			// loose trigger: buildHassBridgeDeviceBlock's own callers do the real, precise
			// resolution at publish time, so an occasional redundant republish here is harmless,
			// matching this file's own existing tolerance for unconditional per-reply republishing
			// (publishStatus, below, already does the same).
			if reply.DeviceID != "" || reply.ViaDeviceID != "" {
				store.Notify(deviceID)
			}
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
func (t *TEntityExistenceTracker) StartEntityExistenceInquiries(mainClient, cloudClient mqtt.Client, ownInstallation string, instances []string, store *TLiveDeviceInfoStore) error {
	if len(instances) == 0 {
		return nil
	}
	for _, instance := range instances {
		if err := t.subscribeExistenceReply(mainClient, cloudClient, ownInstallation, instance, store); err != nil {
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
