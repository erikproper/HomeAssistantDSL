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

25b. Test if the meta call to restart HA's works.

25c. Gateway liveness beyond the Last Will (Zigbee2MQTT first). Covered: every Zigbee2MQTT
     entity's availability includes zigbee2mqtt/bridge/state (all 162 in Vienna, 322 in
     Junglinster, checked 2026-10-30), which the Last Will sets offline when it stops; the retained
     shadows (item 20) keep the values meanwhile.
     - Test (Erik, Sunday 2026-11-01): briefly stop one Zigbee2MQTT; all its entities should go
       unavailable and come back with their values.
     - Hangs (connected, but no Last Will): design (Erik, 2026-10-30), generic, no bridge-specific
       code in the coordinator:
       1. For a declared bundle of topics, the coordinator creates a "suspicion_of_hang"
          binary_sensor (or trigger): true when none of those topics had traffic for a given time.
       2. Rules at the logical level, run on main: on a suspicion, do a ping-pong-like health check
          (for Zigbee2MQTT: zigbee2mqtt/bridge/request/health_check, answer on
          bridge/response/health_check; for Z-Wave JS UI: its MQTT API, e.g. getInfo on
          zwave/_CLIENTS/ZWAVE_GATEWAY-zwave-js-ui/api/getInfo/set, answered on .../api/getInfo --
          to verify live).
       3. When the check concludes the bridge is down, the rules publish "offline" on the right
          topic (Zigbee2MQTT: bridge/state, so every entity goes unavailable). To work out: the
          bridge itself overwrites that topic when it recovers, which is wanted.
       Then ha2mqtt's coordinator ping-pong (instance_liveness.go) could become one instance of the
       same mechanism.
     - To reflect on first (Erik, 2026-10-30): one generic strategy for all liveness checks, rather
       than one mechanism per gateway. Today there are three shapes: a Last Will (Zigbee2MQTT,
       Z-Wave JS UI's client status, the coordinator itself), our own ping-pong (ha2mqtt,
       instance_liveness.go) and a request/response health check (Zigbee2MQTT's health_check,
       Z-Wave JS UI's MQTT API); plus the hosts' ping. A generic strategy would say per gateway, in
       the defs: which topics show its traffic, how to ask it (request topic, payload), what counts
       as an answer and within how long, and which topic carries the verdict -- with the
       coordinator only watching for silence and the rules doing the asking.

25d. Nuanced reporting of suggested entries, based on conceptual versus physical positioning.
So, what is available to be positioned as potential capabilities, and what is available to be positioned at the conceptual layer.

26. Logical layer: what is left (restructured 2026-10-30). Done and removed: availability along
    via_device/dependencies and the integration devices (item 25, Architecture.md §6.16); named
    triggers, now requests (item 49), and with them the raw protocol codes out of the rules and the
    Moes fix; the positioning-grammar redesign; the node/battery_alert rules; the "physical:" sphere
    out of Conceptual.def; space-level aggregation (collect, "no_collect"); "absorb from"; the
    shared-node workarounds (node.vienna_*, fritz_box); capability value mapping (the former item
    49's requests); command choreography for a switched light ("switched by").

    A. Availability
    1. via_device suggestions from shared availability topics, for gateways that report no
       via_device (Z-Wave JS UI, EMS-ESP): a comment ("# suggested via_device:
       integration.zwave2mqtt (shares zwave/driver/status)"), never a value; the value is captured
       or "via_device: <device> forced;". A device with no shared topic is reported (needs human
       input). With a via_device, the device follows its integration's node (item 25's mechanism);
       skipped where the gateway's discovery already carries it (Zigbee2MQTT's bridge/state).
    2. A node for every device: decided 2026-09-01 as a hard error, not built. First count the
       devices still without one (the Envoy's now have one), and decide whether only logical and
       exported devices need it.
    3. Constraint to keep: an exported device publishes exactly one node boolean; an importer sees
       only that and its own link to the cloud broker, never the exporter's chain.
    4. On demand (no case yet): "dependency on entity <id>;" and "dependency on availability of
       entity <id>;" for a raw entity on main (integration_logical_parser.go's header).

    B. Logical-layer constructs, not built
    1. complement:
         complement device DDD with:
           from DDDx:
             CCCx [as CCCy];
           end;
         end;
       (the usual abbreviations for one). The conceptual layer sees DDD extended with DDDx's
       capabilities (CCCy renames on a clash); the device map doesn't change -- an entity from CCCx
       still belongs to DDDx.
    2. aggregate: a new device combining several (absorb, into an existing device, exists).
    3. virtual device: only the header "virtual device DDD with:" was noted; to define.
    4. Device-type templates: a named template expanding into a whole device block (the "basic
       node" shape first); to scope: parameters, where declared, what else would use it.

    C. Open design question: four kinds of logical-layer work -- (1) clustering capabilities, (2)
       derived entities with timing (delay_on/delay_off), (3) derived entities from other devices,
       (4) abstracted entities covering a device's shortcomings (no state reports, multi-broadcast
       radio: FHEM, item 38). (1)-(3) exist. Whether (2) and (4) belong in the coordinator or in a
       separate operational component is undecided (memory:
       project_event_bus_operational_vs_coordinative_roles.md).

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

42. Complementary functionality for e.g. zwave2mqtt and zigbee2mqtt to add (with discovery) via a wrapper additional functionality that can only be done via a web UI. For instance, firmware updates, etc. Hmm. Maybe work with these projects to add more functionality via MQTT.

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
