** TODO 

## Manually (Erik) extend coverage

1. check suggested in both locations
VIE
JLI

2. Ensure we have now all the integrations we need.
VIE
JLI

3. Check for more of the existing integrations, like fritzbox, ems-esp, etc.

4. EMS heater device. More imports?
 
## Migration & improvements

22d. Metadata topics per device, provided by the integration (Erik, 2026-10-27): the coordinator
    isn't aware of each (non-discovery) integration's peculiarities, so every integration gives
    it, per device, the list of topics that carry that device's metadata (manufacturer, model,
    sw_version, ...), which it collects into discovery_device_info/state for generate to weave
    into Physical.def (Architecture.md §6.16). Half there: the hosts integration's type definition
    (THostsEntityMaterialization.DeviceInfoTopic) already sets each device's device_info_topic
    in devices.yaml, but the coordinator ignores it and subscribes to hosts/+/device/state,
    hardcoded. Generalise to a "metadata topics" list per device, for every integration kind
    (hosts, home_assistant bridges, commandline, later fhem2mqtt and raw MQTT).

22f. Vienna's TV ping node (hosts/sony_tv/node/state) never gets a message: the ping reporter
    publishes nothing for sony_tv (seen 2026-10-27, not caused by the YAML switch).

22g. An export that stops is not noticed by the importer (found 2026-10-28, with item 22): the
    exporter clears its retained cloud config, but the importing coordinator skips an empty message
    ("retraction handling: not built yet", discoveryimport.go), so the entry stays in its
    import_catalogue and generate reports nothing. Needs: on a cleared config, drop the catalogue
    entry it fed (the coordinator has to remember topic -> local entity, since an empty message
    carries no export_id), so generate's "doesn't export" ERROR fires.

23. Can we trigger, within HA, the reload of an integration?
The netatmo integration on P-s-2@JLI  often "lost" some connection
leading to entities of especially the "Smart Indoor Air Quality Monitor" to report
as being unavailable.
The idea would be to enable the automatic triggering on P-s-2@JLI of a reload when 
selected "indicator" entities are unavailable.

Another case (2026-10-25, Junglinster main): the Sonos headphones
(media_player.social_house_sonos_headphones) drop out around each deploy's reload; twice they came
back after two minutes, the third time not -- while their ping node stayed on. A manual reload of
the Sonos integration brought them back.
Same on the next deploy (2026-10-25): all 13 Ring entities stayed unavailable after the reload;
reloading the Ring integration brought them back. 

24. (Enforced) default units for e.g. pressure as well as the precision?? This did work in the "bash" based script approach.
    (2026-10-27, Erik) Builds on 19: the defaults (Shared/Definitions/Defaults.def, a house's own
    "defaults:" in Physical.def) set unit/icon/device_class/state_class but only fill gaps, and have
    no precision. Add "precision:" (a sensor's suggested_display_precision -- display only; HA keeps
    full precision in state, history and statistics) and "forced" (override a value the gateway
    supplies, e.g. Zigbee2MQTT's pressure unit). Easy now that entities are YAML (Architecture.md §6.16): the
    generator writes every field itself.
    The old bash approach's settings (Erik): $HOME/go/src/homeassistant/Old/Junglinster/Modules/
    Settings.1.hass -- units per quantity (UnitsOf_pressure="mbar", UnitsOf_temperature="°C",
    illuminance "lux", wind speed "km/h", co2 "ppm", noise "dB", humidity/battery "%") and state
    classes (StateClassOf_...); no precision found there or in the other old modules.
    EP: Correct. Once HA started to allow the specification of this in the UI, I dropped that from the bash scripts. However, it is rather tedious to now do it all by hand ...
    EP: We should at least use "

25. depencency versus via ... also include e.g. the connexoon for the somfy's
- somfy => ha2mqtt => connexoon
  --> Does the relevant integration on ha2mqtt have availabilty as well? 
      We do ping connexoon.
- netatmo's => ha2mqtt => netatmo??
  --> Similar question. We do ping some of the netatmo units.
- xc40 => ha2mqtt => volvo??
- zwave's => zwave2mqtt
- zigbees's => zigbee2mqtt
Note: zwave2mqtt, zigbee2mqtt, and ha2mqtt (and its local integrations) depend on p-s-1 to be up.

25b. Test if the meta call to reset HA's works.

25c. (2026-09-24) Zigbee2MQTT liveness check: detect when a house's own Z2M bridge itself is down
     (not just an individual device), and if so, bulk-mark every Z2M-sourced entity in that house
     as unavailable, rather than leaving each one showing its own last-known (stale) state.
     Mostly covered (2026-10-28): every Zigbee2MQTT entity's availability includes
     zigbee2mqtt/bridge/state, which Zigbee2MQTT's Last Will sets offline when it stops; with the
     retained shadows (item 20) the values stay but show unavailable. Left: a Zigbee2MQTT that hangs
     without dropping its connection (no Last Will), and an actual test (a planned short stop).

25a. Nuanced reporting of suggested entries, based on conceptual versus physical positioning.
So, what is available to be positioned as potential capabilities, and what is available to be positioned at the conceptual layer.

26. Logical layer + available nuances in relation to "via device"  (netatmo radio module via main module) and aggregation.
    Availability via "via_device" (Erik, 2026-10-27): Home Assistant derives nothing from
    via_device -- it only shows "connected via" and the device hierarchy -- so when X hangs off Y
    and Y is down, X's entities stay available unless their own availability says otherwise. The
    generator should infer that dependency from the declared "via_device: Y;" (a device reference
    in the defs, Architecture.md §6.16), one declaration for both the hierarchy and the availability, like
    "dependency on" in Logical.def today. Avoid redundancy where a gateway's own discovery
    already operationalises it (Zigbee2MQTT puts its bridge/state into every device's
    availability; that doesn't mean EMS-ESP or Z-Wave JS UI do), but the defs must not depend on
    that: the dependency is declared, the operationalisation only skips what is already there.
    Also integrations as devices: e.g. "integration.ha2mqtt" as a device of its own, and a device
    it integrates gets "via_device: integration.ha2mqtt;" ("roof/envoy" is integrated via
    ha2mqtt); the same for Zigbee2MQTT, Z-Wave JS UI, EMS-ESP, hosts. To think through carefully.
    Plan (Erik, 2026-10-28, with 22b):
    - Integration devices (integration.zigbee2mqtt, integration.zwave_js_ui, integration.ems_esp,
      integration.ha2mqtt, ...), each with a node from its gateway-level availability topic --
      every gateway already puts one into each of its devices' discovery configs:
      zigbee2mqtt/bridge/state; zwave/driver/status + zwave/_CLIENTS/<client>/status; ems-esp/status
      (ha2mqtt: its bridge reporting) -- ANDed with a "dependency on" its compute host, and
      positioned next to that host (the rack).
    - Which device belongs to which integration: via_device where the gateway reports one (only
      Zigbee2MQTT does); otherwise from the shared availability topics -- a device whose configs
      carry the integration's topic belongs to it. That inference is a suggestion only (gateways
      provide what they "happen" to provide): written as a comment ("# suggested via_device:
      integration.zwave_js_ui (shares zwave/driver/status)"), never as a value. The value is the
      captured via_device or a manual one, overridable as any metadata line: "via_device: <device>
      forced;". A device with no shared topic at all is reported (needs human input).
    - Then a device's availability follows its integration's node (incl. the host dependency);
      22b's "ignore own availability;" (keep only the shared, gateway-level entries) is the first
      step of that.
    - Built 2026-10-28 (Architecture.md §6.16, "Integration devices"): integration.zigbee2mqtt
      (renamed from node.zigbee2mqtt_bridge), integration.zwave2mqtt and integration.ems2mqtt (in
      "integration mqtt"), integration.ha2mqtt (ping-pong) -- positioned next to their hosts, with
      their hosts as dependency. Still open: the devices' own availability following their
      integration's node, and the via_device suggestions from shared topics (both above); the
      Hue Taps' nodes already follow integration.zigbee2mqtt (Logical.def).
    - Note for later (Erik, 2026-10-28), when the via_device relations are designed: "via"
      probably means "data flows via", which is not "depends on" -- and it can branch. Example:
      somfy_1 via ha2mqtt via Overkiz via connexoon, while ha2mqtt and Overkiz depend on 
      protocols-server-1. 
      So the data path
      (via_device chain) and the dependency graph (what must be up) are two different relations;
      availability should follow the dependencies, not simply the via chain, while the via chain should not mix in the "what must be up" hierarchy. 
  
  Capability level mapping of values at logical level?
  Tap 1,2,3,4 => 1,2 = event.event_1 { toggle }
               3,4 = event.event_2 { toggle }
That "toggle" value is just a placeholder for the rename/combination of the original strings.
But we do not need to see these at the conceptual.def level
Plus "conceptualisation" of the triggering rules.
Should be byte level equivallent. 
  
- Note (2026-09-22, from the coordinator-scope discussion that led to the operational-role
  split (then item 19) -- see memory: project_event_bus_operational_vs_coordinative_roles.md and
  project_junglinster_coordinator_crash_loop_2026_09_22.md): Logical.def work breaks down into (at
  least) four distinct situations, worth keeping distinct rather than treating "the logical layer"
  as one undifferentiated bucket when this item is actually designed:
    1. Simply clustering existing capabilities (no new computation, just regrouping/naming).
    2. Defining derived entities that require TIMING (delay_on/delay_off, and similar).
    3. Defining derived entities that rely on OTHER entities from OTHER devices (or from the main
       HA instance) -- cross-device composition.
    4. Defining ABSTRACTED entities that accommodate for shortcomings in the actual device (e.g. a
       device that never reports its own state back, or one that works via multi-broadcast radio
       messages rather than a clean request/reply) -- the FHEM-motivated case.
  Of these, (2) and (4) are the two that could plausibly be implemented either IN the coordinator
  or in a separate component (per that discussion: (4) in particular is operational, not
  coordinative, and may belong in that same higher-reliability separate role rather than the
  coordinator) -- an open design question, not yet decided, to resolve when this item is picked up.
- Deal with the "physical" sphere. Should be empty, where contributing sensors need a conceptual name as well based on the location the material (= real physical) world.
  - Prerequisite step identified 2026-10-15: a rule's "when" clause breaches abstraction by
    containing raw, device-level protocol codes (e.g. "event.physical:door/moes/1 is 2_single or
    ...") -- those belong in the device declaration, not the rule. Step 1 DONE: a new device-block
    construct, "trigger <sphere:path> when <capability> is '<value>'[or in ['<v1>', ...]];"
    (Conceptual_NamedTriggerEntities.go), replaces the explicit "entity ... from <capability>;"
    line for this case. The underlying hidden entity's own local path comes from the device's own
    "at <leaf> with:" header (new, narrow, opt-in-per-line-via-"::" mechanism -- deliberately NOT
    the blanket "device <id> as <leaf> with:" grammar removed outright 2026-10-11, item 10) when
    given, so migrating an EXISTING device's raw entity to this construct reproduces its live
    entity_id exactly rather than inventing a new one; falls back to deriving from the device id
    itself for a device with no live entity_id to preserve. A rule references a named trigger via
    "when trigger.<sphere:path> is triggered" -- deliberately reusing the "trigger." prefix
    "trigger.<attr>" template conditions already use (same underlying idea: a derived value built
    from the firing context), not a separate keyword; parses for free via the ordinary
    entity-ref grammar (domain "trigger"), swapped to Kind "named_trigger" after the fact
    (resolveNamedTriggerReference, rule_parser.go) -- no special lookahead needed. The name itself
    is resolved space-relative (normalizeEntityFullName), same as any other entity reference, so
    the same relative name written in two different spaces does NOT collide. Multiple devices may
    declare the SAME name, OR'd together by the generator into one automation's trigger list
    (administration.go's NamedTriggers registry). First real migration DONE+VERIFIED live on
    Vienna 2026-10-15 (hallway_door_moes/hallway_bedroom_door_moes, 3 rules) -- byte/semantically
    identical generated automations confirmed against the pre-migration baseline.

    Step 2 DONE 2026-10-15: the trigger declaration's own optional "and if trigger.<attr> is
    '<value>';" suffix (the Signify dimmer's press/hold + button-direction case, e.g. "trigger
    social:main/light/dim_up_request when event.pressed is in ['press','hold'] and if
    trigger.button is 'up';") folds into the generated automation's own gating condition at
    generation time (namedTriggerImpliedCondition, generator.go) -- scoped narrowly on purpose:
    only applies when EVERY trigger on the rule is a named trigger and all their sources agree on
    exactly one (attr, value) pair; a mix of trigger kinds or genuine disagreement falls back to
    no implied condition (TNamedTriggerSource's own doc comment, administration.go). Verified
    live on Vienna's whole Signify dimmer (4 rules: turn_on/turn_off/dim_up/dim_down) -- generated
    automations confirmed identical (dim_up/dim_down's own trigger list merges what used to be 2
    separate same-entity trigger entries into 1 with a 2-item event_type list -- same HA schema,
    confirmed semantically equivalent, not a regression).

    Action-verb cleanup DONE 2026-10-15 alongside it: "dim <entity-ref> with <N>%;" /
    "brighten <entity-ref> with <N>%;" (rule_parser.go's DimAction/BrightenAction) are sugar for
    "turn <entity-ref> on with brightness_step_pct -<N>/<N>;" -- no raw negative number/HA field
    name in rule text. "with [default] brightness <N>%;" (DataFields) is sugar for "with
    brightness_pct <N>;" -- "default"/"%" both optional and purely cosmetic, discarded at parse
    time (not a second kind of data field).

    A LOGICAL-layer generalisation floated 2026-10-15, not designed/scoped yet: a "derived
    capability event.YYY with: trigger DDD.XXX turns ZZZ; [device_class ...;] [delay_on ...;]
    [delay_off ...;] end;" construct, letting a derived capability be defined by another
    device's own capability transition (plus HA-style debounce), rather than only the existing
    condition-expression form.

    Migration + live deploy status (updated 2026-10-18): Vienna's hallway door family, the whole
    Signify dimmer, living_room door/kitchen_bj family, terrace/light/toggle (cross-room), and
    bedroom's own door family are all migrated -- 25+ rules, generated output confirmed
    byte-identical to pre-migration, and DEPLOYED LIVE to Vienna 2026-10-18 (real `./generate` +
    `./deploy`, not just a test fixture check).

    CORRECTION 2026-10-19: shower_room's door_moes_1/door_moes_2 family turned out to already be
    migrated (found while fixing the regression below) -- the "not yet migrated" note below was
    wrong.

    Real regression found + fixed live 2026-10-19 (full writeup in README.md's Operational notes):
    every Moes multi-gang button in Vienna broke after this migration -- the coordinator's own
    pre-existing discovery-layer value_template normalizes a multi-gang button's raw action
    ("1_single") into event_type "single" + a separate "button" attribute, which the migrated
    rules never accounted for. Fixed by extending the trigger declaration's embedded attr-condition
    to accept a list of values too, and a new per-source "id"-scoped OR condition
    (namedTriggerSourceCondition, generator.go) for when several different devices feed the same
    named trigger with DIFFERING button-number requirements (falls back to the original simple
    shared-condition shape when they agree, so the Signify dimmer's proven output stays
    byte-identical). 43 Conceptual.def lines fixed, deployed live, first manual test confirmed
    working -- full verification still pending as of 2026-10-19 night.

    Still open, next concrete steps:
    - Confirm the Moes button fix above actually works across ALL affected rules, not just the
      first one tested.
    - A fresh sweep of the rest of Vienna's Conceptual.def for any other remaining raw
      event-code/protocol leakage into `when` clauses the earlier batches might have missed --
      don't assume the file is fully clean without re-checking.
    - Junglinster's Conceptual.def: zero rules migrated to this construct so far -- entirely
      separate, not started. Given Vienna's own Moes devices ALL had the same latent bug, check
      whether Junglinster has any Moes-style multi-gang buttons before migrating there, and build
      the trigger declarations with the normalized event_type + button-attribute form from the
      start rather than naively copying raw action values again.
    - Not all rules need to be "better conceptualised" at once (confirmed with Erik) -- migrate
      rule by rule as time allows, verifying byte/semantic identity against the pre-migration
      baseline each time, same as this batch.
    See /Users/erikproper/.claude/plans/binary-weaving-iverson.md for Step 1's own full design.
- Node/battery_alert rules: DONE -- Rule 1 (node required, live since 2026-09-10) and Rule 2
  (battery_level iff battery_alert, live since 2026-09-11 once both houses' real macro-driven
  usages were migrated to `derived`); see
  /Users/erikproper/.claude/plans/derived-capability-mechanism.md for full history.
- Logical.def + first mechanism ("dependency on <device-id>;"): DONE 2026-09-16, not yet deployed
  -- a logical device (same id as a physical one = extension, not a second device) can declare
  `dependency on <other-id>;` (repeatable, transitive, cycle-detected) so its own availability
  ANDs in another device's own liveness; `hosts`-kind dependency targets only so far. Vienna's
  `node.vienna_livingroom depends on host.netatmo` is the first real usage, replacing the old
  `netatmo_module` placeholder device. QUEUED remaining work for whenever this item is picked back
  up: two more dependency-target forms, `dependency on entity <entity-id>;` (raw state) and
  `dependency on availability of entity <entity-id>;` (raw availability) -- both name an
  already-existing "main" HA entity directly rather than a Physical.def device id, needing (a) a
  new generator-authored automation on main publishing that entity's state/availability to MQTT
  (only when the dependent device is actually positioned/exported -- mirror the existing
  `hasLink`-style gating), and (b) adding the referenced entity to kind-5's existence-check list.
  Full design/implementation detail: memory/project_logical_layer_dependency_on.md.
- Device-positioning grammar redesign (2026-09-24, in progress): `device <spec> from <device-id>
  [with:]` / `device from <device-id> with:` collapse into one `device <device-id> [as
  [sphere:]leaf] with: ...;` construct (device-id first, no "from"; "as" replaces `<spec>`,
  defaulting to `infrastructural`), plus a `with <entity-statement>;` one-liner for the common
  single-capability case. Same device-id legal in as many blocks as needed, no first/subsequent
  distinction (register-once-merge-repeatedly; the old "already positioned, ignoring duplicate"
  guard is retired for this construct -- real double-declarations are already caught at the entity
  level). "node" stops being auto-registered anywhere, including `hosts` (today's one unconditional
  case) -- every kind now needs an explicit `entity binary_sensor.<sphere>:node;`-shaped line, same
  as any other capability; the Logical.def "node override" mechanism moves from being
  positioning-triggered to a check at the top of the generic per-capability dispatcher instead.
  Deliberate tradeoff, confirmed with Erik: `hosts` becoming explicit is more verbose for the
  simple ping/cpu case, accepted anyway to avoid making the mechanism depend on which specific
  integration a device happens to be. Note for later (2026-09-24, refined): rather than reviving the
  old macro mechanism specifically for this, look at a "device type" TEMPLATE construct instead -- a
  named template that expands into a full `device ... end;` clause for the common "basic node"
  shape, rather than a single-line substitution. Not inherently a logical-layer thing, but this is
  the natural moment to look into it since it falls directly out of this same redesign -- worth
  scoping properly (what else besides "basic node" would want a device-type template, parameters,
  where it's declared) when picked up, not bolted on ad hoc.
- complement device DDD with:
    from DDDx:
      CCCx [as CCCy];
    end;
  end;
  with the usual abbreviations when there is only one.
  Semantics: with this definition at the logical layer, physical device DDD is now complemented with the listed capabilities from the other physical devices. 
  The "as CCCy" indicates an optional rename, if a name clash
  would occur.
  The conceptual layer "sees" the extended version, but this does not influence the device map! So if CCCx/CCCy is "used" to materialise an entity E, then E still "belongs" to physical device DDDx.
- virtual device DDD with:


   - Note (2026-09-06): future step -- massage today's raw "physical:" sphere usage in Spaces.def
     out during the logical layer step itself, not as a separate migration first. The concrete
     case driving this: aggregating several same-kind physical sensors in one room (e.g. multiple
     temperature sensors) has no representation above raw "physical:"-sphere entities today; with
     the logical layer, a device's own capability entity can instead be declared to *collect* into
     a space-level aggregate conceptual entity:
         device infrastructural:netatmo from hass.office_corridor with:
           entity sensor.co2      collect sensor.co2;
           entity sensor.humidity collect sensor.humidity;
         end;
     Here the right-hand `sensor.co2`/`sensor.humidity` are SPACE-level conceptual entities
     (defined once at the enclosing space, not per-device) -- each device capability declared with
     `collect` becomes one of potentially several contributing sources feeding that single
     space-level aggregate, instead of each device getting its own separate physical-sphere entity
     as today. "physical" may well stay part of the eventual naming for these device-level source
     entities (`sensor.physical:...`) even after this lands -- the point of this step is removing
     the raw, ungrouped multiplicity at the conceptual layer, not necessarily the sphere name
     itself.
     Aggregation itself (combining collected values into the one space-level entity) should be
     implemented as a generator-authored YAML template sensor on the MAIN HA instance, not as
     coordinator-side logic -- letting the coordinator do this work would mean re-implementing
     HA's own templating/aggregation engine for no benefit, when the main instance already has it
     for free.

   - Note (2026-09-01): enforce that every device entry, for any integration, provides a node
     definition -- currently only "home_assistant" bridge devices (native + imports) can lack one
     (a "node" capability is optional there; "hosts" devices always get one unconditionally, and
     "discovery" gateway devices have no per-device node concept at all). Decided so far: this
     should be a HARD error (aborts generation), not just a warning -- but not yet implemented,
     since Junglinster's real Physical.def currently has 11 devices without one (hass.envoy + its
     10 inverter sub-devices), which would break generation immediately. Needs more thought before
     building: where to enforce it (Physical.def collection time, so it catches every declared
     device regardless of whether it's ever positioned in Spaces.def, vs. today's
     positioning-time-only check), and what to do about the 11 real devices first.
     Note (2026-09-06): see the device-combining note below for a real case
     (hass.fritz_box/host.fritz_box) suggesting this hard-required-node decision may need
     revisiting once logical devices can "lend" a node capability from a sibling physical device --
     possibly only logical devices (and exported devices) need the hard requirement, not every
     physical one.
- Device combining: "aggregate" a new device vs "absorb" into an existing master device (e.g.
  smarty's Zigbee switch into host.smarty) -- another facet of this item alongside "complement".
  Gated on items 4/7/8 (Zigbee2MQTT) and 6 (Z-Wave) both landing.
- Command choreography + the inverse direction (2026-09-09): not designed anywhere yet -- "to be
  discussed." If a logical/aggregated entity accepts commands, should an inverse of whatever
  composed its state exist to split an incoming command back down to its underlying physical
  entities? Concrete forcing case: a `switched_device` combination (a Zigbee dimmable light whose
  actual mains power is switched by a separate Z-Wave wall relay, plus a third device -- a Zigbee
  remote control -- that can also drive the combination) -- today hand-encoded at the conceptual
  layer via generator-authored YAML template light entities + automations, wanted at the logical
  layer instead (see splendid-zooming-lighthouse.md's "Deferred to item 12" section for the full
  worked example [FLAGGED 2026-09-14: that plan file has no section by this or any similar name --
  checked its full current contents, which only mention PROJECT.md items 4/11/14 in passing, all
  using its own 2026-09-09-era numbering, itself now stale after three later renumbering rounds.
  This cross-reference has been mechanically renumbered through all three rounds (14->13->12)
  without anyone verifying the target ever existed; the worked example itself may simply never have
  been written down anywhere. Needs a human decision: either write the worked example (here or in a
  fresh plan file) or drop this pointer]). This is NOT a value-substitution problem like `derived
  from` -- turning the
  combination "on" has to sequence actions across two physical devices (switch the relay on if not
  already, wait, then set the dimmer's brightness to a remembered last-used value), so it needs a
  real command-routing/choreography story, not an inverse function. Linked to item 9's own open
  "Check if exports of commands would still work" question.
- Availability composition, noted 2026-08-31 once the physical-layer coordinator work landed a
  device's node-topic-gated + self-check availability (`buildAvailabilityFields`, discovery.go):
  that two-factor AND is the right amount of refinement for a single physical-layer device, but
  "aggregate"/"absorb" combine devices whose availability chains can be deeper and don't compose
  by simple ANDing alone. Two concrete cases to design against once this item starts:
  (a) a compute host running an integration (e.g. protocols-server-2) has its OWN liveness,
  independent of any device it reports on -- an entity can be "available" per its own device/node
  signals while the host reporting it is itself down; this is the still-open "stacked/dependent
  availability" reminder already flagged in splendid-zooming-lighthouse.md's out-of-scope section.
  (b) Netatmo's IP-connected base modules add a layer of their own: an outdoor/rain/wind module's
  true availability depends on ITS radio link to the base module AND the base module's own IP/WiFi
  link to Netatmo's cloud AND (a) above -- a chain, not a flat set, and today's per-entity "node"
  capability only captures the nearest link in it. Not designed; revisit once this item is
  actually being built, alongside aggregate/absorb themselves.
  Note (2026-09-05), from the cross-house import "node" liveness fix: whatever availability chain
  the EXPORTING installation ends up composing internally for a device (its own liveness, a base
  module's link, a radio link, etc.) must stay entirely its own concern -- the cloud catalogue it
  publishes should keep exposing exactly one boolean per device (today's "node" capability), never
  the individual links that fed it. From an IMPORTER's own perspective there are only ever two
  independent availability dependencies to reason about: (1) the remote device's own reported
  available/not-available state (whatever composed that, on the exporter's side), and (2) this
  house's own connection to the cloud broker. Neither should ever surface any of the exporting
  installation's own internal dependency structure into the importer's device map -- revisit this
  constraint explicitly once the composed-availability design above is actually built.
  Note (2026-09-06), a second concrete case found live: `hass.fritz_box` (a `home_assistant`
  bridge device, reports `sensor.gb_received`, no native node capability of its own) and
  `host.fritz_box` (a `hosts`/ping device, ping-based node) are two separate Physical.def
  declarations describing the exact same real-world router -- the DSL has no way to say so today,
  so `hass.fritz_box` correctly gets flagged as having no node capability, even though the
  physical unit it represents unquestionably has one via its sibling declaration. Interim
  workaround (in use now): fake a node capability on `hass.fritz_box` via the same "is available"
  sugar already used elsewhere (`binary_sensor.node: <already-reported entity> is available;`).
  The real fix is a logical device's ability to "lend" a node capability from whichever
  constituent physical device actually has one, rather than requiring every physical device to
  carry its own. Also matters for `export`: an exported device (physical or logical) needs exactly
  one node capability for the cloud catalogue's "one boolean per device" contract to hold, but a
  physical device on its own may have none and would need to borrow one from a sibling the same
  way.
  Note (2026-09-14), a third concrete case found live, same underlying shape as the fritz_box one
  above but on the OTHER side of the physical/logical boundary: Junglinster's Physical.def has
  `node.vienna_bedroom` and `node.vienna_livingroom` (two separate `home_assistant`/hassbridge
  devices) both declaring `binary_sensor.vienna_bedroom_connectivity` as their own "node"
  capability's source entity -- a deliberate workaround (the living room's own dedicated
  connectivity sensor has proven too flaky; the bedroom module's own connectivity signal is reused
  instead, on the assumption both radio modules share the same real link reliability). Found live
  while building `via_device` (finished, formerly item 3, see /Users/erikproper/.claude/plans/via-device-inference.md):
  the entity-existence tracker has no way to represent "this
  one source entity is legitimately claimed by two devices" (`TEntityExistenceEntry.DeviceID` is
  single-valued), so its own reconciliation pass kept flipping which device the shared entity's
  tracked entry belonged to. Worked around at the physical layer for now (`ResolveViaDevice` keys
  off each device's own declared source entities directly rather than the tracker's ambiguous
  attribution, so `via_device` resolution itself is unaffected) -- but the right fix is the SAME
  one already named above, generalised: this kind of node-capability "reuse"/"override" should be
  **disallowed at the physical layer** for `home_assistant`/hassbridge devices (a physical device
  should only ever declare a node capability sourced from ITS OWN reporting), and instead
  **expressed at the logical layer** -- a logical device explicitly stating "borrow this sibling's
  node capability instead of my own/instead of none." More generally than either concrete case:
  the logical layer should support overriding a device's own node capability from ANY other
  device's own reported liveness, physical-layer kind-agnostic -- a `home_assistant`-bridged
  device borrowing a sibling `home_assistant`-bridged device's own node (this Netatmo case) is the
  same shape of problem as a `home_assistant` device borrowing a `hosts`-kind device's own
  ping/cpu-based node (the fritz_box case), and should be one general mechanism, not two separate
  ad hoc physical-layer workarounds. This also implies a rewiring of availability logic through the
  logical layer once it lands: today's physical-layer `buildAvailabilityFields` (node-topic-gated +
  self-check) has no notion of "my node capability is actually borrowed from elsewhere" -- see the
  "Availability composition" note above, which already flags the closely related "stacked/
  dependent availability" problem this same design work should solve together, not separately.

28. Check space level aggregation of sensors. If one is down, what do we do with data?

29. Installation-level status binary_sensors (meta sphere): one (discovery-created) binary_sensor per HA
      instance signalling (1) a configuration problem on that instance, and (2) updates available
      for it -- purely passive/informational (dashboard-visible), deliberately decoupled from item
      2's reload/restart meta-command mechanism rather than gating it synchronously (see item 2's
      own 2026-09-05 scope decision). Not designed yet:
      [FLAGGED 2026-09-12: "item 2" is Stick migration today and has no reload/restart
      meta-command content -- this reference predates several renumberings and its real target
      can no longer be traced; likely pointed at an item that was later completed and removed
      per this file's own "drop finished items" convention. Needs a human check, not a guess.]
   - (1) config-problem signal needs a per-instance automation to actually call `check_config` (or
     equivalent) and publish its own retained status, which the coordinator turns into a discovery
     binary_sensor per installation -- the same "per-instance status becomes a per-installation
     MQTT discovery entity" shape kind-2/kind-3 existence status already uses, just for a different
     underlying fact.
   - (2) updates-available has no single source across install types: HAOS's own `update.*`
     entities (`update.home_assistant_core_update`/`_supervisor_update`/`_os_update`) are the
     natural source where Supervisor is present, but a plain Container install (e.g.
     protocols-server-2, if still bare Container) has no update entity to poll at all -- needs its
     own per-install-type sourcing story before this can be built uniformly.
   - (3) "Guarding automations" (2026-09-22, not yet designed): some HA-based integrations have
     their own quirks -- concrete case found live the same day, a Netatmo integration on
     Junglinster's `ha2mqtt` instance that went unavailable and needed a manual reload to recover
     (see memory: project_via_device_sibling_correction_bug_2026_09_22.md and
     project_junglinster_coordinator_crash_loop_2026_09_22.md for the same-day incidents this
     surfaced during). The general pattern: if an integration-backed entity is unavailable, attempt
     a reload of that integration, rate-limited (e.g. max 4 attempts per hour) to avoid a reload
     storm on a genuinely-down integration. The user's own stated preference: implement as a
     GENERATOR-CREATED automation living ON the affected instance itself (e.g. `ha2mqtt`) -- not a
     separate "guarding coordinator" component -- consistent with this item's own existing "meta
     sphere" shape (per-instance automations feeding coordinator-published status).
     Refined same day: guard rules are declared in Physical.def's own INTEGRATION sections (per
     integration type, alongside that integration's other declarations -- e.g. inside a
     `home_assistant` block), not as a separate generic "meta" DSL construct bolted on elsewhere.
     Their parsing is INTEGRATION-SPECIFIC, matching this project's existing per-integration parser
     architecture (e.g. integration_hassbridge_parser.go, integration_hosts_parser.go, ... -- each
     integration type owns its own grammar) -- a Netatmo-flavoured guard rule need not look
     syntactically like, say, a Zigbee2MQTT one. Needs its own design pass: how the automation
     identifies "this entity is unavailable because of an integration fault" (vs. genuinely offline
     hardware), how the rate limit is tracked/reset, and how many/which integrations this should
     cover beyond Netatmo.

30. Jointly check the consistency and cleanliness of the DSL.

30a. Review the per-house `generate`/`deploy` scripts (`deploy`, `deploy.d/*`) and decide what to
     offer as generic infrastructure for other users. The pushes to the main HA and the ha2mqtt
     instances, the registry sync, and the reload-or-restart choice (`MetaAction`) are the same in
     both houses apart from host names and ports. Those could be generated from Physical.def, or
     become a shared tool. Also standardize them on one language, Python or Bash (Erik,
     2026-10-26) -- today the deploy is Bash with an embedded Python registry repair.
     Future step (Erik, 2026-10-26): the registry repair as a proper Go app, a "pre-deploy" step
     driven by the list of expected entries (registry_manifest.json), talking to Home Assistant's
     websocket API directly instead of running a Python script over SSH inside the HA add-on. Its
     credentials (a long-lived access token per HA instance) then become explicit secrets in the
     def files, like the MQTT secrets -- today the script picks up the Supervisor token implicitly
     inside the add-on.
     With it (Erik, 2026-10-26): an "as local <domain>.<object>" option on a Physical.def native
     ("integration local") capability, naming the entity's CURRENT HA entity_id, e.g.
     "capability media_player.media_player as local media_player.office;". The pre-deploy step
     looks that entity_id up in HA's registry (read only), and writes the integration's own
     platform and unique_id back into Physical.def as the capability's frozen id, replacing the
     "local ..." -- from then on the native entity is identified by its real unique_id, like the
     generator's own, and the registry repair renames it to its positioned entity_id from the very
     first deploy (no matching by hand in the HA UI, no previous-manifest comparison by source).

31. Code cleaning (dead code, superseded generator logic, parser)
==> Dead code found 2026-10-27 (go/ast analysis, not from item 19, left out of its cleanup): generator
    firstNonEmptyEnv (defined.go), extractVariableName, findAggregateConstituents,
    formatSpaceLabel, scanInputNumberBody (expander.go), bootstrapAutomationBody
    (remote_instance_automations.go), and used only by tests: impliedAggregatesForSpace,
    lookupIcon, lookupAggregatedDomain, homeAssistantCapabilityEntityIDs, ResetGlobalVariables;
    coordinator, used only by tests: drainDiscoveryRelayQueue, loadTopicManifest,
    deviceEventEndpoints. Cross-check them against the grammar first (Erik): a dead function may
    belong to a DSL construct that should go too, or one that should still work.
==> CHeck if node, temperature, etc, are acknowledged as axplicit "sub domains" in the code.
==> Start by checking the dead DSL constructs, and clean the assocuated code.
- Rename the `homeassistant_instances/...` MQTT topic tree (bootstrap/bridge protocol,
  discoveryhassbridge.go) to `homeassistant.instances/...`, matching this project's own
  dot-separated discovery-prefix convention (`homeassistant.physical`/`homeassistant.conceptual`)
  -- purely a naming inconsistency introduced 2026-08-23, not yet fixed. Safe/mechanical now that
  both the coordinator code and every instance's automations are generator-owned; just needs a
  coordinated regenerate+redeploy across every instance at once.
- `isCoordinatorOwnedTopic`'s legacy `host_`/`discovery_` topic-shape branch
  (house_event_bus_coordinator/discoverycleanup.go) exists only to catch pre-migration discovery
  topics during the one-time cutover to identity-based topic keys
  (`.../coordinator/<stable-id>/config`). Once each deployed house's coordinator has run once and
  its retirement log confirms the old topics are gone -- Junglinster first, Vienna once it's
  actually deployed -- remove that branch; it'll be permanently dead code by then. Given how many
  times both houses' coordinators have now run and been verified live (2026-09-08/09's MQTT broker
  migration alone), this is very likely ready to action -- just needs someone to actually check
  the retirement log and pull the branch.
- The `roaming` / `import` (hassbridge) mechanism is unused since 2026-10-21: its only user, the
  iPhone, dropped it in both houses (the Companion app already reports to both Home Assistants
  directly; ~94% of updates reach both, gaps < ~70 min). Candidates: `export_as`/`self_import_from`
  (THassBridgeDevice, generator emission + parser keywords), `subscribeHassBridgeSelfImport`,
  `canonicalizeRoamingBridgeTopic`/`canonicalRoamingInstance`, `hassBridgeAcceptsInstance`'s
  self-import branch, plus related doc comments. Check zero `.def` usage in both houses first.
- General principle: once a platform has fully moved through its incremental integration steps
  (items 4/5/6/7/8 above), retire the now-superseded generator logic for it outright rather than
  carrying both paths indefinitely -- the dead-code entries above are concrete instances of this.

Note (2026-10-05): a dead-code pass ran against this item plus item 11's own historical inventory
(both confirmed zero live `.def` usage first, then removed, then verified byte-identical regenerate
on both houses before and after each removal -- see git history for the exact commit):
- The `condition`/`with adjustment` bare-entity mechanism this item used to describe (parser.go's
  own " with adjustment "/" with condition " inline extraction, `generateTemplateSensors`'s
  adjustment branch, `buildAdjustmentStateExpr`) -- DONE, removed. Also removed the SAME-shaped
  bare "value <entity>!<attribute>;" directive (`TEntityRecord.ValueExpr`, `buildValueStateExpr`),
  found zero-usage during the same audit, not previously tracked anywhere.
- Item 11's own old Logical.def "is available"/enabler/switch-coercion capability shapes
  (`TLogicalCapability.IsAvailable`/`.EnablerEntity`/capability-level `.DependsOn`/`.NoPlayInput`/
  `.Entity`, `registerLogicalIsAvailableCapability`/`registerLogicalMediaSwitchCapability`/
  `resolveLogicalDependencyNodeEntities`) -- DONE, removed.
- The short-lived "flipped" Logical.def absorb qualifier (built, then superseded by "inverted,"
  all same-day, 2026-09-05 -- never deployed) -- DONE, removed (`TLogicalCapability.Flipped`,
  `registerLogicalFlippedAbsorbedCapability`, `generateFlippedSwitches`/`FlippedSourceEntityID`).
- A pre-existing, independently-dead `TEntityRecord.NodeEnablerEntityID`/`.NodeDelayOff` pair
  (`buildTemplateNodeWithRawYAML`/`buildTemplateNodeRawYAML`, generator.go) -- found during the
  same audit (never actually set anywhere, a macro-era leftover missed when item 3's macro removal
  landed) -- DONE, removed.
- The flat-entity-form parser ban (memory: project_flat_entity_form_ban.md) is now unblocked too
  (zero remaining usages confirmed the same day) but NOT yet implemented -- a real hard-error
  addition, not a removal, left for a deliberate follow-up rather than folded into this pass.
- Still open, unrelated to the above: the `homeassistant_instances/...` topic rename and the
  `isCoordinatorOwnedTopic` legacy branch, both listed above.

31a. Batteries: a date-based assumed charge percentage (plus a reset, e.g. when recharged or
    replaced), with its own low-battery warning -- works much better for rechargeable batteries
    than what the devices themselves report. Example: Junglinster's dining Philips dimmer has reported
    ~1 % for six months while working fine (2026-10-23).

31b. Maybe later: Sarah's bedroom cover rules (open/close from its door tap) "smart": slow speed
    at night, normal speed otherwise.


32. Architectural review, code review and documenting
   - Note (2026-09-05): GO_CONVENTIONS.md §7/§8 specifies a three-layer parser architecture
     (character stream -> tokeniser -> recursive-descent, CDL1 bool-returning style, no `error`
     returns from parse functions). The DSL frontend (Physical.def/Spaces.def parsing) does not
     follow this -- it's line-based `regexp.MustCompile`/`FindStringSubmatch` matching throughout,
     confirmed spanning at least 15 non-test files including the generic layer (`Physical_Parser.go`,
     `layers.go`, `expander.go`), not just the per-integration parsers. Flagged by Erik ahead of
     the code-review step above; expected to be remedied, but deliberately NOT attempted piecemeal
     inside unrelated feature work (e.g. the 2026-09-05 import-grammar unification, which
     stayed in the existing regexp style rather than migrating just the one file it touched) --
     needs its own dedicated pass given the scope (the whole DSL frontend, not one file).
   - Note (2026-09-29): concrete evidence of the cost of NOT having done this yet. A newline
     (and, earlier, a tab) not being treated as plain whitespace kept resurfacing as a real bug in
     new shapes -- first tabs inside a single line, then a one-liner "device X with" wrapped right
     after "with", then (found live, same day) an "is switched by" one-liner wrapped mid-expression
     -- because the line-based matching this item describes has no real notion of "whitespace"
     the way a proper character-stream tokeniser (already built once, correctly, for condition/value
     expressions in `logical_condition_expr.go`) does. Patched pragmatically for now
     (`normalizeDefinitionWhitespace`/`joinWrappedStatementContinuations`, `defined.go`: any
     physical line not ending in a real terminator `;`/`:`, quote-aware, comment/blank-exempt, gets
     joined into the next) rather than waiting for this item -- but the real, durable fix is this
     item: a genuine character-stream/token-stream front end where newline is classified as
     ordinary whitespace throughout, not a per-construct patch job. Treat this as a concrete
     requirement/test case for whenever this pass actually happens, not just an aside.
   - Note (2026-10-05): audited both houses' Conceptual.def for any remaining "free floating"
     entity definitions (an `entity ...;` statement not positioned via a `device ... with: ...
     end;` block) -- Erik's own earlier complaint about the `switch.physical:heating_raw`/
     `flipped` pair (since migrated away). Confirmed clean: Vienna has zero; Junglinster's only
     candidates (7 sites, e.g. `sensors.house_heating_power_meter as furnace`/
     `node.sonos_office_left`) turned out to be the ALREADY-FIXED "wrapped one-liner `device X
     with`" shape from this item's own 2026-09-29 note above -- `joinWrappedStatementContinuations`
     already joins them correctly before `parser.go` ever sees them, confirmed live via
     `coordinator/discovery.yaml`/`devices.yaml` (correct gateway/leaf/area/node_entity for all 7,
     not a generic fallback). `extractEntityDeclaration` (`expander.go`) remains a structurally
     UNGATED catch-all, though -- it's reachable regardless of device-block nesting, so a genuinely
     malformed or unattached future `entity ...;` line would still silently succeed with no
     warning, same root cause as every other entry in this item. Another concrete case for the
     real character-stream/token-stream rewrite (gate the generic fallback to where the grammar
     actually allows it) rather than a point patch -- not fixed now, since this item's own pass is
     the right place for it, not a drive-by.
   - Note (2026-10-22, Erik): keep the `entity <spec> from <domain>.<label>;` form usable when
     cleaning the grammar. Item 16 makes it unnecessary in most lines (a capability named after
     its entity's last path segment, or after its domain), but it stays for the cases where the
     capability's name and the entity's path legitimately differ.


38. FHEM/FS20 integration

    Note (2026-09-22, architectural decision, not yet designed in detail): some integrations
    (FHEM is the motivating case) bridge devices that accept on/off commands but never report
    their own state back. Relaying the command alone (FHEM2MQTT's job) isn't enough -- something
    needs to remember the "expected" state and periodically re-broadcast/re-enforce it, since
    there's no device-reported feedback loop to rely on.

    Decided: this "expected state + periodic re-enforcement" responsibility does NOT belong inside
    the coordinator, even though it first looked like a natural extension of it. The coordinator's
    job is COORDINATIVE ("what is available", discovery/existence bookkeeping) -- this new
    responsibility is OPERATIONAL (actually exchanging state/command messages, materially part of
    keeping devices in their expected state). Consequences:
    - It should be its own separate "role" on the Event Bus, in its OWN container -- not folded
      into the existing coordinator process.
    - It needs to be held to a materially HIGHER reliability bar than the coordinator: an
      operational-role crash means devices can silently drift from their expected state (a real
      user-facing failure), whereas a coordinator crash (like the Junglinster incident this same
      day, see memory: project_junglinster_coordinator_crash_loop_2026_09_22.md) does NOT affect
      live state/command exchange at all -- FHEM2MQTT-style relays keep working regardless of
      whether the coordinator is up. This is the reasoning for why the coordinator, as currently
      built, is allowed to be less bulletproof than a component like this one would need to be.

    Not yet designed: how this new role subscribes/publishes relative to the existing coordinator
    and per-integration relays, how "expected state" is declared (DSL-level?), re-enforcement
    cadence, and how this interacts with FHEM/FS20's own protocol specifics.

    See item 11's own 2026-09-22 note for how this fits into the broader Logical.def design: this
    FHEM case is specifically situation (4), "abstracted entities that accommodate for device
    shortcomings" -- one of two Logical.def situations (alongside (2), timing-based derivations)
    where the coordinator-vs-separate-component question is still genuinely open.

39. RAW mqtt as integration; also see notes below on MQTT/JSON structures. So, a situation in which we have MQTT topics, but no discovery data. Then this integration needs to combine this into a discovery compliant topic structure. Sometimes easy to do based on topic and payload. But take the vacuum. If the composed state is represented as a different topics these need to be joint by the coordinator.
    Started 2026-10-28 as "integration mqtt" (Architecture.md §6.16): so far only binary_sensor
    capabilities from a topic ("capability binary_sensor.<label> topic "<topic>" on "<value>";"),
    used for the gateways' status nodes. To come: other domains, a value template for JSON, the
    vacuum's state from several topics, and availability per raw device.
    Retain (from item 20, 2026-10-28; Architecture.md §6.16): a raw device's declaration says
    "is retained;" when its topics are retained; without it its state entities read the
    coordinator's shadow, as for discovered devices. Each raw definition also needs its own
    availability -- "we also need to detect if they are really down" (Erik).
Test with shellys?

40. https://github.com/kaiplaketti/matter2mqtt

41. Publish -- Architecture.md should be ready for others to read, alongside an up-to-date
    README.md, on GitHub.

    Before then: `metaOptionalReloadServices`'s Jinja-templated "try every optional reload service,
    skip whichever doesn't exist" trade-off (remote_instance_meta_control.go) is noisier than
    intended -- found live 2026-09-10, template.reload/command_line.reload etc. each raise their
    own persistent HA Repair issue on any instance that doesn't have that domain, instead of the
    single load-time warning an unwrapped call would have given. Not a functional bug (continue_on_
    error already keeps the reload itself working), but looks unpolished for a wider audience --
    fix before publishing, e.g. by scoping which optional services get tried per instance instead
    of trying all of them everywhere.

    Possibly upgraded from cosmetic to functional (found live 2026-09-11, testing the sun/daylight
    hassbridge migration on Vienna's protocols-server-2): a meta-reload request there logged
    `Error executing script. Service not found for call_service at pos 3: Action template.reload
    not found` on EVERY attempt (both the original deploy's own trigger and a manual retry), and
    the newly-deployed daylight/node/solar_elevation reporting automations -- which trigger on the
    "automation_reloaded" event `automation.reload` (called dead last in this same script) is
    supposed to emit -- never fired either time. Not confirmed whether `continue_on_error` is
    genuinely failing to reach the later steps, or whether automation.reload DID run and something
    else is the reason our automations didn't see the event; parked per Erik's own instruction,
    not investigated further this session. Worth checking specifically when this item is picked up.

    Third occurrence, 2026-09-22/23 (Junglinster, the `environment.weather` device's
    `sensor.physical_terrace_weather_pressure`) ROOT-CAUSED: a BRAND-NEW template sensor file,
    correctly deployed to `/config/entities/template/sensor/physical/...` (confirmed via direct file
    read), stayed genuinely absent from the running instance until a MANUAL YAML reload, despite the
    deploy's own `PublishMetaReload` -> `homeassistant.reload_all` firing correctly. Root cause,
    confirmed via HA's own upstream tracker: both houses' generated `configuration.yaml` route
    EVERY domain (template, automation, binary_sensor, sensor, switch, script, scene, the input_*
    helpers -- all of it) through `homeassistant: packages: !include_dir_named integrations`, and
    Home Assistant has a long-standing, wontfix upstream limitation that **packages are never
    reprocessed by any reload service** -- not `<domain>.reload`, not `homeassistant.reload_all`
    (just a loop over those same per-domain hooks), and not even `homeassistant.reload_core_config`
    (github.com/home-assistant/core#12069, "Reload core config does not load packages," closed
    wontfix). Only a full restart re-runs the package merge. This isn't a narrow "new files
    specifically" gap -- it means `reload_all` is architecturally a no-op for this project's ENTIRE
    generated native-instance config, which reframes the 2026-09-10/09-11 incidents above as the
    exact same root cause, not a separate flaky-service problem. Full analysis, sources, and the
    live investigation transcript: this session's chat, 2026-09-23.

    DONE+DEPLOYED 2026-09-23: `configuration.yaml` generation (generator.go's `integrationDefs`/
    `generateConfigurationFile`, plus `remote_instance_automations.go`'s remote-instance skeleton)
    reworked to drop the `packages:` wrapper entirely -- confirmed every per-domain
    `integrations/<domain>.yaml` file was always a single `!include*` line for exactly ONE domain,
    never genuine cross-domain package grouping, so nothing was lost. Each domain (template,
    automation, binary_sensor, sensor, switch, script, scene, the input_* helpers, tts) is now a
    plain top-level key in `configuration.yaml` (`<domain>: !include integrations/<domain>.yaml`),
    restoring normal per-domain reload semantics -- `homeassistant.reload_all` should now actually
    work for this project's generated config, closing the 2026-09-10/09-11/09-22 incidents above.
    Deployed to all four live instances (Vienna main + ha2mqtt, Junglinster main + ha2mqtt), each via
    file deploy + a one-time `homeassistant.restart` (this specific structural change needs a
    restart to take effect the first time -- the same packages limitation being fixed here), each
    cross-checked healthy after (HTTP 200, zero config/setup errors, entity registry counts intact:
    128/1078/739/5287 respectively). Full test suite green throughout
    (TestGenerateInstanceAutomationTreesWritesMainAndRemoteTrees updated to assert the new plain-key
    shape instead of the old packages one).

43. Future refinement (2026-08-23, not yet designed): replace the `.def` files with an integrated
    database representing the coordinator+transformer's actual current understanding of the
    conceptual and physical layers -- physical-layer content populated/kept live from what the
    integrations themselves discover (MQTT discovery, device_attr(), live-reported metadata),
    rather than hand-declared and re-generated from static text each `./configure` run -- while
    still allowing reconfiguration of key components (e.g. which `home_assistant <qualifier>`
    instances exist) through it. Large, cross-cutting shift from the current
    Definitions-directory-as-source-of-truth model; needs its own design pass once the MQTT
    migration has landed and the shape of "what the coordinator already knows live" is clearer.
    Reinforced by 2026-08-24's protocols-server-2 rollout: compile-time (generator-authored
    automations, `.def`-driven expected-entity lists) and run-time (coordinator's live MQTT state)
    are still two separate worlds today, stitched together only by redeploys and manual "reload
    YAML" steps on each HA instance -- a merged compile-time/run-time coordinator, backed by that
    same database and possibly its own web UI, would remove that whole class of "did you
    redeploy/reload yet" friction rather than just papering over it with better tooling. Genuinely
    orthogonal to item 16's own parser-rewrite scope (that's about HOW the DSL is parsed; this is
    about WHAT the source of truth even is).

44. Future refinement (2026-09-09, not yet designed): a generic, reversible rewriting strategy
    between "one topic, one JSON payload with several fields" and "several topics, one atomic
    value each, one topic per field" -- treating a JSON payload's own field names as if they were
    additional trailing topic-path segments, e.g. `a/b/c {"k": "1", "l": "2"}` <-> `a/b/c/k 1` +
    `a/b/c/l 2`, in both directions. Directly motivated by hassbridge_commands.go's
    domainStatePayloadTemplate (2026-09-09): MQTT vacuum's state schema needs its state_topic
    payload wrapped as `{"state": "$"}` JSON, a one-off, hand-rolled special case for exactly one
    domain, when what's actually needed is the general mechanism -- every other bridged domain's
    already-atomic per-topic value is just this same idea's degenerate one-field case, and a
    genuinely multi-field state (e.g. vacuum's state + battery_level + fan_speed together) would
    need it for real rather than being hand-extended per field. The user had an MSc student
    previously work on this exact theme (topic/JSON <-> topic-tree rewriting) --
    https://repositum.tuwien.at/handle/20.500.12708/220349 -- worth digging up as a starting point
    before designing this from scratch.

    Related, also found live 2026-09-09: THassBridgeCapability.ValueMap (remote_instance_entity_
    reporting.go's applyValueMap) is this project's first DEVICE-specific (not domain-generic)
    value translation -- a Roomba's own native vocabulary ("home", "run", ...) doesn't match HA's
    MQTT vacuum activity strings ("docked", "cleaning", ...), so Physical.def now lets one
    capability declare its own "map: "<from>" "<to>";" pairs. Handled as a one-off DSL feature for
    now, but it's the same class of problem as the topic/tree rewriting above (both are about
    reshaping a value/payload between two representations) -- worth considering together if/when
    this item is actually designed, rather than as two unrelated mechanisms.

    A second, concrete motivating case (2026-09-21): the same vacuum went into a real fault
    ("stuck near a cliff" -- the Roomba's own iRobot integration exposes this as a detailed error
    reason, likely an attribute alongside its bare `state`) that the bridge today has no way to
    surface at all, since `domainStatePayloadTemplate`'s `{"state": "$"}` wrapping only ever
    carries the one `state` field through -- the detailed reason is silently dropped, main HA only
    ever sees the generic "error" state with no explanation. Once this item's own topic/JSON
    rewriting mechanism exists, relaying that extra field becomes straightforward; on top of that,
    a further (separate, later) step would be a notification/alert layer that actually surfaces a
    vacuum (or any device) entering an error state to the user proactively, rather than requiring
    someone to notice a stalled cleaning cycle and go check the app -- not designed either, but
    worth keeping in mind as the actual end goal this data would serve.

45. Future refinement (2026-09-21, not yet designed, long-run/end-of-list item -- deliberately
    parked behind publishing): replace item 8's own Overkiz-through-HA approach (the Somfy
    Connexoon gateway currently integrated via Home Assistant's Overkiz integration, then bridged
    out to MQTT via hassbridge) with a small, dedicated MQTT gateway built directly on the
    `pyoverkiz` Python API -- using HA merely as an Overkiz-to-MQTT relay introduces unnecessary
    overhead once a purpose-built adapter is an option. FHEM also has an Overkiz integration and
    could expose the devices via MQTT instead, but FHEM itself is an increasingly legacy platform,
    not a direction worth building on.

    The gateway would talk to Connexoon/Overkiz directly, publish device state and availability via
    MQTT, and translate canonical MQTT commands (open/close/stop/position) into the corresponding
    Overkiz calls -- a lightweight protocol adapter analogous to the existing Z-Wave and Zigbee
    gateways, keeping every Overkiz-specific detail out of the DSL/generator and out of Home
    Assistant entirely. Once built, item 8's own hassbridge-based SOMFY cover wiring would need
    replacing with a plain discovery-kind integration talking to this new gateway instead.

    Should follow the Zigbee2MQTT/zwave2mqtt pattern completely, not just the state/command halves --
    i.e. the gateway publishes its own HA MQTT Discovery config topics too, self-describing each
    device the same way Z2M/zwave2mqtt already do, rather than requiring this project's own
    generator/coordinator to hand-author that config. That's what makes it useful beyond this
    project's own "federated via coordinator" architecture -- a plain singleton HA setup (no
    coordinator at all) can point its MQTT integration straight at this gateway and get working
    Overkiz devices with zero project-specific glue, the same way anyone can already point a bare HA
    instance at a Zigbee2MQTT broker. MQTT was designed for exactly this kind of IoT device
    integration -- worth leaning into that as far as it goes, rather than building another
    project-specific adapter shape.

47. Future refinement (2026-10-04, not yet designed): neither the Logical layer nor any Physical.def
    integration kind other than `home_assistant` (hassbridge, kind-3) can be "exported" to the cloud
    broker today. Researched, not yet built: `export` (`device <id> export with: ...; end;`,
    Architecture.md §6.12) is wired exclusively onto `THassBridgeDevice`
    (`integration_hassbridge_storage.go`'s own `Export bool` field) -- there is no
    `TLogicalDevice.Export` field, no `export` keyword recognised anywhere in
    `integration_logical_parser.go`, and no equivalent on `discovery`/`hosts`/`local`/`commandline`/
    `import` devices either. A Logical.def device that derives/combines several physical devices
    into one conceptual entity (e.g. the `server.xanadu`/`server.backups` unification, 2026-10-04)
    has no way to cross-post that combined result to another house's coordinator the way a plain
    hassbridge device already can. Would need a `TLogicalDevice.Export` field plus parser
    recognition, and a coordinator-side mechanism resolving a Logical.def device's own entity_id(s)
    the way `registerExportedHassBridgeDevices`/`crossPostHassBridgeToCloud`
    (Conceptual_DeviceExportRegistration.go/discoveryhassbridge.go) already do for a hassbridge
    device -- not yet designed.
