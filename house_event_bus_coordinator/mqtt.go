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
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// connectMQTT connects to secrets' broker, blocking until connected or failed. secrets.TLS
// selects "ssl://" over a plain "tcp://" broker URL, with an empty tls.Config -- meaning the
// system's own root CA pool, sufficient for a publicly-trusted certificate (e.g. Let's Encrypt,
// the cloud broker's own setup) without needing a custom CA file; nothing here supports a
// privately-issued/self-signed cloud broker certificate.
func connectMQTT(secrets TMQTTBrokerSecrets, clientID string) (mqtt.Client, error) {
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
	opts.SetConnectTimeout(10 * time.Second)

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
	for _, cfg := range buildDiscoveryConfigs(deviceID, device, live, viaDeviceID, prefix) {
		data, err := json.Marshal(cfg.Payload)
		if err != nil {
			return published, fmt.Errorf("marshalling discovery payload for %s: %w", cfg.Topic, err)
		}
		if plan.PublishMain {
			if err := publisher.Publish(mainClient, "main", cfg.Topic, data); err != nil {
				return published, err
			}
			published++
		}
		if plan.PublishCloud {
			qualifiedTopic := ownInstallation + "/" + cfg.Topic
			if err := publisher.Publish(cloudClient, "cloud_coordinator", qualifiedTopic, data); err != nil {
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
	token := client.Subscribe("hosts/#", 0, handler)
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
	token := mainClient.Subscribe("hosts/+/device/state", 0, handler)
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		if err := token.Error(); err != nil {
			return fmt.Errorf("subscribing to hosts/+/device/state: %w", err)
		}
		return fmt.Errorf("subscribing to hosts/+/device/state: timed out")
	}
	return nil
}
