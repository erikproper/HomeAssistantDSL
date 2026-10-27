/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: MQTT
 *
 * Connects to the house's main MQTT broker, publishes discovery configs once at startup
 * (retained, so they survive a coordinator restart without republishing), and subscribes to
 * "hosts/#" purely as a debug/sanity aid -- logging that device traffic is actually flowing.
 * The coordinator never bridges or republishes device state itself: Home Assistant subscribes
 * directly to the same topics the discovery payloads point at (Architecture.md §6, the
 * coordinator's job here is projection, not proxying).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 21.08.2026
 *
 */

package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// TReconnectHook lets main() register, well after both broker connections are already
// established and every dependency (store/publisher/existence trackers/etc.) exists, a single
// "republish everything idempotent" callback that fires on every SUBSEQUENT (re)connect of
// either client -- found live 2026-09-09 migrating Vienna's local broker off Green's own add-on
// onto a dedicated container: the coordinator's own discovery/status publishing only ever ran
// once, at process startup, so a device's local discovery config (host.frame's own commandline
// switch, cross-house-imported Netatmo capabilities) never reappeared on the NEW, empty broker
// until the coordinator process itself was restarted -- a broker-side change (or any dropped
// connection) should be self-healing, not require a manual coordinator bump every time.
//
// Set(fn) is called once, near the end of main(), once every real dependency exists; call() is
// what each connectMQTT client's own OnConnectHandler actually invokes on every (re)connect,
// including the very first -- a nil fn (the state before Set has run) is a safe no-op, since
// main()'s own sequential startup code already does the FIRST-time setup explicitly and
// deterministically; only genuine reconnects after that point ever call a non-nil fn. Mutex-
// guarded since paho invokes OnConnectHandler on its own internal goroutine, concurrently with
// whatever main() is still doing.
//
// Subscriptions are renewed separately, before this hook runs (subscriptionRegistry below).
// TSubscriptionRegistry remembers every topic each client subscribes to, so a reconnect can renew
// them (2026-10-26, found live: Junglinster's coordinator reconnected to the cloud broker after a
// DNS outage, but received nothing from it any more -- no ping/cpu host behind the cloud broker
// reported, for hours). The client uses a clean session, so the broker forgets its subscriptions
// on a disconnect, and paho's SetResumeSubs only resends subscribe requests still in flight, not
// established ones. Renewing sends the topics again with a nil handler: paho then keeps the
// handler it already routes the topic to (client.Subscribe only replaces a route when given a
// handler), so nothing is ever handled twice.
type TSubscriptionRegistry struct {
	mu       sync.Mutex
	byClient map[mqtt.Client]map[string]byte // client -> topic filter -> QoS
}

var subscriptionRegistry = &TSubscriptionRegistry{byClient: map[mqtt.Client]map[string]byte{}}

// subscribe subscribes client to topic like client.Subscribe, and records it for renewal.
func subscribe(client mqtt.Client, topic string, qos byte, handler mqtt.MessageHandler) mqtt.Token {
	subscriptionRegistry.mu.Lock()
	if subscriptionRegistry.byClient[client] == nil {
		subscriptionRegistry.byClient[client] = map[string]byte{}
	}
	subscriptionRegistry.byClient[client][topic] = qos
	subscriptionRegistry.mu.Unlock()
	return client.Subscribe(topic, qos, handler)
}

// renew subscribes client again to every topic it subscribed to before; a no-op on the first
// connect, when there are none yet.
func (r *TSubscriptionRegistry) renew(client mqtt.Client) {
	r.mu.Lock()
	topics := make(map[string]byte, len(r.byClient[client]))
	for topic, qos := range r.byClient[client] {
		topics[topic] = qos
	}
	r.mu.Unlock()
	if len(topics) == 0 {
		return
	}
	fmt.Printf("[reconnect] renewing %d subscription(s)\n", len(topics))
	token := client.SubscribeMultiple(topics, nil)
	go func() {
		if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
			fmt.Printf("[reconnect] renewing subscriptions failed: %v\n", token.Error())
		}
	}()
}

type TReconnectHook struct {
	mu sync.Mutex
	fn func()
}

func (h *TReconnectHook) Set(fn func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.fn = fn
}

func (h *TReconnectHook) call() {
	h.mu.Lock()
	fn := h.fn
	h.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// connectMQTT connects to secrets' broker, blocking until connected or failed. secrets.TLS
// selects "ssl://" over a plain "tcp://" broker URL, with an empty tls.Config -- meaning the
// system's own root CA pool, sufficient for a publicly-trusted certificate (e.g. Let's Encrypt,
// the cloud broker's own setup) without needing a custom CA file; nothing here supports a
// privately-issued/self-signed cloud broker certificate. Every (re)connect first renews the
// client's subscriptions (TSubscriptionRegistry), then calls hook -- a safe no-op until main()
// has set a real function on it, see TReconnectHook's own doc comment.
func connectMQTT(secrets TMQTTBrokerSecrets, clientID string, hook *TReconnectHook) (mqtt.Client, error) {
	opts := mqtt.NewClientOptions()
	scheme := "tcp"
	if secrets.TLS {
		scheme = "ssl"
		opts.SetTLSConfig(&tls.Config{})
	}
	opts.AddBroker(fmt.Sprintf("%s://%s:%s", scheme, secrets.Server, secrets.Port))
	opts.SetClientID(clientID)
	opts.SetUsername(secrets.Login)
	opts.SetPassword(secrets.Password)
	opts.SetAutoReconnect(true)
	opts.SetResumeSubs(true)
	opts.SetConnectTimeout(10 * time.Second)
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		subscriptionRegistry.renew(c)
		if hook != nil {
			hook.call()
		}
	})

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		if err := token.Error(); err != nil {
			return nil, fmt.Errorf("cannot connect to MQTT broker %s:%s: %w", secrets.Server, secrets.Port, err)
		}
		return nil, fmt.Errorf("cannot connect to MQTT broker %s:%s: timed out", secrets.Server, secrets.Port)
	}
	return client, nil
}

// publishDeviceDiscovery publishes one device's discovery configs through publisher (content-aware
// retire-then-republish, discoverycleanup.go), using store's current live snapshot for it and
// viaDeviceHostName resolved to another device's identifier via hostNameToDeviceID (silently
// unresolved -- "" -- if no match). Routes to mainClient/cloudClient per deviceRoutingPlan(device)
// (mqtt_relay.go) -- a plain local device publishes to main only, same as always; "cloud"-routed
// and imported devices route differently, see that function's own doc comment for the full table.
// Shared by publishDiscovery (startup, every device) and subscribeDeviceInfo's message handler
// (one device, on update).
func publishDeviceDiscovery(mainClient, cloudClient mqtt.Client, ownInstallation string, deviceID string, device TDevice, store *TLiveDeviceInfoStore, hostNameToDeviceID map[string]string, publisher *TDiscoveryPublisher, prefix string) (int, error) {
	live := store.Snapshot(deviceID)
	viaDeviceID := hostNameToDeviceID[live["via_device"]]
	plan := deviceRoutingPlan(device, cloudClient != nil)

	published := 0
	for _, cfg := range buildDiscoveryConfigs(deviceID, device, live, viaDeviceID, prefix, ownInstallation) {
		if plan.PublishMain {
			data, err := json.Marshal(cfg.Payload)
			if err != nil {
				return published, fmt.Errorf("marshalling discovery payload for %s: %w", cfg.Topic, err)
			}
			if err := publisher.Publish(mainClient, "main", cfg.Topic, data); err != nil {
				return published, err
			}
			published++
		}
		if plan.PublishCloud {
			// Cloud topic is INDEPENDENT of cfg.Topic (the local one) -- keyed by
			// exportStableID(ownInstallation, deviceID, cfg.Capability), the same stable-id scheme
			// discoveryhassbridge.go's own cloud export already uses, so a hosts-kind capability's
			// cloud catalogue entry can be matched by cross-house import
			// (discoveryimport.go's matchImportedCapabilityByStableID) too. Added 2026-09-07;
			// previously this was ownInstallation+"/"+cfg.Topic, the SAME id the local config uses,
			// which was NOT independent of this house's own local naming the way the hassbridge
			// side already was fixed to be.
			//
			// cloudSafePayload (added 2026-09-08) strips suggested_area before this crosses the
			// cloud broker -- see TDiscoveryDevice.withoutSuggestedArea's own doc comment. Marshalled
			// separately from the local payload above (this used to reuse the exact same bytes),
			// since the two payloads can now genuinely differ.
			cloudData, err := json.Marshal(cloudSafePayload(cfg.Payload))
			if err != nil {
				return published, fmt.Errorf("marshalling cloud discovery payload for %s: %w", cfg.Topic, err)
			}
			stableID := exportStableID(ownInstallation, deviceID, cfg.Capability)
			qualifiedTopic := ownInstallation + "/" + discoveryTopic(prefix, cfg.Component, stableID)
			if err := publisher.Publish(cloudClient, "cloud_coordinator", qualifiedTopic, cloudData); err != nil {
				return published, err
			}
			published++
		}
	}
	return published, nil
}

// publishDiscovery publishes every device's discovery configs once, through publisher.
// cloudClient is nil when this house has no cloud broker profile configured.
func publishDiscovery(mainClient, cloudClient mqtt.Client, ownInstallation string, devicesFile TDevicesFile, store *TLiveDeviceInfoStore, hostNameToDeviceID map[string]string, publisher *TDiscoveryPublisher) error {
	published := 0
	prefix := devicesFile.conceptualPrefix()
	for id, device := range devicesFile.Devices {
		if device.Cloud && cloudClient == nil {
			fmt.Printf("[discovery] device %q declares \"cloud\" routing but no cloud broker is configured; skipping\n", id)
			continue
		}
		n, err := publishDeviceDiscovery(mainClient, cloudClient, ownInstallation, id, device, store, hostNameToDeviceID, publisher, prefix)
		if err != nil {
			return err
		}
		published += n
	}
	fmt.Printf("published %d discovery config(s)\n", published)
	return nil
}

// subscribeDebugLogger subscribes to every "hosts/..." topic and logs each message received,
// as a sanity aid that device traffic is actually flowing -- matches the coordinator's
// existing "read + validate + summarize" spirit. Takes no further action on messages; local
// infrastructure coordination (Architecture.md §6.1 role 1) isn't built yet. label prefixes each
// log line (e.g. "main", "cloud_coordinator") so traffic from a second broker connection
// (mqtt_relay.go) is distinguishable in the log.
func subscribeDebugLogger(client mqtt.Client, label string) error {
	handler := func(_ mqtt.Client, msg mqtt.Message) {
		fmt.Printf("[hosts:%s] %s = %s\n", label, msg.Topic(), string(msg.Payload()))
	}
	token := subscribe(client, "hosts/#", 0, handler)
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		if err := token.Error(); err != nil {
			return fmt.Errorf("subscribing to hosts/# on %s broker: %w", label, err)
		}
		return fmt.Errorf("subscribing to hosts/# on %s broker: timed out", label)
	}
	return nil
}

// subscribeDeviceInfo subscribes on mainClient to every device's ".../device/state" topic
// (wildcarded as "hosts/+/device/state" -- one subscription, dispatched via topicToDeviceID
// rather than one subscription per device) and, on each message: best-effort JSON-decodes it
// into map[string]string (logs and skips on a parse error -- a malformed report shouldn't take
// the coordinator down), looks up which device it's about (skips silently if the topic isn't a
// known device's -- devices.yaml may have changed since this message was retained), updates
// store, and republishes just that device's discovery configs with the refreshed data. This is
// what makes a live-reported field actually reach Home Assistant without a coordinator restart.
//
// This always subscribes on mainClient alone, even for a "cloud"+"local" or imported device --
// mqtt_relay.go's relayCloudDevice already bridges that device's raw device/state message onto
// this same bare "hosts/<host>/device/state" topic on main, so it arrives here exactly as if
// reported locally; cloudClient/ownInstallation are only needed to route the resulting discovery
// republish correctly (deviceRoutingPlan, mqtt_relay.go) -- a "cloud"-only (not "local") device
// never reaches this subscription at all (nothing relays it onto main), see
// subscribeCloudOnlyDeviceInfo for that case instead.
func subscribeDeviceInfo(mainClient, cloudClient mqtt.Client, ownInstallation string, devicesFile TDevicesFile, store *TLiveDeviceInfoStore, topicToDeviceID, hostNameToDeviceID map[string]string, publisher *TDiscoveryPublisher) error {
	prefix := devicesFile.conceptualPrefix()
	handler := func(_ mqtt.Client, msg mqtt.Message) {
		var fields map[string]string
		if err := json.Unmarshal(msg.Payload(), &fields); err != nil {
			fmt.Printf("[device-info] %s: cannot parse payload: %v\n", msg.Topic(), err)
			return
		}
		deviceID, known := topicToDeviceID[msg.Topic()]
		if !known {
			return
		}
		store.Update(deviceID, fields)

		device, known := devicesFile.Devices[deviceID]
		if !known {
			return
		}
		if _, err := publishDeviceDiscovery(mainClient, cloudClient, ownInstallation, deviceID, device, store, hostNameToDeviceID, publisher, prefix); err != nil {
			fmt.Printf("[device-info] %s: republishing discovery: %v\n", deviceID, err)
		}
	}
	token := subscribe(mainClient, "hosts/+/device/state", 0, handler)
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		if err := token.Error(); err != nil {
			return fmt.Errorf("subscribing to hosts/+/device/state: %w", err)
		}
		return fmt.Errorf("subscribing to hosts/+/device/state: timed out")
	}
	return nil
}
