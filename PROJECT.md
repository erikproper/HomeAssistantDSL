** TODO 


1 Adding:
- complete candy; ICONS?
- complete vacuum
- check suggested in JLI
- check suggested in VIE
- dsmr-gateway
- envoy-wifi vs envoy
- Hosts:
  - espressif = candy
  - irobot
- fritz box @ Vienna (temp, ++)
      entity sensor.infrastructural:fritz_box_7590_ax_gb_received with call providing :fritz_box;
      entity sensor.infrastructural:fritz_box_7590_ax_gb_sent;
- fritz box @ Junglinster (temp, ++)
- sun via ps2
- roomba via ps2
- Migrate terrace + rack sensors.


2. Stick migration for Pi3 and PiB:
- Copy the pi3 stick in vienna to one of these sticks
- Migrate frame.junglinster to one of these sticks on fedora
  - Note (2026-09-08, from Vienna's protocols-server-2 Fedora migration): a `dnf -y upgrade` right
    after installing X/feh bumped mesa-dri-drivers (26.0.3 -> 26.1.8) and broke the modesetting
    driver's GLAMOR/GPU acceleration on the Pi's SoC GPU -- Xorg segfaulted inside glamoregl right
    after loading it (Xorg.0.log: crash immediately follows loading libglamoregl.so, modeset(0) on
    /dev/dri/cardN). Fix: `/etc/X11/xorg.conf.d/10-modesetting-noaccel.conf` with a `Device`
    section (`Driver "modesetting"`, `Option "AccelMethod" "none"`) forcing software/shadow
    framebuffer rendering -- fine for a static picture-frame slideshow, no GPU needed. Apply this
    same file proactively on frame.junglinster's Fedora install, ideally before the first
    `start_slideshow` test rather than after hitting the same crash again.
  - Note (2026-09-10, item 0): fold the swap/cache fix in here too, rather than doing it
    standalone -- fresh-install-on-USB-then-copy-containers-across is exactly the right moment to
    set up proper disk-backed swap from day one, instead of retrofitting it live like Vienna
    needed. Specifically follow the Vienna protocols-server-2 model (same class of hardware/RAM as
    frame.junglinster, not Junglinster's own existing protocols-server-2, which is a different,
    better-resourced machine): a 4GB disk-backed /swapfile at priority -1, alongside zram at its
    default priority 100 (zram absorbs load first, the swapfile is overflow) -- exact commands in
    memory: project_ps2_swap_fix_procedure.md.

3. NOT FULLY DONE (checked 2026-09-14 -- eriks-mac-mini's own dangling-job check, below, is still
   outstanding). Otherwise done as of 2026-09-11, and the outcome reverses this item's own
   original premise -- kept the
   generic cpu-report-cloud-client LaunchAgent for eriks-mac-studio rather than switching it to the
   per-installation com.erikproper.cpu-report-junglinster.plist (local-broker) one.
   Real macOS-specific obstacle found live: switching to the local-broker plist worked for exactly
   the first one or two runs (confirmed via `log show`'s kernel/NECP trace -- real TCP connections
   completing to the local broker), then macOS's Local Network privacy permission subsystem started
   silently dropping every subsequent connection ("tcp drop outgoing ... reason: NECP") -- a bare
   CLI binary (mosquitto_pub) launched via a LaunchAgent has no real app-bundle identity for macOS
   to associate a granted permission with, so it never gets an interactive consent prompt and
   defaults to deny. The OLD cloud-client job never hit this because it talks to an EXTERNAL host
   (mqtt.erikproper.eu) -- Local Network permission only gates LAN/RFC1918 destinations, which the
   local-broker plist's "junglinster" target (a bare local hostname) is.
   Decision (the user, 2026-09-11): ALL macOS-based hosts should report to the cloud directly, not
   through a house's own local broker.
   - eriks-mac-studio: reverted to cloud-client, confirmed live (real load/temperature/device-info
     data flowing through Junglinster's coordinator, both its local "main" relay and its
     cloud-broker cross-post).
   - eriks-macbook-pro-2: found running BOTH jobs at once -- the 2026-09-08 local-delivery
     migration had left a genuinely working "com.erikproper.cpu-report" (local-broker) job loaded
     (confirmed live data), even though its own LaunchAgents plist file had since been removed from
     disk (a dangling loaded job, config gone but launchd still running it -- the earlier migration
     never did its own `launchctl bootout` step). `launchctl bootout` on the dangling job, confirmed
     `com.erikproper.cpu-report-cloud-client` is now the only one loaded; confirmed live via the
     coordinator log that traffic switched from "via_device": "junglinster" to
     "via_device": "mqtt.erikproper.eu" within the same minute.
   - eriks-mac-mini: per the user (2026-09-11), already reporting to the cloud correctly -- the
     earlier "no retained data" finding here was just the machine being off, not a real problem
     (an earlier note wrongly speculated it might be silently permission-broken like studio was;
     wrong guess, corrected). Still worth checking for eriks-mac-mini's OWN "double reporting"
     issue [corrected 2026-09-14 -- this previously (wrongly) said "eriks-macbook-pro-2's own",
     but macbook-pro-2's own dangling job was already found+fixed two bullets up, in this same
     item; the still-open check has always been about mac-mini] -- a dangling local-broker job
     left loaded (config file already gone) from the same 2026-09-08 migration that never called
     its own `launchctl bootout` -- queued for whenever it's next reachable: `launchctl list | grep
     cpu-report` should show ONLY `com.erikproper.cpu-report-cloud-client`, `bootout` anything else
     found loaded.
     [CHECKED 2026-09-14: re-verified macbook-pro-2 itself (`launchctl list | grep cpu-report`)
     still shows only `com.erikproper.cpu-report-cloud-client` loaded -- that part remains fine.
     mac-mini's own check is STILL not done -- attempted from this session but mac-mini isn't
     reachable on this network (no SSH config entry, `eriks-mac-mini.local` doesn't resolve here).
     This item can't be marked fully DONE until that check actually happens.]
   Unrelated, noticed but not investigated: eriks-mac-studio's own reported load values are
   implausible (300%+) -- likely a macOS `GetLoad`/`sysctl vm.loadavg` parsing issue on this
   specific host, or a genuine load spike; not touched.

4. Zigbee2MQTT (legacy-to-conceptual MQTT passthrough, so entities migrate gradually) migration of devices in Vienna
Passthrough is needed because we will need to shift the discovery topic of Z2M from homeassistant to homeassistant.physical,
and relay the not-migrated ones to the homeassistant topic. 
To this end, the coordinator would need to:
- pass on any config entry with a topic starting a state or command topic starting with "zigbee2mqtt/"
- unless, the config entry pertains to an already migrated device (which would be declared in the physical.def file
  in the discovery integration).
As this migration capability should be a generic one, for given "topic prefixes", we should allow for (in the physical layer) 
an entry:
   discovery_passthrough "zigbee2mqtt/" from "homeassistant.physical";

   [Coordinator mechanism DONE + tested 2026-09-14, migration itself still to do: the bare
   top-level statement above (mirroring mqtt_discovery_clean's own "generic, repeatable
   mechanism" precedent, homeassistant/mqtt_discovery_cleanup.go) is parsed by the new
   homeassistant/discovery_passthrough.go, writing coordinator/discovery_passthrough.yaml.
   generateDiscoveryPrefixBaselineFile (integration_discovery_generator.go) now also writes
   physical_prefix to discovery.yaml unconditionally whenever ${mqtt_discovery_physical} is set --
   needed since passthrough must work even for a house with zero "integration discovery with:
   device ..." blocks declared yet, which generateDiscoveryIntegrationOutputs' own
   gateway-triggered write never covered. house_event_bus_coordinator/discoverybridge.go's
   existing subscribeDiscoveryBridge (already subscribed to <physical_prefix>/+/+/+/config for the
   EMS-ESP-style gateway relay) now also: relays a message byte-for-byte onto conceptualPrefix
   (same topic tail, just the prefix segment swapped) whenever its device matches no declared
   gateway but its state_topic/command_topic matches a declared passthrough prefix; retires that
   raw copy (TDiscoveryPublisher.RetireOne, guarded by a new Publisher.Knows check so it's only
   attempted for a topic actually relayed before) the moment the SAME device starts matching a
   declared gateway, so a migrated device never shows twice; and relays upstream retraction (an
   empty payload) through too, so a Zigbee2MQTT device being fully removed cleans up HA's copy as
   well. Fully unit-tested (9 new tests: generator-side parse/dedupe/sort/emit,
   coordinator-side pass-through/no-match/already-migrated/retire-on-migrate/
   retire-on-retraction/command_topic-matching). NOT yet deployed anywhere live -- no house has
   Zigbee2MQTT reconfigured to publish under ${mqtt_discovery_physical} yet, so there's nothing to
   verify against real traffic. Still open: actually reconfiguring Zigbee2MQTT's own MQTT discovery
   prefix (outside this repo), adding "discovery_passthrough ...;" to Vienna's Physical.def, and
   the device-by-device migration itself (declaring each device as it moves, per this item's own
   title).]

5. MQTT (local) broker in container on p-s-1 @JLI

6. Z-Wave migration from winsock to MQTT. Also on a device per device base, but no MQTT passthrough needed.
   Pre-work notes from the original roadmap: new environment is Home Assistant Green + a
   Raspberry Pi running Podman + Z-Wave JS UI + Aeotec ZWA-2 controller + two Aeotec ZW117 range
   extenders; mesh validation confirmed a Fibaro module excluded/included successfully through
   Basement (ZWA-2) -> 1st floor repeater -> 2nd floor repeater -> device; Node ID allocation
   observed monotonically increasing (first inclusion -> Node ID 4, second -> Node ID 5) rather
   than immediate reuse of released IDs, not fully validated -- migration plan deliberately avoids
   depending on Node ID preservation (possible allocation strategies to watch for: first unused
   ID, highest known ID + 1, highest ever assigned ID + 1); migration order keeps the old Gen5
   mesh operational while building an initial new backbone from the ZW117 repeaters, migrating
   remote mains-powered devices first, letting migrated Fibaro modules become new repeaters
   (strengthening the mesh as migration proceeds), migrating controller-near devices last -- keeps
   RF coverage acceptable throughout and doesn't require every device to be physically reachable
   from the new controller during migration.

7. Migration of Zigbee2MQTT app from Green to p-s-1 in JLI, plus antenna migration.

    Unlike Vienna's move (same physical Sonoff dongle, just relocated -- DONE + VERIFIED LIVE
    2026-09-08, real network/73 devices preserved intact), Junglinster is also swapping the radio
    hardware itself (Sonoff -> Nabu Casa Zigbee antenna), so the network's coordinator IEEE address
    changes unless explicitly transferred -- extra step Vienna's move didn't need. Checklist,
    following the same real procedure worked out live on Vienna:
    - [ ] On Green-equivalent (Junglinster main HA): Settings -> Add-ons -> Zigbee2MQTT -> Settings
          -> Tools -> "Request Z2M backup" -- downloads a real .zip (configuration.yaml,
          database.db, coordinator_backup.json, state.json). Double-check which HA instance/browser
          tab you're actually on first -- this was grabbed from the wrong house once already live
          today (Junglinster's own backup came back when Vienna's was wanted, since both HA tabs
          were open at once).
    - [ ] **IEEE address transfer**: confirmed procedure (2026-09-08), per
          https://www.zigbee2mqtt.io/guide/adapters/flashing/copy_ieee_addr.html -- for the "Home
          Assistant Connect ZBT-2" specifically this is NOT a firmware flash, just:
          `universal-silabs-flasher --device /dev/ttyACM0 write-ieee --ieee 0011223344556677`
          (https://github.com/NabuCasa/universal-silabs-flasher; substitute the ZBT-2's real device
          path and the OLD Sonoff coordinator's real IEEE/EUI64, no `0x` prefix on the address).
          Read the old Sonoff's IEEE address from its own backup/config before it's unplugged and
          retired. Do this BEFORE first pairing the new adapter to zigbee2mqtt -- otherwise every
          currently-paired device treats the new radio as a stranger and needs re-joining one at a
          time.
    - [ ] Stop Junglinster main's own Zigbee2MQTT add-on (releases the USB device) before unplugging
          the Sonoff dongle.
    - [ ] Physically move the Nabu Casa antenna onto protocols-server-1 (the Sonoff comes out
          instead of relocating, since it's being retired here, unlike Vienna).
    - [ ] Confirm the new adapter's `/dev/serial/by-id/...` path on p-s-1 (`ls -la
          /dev/serial/by-id/`), then set that exact path in both `serial.port` (configuration.yaml)
          and the `zigbee.container` quadlet's `AddDevice=` line -- keep `adapter: ember` (Nabu
          Casa ZBT-2 is EmberZNet-based, confirmed already staged in p-s-1's own quadlet/config
          tonight, unlike Vienna's `zstack`).
    - [ ] Install the real backup's `database.db`/`coordinator_backup.json`/`state.json` into
          `/var/lib/home-automation/zigbee/` on p-s-1, `chown 1000:1000`, but do NOT blindly copy
          its `configuration.yaml` wholesale this time -- merge the real `mqtt`/`homeassistant`/
          `advanced` (network_key/pan_id/ext_pan_id) sections into it while keeping `serial.port`/
          `adapter` pointed at the NEW Nabu Casa hardware, not the old Sonoff's by-id string.
    - [ ] Real `base_topic` is `zigbee2mqtt` (confirmed live on both houses' Green-equivalents) --
          the `zigbee2test` topic already used in p-s-1's staged/placeholder config was only ever a
          pre-cutover testing value, must be switched to the real one before this goes live, or
          every existing HA automation/dashboard referencing `zigbee2mqtt/...` topics breaks.
    - [ ] Open port 8099/tcp in firewalld on Junglinster's p-s-1 (`firewall-cmd --permanent
          --add-port=8099/tcp && firewall-cmd --reload`) -- forgotten on Vienna's own p-s-1 first
          time round, only caught after the frontend was reachable locally but refused from the
          LAN.
    - [ ] Restart the `zigbee` service, verify in the log that MQTT devices start reporting real
          (not fresh/empty) state and the frontend actually starts ("Started frontend on port
          8099"), then load `http://<p-s-1 LAN IP>:8099` from another machine on the LAN to confirm
          the firewall opening actually took.
    - [ ] Only decommission/repurpose the old Sonoff dongle once the above is verified stable, not
          immediately.

8. Zigbee2MQTT (legacy-to-conceptual MQTT passthrough, so entities migrate gradually) migration of devices in Junglinster.
    [FLAGGED 2026-09-12: no body yet, unlike item 4's own detailed "discovery_migration"
    design -- likely shares that same mechanism, but Junglinster's own specifics (which
    devices/topics) aren't written down here yet.]

9. Overkiz-based SOMFY cover control: homeassistant@protocols-server-2 -> MQTT.
      DONE (2026-09-09): the command-automation question this item used to flag as open --
      "which commands a capability/domain supports, and how one maps to a remote service call on
      the bridged instance" -- is resolved for the same-broker case: homeassistant/
      hassbridge_commands.go's domainCommands/domainDiscoveryExtras tables (switch, vacuum today),
      homeassistant/remote_instance_entity_commands.go's per-entity-per-command automations
      (exactly the `command_<fully_qualified_entity_name>_<command>.yaml` naming this item already
      sketched), and house_event_bus_coordinator/discoveryhassbridge.go's command_topic wiring.
      Adding "cover" (open/close/stop) for these SOMFY covers is now a one-entry addition to
      domainCommands, not open design. Cross-house command export (this item's own "check if
      exports of commands would still work") is explicitly NOT built yet -- deferred as Phase 2,
      same plan.

10. Volvo integration's cloud auth is broken on the "main" (Junglinster) HA incarnation (401
    Unauthorized, confirmed 2026-08-24 via the assumed-entities warning system flagging 18 missing
    `sensor.social_cars_xc40_*`/etc. entities) -- needs re-authenticating in HA's own UI. Not a
    code issue -- just a standing operational reminder for whenever you're next in that UI.

11. Ensure we have now all the integrations we need.
Then also re-enable the checking of locally (on main HA) assumed entities via the inquiry process. So, using the three values approach, triggering the inquiry of assumed to exist local entities (on main) via the coordinator. 
Also: Check for more of the existing integrations, like fritzbox, ems-esp, etc.
Also: EMS heater device -- not started, design/syntax not yet worked out.

12. Logical layer + available nuances in relation to "via device"  (netatmo radio module via main module) and aggregation.
- Node/battery_alert rules: DONE -- Rule 1 (node required, live since 2026-09-10) and Rule 2
  (battery_level iff battery_alert, live since 2026-09-11 once both houses' real macro-driven
  usages were migrated to `derived`); see
  /Users/erikproper/.claude/plans/derived-capability-mechanism.md for full history.
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

12a. Further conceptualising the conceptual layer.
node, temperature, etc, as "sub domains". 
First move to this first class treatment of sub-domains from conceptual to physical layer imports.
What is generated for the homes after this step, should still be conforming to what was generated before
Existing domains are "catch alls"

12b. Now change the generated friendly names to '<sphere>/<path>/<(sub)domain>' for all.
Also adjust the names of generated automations.

12c. Can we add more DSL syntax for the generation of WHEN-IF-THEN rules?


13. Installation-level status binary_sensors (meta sphere): one (discovery-created) binary_sensor per HA
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

14. Check aggregation of sensors. If one is down, what do we do with data?

15. Code cleaning (dead code, superseded generator logic, parser)
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
- General principle: once a platform has fully moved through its incremental integration steps
  (items 4/5/6/7/8 above), retire the now-superseded generator logic for it outright rather than
  carrying both paths indefinitely -- the dead-code entries above are concrete instances of this.
- `condition`/`with adjustment` on bare Spaces.def entities (and their generator-authored
  `template:` YAML output, `generateConditionEntities`/`generateTemplateSensors`/
  `buildAdjustmentStateExpr`, generator.go) -- confirmed by the user (2026-09-09) to be retired
  outright, code included, once real usage no longer needs them. Not yet: the physical-layer
  "derived" mechanism (see /Users/erikproper/.claude/plans/derived-capability-mechanism.md) can
  only attach to a Physical.def-declared device, so today's bare-entity usages (3 `condition` + 1
  `adjustment`, both houses) stay on the old path until they have a device to attach to -- gated on
  the Zigbee2MQTT migration (item 8) turning raw Zigbee2MQTT-discovered entities like
  `aqara_multi`'s pressure sensor into proper Physical.def-declared "discovery"-kind capabilities,
  and likely further migration beyond that. Also flagged for whenever this is revisited: a possible
  third, conceptual-to-conceptual `derived` tier (one already-positioned Spaces.def entity derived
  from another, distinct from both physical-layer `derived` and today's bare-entity mechanism) --
  not designed, genuinely speculative, only worth resurfacing once the migration above reveals
  whether it's actually still needed.

16. Architectural review, code review and documenting
   - Note (2026-09-05): GO_CONVENTIONS.md §7/§8 specifies a three-layer parser architecture
     (character stream -> tokeniser -> recursive-descent, CDL1 bool-returning style, no `error`
     returns from parse functions). The DSL frontend (Physical.def/Spaces.def parsing) does not
     follow this -- it's line-based `regexp.MustCompile`/`FindStringSubmatch` matching throughout,
     confirmed spanning at least 15 non-test files including the generic layer (`Physical_Parser.go`,
     `layers.go`, `expander.go`), not just the per-integration parsers. Flagged by the user ahead of
     the code-review step above; expected to be remedied, but deliberately NOT attempted piecemeal
     inside unrelated feature work (e.g. the 2026-09-05 import-grammar unification, which
     stayed in the existing regexp style rather than migrating just the one file it touched) --
     needs its own dedicated pass given the scope (the whole DSL frontend, not one file).

17. Test if the meta call to reset HA's works.

18. Check completeness of reported meta data

19. Maybe introduce some macro mechanism to make standard derivations (like adjustments and battery levels easier and standardized)

20. FHEM/FS20 integration

21. Publish -- Architecture.md should be ready for others to read, alongside an up-to-date
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
    else is the reason our automations didn't see the event; parked per the user's own instruction,
    not investigated further this session. Worth checking specifically when this item is picked up.

22. Future refinement (2026-08-23, not yet designed): replace the `.def` files with an integrated
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

23. Future refinement (2026-09-09, not yet designed): a generic, reversible rewriting strategy
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

---------------
# Raspberry Pi fleet storage migration strategy

Covers: the storage medium decision for the Pi 3 fleet (2x Junglinster, 1x Vienna), the
original Model B+ ("the old Pi 2"), and the specific plan for migrating `protocols-server-2`
off its microSD + external SSD "bunny hop" onto a single USB flash stick.

## 1. Storage medium decision (Pi 3 fleet)

- **Chosen medium: the official Raspberry Pi USB flash drive** (128GB), one per board.
- **Why not an SSD:** Pi 3B/3B+ only has USB 2.0, and on the 3B+ it's shared internally
  with onboard Ethernet through one controller — realistic throughput tops out around
  35–40MB/s regardless of what's attached. An SSD's main selling point (speed) is wasted;
  it only adds cost, bulk, an enclosure/cable, and potential power-draw issues on the Pi's
  modest 5V rail.
- **Why not plain/cheap microSD:** weakest option for a server-type role — prone to
  corruption from unclean power loss and wear from sustained small writes (logs, journal,
  container state).
- **Why the official RPi stick specifically:** genuinely USB 3.0 internally (wasted on a
  Pi 3's USB2 port, but indicates a decent controller/NAND), advertises resilience to
  power-loss events and SMART health reporting, and draws modest power — good fit for
  boxes that are hard to reach physically (especially the Vienna unit).
- A spare USB3 stick already on hand is a reasonable pilot/test unit before ordering the
  fleet: a USB3-generation drive, even bottlenecked to USB2 speeds, generally still has a
  meaningfully better controller/NAND than genuine USB2-era sticks, because small-file
  random I/O (what actually matters for rootfs duty) is dominated by controller quality,
  not the interface's rated sequential speed.
- A short "does it boot and run" pilot confirms functionality, not long-term endurance —
  wear-related failures typically only show up after months of sustained writes.

## 2. USB-boot capability varies by board — check before assuming

| Board | Native USB boot? | Notes |
|---|---|---|
| Pi 3B / 3B+ | Yes, via one-time OTP bit or EEPROM `BOOT_ORDER` | Confirmed already **enabled** on `protocols-server-2`. |
| Pi 2 (v1.1, BCM2836) | No | Boot ROM predates USB-MSD boot support entirely. |
| Pi 2 (v1.2, BCM2837) | Yes | Same silicon as Pi 3B — check via `cat /proc/cpuinfo` (`Hardware` line: BCM2836 vs BCM2837). |
| Original Model B/B+, Pi Zero (BCM2835, ARMv6) | **No, ever** | No OTP/EEPROM workaround exists — boot ROM code simply doesn't support it. Confirmed this is what "the old Pi 2" actually is (`cat /proc/cpuinfo` → `Hardware: BCM2835`, `Model: Raspberry Pi Model B Plus Rev 1.2`, single core). |

## 3. The old "Pi 2" is actually an original Model B+ — separate plan

- **Role:** FS20 protocol experiments, running one radio. Light workload.
- **OS:** Raspberry Pi OS — but must be the **Legacy, 32-bit** image, not the current
  default. Fedora is not an option at all (Fedora's 32-bit ARM support always required
  ARMv7+, excluding this ARMv6 chip from day one; Fedora dropped 32-bit ARM entirely after
  Fedora 36, and the chip can't run 64-bit Fedora either since it's 32-bit-only silicon).
  Mainline Raspberry Pi OS has also since narrowed to Zero W/2B onward, hence Legacy.
- **Storage:** a regular flash stick for rootfs is fine for this workload.
- **Permanent constraint:** this board can never boot purely from USB — a small SD card
  carrying the boot partition (`bootcode.bin`/`start.elf`/kernel/`cmdline.txt` with
  `root=` pointing at the stick) is required forever, not as a stepping stone. Same shape
  of hybrid as `protocols-server-2`, but for a hardware reason rather than a device/
  enclosure quirk.

## 4. `protocols-server-2`: current setup and diagnosis

**Current boot chain** (confirmed via `df` and `lsusb`):

- microSD `mmcblk0p1` → EFI System Partition (`/boot/efi`, FAT)
- microSD `mmcblk0p2` → `/boot` (kernel, initramfs, GRUB)
- External 1TB Samsung Portable SSD T3 → root filesystem, inside LVM
  (`/dev/mapper/systemVG-LVRoot`)

This is a full **U-Boot → UEFI → GRUB** chain, not the traditional Raspberry Pi
firmware-loads-kernel-directly process — meaning *all* boot-critical pieces (RPi
firmware, U-Boot, ESP, `/boot`) currently live on the SD card; the SSD only ever holds
root.

**Diagnosis so far:**

- USB boot mode is already enabled on this board.
- The SSD previously had its own boot partition; it didn't work, and the space was
  reclaimed for cache. This rules out "never attempted."
- `lsusb -t` shows the SSD bound to `usb-storage`, **not** `uas` — rules out the common
  UAS/boot-ROM-incompatibility explanation.
- Remaining live hypotheses: stale EEPROM/bootloader firmware (fixable via
  `rpi-eeprom-update`, needs a temporary Raspberry Pi OS boot since Fedora doesn't ship
  the tool), or something specific to the SSD/enclosure's own power-up/init timing that a
  bare flash stick simply won't exhibit.
- **Actual data footprint is tiny:** ~31GB used (30G root + 644M `/boot` + 45M ESP) out of
  929G — comfortably fits a 128GB stick with room to spare. Rules out a raw block-level
  `dd` of the SSD (1TB source won't fit on a 128GB destination) — migration must be a
  proper rebuild/copy, not a block clone.

## 5. Migration plan for `protocols-server-2`

### Step 1 — Build and validate a test stick

1. On a spare/other Pi 3, write a fresh Fedora image to a 128GB Raspberry Pi stick with
   the full self-contained layout (ESP + `/boot` + LVM root all on the one device).
2. Grow the root filesystem to fill the stick.
3. Move that stick to `protocols-server-2`'s actual hardware. Remove the microSD **and**
   the SSD entirely. Attempt to boot from the stick alone.

This isolates the real question — can *this specific board* natively boot a bare USB
stick — from anything about the existing SSD/enclosure. If it boots, the SSD's original
problem was device-specific, and the EEPROM-update path can be skipped entirely.

### Step 2 — Do **not** transplant the old root filesystem onto the stick

Initially considered: boot back on the old microSD+SSD pair, mount the new stick, and
`rsync -aAX` the SSD's root across (excluding `/etc/fstab`, `/boot`, `/boot/efi`).
**Rejected in favour of the approach below** — a straight rootfs transplant risks kernel/
initramfs version mismatches, stale BLS boot entries (keyed by machine-id + kernel
version), and GRUB/UUID references tied to the old device layout — all fixable, but fiddly
and easy to get subtly wrong.

### Step 3 — Selective migration onto the clean, already-tested install (preferred)

Keep the stick's own working OS/boot layer untouched. Boot the old pair, mount the stick's
root partition only (not its `/boot`/`/boot/efi`), and copy across just the things that
are actually stateful/customised:

- [ ] Hostname
- [ ] Go toolchain (if manually installed, e.g. `/usr/local/go`)
- [ ] Podman/Quadlet unit files (`/etc/containers/systemd/` or equivalent)
- [ ] Container **persistent data**, not just the unit files (e.g.
      `/var/lib/home-automation/zigbee` — the zigbee2mqtt config/state/database)
- [ ] crontab / systemd timers (`/var/spool/cron/*`, `/etc/cron.d/*`)
- [ ] Network configuration — static IP / NetworkManager profiles (check **before** the
      swap; a fresh image defaulting to DHCP on a box you can't easily reach is the
      classic way this goes wrong)
- [ ] udev rules (`/etc/udev/rules.d/` — e.g. the stable `/dev/zigbee`-style symlink used
      by the container's device passthrough)
- [ ] SSH host keys (`/etc/ssh/ssh_host_*_key*`) — judgement call: keep for continuity, or
      let it regenerate and clear the resulting `known_hosts` warnings elsewhere
- [ ] `/etc/systemd/system/*.d/` overrides, `sysctl.d` tuning
- [ ] **firewalld** — check `firewall-cmd --list-all-zones` on the old box for open
      ports/services (MQTT, zigbee2mqtt frontend, etc.), then re-apply the equivalent
      `firewall-cmd --permanent --add-port=...` / `--add-service=...` commands on the new
      system rather than copying `/etc/firewalld/zones/*.xml` directly — zones can be
      bound to a specific interface name, which may differ on the new install and cause
      the copied rule to silently do nothing
- [ ] SELinux customisations, if any
- [ ] Any additional local users/groups and their `authorized_keys`

**Finding what's actually been customised**, rather than relying on memory:

```bash
rpm -Va                                   # files differing from package defaults
find /etc -newer /etc/os-release -type f  # rough view of what's been touched since install
```

Neither is exhaustive, but together they're a useful cross-check against the list above.

### Step 4 — Cut over

Once the checklist items are copied and verified, boot `protocols-server-2` from the
stick alone. Keep the old microSD + SSD pair as a fallback until the new setup has proven
itself in normal operation for a while.
</content>
