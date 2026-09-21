package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestProcessDiscoveryRelayJobPublish(t *testing.T) {
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	client := &fakeClient{}

	processDiscoveryRelayJob(TDiscoveryRelayJob{
		Action:  discoveryRelayPublish,
		Client:  client,
		Broker:  "main",
		Topic:   "homeassistant/sensor/coordinator/some_id/config",
		Payload: []byte(`{"a":1}`),
	}, publisher)

	publish, found := findPublish(client.published, "homeassistant/sensor/coordinator/some_id/config")
	if !found {
		t.Fatalf("expected a publish, got %+v", client.published)
	}
	if string(publish.payload) != `{"a":1}` {
		t.Errorf("payload = %s, want %s", publish.payload, `{"a":1}`)
	}
}

// TestProcessDiscoveryRelayJobPublishPassthroughMarksExempt confirms a job with Passthrough: true
// routes to PublishPassthrough (not plain Publish), so the topic is marked exempt from
// RetireMissing's static startup sweep -- see TDiscoveryPublisher.RetireMissing's own doc comment
// for the real incident (2026-09-14) this distinction exists to prevent a repeat of.
func TestProcessDiscoveryRelayJobPublishPassthroughMarksExempt(t *testing.T) {
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	client := &fakeClient{}
	topic := "homeassistant/sensor/zigbee2mqtt/0x00158d0001aabbcc_temperature/config"

	processDiscoveryRelayJob(TDiscoveryRelayJob{
		Action:      discoveryRelayPublish,
		Client:      client,
		Broker:      "main",
		Topic:       topic,
		Payload:     []byte(`{"a":1}`),
		Passthrough: true,
	}, publisher)

	if _, found := findPublish(client.published, topic); !found {
		t.Fatalf("expected a publish, got %+v", client.published)
	}
	if !publisher.passthrough[manifestKey("main", topic)] {
		t.Errorf("expected the topic to be marked passthrough-exempt")
	}
}

func TestProcessDiscoveryRelayJobRetire(t *testing.T) {
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	client := &fakeClient{}
	topic := "homeassistant/sensor/coordinator/some_id/config"

	// Seed publisher's own "known" state first, exactly like a real prior Publish would.
	if err := publisher.Publish(client, "main", topic, []byte(`{"a":1}`)); err != nil {
		t.Fatalf("seeding Publish error: %v", err)
	}
	if !publisher.Knows("main", topic) {
		t.Fatalf("precondition failed: publisher should know %s", topic)
	}

	processDiscoveryRelayJob(TDiscoveryRelayJob{
		Action: discoveryRelayRetire,
		Client: client,
		Broker: "main",
		Topic:  topic,
	}, publisher)

	if publisher.Knows("main", topic) {
		t.Errorf("expected the topic to be forgotten after a retire job")
	}
	// The seed Publish above already recorded one non-empty publish on this same topic --
	// findPublish returns the first match, so check the LAST one (the actual retirement) instead.
	var last recordedPublish
	found := false
	for _, p := range client.published {
		if p.topic == topic {
			last = p
			found = true
		}
	}
	if !found || len(last.payload) != 0 {
		t.Errorf("expected an empty retiring publish, got %+v", client.published)
	}
}

func TestEnqueueDiscoveryRelayJobDropsWhenFull(t *testing.T) {
	jobs := make(chan TDiscoveryRelayJob, 1)
	enqueueDiscoveryRelayJob(jobs, TDiscoveryRelayJob{Topic: "first"})

	// Queue is now full (capacity 1) -- a second enqueue must not block.
	done := make(chan struct{})
	go func() {
		enqueueDiscoveryRelayJob(jobs, TDiscoveryRelayJob{Topic: "second"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("enqueueDiscoveryRelayJob blocked on a full queue instead of dropping")
	}

	if len(jobs) != 1 {
		t.Fatalf("got %d queued jobs, want 1 (the second must have been dropped)", len(jobs))
	}
	if (<-jobs).Topic != "first" {
		t.Errorf("expected the first job to survive, the second to be dropped")
	}
}

func TestDrainDiscoveryRelayQueueProcessesEverythingCurrentlyQueued(t *testing.T) {
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	client := &fakeClient{}
	jobs := make(chan TDiscoveryRelayJob, 10)
	enqueueDiscoveryRelayJob(jobs, TDiscoveryRelayJob{Action: discoveryRelayPublish, Client: client, Broker: "main", Topic: "t1", Payload: []byte("{}")})
	enqueueDiscoveryRelayJob(jobs, TDiscoveryRelayJob{Action: discoveryRelayPublish, Client: client, Broker: "main", Topic: "t2", Payload: []byte("{}")})

	drainDiscoveryRelayQueue(jobs, publisher)

	if len(jobs) != 0 {
		t.Errorf("expected the queue to be fully drained, got %d left", len(jobs))
	}
	if _, found := findPublish(client.published, "t1"); !found {
		t.Errorf("expected t1 to have been published")
	}
	if _, found := findPublish(client.published, "t2"); !found {
		t.Errorf("expected t2 to have been published")
	}
}

// TestNewDiscoveryRelayQueueThrottles is the regression test for the real design requirement
// (2026-09-14, PROJECT.md item 7): jobs must be spaced out, not fired back-to-back as fast as
// possible, even off the MQTT message-dispatch path -- a burst of publishes in a tight loop still
// saturates the shared broker/network right when a remote instance's own MQTT client may be
// processing the very same backlog (confirmed live: HA's own log showed "No ACK from MQTT server
// in 10 seconds" during the incident this queue exists to prevent a repeat of).
func TestNewDiscoveryRelayQueueThrottles(t *testing.T) {
	const throttle = 30 * time.Millisecond

	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	client := &fakeClient{}
	jobs := newDiscoveryRelayQueue(publisher, throttle)

	start := time.Now()
	for i := 0; i < 3; i++ {
		enqueueDiscoveryRelayJob(jobs, TDiscoveryRelayJob{
			Action:  discoveryRelayPublish,
			Client:  client,
			Broker:  "main",
			Topic:   "topic-" + string(rune('a'+i)),
			Payload: []byte("{}"),
		})
	}

	deadline := time.After(3 * time.Second)
	for {
		count := len(client.publishedSnapshot())
		if count >= 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for all 3 queued jobs to be processed, got %d", count)
		case <-time.After(5 * time.Millisecond):
		}
	}
	elapsed := time.Since(start)
	// 3 jobs at 30ms spacing = at least 2 full intervals (60ms) before the last one lands.
	if elapsed < 2*throttle {
		t.Errorf("elapsed = %v, want at least %v -- jobs were not throttled", elapsed, 2*throttle)
	}
}
