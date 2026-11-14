** TODO 

## Purchases

Using an ESP32-CAM variant with an extended ribbon cable (flat cable) is a phenomenal idea. In fact, it completely solves your space restriction and is highly recommended for tight setups.
By separating the actual camera lens from the main motherboard, you gain three major advantages for your 8 cm gap:

1. You gain back precious distance

Standard ESP32-CAM boards have the lens mounted directly on top of the thick PCB, SD card slot, and components. By using a ribbon cable extension, the camera lens itself is only a few millimeters thick. You can mount the tiny lens completely flat against the underside of your bookshelf, pushing your lens-to-glass distance back out closer to 9.5 cm, which gives the camera a wider, clearer field of view.

2. It permanently solves the reflection problem

If you buy an extension ribbon cable, the bright onboard LED flash stays on the main ESP32 board, while the camera lens moves away.
• You can mount the main board (with the LED) further back under the shelf to illuminate the meter from a sharp side angle.
• You can mount the camera lens directly above the meter.
• Because the light source and the lens are physically separated, the light reflection will bounce off the glass at an angle and never blind the camera lens.

3. Easiest mounting possible

Instead of a bulky 3D-printed bracket hanging down from your shelf, you can literally mount the tiny camera lens using a piece of double-sided foam tape or a tiny, low-profile 3D-printed clip screwed to the wood. The main, bulky ESP32 board can be tucked neatly away out of sight at the back of the bookshelf.

What to buy:

Look for an "OV2640 extension cable" or an "ESP32-CAM with extended camera ribbon". They usually come in lengths of 5 cm, 7 cm, or 10 cm. The 5 cm or 7 cm version is perfect for your shelf setup.
(Note: You will still need to do the quick counter-clockwise "lens twist" modification mentioned before to adjust the focus to 9 cm, but handling it will be even easier when the lens is disconnected from the main board!)
Would you like me to find some visual examples or design ideas for how people mount these decoupled ribbon cameras to flat surfaces?

Here is the exact shopping list to build your decoupled AI-on-the-Edge water meter reader using the modular ribbon cable setup.
All of these components are standard items readily available on Amazon.de.

🛠️ Core Hardware Components

• ESP32-CAM Board with OV2640 Camera + Ribbon Extension Cable:
	• Search terms: ESP32-CAM OV2640 Verlängerungskabel or ESP32 CAM flexibles flachkabel
	• What to look for: A kit or bundle that explicitly includes the ESP32-CAM development board and an OV2640 camera sensor attached to a 5cm, 7cm, or 10cm flat ribbon cable.
• MicroSD Card (16GB or 32GB):
	• Search terms: MicroSD Karte 32GB SanDisk
	• What to look for: A standard Class 10 MicroSD card. The AI-on-the-edge system requires an SD card inserted directly into the ESP32-CAM reader slot to store the local neural network config files and image captures.

🔋 Power & Interfacing

• ESP32-CAM MB USB Programmer Base (Highly Recommended):
	• Search terms: ESP32-CAM-MB Micro USB
	• What to look for: A tiny adapter board that snaps onto the back of the ESP32-CAM module, adding a micro-USB port. This makes it incredibly easy to plug the unit straight into your computer via USB to flash the firmware, and later provides an easy way to power it.
• 5V USB Power Supply & Micro-USB Cable:
	• Search terms: USB Netzteil 5V and Micro USB Kabel
	• What to look for: A standard phone charger adapter block and a long enough cable to reach from your bookshelf down to the nearest electrical wall socket.

📐 Mounting the Camera Under the Shelf

• Adjustable Ball-Joint Mini Mount:
	• Search terms: Mini Kugelkopf Halterung or Kamera Wandhalterung klein
	• What to look for: A small metal or plastic swivel joint that you can screw directly into the wooden bottom of your bookshelf. Alternatively, if you or a friend have a 3D printer, you can print a zero-clearance flat surface lens holder from Printables/Thingiverse.
Would you like me to find the direct browser flashing link so you can easily install the software onto the ESP32-CAM once the parts arrive from Amazon?
	 

## Manually (Erik) extend coverage

1. check suggested in both locations
VIE
JLI

2. Ensure we have now all the integrations we need.
VIE
JLI

3. Check for more of the existing integrations, like fritzbox, ems-esp, etc.

4. EMS heater device. More imports? Its via_device by hand: EMS-ESP's discovery carries no
   availability topics, so there was nothing to derive it from (the Zigbee and Z-Wave devices got
   theirs on 2026-11-02): "via_device: integration.ems2mqtt forced;" in utility.heater's block.
 
## Migration & improvements

33. Architectural review, code review and documenting
   - Review progress per Go file: REVIEW.md (not ready / ready / re-review / reviewed).
   - Order: (1) GO_* conventions, (2) shared parser stack, (3) Physical.def as one grammar,
     (4) the other .def files, (5) mechanical fixes and splitting generator.go, (6) comments and
     docs. Steps 1 and 2 are done; next is 3. Open from step 2: parse errors are still warnings
     (exit 0, output written); the conventions want no output from input that failed to parse --
     part of step 3, where each file gets one parser result. Also open from step 2, for step 3:
     positions count from the statement, not the file (the line-based readers hand over one
     statement at a time); a keyword such as "or" can still be taken as a name in a condition
     ("a and or b"); the new parser_*.go headers say "Vienna, Austria" (check with Erik).
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
     expressions in `parser_condition.go`) does. Patched pragmatically for now
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

37. FHEM/FS20 integration

    Status (2026-11-08): FHEM is installed on Junglinster's protocols-server-2 (CUL) and -3 (FHZ),
    reached by "integration fhem" (Architecture.md §6.19); the governor runs next to the
    coordinator, with "integration governed" (§6.20); the FS20 test switch works end to end from
    Home Assistant up to the radio (sending only). Plan, decisions and next steps:
    claude-notes/plans/governed-integration.md.

    Tested 2026-11-12/13 on real devices (Erik): the switch and dimmer learned 44444444 4411/4412 and
    follow Home Assistant, a proof of concept; the switch sent to by protocols-server-3, the dimmer
    by protocols-server-2 (only the first of "heard by" sends). Open, from flipping the switch fast
    (2026-11-12, 23:31 UTC): the CUL ran out of its own air time ("cul_1: Unknown code LOVF" in
    FHEM's log, credit10ms was down to 887 after the evening's tests) and dropped commands, while
    the governor's budget still had room.

    Next weekend (Erik, on site, 2026-11-14/15):
    - FS20 left in Junglinster: only switches, no sensors or transmitters (Erik, 2026-11-13) --
      nothing to test there beyond the switches.
    - DuoFern stick (2026-11-13): FTDI FT232R, the old setup's one stick,
      /dev/serial/by-id/usb-Rademacher_DuoFern_USB-Stick_WR00S1AX-if00-port0. Physical.def's
      `duofern "<path>" code "${duofern_code}";` (the old code 6FEDCB, in Settings.def) gives
      `define duofern_1 DUOFERNSTICK <path>@115200 6FEDCB`. On protocols-server-2, behind a USB
      extension cable, it never initialised (it answered FHEM's init frame with single bytes,
      "c2"/"80"; no under-voltage reported); directly in protocols-server-3 it answers properly
      and FHEM has it "Initialized" (16:57 UTC), so it's declared there. The old setup's only
      DuoFern device, a RolloTron 406DDD (master bedroom; the old bash module was
      Old/RecentOld/mods/bridge_duofern.mod), still answers there: position 43 (DuoFern: 0 open,
      100 closed; the old setup published 100 - position), manualMode on, its automatics off,
      version 1.1. Built and deployed 2026-11-13 (option A): `duofern cover "<code>" heard by
      ...;` in an `integration fhem` device (README); the RolloTron is utility.test_rollotron
      (not yet at a window) -> cover.social_house_test_rollotron, position 57 in HA (43 in
      DuoFern), node on. Round trip from HA (17:51): set_cover_position 70 -> FHEM position 30,
      reported 29, HA 71; open_cover -> up, DuoFern 0, HA 100. Next: the real window (a rename
      from test_rollotron, its frozen IDs kept).

    The test dimmer is a governed light too (brightness, "on at 60 %", default off). To build (the plan's step 4): confirmations (the shadow state), the
    de-doubled transmitters and sensors, DuoFern. An FS20 remote is a set of buttons (Erik): its presses
    become conceptual button events, and dimming is stepwise by rules, as for the Zigbee dimmers.

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
    - It should be its own separate "role" on the Event Bus, a program and service of its own
      (the governor, next to the coordinator on its machine, Erik 2026-11-08) -- not folded into
      the existing coordinator process.
    - It needs to be held to a materially HIGHER reliability bar than the coordinator: an
      operational-role crash means devices can silently drift from their expected state (a real
      user-facing failure), whereas a coordinator crash (like the Junglinster incident this same
      day, see memory: project_junglinster_coordinator_crash_loop_2026_09_22.md) does NOT affect
      live state/command exchange at all -- FHEM2MQTT-style relays keep working regardless of
      whether the coordinator is up. This is the reasoning for why the coordinator, as currently
      built, is allowed to be less bulletproof than a component like this one would need to be.

    Decided 2026-11-02 (from item 26, Erik): of the four kinds of logical-layer work -- clustering
    capabilities, timing, derivation from other devices, abstracted entities covering a device's
    shortcomings -- only the last is operational, and it goes into this separate component; FHEM
    is its first case. Timing stays as built: template entities on main (Architecture.md).
    Before designing: by then the Home Assistant community will likely have settled its stance
    on running radios via ESPHome (Erik), which may change how FHEM/FS20 is reached at all.

    Decided 2026-11-07 (Erik, from the Shelly layer of integration mqtt): FHEM gets a layer of the same shape -- a
    statement inside the device ("fhem ...;", like "shelly "<id>";") that implies the topics,
    metadata and liveness from a family table, with the capabilities still listed by label
    (claude-notes/plans/shelly-layer.md). FHEM's own topics are ours (controlled), unlike Shelly's.

    Discussion (Erik, 2026-11-08); planning started in claude-notes/plans/governed-integration.md.
    Decided since: the "indirect" integration is called "governed"; a radio signal is sent "on by
    doing <button>". The discussion, in Erik's terms:
    - Distinguish (1) devices with a reliable two-way connection -- true light/switch/... devices;
      FHEM's syntax for them is platform-specific and "messy", so sub-integrations still make sense
      -- from (2) devices we "talk" with by radio (also IR and Bluetooth, across integrations,
      triggered by the FHEM/FS20 case), with at most indirect reports of their state.
    - For (2): define outgoing radio signals (FS20) as button capabilities, with a value as
      parameter (on, off, 50%), incoming ones as event capabilities, and create an "indirect"
      integration type at the physical level (needs a better name).
    - The indirect integration defines devices (with their metadata), controls them through
      buttons, and possibly updates their actual state from sensor/event reports. These buttons,
      sensors and events may come from any other integration. It needs its own application,
      running next to the coordinator.
    - Example: the FHEM integration defines a button that makes all known FHEM installations listen
      to its topic, and in parallel send an on/off signal to the FS20 device. The indirect
      integration uses it (and possibly buttons of other radios) to define a switch with an
      "enforced every X" and "optimistic" approach: the state assumed to exist (optimistic) is
      re-sent on the radio through the buttons every X.
    - Another device (of another integration) may let us observe the actual state: a "state
      confirmation" linked to an event, which updates the indirect device's internal shadow state.
      A discrepancy between the reported state and the desired/assumed state leads to immediate
      enforcement of the desired state. Hence "indirect" -- apt, though not the best name.
    - Both FHEM radios are in scope (Erik, 2026-11-08): FS20 and DuoFern -- a cover system,
      reached through FHEM with a dongle, not (yet) supported by Home Assistant itself.
    - To reflect on: radio-specific (e.g. FS20) throttle groups that limit the number of requests
      the indirect application sends to a radio.

40. https://github.com/kaiplaketti/matter2mqtt
I think this one wraps the HA one. Check if it is up-to-date
The idea is to offer matter integration into HA via MQTT.
So, not offering local devices from HA to matter.

    Looked at 2026-11-08: a few hundred lines of Python on the Matter Server's websocket
    (start_listening, OnOff through the CHIP library's clusters), on/off devices only, no
    commissioning; 4 commits Jan-Apr 2026. The Python Matter Server it builds on was archived in
    June 2026 for a matter.js server (secondary sources), so taking it over means rewriting it.
    Suggested instead: the Matter Server (and an OpenThread border router for the stick) on
    p-s-1, and Home Assistant's own Matter integration on ha2mqtt, bridged as the other native
    integrations are. Cheap test devices: IKEA's Matter-over-Thread GRILLPLATS plug, MYGGSPRAY,
    KAJPLATS, BILRESA (about 9 EUR); a Wi-Fi Matter plug needs no Thread at all.

    The Thread radios (Erik, 2026-11-08): each house has a Nabu Casa stick (SkyConnect), free, with
    Thread firmware already, now in its Green; they move to each house's p-s-1 (Vienna's this week)
    -- one Thread mesh per house. Per p-s-1: the OpenThread Border Router and the Matter Server as
    containers, ha2mqtt's Thread and Matter integrations on them. No Thread devices yet, so the new
    border router can form a fresh network; the Green's OTBR add-on goes, so it doesn't advertise a
    border router without its radio.

    Decided (Erik, 2026-11-08): Home Assistant's Matter integration on ha2mqtt, until a bridge of
    our own is needed -- no maintained Matter-controller-to-MQTT bridge exists (the projects found
    go the other way, MQTT to Matter); ours would be a small Go client of the Matter Server's
    websocket API, publishing on our own topics, as the governor does for FS20.

    Vienna, done 2026-11-08: the SkyConnect on protocols-server-1
    (/dev/serial/by-id/usb-Nabu_Casa_SkyConnect_v1.0_bea642ed...-if00-port0, Thread RCP, 460800
    baud, flow control); quadlets otbr.container (ghcr.io/ownbee/hass-otbr-docker:latest, REST API
    on 8081, web UI off, firewall and NAT64 on, backbone eth0) and matter.container
    (ghcr.io/matter-js/matterjs-server:stable, port 5580; its data folder owned by uid 1000, the
    image's user); /etc/sysctl.d/60-thread-border-router.conf (IPv6 forwarding, accept_ra 2,
    accept_ra_rt_info_max_plen 64). Both data folders are under /var/lib/home-automation, so the
    nightly backup-home-automation (generic over that folder, off-box to mqtt.erikproper.eu) takes
    them; update.containers covers every quadlet. Vienna also has an Apple Thread border router
    (Living-Room-2, 192.168.178.44) with a network of its own.
    ha2mqtt has the Open Thread Border Router (http://127.0.0.1:8081), Thread and Matter
    (ws://127.0.0.1:5580/ws) integrations (Erik, 2026-11-08): the border router is leader of
    Vienna's own Thread network, ha2mqtt is connected to the Matter Server; the Green's OTBR
    add-on is gone. Next: a first Matter device in Vienna (commissioned through ha2mqtt, its
    entities onto the bus); the same setup for Junglinster.

    Junglinster, 2026-11-13: the SkyConnect moved from the Green to protocols-server-1 (Erik;
    /dev/serial/by-id/usb-Nabu_Casa_SkyConnect_v1.0_3c093a724bedec11977e4740ad51a8b2-if00-port0);
    Vienna's otbr.container, matter.container and sysctl file, with backbone eno1 (instead of
    eth0). Both active, the RCP answers (Thread 1.4.0); no Thread devices. ha2mqtt has the Open
    Thread Border Router (http://127.0.0.1:8081), Thread and Matter (ws://127.0.0.1:5580/ws)
    integrations (Erik): the border router is leader of Junglinster's own Thread network, ha2mqtt
    is connected to the Matter Server. The Green's OTBR, Thread and SkyConnect entries are gone;
    what's left there for Matter (and ZHA) are ignored discoveries.

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

48. Future refinement (moved from 26 on 2026-11-02; not needed at the moment, Erik): device-type
    templates -- a named template expanding into a whole device block (the "basic node" shape
    first); to scope: parameters, where declared, what else would use it.

49. Future refinement (moved from 26 on 2026-11-02; only when a real case comes up):
    "dependency on entity <id>;" and "dependency on availability of entity <id>;" for a raw
    entity on main (integration_logical_parser.go's header).

50. (was 32b; to observe) Utilise Netatmo's new webhooks? Or wait for HA to pick this up?
    Netatmo cuts polling rate limits from 1 March 2027 (developer email, per openHAB issue 22187);
    HA's integration still polls exactly our weather stations and air-quality monitors. Plan: wait
    for HA; re-check January 2027, else build a webhook receiver in the coordinator.
https://dev.netatmo.com/apidocumentation/webhooks

51. (was 32c; to observe) The deploy's reload check can't read Junglinster's log: system_log/list closes the websocket
    (1006, unexpected EOF) on every deploy there (seen three times 2026-11-05), so a reload error
    there goes unreported. Vienna's works. Likely the size of Junglinster's log in one message.
    Seen twice more 2026-11-08, after reloads that went fine (the new entities there, the error
    log clean). Besides the cause: the check should report a failed read as such, not as "reload
    reported errors". It did cost something on 2026-11-11: main's whole MQTT YAML reload had
    failed since the Zigbee2MQTT 2.14.3 update (an invalid template, fixed in 6cc3995), so no
    MQTT entity change took effect, and no deploy reported it. A fallback worth having: call
    mqtt.reload over REST, whose 400 answer names the failing file.

52. Shelly firmware (Erik, 2026-11-07: there will be more Shellies to update). The Shelly layer
    (claude-notes/plans/shelly-layer.md) could give each device an "update" entity that only signals
    an update -- installed and latest version from "info"'s "update" field (needs a value template
    for JSON), no command topic: the update itself is done by hand (Erik). Gen1 plugs on v1.11.8
    don't update themselves: their web page fails ("Could not get the timezone list"), and both
    /ota?update=true and /ota?url=<Shelly's own file URL> go from "updating" back to "pending"
    without a reboot (cause unknown). What worked (2026-11-07, shellyplug-6CC3D8 and -6CC43B to
    v1.14.0): serve the file on the house's network and point the device at it. The file per model
    is in https://api.shelly.cloud/files/firmware ("data" -> model code -> "url", e.g.
    http://firmware.shelly.cloud/gen1/SHPLG2-1.zip); on p-s-1 (192.168.178.85, firewall off):
    download it into a temporary directory, run "timeout 600 python3 -m http.server 8000 --bind
    192.168.178.85" there, and call http://<device>/ota?url=http://192.168.178.85:8000/<file>.zip.
    It fetched it at once and was updated within 20-40 s, its settings (MQTT, retain, update
    period, id) and topic tree unchanged.

53. (to observe, from the raw MQTT work, 2026-11-07) Two coordinator findings: a shadowed state
    topic's shadow messages arrived twice (harmless, same value; cause not known -- maybe the
    liveness "#" subscription overlapping the shadow's own); and switching a device to "is
    retained;" clears its shadows when the coordinator restarts, before main reloads, so its node
    read "off" for those 2 minutes -- the deploy could reload main before restarting the
    coordinator in that case.

54. Junglinster's office innr plugs drop from Zigbee (found 2026-11-09). Baseline, Zigbee2MQTT
    logs 2026-10-27..11-09: real drops (two failed pings) only of office plugs -- shredder 7,
    cycling_fans 7, mini_fans 8, media_drive 4 -- in bursts (the night of 11-01, 10-29, 11-09),
    with no office plug-switched device on at the time. Not the plugs (the two Vienna spares, SP
    220, dropped in Junglinster but not in Vienna; SP 220/120 have no newer firmware, the SP 240s
    are current), not the network's size (56 routers, 62 end devices). Network map 2026-11-09:
    office links strong (LQI 200-255), but of the powered routers that didn't answer the scan, 12
    of 13 are in the office, server room and storage room (the lights behind Z-Wave relays were
    off and don't count). Suspect: the Wi-Fi repeater 2-3 m from the plugs (Wi-Fi channel 6,
    Zigbee channel 11). Test: switch it off for a day or two, or move Wi-Fi to channel 11/13
    (then Thread on 15, not 25); then recount the drops and repeat the map.
    Test started 2026-11-09 evening: Junglinster's FRITZ!Box fixed on Wi-Fi channel 11 (all
    three repeaters followed at once; fritz-repeater-office is 192.168.178.126). Checked and
    excluded: the Sonos speakers are wired (no SonosNet; only the kitchen Move uses Wi-Fi). Vienna
    is on Wi-Fi 11 too (Zigbee 11, Thread 15 there). In a few days: recount the office drops
    against the baseline above, and request a new network map.
    2026-11-09 late evening (Erik saw them missing): office/cycling offline since 21:33 and
    office/cycling_fans since 23:37 (both SP 220), the latter after the channel change. Erik is in
    Vienna, so no power-cycling for now: check whether they come back by themselves (the mesh
    refreshing its routes), and keep counting from there; power-cycle them once someone is there.
    Re-pairing helps (Erik, 2026-11-10, with reports on the internet): office/shredder was
    re-paired 2026-11-01 12:17 (a fresh interview) after 7 drops the night before, and has not
    dropped once since. The two Vienna spares fit too -- they dropped in Junglinster and are fine in
    Vienna, where they were paired anew. So the main lead is now the plugs' pairing, not the place
    or Wi-Fi: re-pair cycling, cycling_fans, mini_fans and media_drive on site (an innr resets by a
    pattern of power cycles), keep Wi-Fi on 11, and count each plug's drops from its re-pairing.
    Re-paired 2026-11-13 (Erik, on site; Zigbee2MQTT's interviews, UTC): cycling 07:08,
    cycling_fans 07:31, mini_fans 07:40 -- all three online again, with their old names. Still to
    re-pair: media_drive. Count from those times: zigbee2mqtt's journal on p-s-1 (unit "zigbee"),
    two failed pings being a drop, as in the baseline.
    Not only the office: master_bedroom/left_table_lamp (a Philips bulb) had failed commands since
    2026-10-27, worsening (1-3 a day, 9 on 11-11, 29 on 11-12), then dropped off; Erik re-paired it
    2026-11-13 06:46 UTC. Count it too.
    New lead (2026-11-13): the coordinator stick. Junglinster's is Silicon Labs EmberZNet 7.5.1
    (Zigbee2MQTT's "ember" driver), since the p-s-1 rebuild around 2026-10-15 -- before that a TI
    stick, Z-Stack, as Vienna still has without drops (its journal starts 10-15, so drops before
    the change can't be checked). The pattern fits the stack, not the devices: plugs of two models
    and a Philips bulb degrade, then drop, and re-pairing (renewing what the stick keeps) cures
    them. media_drive (removed from Zigbee2MQTT 07:56 UTC) doesn't re-pair: it joined as
    0x0000000000000000 (07:50, the interview failing), then with its real address twice through
    office/shredder (08:37, 08:40), interviewed and left again within 20 s (the second time by
    itself), and at 08:45 (all routers open) wasn't heard at all. Next to the stick, joining on
    the stick only: joined 08:51:42 and 08:58, interviewed, left 18 s and 9 s later. Before the
    second leave, dozens of ROUTE_ERROR_ADDRESS_CONFLICT for network address 18849 and "An ID
    conflict was detected for network address '18849'" -- an address no device in Zigbee2MQTT
    has (a node on the network it doesn't know?); the stack then drops the address from its
    tables (herdsman's ezspIdConflictHandler only logs; its "kicked" is misleading). Such ID
    conflicts since 10-15: 10-26, 10-30 (2), 10-31, 11-02, 11-07, 11-13 -- to set against the
    office drops. Network map (09:47 UTC, 118 nodes): no node has 18849; five routers
    (office/heating, server_room/raids1, christmas_tree, loft/main, kitchen/go) keep an active
    route to it as a direct neighbour -- probably leftovers of media_drive's own joins; 45 route
    destinations belong to no known node. One unknown node: 0x00158d0001c105a3 (Xiaomi/Aqara
    prefix), a sleeping end device, 29181, child of the stick, not in Zigbee2MQTT's device list --
    likely one of the redundant Aqaras Erik removed; batteries out, so the stick ages it out.
    Replaced 2026-11-13 by a new Innr SP 240 (0xa4c13815ca863d27, joined 10:14:57 UTC on the
    stick, stayed); the old SP 220 goes to Vienna, as a test on its TI stick. Physical.def's
    hardware lines swapped, the frozen IDs kept. Open from the swap: the coordinator's discovery
    existence keeps the old plug's leaves known-to-exist for good (Seed prunes only
    known-not-to-exist; no retraction reaches them, the old IEEE no longer maps to the device),
    and the old plug's retained discovery configs (homeassistant.physical/+/0x74057c0000004726/...)
    are still on the broker. The generator's firmware updates took the stale update leaf over
    the positioned one (fixed: firmware_updates.go keeps a positioned link).
    Plug models (failed ping rounds 11-06..11-13): the six SP 240s 0 each; SP 220/120 0-71
    (cycling 71, tv 51, gray_sideboard 48, red_highboard 44, cycling_fans 42); Vienna's SP 220s
    (2.03.10) and SP 120 0 each. 2026-11-13 ~12:35 disks1 (SP 220, 2.03.10) offline; Erik removed
    it (forced, 12:51), disks2 (SP 220, 2.03.10) left 20 s later. SP 220 2.03.10 plugs don't stay
    on the stick: old media_drive, disks2, disks1 (joined 13:05 and 13:09, after a Zigbee2MQTT
    restart that resets the stick's tables) all leave 10-20 s after a successful join and
    interview, with no error logged; the 2.3/2.0 ones (cycling, cycling_fans, mini_fans) re-paired
    fine this morning. Guess: the link-key exchange after the join. A debug-log try (13:11-13:15)
    caught no attempt (disks1 had stopped searching). The drops themselves were partly there on
    the old TI stick too (Erik). Idea (Erik): the 2.03.10 and worst SP 220s to Vienna, its
    non-SP 220/120 plugs (nespresso SP 240, workplace SONOFF) or new SP 240s to Junglinster.
    Debug log of disks2's join (13:32, after a round-trip through an old Hue bridge on channel
    25, which can't update it either): join (STANDARD_SECURITY_UNSECURED_JOIN), interview and
    configure fine, then KeyEstablishment TC_RESPONDED_TO_KEY_REQUEST followed by
    TC_REQUESTER_VERIFY_KEY_FAILURE, and DEVICE_LEFT 18 s later. Likely cause: the coordinator's
    IEEE is the old TI stick's (0x00124b002a2e3ee0), migrated onto the ZBT-2 -- Zigbee2MQTT's
    troubleshooting guide, "Post-IEEE (EUI64) migration troubles": the stack doesn't always take
    the migrated IEEE everywhere, and devices leave seconds after pairing. To do, on site (a
    weekend): stop Zigbee2MQTT, check for the EUI64 mismatch with ember-zli (Stack); if there
    is one, clear the stick's NVM3 (ember-zli, Bootloader) with coordinator_backup.json and
    configuration.yaml in the data folder, start, let it restore; give it a day. Then pair
    disks1, disks2 (and the old SP 220) again.
    ember-zli 6.1.0 (Erik, 13:48, Zigbee2MQTT stopped): node and trust center EUI64 both
    0x00124b002a2e3ee0, "EUI64 match. No fix required." -- so not that bug; no NVM3 clear.
    Left: SP 220 2.03.10 against EmberZNet 7.5.1's link-key verification. Only three plugs in
    Junglinster have 2.03.10 (disks1, disks2, the old media_drive); Vienna's five 2.03.10 run
    fine on its TI stick. Decided (Erik, 2026-11-13): the three 2.03.10 plugs go to Vienna, to
    see whether they stay stable there; in the storage room the two test Shelly plugs take
    disks1's and disks2's places for now. The droppers (2.3: cycling, tv, gray_sideboard,
    red_highboard, cycling_fans) wait -- to be seen later.
    disks1 is the Shelly shellyplug-6CC3D8 since 2026-11-13 14:12 UTC (the former test_shelly_plug,
    its block moved into disks1's place in the Shelly section; switch and node keep disks1's
    frozen IDs; power, energy and consumes new; the Innr's firmware entity is left unavailable in
    the registry); disks2 likewise is shellyplug-6CC43B since 14:18 UTC (the former
    test_shelly_plug_2; its test entities, e.g. switch.social_house_test_plug_2, left unavailable).
    Found on the way: (a) a Shelly block in the Zigbee2MQTT section isn't recognised ("shelly"
    line unrecognised), and the generator's stable-id assignment then appended a second "as
    unique_id" to each capability that already had one -- it should leave a frozen ID alone, also
    in a misparsed block; (b) the deploy's reload check failed three times today (10:37, 14:13, 14:18) with
    "system_log/list: websocket: close 1006 (abnormal closure)" -- the check itself, not HA's
    reload: to look into.
    Next week in Vienna (Erik, decided 2026-11-13): reset SP 220s (2.03.10, from Junglinster)
    replace nespresso's SP 240 (appliance.living_room_kitchen_nespresso) and workplace's SONOFF
    (light.living_room_kitchen_workplace) -- pure hardware swaps in Vienna's Physical.def (new IEEE
    and model lines, frozen IDs kept; neither uses power or energy); Vienna's SP 120 (tv) is
    replaced too, and goes to Junglinster as a test: first just plugged in (paired, undeclared),
    to see whether it holds up there. SP 120s drop in Junglinster as well (studio 0,
    hanging_sideboard 14, mini_fans 25 failed ping rounds 11-06..11-13), so the drops aren't
    SP 220-only; the join failure is (2.03.10 only). Junglinster's own spare SP 240 stays a spare.
    2026-11-13: living/room/tv (SP 220, 2.3) offline from 14:24 UTC (after the 13:50 Zigbee2MQTT
    restart and the ~14:02 power issue); unplugged and replugged by Erik at 16:55, back without a
    re-pair (state ON, LQI 148).
    The stick's tokens (ember-zli 2026-11-13 ~17:05, Get stack config / Get security info / Backup
    NVM3 tokens, /root/ember-zli/tokens_backup.nvm3 on p-s-1): one IEEE everywhere -- the NVM3
    override (0x1E12A) and the trust-center entry (0x1E124) both 0x00124b002a2e3ee0,
    MFG_TOKEN.CUSTOM_EUI_64 unset (all FF). But the link-key table (0x10600, KEY_TABLE_SIZE 12) is
    full: six stale entries (the old media_drive 74057c0000004726, disks2 74057c000000dbcf,
    disks1 c4988600000f73e3, and 74057c0000004f95, 74057c00000062b0, c4988600000f738d -- gone from
    Zigbee2MQTT) and six live ones (shredder, red_highboard, house/server_room/philips_motion,
    house/bathroom/philips_dimmer, house/bathroom/right/philips_motion, the upward_hallway NodOn).
    Likely the TI era's per-device link keys, imported at the migration (as many as fit): a device
    with an entry gets a freshly derived key on joining but is verified against the stored one --
    TC_REQUESTER_VERIFY_KEY_FAILURE; devices without an entry (the SP 240, cycling, cycling_fans,
    mini_fans) join fine. To test: erase disks2's entry, re-pair it. To do: a safe way to erase
    single entries (ember-zli's Repairs menu? else EZSP eraseKeyTableEntry with Zigbee2MQTT
    stopped; not by restoring an edited token backup -- it would reset the frame counters);
    erase the six stale ones; a live device's entry goes before it is re-paired.
    CONFIG.NEIGHBOR_TABLE_SIZE is 16 (firmware default) in a network of 56 routers.
    Confirmed 2026-11-13: the six stale entries erased (17:12 UTC, Zigbee2MQTT off ~6 s; a script
    on ember-zli's own EZSP library, ~/ember-zli/ember-zli/keytable.mjs on p-s-1: lists the
    partners, --erase erases the listed stale ones -- ezspEraseKeyTableEntry), the six live ones
    kept. Then the old media_drive (0x74057c0000004726) joined at 17:14:17 with
    TC_REQUESTER_VERIFY_KEY_SUCCESS and stayed. So the 2.03.10 plugs can join this stick after all;
    before re-pairing a device with a live entry (shredder, red_highboard, the Philips motion
    sensors and dimmer, the NodOn), erase its entry first. Whether the three still go to Vienna:
    Erik's call.
    All three re-paired 2026-11-13, keys verified: 0x74057c0000004726 (17:14, unnamed), disks2
    0x74057c000000dbcf (17:40 -- twice before it joined straight to the far stick and left after
    17-21 s; through a neighbouring plug it stayed), disks1 0xc4988600000f73e3 (17:50, unnamed;
    joining open on all routers). Placement and names: Erik's; Zigbee2MQTT still calls one
    "house/storage_room/disks2", which the Shelly is now.
    Pairing an Innr (Erik): open joining first, then reset or plug it in -- it searches only
    briefly and gives up if joining isn't open yet (as disks1 did at 17:18). After the join it
    sometimes needs unplugging and plugging back in to work properly -- try that before a new
    reset if it leaves or doesn't answer.
    Kept in mind (less needed since the key-table fix): a second Junglinster network on the old TI
    stick (a new IEEE -- its own is the one the ZBT-2 carries
    -- its old network wiped, channel 25, its own Zigbee2MQTT and gateway), if more such devices
    turn up.
    The stick is a Nabu Casa ZBT-2 (serial ...5AB8, next to the ZWA-2). Its firmware update is on
    hold (Erik, 2026-11-13: conservative): 7.5.1 is Nabu Casa's newest stable ZBT-2 Zigbee
    firmware (v2026.02.23); there is no 8.x, only 9.1.1 as a beta (v2026.08.18-beta1,
    v2026.09.17-beta2). Look again when a 9.x is stable, and check Zigbee2MQTT's ember driver
    supports it. How: stop Zigbee2MQTT, back up /var/lib/home-automation/zigbee, unplug the ZBT-2,
    flash from a Mac in Chrome/Edge (WebSerial; Firefox can't) with toolbox.openhomefoundation.org,
    plug it back into any port (the quadlet finds it by serial ID), start, check bridge/info: new
    revision, IEEE still 0x00124b002a2e3ee0. Then recount the drops.

55. Podman's bridge broke on Vienna's protocols-server-1 (2026-11-09, during update.containers):
    every start of mosquitto (the only container there on podman's bridge network) failed with
    "netavark: create bridge: netlink: sequence_number out of sync (got 2, want 3)", and systemd's
    retry then left podman0 down without an address -- mosquitto unreachable, Zigbee2MQTT in a
    restart loop, Vienna's devices down for about half an hour. Deleting podman0 didn't help. Fixed
    by running mosquitto on the host network (its quadlet: Network=host instead of
    PublishPort=1883:1883), as the other containers already are. Cause not known: the first bridge
    set-up since the OpenThread Border Router (wpan0, its routes, NAT64) and IPv6 forwarding came
    to that host the day before (PROJECT.md 40) -- likely related. To do: find the cause; before
    Junglinster gets its border router, put its mosquitto on the host network as well (and any
    other bridge-network container there). Junglinster's mosquitto is on the host network since
    2026-11-09 (Erik). Vienna's p-s-1 rebooted the same day, also into its new kernel 1020: Ubuntu's
    Pi A/B boot had kept it in new/ since 2026-10-16, still marked "bad" from a July test boot of an
    earlier kernel ("piboot-try --reset-new --reboot" tried it; now current/ good, old/ 1017).

57. Junglinster, weekend 2026-11-14/15 (Erik on site): change the batteries the battery warnings
    name -- the frient in the laundry kitchen front and the Aqara magnet on the living room
    windows; take the batteries out of the removed Aqaras (54, the unknown node). Test the FS20
    dimmer's dimming with an incandescent bulb. disks1 and disks2 back to their Innrs (54;
    Zigbee2MQTT: 0xc4988600000f73e3 -> house/storage_room/disks1; Physical.def: both back into the Zigbee2MQTT section, switch and node frozen IDs kept;
    the Shellys' power/energy/consumes go, the firmware entity returns). The two Shellys then go
    back to their test declarations (switches.test_shelly_plug, ..._2, their old frozen IDs): they
    and the FS20 test devices stay as the ultimate backups (Erik). If there is time: the updates
    pending on p-s-3 (checked 2026-11-13): about 45 (kernel, OpenSSL, FHEM). FHEM itself (held by its package) only deliberately, on p-s-2 and
    p-s-3 together, checking the generated fhem.cfg still loads (now with the DuoFern stick).


58. (to observe, found 2026-11-14) Junglinster's Ring events don't reach HA: the doorbell's ding
    and motion events last fired in October 2025, though Ring itself records activity (polling
    works). Cause: HA's firebase_messaging (in HA 2026.11.1) can't decode Ring's newer two-part
    push header, logs "Incorrect padding, shutting down FcmPushClient" and stops listening (HA
    core issues 189885 and 191852; library fix proposed in sdb9696/firebase-messaging PR 37).
    Ring was re-authenticated on 2026-11-14 and all stale "Qemu Python" clients removed (each
    was an earlier HA login), so that isn't it. Wait for an HA release with the fix; no
    third-party workaround on main. After each HA update: ring the bell, check that
    event.social_entrance_front_door_ding fires. Then bring back
    binary_sensor.social_entrance_motion (the old bash generator's template on Ring's
    motion binary_sensor, which HA replaced with event.social_entrance_motion): on for a short
    while after each motion event, which may need the generator to derive a binary_sensor
    from an event. Harmless meanwhile: "Unknown kind: cocoa_camera_v2", the terrace Stick Up
    Cam being newer than the Ring library.
