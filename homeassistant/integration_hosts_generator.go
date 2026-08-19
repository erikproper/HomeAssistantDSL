/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationHostsGenerator
 *
 * Turns the "hosts" integration's parsed/validated devices (integration_hosts_storage.go)
 * into generated output: the ping integration's host list and a devices.yaml input file
 * for the coordinator. generateHostsIntegrationOutputs is the entry point registered in
 * Physical_Storage.go's integrationBodyParsers.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 19.08.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// generatePingHostsFile writes <outputRoot>/ping/hosts: one host name per line, matching
// the plain-text format the existing ping/report script already expects (it globs
// ~/ping/hosts*). Deployment from this generated file to the actual Integrations/ping/
// location is handled outside the generator.
func generatePingHostsFile(outputRoot string, devices []THostDevice) error {
	hosts := dedupedHostNames(devices)
	var sb strings.Builder
	for _, host := range hosts {
		sb.WriteString(host + "\n")
	}
	dir := filepath.Join(outputRoot, "ping")
	return writeYAMLFile(filepath.Join(dir, "hosts"), sb.String())
}

// generateCoordinatorDevicesFile writes <outputRoot>/coordinator/devices.yaml: the raw
// physical-layer device/capability list the coordinator needs to build discovery
// messages. This intentionally carries no space/conceptual positioning -- that mapping
// doesn't exist yet (Entities.def hasn't been updated to connect these devices to
// spaces), so this stays a pure physical-layer (PSM) artifact for now.
func generateCoordinatorDevicesFile(outputRoot string, devices []THostDevice) error {
	seen := map[string]bool{}
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("devices:\n")
	for _, d := range devices {
		if seen[d.DeviceID] {
			continue
		}
		seen[d.DeviceID] = true
		sb.WriteString("  " + d.DeviceID + ":\n")
		sb.WriteString("    host: " + d.HostName + "\n")
		sb.WriteString("    integration: " + d.IntegrationType + "\n")
		if len(d.Capabilities) == 0 {
			sb.WriteString("    capabilities: {}\n")
			continue
		}
		sb.WriteString("    capabilities:\n")
		capNames := make([]string, 0, len(d.Capabilities))
		for name := range d.Capabilities {
			capNames = append(capNames, name)
		}
		sort.Strings(capNames)
		for _, name := range capNames {
			sb.WriteString("      " + name + ":\n")
			sb.WriteString("        entity: " + d.Capabilities[name] + "\n")
		}
	}
	dir := filepath.Join(outputRoot, "coordinator")
	return writeYAMLFile(filepath.Join(dir, "devices.yaml"), sb.String())
}

// generateHostsIntegrationOutputs is the entry point registered in
// Physical_Storage.go's integrationBodyParsers for the "hosts" integration: parses
// the body then writes both the ping hosts file and the coordinator devices file.
func generateHostsIntegrationOutputs(bodyLines []string, outputRoot string) error {
	devices, warnings := parseHostsIntegrationBody(bodyLines)
	for _, w := range warnings {
		fmt.Printf("[integration hosts] %s\n", w)
	}
	if len(devices) == 0 {
		return nil
	}

	for _, w := range warnDuplicateDeviceIDs(devices) {
		fmt.Printf("[integration hosts] %s\n", w)
	}

	if err := generatePingHostsFile(outputRoot, devices); err != nil {
		return err
	}
	return generateCoordinatorDevicesFile(outputRoot, devices)
}
