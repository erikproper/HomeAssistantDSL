package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestUnpositionedCapabilitiesReport: hidden and absorbed capabilities aren't listed; a capability
// positioned in Conceptual.def, or registered another way (by name or raw leaf), isn't either; what
// uses an unpositioned one is noted; a device whose capability is absorbed counts as positioned.
func TestUnpositionedCapabilitiesReport(t *testing.T) {
	admin := newAdministrationState()
	discovery := map[string]TDiscoveryGatewayDevice{
		"appliance.plug": {DeviceID: "appliance.plug", Capabilities: map[string]TDiscoveryCapability{
			"switch":      {Domain: "switch", Leaf: "0x1_switch"},
			"power":       {Domain: "sensor", Leaf: "0x1_power"},
			"consumes":    {Domain: "binary_sensor", DerivedFromCapability: "sensor.power"},
			"update":      {Domain: "update", Leaf: "0x1_update"},
			"linkquality": {Domain: "sensor", Leaf: "0x1_linkquality"},
			"raw":         {Domain: "sensor", Leaf: "0x1_raw", Hidden: true},
		}},
		"switch.relay": {DeviceID: "switch.relay", Capabilities: map[string]TDiscoveryCapability{
			"switch":      {Domain: "switch", Leaf: "0x2_switch"},
			"linkquality": {Domain: "sensor", Leaf: "0x2_linkquality"},
		}},
	}
	logical := map[string]TLogicalDevice{
		"appliance.machine": {DeviceID: "appliance.machine", Capabilities: map[string]TLogicalCapability{
			"switch": {Domain: "switch", AbsorbedFromDeviceID: "switch.relay", AbsorbedFromCapability: "switch"},
		}},
	}
	admin.PositionedCapabilities["appliance.plug"] = map[string]bool{"switch": true, "consumes": true}
	admin.PositionedCapabilities["appliance.machine"] = map[string]bool{"switch": true}
	admin.DevicePositioningOccurrences["appliance.plug"] = []TDevicePositioningOccurrence{{}}
	admin.ConceptualUseBySource["appliance.plug!0x1_update"] = TConceptualSourceUse{} // a firmware update
	candidates := collectUnpositionedCapabilities(admin, discovery, nil, nil, nil, nil, nil, logical)

	root := t.TempDir()
	if err := writeUnpositionedCapabilitiesReport(root, admin, candidates); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "suggestions", "conceptual.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := `# Positioned devices, with capabilities Conceptual.def doesn't position:

device appliance.plug with:
  sensor.linkquality;
  sensor.power; # used by binary_sensor.consumes
end;

device switch.relay with:
  sensor.linkquality;
end;

# Devices Conceptual.def never positions:

`
	if got := string(data); got != generatorHeader+want && got != want {
		t.Errorf("conceptual.txt:\n%s\nwant:\n%s", got, want)
	}
}
