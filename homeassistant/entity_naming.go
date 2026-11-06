/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: EntityNaming
 *
 * PROJECT.md 32 (2026-11-05, Erik; Architecture.md §3 "material"): Home Assistant 2026.11 composes
 * an entity's name from its device's name and its own, so a full conceptual path as the entity
 * name shows the path twice. Every YAML "mqtt:" entity is therefore named, after all writers are
 * done, from its device's node position:
 *
 *   - the device is a material device, named "material/<path of its node, without the node>";
 *   - its own entities (infrastructural, or in the legacy physical sphere) are named relative to
 *     it ("battery_level", "node"), else by their domain or last segment;
 *   - every social (positioned) entity becomes the single entity of a sub-device of the material
 *     device (via_device), named after its conceptual position ("social/apartment/hallway/humidity"),
 *     in the area of its space; the entity has no name of its own (null), so Home Assistant shows
 *     just the sub-device's name (tested on 2026.11.0: a short name under a device name ending in
 *     the same word shows it twice) -- unless another social entity is at the same position: then
 *     each is named by its domain.
 *
 * Done as one pass over the written files, so no writer can be missed. An entity whose device has
 * no positioned node keeps the name its writer gave it, and is reported.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 05.11.2026
 *
 */

package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// materialSphere names the devices: the hardware, where it actually is. socialSphere's entities
// are the positioned capabilities, each a sub-device.
const (
	materialSphere = "material"
	socialSphere   = "social"
)

// entityDeviceIDs maps every entity a device provides to that device's id: the node and attributes
// of every kind, then -- overriding a logical device that absorbs them (e.g. Junglinster's
// fan.house_wc, which absorbs a relay's node) -- a discovery device's entities and its node: the
// hardware whose YAML they are.
func entityDeviceIDs(admin *TAdministrationState) map[string]string {
	entityDevice := map[string]string{}
	for _, deviceID := range sortedMapKeys(admin.DeviceConceptualLinks) {
		link := admin.DeviceConceptualLinks[deviceID]
		if link.NodeEntityID != "" {
			entityDevice[link.NodeEntityID] = deviceID
		}
		for _, label := range sortedMapKeys(link.AttributeEntityIDs) {
			if attribute := link.AttributeEntityIDs[label]; attribute.EntityID != "" {
				entityDevice[attribute.EntityID] = deviceID
			}
		}
	}
	for _, deviceID := range sortedMapKeys(admin.DeviceNodeEntityID) {
		entityDevice[admin.DeviceNodeEntityID[deviceID]] = deviceID
	}
	for entityID, link := range admin.DiscoveryEntityLinks {
		entityDevice[entityID] = link.GatewayDeviceID
	}
	for deviceID, entities := range admin.BatteryEmulationEntities {
		for _, entityID := range []string{entities.Level, entities.Alert, entities.Date, entities.Duration, entities.Reset} {
			if entityID != "" {
				entityDevice[entityID] = deviceID
			}
		}
	}
	for _, records := range admin.EntityRecordsBySpace {
		for _, rec := range records {
			if rec.AvailabilityOfEntityID != "" {
				entityDevice[toHomeAssistantEntityID(rec.Name)] = admin.DiscoveryEntityLinks[rec.AvailabilityOfEntityID].GatewayDeviceID
			}
		}
	}
	return entityDevice
}

// devicePosition returns the position of deviceID's node, without the node itself, and the node's
// entity_id: the node recorded for every kind, else a "hosts"/"import" device's own node entity,
// else a "hosts" device's position (a ping-only device, whose liveness check is its node; no
// entity_id then).
func devicePosition(admin *TAdministrationState, deviceID string) (TEntityIdentity, string, bool) {
	link := admin.DeviceConceptualLinks[deviceID]
	for _, entityID := range []string{admin.DeviceNodeEntityID[deviceID], link.NodeEntityID, link.AttributeEntityIDs["node"].EntityID} {
		if entityID == "" {
			continue
		}
		if identity, found := entityIdentityByID(admin, entityID); found && identity.Sphere != "" {
			return stripLastPathSegment(identity), entityID, true
		}
	}
	if link.HostIdentity.Sphere != "" {
		return link.HostIdentity, "", true
	}
	return TEntityIdentity{}, "", false
}

// nameEntity sets body's own name and device block (see the file comment); spaceArea gives every
// positioned entity's area, shared the positions more than one social entity is at. The material
// device is in its node's area, where the hardware is. It returns false when the entity's device
// has no positioned node, leaving body as it was.
func nameEntity(admin *TAdministrationState, body map[string]interface{}, entityID, deviceID string, spaceArea map[string]string, shared map[string]bool) bool {
	device, hasDevice := body["device"].(map[string]interface{})
	position, nodeEntityID, positioned := devicePosition(admin, deviceID)
	if !hasDevice || !positioned {
		return false
	}
	identity, found := entityIdentityByID(admin, entityID)
	if !found || identity.Sphere == "" {
		return false
	}
	device["name"] = materialSphere + "/" + position.Path
	if area := spaceArea[nodeEntityID]; area != "" {
		device["suggested_area"] = area
	}
	area := spaceArea[entityID]
	if identity.Sphere != socialSphere {
		// The device's own (infrastructural, or the legacy physical sphere's): named relative to
		// it; at the device's own position (e.g. "update.infrastructural:cover" on the device at
		// ".../cover") by its domain; positioned elsewhere by its last segment ("pressed").
		switch relative, under := strings.CutPrefix(identity.Path, position.Path+"/"); {
		case identity.Path == position.Path:
			body["name"] = identity.Domain
		case under:
			body["name"] = relative
		default:
			body["name"] = identity.Path[strings.LastIndex(identity.Path, "/")+1:]
		}
		return true
	}
	sub := map[string]interface{}{
		"identifiers": []string{fmt.Sprint(body["unique_id"])},
		"name":        friendlyEntityName(identity),
		"via_device":  firstIdentifier(device),
	}
	if area != "" {
		sub["suggested_area"] = area
	}
	body["device"] = sub
	body["name"] = nil
	// Two entities at one position (a light and a cover, from two devices) are two sub-devices of
	// the same name: their domain tells them apart ("social/house/wc light", Erik, 2026-11-05).
	if shared[friendlyEntityName(identity)] {
		body["name"] = identity.Domain
	}
	return true
}

// sharedPositions returns the positions more than one of entityIDs' social entities is at.
func sharedPositions(admin *TAdministrationState, entityIDs []string) map[string]bool {
	count := map[string]int{}
	for _, entityID := range entityIDs {
		if identity, found := entityIdentityByID(admin, entityID); found && identity.Sphere == socialSphere {
			count[friendlyEntityName(identity)]++
		}
	}
	shared := map[string]bool{}
	for position, n := range count {
		if n > 1 {
			shared[position] = true
		}
	}
	return shared
}

// nameYAMLMQTTEntities names every YAML "mqtt:" entity under haOutputDir (see the file comment)
// and reports what it did, and every entity it left as it was.
func nameYAMLMQTTEntities(haOutputDir string, admin *TAdministrationState) []string {
	if haOutputDir == "" {
		return nil
	}
	entityDevice := entityDeviceIDs(admin)
	spaceArea := map[string]string{}
	for spaceName, records := range admin.EntityRecordsBySpace {
		for _, rec := range records {
			spaceArea[toHomeAssistantEntityID(rec.Name)] = admin.SpaceAreas[spaceName]
		}
	}
	files, _ := filepath.Glob(filepath.Join(haOutputDir, "entities", "mqtt", "*", "*.yaml"))
	entityIDs := make([]string, 0, len(files))
	for _, file := range files {
		entityIDs = append(entityIDs, strings.TrimSuffix(filepath.Base(file), ".yaml"))
	}
	shared := sharedPositions(admin, entityIDs)
	var reports, unnamed []string
	relative, subDevices := 0, 0
	materials := map[string]bool{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			reports = append(reports, fmt.Sprintf("%s: %v", file, err))
			continue
		}
		var parsed []map[string]map[string]interface{}
		if err := yaml.Unmarshal(data, &parsed); err != nil || len(parsed) != 1 {
			continue // validateYAMLMQTTEntities reports it
		}
		entityID := strings.TrimSuffix(filepath.Base(file), ".yaml")
		for _, body := range parsed[0] {
			if _, hasDevice := body["device"]; !hasDevice {
				continue
			}
			deviceID := entityDevice[entityID]
			if !nameEntity(admin, body, entityID, deviceID, spaceArea, shared) {
				unnamed = append(unnamed, entityID)
				continue
			}
			if name, _ := body["device"].(map[string]interface{})["name"].(string); strings.HasPrefix(name, socialSphere+"/") {
				subDevices++
			} else {
				relative++
				materials[deviceID] = true
			}
		}
		out, err := yaml.Marshal(parsed)
		if err == nil {
			err = writeYAMLFile(file, generatorHeader+string(out))
		}
		if err != nil {
			reports = append(reports, fmt.Sprintf("%s: %v", file, err))
		}
	}
	reports = append(reports, fmt.Sprintf("naming: %d entities named relative to %d material devices, %d positioned entities as sub-devices", relative, len(materials), subDevices))
	if len(unnamed) > 0 {
		sort.Strings(unnamed)
		reports = append(reports, fmt.Sprintf("naming: %d entities kept their name, their device has no positioned node: %s", len(unnamed), strings.Join(unnamed, ", ")))
	}
	return reports
}

// dropFriendlyNames removes the friendly_name from every customization file under outputDir of an
// entity the generator writes itself -- Home Assistant plans to sunset the customized
// friendly_name, and the naming above no longer needs it (PROJECT.md 32, tested in both houses
// 2026-11-05) (entities/.../<entity_id>.yaml), and the files left with
// nothing else (an icon stays), and returns how many it dropped. A native integration's entity
// keeps its friendly_name: nothing else names it ours (found live 2026-11-05, Vienna's System
// Monitor sensors reading "processor_use").
func dropFriendlyNames(outputDir string) (int, error) {
	files, err := filepath.Glob(filepath.Join(outputDir, "customization", "*", "*", "*.yaml"))
	if err != nil {
		return 0, err
	}
	own := map[string]bool{}
	err = filepath.WalkDir(filepath.Join(outputDir, "entities"), func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.HasSuffix(path, ".yaml") {
			own[strings.TrimSuffix(entry.Name(), ".yaml")] = true
		}
		return err
	})
	if err != nil {
		return 0, err
	}
	dropped := 0
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return dropped, err
		}
		var entries map[string]map[string]interface{}
		if err := yaml.Unmarshal(data, &entries); err != nil {
			return dropped, fmt.Errorf("%s: %v", file, err)
		}
		left := map[string]map[string]interface{}{}
		for entityID, fields := range entries {
			if _, has := fields["friendly_name"]; has && own[entityID] {
				delete(fields, "friendly_name")
				dropped++
			}
			if len(fields) > 0 {
				left[entityID] = fields
			}
		}
		if len(left) == 0 {
			if err := os.Remove(file); err != nil {
				return dropped, err
			}
			continue
		}
		out, err := yaml.Marshal(left)
		if err == nil {
			err = writeYAMLFile(file, generatorHeader+string(out))
		}
		if err != nil {
			return dropped, err
		}
	}
	return dropped, nil
}
