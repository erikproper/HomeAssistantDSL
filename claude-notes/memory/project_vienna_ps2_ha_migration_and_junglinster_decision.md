---
name: project-vienna-ps2-ha-migration-and-junglinster-decision
description: "DONE 2026-09-21: Vienna's secondary HA instance moved off protocols-server-2 onto protocols-server-1 (memory/RAM crisis), then protocols-server-2 itself fully renamed to 'frame'. Junglinster's own protocols-server-2 explicitly KEEPS its secondary HA instance -- different decision, not the same fix applied twice."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-21T12:34:18.695Z
---

**Vienna (DONE 2026-09-21):** protocols-server-2 (also "frame", the picture-frame host) hit a real
crisis -- 916MB RAM, ~50MB available, load average ~9.4 on 4 cores, `podman ps` stuck 6+ min in
D-state. Root cause: the picture-frame X11/feh slideshow competing with the HA container for an
already-thin memory margin, both hitting real disk I/O once swap saturated. Fixed by moving the HA
container (bridges washing_machine/vacuum/sun/fritz_box, ~267MB persistent config) to
protocols-server-1 (3.75GB RAM, comfortable headroom), then stripping protocols-server-2 down to
picture-frame + mqtt_commandline + hosts/cpu only (removed podman and all its deps entirely -- no
container runtime needed there anymore). See [[project_vienna_frame_protocols_server_2_identity]]
for the earlier topology history this builds on. The MQTT-only hassbridge design meant zero DSL/
coordinator changes were needed for the move -- only `Vienna/deploy.d/02_deploy.hass.protocols-server-2`'s
SSH target changed (root@p-s-2:1971 -> erikp@p-s-1:1970 + `--rsync-path="sudo rsync"`, since p-s-1
has no root SSH). Bonus find during cleanup: the picture-frame slideshow itself had been silently
SIGSTOP-paused for 11+ days (predating this work), fixed via `~/bin/start_slideshow`.

**Phase 4 (the "protocols-server-2" -> "frame" rename) is ALSO DONE, same day.** The user edited
Physical.def/Conceptual.def directly (all four identity axes: `home_assistant` hassbridge
qualifier, `hosts` DeviceID token, `commandline`/picture-frame routing host, Conceptual.def position
label), then explicitly chose to deploy as-is and accept the DeviceID-token consequence flagged
earlier ("this is genuinely a re-use of the same hardware for a different role -- pure picture
frame now"). Confirmed live: the coordinator's own `RetireMissing` mechanism cleanly removed the old
`..._rack_protocols_server_2_...` entities from HA's registry entirely (not left as disabled stubs)
once Physical.def stopped declaring the old DeviceID; the new `..._rack_frame_...` ones came up live
in their place. Also caught+fixed mid-deploy: `Vienna/deploy.d/05_deploy.commandline`'s
`CMDLINE_HOST` var was still `protocols-server-2` (deliberately frozen when the port/hostname
changed earlier that day, since the DSL rename was still deferred at that point) -- left unchanged
it would have kept `mqtt_commandline.service` pointed at a config file (`protocols-server-2.yaml`)
the generator no longer produces, silently orphaning the daemon's config from the DSL's own source
of truth. Fixed: `CMDLINE_HOST=frame`, stale local+remote `protocols-server-2.yaml`/
`secrets.protocols-server-2` files removed. Vienna no longer has a "protocols-server-2" anywhere --
see Architecture.md 11.3's own dated update for the full writeup.

**Junglinster (explicit decision, 2026-09-21): do NOT do the same migration.** The user was asked
about Junglinster's own protocols-server-2 (similarly tight memory on paper -- ~54MB free/267MB
available/560MB swap used, mostly in fast zram not slow disk-backed swap) but explicitly chose to
**keep its secondary HA instance on protocols-server-2**, not move it to protocols-server-1 --
because Junglinster's protocols-server-1 already carries Zigbee2MQTT, Z-Wave, the coordinator, AND
the MQTT broker, so concentrating the HA instance there too would be a worse trade there than it
was for Vienna. Precisely: both houses' protocols-server-1 run the same base set (coordinator,
broker, Zigbee2MQTT, hassbridge relay) -- the actual difference the user pointed to (2026-09-21) is
(1) Junglinster's Zigbee network is much larger than Vienna's (Vienna: ~73 devices, confirmed at its
own Zigbee2MQTT migration 2026-09-08) and (2) Junglinster also runs Z-Wave there (PROJECT.md item 5,
Vienna has no Z-Wave migration item at all) -- not a vague "busier", these two concrete factors are
the whole delta.

**Why the two houses differ:** Junglinster's protocols-server-2 doesn't run a picture-frame
slideshow (that's a separate, genuinely distinct host there, `frame.junglinster`) -- so it lacks
Vienna's specific trigger (X11/feh competing for the same thin memory margin). Its load average
stays calm (~0.3) even under tight memory, because its swap pressure sits almost entirely in the
fast zram tier, not real disk I/O. [[project_ems_esp_mqtt_direct_architecture]] also confirms
EMS-ESP (a future integration) won't add load there either -- it's pure MQTT-direct to p-s-1's
broker, bypassing p-s-2 entirely.

**How to apply:** don't propose migrating Junglinster's protocols-server-2 HA instance the way
Vienna's was moved -- that's a considered, explicit "no" from the user, not an oversight. If
Junglinster's protocols-server-2 headroom does become a real problem later (worth periodically
re-checking `free -m`/`cat /proc/loadavg` there, especially if load average stops being calm), raise
it fresh rather than assuming Vienna's fix applies -- the two houses' protocols-server-1 boxes carry
meaningfully different baseline loads, so the same trade doesn't automatically hold.
