/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: RemoteInstanceMetaControl
 *
 * PROJECT.md item 2 (deploy-trigger reload mechanism, extended 2026-09-05 to add "restart"
 * alongside "reload"): one pair of automations per declared instance, unconditionally generated
 * exactly like entityExistenceInquiryAutomationBody (remote_instance_entity_existence.go) -- no
 * Physical.def grammar needed, since every instance gets both regardless of what it bridges. Each
 * reacts to an MQTT command on its own "meta/<action>/<name>/request" topic; the deploy script (or
 * a person, via the coordinator's own discovery-published button entities,
 * house_event_bus_coordinator/meta_reload_restart.go) publishes there directly for a single-
 * instance target, or via the coordinator's own installation-/"all"-scoped fan-out for a broader
 * one -- this file only ever needs to know its own instance name, never which scope was pressed.
 *
 * "reload" calls homeassistant.reload_all -- confirmed live present on Vienna (HA Core 2026.9.2,
 * 2026-09-17) and, per HA's own service description, the actual backend of the UI's "Reload all
 * YAML configuration" quick action: it iterates HA's own internal reload-hook registry
 * (helpers/reload.py's async_setup_reload_service, the same registry every "<domain>.reload"
 * service is itself registered into), so it can never miss a domain and never needs a
 * hand-maintained list of "<domain>.reload" service names here. "restart" is a single
 * homeassistant.restart call -- heavier, for changes reload alone can't reach (new custom
 * component, structural config changes).
 *
 * REPLACES a much more complex prior design (SEVERAL independent automations, one per
 * "<domain>.reload" service, sharing one trigger topic) that existed ONLY to work around
 * reload_all's own absence being unknown at the time: HA's script engine RE-RAISES ServiceNotFound
 * regardless of continue_on_error, so a single automation hand-calling every "<domain>.reload"
 * aborted entirely at the first domain not configured on a given instance (real bug found live
 * 2026-09-15, protocols-server-2 missing "template"), and automation.reload itself needed a
 * separate, delayed automation since it reloads and recreates every automation entity including
 * whichever one is calling it, self-cancelling mid-run (real bug found live 2026-09-08). Calling
 * reload_all instead sidesteps this whole problem class structurally: it's HA's own first-party
 * orchestration of exactly this sequencing, not a hand-rolled reimplementation of it.
 *
 * Deliberately fire-and-forget, no reply topic: considered and set aside as overkill for now
 * (2026-09-05 discussion) -- unlike the entity-existence inquiry's request/reply shape, there is no
 * per-press consumer waiting on a specific answer.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 17.09.2026
 *
 */

package main

import "strings"

// metaReloadRequestTopic and metaRestartRequestTopic must stay in sync with the coordinator's own
// metaActionTopic (house_event_bus_coordinator/meta_reload_restart.go) -- both sides build the
// exact same "meta/<action>/<name>/request" string independently, since this file has no build
// dependency on the coordinator module.
func metaReloadRequestTopic(name string) string  { return "meta/reload/" + name + "/request" }
func metaRestartRequestTopic(name string) string { return "meta/restart/" + name + "/request" }

// metaReloadAutomationBody returns the single "reload" automation for a named instance, triggered
// by an MQTT command on metaReloadRequestTopic(name) (payload ignored, a button press carries no
// meaningful content, only the topic identifies the target). One call to homeassistant.reload_all
// -- see this file's own header comment for why that's sufficient and preferable to hand-listing
// every "<domain>.reload" service.
func metaReloadAutomationBody(name string) string {
	var sb strings.Builder
	sb.WriteString("- alias: \"Coordinator meta: reload\"\n")
	sb.WriteString("  id: automation.coordinator_meta_reload_" + name + "\n")
	sb.WriteString("  mode: single\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: mqtt\n")
	sb.WriteString("    topic: \"" + metaReloadRequestTopic(name) + "\"\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: homeassistant.reload_all\n")
	return sb.String()
}

// metaRestartAutomationBody returns the "restart this instance" automation for a named instance --
// triggered by an MQTT command on metaRestartRequestTopic(name).
func metaRestartAutomationBody(name string) string {
	var sb strings.Builder
	sb.WriteString("- alias: \"Coordinator meta: restart\"\n")
	sb.WriteString("  id: automation.coordinator_meta_restart_" + name + "\n")
	sb.WriteString("  mode: single\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: mqtt\n")
	sb.WriteString("    topic: \"" + metaRestartRequestTopic(name) + "\"\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: homeassistant.restart\n")
	return sb.String()
}
