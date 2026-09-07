/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationHassBridgeGenerator
 *
 * Writes <outputRoot>/coordinator/homeassistant_bridge.yaml: for every "home_assistant"
 * integration device with a conceptual link (a "device <spec> from hass.X with: ...; end;"
 * declaration in Spaces.def -- see registerDevicePositioning/registerDeviceCapabilityEntityLink,
 * Conceptual_DevicePositioning.go/Conceptual_DeviceCapabilityEntities.go), which named instance it
 * bridges from and, per declared capability, both the remote source entity_id and the resolved
 * local entity_id the coordinator should discovery-publish it as. The coordinator needs both:
 * which remote entity to expect a value report for, and which local entity_id to publish it
 * under. Also carries the device's constant_attributes (static, generate-time-known values) and
 * device_info_capabilities (dynamic -- names only; the coordinator learns the actual values from
 * the device-info report topic, remote_instance_automations.go's own device-info publish action)
 * so the coordinator can build a real "device:" block for every entity it discovery-publishes for
 * this device, the same way hosts devices already get one. Also carries "export: true" when the
 * device was declared "device <id> export with: ...;" (PROJECT.md 1.2a) -- the coordinator's own
 * cue (house_event_bus_coordinator/discoveryhassbridge.go) to additionally cross-post this
 * device's metadata/state traffic onto the cloud broker.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 23.08.2026
 *
 */

package main

import (
	"path/filepath"
	"sort"
	"strings"
)

// generateHassBridgeFile writes coordinator/homeassistant_bridge.yaml for every hassBridgeDevice
// that also has a DeviceConceptualLinks entry (i.e. was actually positioned via a device.<spec>
// from <device-id> with: attributes; declaration -- a device declared in
// Physical.def but never positioned in Spaces.def has nothing to bridge yet, so it's silently
// skipped here, same as "hosts" devices with no conceptual link). Skipped entirely (no file
// written) when nothing qualifies.
func generateHassBridgeFile(outputRoot string, hassBridgeDevicesByID map[string]THassBridgeDevice, admin *TAdministrationState) error {
	deviceIDs := make([]string, 0, len(hassBridgeDevicesByID))
	for id := range hassBridgeDevicesByID {
		deviceIDs = append(deviceIDs, id)
	}
	sort.Strings(deviceIDs)

	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("devices:\n")
	written := false

	for _, deviceID := range deviceIDs {
		device := hassBridgeDevicesByID[deviceID]
		link, hasLink := admin.DeviceConceptualLinks[deviceID]
		if !hasLink {
			continue
		}

		capabilityNames := make([]string, 0, len(device.Capabilities))
		for name := range device.Capabilities {
			capabilityNames = append(capabilityNames, name)
		}
		sort.Strings(capabilityNames)

		var capLines strings.Builder
		anyResolved := false
		for _, name := range capabilityNames {
			attr, resolved := link.AttributeEntityIDs[name]
			if !resolved {
				continue
			}
			anyResolved = true
			capLines.WriteString("      " + name + ":\n")
			// source_entities is per-instance, never a single value -- a roaming device's own
			// local entity_id for the same capability can genuinely differ across instances
			// (found live 2026-09-05).
			capLines.WriteString("        source_entities:\n")
			sourceInstances := make([]string, 0, len(device.Capabilities[name].Sources))
			for instance := range device.Capabilities[name].Sources {
				sourceInstances = append(sourceInstances, instance)
			}
			sort.Strings(sourceInstances)
			for _, instance := range sourceInstances {
				capLines.WriteString("          " + instance + ": " + device.Capabilities[name].Sources[instance] + "\n")
			}
			capLines.WriteString("        local_entity: " + attr.EntityID + "\n")
			if attr.DeviceClass != "" {
				capLines.WriteString("        device_class: " + attr.DeviceClass + "\n")
			}
			if attr.Unit != "" {
				capLines.WriteString("        unit: \"" + attr.Unit + "\"\n")
			}
			if attr.StateClass != "" {
				capLines.WriteString("        state_class: " + attr.StateClass + "\n")
			}
			if attr.Icon != "" {
				capLines.WriteString("        icon: " + attr.Icon + "\n")
			}
		}
		if !anyResolved {
			continue
		}

		written = true
		sb.WriteString("  " + deviceID + ":\n")
		sb.WriteString("    instances:\n")
		instances := append([]string{}, device.Instances...)
		sort.Strings(instances)
		for _, instance := range instances {
			sb.WriteString("      - " + instance + "\n")
		}
		if device.Export {
			sb.WriteString("    export: true\n")
		}
		if device.ExportAs != "" {
			sb.WriteString("    export_as: \"" + device.ExportAs + "\"\n")
		}
		if device.SelfImportFrom != "" {
			sb.WriteString("    self_import_from: \"" + device.SelfImportFrom + "\"\n")
		}
		if link.DisplayName != "" {
			sb.WriteString("    display_name: " + link.DisplayName + "\n")
		}
		sb.WriteString("    capabilities:\n")
		sb.WriteString(capLines.String())

		if len(device.ConstantAttributes) > 0 {
			sb.WriteString("    constant_attributes:\n")
			names := make([]string, 0, len(device.ConstantAttributes))
			for name := range device.ConstantAttributes {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				attr := device.ConstantAttributes[name]
				sb.WriteString("      " + name + ":\n")
				sb.WriteString("        value: \"" + attr.Value + "\"\n")
				if attr.Forced {
					sb.WriteString("        forced: true\n")
				}
			}
		}

		if len(device.DeviceInfoCapabilities) > 0 {
			sb.WriteString("    device_info_capabilities:\n")
			names := make([]string, 0, len(device.DeviceInfoCapabilities))
			for name := range device.DeviceInfoCapabilities {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				sb.WriteString("      " + name + ":\n")
				sb.WriteString("        source: " + device.DeviceInfoCapabilities[name] + "\n")
				if prefix, hasPrefix := device.DeviceInfoLiteralPrefixes[name]; hasPrefix {
					sb.WriteString("        literal_prefix: \"" + prefix + "\"\n")
				}
			}
		}
	}

	if !written {
		return nil
	}

	dir := filepath.Join(outputRoot, "coordinator")
	return writeYAMLFile(filepath.Join(dir, "homeassistant_bridge.yaml"), sb.String())
}
