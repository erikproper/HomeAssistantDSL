/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: RemoteInstanceEntityExistence
 *
 * The inquiry half of PROJECT.md 1.1's entity-existence design (memory:
 * project_entity_existence_inquiry_design) -- one automation per instance (not per entity: HA's
 * own state machine is a dict, so a single automation parameterized by the incoming MQTT command's
 * payload can answer about *any* entity, one at a time, without iterating states at all). This is
 * deliberately the opposite shape from entitiesDetailedPayloadTemplate (remote_instance_automations.go),
 * the automation that hung "main" on 2026-08-27 by looping over every state synchronously -- this
 * one is a single states[...] dict lookup per invocation, safe at any instance size, which is what
 * lets the coordinator (house_event_bus_coordinator/entity_existence.go) safely paced-inquire about
 * main's entities too, not just protocols-server-2's.
 *
 * Also answers the design's "which entities they provide" half for known devices (not just "does
 * this one entity exist"): when the queried entity resolves to a device, the reply also carries
 * device_entities(device_id) -- every entity_id HA's own registry already associates with that
 * device. The coordinator treats any of those the tracker doesn't already know about as newly
 * discovered (known-to-exist), which is what lets the entity-existence mechanism replace the old
 * manifest/entities_detailed mechanism entirely (2026-08-28) -- discovery is now a byproduct of the
 * same paced, per-entity inquiry cycle, not a separate full-instance-scan automation.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 28.08.2026
 *
 */

package main

import "strings"

// entityExistenceInquiryAutomationBody returns the "answer one entity-existence inquiry" automation
// for a named instance -- triggered by an MQTT command on
// "homeassistant_instances/<name>/inquire" whose *payload* is the entity_id to check (not part of
// the topic, so no per-entity automation/trigger is ever needed), replies on
// "homeassistant_instances/<name>/inquire/reply" with
// {"entity_id", "exists", "state", "unit_of_measurement", "device_class", "device_id",
// "device_name", "sibling_entities"} as JSON.
// states[...] returns None for an entity_id HA has never heard of at all (as opposed to one that
// exists with state "unknown") -- exactly the existence signal this needs, and a single dict
// lookup regardless of how many entities the instance has. device_entities(device_id) is likewise
// one cheap registry lookup, not a scan -- the "which entities they provide" half of the design,
// for whichever device the queried entity happens to belong to. device_attr(did, 'name') is a
// third such lookup, used only to give a manually-bootstrapped device's suggestion grouping a
// readable name (house_event_bus_coordinator/entity_existence.go's DiscoverSiblings) -- an
// already-declared device never needs it, since its DisplayName is already known.
//
// unit_of_measurement/device_class (added 2026-09-06) piggyback on this same single lookup --
// s.attributes already carries both for free, no extra query needed -- so the coordinator can use
// a bridged entity's own remote-reported typing as a THIRD fallback tier (behind Physical.def's
// own explicit declaration and Defaults.def/code-level rules) when neither of those set anything,
// rather than requiring the DSL author to redeclare typing metadata Home Assistant's own upstream
// integration (e.g. a Fritz!Box's own gb_received sensor) already resolved on the remote end.
// Real gap found live 2026-09-06: a bridged capability with no explicit Physical.def/Defaults.def
// typing showed up with no unit/icon at all, even though the remote entity it bridges from
// genuinely has both -- this was simply never being looked at.
func entityExistenceInquiryAutomationBody(name string) string {
	var sb strings.Builder
	sb.WriteString("- alias: \"Coordinator bridge: entity existence inquiry\"\n")
	sb.WriteString("  id: automation.coordinator_bridge_inquire_" + name + "\n")
	// HA's automation default (mode: single) silently drops a new trigger arriving while a
	// previous run is still in progress, instead of queuing it -- confirmed live 2026-08-29: a
	// "Discover entity" batch fires several InquireNow calls back-to-back with no delay between
	// them, and some of that batch's own inquiries (e.g. sensor.vienna_terrace_humidity) never got
	// a reply at all, silently dropped under that default. mode: queued preserves every trigger in
	// order instead of discarding any; max is HA's own queued-mode cap (default 10), raised well
	// past this mechanism's largest realistic batch so a big manual batch can't hit it either.
	sb.WriteString("  mode: queued\n")
	sb.WriteString("  max: 50\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: mqtt\n")
	sb.WriteString("    topic: \"homeassistant_instances/" + name + "/inquire\"\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: mqtt.publish\n")
	sb.WriteString("    data:\n")
	sb.WriteString("      topic: \"homeassistant_instances/" + name + "/inquire/reply\"\n")
	sb.WriteString("      payload: >-\n")
	sb.WriteString("        {% set s = states[trigger.payload] %}{% set did = device_id(trigger.payload) if s else none %}{{ {'entity_id': trigger.payload, 'exists': s is not none, 'state': (s.state if s else none), 'unit_of_measurement': (s.attributes.get('unit_of_measurement') if s else none), 'device_class': (s.attributes.get('device_class') if s else none), 'device_id': did, 'device_name': (device_attr(did, 'name') if did else none), 'sibling_entities': (device_entities(did) if did else [])} | tojson }}\n")
	return sb.String()
}
