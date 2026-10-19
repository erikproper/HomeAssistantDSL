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
	const miniDSL = `device hass.laserjet with:
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
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", nil, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil, nil)
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
	const miniDSL = `device hass.laserjet with:
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
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", nil, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil, nil)
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
// ("this construct doesn't support that kind yet") -- confirms the merged "device <device-id> [as
// ...] with: ... end;" block registers a cpu-type host's explicitly-referenced node (2026-09-24:
// no longer auto-registered by positioning, an ordinary "entity ...;" body line like any other
// capability) and explicitly-named attributes identically to what "with: all entities;" used to
// bulk-imply, without needing that flag at all.
func TestForDeviceBlockRegistersHostsCapabilities(t *testing.T) {
	const miniDSL = `device host.xanadu with:
  entity binary_sensor.infrastructural:xanadu/node     from binary_sensor.node;
  entity sensor.infrastructural:xanadu/cpu/load        from sensor.cpu/load;
  entity sensor.infrastructural:xanadu/cpu/temperature from sensor.cpu/temperature;
end;`

	hostDevicesByID := map[string]THostDevice{
		"host.xanadu": {DeviceID: "host.xanadu", HostName: "xanadu", IntegrationType: "cpu"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", nil, &report, hostDevicesByID, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	link, ok := result.Administration.DeviceConceptualLinks["host.xanadu"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[host.xanadu] to be populated")
	}
	if link.NodeEntityID == "" {
		t.Errorf("link.NodeEntityID = %q, want the node entity registered via the explicit \"node\" reference", link.NodeEntityID)
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
	const miniDSL = `device host.xanadu with:
  entity sensor.infrastructural:xanadu/bogus from bogus;
end;`

	hostDevicesByID := map[string]THostDevice{
		"host.xanadu": {DeviceID: "host.xanadu", HostName: "xanadu", IntegrationType: "cpu"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", nil, &report, hostDevicesByID, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	link := result.Administration.DeviceConceptualLinks["host.xanadu"]
	if _, found := link.AttributeEntityIDs["bogus"]; found {
		t.Errorf("expected \"bogus\" to be rejected (not a real cpu-type attribute), got %v", link.AttributeEntityIDs)
	}
}

// TestHostCapabilityEntityLinkResolvesNode confirms "node" resolves like any other explicit
// capability reference for a hosts device (2026-09-24: no longer auto-registered at positioning
// time for any kind, including hosts -- registerHostCapabilityEntityLink gained a "node" branch
// instead of rejecting it, populating link.NodeEntityID/NodeDeviceClass/NodeIcon exactly like
// autoMaterializePhysicalCapability's own synthetic path already did).
func TestHostCapabilityEntityLinkResolvesNode(t *testing.T) {
	const miniDSL = `device host.xanadu with:
  entity binary_sensor.infrastructural:xanadu/node from binary_sensor.node;
end;`

	hostDevicesByID := map[string]THostDevice{
		"host.xanadu": {DeviceID: "host.xanadu", HostName: "xanadu", IntegrationType: "cpu"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", nil, &report, hostDevicesByID, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	link, ok := result.Administration.DeviceConceptualLinks["host.xanadu"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[host.xanadu] to be populated")
	}
	if link.NodeEntityID == "" {
		t.Errorf("link.NodeEntityID = %q, want it set from the explicit \"node\" reference", link.NodeEntityID)
	}
	if _, found := link.AttributeEntityIDs["node"]; found {
		t.Errorf("link.AttributeEntityIDs = %v, want no \"node\" entry -- it resolves into NodeEntityID, not the ordinary attribute map", link.AttributeEntityIDs)
	}
}

// TestHostCapabilityEntityLinkResolvesCompoundNodeLeafViaNoRenameShorthand is the regression test
// for a real bug found live 2026-09-29 (Junglinster's ring_chime_hallway/corridor migration): a
// BARE, no-rename entity spec whose own compound leaf ends in "/node" (no "as" leaf on the device,
// no explicit "from" clause at all -- "entity binary_sensor.infrastructural:ring_chime/node;")
// must resolve to a hosts device's own "node" capability, exactly the way it already does for
// every OTHER integration kind (discovery/hassbridge/import/local/logical), via
// resolveLongestSuffixCapability's own general progressive-suffix-stripping mechanism
// (Conceptual_DeviceCapabilityEntities.go, confirmed already wired in uniformly for every kind via
// deviceHasCapability). The gap was narrow: deviceHasCapability's own "hosts" branch checked only
// mat.AttributeNames(device), which never includes "node" (a hosts device's own "node" is
// synthesized on the fly from a ping check, never a real map entry) -- so the one candidate
// resolveLongestSuffixCapability was stripping DOWN TO always came back "no" for hosts devices
// specifically, silently defeating the already-general mechanism for this one kind only.
func TestHostCapabilityEntityLinkResolvesCompoundNodeLeafViaNoRenameShorthand(t *testing.T) {
	const miniDSL = `device appliance.ring_chime_hallway with:
  entity binary_sensor.infrastructural:ring_chime/node;
end;`

	hostDevicesByID := map[string]THostDevice{
		"appliance.ring_chime_hallway": {DeviceID: "appliance.ring_chime_hallway", HostName: "ring-chime-hallway", IntegrationType: "ping"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", nil, &report, hostDevicesByID, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	link, ok := result.Administration.DeviceConceptualLinks["appliance.ring_chime_hallway"]
	if !ok {
		t.Fatalf("expected DeviceConceptualLinks[appliance.ring_chime_hallway] to be populated")
	}
	if link.NodeEntityID == "" {
		t.Errorf("link.NodeEntityID = %q, want it resolved via the compound \"ring_chime/node\" no-rename leaf", link.NodeEntityID)
	}
	wantSuffix := "ring_chime_node"
	if !strings.HasSuffix(toHomeAssistantEntityID(link.NodeEntityID), wantSuffix) {
		t.Errorf("NodeEntityID = %q, want it to preserve the full \"ring_chime/node\" conceptual path (ending %q)", link.NodeEntityID, wantSuffix)
	}
}

// TestLogicalAbsorbResolvesHostsNodeWithNoPriorPositioning is the regression test for a real gap
// found live 2026-09-29, minutes after the compound-leaf fix above: Junglinster's own
// "device appliance.ring_chime_hallway with: capabilities from node.ring_chime_hallway:
// binary_sensor.node; end; end;" -- absorbing a hosts/ping device's own "node" capability, where
// that hosts device (node.ring_chime_hallway) has NO Conceptual.def positioning of its own
// anywhere at all (its liveness is otherwise only ever reached indirectly, via Logical.def's own
// "condition node.X;" bare-device-id sugar). Before this fix, absorb's own recursive delegate call
// (registerLogicalAbsorbedCapability -> registerDeviceCapabilityEntityLinkAllowingHidden ->
// registerHostCapabilityEntityLink) required administration.DeviceConceptualLinks[deviceID] to
// ALREADY exist for the "node" case specifically -- i.e. the source hosts device needed at least
// one OTHER direct reference somewhere first -- even though the exact same absence was already
// proven safe to tolerate by autoMaterializePhysicalCapability's own hosts-kind "node" branch,
// which "condition node.X;" already relies on successfully. Confirmed by the user directly: a
// device referenced only indirectly is the desired semantics, not a state to reject.
func TestLogicalAbsorbResolvesHostsNodeWithNoPriorPositioning(t *testing.T) {
	// The absorb block itself is represented via logicalDevicesByID below (pre-parsed Logical.def
	// content, passed as Go structs) -- this miniDSL is Conceptual.def-shaped content only: the one
	// line that positions and so triggers resolution of appliance.ring_chime_hallway's own
	// (absorbed) "node" capability.
	const miniDSL = `entity binary_sensor.infrastructural:ring_chime/node from appliance.ring_chime_hallway binary_sensor.node;`

	hostDevicesByID := map[string]THostDevice{
		"node.ring_chime_hallway": {DeviceID: "node.ring_chime_hallway", HostName: "ring-chime-hallway", IntegrationType: "ping"},
	}
	logicalDevicesByID := map[string]TLogicalDevice{
		"appliance.ring_chime_hallway": {
			DeviceID: "appliance.ring_chime_hallway",
			Capabilities: map[string]TLogicalCapability{
				"node": {Domain: "binary_sensor", AbsorbedFromDeviceID: "node.ring_chime_hallway", AbsorbedFromCapability: "node"},
			},
		},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", nil, &report, hostDevicesByID, nil, nil, nil, nil, nil, logicalDevicesByID, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if report.Len() != 0 {
		t.Fatalf("expected no warnings, got: %s", report.String())
	}
	// node.ring_chime_hallway (the absorbed-FROM device) never gets its own Conceptual.def
	// positioning at all -- confirming this really did resolve with no pre-existing link, not by
	// some other path incidentally populating it first.
	if _, hasOwnPositioning := result.Administration.DeviceConceptualLinks["node.ring_chime_hallway"]; !hasOwnPositioning {
		t.Fatalf("expected node.ring_chime_hallway's own DeviceConceptualLinks entry to exist NOW (created by the absorb resolution itself), even though it has no direct positioning line of its own")
	} else if result.Administration.DeviceConceptualLinks["node.ring_chime_hallway"].NodeEntityID == "" {
		t.Errorf("node.ring_chime_hallway's own NodeEntityID is empty, want it resolved")
	}
}

// TestHostCapabilityEntityLinkDefersWithoutPriorPositioning confirms a capability-link line
// appearing BEFORE its device's positioning line in the same file still resolves correctly (via
// the single retry ParseEntitiesAndFillAdministration makes after the whole file is read), mirroring
// the same order-independence the hassbridge/import branches already have.
func TestHostCapabilityEntityLinkDefersWithoutPriorPositioning(t *testing.T) {
	const miniDSL = `entity sensor.infrastructural:xanadu/cpu/load from host.xanadu sensor.cpu/load;
device host.xanadu with:
end;`

	hostDevicesByID := map[string]THostDevice{
		"host.xanadu": {DeviceID: "host.xanadu", HostName: "xanadu", IntegrationType: "cpu"},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", nil, &report, hostDevicesByID, nil, nil, nil, nil, nil, nil, nil)
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
  device appliance.washing_machine with:
    entity binary_sensor.infrastructural:washing_machine/node from binary_sensor.node;
    entity sensor.status from sensor.status;
    entity switch.social: from switch.core;
    entity sensor.social:power from sensor.power;
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
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", nil, &report, nil, discoveryGatewaysByID, hassBridgeDevicesByID, nil, nil, nil, logicalDevicesByID, nil)
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
  device sensors.terrace_motion with:
    entity binary_sensor.physical:motion from binary_sensor.core;
    entity sensor.physical:illuminance from sensor.illuminance;
    entity input_number.social:sunny_threshold from input_number.sunny_threshold;
    entity binary_sensor.social:sunny from binary_sensor.sunny;
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
					DerivedCondition: testJinjaCondition("($1 | int) > ($2 | int)", "sensor.illuminance", "input_number.sunny_threshold"),
				},
			},
		},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", nil, &report, nil, discoveryGatewaysByID, nil, nil, nil, nil, logicalDevicesByID, nil)
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

	rec, found := findEntityRecordByName(admin, "binary_sensor.social/terrace/sunny")
	if !found {
		t.Fatalf("no entity record registered for binary_sensor.social/terrace/sunny")
	}
	if rec.ConditionExpr == "" {
		t.Fatalf("ConditionExpr is empty, want a rendered expression referencing both illuminance and sunny_threshold")
	}
}

// TestNoRenameShorthandMatchesLongestCapabilitySuffix is the regression test for the real Vienna
// bug (2026-09-25): "entity binary_sensor.infrastructural:aqara/node;" (no "from" clause -- the
// no-rename shorthand) used to infer the capability name as the WHOLE compound leaf text
// ("aqara/node"), which no device ever declares literally -- it must instead try progressively
// shorter "/"-delimited suffixes and match the device's real "node" capability. Confirmed with the
// user: "kk/ll/mm" must match a capability "mm" (not "l/mm", which isn't a real suffix), and prefer
// a capability "ll/mm" over a bare "mm" when the SAME device happens to declare both (see
// TestValidateNoOverlappingCapabilitySuffixes for why that dual-declaration itself is rejected).
func TestNoRenameShorthandMatchesLongestCapabilitySuffix(t *testing.T) {
	const miniDSL = `device sensors.hallway_aqara_windoor with:
  entity binary_sensor.social: from core;
  entity sensor.infrastructural:aqara/battery_level;
  entity sensor.physical:aqara/humidity;
end;`

	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{
		"sensors.hallway_aqara_windoor": {
			DeviceID: "sensors.hallway_aqara_windoor",
			Capabilities: map[string]TDiscoveryCapability{
				"core":          {Domain: "binary_sensor", Leaf: "0x00158d0003d4e964_contact_zigbee2mqtt"},
				"battery_level": {Domain: "sensor", Leaf: "0x00158d0003d4e964_battery_zigbee2mqtt"},
				"humidity":      {Domain: "sensor", Leaf: "0x00158d0003d4e964_humidity_zigbee2mqtt"},
			},
		},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", nil, &report, nil, discoveryGatewaysByID, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	admin := result.Administration

	for label, entityID := range map[string]string{
		"battery_level": "sensor.infrastructural_aqara_battery_level",
		"humidity":      "sensor.physical_aqara_humidity",
	} {
		if _, ok := admin.DiscoveryEntityLinks[entityID]; !ok {
			keys := make([]string, 0, len(admin.DiscoveryEntityLinks))
			for k := range admin.DiscoveryEntityLinks {
				keys = append(keys, k)
			}
			t.Errorf("%s: expected DiscoveryEntityLinks[%q] to exist (longest-suffix match), got keys %v", label, entityID, keys)
		}
	}
}

// TestNoRenameShorthandDoesNotMatchPartialSegment covers the negative case the user explicitly
// called out: a leaf "kk/ll/mm" must NOT match a capability "l/mm" -- "l" is not a whole "/"-
// delimited segment of the leaf path ("ll" is), so this must still warn, never silently match a
// substring.
func TestNoRenameShorthandDoesNotMatchPartialSegment(t *testing.T) {
	if got := resolveLongestSuffixCapability("kk/ll/mm", func(candidate string) bool {
		return candidate == "l/mm"
	}); got != "kk/ll/mm" {
		t.Errorf("resolveLongestSuffixCapability(%q) = %q, want the unchanged original (no real suffix matches)", "kk/ll/mm", got)
	}
	if got := resolveLongestSuffixCapability("kk/ll/mm", func(candidate string) bool {
		return candidate == "ll/mm" || candidate == "mm"
	}); got != "ll/mm" {
		t.Errorf("resolveLongestSuffixCapability(%q) = %q, want the LONGEST match %q when both %q and %q exist", "kk/ll/mm", got, "ll/mm", "ll/mm", "mm")
	}
}

// TestDeferredCapabilityLinkResolvesAgainstItsOwnDeclaredSpace is the regression test for
// PROJECT.md item 6's real bug, found live 2026-09-07: Vienna's "entity
// switch.social:picture_frame from host.frame slideshow;", positioned inside a nested space
// but BEFORE host.frame's own "device ... from host.frame;" positioning appeared later in the same
// file, was deferred (registerDeviceCapabilityEntityLink's own retry-after-full-parse mechanism,
// parser.go's pendingCapabilityLinks) and then, on retry, resolved against whatever
// administration.SpacePath happened to be AFTER the whole file was read (back at "root", both
// spaces closed) instead of the space the reference line actually sat in -- producing
// "switch.social_picture_frame" instead of the wanted
// "switch.social_apartment_living_room_picture_frame". Fixed by snapshotting
// administration.SpacePath into pendingCapabilityLink at defer time and restoring it around each
// retry call (parser.go).
func TestDeferredCapabilityLinkResolvesAgainstItsOwnDeclaredSpace(t *testing.T) {
	const dsl = `space social:apartment with:
  space social:living_room with:
    entity switch.social:picture_frame from hass.frame switch.slideshow;
  end;
end;
device hass.frame with:
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.frame": {DeviceID: "hass.frame", Instances: []string{"protocols-server-2"}, Capabilities: map[string]THassBridgeCapability{
			"node":      {Domain: "binary_sensor", Sources: map[string]string{"protocols-server-2": "sensor.frame is available"}},
			"slideshow": {Domain: "switch", Sources: map[string]string{"protocols-server-2": "switch.frame_slideshow"}},
		}},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", nil, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	link := result.Administration.DeviceConceptualLinks["hass.frame"]
	slideshow, ok := link.AttributeEntityIDs["slideshow"]
	if !ok || slideshow.EntityID != "switch.social_apartment_living_room_picture_frame" {
		t.Errorf("slideshow (deferred capability link) = %+v, ok=%v, want EntityID switch.social_apartment_living_room_picture_frame", slideshow, ok)
	}
}

func TestForDeviceBlockWarnsOnUnrecognisedLineAndKeepsParsing(t *testing.T) {
	const miniDSL = `device hass.laserjet with:
  this is not a valid line;
  entity sensor.status from sensor.status;
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.laserjet": {DeviceID: "hass.laserjet", Instances: []string{"protocols-server-2"}, Capabilities: map[string]THassBridgeCapability{
			"status": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.hewlett_packard_hp_laserjet_professional_p1102w"}},
		}},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(miniDSL, "\n"), nil, "test.def", nil, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil, nil)
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
