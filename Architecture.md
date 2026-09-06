# Federated Home Assistant: an Architecture for Modelled, Multi-House Automation

**Status:** living document. Restructured 2026-08-18 from the original notes plus `Background/Arch1.md`, `Arch2.md`, `Arch3.md`. Superseded fragments were folded in rather than kept as separate files; `Background/*.md` remains as raw source material. Named "Federated Home Assistant" 2026-09-06, resolving the "Federated open home" naming question §13 had carried open since 2026-09-02 (see that section's own history) — the two words capture the two things this project actually is: **Federated**, because autonomous houses (§12) cooperate over a shared event bus without any one of them being a hub for the others; **Home Assistant**, because that's the concrete platform every house's own instance runs, even though the model itself (below) is deliberately platform-agnostic.

## 1. Central principle

> The house is modelled, not programmed.

Home Assistant, FHEM, MQTT, Z-Wave, Zigbee, Matter, and every cloud service are implementation platforms. The source of truth is the semantic model of the house, expressed in the DSL. Platforms consume generated configuration; they do not define the model.

Terminology follows Home Assistant's own vocabulary (entities, devices, integrations, areas) even where that vocabulary isn't ontologically precise — consistency with the target platform's mental model beats theoretical purity.

### What this project actually is

The DSL has outgrown "a configuration language for Home Assistant." It is becoming a model-driven specification language for cyber-physical information systems, of which Home Assistant is one backend and MQTT is one transport — a Model-Driven Engineering (MDE) approach in the direct sense: the DSL's own `.def` files are the model, and the generator is a model-transformation tool projecting that one model onto whichever concrete platform (HA YAML today) needs to consume it. Other projections (documentation, a dependency graph, a digital twin, an ArchiMate diagram) are the same kind of artifact as the HA YAML — just different generators (different transformations) over the same model. Keeping the conceptual core clean (see §3) and resisting the temptation to let HA- or MQTT-specific concepts leak upward into it is what keeps that positioning possible. §2 makes the MDE framing explicit against OMG's own Model Driven Architecture terminology (CIM/PIM/PSM).

---

## 2. The three modelling layers

The architecture mirrors OMG's Model Driven Architecture (CIM/PIM/PSM), and separately reads as classic Information Systems layering (conceptual/logical/physical). Both framings describe the same three layers:

| MDA | This project | Answers | Current DSL file |
|---|---|---|---|
| CIM | **Conceptual** house model | What does the house mean to the people who live in it? | `Entities.def` |
| PIM | **Logical** / canonical model | What devices and capabilities exist, independent of protocol? | *(planned — see §10.4)* |
| PSM | **Physical** / technological model | How is each capability actually implemented (Z-Wave node, MQTT topic, FHEM device, ...)? | `Integrations.def` (draft) |

```
                    Conceptual House Model   (CIM)
                            |
                    Canonical / Logical Model   (PIM)
                            |
                    Technological Model   (PSM)
                            |
              +-------------+-------------+
              |             |             |
        Home Assistant    FHEM        Coordinators
              |             |             |
          Automations    Legacy       Radio/cloud
                         systems      adapters
```

- **Conceptual**: rooms, functions, comfort/automation concepts, aggregates, virtual devices — deliberately free of technology detail. This is what inhabitants perceive: *"Living Room Main Light,"* not *"Z-Wave node 79."*
- **Logical**: devices that provide entities. A device is not yet a "real" entity until it is positioned in a space. Devices may themselves be combined from other devices/entities, via two distinct operations (noted 2026-08-22, not yet designed at the DSL/generator level — gated on Z-Wave and Zigbee2MQTT both being MQTT-based, Phase 3 steps 3-4 in PROJECT.md):
  - **Aggregate**: combine devices into a genuinely *new* logical device that isn't really either constituent alone — e.g. an external temperature sensor merged into a smart plug to form one logical "heater" device.
  - **Absorb**: one device becomes subordinate to, and extends, an existing "master" device rather than forming something new — e.g. `switch.infrastructural:garage/smarty` (a Zigbee-controlled power switch for the `smarty` host) absorbed into `host.smarty` as its on/off switch capability; or the common Z-Wave/Zigbee pairing of a Z-Wave wall switch (linked to a physical wall switch) absorbing the Zigbee dimmer light(s) it actually controls, forming one logical light.
- **Physical**: protocol- and integration-specific reality, abstracted (mostly) to MQTT, with pragmatic exceptions such as media players and Z-Wave (see §11).

The relationship between the conceptual and logical/physical layers is a mapping, not a 1:1 correspondence — one conceptual light can be realized by several physical devices plus an automation stitching them together, and inhabitants never need to know that.

---

## 3. Three semantic namespaces

Entities at the **conceptual layer** (§2) are organized into three namespaces (spheres): `social:`,
`infrastructural:`, `meta:`. Spheres are a conceptual-layer concept, orthogonal to the
conceptual/logical/physical *layer* split in §2 — every entity lives in exactly one sphere and is
still realized through all three layers underneath it; "physical" names a layer in §2, deliberately
not reused below as a sphere name (see the legacy note at the end of this section for why that
wasn't always true).

- **Social** — directly relevant to inhabitants: `social/house/living_room/co2`, `social/house/bedroom/temperature`. Stable even when the underlying technology changes; this is the conceptual layer's surface.
- **Infrastructural** — entities reporting on the state of the automation platform's own IoT infrastructure, not the house itself: `infrastructural/pi4/status`, `infrastructural/mqtt/status`, node/device availability, battery levels, radio/signal strength. These enable infrastructure self-healing (e.g. detect a crashed Pi → power-cycle its smart plug → wait → verify recovery) independently of house semantics.
- **Meta** (added 2026-09-06) — entities that operate the DSL/generator/coordinator system itself, as opposed to sensing or maintaining the house's own IoT infrastructure: the "Discover entity" bootstrap control (§6.9), the reload/restart meta-commands (PROJECT.md item 2 in its pre-2026-09-06 numbering), a future config-problem/updates-available status (PROJECT.md's own "Installation-level status binary_sensors" item). The social/infrastructural split is about *what the entity is about* (the house vs. its own platform); meta is about a third, reflective case — entities *about the architecture's own operation*, not about anything physical at all. Not yet DSL/Spaces.def-driven in practice: today's meta entities (`text.meta_discover_entity`, `button.meta_reload_*`/`meta_restart_*`) are coordinator-authored with hand-picked stable IDs, not positioned through Spaces.def like a social/infrastructural entity would be — making them genuine Spaces.def citizens under this sphere is future work, not done as part of naming the sphere.

**Legacy note**: both real houses' current Spaces.def files also use `physical:` as a sphere
(206 occurrences combined, confirmed live 2026-09-06) — a naming choice from before the
conceptual/logical/physical layer split above was established, when "physical" hadn't yet been
claimed as a layer name and so its reuse as a sphere name wasn't yet a collision. Not part of the
three-sphere standard above, and deliberately not migrated: retiring it would mean renaming ~200
real, already-deployed HA entity_ids across both houses (dashboards, history, automations), a
separate migration in its own right, not a side effect of this naming correction. Its own
description at the time was "implementation detail... not normally exposed as a user-facing
entity" — close enough in spirit to infrastructural that folding it there is the likely eventual
target, whenever that migration is actually undertaken.

---

## 4. Three identities of a device

A physical or logical object carries three distinct identities that must not be conflated:

1. **Domain identity** — stable, technology-independent conceptual identity, e.g. `GroundFloor.Kitchen.Ceiling.Light`.
2. **Platform identity** — assigned by whichever platform hosts it, e.g. a Z-Wave `{node, endpoint}` pair, a Zigbee IEEE address, an FHEM device name. A device may have a platform identity on *more than one* integration; unless disambiguated, proto-entities from same-named devices across integrations are unioned by default (with a warning on unexpected overlap). A specific integration can be forced with an `@` suffix: `<name>@<integration>`.
3. **Bus identity** — the stable identity used on the event bus, e.g. `lighting/kitchen/ceiling`. Applications talk to each other through bus identity, never platform identifiers.

---

## 5. The canonical event bus

MQTT is not merely a transport; it is the semantic integration layer between systems:

```
Below MQTT:  protocol-specific world        Above MQTT:  semantic house world
             FS20, Z-Wave, Zigbee,                       lights, temperature,
             EMS, Netatmo                                heating, security, energy
```

A canonical property describes *what* a device represents, never *how* it's transported:

```yaml
WindowSensor
  reports:
    - OpenClosed
    - BatteryLevel
    - Tamper
```

Each observable property binds to a transport via an explicit **EventEndpoint**, so the semantics of a property are separated from how it's currently communicated:

- identifier
- state topic / command topic / availability topic
- payload converter
- QoS, retain flag
- direction (produce/consume/both)
- producer(s) and consumer(s)

Today the transport is always MQTT; the EventEndpoint abstraction is what would let a future transport (WebSocket, REST, OPC-UA, Kafka, ...) be substituted without touching the domain model.

Two distinct namespaces exist on the bus at any point: a **raw discovery** tree per source protocol (Z-Wave, EMS, Zigbee2MQTT, ...) and a **refined/conceptual discovery** tree produced by the coordinator. Naming in the refined tree is derived from each entity's/device's position in the DSL's space hierarchy, not from the source protocol.

---

## 6. The coordinator

The coordinator is a model-aware runtime component sitting between the integrations/infrastructure and the Home Assistant instance(s). It is **not** a collection of Home Assistant automations, and it is **not merely a discovery refiner** — that's one of three roles.

### 6.1 Three roles

**1. Local infrastructure coordination.** Interprets availability observations and propagates their consequences via the model's dependency graph. It does **not** sense availability itself — that's delegated to other components (ping/host checks, MQTT "are you alive" mechanisms, Zigbee2MQTT/Z-Wave JS health info, or an active probe — see below). Example: `protocols-server-1` goes down → everything Zigbee2MQTT and Z-Wave JS depend on becomes unavailable, propagated per the model rather than per-device.

**2. Discovery refinement.** Some integrations already speak HA MQTT Discovery (Zigbee2MQTT, Z-Wave JS). These publish to a raw discovery namespace; the coordinator applies the model to produce refined discovery (correcting `default_entity_id`, `name`, device metadata, and other model-defined properties, while retaining the rest of the technical description from the source). Integrations that don't natively speak discovery (e.g. `fhem2mqtt`) can instead be model-aware from the start and skip the refinement step entirely — there's no requirement that every producer pass through refinement.

This is also why discovery is preferred over generator-authored YAML MQTT template sensors wherever a producer already speaks it: Z2M and Z-Wave JS already publish rich device metadata (manufacturer, model, `device` grouping, availability, unit/class hints, ...) as part of their raw discovery payload. Refinement lets the coordinator correct and reposition that metadata; a hand-authored YAML template sensor would instead have to reconstruct all of it from nothing, and go stale the moment the source integration's own metadata changes.

**3. Cross-home federation.** Selected entities/devices are cross-posted from one house's MQTT bus to another's, over MQTT, with lineage preserved so the receiving house can tell an entity isn't actually local. See the Netatmo example below.

### 6.2 Model, observations, projections

```
                    MODEL
                      |
                      v
                +-------------+
 Observations ->| COORDINATOR |
                +------+------+
                       |
                 projections
                       |
          +------------+-------------+
          |            |             |
          v            v             v
      discovery   availability   federation
```

- **Model**: what entities/devices exist, how they're named, where they're located, which integration provides them, what infrastructure they depend on, which home owns them, what's exposed cross-home, and their lineage.
- **Observations**: externally-supplied facts (`host UP`, `gateway DOWN`, `device unavailable`, `MQTT connection alive`, `integration healthy`) — the coordinator consumes these, it doesn't implement the sensing mechanisms.
- **Projections**: MQTT-level output derived from model + observations — refined discovery, availability state, cross-home representations, state updates, lineage.

An example of a liveness *observation* source worth reusing directly: Zigbee2MQTT's active probe. Publishing an empty payload to `zigbee2mqtt/bridge/request/health_check` gets a genuine "is it actually alive and processing" answer on `zigbee2mqtt/bridge/response/health_check` (`{"data":{"healthy":true},"status":"ok"}`), as opposed to the passive `bridge/state` LWT flag which only reports "did it disconnect." Same `bridge/request/*` → `bridge/response/*` pattern is reused elsewhere in Z2M (`permit_join`, `restart`, `networkmap`, ...).

### 6.3 Device aggregation

Sometimes an infrastructural device has no built-in power switch or temperature sensor of its own; an external device supplies that capability instead. Rather than special-casing this as a discovery quirk, model it explicitly as **device aggregation**: a logical device composed from multiple physical/source devices, each contributing one or more capabilities.

```
Physical/source devices
  Smart plug ────────────── power state/topic ─────┐
                                                     │
  External temperature sensor ─ temperature topic ──┤
                                                     ▼
                                             Coordinator
                                                     │
                                                     ▼
                                      Logical infrastructure device
                                             "Living Room Heater"
                              ┌──────────────────────┴──────────────────┐
                           power                                     temperature
                              ▼                                         ▼
                         HA entity                                  HA entity
```

Sketch of the aggregation concept:

```yaml
devices:
  living_room_heater:
    type: heater
    name: Living Room Heater
    capabilities:
      power:
        source: smart_plug_living_room
        topic: state
      temperature:
        source: temperature_sensor_living_room
        topic: temperature
```

`smart_plug_living_room` and `temperature_sensor_living_room` remain separate source devices internally; the coordinator presents them as one logical device, and understands that `temperature_sensor_living_room` need not be a top-level conceptual element in its own right — it can be purely a component of another one.

Aggregation must work **both directions**:

```
state:     smart_plug/state ──┐                    commands:   logical_device/power/set
           temperature/state ─┴──> logical_device/power                    │
                                                                             ▼
                                                                        coordinator
                                                                             │
                                                                             ▼
                                                                     smart_plug/power/set
```

So the coordinator maintains a small topic transformation/routing graph, not just a fan-in.

This composes cleanly with incremental migration: a logical device can keep exposing the same conceptual identity while individual capabilities move source (e.g. `power` still comes from the old infrastructure while `temperature` has already moved to the new one) — HA never needs to know a migration is in progress. Aggregation is explicitly **many-to-one**: a logical device could combine a smart switch, an external temperature sensor, a power meter, and an occupancy sensor, all as one conceptual infrastructural element.

### 6.4 Legacy-to-conceptual passthrough

A directly reusable coordinator capability — useful to anyone adopting this model-driven approach on an existing installation, not specific to any one migration: the coordinator can be configured to cross-post every discovery message it sees on a "legacy" (raw, protocol-native) topic tree straight through to the "conceptual" (refined, model-driven) topic tree, unchanged, by default.

This means adopting the architecture never requires a flag-day cutover. Entities move to proper model-driven refinement one at a time — as each is dealt with, the coordinator gains a real decision to make for it (per §6.1's discovery-refinement role) and stops naively passing that one entity through. Everything not yet touched keeps working exactly as it did before the coordinator existed at all.

This is a coordinator *setting* — an explicit passthrough default — not a special case coded into the model. It's the same raw-discovery → refinement pipeline from §6.1, just with refinement defaulting to identity until the model has something to say about a given entity. For example: migrating an existing Zigbee2MQTT deployment onto this architecture doesn't require redefining every device in the model on day one — the coordinator defaults to passing Zigbee2MQTT's raw discovery straight through, and devices move to true model-driven refinement gradually, one at a time.

### 6.5 Worked example: Netatmo cross-home federation

Netatmo is the sharpest example of why federation needs explicit lineage: there is one Netatmo account, but devices physically belong to different houses.

```
Netatmo Cloud
      │
      ▼
HA @ protocols-server-2         (integration-adapter role, §7)
      │  local HA entity + metadata
      ▼
RAW MQTT DISCOVERY
      │
      ▼
Coordinator @ Junglinster
      │  model + lineage
      ▼
Vienna MQTT bus
      │
      ▼
HA @ Vienna                     (consumer role, §7)
```

`protocols-server-2`'s HA instance obtains *all* Netatmo data from the cloud, including Vienna's devices — it publishes RAW discovery for what it locally knows, without needing to know final naming conventions or which house the data conceptually belongs to. The coordinator refines and federates, attaching lineage so Vienna's HA can establish the entity isn't actually local, e.g.:

```
sensor
  └── cloud
       └── smart_home@junglinster
            └── local
```

From Vienna HA's point of view, the entity is just a normal MQTT-discovered local entity — the cross-home, cloud-sourced nature is preserved in lineage metadata, not hidden, but also not intrusive to the consuming side.

### 6.6 Device attributes: constant vs. variable

Devices — and sensors — carry attributes of two different kinds, which must not be conflated:

- **Device attributes** describe the device itself: brand, model, firmware version, CPU load, CPU temperature, availability. These exist independently of what (if anything) the device senses or controls in the world.
- **World attributes** describe what the device observes or manipulates: the temperature as measured in a particular room, a door's open/closed state, a cover's position.

Within device attributes, a further distinction matters for how they're communicated:

- **Constant device attributes** — brand, model, serial number, and the like. Fixed for the device's operational lifetime. These belong in the MQTT discovery message itself (HA discovery's `device` block), published once at discovery time.
- **Variable device attributes** — CPU load, CPU temperature, availability, and the like. Change over time, so baking them into the discovery payload is wrong; they belong on the device's own attributes topic, updated as they change, the same way a world attribute would be.

This distinction anticipates a DSL capability not yet implemented: a `space` definition may come to refer to a device in a general sense (as it does today), or to one of its specific attributes — constant or variable, device or world — which then materializes as its own entity (a sensor, a binary sensor, ...). Exactly how this materializes — grammar, generator output, coordinator projection — is deliberately left open here; per §9.2, it will be discovered incrementally, integration by integration, rather than designed fully up front.

### 6.7 Entity domains with no MQTT discovery equivalent

Not every Home Assistant entity domain can be recreated via MQTT discovery — the mechanism §5's
canonical bus and every "hosts"/"discovery"-style integration this session built relies on. Cross-
checked (2026-08-23) against HA's own full platform list
(`homeassistant/generated/entity_platforms.py`, home-assistant/core) versus the MQTT integration's
documented discovery-supported platforms. Confirmed missing, and practically relevant to a house
(i.e. plausible things a real integration might need to bridge): `weather`, `sun`, `media_player`,
`calendar`, `geo_location`, `air_quality`, `image_processing`, `remote`, `todo`. Also missing, but
unlikely to ever matter for house bridging: the voice-pipeline-only domains `stt`, `tts`,
`conversation`, `assist_satellite`, `ai_task`, `wake_word`.

For anything in the first list, MQTT *discovery* is the wrong tool regardless of how the source
data is obtained — there's no `weather.*`/`sun.*`/`media_player.*`/etc. entity to construct on the
receiving instance. The workaround, worked out this session for weather/sun/media_player: decompose
the source entity's attributes into ordinary `sensor`/`binary_sensor` entities instead (each of
those domains *is* MQTT-discoverable), rather than trying to recreate the native domain. This is a
complete substitute for read-only domains (weather, sun) — nothing is lost. For domains with
control surfaces (media_player, and to a lesser extent cover-like remote-control domains),
decomposition only covers the read side; reconstructing control means separate writable MQTT
entities (`switch`/`number`/`select`/`button`) each mapped back to a service call on the source
instance, at the cost of losing the domain's single unified UI card — a real design trade-off to
make deliberately per case, not a detail to paper over.

A related, structurally different case: entities that already exist as fully-formed entities on
*another* HA instance (not raw physical/protocol data at all) shouldn't be bridged via MQTT
discovery either, even for domains MQTT *does* support — see §6, "coordinator-based warning"
design note (PROJECT.md) for why this calls for a conceptual-to-conceptual mechanism rather than
the physical-layer discovery path "hosts"/"discovery" use. That mechanism is PROJECT.md 1.1's
entity-existence inquiry design (2026-08-28) — per-entity generated reporting/command automations
plus a coordinator-side inquiry loop, *not* HA's `mqtt_statestream`/`event_stream` (both classified
legacy in current Home Assistant, ruled out after this section originally cited statestream as the
model to follow).

### 6.8 Typing-metadata defaults: `Defaults.def`, never coordinator-invented

Every entity the coordinator discovery-publishes carries HA typing metadata — `device_class`,
`unit_of_measurement`, `state_class`, `icon`. This metadata is **always resolved by the
generator, never invented by the coordinator at runtime.** The coordinator only ever relays a
value the generator already computed into `devices.yaml`/`homeassistant_bridge.yaml`; it holds no
lookup table of its own for what a "temperature" or "node" capability's typing should be.

Resolution order, applied uniformly wherever an entity's typing metadata is determined (per-
capability lines in the unified `home_assistant`/`hosts` device grammar, a "hosts" device's node
entity, a "hosts" device's hardwired attribute typing):

1. **Explicit, per-capability metadata** — a capability's own `with: device_class: "...";
   end;` clause, or the older bare `<path> device_class: "...";` trailing-line form. Always wins.
2. **A `defaults: for <domain>.<pattern>: ...; end;` rule** — user-declared, in
   `Shared/Definitions/Defaults.def` (shared across houses; a flat file, no `physical layer with:`
   wrapper needed, same convention as the shared `Settings.def`) or a house's own `Physical.def`.
   `<pattern>` may carry a leading wildcard segment (`*/temperature`) matching any number of
   leading path segments by suffix, or be a bare exact match; one `for` line may name several
   space-separated `<domain>.<pattern>` targets sharing one body. This is the intended, elegant
   place to add or override a default — see `capability_defaults.go`
   (`resolveCapabilityDefaults`/`capabilityDefaultsFor`/`matchesCapabilityPattern`).
3. **A code-level postfix fallback** (`postfixCapabilityDefaults`, `taxonomy.go`) — a small,
   deliberately shrinking table, consulted only for subdomains not (yet) covered by a `defaults:`
   rule. Most of what used to live here (`DeviceClassOf`, `restSensorSubdomainProps`,
   `binarySensorDeviceClassBySubdomain`) has already been migrated into `Defaults.def`; only a
   handful of domain-agnostic icon-only subdomains (`subdomainIcons` — `consumes`, `daylight`,
   `radio`, ...) remain, since they don't fit a single settled domain to write a `defaults:` rule
   against.

### 6.9 Entity-existence discovery always needs a seed

PROJECT.md 1.1's entity-existence mechanism (three-state known-to-exist / not-known-to-exist /
known-not-to-exist tracking, paced ≤1/minute-per-instance inquiry) can *enrich* a device it already
knows about — an inquiry reply about any tracked entity also carries `device_entities()` for its
device, and any sibling entity that reply reveals but the tracker didn't already know about is
folded in as newly known-to-exist (`DiscoverSiblings`, `house_event_bus_coordinator/entity_existence.go`).
That only works because there's an *anchor*: some already-tracked entity belonging to the same
device, obtained from HA's own device_id/device_entities() registry functions.

This is a structural limit, not a missing feature: **there is no way to discover an entirely new,
never-declared device without either a seed or a scan.** A periodic full-instance enumeration
(`{% for s in states %}`-style) is the thing 2026-08-27's incident already ruled out — it's
synchronous and O(every entity on the instance), unsafe at any real scale, which is exactly why the
paced one-entity-at-a-time design exists in the first place. An RPC-style "ask the instance to list
everything it has, right now" call is the other theoretical option, and is rejected for the same
reason from the other direction: it reintroduces a request whose cost scales with instance size,
just wrapped differently — no better than the automation it would replace, and meaningfully more
moving parts (a new protocol on top of the existing paced one, not a variation of it).

**Chosen resolution: a human-provided seed, not an automatic one.** The coordinator discovery-
publishes a single MQTT `text` entity on "main" (HA's `text.mqtt` platform, optimistic — no
`state_topic`, since nothing needs to report a value back) whose `command_topic` the coordinator
itself subscribes to. A person types a fully qualified entity_id into that field when they know
(from checking the remote instance directly) that something new exists but the DSL doesn't know
about it yet. Two accepted input shapes: a bare entity_id is inquired about on *every* declared
instance (it doesn't say which one it belongs to, and asking all of them is cheap); an
`<instance>: <entity_id>` prefix (colon, then exactly one space — never a valid substring of an HA
entity_id, so the split is always unambiguous) routes it to that one instance only. Either shape may
be repeated, separated by `;`, in a single text-field submission (`splitDiscoverEntityRequests`,
2026-08-29) — pasting a whole list of entities, one per line each ending in `;`, some bare and some
instance-prefixed, seeds and inquires about every one of them from one write instead of needing one
submission per entity; one unrecognised entry in the batch is logged and dropped without aborting
the rest. Either way the coordinator treats each request as a fresh seed (no owning DSL device yet)
and inquires about it immediately —
bypassing the normal per-instance pacing, since this is an explicit, rare, human-triggered action,
not the automatic loop the pacing exists to protect the instance from. Once that seed resolves, the
ordinary `DiscoverSiblings` mechanism takes over exactly as it does for any other anchor, surfacing
the rest of that device's entities. Since a manually-seeded anchor has no DSL-declared device to
attribute those siblings to, `DiscoverSiblings` mints a stable synthetic id (`hass.discovered_<slug
of the remote instance's own device name>`) and assigns it to the anchor and every sibling it finds
— so the resulting suggestion entry still renders as a proper `device hass.discovered_... with:
...; end;` block, not dumped into an "no known device grouping" section, and stays stable across
later inquiry rounds for the same device.

The slug alone isn't a safe uniqueness key, though — **confirmed live 2026-08-29**: two genuinely
separate Netatmo modules at the same physical location ("Outdoor Module" and "Wind Gauge") were
both auto-named "Vienna Terrace" by Netatmo/HA, and the old name-only minting collapsed both
modules' entities into one synthetic device. Fixed: `resolveSyntheticDeviceID` keys uniqueness off
the remote instance's own `device_id` (carried in the inquiry reply's `device_id` field, already
used for `device_entities()`/`device_attr()` — just not previously threaded into `DiscoverSiblings`
itself), tracked per entry as `RemoteDeviceID`; reusing an already-minted id for the *same* remote
device_id, and disambiguating with a numeric suffix (`_2`, `_3`, ...) when a *different* remote
device_id would otherwise produce the same name-derived slug — mirroring how the remote instance's
own entity naming already disambiguates same-named siblings (`_2`).

*Implementation note*: this meta entity's own discovery topic must be listed as "expected" in
`main.go` alongside every device/discovery/hassbridge-derived topic — otherwise
`watchForOrphanedDiscoveryTopics` (a live, ongoing watcher, not just a startup sweep) sees it as an
unrecognised "coordinator"-owned topic the moment it's published and retires it again immediately.
Hit and fixed live 2026-08-28: the entity was published but never actually stayed up long enough
for HA to show it.

This is a deliberate trade: bootstrapping a new device is no longer fully automatic (the old
manifest mechanism's one genuine advantage), in exchange for the paced mechanism never being able
to overload an instance regardless of how many entities it has — the same trade the whole
entity-existence design already made everywhere else.

### 6.10 Existence tracking anchors on the physical layer's own declared *source*, per kind

**Layering principle** (the general rule this section's checks are an instance of): anything
declared at the conceptual layer is defined one of three ways — in terms of other, already-existing
conceptual entities; assumed to exist as a native entity on the main HA instance directly (never
imported from anywhere); or defined in terms of an entity the physical layer (or, later, the
logical layer) provides. Existence-checking only ever concerns that third case — and what it
checks is never the conceptual-layer name, always the *source* specification the physical layer
declared it in terms of (a capability line's right-hand side, e.g. `sensor.outdoor_temperature:
sensor.boiler_outdoortemp;`'s `sensor.boiler_outdoortemp`, or
`sensor.production/current/power: sensor.inverter_122325122653;`'s
`sensor.inverter_122325122653`) — the conceptual-layer name on the left is always the generator's
own choice and is never in question.

This reframes what each integration kind's check actually needs, and corrects an earlier version of
this section that wrongly concluded kind-2 needs no existence-tracking machinery at all (see below).

- **Kind 1 (hosts)**: no separate tracking needed — a *direct* link. The coordinator's own scripts
  publish these entities' state themselves, so there is no "does the source exist" question
  distinct from "is it currently reporting," which the existing liveness/ping mechanism already
  answers.
- **Kind 2 (discovery — Zigbee2MQTT, Z-Wave, EMS-ESP)** and **kind 3 (`home_assistant` bridge)**:
  both need the coordinator to maintain and report an up-to-date three-state status
  (known-to-exist / not-known-to-exist / known-not-to-exist) for every declared source entity —
  kind-2's `TDiscoveryEntityLink.Leaf` (the gateway's own leaf identifier), kind-3's
  `bareEntityFromSource(cap.SourceEntity)` — exactly mirroring what
  `house_event_bus_coordinator/entity_existence.go` already does for kind-3. Without this, the
  generator has no existence signal at all for a newly-referenced source when it runs offline — not
  even kind-3's old gap (a bare assumption), but a complete blind spot for kind-2 today.
- **Where kind-2 and kind-3 differ is *how* the coordinator learns the status, not *whether* it
  tracks it**: kind-3 needs active, paced inquiry (PROJECT.md 1.1) because a remote HA instance
  never self-announces anything to the coordinator unprompted. Kind-2 needs no inquiry at all — the
  gateway's own native HA MQTT discovery payload already *is* the existence claim, and its
  retraction (an empty/retracted payload on the same topic) *is* the non-existence claim, both
  observed passively as a byproduct of `discoverybridge.go`'s existing subscription. "We trust the
  advertiser" still holds for kind-2 (no independent verification round, the gateway's own report is
  authoritative) — but *trusting* the signal and *tracking/reporting* it to the generator are two
  different things, and only the first was true of the original (corrected) version of this section.

**Status (2026-08-29): kind-2 passive tracking built** — see PROJECT.md 1.8. Coordinator side
(`discovery_existence.go`): a per-gateway three-state tracker seeded from `discovery.yaml`'s
`EntityLinks` (not-known-to-exist until observed), populated by `discoverybridge.go`'s existing
discovery-payload handler (arrival → known-to-exist, no new subscription), published to
`discovery_gateways/<gatewayID>/existence/state` (retained, local + cloud unconditionally, mirroring
kind-3's `publishStatus`). Generator side (`mqtt_discovery_existence.go`): fetch/cache mirroring
kind-3's `fetchEntityExistence`, and `checkDiscoveryKnownNotToExistErrors`
(`checkKnownNotToExistErrors`'s kind-2 counterpart), wired into `Physical_Generator.go` right after
the kind-3 check. Confirmed via a real `./generate`: soft-fails gracefully offline (no broker
reachable, no cache yet) exactly like kind-3, generation still succeeds. First-phase (arrival-only)
tracking deployed live and confirmed clean (no errors on deploy/generate).

**Retraction (known-not-to-exist) also built, 2026-08-29 (same day, corrected mid-build)** — an
earlier draft of this section wrongly claimed this needed a topic→identity map the coordinator
"doesn't keep and can't build cheaply." That was wrong: the handler already has both the topic and
the decoded `(gatewayID, leaf)` together at the moment any real payload arrives, so it costs nothing
to remember `topic → (gatewayID, leaf)` as it goes (`RecordTopicIdentity`) — no new subscription, no
persistence needed (discovery config topics are retained, so a coordinator restart's own subscribe
naturally replays every gateway's current config before any new retraction could arrive, making the
map self-healing in memory alone). An empty payload on a topic (HA's own MQTT discovery removal
convention) resolves via that map (`MarkRetracted`) and moves the leaf to known-not-to-exist. The
generator-side check (`checkDiscoveryKnownNotToExistErrors`) needed no change at all — it was
already written expecting all three states.

**Suggestion report also built, 2026-08-29 (same day)** — the positive counterpart to the
known-not-to-exist check, mirroring kind-3's `generateEntityCatalogueSuggestions`/
`buildSuggestionReportFromExistence`: `generateDiscoverySuggestions`/
`buildDiscoverySuggestionReport` (`mqtt_discovery_existence.go`) write
`suggestions/discovery.txt` — one combined file (not one per gateway, since kind-2 has no grouping
above "gateway" the way kind-3 groups by instance), one copy-paste-ready
`device discovery.<gatewayID> with: ...; end;` block per gateway with anything known-to-exist but
not yet claimed by a `TDiscoveryEntityLink`. Domain/suffix guessing reuses kind-3's
`recognizedCapabilityKeywords` table but falls back to `sensor` (never the bare leaf name) when
nothing matches — confirmed live against EMS-ESP: ~150 leaves surfaced, most `# not recognized`
(EMS-ESP's protocol-level field names rarely match kind-3's room/measurement-shaped keywords), a
few correctly recognized (`system_uptime` → `sensor.uptime`) — a useful scaffold to hand-edit from,
not a fully-automated result, same expectation kind-3's suggestions already set.

**Cloud topic collision, found and fixed live 2026-09-05**: both `discovery_gateways/<gatewayID>/existence/state`
and kind-3's `homeassistant_instances/<name>/existence/state` were published to the CLOUD broker
under the same bare topic the local broker uses — fine for the local broker (each house only ever
reads its own), but wrong for the shared cloud one: every house's own primary instance is
conventionally named "main", and Junglinster's and Vienna's own retained status were confirmed live
to be overwriting each other on `homeassistant_instances/main/existence/state`. Fixed by prefixing
the CLOUD copy only with the owning installation's own name (`<installation>/discovery_gateways/...`,
`<installation>/homeassistant_instances/...`), matching the whole-topic-prefix convention already
used everywhere else a topic crosses onto the shared cloud broker (`<installation>/homeassistant/...`
discovery configs, `<qualifier>/homeassistant_instances/.../bridge/.../state` below) — not the
`hosts`-kind's different "insert after first segment" convention, which is specifically driven by
that kind's client-ID-scoped cloud ACL and doesn't apply here. The local copy stays bare (unchanged).
Generator-side readers (`fetchEntityExistenceFromBroker`/`fetchDiscoveryExistenceFromBroker`,
`mqtt_entity_existence.go`/`mqtt_discovery_existence.go`) construct the same qualified topic when
reading via the cloud broker, using `ctx.Installation` (this house's own name) — never a different
house's, since every instance/gateway a house's own Physical.def declares is, by construction, that
same house's own.

### 6.11 The coordinator's own runtime state must survive a deploy, not just a restart

The coordinator persists three files into its own working directory (`coordinatorDir`, the CLI
argument the systemd unit passes) alongside the generator-authored YAML config it reads from the
same directory: `entity_existence.json` (kind-3 tracking, §6.9/§6.10), `discovery_existence.json`
(kind-2 tracking, §6.10), `discovery_topics.json` (`discoverycleanup.go`'s own discovery-topic
manifest, pre-dating both). All three exist to survive a coordinator *restart* — that's the whole
point of building persistence for them at all (PROJECT.md 1.1/1.8's own "not yet exercised across a
restart" notes).

**Bug, found and fixed live 2026-08-29**: Junglinster's `deploy.d/04_deploy.coordinator` pushes the
local `coordinator/` directory (generator-authored YAML only — `devices.yaml`, `discovery.yaml`,
`homeassistant_bridge.yaml`, `discovery_cleanup.yaml`, `secrets.yaml`, plus the systemd unit and
`deploy.junglinster` script) to the *same* remote directory the running coordinator uses
(`/home/erikp/coordinator`, confirmed via `coordinator.service`'s `ExecStart`), using
`rsync --delete`. Since the three runtime state files are remote-only — the local source tree has
no reason to contain them — `--delete` silently wiped all three on *every single deploy*, right
before restarting the coordinator into a clean slate. This defeated the persistence-across-restarts
design outright: a deploy is exactly the kind of restart persistence exists to survive, and it was
the one case guaranteed to erase it. Confirmed as the explanation for an earlier live mystery: a
`hass.office_garden` entry that had been confirmed `known-to-exist` (via `DiscoverSiblings`,
Architecture.md §6.9) had inexplicably vanished from the coordinator's own published existence
status after a subsequent deploy — this bug is why.

Fixed: `04_deploy.coordinator`'s `--delete`-armed rsync now carries
`--exclude entity_existence.json --exclude discovery_existence.json --exclude discovery_topics.json`.
Any *new* coordinator-persisted runtime file must be added to that exclude list too, or it will
silently suffer the same fate the next time this bug's root cause recurs.

Where HA itself already supplies a sensible default icon from a `device_class` alone (most of
them — `connectivity`, `temperature`, `battery`, `power`, `energy`, ...), a `defaults:` rule
should *not* also set an explicit `icon:` — that's pure duplication, and worse, a second place
that can drift from HA's own choice. Only give an explicit icon where no `device_class` exists to
imply one (e.g. `door`/`window`/`water`, which HA does have real binary_sensor device classes for
but this project hasn't adopted yet, and a couple of the sensor domain's genuinely typeless
subdomains like `humidity`/`load`).

This resolution order took real effort to land uniformly: `resolveCapabilityDefaults` is the one
shared entry point every generator code path that seeds typing metadata now calls — including
older paths that predate `Defaults.def` entirely (customization files, `condition`/`has_state`
binary sensors, CLI-imported sensors, "hosts" integration attribute typing, and — the one
piece that had been missed on a first pass — a "hosts" device's own `node` (liveness) entity,
which the coordinator had hardcoded `device_class: "connectivity"` for directly, independent of
`Defaults.def`, until 2026-08-26). If a new code path ever needs typing metadata again, it must
call `resolveCapabilityDefaults` rather than inventing its own literal — the coordinator-side
`TDeviceConceptual.NodeDeviceClass`/`NodeIcon` fields (`devices.yaml`'s `conceptual:` block) are
the template for how a value crosses from generator to coordinator: resolved once, generator-side,
carried as a plain field, never re-derived or defaulted again on the coordinator side.

**Kind-2 (`discovery` integration) entities get the same coverage, with one deliberate twist.**
Zigbee2MQTT/Z-Wave/EMS-ESP-style gateways already publish their own complete, correct native HA
MQTT Discovery — they decided their own domain/device_class/unit themselves (§6.1). So for an
`entity <spec> from <gateway-id>.<local-name>;` link, `Defaults.def` acts purely as a
**gap-filler**: `resolveCapabilityDefaults` still runs generator-side
(`registerDiscoveryEntityLink`, `Conceptual_DiscoveryEntities.go`) and its result is still carried
into `discovery.yaml` (`entity_links:` block) for the coordinator to read — but
`discoverybridge.go`'s `buildRelayedDiscoveryConfig` only uses a field from `Defaults.def` when
the gateway's own decoded payload leaves it empty; a device_class/unit/state_class the gateway
already reported is never overridden. `icon` is the one exception with no such tension — the
decoded payload has no icon field to defer to at all (`decodeDiscoveryPayload`'s "extend on
demand" abbreviation set doesn't cover it), so it always comes from `Defaults.def` when set. This
keeps the "coordinator never invents/overrides typing on its own" rule intact while still
respecting that a kind-2 gateway's own report is more specific and trustworthy than a generic
postfix guess.

**Kind-2's own device grammar** (redesigned 2026-08-26, see
[[project_kind2_discovery_device_hierarchy]] in memory for the full worked example): a
`discovery` gateway's own devices are declared with the same nested `device <id> with: ... end;`
grammar the `home_assistant`/`hosts` integrations use, to arbitrary depth
(`integration_discovery_parser.go`'s `parseDiscoveryDeviceBody` is genuine recursive descent,
unlike every other single-level device body parser in this codebase) — modelling a real gateway's
own device hierarchy directly (e.g. EMS-ESP's `ems-esp` root with `ems-esp-boiler`/
`ems-esp-thermostat`/`ems-esp-mixer` chained to it via `via_device`) rather than flattening it
into one bulk `identifiers "<value>";` list. Each device gets an *inferred* default HA identifier
from its own DSL id (`discovery.ems_esp_boiler` → `ems-esp-boiler`), so declaring one whose real
identifier already follows that convention needs no explicit `identifiers` line at all — it
survives only as an override for the cases that don't. Superseding the older flat design turned
out to need **no coordinator-side changes**: `matchingGateway`'s existing one-hop `via_device`
check already treats a child device's payload as belonging to its own declared identifier
regardless of how the DSL models the hierarchy — this was purely a generator-side
(Physical.def-parsing + `discovery.yaml`-shape) change.

Each (sub-)device declares its own capabilities with the same `<domain>.<local-name>: <source>;`
line shape as `home_assistant`/`hosts` devices — e.g. `sensor.outdoor_temperature:
sensor.boiler_outdoortemp;`. `<local-name>` (`outdoor_temperature`) is the DSL-authored, stable
name Spaces.def actually references (`entity <spec> from <gateway-id>.<local-name>;`); `<source>`
is only a *reference* used to resolve the gateway's own raw, unstable leaf `unique_id` to match
live traffic against — a domain-prefixed source (`sensor.boiler_outdoortemp`, the same shape as
the gateway's own reported `default_entity_id`) has that prefix stripped
(`bareDiscoveryLeaf`) down to the bare `unique_id` (`boiler_outdoortemp`) the coordinator
actually matches on. This is deliberate: unlike `home_assistant`/`hosts` sources (a real HA
entity_id, resolved once and never expected to change), a kind-2 gateway's own leaf naming is
externally owned and can drift — Spaces.def must never reference it directly.

A per-leaf **explicit override** (the `hass.envoy`-style "the DSL author knows better than either
the default or the gateway" case) is not yet built for kind-2 — a capability line has no
`with: ...;` clause of its own today, unlike the unified `home_assistant`/`hosts` device
grammar's per-capability metadata. Flagged, not implemented (2026-08-26).

### 6.12 `native`/`export`: cross-posting a device onto the cloud broker (PROJECT.md 1.2a)

**The `local` routing keyword was renamed `native`, 2026-08-30.** `parseRoutingKeywords`
(`mqtt_routing_keywords.go`) still recognises the same two trailing tokens on a "hosts" device
line (`cloud`/`native`), same semantics as before (§ device declarations, `integration_hosts_*`),
just renamed to stop colliding with this codebase's other, unrelated use of "local" to mean the
local/main MQTT broker itself (used constantly throughout this file and `mqtt_relay.go`). Purely a
rename — `THostDevice.Native`/`TDevice.Native` (was `.Local`), `devices.yaml`'s `native:` field
(was `local:`) — no behaviour changed.

**`export` gives a `home_assistant`-bridge device (kind-3) the same cross-broker reach a
`cloud`+`native` hosts device already has**, but the mechanism differs because a bridged entity's
reporting automation always publishes locally (`homeassistant_instances/<name>/bridge/<local
entity>/state`, on this house's own main broker) — there is no equivalent of `cpu/report` choosing
which broker to target, so the *coordinator itself* has to be the one relaying local→cloud, not
the device's own report script. Declared as `device <id> export with: ...; end;`
(`integration_hassbridge_parser.go`/`_storage.go`), propagated to
`coordinator/homeassistant_bridge.yaml` as `export: true`
(`integration_hassbridge_generator.go`), read coordinator-side by
`house_event_bus_coordinator/discoveryhassbridge.go`.

For an exported device, `subscribeHassBridge`/`subscribeHassBridgeDeviceInfo` do two things beyond
the always-on local/main publish, both no-ops when no cloud broker is configured:

1. **Raw value cross-post** (`crossPostHassBridgeToCloud`): each entity-state and device-info
   message gets forward-published (retained) onto the cloud broker at
   `<installation>/<bare local topic>` — a plain topic-prefix qualification, since the coordinator
   (not a per-client-ACL-scoped device script) is the publisher here, unlike `qualifyHostsTopic`'s
   ACL-preserving mid-topic insertion.
2. **A separate, genuinely functional discovery config**, published to the cloud broker at the same
   installation-qualified topic convention every cloud-routed discovery config uses. Deliberately
   **not** a copy of the local payload: its `state_topic` field points at the cross-posted cloud
   topic from (1), and it carries an extra top-level `"installation"` field — both needed because,
   unlike the hosts `cloud`+`native` mechanism's cloud-side discovery (documented in `mqtt_relay.go`
   as a **non-functional catalogue entry** whose embedded `state_topic` still points at a bare local
   topic nobody outside this house can reach), this one has no separate "cloud"-routed origin to
   defer to — the coordinator *is* the origin, so nothing stops the payload from being immediately
   usable by a foreign subscriber. The `"installation"` field exists because `device.Identifiers`
   itself is only the bare local device id (e.g. `hass.office_garden`), which could collide with
   another exporting house's identically-named device on the same shared cloud broker.

The explicit design goal (2026-08-30): this payload must be **rich enough for a future importing
coordinator to reconstruct the device + entity + topic map on its own**, and to run the same
kind-2/kind-3-style existence tracking and suggestion generation against it that this coordinator
already runs against its own sources (§6.9/§6.10) — without that importer needing a copy of this
house's Physical.def. Reusing the existing HA-MQTT-discovery-shaped payload (rather than inventing
a separate manifest format) is deliberate: an exported kind-3 entity is meant to look, wire-format
wise, indistinguishable from a kind-2 gateway's own native self-description, so a future importer
could in principle reuse its existing kind-2 discovery-payload-consuming code path
(`discoverybridge.go`) rather than needing a third, bespoke ingestion mechanism.

**`export` also implies "all entities used," even with no Spaces.def positioning at all
(PROJECT.md 1.2b, 2026-08-30).** Everything above only ever fires for a device with a
`DeviceConceptualLinks` entry — and until 1.2b, the only way to get one was an explicit Spaces.def
positioning (`entity device.<spec> from <device-id> with: all entities;` or the lighter `device
<spec> from <device-id>;` form, both Conceptual_DeviceEntities.go/Conceptual_DevicePositioning.go).
That's exactly backwards for the motivating case: Vienna's Netatmo readings (relayed via
protocols-server-2) have no reason to live anywhere in *Junglinster's* own Spaces.def at all — their
whole purpose is to be exported, not used locally — so without a fix, `export` alone would silently
register nothing.

Fixed: `registerExportedHassBridgeDevices` (`Conceptual_DeviceExportRegistration.go`), called from
`generatePhysicalIntegrationOutputs` right after `collectHassBridgeDevicesByID`, before
`generateHassBridgeFile`. For every `Export=true` device with **no existing**
`DeviceConceptualLinks` entry, it auto-registers one — one entity per declared capability — exactly
as if `device infrastructural:/<bare-id> from <device-id> with: all entities;` had been written at
the root (top-level) space. It calls `registerHassBridgeDeviceImpliedEntities` directly with an
explicitly-built `deviceIdentity`/`displayName` rather than going through
`registerDeviceImpliedEntities`'s own `administration.SpacePath`-based resolution, since `SpacePath`
is parse-time-only state that's no longer meaningful by the time this runs (after Spaces.def
parsing has fully finished); the spec's `infrastructural:/<bare-id>` shape has a leading `/` after
the colon specifically so `normalizeEntityFullName` treats it as absolute and ignores `SpacePath`
regardless. Once the link exists, `generateHassBridgeFile`/`generateInstanceAutomationTrees` pick
it up through their own already-existing `DeviceConceptualLinks` gate with no changes needed there
— so the source instance's per-entity reporting automations get generated too, automatically, for
free.

**Deliberately narrow scope**: only a device with *no* existing link at all is auto-registered here.
A device the DSL author already positioned (fully, or partially via the lighter form plus a few
per-capability lines) keeps exactly what was explicitly declared — `export` never retroactively
expands a manual positioning. Filling in the remaining capabilities under a *different*, synthetic
root-level identity would put them at a visibly different display-name/entity-id path than their
already-positioned siblings, which is worse than just leaving the gap for the DSL author to close
explicitly. A partially-positioned, exported device's un-positioned capabilities staying
unregistered is a known, accepted edge case, not solved here.

Verified live: the real, already-`export`-flagged 5 Vienna devices (none positioned anywhere in
Junglinster's own Spaces.def) now produce full `coordinator/homeassistant_bridge.yaml` entries and
per-entity reporting automations from a real `./generate`, confirmed by inspecting the generated
files directly.

**Not built yet, deliberately**: the consuming ("import") side for hassbridge devices — an
`integration import`-style declaration analogous to hosts' `ImportedFrom`, which would let one
house's Physical.def declare "pull this device from that installation's exported cloud topics."
PROJECT.md 1.2c's own "test re-import 'faking' Vienna import in Junglinster" is the concrete
scenario this was built ahead of; 1.2d is the import mechanism itself. Command topics
(open/close/stop, etc.) don't exist for hassbridge entities at all yet (PROJECT.md 1.6, still
undesigned) — nothing to cross-post there yet either; add it alongside state/device-info once
commands land, not before.

---

## 7. Home Assistant's two roles

A Home Assistant instance is not inherently the centre of the architecture — it's one possible implementation of a platform adapter, optionally combined with user-facing capabilities. It can fulfil either or both of two independent roles:

- **Integration role** — adapter between an external platform and the canonical bus. Typical for cloud-based integrations: Netatmo, Volvo, Google Calendar, weather/energy providers, vendor clouds. Imported entities are projected onto the bus using canonical event definitions/bus identifiers. This is also where the media-player exception lives: the main instance can locally integrate certain entities (in particular media players) and still bridge them onto the bus itself.
- **Consumer role** — subscribes to the canonical bus and reconstructs entities from the event stream. Automations, dashboards, scripts, and visualisations operate *exclusively* on these reconstructed entities, regardless of whether they originated from Z-Wave, Zigbee, Matter, FHEM, another HA instance, or a custom app.

Deployment is independent of this logical split. The two roles may run on separate instances:

```
                   Canonical Event Bus
                          |
      +-------------------+--------------------+
      |                                        |
 Home Assistant                       Home Assistant
 (Integration)                     (Automation / UI)
      |                                        |
 Netatmo, Volvo, ...             Dashboards, Automations
```

...or the same instance can play both roles at once:

```
                   Canonical Event Bus
                          |
                  Home Assistant
         (Integration + Automation + UI)
```

Conceptually these are identical; only the deployment topology differs. This decentralisation of Home Assistant — deliberately not the single hub — is what makes it fair to call this a **Canonical Event-Driven Home Automation Architecture (CEDHA)**: the canonical model and the event bus form the actual centre, and every platform (including HA itself) is an adapter to it. That independence is the direct payoff: swapping Z-Wave for Matter later only touches the platform-specific transformation — canonical model, bus identities, HA entities, automations, and any other bus consumer (digital twin, analytics, ...) stay untouched.

---

## 8. System components ("two apps")

- **coordinator** — the runtime component described in §6: connects to the local MQTT broker and any bridged external brokers, performs refinement/aggregation/federation continuously.
- **generator** — compiles the high-level DSL specification into:
  - YAML for the main Home Assistant instance(s)
  - definitions (YAML) for the coordinator
  - definitions/specifications per integration (e.g. FHEM's MQTT connection config, a host-ping table, a second HA instance used purely to bridge a cloud service)

The main HA instance can simultaneously act as an integration too (again, chiefly for media players) — the generator needs to account for that dual role rather than assuming a clean split always exists.

---

## 9. Codebase & generator strategy

- Coordinator and generator are likely a **split codebase** (different runtime characteristics: coordinator is a long-running process reacting to MQTT; generator is a batch process).
- The generator **evolves from the existing one** rather than a ground-up rewrite — see the incremental migration plan in `PROJECT.md`.
- The generator should have **integration-specific source code per integration**: parsing that integration's source specification, storing the resulting data, and generating that integration's specific output. A bridge to another MQTT broker is itself just a special (bidirectional) kind of integration, not a separate mechanism.
- **Introduce an explicit intermediate semantic model** inside the generator now, before it's strictly necessary, because it will inevitably be needed. Today the generator is roughly:

  ```
  DSL
     │
     ▼
  Home Assistant YAML
  ```

  Target shape:

  ```
  DSL
     │
     ▼
  Semantic Model
     │
     ├── Validator
     ├── Documentation
     ├── Dependency Graph
     ├── MQTT Generator
     ├── HA Generator
     ├── ArchiMate Generator
     └── ...
  ```

  The semantic model becomes the stable internal API; every output (including ones that don't exist yet, like a dependency graph or an ArchiMate view) plugs into it rather than being derived ad hoc from the DSL text.

  Two principles the eventual semantic-model refactor must honor, stated now so new code doesn't keep drifting further from them:

  - **Read-then-work.** `Main.def` declares the *set* of definition files for a house; there is no significant order among its `include` lines. Everything gets read in and merged into one flat pool (macros, settings, variables, entity/space/physical declarations, ...) first; only afterward does interpretation/generation ("going to work") begin. No `include` line's meaning should depend on another `include` line's position.
  - **File names carry no semantics.** What a `.def` file *is* — settings, secrets, entities, physical declarations, whatever — is determined by its *content* (the directives/keywords inside it), never by its filename. `Entities.def`, `Physical.def`, `Secrets.def` etc. are conventional names for human readability, not something the parser is entitled to depend on. A `.def` file could in principle be split, merged, or renamed without changing what it means.

  Known gap against both principles in the current implementation: it is thoroughly filename-driven. `generateFromPaths` reads `Spaces.def`/`Settings.def` by hardcoded name; `resolveMainIncarnationName` reads `Physical.def`/`Settings.def` by name. Each is also its own separate read pass, interleaved with (and in the incarnation-name case, *before*) entity parsing, rather than one unified read-everything-then-interpret pass. Don't grow more filename-keyed side-channel readers in the meantime — this is the concrete shape the semantic-model layer above needs to dissolve.

  **A second, related known gap, found 2026-08-20**: the "hosts" integration's generated `coordinator/devices.yaml` (`integration_hosts_generator.go`) is itself pre-semantic-model scaffolding — an ad hoc, hand-grown YAML shape (`capabilities`, `conceptual`, `topic`, ...), not an implementation of the §5 **EventEndpoint** concept it should really be. Concretely: the `cpu/<host>/state` MQTT topic convention was found hardcoded independently in two places (`generateReportingAutomations` and the deployed `Integrations/cpu/report` script) with no shared source of truth — patched by centralising it in `integration_hosts_storage.go`'s `THostsEntityMaterialization.StateTopicTemplate`, but that's still a topic string bolted onto an ad hoc struct, not a real EventEndpoint (identifier, state/command/availability topic, payload converter, QoS, retain, direction, producer(s)/consumer(s)). Every field added to `devices.yaml` from here is debt against the eventual EventEndpoint-based semantic model, not progress toward it — acceptable for now since the coordinator itself is still a stub with no real consumer forcing the issue, but don't let `devices.yaml`'s shape keep growing ad hoc once the coordinator becomes real; that's the point at which this should become an actual EventEndpoint model instead.

  `Server.def` has been removed: its `main <house> <url>;` directive is now fully subsumed by `home_assistant main:` in `Physical.def` (the operational API target and the output-incarnation name are the same URL now that both houses are reachable via a single Nabu Casa URL, with no local/remote distinction to pick between), and its per-house bridge-target variables (e.g. `${junglinster_instance}`) moved into `Settings.def` as plain constants.

### 9.1 DSL source file split (physical / logical / conceptual)

The three layers of §2 map onto three DSL source files. Only the third exists today per house:

1. **Integration-level device identification** (physical/PSM) — declares what a given integration knows about, driving the physical-level specification. Prototyped today in `$HOUSE/Definitions/Integrations.def` (see §9.2) — **not yet `include`d from `Main.def`**, still exploratory.
2. **Device/entity aggregation** (logical/PIM) — forms the logical level: capability-based aggregation of devices into logical devices/entities (§6.3), independent of both the raw integration and the house's spatial model. **Not started yet.**
3. **`Entities.def`** (conceptual/CIM) — the existing house/space model, already implemented and the most mature of the three.

The dependency direction is bottom-up and constructed on demand: parse the space/entity model first, check completeness of what it needs, then construct the sense/actuate paths down through the logical and physical layers — not the other way around.

### 9.2 `Integrations.def` — current draft state

`Integrations.def` currently prototypes the grammar for declaring what devices an integration exposes:

```
integration <name> [ on <host> ] with device <definition>;
integration <name> [ on <host> ] with devices:
  <definition>;
  ...
end;
```

Working examples already sketched (Junglinster):

- `integration cpu with devices: air-4; backups; ...; xanadu; end;` — one device per host to be CPU-load/temperature-monitored.
- `integration ping on protocols-server-1 with devices: appletv-bedroom-sarah; ...; end;` — one device per host to be liveness-pinged. `cpu` and `ping` are treated as **one conceptual integration** with two data-collection scripts (a "pinger" and an OS-dependent CPU load/temperature collector), and every device declared in `cpu` implies a corresponding device in `ping`.
- `integration hass on protocols-server-2 with devices: weather.junglister with: ...; sensor.envoy with: ...; end;` — importing entities (state + attributes + metadata, kept distinct) from the secondary, integration-adapter HA instance, including looped device generation (`for ${attribute} in ...`, `for ${inverter} in ...`) and a `meta data:` block for otherwise-hardwired device metadata (model/serial/etc.) that will eventually need to travel over MQTT via the coordinator rather than being baked in statically.

Open items visibly still being worked out in that file: whether "device" should carry an explicit `<name> <host_name>;` shorthand; how positioning (the eventual space assignment) determines the actual generated name versus the raw device identity; whether device metadata needs JSON or can stay flat; and a planned generator-time check that entities assumed to exist on the HA side of an integration actually do.

---

## 10. Platform-specific integration notes

No single integration mechanism suits everything — richness of the native API is weighed against MQTT's decoupling benefits per platform:

| Platform | Mechanism | Why |
|---|---|---|
| **Z-Wave** | Native WebSocket to Z-Wave JS UI | Network management, inclusion/exclusion, interviews, firmware updates, configuration parameters, associations — MQTT would lose this richness. |
| **Zigbee** | Zigbee2MQTT, MQTT-native | Z2M deliberately exposes MQTT; excellent interop with HA, FHEM, Node-RED, custom software. |
| **Matter** | Native, close to HA | Not "just another radio protocol" — has its own device models, commissioning, fabrics, application-level interoperability. Feeds `Matter Devices → Home Assistant` directly, not via MQTT. |
| **FS20 / legacy** | Via FHEM | One-way, unconfirmed radio. The coordinator tracks `desired_state` and republishes on MQTT periodically; HA is told `switch: optimistic: true` because that's what accurately represents the underlying hardware — the coordinator provides a stable abstraction despite the unreliable link. |
| **Cloud services** (Netatmo, Volvo, weather, energy, ...) | Via a secondary HA instance in the integration-adapter role (§7) | Isolates cloud auth/rate-limits/quirks from the operational instance; solves multi-house/single-account problems (see §6.5, §12). |

Semantic normalisation happens regardless of source protocol — the consumer never needs to know the original technology:

```
ems/register/0x17/value            →  house/heating/boiler/flow_temperature
netatmo/module/12/outdoorTemperature →  house/weather/outdoor/temperature
```

---

## 11. Deployment topology

```
Home Assistant Green (main instance, per house)
    +-- Home Assistant (Consumer role: automation, dashboards, UI)
    +-- MQTT broker

protocols-server-1                        protocols-server-2
    +-- Z-Wave JS UI (WebSocket)              +-- Home Assistant (Integration/adapter
    +-- Zigbee2MQTT                                role only): Netatmo, envoy solar, ...
    +-- Bluetooth, RTL_433, other radio

FHEM Pi (or host)
    +-- FHEM: FS20, legacy hardware, cloud experiments, protocol adaptation, MQTT publishing
    +-- coordinator

Other Raspberry Pis
    +-- power meter reader, photo frame, other specialised local services
```

`protocols-server-1`/`-2` naming reflects architectural *role*, not current hardware — the point of that naming discipline is that the next protocol to migrate (or the next cloud service to isolate) has an obvious destination without renaming anything.

**Planned, post-MQTT-migration:** the MQTT broker and the coordinator will co-locate on a dedicated Fedora-based Raspberry Pi 4, once the broader move to MQTT-as-integrator is finished — separating both from Home Assistant Green (today's MQTT broker host) and FHEM Pi (the coordinator's placeholder host in the diagram above). Not yet built; the diagram above still reflects the current/interim topology, and this note is what should update it once the migration lands.

### 11.1 Container strategy: appliance vs workbench

Not everything needs Docker/Podman — the decision is whether a service is an *appliance* (stable, should be trivially replaceable) or a *workbench* (actively extended/experimented with):

- **Appliance** (→ containerize, e.g. Podman/Quadlets or Docker): Z-Wave JS UI, Zigbee2MQTT, MQTT bridge services, other radio services. Reproducible deployment, easy migration between hosts (copy the compose/quadlet directory + persistent storage, plug in the hardware, start), isolated dependencies, simple upgrades.
- **Workbench** (→ native install): FHEM. Easier module installation, easier debugging, natural integration with host Linux tooling — containerizing something under active, exploratory development mostly gets in the way.

---

## 12. Multi-house federation

Each house (Junglinster, Vienna) remains autonomous; only selected information crosses the boundary, over MQTT, as **semantic** information rather than implementation detail:

```
Not:  Netatmo API details, Z-Wave node info, device-specific state
But:  house/junglinster/weather/outdoor_temperature
      house/junglinster/security/status
      house/junglinster/energy/consumption
```

Typical shared categories: weather, energy, security state, occupancy (if desired). The Netatmo account problem (§6.5) is the canonical example of why one house sometimes needs to be the sole owner of a cloud integration on the other's behalf, with the coordinator responsible for making that cross-home relationship — and its lineage — explicit rather than silently duplicating credentials or entities.

**Roaming infrastructure devices** (noted 2026-08-21, built 2026-08-26/27): laptops physically move between Junglinster and Vienna — and beyond, since they also travel away from both houses entirely. Each laptop's `cpu` report script (`Integrations/cpu/report`) reports to a shared **bridging MQTT broker** (Mosquitto on mqtt.erikproper.eu, TLS via Let's Encrypt) instead of either house's local broker — reachable from anywhere the laptop has outbound network access, so it keeps reporting even while away from both houses. Same federation shape as the Netatmo cross-home example (§6.5) — a device whose "home" isn't fixed needs its data to flow through a broker neither house directly owns — just for infrastructural monitoring data instead of cloud-sourced semantic data.

Each house's coordinator relays a "cloud"-routed device's traffic from the cloud broker onto its own local broker (`mqtt_relay.go`'s `relayCloudDevice`/`relayCloudDevices`), qualifying the topic with the reporting installation's name inserted *after* the hostname (`hosts/<hostname>/<installation>/...`, never as an overall prefix) so the cloud broker's client-ID-scoped ACL (`hosts/%c/#`) still matches. Liveness can't be an explicit ping here — a laptop "on the road" is unreachable, unlike a LAN-local host — so `TCloudLivenessTracker` infers it instead: arrival of relayed cpu/device-info traffic is itself the liveness signal, "true" the moment traffic resumes, "false" after `cloudDeviceStaleAfter` (3 minutes) of silence, checked every `cloudLivenessSweepInterval` (30 seconds).

The report script's cross-platform TLS handling: Linux has a standard CA bundle file (checked in order — `/etc/ssl/certs/ca-certificates.crt`, `/etc/pki/tls/certs/ca-bundle.crt`, `/etc/ssl/cert.pem`); macOS has none, so the script exports one from the system Keychain on first use and caches it at `~/.cache/mqtt-ca-bundle.pem` (re-exporting on every scheduled run would be wasteful). Deployed and running on `eriks-macbook-pro-2`, `paulas-air-m1`, and `eriks-mac-studio` via `launchd` (`com.erikproper.cpu-report-cloud-client.plist`), all three installations' devices reporting through the shared cloud broker as above. Whatever host ends up running the bridging broker itself is just another `cpu`-type "hosts" integration device — no special-casing needed, the same self-monitoring applies to it too.

**Known macOS/launchd quirk, why every Mac must use the cloud profile, never a local one** (found live 2026-09-05, `eriks-mac-studio`): `mosquitto_pub` (Homebrew, libmosquitto 2.1.0/2.1.2) reliably prints `Error: Bad file descriptor` and silently fails to actually deliver its publish when spawned by a `launchd` LaunchAgent — but *only* when the destination is a LAN address (confirmed with both the local broker's own hostname and its literal IP, both with and without TLS). The identical invocation, same binary, same machine, same LaunchAgent, targeting the external `mqtt.erikproper.eu` (TLS) instead, works cleanly every time (`exit 0`, no error, confirmed delivered). An interactive shell never reproduces it at all, against either destination — it's specific to the no-controlling-terminal LaunchAgent context, and specific to LAN-destined connections within it. Root cause not fully isolated (matches a known *class* of "spawn EBADF in a macOS LaunchAgent context" issue reported for other tools too, with no clean upstream fix found); explicit stdio redirection at the call site didn't resolve it. **Operational rule, not just a workaround**: every Mac host reports via the cloud-routed `cpu` profile (`cloud`+`import` on its Physical.def declaration, `com.erikproper.cpu-report-cloud-client.plist`) even when it's a permanent LAN resident of its own house — never the plain local profile — since the cloud path is the one empirically proven reliable under launchd. `eriks-mac-studio` was switched from `device host.eriks-mac-studio eriks-mac-studio cpu;` (plain local) to `device host.eriks-mac-studio eriks-mac-studio cpu cloud import;` for exactly this reason, joining `eriks-macbook-pro-2`/`host.mqtt` on the same pattern.

**Roaming `home_assistant`-bridge devices** (an HA companion-app device, e.g. a phone, reachable via more than one local HA instance — `hass.eriks_iphone`, built 2026-09-02): `THassBridgeDevice.Instances []string` lets the same device be declared identically under more than one `integration home_assistant <qualifier> with:` block in the same house's Physical.def, merged (not colliding) generator-side. `ExportAs` (the `roaming` keyword, replacing `export`) qualifies the cloud cross-post with a shared virtual installation name ("roaming") instead of this house's own real name, so several real installations can each export their own local copy of the device under one stable cloud identity; `SelfImportFrom` (the `import` keyword) additionally relays that shared cloud data back onto this coordinator's own local bridge topic, feeding the one Spaces.def-positioned entity rather than creating a second one.

Four bugs found and fixed live 2026-09-05, the first day this was exercised for a real device (`hass.eriks_iphone`, roaming across `main`/`protocols-server-2` at Junglinster and `main` at Vienna):

- **Self-import feedback loop**: `subscribeHassBridgeSelfImport`'s relay republishes onto a *synthetic* local topic keyed by this house's own installation name (`homeassistant_instances/<installation>/bridge/...`) — never a real declared instance. Before the fix, `subscribeHassBridge`'s own local wildcard treated that synthetic topic as fresh genuine traffic and cross-posted it straight back to the cloud under `roaming`, which the self-import relay was itself subscribed to — an unbounded republish loop that flooded the shared cloud broker within seconds of first use. Fixed by gating cross-posting on the reporting topic's instance segment genuinely being one of the device's own declared `Instances` (`containsString`) — a synthetic echo never is, so it's relayed locally (the local broker copy still needs it) but never re-exported.
- **Non-canonical roaming topic**: a roaming device's cloud-published bridge topics (entity-state, device-info, and the availability reference inside each discovery config) embedded whichever *real* local instance happened to report most recently (`homeassistant_instances/main/bridge/.../state` vs. `.../protocols-server-2/bridge/.../state`), forcing any importer to watch both topics, or wildcard, to see the device's current value. Fixed by `canonicalizeRoamingBridgeTopic`: for a roaming device only (`ExportAs != ""`), every cloud-bound topic's instance segment is rewritten to one fixed literal (`"main"`) regardless of which real instance actually published — so an importer only ever needs to know one topic per capability.
- **Cross-house existence check false positive**: `checkKnownNotToExistErrors` (§6.9/§6.10, generator side) treats a capability as a real problem only when *every* declared instance confirms it not-to-exist — but `hassBridgeDevicesByID` is built from this house's own Physical.def alone, so a roaming device's `Instances` there is only ever this house's own local subset. Vienna's own `hass.eriks_iphone` declaration (`Instances: ["main"]`, Vienna's own main) failed generation outright the moment Vienna's own existence status became reliably fresh (see the immediate-publish fix below) — Vienna's own main instance genuinely has no local pairing for this capability, but the same roaming device is confirmed known-to-exist on Junglinster's own instances, information Vienna's generator has no way to see. A roaming device's existence can never be authoritatively judged from one house's own local instance(s) alone. Fixed by skipping the hard-fail check entirely for any device with `ExportAs != ""` — generated optimistically, same as an unresolved (`not-known-to-exist`) source.

**Cloud existence-status topic collision, found and fixed live 2026-09-05**: `discovery_gateways/<gatewayID>/existence/state` and `homeassistant_instances/<name>/existence/state` (§6.10) are published to both the local and cloud broker unconditionally when a cloud broker is configured — but, before this fix, under the *same bare topic* on both. Harmless locally (each house only ever reads its own), but wrong on the shared cloud broker: every house's own primary instance is conventionally named "main", and Junglinster's and Vienna's own retained existence status were confirmed live to be silently overwriting each other. Fixed by qualifying the CLOUD copy only with the owning installation's own name (`<installation>/discovery_gateways/...`, `<installation>/homeassistant_instances/...`), the same whole-topic-prefix convention `export`'s own cloud cross-post already uses (§6.12) — not the `hosts` kind's different "insert after first segment" convention, which exists solely for that kind's client-ID-scoped cloud ACL and doesn't apply here. Generator-side readers construct the matching qualified topic (using `ctx.Installation`, this house's own name) only when reading via the cloud broker; the local-broker read path is untouched.

**Follow-up gap, same day**: `publishStatus` (both kind-2 and kind-3) only ever fires on an actual known/retracted transition — a coordinator restart re-seeds already-known status in memory (from persisted JSON) without re-publishing it, so a topic-naming migration like the qualification fix above left the newly-qualified cloud topic with *no* retained value at all until the next real transition, which a stable gateway or instance might not produce again for a long time; every `./generate` cloud fetch timed out in the meantime (confirmed live for Junglinster's discovery gateways). Fixed by publishing every declared instance's/gateway's current status immediately on startup — `TEntityExistenceTracker.StartEntityExistenceInquiries` and the new `TDiscoveryExistenceTracker.PublishAll`, both called right after `Seed`.

---

## 13. Open design questions

- Should **provider** become a first-class DSL concept (as opposed to today's `providing` macro pattern)?
- How should **projections** (to MQTT, to HA YAML, to future backends) and **transformations** be represented in the semantic model — and how should reusable patterns across them work?
- Cross-check **HA areas vs. DSL spaces** — are they the same concept, and if not, how do they relate?
- `Integrations.def` grammar is still being worked out by direct experimentation (§9.2) rather than designed up front — expect it to keep changing shape for a while before the physical/logical file split (§9.1) is worth actually implementing in the parser.
- How a `space` definition refers to a device attribute (constant/variable, device/world — §6.6) rather than only a device, and how that then materializes as a generated entity, is still open.
- ~~Per-integration default icons (noted 2026-08-21)~~ — **done, see §6.8** (2026-08-26): built as the `defaults: for <domain>.<pattern>: ...; end;` grammar plus `Shared/Definitions/Defaults.def`, generalized well beyond icons to device_class/unit/state_class too, and applying to every integration kind, not just "hosts".
- ~~Raw/extensional entity references should become unnecessary post-MQTT-migration~~ — **done** (2026-09-01): the `domain.[raw_entity_id]` syntax is removed outright, not just deprecated — an audit found zero real usages left in either house's Spaces.def/Physical.def, so `TEntityIdentity.IsRaw`/`RawName` and every call site that special-cased raw entities were deleted from the parser/generator.
- ~~**"Federated open home"** (reminder, 2026-09-02, moved here from PROJECT.md's step 1.2g)~~ — **resolved 2026-09-06**: the project is named **Federated Home Assistant** (this document's own title, §0/top-of-file history) — not a new idea beyond §12/§6.5, just the settled working name for that same vision.

### Two-level parser sketch (from early notes, unimplemented)

```
node             ::= NodeForPlatform
WithClause(f)    ::= "with", (":", f | GroupClause(f))
NodeForPlatform  ::= <name> WithClause(MyWithClause)
```

Kept here as a placeholder for whenever the `Integrations.def`/aggregation-file grammar gets formalized — not a commitment to this exact shape.
