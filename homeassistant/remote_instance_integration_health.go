/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: RemoteInstanceIntegrationHealth
 *
 * PROJECT.md 29 (1) and (4), 2026-11-03: an integration that doesn't load -- one that needs
 * re-authentication (Junglinster's Overkiz that morning: a new local key, which only the
 * integration's own reconfigure showed), or that fails to set up after an update -- was only seen
 * by chance. Every instance now reports its config entries that need Erik (setup_error,
 * migration_error, failed_unload, not_loaded; a disabled one doesn't count) -- not setup_retry, a
 * device or service Home Assistant can't reach and keeps retrying (Vienna's TV switched off; an
 * outage, which the reload guards handle), Erik's choice: every few minutes, on
 * homeassistant.instances/<instance>/integrations, not retained. Main reads that as one problem
 * binary_sensor per instance (coordinatorYAMLEntityBodies), which goes unavailable when its
 * instance stops reporting. A Physical.def line names where main tells Erik:
 *
 *   notify problems via notify.mobile_app_eriks_iphone_15_pro;
 *
 * -- once a problem has lasted 15 minutes, and again when it is resolved. A template sees each
 * config entry through its entities (config_entry_id; 0.2 s over Junglinster main's 2,145
 * entities), so an entry without any entity isn't seen -- left so (Erik): the point is that the
 * entities actually used work, and an integration without entities isn't used.
 *
 * PROJECT.md 29 (2), the same day: the same automation reports the instance's own software updates
 * (an update.* entity that is on, but not a device's firmware -- an MQTT one, which Zigbee2MQTT
 * updates) on homeassistant.instances/<instance>/updates, and main shows them per instance. An
 * instance in a container (ha2mqtt) has no update entity for itself: its host's
 * update.containers --check reports the containers with a newer image, retained, on
 * hosts/<host>/containers/updates, and main shows those per host. Dashboard only: updating stays
 * Erik's (Erik).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 03.11.2026
 *
 */

package main

import (
	"regexp"
	"sort"
	"strings"
)

var problemsNotifyPattern = regexp.MustCompile(`^notify\s+problems\s+via\s+(notify\.[a-z0-9_]+)\s*;$`)

// integrationHealthEvery is how often (a time_pattern's minutes) an instance reports its config
// entries that aren't loaded; integrationHealthExpireAfter is how long main's binary_sensor keeps a
// report (three missed ones).
const (
	integrationHealthEvery        = "/5"
	integrationHealthExpireAfter  = 900
	integrationProblemNotifyAfter = "00:15:00"
)

// integrationNeedsAction are the config entry states reported: the ones Erik has to act on.
const integrationNeedsAction = "['setup_error', 'migration_error', 'failed_unload', 'not_loaded']"

// collectProblemsNotifyService returns the service of physicalContent's "notify problems via
// <service>;", "" for none.
func collectProblemsNotifyService(physicalContent string) string {
	for _, rawLine := range splitLines(physicalContent) {
		if m := problemsNotifyPattern.FindStringSubmatch(strings.TrimSpace(stripLineComment(rawLine))); m != nil {
			return m[1]
		}
	}
	return ""
}

// instanceIntegrationsTopic is where an instance reports its config entries that aren't loaded:
// {"items": ["<title> (<domain>): <state>", ...]}.
func instanceIntegrationsTopic(name string) string {
	return "homeassistant.instances/" + name + "/integrations"
}

// instanceIntegrationsEntityID is main's problem binary_sensor for instance's report.
func instanceIntegrationsEntityID(instance string) string {
	return "binary_sensor.infrastructural_integrations_" + entityIDPart(instance)
}

// instanceUpdatesTopic is where an instance reports its own software updates:
// {"items": ["<name>: <installed> -> <latest>", ...]}; hostContainerUpdatesTopic is where
// update.containers --check reports a host's containers with a newer image.
func instanceUpdatesTopic(name string) string      { return "homeassistant.instances/" + name + "/updates" }
func hostContainerUpdatesTopic(host string) string { return "hosts/" + host + "/containers/updates" }

// entityIDPart is name as part of an entity_id.
func entityIDPart(name string) string {
	return strings.NewReplacer(".", "_", "-", "_").Replace(name)
}

// integrationHealthAutomationBody returns the instance's report of its config entries that aren't
// loaded, every integrationHealthEvery minutes.
func integrationHealthAutomationBody(name string) string {
	updates := "{% set ns = namespace(items=[]) %}{% for s in states.update | selectattr('state', 'eq', 'on') %}{% set id = config_entry_id(s.entity_id) %}{% if not id or config_entry_attr(id, 'domain') != 'mqtt' %}{% set ns.items = ns.items + [s.name ~ ': ' ~ s.attributes.installed_version ~ ' -> ' ~ s.attributes.latest_version] %}{% endif %}{% endfor %}{{ {'items': ns.items} | to_json }}"
	problems := "{% set ns = namespace(ids=[], items=[]) %}{% for s in states %}{% set id = config_entry_id(s.entity_id) %}{% if id and id not in ns.ids %}{% set ns.ids = ns.ids + [id] %}{% set state = config_entry_attr(id, 'state') | string %}{% if state in " + integrationNeedsAction + " and not config_entry_attr(id, 'disabled_by') %}{% set ns.items = ns.items + [config_entry_attr(id, 'title') ~ ' (' ~ config_entry_attr(id, 'domain') ~ '): ' ~ state] %}{% endif %}{% endif %}{% endfor %}{{ {'items': ns.items} | to_json }}"
	var sb strings.Builder
	sb.WriteString("- alias: \"infrastructural/coordinator/integration_health\"\n")
	sb.WriteString("  id: automation.coordinator_integration_health_" + name + "\n")
	sb.WriteString("  mode: single\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: time_pattern\n")
	sb.WriteString("    minutes: \"" + integrationHealthEvery + "\"\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: mqtt.publish\n")
	sb.WriteString("    data:\n")
	sb.WriteString("      topic: \"" + instanceIntegrationsTopic(name) + "\"\n")
	sb.WriteString("      payload: \"" + problems + "\"\n")
	sb.WriteString("  - service: mqtt.publish\n")
	sb.WriteString("    data:\n")
	sb.WriteString("      topic: \"" + instanceUpdatesTopic(name) + "\"\n")
	sb.WriteString("      payload: \"" + updates + "\"\n")
	return sb.String()
}

// integrationProblemsNotifyAutomationBody returns main's notification, via service, of an
// instance's config entries that aren't loaded: when its list has stood integrationProblemNotifyAfter
// with problems in it, and when it is empty again after one that had. One tag per instance, so the
// resolution replaces the problem on the phone. The list is read with .get: a missing attribute
// (the entity just created by a deploy) read as ['items'] falls back to the attribute dict's own
// items method -- "<built-in method items ...>", a non-empty "list", and on 2026-11-03 a "loaded
// again" message for every instance, 15 minutes after the deploy.
func integrationProblemsNotifyAutomationBody(instances []string, service, installation string) string {
	var sb strings.Builder
	sb.WriteString("- alias: \"infrastructural/coordinator/integration_problems\"\n")
	sb.WriteString("  id: automation.coordinator_integration_problems\n")
	sb.WriteString("  mode: queued\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id:\n")
	for _, instance := range instances {
		sb.WriteString("    - " + instanceIntegrationsEntityID(instance) + "\n")
	}
	sb.WriteString("    attribute: items\n")
	sb.WriteString("    for: \"" + integrationProblemNotifyAfter + "\"\n")
	sb.WriteString("  variables:\n")
	sb.WriteString("    instance: \"{{ trigger.entity_id.split('_integrations_')[1] }}\"\n")
	sb.WriteString("    now_items: \"{{ trigger.to_state.attributes.get('items', []) if trigger.to_state else [] }}\"\n")
	sb.WriteString("    was_items: \"{{ trigger.from_state.attributes.get('items', []) if trigger.from_state else [] }}\"\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - choose:\n")
	sb.WriteString("    - conditions: \"{{ trigger.to_state.state == 'on' and now_items | count > 0 }}\"\n")
	sb.WriteString("      sequence:\n")
	sb.WriteString("      - service: " + service + "\n")
	sb.WriteString("        data:\n")
	sb.WriteString("          title: \"" + installation + "/{{ instance }}: integration not loaded\"\n")
	sb.WriteString("          message: \"{{ now_items | join('\\n') }}\"\n")
	sb.WriteString("          data:\n")
	sb.WriteString("            tag: \"integrations_" + installation + "_{{ instance }}\"\n")
	sb.WriteString("    - conditions: \"{{ trigger.to_state.state == 'off' and was_items | count > 0 }}\"\n")
	sb.WriteString("      sequence:\n")
	sb.WriteString("      - service: " + service + "\n")
	sb.WriteString("        data:\n")
	sb.WriteString("          title: \"" + installation + "/{{ instance }}: integrations loaded again\"\n")
	sb.WriteString("          message: \"{{ was_items | join('\\n') }}\"\n")
	sb.WriteString("          data:\n")
	sb.WriteString("            tag: \"integrations_" + installation + "_{{ instance }}\"\n")
	return sb.String()
}

// containerUpdatesExpireAfter is how long main keeps a host's container report (a day and a half:
// update.containers --check runs every 6 hours).
const containerUpdatesExpireAfter = 129600

// integrationHealthEntityBody is main's problem binary_sensor for instance's report.
func integrationHealthEntityBody(instance string) THostYAMLEntity {
	return itemsReportEntity("integrations", instance, "problem", "mdi:puzzle-remove", instanceIntegrationsTopic(instance), integrationHealthExpireAfter)
}

// instanceUpdatesEntityBody is main's update binary_sensor for instance's own software updates.
func instanceUpdatesEntityBody(instance string) THostYAMLEntity {
	return itemsReportEntity("updates", instance, "update", "mdi:package-up", instanceUpdatesTopic(instance), integrationHealthExpireAfter)
}

// containerUpdatesEntityBody is main's update binary_sensor for host's containers.
func containerUpdatesEntityBody(host string) THostYAMLEntity {
	return itemsReportEntity("container_updates", host, "update", "mdi:package-up", hostContainerUpdatesTopic(host), containerUpdatesExpireAfter)
}

// itemsReportEntity is main's binary_sensor "infrastructural/<kind>/<name>" for an {"items": [...]}
// report on topic: on while the list has items, the list as its attribute, unavailable after
// expireAfter seconds without a report.
func itemsReportEntity(kind, name, deviceClass, icon, topic string, expireAfter int) THostYAMLEntity {
	entityID := "binary_sensor.infrastructural_" + kind + "_" + entityIDPart(name)
	return THostYAMLEntity{Domain: "binary_sensor", EntityID: entityID, Body: map[string]interface{}{
		"unique_id":             kind + "_" + strings.ReplaceAll(name, ".", "_"),
		"default_entity_id":     entityID,
		"name":                  "infrastructural/" + kind + "/" + name,
		"device_class":          deviceClass,
		"icon":                  icon,
		"state_topic":           topic,
		"value_template":        "{{ 'ON' if value_json.get('items', []) | count > 0 else 'OFF' }}",
		"payload_on":            "ON",
		"payload_off":           "OFF",
		"json_attributes_topic": topic,
		"expire_after":          expireAfter,
		"entity_category":       "diagnostic",
	}}
}

// instanceReportEntities are main's report binary_sensors: per instance its integrations and its
// updates, per container host (containerHosts) its containers' updates.
func instanceReportEntities(instances, containerHosts []string) []THostYAMLEntity {
	var entities []THostYAMLEntity
	for _, instance := range instances {
		entities = append(entities, integrationHealthEntityBody(instance), instanceUpdatesEntityBody(instance))
	}
	for _, host := range containerHosts {
		entities = append(entities, containerUpdatesEntityBody(host))
	}
	return entities
}

// containerHostsOf returns the hosts running instances other than main (in containers: main is
// Home Assistant OS), sorted and each once.
func containerHostsOf(instances map[string]THomeAssistantInstance) []string {
	seen := map[string]bool{}
	var hosts []string
	for name, instance := range instances {
		if name != "main" && instance.Name != "" && !seen[instance.Name] {
			seen[instance.Name] = true
			hosts = append(hosts, instance.Name)
		}
	}
	sort.Strings(hosts)
	return hosts
}
