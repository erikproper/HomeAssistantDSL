/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: Main
 *
 * Starting scaffold for the coordinator described in Architecture.md §6: a model-aware
 * runtime component sitting between integrations/infrastructure and the Home Assistant
 * instance(s), performing discovery refinement, availability propagation, and cross-home
 * federation.
 *
 * This first cut only covers reading and validating the devices.yaml file the generator
 * produces (see homeassistant/physical.go, generatePhysicalIntegrationOutputs) -- it does
 * not yet connect to MQTT or publish discovery. That comes once the ping+cpu integration
 * (Architecture.md §9.2, PROJECT.md Phase 3 step 0) has entities positioned in Entities.def
 * and there's something real to discover.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 18.08.2026
 *
 */

package main

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// TDeviceCapability is one capability entry under a device in devices.yaml, e.g. "load"
// mapping to a source Home Assistant entity id.
type TDeviceCapability struct {
	Entity string `yaml:"entity"`
}

// TDevice is one entry under "devices:" in a generated devices.yaml file.
type TDevice struct {
	Host         string                       `yaml:"host"`
	Integration  string                       `yaml:"integration"`
	Capabilities map[string]TDeviceCapability `yaml:"capabilities"`
}

// TDevicesFile is the top-level shape of a generated devices.yaml file.
type TDevicesFile struct {
	Devices map[string]TDevice `yaml:"devices"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: house_event_bus_coordinator <path/to/devices.yaml>\n")
		os.Exit(1)
	}

	devicesFile, err := loadDevicesFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	printDevicesSummary(devicesFile)
}

// loadDevicesFile reads and parses a generator-produced devices.yaml file.
func loadDevicesFile(path string) (TDevicesFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TDevicesFile{}, fmt.Errorf("cannot read %s: %w", path, err)
	}

	var devicesFile TDevicesFile
	if err := yaml.Unmarshal(data, &devicesFile); err != nil {
		return TDevicesFile{}, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	return devicesFile, nil
}

// printDevicesSummary prints one line per device plus a per-integration-type count, as a
// sanity check that the devices.yaml contract between generator and coordinator holds.
func printDevicesSummary(devicesFile TDevicesFile) {
	deviceIDs := make([]string, 0, len(devicesFile.Devices))
	for id := range devicesFile.Devices {
		deviceIDs = append(deviceIDs, id)
	}
	sort.Strings(deviceIDs)

	countByIntegration := map[string]int{}
	for _, id := range deviceIDs {
		device := devicesFile.Devices[id]
		countByIntegration[device.Integration]++

		capNames := make([]string, 0, len(device.Capabilities))
		for name := range device.Capabilities {
			capNames = append(capNames, name)
		}
		sort.Strings(capNames)

		fmt.Printf("%-40s host=%-30s integration=%-14s capabilities=%v\n", id, device.Host, device.Integration, capNames)
	}

	fmt.Printf("\n%d devices total\n", len(deviceIDs))
	integrations := make([]string, 0, len(countByIntegration))
	for name := range countByIntegration {
		integrations = append(integrations, name)
	}
	sort.Strings(integrations)
	for _, name := range integrations {
		fmt.Printf("  %s: %d\n", name, countByIntegration[name])
	}
}
