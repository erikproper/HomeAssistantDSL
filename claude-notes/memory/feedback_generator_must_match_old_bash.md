---
name: feedback-generator-must-match-old-bash
description: "The Go generator must reproduce every \"implied\" entity the old bash generator (Old/*/Modules/Generate.1.hass) auto-created for a space — never paper over a gap by hand-declaring the entity in a house's Definitions/Entities.def."
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
---

Missing generator features must be fixed by porting the logic from `Old/<House>/Modules/Generate.1.hass`
into the Go generator (`generator.go` and friends), not by adding compensating `entity ...` declarations
to `New/<House>/Definitions/Entities.def`.

**Why:** A prior session found that entities like `input_number.social:.../temperature_desired`,
`heating_target_preset_for_*`, `input_boolean.../heating/enable`, `.../heating/leakage/ignore`,
occupation booleans (`occupation/home`, `/around`, `/away`), and the coverage/heating/vacuuming
`input_datetime` workday/holiday pairs were all **auto-generated ("implied")** by the old bash
generator for any space that qualified (e.g. any space with a climate entity or heating switch;
global ones at the root). Instead of porting that logic, they were manually added as `entity ...`
declarations into `New/Vienna/Definitions/Entities.def`. The user considers this "cheating" — it
masks the real gap, silently diverges Vienna from Junglinster (which never got the same manual
patch, hence a pile of "unaccounted entity reference" warnings), and means the DSL author has to
hand-maintain boilerplate the generator is supposed to produce.

**How to apply:** When investigating why a referenced entity is "unaccounted" / missing, always
check `Old/<House>/Modules/Generate.1.hass` (and `Settings*.hass`, `Definitions*.hass`) for the
matching `Generate*`/`MaybeGenerate*` function before concluding it needs a DSL declaration. Port
the function's logic into the Go generator so it reproduces the same output for *every* qualifying
space (or globally, for root-level functions), across both Vienna and Junglinster. Only if the old
generator itself required an explicit per-room declaration (rare — check `HasProperty`/`entity
declare` calls carefully) is a `Entities.def` entry the right fix. See [[project-heating-generator-port]]
for the specific functions ported/pending.
