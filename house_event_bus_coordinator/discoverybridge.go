/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: DiscoveryBridge
 *
 * Bridges externally discovered MQTT devices (e.g. EMS-ESP, a heating/boiler gateway that
 * already publishes its own complete, correct Home Assistant MQTT Discovery entirely on its
 * own) into our own conceptual entity ids. Such gateways are configured (outside this repo, in
 * their own settings) to publish discovery under a dedicated "physical" prefix
 * (TDiscoveryFile.PhysicalPrefix, resolved from ${mqtt_discovery_physical}) instead of the
 * default "homeassistant" prefix HA itself subscribes to -- so HA never sees a gateway's raw,
 * unpositioned discovery directly. This file subscribes to that prefix, and for every entity
 * the DSL referenced via "entity <spec> from <gateway-id>.<leaf>;"
 * (homeassistant/Conceptual_DiscoveryEntities.go, TDiscoveryFile.EntityLinks), republishes our
 * own discovery config -- our entity id, but state_topic/value_template/device_class/etc.
 * learned from the gateway's own payload -- onto the real "homeassistant/" prefix. Pure
 * discovery-config relay: no state is ever re-published, the coordinator just points our
 * discovery config at the gateway's own already-published state topic.
 *
 * Only the small set of HA MQTT Discovery abbreviations (see
 * home-assistant/core's mqtt/abbreviations.py) this needs to read are decoded -- extend on
 * demand as real gateway payloads need more fields relayed, not by front-loading the full table.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 22.08.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gopkg.in/yaml.v3"
)

// TDiscoveryPassthroughRule is one Physical.def "discovery_passthrough "<topic-prefix>" from
// "<source-prefix>";" declaration (homeassistant/discovery_passthrough.go).
type TDiscoveryPassthroughRule struct {
	TopicPrefix  string `yaml:"topic_prefix"`
	SourcePrefix string `yaml:"source_prefix"`
}

// TDiscoveryPassthroughFile is the top-level shape of a generated
// coordinator/discovery_passthrough.yaml file. Absent entirely when a house has no
// discovery_passthrough statements declared -- loadDiscoveryPassthroughFile treats that as
// "nothing to pass through," not an error, same convention as loadDiscoveryFile/
// loadDiscoveryCleanupFile.
type TDiscoveryPassthroughFile struct {
	Passthrough []TDiscoveryPassthroughRule `yaml:"passthrough"`
}

// loadDiscoveryPassthroughFile reads and parses a generator-produced
// coordinator/discovery_passthrough.yaml file, if one exists.
func loadDiscoveryPassthroughFile(path string) (TDiscoveryPassthroughFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return TDiscoveryPassthroughFile{}, nil
		}
		return TDiscoveryPassthroughFile{}, fmt.Errorf("cannot read %s: %w", path, err)
	}

	var passthroughFile TDiscoveryPassthroughFile
	if err := yaml.Unmarshal(data, &passthroughFile); err != nil {
		return TDiscoveryPassthroughFile{}, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	return passthroughFile, nil
}

// passthroughTopicPrefixes reduces rules to just the topic prefixes actionable against
// physicalPrefix -- a rule declared "from" a different source prefix can never match live traffic
// on this subscription (the generator already warned about this at generate time,
// homeassistant/discovery_passthrough.go); silently ignored here rather than re-warned, so the
// coordinator's own log isn't cluttered by a condition already surfaced at generate time.
func passthroughTopicPrefixes(rules []TDiscoveryPassthroughRule, physicalPrefix string) []string {
	var prefixes []string
	for _, rule := range rules {
		if rule.SourcePrefix != physicalPrefix {
			continue
		}
		prefixes = append(prefixes, rule.TopicPrefix)
	}
	return prefixes
}

// matchesAnyPassthroughPrefix reports whether topic (a discovery payload's own state_topic or
// command_topic, already "~"-expanded) starts with any of prefixes.
func matchesAnyPassthroughPrefix(topic string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if prefix != "" && strings.HasPrefix(topic, prefix) {
			return true
		}
	}
	return false
}

// topicComponent extracts the HA component (domain) segment from a discovery config topic shaped
// "<prefix>/<component>/<node_id>/<object_id>/config" -- passthrough's own suggestion tracking
// uses this as the real, known domain, rather than guessing one the way discovery_existence.go's
// own suggestion report has to for opaque gateway leaf names.
func topicComponent(topic, prefix string) string {
	rest := strings.TrimPrefix(topic, prefix+"/")
	if idx := strings.Index(rest, "/"); idx >= 0 {
		return rest[:idx]
	}
	return ""
}

// firstDeviceIdentifier returns payload's own primary device identifier -- the stable key
// passthrough device tracking (and its Forget-on-migration counterpart) is keyed on -- or "" if
// the payload carried none at all.
func firstDeviceIdentifier(payload tDecodedDiscoveryPayload) string {
	if len(payload.DeviceIdentifiers) == 0 {
		return ""
	}
	return payload.DeviceIdentifiers[0]
}

// tFlexStringList decodes an HA discovery "ids"/"identifiers" field, which may be either a bare
// string or a list of strings.
type tFlexStringList []string

func (f *tFlexStringList) UnmarshalJSON(data []byte) error {
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

// rawDiscoveryDevice decodes an incoming payload's "dev"/"device" block -- just the fields this
// bridge needs (identity + one-hop via_device + a human-readable name for passthrough
// suggestions, PROJECT.md item 7), not HA's full device-map field set.
type rawDiscoveryDevice struct {
	IDs         tFlexStringList `json:"ids"`
	Identifiers tFlexStringList `json:"identifiers"`
	ViaDevice   string          `json:"via_device"`
	Name        string          `json:"name"`
}

func (d rawDiscoveryDevice) identifiers() []string {
	if len(d.IDs) > 0 {
		return d.IDs
	}
	return d.Identifiers
}

// rawDiscoveryPayload decodes an incoming gateway discovery payload, abbreviated or full-length
// key names both accepted (see the component doc comment).
type rawDiscoveryPayload struct {
	TopicPrefix         string             `json:"~"`
	UniqueID            string             `json:"uniq_id"`
	UniqueIDFull        string             `json:"unique_id"`
	StateTopic          string             `json:"stat_t"`
	StateTopicFull      string             `json:"state_topic"`
	CommandTopic        string             `json:"cmd_t"`
	CommandTopicFull    string             `json:"command_topic"`
	ValueTemplate       string             `json:"val_tpl"`
	ValueTemplateFull   string             `json:"value_template"`
	DeviceClass         string             `json:"dev_cla"`
	DeviceClassFull     string             `json:"device_class"`
	Unit                string             `json:"unit_of_meas"`
	UnitFull            string             `json:"unit_of_measurement"`
	StateClass          string             `json:"stat_cla"`
	StateClassFull      string             `json:"state_class"`
	DefaultEntityID     string             `json:"def_ent_id"`
	DefaultEntityIDFull string             `json:"default_entity_id"`
	Device              rawDiscoveryDevice `json:"dev"`
	DeviceFull          rawDiscoveryDevice `json:"device"`
}

// tDecodedDiscoveryPayload is a gateway's discovery payload reduced to what this bridge relays.
type tDecodedDiscoveryPayload struct {
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

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// decodeDiscoveryPayload parses raw and applies "~" topic-prefix substitution to StateTopic, the
// same way HA itself expands it.
func decodeDiscoveryPayload(raw []byte) (tDecodedDiscoveryPayload, error) {
	var p rawDiscoveryPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return tDecodedDiscoveryPayload{}, err
	}

	stateTopic := firstNonEmpty(p.StateTopic, p.StateTopicFull)
	commandTopic := firstNonEmpty(p.CommandTopic, p.CommandTopicFull)
	if p.TopicPrefix != "" {
		stateTopic = strings.ReplaceAll(stateTopic, "~", p.TopicPrefix)
		commandTopic = strings.ReplaceAll(commandTopic, "~", p.TopicPrefix)
	}

	device := p.Device
	if len(device.identifiers()) == 0 && device.ViaDevice == "" {
		device = p.DeviceFull
	}

	return tDecodedDiscoveryPayload{
		UniqueID:          firstNonEmpty(p.UniqueID, p.UniqueIDFull),
		StateTopic:        stateTopic,
		CommandTopic:      commandTopic,
		ValueTemplate:     firstNonEmpty(p.ValueTemplate, p.ValueTemplateFull),
		DeviceClass:       firstNonEmpty(p.DeviceClass, p.DeviceClassFull),
		Unit:              firstNonEmpty(p.Unit, p.UnitFull),
		StateClass:        firstNonEmpty(p.StateClass, p.StateClassFull),
		DeviceIdentifiers: device.identifiers(),
		ViaDevice:         device.ViaDevice,
		DeviceName:        device.Name,
		DefaultEntityID:   firstNonEmpty(p.DefaultEntityID, p.DefaultEntityIDFull),
	}, nil
}

// discoveryNameLooksConsistent reports whether defaultEntityID's own object id plausibly matches
// stateTopic's own current friendly-name path -- i.e. whether a passthrough payload's own claimed
// name is self-consistent with where it's actually still being published, rather than a stale
// leftover from before a Zigbee2MQTT rename (see TPassthroughDeviceTracker.ClaimName's own doc
// comment for the real incident this distinguishes). Zigbee2MQTT's own discovery payloads always
// shape state_topic as "<integration>/<friendly-name>" and default_entity_id as
// "<domain>.<friendly-name>[_<leaf-suffix>]" -- so a currently-correct payload's object id always
// STARTS WITH state_topic's own friendly-name segment, while a stale one (renamed since) doesn't.
// Falls back to true (don't second-guess) whenever either string doesn't have the expected shape,
// so an unusual gateway's payload is never wrongly suppressed by a heuristic built for
// Zigbee2MQTT's own convention specifically.
func discoveryNameLooksConsistent(defaultEntityID, stateTopic string) bool {
	dotIdx := strings.Index(defaultEntityID, ".")
	if dotIdx < 0 {
		return true
	}
	objectID := defaultEntityID[dotIdx+1:]
	slashIdx := strings.Index(stateTopic, "/")
	if slashIdx < 0 {
		return true
	}
	friendlyName := stateTopic[slashIdx+1:]
	return friendlyName == "" || strings.HasPrefix(objectID, friendlyName)
}

// matchingGateway returns the DeviceID of the first TDiscoveryFile.Gateways entry payload
// belongs to -- either directly (its own device identifiers intersect the gateway's) or one hop
// via its device's via_device -- and false if none match.
func matchingGateway(payload tDecodedDiscoveryPayload, gateways map[string]TDiscoveryGateway) (string, bool) {
	for gatewayID, gateway := range gateways {
		for _, want := range gateway.Identifiers {
			for _, got := range payload.DeviceIdentifiers {
				if got == want {
					return gatewayID, true
				}
			}
			// The one-hop via_device rule below absorbs a SIBLING device (e.g. EMS-ESP's
			// "ems-esp-thermostat", via_device: "ems-esp") into its own parent gateway -- built for
			// a narrow multi-facet gateway, not a network's own root device. IgnoreOtherCapabilities
			// opts a gateway out of it entirely (see its own doc comment for the real incident this
			// fixes): such a gateway still matches normally via a direct Identifiers hit above, just
			// never absorbs an unrelated device that merely happens to route through it.
			if !gateway.IgnoreOtherCapabilities && payload.ViaDevice != "" && payload.ViaDevice == want {
				return gatewayID, true
			}
		}
	}
	return "", false
}

// relayedDiscoveryUniqueID builds the stable identity a relayed entity's unique_id/topic are keyed
// on -- the gateway's own identity plus the gateway's own leaf name (payload.UniqueID), never our
// own entityID's readable path, which can be repositioned/renamed independently of the gateway's
// own naming. Shared with discoverycleanup.go's expectedDiscoveryBridgeTopics so the two can't
// drift apart.
func relayedDiscoveryUniqueID(gatewayID, leaf string) string {
	return sanitizeTopicSegment(gatewayID) + "_" + leaf
}

// discoveryAbbreviatedKeyPairs are every (short, long) MQTT-discovery key pair
// buildRelayedDiscoveryConfig itself sets an override for -- both variants are deleted from the
// cloned raw payload before the long form is written, so a stale abbreviated value from the
// gateway's own payload style (EMS-ESP uses short keys throughout) never coexists with our
// override. HA accepts long-form keys fine even alongside a gateway's otherwise all-short-form
// payload -- already proven live, since "default_entity_id" (no natural EMS-ESP short form other
// than "def_ent_id") has always been written this way.
var discoveryAbbreviatedKeyPairs = [][2]string{
	{"uniq_id", "unique_id"},
	{"def_ent_id", "default_entity_id"},
	{"stat_t", "state_topic"},
	{"cmd_t", "command_topic"},
	{"dev_cla", "device_class"},
	{"unit_of_meas", "unit_of_measurement"},
	{"stat_cla", "state_class"},
	{"ic", "icon"},
}

// buildRelayedDiscoveryConfig builds our own discovery payload for entityID (e.g.
// "sensor.social_garage_door_temperature"), sourced from rawPayload -- the gateway leaf's own
// discovery payload, decoded into payload for the fields this function needs to inspect/override,
// but otherwise cloned wholesale. Only unique_id/default_entity_id/name/state_topic/
// command_topic/origin are overridden (all keyed on relayedDiscoveryUniqueID -- gateway+leaf --
// not on objectID, so relaying the same leaf under a different entityID, a Spaces.def reposition,
// changes default_entity_id/name but never the topic itself; "object_id"/"has_entity_name" are
// not real MQTT discovery keys, see discovery.go's payload doc comments); everything else the
// gateway published (brightness/supported_color_modes/effect_list/schema/availability/device/...)
// passes through untouched. This is a deliberate change (2026-09-14) from an earlier version that
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
func wrapMQTTValueTemplate(rawTemplate, wrapExpr string) string {
	inner := strings.TrimSpace(rawTemplate)
	inner = strings.TrimPrefix(inner, "{{")
	inner = strings.TrimSuffix(inner, "}}")
	inner = strings.TrimSpace(inner)
	wrapped := strings.ReplaceAll(wrapExpr, "$", "("+inner+")")
	if key, ok := extractValueJSONKey(inner); ok {
		return fmt.Sprintf("{%% if %q in value_json %%}{{ %s }}{%% else %%}{{ this.state }}{%% endif %%}", key, wrapped)
	}
	return "{{ " + wrapped + " }}"
}

// extractValueJSONKey recognises inner as a plain `value_json["<key>"]` or `value_json['<key>']`
// access (Zigbee2MQTT's own universal value_template shape for a leaf reading) and returns the bare
// key -- see wrapMQTTValueTemplate's own doc comment for why this gates the missing-key-safe wrap.
func extractValueJSONKey(inner string) (key string, ok bool) {
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

func buildRelayedDiscoveryConfig(entityID, gatewayID string, rawPayload map[string]interface{}, payload tDecodedDiscoveryPayload, link TDiscoveryEntityLink, prefix, installation, rawDomain string) (topic string, body map[string]interface{}, ok bool) {
	dotIdx := strings.Index(entityID, ".")
	if dotIdx < 0 || payload.UniqueID == "" {
		return "", nil, false
	}
	domain := entityID[:dotIdx]
	objectID := entityID[dotIdx+1:]
	stableID := relayedDiscoveryUniqueID(gatewayID, payload.UniqueID)

	body = make(map[string]interface{}, len(rawPayload)+4)
	for k, v := range rawPayload {
		body[k] = v
	}
	for _, pair := range discoveryAbbreviatedKeyPairs {
		delete(body, pair[0])
	}
	// A "derived ... via jinja ...;" capability can relay a HIDDEN sibling leaf published under a
	// DIFFERENT raw domain than the derived capability's own target domain -- e.g. a sensor-domain
	// power leaf (Zigbee2MQTT's own native discovery gives it device_class "power"/unit "W"/
	// state_class "measurement") feeding a binary_sensor-domain "consumes" capability. Those three
	// fields are only ever meaningful within the domain that originated them (unit_of_measurement/
	// state_class aren't even valid MQTT binary_sensor schema fields at all), so wholesale-copying
	// them across a domain change is wrong regardless of which values they happen to hold -- real
	// bug found live 2026-09-25 (Junglinster's furnace "consumes" binary_sensor showing permanently
	// unavailable after being hidden-relayed from its own power leaf). Cleared here, before the
	// explicit link/payload-driven override below (which still applies normally either way) --
	// same-domain relays (the common case) are completely unaffected.
	if rawDomain != "" && rawDomain != domain {
		delete(body, "device_class")
		delete(body, "unit_of_measurement")
		delete(body, "state_class")
	}

	body["unique_id"] = stableID
	body["default_entity_id"] = domain + "." + objectID
	body["name"] = objectID
	if payload.StateTopic != "" {
		body["state_topic"] = payload.StateTopic
	}
	if payload.CommandTopic != "" {
		body["command_topic"] = payload.CommandTopic
	}
	body["origin"] = coordinatorOriginMap(installation)
	if payload.ValueTemplate != "" {
		valueTemplate := payload.ValueTemplate
		if link.ValueTemplateWrap != "" {
			valueTemplate = wrapMQTTValueTemplate(valueTemplate, link.ValueTemplateWrap)
		}
		body["value_template"] = valueTemplate
		// Real bug, 2026-09-14: a switch-shaped Zigbee2MQTT payload's own "value_template" key
		// (correct for the domain it was NATIVELY published under) isn't a field every domain's
		// MQTT schema recognises -- HA's Fan integration specifically expects
		// "state_value_template" instead, so relaying a switch's payload verbatim under domain
		// "fan" (see this item's own "declare the leaf as the wrapper's own domain directly"
		// pattern) left the entity stuck on "unknown" forever: messages arrived fine, HA just had
		// no recognised field to extract a value from. Setting both is harmless for domains that
		// only recognise "value_template" -- HA's discovery schemas ignore keys they don't use,
		// confirmed live (no validation error/warning appeared for any already-working entity
		// carrying both).
		body["state_value_template"] = valueTemplate
		// Real bug, 2026-09-25 (Junglinster's furnace "consumes" binary_sensor, first real use of
		// a "derived ... via ...;" wrap on a binary_sensor-domain relay): every jinja macro/formula
		// this codebase writes for an on/off derivation (${int_more_then}/${int_less_then}, and any
		// hand-written "'on' if ... else 'off'" formula) renders lowercase "on"/"off" literal
		// strings -- correct for HA's TEMPLATE platform (lenient, accepts many truthy forms), but
		// MQTT's binary_sensor schema does an exact, case-sensitive string match against
		// payload_on/payload_off, which default to "ON"/"OFF" (uppercase) when unset. Neither
		// rawPayload nor link ever supplies these for a wrapped relay, so the rendered "on"/"off"
		// matched neither default, leaving the entity stuck on "unknown" forever (messages arrived
		// and rendered fine, HA just had no matching payload to recognise as a state). Only set when
		// a wrap is actually applied -- an unwrapped same-domain binary_sensor relay keeps whatever
		// payload_on/payload_off the raw native payload already carries (wholesale-copied above).
		if domain == "binary_sensor" && link.ValueTemplateWrap != "" {
			body["payload_on"] = "on"
			body["payload_off"] = "off"
		}
	}
	// payload.DeviceClass/Unit/StateClass are decoded from the SAME raw payload as the wholesale
	// body copy above, so they carry the identical cross-domain contamination when rawDomain !=
	// domain -- gated the same way, for the same reason (see this function's own doc comment on the
	// delete() block above). Only link's own explicit, domain-aware override may apply then.
	nativeDeviceClass, nativeUnit, nativeStateClass := payload.DeviceClass, payload.Unit, payload.StateClass
	if rawDomain != "" && rawDomain != domain {
		nativeDeviceClass, nativeUnit, nativeStateClass = "", "", ""
	}
	if deviceClass := firstNonEmpty(nativeDeviceClass, link.DeviceClass); deviceClass != "" {
		body["device_class"] = deviceClass
	}
	if unit := firstNonEmpty(nativeUnit, link.Unit); unit != "" {
		body["unit_of_measurement"] = unit
	}
	if stateClass := firstNonEmpty(nativeStateClass, link.StateClass); stateClass != "" {
		body["state_class"] = stateClass
	}
	if link.Icon != "" {
		body["icon"] = link.Icon
	}
	return discoveryTopic(prefix, domain, stableID), body, true
}

// subscribeDiscoveryBridge subscribes to "<physical_prefix>/+/+/+/config" (the same
// <prefix>/<component>/<node_id>/<object_id>/config shape HA's own discovery uses) and, for
// every message whose device matches a declared gateway (matchingGateway) and whose leaf
// (UniqueID) a devices.yaml EntityLink references, republishes our own discovery config through
// publisher (content-aware retire-then-republish, discoverycleanup.go) onto conceptualPrefix.
// Also records the leaf as known-to-exist on existenceTracker (PROJECT.md 1.8, discovery_existence.go)
// regardless of whether an EntityLink claims it yet, and -- when it's newly known -- publishes that
// gateway's updated status to mainClient/cloudClient. An empty payload (HA's own MQTT discovery
// removal convention) is treated as a retraction: existenceTracker.MarkRetracted resolves it back
// to a (gatewayID, leaf) via whatever identity a prior non-empty payload on the same topic recorded
// (RecordTopicIdentity), moving that leaf to known-not-to-exist. existenceTracker may be nil (tests
// that don't need it).
//
// passthroughRules (PROJECT.md item 7, 2026-09-14) is the gradual-migration counterpart to the
// above: for a message whose device DOESN'T match a declared gateway, but whose state_topic or
// command_topic starts with a declared discovery_passthrough prefix, the raw payload is relayed
// byte-for-byte onto conceptualPrefix (topic tail unchanged, only the leading physical-prefix
// segment swapped) -- so HA keeps seeing a not-yet-migrated device exactly as it always has, no
// Physical.def declaration required. The two are mutually exclusive per message specifically so a
// device doesn't ever appear twice once it migrates: the moment a device's identifiers start
// matching a declared gateway, this same handler also retires whatever raw copy it may have been
// relaying for it (publisher.Knows guards the retire so it's only attempted for a topic this
// publisher actually knows about, not every declared-gateway message regardless of passthrough
// history). Only rules whose declared source matches discoveryFile.PhysicalPrefix are actionable
// here (passthroughTopicPrefixes) -- this subscription only ever sees that one prefix.
//
// passthroughDeviceTracker (2026-09-14, found live: real Zigbee2MQTT traffic was flowing through
// passthrough but never showed up in suggestions/discovery.txt; corrected 2026-09-19 -- see the
// "!matched" branch's own comment above) records every device seen under <physical_prefix> that
// no declared gateway claims, keyed by its own real device identifier, so the generator can
// suggest a ready-to-paste declaration for it (discovery_passthrough_devices.go) -- REGARDLESS of
// whether it's also being actively passthrough-relayed into HA (that's the separate "keep it
// visible while unmigrated" decision, still gated on a declared discovery_passthrough rule).
// Forgotten again the instant a device starts matching a declared gateway, right alongside the
// existing raw-topic retirement above. Deliberately Record/Forget only here, never a per-message
// PublishStatus (a second real incident,
// found live minutes after the first deploy of this same feature: the initial subscribe replays
// the whole retained backlog synchronously, and a full-snapshot publish per newly-seen leaf
// starved the client's own later subscribes of their timeout window, crash-looping the
// coordinator) -- main.go publishes the tracker's status once at startup and once per reconnect
// instead, which is all a generate-time suggestion feed needs. May be nil (tests that don't need
// it).
//
// relayJobs (2026-09-14, discovery_relay_queue.go) is where every outbound publish/retire this
// handler decides to make actually goes -- never called directly against publisher any more. See
// that file's own header comment for the two real incidents (a fatal crash loop, then a confirmed
// live "No ACK from MQTT server" warning on HA's own side) that made both decoupling AND throttling
// necessary, not just one or the other. The raw passthrough publish below sets Passthrough: true so
// RetireMissing's static startup sweep never deletes it out from under a live device -- see
// TDiscoveryPublisher.RetireMissing's own doc comment for the real incident (2026-09-14) this
// exists to prevent a repeat of.
func subscribeDiscoveryBridge(client, cloudClient mqtt.Client, ownInstallation string, discoveryFile TDiscoveryFile, publisher *TDiscoveryPublisher, conceptualPrefix string, existenceTracker *TDiscoveryExistenceTracker, passthroughRules []TDiscoveryPassthroughRule, passthroughDeviceTracker *TPassthroughDeviceTracker, relayJobs chan<- TDiscoveryRelayJob) error {
	passthroughPrefixes := passthroughTopicPrefixes(passthroughRules, discoveryFile.PhysicalPrefix)

	handler := func(_ mqtt.Client, msg mqtt.Message) {
		passthroughTopic := conceptualPrefix + strings.TrimPrefix(msg.Topic(), discoveryFile.PhysicalPrefix)

		if len(msg.Payload()) == 0 {
			if existenceTracker != nil {
				if gatewayID, leaf, changed := existenceTracker.MarkRetracted(msg.Topic()); changed {
					fmt.Printf("[discovery-existence] %s: %s -> known-not-to-exist (retracted)\n", gatewayID, leaf)
					existenceTracker.ScheduleAggregateStatusPublish(client, cloudClient, ownInstallation, DiscoveryExistenceStatusDebounceDelay)
				}
			}
			// A still-undeclared (passthrough-tracked) device deleted outright from Zigbee2MQTT --
			// not renamed/migrated into a real Physical.def declaration, genuinely removed from the
			// network -- publishes this same empty-payload retraction on its own discovery config
			// topics. Forget was previously only ever called from the "just started matching a
			// declared gateway" branch below (i.e. migration), so a deleted-but-never-migrated
			// device's own suggestion entry never went away on its own. See ForgetByTopic's own doc
			// comment for the real case this fixes (2026-09-21, Vienna).
			if passthroughDeviceTracker != nil {
				if deviceIdentifier, forgotten := passthroughDeviceTracker.ForgetByTopic(msg.Topic()); forgotten {
					fmt.Printf("[discovery-passthrough] %s: forgotten (retracted)\n", deviceIdentifier)
					passthroughDeviceTracker.ScheduleStatusPublish(client, cloudClient, ownInstallation, PassthroughStatusDebounceDelay)
				}
			}
			if publisher.Knows("main", passthroughTopic) {
				enqueueDiscoveryRelayJob(relayJobs, TDiscoveryRelayJob{Action: discoveryRelayRetire, Client: client, Broker: "main", Topic: passthroughTopic})
			}
			return
		}

		payload, err := decodeDiscoveryPayload(msg.Payload())
		if err != nil {
			fmt.Printf("[discovery-bridge] %s: cannot parse payload: %v\n", msg.Topic(), err)
			return
		}
		if payload.UniqueID == "" {
			return
		}

		gatewayID, matched := matchingGateway(payload, discoveryFile.Gateways)
		if !matched {
			// Real bug found live 2026-09-19: passthroughDeviceTracker.Record used to be nested
			// INSIDE the matchesAnyPassthroughPrefix check below, coupling the generator's own
			// "which real devices exist under ${mqtt_discovery_physical} that Physical.def hasn't
			// declared yet" suggestion feed to whether a "discovery_passthrough ...;" rule happens
			// to be declared at all. This subscription already sees EVERY message under
			// <physical_prefix>/+/+/+/config regardless of passthrough rules -- tracking (for
			// suggestions) and relaying (to keep an undeclared device visible in HA) are two
			// genuinely separate concerns that were wrongly conflated. Recording now happens for
			// ANY undeclared device unconditionally; only the relay decision just below stays
			// gated on matchesAnyPassthroughPrefix, since THAT one really does depend on whether
			// passthrough is configured.
			deviceIdentifier := firstDeviceIdentifier(payload)
			if passthroughDeviceTracker != nil {
				domain := topicComponent(msg.Topic(), discoveryFile.PhysicalPrefix)
				// Recorded before Record() itself, mirroring existenceTracker's own
				// RecordTopicIdentity-then-MarkKnown ordering just below in the matched branch --
				// so a later empty (retracted) payload on this exact topic always has an identity to
				// resolve against, see ForgetByTopic's own doc comment.
				passthroughDeviceTracker.RecordTopicIdentity(msg.Topic(), deviceIdentifier)
				// Record, then debounce a status republish when it actually changed something --
				// NOT a synchronous PublishStatus() call per message (real incident, found live
				// 2026-09-14 right after this deployed: the coordinator's initial subscribe
				// replays the ENTIRE retained backlog synchronously, one message at a time --
				// hundreds of them, each a "first time seen" leaf -- and an ack-waiting network
				// round trip per leaf here starved the client's own connection setup of time to
				// finish its LATER subscribes within their own timeout window, crash-looping the
				// whole coordinator). ScheduleStatusPublish's own debounce coalesces exactly that
				// kind of burst into a single publish once it quiets down, rather than requiring
				// the retained topic to go stale until the next restart/reconnect the way a bare
				// Record-and-discard did (real gap found live 2026-09-19: Junglinster's own Z-Wave
				// devices sat untracked-in-suggestions for over a day of normal operation).
				if passthroughDeviceTracker.Record(deviceIdentifier, payload.DeviceName, domain, payload.UniqueID) {
					passthroughDeviceTracker.ScheduleStatusPublish(client, cloudClient, ownInstallation, PassthroughStatusDebounceDelay)
					passthroughDeviceTracker.SchedulePersist(PassthroughStatusDebounceDelay)
				}
			}
			if matchesAnyPassthroughPrefix(payload.StateTopic, passthroughPrefixes) || matchesAnyPassthroughPrefix(payload.CommandTopic, passthroughPrefixes) {
				// Real bug found live 2026-09-20: a passthrough-relayed device's payload is trusted
				// wholesale, including its own "default_entity_id" -- so a Zigbee2MQTT device
				// renamed away from an old friendly name, whose stale retained discovery config
				// never got refreshed, kept claiming the SAME default_entity_id a different,
				// currently-correct device had since taken over. Both got relayed, and HA silently
				// disambiguated by suffixing one of them ("Entity not found" for whichever name the
				// rest of the system expected). ClaimName lets the first-seen device keep its name;
				// a later, different device claiming the same name is suppressed here and flagged
				// as a collision instead of blindly relayed (discovery_passthrough_suggestions.go's
				// buildPassthroughCollisionReport surfaces it in suggestions/discovery.txt).
				claimOK := true
				evictedTopic := ""
				if passthroughDeviceTracker != nil {
					claimOK, evictedTopic = passthroughDeviceTracker.ClaimName(payload.DefaultEntityID, deviceIdentifier, passthroughTopic, discoveryNameLooksConsistent(payload.DefaultEntityID, payload.StateTopic))
				}
				if claimOK {
					if evictedTopic != "" {
						// A less self-consistent (likely stale, since-renamed) device had already
						// been accepted for relay under this name before the genuinely current one
						// arrived -- retire its topic now so it doesn't linger in HA alongside the
						// new owner. No publisher.Knows guard here: ClaimName only ever returns a
						// non-empty evictedTopic for a topic its own claim previously accepted, so
						// its publish job either already ran or is still sitting in the relay queue
						// -- retiring a topic HA never actually received is a harmless no-op either
						// way, and gating on Knows would race against that not-yet-drained job.
						enqueueDiscoveryRelayJob(relayJobs, TDiscoveryRelayJob{Action: discoveryRelayRetire, Client: client, Broker: "main", Topic: evictedTopic})
						fmt.Printf("[discovery-bridge] retiring displaced passthrough relay %s: default_entity_id %q now correctly claimed by %s\n", evictedTopic, payload.DefaultEntityID, msg.Topic())
					}
					// Payload bytes are copied: msg.Payload() isn't guaranteed valid once this
					// handler returns, and this job may not actually be processed until well after
					// that.
					payloadCopy := append([]byte(nil), msg.Payload()...)
					enqueueDiscoveryRelayJob(relayJobs, TDiscoveryRelayJob{Action: discoveryRelayPublish, Client: client, Broker: "main", Topic: passthroughTopic, Payload: payloadCopy, Passthrough: true})
					fmt.Printf("[discovery-bridge] queued passthrough relay %s -> %s\n", msg.Topic(), passthroughTopic)
				} else {
					fmt.Printf("[discovery-bridge] suppressing passthrough relay %s -> %s: default_entity_id %q already claimed by a different device\n", msg.Topic(), passthroughTopic, payload.DefaultEntityID)
					// Real bug found live 2026-09-20, minutes after the displacement fix above:
					// that fix only retires a topic evicted WITHIN the same in-memory ClaimName
					// session -- but ordinary claims are deliberately NOT persisted across restarts
					// (the crash-loop fix), so a topic this exact device relayed and got RETAINED
					// in an EARLIER (e.g. pre-consistency-check) coordinator run never gets
					// retired at all if, on a LATER restart, it simply loses the claim race outright
					// (no live displacement occurs, since it never held the claim this session to
					// begin with) -- the stale entity then lingers in HA forever, colliding with the
					// correct one exactly as before. publisher.Knows persists independently of
					// ClaimName's own in-memory state, so check it here for every rejected claim,
					// not just a same-session displacement.
					if publisher.Knows("main", passthroughTopic) {
						enqueueDiscoveryRelayJob(relayJobs, TDiscoveryRelayJob{Action: discoveryRelayRetire, Client: client, Broker: "main", Topic: passthroughTopic})
						fmt.Printf("[discovery-bridge] retiring previously-relayed passthrough topic %s: default_entity_id %q now rejected as a collision\n", passthroughTopic, payload.DefaultEntityID)
					}
				}
			}
			return
		}

		if publisher.Knows("main", passthroughTopic) {
			// This device just started matching a declared gateway -- retire whatever raw
			// passthrough copy was relaying it before, so HA never shows both at once.
			enqueueDiscoveryRelayJob(relayJobs, TDiscoveryRelayJob{Action: discoveryRelayRetire, Client: client, Broker: "main", Topic: passthroughTopic})
		}
		if passthroughDeviceTracker != nil {
			// Debounce a status republish here too -- same reasoning as the passthrough branch
			// above, so a just-migrated device stops appearing in suggestions/discovery.txt
			// promptly rather than only after the next restart/reconnect.
			if passthroughDeviceTracker.Forget(firstDeviceIdentifier(payload)) {
				passthroughDeviceTracker.ScheduleStatusPublish(client, cloudClient, ownInstallation, PassthroughStatusDebounceDelay)
			}
		}

		if existenceTracker != nil {
			existenceTracker.RecordTopicIdentity(msg.Topic(), gatewayID, payload.UniqueID)
			if existenceTracker.MarkKnown(gatewayID, payload.UniqueID) {
				fmt.Printf("[discovery-existence] %s: %s -> known-to-exist\n", gatewayID, payload.UniqueID)
				existenceTracker.ScheduleAggregateStatusPublish(client, cloudClient, ownInstallation, DiscoveryExistenceStatusDebounceDelay)
			}
		}

		var rawPayload map[string]interface{}
		if err := json.Unmarshal(msg.Payload(), &rawPayload); err != nil {
			// Already parsed successfully above (decodeDiscoveryPayload) -- shouldn't happen.
			fmt.Printf("[discovery-bridge] %s: re-parsing payload as a raw map: %v\n", msg.Topic(), err)
			return
		}

		for entityID, link := range discoveryFile.EntityLinks {
			if link.Gateway != gatewayID || link.Leaf != payload.UniqueID {
				continue
			}
			// link.SourceDomain, when set, disambiguates a leaf id published under more than one
			// raw domain (TDiscoveryEntityLink's own doc comment) -- require the incoming
			// message's own topic domain to match too. Absent (the common case): unchanged,
			// matches whichever raw domain publishes the leaf.
			if link.SourceDomain != "" && topicComponent(msg.Topic(), discoveryFile.PhysicalPrefix) != link.SourceDomain {
				continue
			}
			topic, body, ok := buildRelayedDiscoveryConfig(entityID, gatewayID, rawPayload, payload, link, conceptualPrefix, ownInstallation, topicComponent(msg.Topic(), discoveryFile.PhysicalPrefix))
			if !ok {
				continue
			}
			data, err := json.Marshal(body)
			if err != nil {
				fmt.Printf("[discovery-bridge] %s: marshalling relayed payload: %v\n", entityID, err)
				continue
			}
			enqueueDiscoveryRelayJob(relayJobs, TDiscoveryRelayJob{Action: discoveryRelayPublish, Client: client, Broker: "main", Topic: topic, Payload: data})
			fmt.Printf("[discovery-bridge] queued relay of %s (gateway %s, leaf %s) -> %s\n", entityID, gatewayID, payload.UniqueID, topic)
		}
	}

	topicFilter := discoveryFile.PhysicalPrefix + "/+/+/+/config"
	token := client.Subscribe(topicFilter, 0, handler)
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		if err := token.Error(); err != nil {
			return fmt.Errorf("subscribing to %s: %w", topicFilter, err)
		}
		return fmt.Errorf("subscribing to %s: timed out", topicFilter)
	}
	return nil
}
