/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: LogicalEnablerNodes
 *
 * A device that is enabled by another device's switch -- "<label> is switched by ... from
 * <device-id>;" (logical_switched_by.go), or an absorbed "capability switch.enabler ...;" -- gets
 * a second node next to its own: "infrastructural/<node path>/enabler/node", following the
 * enabling device's own node (2026-10-26, Erik). The enabling device keeps its node where it is
 * positioned (e.g. a relay in the fuse cabinet); this one shows, at the enabled device, whether
 * what powers it can be reached.
 *
 * Its unique_id is rooted in the frozen unique_id of the device's own node ("<id>_enabler"), so it
 * survives renames the way every other generated entity does, without a line of its own in
 * Logical.def to carry an "as <id>".
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

// enablerDeviceIDs returns the devices whose switch enables device, sorted: the source of every
// "is switched by" fact, and the device an "enabler" capability is absorbed from.
func enablerDeviceIDs(device TLogicalDevice) []string {
	var ids []string
	for _, fact := range device.SwitchedBy {
		ids = addIfAbsent(ids, fact.SourceDeviceID)
	}
	if capability, has := device.Capabilities["enabler"]; has && capability.AbsorbedFromDeviceID != "" {
		ids = addIfAbsent(ids, capability.AbsorbedFromDeviceID)
	}
	sort.Strings(ids)
	return ids
}

// entityRecordByEntityID returns the entity record entityID is generated from, and its space.
func entityRecordByEntityID(admin *TAdministrationState, entityID string) (record TEntityRecord, spaceName string, found bool) {
	for name, records := range admin.EntityRecordsBySpace {
		for _, candidate := range records {
			if toHomeAssistantEntityID(candidate.Name) == entityID {
				return candidate, name, true
			}
		}
	}
	return TEntityRecord{}, "", false
}

// registerEnablerNodes adds the enabler node of every logical device with an enabler (see this
// file's header comment), and returns a warning for each one it can't place: the device's own node
// or the enabler's node isn't positioned.
func registerEnablerNodes(admin *TAdministrationState, logicalByID map[string]TLogicalDevice, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, hostDevicesByID map[string]THostDevice, localDevicesByID map[string]TLocalDevice) []string {
	// A derived node (Physical.def or Logical.def) is a generated record, found by its source key;
	// any other kind (a host's, a discovery leaf's) through the ordinary sibling resolution. A node
	// another logical device absorbs is that device's node, not its source's (e.g. Junglinster's WC
	// fan absorbs the node of the relay that also enables the kitchen workplace light).
	derivedNodes := map[string][]string{}
	for entityID, key := range admin.SourceKeyByEntityID {
		for _, layer := range []string{"logical:", "physical:"} {
			deviceID, isNode := strings.CutSuffix(strings.TrimPrefix(key, layer), "!node")
			if link, absorbed := admin.LogicalEntityLinks[entityID]; absorbed && link.DeviceID != deviceID {
				continue
			}
			if strings.HasPrefix(key, layer) && isNode {
				derivedNodes[layer+deviceID] = append(derivedNodes[layer+deviceID], entityID)
			}
		}
	}
	ambiguous := map[string][]string{}
	resolveNode := func(deviceID string) (string, bool) {
		for _, layer := range []string{"logical:", "physical:"} {
			if ids := derivedNodes[layer+deviceID]; len(ids) == 1 {
				return ids[0], true
			} else if len(ids) > 1 {
				sort.Strings(ids)
				ambiguous[deviceID] = ids
				return "", false
			}
		}
		entityID, ok, _ := resolveLogicalSiblingSourceEntityID(admin, deviceID, "node", false, false, true, discoveryGatewaysByID, hassBridgeDevicesByID, hostDevicesByID, localDevicesByID, "enabler node")
		return entityID, ok
	}

	deviceIDs := make([]string, 0, len(logicalByID))
	for id := range logicalByID {
		deviceIDs = append(deviceIDs, id)
	}
	sort.Strings(deviceIDs)

	var warnings []string
	for _, deviceID := range deviceIDs {
		enablers := enablerDeviceIDs(logicalByID[deviceID])
		if len(enablers) == 0 {
			continue
		}
		nodeEntityID, ok := resolveNode(deviceID)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("%s has an enabler, but no positioned node of its own to place its enabler node next to", deviceID))
			continue
		}
		nodeRecord, spaceName, found := entityRecordByEntityID(admin, nodeEntityID)
		path, isNode := strings.CutSuffix(nodeRecord.Identity.Path, "/node")
		if !found || !isNode {
			warnings = append(warnings, fmt.Sprintf("%s: its node %s is not a generated \".../node\" entity, so its enabler node has no place", deviceID, nodeEntityID))
			continue
		}
		for _, enablerID := range enablers {
			enablerNodeEntityID, ok := resolveNode(enablerID)
			if !ok {
				if ids := ambiguous[enablerID]; len(ids) > 0 {
					warnings = append(warnings, fmt.Sprintf("%s: its enabler %s has several nodes (%s)", deviceID, enablerID, strings.Join(ids, ", ")))
				} else {
					warnings = append(warnings, fmt.Sprintf("%s: its enabler %s has no positioned node", deviceID, enablerID))
				}
				continue
			}
			leaf, sourceKey := "enabler", "enabler:"+deviceID
			if len(enablers) > 1 {
				leaf += "/" + enablerID[strings.Index(enablerID, ".")+1:]
				sourceKey += "!" + enablerID
			}
			name := fmt.Sprintf("binary_sensor.infrastructural/%s/%s/node", path, leaf)
			admin.FrozenUniqueIDs[sourceKey] = admin.UniqueIDFor(nodeEntityID) + "_" + strings.ReplaceAll(leaf, "/", "_")
			admin.AppendEntityRecord(spaceName, TEntityRecord{
				Name:                  name,
				SourceKey:             sourceKey,
				Identity:              extractEntityIdentity(name),
				Provenance:            fmt.Sprintf("enabler node of %s, following %s", deviceID, enablerNodeEntityID),
				ConditionExpr:         fmt.Sprintf("is_state('%s', 'on')", enablerNodeEntityID),
				ConditionDevClass:     "connectivity",
				HasDefinitionOrImport: true,
			})
		}
		if len(logicalByID[deviceID].SwitchedBy) > 0 {
			extendNodeWithSwitchedOff(admin, deviceID, nodeEntityID, spaceName)
		}
	}
	return warnings
}

// extendNodeWithSwitchedOff makes a switched device's own node also count as up while what
// switches it is off (2026-10-26, Erik): a light behind a relay is unreachable exactly then, which
// is no fault -- the "or jinja \"$[switch.enabler] == 'off'\"" a logical device with an absorbed
// enabler spells out in its own node condition. Only a generated condition node is extended.
func extendNodeWithSwitchedOff(admin *TAdministrationState, deviceID, nodeEntityID, spaceName string) {
	var switches []string
	for _, relation := range admin.SwitchedDeviceRelations {
		if relation.DeviceID == deviceID {
			switches = addIfAbsent(switches, fmt.Sprintf("(states('%s') == 'off')", toHomeAssistantEntityID(relation.MainEntity)))
		}
	}
	if len(switches) == 0 {
		return
	}
	records := admin.EntityRecordsBySpace[spaceName]
	for i := range records {
		if toHomeAssistantEntityID(records[i].Name) != nodeEntityID || records[i].ConditionExpr == "" {
			continue
		}
		own := buildConditionStateExpr(records[i].ConditionSources, records[i].ConditionExpr)
		records[i].ConditionSources = nil
		records[i].ConditionExpr = "((" + own + ") or " + strings.Join(switches, " or ") + ")"
	}
}
