/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: RemoteInstanceReloadGuard
 *
 * PROJECT.md 23 (2026-10-28): some integrations on a bridged instance lose their connection now and
 * then (Junglinster's Netatmo on ha2mqtt: the indoor modules' entities go unavailable) and come back
 * with a reload of the integration. A bridged device declares it, in its own integration section
 * (PROJECT.md 29 (3), Erik's design):
 *
 *   reload integration when sensor.co2 is unavailable for 00:05:00;
 *
 * and the generator writes one guard automation on the instance: when an indicator's source entity
 * stays unavailable that long, the instance reloads that entity's integration
 * (homeassistant.reload_config_entry), notes it in the logbook, and then pauses 15 minutes, or
 * what "at most every HH:MM:SS" says (Netatmo: a day, as every reload counts against its API limit,
 * 2026-11-01) -- never a reload storm when the integration is really down. After a start or an
 * automation reload (a deploy), when an entity may already be unavailable without a change to
 * trigger on, it waits the longest guard time once and reloads each config entry still down once. A
 * "while <device> is up" guard only reloads while that device's node is on (a portable speaker
 * switched off triggers nothing).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 28.10.2026
 *
 */

package main

import (
	"fmt"
	"sort"
	"strings"
)

// reloadGuardPause is how long a guard waits after a reload before it may reload again.
const reloadGuardPause = "00:15:00"

// TResolvedReloadGuard is one guard trigger: the entity to watch, for how long, the pause after a
// reload ("" for reloadGuardPause), and the node entity that must be on meanwhile ("" for none).
type TResolvedReloadGuard struct {
	Entity      string
	For         string
	Pause       string
	WhileEntity string
}

// bridgeReloadGuards resolves the bridged devices' guards on instance to their source entities;
// problems reports a guard whose capability has no source entity there, or a "while" clause (a
// node of this house doesn't exist on another instance).
func bridgeReloadGuards(instance string, devices map[string]THassBridgeDevice) (guards []TResolvedReloadGuard, problems []string) {
	ids := make([]string, 0, len(devices))
	for id := range devices {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		for _, guard := range devices[id].ReloadGuards {
			if guard.Instance != instance {
				continue
			}
			source := devices[id].Capabilities[guard.Capability].Sources[instance]
			if source == "" || !strings.Contains(source, ".") || strings.Contains(source, " ") {
				problems = append(problems, fmt.Sprintf("%s: \"reload integration when %s ...\": no source entity of that capability on %s", id, guard.Capability, instance))
				continue
			}
			if guard.WhileUp != "" {
				problems = append(problems, fmt.Sprintf("%s: \"while %s is up\" is only for guards on main (its node doesn't exist on %s) -- ignored", id, guard.WhileUp, instance))
			}
			guards = append(guards, TResolvedReloadGuard{Entity: source, For: guard.For, Pause: guard.Pause})
		}
	}
	return guards, problems
}

// localReloadGuards resolves the local devices' guards (run on main) to their positioned entities
// and, for "while <device> is up", that device's node entity.
func localReloadGuards(devices map[string]TLocalDevice, admin *TAdministrationState) (guards []TResolvedReloadGuard, problems []string) {
	ids := make([]string, 0, len(devices))
	for id := range devices {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		for _, guard := range devices[id].ReloadGuards {
			entity := admin.DeviceConceptualLinks[id].AttributeEntityIDs[guard.Capability].EntityID
			if entity == "" {
				problems = append(problems, fmt.Sprintf("%s: \"reload integration when %s ...\": that capability isn't positioned", id, guard.Capability))
				continue
			}
			resolved := TResolvedReloadGuard{Entity: entity, For: guard.For, Pause: guard.Pause}
			if guard.WhileUp != "" {
				link := admin.DeviceConceptualLinks[guard.WhileUp]
				resolved.WhileEntity = link.NodeEntityID
				if resolved.WhileEntity == "" {
					resolved.WhileEntity = link.AttributeEntityIDs["node"].EntityID
				}
				if resolved.WhileEntity == "" {
					problems = append(problems, fmt.Sprintf("%s: \"while %s is up\": that device has no positioned node", id, guard.WhileUp))
					continue
				}
			}
			guards = append(guards, resolved)
		}
	}
	return guards, problems
}

// reloadGuardAutomationBody returns instance's guard automations for guards, and false when there
// are none: per pause ("at most every"), a guard automation and its start companion. The group with
// the shortest pause keeps the plain names, so an instance with one pause keeps its automation.
func reloadGuardAutomationBody(instance string, guards []TResolvedReloadGuard) (string, bool) {
	if len(guards) == 0 {
		return "", false
	}
	groups := map[int][]TResolvedReloadGuard{}
	for _, g := range guards {
		pause := g.Pause
		if pause == "" {
			pause = reloadGuardPause
		}
		seconds := durationSeconds(pause)
		groups[seconds] = append(groups[seconds], g)
	}
	pauses := make([]int, 0, len(groups))
	for seconds := range groups {
		pauses = append(pauses, seconds)
	}
	sort.Ints(pauses)
	var sb strings.Builder
	for i, seconds := range pauses {
		suffix := ""
		if i > 0 {
			suffix = fmt.Sprintf("_every_%ds", seconds)
		}
		writeReloadGuardGroup(&sb, instance, suffix, seconds, groups[seconds])
	}
	return sb.String(), true
}

// writeReloadGuardGroup writes one pause group's two automations. The guard reloads when a guarded
// entity stays unavailable, at most once per pause: a condition on its own last_triggered, which
// Home Assistant keeps across a restart and an automation reload (a deploy) -- unlike the delay it
// replaces (2026-11-01), which every deploy cut short. So the guard only runs when it reloads: after a
// start or an automation reload, an entity may already be unavailable, with no change to trigger on
// (exactly when Ring stayed away, 2026-10-25), and the start companion waits the longest guard time
// and then asks the guard, by an event, to reload the config entries still down -- each once: six
// Netatmo CO2 sensors are one integration, and reloading it six times in a row (found live
// 2026-11-01) only added to Netatmo's API limit ("429 Too Many Requests").
func writeReloadGuardGroup(sb *strings.Builder, instance, suffix string, pauseSeconds int, guards []TResolvedReloadGuard) {
	alias := "infrastructural/coordinator/reload_guard" + strings.ReplaceAll(suffix, "_", "/")
	id := "automation.coordinator_reload_guard_" + instance + suffix
	eventType := "coordinator_reload_guard" + suffix

	longest := guards[0].For
	var watched []string
	required := map[string]string{}
	for i, g := range guards {
		if g.For > longest {
			longest = g.For
		}
		watched = append(watched, fmt.Sprintf("{'entity': '%s', 'up': '%s'}", g.Entity, g.WhileEntity))
		if g.WhileEntity != "" {
			required[fmt.Sprintf("guard_%d", i)] = g.WhileEntity
		}
	}
	list := "[" + strings.Join(watched, ", ") + "]"
	// The guarded entities still unavailable (and, with a "while", whose device is up), reduced to
	// their config entries, each once.
	stillDown := "{% set ns = namespace(ids=[]) %}{% for w in " + list + " | selectattr('entity', 'is_state', 'unavailable') %}{% set id = config_entry_id(w.entity) %}{% if id and id not in ns.ids and (w.up == '' or is_state(w.up, 'on')) %}{% set ns.ids = ns.ids + [id] %}{% endif %}{% endfor %}{{ ns.ids }}"

	sb.WriteString("- alias: \"" + alias + "\"\n")
	sb.WriteString("  id: " + id + "\n")
	sb.WriteString("  mode: single\n")
	sb.WriteString("  trigger:\n")
	for i, g := range guards {
		sb.WriteString("  - platform: state\n")
		sb.WriteString(fmt.Sprintf("    id: guard_%d\n", i))
		sb.WriteString("    entity_id: " + g.Entity + "\n")
		sb.WriteString("    to: \"unavailable\"\n")
		sb.WriteString("    for: \"" + g.For + "\"\n")
	}
	sb.WriteString("  - platform: event\n")
	sb.WriteString("    id: start\n")
	sb.WriteString("    event_type: " + eventType + "\n")
	sb.WriteString("  condition:\n")
	sb.WriteString("  - condition: template\n")
	sb.WriteString(fmt.Sprintf("    value_template: \"{{ this.attributes.last_triggered is none or (now() - this.attributes.last_triggered).total_seconds() >= %d }}\"\n", pauseSeconds))
	if len(required) > 0 {
		// A "while <device> is up" guard only reloads while that device's node is on.
		ids := make([]string, 0, len(required))
		for id := range required {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		var pairs []string
		for _, id := range ids {
			pairs = append(pairs, fmt.Sprintf("'%s': '%s'", id, required[id]))
		}
		sb.WriteString("  - condition: template\n")
		sb.WriteString("    value_template: \"{% set up = {" + strings.Join(pairs, ", ") + "} %}{{ trigger.id not in up or is_state(up[trigger.id], 'on') }}\"\n")
	}
	sb.WriteString("  action:\n")
	sb.WriteString("  - choose:\n")
	sb.WriteString("    - conditions: \"{{ trigger.id == 'start' }}\"\n")
	sb.WriteString("      sequence:\n")
	sb.WriteString("      - repeat:\n")
	sb.WriteString("          for_each: \"{{ trigger.event.data.entries }}\"\n")
	sb.WriteString("          sequence:\n")
	sb.WriteString("          - service: logbook.log\n")
	sb.WriteString("            data:\n")
	sb.WriteString("              name: \"reload guard\"\n")
	sb.WriteString("              message: \"{{ config_entry_attr(repeat.item, 'title') }} ({{ config_entry_attr(repeat.item, 'domain') }}): guarded entities still unavailable after a start or reload -- reloading it\"\n")
	sb.WriteString("          - service: homeassistant.reload_config_entry\n")
	sb.WriteString("            data:\n")
	sb.WriteString("              entry_id: \"{{ repeat.item }}\"\n")
	sb.WriteString("    default:\n")
	sb.WriteString("    - service: logbook.log\n")
	sb.WriteString("      data:\n")
	sb.WriteString("        name: \"reload guard\"\n")
	sb.WriteString("        message: \"{{ trigger.entity_id }} unavailable for {{ trigger.for }} -- reloading its integration\"\n")
	sb.WriteString("    - service: homeassistant.reload_config_entry\n")
	sb.WriteString("      data:\n")
	sb.WriteString("        entry_id: \"{{ config_entry_id(trigger.entity_id) }}\"\n")

	sb.WriteString("- alias: \"" + alias + "/start\"\n")
	sb.WriteString("  id: " + id + "_start\n")
	sb.WriteString("  mode: single\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: homeassistant\n")
	sb.WriteString("    event: start\n")
	sb.WriteString("  - platform: event\n")
	sb.WriteString("    event_type: automation_reloaded\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - delay: \"" + longest + "\"\n")
	sb.WriteString("  - variables:\n")
	sb.WriteString("      entries: \"" + stillDown + "\"\n")
	sb.WriteString("  - condition: \"{{ entries | count > 0 }}\"\n")
	sb.WriteString("  - event: " + eventType + "\n")
	sb.WriteString("    event_data:\n")
	sb.WriteString("      entries: \"{{ entries }}\"\n")
}

// durationSeconds returns an "HH:MM:SS" duration (the guard pattern's, so well-formed) in seconds.
func durationSeconds(hms string) int {
	var h, m, s int
	fmt.Sscanf(hms, "%d:%d:%d", &h, &m, &s)
	return h*3600 + m*60 + s
}
