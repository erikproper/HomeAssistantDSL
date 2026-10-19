---
name: project-mqtt-migration-plan
description: "Active migration to MQTT as the primary integrator: device.<spec> DSL construct, coordinator with real MQTT discovery + live-reported device metadata, and 'as area' spaces are all built; post-migration cleanup items tracked here too."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-08-31T18:19:50.814Z
---

This migration moved from "future plan" to active work during the 2026-08-19/20 sessions, and by
2026-08-22 the coordinator is live and running in production on protocols-server-1. Status:

- `node_alert` generation was dropped entirely, replaced by a single aggregate binary_sensor over
  every declared `*/node`.
- The conceptual-layer DSL construct `entity device.<sphere>:<path> from <device-id> with:
  variable_device_attributes;` (Conceptual_DeviceEntities.go) links a Spaces.def entity to a
  Physical.def "hosts" integration device, implying a node availability entity plus attribute
  entities (load/temperature for `cpu`/`home_assistant` types today). These are registered
  `DiscoveryImplied` (administration.go) — the generator never authors YAML for them; the
  coordinator creates them via real HA MQTT Discovery.
- `house_event_bus_coordinator/` is no longer a stub: it connects to the MQTT broker, publishes
  discovery configs (retained) at startup, subscribes to `hosts/+/device/state` for live-reported
  device metadata (manufacturer/model/hw_version/sw_version/serial_number/via_device), merges it
  with Physical.def's static/forced constants (forced-DSL > live > default-DSL precedence), and
  republishes a device's discovery config live when new data arrives — confirmed working end to
  end. Deployed as a systemd service on protocols-server-1 (`Integrations/coordinator/`).
- `Integrations/cpu/report` (deployed shell script) reports normalized CPU load (as a % of core
  count, not raw Unix load average) plus device metadata, cross-platform (Linux/Pi/Mac).
- A capability entity reference may use `entity!attribute` (matching Spaces.def's existing
  `value sensor.X!state_message;` convention) and an optional literal-string prefix, e.g.
  `sw_version: "Home Assistant Operating System " update.home_assistant_operating_system_update!installed_version;`.
- New DSL construct `space <spec> as area with:` marks a space as an HA Area; devices positioned
  in it (or an un-areaed sub-space) get it as their `suggested_area`, with nested `as area` spaces
  shadowing their enclosing one.
- Discovery payload entity names are now the full location-aware path standalone (e.g.
  `"infrastructural/house/laundry_kitchen/rack/protocols-server-2/cpu/load"`, `has_entity_name:
  false`), not HA's short-label + device-name auto-combination.
- List declarations (Lists.def) support a wildcard trailing leaf pattern, `domain.*/prefix/*`
  (e.g. `sensor.*/cpu/*`), matching any current or future attribute name under a fixed prefix.

**2026-08-23/24 additions, verified live end-to-end:**
- `home_assistant <qualifier>: <name> [<url>];` names a secondary HA instance; `integration
  home_assistant <qualifier> with: device hass.<id> with: <capability>: <entity>; end; end;`
  bridges specific entities from that instance into the main one, positioned via `entity
  device.<spec> from hass.<id> with: variable_device_attributes;` — no node/liveness entity, every
  capability a variable attribute. First real case: protocols-server-2's HP LaserJet status,
  confirmed flowing live as `sensor.infrastructural_house_downward_hallway_laserjet_status`.
- Coordinator-based "assumed to exist" entity warning replaced the old REST presence check
  entirely — `homeassistant/remote_instance_automations.go` generates a bootstrap automation
  (report_entities/reload-over-MQTT, plus self-report on HA's own startup) for every named
  instance: straight into the deployed tree for "main", into a sibling `hass/<qualifier>/` tree
  (with its own minimal configuration.yaml + integrations/automation.yaml skeleton) for every
  other instance, copied on by hand since there's no deploy path to a remote instance yet.
  Verified live on both "main" and protocols-server-2; correctly caught 18 real missing entities
  on main from a broken Volvo integration auth. See [[project_coordinator_deployment]] for the
  protocols-server-2 infra details and the deploy-script pitfalls hit getting this live.
- Next target: Enphase Envoy solar data via the same `home_assistant` bridge mechanism (reusing
  protocols-server-2's already-declared instance), starting 2026-08-25.

**Post-migration cleanup (noted 2026-08-22, not yet acted on):** once the MQTT migration is fully
complete, raw/extensional entity references in Spaces.def — the `domain.[raw_entity_id]` syntax,
e.g. `entity sensor.[ems_esp_boiler_boiler_outside_temperature];` — should become unnecessary for
entities that belong to a device with a `from <device-id>` clause: the device+attribute link
already identifies the entity, so a separate raw-path declaration duplicates that. Needs auditing
once migration is complete: find remaining `.[...]`-style declarations whose underlying device now
has (or could have) a `device.<spec> from <device-id> with: ...;` link, and see whether they can be
replaced/removed. Not yet scoped as an implementation task — just a thing to check for later.

**Why the migration happened:** the old "node" (device-availability/connectivity) machinery — the
`providing` macro and the old `_node_alert` generation/representative-resolution logic — had proven
to be one of the most convoluted, bug-prone parts of the DSL/generator (see
[[feedback_generator_must_match_old_bash]]). MQTT's native LWT/availability topic support lets the
coordinator own this instead of bespoke per-entity template YAML.

**How to apply:** treat this as ongoing, incremental work. Expect further "hosts" integration
devices to get `device.<spec>` declarations and more capability mappings (numeric or
`entity!attribute`-sourced). When the user says "after the MQTT migration is complete," that's a
real future checkpoint — the raw-path cleanup above is one thing gated on it, not something to
start proactively.

**Two classes of registry zombie, found + resolved during Vienna's Netatmo import cleanup
(2026-08-31) — expect both again for Zigbee2MQTT/Z-Wave.** When a device's entities move from one
mechanism to another but deliberately keep the SAME entity_id (for macro/automation continuity —
this project's own stated design goal every time), HA's entity registry doesn't clean up after
itself, and the two failure modes look different enough to be worth telling apart:
1. **MQTT-discovery collision (`_2` suffix).** The OLD mechanism's registry entry for a name is
   still occupying it when the NEW MQTT-discovered entity asks for that same name, so HA silently
   pushes the NEW (live, correct) entity to `_2` instead. Counterintuitive: the clean name is the
   dead one, `_2` is the one actually working. Fix: delete the clean-name zombie AND the `_2`
   entity via the UI (Settings → Devices & Services → Entities) — deleting just the zombie doesn't
   make the `_2` entity rename itself back; it needs to re-register fresh.
2. **Orphaned `template`-platform leftover, no removal notice at all.** Confirmed via this file's
   own line above: `node_alert` generation was dropped entirely years before this migration,
   replaced by the aggregate binary_sensor. Its old per-device `*_node_alert` template entities
   never got deleted from the registry when that generator code was removed — YAML-`template`-
   platform entities get no "no longer provided" notice the way MQTT-discovery-sourced ones do
   (our own `watchForOrphanedDiscoveryTopics`, discoverycleanup.go, only watches MQTT-owned
   topics), so they linger silently forever. Only found by cross-referencing the live entity
   registry (`.storage/core.entity_registry` on the HA host) against current generator output —
   grepping the entity_id across the currently-generated `hass/<house>/` tree and finding zero
   hits is the tell. Fix: same manual UI deletion, no coordinator-side mechanism can catch these.

**How to check, mechanically** (used successfully 2026-08-31, worth reusing as-is next time): pull
`.storage/core.entity_registry` + `core.device_registry` off the live HA host via SSH (`sudo cat`,
since `/config` is usually a symlink into `/homeassistant` and needs root), then cross-reference
entity_ids against (a) the coordinator's own generated YAML file (e.g.
`coordinator/imported_hassbridge.yaml`'s `local_entity:` values) for expected MQTT-owned names,
and (b) a `grep` across the currently-generated `hass/<house>/` tree for anything claiming to be
generator-authored. No HA API token needed for this read-only pass; a token WOULD be needed to
delete entities programmatically instead of walking the user through the UI by hand — worth asking
for one if this recurs at Zigbee2MQTT/Z-Wave's larger scale.
