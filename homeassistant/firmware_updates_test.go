package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRegisterFirmwareUpdates: a positioned device whose gateway reports an update entity gets
// ".../firmware" next to its node, linked to that leaf; a device without a positioned node gets
// none; and the house's summary covers every update (PROJECT.md 17i).
func TestRegisterFirmwareUpdates(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	cache := `{"sensors.corridor_corb": {"0x01_update_zigbee2mqtt": "known-to-exist", "0x01_light_zigbee2mqtt": "known-to-exist"},
	           "light.vidja_left_1": {"0x02_update_zigbee2mqtt": "known-to-exist"}}`
	if err := os.WriteFile(filepath.Join(dir, ".cache", "discovery_existence.json"), []byte(cache), 0o644); err != nil {
		t.Fatal(err)
	}
	admin := newAdministrationState()
	admin.AppendEntityRecord("social/house/corridor", TEntityRecord{
		Name:      "binary_sensor.infrastructural/house/corridor/corb/node",
		Identity:  extractEntityIdentity("binary_sensor.infrastructural/house/corridor/corb/node"),
		SourceKey: "physical:sensors.corridor_corb!node",
	})
	gateways := map[string]TDiscoveryGatewayDevice{"sensors.corridor_corb": {DeviceID: "sensors.corridor_corb"}, "light.vidja_left_1": {DeviceID: "light.vidja_left_1"}}

	if warnings := registerFirmwareUpdates(admin, dir, gateways); len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	const firmware = "update.infrastructural_house_corridor_corb_firmware"
	link, linked := admin.DiscoveryEntityLinks[firmware]
	if !linked || link.Leaf != "0x01_update_zigbee2mqtt" || link.StableID != "firmware_0x01_update_zigbee2mqtt" {
		t.Errorf("firmware link = %+v (found=%v), want the corb's update leaf", link, linked)
	}
	if len(admin.DiscoveryEntityLinks) != 1 {
		t.Errorf("%d links, want only the positioned device's", len(admin.DiscoveryEntityLinks))
	}
	summary, found := findEntityRecordByName(admin, "binary_sensor.infrastructural/firmware_updates")
	if !found || summary.ConditionExpr != "(is_state('"+firmware+"', 'on'))" {
		t.Errorf("summary = %+v (found=%v), want it on while the corb's update is", summary, found)
	}
}
