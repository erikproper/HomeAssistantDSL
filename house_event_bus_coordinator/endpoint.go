/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: Endpoint
 *
 * TEventEndpoint is this coordinator's (deliberately minimal) take on the EventEndpoint
 * concept Architecture.md §5 specifies -- derived purely from devices.yaml's existing fields
 * at load time, not a change to what the generator emits. QoS and Retain are fixed constants
 * today: every value in use anywhere (the deployed report scripts' plain mosquitto_pub calls,
 * HA's own discovery convention) is QoS 0 / retained, so there's nothing to make configurable
 * yet -- adding per-device fields with no observed variation would be designing ahead of need.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 21.08.2026
 *
 */

package main

// TEventEndpoint is one MQTT-bound observable property: a device's liveness, or one of its
// variable attributes. HAEntityID is "" when the owning device has no "conceptual:" link yet
// (Spaces.def hasn't declared a device.<spec> for it) -- the endpoint still exists (its topic
// is real, something is publishing to it), it's just not yet projected into HA discovery.
type TEventEndpoint struct {
	ID                string // e.g. "host.smarty/node", "host.smarty/load"
	StateTopic        string
	AvailabilityTopic string // "" if this endpoint IS the availability endpoint (the node itself)
	QoS               byte
	Retain            bool
	HAEntityID        string
}

// deviceEventEndpoints derives every EventEndpoint implied by one devices.yaml device: one
// node (liveness/availability) endpoint, always present since every hosts-integration device
// has a NodeTopic, plus one attribute endpoint per device.Conceptual.AttributeEntities entry
// (only when the device has a conceptual link -- devices.yaml's "capabilities" alone isn't
// enough, since that's keyed by capability name against a source entity, not a bus topic).
func deviceEventEndpoints(deviceID string, device TDevice) []TEventEndpoint {
	var endpoints []TEventEndpoint

	nodeEntityID := ""
	if device.Conceptual != nil {
		nodeEntityID = device.Conceptual.NodeEntity
	}
	endpoints = append(endpoints, TEventEndpoint{
		ID:         deviceID + "/node",
		StateTopic: device.NodeTopic,
		QoS:        0,
		Retain:     true,
		HAEntityID: nodeEntityID,
	})

	if device.Conceptual == nil {
		return endpoints
	}
	for attr, link := range device.Conceptual.AttributeEntities {
		endpoints = append(endpoints, TEventEndpoint{
			ID:                deviceID + "/" + attr,
			StateTopic:        device.Topic,
			AvailabilityTopic: device.NodeTopic,
			QoS:               0,
			Retain:            true,
			HAEntityID:        link.Entity,
		})
	}
	return endpoints
}
