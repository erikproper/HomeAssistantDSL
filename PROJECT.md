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

    Next weekend (Erik, on site, 2026-11-14/15):
    - Test the FS20 switch and dimmer on real devices (teach them house code 44444444, 4411/4412).
    - See whether there are FHZ/FHM sensors or controls around (transmitters, sensors: the
      de-doubled events and states of the plan's step 4).
    - Insert the DuoFern stick into one of the two Pis (then: its serial kind, and DuoFern covers).

    The test dimmer is a governed light too (brightness, "on at 60 %", default off; generated,
    not yet deployed). To build (the plan's step 4): confirmations (the shadow state), the
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
    reported errors".

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

    Junglinster, planned for 2026-11-10 (Erik, when the house is empty): update.containers;
    reboot protocols-server-1 into kernel 43 (x86, a plain reboot); on protocols-server-3 the 9
    pending packages (firmware, kernel 6.18.60) and a reboot. FHEM itself (held by its package)
    only deliberately, on p-s-2 and p-s-3 together, checking the generated fhem.cfg still loads.
    Checked 2026-11-09: no failed units, no other bridge-network containers, p-s-2 and frame
    already on kernel 1020 with a clean boot state.

56. Overkiz's low-speed covers (Home Assistant 2026.11 deprecates them, gone in 2027.5): the
    slow covers now command their cover itself with "speed: slow" ("slow speed;" without a
    source, README "Covers with a slow speed"); generated, deployed to Junglinster's coordinator
    and ha2mqtt 2026-11-10, all seven slow covers on main available. This weekend (2026-11-14/15,
    Erik on site): move one slow cover from main (e.g. cover.social_house_shower_room_slow) and
    check it runs slowly; then disable the seven cover.*_low_speed entities in Overkiz on ha2mqtt
    and restart it, so the repair messages go.

57. Junglinster, weekend 2026-11-14/15 (Erik on site): change the batteries of the frient in the
    laundry kitchen front (20 %, alert), the Aqara magnet on the living room windows (alert), the
    Netatmo in the upward hallway first floor (21 %) and the Tuya on the garden terrace (21 %);
    check office/drain/left/aqara_water (offline). Re-pair the office plugs (54); test the slow
    cover and disable the low-speed entities (56); if there is time, item 37's on-site steps.

