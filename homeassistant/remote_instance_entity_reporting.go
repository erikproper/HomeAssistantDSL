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
// "unknown" for a while with no reload-triggered republish at all), and whenever any of
// bareTriggerEntities changes -- one trigger/action pair per entity/attributes topic, per
// PROJECT.md 1.1's naming. bareTriggerEntities is almost always exactly one entity (the source this
// automation reports on), but the "attributes" sibling automation (added 2026-09-21, see
// resolveHassBridgeCapabilityAttributeExprs) may need to trigger on several distinct source
// entities at once if its declared attributes come from more than one -- a plain YAML list handles
// both cases uniformly, HA's own state trigger accepts N>=1 entity_ids there identically. The
// reload trigger is the "event" platform, not "homeassistant" -- HA's own homeassistant trigger
// platform only ever supports start/shutdown, never reload; HA's automation integration fires a
// separate "automation_reloaded" event on the generic event bus instead, whenever automations.yaml
// is reloaded (UI action or the automation.reload service). topic and payloadExpr carry over
// unchanged from the shared-automation predecessor (hassBridgeReportingAutomationBody):
// homeassistant_instances/<name>/bridge/<local_entity>/state, a Jinja expression built by
// sourceToJinja2. aliasSuffix distinguishes this automation's own alias from any sibling reporting
// on the SAME identity -- "" for the ordinary state automation (unchanged alias text from before
// this parameter existed), "/attributes" for the sibling attributes automation (added 2026-09-21):
// without it, both automations would render the exact same alias text, and HA auto-suffixes the
// second one's own entity_id with "_2" on load (the identical collision-avoidance behaviour that
// bit us for real earlier this same session, netatmo battery_alert) -- confusing in the UI even
// though functionally harmless, so this avoids it outright rather than living with it.
func entityReportingAutomationBody(identity TEntityIdentity, bareTriggerEntities []string, topic, payloadExpr, aliasSuffix string) string {
	var sb strings.Builder
	sb.WriteString("- alias: \"reporting/" + fullyQualifiedEntityNameSlashed(identity) + aliasSuffix + "\"\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id:\n")
	for _, entity := range bareTriggerEntities {
		sb.WriteString("      - " + entity + "\n")
	}
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

// resolveHassBridgeCapabilityAttributeExprs resolves capability label's own declared Attributes
// (added 2026-09-21) for instance into (attribute name -> ValueMap-applied Jinja expr), plus the
// sorted, deduplicated set of bare trigger entities across all of them -- almost always exactly
// one (every attribute sourced from the SAME entity as the capability's own state, e.g.
// vacuum.roomba's "error"/"error_code"), but not assumed: an attribute MAY be declared against a
// different source entity than its own capability's state. Attributes only ever apply to an
// atomic (Sources-bearing) capability, never resolved through a "derived" chain the way
// resolveHassBridgeCapabilityExpr's own state resolution is -- a capability with
// DerivedFromCapability set is expected to declare no Attributes of its own (not validated here,
// same tolerance the rest of this file extends to declaration mistakes). ok is false when the
// capability declares no Attributes at all for this instance.
func resolveHassBridgeCapabilityAttributeExprs(device THassBridgeDevice, label, instance string) (exprsByName map[string]string, triggerEntities []string, ok bool) {
	cap, known := device.Capabilities[label]
	if !known || len(cap.Attributes) == 0 {
		return nil, nil, false
	}
	names := make([]string, 0, len(cap.Attributes))
	for name := range cap.Attributes {
		names = append(names, name)
	}
	sort.Strings(names)

	exprsByName = map[string]string{}
	triggerSet := map[string]bool{}
	for _, name := range names {
		source, declaredHere := cap.Attributes[name][instance]
		if !declaredHere {
			continue
		}
		exprsByName[name] = applyValueMap(cap.AttributeValueMaps[name], sourceToJinja2(source))
		triggerSet[bareEntityFromSource(source)] = true
	}
	if len(exprsByName) == 0 {
		return nil, nil, false
	}
	triggerEntities = make([]string, 0, len(triggerSet))
	for entity := range triggerSet {
		triggerEntities = append(triggerEntities, entity)
	}
	sort.Strings(triggerEntities)
	return exprsByName, triggerEntities, true
}

// buildAttributesPayloadExpr renders exprsByName as one Jinja dict literal -- "{'error': (...),
// 'error_code': (...)} | tojson", sorted keys for deterministic output (mirrors applyValueMap's own
// determinism convention). Unlike applyValueMap, this is a straight literal, no get()/fallback --
// there is no "unmapped value" concept for an attribute NAME the way there is for a translated
// VALUE.
func buildAttributesPayloadExpr(exprsByName map[string]string) string {
	names := make([]string, 0, len(exprsByName))
	for name := range exprsByName {
		names = append(names, name)
	}
	sort.Strings(names)
	pairs := make([]string, 0, len(names))
	for _, name := range names {
		pairs = append(pairs, jinjaStringLiteral(name)+": ("+exprsByName[name]+")")
	}
	return "{" + strings.Join(pairs, ", ") + "} | tojson"
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
			payloadExpr, triggerEntities := buildHassBridgeStatePayload(cap, name, mappedExpr, triggerEntity)
			body := entityReportingAutomationBody(attr.Identity, triggerEntities, topic, payloadExpr, "")
			// "infrastructural" regardless of the reported entity's own conceptual sphere
			// (physical/social/whatever it is) -- this automation is physical-layer plumbing
			// (how the value gets from the remote instance onto MQTT), not a conceptual placement
			// of the entity itself, same directory the coordinator_bridge_inquire/report
			// automations already use.
			automationID := "reporting_" + fullyQualifiedEntityNameUnderscored(attr.Identity)
			if err := writeAutomationFile(instanceHAOutputDir, "infrastructural", automationID, generatorHeader+body); err != nil {
				return err
			}

			// Sibling "attributes" reporting automation (added 2026-09-21): only written when this
			// capability declared >=1 "attribute <name>: ...;" for this instance -- publishes a
			// JSON object of all of them to a sibling ".../attributes" topic, which the
			// coordinator's own json_attributes_topic (discoveryhassbridge.go) then points HA's
			// discovery config at, alongside the state topic above. See PROJECT.md's
			// json-attributes-topic work, 2026-09-21: vacuum.roomba's own "error"/"error_code"
			// going unreported during a live fault was the motivating case.
			if attrExprsByName, attrTriggerEntities, attrOk := resolveHassBridgeCapabilityAttributeExprs(device, capability, name); attrOk {
				attrTopic := "homeassistant_instances/" + name + "/bridge/" + attr.EntityID + "/attributes"
				attrPayloadExpr := buildAttributesPayloadExpr(attrExprsByName)
				attrBody := entityReportingAutomationBody(attr.Identity, attrTriggerEntities, attrTopic, attrPayloadExpr, "/attributes")
				attrAutomationID := "reporting_attributes_" + fullyQualifiedEntityNameUnderscored(attr.Identity)
				if err := writeAutomationFile(instanceHAOutputDir, "infrastructural", attrAutomationID, generatorHeader+attrBody); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
