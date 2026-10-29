/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: CommandlineYAMLEntities
 *
 * PROJECT.md 19e (2026-10-27): a device of an "integration commandline" block gets its entities
 * from the generator's own YAML "mqtt:" config instead of the coordinator's discovery --
 * entities/mqtt/<domain>/<entity_id>.yaml, like the hosts devices' (hosts_yaml_entities.go). Each
 * entity is exactly what the coordinator's discovery published (its former
 * buildCommandlineDiscoveryConfigs): the
 * daemon's liveness binary_sensor and one switch/sensor/button per capability, with the same
 * unique_ids, entity_ids, names, topics, payloads and availability (the daemon's own node topic
 * ANDed with every "dependency on" node topic). The coordinator gets nothing about such a device:
 * it only relays the daemon's own topics.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 27.10.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// generateYAMLCommandlineEntities writes the YAML "mqtt:" entities of every commandline device
// (see this file's header comment), and reports what it wrote.
func generateYAMLCommandlineEntities(devices []TCommandlineDevice, ctx TPhysicalGenerationContext) []string {
	if ctx.HAOutputDir == "" {
		return nil // no Home Assistant output directory (a test): nothing to write, never relative to the working directory
	}
	var reports []string
	for _, d := range devices {
		written := 0
		for _, entity := range commandlineYAMLEntityBodies(d, ctx.Admin) {
			data, err := yaml.Marshal([]map[string]interface{}{{entity.Domain: entity.Body}})
			if err != nil {
				reports = append(reports, fmt.Sprintf("%s: %v", entity.EntityID, err))
				continue
			}
			path := filepath.Join(ctx.HAOutputDir, "entities", "mqtt", entity.Domain, entity.EntityID+".yaml")
			if err := writeYAMLFile(path, generatorHeader+string(data)); err != nil {
				reports = append(reports, fmt.Sprintf("%s: %v", entity.EntityID, err))
				continue
			}
			written++
		}
		reports = append(reports, fmt.Sprintf("%s: %d entities via YAML", d.DeviceID, written))
	}
	return reports
}

// commandlineYAMLEntityBodies carries the rules of the coordinator's former
// buildCommandlineDiscoveryConfigs (discoverycommandline.go, removed 2026-10-27); its
// documentation, with the live bug behind it, follows unchanged.
//
// buildCommandlineDiscoveryConfigs returns every HA MQTT Discovery config implied by one
// commandline.yaml device: its own daemon-liveness entity, plus one switch/sensor/button config
// per declared capability. Mirrors buildDiscoveryConfigs' (discovery.go) device-block/unique_id/
// topic conventions so entities from a "hosts"-kind declaration sharing the SAME deviceID (e.g. a
// picture frame that's both "host.frame" for ping/cpu AND "host.frame" for commandline) merge into
// one HA device rather than appearing as two -- both use identical device.Identifiers.
//
// The liveness entity's own unique_id is deliberately "<deviceID>_commandline_node", NOT
// "<deviceID>_node" -- a real bug found live 2026-09-06: a "hosts"-kind declaration sharing the
// same deviceID (as the doc comment above assumes) already owns "<deviceID>_node" for its own
// ping-based liveness signal. Reusing that exact unique_id here made the two integrations fight
// over one HA entity, each one's own periodic republish (hosts' on every live device-info report,
// commandline's at startup) clobbering the other's payload every time -- confirmed live via
// [discovery-cleanup] repeatedly retiring/republishing homeassistant/binary_sensor/coordinator/
// host_frame_node/config. The two liveness signals are semantically different (host reachable vs.
// this specific daemon process alive, PROJECT.md item 1's own stated rationale for using LWT here
// instead of ping-style staleness) and must get their own separate HA entities; only the shared
// Device.Identifiers should be reused, grouping them visually under the one physical device.
//
// commandlineYAMLEntityBodies returns d's YAML entities, in that shape.
func commandlineYAMLEntityBodies(d TCommandlineDevice, admin *TAdministrationState) []THostYAMLEntity {
	deviceName := d.Host
	if admin != nil {
		if link, ok := admin.DeviceConceptualLinks[d.DeviceID]; ok && link.DisplayName != "" {
			deviceName = link.DisplayName
		}
	}
	device := map[string]interface{}{"identifiers": []string{d.DeviceID}, "name": deviceName}
	host := strings.ReplaceAll(d.Host, ".", "_") // the coordinator's sanitizeTopicSegment
	nodeTopic := "commandline/" + d.Host + "/node/state"
	var availability []map[string]interface{}
	for _, topic := range append([]string{nodeTopic}, d.DependsOnAvailabilityTopics...) {
		availability = append(availability, map[string]interface{}{
			"topic":                 topic,
			"value_template":        "{{ 'on' if value in ['on', 'true', 'online'] else 'off' }}",
			"payload_available":     "on",
			"payload_not_available": "off",
		})
	}
	withCommon := func(body map[string]interface{}) map[string]interface{} {
		body["availability"] = availability
		body["availability_mode"] = "all"
		body["device"] = device
		return body
	}

	nodeEntityID, nodeName := commandlineCapabilityRef(admin, d.DeviceID, "node")
	if nodeEntityID == "" {
		nodeEntityID = "binary_sensor." + host + "_commandline_node"
		nodeName = deviceName + "/commandline"
	}
	entities := []THostYAMLEntity{{Domain: "binary_sensor", EntityID: nodeEntityID, Body: withCommon(map[string]interface{}{
		"unique_id":         d.DeviceID + "_commandline_node",
		"default_entity_id": nodeEntityID,
		"name":              nodeName,
		"state_topic":       nodeTopic,
		"payload_on":        hostsNodePayloadOn,
		"payload_off":       hostsNodePayloadOff,
		"device_class":      "connectivity",
	})}}

	names := make([]string, 0, len(d.Capabilities))
	for name := range d.Capabilities {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		capability := d.Capabilities[name]
		entityID, displayName := commandlineCapabilityRef(admin, d.DeviceID, name)
		uniqueID := d.DeviceID + "_" + name
		if entityID == "" {
			entityID = capability.Kind + "." + host + "_" + name
			displayName = d.Host + "/" + name
		} else if capability.AssignedID != "" {
			uniqueID = capability.AssignedID
		}
		body := map[string]interface{}{"unique_id": uniqueID, "default_entity_id": entityID, "name": displayName}
		base := "commandline/" + d.Host + "/" + name
		switch capability.Kind {
		case "switch":
			body["state_topic"] = base + "/state"
			body["command_topic"] = base + "/set"
			body["payload_on"], body["payload_off"] = "1", "0"
			body["state_on"], body["state_off"] = "1", "0"
		case "sensor":
			body["state_topic"] = base + "/state"
		case "button":
			body["command_topic"] = base + "/press"
			body["payload_press"] = "PRESS"
		default:
			continue
		}
		entities = append(entities, THostYAMLEntity{Domain: capability.Kind, EntityID: entityID, Body: withCommon(body)})
	}
	return entities
}
