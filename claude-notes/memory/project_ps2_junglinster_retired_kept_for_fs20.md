---
name: project-ps2-junglinster-retired-kept-for-fs20
description: "protocols-server-2@Junglinster is powered off / fully idle (confirmed 2026-10-15: zero containers, all its roles moved to the new p-s-1 HP EliteDesk), but stays declared in Physical.def/Conceptual.def and physically kept, not decommissioned -- earmarked for a future FS20 experiment."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-15T12:07:35.662Z
---

**State**: protocols-server-2@Junglinster (the old hardware, port 1971) has zero podman containers
running (`sudo podman ps -a` empty) -- everything it used to host (ha2mqtt, Caddy) migrated onto
the new p-s-1 HP EliteDesk earlier this session. Only stock Fedora OS services remain. The user
confirmed it can be switched off.

**Not decommissioned**: its `node.protocols_server_2` liveness/cpu-load/cpu-temperature
declarations stay in Physical.def (line ~486) and Conceptual.def (lines ~253-256) -- explicitly
NOT removed, and the hardware itself is being kept, not freed up/repurposed/sent to Vienna like
the old p-s-1 Pi4+pi3 hardware (see PROJECT.md "Hardware in Junglinster"). Powering it off will
make its binary_sensor "node" go unavailable, which is expected/harmless -- nothing else depends
on its liveness.

**Why**: earmarked for a future "FS20 experiment" (FS20 = an older German home-automation RF
protocol/device family) -- no further detail given yet.

**How to apply**: don't suggest removing p-s-2's DSL declarations or repurposing/freeing its
hardware as part of any future cleanup -- it's intentionally parked, not abandoned. If the FS20
experiment comes up later, this is the hardware already earmarked for it.
