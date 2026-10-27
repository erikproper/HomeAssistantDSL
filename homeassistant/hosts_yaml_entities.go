/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: HostsYAMLEntities
 *
 * PROJECT.md 19e (2026-10-27): a local "hosts" device gets its entities from the generator's own
 * YAML "mqtt:" config instead of the coordinator's discovery -- one file per entity, entities/mqtt/<domain>/<entity_id>.yaml, next to
 * the discovery devices' (mqtt_yaml_entities.go). "hosts" is a controlled integration
 * (Architecture.md §6.16): its topics and payloads are ours, so the defs alone say everything --
 * the device's node binary_sensor and its variable attribute sensors (cpu load/temperature, ...).
 *
 * Each entity is exactly what the coordinator's discovery published for it
 * (house_event_bus_coordinator/discovery.go, buildDiscoveryConfigs): the same unique_id
 * ("<device-id>_node", "<device-id>_<attribute>"), default_entity_id, topics, payloads and
 * availability, so the registry entry, device and history stay. Two things differ: the name is the
 * entity's full conceptual name (PROJECT.md 17h -- the node had none, an attribute only its device
 * path), and the device block takes its metadata from Physical.def alone (what the host reports
 * is captured there, captureHostMetadata), not from the live report. The coordinator publishes no
 * local discovery (PROJECT.md 19); for a cloud-routed device it keeps publishing its cloud copy,
 * the catalogue another house imports from (PROJECT.md 19h).
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

	"homeassistant/hamqtt"

	"gopkg.in/yaml.v3"
)

// Payloads of a hosts device's node topic, as its reporters publish them
// (house_event_bus_coordinator: mqttBoolPayloadTrue/False).
const (
	hostsNodePayloadOn  = "true"
	hostsNodePayloadOff = "false"
)

// hostDeviceViaYAML reports whether d's entities come from YAML: positioned, and with entities on
// this house's broker -- a local device, or a cloud-routed one read back here ("cloud import": the
// coordinator relays its cloud topics onto the same local hosts/<host>/... topics, PROJECT.md 19h).
// A cloud-routed device without "import" has no local entities at all.
func hostDeviceViaYAML(d THostDevice, admin *TAdministrationState) bool {
	if (d.Cloud && d.ImportedFrom == "") || admin == nil {
		return false
	}
	_, positioned := admin.DeviceConceptualLinks[d.DeviceID]
	return positioned
}

// generateYAMLHostsEntities writes the YAML "mqtt:" entities of every local, positioned hosts
// device (see this file's header comment), and reports what it wrote.
func generateYAMLHostsEntities(devices []THostDevice, ctx TPhysicalGenerationContext) []string {
	if ctx.HAOutputDir == "" {
		return nil // no Home Assistant output directory (a test): nothing to write, never relative to the working directory
	}
	var reports []string
	seen := map[string]bool{}
	for _, d := range devices {
		if seen[d.DeviceID] || !hostDeviceViaYAML(d, ctx.Admin) {
			continue
		}
		seen[d.DeviceID] = true
		mat, known := MaterializationForIntegrationType(d.IntegrationType)
		if !known {
			continue
		}
		entities := hostYAMLEntityBodies(d, ctx.Admin.DeviceConceptualLinks[d.DeviceID], mat, ctx.Admin)
		written := 0
		for _, entity := range entities {
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

// THostYAMLEntity is one YAML "mqtt:" entity of a hosts device (and of the other writers).
type THostYAMLEntity struct {
	Domain   string
	EntityID string
	Body     map[string]interface{}
	// Dropped: the fields of its conceptual config Home Assistant's schema doesn't know for Domain,
	// which discoveredYAMLEntity left out.
	Dropped []string
}

// discoveredYAMLEntity is entityID's YAML entity: body as Home Assistant's own discovery would
// keep it (hamqtt.DiscoveredConfig), noting what that drops.
func discoveredYAMLEntity(domain, entityID string, body map[string]interface{}) THostYAMLEntity {
	return THostYAMLEntity{Domain: domain, EntityID: entityID, Body: hamqtt.DiscoveredConfig(body, domain), Dropped: hamqtt.DroppedFields(body, domain)}
}

// droppedFieldsNote summarises the fields Home Assistant doesn't know and the YAML therefore drops,
// per domain with the number of entities ("; dropped, unknown to Home Assistant: button
// state_topic (2)"), or "" when there are none -- so a field a source sends that Home Assistant
// would ignore is seen, not silently lost (PROJECT.md 19i).
func droppedFieldsNote(entities []THostYAMLEntity) string {
	counts := map[string]int{}
	for _, entity := range entities {
		for _, field := range entity.Dropped {
			counts[entity.Domain+" "+field]++
		}
	}
	if len(counts) == 0 {
		return ""
	}
	notes := make([]string, 0, len(counts))
	for field, count := range counts {
		notes = append(notes, fmt.Sprintf("%s (%d)", field, count))
	}
	sort.Strings(notes)
	return "; dropped, unknown to Home Assistant: " + strings.Join(notes, ", ")
}

// hostYAMLEntityBodies returns d's YAML entities: its node binary_sensor and every variable
// attribute sensor -- buildDiscoveryConfigs' (house_event_bus_coordinator/discovery.go) shape.
func hostYAMLEntityBodies(d THostDevice, link TDeviceConceptualLink, mat THostsEntityMaterialization, admin *TAdministrationState) []THostYAMLEntity {
	deviceName := link.DisplayName
	if deviceName == "" {
		deviceName = d.HostName
	}
	device := map[string]interface{}{"identifiers": []string{d.DeviceID}, "name": deviceName}
	constants := link.ConstantAttributes
	if len(constants) == 0 {
		// A host absorbed by a logical device (Logical.def "absorb from") gets a link without
		// them, nor its integration type's defaults -- only its own Physical.def lines count.
		constants = map[string]TDeviceAttributeConstant{}
		for field, attribute := range d.ConstantAttributes {
			constants[field] = TDeviceAttributeConstant{Value: attribute.Value, Forced: attribute.Forced}
		}
	}
	for field, attribute := range constants {
		if attribute.Value == "" {
			continue
		}
		device[field] = attribute.Value
		if field == "serial_number" {
			device["identifiers"] = []string{d.DeviceID, "serial:" + attribute.Value}
		}
	}

	var entities []THostYAMLEntity
	nodeTopic := mat.NodeTopic(d.HostName)
	if link.NodeEntityID != "" {
		body := map[string]interface{}{
			"unique_id":         d.DeviceID + "_node",
			"default_entity_id": link.NodeEntityID,
			"name":              conceptualEntityName(admin, link.NodeEntityID),
			"state_topic":       nodeTopic,
			"payload_on":        hostsNodePayloadOn,
			"payload_off":       hostsNodePayloadOff,
			"device":            device,
		}
		setIfNotEmpty(body, "device_class", link.NodeDeviceClass)
		setIfNotEmpty(body, "icon", link.NodeIcon)
		entities = append(entities, THostYAMLEntity{Domain: "binary_sensor", EntityID: link.NodeEntityID, Body: body})
	}

	// The same attributes devices.yaml lists (generateCoordinatorDevicesFile): the device's own
	// hosts-kind attributes, keyed by their bare leaf -- also the JSON field of the state topic.
	var leaves []string
	for _, fullName := range mat.AttributeNames(d) {
		_, leaf := splitCapabilityName(fullName)
		if _, known := link.AttributeEntityIDs[leaf]; known {
			leaves = append(leaves, leaf)
		}
	}
	sort.Strings(leaves)
	for _, leaf := range leaves {
		attribute := link.AttributeEntityIDs[leaf]
		if attribute.EntityID == "" {
			continue
		}
		name := conceptualEntityName(admin, attribute.EntityID)
		if name == nil {
			name = deviceName + "/" + leaf
		}
		body := map[string]interface{}{
			"unique_id":         d.DeviceID + "_" + leaf,
			"default_entity_id": attribute.EntityID,
			"name":              name,
			"state_topic":       mat.StateTopic(d.HostName),
			"value_template":    "{{ value_json." + leaf + " }}",
			"device":            device,
		}
		if nodeTopic != "" {
			body["availability_topic"] = nodeTopic
			body["payload_available"] = hostsNodePayloadOn
			body["payload_not_available"] = hostsNodePayloadOff
		}
		setIfNotEmpty(body, "device_class", attribute.DeviceClass)
		setIfNotEmpty(body, "unit_of_measurement", attribute.Unit)
		setIfNotEmpty(body, "state_class", attribute.StateClass)
		setIfNotEmpty(body, "icon", attribute.Icon)
		entities = append(entities, THostYAMLEntity{Domain: "sensor", EntityID: attribute.EntityID, Body: body})
	}
	return entities
}

// conceptualEntityName returns entityID's full conceptual name ("sphere/path", PROJECT.md 17h), or
// nil -- YAML null, "the device's name" -- when it has none.
func conceptualEntityName(admin *TAdministrationState, entityID string) interface{} {
	if identity, found := entityIdentityByID(admin, entityID); found && identity.Sphere != "" {
		return friendlyEntityName(identity)
	}
	return nil
}

// setIfNotEmpty sets body[key] to value unless value is empty.
func setIfNotEmpty(body map[string]interface{}, key, value string) {
	if value != "" {
		body[key] = value
	}
}
