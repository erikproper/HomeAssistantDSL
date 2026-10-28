/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationLocalStorage
 *
 * The data model for the "local" integration's parsed devices (2026-09-24, PROJECT.md item 11's
 * follow-up) -- entities that are native to HA "main" and can never be relayed over MQTT (a
 * camera's own video stream is the motivating case, the Ring doorbell). Most of a "local" device's
 * own capabilities carry no source/value/script at all: each "<domain>.<label>;" line is only an
 * ACKNOWLEDGEMENT that such an entity may exist locally, plus (optionally) which sibling
 * capability's presence it tracks ("<domain>.<label> <sibling> is available;"). Conceptual.def's
 * own "device <spec> from <device-id> with: entity ... from <capability>; ...; end;" positioning
 * is what actually CONFIRMS the claim, giving it the same existence-checking discipline a plain
 * bare "entity <spec>;" declaration already has (kind-5, main_entities.go) -- see
 * integration_local_parser.go's own header comment for the full grammar.
 *
 * One real exception (2026-09-30): "derived DDD.NNN from EEE.MMM via TTT;" (TLocalCapability's own
 * DerivedFromCapability/DerivedViaTemplate fields) IS a generator-computed value, not a native
 * acknowledgement -- added so a "local" device's battery_alert (etc.) can live in Physical.def
 * where Rule 2 (physical_rule_battery_alert_biconditional.go) can see it, matching discovery/
 * hassbridge's own long-established "derived" mechanism (Physical_DerivedCapability.go) exactly.
 *
 * Parsing (integration_local_parser.go) produces TLocalDevice values; this file doesn't parse
 * anything itself.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 30.09.2026
 *
 */

package main

import "fmt"

// TLocalCapability is one "<domain>.<label>;", "<domain>.<label> <sibling-domain>.<sibling-label>
// is available;", or "switch.<label> <media-player-entity> [with: no_play_input "<value>"; end;];"
// declaration inside a "local" device's "with: ... end;" block. AvailabilityOf/AvailabilityOfDomain
// are set only for the "is available" shape -- mirrors TDiscoveryCapability's own identically-named
// fields (integration_discovery_storage.go) exactly, same rationale.
//
// MediaPlayerEntity/NoPlayInput (2026-09-27, PROJECT.md's own "logical layer should only ever
// define genuinely derived entities" principle) are set only for the media_player->switch
// coercion shape -- the SAME "media_switch" coercion Logical.def's own "switch.<label>
// <media-player-entity> [with: no_play_input ...; end;];" capability used to perform (moved here
// since a bare domain relabel of one already-existing literal local entity isn't a cross-device
// derivation). MediaPlayerEntity is the literal source entity_id (e.g.
// "media_player.social_house_living_room_tv"); registration
// (registerLocalCapabilityEntityLink, Conceptual_LocalEntities.go) reuses
// generateMediaSwitches' own rendering wholesale, exactly like the Logical.def-sourced version
// already did.
//
// DerivedFromCapability/DerivedViaTemplate (2026-09-30, closing the gap PROJECT.md 1d's
// battery_level-IFF-battery_alert rule found live: a "local" device's own battery_alert could
// only ever be expressed as a Logical.def "derived" capability, which Rule 2 deliberately never
// looks at -- see physical_rule_battery_alert_biconditional.go's own header). Set only for a
// "derived DDD.NNN from EEE.MMM via TTT;" declaration (Physical_DerivedCapability.go's shared
// parser, the SAME one discovery/hassbridge already use) -- mirrors TDiscoveryCapability's own
// identically-named fields exactly: DerivedFromCapability holds EEE.MMM's own map key (MMM),
// DerivedViaTemplate is TTT ("$" standing for the sibling's own resolved value). Mutually
// exclusive with AvailabilityOf/MediaPlayerEntity.
type TLocalCapability struct {
	Domain                string
	AvailabilityOf        string
	AvailabilityOfDomain  string
	MediaPlayerEntity     string
	NoPlayInput           string
	DerivedFromCapability string
	DerivedViaTemplate    string
	// NativeEntity (PROJECT.md 21, 2026-10-28): the entity_id the native integration gave this
	// capability's entity ("capability sensor.cpu/load sensor.processor_use;"). The registry repair
	// renames that entity to the capability's positioned id the first time (TRegistryManifestNative
	// .Seed); afterwards the positioned id is its name, and this one only identifies it.
	NativeEntity string
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
	var jinjaWarnings []string
	physicalContent, jinjaWarnings = resolveJinjaTemplateCallsInDerivedLines(physicalContent, loadJinjaTemplateDefinitions(definitionDir))
	warnings = append(warnings, jinjaWarnings...)
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
