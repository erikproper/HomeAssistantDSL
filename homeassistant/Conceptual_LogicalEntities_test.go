package main

import (
	"fmt"
	"strings"
	"testing"
)

// testJinjaCondition builds a TConditionExpr tree equivalent to the pre-2026-09-24 flat
// "condition "<expr>" over: <sources...>; end;" shape -- for tests exercising
// registerLogicalDerivedConditionCapability's own resolution/deferral behaviour, which doesn't
// depend on the new grammar's boolean composition (and/or/not/nested jinja clauses) at all.
//
// Takes the old "$1"/"$2"-positional expr + sources shape (2026-09-24 grammar) and translates it
// into the current "$[domain.label]"-embedded shape (2026-10-05, PROJECT.md 1h) internally --
// "$N" becomes "$[sources[N-1]]" -- so every existing call site (and its own assertions, which
// only ever check the FINAL resolved entity reference, not the intermediate template text) keeps
// working unchanged.
func testJinjaCondition(expr string, sources ...string) *TConditionExpr {
	// No bare-"$" fallback here (unlike the old render-side buildConditionStateExpr): every real
	// call site below always writes "$1"/"$2" explicitly, and a blind "$" replacement would
	// corrupt the "$[...]" placeholders this loop itself just inserted, since they also contain
	// "$". Substituted from the HIGHEST index down so "$1" can never accidentally match inside an
	// already-substituted "$[...]" from a later (lower) index's own source name.
	for i := len(sources) - 1; i >= 0; i-- {
		expr = strings.ReplaceAll(expr, fmt.Sprintf("$%d", i+1), "$["+sources[i]+"]")
	}
	return &TConditionExpr{Kind: CondJinja, JinjaExpr: expr}
}

// testRefCondition builds a lone CondRef leaf (a bare sibling label/entity reference, optionally
// "is available") -- the "value <ref>;"/"condition <ref> [is available];" atomic shape.
func testRefCondition(ref string, isAvailable bool) *TConditionExpr {
	return &TConditionExpr{Kind: CondRef, Ref: ref, IsAvailable: isAvailable}
}

// testFromCondition builds a CondRef leaf carrying a "from [physical] <device-id>" cross-device
// qualifier (2026-09-24).
func testFromCondition(ref, fromDeviceID string, forcePhysical bool) *TConditionExpr {
	return &TConditionExpr{Kind: CondRef, Ref: ref, FromDeviceID: fromDeviceID, ForcePhysical: forcePhysical}
}

// TestRegisterLogicalDerivedConditionCapabilityResolvesFromDeviceID covers a "<label> from
// <device-id>" reference (2026-09-24) resolving an EXPLICITLY OTHER device's own already-positioned
// physical capability -- switch.washing_machine's own "core" (the plug's main switch), referenced
// from appliance.washing_machine's own override condition.
func TestRegisterLogicalDerivedConditionCapabilityResolvesFromDeviceID(t *testing.T) {
	admin := newAdministrationState()
	admin.DeviceConceptualLinks["switch.washing_machine"] = TDeviceConceptualLink{
		AttributeEntityIDs: map[string]TDeviceAttributeLink{
			"core": {EntityID: "switch.social_apartment_shower_room_washing_machine"},
		},
	}

	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "binary_sensor.infrastructural:node", DeviceID: "appliance.washing_machine", Capability: "binary_sensor.node"}
	capability := TLogicalCapability{
		Domain:             "binary_sensor",
		IsDerivedCondition: true,
		DerivedCondition:   &TConditionExpr{Kind: CondNot, Child: testFromCondition("switch.core", "switch.washing_machine", false)},
	}
	warnings, deferred := registerLogicalDerivedConditionCapability(admin, decl, capability, "node", "washing_machine", "Logical.def", 1, true, nil, nil, nil, nil, nil, nil)
	if deferred {
		t.Fatalf("did not expect deferral -- switch.washing_machine's \"core\" is already positioned")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	rec, found := findEntityRecordByName(admin, "binary_sensor.infrastructural/washing_machine/node")
	if !found {
		t.Fatalf("no entity record registered for binary_sensor.infrastructural/washing_machine/node")
	}
	if !strings.Contains(rec.ConditionExpr, "switch.social_apartment_shower_room_washing_machine") {
		t.Errorf("ConditionExpr = %q, want it to reference switch.washing_machine's own resolved \"core\" entity", rec.ConditionExpr)
	}
}

// TestRegisterLogicalDerivedConditionCapabilityAutoMaterializesFromPhysicalHassBridgeCapability
// covers the key new capability this session's redesign unlocks: "node from physical
// appliance.washing_machine" reaching a hassbridge device's own raw Physical.def "node" capability
// that was NEVER positioned in Conceptual.def at all (because the override precedence skips its
// auto-registration once a Logical.def override claims "node") -- auto-materialized on demand with
// the "physical/<device-id>/<label>" synthetic name, exactly the user's own worked example.
func TestRegisterLogicalDerivedConditionCapabilityAutoMaterializesFromPhysicalHassBridgeCapability(t *testing.T) {
	admin := newAdministrationState()
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"appliance.washing_machine": {
			DeviceID:     "appliance.washing_machine",
			Capabilities: map[string]THassBridgeCapability{"node": {Domain: "binary_sensor"}},
		},
	}

	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "binary_sensor.infrastructural:node", DeviceID: "appliance.washing_machine", Capability: "binary_sensor.node"}
	capability := TLogicalCapability{
		Domain:             "binary_sensor",
		IsDerivedCondition: true,
		DerivedCondition:   testFromCondition("node", "appliance.washing_machine", true),
	}
	warnings, deferred := registerLogicalDerivedConditionCapability(admin, decl, capability, "node", "washing_machine", "Logical.def", 1, true, nil, hassBridgeDevicesByID, nil, nil, nil, nil)
	if deferred {
		t.Fatalf("did not expect deferral")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	wantEntity := "binary_sensor.physical_appliance_washing_machine_node"
	rec, found := findEntityRecordByName(admin, "binary_sensor.infrastructural/washing_machine/node")
	if !found {
		t.Fatalf("no entity record registered for binary_sensor.infrastructural/washing_machine/node")
	}
	if !strings.Contains(rec.ConditionExpr, wantEntity) {
		t.Errorf("ConditionExpr = %q, want it to reference the auto-materialized %q", rec.ConditionExpr, wantEntity)
	}
	link := admin.DeviceConceptualLinks["appliance.washing_machine"]
	if link.AttributeEntityIDs["node"].EntityID != wantEntity {
		t.Errorf("DeviceConceptualLinks[appliance.washing_machine].AttributeEntityIDs[node] = %+v, want EntityID=%q (so a second reference reuses it)", link.AttributeEntityIDs["node"], wantEntity)
	}
}

// TestRegisterLogicalDerivedConditionCapabilityResolvesBareDeviceIDAsNode covers the "bare
// device-id atom" sugar (2026-09-24): "node.candy" used directly as a condition atom means "that
// device's own node capability" -- folding the old device-level "dependency on node.candy;" effect
// into an ordinary condition atom. Reuses the SAME auto-materialization path already proven for
// "hosts"-kind targets (the pre-existing "node.candy" -> "physical_node_candy_node" precedent).
func TestRegisterLogicalDerivedConditionCapabilityResolvesBareDeviceIDAsNode(t *testing.T) {
	admin := newAdministrationState()
	hostDevicesByID := map[string]THostDevice{
		"node.candy": {DeviceID: "node.candy", HostName: "candy", IntegrationType: "ping"},
	}

	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "binary_sensor.infrastructural:node", DeviceID: "appliance.washing_machine", Capability: "binary_sensor.node"}
	capability := TLogicalCapability{
		Domain:             "binary_sensor",
		IsDerivedCondition: true,
		DerivedCondition:   testRefCondition("node.candy", false),
	}
	warnings, deferred := registerLogicalDerivedConditionCapability(admin, decl, capability, "node", "washing_machine", "Logical.def", 1, true, nil, nil, nil, hostDevicesByID, nil, nil)
	if deferred {
		t.Fatalf("did not expect deferral")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	rec, found := findEntityRecordByName(admin, "binary_sensor.infrastructural/washing_machine/node")
	if !found {
		t.Fatalf("no entity record registered for binary_sensor.infrastructural/washing_machine/node")
	}
	if !strings.Contains(rec.ConditionExpr, "binary_sensor.physical_node_candy_node") {
		t.Errorf("ConditionExpr = %q, want it to reference node.candy's own auto-materialized node entity", rec.ConditionExpr)
	}
}

// TestRegisterLogicalDerivedConditionCapabilityForcePhysicalBypassesLogicalOverride confirms
// "from physical <device-id>" skips tier-1 (an already-positioned Logical.def override for the
// same label) while a plain "from <device-id>" (no "physical") still finds it -- the override
// precedence's own escape hatch.
func TestRegisterLogicalDerivedConditionCapabilityForcePhysicalBypassesLogicalOverride(t *testing.T) {
	admin := newAdministrationState()
	admin.LogicalEntityLinks["binary_sensor.social_override_node"] = TLogicalEntityLink{DeviceID: "appliance.thing", Label: "node"}
	admin.DeviceConceptualLinks["appliance.thing"] = TDeviceConceptualLink{
		AttributeEntityIDs: map[string]TDeviceAttributeLink{"node": {EntityID: "binary_sensor.physical_thing_node"}},
	}

	logicalDecl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.infrastructural:x", DeviceID: "other.device", Capability: "sensor.x"}
	logicalCap := TLogicalCapability{Domain: "sensor", IsDerivedValue: true, DerivedValue: testFromCondition("node", "appliance.thing", false)}
	if _, deferred := registerLogicalDerivedConditionCapability(admin, logicalDecl, logicalCap, "x", "other", "Logical.def", 1, true, nil, nil, nil, nil, nil, nil); deferred {
		t.Fatalf("did not expect deferral for the plain \"from\" reference")
	}
	recLogical, _ := findEntityRecordByName(admin, "sensor.infrastructural/other/x")
	if !strings.Contains(recLogical.ConditionExpr, "binary_sensor.social_override_node") {
		t.Errorf("ConditionExpr = %q, want a plain \"from\" reference to resolve the logical override", recLogical.ConditionExpr)
	}

	physicalDecl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.infrastructural:y", DeviceID: "other.device", Capability: "sensor.y"}
	physicalCap := TLogicalCapability{Domain: "sensor", IsDerivedValue: true, DerivedValue: testFromCondition("node", "appliance.thing", true)}
	if _, deferred := registerLogicalDerivedConditionCapability(admin, physicalDecl, physicalCap, "y", "other", "Logical.def", 1, true, nil, nil, nil, nil, nil, nil); deferred {
		t.Fatalf("did not expect deferral for the \"from physical\" reference")
	}
	recPhysical, _ := findEntityRecordByName(admin, "sensor.infrastructural/other/y")
	if !strings.Contains(recPhysical.ConditionExpr, "binary_sensor.physical_thing_node") {
		t.Errorf("ConditionExpr = %q, want \"from physical\" to bypass the logical override and resolve the raw physical entity", recPhysical.ConditionExpr)
	}
}

func findEntityRecordByName(admin *TAdministrationState, name string) (TEntityRecord, bool) {
	for _, records := range admin.EntityRecordsBySpace {
		for _, rec := range records {
			if rec.Name == name {
				return rec, true
			}
		}
	}
	return TEntityRecord{}, false
}

// TestRegisterLogicalAbsorbedCapability covers the "absorb" operation's capability-level dispatch
// (2026-09-18): a device's own "capabilities from <device-id>: ... end;" block line delegates
// straight to the absorbed device's own normal registration path (here, a discovery gateway),
// positioned under the HOST device's own deviceNamePath ("washing_machine", not the absorbed
// device's own leaf).
func TestRegisterLogicalAbsorbedCapability(t *testing.T) {
	admin := newAdministrationState()
	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"discovery.washing_machine_switch": {
			DeviceID:    "discovery.washing_machine_switch",
			Identifiers: []string{"zigbee2mqtt_0x9035eafffe694513"},
			Capabilities: map[string]TDiscoveryCapability{
				"core": {Domain: "switch", Leaf: "0x9035eafffe694513_switch_zigbee2mqtt"},
			},
		},
	}

	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "switch.social:", DeviceID: "appliance.washing_machine", Capability: "switch.core"}
	capability := TLogicalCapability{Domain: "switch", AbsorbedFromDeviceID: "discovery.washing_machine_switch", AbsorbedFromCapability: "core"}

	warnings, deferred := registerLogicalAbsorbedCapability(admin, decl, capability, "core", "washing_machine", discoveryGatewaysByID, nil, nil, nil, nil, nil, map[string]TLogicalDevice{}, "Logical.def", 1, true)
	if deferred {
		t.Fatalf("did not expect deferral")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	var found bool
	for _, link := range admin.DiscoveryEntityLinks {
		if link.GatewayDeviceID == "discovery.washing_machine_switch" && link.Leaf == "0x9035eafffe694513_switch_zigbee2mqtt" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the absorbed capability to resolve via the underlying discovery gateway, got %+v", admin.DiscoveryEntityLinks)
	}
}

// TestAutoMaterializePhysicalCapabilityDiscoveryKindPropagatesStableID is a regression test for a
// real bug found live 2026-10-05, Step 7's PROJECT.md item 7 rollout: the discovery-kind branch of
// autoMaterializePhysicalCapability (the "switched by"/"dependency on" auto-materialization path --
// exactly how e.g. "device X with light.core is switched by ...;" one-liner sugar resolves its own
// light entity) built its own TDiscoveryEntityLink without copying TDiscoveryCapability.AssignedID
// at all, unlike registerDiscoveryEntityLink/registerDiscoveryDerivedFromRawLeafEntityLink (which
// both do) -- silently dropping a frozen stable id for every capability that happens to be
// auto-materialized this way, discovered live on Junglinster's own "sensors.house_kitchen_
// workplace_dimmer" after bootstrapping.
func TestAutoMaterializePhysicalCapabilityDiscoveryKindPropagatesStableID(t *testing.T) {
	admin := newAdministrationState()
	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"sensors.house_kitchen_workplace_dimmer": {
			DeviceID: "sensors.house_kitchen_workplace_dimmer",
			Capabilities: map[string]TDiscoveryCapability{
				"core": {Domain: "light", Leaf: "0x00124b0019e7af0c_light_zigbee2mqtt", AssignedID: "sensors_house_kitchen_workplace_dimmer_0x00124b0019e7af0c_light_zigbee2mqtt"},
			},
		},
	}

	entityID, ok := autoMaterializePhysicalCapability(admin, "sensors.house_kitchen_workplace_dimmer", "core", discoveryGatewaysByID, nil, nil, nil, "test")
	if !ok {
		t.Fatalf("expected auto-materialization to succeed")
	}
	link, found := admin.DiscoveryEntityLinks[entityID]
	if !found {
		t.Fatalf("no DiscoveryEntityLinks entry for %q", entityID)
	}
	if link.StableID != "sensors_house_kitchen_workplace_dimmer_0x00124b0019e7af0c_light_zigbee2mqtt" {
		t.Errorf("StableID = %q, want the capability's own AssignedID carried through", link.StableID)
	}
}

// TestRegisterLogicalAbsorbedCapabilityFromHostsDeviceIgnoresSourcesOwnShadowIdentity is a
// regression test for a real bug found live 2026-10-04, migrating Junglinster's
// server.xanadu/server.backups (the first-ever absorb of a HOSTS-kind device's own attribute into
// a DIFFERENT logical device): registerHostCapabilityEntityLink's attribute branch used to trust
// link.HostIdentity unconditionally, but for an absorbed source device that was NEVER itself
// directly positioned, that field can hold autoMaterializePhysicalCapability's own "physical/
// <device-id>" shadow identity instead (stamped there as a side effect of the source device's bare
// "node.X" liveness being referenced elsewhere, e.g. a hand-written Logical.def "node" condition) --
// producing a wrong, shadow-named entity instead of one positioned under the ABSORBING device's own
// identity. Reproduces the exact ordering that triggered it live: the "node" shadow is
// auto-materialized FIRST (simulating the condition reference), then the absorb runs.
func TestRegisterLogicalAbsorbedCapabilityFromHostsDeviceIgnoresSourcesOwnShadowIdentity(t *testing.T) {
	admin := newAdministrationState()
	hostDevicesByID := map[string]THostDevice{
		"node.xanadu": {DeviceID: "node.xanadu", HostName: "xanadu", IntegrationType: "cpu"},
	}

	// Simulate the source device's own "node" bare-condition reference running first, exactly as
	// Logical.def's own hand-written "condition node.xanadu or jinja ...;" does -- this is what
	// stamps the "physical/node.xanadu" shadow identity onto DeviceConceptualLinks["node.xanadu"].
	if _, ok := autoMaterializePhysicalCapability(admin, "node.xanadu", "node", nil, nil, hostDevicesByID, nil, "test"); !ok {
		t.Fatalf("expected the \"node\" auto-materialization to succeed")
	}
	if sphere := admin.DeviceConceptualLinks["node.xanadu"].HostIdentity.Sphere; sphere != "physical" {
		t.Fatalf("test setup: expected node.xanadu's own HostIdentity to be the \"physical\" shadow, got %q", sphere)
	}

	// Now the absorb itself: server.xanadu absorbs node.xanadu's own "cpu/load" attribute.
	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.infrastructural:cpu/load", DeviceID: "server.xanadu", Capability: "sensor.cpu/load"}
	capability := TLogicalCapability{Domain: "sensor", AbsorbedFromDeviceID: "node.xanadu", AbsorbedFromCapability: "cpu/load"}
	warnings, deferred := registerLogicalAbsorbedCapability(admin, decl, capability, "cpu/load", "xanadu", nil, nil, nil, hostDevicesByID, nil, nil, map[string]TLogicalDevice{}, "Logical.def", 1, true)
	if deferred {
		t.Fatalf("did not expect deferral")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	got := admin.DeviceConceptualLinks["node.xanadu"].AttributeEntityIDs["load"].EntityID
	want := "sensor.infrastructural_xanadu_cpu_load"
	if got != want {
		t.Errorf("entity_id = %q, want %q (positioned under the ABSORBING device's own identity, not the source's own \"physical\" shadow)", got, want)
	}
}

// TestResolveLogicalSiblingSourceEntityIDDeterministicWithMultipleDiscoveryMatches is the
// regression test for a real bug found live 2026-09-27, a THIRD instance of the same
// non-determinism class as the local-kind registerLocalMediaSwitchCapability's own sibling-node
// loop (Conceptual_LocalEntities.go): resolveLogicalSiblingSourceEntityID's own discovery-tier lookup
// used to break on the first DiscoveryEntityLinks match for a (GatewayDeviceID, Leaf) pair while
// ranging over a Go map -- confirmed live regenerating Junglinster's server_room/corridor
// "switched by" pilot (two conceptual positionings genuinely sharing the same raw leaf: an
// explicit one kept for an unrelated "follows" reference, and the switched-by mechanism's own
// auto-materialized entity) producing different `derived binary_sensor.node` output across two
// consecutive, otherwise-unchanged ./generate runs.
func TestResolveLogicalSiblingSourceEntityIDDeterministicWithMultipleDiscoveryMatches(t *testing.T) {
	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"switches.zwave_048": {
			DeviceID:     "switches.zwave_048",
			Capabilities: map[string]TDiscoveryCapability{"light_1": {Domain: "light", Leaf: "48-37-1-currentValue"}},
		},
	}
	for i := 0; i < 20; i++ {
		admin := newAdministrationState()
		admin.DiscoveryEntityLinks["light.physical_house_corridor_main"] = TDiscoveryEntityLink{EntityID: "light.physical_house_corridor_main", GatewayDeviceID: "switches.zwave_048", Leaf: "48-37-1-currentValue"}
		admin.DiscoveryEntityLinks["light.physical_switches_zwave_048_1"] = TDiscoveryEntityLink{EntityID: "light.physical_switches_zwave_048_1", GatewayDeviceID: "switches.zwave_048", Leaf: "48-37-1-currentValue"}

		id, ok, isLiteral := resolveLogicalSiblingSourceEntityID(admin, "switches.zwave_048", "light_1", false, false, false, discoveryGatewaysByID, nil, nil, nil, "test")
		if !ok || isLiteral {
			t.Fatalf("run %d: ok=%v isLiteral=%v, want ok=true isLiteral=false", i, ok, isLiteral)
		}
		want := "light.physical_house_corridor_main" // lexicographically first of the two candidates
		if id != want {
			t.Errorf("run %d: id = %q, want the same deterministic choice %q every time", i, id, want)
		}
	}
}

// TestRegisterLogicalMediaSwitchCapabilityDeterministicWithMultipleNodeMatches is the regression
// test for a real bug found live 2026-09-27: two consecutive ./generate runs, no code or .def
// change in between, produced DIFFERENT output for appliance.sonos_playbar_livingroom's own media
// switch, because the sibling "node" reverse lookup broke out of a map range on the FIRST match --
// Go's own map iteration order is randomised per process, so "first" was pure chance whenever more
// than one LogicalEntityLinks entry genuinely matched this device's own "node" label. The fix picks
// the lexicographically-first candidate deterministically; this test seeds two matching entries
// (deliberately inserted in an order that would, pre-fix, sometimes iterate the "wrong" one first)
// and asserts the SAME one wins every time.
// TestRegisterLogicalDefinedInputNumberCapability covers the "windy" threshold's own shape: a
// "defined input_number.<label> with: minimum/maximum/step/icon/units;" capability, reusing
// TEntityRecord's plain InputNumber* fields wholesale.
func TestRegisterLogicalDefinedInputNumberCapability(t *testing.T) {
	admin := newAdministrationState()
	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "input_number.social:windy_threshold", DeviceID: "sensors.vienna_terrace_wind", Capability: "input_number.windy_threshold"}
	capability := TLogicalCapability{
		Domain: "input_number", IsDefinedInputNumber: true,
		DefinedMinimum: "0", DefinedMaximum: "30", DefinedStep: "1", DefinedIcon: "mdi:weather-windy", DefinedUnits: "km/h",
	}

	warnings := registerLogicalDefinedInputNumberCapability(admin, decl, capability, "windy_threshold", "vienna_terrace_wind", "Logical.def", 1)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	rec, found := findEntityRecordByName(admin, "input_number.social/vienna_terrace_wind/windy_threshold")
	if !found {
		t.Fatalf("no entity record registered for input_number.social/vienna_terrace_wind/windy_threshold")
	}
	if rec.InputNumberMin != "0" || rec.InputNumberMax != "30" || rec.InputNumberStep != "1" || rec.InputNumberIcon != "mdi:weather-windy" || rec.InputNumberUnit != "km/h" {
		t.Errorf("rec = %+v, want all five InputNumber* fields populated from the capability", rec)
	}
	link, ok := admin.LogicalEntityLinks[toHomeAssistantEntityID(rec.Name)]
	if !ok || link.DeviceID != "sensors.vienna_terrace_wind" || link.Label != "windy_threshold" {
		t.Errorf("LogicalEntityLinks[%q] = %+v, ok=%v, want {sensors.vienna_terrace_wind, windy_threshold}", toHomeAssistantEntityID(rec.Name), link, ok)
	}
}

// TestRegisterLogicalDerivedConditionCapability covers the "windy" capability itself: a
// multi-source condition resolving BOTH its "over:" siblings by reverse lookup on
// LogicalEntityLinks, in "over:" order ($1=wind_speed, $2=windy_threshold).
func TestRegisterLogicalDerivedConditionCapability(t *testing.T) {
	admin := newAdministrationState()
	// Simulate both siblings already positioned -- exactly what a real Conceptual.def would have
	// done by the time "windy" itself is reached, given the order in the draft.
	admin.LogicalEntityLinks["sensor.social_apartment_terrace_netatmo_windmeter_wind_speed"] = TLogicalEntityLink{DeviceID: "sensors.vienna_terrace_wind", Label: "wind_speed"}
	admin.LogicalEntityLinks["input_number.social_windy_threshold"] = TLogicalEntityLink{DeviceID: "sensors.vienna_terrace_wind", Label: "windy_threshold"}
	// Their own DECLARATIONS (2026-09-28, mandatory jinja-source domain-qualification): a
	// domain-qualified jinja source is only resolved as a sibling capability, never a literal
	// entity_id, when lookupCapabilityDomain confirms the bare label is a REAL declared capability
	// of this device -- these two maps are what a real Physical.def/Logical.def declaration would
	// have produced by the time "windy" itself is registered.
	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"sensors.vienna_terrace_wind": {DeviceID: "sensors.vienna_terrace_wind", Capabilities: map[string]TDiscoveryCapability{"wind_speed": {Domain: "sensor"}}},
	}
	logicalDevicesByID := map[string]TLogicalDevice{
		"sensors.vienna_terrace_wind": {DeviceID: "sensors.vienna_terrace_wind", Capabilities: map[string]TLogicalCapability{"windy_threshold": {Domain: "input_number"}}},
	}

	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "binary_sensor.social:windy", DeviceID: "sensors.vienna_terrace_wind", Capability: "binary_sensor.windy"}
	capability := TLogicalCapability{
		Domain:             "binary_sensor",
		IsDerivedCondition: true,
		DerivedCondition:   testJinjaCondition("($1 in ['unknown', 'unavailable']) or (($1 | int) > ($2 | int))", "sensor.wind_speed", "input_number.windy_threshold"),
		DerivedDeviceClass: "wind",
		DelayOn:            "00:01:00",
		DelayOff:           "00:10:00",
	}

	warnings, deferred := registerLogicalDerivedConditionCapability(admin, decl, capability, "windy", "vienna_terrace_wind", "Logical.def", 1, true, discoveryGatewaysByID, nil, nil, nil, nil, logicalDevicesByID)
	if deferred {
		t.Fatalf("did not expect deferral -- both siblings are already positioned")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	rec, found := findEntityRecordByName(admin, "binary_sensor.social/vienna_terrace_wind/windy")
	if !found {
		t.Fatalf("no entity record registered for binary_sensor.social/vienna_terrace_wind/windy")
	}
	if rec.ConditionSources != nil {
		t.Errorf("ConditionSources = %v, want nil -- the new grammar fully substitutes at registration time", rec.ConditionSources)
	}
	wantSources := []string{"sensor.social_apartment_terrace_netatmo_windmeter_wind_speed", "input_number.social_windy_threshold"}
	for _, want := range wantSources {
		if !strings.Contains(rec.ConditionExpr, want) {
			t.Errorf("ConditionExpr = %q, want it to reference resolved sibling %q", rec.ConditionExpr, want)
		}
	}
	if strings.Index(rec.ConditionExpr, wantSources[0]) > strings.Index(rec.ConditionExpr, wantSources[1]) {
		t.Errorf("ConditionExpr = %q, want wind_speed ($1) substituted before windy_threshold ($2)", rec.ConditionExpr)
	}
	if rec.ConditionDevClass != "wind" || rec.ConditionDelayOn != "00:01:00" || rec.ConditionDelayOff != "00:10:00" {
		t.Errorf("rec = %+v, want ConditionDevClass=wind ConditionDelayOn=00:01:00 ConditionDelayOff=00:10:00", rec)
	}
	if !rec.HasDefinitionOrImport {
		t.Errorf("HasDefinitionOrImport = false, want true -- this entity is generator-authored, not assumed to already exist on main (real bug found live 2026-09-21, see this file's header comment)")
	}
}

// TestRegisterLogicalDerivedConditionCapabilityRejectsBareConditionRef is the regression test for
// the user's own explicit correction, 2026-09-29 ("that 'core' without domain should be illegal"):
// a plain condition/value sibling reference with no domain prefix ("brews") must now be a hard
// error, generalizing the mandatory-domain rule already enforced for jinja "{ ... }" sources and
// "entity X from Y Z;". Mirrors TestRegisterLogicalDerivedConditionCapability's own harness shape.
func TestRegisterLogicalDerivedConditionCapabilityRejectsBareConditionRef(t *testing.T) {
	admin := newAdministrationState()
	admin.LogicalEntityLinks["binary_sensor.social_house_kitchen_nespresso_brews"] = TLogicalEntityLink{DeviceID: "switch.house_kitchen_nespresso", Label: "brews"}
	logicalDevicesByID := map[string]TLogicalDevice{
		"switch.house_kitchen_nespresso": {DeviceID: "switch.house_kitchen_nespresso", Capabilities: map[string]TLogicalCapability{
			"brews": {Domain: "binary_sensor"},
		}},
	}

	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "binary_sensor.social:in_use", DeviceID: "switch.house_kitchen_nespresso", Capability: "binary_sensor.in_use"}
	bareCondition, warnings := parseConditionExpr("brews", nil, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings building the raw condition expression: %v", warnings)
	}
	capability := TLogicalCapability{Domain: "binary_sensor", IsDerivedCondition: true, DerivedCondition: bareCondition, DelayOff: "00:10:00"}

	warnings, deferred := registerLogicalDerivedConditionCapability(admin, decl, capability, "in_use", "house_kitchen_nespresso", "Logical.def", 1, true, nil, nil, nil, nil, nil, logicalDevicesByID)
	if deferred {
		t.Fatalf("did not expect deferral -- this is a pure syntax rule, not something a later line could resolve")
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], `write "binary_sensor.brews"`) {
		t.Fatalf("warnings = %v, want exactly one warning suggesting \"binary_sensor.brews\"", warnings)
	}

	// The domain-qualified form must succeed cleanly.
	admin2 := newAdministrationState()
	admin2.LogicalEntityLinks["binary_sensor.social_house_kitchen_nespresso_brews"] = TLogicalEntityLink{DeviceID: "switch.house_kitchen_nespresso", Label: "brews"}
	qualifiedCondition, warnings := parseConditionExpr("binary_sensor.brews", nil, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings building the qualified condition expression: %v", warnings)
	}
	capability.DerivedCondition = qualifiedCondition
	warnings, deferred = registerLogicalDerivedConditionCapability(admin2, decl, capability, "in_use", "house_kitchen_nespresso", "Logical.def", 1, true, nil, nil, nil, nil, nil, logicalDevicesByID)
	if deferred {
		t.Fatalf("did not expect deferral")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings for the domain-qualified form: %v", warnings)
	}
}

// TestResolveLeafDefersRatherThanLiteralFallbackForRealCapabilityNotYetPositioned is the
// regression test for a real bug found live 2026-09-29, minutes after mandatory
// domain-qualification first landed for a plain condition/value ref: a domain-qualified token that
// genuinely names a REAL declared capability of the device (confirmed via lookupCapabilityDomain)
// must defer (not literal-fallback) when that capability hasn't been positioned in
// DeviceConceptualLinks YET (finalAttempt=false, simulating an earlier file-processing pass) --
// exactly mirroring how a jinja "{ ... }" source already behaves. Before this fix,
// resolveLogicalSiblingSourceEntityID's tier-4 (`!mustBeSibling && strings.Contains(token, ".")`)
// had no notion of finalAttempt at all, so it happily accepted "media_player.core" as a LITERAL
// entity_id on the very first attempt -- permanently locking in the wrong answer
// (`states('media_player.core')`) instead of ever resolving to the real
// `media_player.social_house_living_room_tv` once that positioning was parsed. Confirmed live:
// Junglinster's own appliance.tv_living_room "enabled by" sugar.
func TestResolveLeafDefersRatherThanLiteralFallbackForRealCapabilityNotYetPositioned(t *testing.T) {
	logicalDevicesByID := map[string]TLogicalDevice{
		"appliance.tv_living_room": {DeviceID: "appliance.tv_living_room", Capabilities: map[string]TLogicalCapability{}},
	}
	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"appliance.tv_living_room": {DeviceID: "appliance.tv_living_room", Capabilities: map[string]TDiscoveryCapability{"core": {Domain: "media_player"}}},
	}
	tree, warnings := parseConditionExpr("media_player.core is available", nil, "test")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings building the condition expression: %v", warnings)
	}
	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "binary_sensor.infrastructural:node", DeviceID: "appliance.tv_living_room", Capability: "binary_sensor.node"}
	capability := TLogicalCapability{Domain: "binary_sensor", IsDerivedCondition: true, DerivedCondition: tree}

	// Pass 1: the device's own "core" capability has NOT been positioned in DeviceConceptualLinks
	// yet (finalAttempt=false, simulating a Conceptual.def line appearing later in the file). Must
	// defer, not silently "succeed" via a literal-entity-id misreading.
	admin := newAdministrationState()
	_, deferred := registerLogicalDerivedConditionCapability(admin, decl, capability, "node", "tv_living_room", "Logical.def", 1, false, discoveryGatewaysByID, nil, nil, nil, nil, logicalDevicesByID)
	if !deferred {
		t.Fatalf("expected a deferral on pass 1 (sibling not yet positioned), got resolved instead")
	}
	rec, found := findEntityRecordByName(admin, "binary_sensor.infrastructural/tv_living_room/node")
	if found && strings.Contains(rec.ConditionExpr, "media_player.core") {
		t.Fatalf("ConditionExpr = %q, must not have locked in the literal \"media_player.core\" on a deferred pass", rec.ConditionExpr)
	}

	// Pass 2: NOW the sibling is positioned (matching real file processing, where Conceptual.def's
	// own "entity media_player.social: from media_player.core;" line has been parsed by this
	// point) and finalAttempt=true. Must resolve to the REAL entity, not the literal token.
	admin.DeviceConceptualLinks["appliance.tv_living_room"] = TDeviceConceptualLink{
		AttributeEntityIDs: map[string]TDeviceAttributeLink{"core": {EntityID: "media_player.social_house_living_room_tv"}},
	}
	warnings, deferred = registerLogicalDerivedConditionCapability(admin, decl, capability, "node", "tv_living_room", "Logical.def", 1, true, discoveryGatewaysByID, nil, nil, nil, nil, logicalDevicesByID)
	if deferred {
		t.Fatalf("did not expect a deferral on pass 2 -- the sibling is now positioned")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings on pass 2: %v", warnings)
	}
	rec, found = findEntityRecordByName(admin, "binary_sensor.infrastructural/tv_living_room/node")
	if !found {
		t.Fatalf("no entity record registered for binary_sensor.infrastructural/tv_living_room/node")
	}
	if !strings.Contains(rec.ConditionExpr, "media_player.social_house_living_room_tv") {
		t.Errorf("ConditionExpr = %q, want it to reference the REAL resolved sibling entity, not a literal token", rec.ConditionExpr)
	}
	if strings.Contains(rec.ConditionExpr, "'media_player.core'") {
		t.Errorf("ConditionExpr = %q, must not contain the literal unresolved token \"media_player.core\"", rec.ConditionExpr)
	}
}

// TestResolveTargetedReusesAlreadyClaimedSourceForAbsorbedCapability is the regression test for a
// real gap found live 2026-09-29 (Junglinster office's "cycling" smart plug, shared as the real
// "enabler" for both apple_tv and tv, while ALSO having its own genuine direct Conceptual.def
// positioning -- confirmed with the user, not a copy/paste mistake): before this fix, an absorbed
// capability (e.g. the "enabled by" sugar's own synthesized "enabler") with no positioning of its
// own OWN either warned "already used as a different conceptual entity" (if given a redundant
// positioning) or "has no positioning yet" (if left unpositioned) -- both wrong when the SAME
// underlying source is legitimately already claimed elsewhere. resolveTargeted now reuses that
// existing entity_id directly, per the user's own explicit instruction ("then re-use the same
// one").
func TestResolveTargetedReusesAlreadyClaimedSourceForAbsorbedCapability(t *testing.T) {
	admin := newAdministrationState()
	// Simulate "device switch.house_office_cycling with entity switch.social:cycling from
	// switch.core;" already having been positioned earlier in the file.
	admin.ConceptualUseBySource["switch.house_office_cycling!core"] = TConceptualSourceUse{
		EntityName: "switch.social/house/office/cycling",
	}

	logicalDevicesByID := map[string]TLogicalDevice{
		"appliance.tv_office": {
			DeviceID: "appliance.tv_office",
			Capabilities: map[string]TLogicalCapability{
				"enabler": {Domain: "switch", AbsorbedFromDeviceID: "switch.house_office_cycling", AbsorbedFromCapability: "core"},
			},
		},
	}

	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "binary_sensor.infrastructural:node", DeviceID: "appliance.tv_office", Capability: "binary_sensor.node"}
	capability := TLogicalCapability{
		Domain:             "binary_sensor",
		IsDerivedCondition: true,
		DerivedCondition:   testJinjaCondition("$1 == 'off'", "switch.enabler"),
	}

	warnings, deferred := registerLogicalDerivedConditionCapability(admin, decl, capability, "node", "tv_office", "Logical.def", 1, true, nil, nil, nil, nil, nil, logicalDevicesByID)
	if deferred {
		t.Fatalf("did not expect a deferral -- the absorbed source is already conceptually claimed")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings (no redundant positioning was ever added, so no collision should be reported): %v", warnings)
	}

	rec, found := findEntityRecordByName(admin, "binary_sensor.infrastructural/tv_office/node")
	if !found {
		t.Fatalf("no entity record registered for binary_sensor.infrastructural/tv_office/node")
	}
	if !strings.Contains(rec.ConditionExpr, "switch.social_house_office_cycling") {
		t.Errorf("ConditionExpr = %q, want it to reuse the already-positioned cycling switch's own real entity_id", rec.ConditionExpr)
	}
}

// TestRegisterLogicalDerivedConditionCapabilityExcludedFromMainEntityIDs is the end-to-end version
// of the HasDefinitionOrImport assertion above: a derived-condition capability's own entity must
// never appear in collectMainEntityIDs' output (main_entities.go), since that list feeds the
// coordinator's kind-5 "assumed to already exist on main" check -- a generator-authored template
// entity failing that check right up until its own first deploy is the exact bug found live
// 2026-09-21 (Vienna's environment.weather "node" capability, auto-positioned with no explicit
// Conceptual.def "entity ...;" line).
func TestRegisterLogicalDerivedConditionCapabilityExcludedFromMainEntityIDs(t *testing.T) {
	admin := newAdministrationState()
	admin.LogicalEntityLinks["weather.forecast"] = TLogicalEntityLink{DeviceID: "environment.weather", Label: "forecast"}

	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "binary_sensor.infrastructural:node", DeviceID: "environment.weather", Capability: "binary_sensor.node"}
	capability := TLogicalCapability{
		Domain:             "binary_sensor",
		IsDerivedCondition: true,
		DerivedCondition:   testRefCondition("forecast", true),
	}
	if _, deferred := registerLogicalDerivedConditionCapability(admin, decl, capability, "node", "weather", "Logical.def", 1, true, nil, nil, nil, nil, nil, nil); deferred {
		t.Fatalf("did not expect deferral")
	}

	ids := collectMainEntityIDs(admin)
	for _, id := range ids {
		if id == "binary_sensor.infrastructural_weather_node" {
			t.Fatalf("collectMainEntityIDs = %v, must not include the generator-authored derived-condition entity", ids)
		}
	}
}

// TestRegisterLogicalDerivedConditionCapabilityRegistersLiteralOverAsExternalReference covers the
// user's own real-world follow-up (2026-09-21): a literal "over:" entity_id (weather.forecast, used
// verbatim since it didn't resolve as a sibling capability label) must now surface in
// collectMainEntityIDs -- same live-existence tracking a genuine "imported"/hassbridge-declared
// entity already gets -- rather than just an unconfirmable local presence-check note forever.
func TestRegisterLogicalDerivedConditionCapabilityRegistersLiteralOverAsExternalReference(t *testing.T) {
	admin := newAdministrationState()

	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.infrastructural:pressure", DeviceID: "environment.weather", Capability: "sensor.pressure"}
	capability := TLogicalCapability{
		Domain:         "sensor",
		IsDerivedValue: true,
		DerivedValue:   testRefCondition("weather.forecast!pressure", false),
	}
	if _, deferred := registerLogicalDerivedConditionCapability(admin, decl, capability, "pressure", "weather", "Logical.def", 1, true, nil, nil, nil, nil, nil, nil); deferred {
		t.Fatalf("did not expect deferral")
	}

	ids := collectMainEntityIDs(admin)
	found := false
	for _, id := range ids {
		if id == "weather.forecast" {
			found = true
		}
	}
	if !found {
		t.Fatalf("collectMainEntityIDs = %v, want it to include the literal \"over:\" reference weather.forecast (stripped of its !attribute suffix)", ids)
	}
}

// TestRegisterLogicalDerivedConditionCapabilityDedupsRepeatedLiteralOverReference covers the exact
// live case that made a plain AppendEntityRecord call unsafe here: environment.weather's own "node"
// and "pressure" capabilities both reference weather.forecast (one bare, one with "!pressure") --
// registering it twice must never produce AppendEntityRecord's "defined more than once" warning.
func TestRegisterLogicalDerivedConditionCapabilityDedupsRepeatedLiteralOverReference(t *testing.T) {
	admin := newAdministrationState()

	nodeDecl := TDeviceCapabilityEntityDeclaration{LocalSpec: "binary_sensor.infrastructural:node", DeviceID: "environment.weather", Capability: "binary_sensor.node"}
	nodeCapability := TLogicalCapability{
		Domain:             "binary_sensor",
		IsDerivedCondition: true,
		DerivedCondition:   testRefCondition("weather.forecast", true),
	}
	if _, deferred := registerLogicalDerivedConditionCapability(admin, nodeDecl, nodeCapability, "node", "weather", "Logical.def", 1, true, nil, nil, nil, nil, nil, nil); deferred {
		t.Fatalf("did not expect deferral for node")
	}

	pressureDecl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.infrastructural:pressure", DeviceID: "environment.weather", Capability: "sensor.pressure"}
	pressureCapability := TLogicalCapability{
		Domain:         "sensor",
		IsDerivedValue: true,
		DerivedValue:   testRefCondition("weather.forecast!pressure", false),
	}
	if _, deferred := registerLogicalDerivedConditionCapability(admin, pressureDecl, pressureCapability, "pressure", "weather", "Logical.def", 1, true, nil, nil, nil, nil, nil, nil); deferred {
		t.Fatalf("did not expect deferral for pressure")
	}

	count := 0
	for _, records := range admin.EntityRecordsBySpace {
		for _, rec := range records {
			if rec.Name == "weather.forecast" {
				count++
			}
		}
	}
	if count != 1 {
		t.Fatalf("weather.forecast registered %d times, want exactly 1 (dedup must apply across separate capabilities referencing the same literal entity)", count)
	}
}

// TestRegisterExternalEntityReferenceSkipsAlreadyDeclaredFinalID covers the real regression found
// live 2026-09-21 (originally via the since-removed registerLogicalIsAvailableCapability, which
// called this same method for every "is available" capability's own Entity): a reference naming
// the EXACT SAME final id a real declaration already claims (e.g. a device's own bare "entity
// media_player.social:sonos;" shorthand line) must not be registered as a SECOND "external
// reference" record -- RegisterExternalEntityReference must recognise the final id is already
// claimed by a real declaration and skip silently instead, or validateNoDuplicateFinalEntityIDs
// trips a false positive.
func TestRegisterExternalEntityReferenceSkipsAlreadyDeclaredFinalID(t *testing.T) {
	admin := newAdministrationState()
	spaceName := admin.CurrentSpaceName()
	admin.AppendEntityRecord(spaceName, TEntityRecord{
		Name:                  "media_player.social/apartment/living_room/sonos",
		Identity:              extractEntityIdentity("media_player.social/apartment/living_room/sonos"),
		HasDefinitionOrImport: true,
	})

	admin.RegisterExternalEntityReference(spaceName, "media_player.social_apartment_living_room_sonos", "test")

	count := 0
	for _, records := range admin.EntityRecordsBySpace {
		for _, rec := range records {
			if toHomeAssistantEntityID(rec.Name) == "media_player.social_apartment_living_room_sonos" {
				count++
			}
		}
	}
	if count != 1 {
		t.Fatalf("media_player.social_apartment_living_room_sonos registered %d times, want exactly 1 -- an external reference must not re-register an already-declared entity", count)
	}
}

// TestRegisterLogicalDerivedConditionCapabilityDefersUntilSiblingPositioned covers the ordering
// dependency: "windy" can't resolve until BOTH its "over:" siblings have their own
// LogicalEntityLinks entry -- deferred=true (no warning) lets the caller retry once after the
// whole file has been read, same convention as registerDiscoveryAvailabilityEntityLink's own
// sibling lookup.
func TestRegisterLogicalDerivedConditionCapabilityDefersUntilSiblingPositioned(t *testing.T) {
	admin := newAdministrationState()
	// Both siblings DECLARED (2026-09-28: a jinja source is only ever a sibling reference once
	// lookupCapabilityDomain confirms it names a real declared capability -- otherwise it'd be
	// (wrongly) treated as an immediately-resolvable literal, never deferring at all) but NEITHER
	// one's own entity_id is POSITIONED yet -- this is exactly the ordering gap this test covers.
	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"sensors.vienna_terrace_wind": {DeviceID: "sensors.vienna_terrace_wind", Capabilities: map[string]TDiscoveryCapability{"wind_speed": {Domain: "sensor"}}},
	}
	logicalDevicesByID := map[string]TLogicalDevice{
		"sensors.vienna_terrace_wind": {DeviceID: "sensors.vienna_terrace_wind", Capabilities: map[string]TLogicalCapability{"windy_threshold": {Domain: "input_number"}}},
	}
	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "binary_sensor.social:windy", DeviceID: "sensors.vienna_terrace_wind", Capability: "binary_sensor.windy"}
	capability := TLogicalCapability{
		Domain:             "binary_sensor",
		IsDerivedCondition: true,
		DerivedCondition:   testJinjaCondition("($1|int) > ($2|int)", "sensor.wind_speed", "input_number.windy_threshold"),
	}

	warnings, deferred := registerLogicalDerivedConditionCapability(admin, decl, capability, "windy", "vienna_terrace_wind", "Logical.def", 1, false, discoveryGatewaysByID, nil, nil, nil, nil, logicalDevicesByID)
	if !deferred {
		t.Fatalf("expected deferral with no sibling positioned yet")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings on a non-final deferred attempt: %v", warnings)
	}

	// finalAttempt=true with the sibling still missing must report a real warning, not silently
	// stay deferred forever.
	warnings, deferred = registerLogicalDerivedConditionCapability(admin, decl, capability, "windy", "vienna_terrace_wind", "Logical.def", 1, true, discoveryGatewaysByID, nil, nil, nil, nil, logicalDevicesByID)
	if deferred {
		t.Fatalf("finalAttempt=true must not defer")
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "wind_speed") {
		t.Errorf("warnings = %v, want exactly one warning naming the missing %q sibling", warnings, "sensor.wind_speed")
	}
}

// TestRegisterLogicalDerivedConditionCapabilityResolvesAbsorbedSibling is a regression test for
// the user's own explicit design (2026-09-18): "over:" resolution must see the COMPLETE logical
// device, including a sibling capability absorbed via "capabilities from ...;" -- not just
// ordinary IsAvailable/switch-coercion ones.
func TestRegisterLogicalDerivedConditionCapabilityResolvesAbsorbedSibling(t *testing.T) {
	admin := newAdministrationState()
	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"discovery.some_plug": {
			DeviceID: "discovery.some_plug",
			Capabilities: map[string]TDiscoveryCapability{
				"power": {Domain: "sensor", Leaf: "0xabc_power_zigbee2mqtt"},
			},
		},
	}
	// Absorb "power" onto appliance.thing, under label "power" -- registerLogicalAbsorbedCapability
	// must record a LogicalEntityLinks entry under appliance.thing/power for this lookup to work.
	absorbDecl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.social:power", DeviceID: "appliance.thing", Capability: "sensor.power"}
	absorbCapability := TLogicalCapability{Domain: "sensor", AbsorbedFromDeviceID: "discovery.some_plug", AbsorbedFromCapability: "power"}
	_, deferred := registerLogicalAbsorbedCapability(admin, absorbDecl, absorbCapability, "power", "thing", discoveryGatewaysByID, nil, nil, nil, nil, nil, map[string]TLogicalDevice{}, "Logical.def", 1, true)
	if deferred {
		t.Fatalf("did not expect deferral for the absorbed sibling itself")
	}

	// Now a "derived" capability on the SAME host device references that absorbed "power" by label.
	// Its own DECLARATION (2026-09-28, mandatory jinja-source domain-qualification) -- matching
	// absorbCapability above, as a real absorb pass would leave in logicalDevicesByID.
	logicalDevicesByID := map[string]TLogicalDevice{
		"appliance.thing": {DeviceID: "appliance.thing", Capabilities: map[string]TLogicalCapability{"power": absorbCapability}},
	}
	derivedDecl := TDeviceCapabilityEntityDeclaration{LocalSpec: "binary_sensor.social:high_power", DeviceID: "appliance.thing", Capability: "binary_sensor.high_power"}
	derivedCapability := TLogicalCapability{
		Domain:             "binary_sensor",
		IsDerivedCondition: true,
		DerivedCondition:   testJinjaCondition("($1|int) > 100", "sensor.power"),
	}
	warnings, deferred := registerLogicalDerivedConditionCapability(admin, derivedDecl, derivedCapability, "high_power", "thing", "Logical.def", 2, true, nil, nil, nil, nil, nil, logicalDevicesByID)
	if deferred {
		t.Fatalf("did not expect deferral -- the absorbed sibling is already positioned")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	rec, found := findEntityRecordByName(admin, "binary_sensor.social/thing/high_power")
	if !found {
		t.Fatalf("no entity record registered for binary_sensor.social/thing/high_power")
	}
	if !strings.Contains(rec.ConditionExpr, "sensor.social_thing_power") {
		t.Errorf("ConditionExpr = %q, want it to reference the absorbed sibling's own resolved entity_id", rec.ConditionExpr)
	}
}

// TestRegisterLogicalDerivedConditionCapabilityResolvesDiscoverySibling is a regression test for
// sensors.terrace_motion's own "sunny" case (2026-09-18): "over:" must also resolve a PLAIN
// capability of a discovery-kind device's own real physical presence (illuminance), not just
// Logical.def-declared or import/hassbridge ones -- registerDiscoveryEntityLink's own
// DiscoveryEntityLinks bookkeeping is keyed by entity_id with (GatewayDeviceID, Leaf), never
// touching LogicalEntityLinks or DeviceConceptualLinks.AttributeEntityIDs at all.
func TestRegisterLogicalDerivedConditionCapabilityResolvesDiscoverySibling(t *testing.T) {
	admin := newAdministrationState()
	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"sensors.terrace_motion": {
			DeviceID: "sensors.terrace_motion",
			Capabilities: map[string]TDiscoveryCapability{
				"illuminance": {Domain: "sensor", Leaf: "0xabc_illuminance_zigbee2mqtt"},
			},
		},
	}
	// Simulate "illuminance" already positioned via the ordinary discovery path.
	admin.DiscoveryEntityLinks["sensor.physical_terrace_signify_motion_illuminance"] = TDiscoveryEntityLink{
		EntityID: "sensor.physical_terrace_signify_motion_illuminance", GatewayDeviceID: "sensors.terrace_motion", Leaf: "0xabc_illuminance_zigbee2mqtt",
	}
	// And "sunny_threshold" positioned via the ordinary "defined" path.
	admin.LogicalEntityLinks["input_number.social_terrace_sunny_threshold"] = TLogicalEntityLink{DeviceID: "sensors.terrace_motion", Label: "sunny_threshold"}
	// Its own DECLARATION (2026-09-28, mandatory jinja-source domain-qualification).
	logicalDevicesByID := map[string]TLogicalDevice{
		"sensors.terrace_motion": {DeviceID: "sensors.terrace_motion", Capabilities: map[string]TLogicalCapability{"sunny_threshold": {Domain: "input_number"}}},
	}

	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "binary_sensor.social:sunny", DeviceID: "sensors.terrace_motion", Capability: "binary_sensor.sunny"}
	capability := TLogicalCapability{
		Domain:             "binary_sensor",
		IsDerivedCondition: true,
		DerivedCondition:   testJinjaCondition("($1 | int) > ($2 | int)", "sensor.illuminance", "input_number.sunny_threshold"),
	}

	warnings, deferred := registerLogicalDerivedConditionCapability(admin, decl, capability, "sunny", "terrace", "Logical.def", 1, true, discoveryGatewaysByID, nil, nil, nil, nil, logicalDevicesByID)
	if deferred {
		t.Fatalf("did not expect deferral -- both siblings are already positioned")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	rec, found := findEntityRecordByName(admin, "binary_sensor.social/terrace/sunny")
	if !found {
		t.Fatalf("no entity record registered for binary_sensor.social/terrace/sunny")
	}
	wantSources := []string{"sensor.physical_terrace_signify_motion_illuminance", "input_number.social_terrace_sunny_threshold"}
	for _, want := range wantSources {
		if !strings.Contains(rec.ConditionExpr, want) {
			t.Errorf("ConditionExpr = %q, want it to reference resolved sibling %q", rec.ConditionExpr, want)
		}
	}
}
