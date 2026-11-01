/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualVolumeSync
 *
 * Implements Conceptual.def's "volume sync of: ... end;" space-level block (2026-11-01, Erik's
 * design): media players whose volumes can each be moved separately (remote control, apps) are
 * kept at the same level --
 *
 *   space social:living_room as area with:
 *     ...
 *     volume sync of:
 *       media_player.social_house_living_room_sonos;
 *       media_player.social_house_living_room_tv;
 *     end;
 *   end;
 *
 * A member is an entity reference as anywhere in Conceptual.def ("media_player.social:tv") or the
 * final entity id ("media_player.social_house_living_room_sonos"); resolveVolumeSyncMembers decides
 * which, once every entity is declared. Two automations per block:
 *
 *   - <space>/volume_sync: when a member's volume moves, the others follow. After setting them
 *     the run waits until they report that volume (at most volumeSyncEchoWait), so their echo
 *     falls inside the run (mode single drops it) and can't set the moved one back; then, if the
 *     moved one has moved on meanwhile, it follows again, until all agree (at most
 *     volumeSyncRounds). The first version (2026-11-01) instead ignored every change for 15
 *     seconds and then reconciled from the first mover -- found live the same day: it dropped
 *     Erik's own changes on the other member and set them back. A Bravia TV reports a commanded
 *     volume within about 2 seconds, but its own changes (remote, app) only at Home Assistant's
 *     10-second poll. A member without a volume (a TV that is off) is skipped; its switching on is
 *     no movement.
 *   - <space>/volume_sync/start: at a Home Assistant start (not an automation reload, so a deploy
 *     leaves the volumes alone), wait until the members report a volume (at most
 *     volumeSyncStartWait), then set them all to their average, kept within volumeSyncMinimum and
 *     volumeSyncMaximum.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 01.11.2026
 *
 */

package main

import (
	"fmt"
	"strings"
)

const (
	// volumeSyncHeader opens the block; each member line is "<entity-ref>;", "end;" closes it.
	volumeSyncHeader = "volume sync of:"
	// volumeSyncEchoWait is how long a sync waits for the members it set to report their new
	// volume (a Bravia TV: about 2 seconds); volumeSyncRounds bounds its follow-again rounds.
	volumeSyncEchoWait = "00:00:05"
	volumeSyncRounds   = 5
	// volumeSyncStartWait is how long the start automation waits for the members' volumes.
	volumeSyncStartWait = "00:05:00"
	// volumeSyncMinimum/volumeSyncMaximum bound the start volume (Home Assistant's 0..1 scale:
	// 5 and 15 on the devices' own 0..100).
	volumeSyncMinimum = "0.05"
	volumeSyncMaximum = "0.15"
	// volumeSyncTolerance: volumes closer than this count as equal (both are whole percents).
	volumeSyncTolerance = "0.005"
)

// TVolumeSync records one "volume sync of: ... end;" block.
type TVolumeSync struct {
	SpaceName string
	Written   []string // the members as written
	Resolved  []string // the members as written, normalized in the block's space (normalizeEntityFullName)
	Members   []string // the members' final entity ids, set by resolveVolumeSyncMembers
}

// resolveVolumeSyncMembers sets each block's Members: a member's normalized DSL name when that is
// declared, else the member as written when that is a declared final entity id. An undeclared
// member is warned about, and taken as a final id when written without a sphere (no ":"); a block
// with fewer than two members, or a member that is no media player, is warned about too.
func resolveVolumeSyncMembers(syncs []TVolumeSync, declared map[string]bool) {
	declaredIDs := map[string]bool{}
	for name := range declared {
		declaredIDs[toHomeAssistantEntityID(name)] = true
	}
	for i := range syncs {
		sync := &syncs[i]
		sync.Members = nil
		if len(sync.Written) < 2 {
			Reporter.Warn("Space %q: volume sync needs at least two media players, has %d", sync.SpaceName, len(sync.Written))
		}
		for j, written := range sync.Written {
			id := toHomeAssistantEntityID(sync.Resolved[j])
			switch {
			case declaredIDs[id]:
			case declaredIDs[written]:
				id = written
			default:
				Reporter.Warn("Space %q: volume sync references undeclared entity %q", sync.SpaceName, written)
				if !strings.Contains(written, ":") {
					id = written
				}
			}
			if !strings.HasPrefix(id, "media_player.") {
				Reporter.Warn("Space %q: volume sync member %q is no media_player", sync.SpaceName, written)
				continue
			}
			sync.Members = append(sync.Members, id)
		}
	}
}

// generateVolumeSyncAutomations writes each block's two automations; a second block in the same
// space gets "_2" and so on.
func generateVolumeSyncAutomations(outputDir string, admin *TAdministrationState) error {
	seen := map[string]int{}
	for _, sync := range admin.VolumeSyncs {
		if len(sync.Members) < 2 {
			continue
		}
		aliasPath := sync.SpaceName + "/volume_sync"
		seen[aliasPath]++
		if seen[aliasPath] > 1 {
			aliasPath += fmt.Sprintf("_%d", seen[aliasPath])
		}
		sphere := aliasPath[:strings.Index(aliasPath+"/", "/")]
		automationID := strings.ReplaceAll(aliasPath, "/", "_")
		ids := sync.Members
		if err := writeAutomationFile(outputDir, sphere, automationID, volumeSyncAutomationYAML(aliasPath, automationID, ids)); err != nil {
			return err
		}
		if err := writeAutomationFile(outputDir, sphere, automationID+"_start", volumeSyncStartAutomationYAML(aliasPath+"/start", automationID+"_start", ids)); err != nil {
			return err
		}
	}
	return nil
}

// volumeSyncJinjaList renders ids as a Jinja list literal.
func volumeSyncJinjaList(ids []string) string {
	return "['" + strings.Join(ids, "', '") + "']"
}

// volumeSyncAutomationYAML: when a member's volume moves, set the others to it and wait for their
// echo; repeat while the moved one moves on.
func volumeSyncAutomationYAML(aliasPath, automationID string, ids []string) string {
	// agree renders "every other member with a volume is within volumeSyncTolerance of v".
	agree := func(v string) string {
		return "{% set ns = namespace(ok=true) %}{% for o in others %}{% set w = state_attr(o, 'volume_level') %}{% if w is number and (w - " + v + ") | abs > " + volumeSyncTolerance + " %}{% set ns.ok = false %}{% endif %}{% endfor %}{{ ns.ok }}"
	}
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- alias: " + aliasPath + "\n")
	sb.WriteString("  id: automation." + automationID + "\n")
	sb.WriteString("  initial_state: True\n")
	sb.WriteString("  mode: single\n")
	sb.WriteString("  max_exceeded: silent\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id:\n")
	for _, id := range ids {
		sb.WriteString("      - " + id + "\n")
	}
	sb.WriteString("    attribute: volume_level\n")
	sb.WriteString("  condition:\n")
	sb.WriteString("  - condition: template\n")
	sb.WriteString("    value_template: \"{{ trigger.from_state is not none and trigger.to_state is not none and trigger.from_state.attributes.volume_level is number and trigger.to_state.attributes.volume_level is number }}\"\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - variables:\n")
	sb.WriteString("      leader: \"{{ trigger.entity_id }}\"\n")
	sb.WriteString("      others: \"{{ " + volumeSyncJinjaList(ids) + " | reject('eq', trigger.entity_id) | list }}\"\n")
	sb.WriteString("  - repeat:\n")
	sb.WriteString("      sequence:\n")
	sb.WriteString("      - variables:\n")
	sb.WriteString("          target: \"{{ state_attr(leader, 'volume_level') }}\"\n")
	sb.WriteString("      - condition: \"{{ target is number }}\"\n")
	sb.WriteString("      - repeat:\n")
	sb.WriteString("          for_each: \"{{ others }}\"\n")
	sb.WriteString("          sequence:\n")
	sb.WriteString("          - if: \"{{ state_attr(repeat.item, 'volume_level') is number and (state_attr(repeat.item, 'volume_level') - target) | abs > " + volumeSyncTolerance + " }}\"\n")
	sb.WriteString("            then:\n")
	sb.WriteString("            - service: media_player.volume_set\n")
	sb.WriteString("              target:\n")
	sb.WriteString("                entity_id: \"{{ repeat.item }}\"\n")
	sb.WriteString("              data:\n")
	sb.WriteString("                volume_level: \"{{ target }}\"\n")
	sb.WriteString("      - wait_template: \"" + agree("target") + "\"\n")
	sb.WriteString("        timeout: \"" + volumeSyncEchoWait + "\"\n")
	sb.WriteString("        continue_on_timeout: true\n")
	// Done when all agree with the moved one's volume now -- or it has none, or the rounds ran out.
	sb.WriteString(fmt.Sprintf("      until: \"{%% set l = state_attr(leader, 'volume_level') %%}{%% if repeat.index >= %d or l is not number %%}true{%% else %%}%s{%% endif %%}\"\n", volumeSyncRounds, agree("l")))
	return sb.String()
}

// volumeSyncStartAutomationYAML: at a Home Assistant start, set the members to their average
// volume, kept within volumeSyncMinimum..volumeSyncMaximum.
func volumeSyncStartAutomationYAML(aliasPath, automationID string, ids []string) string {
	list := volumeSyncJinjaList(ids)
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("- alias: " + aliasPath + "\n")
	sb.WriteString("  id: automation." + automationID + "\n")
	sb.WriteString("  initial_state: True\n")
	sb.WriteString("  mode: single\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: homeassistant\n")
	sb.WriteString("    event: start\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - wait_template: \"{{ " + list + " | map('state_attr', 'volume_level') | select('number') | list | count == " + fmt.Sprint(len(ids)) + " }}\"\n")
	sb.WriteString("    timeout: \"" + volumeSyncStartWait + "\"\n")
	sb.WriteString("    continue_on_timeout: true\n")
	sb.WriteString("  - variables:\n")
	sb.WriteString("      members: \"{% set ns = namespace(ids=[]) %}{% for e in " + list + " %}{% if state_attr(e, 'volume_level') is number %}{% set ns.ids = ns.ids + [e] %}{% endif %}{% endfor %}{{ ns.ids }}\"\n")
	sb.WriteString("      volume: \"{% set v = members | map('state_attr', 'volume_level') | list %}{{ ([[v | sum / v | count, " + volumeSyncMinimum + "] | max, " + volumeSyncMaximum + "] | min) | round(2) if v | count > 0 else none }}\"\n")
	sb.WriteString("  - condition: \"{{ members | count > 0 }}\"\n")
	sb.WriteString("  - service: media_player.volume_set\n")
	sb.WriteString("    target:\n")
	sb.WriteString("      entity_id: \"{{ members }}\"\n")
	sb.WriteString("    data:\n")
	sb.WriteString("      volume_level: \"{{ volume }}\"\n")
	return sb.String()
}
