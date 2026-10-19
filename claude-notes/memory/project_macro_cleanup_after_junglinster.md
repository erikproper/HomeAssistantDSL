---
name: project-macro-cleanup-after-junglinster
description: "DONE 2026-09-30: every macro-call site in both houses migrated off, then the entire macro-definition/expansion engine removed from the generator and Macros.def deleted from disk. Superseded by this final state; see below for what actually happened and the historical inventory for reference."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-10-04T19:48:58.876Z
---

**DONE 2026-09-30, full closure of PROJECT.md's macro-cleanup item**: every `.def`-level macro
call this memory used to track as "still blocked on Junglinster" is now migrated off, and the
generator-side machinery is gone too.

## What actually happened, in order

1. Junglinster's own device-by-device migration (this session, spanning several days) worked
   through every remaining macro family: `providing`, `battery_level_device`, `media_player_device`
   (the last one closed out 2026-09-29/30, including office's `apple_tv`/`tv`/`sonos` and the
   genuinely-shared `cycling` enabler switch), `light_device`, `battery_level`, `switch_device`,
   `thermostat`, `battery_alert`, `power_switch`. Also resolved along the way, not technically
   `Macros.def` macros but the same "legacy DSL construct" class: `definition as switched_device
   ...;` (all 7 Junglinster sites, via the "is switched by" Logical.def mechanism) and the 3 frozen
   flat-form "health" lines (already normalized into their own netatmo device blocks before this
   session's final sweep).
2. Confirmed empirically, safely, before touching any code: emptied `Macros.def`'s own content
   (kept the file) and regenerated both houses -- byte-identical output, zero warnings, proving
   nothing in either house's live `.def` tree depended on macro expansion any more, not even the
   handful of macro definitions still physically present in the file.
3. Removed the macro-definition/expansion engine from the generator itself:
   - `expander.go` shrank from ~2574 to ~690 lines -- deleted `TMacroExpansionContext`/
     `TParsedCreationMacro`/`TMacroInvocation`/`TMacroParameter`/`TParameterKind` and every
     function under them (`ParseCreationMacro`, `ExpandMacro`, `ParseMacroInvocation`,
     `collectExpandedEntityRecords` and its own private scan helpers, validation helpers,
     `loadMacroContext`, etc.). Kept everything genuinely shared (`normalizeEntityFullName`,
     `extractEntityIdentity`, `parseConditionDirective`/`parseConditionSpec` -- confirmed these are
     called directly from `parser.go` for the still-live bare-`condition` mechanism, independent of
     macros -- space-path helpers, `resolveSettingsVar`/`substituteSettingsVariables` for
     Settings.def variable resolution).
   - `parser.go`: removed every "call X;"/inline-macro-invocation handling path; changed
     `ParseEntitiesAndFillAdministration`'s own `ctx *TMacroExpansionContext` parameter to a plain
     `settings map[string]string` (the ONLY thing real callers still needed from that context --
     confirmed via full caller audit, every test call site already passed a bare empty
     `&TMacroExpansionContext{}`).
   - `generator.go`: removed the `loadMacroContext` call site, now just loads Settings.def directly
     into a plain map.
   - 8 macro-engine-internal unit tests deleted from `main_test.go` (they tested
     `ValidateInvocationParameters`/`ExpandMacro`/etc. directly); every other test's own
     `&TMacroExpansionContext{}` passthrough mechanically replaced with `nil`.
4. `Macros.def` deleted from disk entirely (backup kept:
   `Shared/Definitions/Macros.def.bak_2026-09-30`). Regenerated both houses one more time with the
   file completely gone -- byte-identical again, full test suite green throughout every stage.

**Not deployed** -- this is a pure generator-code change with zero generated-output difference, so
there's no live-instance impact either way, but it went through the same regenerate+byte-diff+full
test suite discipline as every `.def` change this session.

## Historical inventory (superseded, kept for the record)

<details>
<summary>Original 2026-09-22 snapshot, before Junglinster's own migration closed these out</summary>

- DONE 2026-09-22 (separately, earlier): Macros.def's `zwave_node`/`windy`/`sunny` + the dead
  `hasSecondColonSeparator` parser check removed first, as a smaller warm-up pass.
- At that point: Vienna 0 / Junglinster still substantial for `providing`(39)/
  `battery_level_device`(45)/`media_player_device`(12)/`light_device`(11)/`battery_level`(9)/
  `switch_device`(9)/`thermostat`(9)/`battery_alert`(4)/`power_switch`(4) -- all since migrated to
  zero, see above.
- `definition as switched_device ...;` -- Vienna 0, Junglinster 7 -- since migrated via the "is
  switched by" Logical.def construct, see [[project_logical_layer_device_combining]].
- The frozen flat-form "health" lines (3, sarah/david/master bedroom) -- since normalized into
  their own netatmo device blocks.

</details>

## Still explicitly NOT part of this cleanup

`discovery_passthrough` (Physical.def's own passthrough rule) -- see
[[feedback_keep_discovery_passthrough_mechanism]]: this is reusable architecture for the next
raw-device migration scenario, not dead code, and was never in scope for this item.
</content>
