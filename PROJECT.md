** TODO (as of 2026-09-02)

1. Control of picture frame via MQTT (commandline integration) --
   design finalized 2026-09-06 (all three open questions from the initial sketch resolved below).
   Build progress (2026-09-06): steps 1-5 of the build plan below are DONE and unit-tested --
   generator (integration_commandline_{storage,parser,generator}.go, writing commandline/<host>.yaml
   + commandline/secrets.<host> + coordinator/commandline.yaml), coordinator
   (house_event_bus_coordinator/discoverycommandline.go, publishing discovery for a node
   binary_sensor + switch/sensor/button entities, gated on commandline/<host>/node/state, wired into
   main.go), and the daemon itself (homeassistant/mqtt_commandline/, a new sibling Go package in
   this same module, built/deployed exactly like house_event_bus_coordinator/) plus its deploy
   scaffolding (SmartLiving/Integrations/mqtt_commandline/{mqtt_commandline.service,deploy.frame},
   mirroring Integrations/coordinator/'s own two files). Switch on/off wire values are "1"/"0",
   chosen to match check_slideshow's own already-deployed convention verbatim -- no existing script
   needs rewriting. Status/sensor values are polled/republished every 60s (a daemon-level choice,
   no interval was specified in the original design) in addition to right after every switch
   command. All three Go packages (homeassistant, house_event_bus_coordinator, mqtt_commandline)
   build/vet/test clean together from the one shared go.mod.
   DONE + VERIFIED LIVE (2026-09-06): deployed to frame (Vienna's own host.frame, already a
   "hosts"-kind cpu device too) -- Vienna/deploy.d/04_deploy.commandline added (pushes
   Integrations/mqtt_commandline/ + generated commandline/ to pi@frame, runs deploy.frame there);
   Vienna/Definitions/Physical.def's "integration commandline with: device host.frame frame with:
   switch.slideshow: ...; end; end;" declared, using check_slideshow/start_slideshow/stop_slideshow's
   own real /home/pi/bin/ paths (systemd services don't get an interactive shell's PATH). Both
   mqtt_commandline.service and coordinator (redeployed with the new commandline.yaml) are enabled
   and running on frame; the switch's discovery config, state, and node-liveness topics all confirmed
   correct via direct broker inspection. Not yet tested: actually toggling the switch (on/off) --
   deliberately left for the user to try via HA's own UI, since it visibly changes the real display.

   Two real bugs found and fixed during this rollout:
   - runScript originally treated a status script's non-zero exit as an execution failure and
     dropped its output -- but the already-deployed check_slideshow prints "1" AND exits 1 to
     signal "not running" (a predicate-script convention, not a failure). Fixed: a non-zero exit is
     no longer treated as an error; only a genuine failure to execute the shell at all is.
   - buildCommandlineDiscoveryConfigs originally gave its own liveness entity the SAME unique_id
     ("<deviceID>_node") a "hosts"-kind declaration of the same deviceID already uses for its own
     ping-based liveness -- host.frame has both (cpu + commandline), so the two integrations'
     periodic republishes kept clobbering each other's payload on one shared HA entity. Fixed: the
     commandline liveness entity now uses "<deviceID>_commandline_node", its own separate HA entity,
     while still sharing Device.Identifiers so both show up grouped under one device.

   Decisions (were open questions, now settled):
   - Liveness: service-wide MQTT LWT on commandline/<host>/node/state, NOT ping-style staleness.
     The new service holds its own persistent broker connection (unlike cpu/report's fire-and-exit
     cron shape), so a broker-native Last Will detects the SERVICE dying immediately -- crash,
     network partition, clean shutdown -- the moment its TCP connection drops. Ping-based staleness
     only tells you the HOST machine is reachable, which says nothing about whether this specific
     service is still alive if it crashed independently of its host. Architecturally this daemon is
     closer to the coordinator (its own persistent connection) than to cpu/report, so it gets the
     coordinator's own class of liveness mechanism, not hosts' ping-based one.
   - DSL keyword: "commandline" confirmed -- deliberately matches HA's own built-in "command_line"
     integration's name, no reason to diverge.
   - Implementation language: Go confirmed -- reuses paho.mqtt.golang plus the same systemd-service
     deploy pattern the coordinator already establishes, rather than working around bash's lack of
     a real persistent-subscribe primitive.

   Design recap (unchanged from the 2026-09-06 sketch):
   - A new, generic "mqtt_commandline" service -- a THIRD long-running Go daemon alongside the
     generator and coordinator -- deployed onto any host that needs local script-backed entities,
     reading a generator-authored YAML config describing three entity kinds:
        switch <name>: status_script, on_script, off_script
        sensor <name>: status_script
        button <name>: press_script
     Scripts are captured as one opaque quoted command-line string each (path + args), with the
     service doing ordinary shell-word-splitting at invocation time -- lets a single generic
     script be reused parametrically rather than needing one bespoke script per entity.
   - Core architectural principle (carried over from cpu/ping/hassbridge): the new service NEVER
     publishes HA MQTT discovery itself, only raw state + command subscriptions -- the coordinator
     stays the sole discovery author, same as every other integration kind. For switch/button, the
     discovery config's own command_topic can point DIRECTLY at the frame's own topic (HA publishes
     there straight from the UI) -- no coordinator-side command relay needed, exactly like a
     hosts-kind sensor's state_topic already points straight at the reporting host with no relay
     for reads either.
   - Wire shape (mirrors hosts/<host>/...): commandline/<host>/<entity>/state (retained,
     status_script's trimmed stdout); commandline/<host>/<entity>/set (switch command -- runs
     on_script/off_script, then immediately re-runs status_script and republishes state, same
     "state reflects reality, never assumed" principle used elsewhere); commandline/<host>/<entity>/press
     (button command, no state at all, matching MQTT button semantics).
   - Physical.def grammar, a fourth integration kind alongside hosts/home_assistant/discovery:
        integration commandline with:
          device host.frame frame with:
            switch.slideshow: "check_slideshow" "start_slideshow" "stop_slideshow";
            button.reboot:    "reboot_frame";
          end;
        end;

   Build plan (files, in dependency order -- mirrors integration_hassbridge_*.go's own
   file-per-concern split, chosen over extending integration_hosts_*.go: a commandline capability's
   source is a local SCRIPT, not another entity's own state, closer in shape to hassbridge's
   "bespoke per-kind data model" than to hosts' "reference another entity" one):
   1. homeassistant/integration_commandline_storage.go -- TCommandlineCapability{Kind (switch/
      sensor/button), StatusScript, OnScript, OffScript, PressScript string},
      TCommandlineDevice{DeviceID, Host string, Capabilities map[string]TCommandlineCapability}.
   2. homeassistant/integration_commandline_parser.go -- Physical.def "integration commandline
      with: device <id> <host> with: <kind>.<name>: <quoted-scripts>; ...; end; end;" grammar,
      mirroring integration_hassbridge_parser.go's own block-scanning shape.
   3. homeassistant/integration_commandline_generator.go -- two outputs: (a) the target host's own
      YAML entity config (script paths/args only, no MQTT-shape knowledge needed there since the
      new service only knows "run this on set/press, publish that on state"), deployed the same
      way cpu/secrets.* already is; (b) coordinator-consumable metadata -- own dedicated
      coordinator/commandline.yaml file (not folded into devices.yaml: switch/button's
      command_topic/payload_on/off/payload_press shape doesn't fit devices.yaml's existing
      read-only-sensor-plus-node model without distorting it).
   4. house_event_bus_coordinator/discoverycommandline.go -- loads commandline.yaml, builds
      discovery configs: reuses TSensorDiscoveryPayload/TBinarySensorDiscoveryPayload for the
      "sensor" kind (no change needed there), adds two genuinely new payload shapes for "switch"
      (command_topic, payload_on/off, state_on/off) and "button" (command_topic, payload_press, no
      state at all) -- the one real new piece of discovery.go work this item needs. Also
      subscribes to commandline/<host>/node/state (the new service's own LWT-backed liveness) as
      the availability gate for every entity on that host, same buildAvailabilityFields two-factor
      pattern already used elsewhere.
   5. Integrations/mqtt_commandline/ (new Go module, own go.mod like house_event_bus_coordinator/)
      -- the actual daemon: reads its YAML config, connects with an LWT set on
      commandline/<host>/node/state, publishes "true" once connected, subscribes to every declared
      switch's .../set and button's .../press topics, runs the corresponding script via
      os/exec with normal shell-word-splitting, and for switch, re-runs status_script and
      republishes .../state immediately after on/off completes. Deployed via a new deploy.d step
      (Integrations/mqtt_commandline/deploy.<host>, mirroring cpu's own deploy.junglinster/
      deploy.frame convention) + a systemd unit, since it needs to run continuously like the
      coordinator does, not fire-and-exit like cpu/report.

   Verification plan once built: unit tests for the parser/generator/discovery-payload-building
   (mirroring existing per-kind test coverage), then a real end-to-end rollout against frame.vienna
   specifically (the only host that needs this today) -- declare host.frame's three slideshow
   scripts in Vienna's Physical.def, generate, deploy the new daemon + its config to frame, confirm
   the resulting switch entity in Vienna's main HA correctly reflects check_slideshow's own state
   and correctly triggers start_slideshow/stop_slideshow on toggle, before considering this item
   done.

1b. Test in vienna first. Then also roll out to JL.

2. Existence checking again. Entities that are provided on the main instance. Do we check their existence as well? And make suggestions based on their device assignments?
Basically treat these as kind 3 ones, but always originating from the main HA instance.
Even though more and more entities will be pushed "under" the MQTT bus, we know that certain domains (media_player, weather, etc) cannot move there yet. So, we will need to rely on integrations that are directly linked to the conceptual layer within the main HA instance (like all entities used to be).

3. EP: Hardware migration in Vienna.
- Mo 1 HASS backup; copy backup to MacMini
- Mo 2 HASS on green
- Mo 3 Backup Samsung + photos/frame to AppleSSD on frame
- Tu 4 Pi4 as P-S-1 for Vienna 
    Restore from AppleSSD (also photos!)
- Tu 5 Pi3 as P-S-2 for Vienna
    Copy photos back to P-S-2
- We 6 Setup P-S-1 for Vienna:
    { zigbee, samba, mqtt, smtpproxy, ... }
- ?? 7 Setup P-S-2 for Vienna:
    { picture frame, HA,... }

4. EP: Fritz's have CPU temperature, plus other things + fix compute tabs + check suggestions.
Vienna: Open. Blocked until we have the new hardware deployment

5. EP to rename device names, hiding the integration part of the name.
Also revisit the names of integrations.

6. 
  device infrastructural:eriks-macbook-pro-2 from import.eriks-macbook-pro-2 with:
    entity sensor.infrastructural:eriks-macbook-pro-2/cpu/load        from entity cpu/load;
    entity sensor.infrastructural:eriks-macbook-pro-2/cpu/temperature from entity cpu/temperature;
  end;

to be:

  device infrastructural:eriks-macbook-pro-2 from import.eriks-macbook-pro-2 with:
    entity sensor.infrastructural:eriks-macbook-pro-2/cpu/load        from sensor.cpu/load;
    entity sensor.infrastructural:eriks-macbook-pro-2/cpu/temperature from sensor.cpu/temperature;
  end;

where the second sensor should match the original type as provided on the MQTT bus.

So, we could have:
  entity binary_sensor.XXX from switch.YYY;
  entity binary_sensor.ZZZ from sensor.KKK with condition "$ | float > NNN"
making the local "coercion" explicit.

7. Also ... make the "device" declaration act like a space.
So:
      device infrastructural:netatmo from hass.living_room_shower_room with:
        entity sensor.physical:netatmo/co2                  from entity sensor.co2;
        entity sensor.physical:netatmo/humidity             from entity sensor.humidity;
        entity sensor.physical:netatmo/temperature          from entity sensor.temperature;
        entity sensor.infrastructural:netatmo/battery_level from entity sensor.battery_level;
      end;

should become:

      device infrastructural:netatmo from hass.living_room_shower_room with:
        entity sensor.physical:co2                  from entity sensor.co2;
        entity sensor.physical:humidity             from entity sensor.humidity;
        entity sensor.physical:temperature          from entity sensor.temperature;
        entity sensor.infrastructural:battery_level from entity sensor.battery_level;
      end;


8. MQTT (local) broker in container on p-s-1 @JL and frame @VIE

9. Zigbee2MQTT (legacy-to-conceptual passthrough, so entities migrate gradually) first @VIE 

10. Also make the adjustments and battery_alert values something for the device level. Check of discovery messages can handle such functions. 

11. Z-Wave (migrated last -- richest device/capability modelling).
Well. Potentially we can do this one after Zigbee2MQTT @ Vie,
as it contains e.g. blinds as well.

12. Zigbee2MQTT (legacy-to-conceptual passthrough, so entities migrate gradually) first @JL

   1.8. Overkiz-based SOMFY cover control: homeassistant@protocols-server-2 -> MQTT.
      Note (2026-08-28): this is also where 1.1's command-automation half
      (command_<fully_qualified_entity_name>_<command>.yaml, deferred there -- see memory:
      project_entity_existence_inquiry_design) needs its open design question resolved: which
      commands a capability/domain supports, and how one maps to a remote service call on the
      bridged instance. Covers (open/close/stop) are the first real commandable domain due here;
      until then everything bridged is sensor-shaped (read-only). 1.5 (Volvo) may bring the first
      buttons/locks even earlier, if that lands first.
   
         - State reporting: one trigger/action pair per entity, one YAML file per entity --
        filename `reporting_<fully_qualified_entity_name>.yaml`, alias
        `reporting/<fully_qualified_entity_name_with_slashes>`.
      
      - Commands (e.g. light_on): one file per entity per command --
        filename `command_<fully_qualified_entity_name>_<command>.yaml`, alias
        `command/<fully_qualified_entity_name_with_slashes>/<command>`.

        Check if exports of commands would still work.

13. Ensure we have now all the integrations we need.
Then also re-enable the checking of locally (on main HA) assumed entities via the inquiry process. So, using the three values approach, triggering the inquiry of assumed to exist local entities (on main) via the coordinator. 

14. Logical layer + available nuances in relation to "via device"  (netatmo radio module via main module) and aggregation.
- A used or exported (logical or physical) device must always have a _node capability. When such a device has a _battery_level capability, then it must also have a _battery_alert capability. 
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
     Note (2026-09-06): see the "Logical-layer device combining" note below (smaller pending
     items) for a real case (hass.fritz_box/host.fritz_box) suggesting this hard-required-node
     decision may need revisiting once logical devices can "lend" a node capability from a
     sibling physical device -- possibly only logical devices (and exported devices) need the
     hard requirement, not every physical one.

15. Installation-level status binary_sensors (meta sphere): one (discovery-created) binary_sensor per HA
      instance signalling (1) a configuration problem on that instance, and (2) updates available
      for it -- purely passive/informational (dashboard-visible), deliberately decoupled from item
      2's reload/restart meta-command mechanism rather than gating it synchronously (see item 2's
      own 2026-09-05 scope decision). Not designed yet:
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

15b. Check aggregation of sensors. If one is down, what do we do with data?

16. Check old todo/plan part below the "--------" below

17. Code cleaning (dead code, superseded generator logic)

18. Architectural review, code review and documenting
   - Note (2026-09-05): GO_CONVENTIONS.md §7/§8 specifies a three-layer parser architecture
     (character stream -> tokeniser -> recursive-descent, CDL1 bool-returning style, no `error`
     returns from parse functions). The DSL frontend (Physical.def/Spaces.def parsing) does not
     follow this -- it's line-based `regexp.MustCompile`/`FindStringSubmatch` matching throughout,
     confirmed spanning at least 15 non-test files including the generic layer (`Physical_Parser.go`,
     `layers.go`, `expander.go`), not just the per-integration parsers. Flagged by the user ahead of
     the code-review step above; expected to be remedied, but deliberately NOT attempted piecemeal
     inside unrelated feature work (e.g. the 2026-09-05 import-grammar unification, item 4a below,
     stayed in the existing regexp style rather than migrating just the one file it touched) --
     needs its own dedicated pass given the scope (the whole DSL frontend, not one file).

18b: Test if the meta call to reset works.

19. FHEM/FS20 integration

20. Publish

21. CHeck for more of the existing integrations, like fritzbox, ems-esp, etc.

---------------
Smaller pending items, not gating the above:
- Logical-layer device combining ("aggregate" a new device vs "absorb" into an existing master
  device, e.g. smarty's Zigbee switch into host.smarty) -- gated on steps 3 and 4 both landing.
- Logical-layer availability composition, noted 2026-08-31 once the physical-layer coordinator
  work landed a device's node-topic-gated + self-check availability (`buildAvailabilityFields`,
  discovery.go): that two-factor AND is the right amount of refinement for a single physical-layer
  device, but "aggregate"/"absorb" combine devices whose availability chains can be deeper and
  don't compose by simple ANDing alone. Two concrete cases to design against once step 6 starts:
  (a) a compute host running an integration (e.g. protocols-server-2) has its OWN liveness,
  independent of any device it reports on -- an entity can be "available" per its own device/node
  signals while the host reporting it is itself down; this is the still-open "stacked/dependent
  availability" reminder already flagged in splendid-zooming-lighthouse.md's out-of-scope section.
  (b) Netatmo's IP-connected base modules add a layer of their own: an outdoor/rain/wind module's
  true availability depends on ITS radio link to the base module AND the base module's own IP/WiFi
  link to Netatmo's cloud AND (a) above -- a chain, not a flat set, and today's per-entity "node"
  capability only captures the nearest link in it. Not designed; revisit once step 6 (Logical
  layer) is actually being built, alongside aggregate/absorb themselves.
  Note (2026-09-05), from the cross-house import "node" liveness fix (item 4a): whatever
  availability chain the EXPORTING installation ends up composing internally for a device (its own
  liveness, a base module's link, a radio link, etc.) must stay entirely its own concern -- the
  cloud catalogue it publishes should keep exposing exactly one boolean per device (today's "node"
  capability), never the individual links that fed it. From an IMPORTER's own perspective there are
  only ever two independent availability dependencies to reason about: (1) the remote device's own
  reported available/not-available state (whatever composed that, on the exporter's side), and (2)
  this house's own connection to the cloud broker. Neither should ever surface any of the
  exporting installation's own internal dependency structure into the importer's device map --
  revisit this constraint explicitly once the composed-availability design above is actually built,
  to make sure the exporter-side composition still collapses to one clean signal before crossing
  the cloud-broker boundary.
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
  carry its own -- directly relevant to item 14's own "should every device be hard-required to
  have a node capability" question (revisit that decision once lending exists; it may turn out
  only *logical* devices need the hard requirement, not every physical one). Also matters for
  `export`: an exported device (physical or logical) needs exactly one node capability for the
  cloud catalogue's "one boolean per device" contract to hold, but a physical device on its own
  may have none and would need to borrow one from a sibling the same way -- e.g. a `hosts`-kind
  ping device lending its liveness to a `home_assistant`-bridge device representing the same
  physical unit.
- DONE (2026-09-01): the raw/extensional "domain.[raw-id]" Spaces.def entity-reference syntax is
  removed -- no real Spaces.def/Physical.def in either house still used it (confirmed by audit),
  so `TEntityIdentity.IsRaw`/`RawName`, the bracket-detection branch in `extractEntityIdentity`
  (expander.go), the bracket-passthrough regex in `toHomeAssistantEntityID` (defined.go), and the
  three generator.go/administration.go/would_define.go call sites that special-cased it were all
  deleted outright, not just deprecated. `sun/solar_elevation` (still declared bare in both
  houses' Spaces.def, sourced from a hand-authored HA template outside generator control) remains
  the one real case that would need this syntax if the DSL ever took it over -- unaddressed, not
  gating anything.
- Future refinement (2026-08-23, not yet designed): replace the `.def` files with an integrated
  database representing the coordinator+transformer's actual current understanding of the
  conceptual and physical layers -- physical-layer content populated/kept live from what the
  integrations themselves discover (MQTT discovery, device_attr(), live-reported metadata),
  rather than hand-declared and re-generated from static text each `./configure` run -- while
  still allowing reconfiguration of key components (e.g. which `home_assistant <qualifier>`
  instances exist) through it. Large, cross-cutting shift from the current
  Definitions-directory-as-source-of-truth model; needs its own design pass once the current MQTT
  migration (steps 1-4 above) has landed and the shape of "what the coordinator already knows
  live" is clearer. Reinforced by 2026-08-24's protocols-server-2 rollout: compile-time
  (generator-authored automations, `.def`-driven expected-entity lists) and run-time (coordinator's
  live MQTT state) are still two separate worlds today, stitched together only by redeploys and
  manual "reload YAML" steps on each HA instance -- a merged compile-time/run-time coordinator,
  backed by that same database and possibly its own web UI, would remove that whole class of
  "did you redeploy/reload yet" friction rather than just papering over it with better tooling.
- Rename the `homeassistant_instances/...` MQTT topic tree (bootstrap/bridge protocol,
  discoveryhassbridge.go) to `homeassistant.instances/...`, matching this
  project's own dot-separated discovery-prefix convention (`homeassistant.physical`/
  `homeassistant.conceptual`) -- purely a naming inconsistency introduced 2026-08-23, not yet
  fixed. Safe/mechanical now that both the coordinator code and every instance's automations are
  generator-owned; just needs a coordinated regenerate+redeploy across every instance at once.
- Volvo integration's cloud auth is broken on the "main" (Junglinster) HA incarnation (401
  Unauthorized, confirmed 2026-08-24 via the assumed-entities warning system flagging 18 missing
  `sensor.social_cars_xc40_*`/etc. entities) -- needs re-authenticating in HA's own UI; not a code
  issue.
- Deploy-script lessons from protocols-server-2's 2026-08-23/24 rollout, worth remembering before
  Vienna gets its own deploy.d scripts: (1) a symlink created via `ssh ... ln -sf <target> <dir>/`
  must use a target relative to the link's own location, not the host's absolute path -- the link
  is later read from *inside* the HA container, where the host's absolute path doesn't exist, and
  HA's own error reporting mislabels the resulting failure as "File not found:
  /config/configuration.yaml" regardless of which file actually broke, which cost a long debugging
  session to trace. (2) current Home Assistant's `mqtt.publish` service no longer accepts
  `payload_template` (schema-validation error "extra keys not allowed") -- template the `payload:`
  field directly instead; both are now fixed in `homeassistant/remote_instance_automations.go` and
  `deploy.d/0{1,2}_deploy.hass.*`.
- `isCoordinatorOwnedTopic`'s legacy `host_`/`discovery_` topic-shape branch
  (house_event_bus_coordinator/discoverycleanup.go) exists only to catch pre-migration discovery
  topics during the one-time cutover to identity-based topic keys (`.../coordinator/<stable-id>/config`).
  Once each deployed house's coordinator has run once and its retirement log confirms the old
  topics are gone -- Junglinster first, Vienna once it's actually deployed -- remove that branch;
  it'll be permanently dead code by then.

---

See below for the full narrative behind each TODO item above; Architecture.md for the target
design (canonical event bus, coordinator, three modelling layers); README.md for current
DSL/usage reference. As each TODO item is finished, its write-up below should be trimmed down to
a one-line DONE note (or removed, if nothing durable needs keeping beyond what's already in
code/README.md/Architecture.md) -- the goal is for this section to end up essentially empty once
Phase 3 is done, and for the TODO list above to stay the only thing anyone needs to read at a
glance.

** Roadmap detail

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
  blocking gate: each integration tackled in Phase 3 (ping/cpu, Netatmo, EMS, Zigbee2MQTT, Z-Wave) surfaces and stabilises its own corner of the existing
  DSL/generator as it's iterated on, so the two phases run together per-integration
  from here on.

Phase 3 — New architecture, incremental learning steps
  Each step tests one new architectural capability before the next is attempted, and
  grows the coordinator's aggregation vocabulary gradually rather than all at once.

  Timeline note (2026-08-18): travelling over the next few weeks, so the bridge role
  (federation across MQTT brokers, §5/§6.5 in Architecture.md) will not be touched
  until September. Physical-level DSL preparation therefore focuses first and only on
  step 0 (ping + cpu) below, since that step needs no cross-broker bridging at all.
  Update (2026-08-22): step 0 is done, delivered while still travelling -- confirms the
  no-cross-broker-bridging framing held.
  Update (2026-08-23): steps 1 and 2 swapped -- protocols-server-2/Netatmo work now takes
  priority over the EMS heater (see the "** TODO" list at the top of this file for the current,
  detailed breakdown). Current focus is step 1's first sub-step, weather forecast bridging.

  0. Ping + CPU integration. DONE (2026-08-22). Treated as one conceptual integration
     with two data-collection scripts (a pinger; an OS-dependent CPU load/temperature
     collector). Every cpu device implies a corresponding ping device. The actual DSL
     shape that shipped differs from the original sketch below (an "integration <name>
     [on <host>] with devices: ..." / Integrations.def syntax was never built) -- what
     exists instead, and is now documented in README.md's "Devices (the hosts
     integration)" and "Lists.def syntax" sections:
     - Physical.def's "integration hosts with: device <id> <host> <type> with: ...;
       end; end;" declares each host's type (cpu/home_assistant/ping), capability
       entity mappings (optionally grouped, e.g. "cpu/load: sensor.processor_use;"),
       and constant device-map overrides ("model: \"...\" [forced];").
     - Spaces.def's "entity device.<spec> from <device-id> with:
       variable_device_attributes;" positions a device in the conceptual space tree,
       implying discovery-implied node/attribute entities -- created by the
       coordinator's own HA MQTT Discovery publish, not generator-authored YAML.
     - "space <spec> as area with:" marks a space as an HA Area feeding suggested_area.
     - house_event_bus_coordinator/ is no longer a stub: connects to the MQTT broker,
       publishes discovery configs (retained), subscribes to live-reported device
       metadata, merges it with Physical.def's static/forced constants (forced-DSL >
       live > default-DSL precedence), and republishes a device's discovery config
       live when new data arrives -- confirmed working end to end. Deployed as a
       systemd service on protocols-server-1 (Junglinster), staged through xanadu.
     - Integrations/cpu/report reports CPU load normalized to a percentage of core
       count (not raw Unix load average) plus device metadata, cross-platform.
     - Space-level aggregates (temperature, humidity, ...) and generator-authored
       customization files both exclude infrastructural-sphere / discovery-implied
       entities, so a device's cpu/temperature never gets folded into the room it
       physically sits in, and never gets a redundant customization file duplicating
       what the coordinator's own discovery payload already sets.

     Still open, deferred to step 1 (Netatmo) or later: per-host metadata aggregation
     with other integrations at the same physical location (e.g. combining a netatmo
     device's readings with the host device it shares a room with) would need a
     "complement" operator, not just aggregation -- not designed yet.

  1. Netatmo and other cloud services, via the secondary HA instance (protocols-server-2,
     integration-adapter role) -- DONE, see the "** TODO" list's 1.2a-1.2d above for the
     actual mechanism (MQTT-based hassbridge export/import, not the REST bridge originally
     sketched here). 1.3a (below) removed the REST bridge mechanism entirely, now that
     nothing depends on it: `Bridges.def` and its `${junglinster_instance}`/
     `${junglinster_api_token}` Settings.def variables deleted from Vienna's real
     Definitions/ (2026-08-31), the only house that ever used them.

  2. EMS heater device + control of picture frame via MQTT (as a commandline
     integration). Not started -- design/syntax not yet worked out.

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
