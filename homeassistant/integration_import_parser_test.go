package main

import "testing"

func TestParseImportIntegrationBody(t *testing.T) {
	devices, warnings := parseImportIntegrationBody([]string{
		"device import.remote_macbook from junglinster host.eriks-macbook-pro-2 with:",
		"  load: sensor.junglinster_eriks_macbook_pro_2_cpu_load;",
		"end;",
		"",
		"# a comment",
	})
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	d := devices[0]
	if d.DeviceID != "import.remote_macbook" || d.RemoteInstallation != "junglinster" || d.RemoteDeviceID != "host.eriks-macbook-pro-2" {
		t.Errorf("got %+v, want DeviceID=import.remote_macbook RemoteInstallation=junglinster RemoteDeviceID=host.eriks-macbook-pro-2", d)
	}
	if got := d.Capabilities["load"]; got.RemoteEntityRef != "sensor.junglinster_eriks_macbook_pro_2_cpu_load" {
		t.Errorf("Capabilities[load] = %+v, want RemoteEntityRef=sensor.junglinster_eriks_macbook_pro_2_cpu_load", got)
	}
}

func TestParseImportIntegrationBodyWarnsOnMalformedLine(t *testing.T) {
	devices, warnings := parseImportIntegrationBody([]string{
		"device import.remote_macbook junglinster;", // missing "from"/remote-device-id/"with:"
	})
	if len(devices) != 0 {
		t.Errorf("got %d devices, want 0", len(devices))
	}
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
}

// TestParseImportIntegrationBodyMultipleCapabilities covers a device importing more than one
// capability, kind-agnostic with respect to whether the remote declared it via "hosts" or
// "home_assistant" -- the unified grammar (2026-09-05) never states which.
func TestParseImportIntegrationBodyMultipleCapabilities(t *testing.T) {
	devices, warnings := parseImportIntegrationBody([]string{
		"device import.vienna_terrace from junglinster hass.vienna_terrace with:",
		"  temperature: sensor.infrastructural_vienna_terrace_temperature;",
		"  humidity:    sensor.infrastructural_vienna_terrace_humidity;",
		"end;",
	})
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	d := devices[0]
	if d.DeviceID != "import.vienna_terrace" || d.RemoteInstallation != "junglinster" || d.RemoteDeviceID != "hass.vienna_terrace" {
		t.Errorf("got %+v, want DeviceID=import.vienna_terrace RemoteInstallation=junglinster RemoteDeviceID=hass.vienna_terrace", d)
	}
	if got := d.Capabilities["temperature"]; got.RemoteEntityRef != "sensor.infrastructural_vienna_terrace_temperature" {
		t.Errorf("Capabilities[temperature] = %+v, want RemoteEntityRef=sensor.infrastructural_vienna_terrace_temperature", got)
	}
	if got := d.Capabilities["humidity"]; got.RemoteEntityRef != "sensor.infrastructural_vienna_terrace_humidity" {
		t.Errorf("Capabilities[humidity] = %+v, want RemoteEntityRef=sensor.infrastructural_vienna_terrace_humidity", got)
	}
}

// TestParseImportIntegrationBodyShorthandCapability covers the "<domain>.<capability>;" shorthand
// (added 2026-09-07) -- RemoteEntityRef must be left "" (the marker discoveryimport.go uses to
// derive the remote stable id itself instead of matching against a declared ref).
func TestParseImportIntegrationBodyShorthandCapability(t *testing.T) {
	devices, warnings := parseImportIntegrationBody([]string{
		"device import.vienna_livingroom from junglinster hass.vienna_livingroom with:",
		"  binary_sensor.node;",
		"  sensor.co2;",
		"end;",
	})
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	d := devices[0]
	if d.DeviceID != "import.vienna_livingroom" || d.RemoteInstallation != "junglinster" || d.RemoteDeviceID != "hass.vienna_livingroom" {
		t.Errorf("got %+v, want DeviceID=import.vienna_livingroom RemoteInstallation=junglinster RemoteDeviceID=hass.vienna_livingroom", d)
	}
	node, ok := d.Capabilities["node"]
	if !ok || node.RemoteEntityRef != "" {
		t.Errorf("Capabilities[node] = %+v (ok=%v), want a present entry with RemoteEntityRef==\"\"", node, ok)
	}
	co2, ok := d.Capabilities["co2"]
	if !ok || co2.RemoteEntityRef != "" {
		t.Errorf("Capabilities[co2] = %+v (ok=%v), want a present entry with RemoteEntityRef==\"\"", co2, ok)
	}
}

// TestParseImportIntegrationBodyMixesShorthandAndExplicit confirms both capability forms can
// coexist in the same device block -- a DSL author overriding just one capability's derivation
// (e.g. because it predates this convention) shouldn't need to rewrite the whole block.
func TestParseImportIntegrationBodyMixesShorthandAndExplicit(t *testing.T) {
	devices, _ := parseImportIntegrationBody([]string{
		"device import.mixed from junglinster hass.mixed with:",
		"  sensor.co2;",
		"  humidity: sensor.some_legacy_explicit_name;",
		"end;",
	})
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	d := devices[0]
	if got := d.Capabilities["co2"]; got.RemoteEntityRef != "" {
		t.Errorf("Capabilities[co2].RemoteEntityRef = %q, want \"\" (shorthand)", got.RemoteEntityRef)
	}
	if got := d.Capabilities["humidity"]; got.RemoteEntityRef != "sensor.some_legacy_explicit_name" {
		t.Errorf("Capabilities[humidity].RemoteEntityRef = %q, want the explicit form preserved", got.RemoteEntityRef)
	}
}

func TestCollectImportedDevicesAcrossMultipleBlocks(t *testing.T) {
	blocks := []TIntegrationBlock{
		{Name: "hosts", BodyLines: []string{"device host.x x cpu;"}},
		{Name: "import", BodyLines: []string{
			"device import.a from junglinster host.a with:",
			"  load: sensor.junglinster_host_a_cpu_load;",
			"end;",
		}},
		{Name: "import", BodyLines: []string{
			"device import.vienna_terrace from junglinster hass.vienna_terrace with:",
			"  temperature: sensor.infrastructural_vienna_terrace_temperature;",
			"end;",
		}},
	}
	devices, warnings := collectImportedDevices(blocks)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2", len(devices))
	}
}
