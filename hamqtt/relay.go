/*
 *
 * Module:    HomeAssistant
 * Package:   HAMQTT
 * Component: Relay
 *
 * Home Assistant's MQTT knowledge shared by the generator and the coordinator (Architecture.md
 * §6.16; PROJECT.md 19g/19i, 2026-10-27): decoding a gateway's raw discovery config, and turning it
 * into the conceptual config Home Assistant gets -- the coordinator's relay
 * (house_event_bus_coordinator/discoverybridge.go, buildRelayedDiscoveryConfig) and, since 19, the
 * generator's YAML entities (homeassistant/mqtt_yaml_entities.go), from the same raw config and
 * the same declaration. One place, so the relay and the YAML can never differ.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 27.10.2026
 *
 */

package hamqtt

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FlexStringList decodes an HA discovery "ids"/"identifiers" field, which may be either a bare
// string or a list of strings.
type FlexStringList []string

func (f *FlexStringList) UnmarshalJSON(data []byte) error {
	var asList []string
	if err := json.Unmarshal(data, &asList); err == nil {
		*f = asList
		return nil
	}
	var asString string
	if err := json.Unmarshal(data, &asString); err != nil {
		return fmt.Errorf("identifiers: neither a string nor a list of strings: %w", err)
	}
	*f = []string{asString}
	return nil
}

type rawDevice struct {
	IDs         FlexStringList `json:"ids"`
	Identifiers FlexStringList `json:"identifiers"`
	ViaDevice   string         `json:"via_device"`
	Name        string         `json:"name"`
}

func (d rawDevice) identifiers() []string {
	if len(d.IDs) > 0 {
		return d.IDs
	}
	return d.Identifiers
}

// rawPayload decodes a gateway discovery payload, abbreviated or full-length key names both
// accepted.
type rawPayload struct {
	TopicPrefix         string    `json:"~"`
	UniqueID            string    `json:"uniq_id"`
	UniqueIDFull        string    `json:"unique_id"`
	StateTopic          string    `json:"stat_t"`
	StateTopicFull      string    `json:"state_topic"`
	CommandTopic        string    `json:"cmd_t"`
	CommandTopicFull    string    `json:"command_topic"`
	ValueTemplate       string    `json:"val_tpl"`
	ValueTemplateFull   string    `json:"value_template"`
	DeviceClass         string    `json:"dev_cla"`
	DeviceClassFull     string    `json:"device_class"`
	Unit                string    `json:"unit_of_meas"`
	UnitFull            string    `json:"unit_of_measurement"`
	StateClass          string    `json:"stat_cla"`
	StateClassFull      string    `json:"state_class"`
	DefaultEntityID     string    `json:"def_ent_id"`
	DefaultEntityIDFull string    `json:"default_entity_id"`
	Device              rawDevice `json:"dev"`
	DeviceFull          rawDevice `json:"device"`
}

// DecodedPayload is what the relay needs from a raw discovery payload.
type DecodedPayload struct {
	UniqueID          string
	StateTopic        string
	CommandTopic      string
	ValueTemplate     string
	DeviceClass       string
	Unit              string
	StateClass        string
	DeviceIdentifiers []string
	ViaDevice         string
	DeviceName        string
	DefaultEntityID   string
}

// FirstNonEmpty returns a, or b when a is empty.
func FirstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Decode decodes a raw discovery payload.
func Decode(raw []byte) (DecodedPayload, error) {
	var p rawPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return DecodedPayload{}, err
	}

	stateTopic := FirstNonEmpty(p.StateTopic, p.StateTopicFull)
	commandTopic := FirstNonEmpty(p.CommandTopic, p.CommandTopicFull)
	if p.TopicPrefix != "" {
		stateTopic = strings.ReplaceAll(stateTopic, "~", p.TopicPrefix)
		commandTopic = strings.ReplaceAll(commandTopic, "~", p.TopicPrefix)
	}

	device := p.Device
	if len(device.identifiers()) == 0 && device.ViaDevice == "" {
		device = p.DeviceFull
	}

	return DecodedPayload{
		UniqueID:          FirstNonEmpty(p.UniqueID, p.UniqueIDFull),
		StateTopic:        stateTopic,
		CommandTopic:      commandTopic,
		ValueTemplate:     FirstNonEmpty(p.ValueTemplate, p.ValueTemplateFull),
		DeviceClass:       FirstNonEmpty(p.DeviceClass, p.DeviceClassFull),
		Unit:              FirstNonEmpty(p.Unit, p.UnitFull),
		StateClass:        FirstNonEmpty(p.StateClass, p.StateClassFull),
		DeviceIdentifiers: device.identifiers(),
		ViaDevice:         device.ViaDevice,
		DeviceName:        device.Name,
		DefaultEntityID:   FirstNonEmpty(p.DefaultEntityID, p.DefaultEntityIDFull),
	}, nil
}

// AbbreviatedKeyPairs are the abbreviated keys the relay drops once it has set their full-length
// counterpart.
var AbbreviatedKeyPairs = [][2]string{
	{"uniq_id", "unique_id"},
	{"def_ent_id", "default_entity_id"},
	{"stat_t", "state_topic"},
	{"cmd_t", "command_topic"},
	{"dev_cla", "device_class"},
	{"unit_of_meas", "unit_of_measurement"},
	{"stat_cla", "state_class"},
	{"ic", "icon"},
}

// RelayedUniqueID is a relayed entity's unique_id before a frozen one was assigned.
func RelayedUniqueID(gatewayID, leaf string) string {
	return strings.ReplaceAll(gatewayID, ".", "_") + "_" + leaf
}

// Link is what the declaration (Physical.def, via the generator) says about one relayed entity.
type Link struct {
	StableID          string // the frozen "as <id>"; empty: RelayedUniqueID
	Name              string // the conceptual name; empty: the gateway's own, or null for a sole capability
	DeviceClass       string
	Unit              string
	StateClass        string
	Icon              string
	ValueTemplateWrap string // a "derived ... via ...;" template, "$" standing for the raw value
	Invert            bool
}

// RelayedConfig carries the rules of the coordinator's former buildRelayedDiscoveryConfig (moved here 2026-10-27); its
// documentation, with the live bugs behind each rule, follows unchanged.
//
// buildRelayedDiscoveryConfig builds our own discovery payload for entityID (e.g.
// "sensor.social_garage_door_temperature"), sourced from rawPayload -- the gateway leaf's own
// discovery payload, decoded into payload for the fields this function needs to inspect/override,
// but otherwise cloned wholesale. Only unique_id/default_entity_id/name/state_topic/
// command_topic/origin are overridden (all keyed on relayedDiscoveryUniqueID -- gateway+leaf --
// not on objectID, so relaying the same leaf under a different entityID, a Spaces.def reposition,
// changes default_entity_id/name but never the topic itself; "object_id"/"has_entity_name" are
// not real MQTT discovery keys, see discovery.go's payload doc comments); everything else the
// gateway published (brightness/supported_color_modes/effect_list/schema/availability/device/...)
// passes through untouched.
//
// "name" is the entity's own conceptual name from discovery.yaml (link.Name, PROJECT.md 17h,
// 2026-10-26): Home Assistant's UI shows an entity by its registry name, which an MQTT entity takes
// from this field -- not by the friendly_name the generator's customization sets -- so the
// gateway's own short label ("Temperature") was all the UI showed. The history below is what
// applies when link.Name is empty (a discovery.yaml from before it existed).
//
// "name" -- real bug found live 2026-09-29 (Junglinster's "furnace switch," reported
// friendly_name "house/laundry_kitchen/heating/furnace_switch social_house_kitchen_workplace_furnace"):
// this used to unconditionally overwrite "name" with objectID (entityID's own sanitised,
// underscore-joined tail) -- the exact same "not real null" mistake TBinarySensorDiscoveryPayload's
// own doc comment already fixed once for the "hosts" node entity (discovery.go), and
// buildHassBridgeEntityDiscoveryBody's own devBlock.Name/DisplaySuffix fix repeated for hassbridge
// (discoveryhassbridge.go) -- HA combines a non-null "name" with device.name for any
// device-grouped MQTT entity, so a single-capability gateway (this furnace switch's own device
// publishes exactly one entity) showed device.name + " " + objectID, doubled and unreadable.
// soleGatewayCapability (discoveryEntityLinksPerGateway, computed once per subscribe rather than
// per message) is true for exactly this shape -- "name" is then set to nil (JSON null), "device
// name alone," matching TBinarySensorDiscoveryPayload's own established convention. A gateway
// relaying SEVERAL capabilities (a 4-facet Aqara multi-sensor, an EMS-ESP boiler) is different:
// there, "name" is left as whatever the clone above already carried from the gateway's own native
// payload (Zigbee2MQTT/EMS-ESP already publish a short, genuinely distinguishing per-entity label
// there -- "Occupancy"/"Battery"/"Boiler Temperature" -- unrelated to this DSL's own entity_id
// positioning, so there was never a real "stale name after a Spaces.def reposition" risk to guard
// against in the first place; forcing objectID onto every one of a multi-entity gateway's
// capabilities would have made them all show the SAME doubled string, an even worse regression
// than the single-capability case this was found from). This is a deliberate change (2026-09-14) from an earlier version that
// reconstructed the whole body from scratch, keyed only on this function's own narrow decoded
// subset: that silently dropped command_topic entirely (a relayed light/switch/fan came up
// completely uncontrollable -- no error, just no way to command it) and never even decoded
// brightness/color-mode/effect support in the first place. Invisible as long as this mechanism
// was only ever exercised against EMS-ESP's own read-only sensors; found live while preparing
// Vienna's first real light migration under PROJECT.md item 7.
//
// device_class/unit_of_measurement/state_class prefer the gateway's own natively-reported value
// (it decided its own typing, see this file's own header comment) -- link's fields
// (Defaults.def-resolved, generator-side) only gap-fill a field the gateway's payload left
// empty, never override one it set. icon has no native counterpart in the decoded payload at
// all (see decodeDiscoveryPayload's "extend on demand" abbreviation set), so it comes from link
// unconditionally.
//
// payload.StateTopic is only conditionally overridden (like CommandTopic already was), not
// required: real bug, found live 2026-09-15 migrating Vienna's first "climate" domain device
// (a Bitron/SMaBiT wall thermostat). HA's MQTT Climate discovery schema has no single top-level
// "state_topic" key at all -- it uses per-feature topics instead (mode_state_topic,
// temperature_state_topic, current_temperature_topic, action_topic), all of which pass through
// untouched via the wholesale rawPayload clone above. Requiring StateTopic unconditionally (as
// this function used to) silently dropped every climate-domain relay with no error at all -- the
// existence tracker still marked the leaf known-to-exist (it decodes independently), which made
// the gap easy to miss without checking the coordinator's own "queued relay of" log lines leaf by
// leaf.
//
// RelayedConfig turns a gateway's raw discovery payload into the conceptual config Home Assistant
// gets for entityID ("<domain>.<object id>") -- see this file's header comment. rawDomain is the
// domain the gateway published it under; soleGatewayCapability that it is its gateway's only
// declared capability (its name is then the device's: null). The caller adds what is its own (the
// coordinator its topic and origin). The rules, and the live bugs behind them, are documented above.
func RelayedConfig(entityID, gatewayID string, raw map[string]interface{}, payload DecodedPayload, link Link, rawDomain string, soleGatewayCapability bool) (map[string]interface{}, bool) {
	dotIdx := strings.Index(entityID, ".")
	if dotIdx < 0 || payload.UniqueID == "" {
		return nil, false
	}
	domain := entityID[:dotIdx]
	objectID := entityID[dotIdx+1:]
	stableID := link.StableID
	if stableID == "" {
		stableID = RelayedUniqueID(gatewayID, payload.UniqueID)
	}

	body := make(map[string]interface{}, len(raw)+4)
	for k, v := range raw {
		body[k] = v
	}
	for _, pair := range AbbreviatedKeyPairs {
		delete(body, pair[0])
	}
	// A leaf relayed under another domain than its own: its typing is only meaningful there.
	if rawDomain != "" && rawDomain != domain {
		delete(body, "device_class")
		delete(body, "unit_of_measurement")
		delete(body, "state_class")
	}

	body["unique_id"] = stableID
	body["default_entity_id"] = domain + "." + objectID
	switch {
	case link.Name != "":
		body["name"] = link.Name
	case soleGatewayCapability:
		body["name"] = nil
	}
	if payload.StateTopic != "" {
		body["state_topic"] = payload.StateTopic
	}
	if payload.CommandTopic != "" {
		body["command_topic"] = payload.CommandTopic
	}
	// A firmware update is only shown, never installed from Home Assistant (PROJECT.md 17i).
	if domain == "update" {
		delete(body, "command_topic")
		delete(body, "payload_install")
	}
	if payload.ValueTemplate != "" {
		valueTemplate := payload.ValueTemplate
		if link.ValueTemplateWrap != "" {
			valueTemplate = WrapValueTemplate(valueTemplate, link.ValueTemplateWrap)
		}
		// The one(s) the domain knows: Home Assistant reads a state through either.
		for _, key := range []string{"value_template", "state_value_template"} {
			if Accepts(domain, body, key) {
				body[key] = valueTemplate
			}
		}
		if domain == "binary_sensor" && link.ValueTemplateWrap != "" {
			body["payload_on"] = "on"
			body["payload_off"] = "off"
		}
	}
	nativeDeviceClass, nativeUnit, nativeStateClass := payload.DeviceClass, payload.Unit, payload.StateClass
	if rawDomain != "" && rawDomain != domain {
		nativeDeviceClass, nativeUnit, nativeStateClass = "", "", ""
	}
	if deviceClass := FirstNonEmpty(nativeDeviceClass, link.DeviceClass); deviceClass != "" {
		body["device_class"] = deviceClass
	}
	if unit := FirstNonEmpty(nativeUnit, link.Unit); unit != "" {
		body["unit_of_measurement"] = unit
	}
	if stateClass := FirstNonEmpty(nativeStateClass, link.StateClass); stateClass != "" {
		body["state_class"] = stateClass
	}
	if link.Icon != "" {
		body["icon"] = link.Icon
	}
	if link.Invert {
		SwapPairWithDefaults(body, "payload_on", "payload_off", "ON", "OFF")
		SwapPairIfPresent(body, "state_on", "state_off")
	}
	return body, true
}

// WrapValueTemplate carries the rules of the coordinator's former wrapMQTTValueTemplate (moved here 2026-10-27); its
// documentation, with the live bugs behind each rule, follows unchanged.
//
// wrapMQTTValueTemplate folds wrapExpr (a generator-authored "derived ... via TTT;" template,
// "$" standing for the raw value) around rawTemplate's own inner Jinja expression -- e.g.
// rawTemplate `{{ value_json["pressure"] }}` and wrapExpr `($ | float(0)) + 20` produce
// `{% if "pressure" in value_json %}{{ ((value_json["pressure"]) | float(0)) + 20 }}{% else %}{{
// this.state }}{% endif %}`. Parenthesizes the substituted inner expression (same convention as
// the generator's own substituteDerivedTemplate, Physical_DerivedCapability.go) so wrapExpr's own
// operators can never silently change the raw extraction's precedence.
//
// Real bug found live 2026-09-16 (Vienna's hallway door/aqara_multi "pressure," derived via
// add_float_value(20)): Zigbee2MQTT devices commonly publish only the attribute(s) that changed in
// a given report, so a message can genuinely contain "humidity" while omitting "pressure"
// entirely, especially right after a Zigbee2MQTT restart (its own per-device cache resets, and a
// slower-reporting attribute like pressure can stay absent for a long time). value_json["pressure"]
// then evaluates to Jinja's Undefined, and piping that into "| float(0)" raises UndefinedError --
// NOT one of the (TypeError, ValueError) the float filter's own default actually catches -- so the
// whole template render fails and Home Assistant marks the ENTITY unavailable, even though the
// underlying sensor is perfectly reachable and its OTHER readings (humidity, temperature, no
// arithmetic wrap) render fine off the exact same message. A plain, unwrapped
// value_json["humidity"] never hits this: Jinja's Undefined renders as an empty string with no
// exception when merely interpolated, only when an operation (float(), +, ...) is applied to it.
//
// Fixed by guarding the WHOLE wrapped expression on the raw key's own presence in value_json,
// falling back to `this.state` (Home Assistant's own "this entity's current state" template
// variable) when it's momentarily missing -- holds the last known good reading instead of either
// flashing an arithmetically-wrong value (a bare "| default(0)" would silently turn a missing
// pressure into "20") or going unavailable for a device that's actually fine. Only applies when
// rawTemplate is recognisably a plain `value_json["<key>"]`/`value_json['<key>']` access (the only
// shape every real "derived ... via jinja ...;" case has ever used) -- any other inner shape falls
// back to the old, unguarded wrap unchanged, rather than guessing at a key name that isn't there.
//
// WrapValueTemplate folds wrapExpr ("$" standing for the raw value) around rawTemplate's own inner
// Jinja expression, guarded on the raw key's presence in value_json when rawTemplate is a plain
// value_json["<key>"] access (else the last state stays).
func WrapValueTemplate(rawTemplate, wrapExpr string) string {
	inner := strings.TrimSpace(rawTemplate)
	inner = strings.TrimPrefix(inner, "{{")
	inner = strings.TrimSuffix(inner, "}}")
	inner = strings.TrimSpace(inner)
	wrapped := strings.ReplaceAll(wrapExpr, "$", "("+inner+")")
	if key, ok := ExtractValueJSONKey(inner); ok {
		return fmt.Sprintf("{%% if %q in value_json %%}{{ %s }}{%% else %%}{{ this.state }}{%% endif %%}", key, wrapped)
	}
	return "{{ " + wrapped + " }}"
}

// ExtractValueJSONKey recognises inner as a plain value_json["<key>"] or value_json['<key>']
// access and returns the bare key.
func ExtractValueJSONKey(inner string) (key string, ok bool) {
	const prefix = "value_json["
	if !strings.HasPrefix(inner, prefix) || !strings.HasSuffix(inner, "]") {
		return "", false
	}
	quoted := inner[len(prefix) : len(inner)-1]
	if len(quoted) < 2 {
		return "", false
	}
	quote := quoted[0]
	if (quote != '"' && quote != '\'') || quoted[len(quoted)-1] != quote {
		return "", false
	}
	return quoted[1 : len(quoted)-1], true
}

// SwapPairWithDefaults swaps body[onKey]/body[offKey], substituting defaultOn/defaultOff for
// whichever the payload left unset first.
func SwapPairWithDefaults(body map[string]interface{}, onKey, offKey, defaultOn, defaultOff string) {
	on, hasOn := body[onKey]
	off, hasOff := body[offKey]
	if !hasOn {
		on = defaultOn
	}
	if !hasOff {
		off = defaultOff
	}
	body[onKey], body[offKey] = off, on
}

// SwapPairIfPresent swaps body[onKey]/body[offKey] only when the payload set at least one of them.
func SwapPairIfPresent(body map[string]interface{}, onKey, offKey string) {
	on, hasOn := body[onKey]
	off, hasOff := body[offKey]
	if !hasOn && !hasOff {
		return
	}
	body[onKey], body[offKey] = off, on
}
