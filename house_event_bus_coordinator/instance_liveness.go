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
 * Since 2026-11-01 (PROJECT.md 25c) one TLivenessSpec of the generic liveness.go. Not signs of life
 * any more: what the coordinator itself and main send there -- the existence inquiry (every
 * minute, so ha2mqtt never got quiet and a hang went unnoticed), its own existence report, and
 * main's entity commands.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 01.11.2026
 *
 */

package main

import (
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

// instanceLivenessSpec is a bridged instance's ping-pong as a liveness spec.
func instanceLivenessSpec(instance string) TLivenessSpec {
	base := "homeassistant_instances/" + instance
	return TLivenessSpec{
		Name:          "instance " + instance,
		Traffic:       base + "/#",
		Ignore:        []string{base + "/inquire", base + "/existence/#", base + "/bridge/+/command"},
		Quiet:         instanceQuietPeriod,
		Ask:           instancePingTopic(instance),
		AskPayload:    "ping",
		Answer:        instancePingTopic(instance),
		AnswerPayload: "pong",
		Within:        instancePongTimeout,
		Verdict:       instanceStatusTopic(instance),
		Online:        instanceOnline,
		Offline:       instanceOffline,
		Misses:        1,
	}
}

// startInstanceLiveness starts tracking every instance but main (the coordinator's own house's
// instance, local: nothing to bridge).
func startInstanceLiveness(client mqtt.Client, instances []string) error {
	for _, instance := range instances {
		if instance == "main" {
			continue
		}
		if err := newLiveness(client, instanceLivenessSpec(instance)).start(); err != nil {
			return err
		}
	}
	return nil
}
