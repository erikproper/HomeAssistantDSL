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
 * (homeassistant.reload_config_entry), notes it in the logbook, and then waits 15 minutes before it
 * may reload again (mode: single) -- at most 4 reloads an hour, never a reload storm when the
 * integration is really down. After a start or an automation reload (a deploy), when an entity may
 * already be unavailable without a change to trigger on, it waits the longest guard time once and
 * reloads for every guarded entity still unavailable. A "while <device> is up" guard only reloads
 * while that device's node is on (a portable speaker switched off triggers nothing).
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

// TResolvedReloadGuard is one guard trigger: the entity to watch, for how long, and the node entity
// that must be on meanwhile ("" for none).
type TResolvedReloadGuard struct {
	Entity      string
	For         string
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
			guards = append(guards, TResolvedReloadGuard{Entity: source, For: guard.For})
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
			resolved := TResolvedReloadGuard{Entity: entity, For: guard.For}
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

// reloadGuardAutomationBody returns instance's guard automation for guards, and false when there
// are none.
func reloadGuardAutomationBody(instance string, guards []TResolvedReloadGuard) (string, bool) {
	if len(guards) == 0 {
		return "", false
	}
	var sb strings.Builder
	sb.WriteString("- alias: \"infrastructural/coordinator/reload_guard\"\n")
	sb.WriteString("  id: automation.coordinator_reload_guard_" + instance + "\n")
	sb.WriteString("  mode: single\n")
	sb.WriteString("  trigger:\n")
	required := map[string]string{}
	for i, g := range guards {
		id := fmt.Sprintf("guard_%d", i)
		sb.WriteString("  - platform: state\n")
		sb.WriteString("    id: " + id + "\n")
		sb.WriteString("    entity_id: " + g.Entity + "\n")
		sb.WriteString("    to: \"unavailable\"\n")
		sb.WriteString("    for: \"" + g.For + "\"\n")
		if g.WhileEntity != "" {
			required[id] = g.WhileEntity
		}
	}
	// After a start or an automation reload (a deploy) an entity may already be unavailable: no
	// change to trigger on, which is exactly when Ring stayed away (2026-10-25). So then: wait the
	// longest guard time once, and reload for every guarded entity still unavailable.
	longest := guards[0].For
	for _, g := range guards {
		if g.For > longest {
			longest = g.For
		}
	}
	sb.WriteString("  - platform: homeassistant\n")
	sb.WriteString("    id: start\n")
	sb.WriteString("    event: start\n")
	sb.WriteString("  - platform: event\n")
	sb.WriteString("    id: start\n")
	sb.WriteString("    event_type: automation_reloaded\n")
	var watched []string
	for _, g := range guards {
		watched = append(watched, fmt.Sprintf("{'entity': '%s', 'up': '%s'}", g.Entity, g.WhileEntity))
	}
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
		sb.WriteString("  condition:\n")
		sb.WriteString("  - condition: template\n")
		sb.WriteString("    value_template: \"{% set up = {" + strings.Join(pairs, ", ") + "} %}{{ trigger.id not in up or is_state(up[trigger.id], 'on') }}\"\n")
	}
	sb.WriteString("  action:\n")
	sb.WriteString("  - choose:\n")
	sb.WriteString("    - conditions: \"{{ trigger.id == 'start' }}\"\n")
	sb.WriteString("      sequence:\n")
	sb.WriteString("      - delay: \"" + longest + "\"\n")
	sb.WriteString("      - repeat:\n")
	sb.WriteString("          for_each: \"{{ [" + strings.Join(watched, ", ") + "] | selectattr('entity', 'is_state', 'unavailable') | rejectattr('up', 'ne', '') | list + ([" + strings.Join(watched, ", ") + "] | selectattr('entity', 'is_state', 'unavailable') | selectattr('up', 'ne', '') | selectattr('up', 'is_state', 'on') | list) }}\"\n")
	sb.WriteString("          sequence:\n")
	sb.WriteString("          - service: logbook.log\n")
	sb.WriteString("            data:\n")
	sb.WriteString("              name: \"reload guard\"\n")
	sb.WriteString("              message: \"{{ repeat.item.entity }} still unavailable after a start or reload -- reloading its integration\"\n")
	sb.WriteString("          - service: homeassistant.reload_config_entry\n")
	sb.WriteString("            target:\n")
	sb.WriteString("              entity_id: \"{{ repeat.item.entity }}\"\n")
	sb.WriteString("    default:\n")
	sb.WriteString("    - service: logbook.log\n")
	sb.WriteString("      data:\n")
	sb.WriteString("        name: \"reload guard\"\n")
	sb.WriteString("        message: \"{{ trigger.entity_id }} unavailable for {{ trigger.for }} -- reloading its integration\"\n")
	sb.WriteString("    - service: homeassistant.reload_config_entry\n")
	sb.WriteString("      target:\n")
	sb.WriteString("        entity_id: \"{{ trigger.entity_id }}\"\n")
	sb.WriteString("  - delay: \"" + reloadGuardPause + "\"\n")
	return sb.String(), true
}
