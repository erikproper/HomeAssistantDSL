package main

import (
	"reflect"
	"testing"
)

// noHassbridgeOrImport is the neverIsHassbridgeOrImport stub every test below uses unless it's
// specifically testing that gate -- every device in these tests is "local"-kind.
func noHassbridgeOrImport(string) bool { return false }

// TestApplyLogicalEnablerDependencySugarMatchesHandWrittenLonghand is the core correctness test:
// the sugared "dependency on"/"enabled by" form for appliance.tv_living_room must produce the
// exact same DerivedCondition tree as Junglinster's own pre-migration hand-written longhand,
// "condition (physical appliance.tv_living_room and utility.sonytv) or jinja \"$1 == 'off'\" {
// enabler };" -- parsed independently here as the ground truth to compare against, so a change to
// either the sugar or the condition-expr grammar itself would be caught by drift between the two.
// The ground truth's own "physical appliance.tv_living_room" and the sugar's own "core is
// available" are DELIBERATELY different text (2026-09-28's "enabled by" generalization, which
// builds the base condition from the named LHS capability rather than the whole device) -- they
// still resolve to the exact same live entity for this specific device, because
// appliance.tv_living_room's own Physical.def "node" capability is ITSELF defined as "derived
// binary_sensor.node with condition media_player.core is available;", confirmed by this test
// asserting the two trees are equal despite the differing source text.
func TestApplyLogicalEnablerDependencySugarMatchesHandWrittenLonghand(t *testing.T) {
	devices := map[string]TLogicalDevice{
		"appliance.tv_living_room": {
			DeviceID:  "appliance.tv_living_room",
			DependsOn: []string{"utility.sonytv"},
			EnabledBy: &TLogicalEnabledByFact{
				OwnDomain:      "media_player",
				OwnLabel:       "core",
				SourceDomain:   "switch",
				SourceLabel:    "core",
				SourceDeviceID: "switch.house_living_room_tv",
			},
			Capabilities: map[string]TLogicalCapability{},
		},
	}

	warnings := applyLogicalEnablerDependencySugar(devices, noHassbridgeOrImport, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	d := devices["appliance.tv_living_room"]
	node, hasNode := d.Capabilities["node"]
	if !hasNode || !node.IsDerivedCondition || node.Domain != "binary_sensor" {
		t.Fatalf("Capabilities[node] = %+v, want a derived-condition binary_sensor capability", node)
	}
	enabler, hasEnabler := d.Capabilities["enabler"]
	if !hasEnabler || enabler.Domain != "switch" || enabler.AbsorbedFromDeviceID != "switch.house_living_room_tv" || enabler.AbsorbedFromCapability != "core" {
		t.Fatalf("Capabilities[enabler] = %+v, want an absorbed switch.enabler from switch.house_living_room_tv!core", enabler)
	}

	wantTree, exprWarnings := parseConditionExpr(`(media_player.core is available and utility.sonytv) or jinja "$[switch.enabler] == 'off'"`, nil, "test")
	if len(exprWarnings) != 0 {
		t.Fatalf("ground-truth longhand itself failed to parse: %v", exprWarnings)
	}
	if !reflect.DeepEqual(node.DerivedCondition, wantTree) {
		t.Errorf("DerivedCondition = %+v, want the hand-written longhand's own tree %+v", node.DerivedCondition, wantTree)
	}
}

// TestApplyLogicalEnablerDependencySugarCarriesNodeDelayOff is a regression test for Vienna's own
// appliance.tv (2026-09-28): a device needing a "delay_off <time>;" debounce on its own synthesized
// "node" capability (the OLD hand-written longhand's own "derived binary_sensor.node with: ...
// delay_off 00:01:00; end;") must not silently lose it when migrated to the "enabled by" sugar.
func TestApplyLogicalEnablerDependencySugarCarriesNodeDelayOff(t *testing.T) {
	devices := map[string]TLogicalDevice{
		"appliance.tv": {
			DeviceID:     "appliance.tv",
			DependsOn:    []string{"node.tv"},
			NodeDelayOff: "00:01:00",
			EnabledBy: &TLogicalEnabledByFact{
				OwnDomain:      "media_player",
				OwnLabel:       "core",
				SourceDomain:   "switch",
				SourceLabel:    "core",
				SourceDeviceID: "appliance.living_room_tv",
			},
			Capabilities: map[string]TLogicalCapability{},
		},
	}

	warnings := applyLogicalEnablerDependencySugar(devices, noHassbridgeOrImport, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	node := devices["appliance.tv"].Capabilities["node"]
	if node.DelayOff != "00:01:00" {
		t.Errorf("node.DelayOff = %q, want the device's own NodeDelayOff (%q) carried through", node.DelayOff, "00:01:00")
	}
}

// TestApplyLogicalEnablerDependencySugarNoOpWithoutEitherKeyword confirms an ordinary device
// (neither keyword declared) is left completely untouched.
func TestApplyLogicalEnablerDependencySugarNoOpWithoutEitherKeyword(t *testing.T) {
	original := TLogicalDevice{DeviceID: "appliance.sonos", Capabilities: map[string]TLogicalCapability{"node": {Domain: "binary_sensor"}}}
	devices := map[string]TLogicalDevice{"appliance.sonos": original}

	warnings := applyLogicalEnablerDependencySugar(devices, noHassbridgeOrImport, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if !reflect.DeepEqual(devices["appliance.sonos"], original) {
		t.Errorf("device = %+v, want completely unchanged", devices["appliance.sonos"])
	}
}

// TestApplyLogicalEnablerDependencySugarSkipsExplicitNodeCapability confirms a device mid-migration
// (still hand-declaring its own "node" capability) is left untouched even though it ALSO declares
// "dependency on"/"enabler" -- an explicit declaration always wins, never silently overridden.
func TestApplyLogicalEnablerDependencySugarSkipsExplicitNodeCapability(t *testing.T) {
	explicitNode := TLogicalCapability{Domain: "binary_sensor", IsDerivedCondition: true}
	devices := map[string]TLogicalDevice{
		"appliance.washing_machine": {
			DeviceID:  "appliance.washing_machine",
			DependsOn: []string{"node.candy"},
			EnabledBy: &TLogicalEnabledByFact{
				OwnDomain:      "switch",
				OwnLabel:       "core",
				SourceDomain:   "switch",
				SourceLabel:    "core",
				SourceDeviceID: "switch.washing_machine",
			},
			Capabilities: map[string]TLogicalCapability{"node": explicitNode},
		},
	}

	warnings := applyLogicalEnablerDependencySugar(devices, noHassbridgeOrImport, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if got := devices["appliance.washing_machine"].Capabilities["node"]; !reflect.DeepEqual(got, explicitNode) {
		t.Errorf("Capabilities[node] = %+v, want the untouched explicit declaration %+v", got, explicitNode)
	}
}

// TestApplyLogicalEnablerDependencySugarSkipsHassbridgeOrImportDependents confirms a
// hassbridge/import-kind device's own "dependency on" (no enabler) does NOT get a synthesized
// condition-tree "node" -- its own real MQTT availability is wired via depends_on_availability
// elsewhere, untouched by this sugar.
func TestApplyLogicalEnablerDependencySugarSkipsHassbridgeOrImportDependents(t *testing.T) {
	devices := map[string]TLogicalDevice{
		"utility.fritz_box": {
			DeviceID:     "utility.fritz_box",
			DependsOn:    []string{"node.fritz_box"},
			Capabilities: map[string]TLogicalCapability{},
		},
	}
	isHassbridgeOrImport := func(id string) bool { return id == "utility.fritz_box" }

	warnings := applyLogicalEnablerDependencySugar(devices, isHassbridgeOrImport, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if _, hasNode := devices["utility.fritz_box"].Capabilities["node"]; hasNode {
		t.Errorf("Capabilities[node] should not have been synthesized for a hassbridge/import-kind dependent")
	}
}

// TestApplyLogicalEnablerDependencySugarSynthesizesNodeForHassbridgeDeviceWithEnabledBy is the
// regression test for the real gap found 2026-09-28 (reviewing this file with the user): a
// hassbridge/import-kind device's OWN "dependency on" alone is correctly left to
// depends_on_availability (see TestApplyLogicalEnablerDependencySugarSkipsHassbridgeOrImportDependents
// immediately above) -- but "enabled by" is an OR/exception ("unless the switch reports literally
// off, that's not a fault"), which MQTT's own "availability:" topic list can only ever AND, never
// express an OR/exception for. So a hassbridge/import-kind device that ALSO declares "enabled by"
// must still get the synthesized template "node" (and its own absorbed "enabler" capability) --
// the OLD code's blanket kind-based skip would have silently absorbed "enabler" as a capability
// while never actually wiring it into the device's own liveness at all.
func TestApplyLogicalEnablerDependencySugarSynthesizesNodeForHassbridgeDeviceWithEnabledBy(t *testing.T) {
	devices := map[string]TLogicalDevice{
		"appliance.washing_machine": {
			DeviceID:  "appliance.washing_machine",
			DependsOn: []string{"node.candy"},
			EnabledBy: &TLogicalEnabledByFact{
				OwnDomain:      "sensor",
				OwnLabel:       "status",
				SourceDomain:   "switch",
				SourceLabel:    "core",
				SourceDeviceID: "switch.washing_machine",
			},
			Capabilities: map[string]TLogicalCapability{},
		},
	}
	isHassbridgeOrImport := func(id string) bool { return id == "appliance.washing_machine" }

	warnings := applyLogicalEnablerDependencySugar(devices, isHassbridgeOrImport, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	node, hasNode := devices["appliance.washing_machine"].Capabilities["node"]
	if !hasNode || !node.IsDerivedCondition || node.Domain != "binary_sensor" {
		t.Fatalf("Capabilities[node] = %+v, want a synthesized derived-condition binary_sensor capability despite the hassbridge/import kind", node)
	}
	enabler, hasEnabler := devices["appliance.washing_machine"].Capabilities["enabler"]
	if !hasEnabler || enabler.Domain != "switch" || enabler.AbsorbedFromDeviceID != "switch.washing_machine" || enabler.AbsorbedFromCapability != "core" {
		t.Errorf("Capabilities[enabler] = %+v, want an absorbed switch.enabler from switch.washing_machine!core", enabler)
	}

	wantTree, exprWarnings := parseConditionExpr(`(sensor.status is available and node.candy) or jinja "$[switch.enabler] == 'off'"`, nil, "test")
	if len(exprWarnings) != 0 {
		t.Fatalf("ground-truth text itself failed to parse: %v", exprWarnings)
	}
	if !reflect.DeepEqual(node.DerivedCondition, wantTree) {
		t.Errorf("DerivedCondition = %+v, want %+v (the enabler wired into the liveness condition, not just absorbed)", node.DerivedCondition, wantTree)
	}
}

// TestApplyLogicalEnablerDependencySugarDependencyOnlyNoEnabler covers a device with "dependency
// on" but no "enabler" at all -- no "or jinja" OR-clause, just a plain AND chain.
func TestApplyLogicalEnablerDependencySugarDependencyOnlyNoEnabler(t *testing.T) {
	devices := map[string]TLogicalDevice{
		"appliance.example": {
			DeviceID:     "appliance.example",
			DependsOn:    []string{"utility.something"},
			Capabilities: map[string]TLogicalCapability{},
		},
	}

	warnings := applyLogicalEnablerDependencySugar(devices, noHassbridgeOrImport, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	node := devices["appliance.example"].Capabilities["node"]
	wantTree, exprWarnings := parseConditionExpr(`physical appliance.example and utility.something`, nil, "test")
	if len(exprWarnings) != 0 {
		t.Fatalf("ground-truth longhand itself failed to parse: %v", exprWarnings)
	}
	if !reflect.DeepEqual(node.DerivedCondition, wantTree) {
		t.Errorf("DerivedCondition = %+v, want %+v", node.DerivedCondition, wantTree)
	}
	if _, hasEnabler := devices["appliance.example"].Capabilities["enabler"]; hasEnabler {
		t.Errorf("Capabilities[enabler] should not exist when no \"enabler\" line was declared")
	}
}
