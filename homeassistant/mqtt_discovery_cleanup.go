/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: MQTTDiscoveryCleanup
 *
 * Parses Physical.def's bare "mqtt_discovery_clean <topic>;" top-level statement (deliberately
 * not nested inside any "integration ... with: ... end;" block -- the legacy node it targets may
 * have no integration block at all, e.g. a bare decommissioned publisher) and writes
 * <outputRoot>/coordinator/discovery_cleanup.yaml, the coordinator's list of legacy discovery
 * node_ids to sweep clean on every restart (house_event_bus_coordinator/discoverycleanup.go).
 *
 * Lets an operator declare a legacy integration's old discovery node_id (e.g. "ems-esp", once
 * reconfigured to publish under ${mqtt_discovery_physical} instead of the real "homeassistant"
 * prefix) as known-obsolete, so the coordinator can retire every topic still lingering under it --
 * any component, any object_id -- without hand-cleaning each leftover entity in Home Assistant.
 * The same situation recurs for every future legacy-integration migration (Zigbee2MQTT, Z-Wave,
 * ...), so this is a general, repeatable mechanism, not a one-off fix for EMS-ESP specifically.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 23.08.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var mqttDiscoveryCleanPattern = regexp.MustCompile(`^mqtt_discovery_clean\s+(\S+)\s*;$`)

// collectMQTTDiscoveryCleanTopics scans physicalContent for every "mqtt_discovery_clean <topic>;"
// statement, deduplicated and sorted. Safe to call regardless of block structure:
// scanGroupClauseBlocks (defined.go) already silently skips any line that isn't a recognised
// "integration ... with:" header, so this independent line-by-line pass -- mirroring Settings.def's
// parseDefinitionAssignments -- can't collide with parseIntegrationBlocks.
func collectMQTTDiscoveryCleanTopics(physicalContent string) []string {
	seen := map[string]bool{}
	var topics []string
	for _, rawLine := range splitLines(physicalContent) {
		line := strings.TrimSpace(rawLine)
		matches := mqttDiscoveryCleanPattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		topic := matches[1]
		if !seen[topic] {
			seen[topic] = true
			topics = append(topics, topic)
		}
	}
	sort.Strings(topics)
	return topics
}

// generateDiscoveryCleanupFile writes <outputRoot>/coordinator/discovery_cleanup.yaml -- skipped
// entirely (no file written) when topics is empty, matching coordinator/discovery.yaml's "absent
// file = nothing declared" convention most houses fall into. Rejects (warns, drops, keeps going)
// any declared topic equal to "coordinator" -- the coordinator's own reserved node_id literal
// (house_event_bus_coordinator/discovery.go's discoveryTopic) -- since sweeping it would retire
// every entity this coordinator manages; the one place a typo here could do real damage.
func generateDiscoveryCleanupFile(outputRoot string, topics []string) error {
	var clean []string
	for _, topic := range topics {
		if topic == "coordinator" {
			fmt.Printf("[physical] mqtt_discovery_clean %q refused: \"coordinator\" is the coordinator's own reserved node_id -- sweeping it would retire every entity this coordinator manages\n", topic)
			continue
		}
		clean = append(clean, topic)
	}
	if len(clean) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("clean_topics:\n")
	for _, topic := range clean {
		sb.WriteString("  - " + topic + "\n")
	}

	dir := filepath.Join(outputRoot, "coordinator")
	return writeYAMLFile(filepath.Join(dir, "discovery_cleanup.yaml"), sb.String())
}
