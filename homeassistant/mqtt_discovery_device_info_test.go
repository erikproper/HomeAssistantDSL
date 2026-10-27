package main

import (
	"strings"
	"testing"
)

// TestWriteDiscoveryMetadata: discovered metadata goes right after the identifiers, aligned; an
// earlier value is replaced, a forced one kept with a differing discovery noted; a second run
// changes nothing; a device without discovered metadata is left alone (PROJECT.md 19b).
func TestWriteDiscoveryMetadata(t *testing.T) {
	content := `integration discovery with:
    device sensors.bedroom_frient with:
      identifiers "zigbee2mqtt_0x01";
      sw_version: "4.0.7";
      capability sensor.temperature 0x01_temperature_zigbee2mqtt;
    end;
    device lights.lamp with:
      identifiers "zigbee2mqtt_0x02";
      model: "My lamp" forced;
      capability light.light 0x02_light_zigbee2mqtt;
    end;
    device lights.other with:
      identifiers "zigbee2mqtt_0x03";
    end;
end;`
	gateways := map[string]TDiscoveryGatewayDevice{
		"sensors.bedroom_frient": {DeviceID: "sensors.bedroom_frient"},
		"lights.lamp":            {DeviceID: "lights.lamp", Metadata: map[string]THostConstantAttribute{"model": {Value: "My lamp", Forced: true}}},
		"lights.other":           {DeviceID: "lights.other"},
	}
	info := map[string]map[string]string{
		"sensors.bedroom_frient": {"manufacturer": "Develco", "model": "Motion sensor", "sw_version": "4.0.8"},
		"lights.lamp":            {"manufacturer": "Signify", "model": "Hue go"},
	}
	got, changed := writeDiscoveryMetadata(content, gateways, info)
	want := `integration discovery with:
    device sensors.bedroom_frient with:
      identifiers "zigbee2mqtt_0x01";
      manufacturer: "Develco";
      model:        "Motion sensor";
      sw_version:   "4.0.8";
      capability sensor.temperature 0x01_temperature_zigbee2mqtt;
    end;
    device lights.lamp with:
      identifiers "zigbee2mqtt_0x02";
      manufacturer: "Signify";
      model:        "My lamp" forced;   # discovery: "Hue go"
      capability light.light 0x02_light_zigbee2mqtt;
    end;
    device lights.other with:
      identifiers "zigbee2mqtt_0x03";
    end;
end;`
	if got != want || changed != 2 {
		t.Fatalf("changed=%d, got:\n%s\nwant:\n%s", changed, got, want)
	}
	if again, changedAgain := writeDiscoveryMetadata(got, gateways, info); again != got || changedAgain != 0 {
		t.Errorf("a second run changed %d device(s):\n%s", changedAgain, again)
	}

	// A flag between the identifiers and the metadata: the metadata stays one block, before it.
	flagged := strings.Replace(got, "      identifiers \"zigbee2mqtt_0x01\";\n", "      identifiers \"zigbee2mqtt_0x01\";\n      ignore other capabilities;\n", 1)
	again, changedAgain := writeDiscoveryMetadata(flagged, gateways, info)
	wantFlagged := strings.Replace(got, "      sw_version:   \"4.0.8\";\n", "      sw_version:   \"4.0.8\";\n      ignore other capabilities;\n", 1)
	if again != wantFlagged || changedAgain != 0 {
		t.Errorf("with a flag in between, changed=%d:\n%s", changedAgain, again)
	}
}
