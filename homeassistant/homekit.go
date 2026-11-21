/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: HomeKit
 *
 * PROJECT.md 62 (2026-11-21, Erik): the main instance's HomeKit bridges, and what each exposes to
 * Apple Home (and so Siri), come from External.def's "homekit bridge" statements
 * (parser_external.go). The generator writes them as Home Assistant's YAML "homekit:"
 * configuration, integrations/homekit.yaml, included from configuration.yaml.
 *
 * The bridges are Home Assistant's YAML-made ones: YAML can't take over a bridge made in the UI (a
 * YAML bridge with a UI bridge's name or port is refused, "port_name_in_use"), so a UI bridge goes
 * before its YAML successor comes, and the iPhones pair with the new one. A bridge is known by its
 * name and port; changing either makes a new bridge, to pair again.
 *
 * Each exposed entity becomes one "include_entities" line, with its spoken name, when given, as its
 * "entity_config" name. Rooms are set in the Home app; HomeKit takes none from Home Assistant. A
 * bridge that would expose nothing is left out: an empty filter makes Home Assistant expose every
 * entity it can.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Vienna, Austria, in collaboration with Claude.ai
 *
 * Version of: 21.11.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// homekitFile is the generated "homekit:" configuration, in the instance's integrations folder.
const homekitFile = "homekit.yaml"

// tHomekitYAMLBridge is one bridge in Home Assistant's "homekit:" YAML.
type tHomekitYAMLBridge struct {
	Name         string                       `yaml:"name"`
	Port         int                          `yaml:"port"`
	Filter       tHomekitYAMLFilter           `yaml:"filter"`
	EntityConfig map[string]map[string]string `yaml:"entity_config,omitempty"`
}

// tHomekitYAMLFilter is a bridge's filter: only the listed entities.
type tHomekitYAMLFilter struct {
	IncludeEntities []string `yaml:"include_entities"`
}

// generateHomekitConfiguration writes bridges as integrations/homekit.yaml, and configuration.yaml
// with its "homekit:" line, when any bridge exposes something; the warnings name what is left out.
func generateHomekitConfiguration(outputDir string, bridges []THomekitBridge, admin *TAdministrationState) ([]string, error) {
	var warnings []string
	var yamlBridges []tHomekitYAMLBridge
	for _, bridge := range bridges {
		yamlBridge := tHomekitYAMLBridge{Name: bridge.Name, Port: bridge.Port}
		for _, exposure := range bridge.Exposed {
			entityID := toHomeAssistantEntityID(exposure.Name)
			if _, _, found := entityRecordByEntityID(admin, entityID); entityID == "" || !found {
				warnings = append(warnings, fmt.Sprintf("homekit bridge %q: no entity %s -- not exposed", bridge.Name, exposure.Name))
				continue
			}
			if containsString(yamlBridge.Filter.IncludeEntities, entityID) {
				continue
			}
			yamlBridge.Filter.IncludeEntities = append(yamlBridge.Filter.IncludeEntities, entityID)
			if exposure.Spoken != "" {
				if yamlBridge.EntityConfig == nil {
					yamlBridge.EntityConfig = map[string]map[string]string{}
				}
				yamlBridge.EntityConfig[entityID] = map[string]string{"name": exposure.Spoken}
			}
		}
		if len(yamlBridge.Filter.IncludeEntities) == 0 {
			warnings = append(warnings, fmt.Sprintf("homekit bridge %q exposes nothing -- left out (an empty filter would expose everything)", bridge.Name))
			continue
		}
		yamlBridges = append(yamlBridges, yamlBridge)
	}
	if len(yamlBridges) == 0 {
		return warnings, nil
	}
	data, err := yaml.Marshal(yamlBridges)
	if err != nil {
		return warnings, err
	}
	if err := writeYAMLFile(filepath.Join(outputDir, "integrations", homekitFile), generatorHeader+string(data)); err != nil {
		return warnings, err
	}
	return warnings, writeConfigurationFile(outputDir, "homekit: !include integrations/"+homekitFile+"\n")
}

// readHomekitBridges is External.def's HomeKit bridges, from the external layer's content.
func readHomekitBridges(content string) []THomekitBridge {
	_, bridges, _ := parseExternalLayer(content)
	return bridges
}
