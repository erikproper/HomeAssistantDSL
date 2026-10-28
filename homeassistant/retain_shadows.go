/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: RetainShadows
 *
 * PROJECT.md 20 (2026-10-28): which state topics of discovered (and, later, raw) devices can be
 * trusted as retained, and which get a shadow -- "<topic>/retain", the coordinator's retained copy
 * of the latest message, which the state entities then read (Architecture.md §6.16). A device's
 * retain status can't be observed, so the worst is assumed unless the defs say otherwise:
 *
 *   - a device's own "is retained;" / "is not retained;" line always wins;
 *   - else the installation's "retained topics "<filter>", ...;" (MQTT filter syntax) decides;
 *   - else: not retained, so a shadow.
 *
 * Each device line therefore only does something against the default around it: "is retained;"
 * where no filter covers the device's topics, "is not retained;" (the way back out of a filter
 * that claims more than is true) where one does.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 28.10.2026
 *
 */

package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// TRetainDeclaration is a device's own retain line, if any.
type TRetainDeclaration int

const (
	retainUndeclared  TRetainDeclaration = iota // no line: the installation's filters decide
	retainDeclared                              // "is retained;"
	retainNotDeclared                           // "is not retained;"
)

// shadowTopicSuffix is appended to a state topic to form its shadow (decided, Erik, 2026-10-27).
const shadowTopicSuffix = "/retain"

// shadowTopic is topic's shadow: the coordinator's retained copy of its latest message.
func shadowTopic(topic string) string {
	return topic + shadowTopicSuffix
}

// topicIsRetained reports whether topic can be trusted as retained -- read directly, no shadow --
// given its device's own declaration and the installation's retained-topic filters.
func topicIsRetained(topic string, device TRetainDeclaration, filters []string) bool {
	switch device {
	case retainDeclared:
		return true
	case retainNotDeclared:
		return false
	}
	return topicMatchesAnyFilter(topic, filters)
}

// topicMatchesAnyFilter reports whether topic matches one of filters.
func topicMatchesAnyFilter(topic string, filters []string) bool {
	for _, filter := range filters {
		if mqttFilterMatches(filter, topic) {
			return true
		}
	}
	return false
}

// mqttFilterMatches reports whether topic matches filter, by MQTT's own rules: "+" matches exactly
// one level, a final "#" the remaining levels (including none: "a/#" matches "a").
func mqttFilterMatches(filter, topic string) bool {
	filterLevels := strings.Split(filter, "/")
	topicLevels := strings.Split(topic, "/")
	for i, level := range filterLevels {
		if level == "#" {
			return true
		}
		if i >= len(topicLevels) {
			return false
		}
		if level != "+" && level != topicLevels[i] {
			return false
		}
	}
	return len(filterLevels) == len(topicLevels)
}

// mqttFilterProblem returns what is wrong with filter as an MQTT topic filter, or "" when nothing:
// "#" only as the whole last level, "+" only as a whole level.
func mqttFilterProblem(filter string) string {
	if filter == "" {
		return "empty filter"
	}
	levels := strings.Split(filter, "/")
	for i, level := range levels {
		if strings.Contains(level, "#") && (level != "#" || i != len(levels)-1) {
			return "\"#\" may only be the whole last level"
		}
		if strings.Contains(level, "+") && level != "+" {
			return "\"+\" may only be a whole level"
		}
	}
	return ""
}

var retainedTopicsPattern = regexp.MustCompile(`^retained\s+topics\s+(.+?)\s*;\s*$`)
var quotedFilterPattern = regexp.MustCompile(`^"([^"]*)"$`)

// parseRetainedTopicsLine parses a 'retained topics "<filter>", ...;' line into its filters,
// reporting a malformed list or filter; ok is false when line isn't such a line at all.
func parseRetainedTopicsLine(line string) (filters, warnings []string, ok bool) {
	matches := retainedTopicsPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil, nil, false
	}
	for _, item := range strings.Split(matches[1], ",") {
		quoted := quotedFilterPattern.FindStringSubmatch(strings.TrimSpace(item))
		if quoted == nil {
			warnings = append(warnings, fmt.Sprintf("Physical.def: \"retained topics\" takes quoted MQTT filters, separated by commas; ignoring %q", strings.TrimSpace(item)))
			continue
		}
		if problem := mqttFilterProblem(quoted[1]); problem != "" {
			warnings = append(warnings, fmt.Sprintf("Physical.def: \"retained topics\" filter %q: %s; ignoring it", quoted[1], problem))
			continue
		}
		filters = append(filters, quoted[1])
	}
	return filters, warnings, true
}

// parseDiscoveryRetainedTopics returns the filters of every 'retained topics ...;' line directly in
// an "integration discovery" block's body (never inside a device: parseDiscoveryIntegrationBody
// reports one there).
func parseDiscoveryRetainedTopics(bodyLines []string) (filters, warnings []string) {
	depth := 0
	for _, line := range bodyLines {
		switch {
		case discoveryDevicePattern.MatchString(line):
			depth++
		case line == "end;":
			depth--
		case depth == 0:
			if lineFilters, lineWarnings, ok := parseRetainedTopicsLine(line); ok {
				filters = append(filters, lineFilters...)
				warnings = append(warnings, lineWarnings...)
			}
		}
	}
	return filters, warnings
}

// coordinatorAvailabilityTopic is the coordinator's own liveness, retained ("online"/"offline",
// its Last Will -- house_event_bus_coordinator/retain_shadows.go).
const coordinatorAvailabilityTopic = "coordinator/availability"

// isStateTopicKey reports whether a YAML mqtt entity's key names a topic it reads state from:
// "state_topic" and the per-feature ones ("mode_state_topic", "current_temperature_topic",
// "position_topic", "json_attributes_topic", ...), never a command topic -- including cover's
// "set_position_topic", the one command key without "command" in its name -- nor availability.
func isStateTopicKey(key string) bool {
	return strings.HasSuffix(key, "topic") && !strings.Contains(key, "command") &&
		key != "set_position_topic" && key != "availability_topic"
}

// applyRetainShadows points body's state topics at their shadows, except those trusted as retained
// (topicIsRetained), and -- when it shadowed any -- adds the coordinator's availability, since a
// stopped coordinator would freeze the shadows' values. An event entity is left alone: it reads
// its original topic, as a retained event would fire again after every reload. Returns the state
// topics it saw and the ones it shadowed (the originals).
func applyRetainShadows(body map[string]interface{}, domain string, device TRetainDeclaration, filters []string) (seen, shadowed []string) {
	keys := make([]string, 0, len(body))
	for key := range body {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		topic, isText := body[key].(string)
		if !isText || topic == "" || !isStateTopicKey(key) {
			continue
		}
		seen = append(seen, topic)
		if domain == "event" || topicIsRetained(topic, device, filters) {
			continue
		}
		body[key] = shadowTopic(topic)
		shadowed = append(shadowed, topic)
	}
	if len(shadowed) > 0 {
		addCoordinatorAvailability(body)
	}
	return seen, shadowed
}

// addCoordinatorAvailability ANDs the coordinator's availability into body's: appended to its
// "availability" list (turning a single "availability_topic" into the list form first), with
// availability_mode "all".
func addCoordinatorAvailability(body map[string]interface{}) {
	var entries []interface{}
	switch list := body["availability"].(type) {
	case []interface{}:
		entries = list
	case []map[string]interface{}:
		for _, entry := range list {
			entries = append(entries, entry)
		}
	}
	if topic, ok := body["availability_topic"].(string); ok && topic != "" {
		entry := map[string]interface{}{"topic": topic}
		for _, key := range []string{"payload_available", "payload_not_available"} {
			if value, has := body[key]; has {
				entry[key] = value
				delete(body, key)
			}
		}
		if template, has := body["availability_template"]; has {
			entry["value_template"] = template
			delete(body, "availability_template")
		}
		delete(body, "availability_topic")
		entries = append(entries, entry)
	}
	entries = append(entries, map[string]interface{}{
		"topic":                 coordinatorAvailabilityTopic,
		"payload_available":     "online",
		"payload_not_available": "offline",
	})
	body["availability"] = entries
	body["availability_mode"] = "all"
}

// TRetainShadowsFile is coordinator/retain_shadows.yaml: the state topics the coordinator shadows.
type TRetainShadowsFile struct {
	Topics []string `yaml:"topics"`
}

// retainDeclarationEffect reports whether device's own retain line changes the decision for any of
// topics, compared with the filters alone -- a line that doesn't is reported.
func retainDeclarationEffect(topics []string, device TRetainDeclaration, filters []string) bool {
	for _, topic := range topics {
		if topicIsRetained(topic, device, filters) != topicIsRetained(topic, retainUndeclared, filters) {
			return true
		}
	}
	return false
}

// unmatchedRetainFilters returns the filters that match none of topics -- most likely a typo.
func unmatchedRetainFilters(filters, topics []string) []string {
	var unmatched []string
	for _, filter := range filters {
		matched := false
		for _, topic := range topics {
			if mqttFilterMatches(filter, topic) {
				matched = true
				break
			}
		}
		if !matched {
			unmatched = append(unmatched, filter)
		}
	}
	return unmatched
}
