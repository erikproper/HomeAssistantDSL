# Home Automation Architecture

**Status:** living document. Restructured 2026-08-18 from the original notes plus `Background/Arch1.md`, `Arch2.md`, `Arch3.md`. Superseded fragments were folded in rather than kept as separate files; `Background/*.md` remains as raw source material.

## 1. Central principle

> The house is modelled, not programmed.

Home Assistant, FHEM, MQTT, Z-Wave, Zigbee, Matter, and every cloud service are implementation platforms. The source of truth is the semantic model of the house, expressed in the DSL. Platforms consume generated configuration; they do not define the model.

Terminology follows Home Assistant's own vocabulary (entities, devices, integrations, areas) even where that vocabulary isn't ontologically precise — consistency with the target platform's mental model beats theoretical purity.

### What this project actually is

The DSL has outgrown "a configuration language for Home Assistant." It is becoming a model-driven specification language for cyber-physical information systems, of which Home Assistant is one backend and MQTT is one transport. Other projections (documentation, a dependency graph, a digital twin, an ArchiMate diagram) are the same kind of artifact as the HA YAML — just different generators over the same model. Keeping the conceptual core clean (see §3) and resisting the temptation to let HA- or MQTT-specific concepts leak upward into it is what keeps that positioning possible.

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
- **Logical**: devices that provide entities. A device is not yet a "real" entity until it is positioned in a space. Devices may themselves be aggregated from other devices/entities (e.g. an external temperature sensor merged into a smart plug to form one logical "heater" device; a Z-Wave wall switch in front of a Zigbee dimmer forming one logical light).
- **Physical**: protocol- and integration-specific reality, abstracted (mostly) to MQTT, with pragmatic exceptions such as media players and Z-Wave (see §11).

The relationship between the conceptual and logical/physical layers is a mapping, not a 1:1 correspondence — one conceptual light can be realized by several physical devices plus an automation stitching them together, and inhabitants never need to know that.

---

## 3. Three semantic namespaces

Entities are organized into three namespaces (spheres), already reflected in the current DSL (`social:`, `physical:`, `infrastructural:` prefixes):

- **Social** — directly relevant to inhabitants: `social/house/living_room/co2`, `social/house/bedroom/temperature`. Stable even when the underlying technology changes; this is the conceptual layer's surface.
- **Physical** — implementation detail: `physical/zwave/living_room_switch`, `physical/netatmo/outdoor_sensor`. Exists because the system requires it; not normally exposed as a user-facing entity.
- **Infrastructural** — entities needed to operate and maintain the automation platform itself, not the house: `infrastructural/pi4/status`, `infrastructural/mqtt/status`, node/device availability. These enable infrastructure self-healing (e.g. detect a crashed Pi → power-cycle its smart plug → wait → verify recovery) independently of house semantics.

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

## 9. Codebase & compiler strategy

- Coordinator and generator are likely a **split codebase** (different runtime characteristics: coordinator is a long-running process reacting to MQTT; generator is a batch compiler).
- The generator **evolves from the existing one** rather than a ground-up rewrite — see the incremental migration plan in `PROJECT.md`.
- The generator should have **integration-specific source code per integration**: parsing that integration's source specification, storing the resulting data, and generating that integration's specific output. A bridge to another MQTT broker is itself just a special (bidirectional) kind of integration, not a separate mechanism.
- **Introduce an explicit intermediate semantic model** inside the compiler now, before it's strictly necessary, because it will inevitably be needed. Today the compiler is roughly:

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

  Known gap against both principles in the current implementation: it is thoroughly filename-driven. `generateFromPaths` reads `Entities.def`/`Settings.def` by hardcoded name; `resolveBridgeTargets` reads `Server.def`/`Secrets.def`/`Bridges.def` by name; `resolveHomeAssistantTarget` reads `Server.def`/`Secrets.def` by name; `resolveMainIncarnationName` reads `Physical.def`/`Secrets.def` by name. Each is also its own separate read pass, interleaved with (and in the incarnation-name case, *before*) entity parsing, rather than one unified read-everything-then-interpret pass. Don't grow more filename-keyed side-channel readers in the meantime — this is the concrete shape the semantic-model layer above needs to dissolve.

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

---

## 13. Open design questions

- Should **provider** become a first-class DSL concept (as opposed to today's `providing` macro pattern)?
- How should **projections** (to MQTT, to HA YAML, to future backends) and **transformations** be represented in the semantic model — and how should reusable patterns across them work?
- Cross-check **HA areas vs. DSL spaces** — are they the same concept, and if not, how do they relate?
- `Integrations.def` grammar is still being worked out by direct experimentation (§9.2) rather than designed up front — expect it to keep changing shape for a while before the physical/logical file split (§9.1) is worth actually implementing in the parser.
- How a `space` definition refers to a device attribute (constant/variable, device/world — §6.6) rather than only a device, and how that then materializes as a generated entity, is still open.

### Two-level parser sketch (from early notes, unimplemented)

```
node             ::= NodeForPlatform
WithClause(f)    ::= "with", (":", f | GroupClause(f))
NodeForPlatform  ::= <name> WithClause(MyWithClause)
```

Kept here as a placeholder for whenever the `Integrations.def`/aggregation-file grammar gets formalized — not a commitment to this exact shape.
