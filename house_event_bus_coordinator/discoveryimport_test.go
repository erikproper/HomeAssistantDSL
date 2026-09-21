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

	body := buildImportedDiscoveryBody(match, payload, nil, "", nil, "test")
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
	origin, ok := body["origin"].(map[string]interface{})
	if !ok || origin["name"] != "House Event Bus Coordinator (test)" {
		t.Errorf("origin = %v, want {\"name\": \"House Event Bus Coordinator (test)\"}", body["origin"])
	}
}

// TestBuildImportedDiscoveryBodySetsViaDeviceWhenTargetAlsoImported is PROJECT.md item 3/
// plans/via-device-inference.md's Phase 2: the exporter's own via_device (Junglinster's local
// "hass.vienna_livingroom") must be translated into whichever of THIS house's own imports
// corresponds to that same remote device -- found purely via the (remoteInstallation,
// RemoteDeviceID) index already built from Physical.def's own import declarations, no new
// inference on this side at all.
func TestBuildImportedDiscoveryBodySetsViaDeviceWhenTargetAlsoImported(t *testing.T) {
	payload := importedDiscoveryPayload{DefaultEntityID: "sensor.infrastructural_vienna_bedroom_battery_level"}
	payload.Device.Identifiers = []string{"hass.vienna_bedroom"}
	payload.Device.ViaDevice = "hass.vienna_livingroom"
	match := importCapabilityMatch{LocalDeviceID: "node.vienna_bedroom", Capability: "battery_level", LocalEntity: "sensor.physical_vienna_bedroom_battery_level"}

	viaDeviceIndex := buildImportViaDeviceIndex(TImportedFile{Devices: map[string]TImportedDevice{
		"node.vienna_livingroom": {RemoteInstallation: "junglinster", RemoteDeviceID: "hass.vienna_livingroom"},
	}})

	body := buildImportedDiscoveryBody(match, payload, nil, "junglinster", viaDeviceIndex, "vienna")
	device := body["device"].(map[string]interface{})
	if device["via_device"] != "node.vienna_livingroom" {
		t.Errorf("device.via_device = %v, want %q (this house's own import of the same remote device)", device["via_device"], "node.vienna_livingroom")
	}
}

// TestBuildImportedDiscoveryBodyOmitsViaDeviceWhenTargetNotImported confirms a correct,
// unremarkable "nothing to link to" -- the exporter named a real via_device, but this house never
// imported that specific remote device too, so there's nothing in this house's own HA device
// registry to point at.
func TestBuildImportedDiscoveryBodyOmitsViaDeviceWhenTargetNotImported(t *testing.T) {
	payload := importedDiscoveryPayload{DefaultEntityID: "sensor.infrastructural_vienna_bedroom_battery_level"}
	payload.Device.Identifiers = []string{"hass.vienna_bedroom"}
	payload.Device.ViaDevice = "hass.vienna_livingroom"
	match := importCapabilityMatch{LocalDeviceID: "node.vienna_bedroom", Capability: "battery_level", LocalEntity: "sensor.physical_vienna_bedroom_battery_level"}

	body := buildImportedDiscoveryBody(match, payload, nil, "junglinster", buildImportViaDeviceIndex(TImportedFile{}), "vienna")
	device := body["device"].(map[string]interface{})
	if _, present := device["via_device"]; present {
		t.Errorf("device.via_device = %v, want it entirely omitted (target not also imported)", device["via_device"])
	}
}

// TestBuildImportViaDeviceIndex confirms the index is keyed on the (RemoteInstallation,
// RemoteDeviceID) pair, not local device id, and skips a device that isn't a real import (no
// remote reference at all -- shouldn't happen for a genuinely "import"-kind device, but
// defensive).
func TestBuildImportViaDeviceIndex(t *testing.T) {
	importFile := TImportedFile{Devices: map[string]TImportedDevice{
		"node.vienna_livingroom": {RemoteInstallation: "junglinster", RemoteDeviceID: "hass.vienna_livingroom"},
		"node.pro-1":             {RemoteInstallation: "junglinster", RemoteDeviceID: "host.pro-1"},
		"no-remote-reference":    {},
	}}
	index := buildImportViaDeviceIndex(importFile)
	if len(index) != 2 {
		t.Fatalf("got %d entries, want 2 (the no-remote-reference device must be skipped): %v", len(index), index)
	}
	if index[importRemoteDeviceKey{RemoteInstallation: "junglinster", RemoteDeviceID: "hass.vienna_livingroom"}] != "node.vienna_livingroom" {
		t.Errorf("index missing/wrong for hass.vienna_livingroom: %v", index)
	}
	if index[importRemoteDeviceKey{RemoteInstallation: "junglinster", RemoteDeviceID: "host.pro-1"}] != "node.pro-1" {
		t.Errorf("index missing/wrong for host.pro-1: %v", index)
	}
}

// TestBuildImportedDiscoveryBodyDeviceNameFallsBackToLocalDeviceID confirms a device with no
// resolved DisplayName (shouldn't normally happen -- registerImportedDevicePositioning always
// sets one -- but defensive) falls back to the bare local device id, never the remote's own name.
func TestBuildImportedDiscoveryBodyDeviceNameFallsBackToLocalDeviceID(t *testing.T) {
	payload := importedDiscoveryPayload{DefaultEntityID: "sensor.infrastructural_vienna_shower_room_temperature"}
	payload.Device.Identifiers = []string{"hass.vienna_shower_room"}
	match := importCapabilityMatch{LocalDeviceID: "hass.vienna_shower_room", Capability: "temperature", LocalEntity: "sensor.physical_shower_room_netatmo_temperature"}

	body := buildImportedDiscoveryBody(match, payload, nil, "", nil, "test")
	device := body["device"].(map[string]interface{})
	if device["name"] != "hass.vienna_shower_room" {
		t.Errorf("device.name = %v, want the fallback bare local device id", device["name"])
	}
}

// TestBuildImportedDiscoveryBodyAppliesLocalSuggestedArea is a regression test for a real gap
// found live 2026-09-08: an imported device positioned inside a local "as area" space had its
// suggested_area correctly computed generator-side (registerImportedDevicePositioning,
// Conceptual_DevicePositioning.go), but nothing ever serialized it into imported.yaml, so it never
// reached the coordinator at all. Confirms it flows device.ConstantAttributes ->
// importCapabilityMatch.SuggestedArea -> the discovery body's own device block -- deliberately
// local, never read from the exporter's own payload (unlike manufacturer/model).
func TestBuildImportedDiscoveryBodyAppliesLocalSuggestedArea(t *testing.T) {
	importFile := TImportedFile{Devices: map[string]TImportedDevice{
		"hass.vienna_livingroom": {
			RemoteInstallation: "junglinster", RemoteDeviceID: "hass.vienna_livingroom",
			Capabilities: map[string]TImportedCapability{
				"co2": {LocalEntity: "sensor.physical_apartment_living_room_netatmo_co2"},
			},
			ConstantAttributes: map[string]TConceptualConstant{"suggested_area": {Value: "social/living_room"}},
		},
	}}
	stableID := exportStableID("junglinster", "hass.vienna_livingroom", "co2")
	match, ok := matchImportedCapabilityByStableID(importFile, "junglinster", "homeassistant/sensor/coordinator/"+stableID+"/config")
	if !ok {
		t.Fatalf("expected a match")
	}
	if match.SuggestedArea != "social/living_room" {
		t.Fatalf("match.SuggestedArea = %q, want %q", match.SuggestedArea, "social/living_room")
	}

	payload := importedDiscoveryPayload{DefaultEntityID: "sensor.vienna_carbon_dioxide"}
	payload.Device.Identifiers = []string{"hass.vienna_livingroom"}
	payload.Device.Manufacturer = "Netatmo" // exporter-learned, must coexist with the local area

	body := buildImportedDiscoveryBody(match, payload, nil, "", nil, "test")
	device := body["device"].(map[string]interface{})
	if device["suggested_area"] != "social/living_room" {
		t.Errorf("device.suggested_area = %v, want %q", device["suggested_area"], "social/living_room")
	}
	if device["manufacturer"] != "Netatmo" {
		t.Errorf("device.manufacturer = %v, want it preserved alongside the local area", device["manufacturer"])
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

	got := importedAvailabilityTopics(importFile, match)
	want := importedStateTopic("binary_sensor.infrastructural_terrace_netatmo_node")
	if len(got) != 1 || got[0] != want {
		t.Errorf("importedAvailabilityTopics = %v, want [%q]", got, want)
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

	if got := importedAvailabilityTopics(importFile, match); len(got) != 0 {
		t.Errorf("importedAvailabilityTopics for the node capability itself = %v, want none (no self-reference)", got)
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

	if got := importedAvailabilityTopics(importFile, match); len(got) != 0 {
		t.Errorf("importedAvailabilityTopics with no declared node capability = %v, want none", got)
	}
}

// TestImportedAvailabilityTopicsIncludesDependencyTopics is the coordinator-side half of the
// logical layer's "dependency on" mechanism (2026-09-16): a device's own DependsOnAvailability
// (decoded from coordinator/imported.yaml's "depends_on_availability" field, resolved
// generator-side) must be ANDed in alongside its own node topic, for every capability including
// the node capability itself (a dependency going down must mark THIS device's own node
// unavailable too, not just its other capabilities -- unlike the device's own self-referential
// node topic, which correctly excludes itself).
func TestImportedAvailabilityTopicsIncludesDependencyTopics(t *testing.T) {
	importFile := TImportedFile{Devices: map[string]TImportedDevice{
		"node.vienna_livingroom": {
			Capabilities: map[string]TImportedCapability{
				"node":        {LocalEntity: "binary_sensor.infrastructural_apartment_living_room_netatmo_node"},
				"temperature": {LocalEntity: "sensor.physical_apartment_living_room_netatmo_temperature"},
			},
			DependsOnAvailability: []string{"hosts/netatmo/node/state"},
		},
	}}

	temperatureMatch := importCapabilityMatch{LocalDeviceID: "node.vienna_livingroom", Capability: "temperature"}
	got := importedAvailabilityTopics(importFile, temperatureMatch)
	wantOwn := importedStateTopic("binary_sensor.infrastructural_apartment_living_room_netatmo_node")
	if len(got) != 2 || got[0] != wantOwn || got[1] != "hosts/netatmo/node/state" {
		t.Errorf("importedAvailabilityTopics(temperature) = %v, want [%q, %q]", got, wantOwn, "hosts/netatmo/node/state")
	}

	// The node capability's own config excludes its OWN topic (no self-reference) but must still
	// carry the dependency topic -- host.netatmo going down marks node.vienna_livingroom's own
	// node unavailable too, per the user's own explicit requirement.
	nodeMatch := importCapabilityMatch{LocalDeviceID: "node.vienna_livingroom", Capability: "node"}
	got = importedAvailabilityTopics(importFile, nodeMatch)
	if len(got) != 1 || got[0] != "hosts/netatmo/node/state" {
		t.Errorf("importedAvailabilityTopics(node) = %v, want [%q]", got, "hosts/netatmo/node/state")
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

	body := buildImportedDiscoveryBody(match, payload, []string{nodeAvailabilityTopic}, "", nil, "test")
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

	body := buildImportedDiscoveryBody(match, payload, nil, "", nil, "test")
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

	body := buildImportedDiscoveryBody(match, payload, nil, "", nil, "test")
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
// buildDiscoveryConfigs) and a bare "state_topic" ("hosts/smarty/cpu/state"). After their
// discovery configs arrive, a single incoming JSON-blob message on that same bare topic (exactly
// what Integrations/cpu/report publishes -- retired 2026-09-08's installation-qualified segment)
// must fan out to BOTH local relay topics, each carrying just its own extracted field --
// confirming the unified importer resolves "hosts"-kind sources purely from the payload's own
// StateTopic/ValueTemplate fields, with no exporter-side change and no DSL-declared "kind".
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

	// The real report script (Integrations/cpu/report) publishes directly to this same bare
	// topic -- exactly what the discovery payload's own state_topic field already carries.
	bareTopic := "hosts/smarty/cpu/state"
	blob := []byte(`{"load": 42.3, "temperature": 19.5}`)
	for _, h := range cloudClient.subscribedHandlersSnapshot() {
		h(cloudClient, fakeMessage{topic: bareTopic, payload: blob})
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
	// it to the SAME bare topic shape as any other "hosts"-kind attribute, so a plain node
	// message arriving here must relay exactly like "load"/"temperature" did above -- no
	// importer-side synthesis involved.
	wantNodeTopic := importedStateTopic(nodeLocalEntity)
	nodeBareTopic := "hosts/smarty/node/state"
	for _, h := range cloudClient.subscribedHandlersSnapshot() {
		h(cloudClient, fakeMessage{topic: nodeBareTopic, payload: []byte("true")})
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

	bareTopic := "hosts/smarty/node/state"
	for _, h := range cloudClient.subscribedHandlersSnapshot() {
		h(cloudClient, fakeMessage{topic: bareTopic, payload: []byte("true")})
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

// TestResolveImportedDerivedCapabilityAtomic covers the single-step case: state_topic is the
// atomic sibling's own local relay topic, template is the bare passthrough "value".
func TestResolveImportedDerivedCapabilityAtomic(t *testing.T) {
	device := TImportedDevice{Capabilities: map[string]TImportedCapability{
		"battery_level": {LocalEntity: "sensor.infrastructural_terrace_netatmo_battery_level"},
	}}
	topic, template, ok := resolveImportedDerivedCapability(device, "battery_level", map[string]bool{})
	if !ok {
		t.Fatalf("expected a resolved chain")
	}
	if topic != importedStateTopic("sensor.infrastructural_terrace_netatmo_battery_level") {
		t.Errorf("topic = %q, want the atomic capability's own local relay topic", topic)
	}
	if template != "value" {
		t.Errorf("template = %q, want the bare passthrough \"value\"", template)
	}
}

// TestResolveImportedDerivedCapabilityComposesChain covers a two-step derivation (a capability
// derived from another derived capability) -- must collapse to the ultimate atomic ancestor's own
// topic, with a fully nested, parenthesized template.
func TestResolveImportedDerivedCapabilityComposesChain(t *testing.T) {
	device := TImportedDevice{Capabilities: map[string]TImportedCapability{
		"battery_level":         {LocalEntity: "sensor.infrastructural_terrace_netatmo_battery_level"},
		"battery_alert":         {Domain: "binary_sensor", DerivedFromCapability: "battery_level", DerivedViaTemplate: "( $ | int(0) < 20 )"},
		"battery_alert_delayed": {Domain: "binary_sensor", DerivedFromCapability: "battery_alert", DerivedViaTemplate: "not ($)"},
	}}
	topic, template, ok := resolveImportedDerivedCapability(device, "battery_alert_delayed", map[string]bool{})
	if !ok {
		t.Fatalf("expected a resolved chain")
	}
	if topic != importedStateTopic("sensor.infrastructural_terrace_netatmo_battery_level") {
		t.Errorf("topic = %q, want the ULTIMATE atomic ancestor's own topic", topic)
	}
	want := "not ((( (value) | int(0) < 20 )))"
	if template != want {
		t.Errorf("template = %q, want %q", template, want)
	}
}

// TestResolveImportedDerivedCapabilityDanglingReference covers "derived from" a label the device
// doesn't declare at all -- must fail cleanly (ok=false), not panic.
func TestResolveImportedDerivedCapabilityDanglingReference(t *testing.T) {
	device := TImportedDevice{Capabilities: map[string]TImportedCapability{
		"battery_alert": {Domain: "binary_sensor", DerivedFromCapability: "battery_level", DerivedViaTemplate: "( $ | int(0) < 20 )"},
	}}
	if _, _, ok := resolveImportedDerivedCapability(device, "battery_alert", map[string]bool{}); ok {
		t.Errorf("expected ok=false: battery_level is not declared on this device")
	}
}

// TestBuildDerivedImportedDiscoveryBody covers the discovery config shape for a derived import
// capability: state_topic/value_template come from the resolved chain, not any exporter payload.
func TestBuildDerivedImportedDiscoveryBody(t *testing.T) {
	device := TImportedDevice{
		DisplayName: "apartment/terrace/netatmo",
		Capabilities: map[string]TImportedCapability{
			"battery_alert": {Domain: "binary_sensor", DerivedFromCapability: "battery_level", DerivedViaTemplate: "( $ | int(0) < 20 )"},
		},
	}
	cap := device.Capabilities["battery_alert"]
	cap.LocalEntity = "binary_sensor.infrastructural_terrace_netatmo_battery_alert"
	baseTopic := importedStateTopic("sensor.infrastructural_terrace_netatmo_battery_level")

	body := buildDerivedImportedDiscoveryBody("import.vienna_terrace", device, "battery_alert", cap, baseTopic, "( (value) | int(0) < 20 )", nil, "test")
	if body["state_topic"] != baseTopic {
		t.Errorf("state_topic = %v, want the resolved atomic ancestor's own topic %q", body["state_topic"], baseTopic)
	}
	if body["value_template"] != "{{ ( (value) | int(0) < 20 ) }}" {
		t.Errorf("value_template = %v, want the composed template wrapped in {{ }}", body["value_template"])
	}
	if body["default_entity_id"] != cap.LocalEntity {
		t.Errorf("default_entity_id = %v, want %q", body["default_entity_id"], cap.LocalEntity)
	}
	if body["payload_on"] != mqttStateOn || body["payload_off"] != mqttStateOff {
		t.Errorf("payload_on/off = %v/%v, want the lowercase binary_sensor convention", body["payload_on"], body["payload_off"])
	}
	deviceBlock, ok := body["device"].(map[string]interface{})
	if !ok || deviceBlock["name"] != "apartment/terrace/netatmo" {
		t.Errorf("device block = %v, want name %q", body["device"], "apartment/terrace/netatmo")
	}
}

// TestPublishDerivedImportedDiscoveryEndToEnd is the end-to-end counterpart: a derived capability
// declared in importFile gets its own discovery config published, with no cloud message involved
// at all -- unlike every other imported capability's own discovery path.
func TestPublishDerivedImportedDiscoveryEndToEnd(t *testing.T) {
	importFile := TImportedFile{Devices: map[string]TImportedDevice{
		"import.vienna_terrace": {
			DisplayName: "apartment/terrace/netatmo",
			Capabilities: map[string]TImportedCapability{
				"battery_level": {LocalEntity: "sensor.infrastructural_terrace_netatmo_battery_level"},
				"battery_alert": {Domain: "binary_sensor", DerivedFromCapability: "battery_level", DerivedViaTemplate: "( $ | int(0) < 20 )", LocalEntity: "binary_sensor.infrastructural_terrace_netatmo_battery_alert"},
			},
		},
	}}
	mainClient := &fakeClient{}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))

	if err := publishDerivedImportedDiscovery(mainClient, importFile, publisher, testPrefix, "test"); err != nil {
		t.Fatalf("publishDerivedImportedDiscovery error: %v", err)
	}

	wantTopic := discoveryTopic(testPrefix, "binary_sensor", importedUniqueID("binary_sensor.infrastructural_terrace_netatmo_battery_alert"))
	var got map[string]interface{}
	found := false
	for _, p := range mainClient.publishedSnapshot() {
		if p.topic == wantTopic {
			found = true
			if err := json.Unmarshal(p.payload, &got); err != nil {
				t.Fatalf("unmarshalling published discovery payload: %v", err)
			}
		}
	}
	if !found {
		t.Fatalf("expected a discovery config published to %q, got %v", wantTopic, mainClient.publishedSnapshot())
	}
	wantStateTopic := importedStateTopic("sensor.infrastructural_terrace_netatmo_battery_level")
	if got["state_topic"] != wantStateTopic {
		t.Errorf("state_topic = %v, want the sibling's own local relay topic %q", got["state_topic"], wantStateTopic)
	}
}

// TestPublishDerivedImportedDiscoverySkipsUnresolvableChain confirms a dangling "derived from"
// reference is skipped (no publish, no error), not fatal -- physical_rule_derived_capabilities.go
// already warns about this at generate time.
func TestPublishDerivedImportedDiscoverySkipsUnresolvableChain(t *testing.T) {
	importFile := TImportedFile{Devices: map[string]TImportedDevice{
		"import.vienna_terrace": {
			Capabilities: map[string]TImportedCapability{
				"battery_alert": {Domain: "binary_sensor", DerivedFromCapability: "battery_level", DerivedViaTemplate: "( $ | int(0) < 20 )", LocalEntity: "binary_sensor.infrastructural_terrace_netatmo_battery_alert"},
			},
		},
	}}
	mainClient := &fakeClient{}
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))

	if err := publishDerivedImportedDiscovery(mainClient, importFile, publisher, testPrefix, "test"); err != nil {
		t.Fatalf("publishDerivedImportedDiscovery error: %v", err)
	}
	if len(mainClient.publishedSnapshot()) != 0 {
		t.Errorf("expected no publish for an unresolvable chain, got %v", mainClient.publishedSnapshot())
	}
}
