package main

import (
	"path/filepath"
	"testing"
)

// TestLiveDeviceInfoStoreUpdateFiresOnChangeOnRealChange is a regression test for the
// "change in store => change in config" contract (liveinfo.go's own doc comment): onChange must
// fire when Update actually introduces a new or different field value.
func TestLiveDeviceInfoStoreUpdateFiresOnChangeOnRealChange(t *testing.T) {
	var notified []string
	store := NewLiveDeviceInfoStore("", func(deviceID string) { notified = append(notified, deviceID) })

	store.Update("hass.davids_bedroom", map[string]string{"manufacturer": "Netatmo"})
	if len(notified) != 1 || notified[0] != "hass.davids_bedroom" {
		t.Fatalf("notified = %+v, want one call for hass.davids_bedroom", notified)
	}
}

// TestLiveDeviceInfoStoreUpdateSkipsOnChangeWhenNothingChanged confirms a caller doesn't get
// spammed on every unchanged periodic report (e.g. a Netatmo device re-reporting the same
// manufacturer/model on every 10-minute update).
func TestLiveDeviceInfoStoreUpdateSkipsOnChangeWhenNothingChanged(t *testing.T) {
	var calls int
	store := NewLiveDeviceInfoStore("", func(string) { calls++ })

	store.Update("hass.davids_bedroom", map[string]string{"manufacturer": "Netatmo"})
	store.Update("hass.davids_bedroom", map[string]string{"manufacturer": "Netatmo"})
	if calls != 1 {
		t.Errorf("got %d onChange calls, want exactly 1 (the second Update repeated identical values)", calls)
	}

	store.Update("hass.davids_bedroom", map[string]string{})
	if calls != 1 {
		t.Errorf("got %d onChange calls after an empty-fields Update, want still 1", calls)
	}
}

// TestLiveDeviceInfoStoreNilOnChangeIsSafe confirms a store with no callback (most tests, and any
// caller not wired into the reactive path) never panics.
func TestLiveDeviceInfoStoreNilOnChangeIsSafe(t *testing.T) {
	store := NewLiveDeviceInfoStore("", nil)
	store.Update("hass.davids_bedroom", map[string]string{"manufacturer": "Netatmo"})
	if got := store.Snapshot("hass.davids_bedroom")["manufacturer"]; got != "Netatmo" {
		t.Errorf("manufacturer = %q, want %q", got, "Netatmo")
	}
}

// TestLiveDeviceInfoStorePersistsAcrossRestarts is a regression test for a real gap found live
// 2026-09-08, the same day it was introduced: a hosts device's own fields resume for free after a
// coordinator restart via a retained MQTT topic, but a hassbridge device's manufacturer/model
// learned only from an entity-existence inquiry reply has no such retained topic behind it -- an
// unpersisted store would silently downgrade that device's discovery config on every single
// coordinator restart (i.e. every deploy) until the next inquiry happened to reach it. Simulates a
// restart by constructing a second store against the same path.
func TestLiveDeviceInfoStorePersistsAcrossRestarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "live_device_info.json")

	first := NewLiveDeviceInfoStore(path, nil)
	first.Update("hass.davids_bedroom", map[string]string{"manufacturer": "Netatmo", "model": "Indoor Module"})

	restarted := NewLiveDeviceInfoStore(path, nil)
	got := restarted.Snapshot("hass.davids_bedroom")
	if got["manufacturer"] != "Netatmo" || got["model"] != "Indoor Module" {
		t.Errorf("Snapshot after simulated restart = %+v, want manufacturer/model to survive", got)
	}
}

// TestLiveDeviceInfoStoreMissingFileStartsEmpty confirms a store with no persisted file yet (first
// ever coordinator run) starts cleanly empty rather than erroring.
func TestLiveDeviceInfoStoreMissingFileStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")
	store := NewLiveDeviceInfoStore(path, nil)
	if got := store.Snapshot("hass.anything"); len(got) != 0 {
		t.Errorf("Snapshot = %+v, want empty for a fresh store with no persisted file", got)
	}
}

// TestLiveDeviceInfoStoreSetOnChangeRewires confirms SetOnChange lets a caller wire the callback
// after construction -- main.go needs this since the callback's own dependencies (devicesFile,
// hassBridgeFile, publisher, ...) aren't all available at the point the store itself is created.
func TestLiveDeviceInfoStoreSetOnChangeRewires(t *testing.T) {
	store := NewLiveDeviceInfoStore("", nil)
	var notified string
	store.SetOnChange(func(deviceID string) { notified = deviceID })

	store.Update("hass.eriks_iphone", map[string]string{"model": "iPhone16,1"})
	if notified != "hass.eriks_iphone" {
		t.Errorf("notified = %q, want %q", notified, "hass.eriks_iphone")
	}
}
