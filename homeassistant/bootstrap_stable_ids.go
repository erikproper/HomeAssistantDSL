/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: BootstrapStableIDs
 *
 * CLI entry point for "-bootstrap_stable_ids <mapping.json>" (PROJECT.md item 7's one-time full
 * bootstrap, memory/project_stable_entity_identity_design.md): assigns a frozen "as <id>;" to
 * every currently-live discovery-kind capability that doesn't have one yet, using its REAL,
 * currently-assigned HA registry unique_id, never a freshly-minted value. Run ONCE per house
 * before this project's first "as <id>"-aware ./generate, never again afterwards (ongoing
 * assignment for brand-new capabilities is autoAssignDiscoveryStableIDs' own job,
 * physical_auto_assign_stable_ids.go, wired into the normal generation pipeline already).
 *
 * mapping.json is a flat {"<entity_id>": "<unique_id>", ...} object -- every mqtt-platform
 * entity from the live HA registry pull (core.entity_registry), no pre-filtering needed; only
 * entries that actually match a live discovery capability are ever consulted.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 05.10.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func runBootstrapStableIDs(cwd, defPath, mappingPath string) error {
	fullDefPath := filepath.Join(cwd, defPath)
	definitionDir := filepath.Dir(fullDefPath)
	sharedDefinitionDir := filepath.Clean(filepath.Join(definitionDir, "..", "..", "Shared", "Definitions"))
	label := filepath.Base(cwd)

	mappingBytes, err := os.ReadFile(mappingPath)
	if err != nil {
		return fmt.Errorf("reading stable-id mapping file %s: %w", mappingPath, err)
	}
	var registryUniqueIDs map[string]string
	if err := json.Unmarshal(mappingBytes, &registryUniqueIDs); err != nil {
		return fmt.Errorf("parsing stable-id mapping file %s: %w", mappingPath, err)
	}

	admin, err := parseAdministrationFromPaths(definitionDir, sharedDefinitionDir, label, false)
	if err != nil {
		return err
	}

	discoveryGatewaysByID, warnings := collectDiscoveryGatewaysByID(definitionDir)
	for _, w := range warnings {
		fmt.Printf("[physical] %s\n", w)
	}

	for _, w := range bootstrapDiscoveryStableIDs(definitionDir, discoveryGatewaysByID, admin, registryUniqueIDs) {
		fmt.Printf("[physical] %s\n", w)
	}
	return nil
}
