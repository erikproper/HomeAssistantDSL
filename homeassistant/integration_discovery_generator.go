/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationDiscoveryGenerator
 *
 * Turns the "discovery" integration's parsed gateway devices (integration_discovery_storage.go)
 * plus Spaces.def's "entity <spec> from <gateway-id>.<leaf>;" links
 * (Conceptual_DiscoveryEntities.go, ctx.Admin.DiscoveryEntityLinks) into
 * coordinator/discovery.yaml -- a separate file from coordinator/devices.yaml (integration
 * hosts_generator.go) since both integrations' generator functions are dispatched and write
 * their output independently (Physical_Generator.go); sharing one file would mean whichever
 * integration's handler runs second overwrites the first's output.
 * generateDiscoveryIntegrationOutputs is the entry point registered in Physical_Storage.go's
 * integrationBodyParsers.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 22.08.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// generateDiscoveryPrefixBaselineFile writes just physical_prefix to coordinator/discovery.yaml --
// the same "always establish a baseline, let a fuller later generator overwrite it" pattern
// generatePhysicalIntegrationOutputs' own baseline devices.yaml call already uses
// (Physical_Generator.go). Needed because a house may declare discovery_passthrough rules
// (discovery_passthrough.go) without ever declaring an "integration discovery with: ... end;"
// block at all -- generateDiscoveryIntegrationOutputs below only runs when that block exists, but
// the coordinator still needs physical_prefix to subscribe on for passthrough alone, even with
// zero declared gateways. Skipped entirely (no file written) when physicalPrefix is unset, same
// "nothing declared" convention discovery.yaml/discovery_cleanup.yaml both already follow. Called
// early in generatePhysicalIntegrationOutputs, before "integration discovery with:" blocks are
// even parsed -- if one turns out to exist, generateDiscoveryIntegrationOutputs's own later write
// overwrites this with the fuller content (it already includes physical_prefix itself), so nothing
// is lost either way.
func generateDiscoveryPrefixBaselineFile(outputRoot, physicalPrefix string) error {
	if physicalPrefix == "" {
		return nil
	}
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("physical_prefix: \"" + physicalPrefix + "\"\n")
	dir := filepath.Join(outputRoot, "coordinator")
	return writeYAMLFile(filepath.Join(dir, "discovery.yaml"), sb.String())
}

// generateDiscoveryIntegrationOutputs parses the "integration discovery with: ... end;" block's
// body into gateway devices, then writes coordinator/discovery.yaml: the declared gateways (id +
// identifiers), the resolved ${mqtt_discovery_physical} topic prefix the coordinator should
// subscribe to, and every "entity ... from <gateway-id>.<leaf>;" link Spaces.def declared
// (ctx.Admin.DiscoveryEntityLinks -- already fully populated by the time any Physical.def
// integration handler runs, since Spaces.def parsing happens first). Missing
// MQTTDiscoveryPhysicalPrefix is not an error -- the coordinator just has nothing to bridge yet.
func generateDiscoveryIntegrationOutputs(bodyLines []string, ctx TPhysicalGenerationContext) error {
	gateways, warnings := parseDiscoveryIntegrationBody(bodyLines)
	for _, w := range warnings {
		fmt.Printf("[integration discovery] %s\n", w)
	}
	if len(gateways) == 0 {
		return nil
	}
	if ctx.MQTTDiscoveryPhysicalPrefix == "" {
		fmt.Printf("[integration discovery] ${mqtt_discovery_physical} not set in Settings.def; declared gateways have nothing to bridge from\n")
	}

	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("physical_prefix: \"" + ctx.MQTTDiscoveryPhysicalPrefix + "\"\n")

	sb.WriteString("gateways:\n")
	sortedGateways := append([]TDiscoveryGatewayDevice{}, gateways...)
	sort.Slice(sortedGateways, func(i, j int) bool { return sortedGateways[i].DeviceID < sortedGateways[j].DeviceID })
	for _, g := range sortedGateways {
		sb.WriteString("  " + g.DeviceID + ":\n")
		sb.WriteString("    identifiers:\n")
		for _, id := range g.Identifiers {
			sb.WriteString("      - \"" + id + "\"\n")
		}
	}

	if ctx.Admin != nil && len(ctx.Admin.DiscoveryEntityLinks) > 0 {
		sb.WriteString("entity_links:\n")
		entityIDs := make([]string, 0, len(ctx.Admin.DiscoveryEntityLinks))
		for id := range ctx.Admin.DiscoveryEntityLinks {
			entityIDs = append(entityIDs, id)
		}
		sort.Strings(entityIDs)
		for _, id := range entityIDs {
			link := ctx.Admin.DiscoveryEntityLinks[id]
			sb.WriteString("  " + id + ":\n")
			sb.WriteString("    gateway: " + link.GatewayDeviceID + "\n")
			sb.WriteString("    leaf: " + link.Leaf + "\n")
			if link.DeviceClass != "" {
				sb.WriteString("    device_class: " + link.DeviceClass + "\n")
			}
			if link.Unit != "" {
				sb.WriteString("    unit: \"" + link.Unit + "\"\n")
			}
			if link.StateClass != "" {
				sb.WriteString("    state_class: " + link.StateClass + "\n")
			}
			if link.Icon != "" {
				sb.WriteString("    icon: " + link.Icon + "\n")
			}
		}
	}

	dir := filepath.Join(ctx.OutputRoot, "coordinator")
	return writeYAMLFile(filepath.Join(dir, "discovery.yaml"), sb.String())
}
