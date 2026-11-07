package main

import (
	"strings"
	"testing"
)

// TestShellyLayer: "shelly" implies the listed capabilities' topics, the manufacturer, the model
// from the captured model_id, where the metadata is read, and the liveness.
func TestShellyLayer(t *testing.T) {
	devices, warnings := parseMQTTIntegrationBody([]string{
		`device switches.test with:`,
		`model_id: "SHPLG2-1";`,
		`shelly "shellyplug-6CC3D8";`,
		`capability binary_sensor.node;`,
		`capability switch.switch as unique_id switch.frozen;`,
		`capability sensor.energy;`,
		`end;`,
	})
	if len(warnings) != 0 || len(devices) != 1 {
		t.Fatalf("devices %v, warnings %v", devices, warnings)
	}
	d := devices[0]
	sw := d.RawTopics["switch"]
	if sw.Topic != "shellies/shellyplug-6CC3D8/relay/0" || sw.Command != "shellies/shellyplug-6CC3D8/relay/0/command" || !sw.Implied || d.Capabilities["switch"].AssignedID != "switch.frozen" {
		t.Errorf("switch = %+v %+v", sw, d.Capabilities["switch"])
	}
	if e := d.RawTopics["energy"]; e.DividedBy != 60 || e.Unit != "Wh" {
		t.Errorf("energy = %+v", e)
	}
	if d.Metadata["manufacturer"].Value != "Shelly" || d.Metadata["model"].Value != "Plug E" {
		t.Errorf("metadata = %v", d.Metadata)
	}
	if len(d.RawMetadata) != 2 || d.RawMetadata[1] != (TRawMetadataTopic{Field: "sw_version", Topic: "shellies/shellyplug-6CC3D8/announce", Key: "fw_ver"}) {
		t.Errorf("raw metadata = %v", d.RawMetadata)
	}
	if l := d.Liveness; l == nil || l.Ask != "shellies/shellyplug-6CC3D8/command" || l.Verdict != "shellies/shellyplug-6CC3D8/online" || l.Quiet != "00:01:30" {
		t.Errorf("liveness = %+v", l)
	}
	if !discoveryCapabilityLinePattern("switch", "switch", "", "", rawTopicForLineSearch(sw)).MatchString(`capability switch.switch;`) {
		t.Error("the frozen-id search doesn't find the implied line")
	}
}

// TestShellyLayerErrors: an unknown family or label, a wrong domain, and an implied capability
// without "shelly".
func TestShellyLayerErrors(t *testing.T) {
	for want, lines := range map[string][]string{
		`"shellyfoo" is no Shelly family`: {`shelly "shellyfoo-1";`, `capability switch.switch;`},
		`has no capability "temperature"`: {`shelly "shellyplug-1";`, `capability sensor.temperature;`},
		`"power" is a sensor`:             {`shelly "shellyplug-1";`, `capability switch.power;`},
		`no "shelly ...;" implies one`:    {`capability switch.switch;`},
		`expected the Shelly's quoted id`: {`shelly shellyplug-1;`},
	} {
		body := append(append([]string{`device switches.x with:`}, lines...), `end;`)
		_, warnings := parseMQTTIntegrationBody(body)
		if len(warnings) == 0 || !strings.Contains(strings.Join(warnings, "\n"), want) {
			t.Errorf("%v: warnings %q, want %q", lines, warnings, want)
		}
	}
}
