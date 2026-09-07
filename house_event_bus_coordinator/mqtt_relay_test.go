package main

import (
	"testing"
	"time"
)

func TestQualifyHostsTopic(t *testing.T) {
	got := qualifyHostsTopic("hosts/eriks-macbook-pro-2/cpu/state", "junglinster")
	want := "hosts/eriks-macbook-pro-2/junglinster/cpu/state"
	if got != want {
		t.Errorf("qualifyHostsTopic() = %q, want %q", got, want)
	}
}

func TestQualifyHostsTopicFallsBackForUnshapedTopic(t *testing.T) {
	got := qualifyHostsTopic("weird/topic", "junglinster")
	want := "junglinster/weird/topic"
	if got != want {
		t.Errorf("qualifyHostsTopic() = %q, want %q", got, want)
	}
}

func TestDeviceRoutingPlan(t *testing.T) {
	cases := []struct {
		name           string
		device         TDevice
		hasCloudClient bool
		want           TDeviceRoutingPlan
	}{
		{"plain local", TDevice{}, true, TDeviceRoutingPlan{PublishMain: true}},
		{"real cross-house import", TDevice{ImportedFrom: "junglinster"}, true, TDeviceRoutingPlan{PublishMain: true}},
		{"cloud only, not imported", TDevice{Cloud: true}, true, TDeviceRoutingPlan{PublishCloud: true}},
		{"cloud + self-import (replaces old cloud+native)", TDevice{Cloud: true, ImportedFrom: "junglinster"}, true, TDeviceRoutingPlan{PublishMain: true, PublishCloud: true}},
		// A self-import's PublishMain never depends on hasCloudClient -- matches how a real
		// cross-house import already behaves (it's still "imported", the device is still visible
		// locally, even though this house happens to have no cloud broker configured right now).
		{"cloud + self-import, no cloud client", TDevice{Cloud: true, ImportedFrom: "junglinster"}, false, TDeviceRoutingPlan{PublishMain: true}},
		{"cloud declared, not imported, no cloud client", TDevice{Cloud: true}, false, TDeviceRoutingPlan{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := deviceRoutingPlan(c.device, c.hasCloudClient)
			if got != c.want {
				t.Errorf("deviceRoutingPlan(%+v, %v) = %+v, want %+v", c.device, c.hasCloudClient, got, c.want)
			}
		})
	}
}

func TestNeedsCloudRelay(t *testing.T) {
	if qualifier, needed := needsCloudRelay(TDevice{}); needed {
		t.Errorf("plain local device should not need relay, got qualifier=%q", qualifier)
	}
	if qualifier, needed := needsCloudRelay(TDevice{Cloud: true}); needed {
		t.Errorf("cloud-only, not imported, device should not need relay, got qualifier=%q", qualifier)
	}
	if qualifier, needed := needsCloudRelay(TDevice{Cloud: true, ImportedFrom: "junglinster"}); !needed || qualifier != "junglinster" {
		t.Errorf("cloud + self-import device should relay with own installation, got qualifier=%q needed=%v", qualifier, needed)
	}
	if qualifier, needed := needsCloudRelay(TDevice{ImportedFrom: "vienna"}); !needed || qualifier != "vienna" {
		t.Errorf("real cross-house imported device should relay with remote installation, got qualifier=%q needed=%v", qualifier, needed)
	}
}

func TestCloudLivenessTrackerTouchPublishesTrueImmediately(t *testing.T) {
	mainClient := &fakeClient{}
	tracker := newCloudLivenessTracker()

	tracker.Touch(mainClient, nil, "host.eriks-macbook-pro-2", "hosts/eriks-macbook-pro-2/node/state", "")

	if len(mainClient.published) != 1 {
		t.Fatalf("got %d publishes, want 1: %v", len(mainClient.published), mainClient.published)
	}
	if mainClient.published[0].topic != "hosts/eriks-macbook-pro-2/node/state" || string(mainClient.published[0].payload) != "true" {
		t.Errorf("got %+v, want topic=hosts/eriks-macbook-pro-2/node/state payload=true", mainClient.published[0])
	}
}

// TestCloudLivenessTrackerTouchCrossPostsToCloud covers the 2026-09-05 fix: the exporting
// installation is the one that knows a "hosts"-kind device has no real liveness source of its
// own, so its own computed "true"/"false" must reach the cloud catalogue too (qualified the same
// way any other relayed traffic is), letting a genuine cross-house importer just relay it like any
// other capability instead of needing its own synthesis.
func TestCloudLivenessTrackerTouchCrossPostsToCloud(t *testing.T) {
	mainClient := &fakeClient{}
	cloudClient := &fakeClient{}
	tracker := newCloudLivenessTracker()

	tracker.Touch(mainClient, cloudClient, "host.eriks-macbook-pro-2", "hosts/eriks-macbook-pro-2/node/state", "junglinster")

	if len(cloudClient.published) != 1 {
		t.Fatalf("got %d cloud publishes, want 1: %v", len(cloudClient.published), cloudClient.published)
	}
	wantTopic := "hosts/eriks-macbook-pro-2/junglinster/node/state"
	if cloudClient.published[0].topic != wantTopic || string(cloudClient.published[0].payload) != "true" {
		t.Errorf("got %+v, want topic=%s payload=true", cloudClient.published[0], wantTopic)
	}
}

// TestCloudLivenessTrackerTouchSkipsCloudCrossPostWithoutQualifier confirms no cloud publish is
// attempted when there's nothing to qualify with (e.g. a cloud client configured but this device
// isn't itself cloud-relayed) -- must not panic or publish an unqualified topic.
func TestCloudLivenessTrackerTouchSkipsCloudCrossPostWithoutQualifier(t *testing.T) {
	mainClient := &fakeClient{}
	cloudClient := &fakeClient{}
	tracker := newCloudLivenessTracker()

	tracker.Touch(mainClient, cloudClient, "host.eriks-macbook-pro-2", "hosts/eriks-macbook-pro-2/node/state", "")

	if len(cloudClient.published) != 0 {
		t.Errorf("got %v, want no cloud publish when qualifier is empty", cloudClient.published)
	}
}

func TestCloudLivenessTrackerSweepMarksStaleDeviceFalseOnce(t *testing.T) {
	mainClient := &fakeClient{}
	tracker := newCloudLivenessTracker()
	devices := map[string]TDevice{
		"host.eriks-macbook-pro-2": {NodeTopic: "hosts/eriks-macbook-pro-2/node/state"},
	}

	// Simulate a device last seen well beyond cloudDeviceStaleAfter ago.
	tracker.lastSeen["host.eriks-macbook-pro-2"] = time.Now().Add(-2 * cloudDeviceStaleAfter)

	tracker.sweepOnce(mainClient, nil, devices)
	if len(mainClient.published) != 1 || string(mainClient.published[0].payload) != "false" {
		t.Fatalf("got %v, want exactly one \"false\" publish", mainClient.published)
	}

	// A second sweep with no intervening Touch must not republish -- already marked stale.
	tracker.sweepOnce(mainClient, nil, devices)
	if len(mainClient.published) != 1 {
		t.Errorf("got %d publishes after second sweep, want still 1 (no repeat publish)", len(mainClient.published))
	}
}

// TestCloudLivenessTrackerSweepCrossPostsFalseToCloud is the sweep-side counterpart of
// TestCloudLivenessTrackerTouchCrossPostsToCloud: a device going stale must also mark it "false"
// on the cloud catalogue, qualified by its own ImportedFrom (always this house's own installation
// name in practice, since relayCloudDevice only ever runs for the self-import case).
func TestCloudLivenessTrackerSweepCrossPostsFalseToCloud(t *testing.T) {
	mainClient := &fakeClient{}
	cloudClient := &fakeClient{}
	tracker := newCloudLivenessTracker()
	devices := map[string]TDevice{
		"host.eriks-macbook-pro-2": {NodeTopic: "hosts/eriks-macbook-pro-2/node/state", ImportedFrom: "junglinster"},
	}
	tracker.lastSeen["host.eriks-macbook-pro-2"] = time.Now().Add(-2 * cloudDeviceStaleAfter)

	tracker.sweepOnce(mainClient, cloudClient, devices)

	wantTopic := "hosts/eriks-macbook-pro-2/junglinster/node/state"
	found := false
	for _, p := range cloudClient.published {
		if p.topic == wantTopic && string(p.payload) == "false" {
			found = true
		}
	}
	if !found {
		t.Errorf("got %v, want a \"false\" publish to %s on the cloud client", cloudClient.published, wantTopic)
	}
}

func TestCloudLivenessTrackerSweepIgnoresRecentDevice(t *testing.T) {
	mainClient := &fakeClient{}
	tracker := newCloudLivenessTracker()
	devices := map[string]TDevice{
		"host.eriks-macbook-pro-2": {NodeTopic: "hosts/eriks-macbook-pro-2/node/state"},
	}
	tracker.lastSeen["host.eriks-macbook-pro-2"] = time.Now()

	tracker.sweepOnce(mainClient, nil, devices)
	if len(mainClient.published) != 0 {
		t.Errorf("got %v, want no publish for a recently-seen device", mainClient.published)
	}
}

func TestCloudLivenessTrackerTouchAfterStaleClearsFlag(t *testing.T) {
	mainClient := &fakeClient{}
	tracker := newCloudLivenessTracker()
	devices := map[string]TDevice{
		"host.eriks-macbook-pro-2": {NodeTopic: "hosts/eriks-macbook-pro-2/node/state"},
	}
	tracker.lastSeen["host.eriks-macbook-pro-2"] = time.Now().Add(-2 * cloudDeviceStaleAfter)
	tracker.sweepOnce(mainClient, nil, devices) // marks stale, publishes "false"

	tracker.Touch(mainClient, nil, "host.eriks-macbook-pro-2", "hosts/eriks-macbook-pro-2/node/state", "") // traffic resumes

	tracker.mu.Lock()
	stillStale := tracker.stale["host.eriks-macbook-pro-2"]
	tracker.mu.Unlock()
	if stillStale {
		t.Errorf("expected Touch to clear the stale flag so a future sweep can mark it stale again")
	}
}
