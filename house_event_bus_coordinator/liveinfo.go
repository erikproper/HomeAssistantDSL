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

import "sync"

// TLiveDeviceInfoStore holds the most recently reported device-metadata fields per deviceID.
// Field names are whatever the reporting side sent (knownLiveDeviceInfoFields' six names plus
// "via_device"; "name" is intentionally never stored -- see discovery.go's buildDiscoveryConfigs,
// DisplayName stays authoritative for the discovery device's name).
type TLiveDeviceInfoStore struct {
	mu       sync.Mutex
	byDevice map[string]map[string]string
}

// NewLiveDeviceInfoStore returns an empty store.
func NewLiveDeviceInfoStore() *TLiveDeviceInfoStore {
	return &TLiveDeviceInfoStore{byDevice: map[string]map[string]string{}}
}

// Update merges fields into deviceID's stored values, overwriting any previous value for a
// repeated key.
func (s *TLiveDeviceInfoStore) Update(deviceID string, fields map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing := s.byDevice[deviceID]
	if existing == nil {
		existing = map[string]string{}
		s.byDevice[deviceID] = existing
	}
	for name, value := range fields {
		existing[name] = value
	}
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
