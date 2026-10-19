---
name: project-stable-entity-identity-implementation-2026-10-05
description: "DONE+BOOTSTRAPPED 2026-10-05: PROJECT.md item 7 (stable coordinator<->HA entity identity via Physical.def \"as <id>\") fully implemented and bootstrapped on both houses -- the full switch, not a partial one. Supersedes project_stable_entity_identity_design.md's own open questions (now resolved)."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-06T01:33:18.991Z
---

Full implementation of the design in [[project_stable_entity_identity_design]] — both of that
memo's own open questions are now resolved and built, not just designed. This is the FULL switch
across both houses, not a scoped/partial one — the user's own explicit correction mid-session:
item 6's registry cleanup existed specifically to prepare for this, clearing orphans/ghosts so the
bootstrap freezes real, live identities only.

**Resolved open questions**:
- Fresh-id format (for a capability with no registry history to preserve): the entity's own
  resolved entity_id at first positioning ("position at birth"), frozen forever after. Collision
  disambiguator is a plain `_n` suffix (`_2`, `_3`, ...), per the user's own explicit correction of
  an earlier `__n` sketch.
- Scope: full two-house bootstrap of every currently-live discovery capability, not a targeted
  patch for only the devices about to be renamed.

**What got built** (all 2026-10-05, one session):
- Grammar: `as <id>;` on a discovery capability's plain raw-leaf line
  (`homeassistant/integration_discovery_parser.go`'s `discoveryAssignedIDPattern`) ->
  `TDiscoveryCapability.AssignedID`. Rejected (warned, not silently accepted) on an `is available`
  capability line — those never flow through unique_id computation at all.
- Generator passthrough: `TDiscoveryEntityLink.StableID` (`administration.go`) ->
  `discovery.yaml`'s new `stable_id:` field (`integration_discovery_generator.go`).
- **Three** construction sites needed the passthrough, not two — `registerDiscoveryEntityLink`
  and `registerDiscoveryDerivedFromRawLeafEntityLink` (both `Conceptual_DiscoveryEntities.go`) were
  obvious, but `autoMaterializePhysicalCapability`'s own discovery-kind branch
  (`Conceptual_LogicalEntities.go` — the "switched by"/"dependency on" auto-materialization path)
  built its own `TDiscoveryEntityLink` independently and was missed on the first pass. Found live,
  post-bootstrap, checking `sensors.house_kitchen_workplace_dimmer`'s own `discovery.yaml` output —
  its `stable_id` was simply absent. **Lesson: `grep -rn "TDiscoveryEntityLink{" --include="*.go" .`
  (or the equivalent for any struct literal being extended) is the way to find every construction
  site, not reasoning from the two "obvious" registration functions** — a third one hiding behind
  a generalized "auto-materialize any physical capability" helper, not named anything
  discovery-specific, was easy to miss by inspection alone.
- Coordinator preference (`house_event_bus_coordinator`): `buildRelayedDiscoveryConfig`
  (`discoverybridge.go`) AND `expectedDiscoveryBridgeTopics` (`discoverycleanup.go`) both needed
  the SAME `link.StableID` preference over `relayedDiscoveryUniqueID(gatewayID, leaf)` — missing
  the second would have made the orphan-cleanup pass wrongly retire a stable-ID'd entity's own
  real topic (it would compute the OLD gatewayID+leaf-based topic as "expected," never the real
  stable-ID-based one actually being published).
- Ongoing auto-assignment for brand-new capabilities (`homeassistant/physical_auto_assign_stable_
  ids.go`, `autoAssignDiscoveryStableIDs`): runs at the end of every normal `./generate`, mints a
  fresh position-at-birth id for any live-but-unassigned capability, and — the one deliberate
  exception to this codebase's otherwise-universal "`.def` files are only ever read, never
  written" rule — rewrites Physical.def to add it. All-or-nothing per run: if any single
  assignment's own capability line can't be found exactly once in the real on-disk file (searched
  by reconstructing the exact domain/label/leaf pattern, never by line-number tracking — rejected
  that approach early, since the one-liner-wrapping normalisation preprocessing pass means a
  parsed line's "line number" doesn't reliably map back to the real file's own layout), NOTHING is
  written for that run rather than a partial, half-assigned file.
- **Real bug avoided before it shipped**: this auto-assignment call was first wired into
  `parseAdministrationFromPaths`, which is ALSO the shared read-only phase
  `-would_define`/`-derived_migration_candidates` call directly — both explicitly documented as
  "without generating any output or editing any file." Caught before running anything for real;
  fixed by adding a `writeBack bool` parameter, true only from the real generation pipeline
  (`generateFromPaths`), false from both read-only diagnostic callers.
- Orphaned-id warning (the complement to "undeclared entity" checks, same file): a capability with
  an `AssignedID` that's no longer live gets a `[physical]` warning instead of being touched.
- One-time bootstrap CLI mode: `homeassistant -bootstrap_stable_ids <mapping.json> Definitions/
  Main.def` (`bootstrap_stable_ids.go`), reading a flat `{entity_id: unique_id}` dump of the live
  registry's own mqtt-platform entities (no pre-filtering needed — only entries that match a live
  discovery capability are ever consulted) and freezing that EXACT value, never a freshly-minted
  one, for every live-but-unassigned capability it can find a match for.

**Full test coverage added**: `integration_discovery_parser_test.go` (new file, this parser had
none before), `physical_auto_assign_stable_ids_test.go` (6 tests: fresh assignment, collision
disambiguation against an already-assigned id — NOT between two fresh ones, which can't actually
happen via DiscoveryEntityLinks since that's already a duplicate-entity-id error caught elsewhere
— orphaned-id warning, ambiguous-match refusal, bootstrap preserving a real registry value,
bootstrap warning on no matching registry entry), a dedicated regression test for the
`autoMaterializePhysicalCapability` gap, and `StableID`-preference cases added to both existing
coordinator tests that needed them (`TestBuildRelayedDiscoveryConfig`/
`TestExpectedDiscoveryBridgeTopicsUsesGatewayAndLeaf`'s own test files). All green throughout.

**Live bootstrap results**: Vienna 158/164 discovery capabilities bootstrapped from the live
registry (6 declared+positioned in Conceptual.def but genuinely never live in HA's own registry
yet — correctly skipped rather than guessed; picked up a fresh position-at-birth id on the very
next ordinary regenerate instead, confirmed). Junglinster 290/290, zero skips. Post-bootstrap full
cross-check (every `discovery.yaml` entity_link's `stable_id`, not a sample, against the real
registry dump): 100% exact match both houses, zero missing, zero mismatched.

**Applied as the first real proof**: the 3 deferred `lights.*` renames from
[[project_dimmer_device_naming_2026_10_05]] — confirmed `discovery.yaml`'s `gateway:` now reads
`lights.*` but `stable_id:` is byte-identical to before the rename for all 3; full `hass/
junglinster/` diff otherwise unchanged (the same 8 already-understood "switched by" helper files
whose own entity_id STRING reference updates — no filename changes, no new/missing files).

**Also corrected mid-session**: the original "buttons.*" rename from
[[project_dimmer_device_naming_2026_10_05]] had been called "safe" based on diffing the
GENERATOR's own static file output only — that check never covered the coordinator's own
RUNTIME-computed `unique_id` (which depends on `gatewayID` too, invisible in any generator-side
diff). All 7 renames (not just the 3 originally flagged) actually needed this mechanism; the 4
`buttons.*` ones are now also correctly bootstrapped and safe to deploy, same as the 3 `lights.*`
ones.

**NOT yet done**: neither live coordinator has been restarted/redeployed with any of this —
everything above is local `Physical.def`/`./generate` verification only. Both houses' dated
`Physical.def` backups (pre-bootstrap and pre-rename) sit in `/tmp/step7_bootstrap/backups/` this
session — not a durable location, re-create if this work is picked up again later and that
directory is gone.

**How to apply**: before deploying either house's coordinator with these changes, re-verify the
`stable_id` cross-check against a FRESH registry pull (the one used here is now stale), and expect
the coordinator's own full test suite (`go test ./house_event_bus_coordinator/...`) to be the
right gate before any live restart, not just the generator's own output.
