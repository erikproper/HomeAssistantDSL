---
name: project-junglinster-ps1-hardware-failure-2026-10-13
description: "p-s-1@Junglinster (Pi4 + external SSD) hit a bus error mid-Fedora-update on 2026-10-13; a power reboot by the user's wife didn't recover it. User checks physically on Thursday 2026-10-15. Lesson learned: Ubuntu LTS (not Fedora, not non-LTS Ubuntu) is the standard for these always-on server boxes going forward."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-15T09:22:37.546Z
---

**Incident, 2026-10-13**: protocols-server-1 at Junglinster (Raspberry Pi 4 + external SSD) hit a
"bus error" while updating Fedora. The user's wife did a power-cycle reboot on-site, but it did not
recover. The user is away and can only check it physically on **Thursday (2026-10-15)**.

**Hardware setup**: same as Vienna's own p-s-1 -- a Pi4 with an external SSD. Radios (Zigbee2MQTT
dongle etc.) are on a powered USB hub; the SSD is connected directly (not through the hub). Open
question the user still wants to figure out: what actually went wrong, and whether this failure
mode is shared with Vienna's identical setup (i.e. a latent risk there too, not just a one-off).

**How to apply**: p-s-1@Junglinster runs the coordinator, MQTT broker, Zigbee2MQTT, and Z-Wave for
that house ([[project_vienna_ps2_ha_migration_and_junglinster_decision]] has the baseline load
comparison between the two houses' p-s-1 boxes). Until this is physically diagnosed, treat
Junglinster's coordinator/MQTT/Zigbee/Z-Wave as degraded or down. See
[[project_junglinster_ps1_ps2_consolidation_plan]] for the longer-term hardware decision this
triggered. Don't propose remote-only fixes assuming SSH access works -- the box may not even be
reachable; confirm reachability first before any diagnostic step.

**Pending fix once this box is back**: [[project_ping_report_ipv6_fallback_bug]] -- a known,
already-diagnosed latent bug in `~/ping/report` (IPv6-first ping never falls back to IPv4) exists
identically on Junglinster's own protocols-server-1, confirmed by the user but not yet patchable
since that host is down. Apply the same one-line `-4` fix there once it's reachable again.

**Resolution, 2026-10-15 (physical check day, as planned)**: the Pi4's root filesystem is
confirmed unrecoverable -- "mounts, checks, but the libraries and kernel got out of sync" -- user
has given up repairing it in place. The Pi4 and its SSD are themselves still physically fine
(confirmed by the user) -- only the Fedora rootfs is bad, cause not fully pinned down (maybe a
power glitch, maybe a botched update sequence). This hardware is NOT scrapped: it's still the
planned future upgrade for Vienna's own p-s-1 once wiped and reinstalled with Ubuntu (see
[[project_junglinster_ps1_ps2_consolidation_plan]]'s own 2026-10-15 correction -- don't assume
that leg of the plan is off just because the current rootfs is dead). The replacement HP EliteDesk
(see
[[project_junglinster_ps1_ps2_consolidation_plan]]) has arrived, is installed with Ubuntu, and is
**already live as the new protocols-server-1** (fresh server install, reachable as usual under
that name) -- this happened faster than the plan's own staged "1a" naming step anticipated; it
went straight in as "protocols-server-1" itself, replacing the Pi4 directly. The old Fedora rootfs
is mounted read-only-ish at `/mnt/temp` on the new box, specifically to copy files out of the old
containers before it's gone for good.

**Recovery order, as the user laid it out**: (1) resurrect the coordinator and Z-Wave containers
(were running on Fedora/podman) -- highest priority, these are in active use; (2) resurrect the
zigbee2mqtt and MQTT-broker containers -- lower priority, these were "preliminary", not yet in
operational use pending the Zigbee antenna migration (a separate, later-staged piece of work), so
getting them running again is prep, not urgent; (3) get the container backups running again.

**How to apply**: don't propose restoring anything onto the Pi4 -- it's confirmed dead/abandoned,
not a repair target anymore. When helping with the coordinator/Z-Wave recovery, expect SSH
target/port changes in `Junglinster/deploy.d/*` (same shape as Vienna's own p-s-2->p-s-1 move --
see [[project_vienna_ps2_ha_migration_and_junglinster_decision]] for that precedent) once the new
box's own access details are confirmed. The old Fedora rootfs at `/mnt/temp` is a source to copy
FROM (configs/secrets/data for the containers being resurrected), not something to preserve
long-term -- don't suggest keeping it mounted indefinitely once the relevant files are recovered.

**Lesson learned (user, 2026-10-13)**: Ubuntu's LTS releases are better suited for these
server-oriented always-on boxes than Fedora's own release cadence (Fedora's non-LTS, faster
release/update cycle is read as a contributing factor in this failure, hitting mid-update). When
any future hardware/OS plan in this project says "Ubuntu" (e.g.
[[project_junglinster_ps1_ps2_consolidation_plan]], PROJECT.md's "Hardware in Junglinster"
section), that means **Ubuntu LTS** specifically, not the latest non-LTS release -- don't suggest
or assume a non-LTS Ubuntu release for any of these boxes.
