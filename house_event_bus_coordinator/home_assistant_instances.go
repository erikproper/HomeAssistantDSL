/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: HomeAssistantInstances
 *
 * Loads coordinator/home_assistant_instances.yaml (generator output,
 * homeassistant/Physical_Generator.go's generateHomeAssistantInstancesFile) -- every declared
 * "home_assistant <qualifier>: <name>;" instance, independent of whether any bridge device has
 * been declared for it yet. Used by entity_existence.go to know which instances to run the
 * paced entity-existence inquiry loop against.
 *
 * This file used to also carry the manifest-relay mechanism (subscribeEntityCatalogue, relaying
 * each instance's "report entities detailed" bootstrap automation output onto
 * meta/manifests/<installation>/<instance>/state) -- removed entirely 2026-08-28, superseded by
 * entity_existence.go's own discovery (DiscoverSiblings, fed by device_entities() in each
 * inquiry reply) rather than a separate full-instance-scan automation. See memory:
 * project_entity_existence_inquiry_design.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 28.08.2026
 *
 */

package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// THomeAssistantInstancesFile is the top-level shape of a generated
// coordinator/home_assistant_instances.yaml file -- every declared "home_assistant <qualifier>:
// <name>;" instance, independent of whether any bridge device has been declared for it yet (an
// instance being onboarded, like protocols-server-2 for a first Netatmo device, may have none).
type THomeAssistantInstancesFile struct {
	Instances []string `yaml:"instances"`
}

// loadHomeAssistantInstancesFile reads and parses a generator-produced
// coordinator/home_assistant_instances.yaml file, if one exists -- a missing file returns a zero
// value, not an error, mirroring every other optional coordinator input file's own convention.
func loadHomeAssistantInstancesFile(path string) (THomeAssistantInstancesFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return THomeAssistantInstancesFile{}, nil
		}
		return THomeAssistantInstancesFile{}, fmt.Errorf("cannot read %s: %w", path, err)
	}
	var file THomeAssistantInstancesFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return THomeAssistantInstancesFile{}, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	return file, nil
}
