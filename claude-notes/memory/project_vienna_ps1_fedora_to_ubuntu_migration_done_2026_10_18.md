---
name: project-vienna-ps1-fedora-to-ubuntu-migration-done-2026-10-18
description: "DONE 2026-10-18: Vienna's p-s-1 (Pi4) migrated live from Fedora to Ubuntu -- containers, coordinator, smtpproxy, Samba all verified working end-to-end. Also: backup-home-automation off-box target moved Xanadu -> mqtt.erikproper.eu on BOTH houses. Several real gotchas found, reusable for Junglinster's own pending Ubuntu consolidation."
metadata:
  node_type: memory
  type: project
  modified: 2026-10-18T21:41:44.555Z
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
---

Executed physically on-site in Vienna: swapped the Pi4's boot SSD from the prepped Fedora disk to
the prepped Ubuntu one (remote prep done 2026-10-16, see
[[project_named_trigger_abstraction_step1_2026_10_15]]'s "Prep done remotely" section), booted
clean, then mounted the old Fedora SSD as secondary at `/mnt/old` to pull data/config across.
Direct SSH access confirmed working via the existing `~/bin/protocols-server-1.vienna` helper
(`erikp@vienna.homelinux.org -p 1970`) once on Vienna's own LAN -- no separate access setup
needed.

**Gotcha 1 -- stale LVM device-mapper table on first mount of the old disk.** `mount
/dev/systemVG/LVRoot /mnt/old` failed with "can't read superblock" / `Buffer I/O error on dev
dm-0, logical block 0, async page read`. Misleading error: `pvs`/`vgs`/`lvs` all reported the VG
healthy, and `pvck -v /dev/sdc3` read the PV's own LVM metadata cleanly -- the actual PV/metadata
were fine. Root cause: `dmsetup table` showed the LV's live kernel mapping pointed at major:minor
`8:19`, but the PV was actually at `8:35` (`/dev/sdc3`, the 3rd SCSI disk's 3rd partition) -- a
stale mapping left over from an earlier activation against a different device-enumeration order.
No USB/SCSI-level errors in `dmesg` at all (clean device attach) -- this is purely an LVM
activation-layer staleness, not a cabling/power issue. **Fix:** `vgchange -an systemVG &&
vgchange -ay systemVG` to force LVM to rebuild the table against the current device nodes, then
mount worked immediately. Generalizes to any LVM-backed disk remounted after being moved between
systems/USB ports/enumeration orders -- directly relevant to Junglinster's own pending HP EliteDesk
consolidation ([[project_junglinster_ps1_ps2_consolidation_plan]]), which will need the exact same
old-disk-remount step.

**Gotcha 2 -- quadlet-generated systemd units reject `systemctl enable`.** After copying the three
`.container` files (`homeassistant.container`, `mosquitto.container`, `zigbee.container`) to
`/etc/containers/systemd/` and `daemon-reload`, `systemctl enable --now homeassistant.service ...`
failed: "Unit ... is transient or generated." Quadlets don't work like ordinary unit files --
their own `[Install]` section is processed automatically by the podman-quadlet generator at every
`daemon-reload`/boot, materializing the want-link directly under
`/run/systemd/generator/multi-user.target.wants/` (confirmed present, correctly persisted). The
fix is simply never calling `systemctl enable` on them -- `daemon-reload` + plain `systemctl
start` is both necessary and sufficient; enablement is already handled.

**What migrated cleanly, no gotchas:** the three containers' bind-mounted data
(`/var/lib/home-automation/{homeassistant,mosquitto,zigbee}`, via `rsync -aHAX --numeric-ids`,
272M+1.9M+84M) -- numeric UID/GID preservation (mosquitto's container-internal uid 1883) needed no
matching host user, it "just worked". The coordinator migrated via its own existing
`~/coordinator/deploy.vienna` script unchanged (copies `~/go/src/homeassistant` +
`~/coordinator`'s config/state, then `go build` + install to `/usr/local/bin` + systemd unit --
exactly the deploy path Junglinster's own `deploy.junglinster` already uses, confirmed by that
script's own header comment). smtpproxy rebuilt fresh from source
(`~/go/src/smtpproxy`, via the newly-installed official `golang-go` apt package) rather than
copying the old binary -- config (`smtpproxy.ini`) and the existing Let's Encrypt cert
(`~/ssl/{fullchain,privkey}.pem`, valid until 2026-12-06) copied over directly. The Zigbee USB
dongle's `/dev/serial/by-id/usb-Silicon_Labs_Sonoff_Zigbee_3.0_USB_Dongle_Plus_0001-if00-port0`
symlink was already present identically on the fresh Ubuntu install, so the zigbee2mqtt quadlet's
`AddDevice=` line needed zero edits.

**Samba gotcha (separate from the container/coordinator work, same session):** the `[music]` share
config and `/srv/samba/music` directory were already correctly pre-staged from the 2026-10-16
remote prep, and `smbd` was active -- but login still failed with "the usual password." Root
cause: `pdbedit -L` was completely empty -- no Samba account existed at all. Samba keeps its own
password database (`tdbsam`), entirely separate from the Unix login password; a `valid users =
erikp` share entry alone creates no implicit account. **Fix:** `sudo smbpasswd -a erikp`
(interactive, run directly by the user rather than scripted over SSH, since it's a password
prompt). Confirmed working immediately after, no service restart needed.

**End-to-end verification, not just "service active":** `sudo podman ps` (NOT plain `podman ps` --
that shows the calling user's own rootless namespace, empty here since these are root-level
quadlets) showed all three containers genuinely running; journal logs showed live MQTT traffic
(mosquitto real client connections, zigbee2mqtt publishing real device state); HA's own `mqtt`
config entry and 483 real `platform:"mqtt"` entities in `core.entity_registry` (including
`binary_sensor.apartment_living_room_philips_motion_occupancy`, matching a device caught live-
publishing in zigbee2mqtt's own log at the same moment) confirmed the full chain -- zigbee2mqtt to
mosquitto to HA's MQTT integration to the entity registry -- not just individual services up in
isolation. One ERROR-level HA log line (`habluetooth.manager: Missing required permissions for
Bluetooth management`) is harmless noise from HA's own container-level Bluetooth manager finding
no usable adapter -- **confirmed Vienna has zero Bluetooth setup anywhere in its own `.def` files**
(grepped, zero matches); don't confuse this with Junglinster's real `frame`-based bt-proxy
([[project_living_room_bt_proxy_plan]]), which is Junglinster-only and was wrongly extrapolated to
Vienna once during this same session before being corrected.

**smtpproxy end-to-end confirmed same session:** a real test mail
(`henderik.proper@tuwien.ac.at` -> `erikproper@fastmail.com` via
`mail.intern.tuwien.ac.at:587`) was sent through the rebuilt proxy, logged as forwarded
correctly, and the user confirmed actual delivery into the Fastmail inbox -- the freshly rebuilt
binary + copied TLS cert + copied `smtpproxy.ini` all work together, not just "the process starts."

**Backup-home-automation off-box target migrated Xanadu -> mqtt.erikproper.eu, same session
(both houses, not just Vienna):** the shared `/usr/local/bin/backup-home-automation` script
(config-driven, identical on every protocols-server-1/2) had its destination *path* hardcoded to
`/mnt/yotta7/HomeAutomation/...` (Xanadu's own RAID mount) even though host/port were already
externalized to `/etc/backup-home-automation.conf` -- added a new `BACKUP_DEST_ROOT` config var
and renamed `XANADU_HOST`/`XANADU_PORT`/`XANADU_OK` to generic `OFFBOX_*` (the target is no longer
necessarily Xanadu). New root on `mqtt.erikproper.eu`: `/home/erikp/backups` (matches the
structure of the user's own preparatory weekend sync -- `$INSTALLATION/$HOSTNAME_SHORT/`, no extra
`HomeAutomation` segment). Deployed fresh on Vienna's new box (nothing existed there yet) and
updated in-place on Junglinster's already-running p-s-1 (script + conf only). **Real gotcha, not a
connectivity problem:** the service runs as root (no `User=` in the unit), and root had no
`known_hosts` file at all on either box -- `BatchMode=yes` host-key verification failed silently
as "unreachable" even with the right private key present. Fixed via `ssh-keyscan >>
/root/.ssh/known_hosts` for both `mqtt.erikproper.eu:22` AND (for the separate "mirror the main HA
instance's own backup folder" leg) each house's own `<house>.homelinux.org:1967` HAOS SSH
endpoint -- two different host-key gaps, not one. Root's *key* also had to be copied from the old
Fedora disk to Vienna's new box (comment `root@protocols-server-1-vienna-backup`) and added to
`mqtt.erikproper.eu`'s `~erikp/.ssh/authorized_keys`; Junglinster's own root key
(`erikproper@fastmail.com`, a personal key reused here) was already trusted there.
**Confirmed end-to-end, both houses' container-backup leg** (tarballs landed at
`~/backups/<house>/protocols-server-1/*.tar.gz` on `mqtt.erikproper.eu`, verified directly).
**Main-HA-instance mirror leg: Vienna confirmed working, Junglinster still failing** --
"permission denied," not a host-key problem this time: Junglinster's own HAOS Terminal & SSH
add-on never had p-s-1's root key added to its *own* `authorized_keys`, and that add-on's config
is UI-only (Settings -> Add-ons -> Terminal & SSH -> Configuration -- see
[[project_ha_entity_registry_collision_lessons]]'s "HAOS SSH is restricted" note), not a plain file
editable over SSH. Left for the user to add `ssh-ed25519
AAAAC3NzaC1lZDI1NTE5AAAAIGgfy/OxPqTsHq2JWhR5bz2RjLQd5+rBVAejTx8Ux11R erikproper@fastmail.com` there
themselves.

**Junglinster's HAOS Terminal & SSH add-on key added by the user, confirmed working same
session:** re-ran the backup on Junglinster after the user added p-s-1's root key via the add-on's
own Configuration tab -- main-HA-instance mirror leg now succeeds there too. Both houses now fully
green on both legs.

**Xanadu's old `/mnt/yotta7/HomeAutomation` backup tree deleted, same session, after a real gap
check:** before deleting, diffed Xanadu's content against `mqtt.erikproper.eu`'s and found two
real (not just timing-skew) gaps -- `vienna/protocols-server-1` had 14 days (Oct 4-17) on Xanadu
from the old Fedora job that only mirrored there, never carried over by the user's weekend
preparatory sync (which predated them); `junglinster/main` had 3 already-rotated-elsewhere HAOS
tars (Oct 9-11) that no longer exist in HAOS's own live `/backup` folder either, so a fresh mirror
could never recreate them. Flagged both explicitly rather than assume a size/date mismatch was
harmless skew. User's call: fine to lose both (rolling safety-net snapshots, not primary data) now
that live backups are confirmed working on both legs, both houses. Deletion itself hit one more
gotcha: `/mnt/yotta7` is root:root `755` -- `erikp` owns everything *inside*
`HomeAutomation` (could delete all its contents) but can't unlink the now-empty directory from its
root-owned parent without `sudo` (erikp is in the `sudo` group there). 5.4G freed.

**One more known_hosts gap, found running the real `./deploy` for the first time post-migration:**
`cpu/deploy.vienna` (Integrations/cpu, Nextcloud-synced) runs ON protocols-server-1 and
self-references it by bare LAN hostname (`DoDist erikp protocols-server-1`, a multi-host fleet
push pattern) -- `erikp`'s own `known_hosts` on the fresh Ubuntu box had never trusted that bare
hostname (only the FQDN forms used from outside the LAN). Same `ssh-keyscan >> ~/.ssh/known_hosts`
fix as the other gaps above. Running a full real `./deploy` (not just spot-checking individual
services) is what surfaces these -- worth doing once, deliberately, after any future Ubuntu
migration rather than assuming the earlier targeted checks caught everything. Separately, the
real `./generate` + `diff -rq` against a pre-migration snapshot of `hass/vienna/` confirmed the
whole named-trigger/`at <leaf>`/dim-brighten rule migration ([[project_named_trigger_abstraction_step1_2026_10_15]])
produces byte-identical output -- deployed live same session, zero behaviour change.

**Sonos-via-Samba gotcha, found live same day, fixed on BOTH houses' p-s-1:** Sonos speakers only
ever speak SMB1/NT1 -- Ubuntu's Samba 4.23.6 defaults to `server min protocol = SMB2_02`,
outright refusing SMB1 negotiation (confirmed via `smbstatus` showing zero Sonos connections even
attempted, not a performance/stutter problem). Fedora's own Samba build defaults more
permissively, which is why this never surfaced before the migration -- real regression, not
pre-existing. Fix: add `server min protocol = NT1` to `[global]` in `/etc/samba/smb.conf` (floor
only, doesn't force other clients down) + `systemctl restart smbd nmbd`. Applied to Vienna's p-s-1
reactively (after the user noticed Sonos streaming failing) and proactively to Junglinster's own
p-s-1 (same Ubuntu Samba version, `[music]` share literally commented "Music for Sonos" --
confirmed same `SMB2_02` default there too, not yet actually relied on for real Sonos streaming
since the real music disk doesn't land there until Friday, but would have hit the identical wall).
Relevant to any future Fedora->Ubuntu Samba migration, both houses.

**Explicitly NOT done yet:** moving Let's Encrypt renewal duty (currently a Fedora
`certbot-renew.timer`) to the new Ubuntu box before the current cert expires 2026-12-06; actually
decommissioning the old Fedora SSD (user is handling format + music copy themselves, not yet
done as of this memory).

**How to apply:** when Junglinster's own Pi4(s)-to-HP-EliteDesk/Ubuntu consolidation
([[project_junglinster_ps1_ps2_consolidation_plan]]) actually happens, re-read this memory first --
both LVM-remount and quadlet-enable gotchas will very likely recur there too, and the overall
sequence (swap boot disk, mount old as secondary, rsync data with `--numeric-ids`, redeploy via the
project's own existing deploy script, rebuild Go binaries fresh rather than copying, verify via
`sudo podman ps` + real log traffic + entity-registry counts rather than trusting `systemctl
is-active` alone) is the proven template to repeat.
