---
name: project-import-rename-remote-device-id-bug
description: "FIXED 2026-09-19, not deployed: renaming an import-kind device's LOCAL id (the 2-token 'device <id> from <installation> with:' shorthand) silently also changes the REMOTE device id it looks up, since the shorthand defaults remote_device_id to the local id. Vienna's sensors.vienna_terrace_wind went completely dark since the 2026-09-18 'sensors.' rename."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-18T22:37:19.506Z
---

Found while chasing why `sensor.social_terrace_wind_speed`/`wind_direction` were "still missing"
after the [[project_device_leaf_naming_conceptual_mismatch_gap]] fix was deployed -- the naming fix
was correct, but the ENTIRE device (`sensors.vienna_terrace_wind`, all of battery_alert/
battery_level/radio/node/windy/windy_threshold too, not just the two flagged entities) had been
silently producing nothing in HA since the day before. Zero coordinator log lines mentioned
"netatmo_windmeter" or "vienna_terrace_wind" at all across a 2000-line journalctl window --
confirmed the device was completely dark, not just misnamed.

**Root cause**: `integration_import_parser.go`'s import header has two forms --
`importHeaderPattern` (3 tokens: `device <local-id> from <installation> <remote-device-id> with:`,
explicit remote id) and `importHeaderShorthandPattern` (2 tokens: `device <local-id> from
<installation> with:`, remote id OMITTED). The shorthand sets `RemoteDeviceID` to the SAME string
as the local `DeviceID` (parser line ~150) -- a convenience that assumes local and remote ids are
identical, which is true UNTIL the local id gets renamed for local naming-convention reasons
(exactly what [[project_zigbee_mqtt_relocation_2026_09]]'s "sensors." convention work did,
2026-09-18: `node.vienna_terrace_wind` -> `sensors.vienna_terrace_wind`, LOCAL rename only). Once
that happens, the shorthand form silently starts looking up the WRONG remote device -- Junglinster
never renamed its own export (`device node.vienna_terrace_wind export with: ...;`, Physical.def,
correctly unchanged, since Vienna's local convention has nothing to do with Junglinster's own id),
so the coordinator's cross-house lookup just stopped matching anything, with no warning anywhere
(the generator has no way to know the remote id is wrong -- it's a cross-installation fact it
can't check).

**Fix**: switched to the explicit 3-token form: `device sensors.vienna_terrace_wind from
junglinster node.vienna_terrace_wind with: ...;` -- local id keeps the new convention, remote
lookup correctly targets Junglinster's real, unrenamed id. Confirmed via regenerate:
`coordinator/imported.yaml`'s `remote_device_id` is `"node.vienna_terrace_wind"` again, matching
every other imported device's own untouched remote id.

**How to apply**: **any future local-id rename of an import-kind device that currently uses the
2-token shorthand must switch to the explicit 3-token form in the same edit**, or the device goes
silently dark with no error from the generator (it's a cross-installation fact the generator
cannot verify by itself). Check `grep "device .* from junglinster with:"` (2-token, shorthand) vs
`grep "device .* from junglinster \S\+ with:"` (3-token, explicit) before renaming any import-kind
device's own local id, on EITHER house. Not yet deployed as of 2026-09-19 -- combined with
[[project_device_leaf_naming_conceptual_mismatch_gap]]'s own pending Vienna deploy.
