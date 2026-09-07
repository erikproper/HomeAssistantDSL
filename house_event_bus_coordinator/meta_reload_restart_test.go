package main

import (
	"encoding/json"
	"testing"
)

func TestPublishMetaReloadRestartButtonsPublishesEveryScope(t *testing.T) {
	client := &fakeClient{}
	if err := publishMetaReloadRestartButtons(client, testPrefix, "junglinster", []string{"main", "protocols-server-2"}); err != nil {
		t.Fatalf("publishMetaReloadRestartButtons error: %v", err)
	}

	// 2 instances + installation + "all" = 4 targets, times 2 actions = 8 buttons.
	if len(client.published) != 8 {
		t.Fatalf("got %d publishes, want 8: %+v", len(client.published), client.published)
	}

	wantTopic, _ := discoveryTopic(testPrefix, "button", "meta_reload_main"), true
	found := false
	for _, p := range client.published {
		if p.topic != wantTopic {
			continue
		}
		found = true
		var body map[string]interface{}
		if err := json.Unmarshal(p.payload, &body); err != nil {
			t.Fatalf("unmarshalling button payload: %v", err)
		}
		if body["command_topic"] != "meta/reload/main/request" {
			t.Errorf("command_topic = %v, want %q", body["command_topic"], "meta/reload/main/request")
		}
		if body["name"] != "meta/reload/main" {
			t.Errorf("name = %v, want %q", body["name"], "meta/reload/main")
		}
	}
	if !found {
		t.Errorf("expected a button published at %q, got %+v", wantTopic, client.published)
	}

	installationTopic := discoveryTopic(testPrefix, "button", "meta_restart_junglinster")
	allTopic := discoveryTopic(testPrefix, "button", "meta_restart_all")
	for _, want := range []string{installationTopic, allTopic} {
		found := false
		for _, p := range client.published {
			if p.topic == want {
				found = true
			}
		}
		if !found {
			t.Errorf("expected a button published at %q, got %+v", want, client.published)
		}
	}
}

// TestExpectedMetaReloadRestartTopicsMatchesPublishedTopics is a regression test for a real bug
// found live 2026-09-05: every meta button vanished from the broker within the same startup pass
// that published it, because watchForOrphanedDiscoveryTopics only ever protects topics listed in
// main.go's own expectedTopics set (the same mechanism discoverEntityStableID's own topic already
// needs, discover_entity_input.go) -- these coordinator-owned buttons were never added there, so
// they were retired as "unexpected" moments after publishMetaReloadRestartButtons published them.
// This asserts expectedMetaReloadRestartTopics names exactly the same topics
// publishMetaReloadRestartButtons actually publishes to, so the two can never drift apart again.
func TestExpectedMetaReloadRestartTopicsMatchesPublishedTopics(t *testing.T) {
	client := &fakeClient{}
	installation := "junglinster"
	instances := []string{"main", "protocols-server-2"}
	if err := publishMetaReloadRestartButtons(client, testPrefix, installation, instances); err != nil {
		t.Fatalf("publishMetaReloadRestartButtons error: %v", err)
	}

	expected := expectedMetaReloadRestartTopics(testPrefix, installation, instances)
	if len(expected) != len(client.published) {
		t.Fatalf("expectedMetaReloadRestartTopics has %d topics, but %d buttons were actually published", len(expected), len(client.published))
	}
	for _, p := range client.published {
		if !expected[p.topic] {
			t.Errorf("published topic %q is not in expectedMetaReloadRestartTopics -- watchForOrphanedDiscoveryTopics would retire it", p.topic)
		}
	}
}

func TestSubscribeMetaFanOutInstallationScopeFansOutLocallyOnly(t *testing.T) {
	client := &fakeClient{}
	cloudClient := &fakeClient{}
	if err := subscribeMetaFanOut(client, cloudClient, "junglinster", []string{"main", "protocols-server-2"}); err != nil {
		t.Fatalf("subscribeMetaFanOut error: %v", err)
	}

	handlers := client.subscribedHandlersSnapshot()
	// Per action: installation handler, then local "all" handler -- reload then restart.
	if len(handlers) != 4 {
		t.Fatalf("got %d local handlers, want 4: %v", len(handlers), handlers)
	}
	installationReloadHandler := handlers[0]
	installationReloadHandler(client, fakeMessage{topic: metaActionTopic("reload", "junglinster"), payload: []byte("PRESS")})

	want := map[string]bool{
		"meta/reload/main/request":               true,
		"meta/reload/protocols-server-2/request": true,
	}
	got := map[string]bool{}
	for _, p := range client.publishedSnapshot() {
		got[p.topic] = true
	}
	for topic := range want {
		if !got[topic] {
			t.Errorf("expected a publish to %q, got %+v", topic, client.publishedSnapshot())
		}
	}
	if len(cloudClient.publishedSnapshot()) != 0 {
		t.Errorf("installation scope must never touch the cloud broker, got %+v", cloudClient.publishedSnapshot())
	}
}

func TestSubscribeMetaFanOutAllScopeLocalPressFansOutAndCrossPostsOnce(t *testing.T) {
	client := &fakeClient{}
	cloudClient := &fakeClient{}
	if err := subscribeMetaFanOut(client, cloudClient, "junglinster", []string{"main", "protocols-server-2"}); err != nil {
		t.Fatalf("subscribeMetaFanOut error: %v", err)
	}

	handlers := client.subscribedHandlersSnapshot()
	localAllReloadHandler := handlers[1] // installation, then "all", per action
	localAllReloadHandler(client, fakeMessage{topic: metaActionTopic("reload", "all"), payload: []byte("PRESS")})

	got := map[string]bool{}
	for _, p := range client.publishedSnapshot() {
		got[p.topic] = true
	}
	if !got["meta/reload/main/request"] || !got["meta/reload/protocols-server-2/request"] {
		t.Errorf("expected local fan-out to both instances, got %+v", client.publishedSnapshot())
	}

	cloudPublishes := cloudClient.publishedSnapshot()
	if len(cloudPublishes) != 1 || cloudPublishes[0].topic != "meta/reload/all/request" {
		t.Errorf("expected exactly one cross-post to the cloud \"all\" topic, got %+v", cloudPublishes)
	}
}

// TestSubscribeMetaFanOutAllScopeCloudArrivalNeverRepublishes is a regression test mirroring
// discoveryhassbridge.go's own self-import feedback-loop fix: an "all" press arriving FROM the
// cloud broker (i.e. a sibling house's own cross-post) must fan out to this house's own local
// instances but never republish anywhere -- neither back onto the cloud "all" topic nor onto the
// local "all" topic -- or every house's own cross-post would bounce off every other house forever.
func TestSubscribeMetaFanOutAllScopeCloudArrivalNeverRepublishes(t *testing.T) {
	client := &fakeClient{}
	cloudClient := &fakeClient{}
	if err := subscribeMetaFanOut(client, cloudClient, "junglinster", []string{"main", "protocols-server-2"}); err != nil {
		t.Fatalf("subscribeMetaFanOut error: %v", err)
	}

	cloudHandlers := cloudClient.subscribedHandlersSnapshot()
	if len(cloudHandlers) != 2 { // reload "all", restart "all"
		t.Fatalf("got %d cloud handlers, want 2: %v", len(cloudHandlers), cloudHandlers)
	}
	cloudAllReloadHandler := cloudHandlers[0]
	cloudAllReloadHandler(cloudClient, fakeMessage{topic: metaActionTopic("reload", "all"), payload: []byte("PRESS")})

	got := map[string]bool{}
	for _, p := range client.publishedSnapshot() {
		got[p.topic] = true
	}
	if !got["meta/reload/main/request"] || !got["meta/reload/protocols-server-2/request"] {
		t.Errorf("expected local fan-out to both instances, got %+v", client.publishedSnapshot())
	}
	if got["meta/reload/all/request"] {
		t.Errorf("must never republish onto the local \"all\" topic on cloud arrival, got %+v", client.publishedSnapshot())
	}
	if len(cloudClient.publishedSnapshot()) != 0 {
		t.Errorf("must never republish anything back onto the cloud broker on cloud arrival, got %+v", cloudClient.publishedSnapshot())
	}
}

func TestSubscribeMetaFanOutNoOpWithNoInstancesOrInstallation(t *testing.T) {
	client := &fakeClient{}
	cloudClient := &fakeClient{}
	if err := subscribeMetaFanOut(client, cloudClient, "", []string{"main"}); err != nil {
		t.Fatalf("subscribeMetaFanOut error: %v", err)
	}
	if len(client.subscribedHandlersSnapshot()) != 0 {
		t.Errorf("expected no subscriptions with an empty installation name, got %d", len(client.subscribedHandlersSnapshot()))
	}
	if err := subscribeMetaFanOut(client, cloudClient, "junglinster", nil); err != nil {
		t.Fatalf("subscribeMetaFanOut error: %v", err)
	}
	if len(client.subscribedHandlersSnapshot()) != 0 {
		t.Errorf("expected no subscriptions with no instances, got %d", len(client.subscribedHandlersSnapshot()))
	}
}

func TestPublishMetaReloadRestartButtonsNoOpWithNoInstallation(t *testing.T) {
	client := &fakeClient{}
	if err := publishMetaReloadRestartButtons(client, testPrefix, "", []string{"main"}); err != nil {
		t.Fatalf("publishMetaReloadRestartButtons error: %v", err)
	}
	if len(client.published) != 0 {
		t.Errorf("expected no buttons published with an empty installation name, got %+v", client.published)
	}
}
