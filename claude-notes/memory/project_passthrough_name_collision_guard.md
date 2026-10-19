---
name: project-passthrough-name-collision-guard
description: "DONE+DEPLOYED 2026-09-20 (Junglinster): passthrough relay now dedupes by discovery payload's own default_entity_id, preferring whichever device's payload is self-consistent with its own state_topic -- fixes 'Entity not found' for renamed Zigbee2MQTT devices (xanadu/disks_1, display1, display2, photography_drive, imac all hit this). Two real bugs found DURING this deploy, not just the original one."
metadata:
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-20T13:52:35.797Z
---

**The original bug** (found live 2026-09-20): `switch.social_house_server_room_xanadu` and several
others ("Entity not found") turned out to be Zigbee2MQTT devices renamed at some point (xanadu ->
storage_room/disks_1 being the clearest case) whose OLD retained discovery payload under
`homeassistant.physical` never got refreshed -- `discoverybridge.go`'s passthrough relay trusted
Z2M's own `default_entity_id` field wholesale, so the stale payload and the new device that took
over the old friendly name both claimed the SAME suggested HA entity id. HA silently disambiguates
by suffixing one of them, which is exactly how a real, working device ends up "not found" under the
name everything else expects.

**Fix**: `TPassthroughDeviceTracker.ClaimName` (house_event_bus_coordinator/
discovery_passthrough_devices.go) tracks which device currently owns each `default_entity_id`. A
losing claim is suppressed (not relayed) and recorded as a collision, surfaced in
suggestions/discovery.txt via `buildPassthroughCollisionReport`
(homeassistant/discovery_passthrough_suggestions.go).

**Two more real bugs found WHILE deploying this, both fixed same session:**

1. **Order-dependency ("first claim wins" is wrong)**: the first version let whichever message the
   MQTT retained backlog happened to replay FIRST win ownership -- pure luck. On the actual
   redeploy, the STALE device won the race, which SUPPRESSED the genuinely correct, currently-
   working switch instead of the stale one -- exactly backwards, and it broke MORE entities
   (display1 disappeared) than the original bug did. Fixed by `discoveryNameLooksConsistent`
   (discoverybridge.go): checks whether a payload's own `default_entity_id` object-id actually
   starts with its own `state_topic`'s current friendly-name path -- Zigbee2MQTT's own convention
   guarantees this holds for a currently-correct device and fails for a stale one. A self-consistent
   claim now always displaces a less-consistent current owner, regardless of arrival order (two
   dedicated tests lock in BOTH orderings). Displacing also retires the loser's already-relayed
   topic (`TPassthroughNameClaim.Topic` tracks it) -- otherwise the stale entity would keep
   lingering in HA alongside the new, correct one even after ownership flipped.

2. **Crash loop from persist()-per-claim** (found live MINUTES after deploying fix #1): `ClaimName`
   called `t.persist()` (a synchronous full-state JSON write) on every first-time claim. During a
   coordinator restart, the retained backlog replay hits this for nearly every one of Junglinster's
   ~983 passthrough devices in one synchronous burst -- stacked on top of `Record`'s own existing
   persist cadence, this pushed startup past the coordinator's systemd watchdog timeout
   (`10-timeout-abort.conf`), producing a genuine crash loop (`status=1/FAILURE` every ~20-25s,
   `NRestarts` climbing, never reaching steady state). Fixed by NOT persisting on the two
   high-volume paths (first claim, same-device reclaim) -- correctness doesn't need it, since
   `discoveryNameLooksConsistent` is recomputed fresh from the live payload on every replay, so
   ownership re-derives itself correctly from scratch regardless of what's on disk. `persist()`
   stays only on the two genuinely rare paths (an actual collision, an actual displacement) --
   exactly the ones suggestions/discovery.txt needs to survive a restart.

**Why the crash loop is worth remembering**: this codebase already had ONE documented incident
(2026-09-14, `PassthroughStatusDebounceDelay`'s own doc comment) about exactly this shape of bug --
a per-message synchronous cost (there: an MQTT publish; here: a disk write) that's individually
cheap but becomes fatal at backlog-replay volume (hundreds to low-thousands of messages in one
synchronous burst at startup). Adding a SECOND instance of the same mistake, in the SAME file, days
later, means: **any new per-message hot-path code touching `TPassthroughDeviceTracker` should ask
"does this fire once per relayed message during a full backlog replay, and if so, is it doing I/O
(disk or network)?" before shipping** -- this is now the second time forgetting to ask that
question caused a live crash loop.

**How to apply**: mechanism is deployed and confirmed stable on Junglinster (protocols-server-1),
zero restarts observed post-fix, correct device consistently wins regardless of order. Not yet
deployed to Vienna (no passthrough-relayed renamed devices reported there yet, but the same class of
bug would apply if/when one shows up). See also [[project_passthrough_suggestion_tracking_decoupling]]
for the broader passthrough-suggestion mechanism this extends.
