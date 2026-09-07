/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: MQTTRelay
 *
 * Handles the "cloud"/"import" routing a device can declare in Physical.def
 * (homeassistant/mqtt_routing_keywords.go, homeassistant/integration_import_parser.go), on top
 * of mqtt.go's existing "Home Assistant subscribes directly, the coordinator never bridges
 * device state" default. Routing table, by TDevice's own Cloud/ImportedFrom fields:
 *
 *   plain local (no Cloud, no ImportedFrom): discovery -> main only, as always -- unaffected by
 *     anything in this file.
 *   Cloud, no ImportedFrom:                   discovery -> cloud only. Never visible on the
 *     local/main broker or this house's own Home Assistant at all -- the device's own report
 *     script publishes directly to the cloud broker, installation-qualified (cpu/report script,
 *     deployed outside this repo).
 *   Cloud && ImportedFrom == this house's own installation ("self-import" -- the "import"
 *     keyword on the SAME "cloud" declaration, parser-level sugar for a device that reports
 *     under its OWN name, homeassistant/mqtt_routing_keywords.go): discovery -> both.
 *     relayCloudDevice bridges its raw state/device-info traffic from the cloud broker's own
 *     installation-qualified topics onto this house's local/main broker's bare topics too, so
 *     its own Home Assistant sees it exactly like a plain local device (source-installation =
 *     this house's own ${installation}, since it published under its own name). Retired
 *     2026-09-02: this used to be a separate "native" keyword/bool -- replaced because "native"
 *     silently assumed THIS house was always the real owner, which broke the moment the same
 *     device got declared "cloud native" independently in a second house too (confirmed live:
 *     host.mqtt/host.eriks-macbook-pro-2 permanently "unavailable" in Vienna, since only
 *     Junglinster is their real owner). Explicit ImportedFrom removes the assumption entirely.
 *   ImportedFrom != "" (real cross-house import, no Cloud on this record -- imports never set
 *     Cloud):                                discovery -> main only (as if plain local; the
 *     remote installation is the one that published its own discovery/cloud-side, not this
 *     house). relayCloudDevice bridges it in exactly like the self-import case above, except the
 *     source-installation is the declared ImportedFrom name, not this house's own.
 *
 * Availability inversion for both relayed cases (the user's own framing): a "cloud"-routed
 * device can't be meaningfully LAN-pinged (it may be roaming), so node/state stops being an
 * independent, externally-ping-sourced signal that cpu/temperature's own availability_topic
 * merely depends on -- instead TCloudLivenessTracker treats *arrival of relayed cpu/device-info
 * traffic itself* as the liveness signal, and node/state becomes a direct, computed reflection
 * of that: "true" the moment traffic resumes, "false" after cloudDeviceStaleAfter (3 minutes) of
 * silence. generatePingHostsFile (homeassistant/integration_hosts_generator.go) excludes every
 * "cloud"-tagged device from ping/hosts entirely, so nothing else ever writes that topic and
 * fights the timeout. This computed value is cross-posted to the cloud broker too (qualified the
 * same way any other relayed traffic is), not just published locally -- the exporting
 * installation is the one that knows whether a device has a real liveness source of its own, so
 * when it doesn't, it publishes its own computed one to the shared cloud catalogue, letting a
 * genuine cross-house importer (house_event_bus_coordinator/discoveryimport.go) relay it like any
 * other capability instead of needing its own synthesis.
 *
 * Cloud-side discovery configs are a best-effort catalogue entry, not independently functional
 * for a third party to consume directly -- their embedded state_topic/etc. fields still point at
 * this house's own bare local topic strings (buildDiscoveryConfigs is unaware of any of this),
 * only the discovery *topic itself* is installation-qualified (avoiding node_id collisions with
 * another installation's identically-shaped device on the same shared broker). A real consumer
 * imports the underlying device instead (this file's own relayCloudDevice, on the OTHER
 * installation's coordinator), which builds its own correctly-qualified subscription from its
 * own "integration import" declaration rather than parsing this catalogue entry's payload.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 26.08.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// qualifyHostsTopic inserts qualifier as a path segment right after a "hosts/<host>/..." bare
// topic's host segment -- "hosts/eriks-macbook-pro-2/cpu/state" + "junglinster" becomes
// "hosts/eriks-macbook-pro-2/junglinster/cpu/state" -- rather than prefixing the whole topic, so
// the result still starts "hosts/<host>/...", matching the cloud broker's own client-ID-scoped
// ACL ("pattern readwrite hosts/%c/#", the report script's own "-i $hostname"); an overall
// "<installation>/hosts/..." prefix would fall outside that ACL and be silently dropped instead.
// cpu/report (deployed, outside this repo) builds its own cloud-routed topics the same way --
// see its own doc comment. Falls back to a plain prefix if bareTopic isn't hosts/-shaped (should
// never happen given the fixed StateTopicTemplate/NodeTopicTemplate/DeviceInfoTopicTemplate
// conventions, homeassistant/integration_hosts_storage.go).
func qualifyHostsTopic(bareTopic, qualifier string) string {
	parts := strings.SplitN(bareTopic, "/", 3)
	if len(parts) != 3 || parts[0] != "hosts" {
		return qualifier + "/" + bareTopic
	}
	return parts[0] + "/" + parts[1] + "/" + qualifier + "/" + parts[2]
}

// TDeviceRoutingPlan says which broker(s) a device's discovery config belongs on -- see this
// file's own doc comment for the full table.
type TDeviceRoutingPlan struct {
	PublishMain  bool
	PublishCloud bool
}

// deviceRoutingPlan resolves device's routing plan. hasCloudClient is false when this house has
// no cloud broker profile configured at all -- in that case nothing ever routes to cloud,
// regardless of what a device declares (publishDiscovery already warns once per such device).
//
// PublishMain and PublishCloud are deliberately independent checks, not a linear if/else-early-
// -return chain (which is what this function used to be, before "native" was retired 2026-09-02)
// -- a device can now be BOTH ImportedFrom-qualified (even self-referentially, "imported" from
// this same house's own installation, the "cloud"+"import" self-import case) AND Cloud at the
// same time, and both facts need to keep mattering independently: PublishMain must be true
// because it's (self-)imported, and PublishCloud must ALSO stay true because it's still Cloud --
// an early return on the ImportedFrom check alone would silently stop publishing the cloud
// catalogue entry for a device that's still very much its own real cloud-side source. PublishMain
// never depends on hasCloudClient, for either kind of import -- consistent with how a real
// cross-house import already behaved before this change.
func deviceRoutingPlan(device TDevice, hasCloudClient bool) TDeviceRoutingPlan {
	var plan TDeviceRoutingPlan
	if device.ImportedFrom != "" || !device.Cloud {
		plan.PublishMain = true
	}
	if device.Cloud && hasCloudClient {
		plan.PublishCloud = true
	}
	return plan
}

// needsCloudRelay reports whether device's raw state/node/device-info traffic needs bridging
// from the cloud broker's installation-qualified topics onto main's bare ones (relayCloudDevice),
// and the source installation to qualify with -- always device.ImportedFrom, whether it names
// this house's own installation (a self-import, the "cloud"+"import" case: the device published
// under its own name, and this coordinator is the owner reading its own data back) or another
// house's (a real cross-house import). Both are structurally identical from here on -- there is
// no longer a separate "cloud && native" case (retired 2026-09-02 along with the "native"
// keyword/field): every consumer, owner included, is now qualified by an explicit, declared
// installation name rather than an implicit "this house must be the owner" assumption.
func needsCloudRelay(device TDevice) (qualifier string, needed bool) {
	if device.ImportedFrom != "" {
		return device.ImportedFrom, true
	}
	return "", false
}

// cloudDeviceStaleAfter is how long a "cloud"-routed device's node/state stays "true" after its
// last relayed cpu/device-info message before TCloudLivenessTracker's sweep marks it "false" --
// roughly 3 report cycles (cpu/report runs every 60s) of grace before declaring it gone quiet,
// not just one missed beat.
const cloudDeviceStaleAfter = 3 * time.Minute

// cloudLivenessSweepInterval is how often TCloudLivenessTracker checks for devices that have
// gone stale -- frequent enough that "false" appears reasonably promptly after
// cloudDeviceStaleAfter elapses, without polling pointlessly often.
const cloudLivenessSweepInterval = 30 * time.Second

// TCloudLivenessTracker computes node/state for every "cloud"-routed device from relayed traffic
// arrival alone (see this file's own doc comment, "Availability inversion") -- Touch marks a
// device alive (publishing "true" immediately) whenever relayCloudDevice forwards a message for
// it, and the sweeper started by StartSweeping marks it "false" once cloudDeviceStaleAfter has
// passed with no Touch.
type TCloudLivenessTracker struct {
	mu       sync.Mutex
	lastSeen map[string]time.Time
	stale    map[string]bool // true once "false" has already been published for this device -- avoids republishing every sweep
}

func newCloudLivenessTracker() *TCloudLivenessTracker {
	return &TCloudLivenessTracker{lastSeen: map[string]time.Time{}, stale: map[string]bool{}}
}

// Register seeds deviceID's last-seen time to now, if it has none yet -- called once when a
// device is set up for relay, so a device that never reports at all gets a full
// cloudDeviceStaleAfter grace period from coordinator startup before being marked stale, rather
// than immediately.
func (t *TCloudLivenessTracker) Register(deviceID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, known := t.lastSeen[deviceID]; !known {
		t.lastSeen[deviceID] = time.Now()
	}
}

// Touch marks deviceID as seen just now, and publishes "true" to nodeTopic on mainClient --
// unconditionally, not just on a stale->alive transition, matching how a report's own cpu/state
// value is republished on every message regardless of whether it changed; a repeat retained
// "true" publish is a cheap no-op on the broker side. Also cross-posts the same "true" to
// cloudClient, qualified the same way any other relayed traffic is (qualifyHostsTopic(nodeTopic,
// qualifier)) -- the user's own framing: the exporting coordinator already knows whether a device
// has a real liveness source of its own; when it doesn't (this whole tracker exists because
// "hosts"-kind devices' report script never publishes node/state itself), it should publish its
// own computed signal to the cloud catalogue too, so a genuine cross-house importer
// (discoveryimport.go) can just relay it like any other capability rather than needing its own
// synthesis. No-op if cloudClient is nil or qualifier is "" (nothing to cross-post to).
func (t *TCloudLivenessTracker) Touch(mainClient, cloudClient mqtt.Client, deviceID, nodeTopic, qualifier string) {
	t.mu.Lock()
	t.lastSeen[deviceID] = time.Now()
	t.stale[deviceID] = false
	t.mu.Unlock()

	if nodeTopic == "" {
		return
	}
	if err := publishRetained(mainClient, nodeTopic, []byte("true")); err != nil {
		fmt.Printf("[liveness] %s: publishing %s: %v\n", deviceID, nodeTopic, err)
	}
	if cloudClient != nil && qualifier != "" {
		cloudTopic := qualifyHostsTopic(nodeTopic, qualifier)
		if err := publishRetained(cloudClient, cloudTopic, []byte("true")); err != nil {
			fmt.Printf("[liveness] %s: publishing %s: %v\n", deviceID, cloudTopic, err)
		}
	}
}

// StartSweeping runs sweepOnce every cloudLivenessSweepInterval in its own goroutine until the
// process exits (the coordinator has no shutdown path for background goroutines today -- the
// same "runs for the process lifetime" shape as every other subscription here).
func (t *TCloudLivenessTracker) StartSweeping(mainClient, cloudClient mqtt.Client, devices map[string]TDevice) {
	go func() {
		ticker := time.NewTicker(cloudLivenessSweepInterval)
		defer ticker.Stop()
		for range ticker.C {
			t.sweepOnce(mainClient, cloudClient, devices)
		}
	}()
}

// staleSince returns every registered deviceID whose last Touch is older than
// cloudDeviceStaleAfter and hasn't already been marked stale, marking them stale in the same pass
// so a repeat sweep won't return them again. Factored out of sweepOnce so a liveness domain with
// no TDevice entry of its own (e.g. discoveryimport.go's hosts-kind imports, which have no
// devices.yaml entry -- see that file's own header comment) can reuse the same tracker/staleness
// logic while resolving "how to publish the false value" its own way.
func (t *TCloudLivenessTracker) staleSince(now time.Time) []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	var goneStale []string
	for deviceID, last := range t.lastSeen {
		if !t.stale[deviceID] && now.Sub(last) > cloudDeviceStaleAfter {
			t.stale[deviceID] = true
			goneStale = append(goneStale, deviceID)
		}
	}
	return goneStale
}

// sweepOnce marks "false" (once -- see t.stale) every tracked device whose last Touch is older
// than cloudDeviceStaleAfter -- on mainClient, and, when cloudClient is configured, also
// cross-posted to cloud qualified by the device's own ImportedFrom (always this house's own
// installation name in practice, since relayCloudDevice only ever runs for the self-import case --
// see Touch's own doc comment for why this cross-post exists at all).
func (t *TCloudLivenessTracker) sweepOnce(mainClient, cloudClient mqtt.Client, devices map[string]TDevice) {
	for _, deviceID := range t.staleSince(time.Now()) {
		device, known := devices[deviceID]
		if !known || device.NodeTopic == "" {
			continue
		}
		if err := publishRetained(mainClient, device.NodeTopic, []byte("false")); err != nil {
			fmt.Printf("[liveness] %s: publishing %s: %v\n", deviceID, device.NodeTopic, err)
			continue
		}
		fmt.Printf("[liveness] %s: no traffic for over %s, marked unavailable\n", deviceID, cloudDeviceStaleAfter)
		if cloudClient != nil && device.ImportedFrom != "" {
			cloudTopic := qualifyHostsTopic(device.NodeTopic, device.ImportedFrom)
			if err := publishRetained(cloudClient, cloudTopic, []byte("false")); err != nil {
				fmt.Printf("[liveness] %s: publishing %s: %v\n", deviceID, cloudTopic, err)
			}
		}
	}
}

// relayCloudDevice subscribes on cloudClient to device's own state/device-info topics, each
// prefixed with qualifier+"/" (the installation-qualified form its own report script -- or, for
// an import, the remote installation's own coordinator -- actually publishes under on the shared
// cloud broker), and republishes each message verbatim (same payload, retained) onto mainClient
// under device's plain, unqualified topic -- the same bare "hosts/<host>/..." form this house's
// own Home Assistant, and this coordinator's own subscribeDeviceInfo, already expect. Every
// relayed message also Touches tracker, so device.NodeTopic reflects "traffic just arrived"
// rather than being separately relayed from anywhere -- see this file's own doc comment,
// "Availability inversion." Empty topic fields (e.g. a "ping"-type device has no Topic) are
// skipped; device.NodeTopic itself is never subscribed from the cloud side at all -- it's
// computed, not relayed.
func relayCloudDevice(cloudClient, mainClient mqtt.Client, deviceID string, device TDevice, qualifier string, tracker *TCloudLivenessTracker) error {
	tracker.Register(deviceID)
	for _, bareTopic := range []string{device.Topic, device.DeviceInfoTopic} {
		if bareTopic == "" {
			continue
		}
		sourceTopic := qualifyHostsTopic(bareTopic, qualifier)
		target := bareTopic
		handler := func(_ mqtt.Client, msg mqtt.Message) {
			token := mainClient.Publish(target, 0, true, msg.Payload())
			if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
				err := token.Error()
				fmt.Printf("[relay] %s: republishing %s -> %s: %v\n", deviceID, msg.Topic(), target, err)
			}
			tracker.Touch(mainClient, cloudClient, deviceID, device.NodeTopic, qualifier)
		}
		tok := cloudClient.Subscribe(sourceTopic, 0, handler)
		if !tok.WaitTimeout(10*time.Second) || tok.Error() != nil {
			if err := tok.Error(); err != nil {
				return fmt.Errorf("relaying %s: subscribing to %s: %w", deviceID, sourceTopic, err)
			}
			return fmt.Errorf("relaying %s: subscribing to %s: timed out", deviceID, sourceTopic)
		}
	}
	return nil
}

// relayCloudDevices calls relayCloudDevice for every device in devicesFile that needsCloudRelay,
// skipping (with a warning) any that would need it but cloudClient is nil (no cloud broker
// profile configured for this house), then starts tracker's background sweep.
func relayCloudDevices(cloudClient, mainClient mqtt.Client, devicesFile TDevicesFile, tracker *TCloudLivenessTracker) error {
	for deviceID, device := range devicesFile.Devices {
		qualifier, needed := needsCloudRelay(device)
		if !needed {
			continue
		}
		if cloudClient == nil {
			fmt.Printf("[relay] device %q needs cloud-broker relay but no cloud broker is configured; skipping\n", deviceID)
			continue
		}
		if err := relayCloudDevice(cloudClient, mainClient, deviceID, device, qualifier, tracker); err != nil {
			return err
		}
	}
	tracker.StartSweeping(mainClient, cloudClient, devicesFile.Devices)
	return nil
}

// subscribeCloudOnlyDeviceInfo subscribes on cloudClient to every "cloud"-but-not-imported
// device's own installation-qualified device/state topic directly (there is no relay onto main
// for these -- see this file's own doc comment -- so subscribeDeviceInfo's main-side
// subscription never sees them), updating store and republishing that device's discovery
// (routed to cloud only, deviceRoutingPlan) on each update -- the cloud-only equivalent of
// subscribeDeviceInfo. No-op (returns nil immediately) if devicesFile has no such device. A
// self-imported device (Cloud && ImportedFrom != "") is excluded here -- its device-info instead
// relays via needsCloudRelay/relayCloudDevice's ImportedFrom branch, same as any other import.
func subscribeCloudOnlyDeviceInfo(cloudClient, mainClient mqtt.Client, ownInstallation string, devicesFile TDevicesFile, store *TLiveDeviceInfoStore, hostNameToDeviceID map[string]string, publisher *TDiscoveryPublisher) error {
	prefix := devicesFile.conceptualPrefix()
	topicToDeviceID := map[string]string{}
	for deviceID, device := range devicesFile.Devices {
		if device.Cloud && device.ImportedFrom == "" && device.DeviceInfoTopic != "" {
			topicToDeviceID[qualifyHostsTopic(device.DeviceInfoTopic, ownInstallation)] = deviceID
		}
	}
	if len(topicToDeviceID) == 0 {
		return nil
	}

	handler := func(_ mqtt.Client, msg mqtt.Message) {
		var fields map[string]string
		if err := json.Unmarshal(msg.Payload(), &fields); err != nil {
			fmt.Printf("[device-info:cloud] %s: cannot parse payload: %v\n", msg.Topic(), err)
			return
		}
		deviceID, known := topicToDeviceID[msg.Topic()]
		if !known {
			return
		}
		store.Update(deviceID, fields)

		device, known := devicesFile.Devices[deviceID]
		if !known {
			return
		}
		if _, err := publishDeviceDiscovery(mainClient, cloudClient, ownInstallation, deviceID, device, store, hostNameToDeviceID, publisher, prefix); err != nil {
			fmt.Printf("[device-info:cloud] %s: republishing discovery: %v\n", deviceID, err)
		}
	}

	for topic := range topicToDeviceID {
		tok := cloudClient.Subscribe(topic, 0, handler)
		if !tok.WaitTimeout(10*time.Second) || tok.Error() != nil {
			if err := tok.Error(); err != nil {
				return fmt.Errorf("subscribing to %s: %w", topic, err)
			}
			return fmt.Errorf("subscribing to %s: timed out", topic)
		}
	}
	return nil
}
