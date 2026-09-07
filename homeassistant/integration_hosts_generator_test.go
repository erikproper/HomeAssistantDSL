/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationHostsGeneratorTest
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 07.09.2026
 *
 */

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateCoordinatorDevicesFileExcludesForeignAttributeEntityIDs is the regression test for a
// real bug found live 2026-09-07: a device positioned under BOTH a "hosts" declaration (e.g. cpu)
// and a "commandline" declaration shares one DeviceConceptualLink (host.frame is both -- see
// discoverycommandline.go's own doc comment), so its AttributeEntityIDs map carries entries from
// BOTH kinds. generateCoordinatorDevicesFile must only ever write the ones that are actually this
// device's own hosts-kind attributes (mat.AttributeNames) into devices.yaml's
// "conceptual.attribute_entities" -- writing a foreign entry (e.g. a commandline capability like
// "slideshow") there made the coordinator's own buildDiscoveryConfigs (discovery.go) publish a
// bogus "sensor" discovery config reading a JSON field ("value_json.slideshow") that the device's
// real hosts/<host>/cpu/state payload never has, alongside the correct "switch" discovery config
// commandline.yaml's own entry produces for the same capability -- two conflicting discovery
// configs for one physical switch.
func TestGenerateCoordinatorDevicesFileExcludesForeignAttributeEntityIDs(t *testing.T) {
	outputRoot := t.TempDir()
	admin := newAdministrationState()
	admin.DeviceConceptualLinks["host.frame"] = TDeviceConceptualLink{
		NodeEntityID: "binary_sensor.infrastructural_frame_node",
		AttributeEntityIDs: map[string]TDeviceAttributeLink{
			// Keyed by bare leaf name ("load"), matching registerHostAttributeEntity's own
			// convention (integration_hosts_storage.go's splitCapabilityName) -- NOT the
			// group-prefixed "cpu/load" form AttributeNames() returns.
			"load": {EntityID: "sensor.infrastructural_frame_cpu_load"},
			// "slideshow" simulates a commandline capability sharing this device's link -- must
			// NOT appear in devices.yaml's attribute_entities at all.
			"slideshow": {EntityID: "switch.social_picture_frame"},
		},
	}
	devices := []THostDevice{
		{DeviceID: "host.frame", HostName: "frame", IntegrationType: "cpu"},
	}

	if err := generateCoordinatorDevicesFile(outputRoot, devices, admin, "homeassistant", ""); err != nil {
		t.Fatalf("generateCoordinatorDevicesFile: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outputRoot, "coordinator", "devices.yaml"))
	if err != nil {
		t.Fatalf("reading devices.yaml: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "        load:\n") {
		t.Errorf("devices.yaml missing its own \"load\" attribute; got:\n%s", content)
	}
	if strings.Contains(content, "slideshow") {
		t.Errorf("devices.yaml must not carry the foreign \"slideshow\" attribute; got:\n%s", content)
	}
}

// TestGenerateCoordinatorDevicesFileWritesGroupPrefixedCapability is the regression test for
// exportStableID's own cross-house import shorthand needing the DSL-wide "cpu/load" capability
// name, not the internal bare "load" leaf key (found live 2026-09-07, alongside the hosts-kind
// stable id work -- see house_event_bus_coordinator/main.go's TConceptualAttribute.Capability doc
// comment). The map key itself must stay bare (value_template's own JSON field name depends on
// it), but a new "capability:" line should additionally carry the full group-prefixed name.
func TestGenerateCoordinatorDevicesFileWritesGroupPrefixedCapability(t *testing.T) {
	outputRoot := t.TempDir()
	admin := newAdministrationState()
	admin.DeviceConceptualLinks["host.pro-1"] = TDeviceConceptualLink{
		NodeEntityID: "binary_sensor.infrastructural_cloud_pro1_node",
		AttributeEntityIDs: map[string]TDeviceAttributeLink{
			"load":        {EntityID: "sensor.infrastructural_cloud_pro1_cpu_load"},
			"temperature": {EntityID: "sensor.infrastructural_cloud_pro1_cpu_temperature"},
		},
	}
	devices := []THostDevice{
		{DeviceID: "host.pro-1", HostName: "pro1", IntegrationType: "cpu"},
	}

	if err := generateCoordinatorDevicesFile(outputRoot, devices, admin, "homeassistant", ""); err != nil {
		t.Fatalf("generateCoordinatorDevicesFile: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outputRoot, "coordinator", "devices.yaml"))
	if err != nil {
		t.Fatalf("reading devices.yaml: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "        load:\n          entity: sensor.infrastructural_cloud_pro1_cpu_load\n          capability: cpu/load\n") {
		t.Errorf("devices.yaml missing \"capability: cpu/load\" right after \"load\"'s entity; got:\n%s", content)
	}
	if !strings.Contains(content, "        temperature:\n          entity: sensor.infrastructural_cloud_pro1_cpu_temperature\n          capability: cpu/temperature\n") {
		t.Errorf("devices.yaml missing \"capability: cpu/temperature\" right after \"temperature\"'s entity; got:\n%s", content)
	}
}
