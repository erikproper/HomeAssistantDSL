/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: DiscoveryCommandline
 *
 * Publishes Home Assistant MQTT Discovery configs for the "commandline" integration's declared
 * devices (homeassistant/integration_commandline_generator.go's coordinator/commandline.yaml) --
 * script-backed switch/sensor/button entities served by the mqtt_commandline daemon
 * (Integrations/mqtt_commandline/) running on the target host.
 *
 * Unlike "hosts"/"home_assistant"/"discovery", this integration needs no live-reported metadata,
 * no cross-house relay, and no conceptual-layer positioning: every field a discovery config needs
 * is already fully known from commandline.yaml alone, and Physical.def's own build plan (PROJECT.md
 * item 1) deliberately has the switch/button discovery config's own command_topic point DIRECTLY
 * at the daemon's own topic -- HA publishes there straight from the UI, no coordinator-side command
 * relay needed, exactly like a "hosts"-kind sensor's state_topic already points straight at the
 * reporting host with no relay for reads either. This file therefore only ever PUBLISHES discovery
 * configs once at startup (retained); it never subscribes to anything.
 *
 * Liveness: every entity on a device is gated by that device's own commandline/<host>/node/state
 * topic (the daemon's own MQTT Last Will, "true" on connect / "false" on disconnect -- see
 * PROJECT.md item 1's own liveness design), reusing buildAvailabilityFields' shared two-factor
 * pattern is unnecessary here (a commandline device has only ONE factor -- its own node topic, no
 * separately-fallible upstream entity to AND against the way hassbridge/import proxies do) --
 * TSensorDiscoveryPayload/TBinarySensorDiscoveryPayload's own plain AvailabilityTopic field
 * (discovery.go) already covers this.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 06.09.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gopkg.in/yaml.v3"
)

// TCommandlineCapabilityRef is one capability's optional Spaces.def positioning (registered via
// "entity <spec> from <device-id> entity <capability>;", homeassistant/
// Conceptual_CommandlineEntities.go) -- both fields empty when the capability was never
// positioned, in which case buildCommandlineDiscoveryConfigs falls back to its own auto-derived
// naming (unchanged from before positioning support existed).
type TCommandlineCapabilityRef struct {
	EntityID string `yaml:"entity_id,omitempty"`
	Name     string `yaml:"name,omitempty"`
}

// TCommandlineDevice is one entry under "devices:" in a generated coordinator/commandline.yaml
// file -- Switches/Sensors/Buttons are keyed by each capability's own declared local name (e.g.
// "slideshow", "reboot"); no script content ever appears here, since the coordinator only ever
// relays commandline/<host>/<entity>/... topics, never invokes a script itself.
type TCommandlineDevice struct {
	Host     string                               `yaml:"host"`
	Switches map[string]TCommandlineCapabilityRef `yaml:"switches,omitempty"`
	Sensors  map[string]TCommandlineCapabilityRef `yaml:"sensors,omitempty"`
	Buttons  map[string]TCommandlineCapabilityRef `yaml:"buttons,omitempty"`
}

// TCommandlineFile is the top-level shape of a generated coordinator/commandline.yaml file --
// absent entirely when a house has no "commandline" integration declared; loadCommandlineFile
// treats that as "nothing to publish," not an error, mirroring loadDiscoveryFile/loadHassBridgeFile's
// own "missing file -> zero value" convention.
type TCommandlineFile struct {
	Devices map[string]TCommandlineDevice `yaml:"devices"`
}

// loadCommandlineFile reads and parses a generator-produced coordinator/commandline.yaml file, if
// one exists.
func loadCommandlineFile(path string) (TCommandlineFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return TCommandlineFile{}, nil
		}
		return TCommandlineFile{}, fmt.Errorf("cannot read %s: %w", path, err)
	}

	var commandlineFile TCommandlineFile
	if err := yaml.Unmarshal(data, &commandlineFile); err != nil {
		return TCommandlineFile{}, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	return commandlineFile, nil
}

// stringPtr returns a pointer to s -- TBinarySensorDiscoveryPayload.Name is *string (nil marshals
// to JSON null, meaning "device.name alone, no combination" -- see that type's own doc comment),
// so a genuinely distinguishing name (this entity is one of two liveness signals sharing a device,
// not "the device itself") needs an addressable non-nil value.
func stringPtr(s string) *string { return &s }

// commandlineNodeTopic is the daemon's own LWT-backed liveness topic for host -- gates every
// entity declared on that host, same "device's node topic gates every other capability" rule
// hosts/hassbridge/imports all already follow (availabilityTopicFor, discovery.go).
func commandlineNodeTopic(host string) string {
	return "commandline/" + host + "/node/state"
}

// commandlineStateTopic/commandlineCommandTopic/commandlinePressTopic are the wire topics the
// mqtt_commandline daemon publishes/subscribes on for one entity -- see Integrations/
// mqtt_commandline's own doc comment for the full wire shape (PROJECT.md item 1).
func commandlineStateTopic(host, name string) string {
	return "commandline/" + host + "/" + name + "/state"
}
func commandlineCommandTopic(host, name string) string {
	return "commandline/" + host + "/" + name + "/set"
}
func commandlinePressTopic(host, name string) string {
	return "commandline/" + host + "/" + name + "/press"
}

// commandlineCapabilityNames returns refs' keys, sorted -- shared by all three capability kinds'
// own iteration in buildCommandlineDiscoveryConfigs.
func commandlineCapabilityNames(refs map[string]TCommandlineCapabilityRef) []string {
	names := make([]string, 0, len(refs))
	for name := range refs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// resolveCommandlineNaming returns a capability's entity id and display name: ref's own
// Spaces.def-positioned values when set (homeassistant/Conceptual_CommandlineEntities.go), else
// the auto-derived defaults this integration always used before positioning support existed --
// "<domain>.<host>_<name>" and "<host>/<name>".
func resolveCommandlineNaming(host, name, domain string, ref TCommandlineCapabilityRef) (entityID, displayName string) {
	entityID = ref.EntityID
	if entityID == "" {
		entityID = domain + "." + sanitizeTopicSegment(host) + "_" + name
	}
	displayName = ref.Name
	if displayName == "" {
		displayName = host + "/" + name
	}
	return entityID, displayName
}

// buildCommandlineDiscoveryConfigs returns every HA MQTT Discovery config implied by one
// commandline.yaml device: its own daemon-liveness entity, plus one switch/sensor/button config
// per declared capability. Mirrors buildDiscoveryConfigs' (discovery.go) device-block/unique_id/
// topic conventions so entities from a "hosts"-kind declaration sharing the SAME deviceID (e.g. a
// picture frame that's both "host.frame" for ping/cpu AND "host.frame" for commandline) merge into
// one HA device rather than appearing as two -- both use identical device.Identifiers.
//
// The liveness entity's own unique_id is deliberately "<deviceID>_commandline_node", NOT
// "<deviceID>_node" -- a real bug found live 2026-09-06: a "hosts"-kind declaration sharing the
// same deviceID (as the doc comment above assumes) already owns "<deviceID>_node" for its own
// ping-based liveness signal. Reusing that exact unique_id here made the two integrations fight
// over one HA entity, each one's own periodic republish (hosts' on every live device-info report,
// commandline's at startup) clobbering the other's payload every time -- confirmed live via
// [discovery-cleanup] repeatedly retiring/republishing homeassistant/binary_sensor/coordinator/
// host_frame_node/config. The two liveness signals are semantically different (host reachable vs.
// this specific daemon process alive, PROJECT.md item 1's own stated rationale for using LWT here
// instead of ping-style staleness) and must get their own separate HA entities; only the shared
// Device.Identifiers should be reused, grouping them visually under the one physical device.
func buildCommandlineDiscoveryConfigs(deviceID string, device TCommandlineDevice, prefix string) []TDiscoveryConfig {
	devBlock := TDiscoveryDevice{Identifiers: []string{deviceID}, Name: device.Host}
	nodeTopic := commandlineNodeTopic(device.Host)

	var configs []TDiscoveryConfig

	nodeUniqueID := deviceID + "_commandline_node"
	configs = append(configs, TDiscoveryConfig{
		Topic: discoveryTopic(prefix, "binary_sensor", nodeUniqueID),
		Payload: TBinarySensorDiscoveryPayload{
			UniqueID:        nodeUniqueID,
			DefaultEntityID: "binary_sensor." + sanitizeTopicSegment(device.Host) + "_commandline_node",
			Name:            stringPtr(device.Host + "/commandline"),
			StateTopic:      nodeTopic,
			PayloadOn:       "true",
			PayloadOff:      "false",
			DeviceClass:     "connectivity",
			Device:          devBlock,
		},
	})

	switches := commandlineCapabilityNames(device.Switches)
	for _, name := range switches {
		uniqueID := deviceID + "_" + name
		entityID, displayName := resolveCommandlineNaming(device.Host, name, "switch", device.Switches[name])
		configs = append(configs, TDiscoveryConfig{
			Topic: discoveryTopic(prefix, "switch", uniqueID),
			Payload: TCommandlineSwitchDiscoveryPayload{
				UniqueID:          uniqueID,
				DefaultEntityID:   entityID,
				Name:              displayName,
				StateTopic:        commandlineStateTopic(device.Host, name),
				CommandTopic:      commandlineCommandTopic(device.Host, name),
				PayloadOn:         "1",
				PayloadOff:        "0",
				StateOn:           "1",
				StateOff:          "0",
				AvailabilityTopic: nodeTopic,
				PayloadAvailable:  "true",
				PayloadNotAvail:   "false",
				Device:            devBlock,
			},
		})
	}

	sensors := commandlineCapabilityNames(device.Sensors)
	for _, name := range sensors {
		uniqueID := deviceID + "_" + name
		entityID, displayName := resolveCommandlineNaming(device.Host, name, "sensor", device.Sensors[name])
		configs = append(configs, TDiscoveryConfig{
			Topic: discoveryTopic(prefix, "sensor", uniqueID),
			Payload: TSensorDiscoveryPayload{
				UniqueID:            uniqueID,
				DefaultEntityID:     entityID,
				Name:                displayName,
				StateTopic:          commandlineStateTopic(device.Host, name),
				AvailabilityTopic:   nodeTopic,
				PayloadAvailable:    "true",
				PayloadNotAvailable: "false",
				Device:              devBlock,
			},
		})
	}

	buttons := commandlineCapabilityNames(device.Buttons)
	for _, name := range buttons {
		uniqueID := deviceID + "_" + name
		entityID, displayName := resolveCommandlineNaming(device.Host, name, "button", device.Buttons[name])
		configs = append(configs, TDiscoveryConfig{
			Topic: discoveryTopic(prefix, "button", uniqueID),
			Payload: TCommandlineButtonDiscoveryPayload{
				UniqueID:          uniqueID,
				DefaultEntityID:   entityID,
				Name:              displayName,
				CommandTopic:      commandlinePressTopic(device.Host, name),
				PayloadPress:      "PRESS",
				AvailabilityTopic: nodeTopic,
				PayloadAvailable:  "true",
				PayloadNotAvail:   "false",
				Device:            devBlock,
			},
		})
	}

	return configs
}

// TCommandlineSwitchDiscoveryPayload is the HA MQTT Discovery config for a commandline switch
// entity. PayloadOn/PayloadOff/StateOn/StateOff are "1"/"0" -- matching the existing, deployed
// check_slideshow-style status scripts' own established convention (PROJECT.md item 1: "status,
// 0/1") verbatim, so no existing script needs rewriting for this integration to use it.
type TCommandlineSwitchDiscoveryPayload struct {
	UniqueID          string           `json:"unique_id"`
	DefaultEntityID   string           `json:"default_entity_id"`
	Name              string           `json:"name"`
	StateTopic        string           `json:"state_topic"`
	CommandTopic      string           `json:"command_topic"`
	PayloadOn         string           `json:"payload_on"`
	PayloadOff        string           `json:"payload_off"`
	StateOn           string           `json:"state_on"`
	StateOff          string           `json:"state_off"`
	AvailabilityTopic string           `json:"availability_topic,omitempty"`
	PayloadAvailable  string           `json:"payload_available,omitempty"`
	PayloadNotAvail   string           `json:"payload_not_available,omitempty"`
	Device            TDiscoveryDevice `json:"device"`
}

// TCommandlineButtonDiscoveryPayload is the HA MQTT Discovery config for a commandline button
// entity -- no state at all, matching HA's own MQTT button semantics.
type TCommandlineButtonDiscoveryPayload struct {
	UniqueID          string           `json:"unique_id"`
	DefaultEntityID   string           `json:"default_entity_id"`
	Name              string           `json:"name"`
	CommandTopic      string           `json:"command_topic"`
	PayloadPress      string           `json:"payload_press"`
	AvailabilityTopic string           `json:"availability_topic,omitempty"`
	PayloadAvailable  string           `json:"payload_available,omitempty"`
	PayloadNotAvail   string           `json:"payload_not_available,omitempty"`
	Device            TDiscoveryDevice `json:"device"`
}

// publishCommandlineDiscovery publishes every commandline.yaml device's discovery configs once,
// through publisher -- local ("main" broker) only, matching PROJECT.md item 1's own scope (no
// cloud routing designed for this integration yet).
func publishCommandlineDiscovery(client mqtt.Client, commandlineFile TCommandlineFile, publisher *TDiscoveryPublisher, prefix string) error {
	published := 0
	deviceIDs := make([]string, 0, len(commandlineFile.Devices))
	for id := range commandlineFile.Devices {
		deviceIDs = append(deviceIDs, id)
	}
	sort.Strings(deviceIDs)
	for _, id := range deviceIDs {
		for _, cfg := range buildCommandlineDiscoveryConfigs(id, commandlineFile.Devices[id], prefix) {
			data, err := json.Marshal(cfg.Payload)
			if err != nil {
				return fmt.Errorf("marshalling discovery payload for %s: %w", cfg.Topic, err)
			}
			if err := publisher.Publish(client, "main", cfg.Topic, data); err != nil {
				return err
			}
			published++
		}
	}
	fmt.Printf("[commandline] published %d discovery config(s)\n", published)
	return nil
}

// expectedCommandlineTopics computes every discovery topic commandlineFile implies, for
// watchForOrphanedDiscoveryTopics' orphan-cleanup set -- topic existence only (not exact expected
// content), matching the discovery-bridge/hassbridge/imported kinds' own simpler convention rather
// than "hosts"' deeper content-aware check.
func expectedCommandlineTopics(commandlineFile TCommandlineFile, prefix string) map[string]bool {
	expected := map[string]bool{}
	for id, device := range commandlineFile.Devices {
		for _, cfg := range buildCommandlineDiscoveryConfigs(id, device, prefix) {
			expected[cfg.Topic] = true
		}
	}
	return expected
}
