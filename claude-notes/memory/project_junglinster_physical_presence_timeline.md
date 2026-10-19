---
name: project-junglinster-physical-presence-timeline
description: "Erik is on-site at Junglinster now (confirmed 2026-10-15, doing hands-on hardware work). Next scheduled physical task: the Zigbee antenna swap (Sonoff -> Nabu Casa ZBT-2 on protocols-server-1), planned for the weekend of 2026-10-24/25, not the coming one."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-15T11:40:00.828Z
---

Supersedes this memory's own earlier "~2026-10-19 return" prediction -- confirmed stale: Erik is
already physically at Junglinster as of 2026-10-15 (did the p-s-1 HP EliteDesk hardware migration,
BT dongle/bt-proxy setup, etc. hands-on this same day).

**Current known date for the next physical task**: the Zigbee antenna swap (retiring the old
Sonoff dongle for the Nabu Casa ZBT-2 on protocols-server-1 -- PROJECT.md's "Hardware in
Junglinster" item 3 checklist) is planned for the weekend of **2026-10-24/25**, explicitly NOT the
coming weekend (2026-10-17/18).

**Why this matters / a real mistake to avoid repeating**: `zigbee.container`'s quadlet already
references the ZBT-2's `/dev/serial/by-id/...` path and `adapter: ember` (staged ahead of time) --
but that staged CONFIG is not the same as the antenna swap having actually happened. Got this
wrong once already this session: assumed "Switch to new Zigbee antenna" was done purely from
seeing the ZBT-2 device path already wired into the live container config, and removed it from
PROJECT.md as complete -- user corrected immediately ("still to be done ... next weekend"). Staged
config referencing new hardware is NOT proof the physical swap/IEEE-transfer happened; check for
an explicit "done" statement from the user (or the actual old Sonoff being unplugged) before
marking a hardware task complete.

**How to apply:** don't assume a PROJECT.md hardware item is done just because a config file
already points at the eventual target hardware -- staging ahead of a physical swap is normal
practice in this project. Before removing a hardware-migration bullet as "done", get an explicit
confirmation.
