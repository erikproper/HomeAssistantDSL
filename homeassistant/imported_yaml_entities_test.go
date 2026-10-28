package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveDerivedImport: a plain capability is its own relayed topic with "value"; a chain of
// derived capabilities composes their templates over the ultimate atomic ancestor's topic; a
// dangling reference doesn't resolve (moved from the coordinator's own tests, PROJECT.md 19h.4).
func TestResolveDerivedImport(t *testing.T) {
	link := TDeviceConceptualLink{AttributeEntityIDs: map[string]TDeviceAttributeLink{
		"battery_level": {EntityID: "sensor.infrastructural_terrace_netatmo_battery_level"},
	}}
	device := TImportedDevice{Capabilities: map[string]TImportedCapability{
		"battery_level":         {},
		"battery_alert":         {Domain: "binary_sensor", DerivedFromCapability: "battery_level", DerivedViaTemplate: "( $ | int(0) < 20 )"},
		"battery_alert_delayed": {Domain: "binary_sensor", DerivedFromCapability: "battery_alert", DerivedViaTemplate: "not ($)"},
		"dangling":              {Domain: "binary_sensor", DerivedFromCapability: "missing", DerivedViaTemplate: "$"},
	}}
	baseTopic := "imports/sensor.infrastructural_terrace_netatmo_battery_level/state"
	if topic, template, ok := resolveDerivedImport(device, link, "battery_level", map[string]bool{}); !ok || topic != baseTopic || template != "value" {
		t.Errorf("atomic: %q %q %v", topic, template, ok)
	}
	if topic, template, ok := resolveDerivedImport(device, link, "battery_alert_delayed", map[string]bool{}); !ok || topic != baseTopic || template != "not ((( (value) | int(0) < 20 )))" {
		t.Errorf("chain: %q %q %v", topic, template, ok)
	}
	if _, _, ok := resolveDerivedImport(device, link, "dangling", map[string]bool{}); ok {
		t.Errorf("a dangling reference resolved")
	}
}

// TestWriteImportRemoteIdentities: "from <export-id>" goes right after the capability's name,
// before its "as"; a renamed remote device's new id goes into the device line; another device's
// line with the same capability name is left alone (PROJECT.md 22).
func TestWriteImportRemoteIdentities(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Physical.def")
	before := `  integration import with:
    device sensors.netatmo_bedroom from junglinster sensors.netatmo_vienna_bedroom with:
      capability sensor.co2 as imported_sensor_physical_apartment_bedroom_netatmo_co2;
      capability sensor.humidity;
    end;
    device sensors.netatmo_terrace from junglinster sensors.netatmo_vienna_terrace with:
      capability sensor.co2 as imported_x;
    end;
    device node.pro_1 from junglinster with:
      capability sensor.cpu/load as imported_load;
    end;
  end;
`
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	writeImportRemoteIdentities(path,
		map[string]string{"sensors.netatmo_bedroom!co2": "hassbridge_sensor_infrastructural_netatmo_bedroom_co2", "sensors.netatmo_bedroom!humidity": "hassbridge_h", "node.pro_1!cpu/load": "host_eriks-macbook-pro-2_load"},
		map[string]string{"sensors.netatmo_bedroom": "sensors.netatmo_vienna_sleeping_room"})
	got, _ := os.ReadFile(path)
	want := `  integration import with:
    device sensors.netatmo_bedroom from junglinster sensors.netatmo_vienna_sleeping_room with:
      capability sensor.co2 from hassbridge_sensor_infrastructural_netatmo_bedroom_co2 as imported_sensor_physical_apartment_bedroom_netatmo_co2;
      capability sensor.humidity from hassbridge_h;
    end;
    device sensors.netatmo_terrace from junglinster sensors.netatmo_vienna_terrace with:
      capability sensor.co2 as imported_x;
    end;
    device node.pro_1 from junglinster with:
      capability sensor.cpu/load from host_eriks-macbook-pro-2_load as imported_load;
    end;
  end;
`
	if defStatements(string(got)) != defStatements(want) {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	devices, warnings := parseImportIntegrationBody(strings.Split(defStatements(string(got)), "\n")[1:11])
	if len(warnings) != 0 {
		t.Errorf("re-parse warnings: %v", warnings)
	}
	for _, d := range devices {
		if d.DeviceID == "sensors.netatmo_bedroom" && d.Capabilities["co2"].RemoteID != "hassbridge_sensor_infrastructural_netatmo_bedroom_co2" {
			t.Errorf("re-parsed co2 = %+v", d.Capabilities["co2"])
		}
	}
}
