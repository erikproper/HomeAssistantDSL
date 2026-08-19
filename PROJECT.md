** New Architecture — Migration Plan (as of 2026-08-18)

See Architecture.md for the target design (canonical event bus, coordinator, three
modelling layers). This section is the project-specific roadmap and current
hardware-level status — not general architecture, which is why it lives here rather
than in Architecture.md.

Phase 1 — Infrastructure: DONE
  protocols-server-1 exists; Podman/Quadlets working; Z-Wave JS UI running and validated.

  Z-Wave migration specifics (in progress):
  - New environment: Home Assistant Green; a Raspberry Pi running Podman; Z-Wave JS UI;
    Aeotec ZWA-2 controller; two Aeotec ZW117 range extenders.
  - Mesh validation: a Fibaro module was excluded/included/excluded/included successfully
    while physically remaining on the second floor, through Basement (ZWA-2) -> 1st floor
    repeater -> 2nd floor repeater -> device -- confirms the temporary mesh is already
    functional enough to migrate onto.
  - Node ID behaviour: observed monotonically increasing allocation (first inclusion ->
    Node ID 4, second -> Node ID 5) rather than immediate reuse of released IDs. Not fully
    validated; migration plan deliberately avoids depending on Node ID preservation.
    Possible allocation strategies to watch for: first unused ID, highest known ID + 1,
    highest ever assigned ID + 1.
  - Migration order: keep the old Gen5 mesh operational -> build an initial new backbone
    from the ZW117 repeaters -> migrate remote mains-powered devices first -> let migrated
    Fibaro modules become new repeaters, strengthening the mesh as migration proceeds ->
    migrate controller-near devices last. Keeps RF coverage acceptable throughout and
    doesn't require every device to be physically reachable from the new controller
    during migration.

Phase 2 — Stabilise the existing DSL/generator
  Test Vienna, test Junglinster, stabilise the compiler against both houses' live
  configuration. The initial bulk pass (2026-08-09 -- 08-18) was a genuine prerequisite
  for starting Phase 3 -- designing the new architecture against infrastructure that
  was still mid-change would have meant designing against a moving target. Beyond that
  bulk pass, Phase 2 continues in parallel with Phase 3 rather than as a separate
  blocking gate: each integration tackled in Phase 3 (ping/cpu, Netatmo, EMS,
  Zigbee2MQTT, Z-Wave) surfaces and stabilises its own corner of the existing
  DSL/generator as it's iterated on, so the two phases run together per-integration
  from here on.

Phase 3 — New architecture, incremental learning steps
  Each step tests one new architectural capability before the next is attempted, and
  grows the coordinator's aggregation vocabulary gradually rather than all at once.

  Timeline note (2026-08-18): travelling over the next few weeks, so the bridge role
  (federation across MQTT brokers, §5/§6.5 in Architecture.md) will not be touched
  until September. Physical-level DSL preparation therefore focuses first and only on
  step 0 (ping + cpu) below, since that step needs no cross-broker bridging at all.

  0. Ping + CPU integration (current focus). Treated as one conceptual integration
     with two data-collection scripts (a pinger; an OS-dependent CPU load/temperature
     collector). Every cpu device implies a corresponding ping device. First real
     exercise of the "integration <name> [on <host>] with devices: ..." DSL syntax
     (see Architecture.md §9.2 and $HOUSE/Definitions/Integrations.def), implied
     node/entity generation (binary_sensor.$host/reachable from a state topic plus an
     availability rule), and a first cut at aggregated/complementing nodes -- e.g. a
     junglinster node built by complementing cpu.junglinster availability with
     ping.junglinster reachability, then exposed conceptually as
     infrastructural:rack/junglinster.

     Deployment scripts (already exist, outside this repo):
     - /Users/erikproper/SmartLiving/Integrations/cpu -- deployed on each individual
       "metal level" Linux/Mac machine; reports load + temperature as JSON on the
       MQTT bus for that host.
     - /Users/erikproper/SmartLiving/Integrations/ping -- deployed on one server only;
       pings a configured list of hosts to determine "up"/"down".
     Config/secrets files the generator needs to know about or produce:
     - /Users/erikproper/SmartLiving/Integrations/cpu/secrets
     - /Users/erikproper/SmartLiving/Integrations/ping/secrets
     - /Users/erikproper/SmartLiving/Integrations/ping/hosts

     The generator's output for this integration splits into two kinds:
     1] YAML for the identified HASS instance(s) that consume the MQTT-reported
        load/temperature as normal sensors, e.g.
        sensor.infrastructural_house_laundry_kitchen_rack_junglinster_load and
        sensor.infrastructural_house_laundry_kitchen_rack_junglinster_temperature.
     2] Input for the coordinator, so it can build the correct MQTT discovery
        messages (and know the secrets it needs) for these devices/entities.

    Also ... enable the inclusion of additional meta-data
    for the (host) devices, and (maybe) enable the logical aggregation of some of the devices. Need to check if this is already needed. Key thing is to be able to add meta-data at the host level. Check, e.g. the netatmo
    devices. 
    Though ... this would mean a "complement" operator
    (of the netatmo with the host device elements) and not 
    just an aggregation.
  
  1. Netatmo and other cloud services, via the secondary HA instance (protocols-server-2,
     integration-adapter role). First real test case for MQTT bridging/federation
     end-to-end (see Architecture.md §6.5). Milestone check: Bridges.def should be
     effectively empty once this step is done -- the only bridge type currently in use
     is the "rest" bridge (confirmed: 0 uses in Junglinster's Entities.def, 29 in
     Vienna's, all of them Netatmo `imported rest` declarations), so nothing else
     currently depends on that mechanism.
  2. EMS heater device.
  3. Zigbee2MQTT. Exercises the legacy-to-conceptual passthrough capability described in
     Architecture.md §6.4, so entities can be switched over to the new architecture
     gradually rather than in one cutover.
  4. Z-Wave. Migrated last, since it needs the most native-API-aware handling and the
     richest device/capability modelling.

Phase 4 — Full platform migration & cleanup
  Once a platform has fully moved through Phase 3's incremental steps, retire the
  now-superseded generator logic for it from the existing app rather than carrying both
  paths indefinitely.

Phase 5 — Publish (not started)
  Architecture.md should, at some stage, be ready for others to read, alongside an
  up-to-date README.md, on GitHub. Explicitly deferred until the architecture above has
  actually been built and proven out, not before.


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
