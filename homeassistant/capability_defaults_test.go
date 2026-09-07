package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectCapabilityDefaultsParsesForRules(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
  defaults:
    for binary_sensor.node:
      device_class: "connectivity";
    end;

    for sensor.temperature:
      unit: "°C";
      device_class: "temperature";
      state_class: "measurement";
    end;
  end;
end;
`)

	rules, warnings := collectCapabilityDefaults(definitionDir, t.TempDir())
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2: %v", len(rules), rules)
	}
	if rules[0].Domain != "binary_sensor" || rules[0].PatternPath != "node" || rules[0].DeviceClass != "connectivity" {
		t.Errorf("rule[0] = %+v, want binary_sensor.node -> connectivity", rules[0])
	}
	if rules[1].Domain != "sensor" || rules[1].PatternPath != "temperature" || rules[1].Unit != "°C" || rules[1].DeviceClass != "temperature" || rules[1].StateClass != "measurement" {
		t.Errorf("rule[1] = %+v, want sensor.temperature -> °C/temperature/measurement", rules[1])
	}
}

// TestCollectCapabilityDefaultsForLineWithMultipleTargets is a regression test for a real bug:
// a "for" line naming several space-separated "<domain>.<pattern>" targets (to share one body
// instead of repeating it) previously didn't match capabilityDefaultsForPattern at all -- the
// whole block, including every metadata line and "end;", fell through as unrecognised, silently
// registering no rule whatsoever.
func TestCollectCapabilityDefaultsForLineWithMultipleTargets(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
  defaults:
    for sensor.*/total_power sensor.*/current_power:
      device_class: "power";
      unit: "W";
      state_class: "measurement";
      icon: "mdi:flash";
    end;
  end;
end;
`)

	rules, warnings := collectCapabilityDefaults(definitionDir, t.TempDir())
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2 (one per target): %v", len(rules), rules)
	}
	for _, want := range []string{"total_power", "current_power"} {
		found := false
		for _, r := range rules {
			if r.Domain == "sensor" && r.PatternPath == "*/"+want {
				found = true
				if r.DeviceClass != "power" || r.Unit != "W" || r.StateClass != "measurement" || r.Icon != "mdi:flash" {
					t.Errorf("rule for %q = %+v, want power/W/measurement/mdi:flash", want, r)
				}
			}
		}
		if !found {
			t.Errorf("expected a sensor.*/%s rule, got %+v", want, rules)
		}
	}
}

// TestCollectCapabilityDefaultsReadsSharedDefaultsDefFile is a regression test for a real bug:
// a shared Defaults.def (Shared/Definitions/Defaults.def, wrapper-less like Settings.def --
// no "physical layer with:" needed) was silently never read, since Main.def's own "include"
// lines are not interpreted by the Go generator at all (definitionDir/sharedDefinitionDir are
// derived once from the CLI path, then specific filenames are read directly) -- a rule declared
// only in the shared file had zero effect, masked by the code-level postfix fallback happening
// to agree on the same values.
func TestCollectCapabilityDefaultsReadsSharedDefaultsDefFile(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
end;
`)
	sharedDefinitionDir := t.TempDir()
	sharedContent := `defaults:
  for binary_sensor.node:
    device_class: "connectivity";
  end;
end;
`
	if err := os.WriteFile(filepath.Join(sharedDefinitionDir, "Defaults.def"), []byte(sharedContent), 0o644); err != nil {
		t.Fatalf("failed to write shared Defaults.def: %v", err)
	}

	rules, warnings := collectCapabilityDefaults(definitionDir, sharedDefinitionDir)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(rules) != 1 || rules[0].Domain != "binary_sensor" || rules[0].PatternPath != "node" || rules[0].DeviceClass != "connectivity" {
		t.Fatalf("rules = %+v, want one binary_sensor.node -> connectivity rule sourced from the shared file", rules)
	}
}

func TestMatchesCapabilityPatternWildcardSuffix(t *testing.T) {
	cases := []struct {
		domain, path, ruleDomain, patternPath string
		want                                  bool
	}{
		{"binary_sensor", "node", "binary_sensor", "node", true},
		{"binary_sensor", "node", "sensor", "node", false},            // domain mismatch
		{"sensor", "cpu/load", "sensor", "cpu/load", true},            // exact match, no wildcard
		{"sensor", "cpu/load", "sensor", "load", false},               // no wildcard -- exact only
		{"sensor", "temperature", "sensor", "temperature", true},      // wildcard pattern, zero prefix segments absorbed
		{"sensor", "cpu/temperature", "sensor", "temperature", false}, // wildcard needed for this to match -- see next case
		{"binary_sensor", "anything", "binary_sensor", "*", true},     // bare "*" matches everything in-domain
	}

	if !matchesCapabilityPattern("sensor", "cpu/temperature", "sensor", "*/temperature") {
		t.Errorf("wildcard-prefixed pattern \"*/temperature\" should match path \"cpu/temperature\"")
	}
	if !matchesCapabilityPattern("sensor", "temperature", "sensor", "*/temperature") {
		t.Errorf("wildcard-prefixed pattern \"*/temperature\" should also match the bare path \"temperature\" (zero prefix segments absorbed)")
	}
	if matchesCapabilityPattern("sensor", "cpu/temperature", "sensor", "temperature") {
		t.Errorf("a non-wildcard pattern must match exactly, not by suffix")
	}

	for _, c := range cases {
		if got := matchesCapabilityPattern(c.domain, c.path, c.ruleDomain, c.patternPath); got != c.want {
			t.Errorf("matchesCapabilityPattern(%q,%q,%q,%q) = %v, want %v", c.domain, c.path, c.ruleDomain, c.patternPath, got, c.want)
		}
	}
}

func TestCapabilityDefaultsForFirstMatchWinsPerField(t *testing.T) {
	rules := []TCapabilityDefaultRule{
		{Domain: "sensor", PatternPath: "*/temperature", DeviceClass: "temperature", Unit: "°C"},
		{Domain: "sensor", PatternPath: "cpu/temperature", DeviceClass: "should-not-win", StateClass: "measurement"},
	}
	deviceClass, unit, stateClass, icon := capabilityDefaultsFor(rules, "sensor", "cpu/temperature")
	if deviceClass != "temperature" {
		t.Errorf("DeviceClass = %q, want the first matching rule's value \"temperature\"", deviceClass)
	}
	if unit != "°C" {
		t.Errorf("Unit = %q, want \"°C\"", unit)
	}
	if stateClass != "measurement" {
		t.Errorf("StateClass = %q, want the second rule's value since the first left it unset", stateClass)
	}
	if icon != "" {
		t.Errorf("Icon = %q, want \"\" -- neither rule declares one", icon)
	}
}

func TestRegisterHassBridgeAttributeEntityUsesCapabilityDefaultsRules(t *testing.T) {
	admin := newAdministrationState()
	admin.CapabilityDefaults = []TCapabilityDefaultRule{
		{Domain: "binary_sensor", PatternPath: "node", DeviceClass: "connectivity"},
		{Domain: "sensor", PatternPath: "*/load", Unit: "%", Icon: "mdi:cpu-64-bit", StateClass: "measurement"},
	}
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"host.junglinster": {DeviceID: "host.junglinster", Instances: []string{"main"}, Capabilities: map[string]THassBridgeCapability{
			"node":     {Domain: "binary_sensor", Sources: map[string]string{"main": "sensor.processor_use is available"}},
			"cpu/load": {Domain: "sensor", Sources: map[string]string{"main": "sensor.processor_use"}},
		}},
	}

	positioningDecl := TDevicePositioningDeclaration{Spec: "infrastructural:junglinster", DeviceID: "host.junglinster"}
	if warnings := registerDevicePositioning(admin, positioningDecl, nil, hassBridgeDevicesByID, nil, "Spaces.def", 1); len(warnings) != 0 {
		t.Fatalf("unexpected warnings positioning the fixture device: %v", warnings)
	}
	capabilityDecl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.infrastructural:junglinster/cpu/load", DeviceID: "host.junglinster", Capability: "cpu/load"}
	if warnings, deferred := registerDeviceCapabilityEntityLink(admin, capabilityDecl, nil, hassBridgeDevicesByID, nil, nil, nil, "Spaces.def", 1, true); len(warnings) != 0 || deferred {
		t.Fatalf("unexpected warnings/deferred: warnings=%v deferred=%v", warnings, deferred)
	}

	link := admin.DeviceConceptualLinks["host.junglinster"]

	node := link.AttributeEntityIDs["node"]
	if node.DeviceClass != "connectivity" {
		t.Errorf("node's DeviceClass = %q, want \"connectivity\" from the defaults rule", node.DeviceClass)
	}

	load := link.AttributeEntityIDs["cpu/load"]
	if load.Unit != "%" || load.Icon != "mdi:cpu-64-bit" || load.StateClass != "measurement" {
		t.Errorf("cpu/load metadata = %+v, want unit=%%, icon=mdi:cpu-64-bit, state_class=measurement from the wildcard defaults rule", load)
	}
}
