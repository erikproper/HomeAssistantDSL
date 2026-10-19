---
name: project-xanadu-repurposing-idea-2026-10-13
description: "Floated 2026-10-13, NOT decided: once Vienna's p-s-1 Ubuntu migration lands (the retired Junglinster Pi4's 2TB SSD reused there), repurpose that SSD as SMB/Plex music storage for remote access. Xanadu (the user's Plex/backup machine, reachable via ~/bin/xanadu) would then only host raid backups (the 'yotta' LVM volumes) and the Plex video collection, and get powered off except when actually watching video or running a backup. Container backups currently on xanadu would need a new home, e.g. the cloud-hosted mqtt.erikproper.eu."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-13T16:09:11.307Z
---

**Idea, not yet decided (user's own words: "food for thought")**: once
[[project_junglinster_ps1_ps2_consolidation_plan]]'s Vienna leg lands -- the retired Junglinster
Pi4+SSD, upgraded to Ubuntu, becoming Vienna's new p-s-1 -- the user is considering repurposing
that 2TB SSD as music storage served over SMB and Plex, for remote access to the music collection.

**Knock-on effect on xanadu** (the user's main Plex/backup machine, reachable via `~/bin/xanadu` ->
`ssh erikp@junglinster.homelinux.org -p 1969`, Core i7-7700T/4c8t/16GB, a large multi-disk LVM
array under `/mnt/yotta*` plus several directly-mounted disks): it would then only need to host
the primary/raid backups (the "yotta" volumes) and the Plex *video* collection (music moving off
it to the new SSD) -- and could be powered off except when actually watching video or running a
backup, rather than staying always-on.

**Open consequence flagged by the user**: container backups (this project's own coordinator/
generator state, etc.) currently live on xanadu too -- if xanadu is normally powered off, those
backups need a new home. The user's own suggestion: `mqtt.erikproper.eu`, a cloud-hosted machine
already part of this project's own cloud-broker vocabulary (Architecture.md's cloud-broker
federation mechanism, §12).

**mqtt.erikproper.eu's own free space, checked 2026-10-13**: `df -h .` on that host shows
`/dev/sda3` at 39G total, 5.9G used, **31G available** (16% used).

**xanadu's current backup set, checked 2026-10-13** (`/mnt/yotta7/HomeAutomation`, `du -h`):
**~13G total** -- Vienna 5.2G (`main` 2.7G, `protocols-server-1`/`protocols-server-2` 1.3G each,
`frame` 64K), Junglinster 7.0G (`main` 3.4G, `protocols-server-1` 46M, `protocols-server-2` 3.6G).
Comfortably fits mqtt.erikproper.eu's 31G free (~42% used, ~18G spare). One easy trim if more
margin is ever wanted: `junglinster/main/config/zigbee2mqtt/log/` alone is 169M of old dated
debug-log directories (Aug 2026) -- pure historical logs, not config/state, a reasonable exclusion
candidate regardless of the space question.

**How to apply**: this is speculative, sequenced strictly AFTER Vienna's own p-s-1 Ubuntu migration
(itself sequenced after Junglinster's HP-box consolidation) -- don't treat it as active or start
designing for it until the user says it's actually happening. If/when it is, the container-backup
relocation to mqtt.erikproper.eu is a real open question to resolve (not yet designed at all),
separate from the music/Plex SMB serving itself.
