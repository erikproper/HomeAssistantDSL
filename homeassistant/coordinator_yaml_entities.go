/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: CoordinatorYAMLEntities
 *
 * PROJECT.md 19h.2 (2026-10-27): the coordinator's own entities come from the generator's YAML
 * "mqtt:" config instead of its discovery -- entities/mqtt/<domain>/<entity_id>.yaml, next to the
 * devices' ones. A fixed set, all ours (Architecture.md §6.16, controlled):
 *
 *   - a reload and a restart button per Home Assistant instance, for this installation, and for
 *     "all" (house_event_bus_coordinator/meta_reload_restart.go handles their command topics),
 *   - the "Discover entity" text input (discover_entity_input.go),
 *   - the "missing declared entities" indicator (missing_declared_entities.go).
 *
 * Each is exactly what the coordinator published: the same unique_id, entity_id, name, topics and
 * payloads (its object_id becomes the default_entity_id), so the registry entry stays. The
 * registry manifest has them through recordInfrastructuralControls.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 27.10.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// generateYAMLCoordinatorEntities writes the coordinator's own YAML "mqtt:" entities (see this
// file's header comment) for instances and installation, and reports what it wrote.
func generateYAMLCoordinatorEntities(instances []string, ctx TPhysicalGenerationContext) []string {
	if ctx.HAOutputDir == "" {
		return nil // no Home Assistant output directory (a test): nothing to write, never relative to the working directory
	}
	var reports []string
	written := 0
	var entities []THostYAMLEntity
	for _, entity := range coordinatorYAMLEntityBodies(instances, ctx.Installation) {
		entities = append(entities, discoveredYAMLEntity(entity.Domain, entity.EntityID, entity.Body))
	}
	for _, entity := range entities {
		data, err := yaml.Marshal([]map[string]interface{}{{entity.Domain: entity.Body}})
		if err != nil {
			reports = append(reports, fmt.Sprintf("%s: %v", entity.EntityID, err))
			continue
		}
		path := filepath.Join(ctx.HAOutputDir, "entities", "mqtt", entity.Domain, entity.EntityID+".yaml")
		if err := writeYAMLFile(path, generatorHeader+string(data)); err != nil {
			reports = append(reports, fmt.Sprintf("%s: %v", entity.EntityID, err))
			continue
		}
		written++
	}
	return append(reports, fmt.Sprintf("coordinator: %d entities via YAML", written)+droppedFieldsNote(entities))
}

// coordinatorYAMLEntityBodies returns the coordinator's own entities, as it published them.
func coordinatorYAMLEntityBodies(instances []string, installation string) []THostYAMLEntity {
	segment := func(s string) string { return strings.ReplaceAll(s, ".", "_") } // the coordinator's sanitizeTopicSegment
	entityPart := func(s string) string { return strings.NewReplacer(".", "_", "-", "_").Replace(s) }
	var entities []THostYAMLEntity
	if installation != "" {
		for _, action := range []string{"reload", "restart"} {
			icon := "mdi:reload"
			if action == "restart" {
				icon = "mdi:restart"
			}
			for _, target := range append(append([]string{}, instances...), installation, "all") {
				entityID := "button.infrastructural_" + action + "_" + entityPart(target)
				entities = append(entities, THostYAMLEntity{Domain: "button", EntityID: entityID, Body: map[string]interface{}{
					"unique_id":         "meta_" + action + "_" + segment(target),
					"default_entity_id": entityID,
					"name":              "infrastructural/" + action + "/" + target,
					"icon":              icon,
					"command_topic":     "meta/" + action + "/" + target + "/request",
					"payload_press":     "PRESS",
					"entity_category":   "config",
				}})
			}
		}
	}
	entities = append(entities, THostYAMLEntity{Domain: "text", EntityID: "text.infrastructural_discover_entity", Body: map[string]interface{}{
		"unique_id":         "meta_discover_entity",
		"default_entity_id": "text.infrastructural_discover_entity",
		"name":              "infrastructural/discover_entity",
		"icon":              "mdi:text-search",
		"command_topic":     "meta/discover_entity/set",
		"optimistic":        true,
		"entity_category":   "config",
	}})
	entities = append(entities, THostYAMLEntity{Domain: "binary_sensor", EntityID: "binary_sensor.missing_declared_entities", Body: map[string]interface{}{
		"unique_id":             "missing_declared_entities",
		"default_entity_id":     "binary_sensor.missing_declared_entities",
		"name":                  "Missing declared entities",
		"device_class":          "problem",
		"icon":                  "mdi:link-off",
		"state_topic":           "coordinator/missing_declared_entities/state",
		"payload_on":            "true",
		"payload_off":           "false",
		"json_attributes_topic": "coordinator/missing_declared_entities/attributes",
		"entity_category":       "diagnostic",
	}})
	return entities
}
