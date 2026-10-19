---
name: project-flat-entity-form-ban
description: "Resolved 2026-10-05: zero remaining flat 'entity <spec> from <device-id> <capability>;' usages in either house — the parser-level hard-error ban (PROJECT.md item 11) can now actually be implemented."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-10-05T13:47:49.104Z
---

User decision 2026-09-18: the standalone flat "entity <spec> from <device-id> <capability>;" line
(outside any "device <spec> from <device-id> with: ... end;" block) should be disallowed from the
DSL grammar entirely — a real parser-level rule, not just a style convention.

**Vienna: DONE.** Both prior occurrences fixed by merging into their device's existing/new block:
- `node.vienna_terrace_wind`'s wind_direction/wind_speed → merged into its existing
  `netatmo_windmeter` block, zero rename. (This device is then immediately superseded by a
  separate, same-day follow-up: renamed to `sensors.vienna_terrace_wind` and given real Logical.def
  capabilities — see [[project_logical_layer_windy_derived_condition]] if that memory exists yet,
  or Vienna's own MigrationNotes.def.)
- `environment.sun`'s 10 lines → merged into one new block on its existing bare positioning
  (9/10 entity_ids preserved exactly; "daylight" renamed to pick up the "sun/" prefix its 9
  siblings already carried — accepted, downstream consumer auto-updated).
Verified via diff both times — see Vienna's own MigrationNotes.def for the full writeup.

**Junglinster: 4 of 5 done, 1 frozen.** `environment.sun` (10 lines, same treatmeant as Vienna)
and `utility.heater`'s garage_door/temperature (wrapped in a brand-new `device infrastructural:
garage_door from utility.heater with: ...;` positioning, zero rename) are done and verified via
diff. **Frozen, explicitly, 2026-09-18**: the 3 bedroom "health" lines (sarah/david/master
bedroom, `entity sensor.social:health from node.<bedroom> sensor.health;`) each carry a real,
deliberate 2026-09-09 comment explaining they're kept OUTSIDE their netatmo device's own "with:"
block specifically so the device-name-path sugar does NOT apply (to avoid renaming "health" into
"netatmo/health"). This directly conflicts with a blanket ban. User's call: **freeze for now, fold
into Junglinster's own migration task** (whatever that project ends up being) rather than resolve
it today.

**Update 2026-09-24:** the "4 of 5" count above was stale/incomplete — `sensors.house_laundry_kitchen_relais_box`'s own "reservoir/warning" flat line (`entity binary_sensor.social:reservoir/warning
from sensors.house_laundry_kitchen_relais_box binary_sensor.input1;`) was a 6th instance never
tracked here. User fixed it directly: merged into a new `device from sensors.house_laundry_kitchen_relais_box with: entity binary_sensor.social:reservoir/warning from binary_sensor.input1; end;`
block. This ALSO fixed a real live bug: the coordinator's main-instance existence check was
flagging `binary_sensor.infrastructural_house_laundry_kitchen_relais_box_node` as confirmed not to
exist — confirmed resolved (regenerate clean, entity present in coordinator/main_entities.yaml).
Re-ran the grep check below on both houses 2026-09-24: **Vienna is fully clean (zero remaining
flat-form usages); Junglinster now has ONLY the 3 frozen bedroom-health lines left** — no other
untracked instances remain in either house.

**Resolved 2026-10-05**: re-ran the exact check below on both houses ahead of PROJECT.md item 11's
code-cleaning pass — **zero remaining flat-form usages in either house**, including the 3 frozen
bedroom-health lines (they were collapsed to the ordinary nested bare no-rename shorthand as a side
effect of the 2026-09-27 mandatory-domain-qualification cleanup, not a dedicated fix for this item).
The blocking condition below is now satisfied — **the hard parser-level ban can actually be
implemented now**, whenever PROJECT.md item 11 is picked up. Re-check immediately before
implementing (a lot can change between sessions):
`grep -nE "^\s*entity\s+\S+\s+from\s+\S+\s+\S+;" Definitions/Conceptual.def | grep -vE "no_collect;$"`
in each house — must come back empty on both before adding the hard error, same as this note
always said.
