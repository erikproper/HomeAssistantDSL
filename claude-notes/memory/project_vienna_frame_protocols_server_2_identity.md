---
name: project-vienna-frame-protocols-server-2-identity
description: "In Vienna, \"frame\" and \"protocols-server-2\" are the same physical machine, not two separate hosts. DONE (2026-09-07/08): now Fedora/user erikp; coordinator moved off it to protocols-server-1, mirroring Junglinster's own topology."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-08T06:54:18.809Z
---

Confirmed by the user (2026-09-07), after I flagged it as an odd-looking finding while auditing
`~/bin` host/port scripts before a deploy: `frame.vienna`, `protocol-server-1.vienna`, and
`protocol-server-2.vienna` all currently resolve to the identical `vienna.homelinux.org:1971`
forward not because of stale/duplicate scripts, but because **`frame` and `protocols-server-2` are
genuinely the same box in Vienna** today -- matches PROJECT.md's own hardware-migration plan
(item 2's "Setup P-S-2 for Vienna: { picture frame, HA, ... }"), just not spelled out explicitly as
an identity anywhere before now.

**Why:** Vienna's hardware footprint is smaller than Junglinster's -- one Pi currently plays both
the "picture frame" (commandline integration target) and "protocols-server-2" (secondary
integration-adapter host, e.g. the Netatmo-via-HA bridge) roles at once. Junglinster does NOT share
this identity -- its `frame.junglinster` (port 1968, user pi) and `protocol-server-2.junglinster`
(port 1971, user erikp) are genuinely separate hosts.

**Current state vs. planned**: today, RaspberryOS, user `pi`. After the Pi-stick migration
(PROJECT.md's Raspberry Pi fleet storage migration plan, step 3/5) moves Vienna's protocols-server-2
onto a fresh stick, it becomes Fedora, user `erikp` -- matching Junglinster's own
protocols-server-2 convention (Fedora/Podman/Quadlet, `erikp` user). The `pi@`/RaspberryOS
convention on that box is a transitional state, not the target.

**How to apply:** don't assume `frame` and `protocols-server-2` are independently deployable hosts
for Vienna the way they are for Junglinster -- a deploy script touching one is touching the same
physical machine as the other. When the Fedora/erikp migration lands, expect `frame.vienna`'s own
`~/bin` script (and any deploy.d script hardcoding `pi@vienna.homelinux.org`) to need updating to
`erikp@` -- revisit [[project_vienna_coordinator_on_frame]] and the Vienna `deploy.d/0{2,3,4}_deploy.*`
scripts at that point, not just the port.

**DONE (2026-09-08)**: `protocols-server-1` for Vienna also migrated onto Fedora, and the
coordinator role moved there — off `frame`/`protocols-server-2` entirely.
`Vienna/deploy.d/02_deploy.coordinator` now targets `erikp@vienna.homelinux.org:1970`; its own
on-target build script was renamed `coordinator/deploy.vienna` (was `deploy.frame`, now misleading
since the coordinator no longer runs there). Coordinator stopped+disabled and its
binary/unit/`~/coordinator` artifacts removed from protocols-server-2 — but its Go toolchain and
`~/go/src/homeassistant` checkout were deliberately KEPT there (see below), so this was a narrower
cleanup than a full "remove Go" pass. Vienna's final topology now mirrors Junglinster's: coordinator
on protocols-server-1 (`erikp@`), `frame`/protocols-server-2 left running only the picture-frame
`mqtt_commandline` daemon.

**Go stays on protocols-server-2, deliberately** — `mqtt_commandline`'s own `deploy.frame` still
builds in place from `~/go/src/homeassistant/mqtt_commandline` there (matches Junglinster's own
frame convention, which also builds in place). The user explicitly overrode an earlier
cross-compile-and-ship-a-binary attempt: "If go is needed for the mqtt_commandline, then... it
needs to stay on p-s-2 as well. No cross compiling, please." Don't revisit this without asking
again.

**Bug found+fixed along the way**: `mqtt_commandline.service` was still `User=pi`/`/home/pi/...`
after the erikp/Fedora migration, crash-looping (`status=217/USER`) until fixed. The shared
`Integrations/mqtt_commandline/mqtt_commandline.service` template (used by BOTH houses) now carries
a `@@CMDLINE_USER@@` placeholder rather than a hardcoded login, substituted per house in each
house's own `deploy.d/0*_deploy.commandline` script (`erikp` for Vienna, `pi` for Junglinster,
which is still RaspberryOS) — needed since the two houses no longer share the same login for this
one file.
