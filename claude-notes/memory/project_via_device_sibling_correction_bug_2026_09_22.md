---
name: via-device-sibling-correction-bug-2026-09-22
description: "DONE+FIXED+DEPLOYED 2026-09-22 - Seed() only corrected via_device for DECLARED capabilities after a device rename, leaving DiscoverSiblings-discovered undeclared siblings stuck under the old id, causing ResolveViaDevice to non-deterministically pick stale vs fresh targets"
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-22T10:44:28.215Z
---

Real bug, found live 2026-09-22 during the Vienna Netatmo `node.vienna_*` -> `sensors.vienna_*`
rename (see [[project_junglinster_coordinator_crash_loop_2026_09_22]] for the same day's earlier,
unrelated coordinator incident). After the rename+redeploy, some capabilities of the SAME renamed
device showed the correct new `via_device` in their MQTT discovery config while others of the exact
same device showed the stale pre-rename id — e.g. `binary_sensor...vienna_bedroom_node` correctly
showed `via_device: "sensors.vienna_livingroom"` while `sensor...vienna_bedroom_temperature` still
showed `via_device: "node.vienna_livingroom"`, on the very same coordinator restart.

**Root cause**: `TEntityExistenceTracker.Seed()`'s rename-correction (entity_existence.go) only
ever fixes tracked entries matching a currently-DECLARED Physical.def capability's own source
entity. `binary_sensor.vienna_connectivity` — the Netatmo base station's own connectivity sensor,
deliberately never used as any device's own declared "node" source (Physical.def's own comment:
"has proven to be too flaky") — is a `DiscoverSiblings`-discovered entry, invisible to that
correction loop entirely (documented, deliberate scoping, not itself wrong). Its `DeviceID` stayed
stuck at the OLD pre-rename id forever, since `entity_existence.json` persists across every
coordinator restart. `ResolveViaDevice`'s own second lookup (a plain, unsorted Go map range over
every tracked entry sharing a via-target's `RemoteDeviceID`) then picked EITHER this stale sibling
or a freshly-corrected declared entry NON-DETERMINISTICALLY on every call — explaining exactly why
different capabilities of the same device disagreed on the same restart.

**Fix**: `Seed()` now does a second pass after its normal declared-capability correction loop:
build an `instance -> RemoteDeviceID -> current DeviceID` map from the entries it just corrected,
then propagate that correction to EVERY tracked entry (declared or undeclared/sibling) sharing the
same `RemoteDeviceID` — so a sibling follows its device's rename automatically, closing the gap
without touching `ResolveViaDevice` itself (once every candidate entry agrees, the non-deterministic
pick no longer matters). Regression test:
`TestEntityExistenceTrackerSeedCorrectsSiblingDeviceIDAfterRename` (entity_existence_test.go) —
calls `ResolveViaDevice` 20 times in a loop to prove determinism, not just correctness once.
Deployed to both Junglinster and Vienna (shared coordinator code); verified live via MQTT discovery
config inspection on Junglinster (all 5 renamed Netatmo devices' `via_device` fields consistent).

**General lesson**: whenever a "declared source wins" correction pass exists, check whether any
OTHER tracking mechanism (here: `DiscoverSiblings`, for entities no DSL declaration ever names) can
create entries the correction pass doesn't reach — and whether anything downstream assumes ALL
entries for a device are mutually consistent (here: `ResolveViaDevice` did, silently).
