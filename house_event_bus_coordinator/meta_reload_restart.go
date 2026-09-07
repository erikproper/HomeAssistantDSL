/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: MetaReloadRestart
 *
 * PROJECT.md item 2 (deploy-trigger reload mechanism, extended 2026-09-05 to add "restart"
 * alongside "reload", plus dashboard-pressable buttons and cross-house "all"). Three scopes for
 * each of "reload"/"restart":
 *
 *   - a single real instance ("main", "protocols-server-2", ...) -- metaActionTopic(action, name)
 *     is handled directly by that instance's own generator-authored automation
 *     (homeassistant/remote_instance_meta_control.go); this file never touches that topic at all,
 *     it's a pure instance-to-instance MQTT command with no coordinator relay in between.
 *   - this house's own installation (e.g. "junglinster") -- fans out to every one of this house's
 *     own real instances' own topics, local broker only. Deliberately never touches the cloud
 *     broker: "installation" scope means *this* house alone, that's the whole distinction from
 *     "all" below.
 *   - "all", across every house sharing the cloud broker -- a local press fans out locally AND
 *     cross-posts once to the cloud broker's own "all" topic; an arrival FROM the cloud broker
 *     fans out locally only, never republishing anywhere. This asymmetry is deliberate and
 *     mirrors discoveryhassbridge.go's own self-import loop-guard (the real feedback-loop
 *     incident found live 2026-09-05): a naive "relay whatever arrives, on either broker, onto the
 *     other" would let each house's own cross-post bounce back off every sibling house forever.
 *
 * Buttons are discovery-published (not generator-authored, unlike the per-instance automations
 * above) since only the coordinator knows, at runtime, this house's own installation name and its
 * current set of real instances -- mirrors discover_entity_input.go's own "coordinator owns this
 * meta control" precedent.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 05.09.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// metaActionTopic must stay in sync with the generator's own metaReloadRequestTopic/
// metaRestartRequestTopic (homeassistant/remote_instance_meta_control.go) for the real-instance
// case -- both sides build the exact same string independently, no shared Go dependency between
// the two modules.
func metaActionTopic(action, target string) string {
	return "meta/" + action + "/" + target + "/request"
}

// metaActions is every scope this file handles, applied identically to "reload" and "restart".
var metaActions = []string{"reload", "restart"}

// expectedMetaReloadRestartTopics is every discovery topic publishMetaReloadRestartButtons
// produces -- must be added to main.go's own expectedTopics set (the same way
// discoverEntityStableID's own topic already is, discover_entity_input.go's doc comment) or
// watchForOrphanedDiscoveryTopics retires every one of these buttons moments after they're first
// published, since they're coordinator-owned and not derived from any devices/discovery/hassbridge
// file this house's own generator output already accounts for. Real bug found live 2026-09-05:
// without this, every meta button vanished from the broker within the same startup pass that
// published it.
func expectedMetaReloadRestartTopics(conceptualPrefix, installation string, instances []string) map[string]bool {
	expected := map[string]bool{}
	if installation == "" {
		return expected
	}
	targets := append([]string{}, instances...)
	targets = append(targets, installation, "all")
	for _, action := range metaActions {
		for _, target := range targets {
			stableID := "meta_" + action + "_" + sanitizeTopicSegment(target)
			expected[discoveryTopic(conceptualPrefix, "button", stableID)] = true
		}
	}
	return expected
}

// publishMetaReloadRestartButtons discovery-publishes one MQTT button per (action, target) pair --
// reload/restart crossed with every one of this house's own real instances, this installation
// itself, and "all" -- e.g. a two-real-instance installation gets (2 instances + 1 installation +
// 1 "all") * 2 actions = 8 buttons.
func publishMetaReloadRestartButtons(client mqtt.Client, conceptualPrefix, installation string, instances []string) error {
	if installation == "" {
		return nil
	}
	targets := append([]string{}, instances...)
	targets = append(targets, installation, "all")
	for _, action := range metaActions {
		icon := "mdi:reload"
		if action == "restart" {
			icon = "mdi:restart"
		}
		for _, target := range targets {
			stableID := "meta_" + action + "_" + sanitizeTopicSegment(target)
			topic := discoveryTopic(conceptualPrefix, "button", stableID)
			body := map[string]interface{}{
				"unique_id":       stableID,
				"object_id":       stableID,
				"name":            "meta/" + action + "/" + target,
				"icon":            icon,
				"command_topic":   metaActionTopic(action, target),
				"payload_press":   "PRESS",
				"entity_category": "config",
			}
			data, err := json.Marshal(body)
			if err != nil {
				return fmt.Errorf("marshalling meta %s button for %q: %w", action, target, err)
			}
			if err := publishRetained(client, topic, data); err != nil {
				return err
			}
		}
	}
	return nil
}

// publishMetaAction sends one fire-and-forget (non-retained) command to target's own
// metaActionTopic(action, target) on client -- the shared low-level primitive every fan-out below
// builds on, mirroring entity_existence.go's own publishInquiry.
func publishMetaAction(client mqtt.Client, action, target string) {
	topic := metaActionTopic(action, target)
	token := client.Publish(topic, 0, false, []byte("PRESS"))
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		if err := token.Error(); err != nil {
			fmt.Printf("[meta] %s %s: %v\n", action, target, err)
		} else {
			fmt.Printf("[meta] %s %s: timed out\n", action, target)
		}
		return
	}
	fmt.Printf("[meta] %s: sent to %s\n", action, target)
}

// fanOutMetaAction publishes action to every one of instances' own topics on client, local broker
// only -- shared by the installation-scoped handler and both "all" handlers (local press and cloud
// arrival) below.
func fanOutMetaAction(client mqtt.Client, action string, instances []string) {
	for _, instance := range instances {
		publishMetaAction(client, action, instance)
	}
}

// subscribeMetaFanOut wires up the installation- and "all"-scoped handlers for every action in
// metaActions -- no-op per action/scope if there is nothing to fan out to (installation == "" or
// instances is empty), and the cloud "all" leg is skipped entirely when cloudClient is nil (no
// cloud broker configured), same graceful-degradation convention every other cloud-crossing
// mechanism in this codebase already follows.
func subscribeMetaFanOut(client, cloudClient mqtt.Client, installation string, instances []string) error {
	if installation == "" || len(instances) == 0 {
		return nil
	}
	for _, action := range metaActions {
		action := action // capture for the closures below

		installationTopic := metaActionTopic(action, installation)
		installationHandler := func(_ mqtt.Client, _ mqtt.Message) {
			fmt.Printf("[meta] %s: installation %q pressed, fanning out to %v\n", action, installation, instances)
			fanOutMetaAction(client, action, instances)
		}
		if err := subscribeMetaTopic(client, installationTopic, installationHandler); err != nil {
			return err
		}

		allTopic := metaActionTopic(action, "all")
		localAllHandler := func(_ mqtt.Client, _ mqtt.Message) {
			fmt.Printf("[meta] %s: \"all\" pressed locally, fanning out to %v and cross-posting to cloud\n", action, instances)
			fanOutMetaAction(client, action, instances)
			if cloudClient != nil {
				publishMetaAction(cloudClient, action, "all")
			}
		}
		if err := subscribeMetaTopic(client, allTopic, localAllHandler); err != nil {
			return err
		}

		if cloudClient != nil {
			cloudAllHandler := func(_ mqtt.Client, _ mqtt.Message) {
				// Never republish onto allTopic (local) or back onto cloud here -- this message
				// already came from the cloud broker's own "all" topic; re-publishing it anywhere
				// is exactly the self-import class of feedback loop found live 2026-09-05
				// (discoveryhassbridge.go's own doc comment). Fan out locally only.
				fmt.Printf("[meta] %s: \"all\" arrived from cloud, fanning out locally to %v\n", action, instances)
				fanOutMetaAction(client, action, instances)
			}
			if err := subscribeMetaTopic(cloudClient, allTopic, cloudAllHandler); err != nil {
				return err
			}
		}
	}
	return nil
}

// subscribeMetaTopic is the shared subscribe-with-timeout boilerplate every handler above uses.
func subscribeMetaTopic(client mqtt.Client, topic string, handler mqtt.MessageHandler) error {
	token := client.Subscribe(topic, 0, handler)
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		if err := token.Error(); err != nil {
			return fmt.Errorf("subscribing to %s: %w", topic, err)
		}
		return fmt.Errorf("subscribing to %s: timed out", topic)
	}
	return nil
}
