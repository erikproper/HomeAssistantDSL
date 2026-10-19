---
name: project-undeclared-entity-audit-technique
description: "Reusable technique to find live HA entities matching the {social,physical,infrastructural}_... naming convention that were never declared/positioned in a house's Spaces.def."
metadata: 
  node_type: memory
  type: reference
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-14T19:46:47.464Z
---

Built and run live 2026-09-14 for both houses, ahead of the item-4/item-8 Zigbee2MQTT migration
work (finding devices that need a Spaces.def declaration before they can be migrated off
discovery_passthrough).

**Technique:** the generator authors exactly one customization or entity YAML file per entity it
knows about, named literally `<entity_id>.yaml` (e.g. `light.social_apartment_living_room_main.yaml`),
somewhere under `hass/<house>/`. So: pull the live `core.entity_registry`, filter to entities whose
object_id (the part after the domain dot) starts with `social_`/`physical_`/`infrastructural_`,
exclude non-positionable domains (automation/script/scene/zone/person/device_tracker/notify/...,
i.e. things Spaces.def never positions), exclude a `switch_as_x` helper's own backing switch (found
via `options.switch_as_x.entity_id` on the helper), then check whether a file with that exact
basename exists anywhere in `hass/<house>/` (`os.walk` + basename set, not a path-shaped guess).
No match anywhere = never declared in Spaces.def.

**Caveats learned running it on both houses:**
- Run `./generate` first so the check is against current output, not a stale tree — the generator
  just needs to have run once; git commit state doesn't matter, this reads the filesystem directly.
- A clean run (Vienna, 2026-09-14: 21 hits) means genuinely-never-positioned devices, a good
  actionable list. A noisy run (Junglinster, same day: 600+ hits) can mean the house's own
  `hass/<house>/` output is itself stale relative to current Definitions (a separate, unrelated
  backlog — check `git status` on the hass output tree first), OR that the house has been through
  multiple space-hierarchy reorganizations over time and many hits are orphaned leftovers of an old
  space path rather than truly undeclared devices. Don't treat a noisy result as a ready-to-position
  batch the way a clean one is — spot-check a sample against current Spaces.def first.
