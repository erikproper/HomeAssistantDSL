/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: RemoteInstanceEntityCommands
 *
 * Per-entity-per-command automations for home_assistant-bridge-sourced entities -- the command
 * half of PROJECT.md item 11's naming convention (command_<fully_qualified_entity_name>_
 * <command>.yaml, alias command/<fully_qualified_entity_name_with_slashes>/<command>), left open
 * since 2026-08-28 pending exactly the domain/command-mapping scoping hassbridge_commands.go now
 * provides. Sibling to remote_instance_entity_reporting.go (state reporting) -- this file only
 * covers commands.
 *
 * A capability whose domain (THassBridgeCapability.Domain) has an entry in domainCommands
 * (hassbridge_commands.go) gets one automation generated per command, deployed onto the remote
 * instance that actually owns the entity. All of one entity's commands share a single MQTT topic
 * (homeassistant_instances/<name>/bridge/<local_entity>/command) -- matching how HA's own MQTT
 * switch/vacuum schemas work: one command_topic per entity, each command discriminated purely by
 * matching the incoming payload against a configured literal (payload_on/payload_start/...), not
 * by separate topics per command. Each generated automation triggers on that shared topic with an
 * exact-payload filter and calls its own single hardcoded service on the remote entity_id -- no
 * Jinja dispatch, no argument parsing.
 *
 * house_event_bus_coordinator only ever authors the discovery config once (command_topic plus
 * each domainCommands entry's own DiscoveryKey: Payload pair, house_event_bus_coordinator/
 * discoveryhassbridge.go's buildHassBridgeEntityDiscoveryBody) -- after that, HA's own MQTT
 * integration on the consuming instance publishes directly to this shared topic and whichever of
 * these automations matches picks it up. Zero coordinator runtime relay for the same-broker case,
 * exactly like state reporting's own "author once, then get out of the way" shape.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 09.09.2026
 *
 */

package main

import (
	"sort"
	"strings"
)

// hassBridgeEntityCommandTopic is the shared command topic every FIXED-payload command a bridged
// entity accepts publishes/triggers on -- must stay in lock-step with house_event_bus_coordinator/
// discoveryhassbridge.go's own hassBridgeEntityCommandTopic (coordinator side), which sets a
// discovery config's command_topic to this exact same string.
func hassBridgeEntityCommandTopic(instance, localEntity string) string {
	return "homeassistant_instances/" + instance + "/bridge/" + localEntity + "/command"
}

// hassBridgeEntityCommandTopicFor is the topic ONE specific command actually triggers on: the
// shared base topic for a fixed-payload command (unchanged), or a DEDICATED topic
// (base + "/" + command.Name) for a ValueTrigger command (2026-09-25, added alongside cover's own
// set_position) -- a ValueTrigger command's own trigger matches ANY payload, so sharing the base
// topic with fixed-payload siblings on the same domain (e.g. cover's open/close/stop) would make
// their own "OPEN"/"CLOSE"/"STOP" payloads ALSO match the value-trigger's "any payload" filter,
// firing both automations ambiguously. Must stay in lock-step with discoveryhassbridge.go's own
// identically-named function (coordinator side), which computes set_position_topic/etc. the same
// way.
func hassBridgeEntityCommandTopicFor(instance, localEntity string, command commandSpec) string {
	base := hassBridgeEntityCommandTopic(instance, localEntity)
	if command.ValueTrigger {
		return base + "/" + command.Name
	}
	return base
}

// commandDataKey is command.DataKey, defaulting to "value" when unset -- every ValueTrigger
// command before cover's own set_position used exactly that name (number.set_value's own "value"
// parameter); set_position needs "position" instead (cover.set_cover_position's own parameter).
func commandDataKey(command commandSpec) string {
	if command.DataKey != "" {
		return command.DataKey
	}
	return "value"
}

// entityCommandAutomationBody is the "act on one command for one entity" automation: triggers on
// topic with an exact-match payload filter (HA's own MQTT trigger platform's "payload:" key), and
// calls service on bareTargetEntity -- the remote instance's own real entity_id, never the local
// (bridged) one. No data/parameters: every fixed-payload command (switch/vacuum/cover/button)
// carries no value with the command at all, matching HA's own MQTT schemas for those domains.
//
// command.ValueTrigger (2026-09-25, "number"/cover's own "set_position") is the one exception: no
// "payload:" filter at all (any message on this command's own dedicated topic -- see
// hassBridgeEntityCommandTopicFor -- is this entity's own newly-committed value, not one of
// several discriminated commands), and the service call passes it through as commandDataKey's own
// named data field rather than a bare no-data call.
func entityCommandAutomationBody(identity TEntityIdentity, command commandSpec, bareTargetEntity, topic string) string {
	var sb strings.Builder
	sb.WriteString("- alias: \"command/" + fullyQualifiedEntityNameSlashed(identity) + "/" + command.Name + "\"\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: mqtt\n")
	sb.WriteString("    topic: \"" + topic + "\"\n")
	if !command.ValueTrigger {
		sb.WriteString("    payload: \"" + command.Payload + "\"\n")
	}
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: " + command.Service + "\n")
	if command.ValueTrigger {
		sb.WriteString("    target:\n")
		sb.WriteString("      entity_id: " + bareTargetEntity + "\n")
		sb.WriteString("    data:\n")
		sb.WriteString("      " + commandDataKey(command) + ": \"{{ trigger.payload }}\"\n")
	} else {
		sb.WriteString("    entity_id: " + bareTargetEntity + "\n")
	}
	return sb.String()
}

// generateHassBridgeEntityCommandAutomations writes one automation file per command, for every
// positioned hassbridge capability bridged from the named instance whose domain is commandable
// (domainCommands) -- same "must be positioned" filter generateHassBridgeEntityReportingAutomations
// already applies (an unpositioned bridge device has nothing to command yet either). Deterministic
// order: devices, then capabilities, then commands, all sorted.
func generateHassBridgeEntityCommandAutomations(instanceHAOutputDir, name string, hassBridgeDevicesByID map[string]THassBridgeDevice, admin *TAdministrationState) error {
	deviceIDs := make([]string, 0, len(hassBridgeDevicesByID))
	for deviceID, device := range hassBridgeDevicesByID {
		if containsString(device.Instances, name) {
			deviceIDs = append(deviceIDs, deviceID)
		}
	}
	sort.Strings(deviceIDs)

	for _, deviceID := range deviceIDs {
		device := hassBridgeDevicesByID[deviceID]
		link, hasLink := admin.DeviceConceptualLinks[deviceID]
		if !hasLink {
			continue
		}

		capabilityNames := make([]string, 0, len(device.Capabilities))
		for capability := range device.Capabilities {
			capabilityNames = append(capabilityNames, capability)
		}
		sort.Strings(capabilityNames)

		for _, capability := range capabilityNames {
			attr, resolved := link.AttributeEntityIDs[capability]
			if !resolved {
				continue
			}
			cap := device.Capabilities[capability]
			commands, commandable := domainCommands[cap.Domain]
			if !commandable {
				continue
			}
			source, declaredHere := cap.Sources[name]
			if !declaredHere {
				continue
			}
			bareTargetEntity := bareEntityFromSource(source)

			for _, command := range commands {
				topic := hassBridgeEntityCommandTopicFor(name, attr.EntityID, command)
				body := entityCommandAutomationBody(attr.Identity, command, bareTargetEntity, topic)
				automationID := "command_" + fullyQualifiedEntityNameUnderscored(attr.Identity) + "_" + command.Name
				if err := writeAutomationFile(instanceHAOutputDir, "infrastructural", automationID, generatorHeader+body); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
