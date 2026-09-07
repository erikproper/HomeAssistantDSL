package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
)

func TestLoadImportedHassBridgeFileMissingReturnsZeroValue(t *testing.T) {
	got, err := loadImportedFile(filepath.Join(t.TempDir(), "does_not_exist.yaml"))
	if err != nil {
		t.Fatalf("loadImportedFile error: %v", err)
	}
	if len(got.Devices) != 0 {
		t.Errorf("got %v, want a zero-value file for a missing path", got)
	}
}

func TestExpectedImportedHassBridgeTopics(t *testing.T) {
	importFile := TImportedFile{Devices: map[string]TImportedDevice{
		"hass.vienna_shower_room": {
			RemoteInstallation: "junglinster", RemoteDeviceID: "hass.vienna_shower_room",
			Capabilities: map[string]TImportedCapability{
				"temperature": {LocalEntity: "sensor.physical_shower_room_netatmo_temperature"},
			},
		},
	}}
	got := expectedImportedTopics(importFile, testPrefix)
	wantTopic := discoveryTopic(testPrefix, "sensor", importedUniqueID("sensor.physical_shower_room_netatmo_temperature"))
	if !got[wantTopic] {
		t.Errorf("expected %v to contain %q", got, wantTopic)
	}
}

// TestExpectedImportedHassBridgeTopicsSkipsUnresolvedCapability confirms a capability with no
// LocalEntity (shouldn't normally reach this file at all -- generateImportedDeviceFile
// already omits it -- but defensive here too) contributes nothing to the expected set.
func TestExpectedImportedHassBridgeTopicsSkipsUnresolvedCapability(t *testing.T) {
	importFile := TImportedFile{Devices: map[string]TImportedDevice{
		"hass.vienna_shower_room": {
			RemoteInstallation: "junglinster", RemoteDeviceID: "hass.vienna_shower_room",
			Capabilities: map[string]TImportedCapability{
				"temperature": {},
			},
		},
	}}
	if got := expectedImportedTopics(importFile, testPrefix); len(got) != 0 {
		t.Errorf("got %v, want an empty set when no capability has a resolved LocalEntity", got)
	}
}

// TestMatchImportedCapabilityByStableID confirms a declared capability is matched purely from the
// topic's own stable-id segment -- recomputed from (RemoteInstallation, RemoteDeviceID, capability
// name) via exportStableID, matching the export side's own scheme (discoveryhassbridge.go) exactly,
// matching the user's own worked example (PROJECT.md item 1 discussion, 2026-09-07).
func TestMatchImportedCapabilityByStableID(t *testing.T) {
	importFile := TImportedFile{Devices: map[string]TImportedDevice{
		"import.vienna_livingroom": {
			RemoteInstallation: "junglinster", RemoteDeviceID: "hass.vienna_livingroom",
			Capabilities: map[string]TImportedCapability{
				"co2": {LocalEntity: "sensor.infrastructural_living_room_co2"},
			},
		},
	}}

	topic := "junglinster/homeassistant/sensor/coordinator/junglinster_hass_vienna_livingroom_co2/config"
	match, matched := matchImportedCapabilityByStableID(importFile, "junglinster", topic)
	if !matched {
		t.Fatalf("expected a match")
	}
	if match.LocalDeviceID != "import.vienna_livingroom" || match.Capability != "co2" || match.LocalEntity != "sensor.infrastructural_living_room_co2" {
		t.Errorf("match = %+v, want {import.vienna_livingroom co2 sensor.infrastructural_living_room_co2}", match)
	}

	// Wrong installation must not match, even with an otherwise-identical stable id.
	if _, matched := matchImportedCapabilityByStableID(importFile, "vienna", topic); matched {
		t.Errorf("expected no match for a different remote installation")
	}
	// A topic whose stable id doesn't correspond to any declared capability must not match.
	unrelatedTopic := "junglinster/homeassistant/sensor/coordinator/junglinster_hass_vienna_livingroom_humidity/config"
	if _, matched := matchImportedCapabilityByStableID(importFile, "junglinster", unrelatedTopic); matched {
		t.Errorf("expected no match for an unrelated stable id")
	}
}

// TestMatchImportedCapabilityByStableIDSkipsUnresolvedCapability confirms a declared but
// never-positioned capability (empty LocalEntity) never matches.
func TestMatchImportedCapabilityByStableIDSkipsUnresolvedCapability(t *testing.T) {
	importFile := TImportedFile{Devices: map[string]TImportedDevice{
		"import.vienna_shower_room": {
			RemoteInstallation: "junglinster", RemoteDeviceID: "hass.vienna_shower_room",
			Capabilities: map[string]TImportedCapability{
				"humidity": {},
			},
		},
	}}
	topic := "junglinster/homeassistant/sensor/coordinator/junglinster_hass_vienna_shower_room_humidity/config"
	if _, matched := matchImportedCapabilityByStableID(importFile, "junglinster", topic); matched {
		t.Errorf("expected no match for a capability with no resolved LocalEntity")
	}
}

// TestMatchImportedCapabilityByStableIDCoercesAcrossDomains is the regression test for the
// coercion the user explicitly asked for (2026-09-07): declaring "binary_sensor.node;" on the
// import side must still resolve even if the exporter actually published it under a different
// domain (e.g. "switch") -- the stable id match never inspects domain at all, only the topic's own
// stable-id segment, which is domain-independent by construction.
func TestMatchImportedCapabilityByStableIDCoercesAcrossDomains(t *testing.T) {
	importFile := TImportedFile{Devices: map[string]TImportedDevice{
		"import.frame": {
			RemoteInstallation: "junglinster", RemoteDeviceID: "hass.frame",
			// Declared (and locally positioned) as binary_sensor -- a deliberate coercion.
			Capabilities: map[string]TImportedCapability{
				"node": {LocalEntity: "binary_sensor.infrastructural_frame_node"},
			},
		},
	}}
	// The exporter actually published this stable id under "switch", not "binary_sensor".
	topic := "junglinster/homeassistant/switch/coordinator/junglinster_hass_frame_node/config"
	match, matched := matchImportedCapabilityByStableID(importFile, "junglinster", topic)
	if !matched {
		t.Fatalf("expected a match despite the domain mismatch -- coercion should not block it")
	}
	if match.LocalEntity != "binary_sensor.infrastructural_frame_node" {
		t.Errorf("LocalEntity = %q, want the DECLARED (coerced) domain preserved, not the remote's own", match.LocalEntity)
	}
}

// TestMatchImportedCapabilityByStableIDMatchesGroupPrefixedHostsCapability is the regression test
// for extending shorthand to hosts-kind capabilities (2026-09-07): a group-prefixed capability
// name ("cpu/load") must still resolve to exactly one topic segment (exportStableID sanitizes "/"
// internally), and the declared capability key on the import side is the SAME group-prefixed name
// a DSL author's Spaces.def already uses -- not the exporter's own internal bare leaf.
func TestMatchImportedCapabilityByStableIDMatchesGroupPrefixedHostsCapability(t *testing.T) {
	importFile := TImportedFile{Devices: map[string]TImportedDevice{
		"import.pro-1": {
			RemoteInstallation: "junglinster", RemoteDeviceID: "host.pro-1",
			Capabilities: map[string]TImportedCapability{
				"cpu/load": {LocalEntity: "sensor.infrastructural_cloud_pro1_cpu_load"},
			},
		},
	}}
	topic := "junglinster/homeassistant/sensor/coordinator/junglinster_host_pro-1_cpu_load/config"
	match, matched := matchImportedCapabilityByStableID(importFile, "junglinster", topic)
	if !matched {
		t.Fatalf("expected a match for the group-prefixed \"cpu/load\" capability")
	}
	if match.LocalDeviceID != "import.pro-1" || match.Capability != "cpu/load" || match.LocalEntity != "sensor.infrastructural_cloud_pro1_cpu_load" {
		t.Errorf("match = %+v, want {import.pro-1 cpu/load sensor.infrastructural_cloud_pro1_cpu_load}", match)
	}
}

func TestBuildImportedDiscoveryBody(t *testing.T) {
	payload := importedDiscoveryPayload{
		DefaultEntityID: "sensor.infrastructural_vienna_shower_room_temperature",
		DeviceClass:     "temperature",
		Unit:            "°C",
		StateClass:      "measurement",
	}
	payload.Device.Identifiers = []string{"hass.vienna_shower_room"}
	match := importCapabilityMatch{
		LocalDeviceID: "hass.vienna_shower_room", Capability: "temperature",
		LocalEntity: "sensor.physical_shower_room_netatmo_temperature",
		DisplayName: "apartment/shower_room/netatmo",
	}

	body := buildImportedDiscoveryBody(match, payload, "")
	if body["default_entity_id"] != "sensor.physical_shower_room_netatmo_temperature" {
		t.Errorf("default_entity_id = %v, want the Spaces.def-resolved local entity id", body["default_entity_id"])
	}
	wantStateTopic := importedStateTopic("sensor.physical_shower_room_netatmo_temperature")
	if body["state_topic"] != wantStateTopic {
		t.Errorf("state_topic = %v, want the local relay topic %q, not the remote's own", body["state_topic"], wantStateTopic)
	}
	if body["device_class"] != "temperature" || body["unit_of_measurement"] != "°C" || body["state_class"] != "measurement" {
		t.Errorf("typing metadata not copied through verbatim: %v", body)
	}
	device, ok := body["device"].(map[string]interface{})
	if !ok {
		t.Fatalf("device block missing or wrong shape: %v", body["device"])
	}
	if ids, ok := device["identifiers"].([]string); !ok || len(ids) != 1 || ids[0] != "hass.vienna_shower_room" {
		t.Errorf("device.identifiers = %v, want [hass.vienna_shower_room]", device["identifiers"])
	}
	if device["name"] != "apartment/shower_room/netatmo" {
		t.Errorf("device.name = %v, want the Spaces.def-resolved display name, not anything from the remote", device["name"])
	}
}

// TestBuildImportedDiscoveryBodyDeviceNameFallsBackToLocalDeviceID confirms a device with no
// resolved DisplayName (shouldn't normally happen -- registerImportedDevicePositioning always
// sets one -- but defensive) falls back to the bare local device id, never the remote's own name.
func TestBuildImportedDiscoveryBodyDeviceNameFallsBackToLocalDeviceID(t *testing.T) {
	payload := importedDiscoveryPayload{DefaultEntityID: "sensor.infrastructural_vienna_shower_room_temperature"}
	payload.Device.Identifiers = []string{"hass.vienna_shower_room"}
	match := importCapabilityMatch{LocalDeviceID: "hass.vienna_shower_room", Capability: "temperature", LocalEntity: "sensor.physical_shower_room_netatmo_temperature"}

	body := buildImportedDiscoveryBody(match, payload, "")
	device := body["device"].(map[string]interface{})
	if device["name"] != "hass.vienna_shower_room" {
		t.Errorf("device.name = %v, want the fallback bare local device id", device["name"])
	}
}

// TestImportedAvailabilityTopicPointsAtDevicesNode covers the real gap found live 2026-08-31: a
// device's upstream integration may correctly track connectivity on its own native entity while
// leaving every other imported entity frozen at its last value with no "unavailable" signal at
// all. Every non-"node" capability's discovery config must point availability_topic at the SAME
// device's own imported "node" capability's local relay topic.
func TestImportedAvailabilityTopicPointsAtDevicesNode(t *testing.T) {
	importFile := TImportedFile{Devices: map[string]TImportedDevice{
		"hass.vienna_terrace": {
			RemoteInstallation: "junglinster", RemoteDeviceID: "hass.vienna_terrace",
			Capabilities: map[string]TImportedCapability{
				"node":        {LocalEntity: "binary_sensor.infrastructural_terrace_netatmo_node"},
				"temperature": {LocalEntity: "sensor.physical_terrace_netatmo_temperature"},
			},
		},
	}}
	match := importCapabilityMatch{LocalDeviceID: "hass.vienna_terrace", Capability: "temperature", LocalEntity: "sensor.physical_terrace_netatmo_temperature"}

	got := importedAvailabilityTopic(importFile, match)
	want := importedStateTopic("binary_sensor.infrastructural_terrace_netatmo_node")
	if got != want {
		t.Errorf("importedAvailabilityTopic = %q, want %q", got, want)
	}
}

// TestImportedAvailabilityTopicAvoidsCircularReferenceForNodeItself covers the user's own explicit
// concern: the node capability's own discovery config must not gate its availability on itself.
func TestImportedAvailabilityTopicAvoidsCircularReferenceForNodeItself(t *testing.T) {
	importFile := TImportedFile{Devices: map[string]TImportedDevice{
		"hass.vienna_terrace": {
			Capabilities: map[string]TImportedCapability{
				"node": {LocalEntity: "binary_sensor.infrastructural_terrace_netatmo_node"},
			},
		},
	}}
	match := importCapabilityMatch{LocalDeviceID: "hass.vienna_terrace", Capability: "node", LocalEntity: "binary_sensor.infrastructural_terrace_netatmo_node"}

	if got := importedAvailabilityTopic(importFile, match); got != "" {
		t.Errorf("importedAvailabilityTopic for the node capability itself = %q, want \"\" (no self-reference)", got)
	}
}

// TestImportedAvailabilityTopicEmptyWhenDeviceHasNoNodeCapability covers a device that never
// declared/positioned a "node" capability at all -- nothing to gate on, must not error or invent
// one.
func TestImportedAvailabilityTopicEmptyWhenDeviceHasNoNodeCapability(t *testing.T) {
	importFile := TImportedFile{Devices: map[string]TImportedDevice{
		"hass.vienna_terrace": {
			Capabilities: map[string]TImportedCapability{
				"temperature": {LocalEntity: "sensor.physical_terrace_netatmo_temperature"},
			},
		},
	}}
	match := importCapabilityMatch{LocalDeviceID: "hass.vienna_terrace", Capability: "temperature", LocalEntity: "sensor.physical_terrace_netatmo_temperature"}

	if got := importedAvailabilityTopic(importFile, match); got != "" {
		t.Errorf("importedAvailabilityTopic with no declared node capability = %q, want \"\"", got)
	}
}

// TestBuildImportedDiscoveryBodyIncludesAvailability confirms availability is the AND
// (availability_mode "all") of the device's node topic (HA's own native binary_sensor "on"/"off"
// convention, not hosts devices' "true"/"false" one) and the capability's own state topic never
// itself reporting the literal string "unavailable" -- node being reachable doesn't guarantee this
// specific reading is currently valid.
func TestBuildImportedDiscoveryBodyIncludesAvailability(t *testing.T) {
	payload := importedDiscoveryPayload{DefaultEntityID: "sensor.infrastructural_vienna_shower_room_temperature"}
	payload.Device.Identifiers = []string{"hass.vienna_shower_room"}
	match := importCapabilityMatch{LocalDeviceID: "hass.vienna_shower_room", Capability: "temperature", LocalEntity: "sensor.physical_shower_room_netatmo_temperature"}
	nodeAvailabilityTopic := importedStateTopic("binary_sensor.infrastructural_shower_room_netatmo_node")

	body := buildImportedDiscoveryBody(match, payload, nodeAvailabilityTopic)
	if body["availability_mode"] != "all" {
		t.Errorf("availability_mode = %v, want \"all\"", body["availability_mode"])
	}
	if e := findAvailabilityEntry(body, nodeAvailabilityTopic); e == nil || e["payload_available"] != "on" || e["payload_not_available"] != "off" {
		t.Errorf("availability = %v, want a node-gate entry on %q (on/off)", body["availability"], nodeAvailabilityTopic)
	}
	ownTopic := importedStateTopic(match.LocalEntity)
	if e := findAvailabilityEntry(body, ownTopic); e == nil || e["payload_available"] != "available" {
		t.Errorf("availability = %v, want a self-check entry on its own state topic %q", body["availability"], ownTopic)
	}
}

// TestBuildImportedDiscoveryBodySetsLowercasePayloadOnOffForBinarySensor is a regression test for
// a real bug found live 2026-08-31: an imported binary_sensor's discovery config (the "node"
// capability, or any other binary_sensor) never set payload_on/payload_off at all, so HA's MQTT
// platform fell back to its own uppercase "ON"/"OFF" default -- which never matched the actual
// lowercase "on"/"off" relayed from the source, leaving the entity stuck at "unknown" forever even
// though the upstream device was perfectly reachable.
func TestBuildImportedDiscoveryBodySetsLowercasePayloadOnOffForBinarySensor(t *testing.T) {
	payload := importedDiscoveryPayload{DefaultEntityID: "binary_sensor.infrastructural_vienna_livingroom_node"}
	payload.Device.Identifiers = []string{"hass.vienna_livingroom"}
	match := importCapabilityMatch{LocalDeviceID: "hass.vienna_livingroom", Capability: "node", LocalEntity: "binary_sensor.infrastructural_apartment_living_room_netatmo_node"}

	body := buildImportedDiscoveryBody(match, payload, "")
	if body["payload_on"] != "on" || body["payload_off"] != "off" {
		t.Errorf("payload_on/payload_off = %v/%v, want on/off", body["payload_on"], body["payload_off"])
	}
}

// TestBuildImportedDiscoveryBodyPreservesHostsKindPayloadOnOff is a regression test for a real bug
// found live 2026-09-05: a hosts-kind node capability's own remote discovery payload carries its
// coordinator-chosen "true"/"false" convention (discovery.go's TBinarySensorDiscoveryPayload,
// matching what TCloudLivenessTracker actually publishes) -- but buildImportedDiscoveryBody
// unconditionally overwrote it with applyProxiedBinarySensorPayload's "on"/"off" default (correct
// only for a hassbridge-sourced binary_sensor), leaving every imported hosts-kind node entity
// (confirmed live: import.eriks-macbook-pro-2's own node, reachable via Vienna's cross-house
// "integration import") stuck at "unknown" forever, since its actual state topic payload never
// matched HA's expected "on"/"off".
func TestBuildImportedDiscoveryBodyPreservesHostsKindPayloadOnOff(t *testing.T) {
	payload := importedDiscoveryPayload{
		DefaultEntityID: "binary_sensor.infrastructural_eriks_macbook_pro_2_node",
		PayloadOn:       "true",
		PayloadOff:      "false",
	}
	payload.Device.Identifiers = []string{"host.eriks-macbook-pro-2"}
	match := importCapabilityMatch{LocalDeviceID: "import.eriks-macbook-pro-2", Capability: "node", LocalEntity: "binary_sensor.infrastructural_eriks_macbook_pro_2_node"}

	body := buildImportedDiscoveryBody(match, payload, "")
	if body["payload_on"] != "true" || body["payload_off"] != "false" {
		t.Errorf("payload_on/payload_off = %v/%v, want the remote's own true/false preserved verbatim", body["payload_on"], body["payload_off"])
	}
}

// TestSubscribeImportedDevicesPublishesLocalDiscoveryAndRelaysValue is the end-to-end path: a
// declared import (with an already-resolved LocalEntity, as generateImportedDeviceFile would
// produce from a real Spaces.def positioning) + an incoming exported discovery payload on the
// cloud broker, keyed by the export side's own stable id (junglinster_hass_vienna_livingroom_co2),
// must produce (1) a local discovery config on the main broker whose default_entity_id/state_topic
// are built from that resolved LocalEntity, not any coordinator-invented naming or anything from
// the payload's own content, and (2) a subscription that forward-publishes the remote's own state
// value onto that same local relay topic, learned lazily from the discovery config's own
// state_topic (stateHandler's relay-index lookup).
func TestSubscribeImportedDevicesPublishesLocalDiscoveryAndRelaysValue(t *testing.T) {
	const localEntity = "sensor.infrastructural_living_room_co2"
	importFile := TImportedFile{Devices: map[string]TImportedDevice{
		"import.vienna_livingroom": {
			RemoteInstallation: "junglinster", RemoteDeviceID: "hass.vienna_livingroom",
			Capabilities: map[string]TImportedCapability{
				"co2": {LocalEntity: localEntity},
			},
		},
	}}

	remoteStateTopic := "junglinster/homeassistant_instances/protocols-server-2/bridge/sensor.vienna_livingroom_co2/state"
	discoveryPayload := map[string]interface{}{
		// default_entity_id deliberately carries the REMOTE's own local name -- must be ignored by
		// this resolution path entirely, since matching is purely stable-id based.
		"default_entity_id": "sensor.vienna_livingroom_co2",
		"state_topic":       remoteStateTopic,
		"device":            map[string]interface{}{"identifiers": []string{"hass.vienna_livingroom"}, "name": "vienna_livingroom"},
	}
	discoveryData, err := json.Marshal(discoveryPayload)
	if err != nil {
		t.Fatalf("marshalling test payload: %v", err)
	}
	stableID := exportStableID("junglinster", "hass.vienna_livingroom", "co2")
	discoveryTopicIn := "junglinster/" + testPrefix + "/sensor/coordinator/" + stableID + "/config"

	cloudClient := &fakeClient{retained: []fakeMessage{{topic: discoveryTopicIn, payload: discoveryData}}}
	mainClient := &fakeClient{}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))

	if err := subscribeImportedDevices(mainClient, cloudClient, "junglinster", importFile, publisher, testPrefix, nil); err != nil {
		t.Fatalf("subscribeImportedDevices error: %v", err)
	}

	wantDiscoveryTopic := discoveryTopic(testPrefix, "sensor", importedUniqueID(localEntity))
	foundDiscovery := false
	var discoveryPayloadGot map[string]interface{}
	// A background goroutine (the relay subscription -- see the comment on the real deadlock bug
	// this avoids, discoveryimport.go) may already be publishing concurrently by this point, so
	// this must read through the thread-safe snapshot, not the raw field, even here.
	for _, p := range mainClient.publishedSnapshot() {
		if p.topic == wantDiscoveryTopic {
			foundDiscovery = true
			if err := json.Unmarshal(p.payload, &discoveryPayloadGot); err != nil {
				t.Fatalf("unmarshalling published discovery payload: %v", err)
			}
		}
	}
	if !foundDiscovery {
		t.Fatalf("expected local discovery config published to %q, got %v", wantDiscoveryTopic, mainClient.publishedSnapshot())
	}
	if discoveryPayloadGot["default_entity_id"] != localEntity {
		t.Errorf("default_entity_id = %v, want the Spaces.def-resolved %q", discoveryPayloadGot["default_entity_id"], localEntity)
	}

	// All three subscriptions (the discovery-config filter, the "home_assistant"-kind state-relay
	// wildcard, and the "hosts"-kind state-relay wildcard) are registered synchronously, once, up
	// front -- unlike the old design (a per-capability relay subscription dynamically spawned off a
	// background goroutine the first time each capability's own discovery CONFIG message was seen),
	// no wait is needed here.
	handlers := cloudClient.subscribedHandlersSnapshot()
	if len(handlers) != 3 {
		t.Fatalf("expected exactly 3 static subscriptions (config + home_assistant-kind state + hosts-kind state), got %d", len(handlers))
	}

	// Now simulate the remote's own raw value arriving on the cloud broker -- must be relayed
	// onto the local relay topic.
	wantLocalStateTopic := importedStateTopic(localEntity)
	for _, h := range handlers {
		h(cloudClient, fakeMessage{topic: remoteStateTopic, payload: []byte("612")})
	}
	found := false
	for _, p := range mainClient.publishedSnapshot() {
		if p.topic == wantLocalStateTopic && string(p.payload) == "612" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the remote value relayed onto %q, got %v", wantLocalStateTopic, mainClient.publishedSnapshot())
	}
}

// TestSubscribeImportedDevicesRelaysManyCapabilitiesAfterConfigBurst is a regression test for the
// real bug found live 2026-08-31: many capabilities' retained discovery CONFIG messages arrive in
// one burst at startup (Vienna's own 25+), and the old design dynamically spawned a separate
// cloudClient.Subscribe call per capability from within the config handler -- some of those
// concurrent subscriptions were silently lost, permanently killing the state relay for the
// affected capabilities. With the new static-wildcard-subscribe design there is exactly one state
// subscription per remote installation regardless of how many capabilities arrive in the burst, so
// every one of them must still relay correctly.
func TestSubscribeImportedDevicesRelaysManyCapabilitiesAfterConfigBurst(t *testing.T) {
	const n = 25
	devices := map[string]TImportedDevice{}
	retained := []fakeMessage{}
	for i := 0; i < n; i++ {
		deviceID := fmt.Sprintf("hass.device_%d", i)
		remoteLocal := fmt.Sprintf("sensor.infrastructural_device_%d_value", i)
		localEntity := fmt.Sprintf("sensor.physical_device_%d_value", i)
		devices[deviceID] = TImportedDevice{
			RemoteInstallation: "junglinster", RemoteDeviceID: deviceID,
			Capabilities: map[string]TImportedCapability{
				"value": {LocalEntity: localEntity},
			},
		}
		payload, err := json.Marshal(map[string]interface{}{
			"default_entity_id": remoteLocal,
			"state_topic":       "junglinster/homeassistant_instances/protocols-server-2/bridge/" + remoteLocal + "/state",
			"device":            map[string]interface{}{"identifiers": []string{deviceID}, "name": deviceID},
		})
		if err != nil {
			t.Fatalf("marshalling test payload %d: %v", i, err)
		}
		stableID := exportStableID("junglinster", deviceID, "value")
		topic := "junglinster/" + testPrefix + "/sensor/coordinator/" + stableID + "/config"
		retained = append(retained, fakeMessage{topic: topic, payload: payload})
	}
	importFile := TImportedFile{Devices: devices}

	cloudClient := &fakeClient{retained: retained}
	mainClient := &fakeClient{}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))

	if err := subscribeImportedDevices(mainClient, cloudClient, "junglinster", importFile, publisher, testPrefix, nil); err != nil {
		t.Fatalf("subscribeImportedDevices error: %v", err)
	}

	handlers := cloudClient.subscribedHandlersSnapshot()
	if len(handlers) != 3 {
		t.Fatalf("expected exactly 3 static subscriptions regardless of capability count, got %d", len(handlers))
	}

	// Simulate every capability's remote value arriving, and verify every single one relays --
	// not just the first (which the old bug's surviving subscription would still have caught).
	for i := 0; i < n; i++ {
		remoteLocal := fmt.Sprintf("sensor.infrastructural_device_%d_value", i)
		remoteStateTopic := "junglinster/homeassistant_instances/protocols-server-2/bridge/" + remoteLocal + "/state"
		for _, h := range handlers {
			h(cloudClient, fakeMessage{topic: remoteStateTopic, payload: []byte(fmt.Sprintf("%d", i))})
		}
	}

	published := mainClient.publishedSnapshot()
	for i := 0; i < n; i++ {
		localEntity := fmt.Sprintf("sensor.physical_device_%d_value", i)
		wantTopic := importedStateTopic(localEntity)
		wantPayload := fmt.Sprintf("%d", i)
		found := false
		for _, p := range published {
			if p.topic == wantTopic && string(p.payload) == wantPayload {
				found = true
			}
		}
		if !found {
			t.Errorf("capability %d: expected relay to %q = %q, not found in %v", i, wantTopic, wantPayload, published)
		}
	}
}

// TestSubscribeImportedDevicesHostsKindQualifiesTopicAndExtractsJSON is the "hosts"-kind
// counterpart of TestSubscribeImportedDevicesPublishesLocalDiscoveryAndRelaysValue: two
// capabilities ("cpu/load", "cpu/temperature") sharing one JSON-blob cloud discovery payload each
// with their own "value_template" ("hosts"-kind's own shape, homeassistant/discovery.go's
// buildDiscoveryConfigs) and a bare "state_topic" ("hosts/smarty/cpu/state", not self-qualifying).
// After their discovery configs arrive, a single incoming JSON-blob message on the real,
// cross-house-qualified topic ("hosts/smarty/junglinster/cpu/state", exactly what
// Integrations/cpu/report publishes when installation=junglinster) must fan out to BOTH local
// relay topics, each carrying just its own extracted field -- confirming the unified importer
// resolves "hosts"-kind sources purely from the payload's own StateTopic/ValueTemplate fields, with
// no exporter-side change and no DSL-declared "kind".
func TestSubscribeImportedDevicesHostsKindQualifiesTopicAndExtractsJSON(t *testing.T) {
	const loadLocalEntity = "sensor.physical_smarty_cpu_load"
	const temperatureLocalEntity = "sensor.physical_smarty_cpu_temperature"
	const nodeLocalEntity = "binary_sensor.physical_smarty_node"
	importFile := TImportedFile{Devices: map[string]TImportedDevice{
		"import.smarty": {
			RemoteInstallation: "junglinster", RemoteDeviceID: "host.smarty",
			Capabilities: map[string]TImportedCapability{
				"cpu/load":        {LocalEntity: loadLocalEntity},
				"cpu/temperature": {LocalEntity: temperatureLocalEntity},
				"node":            {LocalEntity: nodeLocalEntity},
			},
		},
	}}

	makeConfigPayload := func(defaultEntityID, stateTopic, valueTemplate string) []byte {
		data, err := json.Marshal(map[string]interface{}{
			"default_entity_id": defaultEntityID,
			"state_topic":       stateTopic,
			"value_template":    valueTemplate,
			"device":            map[string]interface{}{"identifiers": []string{"host.smarty"}, "name": "smarty"},
		})
		if err != nil {
			t.Fatalf("marshalling test payload: %v", err)
		}
		return data
	}
	loadStableID := exportStableID("junglinster", "host.smarty", "cpu/load")
	temperatureStableID := exportStableID("junglinster", "host.smarty", "cpu/temperature")
	nodeStableID := exportStableID("junglinster", "host.smarty", "node")
	retained := []fakeMessage{
		{topic: "junglinster/" + testPrefix + "/sensor/coordinator/" + loadStableID + "/config", payload: makeConfigPayload("sensor.junglinster_smarty_cpu_load", "hosts/smarty/cpu/state", "{{ value_json.load }}")},
		{topic: "junglinster/" + testPrefix + "/sensor/coordinator/" + temperatureStableID + "/config", payload: makeConfigPayload("sensor.junglinster_smarty_cpu_temperature", "hosts/smarty/cpu/state", "{{ value_json.temperature }}")},
		{topic: "junglinster/" + testPrefix + "/binary_sensor/coordinator/" + nodeStableID + "/config", payload: makeConfigPayload("binary_sensor.junglinster_smarty_node", "hosts/smarty/node/state", "")},
	}

	cloudClient := &fakeClient{retained: retained}
	mainClient := &fakeClient{}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))

	if err := subscribeImportedDevices(mainClient, cloudClient, "junglinster", importFile, publisher, testPrefix, nil); err != nil {
		t.Fatalf("subscribeImportedDevices error: %v", err)
	}

	// The real report script (Integrations/cpu/report) always publishes to the cross-house
	// qualified topic directly -- "hosts/<host>/<installation>/cpu/state" -- never the bare shape
	// the discovery payload's own state_topic field carries.
	qualifiedTopic := "hosts/smarty/junglinster/cpu/state"
	blob := []byte(`{"load": 42.3, "temperature": 19.5}`)
	for _, h := range cloudClient.subscribedHandlersSnapshot() {
		h(cloudClient, fakeMessage{topic: qualifiedTopic, payload: blob})
	}

	published := mainClient.publishedSnapshot()
	wantLoadTopic := importedStateTopic(loadLocalEntity)
	wantTemperatureTopic := importedStateTopic(temperatureLocalEntity)
	foundLoad, foundTemperature := false, false
	for _, p := range published {
		if p.topic == wantLoadTopic && string(p.payload) == "42.3" {
			foundLoad = true
		}
		if p.topic == wantTemperatureTopic && string(p.payload) == "19.5" {
			foundTemperature = true
		}
	}
	if !foundLoad {
		t.Errorf("expected %q extracted onto %q, got %v", "load", wantLoadTopic, published)
	}
	if !foundTemperature {
		t.Errorf("expected %q extracted onto %q, got %v", "temperature", wantTemperatureTopic, published)
	}

	// The real deployed report script never publishes a "node" value of its own -- but this side
	// needs no special handling for that (see TImportedHostsRelay's own doc comment): the
	// EXPORTING installation's own TCloudLivenessTracker (mqtt_relay.go) computes and cross-posts
	// it to the SAME qualified topic shape as any other "hosts"-kind attribute, so a plain node
	// message arriving here must relay exactly like "load"/"temperature" did above -- no
	// importer-side synthesis involved.
	wantNodeTopic := importedStateTopic(nodeLocalEntity)
	nodeQualifiedTopic := "hosts/smarty/junglinster/node/state"
	for _, h := range cloudClient.subscribedHandlersSnapshot() {
		h(cloudClient, fakeMessage{topic: nodeQualifiedTopic, payload: []byte("true")})
	}
	foundNodeTrue := false
	for _, p := range mainClient.publishedSnapshot() {
		if p.topic == wantNodeTopic && string(p.payload) == "true" {
			foundNodeTrue = true
		}
	}
	if !foundNodeTrue {
		t.Errorf("expected the exporter's own cross-posted liveness relayed onto %q = \"true\", got %v", wantNodeTopic, mainClient.publishedSnapshot())
	}
}

// TestSubscribeImportedDevicesHostsKindRelaysNonJSONPayloadVerbatim covers a "hosts"-kind "node"
// (liveness) capability, whose own discovery config carries no "value_template" at all (its raw
// payload is already a bare "true"/"false" scalar, never JSON-wrapped) -- must be relayed
// unmodified, not run through JSON extraction.
func TestSubscribeImportedDevicesHostsKindRelaysNonJSONPayloadVerbatim(t *testing.T) {
	const nodeLocalEntity = "binary_sensor.physical_smarty_node"
	importFile := TImportedFile{Devices: map[string]TImportedDevice{
		"import.smarty": {
			RemoteInstallation: "junglinster", RemoteDeviceID: "host.smarty",
			Capabilities: map[string]TImportedCapability{
				"node": {LocalEntity: nodeLocalEntity},
			},
		},
	}}

	configPayload, err := json.Marshal(map[string]interface{}{
		"default_entity_id": "binary_sensor.junglinster_smarty_node",
		"state_topic":       "hosts/smarty/node/state",
		"device":            map[string]interface{}{"identifiers": []string{"host.smarty"}, "name": "smarty"},
	})
	if err != nil {
		t.Fatalf("marshalling test payload: %v", err)
	}
	nodeStableID := exportStableID("junglinster", "host.smarty", "node")
	retained := []fakeMessage{{topic: "junglinster/" + testPrefix + "/binary_sensor/coordinator/" + nodeStableID + "/config", payload: configPayload}}

	cloudClient := &fakeClient{retained: retained}
	mainClient := &fakeClient{}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))

	if err := subscribeImportedDevices(mainClient, cloudClient, "junglinster", importFile, publisher, testPrefix, nil); err != nil {
		t.Fatalf("subscribeImportedDevices error: %v", err)
	}

	qualifiedTopic := "hosts/smarty/junglinster/node/state"
	for _, h := range cloudClient.subscribedHandlersSnapshot() {
		h(cloudClient, fakeMessage{topic: qualifiedTopic, payload: []byte("true")})
	}

	wantTopic := importedStateTopic(nodeLocalEntity)
	found := false
	for _, p := range mainClient.publishedSnapshot() {
		if p.topic == wantTopic && string(p.payload) == "true" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected raw %q relayed onto %q, got %v", "true", wantTopic, mainClient.publishedSnapshot())
	}
}

func TestSubscribeImportedHassBridgeDevicesNoopWithNoCloudClient(t *testing.T) {
	importFile := TImportedFile{Devices: map[string]TImportedDevice{
		"hass.vienna_terrace": {
			RemoteInstallation: "junglinster", RemoteDeviceID: "hass.vienna_terrace",
			Capabilities: map[string]TImportedCapability{
				"temperature": {LocalEntity: "sensor.physical_terrace_netatmo_temperature"},
			},
		},
	}}
	mainClient := &fakeClient{}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	if err := subscribeImportedDevices(mainClient, nil, "junglinster", importFile, publisher, testPrefix, nil); err != nil {
		t.Fatalf("subscribeImportedDevices error: %v", err)
	}
	if len(mainClient.published) != 0 {
		t.Errorf("expected a full no-op with no cloud client, got %v", mainClient.published)
	}
}
