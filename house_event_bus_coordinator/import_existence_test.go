/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: ImportExistenceTest
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 07.09.2026
 *
 */

package main

import "testing"

func fixtureImportFileForExistence() TImportedFile {
	return TImportedFile{Devices: map[string]TImportedDevice{
		"import.vienna_livingroom": {
			RemoteInstallation: "junglinster", RemoteDeviceID: "hass.vienna_livingroom",
			Capabilities: map[string]TImportedCapability{
				"co2": {LocalEntity: "sensor.infrastructural_living_room_co2"},
			},
		},
	}}
}

func TestImportExistenceTrackerSeedStartsNotKnownToExist(t *testing.T) {
	tracker := newImportExistenceTracker("")
	tracker.Seed(fixtureImportFileForExistence())

	stableID := exportStableID("junglinster", "hass.vienna_livingroom", "co2")
	got := tracker.snapshot("junglinster")
	if got[stableID] != StatusNotKnownToExist {
		t.Errorf("snapshot = %+v, want %q not-known-to-exist", got, stableID)
	}
}

func TestImportExistenceTrackerMarkKnownTransitionsAndIsIdempotent(t *testing.T) {
	tracker := newImportExistenceTracker("")
	tracker.Seed(fixtureImportFileForExistence())
	stableID := exportStableID("junglinster", "hass.vienna_livingroom", "co2")

	if changed := tracker.MarkKnown("junglinster", stableID); !changed {
		t.Errorf("first MarkKnown should report a change")
	}
	if got := tracker.snapshot("junglinster")[stableID]; got != StatusKnownToExist {
		t.Errorf("status = %q, want %q", got, StatusKnownToExist)
	}
	if changed := tracker.MarkKnown("junglinster", stableID); changed {
		t.Errorf("second MarkKnown for an already-known stable id should report no change")
	}
}

func TestImportExistenceTrackerPublishStatusRelaysToLocalAndCloud(t *testing.T) {
	tracker := newImportExistenceTracker("")
	tracker.Seed(fixtureImportFileForExistence())
	stableID := exportStableID("junglinster", "hass.vienna_livingroom", "co2")
	tracker.MarkKnown("junglinster", stableID)

	mainClient := &fakeClient{}
	cloudClient := &fakeClient{}
	if err := tracker.publishStatus(mainClient, cloudClient, "vienna", "junglinster"); err != nil {
		t.Fatalf("publishStatus error: %v", err)
	}

	wantLocalTopic := importExistenceStatusTopic("junglinster")
	wantCloudTopic := "vienna/" + wantLocalTopic
	foundLocal, foundCloud := false, false
	for _, p := range mainClient.publishedSnapshot() {
		if p.topic == wantLocalTopic {
			foundLocal = true
		}
	}
	for _, p := range cloudClient.publishedSnapshot() {
		if p.topic == wantCloudTopic {
			foundCloud = true
		}
	}
	if !foundLocal {
		t.Errorf("expected a local publish to %q", wantLocalTopic)
	}
	if !foundCloud {
		t.Errorf("expected a cloud publish to %q (qualified with ownInstallation, not remoteInstallation)", wantCloudTopic)
	}
}

func TestImportExistenceTrackerPublishAllPublishesEveryDeclaredRemoteInstallation(t *testing.T) {
	tracker := newImportExistenceTracker("")
	importFile := fixtureImportFileForExistence()
	tracker.Seed(importFile)

	mainClient := &fakeClient{}
	tracker.PublishAll(mainClient, nil, "vienna", importFile)

	wantTopic := importExistenceStatusTopic("junglinster")
	found := false
	for _, p := range mainClient.publishedSnapshot() {
		if p.topic == wantTopic {
			found = true
		}
	}
	if !found {
		t.Errorf("expected PublishAll to publish %q for the one declared remote installation", wantTopic)
	}
}

// TestImportExistenceTrackerSeedPrunesConfirmedDeadOrphans mirrors
// TestDiscoveryExistenceTrackerSeedPrunesConfirmedDeadOrphans exactly -- a confirmed-absent stable
// id no longer declared anywhere must not sit in persisted state forever.
func TestImportExistenceTrackerSeedPrunesConfirmedDeadOrphans(t *testing.T) {
	tracker := newImportExistenceTracker("")
	importFile := fixtureImportFileForExistence()
	tracker.Seed(importFile)
	stableID := exportStableID("junglinster", "hass.vienna_livingroom", "co2")

	tracker.mu.Lock()
	tracker.entries["junglinster"][stableID] = StatusKnownNotToExist
	tracker.mu.Unlock()

	// Re-seed with the capability removed entirely.
	tracker.Seed(TImportedFile{})

	got := tracker.snapshot("junglinster")
	if _, stillTracked := got[stableID]; stillTracked {
		t.Errorf("snapshot = %+v, want the confirmed-absent, no-longer-declared stable id pruned", got)
	}
}

// TestImportExistenceTrackerSeedNeverPrunesStillDeclaredEntries confirms a still-declared entry
// -- even one confirmed known-not-to-exist -- is never pruned, matching kind-2's own safety property.
func TestImportExistenceTrackerSeedNeverPrunesStillDeclaredEntries(t *testing.T) {
	tracker := newImportExistenceTracker("")
	importFile := fixtureImportFileForExistence()
	tracker.Seed(importFile)
	stableID := exportStableID("junglinster", "hass.vienna_livingroom", "co2")

	tracker.mu.Lock()
	tracker.entries["junglinster"][stableID] = StatusKnownNotToExist
	tracker.mu.Unlock()

	tracker.Seed(importFile) // same file, capability still declared

	got := tracker.snapshot("junglinster")
	if got[stableID] != StatusKnownNotToExist {
		t.Errorf("snapshot = %+v, want the still-declared entry preserved as known-not-to-exist, not pruned or reset", got)
	}
}
