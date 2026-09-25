/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: HassBridgeCommands
 *
 * The domain -> commands table for hassbridge command routing (PROJECT.md item 11's own
 * "which commands a capability/domain supports, and how one maps to a remote service call on the
 * bridged instance" question, left open since 2026-08-28). Lives once, here, in the generator --
 * house_event_bus_coordinator stays domain-agnostic, reading only the fully-resolved commands/
 * discovery_extra/state_payload_template this file's tables produce, already baked into
 * coordinator/homeassistant_bridge.yaml by integration_hassbridge_generator.go.
 *
 * A hassbridge capability whose domain (the part of its "<domain>.<label>" key before the dot,
 * THassBridgeCapability.Domain) has an entry here automatically gets command generation too --
 * no new Physical.def syntax, pure opt-in-via-declaration (the same principle already used to
 * reject a "forced" override keyword for the battery_level/battery_alert biconditional: a device
 * author who didn't want a domain's commands simply wouldn't declare a capability in that domain).
 * Read-only domains (sensor, binary_sensor, ...) simply aren't in these tables, so nothing changes
 * for them.
 *
 * Field values below are taken from the actual MQTT integration source running live
 * (homeassistant/components/mqtt/switch.py, .../vacuum.py, checked against the real container
 * 2026-09-09), not from memory -- https://www.home-assistant.io/integrations/vacuum.mqtt/ is HA's
 * own docs page for the vacuum schema, confirmed to match.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 09.09.2026
 *
 */

package main

import "strings"

// commandSpec is one command a domain supports: Name identifies it in the generated per-command
// automation's filename/alias (PROJECT.md item 11's own naming convention,
// "command_<fully_qualified_entity_name>_<command>.yaml"); Service is the HA service the remote
// instance's own automation calls on its local entity_id; Payload is the literal string this
// command is recognised by on the entity's single shared command_topic; DiscoveryKey is the
// discovery-config field Payload gets written into (e.g. "payload_on", "payload_start") -- HA's
// MQTT switch/vacuum schemas discriminate every command off one shared command_topic purely by
// matching the incoming payload against each of these configured literals, not by separate topics
// per command.
//
// ValueTrigger (2026-09-25, added for "number") marks a command whose own payload is NOT a fixed
// literal (Payload/DiscoveryKey both left empty) but the actual value the user set -- HA's MQTT
// number schema publishes the committed value verbatim to command_topic, with no payload_* key to
// configure at all. The generated automation triggers on ANY message on its own DEDICATED topic
// (never the shared command_topic other fixed-payload commands on the same domain use -- a
// same-topic "any payload" trigger would collide with THEIR own fixed-string triggers, e.g.
// cover's own set_position sharing a topic with open/close/stop) and passes the message straight
// through as the service call's own data field, instead of the bare no-data call every
// fixed-payload command uses (remote_instance_entity_commands.go's entityCommandAutomationBody).
//
// DataKey (2026-09-25, added alongside cover's own "set_position") names that data field --
// "value" for number.set_value, "position" for cover.set_cover_position -- defaulting to "value"
// when empty (every ValueTrigger command before cover's own used exactly that name).
//
// TopicKey (2026-09-25, added alongside cover's own "set_position") is the discovery-config field
// this command's own topic (hassBridgeEntityCommandTopicFor) gets written into -- "command_topic"
// for every command before cover's own set_position (defaults to that when empty), "set_position_topic"
// for set_position: MQTT cover.py keeps position-setting on its own dedicated field, wholly
// separate from the shared command_topic open/close/stop use.
type commandSpec struct {
	Name         string
	Service      string
	Payload      string
	DiscoveryKey string
	ValueTrigger bool
	DataKey      string
	TopicKey     string
}

// domainCommands is the whole table. Adding a domain is one new entry here, plus (for anything
// shaped like "cover"/"button" -- a fixed set of discrete commands) nothing else; a genuinely
// value-carrying domain like "number" also needs its own ValueTrigger handling verified in
// remote_instance_entity_commands.go and house_event_bus_coordinator/discoveryhassbridge.go (both
// already cope, see their own doc comments).
var domainCommands = map[string][]commandSpec{
	"switch": {
		{Name: "turn_on", Service: "switch.turn_on", Payload: "ON", DiscoveryKey: "payload_on"},
		{Name: "turn_off", Service: "switch.turn_off", Payload: "OFF", DiscoveryKey: "payload_off"},
	},
	"vacuum": {
		{Name: "start", Service: "vacuum.start", Payload: "start", DiscoveryKey: "payload_start"},
		{Name: "pause", Service: "vacuum.pause", Payload: "pause", DiscoveryKey: "payload_pause"},
		{Name: "stop", Service: "vacuum.stop", Payload: "stop", DiscoveryKey: "payload_stop"},
		{Name: "return_to_base", Service: "vacuum.return_to_base", Payload: "return_to_base", DiscoveryKey: "payload_return_to_base"},
		{Name: "locate", Service: "vacuum.locate", Payload: "locate", DiscoveryKey: "payload_locate"},
		{Name: "clean_spot", Service: "vacuum.clean_spot", Payload: "clean_spot", DiscoveryKey: "payload_clean_spot"},
	},
	// cover/button/number (2026-09-25, PROJECT.md item 8's own Overkiz/Somfy migration): cover and
	// button both fit the existing fixed-payload model exactly (HA's real MQTT schema defaults,
	// confirmed against homeassistant/components/mqtt/cover.py and .../button.py); number is the
	// first ValueTrigger case (see commandSpec's own doc comment) -- MQTT number.py's command_topic
	// carries the raw committed value, never a fixed literal, so it has no DiscoveryKey/Payload of
	// its own at all.
	"cover": {
		{Name: "open", Service: "cover.open_cover", Payload: "OPEN", DiscoveryKey: "payload_open"},
		{Name: "close", Service: "cover.close_cover", Payload: "CLOSE", DiscoveryKey: "payload_close"},
		{Name: "stop", Service: "cover.stop_cover", Payload: "STOP", DiscoveryKey: "payload_stop"},
		// set_position (2026-09-25): matches the discovery-kind Z-Wave awnings cover's own
		// existing feature set (full position control) -- MQTT cover.py's own set_position_topic
		// carries the raw target position (0-100), never a fixed literal.
		{Name: "set_position", Service: "cover.set_cover_position", ValueTrigger: true, DataKey: "position", TopicKey: "set_position_topic"},
	},
	"button": {
		{Name: "press", Service: "button.press", Payload: "PRESS", DiscoveryKey: "payload_press"},
	},
	"number": {
		{Name: "set_value", Service: "number.set_value", ValueTrigger: true},
	},
	// lock (2026-09-25, the Volvo XC40's own door lock): matches HA's real MQTT lock schema
	// defaults (homeassistant/components/mqtt/lock.py) -- LOCK/UNLOCK payloads, no "open" command
	// (that's for electric-strike-style locks, not a car). See domainDiscoveryExtras' own "lock"
	// entry for why state_locked/state_unlocked must be overridden here too.
	"lock": {
		{Name: "lock", Service: "lock.lock", Payload: "LOCK", DiscoveryKey: "payload_lock"},
		{Name: "unlock", Service: "lock.unlock", Payload: "UNLOCK", DiscoveryKey: "payload_unlock"},
	},
}

// domainDiscoveryExtras carries fixed, non-payload discovery-config fields a domain's commands
// need beyond the command_topic/payload_* keys buildHassBridgeEntityDiscoveryBody (coordinator)
// already sets generically off domainCommands. Vacuum's "supported_features" is the one real case
// today: MQTT vacuum's own default (DEFAULT_SERVICES in homeassistant/components/mqtt/vacuum.py)
// is only start/stop/return_home/clean_spot -- pause and locate are silently unavailable unless
// listed explicitly. Note the feature STRING for "return to base" is "return_home", genuinely
// different from its own payload_return_to_base/DiscoveryKey name above -- confirmed against the
// live source, not a naming slip here.
var domainDiscoveryExtras = map[string]map[string]interface{}{
	"vacuum": {
		"supported_features": []string{"start", "pause", "stop", "return_home", "clean_spot", "locate"},
	},
	// lock's own state_locked/state_unlocked (2026-09-25): MQTT lock.py's own defaults are
	// uppercase "LOCKED"/"UNLOCKED" (homeassistant/components/mqtt/lock.py), but the bridged
	// source here is itself a native HA lock entity (states('lock.volvo_xc40_lock')), whose own
	// state strings are lowercase "locked"/"unlocked" -- same case-mismatch category as the
	// binary_sensor payload_on/off fix (house_event_bus_coordinator/discoverybridge.go), just
	// declared here instead since this is a hassbridge (kind-3) relay, not a kind-2 one.
	"lock": {
		"state_locked":   "locked",
		"state_unlocked": "unlocked",
	},
}

// domainStatePayloadTemplate is the read-side counterpart: most bridged domains report state as
// an atomic (bare-string) payload, which is all sourceToJinja2 ever produces and all
// entityReportingAutomationBody has ever published. Vacuum is the first domain that needs
// something else -- confirmed against homeassistant/components/mqtt/vacuum.py's own
// _state_message_received, its state_topic payload must already be a JSON object carrying a
// "state" key (there is no value_template hook to reshape a bare string into that at the
// discovery-config level the way a plain sensor could). "$" stands for the domain's own raw
// Jinja value expression (sourceToJinja2's output) -- substituted, then the whole thing still
// gets wrapped in "{{ ... }}" by entityReportingAutomationBody exactly as today. A domain with no
// entry here keeps today's atomic-payload behaviour unchanged.
var domainStatePayloadTemplate = map[string]string{
	"vacuum": "{'state': ($)} | tojson",
}

// applyStatePayloadTemplate renders domain's own state_topic payload expression from expr
// (sourceToJinja2's raw value expression) -- identity (expr unchanged) for any domain with no
// domainStatePayloadTemplate entry.
func applyStatePayloadTemplate(domain, expr string) string {
	template, hasTemplate := domainStatePayloadTemplate[domain]
	if !hasTemplate {
		return expr
	}
	return strings.ReplaceAll(template, "$", expr)
}

// buildHassBridgeStatePayload extends applyStatePayloadTemplate with cap's own PER-CAPABILITY
// Position source (2026-09-25, cover's own current-position feature -- THassBridgeCapability.
// Position's own doc comment), for instance -- unlike domainStatePayloadTemplate (a per-DOMAIN,
// code-level table), Position is a per-CAPABILITY Physical.def declaration, so it can't live in
// that same table at all. Returns the final payload expression plus the full set of bare trigger
// entities to watch (state's own, deduplicated with position's own when they differ -- almost
// always the same entity in practice, since position is typically "!current_position" on that
// SAME entity, but not assumed).
//
// No position declared for this instance: identity, applyStatePayloadTemplate's own result
// unchanged, exactly as before this function existed. Position declared: folds a 'position' key
// into the SAME JSON dict domain-templated domains (vacuum) already build, or starts a fresh
// {'state': ..., 'position': ...} dict for every other (atomic-payload) domain.
func buildHassBridgeStatePayload(cap THassBridgeCapability, instance, mappedExpr, triggerEntity string) (payloadExpr string, triggerEntities []string) {
	triggerEntities = []string{triggerEntity}
	positionSource, hasPosition := cap.Position[instance]
	if !hasPosition {
		return applyStatePayloadTemplate(cap.Domain, mappedExpr), triggerEntities
	}
	positionExpr := sourceToJinja2(positionSource)
	if _, hasDomainTemplate := domainStatePayloadTemplate[cap.Domain]; hasDomainTemplate {
		// No current real case combines a domain-templated domain (vacuum) with Position -- folds
		// position into that SAME dict rather than silently losing the domain's own wrapping.
		payloadExpr = strings.TrimSuffix(applyStatePayloadTemplate(cap.Domain, mappedExpr), "} | tojson") + ", 'position': (" + positionExpr + ")} | tojson"
	} else {
		payloadExpr = "{'state': (" + mappedExpr + "), 'position': (" + positionExpr + ")} | tojson"
	}
	if positionTriggerEntity := bareEntityFromSource(positionSource); positionTriggerEntity != triggerEntity {
		triggerEntities = append(triggerEntities, positionTriggerEntity)
	}
	return payloadExpr, triggerEntities
}
