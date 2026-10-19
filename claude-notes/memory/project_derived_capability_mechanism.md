---
name: project-derived-capability-mechanism
description: "PROJECT.md item 4's \"derived\" capability mechanism -- grammar, both runtime paths (hassbridge + import), and the migration-candidate report tool all built and verified live 2026-09-10. Next/paused step: manually migrate Vienna's 4 real Netatmo battery_alert devices, then verify live."
metadata:
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-10T14:46:30.180Z
---

**Full design and progress log**: `/Users/erikproper/.claude/plans/derived-capability-mechanism.md`
(authoritative, kept up to date every session) — this memory is a short pointer + the concrete
next-step reminder, not a duplicate of that detail.

**What's done (2026-09-10), all tested + verified byte-identical on both real houses:**
- Rule 1 (every home_assistant/hassbridge + import device needs a `node` capability) — warning-only,
  zero real violations.
- `derived entity DDD.NNN from EEE.MMM via TTT;` Physical.def grammar, storage, and a validation
  pass (same-device-sibling presence + cycle-freedom).
- Runtime for BOTH kinds: hassbridge/kind-3 (generator-side only, zero coordinator changes) and
  import/kind-4 (coordinator publishes the derived capability's own discovery config, state_topic
  pointed at its atomic ancestor's own already-relayed local topic + a composed value_template — no
  new relay/reactive state needed either).
- `-derived_migration_candidates` CLI flag — detect-and-report only (the user's explicit choice;
  auto-rewrite deferred until the full Zigbee2MQTT migration needs it). Verified live: finds
  exactly Vienna's 4 real candidates, correctly excludes ~30 unrelated Zigbee-backed macro calls.

**Paused here, resuming when the user has time (said explicitly, boarding a flight 2026-09-10)**:
the actual migration itself, starting with **Vienna's 4 imported Netatmo devices** —
`import.vienna_bedroom`, `import.vienna_shower_room`, `import.vienna_terrace`,
`import.vienna_terrace_wind` (all in `/Users/erikproper/SmartLiving/Vienna/Definitions/Physical.def`,
`integration import with:` block). For each: add the tool's own recommended
`derived entity binary_sensor.battery_alert from sensor.battery_level via "'on' if (($ |
int(0)) < N) else 'off'";` line inside that device's block (N per-device — 20 for all 4 currently,
per the report), then remove the corresponding Spaces.def `call battery_alert :... with
alert_level N;` macro line and replace it with an ordinary `battery_alert` capability positioning
line (same shape any other capability on that device already uses).

**How to resume**: re-run `homeassistant -derived_migration_candidates Definitions/Main.def` from
`/Users/erikproper/SmartLiving/Vienna` first — confirms the report output hasn't gone stale, gives
the exact current lines to edit. Make the edits, `./generate`, review the diff before ever deploying
live, then verify against the actual HA UI (existing session discipline: live-verify one device
first). Once Vienna is confirmed working, repeat for Junglinster's own real candidates (a `-derived_migration_candidates`
run there found 7 of its 12 earlier-documented Rule 2 violators have live macro usage to migrate
from — the plan file has the full list). Only once BOTH houses are migrated does Rule 2 itself get
switched on as a live, enforced check — not before.

**Also unresolved, noted in the plan file**: a `derived` capability on a commandable domain
(switch/vacuum) would get a non-functional `command_topic` today — not a real risk yet since nothing
declares one, but worth fixing before `derived` is ever used on a commandable domain for real.

**Second test case, added by the user 2026-09-10 (also just a note for later, not started)**: Vienna's
`sun`/`daylight` pair — currently `Spaces.def:317-318`:
```
entity sensor.social:sun/solar_elevation;
entity binary_sensor.social:daylight with condition sensor.social:sun/solar_elevation "$ | float > 4";
```
Two manual steps the user specified: (1) source the sun integration from p-s-2's own local HA
instance as a proper Physical.def hassbridge device (this is PROJECT.md item 1's existing "sun via
ps2" todo — do that first), (2) define `daylight` as a `derived entity binary_sensor.daylight from
sensor.solar_elevation via "...";` instead of the old bare `condition`. Deliberately chosen as a
**hassbridge/kind-3** test case (p-s-2 is Vienna's own local instance, same broker) — complements
the Netatmo devices above, which exercise the **import/kind-4** path instead. Good second
end-to-end verification once the Netatmo migration is confirmed working.
