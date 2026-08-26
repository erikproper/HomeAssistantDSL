package main

import "testing"

const testPrefix = "homeassistant"

// TestBuildDiscoveryConfigsNodeHasNoHardcodedDeviceClass is a regression test for a real bug:
// the node entity's device_class/icon used to be hardcoded ("connectivity") directly in this
// package, completely independent of Defaults.def/resolveCapabilityDefaults -- the one place in
// the whole pipeline that didn't inherit its typing from the shared defaults mechanism. Now it
// must come only from devices.yaml's conceptual.node_device_class/node_icon (generator-resolved);
// with neither set, the coordinator must not invent a value of its own.
func TestBuildDiscoveryConfigsNodeHasNoHardcodedDeviceClass(t *testing.T) {
	device := smartyDevice()
	device.Conceptual.NodeDeviceClass = ""
	device.Conceptual.NodeIcon = ""

	configs := buildDiscoveryConfigs("host.smarty", device, nil, "", testPrefix)
	for _, c := range configs {
		payload, ok := c.Payload.(TBinarySensorDiscoveryPayload)
		if !ok {
			continue
		}
		if payload.DeviceClass != "" {
			t.Errorf("node payload DeviceClass = %q, want \"\" -- the coordinator must not hardcode a fallback", payload.DeviceClass)
		}
		if payload.Icon != "" {
			t.Errorf("node payload Icon = %q, want \"\"", payload.Icon)
		}
	}
}

func TestBuildDiscoveryConfigsForDeviceWithConceptualLink(t *testing.T) {
	configs := buildDiscoveryConfigs("host.smarty", smartyDevice(), nil, "", testPrefix)
	if len(configs) != 3 {
		t.Fatalf("got %d discovery configs, want 3: %+v", len(configs), configs)
	}

	byTopic := map[string]TDiscoveryConfig{}
	for _, c := range configs {
		byTopic[c.Topic] = c
	}

	nodeTopic := "homeassistant/binary_sensor/coordinator/host_smarty_node/config"
	nodeCfg, ok := byTopic[nodeTopic]
	if !ok {
		t.Fatalf("missing node discovery topic %q; got %v", nodeTopic, keysOf(byTopic))
	}
	nodePayload, ok := nodeCfg.Payload.(TBinarySensorDiscoveryPayload)
	if !ok {
		t.Fatalf("node payload has wrong type: %T", nodeCfg.Payload)
	}
	if nodePayload.UniqueID != "host.smarty_node" || nodePayload.StateTopic != "hosts/smarty/node/state" ||
		nodePayload.PayloadOn != "true" || nodePayload.PayloadOff != "false" ||
		nodePayload.DeviceClass != "connectivity" || nodePayload.Device.Identifiers[0] != "host.smarty" {
		t.Errorf("node payload = %+v, unexpected shape", nodePayload)
	}
	if nodePayload.DefaultEntityID != "binary_sensor.infrastructural_smarty_node" {
		t.Errorf("node payload DefaultEntityID = %q, want %q", nodePayload.DefaultEntityID, "binary_sensor.infrastructural_smarty_node")
	}
	if nodePayload.Name != nil {
		t.Errorf("node payload Name = %v, want nil (device.name alone, no combination)", nodePayload.Name)
	}
	if nodePayload.Device.Name != "infrastructural/garage/smarty" {
		t.Errorf("node payload Device.Name = %q, want %q", nodePayload.Device.Name, "infrastructural/garage/smarty")
	}
	if nodePayload.Device.Model != "Compute host" {
		t.Errorf("node payload Device.Model = %q, want %q", nodePayload.Device.Model, "Compute host")
	}

	loadTopic := "homeassistant/sensor/coordinator/host_smarty_load/config"
	loadCfg, ok := byTopic[loadTopic]
	if !ok {
		t.Fatalf("missing load discovery topic %q; got %v", loadTopic, keysOf(byTopic))
	}
	loadPayload, ok := loadCfg.Payload.(TSensorDiscoveryPayload)
	if !ok {
		t.Fatalf("load payload has wrong type: %T", loadCfg.Payload)
	}
	if loadPayload.StateTopic != "hosts/smarty/cpu/state" || loadPayload.ValueTemplate != "{{ value_json.load }}" ||
		loadPayload.AvailabilityTopic != "hosts/smarty/node/state" || loadPayload.PayloadAvailable != "true" {
		t.Errorf("load payload = %+v, unexpected shape", loadPayload)
	}
	if loadPayload.DeviceClass != "" || loadPayload.StateClass != "measurement" {
		t.Errorf("load payload DeviceClass/StateClass = %q/%q, want \"\"/\"measurement\"", loadPayload.DeviceClass, loadPayload.StateClass)
	}
	if loadPayload.DefaultEntityID != "sensor.infrastructural_smarty_cpu_load" {
		t.Errorf("load payload DefaultEntityID = %q, want %q", loadPayload.DefaultEntityID, "sensor.infrastructural_smarty_cpu_load")
	}
	if loadPayload.Name != "Load" {
		t.Errorf("load payload Name = %q, want %q (short leaf label -- HA combines it with device.name)", loadPayload.Name, "Load")
	}

	tempTopic := "homeassistant/sensor/coordinator/host_smarty_temperature/config"
	tempCfg, ok := byTopic[tempTopic]
	if !ok {
		t.Fatalf("missing temperature discovery topic %q; got %v", tempTopic, keysOf(byTopic))
	}
	tempPayload, ok := tempCfg.Payload.(TSensorDiscoveryPayload)
	if !ok {
		t.Fatalf("temperature payload has wrong type: %T", tempCfg.Payload)
	}
	if tempPayload.DeviceClass != "temperature" || tempPayload.UnitOfMeasurement != "°C" || tempPayload.StateClass != "measurement" {
		t.Errorf("temperature payload DeviceClass/Unit/StateClass = %q/%q/%q, want \"temperature\"/\"°C\"/\"measurement\"",
			tempPayload.DeviceClass, tempPayload.UnitOfMeasurement, tempPayload.StateClass)
	}
	if tempPayload.Name != "Temperature" {
		t.Errorf("temperature payload Name = %q, want %q", tempPayload.Name, "Temperature")
	}
}

func TestBuildDiscoveryConfigsSkipsDeviceWithoutConceptualLink(t *testing.T) {
	device := TDevice{Host: "air-4", NodeTopic: "hosts/air-4/node/state", Topic: "hosts/air-4/cpu/state"}
	configs := buildDiscoveryConfigs("host.air-4", device, nil, "", testPrefix)
	if configs != nil {
		t.Fatalf("got %d discovery configs, want none (no conceptual link): %+v", len(configs), configs)
	}
}

func TestDiscoveryTopicSanitisesStableID(t *testing.T) {
	got := discoveryTopic(testPrefix, "binary_sensor", "host.smarty_node")
	want := "homeassistant/binary_sensor/coordinator/host_smarty_node/config"
	if got != want {
		t.Errorf("discoveryTopic(...) = %q, want %q", got, want)
	}
}

func TestDiscoveryTopicUsesGivenPrefix(t *testing.T) {
	got := discoveryTopic("homeassistant.physical", "sensor", "discovery.ems_esp_boiler_outdoortemp")
	want := "homeassistant.physical/sensor/coordinator/discovery_ems_esp_boiler_outdoortemp/config"
	if got != want {
		t.Errorf("discoveryTopic(...) = %q, want %q", got, want)
	}
}

func TestResolveConstantAttributePrecedence(t *testing.T) {
	forced := map[string]TConceptualConstant{"model": {Value: "Cool raspi", Forced: true}}
	if got := resolveConstantAttribute("model", forced, map[string]string{"model": "Live Model"}); got != "Cool raspi" {
		t.Errorf("forced DSL should win over live, got %q", got)
	}

	def := map[string]TConceptualConstant{"model": {Value: "Compute host"}}
	if got := resolveConstantAttribute("model", def, map[string]string{"model": "Live Model"}); got != "Live Model" {
		t.Errorf("live should win over a non-forced DSL default, got %q", got)
	}
	if got := resolveConstantAttribute("model", def, nil); got != "Compute host" {
		t.Errorf("default DSL should be used when no live data yet, got %q", got)
	}
	if got := resolveConstantAttribute("model", nil, nil); got != "" {
		t.Errorf("expected \"\" when neither source has a value, got %q", got)
	}
}

func TestBuildDiscoveryConfigsMergesLiveDeviceInfoAndViaDevice(t *testing.T) {
	live := map[string]string{
		"sw_version":    "Debian GNU/Linux 12 (bookworm)",
		"serial_number": "10000000abcdef12",
		"via_device":    "junglinster",
	}
	configs := buildDiscoveryConfigs("host.smarty", smartyDevice(), live, "host.junglinster", testPrefix)

	nodeTopic := "homeassistant/binary_sensor/coordinator/host_smarty_node/config"
	byTopic := map[string]TDiscoveryConfig{}
	for _, c := range configs {
		byTopic[c.Topic] = c
	}
	nodePayload, ok := byTopic[nodeTopic].Payload.(TBinarySensorDiscoveryPayload)
	if !ok {
		t.Fatalf("missing/wrong-typed node discovery payload")
	}

	if nodePayload.Device.SwVersion != "Debian GNU/Linux 12 (bookworm)" {
		t.Errorf("Device.SwVersion = %q, want live-reported value", nodePayload.Device.SwVersion)
	}
	if nodePayload.Device.Model != "Compute host" {
		t.Errorf("Device.Model = %q, want the untouched DSL default (not a live field)", nodePayload.Device.Model)
	}
	if nodePayload.Device.ViaDevice != "host.junglinster" {
		t.Errorf("Device.ViaDevice = %q, want %q", nodePayload.Device.ViaDevice, "host.junglinster")
	}
	wantIdentifiers := []string{"host.smarty", "serial:10000000abcdef12"}
	if len(nodePayload.Device.Identifiers) != 2 || nodePayload.Device.Identifiers[0] != wantIdentifiers[0] || nodePayload.Device.Identifiers[1] != wantIdentifiers[1] {
		t.Errorf("Device.Identifiers = %v, want %v", nodePayload.Device.Identifiers, wantIdentifiers)
	}
}

func keysOf(m map[string]TDiscoveryConfig) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
