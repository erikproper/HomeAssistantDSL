/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: DiscoveryBridge
 *
 * Bridges externally discovered MQTT devices (e.g. EMS-ESP, a heating/boiler gateway that
 * already publishes its own complete, correct Home Assistant MQTT Discovery entirely on its
 * own) into our own conceptual entity ids. Such gateways are configured (outside this repo, in
 * their own settings) to publish discovery under a dedicated "physical" prefix
 * (TDiscoveryFile.PhysicalPrefix, resolved from ${mqtt_discovery_physical}) instead of the
 * default "homeassistant" prefix HA itself subscribes to -- so HA never sees a gateway's raw,
 * unpositioned discovery directly. This file subscribes to that prefix, and for every entity
 * the DSL referenced via "entity <spec> from <gateway-id>.<leaf>;"
 * (homeassistant/Conceptual_DiscoveryEntities.go, TDiscoveryFile.EntityLinks), republishes our
 * own discovery config -- our entity id, but state_topic/value_template/device_class/etc.
 * learned from the gateway's own payload -- onto the real "homeassistant/" prefix. Pure
 * discovery-config relay: no state is ever re-published, the coordinator just points our
 * discovery config at the gateway's own already-published state topic.
 *
 * Only the small set of HA MQTT Discovery abbreviations (see
 * home-assistant/core's mqtt/abbreviations.py) this needs to read are decoded -- extend on
 * demand as real gateway payloads need more fields relayed, not by front-loading the full table.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 22.08.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gopkg.in/yaml.v3"
)

// TDiscoveryPassthroughRule is one Physical.def "discovery_passthrough "<topic-prefix>" from
// "<source-prefix>";" declaration (homeassistant/discovery_passthrough.go).
type TDiscoveryPassthroughRule struct {
	TopicPrefix  string `yaml:"topic_prefix"`
	SourcePrefix string `yaml:"source_prefix"`
}

// TDiscoveryPassthroughFile is the top-level shape of a generated
// coordinator/discovery_passthrough.yaml file. Absent entirely when a house has no
// discovery_passthrough statements declared -- loadDiscoveryPassthroughFile treats that as
// "nothing to pass through," not an error, same convention as loadDiscoveryFile/
// loadDiscoveryCleanupFile.
type TDiscoveryPassthroughFile struct {
	Passthrough []TDiscoveryPassthroughRule `yaml:"passthrough"`
}

// loadDiscoveryPassthroughFile reads and parses a generator-produced
// coordinator/discovery_passthrough.yaml file, if one exists.
func loadDiscoveryPassthroughFile(path string) (TDiscoveryPassthroughFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return TDiscoveryPassthroughFile{}, nil
		}
		return TDiscoveryPassthroughFile{}, fmt.Errorf("cannot read %s: %w", path, err)
	}

	var passthroughFile TDiscoveryPassthroughFile
	if err := yaml.Unmarshal(data, &passthroughFile); err != nil {
		return TDiscoveryPassthroughFile{}, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	return passthroughFile, nil
}

// passthroughTopicPrefixes reduces rules to just the topic prefixes actionable against
// physicalPrefix -- a rule declared "from" a different source prefix can never match live traffic
// on this subscription (the generator already warned about this at generate time,
// homeassistant/discovery_passthrough.go); silently ignored here rather than re-warned, so the
// coordinator's own log isn't cluttered by a condition already surfaced at generate time.
func passthroughTopicPrefixes(rules []TDiscoveryPassthroughRule, physicalPrefix string) []string {
	var prefixes []string
	for _, rule := range rules {
		if rule.SourcePrefix != physicalPrefix {
			continue
		}
		prefixes = append(prefixes, rule.TopicPrefix)
	}
	return prefixes
}

// matchesAnyPassthroughPrefix reports whether topic (a discovery payload's own state_topic or
// command_topic, already "~"-expanded) starts with any of prefixes.
func matchesAnyPassthroughPrefix(topic string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if prefix != "" && strings.HasPrefix(topic, prefix) {
			return true
		}
	}
	return false
}

// topicComponent extracts the HA component (domain) segment from a discovery config topic shaped
// "<prefix>/<component>/<node_id>/<object_id>/config" -- passthrough's own suggestion tracking
// uses this as the real, known domain, rather than guessing one the way discovery_existence.go's
// own suggestion report has to for opaque gateway leaf names.
func topicComponent(topic, prefix string) string {
	rest := strings.TrimPrefix(topic, prefix+"/")
	if idx := strings.Index(rest, "/"); idx >= 0 {
		return rest[:idx]
	}
	return ""
}

// firstDeviceIdentifier returns payload's own primary device identifier -- the stable key
// passthrough device tracking (and its Forget-on-migration counterpart) is keyed on -- or "" if
// the payload carried none at all.
func firstDeviceIdentifier(payload tDecodedDiscoveryPayload) string {
	if len(payload.DeviceIdentifiers) == 0 {
		return ""
	}
	return payload.DeviceIdentifiers[0]
}

// tFlexStringList decodes an HA discovery "ids"/"identifiers" field, which may be either a bare
// string or a list of strings.
type tFlexStringList []string

func (f *tFlexStringList) UnmarshalJSON(data []byte) error {
	var asList []string
	if err := json.Unmarshal(data, &asList); err == nil {
		*f = asList
		return nil
	}
	var asString string
	if err := json.Unmarshal(data, &asString); err != nil {
		return fmt.Errorf("identifiers: neither a string nor a list of strings: %w", err)
	}
	*f = []string{asString}
	return nil
}

// rawDiscoveryDevice decodes an incoming payload's "dev"/"device" block -- just the fields this
// bridge needs (identity + one-hop via_device + a human-readable name for passthrough
// suggestions, PROJECT.md item 4), not HA's full device-map field set.
type rawDiscoveryDevice struct {
	IDs         tFlexStringList `json:"ids"`
	Identifiers tFlexStringList `json:"identifiers"`
	ViaDevice   string          `json:"via_device"`
	Name        string          `json:"name"`
}

func (d rawDiscoveryDevice) identifiers() []string {
	if len(d.IDs) > 0 {
		return d.IDs
	}
	return d.Identifiers
}

// rawDiscoveryPayload decodes an incoming gateway discovery payload, abbreviated or full-length
// key names both accepted (see the component doc comment).
type rawDiscoveryPayload struct {
	TopicPrefix       string             `json:"~"`
	UniqueID          string             `json:"uniq_id"`
	UniqueIDFull      string             `json:"unique_id"`
	StateTopic        string             `json:"stat_t"`
	StateTopicFull    string             `json:"state_topic"`
	CommandTopic      string             `json:"cmd_t"`
	CommandTopicFull  string             `json:"command_topic"`
	ValueTemplate     string             `json:"val_tpl"`
	ValueTemplateFull string             `json:"value_template"`
	DeviceClass       string             `json:"dev_cla"`
	DeviceClassFull   string             `json:"device_class"`
	Unit              string             `json:"unit_of_meas"`
	UnitFull          string             `json:"unit_of_measurement"`
	StateClass        string             `json:"stat_cla"`
	StateClassFull    string             `json:"state_class"`
	Device            rawDiscoveryDevice `json:"dev"`
	DeviceFull        rawDiscoveryDevice `json:"device"`
}

// tDecodedDiscoveryPayload is a gateway's discovery payload reduced to what this bridge relays.
type tDecodedDiscoveryPayload struct {
	UniqueID          string
	StateTopic        string
	CommandTopic      string
	ValueTemplate     string
	DeviceClass       string
	Unit              string
	StateClass        string
	DeviceIdentifiers []string
	ViaDevice         string
	DeviceName        string
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// decodeDiscoveryPayload parses raw and applies "~" topic-prefix substitution to StateTopic, the
// same way HA itself expands it.
func decodeDiscoveryPayload(raw []byte) (tDecodedDiscoveryPayload, error) {
	var p rawDiscoveryPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return tDecodedDiscoveryPayload{}, err
	}

	stateTopic := firstNonEmpty(p.StateTopic, p.StateTopicFull)
	commandTopic := firstNonEmpty(p.CommandTopic, p.CommandTopicFull)
	if p.TopicPrefix != "" {
		stateTopic = strings.ReplaceAll(stateTopic, "~", p.TopicPrefix)
		commandTopic = strings.ReplaceAll(commandTopic, "~", p.TopicPrefix)
	}

	device := p.Device
	if len(device.identifiers()) == 0 && device.ViaDevice == "" {
		device = p.DeviceFull
	}

	return tDecodedDiscoveryPayload{
		UniqueID:          firstNonEmpty(p.UniqueID, p.UniqueIDFull),
		StateTopic:        stateTopic,
		CommandTopic:      commandTopic,
		ValueTemplate:     firstNonEmpty(p.ValueTemplate, p.ValueTemplateFull),
		DeviceClass:       firstNonEmpty(p.DeviceClass, p.DeviceClassFull),
		Unit:              firstNonEmpty(p.Unit, p.UnitFull),
		StateClass:        firstNonEmpty(p.StateClass, p.StateClassFull),
		DeviceIdentifiers: device.identifiers(),
		ViaDevice:         device.ViaDevice,
		DeviceName:        device.Name,
	}, nil
}

// matchingGateway returns the DeviceID of the first TDiscoveryFile.Gateways entry payload
// belongs to -- either directly (its own device identifiers intersect the gateway's) or one hop
// via its device's via_device -- and false if none match.
func matchingGateway(payload tDecodedDiscoveryPayload, gateways map[string]TDiscoveryGateway) (string, bool) {
	for gatewayID, gateway := range gateways {
		for _, want := range gateway.Identifiers {
			for _, got := range payload.DeviceIdentifiers {
				if got == want {
					return gatewayID, true
				}
			}
			if payload.ViaDevice != "" && payload.ViaDevice == want {
				return gatewayID, true
			}
		}
	}
	return "", false
}

// relayedDiscoveryUniqueID builds the stable identity a relayed entity's unique_id/topic are keyed
// on -- the gateway's own identity plus the gateway's own leaf name (payload.UniqueID), never our
// own entityID's readable path, which can be repositioned/renamed independently of the gateway's
// own naming. Shared with discoverycleanup.go's expectedDiscoveryBridgeTopics so the two can't
// drift apart.
func relayedDiscoveryUniqueID(gatewayID, leaf string) string {
	return sanitizeTopicSegment(gatewayID) + "_" + leaf
}

// buildRelayedDiscoveryConfig builds our own discovery payload for entityID (e.g.
// "sensor.social_garage_door_temperature"), sourced from a gateway leaf's decoded payload --
// same domain (component) as the gateway's own entity, our own default_entity_id (the full
// domain.objectID -- the real, documented mechanism for controlling entity_id on first creation;
// "object_id"/"has_entity_name" are not real MQTT discovery keys, see discovery.go's payload doc
// comments). unique_id/topic are keyed on relayedDiscoveryUniqueID (gateway+leaf), not on
// objectID, so relaying the same leaf under a different entityID (a reposition/rename in
// Spaces.def) changes the payload's default_entity_id/name but never the topic itself -- no
// device: block yet, deliberately deferred, same "extend on demand" principle as the
// abbreviation set above.
//
// device_class/unit_of_measurement/state_class prefer the gateway's own natively-reported value
// (it decided its own typing, see this file's own header comment) -- link's fields
// (Defaults.def-resolved, generator-side) only gap-fill a field the gateway's payload left
// empty, never override one it set. icon has no native counterpart in the decoded payload at
// all (see decodeDiscoveryPayload's "extend on demand" abbreviation set), so it comes from link
// unconditionally.
func buildRelayedDiscoveryConfig(entityID, gatewayID string, payload tDecodedDiscoveryPayload, link TDiscoveryEntityLink, prefix, installation string) (topic string, body map[string]interface{}, ok bool) {
	dotIdx := strings.Index(entityID, ".")
	if dotIdx < 0 || payload.StateTopic == "" || payload.UniqueID == "" {
		return "", nil, false
	}
	domain := entityID[:dotIdx]
	objectID := entityID[dotIdx+1:]
	stableID := relayedDiscoveryUniqueID(gatewayID, payload.UniqueID)

	body = map[string]interface{}{
		"unique_id":         stableID,
		"default_entity_id": domain + "." + objectID,
		"name":              objectID,
		"state_topic":       payload.StateTopic,
		"origin":            coordinatorOriginMap(installation),
	}
	if payload.ValueTemplate != "" {
		body["value_template"] = payload.ValueTemplate
	}
	if deviceClass := firstNonEmpty(payload.DeviceClass, link.DeviceClass); deviceClass != "" {
		body["device_class"] = deviceClass
	}
	if unit := firstNonEmpty(payload.Unit, link.Unit); unit != "" {
		body["unit_of_measurement"] = unit
	}
	if stateClass := firstNonEmpty(payload.StateClass, link.StateClass); stateClass != "" {
		body["state_class"] = stateClass
	}
	if link.Icon != "" {
		body["icon"] = link.Icon
	}
	return discoveryTopic(prefix, domain, stableID), body, true
}

// subscribeDiscoveryBridge subscribes to "<physical_prefix>/+/+/+/config" (the same
// <prefix>/<component>/<node_id>/<object_id>/config shape HA's own discovery uses) and, for
// every message whose device matches a declared gateway (matchingGateway) and whose leaf
// (UniqueID) a devices.yaml EntityLink references, republishes our own discovery config through
// publisher (content-aware retire-then-republish, discoverycleanup.go) onto conceptualPrefix.
// Also records the leaf as known-to-exist on existenceTracker (PROJECT.md 1.8, discovery_existence.go)
// regardless of whether an EntityLink claims it yet, and -- when it's newly known -- publishes that
// gateway's updated status to mainClient/cloudClient. An empty payload (HA's own MQTT discovery
// removal convention) is treated as a retraction: existenceTracker.MarkRetracted resolves it back
// to a (gatewayID, leaf) via whatever identity a prior non-empty payload on the same topic recorded
// (RecordTopicIdentity), moving that leaf to known-not-to-exist. existenceTracker may be nil (tests
// that don't need it).
//
// passthroughRules (PROJECT.md item 4, 2026-09-14) is the gradual-migration counterpart to the
// above: for a message whose device DOESN'T match a declared gateway, but whose state_topic or
// command_topic starts with a declared discovery_passthrough prefix, the raw payload is relayed
// byte-for-byte onto conceptualPrefix (topic tail unchanged, only the leading physical-prefix
// segment swapped) -- so HA keeps seeing a not-yet-migrated device exactly as it always has, no
// Physical.def declaration required. The two are mutually exclusive per message specifically so a
// device doesn't ever appear twice once it migrates: the moment a device's identifiers start
// matching a declared gateway, this same handler also retires whatever raw copy it may have been
// relaying for it (publisher.Knows guards the retire so it's only attempted for a topic this
// publisher actually knows about, not every declared-gateway message regardless of passthrough
// history). Only rules whose declared source matches discoveryFile.PhysicalPrefix are actionable
// here (passthroughTopicPrefixes) -- this subscription only ever sees that one prefix.
//
// passthroughDeviceTracker (2026-09-14, found live: real Zigbee2MQTT traffic was flowing through
// passthrough but never showed up in suggestions/discovery.txt) records every device passed
// through, keyed by its own real device identifier, so the generator can suggest a ready-to-paste
// declaration for it (discovery_passthrough_devices.go) -- forgotten again the instant a device
// starts matching a declared gateway, right alongside the existing raw-topic retirement above.
// Deliberately Record/Forget only here, never a per-message PublishStatus (a second real incident,
// found live minutes after the first deploy of this same feature: the initial subscribe replays
// the whole retained backlog synchronously, and a full-snapshot publish per newly-seen leaf
// starved the client's own later subscribes of their timeout window, crash-looping the
// coordinator) -- main.go publishes the tracker's status once at startup and once per reconnect
// instead, which is all a generate-time suggestion feed needs. May be nil (tests that don't need
// it).
//
// relayJobs (2026-09-14, discovery_relay_queue.go) is where every outbound publish/retire this
// handler decides to make actually goes -- never called directly against publisher any more. See
// that file's own header comment for the two real incidents (a fatal crash loop, then a confirmed
// live "No ACK from MQTT server" warning on HA's own side) that made both decoupling AND throttling
// necessary, not just one or the other. The raw passthrough publish below sets Passthrough: true so
// RetireMissing's static startup sweep never deletes it out from under a live device -- see
// TDiscoveryPublisher.RetireMissing's own doc comment for the real incident (2026-09-14) this
// exists to prevent a repeat of.
func subscribeDiscoveryBridge(client, cloudClient mqtt.Client, ownInstallation string, discoveryFile TDiscoveryFile, publisher *TDiscoveryPublisher, conceptualPrefix string, existenceTracker *TDiscoveryExistenceTracker, passthroughRules []TDiscoveryPassthroughRule, passthroughDeviceTracker *TPassthroughDeviceTracker, relayJobs chan<- TDiscoveryRelayJob) error {
	passthroughPrefixes := passthroughTopicPrefixes(passthroughRules, discoveryFile.PhysicalPrefix)

	handler := func(_ mqtt.Client, msg mqtt.Message) {
		passthroughTopic := conceptualPrefix + strings.TrimPrefix(msg.Topic(), discoveryFile.PhysicalPrefix)

		if len(msg.Payload()) == 0 {
			if existenceTracker != nil {
				if gatewayID, leaf, changed := existenceTracker.MarkRetracted(msg.Topic()); changed {
					fmt.Printf("[discovery-existence] %s: %s -> known-not-to-exist (retracted)\n", gatewayID, leaf)
					if err := existenceTracker.publishStatus(client, cloudClient, ownInstallation, gatewayID); err != nil {
						fmt.Printf("[discovery-existence] %v\n", err)
					}
				}
			}
			if publisher.Knows("main", passthroughTopic) {
				enqueueDiscoveryRelayJob(relayJobs, TDiscoveryRelayJob{Action: discoveryRelayRetire, Client: client, Broker: "main", Topic: passthroughTopic})
			}
			return
		}

		payload, err := decodeDiscoveryPayload(msg.Payload())
		if err != nil {
			fmt.Printf("[discovery-bridge] %s: cannot parse payload: %v\n", msg.Topic(), err)
			return
		}
		if payload.UniqueID == "" {
			return
		}

		gatewayID, matched := matchingGateway(payload, discoveryFile.Gateways)
		if !matched {
			if matchesAnyPassthroughPrefix(payload.StateTopic, passthroughPrefixes) || matchesAnyPassthroughPrefix(payload.CommandTopic, passthroughPrefixes) {
				// Payload bytes are copied: msg.Payload() isn't guaranteed valid once this handler
				// returns, and this job may not actually be processed until well after that.
				payloadCopy := append([]byte(nil), msg.Payload()...)
				enqueueDiscoveryRelayJob(relayJobs, TDiscoveryRelayJob{Action: discoveryRelayPublish, Client: client, Broker: "main", Topic: passthroughTopic, Payload: payloadCopy, Passthrough: true})
				fmt.Printf("[discovery-bridge] queued passthrough relay %s -> %s\n", msg.Topic(), passthroughTopic)
				if passthroughDeviceTracker != nil {
					deviceIdentifier := firstDeviceIdentifier(payload)
					domain := topicComponent(msg.Topic(), discoveryFile.PhysicalPrefix)
					// Record only -- deliberately NOT publishing a status update per message (real
					// incident, found live 2026-09-14 right after this deployed: the coordinator's
					// initial subscribe replays the ENTIRE retained backlog synchronously, one
					// message at a time -- hundreds of them, each a "first time seen" leaf -- and a
					// full-snapshot PublishStatus() call per leaf here (an ack-waiting network round
					// trip) starved the client's own connection setup of time to finish its LATER
					// subscribes within their own timeout window, crash-looping the whole
					// coordinator on "subscribing to homeassistant_instances/+/bridge/+/state: timed
					// out" every ~20s. The retained topic is still kept fresh -- just once per
					// startup/reconnect (main.go), not per leaf; good enough for a generate-time
					// suggestion feed nothing live depends on.
					passthroughDeviceTracker.Record(deviceIdentifier, payload.DeviceName, domain, payload.UniqueID)
				}
			}
			return
		}

		if publisher.Knows("main", passthroughTopic) {
			// This device just started matching a declared gateway -- retire whatever raw
			// passthrough copy was relaying it before, so HA never shows both at once.
			enqueueDiscoveryRelayJob(relayJobs, TDiscoveryRelayJob{Action: discoveryRelayRetire, Client: client, Broker: "main", Topic: passthroughTopic})
		}
		if passthroughDeviceTracker != nil {
			// Forget only -- same "no per-message status publish" reasoning as the passthrough
			// branch above.
			passthroughDeviceTracker.Forget(firstDeviceIdentifier(payload))
		}

		if existenceTracker != nil {
			existenceTracker.RecordTopicIdentity(msg.Topic(), gatewayID, payload.UniqueID)
			if existenceTracker.MarkKnown(gatewayID, payload.UniqueID) {
				fmt.Printf("[discovery-existence] %s: %s -> known-to-exist\n", gatewayID, payload.UniqueID)
				if err := existenceTracker.publishStatus(client, cloudClient, ownInstallation, gatewayID); err != nil {
					fmt.Printf("[discovery-existence] %v\n", err)
				}
			}
		}

		for entityID, link := range discoveryFile.EntityLinks {
			if link.Gateway != gatewayID || link.Leaf != payload.UniqueID {
				continue
			}
			topic, body, ok := buildRelayedDiscoveryConfig(entityID, gatewayID, payload, link, conceptualPrefix, ownInstallation)
			if !ok {
				continue
			}
			data, err := json.Marshal(body)
			if err != nil {
				fmt.Printf("[discovery-bridge] %s: marshalling relayed payload: %v\n", entityID, err)
				continue
			}
			enqueueDiscoveryRelayJob(relayJobs, TDiscoveryRelayJob{Action: discoveryRelayPublish, Client: client, Broker: "main", Topic: topic, Payload: data})
			fmt.Printf("[discovery-bridge] queued relay of %s (gateway %s, leaf %s) -> %s\n", entityID, gatewayID, payload.UniqueID, topic)
		}
	}

	topicFilter := discoveryFile.PhysicalPrefix + "/+/+/+/config"
	token := client.Subscribe(topicFilter, 0, handler)
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		if err := token.Error(); err != nil {
			return fmt.Errorf("subscribing to %s: %w", topicFilter, err)
		}
		return fmt.Errorf("subscribing to %s: timed out", topicFilter)
	}
	return nil
}
