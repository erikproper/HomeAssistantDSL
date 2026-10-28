package main

import "testing"

// TestParseMQTTIntegrationBody: a topic capability becomes a raw gateway capability with its own
// leaf and frozen id; its generated raw config reads the topic and is "on" for the given value.
func TestParseMQTTIntegrationBody(t *testing.T) {
	devices, warnings := parseMQTTIntegrationBody([]string{
		`retained topics "zwave/#";`,
		`device integration.zwave2mqtt with:`,
		`capability binary_sensor.node topic "zwave/driver/status" on "true" as binary_sensor.x_node;`,
		`is not retained;`,
		`end;`,
		`device integration.bad with:`,
		`capability sensor.value topic "x";`,
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
