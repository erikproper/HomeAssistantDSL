package hamqtt

import "testing"

// TestBridgeEntityConfigSwitchStateIsLowercase: a bridged switch's state_on/state_off must match
// the proxied lowercase "on"/"off", not its uppercase "ON"/"OFF" command payloads -- with them as
// the default, every bridged switch stayed "unknown" (found live 2026-11-04).
func TestBridgeEntityConfigSwitchStateIsLowercase(t *testing.T) {
	commands := map[string]Command{
		"turn_on":  {Payload: "ON", DiscoveryKey: "payload_on"},
		"turn_off": {Payload: "OFF", DiscoveryKey: "payload_off"},
	}
	for _, commandsEnabled := range []bool{true, false} {
		body := BridgeEntityConfig(BridgeEntity{
			UniqueID: "hassbridge_switch_x", LocalEntity: "switch.x", Instance: "ha2mqtt",
			StateTopic: "homeassistant.instances/ha2mqtt/bridge/switch.x/state", Commands: commands, CommandsEnabled: commandsEnabled,
		})
		if body["state_on"] != StateOn || body["state_off"] != StateOff {
			t.Errorf("commands %v: state_on/state_off = %v/%v, want %q/%q", commandsEnabled, body["state_on"], body["state_off"], StateOn, StateOff)
		}
		if commandsEnabled && (body["payload_on"] != "ON" || body["payload_off"] != "OFF") {
			t.Errorf("payload_on/payload_off = %v/%v, want the command payloads ON/OFF", body["payload_on"], body["payload_off"])
		}
	}
	if body := BridgeEntityConfig(BridgeEntity{UniqueID: "u", LocalEntity: "sensor.x"}); body["state_on"] != nil {
		t.Errorf("sensor got state_on %v", body["state_on"])
	}
}

func TestBridgeEntityConfigSensorUnknownIsNone(t *testing.T) {
	body := BridgeEntityConfig(BridgeEntity{UniqueID: "u", LocalEntity: "sensor.social_x_power", Unit: "W", StateTopic: "t"})
	if body["value_template"] != "{{ 'None' if value == 'unknown' else value }}" {
		t.Errorf("value_template %v", body["value_template"])
	}
	body = BridgeEntityConfig(BridgeEntity{UniqueID: "u", LocalEntity: "switch.social_x", StateTopic: "t"})
	if _, has := body["value_template"]; has {
		t.Errorf("a switch got a value_template: %v", body["value_template"])
	}
}
