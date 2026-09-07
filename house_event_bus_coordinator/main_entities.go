/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: MainEntities
 *
 * Loads coordinator/main_entities.yaml (generator output, homeassistant/main_entities.go's
 * generateMainEntitiesFile) -- the flat list of every "bare" Spaces.def entity declaration (kind-5,
 * PROJECT.md item 1, 2026-09-07), assumed to already exist as a real, native entity on this house's
 * own main Home Assistant instance. Fed into entity_existence.go's TEntityExistenceTracker via
 * SeedMainEntities, reusing that tracker's own instance-generic active-inquiry/suggestion engine
 * (already used for kind-3's bridged instances) for instance "main" specifically.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 07.09.2026
 *
 */

package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// TMainEntitiesFile is the top-level shape of a generated coordinator/main_entities.yaml file.
type TMainEntitiesFile struct {
	Entities []string `yaml:"entities"`
}

// loadMainEntitiesFile reads and parses a generator-produced coordinator/main_entities.yaml file,
// if one exists -- a missing file returns a zero value, not an error, mirroring every other
// optional coordinator input file's own convention.
func loadMainEntitiesFile(path string) (TMainEntitiesFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return TMainEntitiesFile{}, nil
		}
		return TMainEntitiesFile{}, fmt.Errorf("cannot read %s: %w", path, err)
	}
	var file TMainEntitiesFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return TMainEntitiesFile{}, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	return file, nil
}
