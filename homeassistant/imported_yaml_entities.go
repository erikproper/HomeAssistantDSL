/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ImportedYAMLEntities
 *
 * PROJECT.md 19h.4 (2026-10-27): every imported device (an "integration import" device: another
 * installation's export, over the cloud broker) gets its entities from the generator's own YAML
 * "mqtt:" config instead of the coordinator's discovery -- entities/mqtt/<domain>/<entity_id>.yaml.
 * The coordinator keeps relaying the state onto imports/<entity>/state. What the exporting house
 * says about an entity (device metadata, via_device, typing, payloads) belongs to that house, so
 * it comes from the coordinator's import_catalogue/state, cached in
 * Definitions/.cache/import_catalogue.json, never from the importer's defs. A "derived" capability
 * needs nothing from there.
 *
 * Each entity is built by the coordinator's own builders (hamqtt.ImportedEntityConfig,
 * hamqtt.DerivedImportedEntityConfig), from the same data imported.yaml carries: the same
 * unique_id, entity_id, topics and availability, so the registry entry stays. The name is the
 * entity's full conceptual name (PROJECT.md 17h -- the relay used "<device>/<capability>"). An
 * entity whose catalogue entry is missing (the exporting house never published it, nothing cached)
 * is an ERROR and stays out -- never a fallback to discovery. The coordinator publishes no local
 * discovery (PROJECT.md 19).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 27.10.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"homeassistant/hamqtt"

	"gopkg.in/yaml.v3"
)

func importCatalogueCachePath(definitionDir string) string {
	return filepath.Join(definitionDir, ".cache", "import_catalogue.json")
}

// fetchImportCatalogue returns the imported entities' catalogue entries (local entity_id ->
// what the exporting house says), from a broker when one answers (refreshing the cache), else
// from the cache.
func fetchImportCatalogue(definitionDir string, ctx TPhysicalGenerationContext) (map[string]hamqtt.ImportedRemote, error) {
	var catalogue map[string]hamqtt.ImportedRemote
	fetched := false
	if cloudSecrets, ok := coordinatorOnlyBrokerSecrets(ctx); ok {
		fetched = fetchRetainedJSON(cloudSecrets, ctx.Installation+"/import_catalogue/state", "homeassistant-generator-import-catalogue", &catalogue) == nil
	}
	if !fetched {
		fetched = fetchRetainedJSON(ctx.MQTTSecrets, "import_catalogue/state", "homeassistant-generator-import-catalogue", &catalogue) == nil
	}
	cachePath := importCatalogueCachePath(definitionDir)
	if fetched {
		if data, err := json.Marshal(catalogue); err == nil {
			_ = os.MkdirAll(filepath.Dir(cachePath), 0o755)
			_ = os.WriteFile(cachePath, data, 0o644)
		}
		return catalogue, nil
	}
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, fmt.Errorf("no broker answered and no cache at %s yet", cachePath)
	}
	return catalogue, json.Unmarshal(data, &catalogue)
}

// generateYAMLImportedEntities writes the YAML "mqtt:" entities of every imported device (see this
// file's header comment), and reports what it wrote and what it couldn't.
func generateYAMLImportedEntities(definitionDir string, devices []TImportedDevice, ctx TPhysicalGenerationContext) []string {
	if ctx.HAOutputDir == "" {
		return nil // no Home Assistant output directory (a test): nothing to write, never relative to the working directory
	}
	if len(devices) == 0 {
		return nil
	}
	catalogue, err := fetchImportCatalogue(definitionDir, ctx)
	if err != nil {
		catalogue = map[string]hamqtt.ImportedRemote{}
		fmt.Printf("[physical] imported entities: %v\n", err)
	}
	var reports []string
	seen := map[string]bool{}
	remoteIDs := map[string]string{}      // "<device-id>!<capability>" -> the exporter's export_id
	renamedRemotes := map[string]string{} // device id -> the exporter's current device id
	for _, device := range devices {
		if seen[device.DeviceID] {
			continue
		}
		seen[device.DeviceID] = true
		// PROJECT.md 22: the exporter's persistent identity of each capability, from the catalogue,
		// into the declaration -- and a rename on the exporting side noticed.
		if link, ok := ctx.Admin.DeviceConceptualLinks[device.DeviceID]; ok {
			for name, capability := range device.Capabilities {
				remote, found := catalogue[link.AttributeEntityIDs[name].EntityID]
				if !found {
					continue
				}
				if capability.RemoteID == "" && remote.ExportID != "" {
					remoteIDs[device.DeviceID+"!"+name] = remote.ExportID
				}
				if remote.RemoteDeviceID != "" && remote.RemoteDeviceID != device.RemoteDeviceID {
					renamedRemotes[device.DeviceID] = remote.RemoteDeviceID
				}
			}
		}
		entities, missing := importedYAMLEntityBodies(device, ctx.Admin, catalogue)
		if len(entities) == 0 && len(missing) == 0 {
			continue
		}
		written := 0
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
		report := fmt.Sprintf("%s: %d entities via YAML", device.DeviceID, written) + droppedFieldsNote(entities)
		if len(missing) > 0 {
			report += "; ERROR, " + device.RemoteInstallation + " doesn't export these (no catalogue entry, nothing cached -- check its \"export\" and this device's \"from " + device.RemoteInstallation + " " + device.RemoteDeviceID + "\"): " + strings.Join(missing, ", ")
		}
		reports = append(reports, report)
	}
	if len(remoteIDs) > 0 || len(renamedRemotes) > 0 {
		reports = append(reports, writeImportRemoteIdentities(filepath.Join(definitionDir, "Physical.def"), remoteIDs, renamedRemotes)...)
	}
	return reports
}

// importDeviceHeaderPattern matches an import's device line, long ("device <id> from <installation>
// <remote-device-id> with:") or short ("device <id> from <installation> with:", the remote device
// id being the same as the local one).
var importDeviceHeaderPattern = regexp.MustCompile(`^(\s*device\s+(\S+)\s+from\s+\S+)(?:\s+(\S+))?(\s+with:\s*)$`)

// writeImportRemoteIdentities writes into path (Physical.def, keeping its layout) each imported
// capability's "from <export-id>" (remoteIDs, "<device-id>!<capability>" -> export_id), right
// after the capability's name, and each renamed remote device's new id into its "device <id> from
// <installation> <remote-device-id> with:" line (PROJECT.md 22). The import keeps working through
// the rename either way -- the coordinator matches on the export_id -- this keeps the declaration
// telling the truth. A capability line not found exactly once in its device is reported.
func writeImportRemoteIdentities(path string, remoteIDs, renamedRemotes map[string]string) []string {
	lines, err := readDefStatementLines(path) // one line per statement (PROJECT.md 22c)
	if err != nil {
		return []string{fmt.Sprintf("could not read %s to write import identities: %v", path, err)}
	}
	var reports []string
	pending := map[string]bool{}
	for key := range remoteIDs {
		pending[key] = true
	}
	device := ""
	for i, line := range lines {
		if m := importDeviceHeaderPattern.FindStringSubmatch(line); m != nil {
			device = m[2]
			oldID := m[3]
			if oldID == "" {
				oldID = device
			}
			if newID, renamed := renamedRemotes[device]; renamed && newID != oldID {
				lines[i] = m[1] + " " + newID + m[4]
				reports = append(reports, fmt.Sprintf("%s: the exporting house renamed %s to %s -- the declaration follows", device, oldID, newID))
			}
			continue
		}
		if strings.TrimSpace(line) == "end;" {
			device = ""
			continue
		}
		if device == "" {
			continue
		}
		m := importCapabilityLinePattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key := device + "!" + m[2]
		if id, ok := remoteIDs[key]; ok && pending[key] {
			lines[i] = m[1] + " from " + id + m[3]
			delete(pending, key)
			reports = append(reports, fmt.Sprintf("%s: capability %s from %s (the exporter's persistent identity, written into Physical.def)", device, m[2], id))
		}
	}
	for key := range pending {
		reports = append(reports, fmt.Sprintf("import identity for %s not written: its capability line wasn't found", key))
	}
	sort.Strings(reports)
	if err := writeDefFormatted(path, lines); err != nil {
		return append(reports, fmt.Sprintf("could not write %s: %v", path, err))
	}
	return reports
}

// importCapabilityLinePattern splits an import capability line without "from" into its head
// ("capability <domain>.<label>"), its label, and the rest (" as <id>;" or ";").
var importCapabilityLinePattern = regexp.MustCompile(`^(\s*capability\s+[A-Za-z_][A-Za-z0-9_]*\.([A-Za-z_][A-Za-z0-9_/]*))((?:\s+as\s+\S+)?\s*;.*)$`)

// importedYAMLEntityBodies returns device's YAML entities, built from exactly what
// generateImportedDeviceFile writes for the coordinator, and the entities missing a catalogue
// entry.
func importedYAMLEntityBodies(device TImportedDevice, admin *TAdministrationState, catalogue map[string]hamqtt.ImportedRemote) (entities []THostYAMLEntity, missing []string) {
	link, hasLink := admin.DeviceConceptualLinks[device.DeviceID]
	if !hasLink {
		return nil, nil
	}
	deviceName := link.DisplayName
	if deviceName == "" {
		deviceName = device.DeviceID
	}
	area := link.ConstantAttributes["suggested_area"].Value
	localEntity := func(capability string) string {
		return link.AttributeEntityIDs[capability].EntityID
	}

	// The device's own node, ANDed into every other capability's availability (the relay's
	// importedAvailabilityTopics), and every "dependency on" node topic.
	nodeTopic := ""
	if _, declared := device.Capabilities["node"]; declared && localEntity("node") != "" {
		nodeTopic = hamqtt.ImportedStateTopic(localEntity("node"))
	}
	availabilityFor := func(capability string) []string {
		var topics []string
		if capability != "node" && nodeTopic != "" {
			topics = append(topics, nodeTopic)
		}
		return append(topics, device.DependsOnAvailabilityTopics...)
	}

	names := make([]string, 0, len(device.Capabilities))
	for name := range device.Capabilities {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entityID := localEntity(name)
		if entityID == "" {
			continue
		}
		capability := device.Capabilities[name]
		conceptualName := ""
		if identity, found := entityIdentityByID(admin, entityID); found && identity.Sphere != "" {
			conceptualName = friendlyEntityName(identity)
		}
		domain := entityID[:strings.Index(entityID, ".")]
		var body map[string]interface{}
		if capability.DerivedFromCapability != "" {
			baseTopic, template, ok := resolveDerivedImport(device, link, name, map[string]bool{})
			if !ok {
				missing = append(missing, entityID+" (derived: dangling reference or cycle)")
				continue
			}
			body = hamqtt.DerivedImportedEntityConfig(hamqtt.DerivedImportedEntity{
				UniqueID: hamqtt.ImportedUniqueID(entityID, capability.AssignedID), LocalEntity: entityID,
				LocalDeviceID: device.DeviceID, DeviceName: deviceName, Name: conceptualName, Capability: name,
				SuggestedArea: area, BaseTopic: baseTopic, Template: template, NodeAvailabilityTopics: availabilityFor(name),
			})
		} else {
			remote, found := catalogue[entityID]
			if !found {
				missing = append(missing, entityID)
				continue
			}
			body = hamqtt.ImportedEntityConfig(hamqtt.ImportedEntity{
				UniqueID: hamqtt.ImportedUniqueID(entityID, capability.AssignedID), LocalEntity: entityID,
				LocalDeviceID: device.DeviceID, DeviceName: deviceName, Name: conceptualName, Capability: name,
				SuggestedArea: area, Remote: remote, NodeAvailabilityTopics: availabilityFor(name),
			})
		}
		entities = append(entities, discoveredYAMLEntity(domain, entityID, body))
	}
	return entities, missing
}

// resolveDerivedImport resolves a "derived" imported capability to its base capability's relayed
// state topic and the composed template ("value" standing for that state) -- the coordinator's
// resolveImportedDerivedCapability (discoveryimport.go), over the generator's own data.
func resolveDerivedImport(device TImportedDevice, link TDeviceConceptualLink, label string, visited map[string]bool) (topic, template string, ok bool) {
	if visited[label] {
		return "", "", false
	}
	visited[label] = true
	capability, known := device.Capabilities[label]
	if !known {
		return "", "", false
	}
	if capability.DerivedFromCapability == "" {
		entityID := link.AttributeEntityIDs[label].EntityID
		if entityID == "" {
			return "", "", false
		}
		return hamqtt.ImportedStateTopic(entityID), "value", true
	}
	baseTopic, baseTemplate, resolved := resolveDerivedImport(device, link, capability.DerivedFromCapability, visited)
	if !resolved {
		return "", "", false
	}
	return baseTopic, strings.ReplaceAll(capability.DerivedViaTemplate, "$", "("+baseTemplate+")"), true
}
