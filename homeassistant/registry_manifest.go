/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: RegistryManifest
 *
 * PROJECT.md item 48's deploy half (2026-10-21): every generate writes registry_manifest.json next
 * to the instance's own YAML -- per generated entity, the unique_id Home Assistant registers it
 * under and the entity_id it should have. The deploy's registry sync (tools/ha_registry_sync.py)
 * compares it with HA's own entity registry: an entry whose entity_id differs is renamed (keeping
 * its registry entry and history), and an entry this generator created before but no longer
 * generates is removed, so renames and removals never leave orphans behind.
 *
 * Parts: "yaml" -- every entity whose YAML this generator writes (template, group, ...),
 * captured as each one's unique_id is written (generatedUniqueID); "mqtt" -- the coordinator-relayed
 * discovery entities with a frozen stable_id (PROJECT.md item 7); "imported" -- the entities of
 * devices imported from another installation, whose coordinator-published unique_id is still
 * name-based ("imported_<entity_id>"), listed so a later deploy can remove one that is no longer
 * generated and that HA itself didn't clean up (it doesn't for a disabled entity); "native" -- the
 * entities a native HA integration provides (PROJECT.md 17j), keyed by their source in the defs,
 * renamed when their expected entity_id changes between two deploys, never removed.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 21.10.2026
 *
 */

package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// TRegistryManifestEntry is one generated entity: the unique_id HA registers it under, and the
// entity_id it should have.
type TRegistryManifestEntry struct {
	UniqueID string `json:"unique_id"`
	EntityID string `json:"entity_id"`
}

// TRegistryManifestNative is one entity a native HA integration provides (PROJECT.md 17j): its
// source in the defs ("<device-id>!<capability>") and the entity_id the generator expects. The
// registry repair renames it when that expected entity_id changes between two deploys -- the
// source is what identifies it across the rename, since the generator doesn't know the unique_id
// the integration gave it. It is never removed: it belongs to its integration.
type TRegistryManifestNative struct {
	Source   string `json:"source"`
	EntityID string `json:"entity_id"`
}

// TRegistryManifestOther is one generated automation, script or input_* helper: the platform and
// the unique_id HA registers it under -- listed only so a removed one can be cleaned up.
type TRegistryManifestOther struct {
	Platform string `json:"platform"`
	UniqueID string `json:"unique_id"`
}

// TRegistryManifest is registry_manifest.json's shape.
type TRegistryManifest struct {
	YAML     []TRegistryManifestEntry  `json:"yaml"`
	Native   []TRegistryManifestNative `json:"native"`
	MQTT     []TRegistryManifestEntry  `json:"mqtt"`
	Imported []TRegistryManifestEntry  `json:"imported"`
	Bridged  []TRegistryManifestEntry  `json:"hassbridge"`
	Other    []TRegistryManifestOther  `json:"other"`
	// Areas maps every listed or positioned entity_id to its area (PROJECT.md item 17e, 2026-10-26): its device's
	// area, else the area of the space it is positioned in, else defaultArea.
	Areas map[string]string `json:"areas"`
}

// defaultArea is the area of everything not positioned inside an "as area" space.
const defaultArea = "environment"

// generatedUniqueIDs collects, during one house's generation, every entity_id -> unique_id pair
// generatedUniqueID handed out -- i.e. every entity whose YAML was written.
var generatedUniqueIDs map[string]string

// coordinatorPublishedEntities collects the entities the coordinator publishes under a unique_id
// fixed by device id and capability (hosts and commandline devices: "<device-id>_node",
// "<device-id>_<attribute>", ...), recorded by the writer of devices.yaml and by
// recordCommandlineRegistryEntries as they go -- so the registry sync renames them when their positioning changes
// (gap found 2026-10-23, moving protocols-server-2 to another room).
var coordinatorPublishedEntities []TRegistryManifestEntry

// recordInfrastructuralControls records the coordinator's own control entities -- the reload/
// restart buttons per instance, per installation and for "all", and the "discover entity" text
// input (house_event_bus_coordinator/meta_reload_restart.go, discover_entity_input.go) -- so the
// registry repair moves them to their infrastructural entity_id (PROJECT.md item 18, 2026-10-26;
// they were a "meta" sphere of their own before). Their unique_ids keep their "meta_" root.
func recordInfrastructuralControls(instances []string, installation string) {
	if installation == "" {
		return
	}
	segment := func(s string) string { return strings.ReplaceAll(s, ".", "_") }
	entityPart := func(s string) string { return strings.NewReplacer(".", "_", "-", "_").Replace(s) }
	for _, action := range []string{"reload", "restart"} {
		for _, target := range append(append([]string{}, instances...), installation, "all") {
			coordinatorPublishedEntities = append(coordinatorPublishedEntities, TRegistryManifestEntry{
				UniqueID: "meta_" + action + "_" + segment(target),
				EntityID: "button.infrastructural_" + action + "_" + entityPart(target),
			})
		}
	}
	coordinatorPublishedEntities = append(coordinatorPublishedEntities, TRegistryManifestEntry{UniqueID: "meta_discover_entity", EntityID: "text.infrastructural_discover_entity"})
	coordinatorPublishedEntities = append(coordinatorPublishedEntities, TRegistryManifestEntry{UniqueID: "missing_declared_entities", EntityID: "binary_sensor.missing_declared_entities"})
}

// recordingUniqueIDFor wraps admin.UniqueIDFor so every unique_id written is also recorded for the
// manifest.
func recordingUniqueIDFor(admin *TAdministrationState) func(string) string {
	generatedUniqueIDs = map[string]string{}
	return func(entityID string) string {
		uniqueID := admin.UniqueIDFor(entityID)
		generatedUniqueIDs[entityID] = uniqueID
		return uniqueID
	}
}

// generateRegistryManifest writes outputDir/registry_manifest.json -- the last generation step, so
// every YAML-writing step before it has recorded its entities.
func generateRegistryManifest(outputDir string, admin *TAdministrationState) error {
	manifest := TRegistryManifest{YAML: []TRegistryManifestEntry{}, MQTT: []TRegistryManifestEntry{}, Imported: []TRegistryManifestEntry{}, Bridged: []TRegistryManifestEntry{}, Other: []TRegistryManifestOther{}}
	for entityID, uniqueID := range generatedUniqueIDs {
		manifest.YAML = append(manifest.YAML, TRegistryManifestEntry{UniqueID: uniqueID, EntityID: entityID})
	}
	for entityID, link := range admin.DiscoveryEntityLinks {
		if link.StableID != "" {
			manifest.MQTT = append(manifest.MQTT, TRegistryManifestEntry{UniqueID: link.StableID, EntityID: entityID})
		}
	}
	manifest.MQTT = append(manifest.MQTT, coordinatorPublishedEntities...)
	for _, deviceID := range admin.ImportedDeviceIDs {
		for label, attribute := range admin.DeviceConceptualLinks[deviceID].AttributeEntityIDs {
			if attribute.EntityID != "" {
				manifest.Imported = append(manifest.Imported, TRegistryManifestEntry{UniqueID: importedUniqueIDFor(admin, deviceID, label, attribute.EntityID), EntityID: attribute.EntityID})
			}
		}
	}
	for _, deviceID := range admin.BridgedDeviceIDs {
		for label, attribute := range admin.DeviceConceptualLinks[deviceID].AttributeEntityIDs {
			if attribute.EntityID != "" {
				manifest.Bridged = append(manifest.Bridged, TRegistryManifestEntry{UniqueID: bridgedUniqueIDFor(admin, deviceID, label, attribute.EntityID), EntityID: attribute.EntityID})
			}
		}
	}
	manifest.Native = []TRegistryManifestNative{}
	for deviceID, labels := range admin.NativeCapabilities {
		for _, label := range labels {
			if attribute := admin.DeviceConceptualLinks[deviceID].AttributeEntityIDs[label]; attribute.EntityID != "" {
				manifest.Native = append(manifest.Native, TRegistryManifestNative{Source: deviceID + "!" + label, EntityID: attribute.EntityID})
			}
		}
	}
	sort.Slice(manifest.Native, func(i, j int) bool { return manifest.Native[i].Source < manifest.Native[j].Source })
	other, err := collectGeneratedOtherIDs(outputDir)
	if err != nil {
		return err
	}
	manifest.Other = other
	manifest.Areas = entityAreas(admin, manifest)
	for _, list := range [][]TRegistryManifestEntry{manifest.YAML, manifest.MQTT, manifest.Imported, manifest.Bridged} {
		sort.Slice(list, func(i, j int) bool { return list[i].EntityID < list[j].EntityID })
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outputDir, "registry_manifest.json"), append(data, '\n'), 0o644)
}

// importedUniqueIDFor is the unique_id the coordinator publishes an imported capability under:
// its frozen "as <id>" when assigned, otherwise the coordinator's own name-based formula
// (house_event_bus_coordinator/discoveryimport.go's importedUniqueID).
func importedUniqueIDFor(admin *TAdministrationState, deviceID, label, entityID string) string {
	if id, frozen := admin.FrozenUniqueIDs["import:"+deviceID+"!"+label]; frozen {
		return id
	}
	return "imported_" + strings.ReplaceAll(entityID, ".", "_")
}

// bridgedUniqueIDFor is the unique_id the coordinator publishes a bridged ("home_assistant")
// capability under: its frozen "as <id>" when assigned, otherwise the coordinator's own name-based
// formula (house_event_bus_coordinator/discoveryhassbridge.go's hassBridgeUniqueID).
func bridgedUniqueIDFor(admin *TAdministrationState, deviceID, label, entityID string) string {
	if id, frozen := admin.FrozenUniqueIDs["physical:"+deviceID+"!"+label]; frozen {
		return id
	}
	return "hassbridge_" + strings.ReplaceAll(entityID, ".", "_")
}

// generatedOtherDirs maps each generated YAML tree of automations, scripts and input_* helpers to
// its HA platform.
var generatedOtherDirs = map[string]string{
	"automation":              "automation",
	"script":                  "script",
	"entities/input_boolean":  "input_boolean",
	"entities/input_number":   "input_number",
	"entities/input_datetime": "input_datetime",
	"entities/input_select":   "input_select",
	"entities/input_text":     "input_text",
}

// automationIDPattern matches an automation's own "id:" (its unique_id) -- directly in its list
// item, never a deeper one (a trigger's own "id:", say). topLevelKeyPattern matches a script's or
// input_* helper's own key (its unique_id).
var automationIDPattern = regexp.MustCompile(`^(?:- |  )id:\s*(\S+)\s*$`)
var topLevelKeyPattern = regexp.MustCompile(`^([a-z0-9_]+):\s*$`)

// collectGeneratedOtherIDs reads the unique_id of every automation, script and input_* helper just
// written under outputDir.
func collectGeneratedOtherIDs(outputDir string) ([]TRegistryManifestOther, error) {
	var other []TRegistryManifestOther
	dirs := make([]string, 0, len(generatedOtherDirs))
	for dir := range generatedOtherDirs {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		platform := generatedOtherDirs[dir]
		err := filepath.WalkDir(filepath.Join(outputDir, dir), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return filepath.SkipDir
				}
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".yaml") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			pattern := topLevelKeyPattern
			if platform == "automation" {
				pattern = automationIDPattern
			}
			for _, line := range strings.Split(string(data), "\n") {
				if m := pattern.FindStringSubmatch(line); m != nil {
					other = append(other, TRegistryManifestOther{Platform: platform, UniqueID: m[1]})
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return other, nil
}

// entityAreas computes TRegistryManifest.Areas for every entity listed in manifest.
func entityAreas(admin *TAdministrationState, manifest TRegistryManifest) map[string]string {
	spaceArea := map[string]string{}
	for spaceName, records := range admin.EntityRecordsBySpace {
		for _, rec := range records {
			spaceArea[toHomeAssistantEntityID(rec.Name)] = admin.SpaceAreas[spaceName]
		}
	}
	deviceArea := func(deviceID string) string {
		return admin.DeviceConceptualLinks[deviceID].ConstantAttributes["suggested_area"].Value
	}
	// The device of a bridged, host, imported or commandline entity: its node and attributes.
	entityDevice := map[string]string{}
	for deviceID, link := range admin.DeviceConceptualLinks {
		if link.NodeEntityID != "" {
			entityDevice[link.NodeEntityID] = deviceID
		}
		for _, attribute := range link.AttributeEntityIDs {
			if attribute.EntityID != "" {
				entityDevice[attribute.EntityID] = deviceID
			}
		}
	}
	// A discovery device has no area of its own: it is where its positioned entities are, so its
	// hidden ones (e.g. a dimmer's button event) go there too.
	gatewayArea := map[string]string{}
	for _, entityID := range sortedMapKeys(admin.DiscoveryEntityLinks) {
		link := admin.DiscoveryEntityLinks[entityID]
		if area := spaceArea[entityID]; area != "" && gatewayArea[link.GatewayDeviceID] == "" {
			gatewayArea[link.GatewayDeviceID] = area
		}
	}
	// A switched light's hidden parts (its own device and the power switch) are where it is.
	for _, relation := range admin.SwitchedDeviceRelations {
		if area := admin.SpaceAreas[relation.SpaceName]; area != "" {
			for _, name := range []string{relation.Device, relation.MainEntity} {
				if id := toHomeAssistantEntityID(name); spaceArea[id] == "" {
					spaceArea[id] = area
				}
			}
		}
	}
	// A device's area, whatever kind it is: its own, else that of its positioned entities.
	entityAreaOfDevice := map[string]string{}
	for _, entityID := range sortedMapKeys(entityDevice) {
		deviceID := entityDevice[entityID]
		if area := spaceArea[entityID]; area != "" && entityAreaOfDevice[deviceID] == "" {
			entityAreaOfDevice[deviceID] = area
		}
	}
	anyDeviceArea := func(deviceID string) string {
		for _, area := range []string{deviceArea(deviceID), gatewayArea[deviceID], entityAreaOfDevice[deviceID]} {
			if area != "" {
				return area
			}
		}
		return ""
	}
	// A hidden "hosts" node (a ping merged into the device depending on it) is where that device is.
	nodeArea := func(nodeDeviceID string) string {
		for _, dependent := range admin.NodeDependents[nodeDeviceID] {
			if area := anyDeviceArea(dependent); area != "" {
				return area
			}
		}
		return ""
	}
	// An entity without a space of its own (a space aggregate) is in the space its id is named
	// after: the longest space name the id starts with.
	spacePrefixes := make([]string, 0, len(admin.SpaceAreas))
	for spaceName := range admin.SpaceAreas {
		spacePrefixes = append(spacePrefixes, spaceName)
	}
	sort.Slice(spacePrefixes, func(i, j int) bool { return len(spacePrefixes[i]) > len(spacePrefixes[j]) })
	// Matched without the sphere: "physical_house_bathroom_heating_should_be_off" is in the space
	// social/house/bathroom.
	withoutSphere := func(name string) string {
		_, rest, _ := strings.Cut(name, "_")
		return rest
	}
	areaByIDPrefix := func(entityID string) string {
		_, object, _ := strings.Cut(entityID, ".")
		object = withoutSphere(object)
		for _, spaceName := range spacePrefixes {
			if prefix := withoutSphere(strings.TrimPrefix(toHomeAssistantEntityID("x."+spaceName), "x.")); prefix != "" && (object == prefix || strings.HasPrefix(object, prefix+"_")) {
				return admin.SpaceAreas[spaceName]
			}
		}
		return ""
	}
	// A hidden physical entity is named after its device ("physical_<device id>_<label>"): it is
	// where that device is (or, for a merged node, the device depending on it).
	deviceIDs := make([]string, 0, len(admin.DeviceConceptualLinks)+len(admin.NodeDependents))
	for deviceID := range admin.DeviceConceptualLinks {
		deviceIDs = append(deviceIDs, deviceID)
	}
	for deviceID := range admin.NodeDependents {
		deviceIDs = append(deviceIDs, deviceID)
	}
	sort.Slice(deviceIDs, func(i, j int) bool { return len(deviceIDs[i]) > len(deviceIDs[j]) })
	areaByDeviceName := func(entityID string) string {
		_, object, _ := strings.Cut(entityID, ".")
		rest, isPhysical := strings.CutPrefix(object, "physical_")
		if !isPhysical {
			return ""
		}
		for _, deviceID := range deviceIDs {
			if prefix := sanitizeObjectID(strings.ReplaceAll(deviceID, ".", "_")); rest == prefix || strings.HasPrefix(rest, prefix+"_") {
				if area := anyDeviceArea(deviceID); area != "" {
					return area
				}
				return nodeArea(deviceID)
			}
		}
		return ""
	}
	areas := map[string]string{}
	for _, list := range [][]TRegistryManifestEntry{manifest.YAML, manifest.MQTT, manifest.Imported, manifest.Bridged} {
		for _, entry := range list {
			// A positioned entity is where it is positioned -- one device can serve several rooms
			// (a two-channel relay); only an unpositioned one goes by its device.
			area := spaceArea[entry.EntityID]
			if area == "" {
				if link, found := admin.DiscoveryEntityLinks[entry.EntityID]; found {
					area = deviceArea(link.GatewayDeviceID)
					if area == "" {
						area = gatewayArea[link.GatewayDeviceID]
					}
				} else if deviceID, found := entityDevice[entry.EntityID]; found {
					area = deviceArea(deviceID)
					if area == "" {
						area = nodeArea(deviceID)
					}
				}
			}
			if area == "" {
				area = areaByDeviceName(entry.EntityID)
			}
			if _, positioned := spaceArea[entry.EntityID]; area == "" && !positioned {
				area = areaByIDPrefix(entry.EntityID)
			}
			if area == "" {
				area = defaultArea
			}
			areas[entry.EntityID] = area
		}
	}
	// Entities positioned in Conceptual.def but created by an integration of their own (a Sonos's
	// media_player, an imported host) aren't in the lists above; their position gives the area too.
	for _, spaceName := range sortedMapKeys(admin.EntityRecordsBySpace) {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			entityID := toHomeAssistantEntityID(rec.Name)
			if _, listed := areas[entityID]; listed || entityID == "" || strings.HasPrefix(rec.Provenance, "derived") {
				continue
			}
			area := admin.SpaceAreas[spaceName]
			if area == "" {
				area = defaultArea
			}
			areas[entityID] = area
		}
	}
	return areas
}

// conditionExprRefs returns every device or capability reference in a Logical.def condition tree.
func conditionExprRefs(expr *TConditionExpr) []string {
	if expr == nil {
		return nil
	}
	var refs []string
	if expr.Ref != "" {
		refs = append(refs, expr.Ref)
	}
	if expr.FromDeviceID != "" {
		refs = append(refs, expr.FromDeviceID)
	}
	for _, child := range []*TConditionExpr{expr.Left, expr.Right, expr.Child} {
		refs = append(refs, conditionExprRefs(child)...)
	}
	return refs
}

// sortedMapKeys returns m's keys in order, for a deterministic "first one wins".
func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
