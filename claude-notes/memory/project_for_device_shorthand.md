---
name: project-for-device-shorthand
description: "DSL sugar added 2026-08-28 -- \"for <device-id>: entity <spec> from entity <capability>; ...; end;\" abbreviates repeated device-capability-entity lines."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-08-29T20:10:42.455Z
---

New Spaces.def grammar, requested and built 2026-08-28: a `for <device-id>: ... end;` block
abbreviates a run of `entity <spec> from <device-id> entity <capability>;` lines
([[project_entity_existence_inquiry_design]]'s construct -- Conceptual_DeviceCapabilityEntities.go)
that all repeat the same device id. Inside the block, each line drops the device-id token:
`entity <spec> from entity <capability>;`.

**Why:** the user was hand-writing several capability lines for `hass.laserjet` and noticed the
device id repeated on every line; asked for an abbreviation, by example, and asked me to also
convert the real Spaces.def's other repeated blocks.

**Implementation**: purely textual sugar, expanded inline inside parser.go's existing single-pass
main loop (a `forDeviceID` state flag, not a stack -- no nesting support, none needed). Two new
regexes/helpers live in Conceptual_DeviceCapabilityEntities.go (co-located with the construct
they're sugar for): `extractForDeviceHeader` (matches the `for <device-id>:` header) and
`expandForDeviceShorthandLine` (rewrites the shorthand line back to the full
`entity <spec> from <device-id> entity <capability>;` shape before handing it to the ordinary
dispatch chain) -- so there's no new registration path to drift from
`registerDeviceCapabilityEntityLink`.

**How to apply**: when a device (`hass.<id>`) has 2+ `entity ... from <device-id> entity ...;`
lines in the same space, prefer wrapping them in a `for <device-id>: ... end;` block. Scope is
narrow and deliberate -- only this one construct gets the shorthand (not device positioning,
`with: all entities`, or the `as <source>` construct), matching exactly what was asked, not a
speculative generalization.

**Verified**: unit-tested (`Conceptual_DeviceCapabilityEntities_test.go`:
`TestForDeviceBlockRegistersSameAsExplicitDeviceIDLines`,
`TestForDeviceBlockWarnsOnUnrecognisedLineAndKeepsParsing`) and confirmed live -- an A/B
`./generate` file-list checksum comparison (old explicit syntax vs. new `for` block) on Junglinster's
real Spaces.def was byte-identical. Real Spaces.def converted at all three sites that had it:
`hass.laserjet`, `hass.davids_bedroom`, `hass.office_corridor`.

**Unrelated finding during the A/B test**: a handful of `*laserjet_node*.yaml` files that exist in
git history are currently absent from the generated tree -- confirmed via the A/B test to be
pre-existing drift from earlier the same session (not caused by this feature, byte-identical either
way). Not investigated further; flag if it resurfaces as a real question later.

**Real, structural bug found live 2026-08-29 (unrelated to the shorthand syntax itself, but
surfaced through it): typing metadata was never resolved for this construct at all.** After
deploying an unrelated cosmetic change (dropping "infrastructural" from device display names), the
user reported every capability positioned via `hass.davids_bedroom`'s `for hass.davids_bedroom:
...;` block had lost its `state_class`/`unit`/`icon` in HA, and the connectivity `node` binary_sensor
showed "unknown" with no connectivity icon. Root-caused to `registerDeviceSourceEntityLink`
(`Conceptual_DeviceSourceEntities.go` -- shared by the `entity ... as <source> from <device-id>;`
construct AND, via `registerDeviceCapabilityEntityLink`'s dispatch, the `entity ... from
<device-id> entity <capability>;` construct the `for` shorthand expands into): it **never** called
`resolveCapabilityDefaults` at all, unlike the sibling `with: all entities;` bulk-import path
(`registerHassBridgeDeviceImpliedEntities`, `Conceptual_DeviceEntities.go`), which always has.
Confirmed via the generated `homeassistant_bridge.yaml`: every capability on every hassbridge
device (not just davids_bedroom) was missing `device_class`/`unit`/`state_class`/`icon` entirely --
a gap present since this construct was first built (days earlier), not a new regression from the
display-name change, just newly visible because that deploy's retire+republish flurry made the
user actually look. Fixed: `registerDeviceSourceEntityLink` now resolves typing the same way the
bulk path does -- preserves any explicit `device_class`/`unit`/`state_class`/`icon` Physical.def
already declared directly on that capability, gap-filling the rest via
`resolveCapabilityDefaults(administration.CapabilityDefaults, identity.Domain, key)`. Verified live:
`hass.davids_bedroom`'s `co2` now carries `device_class: carbon_dioxide`/`unit: "ppm"`/
`state_class: measurement`, `node` carries `device_class: connectivity`; confirmed the same fix
applies across every hassbridge device (`office_corridor` etc.), not just the one reported. Two
regression tests added (`TestDeviceCapabilityEntityResolvesTypingDefaults`,
`TestDeviceCapabilityEntityExplicitTypingWinsOverDefaults` -- the latter confirms explicit
Physical.def-declared typing still wins over a conflicting default rule, same precedence the bulk
path already guarantees). Not yet deployed live.
