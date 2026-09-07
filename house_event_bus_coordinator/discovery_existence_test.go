package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func fixtureDiscoveryFileForExistence() TDiscoveryFile {
	return TDiscoveryFile{
		Gateways: map[string]TDiscoveryGateway{
			"discovery.ems_esp": {Identifiers: []string{"ems-esp-boiler"}},
		},
		EntityLinks: map[string]TDiscoveryEntityLink{
			"sensor.social_garage_door_temperature": {Gateway: "discovery.ems_esp", Leaf: "boiler_outdoortemp"},
		},
	}
}

func TestDiscoveryExistenceTrackerSeedStartsNotKnownToExist(t *testing.T) {
	tracker := newDiscoveryExistenceTracker("")
	tracker.Seed(fixtureDiscoveryFileForExistence())

	got := tracker.snapshot("discovery.ems_esp")
	if got["boiler_outdoortemp"] != StatusNotKnownToExist {
		t.Errorf("snapshot = %+v, want boiler_outdoortemp not-known-to-exist", got)
	}
}

func TestDiscoveryExistenceTrackerMarkKnownTransitionsAndIsIdempotent(t *testing.T) {
	tracker := newDiscoveryExistenceTracker("")
	tracker.Seed(fixtureDiscoveryFileForExistence())

	if changed := tracker.MarkKnown("discovery.ems_esp", "boiler_outdoortemp"); !changed {
		t.Errorf("first MarkKnown should report a change")
	}
	if got := tracker.snapshot("discovery.ems_esp")["boiler_outdoortemp"]; got != StatusKnownToExist {
		t.Errorf("status = %q, want %q", got, StatusKnownToExist)
	}
	if changed := tracker.MarkKnown("discovery.ems_esp", "boiler_outdoortemp"); changed {
		t.Errorf("second MarkKnown for an already-known leaf should report no change")
	}
}

// TestDiscoveryExistenceTrackerMarkKnownWorksForUndeclaredLeaf confirms a leaf the DSL hasn't
// positioned yet (no EntityLink) still gets tracked -- existence is a property of the gateway's own
// reporting, independent of whether Physical.def has claimed it (PROJECT.md 1.8).
func TestDiscoveryExistenceTrackerMarkKnownWorksForUndeclaredLeaf(t *testing.T) {
	tracker := newDiscoveryExistenceTracker("")
	if changed := tracker.MarkKnown("discovery.ems_esp", "thermostat_lastcode"); !changed {
		t.Errorf("expected MarkKnown to succeed for a previously untracked leaf")
	}
	if got := tracker.snapshot("discovery.ems_esp")["thermostat_lastcode"]; got != StatusKnownToExist {
		t.Errorf("status = %q, want %q", got, StatusKnownToExist)
	}
}

func TestDiscoveryExistenceTrackerPublishStatusRelaysToLocalAndCloud(t *testing.T) {
	tracker := newDiscoveryExistenceTracker("")
	tracker.Seed(fixtureDiscoveryFileForExistence())
	tracker.MarkKnown("discovery.ems_esp", "boiler_outdoortemp")

	mainClient := &fakeClient{}
	cloudClient := &fakeClient{}
	if err := tracker.publishStatus(mainClient, cloudClient, "junglinster", "discovery.ems_esp"); err != nil {
		t.Fatalf("publishStatus error: %v", err)
	}

	wantTopic := discoveryExistenceStatusTopic("discovery.ems_esp")
	mainPublish, found := findPublish(mainClient.published, wantTopic)
	if !found {
		t.Fatalf("main published = %+v, want a status publish to %q", mainClient.published, wantTopic)
	}
	wantCloudTopic := "junglinster/" + wantTopic
	cloudPublish, found := findPublish(cloudClient.published, wantCloudTopic)
	if !found {
		t.Fatalf("cloud published = %+v, want a status publish to %q (installation-qualified)", cloudClient.published, wantCloudTopic)
	}
	if string(mainPublish.payload) != string(cloudPublish.payload) {
		t.Errorf("main and cloud payloads differ: %s vs %s", mainPublish.payload, cloudPublish.payload)
	}
	if !strings.Contains(string(mainPublish.payload), `"boiler_outdoortemp":"known-to-exist"`) {
		t.Errorf("payload = %s, want boiler_outdoortemp marked known-to-exist", mainPublish.payload)
	}
}

// TestDiscoveryExistenceTrackerPublishStatusQualifiesCloudByInstallation confirms two different
// installations sharing the same gatewayID (e.g. both declaring their own "discovery.ems_esp")
// publish to distinct cloud topics rather than colliding -- the bug found live 2026-09-05 for
// entity_existence.go's analogous "main" instance collision, fixed here the same way.
func TestDiscoveryExistenceTrackerPublishStatusQualifiesCloudByInstallation(t *testing.T) {
	tracker := newDiscoveryExistenceTracker("")
	tracker.Seed(fixtureDiscoveryFileForExistence())
	tracker.MarkKnown("discovery.ems_esp", "boiler_outdoortemp")

	cloudClient := &fakeClient{}
	if err := tracker.publishStatus(&fakeClient{}, cloudClient, "junglinster", "discovery.ems_esp"); err != nil {
		t.Fatalf("publishStatus error: %v", err)
	}
	if err := tracker.publishStatus(&fakeClient{}, cloudClient, "vienna", "discovery.ems_esp"); err != nil {
		t.Fatalf("publishStatus error: %v", err)
	}

	if _, found := findPublish(cloudClient.published, "junglinster/"+discoveryExistenceStatusTopic("discovery.ems_esp")); !found {
		t.Errorf("expected a junglinster-qualified cloud publish, got %+v", cloudClient.published)
	}
	if _, found := findPublish(cloudClient.published, "vienna/"+discoveryExistenceStatusTopic("discovery.ems_esp")); !found {
		t.Errorf("expected a vienna-qualified cloud publish, got %+v", cloudClient.published)
	}
}

// TestDiscoveryExistenceTrackerPublishAllPublishesEveryDeclaredGateway is a regression test for the
// same gap TestStartEntityExistenceInquiriesPublishesImmediately fixes for kind-3, found live
// 2026-09-05: kind-2's publishStatus only ever fires on an actual known/retracted transition, so a
// gateway whose leaves are all already known-to-exist (the common case after a restart) may never
// get republished again -- leaving the cloud topic missing/stale after a topic-naming migration like
// §12's installation-qualification fix, with every ./generate run's cloud fetch timing out until the
// next real transition (which may never come for a stable gateway).
func TestDiscoveryExistenceTrackerPublishAllPublishesEveryDeclaredGateway(t *testing.T) {
	tracker := newDiscoveryExistenceTracker("")
	discoveryFile := fixtureDiscoveryFileForExistence()
	tracker.Seed(discoveryFile)
	tracker.MarkKnown("discovery.ems_esp", "boiler_outdoortemp")

	mainClient := &fakeClient{}
	cloudClient := &fakeClient{}
	tracker.PublishAll(mainClient, cloudClient, "junglinster", discoveryFile)

	if _, found := findPublish(mainClient.published, discoveryExistenceStatusTopic("discovery.ems_esp")); !found {
		t.Errorf("expected an immediate local status publish, got %+v", mainClient.published)
	}
	wantCloudTopic := "junglinster/" + discoveryExistenceStatusTopic("discovery.ems_esp")
	if _, found := findPublish(cloudClient.published, wantCloudTopic); !found {
		t.Errorf("expected an immediate qualified cloud status publish to %q, got %+v", wantCloudTopic, cloudClient.published)
	}
}

func TestDiscoveryExistenceTrackerPublishStatusSkipsCloudWhenNil(t *testing.T) {
	tracker := newDiscoveryExistenceTracker("")
	tracker.MarkKnown("discovery.ems_esp", "boiler_outdoortemp")

	mainClient := &fakeClient{}
	if err := tracker.publishStatus(mainClient, nil, "junglinster", "discovery.ems_esp"); err != nil {
		t.Fatalf("publishStatus error: %v", err)
	}
	if _, found := findPublish(mainClient.published, discoveryExistenceStatusTopic("discovery.ems_esp")); !found {
		t.Fatalf("expected a status publish even with no cloud client")
	}
}

// TestDiscoveryExistenceTrackerSeedPrunesConfirmedDeadOrphans is the kind-2 counterpart to
// entity_existence.go's TestEntityExistenceTrackerSeedPrunesConfirmedDeadOrphans (2026-09-06,
// PROJECT.md item 1's own pruning note): a retracted leaf that's no longer declared in
// discovery.yaml's own EntityLinks would otherwise sit in the tracker forever -- MarkRetracted
// only ever flips status, never removes the entry. Seed must delete a (gatewayID, leaf) pair once
// it's both known-not-to-exist and no longer declared, but never touch a not-yet-resolved or
// known-to-exist leaf.
func TestDiscoveryExistenceTrackerSeedPrunesConfirmedDeadOrphans(t *testing.T) {
	tracker := newDiscoveryExistenceTracker("")
	tracker.Seed(fixtureDiscoveryFileForExistence())
	tracker.RecordTopicIdentity("physical/sensor/ems-esp/boiler_outdoortemp/config", "discovery.ems_esp", "boiler_outdoortemp")
	tracker.MarkKnown("discovery.ems_esp", "boiler_outdoortemp")
	tracker.MarkRetracted("physical/sensor/ems-esp/boiler_outdoortemp/config")

	// A second, still-unresolved leaf on the same gateway must survive re-seeding regardless.
	tracker.MarkKnown("discovery.ems_esp", "thermostat_lastcode")

	// The EMS-ESP device gets physically replaced: Physical.def drops the retracted leaf's own
	// EntityLink entirely (the DSL author removed the dead reference), keeping only the other one.
	tracker.Seed(TDiscoveryFile{
		Gateways: map[string]TDiscoveryGateway{"discovery.ems_esp": {Identifiers: []string{"ems-esp-boiler"}}},
		EntityLinks: map[string]TDiscoveryEntityLink{
			"sensor.thermostat_code": {Gateway: "discovery.ems_esp", Leaf: "thermostat_lastcode"},
		},
	})

	snapshot := tracker.snapshot("discovery.ems_esp")
	if _, found := snapshot["boiler_outdoortemp"]; found {
		t.Errorf("confirmed-dead, no-longer-declared ghost leaf was not pruned: %+v", snapshot)
	}
	if got, found := snapshot["thermostat_lastcode"]; !found || got != StatusKnownToExist {
		t.Errorf("a known-to-exist leaf must survive re-seeding, got %q (found=%v)", got, found)
	}
}

// TestDiscoveryExistenceTrackerSeedNeverPrunesStillDeclaredEntries guards against over-pruning: a
// leaf that's STILL declared must keep its known-not-to-exist status across re-seeding -- the
// exact status checkDiscoveryKnownNotToExistErrors depends on to keep flagging a genuinely
// still-broken declaration, so it must never be silently pruned away.
func TestDiscoveryExistenceTrackerSeedNeverPrunesStillDeclaredEntries(t *testing.T) {
	tracker := newDiscoveryExistenceTracker("")
	discoveryFile := fixtureDiscoveryFileForExistence()
	tracker.Seed(discoveryFile)
	tracker.RecordTopicIdentity("physical/sensor/ems-esp/boiler_outdoortemp/config", "discovery.ems_esp", "boiler_outdoortemp")
	tracker.MarkKnown("discovery.ems_esp", "boiler_outdoortemp")
	tracker.MarkRetracted("physical/sensor/ems-esp/boiler_outdoortemp/config")

	tracker.Seed(discoveryFile)

	if got := tracker.snapshot("discovery.ems_esp")["boiler_outdoortemp"]; got != StatusKnownNotToExist {
		t.Errorf("a still-declared known-not-to-exist leaf must never be pruned, got %q", got)
	}
}

func TestDiscoveryExistenceTrackerMarkRetractedResolvesViaRecordedTopicIdentity(t *testing.T) {
	tracker := newDiscoveryExistenceTracker("")
	tracker.Seed(fixtureDiscoveryFileForExistence())
	tracker.RecordTopicIdentity("physical/sensor/ems-esp/boiler_outdoortemp/config", "discovery.ems_esp", "boiler_outdoortemp")
	tracker.MarkKnown("discovery.ems_esp", "boiler_outdoortemp")

	gatewayID, leaf, changed := tracker.MarkRetracted("physical/sensor/ems-esp/boiler_outdoortemp/config")
	if !changed || gatewayID != "discovery.ems_esp" || leaf != "boiler_outdoortemp" {
		t.Fatalf("MarkRetracted = (%q, %q, %v), want (\"discovery.ems_esp\", \"boiler_outdoortemp\", true)", gatewayID, leaf, changed)
	}
	if got := tracker.snapshot("discovery.ems_esp")["boiler_outdoortemp"]; got != StatusKnownNotToExist {
		t.Errorf("status = %q, want %q", got, StatusKnownNotToExist)
	}

	// A second retraction of the same, already-retracted leaf is a no-op.
	if _, _, changed := tracker.MarkRetracted("physical/sensor/ems-esp/boiler_outdoortemp/config"); changed {
		t.Errorf("expected no change retracting an already-known-not-to-exist leaf")
	}
}

func TestDiscoveryExistenceTrackerMarkRetractedUnknownTopicIsNoop(t *testing.T) {
	tracker := newDiscoveryExistenceTracker("")
	gatewayID, leaf, changed := tracker.MarkRetracted("physical/sensor/ems-esp/never_seen/config")
	if changed || gatewayID != "" || leaf != "" {
		t.Errorf("MarkRetracted for an unrecorded topic = (%q, %q, %v), want (\"\", \"\", false)", gatewayID, leaf, changed)
	}
}

// TestDiscoveryExistenceTrackerPersistsAcrossRestart mirrors
// TestEntityExistenceTrackerPersistsAcrossRestart (entity_existence_test.go) for the kind-2 tracker.
func TestDiscoveryExistenceTrackerPersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "discovery_existence.json")

	first := newDiscoveryExistenceTracker(path)
	first.Seed(fixtureDiscoveryFileForExistence())
	first.MarkKnown("discovery.ems_esp", "boiler_outdoortemp")

	second := newDiscoveryExistenceTracker(path)
	second.Seed(fixtureDiscoveryFileForExistence())

	if got := second.snapshot("discovery.ems_esp")["boiler_outdoortemp"]; got != StatusKnownToExist {
		t.Errorf("status not restored across restart, got %q, want %q", got, StatusKnownToExist)
	}
}
