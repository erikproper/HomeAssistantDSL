---
name: project-junglinster-ps1-ps2-consolidation-plan
description: "The authoritative, multi-stage hardware plan lives in PROJECT.md's own '## Hardware in Junglinster' section -- read THAT first, this memory only adds the 2026-10-13 trigger/status context. Junglinster's p-s-1+p-s-2 containers move onto a new HP EliteDesk mini PC (Ubuntu); the retired Junglinster Pi4+SSD then gets reused to upgrade Vienna's own p-s-1 to Ubuntu too."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-15T09:22:27.056Z
---

**The real plan lives in PROJECT.md's `## Hardware in Junglinster` section -- read it directly,
don't rely on this memory's own paraphrase.** As of 2026-10-13 it already lays out the full
multi-stage sequence: new HP box in as "1a" (Ubuntu) -> containers from both protocols-server-2
and protocols-server-1 move onto it -> rename "1a" to "1" -> MQTT broker moves into a container on
it -> Zigbee antenna swap -> **the retired Junglinster Pi4+Pi3 (former p-s-1/p-s-2) and their disks
go to Vienna** -> Ubuntu installed on that Pi4 -> it becomes Vienna's *new* p-s-1 (including
music) -> Vienna's *old* p-s-1 kept at home as a spare/backup machine. Confirmed directly by the
user (2026-10-13): "once the existing pi-4 + SSD in Junglinster is freed up, I will use it to
install Ubuntu on the SSD and then use that to update the pi-4 in Vienna to Ubuntu as well" --
i.e. this is not a new idea, it's this same pre-existing plan being triggered now.

**Why triggered now**: [[project_junglinster_ps1_hardware_failure_2026_10_13]] -- a Pi4 +
external-SSD bus error during a Fedora update that a power-cycle didn't fix. Junglinster is
treated as more urgent than Vienna (the user's wife lives there full-time), so this accelerates
the Junglinster side of the plan specifically. The new hardware for that side: a refurbished HP
EliteDesk 800 G4 DM mini PC (alternate.de product 100163379, ordered), internal SSD, Ubuntu. The
user's own expectation (2026-10-13): this box should be powerful enough to run all of
Junglinster's primary containers -- named explicitly: zigbee2mqtt, zwave2mqtt, mqtt (the broker),
and ha2mqtt (the coordinator/bridge). The Green (HAOS) keeps running the frontend only once these
move off both p-s-1 and p-s-2.

**Supersedes** [[project_vienna_ps2_ha_migration_and_junglinster_decision]]'s 2026-09-21 decision
to keep Junglinster's p-s-2 HA instance separate from p-s-1 -- that reasoning doesn't apply once
both move together onto new, more powerful hardware.

**Status, 2026-10-15 (updated)**: HP box arrived, Ubuntu installed, and already live as the new
protocols-server-1 -- faster than planned, it skipped the staged "1a" interim name and went
straight in as "protocols-server-1" itself (see
[[project_junglinster_ps1_hardware_failure_2026_10_13]]'s own 2026-10-15 update for the full
status). The old Pi4's root filesystem turned out unrecoverable (kernel/library mismatch) and has
been abandoned, its old rootfs mounted read-only at `/mnt/temp` on the new box purely to copy
files out before it's gone. Recovery now in progress in this order: (1) coordinator + Z-Wave
containers (active/urgent), (2) zigbee2mqtt + MQTT broker containers (prep only, not yet
operational pending the antenna migration), (3) container backups. Not yet designed in DSL/
Architecture.md terms -- expect deploy.d SSH targets to change (same shape as the Vienna
p-s-2->p-s-1 move, see [[project_vienna_ps2_ha_migration_and_junglinster_decision]] for that
precedent) once the new box's own access details are confirmed, plus an Architecture.md §11
deployment-topology update.

**Correction, 2026-10-15**: the retired Pi4+SSD is NOT dead hardware -- the user confirmed both
are physically functional; only the Fedora *rootfs* got corrupted (kernel/libraries out of sync
after an update, possibly a power glitch or a botched update sequence, cause not fully
determined). The LATER leg of this plan (wipe it, install Ubuntu, reuse it to upgrade Vienna's own
p-s-1) is therefore still entirely valid and NOT superseded -- the earlier note in this memory
suggesting it might need rethinking was wrong; disregard it.

**How to apply**: don't propose restoring/fixing p-s-1@Junglinster's Pi4+SSD setup as a permanent
fix -- it's being retired from that role (to Fedora->Ubuntu, then to Vienna), not repaired into it.
Before the HP box arrives, p-s-1@Junglinster may need a *temporary* workaround (the user's own
words: "a temporary solution") -- ask what's actually needed rather than assuming full recovery is
the goal. Always check PROJECT.md's `## Hardware in Junglinster` section fresh before acting on
this plan -- it's the user's own living document and may have moved on since this memory was
written.
