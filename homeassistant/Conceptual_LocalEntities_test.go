/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualLocalEntitiesTest
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 24.09.2026
 *
 */

package main

import (
	"strings"
	"testing"
)

// TestLocalDeviceNodeOverrideEndToEnd is the Ring doorbell's own shape, end to end: a "local"
// device (a camera entity native to HA main, plus a "node" capability tracking that camera's own
// availability) whose "node" is OVERRIDDEN by a Logical.def device sharing its id, via the new
// "physical <device-id>" condition shorthand (logical_condition_expr.go). Exercises, together, the
// full wiring this session's "local" integration kind touched: the explicit "entity
// binary_sensor.infrastructural:node from binary_sensor.node;" body line's dispatch hits the generic "node"
// override check at the top of registerDeviceCapabilityEntityLinkAllowingHidden (2026-09-24 --
// "node" is no longer auto-registered at positioning time for any kind, so this check now runs on
// every explicit "node" reference instead), which routes to the Logical.def override rather than
// the local device's own raw "node" capability, and autoMaterializePhysicalCapability's new local
// branch (reached via "physical appliance.front_door_ring", auto-materializing the local device's
// own un-overridden "node" capability on demand).
func TestLocalDeviceNodeOverrideEndToEnd(t *testing.T) {
	localDevicesByID := map[string]TLocalDevice{
		"appliance.front_door_ring": {
			DeviceID: "appliance.front_door_ring",
			Capabilities: map[string]TLocalCapability{
				"core": {Domain: "camera"},
				"node": {Domain: "binary_sensor", AvailabilityOf: "core", AvailabilityOfDomain: "camera"},
			},
		},
	}

	nodeTree, warnings := parseConditionExpr("physical appliance.front_door_ring", nil, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings building the node override condition: %v", warnings)
	}
	logicalDevicesByID := map[string]TLogicalDevice{
		"appliance.front_door_ring": {
			DeviceID: "appliance.front_door_ring",
			Capabilities: map[string]TLogicalCapability{
				"node": {Domain: "binary_sensor", IsDerivedCondition: true, DerivedCondition: nodeTree},
			},
		},
	}

	miniDSL := `conceptual layer with:
  space social:entrance as area with:
    device appliance.front_door_ring as front_door with:
      entity camera.social:ring from camera.core;
      entity binary_sensor.infrastructural:node from binary_sensor.node;
    end;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, localDevicesByID, logicalDevicesByID, nil)
	if err != nil {
		t.Fatalf("ParseEntitiesAndFillAdministration: %v", err)
	}
	admin := result.Administration

	const cameraEntityID = "camera.social_entrance_front_door_ring"
	link := admin.DeviceConceptualLinks["appliance.front_door_ring"]
	if got := link.AttributeEntityIDs["core"].EntityID; got != cameraEntityID {
		t.Fatalf("AttributeEntityIDs[core].EntityID = %q, want %q", got, cameraEntityID)
	}
	if camRec, ok := findEntityRecordByName(admin, "camera.social/entrance/front_door/ring"); !ok || camRec.HasDefinitionOrImport {
		t.Errorf("camera record = %+v (found=%v), want a plain bare entity (HasDefinitionOrImport false)", camRec, ok)
	}

	var nodeEntityID string
	for id, l := range admin.LogicalEntityLinks {
		if l.DeviceID == "appliance.front_door_ring" && l.Label == "node" {
			nodeEntityID = id
		}
	}
	if nodeEntityID == "" {
		t.Fatalf("no LogicalEntityLinks entry for appliance.front_door_ring's own \"node\"; report:\n%s", report.String())
	}

	nodeRec, ok := findEntityRecordByName(admin, "binary_sensor.infrastructural/entrance/front_door/node")
	if !ok {
		t.Fatalf("no entity record for the overridden node; report:\n%s", report.String())
	}
	if !nodeRec.HasDefinitionOrImport {
		t.Errorf("node record HasDefinitionOrImport = false, want true (it's a generator-authored condition entity)")
	}
	if !strings.Contains(nodeRec.ConditionExpr, "physical_appliance_front_door_ring_node") {
		t.Errorf("node ConditionExpr = %q, want it to reference the auto-materialized synthetic physical node entity", nodeRec.ConditionExpr)
	}

	// The local device's own raw "node" (auto-materialized on demand via the override's "physical
	// appliance.front_door_ring" reference) must itself be a generator-authored condition entity --
	// see Conceptual_LogicalEntities.go's own "local" branch in autoMaterializePhysicalCapability.
	physicalNodeRec, ok := findEntityRecordByName(admin, "binary_sensor.physical/appliance.front_door_ring/node")
	if !ok {
		t.Fatalf("no entity record for the auto-materialized synthetic physical node")
	}
	if !physicalNodeRec.HasDefinitionOrImport {
		t.Errorf("synthetic physical node HasDefinitionOrImport = false, want true")
	}
	if len(physicalNodeRec.ConditionSources) != 1 || physicalNodeRec.ConditionSources[0] != cameraEntityID {
		t.Errorf("synthetic physical node ConditionSources = %v, want [%q] (checking the camera's own availability)", physicalNodeRec.ConditionSources, cameraEntityID)
	}
}

// TestLocalCapabilityNoCollectExcludesFromSpaceMediaByName is the regression test for a real bug
// found live 2026-09-29 (Junglinster's office, migrating apple_tv/tv off the old
// media_player_device macro's own "no_collect;" flag): a local-kind device's "media_player.core"
// capability positioned with "with no_collect" parsed clean (TDeviceCapabilityEntityDeclaration.
// NoCollect set correctly) but registerLocalCapabilityEntityLink's own "bare capability" branch
// never threaded that flag into the TEntityRecord it built -- so the entity was NEVER actually
// excluded from SpaceMediaByName (the @media/space-off aggregation) despite "no_collect" being
// honoured for the exact same suffix everywhere else (registerDeviceCapabilityEntityLinkAllowingHiddenCore,
// the discovery-kind path). Confirmed live: office's "space off" switch/state list included
// apple_tv/tv's own media_player entities even with "no_collect" written on their own positioning
// line, where the pre-migration macro-driven deploy correctly excluded them.
func TestLocalCapabilityNoCollectExcludesFromSpaceMediaByName(t *testing.T) {
	localDevicesByID := map[string]TLocalDevice{
		"appliance.tv_office": {
			DeviceID: "appliance.tv_office",
			Capabilities: map[string]TLocalCapability{
				"core": {Domain: "media_player"},
			},
		},
	}

	miniDSL := `conceptual layer with:
  space social:office as area with:
    device appliance.tv_office as tv with:
      entity media_player.social: from media_player.core with no_collect;
    end;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, localDevicesByID, nil, nil)
	if err != nil {
		t.Fatalf("ParseEntitiesAndFillAdministration: %v", err)
	}
	admin := result.Administration

	rec, ok := findEntityRecordByName(admin, "media_player.social/office/tv")
	if !ok {
		t.Fatalf("no entity record for the local media_player capability; report:\n%s", report.String())
	}
	if !rec.NoCollect {
		t.Errorf("media_player entity record NoCollect = false, want true (the \"with no_collect\" suffix must be honoured)")
	}
	for _, name := range admin.SpaceMediaByName["social/office"] {
		if name == "media_player.social/office/tv" {
			t.Errorf("SpaceMediaByName[%q] wrongly includes %q despite \"with no_collect\"", "social/office", name)
		}
	}
}

// TestLocalDeviceCapabilityFallsThroughToPureLogicalOverlay covers the Ring doorbell's own
// "battery_alert" shape: a capability with NO physical "local" counterpart at all, declared only
// in Logical.def on the SAME device id -- must resolve through dispatchLogicalCapability's fallback
// inside registerDeviceCapabilityEntityLinkAllowingHidden's "local" branch, exactly like the
// hassbridge/discovery/imported branches' own identical fallback.
func TestLocalDeviceCapabilityFallsThroughToPureLogicalOverlay(t *testing.T) {
	localDevicesByID := map[string]TLocalDevice{
		"appliance.front_door_ring": {
			DeviceID:     "appliance.front_door_ring",
			Capabilities: map[string]TLocalCapability{"level": {Domain: "sensor"}},
		},
	}
	valueTree, warnings := parseConditionExpr("sensor.level", nil, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings building the value expression: %v", warnings)
	}
	logicalDevicesByID := map[string]TLogicalDevice{
		"appliance.front_door_ring": {
			DeviceID: "appliance.front_door_ring",
			Capabilities: map[string]TLogicalCapability{
				"battery_alert": {Domain: "sensor", IsDerivedValue: true, DerivedValue: valueTree},
			},
		},
	}

	miniDSL := `conceptual layer with:
  device appliance.front_door_ring as front_door with:
    entity sensor.infrastructural:level from sensor.level;
    entity sensor.infrastructural:battery_alert from sensor.battery_alert;
  end;
end;`

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", nil, &report, nil, nil, nil, nil, nil, localDevicesByID, logicalDevicesByID, nil)
	if err != nil {
		t.Fatalf("ParseEntitiesAndFillAdministration: %v", err)
	}
	admin := result.Administration

	if _, ok := findEntityRecordByName(admin, "sensor.infrastructural/front_door/battery_alert"); !ok {
		t.Fatalf("no entity record for the pure Logical.def \"battery_alert\" capability; report:\n%s", report.String())
	}
}
