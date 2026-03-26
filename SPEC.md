** TODO

Active work items, roughly in priority order:

1] YAML generation — implement the generation data model and per-entity-type YAML emitters.

- Introduce a canonical in-memory model after parse+expand containing:
   - entity identity (type/sphere/path/raw),
   - space context (full path, nesting level),
   - definition/import status,
   - options (`providing`, `icon`, `open_stop_close`, `no_collect`, etc.),
   - dependencies/references used by generated templates.
- Record provenance (source file + line + originating macro) for diagnostics.
- Use this model as the sole input for YAML generation and list generation.
- Split generation into per-entity-type emitters: generate_sensor.go, generate_light.go, generate_switch.go, etc.

2] Generation policy for external entities.

- External without local options: do not generate core entity YAML.
- External with local options: generate customisation/configuration YAML only.
- Defined/imported entities: generate full entity YAML.

3] List generation — generate entity-card lists from `list … with …` statements.

- Parse `list` blocks in Lists.def.
- Apply `clean_prefix` / `clean_postfix` filters.
- Emit one list file per list declaration.

4] Harden macro parameter checking.

- Complete runtime checks for all declared parameter kinds (`boolean`, `path`, `option`, `set<...>`, etc.), not only `int` and `entityReference`.
- Add explicit unknown-parameter detection for `with:` blocks.
- Ensure `option` style flags are interpreted consistently (presence = true, absence = false unless defaulted).

5] Online availability checks (optional mode).

- Keep current offline mode as default.
- Add a mode that validates external entities against Home Assistant and reports `available / missing / unreachable`.

6] Tighten entity model cross-checks.

- Verify representative spaces in Vienna and Junglinster (root, top-level spaces, a few deeply nested spaces).
- Confirm raw entities are reported in bracket form (`type.[raw_name]`).
- Confirm contextual path expansion for relative entity specs is correct.
- Confirm `no_collect` markers are present where expected.


** Current State

Both homes (Vienna and Junglinster) are operational in the Go-based parser and expander.
Migration from the legacy bash-based implementation is complete and has been removed from the codebase.
Both homes share a single Macros.def under New/Shared/Definitions/.
The DSL uses `call` / `macro` syntax throughout; the legacy `create` / `creation macro` forms have been removed.

The current operational commands are:

- go run . interpret [Vienna|Junglinster]   — parse and report entity/space structure
- go run . expand [Vienna|Junglinster]      — expand macro invocations and report results
- go run . check [Vienna|Junglinster]       — availability check against Home Assistant

The next major milestone is YAML and list generation.


** Context

A YAML generator, written in Go, to generate:
- YAML-based templates for switches, lights, sensors, etc. that are complementary to the existing entities in Home Assistant installations.
- Lists of entities that can be included, via copy/paste, into entity cards in the Home Assistant UI.

The two homes supported are Vienna and Junglinster (Luxembourg), under New/Vienna and New/Junglinster respectively.
The generated YAML tree (hass/) is rsynced to the relevant Home Assistant instance by the Update script.

During generation the system checks prerequisites: definition files declare assumed entities that must already exist in the local and/or remote Home Assistant instance.

The intended processing order for the generator is:
1. Read the macro definitions (Shared/Definitions/Macros.def). These define what `call <name>` means.
2. Read the space and entity definitions (Entities.def). This establishes the full set of declared entities, their nesting in spaces, and their attributes (no_collect, providing node, open_stop_close, etc.).
3. Run structural and semantic checks on parsed definitions (including type/option checks) and collect warnings and errors.
4. Execute one single round of macro expansions in space context.
5. Run post-expansion checks and collect warnings (unresolved references, unused declarations, suspicious option usage).
6. Use the resulting fully-expanded entity model as the basis for YAML generation and list generation.

Single-pass expansion is used deliberately: macros that reference aggregate entity sets (such as "all lights in a space") would otherwise require fixed-point iteration. One pass keeps semantics predictable.


** Macro definitions

Macros extend the DSL with higher-level constructs that expand into one or more entity declarations and definitions.
All macros are collected in the shared Macros.def file and are read before any per-house definition files.

Macro syntax:

```
macro <name> [no_raw] [space_level] { $p_1 t_1 [op], ..., $p_n t_n [op] }:
   <body>
end;
```

Where:
- `<name>` — macro identifier
- `[no_raw]` — optional constraint: disallow raw entity specifications `[...]` in the target
- `[space_level]` — optional constraint: bind the macro target from the immediate surrounding space rather than an explicit entity target
- `{ $p_i t_i, ... }` — parameter set (order-independent when using option block form)
- `$p_i` — parameter name (used in macro body with `$` prefix)
- `t_i` — parameter type (see Supported types below)
- `[op]` — optional marker; omitted argument binds as empty (or macro default if present)

Supported parameter types:
- (default, omitted) — entity specification (`type.sphere:path` form); validated for proper syntax
- `entityReference` — a concrete reference to an existing or derived entity
- `string` — any text value
- `int` — numeric value; validated as parseable to int64
- `boolean` — true/false flag; missing = false, present = true
- `set<entityReference>` — comma-separated list of entity references
- `set<string>` — comma-separated list of string values
- `set<int>` — comma-separated list of integer values
- `path` — Home Assistant entity or node path; basic format checking

Implicit target binding:
- For regular macros, the entity target from the invocation is implicitly available as `$entity`.
- For `space_level` macros, the implicit target is the immediate surrounding space.
- The implicit target is resolved to extensional form before macro body execution.
- Macros do not declare `$entity` explicitly; it is automatically bound.

Example macro definitions:

```
macro battery_alert { $alert_level integer }:
   entity binary_sensor.infrastructural:$entity:battery_alert with:
      definition as condition sensor.infrastructural:$entity:battery_level "...";
      available always;
   end;
end;

macro power_switch { $node string, $threshold integer }:
   entity sensor.social:$entity:power;
   entity binary_sensor.social:$entity:consumes with
      definition as condition sensor.social:$entity:power "($ | int(0)) > $threshold";
   entity switch.social:$entity;
   call providing $entity/$node;
end;

macro light_device { $sphere string, $name string }:
   entity light.$sphere:$name;
   call providing $name/light;
end;
```


** Macro invocations

Positional form (parameters in declared order):
```
call battery_alert front_door/ring 20;
call power_switch /home/garden/lamp robb 1;
```

Option block form (parameters by name, order-independent):
```
call battery_alert front_door/ring with:
   alert_level 20;
end;

call power_switch /home/garden/lamp with:
   node robb;
   threshold 1;
end;
```

Inline form (when used as the body of a single entity declaration):
```
entity sensor.social:kitchen/temperature with call battery_alert kitchen/thermometer 20;
```

Flag options (without value) are boolean variables in macro scope:
```
call cover_device terrace/awnings with:
   open_stop_close;
   no_collect;
end;
```

Dynamic macro-name interpolation:
- A macro name may include an interpolation segment in angle brackets:
   `call <$type>_device physical:$e;`
- `$type` is the currently bound type value; it is inserted into the macro name token before lookup.


** For-in-do loops in macro bodies

Macro bodies support iteration over set-type parameters:

```
for ${element} in ${group} do
   entity light.physical:${element};
end;
```

The loop variable `${element}` is bound to each comma-separated value in `${group}` in turn.
Loops may be nested and may contain if-then-else constructs.


** Conditional constructs

Macro bodies support conditional inclusion based on whether a parameter is provided:

```
if "${alert_level}" is provided then
   entity binary_sensor.infrastructural:$entity:battery_alert;
else
   entity sensor.infrastructural:$entity:battery_level;
end;
```

The `is not provided` form is also supported:
```
if "${node}" is not provided then
   call providing $entity/default;
end;
```


** Entity specification model

Extensional forms (resolved, ready for generation):
- `<<type>>.<<sphere>>/<<path>>`
   Examples: `sensor.social/apartment/kitchen/door`, `switch.social/apartment/bathroom/washingmachine`
- `<<type>>.[<<raw-name>>]`
   Example: `sensor.[ems_esp_boiler_boiler_outside_temperature]`
   Verbatim Home Assistant entity id; not rewritten by context expansion.

Intensional forms (resolved against the current space context):
- `<<type>>.<<sphere>>:<<path>>`           → `<<type>>.<<sphere>>/x/<<path>>`
- `<<type>>.<<sphere>>:/<<path>>`          → `<<type>>.<<sphere>>/<<path>>`
- `<<type>>.<<sphere>>:<<path>>:<<sub>>`   → `<<type>>.<<sphere>>/x/<<path>>/<<sub>>`
- `<<type>>.<<sphere>>:/<<path>>:<<sub>>`  → `<<type>>.<<sphere>>/<<path>>/<<sub>>`

Where `x` is the current space path.
If the sphere component is omitted, the default sphere is `social`.

Entity namespaces:
- `physical` — entities in the house without an immediate social role
- `social` — entities with a direct role from a usage/social perspective
- `infrastructural` — entities pertaining to the IoT/smart-home infrastructure itself


** Entity definitions

Spaces group entities and provide path context:

```
space social/apartment/living_room:
   entity light.social:main;
   entity sensor.physical:temperature;
   call battery_alert netatmo 20;
end;
```

Entity declaration options:

`no_collect` — exclude from space-based aggregate collections:
```
entity light.social:main no_collect;
```

`providing node` — declare an infrastructure dependency:
```
entity light.social:carport with:
   providing node /house/server_room/zwave/027;
end;
```
The path after `node` forms: `binary_sensor.infrastructural:<path>:node`.

`definition as value` — derive entity value from another entity:
```
entity sensor.physical:garage_door:temperature with:
   definition as value sensor.physical:boiler:outside_temperature;
end;
```

`definition as condition` — derive a binary sensor from a condition:
```
entity binary_sensor.infrastructural:netatmo:node with:
   definition as condition sensor.infrastructural:netatmo:reachability "$ == 'True'";
end;
```

Multi-parameter conditions use `$1`, `$2`, etc.:
```
definition as condition $sunnyThreshold $sensor "($1 | int) < ($2 | int)";
```

`open_stop_close` — cover entity with three operations:
```
entity cover.social:front with:
   open_stop_close;
end;
```

`available always` — entity is unconditionally available:
```
entity binary_sensor.infrastructural:$entity:battery_alert with:
   available always;
end;
```

Alias binding:
```
entity [sensor.ems_esp_boiler_boiler_outside_temperature] as boilerOutsideTemp;
entity sensor.physical:garage_door:temperature with:
   definition as value boilerOutsideTemp;
end;
```
Alias scope is lexical by space tree: visible in the declaring space and all nested sub-spaces.

Set syntax for macro arguments:
```
call zigbee_group light.social:main with group { kitchen, middle, living };
call zigbee_group light.social:left with group { 1, 2, 3, 4, 5, 6 };
```

Virtual spaces:
```
virtual space social/apartment/virtual_all_lights:
   member switch.social:/apartment/living_room;
   member switch.social:/apartment/bedroom;
end;
```
Virtual spaces are semantically distinct from regular spaces.
During generation they use the virtual-local aggregate policy rather than the full regular-space aggregate set.

Built-in DSL statements:
- `lights_motion_guarded` — parses directly to automation rules; not a macro invocation.


** Aggregate entity derivation

When a space is closed:
- If the space has a lights-on list, `light.<spaceName>` is derived and propagated to the parent space's lights-on list.
- If the space has a space-off list, `switch.<spaceName>/space` is derived and propagated to the parent space's space-off list.
Both derived entities are deduplicated.


** Definition files layout

Per-house files under New/<<house>>/Definitions/:
- Main.def      — include order entrypoint
- Settings.def  — per-house settings and global variable overrides
- Server.def    — server/bridge conditional definitions (at-home vs. away)
- Bridges.def   — integration bridge definitions
- Entities.def  — space and entity declarations
- Lists.def     — list declarations

Shared files under New/Shared/Definitions/:
- Macros.def    — all macro definitions (shared across houses)
- Settings.def  — global defaults


** Generated output layout

New/<<house>>/hass/ — generated YAML tree, rsynced to the Home Assistant instance.
New/<<house>>/Expansion.txt — macro expansion report.
New/<<house>>/Availability.txt — external entity availability report.
New/<<house>>/Collections.txt — entity collection report.
