package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// realEMSESPBoilerOutdoortempPayload is the actual discovery payload captured live from EMS-ESP
// for its "boiler_outdoortemp" sensor (this session's conversation) -- used as a golden fixture
// so decodeDiscoveryPayload is verified against real gateway output, not an idealized shape.
const realEMSESPBoilerOutdoortempPayload = `{"~":"ems-esp","uniq_id":"boiler_outdoortemp","def_ent_id":"sensor.boiler_outdoortemp","name":"Outside temperature","stat_t":"~/boiler_data","val_tpl":"{{value_json['outdoortemp'] if value_json['outdoortemp'] is defined else 0}}","avty":[{"t":"~/boiler_data","val_tpl":"{{'online' if value_json['outdoortemp'] is defined else 'offline'}}"},{"t":"~/status"}],"avty_mode":"all","unit_of_meas":"°C","stat_cla":"measurement","dev_cla":"temperature","o":{"name":"EMS-ESP","sw":"v3.8.4","url":"https://emsesp.org"},"dev":{"ids":["ems-esp-boiler"]}}`

// realEMSESPThermostatLastcodePayload carries via_device instead of a direct root identifier --
// also captured live -- used to verify the one-hop via_device matching path.
const realEMSESPThermostatLastcodePayload = `{"~":"ems-esp","uniq_id":"thermostat_lastcode","def_ent_id":"sensor.thermostat_lastcode","name":"Last error code","stat_t":"~/thermostat_data","val_tpl":"{{value_json['lastcode'] if value_json['lastcode'] is defined else 0}}","avty":[{"t":"~/thermostat_data","val_tpl":"{{'online' if value_json['lastcode'] is defined else 'offline'}}"},{"t":"~/status"}],"avty_mode":"all","o":{"name":"EMS-ESP","sw":"v3.8.4","url":"https://emsesp.org"},"dev":{"ids":["ems-esp-thermostat"],"name":"ems-esp Thermostat","mf":"","mdl":"RC3*0, Moduline 3000/1010H, CW400, Sense II, HPC410","sw":"DeviceID:0x10 ProductID:158 Version:18.02","via_device":"ems-esp"}}`

func TestDecodeDiscoveryPayloadRealEMSESPFixture(t *testing.T) {
	got, err := decodeDiscoveryPayload([]byte(realEMSESPBoilerOutdoortempPayload))
	if err != nil {
		t.Fatalf("decodeDiscoveryPayload error: %v", err)
	}
	if got.UniqueID != "boiler_outdoortemp" {
		t.Errorf("UniqueID = %q, want %q", got.UniqueID, "boiler_outdoortemp")
	}
	// "~" ("ems-esp") must be substituted into "~/boiler_data".
	if got.StateTopic != "ems-esp/boiler_data" {
		t.Errorf("StateTopic = %q, want %q (\"~\" substitution)", got.StateTopic, "ems-esp/boiler_data")
	}
	if got.DeviceClass != "temperature" {
		t.Errorf("DeviceClass = %q, want %q", got.DeviceClass, "temperature")
	}
	if got.Unit != "°C" {
		t.Errorf("Unit = %q, want %q", got.Unit, "°C")
	}
	if got.StateClass != "measurement" {
		t.Errorf("StateClass = %q, want %q", got.StateClass, "measurement")
	}
	wantIDs := []string{"ems-esp-boiler"}
	if len(got.DeviceIdentifiers) != 1 || got.DeviceIdentifiers[0] != wantIDs[0] {
		t.Errorf("DeviceIdentifiers = %v, want %v", got.DeviceIdentifiers, wantIDs)
	}
	if got.ViaDevice != "" {
		t.Errorf("ViaDevice = %q, want \"\" (this device has none)", got.ViaDevice)
	}
}

func TestMatchingGatewayDirectIdentifier(t *testing.T) {
	payload, err := decodeDiscoveryPayload([]byte(realEMSESPBoilerOutdoortempPayload))
	if err != nil {
		t.Fatalf("decodeDiscoveryPayload error: %v", err)
	}
	gateways := map[string]TDiscoveryGateway{
		"discovery.ems_esp": {Identifiers: []string{"ems-esp-boiler"}},
	}
	gatewayID, ok := matchingGateway(payload, gateways)
	if !ok || gatewayID != "discovery.ems_esp" {
		t.Errorf("matchingGateway = (%q, %v), want (\"discovery.ems_esp\", true)", gatewayID, ok)
	}
}

func TestMatchingGatewayOneHopViaDevice(t *testing.T) {
	payload, err := decodeDiscoveryPayload([]byte(realEMSESPThermostatLastcodePayload))
	if err != nil {
		t.Fatalf("decodeDiscoveryPayload error: %v", err)
	}
	if len(payload.DeviceIdentifiers) != 1 || payload.DeviceIdentifiers[0] != "ems-esp-thermostat" {
		t.Fatalf("expected DeviceIdentifiers [ems-esp-thermostat], got %v", payload.DeviceIdentifiers)
	}
	// Gateway is declared with only the ROOT identifier -- must still match via via_device.
	gateways := map[string]TDiscoveryGateway{
		"discovery.ems_esp": {Identifiers: []string{"ems-esp"}},
	}
	gatewayID, ok := matchingGateway(payload, gateways)
	if !ok || gatewayID != "discovery.ems_esp" {
		t.Errorf("matchingGateway (via_device) = (%q, %v), want (\"discovery.ems_esp\", true)", gatewayID, ok)
	}
}

// TestMatchingGatewayIgnoreOtherCapabilitiesSkipsOneHopViaDevice is the regression test for a real
// bug found live 2026-09-21 (Vienna): declaring the Zigbee2MQTT bridge itself as a discovery
// gateway made EVERY other Zigbee2MQTT device match it too via the one-hop via_device rule (every
// device on the network sets its own via_device to the bridge) -- IgnoreOtherCapabilities opts a
// gateway out of that absorption entirely, matching only via a direct Identifiers hit.
func TestMatchingGatewayIgnoreOtherCapabilitiesSkipsOneHopViaDevice(t *testing.T) {
	payload, err := decodeDiscoveryPayload([]byte(realEMSESPThermostatLastcodePayload))
	if err != nil {
		t.Fatalf("decodeDiscoveryPayload error: %v", err)
	}
	gateways := map[string]TDiscoveryGateway{
		"discovery.ems_esp": {Identifiers: []string{"ems-esp"}, IgnoreOtherCapabilities: true},
	}
	if _, ok := matchingGateway(payload, gateways); ok {
		t.Errorf("expected no match: IgnoreOtherCapabilities must suppress the one-hop via_device rule")
	}
}

// TestMatchingGatewayIgnoreOtherCapabilitiesStillMatchesOwnIdentifier confirms the flag only
// disables the one-hop ABSORPTION of other devices -- a gateway's own discovery payload (a direct
// Identifiers hit) must still match normally.
func TestMatchingGatewayIgnoreOtherCapabilitiesStillMatchesOwnIdentifier(t *testing.T) {
	payload, err := decodeDiscoveryPayload([]byte(realEMSESPBoilerOutdoortempPayload))
	if err != nil {
		t.Fatalf("decodeDiscoveryPayload error: %v", err)
	}
	gateways := map[string]TDiscoveryGateway{
		"discovery.ems_esp": {Identifiers: []string{"ems-esp-boiler"}, IgnoreOtherCapabilities: true},
	}
	gatewayID, ok := matchingGateway(payload, gateways)
	if !ok || gatewayID != "discovery.ems_esp" {
		t.Errorf("matchingGateway = (%q, %v), want (\"discovery.ems_esp\", true)", gatewayID, ok)
	}
}

func TestMatchingGatewayNoMatch(t *testing.T) {
	payload, err := decodeDiscoveryPayload([]byte(realEMSESPBoilerOutdoortempPayload))
	if err != nil {
		t.Fatalf("decodeDiscoveryPayload error: %v", err)
	}
	gateways := map[string]TDiscoveryGateway{
		"discovery.other": {Identifiers: []string{"some-other-device"}},
	}
	if _, ok := matchingGateway(payload, gateways); ok {
		t.Errorf("expected no match")
	}
}

// rawPayloadFixture unmarshals a fixture string into the generic map buildRelayedDiscoveryConfig
// now clones from -- the same thing subscribeDiscoveryBridge's own handler does with msg.Payload().
func rawPayloadFixture(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshalling fixture: %v", err)
	}
	return m
}

func TestBuildRelayedDiscoveryConfig(t *testing.T) {
	payload, err := decodeDiscoveryPayload([]byte(realEMSESPBoilerOutdoortempPayload))
	if err != nil {
		t.Fatalf("decodeDiscoveryPayload error: %v", err)
	}
	raw := rawPayloadFixture(t, realEMSESPBoilerOutdoortempPayload)

	topic, body, ok := buildRelayedDiscoveryConfig("sensor.social_garage_door_temperature", "discovery.ems_esp", raw, payload, TDiscoveryEntityLink{}, testPrefix, "test", "sensor")
	if !ok {
		t.Fatalf("expected buildRelayedDiscoveryConfig to succeed")
	}
	wantTopic := "homeassistant/sensor/coordinator/discovery_ems_esp_boiler_outdoortemp/config"
	if topic != wantTopic {
		t.Errorf("topic = %q, want %q", topic, wantTopic)
	}
	if body["state_topic"] != "ems-esp/boiler_data" {
		t.Errorf("state_topic = %v, want %q", body["state_topic"], "ems-esp/boiler_data")
	}
	if body["device_class"] != "temperature" {
		t.Errorf("device_class = %v, want %q", body["device_class"], "temperature")
	}
	if body["unit_of_measurement"] != "°C" {
		t.Errorf("unit_of_measurement = %v, want %q", body["unit_of_measurement"], "°C")
	}
	if body["default_entity_id"] != "sensor.social_garage_door_temperature" {
		t.Errorf("default_entity_id = %v, want %q", body["default_entity_id"], "sensor.social_garage_door_temperature")
	}
	if body["object_id"] != nil {
		t.Errorf("object_id = %v, want unset (not a real MQTT discovery key)", body["object_id"])
	}
	if body["has_entity_name"] != nil {
		t.Errorf("has_entity_name = %v, want unset (not a real MQTT discovery key)", body["has_entity_name"])
	}
	// Regression test, 2026-09-14: HA's Fan integration reads state_value_template, not
	// value_template -- both must be set to the same value so a switch-shaped payload relayed
	// under domain "fan" (or any other domain with its own state-template key name) still works.
	wantTemplate := "{{value_json['outdoortemp'] if value_json['outdoortemp'] is defined else 0}}"
	if body["value_template"] != wantTemplate {
		t.Errorf("value_template = %v, want %q", body["value_template"], wantTemplate)
	}
	if body["state_value_template"] != wantTemplate {
		t.Errorf("state_value_template = %v, want %q (HA's Fan integration reads this key, not value_template)", body["state_value_template"], wantTemplate)
	}
}

// TestWrapMQTTValueTemplate proves the "+20" adjustment example from a "derived ... from <hidden
// raw leaf> via ...;" capability folds correctly around the gateway's own native extraction, and
// guards it on the raw key's own presence -- see wrapMQTTValueTemplate's own doc comment for the
// real "pressure missing from a partial Zigbee2MQTT payload" bug this is a regression test for.
func TestWrapMQTTValueTemplate(t *testing.T) {
	got := wrapMQTTValueTemplate(`{{ value_json["pressure"] }}`, "($ | float(0)) + 20")
	want := `{% if "pressure" in value_json %}{{ ((value_json["pressure"]) | float(0)) + 20 }}{% else %}{{ this.state }}{% endif %}`
	if got != want {
		t.Errorf("wrapMQTTValueTemplate = %q, want %q", got, want)
	}
}

// TestWrapMQTTValueTemplateFallsBackForUnrecognisedInnerShape confirms an inner template that
// ISN'T a plain value_json["<key>"] access (single quotes are also recognised; anything else isn't)
// still wraps as before -- there's no key to guard existence on, so guessing one would be worse
// than the old, unguarded behaviour.
func TestWrapMQTTValueTemplateFallsBackForUnrecognisedInnerShape(t *testing.T) {
	got := wrapMQTTValueTemplate(`{{ value_json["pressure"] | round(1) }}`, "($ | float(0)) + 20")
	want := `{{ ((value_json["pressure"] | round(1)) | float(0)) + 20 }}`
	if got != want {
		t.Errorf("wrapMQTTValueTemplate = %q, want %q", got, want)
	}
}

// TestWrapMQTTValueTemplateRecognisesSingleQuotedKey confirms the single-quote form Z2M sometimes
// uses (value_json['pressure']) is recognised too, not just double quotes.
func TestWrapMQTTValueTemplateRecognisesSingleQuotedKey(t *testing.T) {
	got := wrapMQTTValueTemplate(`{{ value_json['pressure'] }}`, "($ | float(0)) + 20")
	want := `{% if "pressure" in value_json %}{{ ((value_json['pressure']) | float(0)) + 20 }}{% else %}{{ this.state }}{% endif %}`
	if got != want {
		t.Errorf("wrapMQTTValueTemplate = %q, want %q", got, want)
	}
}

// TestBuildRelayedDiscoveryConfigAppliesValueTemplateWrap proves link.ValueTemplateWrap (set only
// for a "derived ... from <hidden raw leaf> via ...;" capability) is folded into BOTH
// value_template and state_value_template, on top of the gateway's own native extraction.
func TestBuildRelayedDiscoveryConfigAppliesValueTemplateWrap(t *testing.T) {
	payload := tDecodedDiscoveryPayload{
		UniqueID:      "door_aqara_multi_pressure",
		StateTopic:    "zigbee2mqtt/apartment/hallway/door/aqara_multi",
		ValueTemplate: `{{ value_json["pressure"] }}`,
	}
	raw := map[string]interface{}{
		"unique_id":      "door_aqara_multi_pressure",
		"state_topic":    "zigbee2mqtt/apartment/hallway/door/aqara_multi",
		"value_template": `{{ value_json["pressure"] }}`,
	}
	link := TDiscoveryEntityLink{ValueTemplateWrap: "($ | float(0)) + 20"}

	_, body, ok := buildRelayedDiscoveryConfig("sensor.physical_apartment_hallway_door_aqara_multi_pressure", "discovery.hallway_door_aqara_multi", raw, payload, link, testPrefix, "test", "sensor")
	if !ok {
		t.Fatalf("expected buildRelayedDiscoveryConfig to succeed")
	}
	want := `{% if "pressure" in value_json %}{{ ((value_json["pressure"]) | float(0)) + 20 }}{% else %}{{ this.state }}{% endif %}`
	if body["value_template"] != want {
		t.Errorf("value_template = %v, want %q", body["value_template"], want)
	}
	if body["state_value_template"] != want {
		t.Errorf("state_value_template = %v, want %q", body["state_value_template"], want)
	}
	// payload_on/payload_off are a binary_sensor-only MQTT schema concept -- a wrapped sensor
	// relay (this test) must never carry them.
	if _, present := body["payload_on"]; present {
		t.Errorf("payload_on = %v, want absent -- this is a sensor-domain relay, not binary_sensor", body["payload_on"])
	}
	if _, present := body["payload_off"]; present {
		t.Errorf("payload_off = %v, want absent -- this is a sensor-domain relay, not binary_sensor", body["payload_off"])
	}
}

// TestBuildRelayedDiscoveryConfigDefaultsGapFillOnly is a regression test: Defaults.def-resolved
// typing (link) must only fill a field the gateway's own native payload left empty, never
// override a value the gateway already reported -- and icon, which the decoded payload never
// carries at all, must always come from link.
func TestBuildRelayedDiscoveryConfigDefaultsGapFillOnly(t *testing.T) {
	// A synthetic payload reporting device_class/unit but leaving state_class empty -- gives
	// full control over which fields are "native" vs. genuinely blank, unlike the real EMS-ESP
	// fixture (which happens to report all three).
	payload := tDecodedDiscoveryPayload{
		UniqueID:    "boiler_outdoortemp",
		StateTopic:  "ems-esp/boiler_data",
		DeviceClass: "temperature",
		Unit:        "°C",
	}
	raw := map[string]interface{}{
		"unique_id":           "boiler_outdoortemp",
		"state_topic":         "ems-esp/boiler_data",
		"device_class":        "temperature",
		"unit_of_measurement": "°C",
	}
	// device_class/unit are already native -- link's conflicting values must NOT win.
	// state_class the payload leaves empty, so link's value must gap-fill it. icon has no
	// native counterpart at all, so it always comes from link.
	link := TDiscoveryEntityLink{
		DeviceClass: "should-not-win",
		Unit:        "should-not-win",
		StateClass:  "measurement",
		Icon:        "mdi:thermometer",
	}

	_, body, ok := buildRelayedDiscoveryConfig("sensor.social_garage_door_temperature", "discovery.ems_esp", raw, payload, link, testPrefix, "test", "sensor")
	if !ok {
		t.Fatalf("expected buildRelayedDiscoveryConfig to succeed")
	}
	if body["device_class"] != "temperature" {
		t.Errorf("device_class = %v, want the gateway's own \"temperature\", not link's override", body["device_class"])
	}
	if body["unit_of_measurement"] != "°C" {
		t.Errorf("unit_of_measurement = %v, want the gateway's own \"°C\", not link's override", body["unit_of_measurement"])
	}
	if body["state_class"] != "measurement" {
		t.Errorf("state_class = %v, want link's gap-filled \"measurement\" (the gateway reported none)", body["state_class"])
	}
	if body["icon"] != "mdi:thermometer" {
		t.Errorf("icon = %v, want link's \"mdi:thermometer\" (no native counterpart exists)", body["icon"])
	}
}

// TestBuildRelayedDiscoveryConfigStripsSensorOnlyFieldsOnDomainMismatch is the regression test for
// TWO real bugs found live 2026-09-25 (Junglinster's furnace "consumes" binary_sensor -- the first
// real use of a "hidden" raw sensor leaf relayed, via a value_template wrap, into a binary_sensor-
// domain "derived" capability):
//  1. Zigbee2MQTT's own native discovery for the raw power leaf gives it device_class "power"/unit
//     "W"/state_class "measurement" -- wholesale-copied across the domain change (sensor -> binary_
//     sensor) in TWO independent places (the rawPayload->body map copy, and payload's own decoded
//     DeviceClass/Unit/StateClass fields, both parsed from the same raw JSON) -- unit_of_measurement/
//     state_class aren't even valid MQTT binary_sensor schema fields, and left the entity's own
//     registry entry permanently unavailable.
//  2. Once available, the entity still showed "unknown" forever: every on/off jinja macro/formula
//     this codebase writes (${int_more_then}/${int_less_then}, or a hand-written "'on' if ... else
//     'off'") renders lowercase "on"/"off" -- correct for HA's lenient TEMPLATE platform, but MQTT's
//     binary_sensor schema does an exact, case-sensitive match against payload_on/payload_off, which
//     default to "ON"/"OFF" (uppercase) when unset (as they always were for a wrapped relay before
//     this fix) -- messages arrived and rendered fine, HA just had nothing to recognise as a state.
//
// Same-domain, unwrapped relays (every other test in this file) must keep working unchanged.
func TestBuildRelayedDiscoveryConfigStripsSensorOnlyFieldsOnDomainMismatch(t *testing.T) {
	raw := map[string]interface{}{
		"unique_id":           "0xa4c1386da42c223a_power_b_zigbee2mqtt",
		"state_topic":         "zigbee2mqtt/house/laundry_kitchen/heating/power_meter",
		"value_template":      `{{ value_json["power_b"] }}`,
		"device_class":        "power",
		"unit_of_measurement": "W",
		"state_class":         "measurement",
	}
	payload := tDecodedDiscoveryPayload{
		UniqueID:      "0xa4c1386da42c223a_power_b_zigbee2mqtt",
		StateTopic:    "zigbee2mqtt/house/laundry_kitchen/heating/power_meter",
		ValueTemplate: `{{ value_json["power_b"] }}`,
		DeviceClass:   "power",
		Unit:          "W",
		StateClass:    "measurement",
	}
	link := TDiscoveryEntityLink{ValueTemplateWrap: "'on' if (($ | int(0)) > 50) else 'off'"}

	_, body, ok := buildRelayedDiscoveryConfig("binary_sensor.social_house_kitchen_workplace_furnace_consumes", "sensors.house_heating_power_meter", raw, payload, link, testPrefix, "test", "sensor")
	if !ok {
		t.Fatalf("expected buildRelayedDiscoveryConfig to succeed")
	}
	if _, present := body["device_class"]; present {
		t.Errorf("device_class = %v, want it stripped -- \"power\" is a sensor-domain reading, not this binary_sensor's own device class", body["device_class"])
	}
	if _, present := body["unit_of_measurement"]; present {
		t.Errorf("unit_of_measurement = %v, want it stripped -- not a valid MQTT binary_sensor schema field at all", body["unit_of_measurement"])
	}
	if _, present := body["state_class"]; present {
		t.Errorf("state_class = %v, want it stripped -- not a valid MQTT binary_sensor schema field at all", body["state_class"])
	}
	if body["payload_on"] != "on" {
		t.Errorf("payload_on = %v, want \"on\" -- must match what the wrap's own jinja formula actually renders, not MQTT's uppercase \"ON\" default", body["payload_on"])
	}
	if body["payload_off"] != "off" {
		t.Errorf("payload_off = %v, want \"off\" -- must match what the wrap's own jinja formula actually renders, not MQTT's uppercase \"OFF\" default", body["payload_off"])
	}
}

// realZigbee2MQTTVidjaLeft1Payload is a real Zigbee2MQTT light discovery payload (captured live,
// this session, Vienna's "vidja/left/1" IKEA bulb) -- used to prove buildRelayedDiscoveryConfig
// preserves everything a real controllable light needs (command_topic, brightness,
// supported_color_modes, effect_list, schema, device block), not just what EMS-ESP's read-only
// sensors happened to need. See that function's own doc comment for the real gap this closes.
const realZigbee2MQTTVidjaLeft1Payload = `{"availability":[{"topic":"zigbee2mqtt/bridge/state","value_template":"{{ value_json.state }}"},{"topic":"zigbee2mqtt/apartment/living_room/vidja/left_1/availability","value_template":"{{ value_json.state }}"}],"availability_mode":"all","brightness":true,"brightness_scale":254,"command_topic":"zigbee2mqtt/apartment/living_room/vidja/left_1/set","default_entity_id":"light.apartment/living_room/vidja/left/1","device":{"hw_version":1,"identifiers":["zigbee2mqtt_0x84b4dbfffefbb43a"],"manufacturer":"IKEA","model":"TRADFRI bulb E12/E14/E17, white spectrum, candle, opal, 450/470/440 lm","model_id":"LED1949C5","name":"apartment/living_room/vidja/left/1","sw_version":"1.1.20","via_device":"zigbee2mqtt_bridge_0x00124b0029e8c409"},"effect":true,"effect_list":["blink","breathe","okay","channel_change","finish_effect","stop_effect"],"max_mireds":454,"min_mireds":250,"name":null,"object_id":"apartment/living_room/vidja/left/1","origin":{"name":"Zigbee2MQTT","sw":"2.14.1","url":"https://www.zigbee2mqtt.io"},"schema":"json","state_topic":"zigbee2mqtt/apartment/living_room/vidja/left_1","supported_color_modes":["color_temp"],"unique_id":"0x84b4dbfffefbb43a_light_zigbee2mqtt"}`

func TestBuildRelayedDiscoveryConfigPreservesLightCapabilities(t *testing.T) {
	payload, err := decodeDiscoveryPayload([]byte(realZigbee2MQTTVidjaLeft1Payload))
	if err != nil {
		t.Fatalf("decodeDiscoveryPayload error: %v", err)
	}
	raw := rawPayloadFixture(t, realZigbee2MQTTVidjaLeft1Payload)

	_, body, ok := buildRelayedDiscoveryConfig("light.physical_apartment_living_room_vidja_left_1", "discovery.vidja_left_1", raw, payload, TDiscoveryEntityLink{}, testPrefix, "test", "light")
	if !ok {
		t.Fatalf("expected buildRelayedDiscoveryConfig to succeed")
	}

	// The whole point of this test: command_topic must survive, or the light is uncontrollable.
	if body["command_topic"] != "zigbee2mqtt/apartment/living_room/vidja/left_1/set" {
		t.Errorf("command_topic = %v, want the gateway's own command_topic preserved", body["command_topic"])
	}
	if body["brightness"] != true {
		t.Errorf("brightness = %v, want true (preserved from the gateway's own payload)", body["brightness"])
	}
	if body["schema"] != "json" {
		t.Errorf("schema = %v, want \"json\" (preserved)", body["schema"])
	}
	colorModes, ok := body["supported_color_modes"].([]interface{})
	if !ok || len(colorModes) != 1 || colorModes[0] != "color_temp" {
		t.Errorf("supported_color_modes = %v, want [\"color_temp\"] preserved", body["supported_color_modes"])
	}
	effects, ok := body["effect_list"].([]interface{})
	if !ok || len(effects) != 6 {
		t.Errorf("effect_list = %v, want the gateway's own 6-entry list preserved", body["effect_list"])
	}
	if _, ok := body["device"].(map[string]interface{}); !ok {
		t.Errorf("device = %v, want the gateway's own device block preserved", body["device"])
	}

	// The fields buildRelayedDiscoveryConfig IS meant to override.
	if body["default_entity_id"] != "light.physical_apartment_living_room_vidja_left_1" {
		t.Errorf("default_entity_id = %v, want our own conceptual entity id", body["default_entity_id"])
	}
	if body["unique_id"] != "discovery_vidja_left_1_0x84b4dbfffefbb43a_light_zigbee2mqtt" {
		t.Errorf("unique_id = %v, want the gateway+leaf stable id", body["unique_id"])
	}

	// No abbreviated-key leftovers from overriding the long form.
	for _, pair := range discoveryAbbreviatedKeyPairs {
		if _, present := body[pair[0]]; present {
			t.Errorf("body still has short-form key %q alongside its long-form override", pair[0])
		}
	}
}

func TestBuildRelayedDiscoveryConfigFailsWithoutUniqueID(t *testing.T) {
	_, _, ok := buildRelayedDiscoveryConfig("sensor.social_garage_door_temperature", "discovery.ems_esp", nil, tDecodedDiscoveryPayload{}, TDiscoveryEntityLink{}, testPrefix, "test", "sensor")
	if ok {
		t.Errorf("expected failure when the decoded payload has no unique_id")
	}
}

// TestBuildRelayedDiscoveryConfigSucceedsWithoutStateTopic is a regression test for a real bug
// found live 2026-09-15 migrating Vienna's first "climate" domain device: HA's MQTT Climate
// discovery schema has no top-level "state_topic" key at all (it uses mode_state_topic/
// temperature_state_topic/current_temperature_topic/action_topic instead), and
// buildRelayedDiscoveryConfig used to require StateTopic unconditionally, silently dropping every
// climate-domain relay. A payload with a unique_id but no state_topic must still relay -- with no
// fabricated "state_topic" key added, and every one of the climate schema's own per-feature topic
// keys passed through untouched by the wholesale rawPayload clone.
func TestBuildRelayedDiscoveryConfigSucceedsWithoutStateTopic(t *testing.T) {
	const climatePayload = `{"unique_id":"0x000d6f0018c164d1_climate_zigbee2mqtt","mode_state_topic":"zigbee2mqtt/apartment/living_room/bitron/thermostat","mode_state_template":"{{ value_json[\"system_mode\"] }}","mode_command_topic":"zigbee2mqtt/apartment/living_room/bitron/thermostat/set/system_mode","temperature_state_topic":"zigbee2mqtt/apartment/living_room/bitron/thermostat","temperature_command_topic":"zigbee2mqtt/apartment/living_room/bitron/thermostat/set/occupied_heating_setpoint","current_temperature_topic":"zigbee2mqtt/apartment/living_room/bitron/thermostat","current_temperature_template":"{{ value_json[\"local_temperature\"] }}","modes":["heat","off"]}`

	payload, err := decodeDiscoveryPayload([]byte(climatePayload))
	if err != nil {
		t.Fatalf("decodeDiscoveryPayload error: %v", err)
	}
	if payload.StateTopic != "" {
		t.Fatalf("fixture must have no state_topic, got %q", payload.StateTopic)
	}
	raw := rawPayloadFixture(t, climatePayload)

	topic, body, ok := buildRelayedDiscoveryConfig("climate.physical_apartment_living_room_bitron_thermostat", "discovery.living_room_bitron_thermostat", raw, payload, TDiscoveryEntityLink{}, testPrefix, "test", "climate")
	if !ok {
		t.Fatalf("expected buildRelayedDiscoveryConfig to succeed for a payload with no state_topic")
	}
	wantTopic := "homeassistant/climate/coordinator/discovery_living_room_bitron_thermostat_0x000d6f0018c164d1_climate_zigbee2mqtt/config"
	if topic != wantTopic {
		t.Errorf("topic = %q, want %q", topic, wantTopic)
	}
	if _, present := body["state_topic"]; present {
		t.Errorf("state_topic = %v, want unset (climate has no such key)", body["state_topic"])
	}
	if body["mode_state_topic"] != "zigbee2mqtt/apartment/living_room/bitron/thermostat" {
		t.Errorf("mode_state_topic = %v, want passthrough of the gateway's own value", body["mode_state_topic"])
	}
	if body["temperature_command_topic"] != "zigbee2mqtt/apartment/living_room/bitron/thermostat/set/occupied_heating_setpoint" {
		t.Errorf("temperature_command_topic = %v, want passthrough of the gateway's own value", body["temperature_command_topic"])
	}
	if body["default_entity_id"] != "climate.physical_apartment_living_room_bitron_thermostat" {
		t.Errorf("default_entity_id = %v, want %q", body["default_entity_id"], "climate.physical_apartment_living_room_bitron_thermostat")
	}
}

// TestSubscribeDiscoveryBridgeMarksExistence is PROJECT.md 1.8's own coverage: a real gateway
// discovery payload arriving must mark the matched (gateway, leaf) known-to-exist, alongside the
// pre-existing relay behaviour. The resulting status publish itself is debounced
// (ScheduleAggregateStatusPublish, DiscoveryExistenceStatusDebounceDelay) rather than synchronous,
// so it's covered directly by discovery_existence_test.go's own ScheduleAggregateStatusPublish
// coverage (a short delay there, unlike production's real 30s one) rather than here.
func TestSubscribeDiscoveryBridgeMarksExistence(t *testing.T) {
	discoveryFile := TDiscoveryFile{
		PhysicalPrefix: "physical",
		Gateways: map[string]TDiscoveryGateway{
			"discovery.ems_esp": {Identifiers: []string{"ems-esp-boiler"}},
		},
		EntityLinks: map[string]TDiscoveryEntityLink{
			"sensor.social_garage_door_temperature": {Gateway: "discovery.ems_esp", Leaf: "boiler_outdoortemp"},
		},
	}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	tracker := newDiscoveryExistenceTracker("")
	jobs := make(chan TDiscoveryRelayJob, 100)

	client := &fakeClient{}
	if err := subscribeDiscoveryBridge(client, nil, "junglinster", discoveryFile, publisher, testPrefix, tracker, nil, nil, jobs); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}
	client.subscribedHandlers[0](client, fakeMessage{
		topic:   "physical/sensor/ems-esp/boiler_outdoortemp/config",
		payload: []byte(realEMSESPBoilerOutdoortempPayload),
	})
	drainDiscoveryRelayQueue(jobs, publisher)

	if got := tracker.AggregateSnapshot()["discovery.ems_esp"]["boiler_outdoortemp"]; got != StatusKnownToExist {
		t.Errorf("status = %q, want %q", got, StatusKnownToExist)
	}
}

// realMoesActionSensorPayload and realMoesActionEventPayload are Zigbee2MQTT's own two discovery
// messages for a Moes scene remote's "action" property -- a real case found live 2026-09-15
// (Vienna): the SAME unique_id is published under both the "sensor" (legacy text mirror) and
// "event" (richer, structured) raw domains for the one underlying property.
const realMoesActionSensorPayload = `{"availability":[{"topic":"zigbee2mqtt/bridge/state","value_template":"{{ value_json.state }}"}],"availability_mode":"all","device":{"identifiers":["zigbee2mqtt_0x70ac08fffe4f0c15"]},"object_id":"apartment/hallway/door/moes_action","origin":{"name":"Zigbee2MQTT"},"state_topic":"zigbee2mqtt/apartment/hallway/door/moes","unique_id":"0x70ac08fffe4f0c15_action_zigbee2mqtt","value_template":"{{ value_json[\"action\"] }}"}`
const realMoesActionEventPayload = `{"availability":[{"topic":"zigbee2mqtt/bridge/state","value_template":"{{ value_json.state }}"}],"availability_mode":"all","device":{"identifiers":["zigbee2mqtt_0x70ac08fffe4f0c15"]},"event_types":["1_single","1_double","hold"],"object_id":"apartment/hallway/door/moes_action","origin":{"name":"Zigbee2MQTT"},"state_topic":"zigbee2mqtt/apartment/hallway/door/moes","unique_id":"0x70ac08fffe4f0c15_action_zigbee2mqtt"}`

// TestSubscribeDiscoveryBridgeSourceDomainDisambiguatesSharedLeaf is the regression test for the
// real gap above: without TDiscoveryEntityLink.SourceDomain, matching on (gateway, leaf) alone
// would let EITHER raw message relay under the SAME target entity, non-deterministically depending
// on retained-message delivery order. With SourceDomain "event" declared, only the "event"-domain
// topic relays; the "sensor"-domain one (same gateway, same leaf) must not.
func TestSubscribeDiscoveryBridgeSourceDomainDisambiguatesSharedLeaf(t *testing.T) {
	discoveryFile := TDiscoveryFile{
		PhysicalPrefix: "physical",
		Gateways: map[string]TDiscoveryGateway{
			"discovery.hallway_door_moes": {Identifiers: []string{"zigbee2mqtt_0x70ac08fffe4f0c15"}},
		},
		EntityLinks: map[string]TDiscoveryEntityLink{
			"event.physical_apartment_hallway_door_moes_core": {
				Gateway:      "discovery.hallway_door_moes",
				Leaf:         "0x70ac08fffe4f0c15_action_zigbee2mqtt",
				SourceDomain: "event",
			},
		},
	}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	jobs := make(chan TDiscoveryRelayJob, 100)

	client := &fakeClient{}
	if err := subscribeDiscoveryBridge(client, nil, "vienna", discoveryFile, publisher, testPrefix, nil, nil, nil, jobs); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}

	wantTopic := "homeassistant/event/coordinator/discovery_hallway_door_moes_0x70ac08fffe4f0c15_action_zigbee2mqtt/config"

	// The "sensor"-domain message must NOT relay under the "event"-domain target entity.
	client.subscribedHandlers[0](client, fakeMessage{
		topic:   "physical/sensor/0x70ac08fffe4f0c15/action/config",
		payload: []byte(realMoesActionSensorPayload),
	})
	drainDiscoveryRelayQueue(jobs, publisher)
	if _, found := findPublish(client.published, wantTopic); found {
		t.Fatalf("sensor-domain message must not relay to the event-domain target, but it did: %+v", client.published)
	}

	// The "event"-domain message (same gateway, same leaf) must relay.
	client.subscribedHandlers[0](client, fakeMessage{
		topic:   "physical/event/0x70ac08fffe4f0c15/action/config",
		payload: []byte(realMoesActionEventPayload),
	})
	drainDiscoveryRelayQueue(jobs, publisher)
	publish, found := findPublish(client.published, wantTopic)
	if !found {
		t.Fatalf("expected the event-domain message to relay to %s, got %+v", wantTopic, client.published)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(publish.payload, &body); err != nil {
		t.Fatalf("unmarshal relayed payload: %v", err)
	}
	if body["event_types"] == nil {
		t.Errorf("relayed payload missing event_types -- got the wrong raw message: %s", publish.payload)
	}
}

// TestSubscribeDiscoveryBridgeRetractsOnEmptyPayload confirms an empty (retracted) discovery
// payload on a topic the handler previously saw a real payload on moves that leaf to
// known-not-to-exist -- PROJECT.md 1.8's retraction half. The resulting status publish itself is
// debounced (ScheduleAggregateStatusPublish, DiscoveryExistenceStatusDebounceDelay) rather than
// synchronous, so it's covered directly by discovery_existence_test.go's own
// ScheduleAggregateStatusPublish/PublishAggregateStatus coverage rather than here.
func TestSubscribeDiscoveryBridgeRetractsOnEmptyPayload(t *testing.T) {
	discoveryFile := TDiscoveryFile{
		PhysicalPrefix: "physical",
		Gateways: map[string]TDiscoveryGateway{
			"discovery.ems_esp": {Identifiers: []string{"ems-esp-boiler"}},
		},
		EntityLinks: map[string]TDiscoveryEntityLink{
			"sensor.social_garage_door_temperature": {Gateway: "discovery.ems_esp", Leaf: "boiler_outdoortemp"},
		},
	}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	tracker := newDiscoveryExistenceTracker("")
	jobs := make(chan TDiscoveryRelayJob, 100)

	client := &fakeClient{}
	if err := subscribeDiscoveryBridge(client, nil, "junglinster", discoveryFile, publisher, testPrefix, tracker, nil, nil, jobs); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}
	topic := "physical/sensor/ems-esp/boiler_outdoortemp/config"
	client.subscribedHandlers[0](client, fakeMessage{topic: topic, payload: []byte(realEMSESPBoilerOutdoortempPayload)})
	drainDiscoveryRelayQueue(jobs, publisher)
	if got := tracker.AggregateSnapshot()["discovery.ems_esp"]["boiler_outdoortemp"]; got != StatusKnownToExist {
		t.Fatalf("precondition failed: status = %q, want %q", got, StatusKnownToExist)
	}

	client.subscribedHandlers[0](client, fakeMessage{topic: topic, payload: []byte{}})
	drainDiscoveryRelayQueue(jobs, publisher)

	if got := tracker.AggregateSnapshot()["discovery.ems_esp"]["boiler_outdoortemp"]; got != StatusKnownNotToExist {
		t.Errorf("status after retraction = %q, want %q", got, StatusKnownNotToExist)
	}
}

// TestSubscribeDiscoveryBridgeToleratesNilExistenceTracker confirms the pre-existing relay-only
// behaviour still works when no tracker is supplied (nil is an accepted, deliberate no-op).
func TestSubscribeDiscoveryBridgeToleratesNilExistenceTracker(t *testing.T) {
	discoveryFile := TDiscoveryFile{
		PhysicalPrefix: "physical",
		Gateways: map[string]TDiscoveryGateway{
			"discovery.ems_esp": {Identifiers: []string{"ems-esp-boiler"}},
		},
		EntityLinks: map[string]TDiscoveryEntityLink{
			"sensor.social_garage_door_temperature": {Gateway: "discovery.ems_esp", Leaf: "boiler_outdoortemp"},
		},
	}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	jobs := make(chan TDiscoveryRelayJob, 100)

	client := &fakeClient{}
	if err := subscribeDiscoveryBridge(client, nil, "junglinster", discoveryFile, publisher, testPrefix, nil, nil, nil, jobs); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}
	client.subscribedHandlers[0](client, fakeMessage{
		topic:   "physical/sensor/ems-esp/boiler_outdoortemp/config",
		payload: []byte(realEMSESPBoilerOutdoortempPayload),
	})
	drainDiscoveryRelayQueue(jobs, publisher)

	if _, found := findPublish(client.published, "homeassistant/sensor/coordinator/discovery_ems_esp_boiler_outdoortemp/config"); !found {
		t.Errorf("expected the relayed discovery config publish to still happen, got %+v", client.published)
	}
}

// realZigbee2MQTTAqaraMultiPayload is a representative (not live-captured) Zigbee2MQTT discovery
// payload -- device not declared as any "discovery" gateway, state_topic under "zigbee2mqtt/",
// the shape PROJECT.md item 7's passthrough is meant to relay untouched during a gradual
// device-by-device migration.
const realZigbee2MQTTAqaraMultiPayload = `{"availability":[{"topic":"zigbee2mqtt/bridge/state"}],"device":{"identifiers":["zigbee2mqtt_0x00158d0001aabbcc"],"manufacturer":"Xiaomi","model":"Aqara","name":"aqara_multi"},"device_class":"temperature","state_topic":"zigbee2mqtt/aqara_multi","unique_id":"0x00158d0001aabbcc_temperature_zigbee2mqtt","unit_of_measurement":"°C"}`

func discoveryFileWithPassthrough(gateways map[string]TDiscoveryGateway) TDiscoveryFile {
	return TDiscoveryFile{PhysicalPrefix: "physical", Gateways: gateways}
}

var zigbee2mqttPassthroughRules = []TDiscoveryPassthroughRule{
	{TopicPrefix: "zigbee2mqtt/", SourcePrefix: "physical"},
}

// TestSubscribeDiscoveryBridgePassesThroughUnmatchedDevice is PROJECT.md item 7's own core case:
// a device matching no declared gateway, whose state_topic starts with a declared passthrough
// prefix, gets relayed byte-for-byte (same topic tail, only the leading prefix segment swapped)
// rather than dropped the way it was before this item existed.
func TestSubscribeDiscoveryBridgePassesThroughUnmatchedDevice(t *testing.T) {
	discoveryFile := discoveryFileWithPassthrough(nil)
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	jobs := make(chan TDiscoveryRelayJob, 100)

	client := &fakeClient{}
	if err := subscribeDiscoveryBridge(client, nil, "junglinster", discoveryFile, publisher, testPrefix, nil, zigbee2mqttPassthroughRules, nil, jobs); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}
	sourceTopic := "physical/sensor/zigbee2mqtt/0x00158d0001aabbcc_temperature/config"
	client.subscribedHandlers[0](client, fakeMessage{topic: sourceTopic, payload: []byte(realZigbee2MQTTAqaraMultiPayload)})
	drainDiscoveryRelayQueue(jobs, publisher)

	wantTopic := "homeassistant/sensor/zigbee2mqtt/0x00158d0001aabbcc_temperature/config"
	publish, found := findPublish(client.published, wantTopic)
	if !found {
		t.Fatalf("expected a passthrough publish on %s, got %+v", wantTopic, client.published)
	}
	if string(publish.payload) != realZigbee2MQTTAqaraMultiPayload {
		t.Errorf("passthrough payload = %s, want the raw upstream payload byte-for-byte", publish.payload)
	}
}

// TestSubscribeDiscoveryBridgePassthroughSurvivesRetireMissing is the end-to-end regression test
// for the real incident this session's RetireMissing fix exists for (2026-09-14, PROJECT.md item
// 4): a passthrough-relayed device's topic must still be there after a simulated coordinator
// restart's RetireMissing sweep, even though -- by definition -- no generator-computed expected set
// can ever contain it (that's exactly why it's still on passthrough, not a declared gateway).
// Before the fix, this exact sequence deleted the topic every time.
func TestSubscribeDiscoveryBridgePassthroughSurvivesRetireMissing(t *testing.T) {
	discoveryFile := discoveryFileWithPassthrough(nil)
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	jobs := make(chan TDiscoveryRelayJob, 100)

	client := &fakeClient{}
	if err := subscribeDiscoveryBridge(client, nil, "junglinster", discoveryFile, publisher, testPrefix, nil, zigbee2mqttPassthroughRules, nil, jobs); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}
	sourceTopic := "physical/sensor/zigbee2mqtt/0x00158d0001aabbcc_temperature/config"
	client.subscribedHandlers[0](client, fakeMessage{topic: sourceTopic, payload: []byte(realZigbee2MQTTAqaraMultiPayload)})
	drainDiscoveryRelayQueue(jobs, publisher)

	wantTopic := "homeassistant/sensor/zigbee2mqtt/0x00158d0001aabbcc_temperature/config"
	if _, found := findPublish(client.published, wantTopic); !found {
		t.Fatalf("expected the passthrough publish to happen before the RetireMissing sweep")
	}

	// Simulate the next coordinator restart: an empty expected set, exactly as it would be for a
	// house with no declared gateways at all yet.
	retired := publisher.RetireMissing(client, "main", map[string]bool{}, jobs)
	if len(retired) != 0 {
		t.Errorf("RetireMissing retired %v, want nothing -- passthrough-relayed topics must survive", retired)
	}
	drainDiscoveryRelayQueue(jobs, publisher)
}

// TestSubscribeDiscoveryBridgeDoesNotPassThroughNonMatchingPrefix confirms a device that matches
// no gateway AND no declared passthrough prefix is still simply ignored, exactly as before this
// item existed -- passthrough only widens what gets relayed, never relays everything on the
// physical prefix indiscriminately.
func TestSubscribeDiscoveryBridgeDoesNotPassThroughNonMatchingPrefix(t *testing.T) {
	discoveryFile := discoveryFileWithPassthrough(nil)
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	rules := []TDiscoveryPassthroughRule{{TopicPrefix: "zwave/", SourcePrefix: "physical"}}
	jobs := make(chan TDiscoveryRelayJob, 100)

	client := &fakeClient{}
	if err := subscribeDiscoveryBridge(client, nil, "junglinster", discoveryFile, publisher, testPrefix, nil, rules, nil, jobs); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}
	client.subscribedHandlers[0](client, fakeMessage{
		topic:   "physical/sensor/zigbee2mqtt/0x00158d0001aabbcc_temperature/config",
		payload: []byte(realZigbee2MQTTAqaraMultiPayload),
	})
	drainDiscoveryRelayQueue(jobs, publisher)

	if len(client.published) != 0 {
		t.Errorf("expected no publish at all, got %+v", client.published)
	}
}

// TestSubscribeDiscoveryBridgeSkipsPassthroughForDeclaredGateway confirms a device matching a
// declared gateway never ALSO gets a raw passthrough copy, even if its state_topic happens to
// match a declared passthrough prefix too -- otherwise HA would show the same physical device
// twice once it's migrated.
func TestSubscribeDiscoveryBridgeSkipsPassthroughForDeclaredGateway(t *testing.T) {
	discoveryFile := discoveryFileWithPassthrough(map[string]TDiscoveryGateway{
		"discovery.aqara_multi": {Identifiers: []string{"zigbee2mqtt_0x00158d0001aabbcc"}},
	})
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	jobs := make(chan TDiscoveryRelayJob, 100)

	client := &fakeClient{}
	if err := subscribeDiscoveryBridge(client, nil, "junglinster", discoveryFile, publisher, testPrefix, nil, zigbee2mqttPassthroughRules, nil, jobs); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}
	client.subscribedHandlers[0](client, fakeMessage{
		topic:   "physical/sensor/zigbee2mqtt/0x00158d0001aabbcc_temperature/config",
		payload: []byte(realZigbee2MQTTAqaraMultiPayload),
	})
	drainDiscoveryRelayQueue(jobs, publisher)

	rawTopic := "homeassistant/sensor/zigbee2mqtt/0x00158d0001aabbcc_temperature/config"
	if _, found := findPublish(client.published, rawTopic); found {
		t.Errorf("expected no raw passthrough publish for a device matching a declared gateway, got %+v", client.published)
	}
}

// TestSubscribeDiscoveryBridgeSuppressesPassthroughOnDefaultEntityIDCollision is the regression
// test for the real bug found live 2026-09-20: Junglinster's "xanadu" smart plug was renamed in
// Zigbee2MQTT, but its OLD stale retained discovery payload never got refreshed and kept claiming
// the same "default_entity_id" a DIFFERENT, currently-correct device had since taken over. Both
// used to get relayed byte-for-byte, and HA silently suffixed one of them. The second (different-
// device) claimant must now be suppressed, not relayed.
func TestSubscribeDiscoveryBridgeSuppressesPassthroughOnDefaultEntityIDCollision(t *testing.T) {
	const firstPayload = `{"availability":[{"topic":"zigbee2mqtt/bridge/state"}],"device":{"identifiers":["zigbee2mqtt_0xa4c138a4716e0c02"],"manufacturer":"Innr","model":"Smart plug (EU)","name":"house/server_room/xanadu"},"default_entity_id":"switch.house/server_room/xanadu","state_topic":"zigbee2mqtt/house/server_room/xanadu","unique_id":"0xa4c138a4716e0c02_switch_zigbee2mqtt"}`
	const stalePayload = `{"availability":[{"topic":"zigbee2mqtt/bridge/state"}],"device":{"identifiers":["zigbee2mqtt_0xc4988600000f73e3"],"manufacturer":"Innr","model":"Smart plug","name":"house/server_room/xanadu"},"default_entity_id":"switch.house/server_room/xanadu","state_topic":"zigbee2mqtt/house/storage_room/disks_1","unique_id":"0xc4988600000f73e3_switch_zigbee2mqtt"}`

	discoveryFile := discoveryFileWithPassthrough(nil)
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	deviceTracker := newPassthroughDeviceTracker("")
	jobs := make(chan TDiscoveryRelayJob, 100)

	client := &fakeClient{}
	if err := subscribeDiscoveryBridge(client, nil, "junglinster", discoveryFile, publisher, testPrefix, nil, zigbee2mqttPassthroughRules, deviceTracker, jobs); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}

	client.subscribedHandlers[0](client, fakeMessage{topic: "physical/switch/0xa4c138a4716e0c02/switch/config", payload: []byte(firstPayload)})
	client.subscribedHandlers[0](client, fakeMessage{topic: "physical/switch/0xc4988600000f73e3/switch/config", payload: []byte(stalePayload)})
	drainDiscoveryRelayQueue(jobs, publisher)

	if _, found := findPublish(client.published, "homeassistant/switch/0xa4c138a4716e0c02/switch/config"); !found {
		t.Errorf("expected the first (currently-correct) device's relay to go through")
	}
	if _, found := findPublish(client.published, "homeassistant/switch/0xc4988600000f73e3/switch/config"); found {
		t.Errorf("expected the second (stale-named) device's relay to be suppressed, got %+v", client.published)
	}
	collisions := deviceTracker.collisionsSnapshot()
	if collision, ok := collisions["switch.house/server_room/xanadu"]; !ok || collision.BlockedID != "zigbee2mqtt_0xc4988600000f73e3" {
		t.Errorf("expected the collision to be recorded for suggestions, got %+v", collisions)
	}
}

// TestSubscribeDiscoveryBridgeSuppressesStaleDeviceRegardlessOfArrivalOrder is the regression test
// for a real incident found live MINUTES after this mechanism's own first deploy (2026-09-20): on
// that particular coordinator restart, the retained backlog happened to replay the STALE device's
// message before the currently-correct one's, and plain first-claim-wins let the stale device win
// -- suppressing the genuinely correct, currently-working switch instead of the stale one. This is
// the same two payloads as the test above with the arrival order REVERSED: the correct device must
// still end up owning the name and being relayed, regardless of which one arrived first.
func TestSubscribeDiscoveryBridgeSuppressesStaleDeviceRegardlessOfArrivalOrder(t *testing.T) {
	const correctPayload = `{"availability":[{"topic":"zigbee2mqtt/bridge/state"}],"device":{"identifiers":["zigbee2mqtt_0xa4c138a4716e0c02"],"manufacturer":"Innr","model":"Smart plug (EU)","name":"house/server_room/xanadu"},"default_entity_id":"switch.house/server_room/xanadu","state_topic":"zigbee2mqtt/house/server_room/xanadu","unique_id":"0xa4c138a4716e0c02_switch_zigbee2mqtt"}`
	const stalePayload = `{"availability":[{"topic":"zigbee2mqtt/bridge/state"}],"device":{"identifiers":["zigbee2mqtt_0xc4988600000f73e3"],"manufacturer":"Innr","model":"Smart plug","name":"house/server_room/xanadu"},"default_entity_id":"switch.house/server_room/xanadu","state_topic":"zigbee2mqtt/house/storage_room/disks_1","unique_id":"0xc4988600000f73e3_switch_zigbee2mqtt"}`

	discoveryFile := discoveryFileWithPassthrough(nil)
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	deviceTracker := newPassthroughDeviceTracker("")
	jobs := make(chan TDiscoveryRelayJob, 100)

	client := &fakeClient{}
	if err := subscribeDiscoveryBridge(client, nil, "junglinster", discoveryFile, publisher, testPrefix, nil, zigbee2mqttPassthroughRules, deviceTracker, jobs); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}

	// Stale message replayed FIRST -- the exact ordering that broke live.
	client.subscribedHandlers[0](client, fakeMessage{topic: "physical/switch/0xc4988600000f73e3/switch/config", payload: []byte(stalePayload)})
	client.subscribedHandlers[0](client, fakeMessage{topic: "physical/switch/0xa4c138a4716e0c02/switch/config", payload: []byte(correctPayload)})
	drainDiscoveryRelayQueue(jobs, publisher)

	if _, found := findPublish(client.published, "homeassistant/switch/0xa4c138a4716e0c02/switch/config"); !found {
		t.Errorf("expected the currently-correct device's relay to go through even though it arrived second")
	}
	// The stale device's own topic was legitimately relayed once (nothing else owned the name
	// yet when it arrived) but must have been immediately retired (empty payload) the moment the
	// genuinely correct device displaced it -- findPublish returns the FIRST match, so check the
	// LAST recorded publish on that topic specifically.
	var lastStalePublish recordedPublish
	staleFound := false
	for _, p := range client.published {
		if p.topic == "homeassistant/switch/0xc4988600000f73e3/switch/config" {
			lastStalePublish = p
			staleFound = true
		}
	}
	if !staleFound {
		t.Fatalf("expected the stale device's topic to have been published at least once (then retired), got %+v", client.published)
	}
	if len(lastStalePublish.payload) != 0 {
		t.Errorf("expected the stale device's topic to end up retired (empty payload), got %q", lastStalePublish.payload)
	}
}

// TestSubscribeDiscoveryBridgeRetiresStaleTopicKnownFromAnEarlierSession is the regression test for
// a real bug found live 2026-09-20, minutes after the displacement fix above: that fix only retires
// a topic evicted WITHIN the same in-memory ClaimName session, but ordinary claims are deliberately
// NOT persisted across restarts (the crash-loop fix). So on a coordinator restart where the
// currently-correct device simply happens to claim the name FIRST this time (no live displacement
// ever occurs, since the stale device never holds the claim this session to begin with), a topic
// that GOT RETAINED on the broker during an EARLIER (e.g. pre-consistency-check) coordinator run
// never gets retired at all -- it just lingers forever, colliding with the correct one in HA exactly
// as before the whole mechanism was built. The fix: check the PERSISTENT publisher manifest (which
// survives restarts, unlike ClaimName's own claims) for every rejected claim, not just a
// same-session displacement.
func TestSubscribeDiscoveryBridgeRetiresStaleTopicKnownFromAnEarlierSession(t *testing.T) {
	const correctPayload = `{"availability":[{"topic":"zigbee2mqtt/bridge/state"}],"device":{"identifiers":["zigbee2mqtt_0xa4c138a4716e0c02"],"manufacturer":"Innr","model":"Smart plug (EU)","name":"house/server_room/xanadu"},"default_entity_id":"switch.house/server_room/xanadu","state_topic":"zigbee2mqtt/house/server_room/xanadu","unique_id":"0xa4c138a4716e0c02_switch_zigbee2mqtt"}`
	const stalePayload = `{"availability":[{"topic":"zigbee2mqtt/bridge/state"}],"device":{"identifiers":["zigbee2mqtt_0xc4988600000f73e3"],"manufacturer":"Innr","model":"Smart plug","name":"house/server_room/xanadu"},"default_entity_id":"switch.house/server_room/xanadu","state_topic":"zigbee2mqtt/house/storage_room/disks_1","unique_id":"0xc4988600000f73e3_switch_zigbee2mqtt"}`
	staleTopic := "homeassistant/switch/0xc4988600000f73e3/switch/config"

	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	// Simulate the stale topic having been genuinely relayed and retained in an EARLIER coordinator
	// session (e.g. before the consistency check existed) -- publisher.Knows must now report true
	// for it, independent of anything ClaimName remembers in THIS session.
	if err := publisher.Publish(&fakeClient{}, "main", staleTopic, []byte(stalePayload)); err != nil {
		t.Fatalf("seeding the publisher's prior knowledge of the stale topic: %v", err)
	}
	if !publisher.Knows("main", staleTopic) {
		t.Fatalf("precondition failed: expected the publisher to already know about the stale topic")
	}

	// Fresh restart: a brand-new device tracker (claims reset to empty, as designed) and client.
	discoveryFile := discoveryFileWithPassthrough(nil)
	deviceTracker := newPassthroughDeviceTracker("")
	jobs := make(chan TDiscoveryRelayJob, 100)
	client := &fakeClient{}
	if err := subscribeDiscoveryBridge(client, nil, "junglinster", discoveryFile, publisher, testPrefix, nil, zigbee2mqttPassthroughRules, deviceTracker, jobs); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}

	// The correct device claims first THIS session -- no live displacement occurs when the stale
	// device's own retained backlog message arrives afterward, it's simply rejected outright.
	client.subscribedHandlers[0](client, fakeMessage{topic: "physical/switch/0xa4c138a4716e0c02/switch/config", payload: []byte(correctPayload)})
	client.subscribedHandlers[0](client, fakeMessage{topic: "physical/switch/0xc4988600000f73e3/switch/config", payload: []byte(stalePayload)})
	drainDiscoveryRelayQueue(jobs, publisher)

	var lastStalePublish recordedPublish
	staleFound := false
	for _, p := range client.published {
		if p.topic == staleTopic {
			lastStalePublish = p
			staleFound = true
		}
	}
	if !staleFound {
		t.Fatalf("expected the stale topic (known from an earlier session) to be retired, got no publish at all: %+v", client.published)
	}
	if len(lastStalePublish.payload) != 0 {
		t.Errorf("expected the stale topic to end up retired (empty payload), got %q", lastStalePublish.payload)
	}
}

// TestSubscribeDiscoveryBridgeRetiresPassthroughOnceDeviceMigrates is the migration-transition
// case: a device first seen unmatched (raw passthrough relay happens), then Physical.def declares
// it as a real gateway (a later message for the SAME device now matches) -- the raw copy must be
// retired so HA never ends up showing both the raw legacy entity and the newly declared one at
// once.
func TestSubscribeDiscoveryBridgeRetiresPassthroughOnceDeviceMigrates(t *testing.T) {
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	sourceTopic := "physical/sensor/zigbee2mqtt/0x00158d0001aabbcc_temperature/config"
	rawTopic := "homeassistant/sensor/zigbee2mqtt/0x00158d0001aabbcc_temperature/config"
	jobs := make(chan TDiscoveryRelayJob, 100)

	// Round 1: not yet declared -- passes through raw.
	client := &fakeClient{}
	discoveryFile := discoveryFileWithPassthrough(nil)
	if err := subscribeDiscoveryBridge(client, nil, "junglinster", discoveryFile, publisher, testPrefix, nil, zigbee2mqttPassthroughRules, nil, jobs); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}
	client.subscribedHandlers[0](client, fakeMessage{topic: sourceTopic, payload: []byte(realZigbee2MQTTAqaraMultiPayload)})
	drainDiscoveryRelayQueue(jobs, publisher)
	if _, found := findPublish(client.published, rawTopic); !found {
		t.Fatalf("precondition failed: expected the raw passthrough publish, got %+v", client.published)
	}

	// Round 2: device now declared -- same publisher (carries the "known" manifest forward,
	// exactly like a real coordinator restart after redeploying with the new declaration).
	client2 := &fakeClient{}
	migratedFile := discoveryFileWithPassthrough(map[string]TDiscoveryGateway{
		"discovery.aqara_multi": {Identifiers: []string{"zigbee2mqtt_0x00158d0001aabbcc"}},
	})
	if err := subscribeDiscoveryBridge(client2, nil, "junglinster", migratedFile, publisher, testPrefix, nil, zigbee2mqttPassthroughRules, nil, jobs); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}
	client2.subscribedHandlers[0](client2, fakeMessage{topic: sourceTopic, payload: []byte(realZigbee2MQTTAqaraMultiPayload)})
	drainDiscoveryRelayQueue(jobs, publisher)

	publish, found := findPublish(client2.published, rawTopic)
	if !found {
		t.Fatalf("expected a retiring (empty) publish on the raw passthrough topic, got %+v", client2.published)
	}
	if len(publish.payload) != 0 {
		t.Errorf("retiring publish payload = %q, want empty", publish.payload)
	}
}

// TestSubscribeDiscoveryBridgeRecordsPassthroughDevice is the regression test for the real gap
// found live 2026-09-14: a device relayed raw via passthrough never showed up in
// suggestions/discovery.txt at all, since nothing recorded it anywhere the generator could query.
// A passthrough relay must also record the device (grouped by its own real identifier, with its
// own reported name) on passthroughDeviceTracker.
func TestSubscribeDiscoveryBridgeRecordsPassthroughDevice(t *testing.T) {
	discoveryFile := discoveryFileWithPassthrough(nil)
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	deviceTracker := newPassthroughDeviceTracker("")
	jobs := make(chan TDiscoveryRelayJob, 100)

	client := &fakeClient{}
	if err := subscribeDiscoveryBridge(client, nil, "junglinster", discoveryFile, publisher, testPrefix, nil, zigbee2mqttPassthroughRules, deviceTracker, jobs); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}
	client.subscribedHandlers[0](client, fakeMessage{
		topic:   "physical/sensor/zigbee2mqtt/0x00158d0001aabbcc_temperature/config",
		payload: []byte(realZigbee2MQTTAqaraMultiPayload),
	})

	snap := deviceTracker.snapshot()
	entry, ok := snap["zigbee2mqtt_0x00158d0001aabbcc"]
	if !ok {
		t.Fatalf("expected the device to be tracked, got %+v", snap)
	}
	if entry.Name != "aqara_multi" {
		t.Errorf("Name = %q, want %q", entry.Name, "aqara_multi")
	}
	leaf, ok := entry.Leaves["0x00158d0001aabbcc_temperature_zigbee2mqtt"]
	if !ok {
		t.Fatalf("expected the leaf to be tracked, got %+v", entry.Leaves)
	}
	if leaf.Domain != "sensor" {
		t.Errorf("leaf domain = %q, want %q (the topic's own component)", leaf.Domain, "sensor")
	}
}

// TestSubscribeDiscoveryBridgeRecordsUndeclaredDeviceWithoutPassthroughRule is the regression test
// for a real bug found live 2026-09-19: passthroughDeviceTracker.Record used to be nested INSIDE
// the matchesAnyPassthroughPrefix check, coupling the generator's own "which real devices exist
// under ${mqtt_discovery_physical} that Physical.def hasn't declared yet" suggestion feed to
// whether a "discovery_passthrough ...;" rule happened to be declared at all -- with zero rules
// declared, an undeclared device was invisible to suggestions even though this subscription was
// already receiving its discovery config. Tracking and relaying are separate concerns: a device
// must be recorded here regardless of whether it's ALSO being actively relayed into HA.
func TestSubscribeDiscoveryBridgeRecordsUndeclaredDeviceWithoutPassthroughRule(t *testing.T) {
	discoveryFile := discoveryFileWithPassthrough(nil)
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	deviceTracker := newPassthroughDeviceTracker("")
	jobs := make(chan TDiscoveryRelayJob, 100)

	client := &fakeClient{}
	// No passthrough rules at all -- this is the exact condition that used to suppress tracking.
	if err := subscribeDiscoveryBridge(client, nil, "junglinster", discoveryFile, publisher, testPrefix, nil, nil, deviceTracker, jobs); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}
	client.subscribedHandlers[0](client, fakeMessage{
		topic:   "physical/sensor/zigbee2mqtt/0x00158d0001aabbcc_temperature/config",
		payload: []byte(realZigbee2MQTTAqaraMultiPayload),
	})

	snap := deviceTracker.snapshot()
	if _, ok := snap["zigbee2mqtt_0x00158d0001aabbcc"]; !ok {
		t.Fatalf("expected the device to be tracked even with zero passthrough rules declared, got %+v", snap)
	}

	// The relay decision stays genuinely gated on passthrough rules -- confirm no relay was queued.
	select {
	case job := <-jobs:
		t.Errorf("expected no relay job queued without a matching passthrough rule, got %+v", job)
	default:
	}
}

// TestSubscribeDiscoveryBridgeForgetsPassthroughDeviceOnceMigrated confirms a device stops being
// tracked for suggestion the moment it starts matching a declared gateway -- otherwise it would
// keep being suggested for a migration it has already undergone.
func TestSubscribeDiscoveryBridgeForgetsPassthroughDeviceOnceMigrated(t *testing.T) {
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	deviceTracker := newPassthroughDeviceTracker("")
	sourceTopic := "physical/sensor/zigbee2mqtt/0x00158d0001aabbcc_temperature/config"
	jobs := make(chan TDiscoveryRelayJob, 100)

	client := &fakeClient{}
	discoveryFile := discoveryFileWithPassthrough(nil)
	if err := subscribeDiscoveryBridge(client, nil, "junglinster", discoveryFile, publisher, testPrefix, nil, zigbee2mqttPassthroughRules, deviceTracker, jobs); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}
	client.subscribedHandlers[0](client, fakeMessage{topic: sourceTopic, payload: []byte(realZigbee2MQTTAqaraMultiPayload)})
	if _, ok := deviceTracker.snapshot()["zigbee2mqtt_0x00158d0001aabbcc"]; !ok {
		t.Fatalf("precondition failed: expected the device to be tracked before migration")
	}

	client2 := &fakeClient{}
	migratedFile := discoveryFileWithPassthrough(map[string]TDiscoveryGateway{
		"discovery.aqara_multi": {Identifiers: []string{"zigbee2mqtt_0x00158d0001aabbcc"}},
	})
	if err := subscribeDiscoveryBridge(client2, nil, "junglinster", migratedFile, publisher, testPrefix, nil, zigbee2mqttPassthroughRules, deviceTracker, jobs); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}
	client2.subscribedHandlers[0](client2, fakeMessage{topic: sourceTopic, payload: []byte(realZigbee2MQTTAqaraMultiPayload)})

	if _, ok := deviceTracker.snapshot()["zigbee2mqtt_0x00158d0001aabbcc"]; ok {
		t.Errorf("expected the device to be forgotten once migrated, got %+v", deviceTracker.snapshot())
	}
}

// TestSubscribeDiscoveryBridgeForgetsPassthroughDeviceOnDeletion is the regression test for a real
// gap found live 2026-09-21 (Vienna): a device deleted outright from Zigbee2MQTT (not migrated into
// a real Physical.def declaration -- genuinely removed from the network) publishes an empty payload
// on its own discovery config topic, same as TestSubscribeDiscoveryBridgeRetiresPassthroughOnUpstream
// Retraction below -- but before this fix, only the raw HA relay was retired; the passthrough
// device tracker's own suggestion entry (what ends up in suggestions/discovery.txt) was never
// forgotten, since Forget was previously only ever called from the "device just started matching a
// declared gateway" (migration) branch. Confirmed live: "discovery.apartment_living_room_rack_aqara_
// multi" stayed suggested well after the underlying Zigbee device was deleted from the gateway.
func TestSubscribeDiscoveryBridgeForgetsPassthroughDeviceOnDeletion(t *testing.T) {
	discoveryFile := discoveryFileWithPassthrough(nil)
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	deviceTracker := newPassthroughDeviceTracker("")
	sourceTopic := "physical/sensor/zigbee2mqtt/0x00158d0001aabbcc_temperature/config"
	jobs := make(chan TDiscoveryRelayJob, 100)

	client := &fakeClient{}
	if err := subscribeDiscoveryBridge(client, nil, "junglinster", discoveryFile, publisher, testPrefix, nil, zigbee2mqttPassthroughRules, deviceTracker, jobs); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}
	client.subscribedHandlers[0](client, fakeMessage{topic: sourceTopic, payload: []byte(realZigbee2MQTTAqaraMultiPayload)})
	drainDiscoveryRelayQueue(jobs, publisher)
	if _, ok := deviceTracker.snapshot()["zigbee2mqtt_0x00158d0001aabbcc"]; !ok {
		t.Fatalf("precondition failed: expected the device to be tracked before deletion")
	}

	client.subscribedHandlers[0](client, fakeMessage{topic: sourceTopic, payload: []byte{}})
	drainDiscoveryRelayQueue(jobs, publisher)

	if _, ok := deviceTracker.snapshot()["zigbee2mqtt_0x00158d0001aabbcc"]; ok {
		t.Errorf("expected the device to be forgotten once its discovery config was retracted, got %+v", deviceTracker.snapshot())
	}
}

// TestSubscribeDiscoveryBridgeRetiresPassthroughOnUpstreamRetraction confirms Zigbee2MQTT itself
// removing a not-yet-migrated device (an empty payload on its own discovery topic, HA's standard
// retraction convention) is relayed through too, so HA's copy of the raw entity disappears along
// with the real one rather than lingering as an orphan.
func TestSubscribeDiscoveryBridgeRetiresPassthroughOnUpstreamRetraction(t *testing.T) {
	discoveryFile := discoveryFileWithPassthrough(nil)
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	sourceTopic := "physical/sensor/zigbee2mqtt/0x00158d0001aabbcc_temperature/config"
	rawTopic := "homeassistant/sensor/zigbee2mqtt/0x00158d0001aabbcc_temperature/config"
	jobs := make(chan TDiscoveryRelayJob, 100)

	client := &fakeClient{}
	if err := subscribeDiscoveryBridge(client, nil, "junglinster", discoveryFile, publisher, testPrefix, nil, zigbee2mqttPassthroughRules, nil, jobs); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}
	client.subscribedHandlers[0](client, fakeMessage{topic: sourceTopic, payload: []byte(realZigbee2MQTTAqaraMultiPayload)})
	drainDiscoveryRelayQueue(jobs, publisher)
	if _, found := findPublish(client.published, rawTopic); !found {
		t.Fatalf("precondition failed: expected the raw passthrough publish, got %+v", client.published)
	}

	client.subscribedHandlers[0](client, fakeMessage{topic: sourceTopic, payload: []byte{}})
	drainDiscoveryRelayQueue(jobs, publisher)

	var last recordedPublish
	found := false
	for _, p := range client.published {
		if p.topic == rawTopic {
			last = p
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a retiring publish on the raw topic after upstream retraction, got %+v", client.published)
	}
	if len(last.payload) != 0 {
		t.Errorf("retiring publish payload = %q, want empty", last.payload)
	}
}

// TestSubscribeDiscoveryBridgeMatchesCommandTopicPrefix confirms the prefix check also looks at
// command_topic, not just state_topic -- a real requirement for commandable Zigbee2MQTT domains
// (switches, lights, covers) whose discovery payload may carry only a command_topic with no
// state_topic at all.
func TestSubscribeDiscoveryBridgeMatchesCommandTopicPrefix(t *testing.T) {
	const payload = `{"device":{"identifiers":["zigbee2mqtt_0xswitch"]},"command_topic":"zigbee2mqtt/hallway_switch/set","unique_id":"0xswitch_switch_zigbee2mqtt"}`
	discoveryFile := discoveryFileWithPassthrough(nil)
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	jobs := make(chan TDiscoveryRelayJob, 100)

	client := &fakeClient{}
	if err := subscribeDiscoveryBridge(client, nil, "junglinster", discoveryFile, publisher, testPrefix, nil, zigbee2mqttPassthroughRules, nil, jobs); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}
	client.subscribedHandlers[0](client, fakeMessage{
		topic:   "physical/switch/zigbee2mqtt/hallway_switch/config",
		payload: []byte(payload),
	})
	drainDiscoveryRelayQueue(jobs, publisher)

	wantTopic := "homeassistant/switch/zigbee2mqtt/hallway_switch/config"
	if _, found := findPublish(client.published, wantTopic); !found {
		t.Errorf("expected a passthrough publish matched via command_topic on %s, got %+v", wantTopic, client.published)
	}
}
