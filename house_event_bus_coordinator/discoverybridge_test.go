package main

import (
	"path/filepath"
	"strings"
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

func TestBuildRelayedDiscoveryConfig(t *testing.T) {
	payload, err := decodeDiscoveryPayload([]byte(realEMSESPBoilerOutdoortempPayload))
	if err != nil {
		t.Fatalf("decodeDiscoveryPayload error: %v", err)
	}

	topic, body, ok := buildRelayedDiscoveryConfig("sensor.social_garage_door_temperature", "discovery.ems_esp", payload, TDiscoveryEntityLink{}, testPrefix)
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
	// device_class/unit are already native -- link's conflicting values must NOT win.
	// state_class the payload leaves empty, so link's value must gap-fill it. icon has no
	// native counterpart at all, so it always comes from link.
	link := TDiscoveryEntityLink{
		DeviceClass: "should-not-win",
		Unit:        "should-not-win",
		StateClass:  "measurement",
		Icon:        "mdi:thermometer",
	}

	_, body, ok := buildRelayedDiscoveryConfig("sensor.social_garage_door_temperature", "discovery.ems_esp", payload, link, testPrefix)
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

func TestBuildRelayedDiscoveryConfigFailsWithoutStateTopic(t *testing.T) {
	_, _, ok := buildRelayedDiscoveryConfig("sensor.social_garage_door_temperature", "discovery.ems_esp", tDecodedDiscoveryPayload{}, TDiscoveryEntityLink{}, testPrefix)
	if ok {
		t.Errorf("expected failure when the decoded payload has no state topic")
	}
}

// TestSubscribeDiscoveryBridgeMarksExistenceAndPublishesStatus is PROJECT.md 1.8's own coverage:
// a real gateway discovery payload arriving must mark the matched (gateway, leaf) known-to-exist
// and publish that gateway's updated existence status, alongside the pre-existing relay behaviour.
func TestSubscribeDiscoveryBridgeMarksExistenceAndPublishesStatus(t *testing.T) {
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

	client := &fakeClient{}
	if err := subscribeDiscoveryBridge(client, nil, "junglinster", discoveryFile, publisher, testPrefix, tracker); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}
	client.subscribedHandlers[0](client, fakeMessage{
		topic:   "physical/sensor/ems-esp/boiler_outdoortemp/config",
		payload: []byte(realEMSESPBoilerOutdoortempPayload),
	})

	if got := tracker.snapshot("discovery.ems_esp")["boiler_outdoortemp"]; got != StatusKnownToExist {
		t.Errorf("status = %q, want %q", got, StatusKnownToExist)
	}
	if _, found := findPublish(client.published, discoveryExistenceStatusTopic("discovery.ems_esp")); !found {
		t.Errorf("expected an existence-status publish, got %+v", client.published)
	}
}

// TestSubscribeDiscoveryBridgeRetractsOnEmptyPayload confirms an empty (retracted) discovery
// payload on a topic the handler previously saw a real payload on moves that leaf to
// known-not-to-exist and publishes the updated status -- PROJECT.md 1.8's retraction half.
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

	client := &fakeClient{}
	if err := subscribeDiscoveryBridge(client, nil, "junglinster", discoveryFile, publisher, testPrefix, tracker); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}
	topic := "physical/sensor/ems-esp/boiler_outdoortemp/config"
	client.subscribedHandlers[0](client, fakeMessage{topic: topic, payload: []byte(realEMSESPBoilerOutdoortempPayload)})
	if got := tracker.snapshot("discovery.ems_esp")["boiler_outdoortemp"]; got != StatusKnownToExist {
		t.Fatalf("precondition failed: status = %q, want %q", got, StatusKnownToExist)
	}

	client.subscribedHandlers[0](client, fakeMessage{topic: topic, payload: []byte{}})

	if got := tracker.snapshot("discovery.ems_esp")["boiler_outdoortemp"]; got != StatusKnownNotToExist {
		t.Errorf("status after retraction = %q, want %q", got, StatusKnownNotToExist)
	}
	// findPublish returns the first match, but this topic is published twice (known-to-exist, then
	// retracted) -- the retained MQTT semantics mean only the last one matters, so check that one.
	statusTopic := discoveryExistenceStatusTopic("discovery.ems_esp")
	var publish recordedPublish
	found := false
	for _, p := range client.published {
		if p.topic == statusTopic {
			publish = p
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an existence-status publish after retraction, got %+v", client.published)
	}
	if !strings.Contains(string(publish.payload), `"boiler_outdoortemp":"known-not-to-exist"`) {
		t.Errorf("published status = %s, want boiler_outdoortemp known-not-to-exist", publish.payload)
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

	client := &fakeClient{}
	if err := subscribeDiscoveryBridge(client, nil, "junglinster", discoveryFile, publisher, testPrefix, nil); err != nil {
		t.Fatalf("subscribeDiscoveryBridge error: %v", err)
	}
	client.subscribedHandlers[0](client, fakeMessage{
		topic:   "physical/sensor/ems-esp/boiler_outdoortemp/config",
		payload: []byte(realEMSESPBoilerOutdoortempPayload),
	})

	if _, found := findPublish(client.published, "homeassistant/sensor/coordinator/discovery_ems_esp_boiler_outdoortemp/config"); !found {
		t.Errorf("expected the relayed discovery config publish to still happen, got %+v", client.published)
	}
}
