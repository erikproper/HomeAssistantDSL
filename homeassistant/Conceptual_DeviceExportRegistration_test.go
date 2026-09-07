package main

import "testing"

// TestRegisterExportedHassBridgeDevicesRegistersUnpositionedDevice is PROJECT.md 1.2b's core
// case: a device declared "export" but never positioned anywhere in Spaces.def must still get a
// full DeviceConceptualLinks entry, one entity per declared capability, auto-registered at the
// root as if every capability had been explicitly positioned there.
func TestRegisterExportedHassBridgeDevicesRegistersUnpositionedDevice(t *testing.T) {
	admin := newAdministrationState()
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.vienna_terrace": {
			DeviceID: "hass.vienna_terrace", Instances: []string{"protocols-server-2"}, Export: true,
			Capabilities: map[string]THassBridgeCapability{
				"temperature": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.vienna_terrace_temperature"}},
				"humidity":    {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.vienna_terrace_humidity"}},
			},
		},
	}

	warnings := registerExportedHassBridgeDevices(admin, hassBridgeDevicesByID)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	link, ok := admin.DeviceConceptualLinks["hass.vienna_terrace"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[hass.vienna_terrace] to be populated")
	}
	temp, ok := link.AttributeEntityIDs["temperature"]
	if !ok || temp.EntityID != "sensor.infrastructural_vienna_terrace_temperature" {
		t.Errorf("temperature EntityID = %+v, want sensor.infrastructural_vienna_terrace_temperature", temp)
	}
	humidity, ok := link.AttributeEntityIDs["humidity"]
	if !ok || humidity.EntityID != "sensor.infrastructural_vienna_terrace_humidity" {
		t.Errorf("humidity EntityID = %+v, want sensor.infrastructural_vienna_terrace_humidity", humidity)
	}
}

// TestRegisterExportedHassBridgeDevicesSkipsNonExportDevices confirms a device that never
// declared "export" is left entirely alone -- this function only ever acts on Export=true devices.
func TestRegisterExportedHassBridgeDevicesSkipsNonExportDevices(t *testing.T) {
	admin := newAdministrationState()
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.laserjet": {
			DeviceID: "hass.laserjet", Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"status": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.hewlett_packard_hp_laserjet_professional_p1102w"}},
			},
		},
	}

	if warnings := registerExportedHassBridgeDevices(admin, hassBridgeDevicesByID); len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if _, ok := admin.DeviceConceptualLinks["hass.laserjet"]; ok {
		t.Errorf("expected hass.laserjet (no \"export\") to stay unregistered, got a link")
	}
}

// TestRegisterExportedHassBridgeDevicesLeavesAlreadyPositionedDeviceUntouched is the deliberate
// scope boundary documented in this file's own header: a device the DSL author already
// positioned in Spaces.def (simulated here by pre-seeding DeviceConceptualLinks, standing in for
// whatever a real "device <spec> from <device-id>;" declaration would have produced) must not be
// silently expanded/overwritten by the export auto-registration path, even if that positioning
// only covers a subset of the device's declared capabilities.
func TestRegisterExportedHassBridgeDevicesLeavesAlreadyPositionedDeviceUntouched(t *testing.T) {
	admin := newAdministrationState()
	existing := TDeviceConceptualLink{
		DisplayName:        "front/netatmo",
		AttributeEntityIDs: map[string]TDeviceAttributeLink{"temperature": {EntityID: "sensor.infrastructural_front_netatmo_temperature"}},
	}
	admin.DeviceConceptualLinks["hass.office_garden"] = existing

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.office_garden": {
			DeviceID: "hass.office_garden", Instances: []string{"protocols-server-2"}, Export: true,
			Capabilities: map[string]THassBridgeCapability{
				"temperature": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.office_garden_temperature"}},
				"humidity":    {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.office_garden_humidity"}},
			},
		},
	}

	if warnings := registerExportedHassBridgeDevices(admin, hassBridgeDevicesByID); len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	got := admin.DeviceConceptualLinks["hass.office_garden"]
	if got.DisplayName != "front/netatmo" {
		t.Errorf("DisplayName = %q, want the already-positioned value untouched", got.DisplayName)
	}
	if len(got.AttributeEntityIDs) != 1 {
		t.Errorf("AttributeEntityIDs = %v, want exactly the one already-positioned capability -- \"humidity\" must NOT have been auto-added", got.AttributeEntityIDs)
	}
}
