---
name: project-conflicting-device-name-and-cursor-persistence-fixes
description: "PROJECT.md item 0 (cross-kind + within-kind duplicate DeviceID warnings) and a real entity-existence-tracker cursor-persistence bug (manufacturer/model never refreshing for some hassbridge/import devices) -- both found, fixed, tested, and deployed live to both houses 2026-09-11."
metadata:
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-11T07:47:08.444Z
---

**Trigger**: while checking the [[project_derived_capability_mechanism]] battery_alert migration
live, the user noticed Vienna's imported Netatmo devices were missing manufacturer/model and a "via
device" link in HA's device-info screen, compared to how they show on Junglinster's own
protocols-server-2. Asked to investigate+fix the manufacturer/model half now, combined with
PROJECT.md item 0 (an already-queued "warn on conflicting device names" fix); asked to queue the
`via_device` half as its own next item, after item 4.

**Fix 1 — entity-existence cursor persistence (`house_event_bus_coordinator/entity_existence.go`)**:
`nextToInquire`'s two round-robin positions (`cursor`, `backlogCursor`) were never persisted to
`entity_existence.json` — every coordinator restart reset both to 0, so the "recheck already-known
entities" pass always restarted from the front of `order`. An instance with many tracked entities
and frequent restarts (redeploys) could go a very long time — in the worst case indefinitely —
without ever reaching entities near the tail of its own list, even though the remote automation was
always fully capable of returning fresh manufacturer/model/typing data. **Confirmed via a live
manual inquiry** (published directly to `homeassistant_instances/protocols-server-2/inquire` on
Junglinster) — the reply came back instantly with full manufacturer/model, proving the remote side
was never the problem. Fixed by persisting both cursors; regression-tested
(`TestEntityExistenceTrackerPersistsRoundRobinCursorsAcrossRestart`); deployed live to both
Junglinster's and Vienna's coordinators (rebuilt+restarted cleanly, zero errors, `backlog_cursor` now
observed actually persisting).

**How to apply**: if a similar "coordinator knows less than it should, despite the remote side being
fully capable" symptom ever comes up again, the diagnostic technique that cracked this one — publish
a manual inquiry directly (`mosquitto_pub`/`mosquitto_sub` against
`homeassistant_instances/<instance>/inquire`/`.../inquire/reply`, credentials from
`coordinator/secrets.yaml`'s own `mqtt:` block) and compare the fresh reply against what's actually
stored — is worth reaching for before assuming a deeper design gap.

**Fix 2 — PROJECT.md item 0, conflicting device names**: real incident (Junglinster's own
`host.frame` declared twice, found only after its second declaration's capabilities silently went
missing, fixed manually before this session). Two-part fix, both warning-only:
- New cross-kind check, `homeassistant/physical_rule_no_conflicting_device_names.go`'s
  `validateNoConflictingDeviceNames` — warns when the same DeviceID appears under more than one
  integration kind, EXCEPT the one deliberate case (a commandline device sharing identity with an
  already-positioned hosts device of the same id, e.g. `host.frame`'s own picture-frame daemon) — a
  third kind also using that id still gets flagged even then.
- Within-kind: hosts/discovery/commandline/import's own collectors used to silently drop a
  duplicate DeviceID within their OWN integration with zero feedback (hassbridge's collector already
  warned on this) — now all five warn consistently.
- Verified live: zero warnings on either real house today; both regenerate byte-identical.

**Noticed but explicitly NOT touched**: Junglinster's own Physical.def has a
`device appliance.picture_frame with: ...;` commandline declaration (mirroring Vienna's own working
picture-frame migration) that the commandline parser rejects outright on every `./generate`
("unrecognised line" warnings) — looks like it's missing the host/instance qualifier token Vienna's
working version has. Looked like the user's own in-progress edit (file-changed-on-disk reminders
fired repeatedly this session for Junglinster's Physical.def/Spaces.def) — flagged to the user rather
than silently "fixed."

**Queued next (PROJECT.md item 24, after item 4 finishes)**: the `via_device` half of the original
request — neither hassbridge (kind-3) nor import (kind-4) has any via_device inference at all today
(unlike hosts and kind-2 discovery gateways, which already do). The user's own framing: once the
coordinator can infer, from info it's already collected via discovery/inquiry, that one advertised
device depends on another also-advertised device, add a via_device field using the device names as
already advertised in their own config payloads — no new Physical.def syntax needed. Related to
PROJECT.md item 14's own "via device"/availability-composition notes (the Netatmo base-module chain
is the exact same real-world topology already flagged there) — this new item is the narrower,
physical-layer, concretely-buildable slice of that broader logical-layer topic.
