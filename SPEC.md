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

Macros are normalized into one file during migration, but must still be read before the regular definitions because they act as DSL extensions.

See the earlier mailfilter project regarding the Pascal-ish formatting style.

3] Develop a parser for these def files in the CDL1-like style.

Current parser milestone:
- parse the normalized files in New/<<house>>/Definitions/*.def into a structural tree (block / statement / comment)
- support begin/end block structure and semicolon statements as produced by the migration tool
- write an interpretation file per house to New/<<house>>/interpretation.txt, similar in spirit to the mailfilters interpretation output

This interpretation output is intended for verification while grammar and migration are still evolving.

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

** Macro definitions
The macros should be read before the regular definitions.
They allow the DSL to be extended with more specific notions.

Macro expansion must preserve semantic flags and attributes from the target DSL.
In particular, no_collect is a specific point of attention for macro definitions: if a macro introduces or wraps entities that should stay out of space-based aggregate collections, that exclusion must remain explicit and correct in the expanded result.

Macro bodies also need access to the entity specification passed by the create target itself.
The create target passed to a macro must be resolved to an extensional entity specification first.

Implied macro variables from the resolved target:
- $raw: boolean, true when the target uses raw Home Assistant naming
- $type: the part before the '.' in the entity specification
- $name: the raw Home Assistant name (only meaningful when $raw is true)
- When $raw is false:
   - $sphere: the first segment after '.' up to '/'
   - $entity: the remaining path after $sphere
   - $sub_type: optional semantic subtype derived from the path tail

Macro headers may constrain accepted target forms.
Example intent:
- macro power_switch [no raw] (...):
This means the macro may only be called with non-raw (structured) targets.

Typed macro parameters such as `($node string, $threshold int)` are a design goal, but this is not implemented yet in the current parser/runtime.

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

Some macro definitions contain intentional aggregate references, such as "all light entities in a given space".
If macro expansion were applied repeatedly, this could lead to a fixed-point computation because each expansion round might add new entities that satisfy the aggregate reference in the next round.
To avoid this complexity — and to keep the semantics predictable — macro expansion is performed exactly once, after the base entity definitions have been fully read.

The intended processing order for the generator is therefore:
1. Read the macro definitions (Macros.def). These define what create <name> means.
2. Read the space and entity definitions (Entities.def). This establishes the full set of declared entities, their nesting in spaces, and their attributes (no_collect, providing node, open_stop_close, etc.).
3. Apply one single round of create expansions. Each create statement is expanded using the macro body and the current entity state at that point. No further rounds follow.
4. Use the resulting fully-expanded entity model as the basis for YAML generation, dependency checks, and list generation.

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

This resolution must happen before macro body execution, so macros always receive an extensional target representation.

Partial entity specifications in macro call arguments:

Macro call arguments may omit any or all of type, sphere, and sub_type.
A bare name is the most common partial form.
Resolution rules for partial specs:

1. If the argument is wrapped in []: raw form, $raw = true, $name = the bracketed string, all other implied variables = empty.
2. If the argument contains '.': the part before '.' is $type; the rest is parsed as sphere and path as in the intensional form.
3. If the argument contains no '.': $type = empty.
4. If there is no sphere component: $sphere = empty.
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

Alias assignment convention on declarations:
- A declare entity statement may bind an alias using as <alias>.
- Alias scope is lexical by space tree: the alias is visible in the declaring space and all its nested sub-spaces.
- The alias is not visible outside that space subtree (for example, not in sibling spaces or parent-only contexts).
- This allows later statements to refer back to the declared entity without repeating a long name.

Example:
"declare entity [sensor.ems_esp_boiler_boiler_outside_temperature] as boilerOutsideTemp;"
"declare entity sensor.physical:garage_door:temperature with:
    value boilerOutsideTemp;
 end;"

"declare entity binary_sensor.infrastructural:netatmo:node
   condition sensor.infrastructural:netatmo:reachability \"$ == 'True'\""

The condition following declare entity binary_sensor.infrastructural:netatmo:node clarifies that this entity is not only assumed to be present, but can also be derived. In this case it is a binary sensor based on a Home Assistant-compatible condition, where the $ placeholder is replaced with a reference to the current value.

For direct value derivations, use value, e.g.:
"value sensor.infrastructural:laserjet/status!state_message"

"declare entity light.social:carport with:
   providing node /house/server_room/zwave/027"

The providing node statement is an explicit DSL notion.
It means that the referenced computational node has a corresponding availability entity, conceptually at a path such as /house/server_room/zwave/027/node, represented in Home Assistant as an infrastructural binary_sensor.
This node entity is true when the node is up and running, and false when it is not.

Because some Home Assistant integrations do not expose node-up/node-down state directly, the generator must be able to materialize these node entities as virtual/template-based binary sensors.

"create battery_level_device dining/philips_dimmer 20"

This is a macro call. The keyword battery_level_device triggers the macro definition from the Macros folder.

"declare entity cover.social:front with:
   open_stop_close"

The open_stop_close statement is an explicit DSL notion for blinds and related cover-like entities.
It denotes the three key operations that such an entity supports: an open command, a stop command, and a close command.

"declare entity light.social:main no_collect"

The no_collect flag is an explicit DSL attribute.
It means that the entity must be excluded from the space-based collection of entities of the same kind.
For example, a light marked no_collect inside a space X must not be included in the aggregated light entity for that space.

The entity grammar families that are already visible from the legacy files are:
- begin space / end space
- declare entity ...
- declare entity ... as <alias>
- declare entity [<verbatim-home-assistant-entity-id>] ...
- value ...
- condition ...
- define ... (for specialised declarations such as cli_sensor, switched_device, has_state, etc.)
- providing node ...
- macro invocations such as create battery_level_device ...
- list ... with clean_prefix / clean_postfix filters

** Priority Update
Before continuing macro work, we first clean up entity definitions.

The first cleanup step is to revisit the begin/end representation in entity-oriented files, especially New/<<house>>/Definitions/Entities.def, so that block boundaries and statement-level intent remain clear and consistent.

This revisit should explicitly cover:
- where begin/end is required versus optional
- whether single-line declarations should remain compact or be lifted into explicit begin/end blocks
- consistency between begin space / end space and other begin/end block forms
- readability of nested spaces and nested entity blocks in large files

Target shape for entity-oriented blocks:
- declare entity <name> with:
- space <name> with:
- end;

** Work Items

Current active work items are:
- [ ] Clean up entity definitions before further macro changes.
- [ ] Revisit begin/end representation for Entities.def and define a consistent block-style guideline.
- [ ] Freeze macro target syntax.
- [ ] Define macro argument grammar.
- [ ] Specify macro expansion semantics.
- [ ] Implement entity target resolution (intensional -> extensional) before macro execution.
- [ ] Implement macro target constraints such as [no raw].
- [ ] Inventory icon and unit sources (Vienna and Junglinster), including values currently embedded in legacy module files such as Modules/Settings.1.hass.
- [ ] Design a normalized settings-import model for icon/unit/etc. metadata.
- [ ] Map Vienna and Junglinster deltas for settings and defaults.
- [ ] Add parser and migration changes that implement the agreed macro and settings model.
- [ ] Validate the result with interpretation output and regression tests.
