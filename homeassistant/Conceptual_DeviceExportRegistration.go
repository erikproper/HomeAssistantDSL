/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualDeviceExportRegistration
 *
 * PROJECT.md 1.2b: an "export"-flagged home_assistant-bridge device (THassBridgeDevice.Export,
 * integration_hassbridge_storage.go) must behave as if all its declared entities were "used" --
 * the coordinator can only cross-post (house_event_bus_coordinator/discoveryhassbridge.go) and
 * the source instance can only report (remote_instance_entity_reporting.go) entities that already
 * have a DeviceConceptualLinks entry (Conceptual_DeviceEntities.go), and today that entry only
 * ever comes from an explicit Spaces.def "entity device.<spec> from <device-id> with: all
 * entities;" / "device <spec> from <device-id>;" positioning. A device whose data is meant to be
 * exported (e.g. Vienna's Netatmo readings, relayed via protocols-server-2, before they have --
 * or need -- any home in Junglinster's own Spaces.def) would otherwise never get registered at
 * all, silently defeating "export" for exactly the case it was built for.
 *
 * registerExportedHassBridgeDevices closes that gap: for every export-flagged device with no
 * existing DeviceConceptualLinks entry, it auto-registers one -- exactly as if "device
 * infrastructural:/<bare-id> from <device-id> with: all entities;" had been declared at the
 * top-level (root) space -- one entity per declared capability. Once that link exists,
 * generateHassBridgeFile/generateInstanceAutomationTrees pick it up through their own existing
 * "hasLink" gate with no further changes needed there.
 *
 * Scope, deliberately narrow: only a device with NO existing link at all is auto-registered here.
 * A device the DSL author already positioned in Spaces.def -- fully via "all entities", or
 * partially via the lighter "device <spec> from <device-id>;" form plus a handful of per-capability
 * lines -- keeps exactly what was explicitly declared; export does not retroactively expand a
 * manual positioning. Silently filling in the remaining capabilities under a *different*,
 * synthetic root-level identity would produce entities living at a visibly different path/display
 * name than their already-positioned siblings -- worse than just leaving the gap for the DSL
 * author to close explicitly. Flagged as an open edge case (a partially-positioned, exported
 * device's un-positioned capabilities stay un-registered), not solved here.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 30.08.2026
 *
 */

package main

import (
	"fmt"
	"sort"
	"strings"
)

// registerExportedHassBridgeDevices must run after Spaces.def has been fully parsed (admin's
// DeviceConceptualLinks reflects every explicit positioning already) and before
// generateHassBridgeFile/generateInstanceAutomationTrees read it -- Physical_Generator.go calls
// this right after collecting hassBridgeDevicesByID, before generateHassBridgeFile.
//
// Builds deviceIdentity/displayName directly (bypassing registerDevicePositioning's own
// administration.SpacePath-based resolution) via an explicitly absolute spec
// ("infrastructural:/<bare-id>", leading "/" after the colon) -- SpacePath is parse-time-only
// state that no longer reflects anything meaningful by the time this runs, and an absolute spec
// makes normalizeEntityFullName ignore it entirely regardless.
func registerExportedHassBridgeDevices(administration *TAdministrationState, hassBridgeDevicesByID map[string]THassBridgeDevice) []string {
	var warnings []string

	deviceIDs := make([]string, 0, len(hassBridgeDevicesByID))
	for id := range hassBridgeDevicesByID {
		deviceIDs = append(deviceIDs, id)
	}
	sort.Strings(deviceIDs)

	for _, deviceID := range deviceIDs {
		device := hassBridgeDevicesByID[deviceID]
		if !device.Export {
			continue
		}
		if _, hasLink := administration.DeviceConceptualLinks[deviceID]; hasLink {
			continue
		}

		bareLocalName := deviceID
		if dotIdx := strings.Index(deviceID, "."); dotIdx >= 0 {
			bareLocalName = deviceID[dotIdx+1:]
		}
		deviceSpec := "infrastructural:/" + bareLocalName

		deviceIdentity := extractEntityIdentity(normalizeEntityFullName("device."+deviceSpec, nil))
		if deviceIdentity.Sphere == "" || deviceIdentity.Path == "" {
			warnings = append(warnings, fmt.Sprintf("export: device %q: could not resolve a sphere/path from %q; skipping auto-registration", deviceID, deviceSpec))
			continue
		}
		displayName := deviceDisplayName("root", deviceIdentity.Sphere, deviceSpecLeafPath(deviceSpec))
		provenance := fmt.Sprintf("export: device %q declares \"export\" but is never positioned in Spaces.def; auto-registering all its entities", deviceID)

		if len(device.Capabilities) == 0 {
			warnings = append(warnings, fmt.Sprintf("%s: device %q declares no capabilities; nothing to register", provenance, deviceID))
			continue
		}

		constantAttrs := make(map[string]TDeviceAttributeConstant, len(device.ConstantAttributes)+len(device.DeviceInfoCapabilities))
		for name, attr := range device.ConstantAttributes {
			constantAttrs[name] = TDeviceAttributeConstant{Value: attr.Value, Forced: attr.Forced}
		}

		link := TDeviceConceptualLink{
			DisplayName:        displayName,
			ConstantAttributes: constantAttrs,
			AttributeEntityIDs: map[string]TDeviceAttributeLink{},
		}

		capabilityNames := make([]string, 0, len(device.Capabilities))
		for attr := range device.Capabilities {
			capabilityNames = append(capabilityNames, attr)
		}
		sort.Strings(capabilityNames)
		for _, attr := range capabilityNames {
			registerHassBridgeAttributeEntity(administration, device, deviceIdentity, "root", provenance, link, attr)
		}

		administration.DeviceConceptualLinks[deviceID] = link
	}
	return warnings
}
