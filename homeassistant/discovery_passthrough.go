/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: DiscoveryPassthrough
 *
 * Parses Physical.def's bare "discovery_passthrough "<topic-prefix>" from "<source-prefix>";"
 * top-level statement (deliberately not nested inside any "integration ... with: ... end;" block,
 * mirroring mqtt_discovery_cleanup.go's own "mqtt_discovery_clean <topic>;" -- this is the same
 * "generic, repeatable mechanism for every future legacy-integration migration" that file's own
 * header comment already anticipated) and writes <outputRoot>/coordinator/discovery_passthrough.yaml.
 *
 * PROJECT.md item 7's own design: during a gradual Zigbee2MQTT-to-conceptual migration, Zigbee2MQTT
 * itself gets reconfigured to publish its OWN discovery under ${mqtt_discovery_physical} (the same
 * "homeassistant.physical" prefix EMS-ESP-style "discovery" gateways already use, discoverybridge.go)
 * instead of the real "homeassistant" prefix HA subscribes to -- so HA never sees a not-yet-migrated
 * device's raw discovery directly any more. Once migrated (declared via "integration discovery
 * with: device <id> with: ... end; end;" and an "entity ... from <gateway-id>.<leaf>;" link), a
 * device's entities already flow through the existing discoverybridge.go relay, repositioned under
 * our own conceptual entity ids. Everything NOT yet declared that way still needs HA to see it
 * exactly as before, so operators aren't blocked mid-migration -- discovery_passthrough declares
 * which raw topic prefixes (e.g. Zigbee2MQTT's own "zigbee2mqtt/" state/command topics) the
 * coordinator should relay byte-for-byte from the physical prefix onto the real one, for any device
 * a declared gateway hasn't already claimed (house_event_bus_coordinator/discoverybridge.go's own
 * matchingGateway check decides that, live, per message -- this file only carries the declared
 * prefixes/source through to the coordinator).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 14.09.2026
 *
 */

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// TDiscoveryPassthroughRule is one declared "discovery_passthrough "<topic-prefix>" from
// "<source-prefix>";" statement.
type TDiscoveryPassthroughRule struct {
	TopicPrefix  string
	SourcePrefix string
}

var discoveryPassthroughPattern = regexp.MustCompile(`^discovery_passthrough\s+"([^"]*)"\s+from\s+"([^"]*)"\s*;$`)

// collectDiscoveryPassthroughRules scans physicalContent for every "discovery_passthrough
// "<topic-prefix>" from "<source-prefix>";" statement, deduplicated (by the full rule) and sorted
// -- mirrors collectMQTTDiscoveryCleanTopics exactly: an independent line-by-line pass, safe
// regardless of surrounding "integration ... with: ... end;" block structure, since
// scanGroupClauseBlocks only ever recognises its own header lines.
//
// Either quoted value may itself be a "${name}" Settings.def reference (e.g. `from
// "${mqtt_discovery_physical}"` rather than the literal resolved string) -- resolveDefinitionReference
// (defined.go) already handles both a bare literal (returned unchanged) and a reference (resolved
// against Settings.def) uniformly, so every captured value is passed through it unconditionally.
// Real gap found live 2026-09-14: Vienna's own declaration used the "${mqtt_discovery_physical}"
// form (the natural, DRY way to write it, matching every other Settings.def-backed field in this
// DSL), which this function originally took as a literal string instead -- silently never matching
// discoveryFile.PhysicalPrefix, so the coordinator would relay nothing at all.
func collectDiscoveryPassthroughRules(physicalContent, definitionDir string) []TDiscoveryPassthroughRule {
	settingsContent := readCombinedSettingsContent(definitionDir)
	vars := parseDefinitionAssignments(settingsContent)

	seen := map[TDiscoveryPassthroughRule]bool{}
	var rules []TDiscoveryPassthroughRule
	for _, rawLine := range splitLines(physicalContent) {
		line := strings.TrimSpace(rawLine)
		matches := discoveryPassthroughPattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		rule := TDiscoveryPassthroughRule{
			TopicPrefix:  resolveDefinitionReference(matches[1], vars),
			SourcePrefix: resolveDefinitionReference(matches[2], vars),
		}
		if !seen[rule] {
			seen[rule] = true
			rules = append(rules, rule)
		}
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].SourcePrefix != rules[j].SourcePrefix {
			return rules[i].SourcePrefix < rules[j].SourcePrefix
		}
		return rules[i].TopicPrefix < rules[j].TopicPrefix
	})
	return rules
}

// generateDiscoveryPassthroughFile writes <outputRoot>/coordinator/discovery_passthrough.yaml, or
// removes it when rules is empty -- an ABSENT file means "coordinator, relay nothing," which must
// be authoritative, not just "generator didn't bother writing this run." Real bug found live
// 2026-09-19: the previous behaviour ("skip writing, no file written") only matches
// coordinator/discovery.yaml's/discovery_cleanup.yaml's own "absent = nothing declared" convention
// for a house that NEVER had passthrough rules -- for a house that HAD them and then removed the
// declaration (e.g. Vienna's own item-4 migration finishing up), the stale file from an earlier
// run lingered with its OLD rules forever, meaning the coordinator would keep relaying
// zigbee2mqtt/ traffic it was explicitly told to stop relaying. Mirrors
// generateDiscoverySuggestions' own identical fix for suggestions/discovery.txt (an empty report
// removes any stale file rather than leaving outdated content in place). Warns (doesn't drop,
// doesn't error) for any rule whose declared source prefix doesn't match resolvedPhysicalPrefix
// (${mqtt_discovery_physical}) -- the coordinator today only ever subscribes to that one physical
// prefix (discoverybridge.go), so such a rule can never actually match live traffic; still written
// through so a future second physical prefix doesn't need this file's shape to change, just the
// coordinator's own subscription scope.
func generateDiscoveryPassthroughFile(outputRoot string, rules []TDiscoveryPassthroughRule, resolvedPhysicalPrefix string) error {
	passthroughPath := filepath.Join(outputRoot, "coordinator", "discovery_passthrough.yaml")
	if len(rules) == 0 {
		if err := os.Remove(passthroughPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	for _, rule := range rules {
		if rule.SourcePrefix != resolvedPhysicalPrefix {
			fmt.Printf("[physical] discovery_passthrough %q from %q: declared source prefix doesn't match ${mqtt_discovery_physical} (%q) -- the coordinator only relays from that prefix today, this rule will never match live traffic\n", rule.TopicPrefix, rule.SourcePrefix, resolvedPhysicalPrefix)
		}
	}

	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("passthrough:\n")
	for _, rule := range rules {
		sb.WriteString("  - topic_prefix: \"" + rule.TopicPrefix + "\"\n")
		sb.WriteString("    source_prefix: \"" + rule.SourcePrefix + "\"\n")
	}

	return writeYAMLFile(passthroughPath, sb.String())
}
