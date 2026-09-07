package main

import (
	"encoding/json"
	"testing"
)

func TestPublishDiscoverEntityInputPayloadShape(t *testing.T) {
	client := &fakeClient{}
	if err := publishDiscoverEntityInput(client, "homeassistant"); err != nil {
		t.Fatalf("publishDiscoverEntityInput error: %v", err)
	}
	if len(client.published) != 1 {
		t.Fatalf("got %d publishes, want 1", len(client.published))
	}

	var body map[string]interface{}
	if err := json.Unmarshal(client.published[0].payload, &body); err != nil {
		t.Fatalf("payload is not valid JSON: %v (%s)", err, client.published[0].payload)
	}
	if body["command_topic"] != discoverEntityCommandTopic() {
		t.Errorf("command_topic = %v, want %q", body["command_topic"], discoverEntityCommandTopic())
	}
	if optimistic, ok := body["optimistic"].(bool); !ok || !optimistic {
		t.Errorf("optimistic = %v, want true (no state_topic, nothing reports a value back)", body["optimistic"])
	}
	if _, hasStateTopic := body["state_topic"]; hasStateTopic {
		t.Errorf("body = %+v, want no state_topic at all", body)
	}
}

func TestSubscribeDiscoverEntityRequestsSeedsAndInquiresOnEveryInstance(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	client := &fakeClient{}

	if err := subscribeDiscoverEntityRequests(client, tracker, []string{"main", "protocols-server-2"}); err != nil {
		t.Fatalf("subscribeDiscoverEntityRequests error: %v", err)
	}
	if len(client.subscribedHandlers) != 1 {
		t.Fatalf("got %d subscriptions, want 1", len(client.subscribedHandlers))
	}

	client.subscribedHandlers[0](client, fakeMessage{
		topic:   discoverEntityCommandTopic(),
		payload: []byte("sensor.master_bedroom_carbon_dioxide"),
	})

	for _, instance := range []string{"main", "protocols-server-2"} {
		byDevice := tracker.snapshotByDevice(instance)
		if _, found := byDevice[""]["sensor.master_bedroom_carbon_dioxide"]; !found {
			t.Errorf("expected the requested entity to be seeded for %q, got %+v", instance, byDevice)
		}
		if _, found := findPublish(client.published, existenceInquiryTopic(instance)); !found {
			t.Errorf("expected an immediate inquiry publish for %q, got %+v", instance, client.published)
		}
	}
}

func TestSubscribeDiscoverEntityRequestsIgnoresEmptyPayload(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	client := &fakeClient{}
	if err := subscribeDiscoverEntityRequests(client, tracker, []string{"main"}); err != nil {
		t.Fatalf("subscribeDiscoverEntityRequests error: %v", err)
	}
	client.subscribedHandlers[0](client, fakeMessage{topic: discoverEntityCommandTopic(), payload: []byte("   ")})

	byDevice := tracker.snapshotByDevice("main")
	if len(byDevice) != 0 {
		t.Errorf("expected no entries seeded from a blank payload, got %+v", byDevice)
	}
	if len(client.published) != 0 {
		t.Errorf("expected no inquiry published for a blank payload, got %+v", client.published)
	}
}

func TestParseDiscoverEntityRequest(t *testing.T) {
	cases := []struct {
		payload      string
		wantInstance string
		wantEntity   string
		wantOK       bool
	}{
		{"protocols-server-2: sensor.master_bedroom_carbon_dioxide", "protocols-server-2", "sensor.master_bedroom_carbon_dioxide", true},
		{"main: sensor.processor_use", "main", "sensor.processor_use", true},
		{"sensor.master_bedroom_carbon_dioxide", "", "", false}, // bare entity_id, no instance prefix
		{"sensor.master_bedroom:carbon_dioxide", "", "", false}, // colon with no following space -- not the separator
		{": sensor.foo", "", "", false},                         // empty instance
		{"main: ", "", "", false},                               // empty entity_id
	}
	for _, c := range cases {
		instance, entity, ok := parseDiscoverEntityRequest(c.payload)
		if ok != c.wantOK || instance != c.wantInstance || entity != c.wantEntity {
			t.Errorf("parseDiscoverEntityRequest(%q) = (%q, %q, %v), want (%q, %q, %v)",
				c.payload, instance, entity, ok, c.wantInstance, c.wantEntity, c.wantOK)
		}
	}
}

func TestSubscribeDiscoverEntityRequestsRoutesToNamedInstanceOnly(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	client := &fakeClient{}
	if err := subscribeDiscoverEntityRequests(client, tracker, []string{"main", "protocols-server-2"}); err != nil {
		t.Fatalf("subscribeDiscoverEntityRequests error: %v", err)
	}

	client.subscribedHandlers[0](client, fakeMessage{
		topic:   discoverEntityCommandTopic(),
		payload: []byte("protocols-server-2: sensor.master_bedroom_carbon_dioxide"),
	})

	byDevice := tracker.snapshotByDevice("protocols-server-2")
	if _, found := byDevice[""]["sensor.master_bedroom_carbon_dioxide"]; !found {
		t.Errorf("expected the entity seeded for protocols-server-2, got %+v", byDevice)
	}
	if _, found := findPublish(client.published, existenceInquiryTopic("protocols-server-2")); !found {
		t.Errorf("expected an inquiry publish for protocols-server-2, got %+v", client.published)
	}

	if byDevice := tracker.snapshotByDevice("main"); len(byDevice) != 0 {
		t.Errorf("expected \"main\" to be untouched by an instance-scoped request, got %+v", byDevice)
	}
	if _, found := findPublish(client.published, existenceInquiryTopic("main")); found {
		t.Errorf("expected no inquiry publish for \"main\", got %+v", client.published)
	}
}

func TestSubscribeDiscoverEntityRequestsIgnoresUnknownInstance(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	client := &fakeClient{}
	if err := subscribeDiscoverEntityRequests(client, tracker, []string{"main"}); err != nil {
		t.Fatalf("subscribeDiscoverEntityRequests error: %v", err)
	}

	client.subscribedHandlers[0](client, fakeMessage{
		topic:   discoverEntityCommandTopic(),
		payload: []byte("not-a-real-instance: sensor.foo"),
	})

	if len(client.published) != 0 {
		t.Errorf("expected no inquiry published for an unrecognised instance, got %+v", client.published)
	}
}

func TestSplitDiscoverEntityRequests(t *testing.T) {
	payload := "sensor.living_room_atmospheric_pressure;\nprotocols-server-2: sensor.office_bathroom_carbon_dioxide;\n\n  ;\nsensor.office_garden_connectivity"
	got := splitDiscoverEntityRequests(payload)
	want := []string{
		"sensor.living_room_atmospheric_pressure",
		"protocols-server-2: sensor.office_bathroom_carbon_dioxide",
		"sensor.office_garden_connectivity",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d requests %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("request[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestSubscribeDiscoverEntityRequestsHandlesSemicolonSeparatedBatch is the regression test for the
// user's own real request 2026-08-29: a whole pasted list of entities, one per line each ending in
// ";", some bare (broadcast to every instance) and some "<instance>: "-prefixed (routed to just
// that instance), submitted as a single text-field write.
func TestSubscribeDiscoverEntityRequestsHandlesSemicolonSeparatedBatch(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	client := &fakeClient{}
	if err := subscribeDiscoverEntityRequests(client, tracker, []string{"main", "protocols-server-2"}); err != nil {
		t.Fatalf("subscribeDiscoverEntityRequests error: %v", err)
	}

	payload := "sensor.living_room_atmospheric_pressure;\n" +
		"protocols-server-2: sensor.office_bathroom_carbon_dioxide;\n" +
		"main: sensor.office_garden_connectivity;"
	client.subscribedHandlers[0](client, fakeMessage{topic: discoverEntityCommandTopic(), payload: []byte(payload)})

	// Bare entity -- broadcast to both declared instances.
	for _, instance := range []string{"main", "protocols-server-2"} {
		byDevice := tracker.snapshotByDevice(instance)
		if _, found := byDevice[""]["sensor.living_room_atmospheric_pressure"]; !found {
			t.Errorf("expected the bare entity seeded for %q, got %+v", instance, byDevice)
		}
	}

	// Instance-scoped entries -- each only on its own named instance.
	psByDevice := tracker.snapshotByDevice("protocols-server-2")
	if _, found := psByDevice[""]["sensor.office_bathroom_carbon_dioxide"]; !found {
		t.Errorf("expected sensor.office_bathroom_carbon_dioxide seeded for protocols-server-2, got %+v", psByDevice)
	}
	if _, found := psByDevice[""]["sensor.office_garden_connectivity"]; found {
		t.Errorf("expected sensor.office_garden_connectivity NOT seeded for protocols-server-2 (main-scoped), got %+v", psByDevice)
	}
	mainByDevice := tracker.snapshotByDevice("main")
	if _, found := mainByDevice[""]["sensor.office_garden_connectivity"]; !found {
		t.Errorf("expected sensor.office_garden_connectivity seeded for main, got %+v", mainByDevice)
	}
	if _, found := mainByDevice[""]["sensor.office_bathroom_carbon_dioxide"]; found {
		t.Errorf("expected sensor.office_bathroom_carbon_dioxide NOT seeded for main (protocols-server-2-scoped), got %+v", mainByDevice)
	}

	// One inquiry publish per (entity, instance) pair actually targeted: 2 (bare, both instances) +
	// 1 (protocols-server-2-scoped) + 1 (main-scoped) = 4.
	if len(client.published) != 4 {
		t.Errorf("got %d inquiry publishes, want 4, published = %+v", len(client.published), client.published)
	}
}

// TestSubscribeDiscoverEntityRequestsBatchContinuesPastUnknownInstance confirms one bad entry
// (an unrecognised instance prefix) in a semicolon-separated batch doesn't abort the rest of it.
func TestSubscribeDiscoverEntityRequestsBatchContinuesPastUnknownInstance(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	client := &fakeClient{}
	if err := subscribeDiscoverEntityRequests(client, tracker, []string{"main"}); err != nil {
		t.Fatalf("subscribeDiscoverEntityRequests error: %v", err)
	}

	payload := "not-a-real-instance: sensor.foo;\nsensor.office_garden_connectivity;"
	client.subscribedHandlers[0](client, fakeMessage{topic: discoverEntityCommandTopic(), payload: []byte(payload)})

	byDevice := tracker.snapshotByDevice("main")
	if _, found := byDevice[""]["sensor.office_garden_connectivity"]; !found {
		t.Errorf("expected the entry after the bad one to still be processed, got %+v", byDevice)
	}
	if _, found := byDevice[""]["sensor.foo"]; found {
		t.Errorf("expected the unrecognised-instance entry to be dropped, got %+v", byDevice)
	}
}

func TestSubscribeDiscoverEntityRequestsNoInstancesIsNoop(t *testing.T) {
	tracker := newEntityExistenceTracker("")
	client := &fakeClient{}
	if err := subscribeDiscoverEntityRequests(client, tracker, nil); err != nil {
		t.Fatalf("subscribeDiscoverEntityRequests error: %v", err)
	}
	if len(client.subscribedHandlers) != 0 {
		t.Errorf("expected no subscription when instances is empty, got %d", len(client.subscribedHandlers))
	}
}
