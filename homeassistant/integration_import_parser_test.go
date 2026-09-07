package main

import "testing"

func TestParseImportIntegrationBody(t *testing.T) {
	devices, warnings := parseImportIntegrationBody([]string{
		"device import.remote_macbook from junglinster host.eriks-macbook-pro-2 with:",
		"  sensor.load;",
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
	if _, ok := d.Capabilities["load"]; !ok {
		t.Errorf("Capabilities = %+v, want a declared \"load\" entry", d.Capabilities)
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
		"  sensor.temperature;",
		"  sensor.humidity;",
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
	if _, ok := d.Capabilities["temperature"]; !ok {
		t.Errorf("Capabilities = %+v, want a declared \"temperature\" entry", d.Capabilities)
	}
	if _, ok := d.Capabilities["humidity"]; !ok {
		t.Errorf("Capabilities = %+v, want a declared \"humidity\" entry", d.Capabilities)
	}
}

// TestParseImportIntegrationBodyShorthandCapability covers the "<domain>.<capability>;" grammar --
// domain is captured for readability only and never stored (see this file's own header comment).
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
	if _, ok := d.Capabilities["node"]; !ok {
		t.Errorf("Capabilities = %+v, want a declared \"node\" entry", d.Capabilities)
	}
	if _, ok := d.Capabilities["co2"]; !ok {
		t.Errorf("Capabilities = %+v, want a declared \"co2\" entry", d.Capabilities)
	}
}

// TestParseImportIntegrationBodyAllowsSlashInCapabilityName covers a hosts-kind group-prefixed
// capability (e.g. "cpu/load") -- see importCapabilityPattern's own doc comment for why "/" is in
// its charset.
func TestParseImportIntegrationBodyAllowsSlashInCapabilityName(t *testing.T) {
	devices, warnings := parseImportIntegrationBody([]string{
		"device import.pro-1 from junglinster host.pro-1 with:",
		"  sensor.cpu/load;",
		"  sensor.cpu/temperature;",
		"end;",
	})
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	d := devices[0]
	if _, ok := d.Capabilities["cpu/load"]; !ok {
		t.Errorf("Capabilities = %+v, want a declared \"cpu/load\" entry", d.Capabilities)
	}
	if _, ok := d.Capabilities["cpu/temperature"]; !ok {
		t.Errorf("Capabilities = %+v, want a declared \"cpu/temperature\" entry", d.Capabilities)
	}
}

func TestCollectImportedDevicesAcrossMultipleBlocks(t *testing.T) {
	blocks := []TIntegrationBlock{
		{Name: "hosts", BodyLines: []string{"device host.x x cpu;"}},
		{Name: "import", BodyLines: []string{
			"device import.a from junglinster host.a with:",
			"  sensor.load;",
			"end;",
		}},
		{Name: "import", BodyLines: []string{
			"device import.vienna_terrace from junglinster hass.vienna_terrace with:",
			"  sensor.temperature;",
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
