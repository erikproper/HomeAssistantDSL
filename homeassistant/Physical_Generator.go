/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: PhysicalGenerator
 *
 * Orchestrates the physical layer's generated output: collects the physical layer's
 * content (layers.go), parses its "integration ... with: ... end;" blocks
 * (Physical_Parser.go), and dispatches each block's body to the generator registered
 * for its name (Physical_Storage.go).
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

// generateCoordinatorSecretsFile writes <outputRoot>/coordinator/secrets.yaml: the "main" MQTT
// broker's connection secrets the coordinator needs to actually connect (under the "mqtt:" key,
// unchanged shape), plus one entry under "brokers:" for every OTHER profile declared
// "coordinator_only true;" (e.g. "cloud_coordinator") -- exactly the profiles meant for the
// coordinator's own secondary connections (mqtt_relay.go, coordinator side), never for a leaf
// host's report script (generateCPUBrokerSecretsFiles excludes those same profiles the other
// way around). profiles must contain "main" -- callers only invoke this once ctx.HasMQTTSecrets
// is true.
func generateCoordinatorSecretsFile(outputRoot string, profiles map[string]TMQTTBrokerSecrets) error {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("mqtt:\n")
	writeMQTTSecretsFields(&sb, "  ", profiles["main"])

	var brokerNames []string
	for name, secrets := range profiles {
		if name == "main" || !secrets.CoordinatorOnly {
			continue
		}
		brokerNames = append(brokerNames, name)
	}
	sort.Strings(brokerNames)
	if len(brokerNames) > 0 {
		sb.WriteString("brokers:\n")
		for _, name := range brokerNames {
			sb.WriteString("  " + name + ":\n")
			writeMQTTSecretsFields(&sb, "    ", profiles[name])
		}
	}

	dir := filepath.Join(outputRoot, "coordinator")
	return writeYAMLFile(filepath.Join(dir, "secrets.yaml"), sb.String())
}

// writeMQTTSecretsFields writes secrets' connection fields as indent-prefixed YAML lines
// (server/port/login/password/tls), shared by generateCoordinatorSecretsFile's "mqtt:" and
// "brokers:" entries.
func writeMQTTSecretsFields(sb *strings.Builder, indent string, secrets TMQTTBrokerSecrets) {
	sb.WriteString(indent + "server: \"" + secrets.Server + "\"\n")
	sb.WriteString(indent + "port: \"" + secrets.Port + "\"\n")
	sb.WriteString(indent + "login: \"" + secrets.Login + "\"\n")
	sb.WriteString(indent + "password: \"" + secrets.Password + "\"\n")
	tls := "false"
	if secrets.TLS {
		tls = "true"
	}
	sb.WriteString(indent + "tls: " + tls + "\n")
}

// generateHomeAssistantInstancesFile writes <outputRoot>/coordinator/home_assistant_instances.yaml:
// the flat list of every declared "home_assistant <qualifier>: <name>;" instance -- the
// coordinator's own entity_catalogue.go reads this to know which instances' "report entities
// detailed" bootstrap automation to subscribe/request from, independent of whether any
// "home_assistant" bridge *device* has been declared for that instance yet (homeassistant_bridge.yaml
// alone can't answer that -- it's keyed by device, and an instance being onboarded, like
// protocols-server-2 for a first Netatmo device, may have no devices declared at all yet).
func generateHomeAssistantInstancesFile(outputRoot string, instances map[string]THomeAssistantInstance) error {
	names := make([]string, 0, len(instances))
	for qualifier := range instances {
		names = append(names, qualifier)
	}
	sort.Strings(names)

	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("instances:\n")
	for _, name := range names {
		sb.WriteString("  - " + name + "\n")
	}
	dir := filepath.Join(outputRoot, "coordinator")
	return writeYAMLFile(filepath.Join(dir, "home_assistant_instances.yaml"), sb.String())
}

// generatePhysicalIntegrationOutputs resolves the main MQTT broker's secrets once
// (resolveMQTTBrokerSecrets, defined.go) and writes the coordinator's secrets.yaml from them
// (skipped, with a warning, when the house has no MQTT settings configured yet -- e.g. Vienna
// today, not every house has adopted MQTT). It then collects the physical layer's content (see
// layers.go; today just Physical.def, merged across any "physical layer with: ... end;" chunks
// it contains), parses its "integration ... with: ... end;" blocks generically, and dispatches
// each block's body -- along with the same resolved secrets, bundled into a
// TPhysicalGenerationContext (Physical_Storage.go) so integrations needing MQTT credentials
// (e.g. "hosts", for ping/cpu/secrets) don't each re-resolve Settings.def themselves -- to the
// generator registered for its name. An unrecognised integration name is reported and skipped
// rather than treated as an error, so the physical layer can keep growing ahead of the
// generator's support for it.
func generatePhysicalIntegrationOutputs(definitionDir, outputRoot, haOutputDir string, admin *TAdministrationState) error {
	mqttBrokerProfiles, mqttProfileWarnings := resolveMQTTBrokerProfiles(definitionDir)
	for _, w := range mqttProfileWarnings {
		fmt.Printf("[physical] %s\n", w)
	}
	mqttSecrets, hasMQTTSecrets := resolveMQTTBrokerSecrets(definitionDir)
	if !hasMQTTSecrets {
		fmt.Printf("[physical] no MQTT broker settings found (${main_mqtt_server}/${main_mqtt_login}/${main_mqtt_password}/${main_mqtt_port} in Settings.def); skipping coordinator/secrets.yaml and ping/cpu secrets\n")
	} else if err := generateCoordinatorSecretsFile(outputRoot, mqttBrokerProfiles); err != nil {
		return err
	}

	ctx := TPhysicalGenerationContext{
		OutputRoot:                    outputRoot,
		HAOutputDir:                   haOutputDir,
		Admin:                         admin,
		MQTTSecrets:                   mqttSecrets,
		HasMQTTSecrets:                hasMQTTSecrets,
		MQTTBrokerProfiles:            mqttBrokerProfiles,
		MQTTDiscoveryPhysicalPrefix:   resolveMQTTDiscoveryPhysicalPrefix(definitionDir),
		MQTTDiscoveryConceptualPrefix: resolveMQTTDiscoveryConceptualPrefix(definitionDir),
		Installation:                  resolveInstallationName(definitionDir),
	}

	physicalContent, mergedLineNos, layerWarnings := collectLayerContent(definitionDir, []string{"Physical.def"}, LayerPhysical)
	for _, w := range layerWarnings {
		fmt.Printf("[physical] %s\n", w)
	}
	if strings.TrimSpace(physicalContent) == "" {
		return nil
	}

	if err := generateDiscoveryCleanupFile(outputRoot, collectMQTTDiscoveryCleanTopics(physicalContent)); err != nil {
		return err
	}

	instances := collectHomeAssistantInstances(physicalContent)
	if err := generateHomeAssistantInstancesFile(outputRoot, instances); err != nil {
		return err
	}

	hassBridgeDevicesByID, hassBridgeWarnings := collectHassBridgeDevicesByID(definitionDir)
	for _, w := range hassBridgeWarnings {
		fmt.Printf("[physical] %s\n", w)
	}
	// PROJECT.md 1.2b: "export" implies "all entities used," even for a device Spaces.def never
	// positions at all -- must run after Spaces.def parsing (admin is already fully populated by
	// the time generatePhysicalIntegrationOutputs runs) and before generateHassBridgeFile/
	// generateInstanceAutomationTrees, both of which gate on DeviceConceptualLinks.
	for _, w := range registerExportedHassBridgeDevices(admin, hassBridgeDevicesByID) {
		fmt.Printf("[physical] %s\n", w)
	}
	if err := generateHassBridgeFile(outputRoot, hassBridgeDevicesByID, admin); err != nil {
		return err
	}
	// PROJECT.md 1.1: the only entity-existence status that blocks generation is a *confirmed*
	// known-not-to-exist verdict from the coordinator -- not-known-to-exist (the coordinator
	// hasn't gotten to it yet) and known-to-exist both generate optimistically, same as before
	// this check existed. Skipped entirely when the house has no MQTT settings (Vienna today).
	if hasMQTTSecrets {
		if err := checkKnownNotToExistErrors(definitionDir, hassBridgeDevicesByID, ctx); err != nil {
			return err
		}
		// PROJECT.md 1.8: kind-2's own passive counterpart -- same rule, a confirmed
		// known-not-to-exist source leaf blocks generation, not-known-to-exist/known-to-exist both
		// generate optimistically.
		if err := checkDiscoveryKnownNotToExistErrors(definitionDir, admin.DiscoveryEntityLinks, ctx); err != nil {
			return err
		}
	}
	if err := generateInstanceAutomationTrees(haOutputDir, instances, hassBridgeDevicesByID, admin); err != nil {
		return err
	}
	if err := generateEntityCatalogueSuggestions(definitionDir, outputRoot, instances, hassBridgeDevicesByID, ctx); err != nil {
		return err
	}
	// PROJECT.md 1.8: kind-2's own suggestion report, mirroring the kind-3 one just above --
	// collected independently here, same as hassBridgeDevicesByID above, rather than threaded
	// through from generator.go's own earlier parse.
	discoveryGatewaysByID, discoveryGatewayWarnings := collectDiscoveryGatewaysByID(definitionDir)
	for _, w := range discoveryGatewayWarnings {
		fmt.Printf("[physical] %s\n", w)
	}
	if err := generateDiscoverySuggestions(definitionDir, outputRoot, discoveryGatewaysByID, admin.DiscoveryEntityLinks, ctx); err != nil {
		return err
	}

	blocks, warnings := parseIntegrationBlocks(physicalContent, mergedLineNos)
	for _, w := range warnings {
		fmt.Printf("[physical] %s\n", w)
	}

	importedDevices, importWarnings := collectImportedDevices(blocks)
	for _, w := range importWarnings {
		fmt.Printf("[physical] %s\n", w)
	}
	ctx.ImportedDevices = importedDevices
	if err := generateImportedDeviceFile(outputRoot, importedDevices, ctx.Admin); err != nil {
		return err
	}
	// Baseline devices.yaml write, unconditional: a house with no "integration hosts" block at
	// all (e.g. Vienna today, PROJECT.md 1.2c/1.2d -- coordinator/cloud broker stood up before any
	// local hosts devices exist) still needs devices.yaml to carry Installation/
	// MQTTDiscoveryConceptualPrefix, which every other coordinator subscription
	// (subscribeImportedDevices included) reads from there. If an "integration hosts" block DOES
	// exist, generateHostsIntegrationOutputs (below, per-block dispatch) overwrites this with the
	// fuller hosts device list -- this call only ever supplies the fallback baseline for a house
	// that has none. Real bug found live 2026-08-30: without this, Vienna's coordinator logged "a
	// cloud broker is configured but devices.yaml has no installation set" and refused to
	// publish/clean up cloud-side topics at all. Cross-house imports no longer belong in
	// devices.yaml at all (2026-09-05 unification -- they resolve entirely through
	// coordinator/imported.yaml and discoveryimport.go instead), so this baseline call always
	// passes an empty device list now; only a genuinely native "hosts" block ever populates
	// devices.yaml's own device entries.
	if err := generateCoordinatorDevicesFile(ctx.OutputRoot, nil, ctx.Admin, ctx.MQTTDiscoveryConceptualPrefix, ctx.Installation); err != nil {
		return err
	}

	for _, block := range blocks {
		handler, known := integrationBodyParsers[block.Name]
		if !known {
			fmt.Printf("[physical] integration %q (line %d): no parser registered; skipping\n", block.Name, block.StartLine)
			continue
		}
		if err := handler(block.BodyLines, ctx); err != nil {
			return err
		}
	}
	return nil
}
