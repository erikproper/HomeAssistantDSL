---
name: project-samba-plex-xanadu-to-ps1-migration-plan
description: "Samba+Plex test DONE 2026-10-16 on protocols-server-1@Junglinster (500GB ex-p-s-2 SSD mounted at /mnt/media, music at /mnt/media/music, both native not containerized). Container-backups-to-mqtt.erikproper.eu item still NOT started."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-16T07:40:13.488Z
---

**DONE 2026-10-16**: Samba + Plex (music only) now running natively on
protocols-server-1@Junglinster, testing on the 500GB SSD salvaged from the former
p-s-2@Junglinster hardware (mounted at `/mnt/media`, 916G total; music library at
`/mnt/media/music`, confirmed already populated with real artist folders -- 64G used/806G free
at the time). Deliberately NATIVE, not podman-containerized (confirmed with the user: Samba needs
direct host filesystem permission/ACL access with no benefit from containerizing; Plex here is
music-only, so its main container use case -- GPU passthrough for video transcoding -- doesn't
apply; both are well-supported native Ubuntu packages, unlike this box's OTHER services
(coordinator/zigbee2mqtt/zwave-js-ui/mosquitto/homeassistant/caddy/bt-proxy), which are
podman quadlets specifically because those are Docker-first upstream projects).

**Samba setup**: `apt-get install samba`. Added a `[music]` share to `/etc/samba/smb.conf`
(mirroring xanadu's own existing share block, path swapped to `/mnt/media/music`):
```
[music]
  comment = Music for Sonos
  browsable = yes
  path = /mnt/media/music
  guest ok = no
  read only = no
  create mask = 0775
  directory mask = 0775
```
(mask bumped from xanadu's own `0700` to `0775` -- `0700` would have made every new file/folder
written through the share accessible ONLY to whichever single Samba-mapped Unix user created it,
locking out even `erikp`'s own SSH access to the same files.) Real gotcha: `smbpasswd -a <name>`
requires a PRE-EXISTING Unix account of that exact name (Samba's default passdb backend maps 1:1
onto Unix/NSS accounts) -- `smbpasswd -a music` failed ("Failed to add entry") until `useradd
--system --no-create-home --shell /usr/sbin/nologin music` was run first. That `music` Unix user
also needed adding to `erikp`'s own group (`usermod -aG erikp music`) plus `chmod -R g+w` +
`chmod g+s` on every directory under `/mnt/media/music`, since the share directory was already
owned `erikp:erikp` 0755 -- without this, writes through the `music` Samba login would have
failed on a real permission mismatch, not just a smb.conf setting. `smbpasswd -a music` itself
needs a real interactive TTY (failed over a non-interactive SSH -t call) -- the user ran it
directly in their own already-open root shell. Confirmed working end to end afterwards
(`erikp`/`music` both show up in `pdbedit -L`, `smbd` active, user connected successfully).

**Plex setup**: no official Ubuntu/Debian repo preinstalled -- added Plex's own apt repo first
(`https://downloads.plex.tv/repo/deb public main`, keyring at
`/usr/share/keyrings/plex-archive-keyring.gpg`), then `apt-get install plexmediaserver`
(1.42.2.10156 at install time). `plex` system user already has read access to
`/mnt/media/music` with no extra permission work needed (directory's `o+rx` from the 0755
default covers it). One-time claim/library setup done via the user's own browser
(`http://<lan-ip>:32400/web`, needs their real Plex account sign-in -- not SSH-scriptable).
**Known, deliberately not chased**: Plex's in-app "update available" notice points at a build
newer than what Plex's own "public" apt channel currently offers (`apt-cache policy
plexmediaserver` showed Installed == Candidate, nothing newer in the repo yet) -- decided to let
a normal `apt-get upgrade` pick it up naturally once Plex publishes it there, rather than manually
downloading a `.deb`.

**Still NOT done** (separate item raised same day, not yet started): moving the home-automation
CONTAINER backups off xanadu to mqtt.erikproper.eu -- tracked as its own PROJECT.md bullet
("Hardware in Junglinster" section), confirmed as part of this same work batch but not actually
touched yet.

**How to apply if this continues**: the actual live CUTOVER from xanadu (vs. this test) -- syncing
the real full music library over, pointing Sonos/other clients at the new share, decommissioning
xanadu's own Samba/Plex -- has NOT happened yet; this was explicitly framed as "see if it works"
testing only.
