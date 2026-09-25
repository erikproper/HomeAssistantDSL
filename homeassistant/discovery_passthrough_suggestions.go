/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: DiscoveryPassthroughSuggestions
 *
 * The missing half of PROJECT.md 1.8's suggestion mechanism for PROJECT.md item 7's passthrough
 * relay (house_event_bus_coordinator/discoverybridge.go, discovery_passthrough_devices.go):
 * mqtt_discovery_existence.go's own suggestion report only ever covers undeclared LEAVES of an
 * ALREADY-declared "discovery" gateway (a gatewayID is required just to key its tracking), so a
 * device passthrough is relaying byte-for-byte -- one no declared gateway claims at all yet -- was
 * entirely invisible in suggestions/discovery.txt, defeating a good chunk of the point of a
 * *gradual* migration (found live 2026-09-14: real Zigbee2MQTT traffic was flowing through
 * passthrough but never showed up there).
 *
 * Fetches the coordinator's own retained passthroughDevicesStatusTopic snapshot (same
 * fetch-when-online/fall-back-to-cache pattern mqtt_discovery_existence.go already established)
 * and turns it into copy-paste-ready "device discovery.<slug> with: identifiers "<real-id>";
 * ...; end;" blocks -- unlike a gateway-leaf suggestion, these need an EXPLICIT identifiers line,
 * since the suggested local DeviceID slug (derived from the device's own reported name) has no
 * relation to inferredDiscoveryIdentifier's naming convention the way an already-declared device's
 * own id does.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 14.09.2026
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

// fetchPassthroughDevicesTimeout mirrors fetchDiscoveryExistenceTimeout's own reasoning exactly.
const fetchPassthroughDevicesTimeout = 5 * time.Second

// passthroughDevicesStatusTopic is house_event_bus_coordinator/discovery_passthrough_devices.go's
// own retained status topic -- an independent copy of the same literal string, since the two
// packages never share Go types (same convention discoveryGatewayExistenceStatusTopic/
// discoveryExistenceStatusTopic already established).
const passthroughDevicesStatusTopic = "discovery_passthrough/devices/state"

// tPassthroughLeafSuggestion mirrors discoverybridge.go's own TPassthroughLeaf.
type tPassthroughLeafSuggestion struct {
	Domain string `json:"domain"`
	Leaf   string `json:"leaf"`
}

// tPassthroughDeviceSuggestion mirrors discovery_passthrough_devices.go's own
// tPassthroughDeviceEntry -- what the coordinator's retained status payload actually carries.
type tPassthroughDeviceSuggestion struct {
	Name   string                                `json:"name"`
	Leaves map[string]tPassthroughLeafSuggestion `json:"leaves"`
}

// tPassthroughNameCollisionSuggestion mirrors house_event_bus_coordinator/
// discovery_passthrough_devices.go's own TPassthroughNameCollision (2026-09-20) -- two different
// devices' passthrough-relayed payloads both claimed the same default_entity_id; ClaimedBy's relay
// went through, BlockedID's was suppressed. See buildPassthroughCollisionReport.
type tPassthroughNameCollisionSuggestion struct {
	ClaimedBy string `json:"claimed_by"`
	BlockedID string `json:"blocked_id"`
}

// tPassthroughStatusSuggestion mirrors house_event_bus_coordinator's own tPassthroughStatusSnapshot
// -- the full shape of the retained passthroughDevicesStatusTopic payload.
type tPassthroughStatusSuggestion struct {
	Devices    map[string]tPassthroughDeviceSuggestion        `json:"devices"`
	Collisions map[string]tPassthroughNameCollisionSuggestion `json:"collisions"`
}

// passthroughDevicesCachePath is the local, persisted cache file for the passthrough device
// snapshot -- same convention discoveryExistenceCachePath already established for kind-2 gateways.
func passthroughDevicesCachePath(definitionDir string) string {
	return filepath.Join(definitionDir, ".cache", "discovery_passthrough_devices.json")
}

// fetchPassthroughDevices tries a fresh read, cloud broker first then local
// (fetchPassthroughDevicesPreferringCloud), and on success updates the local cache file. On any
// failure, falls back to that cache's last-known contents. Returns an error only when neither live
// broker nor a usable cache is available.
func fetchPassthroughDevices(definitionDir string, ctx TPhysicalGenerationContext) (tPassthroughStatusSuggestion, error) {
	cachePath := passthroughDevicesCachePath(definitionDir)

	status, fetchErr := fetchPassthroughDevicesPreferringCloud(ctx)
	if fetchErr == nil {
		if data, err := json.Marshal(status); err == nil {
			if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err == nil {
				if err := os.WriteFile(cachePath, data, 0o644); err != nil {
					fmt.Printf("[physical] passthrough devices: fetched fresh but could not update local cache %s: %v\n", cachePath, err)
				}
			}
		}
		return status, nil
	}

	cached, cacheErr := os.ReadFile(cachePath)
	if cacheErr != nil {
		return tPassthroughStatusSuggestion{}, fmt.Errorf("%w (and no local cache at %s to fall back to)", fetchErr, cachePath)
	}
	var cachedStatus tPassthroughStatusSuggestion
	if err := json.Unmarshal(cached, &cachedStatus); err != nil {
		return tPassthroughStatusSuggestion{}, fmt.Errorf("%w (local cache at %s is also unreadable: %v)", fetchErr, cachePath, err)
	}
	printOfflineCacheNoticeOnce()
	return cachedStatus, nil
}

// fetchPassthroughDevicesPreferringCloud tries the "coordinator_only" cloud broker first (when
// declared), falling back to the local broker -- matches fetchDiscoveryExistencePreferringCloud's
// own policy exactly.
func fetchPassthroughDevicesPreferringCloud(ctx TPhysicalGenerationContext) (tPassthroughStatusSuggestion, error) {
	// Cloud failure here is silent -- see printOfflineCacheNoticeOnce's own doc comment.
	if cloudSecrets, ok := coordinatorOnlyBrokerSecrets(ctx); ok {
		if status, err := fetchPassthroughDevicesFromBroker(cloudSecrets, ctx.Installation); err == nil {
			return status, nil
		}
	}
	return fetchPassthroughDevicesFromBroker(ctx.MQTTSecrets, "")
}

// fetchPassthroughDevicesFromBroker connects to secrets' broker as a one-shot client and returns
// whatever the coordinator has last published (retained) on passthroughDevicesStatusTopic.
// ownInstallation, when non-empty, additionally prefixes the topic -- the cloud broker's own
// qualification convention, mirrors fetchDiscoveryExistenceFromBroker exactly.
func fetchPassthroughDevicesFromBroker(secrets TMQTTBrokerSecrets, ownInstallation string) (tPassthroughStatusSuggestion, error) {
	if err, known := brokerKnownUnreachable(secrets); known {
		return tPassthroughStatusSuggestion{}, fmt.Errorf("broker %s:%s already known unreachable this run: %w", secrets.Server, secrets.Port, err)
	}

	topic := passthroughDevicesStatusTopic
	if ownInstallation != "" {
		topic = ownInstallation + "/" + topic
	}

	opts := mqtt.NewClientOptions()
	scheme := "tcp"
	if secrets.TLS {
		scheme = "ssl"
	}
	opts.AddBroker(fmt.Sprintf("%s://%s:%s", scheme, secrets.Server, secrets.Port))
	opts.SetClientID("homeassistant-generator-discovery-passthrough")
	opts.SetUsername(secrets.Login)
	opts.SetPassword(secrets.Password)
	opts.SetConnectTimeout(fetchPassthroughDevicesTimeout)
	// See mqtt_discovery_existence.go's identical call for why: a one-shot fetch client must
	// never keep retrying in the background after this function itself has given up.
	opts.SetAutoReconnect(false)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(fetchPassthroughDevicesTimeout) || token.Error() != nil {
		var err error
		if tokenErr := token.Error(); tokenErr != nil {
			err = fmt.Errorf("connecting to MQTT broker %s:%s: %w", secrets.Server, secrets.Port, tokenErr)
		} else {
			err = fmt.Errorf("connecting to MQTT broker %s:%s: timed out", secrets.Server, secrets.Port)
		}
		markBrokerUnreachable(secrets, err)
		return tPassthroughStatusSuggestion{}, err
	}
	defer client.Disconnect(250)

	received := make(chan []byte, 1)
	subToken := client.Subscribe(topic, 0, func(_ mqtt.Client, msg mqtt.Message) {
		select {
		case received <- msg.Payload():
		default:
		}
	})
	if !subToken.WaitTimeout(fetchPassthroughDevicesTimeout) || subToken.Error() != nil {
		if err := subToken.Error(); err != nil {
			return tPassthroughStatusSuggestion{}, fmt.Errorf("subscribing to %s: %w", topic, err)
		}
		return tPassthroughStatusSuggestion{}, fmt.Errorf("subscribing to %s: timed out", topic)
	}

	select {
	case payload := <-received:
		var status tPassthroughStatusSuggestion
		if err := json.Unmarshal(payload, &status); err != nil {
			return tPassthroughStatusSuggestion{}, fmt.Errorf("parsing %s payload: %w", topic, err)
		}
		return status, nil
	case <-time.After(fetchPassthroughDevicesTimeout):
		return tPassthroughStatusSuggestion{}, fmt.Errorf("timed out waiting for %s (has the coordinator published passthrough device status yet?)", topic)
	}
}

// alreadyDeclaredIdentifiers collects every identifier any declared "discovery" gateway already
// claims -- a generate-time safety net alongside the coordinator's own Forget-on-migration
// (discoverybridge.go): a device the coordinator hasn't yet re-processed a live message for since
// a redeploy could otherwise still appear here for one stale cycle.
func alreadyDeclaredIdentifiers(discoveryGatewaysByID map[string]TDiscoveryGatewayDevice) map[string]bool {
	claimed := map[string]bool{}
	for _, device := range discoveryGatewaysByID {
		for _, id := range device.Identifiers {
			claimed[id] = true
		}
	}
	return claimed
}

// buildPassthroughSuggestionReport formats every still-undeclared passthrough device as a
// copy-paste-ready Physical.def "device discovery.<slug> with: ...; end;" block, sorted by device
// identifier for deterministic output -- the passthrough counterpart to
// buildDiscoverySuggestionReport. Unlike a gateway-leaf suggestion, each block carries an explicit
// "identifiers" line: the suggested local id is slugified from the device's own reported name
// (falling back to the raw identifier when no name was ever reported), which has no relation to
// inferredDiscoveryIdentifier's naming convention the way an already-declared device's own id does.
func buildPassthroughSuggestionReport(devices map[string]tPassthroughDeviceSuggestion, claimed map[string]bool) string {
	identifiers := make([]string, 0, len(devices))
	for id := range devices {
		if claimed[id] {
			continue
		}
		identifiers = append(identifiers, id)
	}
	sort.Strings(identifiers)

	var sb strings.Builder
	for _, identifier := range identifiers {
		device := devices[identifier]
		if len(device.Leaves) == 0 {
			continue
		}
		slug := sanitizeObjectID(device.Name)
		if slug == "" {
			slug = sanitizeObjectID(identifier)
		}

		leafKeys := make([]string, 0, len(device.Leaves))
		for leaf := range device.Leaves {
			leafKeys = append(leafKeys, leaf)
		}
		sort.Strings(leafKeys)

		type line struct{ lhs, rhs, comment string }
		var lines []line
		maxLHS := 0
		for _, leaf := range leafKeys {
			domain := device.Leaves[leaf].Domain
			_, suffix := recognizeDiscoveryCapability(leaf)
			comment := ""
			if suffix == "" {
				suffix = leaf
				comment = " # not recognized"
			}
			lhs := domain + "." + suffix
			if len(lhs) > maxLHS {
				maxLHS = len(lhs)
			}
			lines = append(lines, line{lhs: lhs, rhs: domain + "." + leaf + ";", comment: comment})
		}

		sb.WriteString("device discovery." + slug + " with:\n")
		sb.WriteString("  identifiers \"" + identifier + "\";\n")
		for _, l := range lines {
			sb.WriteString("  " + l.lhs + strings.Repeat(" ", maxLHS-len(l.lhs)+1) + l.rhs + l.comment + "\n")
		}
		sb.WriteString("end;\n\n")
	}
	return sb.String()
}

// buildPassthroughCollisionReport formats every currently-suppressed same-name passthrough clash
// (house_event_bus_coordinator/discovery_passthrough_devices.go's ClaimName, 2026-09-20) as a
// human-readable warning block, sorted by default_entity_id for deterministic output. Unlike
// buildPassthroughSuggestionReport's copy-paste-ready Physical.def blocks, there's nothing to paste
// here -- resolving a collision means fixing the SOURCE (typically: the blocked device was renamed
// in Zigbee2MQTT and its old retained discovery payload never got refreshed to match; reconfiguring
// or renaming it again in Zigbee2MQTT's own UI forces a fresh, correctly-named republish).
func buildPassthroughCollisionReport(collisions map[string]tPassthroughNameCollisionSuggestion) string {
	names := make([]string, 0, len(collisions))
	for name := range collisions {
		names = append(names, name)
	}
	sort.Strings(names)

	var sb strings.Builder
	for _, name := range names {
		collision := collisions[name]
		sb.WriteString("# WARNING: default_entity_id \"" + name + "\" is claimed by two different passthrough\n")
		sb.WriteString("# devices -- only \"" + collision.ClaimedBy + "\" is being relayed into Home Assistant;\n")
		sb.WriteString("# \"" + collision.BlockedID + "\" is suppressed until this is resolved. Likely a stale\n")
		sb.WriteString("# Zigbee2MQTT discovery payload left over from a device rename -- reconfigure or rename\n")
		sb.WriteString("# that device in Zigbee2MQTT to force it to republish under its current identity.\n\n")
	}
	return sb.String()
}
