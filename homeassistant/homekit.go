/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: HomeKit
 *
 * PROJECT.md 62 (2026-11-21, Erik): what the main instance's HomeKit bridges expose to Apple Home
 * (and so Siri) comes from External.def's "homekit bridge" statements (parser_external.go). The
 * generator writes, per bridge, its name, port and entities into homekit.json next to the
 * instance's YAML; the deploy sets each bridge's filter to exactly those entities
 * (homekit_deploy.go).
 *
 * The bridges themselves are made in Home Assistant's UI, and stay that way: Home Assistant only
 * lets YAML take over a bridge that YAML made (a YAML bridge with a UI bridge's name and port is
 * refused, "port_name_in_use"), and a new bridge would mean pairing every iPhone again. Names and
 * rooms in Apple Home are set in the Home app; HomeKit has no rooms of Home Assistant's to take.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Vienna, Austria, in collaboration with Claude.ai
 *
 * Version of: 21.11.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// homekitManifestFile holds what each HomeKit bridge exposes, in the instance's folder.
const homekitManifestFile = "homekit.json"

// THomekitManifestBridge is one bridge in homekit.json: how Home Assistant knows it, and the
// entity_ids it exposes.
type THomekitManifestBridge struct {
	Name     string   `json:"name"`
	Port     int      `json:"port"`
	Entities []string `json:"entities"`
}

// generateHomekitManifest writes bridges as homekit.json, when any bridge exposes something; the
// warnings name what is left out. A bridge exposing nothing is left out: an empty filter makes
// Home Assistant expose every entity it can.
func generateHomekitManifest(outputDir string, bridges []THomekitBridge, admin *TAdministrationState) ([]string, error) {
	var warnings []string
	var manifest []THomekitManifestBridge
	for _, bridge := range bridges {
		entry := THomekitManifestBridge{Name: bridge.Name, Port: bridge.Port, Entities: []string{}}
		for _, exposure := range bridge.Exposed {
			entityID := toHomeAssistantEntityID(exposure.Name)
			if _, _, found := entityRecordByEntityID(admin, entityID); entityID == "" || !found {
				warnings = append(warnings, fmt.Sprintf("homekit bridge %q: no entity %s -- not exposed", bridge.Name, exposure.Name))
				continue
			}
			if !containsString(entry.Entities, entityID) {
				entry.Entities = append(entry.Entities, entityID)
			}
		}
		if len(entry.Entities) == 0 {
			warnings = append(warnings, fmt.Sprintf("homekit bridge %q exposes nothing -- left out (an empty filter would expose everything)", bridge.Name))
			continue
		}
		manifest = append(manifest, entry)
	}
	if len(manifest) == 0 {
		return warnings, nil
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return warnings, err
	}
	return warnings, os.WriteFile(filepath.Join(outputDir, homekitManifestFile), append(data, '\n'), 0o644)
}

// readHomekitBridges is External.def's HomeKit bridges, from the external layer's content.
func readHomekitBridges(content string) []THomekitBridge {
	_, bridges, _ := parseExternalLayer(content)
	return bridges
}
