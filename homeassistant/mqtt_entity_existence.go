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
	if err, known := brokerKnownUnreachable(secrets); known {
		return nil, fmt.Errorf("broker %s:%s already known unreachable this run: %w", secrets.Server, secrets.Port, err)
	}

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
	// See mqtt_discovery_existence.go's identical call for why: a one-shot fetch client must
	// never keep retrying in the background after this function itself has given up.
	opts.SetAutoReconnect(false)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(fetchExistenceTimeout) || token.Error() != nil {
		var err error
		if tokenErr := token.Error(); tokenErr != nil {
			err = fmt.Errorf("connecting to MQTT broker %s:%s: %w", secrets.Server, secrets.Port, tokenErr)
		} else {
			err = fmt.Errorf("connecting to MQTT broker %s:%s: timed out", secrets.Server, secrets.Port)
		}
		markBrokerUnreachable(secrets, err)
		return nil, err
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
// distinct instance hassBridgeDevicesByID references, and returns every declared capability whose
// source entity the coordinator has confirmed known-not-to-exist. not-known-to-exist (unresolved)
// and known-to-exist both generate optimistically. Soft-fails (a warning, not an error) per
// instance when neither a fresh read nor a cache is available -- an instance the coordinator
// hasn't reported on yet shouldn't block generation any more than "no cache yet" already doesn't
// for the older manifest mechanism.
//
// Returns problems for the caller to fold into a TMissingEntitiesReport (missing_entities_report.go)
// rather than an error -- PROJECT.md item 1a (2026-09-21, "stable ID-based link" architecture): see
// checkDiscoveryKnownNotToExistErrors' own doc comment (mqtt_discovery_existence.go) for the full
// rationale, identical here.
func checkKnownNotToExistErrors(definitionDir string, hassBridgeDevicesByID map[string]THassBridgeDevice, ctx TPhysicalGenerationContext) []string {
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

	if len(problems) > 0 {
		fmt.Printf("[physical] entity existence check: the coordinator has confirmed %d declared capability/capabilities reference an entity that does not exist -- see suggestions/missing.txt\n", len(problems))
	}
	return problems
}

// checkMainEntityKnownNotToExistErrors is checkKnownNotToExistErrors' kind-5 counterpart
// (PROJECT.md item 1, 2026-09-07): mainEntityIDs (collectMainEntityIDs, main_entities.go) has no
// device grouping known ahead of time -- unlike a hassbridge capability's own declared device id,
// a bare Spaces.def entity is just a name -- so this scans every device bucket in instance "main"'s
// own status payload (including the "" no-device bucket) for a matching entry, rather than
// indexing by device id the way checkKnownNotToExistErrors does. Same rule as every other kind:
// not-known-to-exist and known-to-exist both generate optimistically -- see
// checkDiscoveryKnownNotToExistErrors' own doc comment for why confirmed-missing is now reported
// rather than a build-blocking error (PROJECT.md item 1a, 2026-09-21).
func checkMainEntityKnownNotToExistErrors(definitionDir string, mainEntityIDs []string, ctx TPhysicalGenerationContext) []string {
	if len(mainEntityIDs) == 0 {
		return nil
	}
	status, err := fetchEntityExistence(definitionDir, ctx, "main")
	if err != nil {
		fmt.Printf("[physical] entity existence for %q: %v\n", "main", err)
		return nil
	}

	byEntity := map[string]TEntityExistenceStatusEntry{}
	for _, device := range status {
		for entityID, entry := range device.Entities {
			byEntity[entityID] = entry
		}
	}

	var problems []string
	for _, entityID := range mainEntityIDs {
		entry, known := byEntity[entityID]
		if !known || entry.Status != existenceStatusKnownNotToExist {
			continue
		}
		problems = append(problems, fmt.Sprintf("%s: confirmed not to exist on instance \"main\"", entityID))
	}

	if len(problems) > 0 {
		fmt.Printf("[physical] main-instance entity existence check: the coordinator has confirmed %d declared entit(y/ies) do not exist -- see suggestions/missing.txt\n", len(problems))
	}
	return problems
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

// entityLocalName returns entityID's own local name -- everything after the first "." -- or
// entityID itself if it has no domain prefix at all.
func entityLocalName(entityID string) string {
	if idx := strings.Index(entityID, "."); idx >= 0 {
		return entityID[idx+1:]
	}
	return entityID
}

// commonEntityLocalNamePrefix returns the longest common prefix shared by every one of
// entityIDs' own local names (entityLocalName) -- computed over a device's ENTIRE entity set,
// used and not-yet-used alike, so a bare already-declared entity with no attribute suffix at all
// (e.g. "sensor.bathroom_washing_machine", the device's own base name) correctly clamps the
// prefix there, rather than a longer one only the not-yet-declared subset happens to share (e.g.
// "bathroom_washing_machine_wash_", if every undeclared sensor happened to also share "wash_").
// Returns "" for fewer than two entities or no common prefix at all -- callers treat that as "no
// suggestion available", not an error.
func commonEntityLocalNamePrefix(entityIDs []string) string {
	if len(entityIDs) < 2 {
		return ""
	}
	prefix := entityLocalName(entityIDs[0])
	for _, id := range entityIDs[1:] {
		name := entityLocalName(id)
		for !strings.HasPrefix(name, prefix) {
			prefix = prefix[:len(prefix)-1]
			if prefix == "" {
				return ""
			}
		}
	}
	return prefix
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

// ignoredHassBridgeDeviceIDs returns every "home_assistant" bridge device id belonging to
// instanceName that declared "ignore other capabilities;" -- see
// THassBridgeDevice.IgnoreOtherCapabilities' own doc comment.
func ignoredHassBridgeDeviceIDs(hassBridgeDevicesByID map[string]THassBridgeDevice, instanceName string) map[string]bool {
	ignored := map[string]bool{}
	for deviceID, device := range hassBridgeDevicesByID {
		if device.IgnoreOtherCapabilities && containsString(device.Instances, instanceName) {
			ignored[deviceID] = true
		}
	}
	return ignored
}

// ignoredHostDeviceIDs returns every "hosts" device id that declared "ignore other
// capabilities;" -- a "hosts" device's own entities always live on "main" (no per-instance
// targeting the way a hassbridge device's Instances has), so this is unconditional. See
// THostDevice.IgnoreOtherCapabilities' own doc comment.
func ignoredHostDeviceIDs(hostDevicesByID map[string]THostDevice) map[string]bool {
	ignored := map[string]bool{}
	for deviceID, device := range hostDevicesByID {
		if device.IgnoreOtherCapabilities {
			ignored[deviceID] = true
		}
	}
	return ignored
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

// expandUsedAcrossDiscoverySourcedDeviceGroups extends used in place: for every device grouping in
// status that already contains at least one discovery-implied entity (discoveryImpliedEntityIDs --
// an entity the coordinator's own "discovery" integration relay produces, collectDiscoveryImpliedEntityIDs'
// own doc comment), every OTHER entity sharing that SAME device grouping is marked used too, even
// when discoveryImpliedEntityIDs doesn't itself name it.
//
// Real bug found live 2026-09-21 (Vienna): signify_dimmer's own Conceptual.def positions only its
// "event" domain leaf, never the "sensor" domain mirror Zigbee2MQTT ALSO publishes for the same
// shared unique_id (the same shared-unique_id situation bj/Moes have, see MigrationNotes.def) --
// but a pre-migration relay of that sensor-domain leaf left a real (now permanently "unavailable")
// entity sitting in HA's own state machine, grouped by HA's device registry alongside the two
// properly-declared discovery entities (event + battery_level). It kept reappearing as a
// "hass.discovered_..." hassbridge suggestion -- structurally the wrong kind of suggestion even
// when accurate, since a hassbridge cross-post declaration doesn't apply to an entity this same
// instance's own discovery integration already produces; and in this specific case not even
// accurate, since the entity is an orphan nothing produces any more. Either way, once a device is
// known (via even one sibling) to be discovery-sourced, none of its other entities are genuine
// hassbridge candidates.
func expandUsedAcrossDiscoverySourcedDeviceGroups(status TEntityExistenceStatusPayload, discoveryImpliedEntityIDs []string, used map[string]bool) {
	discoveryImplied := make(map[string]bool, len(discoveryImpliedEntityIDs))
	for _, id := range discoveryImpliedEntityIDs {
		discoveryImplied[id] = true
	}
	for _, deviceStatus := range status {
		hasDiscoverySourcedSibling := false
		for entityID := range deviceStatus.Entities {
			if discoveryImplied[entityID] {
				hasDiscoverySourcedSibling = true
				break
			}
		}
		if !hasDiscoverySourcedSibling {
			continue
		}
		for entityID := range deviceStatus.Entities {
			used[entityID] = true
		}
	}
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
// old report's convention -- these are usually not real "hassbridge candidates" at all (a bare
// hassbridge candidate almost always has SOME device grouping) but leftovers from a since-changed
// Conceptual.def/Logical.def declaration (a rename, a leaf-naming fix, a device consolidation --
// e.g. individual Zigbee bulbs folded into a light group): the coordinator's existence check still
// finds them live in HA (nothing ever deletes an orphaned registry entry on its own, see memory:
// "a restart does NOT clean up orphaned registry entities"), but no current declaration produces
// them any more. Confirmed live 2026-09-20 on Vienna: every entity in this bucket was absent from
// every currently-generated YAML file. Flagged as such in the report itself (see the header text
// below) so this bucket reads as "probably safe to delete from HA's entity registry" rather than
// "an unclaimed hassbridge candidate, needs a device: block".
//
// ignoredDeviceIDs (2026-09-21) names every hassbridge/hosts device that declared its own "ignore
// other capabilities;" (THassBridgeDevice/THostDevice's identically-named field) -- checked against
// BOTH deviceID and its resolved displayDeviceID, since either might be the declared id depending
// on whether the coordinator's own existence check already recognizes it under its real name. A
// matching device's entire block is skipped outright, not just its individual entities: the whole
// point of the directive is "I already know what else is here; stop suggesting it," so even an
// entity type not seen before should still be silenced rather than surfacing as a surprise later.
func buildSuggestionReportFromExistence(status TEntityExistenceStatusPayload, used map[string]bool, declaredDeviceID map[string]string, ignoredDeviceIDs map[string]bool) string {
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
		if ignoredDeviceIDs[displayDeviceID] || ignoredDeviceIDs[deviceID] {
			continue
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

		allEntityIDs := make([]string, 0, len(status[deviceID].Entities))
		for entityID := range status[deviceID].Entities {
			allEntityIDs = append(allEntityIDs, entityID)
		}
		commonPrefix := commonEntityLocalNamePrefix(allEntityIDs)

		type line struct{ lhs, rhs, comment string }
		var lines []line
		maxLHS := 0
		for _, entityID := range entityIDs {
			domain, suffix := recognizeCapability(entityID)
			comment := ""
			if suffix == "" {
				comment = " # not recognized"
				// Not recognized by keyword, but a guess is still better than an empty label:
				// strip the prefix every one of this device's own entities (used and unused
				// alike) shares, so e.g. "bathroom_washing_machine_wash_delay_start" suggests
				// "wash_delay_start" rather than leaving the DSL author to fill in a blank.
				if guess := strings.TrimPrefix(strings.TrimPrefix(entityLocalName(entityID), commonPrefix), "_"); guess != "" {
					suffix = guess
				}
			}
			lhs := domain + "." + suffix
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
			sb.WriteString("# entities with no known device grouping -- likely orphaned (no longer produced by\n")
			sb.WriteString("# any current Physical.def/Conceptual.def/Logical.def declaration, e.g. after a rename\n")
			sb.WriteString("# or device consolidation); check history, then probably safe to delete from HA's own\n")
			sb.WriteString("# entity registry rather than positioned:\n")
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
//
// mainEntityIDs (collectMainEntityIDs, main_entities.go, PROJECT.md item 1, 2026-09-07) is folded
// into instance "main"'s own `used` set -- without this, every already-declared bare entity would
// be suggested right back as if unclaimed, since usedHassBridgeEntityIDs only knows about
// hassbridge-declared sources, not kind-5's own flat list.
func generateEntityCatalogueSuggestions(definitionDir, outputRoot string, instances map[string]THomeAssistantInstance, hassBridgeDevicesByID map[string]THassBridgeDevice, hostDevicesByID map[string]THostDevice, mainEntityIDs []string, discoveryImpliedEntityIDs []string, ctx TPhysicalGenerationContext) error {
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
		ignoredDeviceIDs := ignoredHassBridgeDeviceIDs(hassBridgeDevicesByID, name)
		// Real bug found live 2026-09-21: this fold used to run only for name == "main" (matching
		// mainEntityIDs/discoveryImpliedEntityIDs, which genuinely ARE main-only concepts), on the
		// assumption a "hosts" device's own entities only ever appear on this house's own "main"
		// instance -- but the coordinator's own existence check grouped node.fritz_box's
		// undeclared "image" entity under the SAME real device id on the "protocols-server-2"
		// instance's own report too (confirmed live: suggestions/home_assistant_protocols-
		// server-2.txt), so an "ignore other capabilities;" declared on a hosts device must apply
		// to every instance's report, not just main's.
		for id := range ignoredHostDeviceIDs(hostDevicesByID) {
			ignoredDeviceIDs[id] = true
		}
		if name == "main" {
			for _, id := range mainEntityIDs {
				used[id] = true
			}
			// Real bug found live 2026-09-20: every discovery-declared device's own entities kept
			// reappearing here as "hass.discovered_..." suggestions -- they're already fully
			// claimed by their own Physical.def/Conceptual.def declaration, just via a different
			// mechanism (coordinator MQTT relay) than a hassbridge cross-post. See
			// collectDiscoveryImpliedEntityIDs' own doc comment.
			for _, id := range discoveryImpliedEntityIDs {
				used[id] = true
			}
			// Real bug found live 2026-09-21: signify_dimmer's own Conceptual.def only positions
			// its "event" domain leaf, never the "sensor" domain mirror Zigbee2MQTT ALSO publishes
			// for the same shared unique_id (the same shared-unique_id situation as bj/Moes, see
			// MigrationNotes.def) -- but the OLD, pre-migration relay for that sensor-domain leaf
			// left a real (if now-orphaned, "unavailable" forever) entity sitting in HA's own state
			// machine, grouped by HA's device registry under the SAME device as the two properly-
			// declared discovery entities (event + battery_level). Once ANY sibling under a
			// suggested device is known discovery-implied, every OTHER entity sharing that same
			// device grouping is either an already-covered discovery leaf or an orphaned one --
			// never a genuine hassbridge candidate (a hassbridge cross-post declaration doesn't
			// even apply to an entity this same instance's own discovery integration already
			// produced). See expandUsedAcrossDiscoverySourcedDeviceGroups' own doc comment.
			expandUsedAcrossDiscoverySourcedDeviceGroups(status, discoveryImpliedEntityIDs, used)
		}
		report := buildSuggestionReportFromExistence(status, used, declared, ignoredDeviceIDs)
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
