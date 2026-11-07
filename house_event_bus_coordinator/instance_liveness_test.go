package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// livenessPublished returns the last payload published per topic.
func livenessPublished(client *fakeClient) map[string]string {
	got := map[string]string{}
	for _, p := range client.publishedSnapshot() {
		got[p.topic] = string(p.payload)
	}
	return got
}

// livenessCount counts the publishes on topic.
func livenessCount(client *fakeClient, topic string) int {
	n := 0
	for _, p := range client.publishedSnapshot() {
		if p.topic == topic {
			n++
		}
	}
	return n
}

// TestInstanceLiveness: a ping at start; no pong -> offline; the coordinator's own ping, its
// inquiry and main's commands are no sign of life; any message from the instance (here the pong)
// -> online.
func TestInstanceLiveness(t *testing.T) {
	client := &fakeClient{}
	spec := instanceLivenessSpec("ha2mqtt")
	spec.Quiet, spec.Within = time.Hour, 20*time.Millisecond
	l := newLiveness(client, spec)
	if err := l.start(); err != nil {
		t.Fatal(err)
	}
	if livenessPublished(client)[instancePingTopic("ha2mqtt")] != "ping" {
		t.Fatalf("no ping at start: %v", livenessPublished(client))
	}
	time.Sleep(60 * time.Millisecond)
	if livenessPublished(client)[instanceStatusTopic("ha2mqtt")] != instanceOffline {
		t.Fatalf("no pong: want offline, got %v", livenessPublished(client))
	}
	for _, topic := range []string{"homeassistant.instances/ha2mqtt/inquire", "homeassistant.instances/ha2mqtt/bridge/switch.x/command", instancePingTopic("ha2mqtt")} {
		payload := "ping"
		if topic != instancePingTopic("ha2mqtt") {
			payload = "x"
		}
		l.observe(topic, payload, false)
		if livenessPublished(client)[instanceStatusTopic("ha2mqtt")] != instanceOffline {
			t.Errorf("%s counted as a sign of life", topic)
		}
	}
	l.observe(instancePingTopic("ha2mqtt"), "pong", false)
	if livenessPublished(client)[instanceStatusTopic("ha2mqtt")] != instanceOnline {
		t.Errorf("pong: want online, got %v", livenessPublished(client))
	}
}

// TestGatewayLiveness: a Zigbee2MQTT-shaped spec -- a healthy JSON answer counts, an unhealthy one
// and a command from main don't; one miss asks again, the second makes it offline; after the Last
// Will's "offline", the next sign of life republishes online.
func TestGatewayLiveness(t *testing.T) {
	client := &fakeClient{}
	verdict := "zigbee2mqtt/bridge/state"
	online, offline := `{"state":"online"}`, `{"state":"offline"}`
	l := newLiveness(client, TLivenessSpec{
		Name: "zigbee2mqtt", Traffic: "zigbee2mqtt/#", Ignore: []string{"zigbee2mqtt/#/set", "zigbee2mqtt/#/retain"},
		Quiet: time.Hour, Ask: "zigbee2mqtt/bridge/request/health_check", AskPayload: "",
		Answer: "zigbee2mqtt/bridge/response/health_check", AnswerField: "data.healthy", Within: 20 * time.Millisecond,
		Verdict: verdict, Online: online, Offline: offline, Misses: 2,
	})
	if err := l.start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	if livenessCount(client, verdict) != 0 || livenessCount(client, "zigbee2mqtt/bridge/request/health_check") != 2 {
		t.Fatalf("after one miss: want a second question and no verdict, got %v", client.publishedSnapshot())
	}
	time.Sleep(30 * time.Millisecond)
	if livenessPublished(client)[verdict] != offline {
		t.Fatalf("two misses: want offline, got %v", livenessPublished(client))
	}
	l.observe("zigbee2mqtt/house/kitchen/light/set", `{"state":"ON"}`, false)
	l.observe("zigbee2mqtt/house/kitchen/light/retain", `{"state":"ON"}`, false) // the coordinator's own shadow
	l.observe("zigbee2mqtt/bridge/response/health_check", `{"data":{"healthy":false},"status":"ok"}`, false)
	l.observe("zigbee2mqtt/house/kitchen/light", `{"state":"ON"}`, true)
	if livenessPublished(client)[verdict] != offline {
		t.Errorf("a command, an unhealthy answer or a retained message counted as a sign of life")
	}
	l.observe("zigbee2mqtt/bridge/response/health_check", `{"data":{"healthy":true},"status":"ok"}`, false)
	if livenessPublished(client)[verdict] != online {
		t.Fatalf("healthy answer: want online, got %v", livenessPublished(client))
	}
	l.observe(verdict, offline, false) // the Last Will, which we then keep retained
	l.observe("zigbee2mqtt/house/kitchen/light", `{"state":"OFF"}`, false)
	if livenessPublished(client)[verdict] != online || livenessCount(client, verdict) != 4 {
		t.Errorf("after the Last Will, a sign of life should republish online: %v", client.publishedSnapshot())
	}
}

// TestTopicMatches covers + and #.
func TestTopicMatches(t *testing.T) {
	for _, c := range []struct {
		filter, topic string
		want          bool
	}{
		{"zigbee2mqtt/#", "zigbee2mqtt/bridge/state", true},
		{"zigbee2mqtt/+/set", "zigbee2mqtt/lamp/set", true},
		{"zigbee2mqtt/+/set", "zigbee2mqtt/a/b/set", false},
		{"a/b", "a/b/c", false},
		{"a/+/c", "a/b/c", true},
	} {
		if got := topicMatches(c.filter, c.topic); got != c.want {
			t.Errorf("topicMatches(%q, %q) = %v", c.filter, c.topic, got)
		}
	}
	for _, c := range []struct {
		filter, topic string
		want          bool
	}{
		{"zigbee2mqtt/#/set", "zigbee2mqtt/apartment/living_room/cycling_fan/set", true},
		{"zigbee2mqtt/#/set", "zigbee2mqtt/lamp/set", true},
		{"zigbee2mqtt/#/set", "zigbee2mqtt/apartment/living_room/cycling_fan", false},
		{"zigbee2mqtt/#/retain", "zigbee2mqtt/a/b/retain", true},
		{"zigbee2mqtt/bridge/request/#", "zigbee2mqtt/bridge/request/health_check", true},
		{"zigbee2mqtt/#", "zigbee2mqtt", true},
	} {
		if got := ignoreMatches(c.filter, c.topic); got != c.want {
			t.Errorf("ignoreMatches(%q, %q) = %v", c.filter, c.topic, got)
		}
	}
}

// TestLivenessFile: the generator's coordinator/liveness.yaml becomes specs; a bad duration is
// skipped.
func TestLivenessFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "liveness.yaml")
	content := `### This file was automatically generated. Don't edit it!
gateways:
    - device: integration.zigbee2mqtt
      traffic: zigbee2mqtt/#
      ignore:
        - zigbee2mqtt/+/set
      quiet: "00:05:00"
      ask: zigbee2mqtt/bridge/request/health_check
      ask_payload: ""
      answer: zigbee2mqtt/bridge/response/health_check
      answer_field: data.healthy
      within: "00:00:30"
      verdict: zigbee2mqtt/bridge/state
      online: '{"state":"online"}'
      offline: '{"state":"offline"}'
      misses: 2
    - device: integration.broken
      traffic: x/#
      quiet: "5 minutes"
      ask: x/ping
      answer: x/ping
      within: "00:00:30"
      verdict: x/status
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := loadLivenessFile(path)
	if err != nil {
		t.Fatal(err)
	}
	specs := livenessSpecs(file)
	if len(specs) != 1 || specs[0].Quiet != 5*time.Minute || specs[0].Within != 30*time.Second || specs[0].AnswerField != "data.healthy" ||
		specs[0].Online != `{"state":"online"}` || len(specs[0].Ignore) != 1 || specs[0].Misses != 2 {
		t.Errorf("specs %+v", specs)
	}
	if missing, err := loadLivenessFile(filepath.Join(t.TempDir(), "none.yaml")); err != nil || len(missing.Gateways) != 0 {
		t.Errorf("a missing file must mean no gateways: %v %v", missing, err)
	}
}
