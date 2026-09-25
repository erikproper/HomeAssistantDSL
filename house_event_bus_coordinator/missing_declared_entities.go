/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: MissingDeclaredEntities
 *
 * PROJECT.md item 1a (2026-09-21, "stable ID-based link" architecture) -- the live half of
 * principle 2 (memory: project_stable_discovery_identity_architecture.md): "if a previously-known
 * source vanishes, this must be reported, not silently acted on." The itemized-at-generate-time
 * half is homeassistant/missing_entities_report.go (suggestions/missing.txt); this file is the
 * always-visible-in-HA half -- one coordinator-authored binary_sensor, device_class "problem",
 * that's "on" whenever ANY declared entity across kind-2 (discovery), kind-3/kind-5 (hassbridge/
 * main-instance, one shared tracker), or kind-4 (import) is currently StatusKnownNotToExist.
 *
 * Discovery-published directly by the coordinator, never relayed from a gateway -- mirrors
 * meta_reload_restart.go's own map-built style exactly (no "device" block, local broker only: this
 * is a house-local HA indicator, never cloud-relayed, same reasoning as the meta buttons' own
 * per-instance topics).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 21.09.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// missingDeclaredEntitiesStableID is this binary_sensor's own unique_id/object_id -- one fixed
// entity, not one per kind, since the itemized breakdown belongs in this entity's own attributes
// (PublishNow) and the generate-time report, not in a proliferation of near-identical entities.
const missingDeclaredEntitiesStableID = "missing_declared_entities"

// missingDeclaredEntitiesDiscoveryTopic mirrors every other coordinator-authored discovery entity
// in this codebase (discoveryTopic, discovery.go) -- conceptualPrefix-qualified, same as the meta
// reload/restart buttons.
func missingDeclaredEntitiesDiscoveryTopic(conceptualPrefix string) string {
	return discoveryTopic(conceptualPrefix, "binary_sensor", missingDeclaredEntitiesStableID)
}

// missingDeclaredEntitiesStateTopic/missingDeclaredEntitiesAttributesTopic are bare (not
// conceptualPrefix-qualified) -- this entity's own state/attributes are coordinator-internal
// plumbing, never relayed anywhere else, so there's no collision risk to guard against the way a
// relayed gateway topic would need.
func missingDeclaredEntitiesStateTopic() string {
	return "coordinator/missing_declared_entities/state"
}

func missingDeclaredEntitiesAttributesTopic() string {
	return "coordinator/missing_declared_entities/attributes"
}

// expectedMissingDeclaredEntitiesTopics is the one discovery topic publishMissingDeclaredEntitiesDiscoveryConfig
// produces -- must be folded into main.go's own expectedTopics set or watchForOrphanedDiscoveryTopics
// retires it moments after it's first published, exactly the same real bug
// expectedMetaReloadRestartTopics' own doc comment describes (found live 2026-09-05) for the meta
// buttons.
func expectedMissingDeclaredEntitiesTopics(conceptualPrefix string) map[string]bool {
	return map[string]bool{missingDeclaredEntitiesDiscoveryTopic(conceptualPrefix): true}
}

// publishMissingDeclaredEntitiesDiscoveryConfig discovery-publishes the one binary_sensor -- local
// broker only, called once at startup and again on every reconnect (main.go), alongside
// publishMetaReloadRestartButtons' own identical call sites.
func publishMissingDeclaredEntitiesDiscoveryConfig(client mqtt.Client, conceptualPrefix, installation string) error {
	topic := missingDeclaredEntitiesDiscoveryTopic(conceptualPrefix)
	body := map[string]interface{}{
		"unique_id":             missingDeclaredEntitiesStableID,
		"object_id":             missingDeclaredEntitiesStableID,
		"name":                  "Missing declared entities",
		"device_class":          "problem",
		"icon":                  "mdi:link-off",
		"state_topic":           missingDeclaredEntitiesStateTopic(),
		"payload_on":            "true",
		"payload_off":           "false",
		"json_attributes_topic": missingDeclaredEntitiesAttributesTopic(),
		"entity_category":       "diagnostic",
		"origin":                coordinatorOriginMap(installation),
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshalling missing declared entities discovery config: %w", err)
	}
	return publishRetained(client, topic, data)
}

// TMissingDeclaredEntitiesPublisher debounces/schedules PublishNow -- same shape as
// TDiscoveryExistenceTracker's own ScheduleAggregateStatusPublish/StartPeriodicAggregateStatusPublish
// pair (discovery_existence.go), just as its own small type rather than a tracker method, since
// this aggregates ACROSS three independent trackers rather than belonging to any one of them.
type TMissingDeclaredEntitiesPublisher struct {
	mu            sync.Mutex
	debounceTimer *time.Timer
}

// PublishNow concatenates all three trackers' own KnownNotToExistItems() (nil-safe: a nil tracker
// pointer is simply skipped, so tests/houses without a given kind wired up still work) and
// publishes the aggregate state + itemized attributes, both retained.
func (p *TMissingDeclaredEntitiesPublisher) PublishNow(client mqtt.Client, discoveryTracker *TDiscoveryExistenceTracker, entityTracker *TEntityExistenceTracker, importTracker *TImportExistenceTracker) error {
	var items []string
	if discoveryTracker != nil {
		items = append(items, discoveryTracker.KnownNotToExistItems()...)
	}
	if entityTracker != nil {
		items = append(items, entityTracker.KnownNotToExistItems()...)
	}
	if importTracker != nil {
		items = append(items, importTracker.KnownNotToExistItems()...)
	}

	state := "false"
	if len(items) > 0 {
		state = "true"
	}
	if err := publishRetained(client, missingDeclaredEntitiesStateTopic(), []byte(state)); err != nil {
		return fmt.Errorf("publishing %s: %w", missingDeclaredEntitiesStateTopic(), err)
	}

	data, err := json.Marshal(map[string]interface{}{"count": len(items), "items": items})
	if err != nil {
		return fmt.Errorf("marshalling missing declared entities attributes: %w", err)
	}
	if err := publishRetained(client, missingDeclaredEntitiesAttributesTopic(), data); err != nil {
		return fmt.Errorf("publishing %s: %w", missingDeclaredEntitiesAttributesTopic(), err)
	}
	return nil
}

// MissingDeclaredEntitiesDebounceDelay/MissingDeclaredEntitiesPeriodicInterval mirror
// DiscoveryExistenceStatusDebounceDelay/DiscoveryExistenceStatusPeriodicInterval exactly (same
// crash-loop-avoidance reasoning as that pair's own doc comment -- a retained-backlog replay at
// startup can fire onChange many times in a tight burst).
const (
	MissingDeclaredEntitiesDebounceDelay    = 30 * time.Second
	MissingDeclaredEntitiesPeriodicInterval = 10 * time.Minute
)

// Schedule debounces a PublishNow call by delay -- safe to call directly from a tracker's own
// onChange callback (main.go wires this up), same non-blocking guarantee as
// ScheduleAggregateStatusPublish's own doc comment.
func (p *TMissingDeclaredEntitiesPublisher) Schedule(client mqtt.Client, discoveryTracker *TDiscoveryExistenceTracker, entityTracker *TEntityExistenceTracker, importTracker *TImportExistenceTracker, delay time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.debounceTimer != nil {
		p.debounceTimer.Reset(delay)
		return
	}
	p.debounceTimer = time.AfterFunc(delay, func() {
		p.mu.Lock()
		p.debounceTimer = nil
		p.mu.Unlock()
		if err := p.PublishNow(client, discoveryTracker, entityTracker, importTracker); err != nil {
			fmt.Printf("[missing-declared-entities] debounced publish: %v\n", err)
		}
	})
}

// StartPeriodicPublish starts a background goroutine republishing on a fixed interval cadence, for
// the lifetime of the process -- purely a safety net alongside Schedule, same reasoning as every
// other tracker's own periodic-publish pair in this codebase.
func (p *TMissingDeclaredEntitiesPublisher) StartPeriodicPublish(client mqtt.Client, discoveryTracker *TDiscoveryExistenceTracker, entityTracker *TEntityExistenceTracker, importTracker *TImportExistenceTracker, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			if err := p.PublishNow(client, discoveryTracker, entityTracker, importTracker); err != nil {
				fmt.Printf("[missing-declared-entities] periodic publish: %v\n", err)
			}
		}
	}()
}
