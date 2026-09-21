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
			// domain/derived_from/derived_via (added 2026-09-10, plans/
			// derived-capability-mechanism.md Phase 2): only ever set for a "derived ...
			// from ... via ...;" capability (Physical_DerivedCapability.go) -- an ordinary imported
			// capability's domain always comes from wherever Spaces.def positions it (see
			// integration_import_storage.go's own header comment), so these three are omitted for
			// it, exactly as before this feature existed. The coordinator resolves the full
			// derivation chain itself (discoveryimport.go) -- this file only ever passes through
			// what Physical.def declared, verbatim, the same division of responsibility
			// generateHassBridgeFile's own commands/discovery_extra fields already follow.
			if cap := device.Capabilities[name]; cap.DerivedFromCapability != "" {
				capLines.WriteString("        domain: " + cap.Domain + "\n")
				capLines.WriteString("        derived_from: " + cap.DerivedFromCapability + "\n")
				capLines.WriteString("        derived_via: \"" + cap.DerivedViaTemplate + "\"\n")
			}
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
		// depends_on_availability (2026-09-16, PROJECT.md's logical-layer work): a Logical.def
		// "device <this-id> with: dependency on <other-id>; end;" declaration sharing deviceID's own
		// id -- resolved once, generator-side, into the raw MQTT topics the coordinator must
		// additionally AND into every one of this device's own capabilities' availability
		// (integration_logical_storage.go's resolveDependencyAvailabilityTopics, already flattened
		// across the transitive dependency chain and cycle-checked). Absent entirely for a device
		// with no matching Logical.def declaration -- unchanged behaviour from before this existed.
		if len(device.DependsOnAvailabilityTopics) > 0 {
			sb.WriteString("    depends_on_availability:\n")
			for _, topic := range device.DependsOnAvailabilityTopics {
				sb.WriteString("      - \"" + topic + "\"\n")
			}
		}
		sb.WriteString("    capabilities:\n")
		sb.WriteString(capLines.String())

		// suggested_area (added 2026-09-08) is the one constant attribute an imported device can
		// carry -- purely a local Spaces.def positioning concept (which area this house's own
		// "as area" space put it under), never something the exporting installation should dictate,
		// unlike manufacturer/model/... which genuinely belong to the remote device and are instead
		// learned live from the exporter's own cloud-crossed discovery config
		// (discoveryimport.go's buildImportedDiscoveryBody). Real gap found live 2026-09-08:
		// registerImportedDevicePositioning (Conceptual_DevicePositioning.go) already computed this
		// into link.ConstantAttributes, but nothing here ever serialized it into imported.yaml, so
		// it never reached the coordinator at all.
		if area, ok := link.ConstantAttributes["suggested_area"]; ok && area.Value != "" {
			sb.WriteString("    constant_attributes:\n")
			sb.WriteString("      suggested_area:\n")
			sb.WriteString("        value: \"" + area.Value + "\"\n")
		}
	}

	if !written {
		return nil
	}

	dir := filepath.Join(outputRoot, "coordinator")
	return writeYAMLFile(filepath.Join(dir, "imported.yaml"), sb.String())
}
