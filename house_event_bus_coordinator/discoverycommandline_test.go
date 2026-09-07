/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: DiscoveryCommandlineTest
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 06.09.2026
 *
 */

package main

import "testing"

func frameDevice() TCommandlineDevice {
	return TCommandlineDevice{
		Host:     "frame",
		Switches: map[string]TCommandlineCapabilityRef{"slideshow": {}},
		Buttons:  map[string]TCommandlineCapabilityRef{"reboot": {}},
	}
}

func TestBuildCommandlineDiscoveryConfigsNodeSwitchAndButton(t *testing.T) {
	configs := buildCommandlineDiscoveryConfigs("host.frame", frameDevice(), testPrefix)
	if len(configs) != 3 {
		t.Fatalf("got %d discovery configs, want 3 (node + switch + button): %+v", len(configs), configs)
	}

	byTopic := map[string]TDiscoveryConfig{}
	for _, c := range configs {
		byTopic[c.Topic] = c
	}

	nodeTopic := "homeassistant/binary_sensor/coordinator/host_frame_commandline_node/config"
	nodeCfg, ok := byTopic[nodeTopic]
	if !ok {
		t.Fatalf("missing node discovery topic %q; got %v", nodeTopic, keysOf(byTopic))
	}
	nodePayload, ok := nodeCfg.Payload.(TBinarySensorDiscoveryPayload)
	if !ok {
		t.Fatalf("node payload has wrong type: %T", nodeCfg.Payload)
	}
	if nodePayload.StateTopic != "commandline/frame/node/state" || nodePayload.PayloadOn != "true" ||
		nodePayload.PayloadOff != "false" || nodePayload.DeviceClass != "connectivity" ||
		nodePayload.Device.Identifiers[0] != "host.frame" {
		t.Errorf("node payload = %+v, unexpected shape", nodePayload)
	}
	if nodePayload.UniqueID != "host.frame_commandline_node" {
		t.Errorf("node payload UniqueID = %q, want %q -- must NOT collide with a hosts-kind device's own \"<id>_node\" unique_id", nodePayload.UniqueID, "host.frame_commandline_node")
	}
	if nodePayload.Name == nil || *nodePayload.Name != "frame/commandline" {
		t.Errorf("node payload Name = %v, want a non-nil distinguishing name (this is one of two liveness signals sharing a device, not \"the device itself\")", nodePayload.Name)
	}

	switchTopic := "homeassistant/switch/coordinator/host_frame_slideshow/config"
	switchCfg, ok := byTopic[switchTopic]
	if !ok {
		t.Fatalf("missing switch discovery topic %q; got %v", switchTopic, keysOf(byTopic))
	}
	switchPayload, ok := switchCfg.Payload.(TCommandlineSwitchDiscoveryPayload)
	if !ok {
		t.Fatalf("switch payload has wrong type: %T", switchCfg.Payload)
	}
	if switchPayload.StateTopic != "commandline/frame/slideshow/state" ||
		switchPayload.CommandTopic != "commandline/frame/slideshow/set" ||
		switchPayload.PayloadOn != "1" || switchPayload.PayloadOff != "0" ||
		switchPayload.StateOn != "1" || switchPayload.StateOff != "0" ||
		switchPayload.AvailabilityTopic != "commandline/frame/node/state" {
		t.Errorf("switch payload = %+v, unexpected shape (must match check_slideshow's existing 0/1 convention verbatim)", switchPayload)
	}

	buttonTopic := "homeassistant/button/coordinator/host_frame_reboot/config"
	buttonCfg, ok := byTopic[buttonTopic]
	if !ok {
		t.Fatalf("missing button discovery topic %q; got %v", buttonTopic, keysOf(byTopic))
	}
	buttonPayload, ok := buttonCfg.Payload.(TCommandlineButtonDiscoveryPayload)
	if !ok {
		t.Fatalf("button payload has wrong type: %T", buttonCfg.Payload)
	}
	if buttonPayload.CommandTopic != "commandline/frame/reboot/press" || buttonPayload.PayloadPress != "PRESS" ||
		buttonPayload.AvailabilityTopic != "commandline/frame/node/state" {
		t.Errorf("button payload = %+v, unexpected shape", buttonPayload)
	}
}

// TestBuildCommandlineDiscoveryConfigsSensorHasNoCommandTopic guards against a sensor capability
// accidentally picking up a command_topic/set semantics -- a commandline sensor is read-only,
// same as any other MQTT sensor, only ever published to by the daemon's own status_script.
func TestBuildCommandlineDiscoveryConfigsSensorHasNoCommandTopic(t *testing.T) {
	device := TCommandlineDevice{Host: "frame", Sensors: map[string]TCommandlineCapabilityRef{"uptime": {}}}
	configs := buildCommandlineDiscoveryConfigs("host.frame", device, testPrefix)

	var sensorCfg *TDiscoveryConfig
	for i := range configs {
		if _, ok := configs[i].Payload.(TSensorDiscoveryPayload); ok {
			sensorCfg = &configs[i]
		}
	}
	if sensorCfg == nil {
		t.Fatalf("expected a sensor discovery config among %+v", configs)
	}
	payload := sensorCfg.Payload.(TSensorDiscoveryPayload)
	if payload.StateTopic != "commandline/frame/uptime/state" {
		t.Errorf("StateTopic = %q, want %q", payload.StateTopic, "commandline/frame/uptime/state")
	}
	if payload.AvailabilityTopic != "commandline/frame/node/state" {
		t.Errorf("AvailabilityTopic = %q, want the device's node topic", payload.AvailabilityTopic)
	}
}

// TestBuildCommandlineDiscoveryConfigsUsesPositionedEntityIDWhenSet confirms a capability
// positioned in Spaces.def (homeassistant/Conceptual_CommandlineEntities.go writes entity_id/name
// into commandline.yaml for it) is used verbatim instead of the auto-derived fallback naming.
func TestBuildCommandlineDiscoveryConfigsUsesPositionedEntityIDWhenSet(t *testing.T) {
	device := TCommandlineDevice{
		Host: "frame",
		Switches: map[string]TCommandlineCapabilityRef{
			"slideshow": {EntityID: "switch.social_apartment_living_room_picture_frame", Name: "social/apartment/living_room/picture_frame"},
		},
	}
	configs := buildCommandlineDiscoveryConfigs("host.frame", device, testPrefix)

	var switchCfg TCommandlineSwitchDiscoveryPayload
	for _, c := range configs {
		if p, ok := c.Payload.(TCommandlineSwitchDiscoveryPayload); ok {
			switchCfg = p
		}
	}
	if switchCfg.DefaultEntityID != "switch.social_apartment_living_room_picture_frame" {
		t.Errorf("DefaultEntityID = %q, want the positioned entity id", switchCfg.DefaultEntityID)
	}
	if switchCfg.Name != "social/apartment/living_room/picture_frame" {
		t.Errorf("Name = %q, want the positioned display name", switchCfg.Name)
	}
}

// TestBuildCommandlineDiscoveryConfigsSharesDeviceIdentityWithHostsKind confirms a commandline
// device deliberately shares its device: block identity with any "hosts"-kind declaration using
// the same DeviceID -- e.g. host.frame declared under both "integration hosts" (ping/cpu) and
// "integration commandline" (switch/button) -- so HA's device registry merges them into one
// device rather than showing two, per this file's own doc comment.
func TestBuildCommandlineDiscoveryConfigsSharesDeviceIdentityWithHostsKind(t *testing.T) {
	configs := buildCommandlineDiscoveryConfigs("host.frame", frameDevice(), testPrefix)
	nodeCfg := configs[0].Payload.(TBinarySensorDiscoveryPayload)
	if len(nodeCfg.Device.Identifiers) != 1 || nodeCfg.Device.Identifiers[0] != "host.frame" {
		t.Errorf("Device.Identifiers = %v, want exactly [\"host.frame\"] to match a hosts-kind declaration of the same id", nodeCfg.Device.Identifiers)
	}
}

// TestBuildCommandlineDiscoveryConfigsNodeUniqueIDDoesNotCollideWithHostsKind is the regression
// test for a real bug found live 2026-09-06: a commandline device's own liveness entity used
// "<deviceID>_node" -- the EXACT unique_id a "hosts"-kind declaration of the same deviceID already
// uses for its own ping-based liveness signal (buildDiscoveryConfigs, discovery.go). Confirmed
// live on host.frame (both a "hosts" cpu device and a "commandline" device): the two integrations'
// own periodic republishes repeatedly clobbered each other's payload on the shared topic
// (homeassistant/binary_sensor/coordinator/host_frame_node/config), each one's own refresh
// flip-flopping the entity between hosts' ping-based state and commandline's own LWT-based one.
func TestBuildCommandlineDiscoveryConfigsNodeUniqueIDDoesNotCollideWithHostsKind(t *testing.T) {
	configs := buildCommandlineDiscoveryConfigs("host.frame", frameDevice(), testPrefix)
	nodeCfg := configs[0].Payload.(TBinarySensorDiscoveryPayload)
	hostsKindNodeUniqueID := "host.frame" + "_node" // buildDiscoveryConfigs' own formula, discovery.go
	if nodeCfg.UniqueID == hostsKindNodeUniqueID {
		t.Fatalf("commandline node UniqueID = %q, collides with a hosts-kind device's own node unique_id -- must be distinct", nodeCfg.UniqueID)
	}
}
