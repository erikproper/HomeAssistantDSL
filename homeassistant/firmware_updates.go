/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: FirmwareUpdates
 *
 * PROJECT.md 17i (2026-10-26, Erik): a device's firmware update follows its node. It is written
 * into the defs like any capability -- Physical.def "capability update.update <leaf> as
 * firmware_<leaf>;", Conceptual.def "entity update.infrastructural:<node leaf>/firmware from
 * update;" next to the device's node line. A device whose node the generator places itself (a
 * colocated plug, a logical node) gets its firmware update placed the same way, here:
 * "update.infrastructural:<its node's path>/firmware". Next to it, the house gets one
 * "binary_sensor.infrastructural:firmware_updates", on while any device has an update waiting.
 * Home Assistant only shows them: the coordinator relays update entities without their install
 * command (discoverybridge.go).
 *
 * Which devices report an update comes from the coordinator's discovery existence cache
 * (Definitions/.cache/discovery_existence.json, refreshed by every generate), so a newly paired
 * device shows its update one generate later. An update capability positioned explicitly in
 * Conceptual.def keeps that positioning.
 *
 * Z-Wave devices have none: Z-Wave JS UI publishes no update entities over MQTT.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 26.10.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// firmwareUpdateLeafSuffix ends the leaf of a Zigbee2MQTT device's own firmware update entity.
const firmwareUpdateLeafSuffix = "_update_zigbee2mqtt"

// deviceNodeEntityID returns the entity_id of deviceID's own derived node (Physical.def or
// Logical.def), if it has exactly one.
func deviceNodeEntityID(admin *TAdministrationState, deviceID string) (string, bool) {
	for _, layer := range []string{"logical:", "physical:"} {
		var ids []string
		for entityID, key := range admin.SourceKeyByEntityID {
			if key == layer+deviceID+"!node" {
				if link, absorbed := admin.LogicalEntityLinks[entityID]; absorbed && link.DeviceID != deviceID {
					continue
				}
				ids = append(ids, entityID)
			}
		}
		if len(ids) == 1 {
			return ids[0], true
		}
	}
	return "", false
}

// registerFirmwareUpdates adds the firmware update of every positioned device that reports one,
// and the house's firmware_updates summary (see this file's header comment).
func registerFirmwareUpdates(admin *TAdministrationState, definitionDir string, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice) []string {
	raw, err := os.ReadFile(discoveryExistenceAggregateCachePath(definitionDir))
	if err != nil {
		return nil // no cache yet: the next generate has one
	}
	var status TDiscoveryExistenceAggregatePayload
	if err := json.Unmarshal(raw, &status); err != nil {
		return []string{fmt.Sprintf("firmware updates: unreadable discovery existence cache: %v", err)}
	}
	linked := map[string]bool{}
	for _, link := range admin.DiscoveryEntityLinks {
		linked[link.GatewayDeviceID+"!"+link.Leaf] = true
	}

	gatewayIDs := make([]string, 0, len(status))
	for id := range status {
		if _, declared := discoveryGatewaysByID[id]; declared {
			gatewayIDs = append(gatewayIDs, id)
		}
	}
	sort.Strings(gatewayIDs)

	var updates []string
	for _, gatewayID := range gatewayIDs {
		for leaf, leafStatus := range status[gatewayID] {
			if leafStatus != "known-to-exist" || !strings.HasSuffix(leaf, firmwareUpdateLeafSuffix) || linked[gatewayID+"!"+leaf] {
				continue
			}
			nodeEntityID, found := deviceNodeEntityID(admin, gatewayID)
			if !found {
				continue // a device without a positioned node isn't positioned at all
			}
			nodeRecord, spaceName, found := entityRecordByEntityID(admin, nodeEntityID)
			path, isNode := strings.CutSuffix(nodeRecord.Identity.Path, "/node")
			if !found || !isNode {
				continue
			}
			name := "update.infrastructural/" + path + "/firmware"
			entityID := toHomeAssistantEntityID(name)
			admin.RegisterDiscoveryImpliedEntity(spaceName, name, "firmware update of "+gatewayID, gatewayID+"!"+leaf, true)
			admin.DiscoveryEntityLinks[entityID] = TDiscoveryEntityLink{
				EntityID:        entityID,
				GatewayDeviceID: gatewayID,
				Leaf:            leaf,
				StableID:        "firmware_" + leaf,
			}
			updates = append(updates, entityID)
		}
	}
	for _, link := range admin.DiscoveryEntityLinks {
		if strings.HasPrefix(link.EntityID, "update.") && !containsString(updates, link.EntityID) {
			updates = append(updates, link.EntityID)
		}
	}
	if len(updates) == 0 {
		return nil
	}
	sort.Strings(updates)

	terms := make([]string, len(updates))
	for i, entityID := range updates {
		terms[i] = fmt.Sprintf("is_state('%s', 'on')", entityID)
	}
	const summary = "binary_sensor.infrastructural/firmware_updates"
	admin.AppendEntityRecord("root", TEntityRecord{
		Name:                  summary,
		Identity:              extractEntityIdentity(summary),
		Provenance:            fmt.Sprintf("firmware updates of %d devices", len(updates)),
		ConditionExpr:         "(" + strings.Join(terms, " or ") + ")",
		ConditionDevClass:     "update",
		HasDefinitionOrImport: true,
	})
	return nil
}
