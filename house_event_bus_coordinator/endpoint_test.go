package main

import "testing"

func smartyDevice() TDevice {
	return TDevice{
		Host:        "smarty",
		Integration: "cpu",
		Topic:       "hosts/smarty/cpu/state",
		NodeTopic:   "hosts/smarty/node/state",
		Conceptual: &TDeviceConceptual{
			NodeEntity:         "binary_sensor.infrastructural_smarty_node",
			NodeDeviceClass:    "connectivity",
			DisplayName:        "infrastructural/garage/smarty",
			ConstantAttributes: map[string]TConceptualConstant{"model": {Value: "Compute host"}},
			AttributeEntities: map[string]TConceptualAttribute{
				"load":        {Entity: "sensor.infrastructural_smarty_cpu_load", StateClass: "measurement"},
				"temperature": {Entity: "sensor.infrastructural_smarty_cpu_temperature", DeviceClass: "temperature", Unit: "°C", StateClass: "measurement"},
			},
		},
	}
}

func TestDeviceEventEndpointsWithConceptualLink(t *testing.T) {
	endpoints := deviceEventEndpoints("host.smarty", smartyDevice())
	if len(endpoints) != 3 {
		t.Fatalf("got %d endpoints, want 3 (node + load + temperature): %+v", len(endpoints), endpoints)
	}

	byID := map[string]TEventEndpoint{}
	for _, e := range endpoints {
		byID[e.ID] = e
	}

	node, ok := byID["host.smarty/node"]
	if !ok {
		t.Fatalf("missing node endpoint")
	}
	if node.StateTopic != "hosts/smarty/node/state" || node.AvailabilityTopic != "" || node.HAEntityID != "binary_sensor.infrastructural_smarty_node" {
		t.Errorf("node endpoint = %+v, unexpected shape", node)
	}
	if node.QoS != 0 || !node.Retain {
		t.Errorf("node endpoint QoS/Retain = %d/%v, want 0/true", node.QoS, node.Retain)
	}

	load, ok := byID["host.smarty/load"]
	if !ok {
		t.Fatalf("missing load endpoint")
	}
	if load.StateTopic != "hosts/smarty/cpu/state" || load.AvailabilityTopic != "hosts/smarty/node/state" || load.HAEntityID != "sensor.infrastructural_smarty_cpu_load" {
		t.Errorf("load endpoint = %+v, unexpected shape", load)
	}
}

func TestDeviceEventEndpointsWithoutConceptualLink(t *testing.T) {
	device := TDevice{
		Host:      "air-4",
		NodeTopic: "hosts/air-4/node/state",
		Topic:     "hosts/air-4/cpu/state",
	}
	endpoints := deviceEventEndpoints("host.air-4", device)
	if len(endpoints) != 1 {
		t.Fatalf("got %d endpoints, want 1 (node only, no conceptual link): %+v", len(endpoints), endpoints)
	}
	if endpoints[0].HAEntityID != "" {
		t.Errorf("HAEntityID = %q, want empty (no conceptual link)", endpoints[0].HAEntityID)
	}
}
