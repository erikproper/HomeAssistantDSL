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
 * routing; (b) <outputRoot>/coordinator/commandline.yaml, the coordinator-consumable device/
 * capability list house_event_bus_coordinator/discoverycommandline.go reads to build MQTT
 * discovery configs -- scripts themselves never appear there, since the coordinator only ever
 * relays state/command topics, never invokes anything.
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
// "entity <spec> from <device-id> entity <capability>;" construct), returning the entity id and a
// sphere/path-derived display name -- both "" if this capability was never positioned in
// Spaces.def, in which case discoverycommandline.go falls back to its own auto-derived naming.
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

// generateCommandlineCoordinatorFile writes <outputRoot>/coordinator/commandline.yaml: every
// declared device's id/host plus its capability names grouped by kind -- no script content, since
// the coordinator only ever relays commandline/<host>/<entity>/... topics, never invokes a script
// itself. A capability positioned in Spaces.def (via "entity <spec> from <device-id> entity
// <capability>;") carries its resolved entity_id/name along too, so the coordinator can use the
// DSL author's own chosen conceptual position instead of an auto-derived "switch.<host>_<name>".
func generateCommandlineCoordinatorFile(outputRoot string, devices []TCommandlineDevice, admin *TAdministrationState) error {
	sortedDevices := append([]TCommandlineDevice{}, devices...)
	sort.Slice(sortedDevices, func(i, j int) bool { return sortedDevices[i].DeviceID < sortedDevices[j].DeviceID })

	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("devices:\n")
	for _, d := range sortedDevices {
		names := make([]string, 0, len(d.Capabilities))
		for name := range d.Capabilities {
			names = append(names, name)
		}
		sort.Strings(names)

		sb.WriteString("  " + d.DeviceID + ":\n")
		sb.WriteString("    host: \"" + d.Host + "\"\n")

		writeKindList := func(kind, section string) {
			var kindNames []string
			for _, name := range names {
				if d.Capabilities[name].Kind == kind {
					kindNames = append(kindNames, name)
				}
			}
			if len(kindNames) == 0 {
				return
			}
			sb.WriteString("    " + section + ":\n")
			for _, name := range kindNames {
				entityID, displayName := commandlineCapabilityRef(admin, d.DeviceID, name)
				if entityID == "" {
					sb.WriteString("      " + name + ": {}\n")
					continue
				}
				sb.WriteString("      " + name + ":\n")
				sb.WriteString("        entity_id: \"" + entityID + "\"\n")
				sb.WriteString("        name: \"" + displayName + "\"\n")
			}
		}
		writeKindList("switch", "switches")
		writeKindList("sensor", "sensors")
		writeKindList("button", "buttons")
	}

	dir := filepath.Join(outputRoot, "coordinator")
	return writeYAMLFile(filepath.Join(dir, "commandline.yaml"), sb.String())
}

// generateCommandlineIntegrationOutputs parses the "integration commandline with: ... end;"
// block's body into devices, then writes the per-host daemon config file(s), each host's own
// "main" broker secrets (mirroring cpu/ping's own "deployed the same way cpu/secrets.* already
// is" convention -- a commandline device is a plain local LAN device like ping/cpu, no per-device
// broker profile selection needed), and the coordinator-consumable commandline.yaml.
func generateCommandlineIntegrationOutputs(bodyLines []string, ctx TPhysicalGenerationContext) error {
	devices, warnings := parseCommandlineIntegrationBody(bodyLines)
	for _, w := range warnings {
		fmt.Printf("[integration commandline] %s\n", w)
	}
	if len(devices) == 0 {
		return nil
	}

	for _, d := range devices {
		if err := generateCommandlineHostFile(ctx.OutputRoot, d); err != nil {
			return err
		}
		if ctx.HasMQTTSecrets {
			if err := generateMQTTShellSecretsFile(ctx.OutputRoot, "commandline", "secrets."+d.Host, ctx.MQTTSecrets, ctx.Installation); err != nil {
				return err
			}
		}
	}
	return generateCommandlineCoordinatorFile(ctx.OutputRoot, devices, ctx.Admin)
}
