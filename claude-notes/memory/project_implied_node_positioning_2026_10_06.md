---
name: project-implied-node-positioning-2026-10-06
description: "DONE+DEPLOYED 2026-10-07: PROJECT.md item 8 (device/entity friendly names from conceptual position) complete end-to-end and removed from PROJECT.md -- implied node positioning, the universal DeviceNodeEntityID choke point, the new Logical.def \"colocated\" construct, commandline's own physical-layer \"dependency on\", and a sphere-agnostic heating-detection fix. Both houses deployed and live-verified."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-07T14:22:10.933Z
---

PROJECT.md item 8, full arc. Continuation of [[project_stable_entity_identity_implementation_2026_10_05]].
Design: entity friendly name = `identity.Sphere + "/" + identity.Path`; device friendly name = its
positioned `node` entity's identity with the trailing `/node` dropped (a Z-Wave relay's own friendly
name describes where the relay module physically IS, not the room it controls).

**Mechanisms built** (`device_friendly_name.go`, `administration.go`, `integration_logical_parser.go`/
`integration_logical_storage.go`, `integration_commandline_*.go`):
- `DevicePositioningOccurrences[deviceID]` -- one `{SpacePath, DeviceNamePath}` per "device X [as ...]
  with:" occurrence, recorded unconditionally in `registerDevicePositioning` before its own
  register-once guard.
- `applyImpliedNodePositioning` -- a device positioned in exactly ONE space gets `node` auto-implied
  (only one possible answer); 0 or 2+ occurrences get a warning instead of a guess.
- `DeviceNodeEntityID[deviceID] -> entityID` -- the universal choke point fix. Four different internal
  shapes back a device's own "node" (raw discovery leaf, discovery `derived ... is available`,
  Logical.def override, hosts/import/commandline's `AttributeEntityIDs["node"]`); per-kind lookups
  missed one, producing colliding duplicate-node registrations. Fixed by populating this ONE map
  inside `registerDeviceCapabilityEntityLinkAllowingHidden` itself, right after any successful "node"
  dispatch, regardless of which internal shape handled it.
- `findSharedDevicePositions` -- catches two DIFFERENT devices weaving onto the same conceptual
  position, each independently lacking node, that would otherwise collide on the same bare
  ".../node" (reports only when 2+ group members genuinely lack a positioned node, not merely when
  they share a position).
- **New Logical.def construct**: `colocated <device-id> as <leaf>;` (repeatable, on the anchor
  device's own block) -- positions a device with no location of its own (e.g. a bare Z-Wave relay
  switch) at the anchor's own resolved position + the declared leaf. Reuses
  `registerDevicePositioning` directly, so the colocated device's own node then gets auto-implied for
  free.
- **Commandline's own `dependency on <id>;`**, declared directly inside Physical.def's `integration
  commandline with:` block (NOT via Logical.def like hassbridge/import) -- the user's explicit call:
  commandline is a physical-layer integration, the dependency belongs at the physical layer. Resolved
  single-hop against `hostDevicesByID`, ANDed into every entity including the device's own liveness
  one, via a new shared `TAvailabilityEntry`/`nodeAvailabilityEntries` helper
  (`house_event_bus_coordinator/discovery.go`) refactored out of `buildAvailabilityFields`.

**Real bugs found running this against both houses' real Conceptual.def (not by code review)**:
1. First "fixed" `resolveDeviceFriendlyName` still missed the discovery `derived ... is available`
   shape -- flagged ~50+ positioned devices as unpositioned, causing colliding duplicate registrations
   (fixed by the `DeviceNodeEntityID` choke point above).
2. `heatingCapableSpaces` (admin.go) and `generateSwitchControlledHeatingContent` (generator.go) both
   hardcoded `Sphere == "physical"` to detect a space's heating switch, AND the latter hardcoded its
   generated `switch.turn_on`/`turn_off` automations' target entity_id to
   `switch.physical_<context>_heating` regardless of the switch's real sphere. Renaming a heating
   switch to `social` sphere (a real, user-requested cleanup) silently deleted the entire
   leakage/mode-preset subsystem for that room, and would have pointed live automations at a
   nonexistent entity. Fixed: both are now sphere-agnostic, triggered purely by the `heating` path
   tail; the automation's target entity_id is resolved from the real scan, never hardcoded.
   **Ironic finding while verifying**: Junglinster's `switch.social_house_bathroom_heating` had
   already been the REAL live entity for 9+ days -- the `.def` source and deployed reality had
   drifted out of sync (source said `physical`, live state was already `social`). The fix was
   corrective, not just safe.

**Deployed and live-verified, both houses, 2026-10-07**: full `./generate` + `./deploy` (HA main +
ha2mqtt, hosts/cpu/ping, coordinator rebuild+restart, commandline/picture-frame). Verified via direct
recorder-DB queries (Vienna needs `sudo` for `/config/*.db` access via the `hassio` SSH user --
`ls`/dir-walk works without it, individual file reads don't; Junglinster's own SSH access doesn't need
`sudo` for the same files) -- zero `stable_id` changes anywhere in either house's `discovery.yaml`
(confirmed via full diff before deploying), all new/renamed entities confirmed live and freshly
updating. PROJECT.md item 8 removed entirely (zero-trace convention); lasting design detail moved to
Architecture.md §6.15.

**How to apply**: `DeviceNodeEntityID` is now the one authoritative source for "where is this device's
own node positioned" -- don't re-derive per-kind lookups again. Any FUTURE heating-switch-sphere-style
hardcoding should be grepped for (`Sphere == "` across generator.go/administration.go) before trusting
a sphere rename is safe -- this bug class (a hardcoded sphere assumption several functions deep from
the actual rename) is exactly the kind of thing `-would_define` dry runs don't catch, since it only
manifests in the FULL `./generate` output diff, not the "[conceptual]" warning stream.
