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

// TDiscoveryExistenceStatusPayload is one gateway's own leaf->status slice of the aggregate
// existence payload -- keyed by leaf (TDiscoveryEntityLink.Leaf), no device grouping
// (discoverybridge.go's relay is flat, leaf-keyed, unlike kind-3's device-linked hassbridge
// capabilities). Kept as its own named type since both consumers (checkDiscoveryKnownNotToExistErrors,
// buildDiscoverySuggestionReport) already work in terms of "one gateway's own status map".
type TDiscoveryExistenceStatusPayload map[string]string

// TDiscoveryExistenceAggregatePayload is the top-level shape published to
// discoveryExistenceAggregateStatusTopic() -- every declared gateway's own status, together, in one
// retained message. See house_event_bus_coordinator/discovery_existence.go's own
// discoveryExistenceAggregateStatusTopic doc comment (2026-09-20) for why this replaced the earlier
// one-topic-per-gateway design: at Vienna's own scale (70+ gateways), that meant 70+ separate MQTT
// round trips on every single ./generate run, most of them paying a full connect-timeout for a
// gateway the coordinator hadn't reported on yet (e.g. right after a device-id rename). This
// aggregate is fetched exactly ONCE per generate run regardless of how many gateways exist.
type TDiscoveryExistenceAggregatePayload map[string]TDiscoveryExistenceStatusPayload

// discoveryExistenceAggregateStatusTopic mirrors house_event_bus_coordinator/discovery_existence.go's
// own copy of this same literal string exactly (the two packages never share Go types).
func discoveryExistenceAggregateStatusTopic() string {
	return "discovery_existence/state"
}

// discoveryExistenceAggregateCachePath is the ONE local, persisted cache file for the whole
// aggregate payload -- replaces the earlier per-gateway cache files (discoveryExistenceCachePath)
// now that there's only ever one fetch, not one per gateway.
func discoveryExistenceAggregateCachePath(definitionDir string) string {
	return filepath.Join(definitionDir, ".cache", "discovery_existence.json")
}

// sanitizeCacheFileSegment replaces "." with "_" in an id (e.g. "discovery.ems_esp") so a cache
// filename built from it never carries a stray extension-looking dot. Shared with
// mqtt_import_existence.go's own per-installation cache path.
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

// tDiscoveryExistenceFetchResult is fetchDiscoveryExistenceAggregate's own memoized outcome -- see
// TPhysicalGenerationContext.DiscoveryExistenceAggregate's own doc comment for why this is a
// pointer to a struct with an explicit "fetched" flag, rather than a nil check on the pointer
// itself: a genuinely successful fetch can return an empty (but non-nil-meaningful) result, which a
// bare nil-check on the RESULT couldn't distinguish from "not fetched yet".
type tDiscoveryExistenceFetchResult struct {
	fetched bool
	status  TDiscoveryExistenceAggregatePayload
	err     error
}

// fetchDiscoveryExistenceAggregate tries a fresh read, cloud broker first then local
// (fetchDiscoveryExistenceAggregatePreferringCloud), and on success updates the local cache file.
// On any failure, falls back to that cache's last-known contents. Returns an error only when
// neither live broker nor a usable cache is available. Called exactly ONCE per generate run
// (memoized in ctx.DiscoveryExistenceAggregate, see that field's own doc comment) regardless of how
// many gateways/callers need existence data -- see TDiscoveryExistenceAggregatePayload's own doc
// comment for the real incident (2026-09-20) this replaces the earlier per-gateway design for.
func fetchDiscoveryExistenceAggregate(definitionDir string, ctx TPhysicalGenerationContext) (TDiscoveryExistenceAggregatePayload, error) {
	if ctx.DiscoveryExistenceAggregate != nil && ctx.DiscoveryExistenceAggregate.fetched {
		return ctx.DiscoveryExistenceAggregate.status, ctx.DiscoveryExistenceAggregate.err
	}

	cachePath := discoveryExistenceAggregateCachePath(definitionDir)
	status, fetchErr := fetchDiscoveryExistenceAggregatePreferringCloud(ctx)
	if fetchErr == nil {
		if data, err := json.Marshal(status); err == nil {
			if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err == nil {
				if err := os.WriteFile(cachePath, data, 0o644); err != nil {
					fmt.Printf("[physical] discovery existence: fetched fresh but could not update local cache %s: %v\n", cachePath, err)
				}
			}
		}
		if ctx.DiscoveryExistenceAggregate != nil {
			*ctx.DiscoveryExistenceAggregate = tDiscoveryExistenceFetchResult{fetched: true, status: status}
		}
		return status, nil
	}

	cached, cacheErr := os.ReadFile(cachePath)
	if cacheErr != nil {
		err := fmt.Errorf("%w (and no local cache at %s to fall back to)", fetchErr, cachePath)
		if ctx.DiscoveryExistenceAggregate != nil {
			*ctx.DiscoveryExistenceAggregate = tDiscoveryExistenceFetchResult{fetched: true, err: err}
		}
		return nil, err
	}
	var cachedStatus TDiscoveryExistenceAggregatePayload
	if err := json.Unmarshal(cached, &cachedStatus); err != nil {
		err = fmt.Errorf("%w (local cache at %s is also unreadable: %v)", fetchErr, cachePath, err)
		if ctx.DiscoveryExistenceAggregate != nil {
			*ctx.DiscoveryExistenceAggregate = tDiscoveryExistenceFetchResult{fetched: true, err: err}
		}
		return nil, err
	}
	printOfflineCacheNoticeOnce()
	if ctx.DiscoveryExistenceAggregate != nil {
		*ctx.DiscoveryExistenceAggregate = tDiscoveryExistenceFetchResult{fetched: true, status: cachedStatus}
	}
	return cachedStatus, nil
}

// fetchDiscoveryExistenceAggregatePreferringCloud tries the "coordinator_only" cloud broker first
// (when declared), falling back to the local broker -- matches every other fetchXPreferringCloud
// function's own policy exactly.
func fetchDiscoveryExistenceAggregatePreferringCloud(ctx TPhysicalGenerationContext) (TDiscoveryExistenceAggregatePayload, error) {
	// Cloud failure here is silent -- see printOfflineCacheNoticeOnce's own doc comment.
	if cloudSecrets, ok := coordinatorOnlyBrokerSecrets(ctx); ok {
		if status, err := fetchDiscoveryExistenceAggregateFromBroker(cloudSecrets, ctx.Installation); err == nil {
			return status, nil
		}
	}
	return fetchDiscoveryExistenceAggregateFromBroker(ctx.MQTTSecrets, "")
}

// fetchDiscoveryExistenceAggregateFromBroker connects to secrets' broker as a one-shot client and
// returns whatever the coordinator has last published (retained) on
// discoveryExistenceAggregateStatusTopic().
//
// ownInstallation, when non-empty, additionally prefixes the topic with this house's own
// installation name -- the cloud broker's own qualification convention, same reasoning as every
// other aggregate/per-instance status topic in this codebase. Pass "" for the local broker, whose
// copy stays bare.
func fetchDiscoveryExistenceAggregateFromBroker(secrets TMQTTBrokerSecrets, ownInstallation string) (TDiscoveryExistenceAggregatePayload, error) {
	if err, known := brokerKnownUnreachable(secrets); known {
		return nil, fmt.Errorf("broker %s:%s already known unreachable this run: %w", secrets.Server, secrets.Port, err)
	}

	topic := discoveryExistenceAggregateStatusTopic()
	if ownInstallation != "" {
		topic = ownInstallation + "/" + topic
	}

	opts := mqtt.NewClientOptions()
	scheme := "tcp"
	if secrets.TLS {
		scheme = "ssl"
	}
	opts.AddBroker(fmt.Sprintf("%s://%s:%s", scheme, secrets.Server, secrets.Port))
	opts.SetClientID("homeassistant-generator-discovery-existence")
	opts.SetUsername(secrets.Login)
	opts.SetPassword(secrets.Password)
	opts.SetConnectTimeout(fetchDiscoveryExistenceTimeout)
	// A one-shot fetch-and-disconnect client should never keep retrying in the background --
	// paho's own AutoReconnect defaults to true, so without this a failed connection attempt
	// leaves an orphaned reconnect loop running for the rest of the process, even after this
	// function has already given up and returned an error. See mqtt_broker_reachability.go's own
	// header comment for the real incident (2026-09-20) this compounds.
	opts.SetAutoReconnect(false)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(fetchDiscoveryExistenceTimeout) || token.Error() != nil {
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
	if !subToken.WaitTimeout(fetchDiscoveryExistenceTimeout) || subToken.Error() != nil {
		if err := subToken.Error(); err != nil {
			return nil, fmt.Errorf("subscribing to %s: %w", topic, err)
		}
		return nil, fmt.Errorf("subscribing to %s: timed out", topic)
	}

	select {
	case payload := <-received:
		var status TDiscoveryExistenceAggregatePayload
		if err := json.Unmarshal(payload, &status); err != nil {
			return nil, fmt.Errorf("parsing %s payload: %w", topic, err)
		}
		return status, nil
	case <-time.After(fetchDiscoveryExistenceTimeout):
		return nil, fmt.Errorf("timed out waiting for %s (has the coordinator published discovery existence status yet?)", topic)
	}
}

// checkDiscoveryKnownNotToExistErrors fetches (fetch-then-cache-fallback) the aggregate existence
// status once and slices out every distinct gateway discoveryEntityLinks references, returning
// every declared entity link whose source leaf the coordinator has confirmed known-not-to-exist --
// the same rule as kind-3's checkKnownNotToExistErrors. not-known-to-exist and known-to-exist both
// generate optimistically. Soft-fails (a warning) when neither a fresh read nor a cache is
// available at all.
//
// Returns problems for the caller to fold into a TMissingEntitiesReport (missing_entities_report.go)
// rather than an error -- PROJECT.md item 1a (2026-09-21, "stable ID-based link" architecture):
// previously this returned a hard error that aborted the ENTIRE `./generate` run the moment any one
// declared entity's physical source was confirmed gone (a dead Zigbee battery, a rebooting router),
// which is the opposite of the agreed principle that a vanished physical source must be REPORTED,
// not acted on. See memory: project_stable_discovery_identity_architecture.md.
func checkDiscoveryKnownNotToExistErrors(definitionDir string, discoveryEntityLinks map[string]TDiscoveryEntityLink, ctx TPhysicalGenerationContext) []string {
	gatewayIDs := map[string]bool{}
	for _, link := range discoveryEntityLinks {
		if link.GatewayDeviceID != "" {
			gatewayIDs[link.GatewayDeviceID] = true
		}
	}
	if len(gatewayIDs) == 0 {
		return nil
	}

	// A fetch failure is already printed once by fetchDiscoveryExistenceAggregate itself (its own
	// memoization wrapper) -- not repeated here.
	aggregate, err := fetchDiscoveryExistenceAggregate(definitionDir, ctx)
	statusByGateway := map[string]TDiscoveryExistenceStatusPayload{}
	if err == nil {
		for gatewayID := range gatewayIDs {
			if status, ok := aggregate[gatewayID]; ok {
				statusByGateway[gatewayID] = status
			}
		}
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

	if len(problems) > 0 {
		fmt.Printf("[physical] discovery existence check: the coordinator has confirmed %d declared entity/entities reference a leaf that does not exist -- see suggestions/missing.txt\n", len(problems))
	}
	return problems
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

// declaredDiscoveryLeaves returns the set of raw leaf ids gateway already names on ANY of its own
// "<domain>.<label>: <leaf>;" capability lines -- independent of whether that capability has gone
// on to be positioned at the conceptual layer (usedDiscoveryLeaves' own, narrower criterion).
// buildDiscoverySuggestionReport treats this the same as "used": once a leaf has a real,
// hand-chosen Physical.def label, there is nothing left for this report to suggest for it, even
// when that capability is a diagnostic one deliberately left unpositioned (the desktop/nespresso
// precedent, MigrationNotes.def) -- before this, such a leaf kept reappearing here forever, always
// under the SAME "sensor. <leaf>; # not recognized" guess (recognizeDiscoveryCapability has no
// knowledge of Physical.def's own declarations, only a generic, Z2M-unaware keyword table), even
// immediately after being correctly labelled by hand -- confusing enough live (2026-09-17) to read
// as "the label was never actually applied", when it had been.
func declaredDiscoveryLeaves(gateway TDiscoveryGatewayDevice) map[string]bool {
	declared := map[string]bool{}
	for _, capability := range gateway.Capabilities {
		if capability.Leaf != "" {
			declared[capability.Leaf] = true
		}
	}
	return declared
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
// discoveryEntityLinks already claims, and minus whatever the gateway's own Physical.def
// declaration already names under some capability -- declaredDiscoveryLeaves, positioned or not)
// as copy-paste-ready Physical.def "device discovery.<id> with: ...; end;" blocks, one per
// gateway, sorted for deterministic output -- the kind-2 counterpart to
// buildSuggestionReportFromExistence. Unlike kind-3's per-instance/per-device two-level grouping,
// a gateway id here already *is* the device (no separate "device" concept above it in the kind-2
// status payload), so this is a single flat pass per gateway. not-known-to-exist and
// known-not-to-exist leaves are never suggested -- only a confirmed existence is copy-paste-worthy.
//
// Real bug found live 2026-09-16: each line's own RHS used to be "domain + \".\" + leaf" (e.g.
// "sensor.0x00158d0003f0d585_linkquality_zigbee2mqtt;") -- but a real Physical.def capability
// line's own RHS is always the BARE leaf value, never domain-prefixed (compare any actually
// declared line, e.g. "switch.core: 0xc4988600000fbf8e_switch_zigbee2mqtt;"). Copy-pasting a
// suggestion verbatim produced a syntactically broken line. Fixed to just "leaf + \";\"".
//
// Real bug found live 2026-09-17: a leaf given a real, hand-chosen label (e.g.
// "sensor.color_options: 0x..._color_options_zigbee2mqtt;") but deliberately left unpositioned
// (a diagnostic-only capability, the desktop/nespresso precedent) kept reappearing here forever --
// declaredDiscoveryLeaves closes that gap by excluding any leaf ALREADY named by the gateway's own
// declaration, not just ones gone on to be positioned.
func buildDiscoverySuggestionReport(statusByGateway map[string]TDiscoveryExistenceStatusPayload, discoveryEntityLinks map[string]TDiscoveryEntityLink, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice) string {
	gatewayIDs := make([]string, 0, len(statusByGateway))
	for id := range statusByGateway {
		gatewayIDs = append(gatewayIDs, id)
	}
	sort.Strings(gatewayIDs)

	var sb strings.Builder
	for _, gatewayID := range gatewayIDs {
		used := usedDiscoveryLeaves(discoveryEntityLinks, gatewayID)
		declared := declaredDiscoveryLeaves(discoveryGatewaysByID[gatewayID])
		leaves := make([]string, 0, len(statusByGateway[gatewayID]))
		for leaf, status := range statusByGateway[gatewayID] {
			if status != existenceStatusKnownToExist || used[leaf] || declared[leaf] {
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
			lhs := domain + "." + suffix
			comment := ""
			if suffix == "" {
				comment = " # not recognized"
			}
			if len(lhs) > maxLHS {
				maxLHS = len(lhs)
			}
			lines = append(lines, line{lhs: lhs, rhs: leaf + ";", comment: comment})
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
// (fetchDiscoveryExistence -- fresh over MQTT when possible, local cache otherwise) plus, since
// PROJECT.md item 7/2026-09-14, every still-undeclared device passthrough is relaying raw
// (fetchPassthroughDevices, discovery_passthrough_suggestions.go), and writes
// <outputRoot>/suggestions/discovery.txt for the whole discovery integration -- the kind-2
// counterpart to generateEntityCatalogueSuggestions. One combined file, not one per gateway:
// unlike kind-3's per-instance grouping, there's no grouping above "gateway" for kind-2 to key
// separate files on, and every gateway is small enough that one file stays readable. Soft-fails (a
// printed warning, not an error) per gateway/passthrough when neither a fresh nor a cached status
// is available; if literally nothing could be fetched, any existing suggestions file is left
// untouched (a fetch failure says nothing about whether it's still accurate). passthroughRules is
// only used to decide whether it's worth attempting the passthrough fetch at all -- an empty
// declaration list with zero declared gateways too means there's nothing this integration could
// ever suggest, so the whole function no-ops without touching MQTT. No-op entirely when ctx has no
// MQTT secrets configured.
func generateDiscoverySuggestions(definitionDir, outputRoot string, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, discoveryEntityLinks map[string]TDiscoveryEntityLink, passthroughRules []TDiscoveryPassthroughRule, ctx TPhysicalGenerationContext) error {
	if !ctx.HasMQTTSecrets || (len(discoveryGatewaysByID) == 0 && len(passthroughRules) == 0) {
		return nil
	}

	statusByGateway := map[string]TDiscoveryExistenceStatusPayload{}
	fetchedAny := false
	// A fetch failure is already printed once by fetchDiscoveryExistenceAggregate itself (its own
	// memoization wrapper) -- not repeated here.
	if aggregate, err := fetchDiscoveryExistenceAggregate(definitionDir, ctx); err == nil {
		fetchedAny = true
		for gatewayID := range discoveryGatewaysByID {
			if status, ok := aggregate[gatewayID]; ok {
				statusByGateway[gatewayID] = status
			}
		}
	}

	report := buildDiscoverySuggestionReport(statusByGateway, discoveryEntityLinks, discoveryGatewaysByID)

	// Real bug found live 2026-09-19: this used to be gated on "len(passthroughRules) > 0",
	// coupling the "which real devices exist under ${mqtt_discovery_physical} that Physical.def
	// hasn't declared yet" suggestion feed to whether a "discovery_passthrough ...;" rule happens
	// to be declared at all. The coordinator's own tracker (discoverybridge.go's
	// passthroughDeviceTracker, fixed the same day) now records ANY undeclared device seen under
	// the physical prefix unconditionally -- whether or not it's also being actively relayed into
	// HA -- so the generator must always attempt this fetch too, not just when passthrough rules
	// exist. By the time this line is reached, the function's own top-of-function guard already
	// guarantees at least one of discoveryGatewaysByID/passthroughRules is non-empty, i.e.
	// ${mqtt_discovery_physical} is genuinely in use -- always worth asking.
	{
		status, err := fetchPassthroughDevices(definitionDir, ctx)
		if err != nil {
			fmt.Printf("[physical] passthrough devices: %v\n", err)
		} else {
			fetchedAny = true
			report += buildPassthroughCollisionReport(status.Collisions)
			report += buildPassthroughSuggestionReport(status.Devices, alreadyDeclaredIdentifiers(discoveryGatewaysByID))
		}
	}

	if !fetchedAny {
		return nil
	}

	suggestionPath := filepath.Join(outputRoot, "suggestions", "discovery.txt")
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
