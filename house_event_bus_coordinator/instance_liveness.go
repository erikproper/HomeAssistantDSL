/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: InstanceLiveness
 *
 * PROJECT.md 26 (Erik's ping-pong, 2026-10-28): whether a bridged Home Assistant instance (ha2mqtt,
 * later fhem2mqtt and the like) is up, kept retained on homeassistant_instances/<instance>/status
 * ("online"/"offline") -- the topic the instance's integration device's node reads
 * (homeassistant/integration_mqtt.go). Every message from the instance (anything under
 * homeassistant_instances/<instance>/) is a sign of life. Only after a quiet spell
 * (instanceQuietPeriod) does the coordinator ask: "ping" on homeassistant_instances/<instance>/ping,
 * which the instance's generated automation answers with "pong" on the same topic; no pong within
 * instancePongTimeout means it is down. So nothing is sent while the instance reports anyway, and
 * the answer proves its automations and MQTT work, end to end. At start the coordinator pings at
 * once, so the status is known right away.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 28.10.2026
 *
 */

package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	instanceQuietPeriod = 5 * time.Minute
	instancePongTimeout = 30 * time.Second
	instanceOnline      = "online"
	instanceOffline     = "offline"
)

func instanceStatusTopic(instance string) string {
	return "homeassistant_instances/" + instance + "/status"
}

func instancePingTopic(instance string) string {
	return "homeassistant_instances/" + instance + "/ping"
}

// TInstanceLiveness tracks one bridged instance.
type TInstanceLiveness struct {
	mu        sync.Mutex
	client    mqtt.Client
	instance  string
	status    string
	quiet     *time.Timer
	pongWait  *time.Timer
	quietTime time.Duration
	pongTime  time.Duration
}

// startInstanceLiveness starts tracking every instance but main (the coordinator's own house's
// instance, local: nothing to bridge).
func startInstanceLiveness(client mqtt.Client, instances []string) error {
	for _, instance := range instances {
		if instance == "main" {
			continue
		}
		liveness := &TInstanceLiveness{client: client, instance: instance, quietTime: instanceQuietPeriod, pongTime: instancePongTimeout}
		if err := liveness.start(); err != nil {
			return err
		}
	}
	return nil
}

func (l *TInstanceLiveness) start() error {
	filter := "homeassistant_instances/" + l.instance + "/#"
	token := subscribe(l.client, filter, 0, func(_ mqtt.Client, msg mqtt.Message) {
		l.observe(msg.Topic(), string(msg.Payload()), msg.Retained())
	})
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		return fmt.Errorf("subscribing to %s: %v", filter, token.Error())
	}
	l.ping()
	fmt.Printf("[instance-liveness] tracking %s\n", l.instance)
	return nil
}

// observe handles one message under the instance's topics: anything but the coordinator's own
// ping and status -- and a retained message, which says nothing about now -- is a sign of life.
func (l *TInstanceLiveness) observe(topic, payload string, retained bool) {
	if retained || topic == instanceStatusTopic(l.instance) || (topic == instancePingTopic(l.instance) && payload == "ping") {
		return
	}
	if !strings.HasPrefix(topic, "homeassistant_instances/"+l.instance+"/") {
		return
	}
	l.alive()
}

func (l *TInstanceLiveness) alive() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.pongWait != nil {
		l.pongWait.Stop()
		l.pongWait = nil
	}
	l.setStatus(instanceOnline)
	l.resetQuiet()
}

// resetQuiet (re)arms the quiet-spell timer; l.mu held.
func (l *TInstanceLiveness) resetQuiet() {
	if l.quiet != nil {
		l.quiet.Stop()
	}
	l.quiet = time.AfterFunc(l.quietTime, l.ping)
}

// ping asks the instance, and marks it offline if no pong comes in time.
func (l *TInstanceLiveness) ping() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.pongWait != nil {
		return // already waiting
	}
	l.client.Publish(instancePingTopic(l.instance), 0, false, "ping")
	l.pongWait = time.AfterFunc(l.pongTime, func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.pongWait = nil
		l.setStatus(instanceOffline)
		l.resetQuiet() // ask again after the next quiet spell
	})
}

// setStatus publishes status, retained, when it changed; l.mu held.
func (l *TInstanceLiveness) setStatus(status string) {
	if l.status == status {
		return
	}
	l.status = status
	fmt.Printf("[instance-liveness] %s: %s\n", l.instance, status)
	l.client.Publish(instanceStatusTopic(l.instance), 1, true, status)
}
