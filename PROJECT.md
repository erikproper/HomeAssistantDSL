** Current Todo Items (as of 2026-04-27)

Active work — roughly in priority order:

[1] Entity presence checker — integrated into "homeassistant generate"
    After generation, validate completeness:
    - [1a] Referential integrity (always, offline): scan all generated YAML for
      entity_id strings; report any not in the declared set (DSL-declared entities
      + generator-implied entities from output file names).
    - [1b] Online availability (when HA is reachable): verify assumed entities
      (those without a definition or import in the DSL) against the live HA instance.
    Both checks print inline warnings; generation still completes.
    Status: DONE — presence.go; no external files needed

[2] Harden macro parameter checking.
    Complete runtime checks for all declared parameter kinds.
    Add explicit unknown-parameter detection for with: blocks.
    Status: DONE — validateParameterType covers all kinds including ParamEntity (default)
      and ParamSetOfInt (new); unknown-parameter and missing-required-parameter detection
      already present in ValidateInvocationParameters, called from ParseEntitiesAndFillAdministration.
      Tests added for all three cases.

[3] Online availability checks (optional mode).
    Keep current offline mode as default.
    Add mode to validate external entities against Home Assistant.
    Status: DONE — integrated into generate (presence.go):
      [3a] main instance: assumed entities verified against live HA (when reachable)
      [3b] bridge instances: REST bridge source entities verified per bridge (when reachable)
      Offline mode remains the default; online checks are automatic and silent when unreachable.

[4] Tighten entity model cross-checks.
    Verify representative spaces in Vienna and Junglinster.
    Status: DONE — RunPostParseChecks (checks.go) extended with relation cross-checks:
      follows (follower + leader), switched_device (device + main), timer limits (timer + bound).
      Both Vienna and Junglinster verified: Vienna clean, Junglinster shows only the
      pre-existing heating-related gaps documented in Current State below.

[5] Update README.md file
    Status: DONE — README.md rewritten with: usage, repository layout, entity specification
      model (extensional/intensional, spheres), DSL syntax (spaces, entities, macros),
      parameter type table, implied parameters, and post-generation check summary.

** Current State

YAML generation is operational for both Vienna and Junglinster.
Invocation: homeassistant Definitions/Main.def from New/<house>/,
outputting to hass/ with list.* alongside it.
The legacy multi-command CLI (generate/interpret/expand/check) has been removed;
the sole supported invocation is the .def path form above.

List pattern syntax: domain.sphere/*/suffix filters by sphere; domain.*/suffix matches any sphere.
Sphere-filtered patterns also include derived leaf-level social groups (e.g. social_apartment_bedroom_door
wrapping a physical sensor), while excluding parent-level aggregates (e.g. social_apartment_door).

Diff count (Old vs New Vienna, April 2026): 28 Only-in-Old, 67 Only-in-New.
Vienna remaining differences are all intentional:
- Superseded hallway door physical-first approach (2 files)
- Intentional entity renaming (e.g. binary_sensor.social_windy → binary_sensor.social_terrace_windy)
- Old generator bug (empty rack switch)

Diff count (Old vs New Junglinster, April 2026): 539 Only-in-Old, 398 Only-in-New.
Junglinster remaining differences:
- Naming: New uses _radiator_ infix for climate entities (reflects actual entity paths)
- Missing occupation booleans in Entities.def (heating preset automations not generated)
- Missing covers/auto_control in Entities.def (cover automations not generated)
- Various entity name differences from deliberate DSL improvements


** Context

A YAML generator, written in Go, that produces:
- YAML-based templates for switches, lights, sensors, binary sensors, scripts,
  automations, and entity configuration files for Home Assistant.
- entity-card list files (list.*) for the Home Assistant Lovelace UI.

The two homes supported are Vienna and Junglinster (Luxembourg),
under New/Vienna and New/Junglinster respectively.
The generated hass/ tree is rsynced to the relevant Home Assistant instance
by the Update script.

Intended processing order:
1. Read shared macro definitions (Shared/Definitions/Macros.def).
2. Read per-house settings and server/bridge config.
3. Read space and entity definitions (Entities.def).
4. Read list declarations (Lists.def).
5. Run structural checks; report warnings.
6. Execute one round of macro expansions.
7. Run post-expansion checks.
8. Generate YAML output (hass/).
9. Generate list files (list.*).
10. Check referenced entities for completeness.


** Definition files layout

Per-house: New/<house>/Definitions/
  Main.def      — include-order entrypoint (drives all other includes)
  Settings.def  — per-house variable overrides
  Server.def    — server/bridge conditional definitions
  Bridges.def   — integration bridge definitions
  Entities.def  — space and entity declarations
  Lists.def     — list declarations

Shared: New/Shared/Definitions/
  Macros.def    — all macro definitions (shared across houses)
  Settings.def  — global defaults


** Generated output layout (target)

New/<house>/hass/    — generated YAML tree, rsynced to the Home Assistant instance
New/<house>/list.*   — Lovelace entity-card lists


** Macro definitions

Macros extend the DSL with higher-level constructs that expand into one or
more entity declarations and definitions.

Macro syntax:
  macro <name> [no_raw] [space_level] { $p_1 t_1 [op], ..., $p_n t_n [op] }:
     <body>
  end;

Supported parameter types:
  (default)        entity specification (type.sphere:path form)
  entityReference  concrete reference to an existing or derived entity
  string           any text value
  int              numeric value
  boolean          true/false flag
  set<entityReference>  comma-separated entity reference list
  set<string>      comma-separated string list
  set<int>         comma-separated integer list
  path             Home Assistant entity or node path


** Entity specification model

Extensional: type.sphere/path  or  type.[raw-name]
Intensional: type.sphere:path  (resolved against current space context)
  type.sphere:path        → type.sphere/x/path
  type.sphere:/path       → type.sphere/path
  type.sphere:path:sub    → type.sphere/x/path/sub

Entity namespaces:
  physical       entities without an immediate social role
  social         entities with a direct social/usage role
  infrastructural entities pertaining to the IoT infrastructure
