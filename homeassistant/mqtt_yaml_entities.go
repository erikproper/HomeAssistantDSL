/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: MQTTYAMLEntities
 *
 * PROJECT.md 19 (2026-10-27): every discovery device declared in Physical.def gets its entities
 * from the generator's own YAML "mqtt:" config, never from a discovery relay (only an undeclared
 * device can still reach Home Assistant by discovery, through the passthrough, for a migration)
 * -- one file per entity, entities/mqtt/<domain>/<entity_id>.yaml (included from
 * configuration.yaml as "mqtt: !include integrations/mqtt.yaml").
 *
 * Each file is built from the gateway's own raw discovery config (collected by the coordinator for
 * every entity of every declared gateway, declared or not: discovery_raw_configs/<gateway>/state,
 * cached in Definitions/.cache/discovery_raw_configs.json) and the declaration, by the same
 * transformation as the coordinator's relay (hamqtt.RelayedConfig), so the entity is what the relay
 * would have published: same unique_id (the frozen stable_id), same default_entity_id (its
 * entity_id, so a re-created registry entry gets it back, and with it the history), same conceptual
 * name. A capability declared for the first time therefore gets its YAML on that very generate.
 * Its device block takes the metadata captured in Physical.def (19b) and the area its entities are
 * positioned in (suggested_area).
 *
 * The coordinator relays no discovery to Home Assistant (PROJECT.md 19): it only records each
 * gateway's raw configs (discovery_raw_configs/<gateway>/state), which this file reads. There is
 * no fallback to discovery: an entity without a raw config is reported, and left out until there
 * is one.
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
	"sort"
	"strings"

	"homeassistant/hamqtt"

	"gopkg.in/yaml.v3"
)

func discoveryRawConfigsCachePath(definitionDir string) string {
	return filepath.Join(definitionDir, ".cache", "discovery_raw_configs.json")
}

// TDiscoveryRawConfigs is every declared gateway's raw discovery configs, as the coordinator
// collects them (house_event_bus_coordinator/discovery_device_info.go): gateway id -> "<raw
// domain>/<unique_id>" -> the config as the gateway published it.
type TDiscoveryRawConfigs map[string]map[string]map[string]interface{}

// fetchDiscoveryRawConfigs returns the gateways' raw configs, from a broker when one answers
// (refreshing the cache), else from the cache. A gateway the broker didn't deliver keeps its cached
// configs: over a slow link the retained aggregates can arrive further apart than
// fetchRetainedAll's quiet period, and a partial fetch must never lose the rest (found 2026-10-27,
// Junglinster over the cloud broker, 46 gateways left without YAML).
func fetchDiscoveryRawConfigs(definitionDir string, ctx TPhysicalGenerationContext) (TDiscoveryRawConfigs, error) {
	var messages map[string][]byte
	prefix := ""
	if cloudSecrets, ok := coordinatorOnlyBrokerSecrets(ctx); ok {
		prefix = ctx.Installation + "/"
		messages, _ = fetchRetainedAll(cloudSecrets, prefix+"discovery_raw_configs/+/state", "homeassistant-generator-raw-configs")
	}
	if messages == nil {
		prefix = ""
		messages, _ = fetchRetainedAll(ctx.MQTTSecrets, "discovery_raw_configs/+/state", "homeassistant-generator-raw-configs")
	}
	cachePath := discoveryRawConfigsCachePath(definitionDir)
	configs := TDiscoveryRawConfigs{}
	data, cacheErr := os.ReadFile(cachePath)
	if cacheErr == nil {
		if err := json.Unmarshal(data, &configs); err != nil {
			configs = TDiscoveryRawConfigs{}
		}
	}
	if messages == nil {
		if cacheErr != nil {
			return nil, fmt.Errorf("no broker answered and no cache at %s yet", cachePath)
		}
		return configs, nil
	}
	for topic, payload := range messages {
		gatewayID := strings.TrimSuffix(strings.TrimPrefix(topic, prefix+"discovery_raw_configs/"), "/state")
		var gatewayConfigs map[string]map[string]interface{}
		if json.Unmarshal(payload, &gatewayConfigs) == nil {
			configs[gatewayID] = gatewayConfigs
		}
	}
	if data, err := json.Marshal(configs); err == nil {
		_ = os.MkdirAll(filepath.Dir(cachePath), 0o755)
		_ = os.WriteFile(cachePath, data, 0o644)
	}
	return configs, nil
}

// rawConfigFor returns link's raw config among its gateway's, and the domain it was published
// under: the one of link.SourceDomain when set, else the entity's own domain, else the only one.
func rawConfigFor(configs TDiscoveryRawConfigs, entityID string, link TDiscoveryEntityLink) (map[string]interface{}, string, bool) {
	gatewayConfigs := configs[link.GatewayDeviceID]
	domain := entityID[:strings.Index(entityID, ".")]
	for _, candidate := range []string{link.SourceDomain, domain} {
		if raw, found := gatewayConfigs[candidate+"/"+link.Leaf]; candidate != "" && found {
			return raw, candidate, true
		}
	}
	var keys []string
	for key := range gatewayConfigs {
		if strings.HasSuffix(key, "/"+link.Leaf) {
			keys = append(keys, key)
		}
	}
	if len(keys) != 1 || link.SourceDomain != "" {
		return nil, "", false
	}
	return gatewayConfigs[keys[0]], keys[0][:strings.Index(keys[0], "/")], true
}

// generateYAMLMQTTEntities writes the YAML "mqtt:" entities of every device marked "entities via
// yaml;", built from the gateway's own raw discovery configs and the declaration -- the same
// transformation as the coordinator's relay (hamqtt.RelayedConfig) -- records them in
// admin.YAMLEntities, and reports what it couldn't do. Never a fallback to discovery: an entity
// without a raw config yet (the gateway never published it, nothing cached) is reported and left
// out until it has one.
func generateYAMLMQTTEntities(definitionDir string, gateways map[string]TDiscoveryGatewayDevice, ctx TPhysicalGenerationContext) []string {
	if ctx.HAOutputDir == "" {
		return nil // no Home Assistant output directory (a test): nothing to write, never relative to the working directory
	}
	admin := ctx.Admin
	admin.YAMLEntities = map[string]map[string]interface{}{}
	dir := filepath.Join(ctx.HAOutputDir, "entities", "mqtt")
	if err := os.RemoveAll(dir); err != nil {
		return []string{fmt.Sprintf("YAML mqtt entities: %v", err)}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return []string{fmt.Sprintf("YAML mqtt entities: %v", err)}
	}
	var marked []string
	for id := range gateways {
		marked = append(marked, id)
	}
	if len(marked) == 0 {
		return nil
	}
	sort.Strings(marked)
	configs, err := fetchDiscoveryRawConfigs(definitionDir, ctx)
	if err != nil {
		return []string{fmt.Sprintf("YAML mqtt entities: %v -- ERROR: no YAML entities for the discovery devices", err)}
	}
	// PROJECT.md 39: an "integration mqtt" device's raw configs are the generator's own.
	for id, gateway := range gateways {
		if gateway.Raw {
			configs[id] = rawMQTTConfigs(gateway)
		}
	}

	spaceArea := map[string]string{}
	for spaceName, records := range admin.EntityRecordsBySpace {
		for _, rec := range records {
			spaceArea[toHomeAssistantEntityID(rec.Name)] = admin.SpaceAreas[spaceName]
		}
	}
	linksPerGateway := map[string]int{}
	for _, link := range admin.DiscoveryEntityLinks {
		linksPerGateway[link.GatewayDeviceID]++
	}
	var reports []string
	// PROJECT.md 20: state topics that can't be trusted as retained read the coordinator's shadow.
	retainFilters := collectDiscoveryRetainedTopics(definitionDir)
	// PROJECT.md 22b: the gateway-level availability topics, for "ignore own availability;".
	var sharedAvailability map[string]bool
	for _, gateway := range gateways {
		if gateway.IgnoreOwnAvailability {
			sharedAvailability = sharedAvailabilityTopics(configs)
			break
		}
	}
	seenStateTopics := map[string]bool{}
	shadowedTopics := map[string]bool{}
	for _, gatewayID := range marked {
		var gatewaySeen []string
		ownDropped := map[string]bool{}
		ownKeptAll := false
		var entityIDs []string
		for entityID, link := range admin.DiscoveryEntityLinks {
			if link.GatewayDeviceID == gatewayID {
				entityIDs = append(entityIDs, entityID)
			}
		}
		sort.Strings(entityIDs)
		if len(entityIDs) == 0 {
			// E.g. a group's member bulbs: only the group's entities are positioned.
			reports = append(reports, fmt.Sprintf("%s: no positioned entities -- nothing to write", gatewayID))
			continue
		}
		area := ""
		for _, entityID := range entityIDs {
			if area == "" {
				area = spaceArea[entityID]
			}
		}
		written := 0
		var missing []string
		var noted []THostYAMLEntity
		for _, entityID := range entityIDs {
			link := admin.DiscoveryEntityLinks[entityID]
			raw, rawDomain, found := rawConfigFor(configs, entityID, link)
			if !found {
				missing = append(missing, entityID+" (leaf "+link.Leaf+")")
				continue
			}
			rawData, _ := json.Marshal(raw)
			decoded, err := hamqtt.Decode(rawData)
			if err != nil {
				missing = append(missing, entityID+": "+err.Error())
				continue
			}
			name := ""
			if identity, found := entityIdentityByID(admin, entityID); found && identity.Sphere != "" {
				name = friendlyEntityName(identity)
			}
			relayed, ok := hamqtt.RelayedConfig(entityID, gatewayID, raw, decoded, hamqtt.Link{
				StableID: link.StableID, Name: name, DeviceClass: link.DeviceClass, Unit: link.Unit,
				StateClass: link.StateClass, Icon: link.Icon, ValueTemplateWrap: link.ValueTemplateWrap, Invert: link.Invert,
			}, rawDomain, linksPerGateway[gatewayID] == 1)
			if !ok {
				missing = append(missing, entityID+" (no unique_id in its raw config)")
				continue
			}
			domain := entityID[:strings.Index(entityID, ".")]
			body := yamlEntityBody(relayed, domain, gateways[gatewayID], area)
			// A Logical.def "dependency on" (e.g. an integration device on its compute host):
			// ANDed into the availability, as for every other kind's entities.
			if topics := admin.DependsOnAvailabilityTopics[gatewayID]; len(topics) > 0 {
				addAvailabilityEntries(body, hamqtt.NodeAvailabilityEntries(topics...))
			}
			if gateways[gatewayID].CoordinatorStatus {
				addCoordinatorAvailability(body)
			}
			if gateways[gatewayID].Raw {
				// An "integration mqtt" device's name is its positioned one: nothing reports one.
				if device, isMap := body["device"].(map[string]interface{}); isMap {
					if name := admin.DeviceConceptualLinks[gatewayID].DisplayName; name != "" {
						device["name"] = name
					}
				}
			}
			if gateways[gatewayID].IgnoreOwnAvailability {
				dropped, kept := keepSharedAvailability(body, sharedAvailability)
				for _, topic := range dropped {
					ownDropped[topic] = true
				}
				if !kept {
					ownKeptAll = true
				}
			}
			seen, shadowed := applyRetainShadows(body, domain, gateways[gatewayID].Retain, retainFilters)
			gatewaySeen = append(gatewaySeen, seen...)
			for _, topic := range seen {
				seenStateTopics[topic] = true
			}
			for _, topic := range shadowed {
				shadowedTopics[topic] = true
			}
			noted = append(noted, THostYAMLEntity{Domain: domain, EntityID: entityID, Dropped: hamqtt.DroppedFields(relayed, domain)})
			data, err := yaml.Marshal([]map[string]interface{}{{domain: body}})
			if err != nil {
				reports = append(reports, fmt.Sprintf("%s: %v", entityID, err))
				continue
			}
			path := filepath.Join(dir, domain, entityID+".yaml")
			if err := writeYAMLFile(path, generatorHeader+string(data)); err != nil {
				reports = append(reports, fmt.Sprintf("%s: %v", entityID, err))
				continue
			}
			admin.YAMLEntities[entityID] = body
			written++
		}
		report := fmt.Sprintf("%s: %d entities via YAML", gatewayID, written) + droppedFieldsNote(noted)
		if len(ownDropped) > 0 {
			topics := make([]string, 0, len(ownDropped))
			for topic := range ownDropped {
				topics = append(topics, topic)
			}
			sort.Strings(topics)
			report += "; own availability ignored: " + strings.Join(topics, ", ")
		}
		if ownKeptAll {
			report += "; WARNING, \"ignore own availability;\" but an entity has no availability its gateway shares with other devices -- left as it was"
		}
		if declaration := gateways[gatewayID].Retain; declaration != retainUndeclared && len(gatewaySeen) > 0 && !retainDeclarationEffect(gatewaySeen, declaration, retainFilters) {
			report += "; its \"is retained;\"/\"is not retained;\" line has no effect (the \"retained topics\" filters already say the same)"
		}
		if len(missing) > 0 {
			report += fmt.Sprintf("; ERROR, no raw discovery config (not published by the gateway, not cached): %s", strings.Join(missing, ", "))
		}
		reports = append(reports, report)
	}

	var seenList, shadowedList []string
	for topic := range seenStateTopics {
		seenList = append(seenList, topic)
	}
	for topic := range shadowedTopics {
		shadowedList = append(shadowedList, topic)
	}
	sort.Strings(shadowedList)
	for _, filter := range unmatchedRetainFilters(retainFilters, seenList) {
		reports = append(reports, fmt.Sprintf("retained topics: filter %q matches none of the discovered entities' state topics (a typo?)", filter))
	}
	// PROJECT.md 22d: where the coordinator reads each raw device's metadata.
	metadataTopics := TMetadataTopicsFile{Devices: map[string][]TRawMetadataTopic{}}
	for id, gateway := range gateways {
		if len(gateway.RawMetadata) > 0 {
			metadataTopics.Devices[id] = gateway.RawMetadata
		}
	}
	if data, err := yaml.Marshal(metadataTopics); err == nil {
		if err := writeYAMLFile(filepath.Join(ctx.OutputRoot, "coordinator", "metadata_topics.yaml"), generatorHeader+string(data)); err != nil {
			reports = append(reports, fmt.Sprintf("metadata topics: %v", err))
		}
	}
	data, err := yaml.Marshal(TRetainShadowsFile{Topics: shadowedList})
	if err == nil {
		err = writeYAMLFile(filepath.Join(ctx.OutputRoot, "coordinator", "retain_shadows.yaml"), generatorHeader+string(data))
	}
	if err != nil {
		reports = append(reports, fmt.Sprintf("retain shadows: %v", err))
	}
	return reports
}

// yamlEntityBody turns the coordinator's relayed discovery config into a YAML "mqtt:" entity: the
// discovery-only keys go, its device block takes the device's captured metadata and its area.
func yamlEntityBody(relayed map[string]interface{}, domain string, gateway TDiscoveryGatewayDevice, area string) map[string]interface{} {
	body := hamqtt.DiscoveredConfig(relayed, domain)
	device, _ := body["device"].(map[string]interface{})
	if device == nil {
		device = map[string]interface{}{}
	}
	copied := map[string]interface{}{}
	for key, value := range device {
		copied[key] = value
	}
	for field, attribute := range gateway.Metadata {
		copied[field] = attribute.Value
	}
	if area != "" {
		copied["suggested_area"] = area
	}
	body["device"] = copied
	return body
}

// validateYAMLMQTTEntities checks every YAML "mqtt:" entity under haOutputDir against Home
// Assistant's field table, and returns one problem per file with a field its domain (or light
// schema) doesn't know. Run after every writer (PROJECT.md 19h.3, 2026-10-27): one such field
// makes Home Assistant's WHOLE MQTT reload fail -- found live, when the bridge writer passed the
// relay's state_topic on a button through and 64 entities went missing.
func validateYAMLMQTTEntities(haOutputDir string) []string {
	if haOutputDir == "" {
		return nil
	}
	files, _ := filepath.Glob(filepath.Join(haOutputDir, "entities", "mqtt", "*", "*.yaml"))
	var problems []string
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", file, err))
			continue
		}
		var parsed []map[string]map[string]interface{}
		if err := yaml.Unmarshal(data, &parsed); err != nil || len(parsed) != 1 {
			problems = append(problems, fmt.Sprintf("%s: not one YAML mqtt entity (%v)", file, err))
			continue
		}
		for domain, body := range parsed[0] {
			if domain == "vacuum" {
				continue // vacuum's schema alone allows extra fields (ALLOW_EXTRA)
			}
			known := hamqtt.Fields.Domains[domain]
			if domain == "light" {
				schema, _ := body["schema"].(string)
				if schema == "" {
					schema = "basic"
				}
				known = hamqtt.Fields.LightSchemas[schema]
			}
			accepted := map[string]bool{}
			for _, field := range known {
				accepted[field] = true
			}
			var unknown []string
			for field := range body {
				if !accepted[field] {
					unknown = append(unknown, field)
				}
			}
			if len(known) == 0 {
				unknown = []string{"(domain " + domain + " unknown to the field table)"}
			}
			if len(unknown) > 0 {
				sort.Strings(unknown)
				problems = append(problems, fmt.Sprintf("%s: fields Home Assistant doesn't know for %s: %s", filepath.Base(file), domain, strings.Join(unknown, ", ")))
			}
		}
	}
	return problems
}

// addAvailabilityEntries ANDs entries into body's availability (availability_mode "all"), turning a
// single "availability_topic" into the list form first.
func addAvailabilityEntries(body map[string]interface{}, entries []map[string]interface{}) {
	var list []interface{}
	if existing, isList := body["availability"].([]interface{}); isList {
		list = existing
	}
	if topic, ok := body["availability_topic"].(string); ok && topic != "" {
		entry := map[string]interface{}{"topic": topic}
		for _, key := range []string{"payload_available", "payload_not_available"} {
			if value, has := body[key]; has {
				entry[key] = value
				delete(body, key)
			}
		}
		delete(body, "availability_topic")
		list = append(list, entry)
	}
	for _, entry := range entries {
		list = append(list, entry)
	}
	body["availability"] = list
	body["availability_mode"] = "all"
}
