package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPassthroughDeviceTrackerRecordAndSnapshot(t *testing.T) {
	tracker := newPassthroughDeviceTracker("")

	if changed := tracker.Record("zigbee2mqtt_0xaabbcc", "aqara_multi", "sensor", "0xaabbcc_temperature"); !changed {
		t.Fatalf("expected the first Record to report a change")
	}
	if changed := tracker.Record("zigbee2mqtt_0xaabbcc", "aqara_multi", "sensor", "0xaabbcc_temperature"); changed {
		t.Errorf("expected an identical repeat Record to report no change")
	}
	if changed := tracker.Record("zigbee2mqtt_0xaabbcc", "aqara_multi", "sensor", "0xaabbcc_humidity"); !changed {
		t.Errorf("expected a new leaf on an already-known device to report a change")
	}

	snap := tracker.snapshot()
	entry, ok := snap["zigbee2mqtt_0xaabbcc"]
	if !ok {
		t.Fatalf("expected the device to be tracked, got %+v", snap)
	}
	if entry.Name != "aqara_multi" {
		t.Errorf("Name = %q, want %q", entry.Name, "aqara_multi")
	}
	if len(entry.Leaves) != 2 {
		t.Fatalf("got %d leaves, want 2: %+v", len(entry.Leaves), entry.Leaves)
	}
	if entry.Leaves["0xaabbcc_temperature"] != (TPassthroughLeaf{Domain: "sensor", Leaf: "0xaabbcc_temperature"}) {
		t.Errorf("temperature leaf = %+v, want domain sensor", entry.Leaves["0xaabbcc_temperature"])
	}
}

func TestPassthroughDeviceTrackerRecordIgnoresEmptyKeys(t *testing.T) {
	tracker := newPassthroughDeviceTracker("")
	if changed := tracker.Record("", "aqara_multi", "sensor", "leaf"); changed {
		t.Errorf("expected Record with an empty device identifier to be a no-op")
	}
	if changed := tracker.Record("zigbee2mqtt_0xaabbcc", "aqara_multi", "sensor", ""); changed {
		t.Errorf("expected Record with an empty leaf to be a no-op")
	}
	if len(tracker.snapshot()) != 0 {
		t.Errorf("expected nothing tracked, got %+v", tracker.snapshot())
	}
}

// TestPassthroughDeviceTrackerForget is the migration-transition coverage: a device that starts
// matching a declared gateway must be forgotten so it stops being suggested for a migration it
// has already undergone.
func TestPassthroughDeviceTrackerForget(t *testing.T) {
	tracker := newPassthroughDeviceTracker("")
	tracker.Record("zigbee2mqtt_0xaabbcc", "aqara_multi", "sensor", "0xaabbcc_temperature")

	if !tracker.Forget("zigbee2mqtt_0xaabbcc") {
		t.Fatalf("expected Forget to report a change for a tracked device")
	}
	if _, ok := tracker.snapshot()["zigbee2mqtt_0xaabbcc"]; ok {
		t.Errorf("expected the device to be gone after Forget, got %+v", tracker.snapshot())
	}
	if tracker.Forget("zigbee2mqtt_0xaabbcc") {
		t.Errorf("expected a repeat Forget on an already-forgotten device to report no change")
	}
	if tracker.Forget("") {
		t.Errorf("expected Forget(\"\") to report no change")
	}
}

// TestPassthroughDeviceTrackerForgetByTopicResolvesViaRecordedTopicIdentity is the regression test
// for a real gap found live 2026-09-21 (Vienna): a device deleted outright from Zigbee2MQTT (not
// migrated into a real Physical.def declaration, genuinely removed from the network) publishes
// empty payloads on its own discovery config topics, but Forget was previously only ever called
// when a device started matching a declared gateway -- nothing removed a deleted device's own
// still-undeclared suggestion entry, leaving it suggested in suggestions/discovery.txt forever.
func TestPassthroughDeviceTrackerForgetByTopicResolvesViaRecordedTopicIdentity(t *testing.T) {
	tracker := newPassthroughDeviceTracker("")
	tracker.RecordTopicIdentity("physical/sensor/rack_aqara_multi/temperature/config", "zigbee2mqtt_0xaabbcc")
	tracker.Record("zigbee2mqtt_0xaabbcc", "aqara_multi", "sensor", "0xaabbcc_temperature")

	deviceIdentifier, ok := tracker.ForgetByTopic("physical/sensor/rack_aqara_multi/temperature/config")
	if !ok || deviceIdentifier != "zigbee2mqtt_0xaabbcc" {
		t.Fatalf("ForgetByTopic = (%q, %v), want (\"zigbee2mqtt_0xaabbcc\", true)", deviceIdentifier, ok)
	}
	if _, tracked := tracker.snapshot()["zigbee2mqtt_0xaabbcc"]; tracked {
		t.Errorf("expected the device to be gone after ForgetByTopic, got %+v", tracker.snapshot())
	}

	// A second retraction of the same, already-forgotten topic is a no-op.
	if _, ok := tracker.ForgetByTopic("physical/sensor/rack_aqara_multi/temperature/config"); ok {
		t.Errorf("expected no change retracting an already-forgotten device's topic")
	}
}

func TestPassthroughDeviceTrackerForgetByTopicUnknownTopicIsNoop(t *testing.T) {
	tracker := newPassthroughDeviceTracker("")
	deviceIdentifier, ok := tracker.ForgetByTopic("physical/sensor/never_seen/temperature/config")
	if ok || deviceIdentifier != "" {
		t.Errorf("ForgetByTopic for an unrecorded topic = (%q, %v), want (\"\", false)", deviceIdentifier, ok)
	}
}

// TestPassthroughDeviceTrackerForgetReleasesTopicIdentities guards against a leaked topicDevice
// entry once a device is forgotten via the ORDINARY migration path (Forget, not ForgetByTopic) --
// a stale mapping left behind here would be harmless (ForgetByTopic on it later just no-ops, see
// its own doc comment) but would otherwise grow this map unboundedly over a coordinator's lifetime.
func TestPassthroughDeviceTrackerForgetReleasesTopicIdentities(t *testing.T) {
	tracker := newPassthroughDeviceTracker("")
	tracker.RecordTopicIdentity("physical/sensor/rack_aqara_multi/temperature/config", "zigbee2mqtt_0xaabbcc")
	tracker.Record("zigbee2mqtt_0xaabbcc", "aqara_multi", "sensor", "0xaabbcc_temperature")

	tracker.Forget("zigbee2mqtt_0xaabbcc")

	if len(tracker.topicDevice) != 0 {
		t.Errorf("expected topicDevice to be empty after Forget, got %+v", tracker.topicDevice)
	}
}

func TestPassthroughDeviceTrackerPersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "discovery_passthrough_devices.json")
	tracker := newPassthroughDeviceTracker(path)
	tracker.Record("zigbee2mqtt_0xaabbcc", "aqara_multi", "sensor", "0xaabbcc_temperature")
	// Record itself no longer persists synchronously (2026-09-22 crash-loop fix -- see its own doc
	// comment); production callers debounce via SchedulePersist instead. Calling persist() directly
	// here simulates that debounced write actually firing, without the test needing to sleep past a
	// real timer.
	tracker.persist()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected a persisted file, got: %v", err)
	}

	restarted := newPassthroughDeviceTracker(path)
	snap := restarted.snapshot()
	entry, ok := snap["zigbee2mqtt_0xaabbcc"]
	if !ok || entry.Name != "aqara_multi" {
		t.Errorf("expected persisted state to survive a restart, got %+v", snap)
	}
}

func TestPassthroughDeviceTrackerPublishStatus(t *testing.T) {
	tracker := newPassthroughDeviceTracker("")
	tracker.Record("zigbee2mqtt_0xaabbcc", "aqara_multi", "sensor", "0xaabbcc_temperature")

	client := &fakeClient{}
	if err := tracker.PublishStatus(client, nil, "junglinster"); err != nil {
		t.Fatalf("PublishStatus error: %v", err)
	}
	publish, found := findPublish(client.published, passthroughDevicesStatusTopic)
	if !found {
		t.Fatalf("expected a publish on %s, got %+v", passthroughDevicesStatusTopic, client.published)
	}
	var payload tPassthroughStatusSnapshot
	if err := json.Unmarshal(publish.payload, &payload); err != nil {
		t.Fatalf("unmarshalling published payload: %v", err)
	}
	if _, ok := payload.Devices["zigbee2mqtt_0xaabbcc"]; !ok {
		t.Errorf("published payload = %+v, want the tracked device", payload)
	}
}

func TestPassthroughDeviceTrackerPublishStatusAlsoPublishesToCloud(t *testing.T) {
	tracker := newPassthroughDeviceTracker("")
	tracker.Record("zigbee2mqtt_0xaabbcc", "aqara_multi", "sensor", "0xaabbcc_temperature")

	mainClient := &fakeClient{}
	cloudClient := &fakeClient{}
	if err := tracker.PublishStatus(mainClient, cloudClient, "junglinster"); err != nil {
		t.Fatalf("PublishStatus error: %v", err)
	}
	if _, found := findPublish(cloudClient.published, "junglinster/"+passthroughDevicesStatusTopic); !found {
		t.Errorf("expected an installation-qualified publish on the cloud client, got %+v", cloudClient.published)
	}
}

const (
	xanaduTopic = "homeassistant/switch/0xa4c138a4716e0c02/switch/config"
	disksTopic  = "homeassistant/switch/0xc4988600000f73e3/switch/config"
)

// TestPassthroughDeviceTrackerClaimNameFirstClaimWins is the ordinary case: a device's own name is
// unclaimed, so it wins outright and no collision is recorded.
func TestPassthroughDeviceTrackerClaimNameFirstClaimWins(t *testing.T) {
	tracker := newPassthroughDeviceTracker("")
	if ok, evicted := tracker.ClaimName("switch.house/server_room/xanadu", "zigbee2mqtt_0xa4c138a4716e0c02", xanaduTopic, true); !ok || evicted != "" {
		t.Fatalf("expected an unclaimed name to be claimed successfully with nothing evicted, got ok=%v evicted=%q", ok, evicted)
	}
	if collisions := tracker.collisionsSnapshot(); len(collisions) != 0 {
		t.Errorf("expected no collisions, got %+v", collisions)
	}
}

// TestPassthroughDeviceTrackerClaimNameSameDeviceReclaimingIsFine covers the overwhelmingly common
// case: the SAME device re-affirms its own name on every retained-backlog replay/reconnect.
func TestPassthroughDeviceTrackerClaimNameSameDeviceReclaimingIsFine(t *testing.T) {
	tracker := newPassthroughDeviceTracker("")
	tracker.ClaimName("switch.house/server_room/xanadu", "zigbee2mqtt_0xa4c138a4716e0c02", xanaduTopic, true)
	if ok, evicted := tracker.ClaimName("switch.house/server_room/xanadu", "zigbee2mqtt_0xa4c138a4716e0c02", xanaduTopic, true); !ok || evicted != "" {
		t.Errorf("expected the same device reclaiming its own name to succeed with nothing evicted, got ok=%v evicted=%q", ok, evicted)
	}
	if collisions := tracker.collisionsSnapshot(); len(collisions) != 0 {
		t.Errorf("expected no collisions, got %+v", collisions)
	}
}

// TestPassthroughDeviceTrackerClaimNameDifferentDeviceCollides is the real bug this exists for
// (found live 2026-09-20): Junglinster's "xanadu" plug was renamed in Zigbee2MQTT, but its OLD
// stale retained discovery config kept claiming the same default_entity_id a DIFFERENT, currently-
// correct device had since taken over. The second claimant must be rejected, not silently relayed.
func TestPassthroughDeviceTrackerClaimNameDifferentDeviceCollides(t *testing.T) {
	tracker := newPassthroughDeviceTracker("")
	if ok, _ := tracker.ClaimName("switch.house/server_room/xanadu", "zigbee2mqtt_0xa4c138a4716e0c02", xanaduTopic, true); !ok {
		t.Fatalf("expected the first claim to succeed")
	}
	if ok, evicted := tracker.ClaimName("switch.house/server_room/xanadu", "zigbee2mqtt_0xc4988600000f73e3", disksTopic, true); ok || evicted != "" {
		t.Fatalf("expected a different device claiming the same name to be rejected with nothing evicted, got ok=%v evicted=%q", ok, evicted)
	}
	collisions := tracker.collisionsSnapshot()
	collision, ok := collisions["switch.house/server_room/xanadu"]
	if !ok {
		t.Fatalf("expected the collision to be recorded, got %+v", collisions)
	}
	if collision.ClaimedBy != "zigbee2mqtt_0xa4c138a4716e0c02" || collision.BlockedID != "zigbee2mqtt_0xc4988600000f73e3" {
		t.Errorf("collision = %+v, want ClaimedBy/BlockedID matching the two devices", collision)
	}
}

// TestPassthroughDeviceTrackerClaimNameConsistentClaimDisplacesStaleOwner is the regression test
// for a real incident found live MINUTES after this mechanism's own first deploy (2026-09-20):
// plain first-claim-wins let the STALE (renamed-away) device win the race on that particular
// coordinator restart -- purely because its retained message happened to be replayed first -- which
// then suppressed the CURRENTLY CORRECT device's relay instead of the stale one's, exactly
// backwards from what the whole mechanism exists to prevent. A later, self-consistent claim must
// always displace an earlier, inconsistent one, regardless of arrival order, AND the displaced
// owner's own already-relayed topic must be handed back to the caller so it can be retired --
// otherwise the stale entity keeps lingering in HA alongside the new, correct one.
func TestPassthroughDeviceTrackerClaimNameConsistentClaimDisplacesStaleOwner(t *testing.T) {
	tracker := newPassthroughDeviceTracker("")
	// Stale device claims first (inconsistent: consistent=false), as happened live.
	if ok, _ := tracker.ClaimName("switch.house/server_room/xanadu", "zigbee2mqtt_0xc4988600000f73e3", disksTopic, false); !ok {
		t.Fatalf("expected the first claim (even if inconsistent) to succeed when nothing else owns the name yet")
	}
	// The genuinely correct device arrives next -- must win despite arriving second, and the
	// stale owner's own topic must come back for the caller to retire.
	ok, evicted := tracker.ClaimName("switch.house/server_room/xanadu", "zigbee2mqtt_0xa4c138a4716e0c02", xanaduTopic, true)
	if !ok {
		t.Fatalf("expected a self-consistent claim to displace a less consistent current owner")
	}
	if evicted != disksTopic {
		t.Errorf("evicted topic = %q, want the displaced stale device's own topic %q", evicted, disksTopic)
	}
	if ok, evicted := tracker.ClaimName("switch.house/server_room/xanadu", "zigbee2mqtt_0xa4c138a4716e0c02", xanaduTopic, true); !ok || evicted != "" {
		t.Errorf("expected the new owner to keep winning on subsequent messages with nothing further evicted, got ok=%v evicted=%q", ok, evicted)
	}
	// The stale device trying again afterwards must now be the one rejected -- and flagged, so
	// suggestions/discovery.txt tells the operator which device still needs fixing at the source.
	if ok, evicted := tracker.ClaimName("switch.house/server_room/xanadu", "zigbee2mqtt_0xc4988600000f73e3", disksTopic, false); ok || evicted != "" {
		t.Errorf("expected the displaced stale device to be rejected on a later attempt, got ok=%v evicted=%q", ok, evicted)
	}
	collisions := tracker.collisionsSnapshot()
	collision, seen := collisions["switch.house/server_room/xanadu"]
	if !seen {
		t.Fatalf("expected the stale device's later attempt to be recorded as a collision, got %+v", collisions)
	}
	if collision.ClaimedBy != "zigbee2mqtt_0xa4c138a4716e0c02" || collision.BlockedID != "zigbee2mqtt_0xc4988600000f73e3" {
		t.Errorf("collision = %+v, want the correct device as owner and the stale one blocked", collision)
	}
}

// TestPassthroughDeviceTrackerClaimNameConsistentClaimWinsRegardlessOfOrder confirms the SAME
// outcome when the consistent device happens to arrive first instead -- ownership must not depend
// on arrival order either way, and nothing needs evicting since the stale claimant never won
// ownership at all in this ordering.
func TestPassthroughDeviceTrackerClaimNameConsistentClaimWinsRegardlessOfOrder(t *testing.T) {
	tracker := newPassthroughDeviceTracker("")
	if ok, _ := tracker.ClaimName("switch.house/server_room/xanadu", "zigbee2mqtt_0xa4c138a4716e0c02", xanaduTopic, true); !ok {
		t.Fatalf("expected the first (consistent) claim to succeed")
	}
	if ok, evicted := tracker.ClaimName("switch.house/server_room/xanadu", "zigbee2mqtt_0xc4988600000f73e3", disksTopic, false); ok || evicted != "" {
		t.Errorf("expected the later inconsistent claim to be rejected, not displace the consistent owner, got ok=%v evicted=%q", ok, evicted)
	}
}

// TestPassthroughDeviceTrackerClaimNameIgnoresEmptyArguments matches Record/Forget's own
// no-op-on-empty-key convention -- nothing stable to key on, so nothing is blocked.
func TestPassthroughDeviceTrackerClaimNameIgnoresEmptyArguments(t *testing.T) {
	tracker := newPassthroughDeviceTracker("")
	if ok, _ := tracker.ClaimName("", "zigbee2mqtt_0xaabbcc", "", true); !ok {
		t.Errorf("expected an empty default_entity_id to be a no-op (claim succeeds)")
	}
	if ok, _ := tracker.ClaimName("switch.house/server_room/xanadu", "", "", true); !ok {
		t.Errorf("expected an empty device identifier to be a no-op (claim succeeds)")
	}
}

// TestPassthroughDeviceTrackerForgetReleasesClaimsAndCollisions is the lifecycle counterpart: once
// a device migrates (Forget), its claim/collision must not permanently block some OTHER
// still-undeclared device from ever claiming that name again.
func TestPassthroughDeviceTrackerForgetReleasesClaimsAndCollisions(t *testing.T) {
	tracker := newPassthroughDeviceTracker("")
	tracker.Record("zigbee2mqtt_0xa4c138a4716e0c02", "xanadu", "switch", "leaf")
	tracker.ClaimName("switch.house/server_room/xanadu", "zigbee2mqtt_0xa4c138a4716e0c02", xanaduTopic, true)
	tracker.ClaimName("switch.house/server_room/xanadu", "zigbee2mqtt_0xc4988600000f73e3", disksTopic, true)

	tracker.Forget("zigbee2mqtt_0xa4c138a4716e0c02")

	if collisions := tracker.collisionsSnapshot(); len(collisions) != 0 {
		t.Errorf("expected Forget to clear the collision it was party to, got %+v", collisions)
	}
	if ok, _ := tracker.ClaimName("switch.house/server_room/xanadu", "zigbee2mqtt_0xc4988600000f73e3", disksTopic, true); !ok {
		t.Errorf("expected the previously-blocked device to be able to claim the name after Forget")
	}
}

// TestPassthroughDeviceTrackerPublishStatusIncludesCollisions confirms the collision map rides
// along on the same retained topic the generator already reads for undeclared devices, so it
// doesn't need a second subscription.
func TestPassthroughDeviceTrackerPublishStatusIncludesCollisions(t *testing.T) {
	tracker := newPassthroughDeviceTracker("")
	tracker.ClaimName("switch.house/server_room/xanadu", "zigbee2mqtt_0xa4c138a4716e0c02", xanaduTopic, true)
	tracker.ClaimName("switch.house/server_room/xanadu", "zigbee2mqtt_0xc4988600000f73e3", disksTopic, true)

	client := &fakeClient{}
	if err := tracker.PublishStatus(client, nil, "junglinster"); err != nil {
		t.Fatalf("PublishStatus error: %v", err)
	}
	publish, found := findPublish(client.published, passthroughDevicesStatusTopic)
	if !found {
		t.Fatalf("expected a publish on %s, got %+v", passthroughDevicesStatusTopic, client.published)
	}
	var payload tPassthroughStatusSnapshot
	if err := json.Unmarshal(publish.payload, &payload); err != nil {
		t.Fatalf("unmarshalling published payload: %v", err)
	}
	if _, ok := payload.Collisions["switch.house/server_room/xanadu"]; !ok {
		t.Errorf("published payload = %+v, want the recorded collision", payload)
	}
}

// TestPassthroughDeviceTrackerClaimsPersistAcrossRestart mirrors
// TestPassthroughDeviceTrackerPersistsAcrossRestart for the new claims/collisions state.
func TestPassthroughDeviceTrackerClaimsPersistAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "discovery_passthrough_devices.json")
	tracker := newPassthroughDeviceTracker(path)
	tracker.ClaimName("switch.house/server_room/xanadu", "zigbee2mqtt_0xa4c138a4716e0c02", xanaduTopic, true)
	tracker.ClaimName("switch.house/server_room/xanadu", "zigbee2mqtt_0xc4988600000f73e3", disksTopic, true)

	restarted := newPassthroughDeviceTracker(path)
	if ok, evicted := restarted.ClaimName("switch.house/server_room/xanadu", "zigbee2mqtt_0xc4988600000f73e3", disksTopic, true); ok || evicted != "" {
		t.Errorf("expected the original owner's claim to survive a restart, got ok=%v evicted=%q", ok, evicted)
	}
	if collisions := restarted.collisionsSnapshot(); len(collisions) != 1 {
		t.Errorf("expected the collision to survive a restart, got %+v", collisions)
	}
}

// TestPassthroughDeviceTrackerScheduleStatusPublishFiresAfterDelay is the regression test for the
// real gap found live 2026-09-19: Record's own "changed" return value was discarded at its call
// site (discoverybridge.go), so a newly-seen passthrough device never triggered a fresh status
// publish at all -- the retained MQTT topic the generator reads went stale until the next
// coordinator restart/reconnect. ScheduleStatusPublish must actually publish, after delay elapses.
func TestPassthroughDeviceTrackerScheduleStatusPublishFiresAfterDelay(t *testing.T) {
	tracker := newPassthroughDeviceTracker("")
	tracker.Record("zigbee2mqtt_0xaabbcc", "aqara_multi", "sensor", "0xaabbcc_temperature")

	client := &fakeClient{}
	tracker.ScheduleStatusPublish(client, nil, "junglinster", 10*time.Millisecond)

	if len(client.publishedSnapshot()) != 0 {
		t.Fatalf("expected no publish before delay elapses, got %+v", client.publishedSnapshot())
	}
	time.Sleep(50 * time.Millisecond)
	if _, found := findPublish(client.publishedSnapshot(), passthroughDevicesStatusTopic); !found {
		t.Fatalf("expected a publish on %s after delay elapsed, got %+v", passthroughDevicesStatusTopic, client.publishedSnapshot())
	}
}

// TestPassthroughDeviceTrackerScheduleStatusPublishCoalescesBurst is the core coalescing coverage
// this mechanism exists for (see PassthroughStatusDebounceDelay's own doc comment, and the
// 2026-09-14 crash-loop incident it must never repeat): many ScheduleStatusPublish calls in a tight
// burst -- exactly the shape of a startup retained-backlog replay -- must produce exactly ONE
// publish, not one per call.
func TestPassthroughDeviceTrackerScheduleStatusPublishCoalescesBurst(t *testing.T) {
	tracker := newPassthroughDeviceTracker("")
	client := &fakeClient{}

	for i := 0; i < 50; i++ {
		tracker.Record("zigbee2mqtt_0xaabbcc", "aqara_multi", "sensor", "0xaabbcc_temperature")
		tracker.ScheduleStatusPublish(client, nil, "junglinster", 20*time.Millisecond)
	}

	time.Sleep(80 * time.Millisecond)
	published := client.publishedSnapshot()
	count := 0
	for _, p := range published {
		if p.topic == passthroughDevicesStatusTopic {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 publish after a 50-call burst, got %d: %+v", count, published)
	}
}

// TestPassthroughDeviceTrackerStartPeriodicStatusPublishFiresRepeatedly is the safety-net coverage:
// independent of any debounced call, the periodic publisher must keep republishing on its own
// interval for the life of the process.
func TestPassthroughDeviceTrackerStartPeriodicStatusPublishFiresRepeatedly(t *testing.T) {
	tracker := newPassthroughDeviceTracker("")
	tracker.Record("zigbee2mqtt_0xaabbcc", "aqara_multi", "sensor", "0xaabbcc_temperature")

	client := &fakeClient{}
	tracker.StartPeriodicStatusPublish(client, nil, "junglinster", 15*time.Millisecond)

	time.Sleep(70 * time.Millisecond)
	count := 0
	for _, p := range client.publishedSnapshot() {
		if p.topic == passthroughDevicesStatusTopic {
			count++
		}
	}
	if count < 2 {
		t.Errorf("expected at least 2 periodic publishes within 70ms at a 15ms interval, got %d", count)
	}
}
