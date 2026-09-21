package main

import (
	"strings"
	"testing"
)

func TestExtractDeviceCapabilityEntityDeclarationToleratesColumnAlignmentSpacing(t *testing.T) {
	// Spaces.def commonly pads with extra spaces for column alignment (e.g. multiple sensor
	// lines' "from" clauses lined up) -- a regression test for a real bug where the pattern used
	// a literal single space instead of \s+, silently failing to match any padded line.
	decl, ok := extractDeviceCapabilityEntityDeclaration("entity sensor.physical:netatmo/co2                 from hass.davids_bedroom sensor.co2;")
	if !ok {
		t.Fatalf("expected the padded line to match")
	}
	want := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.physical:netatmo/co2", DeviceID: "hass.davids_bedroom", Capability: "sensor.co2"}
	if *decl != want {
		t.Errorf("got %+v, want %+v", *decl, want)
	}
}

func TestExtractDeviceCapabilityEntityDeclarationSingleSpace(t *testing.T) {
	decl, ok := extractDeviceCapabilityEntityDeclaration("entity binary_sensor.infrastructural:netatmo/radio from hass.davids_bedroom binary_sensor.radio;")
	if !ok {
		t.Fatalf("expected the single-space line to match")
	}
	want := TDeviceCapabilityEntityDeclaration{LocalSpec: "binary_sensor.infrastructural:netatmo/radio", DeviceID: "hass.davids_bedroom", Capability: "binary_sensor.radio"}
	if *decl != want {
		t.Errorf("got %+v, want %+v", *decl, want)
	}
}

func TestBareCapabilityName(t *testing.T) {
	if got := bareCapabilityName("sensor.co2"); got != "co2" {
		t.Errorf("bareCapabilityName(sensor.co2) = %q, want co2", got)
	}
	if got := bareCapabilityName("co2"); got != "co2" {
		t.Errorf("bareCapabilityName(co2) = %q, want co2 (no domain prefix to strip)", got)
	}
}

func TestExtractDeviceCapabilityEntityDeclarationNoCollectSuffix(t *testing.T) {
	decl, ok := extractDeviceCapabilityEntityDeclaration("entity sensor.physical:dishwasher/robb/temperature from discovery.dishwasher_robb sensor.temperature with no_collect;")
	if !ok {
		t.Fatalf("expected the \"with no_collect\"-suffixed line to match")
	}
	want := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.physical:dishwasher/robb/temperature", DeviceID: "discovery.dishwasher_robb", Capability: "sensor.temperature", NoCollect: true}
	if *decl != want {
		t.Errorf("got %+v, want %+v", *decl, want)
	}

	decl, ok = extractDeviceCapabilityEntityDeclaration("entity sensor.physical:dishwasher/robb/temperature from discovery.dishwasher_robb sensor.temperature;")
	if !ok {
		t.Fatalf("expected the plain (no-suffix) line to still match")
	}
	if decl.NoCollect {
		t.Errorf("expected NoCollect false without the suffix")
	}
}

func TestExpandForDeviceShorthandLineNoCollectSuffix(t *testing.T) {
	got, ok := expandForDeviceShorthandLine("entity sensor.physical:dishwasher/robb/temperature from temperature with no_collect;", "discovery.dishwasher_robb")
	if !ok {
		t.Fatalf("expected the \"with no_collect\"-suffixed shorthand line to match")
	}
	want := "entity sensor.physical:dishwasher/robb/temperature from discovery.dishwasher_robb temperature with no_collect;"
	if got != want {
		t.Errorf("expandForDeviceShorthandLine = %q, want %q", got, want)
	}
	decl, ok := extractDeviceCapabilityEntityDeclaration(got)
	if !ok || !decl.NoCollect {
		t.Errorf("expected the re-expanded line to still parse with NoCollect true, got %+v (ok=%v)", decl, ok)
	}
}

func TestExpandForDeviceShorthandLine(t *testing.T) {
	got, ok := expandForDeviceShorthandLine("entity sensor.status from sensor.status;", "hass.laserjet")
	if !ok {
		t.Fatalf("expected the shorthand line to match")
	}
	want := "entity sensor.status from hass.laserjet sensor.status;"
	if got != want {
		t.Errorf("expandForDeviceShorthandLine = %q, want %q", got, want)
	}
	if _, ok := expandForDeviceShorthandLine("entity sensor.status from hass.laserjet sensor.status;", "hass.laserjet"); ok {
		t.Errorf("expected no match for an already-expanded (non-shorthand) line")
	}
}

// TestExpandForDeviceBareEntityLine covers the 2026-09-19 "entity <spec>;" shorthand-of-a-
// shorthand: no "from <capability>;" at all, capability auto-inferred as the spec's own explicit
// path (deviceSpecLeafPath) -- only valid when that path exactly matches the desired capability
// name, i.e. no rename.
func TestExpandForDeviceBareEntityLine(t *testing.T) {
	got, ok := expandForDeviceBareEntityLine("entity sensor.physical:co2;", "node.vienna_bedroom")
	if !ok {
		t.Fatalf("expected the bare-entity line to match")
	}
	want := "entity sensor.physical:co2 from node.vienna_bedroom co2;"
	if got != want {
		t.Errorf("expandForDeviceBareEntityLine = %q, want %q", got, want)
	}
}

// TestExpandForDeviceBareEntityLineNoCollectSuffix covers the "with no_collect" trailing suffix
// carrying through the bare-entity shorthand exactly like it does for the "from"-having one.
func TestExpandForDeviceBareEntityLineNoCollectSuffix(t *testing.T) {
	got, ok := expandForDeviceBareEntityLine("entity sensor.physical:robb/temperature with no_collect;", "discovery.some_plug")
	if !ok {
		t.Fatalf("expected the bare-entity line to match")
	}
	want := "entity sensor.physical:robb/temperature from discovery.some_plug robb/temperature with no_collect;"
	if got != want {
		t.Errorf("expandForDeviceBareEntityLine = %q, want %q", got, want)
	}
}

// TestExpandForDeviceBareEntityLineDoubleColonForm is the regression test for a real bug found
// live 2026-09-19 (Vienna's daylight entity): combining this shorthand with the "sphere::path"
// empty-leaf-override form (hasDeviceLeafOverride, Conceptual_DeviceEntities.go) used to
// infer capability ":daylight" (deviceSpecLeafPath only strips up to the FIRST colon, leaving the
// second one attached), which could never match a real Physical.def capability -- the correct
// inferred capability is "daylight", with the original spec's own "::" passed through unchanged
// so naming resolution still applies the empty-leaf override.
func TestExpandForDeviceBareEntityLineDoubleColonForm(t *testing.T) {
	got, ok := expandForDeviceBareEntityLine("entity binary_sensor.social::daylight;", "environment.sun")
	if !ok {
		t.Fatalf("expected the bare-entity line to match")
	}
	want := "entity binary_sensor.social::daylight from environment.sun daylight;"
	if got != want {
		t.Errorf("expandForDeviceBareEntityLine = %q, want %q", got, want)
	}
}

// TestExpandForDeviceBareEntityLineExplicitLeafOverrideForm is the regression test for the
// 2026-09-21 case (environment.weather's own "pressure" capability): combining this shorthand with
// the "sphere:leaf:path" explicit-replacement-leaf form (hasDeviceLeafOverride,
// Conceptual_DeviceEntities.go) must infer capability "pressure", not the leftover
// "terrace:pressure" (deviceSpecLeafPath only strips up to the FIRST colon, leaving the second
// colon-separated segment attached) -- mirrors TestExpandForDeviceBareEntityLineDoubleColonForm's
// own reasoning for the sibling double-colon form.
func TestExpandForDeviceBareEntityLineExplicitLeafOverrideForm(t *testing.T) {
	got, ok := expandForDeviceBareEntityLine("entity sensor.social:terrace:pressure;", "environment.weather")
	if !ok {
		t.Fatalf("expected the bare-entity line to match")
	}
	want := "entity sensor.social:terrace:pressure from environment.weather pressure;"
	if got != want {
		t.Errorf("expandForDeviceBareEntityLine = %q, want %q", got, want)
	}
}

// TestExpandForDeviceBareEntityLineRejectsEmptyPath covers a spec with no explicit path at all
// (e.g. "switch.social:", relying entirely on deviceNamePath injection) -- there is nothing to
// infer a capability from, so this shorthand must not match; the caller still needs the explicit
// "entity <spec> from <capability>;" form for a rename like this.
func TestExpandForDeviceBareEntityLineRejectsEmptyPath(t *testing.T) {
	if _, ok := expandForDeviceBareEntityLine("entity switch.social:;", "discovery.washing_machine_switch"); ok {
		t.Errorf("expected no match when the spec has no explicit path to infer a capability from")
	}
}

// TestExpandForDeviceBareEntityLineRejectsFromLine confirms the two shorthand shapes never
// overlap -- a line that already has "from <capability>;" is left to expandForDeviceShorthandLine
// alone.
func TestExpandForDeviceBareEntityLineRejectsFromLine(t *testing.T) {
	if _, ok := expandForDeviceBareEntityLine("entity light.social:main from core;", "discovery.hallway_light_main"); ok {
		t.Errorf("expected no match for a line that already has a \"from\" clause")
	}
}

// TestForDeviceBlockRegistersSameAsExplicitDeviceIDLines is the regression test for the "for
// <device-id>: ... end;" abbreviation the user asked for 2026-08-28, to avoid repeating a device
// id on every "entity ... from <device-id> <capability>;" line -- confirms the shorthand
// (now only reachable inside the merged "device <spec> from <device-id> with: ... end;" block,
// PROJECT.md unification plan 2026-09-01) registers identically to writing the device id out on
// every line.
func TestForDeviceBlockRegistersSameAsExplicitDeviceIDLines(t *testing.T) {
	const miniDSL = `device infrastructural:laserjet from hass.laserjet with:
  entity sensor.status   from sensor.status;
  entity sensor.cardrige from sensor.cardrige;
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.laserjet": {DeviceID: "hass.laserjet", Instances: []string{"protocols-server-2"}, Capabilities: map[string]THassBridgeCapability{
			"status":   {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.hewlett_packard_hp_laserjet_professional_p1102w"}},
			"cardrige": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.hewlett_packard_hp_laserjet_professional_p1102w_black_cartridge_hp_ce285a"}},
		}},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration

	link, ok := admin.DeviceConceptualLinks["hass.laserjet"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[hass.laserjet] to be populated")
	}
	for _, capabilityKey := range []string{"status", "cardrige"} {
		if _, found := link.AttributeEntityIDs[capabilityKey]; !found {
			t.Errorf("expected capability %q to be registered via the \"for\" block, got %v", capabilityKey, link.AttributeEntityIDs)
		}
	}
}

// TestForDeviceBlockBareEntityShorthandRegistersSameAsExplicitFromLines is the end-to-end
// regression test for the 2026-09-19 "entity <spec>;" shorthand: confirms it registers identically
// to writing out "entity <spec> from <capability>;" explicitly, through the real dispatch
// pipeline (not just the textual expansion unit tests above).
func TestForDeviceBlockBareEntityShorthandRegistersSameAsExplicitFromLines(t *testing.T) {
	const miniDSL = `device infrastructural:laserjet from hass.laserjet with:
  entity sensor.status;
  entity sensor.cardrige from sensor.cardrige;
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.laserjet": {DeviceID: "hass.laserjet", Instances: []string{"protocols-server-2"}, Capabilities: map[string]THassBridgeCapability{
			"status":   {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.hewlett_packard_hp_laserjet_professional_p1102w"}},
			"cardrige": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.hewlett_packard_hp_laserjet_professional_p1102w_black_cartridge_hp_ce285a"}},
		}},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration

	link, ok := admin.DeviceConceptualLinks["hass.laserjet"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[hass.laserjet] to be populated")
	}
	for _, capabilityKey := range []string{"status", "cardrige"} {
		if _, found := link.AttributeEntityIDs[capabilityKey]; !found {
			t.Errorf("expected capability %q to be registered via the bare-entity shorthand, got %v", capabilityKey, link.AttributeEntityIDs)
		}
	}
}

// TestForDeviceBlockRegistersHostsCapabilities is the "hosts"-kind counterpart to
// TestForDeviceBlockRegistersSameAsExplicitDeviceIDLines (PROJECT.md, unification plan
// 2026-09-01): "hosts" devices used to be explicitly rejected by this construct
// ("this construct doesn't support that kind yet") -- confirms the merged "device <spec> from
// <device-id> with: ... end;" block registers a cpu-type host's node (unconditional) and
// explicitly-named attributes identically to what "with: all entities;" used to bulk-imply,
// without needing that flag at all.
func TestForDeviceBlockRegistersHostsCapabilities(t *testing.T) {
	const miniDSL = `device infrastructural:xanadu from host.xanadu with:
  entity sensor.infrastructural:xanadu/cpu/load        from cpu/load;
  entity sensor.infrastructural:xanadu/cpu/temperature from cpu/temperature;
end;`

	hostDevicesByID := map[string]THostDevice{
		"host.xanadu": {DeviceID: "host.xanadu", HostName: "xanadu", IntegrationType: "cpu"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, hostDevicesByID, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	link, ok := result.Administration.DeviceConceptualLinks["host.xanadu"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[host.xanadu] to be populated")
	}
	if link.NodeEntityID == "" {
		t.Errorf("link.NodeEntityID = %q, want the node entity registered by positioning (unconditional)", link.NodeEntityID)
	}
	for _, attr := range []string{"load", "temperature"} {
		if _, found := link.AttributeEntityIDs[attr]; !found {
			t.Errorf("expected attribute %q to be registered via the \"for\" block, got %v", attr, link.AttributeEntityIDs)
		}
	}
}

// TestHostCapabilityEntityLinkRejectsUnknownAttribute confirms an attribute name that isn't part
// of the device's integration type's materialization is rejected with a clear message, not
// silently accepted or matched against the wrong thing.
func TestHostCapabilityEntityLinkRejectsUnknownAttribute(t *testing.T) {
	const miniDSL = `device infrastructural:xanadu from host.xanadu with:
  entity sensor.infrastructural:xanadu/bogus from bogus;
end;`

	hostDevicesByID := map[string]THostDevice{
		"host.xanadu": {DeviceID: "host.xanadu", HostName: "xanadu", IntegrationType: "cpu"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, hostDevicesByID, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	link := result.Administration.DeviceConceptualLinks["host.xanadu"]
	if _, found := link.AttributeEntityIDs["bogus"]; found {
		t.Errorf("expected \"bogus\" to be rejected (not a real cpu-type attribute), got %v", link.AttributeEntityIDs)
	}
}

// TestHostCapabilityEntityLinkRejectsNode confirms "node" can't be referenced explicitly for a
// hosts device -- it's unconditional, already registered the moment the device is positioned, so
// an explicit reference would just be a confusing, redundant second path to the same entity.
func TestHostCapabilityEntityLinkRejectsNode(t *testing.T) {
	const miniDSL = `device infrastructural:xanadu from host.xanadu with:
  entity binary_sensor.infrastructural:xanadu/node from node;
end;`

	hostDevicesByID := map[string]THostDevice{
		"host.xanadu": {DeviceID: "host.xanadu", HostName: "xanadu", IntegrationType: "cpu"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, hostDevicesByID, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	link, ok := result.Administration.DeviceConceptualLinks["host.xanadu"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[host.xanadu] to be populated")
	}
	if link.NodeEntityID == "" {
		t.Errorf("link.NodeEntityID = %q, want it still set from positioning, unaffected by the rejected explicit reference", link.NodeEntityID)
	}
	if _, found := link.AttributeEntityIDs["node"]; found {
		t.Errorf("link.AttributeEntityIDs = %v, want no \"node\" entry -- it's rejected, not registered as a regular attribute", link.AttributeEntityIDs)
	}
}

// TestHostCapabilityEntityLinkDefersWithoutPriorPositioning confirms a capability-link line
// appearing BEFORE its device's positioning line in the same file still resolves correctly (via
// the single retry ParseEntitiesAndFillAdministration makes after the whole file is read), mirroring
// the same order-independence the hassbridge/import branches already have.
func TestHostCapabilityEntityLinkDefersWithoutPriorPositioning(t *testing.T) {
	const miniDSL = `entity sensor.infrastructural:xanadu/cpu/load from host.xanadu cpu/load;
device infrastructural:xanadu from host.xanadu;`

	hostDevicesByID := map[string]THostDevice{
		"host.xanadu": {DeviceID: "host.xanadu", HostName: "xanadu", IntegrationType: "cpu"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, hostDevicesByID, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	link, ok := result.Administration.DeviceConceptualLinks["host.xanadu"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[host.xanadu] to be populated")
	}
	if _, found := link.AttributeEntityIDs["load"]; !found {
		t.Errorf("expected \"load\" to resolve via the deferred retry, got %v", link.AttributeEntityIDs)
	}
}

// TestAbsorbedHassBridgeDeviceKeepsOwnPhysicalPositioning is the end-to-end regression test for
// the "absorb" operation's dispatch-order fix (2026-09-18, Conceptual_DevicePositioning.go's
// hasPhysicalPresence guard): appliance.washing_machine is BOTH a real hassbridge device (its own
// Physical.def "status"/"node" capabilities) AND, once it gains an absorb block, a Logical.def
// device with non-empty Capabilities -- before the fix, the pure-logical branch would have
// intercepted its "device ... from appliance.washing_machine with: ...;" positioning line entirely,
// silently dropping the device's own real node/constantAttrs registration. Also exercises the
// hassbridge capability-fallback (Conceptual_DeviceCapabilityEntities.go's dispatchLogicalCapability):
// "status" resolves via the ordinary hassbridge path, while "core"/"power" (not hassbridge
// capabilities at all) fall through to the Logical.def absorb overlay and delegate to the
// underlying discovery gateway.
func TestAbsorbedHassBridgeDeviceKeepsOwnPhysicalPositioning(t *testing.T) {
	const miniDSL = `space social:shower_room with:
  device infrastructural:washing_machine from appliance.washing_machine with:
    entity sensor.status from status;
    entity switch.social: from core;
    entity sensor.social:power from power;
  end;
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"appliance.washing_machine": {
			DeviceID:  "appliance.washing_machine",
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"status": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.bathroom_washing_machine_status"}},
				"node":   {Domain: "binary_sensor", Sources: map[string]string{"protocols-server-2": "sensor.bathroom_washing_machine is available"}},
			},
		},
	}

	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"discovery.washing_machine_switch": {
			DeviceID:    "discovery.washing_machine_switch",
			Identifiers: []string{"zigbee2mqtt_0x9035eafffe694513"},
			Capabilities: map[string]TDiscoveryCapability{
				"core":  {Domain: "switch", Leaf: "0x9035eafffe694513_switch_zigbee2mqtt"},
				"power": {Domain: "sensor", Leaf: "0x9035eafffe694513_power_zigbee2mqtt"},
			},
		},
	}

	logicalDevicesByID := map[string]TLogicalDevice{
		"appliance.washing_machine": {
			DeviceID: "appliance.washing_machine",
			Capabilities: map[string]TLogicalCapability{
				"core":  {Domain: "switch", AbsorbedFromDeviceID: "discovery.washing_machine_switch", AbsorbedFromCapability: "core"},
				"power": {Domain: "sensor", AbsorbedFromDeviceID: "discovery.washing_machine_switch", AbsorbedFromCapability: "power"},
			},
		},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, discoveryGatewaysByID, hassBridgeDevicesByID, nil, nil, logicalDevicesByID, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration

	link, ok := admin.DeviceConceptualLinks["appliance.washing_machine"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[appliance.washing_machine] to be populated (hassbridge positioning must win over the pure-logical branch)")
	}
	if _, found := link.AttributeEntityIDs["node"]; !found {
		t.Errorf("expected the device's own real hassbridge node to still be auto-registered, got %v", link.AttributeEntityIDs)
	}
	if _, found := link.AttributeEntityIDs["status"]; !found {
		t.Errorf("expected the ordinary hassbridge \"status\" capability to still resolve, got %v", link.AttributeEntityIDs)
	}

	var foundCore, foundPower bool
	for _, discLink := range admin.DiscoveryEntityLinks {
		if discLink.GatewayDeviceID != "discovery.washing_machine_switch" {
			continue
		}
		if discLink.Leaf == "0x9035eafffe694513_switch_zigbee2mqtt" {
			foundCore = true
		}
		if discLink.Leaf == "0x9035eafffe694513_power_zigbee2mqtt" {
			foundPower = true
		}
	}
	if !foundCore {
		t.Errorf("expected the absorbed \"core\" capability to resolve via discovery.washing_machine_switch, got %+v", admin.DiscoveryEntityLinks)
	}
	if !foundPower {
		t.Errorf("expected the absorbed \"power\" capability to resolve via discovery.washing_machine_switch, got %+v", admin.DiscoveryEntityLinks)
	}
}

// TestDiscoveryDeviceKeepsOwnPhysicalPositioningWhenGainingLogicalCapabilities is the
// discovery-kind counterpart to TestAbsorbedHassBridgeDeviceKeepsOwnPhysicalPositioning
// (2026-09-18): sensors.terrace_motion's own "sunny_threshold"/"sunny", declared in Logical.def
// alongside its real discovery-relayed illuminance/motion capabilities. Confirms both the
// dispatch-order guard (registerDevicePositioning's own physical positioning must still win, not
// the pure-logical branch) and the discovery branch's own capability-level fallback to Logical.def
// on a miss.
func TestDiscoveryDeviceKeepsOwnPhysicalPositioningWhenGainingLogicalCapabilities(t *testing.T) {
	const miniDSL = `space social:terrace with:
  device infrastructural:signify_motion from sensors.terrace_motion with:
    entity binary_sensor.physical:motion from core;
    entity sensor.physical:illuminance from illuminance;
    entity input_number.social:sunny_threshold from sunny_threshold;
    entity binary_sensor.social:sunny from sunny;
  end;
end;`

	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"sensors.terrace_motion": {
			DeviceID:    "sensors.terrace_motion",
			Identifiers: []string{"zigbee2mqtt_0x001788010cdb8ee0"},
			Capabilities: map[string]TDiscoveryCapability{
				"core":        {Domain: "binary_sensor", Leaf: "0x001788010cdb8ee0_occupancy_zigbee2mqtt"},
				"illuminance": {Domain: "sensor", Leaf: "0x001788010cdb8ee0_illuminance_zigbee2mqtt"},
			},
		},
	}

	logicalDevicesByID := map[string]TLogicalDevice{
		"sensors.terrace_motion": {
			DeviceID: "sensors.terrace_motion",
			Capabilities: map[string]TLogicalCapability{
				"sunny_threshold": {Domain: "input_number", IsDefinedInputNumber: true, DefinedMinimum: "0", DefinedMaximum: "1000"},
				"sunny": {
					Domain: "binary_sensor", IsDerivedCondition: true,
					DerivedConditionExpr: "($1 | int) > ($2 | int)",
					DerivedConditionOver: []string{"illuminance", "sunny_threshold"},
				},
			},
		},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, discoveryGatewaysByID, nil, nil, nil, logicalDevicesByID, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration

	if _, ok := admin.DeviceConceptualLinks["sensors.terrace_motion"]; !ok {
		t.Fatalf("expected DeviceConceptualLinks[sensors.terrace_motion] to be populated (discovery positioning must win over the pure-logical branch)")
	}
	// Discovery-kind capabilities never populate DeviceConceptualLinks.AttributeEntityIDs (that's
	// a hosts/hassbridge/import-only mechanism) -- they resolve via the separate
	// DiscoveryEntityLinks map instead, keyed by entity_id.
	var foundMotion bool
	for _, discLink := range admin.DiscoveryEntityLinks {
		if discLink.GatewayDeviceID == "sensors.terrace_motion" && discLink.Leaf == "0x001788010cdb8ee0_occupancy_zigbee2mqtt" {
			foundMotion = true
		}
	}
	if !foundMotion {
		t.Errorf("expected the ordinary discovery \"motion\" capability to still resolve, got %+v", admin.DiscoveryEntityLinks)
	}

	rec, found := findEntityRecordByName(admin, "binary_sensor.social/terrace/signify_motion/sunny")
	if !found {
		t.Fatalf("no entity record registered for binary_sensor.social/terrace/signify_motion/sunny")
	}
	if len(rec.ConditionSources) != 2 {
		t.Fatalf("ConditionSources = %v, want 2 sources (illuminance, sunny_threshold)", rec.ConditionSources)
	}
}

func TestForDeviceBlockWarnsOnUnrecognisedLineAndKeepsParsing(t *testing.T) {
	const miniDSL = `device infrastructural:laserjet from hass.laserjet with:
  this is not a valid line;
  entity sensor.status from sensor.status;
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.laserjet": {DeviceID: "hass.laserjet", Instances: []string{"protocols-server-2"}, Capabilities: map[string]THassBridgeCapability{
			"status": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.hewlett_packard_hp_laserjet_professional_p1102w"}},
		}},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	link, ok := result.Administration.DeviceConceptualLinks["hass.laserjet"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[hass.laserjet] to be populated")
	}
	if _, found := link.AttributeEntityIDs["status"]; !found {
		t.Errorf("expected the valid line after the bad one to still register, got %v", link.AttributeEntityIDs)
	}
}
