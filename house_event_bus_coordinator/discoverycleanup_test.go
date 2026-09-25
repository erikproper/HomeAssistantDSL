package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// --- minimal mqtt.Client/Token/Message fakes, just enough for these tests -- no real broker. ---

type fakeToken struct{}

func (fakeToken) Wait() bool                     { return true }
func (fakeToken) WaitTimeout(time.Duration) bool { return true }
func (fakeToken) Done() <-chan struct{}          { ch := make(chan struct{}); close(ch); return ch }
func (fakeToken) Error() error                   { return nil }

type recordedPublish struct {
	topic   string
	payload []byte
}

type fakeMessage struct {
	topic   string
	payload []byte
}

func (m fakeMessage) Duplicate() bool   { return false }
func (m fakeMessage) Qos() byte         { return 0 }
func (m fakeMessage) Retained() bool    { return true }
func (m fakeMessage) Topic() string     { return m.topic }
func (m fakeMessage) MessageID() uint16 { return 0 }
func (m fakeMessage) Payload() []byte   { return m.payload }
func (m fakeMessage) Ack()              {}

// fakeClient records every Publish call, and -- on Subscribe -- synchronously "delivers" whatever
// retained messages were pre-seeded, matching a real broker's prompt retained-message delivery
// closely enough for these tests without any goroutine/timing complexity. mu guards
// published/subscribedHandlers specifically for tests exercising production code that spawns its
// own goroutine to call Subscribe/Publish (e.g. subscribeImportedDevices's relay setup,
// discoveryimport_test.go) -- tests with no concurrent access of their own can keep reading the
// fields directly without locking, same as before.
type fakeClient struct {
	mqtt.Client
	mu                 sync.Mutex
	published          []recordedPublish
	retained           []fakeMessage
	subscribedHandlers []mqtt.MessageHandler
}

func (c *fakeClient) Publish(topic string, _ byte, _ bool, payload interface{}) mqtt.Token {
	var data []byte
	switch p := payload.(type) {
	case []byte:
		data = p
	case string:
		data = []byte(p)
	}
	c.mu.Lock()
	c.published = append(c.published, recordedPublish{topic: topic, payload: data})
	c.mu.Unlock()
	return fakeToken{}
}

// publishedSnapshot/subscribedHandlersSnapshot are the thread-safe way to read
// c.published/c.subscribedHandlers from a test that also exercises a concurrent Subscribe/Publish
// (see this type's own doc comment).
func (c *fakeClient) publishedSnapshot() []recordedPublish {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]recordedPublish{}, c.published...)
}

func (c *fakeClient) subscribedHandlersSnapshot() []mqtt.MessageHandler {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]mqtt.MessageHandler{}, c.subscribedHandlers...)
}

func (c *fakeClient) Subscribe(_ string, _ byte, callback mqtt.MessageHandler) mqtt.Token {
	c.mu.Lock()
	c.subscribedHandlers = append(c.subscribedHandlers, callback)
	c.mu.Unlock()
	for _, m := range c.retained {
		callback(c, m)
	}
	return fakeToken{}
}

// --- isCoordinatorOwnedTopic ---

func TestIsCoordinatorOwnedTopicRecognizesCurrentAndLegacyShapes(t *testing.T) {
	cases := map[string]bool{
		"homeassistant/sensor/coordinator/host_smarty_temperature/config":  true,
		"homeassistant/binary_sensor/host_smarty/some_reading/config":      true,
		"homeassistant/sensor/discovery_ems_esp/boiler_outdoortemp/config": true,
		"homeassistant/sensor/zigbee2mqtt/some_other_device/config":        false,
		"homeassistant/sensor/coordinator/host_smarty_temperature/state":   false,
	}
	for topic, want := range cases {
		if got := isCoordinatorOwnedTopic(topic, testPrefix); got != want {
			t.Errorf("isCoordinatorOwnedTopic(%q, %q) = %v, want %v", topic, testPrefix, got, want)
		}
	}
	if isCoordinatorOwnedTopic("homeassistant/sensor/coordinator/host_smarty_node/config", "homeassistant.physical") {
		t.Errorf("expected a topic under a different prefix to not be recognised as ours")
	}
}

func TestIsCoordinatorOwnedTopicWithCompoundInstallationQualifiedPrefix(t *testing.T) {
	compoundPrefix := "junglinster/homeassistant"
	if !isCoordinatorOwnedTopic("junglinster/homeassistant/sensor/coordinator/host_smarty_node/config", compoundPrefix) {
		t.Errorf("expected a topic under this installation's own compound prefix to be recognised as ours")
	}
	// A sibling installation's own qualified topic on the same shared broker must never match --
	// this is what keeps watchForOrphanedDiscoveryTopics scoped to only this installation's own
	// cloud-broker cleanup (main.go).
	if isCoordinatorOwnedTopic("vienna/homeassistant/sensor/coordinator/host_smarty_node/config", compoundPrefix) {
		t.Errorf("expected another installation's own qualified topic to not be recognised as ours")
	}
}

// --- manifest loading ---

func TestLoadTopicManifestMissingFileReturnsEmpty(t *testing.T) {
	got := loadTopicManifest(filepath.Join(t.TempDir(), "does_not_exist.json"))
	if len(got) != 0 {
		t.Errorf("got %v, want an empty map for a missing file", got)
	}
}

func TestLoadTopicManifestFallsBackToEmptyOnOldArrayShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "discovery_topics.json")
	if err := os.WriteFile(path, []byte(`{"topics": ["a", "b"]}`), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	got := loadTopicManifest(path)
	if len(got) != 0 {
		t.Errorf("got %v, want an empty map for the previous fix's plain-array shape", got)
	}
}

// --- TDiscoveryPublisher ---

func TestDiscoveryPublisherPublishesFreshTopicWithoutRetiring(t *testing.T) {
	client := &fakeClient{}
	p := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	topic := "homeassistant/sensor/coordinator/host_x_node/config"

	if err := p.Publish(client, "main", topic, []byte(`{"a":1}`)); err != nil {
		t.Fatalf("Publish error: %v", err)
	}
	if len(client.published) != 1 {
		t.Fatalf("got %d publishes, want 1 (no retirement for a brand-new topic): %+v", len(client.published), client.published)
	}
	if string(client.published[0].payload) != `{"a":1}` {
		t.Errorf("published payload = %q, want the given content", client.published[0].payload)
	}
}

func TestDiscoveryPublisherSkipsUnchangedContent(t *testing.T) {
	client := &fakeClient{}
	p := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	topic := "homeassistant/sensor/coordinator/host_x_node/config"

	if err := p.Publish(client, "main", topic, []byte(`{"a":1}`)); err != nil {
		t.Fatalf("first Publish error: %v", err)
	}
	if err := p.Publish(client, "main", topic, []byte(`{"a":1}`)); err != nil {
		t.Fatalf("second Publish error: %v", err)
	}
	if len(client.published) != 1 {
		t.Fatalf("got %d publishes, want 1 (identical republish should be a no-op): %+v", len(client.published), client.published)
	}
}

func TestDiscoveryPublisherRetiresBeforeRepublishOnContentChange(t *testing.T) {
	client := &fakeClient{}
	p := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	topic := "homeassistant/sensor/coordinator/host_x_node/config"

	if err := p.Publish(client, "main", topic, []byte(`{"a":1}`)); err != nil {
		t.Fatalf("first Publish error: %v", err)
	}
	if err := p.Publish(client, "main", topic, []byte(`{"a":2}`)); err != nil {
		t.Fatalf("second Publish error: %v", err)
	}
	if len(client.published) != 3 {
		t.Fatalf("got %d publishes, want 3 (initial, empty retirement, corrected republish): %+v", len(client.published), client.published)
	}
	if len(client.published[1].payload) != 0 {
		t.Errorf("published[1] (the retirement) payload = %q, want empty", client.published[1].payload)
	}
	if string(client.published[2].payload) != `{"a":2}` {
		t.Errorf("published[2] (the republish) payload = %q, want the new content", client.published[2].payload)
	}
}

// TestDiscoveryPublisherKnows is discoverybridge.go's own passthrough-retire guard's coverage:
// Knows must report false before anything's been published to a topic (on a given broker), true
// immediately after, and false again once explicitly retired via RetireOne.
func TestDiscoveryPublisherKnows(t *testing.T) {
	client := &fakeClient{}
	p := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	topic := "homeassistant/sensor/zigbee2mqtt/some_device/config"

	if p.Knows("main", topic) {
		t.Errorf("Knows = true before anything was published, want false")
	}
	if err := p.Publish(client, "main", topic, []byte(`{"a":1}`)); err != nil {
		t.Fatalf("Publish error: %v", err)
	}
	if !p.Knows("main", topic) {
		t.Errorf("Knows = false after Publish, want true")
	}
	if p.Knows("cloud_coordinator", topic) {
		t.Errorf("Knows = true for a different broker label, want false (manifestKey is broker-scoped)")
	}
	p.RetireOne(client, "main", topic)
	if p.Knows("main", topic) {
		t.Errorf("Knows = true after RetireOne, want false")
	}
}

func TestDiscoveryPublisherRetireMissing(t *testing.T) {
	client := &fakeClient{}
	manifestPath := filepath.Join(t.TempDir(), "discovery_topics.json")
	p := newDiscoveryPublisher(manifestPath)

	if err := p.Publish(client, "main", "homeassistant/sensor/coordinator/a/config", []byte("a")); err != nil {
		t.Fatalf("Publish error: %v", err)
	}
	if err := p.Publish(client, "main", "homeassistant/sensor/coordinator/b/config", []byte("b")); err != nil {
		t.Fatalf("Publish error: %v", err)
	}

	jobs := make(chan TDiscoveryRelayJob, 100)
	retired := p.RetireMissing(client, "main", map[string]bool{"homeassistant/sensor/coordinator/b/config": true}, jobs)
	if len(retired) != 1 || retired[0] != "homeassistant/sensor/coordinator/a/config" {
		t.Errorf("RetireMissing returned %v, want [homeassistant/sensor/coordinator/a/config]", retired)
	}
	drainDiscoveryRelayQueue(jobs, p)
	// persist() itself is now debounced (schedulePersist); flush synchronously here rather than
	// waiting on a real timer -- see schedulePersist's own doc comment.
	p.persist()

	reloaded := newDiscoveryPublisher(manifestPath)
	if _, known := reloaded.known[manifestKey("main", "homeassistant/sensor/coordinator/a/config")]; known {
		t.Errorf("expected the retired topic to be gone from the persisted manifest")
	}
	if _, known := reloaded.known[manifestKey("main", "homeassistant/sensor/coordinator/b/config")]; !known {
		t.Errorf("expected the still-expected topic to remain in the persisted manifest")
	}
}

// TestDiscoveryPublisherRetireMissingExemptsPassthroughTopics is the regression test for the real
// incident this fix exists for (2026-09-14, PROJECT.md item 7): a passthrough-relayed Zigbee2MQTT
// topic is recorded as "known" (PublishPassthrough) but can never appear in any generator-computed
// expected set, since its whole point is that no Physical.def declaration exists for it yet. Before
// this fix, RetireMissing treated that as "no longer wanted" and deleted it on every single
// coordinator restart -- wiping the live HA entity (and any manual rename on it) each time, only for
// Zigbee2MQTT to eventually republish a fresh, unqualified one.
func TestDiscoveryPublisherRetireMissingExemptsPassthroughTopics(t *testing.T) {
	client := &fakeClient{}
	manifestPath := filepath.Join(t.TempDir(), "discovery_topics.json")
	p := newDiscoveryPublisher(manifestPath)

	passthroughTopic := "homeassistant/sensor/zigbee2mqtt/0x00158d0001aabbcc_temperature/config"
	if err := p.PublishPassthrough(client, "main", passthroughTopic, []byte("a")); err != nil {
		t.Fatalf("PublishPassthrough error: %v", err)
	}

	// expected is empty -- passthrough topics are never in any generator-computed expected set.
	jobs := make(chan TDiscoveryRelayJob, 100)
	retired := p.RetireMissing(client, "main", map[string]bool{}, jobs)
	if len(retired) != 0 {
		t.Errorf("RetireMissing retired %v, want nothing -- passthrough topics must be exempt", retired)
	}
	drainDiscoveryRelayQueue(jobs, p)
	if !p.Knows("main", passthroughTopic) {
		t.Errorf("expected the passthrough topic to still be known after RetireMissing")
	}
	// persist() itself is now debounced (schedulePersist); flush synchronously here rather than
	// waiting on a real timer -- see schedulePersist's own doc comment.
	p.persist()

	reloaded := newDiscoveryPublisher(manifestPath)
	if !reloaded.passthrough[manifestKey("main", passthroughTopic)] {
		t.Errorf("expected the passthrough marking to survive a manifest reload")
	}
	// A second RetireMissing call, after a fresh process restart (simulated by reloading from
	// disk), must still exempt it -- the exemption has to be persisted, not just in-memory.
	reloadedJobs := make(chan TDiscoveryRelayJob, 100)
	if retired := reloaded.RetireMissing(client, "main", map[string]bool{}, reloadedJobs); len(retired) != 0 {
		t.Errorf("RetireMissing after reload retired %v, want nothing", retired)
	}
	drainDiscoveryRelayQueue(reloadedJobs, reloaded)
}

// TestDiscoveryPublisherRetireOneForgetsPassthroughMarking confirms an explicit RetireOne (the
// path discoverybridge.go uses when a device migrates to a declared gateway, or is retracted
// upstream) clears the passthrough marking too, not just the known-content entry -- otherwise a
// later, unrelated topic reusing the exact same string could inherit a stale exemption.
func TestDiscoveryPublisherRetireOneForgetsPassthroughMarking(t *testing.T) {
	client := &fakeClient{}
	p := newDiscoveryPublisher(filepath.Join(t.TempDir(), "discovery_topics.json"))
	topic := "homeassistant/sensor/zigbee2mqtt/0x00158d0001aabbcc_temperature/config"

	if err := p.PublishPassthrough(client, "main", topic, []byte("a")); err != nil {
		t.Fatalf("PublishPassthrough error: %v", err)
	}
	p.RetireOne(client, "main", topic)

	if p.passthrough[manifestKey("main", topic)] {
		t.Errorf("expected the passthrough marking to be forgotten after RetireOne")
	}
}

// --- expected topic/payload computation ---

func TestExpectedHostsPayloadsMatchesBuildDiscoveryConfigs(t *testing.T) {
	devicesFile := TDevicesFile{Devices: map[string]TDevice{"host.smarty": smartyDevice()}}
	got, err := expectedHostsPayloads(devicesFile, true, NewLiveDeviceInfoStore("", nil))
	if err != nil {
		t.Fatalf("expectedHostsPayloads error: %v", err)
	}

	want := map[string]bool{}
	for _, cfg := range buildDiscoveryConfigs("host.smarty", smartyDevice(), nil, "", devicesFile.conceptualPrefix(), devicesFile.Installation) {
		want[cfg.Topic] = true
	}
	if len(got) != len(want) {
		t.Fatalf("got topics %v, want %v", got, want)
	}
	for topic := range want {
		if _, ok := got[topic]; !ok {
			t.Errorf("missing expected topic %q", topic)
		}
	}
}

// TestExpectedHostsPayloadsUsesPersistedLiveDeviceInfo is the regression test for a real bug found
// live 2026-09-09 (Vienna): expectedHostsPayloads used to always compute its baseline with
// live=nil, so a device whose manufacturer/model/sw_version/... was already learned and persisted
// across a PREVIOUS coordinator run had a startup "expected" payload that could never match what
// was actually, correctly sitting on the broker from before the restart (already live-enriched).
// watchForOrphanedDiscoveryTopics' startup content-check then wrongly retired the topic. Confirms
// expectedHostsPayloads now folds the store's own persisted snapshot into its baseline, matching
// buildDiscoveryConfigs(..., store.Snapshot(id), ...) exactly -- not the bare, live-data-free one.
func TestExpectedHostsPayloadsUsesPersistedLiveDeviceInfo(t *testing.T) {
	devicesFile := TDevicesFile{Devices: map[string]TDevice{"host.smarty": smartyDevice()}}

	store := NewLiveDeviceInfoStore("", nil)
	store.Update("host.smarty", map[string]string{"manufacturer": "raspberrypi", "sw_version": "Fedora Linux 44"})

	got, err := expectedHostsPayloads(devicesFile, true, store)
	if err != nil {
		t.Fatalf("expectedHostsPayloads error: %v", err)
	}

	wantEnriched := map[string]string{}
	for _, cfg := range buildDiscoveryConfigs("host.smarty", smartyDevice(), store.Snapshot("host.smarty"), "", devicesFile.conceptualPrefix(), devicesFile.Installation) {
		data, err := json.Marshal(cfg.Payload)
		if err != nil {
			t.Fatalf("marshalling want payload: %v", err)
		}
		wantEnriched[cfg.Topic] = string(data)
	}
	if len(wantEnriched) == 0 {
		t.Fatalf("test setup produced no discovery configs to compare against")
	}
	for topic, want := range wantEnriched {
		if got[topic] != want {
			t.Errorf("got[%q] = %s, want the live-enriched payload %s", topic, got[topic], want)
		}
	}

	// The bug's own symptom, made explicit: the bare (live=nil) baseline must NOT match what
	// expectedHostsPayloads now computes, for at least the device-info-bearing "device" fields --
	// otherwise this test would pass even with the old, broken behaviour.
	bare := map[string]string{}
	for _, cfg := range buildDiscoveryConfigs("host.smarty", smartyDevice(), nil, "", devicesFile.conceptualPrefix(), devicesFile.Installation) {
		data, err := json.Marshal(cfg.Payload)
		if err != nil {
			t.Fatalf("marshalling bare payload: %v", err)
		}
		bare[cfg.Topic] = string(data)
	}
	sawDifference := false
	for topic, want := range wantEnriched {
		if bare[topic] != want {
			sawDifference = true
			break
		}
	}
	if !sawDifference {
		t.Fatalf("test setup error: live-enriched and bare payloads are identical, so this test can't distinguish the fix from the bug")
	}
}

// TestExpectedCloudHostsPayloadsQualifiesTopicsAndSkipsNonCloudAndRealImportedDevices confirms
// expectedCloudHostsPayloads includes every Cloud device -- whether self-imported (ImportedFrom
// naming this house's own installation, the "cloud"+"import" case that replaced "native" 2026-09-02)
// or not imported at all -- and excludes a plain-local device and a REAL cross-house import (which
// never sets Cloud at all, so it was never a candidate for the cloud-expected set in the first
// place; this is the case the old "native"-based test used to call "cloud_native" and wrongly
// treat as excluded).
func TestExpectedCloudHostsPayloadsQualifiesTopicsAndSkipsNonCloudAndRealImportedDevices(t *testing.T) {
	cloudDevice := smartyDevice()
	cloudDevice.Cloud = true
	selfImportDevice := smartyDevice()
	selfImportDevice.Host = "eriks-macbook-pro-2"
	selfImportDevice.Topic = "hosts/eriks-macbook-pro-2/cpu/state"
	selfImportDevice.NodeTopic = "hosts/eriks-macbook-pro-2/node/state"
	selfImportDevice.Cloud, selfImportDevice.ImportedFrom = true, "junglinster"
	plainLocalDevice := smartyDevice()
	plainLocalDevice.Host = "plain"
	plainLocalDevice.Topic = "hosts/plain/cpu/state"
	importedDevice := smartyDevice()
	importedDevice.Host = "imported"
	importedDevice.Topic = "hosts/imported/cpu/state"
	importedDevice.ImportedFrom = "vienna"

	devicesFile := TDevicesFile{Devices: map[string]TDevice{
		"host.cloud_only":  cloudDevice,
		"host.self_import": selfImportDevice,
		"host.plain":       plainLocalDevice,
		"host.imported":    importedDevice,
	}}

	got, err := expectedCloudHostsPayloads(devicesFile, "junglinster", NewLiveDeviceInfoStore("", nil))
	if err != nil {
		t.Fatalf("expectedCloudHostsPayloads error: %v", err)
	}

	prefix := devicesFile.conceptualPrefix()
	wantCloudOnly := map[string]bool{}
	for _, cfg := range buildDiscoveryConfigs("host.cloud_only", cloudDevice, nil, "", prefix, "test") {
		stableID := exportStableID("junglinster", "host.cloud_only", cfg.Capability)
		wantCloudOnly["junglinster/"+discoveryTopic(prefix, cfg.Component, stableID)] = true
	}
	wantSelfImport := map[string]bool{}
	for _, cfg := range buildDiscoveryConfigs("host.self_import", selfImportDevice, nil, "", prefix, "test") {
		stableID := exportStableID("junglinster", "host.self_import", cfg.Capability)
		wantSelfImport["junglinster/"+discoveryTopic(prefix, cfg.Component, stableID)] = true
	}

	for topic := range wantCloudOnly {
		if _, ok := got[topic]; !ok {
			t.Errorf("missing expected cloud-only device topic %q in %v", topic, got)
		}
	}
	for topic := range wantSelfImport {
		if _, ok := got[topic]; !ok {
			t.Errorf("missing expected self-import device topic %q in %v -- self-import must still publish its cloud catalogue entry", topic, got)
		}
	}
	if len(got) != len(wantCloudOnly)+len(wantSelfImport) {
		t.Errorf("got %d topics %v, want exactly the cloud-routed devices' own %d topics (plain-local and a real cross-house import must be excluded)", len(got), got, len(wantCloudOnly)+len(wantSelfImport))
	}
	for topic := range got {
		if topic == "junglinster/hosts/plain/cpu/state" || topic == "junglinster/hosts/imported/cpu/state" {
			t.Errorf("plain-local/real-imported device topic %q must not appear in the cloud broker's expected set", topic)
		}
	}
}

func TestExpectedDiscoveryBridgeTopicsUsesGatewayAndLeaf(t *testing.T) {
	discoveryFile := TDiscoveryFile{
		EntityLinks: map[string]TDiscoveryEntityLink{
			"sensor.social_garage_door_temperature": {Gateway: "discovery.ems_esp", Leaf: "boiler_outdoortemp"},
		},
	}
	got := expectedDiscoveryBridgeTopics(discoveryFile, testPrefix)
	want := "homeassistant/sensor/coordinator/discovery_ems_esp_boiler_outdoortemp/config"
	if !got[want] {
		t.Fatalf("got %v, want a set containing %q", got, want)
	}

	// Renaming/repositioning the entity (a different key, same gateway+leaf) must yield the same
	// topic -- that's the whole point of keying on gateway+leaf rather than our own entity id.
	renamed := TDiscoveryFile{
		EntityLinks: map[string]TDiscoveryEntityLink{
			"sensor.infrastructural_boiler_outside_temp": {Gateway: "discovery.ems_esp", Leaf: "boiler_outdoortemp"},
		},
	}
	gotRenamed := expectedDiscoveryBridgeTopics(renamed, testPrefix)
	if !gotRenamed[want] {
		t.Errorf("got %v after rename, want the same topic %q unchanged", gotRenamed, want)
	}
}

// --- watchForOrphanedDiscoveryTopics ---

func withShortSettleWindow(t *testing.T) {
	t.Helper()
	orig := discoveryOrphanWatchSettleWindow
	discoveryOrphanWatchSettleWindow = time.Millisecond
	t.Cleanup(func() { discoveryOrphanWatchSettleWindow = orig })
}

func TestWatchForOrphanedDiscoveryTopicsRetiresContentMismatch(t *testing.T) {
	withShortSettleWindow(t)
	topic := "homeassistant/sensor/coordinator/host_x_node/config"
	client := &fakeClient{retained: []fakeMessage{{topic: topic, payload: []byte(`{"old":true}`)}}}

	publisher := newDiscoveryPublisher("")
	jobs := make(chan TDiscoveryRelayJob, 100)
	err := watchForOrphanedDiscoveryTopics(client, publisher, "main", map[string]bool{topic: true}, map[string]string{topic: `{"new":true}`}, map[string]bool{}, testPrefix, jobs)
	if err != nil {
		t.Fatalf("watchForOrphanedDiscoveryTopics error: %v", err)
	}
	drainDiscoveryRelayQueue(jobs, publisher)
	if len(client.published) != 1 {
		t.Fatalf("got %d publishes, want 1 (the retirement): %+v", len(client.published), client.published)
	}
	if client.published[0].topic != topic || len(client.published[0].payload) != 0 {
		t.Errorf("published = %+v, want an empty retained publish to %q", client.published[0], topic)
	}
}

func TestWatchForOrphanedDiscoveryTopicsLeavesMatchingContentAlone(t *testing.T) {
	withShortSettleWindow(t)
	topic := "homeassistant/sensor/coordinator/host_x_node/config"
	client := &fakeClient{retained: []fakeMessage{{topic: topic, payload: []byte(`{"same":true}`)}}}

	publisher := newDiscoveryPublisher("")
	jobs := make(chan TDiscoveryRelayJob, 100)
	err := watchForOrphanedDiscoveryTopics(client, publisher, "main", map[string]bool{topic: true}, map[string]string{topic: `{"same":true}`}, map[string]bool{}, testPrefix, jobs)
	if err != nil {
		t.Fatalf("watchForOrphanedDiscoveryTopics error: %v", err)
	}
	drainDiscoveryRelayQueue(jobs, publisher)
	if len(client.published) != 0 {
		t.Fatalf("got %d publishes, want 0 (content already matches): %+v", len(client.published), client.published)
	}
}

func TestWatchForOrphanedDiscoveryTopicsRetiresUnexpectedLegacyTopic(t *testing.T) {
	withShortSettleWindow(t)
	legacyTopic := "homeassistant/sensor/host_smarty/infrastructural_smarty_cpu_temperature/config"
	client := &fakeClient{retained: []fakeMessage{{topic: legacyTopic, payload: []byte(`{"old":true}`)}}}

	publisher := newDiscoveryPublisher("")
	jobs := make(chan TDiscoveryRelayJob, 100)
	err := watchForOrphanedDiscoveryTopics(client, publisher, "main", map[string]bool{}, map[string]string{}, map[string]bool{}, testPrefix, jobs)
	if err != nil {
		t.Fatalf("watchForOrphanedDiscoveryTopics error: %v", err)
	}
	drainDiscoveryRelayQueue(jobs, publisher)
	if len(client.published) != 1 || client.published[0].topic != legacyTopic {
		t.Fatalf("published = %+v, want a single empty retire of %q", client.published, legacyTopic)
	}
}

// TestWatchForOrphanedDiscoveryTopicsRetiresDeclaredCleanNodeUnconditionally covers
// mqtt_discovery_clean: a topic under an operator-declared legacy node_id (e.g. "ems-esp", never
// one of ours) is retired regardless of expectedTopics/expectedPayloads -- there's no legitimate
// content that could ever belong there once declared, so no settle-window restriction either.
func TestWatchForOrphanedDiscoveryTopicsRetiresDeclaredCleanNodeUnconditionally(t *testing.T) {
	withShortSettleWindow(t)
	legacyTopic := "homeassistant/number/ems-esp/thermostat_hc2_solarinfl/config"
	client := &fakeClient{retained: []fakeMessage{{topic: legacyTopic, payload: []byte(`{"anything":true}`)}}}

	publisher := newDiscoveryPublisher("")
	jobs := make(chan TDiscoveryRelayJob, 100)
	err := watchForOrphanedDiscoveryTopics(client, publisher, "main", map[string]bool{}, map[string]string{}, map[string]bool{"ems-esp": true}, testPrefix, jobs)
	if err != nil {
		t.Fatalf("watchForOrphanedDiscoveryTopics error: %v", err)
	}
	drainDiscoveryRelayQueue(jobs, publisher)
	if len(client.published) != 1 || client.published[0].topic != legacyTopic {
		t.Fatalf("published = %+v, want a single empty retire of %q", client.published, legacyTopic)
	}

	// Also confirm it's swept after the settle window, and for an entirely different
	// component/object_id under the same declared node_id -- the whole point of the wildcard.
	other := "homeassistant/climate/ems-esp/thermostat_hc1/config"
	client.subscribedHandlers[0](client, fakeMessage{topic: other, payload: []byte(`{"else":true}`)})
	drainDiscoveryRelayQueue(jobs, publisher)
	if len(client.published) != 2 || client.published[1].topic != other {
		t.Fatalf("published = %+v, want the second, differently-shaped topic also retired", client.published)
	}
}

func TestLoadDiscoveryCleanupFileMissingReturnsZeroValue(t *testing.T) {
	got, err := loadDiscoveryCleanupFile(filepath.Join(t.TempDir(), "does_not_exist.yaml"))
	if err != nil {
		t.Fatalf("loadDiscoveryCleanupFile error: %v", err)
	}
	if len(got.CleanTopics) != 0 {
		t.Errorf("got %v, want a zero value for a missing file", got)
	}
}

// TestWatchForOrphanedDiscoveryTopicsIgnoresContentDriftAfterSettleWindow is a regression test
// for the real bug this fix was for: expectedPayloads is a static, live-data-free snapshot, but a
// device's correct steady-state content legitimately gains fields (e.g. via_device) once live
// data starts flowing in after startup. A live-enriched republish delivered to this same
// subscription *after* the settle window must not be mistaken for drift and retired -- that would
// mean the watcher fights subscribeDeviceInfo's own, entirely correct, live-update republish.
func TestWatchForOrphanedDiscoveryTopicsIgnoresContentDriftAfterSettleWindow(t *testing.T) {
	withShortSettleWindow(t)
	topic := "homeassistant/sensor/coordinator/host_backups_node/config"
	client := &fakeClient{} // nothing retained yet at subscribe time

	publisher := newDiscoveryPublisher("")
	jobs := make(chan TDiscoveryRelayJob, 100)
	if err := watchForOrphanedDiscoveryTopics(client, publisher, "main", map[string]bool{topic: true}, map[string]string{topic: `{"live_enriched":false}`}, map[string]bool{}, testPrefix, jobs); err != nil {
		t.Fatalf("watchForOrphanedDiscoveryTopics error: %v", err)
	}

	// Simulate a live-enriched republish arriving well after the settle window -- e.g. via_device
	// resolved from a retained "hosts/+/device/state" message subscribeDeviceInfo just processed.
	if len(client.subscribedHandlers) != 1 {
		t.Fatalf("expected exactly one Subscribe call, got %d", len(client.subscribedHandlers))
	}
	client.subscribedHandlers[0](client, fakeMessage{topic: topic, payload: []byte(`{"live_enriched":true}`)})
	drainDiscoveryRelayQueue(jobs, publisher)

	if len(client.published) != 0 {
		t.Fatalf("got %d publishes, want 0 (a legitimate live-enriched republish must not be retired): %+v", len(client.published), client.published)
	}
}

// TestWatchForOrphanedDiscoveryTopicsForgetsRetiredTopicFromManifest is the regression test for a
// real bug found live 2026-08-30: retiring an orphaned topic must also forget it from publisher's
// own manifest, or a later legitimate Publish call with the exact same (now-stale) content is
// silently skipped by Publish's own unchanged-content fast path -- the coordinator believes it
// already republished, but the broker has nothing there since it was just retired. Confirmed live:
// PROJECT.md 1.2b's cloud-side hassbridge export discovery configs were correctly added to the
// expected set, redeployed, and still never reappeared on the broker, because the manifest still
// remembered them from before the (then-buggy) orphan retirement wiped them off the wire.
func TestWatchForOrphanedDiscoveryTopicsForgetsRetiredTopicFromManifest(t *testing.T) {
	withShortSettleWindow(t)
	topic := "homeassistant/sensor/coordinator/hassbridge_sensor_infrastructural_vienna_terrace_temperature/config"
	content := `{"unique_id":"hassbridge_..."}`

	publisher := newDiscoveryPublisher("")
	client := &fakeClient{}
	if err := publisher.Publish(client, "cloud_coordinator", topic, []byte(content)); err != nil {
		t.Fatalf("seeding publisher.Publish error: %v", err)
	}
	client.published = nil // the seeding publish isn't part of what this test checks

	// Now the topic is no longer expected (e.g. "export" got removed) -- the watcher sees it
	// arrive and retires it.
	watchClient := &fakeClient{retained: []fakeMessage{{topic: topic, payload: []byte(content)}}}
	jobs := make(chan TDiscoveryRelayJob, 100)
	if err := watchForOrphanedDiscoveryTopics(watchClient, publisher, "cloud_coordinator", map[string]bool{}, map[string]string{}, map[string]bool{}, testPrefix, jobs); err != nil {
		t.Fatalf("watchForOrphanedDiscoveryTopics error: %v", err)
	}
	drainDiscoveryRelayQueue(jobs, publisher)
	if len(watchClient.published) != 1 || len(watchClient.published[0].payload) != 0 {
		t.Fatalf("published = %+v, want a single empty retire", watchClient.published)
	}

	// The device is exported again (or was never really meant to be removed) -- a fresh Publish
	// call with the SAME content must actually go out on the wire, not be skipped as "unchanged".
	republishClient := &fakeClient{}
	if err := publisher.Publish(republishClient, "cloud_coordinator", topic, []byte(content)); err != nil {
		t.Fatalf("re-publish error: %v", err)
	}
	if len(republishClient.published) != 1 || string(republishClient.published[0].payload) != content {
		t.Errorf("republish after retirement = %+v, want the config actually republished, not silently skipped as already-known", republishClient.published)
	}
}

func TestWatchForOrphanedDiscoveryTopicsIgnoresUnrelatedTopics(t *testing.T) {
	withShortSettleWindow(t)
	other := "homeassistant/sensor/zigbee2mqtt/some_other_device/config"
	client := &fakeClient{retained: []fakeMessage{{topic: other, payload: []byte(`{"unrelated":true}`)}}}

	publisher := newDiscoveryPublisher("")
	jobs := make(chan TDiscoveryRelayJob, 100)
	err := watchForOrphanedDiscoveryTopics(client, publisher, "main", map[string]bool{}, map[string]string{}, map[string]bool{}, testPrefix, jobs)
	if err != nil {
		t.Fatalf("watchForOrphanedDiscoveryTopics error: %v", err)
	}
	drainDiscoveryRelayQueue(jobs, publisher)
	if len(client.published) != 0 {
		t.Fatalf("got %d publishes, want 0 (not a coordinator-owned topic): %+v", len(client.published), client.published)
	}
}
