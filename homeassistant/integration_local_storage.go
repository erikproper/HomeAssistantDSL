/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationLocalStorage
 *
 * The data model for the "local" integration's parsed devices (2026-09-24, PROJECT.md item 11's
 * follow-up) -- entities that are native to HA "main" and can never be relayed over MQTT (a
 * camera's own video stream is the motivating case, the Ring doorbell). Unlike every other
 * integration kind, a "local" device's own capabilities carry no source/value/script at all: each
 * "<domain>.<label>;" line is only an ACKNOWLEDGEMENT that such an entity may exist locally, plus
 * (optionally) which sibling capability's presence it tracks ("<domain>.<label> <sibling> is
 * available;"). Conceptual.def's own "device <spec> from <device-id> with: entity ... from
 * <capability>; ...; end;" positioning is what actually CONFIRMS the claim, giving it the same
 * existence-checking discipline a plain bare "entity <spec>;" declaration already has (kind-5,
 * main_entities.go) -- see integration_local_parser.go's own header comment for the full grammar.
 *
 * Parsing (integration_local_parser.go) produces TLocalDevice values; this file doesn't parse
 * anything itself.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 24.09.2026
 *
 */

package main

import "fmt"

// TLocalCapability is one "<domain>.<label>;" or "<domain>.<label> <sibling-domain>.<sibling-label>
// is available;" declaration inside a "local" device's "with: ... end;" block. AvailabilityOf/
// AvailabilityOfDomain are set only for the latter shape -- mirrors TDiscoveryCapability's own
// identically-named fields (integration_discovery_storage.go) exactly, same rationale.
type TLocalCapability struct {
	Domain               string
	AvailabilityOf       string
	AvailabilityOfDomain string
}

// TLocalDevice is one "device <id> with: <domain>.<label>; ...; end;" declaration inside
// Physical.def's "integration local with: ... end;" block.
type TLocalDevice struct {
	DeviceID     string
	Capabilities map[string]TLocalCapability
}

// collectLocalDevicesByID re-parses Physical.def's "local" integration block and returns every
// declared device keyed by DeviceID -- mirrors collectCommandlineDevicesByID's own shape exactly.
// First declaration wins for a duplicated id.
func collectLocalDevicesByID(definitionDir string) (map[string]TLocalDevice, []string) {
	physicalContent, mergedLineNos, warnings := collectLayerContent(definitionDir, []string{"Physical.def"}, LayerPhysical)
	physicalContent = resolveDerivedConditionOneLiners(physicalContent)
	blocks, blockWarnings := parseIntegrationBlocks(physicalContent, mergedLineNos)
	warnings = append(warnings, blockWarnings...)

	byID := map[string]TLocalDevice{}
	for _, block := range blocks {
		if block.Name != "local" {
			continue
		}
		devices, bodyWarnings := parseLocalIntegrationBody(block.BodyLines)
		warnings = append(warnings, bodyWarnings...)
		for _, d := range devices {
			if _, exists := byID[d.DeviceID]; !exists {
				byID[d.DeviceID] = d
			} else {
				// See integration_hosts_storage.go's own "declared more than once" warning for the
				// real incident (2026-09-10) that prompted adding this across every integration
				// kind's own collector.
				warnings = append(warnings, fmt.Sprintf("Physical.def: device %q declared more than once within the \"local\" integration; keeping the first declaration", d.DeviceID))
			}
		}
	}
	return byID, warnings
}
