package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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

func TestPassthroughDeviceTrackerPersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "discovery_passthrough_devices.json")
	tracker := newPassthroughDeviceTracker(path)
	tracker.Record("zigbee2mqtt_0xaabbcc", "aqara_multi", "sensor", "0xaabbcc_temperature")

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
	var payload map[string]tPassthroughDeviceEntry
	if err := json.Unmarshal(publish.payload, &payload); err != nil {
		t.Fatalf("unmarshalling published payload: %v", err)
	}
	if _, ok := payload["zigbee2mqtt_0xaabbcc"]; !ok {
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
