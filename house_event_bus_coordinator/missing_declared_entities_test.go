package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPublishMissingDeclaredEntitiesDiscoveryConfig(t *testing.T) {
	client := &fakeClient{}
	if err := publishMissingDeclaredEntitiesDiscoveryConfig(client, "homeassistant.physical", "vienna"); err != nil {
		t.Fatalf("publishMissingDeclaredEntitiesDiscoveryConfig error: %v", err)
	}
	wantTopic := discoveryTopic("homeassistant.physical", "binary_sensor", missingDeclaredEntitiesStableID)
	publish, found := findPublish(client.published, wantTopic)
	if !found {
		t.Fatalf("expected a discovery config publish to %q, got %+v", wantTopic, client.published)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(publish.payload, &body); err != nil {
		t.Fatalf("unmarshal discovery config: %v", err)
	}
	if body["device_class"] != "problem" {
		t.Errorf("device_class = %v, want \"problem\"", body["device_class"])
	}
	if body["state_topic"] != missingDeclaredEntitiesStateTopic() {
		t.Errorf("state_topic = %v, want %q", body["state_topic"], missingDeclaredEntitiesStateTopic())
	}
	if body["json_attributes_topic"] != missingDeclaredEntitiesAttributesTopic() {
		t.Errorf("json_attributes_topic = %v, want %q", body["json_attributes_topic"], missingDeclaredEntitiesAttributesTopic())
	}
}

// TestMissingDeclaredEntitiesPublisherPublishNowAggregatesAllTrackers confirms the aggregate state
// reflects ANY of the three trackers having a known-not-to-exist item, and the itemized attributes
// concatenate all three -- PROJECT.md item 1a's own "one shared indicator across kind-2/kind-3/
// kind-5/kind-4" design.
func TestMissingDeclaredEntitiesPublisherPublishNowAggregatesAllTrackers(t *testing.T) {
	discoveryTracker := newDiscoveryExistenceTracker("")
	discoveryTracker.Seed(fixtureDiscoveryFileForExistence())
	discoveryTracker.RecordTopicIdentity("physical/sensor/ems-esp/boiler_outdoortemp/config", "discovery.ems_esp", "boiler_outdoortemp")
	discoveryTracker.MarkKnown("discovery.ems_esp", "boiler_outdoortemp")
	discoveryTracker.MarkRetracted("physical/sensor/ems-esp/boiler_outdoortemp/config")

	entityTracker := newEntityExistenceTracker("")
	entityTracker.Seed(fixtureBridgeFileForExistence())
	entityTracker.Record("protocols-server-2", "sensor.davids_bedroom_humidity", false, "", "", "", "", "", "")

	publisher := &TMissingDeclaredEntitiesPublisher{}
	client := &fakeClient{}
	if err := publisher.PublishNow(client, discoveryTracker, entityTracker, nil); err != nil {
		t.Fatalf("PublishNow error: %v", err)
	}

	statePublish, found := findPublish(client.published, missingDeclaredEntitiesStateTopic())
	if !found || string(statePublish.payload) != "true" {
		t.Fatalf("state publish = %+v, found=%v, want payload \"true\"", statePublish, found)
	}

	attrsPublish, found := findPublish(client.published, missingDeclaredEntitiesAttributesTopic())
	if !found {
		t.Fatalf("expected an attributes publish, got %+v", client.published)
	}
	var attrs struct {
		Count int      `json:"count"`
		Items []string `json:"items"`
	}
	if err := json.Unmarshal(attrsPublish.payload, &attrs); err != nil {
		t.Fatalf("unmarshal attributes: %v", err)
	}
	if attrs.Count != 2 {
		t.Errorf("count = %d, want 2", attrs.Count)
	}
	wantItems := map[string]bool{
		"discovery.ems_esp/boiler_outdoortemp":              true,
		"protocols-server-2/sensor.davids_bedroom_humidity": true,
	}
	for _, item := range attrs.Items {
		if !wantItems[item] {
			t.Errorf("unexpected item %q in %v", item, attrs.Items)
		}
		delete(wantItems, item)
	}
	if len(wantItems) != 0 {
		t.Errorf("missing expected items: %v", wantItems)
	}
}

// TestMissingDeclaredEntitiesPublisherPublishNowFalseWhenClean confirms the state publishes
// "false" when every tracker (including nil ones, e.g. a house with no "discovery" integration at
// all) has nothing known-not-to-exist.
func TestMissingDeclaredEntitiesPublisherPublishNowFalseWhenClean(t *testing.T) {
	entityTracker := newEntityExistenceTracker("")
	entityTracker.Seed(fixtureBridgeFileForExistence())

	publisher := &TMissingDeclaredEntitiesPublisher{}
	client := &fakeClient{}
	if err := publisher.PublishNow(client, nil, entityTracker, nil); err != nil {
		t.Fatalf("PublishNow error: %v", err)
	}

	statePublish, found := findPublish(client.published, missingDeclaredEntitiesStateTopic())
	if !found || string(statePublish.payload) != "false" {
		t.Fatalf("state publish = %+v, found=%v, want payload \"false\"", statePublish, found)
	}
}

// TestMissingDeclaredEntitiesPublisherScheduleCoalescesBurst mirrors
// TestPassthroughDeviceTrackerScheduleStatusPublishCoalescesBurst (discovery_passthrough_devices_test.go)
// -- many Schedule calls in a tight burst must coalesce into exactly one PublishNow, not one per
// call, same crash-loop-avoidance reasoning as every other debounced publisher in this codebase.
func TestMissingDeclaredEntitiesPublisherScheduleCoalescesBurst(t *testing.T) {
	entityTracker := newEntityExistenceTracker("")
	publisher := &TMissingDeclaredEntitiesPublisher{}
	client := &fakeClient{}

	for i := 0; i < 20; i++ {
		publisher.Schedule(client, nil, entityTracker, nil, 20*time.Millisecond)
	}

	time.Sleep(100 * time.Millisecond)
	count := 0
	for _, p := range client.published {
		if p.topic == missingDeclaredEntitiesStateTopic() {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one coalesced state publish after a burst of Schedule calls, got %d: %+v", count, client.published)
	}
}
