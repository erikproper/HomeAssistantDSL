---
name: project-kind5-main-entity-existence
description: "PROJECT.md items 1/1b: kind-5 existence checking + suggestions for \"bare\" Spaces.def entities assumed to exist on the main HA instance. DONE + DEPLOYED live to both houses 2026-09-07/08, see Architecture.md §6.10."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-08T06:53:42.332Z
---

Full plan lives at `/Users/erikproper/.claude/plans/splendid-zooming-lighthouse.md` (2026-09-07
version — the file was previously a different, already-shipped plan about import grammar
unification; overwritten once superseded).

**What**: ~144 (Junglinster) / ~28 (Vienna) bare `entity <domain>.<path>;` Spaces.def declarations
(no body at all — `TEntityRecord.HasDefinitionOrImport == false`) currently have zero existence
checking. Reuses kind-3's already-instance-generic `TEntityExistenceTracker` engine
(`house_event_bus_coordinator/entity_existence.go`) almost entirely as-is, scoped to instance
"main" — new work is narrow: collect the bare-entity list
(`homeassistant/main_entities.go`'s `collectMainEntityIDs`, via `EntityRecordsBySpace` +
`toHomeAssistantEntityID`, NOT the unused `ExternalEntitiesBySpace`), a new
`coordinator/main_entities.yaml`, a new `TEntityExistenceTracker.SeedMainEntities` method, and a
parallel `checkMainEntityKnownNotToExistErrors` (mirroring kind-3/4's own check functions, not a
shared generalized signature — matches every prior kind's own choice).

**Real bug found during design review (Plan agent), not yet fixed**: `Seed(bridgeFile)`'s own
ghost-pruning would silently wipe a confirmed-dead bare entity's status on every coordinator
restart, since bridgeFile never declares it. Fix: an in-memory-only `mainEntities` protected-set
field, consulted by `Seed`'s own pruning loop; `SeedMainEntities` must run before `Seed` each
startup.

**Deliberately zero new DSL grammar/parsing** — operates entirely on already-parsed
`EntityRecordsBySpace` data. See [[feedback_no_new_regexp_parsers]] for why that mattered enough to
call out explicitly in the plan.

**Rollout risk flagged in the plan**: `nextToInquire` drains the full not-known-to-exist backlog
before any round-robin refresh — first rollout costs ~2.4h (Junglinster)/~28min (Vienna) during
which existing "main"-instance rechecks are blocked. One-time, self-resolving.

**Status as of 2026-09-07: implementation complete, locally verified, NOT yet deployed.**
`go vet`/`go test -race` clean across all three packages (new tests: 3 coordinator-side
`TEntityExistenceTracker` tests including the pruning-bug regression, 2 generator-side
`collectMainEntityIDs` tests, 3 `checkMainEntityKnownNotToExistErrors` tests). Both real houses
regenerated and diffed: exactly one new file each (`coordinator/main_entities.yaml`), zero change
to anything else -- confirms the hard-fail check is a true no-op with no live status yet, exactly
as designed.

**Real counts turned out bigger than the plan's own rough estimate**: 318 entities for Junglinster,
123 for Vienna (the plan's ~144/~28 came from a narrow textual grep for literal bare
`entity <domain>.<path>;` lines; `collectMainEntityIDs` also correctly sweeps in macro-expanded
records with `HasDefinitionOrImport=false` -- e.g. `battery_alert` binary_sensors -- which never
matched that narrower grep pattern but are exactly the same kind of "assumed to exist, no explicit
value/condition" record). This means the plan's own stated rollout risk (backlog-drain time before
periodic rechecks of other main-instance entries resume) is correspondingly bigger on first
rollout: roughly 5.3h for Junglinster, ~2h for Vienna, at the existing 1-inquiry/minute-per-instance
pace -- still one-time and self-resolving, not a recurring cost, but worth restating with the real
number before deploying.

**Status as of 2026-09-08: DONE + DEPLOYED live to both houses.** Committed (`00299b5`), pushed,
deployed. Confirmed live: Junglinster's first hard-fail run flagged 21 genuinely-absent bare
entities on the first try -- real value delivered immediately.

**Starvation bug found live during rollout, fixed and deployed (commit `1e5dd8c`)**: `nextToInquire`
always rescanned the backlog from index 0, so one never-resolving entry (a typo'd entity name,
found live: `sensor.social_terra_sun_solar_elevation` vs. the real
`sensor.social_terrace_sun_solar_elevation`) permanently starved everything registered after it --
no warning was ever raised for it across multiple `./generate` runs over hours, which is what
surfaced the bug (the user correctly insisted "persisted across multiple runs" meant a real bug,
not just pacing, after an initial wrong "just pacing" explanation from Claude). Fixed via
a `backlogCursor` that rotates the scan start each call. Confirmed live: both houses' backlogs fully
drained (Vienna 395/395, Junglinster 1242/1242, 22 confirmed-absent), nothing stuck.

**Also found+fixed live, one-off manual cleanup not a code change**: a stale retained MQTT message
on `meta/discover_entity/set` was re-seeding `sensor.vienna_terrace_humidity` on every coordinator
restart. Fixed by clearing the retained message first, then removing the persisted JSON entry
(order matters -- doing the JSON edit first gets undone by the next restart's retained replay).

PROJECT.md items 1/1b trimmed to a one-line DONE pointer at Architecture.md §6.10 (2026-09-08),
following [[feedback_project_md_stays_a_todo]]. Note: PROJECT.md item 13 ("re-enable checking of
locally assumed entities... once we have all the integrations we need") describes essentially this
same feature, originally deferred behind that gate -- now redundant/needs re-scoping since kind-5
shipped ahead of that gate; flagged to the user, not yet resolved.
