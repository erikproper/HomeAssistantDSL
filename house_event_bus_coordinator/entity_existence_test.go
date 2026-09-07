package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

// findPublish locates a specific topic among a fakeClient's recorded publishes, since a test may
// trigger more than one publish (e.g. an inquiry publish and a status publish) and needs to check
// a specific one rather than assume ordering.
func findPublish(published []recordedPublish, topic string) (recordedPublish, bool) {
	for _, p := range published {
		if p.topic == topic {
			return p, true
		}
	}
	return recordedPublish{}, false
}

func fixtureBridgeFileForExistence() THassBridgeFile {
	return THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.davids_bedroom": {
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"co2":      {SourceEntities: map[string]string{"protocols-server-2": "sensor.davids_bedroom_carbon_dioxide"}, LocalEntity: "sensor.physical_david_bedroom_netatmo_co2"},
				"humidity": {SourceEntities: map[string]string{"protocols-server-2": "sensor.davids_bedroom_humidity"}, LocalEntity: "sensor.physical_david_bedroom_netatmo_humidity"},
			},
		},
	}}
}

func TestBareEntityFromSource(t *testing.T) {
	cases := []struct{ src, want string }{
		{"sensor.processor_use is available", "sensor.processor_use"},
		{"update.foo!installed_version", "update.foo"},
		{"sensor.plain_entity", "sensor.plain_entity"},
	}
	for _, c := range cases {
		if got := bareEntityFromSource(c.src); got != c.want {
			t.Errorf("bareEntityFromSource(%q) = %q, want %q", c.src, got, c.want)
		}
	}
}

// TestEntityExistenceTrackerSeedStripsIsAvailableSugar is the regression test for a real bug
// confirmed live 2026-08-28 via the coordinator's own journal: Seed used to track
// "sensor.processor_use is available" verbatim (a "node" capability's raw source, DSL sugar, not
// a real entity_id) -- states[...] can never resolve that literal string, so it permanently
// occupied "main"'s top-priority not-known-to-exist slot every tick, starving
// sensor.processor_temperature (a sibling capability on the same device) from ever being reached.
func TestEntityExistenceTrackerSeedStripsIsAvailableSugar(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.Seed(THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.junglinster": {
			Instances: []string{"main"},
			Capabilities: map[string]THassBridgeCapability{
				"node":            {SourceEntities: map[string]string{"main": "sensor.processor_use is available"}},
				"cpu/temperature": {SourceEntities: map[string]string{"main": "sensor.processor_temperature"}},
			},
		},
	}})

	byDevice := tracker.snapshotByDevice("main")
	entities := byDevice["hass.junglinster"]
	if _, found := entities["sensor.processor_use is available"]; found {
		t.Errorf("tracked the raw sugared source verbatim, want it stripped to the bare entity_id: %+v", entities)
	}
	if _, found := entities["sensor.processor_use"]; !found {
		t.Errorf("expected the bare entity_id to be tracked, got %+v", entities)
	}
	if _, found := entities["sensor.processor_temperature"]; !found {
		t.Errorf("expected the sibling capability to be tracked too, got %+v", entities)
	}
}

// TestEntityExistenceTrackerSeedPrunesConfirmedDeadOrphans is the regression test for a real
// operational annoyance found live 2026-09-05: a Physical.def typo
// (sensor.living_room__battery, double underscore) gets confirmed known-not-to-exist, then fixed
// in Physical.def -- but the old, now-undeclared ghost entry stayed in persisted state forever,
// permanently consuming a "recheck known" round-robin slot even though nothing references it any
// more. Seed must delete a (instance, entity) pair once it's both confirmed not-to-exist AND no
// longer declared -- but must never touch a not-yet-resolved or known-to-exist entry, since those
// are exactly the shape a DiscoverSiblings-discovered sibling or an in-progress manual
// "Discover entity" seed has (neither is ever bridgeFile-declared either).
func TestEntityExistenceTrackerSeedPrunesConfirmedDeadOrphans(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.Seed(THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.living_room_terrace": {
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"battery": {SourceEntities: map[string]string{"protocols-server-2": "sensor.living_room__battery"}},
			},
		},
	}})
	tracker.Record("protocols-server-2", "sensor.living_room__battery", false, "", "", "")

	// A sibling discovered independently of any Physical.def declaration, and a manually-typed
	// "Discover entity" seed -- neither ever appears in bridgeFile, both must survive re-seeding
	// since neither is known-not-to-exist.
	tracker.DiscoverSiblings("protocols-server-2", "sensor.living_room__battery", "remote-device-1", "Living Room Terrace", []string{"sensor.living_room_terrace_uptime"})
	tracker.SeedManualEntity("protocols-server-2", "sensor.manually_typed_candidate")

	// Physical.def gets fixed: the typo'd capability now points at the correct entity_id, so the
	// old one no longer appears anywhere in bridgeFile.
	tracker.Seed(THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.living_room_terrace": {
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"battery": {SourceEntities: map[string]string{"protocols-server-2": "sensor.living_room_battery"}},
			},
		},
	}})

	byDevice := tracker.snapshotByDevice("protocols-server-2")
	if _, found := byDevice["hass.living_room_terrace"]["sensor.living_room__battery"]; found {
		t.Errorf("confirmed-dead, no-longer-declared ghost entry was not pruned: %+v", byDevice)
	}
	if _, found := byDevice["hass.living_room_terrace"]["sensor.living_room_battery"]; !found {
		t.Errorf("expected the corrected entity_id to be freshly tracked, got %+v", byDevice)
	}
	if _, found := byDevice["hass.living_room_terrace"]["sensor.living_room_terrace_uptime"]; !found {
		t.Errorf("a not-yet-resolved sibling must survive re-seeding even though it's never bridgeFile-declared, got %+v", byDevice)
	}
	if _, found := byDevice[""]["sensor.manually_typed_candidate"]; !found {
		t.Errorf("an in-progress manual seed must survive re-seeding even though it's never bridgeFile-declared, got %+v", byDevice)
	}
}

// TestEntityExistenceTrackerSeedNeverPrunesStillDeclaredEntries guards against over-pruning: a
// capability that's STILL declared must keep its known-not-to-exist status across re-seeding --
// this is the exact status checkKnownNotToExistErrors (generator side) depends on to keep flagging
// a genuinely still-broken declaration, so it must never be silently pruned away.
func TestEntityExistenceTrackerSeedNeverPrunesStillDeclaredEntries(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	bridgeFile := THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.living_room_terrace": {
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"battery": {SourceEntities: map[string]string{"protocols-server-2": "sensor.still_broken"}},
			},
		},
	}}
	tracker.Seed(bridgeFile)
	tracker.Record("protocols-server-2", "sensor.still_broken", false, "", "", "")

	tracker.Seed(bridgeFile)

	byDevice := tracker.snapshotByDevice("protocols-server-2")
	entry, found := byDevice["hass.living_room_terrace"]["sensor.still_broken"]
	if !found || entry.Status != StatusKnownNotToExist {
		t.Errorf("a still-declared known-not-to-exist entry must never be pruned, got %+v (found=%v)", entry, found)
	}
}

// TestEntityExistenceTrackerSeedMainEntities confirms a fresh SeedMainEntities call creates
// not-known-to-exist entries for instance "main", and a second call leaves an already-tracked
// (and since-confirmed) one untouched rather than resetting its status.
func TestEntityExistenceTrackerSeedMainEntities(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.SeedMainEntities([]string{"sensor.physical_door_aqara_multi_temperature", "sensor.already_known"})

	byDevice := tracker.snapshotByDevice("main")
	entry, found := byDevice[""]["sensor.physical_door_aqara_multi_temperature"]
	if !found || entry.Status != StatusNotKnownToExist {
		t.Errorf("expected a fresh not-known-to-exist entry, got %+v (found=%v)", entry, found)
	}

	tracker.Record("main", "sensor.already_known", true, "21.0", "°C", "temperature")
	tracker.SeedMainEntities([]string{"sensor.physical_door_aqara_multi_temperature", "sensor.already_known"})

	byDevice = tracker.snapshotByDevice("main")
	known, found := byDevice[""]["sensor.already_known"]
	if !found || known.Status != StatusKnownToExist {
		t.Errorf("expected the already-tracked, since-confirmed entry to survive a re-seed untouched, got %+v (found=%v)", known, found)
	}
}

// TestEntityExistenceTrackerSeedMainEntitiesPrunesOwnGhosts confirms a confirmed-dead main entity
// dropped from a later SeedMainEntities call (its own declaration removed from Spaces.def) is
// removed, mirroring Seed's own pruning behaviour for kind-3.
func TestEntityExistenceTrackerSeedMainEntitiesPrunesOwnGhosts(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.SeedMainEntities([]string{"sensor.removed_from_spaces_def"})
	tracker.Record("main", "sensor.removed_from_spaces_def", false, "", "", "")

	tracker.SeedMainEntities(nil)

	byDevice := tracker.snapshotByDevice("main")
	if _, found := byDevice[""]["sensor.removed_from_spaces_def"]; found {
		t.Errorf("confirmed-dead, no-longer-declared main entity was not pruned: %+v", byDevice)
	}
}

// TestEntityExistenceTrackerSeedNeverPrunesMainEntities is the regression test for a real bug
// found during design review (2026-09-07): Seed(bridgeFile)'s own pruning has no visibility into
// kind-5's own main-instance entities at all, so without the mainEntities protected-set check it
// would silently wipe a confirmed-dead bare entity's status on every coordinator restart --
// re-opening the "not yet checked" window for hours (see entity_existence.go's own doc comments on
// Seed/SeedMainEntities for the full mutual-protection contract).
func TestEntityExistenceTrackerSeedNeverPrunesMainEntities(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.SeedMainEntities([]string{"sensor.physical_door_aqara_multi_temperature"})
	tracker.Record("main", "sensor.physical_door_aqara_multi_temperature", false, "", "", "")

	// A bridgeFile that never mentions this entity at all -- exactly what every real coordinator
	// restart looks like for a kind-5 entity, since Seed only ever knows about hassbridge devices.
	tracker.Seed(THassBridgeFile{})

	byDevice := tracker.snapshotByDevice("main")
	entry, found := byDevice[""]["sensor.physical_door_aqara_multi_temperature"]
	if !found || entry.Status != StatusKnownNotToExist {
		t.Errorf("a confirmed-dead main entity must survive Seed(bridgeFile) re-seeding even though bridgeFile never declares it, got %+v (found=%v)", entry, found)
	}
}

func TestEntityExistenceTrackerSeedManualEntity(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.SeedManualEntity("main", "sensor.newly_typed_in")

	byDevice := tracker.snapshotByDevice("main")
	entry, found := byDevice[""]["sensor.newly_typed_in"]
	if !found {
		t.Fatalf("expected the manually seeded entity to be tracked (ungrouped), got %+v", byDevice)
	}
	if entry.Status != StatusNotKnownToExist {
		t.Errorf("status = %q, want %q", entry.Status, StatusNotKnownToExist)
	}

	// Re-seeding an already-known entity must not reset a status it has already resolved to.
	tracker.Record("main", "sensor.newly_typed_in", true, "42", "", "")
	tracker.SeedManualEntity("main", "sensor.newly_typed_in")
	byDevice = tracker.snapshotByDevice("main")
	if byDevice[""]["sensor.newly_typed_in"].Status != StatusKnownToExist {
		t.Errorf("re-seeding reset an already-resolved status, got %+v", byDevice[""]["sensor.newly_typed_in"])
	}
}

func TestEntityExistenceTrackerInquireNowBypassesPacing(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	client := &fakeClient{}

	// A normal maybeInquire call establishes pacing for "main".
	tracker.Seed(THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.junglinster": {Instances: []string{"main"}, Capabilities: map[string]THassBridgeCapability{
			"cpu/load": {SourceEntities: map[string]string{"main": "sensor.processor_use"}},
		}},
	}})
	tracker.maybeInquire(client, "main")
	firstCount := len(client.published)

	// InquireNow must still publish immediately, even though "main" isn't due again yet.
	tracker.InquireNow(client, "main", "sensor.manually_requested")
	if len(client.published) != firstCount+1 {
		t.Fatalf("InquireNow did not publish despite pacing, got %d publishes (was %d)", len(client.published), firstCount)
	}
	last := client.published[len(client.published)-1]
	if last.topic != existenceInquiryTopic("main") || string(last.payload) != "sensor.manually_requested" {
		t.Errorf("expected the manually requested entity's own inquiry publish last, got %+v", client.published)
	}

	// And the automatic loop's own pacing is still respected afterwards -- InquireNow updates
	// lastSent too, so maybeInquire must not fire again immediately.
	beforeAutomatic := len(client.published)
	tracker.maybeInquire(client, "main")
	if len(client.published) != beforeAutomatic {
		t.Errorf("maybeInquire fired again right after InquireNow, want it still paced")
	}
}

func TestEntityExistenceTrackerSeedThenNextToInquirePrefersUnknown(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.Seed(fixtureBridgeFileForExistence())

	first := tracker.nextToInquire("protocols-server-2")
	if first != "sensor.davids_bedroom_carbon_dioxide" && first != "sensor.davids_bedroom_humidity" {
		t.Fatalf("nextToInquire = %q, want one of the two seeded (not-known-to-exist) entities", first)
	}

	// Resolve both; nextToInquire must now fall back to round-robin over known entities, not
	// return "" and not get stuck.
	tracker.Record("protocols-server-2", "sensor.davids_bedroom_carbon_dioxide", true, "412.3", "", "")
	tracker.Record("protocols-server-2", "sensor.davids_bedroom_humidity", false, "", "", "")

	seen := map[string]bool{}
	for i := 0; i < 4; i++ {
		seen[tracker.nextToInquire("protocols-server-2")] = true
	}
	if len(seen) != 2 {
		t.Errorf("round-robin refresh saw %v, want both entities cycled through", seen)
	}
}

func TestEntityExistenceTrackerNextToInquireEmptyInstance(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	if got := tracker.nextToInquire("nothing-tracked"); got != "" {
		t.Errorf("nextToInquire on an untracked instance = %q, want \"\"", got)
	}
}

func TestEntityExistenceTrackerRecordGroupsByDevice(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.Seed(fixtureBridgeFileForExistence())
	tracker.Record("protocols-server-2", "sensor.davids_bedroom_carbon_dioxide", true, "412.3", "", "")
	tracker.Record("protocols-server-2", "sensor.davids_bedroom_humidity", false, "", "", "")

	byDevice := tracker.snapshotByDevice("protocols-server-2")
	entities, ok := byDevice["hass.davids_bedroom"]
	if !ok {
		t.Fatalf("snapshotByDevice = %+v, want an entry for hass.davids_bedroom", byDevice)
	}
	if entities["sensor.davids_bedroom_carbon_dioxide"].Status != StatusKnownToExist {
		t.Errorf("co2 status = %q, want %q", entities["sensor.davids_bedroom_carbon_dioxide"].Status, StatusKnownToExist)
	}
	if entities["sensor.davids_bedroom_humidity"].Status != StatusKnownNotToExist {
		t.Errorf("humidity status = %q, want %q", entities["sensor.davids_bedroom_humidity"].Status, StatusKnownNotToExist)
	}
}

// TestRecordStoresLiveTypingOnlyWhenKnownToExist covers the 2026-09-06 addition: unit_of_measurement/
// device_class piggyback on the existing inquiry reply, so LiveTyping can serve as a fallback
// typing source (discoveryhassbridge.go) when Physical.def/Defaults.def set nothing. A
// known-not-to-exist entity must never carry stale typing from an earlier known-to-exist state.
func TestRecordStoresLiveTypingOnlyWhenKnownToExist(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.Seed(fixtureBridgeFileForExistence())

	tracker.Record("protocols-server-2", "sensor.davids_bedroom_carbon_dioxide", true, "412.3", "ppm", "carbon_dioxide")
	if unit, deviceClass := tracker.LiveTyping("protocols-server-2", "sensor.davids_bedroom_carbon_dioxide"); unit != "ppm" || deviceClass != "carbon_dioxide" {
		t.Errorf("LiveTyping = (%q, %q), want (\"ppm\", \"carbon_dioxide\")", unit, deviceClass)
	}

	tracker.Record("protocols-server-2", "sensor.davids_bedroom_humidity", false, "", "should-not-be-stored", "should-not-be-stored")
	if unit, deviceClass := tracker.LiveTyping("protocols-server-2", "sensor.davids_bedroom_humidity"); unit != "" || deviceClass != "" {
		t.Errorf("LiveTyping = (%q, %q), want (\"\", \"\") for a known-not-to-exist entity", unit, deviceClass)
	}

	if unit, deviceClass := tracker.LiveTyping("protocols-server-2", "sensor.never_tracked_at_all"); unit != "" || deviceClass != "" {
		t.Errorf("LiveTyping = (%q, %q), want (\"\", \"\") for an entity this tracker has never heard of", unit, deviceClass)
	}
}

func TestEntityExistenceTrackerDiscoverSiblingsAddsNewEntitiesUnderSameDevice(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.Seed(fixtureBridgeFileForExistence())
	tracker.Record("protocols-server-2", "sensor.davids_bedroom_carbon_dioxide", true, "412.3", "", "")

	tracker.DiscoverSiblings("protocols-server-2", "sensor.davids_bedroom_carbon_dioxide", "remote-device-davids-bedroom", "David's Bedroom", []string{
		"sensor.davids_bedroom_carbon_dioxide",        // the anchor itself -- must not duplicate
		"sensor.davids_bedroom_humidity",              // already seeded -- must not clobber its own status
		"binary_sensor.davids_bedroom_pressure_alert", // genuinely new
	})

	byDevice := tracker.snapshotByDevice("protocols-server-2")
	entities := byDevice["hass.davids_bedroom"]

	discovered, ok := entities["binary_sensor.davids_bedroom_pressure_alert"]
	if !ok {
		t.Fatalf("expected the new sibling to be discovered, got %+v", entities)
	}
	if discovered.Status != StatusKnownToExist {
		t.Errorf("discovered sibling status = %q, want %q -- device_entities() only ever returns real entities", discovered.Status, StatusKnownToExist)
	}
	if discovered.DeviceID != "hass.davids_bedroom" {
		t.Errorf("discovered sibling DeviceID = %q, want it attributed to the anchor's own device", discovered.DeviceID)
	}

	// The already-seeded humidity entity keeps its own (still not-known-to-exist) status --
	// DiscoverSiblings must not silently promote entities it didn't itself confirm.
	if entities["sensor.davids_bedroom_humidity"].Status != StatusNotKnownToExist {
		t.Errorf("humidity status = %q, want unchanged (%q)", entities["sensor.davids_bedroom_humidity"].Status, StatusNotKnownToExist)
	}
}

func TestEntityExistenceTrackerDiscoverSiblingsNoopWhenAnchorUntracked(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.DiscoverSiblings("protocols-server-2", "sensor.never_seeded", "remote-device-x", "", []string{"sensor.something"})
	byDevice := tracker.snapshotByDevice("protocols-server-2")
	if len(byDevice) != 0 {
		t.Errorf("expected no entries created when the anchor itself isn't tracked, got %+v", byDevice)
	}
}

// TestEntityExistenceTrackerDiscoverSiblingsDisambiguatesSameNameDifferentRemoteDevice is the
// regression test for a real bug caught live 2026-08-29: two genuinely separate Netatmo modules
// (an "Outdoor Module" and a "Wind Gauge") at the same location both auto-named "Vienna Terrace"
// by Netatmo/HA -- their entities were wrongly merged into one synthetic device, since the old
// syntheticDeviceID keyed purely off the (non-unique) display name. Two anchors with the same
// deviceName but different remoteDeviceID must get two different synthetic ids.
func TestEntityExistenceTrackerDiscoverSiblingsDisambiguatesSameNameDifferentRemoteDevice(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.SeedManualEntity("protocols-server-2", "sensor.vienna_terrace_wind_direction")
	tracker.Record("protocols-server-2", "sensor.vienna_terrace_wind_direction", true, "180", "", "")
	tracker.SeedManualEntity("protocols-server-2", "sensor.vienna_terrace_humidity")
	tracker.Record("protocols-server-2", "sensor.vienna_terrace_humidity", true, "55", "", "")

	tracker.DiscoverSiblings("protocols-server-2", "sensor.vienna_terrace_wind_direction", "remote-wind-module", "Vienna Terrace", []string{
		"sensor.vienna_terrace_wind_speed",
		"binary_sensor.vienna_terrace_connectivity_2",
	})
	tracker.DiscoverSiblings("protocols-server-2", "sensor.vienna_terrace_humidity", "remote-outdoor-module", "Vienna Terrace", []string{
		"sensor.vienna_terrace_temperature",
		"binary_sensor.vienna_terrace_connectivity",
	})

	byDevice := tracker.snapshotByDevice("protocols-server-2")
	var windDeviceID, humidityDeviceID string
	for deviceID, entities := range byDevice {
		if _, found := entities["sensor.vienna_terrace_wind_speed"]; found {
			windDeviceID = deviceID
		}
		if _, found := entities["sensor.vienna_terrace_temperature"]; found {
			humidityDeviceID = deviceID
		}
	}
	if windDeviceID == "" || humidityDeviceID == "" {
		t.Fatalf("expected both anchors' siblings to be grouped somewhere, got %+v", byDevice)
	}
	if windDeviceID == humidityDeviceID {
		t.Errorf("two genuinely different remote devices sharing a display name collapsed into one synthetic device %q, got %+v", windDeviceID, byDevice)
	}
	// device_entities() for the wind module must never include the humidity module's own entities,
	// and vice versa -- confirming they're kept fully separate, not just differently labelled.
	if _, found := byDevice[windDeviceID]["sensor.vienna_terrace_temperature"]; found {
		t.Errorf("wind module's group wrongly contains the humidity module's own entity, got %+v", byDevice[windDeviceID])
	}
	if _, found := byDevice[humidityDeviceID]["sensor.vienna_terrace_wind_speed"]; found {
		t.Errorf("humidity module's group wrongly contains the wind module's own entity, got %+v", byDevice[humidityDeviceID])
	}
}

// TestEntityExistenceTrackerDiscoverSiblingsMintsSyntheticDeviceForManuallySeededAnchor is the
// regression test for the gap the user flagged live 2026-08-28: an entity seeded via
// discover_entity_input.go's manual bootstrap starts with DeviceID "" (unknown), so its discovered
// siblings used to land ungrouped in the suggestion report even though the remote instance's own
// device_entities()/device_attr() replies prove they share a real device. DiscoverSiblings must
// now mint a stable synthetic id and group the anchor and its siblings under it.
func TestEntityExistenceTrackerDiscoverSiblingsMintsSyntheticDeviceForManuallySeededAnchor(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.SeedManualEntity("protocols-server-2", "sensor.office_storage_carbon_dioxide")
	tracker.Record("protocols-server-2", "sensor.office_storage_carbon_dioxide", true, "612", "", "")

	tracker.DiscoverSiblings("protocols-server-2", "sensor.office_storage_carbon_dioxide", "remote-device-office-storage", "Office Storage", []string{
		"sensor.office_storage_humidity",
		"sensor.office_storage_temperature",
	})

	byDevice := tracker.snapshotByDevice("protocols-server-2")
	if _, stillUngrouped := byDevice[""]; stillUngrouped {
		t.Errorf("expected nothing left ungrouped, got %+v", byDevice)
	}

	var syntheticID string
	for deviceID := range byDevice {
		if deviceID != "" {
			syntheticID = deviceID
		}
	}
	if syntheticID == "" {
		t.Fatalf("expected a synthetic device grouping, got %+v", byDevice)
	}
	entities := byDevice[syntheticID]
	for _, entity := range []string{"sensor.office_storage_carbon_dioxide", "sensor.office_storage_humidity", "sensor.office_storage_temperature"} {
		if _, found := entities[entity]; !found {
			t.Errorf("expected %s grouped under synthetic device %q, got %+v", entity, syntheticID, entities)
		}
	}

	// A second discovery round (a later inquiry reply) must reuse the same synthetic id rather
	// than minting a new one each time -- the anchor's DeviceID is no longer "" after the first
	// round, so it must stick.
	tracker.DiscoverSiblings("protocols-server-2", "sensor.office_storage_carbon_dioxide", "remote-device-office-storage", "Office Storage", []string{
		"binary_sensor.office_storage_connectivity",
	})
	byDevice = tracker.snapshotByDevice("protocols-server-2")
	if _, found := byDevice[syntheticID]["binary_sensor.office_storage_connectivity"]; !found {
		t.Errorf("expected the second discovery round to reuse the same synthetic device id %q, got %+v", syntheticID, byDevice)
	}
}

func TestSlugify(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Office Storage", "office_storage"},
		{"David's Bedroom", "david_s_bedroom"},
		{"  leading/trailing  ", "leading_trailing"},
		{"sensor.office_storage_carbon_dioxide", "sensor_office_storage_carbon_dioxide"},
	}
	for _, c := range cases {
		if got := slugify(c.in); got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestResolveSyntheticDeviceID(t *testing.T) {
	byEntity := map[string]*TEntityExistenceEntry{
		"sensor.vienna_terrace_wind_speed": {DeviceID: "hass.discovered_vienna_terrace", RemoteDeviceID: "remote-wind-module"},
	}
	// Same remote device_id as an already-tracked entry -- must reuse its id exactly.
	if got := resolveSyntheticDeviceID(byEntity, "remote-wind-module", "Vienna Terrace", "sensor.x"); got != "hass.discovered_vienna_terrace" {
		t.Errorf("resolveSyntheticDeviceID (same remote) = %q, want reuse of the existing id", got)
	}
	// Different remote device_id, same display name -- must disambiguate, not collide.
	got := resolveSyntheticDeviceID(byEntity, "remote-outdoor-module", "Vienna Terrace", "sensor.y")
	if got == "hass.discovered_vienna_terrace" {
		t.Errorf("resolveSyntheticDeviceID (different remote, same name) = %q, want a disambiguated id, not a collision", got)
	}
	if got != "hass.discovered_vienna_terrace_2" {
		t.Errorf("resolveSyntheticDeviceID (different remote, same name) = %q, want \"hass.discovered_vienna_terrace_2\"", got)
	}
	// A genuinely new name with no collision at all -- no disambiguation needed.
	if got := resolveSyntheticDeviceID(byEntity, "remote-other", "Office Storage", "sensor.z"); got != "hass.discovered_office_storage" {
		t.Errorf("resolveSyntheticDeviceID (no collision) = %q, want \"hass.discovered_office_storage\"", got)
	}
}

func TestEntityExistenceTrackerRecordIgnoresUnknownReply(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.Seed(fixtureBridgeFileForExistence())
	// A reply about an entity/instance never seeded must not panic or create a phantom entry.
	tracker.Record("protocols-server-2", "sensor.never_asked_about", true, "1", "", "")
	tracker.Record("some-other-instance", "sensor.davids_bedroom_carbon_dioxide", true, "1", "", "")

	byDevice := tracker.snapshotByDevice("protocols-server-2")
	if _, found := byDevice["hass.davids_bedroom"]["sensor.never_asked_about"]; found {
		t.Errorf("unexpected phantom entry created from an unrecognised reply")
	}
}

func TestEntityExistenceTrackerDueForInquiryPacesPerInstance(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	now := time.Now()

	if !tracker.dueForInquiry("protocols-server-2", now) {
		t.Fatalf("first inquiry should always be due")
	}
	if tracker.dueForInquiry("protocols-server-2", now.Add(10*time.Second)) {
		t.Errorf("a second inquiry within the same minute must not be due")
	}
	// A different instance has its own independent budget.
	if !tracker.dueForInquiry("main", now.Add(10*time.Second)) {
		t.Errorf("a different instance must not be gated by another instance's pacing")
	}
	if !tracker.dueForInquiry("protocols-server-2", now.Add(61*time.Second)) {
		t.Errorf("after existenceMinInterval has elapsed, the next inquiry should be due again")
	}
}

func TestSubscribeExistenceReplyRecordsAndPublishesStatus(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.Seed(fixtureBridgeFileForExistence())

	client := &fakeClient{}
	if err := tracker.subscribeExistenceReply(client, nil, "junglinster", "protocols-server-2"); err != nil {
		t.Fatalf("subscribeExistenceReply error: %v", err)
	}
	if len(client.subscribedHandlers) != 1 {
		t.Fatalf("got %d subscriptions, want 1", len(client.subscribedHandlers))
	}

	reply := existenceInquiryReply{EntityID: "sensor.davids_bedroom_carbon_dioxide", Exists: true, State: "412.3"}
	payload, _ := json.Marshal(reply)
	client.subscribedHandlers[0](client, fakeMessage{topic: existenceReplyTopic("protocols-server-2"), payload: payload})

	publish, found := findPublish(client.published, existenceStatusTopic("protocols-server-2"))
	if !found {
		t.Fatalf("published = %+v, want a status publish to %q", client.published, existenceStatusTopic("protocols-server-2"))
	}

	var status map[string]existenceStatusDevicePayload
	if err := json.Unmarshal(publish.payload, &status); err != nil {
		t.Fatalf("status payload is not valid JSON: %v (%s)", err, publish.payload)
	}
	entity, ok := status["hass.davids_bedroom"].Entities["sensor.davids_bedroom_carbon_dioxide"]
	if !ok || entity.Status != string(StatusKnownToExist) || entity.State != "412.3" {
		t.Errorf("status = %+v, want co2 marked known-to-exist with state 412.3", status)
	}
}

// TestSubscribeExistenceReplyRelaysStatusToCloudUnconditionally mirrors entity_catalogue.go's own
// "relay to local and cloud unconditionally" test for the manifest topic -- the coordinator's
// entity-existence status must follow the exact same policy so the generator can read it from
// anywhere, cloud-preferred (mqtt_entity_existence.go's fetchEntityExistencePreferringCloud). The
// cloud copy's topic is additionally qualified with ownInstallation (fixed live 2026-09-05: two
// houses' own "main" instances were colliding on the shared cloud broker under the bare topic).
func TestSubscribeExistenceReplyRelaysStatusToCloudUnconditionally(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.Seed(fixtureBridgeFileForExistence())

	mainClient := &fakeClient{}
	cloudClient := &fakeClient{}
	if err := tracker.subscribeExistenceReply(mainClient, cloudClient, "junglinster", "protocols-server-2"); err != nil {
		t.Fatalf("subscribeExistenceReply error: %v", err)
	}

	reply := existenceInquiryReply{EntityID: "sensor.davids_bedroom_carbon_dioxide", Exists: true, State: "412.3"}
	payload, _ := json.Marshal(reply)
	mainClient.subscribedHandlers[0](mainClient, fakeMessage{topic: existenceReplyTopic("protocols-server-2"), payload: payload})

	mainPublish, found := findPublish(mainClient.published, existenceStatusTopic("protocols-server-2"))
	if !found {
		t.Fatalf("main published = %+v, want a status publish", mainClient.published)
	}
	wantCloudTopic := "junglinster/" + existenceStatusTopic("protocols-server-2")
	cloudPublish, found := findPublish(cloudClient.published, wantCloudTopic)
	if !found {
		t.Fatalf("cloud published = %+v, want a status publish to %q (installation-qualified)", cloudClient.published, wantCloudTopic)
	}
	if string(mainPublish.payload) != string(cloudPublish.payload) {
		t.Errorf("main and cloud payloads differ: %s vs %s", mainPublish.payload, cloudPublish.payload)
	}
}

func TestSubscribeExistenceReplySkipsCloudWhenNil(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.Seed(fixtureBridgeFileForExistence())

	mainClient := &fakeClient{}
	if err := tracker.subscribeExistenceReply(mainClient, nil, "junglinster", "protocols-server-2"); err != nil {
		t.Fatalf("subscribeExistenceReply error: %v", err)
	}
	reply := existenceInquiryReply{EntityID: "sensor.davids_bedroom_carbon_dioxide", Exists: true, State: "412.3"}
	payload, _ := json.Marshal(reply)
	mainClient.subscribedHandlers[0](mainClient, fakeMessage{topic: existenceReplyTopic("protocols-server-2"), payload: payload})

	if _, found := findPublish(mainClient.published, existenceStatusTopic("protocols-server-2")); !found {
		t.Fatalf("main published = %+v, want a status publish even with no cloud client", mainClient.published)
	}
}

func TestMaybeInquirePublishesEntityIDAsRawPayload(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.Seed(fixtureBridgeFileForExistence())

	client := &fakeClient{}
	tracker.maybeInquire(client, "protocols-server-2")

	publish, found := findPublish(client.published, existenceInquiryTopic("protocols-server-2"))
	if !found {
		t.Fatalf("published = %+v, want an inquiry publish to %q", client.published, existenceInquiryTopic("protocols-server-2"))
	}
	got := string(publish.payload)
	if got != "sensor.davids_bedroom_carbon_dioxide" && got != "sensor.davids_bedroom_humidity" {
		t.Errorf("inquiry payload = %q, want one of the seeded entity ids verbatim (no JSON wrapping)", got)
	}
}

func TestMaybeInquireRespectsPacing(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.Seed(fixtureBridgeFileForExistence())

	client := &fakeClient{}
	tracker.maybeInquire(client, "protocols-server-2")
	firstCount := len(client.published)
	tracker.maybeInquire(client, "protocols-server-2")
	if len(client.published) != firstCount {
		t.Errorf("a second maybeInquire call within the same minute must not publish again, got %d more publishes", len(client.published)-firstCount)
	}
}

// TestStartEntityExistenceInquiriesPublishesImmediately is a regression test for a real gap found
// live 2026-09-05, the day the cloud installation-qualification fix (§12) shipped: publishStatus
// previously only fired on an actual inquiry reply, so a coordinator restart re-seeded already-known
// status in memory without ever re-publishing it -- leaving the (newly-qualified) cloud topic with
// no retained value at all until the next real reply, and every ./generate run's cloud fetch timed
// out in the meantime. StartEntityExistenceInquiries must now publish each instance's current
// status immediately, before any reply has arrived.
func TestStartEntityExistenceInquiriesPublishesImmediately(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.Seed(fixtureBridgeFileForExistence())

	mainClient := &fakeClient{}
	cloudClient := &fakeClient{}
	if err := tracker.StartEntityExistenceInquiries(mainClient, cloudClient, "junglinster", []string{"protocols-server-2"}); err != nil {
		t.Fatalf("StartEntityExistenceInquiries error: %v", err)
	}

	if _, found := findPublish(mainClient.published, existenceStatusTopic("protocols-server-2")); !found {
		t.Errorf("expected an immediate local status publish, got %+v", mainClient.published)
	}
	wantCloudTopic := "junglinster/" + existenceStatusTopic("protocols-server-2")
	if _, found := findPublish(cloudClient.published, wantCloudTopic); !found {
		t.Errorf("expected an immediate qualified cloud status publish to %q, got %+v", wantCloudTopic, cloudClient.published)
	}
}

func TestStartEntityExistenceInquiriesNoInstancesIsNoop(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	client := &fakeClient{}
	if err := tracker.StartEntityExistenceInquiries(client, nil, "junglinster", nil); err != nil {
		t.Fatalf("StartEntityExistenceInquiries error: %v", err)
	}
	if len(client.subscribedHandlers) != 0 {
		t.Errorf("expected no subscriptions when instances is empty, got %d", len(client.subscribedHandlers))
	}
}

// TestSubscribeExistenceReplyQualifiesCloudByInstallation confirms two different installations
// each running their own "main" instance -- the real, live-confirmed collision found 2026-09-05 --
// publish to distinct cloud topics rather than overwriting each other's retained status.
func TestSubscribeExistenceReplyQualifiesCloudByInstallation(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.Seed(fixtureBridgeFileForExistence())

	mainClient := &fakeClient{}
	cloudClient := &fakeClient{}
	if err := tracker.subscribeExistenceReply(mainClient, cloudClient, "junglinster", "main"); err != nil {
		t.Fatalf("subscribeExistenceReply error: %v", err)
	}
	if err := tracker.subscribeExistenceReply(mainClient, cloudClient, "vienna", "main"); err != nil {
		t.Fatalf("subscribeExistenceReply error: %v", err)
	}

	reply := existenceInquiryReply{EntityID: "sensor.davids_bedroom_carbon_dioxide", Exists: true, State: "412.3"}
	payload, _ := json.Marshal(reply)
	for _, handler := range mainClient.subscribedHandlers {
		handler(mainClient, fakeMessage{topic: existenceReplyTopic("main"), payload: payload})
	}

	if _, found := findPublish(cloudClient.published, "junglinster/"+existenceStatusTopic("main")); !found {
		t.Errorf("expected a junglinster-qualified cloud publish, got %+v", cloudClient.published)
	}
	if _, found := findPublish(cloudClient.published, "vienna/"+existenceStatusTopic("main")); !found {
		t.Errorf("expected a vienna-qualified cloud publish, got %+v", cloudClient.published)
	}
}

// TestEntityExistenceTrackerPersistsAcrossRestart is PROJECT.md 1.1's "remaining, beyond today"
// item, closed 2026-08-28: a coordinator restart used to re-seed from homeassistant_bridge.yaml
// and forget every previously resolved status, every discovered sibling, and every synthetic
// device grouping. A tracker constructed with a real path must resume exactly where a prior
// tracker (constructed with the same path) left off.
func TestEntityExistenceTrackerPersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "entity_existence.json")

	first := newEntityExistenceTracker(path)
	first.Seed(fixtureBridgeFileForExistence())
	first.Record("protocols-server-2", "sensor.davids_bedroom_carbon_dioxide", true, "412.3", "", "")
	first.Record("protocols-server-2", "sensor.davids_bedroom_humidity", false, "", "", "")
	first.SeedManualEntity("protocols-server-2", "sensor.office_storage_carbon_dioxide")
	first.Record("protocols-server-2", "sensor.office_storage_carbon_dioxide", true, "612", "", "")
	first.DiscoverSiblings("protocols-server-2", "sensor.office_storage_carbon_dioxide", "remote-device-office-storage", "Office Storage", []string{
		"sensor.office_storage_humidity",
	})

	// A fresh tracker on the same path, simulating a coordinator restart -- Seed runs again first,
	// exactly as main.go does, and must not clobber anything loadPersisted already restored.
	second := newEntityExistenceTracker(path)
	second.Seed(fixtureBridgeFileForExistence())

	byDevice := second.snapshotByDevice("protocols-server-2")
	bedroom := byDevice["hass.davids_bedroom"]
	if bedroom["sensor.davids_bedroom_carbon_dioxide"].Status != StatusKnownToExist {
		t.Errorf("co2 status not restored, got %+v", bedroom)
	}
	if bedroom["sensor.davids_bedroom_humidity"].Status != StatusKnownNotToExist {
		t.Errorf("humidity status not restored, got %+v", bedroom)
	}

	var discoveredDeviceID string
	for deviceID, entities := range byDevice {
		if _, found := entities["sensor.office_storage_carbon_dioxide"]; found {
			discoveredDeviceID = deviceID
		}
	}
	if discoveredDeviceID == "" {
		t.Fatalf("office_storage anchor not restored at all, got %+v", byDevice)
	}
	entities := byDevice[discoveredDeviceID]
	if entities["sensor.office_storage_carbon_dioxide"].Status != StatusKnownToExist {
		t.Errorf("office_storage anchor status not restored, got %+v", entities)
	}
	if _, found := entities["sensor.office_storage_humidity"]; !found {
		t.Errorf("discovered sibling not restored, got %+v", entities)
	}

	// A restart must not re-inquire about anything already resolved -- nextToInquire should have
	// nothing left in the not-known-to-exist backlog for this instance.
	if next := second.nextToInquire("protocols-server-2"); next == "" {
		t.Errorf("expected round-robin refresh to still return something after a restart, got \"\"")
	}
}

func TestEntityExistenceTrackerNoPathDoesNotTouchDisk(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	tracker.Seed(fixtureBridgeFileForExistence())
	tracker.Record("protocols-server-2", "sensor.davids_bedroom_carbon_dioxide", true, "412.3", "", "")
	// No assertion beyond "this doesn't panic or error" -- persist()/loadPersisted() must both be
	// true no-ops when path == "", which every other test in this file already implicitly relies
	// on by never touching a real filesystem path.
}
