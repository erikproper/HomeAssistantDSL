---
name: project-logical-layer-introduction
description: "Started 2026-09-16: Spaces.def renamed to Conceptual.def (both houses) and Vienna got a new, currently-empty Logical.def, wired into Main.def. Read this before the next item-4-style migration on Vienna — check whether the device belongs at the logical layer instead of physical."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-16T05:28:10.724Z
---

## What changed (2026-09-16)

- Both houses' `Spaces.def` renamed to `Conceptual.def` — purely a filename/editing-convenience
  choice, no semantic change (the DSL's own "conceptual layer with: ... end;" keyword, not the
  filename, is what the parser keys off — `layers.go`'s `collectLayerContent` already took a
  filename-candidate LIST per layer, so only ONE hardcoded Go string needed updating:
  `generator.go`'s `entitiesPath`/`collectLayerContent` call, "Spaces.def" → "Conceptual.def").
  Verified byte-identical generated output for both houses after the rename (Junglinster's own
  diff showed only the already-known, unrelated meta-reload automation content from
  [[project_meta_reload_servicenotfound_fix]], not deployed there yet).
- Vienna's `Main.def` gained `include Logical.def;` (between Physical.def and Conceptual.def) and
  a new `Logical.def` file, currently just:
  ```
  logical layer with:
  end;
  ```
  `LayerLogical = "logical"` already existed as a recognised layer constant in `layers.go` (valid
  since way earlier), but nothing has ever called `collectLayerContent(..., LayerLogical)` — the
  parser accepts a "logical layer with: ... end;" block syntactically, but no generator-side
  collection/generation exists for it yet. Junglinster did NOT get a Logical.def yet (not asked,
  not yet in scope — Junglinster's own migration hasn't started).

## Why (the user's own framing)

While doing Vienna's item-4 (physical-layer, discovery_passthrough) migrations one device at a
time, the user is now going to also scan Spaces.def/Conceptual.def from the top for "things that
should already have a definition at the logical layer" — e.g. `call media_player_device
:apple_tv;` / `call media_player_device :tv with: enabler switch.social:tv; delay_off
00:01:00; end;` are logical/derived devices (a media_player + a switch relationship), not raw
physical hardware, and don't belong being expressed via the physical/discovery mechanism at all.

The stated strategy is explicitly **incremental/on-demand**: extend the logical layer's grammar
and generator support only when a concrete migration actually needs it, not build the whole
mechanism speculatively up front. Devices defined at the logical layer should, at the CONCEPTUAL
layer, be positioned exactly the same way a physical device is (`device <spec> from
<logical-device-id> with: entity ... from <capability>; end;`) — only the generation semantics
differ (a logical device is expected to produce YAML templates, not a discovery/hassbridge
relay).

## Sequencing (the user's own stated plan)

1. Finish Vienna's physical-layer (item-4) migrations one device at a time, AND opportunistically
   build out the logical layer whenever a genuinely logical device is encountered along the way
   (media_player_device-style macros are the first flagged candidate, not yet migrated).
2. Once Vienna is done, move to Junglinster with the same one-by-one Conceptual.def-driven
   strategy, additionally covering things Vienna doesn't have: Zigbee2MQTT devices (first needs
   turning on `homeassistant.physical` → `homeassistant` relaying there, per
   [[project_discovery_device_block_and_node_capability]]'s own "Junglinster differences" note),
   Z-Wave devices (moved one by one from whatever "winsock" bridge to MQTT), Overkiz-based Somfy
   blinds, and likely more found along the way.

No logical-layer grammar/generator design exists yet — that's deliberately deferred until the
first real candidate is actually migrated.
