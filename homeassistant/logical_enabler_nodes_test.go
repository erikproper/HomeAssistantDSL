package main

import "testing"

// TestRegisterEnablerNodes covers Junglinster's kitchen workplace light: switched by a relay whose
// node the WC fan also absorbs. The light gets ".../light/enabler/node" following the relay's own
// node (not the fan's absorbed copy), under a unique_id rooted in the light node's frozen one, and
// its own node also counts as up while the relay is off.
func TestRegisterEnablerNodes(t *testing.T) {
	admin := newAdministrationState()
	addNode := func(spaceName, name, sourceKey string) {
		admin.AppendEntityRecord(spaceName, TEntityRecord{Name: name, SourceKey: sourceKey, Identity: extractEntityIdentity(name)})
	}
	admin.AppendEntityRecord("social/house/kitchen", TEntityRecord{
		Name:             "binary_sensor.infrastructural/house/kitchen/workplace/light/node",
		SourceKey:        "physical:lights.workplace_dimmer!node",
		Identity:         extractEntityIdentity("binary_sensor.infrastructural/house/kitchen/workplace/light/node"),
		ConditionSources: []string{"light.physical_workplace_dimmer"},
		ConditionExpr:    "$ != 'unavailable'",
	})
	admin.SwitchedDeviceRelations = append(admin.SwitchedDeviceRelations, TSwitchedDeviceRelation{DeviceID: "lights.workplace_dimmer", MainEntity: "light.physical/switches.zwave_058"})
	addNode("social/garage", "binary_sensor.infrastructural/garage/zwave_058/node", "physical:switches.zwave_058!node")
	addNode("social/house/wc", "binary_sensor.infrastructural/house/wc/fan/node", "physical:switches.zwave_058!node")
	admin.LogicalEntityLinks["binary_sensor.infrastructural_house_wc_fan_node"] = TLogicalEntityLink{DeviceID: "fan.house_wc", Label: "node"}
	admin.FrozenUniqueIDs["physical:lights.workplace_dimmer!node"] = "binary_sensor.frozen_workplace_node"

	logicalByID := map[string]TLogicalDevice{
		"lights.workplace_dimmer": {
			DeviceID:   "lights.workplace_dimmer",
			SwitchedBy: map[string]TLogicalSwitchedByFact{"light": {SourceDeviceID: "switches.zwave_058"}},
		},
	}
	if warnings := registerEnablerNodes(admin, logicalByID, nil, nil, nil, nil); len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	const name = "binary_sensor.infrastructural/house/kitchen/workplace/light/enabler/node"
	rec, found := findEntityRecordByName(admin, name)
	if !found {
		t.Fatalf("no entity record for %s", name)
	}
	if want := "is_state('binary_sensor.infrastructural_garage_zwave_058_node', 'on')"; rec.ConditionExpr != want {
		t.Errorf("ConditionExpr = %q, want %q (the relay's own node)", rec.ConditionExpr, want)
	}
	if got, want := admin.UniqueIDFor(toHomeAssistantEntityID(name)), "binary_sensor.frozen_workplace_node_enabler"; got != want {
		t.Errorf("unique_id = %q, want %q", got, want)
	}

	node, _ := findEntityRecordByName(admin, "binary_sensor.infrastructural/house/kitchen/workplace/light/node")
	if want := "((states('light.physical_workplace_dimmer') != 'unavailable') or (states('light.physical_switches_zwave_058') == 'off'))"; buildConditionStateExpr(node.ConditionSources, node.ConditionExpr) != want {
		t.Errorf("light node condition = %q, want %q", buildConditionStateExpr(node.ConditionSources, node.ConditionExpr), want)
	}
}
