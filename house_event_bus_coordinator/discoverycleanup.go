/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: DiscoveryCleanup
 *
 * Retires orphaned Home Assistant MQTT Discovery topics -- ones this coordinator (or an earlier
 * version of it) previously published under content or a topic-naming scheme it no longer
 * expects, whose retained message would otherwise sit on the broker forever and show up in HA as
 * a stale/duplicate/wrongly-named entity. Two complementary, content-aware mechanisms:
 *
 *   - TDiscoveryPublisher is the single point every discovery config this coordinator owns gets
 *     published through (mqtt.go, discoverybridge.go). It retires (empty-retained) a topic first
 *     only when its content is actually changing from what a small local manifest
 *     (discovery_topics.json, coordinator-owned runtime state) last recorded -- self-healing any
 *     past payload-shape bug the moment its topic is next touched, whether at startup or via a
 *     live update mid-run -- and skips outright when content is unchanged, avoiding needless
 *     entity churn on routine restarts.
 *   - watchForOrphanedDiscoveryTopics is the broker-driven safety net for what the local manifest
 *     can't know: it has no memory of content published *before* it started tracking content (or
 *     before this coordinator process ever ran at all). Subscribing to the broker's own current
 *     retained state and comparing it against what we're actually about to publish closes that
 *     gap without needing any local memory of the bug -- this is what self-heals an
 *     already-wrong registration made under an earlier version of this coordinator's payload
 *     logic.
 *
 * Both are scoped strictly to topics this coordinator itself manages (its own
 * devices.yaml/discovery.yaml entries) -- never a broader archive of everything on the broker.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 23.08.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gopkg.in/yaml.v3"
)

// discoveryOrphanWatchSettleWindow is how long watchForOrphanedDiscoveryTopics waits, once
// subscribed, before returning -- giving the broker time to deliver whatever it currently holds
// retained (and this coordinator time to act on it) before the caller proceeds to republish the
// current expected set. Without this wait, a stale retained message delivered asynchronously
// *after* a fresh, correct republish could retire the topic the fresh publish just fixed. A var,
// not a const, so tests can shrink it instead of eating the real delay.
var discoveryOrphanWatchSettleWindow = 3 * time.Second

// TTopicManifest is the coordinator's own on-disk record of the last-published content of every
// discovery topic it manages -- scoped strictly to its own devices.yaml/discovery.yaml entries,
// never a broader archive of broker traffic.
type TTopicManifest struct {
	Topics map[string]string `json:"topics"` // topic -> last-published JSON payload
}

// loadTopicManifest reads path's content map. A missing file, or one in an incompatible shape
// (e.g. an earlier version's plain topic-list form), both fall back to an empty map rather than
// an error -- this is a coordinator-owned runtime cache, always safe to rebuild from nothing.
func loadTopicManifest(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	var manifest TTopicManifest
	if err := json.Unmarshal(data, &manifest); err != nil || manifest.Topics == nil {
		return map[string]string{}
	}
	return manifest.Topics
}

// TDiscoveryCleanupFile is the top-level shape of a generated coordinator/discovery_cleanup.yaml
// file -- operator-declared legacy node_ids (Physical.def's "mqtt_discovery_clean <topic>;")
// whose entire discovery footprint (every component, every object_id) should be swept clean on
// every restart, for as long as the declaration remains -- enabling a clean migration off an old
// integration (EMS-ESP moving to ${mqtt_discovery_physical}, a future Zigbee2MQTT/Z-Wave
// migration, ...) without hand-cleaning every leftover entity in Home Assistant.
type TDiscoveryCleanupFile struct {
	CleanTopics []string `yaml:"clean_topics"`
}

// loadDiscoveryCleanupFile reads and parses a generator-produced coordinator/discovery_cleanup.yaml
// file, if one exists -- mirrors loadDiscoveryFile's pattern: a missing file returns a zero value,
// not an error, since most houses have nothing declared to clean.
func loadDiscoveryCleanupFile(path string) (TDiscoveryCleanupFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return TDiscoveryCleanupFile{}, nil
		}
		return TDiscoveryCleanupFile{}, fmt.Errorf("cannot read %s: %w", path, err)
	}

	var cleanupFile TDiscoveryCleanupFile
	if err := yaml.Unmarshal(data, &cleanupFile); err != nil {
		return TDiscoveryCleanupFile{}, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	return cleanupFile, nil
}

// topicNodeID extracts a "<prefix>/<component>/<node_id>/<object_id>/config" topic's node_id
// segment, ok=false if topic isn't shaped like a discovery config topic under prefix at all.
// prefix is matched as a literal string prefix (not just a first path segment), so a compound,
// installation-qualified prefix (e.g. "junglinster/homeassistant", the cloud broker's own
// discovery namespace -- see expectedCloudHostsPayloads) works exactly like a plain one.
func topicNodeID(topic, prefix string) (nodeID string, ok bool) {
	rest := strings.TrimPrefix(topic, prefix+"/")
	if rest == topic {
		return "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 4 || parts[3] != "config" {
		return "", false
	}
	return parts[1], true
}

// isCoordinatorOwnedTopic recognises a topic (under prefix) as one the coordinator itself would
// publish -- either the current "coordinator" literal node_id (discovery.go's discoveryTopic), or
// (transitional, until every existing entity has migrated once) a legacy deviceID/gatewayID-based
// node_id ("host_"/"discovery_" prefix) from before topic keys became identity-based.
func isCoordinatorOwnedTopic(topic, prefix string) bool {
	nodeID, ok := topicNodeID(topic, prefix)
	if !ok {
		return false
	}
	return nodeID == "coordinator" || strings.HasPrefix(nodeID, "host_") || strings.HasPrefix(nodeID, "discovery_")
}

// expectedHostsPayloads computes every discovery topic AND its exact expected JSON payload this
// run's devices.yaml implies for the main broker, reusing buildDiscoveryConfigs (discovery.go)
// itself so this can never drift from what actually gets published. Calls deviceRoutingPlan
// directly (mqtt_relay.go) rather than re-deriving its PublishMain logic inline -- this file's own
// header comment already flags that duplication as a drift risk, and it actually drifted once
// already (2026-09-02, when "native" was retired: this function's own inline copy of the check
// would have silently kept excluding a self-imported device from "expected" if it hadn't been
// rewritten to call the one true source of the routing decision instead). hasCloudClient must be
// the real value (not hardcoded true) -- a Cloud-only device that would otherwise be excluded here
// is instead correctly EXCLUDED when hasCloudClient is false, matching deviceRoutingPlan's own
// "no cloud client configured" case.
func expectedHostsPayloads(devicesFile TDevicesFile, hasCloudClient bool) (map[string]string, error) {
	expected := map[string]string{}
	prefix := devicesFile.conceptualPrefix()
	for id, device := range devicesFile.Devices {
		if !deviceRoutingPlan(device, hasCloudClient).PublishMain {
			continue
		}
		for _, cfg := range buildDiscoveryConfigs(id, device, nil, "", prefix) {
			data, err := json.Marshal(cfg.Payload)
			if err != nil {
				return nil, fmt.Errorf("marshalling discovery payload for %s: %w", cfg.Topic, err)
			}
			expected[cfg.Topic] = string(data)
		}
	}
	return expected, nil
}

// expectedCloudHostsPayloads computes every discovery topic AND its exact expected JSON payload
// this run's devices.yaml implies for the *cloud* broker, with each topic qualified by
// ownInstallation+"/" exactly as publishDeviceDiscovery itself qualifies it. Calls
// deviceRoutingPlan directly (mqtt_relay.go), same rationale as expectedHostsPayloads above --
// this is also where the real 2026-09-02 bug would have landed if left as an inline check: a
// self-imported device (Cloud && ImportedFrom == this house's own installation) must still
// publish its cloud catalogue entry, but the old inline "device.ImportedFrom != \"\" -> skip"
// check would have wrongly excluded it the moment ImportedFrom became non-empty for that case.
// hasCloudClient is always true here -- this function is only ever meaningful when a cloud broker
// already exists, which its own caller already gates on.
func expectedCloudHostsPayloads(devicesFile TDevicesFile, ownInstallation string) (map[string]string, error) {
	expected := map[string]string{}
	prefix := devicesFile.conceptualPrefix()
	for id, device := range devicesFile.Devices {
		if !deviceRoutingPlan(device, true).PublishCloud {
			continue
		}
		for _, cfg := range buildDiscoveryConfigs(id, device, nil, "", prefix) {
			data, err := json.Marshal(cfg.Payload)
			if err != nil {
				return nil, fmt.Errorf("marshalling discovery payload for %s: %w", cfg.Topic, err)
			}
			expected[ownInstallation+"/"+cfg.Topic] = string(data)
		}
	}
	return expected, nil
}

// expectedDiscoveryBridgeTopics computes every discovery topic this run's discovery.yaml entity
// links imply -- the topic depends only on gatewayID+leaf (relayedDiscoveryUniqueID,
// discoverybridge.go), both known statically, so no live gateway payload is needed to know which
// topics to expect (only to fill their bodies in later -- so, unlike expectedHostsPayloads, this
// can only report topic existence, not expected content).
func expectedDiscoveryBridgeTopics(discoveryFile TDiscoveryFile, prefix string) map[string]bool {
	expected := map[string]bool{}
	for entityID, link := range discoveryFile.EntityLinks {
		dotIdx := strings.Index(entityID, ".")
		if dotIdx < 0 {
			continue
		}
		domain := entityID[:dotIdx]
		expected[discoveryTopic(prefix, domain, relayedDiscoveryUniqueID(link.Gateway, link.Leaf))] = true
	}
	return expected
}

// topicSet reduces a topic->payload map to a topic set, for callers that only need to know
// whether a topic is expected at all, not its expected content.
func topicSet(payloads map[string]string) map[string]bool {
	set := make(map[string]bool, len(payloads))
	for topic := range payloads {
		set[topic] = true
	}
	return set
}

// publishRetained publishes payload to topic, retained, waiting for broker acknowledgement -- the
// shared low-level primitive every discovery publish (real content or an empty retiring one)
// goes through.
func publishRetained(client mqtt.Client, topic string, payload []byte) error {
	token := client.Publish(topic, 0, true, payload)
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		if err := token.Error(); err != nil {
			return fmt.Errorf("publishing %s: %w", topic, err)
		}
		return fmt.Errorf("publishing %s: timed out", topic)
	}
	return nil
}

// retireDiscoveryTopic publishes an empty retained payload to topic -- the standard MQTT
// discovery removal signal HA responds to by dropping the entity.
func retireDiscoveryTopic(client mqtt.Client, topic string) {
	if err := publishRetained(client, topic, []byte{}); err != nil {
		fmt.Printf("[discovery-cleanup] retiring %s: %v\n", topic, err)
		return
	}
	fmt.Printf("[discovery-cleanup] retired stale topic %s\n", topic)
}

// TDiscoveryPublisher is the single point every discovery config this coordinator owns gets
// published through -- see the component doc comment.
type TDiscoveryPublisher struct {
	mu           sync.Mutex
	manifestPath string
	known        map[string]string
}

// newDiscoveryPublisher loads manifestPath (loadTopicManifest -- never fails, missing/incompatible
// falls back to empty).
func newDiscoveryPublisher(manifestPath string) *TDiscoveryPublisher {
	return &TDiscoveryPublisher{manifestPath: manifestPath, known: loadTopicManifest(manifestPath)}
}

// manifestKey combines broker and topic into p.known's map key -- a broker label, not just the
// bare topic, since the same topic string can legitimately be published to two different broker
// connections (a "cloud"+"import" self-imported device's discovery config, published to both
// "main" and "cloud_coordinator" -- see mqtt_relay.go); keying purely by topic would make the
// second broker's publish look like a no-op repeat of the first's and wrongly skip it.
func manifestKey(broker, topic string) string {
	return broker + "\x00" + topic
}

// Publish publishes payload to topic on client (labelled broker, e.g. "main" or
// "cloud_coordinator" -- see manifestKey), retiring the topic first if -- and only if -- content
// previously recorded there (for that broker) differs from payload; skips the publish entirely
// if content is already exactly what's recorded, avoiding needless entity churn.
func (p *TDiscoveryPublisher) Publish(client mqtt.Client, broker, topic string, payload []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := manifestKey(broker, topic)
	content := string(payload)
	if prev, ok := p.known[key]; ok {
		if prev == content {
			return nil
		}
		retireDiscoveryTopic(client, topic) // content changed -- force a fresh HA registration
	}
	if err := publishRetained(client, topic, payload); err != nil {
		return err
	}
	p.known[key] = content
	if err := p.persist(); err != nil {
		fmt.Printf("[discovery-cleanup] persisting manifest: %v\n", err)
	}
	return nil
}

// RetireMissing retires+forgets every known topic on broker no longer present in expected at all
// (device or entity link removed, or an old topic-naming scheme superseded). Returns what it
// retired, for logging/testing.
func (p *TDiscoveryPublisher) RetireMissing(client mqtt.Client, broker string, expected map[string]bool) []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	prefix := broker + "\x00"
	var retired []string
	for key := range p.known {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		topic := strings.TrimPrefix(key, prefix)
		if expected[topic] {
			continue
		}
		retired = append(retired, topic)
		retireDiscoveryTopic(client, topic)
		delete(p.known, key)
	}
	sort.Strings(retired)
	if len(retired) > 0 {
		if err := p.persist(); err != nil {
			fmt.Printf("[discovery-cleanup] persisting manifest: %v\n", err)
		}
	}
	return retired
}

// RetireOne retires a single topic (empty retained payload) and forgets it from the manifest --
// the single-topic counterpart to RetireMissing's bulk startup sweep, for watchForOrphanedDiscoveryTopics'
// ongoing runtime watch. Forgetting it matters: without this, a later legitimate republish of the
// exact same content Publish already has recorded as "known" would be silently skipped by
// Publish's own unchanged-content fast path, even though the broker itself no longer has
// anything there (it was just retired) -- a real bug found live 2026-08-30. The bare
// retireDiscoveryTopic call this replaces never touched p.known at all, so PROJECT.md 1.2b's
// freshly-fixed cloud-side hassbridge export topics (just added to the expected set) stayed
// permanently empty on the broker even after the fix deployed: the coordinator's own persisted
// manifest still remembered them as already-published from before they'd been wrongly retired,
// so every subsequent Publish call for the same unchanged content was a silent no-op.
func (p *TDiscoveryPublisher) RetireOne(client mqtt.Client, broker, topic string) {
	retireDiscoveryTopic(client, topic)
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.known, manifestKey(broker, topic))
	if err := p.persist(); err != nil {
		fmt.Printf("[discovery-cleanup] persisting manifest: %v\n", err)
	}
}

func (p *TDiscoveryPublisher) persist() error {
	data, err := json.MarshalIndent(TTopicManifest{Topics: p.known}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling topic manifest: %w", err)
	}
	if err := os.WriteFile(p.manifestPath, data, 0o644); err != nil {
		return fmt.Errorf("cannot write %s: %w", p.manifestPath, err)
	}
	return nil
}

// watchForOrphanedDiscoveryTopics is the broker-driven safety net alongside TDiscoveryPublisher's
// local, proactive tracking -- see the component doc comment for why both are needed. Subscribes
// to "<prefix>/+/+/+/config" and, for the discoveryOrphanWatchSettleWindow immediately after
// subscribing (the startup retained-message burst), retires any coordinator-owned topic that's
// either not in expectedTopics at all, or is expected with known content (expectedPayloads --
// hosts-sourced only; a discovery-bridge topic's correct content isn't known until a gateway
// message arrives) that doesn't match the wire content -- this is what closes the local
// manifest's bootstrap gap (it has no memory of content published before this coordinator
// process, or before content-tracking existed at all). Retires through publisher.RetireOne
// (broker identifies which of publisher's per-broker manifest entries to forget), not a bare
// retireDiscoveryTopic call -- see RetireOne's own doc comment for why that distinction is load-
// bearing, not stylistic (a bug, found live 2026-08-30, from when this used the bare call).
//
// Content comparison is deliberately scoped to that startup window only, NOT the rest of the
// process's life: expectedPayloads is a static snapshot built without any live-reported data
// (buildDiscoveryConfigs called with live=nil), but a device's *correct* steady-state content
// legitimately gains fields (via_device, sw_version, ...) once subscribeDeviceInfo starts
// processing retained "hosts/+/device/state" messages after startup -- comparing an ongoing
// publish against the live-data-free baseline would flag that entirely correct, freshly
// live-enriched republish as "wrong" and retire it right back out, fighting the very mechanism
// that's supposed to enrich it. Ongoing content integrity is already TDiscoveryPublisher.Publish's
// job (it compares against its own manifest, which stays accurate since it's the only thing that
// legitimately writes these topics) -- this function doesn't need to duplicate that. Existence
// checking (is this topic expected at all) has no such conflict -- expectedTopics never changes
// during a single process's life, so it stays active for the life of the subscription, as a cheap
// safety net against something external publishing under our own topic space.
//
// declaredCleanNodeIDs (operator-declared via Physical.def's "mqtt_discovery_clean <topic>;",
// coordinator/discovery_cleanup.yaml) is a third, independent check: a legacy node_id that was
// never ours to begin with (e.g. "ems-esp", before it moved to publishing its own discovery under
// ${mqtt_discovery_physical} instead) but is now known-obsolete. Anything seen under a declared
// node_id is retired unconditionally -- no expectedTopics/expectedPayloads lookup, no settle-window
// restriction -- since by definition there's no legitimate content that could ever belong there;
// re-swept for the life of the process (harmless against an already-empty topic) rather than
// tracked as "already done", so it self-heals if a lingering old publisher ever republishes.
func watchForOrphanedDiscoveryTopics(client mqtt.Client, publisher *TDiscoveryPublisher, broker string, expectedTopics map[string]bool, expectedPayloads map[string]string, declaredCleanNodeIDs map[string]bool, prefix string) error {
	var mu sync.Mutex
	seen := map[string]bool{}
	checkingContent := true

	handler := func(_ mqtt.Client, msg mqtt.Message) {
		topic := msg.Topic()
		if len(msg.Payload()) == 0 {
			return
		}

		mu.Lock()
		alreadyHandled := seen[topic]
		mu.Unlock()
		if alreadyHandled {
			return
		}

		nodeID, shaped := topicNodeID(topic, prefix)
		if !shaped {
			return
		}

		stale := declaredCleanNodeIDs[nodeID]
		if !stale && isCoordinatorOwnedTopic(topic, prefix) {
			mu.Lock()
			checkContentNow := checkingContent
			mu.Unlock()

			stale = !expectedTopics[topic]
			if !stale && checkContentNow {
				if want, known := expectedPayloads[topic]; known && string(msg.Payload()) != want {
					stale = true
				}
			}
		}
		if !stale {
			return
		}

		mu.Lock()
		seen[topic] = true
		mu.Unlock()
		publisher.RetireOne(client, broker, topic)
	}

	filter := prefix + "/+/+/+/config"
	token := client.Subscribe(filter, 0, handler)
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		if err := token.Error(); err != nil {
			return fmt.Errorf("subscribing to %s: %w", filter, err)
		}
		return fmt.Errorf("subscribing to %s: timed out", filter)
	}
	time.Sleep(discoveryOrphanWatchSettleWindow)

	mu.Lock()
	checkingContent = false
	mu.Unlock()

	return nil
}
