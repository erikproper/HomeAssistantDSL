/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: RetainShadows
 *
 * PROJECT.md 20 (2026-10-28): the coordinator's retained copies of state topics that can't be
 * trusted as retained (Architecture.md §6.16). Home Assistant's MQTT reload re-creates every YAML
 * mqtt entity, and only a retained topic gives it a value back; Zigbee2MQTT, for one, retains
 * nothing. So for every topic in coordinator/retain_shadows.yaml -- the generator decides which
 * (homeassistant/retain_shadows.go), never the coordinator -- the latest message is posted again,
 * unchanged and retained, on "<topic>/retain", the shadow the state entities read. An empty
 * message (a cleared retained topic) clears the shadow too.
 *
 * The coordinator remembers which shadows it owns in a retained manifest on the broker
 * (coordinator/retain_shadows/state), not in a file a deploy's "rsync --delete" could wipe. At
 * every start it clears the retained message of every shadow in that manifest that is no longer
 * listed -- a device switched to "is retained;", or one removed -- then publishes the new
 * manifest. A new shadow fills at once from the original's retained message, if there is one;
 * otherwise at the device's next report.
 *
 * coordinator/availability says whether the coordinator runs ("online" on every connect,
 * "offline" as its Last Will): with the coordinator on the data path, a stopped one would freeze
 * the shadows' values, so every entity reading a shadow also depends on it.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 28.10.2026
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

const (
	// retainShadowSuffix is appended to a state topic to form its shadow (Erik, 2026-10-27).
	retainShadowSuffix = "/retain"
	// retainShadowsManifestTopic holds, retained, the topics whose shadows this coordinator owns.
	retainShadowsManifestTopic = "coordinator/retain_shadows/state"
	// coordinatorAvailabilityTopic is the coordinator's own liveness, retained.
	coordinatorAvailabilityTopic = "coordinator/availability"
	coordinatorOnline            = "online"
	coordinatorOffline           = "offline"
	// retainShadowsManifestWait is how long startup waits for the previous manifest.
	retainShadowsManifestWait = 3 * time.Second
)

// TRetainShadowsFile is coordinator/retain_shadows.yaml: the state topics to shadow.
type TRetainShadowsFile struct {
	Topics []string `yaml:"topics"`
}

// loadRetainShadowsFile reads path; a missing file means no shadows.
func loadRetainShadowsFile(path string) (TRetainShadowsFile, error) {
	var file TRetainShadowsFile
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return file, nil
	}
	if err != nil {
		return file, err
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return file, fmt.Errorf("%s: %w", path, err)
	}
	return file, nil
}

// retainShadowTopic is topic's shadow.
func retainShadowTopic(topic string) string {
	return topic + retainShadowSuffix
}

// shadowedTopics returns topics, deduplicated and sorted, without any that is itself a shadow (a
// shadow of a shadow would only loop).
func shadowedTopics(topics []string) []string {
	set := map[string]bool{}
	for _, topic := range topics {
		if topic != "" && !strings.HasSuffix(topic, retainShadowSuffix) {
			set[topic] = true
		}
	}
	out := make([]string, 0, len(set))
	for topic := range set {
		out = append(out, topic)
	}
	sort.Strings(out)
	return out
}

// staleRetainShadows returns the topics in manifest no longer in current: their shadows go.
func staleRetainShadows(manifest, current []string) []string {
	keep := map[string]bool{}
	for _, topic := range current {
		keep[topic] = true
	}
	var stale []string
	for _, topic := range manifest {
		if !keep[topic] {
			stale = append(stale, topic)
		}
	}
	sort.Strings(stale)
	return stale
}

// startRetainShadows subscribes client to every listed topic, posting each message on its shadow,
// and -- once the previous manifest has arrived (or right away when there is none) -- clears the
// shadows no longer listed and publishes the new manifest.
func startRetainShadows(client mqtt.Client, file TRetainShadowsFile) error {
	topics := shadowedTopics(file.Topics)
	listed := map[string]bool{}
	for _, topic := range topics {
		listed[topic] = true
	}

	manifestData, err := json.Marshal(topics)
	if err != nil {
		return err
	}
	// The manifest is retained, so the broker delivers the previous one, if any, right after the
	// subscribe -- asynchronously, so wait for it a moment (once, at startup); a broker without
	// one delivers nothing, and then every shadow is new.
	var once sync.Once
	arrived := make(chan []string, 1)
	token := subscribe(client, retainShadowsManifestTopic, 1, func(_ mqtt.Client, msg mqtt.Message) {
		if msg.Topic() != retainShadowsManifestTopic || !msg.Retained() {
			return
		}
		var previous []string
		if err := json.Unmarshal(msg.Payload(), &previous); err != nil {
			fmt.Printf("[retain-shadows] unreadable manifest, clearing nothing: %v\n", err)
			previous = nil
		}
		once.Do(func() { arrived <- previous })
	})
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("subscribing to %s: %w", retainShadowsManifestTopic, token.Error())
	}
	var previous []string
	select {
	case previous = <-arrived:
	case <-time.After(retainShadowsManifestWait):
		once.Do(func() {}) // a manifest arriving later still changes nothing
	}
	for _, topic := range staleRetainShadows(previous, topics) {
		fmt.Printf("[retain-shadows] clearing %s (no longer listed)\n", retainShadowTopic(topic))
		client.Publish(retainShadowTopic(topic), 1, true, []byte{})
	}
	client.Publish(retainShadowsManifestTopic, 1, true, manifestData)

	mirror := func(_ mqtt.Client, msg mqtt.Message) {
		if !listed[msg.Topic()] {
			return
		}
		client.Publish(retainShadowTopic(msg.Topic()), 0, true, msg.Payload())
	}
	for _, topic := range topics {
		if token := subscribe(client, topic, 0, mirror); token.Wait() && token.Error() != nil {
			return fmt.Errorf("subscribing to %s: %w", topic, token.Error())
		}
	}
	fmt.Printf("[retain-shadows] shadowing %d topic(s)\n", len(topics))
	return nil
}
