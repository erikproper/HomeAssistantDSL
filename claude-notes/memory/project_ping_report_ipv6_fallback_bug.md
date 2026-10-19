---
name: project-ping-report-ipv6-fallback-bug
description: "Real bug found live 2026-10-14 on Vienna's protocols-server-1: the hand-maintained ~/ping/report cron script's bare `ping` call resolves a LAN hostname to an unreachable IPv6 address and never falls back to IPv4, so it publishes a false 'not alive' node/state for hosts that are actually fine. Fixed on Vienna (added -4). Junglinster's own protocols-server-1 has the identical script and the identical latent bug, confirmed by the user, but can't be patched until p-s-1@Junglinster is back up."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-14T06:29:53.646Z
---

**The bug**: `~/ping/report` (a hand-maintained cron script on protocols-server-1, NOT part of
this Go generator/repo at all -- only the list of hosts it pings, `ping/hosts`, is generator-
written, via `generatePingHostsFile`) runs `ping -c 1 -W 2 "$hostname"` with no address-family
flag. When `getent hosts <name>` resolves to an IPv6 address that's currently unreachable (seen
live: Vienna's `frame` resolved to `fdf5:852c:f4e:0:d2c0:2944:8421:2019`, "Destination
unreachable"), `ping` never falls back to IPv4 even though the IPv4 address
(`192.168.178.46`) answers instantly (0.6ms). The script then publishes `hosts/<name>/node/state
= false` on a perfectly healthy host, every 10 minutes, with no visible error anywhere.

**Real symptom this caused (Vienna, 2026-10-14)**: the picture frame's HA switch
(`switch.social_apartment_living_room_picture_frame`) showed `unavailable` despite the frame
genuinely being on and its own `mqtt_commandline` daemon reporting fine -- because that switch's
availability has `dependency on node.picture_frame` (Physical.def), and `node.picture_frame`'s own
liveness comes from exactly this ping script. Confirmed as the sole root cause: forcing IPv4
(`ping -4 -c 1 -W 2`) immediately fixed it, verified via the live HA API (`binary_sensor...node`
and the switch itself all flipped to `on` within seconds of a manual re-run).

**Fixed live on Vienna's protocols-server-1**, 2026-10-14: edited `~/ping/report` directly on the
host (backed up first as `~/ping/report.bak_2026-10-14`), changed the ping call to `ping -4 -c 1
-W 2 "$hostname"`. Verified immediately via the live HA API: all three dependent entities flipped
from unavailable/off to on within seconds of a manual re-run.

**The real source of truth is `/Users/erikproper/SmartLiving/Integrations/ping/report`** -- a
SHARED file both houses' own `deploy.d/03_deploy.hosts` step rsyncs from (first locally into each
house's own `./ping/report`, excluding `hosts`/`secrets`, then from there onto that house's live
protocols-server-1 over SSH). Fixed there too, 2026-10-14, same one-line `-4` change -- this is
what makes the fix "the" fix rather than a Vienna-only patch, and what any future `./deploy` on
either house will push from.

**Staged but NOT deployed for Junglinster**, 2026-10-14, per explicit user instruction ("apply
fix, but do not deploy"): both `/Users/erikproper/SmartLiving/Vienna/ping/report` and
`/Users/erikproper/SmartLiving/Junglinster/ping/report` (the local staged copies) were refreshed
from the fixed shared file via the SAME local-only rsync line `03_deploy.hosts` itself uses
(`rsync -arv --exclude hosts --exclude secrets ../Integrations/ping/ ./ping` -- no `-e ssh`, purely
local, confirmed to touch no remote host). Junglinster's own protocols-server-1 is unreachable
right now anyway ("No route to host", confirmed 2026-10-14) from
[[project_junglinster_ps1_hardware_failure_2026_10_13]] -- nothing was or could be pushed there.

**How to apply**: once p-s-1@Junglinster is reachable again (temporary fix or the new HP-box
consolidation), the fix is already staged and ready -- just run Junglinster's own
`./deploy 03_deploy.hosts` (or the full `./deploy`, which runs every `deploy.d/*` file regardless
of which one you name -- see the `./deploy` script's own `for s in deploy.d/*` loop) to push it
live. No further editing needed, don't re-diagnose this.
