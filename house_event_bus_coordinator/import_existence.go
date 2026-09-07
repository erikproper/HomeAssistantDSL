/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: ImportExistence
 *
 * Kind-4 (cross-house import) existence tracking -- the passive counterpart to entity_existence.go's
 * kind-3 mechanism, structurally mirroring discovery_existence.go's own kind-2 shape almost exactly:
 * the exporting installation's own coordinator already self-announces via retained MQTT discovery
 * configs on the shared cloud broker (discoveryhassbridge.go's own export cross-post), so no active
 * inquiry protocol is needed the way kind-3 needs one -- this coordinator just needs to track and
 * report each declared SHORTHAND import capability's (homeassistant/integration_import_parser.go's
 * "<domain>.<capability>;" form) three-state status, populated passively as a byproduct of
 * discoveryimport.go's existing discovery-config subscription.
 *
 * Scoped to shorthand-declared capabilities only (TImportedCapability.RemoteEntityRef == ""):
 * an explicit-ref capability predates this mechanism and is matched by payload content, not a
 * stable id, so there is nothing meaningful to track its existence by here. This matters more for
 * shorthand than the explicit form ever needed it: a hand-typed RemoteEntityRef at least got a
 * human's eyes on it once; a purely derived stable id never does, so a typo in the DSL author's own
 * capability name (or a since-repositioned/retired remote capability) would otherwise surface as a
 * silently-never-populated local entity with no error anywhere pointing at the cause.
 *
 * Retraction (known-not-to-exist) is deliberately not built yet, matching kind-2's own retained
 * follow-up gap exactly (see that file's own header comment for why) -- every tracked stable id
 * starts not-known-to-exist and can only ever move to known-to-exist today.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 07.09.2026
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

// TImportExistenceTracker holds, per remote installation, every declared shorthand import
// capability's current status, keyed by its own stable id (exportStableID's own scheme).
type TImportExistenceTracker struct {
	mu      sync.Mutex
	path    string
	entries map[string]map[string]TEntityExistenceStatus // remoteInstallation -> stableID -> status
}

// newImportExistenceTracker loads path's previously persisted state, if any -- mirrors
// newDiscoveryExistenceTracker exactly (never fails, missing/incompatible falls back to empty;
// path == "" disables persistence outright).
func newImportExistenceTracker(path string) *TImportExistenceTracker {
	t := &TImportExistenceTracker{path: path, entries: map[string]map[string]TEntityExistenceStatus{}}
	t.loadPersisted()
	return t
}

// importExistencePersistedFile is import_existence.json's own on-disk shape.
type importExistencePersistedFile struct {
	Installations map[string]map[string]TEntityExistenceStatus `json:"installations"`
}

func (t *TImportExistenceTracker) loadPersisted() {
	if t.path == "" {
		return
	}
	data, err := os.ReadFile(t.path)
	if err != nil {
		return
	}
	var persisted importExistencePersistedFile
	if err := json.Unmarshal(data, &persisted); err != nil {
		return
	}
	for installation, stableIDs := range persisted.Installations {
		t.entries[installation] = stableIDs
	}
}

func (t *TImportExistenceTracker) persist() {
	if t.path == "" {
		return
	}
	data, err := json.MarshalIndent(importExistencePersistedFile{Installations: t.entries}, "", "  ")
	if err != nil {
		fmt.Printf("[import-existence] marshalling persisted state: %v\n", err)
		return
	}
	if err := os.WriteFile(t.path, data, 0o644); err != nil {
		fmt.Printf("[import-existence] persisting state to %s: %v\n", t.path, err)
	}
}

// Seed registers every declared shorthand import capability's own stable id as not-known-to-exist
// -- the generator's own "assumed to exist" list for kind-4, exactly like discovery_existence.go's
// Seed reads discovery.yaml's EntityLinks for kind-2. An explicit-ref capability
// (RemoteEntityRef != "") is skipped entirely -- see this file's own header comment. Already-tracked
// stable ids are left untouched (status preserved as-is), so a coordinator restart never resets an
// already-observed one back to unknown.
func (t *TImportExistenceTracker) Seed(importFile TImportedFile) {
	t.mu.Lock()
	defer t.mu.Unlock()
	declared := map[string]map[string]bool{} // installation -> stableID -> still declared this round
	for _, device := range importFile.Devices {
		if device.RemoteInstallation == "" || device.RemoteDeviceID == "" {
			continue
		}
		for capability, cap := range device.Capabilities {
			if cap.RemoteEntityRef != "" {
				continue
			}
			stableID := exportStableID(device.RemoteInstallation, device.RemoteDeviceID, capability)
			if _, ok := t.entries[device.RemoteInstallation]; !ok {
				t.entries[device.RemoteInstallation] = map[string]TEntityExistenceStatus{}
			}
			if declared[device.RemoteInstallation] == nil {
				declared[device.RemoteInstallation] = map[string]bool{}
			}
			declared[device.RemoteInstallation][stableID] = true
			if _, seen := t.entries[device.RemoteInstallation][stableID]; seen {
				continue
			}
			t.entries[device.RemoteInstallation][stableID] = StatusNotKnownToExist
		}
	}

	// Prune confirmed-dead ghost entries no longer declared anywhere -- mirrors
	// discovery_existence.go's own Seed pruning exactly, same reasoning and safety property.
	for installation, stableIDs := range t.entries {
		for stableID, status := range stableIDs {
			if status != StatusKnownNotToExist || declared[installation][stableID] {
				continue
			}
			delete(t.entries[installation], stableID)
			fmt.Printf("[import-existence] %s: pruned %q -- confirmed not-to-exist and no longer declared\n", installation, stableID)
		}
	}

	t.persist()
}

// MarkKnown records stableID as known-to-exist on remoteInstallation -- called by
// discoveryimport.go's own configHandler the moment a shorthand-matched discovery config arrives,
// whether or not this is the first time. Returns false (a no-op past the first observation) if
// stableID is already known-to-exist, so callers can skip a redundant publish on every retained-
// message replay at startup.
func (t *TImportExistenceTracker) MarkKnown(remoteInstallation, stableID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.entries[remoteInstallation]; !ok {
		t.entries[remoteInstallation] = map[string]TEntityExistenceStatus{}
	}
	if t.entries[remoteInstallation][stableID] == StatusKnownToExist {
		return false
	}
	t.entries[remoteInstallation][stableID] = StatusKnownToExist
	t.persist()
	return true
}

// PublishAll immediately (re-)publishes every remote installation importFile declares -- called
// once right after Seed, mirroring discovery_existence.go's own PublishAll (see its doc comment for
// the full "no periodic self-heal loop, so a coordinator restart needs an immediate publish" reasoning,
// unchanged here).
func (t *TImportExistenceTracker) PublishAll(mainClient, cloudClient mqtt.Client, ownInstallation string, importFile TImportedFile) {
	installations := map[string]bool{}
	for _, device := range importFile.Devices {
		if device.RemoteInstallation != "" {
			installations[device.RemoteInstallation] = true
		}
	}
	for installation := range installations {
		if err := t.publishStatus(mainClient, cloudClient, ownInstallation, installation); err != nil {
			fmt.Printf("[import-existence] %v\n", err)
		}
	}
}

func (t *TImportExistenceTracker) snapshot(remoteInstallation string) map[string]TEntityExistenceStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]TEntityExistenceStatus, len(t.entries[remoteInstallation]))
	for stableID, status := range t.entries[remoteInstallation] {
		out[stableID] = status
	}
	return out
}

func importExistenceStatusTopic(remoteInstallation string) string {
	return "import_existence/" + remoteInstallation + "/existence/state"
}

// publishStatus publishes remoteInstallation's current status snapshot to mainClient (bare topic)
// and -- when cloudClient is non-nil -- also to cloudClient, retained, qualified with
// ownInstallation (this coordinator's own name, not remoteInstallation) -- same installation-
// collision-avoidance reasoning as entity_existence.go/discovery_existence.go's own publishStatus:
// two houses importing from the same third installation would otherwise silently overwrite each
// other's retained status for it on the shared cloud broker.
func (t *TImportExistenceTracker) publishStatus(mainClient, cloudClient mqtt.Client, ownInstallation, remoteInstallation string) error {
	snapshot := t.snapshot(remoteInstallation)
	payload := make(map[string]string, len(snapshot))
	for stableID, status := range snapshot {
		payload[stableID] = string(status)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling import existence status for %s: %w", remoteInstallation, err)
	}
	topic := importExistenceStatusTopic(remoteInstallation)
	if err := publishRetained(mainClient, topic, data); err != nil {
		return fmt.Errorf("publishing %s (local): %w", topic, err)
	}
	if cloudClient != nil {
		cloudTopic := ownInstallation + "/" + topic
		if err := publishRetained(cloudClient, cloudTopic, data); err != nil {
			return fmt.Errorf("publishing %s (cloud): %w", cloudTopic, err)
		}
	}
	return nil
}
