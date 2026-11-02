package main

import (
	"strings"
	"testing"
)

// TestValidateEveryDeviceHasNode: a device of any kind without a node in Physical.def or Logical.def
// is a problem; a node from Logical.def counts; hosts and commandline devices, requests and a
// "dependency on" extension are exempt (PROJECT.md 26).
func TestValidateEveryDeviceHasNode(t *testing.T) {
	logical := map[string]TLogicalDevice{
		"buttons.tap":             {DeviceID: "buttons.tap", Capabilities: map[string]TLogicalCapability{"node": {Domain: "binary_sensor", IsDerivedCondition: true}}},
		"fan.missing":             {DeviceID: "fan.missing", Capabilities: map[string]TLogicalCapability{"fan": {Domain: "fan"}}},
		"fan.compliant":           {DeviceID: "fan.compliant", Capabilities: map[string]TLogicalCapability{"fan": {Domain: "fan"}, "node": {Domain: "binary_sensor"}}},
		"request.toggle":          {DeviceID: "request.toggle", Capabilities: map[string]TLogicalCapability{"pressed": {Domain: "event", IsDerivedCondition: true}}},
		"utility.dependency_only": {DeviceID: "utility.dependency_only", DependsOn: []string{"node.somewhere"}},
		"node.host":               {DeviceID: "node.host", Capabilities: map[string]TLogicalCapability{"extra": {Domain: "sensor"}}},
	}
	hosts := map[string]THostDevice{"node.host": {}}
	discovery := map[string]TDiscoveryGatewayDevice{
		"buttons.tap":       {DeviceID: "buttons.tap", Capabilities: map[string]TDiscoveryCapability{"pressed": {Domain: "event"}}},
		"sensors.no_node":   {DeviceID: "sensors.no_node", Capabilities: map[string]TDiscoveryCapability{"temperature": {Domain: "sensor"}}},
		"sensors.with_node": {DeviceID: "sensors.with_node", Capabilities: map[string]TDiscoveryCapability{"node": {Domain: "binary_sensor"}}},
	}
	bridge := map[string]THassBridgeDevice{"hass.missing": {DeviceID: "hass.missing", Capabilities: map[string]THassBridgeCapability{"co2": {Domain: "sensor"}}}}
	imported := map[string]TImportedDevice{"import.compliant": {DeviceID: "import.compliant", Capabilities: map[string]TImportedCapability{"node": {}}}}
	local := map[string]TLocalDevice{"appliance.chime": {DeviceID: "appliance.chime", Capabilities: map[string]TLocalCapability{"volume": {}}}}

	problems := validateEveryDeviceHasNode(logical, hosts, discovery, bridge, imported, map[string]TCommandlineDevice{}, local)
	joined := strings.Join(problems, "\n")
	for _, id := range []string{"fan.missing", "sensors.no_node", "hass.missing", "appliance.chime"} {
		if !strings.Contains(joined, `"`+id+`"`) {
			t.Errorf("expected a problem for %s, got:\n%s", id, joined)
		}
	}
	if len(problems) != 4 {
		t.Errorf("expected exactly 4 problems, got %d:\n%s", len(problems), joined)
	}
}
