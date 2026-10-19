---
name: taps-energy-harvesting-unavailable
description: "Junglinster/Vienna Hue-style taps are energy-harvesting (kinetic), not battery powered — \"unavailable\" after disuse is normal hardware behaviour, not a deploy fault."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-27T15:04:17.418Z
---

The tap switches (e.g. `event.physical_house_*_tap`) are energy-harvesting/kinetic devices, not
battery powered. After a long period unused they drain their internal capacitor and report
`unavailable` — this is expected hardware behaviour, not a sign of a broken deploy or migration.

**Why:** confirmed by Erik 2026-09-27 after the full 18-device Junglinster tap `event.received`
migration rollout showed all newly migrated taps as `unavailable` — initially looked alarming but
is normal for idle kinetic switches.

**How to apply:** don't treat a tap entity's `unavailable` state as evidence of a failed migration
or deploy — check the automation/entity registry and trigger config instead (as done in the
2026-09-27 rollout, see [[project_junglinster_physical_presence_timeline]]). When physically or
MQTT-testing a tap that's been idle a while, it may take multiple presses ("recharging" the
capacitor a bit) before it reports and the automation fires — a single press not registering isn't
necessarily a bug.
