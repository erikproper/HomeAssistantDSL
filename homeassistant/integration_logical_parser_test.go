package main

import (
	"strings"
	"testing"
)

func TestParseLogicalLayerBody(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device node.vienna_livingroom with:",
		"  dependency on host.netatmo;",
		"end;",
		"",
		"# a comment",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	d := devices[0]
	if d.DeviceID != "node.vienna_livingroom" {
		t.Errorf("DeviceID = %q, want node.vienna_livingroom", d.DeviceID)
	}
	if len(d.DependsOn) != 1 || d.DependsOn[0] != "host.netatmo" {
		t.Errorf("DependsOn = %v, want [host.netatmo]", d.DependsOn)
	}
}

// TestParseLogicalLayerBodyMultipleDependencies covers a device depending on more than one other
// device -- "dependency on <id>;" is repeatable, one per line, no comma-list syntax.
func TestParseLogicalLayerBodyMultipleDependencies(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device node.a with:",
		"  dependency on host.b;",
		"  dependency on host.c;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	want := []string{"host.b", "host.c"}
	got := devices[0].DependsOn
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("DependsOn = %v, want %v", got, want)
	}
}

// TestParseLogicalLayerBodyOneLinerDependencyOn is the regression test for the picture_frame case
// found live 2026-09-25: a device declaring exactly one "dependency on <id>;" body line can now be
// written as a single-line shorthand, "device <id> with dependency on <id>;", instead of the full
// "with: ... end;" block -- must parse identically to the block form.
func TestParseLogicalLayerBodyOneLinerDependencyOn(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device appliance.picture_frame with dependency on node.picture_frame;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	d := devices[0]
	if d.DeviceID != "appliance.picture_frame" {
		t.Errorf("DeviceID = %q, want appliance.picture_frame", d.DeviceID)
	}
	if len(d.DependsOn) != 1 || d.DependsOn[0] != "node.picture_frame" {
		t.Errorf("DependsOn = %v, want [node.picture_frame]", d.DependsOn)
	}
}

// TestParseLogicalLayerBodyOneLinerBareCapability covers the other single-line-complete body
// shape -- a bare capability line -- shortened the same way.
func TestParseLogicalLayerBodyOneLinerBareCapability(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		`device utility.thing with binary_sensor.node media_player.core is available;`,
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	cap, ok := devices[0].Capabilities["node"]
	if !ok {
		t.Fatalf("Capabilities = %v, want a \"node\" entry", devices[0].Capabilities)
	}
	if cap.Domain != "binary_sensor" || cap.Entity != "media_player.core" || !cap.IsAvailable {
		t.Errorf("capability = %+v, want {Domain: binary_sensor, Entity: media_player.core, IsAvailable: true}", cap)
	}
}

// TestParseLogicalLayerBodyOneLinerRejectsBlockOpeningStatement confirms a block-opening body
// shape (e.g. "defined ... with: ...; end;") is NOT accepted as a one-liner's single statement --
// there is no one-line form for those, so this must warn rather than silently mis-parse.
func TestParseLogicalLayerBodyOneLinerRejectsBlockOpeningStatement(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device utility.thing with defined sensor.threshold with: minimum 0; end;",
	}, nil)
	if len(devices) != 0 {
		t.Errorf("got %d devices, want 0 (rejected one-liner shouldn't register a device)", len(devices))
	}
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "utility.thing") || !strings.Contains(warnings[0], "one-line") {
		t.Errorf("warning = %q, want it to name the device and call out the one-line restriction", warnings[0])
	}
}

// TestParseLogicalLayerBodyBlockFormStillWorksAlongsideOneLiner confirms the pre-existing
// multi-line "with: ... end;" block form is unaffected by the new one-liner pattern -- both can
// appear in the same file.
func TestParseLogicalLayerBodyBlockFormStillWorksAlongsideOneLiner(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device utility.fritz_box with:",
		"  dependency on node.fritz_box;",
		"end;",
		"device appliance.picture_frame with dependency on node.picture_frame;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2", len(devices))
	}
	if devices[0].DeviceID != "utility.fritz_box" || devices[1].DeviceID != "appliance.picture_frame" {
		t.Errorf("devices = %+v", devices)
	}
}

func TestParseLogicalLayerBodyWarnsOnMalformedLine(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device node.vienna_livingroom with:",
		"  depends on host.netatmo;", // old keyword, no longer recognised
		"end;",
	}, nil)
	if len(devices) != 1 || len(devices[0].DependsOn) != 0 {
		t.Errorf("got devices=%+v, want 1 device with no dependencies (the malformed line should be skipped, not silently accepted)", devices)
	}
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
}

// TestParseLogicalLayerBodyRejectsEntityDependencyForNow documents the current, deliberate scope
// limit: "dependency on entity <id>;"/"dependency on availability of entity <id>;" are planned
// (see this file's own header comment) but not built yet -- a two/four-token remainder must fall
// through to the generic warning rather than being misread as a device-id of "entity"/
// "availability".
func TestParseLogicalLayerBodyRejectsEntityDependencyForNow(t *testing.T) {
	_, warnings := parseLogicalLayerBody([]string{
		"device node.x with:",
		"  dependency on entity binary_sensor.kkk;",
		"end;",
	}, nil)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "unrecognised line") {
		t.Errorf("warnings = %v, want exactly one \"unrecognised line\" warning", warnings)
	}
}

// TestParseLogicalLayerBodyPlainIsAvailableCapability covers the media_player_device migration's
// simplest case (utility.apple_tv): a plain, self-contained "<domain>.<label>: <entity> is
// available;" line, no "with:" block at all.
func TestParseLogicalLayerBodyPlainIsAvailableCapability(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device utility.apple_tv with:",
		"  binary_sensor.node media_player.social_apartment_living_room_apple_tv is available;",
		"  switch.media media_player.social_apartment_living_room_apple_tv;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	node, ok := devices[0].Capabilities["node"]
	if !ok {
		t.Fatalf("Capabilities = %+v, want a \"node\" entry", devices[0].Capabilities)
	}
	if node.Domain != "binary_sensor" || node.Entity != "media_player.social_apartment_living_room_apple_tv" || !node.IsAvailable {
		t.Errorf("node capability = %+v, want Domain=binary_sensor Entity=media_player.social_apartment_living_room_apple_tv IsAvailable=true", node)
	}
	media, ok := devices[0].Capabilities["media"]
	if !ok {
		t.Fatalf("Capabilities = %+v, want a \"media\" entry", devices[0].Capabilities)
	}
	if media.Domain != "switch" || media.Entity != "media_player.social_apartment_living_room_apple_tv" || media.IsAvailable {
		t.Errorf("media capability = %+v, want Domain=switch Entity=media_player.social_apartment_living_room_apple_tv IsAvailable=false", media)
	}
}

// TestParseLogicalLayerBodyIsAvailableWithEnablerAndDelayOff covers utility.tv's own shape:
// enabler and delay_off nested inside the "node" capability's own "with:" block.
func TestParseLogicalLayerBodyIsAvailableWithEnablerAndDelayOff(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device utility.tv with:",
		"  binary_sensor.node media_player.social_apartment_living_room_tv is available with:",
		"    enabler switch.social_apartment_living_room_tv;",
		"    delay_off 00:01:00;",
		"  end;",
		"  switch.media media_player.social_apartment_living_room_tv;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	node, ok := devices[0].Capabilities["node"]
	if !ok {
		t.Fatalf("Capabilities = %+v, want a \"node\" entry", devices[0].Capabilities)
	}
	if !node.IsAvailable || node.Entity != "media_player.social_apartment_living_room_tv" {
		t.Errorf("node capability = %+v, want IsAvailable=true Entity=media_player.social_apartment_living_room_tv", node)
	}
	if node.EnablerEntity != "switch.social_apartment_living_room_tv" {
		t.Errorf("node.EnablerEntity = %q, want switch.social_apartment_living_room_tv", node.EnablerEntity)
	}
	if node.DelayOff != "00:01:00" {
		t.Errorf("node.DelayOff = %q, want 00:01:00", node.DelayOff)
	}
}

// TestParseLogicalLayerBodyIsAvailableWithDependencyOn is a regression test for the user's own
// explicit design (2026-09-16, retrofitting appliance.apple_tv/appliance.tv): "dependency on
// <device-id>;" is ALSO valid nested inside an "is available with:" capability body, scoped to
// that capability alone (TLogicalCapability.DependsOn) -- distinct from the device-level
// "dependency on" (TLogicalDevice.DependsOn, tested by TestParseLogicalLayerBody/
// TestParseLogicalLayerBodyMultipleDependencies above), which feeds a completely different
// consumer (resolveDependencyAvailabilityTopics's own MQTT depends_on_availability). Repeatable,
// and freely mixed with enabler/delay_off in any order.
func TestParseLogicalLayerBodyIsAvailableWithDependencyOn(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device appliance.tv with:",
		"  binary_sensor.node media_player.social_apartment_living_room_tv is available with:",
		"    dependency on node.appletv;",
		"    dependency on node.tv;",
		"    enabler switch.social_apartment_living_room_tv;",
		"    delay_off 00:01:00;",
		"  end;",
		"  switch.media media_player.social_apartment_living_room_tv;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	// The device-level DependsOn must stay empty -- this "dependency on" is capability-scoped only.
	if len(devices[0].DependsOn) != 0 {
		t.Errorf("device-level DependsOn = %v, want empty (capability-scoped only)", devices[0].DependsOn)
	}
	node, ok := devices[0].Capabilities["node"]
	if !ok {
		t.Fatalf("Capabilities = %+v, want a \"node\" entry", devices[0].Capabilities)
	}
	wantDeps := []string{"node.appletv", "node.tv"}
	if len(node.DependsOn) != len(wantDeps) || node.DependsOn[0] != wantDeps[0] || node.DependsOn[1] != wantDeps[1] {
		t.Errorf("node.DependsOn = %v, want %v", node.DependsOn, wantDeps)
	}
	if node.EnablerEntity != "switch.social_apartment_living_room_tv" || node.DelayOff != "00:01:00" {
		t.Errorf("enabler/delay_off should still parse alongside dependency on: %+v", node)
	}
}

// TestParseLogicalLayerBodyMediaSwitchWithNoPlayInput covers Junglinster's own sonos shape (the
// grammar this session built with that case in mind, even though only Vienna is migrated today).
func TestParseLogicalLayerBodyMediaSwitchWithNoPlayInput(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device utility.sonos with:",
		"  binary_sensor.node media_player.social_apartment_living_room_sonos is available;",
		"  switch.media media_player.social_apartment_living_room_sonos with:",
		"    no_play_input \"TV\";",
		"  end;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	media, ok := devices[0].Capabilities["media"]
	if !ok {
		t.Fatalf("Capabilities = %+v, want a \"media\" entry", devices[0].Capabilities)
	}
	if media.NoPlayInput != "TV" {
		t.Errorf("media.NoPlayInput = %q, want TV", media.NoPlayInput)
	}
}

// TestParseLogicalLayerBodyWarnsOnUnrecognisedCapabilityBodyLine covers a malformed line inside a
// capability's own "with:" block (not "enabler"/"delay_off"/"no_play_input").
func TestParseLogicalLayerBodyWarnsOnUnrecognisedCapabilityBodyLine(t *testing.T) {
	_, warnings := parseLogicalLayerBody([]string{
		"device utility.tv with:",
		"  binary_sensor.node media_player.social_apartment_living_room_tv is available with:",
		"    bogus_field 42;",
		"  end;",
		"end;",
	}, nil)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "node") {
		t.Errorf("warnings = %v, want exactly one warning naming the \"node\" capability", warnings)
	}
}

// TestParseLogicalLayerBodyAbsorbBlock covers the "absorb" operation's own grammar (2026-09-18):
// a device's "capabilities from <device-id>: ... end;" block and its repeatable
// "<domain>.<label>: <bare-capability>;" lines -- the concrete appliance.washing_machine/
// discovery.washing_machine_switch shape from the approved plan.
func TestParseLogicalLayerBodyAbsorbBlock(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device appliance.washing_machine with:",
		"  capabilities from discovery.washing_machine_switch:",
		"    switch.core            switch.core;",
		"    sensor.power           sensor.power;",
		"    sensor.energy          sensor.energy;",
		"    sensor.current         sensor.current;",
		"    binary_sensor.consumes binary_sensor.consumes;",
		"  end;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	d := devices[0]
	wantLabels := map[string]struct{ domain, capability string }{
		"core":     {"switch", "core"},
		"power":    {"sensor", "power"},
		"energy":   {"sensor", "energy"},
		"current":  {"sensor", "current"},
		"consumes": {"binary_sensor", "consumes"},
	}
	if len(d.Capabilities) != len(wantLabels) {
		t.Fatalf("Capabilities = %+v, want %d entries", d.Capabilities, len(wantLabels))
	}
	for label, want := range wantLabels {
		capEntry, ok := d.Capabilities[label]
		if !ok {
			t.Fatalf("Capabilities missing label %q: %+v", label, d.Capabilities)
		}
		if capEntry.Domain != want.domain {
			t.Errorf("Capabilities[%q].Domain = %q, want %q", label, capEntry.Domain, want.domain)
		}
		if capEntry.AbsorbedFromDeviceID != "discovery.washing_machine_switch" {
			t.Errorf("Capabilities[%q].AbsorbedFromDeviceID = %q, want discovery.washing_machine_switch", label, capEntry.AbsorbedFromDeviceID)
		}
		if capEntry.AbsorbedFromCapability != want.capability {
			t.Errorf("Capabilities[%q].AbsorbedFromCapability = %q, want %q", label, capEntry.AbsorbedFromCapability, want.capability)
		}
	}
}

// TestParseLogicalLayerBodyAbsorbBlockBareShorthand covers the 2026-09-19 "<domain>.<label>;"
// shorthand (no ":<value>" at all) -- registers identically to
// TestParseLogicalLayerBodyAbsorbBlock's explicit "<domain>.<label>: <value>;" form, valid because
// every label here is textually identical to the absorbed capability it names.
func TestParseLogicalLayerBodyAbsorbBlockBareShorthand(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device appliance.washing_machine with:",
		"  capabilities from discovery.washing_machine_switch:",
		"    switch.core;",
		"    sensor.power;",
		"    binary_sensor.consumes;",
		"  end;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	wantLabels := map[string]struct{ domain, capability string }{
		"core":     {"switch", "core"},
		"power":    {"sensor", "power"},
		"consumes": {"binary_sensor", "consumes"},
	}
	d := devices[0]
	if len(d.Capabilities) != len(wantLabels) {
		t.Fatalf("Capabilities = %+v, want %d entries", d.Capabilities, len(wantLabels))
	}
	for label, want := range wantLabels {
		capEntry, ok := d.Capabilities[label]
		if !ok {
			t.Fatalf("Capabilities missing label %q: %+v", label, d.Capabilities)
		}
		if capEntry.Domain != want.domain || capEntry.AbsorbedFromDeviceID != "discovery.washing_machine_switch" || capEntry.AbsorbedFromCapability != want.capability {
			t.Errorf("Capabilities[%q] = %+v, want Domain=%q AbsorbedFromDeviceID=discovery.washing_machine_switch AbsorbedFromCapability=%q", label, capEntry, want.domain, want.capability)
		}
	}
}

// TestParseLogicalLayerBodyRejectsOldColonForm is the regression test for the 2026-09-19 grammar
// restriction: logicalCapabilityPattern/logicalCapabilityWithBlockPattern briefly accepted an
// OPTIONAL ":" between <label> and <value> as a trial (both houses' Physical.def/Logical.def were
// then rewritten to the colon-less form and verified byte-identical on regenerate) -- now that
// the migration is complete, the old "switch.media: media_player.social:sonos;" colon form must
// be REJECTED outright, not silently tolerated.
func TestParseLogicalLayerBodyRejectsOldColonForm(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device appliance.sonos_complement with:",
		"  switch.media: media_player.social:sonos;",
		"end;",
	}, nil)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "unrecognised line") {
		t.Fatalf("expected exactly one \"unrecognised line\" warning for the old colon form, got: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	if _, ok := devices[0].Capabilities["media"]; ok {
		t.Errorf("Capabilities = %+v, want the old colon-form line rejected, not silently parsed", devices[0].Capabilities)
	}
}

// TestParseLogicalLayerBodyAbsorbBlockCoexistsWithOrdinaryCapability covers a device with BOTH an
// absorb block AND an ordinary "is available" capability of its own (appliance.washing_machine's
// real shape: the absorb block for the plug's capabilities, plus a wrapping "available" capability
// referencing the device's own already-positioned raw node) -- the two block kinds must not
// interfere with each other's state tracking.
func TestParseLogicalLayerBodyAbsorbBlockCoexistsWithOrdinaryCapability(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device appliance.washing_machine with:",
		"  binary_sensor.available binary_sensor.infrastructural_apartment_shower_room_washing_machine_node is available with:",
		"    enabler switch.social_apartment_shower_room_washing_machine;",
		"  end;",
		"  capabilities from discovery.washing_machine_switch:",
		"    switch.core switch.core;",
		"  end;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	d := devices[0]
	available, ok := d.Capabilities["available"]
	if !ok || !available.IsAvailable || available.EnablerEntity != "switch.social_apartment_shower_room_washing_machine" {
		t.Errorf("available capability = %+v, want IsAvailable=true with the given enabler", available)
	}
	core, ok := d.Capabilities["core"]
	if !ok || core.AbsorbedFromDeviceID != "discovery.washing_machine_switch" || core.AbsorbedFromCapability != "core" {
		t.Errorf("core capability = %+v, want an absorbed capability from discovery.washing_machine_switch", core)
	}
}

// TestParseLogicalLayerBodyWarnsOnUnrecognisedAbsorbBodyLine covers a malformed line inside an
// absorb block (neither "enabler ...;" nor "<domain>.<label>: <bare-capability>;").
func TestParseLogicalLayerBodyWarnsOnUnrecognisedAbsorbBodyLine(t *testing.T) {
	_, warnings := parseLogicalLayerBody([]string{
		"device appliance.washing_machine with:",
		"  capabilities from discovery.washing_machine_switch:",
		"    bogus_field 42;",
		"  end;",
		"end;",
	}, nil)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "capabilities from") {
		t.Errorf("warnings = %v, want exactly one warning naming the absorb block", warnings)
	}
}

// TestParseLogicalLayerBodyDefinedInputNumber covers the "defined <domain>.<label> with: ...;
// end;" block (2026-09-18) -- the "windy" threshold's own shape, replacing Macros.def's "windy"
// macro's "${x} = entity input_number.social/x_threshold with: ...;" line.
func TestParseLogicalLayerBodyDefinedInputNumber(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device sensors.vienna_terrace_wind with:",
		"  defined input_number.windy_threshold with:",
		"    minimum 0;",
		"    maximum 30;",
		"    step 1;",
		"    icon mdi:weather-windy;",
		"    units km/h;",
		"  end;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	capEntry, ok := devices[0].Capabilities["windy_threshold"]
	if !ok {
		t.Fatalf("Capabilities = %+v, want a %q entry", devices[0].Capabilities, "windy_threshold")
	}
	if !capEntry.IsDefinedInputNumber || capEntry.Domain != "input_number" {
		t.Errorf("capEntry = %+v, want IsDefinedInputNumber=true Domain=input_number", capEntry)
	}
	if capEntry.DefinedMinimum != "0" || capEntry.DefinedMaximum != "30" || capEntry.DefinedStep != "1" || capEntry.DefinedIcon != "mdi:weather-windy" || capEntry.DefinedUnits != "km/h" {
		t.Errorf("capEntry = %+v, want all five defined fields populated", capEntry)
	}
}

// TestParseLogicalLayerBodyDerivedCondition covers the "derived <domain>.<label> with: condition
// jinja "<expr>" { ... }; device_class ...; delay_on ...; delay_off ...; end;" block (2026-09-18,
// grammar redesigned 2026-09-24) -- the "windy" capability's own shape, replacing Macros.def's
// "windy" macro's "condition ${entity} ${windy_threshold} ...;" line.
func TestParseLogicalLayerBodyDerivedCondition(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device sensors.vienna_terrace_wind with:",
		`  derived binary_sensor.windy with:`,
		`    condition jinja "($1 in ['unknown', 'unavailable']) or (($1 | int) > ($2 | int))" { wind_speed, windy_threshold };`,
		"    device_class wind;",
		"    delay_on  00:01:00;",
		"    delay_off 00:10:00;",
		"  end;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	capEntry, ok := devices[0].Capabilities["windy"]
	if !ok {
		t.Fatalf("Capabilities = %+v, want a %q entry", devices[0].Capabilities, "windy")
	}
	if !capEntry.IsDerivedCondition || capEntry.Domain != "binary_sensor" {
		t.Errorf("capEntry = %+v, want IsDerivedCondition=true Domain=binary_sensor", capEntry)
	}
	if capEntry.DerivedCondition == nil || capEntry.DerivedCondition.Kind != CondJinja {
		t.Fatalf("DerivedCondition = %+v, want a single CondJinja node", capEntry.DerivedCondition)
	}
	wantExpr := "($1 in ['unknown', 'unavailable']) or (($1 | int) > ($2 | int))"
	if capEntry.DerivedCondition.JinjaExpr != wantExpr {
		t.Errorf("JinjaExpr = %q, want %q", capEntry.DerivedCondition.JinjaExpr, wantExpr)
	}
	wantOver := []string{"wind_speed", "windy_threshold"}
	gotOver := capEntry.DerivedCondition.JinjaSources
	if len(gotOver) != len(wantOver) || gotOver[0] != wantOver[0] || gotOver[1] != wantOver[1] {
		t.Errorf("JinjaSources = %v, want %v", gotOver, wantOver)
	}
	if capEntry.DerivedDeviceClass != "wind" || capEntry.DelayOn != "00:01:00" || capEntry.DelayOff != "00:10:00" {
		t.Errorf("capEntry = %+v, want DerivedDeviceClass=wind DelayOn=00:01:00 DelayOff=00:10:00", capEntry)
	}
}

// TestParseLogicalLayerBodyDefinedAndDerivedCoexist covers the full real "windy" device shape:
// both a "defined" and a "derived" block on the same device, alongside each other.
func TestParseLogicalLayerBodyDefinedAndDerivedCoexist(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device sensors.vienna_terrace_wind with:",
		"  defined input_number.windy_threshold with:",
		"    minimum 0;",
		"    maximum 30;",
		"    step 1;",
		"    icon mdi:weather-windy;",
		"    units km/h;",
		"  end;",
		`  derived binary_sensor.windy with:`,
		`    condition jinja "($1|int) > ($2|int)" { wind_speed, windy_threshold };`,
		"    device_class wind;",
		"    delay_on  00:01:00;",
		"    delay_off 00:10:00;",
		"  end;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 || len(devices[0].Capabilities) != 2 {
		t.Fatalf("got %+v, want 1 device with 2 capabilities", devices)
	}
}

// TestSubstituteSettingsVariables covers Logical.def's own "${name}" resolution (2026-09-18) --
// unlike Conceptual.def/Macros.def, Logical.def has no per-field or macro-argument resolution of
// its own, so collectLogicalDevicesByID substitutes over the whole raw body once, before parsing.
func TestSubstituteSettingsVariables(t *testing.T) {
	settings := map[string]string{"wind_speed_min": "0", "windy_device_class": `"safety"`}
	got := substituteSettingsVariables(`minimum ${wind_speed_min}; device_class ${windy_device_class};`, settings)
	want := `minimum 0; device_class safety;`
	if got != want {
		t.Errorf("substituteSettingsVariables = %q, want %q", got, want)
	}
}

// TestSubstituteSettingsVariablesLeavesUnknownNameUnchanged mirrors resolveSettingsVar's own
// silent-passthrough convention -- an unresolvable "${name}" stays literal rather than erroring.
func TestSubstituteSettingsVariablesLeavesUnknownNameUnchanged(t *testing.T) {
	got := substituteSettingsVariables(`icon ${bogus_unknown_name};`, map[string]string{})
	want := `icon ${bogus_unknown_name};`
	if got != want {
		t.Errorf("substituteSettingsVariables = %q, want %q (unchanged)", got, want)
	}
}

// TestParseLogicalLayerBodyDerivedConditionJinjaNamedTemplateCall covers the "condition jinja
// ${name}(args) { ... };" alternative head (2026-09-19, appliance.sonos_roam's own battery_alert;
// folded into logical_condition_expr.go's own grammar 2026-09-24) -- reuses the SAME Settings.def
// named jinja templates Physical.def's own "derived ... via jinja ${name}(args);" already uses,
// rewriting the template's own "$" placeholder to "$1" (this form's "{ ... }" list is always
// exactly one entry).
func TestParseLogicalLayerBodyDerivedConditionJinjaNamedTemplateCall(t *testing.T) {
	jinjaTemplates := map[string]TJinjaTemplateDefinition{
		"int_less_then": {Params: []string{"i"}, Template: "'on' if (($ | int(0)) < ${i}) else 'off'"},
	}
	devices, warnings := parseLogicalLayerBody([]string{
		"device appliance.sonos_roam with:",
		"  derived binary_sensor.battery_alert with:",
		"    condition jinja ${int_less_then}(10) { sensor.infrastructural_apartment_bedroom_sonos_battery_level };",
		"  end;",
		"end;",
	}, jinjaTemplates)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	capEntry, ok := devices[0].Capabilities["battery_alert"]
	if !ok || !capEntry.IsDerivedCondition {
		t.Fatalf("Capabilities = %+v, want a derived-condition \"battery_alert\" entry", devices[0].Capabilities)
	}
	if capEntry.DerivedCondition == nil || capEntry.DerivedCondition.Kind != CondJinja {
		t.Fatalf("DerivedCondition = %+v, want a single CondJinja node", capEntry.DerivedCondition)
	}
	if capEntry.DerivedCondition.JinjaExpr != "'on' if (($1 | int(0)) < 10) else 'off'" {
		t.Errorf("JinjaExpr = %q, want the resolved named jinja template", capEntry.DerivedCondition.JinjaExpr)
	}
	wantOver := []string{"sensor.infrastructural_apartment_bedroom_sonos_battery_level"}
	gotOver := capEntry.DerivedCondition.JinjaSources
	if len(gotOver) != 1 || gotOver[0] != wantOver[0] {
		t.Errorf("JinjaSources = %v, want %v", gotOver, wantOver)
	}
}

// TestParseLogicalLayerBodyDerivedConditionJinjaNamedTemplateCallWarnsOnUndeclaredTemplate mirrors
// the Physical.def sibling's own behaviour: an undeclared jinja template name is reported, and the
// capability is left unregistered rather than silently guessed at.
func TestParseLogicalLayerBodyDerivedConditionJinjaNamedTemplateCallWarnsOnUndeclaredTemplate(t *testing.T) {
	_, warnings := parseLogicalLayerBody([]string{
		"device appliance.sonos_roam with:",
		"  derived binary_sensor.battery_alert with:",
		"    condition jinja ${no_such_template}(10) { sensor.battery_level };",
		"  end;",
		"end;",
	}, map[string]TJinjaTemplateDefinition{})
	if len(warnings) != 1 || !strings.Contains(warnings[0], "no_such_template") {
		t.Fatalf("warnings = %v, want exactly one mentioning \"no_such_template\"", warnings)
	}
}

// TestRegisterLogicalDerivedConditionCapabilityResolvesLiteralEntityInOver is a regression test
// for appliance.sonos_roam's own battery_alert (2026-09-19): an "over:" entry that doesn't resolve
// as a sibling capability label in any of the three lookups, but looks like an already-qualified
// entity_id (contains a "."), is used verbatim -- covering a bare, independently-existing entity
// like a media_player's own battery_level attribute, which is not a capability of this device.
func TestRegisterLogicalDerivedConditionCapabilityResolvesLiteralEntityInOver(t *testing.T) {
	admin := newAdministrationState()
	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "binary_sensor.infrastructural:battery_alert", DeviceID: "appliance.sonos_roam", Capability: "binary_sensor.battery_alert"}
	capability := TLogicalCapability{
		Domain:             "binary_sensor",
		IsDerivedCondition: true,
		DerivedCondition:   testJinjaCondition("'on' if (($1 | int(0)) < 10) else 'off'", "sensor.infrastructural_apartment_bedroom_sonos_battery_level"),
	}

	warnings, deferred := registerLogicalDerivedConditionCapability(admin, decl, capability, "battery_alert", "sonos", "Logical.def", 1, true, nil, nil, nil, nil, nil, nil)
	if deferred {
		t.Fatalf("did not expect deferral -- a literal entity_id never needs to wait on anything")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	rec, found := findEntityRecordByName(admin, "binary_sensor.infrastructural/sonos/battery_alert")
	if !found {
		t.Fatalf("no entity record registered for binary_sensor.infrastructural/sonos/battery_alert")
	}
	if !strings.Contains(rec.ConditionExpr, "sensor.infrastructural_apartment_bedroom_sonos_battery_level") {
		t.Errorf("ConditionExpr = %q, want it to reference the literal entity_id used verbatim", rec.ConditionExpr)
	}
}

// TestParseLogicalLayerBodyValueAcceptsEntityAttributeLiteral is the regression test for the
// 2026-09-21 fix (the "!" charset addition, now on the new grammar's bare-ref token, 2026-09-24):
// environment.weather's own "pressure" capability, sourced from weather.forecast's own "pressure"
// attribute -- a literal, already-existing main-instance entity!attribute reference, not a sibling
// capability of this device. "pressure" is a "sensor" domain, so it uses "value", not "condition"
// (the exact bug this whole grammar redesign fixes).
func TestParseLogicalLayerBodyValueAcceptsEntityAttributeLiteral(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device environment.weather with:",
		`  derived sensor.pressure with:`,
		`    value weather.forecast!pressure;`,
		"  end;",
		"end;",
	}, nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	capEntry, ok := devices[0].Capabilities["pressure"]
	if !ok {
		t.Fatalf("Capabilities = %+v, want a %q entry", devices[0].Capabilities, "pressure")
	}
	if !capEntry.IsDerivedValue || capEntry.Domain != "sensor" {
		t.Errorf("capEntry = %+v, want IsDerivedValue=true Domain=sensor", capEntry)
	}
	if capEntry.DerivedValue == nil || capEntry.DerivedValue.Kind != CondRef || capEntry.DerivedValue.Ref != "weather.forecast!pressure" {
		t.Errorf("DerivedValue = %+v, want a CondRef leaf for weather.forecast!pressure", capEntry.DerivedValue)
	}
}

// TestRegisterLogicalDerivedConditionCapabilityResolvesLiteralEntityAttributeInOver mirrors
// TestRegisterLogicalDerivedConditionCapabilityResolvesLiteralEntityInOver for the "!attribute"
// form -- confirms the literal-entity_id fallback (registerLogicalDerivedConditionCapability,
// Conceptual_LogicalEntities.go) passes an "entity!attribute" token through verbatim into the
// rendered expression, unmodified except for generator.go's own sourceToJinja2 turning it into
// state_attr(...).
func TestRegisterLogicalDerivedConditionCapabilityResolvesLiteralEntityAttributeInOver(t *testing.T) {
	admin := newAdministrationState()
	decl := TDeviceCapabilityEntityDeclaration{LocalSpec: "sensor.infrastructural:pressure", DeviceID: "environment.weather", Capability: "sensor.pressure"}
	capability := TLogicalCapability{
		Domain:         "sensor",
		IsDerivedValue: true,
		DerivedValue:   testRefCondition("weather.forecast!pressure", false),
	}

	warnings, deferred := registerLogicalDerivedConditionCapability(admin, decl, capability, "pressure", "weather", "Logical.def", 1, true, nil, nil, nil, nil, nil, nil)
	if deferred {
		t.Fatalf("did not expect deferral -- a literal entity_id!attribute never needs to wait on anything")
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	rec, found := findEntityRecordByName(admin, "sensor.infrastructural/weather/pressure")
	if !found {
		t.Fatalf("no entity record registered for sensor.infrastructural/weather/pressure")
	}
	if rec.ConditionExpr != "state_attr('weather.forecast', 'pressure')" {
		t.Errorf("ConditionExpr = %q, want state_attr('weather.forecast', 'pressure')", rec.ConditionExpr)
	}
}

func TestCollectLogicalDevicesByIDDuplicateWarns(t *testing.T) {
	devices, warnings := parseLogicalLayerBody([]string{
		"device node.a with:",
		"  dependency on host.b;",
		"end;",
		"device node.a with:",
		"  dependency on host.c;",
		"end;",
	}, nil)
	if len(devices) != 2 {
		t.Fatalf("parseLogicalLayerBody itself doesn't dedupe -- got %d devices, want 2 (collectLogicalDevicesByID does the deduping)", len(devices))
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings from the parser itself: %v", warnings)
	}
}
