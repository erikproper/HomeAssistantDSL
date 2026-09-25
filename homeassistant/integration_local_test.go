/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationLocalTest
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 24.09.2026
 *
 */

package main

import (
	"strings"
	"testing"
)

func TestParseLocalIntegrationBodyBareAndAvailabilityCapabilities(t *testing.T) {
	devices, warnings := parseLocalIntegrationBody(strings.Split(`device appliance.front_door_ring with:
  binary_sensor.node camera.core is available;
  camera.core;
  binary_sensor.ding;
end;`, "\n"))
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 || devices[0].DeviceID != "appliance.front_door_ring" {
		t.Fatalf("devices = %+v, want exactly one appliance.front_door_ring", devices)
	}
	device := devices[0]

	core, ok := device.Capabilities["core"]
	if !ok || core.Domain != "camera" || core.AvailabilityOf != "" {
		t.Errorf("Capabilities[core] = %+v, want a bare camera capability", core)
	}
	ding, ok := device.Capabilities["ding"]
	if !ok || ding.Domain != "binary_sensor" || ding.AvailabilityOf != "" {
		t.Errorf("Capabilities[ding] = %+v, want a bare binary_sensor capability", ding)
	}
	node, ok := device.Capabilities["node"]
	if !ok || node.Domain != "binary_sensor" || node.AvailabilityOf != "core" || node.AvailabilityOfDomain != "camera" {
		t.Errorf("Capabilities[node] = %+v, want Domain=binary_sensor AvailabilityOf=core AvailabilityOfDomain=camera", node)
	}
}

// TestParseLocalIntegrationBodyRejectsNonDomainQualifiedSibling mirrors
// integration_discovery_parser.go's own identical guard: the "is available" sibling reference must
// be domain-qualified, not a bare label.
func TestParseLocalIntegrationBodyRejectsNonDomainQualifiedSibling(t *testing.T) {
	_, warnings := parseLocalIntegrationBody(strings.Split(`device appliance.front_door_ring with:
  binary_sensor.node core is available;
end;`, "\n"))
	if len(warnings) != 1 || !strings.Contains(warnings[0], "isn't domain-qualified") {
		t.Fatalf("expected exactly one \"isn't domain-qualified\" warning, got: %v", warnings)
	}
}

func TestCollectLocalDevicesByIDWarnsOnDuplicateDevice(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
  integration local with:
    device appliance.front_door_ring with:
      camera.core;
    end;
    device appliance.front_door_ring with:
      camera.core;
    end;
  end;
end;
`)

	byID, warnings := collectLocalDevicesByID(definitionDir)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "declared more than once") {
		t.Fatalf("expected exactly one \"declared more than once\" warning, got: %v", warnings)
	}
	if _, ok := byID["appliance.front_door_ring"]; !ok {
		t.Fatalf("expected the first declaration to still be kept")
	}
}
