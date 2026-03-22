** Overall Strategy

The overall execution strategy for this project is:

1] Re-implement the behaviour of the current bash-based implementation first.

2] Test this for Vienna first.

3] Improve where needed based on the Vienna results.

4] Test this for Junglinster.

5] Improve where needed based on the Junglinster results.

6] Clean up the migration-specific code once both homes are operational in the Go-based version.

7] Freeze a fork of that version as the operational baseline.

8] Continue with the next items on the roadmap, such as availability handling and better warnings for low battery power.

** Current Todo

1] Reach behavioural parity with the current bash-based implementation for Vienna.

- Keep migration, interpretation, macro expansion, and later generation work focused on reproducing current behaviour first.
- Use Vienna as the first end-to-end validation target.
- Treat mismatches against the bash-based implementation as defects, not as redesign opportunities.

2] Tighten validation and tests around the Vienna path.

- Keep using interpretation and expansion output as verification tools while the DSL and generator settle.
- Add regression tests whenever Vienna behaviour is clarified or fixed.
- Keep the normalized DSL stable enough that the Go parser, expander, and generator all work from the same conventions.

3] Repeat the same parity-and-fix cycle for Junglinster.

- Run the same migration and validation flow for Junglinster once Vienna is operational.
- Capture house-specific deltas explicitly instead of letting them drift into implicit behaviour.

4] Remove migration-specific scaffolding only after both homes are operational.

- Clean up temporary migration code once it no longer carries operational value.
- Preserve the operational behaviour while simplifying the implementation.

5] Freeze the resulting operational version.

- Fork the first fully operational Go-based implementation as the stable baseline.
- Use that frozen version as the reference point for subsequent redesign work.

6] Continue with the next roadmap items.

- Availability handling.
- Better warnings for low battery power.
- Further cleanup and improvements once the operational baseline is secured.

** Implementation Backlog

1] Cross-check the "=== ENTITIES BY SPACE (FULL NAMES) ===" output against source intent.

- Verify representative spaces in Vienna and Junglinster (root, top-level spaces, and a few deep nested spaces).
- Confirm raw entities are reported in bracket form (`type.[raw_name]`) and not internal normalized form.
- Confirm contextual path expansion for relative entity specs is correct.
- Confirm `no_collect` markers are present where expected.
- Confirm external-entity classification is only a "needs-check" list, not yet a hard generation decision.

2] Harden macro call checking to fully match DSL intent.

- Keep required/optional checks for declared parameters.
- Add explicit unknown-parameter detection for `with:` blocks.
- Complete runtime checks for all declared parameter kinds (`boolean`, `path`, `option`, `set<...>`, etc.), not only `int` and `entityReference`.
- Ensure `option` style flags are interpreted consistently (presence=true, absence=false unless defaulted).

3] Define and implement the generation data model.

- Introduce one canonical in-memory model after parse+expand containing:
   - entity identity (type/sphere/path/raw),
   - space context (full path, nesting level),
   - definition/import status,
   - options (`providing`, `icon`, `open_stop_close`, `no_collect`, etc.),
   - dependencies/references used by generated templates.
- Record provenance (source file + line + originating macro) for diagnostics.
- Use this model as the sole input for YAML generation and list generation.

4] Implement generation policy split for external entities.

- External without local options: do not generate core entity YAML.
- External with local options: generate customization/configuration YAML only.
- Defined/imported entities: generate full entity YAML as today.

5] Add optional online availability checks.

- Keep current offline mode as default.
- Add a mode that validates external entities against Home Assistant and reports `available/missing/unreachable`.

6] Keep the normalized DSL internally consistent.

- Prefer one structural form for blocks and conditionals across migration output, parser rules, and macro expansion.
- Keep top-level settings as global assignments in Settings.def.
- Keep defaults, runtime administrative state, and taxonomy separated in the Go implementation.

** Context
A YAML generator, written in Go, to generate:
- YAML-based templates for switches, lights, sensors, etc. that are complementary to the existing entities in Home Assistant installations.
- Lists of entities that can be included, via copy/paste, into entity cards in the Home Assistant UI.

The existing implementation uses bash scripting.
These scripts exist for two homes, Vienna and Junglinster, in the Old folder.
The driving script is Configure.

During generation, the system also checks prerequisites, in the sense that the definition files assume the existence of entities in the local and/or remote Home Assistant instance.
The generated tree of YAML files is rsynced by an Update script to the relevant instance.

The two homes are provided as examples. End-to-end migration and deployment testing can initially focus on Vienna, as this is the less critical location.

** Steps
1] Explore and freeze the legacy source model.

The output of step 1 should be concrete:
- an inventory of the legacy source inputs per home
- a normalized target layout under New/<<house>>
- clarification of the open design questions that affect the migration script and parser
- a clear distinction between legacy source files and legacy implementation details

2] Work on a migration script/programme to convert the existing definition files while the Go version of the YAML generator is being developed.

Based on the existing definition files, the normalized target per home should be:
- Definitions/Settings.def
- Definitions/Server.def
- Definitions/Bridges.def
- Definitions/Entities.def
- Definitions/Lists.def
- Definitions/Macros.def

Creation macros are normalized into one file during migration, but must still be read before the regular definitions because they act as DSL extensions.

See the earlier mailfilter project regarding the Pascal-ish formatting style.

3] Develop a parser for these def files in the CDL1-like style.

Current parser milestone:
- parse the normalized files in New/<<house>>/Definitions/*.def into a structural tree (block / statement / comment)
- support the current normalized syntax: colon-style block headers, `end;` block termination, semicolon-terminated statements, and `if` / `elif` / `else` blocks
- write an interpretation file per house to New/<<house>>/interpretation.txt, similar in spirit to the mailfilters interpretation output

This interpretation output is intended for verification while grammar and migration are still evolving.

3b] Macro expansion and semantic validation (20.03.2026).

The "expand" command analyzes creation macro definitions and their usage in Entities.def:
- Parses all creation macro definitions from Macros.def with full parameter type information
- Identifies all macro invocations in Entities.def with line numbers and nested space context
- Validates macro invocation parameters against defined parameter types (int, string, entityReference, etc.)
- Generates a comprehensive report: Expansion.txt in each house's Definitions folder
- Reports summary statistics: total invocations, valid vs. invalid, type/validation errors

Usage: go run . expand [Vienna|Junglinster]

This tool enables verification of:
- Alignment between Macros.def definitions and actual usage patterns
- Parameter type correctness (e.g., integer values passed to int parameters)
- Coverage of creation macros (which are defined but may not be used, or vice versa)
- Inconsistencies between Vienna and Junglinster implementations

Output example (Expansion.txt):
- CREATION MACRO DEFINITIONS section: lists all macros with parameter signatures and type information
- MACRO INVOCATIONS IN ENTITIES.DEF section: each invocation with validation status and parameter bindings
- SUMMARY section: statistics on valid vs. error invocations

4] Develop the dependency checks.

5] Generate YAML.

Note: it makes sense to have parse / check / generate Go files, with smaller generate_<<xx>> files per entity type, such as sensor, light, switch, etc.

Operational commands used during development:
- go run . migrate [Vienna|Junglinster]
- go run . interpret [Vienna|Junglinster]
- go run . refresh [Vienna|Junglinster]  (migration + interpretation)

Equivalent make targets:
- make migrate
- make migrate-vienna
- make migrate-junglinster
- make interpret
- make interpret-vienna
- make interpret-junglinster
- make refresh
- make test-migration

** Explanations

The current bash-based scripts use:
- /tmp/... as a database-like working area

Overall, there are three namespaces for entities:
- physical = entities in the house that do not have an immediate social role
- social = entities that have a clear direct role from a social / usage perspective
- infrastructural = entities that pertain to the IoT / smart-home infrastructure itself

Virtual spaces are semantically distinct from regular spaces.
- They still define a space context and can contain members and space-level actions.
- During generation they follow the legacy `begin_virtual_space` / `end_virtual_space` behaviour, which uses the virtual-local aggregate policy rather than the full regular-space aggregate set.

The current execution order of the bash version is:
- read modules
- read macros
- clean generated things
- read settings
- read definitions
- check entity usage
- generate configurations
- generate lists
- optionally validate assumed entities against Home Assistant

Legacy source inputs per home are:
- Definitions folder:
   - 00_server.def
   - 01_bridges.def
   - 02_entities.def
   - 03_lists.def
- Settings folder:
   - local.def
   - general.def
   - secrets.def
- Macros folder:
   - a set of macros / templates that extend the DSL

Legacy implementation details, which are useful for understanding behaviour but are not themselves the migration target, are:
- Modules/*
- the /tmp/... working area
- the generated hass tree in Old/<<house>>/hass

The hass folder contains the generated files, which are uploaded to the running Home Assistant instance via the Update script. There is no need to change the Update script yet.

In the new Go-based version, the structure should be:
- New/<<house>>/Definitions ==> normalized def files, including Macros.def
- New/<<house>>/hass ==> generated YAML tree

** Step 1 Clarifications

From the existing bash implementation, step 1 should explicitly answer the following:
- how the conditional logic in 00_server.def should be represented in the normalized Pascal-ish format, especially the distinction between at-home and away
- whether secrets should remain external to the normalized definitions, or whether the migration script should produce placeholders / references instead of copying secret values
- whether macro files should keep the .def extension or move to a dedicated extension such as .mac
- whether settings remain as simple assignments in Settings.def, or whether they should also be converted into a block-based DSL form
- which parts of the shell implementation are semantic source rules that must be preserved, and which parts are merely one particular bash implementation strategy

The answers currently fixed for step 1 are:
- secrets and environment-specific values stay outside the normalized definition files
- the local-versus-remote server logic must become part of the DSL
- macros are collapsed into one normalized Macros.def file
- icon-style global constants used by macros should move into Settings.def instead of remaining implicit in legacy Modules/Settings.1.hass

** Creation Macro definitions
The creation macros should be read before the regular definitions.
They allow the DSL to be extended with more specific notions.

Creation macro expansion must preserve semantic flags and attributes from the target DSL.
In particular, no_collect is a specific point of attention for creation macro definitions: if a creation macro introduces or wraps entities that should stay out of space-based aggregate collections, that exclusion must remain explicit and correct in the expanded result.

Creation macro bodies also need access to the entity specification passed by the create target itself.
The create target passed to a creation macro must be resolved to an extensional entity specification first.

Implied creation macro variables from the resolved target:
- $raw: boolean, true when the target uses raw Home Assistant naming
- $type: the part before the '.' in the entity specification
- $name: the raw Home Assistant name (only meaningful when $raw is true)
- When $raw is false:
   - $sphere: the first segment after '.' up to '/'
   - $entity: the remaining path after $sphere
   - $sub_type: optional semantic subtype derived from the path tail

Creation macro headers may constrain accepted target forms.
Example intent:
- creation macro power_switch [no raw] (...):
This means the creation macro may only be called with non-raw (structured) targets.

Typed creation macro parameters such as `($node entityReference, $threshold int)` are a design goal, but this is not implemented yet in the current parser/runtime.

Examples from the legacy version are:
- entity_battery_level_device
- entity_media_player
- entity_light_device
- entity_power_switch
- entity_thermostat

These are effectively higher-level DSL constructs that expand into one or more entity declarations plus definitions.
In the normalized source they are collected into a single Macros.def file, but semantically they still form a separate prelude that must be parsed before the regular definition files.

** create statements and macro expansion semantics

In the normalized DSL, create <macro-name> <args...> is the surface syntax for invoking a macro.
It represents an intentional, parameterized definition of one or more entity declarations.

Create invocations support two equivalent styles:
- Positional shorthand:
   create battery_alert front_door/ring 20;
- Option block form:
   create battery_alert front_door/ring with:
      alert_threshold 20;
   end;

In the option block form, each option becomes a named macro variable for that invocation.
For example, alert_threshold 20 binds $alert_threshold to 20 in the macro body.

Option blocks can also pass flag-style global options without a value.
Example:
- create cover_device terrace/awnings with:
      open_stop_close;
      no_collect;
   end;

In this case, $open_stop_close and $no_collect are available in the macro body as boolean variables.
A present flag means true; an absent flag means false (unless a macro default overrides it).

Option-passing rules:
- Options are invocation-local (no leakage across create statements).
- Missing options use macro defaults if defined by the macro.
- Flag options (without explicit value) are boolean variables in macro scope.
- Unknown option names should be treated as expansion errors.
- The option block form is preferred when readability is better than positional arguments.


Macro definitions and parameter declarations:

Creation Macro syntax:

```
creation macro <name> [constraints...] { <parameter-list> }:
   <body>
end;
```

General form:
```
creation macro <name> [no_raw] [space_level] { $p_1 t_1 [op], ..., $p_n t_n [op] }:
```

Where:
- `<name>` — macro identifier
- `[no_raw]` — optional constraint: disallow raw entity specifications `[...]` in the target
- `[space_level]` — optional constraint: bind the macro target from the immediate surrounding space specification instead of an explicit entity target
- `{ $p_i t_i, ... }` — parameter set (order-independent when using option block form)
- `$p_i` — parameter name (used in macro body with $ prefix)
- `t_i` — parameter type (see Supported types below)
- `[op]` — optional marker on a parameter; omitted argument binds as empty (or macro-defined default when present)

Braces `{ }` signal "this is a parameter set" similar to Rust/Swift function parameters.
The order of parameters is only significant in positional invocation form.
When using the option block form (`with: ... end;`), parameter order is irrelevant.

Supported parameter types:
- (default, omitted) — entity specification (`type.sphere:path` form); validated for proper syntax; this is the implied type and need not be written
- `entityReference` — an extensionally identified entity specification; a concrete reference to an existing or derived entity
- `string` — any text value
- `int` — numeric value; validated as parseable to int64
- `boolean` — true/false flag; missing flag = false, present flag = true
- `set<entityReference>` — comma-separated list of entity references, each validated as a well-formed entity specification
- `set<string>` — comma-separated list of string values, each validated independently
- `set<int>` — comma-separated list of integer values
- `path` — Home Assistant entity or node path; basic format checking

Implicit target binding:
- For regular creation macros, the entity target from the invocation is implicitly available as `$entity`.
- For `space_level` creation macros, the implicit target is the immediate surrounding space specification.
- In both cases, the implicit target is resolved to extensional form before macro body execution.
- Creation macros do not declare `$entity` explicitly; it is automatically bound.

`space_level` binding details:
- The surrounding space contributes implicit target components (type/sphere/path/sub-type where relevant).
- This allows a creation macro to operate on "the current space" without passing a separate entity target.
- A `space_level` creation macro is invalid outside a space context and should raise an expansion error.

Optional parameter (`op`) rules:
- If a parameter is marked `op`, it may be omitted in positional form.
- In option-block form, an `op` parameter may be absent.
- When omitted, the value binds as empty unless the macro defines a default.
- A non-optional parameter (no `op`) is mandatory.
- Type validation is applied only when a value is present or when a default is injected.

Example macro definitions:

```
macro battery_alert { $alert_level integer }:
   # $entity is implicit from the create target
   declare entity binary_sensor.infrastructural:$entity:battery_alert;
   if "$alert_level" != "" then
      define satisfies sensor.infrastructural:$entity:battery_level "($ | int(0)) < $alert_level";
   end;
end;

macro power_switch { $node string, $threshold integer }:
   # Implicit $entity from create target
   declare entity sensor.social:$entity:power;
   declare entity binary_sensor.social:$entity:consumes with
      define satisfies sensor.social:$entity:power "($ | int(0)) > $threshold";
   end;
   declare entity switch.social:$entity;
   providing node $entity/$node;
end;

macro zigbee_group { $group set<string> }:
   # Implicit $entity (the light target)
   # $group contains { 1, 2, 3, 4, 5, 6 } etc.
   for member in $group do
      declare entity light.physical:$member;
   end;
end;

macro light_device { $sphere string, $name string }:
   declare entity light.$sphere:$name;
   providing node $name/light;
end;
```

Macro invocations:

Positional form (parameters in declared order):
```
create battery_alert front_door/ring 20;
create power_switch /home/garden/lamp robb 1;
```

Option block form (parameters by name, order-independent):
```
create battery_alert front_door/ring with
   alert_level 20;
end;

create power_switch /home/garden/lamp with
   node robb;
   threshold 1;
end;
```

Type validation during macro expansion:
- Argument values are checked against the declared parameter type.
- Type mismatch errors are reported with the invocation location and expected/actual types.
- For `set<T>` types, each element is validated (e.g., `set<integer>` validates each comma-separated item).
- Unknown macro parameters are treated as expansion errors.
- Missing non-optional parameters are treated as expansion errors.
Some macro definitions contain intentional aggregate references, such as "all light entities in a given space".
If macro expansion were applied repeatedly, this could lead to a fixed-point computation because each expansion round might add new entities that satisfy the aggregate reference in the next round.
To avoid this complexity — and to keep the semantics predictable — macro expansion is performed exactly once, after the base entity definitions have been fully read.

The intended processing order for the generator is therefore:
1. Read the macro definitions (Macros.def). These define what create <name> means.
2. Read the space and entity definitions (Entities.def). This establishes the full set of declared entities, their nesting in spaces, and their attributes (no_collect, providing node, open_stop_close, etc.).
3. Run structural and semantic checks on parsed definitions (including type/option checks) and collect warnings and errors.
4. Execute declare statements and apply one single round of macro expansions in space context. No further rounds follow.
5. Run post-expansion checks and collect warnings (for example unresolved references, unused declarations, and suspicious option usage).
6. Use the resulting fully-expanded entity model as the basis for YAML generation and list generation.

This single-pass expansion avoids fixed-point iteration while still allowing macros to reference the entity context at the time of expansion (e.g. to compute aggregated light groups for a space).

Entity specification model for macro create targets:

Extensional forms:
- <<type>>.<<sphere>>/<<path>>
   - Examples: sensor.social/apartment/kitchen/door, switch.social/apartment/bathroom/washingmachine
   - The tail segment may carry semantic roles (for example door/window) that can be used for space-level aggregation.
- <<type>>.[<<raw-name>>]
   - Example: sensor.[ems_esp_boiler_boiler_outside_temperature]
   - This is a verbatim Home Assistant entity id under a typed wrapper.

Intensional forms:
- <<type>>.<<sphere>>:<<path>>
- <<type>>.<<sphere>>:/<<path>>
- <<type>>.<<sphere>>:<<path>>:<<sub-type>>
- <<type>>.<<sphere>>:/<<path>>:<<sub-type>>

Resolution inside current space x:
- <<type>>.<<sphere>>:<<path>> -> <<type>>.<<sphere>>/x/<<path>>
- <<type>>.<<sphere>>:/<<path>> -> <<type>>.<<sphere>>/<<path>>
- <<type>>.<<sphere>>:<<path>>:<<sub-type>> -> <<type>>.<<sphere>>/x/<<path>>/<<sub-type>>
- <<type>>.<<sphere>>:/<<path>>:<<sub-type>> -> <<type>>.<<sphere>>/<<path>>/<<sub-type>>

If the sphere component is omitted in an intensional specification, the default sphere is social before extensionalization.

Illustrative example (relative intensional form):
- Input in space path x: sensor.physical:frient:illuminance
- First ':' introduces the context-relative path part (frient), so x is inserted before it.
- Second ':' introduces the semantic sub-type (illuminance).
- Result: sensor.physical/x/frient/illuminance

This resolution must happen before macro body execution, so macros always receive an extensional target representation.

Partial entity specifications in macro call arguments:

Macro call arguments may omit any or all of type, sphere, and sub_type.
A bare name is the most common partial form.
Resolution rules for partial specs:

1. If the argument is wrapped in []: raw form, $raw = true, $name = the bracketed string, all other implied variables = empty.
2. If the argument contains '.': the part before '.' is $type; the rest is parsed as sphere and path as in the intensional form.
3. If the argument contains no '.': $type = empty.
4. If there is no sphere component: $sphere = social.
5. If there is no sub_type component: $sub_type = empty.
6. For the path/name component:
   - If it starts with '/': absolute path relative to the house root, used as-is.
   - Otherwise: relative name, completed by prepending the current space path.

Example: create battery_alert netatmo_outdoor inside space social:terrace resolves as:
- $raw = false
- $type = empty (no '.' in the argument)
- $sphere = empty
- $sub_type = empty
- $entity = terrace/netatmo_outdoor  (current space path 'terrace' prepended to the bare name)

A macro that does not use $type or $sphere from its call argument is independent of how the caller specified the target.
The macro entity_battery_alert is a clear example: it supplies binary_sensor.infrastructural and sensor.infrastructural as fixed types and spheres in its body, and uses $entity only as the path component.
This means it works correctly whether called with a bare name, a partial path, or a full entity spec — as long as $entity carries the right resolved path.

** Entity definitions
Using the Junglinster example:

"declare entity sensor.social:power_total"
"declare entity sensor.physical:netatmo:humidity"

These are declarations of entities that are assumed to exist already.
The first colon is used to signify contextualisation within the house and its spaces.
In the actual name, it is replaced by a nested path such as house/living_room, derived from begin space nesting.
The last colon is used to identify the semantic unit of the sensor, such as temperature, power, or humidity, separate from the unit of measurement such as percent or Celsius.

Verbatim naming convention:
- If an entity name must be treated literally (exactly as it already exists in Home Assistant), it is written between [ and ].
- A bracketed entity name is not rewritten by context expansion rules.

Example:
"declare entity [sensor.ems_esp_boiler_boiler_outside_temperature];"

Set syntax for macro arguments:
- Comma-separated lists used as macro arguments are wrapped in curly braces { }.
- Each item in the set is separated by commas and optional whitespace.
- Braces provide explicit boundaries for the list.

Example:
"zigbee_group light.social:main with group { kitchen, middle, living };"
"zigbee_group light.social:left with group { 1, 2, 3, 4, 5, 6 };"

This notation makes parsing unambiguous and future-proof for enhancements like range syntax { 1..6 }.

Alias assignment convention on declarations:
- A declare entity statement may bind an alias using as <alias>.
- Alias scope is lexical by space tree: the alias is visible in the declaring space and all its nested sub-spaces.
- The alias is not visible outside that space subtree (for example, not in sibling spaces or parent-only contexts).
- This allows later statements to refer back to the declared entity without repeating a long name.

Example:
"declare entity [sensor.ems_esp_boiler_boiler_outside_temperature] as boilerOutsideTemp;"
"declare entity sensor.physical:garage_door:temperature with:
    definition as value boilerOutsideTemp;
 end;"

"declare entity binary_sensor.infrastructural:netatmo:node
   definition as condition sensor.infrastructural:netatmo:reachability \"$ == 'True'\""

The condition following declare entity binary_sensor.infrastructural:netatmo:node clarifies that this entity is not only assumed to be present, but can also be derived. In this case it is a binary sensor based on a Home Assistant-compatible condition.

Condition syntax variations:

Single-parameter conditions use `$` or `$1` to refer to the entity value:
  definition as condition sensor.infrastructural:netatmo:reachability "$ == 'True'"

Multi-parameter conditions allow multiple entity references and use positional parameters `$1`, `$2`, etc.:
  definition as condition $sunnyThreshold $sensor "($1 | int) < ($2 | int)"

In this form:
- `$sunnyThreshold` is the first entity reference (available as `$1` in the expression)
- `$sensor` is the second entity reference (available as `$2` in the expression)
- The expression evaluates using Home Assistant template syntax with explicit parameter indexing

This allows binary sensors to be derived from the relationship between two entities, for example comparing an illuminance threshold against a sensor value.

For direct value derivations, use `definition as value`, e.g.:
"definition as value sensor.infrastructural:laserjet/status!state_message"

"declare entity light.social:carport with:
   providing node /house/server_room/zwave/027"

The providing node statement is an explicit DSL notion.
The path argument `<XXX>` after `node` is used verbatim to form the entity name:
  `binary_sensor.infrastructural:<XXX>:node`
This entity is true when the node is up and running, and false when it is not.

Examples:
- `providing node /house/server_room/zwave/027` → `binary_sensor.infrastructural:/house/server_room/zwave/027:node`
- `providing node laserjet` → `binary_sensor.infrastructural:laserjet:node`

Because some Home Assistant integrations do not expose node-up/node-down state directly, the generator must be able to materialize these node entities as virtual/template-based binary sensors.

"create battery_level_device dining/philips_dimmer 20"

This is a macro call. The keyword battery_level_device triggers the macro definition from the Macros folder.

Macro application phase:
- Macros are applied during parsing, not as a later rendering/post-processing step.
- The parser must resolve and bind macro invocations while building the interpretation/execution structure.

Dynamic macro-name interpolation:
- A macro name may include an interpolation segment in angle brackets, for example:
   `create <$type>_device physical:$e;`
- In this form, `$type` is the currently bound type value from the provided entity specification/context.
- The value is inserted into the macro name token itself (before lookup), yielding `<resolved_type>_device`.
- This is not the same as rewriting `_device` to a separate `device <x>` token sequence.

Built-in DSL constructs:
- `lights_motion_guarded` is a built-in DSL statement, not a macro definition.
- It should be parsed directly by the DSL and is intended to compile to rules/automations rather than ordinary entity-declaration expansion.

"declare entity cover.social:front with:
   open_stop_close"

The open_stop_close statement is an explicit DSL notion for blinds and related cover-like entities.
It denotes the three key operations that such an entity supports: an open command, a stop command, and a close command.

"declare entity light.social:main no_collect"

The no_collect flag is an explicit DSL attribute.
It means that the entity must be excluded from the space-based collection of entities of the same kind.
For example, a light marked no_collect inside a space X must not be included in the aggregated light entity for that space.

Entity availability and conditional properties:

The "available" property specifies the availability conditions for an entity. Currently supported forms:

- `available always;` — The entity is always available (unconditional)
- `available is defined by <entity>;` — The entity is available when a referenced entity is defined (future: planned for implementation)
- `available <condition>;` — The entity is available when a condition evaluates to true (future: planned for implementation)

The "available" property is often set within entity blocks in creation macros. For example:
```
entity binary_sensor.infrastructural:$entity:battery_alert with:
  begin
    definition as condition sensor.infrastructural:$entity:battery_level "...";
    available always;
  end;
```

This declares that the binary_sensor is always available (not dependent on external factors). The "available" setting is a declarative property that Home Assistant generators will use to determine:
- Whether to always materialize the entity
- Whether to materialize conditionally based on other entities
- Default visibility and refresh behavior

The entity grammar families that are already visible from the legacy files are:
- begin space / end space
- declare entity ...
- declare entity ... as <alias>
- declare entity [<verbatim-home-assistant-entity-id>] ...
- definition as value ...
- definition as condition ...
- definition as <kind> ... (for specialised declarations such as cli_sensor, switched_device, has_state, adjustment, flipped, etc.)
- providing node ...
- macro invocations such as create battery_level_device ...
- built-in DSL statements such as lights_motion_guarded with delay ...
- list ... with clean_prefix / clean_postfix filters

