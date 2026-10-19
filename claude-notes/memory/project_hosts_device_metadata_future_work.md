---
name: project-hosts-device-metadata-future-work
description: "Per-device constant-attribute overrides + live-reported device metadata (item 1) are now DONE — see project-mqtt-migration-plan. Only item 2 (per-integration default-icon grammar) remains open, unbuilt future work."
metadata:
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-08-22T08:54:43.328Z
---

Originally two future-work items noted 2026-08-21. **Item 1 is now built** (2026-08-22): per-device
constant-attribute overrides (`ConstantAttributes` in a device's `with:` block, `forced` flag),
live-reported device metadata via an enriched `Integrations/cpu/report` and a generated
`home_assistant`-type reporting automation, and the coordinator merging forced-DSL > live > default
with a live `TLiveDeviceInfoStore` that republishes discovery on update — all confirmed working
against the real Junglinster instance. Full detail lives in [[project-mqtt-migration-plan]]; this
memory is kept only for item 2, which is still genuinely open.

**2. Per-integration default icons (still open).** The "hosts" integration doesn't need explicit
icons in its discovery payloads (`device_class`/`state_class`/a hardwired per-attribute `Icon` in
`THostsAttributeSpec` already cover it — see `cpuAttributeSpecs`'s `mdi:cpu-64-bit` for "load").
But the user flagged that a future integration might want a *default* icon at the integration-type
level, and a generic `integration <name> with:` grammar option for that is a plausible extension
point. Not designed further, no integration needs it yet.

**How to apply:** don't build this speculatively. If asked, it's recorded in Architecture.md §13
(Open design questions) as a placeholder — no concrete grammar agreed yet, confirm with the user
before building.
