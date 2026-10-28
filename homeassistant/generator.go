/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Generator
 *
 * Generates Home Assistant YAML files from the parsed administration state.
 * Phase 1 covers administratively-derived entities: aggregate template lights,
 * template switches, binary sensor groups (heating leakage, per-subdomain door/window/
 * motion/water, windoor), sensor subdomain mean groups (temperature/humidity/co2/noise/
 * pressure/illuminance), Zigbee light groups, and entity customization files.
 * Entity body properties (condition, cli_switch, adjustment, input_number, scripts,
 * automations) are Phase 2.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 26.03.2026
 *
 */

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const generatorHeader = "### This file was automatically generated. Don't edit it!\n"

// --- entry points ---

// runGenerationFromDefFile is the local-mode entry point:
//
//	homeassistant generate Definitions/Main.def
//
// Derives definitionDir from the .def path, shared definitions via ../Shared/Definitions,
// and writes YAML to hass/ and list.* files to the current working directory.
func runGenerationFromDefFile(cwd, defPath string) error {
	fullDefPath := filepath.Join(cwd, defPath)
	definitionDir := filepath.Dir(fullDefPath)
	sharedDefinitionDir := filepath.Clean(filepath.Join(definitionDir, "..", "..", "Shared", "Definitions"))
	// Both houses have adopted "home_assistant main: <name>;" (PROJECT.md item 4/5), so a failure
	// to resolve it today means Physical.def was empty/unreadable or the directive didn't match at
	// the moment this ran (e.g. a concurrent editor save) -- not the "house hasn't adopted
	// Physical.def yet" steady state resolveMainIncarnationName's own doc comment describes.
	// Silently falling back to writing a full generation into bare hass/ instead of
	// hass/<incarnation>/ produced a real, repeatedly-recurring stale-directory confusion (found
	// live 2026-09-16) -- refuse to write anywhere rather than degrade to that wrong location
	// (explicit user decision, 2026-09-16: abort over warn-and-fall-back).
	incarnation := resolveMainIncarnationName(definitionDir)
	if incarnation == "" {
		return fmt.Errorf("no \"home_assistant main: <name>;\" found (or Physical.def unreadable) -- refusing to write to bare hass/")
	}
	outputDir := filepath.Join(cwd, "hass", incarnation)
	listOutputDir := cwd
	label := filepath.Base(cwd)

	admin, err := generateFromPaths(definitionDir, sharedDefinitionDir, outputDir, listOutputDir, label)
	if err != nil {
		return err
	}
	// generatePhysicalIntegrationOutputs runs after generateFromPaths (and therefore after
	// its runPostGenerationChecks call, presence.go), so any HA YAML it writes -- e.g. the
	// "hosts" integration's reporting automations -- lands too late for check [1]
	// (referential integrity, checkEntityReferences) to see it: those files exist on disk
	// but outside the scan that check performed. Currently harmless (the only entities a
	// reporting automation references are already covered by check [2] via
	// homeAssistantCapabilityEntityIDs), but worth revisiting once secondary HA instances
	// exist (see the commented-out "home_assistant: protocols-server-2
	// ${protocols_server_2_home_assistant_url};" line in Physical.def) and this path grows
	// more physical-layer-generated HA YAML.
	if err := generatePhysicalIntegrationOutputs(definitionDir, cwd, outputDir, admin); err != nil {
		return err
	}
	// Last: every YAML-writing step, the physical layer's included, has recorded its entities.
	if err := generateRegistryManifest(outputDir, admin); err != nil {
		return err
	}
	fmt.Printf("generated %s\n", label)
	return nil
}

// parseAdministrationFromPaths runs the generation pipeline's read-and-interpret phase only:
// settings, "hosts"/"discovery" device collection, and Spaces.def entity parsing -- everything
// generateFromPaths needs before it starts writing any YAML or touching the network. Split out
// so a caller that only needs the resulting entity registry (e.g. runWouldDefineCheck,
// would_define.go) can get it without the side effects (output directory wiped/rewritten, live
// presence-check HTTP calls) the rest of generateFromPaths performs.
// writeBack, when true, allows autoAssignDiscoveryStableIDs (PROJECT.md item 7) to rewrite
// Physical.def with freshly-assigned stable ids -- true only from the real generation pipeline
// (generateFromPaths); false from every read-only diagnostic caller (would_define.go,
// derived_migration_helper.go), both explicitly documented as never editing any file.
func parseAdministrationFromPaths(definitionDir, sharedDefinitionDir, label string, writeBack bool) (*TAdministrationState, error) {
	fmt.Printf("%s: reading configuration...\n", label)

	sharedSettingsBytes, _ := os.ReadFile(filepath.Join(sharedDefinitionDir, "Settings.def"))
	localSettingsBytes, _ := os.ReadFile(filepath.Join(definitionDir, "Settings.def"))
	combinedSettings := string(sharedSettingsBytes) + "\n" + string(localSettingsBytes)
	settings := parseDefinitionAssignments(combinedSettings)

	fmt.Printf("%s: interpreting entities...\n", label)

	hostDevicesByID, hostDeviceWarnings := collectHostsDevicesByID(definitionDir)
	for _, w := range hostDeviceWarnings {
		fmt.Printf("[physical] %s\n", w)
	}

	discoveryGatewaysByID, discoveryGatewayWarnings := collectDiscoveryGatewaysByID(definitionDir)
	for _, w := range discoveryGatewayWarnings {
		fmt.Printf("[physical] %s\n", w)
	}

	hassBridgeDevicesByID, hassBridgeWarnings := collectHassBridgeDevicesByID(definitionDir)
	for _, w := range hassBridgeWarnings {
		fmt.Printf("[physical] %s\n", w)
	}

	importedDevicesByID, importedDeviceWarnings := collectImportedDevicesByID(definitionDir)
	for _, w := range importedDeviceWarnings {
		fmt.Printf("[physical] %s\n", w)
	}

	logicalByID, logicalWarnings := collectLogicalDevicesByID(definitionDir, settings)
	for _, w := range logicalWarnings {
		fmt.Printf("[logical] %s\n", w)
	}

	for _, report := range autoAssignCommandlineFrozenUniqueIDs(definitionDir) {
		fmt.Printf("[physical] %s\n", report)
	}
	commandlineDevicesByID, commandlineDeviceWarnings := collectCommandlineDevicesByID(definitionDir)
	for _, w := range commandlineDeviceWarnings {
		fmt.Printf("[physical] %s\n", w)
	}

	localDevicesByID, localDeviceWarnings := collectLocalDevicesByID(definitionDir)
	for _, w := range localDeviceWarnings {
		fmt.Printf("[physical] %s\n", w)
	}

	for _, w := range validateNoConflictingDeviceNames(hostDevicesByID, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, commandlineDevicesByID, localDevicesByID, logicalByID) {
		fmt.Printf("[physical] %s\n", w)
	}

	for _, w := range validateNodeCapabilityRequired(hassBridgeDevicesByID, importedDevicesByID) {
		fmt.Printf("[physical] %s\n", w)
	}

	for _, w := range applyEnabledBy(logicalByID, localDevicesByID, discoveryGatewaysByID, hassBridgeDevicesByID) {
		fmt.Printf("[logical] %s\n", w)
	}

	for _, w := range validateLogicalNodeCapabilityRequired(logicalByID, hostDevicesByID, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, commandlineDevicesByID, localDevicesByID) {
		fmt.Printf("[logical] %s\n", w)
	}

	for _, w := range validateHassBridgeAvailabilitySources(hassBridgeDevicesByID) {
		fmt.Printf("[physical] %s\n", w)
	}

	for _, w := range validateDerivedCapabilities(hassBridgeDevicesByID, importedDevicesByID, discoveryGatewaysByID) {
		fmt.Printf("[physical] %s\n", w)
	}

	for _, w := range validateBatteryLevelAlertBiconditional(hostDevicesByID, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, commandlineDevicesByID, localDevicesByID) {
		fmt.Printf("[physical] %s\n", w)
	}

	for _, w := range validateLogicalBatteryLevelAlertBiconditional(logicalByID) {
		fmt.Printf("[logical] %s\n", w)
	}

	for _, w := range validatePowerImpliesConsumes(hassBridgeDevicesByID, importedDevicesByID, discoveryGatewaysByID) {
		fmt.Printf("[physical] %s\n", w)
	}

	for _, w := range validateNoOverlappingCapabilitySuffixes(discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, commandlineDevicesByID, localDevicesByID, logicalByID) {
		fmt.Printf("[physical] %s\n", w)
	}

	capabilityDefaults, capabilityDefaultsWarnings := collectCapabilityDefaults(definitionDir, sharedDefinitionDir)
	for _, w := range capabilityDefaultsWarnings {
		fmt.Printf("[physical] %s\n", w)
	}

	entitiesPath := filepath.Join(definitionDir, "Conceptual.def")
	entitiesContent, entitiesLineNos, layerWarnings := collectLayerContent(definitionDir, []string{"Conceptual.def"}, LayerConceptual)
	for _, w := range layerWarnings {
		fmt.Printf("[conceptual] %s\n", w)
	}
	if strings.TrimSpace(entitiesContent) == "" {
		return nil, fmt.Errorf("error reading entities: no %q layer content found in %s", LayerConceptual, entitiesPath)
	}

	var report strings.Builder
	parseResult, err := ParseEntitiesAndFillAdministration(
		strings.Split(entitiesContent, "\n"), entitiesLineNos, entitiesPath, settings, &report, hostDevicesByID, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, commandlineDevicesByID, localDevicesByID, logicalByID, capabilityDefaults)
	if err != nil {
		return nil, err
	}
	admin := parseResult.Administration
	admin.RawGatewayIDs = map[string]bool{}
	for id, gateway := range discoveryGatewaysByID {
		if gateway.Raw {
			admin.RawGatewayIDs[id] = true
		}
	}
	for id := range logicalByID {
		topics, w := resolveDependencyAvailabilityTopics(id, logicalByID, hostDevicesByID)
		for _, warning := range w {
			fmt.Printf("[logical] %s\n", warning)
		}
		if len(topics) > 0 {
			admin.DependsOnAvailabilityTopics[id] = topics
		}
		// A "hosts" device this one depends on, or refers to in a condition (e.g. its node's
		// "physical appliance.sonos_bathroom and node.sonos_bathroom"): its node is merged here.
		nodeRefs := append([]string{}, logicalByID[id].DependsOn...)
		for _, capability := range logicalByID[id].Capabilities {
			nodeRefs = append(nodeRefs, conditionExprRefs(capability.DerivedCondition)...)
		}
		for _, depID := range nodeRefs {
			if _, isHost := hostDevicesByID[depID]; isHost && !slices.Contains(admin.NodeDependents[depID], id) {
				if admin.NodeDependents == nil {
					admin.NodeDependents = map[string][]string{}
				}
				admin.NodeDependents[depID] = append(admin.NodeDependents[depID], id)
			}
		}
	}
	dropShadowedDerivedAggregates(admin)
	for _, w := range validateNoDuplicateFinalEntityIDs(admin) {
		fmt.Printf("[conceptual] %s\n", w)
	}

	for _, w := range applyColocatedPositioning(admin, logicalByID, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, localDevicesByID, entitiesPath) {
		fmt.Printf("[conceptual] %s\n", w)
	}

	triggerLogicalOwnNodeCapabilities(admin, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, localDevicesByID, logicalByID, entitiesPath)

	// Must run AFTER triggerLogicalOwnNodeCapabilities, not before: a device whose own "node" is
	// still unresolved at THIS point might still get a distinguishing leaf from it moments later
	// (deriveDeviceOwnLeafFromSiblings), which would resolve what looked like a real collision
	// here into two perfectly distinct positions. Real false positive found live: Junglinster's
	// appliance.picture_frame/buttons.house_living_room_living_table_tap, both positioned at the
	// same bare space with no "as" leaf of their own, warned about as a share-position collision
	// even though triggerLogicalOwnNodeCapabilities goes on to give them "picture_frame" and
	// "table/tap" respectively, with zero actual collision in the generated output.
	for _, w := range findSharedDevicePositions(admin, discoveryGatewaysByID) {
		fmt.Printf("[conceptual] %s\n", w)
	}

	fixUpDisplayNameFromNode(admin, discoveryGatewaysByID)

	// A hard abort only for the real generation run (writeBack=true, generateFromPaths's own
	// caller) -- every other caller of this shared parse (would_define.go, bootstrap_stable_ids.go,
	// flatten_conceptual_positions.go, backfill_explicit_node_positioning.go, writeBack=false) is
	// read-only analysis/migration tooling that must still be able to PARSE a currently-imperfect
	// Conceptual.def in order to go FIX it -- real case found live: backfill_explicit_node_
	// positioning.go's own parseAdministrationFromPaths call failed outright on the very devices it
	// was about to backfill, since at that point in its own run they genuinely have no node yet.
	if w := validateDevicesHaveFriendlyNames(admin, discoveryGatewaysByID); len(w) > 0 {
		if writeBack {
			return nil, fmt.Errorf("%s", strings.Join(w, "\n"))
		}
		for _, warning := range w {
			fmt.Printf("[conceptual] %s\n", warning)
		}
	}

	loadFrozenUniqueIDs(admin, definitionDir, logicalByID)
	for _, w := range registerEnablerNodes(admin, logicalByID, discoveryGatewaysByID, hassBridgeDevicesByID, hostDevicesByID, localDevicesByID) {
		fmt.Printf("[logical] %s\n", w)
	}
	for _, w := range registerFirmwareUpdates(admin, definitionDir, discoveryGatewaysByID) {
		fmt.Printf("[physical] %s\n", w)
	}
	attributeAbsorbedEntities(admin, logicalByID)
	for id := range hassBridgeDevicesByID {
		admin.BridgedDeviceIDs = append(admin.BridgedDeviceIDs, id)
	}
	admin.NativeCapabilities = map[string][]string{}
	admin.NativeSeeds = map[string]string{}
	for id, device := range localDevicesByID {
		for label, capability := range device.Capabilities {
			// A capability the generator builds itself (an "is available" node, a derived value, a
			// media switch wrapping a player) is generator-owned YAML, not a native entity.
			if capability.AvailabilityOf == "" && capability.DerivedFromCapability == "" && capability.DerivedViaTemplate == "" && capability.MediaPlayerEntity == "" {
				admin.NativeCapabilities[id] = append(admin.NativeCapabilities[id], label)
				if capability.NativeEntity != "" {
					admin.NativeSeeds[id+"!"+label] = capability.NativeEntity
				}
			}
		}
	}
	for id, device := range importedDevicesByID {
		admin.ImportedDeviceIDs = append(admin.ImportedDeviceIDs, id)
		for label, capability := range device.Capabilities {
			if capability.AssignedID != "" {
				admin.FrozenUniqueIDs["import:"+id+"!"+label] = capability.AssignedID
			}
		}
	}

	if writeBack {
		for _, w := range autoAssignDiscoveryStableIDs(definitionDir, discoveryGatewaysByID, admin) {
			fmt.Printf("[physical] %s\n", w)
		}
		for _, w := range autoAssignFrozenUniqueIDs(definitionDir, admin) {
			fmt.Printf("[physical] %s\n", w)
		}
	}

	return admin, nil
}

// generatedUniqueID is the unique_id every generated entity's YAML gets (PROJECT.md item 48): its
// capability's frozen universal ID when it has one, otherwise its own entity_id. A package-level
// lookup rather than a parameter, set once per house by generateFromPaths, so the many small
// buildXxxYAML helpers below (which only ever receive an entity_id) don't each need the whole
// administration threaded through.
var generatedUniqueID = func(entityID string) string { return entityID }

// generateFromPaths contains the full generation pipeline given resolved directory paths.
func generateFromPaths(definitionDir, sharedDefinitionDir, outputDir, listOutputDir, label string) (*TAdministrationState, error) {
	// Before parsing: the physical layer's writers (devices.yaml, the commandline devices) record
	// their entities for the registry manifest as they go.
	coordinatorPublishedEntities = nil
	spaceAggregateMembers = nil
	positionedEntityIDs = nil
	admin, err := parseAdministrationFromPaths(definitionDir, sharedDefinitionDir, label, true)
	if err != nil {
		return nil, err
	}
	generatedUniqueID = recordingUniqueIDFor(admin)

	fmt.Printf("%s: generating YAML...\n", label)

	if err := os.RemoveAll(outputDir); err != nil {
		return nil, fmt.Errorf("cannot clean output directory: %w", err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create output directory: %w", err)
	}

	steps := []struct {
		name string
		fn   func(string, *TAdministrationState) error
	}{
		{"configuration", generateConfigurationFile},
		{"integration files", generateIntegrationFiles},
		{"physical zigbee groups", injectPhysicalZigbeeGroups},
		{"template lights", generateTemplateLights},
		{"template switches", generateTemplateSwitches},
		{"template binary sensors", generateTemplateBinarySensors},
		{"binary sensor groups", generateBinarySensorGroups},
		{"binary sensor subdomain groups", generateBinarySensorSubdomainGroups},
		{"sensor subdomain groups", generateSensorSubdomainGroups},
		{"light groups", generateLightGroups},
		{"switched device entities", generateSwitchedDeviceEntities},
		{"has_state entities", generateHasStateEntities},
		{"timer definitions", generateTimerDefinitions},
		{"customization", generateCustomizationFiles},
		{"cli switches", generateCliSwitches},
		{"input numbers", generateInputNumbers},
		{"input booleans", generateInputBooleans},
		{"input datetimes", generateInputDatetimes},
		{"template sensors", generateTemplateSensors},
		{"condition entities", generateConditionEntities},
		{"infrastructural binary sensor groups", generateInfrastructuralGroups},
		{"media switches", generateMediaSwitches},
		{"heating scripts", generateHeatingScripts},
		{"cover away entities", generateCoverAwayEntities},
		{"cover away scripts", generateCoverAwayScripts},
		{"derived binary sensors", generateDerivedBinarySensors},
		{"named trigger events", generateNamedTriggerEvents},
		{"automations", generateAutomations},
	}
	for _, step := range steps {
		if err := step.fn(outputDir, admin); err != nil {
			return nil, fmt.Errorf("%s: %w", step.name, err)
		}
	}

	listsContent, _, listLayerWarnings := collectLayerContent(definitionDir, []string{"External.def"}, LayerExternal)
	for _, w := range listLayerWarnings {
		fmt.Printf("[external] %s\n", w)
	}
	if strings.TrimSpace(listsContent) != "" {
		if err := generateListFiles(listOutputDir, []byte(listsContent), admin); err != nil {
			return nil, fmt.Errorf("list files: %w", err)
		}
	}
	warnOccupiedSpaceAggregates()
	noteNodesAtSpaceNames(admin)

	runPostGenerationChecks(definitionDir, outputDir, label, admin)

	return admin, nil
}

// --- configuration.yaml ---

// configurationYAMLBody is built dynamically now (generateConfigurationFile), not a fixed
// constant -- see integrationDefs' own doc comment for why. Kept only as the fixed preamble
// (default_config + homeassistant:/customize) every domain line gets appended after.
const configurationYAMLPreamble = "default_config:\n\nhomeassistant:\n  customize: !include_dir_merge_named customization\n"

// generateConfigurationFile writes configuration.yaml with one plain top-level key per
// integrationDefs entry (each an "!include integrations/<file>", never a "homeassistant:
// packages:" wrapper) -- see integrationDefs' own doc comment for the real incident this fixes.
func generateConfigurationFile(outputDir string, _ *TAdministrationState) error {
	var sb strings.Builder
	sb.WriteString(configurationYAMLPreamble)
	sb.WriteString("\n")
	for _, def := range integrationDefs {
		sb.WriteString(def.domain + ": !include integrations/" + def.file + "\n")
	}
	return writeYAMLFile(filepath.Join(outputDir, "configuration.yaml"), generatorHeader+sb.String())
}

// --- integration aggregator files ---

// integrationDefs maps each domain's own generated skeleton file to its !include directive
// content (the file's own body -- no "<domain>: " prefix; the domain key itself now lives in
// configuration.yaml, which !includes this file directly as that key's value).
//
// Real incident, 2026-09-22/23 (Junglinster, environment.weather's pressure sensor): these files
// used to be self-contained "<domain>: <content>" package fragments, referenced from
// configuration.yaml via "homeassistant: packages: !include_dir_named integrations" -- but Home
// Assistant has a long-standing, upstream wontfix limitation that packages are NEVER reprocessed
// by any reload service (not "<domain>.reload", not "homeassistant.reload_all", not even
// "homeassistant.reload_core_config" -- github.com/home-assistant/core#12069, closed wontfix).
// Since this project's meta-reload mechanism (PublishMetaReload -> reload_all,
// remote_instance_meta_control.go) exists specifically so a deploy doesn't need a full restart,
// wrapping every domain in a package silently defeated that for the WHOLE generated config, not
// just this one device -- confirmed live: a brand-new template sensor file, correctly deployed,
// stayed absent from the running instance until a manual reload. No DSL feature or generated
// output anywhere ever exercised a package's actual point (multiple domains merged from one
// file) -- every entry below was always exactly one file, one domain -- so there was nothing to
// lose by splitting each into a plain top-level "<domain>: !include ..." key instead, which DOES
// support normal reload semantics. See PROJECT.md item 20 for the full incident writeup.
var integrationDefs = []struct{ file, domain, content string }{
	{"automation.yaml", "automation", "!include_dir_merge_list ../automation"},
	{"binary_sensor.yaml", "binary_sensor", "!include_dir_list ../entities/binary_sensor"},
	{"input_boolean.yaml", "input_boolean", "!include_dir_merge_named ../entities/input_boolean"},
	{"input_datetime.yaml", "input_datetime", "!include_dir_merge_named ../entities/input_datetime"},
	{"input_number.yaml", "input_number", "!include_dir_merge_named ../entities/input_number"},
	{"input_select.yaml", "input_select", "!include_dir_merge_named ../entities/input_select"},
	{"light.yaml", "light", "!include_dir_list ../entities/light"},
	{"mqtt.yaml", "mqtt", "!include_dir_merge_list ../entities/mqtt"},
	{"scene.yaml", "scene", "!include_dir_merge_list ../scene"},
	{"script.yaml", "script", "!include_dir_merge_named ../script"},
	{"sensor.yaml", "sensor", "!include_dir_list ../entities/sensor"},
	{"switch.yaml", "switch", "!include_dir_list ../entities/switch"},
	{"template.yaml", "template", "!include_dir_list ../entities/template"},
	{"timer.yaml", "timer", "!include_dir_merge_named ../entities/timer"},
	{"tts.yaml", "tts", "- platform: google_translate"},
}

func generateIntegrationFiles(outputDir string, _ *TAdministrationState) error {
	intDir := filepath.Join(outputDir, "integrations")
	if err := os.MkdirAll(intDir, 0755); err != nil {
		return err
	}
	for _, def := range integrationDefs {
		body := generatorHeader + def.content + "\n"
		if err := writeYAMLFile(filepath.Join(intDir, def.file), body); err != nil {
			return err
		}
	}
	return nil
}

// --- physical zigbee group injection ---

// injectPhysicalZigbeeGroups adds physical light group entities and physical space switches
// to the SpaceOffByName of the corresponding social parent space.  This mirrors old-generator
// behaviour where no_collect physical sub-light spaces contributed their group light and
// space switch to parent social space templates.
func injectPhysicalZigbeeGroups(_ string, admin *TAdministrationState) error {
	for _, spaceName := range admin.SpaceOrder {
		records := admin.EntityRecordsBySpace[spaceName]
		if !isNoCollectLightSpace(spaceName, records) {
			continue
		}
		// Populate the physical space's own SpaceOffByName with its sub-lights so that
		// generateTemplateSwitches will produce a physical space switch.
		for _, rec := range records {
			if rec.Identity.Domain == "light" {
				admin.SpaceOffByName[spaceName] = addIfAbsent(admin.SpaceOffByName[spaceName], rec.Name)
			}
		}
		// Find the social parent: strip "physical/" prefix, remove the last path component,
		// then prepend "social/".
		if !strings.HasPrefix(spaceName, "physical/") {
			continue
		}
		noSphere := spaceName[len("physical/"):]
		slashIdx := strings.LastIndex(noSphere, "/")
		if slashIdx < 0 {
			continue
		}
		socialParent := "social/" + noSphere[:slashIdx]
		if _, exists := admin.EntityRecordsBySpace[socialParent]; !exists {
			continue
		}
		physSwitch := "switch." + spaceName + "/space"
		admin.SpaceOffByName[socialParent] = addIfAbsent(admin.SpaceOffByName[socialParent], physSwitch)
		// Include the physical group light in the social parent's off list only when the
		// parent has no explicit "space off:" directive.  When explicit off is set, lights
		// are excluded from the space switch (onItems = []) so adding a physical group
		// light there would be inconsistent with the social lights not being present.
		if !admin.SpaceHasExplicitOff[socialParent] {
			physGroup := "light." + spaceName
			admin.SpaceOffByName[socialParent] = addIfAbsent(admin.SpaceOffByName[socialParent], physGroup)
		}
	}
	return nil
}

// --- template light entities ---

// generateTemplateLights writes entities/template/light/<sphere>/<id>.yaml for every
// space that has a non-empty lights-on list (SpaceOnByName).
// After all named spaces are processed, sphere-level aggregate entities are generated
// from the root space's accumulated lights (e.g. light.social from apartment + terrace).
func generateTemplateLights(outputDir string, admin *TAdministrationState) error {
	for _, spaceName := range admin.SpaceOrder {
		if spaceName == "root" {
			continue
		}
		onItems := admin.SpaceOnByName[spaceName]
		if len(onItems) == 0 {
			continue
		}

		entityName := "light." + spaceName
		entityID := toHomeAssistantEntityID(entityName)
		if entityID == "" {
			continue
		}

		identity := extractEntityIdentity(entityName)
		if identity.Sphere == "" {
			continue
		}

		// turn_on uses the explicit "light on:" directive items (SpaceOnExplicitByName) when
		// present; otherwise falls back to the full accumulated list.  Items are sorted
		// alphabetically to match the ordering produced by the old bash generator.
		turnOnItems := sortedCopy(admin.SpaceOnExplicitByName[spaceName])
		if len(turnOnItems) == 0 {
			turnOnItems = sortedCopy(onItems)
		}
		// turn_off: prefer an explicit "light off:" directive.  Otherwise compute the union
		// of SpaceOnByName (which includes child-propagated derived lights like light.<bed>)
		// and SpaceLightsByName (which includes directly declared lights that were excluded
		// from the explicit "light on:" directive, e.g. a mirror light).  This ensures that
		// both propagated aggregates and any extra directly-declared lights are turned off.
		offItems := admin.SpaceLightOffByName[spaceName]
		if len(offItems) == 0 {
			seen := map[string]bool{}
			for _, item := range append(append([]string{}, onItems...), admin.SpaceLightsByName[spaceName]...) {
				if !seen[item] {
					seen[item] = true
					offItems = append(offItems, item)
				}
			}
		}
		// Snapshot the base off list before adding Zigbee physical complements, so we
		// can decide below whether to emit a light group or a template light.
		baseOffItems := sortedCopy(offItems)

		// Also add the physical group light for any social light whose context path
		// corresponds to a no_collect physical light space (a Zigbee group).  The
		// physical group needs to be turned off explicitly for zigbee reliability.
		physSeen := map[string]bool{}
		for _, item := range offItems {
			physSeen[item] = true
		}
		for _, item := range onItems {
			itemID := extractEntityIdentity(item)
			if itemID.Sphere == "" {
				continue
			}
			physSpaceName := "physical/" + itemID.Path
			if physRecs, ok := admin.EntityRecordsBySpace[physSpaceName]; ok {
				if isNoCollectLightSpace(physSpaceName, physRecs) {
					physGroup := "light." + physSpaceName
					if !physSeen[physGroup] {
						physSeen[physGroup] = true
						offItems = append(offItems, physGroup)
					}
				}
			}
		}
		offItems = sortedCopy(offItems)

		displayName := entityName[len("light."):]
		customContent := buildCustomizationYAML(entityID, displayName, domainDefaultIcons["light"])
		customDir := filepath.Join(outputDir, "customization", "light", identity.Sphere)

		// A light group is used when the base turn-on set equals the base turn-off set
		// (symmetric on/off, no subset selection).  The group members are the full off list,
		// which includes any Zigbee physical complements needed for reliable group control.
		if !recordSpaceAggregate(admin, entityID, entityNamesToIDs(append(append([]string{}, turnOnItems...), offItems...))) {
			continue
		}
		if slices.Equal(sortedCopy(turnOnItems), baseOffItems) {
			content := buildLightGroupYAML(entityID, displayName, entityNamesToIDs(offItems))
			dir := filepath.Join(outputDir, "entities", "light", identity.Sphere)
			if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
				return err
			}
		} else {
			content := buildTemplateLightYAML(entityID, displayName, turnOnItems, offItems, admin.SpaceAvailabilityAll[spaceName])
			dir := filepath.Join(outputDir, "entities", "template", "light", identity.Sphere)
			if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
				return err
			}
		}
		if err := writeYAMLFile(filepath.Join(customDir, entityID+".yaml"), customContent); err != nil {
			return err
		}
	}

	// Generate sphere-level aggregate lights from root-propagated entries.
	// When top-level child spaces (e.g. social:apartment, social:terrace) each derive a
	// light entity, those entities are propagated into SpaceOnByName["root"].  Group them
	// by sphere to produce e.g. light.social from [light.social/apartment, light.social/terrace].
	sphereItems := map[string][]string{}
	for _, entry := range admin.SpaceOnByName["root"] {
		identity := extractEntityIdentity(entry)
		if identity.Sphere == "" {
			continue
		}
		sphereItems[identity.Sphere] = append(sphereItems[identity.Sphere], entry)
	}
	spheres := make([]string, 0, len(sphereItems))
	for sphere := range sphereItems {
		spheres = append(spheres, sphere)
	}
	sort.Strings(spheres)
	for _, sphere := range spheres {
		items := sortedCopy(sphereItems[sphere])
		entityID := "light." + sphere
		displayName := sphere
		if !recordSpaceAggregate(admin, entityID, entityNamesToIDs(items)) {
			continue
		}
		content := buildTemplateLightYAML(entityID, displayName, items, items, false)
		dir := filepath.Join(outputDir, "entities", "template", "light", sphere)
		if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
			return err
		}
		customContent := buildCustomizationYAML(entityID, displayName, domainDefaultIcons["light"])
		customDir := filepath.Join(outputDir, "customization", "light", sphere)
		if err := writeYAMLFile(filepath.Join(customDir, entityID+".yaml"), customContent); err != nil {
			return err
		}
	}

	return nil
}

func buildTemplateLightYAML(entityID, displayName string, turnOnItems, turnOffItems []string, availabilityAll bool) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("light:\n")
	sb.WriteString("- name: " + displayName + "\n")
	sb.WriteString("  unique_id: " + generatedUniqueID(entityID) + "\n")
	sb.WriteString("  default_entity_id: " + entityID + "\n")
	sb.WriteString("  icon: mdi:lightbulb-group\n")
	appendAvailability(&sb, aggregateAvailability(entityNamesToIDs(turnOffItems), availabilityAll))

	sb.WriteString("  turn_on:\n")
	for _, item := range turnOnItems {
		itemID := toHomeAssistantEntityID(item)
		sb.WriteString("  - entity_id:\n")
		sb.WriteString("    - " + itemID + "\n")
		sb.WriteString("    action: light.turn_on\n")
	}

	sb.WriteString("  turn_off:\n")
	for _, item := range turnOffItems {
		itemID := toHomeAssistantEntityID(item)
		action := domainTurnOffAction(item)
		sb.WriteString("  - entity_id:\n")
		sb.WriteString("    - " + itemID + "\n")
		sb.WriteString("    action: " + action + "\n")
	}

	appendStateBlock(&sb, turnOffItems)
	return sb.String()
}

// --- template switch entities ---

// generateTemplateSwitches writes entities/template/switch/<sphere>/<id>.yaml for every
// space that has a non-empty space-off list (SpaceOffByName).
func generateTemplateSwitches(outputDir string, admin *TAdministrationState) error {
	for _, spaceName := range admin.SpaceOrder {
		if spaceName == "root" {
			continue
		}
		offItems := admin.SpaceOffByName[spaceName]
		if len(offItems) == 0 {
			continue
		}

		entityName := "switch." + spaceName + "/space"
		entityID := toHomeAssistantEntityID(entityName)
		if entityID == "" {
			continue
		}

		identity := extractEntityIdentity(entityName)
		if identity.Sphere == "" {
			continue
		}

		displayName := entityName[len("switch."):]

		// When a space has an explicit "space off:" directive, only the explicitly listed items
		// (plus any child space switches that propagated in) appear in turn_off and state.
		// When there is no explicit directive (defaults or child propagation only), the
		// lights from SpaceOnByName are also included so everything in the space goes off.
		explicitOff := admin.SpaceHasExplicitOff[spaceName]
		var onItems []string
		if !explicitOff {
			onItems = sortedCopy(admin.SpaceOnByName[spaceName])
		}
		offItems = sortedCopy(offItems)
		explicitOnItems := sortedCopy(admin.SpaceSwitchOnByName[spaceName])
		if !recordSpaceAggregate(admin, entityID, entityNamesToIDs(append(append(append([]string{}, onItems...), offItems...), explicitOnItems...))) {
			continue
		}
		content := buildTemplateSwitchYAML(entityID, displayName, onItems, offItems, explicitOnItems, admin.SpaceAvailabilityAll[spaceName])
		dir := filepath.Join(outputDir, "entities", "template", "switch", identity.Sphere)
		if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
			return err
		}
	}

	// Sphere-level space switches: derived from root's accumulated off/on lists.
	sphereOnItems := map[string][]string{}
	for _, entry := range admin.SpaceOnByName["root"] {
		identity := extractEntityIdentity(entry)
		if identity.Sphere != "" && identity.Domain == "light" {
			sphereOnItems[identity.Sphere] = append(sphereOnItems[identity.Sphere], entry)
		}
	}
	sphereOffItems := map[string][]string{}
	for _, entry := range admin.SpaceOffByName["root"] {
		identity := extractEntityIdentity(entry)
		if identity.Sphere != "" && identity.Domain == "switch" {
			sphereOffItems[identity.Sphere] = append(sphereOffItems[identity.Sphere], entry)
		}
	}
	for _, sphere := range sortedKeys(sphereOffItems) {
		offItems := sortedCopy(sphereOffItems[sphere])
		hasSpace := false
		for _, item := range offItems {
			if strings.HasSuffix(item, "/space") {
				hasSpace = true
				break
			}
		}
		if !hasSpace {
			continue
		}
		onItems := sortedCopy(sphereOnItems[sphere])
		entityID := toHomeAssistantEntityID("switch." + sphere + "/space")
		displayName := sphere + "/space"
		if !recordSpaceAggregate(admin, entityID, entityNamesToIDs(append(append([]string{}, onItems...), offItems...))) {
			continue
		}
		content := buildTemplateSwitchYAML(entityID, displayName, onItems, offItems, nil, false)
		dir := filepath.Join(outputDir, "entities", "template", "switch", sphere)
		if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
			return err
		}
	}
	return nil
}

func buildTemplateSwitchYAML(entityID, displayName string, onItems, offItems, explicitOnItems []string, availabilityAll bool) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("switch:\n")
	sb.WriteString("- name: " + displayName + "\n")
	sb.WriteString("  unique_id: " + generatedUniqueID(entityID) + "\n")
	sb.WriteString("  default_entity_id: " + entityID + "\n")
	appendAvailability(&sb, aggregateAvailability(entityNamesToIDs(sortedCopy(deduplicated(append(append([]string{}, onItems...), offItems...)))), availabilityAll))
	// turn_on: if an explicit "space on:" list is provided it governs turn_on entirely.
	// Otherwise auto-include child space-aggregate switches (items ending with "/space")
	// so that activating a parent space switch cascades to its direct children.
	sb.WriteString("  turn_on:\n")
	for _, item := range explicitOnItems {
		itemID := toHomeAssistantEntityID(item)
		action := domainTurnOnAction(item)
		sb.WriteString("  - entity_id:\n")
		sb.WriteString("    - " + itemID + "\n")
		sb.WriteString("    action: " + action + "\n")
	}
	if len(explicitOnItems) == 0 {
		for _, item := range offItems {
			if !strings.HasSuffix(item, "/space") {
				continue
			}
			itemID := toHomeAssistantEntityID(item)
			sb.WriteString("  - entity_id:\n")
			sb.WriteString("    - " + itemID + "\n")
			sb.WriteString("    action: switch.turn_on\n")
		}
	}
	// turn_off: union of onItems and offItems, sorted alphabetically for deterministic output.
	sb.WriteString("  turn_off:\n")
	turnOffItems := sortedCopy(deduplicated(append(append([]string{}, onItems...), offItems...)))
	for _, item := range turnOffItems {
		itemID := toHomeAssistantEntityID(item)
		action := domainTurnOffAction(item)
		sb.WriteString("  - entity_id:\n")
		sb.WriteString("    - " + itemID + "\n")
		sb.WriteString("    action: " + action + "\n")
	}

	appendStateBlock(&sb, turnOffItems)
	return sb.String()
}

// --- template binary sensor entities (node, node_alert, battery_alert) ---

// generateTemplateBinarySensors writes entities/template/binary_sensor/infrastructural/<id>.yaml
// for node, node_alert, and battery_alert entities derived from providing/battery_alert macro expansion.
func generateTemplateBinarySensors(outputDir string, admin *TAdministrationState) error {
	// Collect all binary_sensor.infrastructural entity records across all spaces, deduplicated.
	seen := map[string]bool{}
	generatedNodeEntityIDs := map[string]bool{}
	type infraRecord struct {
		name string
		rec  TEntityRecord
	}
	var infraRecords []infraRecord
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain != "binary_sensor" || rec.Identity.Sphere != "infrastructural" {
				continue
			}
			if seen[rec.Name] {
				continue
			}
			seen[rec.Name] = true
			infraRecords = append(infraRecords, infraRecord{name: rec.Name, rec: rec})
		}
	}

	for _, ir := range infraRecords {
		entityID := toHomeAssistantEntityID(ir.name)
		if entityID == "" {
			continue
		}
		path := ir.rec.Identity.Path

		sphere := ir.rec.Identity.Sphere

		if strings.HasSuffix(path, "/node") {
			nodeEntityID, repID := resolvedNodeEntityID(ir.name, ir.rec, admin)
			if repID == "" {
				continue
			}
			nodePath := path
			generatedNodeEntityIDs[nodeEntityID] = true
			dir := filepath.Join(outputDir, "entities", "template", "binary_sensor", "infrastructural")
			customDir := filepath.Join(outputDir, "customization", "binary_sensor", "infrastructural")
			nodeContent := buildTemplateNodeYAML(nodeEntityID, sphere+"/"+nodePath, repID)
			if err := writeYAMLFile(filepath.Join(dir, nodeEntityID+".yaml"), nodeContent); err != nil {
				return err
			}
			// Customization for the node entity itself. No icon: device_class connectivity
			// (buildTemplateNodeYAML) already gives HA a sensible default.
			nodeCustom := buildCustomizationYAML(nodeEntityID, sphere+"/"+nodePath, "")
			if err := writeYAMLFile(filepath.Join(customDir, nodeEntityID+".yaml"), nodeCustom); err != nil {
				return err
			}
			continue
		}

		if strings.HasSuffix(path, "/battery_alert") && ir.rec.BatteryAlertLevel > 0 {
			// Derive battery_level sensor entity ID: same path hierarchy but sensor.infrastructural domain.
			batteryLevelID := toHomeAssistantEntityID("sensor.infrastructural/" + strings.TrimSuffix(path, "/battery_alert") + "/battery_level")
			content := buildTemplateBatteryAlertYAML(entityID, sphere+"/"+path, batteryLevelID, ir.rec.BatteryAlertLevel)
			dir := filepath.Join(outputDir, "entities", "template", "binary_sensor", "infrastructural")
			if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
				return err
			}
			// Customization for battery_alert (derived entity, not in EntityRecordsBySpace).
			customDir := filepath.Join(outputDir, "customization", "binary_sensor", "infrastructural")
			custom := buildCustomizationYAML(entityID, sphere+"/"+path, "mdi:battery-alert")
			if err := writeYAMLFile(filepath.Join(customDir, entityID+".yaml"), custom); err != nil {
				return err
			}
			continue
		}
	}

	// Generate nodes for media_player entities from media_player_device without an enabler.
	// When the enabler is absent the macro does not call providing, so no infra node entity is
	// created by the standard path. Old system always emits a node for every media_player entity.
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain != "media_player" {
				continue
			}
			// Derive expected node entity ID from the media_player path.
			spacePath := rec.Identity.Path // e.g. "apartment/living_room/sonos"
			nodeEntityID := toHomeAssistantEntityID("binary_sensor.infrastructural/" + spacePath + "/node")
			if generatedNodeEntityIDs[nodeEntityID] {
				continue // already generated (e.g. TV with enabler)
			}
			generatedNodeEntityIDs[nodeEntityID] = true
			mediaPlayerHAID := toHomeAssistantEntityID(rec.Name)
			displayPath := "infrastructural/" + spacePath + "/node"
			dir := filepath.Join(outputDir, "entities", "template", "binary_sensor", "infrastructural")
			customDir := filepath.Join(outputDir, "customization", "binary_sensor", "infrastructural")
			nodeContent := buildTemplateNodeYAML(nodeEntityID, displayPath, mediaPlayerHAID)
			if err := writeYAMLFile(filepath.Join(dir, nodeEntityID+".yaml"), nodeContent); err != nil {
				return err
			}
			// No icon: device_class connectivity (buildTemplateNodeYAML) already gives HA a
			// sensible default.
			nodeCustom := buildCustomizationYAML(nodeEntityID, displayPath, "")
			if err := writeYAMLFile(filepath.Join(customDir, nodeEntityID+".yaml"), nodeCustom); err != nil {
				return err
			}
		}
	}

	return nil
}

// resolveNodeRepresentative returns the HA entity ID of the entity that the node monitors.
// Returns empty string if no representative can be determined.
func resolveNodeRepresentative(entityName string, rec TEntityRecord, admin *TAdministrationState) string {
	if id, ok := admin.NodeRepresentativeByEntityID[entityName]; ok {
		return id
	}
	if rec.NodeRepresentativeEntityID != "" {
		return rec.NodeRepresentativeEntityID
	}
	return ""
}

// resolvedNodeEntityID returns the actual generated entity ID and representative for a "/node"
// record. The "_light"/"_switch" infix (where applicable) is baked directly into the entity's
// path by the light_device/switch_device macros (via "call providing ${entity.path}/light" etc.
// in Macros.def), so entityID here is simply the record's own name — no renaming needed. This is
// the single source of truth for representative resolution — generateTemplateBinarySensors,
// generateInfrastructuralGroups, and resolveListEntries all call it, rather than each re-deriving
// the rule independently.
func resolvedNodeEntityID(entityName string, rec TEntityRecord, admin *TAdministrationState) (entityID, repID string) {
	entityID = toHomeAssistantEntityID(entityName)
	repID = resolveNodeRepresentative(entityName, rec, admin)
	return entityID, repID
}

func buildTemplateNodeYAML(entityID, displayPath, representativeID string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("binary_sensor:\n")
	sb.WriteString("- name: " + displayPath + "\n")
	sb.WriteString("  unique_id: " + generatedUniqueID(entityID) + "\n")
	sb.WriteString("  device_class: connectivity\n")
	sb.WriteString("  state: \"{{ states('" + representativeID + "') != 'unavailable' }}\"\n")
	return sb.String()
}

func buildTemplateBatteryAlertYAML(entityID, displayPath, batteryLevelEntityID string, alertLevel int) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("binary_sensor:\n")
	sb.WriteString("- name: " + displayPath + "\n")
	sb.WriteString("  unique_id: " + generatedUniqueID(entityID) + "\n")
	sb.WriteString("  device_class: battery\n")
	sb.WriteString(fmt.Sprintf("  state: \"{{ (states('%s') | int(0)) < %d }}\"\n", batteryLevelEntityID, alertLevel))
	return sb.String()
}

func generateCliSwitches(outputDir string, admin *TAdministrationState) error {
	for _, rec := range admin.CliSwitches {
		id := toHomeAssistantEntityID(rec.LocalEntityName)
		if id == "" {
			continue
		}
		identity := extractEntityIdentity(rec.LocalEntityName)
		displayName := identity.Sphere + "/" + identity.Path
		content := buildCliSwitchYAML(id, displayName, rec.UserAlias, rec.HostFQDN, rec.OnScript, rec.OffScript, rec.StateScript)
		dir := filepath.Join(outputDir, "entities", "command_line", "switch", identity.Sphere)
		if err := writeYAMLFile(filepath.Join(dir, id+".yaml"), content); err != nil {
			return err
		}
	}
	return nil
}

func buildCliSwitchYAML(id, displayName, userAlias, hostFQDN, onScript, offScript, stateScript string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- switch:\n")
	sb.WriteString("    name: " + displayName + "\n")
	sb.WriteString("    unique_id: " + generatedUniqueID(id) + "\n")
	sb.WriteString("    command_on: \"bash /config/bin/run " + userAlias + " " + hostFQDN + " " + onScript + "\"\n")
	sb.WriteString("    command_off: \"bash /config/bin/run " + userAlias + " " + hostFQDN + " " + offScript + "\"\n")
	sb.WriteString("    command_state: \"bash /config/bin/run " + userAlias + " " + hostFQDN + " " + stateScript + "\"\n")
	return sb.String()
}

func generateInputNumbers(outputDir string, admin *TAdministrationState) error {
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain != "input_number" || rec.InputNumberMin == "" {
				continue
			}
			id := toHomeAssistantEntityID(rec.Name)
			if id == "" {
				continue
			}
			identity := rec.Identity
			displayName := identity.Sphere + "/" + identity.Path
			key := strings.TrimPrefix(id, "input_number.")
			content := buildInputNumberYAML(key, displayName, rec.InputNumberMin, rec.InputNumberMax, rec.InputNumberStep, rec.InputNumberUnit, rec.InputNumberIcon, "")
			dir := filepath.Join(outputDir, "entities", "input_number", identity.Sphere)
			if err := writeYAMLFile(filepath.Join(dir, id+".yaml"), content); err != nil {
				return err
			}
		}
	}
	// Generate a temperature_target input_number for every space that contains a climate entity.
	seen := map[string]bool{}
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain != "climate" {
				continue
			}
			physPath := strings.TrimPrefix(spaceName, "social/")
			if seen[physPath] {
				continue
			}
			seen[physPath] = true
			entityID := toHomeAssistantEntityID("input_number.physical/" + physPath + "/temperature_target")
			key := strings.TrimPrefix(entityID, "input_number.")
			displayName := "physical/" + physPath + "/temperature_target"
			content := buildInputNumberYAML(key, displayName, "10", "30", "1", "°C", "mdi:thermometer", "")
			dir := filepath.Join(outputDir, "entities", "input_number", "physical")
			if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
				return err
			}
		}
	}
	return nil
}

func buildInputNumberYAML(key, displayName, min, max, step, unit, icon, initial string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString(key + ":\n")
	sb.WriteString("  name: " + displayName + "\n")
	sb.WriteString("  min: " + min + "\n")
	sb.WriteString("  max: " + max + "\n")
	if step != "" {
		sb.WriteString("  step: " + step + "\n")
	}
	if icon != "" {
		sb.WriteString("  icon: " + icon + "\n")
	}
	if unit != "" {
		sb.WriteString("  unit_of_measurement: " + unit + "\n")
	}
	if initial != "" {
		sb.WriteString("  initial: " + initial + "\n")
	}
	return sb.String()
}

func generateInputBooleans(outputDir string, admin *TAdministrationState) error {
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain != "input_boolean" || rec.InputBooleanIcon == "" {
				continue
			}
			id := toHomeAssistantEntityID(rec.Name)
			if id == "" {
				continue
			}
			identity := rec.Identity
			displayName := identity.Sphere + "/" + identity.Path
			key := strings.TrimPrefix(id, "input_boolean.")
			var sb strings.Builder
			sb.WriteString(generatorHeader)
			sb.WriteString(key + ":\n")
			sb.WriteString("  name: " + displayName + "\n")
			sb.WriteString("  icon: " + rec.InputBooleanIcon + "\n")
			dir := filepath.Join(outputDir, "entities", "input_boolean", identity.Sphere)
			if err := writeYAMLFile(filepath.Join(dir, id+".yaml"), sb.String()); err != nil {
				return err
			}
		}
	}
	return nil
}

func generateInputDatetimes(outputDir string, admin *TAdministrationState) error {
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain != "input_datetime" || (!rec.InputDatetimeHasDate && !rec.InputDatetimeHasTime) {
				continue
			}
			id := toHomeAssistantEntityID(rec.Name)
			if id == "" {
				continue
			}
			identity := rec.Identity
			displayName := identity.Sphere + "/" + identity.Path
			key := strings.TrimPrefix(id, "input_datetime.")
			var sb strings.Builder
			sb.WriteString(generatorHeader)
			sb.WriteString(key + ":\n")
			sb.WriteString("  name: " + displayName + "\n")
			if rec.InputDatetimeHasDate {
				sb.WriteString("  has_date: true\n")
			} else {
				sb.WriteString("  has_date: false\n")
			}
			if rec.InputDatetimeHasTime {
				sb.WriteString("  has_time: true\n")
			} else {
				sb.WriteString("  has_time: false\n")
			}
			dir := filepath.Join(outputDir, "entities", "input_datetime", identity.Sphere)
			if err := writeYAMLFile(filepath.Join(dir, id+".yaml"), sb.String()); err != nil {
				return err
			}
		}
	}
	return nil
}

// generateTemplateSensors emits, for each space containing a climate entity,
// temperature_desired/temperature_target template sensors mirroring their respective
// input_numbers. (Two earlier shapes this function also rendered -- an "adjustment"-offset sensor
// and a bare "value <entity>!<attribute>;" sensor -- were retired 2026-10-05 once both were
// confirmed at zero remaining usage in either house; see parser.go's own history for the retired
// "with adjustment"/"value" directives.)
func generateTemplateSensors(outputDir string, admin *TAdministrationState) error {
	// For each space containing a climate entity, generate temperature_desired and
	// temperature_target template sensors that mirror their respective input_numbers.
	seen := map[string]bool{}
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain != "climate" {
				continue
			}
			physPath := strings.TrimPrefix(spaceName, "social/")
			if seen[physPath] {
				continue
			}
			seen[physPath] = true
			socialPath := "social/" + physPath
			for _, variant := range []struct{ suffix, inputSphere string }{
				{"temperature_desired", "social"},
				{"temperature_target", "physical"},
			} {
				sensorName := "sensor." + socialPath + "/" + variant.suffix
				entityID := toHomeAssistantEntityID(sensorName)
				displayName := socialPath + "/" + variant.suffix
				inputID := toHomeAssistantEntityID("input_number." + variant.inputSphere + "/" + physPath + "/" + variant.suffix)
				stateExpr := "states('" + inputID + "')"
				content := buildTemplateSensorYAML(entityID, displayName, "°C", "temperature", "measurement", "mdi:thermometer", stateExpr, derivedValueAvailability(stateExpr))
				dir := filepath.Join(outputDir, "entities", "template", "sensor", "social")
				if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
					return err
				}
				custom := buildCustomizationYAML(entityID, displayName, "mdi:thermometer")
				customDir := filepath.Join(outputDir, "customization", "sensor", "social")
				if err := writeYAMLFile(filepath.Join(customDir, entityID+".yaml"), custom); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// generateConditionEntities emits template binary_sensor or sensor YAML for any entity
// whose body contains a generic "condition <sources> <expr>" directive.
func generateConditionEntities(outputDir string, admin *TAdministrationState) error {
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.ConditionExpr == "" {
				continue
			}
			id := toHomeAssistantEntityID(rec.Name)
			if id == "" {
				continue
			}
			identity := rec.Identity
			displayName := identity.Sphere + "/" + identity.Path
			stateExpr := buildConditionStateExpr(rec.ConditionSources, rec.ConditionExpr)
			var content string
			switch identity.Domain {
			case "binary_sensor":
				deviceClass := rec.ConditionDevClass
				if deviceClass == "" {
					deviceClass, _, _, _ = resolveCapabilityDefaults(admin.CapabilityDefaults, "binary_sensor", identity.Path)
				}
				availability := ""
				// A node, a battery alert and the firmware updates summary stay available while one
				// of their sources is down: reporting exactly that, or ignoring it, is their job.
				if !strings.HasSuffix(identity.Path, "/node") && !strings.HasSuffix(identity.Path, "/battery_alert") && identity.Path != "firmware_updates" {
					availability = derivedValueAvailability(stateExpr)
				}
				content = buildConditionBinarySensorYAML(id, displayName, stateExpr, deviceClass, rec.ConditionDelayOn, rec.ConditionDelayOff, availability)
			case "sensor":
				deviceClass, unit, stateClass, icon := resolveCapabilityDefaults(admin.CapabilityDefaults, "sensor", identity.Path)
				content = buildTemplateSensorYAML(id, displayName, unit, deviceClass, stateClass, icon, stateExpr, derivedValueAvailability(stateExpr))
			default:
				continue
			}
			dir := filepath.Join(outputDir, "entities", "template", identity.Domain, identity.Sphere)
			if err := writeYAMLFile(filepath.Join(dir, id+".yaml"), content); err != nil {
				return err
			}
		}
	}
	return nil
}

// buildConditionStateExpr substitutes $/$1/$2/... placeholders in a condition expression
// with states('<entity_id>') calls for the corresponding sources.
// sourceToJinja2 converts a condition source token to its Jinja2 expression.
// A plain token produces states('entity_id'); one with "!attr" produces state_attr('entity_id', 'attr');
// one with a trailing " is available" produces the standard HA liveness check -- sugar for what
// used to require spelling out `states('$1') != 'unavailable'` by hand in every
// Macros.def condition body that needed it (was "forgotten" boilerplate, not a distinct concept).
// Rendered as the literal strings "on"/"off", not a raw Python boolean: verified against HA's own
// source (template.result_as_boolean, used by both template binary_sensor's "state:" field and
// this codebase's own condition-clause consumers) that "on"/"ON"/"true"/"1" are accepted exactly
// like True/False there, while a raw "True"/"False" is NOT one of MQTT binary_sensor discovery's
// default payload_on/payload_off values -- rendering on/off directly is correct for both
// consumers, whereas raw Python booleans silently break the MQTT relay path (confirmed live:
// host.junglinster's node capability stayed "unknown" in HA despite MQTT showing the retained
// state "True").
//
// Lowercase, not uppercase: when this feeds a hassbridge/import "node" capability's reporting
// automation, the coordinator relays the retained payload verbatim into TWO places that both
// match it case-sensitively (raw MQTT string comparison, no Jinja involved) -- applyProxiedBinarySensorPayload's
// own payload_on/payload_off ("on"/"off", house_event_bus_coordinator/discovery.go) for the node
// entity's own displayed state, and buildAvailabilityFields' nodeTopic factor (payload_available
// "on"/payload_not_available "off", same file) that gates every OTHER capability of the same
// device. A prior version of this literal was uppercase "ON"/"OFF" -- accepted fine by template
// binary_sensor's own case-insensitive "state:" evaluation, but silently never matched either of
// those two lowercase MQTT comparisons, permanently stranding the node entity's own display AND
// every sibling capability's availability at "unavailable" for any device using this "is
// available" node-sourcing form (confirmed live 2026-09-02: hass.envoy + its 10 inverters, the
// first devices to combine this form with the newer availability-gating mechanism).
func sourceToJinja2(src string) string {
	if entityID, ok := strings.CutSuffix(src, " is available"); ok {
		return fmt.Sprintf("'on' if (states('%s') != 'unavailable') else 'off'", entityID)
	}
	if bangIdx := strings.Index(src, "!"); bangIdx > 0 {
		entityID := src[:bangIdx]
		attr := src[bangIdx+1:]
		return fmt.Sprintf("state_attr('%s', '%s')", entityID, attr)
	}
	return fmt.Sprintf("states('%s')", src)
}

// jinjaStringLiteral renders s as a single-quoted Jinja2 string literal, escaping any embedded
// single quotes (Jinja/Python-style, "\'") so a literal like "Erik's Pi" doesn't break out of
// the quoting.
func jinjaStringLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "\\'") + "'"
}

func buildConditionStateExpr(sources []string, expr string) string {
	result := expr
	if len(sources) == 1 {
		jinja := sourceToJinja2(sources[0])
		result = strings.ReplaceAll(result, "$1", jinja)
		result = strings.ReplaceAll(result, "$", jinja)
	} else {
		for i, src := range sources {
			result = strings.ReplaceAll(result, fmt.Sprintf("$%d", i+1), sourceToJinja2(src))
		}
		if len(sources) > 0 {
			result = strings.ReplaceAll(result, "$", sourceToJinja2(sources[0]))
		}
	}
	return result
}

func buildConditionBinarySensorYAML(id, displayName, stateExpr, deviceClass, delayOn, delayOff, availability string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("binary_sensor:\n")
	sb.WriteString("- name: " + displayName + "\n")
	sb.WriteString("  unique_id: " + generatedUniqueID(id) + "\n")
	if deviceClass != "" {
		sb.WriteString("  device_class: " + deviceClass + "\n")
	}
	if delayOn != "" {
		sb.WriteString("  delay_on: " + delayOn + "\n")
	}
	if delayOff != "" {
		sb.WriteString("  delay_off: " + delayOff + "\n")
	}
	appendAvailability(&sb, availability)
	sb.WriteString("  state: \"{{ " + stateExpr + " }}\"\n")
	return sb.String()
}

func buildTemplateSensorYAML(id, displayName, unit, deviceClass, stateClass, icon, stateExpr, availability string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("sensor:\n")
	sb.WriteString("- name: " + displayName + "\n")
	sb.WriteString("  unique_id: " + generatedUniqueID(id) + "\n")
	if unit != "" {
		sb.WriteString("  unit_of_measurement: \"" + unit + "\"\n")
	}
	if deviceClass != "" {
		sb.WriteString("  device_class: " + deviceClass + "\n")
	}
	if stateClass != "" {
		sb.WriteString("  state_class: " + stateClass + "\n")
	}
	appendAvailability(&sb, availability)
	sb.WriteString("  state: \"{{ " + stateExpr + " }}\"\n")
	return sb.String()
}

// --- infrastructural binary sensor aggregate groups ---

// generateInfrastructuralGroups writes binary_sensor/infrastructural/binary_sensor.infrastructural_battery_alert.yaml
// and binary_sensor.infrastructural_node_alert.yaml, listing all matching entities sorted alphabetically.
func generateInfrastructuralGroups(outputDir string, admin *TAdministrationState) error {
	var batteryAlerts, nodes []string
	seen := map[string]bool{}
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain != "binary_sensor" || rec.Identity.Sphere != "infrastructural" {
				continue
			}
			id := toHomeAssistantEntityID(rec.Name)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			if strings.HasSuffix(rec.Identity.Path, "/battery_alert") {
				batteryAlerts = append(batteryAlerts, id)
			} else if strings.HasSuffix(rec.Identity.Path, "/node") {
				// Every declared /node binary_sensor feeds the AND aggregate below, whether it
				// has a generator-built representative or is purely assumed/discovery-implied
				// (e.g. an "entity device.<name> from <device-id> ...;" node -- Conceptual_DeviceEntities.go).
				nodes = append(nodes, id)
			}
		}
	}
	// Add nodes for media_player entities that have no explicit infra node entity.
	// generateTemplateBinarySensors synthesises a node for every media_player that was not
	// already covered by the providing macro; include those here too.
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain != "media_player" {
				continue
			}
			nodeEntityID := toHomeAssistantEntityID("binary_sensor.infrastructural/" + rec.Identity.Path + "/node")
			if nodeEntityID == "" || seen[nodeEntityID] {
				continue
			}
			seen[nodeEntityID] = true
			nodes = append(nodes, nodeEntityID)
		}
	}
	sort.Strings(batteryAlerts)
	sort.Strings(nodes)

	dir := filepath.Join(outputDir, "entities", "binary_sensor", "infrastructural")
	customDir := filepath.Join(outputDir, "customization", "binary_sensor", "infrastructural")
	if len(batteryAlerts) > 0 {
		const baID = "binary_sensor.infrastructural_battery_alert"
		const baName = "infrastructural/battery_alert"
		deviceClass, _, _, _ := resolveCapabilityDefaults(admin.CapabilityDefaults, "binary_sensor", "battery_alert")
		content := buildInfraGroupYAML(baID, baName, batteryAlerts, false, deviceClass)
		if err := writeYAMLFile(filepath.Join(dir, baID+".yaml"), content); err != nil {
			return err
		}
		// No icon: device_class (above) already gives HA a sensible default.
		custom := buildCustomizationYAML(baID, baName, "")
		if err := writeYAMLFile(filepath.Join(customDir, baID+".yaml"), custom); err != nil {
			return err
		}
	}
	if len(nodes) > 0 {
		const nID = "binary_sensor.infrastructural_nodes"
		const nName = "infrastructural/nodes"
		deviceClass, _, _, _ := resolveCapabilityDefaults(admin.CapabilityDefaults, "binary_sensor", "nodes")
		content := buildInfraGroupYAML(nID, nName, nodes, true, deviceClass)
		if err := writeYAMLFile(filepath.Join(dir, nID+".yaml"), content); err != nil {
			return err
		}
		// No icon: device_class (above) already gives HA a sensible default.
		custom := buildCustomizationYAML(nID, nName, "")
		if err := writeYAMLFile(filepath.Join(customDir, nID+".yaml"), custom); err != nil {
			return err
		}
	}
	return nil
}

// buildInfraGroupYAML builds a "platform: group" aggregate. all=true gives AND semantics
// (state is "on" only when every member is "on" -- used for the nodes aggregate); all=false
// keeps the default OR semantics (state is "on" when any member is "on" -- used for alerts,
// where any single alert firing should trigger the aggregate).
func buildInfraGroupYAML(uniqueID, name string, entities []string, all bool, deviceClass string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("platform: group\n")
	sb.WriteString("name: " + name + "\n")
	sb.WriteString("unique_id: " + generatedUniqueID(uniqueID) + "\n")
	if all {
		sb.WriteString("all: true\n")
	}
	if deviceClass != "" {
		sb.WriteString("device_class: " + deviceClass + "\n")
	}
	sb.WriteString("entities:\n")
	for _, e := range entities {
		sb.WriteString("- " + e + "\n")
	}
	return sb.String()
}

// --- binary sensor group (heating leakage evidence) ---

// generateBinarySensorGroups writes entities/binary_sensor/physical/<id>.yaml for every space
// with a climate entity or a physical heating switch. The group sensor is true when any listed
// door/window is open (i.e. in the 'on' state for a binary_sensor); a space with no "heating
// leak:" entries has no leakage, and no group.
func generateBinarySensorGroups(outputDir string, admin *TAdministrationState) error {
	for _, spaceName := range heatingCapableSocialSpaceNames(admin) {
		leaks := admin.HeatingLeaksByName[spaceName]
		if len(leaks) == 0 {
			continue
		}

		// Derive the entity name as registered during parsing.
		contextPath := spaceNameContextPath(spaceName)
		var entityName string
		if contextPath == "" {
			entityName = "binary_sensor.physical/heating/leakage/evidence"
		} else {
			entityName = fmt.Sprintf("binary_sensor.physical/%s/heating/leakage/evidence", contextPath)
		}

		entityID := toHomeAssistantEntityID(entityName)
		if entityID == "" {
			continue
		}

		displayName := entityName[len("binary_sensor."):]

		// Members: HA entity IDs for each leaking door/window sensor, sorted alphabetically.
		memberIDs := make([]string, 0, len(leaks))
		for _, leak := range sortedCopy(leaks) {
			if id := toHomeAssistantEntityID(leak); id != "" {
				memberIDs = append(memberIDs, id)
			}
		}

		if !recordSpaceAggregate(admin, entityID, memberIDs) {
			continue
		}
		content := buildBinarySensorGroupYAML(entityID, displayName, memberIDs)
		dir := filepath.Join(outputDir, "entities", "binary_sensor", "physical")
		if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
			return err
		}
	}
	return nil
}

func buildBinarySensorGroupYAML(entityID, displayName string, memberIDs []string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("platform: group\n")
	sb.WriteString("name: " + displayName + "\n")
	sb.WriteString("unique_id: " + generatedUniqueID(entityID) + "\n")
	sb.WriteString("entities:\n")
	for _, id := range memberIDs {
		sb.WriteString("- " + id + "\n")
	}
	return sb.String()
}

// --- binary sensor subdomain groups (door, window, motion, water) ---

// binarySensorSubdomains lists the entity path suffixes that are aggregated per social space.
var binarySensorSubdomains = []string{"door", "motion", "water", "window"}

// generateBinarySensorSubdomainGroups writes entities/binary_sensor/social/<id>.yaml for
// every social space that has at least one binary sensor entity (or child group) matching a
// subdomain.  Groups are built hierarchically: leaf spaces collect declared entities directly;
// parent spaces collect the derived child-space group entities.
//
// Additionally, sphere-level aggregates (binary_sensor.social/<sub>) and a windoor group
// (binary_sensor.social/windoor = union of top-level door + window groups) are emitted.
// Template-derived subdomains such as windy and sunny are not aggregated at the sphere level;
// their terrace-specific entities are used directly.
func generateBinarySensorSubdomainGroups(outputDir string, admin *TAdministrationState) error {
	// derivedGroups[subdomain][spaceName] = full entity name of the derived group.
	derivedGroups := map[string]map[string]string{}
	for _, sub := range binarySensorSubdomains {
		derivedGroups[sub] = map[string]string{}
	}

	// Process in reverse SpaceOrder so children are handled before their parents.
	for i := len(admin.SpaceOrder) - 1; i >= 0; i-- {
		spaceName := admin.SpaceOrder[i]
		if !strings.HasPrefix(spaceName, "social/") {
			continue
		}
		for _, sub := range binarySensorSubdomains {
			members := binarySensorSubdomainMembers(spaceName, sub, admin, derivedGroups)
			groupName := "binary_sensor." + spaceName + "/" + sub
			if len(members) == 0 {
				// If an entity with the same name as the would-be group is declared directly
				// in this space, register it so parent spaces reference it via derivedGroups.
				for _, rec := range admin.EntityRecordsBySpace[spaceName] {
					if rec.Name == groupName && !strings.HasPrefix(rec.Provenance, "derived") {
						derivedGroups[sub][spaceName] = groupName
						break
					}
				}
				continue
			}
			groupID := toHomeAssistantEntityID(groupName)
			displayName := spaceName + "/" + sub
			if !recordSpaceAggregate(admin, groupID, entityNamesToIDs(members)) {
				derivedGroups[sub][spaceName] = groupName
				continue
			}
			content := buildBinarySensorGroupYAML(groupID, displayName, entityNamesToIDs(sortedCopy(members)))
			dir := filepath.Join(outputDir, "entities", "binary_sensor", "social")
			if err := writeYAMLFile(filepath.Join(dir, groupID+".yaml"), content); err != nil {
				return err
			}
			derivedGroups[sub][spaceName] = groupName
		}
	}

	writeBinarySensorSphereGroup := func(groupID, displayName string, members []string) error {
		if !recordSpaceAggregate(admin, groupID, entityNamesToIDs(members)) {
			return nil
		}
		content := buildBinarySensorGroupYAML(groupID, displayName, entityNamesToIDs(sortedCopy(members)))
		dir := filepath.Join(outputDir, "entities", "binary_sensor", "social")
		if err := writeYAMLFile(filepath.Join(dir, groupID+".yaml"), content); err != nil {
			return err
		}
		sub := displayName[strings.LastIndex(displayName, "/")+1:]
		_, _, _, icon := resolveCapabilityDefaults(admin.CapabilityDefaults, "binary_sensor", sub)
		custom := buildCustomizationYAML(groupID, displayName, icon)
		customDir := filepath.Join(outputDir, "customization", "binary_sensor", "social")
		return writeYAMLFile(filepath.Join(customDir, groupID+".yaml"), custom)
	}

	// Sphere-level aggregates: binary_sensor.social/<sub> from direct children of the social sphere.
	socialChildren := directChildSpaces("social", admin.SpaceOrder)
	for _, sub := range binarySensorSubdomains {
		members := []string{}
		for _, child := range socialChildren {
			if group, ok := derivedGroups[sub][child]; ok {
				members = append(members, group)
			}
		}
		if len(members) == 0 {
			continue
		}
		groupID := toHomeAssistantEntityID("binary_sensor.social/" + sub)
		displayName := "social/" + sub
		if err := writeBinarySensorSphereGroup(groupID, displayName, members); err != nil {
			return err
		}
		derivedGroups[sub]["social"] = "binary_sensor.social/" + sub
	}

	// Windoor: union of door and window groups from each direct social child (not the
	// sphere-level aggregates themselves, to keep the list flat and match the old system).
	windoorMembers := []string{}
	for _, sub := range []string{"door", "window"} {
		for _, child := range socialChildren {
			if group, ok := derivedGroups[sub][child]; ok {
				windoorMembers = append(windoorMembers, group)
			}
		}
	}
	if len(windoorMembers) > 0 {
		groupID := toHomeAssistantEntityID("binary_sensor.social/windoor")
		if err := writeBinarySensorSphereGroup(groupID, "social/windoor", windoorMembers); err != nil {
			return err
		}
	}

	return nil
}

// binarySensorSubdomainMembers collects the members for a binary sensor subdomain group in a
// given space: direct binary sensor entities in the space whose path ends with /<subdomain>,
// plus any direct child spaces that already have a derived group for the subdomain.
// Derived aggregate entities (synthetic placeholders) and the group entity itself are excluded
// to prevent self-references in the generated YAML.
func binarySensorSubdomainMembers(spaceName, subdomain string, admin *TAdministrationState, derivedGroups map[string]map[string]string) []string {
	groupName := "binary_sensor." + spaceName + "/" + subdomain
	var members []string
	for _, rec := range admin.EntityRecordsBySpace[spaceName] {
		if rec.Identity.Domain == "binary_sensor" &&
			strings.HasSuffix(rec.Identity.Path, "/"+subdomain) &&
			rec.Name != groupName &&
			rec.Provenance != "derived binary_sensor subdomain aggregate" {
			members = append(members, rec.Name)
		}
	}
	for _, child := range directChildSpaces(spaceName, admin.SpaceOrder) {
		if group, ok := derivedGroups[subdomain][child]; ok {
			members = append(members, group)
		} else {
			// When the child space has no derived group, include any direct entities
			// with the subdomain path (e.g. a single door sensor declared as social domain,
			// whose name would collide with the would-be group name).
			for _, rec := range admin.EntityRecordsBySpace[child] {
				if rec.Identity.Domain == "binary_sensor" &&
					strings.HasSuffix(rec.Identity.Path, "/"+subdomain) &&
					rec.Provenance != "derived binary_sensor subdomain aggregate" {
					members = append(members, rec.Name)
				}
			}
		}
	}
	return members
}

// --- sensor subdomain mean groups (temperature, humidity, co2, noise, pressure, illuminance) ---

// sensorSubdomains lists the entity path suffixes aggregated into mean sensor groups.
var sensorSubdomains = []string{"co2", "humidity", "illuminance", "noise", "pressure", "temperature"}

// generateSensorSubdomainGroups writes entities/sensor/social/<id>.yaml for every social
// space (and for the social sphere as a whole) that contains at least one sensor entity
// whose path ends with /<subdomain>.  Sensor groups use type:mean and collect all matching
// sensor entities from the entire subtree under the space (not recursive social aggregates),
// which is how the old bash system computed space-level climate readings.
func generateSensorSubdomainGroups(outputDir string, admin *TAdministrationState) error {
	writeSensorGroup := func(groupID, displayName string, memberIDs []string) error {
		if !recordSpaceAggregate(admin, groupID, memberIDs) {
			return nil
		}
		content := buildSensorGroupYAML(groupID, displayName, memberIDs)
		dir := filepath.Join(outputDir, "entities", "sensor", "social")
		if err := writeYAMLFile(filepath.Join(dir, groupID+".yaml"), content); err != nil {
			return err
		}
		subdomain := displayName[strings.LastIndex(displayName, "/")+1:]
		_, _, _, icon := resolveCapabilityDefaults(admin.CapabilityDefaults, "sensor", subdomain)
		customContent := buildCustomizationYAML(groupID, displayName, icon)
		customDir := filepath.Join(outputDir, "customization", "sensor", "social")
		return writeYAMLFile(filepath.Join(customDir, groupID+".yaml"), customContent)
	}

	// Per-space aggregates.
	for _, spaceName := range admin.SpaceOrder {
		if !strings.HasPrefix(spaceName, "social/") {
			continue
		}
		for _, sub := range sensorSubdomains {
			members := sensorSubdomainEntities(spaceName, sub, admin)
			if len(members) == 0 {
				continue
			}
			groupID := toHomeAssistantEntityID("sensor." + spaceName + "/" + sub)
			displayName := spaceName + "/" + sub
			if err := writeSensorGroup(groupID, displayName, entityNamesToIDs(sortedCopy(members))); err != nil {
				return err
			}
		}
	}

	// Sphere-level aggregates: collect across all social-sphere spaces.
	for _, sub := range sensorSubdomains {
		members := sensorSubdomainEntities("social", sub, admin)
		if len(members) == 0 {
			continue
		}
		groupID := toHomeAssistantEntityID("sensor.social/" + sub)
		displayName := "social/" + sub
		if err := writeSensorGroup(groupID, displayName, entityNamesToIDs(sortedCopy(members))); err != nil {
			return err
		}
	}

	return nil
}

// sensorSubdomainEntities collects all sensor entities whose path ends with /<subdomain>
// from the given space and all of its descendant spaces.  Passing "social" as spaceName
// collects from all social-sphere spaces (the sphere-level aggregate case). Members are
// deliberately not restricted to Sphere=="social": a room's climate readings are commonly
// declared physical-sphere (raw hardware sensors, e.g. "sensor.physical:rear/aqara_multi/humidity")
// while still belonging in that room's social-facing aggregate -- that's the dominant existing
// pattern, not an exception. What must be excluded is Sphere=="infrastructural": those are
// device/node health sensors (e.g. "sensor.infrastructural:/smarty/cpu/temperature",
// Conceptual_DeviceEntities.go), not room climate, and can be filed under a "social/..." space
// bucket via a sphere-absolute path while having nothing to do with that room.
func sensorSubdomainEntities(spaceName, subdomain string, admin *TAdministrationState) []string {
	prefix := spaceName + "/"
	seen := map[string]bool{}
	var result []string
	for _, s := range admin.SpaceOrder {
		if s != spaceName && !strings.HasPrefix(s, prefix) {
			continue
		}
		for _, rec := range admin.EntityRecordsBySpace[s] {
			if rec.Identity.Domain == "sensor" &&
				rec.Identity.Sphere != "infrastructural" &&
				strings.HasSuffix(rec.Identity.Path, "/"+subdomain) &&
				!rec.NoCollect &&
				!seen[rec.Name] {
				seen[rec.Name] = true
				result = append(result, rec.Name)
			}
		}
	}
	return result
}

func buildSensorGroupYAML(entityID, displayName string, memberIDs []string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("platform: group\n")
	sb.WriteString("name: " + displayName + "\n")
	sb.WriteString("unique_id: " + generatedUniqueID(entityID) + "\n")
	sb.WriteString("type: mean\n")
	sb.WriteString("ignore_non_numeric: true\n")
	sb.WriteString("state_class: measurement\n")
	sb.WriteString("entities:\n")
	for _, id := range memberIDs {
		sb.WriteString("- " + id + "\n")
	}
	return sb.String()
}

// --- light group entities (Zigbee physical groups) ---

// generateLightGroups writes entities/light/<sphere>/<id>.yaml for every space where
// all registered entities are no_collect light entities.  Such spaces are created by the
// zigbee_group macro and represent physical Zigbee groups whose members are individual bulbs.
func generateLightGroups(outputDir string, admin *TAdministrationState) error {
	for _, spaceName := range admin.SpaceOrder {
		records := admin.EntityRecordsBySpace[spaceName]
		if !isNoCollectLightSpace(spaceName, records) {
			continue
		}

		entityName := "light." + spaceName
		entityID := toHomeAssistantEntityID(entityName)
		if entityID == "" {
			continue
		}

		identity := extractEntityIdentity(entityName)
		if identity.Sphere == "" {
			continue
		}

		displayName := entityName[len("light."):]

		// Members: the individual bulb light entities in this no_collect space.
		memberIDs := make([]string, 0, len(records))
		for _, rec := range records {
			if rec.Identity.Domain != "light" {
				continue
			}
			if id := toHomeAssistantEntityID(rec.Name); id != "" {
				memberIDs = append(memberIDs, id)
			}
		}
		// Sort for deterministic output.
		sort.Strings(memberIDs)

		if !recordSpaceAggregate(admin, entityID, memberIDs) {
			continue
		}
		content := buildLightGroupYAML(entityID, displayName, memberIDs)
		dir := filepath.Join(outputDir, "entities", "light", identity.Sphere)
		if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
			return err
		}

		customContent := buildCustomizationYAML(entityID, displayName, domainDefaultIcons["light"])
		customDir := filepath.Join(outputDir, "customization", "light", identity.Sphere)
		if err := writeYAMLFile(filepath.Join(customDir, entityID+".yaml"), customContent); err != nil {
			return err
		}
	}
	return nil
}

func buildLightGroupYAML(entityID, displayName string, memberIDs []string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("platform: group\n")
	sb.WriteString("name: " + displayName + "\n")
	sb.WriteString("unique_id: " + generatedUniqueID(entityID) + "\n")
	sb.WriteString("entities:\n")
	for _, id := range memberIDs {
		sb.WriteString("- " + id + "\n")
	}
	return sb.String()
}

// --- customization files ---

// subdomainIcons maps the last path segment (subdomain) of an entity to a default icon, for
// subdomains with no fixed domain (used regardless of what domain the entity actually is) --
// everything with a settled domain now lives in Shared/Definitions/Defaults.def's "defaults:"
// block instead (capability_defaults.go's resolveCapabilityDefaults, domain-gated), which is
// consulted first; this table is only the remaining domain-agnostic fallback.
// Entity-specific icons from "icon:" body properties are not yet tracked and take precedence
// once Phase 2 adds Icon to TEntityRecord.
var subdomainIcons = map[string]string{
	"consumes":        "mdi:flash",
	"daylight":        "mdi:weather-sunset-up",
	"health":          "mdi:cloud",
	"dishwasher":      "mdi:dishwasher",
	"media":           "mdi:monitor-speaker",
	"radiator":        "mdi:heating-coil",
	"radio":           "mdi:signal",
	"washing_machine": "mdi:washing-machine",
	"wind_direction":  "mdi:compass-outline",
	// Added 2026-09-25 (the Overkiz/Somfy cover migration): generic enough to apply to any future
	// device using the same button/number shape, not Somfy-specific.
	"identify":          "mdi:led-on",
	"my_position":       "mdi:map-marker-distance",
	"go_to_my_position": "mdi:map-marker-right",
}

// domainDefaultIcons maps HA entity domains to a fallback icon when no subdomain-specific
// icon applies.  These match the old bash generator's domain-level defaults.
var domainDefaultIcons = map[string]string{
	"cover": "mdi:blinds-horizontal",
	"light": "mdi:lightbulb-group",
	// Added 2026-09-25 (the Overkiz/Somfy cover migration, first real hassbridge use of either
	// domain): generic per-domain fallbacks, not Somfy-specific.
	"button": "mdi:gesture-tap-button",
	"number": "mdi:numeric",
}

// iconForEntity returns the icon string for an entity: an explicit rec.EntityIcon first, then a
// "defaults: for ...;" rule or the code-level postfix table (resolveCapabilityDefaults), falling
// back to a domain-level default. Returns "" when no icon is known.
func iconForEntity(rec TEntityRecord, capabilityDefaults []TCapabilityDefaultRule) string {
	if rec.EntityIcon != "" {
		return rec.EntityIcon
	}
	if rec.Identity.Path != "" {
		if _, _, _, icon := resolveCapabilityDefaults(capabilityDefaults, rec.Identity.Domain, rec.Identity.Path); icon != "" {
			return icon
		}
	}
	return domainDefaultIcons[rec.Identity.Domain]
}

// generateCustomizationFiles writes customization/<domain>/<sphere>/<id>.yaml for every
// registered entity, providing a human-readable friendly_name and, where known, an icon.
func generateCustomizationFiles(outputDir string, admin *TAdministrationState) error {
	// Collect all unique entities to avoid duplicates across spaces.
	seen := map[string]bool{}
	// A native entity (a local capability whose entity its own integration provides, PROJECT.md
	// 17j/21) only gets the defaults' typing through customization: device_class and state_class,
	// so e.g. a battery level keeps its long-term statistics (2026-10-28: the iPhone's lost them when
	// it moved from the bridge to "local" -- the Mobile App gives it no state class). Not the unit:
	// the integration states its own, and a different one would confuse the statistics.
	native := map[string]bool{}
	for deviceID, labels := range admin.NativeCapabilities {
		for _, label := range labels {
			if id := admin.DeviceConceptualLinks[deviceID].AttributeEntityIDs[label].EntityID; id != "" {
				native[id] = true
			}
		}
	}
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain == "" || rec.Identity.Sphere == "" {
				continue
			}
			// Discovery-implied entities (device.<spec> node/attribute entities, Conceptual_DeviceEntities.go)
			// are created by the MQTT discovery coordinator, which sets their icon/device_class in its
			// own discovery payload -- but HA names such an entity "<device name> <entity name>" (e.g.
			// "house/kitchen/netatmo humidity"), so they get a friendly_name of their own here, and only
			// that (PROJECT.md item 17e, 2026-10-26): "<sphere>/<position path>" like every other entity.
			// Node entities with a representative have their customization emitted by
			// generateTemplateBinarySensors (possibly under a renamed entity ID such as _light_node).
			if strings.HasSuffix(rec.Identity.Path, "/node") && resolveNodeRepresentative(rec.Name, rec, admin) != "" {
				continue
			}
			// No_collect lights are Zigbee group members; the group entity itself is
			// customised by generateLightGroups, so individual bulbs are skipped here.
			if rec.NoCollect && rec.Identity.Domain == "light" {
				continue
			}
			entityID := toHomeAssistantEntityID(rec.Name)
			if entityID == "" || seen[entityID] {
				continue
			}
			seen[entityID] = true

			displayName := rec.Name[len(rec.Identity.Domain)+1:] // strip "domain."
			icon := ""
			if !rec.DiscoveryImplied {
				icon = iconForEntity(rec, admin.CapabilityDefaults)
			}
			content := buildCustomizationYAML(entityID, displayName, icon)
			if native[entityID] && rec.Identity.Path != "" {
				if deviceClass, _, stateClass, _ := resolveCapabilityDefaults(admin.CapabilityDefaults, rec.Identity.Domain, rec.Identity.Path); deviceClass != "" || stateClass != "" {
					if deviceClass != "" {
						content += "  device_class: " + deviceClass + "\n"
					}
					if stateClass != "" {
						content += "  state_class: " + stateClass + "\n"
					}
				}
			}
			dir := filepath.Join(outputDir, "customization", rec.Identity.Domain, rec.Identity.Sphere)
			if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
				return err
			}
		}
	}
	return nil
}

func buildCustomizationYAML(entityID, displayName, icon string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString(entityID + ":\n")
	if icon != "" {
		sb.WriteString("  icon: " + icon + "\n")
	}
	sb.WriteString("  friendly_name: " + displayName + "\n")
	return sb.String()
}

// --- media switch entities ---

// generateMediaSwitches writes entities/template/switch/<sphere>/<id>.yaml for media switches.
// Leaf switches control individual media_player entities; aggregate switches group leaves and
// child-space aggregate switches per space.
func generateMediaSwitches(outputDir string, admin *TAdministrationState) error {
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain != "switch" || !strings.HasSuffix(rec.Identity.Path, "/media") {
				continue
			}
			if rec.MediaSwitchPlayerName == "" {
				continue
			}
			entityID := toHomeAssistantEntityID(rec.Name)
			if entityID == "" {
				continue
			}
			displayName := rec.Name[len("switch."):]
			playerEntityID := toHomeAssistantEntityID(rec.MediaSwitchPlayerName)
			content := buildLeafMediaSwitchYAML(entityID, displayName, playerEntityID, rec.MediaSwitchNoPlayInput, rec.MediaSwitchAvailabilityEntityID)
			dir := filepath.Join(outputDir, "entities", "template", "switch", rec.Identity.Sphere)
			if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
				return err
			}
		}
	}

	// wrapperOfPlayer maps each media player's entity_id to the leaf media switch actually
	// positioned for it, which need not be named after the player: a space's single player's
	// switch may carry the space's own media switch name and stand for the space (PROJECT.md 17g,
	// 2026-10-26).
	wrapperOfPlayer := map[string]string{}
	for _, records := range admin.EntityRecordsBySpace {
		for _, rec := range records {
			if rec.Identity.Domain == "switch" && rec.MediaSwitchPlayerName != "" {
				wrapperOfPlayer[rec.MediaSwitchPlayerName] = rec.Name
			}
		}
	}

	// Aggregate media switches: for each space that has media players or child media aggregates.
	for _, spaceName := range admin.SpaceOrder {
		if spaceName == "root" {
			continue
		}
		leafPlayers := admin.SpaceMediaByName[spaceName]
		var childMediaSwitches []string
		for _, item := range admin.SpaceOffByName[spaceName] {
			if strings.HasSuffix(item, "/media") {
				childMediaSwitches = append(childMediaSwitches, item)
			}
		}
		if len(leafPlayers) == 0 && len(childMediaSwitches) == 0 {
			continue
		}

		entityName := "switch." + spaceName + "/media"
		entityID := toHomeAssistantEntityID(entityName)
		if entityID == "" {
			continue
		}
		identity := extractEntityIdentity(entityName)
		if identity.Sphere == "" {
			continue
		}

		// Derive the turn_off list: leaf media switches + child aggregate media switches.
		var offItems []string
		for _, player := range sortedCopy(leafPlayers) {
			// Build leaf switch name: strip "media_player." and this space's own exact "sphere/path/"
			// prefix, keeping the player's own FULL remaining sub-path as its shortname within the
			// space -- real bug found live 2026-09-30 (Junglinster's "sonos_headphones", positioned
			// two segments deep as "sonos/headphones" rather than directly under the space like every
			// other media player): the old code took only the LAST segment ("headphones"), silently
			// dropping "sonos/" and producing a switch name ("switch.house/headphones/media") that was
			// never actually generated, instead of the real one ("switch.house/sonos/headphones/media").
			// That had worked by coincidence for every single-segment player (e.g. "garage/sonos"),
			// where the last segment and the full sub-path are the same string.
			if wrapper, positioned := wrapperOfPlayer[toHomeAssistantEntityID(player)]; positioned {
				offItems = append(offItems, wrapper)
				continue
			}
			playerPath := strings.TrimPrefix(player[len("media_player."):], spaceName+"/")
			switchName := "switch." + spaceName + "/" + playerPath + "/media"
			offItems = append(offItems, switchName)
		}
		offItems = append(offItems, sortedCopy(childMediaSwitches)...)

		displayName := entityName[len("switch."):]
		if !recordSpaceAggregate(admin, entityID, entityNamesToIDs(offItems)) {
			continue
		}
		content := buildAggregateMediaSwitchYAML(entityID, displayName, offItems, admin.SpaceAvailabilityAll[spaceName])
		dir := filepath.Join(outputDir, "entities", "template", "switch", identity.Sphere)
		if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
			return err
		}
	}

	// Sphere-level media switches: derived from root's accumulated off list.
	sphereMediaItems := map[string][]string{}
	for _, entry := range admin.SpaceOffByName["root"] {
		if !strings.HasSuffix(entry, "/media") {
			continue
		}
		identity := extractEntityIdentity(entry)
		if identity.Sphere != "" {
			sphereMediaItems[identity.Sphere] = append(sphereMediaItems[identity.Sphere], entry)
		}
	}
	for _, sphere := range sortedKeys(sphereMediaItems) {
		offItems := sortedCopy(sphereMediaItems[sphere])
		entityID := toHomeAssistantEntityID("switch." + sphere + "/media")
		displayName := sphere + "/media"
		if !recordSpaceAggregate(admin, entityID, entityNamesToIDs(offItems)) {
			continue
		}
		content := buildAggregateMediaSwitchYAML(entityID, displayName, offItems, false)
		dir := filepath.Join(outputDir, "entities", "template", "switch", sphere)
		if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
			return err
		}
		custom := buildCustomizationYAML(entityID, displayName, "mdi:monitor-speaker")
		customDir := filepath.Join(outputDir, "customization", "switch", sphere)
		if err := writeYAMLFile(filepath.Join(customDir, entityID+".yaml"), custom); err != nil {
			return err
		}
	}

	return nil
}

// availabilityEntityID (2026-09-16, the logical layer's own "switch.media" coercion capability)
// is "" for every pre-existing caller (the hand-written "media_switch" directive never had an
// availability concept of its own) -- when non-empty, adds an "availability:" field gating the
// switch on that entity's own boolean state ("on"), the same convention this codebase's other
// generated switches already use to gate on a sibling "node" capability.
func buildLeafMediaSwitchYAML(entityID, displayName, playerID, noPlayInput, availabilityEntityID string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("switch:\n")
	sb.WriteString("- name: " + displayName + "\n")
	sb.WriteString("  unique_id: " + generatedUniqueID(entityID) + "\n")
	sb.WriteString("  default_entity_id: " + entityID + "\n")
	if availabilityEntityID != "" {
		sb.WriteString("  availability: \"{{ is_state('" + availabilityEntityID + "', 'on') }}\"\n")
	}
	// No turn_on: switching media "on" means nothing (its template switch's turn_on is optional;
	// this replaces the bash-era dummy input_boolean, 2026-10-26).
	sb.WriteString("  turn_off:\n")
	sb.WriteString("  - entity_id:\n")
	sb.WriteString("    - " + playerID + "\n")
	sb.WriteString("    action: media_player.media_stop\n")
	sb.WriteString("  state: >-\n")
	sb.WriteString("      {{\n")
	if noPlayInput != "" {
		sb.WriteString("        (\n")
		sb.WriteString("          is_state('" + playerID + "', 'on')\n")
		sb.WriteString("        or\n")
		sb.WriteString("          (\n")
		sb.WriteString("            is_state('" + playerID + "', 'playing')\n")
		sb.WriteString("          and\n")
		sb.WriteString("            not is_state_attr('" + playerID + "', 'source', '" + noPlayInput + "')\n")
		sb.WriteString("          )\n")
		sb.WriteString("        )\n")
	} else {
		sb.WriteString("        is_state('" + playerID + "', 'on')\n")
		sb.WriteString("      or\n")
		sb.WriteString("        is_state('" + playerID + "', 'playing')\n")
	}
	sb.WriteString("      }}\n")
	return sb.String()
}

func buildAggregateMediaSwitchYAML(entityID, displayName string, offItems []string, availabilityAll bool) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("switch:\n")
	sb.WriteString("- name: " + displayName + "\n")
	sb.WriteString("  unique_id: " + generatedUniqueID(entityID) + "\n")
	sb.WriteString("  default_entity_id: " + entityID + "\n")
	sb.WriteString("  icon: mdi:monitor-speaker\n")
	appendAvailability(&sb, aggregateAvailability(entityNamesToIDs(offItems), availabilityAll))
	// No turn_on: switching media "on" means nothing (its template switch's turn_on is optional;
	// this replaces the bash-era dummy input_boolean, 2026-10-26).
	sb.WriteString("  turn_off:\n")
	for _, item := range offItems {
		itemID := toHomeAssistantEntityID(item)
		sb.WriteString("  - entity_id:\n")
		sb.WriteString("    - " + itemID + "\n")
		sb.WriteString("    action: switch.turn_off\n")
	}
	appendStateBlock(&sb, offItems)
	return sb.String()
}

var heatingPresets = []string{"around", "away", "day", "night"}

// generateHeatingScripts emits set_heating_to_<preset> scripts that cascade from the
// sphere root down to each space that owns a temperature_desired input_number entity.
// The leaf script calls input_number.set_value; ancestor scripts delegate via script.turn_on.
func generateHeatingScripts(outputDir string, admin *TAdministrationState) error {
	// leafSpaces: spaces that own a temperature_desired input_number.
	leafSpaces := map[string]bool{}
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain == "input_number" &&
				strings.HasSuffix(rec.Identity.Path, "/temperature_desired") {
				leafSpaces[spaceName] = true
			}
		}
	}
	if len(leafSpaces) == 0 {
		return nil
	}

	// Propagate up to mark all ancestor spaces.
	heatingSpaces := map[string]bool{}
	for spaceName := range leafSpaces {
		heatingSpaces[spaceName] = true
		parts := strings.Split(spaceName, "/")
		for i := 1; i < len(parts); i++ {
			heatingSpaces[strings.Join(parts[:i], "/")] = true
		}
	}

	for _, spaceName := range admin.SpaceOrder {
		if !heatingSpaces[spaceName] {
			continue
		}
		sphere := spaceName
		if idx := strings.Index(spaceName, "/"); idx >= 0 {
			sphere = spaceName[:idx]
		}
		spaceUnder := strings.ReplaceAll(spaceName, "/", "_")
		scriptDir := filepath.Join(outputDir, "script", sphere)

		for _, preset := range heatingPresets {
			scriptID := spaceUnder + "_set_heating_to_" + preset
			alias := strings.ReplaceAll(spaceName, "/", "/") + "/set_heating_to_" + preset

			var sb strings.Builder
			sb.WriteString(generatorHeader)
			sb.WriteString(scriptID + ":\n")
			sb.WriteString("  alias: " + alias + "\n")
			sb.WriteString("  mode: queued\n")
			sb.WriteString("  sequence:\n")

			if leafSpaces[spaceName] {
				// Leaf: set temperature_desired from the preset input_number.
				tempDesiredID := spaceUnder + "_temperature_desired"
				presetID := spaceUnder + "_heating_target_preset_for_" + preset
				sb.WriteString("  - service: input_number.set_value\n")
				sb.WriteString("    data:\n")
				sb.WriteString("      entity_id: input_number." + tempDesiredID + "\n")
				sb.WriteString("      value: \"{{ states('input_number." + presetID + "') }}\"\n")
			} else {
				// Non-leaf: call each child heating space's script.
				for _, child := range directChildSpaces(spaceName, admin.SpaceOrder) {
					if heatingSpaces[child] {
						childUnder := strings.ReplaceAll(child, "/", "_")
						sb.WriteString("  - service: script.turn_on\n")
						sb.WriteString("    entity_id: script." + childUnder + "_set_heating_to_" + preset + "\n")
					}
				}
			}

			if err := writeYAMLFile(filepath.Join(scriptDir, "script."+scriptID+".yaml"), sb.String()); err != nil {
				return err
			}
		}
	}

	// Sphere-level scripts: the sphere root (e.g. "social") is not in SpaceOrder,
	// so handle it separately by collecting direct child heating spaces.
	sphereChildren := map[string][]string{}
	for _, spaceName := range admin.SpaceOrder {
		if !heatingSpaces[spaceName] {
			continue
		}
		if strings.Count(spaceName, "/") != 1 {
			continue
		}
		sphere := spaceName[:strings.Index(spaceName, "/")]
		sphereChildren[sphere] = append(sphereChildren[sphere], spaceName)
	}
	for _, sphere := range sortedKeys(sphereChildren) {
		scriptDir := filepath.Join(outputDir, "script", sphere)
		for _, preset := range heatingPresets {
			rollupID := sphere + "_set_heating_to_" + preset
			var sb strings.Builder
			sb.WriteString(generatorHeader)
			sb.WriteString(rollupID + ":\n")
			sb.WriteString("  alias: " + sphere + "/set_heating_to_" + preset + "\n")
			sb.WriteString("  mode: queued\n")
			sb.WriteString("  sequence:\n")
			for _, child := range sphereChildren[sphere] {
				childUnder := strings.ReplaceAll(child, "/", "_")
				sb.WriteString("  - service: script.turn_on\n")
				sb.WriteString("    entity_id: script." + childUnder + "_set_heating_to_" + preset + "\n")
			}
			if err := writeYAMLFile(filepath.Join(scriptDir, "script."+rollupID+".yaml"), sb.String()); err != nil {
				return err
			}
		}
	}

	return nil
}

// --- cover away-behaviour entities and scripts ---

// coverAwayPathSuffix returns the "cover/<path>" segment shared by every away-behaviour
// entity/script id and display name for a given cover record.
func coverAwayPathSuffix(rec TEntityRecord) string {
	return "cover/" + rec.Identity.Path
}

// generateCoverAwayEntities writes, for every cover entity, two input_numbers holding the
// positions to move to while away (closed_position_when_away and its mirror,
// opened_position_when_away), and an input_select letting the cover opt out of the
// default away behaviour (Follow/Open/Closed/Ignore).
//
// None of the three declare an "initial:" value (real bug found live 2026-09-21, present since
// 2026-08-19 -- reported by the user as "does_when_away resets after restart", confirmed to
// actually hit all three entities, on EVERY restart or homeassistant.reload_all, not just
// restarts, since this project's own meta-reload mechanism calls that service on every deploy).
// Per Home Assistant's own input_number/input_select source (homeassistant/components/
// input_number|input_select/__init__.py): async_added_to_hass only ever attempts its own
// RestoreEntity lookup when the entity's value/option is still None going in -- __init__ sets it
// immediately from config["initial"] when present, so a configured "initial:" permanently
// disables restore, not just on first setup. Dropping it here lets a user's own choice survive
// every future reload/restart, matching how these helpers are meant to behave.
//
// The one real trade-off: without "initial:", a genuinely NEW entity (never seen before, nothing
// to restore) falls back to each domain's own default -- input_select uses options[0], input_number
// uses its own configured min. options is ordered "Ignore" first specifically so a brand-new
// cover's does_when_away starts on the safe do-nothing choice, same as before this fix. The two
// input_numbers have no equivalent "reorder" trick (opened_position_when_away's own sensible new-
// entity default would be its max, 100, not its min, 0) -- left as min-on-first-setup regardless,
// since it's operationally inert either way: does_when_away already defaults to "Ignore" for that
// same new cover, so nothing ever reads these two positions until a user deliberately switches
// away from "Ignore" -- at which point they'd naturally set/check the positions too.
func generateCoverAwayEntities(outputDir string, admin *TAdministrationState) error {
	for _, rec := range controlledCovers(admin) {
		sphere := rec.Identity.Sphere
		pathSuffix := coverAwayPathSuffix(rec)

		for _, name := range []string{"closed_position_when_away", "opened_position_when_away"} {
			positionID := toHomeAssistantEntityID("input_number." + sphere + "/" + pathSuffix + "/" + name)
			positionKey := strings.TrimPrefix(positionID, "input_number.")
			positionDisplay := sphere + "/" + pathSuffix + "/" + name
			positionContent := buildInputNumberYAML(positionKey, positionDisplay, "0", "100", "1", "\"%\"", "mdi:blinds-horizontal", "")
			numberDir := filepath.Join(outputDir, "entities", "input_number", sphere)
			if err := writeYAMLFile(filepath.Join(numberDir, positionID+".yaml"), positionContent); err != nil {
				return err
			}
		}

		selectID := toHomeAssistantEntityID("input_select." + sphere + "/" + pathSuffix + "/does_when_away")
		selectKey := strings.TrimPrefix(selectID, "input_select.")
		selectDisplay := pathSuffix
		selectContent := buildInputSelectYAML(selectKey, selectDisplay, []string{"Ignore", "Follow", "Open", "Closed"}, "mdi:airplane", "")
		selectDir := filepath.Join(outputDir, "entities", "input_select", sphere)
		if err := writeYAMLFile(filepath.Join(selectDir, selectID+".yaml"), selectContent); err != nil {
			return err
		}
	}
	return nil
}

// controlledCovers is every cover the generated cover control handles: all cover entities except
// slow-speed shadows (Physical.def's "slow speed ...;", 2026-10-23), which are the same physical
// cover as the entity they shadow -- the control uses the shadow for that cover instead.
func controlledCovers(admin *TAdministrationState) []TEntityRecord {
	shadows := map[string]bool{}
	for _, slow := range admin.SlowSpeedShadows {
		shadows[slow] = true
	}
	var covers []TEntityRecord
	for _, rec := range allEntityRecordsByDomain(admin, "cover") {
		if !shadows[toHomeAssistantEntityID(rec.Name)] {
			covers = append(covers, rec)
		}
	}
	return covers
}

func buildInputSelectYAML(key, displayName string, options []string, icon, initial string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString(key + ":\n")
	sb.WriteString("  name: " + displayName + "\n")
	sb.WriteString("  options:\n")
	for _, option := range options {
		sb.WriteString("  - " + option + "\n")
	}
	if icon != "" {
		sb.WriteString("  icon: " + icon + "\n")
	}
	if initial != "" {
		sb.WriteString("  initial: " + initial + "\n")
	}
	return sb.String()
}

// generateCoverAwayScripts writes a close_while_away/open_while_away script pair for every
// cover entity, plus one close_while_away/open_while_away rollup script per sphere that
// sequences every cover in that sphere. There is no per-space breakdown: a cover's
// does_when_away select lets it opt out (Open: always move to its opened position; Closed:
// always move to its closed position; Ignore: skip the cover entirely) of the default
// Follow behaviour, so per-space granularity is no longer needed.
func generateCoverAwayScripts(outputDir string, admin *TAdministrationState) error {
	covers := controlledCovers(admin)

	coverScriptKeys := map[string]struct{ closeKey, openKey string }{}
	sphereCovers := map[string][]TEntityRecord{}
	spheres := map[string]bool{}

	for _, rec := range covers {
		sphere := rec.Identity.Sphere
		sphereCovers[sphere] = append(sphereCovers[sphere], rec)
		spheres[sphere] = true

		coverID := toHomeAssistantEntityID(rec.Name)
		if slow, hasSlow := admin.SlowSpeedShadows[coverID]; hasSlow {
			coverID = slow // the same cover, moving at slow speed
		}
		selectID := toHomeAssistantEntityID("input_select." + sphere + "/" + coverAwayPathSuffix(rec) + "/does_when_away")
		closedPositionID := toHomeAssistantEntityID("input_number." + sphere + "/" + coverAwayPathSuffix(rec) + "/closed_position_when_away")
		openedPositionID := toHomeAssistantEntityID("input_number." + sphere + "/" + coverAwayPathSuffix(rec) + "/opened_position_when_away")

		under := sphere + "_cover_" + strings.ReplaceAll(rec.Identity.Path, "/", "_")
		alias := sphere + "/" + coverAwayPathSuffix(rec)
		scriptDir := filepath.Join(outputDir, "script", sphere)

		closeKey := under + "_close_while_away"
		closeContent := buildCoverAwayScriptYAML(closeKey, alias+"/close_while_away", coverID, selectID, closedPositionID, openedPositionID, "Open")
		if err := writeYAMLFile(filepath.Join(scriptDir, "script."+closeKey+".yaml"), closeContent); err != nil {
			return err
		}

		openKey := under + "_open_while_away"
		openContent := buildCoverAwayScriptYAML(openKey, alias+"/open_while_away", coverID, selectID, closedPositionID, openedPositionID, "Closed")
		if err := writeYAMLFile(filepath.Join(scriptDir, "script."+openKey+".yaml"), openContent); err != nil {
			return err
		}

		coverScriptKeys[rec.Name] = struct{ closeKey, openKey string }{closeKey, openKey}
	}

	for _, sphere := range sortedStringSlice(spheres) {
		scriptDir := filepath.Join(outputDir, "script", sphere)
		customDir := filepath.Join(outputDir, "customization", "script", sphere)

		rollupClose := sphere + "_cover_close_while_away"
		rollupOpen := sphere + "_cover_open_while_away"
		var closeSeq, openSeq []struct{ service, entityID string }
		for _, rec := range sphereCovers[sphere] {
			keys := coverScriptKeys[rec.Name]
			closeSeq = append(closeSeq, struct{ service, entityID string }{"script.turn_on", "script." + keys.closeKey})
			openSeq = append(openSeq, struct{ service, entityID string }{"script.turn_on", "script." + keys.openKey})
		}

		if err := writeYAMLFile(filepath.Join(scriptDir, "script."+rollupClose+".yaml"),
			buildCoverAwayRollupYAML(rollupClose, sphere+"/cover/close_while_away", closeSeq)); err != nil {
			return err
		}
		if err := writeYAMLFile(filepath.Join(customDir, "script."+rollupClose+".yaml"),
			buildCustomizationYAML("script."+rollupClose, sphere+"/cover/close_while_away", "mdi:triangle-down")); err != nil {
			return err
		}

		if err := writeYAMLFile(filepath.Join(scriptDir, "script."+rollupOpen+".yaml"),
			buildCoverAwayRollupYAML(rollupOpen, sphere+"/cover/open_while_away", openSeq)); err != nil {
			return err
		}
		if err := writeYAMLFile(filepath.Join(customDir, "script."+rollupOpen+".yaml"),
			buildCustomizationYAML("script."+rollupOpen, sphere+"/cover/open_while_away", "mdi:triangle")); err != nil {
			return err
		}
	}
	return nil
}

// buildCoverAwayScriptYAML builds a per-cover close/open_while_away script. overrideState is
// the does_when_away option that flips the default action: "Open" makes close_while_away move
// to the opened position instead of the closed one; "Closed" makes open_while_away move to the
// closed position instead of the opened one. Regardless of branch, "Ignore" short-circuits the
// whole script via a leading condition action, leaving the cover untouched -- this guards both
// direct calls to this script and the sphere-level rollup that calls it via script.turn_on.
func buildCoverAwayScriptYAML(key, alias, coverID, selectID, closedPositionID, openedPositionID, overrideState string) string {
	openAction := "  - service: cover.set_cover_position\n    entity_id: " + coverID +
		"\n    data:\n      position: \"{{ states('" + openedPositionID + "') | int }}\"\n"
	closeAction := "  - service: cover.set_cover_position\n    entity_id: " + coverID +
		"\n    data:\n      position: \"{{ states('" + closedPositionID + "') | int }}\"\n"

	thenAction, elseAction := closeAction, openAction
	if overrideState == "Open" {
		thenAction, elseAction = openAction, closeAction
	}

	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString(key + ":\n")
	sb.WriteString("  alias: " + alias + "\n")
	sb.WriteString("  mode: queued\n")
	sb.WriteString("  sequence:\n")
	sb.WriteString("  - condition: not\n")
	sb.WriteString("    conditions:\n")
	sb.WriteString("    - condition: state\n")
	sb.WriteString("      entity_id: " + selectID + "\n")
	sb.WriteString("      state: Ignore\n")
	sb.WriteString("  - if:\n")
	sb.WriteString("    - condition: state\n")
	sb.WriteString("      entity_id: " + selectID + "\n")
	sb.WriteString("      state: " + overrideState + "\n")
	sb.WriteString("    then:\n")
	sb.WriteString(indentYAMLLines(thenAction, "    "))
	sb.WriteString("    else:\n")
	sb.WriteString(indentYAMLLines(elseAction, "    "))
	return sb.String()
}

// indentYAMLLines prefixes every line of block with the given indent, preserving existing
// relative indentation between lines.
func indentYAMLLines(block, indent string) string {
	lines := strings.Split(strings.TrimSuffix(block, "\n"), "\n")
	var sb strings.Builder
	for _, line := range lines {
		sb.WriteString(indent + line + "\n")
	}
	return sb.String()
}

func buildCoverAwayRollupYAML(key, alias string, seq []struct{ service, entityID string }) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString(key + ":\n")
	sb.WriteString("  alias: " + alias + "\n")
	sb.WriteString("  mode: queued\n")
	sb.WriteString("  sequence:\n")
	for _, item := range seq {
		sb.WriteString("  - service: " + item.service + "\n")
		sb.WriteString("    entity_id: " + item.entityID + "\n")
	}
	return sb.String()
}

// --- derived binary sensors (heating, time-window, cover) ---

func generateDerivedBinarySensors(outputDir string, admin *TAdministrationState) error {
	if err := generateHeatingSetToSensors(outputDir, admin); err != nil {
		return err
	}
	if err := generateHeatingLeakageSensors(outputDir, admin); err != nil {
		return err
	}
	if err := generateHeatingShouldBeOffSensors(outputDir, admin); err != nil {
		return err
	}
	if err := generateTimeWindowBinarySensors(outputDir, admin); err != nil {
		return err
	}
	return generateCoverBinarySensors(outputDir, admin)
}

// generateCoverBinarySensors writes the sphere-level auto_control and should_be_closed binary
// sensors that drive the automatic (windy/daylight/sunny-aware) cover control automations.  This
// is independent of the per-cover away-behaviour scripts: should_be_closed decides *whether* an
// automatic sweep should run right now, the away scripts decide *what* each individual cover does
// when it runs.
func generateCoverBinarySensors(outputDir string, admin *TAdministrationState) error {
	spheres := map[string]bool{}
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain == "input_boolean" &&
				strings.HasSuffix(rec.Identity.Path, "covers/auto_control") {
				spheres[rec.Identity.Sphere] = true
			}
		}
	}

	for _, sphere := range sortedStringSlice(spheres) {
		dir := filepath.Join(outputDir, "entities", "template", "binary_sensor", sphere)
		customDir := filepath.Join(outputDir, "customization", "binary_sensor", sphere)

		autoCtrlInputID := toHomeAssistantEntityID("input_boolean." + sphere + "/covers/auto_control")
		occupationHomeID := toHomeAssistantEntityID("input_boolean." + sphere + "/occupation/home")

		acID := toHomeAssistantEntityID("binary_sensor." + sphere + "/covers/auto_control")
		acDisplay := sphere + "/covers/auto_control"
		acState := "is_state('" + autoCtrlInputID + "', 'on') and is_state('" + occupationHomeID + "', 'off')"
		if err := writeYAMLFile(filepath.Join(dir, acID+".yaml"), buildConditionBinarySensorYAML(acID, acDisplay, acState, "", "", "", "")); err != nil {
			return err
		}
		if err := writeYAMLFile(filepath.Join(customDir, acID+".yaml"), buildCustomizationYAML(acID, acDisplay, "mdi:home-export-outline")); err != nil {
			return err
		}

		windyID := findBinarySensorBySuffix(admin, sphere, "windy")
		sunnyID := findBinarySensorBySuffix(admin, sphere, "sunny")
		daylightID := findBinarySensorBySuffix(admin, sphere, "daylight")
		coverageTimeID := toHomeAssistantEntityID("binary_sensor." + sphere + "/is_coverage_time")

		// windy/sunny/daylight are each optional: coverage-time is the only unconditional term,
		// daylight and sunny extend it with "or", and windy narrows the whole thing with "and".
		orTerms := []string{"is_state('" + coverageTimeID + "', 'on')"}
		if daylightID != "" {
			orTerms = append(orTerms, "is_state('"+daylightID+"', 'off')")
		}
		if sunnyID != "" {
			orTerms = append(orTerms, "is_state('"+sunnyID+"', 'on')")
		}
		scState := strings.Join(orTerms, " or ")
		if len(orTerms) > 1 {
			scState = "(" + scState + ")"
		}
		if windyID != "" {
			scState += " and is_state('" + windyID + "', 'off')"
		}

		scID := toHomeAssistantEntityID("binary_sensor." + sphere + "/covers/should_be_closed")
		scDisplay := sphere + "/covers/should_be_closed"
		if err := writeYAMLFile(filepath.Join(dir, scID+".yaml"), buildConditionBinarySensorYAML(scID, scDisplay, scState, "", "", "", "")); err != nil {
			return err
		}
		if err := writeYAMLFile(filepath.Join(customDir, scID+".yaml"), buildCustomizationYAML(scID, scDisplay, "mdi:triangle-down")); err != nil {
			return err
		}
	}
	return nil
}

func generateHeatingSetToSensors(outputDir string, admin *TAdministrationState) error {
	presets := []string{"around", "away", "day", "night"}

	leafSpaces := map[string]bool{}
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain == "input_number" &&
				strings.HasSuffix(rec.Identity.Path, "heating_target_preset_for_around") {
				leafSpaces[spaceName] = true
			}
		}
	}
	if len(leafSpaces) == 0 {
		return nil
	}

	heatingSpaces := map[string]bool{}
	for i := len(admin.SpaceOrder) - 1; i >= 0; i-- {
		spaceName := admin.SpaceOrder[i]
		if spaceName == "root" {
			continue
		}
		sphere := spaceName
		if idx := strings.Index(spaceName, "/"); idx > 0 {
			sphere = spaceName[:idx]
		}
		dir := filepath.Join(outputDir, "entities", "template", "binary_sensor", sphere)
		customDir := filepath.Join(outputDir, "customization", "binary_sensor", sphere)

		if leafSpaces[spaceName] {
			desiredID := toHomeAssistantEntityID("input_number." + spaceName + "/temperature_desired")
			for _, preset := range presets {
				presetID := toHomeAssistantEntityID("input_number." + spaceName + "/heating_target_preset_for_" + preset)
				entityID := toHomeAssistantEntityID("binary_sensor." + spaceName + "/heating_set_to_" + preset)
				displayName := spaceName + "/heating_set_to_" + preset
				stateExpr := fmt.Sprintf("(states('%s') | round(1)) == (states('%s') | round(1))", desiredID, presetID)
				if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), buildBinarySensorBlockYAML(entityID, displayName, stateExpr)); err != nil {
					return err
				}
				if err := writeYAMLFile(filepath.Join(customDir, entityID+".yaml"), buildCustomizationYAML(entityID, displayName, "mdi:radiator")); err != nil {
					return err
				}
			}
			heatingSpaces[spaceName] = true
		} else {
			children := directChildSpaces(spaceName, admin.SpaceOrder)
			var heatingChildren []string
			for _, child := range children {
				if heatingSpaces[child] {
					heatingChildren = append(heatingChildren, child)
				}
			}
			if len(heatingChildren) == 0 {
				continue
			}
			for _, preset := range presets {
				var childIDs []string
				for _, child := range heatingChildren {
					childIDs = append(childIDs, toHomeAssistantEntityID("binary_sensor."+child+"/heating_set_to_"+preset))
				}
				entityID := toHomeAssistantEntityID("binary_sensor." + spaceName + "/heating_set_to_" + preset)
				displayName := spaceName + "/heating_set_to_" + preset
				if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), buildBinarySensorORBlockYAML(entityID, displayName, childIDs)); err != nil {
					return err
				}
				if err := writeYAMLFile(filepath.Join(customDir, entityID+".yaml"), buildCustomizationYAML(entityID, displayName, "mdi:radiator")); err != nil {
					return err
				}
			}
			heatingSpaces[spaceName] = true
		}
	}

	// Generate sphere-level sensors: collect direct children of each sphere root.
	sphereChildren := map[string][]string{}
	for spaceName := range heatingSpaces {
		if strings.Count(spaceName, "/") == 1 {
			sphere := spaceName[:strings.Index(spaceName, "/")]
			sphereChildren[sphere] = append(sphereChildren[sphere], spaceName)
		}
	}
	for _, sphere := range sortedKeys(sphereChildren) {
		children := sortedCopy(sphereChildren[sphere])
		dir := filepath.Join(outputDir, "entities", "template", "binary_sensor", sphere)
		customDir := filepath.Join(outputDir, "customization", "binary_sensor", sphere)
		for _, preset := range presets {
			var childIDs []string
			for _, child := range children {
				childIDs = append(childIDs, toHomeAssistantEntityID("binary_sensor."+child+"/heating_set_to_"+preset))
			}
			entityID := toHomeAssistantEntityID("binary_sensor." + sphere + "/heating_set_to_" + preset)
			displayName := sphere + "/heating_set_to_" + preset
			if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), buildBinarySensorORBlockYAML(entityID, displayName, childIDs)); err != nil {
				return err
			}
			if err := writeYAMLFile(filepath.Join(customDir, entityID+".yaml"), buildCustomizationYAML(entityID, displayName, "mdi:radiator")); err != nil {
				return err
			}
		}
	}
	return nil
}

func generateHeatingLeakageSensors(outputDir string, admin *TAdministrationState) error {
	for _, spaceName := range heatingCapableSocialSpaceNames(admin) {
		if len(admin.HeatingLeaksByName[spaceName]) == 0 {
			continue
		}
		contextPath := spaceNameContextPath(spaceName)
		var evidenceName string
		if contextPath == "" {
			evidenceName = "binary_sensor.physical/heating/leakage/evidence"
		} else {
			evidenceName = "binary_sensor.physical/" + contextPath + "/heating/leakage/evidence"
		}
		evidenceID := toHomeAssistantEntityID(evidenceName)

		sphere := spaceName
		if idx := strings.Index(spaceName, "/"); idx > 0 {
			sphere = spaceName[:idx]
		}
		var sensorName string
		if contextPath == "" {
			sensorName = "binary_sensor." + sphere + "/heating/leakage/identified"
		} else {
			sensorName = "binary_sensor." + sphere + "/" + contextPath + "/heating/leakage/identified"
		}
		entityID := toHomeAssistantEntityID(sensorName)
		displayName := sensorName[len("binary_sensor."):]
		stateExpr := "is_state('" + evidenceID + "', 'on')"
		content := buildConditionBinarySensorYAML(entityID, displayName, stateExpr, "", "00:05:00", "", "")
		dir := filepath.Join(outputDir, "entities", "template", "binary_sensor", sphere)
		if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
			return err
		}
		custom := buildCustomizationYAML(entityID, displayName, "mdi:radiator")
		customDir := filepath.Join(outputDir, "customization", "binary_sensor", sphere)
		if err := writeYAMLFile(filepath.Join(customDir, entityID+".yaml"), custom); err != nil {
			return err
		}
	}
	return nil
}

func generateTimeWindowBinarySensors(outputDir string, admin *TAdministrationState) error {
	// Find time-window domains by scanning for input_datetime entities with
	// a path matching <domain>_workday/start_day.
	seen := map[timeWindowKey]bool{}
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain != "input_datetime" {
				continue
			}
			path := rec.Identity.Path
			// Path of the form "<domain>_workday/start_day"
			if !strings.HasSuffix(path, "_workday/start_day") {
				continue
			}
			domain := strings.TrimSuffix(path, "_workday/start_day")
			if domain == "" {
				continue
			}
			seen[timeWindowKey{rec.Identity.Sphere, domain}] = true
		}
	}

	for _, key := range sortedTimeWindowKeys(seen) {
		sphere, domain := key.sphere, key.domain
		workdaySensorID := toHomeAssistantEntityID("binary_sensor." + sphere + "/workday")
		wkdaySDID := toHomeAssistantEntityID("input_datetime." + sphere + "/" + domain + "_workday/start_day")
		wkdaySNID := toHomeAssistantEntityID("input_datetime." + sphere + "/" + domain + "_workday/start_night")
		holidaySDID := toHomeAssistantEntityID("input_datetime." + sphere + "/" + domain + "_holiday/start_day")
		holidaySNID := toHomeAssistantEntityID("input_datetime." + sphere + "/" + domain + "_holiday/start_night")

		dir := filepath.Join(outputDir, "entities", "template", "binary_sensor", sphere)
		customDir := filepath.Join(outputDir, "customization", "binary_sensor", sphere)

		// Write a customization for the workday binary sensor (provided by HA integration).
		workdayDisplay := sphere + "/workday"
		if err := writeYAMLFile(filepath.Join(customDir, workdaySensorID+".yaml"),
			buildCustomizationYAML(workdaySensorID, workdayDisplay, "mdi:office-building-cog")); err != nil {
			return err
		}

		// Day-time sensor name: coverage → is_coverage_time; others → is_<domain>_day_time
		var dayTimeName string
		if domain == "coverage" {
			dayTimeName = "binary_sensor." + sphere + "/is_coverage_time"
		} else {
			dayTimeName = "binary_sensor." + sphere + "/is_" + domain + "_day_time"
		}
		dayTimeID := toHomeAssistantEntityID(dayTimeName)
		dayTimeDisplay := dayTimeName[len("binary_sensor."):]
		dayContent := buildTimeWindowBinarySensorYAML(dayTimeID, dayTimeDisplay, workdaySensorID, wkdaySDID, wkdaySNID, holidaySDID, holidaySNID)
		if err := writeYAMLFile(filepath.Join(dir, dayTimeID+".yaml"), dayContent); err != nil {
			return err
		}
		dayTimeIcon := "mdi:sun-clock"
		if domain == "coverage" {
			dayTimeIcon = "mdi:clock-start"
		}
		if err := writeYAMLFile(filepath.Join(customDir, dayTimeID+".yaml"), buildCustomizationYAML(dayTimeID, dayTimeDisplay, dayTimeIcon)); err != nil {
			return err
		}

		// Heating also gets a complementary night-time sensor.
		if domain == "heating" {
			nightID := toHomeAssistantEntityID("binary_sensor." + sphere + "/is_heating_night_time")
			nightDisplay := sphere + "/is_heating_night_time"
			nightState := "is_state('" + dayTimeID + "', 'off')"
			nightContent := buildConditionBinarySensorYAML(nightID, nightDisplay, nightState, "", "", "", "")
			if err := writeYAMLFile(filepath.Join(dir, nightID+".yaml"), nightContent); err != nil {
				return err
			}
			if err := writeYAMLFile(filepath.Join(customDir, nightID+".yaml"), buildCustomizationYAML(nightID, nightDisplay, "mdi:bed-clock")); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- heating_should_be_off sensor ---

// generateHeatingShouldBeOffSensors generates a binary_sensor.physical_*_heating_should_be_off
// for each space that has heating leaks declared.  The sensor fires when leakage is detected AND
// not ignored, OR when heating is explicitly disabled.
func generateHeatingShouldBeOffSensors(outputDir string, admin *TAdministrationState) error {
	for _, spaceName := range heatingCapableSocialSpaceNames(admin) {
		contextPath := spaceNameContextPath(spaceName) // "apartment/living_room"

		var physEntityName, socialBase string
		if contextPath == "" {
			physEntityName = "binary_sensor.physical/heating/should_be_off"
			socialBase = spaceName // bare sphere
		} else {
			physEntityName = "binary_sensor.physical/" + contextPath + "/heating/should_be_off"
			socialBase = "social/" + contextPath
		}

		socialBaseUnder := strings.ReplaceAll(socialBase, "/", "_")
		leakageIdentifiedID := "binary_sensor." + socialBaseUnder + "_heating_leakage_identified"
		leakageIgnoreID := "input_boolean." + socialBaseUnder + "_heating_leakage_ignore"
		heatingEnableID := "input_boolean." + socialBaseUnder + "_heating_enable"

		entityID := toHomeAssistantEntityID(physEntityName)
		displayName := physEntityName[len("binary_sensor."):]
		if len(admin.HeatingLeaksByName[spaceName]) == 0 {
			leakageIdentifiedID = "" // no doors/windows to watch: only "heating off" counts
		}
		content := buildHeatingShouldBeOffYAML(entityID, displayName, leakageIdentifiedID, leakageIgnoreID, heatingEnableID)
		dir := filepath.Join(outputDir, "entities", "template", "binary_sensor", "physical")
		if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
			return err
		}
		custom := buildCustomizationYAML(entityID, displayName, "mdi:radiator")
		customDir := filepath.Join(outputDir, "customization", "binary_sensor", "physical")
		if err := writeYAMLFile(filepath.Join(customDir, entityID+".yaml"), custom); err != nil {
			return err
		}
	}
	return nil
}

func buildHeatingShouldBeOffYAML(entityID, displayName, leakageIdentifiedID, leakageIgnoreID, heatingEnableID string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("binary_sensor:\n")
	sb.WriteString("- name: " + displayName + "\n")
	sb.WriteString("  unique_id: " + generatedUniqueID(entityID) + "\n")
	if leakageIdentifiedID == "" {
		sb.WriteString("  state: \"{{ is_state('" + heatingEnableID + "', 'off') }}\"\n")
		return sb.String()
	}
	sb.WriteString("  state: >-\n")
	sb.WriteString("    {{\n")
	sb.WriteString("      (\n")
	sb.WriteString("        is_state('" + leakageIdentifiedID + "', 'on')\n")
	sb.WriteString("      and\n")
	sb.WriteString("        is_state('" + leakageIgnoreID + "', 'off')\n")
	sb.WriteString("      )\n")
	sb.WriteString("    or\n")
	sb.WriteString("      is_state('" + heatingEnableID + "', 'off')\n")
	sb.WriteString("    }}\n")
	return sb.String()
}

// --- automation generation ---

// generateAutomations dispatches to per-group automation generators.
func generateAutomations(outputDir string, admin *TAdministrationState) error {
	if err := generateOccupationAutomations(outputDir, admin); err != nil {
		return err
	}
	if err := generateThermostatAutomations(outputDir, admin); err != nil {
		return err
	}
	if err := generateHeatingPresetAutomations(outputDir, admin); err != nil {
		return err
	}
	if err := generateVacuumAutomations(outputDir, admin); err != nil {
		return err
	}
	if err := generateVacuumingTimeWindowAutomations(outputDir, admin); err != nil {
		return err
	}
	if err := generateCoverAutomations(outputDir, admin); err != nil {
		return err
	}
	if err := generateNoMotionAutomations(outputDir, admin); err != nil {
		return err
	}
	if err := generateFollowsAutomations(outputDir, admin); err != nil {
		return err
	}
	if err := generateRuleAutomations(outputDir, admin); err != nil {
		return err
	}
	if err := generateSwitchedDeviceAutomations(outputDir, admin); err != nil {
		return err
	}
	if err := generateTimerLimitsAutomations(outputDir, admin); err != nil {
		return err
	}
	return generateSwitchControlledHeatingContent(outputDir, admin)
}

// writeAutomationFile writes an automation YAML file to <outputDir>/automation/<sphere>/.
func writeAutomationFile(outputDir, sphere, automationID, content string) error {
	dir := filepath.Join(outputDir, "automation", sphere)
	return writeYAMLFile(filepath.Join(dir, "automation."+automationID+".yaml"), content)
}

// allEntityRecordsByDomainSuffix scans all spaces (including root) for entities
// with the given domain whose path ends with the given suffix.
func allEntityRecordsByDomainSuffix(admin *TAdministrationState, domain, suffix string) []TEntityRecord {
	allSpaces := append([]string{"root"}, admin.SpaceOrder...)
	seen := map[string]bool{}
	var result []TEntityRecord
	for _, spaceName := range allSpaces {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain != domain {
				continue
			}
			if strings.HasSuffix(rec.Identity.Path, suffix) && !seen[rec.Name] {
				seen[rec.Name] = true
				result = append(result, rec)
			}
		}
	}
	return result
}

// allEntityRecordsByDomain scans all spaces (including root) for entities with the given domain.
func allEntityRecordsByDomain(admin *TAdministrationState, domain string) []TEntityRecord {
	allSpaces := append([]string{"root"}, admin.SpaceOrder...)
	seen := map[string]bool{}
	var result []TEntityRecord
	for _, spaceName := range allSpaces {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain == domain && !seen[rec.Name] {
				seen[rec.Name] = true
				result = append(result, rec)
			}
		}
	}
	return result
}

// --- occupation mutual-exclusion automations (a) ---

func generateOccupationAutomations(outputDir string, admin *TAdministrationState) error {
	homeByS := map[string]string{}
	aroundByS := map[string]string{}
	awayByS := map[string]string{}

	for _, rec := range allEntityRecordsByDomainSuffix(admin, "input_boolean", "occupation/home") {
		homeByS[rec.Identity.Sphere] = toHomeAssistantEntityID(rec.Name)
	}
	for _, rec := range allEntityRecordsByDomainSuffix(admin, "input_boolean", "occupation/around") {
		aroundByS[rec.Identity.Sphere] = toHomeAssistantEntityID(rec.Name)
	}
	for _, rec := range allEntityRecordsByDomainSuffix(admin, "input_boolean", "occupation/away") {
		awayByS[rec.Identity.Sphere] = toHomeAssistantEntityID(rec.Name)
	}

	spheres := map[string]bool{}
	for sphere := range homeByS {
		if _, ok := aroundByS[sphere]; ok {
			if _, ok := awayByS[sphere]; ok {
				spheres[sphere] = true
			}
		}
	}

	for _, sphere := range sortedStringSlice(spheres) {
		homeID := homeByS[sphere]
		aroundID := aroundByS[sphere]
		awayID := awayByS[sphere]
		_ = sphere

		err1 := writeAutomationFile(outputDir, "physical", "physical_selected_occupation_home",
			buildOccupationMutexAutomation("home", homeID, []string{aroundID, awayID}))
		err2 := writeAutomationFile(outputDir, "physical", "physical_selected_occupation_around",
			buildOccupationMutexAutomation("around", aroundID, []string{homeID, awayID}))
		err3 := writeAutomationFile(outputDir, "physical", "physical_selected_occupation_away",
			buildOccupationMutexAutomation("away", awayID, []string{homeID, aroundID}))
		err4 := writeAutomationFile(outputDir, "physical", "physical_selected_occupation_equalize",
			buildOccupationEqualizeAutomation(homeID, aroundID, awayID))
		for _, err := range []error{err1, err2, err3, err4} {
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func buildOccupationMutexAutomation(preset, triggerID string, turnOffIDs []string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- alias: physical/selected_occupation_" + preset + "\n")
	sb.WriteString("  id: automation.physical_selected_occupation_" + preset + "\n")
	sb.WriteString("  initial_state: True\n")
	sb.WriteString("  mode: queued\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + triggerID + "\n")
	sb.WriteString("    to: 'on'\n")
	sb.WriteString("  action:\n")
	for _, id := range turnOffIDs {
		sb.WriteString("  - service: input_boolean.turn_off\n")
		sb.WriteString("    entity_id: " + id + "\n")
	}
	return sb.String()
}

func buildOccupationEqualizeAutomation(homeID, aroundID, awayID string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- alias: physical/selected_occupation_equalize\n")
	sb.WriteString("  id: automation.physical_selected_occupation_equalize\n")
	sb.WriteString("  initial_state: True\n")
	sb.WriteString("  mode: queued\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: homeassistant\n")
	sb.WriteString("    event: start\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + aroundID + "\n")
	sb.WriteString("    to: 'off'\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + awayID + "\n")
	sb.WriteString("    to: 'off'\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + homeID + "\n")
	sb.WriteString("    to: 'off'\n")
	sb.WriteString("  condition:\n")
	sb.WriteString("    condition: or\n")
	sb.WriteString("    conditions:\n")
	sb.WriteString("    - condition: and\n")
	sb.WriteString("      conditions:\n")
	sb.WriteString("      - condition: state\n")
	sb.WriteString("        entity_id: " + homeID + "\n")
	sb.WriteString("        state: 'on'\n")
	sb.WriteString("      - condition: state\n")
	sb.WriteString("        entity_id: " + awayID + "\n")
	sb.WriteString("        state: 'on'\n")
	sb.WriteString("    - condition: and\n")
	sb.WriteString("      conditions:\n")
	sb.WriteString("      - condition: state\n")
	sb.WriteString("        entity_id: " + homeID + "\n")
	sb.WriteString("        state: 'on'\n")
	sb.WriteString("      - condition: state\n")
	sb.WriteString("        entity_id: " + aroundID + "\n")
	sb.WriteString("        state: 'on'\n")
	sb.WriteString("    - condition: and\n")
	sb.WriteString("      conditions:\n")
	sb.WriteString("      - condition: state\n")
	sb.WriteString("        entity_id: " + homeID + "\n")
	sb.WriteString("        state: 'off'\n")
	sb.WriteString("      - condition: state\n")
	sb.WriteString("        entity_id: " + aroundID + "\n")
	sb.WriteString("        state: 'off'\n")
	sb.WriteString("      - condition: state\n")
	sb.WriteString("        entity_id: " + awayID + "\n")
	sb.WriteString("        state: 'off'\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: input_boolean.turn_on\n")
	sb.WriteString("    entity_id: " + homeID + "\n")
	sb.WriteString("  - service: input_boolean.turn_off\n")
	sb.WriteString("    entity_id: " + aroundID + "\n")
	sb.WriteString("  - service: input_boolean.turn_off\n")
	sb.WriteString("    entity_id: " + awayID + "\n")
	return sb.String()
}

// --- thermostat control automations (b) ---

func generateThermostatAutomations(outputDir string, admin *TAdministrationState) error {
	// The room a climate device heats is the DSL space it is declared in — not the device's
	// own entity path, which may carry extra segments (e.g. "radiator/aqara_thermostat", or
	// "front/radiator/..." for a room with several radiators sharing one target). Group climate
	// devices by room first, since a room with more than one radiator (e.g. living_room's front
	// and rear units) still shares a single target/desired/leakage set of automations, with every
	// device listed as a trigger and action of the "set to target" automation.
	var roomOrder []string
	climateIDsByRoom := map[string][]string{}
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain != "climate" {
				continue
			}
			physPath := strings.TrimPrefix(spaceName, "social/")
			if _, exists := climateIDsByRoom[physPath]; !exists {
				roomOrder = append(roomOrder, physPath)
			}
			climateIDsByRoom[physPath] = append(climateIDsByRoom[physPath], toHomeAssistantEntityID(rec.Name))
		}
	}

	for _, physPath := range roomOrder {
		physSpaceName := "physical/" + physPath
		physSpaceUnder := strings.ReplaceAll(physSpaceName, "/", "_")
		socialSpaceName := "social/" + physPath
		socialSpaceUnder := strings.ReplaceAll(socialSpaceName, "/", "_")

		targetID := "input_number." + physSpaceUnder + "_temperature_target"
		heatingShouldBeOffID := "binary_sensor." + physSpaceUnder + "_heating_should_be_off"
		desiredID := "input_number." + socialSpaceUnder + "_temperature_desired"
		leakagePresetID := "input_number." + socialSpaceUnder + "_heating_target_preset_for_leakage"

		base := physSpaceUnder
		alias := physSpaceName

		if err := writeAutomationFile(outputDir, "physical", base+"_set_climate_device_to_target_temperature",
			buildSetClimateToTargetYAML(alias, base, climateIDsByRoom[physPath], targetID)); err != nil {
			return err
		}
		if err := writeAutomationFile(outputDir, "physical", base+"_set_target_temperature_to_desired_temperature",
			buildSetTargetToDesiredYAML(alias, base, heatingShouldBeOffID, targetID, desiredID)); err != nil {
			return err
		}
		if err := writeAutomationFile(outputDir, "physical", base+"_set_target_temperature_to_leakage_temperature",
			buildSetTargetToLeakageYAML(alias, base, heatingShouldBeOffID, targetID, leakagePresetID)); err != nil {
			return err
		}
	}
	return nil
}

func buildSetClimateToTargetYAML(aliasPath, idBase string, climateIDs []string, targetID string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- alias: " + aliasPath + "/set_climate_device_to_target_temperature\n")
	sb.WriteString("  id: automation." + idBase + "_set_climate_device_to_target_temperature\n")
	sb.WriteString("  initial_state: True\n")
	sb.WriteString("  mode: queued\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + targetID + "\n")
	for _, climateID := range climateIDs {
		sb.WriteString("  - platform: state\n")
		sb.WriteString("    entity_id: " + climateID + "\n")
	}
	sb.WriteString("  action:\n")
	for _, climateID := range climateIDs {
		sb.WriteString("  - service: climate.set_temperature\n")
		sb.WriteString("    data_template:\n")
		sb.WriteString("      entity_id: " + climateID + "\n")
		sb.WriteString("      temperature: \"{{ states('" + targetID + "') | float(0) }}\"\n")
	}
	return sb.String()
}

func buildSetTargetToDesiredYAML(aliasPath, idBase, heatingShouldBeOffID, targetID, desiredID string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- alias: " + aliasPath + "/set_target_temperature_to_desired_temperature\n")
	sb.WriteString("  id: automation." + idBase + "_set_target_temperature_to_desired_temperature\n")
	sb.WriteString("  initial_state: True\n")
	sb.WriteString("  mode: queued\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + heatingShouldBeOffID + "\n")
	sb.WriteString("    to: 'off'\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + desiredID + "\n")
	sb.WriteString("  - platform: time_pattern\n")
	sb.WriteString("    minutes: '/1'\n")
	sb.WriteString("  condition:\n")
	sb.WriteString("  - condition: state\n")
	sb.WriteString("    entity_id: " + heatingShouldBeOffID + "\n")
	sb.WriteString("    state: 'off'\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: input_number.set_value\n")
	sb.WriteString("    data:\n")
	sb.WriteString("      entity_id: " + targetID + "\n")
	sb.WriteString("      value: \"{{ states('" + desiredID + "') }}\"\n")
	return sb.String()
}

func buildSetTargetToLeakageYAML(aliasPath, idBase, heatingShouldBeOffID, targetID, leakagePresetID string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- alias: " + aliasPath + "/set_target_temperature_to_leakage_temperature\n")
	sb.WriteString("  id: automation." + idBase + "_set_target_temperature_to_leakage_temperature\n")
	sb.WriteString("  initial_state: True\n")
	sb.WriteString("  mode: queued\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + heatingShouldBeOffID + "\n")
	sb.WriteString("    to: 'on'\n")
	sb.WriteString("  - platform: time_pattern\n")
	sb.WriteString("    minutes: '/1'\n")
	sb.WriteString("  condition:\n")
	sb.WriteString("  - condition: state\n")
	sb.WriteString("    entity_id: " + heatingShouldBeOffID + "\n")
	sb.WriteString("    state: 'on'\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: input_number.set_value\n")
	sb.WriteString("    data:\n")
	sb.WriteString("      entity_id: " + targetID + "\n")
	sb.WriteString("      value: \"{{ states('" + leakagePresetID + "') }}\"\n")
	return sb.String()
}

// --- heating preset automations (c) ---

func generateHeatingPresetAutomations(outputDir string, admin *TAdministrationState) error {
	// Detect leaf heating spaces (have temperature_desired input_number).
	leafSpaces := map[string]bool{}
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain == "input_number" &&
				strings.HasSuffix(rec.Identity.Path, "/temperature_desired") {
				leafSpaces[spaceName] = true
			}
		}
	}
	if len(leafSpaces) == 0 {
		return nil
	}

	// Propagate up to mark ancestor spaces.
	heatingSpaces := map[string]bool{}
	for spaceName := range leafSpaces {
		heatingSpaces[spaceName] = true
		parts := strings.Split(spaceName, "/")
		for i := 1; i < len(parts); i++ {
			heatingSpaces[strings.Join(parts[:i], "/")] = true
		}
	}

	// Collect depth-2 heating spaces (direct children of sphere root, exactly 1 slash).
	depth2ByHeatingSphere := map[string][]string{} // sphere → sorted list of depth-2 heating spaces
	for spaceName := range heatingSpaces {
		if strings.Count(spaceName, "/") == 1 {
			sphere := spaceName[:strings.Index(spaceName, "/")]
			depth2ByHeatingSphere[sphere] = append(depth2ByHeatingSphere[sphere], spaceName)
		}
	}
	if len(depth2ByHeatingSphere) == 0 {
		return nil
	}

	// Find occupation and time-window entity IDs (scan root + all spaces).
	homeByS := map[string]string{}
	aroundByS := map[string]string{}
	awayByS := map[string]string{}
	for _, rec := range allEntityRecordsByDomainSuffix(admin, "input_boolean", "occupation/home") {
		homeByS[rec.Identity.Sphere] = toHomeAssistantEntityID(rec.Name)
	}
	for _, rec := range allEntityRecordsByDomainSuffix(admin, "input_boolean", "occupation/around") {
		aroundByS[rec.Identity.Sphere] = toHomeAssistantEntityID(rec.Name)
	}
	for _, rec := range allEntityRecordsByDomainSuffix(admin, "input_boolean", "occupation/away") {
		awayByS[rec.Identity.Sphere] = toHomeAssistantEntityID(rec.Name)
	}

	for sphere, depth2Spaces := range depth2ByHeatingSphere {
		sort.Strings(depth2Spaces)
		homeID := homeByS[sphere]
		aroundID := aroundByS[sphere]
		awayID := awayByS[sphere]
		dayTimeID := "binary_sensor." + sphere + "_is_heating_day_time"
		nightTimeID := "binary_sensor." + sphere + "_is_heating_night_time"

		// Collect script IDs for each preset across all depth-2 heating spaces.
		scriptsByPreset := map[string][]string{}
		for _, d2Space := range depth2Spaces {
			d2Under := strings.ReplaceAll(d2Space, "/", "_")
			for _, preset := range heatingPresets {
				scriptsByPreset[preset] = append(scriptsByPreset[preset], "script."+d2Under+"_set_heating_to_"+preset)
			}
		}

		if aroundID != "" {
			if err := writeAutomationFile(outputDir, "physical", "physical_heating_set_to_around",
				buildHeatingPresetSimpleAutomation("around", aroundID, scriptsByPreset["around"])); err != nil {
				return err
			}
		}
		if awayID != "" {
			if err := writeAutomationFile(outputDir, "physical", "physical_heating_set_to_away",
				buildHeatingPresetSimpleAutomation("away", awayID, scriptsByPreset["away"])); err != nil {
				return err
			}
		}
		if homeID != "" && dayTimeID != "" {
			if err := writeAutomationFile(outputDir, "physical", "physical_heating_set_to_day",
				buildHeatingPresetTimeAutomation("day", homeID, dayTimeID, scriptsByPreset["day"])); err != nil {
				return err
			}
		}
		if homeID != "" && nightTimeID != "" {
			if err := writeAutomationFile(outputDir, "physical", "physical_heating_set_to_night",
				buildHeatingPresetTimeAutomation("night", homeID, nightTimeID, scriptsByPreset["night"])); err != nil {
				return err
			}
		}
	}
	return nil
}

func buildHeatingPresetSimpleAutomation(preset, triggerID string, scriptIDs []string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- alias: physical/heating_set_to_" + preset + "\n")
	sb.WriteString("  id: automation.physical_heating_set_to_" + preset + "\n")
	sb.WriteString("  initial_state: True\n")
	sb.WriteString("  mode: queued\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + triggerID + "\n")
	sb.WriteString("    to: 'on'\n")
	sb.WriteString("  action:\n")
	for _, scriptID := range scriptIDs {
		sb.WriteString("  - service: script.turn_on\n")
		sb.WriteString("    entity_id: " + scriptID + "\n")
	}
	return sb.String()
}

func buildHeatingPresetTimeAutomation(preset, homeID, timeID string, scriptIDs []string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- alias: physical/heating_set_to_" + preset + "\n")
	sb.WriteString("  id: automation.physical_heating_set_to_" + preset + "\n")
	sb.WriteString("  initial_state: True\n")
	sb.WriteString("  mode: queued\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + homeID + "\n")
	sb.WriteString("    to: 'on'\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + timeID + "\n")
	sb.WriteString("    to: 'on'\n")
	sb.WriteString("  condition:\n")
	sb.WriteString("  - condition: and\n")
	sb.WriteString("    conditions:\n")
	sb.WriteString("    - condition: state\n")
	sb.WriteString("      entity_id: " + homeID + "\n")
	sb.WriteString("      state: 'on'\n")
	sb.WriteString("    - condition: state\n")
	sb.WriteString("      entity_id: " + timeID + "\n")
	sb.WriteString("      state: 'on'\n")
	sb.WriteString("  action:\n")
	for _, scriptID := range scriptIDs {
		sb.WriteString("  - service: script.turn_on\n")
		sb.WriteString("    entity_id: " + scriptID + "\n")
	}
	return sb.String()
}

// --- vacuum automations (d) ---

func generateVacuumAutomations(outputDir string, admin *TAdministrationState) error {
	vacuumRecs := allEntityRecordsByDomain(admin, "vacuum")
	if len(vacuumRecs) == 0 {
		return nil
	}

	for _, rec := range vacuumRecs {
		if rec.Identity.Sphere == "" {
			continue
		}
		sphere := rec.Identity.Sphere
		vacuumID := toHomeAssistantEntityID(rec.Name)
		shouldBeActiveID := "input_boolean." + sphere + "_vacuum_should_be_active"
		vacuumingRequestedID := "input_boolean." + sphere + "_vacuuming_requested"

		homeID := ""
		for _, r := range allEntityRecordsByDomainSuffix(admin, "input_boolean", "occupation/home") {
			if r.Identity.Sphere == sphere {
				homeID = toHomeAssistantEntityID(r.Name)
				break
			}
		}

		if err := writeAutomationFile(outputDir, "physical", "physical_vacuum_start",
			buildVacuumStartAutomation(vacuumID, shouldBeActiveID)); err != nil {
			return err
		}
		if err := writeAutomationFile(outputDir, "physical", "physical_vacuum_return_to_base",
			buildVacuumReturnAutomation(vacuumID, shouldBeActiveID)); err != nil {
			return err
		}
		if homeID != "" {
			if err := writeAutomationFile(outputDir, "social", "social_vacuum_start_required",
				buildVacuumStartRequiredAutomation(vacuumingRequestedID, homeID, shouldBeActiveID)); err != nil {
				return err
			}
			if err := writeAutomationFile(outputDir, "social", "social_vacuum_stop_required",
				buildVacuumStopRequiredAutomation(homeID, shouldBeActiveID)); err != nil {
				return err
			}
		}
		if err := writeAutomationFile(outputDir, "social", "social_vacuuming_requested_finished_turn_off",
			buildVacuumingFinishedAutomation(vacuumID, shouldBeActiveID, vacuumingRequestedID)); err != nil {
			return err
		}
	}
	return nil
}

func buildVacuumStartAutomation(vacuumID, shouldBeActiveID string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- alias: physical/vacuum/start\n")
	sb.WriteString("  id: automation.physical_vacuum_start\n")
	sb.WriteString("  initial_state: True\n")
	sb.WriteString("  mode: queued\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + shouldBeActiveID + "\n")
	sb.WriteString("    to: 'on'\n")
	sb.WriteString("  - platform: homeassistant\n")
	sb.WriteString("    event: start\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + vacuumID + "\n")
	sb.WriteString("    from:\n")
	sb.WriteString("      - 'unavailable'\n")
	sb.WriteString("  condition:\n")
	sb.WriteString("  - condition: state\n")
	sb.WriteString("    entity_id: " + shouldBeActiveID + "\n")
	sb.WriteString("    state: 'on'\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: vacuum.start\n")
	sb.WriteString("    entity_id: " + vacuumID + "\n")
	return sb.String()
}

func buildVacuumReturnAutomation(vacuumID, shouldBeActiveID string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- alias: physical/vacuum/return_to_base\n")
	sb.WriteString("  id: automation.physical_vacuum_return_to_base\n")
	sb.WriteString("  initial_state: True\n")
	sb.WriteString("  mode: queued\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + shouldBeActiveID + "\n")
	sb.WriteString("    to: 'off'\n")
	sb.WriteString("  - platform: homeassistant\n")
	sb.WriteString("    event: start\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + vacuumID + "\n")
	sb.WriteString("    from:\n")
	sb.WriteString("      - 'unavailable'\n")
	sb.WriteString("  condition:\n")
	sb.WriteString("  - condition: state\n")
	sb.WriteString("    entity_id: " + shouldBeActiveID + "\n")
	sb.WriteString("    state: 'off'\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: vacuum.pause\n")
	sb.WriteString("    entity_id: " + vacuumID + "\n")
	sb.WriteString("  - service: vacuum.return_to_base\n")
	sb.WriteString("    entity_id: " + vacuumID + "\n")
	return sb.String()
}

func buildVacuumStartRequiredAutomation(vacuumingRequestedID, homeID, shouldBeActiveID string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- alias: social/vacuum/start_required\n")
	sb.WriteString("  id: automation.social_vacuum_start_required\n")
	sb.WriteString("  initial_state: True\n")
	sb.WriteString("  mode: queued\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + vacuumingRequestedID + "\n")
	sb.WriteString("    to: 'on'\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + homeID + "\n")
	sb.WriteString("    to: 'off'\n")
	sb.WriteString("  - platform: homeassistant\n")
	sb.WriteString("    event: start\n")
	sb.WriteString("  condition:\n")
	sb.WriteString("  - condition: state\n")
	sb.WriteString("    entity_id: " + vacuumingRequestedID + "\n")
	sb.WriteString("    state: 'on'\n")
	sb.WriteString("  - condition: state\n")
	sb.WriteString("    entity_id: " + homeID + "\n")
	sb.WriteString("    state: 'off'\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: input_boolean.turn_on\n")
	sb.WriteString("    entity_id: " + shouldBeActiveID + "\n")
	return sb.String()
}

func buildVacuumStopRequiredAutomation(homeID, shouldBeActiveID string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- alias: social/vacuum/stop_required\n")
	sb.WriteString("  id: automation.social_vacuum_stop_required\n")
	sb.WriteString("  initial_state: True\n")
	sb.WriteString("  mode: queued\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + homeID + "\n")
	sb.WriteString("    to: 'on'\n")
	sb.WriteString("  - platform: homeassistant\n")
	sb.WriteString("    event: start\n")
	sb.WriteString("  condition:\n")
	sb.WriteString("  - condition: state\n")
	sb.WriteString("    entity_id: " + homeID + "\n")
	sb.WriteString("    state: 'on'\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: input_boolean.turn_off\n")
	sb.WriteString("    entity_id: " + shouldBeActiveID + "\n")
	return sb.String()
}

func buildVacuumingFinishedAutomation(vacuumID, shouldBeActiveID, vacuumingRequestedID string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- alias: social/vacuuming_requested/finished/turn_off\n")
	sb.WriteString("  id: automation.social_vacuuming_requested_finished_turn_off\n")
	sb.WriteString("  initial_state: True\n")
	sb.WriteString("  mode: queued\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + vacuumID + "\n")
	sb.WriteString("    from:\n")
	sb.WriteString("      - 'cleaning'\n")
	sb.WriteString("      - 'returning'\n")
	sb.WriteString("    to: 'docked'\n")
	sb.WriteString("  condition:\n")
	sb.WriteString("  - condition: state\n")
	sb.WriteString("    entity_id: " + vacuumID + "\n")
	sb.WriteString("    state: 'docked'\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: input_boolean.turn_off\n")
	sb.WriteString("    entity_id: " + shouldBeActiveID + "\n")
	sb.WriteString("  - service: input_boolean.turn_off\n")
	sb.WriteString("    entity_id: " + vacuumingRequestedID + "\n")
	return sb.String()
}

// --- vacuuming time-window automations (e) ---

func generateVacuumingTimeWindowAutomations(outputDir string, admin *TAdministrationState) error {
	seen := map[string]bool{}
	for _, spaceName := range append([]string{"root"}, admin.SpaceOrder...) {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain != "input_datetime" {
				continue
			}
			if strings.HasSuffix(rec.Identity.Path, "vacuuming_workday/start_day") {
				seen[rec.Identity.Sphere] = true
			}
		}
	}

	for _, sphere := range sortedStringSlice(seen) {
		dayTimeID := "binary_sensor." + sphere + "_is_vacuuming_day_time"
		vacuumingRequestedID := "input_boolean." + sphere + "_vacuuming_requested"

		if err := writeAutomationFile(outputDir, "social", "social_vacuuming_requested_time_turn_on",
			buildVacuumingTimeAutomation("on", dayTimeID, vacuumingRequestedID)); err != nil {
			return err
		}
		if err := writeAutomationFile(outputDir, "social", "social_vacuuming_requested_time_turn_off",
			buildVacuumingTimeAutomation("off", dayTimeID, vacuumingRequestedID)); err != nil {
			return err
		}
	}
	return nil
}

func buildVacuumingTimeAutomation(state, dayTimeID, vacuumingRequestedID string) string {
	action := "turn_on"
	if state == "off" {
		action = "turn_off"
	}
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- alias: social/vacuuming_requested/time/turn_" + state + "\n")
	sb.WriteString("  id: automation.social_vacuuming_requested_time_turn_" + state + "\n")
	sb.WriteString("  initial_state: True\n")
	sb.WriteString("  mode: queued\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + dayTimeID + "\n")
	sb.WriteString("    to: '" + state + "'\n")
	sb.WriteString("  - platform: homeassistant\n")
	sb.WriteString("    event: start\n")
	sb.WriteString("  condition:\n")
	sb.WriteString("  - condition: state\n")
	sb.WriteString("    entity_id: " + dayTimeID + "\n")
	sb.WriteString("    state: '" + state + "'\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: input_boolean." + action + "\n")
	sb.WriteString("    entity_id: " + vacuumingRequestedID + "\n")
	return sb.String()
}

// --- cover auto-control automations (f) ---

// generateCoverAutomations wires the sphere-level should_be_closed/auto_control condition to the
// overall away-behaviour rollup scripts: an automatic close/open sweep now runs the same
// close_while_away/open_while_away sequence that a manual "going away" trigger would, so every
// cover's does_when_away override (Open/Closed/Follow) applies equally to both.
func generateCoverAutomations(outputDir string, admin *TAdministrationState) error {
	spheres := map[string]bool{}
	for _, spaceName := range append([]string{"root"}, admin.SpaceOrder...) {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain == "input_boolean" &&
				strings.HasSuffix(rec.Identity.Path, "covers/auto_control") {
				spheres[rec.Identity.Sphere] = true
			}
		}
	}

	for _, sphere := range sortedStringSlice(spheres) {
		autoControlID := "binary_sensor." + sphere + "_covers_auto_control"
		shouldBeClosedID := "binary_sensor." + sphere + "_covers_should_be_closed"
		closeScriptID := "script." + sphere + "_cover_close_while_away"
		openScriptID := "script." + sphere + "_cover_open_while_away"

		if err := writeAutomationFile(outputDir, "social", "social_covers_close",
			buildCoverAutomation("close", autoControlID, shouldBeClosedID, closeScriptID)); err != nil {
			return err
		}
		if err := writeAutomationFile(outputDir, "social", "social_covers_open",
			buildCoverOpenAutomation(autoControlID, shouldBeClosedID, openScriptID)); err != nil {
			return err
		}
	}
	return nil
}

func buildCoverAutomation(action, autoControlID, shouldBeClosedID, scriptID string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- alias: social/covers/" + action + "\n")
	sb.WriteString("  id: automation.social_covers_" + action + "\n")
	sb.WriteString("  initial_state: True\n")
	sb.WriteString("  mode: queued\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + shouldBeClosedID + "\n")
	sb.WriteString("    to: 'on'\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + autoControlID + "\n")
	sb.WriteString("    to: 'on'\n")
	sb.WriteString("  condition:\n")
	sb.WriteString("  - condition: state\n")
	sb.WriteString("    entity_id: " + shouldBeClosedID + "\n")
	sb.WriteString("    state: 'on'\n")
	sb.WriteString("  - condition: state\n")
	sb.WriteString("    entity_id: " + autoControlID + "\n")
	sb.WriteString("    state: 'on'\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: script.turn_on\n")
	sb.WriteString("    entity_id: " + scriptID + "\n")
	return sb.String()
}

func buildCoverOpenAutomation(autoControlID, shouldBeClosedID, scriptID string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- alias: social/covers/open\n")
	sb.WriteString("  id: automation.social_covers_open\n")
	sb.WriteString("  initial_state: True\n")
	sb.WriteString("  mode: queued\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + shouldBeClosedID + "\n")
	sb.WriteString("    to: 'off'\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + autoControlID + "\n")
	sb.WriteString("    to: 'on'\n")
	sb.WriteString("  condition:\n")
	sb.WriteString("  - condition: state\n")
	sb.WriteString("    entity_id: " + shouldBeClosedID + "\n")
	sb.WriteString("    state: 'off'\n")
	sb.WriteString("  - condition: state\n")
	sb.WriteString("    entity_id: " + autoControlID + "\n")
	sb.WriteString("    state: 'on'\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: script.turn_on\n")
	sb.WriteString("    entity_id: " + scriptID + "\n")
	return sb.String()
}

// --- no-motion light-off automations (g) ---

func generateNoMotionAutomations(outputDir string, admin *TAdministrationState) error {
	const ignoreSuffix = "no_motion/ignore"

	for _, rec := range allEntityRecordsByDomainSuffix(admin, "input_boolean", ignoreSuffix) {
		path := rec.Identity.Path
		parentP := strings.TrimSuffix(path, "/"+ignoreSuffix)
		if parentP == path {
			continue // path did not contain the suffix with a leading "/"
		}
		spaceName := rec.Identity.Sphere + "/" + parentP
		spaceUnder := strings.ReplaceAll(spaceName, "/", "_")
		ignoreID := toHomeAssistantEntityID(rec.Name)
		motionID := "binary_sensor." + spaceUnder + "_motion"
		lightID := "light." + spaceUnder

		automationID := spaceUnder + "_no_motion_light_off"
		aliasPath := spaceName + "/no_motion/light_off"

		delay := admin.SpaceNoMotionDelayByName[spaceName]
		if delay == 0 {
			delay = 15
		}

		if err := writeAutomationFile(outputDir, "social", automationID,
			buildNoMotionLightOffAutomation(aliasPath, automationID, ignoreID, motionID, lightID, delay)); err != nil {
			return err
		}
	}
	return nil
}

func buildNoMotionLightOffAutomation(aliasPath, automationID, ignoreID, motionID, lightID string, delayMinutes int) string {
	mins := strconv.Itoa(delayMinutes)
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- alias: " + aliasPath + "\n")
	sb.WriteString("  id: automation." + automationID + "\n")
	sb.WriteString("  initial_state: True\n")
	sb.WriteString("  mode: queued\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("    platform: time_pattern\n")
	sb.WriteString("    minutes: '/1'\n")
	sb.WriteString("  condition:\n")
	sb.WriteString("  - condition: state\n")
	sb.WriteString("    entity_id: " + ignoreID + "\n")
	sb.WriteString("    state: 'off'\n")
	sb.WriteString("  - condition: state\n")
	sb.WriteString("    entity_id: " + motionID + "\n")
	sb.WriteString("    state: 'off'\n")
	sb.WriteString("    for:\n")
	sb.WriteString("      minutes: " + mins + "\n")
	sb.WriteString("  - condition: state\n")
	sb.WriteString("    entity_id: " + lightID + "\n")
	sb.WriteString("    state: 'on'\n")
	sb.WriteString("    for:\n")
	sb.WriteString("      minutes: " + mins + "\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: light.turn_off\n")
	sb.WriteString("    entity_id: " + lightID + "\n")
	return sb.String()
}

// entitySphere extracts the sphere segment from a fully-qualified DSL entity name.
// For "light.social/house/corridor/corb" it returns "social".
func entitySphere(entityName string) string {
	dotIdx := strings.Index(entityName, ".")
	if dotIdx < 0 {
		return ""
	}
	rest := entityName[dotIdx+1:]
	slashIdx := strings.Index(rest, "/")
	if slashIdx < 0 {
		return rest
	}
	return rest[:slashIdx]
}

// buildBinarySensorBlockYAML builds a binary_sensor template YAML using a >- block state
// expression (4-space indented {{ }}).
func buildBinarySensorBlockYAML(entityID, displayName, stateExpr string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("binary_sensor:\n")
	sb.WriteString("- name: " + displayName + "\n")
	sb.WriteString("  unique_id: " + generatedUniqueID(entityID) + "\n")
	sb.WriteString("  state: >-\n")
	sb.WriteString("    {{\n")
	sb.WriteString("      " + stateExpr + "\n")
	sb.WriteString("    }}\n")
	return sb.String()
}

// buildBinarySensorORBlockYAML builds a binary_sensor template YAML with state as an OR
// of is_state checks in a >- block.
func buildBinarySensorORBlockYAML(entityID, displayName string, childIDs []string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("binary_sensor:\n")
	sb.WriteString("- name: " + displayName + "\n")
	sb.WriteString("  unique_id: " + generatedUniqueID(entityID) + "\n")
	sb.WriteString("  state: >-\n")
	sb.WriteString("    {{\n")
	for i, id := range childIDs {
		if i > 0 {
			sb.WriteString("    or\n")
		}
		sb.WriteString("      is_state('" + id + "', 'on')\n")
	}
	sb.WriteString("    }}\n")
	return sb.String()
}

// buildTimeWindowBinarySensorYAML builds the complex time-window state expression
// that checks whether "now" is within a configurable daily start_day/start_night window,
// handling both workday and holiday variants.
func buildTimeWindowBinarySensorYAML(entityID, displayName, workdaySensorID, wkdaySDID, wkdaySNID, holidaySDID, holidaySNID string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("binary_sensor:\n")
	sb.WriteString("- name: " + displayName + "\n")
	sb.WriteString("  unique_id: " + generatedUniqueID(entityID) + "\n")
	sb.WriteString("  state: >-\n")
	sb.WriteString("    {{\n")
	sb.WriteString("      (\n")
	sb.WriteString("        is_state('" + workdaySensorID + "', 'on')\n")
	sb.WriteString("      and\n")
	sb.WriteString("        (\n")
	appendTimeWindowCases(&sb, wkdaySDID, wkdaySNID)
	sb.WriteString("        )\n")
	sb.WriteString("      )\n")
	sb.WriteString("    or\n")
	sb.WriteString("      (\n")
	sb.WriteString("        is_state('" + workdaySensorID + "', 'off')\n")
	sb.WriteString("      and\n")
	sb.WriteString("        (\n")
	appendTimeWindowCases(&sb, holidaySDID, holidaySNID)
	sb.WriteString("        )\n")
	sb.WriteString("      )\n")
	sb.WriteString("    }}\n")
	return sb.String()
}

// appendTimeWindowCases writes the three OR cases for a time window into the state block.
// All three cases detect whether "now" falls within the [startDay, startNight) window,
// accounting for both normal (SD < SN) and wrap-around (SD > SN) configurations.
func appendTimeWindowCases(sb *strings.Builder, startDayID, startNightID string) {
	sd := "today_at(states('" + startDayID + "'))"
	sn := "today_at(states('" + startNightID + "'))"
	sb.WriteString("           (\n")
	sb.WriteString("             (" + sd + " < " + sn + ")\n")
	sb.WriteString("           and\n")
	sb.WriteString("             (" + sd + " < now())\n")
	sb.WriteString("           and\n")
	sb.WriteString("             (now() < " + sn + ")\n")
	sb.WriteString("           )\n")
	sb.WriteString("        or\n")
	sb.WriteString("           (\n")
	sb.WriteString("             (" + sd + " > " + sn + ")\n")
	sb.WriteString("           and\n")
	sb.WriteString("             (" + sd + " < now())\n")
	sb.WriteString("           )\n")
	sb.WriteString("        or\n")
	sb.WriteString("           (\n")
	sb.WriteString("             (" + sd + " > " + sn + ")\n")
	sb.WriteString("           and\n")
	sb.WriteString("             (now() < " + sn + ")\n")
	sb.WriteString("           )\n")
}

// findBinarySensorBySuffix returns the HA entity ID of the first binary_sensor in the
// given sphere whose path ends with the specified suffix segment.
func findBinarySensorBySuffix(admin *TAdministrationState, sphere, suffix string) string {
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain != "binary_sensor" || rec.Identity.Sphere != sphere {
				continue
			}
			seg := lastPathSegment(rec.Identity.Path)
			if seg == suffix {
				return toHomeAssistantEntityID(rec.Name)
			}
		}
	}
	return ""
}

type timeWindowKey struct{ sphere, domain string }

func sortedTimeWindowKeys(m map[timeWindowKey]bool) []timeWindowKey {
	keys := make([]timeWindowKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].sphere != keys[j].sphere {
			return keys[i].sphere < keys[j].sphere
		}
		return keys[i].domain < keys[j].domain
	})
	return keys
}

func sortedStringSlice(m map[string]bool) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}

// --- shared YAML helpers ---

// appendStateBlock writes the Jinja2 state expression as an OR of is_state checks.
func appendStateBlock(sb *strings.Builder, items []string) {
	sb.WriteString("  state: >-\n")
	sb.WriteString("      {{\n")
	for i, item := range items {
		itemID := toHomeAssistantEntityID(item)
		if i > 0 {
			sb.WriteString("      or\n")
		}
		sb.WriteString("        is_state('" + itemID + "', 'on')\n")
		// A playing Sonos is "playing", never "on" -- the same check as the leaf media switches.
		if strings.HasPrefix(item, "media_player.") {
			sb.WriteString("      or\n")
			sb.WriteString("        is_state('" + itemID + "', 'playing')\n")
		}
	}
	sb.WriteString("      }}\n")
}

// domainTurnOnAction returns the HA service call for turning on an entity,
// given its full DSL name (domain extracted from the name).
func domainTurnOnAction(entityName string) string {
	dotIdx := strings.Index(entityName, ".")
	if dotIdx <= 0 {
		return "homeassistant.turn_on"
	}
	domain := entityName[:dotIdx]
	switch domain {
	case "cover":
		return "cover.open"
	default:
		return domain + ".turn_on"
	}
}

// domainTurnOffAction returns the HA service call for turning off an entity,
// given its full DSL name (domain extracted from the name).
func domainTurnOffAction(entityName string) string {
	dotIdx := strings.Index(entityName, ".")
	if dotIdx <= 0 {
		return "homeassistant.turn_off"
	}
	domain := entityName[:dotIdx]
	switch domain {
	case "cover":
		return "cover.close"
	case "media_player":
		// Not every player supports turn_off (Sonos doesn't), and HA aborts the whole turn_off
		// sequence on an unsupported action; switching a space off means stopping its media anyway.
		return "media_player.media_stop"
	default:
		return domain + ".turn_off"
	}
}

// isNoCollectLightSpace returns true when the space is a Zigbee group space: it has at
// least one no_collect light entity whose path extends beyond the space name (i.e. an
// individual bulb, not the derived space aggregate), and no non-no_collect light entities.
// Non-light entities such as infrastructural node sensors are tolerated.
func isNoCollectLightSpace(spaceName string, records []TEntityRecord) bool {
	derivedAggregate := "light." + spaceName
	hasSubLight := false
	for _, rec := range records {
		if rec.Identity.Domain != "light" {
			continue
		}
		if !rec.NoCollect {
			return false
		}
		if rec.Name == derivedAggregate {
			return false // derived space-aggregate is not a sub-light
		}
		hasSubLight = true
	}
	return hasSubLight
}

// spaceNameContextPath extracts the path component from a space name, stripping the
// leading sphere name.  For example "social/apartment/living_room" → "apartment/living_room".
// A bare sphere name like "social" returns "".
func spaceNameContextPath(spaceName string) string {
	slashIdx := strings.Index(spaceName, "/")
	if slashIdx < 0 {
		return ""
	}
	return spaceName[slashIdx+1:]
}

// directChildSpaces returns the registered spaces that are direct children of parent
// (i.e. their name starts with parent+"/" and has exactly one more path segment).
func directChildSpaces(parent string, allSpaces []string) []string {
	parentDepth := strings.Count(parent, "/")
	prefix := parent + "/"
	var children []string
	for _, s := range allSpaces {
		if strings.HasPrefix(s, prefix) && strings.Count(s, "/") == parentDepth+1 {
			children = append(children, s)
		}
	}
	return children
}

// entityNamesToIDs converts a slice of full DSL entity names to HA entity IDs,
// dropping any that map to an empty string.
func entityNamesToIDs(names []string) []string {
	ids := make([]string, 0, len(names))
	for _, name := range names {
		if id := toHomeAssistantEntityID(name); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// sortedKeys returns the map keys in sorted order.
func sortedCopy(items []string) []string {
	cp := append([]string{}, items...)
	sort.Strings(cp)
	return cp
}

func deduplicated(items []string) []string {
	seen := map[string]bool{}
	result := items[:0:0]
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// removeGeneratedFiles deletes every YAML file under root whose first line is the
// generator header, then removes any directories that became empty as a result.
// writeYAMLFile creates all parent directories and writes content to path.
func writeYAMLFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("cannot create directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("cannot write %s: %w", path, err)
	}
	return nil
}

// --- follows automations ---

// buildStateTriggerAutomationYAML renders one "entity X changes to triggerState -> call
// actionDomain.turn_<actionState> on actionEntity" automation, exactly the shape both
// generateFollowsAutomations and generateWhenRuleAutomations need -- a "follows <follower>
// <leader>;" directive really is just permanent sugar for two of these (see TWhenRule's own doc
// comment), and sharing this one builder is what makes that "sugar for" relationship real at the
// implementation level rather than just a comment. triggerState/actionState are "on"/"off".
func buildStateTriggerAutomationYAML(aliasPath, automationID, triggerEntity, triggerState, actionDomain, actionEntity, actionState string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- alias: " + aliasPath + "\n")
	sb.WriteString("  id: automation." + automationID + "\n")
	sb.WriteString("  initial_state: True\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id: " + triggerEntity + "\n")
	sb.WriteString("    to: '" + triggerState + "'\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: " + actionDomain + ".turn_" + actionState + "\n")
	sb.WriteString("    entity_id: " + actionEntity + "\n")
	return sb.String()
}

// generateFollowsAutomations names each pair "<sphere1>/<path1>/<domain1>/follows/turn_on|off/
// <sphere2>/<path2>/<domain2>" (2026-09-29, replacing the earlier "<followerPath>/follows/
// <leaderDomain>/<leaderPath>/turn_on|off" shape) -- symmetric in follower/leader (each contributes
// its own full "sphere/path/domain" triple), matching the naming the user settled on for the new
// named "rule" construct's own sibling automations (generateWhenRuleAutomations).
func generateFollowsAutomations(outputDir string, admin *TAdministrationState) error {
	for _, rel := range admin.FollowsRelations {
		followerID := toHomeAssistantEntityID(rel.Follower)
		leaderID := toHomeAssistantEntityID(rel.Leader)

		followerPath := rel.Follower[strings.Index(rel.Follower, ".")+1:]
		followerDomain := rel.Follower[:strings.Index(rel.Follower, ".")]
		leaderPath := rel.Leader[strings.Index(rel.Leader, ".")+1:]
		leaderDomain := rel.Leader[:strings.Index(rel.Leader, ".")]

		sphere := entitySphere(rel.Follower)

		for _, dir := range []string{"turn_on", "turn_off"} {
			state := strings.TrimPrefix(dir, "turn_")
			aliasPath := followerPath + "/" + followerDomain + "/follows/" + dir + "/" + leaderPath + "/" + leaderDomain
			automationID := strings.ReplaceAll(aliasPath, "/", "_")

			content := buildStateTriggerAutomationYAML(aliasPath, automationID, leaderID, state, leaderDomain, followerID, state)
			if err := writeAutomationFile(outputDir, sphere, automationID, content); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- rule automations ---

// generateRuleAutomations writes one automation per TRule, named directly off the rule's own
// explicit "rule <spec>" Name (2026-09-29, replacing the earlier trigger/action-derived
// "<eventPath>/when/turns_.../then/turn_..." shape -- that derivation stops being mechanically
// possible once a rule can have several triggers/conditions/actions, which is exactly why the
// name became mandatory; see TRule's own doc comment). The output sphere/directory comes from
// the rule's own Name (its first "/"-segment) rather than the trigger entity's sphere -- the rule
// has its own identity, independent of which entity happens to trigger it.
//
// A rule shaped exactly like PROJECT.md item 13 Phase 1's own scope (one state-transition
// trigger, no conditions, one bare "turn" action) still goes through buildStateTriggerAutomationYAML
// unchanged -- byte-identical to what Phase 1 already verified and (for Junglinster's
// done_removing_smells) already deployed, rather than switching to buildRuleAutomationYAML's
// richer-but-differently-shaped YAML for a rule that never needed the richness in the first
// place. Every other shape uses buildRuleAutomationYAML (Phase 2).
func generateRuleAutomations(outputDir string, admin *TAdministrationState) error {
	for _, rule := range admin.Rules {
		sphere := rule.Name
		if slashIdx := strings.Index(sphere, "/"); slashIdx >= 0 {
			sphere = sphere[:slashIdx]
		}
		automationID := strings.ReplaceAll(rule.Name, "/", "_")

		var content string
		if isPhase1ShapedRule(rule) {
			trigger := rule.Triggers[0]
			action := rule.Actions[0]
			eventID := toHomeAssistantEntityID(trigger.EntitySpec)
			actionID := toHomeAssistantEntityID(action.EntitySpec)
			actionDomain := action.EntitySpec[:strings.Index(action.EntitySpec, ".")]
			content = buildStateTriggerAutomationYAML(rule.Name, automationID, eventID, trigger.State, actionDomain, actionID, action.State)
		} else {
			content = buildRuleAutomationYAML(rule, automationID)
		}

		if err := writeAutomationFile(outputDir, sphere, automationID, content); err != nil {
			return err
		}
	}
	return nil
}

// isPhase1ShapedRule reports whether rule needs nothing beyond what the old ad hoc TWhenRule
// construct already covered: exactly one state-transition trigger (no "for" duration), no "if"
// clause, and exactly one bare "turn" action (no data fields).
func isPhase1ShapedRule(rule TRule) bool {
	if len(rule.Triggers) != 1 || rule.Condition != nil || len(rule.Actions) != 1 {
		return false
	}
	trigger := rule.Triggers[0]
	action := rule.Actions[0]
	return trigger.Kind == "state" && trigger.State != "" && trigger.For == "" &&
		action.Verb == "turn" && len(action.Data) == 0
}

// buildRuleAutomationYAML builds the complete automation YAML for one TRule: multiple triggers,
// an optional "if" clause (a full and/or/not boolean expression, PROJECT.md item 13's own
// "when"/"if"/"then" phrase), and multiple ordered actions (including "if"'s own recursive
// then/else branches and bare "delay" steps).
func buildRuleAutomationYAML(rule TRule, automationID string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- alias: " + rule.Name + "\n")
	sb.WriteString("  id: automation." + automationID + "\n")
	sb.WriteString("  initial_state: True\n")
	sb.WriteString("  mode: queued\n")

	sb.WriteString("  trigger:\n")
	for _, trigger := range rule.Triggers {
		renderRuleTriggerYAML(&sb, trigger)
	}

	if rule.Condition != nil {
		renderRuleConditionTopLevel(&sb, *rule.Condition)
	}

	sb.WriteString("  action:\n")
	for _, action := range rule.Actions {
		renderRuleActionYAML(&sb, "  ", action)
	}

	return sb.String()
}

// namedTriggerEventType is the one event type a named trigger's own event entity raises
// (generateNamedTriggerEvents) -- the "raised" of "when event.<name> is raised".
const namedTriggerEventType = "raised"

// namedTriggerEventEntityID is the HA entity_id of named trigger name's own event entity.
func namedTriggerEventEntityID(name string) string {
	return toHomeAssistantEntityID("event." + name)
}

// generateNamedTriggerEvents writes one trigger-based template event entity per named trigger
// (PROJECT.md item 49, 2026-10-22): its triggers are every contributing device's own matching
// events, its conditions the terms' "and if trigger.<attr> ..." filters
// (namedTriggerSourceCondition), and it raises namedTriggerEventType whenever one passes. Rules
// then trigger on that one entity (renderRuleTriggerYAML) -- the same expansion and gating that
// used to be inlined into every rule, now done once in HA, where hand-made automations can use it
// too.
func generateNamedTriggerEvents(outputDir string, admin *TAdministrationState) error {
	names := make([]string, 0, len(admin.NamedTriggers))
	for name := range admin.NamedTriggers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if len(admin.NamedTriggers[name]) == 0 {
			continue
		}
		displayName := strings.Replace(name, ":", "/", 1)
		sphere := displayName[:strings.Index(displayName+"/", "/")]
		entityID := namedTriggerEventEntityID(name)
		triggers := []TRuleTrigger{{Kind: "named_trigger", Name: name}}
		needsSourceIDs, implied := namedTriggerSourceCondition(triggers, admin.NamedTriggers)

		var sb strings.Builder
		sb.WriteString(generatorHeader)
		sb.WriteString("triggers:\n")
		sourceIndex := 0
		renderNamedTriggerSourcesYAML(&sb, admin.NamedTriggers[name], needsSourceIDs, &sourceIndex)
		if implied != nil {
			sb.WriteString("conditions:\n")
			if implied.Kind == "and" {
				for _, operand := range implied.Operands {
					renderRuleConditionYAML(&sb, "  ", operand)
				}
			} else {
				renderRuleConditionYAML(&sb, "  ", *implied)
			}
		}
		sb.WriteString("event:\n")
		sb.WriteString("- name: " + displayName + "\n")
		sb.WriteString("  unique_id: " + generatedUniqueID(entityID) + "\n")
		sb.WriteString("  event_type: " + namedTriggerEventType + "\n")
		sb.WriteString("  event_types: \"{{ ['" + namedTriggerEventType + "'] }}\"\n")

		dir := filepath.Join(outputDir, "entities", "template", "event", sphere)
		if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), sb.String()); err != nil {
			return err
		}
	}
	return nil
}

// namedTriggerSourceID returns the per-(trigger,source) id this generator assigns when
// per-source "id"-scoped conditions are needed (namedTriggerSourceCondition's own doc comment) --
// shared by that function and renderNamedTriggerSourcesYAML so both agree on the same ids without
// either having to pass the other a lookup table. Index is the running count across ALL
// named_trigger sources seen so far in iteration order (trigger list order, then
// namedTriggers[trigger.Name] order) -- both call sites iterate identically, so index N always
// means the same source in both places.
func namedTriggerSourceID(index int) string {
	return fmt.Sprintf("nt%d", index)
}

// namedTriggerSourceCondition computes the gating condition implied by a rule's own named
// triggers' device-level "and if trigger.<attr> is '<value>'[in [...]];" declarations (Step 2,
// 2026-10-15; extended 2026-10-19 after a real regression found live in Vienna: SEVERAL
// DIFFERENT devices commonly feed the SAME named trigger -- e.g. four different Moes/door
// buttons all feeding "bedroom/main/light/toggle" -- and each may need its OWN, DIFFERENT
// button-number filter. A single shared condition can't express that: HA's automation template
// context (trigger.to_state) only ever reflects whichever ONE entity's trigger actually fired,
// so a condition checking "attribute == X" is either right for every contributing device or
// wrong for some of them, never scoped per-device on its own.
//
// Returns (false, nil) if no source has any attr condition at all -- the common case, no
// implied condition, unchanged from before this extension existed.
//
// Returns (false, cond) when every source that HAS an attr condition agrees on exactly one
// (attr, valueset) pair AND no source is missing one -- preserves the exact single
// shared-condition shape already proven live on the Signify dimmer (one device, several trigger
// declarations, all agreeing), with no per-source "id" machinery at all.
//
// Otherwise returns (true, cond): the caller (generateNamedTriggerEvents) must have
// renderNamedTriggerSourcesYAML emit namedTriggerSourceID(i) as that trigger entry's own "id", in the
// same iteration order this function uses -- cond is an OR, across all sources, of "trigger.id
// == this source's id" alone (a source with no attr condition: always true once it's the one
// that fired, no further restriction) AND-ed with its own attr check (a source that has one).
func namedTriggerSourceCondition(triggers []TRuleTrigger, namedTriggers map[string][]TNamedTriggerSource) (bool, *TRuleCondition) {
	type sourceRef struct {
		index  int
		source TNamedTriggerSource
	}
	var refs []sourceRef
	for _, trigger := range triggers {
		if trigger.Kind != "named_trigger" {
			return false, nil
		}
		for _, source := range namedTriggers[trigger.Name] {
			refs = append(refs, sourceRef{index: len(refs), source: source})
		}
	}
	if len(refs) == 0 {
		return false, nil
	}

	anyAttr, allAgree := false, true
	var sharedAttr string
	var sharedValues []string
	for _, ref := range refs {
		if ref.source.TriggerAttr == "" {
			allAgree = false
			continue
		}
		anyAttr = true
		if sharedAttr == "" {
			sharedAttr, sharedValues = ref.source.TriggerAttr, ref.source.TriggerAttrValues
			continue
		}
		if ref.source.TriggerAttr != sharedAttr || !stringSlicesEqual(ref.source.TriggerAttrValues, sharedValues) {
			allAgree = false
		}
	}
	if !anyAttr {
		return false, nil
	}
	if allAgree {
		return false, attrValueCondition(sharedAttr, sharedValues)
	}

	var operands []TRuleCondition
	for _, ref := range refs {
		idCond := TRuleCondition{Kind: "trigger_id", Value: namedTriggerSourceID(ref.index)}
		if ref.source.TriggerAttr == "" {
			operands = append(operands, idCond)
			continue
		}
		operands = append(operands, TRuleCondition{Kind: "and", Operands: []TRuleCondition{idCond, *attrValueCondition(ref.source.TriggerAttr, ref.source.TriggerAttrValues)}})
	}
	return true, &TRuleCondition{Kind: "or", Operands: operands}
}

// attrValueCondition builds a single-value "template" condition, or an "or" of one per value
// when there's more than one -- reuses the existing and/or/template machinery rather than
// teaching the "template" kind its own second, list-shaped rendering.
func attrValueCondition(attr string, values []string) *TRuleCondition {
	if len(values) == 1 {
		return &TRuleCondition{Kind: "template", TriggerAttr: attr, Value: values[0]}
	}
	operands := make([]TRuleCondition, len(values))
	for i, v := range values {
		operands[i] = TRuleCondition{Kind: "template", TriggerAttr: attr, Value: v}
	}
	return &TRuleCondition{Kind: "or", Operands: operands}
}

// stringSlicesEqual reports whether a and b hold the same values in the same order.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// renderNamedTriggerSourcesYAML writes one event.received trigger per device contributing to a
// named trigger, each with that device's own EventTypes as its event_type list. needsSourceIDs/
// sourceIndex (2026-10-19): when namedTriggerSourceCondition decided several sources disagree on
// their own embedded button-attribute condition, each entry gets its own "id" so the generated
// condition can scope to exactly the one that fired -- namedTriggerSourceID's own doc comment
// explains why both sides agree on the same id without a lookup table (identical iteration order).
func renderNamedTriggerSourcesYAML(sb *strings.Builder, sources []TNamedTriggerSource, needsSourceIDs bool, sourceIndex *int) {
	for _, source := range sources {
		sb.WriteString("  - trigger: event.received\n")
		if needsSourceIDs {
			sb.WriteString("    id: '" + namedTriggerSourceID(*sourceIndex) + "'\n")
		}
		*sourceIndex++
		sb.WriteString("    target:\n")
		sb.WriteString("      entity_id:\n")
		sb.WriteString("      - " + source.EntityID + "\n")
		sb.WriteString("    options:\n")
		sb.WriteString("      event_type:\n")
		for _, eventType := range source.EventTypes {
			sb.WriteString("      - '" + eventType + "'\n")
		}
	}
}

// renderRuleTriggerYAML writes one trigger list item. "event" kind matches the exact schema real
// Zigbee/moes button automations already use live (confirmed against both houses' pulled
// /config/automations.yaml, PROJECT.md item 13's own survey) -- entity-domain event-received,
// not a "platform: event" built-in HA trigger. "state" kind keeps the older "platform: state"
// shape buildStateTriggerAutomationYAML already uses, for consistency with follows'/Phase 1's
// own already-deployed output.
func renderRuleTriggerYAML(sb *strings.Builder, trigger TRuleTrigger) {
	switch trigger.Kind {
	case "named_trigger":
		// The named trigger's own event entity (generateNamedTriggerEvents) already expands and
		// gates every contributing device -- the rule just listens to it.
		sb.WriteString("  - trigger: event.received\n")
		sb.WriteString("    target:\n")
		sb.WriteString("      entity_id:\n")
		sb.WriteString("      - " + namedTriggerEventEntityID(trigger.Name) + "\n")
		sb.WriteString("    options:\n")
		sb.WriteString("      event_type:\n")
		sb.WriteString("      - '" + namedTriggerEventType + "'\n")
	case "event":
		entityID := toHomeAssistantEntityID(trigger.EntitySpec)
		sb.WriteString("  - trigger: event.received\n")
		sb.WriteString("    target:\n")
		sb.WriteString("      entity_id:\n")
		sb.WriteString("      - " + entityID + "\n")
		sb.WriteString("    options:\n")
		sb.WriteString("      event_type:\n")
		// Single-quoted: YAML 1.1's "Norway problem" parses a bare on/off/yes/no/y/n/true/false
		// as a boolean, not a string -- a real bug found live 2026-10-14, Vienna's IKEA shortcut
		// buttons (event_type "on"): the unquoted value matched against the trigger's own
		// event_type as a boolean, which can never equal the real string Zigbee2MQTT reports, so
		// the automation silently never fired despite the MQTT message arriving correctly. Every
		// other string literal this generator writes into automation YAML (trigger.State,
		// cond.State, the "time" trigger's own EventType two cases below) already quotes for
		// exactly this reason; this was the one spot that didn't.
		sb.WriteString("      - '" + trigger.EventType + "'\n")
	case "time":
		sb.WriteString("  - platform: time\n")
		sb.WriteString("    at: '" + trigger.EventType + "'\n")
	case "time_pattern":
		sb.WriteString("  - platform: time_pattern\n")
		sb.WriteString("    minutes: '/1'\n")
	default: // "state"
		entityID := toHomeAssistantEntityID(trigger.EntitySpec)
		sb.WriteString("  - platform: state\n")
		sb.WriteString("    entity_id: " + entityID + "\n")
		if trigger.State != "" {
			sb.WriteString("    to: '" + trigger.State + "'\n")
		}
		if trigger.For != "" {
			hours, minutes, seconds := splitRuleDuration(trigger.For)
			sb.WriteString("    for:\n")
			sb.WriteString("      hours: " + hours + "\n")
			sb.WriteString("      minutes: " + minutes + "\n")
			sb.WriteString("      seconds: " + seconds + "\n")
		}
	}
}

// renderRuleConditionTopLevel writes the rule's own "condition:" key and its value. A top-level
// plain "and" renders as a flat list of its own operands -- matching HA's own implicit-AND
// convention for a bare condition list (and this generator's pre-existing output shape, from
// before the "if" clause allowed real and/or/not composition, for the common "several ANDed
// checks" case) -- rather than wrapping in a nested "condition: and" node, which only a
// top-level "or"/"not" (or a nested "and" several levels down) actually needs.
func renderRuleConditionTopLevel(sb *strings.Builder, cond TRuleCondition) {
	sb.WriteString("  condition:\n")
	if cond.Kind == "and" {
		for _, operand := range cond.Operands {
			renderRuleConditionYAML(sb, "  ", operand)
		}
		return
	}
	renderRuleConditionYAML(sb, "  ", cond)
}

// renderRuleConditionYAML writes one condition list item at indent -- a leaf (state/template/
// time) or a nested "and"/"or"/"not" node with its own "conditions:" list.
func renderRuleConditionYAML(sb *strings.Builder, indent string, cond TRuleCondition) {
	switch cond.Kind {
	case "and", "or", "not":
		sb.WriteString(indent + "- condition: " + cond.Kind + "\n")
		sb.WriteString(indent + "  conditions:\n")
		for _, operand := range cond.Operands {
			renderRuleConditionYAML(sb, indent+"  ", operand)
		}
	case "template":
		sb.WriteString(indent + "- condition: template\n")
		sb.WriteString(indent + "  value_template: '{{ trigger.to_state.attributes." + cond.TriggerAttr + " == " + jinjaQuotedLiteral(cond.Value) + " }}'\n")
	case "trigger_id":
		// Scopes a per-source condition (namedTriggerSourceCondition's own doc comment) to
		// exactly the trigger entry that actually fired -- renderNamedTriggerSourcesYAML must emit the
		// matching "id: '<value>'" on that same trigger entry, or this never matches anything.
		sb.WriteString(indent + "- condition: template\n")
		sb.WriteString(indent + "  value_template: '{{ trigger.id == " + jinjaQuotedLiteral(cond.Value) + " }}'\n")
	case "time":
		sb.WriteString(indent + "- condition: time\n")
		if cond.After != "" {
			sb.WriteString(indent + "  after: '" + cond.After + "'\n")
		}
		if cond.Before != "" {
			sb.WriteString(indent + "  before: '" + cond.Before + "'\n")
		}
		if len(cond.Weekdays) > 0 {
			sb.WriteString(indent + "  weekday:\n")
			for _, day := range cond.Weekdays {
				sb.WriteString(indent + "  - " + day + "\n")
			}
		}
	default: // "state"
		entityID := toHomeAssistantEntityID(cond.EntitySpec)
		sb.WriteString(indent + "- condition: state\n")
		sb.WriteString(indent + "  entity_id: " + entityID + "\n")
		sb.WriteString(indent + "  state: '" + cond.State + "'\n")
		if cond.For != "" {
			hours, minutes, seconds := splitRuleDuration(cond.For)
			sb.WriteString(indent + "  for:\n")
			sb.WriteString(indent + "    hours: " + hours + "\n")
			sb.WriteString(indent + "    minutes: " + minutes + "\n")
			sb.WriteString(indent + "    seconds: " + seconds + "\n")
		}
	}
}

// renderRuleActionYAML writes one action list item at indent, recursing (at indent+"  ") for an
// "if" action's own ThenActions/ElseActions.
// boundedBrightnessTemplate is the brightness_pct a "dim"/"brighten" step sets: entityID's current
// brightness (0 when off) plus step percentage points, never below dimBrightnessFloorPct and never
// above 100 -- so dimming a light at its lowest level keeps it on, and either one switches an off
// light on at the floor level. A plain brightness_step_pct can't do this: from 10% a step of -10
// reaches 0%, which Home Assistant treats as "off" (found 2026-10-23 on Junglinster's dimmers).
func boundedBrightnessTemplate(entityID, step string) string {
	return fmt.Sprintf("\"{{ [[((state_attr('%s', 'brightness') or 0) / 2.55) | round | int + (%s), %d] | max, 100] | min }}\"", entityID, step, dimBrightnessFloorPct)
}

func renderRuleActionYAML(sb *strings.Builder, indent string, action TRuleAction) {
	if action.Verb == "if" {
		condEntityID := toHomeAssistantEntityID(action.CondEntitySpec)
		sb.WriteString(indent + "- if:\n")
		sb.WriteString(indent + "  - condition: state\n")
		sb.WriteString(indent + "    entity_id: " + condEntityID + "\n")
		sb.WriteString(indent + "    state: '" + action.CondState + "'\n")
		sb.WriteString(indent + "  then:\n")
		for _, sub := range action.ThenActions {
			renderRuleActionYAML(sb, indent+"  ", sub)
		}
		if len(action.ElseActions) > 0 {
			sb.WriteString(indent + "  else:\n")
			for _, sub := range action.ElseActions {
				renderRuleActionYAML(sb, indent+"  ", sub)
			}
		}
		return
	}

	if action.Verb == "delay" {
		hours, minutes, seconds := splitRuleDuration(action.Duration)
		sb.WriteString(indent + "- delay:\n")
		sb.WriteString(indent + "    hours: " + hours + "\n")
		sb.WriteString(indent + "    minutes: " + minutes + "\n")
		sb.WriteString(indent + "    seconds: " + seconds + "\n")
		return
	}

	entityID := toHomeAssistantEntityID(action.EntitySpec)
	domain := ""
	if dotIdx := strings.Index(action.EntitySpec, "."); dotIdx >= 0 {
		domain = action.EntitySpec[:dotIdx]
	}

	switch action.Verb {
	case "turn":
		sb.WriteString(indent + "- service: " + domain + ".turn_" + action.State + "\n")
		if len(action.Data) > 0 {
			sb.WriteString(indent + "  data:\n")
			for _, field := range action.Data {
				if field.Name == brightnessBoundedStepField {
					sb.WriteString(indent + "    brightness_pct: " + boundedBrightnessTemplate(entityID, field.Value) + "\n")
					continue
				}
				sb.WriteString(indent + "    " + field.Name + ": " + renderRuleValue(field.Value, field.IsJinja) + "\n")
			}
		}
		sb.WriteString(indent + "  entity_id: " + entityID + "\n")
	case "toggle":
		sb.WriteString(indent + "- service: " + domain + ".toggle\n")
		if len(action.Data) > 0 {
			sb.WriteString(indent + "  data:\n")
			for _, field := range action.Data {
				sb.WriteString(indent + "    " + field.Name + ": " + renderRuleValue(field.Value, field.IsJinja) + "\n")
			}
		}
		sb.WriteString(indent + "  entity_id: " + entityID + "\n")
	case "press":
		sb.WriteString(indent + "- service: button.press\n")
		sb.WriteString(indent + "  entity_id: " + entityID + "\n")
	case "stop":
		sb.WriteString(indent + "- service: cover.stop_cover\n")
		sb.WriteString(indent + "  entity_id: " + entityID + "\n")
	case "close":
		sb.WriteString(indent + "- service: cover.close_cover\n")
		sb.WriteString(indent + "  entity_id: " + entityID + "\n")
	case "open":
		sb.WriteString(indent + "- service: cover.open_cover\n")
		sb.WriteString(indent + "  entity_id: " + entityID + "\n")
	case "set":
		// "set" is cover-position-specific -- the only real case among the manual automations
		// being migrated (PROJECT.md item 13's own survey); not a general cross-domain verb.
		sb.WriteString(indent + "- service: cover.set_cover_position\n")
		sb.WriteString(indent + "  data:\n")
		sb.WriteString(indent + "    position: " + renderRuleValue(action.Value, action.ValueIsJinja) + "\n")
		sb.WriteString(indent + "  entity_id: " + entityID + "\n")
	case "start":
		sb.WriteString(indent + "- service: " + domain + ".start\n")
		sb.WriteString(indent + "  entity_id: " + entityID + "\n")
	case "pause":
		sb.WriteString(indent + "- service: " + domain + ".pause\n")
		sb.WriteString(indent + "  entity_id: " + entityID + "\n")
	case "return_to_base":
		sb.WriteString(indent + "- service: vacuum.return_to_base\n")
		sb.WriteString(indent + "  entity_id: " + entityID + "\n")
	}
}

// renderRuleValue renders a data-field/set value for YAML: a Jinja value becomes a single-quoted
// "{{ ... }}" template; a plain value is emitted bare if it parses as a number, single-quoted
// otherwise.
func renderRuleValue(value string, isJinja bool) string {
	if isJinja {
		// The whole "{{ ... }}" expression is itself wrapped in a single-quoted YAML
		// scalar, so any single quote the Jinja author wrote inside it (e.g.
		// "states('number.foo')") must be doubled for YAML's own escaping -- real bug
		// found live 2026-10-13, deploying Junglinster's sarah_bedroom cover/close rule:
		// the unescaped quote broke the YAML block mapping and put the whole house's HA
		// core into recovery mode. jinjaQuotedLiteral (below) already does this doubling
		// for a different call site (a literal string value inside a template
		// condition) -- this is the same YAML-level rule applied to raw jinja pass-through
		// text instead.
		return "'{{ " + strings.ReplaceAll(value, "'", "''") + " }}'"
	}
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return value
	}
	return "'" + value + "'"
}

// jinjaQuotedLiteral renders a literal value as a Jinja string literal, doubled-single-quoted --
// Jinja itself uses single quotes for strings, and YAML's own single-quote escaping (one '
// becomes two ”) applies on top since the whole value_template is itself a single-quoted YAML
// scalar (e.g. the real "trigger.to_state.attributes.button == 'on'" case becomes
// "... == ”on” ..." once YAML-escaped).
func jinjaQuotedLiteral(value string) string {
	return "''" + value + "''"
}

// splitRuleDuration splits a TRuleTrigger.For/TRuleAction.Duration "HH:MM:SS" string into its
// three fields, for a YAML "for:"/"delay:" block. Malformed input (should never happen -- the
// parser's own Duration grammar rule guarantees the shape) falls back to all-zero.
func splitRuleDuration(duration string) (hours, minutes, seconds string) {
	parts := strings.Split(duration, ":")
	if len(parts) != 3 {
		return "0", "0", "0"
	}
	return parts[0], parts[1], parts[2]
}

// --- switched_device entities ---

// generateSwitchedDeviceEntities writes the group entity/entities for every "definition as
// switched_device <device> <main>;" directive. When the declaring entity is a light, an
// intermediate "all: true" helper group combines device+main, and the declaring entity becomes
// a default (any-on) group of [helper, main]. For any other domain, a single "all: true" group
// of [device, main] is written directly as the declaring entity.
func generateSwitchedDeviceEntities(outputDir string, admin *TAdministrationState) error {
	for _, rel := range admin.SwitchedDeviceRelations {
		selfID := toHomeAssistantEntityID(rel.SelfEntity)
		deviceID := toHomeAssistantEntityID(rel.Device)
		mainID := toHomeAssistantEntityID(rel.MainEntity)
		if selfID == "" || deviceID == "" || mainID == "" {
			continue
		}
		selfIdentity := extractEntityIdentity(rel.SelfEntity)
		deviceIdentity := extractEntityIdentity(rel.Device)
		mainIdentity := extractEntityIdentity(rel.MainEntity)

		if deviceIdentity.Domain == "light" {
			helperName := "light.physical/" + mainIdentity.Path + "/helper"
			helperID := toHomeAssistantEntityID(helperName)
			helperDisplayName := strings.TrimPrefix(helperName, "light.")
			helperContent := buildPlatformGroupYAML(helperID, helperDisplayName, true, []string{deviceID, mainID})
			helperDir := filepath.Join(outputDir, "entities", "light", "physical")
			if err := writeYAMLFile(filepath.Join(helperDir, helperID+".yaml"), helperContent); err != nil {
				return err
			}

			selfDisplayName := strings.TrimPrefix(rel.SelfEntity, "light.")
			selfContent := buildPlatformGroupYAML(selfID, selfDisplayName, false, []string{helperID, mainID})
			selfDir := filepath.Join(outputDir, "entities", "light", selfIdentity.Sphere)
			if err := writeYAMLFile(filepath.Join(selfDir, selfID+".yaml"), selfContent); err != nil {
				return err
			}
		} else {
			selfDisplayName := strings.TrimPrefix(rel.SelfEntity, selfIdentity.Domain+".")
			content := buildPlatformGroupYAML(selfID, selfDisplayName, true, []string{deviceID, mainID})
			dir := filepath.Join(outputDir, "entities", selfIdentity.Domain, selfIdentity.Sphere)
			if err := writeYAMLFile(filepath.Join(dir, selfID+".yaml"), content); err != nil {
				return err
			}
		}
	}
	return nil
}

// buildPlatformGroupYAML builds a "platform: group" entity, optionally requiring all members to
// be on (rather than any). displayName is the slash-separated conceptual path (e.g.
// "social/house/kitchen/workplace/main", domain stripped) -- HA slugifies this into the object_id
// when no unique_id is given, but here entityID is ALWAYS supplied explicitly (matching
// buildLightGroupYAML's own established convention, generator.go, used for room-level light
// aggregates), so "name" is purely the friendly_name HA displays, decoupled from the actual
// entity_id. A real gap found live 2026-09-28: this function used to write the already-sanitised,
// underscore-joined object_id itself as "name" (e.g. "social_house_kitchen_workplace_main"), unlike
// every other group-style light entity in this codebase -- fixed to match the same convention.
func buildPlatformGroupYAML(entityID, displayName string, all bool, memberIDs []string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("platform: group\n")
	if all {
		sb.WriteString("all: true\n")
	}
	sb.WriteString("name: " + displayName + "\n")
	sb.WriteString("unique_id: " + generatedUniqueID(entityID) + "\n")
	sb.WriteString("entities:\n")
	for _, id := range memberIDs {
		sb.WriteString("- " + id + "\n")
	}
	return sb.String()
}

// --- has_state entities ---

// generateHasStateEntities writes the template binary_sensor entity for every "definition as
// has_state <source> <state> [delay_on] [delay_off];" directive: its state mirrors whether
// <source> is in <state> (or has that attribute value, for "entity!attribute" sources).
func generateHasStateEntities(outputDir string, admin *TAdministrationState) error {
	for _, rel := range admin.HasStateRelations {
		selfID := toHomeAssistantEntityID(rel.SelfEntity)
		if selfID == "" {
			continue
		}
		identity := extractEntityIdentity(rel.SelfEntity)
		displayName := identity.Sphere + "/" + identity.Path
		stateExpr := hasStateExpr(rel.Source, rel.State)
		deviceClass, _, _, _ := resolveCapabilityDefaults(admin.CapabilityDefaults, "binary_sensor", identity.Path)
		content := buildConditionBinarySensorYAML(selfID, displayName, stateExpr, deviceClass, rel.DelayOn, rel.DelayOff, "")
		dir := filepath.Join(outputDir, "entities", "template", "binary_sensor", identity.Sphere)
		if err := writeYAMLFile(filepath.Join(dir, selfID+".yaml"), content); err != nil {
			return err
		}
	}
	return nil
}

func hasStateExpr(source, state string) string {
	if bangIdx := strings.Index(source, "!"); bangIdx > 0 {
		entityID := toHomeAssistantEntityID(source[:bangIdx])
		attr := source[bangIdx+1:]
		return fmt.Sprintf("is_state_attr('%s', '%s', '%s')", entityID, attr, state)
	}
	entityID := toHomeAssistantEntityID(source)
	return fmt.Sprintf("is_state('%s', '%s')", entityID, state)
}

// --- timer entities ---

// generateTimerDefinitions writes the timer entity for every "definition as timer
// \"<duration>\";" directive.
func generateTimerDefinitions(outputDir string, admin *TAdministrationState) error {
	for _, rel := range admin.TimerDefRelations {
		selfID := toHomeAssistantEntityID(rel.SelfEntity)
		if selfID == "" {
			continue
		}
		identity := extractEntityIdentity(rel.SelfEntity)
		displayName := identity.Sphere + "/" + identity.Path
		key := strings.TrimPrefix(selfID, "timer.")
		var sb strings.Builder
		sb.WriteString(generatorHeader)
		sb.WriteString(key + ":\n")
		sb.WriteString("  name: " + displayName + "\n")
		sb.WriteString("  duration: '" + rel.Duration + "'\n")
		dir := filepath.Join(outputDir, "entities", "timer", identity.Sphere)
		if err := writeYAMLFile(filepath.Join(dir, selfID+".yaml"), sb.String()); err != nil {
			return err
		}
	}
	return nil
}

// --- switched_device automations ---

func generateSwitchedDeviceAutomations(outputDir string, admin *TAdministrationState) error {
	for _, rel := range admin.SwitchedDeviceRelations {
		deviceID := toHomeAssistantEntityID(rel.Device)
		mainID := toHomeAssistantEntityID(rel.MainEntity)

		mainPath := rel.MainEntity[strings.Index(rel.MainEntity, ".")+1:]
		sphere := entitySphere(rel.MainEntity)
		domain := rel.MainEntity[:strings.Index(rel.MainEntity, ".")]
		// mainObjectID is mainID (already fully sanitized by toHomeAssistantEntityID) with its own
		// "<domain>." prefix stripped -- used for automationID instead of a raw slash-only
		// replacement of mainPath, since mainPath may itself embed a literal "." when MainEntity was
		// auto-materialized from a raw DSL device-id rather than an ordinary Conceptual.def spec
		// (logical_switched_by.go, 2026-09-27's "switched by" migration -- a hand-written spec never
		// itself contains a "."). A raw dot slipping past this file's own slash-only replacement
		// used to leak verbatim into automation.<id>, a real bug found live regenerating Junglinster's
		// server_room/corridor pilot. Byte-identical to the old derivation for every pre-existing
		// switched_device instance (none of their MainEntity paths ever contained a "." to begin
		// with, so sanitizeObjectID's own dot-handling is a no-op there).
		mainObjectID := strings.TrimPrefix(mainID, domain+".")

		for _, dir := range []string{"switched_on", "switched_off"} {
			aliasPath := mainPath + "/" + dir
			automationID := mainObjectID + "_" + dir
			toState, condState, service := "'on'", "'off'", domain+".turn_on"
			if dir == "switched_off" {
				toState, condState, service = "'off'", "'on'", domain+".turn_off"
			}

			var sb strings.Builder
			sb.WriteString(generatorHeader)
			sb.WriteString("- alias: " + aliasPath + "\n")
			sb.WriteString("  id: automation." + automationID + "\n")
			sb.WriteString("  initial_state: True\n")
			sb.WriteString("  mode: queued\n")
			sb.WriteString("  trigger:\n")
			sb.WriteString("  - platform: state\n")
			sb.WriteString("    entity_id: " + mainID + "\n")
			sb.WriteString("    to: " + toState + "\n")
			sb.WriteString("  condition:\n")
			sb.WriteString("  - condition: state\n")
			sb.WriteString("    entity_id: " + deviceID + "\n")
			sb.WriteString("    state: " + condState + "\n")
			sb.WriteString("  action:\n")
			sb.WriteString("  - service: " + service + "\n")
			sb.WriteString("    entity_id: " + deviceID + "\n")

			if err := writeAutomationFile(outputDir, sphere, automationID, sb.String()); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- timer limits automations (removing_smell) ---

func generateTimerLimitsAutomations(outputDir string, admin *TAdministrationState) error {
	for _, rel := range admin.TimerLimitsRelations {
		timerID := toHomeAssistantEntityID(rel.TimerEntity)
		boundID := toHomeAssistantEntityID(rel.BoundEntity)

		timerPath := rel.TimerEntity[strings.Index(rel.TimerEntity, ".")+1:]
		sphere := entitySphere(rel.TimerEntity)
		boundDomain := rel.BoundEntity[:strings.Index(rel.BoundEntity, ".")]

		prefix := strings.ReplaceAll(timerPath, "/", "_")

		// start: bound entity turns on → start timer
		startID := prefix + "_start"
		{
			var sb strings.Builder
			sb.WriteString(generatorHeader)
			sb.WriteString("- alias: " + timerPath + "/start\n")
			sb.WriteString("  id: automation." + startID + "\n")
			sb.WriteString("  initial_state: True\n")
			sb.WriteString("  mode: queued\n")
			sb.WriteString("  trigger:\n")
			sb.WriteString("  - entity_id: " + boundID + "\n")
			sb.WriteString("    platform: state\n")
			sb.WriteString("    to: 'on'\n")
			sb.WriteString("  action:\n")
			sb.WriteString("  - service: timer.start\n")
			sb.WriteString("    entity_id: " + timerID + "\n")
			if err := writeAutomationFile(outputDir, sphere, startID, sb.String()); err != nil {
				return err
			}
		}

		// finish: timer.finished event → turn off bound entity
		finishID := prefix + "_finish"
		{
			var sb strings.Builder
			sb.WriteString(generatorHeader)
			sb.WriteString("- alias: " + timerPath + "/finish\n")
			sb.WriteString("  id: automation." + finishID + "\n")
			sb.WriteString("  initial_state: True\n")
			sb.WriteString("  mode: queued\n")
			sb.WriteString("  trigger:\n")
			sb.WriteString("  - platform: event\n")
			sb.WriteString("    event_type: timer.finished\n")
			sb.WriteString("    event_data:\n")
			sb.WriteString("      entity_id: " + timerID + "\n")
			sb.WriteString("  action:\n")
			sb.WriteString("  - service: " + boundDomain + ".turn_off\n")
			sb.WriteString("    entity_id: " + boundID + "\n")
			if err := writeAutomationFile(outputDir, sphere, finishID, sb.String()); err != nil {
				return err
			}
		}

		// cancel: bound entity turns off → cancel timer
		cancelID := prefix + "_cancel"
		{
			var sb strings.Builder
			sb.WriteString(generatorHeader)
			sb.WriteString("- alias: " + timerPath + "/cancel\n")
			sb.WriteString("  id: automation." + cancelID + "\n")
			sb.WriteString("  initial_state: True\n")
			sb.WriteString("  mode: queued\n")
			sb.WriteString("  trigger:\n")
			sb.WriteString("  - entity_id: " + boundID + "\n")
			sb.WriteString("    platform: state\n")
			sb.WriteString("    to: 'off'\n")
			sb.WriteString("  action:\n")
			sb.WriteString("  - service: timer.cancel\n")
			sb.WriteString("    entity_id: " + timerID + "\n")
			if err := writeAutomationFile(outputDir, sphere, cancelID, sb.String()); err != nil {
				return err
			}
		}

		// was: time_pattern + timer idle + bound on → restart timer
		wasID := prefix + "_was"
		{
			var sb strings.Builder
			sb.WriteString(generatorHeader)
			sb.WriteString("- alias: " + timerPath + "/was\n")
			sb.WriteString("  id: automation." + wasID + "\n")
			sb.WriteString("  initial_state: True\n")
			sb.WriteString("  mode: queued\n")
			sb.WriteString("  trigger:\n")
			sb.WriteString("    platform: time_pattern\n")
			sb.WriteString("    minutes: '/1'\n")
			sb.WriteString("  condition:\n")
			sb.WriteString("  - condition: state\n")
			sb.WriteString("    entity_id: " + timerID + "\n")
			sb.WriteString("    state: 'idle'\n")
			sb.WriteString("  - condition: state\n")
			sb.WriteString("    entity_id: " + boundID + "\n")
			sb.WriteString("    state: 'on'\n")
			sb.WriteString("  action:\n")
			sb.WriteString("  - service: timer.start\n")
			sb.WriteString("    entity_id: " + timerID + "\n")
			if err := writeAutomationFile(outputDir, sphere, wasID, sb.String()); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- switch-controlled heating: input_booleans, scripts, automations ---

func generateSwitchControlledHeatingContent(outputDir string, admin *TAdministrationState) error {
	// Collect all spaces that have a heating switch entity -- sphere-agnostic (confirmed with the
	// user 2026-10-07, the same fix as heatingCapableSpaces/administration.go): detection is
	// triggered purely by the "heating" path tail, not a specific sphere. heatingSwitchEntityIDs
	// remembers each context path's own REAL switch entity_id (whichever sphere it's actually
	// positioned under) -- a real bug found live generating Junglinster's own output: this used to
	// hardcode "switch.physical_<context>_heating" below regardless of the switch's own real
	// sphere, which silently pointed every switch.turn_on/turn_off automation at a NON-EXISTENT
	// entity once a heating switch was repositioned to "social".
	seen := map[string]bool{}
	var contextPaths []string
	heatingSwitchEntityIDs := map[string]string{}
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain != "switch" {
				continue
			}
			p := rec.Identity.Path
			if !strings.HasSuffix(p, "/heating") && p != "heating" {
				continue
			}
			cp := strings.TrimSuffix(p, "/heating")
			if cp == p {
				cp = ""
			}
			if !seen[cp] {
				seen[cp] = true
				contextPaths = append(contextPaths, cp)
				heatingSwitchEntityIDs[cp] = toHomeAssistantEntityID(rec.Name)
			}
		}
	}
	sort.Strings(contextPaths)

	for _, cp := range contextPaths {
		var socialPath, physPath string
		if cp == "" {
			socialPath = "social"
			physPath = "physical"
		} else {
			socialPath = "social/" + cp
			physPath = "physical/" + cp
		}
		socialUnder := strings.ReplaceAll(socialPath, "/", "_")
		physUnder := strings.ReplaceAll(physPath, "/", "_")

		// Check whether this space also has a heating leak (needed for should_be_off sensor).
		heatingShouldBeOffID := "binary_sensor." + physUnder + "_heating_should_be_off"

		// --- Input booleans ---
		type boolSpec struct {
			key         string
			displayName string
			dir         string
		}
		booleans := []boolSpec{
			{socialUnder + "_heating_enable", socialPath + "/heating/enable", "social"},
			{socialUnder + "_heating_leakage_ignore", socialPath + "/heating/leakage/ignore", "social"},
			{socialUnder + "_heating_mode_desired", socialPath + "/heating_mode_desired", "social"},
			{socialUnder + "_heating_mode_preset_for_around", socialPath + "/heating_mode_preset_for_around", "social"},
			{socialUnder + "_heating_mode_preset_for_away", socialPath + "/heating_mode_preset_for_away", "social"},
			{socialUnder + "_heating_mode_preset_for_day", socialPath + "/heating_mode_preset_for_day", "social"},
			{socialUnder + "_heating_mode_preset_for_night", socialPath + "/heating_mode_preset_for_night", "social"},
			{physUnder + "_heating_mode_target", physPath + "/heating_mode_target", "physical"},
			{physUnder + "_heating_mode_preset_for_leakage", physPath + "/heating_mode_preset_for_leakage", "physical"},
		}
		for _, b := range booleans {
			var sb strings.Builder
			sb.WriteString(generatorHeader)
			sb.WriteString(b.key + ":\n")
			sb.WriteString("  name: " + b.displayName + "\n")
			sb.WriteString("  icon: mdi:radiator\n")
			dir := filepath.Join(outputDir, "entities", "input_boolean", b.dir)
			if err := writeYAMLFile(filepath.Join(dir, "input_boolean."+b.key+".yaml"), sb.String()); err != nil {
				return err
			}
			customDir := filepath.Join(outputDir, "customization", "input_boolean", b.dir)
			if err := writeYAMLFile(filepath.Join(customDir, "input_boolean."+b.key+".yaml"),
				buildCustomizationYAML("input_boolean."+b.key, b.displayName, "mdi:radiator")); err != nil {
				return err
			}
		}

		heatingModeDesiredID := "input_boolean." + socialUnder + "_heating_mode_desired"
		heatingModeTargetID := "input_boolean." + physUnder + "_heating_mode_target"
		heatingModePresetLeakageID := "input_boolean." + physUnder + "_heating_mode_preset_for_leakage"
		heatingSwitch := heatingSwitchEntityIDs[cp]

		// --- Binary sensor mirrors ---
		// Both mirrors live under "social", even though heating_mode_target mirrors the
		// "physical" heatingModeTargetID input_boolean — matches old bash's
		// "entity declare binary_sensor.social:/$SpaceLocation:heating_mode_target".
		mirrors := []struct{ key, displayName, sourceID string }{
			{socialUnder + "_heating_mode_desired", socialPath + "/heating_mode_desired", heatingModeDesiredID},
			{socialUnder + "_heating_mode_target", socialPath + "/heating_mode_target", heatingModeTargetID},
		}
		for _, m := range mirrors {
			entityID := "binary_sensor." + m.key
			content := buildConditionBinarySensorYAML(entityID, m.displayName, "is_state('"+m.sourceID+"', 'on')", "", "", "", "")
			dir := filepath.Join(outputDir, "entities", "template", "binary_sensor", "social")
			if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
				return err
			}
			customDir := filepath.Join(outputDir, "customization", "binary_sensor", "social")
			if err := writeYAMLFile(filepath.Join(customDir, entityID+".yaml"),
				buildCustomizationYAML(entityID, m.displayName, "mdi:radiator")); err != nil {
				return err
			}
		}

		// --- Scripts ---
		presets := []string{"around", "away", "day", "night"}
		for _, preset := range presets {
			presetBoolID := "input_boolean." + socialUnder + "_heating_mode_preset_for_" + preset
			scriptKey := socialUnder + "_set_heating_to_" + preset
			scriptAlias := socialPath + "/set_heating_to_" + preset

			var sb strings.Builder
			sb.WriteString(generatorHeader)
			sb.WriteString(scriptKey + ":\n")
			sb.WriteString("  alias: " + scriptAlias + "\n")
			sb.WriteString("  mode: queued\n")
			sb.WriteString("  sequence:\n")
			sb.WriteString("  - service_template: >\n")
			sb.WriteString("      {% if is_state('" + presetBoolID + "', 'on') %}\n")
			sb.WriteString("        input_boolean.turn_on\n")
			sb.WriteString("      {% else %}\n")
			sb.WriteString("        input_boolean.turn_off\n")
			sb.WriteString("      {% endif %}\n")
			sb.WriteString("    entity_id: " + heatingModeDesiredID + "\n")

			dir := filepath.Join(outputDir, "script", "social")
			if err := writeYAMLFile(filepath.Join(dir, "script."+scriptKey+".yaml"), sb.String()); err != nil {
				return err
			}
		}

		// --- Automations ---
		// set_heating_to_desired_mode
		{
			autoID := physUnder + "_set_heating_to_desired_mode"
			var sb strings.Builder
			sb.WriteString(generatorHeader)
			sb.WriteString("- alias: " + physPath + "/set_heating_to_desired_mode\n")
			sb.WriteString("  id: automation." + autoID + "\n")
			sb.WriteString("  initial_state: True\n")
			sb.WriteString("  mode: queued\n")
			sb.WriteString("  trigger:\n")
			sb.WriteString("  - platform: state\n")
			sb.WriteString("    entity_id: " + heatingShouldBeOffID + "\n")
			sb.WriteString("    to: 'off'\n")
			sb.WriteString("  - platform: state\n")
			sb.WriteString("    entity_id: " + heatingModeDesiredID + "\n")
			sb.WriteString("  - platform: time_pattern\n")
			sb.WriteString("    minutes: '/1'\n")
			sb.WriteString("  condition:\n")
			sb.WriteString("  - condition: state\n")
			sb.WriteString("    entity_id: " + heatingShouldBeOffID + "\n")
			sb.WriteString("    state: 'off'\n")
			sb.WriteString("  action:\n")
			sb.WriteString("  - service_template: >\n")
			sb.WriteString("      {% if is_state('" + heatingModeDesiredID + "', 'on') %}\n")
			sb.WriteString("        input_boolean.turn_on\n")
			sb.WriteString("      {% else %}\n")
			sb.WriteString("        input_boolean.turn_off\n")
			sb.WriteString("      {% endif %}\n")
			sb.WriteString("    entity_id: " + heatingModeTargetID + "\n")
			if err := writeAutomationFile(outputDir, "physical", autoID, sb.String()); err != nil {
				return err
			}
		}

		// set_heating_to_leakage_mode
		{
			autoID := physUnder + "_set_heating_to_leakage_mode"
			var sb strings.Builder
			sb.WriteString(generatorHeader)
			sb.WriteString("- alias: " + physPath + "/set_heating_to_leakage_mode\n")
			sb.WriteString("  id: automation." + autoID + "\n")
			sb.WriteString("  initial_state: True\n")
			sb.WriteString("  mode: queued\n")
			sb.WriteString("  trigger:\n")
			sb.WriteString("  - platform: state\n")
			sb.WriteString("    entity_id: " + heatingShouldBeOffID + "\n")
			sb.WriteString("    to: 'on'\n")
			sb.WriteString("  - platform: time_pattern\n")
			sb.WriteString("    minutes: '/1'\n")
			sb.WriteString("  condition:\n")
			sb.WriteString("  - condition: state\n")
			sb.WriteString("    entity_id: " + heatingShouldBeOffID + "\n")
			sb.WriteString("    state: 'on'\n")
			sb.WriteString("  action:\n")
			sb.WriteString("  - service_template: >\n")
			sb.WriteString("      {% if is_state('" + heatingModePresetLeakageID + "', 'on') %}\n")
			sb.WriteString("        input_boolean.turn_on\n")
			sb.WriteString("      {% else %}\n")
			sb.WriteString("        input_boolean.turn_off\n")
			sb.WriteString("      {% endif %}\n")
			sb.WriteString("    entity_id: " + heatingModeTargetID + "\n")
			if err := writeAutomationFile(outputDir, "physical", autoID, sb.String()); err != nil {
				return err
			}
		}

		// set_heating_to_target_mode_off
		{
			autoID := physUnder + "_set_heating_to_target_mode_off"
			var sb strings.Builder
			sb.WriteString(generatorHeader)
			sb.WriteString("- alias: " + physPath + "/set_heating_to_target_mode_off\n")
			sb.WriteString("  id: automation." + autoID + "\n")
			sb.WriteString("  initial_state: True\n")
			sb.WriteString("  mode: queued\n")
			sb.WriteString("  trigger:\n")
			sb.WriteString("  - platform: state\n")
			sb.WriteString("    entity_id: " + heatingModeTargetID + "\n")
			sb.WriteString("  - platform: state\n")
			sb.WriteString("    entity_id: " + heatingSwitch + "\n")
			sb.WriteString("  condition:\n")
			sb.WriteString("  - condition: state\n")
			sb.WriteString("    entity_id: " + heatingModeTargetID + "\n")
			sb.WriteString("    state: 'off'\n")
			sb.WriteString("  action:\n")
			sb.WriteString("  - service: switch.turn_off\n")
			sb.WriteString("    entity_id: " + heatingSwitch + "\n")
			if err := writeAutomationFile(outputDir, "physical", autoID, sb.String()); err != nil {
				return err
			}
		}

		// set_heating_to_target_mode_on
		{
			autoID := physUnder + "_set_heating_to_target_mode_on"
			var sb strings.Builder
			sb.WriteString(generatorHeader)
			sb.WriteString("- alias: " + physPath + "/set_heating_to_target_mode_on\n")
			sb.WriteString("  id: automation." + autoID + "\n")
			sb.WriteString("  initial_state: True\n")
			sb.WriteString("  mode: queued\n")
			sb.WriteString("  trigger:\n")
			sb.WriteString("  - platform: state\n")
			sb.WriteString("    entity_id: " + heatingModeTargetID + "\n")
			sb.WriteString("  - platform: state\n")
			sb.WriteString("    entity_id: " + heatingSwitch + "\n")
			sb.WriteString("  condition:\n")
			sb.WriteString("  - condition: state\n")
			sb.WriteString("    entity_id: " + heatingModeTargetID + "\n")
			sb.WriteString("    state: 'on'\n")
			sb.WriteString("  action:\n")
			sb.WriteString("  - service: switch.turn_on\n")
			sb.WriteString("    entity_id: " + heatingSwitch + "\n")
			if err := writeAutomationFile(outputDir, "physical", autoID, sb.String()); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- list file generation ---

type TListPattern struct {
	domain       string
	sphere       string // empty = match any sphere; otherwise only entities in this sphere match
	pathSuffix   string // empty = match all; otherwise path must end with (wildcardLeaf: contain, one more segment past) this segment sequence
	wildcardLeaf bool   // true for a trailing "/*" pattern segment -- see parseListPatterns
}

type TListCleanOp struct {
	kind  string // "prefix" or "postfix"
	value string
}

type TListDeclaration struct {
	title    string
	patterns []TListPattern
	cleanOps []TListCleanOp
	// asCards marks a "list ... as cards with: ... end;" declaration: render as a Lovelace
	// vertical-stack of per-entity built-in "sensor" mini-graph cards (buildSensorGraphListFileYAML)
	// instead of the default flat entities list (buildListFileYAML).
	asCards bool
	// detailLevel is the "detail_level N;" with:-block option -- only meaningful when asCards is
	// true. Defaults to 2 if not given.
	detailLevel int
}

// generateListFiles parses External.def content and writes a list.* file for each declaration.
func generateListFiles(outputDir string, content []byte, admin *TAdministrationState) error {
	declarations := parseListDeclarations(string(content))
	for _, decl := range declarations {
		entries := resolveListEntries(decl, admin)
		if len(entries) == 0 {
			continue
		}
		var yaml string
		if decl.asCards {
			yaml = buildSensorGraphListFileYAML(entries, decl.detailLevel)
		} else {
			yaml = buildListFileYAML(decl.title, entries)
		}
		fileName := "list." + listTitleToFileName(decl.title)
		if err := os.WriteFile(filepath.Join(outputDir, fileName), []byte(yaml), 0644); err != nil {
			return fmt.Errorf("cannot write %s: %w", fileName, err)
		}
	}
	return nil
}

// parseListDeclarations extracts all list declarations from External.def source.
func parseListDeclarations(src string) []TListDeclaration {
	var declarations []TListDeclaration
	lines := strings.Split(src, "\n")
	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			i++
			continue
		}
		if !strings.HasPrefix(line, "list ") {
			i++
			continue
		}
		// Parse: list "Title" pattern... [with: cleanOps end;]
		rest := strings.TrimSpace(line[len("list "):])

		// Extract quoted title.
		if !strings.HasPrefix(rest, `"`) {
			i++
			continue
		}
		closeQuote := strings.Index(rest[1:], `"`)
		if closeQuote < 0 {
			i++
			continue
		}
		title := rest[1 : closeQuote+1]
		rest = strings.TrimSpace(rest[closeQuote+2:])

		// Extract patterns (before optional "with:" or end of line).
		patternsPart := rest
		withIdx := strings.Index(rest, " with:")
		if withIdx >= 0 {
			patternsPart = rest[:withIdx]
		} else {
			// Patterns may end with ";" if no with block.
			patternsPart = strings.TrimSuffix(strings.TrimSpace(patternsPart), ";")
		}

		// An "as cards" marker trails the patterns, before "with:" -- same convention as
		// "space <spec> as area with:".
		asCards := false
		patternsPart = strings.TrimSpace(patternsPart)
		if trimmed := strings.TrimSuffix(patternsPart, " as cards"); trimmed != patternsPart {
			asCards = true
			patternsPart = trimmed
		}
		patterns := parseListPatterns(patternsPart)

		var cleanOps []TListCleanOp
		detailLevel := 2
		if withIdx >= 0 {
			// Read clean_prefix / clean_postfix / detail_level lines until "end;".
			i++
			for i < len(lines) {
				cline := strings.TrimSpace(lines[i])
				if cline == "end;" || cline == "end" {
					break
				}
				cline = strings.TrimSuffix(cline, ";")
				fields := strings.Fields(cline)
				if len(fields) == 2 {
					switch fields[0] {
					case "clean_prefix":
						cleanOps = append(cleanOps, TListCleanOp{"prefix", fields[1]})
					case "clean_postfix":
						cleanOps = append(cleanOps, TListCleanOp{"postfix", fields[1]})
					case "detail_level":
						if n, err := strconv.Atoi(fields[1]); err == nil {
							detailLevel = n
						}
					}
				}
				i++
			}
		}

		declarations = append(declarations, TListDeclaration{
			title: title, patterns: patterns, cleanOps: cleanOps,
			asCards: asCards, detailLevel: detailLevel,
		})
		i++
	}
	return declarations
}

// parseListPatterns parses one or more space-separated entity patterns.
// Pattern forms:
//
//	"domain.*"                 — match all entities with domain, any sphere
//	"domain.*/suffix"          — match by path tail, any sphere
//	"domain.sphere/*/suffix"   — match by path tail, specific sphere only
//	"domain.*/prefix/*"        — match any path with "prefix" as its second-to-last segment
//	                              and exactly one more (any) trailing leaf segment, any sphere
//	                              -- e.g. "sensor.*/cpu/*" matches "…/cpu/load", "…/cpu/temperature",
//	                              any current or future cpu-suffix attribute, but not bare "…/cpu".
//
// The keyword "all" and any other token without a '.' are silently ignored.
func parseListPatterns(src string) []TListPattern {
	var patterns []TListPattern
	for _, token := range strings.Fields(src) {
		token = strings.TrimSuffix(token, ";")
		dotIdx := strings.Index(token, ".")
		if dotIdx <= 0 {
			continue // skip "all" and other non-pattern tokens
		}
		domain := token[:dotIdx]
		rest := token[dotIdx+1:] // e.g. "*/node", "social/*/door", "*"

		sphere := ""
		firstSlash := strings.Index(rest, "/")
		if firstSlash > 0 {
			prefix := rest[:firstSlash]
			if prefix != "*" {
				// Named sphere before the first slash: extract it.
				sphere = prefix
				rest = rest[firstSlash+1:] // e.g. "*/door"
			}
		}

		pathSuffix := ""
		if slashIdx := strings.Index(rest, "/"); slashIdx >= 0 {
			pathSuffix = rest[slashIdx+1:]
		}
		wildcardLeaf := false
		if pathSuffix == "*" {
			pathSuffix = "" // "domain.*/*" degenerates to "domain.*" -- any path at all
		} else if strings.HasSuffix(pathSuffix, "/*") {
			pathSuffix = strings.TrimSuffix(pathSuffix, "/*")
			wildcardLeaf = true
		}
		patterns = append(patterns, TListPattern{domain: domain, sphere: sphere, pathSuffix: pathSuffix, wildcardLeaf: wildcardLeaf})
	}
	return patterns
}

type TListEntry struct {
	entityID    string
	displayName string
}

// resolveListEntries collects all entity records that match the declaration's patterns,
// applies clean operations, deduplicates, and sorts by display name.
//
// For sphere-filtered patterns (e.g. binary_sensor.social/*/door), derived binary_sensor
// subdomain aggregate records are also considered when they represent a leaf group — i.e.
// the space contains at least one non-derived entity with the matching subdomain. This lets
// the social group entity (e.g. social_apartment_bedroom_door) appear instead of the raw
// physical entity that lives in that space.
func resolveListEntries(decl TListDeclaration, admin *TAdministrationState) []TListEntry {
	seen := map[string]bool{}
	var entries []TListEntry

	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Provenance == "derived binary_sensor subdomain aggregate" {
				if !isLeafDerivedAggregate(rec, spaceName, decl.patterns, admin) {
					continue
				}
			} else if strings.HasPrefix(rec.Provenance, "derived") {
				continue
			} else if !matchesAnyListPattern(rec, decl.patterns) {
				continue
			}
			entityID := toHomeAssistantEntityID(rec.Name)
			if entityID == "" {
				continue
			}
			if strings.HasSuffix(entityID, "_node") {
				entityID, _ = resolvedNodeEntityID(rec.Name, rec, admin)
			}
			if seen[entityID] {
				continue
			}
			seen[entityID] = true

			displayName := applyListCleanOps(rec.Identity.Sphere+"/"+rec.Identity.Path, decl.cleanOps)
			entries = append(entries, TListEntry{entityID, displayName})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].displayName < entries[j].displayName
	})
	return entries
}

// isLeafDerivedAggregate returns true when a derived binary_sensor subdomain aggregate should
// appear in the list. It checks two conditions:
//  1. At least one pattern has a matching sphere filter and the record matches that pattern.
//  2. The space has a non-derived entity whose domain and path are EXACTLY the same as the
//     aggregate's own path. This identifies leaf groups (e.g. social/apartment/bedroom/door
//     wrapping physical/apartment/bedroom/door) and excludes parent groups that only aggregate
//     child-space groups (e.g. social/apartment/living_room/door has no direct sensor with
//     that exact path — the living_room sensor has path apartment/living_room/living/door).
func isLeafDerivedAggregate(rec TEntityRecord, spaceName string, patterns []TListPattern, admin *TAdministrationState) bool {
	for _, p := range patterns {
		if p.sphere == "" {
			continue // only apply this logic for sphere-filtered patterns
		}
		if rec.Identity.Domain != p.domain || rec.Identity.Sphere != p.sphere {
			continue
		}
		if p.pathSuffix != "" {
			path := rec.Identity.Path
			if path != p.pathSuffix && !strings.HasSuffix(path, "/"+p.pathSuffix) {
				continue
			}
		}
		// Pattern matches. Require a non-derived sibling with the exact same path as the
		// aggregate entity (not merely the same suffix), so parent aggregates are excluded.
		for _, sibling := range admin.EntityRecordsBySpace[spaceName] {
			if !strings.HasPrefix(sibling.Provenance, "derived") &&
				sibling.Identity.Domain == p.domain &&
				sibling.Identity.Path == rec.Identity.Path {
				return true
			}
		}
	}
	return false
}

func matchesAnyListPattern(rec TEntityRecord, patterns []TListPattern) bool {
	for _, p := range patterns {
		if rec.Identity.Domain != p.domain {
			continue
		}
		if p.sphere != "" && rec.Identity.Sphere != p.sphere {
			continue
		}
		if p.pathSuffix == "" {
			return true
		}
		path := rec.Identity.Path
		if p.wildcardLeaf {
			// p.pathSuffix must appear immediately before exactly one more trailing leaf
			// segment -- "prefix/<anything>" at the start of path, or "…/prefix/<anything>"
			// anywhere later in it. A bare path ending in just "prefix" (no leaf) doesn't count.
			if strings.HasPrefix(path, p.pathSuffix+"/") || strings.Contains(path, "/"+p.pathSuffix+"/") {
				return true
			}
			continue
		}
		if path == p.pathSuffix || strings.HasSuffix(path, "/"+p.pathSuffix) {
			return true
		}
	}
	return false
}

func applyListCleanOps(name string, ops []TListCleanOp) string {
	for _, op := range ops {
		switch op.kind {
		case "prefix":
			prefix := op.value + "/"
			if strings.HasPrefix(name, prefix) {
				name = name[len(prefix):]
			}
		case "postfix":
			postfix := "/" + op.value
			if strings.HasSuffix(name, postfix) {
				name = name[:len(name)-len(postfix)]
			}
		}
	}
	return name
}

func buildListFileYAML(title string, entries []TListEntry) string {
	var sb strings.Builder
	sb.WriteString("entities:\n")
	for _, e := range entries {
		sb.WriteString("  - entity: " + e.entityID + "\n")
		sb.WriteString("    name: '" + e.displayName + "'\n")
	}
	sb.WriteString("title: " + title + "\n")
	sb.WriteString("state_color: true\n")
	sb.WriteString("type: entities\n")
	sb.WriteString("show_header_toggle: false\n")
	return sb.String()
}

// buildSensorGraphListFileYAML renders entries as a Lovelace vertical-stack card containing one
// built-in "sensor" mini-graph card per entity -- the "as cards" list variant (parseListDeclarations),
// an alternative to buildListFileYAML's flat "entities" list. Uses the same TListEntry.displayName
// (full cleaned path, via the declaration's own clean_prefix/clean_postfix ops) as the flat form --
// only the rendering shape differs, not how names are derived. No "title:" field: unlike the
// "entities" card type, a vertical-stack has no title slot of its own in Lovelace.
func buildSensorGraphListFileYAML(entries []TListEntry, detailLevel int) string {
	var sb strings.Builder
	sb.WriteString("cards:\n")
	for _, e := range entries {
		sb.WriteString("  - detail: " + strconv.Itoa(detailLevel) + "\n")
		sb.WriteString("    entity: " + e.entityID + "\n")
		sb.WriteString("    graph: line\n")
		sb.WriteString("    name: '" + e.displayName + "'\n")
		sb.WriteString("    type: sensor\n")
	}
	sb.WriteString("type: vertical-stack\n")
	return sb.String()
}

func listTitleToFileName(title string) string {
	return strings.ToLower(strings.ReplaceAll(title, " ", "_"))
}
