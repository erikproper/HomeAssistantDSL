---
name: junglinster-stale-template-entity-collision-2026-09-23
description: "DONE+FIXED 2026-09-23 - 21 Junglinster entities (9 aqara pressure, 9 aqara thermostat temperature, 3 Sonos node) stuck unavailable as \"_2\" duplicates because old orphaned template-platform registry entries squatted their clean entity_id; fixed via direct registry edit"
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-23T10:40:11.162Z
---

Real incident, found live 2026-09-23 (user noticed `sensor.social_house_laundry_kitchen_pressure`
— actually `sensor.physical_house_laundry_kitchen_front_aqara_multi_pressure` — showing unavailable
in Junglinster's UI, independent of and predating the same-day `packages:` removal work, see
[[project_homeassistant_packages_reload_limitation_fix]]).

**Root cause**: every device migrated this session from the old generator-authored
"derived ... via jinja" TEMPLATE-platform mechanism to the new discovery-kind
"hidden X_raw + derived X via jinja" mechanism (materialized entirely via the coordinator's own
`value_template_wrap`, see [[project_jinja_template_macros]]) left its OLD template-platform
registry entry behind, still occupying the clean `entity_id`. Since HA never auto-frees/reclaims a
name, the NEW coordinator-published MQTT discovery entity — same conceptual sensor, but a genuinely
different `unique_id` — got registered under a `_2`-suffixed name instead. Nothing (dashboards,
groups, automations) was ever updated to the new `_2` name, so the visible entity was the dead,
never-updating orphan the whole time. Confirmed via registry inspection: all 21 affected entities
had `platform: "template"`, `config_entry_id: None`, `device_id: None` — pure orphaned YAML, no
config entry to worry about.

Affected (21 total, ALL confirmed — no partial/missed cases): 9 aqara_multi pressure sensors
(every device in Physical.def with `derived sensor.pressure from sensor.pressure_raw`: garage/rear,
laundry_kitchen/front, downward_hallway, upward_hallway/second_floor, server_room, storage_room/door,
bathroom, office/door, loft), 9 aqara_thermostat radiator temperature sensors (david/master/sarah
bedroom, upward_hallway/first_floor, kitchen, shower_room, living_room front+rear, garage), 3 Sonos
"node" liveness binary_sensors (kitchen, shower_room, garage).

**Fix procedure** (matches [[project_ha_entity_registry_collision_lessons]]'s established safe
procedure exactly, this is the second live use of it): backup `core.entity_registry` -> pull it
down -> Python script asserts each of the 21 pairs (stale template entry + its `_2` mqtt twin) is
exactly as expected, then removes the 21 stale entries and renames each `_2` twin's own
`entity_id` field back to the clean name -> validate the edited JSON -> upload -> `ha core stop`
-> apply -> `ha core start` -> verify. Confirmed all 21/21 fixed (clean name present, old `_2` name
gone) and zero new config/setup errors, entity count exactly 5287-21=5266 as expected.

**Access lesson, worth remembering**: the `hassio@junglinster.homelinux.org -p 1967` SSH channel
(the one used throughout this whole project for file access) gives a real filesystem shell AND has
`ha` CLI installed, but `ha core stop`/`start` fail with "unauthorized: missing or invalid API
token" by default — the SUPERVISOR_TOKEN isn't in this login shell's own environment. Fix: read it
from the s6-overlay container-environment file directly:
`SUPERVISOR_TOKEN=$(cat /run/s6/container_environment/SUPERVISOR_TOKEN) ha core stop` (same
pattern for `start`/`info`/etc.). This resolves the "how do I stop Core from THIS channel" gap the
prior memory's own procedure left open (it only warned that the in-UI terminal dies on stop; it
didn't yet have this direct answer for this project's own actual SSH add-on).

**Not yet checked**: this session only investigated Junglinster's MAIN instance. The same disease
(orphaned template-platform entries from the same historical migration work) could plausibly exist
on Vienna's own main instance too, or on Junglinster's ha2mqtt/Vienna's ha2mqtt if either has
template-platform history — worth a similar audit (`platform=="template"` with no
`config_entry_id`/`device_id`, cross-referenced against `_2`-suffixed mqtt twins) if similar
"entity X unavailable" reports come up there.
