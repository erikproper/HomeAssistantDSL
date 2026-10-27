package main

import (
	"testing"
)

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

// TestSubscribeMetaInstanceCloudRelayRelaysToLocalTopic is a regression test for a real gap found
// live 2026-09-08: PublishMetaReload (the deploy scripts' own reload trigger) connects directly to
// the local broker by bare hostname, which fails with a DNS lookup error when run from off this
// house's own LAN. This asserts a cloud-arriving, installation-qualified per-instance topic relays
// to the plain local instance topic, and never touches any other topic.
func TestSubscribeMetaInstanceCloudRelayRelaysToLocalTopic(t *testing.T) {
	client := &fakeClient{}
	cloudClient := &fakeClient{}
	if err := subscribeMetaInstanceCloudRelay(client, cloudClient, "vienna", []string{"main", "protocols-server-2"}); err != nil {
		t.Fatalf("subscribeMetaInstanceCloudRelay error: %v", err)
	}

	handlers := cloudClient.subscribedHandlersSnapshot()
	// 2 instances * 2 actions (reload, restart) = 4 cloud subscriptions.
	if len(handlers) != 4 {
		t.Fatalf("got %d cloud handlers, want 4: %v", len(handlers), handlers)
	}
	reloadMainHandler := handlers[0]
	reloadMainHandler(cloudClient, fakeMessage{topic: "vienna/meta/reload/main/request", payload: []byte("PRESS")})

	published := client.publishedSnapshot()
	if len(published) != 1 || published[0].topic != "meta/reload/main/request" {
		t.Errorf("expected exactly one publish to %q, got %+v", "meta/reload/main/request", published)
	}
	if len(cloudClient.publishedSnapshot()) != 0 {
		t.Errorf("must never republish anything back onto the cloud broker, got %+v", cloudClient.publishedSnapshot())
	}
}

func TestSubscribeMetaInstanceCloudRelayNoOpWithoutCloudOrInstallation(t *testing.T) {
	client := &fakeClient{}
	if err := subscribeMetaInstanceCloudRelay(client, nil, "vienna", []string{"main"}); err != nil {
		t.Fatalf("subscribeMetaInstanceCloudRelay error: %v", err)
	}
	cloudClient := &fakeClient{}
	if err := subscribeMetaInstanceCloudRelay(client, cloudClient, "", []string{"main"}); err != nil {
		t.Fatalf("subscribeMetaInstanceCloudRelay error: %v", err)
	}
	if len(cloudClient.subscribedHandlersSnapshot()) != 0 {
		t.Errorf("expected no subscriptions with an empty installation name, got %d", len(cloudClient.subscribedHandlersSnapshot()))
	}
}
