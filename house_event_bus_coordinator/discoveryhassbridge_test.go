package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadHassBridgeFileMissingReturnsZeroValue(t *testing.T) {
	got, err := loadHassBridgeFile(filepath.Join(t.TempDir(), "does_not_exist.yaml"))
	if err != nil {
		t.Fatalf("loadHassBridgeFile error: %v", err)
	}
	if len(got.Devices) != 0 {
		t.Errorf("got %v, want a zero-value file for a missing path", got)
	}
}

func TestSubscribeHassBridgePublishesDiscoveryAndState(t *testing.T) {
	bridgeFile := THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.laserjet": {
			Instances:   []string{"protocols-server-2"},
			DisplayName: "infrastructural/house/downward_hallway/laserjet",
			Capabilities: map[string]THassBridgeCapability{
				"status": {
					SourceEntities: map[string]string{"protocols-server-2": "sensor.hewlett_packard_hp_laserjet_professional_p1102w"},
					LocalEntity:    "sensor.infrastructural_house_downward_hallway_laserjet_status",
				},
			},
			ConstantAttributes: map[string]TConceptualConstant{
				"model": {Value: "LaserJet P1102w"},
			},
		},
	}}

	localTopic := "homeassistant_instances/protocols-server-2/bridge/sensor.infrastructural_house_downward_hallway_laserjet_status/state"
	client := &fakeClient{retained: []fakeMessage{{topic: localTopic, payload: []byte("idle")}}}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	store := NewLiveDeviceInfoStore()

	if err := subscribeHassBridge(client, nil, "", bridgeFile, store, publisher, testPrefix, nil); err != nil {
		t.Fatalf("subscribeHassBridge error: %v", err)
	}

	wantTopic, ok := hassBridgeDiscoveryTopic("sensor.infrastructural_house_downward_hallway_laserjet_status", testPrefix)
	if !ok {
		t.Fatalf("hassBridgeDiscoveryTopic returned ok=false")
	}

	found := false
	for _, p := range client.published {
		if p.topic != wantTopic {
			continue
		}
		found = true
		var body map[string]interface{}
		if err := json.Unmarshal(p.payload, &body); err != nil {
			t.Fatalf("unmarshalling discovery payload: %v", err)
		}
		if body["default_entity_id"] != "sensor.infrastructural_house_downward_hallway_laserjet_status" {
			t.Errorf("default_entity_id = %v, want the local entity_id", body["default_entity_id"])
		}
		if body["state_topic"] != localTopic {
			t.Errorf("state_topic = %v, want %q (pure relay, no republishing)", body["state_topic"], localTopic)
		}
		if body["name"] != "infrastructural/house/downward_hallway/laserjet/status" {
			t.Errorf("name = %v, want the device location baked in ahead of the bare capability", body["name"])
		}
		device, ok := body["device"].(map[string]interface{})
		if !ok {
			t.Fatalf("device block missing or wrong shape: %v", body["device"])
		}
		if device["name"] != "infrastructural/house/downward_hallway/laserjet" {
			t.Errorf("device.name = %v, want the DisplayName", device["name"])
		}
		if device["model"] != "LaserJet P1102w" {
			t.Errorf("device.model = %v, want the declared constant attribute", device["model"])
		}
	}
	if !found {
		t.Errorf("expected a discovery config published to %q, got %v", wantTopic, client.published)
	}
}

// TestSubscribeHassBridgeFallsBackToLiveTypingWhenDeclaredEmpty is a regression test for a real
// gap found live 2026-09-06: a bridged capability with no explicit Physical.def/Defaults.def
// typing showed up with no unit_of_measurement/device_class at all, even though the remote
// entity it bridges from genuinely has both (e.g. a Fritz!Box's own gb_received sensor) --
// existence_tracker.LiveTyping (populated by the same inquiry reply that already answers "does
// this exist") must be consulted as a fallback.
func TestSubscribeHassBridgeFallsBackToLiveTypingWhenDeclaredEmpty(t *testing.T) {
	bridgeFile := THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.fritz_box": {
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"gb_received": {
					// No Unit/DeviceClass declared -- neither Physical.def nor Defaults.def set
					// anything for this capability.
					SourceEntities: map[string]string{"protocols-server-2": "sensor.fritz_box_7490_gb_received"},
					LocalEntity:    "sensor.infrastructural_fritz_box_gb_received",
				},
			},
		},
	}}

	existenceTracker := newEntityExistenceTracker("")
	existenceTracker.Seed(bridgeFile)
	existenceTracker.Record("protocols-server-2", "sensor.fritz_box_7490_gb_received", true, "1234", "GB", "data_size")

	localTopic := "homeassistant_instances/protocols-server-2/bridge/sensor.infrastructural_fritz_box_gb_received/state"
	client := &fakeClient{retained: []fakeMessage{{topic: localTopic, payload: []byte("1234")}}}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	store := NewLiveDeviceInfoStore()

	if err := subscribeHassBridge(client, nil, "", bridgeFile, store, publisher, testPrefix, existenceTracker); err != nil {
		t.Fatalf("subscribeHassBridge error: %v", err)
	}

	wantTopic, ok := hassBridgeDiscoveryTopic("sensor.infrastructural_fritz_box_gb_received", testPrefix)
	if !ok {
		t.Fatalf("hassBridgeDiscoveryTopic returned ok=false")
	}
	found := false
	for _, p := range client.published {
		if p.topic != wantTopic {
			continue
		}
		found = true
		var body map[string]interface{}
		if err := json.Unmarshal(p.payload, &body); err != nil {
			t.Fatalf("unmarshalling discovery payload: %v", err)
		}
		if body["unit_of_measurement"] != "GB" {
			t.Errorf("unit_of_measurement = %v, want the live-reported \"GB\" fallback", body["unit_of_measurement"])
		}
		if body["device_class"] != "data_size" {
			t.Errorf("device_class = %v, want the live-reported \"data_size\" fallback", body["device_class"])
		}
	}
	if !found {
		t.Errorf("expected a discovery config published to %q, got %v", wantTopic, client.published)
	}
}

func TestSubscribeHassBridgePublishesTypingMetadata(t *testing.T) {
	bridgeFile := THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"host.junglinster": {
			Instances: []string{"main"},
			Capabilities: map[string]THassBridgeCapability{
				"node": {
					SourceEntities: map[string]string{"main": "sensor.processor_use is available"},
					LocalEntity:    "binary_sensor.infrastructural_junglinster_node",
					DeviceClass:    "connectivity",
				},
				"cpu/load": {
					SourceEntities: map[string]string{"main": "sensor.processor_use"},
					LocalEntity:    "sensor.infrastructural_junglinster_cpu_load",
					Unit:           "%",
					Icon:           "mdi:cpu-64-bit",
					StateClass:     "measurement",
				},
			},
		},
	}}

	client := &fakeClient{retained: []fakeMessage{
		{topic: "homeassistant_instances/main/bridge/binary_sensor.infrastructural_junglinster_node/state", payload: []byte("ON")},
		{topic: "homeassistant_instances/main/bridge/sensor.infrastructural_junglinster_cpu_load/state", payload: []byte("12.3")},
	}}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	store := NewLiveDeviceInfoStore()

	if err := subscribeHassBridge(client, nil, "", bridgeFile, store, publisher, testPrefix, nil); err != nil {
		t.Fatalf("subscribeHassBridge error: %v", err)
	}

	byTopic := map[string]map[string]interface{}{}
	for _, p := range client.published {
		var body map[string]interface{}
		if err := json.Unmarshal(p.payload, &body); err != nil {
			t.Fatalf("unmarshalling discovery payload for %s: %v", p.topic, err)
		}
		byTopic[p.topic] = body
	}

	nodeTopic, _ := hassBridgeDiscoveryTopic("binary_sensor.infrastructural_junglinster_node", testPrefix)
	nodeBody, ok := byTopic[nodeTopic]
	if !ok || nodeBody["device_class"] != "connectivity" {
		t.Errorf("node's device_class = %v, want \"connectivity\"", byTopic[nodeTopic])
	}
	// Real bug found live 2026-08-31: without an explicit payload_on/payload_off, HA's MQTT
	// binary_sensor platform falls back to its own uppercase "ON"/"OFF" default, which never
	// matches this entity's actual lowercase "on"/"off" state -- leaving it stuck at "unknown".
	if nodeBody["payload_on"] != "on" || nodeBody["payload_off"] != "off" {
		t.Errorf("node's payload_on/payload_off = %v/%v, want on/off", nodeBody["payload_on"], nodeBody["payload_off"])
	}
	// Real gap found live 2026-08-31: the node entity must never gate its own availability on
	// itself (a circular reference) -- only its own self-availability check applies, so the list
	// has exactly one entry, on its own state topic.
	nodeStateTopic := "homeassistant_instances/main/bridge/binary_sensor.infrastructural_junglinster_node/state"
	if e := findAvailabilityEntry(nodeBody, nodeStateTopic); e == nil || e["payload_available"] != "available" {
		t.Errorf("node's own availability = %v, want a self-check entry on %q", nodeBody["availability"], nodeStateTopic)
	}
	if entries, _ := nodeBody["availability"].([]interface{}); len(entries) != 1 {
		t.Errorf("node's own availability = %v, want exactly 1 entry (self-check only, no device gate)", nodeBody["availability"])
	}

	loadTopic, _ := hassBridgeDiscoveryTopic("sensor.infrastructural_junglinster_cpu_load", testPrefix)
	loadBody, ok := byTopic[loadTopic]
	if !ok {
		t.Fatalf("expected a discovery config for cpu/load")
	}
	if loadBody["unit_of_measurement"] != "%" {
		t.Errorf("cpu/load's unit_of_measurement = %v, want \"%%\"", loadBody["unit_of_measurement"])
	}
	if loadBody["icon"] != "mdi:cpu-64-bit" {
		t.Errorf("cpu/load's icon = %v, want \"mdi:cpu-64-bit\"", loadBody["icon"])
	}
	if loadBody["state_class"] != "measurement" {
		t.Errorf("cpu/load's state_class = %v, want \"measurement\"", loadBody["state_class"])
	}
	if _, present := loadBody["device_class"]; present {
		t.Errorf("cpu/load's device_class = %v, want omitted (none declared)", loadBody["device_class"])
	}
	// Real gap found live 2026-08-31: an upstream integration may correctly track connectivity on
	// its own native entity while leaving every OTHER entity it reports frozen at its last value
	// with no "unavailable" signal at all -- cpu/load's own config must gate its availability on
	// the SAME device's node topic, so HA shows it unavailable exactly when the device itself is.
	// Refined the same day: node being reachable doesn't guarantee THIS reading is itself valid,
	// so availability is the AND (availability_mode "all") of the node topic AND cpu/load's own
	// state topic never itself reporting the literal string "unavailable".
	if loadBody["availability_mode"] != "all" {
		t.Errorf("cpu/load's availability_mode = %v, want \"all\"", loadBody["availability_mode"])
	}
	wantNodeTopic := "homeassistant_instances/main/bridge/binary_sensor.infrastructural_junglinster_node/state"
	if e := findAvailabilityEntry(loadBody, wantNodeTopic); e == nil || e["payload_available"] != "on" || e["payload_not_available"] != "off" {
		t.Errorf("cpu/load's availability = %v, want a node-gate entry on %q (on/off)", loadBody["availability"], wantNodeTopic)
	}
	wantOwnTopic := "homeassistant_instances/main/bridge/sensor.infrastructural_junglinster_cpu_load/state"
	if e := findAvailabilityEntry(loadBody, wantOwnTopic); e == nil || e["payload_available"] != "available" {
		t.Errorf("cpu/load's availability = %v, want a self-check entry on its own state topic %q", loadBody["availability"], wantOwnTopic)
	}
}

func TestSubscribeHassBridgeNoDevicesIsNoop(t *testing.T) {
	client := &fakeClient{}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	if err := subscribeHassBridge(client, nil, "", THassBridgeFile{}, NewLiveDeviceInfoStore(), publisher, testPrefix, nil); err != nil {
		t.Fatalf("subscribeHassBridge error: %v", err)
	}
	if len(client.published) != 0 || len(client.subscribedHandlers) != 0 {
		t.Errorf("expected a full no-op with no devices declared, got published=%v subscribed=%d", client.published, len(client.subscribedHandlers))
	}
}

func TestSubscribeHassBridgeDeviceInfoUpdatesStoreAndRepublishesCapabilities(t *testing.T) {
	bridgeFile := THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"host.junglinster": {
			Instances: []string{"main"},
			Capabilities: map[string]THassBridgeCapability{
				"cpu/load": {SourceEntities: map[string]string{"main": "sensor.processor_use"}, LocalEntity: "sensor.infrastructural_junglinster_cpu_load"},
			},
			ConstantAttributes: map[string]TConceptualConstant{
				"model": {Value: "Home Assistant Green"},
			},
		},
	}}

	deviceInfoTopic := "homeassistant_instances/main/bridge/device/host.junglinster/state"
	deviceInfoPayload := []byte(`{"sw_version": "Home Assistant Operating System 2025.1.0"}`)
	client := &fakeClient{retained: []fakeMessage{{topic: deviceInfoTopic, payload: deviceInfoPayload}}}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	store := NewLiveDeviceInfoStore()

	if err := subscribeHassBridgeDeviceInfo(client, nil, "", bridgeFile, store, publisher, testPrefix, nil); err != nil {
		t.Fatalf("subscribeHassBridgeDeviceInfo error: %v", err)
	}

	if snap := store.Snapshot("host.junglinster"); snap["sw_version"] != "Home Assistant Operating System 2025.1.0" {
		t.Errorf("store snapshot = %v, want sw_version to be updated", snap)
	}

	wantTopic, ok := hassBridgeDiscoveryTopic("sensor.infrastructural_junglinster_cpu_load", testPrefix)
	if !ok {
		t.Fatalf("hassBridgeDiscoveryTopic returned ok=false")
	}
	found := false
	for _, p := range client.published {
		if p.topic != wantTopic {
			continue
		}
		found = true
		var body map[string]interface{}
		if err := json.Unmarshal(p.payload, &body); err != nil {
			t.Fatalf("unmarshalling discovery payload: %v", err)
		}
		device, ok := body["device"].(map[string]interface{})
		if !ok {
			t.Fatalf("device block missing or wrong shape: %v", body["device"])
		}
		if device["sw_version"] != "Home Assistant Operating System 2025.1.0" {
			t.Errorf("device.sw_version = %v, want the freshly live-reported value", device["sw_version"])
		}
		if device["model"] != "Home Assistant Green" {
			t.Errorf("device.model = %v, want the static constant attribute still present", device["model"])
		}
	}
	if !found {
		t.Errorf("expected cpu/load's discovery config to be republished with fresh device info, got %v", client.published)
	}
}

// TestSubscribeHassBridgeDeviceInfoSkipsCrossPostForSelfImportEcho is
// TestSubscribeHassBridgeSkipsCrossPostForSelfImportEcho's device-info counterpart: the identical
// feedback-loop risk exists on subscribeHassBridgeDeviceInfo's own six-segment device-info topic
// (deviceInfoHandler in subscribeHassBridgeSelfImport relays onto
// "homeassistant_instances/<ownInstallation>/bridge/device/<id>/state", a synthetic instance name
// this device never actually declared).
func TestSubscribeHassBridgeDeviceInfoSkipsCrossPostForSelfImportEcho(t *testing.T) {
	bridgeFile := THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.eriks_iphone": {
			Instances: []string{"protocols-server-2"}, Export: true, ExportAs: "roaming", SelfImportFrom: "junglinster",
			Capabilities: map[string]THassBridgeCapability{
				"battery_level": {LocalEntity: "sensor.infrastructural_eriks_iphone_battery_level"},
			},
		},
	}}

	echoTopic := "homeassistant_instances/junglinster/bridge/device/hass.eriks_iphone/state"
	client := &fakeClient{retained: []fakeMessage{{topic: echoTopic, payload: []byte(`{"manufacturer": "Apple"}`)}}}
	cloudClient := &fakeClient{}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	store := NewLiveDeviceInfoStore()

	if err := subscribeHassBridgeDeviceInfo(client, cloudClient, "junglinster", bridgeFile, store, publisher, testPrefix, nil); err != nil {
		t.Fatalf("subscribeHassBridgeDeviceInfo error: %v", err)
	}

	if len(cloudClient.publishedSnapshot()) != 0 {
		t.Errorf("expected NO cross-post to cloud for a self-import device-info echo (would loop forever), got %v", cloudClient.publishedSnapshot())
	}
}

func TestSubscribeHassBridgeExportCrossPostsToCloud(t *testing.T) {
	bridgeFile := THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.vienna_terrace": {
			Instances: []string{"protocols-server-2"},
			Export:    true,
			Capabilities: map[string]THassBridgeCapability{
				"temperature": {
					SourceEntities: map[string]string{"protocols-server-2": "sensor.vienna_terrace_temperature"},
					LocalEntity:    "sensor.infrastructural_vienna_terrace_temperature",
				},
			},
		},
	}}

	localTopic := "homeassistant_instances/protocols-server-2/bridge/sensor.infrastructural_vienna_terrace_temperature/state"
	client := &fakeClient{retained: []fakeMessage{{topic: localTopic, payload: []byte("21.4")}}}
	cloudClient := &fakeClient{}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	store := NewLiveDeviceInfoStore()

	if err := subscribeHassBridge(client, cloudClient, "junglinster", bridgeFile, store, publisher, testPrefix, nil); err != nil {
		t.Fatalf("subscribeHassBridge error: %v", err)
	}

	wantRawTopic := "junglinster/" + localTopic
	foundRaw := false
	for _, p := range cloudClient.published {
		if p.topic == wantRawTopic && string(p.payload) == "21.4" {
			foundRaw = true
		}
	}
	if !foundRaw {
		t.Errorf("expected raw state cross-posted to %q with the original payload, got %v", wantRawTopic, cloudClient.published)
	}

	stableID := exportStableID("junglinster", "hass.vienna_terrace", "temperature")
	discoveryTopic, ok := hassBridgeCloudDiscoveryTopic("sensor.infrastructural_vienna_terrace_temperature", stableID, testPrefix)
	if !ok {
		t.Fatalf("hassBridgeCloudDiscoveryTopic returned ok=false")
	}
	wantCloudDiscoveryTopic := "junglinster/" + discoveryTopic
	foundDiscovery := false
	for _, p := range cloudClient.published {
		if p.topic != wantCloudDiscoveryTopic {
			continue
		}
		foundDiscovery = true
		var body map[string]interface{}
		if err := json.Unmarshal(p.payload, &body); err != nil {
			t.Fatalf("unmarshalling cloud discovery payload: %v", err)
		}
		if body["state_topic"] != wantRawTopic {
			t.Errorf("cloud discovery state_topic = %v, want the qualified cross-posted topic %q (must be functional, not the bare local one)", body["state_topic"], wantRawTopic)
		}
		if body["installation"] != "junglinster" {
			t.Errorf("cloud discovery installation = %v, want \"junglinster\"", body["installation"])
		}
	}
	if !foundDiscovery {
		t.Errorf("expected a discovery config published to cloud at %q, got %v", wantCloudDiscoveryTopic, cloudClient.published)
	}

	// The local/main copy must be untouched -- still the pure relay, bare local state_topic.
	for _, p := range client.published {
		var body map[string]interface{}
		if err := json.Unmarshal(p.payload, &body); err != nil {
			continue
		}
		if body["state_topic"] != localTopic {
			t.Errorf("local discovery state_topic = %v, want the bare local topic %q unaffected by export", body["state_topic"], localTopic)
		}
	}
}

// TestSubscribeHassBridgeSkipsCrossPostForSelfImportEcho is a regression test for a real bug found
// live 2026-09-05, the first time "roaming"+"import" was ever exercised for a real device:
// subscribeHassBridgeSelfImport relays a sibling's cloud-published traffic onto a SYNTHETIC local
// topic named after ownInstallation (e.g. "homeassistant_instances/junglinster/bridge/.../state")
// -- "junglinster" is never a genuinely declared instance (real ones are "main"/
// "protocols-server-2"). Before this fix, subscribeHassBridge's own wildcard treated that as fresh
// local traffic and cross-posted it right back to cloud under "roaming", which the self-import
// relay was ALSO subscribed to -- republishing it locally again, forever. This confirms a message
// whose reporting-instance segment isn't one of the device's own declared Instances is relayed
// locally (main copy still needs it) but never cross-posted.
func TestSubscribeHassBridgeSkipsCrossPostForSelfImportEcho(t *testing.T) {
	bridgeFile := THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.eriks_iphone": {
			Instances: []string{"protocols-server-2"}, Export: true, ExportAs: "roaming", SelfImportFrom: "junglinster",
			Capabilities: map[string]THassBridgeCapability{
				"battery_level": {LocalEntity: "sensor.infrastructural_eriks_iphone_battery_level"},
			},
		},
	}}

	// Exactly what subscribeHassBridgeSelfImport's own relay handler publishes: ownInstallation
	// ("junglinster") used as the topic's instance segment, never a real declared one.
	echoTopic := "homeassistant_instances/junglinster/bridge/sensor.infrastructural_eriks_iphone_battery_level/state"
	client := &fakeClient{retained: []fakeMessage{{topic: echoTopic, payload: []byte("87")}}}
	cloudClient := &fakeClient{}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	store := NewLiveDeviceInfoStore()

	if err := subscribeHassBridge(client, cloudClient, "junglinster", bridgeFile, store, publisher, testPrefix, nil); err != nil {
		t.Fatalf("subscribeHassBridge error: %v", err)
	}

	if len(cloudClient.publishedSnapshot()) != 0 {
		t.Errorf("expected NO cross-post to cloud for a self-import echo (would loop forever), got %v", cloudClient.publishedSnapshot())
	}
	// The local/main discovery config still needs to be published/refreshed -- only the cloud
	// cross-post is suppressed.
	foundLocal := false
	for _, p := range client.publishedSnapshot() {
		var body map[string]interface{}
		if err := json.Unmarshal(p.payload, &body); err == nil && body["state_topic"] == echoTopic {
			foundLocal = true
		}
	}
	if !foundLocal {
		t.Errorf("expected the local discovery config still published pointing at %q, got %v", echoTopic, client.publishedSnapshot())
	}
}

// TestSubscribeHassBridgeCanonicalizesRoamingCloudTopic is a regression test for the surprise the
// user flagged live 2026-09-05: a roaming device reachable via more than one real local instance
// cross-posted to a DIFFERENT cloud topic depending on which instance last reported
// ("roaming/homeassistant_instances/main/bridge/.../state" vs ".../protocols-server-2/bridge/..."),
// forcing an importer to watch both. The cloud copy's instance segment must always be
// canonicalRoamingInstance ("main"), regardless of which real declared instance actually published.
func TestSubscribeHassBridgeCanonicalizesRoamingCloudTopic(t *testing.T) {
	bridgeFile := THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.eriks_iphone": {
			Instances: []string{"main", "protocols-server-2"}, Export: true, ExportAs: "roaming",
			Capabilities: map[string]THassBridgeCapability{
				"battery_level": {LocalEntity: "sensor.infrastructural_eriks_iphone_battery_level"},
			},
		},
	}}

	realTopic := "homeassistant_instances/protocols-server-2/bridge/sensor.infrastructural_eriks_iphone_battery_level/state"
	client := &fakeClient{retained: []fakeMessage{{topic: realTopic, payload: []byte("87")}}}
	cloudClient := &fakeClient{}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	store := NewLiveDeviceInfoStore()

	if err := subscribeHassBridge(client, cloudClient, "junglinster", bridgeFile, store, publisher, testPrefix, nil); err != nil {
		t.Fatalf("subscribeHassBridge error: %v", err)
	}

	wantCanonicalTopic := "roaming/homeassistant_instances/main/bridge/sensor.infrastructural_eriks_iphone_battery_level/state"
	foundRaw := false
	for _, p := range cloudClient.publishedSnapshot() {
		if p.topic == wantCanonicalTopic && string(p.payload) == "87" {
			foundRaw = true
		}
		if strings.Contains(p.topic, "protocols-server-2") {
			t.Errorf("cloud publish %q must never carry the real reporting instance, want it canonicalized to %q", p.topic, canonicalRoamingInstance)
		}
	}
	if !foundRaw {
		t.Errorf("expected raw state cross-posted to canonicalized topic %q, got %v", wantCanonicalTopic, cloudClient.publishedSnapshot())
	}

	stableID := exportStableID("roaming", "hass.eriks_iphone", "battery_level")
	discoveryTopic, ok := hassBridgeCloudDiscoveryTopic("sensor.infrastructural_eriks_iphone_battery_level", stableID, testPrefix)
	if !ok {
		t.Fatalf("hassBridgeCloudDiscoveryTopic returned ok=false")
	}
	wantCloudDiscoveryTopic := "roaming/" + discoveryTopic
	foundDiscovery := false
	for _, p := range cloudClient.publishedSnapshot() {
		if p.topic != wantCloudDiscoveryTopic {
			continue
		}
		foundDiscovery = true
		var body map[string]interface{}
		if err := json.Unmarshal(p.payload, &body); err != nil {
			t.Fatalf("unmarshalling cloud discovery payload: %v", err)
		}
		if body["state_topic"] != wantCanonicalTopic {
			t.Errorf("cloud discovery state_topic = %v, want the canonicalized topic %q", body["state_topic"], wantCanonicalTopic)
		}
	}
	if !foundDiscovery {
		t.Errorf("expected a discovery config published to cloud at %q, got %v", wantCloudDiscoveryTopic, cloudClient.publishedSnapshot())
	}
}

// TestSubscribeHassBridgeDeviceInfoCanonicalizesRoamingCloudTopic is
// TestSubscribeHassBridgeCanonicalizesRoamingCloudTopic's device-info counterpart:
// hassBridgeDefaultInstance(device) alone (device.Instances[0]) isn't a reliable canonicalization --
// it depends on declaration order, e.g. here Instances[0] is "protocols-server-2", not "main". The
// cloud-bound state_topic must still land on canonicalRoamingInstance ("main") regardless.
func TestSubscribeHassBridgeDeviceInfoCanonicalizesRoamingCloudTopic(t *testing.T) {
	bridgeFile := THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.eriks_iphone": {
			Instances: []string{"protocols-server-2", "main"}, Export: true, ExportAs: "roaming",
			Capabilities: map[string]THassBridgeCapability{
				"battery_level": {LocalEntity: "sensor.infrastructural_eriks_iphone_battery_level"},
			},
		},
	}}

	deviceInfoTopic := "homeassistant_instances/protocols-server-2/bridge/device/hass.eriks_iphone/state"
	deviceInfoPayload := []byte(`{"manufacturer": "Apple"}`)
	client := &fakeClient{retained: []fakeMessage{{topic: deviceInfoTopic, payload: deviceInfoPayload}}}
	cloudClient := &fakeClient{}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	store := NewLiveDeviceInfoStore()

	if err := subscribeHassBridgeDeviceInfo(client, cloudClient, "junglinster", bridgeFile, store, publisher, testPrefix, nil); err != nil {
		t.Fatalf("subscribeHassBridgeDeviceInfo error: %v", err)
	}

	wantCanonicalTopic := "roaming/homeassistant_instances/main/bridge/sensor.infrastructural_eriks_iphone_battery_level/state"
	found := false
	for _, p := range cloudClient.publishedSnapshot() {
		var body map[string]interface{}
		if err := json.Unmarshal(p.payload, &body); err != nil {
			continue
		}
		if stateTopic, ok := body["state_topic"]; ok {
			if strings.Contains(stateTopic.(string), "protocols-server-2") {
				t.Errorf("cloud discovery state_topic = %v, must never carry the real reporting instance", stateTopic)
			}
			if stateTopic == wantCanonicalTopic {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected a cloud discovery config with state_topic %q, got %v", wantCanonicalTopic, cloudClient.publishedSnapshot())
	}
}

// TestSubscribeHassBridgeExportCloudAvailabilityTopicIsInstallationQualified covers the cloud-side
// counterpart of the availability wiring: the cross-posted discovery config's availability_topic
// must point at the CLOUD-qualified node topic (same qualification its own state_topic already
// gets), not the bare local one the "main" broker's own copy uses.
func TestSubscribeHassBridgeExportCloudAvailabilityTopicIsInstallationQualified(t *testing.T) {
	bridgeFile := THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.vienna_terrace": {
			Instances: []string{"protocols-server-2"},
			Export:    true,
			Capabilities: map[string]THassBridgeCapability{
				"node": {
					SourceEntities: map[string]string{"protocols-server-2": "sensor.vienna_terrace_connectivity is available"},
					LocalEntity:    "binary_sensor.infrastructural_vienna_terrace_node",
				},
				"temperature": {
					SourceEntities: map[string]string{"protocols-server-2": "sensor.vienna_terrace_temperature"},
					LocalEntity:    "sensor.infrastructural_vienna_terrace_temperature",
				},
			},
		},
	}}

	localTemperatureTopic := "homeassistant_instances/protocols-server-2/bridge/sensor.infrastructural_vienna_terrace_temperature/state"
	client := &fakeClient{retained: []fakeMessage{{topic: localTemperatureTopic, payload: []byte("21.4")}}}
	cloudClient := &fakeClient{}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	store := NewLiveDeviceInfoStore()

	if err := subscribeHassBridge(client, cloudClient, "junglinster", bridgeFile, store, publisher, testPrefix, nil); err != nil {
		t.Fatalf("subscribeHassBridge error: %v", err)
	}

	stableID := exportStableID("junglinster", "hass.vienna_terrace", "temperature")
	discoveryTopic, ok := hassBridgeCloudDiscoveryTopic("sensor.infrastructural_vienna_terrace_temperature", stableID, testPrefix)
	if !ok {
		t.Fatalf("hassBridgeCloudDiscoveryTopic returned ok=false")
	}
	wantCloudDiscoveryTopic := "junglinster/" + discoveryTopic
	wantNodeTopic := "junglinster/homeassistant_instances/protocols-server-2/bridge/binary_sensor.infrastructural_vienna_terrace_node/state"
	wantOwnTopic := "junglinster/" + localTemperatureTopic

	found := false
	for _, p := range cloudClient.published {
		if p.topic != wantCloudDiscoveryTopic {
			continue
		}
		found = true
		var body map[string]interface{}
		if err := json.Unmarshal(p.payload, &body); err != nil {
			t.Fatalf("unmarshalling cloud discovery payload: %v", err)
		}
		if e := findAvailabilityEntry(body, wantNodeTopic); e == nil || e["payload_available"] != "on" || e["payload_not_available"] != "off" {
			t.Errorf("cloud availability = %v, want a node-gate entry on the installation-qualified %q (on/off)", body["availability"], wantNodeTopic)
		}
		if e := findAvailabilityEntry(body, wantOwnTopic); e == nil || e["payload_available"] != "available" {
			t.Errorf("cloud availability = %v, want a self-check entry on the installation-qualified own state topic %q", body["availability"], wantOwnTopic)
		}
	}
	if !found {
		t.Fatalf("expected a discovery config published to cloud at %q, got %v", wantCloudDiscoveryTopic, cloudClient.published)
	}
}

func TestSubscribeHassBridgeNonExportDeviceNeverPublishesToCloud(t *testing.T) {
	bridgeFile := THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.laserjet": {
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"status": {
					SourceEntities: map[string]string{"protocols-server-2": "sensor.hewlett_packard_hp_laserjet_professional_p1102w"},
					LocalEntity:    "sensor.infrastructural_house_downward_hallway_laserjet_status",
				},
			},
		},
	}}

	localTopic := "homeassistant_instances/protocols-server-2/bridge/sensor.infrastructural_house_downward_hallway_laserjet_status/state"
	client := &fakeClient{retained: []fakeMessage{{topic: localTopic, payload: []byte("idle")}}}
	cloudClient := &fakeClient{}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	store := NewLiveDeviceInfoStore()

	if err := subscribeHassBridge(client, cloudClient, "junglinster", bridgeFile, store, publisher, testPrefix, nil); err != nil {
		t.Fatalf("subscribeHassBridge error: %v", err)
	}

	if len(cloudClient.published) != 0 {
		t.Errorf("non-export device must never publish to the cloud broker, got %v", cloudClient.published)
	}
}

// TestExpectedHassBridgeCloudTopicsOnlyIncludesExportedDevices is the regression test for a real
// bug found live 2026-08-30: without this function's result merged into main.go's cloud-side
// expected-topics set, watchForOrphanedDiscoveryTopics retired every hassbridge export discovery
// config within seconds of it being published, since it wasn't in the (then hosts-only) expected
// set at all -- confirmed live via journalctl showing "[discovery-cleanup] retired stale topic
// junglinster/homeassistant/.../hassbridge_.../config" moments after the Vienna devices' first
// real export traffic arrived.
func TestExpectedHassBridgeCloudTopicsOnlyIncludesExportedDevices(t *testing.T) {
	bridgeFile := THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.vienna_terrace": {
			Instances: []string{"protocols-server-2"}, Export: true,
			Capabilities: map[string]THassBridgeCapability{
				"temperature": {LocalEntity: "sensor.infrastructural_vienna_terrace_temperature"},
			},
		},
		"hass.laserjet": {
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"status": {LocalEntity: "sensor.infrastructural_house_downward_hallway_laserjet_status"},
			},
		},
	}}

	got := expectedHassBridgeCloudTopics(bridgeFile, "junglinster", testPrefix)

	exportedStableID := exportStableID("junglinster", "hass.vienna_terrace", "temperature")
	exportedTopic, ok := hassBridgeCloudDiscoveryTopic("sensor.infrastructural_vienna_terrace_temperature", exportedStableID, testPrefix)
	if !ok {
		t.Fatalf("hassBridgeCloudDiscoveryTopic returned ok=false")
	}
	if !got["junglinster/"+exportedTopic] {
		t.Errorf("expected %v to contain the exported device's qualified topic %q", got, "junglinster/"+exportedTopic)
	}

	nonExportedStableID := exportStableID("junglinster", "hass.laserjet", "status")
	nonExportedTopic, ok := hassBridgeCloudDiscoveryTopic("sensor.infrastructural_house_downward_hallway_laserjet_status", nonExportedStableID, testPrefix)
	if !ok {
		t.Fatalf("hassBridgeCloudDiscoveryTopic returned ok=false")
	}
	if got["junglinster/"+nonExportedTopic] {
		t.Errorf("expected %v to NOT contain the non-exported device's topic %q -- only Export=true devices belong on the cloud broker", got, "junglinster/"+nonExportedTopic)
	}
	if len(got) != 1 {
		t.Errorf("got %d topics %v, want exactly 1 (only the exported device's one capability)", len(got), got)
	}
}

// TestExpectedHassBridgeCloudTopicsEmptyWithNoInstallation confirms the "no installation
// configured" guard -- can't qualify a topic without one, so return nothing rather than an
// unqualified (and therefore wrong/colliding) topic.
func TestExpectedHassBridgeCloudTopicsEmptyWithNoInstallation(t *testing.T) {
	bridgeFile := THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.vienna_terrace": {
			Instances: []string{"protocols-server-2"}, Export: true,
			Capabilities: map[string]THassBridgeCapability{
				"temperature": {LocalEntity: "sensor.infrastructural_vienna_terrace_temperature"},
			},
		},
	}}
	if got := expectedHassBridgeCloudTopics(bridgeFile, "", testPrefix); len(got) != 0 {
		t.Errorf("got %v, want an empty set when ownInstallation is \"\"", got)
	}
}

func TestExpectedHassBridgeTopicsMatchesPublishedTopic(t *testing.T) {
	bridgeFile := THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.laserjet": {
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"status": {
					SourceEntities: map[string]string{"protocols-server-2": "sensor.hewlett_packard_hp_laserjet_professional_p1102w"},
					LocalEntity:    "sensor.infrastructural_house_downward_hallway_laserjet_status",
				},
			},
		},
	}}

	expected := expectedHassBridgeTopics(bridgeFile, testPrefix)
	wantTopic, ok := hassBridgeDiscoveryTopic("sensor.infrastructural_house_downward_hallway_laserjet_status", testPrefix)
	if !ok {
		t.Fatalf("hassBridgeDiscoveryTopic returned ok=false")
	}
	if !expected[wantTopic] {
		t.Errorf("expectedHassBridgeTopics = %v, want it to contain %q", expected, wantTopic)
	}
}

// --- exportQualifier / "roaming" / self-import (2026-09-02) ---

// TestExportStableIDIndependentOfLocalNaming is the regression test for the robustness gap found
// live 2026-09-07: the cloud stable id must depend only on (installation, deviceID, capability),
// never on how this house's own Spaces.def happens to position/name the local entity -- two wildly
// different local entity names for the exact same declaration must still produce the identical
// cloud stable id.
func TestExportStableIDIndependentOfLocalNaming(t *testing.T) {
	got := exportStableID("junglinster", "hass.vienna_livingroom", "node")
	want := "junglinster_hass_vienna_livingroom_node"
	if got != want {
		t.Errorf("exportStableID = %q, want %q", got, want)
	}
}

func TestExportQualifierUsesExportAsWhenSet(t *testing.T) {
	device := THassBridgeDevice{ExportAs: "roaming"}
	if got := exportQualifier(device, "junglinster"); got != "roaming" {
		t.Errorf("exportQualifier = %q, want %q (ExportAs wins over ownInstallation)", got, "roaming")
	}
}

func TestExportQualifierFallsBackToOwnInstallation(t *testing.T) {
	device := THassBridgeDevice{}
	if got := exportQualifier(device, "junglinster"); got != "junglinster" {
		t.Errorf("exportQualifier = %q, want %q (today's unchanged behaviour when ExportAs is unset)", got, "junglinster")
	}
}

// TestExpectedHassBridgeCloudTopicsUsesExportAsQualifier confirms a "roaming"-exported device's
// cloud topic is qualified with "roaming", not this coordinator's own ownInstallation.
func TestExpectedHassBridgeCloudTopicsUsesExportAsQualifier(t *testing.T) {
	bridgeFile := THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.eriks_iphone": {
			Instances: []string{"protocols-server-2"}, Export: true, ExportAs: "roaming",
			Capabilities: map[string]THassBridgeCapability{
				"battery_level": {LocalEntity: "sensor.infrastructural_eriks_iphone_battery_level"},
			},
		},
	}}

	got := expectedHassBridgeCloudTopics(bridgeFile, "junglinster", testPrefix)

	stableID := exportStableID("roaming", "hass.eriks_iphone", "battery_level")
	wantTopic, ok := hassBridgeCloudDiscoveryTopic("sensor.infrastructural_eriks_iphone_battery_level", stableID, testPrefix)
	if !ok {
		t.Fatalf("hassBridgeCloudDiscoveryTopic returned ok=false")
	}
	if !got["roaming/"+wantTopic] {
		t.Errorf("got %v, want it qualified with \"roaming/\", not \"junglinster/\"", got)
	}
	if got["junglinster/"+wantTopic] {
		t.Errorf("got %v, must NOT also contain the ownInstallation-qualified topic", got)
	}
}

// TestSubscribeHassBridgeSelfImportRelaysStateOntoLocalTopic confirms a self-imported device's
// cloud-broker traffic (published under its own ExportAs qualifier by a real -- possibly sibling
// -- installation) gets relayed verbatim onto this instance's own bare local bridge topic, the
// SAME topic subscribeHassBridge's own local handler already treats as this entity's live-value
// source -- no second discovery entity, just feeding the one that already exists.
func TestSubscribeHassBridgeSelfImportRelaysStateOntoLocalTopic(t *testing.T) {
	bridgeFile := THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.eriks_iphone": {
			Instances: []string{"protocols-server-2"}, Export: true, ExportAs: "roaming", SelfImportFrom: "junglinster",
			Capabilities: map[string]THassBridgeCapability{
				"battery_level": {LocalEntity: "sensor.infrastructural_eriks_iphone_battery_level"},
			},
		},
	}}

	// Published by a sibling installation (protocols-server-2) under the shared "roaming" qualifier.
	cloudTopic := "roaming/homeassistant_instances/protocols-server-2/bridge/sensor.infrastructural_eriks_iphone_battery_level/state"
	cloudClient := &fakeClient{retained: []fakeMessage{{topic: cloudTopic, payload: []byte("87")}}}
	mainClient := &fakeClient{}

	if err := subscribeHassBridgeSelfImport(cloudClient, mainClient, "junglinster", bridgeFile); err != nil {
		t.Fatalf("subscribeHassBridgeSelfImport error: %v", err)
	}

	wantTopic := "homeassistant_instances/junglinster/bridge/sensor.infrastructural_eriks_iphone_battery_level/state"
	published := mainClient.publishedSnapshot()
	found := false
	for _, p := range published {
		if p.topic == wantTopic && string(p.payload) == "87" {
			found = true
		}
	}
	if !found {
		t.Errorf("mainClient published = %v, want a publish to %q with payload \"87\"", published, wantTopic)
	}
}

// TestSubscribeHassBridgeSelfImportSkipsDevicesWithoutSelfImportFrom confirms a device that
// declared "roaming"/"export" but NOT "import" is never relayed -- SelfImportFrom empty means
// this installation never asked to see this data reflected back onto its own local topic.
func TestSubscribeHassBridgeSelfImportSkipsDevicesWithoutSelfImportFrom(t *testing.T) {
	bridgeFile := THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.eriks_iphone": {
			Instances: []string{"protocols-server-2"}, Export: true, ExportAs: "roaming",
			Capabilities: map[string]THassBridgeCapability{
				"battery_level": {LocalEntity: "sensor.infrastructural_eriks_iphone_battery_level"},
			},
		},
	}}
	cloudClient := &fakeClient{}
	mainClient := &fakeClient{}

	if err := subscribeHassBridgeSelfImport(cloudClient, mainClient, "junglinster", bridgeFile); err != nil {
		t.Fatalf("subscribeHassBridgeSelfImport error: %v", err)
	}
	if len(cloudClient.subscribedHandlersSnapshot()) != 0 {
		t.Errorf("expected no cloud subscriptions at all when no device declares SelfImportFrom")
	}
}

// TestSubscribeHassBridgeSelfImportNoopWhenCloudClientNil confirms no panic/subscription attempt
// when this house has no cloud broker configured.
func TestSubscribeHassBridgeSelfImportNoopWhenCloudClientNil(t *testing.T) {
	bridgeFile := THassBridgeFile{Devices: map[string]THassBridgeDevice{
		"hass.eriks_iphone": {
			Instances: []string{"protocols-server-2"}, Export: true, ExportAs: "roaming", SelfImportFrom: "junglinster",
			Capabilities: map[string]THassBridgeCapability{
				"battery_level": {LocalEntity: "sensor.infrastructural_eriks_iphone_battery_level"},
			},
		},
	}}
	if err := subscribeHassBridgeSelfImport(nil, &fakeClient{}, "junglinster", bridgeFile); err != nil {
		t.Fatalf("subscribeHassBridgeSelfImport error: %v", err)
	}
}
