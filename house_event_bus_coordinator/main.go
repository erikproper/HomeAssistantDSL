/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: Main
 *
 * The coordinator described in Architecture.md §6: a model-aware runtime component sitting
 * between integrations/infrastructure and the Home Assistant instance(s). This first real
 * version covers role 2, discovery refinement (§6.1): it reads devices.yaml + secrets.yaml
 * (both generated alongside each other by homeassistant/integration_hosts_generator.go),
 * connects to the house's MQTT broker, publishes Home Assistant MQTT Discovery config for
 * every device with a conceptual link (endpoint.go, discovery.go), and logs traffic on the
 * "hosts/#" topics as a debug/sanity aid. It does not bridge or republish device state itself
 * -- Home Assistant subscribes directly to the same topics the discovery payloads point at.
 * Local infrastructure coordination (§6.1 role 1) and cross-home federation (role 3) are not
 * built yet.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 21.08.2026
 *
 */

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gopkg.in/yaml.v3"
)

// TDeviceCapability is one capability entry under a device in devices.yaml, e.g. "load"
// mapping to a source Home Assistant entity id.
type TDeviceCapability struct {
	Entity string `yaml:"entity"`
}

// TDeviceConceptual is a device's "conceptual:" block in devices.yaml -- the HA entity ids an
// "entity device.<spec> from <device-id> ...;" declaration in Spaces.def implies for it, plus
// the naming/typing metadata its integration type hardwires (DisplayName, ConstantAttributes
// defaults/overrides, per-attribute device_class/unit/state_class). Absent (nil) for devices
// with no such declaration yet -- most devices today.
type TDeviceConceptual struct {
	NodeEntity string `yaml:"node_entity"`
	// NodeDeviceClass/NodeIcon are the node entity's own typing metadata -- resolved generator-
	// side from Defaults.def/a house's own Physical.def "defaults: for ...;" rules
	// (resolveCapabilityDefaults), not coordinator-invented. Empty when no rule matches.
	NodeDeviceClass    string                          `yaml:"node_device_class,omitempty"`
	NodeIcon           string                          `yaml:"node_icon,omitempty"`
	DisplayName        string                          `yaml:"display_name,omitempty"`
	ConstantAttributes map[string]TConceptualConstant  `yaml:"constant_attributes,omitempty"`
	AttributeEntities  map[string]TConceptualAttribute `yaml:"attribute_entities,omitempty"`
}

// TConceptualConstant is one HA discovery device-map field's resolved value (e.g. "model" ->
// "Compute host", or a per-device override like "hw_version" -> "1928930"). Forced means the
// value must never be overwritten by live-reported data (once that mechanism exists).
type TConceptualConstant struct {
	Value  string `yaml:"value"`
	Forced bool   `yaml:"forced,omitempty"`
}

// TConceptualAttribute is one variable attribute's HA entity id plus the typing metadata its
// integration type hardwires for it (device_class/unit/state_class/icon -- e.g. "temperature"
// reports device_class "temperature", unit "°C"). No Name: the coordinator always derives the
// attribute's displayed name from its own leaf key (capitalize(attr), discovery.go) now that HA
// combines it with the device's own name automatically -- see discovery.go's doc comments.
type TConceptualAttribute struct {
	Entity      string `yaml:"entity"`
	DeviceClass string `yaml:"device_class,omitempty"`
	Unit        string `yaml:"unit,omitempty"`
	StateClass  string `yaml:"state_class,omitempty"`
	Icon        string `yaml:"icon,omitempty"`
}

// TDevice is one entry under "devices:" in a generated devices.yaml file. Topic and NodeTopic
// are the MQTT topics ("hosts/<host>/cpu/state", "hosts/<host>/node/state") this device's
// variable attributes and liveness are reported on -- Topic may be empty for a device type
// with no state topic (e.g. ping, liveness-only).
type TDevice struct {
	Host            string                       `yaml:"host"`
	Integration     string                       `yaml:"integration"`
	Topic           string                       `yaml:"topic"`
	NodeTopic       string                       `yaml:"node_topic"`
	DeviceInfoTopic string                       `yaml:"device_info_topic,omitempty"`
	Capabilities    map[string]TDeviceCapability `yaml:"capabilities"`
	Conceptual      *TDeviceConceptual           `yaml:"conceptual,omitempty"`
	// Cloud/Local/ImportedFrom are the "cloud"/"local"/"integration import" routing this device
	// declared in Physical.def (homeassistant/mqtt_routing_keywords.go,
	// homeassistant/integration_import_parser.go) -- see mqtt_relay.go's own doc comment for the
	// full routing table this drives (which broker(s) get this device's discovery config, and
	// whether its state/device-info traffic needs relaying from the cloud broker onto main).
	Cloud        bool   `yaml:"cloud,omitempty"`
	Local        bool   `yaml:"local,omitempty"`
	ImportedFrom string `yaml:"imported_from,omitempty"`
}

// TDevicesFile is the top-level shape of a generated devices.yaml file. MQTTDiscoveryConceptualPrefix
// is the prefix this coordinator publishes its own conceptual-layer discovery under (resolved
// from ${mqtt_discovery_conceptual}, generator-side; defaults to "homeassistant" -- HA's own
// default discovery prefix -- when a devices.yaml predates this field or the setting is unset).
// Installation is this house's own ${installation} setting -- "" if unset (a house that hasn't
// adopted the cloud broker "cloud"/"local"/"import" mechanism yet) -- used to qualify any
// "cloud"-routed device's own topics on the shared cloud broker (mqtt_relay.go).
type TDevicesFile struct {
	Devices                       map[string]TDevice `yaml:"devices"`
	MQTTDiscoveryConceptualPrefix string             `yaml:"mqtt_discovery_conceptual_prefix"`
	Installation                  string             `yaml:"installation"`
}

// conceptualPrefix returns f's MQTTDiscoveryConceptualPrefix, or "homeassistant" if unset.
func (f TDevicesFile) conceptualPrefix() string {
	if f.MQTTDiscoveryConceptualPrefix != "" {
		return f.MQTTDiscoveryConceptualPrefix
	}
	return "homeassistant"
}

// TDiscoveryGateway is one "discovery" integration gateway device from devices.yaml's sibling
// coordinator/discovery.yaml -- an externally discovered MQTT device (e.g. EMS-ESP) that
// publishes its own native HA MQTT Discovery under the physical-discovery prefix. Identifiers
// matches an incoming payload's own device identifiers or (one hop) its via_device --
// discoverybridge.go.
type TDiscoveryGateway struct {
	Identifiers []string `yaml:"identifiers"`
}

// TDiscoveryEntityLink is one "entity <spec> from <gateway-id>.<leaf>;" link from devices.yaml's
// sibling coordinator/discovery.yaml -- our own entity id (the map key) sourced from Gateway's
// Leaf entity.
type TDiscoveryEntityLink struct {
	Gateway string `yaml:"gateway"`
	Leaf    string `yaml:"leaf"`

	// DeviceClass/Unit/StateClass/Icon are generator-resolved gap-fillers (Defaults.def, via
	// resolveCapabilityDefaults) -- discoverybridge.go's buildRelayedDiscoveryConfig only uses
	// these when the gateway's own natively-published discovery payload leaves the field empty,
	// never overriding a value the gateway already reported.
	DeviceClass string `yaml:"device_class,omitempty"`
	Unit        string `yaml:"unit,omitempty"`
	StateClass  string `yaml:"state_class,omitempty"`
	Icon        string `yaml:"icon,omitempty"`
}

// TDiscoveryFile is the top-level shape of a generated coordinator/discovery.yaml file --
// separate from devices.yaml since the "hosts" and "discovery" Physical.def integrations are
// generated independently (homeassistant/integration_discovery_generator.go's own doc comment
// explains why they can't share one file). Absent entirely when a house has no "discovery"
// integration declared -- loadDiscoveryFile treats that as "nothing to bridge," not an error.
type TDiscoveryFile struct {
	PhysicalPrefix string                          `yaml:"physical_prefix"`
	Gateways       map[string]TDiscoveryGateway    `yaml:"gateways"`
	EntityLinks    map[string]TDiscoveryEntityLink `yaml:"entity_links"`
}

func main() {
	dryRun := flag.Bool("dry-run", false, "print discovery topics/payloads instead of connecting and publishing")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: house_event_bus_coordinator [-dry-run] <path/to/coordinator>\n")
		os.Exit(1)
	}
	coordinatorDir := flag.Arg(0)

	devicesFile, err := loadDevicesFile(filepath.Join(coordinatorDir, "devices.yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	printDevicesSummary(devicesFile)

	if *dryRun {
		printDiscoveryDryRun(devicesFile)
		return
	}

	secrets, err := loadSecretsFile(filepath.Join(coordinatorDir, "secrets.yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	discoveryFile, err := loadDiscoveryFile(filepath.Join(coordinatorDir, "discovery.yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	cleanupFile, err := loadDiscoveryCleanupFile(filepath.Join(coordinatorDir, "discovery_cleanup.yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	hassBridgeFile, err := loadHassBridgeFile(filepath.Join(coordinatorDir, "homeassistant_bridge.yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	homeAssistantInstancesFile, err := loadHomeAssistantInstancesFile(filepath.Join(coordinatorDir, "home_assistant_instances.yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	client, err := connectMQTT(secrets.MQTT, "house_event_bus_coordinator")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer client.Disconnect(250)

	// This coordinator only supports one secondary (cloud) broker connection at a time --
	// today's real need (a house's own "cloud"+"local"/imported devices, mqtt_relay.go). A
	// house declaring more than one "coordinator_only true;" profile in Physical.def is an
	// unsupported configuration for now; warn and use none of them rather than guessing.
	var cloudClient mqtt.Client
	switch len(secrets.Brokers) {
	case 0:
		// nothing to connect -- no house-declared "cloud"/"local"/import routing yet.
	case 1:
		for name, brokerSecrets := range secrets.Brokers {
			cloudClient, err = connectMQTT(brokerSecrets, "house_event_bus_coordinator-"+name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: connecting to %q broker: %v\n", name, err)
				os.Exit(1)
			}
			defer cloudClient.Disconnect(250)
		}
	default:
		fmt.Printf("[main] %d coordinator-only broker profiles declared; only one secondary broker is supported today, connecting to none of them\n", len(secrets.Brokers))
	}

	store := NewLiveDeviceInfoStore()
	topicToDeviceID, hostNameToDeviceID := buildDeviceLookups(devicesFile)

	conceptualPrefix := devicesFile.conceptualPrefix()
	expectedHostsContent, err := expectedHostsPayloads(devicesFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	expectedTopics := topicSet(expectedHostsContent)
	for t := range expectedDiscoveryBridgeTopics(discoveryFile, conceptualPrefix) {
		expectedTopics[t] = true
	}
	for t := range expectedHassBridgeTopics(hassBridgeFile, conceptualPrefix) {
		expectedTopics[t] = true
	}
	// The "Discover entity" meta control (discover_entity_input.go) is permanent, coordinator-owned,
	// and not derived from any devices/discovery/hassbridge file -- must be listed here explicitly,
	// or watchForOrphanedDiscoveryTopics (below) retires it moments after publishDiscoverEntityInput
	// (further down this function) ever publishes it, since its own "coordinator"-owned topic would
	// otherwise never appear "expected". Confirmed live 2026-08-28: without this, the entity never
	// stayed up long enough for HA to show it at all.
	expectedTopics[discoveryTopic(conceptualPrefix, "text", discoverEntityStableID)] = true

	declaredCleanNodeIDs := make(map[string]bool, len(cleanupFile.CleanTopics))
	for _, t := range cleanupFile.CleanTopics {
		declaredCleanNodeIDs[t] = true
	}

	publisher := newDiscoveryPublisher(filepath.Join(coordinatorDir, "discovery_topics.json"))
	publisher.RetireMissing(client, "main", expectedTopics)
	if err := watchForOrphanedDiscoveryTopics(client, expectedTopics, expectedHostsContent, declaredCleanNodeIDs, conceptualPrefix); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Cloud-broker discovery cleanup mirrors main's own, above, but scoped strictly to this
	// installation's own "cloud"-routed topics (expectedCloudHostsPayloads, discoverycleanup.go)
	// under its own installation-qualified prefix -- watchForOrphanedDiscoveryTopics' subscribe
	// filter ("<ownInstallation>/<conceptualPrefix>/+/+/+/config") never matches another
	// installation's own qualified topics on the same shared broker, so this coordinator only
	// ever cleans up after itself, never another installation sharing the broker.
	if cloudClient != nil && devicesFile.Installation == "" {
		fmt.Println("[main] a cloud broker is configured but devices.yaml has no \"installation\" set (${installation} in Settings.def) -- cloud-side topics would be malformed, refusing to publish/clean them up")
		cloudClient = nil
	}
	if cloudClient != nil {
		expectedCloudContent, err := expectedCloudHostsPayloads(devicesFile, devicesFile.Installation)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		expectedCloudTopics := topicSet(expectedCloudContent)
		publisher.RetireMissing(cloudClient, "cloud_coordinator", expectedCloudTopics)
		cloudPrefix := devicesFile.Installation + "/" + conceptualPrefix
		if err := watchForOrphanedDiscoveryTopics(cloudClient, expectedCloudTopics, expectedCloudContent, declaredCleanNodeIDs, cloudPrefix); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	if err := publishDiscovery(client, cloudClient, devicesFile.Installation, devicesFile, store, hostNameToDeviceID, publisher); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if err := subscribeDebugLogger(client, "main"); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if err := subscribeDeviceInfo(client, cloudClient, devicesFile.Installation, devicesFile, store, topicToDeviceID, hostNameToDeviceID, publisher); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if cloudClient != nil {
		if err := subscribeDebugLogger(cloudClient, "cloud_coordinator"); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if err := relayCloudDevices(cloudClient, client, devicesFile, newCloudLivenessTracker()); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if err := subscribeCloudOnlyDeviceInfo(cloudClient, client, devicesFile.Installation, devicesFile, store, hostNameToDeviceID, publisher); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}
	if discoveryFile.PhysicalPrefix != "" {
		// PROJECT.md 1.8: kind-2's own passive counterpart to kind-3's entity-existence inquiry --
		// no active inquiry needed (a gateway self-announces), but the coordinator still needs to
		// track and report each declared source leaf's status, seeded not-known-to-exist from
		// discovery.yaml's own EntityLinks, populated as a byproduct of the discovery-bridge
		// subscription below.
		discoveryExistenceTracker := newDiscoveryExistenceTracker(filepath.Join(coordinatorDir, "discovery_existence.json"))
		discoveryExistenceTracker.Seed(discoveryFile)
		if err := subscribeDiscoveryBridge(client, cloudClient, discoveryFile, publisher, conceptualPrefix, discoveryExistenceTracker); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}
	if err := subscribeHassBridge(client, hassBridgeFile, store, publisher, conceptualPrefix); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if err := subscribeHassBridgeDeviceInfo(client, hassBridgeFile, store, publisher, conceptualPrefix); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Entity-existence inquiry (PROJECT.md 1.1): seeded from hassBridgeFile (every declared
	// capability's source entity is "assumed to exist" per Physical.def/Spaces.def), paced-inquired
	// one at a time per named instance -- applies to every declared instance, "main" included, not
	// just remote bridge sources (that's the whole point of asking one entity at a time instead of
	// scanning every state, unlike the disabled coordinator_bootstrap mechanism).
	existenceTracker := newEntityExistenceTracker(filepath.Join(coordinatorDir, "entity_existence.json"))
	existenceTracker.Seed(hassBridgeFile)
	if err := existenceTracker.StartEntityExistenceInquiries(client, cloudClient, homeAssistantInstancesFile.Instances); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Architecture.md §6.9: bootstrapping an entirely new, undeclared device needs a human-provided
	// seed -- DiscoverSiblings can only enrich a device the tracker already has some anchor for.
	if err := publishDiscoverEntityInput(client, conceptualPrefix); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[existence] published \"Discover entity\" meta control at %s\n", discoveryTopic(conceptualPrefix, "text", discoverEntityStableID))
	if err := subscribeDiscoverEntityRequests(client, existenceTracker, homeAssistantInstancesFile.Instances); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("coordinator running -- Ctrl-C to stop")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
}

// buildDeviceLookups builds the two reverse lookups discovery/live-info handling needs:
// topicToDeviceID (a device's DeviceInfoTopic -> its deviceID, for routing an incoming
// ".../device/state" message back to the device it's about) and hostNameToDeviceID (a device's
// Host -> its deviceID, for resolving a reported "via_device" hostname to that host's own
// hosts-integration device identifier, if it has one).
func buildDeviceLookups(devicesFile TDevicesFile) (topicToDeviceID, hostNameToDeviceID map[string]string) {
	topicToDeviceID = map[string]string{}
	hostNameToDeviceID = map[string]string{}
	for id, device := range devicesFile.Devices {
		if device.DeviceInfoTopic != "" {
			topicToDeviceID[device.DeviceInfoTopic] = id
		}
		if device.Host != "" {
			hostNameToDeviceID[device.Host] = id
		}
	}
	return topicToDeviceID, hostNameToDeviceID
}

// loadDiscoveryFile reads and parses a generator-produced coordinator/discovery.yaml file, if
// one exists -- returns a zero-value TDiscoveryFile (not an error) when the file is absent, since
// most houses don't declare a "discovery" integration.
func loadDiscoveryFile(path string) (TDiscoveryFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return TDiscoveryFile{}, nil
		}
		return TDiscoveryFile{}, fmt.Errorf("cannot read %s: %w", path, err)
	}

	var discoveryFile TDiscoveryFile
	if err := yaml.Unmarshal(data, &discoveryFile); err != nil {
		return TDiscoveryFile{}, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	return discoveryFile, nil
}

// loadDevicesFile reads and parses a generator-produced devices.yaml file.
func loadDevicesFile(path string) (TDevicesFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TDevicesFile{}, fmt.Errorf("cannot read %s: %w", path, err)
	}

	var devicesFile TDevicesFile
	if err := yaml.Unmarshal(data, &devicesFile); err != nil {
		return TDevicesFile{}, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	return devicesFile, nil
}

// printDevicesSummary prints one line per device plus a per-integration-type count, as a
// sanity check that the devices.yaml contract between generator and coordinator holds.
func printDevicesSummary(devicesFile TDevicesFile) {
	deviceIDs := make([]string, 0, len(devicesFile.Devices))
	for id := range devicesFile.Devices {
		deviceIDs = append(deviceIDs, id)
	}
	sort.Strings(deviceIDs)

	countByIntegration := map[string]int{}
	for _, id := range deviceIDs {
		device := devicesFile.Devices[id]
		countByIntegration[device.Integration]++

		capNames := make([]string, 0, len(device.Capabilities))
		for name := range device.Capabilities {
			capNames = append(capNames, name)
		}
		sort.Strings(capNames)

		fmt.Printf("%-40s host=%-30s integration=%-14s topic=%-24s node_topic=%-24s capabilities=%v\n",
			id, device.Host, device.Integration, device.Topic, device.NodeTopic, capNames)
	}

	fmt.Printf("\n%d devices total\n", len(deviceIDs))
	integrations := make([]string, 0, len(countByIntegration))
	for name := range countByIntegration {
		integrations = append(integrations, name)
	}
	sort.Strings(integrations)
	for _, name := range integrations {
		fmt.Printf("  %s: %d\n", name, countByIntegration[name])
	}
}
