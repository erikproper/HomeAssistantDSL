/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: DiscoveryRelayQueue
 *
 * Real incident, 2026-09-14 (PROJECT.md item 7): discoverybridge.go's message handler used to call
 * publisher.Publish/RetireOne synchronously, directly inside the MQTT client's own message-dispatch
 * callback -- each one a blocking, ack-waiting network round trip (up to 10s). The initial subscribe
 * to a physical-prefix topic replays the ENTIRE currently-retained backlog at once; for a
 * mid-migration Zigbee2MQTT network that's easily 1000+ messages. Processing that burst
 * synchronously, one ack-wait after another, starved a LATER, entirely unrelated client.Subscribe
 * call (discoveryhassbridge.go's own "homeassistant_instances/+/bridge/+/state" subscription) of
 * its own ack within its own 10s timeout -- main() treats any subscribe failure as fatal, so this
 * crash-looped the whole coordinator on a ~20s cycle until the underlying retained backlog was
 * manually purged from the broker. Confirmed live: even a not-fatal fix that only stopped the
 * *extra* per-message work this session's passthrough feature had added (a status snapshot publish
 * per newly-seen leaf) did NOT stop the crash loop -- the pre-existing, synchronous relay Publish
 * calls alone were enough once the backlog was large enough. Also observed live: Home Assistant's
 * own MQTT client logged "No ACK from MQTT server in 10 seconds" during the same incident window --
 * so simply moving this work off the message-dispatch goroutine isn't enough on its own either; a
 * burst of publishes fired back-to-back as fast as possible still saturates the shared
 * broker/network right when a remote instance's own MQTT client is trying to process that same
 * backlog. This queue is deliberately throttled, not just decoupled, for that reason.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 14.09.2026
 *
 */

package main

import (
	"fmt"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// discoveryRelayAction is which TDiscoveryPublisher method a queued job performs.
type discoveryRelayAction int

const (
	discoveryRelayPublish discoveryRelayAction = iota
	discoveryRelayRetire
)

func (a discoveryRelayAction) String() string {
	if a == discoveryRelayRetire {
		return "retire"
	}
	return "publish"
}

// TDiscoveryRelayJob is one outbound publish or retire discoverybridge.go's message handler
// decided to make, queued rather than performed directly -- see this component's own doc comment.
type TDiscoveryRelayJob struct {
	Action      discoveryRelayAction
	Client      mqtt.Client
	Broker      string
	Topic       string
	Payload     []byte // only meaningful for discoveryRelayPublish
	Passthrough bool   // only meaningful for discoveryRelayPublish -- see TDiscoveryPublisher.PublishPassthrough
}

// discoveryRelayQueueCapacity is generous enough to soak a full realistic migration backlog
// (Junglinster's own incident involved close to 1000 messages) without ever blocking the sender.
const discoveryRelayQueueCapacity = 4096

// discoveryRelayDefaultThrottle is the minimum spacing between two consecutive processed relay
// jobs in production -- deliberately paced, not just off the callback path (see component doc
// comment). At this rate a 1000-job backlog drains in under two minutes, which is more than
// acceptable for a one-time migration catch-up nothing live depends on completing instantly.
const discoveryRelayDefaultThrottle = 100 * time.Millisecond

// newDiscoveryRelayQueue creates a buffered job channel and starts its own background worker
// goroutine draining it at throttle's own pace, for the lifetime of the process (the coordinator
// never shuts this down cleanly, matching every other background piece in this codebase). Returns
// the channel for callers to enqueue onto (enqueueDiscoveryRelayJob). throttle is an explicit
// parameter rather than a package-level var deliberately: this goroutine never stops, so a test
// that mutated a shared var after starting one would race with it reading that same var on every
// iteration for the rest of the test binary's process lifetime -- passing the pace in once, at
// creation, needs no shared mutable state at all.
func newDiscoveryRelayQueue(publisher *TDiscoveryPublisher, throttle time.Duration) chan TDiscoveryRelayJob {
	jobs := make(chan TDiscoveryRelayJob, discoveryRelayQueueCapacity)
	go func() {
		for job := range jobs {
			processDiscoveryRelayJob(job, publisher)
			time.Sleep(throttle)
		}
	}()
	return jobs
}

// processDiscoveryRelayJob performs job's own action against publisher -- shared by the real
// background worker (newDiscoveryRelayQueue) and tests, which drain a job channel synchronously
// instead of waiting on discoveryRelayThrottle's real timing.
func processDiscoveryRelayJob(job TDiscoveryRelayJob, publisher *TDiscoveryPublisher) {
	switch job.Action {
	case discoveryRelayPublish:
		publish := publisher.Publish
		if job.Passthrough {
			publish = publisher.PublishPassthrough
		}
		if err := publish(job.Client, job.Broker, job.Topic, job.Payload); err != nil {
			fmt.Printf("[discovery-bridge] queued publish %s: %v\n", job.Topic, err)
		}
	case discoveryRelayRetire:
		publisher.RetireOne(job.Client, job.Broker, job.Topic)
	}
}

// enqueueDiscoveryRelayJob is a non-blocking send: a full queue drops the job (logged) rather than
// blocking the MQTT message handler that's trying to enqueue it -- the exact synchronous-blocking
// failure mode this whole mechanism exists to prevent. Harmless when it happens: MQTT's own
// retained-message replay means a dropped relay job's source message simply arrives again on the
// next coordinator restart or reconnect.
func enqueueDiscoveryRelayJob(jobs chan<- TDiscoveryRelayJob, job TDiscoveryRelayJob) {
	select {
	case jobs <- job:
	default:
		fmt.Printf("[discovery-bridge] relay queue full, dropping %s %s\n", job.Action, job.Topic)
	}
}

// drainDiscoveryRelayQueue processes every job currently sitting in jobs, synchronously, without
// waiting on discoveryRelayThrottle's own pacing -- test-only helper, mirroring what the real
// background worker eventually does, just deterministically and immediately.
func drainDiscoveryRelayQueue(jobs chan TDiscoveryRelayJob, publisher *TDiscoveryPublisher) {
	for {
		select {
		case job := <-jobs:
			processDiscoveryRelayJob(job, publisher)
		default:
			return
		}
	}
}
