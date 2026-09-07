/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: RemoteInstanceEntityReporting
 *
 * Per-entity state-reporting automations for home_assistant-bridge-sourced entities -- PROJECT.md
 * 1.1's "entity-existence inquiry & optimistic generation" design (2026-08-28, memory:
 * project_entity_existence_inquiry_design). Generation is unconditional -- optimistic per that
 * design: a capability is reported on as soon as it's positioned, whether or not the coordinator
 * has yet confirmed its source entity actually exists. Replaces the shared, one-automation-per-
 * instance capability reporting hassBridgeReportingAutomationBody used to also do; device-info
 * reporting stays combined per-instance there (this file only covers individual entities).
 *
 * Command automations (the other half of PROJECT.md 1.1's per-entity naming convention,
 * command_<fully_qualified_entity_name>_<command>.yaml) are deliberately not built here yet --
 * there is no existing notion anywhere in this codebase of which commands a capability/domain
 * supports or how one maps to a remote service call; that needs its own scoping before it can be
 * generated, not a guessed-at shape.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 28.08.2026
 *
 */

package main

import (
	"sort"
	"strings"
)

// fullyQualifiedEntitySegments returns identity's Domain, Sphere, and each "/"-split Path segment,
// in order -- the one lossless source both fullyQualifiedEntityNameUnderscored (filenames) and
// fullyQualifiedEntityNameSlashed (HA automation aliases) are built from. Reconstructing this from
// an already-flattened HA entity_id (toHomeAssistantEntityID's underscore-joined object_id) isn't
// possible -- segment boundaries and underscores that were already part of a segment's own name
// (e.g. "wi_fi_strength") become indistinguishable once joined, so this must be built from the
// pre-flattening TEntityIdentity every time (TDeviceAttributeLink.Identity carries it for exactly
// this reason).
func fullyQualifiedEntitySegments(identity TEntityIdentity) []string {
	segments := []string{identity.Domain, identity.Sphere}
	for _, part := range strings.Split(identity.Path, "/") {
		if part != "" {
			segments = append(segments, part)
		}
	}
	return segments
}

// fullyQualifiedEntityNameUnderscored is the filename-safe form, e.g.
// "sensor_infrastructural_netatmo_co2" -- passed as the generator's own automationID (see
// writeAutomationFile), which already wraps it as "automation.<id>.yaml".
func fullyQualifiedEntityNameUnderscored(identity TEntityIdentity) string {
	return strings.Join(fullyQualifiedEntitySegments(identity), "_")
}

// fullyQualifiedEntityNameSlashed is the HA-friendly-name form, e.g.
// "sensor/infrastructural/netatmo/co2" -- used as the generated automation's own alias: (HA's own
// notion of "friendly name" -- confirmed with the user 2026-08-28 this is what PROJECT.md 1.1
// means by "friendly name", not an MQTT topic segment).
func fullyQualifiedEntityNameSlashed(identity TEntityIdentity) string {
	return strings.Join(fullyQualifiedEntitySegments(identity), "/")
}

// entityReportingAutomationBody is the "report this one entity's state" automation: fires at HA
// startup (so a fresh restart republishes current state), on every automations reload (so a plain
// "reload automations" -- far more common than a full restart, and the only thing a deploy of this
// very automation triggers -- also republishes current state immediately, rather than leaving the
// entity stuck on whatever it last reported until either a real restart or the next incidental
// source state-change; confirmed live 2026-08-29: a newly-redeployed reporting automation sat on
// "unknown" for a while with no reload-triggered republish at all), and whenever the underlying
// source entity changes -- one trigger/action pair, one entity, per PROJECT.md 1.1's naming. The
// reload trigger is the "event" platform, not "homeassistant" -- HA's own homeassistant trigger
// platform only ever supports start/shutdown, never reload; HA's automation integration fires a
// separate "automation_reloaded" event on the generic event bus instead, whenever automations.yaml
// is reloaded (UI action or the automation.reload service). topic and payloadExpr carry over
// unchanged from the shared-automation predecessor (hassBridgeReportingAutomationBody):
// homeassistant_instances/<name>/bridge/<local_entity>/state, a Jinja expression built by
// sourceToJinja2.
func entityReportingAutomationBody(identity TEntityIdentity, bareTriggerEntity, topic, payloadExpr string) string {
	var sb strings.Builder
	sb.WriteString("- alias: \"reporting/" + fullyQualifiedEntityNameSlashed(identity) + "\"\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + bareTriggerEntity + "\n")
	sb.WriteString("  - platform: homeassistant\n")
	sb.WriteString("    event: start\n")
	sb.WriteString("  - platform: event\n")
	sb.WriteString("    event_type: automation_reloaded\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: mqtt.publish\n")
	sb.WriteString("    data:\n")
	sb.WriteString("      topic: \"" + topic + "\"\n")
	sb.WriteString("      retain: true\n")
	sb.WriteString("      payload: \"{{ " + payloadExpr + " }}\"\n")
	return sb.String()
}

// generateHassBridgeEntityReportingAutomations writes one reporting automation file per positioned
// hassbridge capability bridged from the named instance -- same "must be positioned" filter
// collectHassBridgeCapabilitiesByInstance already applies elsewhere (an unpositioned bridge device
// publishes nothing worth reporting). Deterministic order: devices then capabilities, sorted.
func generateHassBridgeEntityReportingAutomations(instanceHAOutputDir, name string, hassBridgeDevicesByID map[string]THassBridgeDevice, admin *TAdministrationState) error {
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
			// Per-instance, never a shared value: a roaming device's own local entity_id for the
			// same capability can genuinely differ across instances (found live 2026-09-05). A
			// capability this specific instance never declared a source for is skipped entirely --
			// not every instance necessarily reports every capability.
			source, declaredHere := cap.Sources[name]
			if !declaredHere {
				continue
			}
			topic := "homeassistant_instances/" + name + "/bridge/" + attr.EntityID + "/state"
			body := entityReportingAutomationBody(attr.Identity, bareEntityFromSource(source), topic, sourceToJinja2(source))
			// "infrastructural" regardless of the reported entity's own conceptual sphere
			// (physical/social/whatever it is) -- this automation is physical-layer plumbing
			// (how the value gets from the remote instance onto MQTT), not a conceptual placement
			// of the entity itself, same directory the coordinator_bridge_inquire/report
			// automations already use.
			automationID := "reporting_" + fullyQualifiedEntityNameUnderscored(attr.Identity)
			if err := writeAutomationFile(instanceHAOutputDir, "infrastructural", automationID, generatorHeader+body); err != nil {
				return err
			}
		}
	}
	return nil
}
