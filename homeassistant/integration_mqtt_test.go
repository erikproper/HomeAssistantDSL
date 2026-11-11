package main

import (
	"strings"
	"testing"
)

// TestParseMQTTIntegrationBody: a topic capability becomes a raw gateway capability with its own
// leaf and frozen id; its generated raw config reads the topic and is "on" for the given value.
func TestParseMQTTIntegrationBody(t *testing.T) {
	devices, warnings := parseMQTTIntegrationBody([]string{
		`retained topics "zwave/#";`,
		`device integration.zwave2mqtt with:`,
		`capability binary_sensor.node topic "zwave/driver/status" on "true" as unique_id binary_sensor.x_node;`,
		`is not retained;`,
		`end;`,
		`device integration.bad with:`,
		`capability light.value topic "x";`,
		`end;`,
	})
	if len(warnings) != 1 || len(devices) != 2 {
		t.Fatalf("devices %v, warnings %v", devices, warnings)
	}
	d := devices[0]
	if !d.Raw || d.Retain != retainNotDeclared || d.Capabilities["node"].Leaf != "integration_zwave2mqtt_node" || d.Capabilities["node"].AssignedID != "binary_sensor.x_node" {
		t.Errorf("device = %+v", d)
	}
	configs := rawMQTTConfigs(d)
	c := configs["binary_sensor/integration_zwave2mqtt_node"]
	if c["state_topic"] != "zwave/driver/status" || c["value_template"] != `{{ 'ON' if value == "true" else 'OFF' }}` || c["device_class"] != "connectivity" {
		t.Errorf("config = %v", c)
	}
}

// TestRawMQTTShellyPlug: a switch and sensors from topics, each available as its node says; the
// frozen-id search finds their lines.
func TestRawMQTTShellyPlug(t *testing.T) {
	devices, warnings := parseMQTTIntegrationBody([]string{
		`device switches.test with:`,
		`is retained;`,
		`capability binary_sensor.node topic "plug/online" on "true";`,
		`capability switch.switch topic "plug/relay/0" command "plug/relay/0/command" as unique_id switch.frozen;`,
		`capability sensor.power topic "plug/relay/0/power" unit "W";`,
		`capability sensor.energy topic "plug/relay/0/energy" unit "Wh" divided by 60;`,
		`capability sensor.rssi/level topic "plug/rssi";`,
		`capability event.press topic "plug/event" field "event" types "S", "L";`,
		`end;`,
	})
	if len(warnings) != 0 || len(devices) != 1 {
		t.Fatalf("devices %v, warnings %v", devices, warnings)
	}
	d := devices[0]
	if d.Capabilities["switch"].AssignedID != "switch.frozen" {
		t.Errorf("switch = %+v", d.Capabilities["switch"])
	}
	configs := rawMQTTConfigs(d)
	sw := configs["switch/switches_test_switch"]
	if sw["state_topic"] != "plug/relay/0" || sw["command_topic"] != "plug/relay/0/command" || sw["payload_on"] != "on" || sw["payload_off"] != "off" || sw["availability"] == nil {
		t.Errorf("switch config = %v", sw)
	}
	energy := configs["sensor/switches_test_energy"]
	if energy["value_template"] != "{{ (value | float / 60) | round(3) }}" || energy["unit_of_measurement"] != "Wh" || energy["device_class"] != "energy" || energy["state_class"] != "total_increasing" {
		t.Errorf("energy config = %v", energy)
	}
	if power := configs["sensor/switches_test_power"]; power["device_class"] != "power" || power["value_template"] != nil {
		t.Errorf("power config = %v", power)
	}
	if node := configs["binary_sensor/switches_test_node"]; node["availability"] != nil || node["device_class"] != "connectivity" {
		t.Errorf("node config = %v", node)
	}
	for line, topic := range map[string]string{
		`capability sensor.energy topic "plug/relay/0/energy" unit "Wh" divided by 60;`: "plug/relay/0/energy",
		`capability switch.switch topic "plug/relay/0" command "plug/relay/0/command";`: "plug/relay/0",
		`capability event.press topic "plug/event" field "event" types "S", "L";`:       "plug/event",
	} {
		domain, label, _ := strings.Cut(strings.Fields(line)[1], ".")
		if !discoveryCapabilityLinePattern(domain, label, "", "", topic).MatchString(line) {
			t.Errorf("no match for %q", line)
		}
	}
}

// TestRawMQTTCapabilityErrors: what is wrong with a capability is said at the word.
func TestRawMQTTCapabilityErrors(t *testing.T) {
	for line, want := range map[string]string{
		`capability light.x topic "t";`:               `"light" is not a domain that can read a topic`,
		`capability switch.x topic "t";`:              `expected "command", got ";"`,
		`capability binary_sensor.x topic "t";`:       `expected "on", got ";"`,
		`capability sensor.x topic "t" divided by 0;`: `"0" is not the number to divide by`,
		`capability sensor.x topic t;`:                `expected a quoted MQTT topic, got "t"`,
	} {
		p, ok := parseMQTTCapabilityLine(line)
		if ok || len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0], want) {
			t.Errorf("%q: ok %v, warnings %q, want %q", line, ok, p.Warnings, want)
		}
	}
}

// TestRawSensorUnits: a unit gives a raw sensor the device and state class Home Assistant's
// statistics (and the energy dashboard) need.
func TestRawSensorUnits(t *testing.T) {
	for unit, want := range map[string][2]string{
		"W":   {"power", "measurement"},
		"kW":  {"power", "measurement"},
		"kWh": {"energy", "total_increasing"},
	} {
		config := map[string]interface{}{}
		addRawSensorFields(config, TRawTopicCapability{Unit: unit})
		if config["device_class"] != want[0] || config["state_class"] != want[1] {
			t.Errorf("%s: %v", unit, config)
		}
	}
}
