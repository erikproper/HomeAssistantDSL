/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: MQTTImportExistence
 *
 * Kind-4 (cross-house import) half of PROJECT.md item 1 (2026-09-07) -- reads the coordinator's own
 * three-state existence status for declared import capabilities
 * (house_event_bus_coordinator/import_existence.go publishes it to
 * "import_existence/<remoteInstallation>/existence/state", retained, on both the local and cloud
 * broker unconditionally when a cloud one is configured), and turns a confirmed known-not-to-exist
 * verdict into a generate-time error -- exactly mirroring mqtt_discovery_existence.go's
 * checkDiscoveryKnownNotToExistErrors for kind-2, just keyed by (remote installation, stable id)
 * instead of (gateway, leaf).
 *
 * Like kind-2, this status is populated *passively* (the exporting installation's own coordinator
 * already self-announces via retained cloud discovery configs) -- a stable id can only ever be
 * not-known-to-exist or known-to-exist today; known-not-to-exist (retraction) is a coordinator-side
 * gap not yet built, matching kind-2's own.
 *
 * Persisted local cache (Definitions/.cache/), same fetch-when-online/fall-back-to-cache pattern
 * every other existence mechanism already established.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 07.09.2026
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

// fetchImportExistenceTimeout mirrors fetchExistenceTimeout's own reasoning exactly.
const fetchImportExistenceTimeout = 5 * time.Second

// TImportExistenceStatusPayload is the top-level shape published to
// "import_existence/<remoteInstallation>/existence/state" -- keyed by stable id (exportStableID's
// own scheme), no device grouping, same flat shape kind-2's own status payload uses.
type TImportExistenceStatusPayload map[string]string

func importExistenceGeneratorStatusTopic(remoteInstallation string) string {
	return "import_existence/" + remoteInstallation + "/existence/state"
}

// importExistenceCachePath is remoteInstallation's own local, persisted cache file.
func importExistenceCachePath(definitionDir, remoteInstallation string) string {
	return filepath.Join(definitionDir, ".cache", "import_existence_"+sanitizeCacheFileSegment(remoteInstallation)+".json")
}

// fetchImportExistence tries a fresh read, cloud broker first then local
// (fetchImportExistencePreferringCloud), and on success updates remoteInstallation's own local
// cache file. On any failure, falls back to that cache's last-known contents. Returns an error only
// when neither live broker nor a usable cache is available.
func fetchImportExistence(definitionDir string, ctx TPhysicalGenerationContext, remoteInstallation string) (TImportExistenceStatusPayload, error) {
	cachePath := importExistenceCachePath(definitionDir, remoteInstallation)

	status, fetchErr := fetchImportExistencePreferringCloud(ctx, remoteInstallation)
	if fetchErr == nil {
		if data, err := json.Marshal(status); err == nil {
			if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err == nil {
				if err := os.WriteFile(cachePath, data, 0o644); err != nil {
					fmt.Printf("[physical] import existence for %q: fetched fresh but could not update local cache %s: %v\n", remoteInstallation, cachePath, err)
				}
			}
		}
		return status, nil
	}

	cached, cacheErr := os.ReadFile(cachePath)
	if cacheErr != nil {
		return nil, fmt.Errorf("%w (and no local cache at %s to fall back to)", fetchErr, cachePath)
	}
	var cachedStatus TImportExistenceStatusPayload
	if err := json.Unmarshal(cached, &cachedStatus); err != nil {
		return nil, fmt.Errorf("%w (local cache at %s is also unreadable: %v)", fetchErr, cachePath, err)
	}
	fmt.Printf("[physical] import existence for %q: live fetch failed (%v), using cached copy from %s\n", remoteInstallation, fetchErr, cachePath)
	return cachedStatus, nil
}

// fetchImportExistencePreferringCloud tries the "coordinator_only" cloud broker first (when
// declared), falling back to the local broker -- matches every other existence mechanism's own
// policy exactly. Preferring cloud isn't just convention here: a cross-house import's own remote
// installation status is published by THAT installation's own coordinator, so unlike kind-2/kind-3
// (whose local-broker copy is authoritative for this same house), the cloud broker is often the
// ONLY place this specific status is reachable from at all.
func fetchImportExistencePreferringCloud(ctx TPhysicalGenerationContext, remoteInstallation string) (TImportExistenceStatusPayload, error) {
	if cloudSecrets, ok := coordinatorOnlyBrokerSecrets(ctx); ok {
		if status, err := fetchImportExistenceFromBroker(cloudSecrets, remoteInstallation, ctx.Installation); err == nil {
			return status, nil
		} else {
			fmt.Printf("[physical] import existence for %q: cloud broker attempt failed (%v), trying local\n", remoteInstallation, err)
		}
	}
	return fetchImportExistenceFromBroker(ctx.MQTTSecrets, remoteInstallation, "")
}

// fetchImportExistenceFromBroker connects to secrets' broker as a one-shot client and returns
// whatever the coordinator has last published (retained) on
// importExistenceGeneratorStatusTopic(remoteInstallation).
//
// ownInstallation, when non-empty, additionally prefixes the topic with THIS house's own
// installation name -- house_event_bus_coordinator/import_existence.go's own publishStatus
// qualifies its cloud copy with ITS OWN installation (the house running the import), not
// remoteInstallation, so this must match that exactly. Pass "" for the local broker, whose copy
// stays bare.
func fetchImportExistenceFromBroker(secrets TMQTTBrokerSecrets, remoteInstallation, ownInstallation string) (TImportExistenceStatusPayload, error) {
	topic := importExistenceGeneratorStatusTopic(remoteInstallation)
	if ownInstallation != "" {
		topic = ownInstallation + "/" + topic
	}

	opts := mqtt.NewClientOptions()
	scheme := "tcp"
	if secrets.TLS {
		scheme = "ssl"
	}
	opts.AddBroker(fmt.Sprintf("%s://%s:%s", scheme, secrets.Server, secrets.Port))
	opts.SetClientID("homeassistant-generator-import-existence-" + remoteInstallation)
	opts.SetUsername(secrets.Login)
	opts.SetPassword(secrets.Password)
	opts.SetConnectTimeout(fetchImportExistenceTimeout)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(fetchImportExistenceTimeout) || token.Error() != nil {
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
	if !subToken.WaitTimeout(fetchImportExistenceTimeout) || subToken.Error() != nil {
		if err := subToken.Error(); err != nil {
			return nil, fmt.Errorf("subscribing to %s: %w", topic, err)
		}
		return nil, fmt.Errorf("subscribing to %s: timed out", topic)
	}

	select {
	case payload := <-received:
		var status TImportExistenceStatusPayload
		if err := json.Unmarshal(payload, &status); err != nil {
			return nil, fmt.Errorf("parsing %s payload: %w", topic, err)
		}
		return status, nil
	case <-time.After(fetchImportExistenceTimeout):
		return nil, fmt.Errorf("timed out waiting for %s (has the remote installation's coordinator published import existence status yet?)", topic)
	}
}

// checkImportKnownNotToExistErrors fetches (fetch-then-cache-fallback) existence status for every
// distinct remote installation a declared import capability references, and returns a combined
// error listing every one the coordinator has confirmed known-not-to-exist -- the same rule every
// other existence mechanism already follows: this is the *only* status that blocks generation;
// not-known-to-exist and known-to-exist both generate optimistically. Soft-fails (a warning) per
// installation when neither a fresh read nor a cache is available.
func checkImportKnownNotToExistErrors(definitionDir string, importedDevices []TImportedDevice, ctx TPhysicalGenerationContext) error {
	installations := map[string]bool{}
	for _, device := range importedDevices {
		if len(device.Capabilities) > 0 && device.RemoteInstallation != "" {
			installations[device.RemoteInstallation] = true
		}
	}
	if len(installations) == 0 {
		return nil
	}

	statusByInstallation := map[string]TImportExistenceStatusPayload{}
	for installation := range installations {
		status, err := fetchImportExistence(definitionDir, ctx, installation)
		if err != nil {
			fmt.Printf("[physical] import existence for %q: %v\n", installation, err)
			continue
		}
		statusByInstallation[installation] = status
	}

	deviceIDs := make([]string, 0, len(importedDevices))
	byID := map[string]TImportedDevice{}
	for _, d := range importedDevices {
		if _, exists := byID[d.DeviceID]; exists {
			continue
		}
		byID[d.DeviceID] = d
		deviceIDs = append(deviceIDs, d.DeviceID)
	}
	sort.Strings(deviceIDs)

	var problems []string
	for _, deviceID := range deviceIDs {
		device := byID[deviceID]
		status, known := statusByInstallation[device.RemoteInstallation]
		if !known {
			continue
		}
		capNames := make([]string, 0, len(device.Capabilities))
		for name := range device.Capabilities {
			capNames = append(capNames, name)
		}
		sort.Strings(capNames)
		for _, capability := range capNames {
			stableID := exportStableIDGen(device.RemoteInstallation, device.RemoteDeviceID, capability)
			if status[stableID] != existenceStatusKnownNotToExist {
				continue
			}
			problems = append(problems, fmt.Sprintf("%s (device %q, capability %q): remote stable id %q confirmed not to exist on installation %q",
				capability, deviceID, capability, stableID, device.RemoteInstallation))
		}
	}

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("import existence check failed -- the coordinator has confirmed %d declared shorthand capability/capabilities reference an entity that does not exist:\n  %s",
		len(problems), strings.Join(problems, "\n  "))
}

// exportStableIDGen mirrors house_event_bus_coordinator/discoveryhassbridge.go's own exportStableID
// formula exactly ("<qualifier>_<deviceID-with-dots-as-underscores>_<capability>") -- duplicated
// here rather than imported since the generator and coordinator are separate Go modules with no
// shared internal package. Keep the two in sync if the formula ever changes.
func exportStableIDGen(qualifier, deviceID, capability string) string {
	return qualifier + "_" + strings.ReplaceAll(deviceID, ".", "_") + "_" + capability
}
