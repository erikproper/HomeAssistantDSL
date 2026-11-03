package main

import "testing"

// TestCutCapabilityIcons: a capability's "with icon: ..." is taken off its line, by device and
// label, for any integration's capability shape (PROJECT.md 30).
func TestCutCapabilityIcons(t *testing.T) {
	lines, icons := cutCapabilityIcons([]string{
		"device node.x host x cpu with:",
		`capability cpu/load: sensor.processor_use with icon: "mdi:cpu-64-bit";`,
		"end;",
		"device appliance.frame host frame with:",
		`capability switch.slideshow as unique_id frame_slideshow "a" "b" "c" with icon: "mdi:image";`,
		`capability sensor.uptime "uptime";`,
		"end;",
	})
	if lines[1] != "capability cpu/load: sensor.processor_use;" || lines[4] != `capability switch.slideshow as unique_id frame_slideshow "a" "b" "c";` {
		t.Errorf("lines = %q", lines)
	}
	if icons["node.x!cpu/load"] != "mdi:cpu-64-bit" || icons["appliance.frame!slideshow"] != "mdi:image" || len(icons) != 2 {
		t.Errorf("icons = %v", icons)
	}
}

// TestParseHostsIntegrationBodyCapabilityIcon: a hosts capability's icon is kept by its leaf.
func TestParseHostsIntegrationBodyCapabilityIcon(t *testing.T) {
	devices, warnings := parseHostsIntegrationBody([]string{
		"device host.junglinster host junglinster home_assistant with:",
		`capability cpu/load: sensor.processor_use with icon: "mdi:chip";`,
		"end;",
	})
	if len(warnings) != 0 || len(devices) != 1 || devices[0].CapabilityIcons["load"] != "mdi:chip" || devices[0].Capabilities["cpu/load"] != "sensor.processor_use" {
		t.Errorf("devices = %+v, warnings = %v", devices, warnings)
	}
}

// TestResolveCapabilityIconSettings: a "${...}" setting inside an icon's quotes is resolved.
func TestResolveCapabilityIconSettings(t *testing.T) {
	got := resolveCapabilityIconSettings(`capability binary_sensor.x y with icon: "${water_icon}";`+"\n"+`  icon: "${water_icon}";`, map[string]string{"water_icon": "mdi:water-off"})
	want := `capability binary_sensor.x y with icon: "mdi:water-off";` + "\n" + `  icon: "mdi:water-off";`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
