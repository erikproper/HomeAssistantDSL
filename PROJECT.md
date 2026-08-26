** TODO (as of 2026-08-26)

Current focus: 1.3 mobile (laptop) devices reporting CPU data to the cloud broker -- the next
piece of plumbing now that 1.2 (cloud-based MQTT broker) is basically finished. See step 1.3
below for the staleness-based liveness design already sketched, plus a new note on cross-platform
TLS CA handling the report script will need. `home_assistant` bridge devices (Envoy etc., 1.1
below) can continue in parallel, not gated on this.

1. Netatmo + other cloud services, via protocols-server-2 (first real MQTT bridging/federation
   test end-to-end) -- broken into concrete steps, roughly in this order though not strictly
   gating each other:
   1.2. Cloud-based MQTT broker -- DONE (2026-08-26). Mosquitto installed on mqtt.erikproper.eu,
      TLS via Let's Encrypt (certbot standalone + a renewal deploy-hook copying certs to where
      Mosquitto can read them, working around the mosquitto user's default lack of access to
      /etc/letsencrypt/live), two auth classes (`coordinator`: broad `#` access; `client`: scoped
      to `hosts/<own-client-id>/#` via Mosquitto's `%c` ACL pattern-matching, so every leaf device
      can share one login without losing per-device topic isolation). Verified end-to-end (TLS
      handshake, auth, publish/subscribe, and per-client isolation both allowed and rejected
      cases) from the server itself, from xanadu (Junglinster), and from a MacBook. Design
      confirmed: coordinator-mediated relay (not native Mosquitto broker-to-broker bridging),
      since JSON payload reshaping will be needed at the relay point, which a dumb broker bridge
      can't do -- see [[project_roaming_devices_bridging_broker]] (memory) for the fuller
      rationale. Not yet done: the actual coordinator-side relay code, and wiring real
      house/laptop credentials into Settings.def.
   1.3. Mobile (My laptop) devices to report CPU data (and implied availability). Unlike a fixed
      host, a laptop has no reliable "going offline" hook (sleep, lid closed, network loss all
      look the same as just not reporting) -- so its "node" liveness can't be an explicit
      true/false report the way it is today. Instead it needs to be staleness-based: the
      coordinator tracks each such device's last-reported time and infers "offline" once nothing
      has arrived for e.g. 3x the expected update_interval, publishing the node state itself
      rather than relaying an explicit report -- a new coordinator responsibility (a per-device
      staleness watchdog), not just a report-script change.
      EP: The difference is that a device on the "home" network can be pinged. A laptop that is "on the road" cannot be pinged. For such devices, we need a "liveness" strategy. Potentially not only in such cases.
      Cross-platform TLS CA handling (noted 2026-08-26): `Integrations/cpu/report` runs
      cross-platform already but has only ever talked to a local, non-TLS house broker, so it has
      no CA-bundle logic at all today. Reporting to mqtt.erikproper.eu over TLS will need a
      per-OS CA path -- confirmed working around this manually during broker testing: Debian/
      Ubuntu has a standard bundle at /etc/ssl/certs/ca-certificates.crt, but macOS has no
      equivalent file and needs one exported from the system Keychain
      (`security find-certificate -a -p /System/Library/Keychains/SystemRootCertificates.keychain`)
      since Homebrew-installed CLI tools don't consult the Keychain automatically. The report
      script needs this same per-OS handling built in before it can report to the cloud broker.
   1.4. With this in place, we can also update the architecture with regards to the reporting by the coordinator of the available entities, and then syncing of these lists to the transformer's local cache, and then using this to give warnings of non existance, and possibly suggestions of "addable" entities in a copy-paste friendly format for the physical layer.
   1.5. Netatmo temperature/weather data, flow to cloud MQTT, and test re-import "faking" Vienna import in Junglinster. Can't do this in vienna yet due to crashed systems in Vienna. homeassistant@protocols-server-2 -> MQTT. 
   1.6. Volvo stuff back
   1.7. Overkiz-based SOMFY cover control: homeassistant@protocols-server-2 -> MQTT.
   1.8. Netatmo flow to Vienna
   Blocked
      specifically: a hardware (bridge/router) issue at the Vienna site (noted 2026-08-23) blocks
      testing this data path to Vienna, independent of the cloud-broker work above.
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
  migration is fully complete. (Step 1.3's sun.sun work is the first concrete case.)
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
  discoveryassumed.go/discoveryhassbridge.go) to `homeassistant.instances/...`, matching this
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
