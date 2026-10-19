---
name: feedback-keep-node-identity-blocks-sorted
description: "Keep each space's Z-Wave node-identity registration block sorted numerically by node ID as new nodes are added, in both houses."
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-24T18:32:06.115Z
---

Whenever adding a new Z-Wave node's identity-registration line (the `device switches.<location>_zwave_<NNN> as fuse_cabinet/zwave/<NNN> with entity binary_sensor.infrastructural:node;` block, one per space that has a fuse cabinet/box) to Junglinster's Conceptual.def, insert it in numeric ID order among its siblings, not just appended at the end.

**Why:** the user explicitly asked (2026-09-24) to keep these blocks tidy as they grow — "There will be a few more in both locations" (garage's fuse cabinet block, e.g. 033/042/067, and server_room's, e.g. 054/055/064). Not a one-off cleanup; an ongoing habit for every future node added to either block.

**How to apply:** when migrating a new Z-Wave node in Junglinster, after adding its identity line to the relevant space's block, re-sort that whole block numerically by node ID before regenerating — same discipline in both the garage and server_room blocks, and any similar block that emerges elsewhere. Regeneration should stay byte-identical (pure reordering, confirmed via `diff -rq` on the `hass/` tree after the 033/042/067 reorder).
