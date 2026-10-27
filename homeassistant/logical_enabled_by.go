/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: LogicalEnabledBy
 *
 * Logical.def's device statement "enabled by <domain>.<label> from <device-id> [as <id>] [with
 * delay_off <time>];" (PROJECT.md 17k, 2026-10-26, Erik): the device is powered by another
 * device's switch, e.g.
 *
 *   device server.xanadu with:
 *     enabled by switch.switch from switch.house_server_room_xanadu;
 *     absorb from node.xanadu: ... end;
 *   end;
 *
 * It replaces the pattern every such device used to spell out by hand: the switch absorbed as the
 * device's "enabler", and a node condition ending in "or jinja "$[switch.enabler] == 'off'"" -- a
 * device whose power is off on purpose is not down. applyEnabledBy turns the statement into
 * exactly those capabilities, so everything downstream (positioning, the enabler node of
 * logical_enabler_nodes.go, frozen unique_ids) sees what the hand-written form gave it.
 *
 * The node is implied: (the device's own Physical.def node, if it has one) and (the node of every
 * other device it is built from -- absorbed, colocated or depended on) or (the enabler is off). The
 * "as <id>" and "with delay_off" belong to that implied node. A device that does declare its own
 * node keeps it, with "or the enabler is off" added.
 *
 * Unlike the "enabled by" sugar removed in PROJECT.md item 11a, this never builds a base
 * condition of its own when a real one exists: it only adds the "or off" part to it.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 26.10.2026
 *
 */

package main

import (
	"fmt"
	"sort"
	"strings"
)

// hasPhysicalNode reports whether deviceID declares a node capability of its own in Physical.def.
func hasPhysicalNode(deviceID string, localDevicesByID map[string]TLocalDevice, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice) bool {
	if device, found := localDevicesByID[deviceID]; found {
		_, has := device.Capabilities["node"]
		return has
	}
	if gateway, found := discoveryGatewaysByID[deviceID]; found {
		_, has := gateway.Capabilities["node"]
		return has
	}
	if device, found := hassBridgeDevicesByID[deviceID]; found {
		_, has := device.Capabilities["node"]
		return has
	}
	return false
}

// applyEnabledBy expands every "enabled by" fact in logicalByID into the device's "enabler"
// capability and its node (see this file's header comment), and reports what it can't expand.
func applyEnabledBy(logicalByID map[string]TLogicalDevice, localDevicesByID map[string]TLocalDevice, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice) []string {
	ids := make([]string, 0, len(logicalByID))
	for id, device := range logicalByID {
		if device.EnabledBy != nil {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	var warnings []string
	for _, id := range ids {
		device := logicalByID[id]
		fact := device.EnabledBy
		source := fmt.Sprintf("Logical.def: device %q's \"enabled by\"", id)
		if _, has := device.Capabilities["enabler"]; has {
			warnings = append(warnings, fmt.Sprintf("%s: it also has an \"enabler\" capability of its own -- drop that one", source))
			continue
		}
		device.Capabilities["enabler"] = TLogicalCapability{Domain: fact.Domain, AbsorbedFromDeviceID: fact.DeviceID, AbsorbedFromCapability: fact.Label}
		enablerOff := fmt.Sprintf("jinja \"$[%s.enabler] == 'off'\"", fact.Domain)

		if node, has := device.Capabilities["node"]; has {
			if !node.IsDerivedCondition {
				warnings = append(warnings, fmt.Sprintf("%s: its own node is no derived condition, so \"or the enabler is off\" can't be added to it", source))
			} else {
				off, w := parseConditionExpr(enablerOff, nil, source)
				warnings = append(warnings, w...)
				node.DerivedCondition = &TConditionExpr{Kind: CondOr, Left: node.DerivedCondition, Right: off}
				device.Capabilities["node"] = node
			}
			if fact.AssignedID != "" || fact.DelayOff != "" {
				warnings = append(warnings, fmt.Sprintf("%s: its \"as\"/\"with delay_off\" belong to an implied node, but the device declares its own -- put them there", source))
			}
			logicalByID[id] = device
			continue
		}

		var parts []string
		if hasPhysicalNode(id, localDevicesByID, discoveryGatewaysByID, hassBridgeDevicesByID) {
			parts = append(parts, "physical "+id)
		}
		var builtFrom []string
		for _, capability := range device.Capabilities {
			if capability.AbsorbedFromDeviceID != "" {
				builtFrom = addIfAbsent(builtFrom, capability.AbsorbedFromDeviceID)
			}
		}
		for _, fact := range device.Colocated {
			builtFrom = addIfAbsent(builtFrom, fact.DeviceID)
		}
		sort.Strings(builtFrom)
		for _, dependency := range append(builtFrom, device.DependsOn...) {
			if dependency != fact.DeviceID && dependency != id && !containsString(parts, dependency) {
				parts = append(parts, dependency)
			}
		}
		if len(parts) == 0 {
			warnings = append(warnings, fmt.Sprintf("%s: the device has no node of its own and is built from nothing that has one, so its node can't be implied -- declare it", source))
			logicalByID[id] = device
			continue
		}
		tree, w := parseConditionExpr("("+strings.Join(parts, " and ")+") or "+enablerOff, nil, source)
		warnings = append(warnings, w...)
		device.Capabilities["node"] = TLogicalCapability{
			Domain:             "binary_sensor",
			IsDerivedCondition: true,
			DerivedCondition:   tree,
			DelayOff:           fact.DelayOff,
			AssignedID:         fact.AssignedID,
		}
		logicalByID[id] = device
	}
	return warnings
}
