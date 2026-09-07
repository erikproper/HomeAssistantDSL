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
 *      <domain>.<capability>;
 *      ...
 *    end;"
 *
 * <remote-device-id> is the remote installation's own DeviceID for the device (the key its own
 * cloud-published discovery payloads' "device.identifiers[0]" carry). <domain> is captured but
 * never stored -- it exists purely for readability/convention consistency with hassbridge's own
 * "domain always explicit" style; the LOCAL discovery config's own domain always comes from
 * wherever Spaces.def positions this capability's LocalEntity, independent of whatever domain the
 * remote side actually used (a deliberate coercion,
 * house_event_bus_coordinator/discoveryimport.go's own stable-id-based matching, which never
 * inspects the incoming payload's own domain at all). <capability> is combined with
 * (RemoteInstallation, RemoteDeviceID) to compute the exporter's own stable id (exportStableID,
 * house_event_bus_coordinator/discoveryhassbridge.go) -- the DSL author never spells out the
 * exporting installation's own internal entity naming by hand, and never states which integration
 * kind ("hosts" or "home_assistant"/hassbridge) the exporting installation used;
 * discoveryimport.go resolves that purely from fields already present on the cloud discovery
 * payload itself. <capability> allows "/" (e.g. "sensor.cpu/load;") since a hosts-kind export's
 * capability names are group-prefixed (integration_hosts_storage.go's AttributeNames).
 *
 * This grammar replaced an earlier, more verbose explicit form
 * ("<local-capability>: <domain>.<remote-entity-local-part>;", requiring the DSL author to look up
 * and copy the exporter's own local entity_id by hand) on 2026-09-07, once no real Physical.def
 * still used it -- removed outright rather than kept alongside, per the user's own explicit
 * instruction, since the shorthand form covers every case the explicit form did.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 07.09.2026
 *
 */

package main

import (
	"fmt"
	"regexp"
	"strings"
)

var importHeaderPattern = regexp.MustCompile(`^device\s+(\S+)\s+from\s+(\S+)\s+(\S+)\s+with:\s*$`)

// importCapabilityPattern matches "<domain>.<capability>;" -- see this file's own header comment
// for the full rationale.
var importCapabilityPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\.([A-Za-z_][A-Za-z0-9_/]*)\s*;\s*$`)

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
