package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePhysicalDefFixture(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "Physical.def"), []byte(content), 0o644); err != nil {
		t.Fatalf("writing Physical.def fixture: %v", err)
	}
}

func readPhysicalDefFixture(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "Physical.def"))
	if err != nil {
		t.Fatalf("reading Physical.def fixture: %v", err)
	}
	return string(data)
}

// TestAutoAssignDiscoveryStableIDsAssignsFreshIDForLiveCapability covers the core "born after"
// case: a plain raw-leaf capability with no AssignedID, currently positioned (a live
// DiscoveryEntityLinks entry), gets a fresh id written back into Physical.def -- and ONLY that
// capability's own line, a sibling capability on the same device is left untouched.
func TestAutoAssignDiscoveryStableIDsAssignsFreshIDForLiveCapability(t *testing.T) {
	dir := t.TempDir()
	writePhysicalDefFixture(t, dir, strings.Join([]string{
		"integration discovery with:",
		"  device switches.garage_fuse_cabinet_zwave_058 with:",
		"    identifiers \"zwavejs2mqtt_0xd0204fb2_node58\";",
		"    fan.fan zwavejs2mqtt_0xd0204fb2_58-37-1-currentValue;",
		"    light.light zwavejs2mqtt_0xd0204fb2_58-37-2-currentValue;",
		"  end;",
		"end;",
	}, "\n"))

	gateways := map[string]TDiscoveryGatewayDevice{
		"switches.garage_fuse_cabinet_zwave_058": {
			DeviceID: "switches.garage_fuse_cabinet_zwave_058",
			Capabilities: map[string]TDiscoveryCapability{
				"fan":   {Domain: "fan", Leaf: "zwavejs2mqtt_0xd0204fb2_58-37-1-currentValue"},
				"light": {Domain: "light", Leaf: "zwavejs2mqtt_0xd0204fb2_58-37-2-currentValue"},
			},
		},
	}
	admin := newAdministrationState()
	admin.DiscoveryEntityLinks["fan.social_house_wc"] = TDiscoveryEntityLink{
		EntityID: "fan.social_house_wc", GatewayDeviceID: "switches.garage_fuse_cabinet_zwave_058",
		Leaf: "zwavejs2mqtt_0xd0204fb2_58-37-1-currentValue",
	}
	// "light" has no live link -- it should be left completely alone (no AssignedID, not live).

	warnings := autoAssignDiscoveryStableIDs(dir, gateways, admin)

	found := false
	for _, w := range warnings {
		if strings.Contains(w, `assigned stable id "fan.social_house_wc"`) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an \"assigned stable id\" warning, got: %v", warnings)
	}

	got := readPhysicalDefFixture(t, dir)
	if !strings.Contains(got, "fan.fan zwavejs2mqtt_0xd0204fb2_58-37-1-currentValue as fan.social_house_wc;") {
		t.Errorf("fan capability's line wasn't rewritten as expected, got:\n%s", got)
	}
	if !strings.Contains(got, "light.light zwavejs2mqtt_0xd0204fb2_58-37-2-currentValue;\n") {
		t.Errorf("light capability's line (not live, should be untouched) was modified, got:\n%s", got)
	}
}

// TestAutoAssignDiscoveryStableIDsDisambiguatesCollision covers a freshly-minted "position at
// birth" id colliding with an id some OTHER capability already has assigned (e.g. from the
// one-time bootstrap) -- per the user's own explicit correction, the disambiguator is a plain
// "_n" suffix, not "__n". (Two different live capabilities naturally resolving to the exact same
// entity_id can't happen via DiscoveryEntityLinks at all -- that's already a duplicate-entity-id
// error caught elsewhere in the pipeline, validateNoDuplicateFinalEntityIDs -- so the realistic
// collision is always against an id some other capability already owns, not between two fresh
// ones.)
func TestAutoAssignDiscoveryStableIDsDisambiguatesCollision(t *testing.T) {
	dir := t.TempDir()
	writePhysicalDefFixture(t, dir, strings.Join([]string{
		"integration discovery with:",
		"  device sensors.a with:",
		"    sensor.temperature 0xAAA_temperature_zigbee2mqtt;",
		"  end;",
		"  device sensors.b with:",
		"    sensor.temperature 0xBBB_temperature_zigbee2mqtt as sensor.physical_duplicate_name;",
		"  end;",
		"end;",
	}, "\n"))

	gateways := map[string]TDiscoveryGatewayDevice{
		"sensors.a": {DeviceID: "sensors.a", Capabilities: map[string]TDiscoveryCapability{
			"temperature": {Domain: "sensor", Leaf: "0xAAA_temperature_zigbee2mqtt"},
		}},
		"sensors.b": {DeviceID: "sensors.b", Capabilities: map[string]TDiscoveryCapability{
			// Already assigned (e.g. from the bootstrap) -- sits in usedIDs, not itself touched.
			"temperature": {Domain: "sensor", Leaf: "0xBBB_temperature_zigbee2mqtt", AssignedID: "sensor.physical_duplicate_name"},
		}},
	}
	admin := newAdministrationState()
	// sensors.a's own fresh id happens to collide with sensors.b's already-assigned one.
	admin.DiscoveryEntityLinks["sensor.physical_duplicate_name"] = TDiscoveryEntityLink{
		GatewayDeviceID: "sensors.a", Leaf: "0xAAA_temperature_zigbee2mqtt",
	}
	// sensors.b is live too (isolates this test to the collision-disambiguation behaviour only,
	// not Part 4's orphaned-id warning).
	admin.DiscoveryEntityLinks["sensor.physical_duplicate_name_b"] = TDiscoveryEntityLink{
		GatewayDeviceID: "sensors.b", Leaf: "0xBBB_temperature_zigbee2mqtt",
	}

	autoAssignDiscoveryStableIDs(dir, gateways, admin)

	got := readPhysicalDefFixture(t, dir)
	if !strings.Contains(got, "0xAAA_temperature_zigbee2mqtt as sensor.physical_duplicate_name_2;") {
		t.Errorf("expected sensors.a's own assignment to use a \"_2\" suffix (not \"__2\") to avoid colliding with sensors.b's, got:\n%s", got)
	}
	if !strings.Contains(got, "0xBBB_temperature_zigbee2mqtt as sensor.physical_duplicate_name;") {
		t.Errorf("sensors.b's own pre-existing assignment should be untouched, got:\n%s", got)
	}
}

// TestAutoAssignDiscoveryStableIDsWarnsOnOrphanedID covers the Part 4 complement: a capability
// that HAS an AssignedID but is no longer positioned anywhere gets a warning, and Physical.def is
// left completely untouched (no rewrite to perform at all).
func TestAutoAssignDiscoveryStableIDsWarnsOnOrphanedID(t *testing.T) {
	dir := t.TempDir()
	content := strings.Join([]string{
		"integration discovery with:",
		"  device sensors.retired with:",
		"    sensor.temperature 0xCCC_temperature_zigbee2mqtt as sensor.physical_old_room_temperature;",
		"  end;",
		"end;",
	}, "\n")
	writePhysicalDefFixture(t, dir, content)

	gateways := map[string]TDiscoveryGatewayDevice{
		"sensors.retired": {DeviceID: "sensors.retired", Capabilities: map[string]TDiscoveryCapability{
			"temperature": {Domain: "sensor", Leaf: "0xCCC_temperature_zigbee2mqtt", AssignedID: "sensor.physical_old_room_temperature"},
		}},
	}
	admin := newAdministrationState() // nothing positions it -- DiscoveryEntityLinks stays empty

	warnings := autoAssignDiscoveryStableIDs(dir, gateways, admin)

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "nothing positions it in Conceptual.def anymore") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an orphaned-id warning, got: %v", warnings)
	}
	if got := readPhysicalDefFixture(t, dir); got != content {
		t.Errorf("Physical.def should be untouched for an orphaned-id-only run, got:\n%s", got)
	}
}

// TestBootstrapDiscoveryStableIDsPreservesRegistryValue covers the one-time bootstrap path: a
// live capability with no AssignedID gets the REAL registry unique_id supplied in the mapping --
// never a freshly-minted "position at birth" one, preserving exactly what's already live.
func TestBootstrapDiscoveryStableIDsPreservesRegistryValue(t *testing.T) {
	dir := t.TempDir()
	writePhysicalDefFixture(t, dir, strings.Join([]string{
		"integration discovery with:",
		"  device sensors.house_kitchen_workplace_dimmer with:",
		"    light.light 0x00124b0019e7af0c_light_zigbee2mqtt;",
		"  end;",
		"end;",
	}, "\n"))

	gateways := map[string]TDiscoveryGatewayDevice{
		"sensors.house_kitchen_workplace_dimmer": {DeviceID: "sensors.house_kitchen_workplace_dimmer", Capabilities: map[string]TDiscoveryCapability{
			"light": {Domain: "light", Leaf: "0x00124b0019e7af0c_light_zigbee2mqtt"},
		}},
	}
	admin := newAdministrationState()
	admin.DiscoveryEntityLinks["light.physical_sensors_house_kitchen_workplace_dimmer"] = TDiscoveryEntityLink{
		GatewayDeviceID: "sensors.house_kitchen_workplace_dimmer", Leaf: "0x00124b0019e7af0c_light_zigbee2mqtt",
	}
	registryUniqueIDs := map[string]string{
		// The real, verified-live value -- NOT the entity_id, and NOT a "position at birth" string.
		"light.physical_sensors_house_kitchen_workplace_dimmer": "sensors_house_kitchen_workplace_dimmer_0x00124b0019e7af0c_light_zigbee2mqtt",
	}

	warnings := bootstrapDiscoveryStableIDs(dir, gateways, admin, registryUniqueIDs)

	found := false
	for _, w := range warnings {
		if strings.Contains(w, `bootstrapped stable id "sensors_house_kitchen_workplace_dimmer_0x00124b0019e7af0c_light_zigbee2mqtt"`) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a \"bootstrapped stable id\" warning with the real registry value, got: %v", warnings)
	}

	got := readPhysicalDefFixture(t, dir)
	if !strings.Contains(got, "0x00124b0019e7af0c_light_zigbee2mqtt as sensors_house_kitchen_workplace_dimmer_0x00124b0019e7af0c_light_zigbee2mqtt;") {
		t.Errorf("expected the real registry unique_id to be frozen, got:\n%s", got)
	}
}

// TestBootstrapDiscoveryStableIDsWarnsOnMissingRegistryEntry covers a live capability with no
// matching entry in the supplied mapping -- reported, not silently skipped or guessed.
func TestBootstrapDiscoveryStableIDsWarnsOnMissingRegistryEntry(t *testing.T) {
	dir := t.TempDir()
	content := strings.Join([]string{
		"integration discovery with:",
		"  device sensors.example with:",
		"    light.light 0xABC_light_zigbee2mqtt;",
		"  end;",
		"end;",
	}, "\n")
	writePhysicalDefFixture(t, dir, content)

	gateways := map[string]TDiscoveryGatewayDevice{
		"sensors.example": {DeviceID: "sensors.example", Capabilities: map[string]TDiscoveryCapability{
			"light": {Domain: "light", Leaf: "0xABC_light_zigbee2mqtt"},
		}},
	}
	admin := newAdministrationState()
	admin.DiscoveryEntityLinks["light.physical_example"] = TDiscoveryEntityLink{
		GatewayDeviceID: "sensors.example", Leaf: "0xABC_light_zigbee2mqtt",
	}

	warnings := bootstrapDiscoveryStableIDs(dir, gateways, admin, map[string]string{})

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "no matching entry in the supplied registry mapping") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a \"no matching entry\" warning, got: %v", warnings)
	}
	if got := readPhysicalDefFixture(t, dir); got != content {
		t.Errorf("Physical.def should be untouched when the registry mapping has no entry, got:\n%s", got)
	}
}

// TestAutoAssignDiscoveryStableIDsRefusesAmbiguousMatch covers the all-or-nothing safety net: if
// a capability's own line can't be found exactly once, nothing is written at all, for ANY
// assignment in that run -- never a partial, half-assigned Physical.def.
func TestAutoAssignDiscoveryStableIDsRefusesAmbiguousMatch(t *testing.T) {
	dir := t.TempDir()
	// Deliberately missing the capability's own line entirely -- zero matches.
	content := "integration discovery with:\n  device sensors.a with:\n  end;\nend;"
	writePhysicalDefFixture(t, dir, content)

	gateways := map[string]TDiscoveryGatewayDevice{
		"sensors.a": {DeviceID: "sensors.a", Capabilities: map[string]TDiscoveryCapability{
			"temperature": {Domain: "sensor", Leaf: "0xAAA_temperature_zigbee2mqtt"},
		}},
	}
	admin := newAdministrationState()
	admin.DiscoveryEntityLinks["sensor.physical_a_temperature"] = TDiscoveryEntityLink{
		GatewayDeviceID: "sensors.a", Leaf: "0xAAA_temperature_zigbee2mqtt",
	}

	warnings := autoAssignDiscoveryStableIDs(dir, gateways, admin)

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "found 0 matching") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a \"found 0 matching\" warning, got: %v", warnings)
	}
	if got := readPhysicalDefFixture(t, dir); got != content {
		t.Errorf("Physical.def should be untouched when a match can't be found, got:\n%s", got)
	}
}
