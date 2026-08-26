/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationHostsGenerator
 *
 * Turns the "hosts" integration's parsed/validated devices (integration_hosts_storage.go)
 * into generated output: the ping integration's host list and MQTT secrets file, a matching
 * cpu integration secrets file, a devices.yaml input file for the coordinator, and (for
 * home_assistant-type devices) reporting automations that poll their capability entities and
 * publish them as JSON on the device's MQTT state topic (integration_hosts_storage.go's
 * THostsEntityMaterialization.StateTopicTemplate is the single source of truth for that topic
 * -- this file never hardcodes it). generateHostsIntegrationOutputs is the entry point
 * registered in Physical_Storage.go's integrationBodyParsers.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 19.08.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// generatePingHostsFile writes <outputRoot>/ping/hosts: one host name per line, matching
// the plain-text format the existing ping/report script already expects (it globs
// ~/ping/hosts*). Deployment from this generated file to the actual Integrations/ping/
// location is handled outside the generator. Excludes "cloud"-routed devices -- a device that
// may be roaming can't be meaningfully LAN-pinged, and the coordinator now owns their node/state
// liveness itself, via a 3-minute no-traffic timeout rather than ping (mqtt_relay.go's
// TCloudLivenessTracker, house_event_bus_coordinator) -- ping/report publishing a stale "false"
// for an off-LAN device would just fight that timeout on the same retained topic.
func generatePingHostsFile(outputRoot string, devices []THostDevice) error {
	var pingable []THostDevice
	for _, d := range devices {
		if !d.Cloud {
			pingable = append(pingable, d)
		}
	}
	hosts := dedupedHostNames(pingable)
	var sb strings.Builder
	for _, host := range hosts {
		sb.WriteString(host + "\n")
	}
	dir := filepath.Join(outputRoot, "ping")
	return writeYAMLFile(filepath.Join(dir, "hosts"), sb.String())
}

// generateCPUBrokerSecretsFiles writes <outputRoot>/cpu/secrets.<name> for every declared MQTT
// broker profile except ones marked "coordinator_only true;" (TMQTTBrokerSecrets.CoordinatorOnly
// -- those hold credentials meant for the coordinator process, never for a leaf host's report
// script). "main" is written as "secrets.default", matching cpu/report's own "$1, defaulting to
// default" invocation convention -- every other profile keeps its own declared name (e.g.
// "secrets.cloud_client"). Which of these a given host machine's scheduler (cron/launchd) actually
// invokes cpu/report against is entirely outside this generator's concern -- it just makes every
// leaf-usable profile's secrets available, once, for any host to use.
func generateCPUBrokerSecretsFiles(outputRoot string, profiles map[string]TMQTTBrokerSecrets, installation string) error {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		secrets := profiles[name]
		if secrets.CoordinatorOnly {
			continue
		}
		filename := "secrets." + name
		// "main" never needs installation-qualification -- its reports stay on the house's own
		// local broker, under the plain "hosts/<host>/..." topics every local consumer already
		// expects (only a cloud-routed report needs to disambiguate itself from another
		// installation sharing the same broker -- see cpu/report's own topic-selection logic).
		reportInstallation := installation
		if name == "main" {
			filename = "secrets.default"
			reportInstallation = ""
		}
		if err := generateMQTTShellSecretsFile(outputRoot, "cpu", filename, secrets, reportInstallation); err != nil {
			return err
		}
	}
	return nil
}

// generateMQTTShellSecretsFile writes <outputRoot>/<subdir>/<filename>: one MQTT broker
// profile's connection values in the plain "key=value" shell-sourceable format
// Integrations/ping/report and Integrations/cpu/report already expect (they do
// "source ~/ping/secrets" / "source ~/cpu/secrets" for the "main" broker's file -- see
// generateHostsIntegrationOutputs for how a non-"main" broker gets its own "secrets.<broker>"
// file alongside it). mqtt_tls is always written (1 or 0), not omitted when false, so a report
// script can rely on the key always being present rather than treating "absent" as meaningful.
// Deployment from this generated file to the actual Integrations/ location is handled outside
// the generator, same as ping/hosts.
// installation, when non-empty, is written as "installation=<value>" -- cpu/report uses its
// presence (not just its value) to decide whether to qualify its report topics with it, so this
// must be omitted entirely for "main" (a purely local report), not just left blank.
func generateMQTTShellSecretsFile(outputRoot, subdir, filename string, secrets TMQTTBrokerSecrets, installation string) error {
	var sb strings.Builder
	sb.WriteString("mqtt_server=" + secrets.Server + "\n")
	sb.WriteString("mqtt_login=" + secrets.Login + "\n")
	sb.WriteString("mqtt_password=" + secrets.Password + "\n")
	sb.WriteString("mqtt_port=" + secrets.Port + "\n")
	tls := "0"
	if secrets.TLS {
		tls = "1"
	}
	sb.WriteString("mqtt_tls=" + tls + "\n")
	if installation != "" {
		sb.WriteString("installation=" + installation + "\n")
	}
	dir := filepath.Join(outputRoot, subdir)
	return writeYAMLFile(filepath.Join(dir, filename), sb.String())
}

// generateCoordinatorDevicesFile writes <outputRoot>/coordinator/devices.yaml: the
// physical-layer device/capability list the coordinator needs to build discovery
// messages, enriched (when admin has a link for a device -- see
// Conceptual_DeviceEntities.go) with the conceptual-layer HA entity ids an "entity
// device.<name> from <device-id> ...;" declaration in Spaces.def implies for it. Devices
// with no such declaration yet simply have no "conceptual:" key -- most don't, since this
// positioning is opt-in per device, not required. conceptualPrefix (resolved from
// ${mqtt_discovery_conceptual}, defaulting to "homeassistant") is written unconditionally as a
// top-level field -- the prefix the coordinator publishes our own conceptual-layer discovery
// under, mirroring coordinator/discovery.yaml's physical_prefix for the source side.
func generateCoordinatorDevicesFile(outputRoot string, devices []THostDevice, admin *TAdministrationState, conceptualPrefix, installation string) error {
	seen := map[string]bool{}
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("mqtt_discovery_conceptual_prefix: \"" + conceptualPrefix + "\"\n")
	if installation != "" {
		sb.WriteString("installation: \"" + installation + "\"\n")
	}
	sb.WriteString("devices:\n")
	for _, d := range devices {
		if seen[d.DeviceID] {
			continue
		}
		seen[d.DeviceID] = true
		sb.WriteString("  " + d.DeviceID + ":\n")
		sb.WriteString("    host: " + d.HostName + "\n")
		sb.WriteString("    integration: " + d.IntegrationType + "\n")
		if d.Cloud {
			sb.WriteString("    cloud: true\n")
		}
		if d.Local {
			sb.WriteString("    local: true\n")
		}
		if d.ImportedFrom != "" {
			sb.WriteString("    imported_from: \"" + d.ImportedFrom + "\"\n")
		}
		if mat, known := MaterializationForIntegrationType(d.IntegrationType); known {
			if topic := mat.StateTopic(d.HostName); topic != "" {
				sb.WriteString("    topic: " + topic + "\n")
			}
			if nodeTopic := mat.NodeTopic(d.HostName); nodeTopic != "" {
				sb.WriteString("    node_topic: " + nodeTopic + "\n")
			}
			if deviceInfoTopic := mat.DeviceInfoTopic(d.HostName); deviceInfoTopic != "" {
				sb.WriteString("    device_info_topic: " + deviceInfoTopic + "\n")
			}
		}
		if len(d.Capabilities) == 0 {
			sb.WriteString("    capabilities: {}\n")
		} else {
			sb.WriteString("    capabilities:\n")
			capNames := make([]string, 0, len(d.Capabilities))
			for name := range d.Capabilities {
				capNames = append(capNames, name)
			}
			sort.Strings(capNames)
			for _, name := range capNames {
				sb.WriteString("      " + name + ":\n")
				sb.WriteString("        entity: " + d.Capabilities[name] + "\n")
			}
		}
		if admin != nil {
			if link, hasLink := admin.DeviceConceptualLinks[d.DeviceID]; hasLink {
				sb.WriteString("    conceptual:\n")
				sb.WriteString("      node_entity: " + link.NodeEntityID + "\n")
				if link.NodeDeviceClass != "" {
					sb.WriteString("      node_device_class: " + link.NodeDeviceClass + "\n")
				}
				if link.NodeIcon != "" {
					sb.WriteString("      node_icon: " + link.NodeIcon + "\n")
				}
				if link.DisplayName != "" {
					sb.WriteString("      display_name: " + link.DisplayName + "\n")
				}
				if len(link.ConstantAttributes) > 0 {
					sb.WriteString("      constant_attributes:\n")
					attrNames := make([]string, 0, len(link.ConstantAttributes))
					for name := range link.ConstantAttributes {
						attrNames = append(attrNames, name)
					}
					sort.Strings(attrNames)
					for _, name := range attrNames {
						attr := link.ConstantAttributes[name]
						sb.WriteString("        " + name + ":\n")
						sb.WriteString("          value: \"" + attr.Value + "\"\n")
						if attr.Forced {
							sb.WriteString("          forced: true\n")
						}
					}
				}
				if len(link.AttributeEntityIDs) > 0 {
					sb.WriteString("      attribute_entities:\n")
					attrNames := make([]string, 0, len(link.AttributeEntityIDs))
					for name := range link.AttributeEntityIDs {
						attrNames = append(attrNames, name)
					}
					sort.Strings(attrNames)
					for _, name := range attrNames {
						attr := link.AttributeEntityIDs[name]
						sb.WriteString("        " + name + ":\n")
						sb.WriteString("          entity: " + attr.EntityID + "\n")
						if attr.DeviceClass != "" {
							sb.WriteString("          device_class: " + attr.DeviceClass + "\n")
						}
						if attr.Unit != "" {
							sb.WriteString("          unit: \"" + attr.Unit + "\"\n")
						}
						if attr.StateClass != "" {
							sb.WriteString("          state_class: " + attr.StateClass + "\n")
						}
						if attr.Icon != "" {
							sb.WriteString("          icon: " + attr.Icon + "\n")
						}
					}
				}
			}
		}
	}
	dir := filepath.Join(outputRoot, "coordinator")
	return writeYAMLFile(filepath.Join(dir, "devices.yaml"), sb.String())
}

// generateReportingAutomations writes one automation per home_assistant-type device: a
// periodic time_pattern trigger whose actions read each declared capability's local entity and
// publish them as JSON, split by whether the capability's declared name carries an explicit
// "<group>/<leaf>" prefix (splitCapabilityName, integration_hosts_storage.go) into up to two
// mqtt.publish actions -- grouped variable attributes (e.g. "cpu/load", "cpu/temperature") to
// StateTopic, keyed by their bare leaf name ("load") to match Integrations/cpu/report's flat
// {"load": ..., "temperature": ...} JSON shape; ungrouped device-info strings (sw_version,
// model, ...) to DeviceInfoTopic, plus literal 'via_device'/'name' fields (brokerHostName, the
// device's own HostName) on the latter. Posting a variable attribute through the device-info
// action (or vice versa) would be wrong: the former gets | float(0)-coerced, the latter doesn't.
// A capability's entity reference may use the same "<entity>!<attribute>" syntax Spaces.def
// entity bodies already use (e.g. "value sensor.X!state_message;") to read one of the entity's
// attributes instead of its bare state -- sourceToJinja2 (generator.go) resolves it either way,
// needed for e.g. "sw_version: update.home_assistant_operating_system_update!installed_version;",
// since an `update` entity's own state is just on/off, not a version string. Devices with no
// capabilities, or whose type has neither topic, are skipped.
func generateReportingAutomations(haOutputDir string, devices []THostDevice, brokerHostName string) error {
	if haOutputDir == "" {
		return nil
	}
	for _, d := range devices {
		if d.IntegrationType != "home_assistant" || len(d.Capabilities) == 0 {
			continue
		}
		mat, known := MaterializationForIntegrationType(d.IntegrationType)
		if !known {
			continue
		}
		topic := mat.StateTopic(d.HostName)
		deviceInfoTopic := mat.DeviceInfoTopic(d.HostName)

		// Split declared capability names by whether they carry an explicit group prefix:
		// grouped ("cpu/load") -> numeric variable attribute, posted to the cpu/state-style
		// topic under its bare leaf name; ungrouped ("sw_version") -> device-info string,
		// posted to the device/state-style topic under its own (already bare) name.
		var attrDeclared, deviceInfoNames []string
		attrLeafByDeclared := map[string]string{}
		for declared := range d.Capabilities {
			if group, leaf := splitCapabilityName(declared); group != "" {
				attrDeclared = append(attrDeclared, declared)
				attrLeafByDeclared[declared] = leaf
			} else {
				deviceInfoNames = append(deviceInfoNames, declared)
			}
		}
		sort.Strings(attrDeclared)
		sort.Strings(deviceInfoNames)

		if (len(attrDeclared) == 0 || topic == "") && (len(deviceInfoNames) == 0 || deviceInfoTopic == "") {
			continue
		}

		automationID := "reporting_device_" + d.DeviceID

		var sb strings.Builder
		sb.WriteString(generatorHeader)
		sb.WriteString("- alias: " + automationID + "\n")
		sb.WriteString("  id: automation." + automationID + "\n")
		sb.WriteString("  initial_state: True\n")
		sb.WriteString("  mode: queued\n")
		sb.WriteString("  trigger:\n")
		sb.WriteString("  - platform: time_pattern\n")
		sb.WriteString("    minutes: '/1'\n")
		sb.WriteString("  action:\n")

		// capabilityExpr builds name's Jinja value expression, prepending its declared literal
		// prefix (if any) as a Jinja string concatenation ("'<literal>' ~ <expr>").
		capabilityExpr := func(name string) string {
			expr := sourceToJinja2(d.Capabilities[name])
			if prefix, hasPrefix := d.CapabilityLiteralPrefixes[name]; hasPrefix {
				expr = jinjaStringLiteral(prefix) + " ~ " + expr
			}
			return expr
		}

		if len(attrDeclared) > 0 && topic != "" {
			sb.WriteString("  - service: mqtt.publish\n")
			sb.WriteString("    data:\n")
			sb.WriteString("      topic: " + topic + "\n")
			sb.WriteString("      payload: \"{{ {")
			for i, declared := range attrDeclared {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString("'" + attrLeafByDeclared[declared] + "': " + capabilityExpr(declared) + " | float(0)")
			}
			sb.WriteString("} | tojson }}\"\n")
		}

		if len(deviceInfoNames) > 0 && deviceInfoTopic != "" {
			sb.WriteString("  - service: mqtt.publish\n")
			sb.WriteString("    data:\n")
			sb.WriteString("      topic: " + deviceInfoTopic + "\n")
			sb.WriteString("      payload: \"{{ {")
			for _, name := range deviceInfoNames {
				sb.WriteString("'" + name + "': " + capabilityExpr(name) + ", ")
			}
			if brokerHostName != "" {
				sb.WriteString("'via_device': '" + brokerHostName + "', ")
			}
			sb.WriteString("'name': '" + d.HostName + "'")
			sb.WriteString("} | tojson }}\"\n")
		}

		if err := writeAutomationFile(haOutputDir, "infrastructural", automationID, sb.String()); err != nil {
			return err
		}
	}
	return nil
}

// generateHostsIntegrationOutputs is the entry point registered in Physical_Storage.go's
// integrationBodyParsers for the "hosts" integration: parses the body then writes the ping
// hosts+secrets files, a cpu secrets file, the coordinator devices file, and the reporting
// automations for home_assistant-type devices. The ping/cpu secrets files are skipped, with a
// warning, when ctx.HasMQTTSecrets is false (no MQTT settings configured for this house yet).
func generateHostsIntegrationOutputs(bodyLines []string, ctx TPhysicalGenerationContext) error {
	devices, warnings := parseHostsIntegrationBody(bodyLines)
	for _, w := range warnings {
		fmt.Printf("[integration hosts] %s\n", w)
	}
	if len(devices) == 0 && len(ctx.ImportedDevices) == 0 {
		return nil
	}

	for _, w := range warnDuplicateDeviceIDs(devices) {
		fmt.Printf("[integration hosts] %s\n", w)
	}
	for _, d := range devices {
		if d.Local && !d.Cloud {
			fmt.Printf("[integration hosts] device %q: \"local\" without \"cloud\" has no effect; ignored\n", d.DeviceID)
		}
	}

	if err := generatePingHostsFile(ctx.OutputRoot, devices); err != nil {
		return err
	}
	// The coordinator's devices.yaml additionally carries every "integration import"-declared
	// device (ctx.ImportedDevices, pre-collected in generatePhysicalIntegrationOutputs) --
	// discovery/materialization is identical to a genuinely local device once its data starts
	// flowing (mqtt_relay.go bridges it in, coordinator side), so it belongs in the same file.
	// generatePingHostsFile/generateReportingAutomations deliberately still use "devices" alone,
	// not this combined list -- an imported device is never on this house's own LAN to ping, and
	// carries no local capabilities to poll (see integration_import_parser.go's own doc comment).
	coordinatorDevices := append(append([]THostDevice{}, devices...), ctx.ImportedDevices...)
	if err := generateCoordinatorDevicesFile(ctx.OutputRoot, coordinatorDevices, ctx.Admin, ctx.MQTTDiscoveryConceptualPrefix, ctx.Installation); err != nil {
		return err
	}
	if ctx.HasMQTTSecrets {
		// generatePhysicalIntegrationOutputs already warns once if MQTT settings are missing
		// (it resolves them before dispatching here) -- no need to repeat that here. ping/report
		// is a single centralized process pinging every declared host from one place
		// (generatePingHostsFile's one shared "ping/hosts" list), not something that runs
		// distributed per-host the way cpu/report does -- it only ever needs the one "main"
		// broker, so it keeps the plain unsuffixed "secrets" filename.
		if err := generateMQTTShellSecretsFile(ctx.OutputRoot, "ping", "secrets", ctx.MQTTSecrets, ""); err != nil {
			return err
		}
	}
	// cpu/report runs distributed, once per host machine, and decides which broker to report to
	// per invocation (an optional "$1" argument, defaulting to "default" -- see cpu/report and
	// its plist(s)). So every declared, leaf-usable broker profile gets its own secrets file here
	// unconditionally, not just "main" -- generateCPUBrokerSecretsFiles excludes anything marked
	// "coordinator_only true;".
	if err := generateCPUBrokerSecretsFiles(ctx.OutputRoot, ctx.MQTTBrokerProfiles, ctx.Installation); err != nil {
		return err
	}
	return generateReportingAutomations(ctx.HAOutputDir, devices, ctx.MQTTSecrets.Server)
}
