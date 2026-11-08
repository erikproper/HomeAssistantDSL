package main

import (
	"strings"
	"testing"
)

var governedTestBody = []string{
	"device lights.test_fs20_switch with:",
	`  manufacturer: "ELV";`,
	"  capability binary_sensor.node;",
	"  capability switch.switch",
	"    by doing button.set from radio.test_fs20_switch",
	"    enforced every 00:05:00",
	"    default off  # when the broker lost it",
	"    as unique_id switch.social_house_test_fs20_switch;",
	"end;",
}

var governedTestRadios = []string{
	"installation h2 with: cul \"/dev/x\"; end;",
	"installation h3 with: fhz \"/dev/y\"; end;",
	`device radio.test_fs20_switch with: fs20 switch "44444444" "4411" heard by h2, h3; capability button.set; end;`,
	`device radio.test_fs20_dimmer with: fs20 dimmer "44444444" "4412" heard by h2; capability button.set; end;`,
}

// TestParseGovernedIntegrationBody: metadata, the node, and a governed capability with its
// clauses and frozen id.
func TestParseGovernedIntegrationBody(t *testing.T) {
	devices, warnings := parseGovernedIntegrationBody(governedTestBody)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	d := devices[0]
	c := d.Capabilities[1]
	if d.Metadata["manufacturer"] != "ELV" || d.Capabilities[0].Button != "" || c.Button != "button.set" || c.From != "radio.test_fs20_switch" ||
		c.EnforceEvery != "00:05:00" || c.Default != "off" || c.AssignedID != "switch.social_house_test_fs20_switch" {
		t.Errorf("device = %+v", d)
	}
	_, radios, _ := parseFHEMIntegrationBody(governedTestRadios)
	if problems := checkGovernedIntegration(devices, radios); len(problems) != 0 {
		t.Errorf("check = %v", problems)
	}
}

// TestCheckGovernedIntegration: an unknown radio, a button it lacks, an FS20 kind that cannot
// carry the domain, a bad default and duration, a non-node without "by doing".
func TestCheckGovernedIntegration(t *testing.T) {
	devices, warnings := parseGovernedIntegrationBody([]string{
		"device lights.a with:",
		"  capability switch.one by doing button.set from radio.missing enforced every 00:05:00;",
		"  capability switch.two by doing button.other from radio.test_fs20_switch enforced every 00:05:00;",
		"  capability switch.three by doing button.set from radio.test_fs20_dimmer enforced every 00:05:00 default half;",
		"  capability switch.four by doing button.set from radio.test_fs20_switch enforced every 00:99:00;",
		"  capability switch.five;",
		"end;",
	})
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	_, radios, _ := parseFHEMIntegrationBody(governedTestRadios)
	problems := strings.Join(checkGovernedIntegration(devices, radios), "\n")
	for _, want := range []string{"no device of \"integration fhem\"", "has no button.other", "cannot carry a switch", "default \"half\"", "no duration", "only its node"} {
		if !strings.Contains(problems, want) {
			t.Errorf("problems lack %q:\n%s", want, problems)
		}
	}
}

// TestParseGovernedErrors: a wrong clause or metadata field is reported at its word.
func TestParseGovernedErrors(t *testing.T) {
	for want, line := range map[string]string{
		"enforced, default, on":          "capability switch.s by doing button.set from radio.r every 00:05:00;",
		"a metadata field":               `colour: "red";`,
		"the level it is switched on at": "capability switch.s by doing button.set from radio.r on at 0 %;",
	} {
		_, warnings := parseGovernedIntegrationBody([]string{"device lights.a with:", line, "end;"})
		if len(warnings) != 1 || !strings.Contains(warnings[0], want) {
			t.Errorf("%q: warnings = %v, want one about %q", line, warnings, want)
		}
	}
}

// TestGovernedGatewayAndGovernorFile: for Home Assistant a raw device on the governor's topics;
// for the governor its entity with the FHEM device's topics and both installations.
func TestGovernedGatewayAndGovernorFile(t *testing.T) {
	devices, _ := parseGovernedIntegrationBody(governedTestBody)
	_, radios, _ := parseFHEMIntegrationBody(governedTestRadios)
	g := governedGatewayDevices(devices)[0]
	if !g.Raw || g.Retain != retainDeclared || g.RawTopics["node"].Topic != governorStatusTopic || !g.RawTopics["node"].Implied ||
		g.RawTopics["switch"].Command != "governed/lights.test_fs20_switch/switch/set" || !g.RawTopics["switch"].Governed {
		t.Errorf("gateway = %+v", g)
	}
	configs := rawMQTTConfigs(g)
	if c := configs["switch/"+g.Capabilities["switch"].Leaf]; c["state_topic"] != "governed/lights.test_fs20_switch/switch" || c["availability"] == nil {
		t.Errorf("switch config = %v", c)
	}
	file := governorDevicesFile(devices, radios)
	e := file.Entities[0]
	if len(file.Entities) != 1 || e.Kind != "switch" || e.Paths[0].Set != "fhem/fs20/radio.test_fs20_switch/set" || e.Paths[0].Received != "fhem/fs20/radio.test_fs20_switch" ||
		e.Paths[0].Encoding != "fs20_switch" || e.Paths[0].Copies != 2 || e.Default != "off" {
		t.Errorf("governor file = %+v", file)
	}
}

// TestGovernedFrozenIDLine: the stable-id search finds a governed capability's line, and the
// node's implied one.
func TestGovernedFrozenIDLine(t *testing.T) {
	line := "      capability switch.switch by doing button.set from radio.test_fs20_switch enforced every 00:05:00 default off;"
	if !discoveryCapabilityLinePattern("switch", "switch", "", "x", governedRawTopic).MatchString(line) {
		t.Errorf("governed line not found: %q", line)
	}
	if !discoveryCapabilityLinePattern("binary_sensor", "node", "", "x", impliedRawTopic).MatchString("      capability binary_sensor.node;") {
		t.Errorf("node line not found")
	}
}

// TestGovernorDeployPlan: the governor runs on the coordinator's machine, only when there are
// governed devices.
func TestGovernorDeployPlan(t *testing.T) {
	p := &tDeployPlanner{plan: TDeployPlan{Coordinator: "ps1"}}
	p.planGovernorDeploy("integration governed with:\nend;\n")
	if p.plan.Governor != "ps1" {
		t.Errorf("governor = %q", p.plan.Governor)
	}
	p = &tDeployPlanner{plan: TDeployPlan{Coordinator: "ps1"}}
	p.planGovernorDeploy("integration fhem with:\nend;\n")
	if p.plan.Governor != "" {
		t.Errorf("governor without governed devices = %q", p.plan.Governor)
	}
}

// TestGovernedLight: a light on an FS20 dimmer, with its on-level -- Home Assistant's JSON light
// for the YAML, fs20_dimmer for the governor; a level as default, an on-level only for a light.
func TestGovernedLight(t *testing.T) {
	devices, warnings := parseGovernedIntegrationBody([]string{
		"device lights.test_fs20_dimmer with:",
		"  capability binary_sensor.node;",
		"  capability light.light by doing button.set from radio.test_fs20_dimmer enforced every 00:05:00 default off on at 60 %;",
		"end;",
		"device lights.wrong with:",
		"  capability light.light by doing button.set from radio.test_fs20_dimmer enforced every 00:05:00 default 101;",
		"  capability switch.switch by doing button.set from radio.test_fs20_switch enforced every 00:05:00 on at 50 %;",
		"end;",
	})
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	_, radios, _ := parseFHEMIntegrationBody(governedTestRadios)
	problems := checkGovernedIntegration(devices, radios)
	if len(problems) != 2 || !strings.Contains(problems[0], `default "101"`) || !strings.Contains(problems[1], "only a light") {
		t.Errorf("problems = %v", problems)
	}
	file := governorDevicesFile(devices[:1], radios)
	if e := file.Entities[0]; e.Kind != "light" || e.OnLevel != 60 || e.Paths[0].Encoding != "fs20_dimmer" || e.Paths[0].Copies != 1 {
		t.Errorf("governor file = %+v", file)
	}
	g := governedGatewayDevices(devices[:1])[0]
	c := rawMQTTConfigs(g)["light/"+g.Capabilities["light"].Leaf]
	if c["schema"] != "json" || c["brightness_scale"] != 100 || c["command_topic"] != "governed/lights.test_fs20_dimmer/light/set" {
		t.Errorf("light config = %v", c)
	}
}
