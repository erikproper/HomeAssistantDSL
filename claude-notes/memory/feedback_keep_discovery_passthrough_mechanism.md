---
name: feedback-keep-discovery-passthrough-mechanism
description: "Don't remove/prune the discovery_passthrough mechanism during future code cleanup, even once both houses finish migrating off it -- it's a reusable architectural feature for others, not dead code."
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-20T15:46:03.463Z
---

Erik's own instruction (2026-09-20), given while other dead-code cleanup was being discussed
(see [[project_macro_cleanup_after_junglinster]]): when eventually cleaning up the codebase after
Junglinster catches up to Vienna's own migration, do NOT remove the `discovery_passthrough`
mechanism itself (Physical.def's `discovery_passthrough "<prefix>" from "<source>";` grammar,
house_event_bus_coordinator's relay/name-collision-guard code, the passthrough suggestion
generator) even if neither house is still actively relying on it for any currently-undeclared
device.

**Why:** it's a reusable, general-purpose feature of this architecture -- the "gradual migration"
on-ramp that lets a brand-new Zigbee2MQTT device show up in Home Assistant immediately, with zero
DSL declaration, while a suggestion gets generated for eventually declaring it properly. Someone
else adopting this DSL/coordinator architecture (or Erik himself, adding a THIRD house, or a new
device type down the line) would need this exact mechanism again. "Currently unused by either
house" is not the same as "dead code" here.

**How to apply:** when doing dead-code sweeps or macro/mechanism cleanup (e.g. the queued
[[project_macro_cleanup_after_junglinster]] pass), explicitly exclude discovery_passthrough-related
code from removal candidates, even if a usage-grep shows zero *active* declarations in either
house's own Physical.def at cleanup time. Only remove it if Erik explicitly says the mechanism
itself (not just its current usage) is being retired.
