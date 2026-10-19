---
name: project-final-orphan-check-2026-10-06
description: "DONE 2026-10-06: PROJECT.md item 7b, the final post-item-7 orphan sweep on both houses. Vienna clean. Junglinster: 2 new template-entity ghosts found+cleaned (device zwave_057/058 renamed same day, unrelated to the stable-id rollout); the ~40-entry item 20b \"live but unpositioned\" bucket and the sun_solar_elevation collision are pre-existing, already tracked, deliberately untouched."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-06T10:25:07.498Z
---

Ran the same recorder-DB-grounded audit technique from [[project_registry_orphan_audit_2026_10_05]]
against both houses' live registries right after item 7's deploy, as the explicit "did tonight's
deploy (or anything since) leave anything behind" close-out pass.

**Vienna**: clean. The only hits were the `meta_reload_*`/`meta_restart_*`/`meta_discover_entity`
buttons, which are a known false-positive class (dynamically generated at coordinator runtime from
`home_assistant_instances.yaml`, never appearing as a static value in any coordinator YAML file —
the quick audit script rebuilt for this pass didn't carry over the original script's whitelist for
them; not a real orphan).

**Junglinster**: 2 genuinely new template ghosts found and cleaned —
`binary_sensor.infrastructural_house_kitchen_fuse_cabinet_zwave_057_hallway_door_socket_node` and
`..._zwave_058_node`. Root cause: `switches.house_kitchen_hallway_door_power_socket_zwave_057`/
`switches.garage_fuse_cabinet_zwave_058` were renamed/repositioned in Conceptual.def earlier the
same day (2026-10-06, ~19:23-19:39 UTC the day before per the recorder — almost certainly the
user's own concurrent edit, flagged mid-session during the step-7 work), leaving their OLD
`node`-capability template entities stuck at `unavailable` with no further updates ever since.
**Unrelated to the stable-id mechanism** — `node` is always a locally-computed template entity
(`derived binary_sensor.node with condition ...;`), never a raw-leaf MQTT relay, so it was never in
scope for [[project_stable_entity_identity_implementation_2026_10_05]] to cover; this is the
ordinary "renaming a device leaves its old template entity behind" case item 6 was built to catch,
just two instances that either postdated that sweep or were missed. Cleaned via the same
stop-core/backup/swap/verify registry-surgery procedure, confirmed via a recorder check first
(62/77 historically real rows, but zero real activity since the rename — genuinely, permanently
dead, not a transient blip).

**Deliberately left untouched, both already tracked elsewhere**:
- Junglinster's ~40-entry "live but unpositioned" bucket (radiator calibrate buttons, aqara_multi
  pressure/temperature sensors, several `living_room_living` switches, etc.) — the exact same
  item 20b candidates [[project_registry_orphan_audit_2026_10_05]] already catalogued and
  deliberately left alone (these are real, live, just not yet given Conceptual.def positioning —
  a DSL task, not a registry-cleanup one).
- `sensor.social_sun_solar_elevation` (platform `sun`, still a native integration entity) and its
  `_2` hassbridge-imported twin **still both exist** as of this check, despite the user's own
  "Fixed" note earlier in the step-7 work (disabling the native `sun` integration on Junglinster's
  main HA). Worth a second look if this collision is still meant to be resolved — not re-litigated
  or touched here since the user explicitly owns this one.

**How to apply**: the quick-rebuilt audit script for this pass needs the `meta_*` whitelist
(`is_meta_whitelisted` in the original item 6 script) added back if reused again, to avoid
re-flagging those 9-ish entries as false orphans every time.
