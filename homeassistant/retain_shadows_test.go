package main

import (
	"reflect"
	"testing"
)

// TestMQTTFilterMatches: MQTT's own rules -- "+" one level, a final "#" the rest, including none.
func TestMQTTFilterMatches(t *testing.T) {
	for _, c := range []struct {
		filter, topic string
		want          bool
	}{
		{"zwave/#", "zwave/garage/fuse_cabinet/switch_33/37/1/currentValue", true},
		{"zwave/#", "zwave", true},
		{"zwave/#", "zwavejs/x", false},
		{"ems-esp/#", "ems-esp/boiler_data", true},
		{"zigbee2mqtt/+/radiator/robb", "zigbee2mqtt/shower_room/radiator/robb", true},
		{"zigbee2mqtt/+/radiator/robb", "zigbee2mqtt/a/b/radiator/robb", false},
		{"a/b", "a/b", true},
		{"a/b", "a/b/c", false},
		{"a/b/c", "a/b", false},
		{"+", "a", true},
		{"+", "a/b", false},
	} {
		if got := mqttFilterMatches(c.filter, c.topic); got != c.want {
			t.Errorf("mqttFilterMatches(%q, %q) = %v, want %v", c.filter, c.topic, got, c.want)
		}
	}
}

// TestMQTTFilterProblem: "#" only as the whole last level, "+" only as a whole level.
func TestMQTTFilterProblem(t *testing.T) {
	for filter, bad := range map[string]bool{
		"zwave/#": false, "#": false, "+/x/+": false, "a/b": false,
		"": true, "zwave#": true, "a/#/b": true, "a+/b": true,
	} {
		if got := mqttFilterProblem(filter) != ""; got != bad {
			t.Errorf("mqttFilterProblem(%q) reports a problem: %v, want %v", filter, got, bad)
		}
	}
}

// TestTopicIsRetained: the device's own line always wins; else the filters; else not retained.
func TestTopicIsRetained(t *testing.T) {
	filters := []string{"zwave/#", "ems-esp/#"}
	for _, c := range []struct {
		topic  string
		device TRetainDeclaration
		want   bool
	}{
		{"zwave/x/currentValue", retainUndeclared, true},
		{"zwave/x/currentValue", retainNotDeclared, false}, // backing out of the wildcard
		{"zigbee2mqtt/plug", retainUndeclared, false},      // the default
		{"zigbee2mqtt/plug", retainDeclared, true},
		{"zwave/x/currentValue", retainDeclared, true},
	} {
		if got := topicIsRetained(c.topic, c.device, filters); got != c.want {
			t.Errorf("topicIsRetained(%q, %v) = %v, want %v", c.topic, c.device, got, c.want)
		}
	}
	if topicIsRetained("zwave/x", retainUndeclared, nil) {
		t.Errorf("no filters, no line: want not retained")
	}
	if got := shadowTopic("zigbee2mqtt/plug"); got != "zigbee2mqtt/plug/retain" {
		t.Errorf("shadowTopic = %q", got)
	}
}

// TestParseDiscoveryRetainedTopics: only the block's own lines, malformed filters reported and
// left out, and the body parser accepts the line silently.
func TestParseDiscoveryRetainedTopics(t *testing.T) {
	body := []string{
		`retained topics "zwave/#", "ems-esp/#";`,
		`device discovery.ems_esp with:`,
		`  retained topics "inside/#";`,
		`end;`,
		`retained topics "a/#/b", zwave;`,
	}
	filters, warnings := parseDiscoveryRetainedTopics(trimmedLines(body))
	if want := []string{"zwave/#", "ems-esp/#"}; !reflect.DeepEqual(filters, want) {
		t.Errorf("filters = %v, want %v", filters, want)
	}
	if len(warnings) != 2 {
		t.Errorf("warnings = %v, want 2 (bad filter, unquoted item)", warnings)
	}
	_, bodyWarnings := parseDiscoveryIntegrationBody(trimmedLines(body))
	if len(bodyWarnings) != 1 {
		t.Errorf("body warnings = %v, want only the one for the line inside the device", bodyWarnings)
	}
}

// TestParseDiscoveryDeviceRetain: "is retained;" / "is not retained;" set the device's own
// declaration; both on one device is reported.
func TestParseDiscoveryDeviceRetain(t *testing.T) {
	devices, warnings := parseDiscoveryIntegrationBody(trimmedLines([]string{
		`device switches.a with:`,
		`  is not retained;`,
		`end;`,
		`device switches.b with:`,
		`  is retained;`,
		`end;`,
		`device switches.c with:`,
		`end;`,
		`device switches.d with:`,
		`  is retained;`,
		`  is not retained;`,
		`end;`,
	}))
	got := map[string]TRetainDeclaration{}
	for _, d := range devices {
		got[d.DeviceID] = d.Retain
	}
	want := map[string]TRetainDeclaration{"switches.a": retainNotDeclared, "switches.b": retainDeclared, "switches.c": retainUndeclared, "switches.d": retainNotDeclared}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Retain = %v, want %v", got, want)
	}
	if len(warnings) != 1 {
		t.Errorf("warnings = %v, want 1 (switches.d says both)", warnings)
	}
}

func trimmedLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		for len(line) > 0 && line[0] == ' ' {
			line = line[1:]
		}
		out[i] = line
	}
	return out
}

// TestStripLineComment: "#" starts a comment only outside a quoted string.
func TestStripLineComment(t *testing.T) {
	for line, want := range map[string]string{
		`  retained topics "zwave/#", "ems-esp/#";  # Z-Wave JS UI and EMS-ESP retain`: `retained topics "zwave/#", "ems-esp/#";`,
		`model: "Roomba" forced;   # discovery: "i755640"`:                             `model: "Roomba" forced;`,
		`# a whole-line comment with "quotes"`:                                         ``,
		`name "a \"#\" inside";`:                                                       `name "a \"#\" inside";`,
		`plain;`:                                                                       `plain;`,
	} {
		if got := stripLineComment(line); got != want {
			t.Errorf("stripLineComment(%q) = %q, want %q", line, got, want)
		}
	}
}

// TestApplyRetainShadows: state topics (also per-feature and attributes) go to the shadow,
// command topics (incl. set_position_topic) never; the coordinator's availability is added.
func TestApplyRetainShadows(t *testing.T) {
	body := map[string]interface{}{
		"state_topic":           "z2m/cover",
		"position_topic":        "z2m/cover",
		"json_attributes_topic": "z2m/cover",
		"command_topic":         "z2m/cover/set",
		"set_position_topic":    "z2m/cover/set",
		"availability":          []interface{}{map[string]interface{}{"topic": "z2m/bridge/state"}},
		"availability_mode":     "all",
	}
	seen, shadowed := applyRetainShadows(body, "cover", retainUndeclared, nil)
	if len(seen) != 3 || len(shadowed) != 3 {
		t.Errorf("seen %v, shadowed %v: want the three state topics", seen, shadowed)
	}
	for key, want := range map[string]string{"state_topic": "z2m/cover/retain", "position_topic": "z2m/cover/retain", "json_attributes_topic": "z2m/cover/retain", "command_topic": "z2m/cover/set", "set_position_topic": "z2m/cover/set"} {
		if body[key] != want {
			t.Errorf("%s = %v, want %q", key, body[key], want)
		}
	}
	availability := body["availability"].([]interface{})
	if len(availability) != 2 || availability[1].(map[string]interface{})["topic"] != coordinatorAvailabilityTopic {
		t.Errorf("availability = %v, want the bridge's and the coordinator's", availability)
	}
}

// TestApplyRetainShadowsLeavesRetainedAndEvents: a retained topic and an event entity keep their
// original topic and get no coordinator availability.
func TestApplyRetainShadowsLeavesRetainedAndEvents(t *testing.T) {
	retained := map[string]interface{}{"state_topic": "zwave/x/currentValue"}
	if _, shadowed := applyRetainShadows(retained, "switch", retainUndeclared, []string{"zwave/#"}); len(shadowed) != 0 || retained["state_topic"] != "zwave/x/currentValue" || retained["availability"] != nil {
		t.Errorf("retained topic changed: %v", retained)
	}
	event := map[string]interface{}{"state_topic": "z2m/tap"}
	if _, shadowed := applyRetainShadows(event, "event", retainUndeclared, nil); len(shadowed) != 0 || event["state_topic"] != "z2m/tap" {
		t.Errorf("event entity changed: %v", event)
	}
	backedOut := map[string]interface{}{"state_topic": "zwave/y/currentValue"}
	if _, shadowed := applyRetainShadows(backedOut, "switch", retainNotDeclared, []string{"zwave/#"}); len(shadowed) != 1 || backedOut["state_topic"] != "zwave/y/currentValue/retain" {
		t.Errorf("\"is not retained;\" didn't back out of the filter: %v", backedOut)
	}
}

// TestAddCoordinatorAvailabilitySingleTopic: a single availability_topic becomes the list form.
func TestAddCoordinatorAvailabilitySingleTopic(t *testing.T) {
	body := map[string]interface{}{"availability_topic": "ems-esp/status", "payload_available": "online", "payload_not_available": "offline"}
	addCoordinatorAvailability(body)
	list := body["availability"].([]interface{})
	if len(list) != 2 || list[0].(map[string]interface{})["topic"] != "ems-esp/status" || body["availability_topic"] != nil || body["payload_available"] != nil {
		t.Errorf("body = %v", body)
	}
}

// TestRetainReports: a line without effect, and a filter matching nothing.
func TestRetainReports(t *testing.T) {
	filters := []string{"zwave/#"}
	if retainDeclarationEffect([]string{"zwave/x"}, retainDeclared, filters) {
		t.Errorf("\"is retained;\" under a matching filter should have no effect")
	}
	if !retainDeclarationEffect([]string{"zwave/x"}, retainNotDeclared, filters) {
		t.Errorf("\"is not retained;\" under a matching filter should have an effect")
	}
	if !retainDeclarationEffect([]string{"z2m/plug"}, retainDeclared, filters) {
		t.Errorf("\"is retained;\" without a matching filter should have an effect")
	}
	if got := unmatchedRetainFilters([]string{"zwave/#", "zwav/#"}, []string{"zwave/x", "z2m/plug"}); !reflect.DeepEqual(got, []string{"zwav/#"}) {
		t.Errorf("unmatched = %v", got)
	}
}
