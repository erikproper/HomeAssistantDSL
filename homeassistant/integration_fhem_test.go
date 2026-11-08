package main

import (
	"os/exec"
	"strings"
	"testing"
)

var fhemTestBody = []string{
	"installation protocols-server-2 with:",
	`  cul "/dev/serial/by-id/usb-busware.de_CUL868-if00";`,
	"end;",
	"installation protocols-server-3 with:",
	`  fhz "/dev/serial/by-id/usb-ELV_AG_ELV_FHZ_1000_PC_ELTLYOUK-if00-port0";  # the old one`,
	"end;",
	"device radio.hallway_lamp with:",
	`  fs20 switch "1234" "02" heard by protocols-server-2;`,
	"  capability button.set;",
	"end;",
	"device radio.living_room_lamp with:",
	`  fs20 dimmer "12341234" "1111" heard by protocols-server-2, protocols-server-3;`,
	"  capability button.set;",
	"end;",
	"device sensors.garage_motion with:",
	`  fs20 sensor "1234" "20" heard by protocols-server-3;`,
	"  capability binary_sensor.motion;",
	"end;",
}

// TestParseFHEMIntegrationBody: installations with their serial devices, and FS20 devices with
// their kind, codes and installations.
func TestParseFHEMIntegrationBody(t *testing.T) {
	installations, devices, warnings := parseFHEMIntegrationBody(fhemTestBody)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if len(installations) != 2 || installations[1].Host != "protocols-server-3" || installations[1].Serials[0].Kind != "fhz" ||
		installations[0].Serials[0].Path != "/dev/serial/by-id/usb-busware.de_CUL868-if00" {
		t.Errorf("installations = %+v", installations)
	}
	if len(devices) != 3 || devices[1].DeviceID != "radio.living_room_lamp" || devices[1].FS20.Kind != "dimmer" ||
		devices[1].FS20.HouseCode != "12341234" || len(devices[1].FS20.HeardBy) != 2 || devices[2].Capabilities[0] != "binary_sensor.motion" {
		t.Errorf("devices = %+v", devices)
	}
	if problems := checkFHEMIntegration(installations, devices); len(problems) != 0 {
		t.Errorf("check = %v", problems)
	}
}

// TestParseFHEMIntegrationErrors: an error is reported at its word, and parsing goes on with the
// next statement.
func TestParseFHEMIntegrationErrors(t *testing.T) {
	cases := map[string][]string{
		"usb":                    {`installation h with: usb "/dev/x"; end;`},
		"12G4":                   {`device radio.x with: fs20 switch "12G4" "01" heard by h; end;`},
		"blinds":                 {`device radio.x with: fs20 blinds "1234" "01" heard by h; end;`},
		"fs20":                   {`device radio.x with: fs20 switch "1234" "01" heard by h; fs20 switch "1234" "02" heard by h; end;`},
		"installation or device": {"capability button.set;"},
	}
	for want, body := range cases {
		_, _, warnings := parseFHEMIntegrationBody(append(body, "installation h with: end;"))
		if len(warnings) != 1 || !strings.Contains(warnings[0], want) {
			t.Errorf("%v: warnings = %v, want one about %q", body, warnings, want)
		}
	}
}

// TestCheckFHEMIntegration: a device heard by an unknown installation or one without an FS20
// radio, and a device with the wrong capability for its kind.
func TestCheckFHEMIntegration(t *testing.T) {
	installations, devices, _ := parseFHEMIntegrationBody([]string{
		"installation h with: end;",
		`device radio.a with: fs20 switch "1234" "01" heard by h, other; capability event.button; end;`,
	})
	problems := checkFHEMIntegration(installations, devices)
	if len(problems) != 3 || !strings.Contains(problems[0], "no FHEM radio") && !strings.Contains(problems[0], "no FS20 radio") ||
		!strings.Contains(problems[1], "no installation") || !strings.Contains(problems[2], "button.set") {
		t.Errorf("problems = %v", problems)
	}
}

// TestFHEMConfig: an installation's fhem.cfg has its radio, the broker without its password, and
// only the devices it hears.
func TestFHEMConfig(t *testing.T) {
	installations, devices, _ := parseFHEMIntegrationBody(fhemTestBody)
	broker := TMQTTBrokerSecrets{Server: "ps1.example", Port: "1883", Login: "house", Password: "s3cret"}
	config := fhemConfig(installations[0], devices, broker)
	for _, want := range []string{
		"define cul_1 CUL /dev/serial/by-id/usb-busware.de_CUL868-if00@9600 0000\n",
		"define mqtt MQTT2_CLIENT ps1.example:1883\n",
		"attr mqtt username house\n",
		"attr mqtt subscriptions fhem/fs20/radio.hallway_lamp/set fhem/fs20/radio.living_room_lamp/set\n",
		"attr mqtt msgAfterConnect -r fhem/installation/protocols-server-2 online\n",
		"define radio.hallway_lamp FS20 1234 02\nattr radio.hallway_lamp IODev cul_1\n",
		`attr radio.hallway_lamp.command readingList fhem/fs20/radio\.hallway_lamp/set:(on|off) { fhem("set radio.hallway_lamp $EVENT");; {} }`,
		"define radio.hallway_lamp.publish notify radio\\.hallway_lamp:.* set mqtt publish fhem/fs20/radio.hallway_lamp $EVENT\n",
		"define radio.living_room_lamp FS20 12341234 1111\n",
	} {
		if !strings.Contains(config, want) {
			t.Errorf("fhem.cfg lacks %q:\n%s", want, config)
		}
	}
	if strings.Contains(config, "s3cret") || strings.Contains(config, "garage_motion") {
		t.Errorf("fhem.cfg has the password or a device it doesn't hear:\n%s", config)
	}
	sensor := fhemConfig(installations[1], devices, broker)
	if !strings.Contains(sensor, "set mqtt publish -r fhem/fs20/sensors.garage_motion $EVENT") || !strings.Contains(sensor, "define fhz_1 FHZ ") {
		t.Errorf("protocols-server-3's fhem.cfg:\n%s", sensor)
	}
}

// TestFS20DimExpression runs the dimmer's Perl expression, as FHEM would, on values Home
// Assistant may send.
func TestFS20DimExpression(t *testing.T) {
	if _, err := exec.LookPath("perl"); err != nil {
		t.Skip("no perl")
	}
	expression := strings.ReplaceAll(fs20DimExpression(), ";;", ";")
	for event, want := range map[string]string{
		"on": "on", "off": "off", "0": "off", "2": "off", "3": "dim06%", "40": "dim37%", "47": "dim50%",
		"50": "dim50%", "97": "dim100%", "100": "dim100%", "junk": "off",
	} {
		out, err := exec.Command("perl", "-e", `my $EVENT = $ARGV[0]; print `+expression, event).CombinedOutput()
		if err != nil || string(out) != want {
			t.Errorf("%q: got %q (%v), want %q", event, out, err, want)
		}
	}
}

// TestFHEMDeployPlan: each installation runs on its hosts device's machine.
func TestFHEMDeployPlan(t *testing.T) {
	physical := "integration fhem with:\n  installation ps2 with:\n    cul \"/dev/x\";\n  end;\nend;\n"
	p := &tDeployPlanner{plan: TDeployPlan{Machines: map[string]TSSHEndpoint{"ps2": {User: "erikp", Host: "h.example", Port: 1971}}}}
	p.planFHEMDeploy(physical)
	if len(p.plan.FHEM) != 1 || p.plan.FHEM[0].Machine != "ps2" || p.plan.FHEM[0].Files != "fhem/ps2" || len(p.warnings) != 0 {
		t.Errorf("plan = %+v, warnings = %v", p.plan.FHEM, p.warnings)
	}
}

// TestFHEMTransmitter: a remote's events are published as FHEM reports them -- no eventMap, so a
// held button's dimup/dimdown stays apart from on/off -- and not retained.
func TestFHEMTransmitter(t *testing.T) {
	installations, devices, warnings := parseFHEMIntegrationBody([]string{
		`installation h with: cul "/dev/x"; end;`,
		`device buttons.remote with: fs20 transmitter "1234" "10" heard by h; capability event.button; end;`,
	})
	if len(warnings) != 0 || len(checkFHEMIntegration(installations, devices)) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	config := fhemConfig(installations[0], devices, TMQTTBrokerSecrets{Server: "s", Port: "1883"})
	if strings.Contains(config, "eventMap") || strings.Contains(config, "buttons.remote.command") ||
		!strings.Contains(config, "define buttons.remote.publish notify buttons\\.remote:.* set mqtt publish fhem/fs20/buttons.remote $EVENT\n") {
		t.Errorf("fhem.cfg:\n%s", config)
	}
}
