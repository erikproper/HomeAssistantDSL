---
name: project-envoy-power-energy-naming-cleanup
description: "Known future cleanup — Envoy's raw social_power_panels_production_total entity is an energy reading, not power, and the whole power_total/current_power naming needs revisiting"
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-08-25T21:58:32.666Z
---

Flagged 2026-08-25, explicitly deferred — user said "keep your hands off these for now, other
than reminding me we need to fix these."

The real (cloud-based) Envoy sensor `social_power_panels_production_total` reports a cumulative
value in MWh (7.353 MWh observed) — that's an **energy** reading, not power, despite "power"
being baked into its name. Its sibling `social_power_panels_production_current` (0.000 kW) is a
genuine instantaneous power reading. Meanwhile Spaces.def's own conceptual entities
`sensor.social:total_power`/`sensor.social:current_power` are currently both typed as generic
`power`/W/measurement via [[project_new_architecture_plan]]'s `defaults:` mechanism
(Shared/Definitions/Defaults.def) — deliberately kept generic per the user's correction ("the
defaults are OK... it is just envoy that has another view").

**Why**: the entity/capability naming here (both the raw Envoy sensor name and the DSL's own
`total_power`/`current_power` postfixes) predates the realization that "total" here actually
means "lifetime energy produced," not "current total power" — a naming mismatch, not a typing
bug in the defaults mechanism itself.

**How to apply**: when this actually gets tackled (not proactively), the fix is almost certainly
two-part: (1) rename the DSL-side entities/postfixes to something that doesn't say "power" for an
energy value (e.g. something under a `total_energy`/`lifetime_energy` postfix, consistent with
the envoy device's other sensors: `energy_production_today`, `energy_production_last_seven_days`,
`lifetime_energy_production`), and (2) once the Envoy capability line is actually wired into
Physical.def's `hass.envoy` device, give *that specific capability* its own per-capability
`with: ...; end;` override (device_class energy, unit MWh or kWh, state_class
total_increasing) rather than touching the shared `defaults:` rule, which should stay generically
power-typed for the general "power" postfix family.

**General unit principle** (user's own framing, 2026-08-25): Power is measured in W/kW; Energy is
measured in Wh/kWh/MWh (Watt-*hours*, not "W/h"). This distinction is the general rule for
picking device_class/unit when writing or reviewing any `defaults:` rule or per-capability
override for a power- or energy-flavoured sensor — but the *generic* rule (what a `defaults:`
rule should say) and the *actual instance's* real reported unit (what a specific device, like
Envoy, genuinely emits) must each be checked independently; don't assume they match. This Envoy
case is the concrete example: a "total" postfix generically reads as power, but this specific
instance actually reports cumulative energy in MWh.
