---
name: project-logical-literal-reference-ordering-bug
description: "A device block's own \"node\" line must come AFTER any entity line that positions the raw entity a derived condition references literally, or RegisterExternalEntityReference races the real declaration and falsely flags a duplicate final entity id."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-25T06:05:51.930Z
---

Real bug found live 2026-09-25 migrating Junglinster's living-room TV (`appliance.tv_living_room`, `media_player_device :tv with enabler ...`).

**The bug:** `administration.RegisterExternalEntityReference` (administration.go) is supposed to skip registering a literal jinja reference as an "external reference" when a real entity declaration already claims that same final entity_id — but it only checks `EntityRecordsBySpace` for what's *already been processed*, at the moment it's called. A literal token (e.g. `jinja "..." { media_player.social_house_living_room_tv }` inside a `derived binary_sensor.node` condition) resolves immediately, no deferral possible (`resolveLogicalSiblingSourceEntityID`'s `isLiteral` fallback always succeeds on the first pass). So if the device block's `node` line is declared *before* the line that actually positions that same entity (`entity media_player.social: from core;`), the literal reference registers first — finding no matching record yet — and the later real declaration then collides with it: `"entity id ... is produced by 2 separate declarations"`.

**Why it doesn't show up for `apple_tv`/`sonos_garage`:** those devices' own `node` condition uses `physical X and node.Y` (ping + local-availability), never a literal reference to the media_player's own raw entity_id — so there's nothing to race. Junglinster's TV needed the literal form specifically because there's no local-kind ping counterpart backing its own availability (see [[project_junglinster_migration_starting_checklist]] and the ongoing per-device migration sweep this session).

**How to apply:** whenever a `derived` capability's own condition references another entity of the SAME device literally by its final entity_id (not a sibling capability label), put that entity's own positioning line BEFORE the derived/node line in Conceptual.def's device block — order matters here even though the generator is order-independent almost everywhere else. Relevant for the still-pending `:sonos` migration (its own `no_play_input` media_player_device call likely needs the same literal-reference treatment, per the user's own note to "analyse the macro jointly" for that one). Also relevant: `switch.media`'s own registration (`registerLogicalMediaSwitchCapability`) separately defers until `node` has resolved (needs `node`'s entity_id for its own availability field) — so never have `node`'s own condition reference the `media` SIBLING label either, that's a genuine unbreakable circular deadlock under this generator's single-retry-pass design (fixed for the TV by referencing the media_player's raw entity_id literally instead of the `media` sibling).
