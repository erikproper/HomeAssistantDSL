/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationImportGenerator
 *
 * Writes <outputRoot>/coordinator/imported.yaml -- one entry per capability of an import
 * declaration (integration_import_storage.go) that Spaces.def actually positioned
 * (registerImportedDevicePositioning/registerImportedDeviceSourceEntityLink,
 * Conceptual_DevicePositioning.go/Conceptual_DeviceSourceEntities.go), for
 * house_event_bus_coordinator/discoveryimport.go to subscribe against on the cloud broker and
 * publish under. Kind-agnostic since the 2026-09-05 grammar unification -- the same file shape
 * serves an import of a "hosts"-declared or a "home_assistant"-declared remote device alike, since
 * discoveryimport.go resolves the difference from the cloud discovery payload itself, not from
 * anything recorded here. Mirrors generateHassBridgeFile's own "hasLink"/AttributeEntityIDs-resolved
 * gate exactly, for the same reason: a device (or one of its capabilities) Physical.def declares but
 * Spaces.def never positions has nothing local to publish it as, so it's silently skipped here
 * rather than left to invent a name on the coordinator side -- Physical.def stays the source of
 * declared capabilities, Spaces.def stays the naming authority for where they live locally,
 * consistent with every other device kind's own conceptual-link gate.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 05.09.2026
 *
 */

package main

import (
	"path/filepath"
	"sort"
	"strings"
)

// generateImportedDeviceFile writes coordinator/imported.yaml for every declared import device
// that also has a DeviceConceptualLinks entry (i.e. was actually positioned via a
// "device <spec> from <device-id>;" declaration in Spaces.def) -- a device declared in
// Physical.def but never positioned has nothing to export locally yet, same as
// generateHassBridgeFile's own "hasLink" skip. Within a positioned device, only capabilities
// Spaces.def actually referenced (link.AttributeEntityIDs resolved) are written -- a declared but
// unused capability is silently skipped, same as the export side.
func generateImportedDeviceFile(outputRoot string, devices []TImportedDevice, admin *TAdministrationState) error {
	deviceIDs := make([]string, 0, len(devices))
	byID := map[string]TImportedDevice{}
	for _, d := range devices {
		if _, exists := byID[d.DeviceID]; exists {
			continue
		}
		byID[d.DeviceID] = d
		deviceIDs = append(deviceIDs, d.DeviceID)
	}
	sort.Strings(deviceIDs)

	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("devices:\n")
	written := false

	for _, deviceID := range deviceIDs {
		device := byID[deviceID]
		link, hasLink := admin.DeviceConceptualLinks[deviceID]
		if !hasLink {
			continue
		}

		capNames := make([]string, 0, len(device.Capabilities))
		for name := range device.Capabilities {
			capNames = append(capNames, name)
		}
		sort.Strings(capNames)

		var capLines strings.Builder
		anyResolved := false
		for _, name := range capNames {
			attr, resolved := link.AttributeEntityIDs[name]
			if !resolved {
				continue
			}
			anyResolved = true
			capLines.WriteString("      " + name + ":\n")
			capLines.WriteString("        local_entity: " + attr.EntityID + "\n")
		}
		if !anyResolved {
			continue
		}

		written = true
		sb.WriteString("  " + deviceID + ":\n")
		sb.WriteString("    remote_installation: \"" + device.RemoteInstallation + "\"\n")
		sb.WriteString("    remote_device_id: \"" + device.RemoteDeviceID + "\"\n")
		// display_name is THIS house's own space-positioning-derived name (deviceDisplayName,
		// Conceptual_DeviceEntities.go -- e.g. "apartment/bedroom/netatmo" for a device positioned
		// inside "space social:apartment with: space social:bedroom with: ...;"), never the
		// exporting installation's own name for it -- mirrors generateHassBridgeFile's own
		// identical field exactly. Real bug found live 2026-08-30: the coordinator originally fell
		// back to the REMOTE's own exported device name (e.g. bare "vienna_bedroom") when this
		// wasn't threaded through, which is wrong for the same reason a hosts device's own display
		// name is never taken from anywhere but its own local positioning.
		if link.DisplayName != "" {
			sb.WriteString("    display_name: " + link.DisplayName + "\n")
		}
		sb.WriteString("    capabilities:\n")
		sb.WriteString(capLines.String())
	}

	if !written {
		return nil
	}

	dir := filepath.Join(outputRoot, "coordinator")
	return writeYAMLFile(filepath.Join(dir, "imported.yaml"), sb.String())
}
