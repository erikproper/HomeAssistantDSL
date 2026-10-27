/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationCommandlineGenerator
 *
 * Turns the "commandline" integration's parsed devices (integration_commandline_storage.go) into
 * two outputs: (a) <outputRoot>/commandline/<host>.yaml, the target host's own script config for
 * the mqtt_commandline daemon (Integrations/mqtt_commandline/) -- deployed the same way
 * cpu/secrets.* already is, via a deploy.d step naming the host explicitly, not generator-decided
 * routing; (b) the registry manifest's entries for its entities (recordCommandlineRegistryEntries),
 * whose YAML "mqtt:" config commandline_yaml_entities.go writes (PROJECT.md 19e).
 * generateCommandlineIntegrationOutputs is the entry point registered in Physical_Storage.go's
 * integrationBodyParsers.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 06.09.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// generateCommandlineHostFile writes <outputRoot>/commandline/<host>.yaml -- one device's own
// script config, consumed by the mqtt_commandline daemon running on that host. If more than one
// declared device shares the same Host, the later one silently overwrites the earlier one's file
// (same "not a design goal, first/last wins" convention used elsewhere in this generator for
// genuinely duplicate declarations) -- one daemon instance per host is the only shape this item's
// design actually covers.
func generateCommandlineHostFile(outputRoot string, device TCommandlineDevice) error {
	names := make([]string, 0, len(device.Capabilities))
	for name := range device.Capabilities {
		names = append(names, name)
	}
	sort.Strings(names)

	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("host: \"" + device.Host + "\"\n")

	writeKindSection := func(kind, section string) {
		var kindNames []string
		for _, name := range names {
			if device.Capabilities[name].Kind == kind {
				kindNames = append(kindNames, name)
			}
		}
		if len(kindNames) == 0 {
			return
		}
		sb.WriteString(section + ":\n")
		for _, name := range kindNames {
			cap := device.Capabilities[name]
			sb.WriteString("  " + name + ":\n")
			switch kind {
			case "sensor":
				sb.WriteString("    status_script: \"" + cap.StatusScript + "\"\n")
			case "switch":
				sb.WriteString("    status_script: \"" + cap.StatusScript + "\"\n")
				sb.WriteString("    on_script: \"" + cap.OnScript + "\"\n")
				sb.WriteString("    off_script: \"" + cap.OffScript + "\"\n")
			case "button":
				sb.WriteString("    press_script: \"" + cap.PressScript + "\"\n")
			}
		}
	}
	writeKindSection("switch", "switches")
	writeKindSection("sensor", "sensors")
	writeKindSection("button", "buttons")

	dir := filepath.Join(outputRoot, "commandline")
	return writeYAMLFile(filepath.Join(dir, device.Host+".yaml"), sb.String())
}

// commandlineCapabilityRef looks up a commandline capability's conceptual positioning (registered
// by Conceptual_CommandlineEntities.go's registerCommandlineCapabilityEntityLink, via the ordinary
// "entity <spec> from <device-id> <capability>;" construct), returning the entity id and a
// sphere/path-derived display name -- both "" if this capability was never positioned in
// Spaces.def, in which case commandline_yaml_entities.go falls back to its own auto-derived naming.
func commandlineCapabilityRef(admin *TAdministrationState, deviceID, name string) (entityID, displayName string) {
	if admin == nil {
		return "", ""
	}
	link, ok := admin.DeviceConceptualLinks[deviceID]
	if !ok {
		return "", ""
	}
	attr, ok := link.AttributeEntityIDs[name]
	if !ok {
		return "", ""
	}
	return attr.EntityID, attr.Identity.Sphere + "/" + attr.Identity.Path
}

// recordCommandlineRegistryEntries records every commandline entity's unique_id and entity_id
// for the registry manifest (registry_manifest.go): the daemon's liveness node and each positioned
// capability. The coordinator no longer gets a commandline.yaml -- Home Assistant gets these
// entities from the generator's YAML (commandline_yaml_entities.go, PROJECT.md 19e).
func recordCommandlineRegistryEntries(devices []TCommandlineDevice, admin *TAdministrationState) {
	for _, d := range devices {
		for name, capability := range d.Capabilities {
			entityID, _ := commandlineCapabilityRef(admin, d.DeviceID, name)
			if entityID == "" {
				continue
			}
			uniqueID := d.DeviceID + "_" + name
			if capability.AssignedID != "" {
				uniqueID = capability.AssignedID
			}
			coordinatorPublishedEntities = append(coordinatorPublishedEntities, TRegistryManifestEntry{UniqueID: uniqueID, EntityID: entityID})
		}
		if nodeEntityID, _ := commandlineCapabilityRef(admin, d.DeviceID, "node"); nodeEntityID != "" {
			coordinatorPublishedEntities = append(coordinatorPublishedEntities, TRegistryManifestEntry{UniqueID: d.DeviceID + "_commandline_node", EntityID: nodeEntityID})
		}
	}
}

// generateCommandlineIntegrationOutputs parses the "integration commandline with: ... end;"
// block's body into devices, then writes the per-host daemon config file(s), each host's own
// "main" broker secrets (mirroring cpu/ping's own "deployed the same way cpu/secrets.* already
// is" convention -- a commandline device is a plain local LAN device like ping/cpu, no per-device
// broker profile selection needed), and records its entities for the registry manifest.
func generateCommandlineIntegrationOutputs(bodyLines []string, ctx TPhysicalGenerationContext) error {
	devices, warnings := parseCommandlineIntegrationBody(bodyLines)
	for _, w := range warnings {
		fmt.Printf("[integration commandline] %s\n", w)
	}
	if len(devices) == 0 {
		return nil
	}

	// "dependency on <id>;" (2026-10-07, declared directly inside this device's own Physical.def
	// block -- see TCommandlineDevice.DependsOn's own doc comment for why commandline gets this at
	// the physical layer directly, unlike hassbridge/import's Logical.def-only mechanism).
	// Single-hop only against hostDevicesByID -- no transitive chain walk, commandline devices
	// aren't part of Logical.def's own dependency graph.
	for i := range devices {
		for _, depID := range devices[i].DependsOn {
			host, ok := ctx.HostDevicesByID[depID]
			if !ok {
				fmt.Printf("[integration commandline] device %q depends on %q, which isn't a \"hosts\" integration device (commandline dependency resolution isn't built for any other kind yet) -- ignoring this dependency\n", devices[i].DeviceID, depID)
				continue
			}
			mat, ok := MaterializationForIntegrationType(host.IntegrationType)
			if !ok {
				continue // can't happen for a real THostDevice (IntegrationType is validated at parse time)
			}
			if topic := mat.NodeTopic(host.HostName); topic != "" {
				devices[i].DependsOnAvailabilityTopics = append(devices[i].DependsOnAvailabilityTopics, topic)
			}
		}
	}

	for _, d := range devices {
		if err := generateCommandlineHostFile(ctx.OutputRoot, d); err != nil {
			return err
		}
		if ctx.HasMQTTSecrets {
			if err := generateMQTTShellSecretsFile(ctx.OutputRoot, "commandline", "secrets."+d.Host, ctx.MQTTSecrets); err != nil {
				return err
			}
		}
	}
	recordCommandlineRegistryEntries(devices, ctx.Admin)
	for _, report := range generateYAMLCommandlineEntities(devices, ctx) {
		fmt.Printf("[integration commandline] %s\n", report)
	}
	return nil
}
