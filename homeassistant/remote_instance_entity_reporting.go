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
 * Command automations (the other half of PROJECT.md item 11's per-entity naming convention,
 * command_<fully_qualified_entity_name>_<command>.yaml) are built in the sibling file
 * remote_instance_entity_commands.go, not here -- this file only covers state reporting.
 *
 * State reporting itself gained one domain-specific wrinkle 2026-09-09: most domains report an
 * atomic (bare-string) payload, all sourceToJinja2 has ever produced, but "vacuum" needs its
 * state_topic payload to already be a JSON object (hassbridge_commands.go's
 * domainStatePayloadTemplate/applyStatePayloadTemplate) -- confirmed against the live MQTT vacuum
 * component's own _state_message_received, which json-parses the raw payload before anything
 * else, with no value_template hook to reshape a bare string at the discovery-config level.
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

// applyValueMap renders expr's own raw value through capability's Physical.def-declared
// ValueMap, when one was declared -- a DEVICE-specific translation (this exact Roomba's own
// native vocabulary), not a domain-generic rule the way hassbridge_commands.go's
// domainCommands/domainStatePayloadTemplate are (those apply to every device of a domain; a
// ValueMap only ever applies to the one capability instance that declared it). ".get(expr,
// expr)" passes any value with no entry straight through unchanged -- a translation table, not
// an enum whitelist, so an unexpected/new native value never breaks generation. expr is
// evaluated twice in the rendered Jinja (once as the lookup key, once as the fallback default);
// negligible cost for a single states() read, and the simplest correct way to express
// "translate-or-pass-through" as one Jinja expression fragment without a separate {% set %}.
func applyValueMap(valueMap map[string]string, expr string) string {
	if len(valueMap) == 0 {
		return expr
	}
	keys := make([]string, 0, len(valueMap))
	for key := range valueMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, jinjaStringLiteral(key)+": "+jinjaStringLiteral(valueMap[key]))
	}
	return "{" + strings.Join(pairs, ", ") + "}.get(" + expr + ", " + expr + ")"
}

// resolveHassBridgeCapabilityExpr resolves capability label's own value, as reported by instance,
// into a Jinja expression plus the bare HA entity_id whose state changing should trigger
// republishing. For an ordinary (atomic) capability this is exactly what it always was --
// applyValueMap(sourceToJinja2(Source)) and bareEntityFromSource(Source), both per-instance. For a
// "derived" capability (DerivedFromCapability/DerivedViaTemplate,
// Physical_DerivedCapability.go/plans/derived-capability-mechanism.md Phase 2), there is no
// Source of its own to read -- instead this recursively resolves the SAME-DEVICE sibling capability
// named by DerivedFromCapability (which may itself be derived, composing further) and substitutes
// its resolved expression into DerivedViaTemplate; the trigger entity is always the ultimate
// atomic base's own entity, however many derivation steps deep, since that's the one whose real HA
// state changes drive every capability derived from it. ok is false when the chain can't be
// resolved (an unknown label, no Source declared for this instance anywhere along the chain, or a
// cycle -- validateDerivedCapabilities already warns about a bad chain at collection time; visited
// is this function's own defence in depth against acting on one anyway).
func resolveHassBridgeCapabilityExpr(device THassBridgeDevice, label, instance string, visited map[string]bool) (expr, triggerEntity string, ok bool) {
	if visited[label] {
		return "", "", false
	}
	visited[label] = true

	cap, known := device.Capabilities[label]
	if !known {
		return "", "", false
	}

	if cap.DerivedFromCapability == "" {
		source, declaredHere := cap.Sources[instance]
		if !declaredHere {
			return "", "", false
		}
		return applyValueMap(cap.ValueMap, sourceToJinja2(source)), bareEntityFromSource(source), true
	}

	baseExpr, baseTriggerEntity, resolved := resolveHassBridgeCapabilityExpr(device, cap.DerivedFromCapability, instance, visited)
	if !resolved {
		return "", "", false
	}
	return substituteDerivedTemplate(cap.DerivedViaTemplate, baseExpr), baseTriggerEntity, true
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
			// capability whose resolution chain (itself, for an atomic one; its ultimate derived
			// base, for a "derived" one) never declared a source for this specific instance is
			// skipped entirely -- not every instance necessarily reports every capability.
			mappedExpr, triggerEntity, resolved := resolveHassBridgeCapabilityExpr(device, capability, name, map[string]bool{})
			if !resolved {
				continue
			}
			topic := "homeassistant_instances/" + name + "/bridge/" + attr.EntityID + "/state"
			payloadExpr := applyStatePayloadTemplate(cap.Domain, mappedExpr)
			body := entityReportingAutomationBody(attr.Identity, triggerEntity, topic, payloadExpr)
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
