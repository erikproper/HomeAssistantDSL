---
name: project-dimmer-device-naming-2026-10-05
description: "DONE 2026-10-05 (Junglinster): all 7 mis-prefixed sensors.* devices renamed -- 4 philips_dimmer -> buttons.*, and (once item 7's stable-id mechanism landed same day) the 3 main_dimmer/workplace_dimmer devices -> lights.* too, with zero registry churn. See project_stable_entity_identity_implementation_2026_10_05.md for the mechanism that unblocked the second half."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-06T01:33:49.169Z
---

User flagged (reading Physical.def) that 7 Junglinster devices had the wrong functional-category
device-ID prefix, per [[feedback_device_naming_avoid_integration_names]]'s own rule (device ids
name the device's functional category, never the backing mechanism):

- 4 "philips_dimmer" devices (`sensors.living_room_dining_philips_dimmer`, `..._living_room_living_
  philips_dimmer`, `..._house_kitchen_terrace_door_philips_dimmer`, `..._bathroom_philips_dimmer`)
  — these are battery-powered remote BUTTONS (`sensor.battery_level` + `event.pressed`, no sensor
  reading of their own), not sensors.
- 3 "main_dimmer"/"workplace_dimmer" devices (`sensors.living_room_living_main_dimmer`, `..._
  living_room_dining_main_dimmer`, `..._house_kitchen_workplace_dimmer`) — these are actual
  dimmable LIGHTS (`light.core`), not sensors.

**Renamed the 4 button devices to `buttons.*`** — confirmed SAFE first: generated before/after,
`diff -rq` on the entire `hass/<house>/` tree plus `discovery.yaml`/`devices.yaml`/
`homeassistant_bridge.yaml` came back byte-identical except the renamed device-ID strings
themselves (never visible in any HA-facing entity_id). These devices have explicit Conceptual.def
positioning for every capability, so the device-ID prefix never leaks into a generated name.

**Initially deferred, then completed same day once item 7 landed**: the 3 light devices'
`light.core` entity would otherwise have changed 3 REAL, currently-live entity_ids on a bare
rename:
- `light.physical_sensors_living_room_living_main_dimmer` -> `light.physical_lights_living_room_living_main_dimmer`
- `light.physical_sensors_living_room_dining_main_dimmer` -> `light.physical_lights_living_room_dining_main_dimmer`
- `light.physical_sensors_house_kitchen_workplace_dimmer` -> `light.physical_lights_house_kitchen_workplace_dimmer`

**Why these 3 differ from the 4 buttons**: these devices use the `light.core is switched by
<leaf> from <source-device>;` one-liner sugar (Logical.def), not an explicit Conceptual.def
positioning line for their own `light.core` capability — the auto-materialization fallback that
sugar uses names the entity directly from the device ID, so changing the ID changes the name. The
4 button devices have no such sugar; every capability is explicitly positioned with its own clean
name, independent of the device ID.

**Resolved same day**: once [[project_stable_entity_identity_implementation_2026_10_05]] (item 7)
was built and bootstrapped, these 3 renames were applied as the mechanism's own first real proof
— `discovery.yaml`'s `gateway:` now reads `lights.*` for all 3, but `stable_id:` (the field that
actually matters for the live HA registry) is byte-identical to before the rename, confirmed. No
registry churn. Same sequencing precedent now applies to PROJECT.md item 14's own moes/somfy
renames, which can proceed the same way.

**IMPORTANT correction, found while building item 7**: the "4 buttons" rename above, done BEFORE
item 7 existed, had only been verified by diffing the GENERATOR's own static file output — that
check never covered the COORDINATOR's own runtime-computed `unique_id` (a function of `gatewayID`
too, invisible in any generator-side diff). All 7 renames, not just the 3 originally flagged,
actually needed the stable-id mechanism to be safe to deploy. The 4 `buttons.*` devices are now
ALSO correctly bootstrapped (their current raw unique_id frozen) as part of item 7's full
two-house sweep, so they're safe now too — but they were NOT safe between being renamed and item 7
landing, purely by luck of not having been deployed in between.

**How to apply**: before renaming ANY Physical.def/Logical.def device ID in this codebase, always
generate before/after and `diff -rq` the full `hass/<house>/` tree plus the 3 coordinator YAML
files (`discovery.yaml`/`devices.yaml`/`homeassistant_bridge.yaml`) BEFORE deciding it's safe —
don't assume a device-ID rename is cosmetic just because it usually is. The "switched by"/
"enabled by" one-liner sugars (and likely other auto-materialization fallback paths) are the
known exception: they can bake the raw device ID into a real entity_id when there's no explicit
positioning line to override it.
