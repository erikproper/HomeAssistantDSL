package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestLivenessBlock: a gateway's "liveness with:" block is parsed (both answer forms, escaped
// quotes, ignore filters, default misses), an incomplete one is reported and dropped, and the
// generator writes the parsed ones to coordinator/liveness.yaml.
func TestLivenessBlock(t *testing.T) {
	devices, warnings := parseDiscoveryIntegrationBody([]string{
		"device integration.zigbee2mqtt with:",
		"liveness with:",
		`traffic "zigbee2mqtt/#" quiet for 00:05:00;`,
		`ignore "zigbee2mqtt/+/set", "zigbee2mqtt/+/+/+/set";`,
		`ask "zigbee2mqtt/bridge/request/health_check" with "";`,
		`answer on "zigbee2mqtt/bridge/response/health_check" has data.healthy within 00:00:30;`,
		`verdict on "zigbee2mqtt/bridge/state" online "{\"state\":\"online\"}" offline "{\"state\":\"offline\"}";`,
		"end;",
		"end;",
		"device integration.other with:",
		"liveness with:",
		`traffic "other/#" quiet for 00:05:00;`,
		`ask "other/ping" with "ping";`,
		`answer on "other/ping" is "pong" within 00:00:10;`,
		`verdict on "other/status" online "online" offline "offline";`,
		"misses 1;",
		"end;",
		"end;",
		"device integration.incomplete with:",
		"liveness with:",
		`traffic "x/#" quiet for 00:05:00;`,
		"end;",
		"end;",
	})
	gateways := map[string]TDiscoveryGatewayDevice{}
	for _, d := range devices {
		gateways[d.DeviceID] = d
	}
	z := gateways["integration.zigbee2mqtt"].Liveness
	if z == nil || z.Traffic != "zigbee2mqtt/#" || z.Quiet != "00:05:00" || len(z.Ignore) != 2 || z.AskPayload != "" ||
		z.AnswerField != "data.healthy" || z.Within != "00:00:30" || z.Online != `{"state":"online"}` || z.Offline != `{"state":"offline"}` || z.Misses != 2 {
		t.Errorf("zigbee2mqtt liveness %+v", z)
	}
	o := gateways["integration.other"].Liveness
	if o == nil || o.AnswerPayload != "pong" || o.AnswerField != "" || o.Misses != 1 {
		t.Errorf("other liveness %+v", o)
	}
	if gateways["integration.incomplete"].Liveness != nil || len(warnings) != 1 || !strings.Contains(warnings[0], "integration.incomplete") {
		t.Errorf("incomplete block: liveness %+v, warnings %v", gateways["integration.incomplete"].Liveness, warnings)
	}

	raw, rawWarnings := parseMQTTIntegrationBody([]string{
		"device integration.zwave2mqtt with:",
		"liveness with:",
		`traffic "zwave/#" quiet for 00:05:00;`,
		`ask "zwave/ping/set" with "{}";`,
		`answer on "zwave/ping" has success within 00:00:30;`,
		`verdict on "zwave/status" online "{\"value\":true}" offline "{\"value\":false}";`,
		"end;",
		`capability binary_sensor.node topic "zwave/driver/status" on "true";`,
		"end;",
	})
	if len(raw) != 1 || raw[0].Liveness == nil || raw[0].Liveness.AnswerField != "success" || len(raw[0].RawTopics) != 1 || len(rawWarnings) != 0 {
		t.Errorf("raw mqtt device: %+v, warnings %v", raw, rawWarnings)
	}

	root := t.TempDir()
	if err := writeLivenessFile(root, gateways); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "coordinator", "liveness.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var file TLivenessFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Gateways) != 2 || file.Gateways[0].Device != "integration.other" || file.Gateways[1].Verdict != "zigbee2mqtt/bridge/state" {
		t.Errorf("liveness.yaml:\n%s", data)
	}
}
