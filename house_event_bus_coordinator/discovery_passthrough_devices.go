/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: DiscoveryPassthroughDevices
 *
 * The missing half of PROJECT.md 1.8's suggestion mechanism for PROJECT.md item 4's passthrough
 * relay (discoverybridge.go): discovery_existence.go's own suggestion report only ever covers
 * undeclared LEAVES of an ALREADY-declared "discovery" gateway (a gatewayID is required just to
 * key its tracking), so a device passthrough is relaying byte-for-byte -- one no declared gateway
 * claims at all yet -- was entirely invisible to the operator, defeating a good chunk of the
 * point of a *gradual* migration (found live 2026-09-14: real Zigbee2MQTT traffic was flowing
 * through passthrough but never showed up in suggestions/discovery.txt).
 *
 * Keyed by the device's own real identifier (the raw payload's device.identifiers, not a
 * coordinator-invented slug -- Zigbee2MQTT already provides a genuinely stable one per device), so
 * the generator's suggestion report can emit a ready-to-paste "device discovery.<slug> with:
 * identifiers "<real-id>"; ...; end;" block per still-undeclared device. A device is forgotten the
 * moment it starts matching a declared gateway (discoverybridge.go's own passthrough branch calls
 * Forget alongside its existing raw-topic retirement), so a migrated device stops being suggested
 * for migration it has already undergone.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 14.09.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// TPassthroughLeaf is one observed capability under an undeclared passthrough device -- domain is
// the real HA component, taken directly from the discovery topic (unlike discovery_existence.go's
// own leaf tracking, passthrough always knows the true domain, never needs to guess it).
type TPassthroughLeaf struct {
	Domain string `json:"domain"`
	Leaf   string `json:"leaf"` // the gateway's own unique_id
}

// tPassthroughDeviceEntry is one undeclared device's own tracked state.
type tPassthroughDeviceEntry struct {
	Name   string                      `json:"name"` // the device's own human-readable name, if reported
	Leaves map[string]TPassthroughLeaf `json:"leaves"`
}

// TPassthroughDeviceTracker tracks every device discoverybridge.go's passthrough relay has
// observed but that no declared "discovery" gateway claims yet.
type TPassthroughDeviceTracker struct {
	mu      sync.Mutex
	path    string // "" disables persistence entirely (tests)
	devices map[string]*tPassthroughDeviceEntry
}

type passthroughDevicesPersistedFile struct {
	Devices map[string]*tPassthroughDeviceEntry `json:"devices"`
}

// newPassthroughDeviceTracker loads path's previously persisted state, if any -- mirrors
// newDiscoveryExistenceTracker exactly (never fails, missing/incompatible falls back to empty).
func newPassthroughDeviceTracker(path string) *TPassthroughDeviceTracker {
	t := &TPassthroughDeviceTracker{path: path, devices: map[string]*tPassthroughDeviceEntry{}}
	t.loadPersisted()
	return t
}

func (t *TPassthroughDeviceTracker) loadPersisted() {
	if t.path == "" {
		return
	}
	data, err := os.ReadFile(t.path)
	if err != nil {
		return
	}
	var persisted passthroughDevicesPersistedFile
	if err := json.Unmarshal(data, &persisted); err != nil {
		return
	}
	if persisted.Devices != nil {
		t.devices = persisted.Devices
	}
}

func (t *TPassthroughDeviceTracker) persist() {
	if t.path == "" {
		return
	}
	data, err := json.MarshalIndent(passthroughDevicesPersistedFile{Devices: t.devices}, "", "  ")
	if err != nil {
		fmt.Printf("[discovery-passthrough] marshalling persisted state: %v\n", err)
		return
	}
	if err := os.WriteFile(t.path, data, 0o644); err != nil {
		fmt.Printf("[discovery-passthrough] persisting state to %s: %v\n", t.path, err)
	}
}

// Record notes deviceIdentifier (the raw device's own first identifier) has leaf (component +
// its own unique_id) -- called for every passthrough-relayed, non-empty message. A "" identifier
// or leaf is a no-op: nothing stable to key on. name, when non-empty, updates the device's own
// last-known human-readable name. Returns true the first time this exact fact is recorded, so
// callers can skip a redundant publish on every retained-message replay at startup.
func (t *TPassthroughDeviceTracker) Record(deviceIdentifier, name, domain, leaf string) bool {
	if deviceIdentifier == "" || leaf == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	entry, ok := t.devices[deviceIdentifier]
	if !ok {
		entry = &tPassthroughDeviceEntry{Leaves: map[string]TPassthroughLeaf{}}
		t.devices[deviceIdentifier] = entry
	}
	changed := false
	if name != "" && entry.Name != name {
		entry.Name = name
		changed = true
	}
	if existing, seen := entry.Leaves[leaf]; !seen || existing.Domain != domain {
		entry.Leaves[leaf] = TPassthroughLeaf{Domain: domain, Leaf: leaf}
		changed = true
	}
	if changed {
		t.persist()
	}
	return changed
}

// Forget removes deviceIdentifier entirely -- called the moment a device starts matching a
// declared gateway, so a migrated device is never suggested again. Returns false (a no-op) when
// deviceIdentifier was never tracked to begin with -- the overwhelmingly common case for ordinary
// declared-gateway traffic unrelated to passthrough at all -- so callers can skip a redundant
// status publish.
func (t *TPassthroughDeviceTracker) Forget(deviceIdentifier string) bool {
	if deviceIdentifier == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.devices[deviceIdentifier]; !ok {
		return false
	}
	delete(t.devices, deviceIdentifier)
	t.persist()
	return true
}

// snapshot returns a deep-enough copy of the current device set for publishing.
func (t *TPassthroughDeviceTracker) snapshot() map[string]tPassthroughDeviceEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]tPassthroughDeviceEntry, len(t.devices))
	for id, entry := range t.devices {
		leaves := make(map[string]TPassthroughLeaf, len(entry.Leaves))
		for k, v := range entry.Leaves {
			leaves[k] = v
		}
		out[id] = tPassthroughDeviceEntry{Name: entry.Name, Leaves: leaves}
	}
	return out
}

// passthroughDevicesStatusTopic is the retained topic this tracker's full snapshot is published
// to -- homeassistant/discovery_passthrough_suggestions.go's own copy of this same literal string
// is what the generator subscribes to at generate time (mirrors discoveryExistenceStatusTopic's
// own cross-package duplication -- the two packages never share Go types).
const passthroughDevicesStatusTopic = "discovery_passthrough/devices/state"

// PublishStatus publishes the tracker's current full device set, retained, to mainClient and --
// when cloudClient is non-nil -- also to cloudClient, prefixed with ownInstallation (same
// collision-avoidance reasoning as discovery_existence.go's own publishStatus).
func (t *TPassthroughDeviceTracker) PublishStatus(mainClient, cloudClient mqtt.Client, ownInstallation string) error {
	data, err := json.Marshal(t.snapshot())
	if err != nil {
		return fmt.Errorf("marshalling passthrough device status: %w", err)
	}
	if err := publishRetained(mainClient, passthroughDevicesStatusTopic, data); err != nil {
		return fmt.Errorf("publishing %s (local): %w", passthroughDevicesStatusTopic, err)
	}
	if cloudClient != nil {
		cloudTopic := ownInstallation + "/" + passthroughDevicesStatusTopic
		if err := publishRetained(cloudClient, cloudTopic, data); err != nil {
			return fmt.Errorf("publishing %s (cloud): %w", cloudTopic, err)
		}
	}
	return nil
}
