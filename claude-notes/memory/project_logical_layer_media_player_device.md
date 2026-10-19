---
name: project-logical-layer-media-player-device
description: "DONE+DEPLOYED (2026-09-16/17): second logical-layer mechanism -- a logical device can declare its own capabilities (an 'is available' condition + a media_player->switch coercion), letting it be positioned at the conceptual layer like a physical device. Migrated Vienna's apple_tv/tv/sonos(pair)/bedroom sonos-roam off the old media_player_device macro. A real bug in the dependency-target fallback naming (permanently-false is_state checks) was found and fixed 2026-09-17 -- read that section before trusting this mechanism's own \"dependency on\" output elsewhere. Read before extending the logical layer's capability shapes further (e.g. Junglinster's own no_collect case)."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-17T13:41:35.043Z
---

## What this is

Second logical-layer construct (after [[project_logical_layer_dependency_on]]), migrating
Vienna's `media_player_device` macro calls:
```
call media_player_device :apple_tv;
call media_player_device :tv with: enabler switch.social:tv; delay_off 00:01:00; end;
```
to:
```
logical layer with:
  device utility.apple_tv with:
    binary_sensor.node: media_player.social_apartment_living_room_apple_tv is available;
    switch.media: media_player.social_apartment_living_room_apple_tv;
  end;
  device utility.tv with:
    binary_sensor.node: media_player.social_apartment_living_room_tv is available with:
      enabler switch.social_apartment_living_room_tv;
      delay_off 00:01:00;
    end;
    switch.media: media_player.social_apartment_living_room_tv;
  end;
end;
```
positioned at the conceptual layer exactly like a physical device:
```
device infrastructural:apple_tv from utility.apple_tv with:
  entity switch.social:media from media;
end;
```
`call media_player_device :sonos;` (both Vienna occurrences) was explicitly OUT of scope this
pass — Junglinster's own sonos case needs `no_play_input`, not migrated yet.

**Important**: the underlying `media_player.*` entity is NOT part of the logical device at all --
it's a separate, still-bare, kind-5 "assumed to exist on main" entity, exactly as before. A real
gap found live during this migration: removing the old macro call also removed its own implicit
bare declaration of that entity, which broke the unrelated "presence" check
(`checkEntityReferences`, presence.go) — every literal entity_id referenced inside generated Jinja
must ALSO be independently declared somewhere in Conceptual.def, or it shows up as "unaccounted
for." Fix: keep an explicit `entity media_player.social:apple_tv;` line alongside the new device
block.

## New grammar (`integration_logical_parser.go`)

```
device <id> with:
  <domain>.<label>: <entity> is available [with: enabler <entity-ref>; delay_off <time>; end;];
  <domain>.<label>: <entity> [with: no_play_input "<value>"; end;];
end;
```
Both `Entity` and `EnablerEntity` are **literal, already-existing HA entity_ids**, used verbatim
— never a space-relative DSL spec (`switch.social:tv`-style shorthand), since Logical.def has no
space nesting of its own to resolve one against. This was a real design judgment call (the user's
own sketch used the shorthand form for the enabler) — worth confirming if it ever feels like the
wrong call; for now Vienna's Logical.def spells both out fully qualified
(`switch.social_apartment_living_room_tv`, not `switch.social:tv`).

## How it registers (`Conceptual_LogicalEntities.go`, new file)

- **`is available`** (only "node" today, not label-restricted) reuses the discovery gateway's own
  `TEntityRecord`/`ConditionSources`/`ConditionExpr` → `buildConditionBinarySensorYAML` pipeline
  wholesale (`registerDiscoveryAvailabilityEntityLink`'s own pattern) — no new YAML rendering at
  all.
  **Real bug found live**: the enabler must be folded into `ConditionExpr` as a literal
  Go-formatted string (`fmt.Sprintf("... is_state('%s', 'off')", enabler)`), NOT as a second `$2`
  placeholder — `buildConditionStateExpr` wraps EVERY `$N` in `sourceToJinja2(...)` (a
  `states(...)` call), which is right for `$1 not in [...]` (wants a value) but produces a
  syntactically broken doubly-wrapped `is_state('states(\'switch...\')', 'off')` when used for
  `is_state()`'s own first argument (wants a bare entity_id string). Caught by regenerating and
  reading the actual rendered YAML before considering the migration done — the generic parser/build
  tests alone would NOT have caught this (they don't render through `buildConditionStateExpr`
  unless a test explicitly does, which the fix's own regression test now does).
- **`switch.media`-style media_player→switch coercion** reuses `generateMediaSwitches`
  (`generator.go`) wholesale too — that function already dispatches on ANY `TEntityRecord` with
  `Identity.Domain=="switch"`, `Identity.Path` ending in `"/media"`, and `MediaSwitchPlayerName`
  set, regardless of which registration path produced it (the pre-existing hand-written
  `media_switch` directive already worked this way). New field added:
  `TEntityRecord.MediaSwitchAvailabilityEntityID`, threaded into `buildLeafMediaSwitchYAML`'s own
  new optional parameter, rendering `availability: "{{ is_state('<node-id>', 'on') }}"`. Detected
  by DOMAIN MISMATCH (declared `switch`, source entity's own domain `media_player`), not a
  dedicated keyword — any other mismatch warns clearly rather than guessing (on-demand extension
  discipline, matching the rest of this layer). Constraint worth remembering: this reuse only
  fires when the capability's own label produces a path ending in `/media` — true for every real
  case so far since the label itself is literally "media".
- Reverse lookup (media.media's own sibling "node" capability's resolved entity_id) never needs to
  defer, unlike discovery's own sibling lookups: `registerDevicePositioning` always auto-registers
  "node" synchronously the moment a logical device is positioned, before any of its other
  capabilities' own explicit Conceptual.def lines are even reached.

## Logical devices as a 6th device kind

Unlike "dependency on" (which only ever overlays an EXISTING physical device sharing the same id),
`utility.apple_tv`/`utility.tv` are brand-new ids with their own real capabilities — they needed
full positioning support, threaded as a 6th kind (`logicalDevicesByID`) through the same ~10 call
sites `commandlineDevicesByID` (the 5th kind) already flows through:
`Conceptual_DeviceCapabilityEntities.go`, `Conceptual_DevicePositioning.go`, `parser.go`,
`physical_rule_no_conflicting_device_names.go`, `generator.go` — purely mechanical parameter
threading, verified via full test suite + two manual spot-checks of the auto-patched test files.
`validateNoConflictingDeviceNames` gained a SECOND deliberate 2-kind exception (alongside the
existing hosts+commandline one): `{any-physical-kind, logical}` is fine (the same-id overlay case),
a 3rd kind sharing the id is still flagged.

## Verified

Full regenerate+diff of Vienna against a pre-change snapshot showed exactly the expected files:
the aggregate node list (+apple_tv's new node), `tv_node.yaml`'s own content (same entity_id,
correct fixed Jinja), the old `tv_node_raw.yaml` retired (the old macro's two-file
enabler-support shape collapsed into one, semantically equivalent), and both `*_media.yaml`
switches gaining their new `availability:` field. `go build/vet/test` all clean, 20 new unit
tests. **Deployed + verified live (2026-09-16)**: the `tv` node's enabler branch was confirmed via
read-only MQTT inspection (the gating smart plug was already off, so no live toggle was needed) —
node correctly stayed "on" via the enabler branch.

## Queued follow-up

Junglinster's own `media_player_device` variants (`tv` with `no_collect`+enabler, `sonos` with
`no_play_input`) are NOT migrated — `no_play_input` is already supported by the grammar/generator
(untested against a real house yet); `no_collect` isn't handled at all yet (need to figure out
what "exclude this switch from the room's collection" means for a logical-layer positioned entity
— nothing built or even designed for this yet).

## Bedroom sonos-roam migrated (2026-09-17)

Same mechanism applied to the bedroom's single Sonos Roam (`call media_player_device :sonos;
device infrastructural:sonos from node.sonos-roam;`) — new `appliance.sonos_roam` Logical.def
device (distinct id from living_room's own `appliance.sonos`), single `dependency on
node.sonos-roam;` (vs. the living_room pair's two).

## Real bug found and fixed live 2026-09-17: dependency-target fallback never actually materialized

`resolveLogicalDependencyNodeEntities`'s own fallback path (for a `dependency on <id>;` target
with no Conceptual.def positioning of its own — true for `node.apple_tv`, `node.tv`,
`node.sonos-left`, `node.sonos-right`, ALL FOUR already-deployed dependencies from the very first
migration) claimed in its own doc comment to produce "a real, live entity all the same" under a
default `physical/<raw-device-id>/node` name, but only ever called
`RegisterDiscoveryImpliedEntity` — bookkeeping-only (satisfies `checkEntityReferences`, skips a
customization file), never gives `generateHostsIntegrationOutputs` a `DeviceConceptualLinks` entry
to find. That function only emits a `conceptual: node_entity: ...;` block into
`coordinator/devices.yaml` when that link exists — without it, the coordinator never published
MQTT discovery for the fallback entity at all, so every `is_state()` check referencing it was
PERMANENTLY FALSE (an unknown entity's state can never match `'on'`).

**Impact, confirmed live on Vienna before the fix**: `binary_sensor.infrastructural_apartment_
living_room_apple_tv_node` and the living_room sonos-pair's own node were stuck `off` permanently;
`tv`'s own node only showed `on` by coincidence of its separate `or is_state(switch, 'off')`
clause (i.e. it was tracking "the TV's smart plug happens to be off" and calling that "available",
never anything about the TV or Apple TV itself). This had been live and silently broken since the
apple_tv/tv migration (2026-09-16) shipped — nobody noticed until the sonos-roam migration
prompted a live-state spot check.

**Fix** (`integration_logical_storage.go`): the fallback path now calls `registerHostNodeEntity`
(the same helper a real `device <spec> from <device-id>;` positioning line triggers) and stores
the result into `administration.DeviceConceptualLinks[depID]` itself — i.e. it actually PERFORMS
the positioning a human forgot to write, rather than just pretending to. Same entity_id as before
for all four pre-existing devices (`binary_sensor.physical_node_<id>_node` — nothing downstream
needed to change), just genuinely backed by real coordinator discovery now. Regression test added
in `Conceptual_LogicalEntities_test.go` (asserts `DeviceConceptualLinks` actually gets populated,
not just the bookkeeping-only entity record).

**Broader lesson**: this was the THIRD live incident this session where a "dependency on"-style
target not being properly positioned silently broke something (see
[[project_logical_layer_dependency_on]] for the first two, both "positioning line got dropped"
regressions on `infrastructural:sonos/left`/`right`/`appletv`/`sony_tv`). This time the root cause
was different — the positioning genuinely never existed, and the code's own "fallback" path for
that case was itself incomplete — but the SYMPTOM (a dependency silently not tracked, no warning
printed) is the same shape every time. Worth considering a dedicated validation rule some day that
catches "a dependency on target has no real backing entity" at generate time, rather than only
catching it via a live spot-check.
