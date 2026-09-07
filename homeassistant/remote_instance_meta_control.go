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
 * "reload" calls every YAML-backed domain's own "<domain>.reload" service plus
 * homeassistant.reload_core_config -- the same set the UI's own "Reload all YAML configuration"
 * quick action calls, bundled into one automation rather than one topic per domain (considered and
 * set aside as overkill, 2026-09-05). continue_on_error on every step: an instance that doesn't
 * have e.g. the "group" integration configured at all must not let that one failing reload abort
 * the rest of the list. "restart" is a single homeassistant.restart call -- heavier, for changes
 * reload alone can't reach (new custom component, structural config changes).
 *
 * Deliberately fire-and-forget, no reply topic: considered and set aside as overkill for now (same
 * 2026-09-05 discussion) -- unlike the entity-existence inquiry's request/reply shape, there is no
 * per-press consumer waiting on a specific answer.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 05.09.2026
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

// metaReloadServices is every ALWAYS-registered core reload service this automation calls
// unconditionally, in a fixed order -- automation/script are core (default_config, never
// absent), homeassistant.reload_core_config is core config (customize.yaml). Called with a plain
// static "service:" string since these are never at risk of HA's own static schema validation
// flagging them as an unknown action.
var metaReloadServices = []string{
	"automation.reload",
	"script.reload",
}

// metaOptionalReloadServices is every OTHER YAML-backed domain's own "<domain>.reload" service --
// mirrors HA's own "Reload all YAML configuration" quick action, but each of these is only
// actually REGISTERED when that domain has at least one entity/use configured on the target
// instance (e.g. no YAML template entities anywhere -> no template.reload service at all). Real
// bug found live 2026-09-06: HA statically validates a literal "service: template.reload" at
// automation-LOAD time, not run time, so an instance without any template entities got a
// PERSISTENT "unknown action" Repair issue that continue_on_error can't help with at all (that
// only guards a runtime failure, never a load-time validation error). Fixed by wrapping the
// service name in a trivial Jinja template ("{{ '<service>' }}") -- HA can't statically validate
// a templated service name, so it defers the check to execution time, where continue_on_error
// then correctly skips a genuinely-absent one instead of blocking the whole automation from
// loading.
var metaOptionalReloadServices = []string{
	"scene.reload",
	"template.reload",
	"group.reload",
	"input_boolean.reload",
	"input_datetime.reload",
	"input_number.reload",
	"input_select.reload",
	"input_text.reload",
}

// metaReloadAutomationBody returns the "reload every YAML-backed domain" automation for a named
// instance -- triggered by an MQTT command on metaReloadRequestTopic(name), payload ignored (a
// button press carries no meaningful content, only the topic identifies the target).
func metaReloadAutomationBody(name string) string {
	var sb strings.Builder
	sb.WriteString("- alias: \"Coordinator meta: reload\"\n")
	sb.WriteString("  id: automation.coordinator_meta_reload_" + name + "\n")
	sb.WriteString("  mode: single\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: mqtt\n")
	sb.WriteString("    topic: \"" + metaReloadRequestTopic(name) + "\"\n")
	sb.WriteString("  action:\n")
	for _, service := range metaReloadServices {
		sb.WriteString("  - service: " + service + "\n")
		sb.WriteString("    continue_on_error: true\n")
	}
	for _, service := range metaOptionalReloadServices {
		sb.WriteString("  - service: \"{{ '" + service + "' }}\"\n")
		sb.WriteString("    continue_on_error: true\n")
	}
	sb.WriteString("  - service: homeassistant.reload_core_config\n")
	sb.WriteString("    continue_on_error: true\n")
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
