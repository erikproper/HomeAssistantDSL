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
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

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
// bridge needs (identity + one-hop via_device), not HA's full device-map field set.
type rawDiscoveryDevice struct {
	IDs         tFlexStringList `json:"ids"`
	Identifiers tFlexStringList `json:"identifiers"`
	ViaDevice   string          `json:"via_device"`
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
	ValueTemplate     string
	DeviceClass       string
	Unit              string
	StateClass        string
	DeviceIdentifiers []string
	ViaDevice         string
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
	if p.TopicPrefix != "" {
		stateTopic = strings.ReplaceAll(stateTopic, "~", p.TopicPrefix)
	}

	device := p.Device
	if len(device.identifiers()) == 0 && device.ViaDevice == "" {
		device = p.DeviceFull
	}

	return tDecodedDiscoveryPayload{
		UniqueID:          firstNonEmpty(p.UniqueID, p.UniqueIDFull),
		StateTopic:        stateTopic,
		ValueTemplate:     firstNonEmpty(p.ValueTemplate, p.ValueTemplateFull),
		DeviceClass:       firstNonEmpty(p.DeviceClass, p.DeviceClassFull),
		Unit:              firstNonEmpty(p.Unit, p.UnitFull),
		StateClass:        firstNonEmpty(p.StateClass, p.StateClassFull),
		DeviceIdentifiers: device.identifiers(),
		ViaDevice:         device.ViaDevice,
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
func buildRelayedDiscoveryConfig(entityID, gatewayID string, payload tDecodedDiscoveryPayload, link TDiscoveryEntityLink, prefix string) (topic string, body map[string]interface{}, ok bool) {
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
func subscribeDiscoveryBridge(client, cloudClient mqtt.Client, ownInstallation string, discoveryFile TDiscoveryFile, publisher *TDiscoveryPublisher, conceptualPrefix string, existenceTracker *TDiscoveryExistenceTracker) error {
	handler := func(_ mqtt.Client, msg mqtt.Message) {
		if len(msg.Payload()) == 0 {
			if existenceTracker != nil {
				if gatewayID, leaf, changed := existenceTracker.MarkRetracted(msg.Topic()); changed {
					fmt.Printf("[discovery-existence] %s: %s -> known-not-to-exist (retracted)\n", gatewayID, leaf)
					if err := existenceTracker.publishStatus(client, cloudClient, ownInstallation, gatewayID); err != nil {
						fmt.Printf("[discovery-existence] %v\n", err)
					}
				}
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
			return
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
			topic, body, ok := buildRelayedDiscoveryConfig(entityID, gatewayID, payload, link, conceptualPrefix)
			if !ok {
				continue
			}
			data, err := json.Marshal(body)
			if err != nil {
				fmt.Printf("[discovery-bridge] %s: marshalling relayed payload: %v\n", entityID, err)
				continue
			}
			if err := publisher.Publish(client, "main", topic, data); err != nil {
				fmt.Printf("[discovery-bridge] publishing %s: %v\n", topic, err)
				continue
			}
			fmt.Printf("[discovery-bridge] relayed %s (gateway %s, leaf %s) -> %s\n", entityID, gatewayID, payload.UniqueID, topic)
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
