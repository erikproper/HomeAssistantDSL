/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationImportParser
 *
 * Parses Physical.def's "integration import with: ... end;" block -- one integration-agnostic
 * device-import form (unified 2026-09-05 from two previously separate, structurally unrelated
 * forms; see integration_import_storage.go's own header comment for the design rationale):
 *
 *   "device <local-id> from <remote-installation> <remote-device-id> with:
 *      <local-capability>: <domain>.<remote-entity-local-part>;
 *      ...
 *    end;"
 *
 * <remote-device-id> is the remote installation's own DeviceID for the device (the key its own
 * cloud-published discovery payloads' "device.identifiers[0]" carry). <domain>.<remote-entity-
 * local-part> is the remote installation's own already-positioned local Home Assistant entity id
 * for that capability (e.g. "sensor.junglinster_smarty_cpu_load") -- matched against each incoming
 * cloud discovery payload's own "default_entity_id" field to recognise which capability arrived.
 * The DSL author never states which integration kind ("hosts" or "home_assistant"/hassbridge) the
 * exporting installation used -- house_event_bus_coordinator/discoveryimport.go resolves that
 * purely from fields already present on the cloud discovery payload itself.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 05.09.2026
 *
 */

package main

import (
	"fmt"
	"regexp"
	"strings"
)

var importHeaderPattern = regexp.MustCompile(`^device\s+(\S+)\s+from\s+(\S+)\s+(\S+)\s+with:\s*$`)
var importCapabilityPattern = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_/]*):\s*(\S+)\s*;\s*$`)

// importShorthandCapabilityPattern matches "<domain>.<capability>;" -- added 2026-09-07 for a
// capability whose remote entity ref is fully predictable from (remote installation, remote
// device id, capability name) alone, letting the DSL author skip spelling out the exporting
// installation's own internal entity-naming convention by hand. Structurally unambiguous against
// importCapabilityPattern: the explicit form always has a colon right after the capability name,
// this form never does.
//
// <domain> is captured but never stored -- it exists purely for readability/convention consistency
// with hassbridge's own "domain always explicit" style. The LOCAL discovery config's own domain
// always comes from wherever Spaces.def positions this capability's LocalEntity, independent of
// both this declared domain and whatever domain the remote side actually used -- a deliberate
// coercion (house_event_bus_coordinator/discoveryimport.go's own stable-id-based matching, which
// never inspects the incoming payload's own domain at all).
var importShorthandCapabilityPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\.([A-Za-z_][A-Za-z0-9_]*)\s*;\s*$`)

// parseImportIntegrationBody parses the body lines of an "integration import with: ... end;"
// block. Lines that don't parse cleanly are reported as warnings rather than aborting the parse,
// matching parseHostsIntegrationBody's own policy.
func parseImportIntegrationBody(bodyLines []string) ([]TImportedDevice, []string) {
	var devices []TImportedDevice
	var warnings []string

	inCapabilities := false
	var current TImportedDevice

	for lineIdx, rawLine := range bodyLines {
		line := strings.TrimSpace(rawLine)
		if commentIdx := strings.Index(line, "#"); commentIdx >= 0 {
			line = strings.TrimSpace(line[:commentIdx])
		}
		if line == "" {
			continue
		}

		if inCapabilities {
			if line == "end;" {
				devices = append(devices, current)
				inCapabilities = false
				continue
			}
			if matches := importCapabilityPattern.FindStringSubmatch(line); matches != nil {
				current.Capabilities[matches[1]] = TImportedCapability{RemoteEntityRef: matches[2]}
				continue
			}
			if matches := importShorthandCapabilityPattern.FindStringSubmatch(line); matches != nil {
				// RemoteEntityRef left "" deliberately -- this is the shorthand form's own marker:
				// house_event_bus_coordinator/discoveryimport.go derives the remote stable id itself
				// from (device.RemoteInstallation, device.RemoteDeviceID, this capability's own map
				// key) when it finds one empty, rather than matching against a declared ref.
				current.Capabilities[matches[1]] = TImportedCapability{}
				continue
			}
			warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line inside imported device %q's capability block (body line %d): %q", current.DeviceID, lineIdx+1, line))
			continue
		}

		if matches := importHeaderPattern.FindStringSubmatch(line); matches != nil {
			current = TImportedDevice{
				DeviceID: matches[1], RemoteInstallation: matches[2], RemoteDeviceID: matches[3],
				Capabilities: map[string]TImportedCapability{},
			}
			inCapabilities = true
			continue
		}

		warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line in \"integration import\" block (body line %d): %q", lineIdx+1, line))
	}

	return devices, warnings
}

// collectImportedDevices re-parses Physical.def's "import" integration blocks (there may be more
// than one "integration import with: ... end;" chunk, same as any other integration) into one
// combined device list, for generatePhysicalIntegrationOutputs to pre-collect into
// TPhysicalGenerationContext.ImportedDevices before the generic per-block dispatch loop runs.
func collectImportedDevices(blocks []TIntegrationBlock) ([]TImportedDevice, []string) {
	var devices []TImportedDevice
	var warnings []string
	for _, block := range blocks {
		if block.Name != "import" {
			continue
		}
		blockDevices, blockWarnings := parseImportIntegrationBody(block.BodyLines)
		devices = append(devices, blockDevices...)
		warnings = append(warnings, blockWarnings...)
	}
	return devices, warnings
}

// collectImportedDevicesByID re-parses Physical.def's "import" integration blocks, keyed by
// DeviceID -- for generator.go to pre-collect alongside hostDevicesByID/hassBridgeDevicesByID/
// discoveryGatewaysByID, before Spaces.def parsing begins, mirroring collectHostsDevicesByID's own
// shape exactly. A separate, independent re-parse from generatePhysicalIntegrationOutputs' own
// collectImportedDevices call, matching this codebase's established convention
// (collectHassBridgeDevicesByID/collectHostsDevicesByID are each independently re-parsed wherever
// needed too, rather than threaded through as one shared value) -- Physical.def is cheap enough to
// reparse, and it keeps each caller's own concerns (Spaces.def resolution vs.
// coordinator/imported.yaml generation) fully independent.
func collectImportedDevicesByID(definitionDir string) (map[string]TImportedDevice, []string) {
	physicalContent, mergedLineNos, warnings := collectLayerContent(definitionDir, []string{"Physical.def"}, LayerPhysical)
	blocks, blockWarnings := parseIntegrationBlocks(physicalContent, mergedLineNos)
	warnings = append(warnings, blockWarnings...)

	devices, importWarnings := collectImportedDevices(blocks)
	warnings = append(warnings, importWarnings...)

	byID := map[string]TImportedDevice{}
	for _, d := range devices {
		if _, exists := byID[d.DeviceID]; !exists {
			byID[d.DeviceID] = d
		}
	}
	return byID, warnings
}
