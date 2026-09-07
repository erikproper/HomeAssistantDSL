/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: MQTTEntityExistence
 *
 * Reads the coordinator's own three-state entity-existence status
 * (house_event_bus_coordinator/entity_existence.go publishes it to
 * "homeassistant_instances/<name>/existence/state", retained, on both the local and cloud broker
 * unconditionally when a cloud one is configured) for two purposes:
 *
 *   1. checkKnownNotToExistErrors turns a known-not-to-exist verdict into a real generate-time
 *      error -- PROJECT.md 1.1's own rule: generate optimistically in every other case
 *      (known-to-exist, or not-known-to-exist because the coordinator hasn't gotten to it yet),
 *      only a *confirmed* absence blocks generation.
 *   2. generateEntityCatalogueSuggestions writes suggestions/home_assistant_<name>.txt --
 *      copy-paste-ready "device hass.<id> with: ...; end;" blocks for every known-to-exist entity
 *      not already claimed by a declared capability.
 *
 * This entirely replaces the older manifest/entities_detailed mechanism (mqtt_entity_catalogue.go,
 * house_event_bus_coordinator/entity_catalogue.go, both deleted 2026-08-28): that mechanism
 * depended on a full-instance-scan bootstrap automation that's permanently disabled (it's the one
 * that hung "main"), so it could never produce fresh data again. Its "which entities does this
 * instance have" job is now folded into the entity-existence inquiry loop itself: an inquiry reply
 * about a known entity also carries device_entities() for its device
 * (homeassistant/remote_instance_entity_existence.go), and the coordinator folds any previously
 * unseen sibling into the same known-to-exist tracking (DiscoverSiblings,
 * house_event_bus_coordinator/entity_existence.go) -- discovery is a byproduct of the ordinary
 * paced cycle, not a separate mechanism, and the suggestion file's device groupings are therefore
 * always real, already-valid Physical.def device ids (no name-guessing needed, unlike the old
 * mechanism's suggestedLocalDeviceName). The one thing this can't do that the old mechanism could:
 * discover an *entirely new* device with no declared capability at all yet (every discovered
 * sibling is attributed to an already-known device) -- accepted scope, not being solved here.
 *
 * Persisted local cache (Definitions/.cache/, fetch-when-online/fall-back-to-cache) -- same
 * standing pattern the old manifest mechanism established, kept for this mechanism too.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 28.08.2026
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
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// fetchExistenceTimeout mirrors fetchManifestTimeout's own reasoning exactly: short, since this
// runs once per instance on every ./generate, and the local cache fallback exists precisely so an
// unreachable broker doesn't stall generation.
const fetchExistenceTimeout = 5 * time.Second

// TEntityExistenceStatusEntry is one tracked entity's status, as published by the coordinator --
// house_event_bus_coordinator/entity_existence.go's existenceStatusEntityPayload is the single
// source of truth for this shape; keep both in sync.
type TEntityExistenceStatusEntry struct {
	Status string `json:"status"`
	State  string `json:"state,omitempty"`
}

// TEntityExistenceStatusDevice is one device's entities, keyed by their source entity_id -- the
// coordinator's own existenceStatusDevicePayload.
type TEntityExistenceStatusDevice struct {
	Entities map[string]TEntityExistenceStatusEntry `json:"entities"`
}

// TEntityExistenceStatusPayload is the top-level shape published to
// "homeassistant_instances/<name>/existence/state" -- keyed by device id ("" holds any entity with
// no owning device).
type TEntityExistenceStatusPayload map[string]TEntityExistenceStatusDevice

func existenceStatusTopic(instanceName string) string {
	return "homeassistant_instances/" + instanceName + "/existence/state"
}

// coordinatorOnlyBrokerSecrets returns whichever "coordinator_only true;" broker profile is
// declared (e.g. "cloud_coordinator") -- reachable from anywhere, since the coordinator relays
// existence status onto it unconditionally alongside the local broker (entity_existence.go's
// publishStatus, coordinator side) -- ok is false if no such profile exists.
func coordinatorOnlyBrokerSecrets(ctx TPhysicalGenerationContext) (TMQTTBrokerSecrets, bool) {
	for _, secrets := range ctx.MQTTBrokerProfiles {
		if secrets.CoordinatorOnly {
			return secrets, true
		}
	}
	return TMQTTBrokerSecrets{}, false
}

// existenceCachePath is instanceName's own local, persisted cache file -- under Definitions/.cache/
// (definitionDir, not any generated-output directory), same convention manifestCachePath already
// established.
func existenceCachePath(definitionDir, instanceName string) string {
	return filepath.Join(definitionDir, ".cache", "entity_existence_"+instanceName+".json")
}

// fetchEntityExistence tries a fresh read, cloud broker first then local (fetchEntityExistencePreferringCloud
// -- same standing policy as fetchManifest, mqtt_entity_catalogue.go: prefer cloud reachability,
// fall back to local), and on success updates instanceName's own local cache file. On any failure,
// falls back to that cache's last-known contents. Returns an error only when neither live broker
// nor a usable cache is available.
func fetchEntityExistence(definitionDir string, ctx TPhysicalGenerationContext, instanceName string) (TEntityExistenceStatusPayload, error) {
	cachePath := existenceCachePath(definitionDir, instanceName)

	status, fetchErr := fetchEntityExistencePreferringCloud(ctx, instanceName)
	if fetchErr == nil {
		if data, err := json.Marshal(status); err == nil {
			if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err == nil {
				if err := os.WriteFile(cachePath, data, 0o644); err != nil {
					fmt.Printf("[physical] entity existence for %q: fetched fresh but could not update local cache %s: %v\n", instanceName, cachePath, err)
				}
			}
		}
		return status, nil
	}

	cached, cacheErr := os.ReadFile(cachePath)
	if cacheErr != nil {
		return nil, fmt.Errorf("%w (and no local cache at %s to fall back to)", fetchErr, cachePath)
	}
	var cachedStatus TEntityExistenceStatusPayload
	if err := json.Unmarshal(cached, &cachedStatus); err != nil {
		return nil, fmt.Errorf("%w (local cache at %s is also unreadable: %v)", fetchErr, cachePath, err)
	}
	fmt.Printf("[physical] entity existence for %q: live fetch failed (%v), using cached copy from %s\n", instanceName, fetchErr, cachePath)
	return cachedStatus, nil
}

// fetchEntityExistencePreferringCloud tries the "coordinator_only" cloud broker first (when
// declared), falling back to the local broker (ctx.MQTTSecrets) if that connection fails or no
// cloud profile exists -- matches fetchManifestPreferringCloud's own policy exactly
// (mqtt_entity_catalogue.go): the coordinator publishes existence status on both local and cloud
// unconditionally when a cloud broker is configured (house_event_bus_coordinator/entity_existence.go's
// publishStatus), so the generator should read via whichever is reachable, cloud first.
func fetchEntityExistencePreferringCloud(ctx TPhysicalGenerationContext, instanceName string) (TEntityExistenceStatusPayload, error) {
	if cloudSecrets, ok := coordinatorOnlyBrokerSecrets(ctx); ok {
		if status, err := fetchEntityExistenceFromBroker(cloudSecrets, instanceName, ctx.Installation); err == nil {
			return status, nil
		} else {
			// Printed, not swallowed: a silent cloud failure here previously made a subsequent
			// local-broker failure look like the *only* attempt made, which is misleading -- the
			// cloud broker can fail for a perfectly ordinary reason too (e.g. the coordinator
			// simply hasn't published anything for this instance yet).
			fmt.Printf("[physical] entity existence for %q: cloud broker attempt failed (%v), trying local\n", instanceName, err)
		}
	}
	return fetchEntityExistenceFromBroker(ctx.MQTTSecrets, instanceName, "")
}

// fetchEntityExistenceFromBroker connects to secrets' broker as a one-shot client and returns
// whatever the coordinator has last published (retained) on existenceStatusTopic(instanceName).
//
// ownInstallation, when non-empty, additionally prefixes the topic with this house's own
// installation name -- the CLOUD broker's own qualification convention
// (house_event_bus_coordinator/entity_existence.go's publishStatus, fixed live 2026-09-05 after two
// houses' own "main" instances were found colliding on the shared cloud broker's bare topic). Pass
// "" for the local broker, whose copy stays bare -- every instanceName named in this house's own
// declarations is, by construction, this same house's own instance, so the local broker never needs
// disambiguating.
func fetchEntityExistenceFromBroker(secrets TMQTTBrokerSecrets, instanceName, ownInstallation string) (TEntityExistenceStatusPayload, error) {
	topic := existenceStatusTopic(instanceName)
	if ownInstallation != "" {
		topic = ownInstallation + "/" + topic
	}

	opts := mqtt.NewClientOptions()
	scheme := "tcp"
	if secrets.TLS {
		scheme = "ssl"
	}
	opts.AddBroker(fmt.Sprintf("%s://%s:%s", scheme, secrets.Server, secrets.Port))
	opts.SetClientID("homeassistant-generator-existence-" + instanceName)
	opts.SetUsername(secrets.Login)
	opts.SetPassword(secrets.Password)
	opts.SetConnectTimeout(fetchExistenceTimeout)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(fetchExistenceTimeout) || token.Error() != nil {
		if err := token.Error(); err != nil {
			return nil, fmt.Errorf("connecting to MQTT broker %s:%s: %w", secrets.Server, secrets.Port, err)
		}
		return nil, fmt.Errorf("connecting to MQTT broker %s:%s: timed out", secrets.Server, secrets.Port)
	}
	defer client.Disconnect(250)

	received := make(chan []byte, 1)
	subToken := client.Subscribe(topic, 0, func(_ mqtt.Client, msg mqtt.Message) {
		select {
		case received <- msg.Payload():
		default:
		}
	})
	if !subToken.WaitTimeout(fetchExistenceTimeout) || subToken.Error() != nil {
		if err := subToken.Error(); err != nil {
			return nil, fmt.Errorf("subscribing to %s: %w", topic, err)
		}
		return nil, fmt.Errorf("subscribing to %s: timed out", topic)
	}

	select {
	case payload := <-received:
		var status TEntityExistenceStatusPayload
		if err := json.Unmarshal(payload, &status); err != nil {
			return nil, fmt.Errorf("parsing %s payload: %w", topic, err)
		}
		return status, nil
	case <-time.After(fetchExistenceTimeout):
		return nil, fmt.Errorf("timed out waiting for %s (has the coordinator published existence status for this instance yet?)", topic)
	}
}

// checkKnownNotToExistErrors fetches (fetch-then-cache-fallback) existence status for every
// distinct instance hassBridgeDevicesByID references, and returns a combined error listing every
// declared capability whose source entity the coordinator has confirmed known-not-to-exist --
// PROJECT.md 1.1's own rule: this is the *only* status that blocks generation; not-known-to-exist
// (unresolved) and known-to-exist both generate optimistically, same as before this mechanism
// existed. Soft-fails (a warning, not an error) per instance when neither a fresh read nor a cache
// is available -- an instance the coordinator hasn't reported on yet shouldn't block generation
// any more than "no cache yet" already doesn't for the older manifest mechanism.
func checkKnownNotToExistErrors(definitionDir string, hassBridgeDevicesByID map[string]THassBridgeDevice, ctx TPhysicalGenerationContext) error {
	instanceNames := map[string]bool{}
	for _, device := range hassBridgeDevicesByID {
		for _, instance := range device.Instances {
			if instance != "" {
				instanceNames[instance] = true
			}
		}
	}
	if len(instanceNames) == 0 {
		return nil
	}

	statusByInstance := map[string]TEntityExistenceStatusPayload{}
	for name := range instanceNames {
		status, err := fetchEntityExistence(definitionDir, ctx, name)
		if err != nil {
			fmt.Printf("[physical] entity existence for %q: %v\n", name, err)
			continue
		}
		statusByInstance[name] = status
	}

	deviceIDs := make([]string, 0, len(hassBridgeDevicesByID))
	for id := range hassBridgeDevicesByID {
		deviceIDs = append(deviceIDs, id)
	}
	sort.Strings(deviceIDs)

	var problems []string
	for _, deviceID := range deviceIDs {
		device := hassBridgeDevicesByID[deviceID]
		// A roaming device (ExportAs != "") may be declared identically in more than one house's
		// own Physical.def, each with its own local Instances -- but this house's generator only
		// ever loads its OWN Physical.def, so device.Instances here is only ever THIS house's own
		// subset. "Every declared instance we have status for confirms not-to-exist" (the check
		// just below) is therefore meaningless for a roaming device: "not confirmed on any local
		// instance this house declares" says nothing about whether it's genuinely paired with
		// another house's own instance this generator has no visibility into at all. Real incident,
		// found live 2026-09-05: Vienna's own hass.eriks_iphone declaration (Instances: ["main"])
		// failed generation the moment Vienna's coordinator's own existence status became reliably
		// fresh (see entity_existence.go's StartEntityExistenceInquiries immediate-publish fix),
		// even though the SAME roaming device is confirmed known-to-exist on Junglinster's own
		// instances -- information Vienna's generator can't see and was never meant to need. Skip
		// the hard-fail check entirely for a roaming device -- generate optimistically, same as any
		// not-known-to-exist source, regardless of what THIS house's own local instance(s) confirm.
		if device.ExportAs != "" {
			continue
		}
		capabilityNames := make([]string, 0, len(device.Capabilities))
		for name := range device.Capabilities {
			capabilityNames = append(capabilityNames, name)
		}
		sort.Strings(capabilityNames)
		for _, capability := range capabilityNames {
			cap := device.Capabilities[capability]
			// A capability is only a real problem when EVERY declared instance we have status
			// for confirms it not-to-exist -- a device declared on more than one LOCAL instance
			// only needs to exist on ONE of them to actually work. Each instance's OWN declared
			// source is looked up per-instance (cap.Sources[instance]), never a shared value -- a
			// roaming device's own local entity_id for the same capability can genuinely differ
			// across instances (found live 2026-09-05).
			checkedAny, confirmedNotToExist := false, false
			for _, instance := range device.Instances {
				source, declaredHere := cap.Sources[instance]
				if !declaredHere {
					continue
				}
				status, known := statusByInstance[instance]
				if !known {
					continue
				}
				deviceStatus, hasDevice := status[deviceID]
				if !hasDevice {
					continue
				}
				entry, hasEntry := deviceStatus.Entities[source]
				if !hasEntry {
					continue
				}
				checkedAny = true
				if entry.Status != existenceStatusKnownNotToExist {
					confirmedNotToExist = false
					break
				}
				confirmedNotToExist = true
			}
			if !checkedAny || !confirmedNotToExist {
				continue
			}
			problems = append(problems, fmt.Sprintf("%s (device %q, capability %q): source entit(y/ies) %s confirmed not to exist on instance(s) %v",
				capability, deviceID, capability, formatInstanceSources(cap.Sources, device.Instances), device.Instances))
		}
	}

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("entity existence check failed -- the coordinator has confirmed %d declared capability/capabilities reference an entity that does not exist:\n  %s",
		len(problems), strings.Join(problems, "\n  "))
}

// existenceStatusKnownNotToExist/existenceStatusKnownToExist mirror house_event_bus_coordinator's
// own Status* constant strings exactly -- kept as plain strings here (not an imported type; the
// two are separate Go modules) so this file has no build dependency on the coordinator.
const (
	existenceStatusKnownToExist    = "known-to-exist"
	existenceStatusKnownNotToExist = "known-not-to-exist"
)

// recognizedCapabilityKeywords maps a substring commonly found in an entity_id's own descriptive
// name to the capability domain/suffix it implies for a suggested Physical.def line -- e.g.
// "carbon_dioxide" -> sensor/co2. Order matters: first match wins, so a more specific keyword
// (e.g. "carbon_dioxide") must precede a more general one it could also be mistaken for.
var recognizedCapabilityKeywords = []struct {
	keyword string
	domain  string
	suffix  string
}{
	{"connectivity", "binary_sensor", "node"},
	{"carbon_dioxide", "sensor", "co2"},
	{"co2", "sensor", "co2"},
	{"humidity", "sensor", "humidity"},
	{"noise", "sensor", "noise"},
	{"pressure", "sensor", "pressure"},
	{"temperature", "sensor", "temperature"},
	{"wi_fi_strength", "sensor", "radio"},
	{"rf_strength", "sensor", "radio"},
	{"battery", "sensor", "battery_level"},
	{"uptime", "sensor", "uptime"},
}

// recognizeCapability guesses entityID's local capability domain/suffix from a known keyword in
// its own descriptive name -- suffix is "" if nothing matched (domain still falls back to the
// entity's own domain prefix, so the suggested line is at least domain-correct even unrecognised).
func recognizeCapability(entityID string) (domain, suffix string) {
	for _, k := range recognizedCapabilityKeywords {
		if strings.Contains(entityID, k.keyword) {
			return k.domain, k.suffix
		}
	}
	domain = entityID
	if idx := strings.Index(entityID, "."); idx >= 0 {
		domain = entityID[:idx]
	}
	return domain, ""
}

// usedHassBridgeEntityIDs returns every bare source entity_id already referenced (as any
// capability's or device-info field's source) by a declared "home_assistant" bridge device
// belonging to instanceName -- what buildSuggestionReportFromExistence excludes from its output,
// since those are already positioned.
func usedHassBridgeEntityIDs(hassBridgeDevicesByID map[string]THassBridgeDevice, instanceName string) map[string]bool {
	used := map[string]bool{}
	for entity := range declaredDeviceIDByEntity(hassBridgeDevicesByID, instanceName) {
		used[entity] = true
	}
	return used
}

// declaredDeviceIDByEntity returns, for every bare source entity_id already referenced by a
// declared "home_assistant" bridge device belonging to instanceName, that device's own Physical.def
// id -- the real, human-chosen name Physical.def has already given it. Physical.def is always the
// naming authority for a device (see buildSuggestionReportFromExistence's own doc comment): once
// even one sibling of a coordinator-discovered, synthetically-grouped device
// (house_event_bus_coordinator/entity_existence.go's DiscoverSiblings, "hass.discovered_...") gets
// declared under a real id here, the rest of that same device's still-undeclared siblings should be
// suggested under that real id too, not the coordinator's placeholder.
func declaredDeviceIDByEntity(hassBridgeDevicesByID map[string]THassBridgeDevice, instanceName string) map[string]string {
	declared := map[string]string{}
	for deviceID, device := range hassBridgeDevicesByID {
		if !containsString(device.Instances, instanceName) {
			continue
		}
		for _, cap := range device.Capabilities {
			source, declaredHere := cap.Sources[instanceName]
			if !declaredHere {
				continue
			}
			declared[bareEntityFromSource(source)] = deviceID
		}
		for _, source := range device.DeviceInfoCapabilities {
			declared[bareEntityFromSource(source)] = deviceID
		}
	}
	return declared
}

// buildSuggestionReportFromExistence formats every known-to-exist entity (minus whatever used
// already claims) as copy-paste-ready Physical.def "device ... with: ...; end;" blocks, grouped by
// device id and sorted for deterministic output. A device id here starts out as either a real,
// already-declared Physical.def device id (e.g. "hass.davids_bedroom" -- an entity the generator
// itself seeded from an already-declared device, or a sibling the coordinator discovered via
// device_entities() on one) or a coordinator-minted synthetic one for a device the DSL doesn't know
// about yet (e.g. "hass.discovered_office_garden" -- house_event_bus_coordinator/entity_existence.go's
// syntheticDeviceID, for a "Discover entity"-seeded anchor with no owning device).
//
// Physical.def is always the naming authority, though, not the coordinator: if declaredDeviceID
// shows that even one sibling of a synthetically-grouped device has since been declared under a
// real id (the person copy-pasted part of an earlier "hass.discovered_..." suggestion and gave it a
// real name), the *rest* of that same device's still-undeclared siblings are grouped and suggested
// under that real id too, overriding the coordinator's own placeholder -- so a device only ever
// shows up once, under its real name, split across an already-declared block and a suggestion
// block, never twice under two different names for the same physical device. not-known-to-exist and
// known-not-to-exist entities are never suggested -- only a confirmed existence is copy-paste-worthy.
// Entities with no device at all (device id "") are listed separately, commented out, same as the
// old report's convention.
func buildSuggestionReportFromExistence(status TEntityExistenceStatusPayload, used map[string]bool, declaredDeviceID map[string]string) string {
	deviceIDs := make([]string, 0, len(status))
	for id := range status {
		if id != "" {
			deviceIDs = append(deviceIDs, id)
		}
	}
	sort.Strings(deviceIDs)

	var sb strings.Builder
	for _, deviceID := range deviceIDs {
		displayDeviceID := deviceID
		var realNames []string
		for entityID := range status[deviceID].Entities {
			if real, ok := declaredDeviceID[entityID]; ok {
				realNames = append(realNames, real)
			}
		}
		if len(realNames) > 0 {
			sort.Strings(realNames)
			displayDeviceID = realNames[0]
		}

		entityIDs := make([]string, 0, len(status[deviceID].Entities))
		for entityID, entry := range status[deviceID].Entities {
			if entry.Status != existenceStatusKnownToExist || used[entityID] {
				continue
			}
			entityIDs = append(entityIDs, entityID)
		}
		if len(entityIDs) == 0 {
			continue
		}
		sort.Strings(entityIDs)

		type line struct{ lhs, rhs, comment string }
		var lines []line
		maxLHS := 0
		for _, entityID := range entityIDs {
			domain, suffix := recognizeCapability(entityID)
			lhs := domain + "." + suffix + ":"
			comment := ""
			if suffix == "" {
				comment = " # not recognized"
			}
			if len(lhs) > maxLHS {
				maxLHS = len(lhs)
			}
			lines = append(lines, line{lhs: lhs, rhs: entityID + ";", comment: comment})
		}

		sb.WriteString("device " + displayDeviceID + " with:\n")
		for _, l := range lines {
			sb.WriteString("  " + l.lhs + strings.Repeat(" ", maxLHS-len(l.lhs)+1) + l.rhs + l.comment + "\n")
		}
		sb.WriteString("end;\n\n")
	}

	if orphans, ok := status[""]; ok {
		var standalone []string
		for entityID, entry := range orphans.Entities {
			if entry.Status == existenceStatusKnownToExist && !used[entityID] {
				standalone = append(standalone, entityID)
			}
		}
		if len(standalone) > 0 {
			sort.Strings(standalone)
			sb.WriteString("# entities with no known device grouping:\n")
			for _, entityID := range standalone {
				sb.WriteString("# " + entityID + "\n")
			}
		}
	}

	return sb.String()
}

// generateEntityCatalogueSuggestions fetches each declared instance's own existence status
// (fetchEntityExistence -- fresh over MQTT when possible, this house's own local cache otherwise)
// and writes <outputRoot>/suggestions/home_assistant_<name>.txt for it. Soft-fails (a printed
// warning, not an error) per instance when neither a fresh nor a cached status is available, so an
// unreachable broker/coordinator never breaks an otherwise successful ./generate run. No-op
// entirely when ctx has no MQTT secrets configured or no "home_assistant" instance is declared.
func generateEntityCatalogueSuggestions(definitionDir, outputRoot string, instances map[string]THomeAssistantInstance, hassBridgeDevicesByID map[string]THassBridgeDevice, ctx TPhysicalGenerationContext) error {
	if !ctx.HasMQTTSecrets || len(instances) == 0 {
		return nil
	}

	names := make([]string, 0, len(instances))
	for name := range instances {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		status, err := fetchEntityExistence(definitionDir, ctx, name)
		if err != nil {
			// Leave whatever suggestions file already exists untouched -- a fetch failure (offline,
			// unreachable broker) says nothing about whether there's still something to suggest, so
			// deleting a possibly-still-accurate file here would throw away real information.
			fmt.Printf("[physical] entity existence for %q: %v\n", name, err)
			continue
		}
		declared := declaredDeviceIDByEntity(hassBridgeDevicesByID, name)
		used := usedHassBridgeEntityIDs(hassBridgeDevicesByID, name)
		report := buildSuggestionReportFromExistence(status, used, declared)
		suggestionPath := filepath.Join(outputRoot, "suggestions", "home_assistant_"+name+".txt")
		if strings.TrimSpace(report) == "" {
			// Unlike a fetch failure, this *is* an authoritative "nothing to suggest right now" --
			// remove any stale file from an earlier run rather than silently leaving outdated
			// suggestions in place (confirmed live 2026-08-28: a file from the old, now-removed
			// manifest mechanism stayed on disk, unchanged, looking current when it no longer was).
			if err := os.Remove(suggestionPath); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		if err := writeYAMLFile(suggestionPath, report); err != nil {
			return err
		}
	}
	return nil
}
