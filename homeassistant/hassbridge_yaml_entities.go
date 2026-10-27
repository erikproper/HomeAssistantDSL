/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: HassBridgeYAMLEntities
 *
 * PROJECT.md 19h.3 (2026-10-27): every "home_assistant" bridge device (an entity of ha2mqtt, or of
 * main itself, reported over MQTT by the generator's reporting automations) gets its entities from
 * the generator's own YAML "mqtt:" config instead of the coordinator's discovery --
 * entities/mqtt/<domain>/<entity_id>.yaml. A bridged integration (Architecture.md §6.16): its
 * topics and payloads are ours, so the declaration says nearly everything; what the source
 * entities report themselves comes from the coordinator -- their device's metadata and via_device,
 * captured into Physical.def (writeBridgeMetadata), and their typing (unit, device_class, icon),
 * cached in Definitions/.cache/hassbridge_typing.json (the declaration takes precedence).
 *
 * Each entity is built by the coordinator's own builder (hamqtt.BridgeEntityConfig), from the same
 * data homeassistant_bridge.yaml carries: the same unique_id, entity_id, topics, commands and
 * availability, so the registry entry stays. Two things differ: the name is the entity's full
 * conceptual name (PROJECT.md 17h -- the relay used a bare suffix), and the device block takes its
 * metadata from Physical.def. The coordinator publishes no local discovery (PROJECT.md 19); an
 * exported device's cloud copy stays.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 27.10.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"homeassistant/hamqtt"

	"gopkg.in/yaml.v3"
)

func hassBridgeTypingCachePath(definitionDir string) string {
	return filepath.Join(definitionDir, ".cache", "hassbridge_typing.json")
}

// fetchHassBridgeTyping returns the bridge entities' typing (local entity_id -> unit/device_class/
// icon), from a broker when one answers (refreshing the cache), else from the cache.
func fetchHassBridgeTyping(definitionDir string, ctx TPhysicalGenerationContext) (map[string]map[string]string, error) {
	var typing map[string]map[string]string
	fetched := false
	if cloudSecrets, ok := coordinatorOnlyBrokerSecrets(ctx); ok {
		fetched = fetchRetainedJSON(cloudSecrets, ctx.Installation+"/hassbridge_typing/state", "homeassistant-generator-hassbridge-typing", &typing) == nil
	}
	if !fetched {
		fetched = fetchRetainedJSON(ctx.MQTTSecrets, "hassbridge_typing/state", "homeassistant-generator-hassbridge-typing", &typing) == nil
	}
	cachePath := hassBridgeTypingCachePath(definitionDir)
	if fetched {
		if data, err := json.Marshal(typing); err == nil {
			_ = os.MkdirAll(filepath.Dir(cachePath), 0o755)
			_ = os.WriteFile(cachePath, data, 0o644)
		}
		return typing, nil
	}
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, fmt.Errorf("no broker answered and no cache at %s yet", cachePath)
	}
	return typing, json.Unmarshal(data, &typing)
}

// generateYAMLHassBridgeEntities writes the YAML "mqtt:" entities of every bridge device (see this
// file's header comment), and reports what it wrote.
func generateYAMLHassBridgeEntities(definitionDir string, devices map[string]THassBridgeDevice, ctx TPhysicalGenerationContext) []string {
	if ctx.HAOutputDir == "" {
		return nil // no Home Assistant output directory (a test): nothing to write, never relative to the working directory
	}
	if len(devices) == 0 {
		return nil
	}
	typing, err := fetchHassBridgeTyping(definitionDir, ctx)
	if err != nil {
		typing = map[string]map[string]string{}
		fmt.Printf("[physical] bridge entities: %v -- no live typing (unit, device_class, icon) beyond the declaration\n", err)
	}
	deviceIDs := make([]string, 0, len(devices))
	for id := range devices {
		deviceIDs = append(deviceIDs, id)
	}
	sort.Strings(deviceIDs)
	var reports []string
	for _, deviceID := range deviceIDs {
		entities, problem := hassBridgeYAMLEntityBodies(deviceID, devices[deviceID], ctx.Admin, typing)
		if problem != "" {
			reports = append(reports, fmt.Sprintf("%s: ERROR, %s", deviceID, problem))
			continue
		}
		if len(entities) == 0 {
			continue
		}
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
		reports = append(reports, fmt.Sprintf("%s: %d entities via YAML", deviceID, written)+droppedFieldsNote(entities))
	}
	return reports
}

// hassBridgeYAMLEntityBodies returns deviceID's YAML entities, built from exactly what
// generateHassBridgeFile writes for the coordinator; problem says why it has none.
func hassBridgeYAMLEntityBodies(deviceID string, device THassBridgeDevice, admin *TAdministrationState, typing map[string]map[string]string) (entities []THostYAMLEntity, problem string) {
	link, hasLink := admin.DeviceConceptualLinks[deviceID]
	if !hasLink {
		return nil, ""
	}
	if len(device.Instances) != 1 {
		// Today none has more: the relay followed whichever instance reported; YAML can't.
		return nil, fmt.Sprintf("%d instances -- a YAML entity has one state topic", len(device.Instances))
	}
	instance := device.Instances[0]

	deviceName := link.DisplayName
	if deviceName == "" {
		deviceName = deviceID
	}
	block := map[string]interface{}{"identifiers": []string{deviceID}, "name": deviceName}
	for field, attribute := range link.ConstantAttributes {
		if attribute.Value == "" {
			continue
		}
		if field == "via_device" {
			identifier, known := haDeviceIdentifier(admin, attribute.Value)
			if !known {
				fmt.Printf("[physical] %s: via_device %s isn't a declared, positioned device -- left out\n", deviceID, attribute.Value)
				continue
			}
			block[field] = identifier
			continue
		}
		block[field] = attribute.Value
	}

	// The device's own node, ANDed into every other capability's availability (the relay's
	// hassBridgeAvailabilityTopics), and every "dependency on" node topic.
	nodeTopic := ""
	if node, resolved := link.AttributeEntityIDs["node"]; resolved {
		if _, declared := device.Capabilities["node"]; declared && node.EntityID != "" {
			nodeTopic = hamqtt.BridgeStateTopic(instance, node.EntityID)
		}
	}

	names := make([]string, 0, len(device.Capabilities))
	for name := range device.Capabilities {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		attribute, resolved := link.AttributeEntityIDs[name]
		if !resolved || attribute.EntityID == "" {
			continue
		}
		capability := device.Capabilities[name]
		localEntity := attribute.EntityID
		var nodeTopics []string
		if name != "node" && nodeTopic != "" {
			nodeTopics = append(nodeTopics, nodeTopic)
		}
		nodeTopics = append(nodeTopics, device.DependsOnAvailabilityTopics...)

		var commands map[string]hamqtt.Command
		var extra map[string]interface{}
		if specs, commandable := domainCommands[capability.Domain]; commandable {
			commands = map[string]hamqtt.Command{}
			for _, spec := range specs {
				command := hamqtt.Command{Payload: spec.Payload, DiscoveryKey: spec.DiscoveryKey, ValueTrigger: spec.ValueTrigger, TopicKey: spec.TopicKey}
				if spec.ValueTrigger {
					command.DataKey = commandDataKey(spec)
				}
				commands[spec.Name] = command
			}
			extra = domainDiscoveryExtras[capability.Domain]
		}

		displaySuffix := attribute.DisplaySuffix
		if identity, found := entityIdentityByID(admin, localEntity); found && identity.Sphere != "" {
			displaySuffix = friendlyEntityName(identity)
		}
		live := typing[localEntity]
		body := hamqtt.BridgeEntityConfig(hamqtt.BridgeEntity{
			UniqueID:      hamqtt.BridgeUniqueID(localEntity, admin.FrozenUniqueIDs["physical:"+deviceID+"!"+name]),
			LocalEntity:   localEntity,
			Capability:    name,
			DisplaySuffix: displaySuffix,
			DeviceClass:   attribute.DeviceClass, Unit: attribute.Unit, StateClass: attribute.StateClass, Icon: attribute.Icon,
			LiveUnit: live["unit"], LiveDeviceClass: live["device_class"], LiveIcon: live["icon"],
			Commands: commands, DiscoveryExtra: extra,
			HasAttributes: len(capability.Attributes) > 0, HasPosition: len(capability.Position) > 0,
			Instance:   instance,
			StateTopic: hamqtt.BridgeStateTopic(instance, localEntity), AttributesTopic: hamqtt.BridgeAttributesTopic(instance, localEntity),
			NodeAvailabilityTopics: nodeTopics, CommandsEnabled: true, Device: block,
		})
		// Only the fields Home Assistant knows for the domain, as its discovery keeps them (e.g. the
		// relay's state_topic on a button): one unknown field makes the whole MQTT reload fail
		// (found live 2026-10-27, Vienna's Fritz!Box buttons).
		domain := localEntity[:strings.Index(localEntity, ".")]
		entities = append(entities, discoveredYAMLEntity(domain, localEntity, body))
	}
	return entities, ""
}

// discoveryDeviceIdentifiers is every discovery device's Home Assistant identifier (its first
// "identifiers" value), by its declared id -- set by generatePhysicalIntegrationOutputs.
var discoveryDeviceIdentifiers = map[string]string{}

// haDeviceIdentifier is the Home Assistant identifier a device reference ("via_device:
// utility.envoy;") stands for: a discovery device's own first identifier, every other device's
// declared id (its YAML device block's identifier); known reports whether ref is a declared,
// positioned device.
func haDeviceIdentifier(admin *TAdministrationState, ref string) (identifier string, known bool) {
	if identifier, isDiscovery := discoveryDeviceIdentifiers[ref]; isDiscovery {
		return identifier, true
	}
	if _, positioned := admin.DeviceConceptualLinks[ref]; positioned {
		return ref, true
	}
	return "", false
}
