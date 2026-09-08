/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: LiveInfo
 *
 * In-memory store for the live device metadata reported on each device's ".../device/state"
 * MQTT topic (retained, so a fresh subscription immediately redelivers the last-known values
 * after a coordinator restart -- no separate bootstrap needed). Concurrency-safe: MQTT message
 * handlers run in their own goroutine (mqtt.go), and the store is read from the main goroutine's
 * startup publish pass too.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 22.08.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// TLiveDeviceInfoStore holds the most recently reported device-metadata fields per deviceID.
// Field names are whatever the reporting side sent (knownLiveDeviceInfoFields' six names plus
// "via_device"; "name" is intentionally never stored -- see discovery.go's buildDiscoveryConfigs,
// DisplayName stays authoritative for the discovery device's name).
//
// onChange (added 2026-09-08) makes the store reactive: "change in store => change in config",
// irrespective of which of this store's several callers actually made the change (a hosts
// device's own device-info topic, mqtt.go; a hassbridge device-info topic, discoveryhassbridge.go;
// or an entity-existence inquiry reply carrying manufacturer/model, entity_existence.go). Before
// this, only the first two callers republished discovery immediately after updating the store --
// the third left a device's fresh manufacturer/model sitting in the store, invisible until that
// entity's own next unrelated state-topic update happened to trigger a republish (confirmed live
// 2026-09-08: a hassbridge device's manufacturer/model updated in the store immediately on a
// forced inquiry, but Home Assistant's own discovery config kept showing the old, incomplete
// device block for minutes until the next natural sensor reading). Centralising the "notify on
// change" behaviour in the store itself, rather than duplicating a republish call at every one of
// Update's callers, means a future fourth caller gets this for free too.
//
// path (added 2026-09-08, same day, caught before it ever shipped) persists byDevice to disk --
// without this, a coordinator restart wipes the store clean same as any in-memory cache, but
// unlike entity_existence.json's own tracked status (which resumes from disk) this store had
// nothing to resume from. Harmless for a hosts device's own fields (its ".../device/state" topic
// is retained, so a fresh subscription redelivers them immediately, no gap) and for a hassbridge
// device using DeviceInfoCapabilities (same retained-topic story, remote_instance_automations.go's
// own "coordinator_bridge_report" automation) -- but a hassbridge device's manufacturer/model
// learned only from an entity-existence inquiry reply (entity_existence.go, added 2026-09-08) has
// no retained topic behind it at all: without persistence, EVERY coordinator restart (i.e. every
// deploy) would silently downgrade that device's discovery config back to no manufacturer/model
// until the next inquiry happens to reach it, possibly hours later at the existing round-robin
// pace. Persisted the same way entity_existence.go does: best-effort, missing/unparseable file
// falls back to empty, synchronous write on every actual change.
type TLiveDeviceInfoStore struct {
	mu       sync.Mutex
	path     string // persistPath's own record; "" disables persistence entirely (tests)
	byDevice map[string]map[string]string
	onChange func(deviceID string)
}

// NewLiveDeviceInfoStore returns a store pre-loaded from path, if set and readable (a missing
// file, an empty path, or unparseable content all fall back to starting empty -- this is a
// coordinator-owned runtime cache, always safe to rebuild from nothing, just more slowly). onChange,
// if non-nil, is invoked (outside the store's own lock, so it may safely call back into the store)
// after any Update call that actually changes deviceID's stored fields -- never for a no-op update
// (identical values resent, or an empty fields map), so a caller doesn't get spammed on every
// unchanged periodic report.
func NewLiveDeviceInfoStore(path string, onChange func(deviceID string)) *TLiveDeviceInfoStore {
	s := &TLiveDeviceInfoStore{path: path, byDevice: map[string]map[string]string{}, onChange: onChange}
	s.loadPersisted()
	return s
}

// loadPersisted populates s.byDevice from s.path -- see NewLiveDeviceInfoStore's own doc comment
// for the fallback-to-empty rules.
func (s *TLiveDeviceInfoStore) loadPersisted() {
	if s.path == "" {
		return
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var persisted map[string]map[string]string
	if err := json.Unmarshal(data, &persisted); err != nil {
		return
	}
	s.byDevice = persisted
}

// persist writes s's current byDevice to s.path -- a no-op if persistence is disabled (s.path ==
// ""). Must be called with s.mu already held, mirroring entity_existence.go's own persist.
func (s *TLiveDeviceInfoStore) persist() {
	if s.path == "" {
		return
	}
	data, err := json.MarshalIndent(s.byDevice, "", "  ")
	if err != nil {
		fmt.Printf("[device-info] marshalling persisted state: %v\n", err)
		return
	}
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		fmt.Printf("[device-info] persisting state to %s: %v\n", s.path, err)
	}
}

// Update merges fields into deviceID's stored values, overwriting any previous value for a
// repeated key, then -- if anything actually changed -- persists and calls onChange(deviceID).
func (s *TLiveDeviceInfoStore) Update(deviceID string, fields map[string]string) {
	s.mu.Lock()
	existing := s.byDevice[deviceID]
	if existing == nil {
		existing = map[string]string{}
		s.byDevice[deviceID] = existing
	}
	changed := false
	for name, value := range fields {
		if existing[name] != value {
			existing[name] = value
			changed = true
		}
	}
	if changed {
		s.persist()
	}
	onChange := s.onChange
	s.mu.Unlock()
	if changed && onChange != nil {
		onChange(deviceID)
	}
}

// SetOnChange (re)sets the store's onChange callback -- lets main.go wire it up once every
// dependency the callback itself needs (devicesFile, hassBridgeFile, publisher, ...) is available,
// rather than requiring NewLiveDeviceInfoStore to be called only after all of those are ready.
func (s *TLiveDeviceInfoStore) SetOnChange(onChange func(deviceID string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = onChange
}

// Snapshot returns a copy of deviceID's currently known fields, safe for the caller to read
// without holding the store's lock. Returns an empty (non-nil) map if nothing has been reported
// for deviceID yet.
func (s *TLiveDeviceInfoStore) Snapshot(deviceID string) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := make(map[string]string, len(s.byDevice[deviceID]))
	for name, value := range s.byDevice[deviceID] {
		snapshot[name] = value
	}
	return snapshot
}
