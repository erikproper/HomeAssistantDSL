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
 * Entity body properties (condition, REST import, cli_sensor, cli_switch, adjustment,
 * input_number, scripts, automations) are Phase 2.
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
	outputDir := filepath.Join(cwd, "hass")
	listOutputDir := cwd
	label := filepath.Base(cwd)

	if err := generateFromPaths(definitionDir, sharedDefinitionDir, outputDir, listOutputDir, label); err != nil {
		return err
	}
	fmt.Printf("generated %s\n", label)
	return nil
}

// generateFromPaths contains the full generation pipeline given resolved directory paths.
func generateFromPaths(definitionDir, sharedDefinitionDir, outputDir, listOutputDir, label string) error {
	fmt.Printf("%s: reading configuration...\n", label)

	ctx, err := loadMacroContext(sharedDefinitionDir)
	if err != nil {
		return err
	}

	sharedSettingsBytes, _ := os.ReadFile(filepath.Join(sharedDefinitionDir, "Settings.def"))
	localSettingsBytes, _ := os.ReadFile(filepath.Join(definitionDir, "Settings.def"))
	combinedSettings := string(sharedSettingsBytes) + "\n" + string(localSettingsBytes)
	trustedProxies := parseHTTPProxies(combinedSettings)
	ctx.Settings = parseDefinitionAssignments(combinedSettings)

	fmt.Printf("%s: interpreting entities...\n", label)

	entitiesPath := filepath.Join(definitionDir, "Entities.def")
	entitiesContent, err := os.ReadFile(entitiesPath)
	if err != nil {
		return fmt.Errorf("error reading entities: %w", err)
	}

	var report strings.Builder
	parseResult, err := ParseEntitiesAndFillAdministration(
		strings.Split(string(entitiesContent), "\n"), entitiesPath, ctx, &report)
	if err != nil {
		return err
	}
	admin := parseResult.Administration
	admin.TrustedProxies = trustedProxies
	if bridgeTargets, bridgeErr := resolveBridgeTargets(definitionDir); bridgeErr == nil {
		admin.BridgeTargets = bridgeTargets
	}

	fmt.Printf("%s: generating YAML...\n", label)

	if err := os.RemoveAll(outputDir); err != nil {
		return fmt.Errorf("cannot clean output directory: %w", err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("cannot create output directory: %w", err)
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
		{"customization", generateCustomizationFiles},
		{"rest imported sensors", generateRestImportedSensors},
		{"cli sensors", generateCliSensors},
		{"cli switches", generateCliSwitches},
		{"input numbers", generateInputNumbers},
		{"input booleans", generateInputBooleans},
		{"input datetimes", generateInputDatetimes},
		{"template sensors", generateTemplateSensors},
		{"condition entities", generateConditionEntities},
		{"infrastructural binary sensor groups", generateInfrastructuralGroups},
		{"media switches", generateMediaSwitches},
		{"heating scripts", generateHeatingScripts},
		{"cover scripts", generateCoverScripts},
		{"derived binary sensors", generateDerivedBinarySensors},
		{"automations", generateAutomations},
		{"http integration", generateHTTPIntegration},
	}
	for _, step := range steps {
		if err := step.fn(outputDir, admin); err != nil {
			return fmt.Errorf("%s: %w", step.name, err)
		}
	}

	listsDefPath := filepath.Join(definitionDir, "Lists.def")
	if listsContent, readErr := os.ReadFile(listsDefPath); readErr == nil {
		if err := generateListFiles(listOutputDir, listsContent, admin); err != nil {
			return fmt.Errorf("list files: %w", err)
		}
	}

	runPostGenerationChecks(definitionDir, outputDir, label, admin)

	return nil
}


// --- configuration.yaml ---

const configurationYAMLBody = "default_config:\n\nhomeassistant:\n  packages: !include_dir_named integrations\n  customize: !include_dir_merge_named customization\n"

func generateConfigurationFile(outputDir string, _ *TAdministrationState) error {
	return writeYAMLFile(filepath.Join(outputDir, "configuration.yaml"), generatorHeader+configurationYAMLBody)
}

// --- integration aggregator files ---

// integrationDefs maps integration filename to !include directive content.
var integrationDefs = []struct{ file, content string }{
	{"automation.yaml", "automation: !include_dir_merge_list ../automation"},
	{"binary_sensor.yaml", "binary_sensor: !include_dir_list ../entities/binary_sensor"},
	{"command_line.yaml", "command_line: !include_dir_merge_list ../entities/command_line"},
	{"input_boolean.yaml", "input_boolean: !include_dir_merge_named ../entities/input_boolean"},
	{"input_datetime.yaml", "input_datetime: !include_dir_merge_named ../entities/input_datetime"},
	{"input_number.yaml", "input_number: !include_dir_merge_named ../entities/input_number"},
	{"light.yaml", "light: !include_dir_list ../entities/light"},
	{"scene.yaml", "scene: !include_dir_merge_list ../scene"},
	{"script.yaml", "script: !include_dir_merge_named ../script"},
	{"sensor.yaml", "sensor: !include_dir_list ../entities/sensor"},
	{"switch.yaml", "switch: !include_dir_list ../entities/switch"},
	{"template.yaml", "template: !include_dir_list ../entities/template"},
	{"timer.yaml", "timer: !include_dir_merge_named ../entities/timer"},
	{"tts.yaml", "tts:\n  - platform: google_translate"},
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
		if slices.Equal(sortedCopy(turnOnItems), baseOffItems) {
			content := buildLightGroupYAML(entityID, displayName, entityNamesToIDs(offItems))
			dir := filepath.Join(outputDir, "entities", "light", identity.Sphere)
			if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
				return err
			}
		} else {
			content := buildTemplateLightYAML(entityID, displayName, turnOnItems, offItems)
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
		content := buildTemplateLightYAML(entityID, displayName, items, items)
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

func buildTemplateLightYAML(entityID, displayName string, turnOnItems, turnOffItems []string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("light:\n")
	sb.WriteString("- name: " + displayName + "\n")
	sb.WriteString("  unique_id: " + entityID + "\n")
	sb.WriteString("  default_entity_id: " + entityID + "\n")
	sb.WriteString("  icon: mdi:lightbulb-group\n")

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
		content := buildTemplateSwitchYAML(entityID, displayName, onItems, offItems, explicitOnItems)
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
		content := buildTemplateSwitchYAML(entityID, displayName, onItems, offItems, nil)
		dir := filepath.Join(outputDir, "entities", "template", "switch", sphere)
		if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
			return err
		}
	}
	return nil
}

func buildTemplateSwitchYAML(entityID, displayName string, onItems, offItems, explicitOnItems []string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("switch:\n")
	sb.WriteString("- name: " + displayName + "\n")
	sb.WriteString("  unique_id: " + entityID + "\n")
	sb.WriteString("  default_entity_id: " + entityID + "\n")
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
			repID := resolveNodeRepresentative(ir.name, ir.rec, admin)
			if repID == "" {
				continue
			}
			// Extract domain from representative entity ID (e.g. "light" from "light.social_...").
			repDomain := ""
			if dotIdx := strings.Index(repID, "."); dotIdx > 0 {
				repDomain = repID[:dotIdx]
			}
			// Old system inserts the representative's domain before "node" for light entities.
			nodeEntityID := entityID
			nodePath := path
			if repDomain == "light" {
				nodeEntityID = strings.TrimSuffix(entityID, "_node") + "_light_node"
				nodePath = strings.TrimSuffix(path, "/node") + "/light/node"
			}
			generatedNodeEntityIDs[nodeEntityID] = true
			dir := filepath.Join(outputDir, "entities", "template", "binary_sensor", "infrastructural")
			customDir := filepath.Join(outputDir, "customization", "binary_sensor", "infrastructural")
			if ir.rec.NodeEnablerEntityID != "" {
				// Enabler case: generate _node_raw (plain availability) and _node (enabler logic).
				rawEntityID := strings.TrimSuffix(nodeEntityID, "_node") + "_node_raw"
				rawPath := strings.TrimSuffix(nodePath, "/node") + "/node/raw"
				rawContent := buildTemplateNodeRawYAML(rawEntityID, sphere+"/"+rawPath, repID)
				if err := writeYAMLFile(filepath.Join(dir, rawEntityID+".yaml"), rawContent); err != nil {
					return err
				}
				nodeContent := buildTemplateNodeWithRawYAML(nodeEntityID, sphere+"/"+nodePath, rawEntityID, ir.rec.NodeEnablerEntityID, ir.rec.NodeDelayOff)
				if err := writeYAMLFile(filepath.Join(dir, nodeEntityID+".yaml"), nodeContent); err != nil {
					return err
				}
			} else {
				nodeContent := buildTemplateNodeYAML(nodeEntityID, sphere+"/"+nodePath, repID)
				if err := writeYAMLFile(filepath.Join(dir, nodeEntityID+".yaml"), nodeContent); err != nil {
					return err
				}
			}
			// Customization for the node entity itself.
			nodeCustom := buildCustomizationYAML(nodeEntityID, sphere+"/"+nodePath, "mdi:server-network")
			if err := writeYAMLFile(filepath.Join(customDir, nodeEntityID+".yaml"), nodeCustom); err != nil {
				return err
			}
			// Generate node_alert and its customization for every node.
			alertEntityID := strings.TrimSuffix(nodeEntityID, "_node") + "_node_alert"
			alertPath := strings.TrimSuffix(nodePath, "/node") + "/node_alert"
			alertContent := buildTemplateNodeAlertYAML(alertEntityID, sphere+"/"+alertPath, nodeEntityID)
			if err := writeYAMLFile(filepath.Join(dir, alertEntityID+".yaml"), alertContent); err != nil {
				return err
			}
			alertCustom := buildCustomizationYAML(alertEntityID, sphere+"/"+alertPath, "mdi:server-off")
			if err := writeYAMLFile(filepath.Join(customDir, alertEntityID+".yaml"), alertCustom); err != nil {
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

	// Generate node_alert for /node entities that were declared without a body (no template node file).
	// These are entities like "entity binary_sensor.infrastructural:X/node;" or those imported via REST.
	for _, ir := range infraRecords {
		if !strings.HasSuffix(ir.rec.Identity.Path, "/node") {
			continue
		}
		entityID := toHomeAssistantEntityID(ir.name)
		path := ir.rec.Identity.Path
		repID := resolveNodeRepresentative(ir.name, ir.rec, admin)
		if repID != "" {
			// Already handled above (has representative → node + node_alert both generated).
			continue
		}
		// No representative: generate only node_alert (the node entity itself comes from elsewhere).
		alertEntityID := strings.TrimSuffix(entityID, "_node") + "_node_alert"
		alertPath := ir.rec.Identity.Sphere + "/" + strings.TrimSuffix(path, "/node") + "/node_alert"
		alertContent := buildTemplateNodeAlertYAML(alertEntityID, alertPath, entityID)
		dir := filepath.Join(outputDir, "entities", "template", "binary_sensor", "infrastructural")
		if err := writeYAMLFile(filepath.Join(dir, alertEntityID+".yaml"), alertContent); err != nil {
			return err
		}
		// Customization for this node_alert too.
		customDir := filepath.Join(outputDir, "customization", "binary_sensor", "infrastructural")
		alertCustom := buildCustomizationYAML(alertEntityID, alertPath, "mdi:server-off")
		if err := writeYAMLFile(filepath.Join(customDir, alertEntityID+".yaml"), alertCustom); err != nil {
			return err
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
			alertEntityID := strings.TrimSuffix(nodeEntityID, "_node") + "_node_alert"
			alertDisplayPath := "infrastructural/" + strings.TrimSuffix(spacePath, "/node") + "/node_alert"
			dir := filepath.Join(outputDir, "entities", "template", "binary_sensor", "infrastructural")
			customDir := filepath.Join(outputDir, "customization", "binary_sensor", "infrastructural")
			nodeContent := buildTemplateNodeYAML(nodeEntityID, displayPath, mediaPlayerHAID)
			if err := writeYAMLFile(filepath.Join(dir, nodeEntityID+".yaml"), nodeContent); err != nil {
				return err
			}
			nodeCustom := buildCustomizationYAML(nodeEntityID, displayPath, "mdi:server-network")
			if err := writeYAMLFile(filepath.Join(customDir, nodeEntityID+".yaml"), nodeCustom); err != nil {
				return err
			}
			alertContent := buildTemplateNodeAlertYAML(alertEntityID, alertDisplayPath, nodeEntityID)
			if err := writeYAMLFile(filepath.Join(dir, alertEntityID+".yaml"), alertContent); err != nil {
				return err
			}
			alertCustom := buildCustomizationYAML(alertEntityID, alertDisplayPath, "mdi:server-off")
			if err := writeYAMLFile(filepath.Join(customDir, alertEntityID+".yaml"), alertCustom); err != nil {
				return err
			}
		}
	}

	// Static on/off infrastructure sensors — present in every house.
	dir := filepath.Join(outputDir, "entities", "template", "binary_sensor", "infrastructural")
	for _, which := range []string{"on", "off"} {
		id := "binary_sensor.infrastructural_" + which
		content := generatorHeader + "binary_sensor:\n" +
			"- name: infrastructural/" + which + "\n" +
			"  unique_id: " + id + "\n" +
			"  state: \"{{ '" + which + "' }}\"\n"
		if err := writeYAMLFile(filepath.Join(dir, id+".yaml"), content); err != nil {
			return err
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

func buildTemplateNodeYAML(entityID, displayPath, representativeID string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("binary_sensor:\n")
	sb.WriteString("- name: " + displayPath + "\n")
	sb.WriteString("  unique_id: " + entityID + "\n")
	sb.WriteString("  device_class: connectivity\n")
	sb.WriteString("  state: \"{{ states('" + representativeID + "') != 'unavailable' }}\"\n")
	return sb.String()
}

// buildTemplateNodeRawYAML generates the intermediate _node_raw entity for the media-player-with-enabler case.
// It checks plain availability of the representative without device_class or delay.
func buildTemplateNodeRawYAML(entityID, displayPath, representativeID string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("binary_sensor:\n")
	sb.WriteString("- name: " + displayPath + "\n")
	sb.WriteString("  unique_id: " + entityID + "\n")
	sb.WriteString("  state: \"{{ states('" + representativeID + "') != 'unavailable' }}\"\n")
	return sb.String()
}

// buildTemplateNodeWithRawYAML generates the _node entity that combines an enabler switch and a _node_raw entity.
func buildTemplateNodeWithRawYAML(entityID, displayPath, rawEntityID, enablerID, delayOff string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("binary_sensor:\n")
	sb.WriteString("- name: " + displayPath + "\n")
	sb.WriteString("  unique_id: " + entityID + "\n")
	sb.WriteString("  device_class: connectivity\n")
	sb.WriteString("  state: \"{{ is_state('" + enablerID + "', 'off') or is_state('" + rawEntityID + "', 'on') }}\"\n")
	if delayOff != "" {
		sb.WriteString("  delay_off: " + delayOff + "\n")
	}
	return sb.String()
}

func buildTemplateNodeAlertYAML(entityID, displayPath, nodeEntityID string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("binary_sensor:\n")
	sb.WriteString("- name: " + displayPath + "\n")
	sb.WriteString("  unique_id: " + entityID + "\n")
	sb.WriteString("  device_class: connectivity\n")
	sb.WriteString("  state: \"{{ is_state('" + nodeEntityID + "', 'off') }}\"\n")
	return sb.String()
}

func buildTemplateBatteryAlertYAML(entityID, displayPath, batteryLevelEntityID string, alertLevel int) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("binary_sensor:\n")
	sb.WriteString("- name: " + displayPath + "\n")
	sb.WriteString("  unique_id: " + entityID + "\n")
	sb.WriteString("  device_class: battery\n")
	sb.WriteString(fmt.Sprintf("  state: \"{{ (states('%s') | int(0)) < %d }}\"\n", batteryLevelEntityID, alertLevel))
	return sb.String()
}

// --- REST-imported sensor and binary_sensor files ---

// restSensorSubdomainProps maps the last path segment of an imported REST sensor to its
// HA platform properties. Fields left empty are omitted from the generated YAML.
var restSensorSubdomainProps = map[string]struct {
	DeviceClass string
	Unit        string
	StateClass  string
}{
	"battery_level": {"battery", "%", "measurement"},
	"co2":           {"carbon_dioxide", "ppm", "measurement"},
	"humidity":      {"", "%", "measurement"},
	"illuminance":   {"illuminance", "lx", "measurement"},
	"noise":         {"signal_strength", "dB", "measurement"},
	"pressure":      {"atmospheric_pressure", "mbar", "measurement"},
	"temperature":   {"temperature", "°C", "measurement"},
	"wind_speed":    {"wind_speed", "km/h", "measurement"},
}

// generateRestImportedSensors writes entities/sensor/<sphere>/ and
// entities/binary_sensor/<sphere>/ YAML files for each "imported rest" directive.
func generateRestImportedSensors(outputDir string, admin *TAdministrationState) error {
	for _, rec := range admin.RestImports {
		bridge, ok := admin.BridgeTargets[rec.BridgeName]
		if !ok {
			continue
		}
		id := toHomeAssistantEntityID(rec.LocalEntityName)
		if id == "" {
			continue
		}
		identity := extractEntityIdentity(rec.LocalEntityName)
		statesPath := bridge.StatesPath
		if statesPath == "" {
			statesPath = "/api/states"
		}
		resourceURL := strings.TrimSuffix(bridge.BaseURL, "/") + statesPath + "/" + rec.RemoteEntityID

		var content string
		if rec.ValueExpr != "" {
			// Binary sensor (reachability / connectivity) — value_template evaluates to bool.
			valTemplate := strings.ReplaceAll(rec.ValueExpr, "$", "value_json.state")
			displayName := identity.Sphere + "/" + identity.Path
			content = buildRestBinarySensorYAML(id, displayName, resourceURL, bridge.Token, valTemplate, rec.ScanInterval)
			dir := filepath.Join(outputDir, "entities", "binary_sensor", identity.Sphere)
			if err := writeYAMLFile(filepath.Join(dir, id+".yaml"), content); err != nil {
				return err
			}
		} else {
			// Regular sensor.
			sub := lastPathSegment(identity.Path)
			props := restSensorSubdomainProps[sub]
			icon, _ := lookupIcon(sub)
			displayName := identity.Sphere + "/" + identity.Path
			content = buildRestSensorYAML(id, displayName, resourceURL, bridge.Token, props.DeviceClass, props.Unit, props.StateClass, icon, rec.ScanInterval)
			dir := filepath.Join(outputDir, "entities", "sensor", identity.Sphere)
			if err := writeYAMLFile(filepath.Join(dir, id+".yaml"), content); err != nil {
				return err
			}
		}
	}
	return nil
}

func generateCliSensors(outputDir string, admin *TAdministrationState) error {
	for _, rec := range admin.CliSensors {
		id := toHomeAssistantEntityID(rec.LocalEntityName)
		if id == "" {
			continue
		}
		identity := extractEntityIdentity(rec.LocalEntityName)
		displayName := identity.Sphere + "/" + identity.Path
		sub := lastPathSegment(identity.Path)
		unit := restSensorSubdomainProps[sub].Unit
		cmd := fmt.Sprintf("bash /config/bin/run %s %s %s", rec.UserAlias, rec.HostFQDN, rec.ScriptPath)
		content := buildCliSensorYAML(displayName, cmd, unit)
		dir := filepath.Join(outputDir, "entities", "command_line", "sensor", identity.Sphere)
		if err := writeYAMLFile(filepath.Join(dir, id+".yaml"), content); err != nil {
			return err
		}
	}
	return nil
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

func buildCliSensorYAML(displayName, cmd, unit string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- sensor:\n")
	sb.WriteString("    name: " + displayName + "\n")
	sb.WriteString("    command: \"" + cmd + "\"\n")
	sb.WriteString("    command_timeout: 15\n")
	sb.WriteString("    scan_interval: 60\n")
	if unit != "" {
		sb.WriteString("    unit_of_measurement: " + unit + "\n")
	}
	return sb.String()
}

func buildCliSwitchYAML(id, displayName, userAlias, hostFQDN, onScript, offScript, stateScript string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- switch:\n")
	sb.WriteString("    name: " + displayName + "\n")
	sb.WriteString("    unique_id: " + id + "\n")
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
			content := buildInputNumberYAML(key, displayName, rec.InputNumberMin, rec.InputNumberMax, rec.InputNumberStep, rec.InputNumberUnit, rec.InputNumberIcon)
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
			content := buildInputNumberYAML(key, displayName, "10", "30", "1", "°C", "mdi:thermometer")
			dir := filepath.Join(outputDir, "entities", "input_number", "physical")
			if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
				return err
			}
		}
	}
	return nil
}

func buildInputNumberYAML(key, displayName, min, max, step, unit, icon string) string {
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
	const dummyID = "input_boolean.infrastructural_dummy"
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("infrastructural_dummy:\n")
	sb.WriteString("  name: infrastructural/dummy\n")
	sb.WriteString("  initial: off\n")
	dir := filepath.Join(outputDir, "entities", "input_boolean", "infrastructural")
	return writeYAMLFile(filepath.Join(dir, dummyID+".yaml"), sb.String())
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

func generateTemplateSensors(outputDir string, admin *TAdministrationState) error {
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain != "sensor" {
				continue
			}
			if rec.AdjustmentOffset == "" && rec.ValueExpr == "" {
				continue
			}
			id := toHomeAssistantEntityID(rec.Name)
			if id == "" {
				continue
			}
			identity := rec.Identity
			displayName := identity.Sphere + "/" + identity.Path
			sub := lastPathSegment(identity.Path)
			props := restSensorSubdomainProps[sub]
			icon, _ := lookupIcon(sub)
			var content string
			if rec.AdjustmentOffset != "" {
				rawID := id + "_raw"
				stateExpr := buildAdjustmentStateExpr(rawID, rec.AdjustmentOffset, rec.AdjustmentScale)
				content = buildTemplateSensorYAML(id, displayName, props.Unit, props.DeviceClass, props.StateClass, icon, stateExpr)
			} else {
				stateExpr := buildValueStateExpr(rec.ValueExpr, admin.SpaceOrder, admin)
				content = buildTemplateSensorYAML(id, displayName, props.Unit, props.DeviceClass, props.StateClass, icon, stateExpr)
			}
			dir := filepath.Join(outputDir, "entities", "template", "sensor", identity.Sphere)
			if err := writeYAMLFile(filepath.Join(dir, id+".yaml"), content); err != nil {
				return err
			}
		}
	}
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
				content := buildTemplateSensorYAML(entityID, displayName, "°C", "temperature", "measurement", "mdi:thermometer", stateExpr)
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
				content = buildConditionBinarySensorYAML(id, displayName, stateExpr, rec.ConditionDevClass, rec.ConditionDelayOn, rec.ConditionDelayOff)
			case "sensor":
				sub := lastPathSegment(identity.Path)
				props := restSensorSubdomainProps[sub]
				icon, _ := lookupIcon(sub)
				content = buildTemplateSensorYAML(id, displayName, props.Unit, props.DeviceClass, props.StateClass, icon, stateExpr)
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
// A plain token produces states('entity_id'); one with "!attr" produces state_attr('entity_id', 'attr').
func sourceToJinja2(src string) string {
	if bangIdx := strings.Index(src, "!"); bangIdx > 0 {
		entityID := src[:bangIdx]
		attr := src[bangIdx+1:]
		return fmt.Sprintf("state_attr('%s', '%s')", entityID, attr)
	}
	return fmt.Sprintf("states('%s')", src)
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

func buildConditionBinarySensorYAML(id, displayName, stateExpr, deviceClass, delayOn, delayOff string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("binary_sensor:\n")
	sb.WriteString("- name: " + displayName + "\n")
	sb.WriteString("  unique_id: " + id + "\n")
	if deviceClass != "" {
		sb.WriteString("  device_class: " + deviceClass + "\n")
	}
	if delayOn != "" {
		sb.WriteString("  delay_on: " + delayOn + "\n")
	}
	if delayOff != "" {
		sb.WriteString("  delay_off: " + delayOff + "\n")
	}
	sb.WriteString("  state: \"{{ " + stateExpr + " }}\"\n")
	return sb.String()
}

func buildAdjustmentStateExpr(rawID, offset, scale string) string {
	if scale == "1" || scale == "" {
		return fmt.Sprintf("states('%s') | float(0) + %s", rawID, offset)
	}
	return fmt.Sprintf("states('%s') | float(0) * %s + %s", rawID, scale, offset)
}

func buildValueStateExpr(valueExpr string, spaceOrder []string, admin *TAdministrationState) string {
	exclamIdx := strings.Index(valueExpr, "!")
	if exclamIdx < 0 {
		entityID := toHomeAssistantEntityID(valueExpr)
		return fmt.Sprintf("states('%s')", entityID)
	}
	entitySpec := valueExpr[:exclamIdx]
	attribute := valueExpr[exclamIdx+1:]
	entityID := toHomeAssistantEntityID(entitySpec)
	return fmt.Sprintf("state_attr('%s', '%s')", entityID, attribute)
}

func buildTemplateSensorYAML(id, displayName, unit, deviceClass, stateClass, icon, stateExpr string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("sensor:\n")
	sb.WriteString("- name: " + displayName + "\n")
	sb.WriteString("  unique_id: " + id + "\n")
	if unit != "" {
		sb.WriteString("  unit_of_measurement: \"" + unit + "\"\n")
	}
	if deviceClass != "" {
		sb.WriteString("  device_class: " + deviceClass + "\n")
	}
	if stateClass != "" {
		sb.WriteString("  state_class: " + stateClass + "\n")
	}
	sb.WriteString("  state: \"{{ " + stateExpr + " }}\"\n")
	return sb.String()
}

func buildRestSensorYAML(id, displayName, resourceURL, token, deviceClass, unit, stateClass, icon string, scanInterval int) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("platform: rest\n")
	sb.WriteString("resource: " + resourceURL + "\n")
	sb.WriteString("name: " + displayName + "\n")
	sb.WriteString("value_template: \"{{ value_json.state }}\"\n")
	sb.WriteString("unique_id: " + id + "\n")
	if unit != "" {
		sb.WriteString("unit_of_measurement: \"" + unit + "\"\n")
	}
	if deviceClass != "" {
		sb.WriteString("device_class: " + deviceClass + "\n")
	}
	if stateClass != "" {
		sb.WriteString("state_class: " + stateClass + "\n")
	}
	if icon != "" {
		sb.WriteString("icon: " + icon + "\n")
	}
	sb.WriteString(fmt.Sprintf("scan_interval: %d\n", scanInterval))
	sb.WriteString("headers:\n")
	sb.WriteString("  authorization: Bearer " + token + "\n")
	sb.WriteString("  content-type: \"application/json\"\n")
	return sb.String()
}

func buildRestBinarySensorYAML(id, displayName, resourceURL, token, valueTemplate string, scanInterval int) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("platform: rest\n")
	sb.WriteString("resource: " + resourceURL + "\n")
	sb.WriteString("name: " + displayName + "\n")
	sb.WriteString("value_template: \"{{ " + valueTemplate + " }}\"\n")
	sb.WriteString("unique_id: " + id + "\n")
	sb.WriteString("device_class: connectivity\n")
	sb.WriteString("icon: mdi:server-network\n")
	sb.WriteString(fmt.Sprintf("scan_interval: %d\n", scanInterval))
	sb.WriteString("headers:\n")
	sb.WriteString("  authorization: Bearer " + token + "\n")
	sb.WriteString("  content-type: \"application/json\"\n")
	return sb.String()
}

// --- infrastructural binary sensor aggregate groups ---

// generateInfrastructuralGroups writes binary_sensor/infrastructural/binary_sensor.infrastructural_battery_alert.yaml
// and binary_sensor.infrastructural_node_alert.yaml, listing all matching entities sorted alphabetically.
func generateInfrastructuralGroups(outputDir string, admin *TAdministrationState) error {
	var batteryAlerts, nodeAlerts []string
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
				// node_alert is auto-derived for every /node entity (generated by the template step).
				// For light-domain representatives the entity is renamed _light_node / _light_node_alert.
				repID := resolveNodeRepresentative(rec.Name, rec, admin)
				repDomain := ""
				if dotIdx := strings.Index(repID, "."); dotIdx > 0 {
					repDomain = repID[:dotIdx]
				}
				base := strings.TrimSuffix(id, "_node")
				if repDomain == "light" {
					nodeAlerts = append(nodeAlerts, base+"_light_node_alert")
				} else {
					nodeAlerts = append(nodeAlerts, base+"_node_alert")
				}
			}
		}
	}
	// Add node_alert for media_player entities that have no explicit infra node entity.
	// generateTemplateBinarySensors synthesises a node/node_alert pair for every media_player
	// that was not already covered by the providing macro; include those here too.
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
			nodeAlerts = append(nodeAlerts, strings.TrimSuffix(nodeEntityID, "_node")+"_node_alert")
		}
	}
	sort.Strings(batteryAlerts)
	sort.Strings(nodeAlerts)

	dir := filepath.Join(outputDir, "entities", "binary_sensor", "infrastructural")
	customDir := filepath.Join(outputDir, "customization", "binary_sensor", "infrastructural")
	if len(batteryAlerts) > 0 {
		const baID = "binary_sensor.infrastructural_battery_alert"
		const baName = "infrastructural/battery_alert"
		content := buildInfraGroupYAML(baID, baName, batteryAlerts)
		if err := writeYAMLFile(filepath.Join(dir, baID+".yaml"), content); err != nil {
			return err
		}
		custom := buildCustomizationYAML(baID, baName, "mdi:battery-alert")
		if err := writeYAMLFile(filepath.Join(customDir, baID+".yaml"), custom); err != nil {
			return err
		}
	}
	if len(nodeAlerts) > 0 {
		const naID = "binary_sensor.infrastructural_node_alert"
		const naName = "infrastructural/node_alert"
		content := buildInfraGroupYAML(naID, naName, nodeAlerts)
		if err := writeYAMLFile(filepath.Join(dir, naID+".yaml"), content); err != nil {
			return err
		}
		custom := buildCustomizationYAML(naID, naName, "mdi:server-off")
		if err := writeYAMLFile(filepath.Join(customDir, naID+".yaml"), custom); err != nil {
			return err
		}
	}
	return nil
}

func buildInfraGroupYAML(uniqueID, name string, entities []string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("platform: group\n")
	sb.WriteString("name: " + name + "\n")
	sb.WriteString("unique_id: " + uniqueID + "\n")
	sb.WriteString("entities:\n")
	for _, e := range entities {
		sb.WriteString("- " + e + "\n")
	}
	return sb.String()
}

// --- binary sensor group (heating leakage evidence) ---

// generateBinarySensorGroups writes entities/binary_sensor/physical/<id>.yaml for every space
// with a climate entity or a physical heating switch. The group sensor is true when any listed
// door/window is open (i.e. in the 'on' state for a binary_sensor); a space with no explicit
// "heating leak:" entries falls back to the always-off constant sensor, so leakage never fires.
func generateBinarySensorGroups(outputDir string, admin *TAdministrationState) error {
	for _, spaceName := range heatingCapableSocialSpaceNames(admin) {
		leaks := admin.HeatingLeaksByName[spaceName]
		if len(leaks) == 0 {
			leaks = []string{"binary_sensor.infrastructural/off"}
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
	sb.WriteString("unique_id: " + entityID + "\n")
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
			content := buildBinarySensorGroupYAML(groupID, displayName, entityNamesToIDs(sortedCopy(members)))
			dir := filepath.Join(outputDir, "entities", "binary_sensor", "social")
			if err := writeYAMLFile(filepath.Join(dir, groupID+".yaml"), content); err != nil {
				return err
			}
			derivedGroups[sub][spaceName] = groupName
		}
	}

	writeBinarySensorSphereGroup := func(groupID, displayName string, members []string) error {
		content := buildBinarySensorGroupYAML(groupID, displayName, entityNamesToIDs(sortedCopy(members)))
		dir := filepath.Join(outputDir, "entities", "binary_sensor", "social")
		if err := writeYAMLFile(filepath.Join(dir, groupID+".yaml"), content); err != nil {
			return err
		}
		sub := displayName[strings.LastIndex(displayName, "/")+1:]
		custom := buildCustomizationYAML(groupID, displayName, subdomainIcons[sub])
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
		content := buildSensorGroupYAML(groupID, displayName, memberIDs)
		dir := filepath.Join(outputDir, "entities", "sensor", "social")
		if err := writeYAMLFile(filepath.Join(dir, groupID+".yaml"), content); err != nil {
			return err
		}
		subdomain := displayName[strings.LastIndex(displayName, "/")+1:]
		customContent := buildCustomizationYAML(groupID, displayName, subdomainIcons[subdomain])
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
// collects from all social-sphere spaces (the sphere-level aggregate case).
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
	sb.WriteString("unique_id: " + entityID + "\n")
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
	sb.WriteString("unique_id: " + entityID + "\n")
	sb.WriteString("entities:\n")
	for _, id := range memberIDs {
		sb.WriteString("- " + id + "\n")
	}
	return sb.String()
}

// --- customization files ---

// subdomainIcons maps the last path segment (subdomain) of an entity to a default icon.
// Entity-specific icons from "icon:" body properties are not yet tracked and take precedence
// once Phase 2 adds Icon to TEntityRecord.
var subdomainIcons = map[string]string{
	"battery_alert":   "mdi:battery-alert",
	"battery_level":   "mdi:battery",
	"co2":             "mdi:cloud",
	"consumes":        "mdi:flash",
	"daylight":        "mdi:weather-sunset-up",
	"dishwasher":      "mdi:dishwasher",
	"door":            "mdi:door-open",
	"humidity":        "mdi:water-percent",
	"illuminance":     "mdi:brightness-5",
	"load":            "mdi:cpu-64-bit",
	"media":           "mdi:monitor-speaker",
	"motion":          "mdi:motion-sensor",
	"node":            "mdi:server-network",
	"noise":           "mdi:volume-high",
	"pressure":        "mdi:gauge",
	"radiator":        "mdi:heating-coil",
	"radio":           "mdi:signal",
	"sunny":           "mdi:sunglasses",
	"temperature":     "mdi:thermometer",
	"washing_machine": "mdi:washing-machine",
	"water":           "mdi:water-off",
	"wind_direction":  "mdi:compass-outline",
	"wind_speed":      "mdi:weather-windy",
	"windy":           "mdi:weather-windy",
}

// domainDefaultIcons maps HA entity domains to a fallback icon when no subdomain-specific
// icon applies.  These match the old bash generator's domain-level defaults.
var domainDefaultIcons = map[string]string{
	"cover": "mdi:blinds-horizontal",
	"light": "mdi:lightbulb-group",
}

// iconForEntity returns the icon string for an entity, first checking the last path segment
// against subdomainIcons, then falling back to a domain-level default.
// Returns "" when no icon is known (entity-specific icons require Phase 2 support).
func iconForEntity(rec TEntityRecord) string {
	if rec.EntityIcon != "" {
		return rec.EntityIcon
	}
	path := rec.Identity.Path
	if path != "" {
		parts := strings.Split(path, "/")
		subdomain := parts[len(parts)-1]
		if icon, ok := subdomainIcons[subdomain]; ok {
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
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.IsRaw || rec.Identity.Domain == "" || rec.Identity.Sphere == "" {
				continue
			}
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
			icon := iconForEntity(rec)
			content := buildCustomizationYAML(entityID, displayName, icon)
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

// --- http integration ---

// parseHTTPProxies scans settings content for "http proxies <cidr>, ...;" and returns
// the list of CIDR/IP entries.  Returns nil when no such directive is found.
func parseHTTPProxies(content string) []string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "http proxies ") {
			continue
		}
		rest := strings.TrimPrefix(trimmed, "http proxies ")
		rest = strings.TrimSuffix(rest, ";")
		var proxies []string
		for _, part := range strings.Split(rest, ",") {
			if p := strings.TrimSpace(part); p != "" {
				proxies = append(proxies, p)
			}
		}
		return proxies
	}
	return nil
}

// generateHTTPIntegration writes integrations/http.yaml when trusted proxies are configured.
func generateHTTPIntegration(outputDir string, admin *TAdministrationState) error {
	if len(admin.TrustedProxies) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("http:\n")
	sb.WriteString("  use_x_forwarded_for: true\n")
	sb.WriteString("  trusted_proxies:\n")
	for _, proxy := range admin.TrustedProxies {
		sb.WriteString("    - " + proxy + "\n")
	}
	intDir := filepath.Join(outputDir, "integrations")
	return writeYAMLFile(filepath.Join(intDir, "http.yaml"), sb.String())
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
			content := buildLeafMediaSwitchYAML(entityID, displayName, playerEntityID, rec.MediaSwitchNoPlayInput)
			dir := filepath.Join(outputDir, "entities", "template", "switch", rec.Identity.Sphere)
			if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
				return err
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
			// Build leaf switch name: strip "media_player." prefix and sphere/path up to the space,
			// then take the last segment as the player shortname within the space.
			playerPath := player[len("media_player."):]
			dotIdx := strings.Index(playerPath, "/")
			if dotIdx >= 0 {
				playerPath = playerPath[dotIdx+1:]
			}
			switchName := "switch." + spaceName + "/" + lastPathSegment(playerPath) + "/media"
			offItems = append(offItems, switchName)
		}
		offItems = append(offItems, sortedCopy(childMediaSwitches)...)

		displayName := entityName[len("switch."):]
		content := buildAggregateMediaSwitchYAML(entityID, displayName, offItems)
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
		content := buildAggregateMediaSwitchYAML(entityID, displayName, offItems)
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

func buildLeafMediaSwitchYAML(entityID, displayName, playerID, noPlayInput string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("switch:\n")
	sb.WriteString("- name: " + displayName + "\n")
	sb.WriteString("  unique_id: " + entityID + "\n")
	sb.WriteString("  default_entity_id: " + entityID + "\n")
	sb.WriteString("  turn_on:\n")
	sb.WriteString("  - entity_id:\n")
	sb.WriteString("    - input_boolean.infrastructural_dummy\n")
	sb.WriteString("    action: input_boolean.turn_on\n")
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

func buildAggregateMediaSwitchYAML(entityID, displayName string, offItems []string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("switch:\n")
	sb.WriteString("- name: " + displayName + "\n")
	sb.WriteString("  unique_id: " + entityID + "\n")
	sb.WriteString("  default_entity_id: " + entityID + "\n")
	sb.WriteString("  icon: mdi:monitor-speaker\n")
	sb.WriteString("  turn_on:\n")
	sb.WriteString("  - entity_id:\n")
	sb.WriteString("    - input_boolean.infrastructural_dummy\n")
	sb.WriteString("    action: input_boolean.turn_on\n")
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

// --- cover open/stop/close scripts ---

var coverActions = []struct{ name, service string }{
	{"open", "cover.open_cover"},
	{"close", "cover.close_cover"},
	{"stop", "cover.stop_cover"},
}

// generateCoverScripts writes script/<sphere>/script.<id>.yaml for every cover entity
// that carries the open_stop_close option.
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

//
// Each space that contains covers (directly or via descendants) gets a space script
// named <spacePath>_cover_<action>.  A cover entity "owns" a space when its full path
// (sphere/path) matches the space name; in that case the space script calls the HA
// cover service directly, avoiding redundant double-path naming.  When a cover is
// declared in a parent space (its path extends the space path), a separate leaf script
// is emitted and the parent space script delegates to it.
func generateCoverScripts(outputDir string, admin *TAdministrationState) error {
	// coversBySpace: direct cover entities with OpenStopClose per space.
	coversBySpace := map[string][]TEntityRecord{}
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain == "cover" && rec.OpenStopClose {
				coversBySpace[spaceName] = append(coversBySpace[spaceName], rec)
			}
		}
	}
	if len(coversBySpace) == 0 {
		return nil
	}

	// Determine which spaces are "cover spaces" (have covers directly or via descendants),
	// processing children before parents so propagation works bottom-up.
	coverSpaces := map[string]bool{}
	for i := len(admin.SpaceOrder) - 1; i >= 0; i-- {
		spaceName := admin.SpaceOrder[i]
		if len(coversBySpace[spaceName]) > 0 {
			coverSpaces[spaceName] = true
			continue
		}
		for _, child := range directChildSpaces(spaceName, admin.SpaceOrder) {
			if coverSpaces[child] {
				coverSpaces[spaceName] = true
				break
			}
		}
	}

	coverActionIcon := map[string]string{
		"open":  "mdi:triangle",
		"close": "mdi:triangle-down",
		"stop":  "mdi:rectangle",
	}

	for _, spaceName := range admin.SpaceOrder {
		if !coverSpaces[spaceName] {
			continue
		}

		sphere := spaceName
		if idx := strings.Index(spaceName, "/"); idx >= 0 {
			sphere = spaceName[:idx]
		}
		spaceUnder := strings.ReplaceAll(spaceName, "/", "_")
		scriptDir := filepath.Join(outputDir, "script", sphere)

		for _, act := range coverActions {
			var seq []struct{ service, entityID string }

			// Child cover spaces come first (in SpaceOrder).
			for _, child := range directChildSpaces(spaceName, admin.SpaceOrder) {
				if coverSpaces[child] {
					childUnder := strings.ReplaceAll(child, "/", "_")
					seq = append(seq, struct{ service, entityID string }{
						"script.turn_on",
						"script." + childUnder + "_cover_" + act.name,
					})
				}
			}

			// Direct covers: owning covers inline the service call; non-owning covers
			// get a separate leaf script delegated to via script.turn_on.
			for _, rec := range coversBySpace[spaceName] {
				coverID := toHomeAssistantEntityID(rec.Name)
				ownsSpace := rec.Identity.Sphere+"/"+rec.Identity.Path == spaceName
				if ownsSpace {
					seq = append(seq, struct{ service, entityID string }{act.service, coverID})
				} else {
					spacePathAfterSphere := strings.TrimPrefix(spaceName, rec.Identity.Sphere+"/")
					relPath := strings.TrimPrefix(rec.Identity.Path, spacePathAfterSphere+"/")
					leafID := spaceUnder + "_" + strings.ReplaceAll(relPath, "/", "_") + "_cover_" + act.name
					leafAlias := spaceName + "/" + relPath + "/cover_" + act.name
					leafContent := buildCoverScriptYAML(leafID, leafAlias,
						[]struct{ service, entityID string }{{act.service, coverID}})
					if err := writeYAMLFile(filepath.Join(scriptDir, "script."+leafID+".yaml"), leafContent); err != nil {
						return err
					}
					seq = append(seq, struct{ service, entityID string }{
						"script.turn_on", "script." + leafID,
					})
				}
			}

			spaceScriptID := spaceUnder + "_cover_" + act.name
			spaceAlias := spaceName + "/cover_" + act.name
			content := buildCoverScriptYAML(spaceScriptID, spaceAlias, seq)
			if err := writeYAMLFile(filepath.Join(scriptDir, "script."+spaceScriptID+".yaml"), content); err != nil {
				return err
			}
			customDir := filepath.Join(outputDir, "customization", "script", sphere)
			custom := buildCustomizationYAML("script."+spaceScriptID, spaceAlias, coverActionIcon[act.name])
			if err := writeYAMLFile(filepath.Join(customDir, "script."+spaceScriptID+".yaml"), custom); err != nil {
				return err
			}
		}
	}

	// Sphere-level scripts: for each sphere that has at least one direct-child cover
	// space, emit a <sphere>_cover_<action> script.  The sphere name itself is never an
	// entry in SpaceOrder, so it must be handled separately here.
	sphereChildren := map[string][]string{} // sphere → direct child space names with covers
	for _, spaceName := range admin.SpaceOrder {
		if !coverSpaces[spaceName] {
			continue
		}
		if strings.Count(spaceName, "/") != 1 {
			continue
		}
		sphere := spaceName[:strings.Index(spaceName, "/")]
		sphereChildren[sphere] = append(sphereChildren[sphere], spaceName)
	}
	spheres := sortedKeys(sphereChildren)
	for _, sphere := range spheres {
		scriptDir := filepath.Join(outputDir, "script", sphere)
		for _, act := range coverActions {
			var seq []struct{ service, entityID string }
			for _, child := range sphereChildren[sphere] {
				childUnder := strings.ReplaceAll(child, "/", "_")
				seq = append(seq, struct{ service, entityID string }{
					"script.turn_on", "script." + childUnder + "_cover_" + act.name,
				})
			}
			rollupID := sphere + "_cover_" + act.name
			rollupAlias := sphere + "/cover_" + act.name
			content := buildCoverScriptYAML(rollupID, rollupAlias, seq)
			if err := writeYAMLFile(filepath.Join(scriptDir, "script."+rollupID+".yaml"), content); err != nil {
				return err
			}
			customDir := filepath.Join(outputDir, "customization", "script", sphere)
			custom := buildCustomizationYAML("script."+rollupID, rollupAlias, coverActionIcon[act.name])
			if err := writeYAMLFile(filepath.Join(customDir, "script."+rollupID+".yaml"), custom); err != nil {
				return err
			}
		}
	}

	return nil
}

func buildCoverScriptYAML(scriptID, alias string, seq []struct{ service, entityID string }) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString(scriptID + ":\n")
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
		content := buildConditionBinarySensorYAML(entityID, displayName, stateExpr, "", "00:05:00", "")
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
		if err := writeYAMLFile(filepath.Join(customDir, dayTimeID+".yaml"), buildCustomizationYAML(dayTimeID, dayTimeDisplay, "mdi:sun-clock")); err != nil {
			return err
		}

		// Heating also gets a complementary night-time sensor.
		if domain == "heating" {
			nightID := toHomeAssistantEntityID("binary_sensor." + sphere + "/is_heating_night_time")
			nightDisplay := sphere + "/is_heating_night_time"
			nightState := "is_state('" + dayTimeID + "', 'off')"
			nightContent := buildConditionBinarySensorYAML(nightID, nightDisplay, nightState, "", "", "")
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
		if err := writeYAMLFile(filepath.Join(dir, acID+".yaml"), buildConditionBinarySensorYAML(acID, acDisplay, acState, "", "", "")); err != nil {
			return err
		}
		if err := writeYAMLFile(filepath.Join(customDir, acID+".yaml"), buildCustomizationYAML(acID, acDisplay, "mdi:dots-vertical-circle")); err != nil {
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
		if err := writeYAMLFile(filepath.Join(dir, scID+".yaml"), buildConditionBinarySensorYAML(scID, scDisplay, scState, "", "", "")); err != nil {
			return err
		}
		if err := writeYAMLFile(filepath.Join(customDir, scID+".yaml"), buildCustomizationYAML(scID, scDisplay, "mdi:triangle-down")); err != nil {
			return err
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
		content := buildHeatingShouldBeOffYAML(entityID, displayName, leakageIdentifiedID, leakageIgnoreID, heatingEnableID)
		dir := filepath.Join(outputDir, "entities", "template", "binary_sensor", "physical")
		if err := writeYAMLFile(filepath.Join(dir, entityID+".yaml"), content); err != nil {
			return err
		}
		custom := buildCustomizationYAML(entityID, displayName, "mdi:radiator-off")
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
	sb.WriteString("  unique_id: " + entityID + "\n")
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
			if rec.Identity.Domain != "climate" || rec.Identity.IsRaw {
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
		if rec.Identity.IsRaw || rec.Identity.Sphere == "" {
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
		closeScriptID := "script." + sphere + "_cover_close"
		openScriptID := "script." + sphere + "_cover_open"

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
	sb.WriteString("  unique_id: " + entityID + "\n")
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
	sb.WriteString("  unique_id: " + entityID + "\n")
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
	sb.WriteString("  unique_id: " + entityID + "\n")
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

func generateFollowsAutomations(outputDir string, admin *TAdministrationState) error {
	for _, rel := range admin.FollowsRelations {
		followerID := toHomeAssistantEntityID(rel.Follower)
		leaderID := toHomeAssistantEntityID(rel.Leader)

		followerPath := rel.Follower[strings.Index(rel.Follower, ".")+1:]
		leaderPath := rel.Leader[strings.Index(rel.Leader, ".")+1:]
		leaderDomain := rel.Leader[:strings.Index(rel.Leader, ".")]

		sphere := entitySphere(rel.Follower)

		for _, dir := range []string{"turn_on", "turn_off"} {
			aliasPath := followerPath + "/follows/" + leaderDomain + "/" + leaderPath + "/" + dir
			automationID := strings.ReplaceAll(aliasPath, "/", "_")
			service := leaderDomain + "." + dir

			var sb strings.Builder
			sb.WriteString(generatorHeader)
			sb.WriteString("- alias: " + aliasPath + "\n")
			sb.WriteString("  id: automation." + automationID + "\n")
			sb.WriteString("  initial_state: True\n")
			sb.WriteString("  trigger:\n")
			sb.WriteString("  - platform: state\n")
			sb.WriteString("    entity_id: " + leaderID + "\n")
			if dir == "turn_on" {
				sb.WriteString("    to: 'on'\n")
			} else {
				sb.WriteString("    to: 'off'\n")
			}
			sb.WriteString("  action:\n")
			sb.WriteString("  - service: " + service + "\n")
			sb.WriteString("    entity_id: " + followerID + "\n")

			if err := writeAutomationFile(outputDir, sphere, automationID, sb.String()); err != nil {
				return err
			}
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

		for _, dir := range []string{"switched_on", "switched_off"} {
			aliasPath := mainPath + "/" + dir
			automationID := strings.ReplaceAll(aliasPath, "/", "_")
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
	// Collect all spaces that have a switch.physical/.../heating entity.
	seen := map[string]bool{}
	var contextPaths []string
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain != "switch" || rec.Identity.Sphere != "physical" {
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
		heatingSwitch := "switch." + physUnder + "_heating"

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
	domain     string
	sphere     string // empty = match any sphere; otherwise only entities in this sphere match
	pathSuffix string // empty = match all; otherwise path must end with this segment sequence
}

type TListCleanOp struct {
	kind  string // "prefix" or "postfix"
	value string
}

type TListDeclaration struct {
	title    string
	patterns []TListPattern
	cleanOps []TListCleanOp
}

// generateListFiles parses Lists.def content and writes a list.* file for each declaration.
func generateListFiles(outputDir string, content []byte, admin *TAdministrationState) error {
	declarations := parseListDeclarations(string(content))
	for _, decl := range declarations {
		entries := resolveListEntries(decl, admin)
		if len(entries) == 0 {
			continue
		}
		yaml := buildListFileYAML(decl.title, entries)
		fileName := "list." + listTitleToFileName(decl.title)
		if err := os.WriteFile(filepath.Join(outputDir, fileName), []byte(yaml), 0644); err != nil {
			return fmt.Errorf("cannot write %s: %w", fileName, err)
		}
	}
	return nil
}

// parseListDeclarations extracts all list declarations from Lists.def source.
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
		patterns := parseListPatterns(patternsPart)

		var cleanOps []TListCleanOp
		if withIdx >= 0 {
			// Read clean_prefix / clean_postfix lines until "end;".
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
					}
				}
				i++
			}
		}

		declarations = append(declarations, TListDeclaration{title, patterns, cleanOps})
		i++
	}
	return declarations
}

// parseListPatterns parses one or more space-separated entity patterns.
// Pattern forms:
//   "domain.*"              — match all entities with domain, any sphere
//   "domain.*/suffix"       — match by path tail, any sphere
//   "domain.sphere/*/suffix" — match by path tail, specific sphere only
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
		patterns = append(patterns, TListPattern{domain: domain, sphere: sphere, pathSuffix: pathSuffix})
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
			// Node entities with a light-domain representative are renamed _light_node by the generator.
			if strings.HasSuffix(entityID, "_node") {
				if repID := resolveNodeRepresentative(rec.Name, rec, admin); strings.HasPrefix(repID, "light.") {
					entityID = strings.TrimSuffix(entityID, "_node") + "_light_node"
				}
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

func listTitleToFileName(title string) string {
	return strings.ToLower(strings.ReplaceAll(title, " ", "_"))
}
