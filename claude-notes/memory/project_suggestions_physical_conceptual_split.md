---
name: project-suggestions-physical-conceptual-split
description: "Future work -- split suggestions/ into suggestions/physical/ (as-is) and suggestions/conceptual/ (declared-but-unused gap), queued 2026-09-17"
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-17T09:29:50.838Z
---

Future work, requested by the user 2026-09-17 while discussing Rule 3 (physical_rule_power_implies_consumes.go): split today's flat `suggestions/` output into two directories.

- `suggestions/physical/` -- today's existing "as is" suggestions unchanged (e.g. `suggestions/discovery.txt`'s `buildDiscoverySuggestionReport`, kind-3's `buildSuggestionReportFromExistence`): raw gateway/leaf existence vs what Physical.def's own capability declarations already name.
- `suggestions/conceptual/` -- a NEW report based on the gap between what's *available* (capabilities already declared on a Physical.def/Logical.def device, whether atomic or derived) versus what's actually *used* -- i.e. positioned -- at the Conceptual layer. This is the class of gap `declaredDiscoveryLeaves` (mqtt_discovery_existence.go, added 2026-09-16/17 to stop already-labelled-but-unpositioned leaves from reappearing in the physical suggestions report) already has to compute internally to do its own exclusion -- the same computation, surfaced as its own suggestion report instead of just an exclusion filter.

**Why:** the user wants this distinction structurally, not just a side effect of one report's filter -- a capability that's declared but never positioned (e.g. `discovery.living_room_tv`/`discovery.living_room_kitchen_nespresso`'s own `power`/`consumes` capabilities, or `environment.sun`'s `solar_azimuth`/six `next_*` sensors before they were positioned 2026-09-16) is a genuinely different kind of "thing to look at" than a brand-new, never-labelled raw leaf. Rule 3 (and Rule 2 before it) can only warn about a capability MISSING from Physical.def's own declaration -- they say nothing about a capability that's declared but sitting unpositioned, which is exactly what a `suggestions/conceptual/` report would surface.

**How to apply:** when picking this up, look at `declaredDiscoveryLeaves`/`usedDiscoveryLeaves` (mqtt_discovery_existence.go) as the reusable building blocks -- the "declared" side is already computed per-gateway from `TDiscoveryGatewayDevice.Capabilities`; the "used" side already exists via `discoveryEntityLinks`/`administration.DiscoveryEntityLinks`. A `suggestions/conceptual/discovery.txt`-style report would just be: for each gateway, `declared - used`, formatted as ready-to-position `entity <domain>.<sphere>:<path> from <capability>;` lines (mirroring the existing physical report's copy-paste-ready style) rather than raw Physical.def capability lines. The same gap likely also applies to hassbridge/import/logical device kinds, not just discovery gateways -- worth checking whether a house-agnostic version generalizes cleanly across all of PROJECT.md item 4's five (now six, with the logical-layer mechanism) device kinds, or whether discovery is the only one where "declared but unpositioned" is common enough to be worth a report.

Not started -- purely a design note, no code written yet.
