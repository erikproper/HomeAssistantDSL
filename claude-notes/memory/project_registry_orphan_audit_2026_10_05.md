---
name: project-registry-orphan-audit-2026-10-05
description: "PROJECT.md item 6 (registry orphan audit, prep for item 7's stable-id bootstrap) run 2026-10-05 on both houses' main HA core.entity_registry: Vienna 45 orphans/1087 live entities, Junglinster 348/3599. Categorized, not yet cleaned up."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-05T19:25:43.082Z
---

Ran PROJECT.md's own item 6 ("check HA main's IDs from the registry... any orphans? any unused
ones we may want to re-use later?") against both houses' live main HA instance, via
[[reference_main_ha_ssh_access]]. Pulled `.storage/core.entity_registry` from both, cross-referenced
every sphere-prefixed (`social_`/`physical_`/`infrastructural_`/`meta_`) live entity against what
the generator currently believes should exist — `template`-platform entities against the backing
`<entity_id>.yaml` file under `hass/<house>/` (same method as
[[project_undeclared_entity_audit_technique]], inverted), `mqtt`-platform entities against
`discovery.yaml`/`devices.yaml`/`homeassistant_bridge.yaml` (`local_entity:`/`node_entity:`/bare
`entity:` fields plus top-level `<domain>.<path>:` keys). Non-generator-managed platforms (`group`,
`switch_as_x`, `ring`, `apple_tv`, `esphome`, `workday`, `sun`, ...) excluded outright — those are
HA-UI-configured, never generator output, regardless of sphere-prefixed naming coincidence.

**Results**: Vienna 45 orphans / 1087 live sphere-prefixed-domain entities checked; Junglinster 348
/ 3599. Breakdown by category (script: `audit_registry2.py`, scratchpad of the 2026-10-05 session —
not committed anywhere, re-derivable from this method if needed again):

| category | Vienna | Junglinster | meaning |
|---|---|---|---|
| `retired_node_alert_feature` | 0 | 159 | the whole `node_alert` mechanism was dropped years ago (confirmed zero live usage anywhere in `hass/`/`coordinator/`) — see [[project_mqtt_migration_plan]]'s own 2026-08-31 note already flagging this feature as dead. Safe bulk-delete candidate. |
| `stale_node_old_device_name` | 8 | 78 | old `_node` entity from a device later renamed/repositioned; current device has a new `_node` entity already live. |
| `stale_battery_alert_old_device_name` | 10 | 6 | same shape, `_battery_alert` suffix. |
| `stale_battery_level_old_device_name` | 0 | 1 | same shape, `_battery_level` suffix. |
| `duplicate_suffix_collision` (`_2`/`_3`) | 12 | 7 | the actual "_2 registry-collision dance" — a stale entry squatted the clean entity_id, forcing a later real registration to `_2`. Direct motivation for item 7's stable-id design. |
| `retired_meta_installation_qualifier` | 2 | 2 | `button.meta_reload_<house>`/`meta_restart_<house>` (e.g. `meta_reload_vienna`) — leftover from before [[project_meta_reload_all_simplification]] collapsed per-installation qualifiers into instance-name + `all`. |
| `misc_oneoff` (needs manual review) | 13 | 95 | mixed bag, NOT verified individually except a sample — see caveats below. |

**Caveats, important before acting on the `misc_oneoff` bucket or doing any bulk cleanup**:
- Spot-checked several Junglinster `misc_oneoff` entries and found at least two distinct real causes,
  not script bugs: (a) a domain change (`switch.social_house_living_room_living_christmas_tree` is a
  genuine orphan — the device now exposes `light.social_house_living_room_living_christmas_tree`
  instead), and (b) a device still declared in Physical.def (`sensors.bathroom_aqara_multi`,
  temperature/humidity/pressure/battery_level capabilities) that currently has ZERO Conceptual.def
  positioning at all — a live instance of PROJECT.md item 20b's own "declared but unpositioned
  capability" gap, not a simple ghost. Its registry entries are the concrete "unused ones we may
  want to re-use later" case item 6 itself asks about.
- Vienna's `update.*` orphans (`update.infrastructural_apartment_bedroom_bed_shortcut_left` etc.)
  are very likely FALSE POSITIVES, not real orphans: this audit's `mqtt`-platform check only looks at
  the generator's own 3 coordinator YAML files, which do NOT cover
  [[feedback_keep_discovery_passthrough_mechanism]]'s raw Zigbee2MQTT-native passthrough entities
  (e.g. OTA `update` entities) — those are coordinator-relayed but never generator-synthesized, so
  they'll never appear in `discovery.yaml`/`devices.yaml`. Don't delete anything in this shape
  without checking passthrough state first.
- `_2`/`_3` duplicates found so far were NOT active collisions (the clean-name entity_id wasn't
  simultaneously present in most checked cases) — they're historical artifacts from a now-resolved
  collision, several already `disabled_by: "user"`. Real evidence FOR item 7's stable-id design, not
  an open incident needing immediate action.

**Cleanup DONE 2026-10-05, same session**: the four "safe" buckets (`retired_node_alert_feature`,
`stale_node_old_device_name`, `stale_battery_alert_old_device_name`,
`stale_battery_level_old_device_name`, `retired_meta_installation_qualifier`) were deleted via the
established direct-registry-edit procedure ([[project_ha_entity_registry_collision_lessons]]):
dated backup on each host, upload edited JSON to `/tmp`, byte/count-verified, `ha core stop` ->
swap `/config/.storage/core.entity_registry` -> `ha core start`, verified via HTTP 200 + diffing
before/after entity sets. The `duplicate_suffix_collision` and `misc_oneoff` buckets were
deliberately left untouched (per the user's own explicit choice: clean the safe buckets now, leave
the rest for later triage/item 7).

**One real catch before deleting anything, worth remembering for next time**: before trusting the
suffix-pattern categorization, I individually checked every `mqtt`-platform entry across both safe
lists (5 total) for `device_id`/`modified_at`, since template-platform entries carry no passthrough
risk but mqtt-platform ones can. Found one genuine false positive:
`sensor.infrastructural_house_bathroom_aqara_multi_battery_level` (Junglinster) matched the
`stale_battery_level` suffix pattern but had a real `device_id` and a `modified_at` of
`2026-02-09`, not epoch — it's `sensors.bathroom_aqara_multi`'s own Zigbee2MQTT-native battery
diagnostic, still alive via raw passthrough even though the device itself has zero Conceptual.def
positioning (same device as the "declared but unpositioned" case above). Excluded it; final
delete counts were Vienna 20, Junglinster 245 (246 minus this one).

**Outcome**: Vienna 18/20 removed permanently — `button.meta_reload_vienna`/`meta_restart_vienna`
came back on the very next restart, meaning they're NOT actually dead (still republished via some
retained MQTT discovery path, contradicting the assumption that
[[project_meta_reload_all_simplification]] fully superseded the per-house-name qualifier on
Vienna). Harmless either way (duplicate-free, no error), but a real loose end: if this item is
revisited, check whether Vienna's coordinator (unlike Junglinster's, where the equivalent pair did
NOT come back) still emits this discovery topic, or whether it's a stale retained MQTT message that
never got cleared. Junglinster: all 245/245 removed cleanly, nothing came back, zero new errors in
either house's post-restart logs (only pre-existing, unrelated native-integration noise — Ring API
connectivity, an unrelated template/sensor "unknown"-state warning on Vienna).

**Round 2, same day, the `misc_oneoff` bucket**: triaged and cleaned up. Critical methodology
correction first: `device_id`/`modified_at` registry metadata (round 1's liveness signal) proved
UNRELIABLE — both fields persist from whenever the entity was last registered/renamed and say
nothing about whether it's reporting real data *now*. Caught via `sensor.infrastructural_house_
bathroom_aqara_multi_battery_level` (round 1 excluded it as "LIVE_PASSTHROUGH" on exactly this
basis) — querying the live recorder DB (`/config/home-assistant_v2.db`, `states`/`states_meta`
tables, `sudo sqlite3` over the same SSH channel) showed it has had ZERO non-unavailable/unknown
rows in 48 recorded transitions back to 2026-09-29. It was actually dead; round 1's exclusion was
wrong. **The recorder DB, not entity_registry metadata, is the only reliable liveness signal** —
used for the rest of this round and worth remembering for any future registry work.

Full-history recorder query (total rows vs. non-unavailable/unknown rows, then last-real-value
timestamp for anything with at least one) against both houses' 95+13 `misc_oneoff` entries found
three distinct groups, not two:
1. **Zero real rows, ever** (50 Junglinster) — straightforwardly dead, same as any other ghost.
2. **Real rows exist, but none in 6+ days** (6 Junglinster: 3 `*_apple_tv_node_raw`/`*_tv_node_raw`
   binary_sensors last real 2026-09-29 — the OLD pre-media_player_device-migration mechanism,
   Junglinster's own migration is still queued per [[project_logical_layer_media_player_device]];
   2 `light.*_main_dimmer` entities last real 2026-09-27; `switch.physical_house_kitchen_
   workplace_furnace` last real 2026-09-28 — superseded the same day by the heating-relay fix noted
   in [[project_stable_entity_identity_design]]'s "second live motivating incident, 2026-10-05").
   Treated the same as group 1 — a mechanism/device that's been stale for a week-plus during
   active development is a real orphan, not a transient gap.
3. **Real rows, most recent within the last day** (40 Junglinster, 6 Vienna) — genuinely live
   today, e.g. aqara_multi pressure/temperature sensors, radiator calibrate buttons, several
   `switch.social_house_living_room_living_*` entities (christmas_tree/gray_sideboard/
   hanging_sideboard/picture_frame/indirect), Vienna's `update.*` passthrough entities and its own
   picture_frame switch. **Left untouched** — these are the concrete instances of item 20b's
   "declared but unpositioned capability" gap (confirmed for several: still declared in
   Physical.def, zero Conceptual.def positioning, kept alive only by raw Zigbee2MQTT/passthrough
   publishing, not by anything the generator currently emits). A real DSL/positioning task, not a
   registry-cleanup one — don't delete these, and don't forget they exist next time item 20b or a
   "declared but unpositioned" report is built.

Deleted (groups 1+2 combined, same direct-registry-edit procedure, dated backups, byte/count
verified): Junglinster 56 (55 from the misc bucket + the 1 battery_level correction above),
Vienna 7. Both restarts clean (HTTP 200, no `entity_registry` errors). Junglinster's
`meta_reload_junglinster`/`meta_restart_junglinster` reappeared again on this restart (same
still-unexplained republishing as Vienna's pair in round 1 — see that note above, still an open
loose end, now confirmed on both houses).

**Final tally across both rounds**: Vienna 18+7=25 entities permanently removed (of 45 original
orphans), Junglinster 245+56=301 removed (of 348 original + the 1 correction). Remaining
untouched: the `duplicate_suffix_collision` bucket (12 Vienna/7 Junglinster — not an active
problem, direct motivating evidence for item 7) and the 46 confirmed-live misc entries above
(an item-20b positioning task, not a cleanup one).

**Round 3+4, same day, the `_2`/`_3` collision bucket**: fully emptied (19/19), not by blind
deletion — each of the 19 needed individual judgment, and roughly a third turned out to need a
RENAME (recovering the clean name) rather than a delete. Breakdown:
- **10 Vienna `*_battery_level_2`**: confirmed dead (zero recorder rows ever, already
  `disabled_by: user`) — straightforward delete.
- **3 Junglinster `*_node_2`** (xanadu, shower_room_sonos, storage_room_backups): the CLEAN name
  didn't exist in the registry at all (removed at some point before this session, unrelated to
  this work) and the `_2` was the only real entity for each device — renamed `_2` back to the
  clean name (nothing to delete, nothing blocking the rename).
- **2 Vienna "both look alive" cases, resolved by the user's own call**: `sensor.social_
  apartment_living_room_temperature` and `..._rack_humidity` each had a `group`-platform clean
  entity (self-referential unique_id, `config_entry_id: null`, no backing YAML/helper found
  anywhere) alongside a real MQTT `_2` twin. The `temperature` group was actively "live" but
  producing floating-point drift artifacts (`24.7`, `24.7000000000001`, `24.7000000000003`, ...,
  each step roughly 4x the prior epsilon — a self-referential recalculation loop); the
  `rack_humidity` group had never produced a real value at all. User's call: **the `_2` versions
  win** — deleted both buggy/dead `group` entries, renamed both `_2` twins onto the clean names.
- **3 Junglinster `light.*_2`** (christmas_tree/gray_sideboard/hanging_sideboard): the clean name
  was a manually-created **"Switch as X" UI helper** (`switch_as_x` platform, wrapping the
  device's own switch entity to present it as a light), predating the device's later native MQTT
  light discovery (the `_2`). User's diagnosis: "the aliases probably didn't clear out when
  switching off the 'MQTT forward' option" — confirmed via `core.config_entries` (each had a
  dedicated `switch_as_x` entry, `source: "user"`, wrapping `switch.social_house_living_room_
  living_*`). **Deleting the entity_registry row alone is NOT enough for a helper** — the
  `switch_as_x` config entry would recreate it on the next restart (same class of problem as the
  `meta_reload_<house>` buttons coming back in rounds 1/2). Fix: removed both the entity_registry
  row AND the matching `core.config_entries` entry (same stop/backup/edit/verify/start discipline,
  applied to a second `.storage` file for the first time this session), then renamed `_2` → clean.
  Verified post-restart: real `off` state under the recovered name, no `_2` twin recreated.
  Note: two OTHER already-orphaned duplicate `switch_as_x` entries for the same two sideboards
  were found sitting inert in `core.config_entries` (superseded by the ones removed here, no
  matching live entity) — left untouched, out of scope, flagged only.
- **1 Junglinster `sensor.social_sun_solar_elevation`**: NOT touched by this audit — the user
  diagnosed and fixed it independently (the native `sun` integration was still active on main HA
  and shouldn't be; such integrations belong on the `ha_mqtt`/`ha2mqtt` instance instead). Same
  "sun" issue found dormant on Vienna's own main HA too (not yet fixed there as of this session).

**Methodology bug found and fixed mid-triage**: the "declared" check (used to decide whether an
entity is still generator-declared) only matched specific YAML field NAMES (`entity:`/
`local_entity:`/`node_entity:`) — missed `commandline.yaml`'s `entity_id:` field entirely. Caught
because the user flagged `switch.social_apartment_living_room_picture_frame` as a known, correctly
positioned device (`device appliance.picture_frame as picture_frame with entity switch.social:
from switch.slideshow;`, Conceptual.def) that this audit had wrongly reported as "declared but
unpositioned." Root cause: the coordinator's own YAML output uses many different field names for
an entity_id value depending on integration type (`entity:`, `entity_id:`, `local_entity:`,
`node_entity:`, plus arbitrary capability-named keys like `slideshow:`/`fritz_os:`/`daylight:`
under `switches:`/`capabilities:` blocks) — matching by field NAME is inherently fragile. Fixed by
switching to a VALUE-shape scan instead (any YAML line whose value matches `<ha_domain>.<rest>`,
regardless of the key name) — this found 193 Vienna / 558 Junglinster declared entity_ids
(vs. far fewer under the old name-matching regex). Re-checked all 46 "live but unpositioned"
candidates against the fixed scan: exactly 2 were false positives (both named `picture_frame`,
both declared via `commandline.yaml`'s `slideshow:` field) — corrected final counts: **44 genuinely
unpositioned-but-live** (39 Junglinster, 5 Vienna), not 46. No deletions were affected by this bug
— round 2's actual deletions were decided from live recorder data, not this "declared" check; the
bug only tainted the DESCRIPTIVE claim about which entities need item 20b positioning work, now
corrected.

**Final state**: the `_2`/`_3` collision bucket is now fully empty on both houses (0/19 remaining).
The 44 live-but-unpositioned entities remain, untouched, as item 20b's concrete worklist.

**How to apply**: before starting item 7's bootstrap (seeding `as <id>` from each capability's
CURRENT unique_id), re-run this same audit fresh — numbers here are already stale. Whenever judging
whether a registry entry is still live, query the recorder DB directly
(`sudo sqlite3 /config/home-assistant_v2.db`, join `states`/`states_meta` on `metadata_id`, check
for non-unavailable/unknown rows and their recency) — never trust `device_id`/`modified_at`/
`created_at` alone, they reflect registration history, not live reporting. When retiring a
registry entry that belongs to a UI-created helper (`switch_as_x`, `group`, `template` helpers,
etc. — check for a matching row in `core.config_entries`), the config entry must be removed too, or
it recreates the entity on the next restart — the same lesson as the still-unexplained
`meta_reload_<house>` buttons from rounds 1/2, generalized. And when scanning generator-authored
YAML for "is this entity declared," match by VALUE shape (`<domain>.<rest>`), never by a fixed set
of field names — this project's own coordinator YAML has no single consistent field-naming
convention for entity_id values across integration types.
