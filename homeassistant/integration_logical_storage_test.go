package main

import (
	"strings"
	"testing"
)

func TestResolveTransitiveDependenciesLinearChain(t *testing.T) {
	logicalByID := map[string]TLogicalDevice{
		"a": {DeviceID: "a", DependsOn: []string{"b"}},
		"b": {DeviceID: "b", DependsOn: []string{"c"}},
		"c": {DeviceID: "c"},
	}
	chain, cycle := resolveTransitiveDependencies("a", logicalByID)
	if cycle != nil {
		t.Fatalf("unexpected cycle: %v", cycle)
	}
	seen := map[string]bool{}
	for _, id := range chain {
		seen[id] = true
	}
	if !seen["b"] || !seen["c"] {
		t.Errorf("chain = %v, want it to contain both b and c (A depends on B depends on C, transitively)", chain)
	}
}

// TestResolveTransitiveDependenciesDetectsCycle is the user's own explicitly-flagged requirement:
// device A depends on B depends on C depends back on A must be caught, not hang forever.
func TestResolveTransitiveDependenciesDetectsCycle(t *testing.T) {
	logicalByID := map[string]TLogicalDevice{
		"a": {DeviceID: "a", DependsOn: []string{"b"}},
		"b": {DeviceID: "b", DependsOn: []string{"c"}},
		"c": {DeviceID: "c", DependsOn: []string{"a"}},
	}
	_, cycle := resolveTransitiveDependencies("a", logicalByID)
	if cycle == nil {
		t.Fatalf("expected a cycle to be detected for a->b->c->a")
	}
	if cycle[0] != "a" || cycle[len(cycle)-1] != "a" {
		t.Errorf("cycle = %v, want it to start and end at the repeated node \"a\"", cycle)
	}
}

// TestResolveTransitiveDependenciesSelfCycle covers the degenerate single-node cycle: a device
// depending directly on itself.
func TestResolveTransitiveDependenciesSelfCycle(t *testing.T) {
	logicalByID := map[string]TLogicalDevice{
		"a": {DeviceID: "a", DependsOn: []string{"a"}},
	}
	_, cycle := resolveTransitiveDependencies("a", logicalByID)
	if cycle == nil {
		t.Fatalf("expected a cycle to be detected for a device depending on itself")
	}
}

func TestResolveTransitiveDependenciesNoLogicalDeclaration(t *testing.T) {
	chain, cycle := resolveTransitiveDependencies("host.netatmo", map[string]TLogicalDevice{})
	if chain != nil || cycle != nil {
		t.Errorf("chain=%v cycle=%v, want both nil for a device with no logical declaration at all", chain, cycle)
	}
}

// TestResolveDependencyAvailabilityTopicsHostsKind is the user's own concrete example:
// node.vienna_livingroom depends on host.netatmo, a "ping"-type hosts device -- must resolve to
// that device's own NodeTopic.
func TestResolveDependencyAvailabilityTopicsHostsKind(t *testing.T) {
	logicalByID := map[string]TLogicalDevice{
		"node.vienna_livingroom": {DeviceID: "node.vienna_livingroom", DependsOn: []string{"host.netatmo"}},
	}
	hostDevicesByID := map[string]THostDevice{
		"host.netatmo": {DeviceID: "host.netatmo", HostName: "netatmo", IntegrationType: "ping"},
	}
	topics, warnings := resolveDependencyAvailabilityTopics("node.vienna_livingroom", logicalByID, hostDevicesByID)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	want := "hosts/netatmo/node/state"
	if len(topics) != 1 || topics[0] != want {
		t.Errorf("topics = %v, want [%q]", topics, want)
	}
}

func TestResolveDependencyAvailabilityTopicsUnsupportedKindWarns(t *testing.T) {
	logicalByID := map[string]TLogicalDevice{
		"node.x": {DeviceID: "node.x", DependsOn: []string{"hass.y"}},
	}
	topics, warnings := resolveDependencyAvailabilityTopics("node.x", logicalByID, map[string]THostDevice{})
	if len(topics) != 0 {
		t.Errorf("topics = %v, want none (hass.y isn't a hosts device)", topics)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "hass.y") {
		t.Errorf("warnings = %v, want one naming hass.y", warnings)
	}
}

func TestResolveDependencyAvailabilityTopicsCycleWarnsAndIgnoresEntirely(t *testing.T) {
	logicalByID := map[string]TLogicalDevice{
		"a": {DeviceID: "a", DependsOn: []string{"b"}},
		"b": {DeviceID: "b", DependsOn: []string{"a"}},
	}
	hostDevicesByID := map[string]THostDevice{
		"a": {DeviceID: "a", HostName: "a", IntegrationType: "ping"},
		"b": {DeviceID: "b", HostName: "b", IntegrationType: "ping"},
	}
	topics, warnings := resolveDependencyAvailabilityTopics("a", logicalByID, hostDevicesByID)
	if len(topics) != 0 {
		t.Errorf("topics = %v, want none -- a cycle must never partially apply", topics)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "cycles") {
		t.Errorf("warnings = %v, want one mentioning the cycle", warnings)
	}
}

func TestResolveDependencyAvailabilityTopicsNoDependencyDeclaration(t *testing.T) {
	topics, warnings := resolveDependencyAvailabilityTopics("node.unrelated", map[string]TLogicalDevice{}, map[string]THostDevice{})
	if topics != nil || warnings != nil {
		t.Errorf("topics=%v warnings=%v, want both nil for a device with no Logical.def declaration at all", topics, warnings)
	}
}

func TestApplyResolvedDependencyTopics(t *testing.T) {
	admin := newAdministrationState()
	admin.DependsOnAvailabilityTopics["node.vienna_livingroom"] = []string{"hosts/netatmo/node/state"}

	devices := []TImportedDevice{
		{DeviceID: "node.vienna_livingroom"},
		{DeviceID: "node.other"},
	}
	applyResolvedDependencyTopics(devices, admin)

	if len(devices[0].DependsOnAvailabilityTopics) != 1 || devices[0].DependsOnAvailabilityTopics[0] != "hosts/netatmo/node/state" {
		t.Errorf("devices[0].DependsOnAvailabilityTopics = %v, want [hosts/netatmo/node/state]", devices[0].DependsOnAvailabilityTopics)
	}
	if len(devices[1].DependsOnAvailabilityTopics) != 0 {
		t.Errorf("devices[1].DependsOnAvailabilityTopics = %v, want none (no matching admin entry)", devices[1].DependsOnAvailabilityTopics)
	}
}
