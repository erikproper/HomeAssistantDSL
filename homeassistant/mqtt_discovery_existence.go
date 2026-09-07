/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: MQTTDiscoveryExistence
 *
 * Kind-2 (discovery) half of PROJECT.md 1.8 -- reads the coordinator's own three-state existence
 * status for "discovery" integration gateways (house_event_bus_coordinator/discovery_existence.go
 * publishes it to "discovery_gateways/<gatewayID>/existence/state", retained, on both the local and
 * cloud broker unconditionally when a cloud one is configured), and turns a confirmed
 * known-not-to-exist verdict into a generate-time error -- exactly mirroring
 * mqtt_entity_existence.go's checkKnownNotToExistErrors for kind-3, just against
 * TDiscoveryEntityLink.Leaf (a capability line's right-hand side) instead of a hassbridge
 * capability's SourceEntity.
 *
 * Unlike kind-3, this status is populated *passively* by the coordinator (a gateway self-announces
 * via native HA MQTT discovery; no active inquiry exists or is needed for kind-2) -- so a leaf can
 * only ever be not-known-to-exist or known-to-exist today; known-not-to-exist (retraction) is a
 * coordinator-side gap not yet built (see discovery_existence.go's own doc comment). This file's own
 * check is still written expecting all three states, so it needs no change once retraction lands.
 *
 * Persisted local cache (Definitions/.cache/), same fetch-when-online/fall-back-to-cache pattern
 * mqtt_entity_existence.go already established for kind-3.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 29.08.2026
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

// fetchDiscoveryExistenceTimeout mirrors fetchExistenceTimeout's own reasoning exactly (mqtt_entity_existence.go).
const fetchDiscoveryExistenceTimeout = 5 * time.Second

// TDiscoveryExistenceStatusPayload is the top-level shape published to
// "discovery_gateways/<gatewayID>/existence/state" -- keyed by leaf (TDiscoveryEntityLink.Leaf),
// no device grouping (discoverybridge.go's relay is flat, leaf-keyed, unlike kind-3's device-linked
// hassbridge capabilities).
type TDiscoveryExistenceStatusPayload map[string]string

func discoveryGatewayExistenceStatusTopic(gatewayID string) string {
	return "discovery_gateways/" + gatewayID + "/existence/state"
}

// discoveryExistenceCachePath is gatewayID's own local, persisted cache file -- same convention
// existenceCachePath already established for kind-3.
func discoveryExistenceCachePath(definitionDir, gatewayID string) string {
	return filepath.Join(definitionDir, ".cache", "discovery_existence_"+sanitizeCacheFileSegment(gatewayID)+".json")
}

// sanitizeCacheFileSegment replaces "." with "_" in gatewayID (e.g. "discovery.ems_esp") so the
// cache filename never carries a stray extension-looking dot.
func sanitizeCacheFileSegment(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '.' {
			out = append(out, '_')
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

// fetchDiscoveryExistence tries a fresh read, cloud broker first then local
// (fetchDiscoveryExistencePreferringCloud), and on success updates gatewayID's own local cache
// file. On any failure, falls back to that cache's last-known contents. Returns an error only when
// neither live broker nor a usable cache is available.
func fetchDiscoveryExistence(definitionDir string, ctx TPhysicalGenerationContext, gatewayID string) (TDiscoveryExistenceStatusPayload, error) {
	cachePath := discoveryExistenceCachePath(definitionDir, gatewayID)

	status, fetchErr := fetchDiscoveryExistencePreferringCloud(ctx, gatewayID)
	if fetchErr == nil {
		if data, err := json.Marshal(status); err == nil {
			if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err == nil {
				if err := os.WriteFile(cachePath, data, 0o644); err != nil {
					fmt.Printf("[physical] discovery existence for %q: fetched fresh but could not update local cache %s: %v\n", gatewayID, cachePath, err)
				}
			}
		}
		return status, nil
	}

	cached, cacheErr := os.ReadFile(cachePath)
	if cacheErr != nil {
		return nil, fmt.Errorf("%w (and no local cache at %s to fall back to)", fetchErr, cachePath)
	}
	var cachedStatus TDiscoveryExistenceStatusPayload
	if err := json.Unmarshal(cached, &cachedStatus); err != nil {
		return nil, fmt.Errorf("%w (local cache at %s is also unreadable: %v)", fetchErr, cachePath, err)
	}
	fmt.Printf("[physical] discovery existence for %q: live fetch failed (%v), using cached copy from %s\n", gatewayID, fetchErr, cachePath)
	return cachedStatus, nil
}

// fetchDiscoveryExistencePreferringCloud tries the "coordinator_only" cloud broker first (when
// declared), falling back to the local broker -- matches fetchEntityExistencePreferringCloud's own
// policy exactly.
func fetchDiscoveryExistencePreferringCloud(ctx TPhysicalGenerationContext, gatewayID string) (TDiscoveryExistenceStatusPayload, error) {
	if cloudSecrets, ok := coordinatorOnlyBrokerSecrets(ctx); ok {
		if status, err := fetchDiscoveryExistenceFromBroker(cloudSecrets, gatewayID, ctx.Installation); err == nil {
			return status, nil
		} else {
			fmt.Printf("[physical] discovery existence for %q: cloud broker attempt failed (%v), trying local\n", gatewayID, err)
		}
	}
	return fetchDiscoveryExistenceFromBroker(ctx.MQTTSecrets, gatewayID, "")
}

// fetchDiscoveryExistenceFromBroker connects to secrets' broker as a one-shot client and returns
// whatever the coordinator has last published (retained) on discoveryGatewayExistenceStatusTopic(gatewayID).
//
// ownInstallation, when non-empty, additionally prefixes the topic with this house's own
// installation name -- the cloud broker's own qualification convention
// (house_event_bus_coordinator/discovery_existence.go's publishStatus, fixed live 2026-09-05
// alongside kind-3's analogous "main" instance collision). Pass "" for the local broker, whose copy
// stays bare.
func fetchDiscoveryExistenceFromBroker(secrets TMQTTBrokerSecrets, gatewayID, ownInstallation string) (TDiscoveryExistenceStatusPayload, error) {
	topic := discoveryGatewayExistenceStatusTopic(gatewayID)
	if ownInstallation != "" {
		topic = ownInstallation + "/" + topic
	}

	opts := mqtt.NewClientOptions()
	scheme := "tcp"
	if secrets.TLS {
		scheme = "ssl"
	}
	opts.AddBroker(fmt.Sprintf("%s://%s:%s", scheme, secrets.Server, secrets.Port))
	opts.SetClientID("homeassistant-generator-discovery-existence-" + gatewayID)
	opts.SetUsername(secrets.Login)
	opts.SetPassword(secrets.Password)
	opts.SetConnectTimeout(fetchDiscoveryExistenceTimeout)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(fetchDiscoveryExistenceTimeout) || token.Error() != nil {
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
	if !subToken.WaitTimeout(fetchDiscoveryExistenceTimeout) || subToken.Error() != nil {
		if err := subToken.Error(); err != nil {
			return nil, fmt.Errorf("subscribing to %s: %w", topic, err)
		}
		return nil, fmt.Errorf("subscribing to %s: timed out", topic)
	}

	select {
	case payload := <-received:
		var status TDiscoveryExistenceStatusPayload
		if err := json.Unmarshal(payload, &status); err != nil {
			return nil, fmt.Errorf("parsing %s payload: %w", topic, err)
		}
		return status, nil
	case <-time.After(fetchDiscoveryExistenceTimeout):
		return nil, fmt.Errorf("timed out waiting for %s (has the coordinator published discovery existence status for this gateway yet?)", topic)
	}
}

// checkDiscoveryKnownNotToExistErrors fetches (fetch-then-cache-fallback) existence status for
// every distinct gateway discoveryEntityLinks references, and returns a combined error listing
// every declared entity link whose source leaf the coordinator has confirmed known-not-to-exist --
// the same rule as kind-3's checkKnownNotToExistErrors: this is the *only* status that blocks
// generation; not-known-to-exist and known-to-exist both generate optimistically. Soft-fails (a
// warning) per gateway when neither a fresh read nor a cache is available.
func checkDiscoveryKnownNotToExistErrors(definitionDir string, discoveryEntityLinks map[string]TDiscoveryEntityLink, ctx TPhysicalGenerationContext) error {
	gatewayIDs := map[string]bool{}
	for _, link := range discoveryEntityLinks {
		if link.GatewayDeviceID != "" {
			gatewayIDs[link.GatewayDeviceID] = true
		}
	}
	if len(gatewayIDs) == 0 {
		return nil
	}

	statusByGateway := map[string]TDiscoveryExistenceStatusPayload{}
	for gatewayID := range gatewayIDs {
		status, err := fetchDiscoveryExistence(definitionDir, ctx, gatewayID)
		if err != nil {
			fmt.Printf("[physical] discovery existence for %q: %v\n", gatewayID, err)
			continue
		}
		statusByGateway[gatewayID] = status
	}

	entityIDs := make([]string, 0, len(discoveryEntityLinks))
	for id := range discoveryEntityLinks {
		entityIDs = append(entityIDs, id)
	}
	sort.Strings(entityIDs)

	var problems []string
	for _, entityID := range entityIDs {
		link := discoveryEntityLinks[entityID]
		status, known := statusByGateway[link.GatewayDeviceID]
		if !known {
			continue
		}
		if status[link.Leaf] != existenceStatusKnownNotToExist {
			continue
		}
		problems = append(problems, fmt.Sprintf("%s (gateway %q): source leaf %q confirmed not to exist",
			entityID, link.GatewayDeviceID, link.Leaf))
	}

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("discovery existence check failed -- the coordinator has confirmed %d declared entity/entities reference a leaf that does not exist:\n  %s",
		len(problems), strings.Join(problems, "\n  "))
}

// usedDiscoveryLeaves returns the set of leaf names already claimed by a declared
// TDiscoveryEntityLink for gatewayID -- what buildDiscoverySuggestionReport excludes from its
// output, mirroring usedHassBridgeEntityIDs for kind-3.
func usedDiscoveryLeaves(discoveryEntityLinks map[string]TDiscoveryEntityLink, gatewayID string) map[string]bool {
	used := map[string]bool{}
	for _, link := range discoveryEntityLinks {
		if link.GatewayDeviceID == gatewayID {
			used[link.Leaf] = true
		}
	}
	return used
}

// recognizeDiscoveryCapability guesses a bare gateway leaf's local capability domain/suffix from
// the same recognizedCapabilityKeywords table kind-3 suggestions use (mqtt_entity_existence.go) --
// but falls back to "sensor" when nothing matches, never the bare leaf itself: unlike a real
// entity_id, a gateway leaf (e.g. "boiler_outdoortemp") has no domain prefix of its own to fall
// back to, and most EMS-ESP-style leaf names won't hit any of that table's room/measurement-shaped
// keywords at all -- "sensor" is simply the most common real domain for this integration kind's
// leaves, not a confident guess.
func recognizeDiscoveryCapability(leaf string) (domain, suffix string) {
	for _, k := range recognizedCapabilityKeywords {
		if strings.Contains(leaf, k.keyword) {
			return k.domain, k.suffix
		}
	}
	return "sensor", ""
}

// buildDiscoverySuggestionReport formats every known-to-exist leaf (minus whatever
// discoveryEntityLinks already claims) as copy-paste-ready Physical.def
// "device discovery.<id> with: ...; end;" blocks, one per gateway, sorted for deterministic
// output -- the kind-2 counterpart to buildSuggestionReportFromExistence. Unlike kind-3's
// per-instance/per-device two-level grouping, a gateway id here already *is* the device (no
// separate "device" concept above it in the kind-2 status payload), so this is a single flat pass
// per gateway. not-known-to-exist and known-not-to-exist leaves are never suggested -- only a
// confirmed existence is copy-paste-worthy.
func buildDiscoverySuggestionReport(statusByGateway map[string]TDiscoveryExistenceStatusPayload, discoveryEntityLinks map[string]TDiscoveryEntityLink) string {
	gatewayIDs := make([]string, 0, len(statusByGateway))
	for id := range statusByGateway {
		gatewayIDs = append(gatewayIDs, id)
	}
	sort.Strings(gatewayIDs)

	var sb strings.Builder
	for _, gatewayID := range gatewayIDs {
		used := usedDiscoveryLeaves(discoveryEntityLinks, gatewayID)
		leaves := make([]string, 0, len(statusByGateway[gatewayID]))
		for leaf, status := range statusByGateway[gatewayID] {
			if status != existenceStatusKnownToExist || used[leaf] {
				continue
			}
			leaves = append(leaves, leaf)
		}
		if len(leaves) == 0 {
			continue
		}
		sort.Strings(leaves)

		type line struct{ lhs, rhs, comment string }
		var lines []line
		maxLHS := 0
		for _, leaf := range leaves {
			domain, suffix := recognizeDiscoveryCapability(leaf)
			lhs := domain + "." + suffix + ":"
			comment := ""
			if suffix == "" {
				comment = " # not recognized"
			}
			if len(lhs) > maxLHS {
				maxLHS = len(lhs)
			}
			lines = append(lines, line{lhs: lhs, rhs: domain + "." + leaf + ";", comment: comment})
		}

		sb.WriteString("device " + gatewayID + " with:\n")
		for _, l := range lines {
			sb.WriteString("  " + l.lhs + strings.Repeat(" ", maxLHS-len(l.lhs)+1) + l.rhs + l.comment + "\n")
		}
		sb.WriteString("end;\n\n")
	}
	return sb.String()
}

// generateDiscoverySuggestions fetches each declared "discovery" gateway's own existence status
// (fetchDiscoveryExistence -- fresh over MQTT when possible, local cache otherwise) and writes
// <outputRoot>/suggestions/discovery.txt for the whole discovery integration -- the kind-2
// counterpart to generateEntityCatalogueSuggestions. One combined file, not one per gateway:
// unlike kind-3's per-instance grouping, there's no grouping above "gateway" for kind-2 to key
// separate files on, and every gateway is small enough that one file stays readable. Soft-fails (a
// printed warning, not an error) per gateway when neither a fresh nor a cached status is
// available; if literally every gateway soft-fails, any existing suggestions file is left
// untouched (a fetch failure says nothing about whether it's still accurate). No-op entirely when
// ctx has no MQTT secrets configured or no "discovery" gateways are declared.
func generateDiscoverySuggestions(definitionDir, outputRoot string, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, discoveryEntityLinks map[string]TDiscoveryEntityLink, ctx TPhysicalGenerationContext) error {
	if !ctx.HasMQTTSecrets || len(discoveryGatewaysByID) == 0 {
		return nil
	}

	gatewayIDs := make([]string, 0, len(discoveryGatewaysByID))
	for id := range discoveryGatewaysByID {
		gatewayIDs = append(gatewayIDs, id)
	}
	sort.Strings(gatewayIDs)

	statusByGateway := map[string]TDiscoveryExistenceStatusPayload{}
	fetchedAny := false
	for _, gatewayID := range gatewayIDs {
		status, err := fetchDiscoveryExistence(definitionDir, ctx, gatewayID)
		if err != nil {
			fmt.Printf("[physical] discovery existence for %q: %v\n", gatewayID, err)
			continue
		}
		fetchedAny = true
		statusByGateway[gatewayID] = status
	}
	if !fetchedAny {
		return nil
	}

	suggestionPath := filepath.Join(outputRoot, "suggestions", "discovery.txt")
	report := buildDiscoverySuggestionReport(statusByGateway, discoveryEntityLinks)
	if strings.TrimSpace(report) == "" {
		// Unlike a fetch failure, this *is* an authoritative "nothing to suggest right now" --
		// remove any stale file from an earlier run rather than silently leaving outdated
		// suggestions in place (mirrors generateEntityCatalogueSuggestions' own fix for exactly
		// this, 2026-08-28).
		if err := os.Remove(suggestionPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return writeYAMLFile(suggestionPath, report)
}
