package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// TestPublishDeviceDiscoveryStripsSuggestedAreaFromCloudOnly is a regression test for a real leak
// found live 2026-09-08: publishDeviceDiscovery used to marshal one payload and publish the exact
// same bytes to both the local and cloud brokers, so a device's own suggested_area (which area
// THIS house's own Spaces.def positioning put it under) crossed the cloud broker verbatim. Which
// area a device sits in is never a fact for an importing house to inherit -- see
// TDiscoveryDevice.withoutSuggestedArea's own doc comment. Confirms the local payload keeps
// suggested_area while the cloud payload for the exact same device/capability omits it.
func TestPublishDeviceDiscoveryStripsSuggestedAreaFromCloudOnly(t *testing.T) {
	baseDevice := smartyDevice()
	baseDevice.Conceptual.ConstantAttributes["suggested_area"] = TConceptualConstant{Value: "garage"}
	nodeTopic := "homeassistant/binary_sensor/coordinator/host_smarty_node/config"

	// deviceRoutingPlan only ever routes a device to ONE of local/cloud at a time (Cloud=false ->
	// local only, Cloud=true -> cloud only, ImportedFrom aside) -- two separate publishes, same
	// underlying device/suggested_area, isolate exactly what each leg actually sends.
	localClient := &fakeClient{}
	store := NewLiveDeviceInfoStore("", nil)
	publisher := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	if _, err := publishDeviceDiscovery(localClient, nil, "junglinster", "host.smarty", baseDevice, store, nil, publisher, testPrefix); err != nil {
		t.Fatalf("publishDeviceDiscovery (local) error: %v", err)
	}
	localPublish, found := findPublish(localClient.publishedSnapshot(), nodeTopic)
	if !found {
		t.Fatalf("expected a local publish to %q, got %+v", nodeTopic, localClient.publishedSnapshot())
	}
	var localPayload struct {
		Device struct {
			SuggestedArea string `json:"suggested_area"`
		} `json:"device"`
	}
	if err := json.Unmarshal(localPublish.payload, &localPayload); err != nil {
		t.Fatalf("unmarshalling local payload: %v", err)
	}
	if localPayload.Device.SuggestedArea != "garage" {
		t.Errorf("local payload suggested_area = %q, want %q", localPayload.Device.SuggestedArea, "garage")
	}

	cloudDevice := baseDevice
	cloudDevice.Cloud = true
	cloudClient := &fakeClient{}
	if _, err := publishDeviceDiscovery(nil, cloudClient, "junglinster", "host.smarty", cloudDevice, store, nil, publisher, testPrefix); err != nil {
		t.Fatalf("publishDeviceDiscovery (cloud) error: %v", err)
	}
	cloudPublishes := cloudClient.publishedSnapshot()
	if len(cloudPublishes) == 0 {
		t.Fatalf("expected at least one cloud publish, got none")
	}
	for _, p := range cloudPublishes {
		var cloudPayload struct {
			Device struct {
				SuggestedArea string `json:"suggested_area"`
			} `json:"device"`
		}
		if err := json.Unmarshal(p.payload, &cloudPayload); err != nil {
			t.Fatalf("unmarshalling cloud payload for %q: %v", p.topic, err)
		}
		if cloudPayload.Device.SuggestedArea != "" {
			t.Errorf("cloud payload for %q has suggested_area = %q, want it stripped", p.topic, cloudPayload.Device.SuggestedArea)
		}
	}
}
