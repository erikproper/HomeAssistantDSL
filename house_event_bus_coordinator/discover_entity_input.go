/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: DiscoverEntityInput
 *
 * The human-provided-seed resolution to Architecture.md §6.9 ("entity-existence discovery always
 * needs a seed"): entity_existence.go's DiscoverSiblings can only enrich a device it already has
 * *some* tracked anchor entity for -- there is no way to discover an entirely new, never-declared
 * device without either a periodic full-instance scan (ruled out, that's the 2026-08-27 incident)
 * or an RPC-style "list everything" call (rejected, same unbounded-cost problem from a different
 * angle). So bootstrapping a new device is made an explicit, rare, human-triggered action instead
 * of an automatic one: a single MQTT `text` entity, discovery-published once, whose command_topic
 * this file subscribes to. Bypasses the normal per-instance pacing entirely (that pacing exists to
 * protect an instance from the *automatic* loop, not from one rare, deliberate human action).
 *
 * Two accepted input shapes (parseDiscoverEntityRequest):
 *   - "<entity_id>" alone -- inquired about on *every* declared instance, since a bare entity_id
 *     doesn't say which one it belongs to, and asking all of them is cheap.
 *   - "<instance>: <entity_id>" (colon, then exactly one space) -- routed to that one instance
 *     only. HA entity_ids are restricted to [a-z0-9_.], so ": " can never appear inside one,
 *     making the split always unambiguous.
 *
 * Multiple requests, each in either shape, may be submitted in one payload separated by ";" --
 * splitDiscoverEntityRequests -- so a whole pasted list (one entity per line, each ending in ";",
 * each optionally with its own "<instance>: " prefix) seeds and inquires about every one of them
 * from a single text-field submission, instead of needing one submission per entity.
 *
 * Once that seed resolves, the ordinary DiscoverSiblings mechanism takes over exactly as it does
 * for any other anchor -- no separate discovery logic needed here beyond getting the first entity
 * into the tracker and asking about it right away.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 28.08.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// discoverEntityStableID is this meta entity's own stable discovery id -- fixed, not tied to any
// device or instance, since it's a single coordinator-owned control, not a per-instance one.
const discoverEntityStableID = "meta_discover_entity"

// discoverEntityCommandTopic is where the coordinator listens for a manually-typed fully qualified
// entity_id to bootstrap.
func discoverEntityCommandTopic() string {
	return "meta/discover_entity/set"
}

// publishDiscoverEntityInput discovery-publishes the single MQTT text entity a person uses to seed
// a brand-new device. Optimistic (no state_topic) -- nothing needs to report a value back, HA just
// keeps showing whatever was last typed until the person clears or replaces it themselves.
func publishDiscoverEntityInput(client mqtt.Client, conceptualPrefix string) error {
	topic := discoveryTopic(conceptualPrefix, "text", discoverEntityStableID)
	body := map[string]interface{}{
		"unique_id": discoverEntityStableID,
		// object_id pins the entity_id itself (text.meta_discover_entity) independent of "name" --
		// HA would otherwise slugify "name" into the entity_id, which previously left this at
		// text.discover_entity (no "meta_" prefix) even though every other meta control's own
		// entity_id already carries it.
		"object_id":       discoverEntityStableID,
		"name":            "meta/discover_entity",
		"icon":            "mdi:text-search",
		"command_topic":   discoverEntityCommandTopic(),
		"optimistic":      true,
		"entity_category": "config",
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshalling discover-entity discovery payload: %w", err)
	}
	return publishRetained(client, topic, data)
}

// discoverEntityRequestSeparator splits an "<instance>: <entity_id>" request -- colon followed by
// exactly one space, chosen because no valid HA entity_id can ever contain it.
const discoverEntityRequestSeparator = ": "

// parseDiscoverEntityRequest splits payload on discoverEntityRequestSeparator. ok is false when
// payload has no such prefix (or either side would be empty), meaning it's a bare entity_id to
// broadcast to every declared instance instead of one specific one.
func parseDiscoverEntityRequest(payload string) (instance, entityID string, ok bool) {
	idx := strings.Index(payload, discoverEntityRequestSeparator)
	if idx < 0 {
		return "", "", false
	}
	instance = payload[:idx]
	entityID = strings.TrimSpace(payload[idx+len(discoverEntityRequestSeparator):])
	if instance == "" || entityID == "" {
		return "", "", false
	}
	return instance, entityID, true
}

// declaredInstance reports whether instance is one of the house's own declared instances.
func declaredInstance(instances []string, instance string) bool {
	for _, name := range instances {
		if name == instance {
			return true
		}
	}
	return false
}

// splitDiscoverEntityRequests splits payload on ";" into individual "<entity_id>" or
// "<instance>: <entity_id>" requests, trimming each and dropping blank pieces -- a trailing ";",
// or blank lines from a multi-line paste. Lets a whole list of entities (one per line, each ending
// in ";") be submitted in a single "Discover entity" text-field write.
func splitDiscoverEntityRequests(payload string) []string {
	var requests []string
	for _, part := range strings.Split(payload, ";") {
		part = strings.TrimSpace(part)
		if part != "" {
			requests = append(requests, part)
		}
	}
	return requests
}

// subscribeDiscoverEntityRequests subscribes to discoverEntityCommandTopic() and, on every
// message, splits the payload into individual requests (splitDiscoverEntityRequests) and, for
// each, parses it (parseDiscoverEntityRequest) and seeds + immediately inquires about the
// requested entity_id -- on just the named instance if one was given (and it's a recognised one;
// otherwise that one request is logged and dropped, not misdirected at a topic nothing listens to
// -- the rest of the batch still proceeds), or on every declared instance for a bare entity_id.
// No-op if instances is empty.
func subscribeDiscoverEntityRequests(client mqtt.Client, tracker *TEntityExistenceTracker, instances []string) error {
	if len(instances) == 0 {
		return nil
	}
	handler := func(_ mqtt.Client, msg mqtt.Message) {
		payload := strings.TrimSpace(string(msg.Payload()))
		if payload == "" {
			return
		}
		for _, request := range splitDiscoverEntityRequests(payload) {
			if instance, entityID, ok := parseDiscoverEntityRequest(request); ok {
				if !declaredInstance(instances, instance) {
					fmt.Printf("[existence] manual discovery request %q: %q is not a declared instance, ignoring\n", request, instance)
					continue
				}
				fmt.Printf("[existence] manual discovery request: %s (instance %q only)\n", entityID, instance)
				tracker.SeedManualEntity(instance, entityID)
				tracker.InquireNow(client, instance, entityID)
				continue
			}
			entityID := request
			fmt.Printf("[existence] manual discovery request: %s (every declared instance)\n", entityID)
			for _, instance := range instances {
				tracker.SeedManualEntity(instance, entityID)
				tracker.InquireNow(client, instance, entityID)
			}
		}
	}
	topic := discoverEntityCommandTopic()
	token := client.Subscribe(topic, 0, handler)
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		if err := token.Error(); err != nil {
			return fmt.Errorf("subscribing to %s: %w", topic, err)
		}
		return fmt.Errorf("subscribing to %s: timed out", topic)
	}
	return nil
}
