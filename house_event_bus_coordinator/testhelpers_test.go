package main

// Wrappers the tests still call by their old names -- the builders and helpers themselves moved to
// homeassistant/hamqtt (PROJECT.md 19, 2026-10-27), shared with the generator's YAML entities; the
// coordinator no longer publishes these configs to Home Assistant. The tests keep recording the
// rules and the live bugs behind them.

import (
	"encoding/json"
	"strings"

	"homeassistant/hamqtt"
)

func wrapMQTTValueTemplate(rawTemplate, wrapExpr string) string {
	return hamqtt.WrapValueTemplate(rawTemplate, wrapExpr)
}

func applyProxiedBinarySensorPayload(body map[string]interface{}, localEntity string) {
	if strings.HasPrefix(localEntity, "binary_sensor.") {
		body["payload_on"], body["payload_off"] = hamqtt.StateOn, hamqtt.StateOff
	}
}

func buildAvailabilityFields(body map[string]interface{}, stateTopic string, nodeTopics ...string) {
	body["availability"] = availabilityEntriesFrom(hamqtt.ProxiedAvailability(stateTopic, nodeTopics...))
	body["availability_mode"] = "all"
}

var discoveryAbbreviatedKeyPairs = hamqtt.AbbreviatedKeyPairs

// buildRelayedDiscoveryConfig is the relayed config the coordinator once published for a declared
// discovery device's entity -- now hamqtt.RelayedConfig, which the generator's YAML uses.
func buildRelayedDiscoveryConfig(entityID, gatewayID string, rawPayload map[string]interface{}, payload tDecodedDiscoveryPayload, link hamqtt.Link, prefix, installation, rawDomain string, soleGatewayCapability bool) (topic string, body map[string]interface{}, ok bool) {
	body, ok = hamqtt.RelayedConfig(entityID, gatewayID, rawPayload, hamqtt.DecodedPayload(payload), link, rawDomain, soleGatewayCapability)
	if !ok {
		return "", nil, false
	}
	body["origin"] = coordinatorOriginMap(installation)
	return discoveryTopic(prefix, entityID[:strings.Index(entityID, ".")], body["unique_id"].(string)), body, true
}

// localBridgeConfigs is, for each "<instance>|<local entity>" message, the discovery config the
// coordinator once published to Home Assistant for a bridge entity -- its device block (store and
// via_device), the source entity's live typing and the shared builder: exactly what the generator's
// YAML is built from now (PROJECT.md 19h.3). The tests keep checking that combination.
func localBridgeConfigs(bridgeFile THassBridgeFile, store *TLiveDeviceInfoStore, tracker *TEntityExistenceTracker, messages ...string) []recordedPublish {
	byLocal := hassBridgeCapabilitiesByLocalEntity(bridgeFile)
	var published []recordedPublish
	for _, message := range messages {
		instance, localEntity, _ := strings.Cut(message, "|")
		found, known := byLocal[localEntity]
		if !known {
			continue
		}
		device := bridgeFile.Devices[found.DeviceID]
		viaDeviceID, liveUnit, liveDeviceClass, liveIcon := "", "", "", ""
		if tracker != nil {
			viaDeviceID, _ = tracker.ResolveViaDevice(instance, found.DeviceID, hassBridgeOwnSourceEntities(device, instance))
			if source, ok := found.SourceEntities[instance]; ok {
				liveUnit, liveDeviceClass, liveIcon = tracker.LiveTyping(instance, source)
			}
		}
		devBlock := buildHassBridgeDeviceBlock(found.DeviceID, device, store.Snapshot(found.DeviceID), viaDeviceID)
		body := buildHassBridgeEntityDiscoveryBody(hassBridgeUniqueID(localEntity, found.StableID), localEntity, found, hassBridgeEntityStateTopic(instance, localEntity), hassBridgeEntityAttributesTopic(instance, localEntity), devBlock, hassBridgeAvailabilityTopics(device, found.Capability), liveUnit, liveDeviceClass, liveIcon, instance, true, "")
		topic, _ := hassBridgeDiscoveryTopic(localEntity, found.StableID, testPrefix)
		data, _ := json.Marshal(body)
		published = append(published, recordedPublish{topic: topic, payload: data})
	}
	return published
}

// buildImportedDiscoveryBody and buildDerivedImportedDiscoveryBody are the configs the coordinator
// once published for an imported entity -- now hamqtt.ImportedEntityConfig and
// hamqtt.DerivedImportedEntityConfig, which the generator's YAML uses (PROJECT.md 19h.4).
func buildImportedDiscoveryBody(match importCapabilityMatch, payload importedDiscoveryPayload, nodeAvailabilityTopics []string, remoteInstallation string, viaDeviceIndex map[importRemoteDeviceKey]string, installation string) map[string]interface{} {
	deviceName := match.DisplayName
	if deviceName == "" {
		deviceName = match.LocalDeviceID
	}
	body := hamqtt.ImportedEntityConfig(hamqtt.ImportedEntity{
		UniqueID: importedUniqueID(match.LocalEntity, match.StableID), LocalEntity: match.LocalEntity,
		LocalDeviceID: match.LocalDeviceID, DeviceName: deviceName, Capability: match.Capability,
		SuggestedArea: match.SuggestedArea, Remote: importedRemote(payload, remoteInstallation, viaDeviceIndex),
		NodeAvailabilityTopics: nodeAvailabilityTopics,
	})
	body["availability"] = availabilityEntriesFrom(body["availability"])
	body["origin"] = coordinatorOriginMap(installation)
	return body
}

func buildDerivedImportedDiscoveryBody(localDeviceID string, device TImportedDevice, capability string, cap TImportedCapability, baseTopic, template string, nodeAvailabilityTopics []string, installation string) map[string]interface{} {
	deviceName := device.DisplayName
	if deviceName == "" {
		deviceName = localDeviceID
	}
	body := hamqtt.DerivedImportedEntityConfig(hamqtt.DerivedImportedEntity{
		UniqueID: importedUniqueID(cap.LocalEntity, cap.StableID), LocalEntity: cap.LocalEntity,
		LocalDeviceID: localDeviceID, DeviceName: deviceName, Capability: capability,
		SuggestedArea: device.ConstantAttributes["suggested_area"].Value,
		BaseTopic:     baseTopic, Template: template, NodeAvailabilityTopics: nodeAvailabilityTopics,
	})
	body["availability"] = availabilityEntriesFrom(body["availability"])
	body["origin"] = coordinatorOriginMap(installation)
	return body
}

func importedUniqueID(localEntity, stableID string) string {
	return hamqtt.ImportedUniqueID(localEntity, stableID)
}

// importedAvailabilityTopics is an imported entity's node and "dependency on" topics.
func importedAvailabilityTopics(importFile TImportedFile, match importCapabilityMatch) []string {
	device, ok := importFile.Devices[match.LocalDeviceID]
	if !ok {
		return nil
	}
	var topics []string
	if node, ok := device.Capabilities["node"]; ok && node.LocalEntity != "" && match.Capability != "node" {
		topics = append(topics, importedStateTopic(node.LocalEntity))
	}
	return append(topics, device.DependsOnAvailability...)
}
