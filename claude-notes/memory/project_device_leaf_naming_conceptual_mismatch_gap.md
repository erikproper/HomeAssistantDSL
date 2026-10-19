---
name: project-device-leaf-naming-conceptual-mismatch-gap
description: "PARTIALLY DONE 2026-09-19: added 'sphere::path' (double colon, empty override) meaning 'space context, skip device leaf' -- fixes wind_speed/wind_direction/windy/sunny on Vienna. A third form ('sphere:leaf:path', explicit replacement leaf) is still deferred."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-18T22:23:56.129Z
---

User's own diagnosis (2026-09-19), verbatim because it's the right framing: "This is caused by
treating the devices as a de-facto space, and then the migrations. The problem is that sometimes
the infrastructural names deviate from the ones one would use from a conceptual perspective."

Concrete trigger: `sensor.social:wind_speed`/`wind_direction` (Conceptual.def, under `device
infrastructural:netatmo_windmeter from sensors.vienna_terrace_wind with: ...;`) resolve to
`sensor.social_terrace_netatmo_windmeter_wind_speed`, not the `sensor.social_terrace_wind_speed`
the user expected -- because `namingSpacePath`/`specHasExplicitSpherePath`
(Conceptual_DeviceEntities.go) only offers two naming modes for the `sphere:path` colon form:

1. **Relative** (`sphere:path`, no leading slash): space context AND the enclosing device's own
   leaf (`deviceSpecLeafPath`, e.g. "netatmo_windmeter") both get folded in.
2. **Absolute** (`sphere:/path`, leading slash): neither space context nor device leaf apply at
   all -- a fully bare override, used for e.g. Junglinster's laserjet precedent
   (`sensor.social_status`, not `sensor.social_laserjet_status`).

There is NO third form meaning "keep the space context (e.g. 'terrace'), but skip the device's own
leaf (e.g. 'netatmo_windmeter')" -- exactly the case the user actually wants for
wind_speed/wind_direction (real physical measurements naturally named at the space level, same
flat pattern as `sensor.social_terrace_battery_level` etc.), and, per the user's own broader
framing, likely other cases where the "device block acts like a space"
([[project_for_device_shorthand]] / PROJECT.md item 5) convention's own naming folds in an
"infrastructural" internal device name that was never meant to be part of the conceptual-layer
name at all.

Also affects `binary_sensor.social:daylight` (Physical.def's "derived binary_sensor.daylight from
sensor.solar_elevation via jinja...", positioned under `device infrastructural:sun from
environment.sun with: ...;`) -- resolves to `binary_sensor.social_sun_daylight`, an
ALREADY-ACCEPTED rename from 2026-09-15 (MigrationNotes.def:588), not a new instance, but the same
underlying mechanism.

**Not a registry issue** -- confirmed `coordinator/imported.yaml`'s own `local_entity:` values
match exactly what HA would show; the generator and the deployed config are consistent with each
other. The mismatch is purely between what the DSL author's mental model expects from the literal
`sphere:path` text and what the folding rule actually produces.

**Fixed 2026-09-19** (the day after "we need to fix this tomorrow" -- user came back with a
concrete syntax proposal rather than leaving it to me to design): a new `sphere::path` form (two
colons, empty leaf-override segment) between the sphere and the path. `hasEmptyDeviceLeafOverride`
(Conceptual_DeviceEntities.go) detects it; `namingSpacePath` treats it exactly like
`deviceNamePath == ""` (returns spacePath unextended) when present. No change needed in
`normalizeEntityFullName` itself -- it already resolves a lone leftover colon in the path portion
to an empty, trimmed leading segment, so the second colon just disappears harmlessly once
`namingSpacePath` stops folding the device leaf in. New test
`TestNamingSpacePathSkipsDeviceLeafForDoubleColonForm`.

Applied live to Vienna: `sensor.social::wind_speed`/`wind_direction`,
`input_number.social::windy_threshold`/`sunny_threshold`, `binary_sensor.social::windy`/`sunny` --
all now resolve flat (`sensor.social_terrace_wind_speed`, `binary_sensor.social_terrace_windy`,
etc.), reverting the "accepted" long-name renames from the day before
([[project_logical_layer_defined_and_derived_capabilities]]). Verified via a before/after
regenerate diff (single-colon vs double-colon Conceptual.def): the only changes are the four
renames plus the downstream `binary_sensor.social_covers_should_be_closed` automation's own
`is_state()` references picking up the new names correctly. `daylight` (the `binary_sensor.social:
daylight` under `device infrastructural:sun`) was NOT touched -- it's a top-level device, not one
of the confusing cases, and its existing name is already correct/accepted.

**Still deferred**: the THIRD form, `sphere:leaf:path` (an explicit REPLACEMENT leaf, not just "no
leaf") -- e.g. for a case where the device's own real leaf is wrong but simply dropping it
entirely isn't what's wanted either. Not implemented; revisit `stripDeviceLeafOverride`-style
parsing (extracting a middle segment, not just detecting emptiness) if a real case comes up.
Vienna's own change here is NOT yet deployed (regenerated + verified locally only) -- Junglinster's
`Definitions/Conceptual.def` hasn't been touched at all for this yet, per the user's own "see if it
works there, we can then fix the other ones" sequencing.
