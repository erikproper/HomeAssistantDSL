** TODO (as of 2026-08-28)

Current focus: 1.1 entity-existence inquiry & optimistic generation for the two HA incarnations
(main, protocols-server-2) -- core mechanism BUILT, DEPLOYED, and confirmed running live

1. Netatmo + other cloud services, via protocols-server-2 (first real MQTT bridging/federation
   test end-to-end) -- broken into concrete steps, roughly in this order though not strictly
   gating each other:

   1.1. Finish migration of netatmo
   - First trigger the discovery for the physical layer
   - Then update the space.def file ==> Can Claude help here?

   1.2. Netatmo temperature/weather data, flow to cloud MQTT, and test re-import "faking" Vienna import in Junglinster. Can't do this in vienna yet due to crashed systems in Vienna. homeassistant@protocols-server-2 -> MQTT.
   Note: rename local keyword to "native", as "local" is also used to speak about the local MQTT broker. And then use "export" as additional keyword to signify the export of devices, and "import" as integration.
   
   1.2b Also: discovery based integration ... also a suggestion file for these. In a way, the HA version of this is a "weaker" version, as HA needs to use the 3-value logic, where the discovery based ones are more explicit ... present or not present.
   
   1.3. Deprecate the use of "entity device.infrastructural:ring-camera from host.ring-camera with: all entities;" by doing a one time replace.
      Note (2026-08-27): mqtt does not have a temperature sensor, so don't include one for that
      device when doing the replacement.

   1.4. Deploy-trigger reload mechanism: a `meta/reload/<installation>/request` topic the deploy
      script posts to (cloud or local, a command-line choice, with broker secrets looked up from
      the def files), which the coordinator relays cloud->local when it arrives via the cloud
      broker for its own installation, and each HA instance's bootstrap automation reacts to by
      reloading its own YAML. Spun out of the (now-done) entity-manifests item; deferred until
      now, no longer blocked on anything.
   
   1.5. Try to get Volvo stuff back on-line.
   
   1.6. Overkiz-based SOMFY cover control: homeassistant@protocols-server-2 -> MQTT.
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

   1.7. Netatmo flow to Vienna
   Blocked
      specifically: a hardware (bridge/router) issue at the Vienna site (noted 2026-08-23) blocks
      testing this data path to Vienna, independent of the cloud-broker work above.

   1.8. Kind-2 (discovery) passive existence tracking -- closes the scope gap 1.1's own
      "Scope confirmation" note got wrong (2026-08-29, memory: project_entity_existence_inquiry_design).
      Kind-2 gateways (Zigbee2MQTT, Z-Wave, EMS-ESP) don't need active inquiry the way kind-3
      (`home_assistant` bridge) does -- a gateway self-announces via its own native HA MQTT
      discovery, unlike a remote HA instance, which never tells the coordinator anything
      unprompted -- but the coordinator still needs to track and report each declared kind-2
      *source* entity's three-state status (known-to-exist / not-known-to-exist /
      known-not-to-exist) to the generator, exactly as it already does for kind-3. Today the
      generator has zero existence signal for kind-2 sources at all -- not even kind-3's old bare
      assumption, a complete blind spot. See Architecture.md §6.10 for the corrected, unified
      framing (existence checks always anchor on the physical layer's own declared *source*
      specification -- a capability line's right-hand side -- never the conceptual-layer name).

      **Status (2026-08-29): first-phase built.** Coordinator side (`discovery_existence.go`, new
      file): `TDiscoveryExistenceTracker`, keyed by `(gatewayID, leaf)`
      (`TDiscoveryEntityLink.Leaf` -- the same field `discoverybridge.go`'s
      `matchingGateway`/`buildRelayedDiscoveryConfig` already match against), `Seed` reads
      `discovery.yaml`'s `EntityLinks` as not-known-to-exist, `MarkKnown` is called directly from
      `discoverybridge.go`'s existing discovery-payload subscription handler (no new subscription)
      the moment any payload for a leaf arrives -- whether or not an `EntityLink` claims it yet --
      `publishStatus` relays to `discovery_gateways/<gatewayID>/existence/state` (retained, local +
      cloud unconditionally, mirroring kind-3's own policy), persisted to
      `discovery_existence.json` across restarts (mirroring `entity_existence.json`'s pattern).
      Generator side (`mqtt_discovery_existence.go`, new file): `fetchDiscoveryExistence`
      (fetch-then-cache-fallback, mirroring `fetchEntityExistence`) and
      `checkDiscoveryKnownNotToExistErrors` (`checkKnownNotToExistErrors`'s kind-2 counterpart),
      wired into `Physical_Generator.go` right after the kind-3 check, gated the same way
      (`hasMQTTSecrets`). Unit-tested both sides; confirmed via a real `./generate` against
      Junglinster: soft-fails gracefully offline (no broker reachable, no local cache yet) exactly
      like kind-3's own behaviour, generation still succeeds. **Deployed live and confirmed clean**
      (no errors on deploy/generate) -- not yet exercised against a real gateway discovery payload
      though (no live `[discovery-existence] ... known-to-exist` log line seen yet).

      **Retraction (known-not-to-exist) -- [DONE, 2026-08-29, same day].** The first draft of this
      item wrongly deferred this, claiming it needed a topic→identity map the coordinator "doesn't
      keep and can't build cheaply." Corrected same day: the discovery-payload handler already has
      both the topic and the decoded `(gatewayID, leaf)` together at the moment any real payload
      arrives, so remembering `topic -> (gatewayID, leaf)` costs nothing
      (`TDiscoveryExistenceTracker.RecordTopicIdentity`) -- no new subscription, and no persistence
      needed either, since discovery config topics are retained: a coordinator restart's own
      subscribe naturally replays every gateway's current config before any new retraction could
      arrive, so the map self-heals in memory alone. An empty payload on a topic (HA's own MQTT
      discovery removal convention) resolves via that map (`MarkRetracted`) and moves the leaf to
      known-not-to-exist; `checkDiscoveryKnownNotToExistErrors` needed no change at all, it was
      already written expecting all three states. Unit-tested
      (`TestDiscoveryExistenceTrackerMarkRetractedResolvesViaRecordedTopicIdentity`,
      `TestSubscribeDiscoveryBridgeRetractsOnEmptyPayload`); not yet deployed/exercised live.

2. Control of picture frame via MQTT (commandline integration) --
   design/syntax not yet worked out.
3. MQTT (local) broker in container on p-s-1
4. Zigbee2MQTT (legacy-to-conceptual passthrough, so entities migrate gradually).
5. Z-Wave (migrated last -- richest device/capability modelling).
6. Phase 4 (retire superseded generator logic) + Phase 5 (publish) -- after all of the above.

Smaller pending items, not gating the above:
- Logical-layer device combining ("aggregate" a new device vs "absorb" into an existing master
  device, e.g. smarty's Zigbee switch into host.smarty) -- gated on steps 3 and 4 both landing.
- Raw/extensional Spaces.def entity references ("domain.[raw-id]") should become unnecessary
  once a device has a `device.<spec> from <device-id>` link instead -- audit once the MQTT
  migration is fully complete. (Step 1.2's sun.sun work is the first concrete case.)
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
     integration-adapter role). First real test case for MQTT bridging/federation
     end-to-end (see Architecture.md §6.5). Current focus (2026-08-23) -- see the "** TODO"
     list at the top of this file for the detailed, current sub-step breakdown (weather
     forecast, presence/reload via the coordinator, sun.sun, IPP printer, Enphase Envoy,
     Netatmo, Overkiz/SOMFY). Milestone check: Bridges.def should be effectively empty once
     this step is done -- the only bridge type currently in use is the "rest" bridge
     (confirmed: 0 uses in Junglinster's Entities.def, 29 in Vienna's, all of them Netatmo
     `imported rest` declarations), so nothing else currently depends on that mechanism.
     Bridges.def's content, and the ${junglinster_instance} variable it needs
     (Vienna/Definitions/Settings.def, formerly in the now-removed Server.def), are accepted
     as-is until this step removes the need for the REST cross-instance bridge entirely --
     both can then be deleted together.
     Also: once this step gives Physical.def a way to declare a device against a named,
     non-main HA instance (protocols-server-2 -- see the commented-out
     "home_assistant: protocols-server-2 ${protocols_server_2_home_assistant_url};" line
     already sitting in Junglinster's Physical.def, the anticipated syntax), the presence
     checker's assumed-entity check needs to become per-instance too -- see the "** TODO"
     list's item 1.2 for the current (2026-08-23) coordinator-based design superseding the
     old compile-time REST check idea sketched here previously.

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

---

Pending live test, once we're done building for now (2026-08-29):
- PROJECT.md 1.8's retraction (known-not-to-exist) half -- code-complete, unit-tested, not yet
  exercised live. Doesn't need a real gateway event: publish a dummy discovery config (matching a
  declared gateway's identifiers) to the physical-discovery prefix, confirm it's picked up
  (known-to-exist), then publish an empty retained payload to that same topic and confirm a
  "[discovery-existence] ... known-not-to-exist (retracted)" log line plus an updated
  discovery_gateways/<gatewayID>/existence/state publish.
