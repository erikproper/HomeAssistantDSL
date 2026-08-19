ADD 0] {{ { 'manufacturer': device_attr(device_id('sensor.envoy_122326133466_current_power_production'), 'manufacturer'), 'model': device_attr(device_id('sensor.envoy_122326133466_current_power_production'), 'model'), 'sw_version': device_attr(device_id('sensor.envoy_122326133466_current_power_production'), 'sw_version'), 'hw_version': device_attr(device_id('sensor.envoy_122326133466_current_power_production'), 'hw_version'), 'serial_number': device_attr(device_id('sensor.envoy_122326133466_current_power_production'), 'serial_number') } | tojson }}
Do this for "known attributes"

Got it — that's a meaningfully bigger scope than what we've built so far. Publishing device attributes as a flat JSON blob (what we've done) is different from MQTT discovery proper: discovery needs a config payload per entity, published (retained) to `homeassistant/<component>/<node_id>/<object_id>/config`, containing things like `unique_id`, `name`, `state_topic`, `device_class`, `unit_of_measurement`, and a `device` block — which is exactly the device-registry attribute set we've been discussing. Then separately, each entity's actual value has to be mirrored to whatever `state_topic` that config payload points at, so instance B's MQTT integration can both discover the entity and receive live updates for it.

So the eventual generator would need two things per entity: the entity's platform-specific config schema (sensor vs binary_sensor vs climate etc. all have different required/optional fields) plus the device block, and the state-mirroring side we've already prototyped in this conversation. Good context to have — noted for when you want to start on the Go side.

ADD 1] Another conceptual task for the "coordinator".

Sometimes devices (in the infrastructure sense) don't have their own (smart) power switch, or temperature sensor. In that case we want to merge an external sensor "into" an actual (logical) device that is then represented as one infrastructural element at the top conceptual level.

To do so, the coordinator must combine the states (and topics) of two sources topics into one for the "aggregated" device.

Yes. I think this is an important conceptual capability of the coordinator, and I would model it as **device aggregation**, rather than as a special case of discovery.

The key distinction is:

- **Physical devices** are the sources of observations and commands on the infrastructure buses.
- **Logical devices** are the conceptual infrastructural elements exposed to the higher-level model.
- A logical device may therefore be **composed from multiple physical/source devices**.

For example:

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
                                                     │
                              ┌──────────────────────┴──────────────────┐
                              │                                         │
                           power                                     temperature
                              │                                         │
                              ▼                                         ▼
                         HA entity                                  HA entity
```

The coordinator consequently needs to do more than simply relay MQTT topics. It needs to maintain a **mapping from source capabilities to logical-device capabilities**.

### I would introduce an explicit aggregation concept

Something along these lines conceptually:

```
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

The important point is that `smart_plug_living_room` and `temperature_sensor_living_room` remain separate source devices internally.

The coordinator presents them as **one logical device**.

This also means the coordinator needs to understand that:

> `temperature_sensor_living_room` is not necessarily a top-level infrastructure element in the conceptual model.

It can be a **component/source of another infrastructure element**.

### Aggregation should work in both directions

For **state**, the coordinator combines incoming topics:

```
smart_plug/state
        ───────────────┐
                       ├──> logical_device/power
temperature_sensor/state
        ───────────────┘
```

But for **commands**, it must route the command back to the appropriate source:

```
logical_device/power/set
        │
        ▼
coordinator
        │
        ▼
smart_plug/power/set
```

So the coordinator is effectively maintaining a small **topic transformation/routing graph**.

### This also fits nicely with the "not yet migrated" idea

The aggregation should be independent of whether a source has already migrated.

For example:

```
Logical device
    │
    ├── power ──> source A ──> migrated? yes/no
    │
    └── temp  ──> source B ──> migrated? yes/no
```

ADD 2] During migration, the coordinator could continue exposing the **same logical device**, while individual source capabilities move from the old infrastructure to the new one.

That is much cleaner than making HA aware of the migration.

------
### And it reinforces the distinction we already made

I would therefore give the coordinator three conceptually separate responsibilities:

1. **Discovery translation**  
    Source-device information → conceptual HA representation.
2. **Aggregation**  
    Several source devices → one logical infrastructural device.
3. **Routing/transformation**  
    Logical-device topics ↔ source-device topics.

The third one is particularly important: the coordinator isn't merely a discovery proxy. It becomes the **runtime realisation of the conceptual model on the event bus**.

And I would explicitly allow aggregation to be **many-to-one**, not merely one external sensor attached to one device. A logical device could potentially be assembled from a smart switch, an external temperature sensor, a power meter, and perhaps an occupancy sensor — all presented as one infrastructural element at the conceptual level.

2] During the migration, we may need the coordinator to blindly cross post discovery messages of "not yet migrated" entities. Might be good to include as a setting for the coordinator.

## Home Assistant on `protocols-server-2` as an Integration Adapter

The Home Assistant incarnation on `protocols-server-2` has a specific architectural role: it acts as an **integration adapter from external/cloud services into the MQTT-based home-automation infrastructure**.

### Raw discovery

For entities exposed by external/cloud integrations, this Home Assistant instance should publish **RAW discovery** information to MQTT.

The discovery information can be constructed from the local Home Assistant entities and their metadata, including information such as attribution and integration-specific entity/device metadata.

The important distinction is that this is **RAW discovery**. The adapter reports what the local Home Assistant instance knows about the entity. It does not need to know the final naming conventions or other system-level semantics defined by the model.

The coordinator can subsequently refine this information.

### Discovery publication

The adapter should publish discovery information:

1. when the Home Assistant instance starts;
2. when relevant entity metadata changes, if such changes can be reliably detected;
3. periodically as a safety mechanism.

The desired event flow is:

```text
HA startup ────────────────┐
                           │
entity metadata change ────┤
                           │
periodic refresh ──────────┤
                           ▼
                   publish RAW discovery
```

The periodic refresh provides eventual consistency in case a metadata change is not exposed through a sufficiently reliable Home Assistant event mechanism.

An initial implementation could therefore use an hourly refresh:

```text
discovery_refresh_interval = 1h
```

### Division of responsibilities

#### Home Assistant on `protocols-server-2`

**Integration adapter**

> "I have these entities, this is what I know about them, and here is their raw discovery information."

It publishes:

- raw discovery;
- state;
- availability;
- other relevant entity information.

#### Coordinator

**Model/infrastructure/federation layer**

> "I know what these entities mean in the overall system, where they belong, what they depend on, how they should be named, and which other home(s) should see them."

It performs:

- discovery refinement;
- infrastructure availability propagation;
- cross-home projection;
- lineage;
- model-based naming and refinement.

### Netatmo example

The Netatmo case illustrates the complete pipeline particularly well:

```text
Netatmo Cloud
      │
      ▼
HA @ protocols-server-2
      │
      │ local HA entity + metadata
      ▼
RAW MQTT DISCOVERY
      │
      ▼
Coordinator @ Junglinster
      │
      │ model + lineage
      ▼
Vienna MQTT bus
      │
      ▼
HA @ Vienna
```

The Vienna Home Assistant instance therefore sees a normal MQTT-discovered entity on its local MQTT bus, even though the actual source is a Netatmo device connected through the cloud and the intermediary is the Home Assistant instance running at Junglinster.

The coordinator is responsible for making the cross-home nature and lineage explicit.

This preserves the architectural separation:

```text
External integration
        │
        ▼
Local HA adapter
        │
        ▼
   RAW discovery
        │
        ▼
    Coordinator
        │
        ├── model refinement
        ├── availability semantics
        └── cross-home federation
        │
        ▼
   MQTT representation
        │
        ▼
Other Home Assistant
```

The raw/refined distinction therefore remains useful: **each adapter publishes what it genuinely knows, while the coordinator remains the authoritative place for system-level semantics.**


# Model-Aware Home Automation Coordinator

## Purpose

The coordinator is a model-aware component that sits between the infrastructure/integrations and the Home Assistant instances.

It is **not merely a discovery refiner**.
Discovery refinement is one of three principal coordinator roles.

The coordinator consumes observations from other components, interprets them using the model, and produces MQTT-level projections such as refined discovery, availability state, and cross-home representations.

The MQTT bus remains the principal integration boundary.

## Three coordinator roles

### 1. Local infrastructure coordination

The coordinator is responsible for interpreting local infrastructure availability and propagating its consequences.

It is **not responsible for sensing availability**.

Availability sensing is performed by other components, for example:

- ping/host health checks;
- MQTT "are you alive?" mechanisms;
- integration-specific health monitoring;
- Zigbee2MQTT health information;
- Z-Wave JS health information.

These components provide observations to the coordinator.

The coordinator uses the model to understand dependencies and ownership.

For example:

```text
protocols-server-1 = unavailable
        |
        +-- Zigbee2MQTT depends on protocols-server-1
        |      +-- device A
        |      +-- device B
        |      `-- device C
        |
        `-- Z-Wave JS depends on protocols-server-1
               +-- device D
               `-- device E
```

The coordinator can consequently propagate the appropriate availability state to affected devices/entities.

The architectural distinction is:

> **The coordinator consumes availability observations; it does not perform the availability sensing.**

### 2. Discovery refinement

Some integrations already provide Home Assistant MQTT Discovery information.

Examples include:

- Zigbee2MQTT;
- Z-Wave JS;
- other integrations that can generate HA discovery messages.

These can publish raw discovery information to a raw discovery namespace.

The coordinator can then apply the model to produce refined discovery information.

```text
Zigbee2MQTT ----\
                 \
Z-Wave JS -------+--> raw discovery --> coordinator --> refined discovery
                 /
other producers /
```

The refinement may, for example, change:

- `default_entity_id`;
- `name`;
- model-specific device metadata;
- other properties defined by the model.

The remainder of the technical discovery description should be retained from the source wherever possible.

This gives a clean separation between:

- **raw discovery**: what the integration knows about the device/entity;
- **model refinement**: how that device/entity should be represented in this particular home.

Integrations that do not themselves provide discovery can be model-aware from the start.

For example, `fhem2mqtt` can use the model while generating its discovery information:

```text
FHEM --> fhem2mqtt + model --> final discovery
```

There is therefore no requirement that every discovery producer pass through a refinement stage.

### 3. Cross-home federation

The coordinator also acts as the federation mechanism between the homes.

Relevant entities/devices can be cross-posted from one home to another over MQTT.

The receiving home should see the remote entity as a local MQTT-based integration/bridge, rather than needing direct access to the original integration.

Conceptually:

```text
Home A
    source entity
        |
        v
    Coordinator A
        |
        | MQTT
        v
    Coordinator B
        |
        v
    local bridge representation
        |
        v
    Home Assistant B
```

The cross-home representation must preserve **lineage**.

The receiving home should be able to establish that an entity is not actually locally owned, but is a representation of an entity originating elsewhere.

## Special case: Netatmo federation

Netatmo is a particularly important example because the two homes do not have independent Netatmo data sources.

The Netatmo devices physically associated with Vienna are connected to the Netatmo cloud account.

The Junglinster Home Assistant instance, running on `protocols-server-2`, obtains **all Netatmo data from the cloud**, including the data belonging to the Vienna Netatmo devices.

That means the Junglinster side effectively has a view of:

```text
Netatmo Cloud
    |
    +-- Vienna Netatmo devices
    |
    +-- Junglinster Netatmo devices
    |
    `-- other data available through the account
```

The Vienna Home Assistant instance, however, should receive the relevant Vienna Netatmo data through the **Vienna MQTT bus**, as if it were local data.

The coordinator on the Junglinster side therefore needs to project the relevant Netatmo entities onto the Vienna MQTT bus.

The important point is that this is **not simply a generic copy of the Netatmo entities**.

The cross-posted entities need to carry their origin/lineage.

For example, the conceptual device tree should contain something like:

```text
sensor
  |
  `-- cloud
       |
       `-- smart_home@junglinster
            |
            `-- local
```

The precise modelling of the tree can evolve, but the essential semantics are:

- the entity is a `sensor`;
- its data originates from the cloud;
- the cloud-side Home Assistant/infrastructure involved is `smart_home@junglinster`;
- the representation exposed on the Vienna MQTT bus is local to the Vienna side of the integration boundary.

Thus, from the perspective of Vienna Home Assistant, the entity is available on the local MQTT bus, while its lineage makes its true origin explicit.

## Model, observations, and projections

A useful conceptualisation of the coordinator is in terms of three things.

### Model

The model describes:

- what entities and devices exist;
- how they are named;
- where they are located;
- which integration provides them;
- which infrastructure they depend on;
- which home owns them;
- which entities/devices are exposed to another home;
- their lineage.

### Observations

The coordinator receives observations from other components, such as:

```text
host UP
gateway DOWN
device unavailable
MQTT connection alive
integration healthy
```

The coordinator does not need to implement the mechanisms that obtain these observations.

### Projections

The coordinator turns the model plus observations into MQTT-level projections, such as:

- refined discovery;
- availability;
- cross-home entity/device representations;
- state updates;
- lineage information.

Conceptually:

```text
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
          |            |             |
          +------------+-------------+
                       |
                       v
                      MQTT
```

## Architectural consequence

The coordinator is therefore best regarded as a genuine architectural component rather than as a collection of Home Assistant automations.

Home Assistant remains responsible for consuming the resulting MQTT representation and providing the user-facing automation and UI environment.

The coordinator remains responsible for interpreting the model and the infrastructure behind the MQTT entities.

This separation also keeps the two homes loosely coupled.

A source integration can remain unaware of the other home, while the coordinator determines which entities are projected across the home boundary and how their lineage is represented.

## Summary

The coordinator has three principal roles:

1. **Local infrastructure coordination** — interpret externally supplied availability observations and propagate their consequences according to the model.
2. **Discovery refinement** — transform raw discovery from integrations such as Zigbee2MQTT and Z-Wave JS according to the model.
3. **Cross-home federation** — project selected entities/devices and their updates onto another home's MQTT bus, preserving their lineage.

For integrations such as `fhem2mqtt` that can directly consume the model, discovery can instead be generated correctly from the outset.

The resulting architecture treats MQTT as the integration boundary, the model as the source of semantic and infrastructural knowledge, and the coordinator as the component that turns that knowledge plus runtime observations into useful local and cross-home projections.




Yes, exactly that — `zigbee2mqtt/bridge/request/health_check`. Publish an empty payload there and Z2M responds on `zigbee2mqtt/bridge/response/health_check` with something like:

json

```json
{"data":{"healthy":true},"status":"ok"}
```

That's a genuine active on-demand probe rather than a passive availability flag — you publish, it has to actually be alive and processing to answer back. This is part of Z2M's broader `bridge/request/*` → `bridge/response/*` pattern (same mechanism used for things like `permit_join`, `restart`, `networkmap`, device rename/remove, etc.), where each request can carry a transaction ID so you can correlate a specific response to a specific request if you're firing them concurrently.

So between the two: `bridge/state` gives you a passive "did it disconnect" signal via retained message + LWT, and `bridge/request/health_check` gives you an active "is it actually responsive right now" probe — the latter is the closer fit for what you were describing as a "liveness" check.

zigbee2mqtt/bridge/request/**health**_check ping

zigbee2mqtt/bridge/logging {"level":"info","message":"z2m:mqtt: MQTT publish: topic 'zigbee2mqtt/bridge/response/**health**_check', payload '{\"data\":{\"**health**y\":true},\"status\":\"ok\"}'"}

zigbee2mqtt/bridge/response/**health**_check {"data":{"**health**y":true},"status":"ok"}

----

FHEM@old_pi3_vienna

HASS2@old_fhem with old samsung 

scripts to ssh tunnel localhost:XXX for the different services on the inside.

Start with the CPU heating, etc.

Per platform (zwave, fhem. Z2mq, hass) a spec file. It generates the platform config where needed and a spec for the MQTT based entries for the central generator.
Maybe we need a bus_id … then the house.def file can position these semantically, where we create a spec that contains per busID the type and topics.

Two kinds:
- produces its own mqtt (z2mq, zwave js, ems-esp)
- needs an mqtt connector (fhem, hass)

I actually think the Z-Wave issue may have done you a favour.
Not because it was pleasant 😊, but because it forced you to establish what I would consider the target infrastructure first:

protocols-server-1 exists.
Fedora + Podman + Quadlets are working.
Z-Wave is no longer tied to a Home Assistant host.
The naming scheme (protocols-server-*) reflects the architectural role rather than the current hardware.
The next protocol (Zigbee) now has an obvious destination.
Had you continued immediately with the DSL work, you might have found yourself designing against an architecture that was about to change.
I think the next phase naturally becomes
Phase 1  Infrastructure
-----------------------
✓ protocols-server-1
✓ Podman
✓ Z-Wave JS

Phase 2  Existing DSL
---------------------
□ Test Vienna
□ Test Junglinster
□ Stabilise compiler

Phase 3  New Architecture
-------------------------
□ Canonical MQTT interaction model
□ Platform adapters
□ Generated Home Assistant configuration

Phase 4  Platform migrations
----------------------------
□ Z-Wave
□ Zigbee
□ EMS-ESP
□ Netatmo
...
That ordering feels very safe.
I also think Claude Code is a good fit
For the implementation work, I suspect Claude Code will be excellent.
What I think I can contribute is a bit different.

There are (at least) three levels of work here:

1. Architecture (our discussions)
Questions like:
Should this concept belong to the COM or the CDM?
Is this really a platform adapter?
What is the canonical interaction model?
Should this be generated or configured?
These are design discussions.
2. Language design
Things like:
Should provider become a first-class concept?
How should projections be expressed?
How should transformations be represented?
How should reusable patterns work?
Again, mostly conceptual.
3. Implementation
Writing parsers.
Extending the compiler.

Generating YAML.

Refactoring.

Unit tests.

That's exactly where Claude Code shines.

One thing I would encourage
Since you're going to touch the compiler anyway, I'd start introducing an explicit intermediate model.
Not immediately because you need it.

Because you'll inevitably want it.

Today the compiler is probably something like

DSL
   │
   ▼
Home Assistant YAML
I would evolve it towards
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
   ├── ArchiMate Generator (!)
   └── ...
The semantic model becomes the stable API inside the compiler.
Everything else plugs into it.

That's a very MDE-like architecture.

And one thought that occurred to me...
Your DSL is beginning to look like something that has its own identity.
It's no longer "a configuration language for Home Assistant."

It's becoming a language for specifying domestic information systems.

Or perhaps more generally:

A model-driven specification language for cyber-physical information systems.
That's a very different positioning.
Home Assistant just happens to be one backend.

MQTT just happens to be one backend.

ArchiMate diagrams could become another.

Documentation another.

Simulation another.

I'm genuinely looking forward to where this goes over the coming months. One thing that struck me when you showed the .def file is that it doesn't feel like a collection of YAML shortcuts—it already has the flavour of a proper modelling language. If you continue to keep the conceptual core clean (COM → CDM → projections) while resisting the temptation to let Home Assistant or MQTT concepts leak into it, I think you'll end up with something that is both academically interesting and practically useful. And, perhaps most satisfyingly, it will be a concrete, executable demonstration of the Information Systems Engineering philosophy you've been articulating in your manifesto.


### Home Assistant deployment roles

The architecture distinguishes **logical roles** from their **deployment**.

A Home Assistant instance is not inherently the centre of the architecture. Instead, it is one possible implementation of a **Platform Adapter**, potentially combined with user-facing capabilities such as dashboards and automations.

A Home Assistant instance may simultaneously fulfil two independent roles.

#### Integration role

In this role, Home Assistant acts as an adapter between external platforms and the canonical event bus.

Typical examples are cloud-based integrations, such as:

- Netatmo
- Volvo
- Google Calendar
- Weather providers
- Energy providers
- Vendor-specific cloud services

The imported entities are projected onto the canonical event bus using their canonical event definitions and bus identifiers.

#### Consumer role

In this role, Home Assistant subscribes to the canonical event bus and reconstructs entities from the event stream.

These entities may originate from any platform participating in the architecture, including:

- Z-Wave
- Zigbee
- Matter
- FHEM
- ESPHome
- other Home Assistant instances
- custom applications

Automations, dashboards, scripts, and visualisations operate exclusively on these reconstructed entities.

### Deployment flexibility

The logical architecture is independent of deployment.

For example, in one deployment the two roles may be separated.

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

This separation isolates cloud integrations from the operational Home Assistant instance.

In another deployment, both roles may be implemented by the same Home Assistant instance.

```
                   Canonical Event Bus
                          |
                  Home Assistant
         (Integration + Automation + UI)
```

Conceptually, both deployments are identical.

Only the deployment architecture differs.

### Architectural consequence

The canonical model and the event bus form the centre of the architecture.

Platforms—including Home Assistant itself—are simply adapters between their own internal representation and the canonical event model.

This makes the architecture independent of:

- the number of Home Assistant instances,
- the home automation platform,
- communication protocols,
- deployment topology.

The same canonical model can therefore be deployed in different environments while preserving identical semantics.
I actually think this is becoming a fairly coherent architectural style. It is no longer "model-driven Home Assistant", but rather a Canonical Event-Driven Home Automation Architecture (CEDHA?) in which Home Assistant is deliberately decentralised. That is a rather uncommon design in the home automation world, but conceptually it is very clean and very much aligned with model-driven information systems engineering.

## Towards a Canonical Home Automation Model

Rather than modelling Home Assistant, Z-Wave, Zigbee, FHEM, MQTT, etc., directly, define a **canonical domain model** that captures the semantics of the home and its devices independently of any implementation technology.

```
                  Canonical Domain Model
                          |
          +---------------+---------------+
          |               |               |
     Home Assistant    MQTT Event Bus   Other projections
          |               |               |
      YAML entities   Topic mappings   Digital Twin, Analytics, ...
```

The various platforms (Z-Wave JS, Zigbee2MQTT, FHEM, Matter, ...) become *producers* and/or *consumers* of the canonical model rather than defining it.

### Three identities

A physical or logical object has three distinct identities:

1. **Domain identity**
   - Stable conceptual identity.
   - Independent of technology.
   - Example:

     ```
     GroundFloor.Kitchen.Ceiling.Light
     ```

2. **Platform identity**
   - Identity assigned by a specific platform.
   - Examples:

     ```yaml
     zwave:
       node: 17
       endpoint: 0

     zigbee:
       ieee: 0x00158d0009abc123

     fhem:
       device: KitchenLight
     ```

3. **Bus identity**
   - Stable identity used on the event bus.
   - Example:

     ```
     lighting/kitchen/ceiling
     ```

Applications communicate through the bus identity rather than platform-specific identifiers.

### Canonical properties

The canonical model should describe *what* a device represents rather than *how* information is transported.

Example:

```yaml
WindowSensor
  reports:
    - OpenClosed
    - BatteryLevel
    - Tamper
```

No MQTT topics, Z-Wave nodes, or Home Assistant entities appear in the conceptual model.

### Event endpoints

Rather than merely assigning a `bus_id`, explicitly model an **EventEndpoint**.

An EventEndpoint may include:

- identifier
- state topic
- command topic
- availability topic
- payload converter
- QoS
- retain flag
- communication direction
- producer(s)
- consumer(s)

This separates the semantics of an observable property from the transport used to communicate it.

### Model transformations

The canonical model becomes the source for multiple generated artefacts.

```
Canonical Model
      |
      +--> Home Assistant YAML
      |
      +--> MQTT topic mappings
      |
      +--> Z-Wave configuration
      |
      +--> Zigbee configuration
      |
      +--> FHEM configuration
      |
      +--> Digital Twin representation
```

Platform-specific models contribute implementation details (such as node identifiers, MQTT topics, converters, etc.), while the compiler generates the required configurations.

### Home Assistant

Home Assistant becomes just one projection of the canonical model.

Entities may be generated either:

- as native Home Assistant YAML definitions, or
- as MQTT entities consuming canonical event topics.

Ultimately, MQTT Discovery can be disabled entirely, with all entities being generated from the canonical model.

### Migration benefits

When replacing one platform by another (for example Z-Wave by Matter), only the platform-specific transformation changes.

The following remain unchanged:

- canonical model
- bus identities
- Home Assistant entities
- automations
- Digital Twin
- other applications consuming the event bus

This isolates technological evolution from the conceptual model.

### Long-term architectural direction

Treat the event bus as a first-class architectural constituent rather than merely an MQTT broker.

Observable properties are bound to one or more transport technologies (MQTT, WebSocket, REST, OPC-UA, Kafka, ...), allowing transport mechanisms to evolve independently of the domain model.

The resulting architecture distinguishes clearly between:

- **semantics** (canonical domain model),
- **technology-specific implementations** (Z-Wave, Zigbee, Matter, ...), and
- **transport bindings** (MQTT, WebSocket, REST, ...).

This separation allows all implementation-specific artefacts—including Home Assistant configuration, MQTT mappings, platform configuration, and Digital Twin representations—to be generated from a single authoritative model.


# Home Automation Architecture Vision

THINK CIM/PIM/PSM

# Reflection: Model Driven Architecture (MDA)

This architecture resembles OMG's Model Driven Architecture approach:

- CIM: conceptual understanding of the house and its functions.
- PIM: semantic model independent of implementation technology.
- PSM: technological realization using Z-Wave, FHEM, MQTT, Matter, etc.

The objective is to keep transformations explicit and avoid coupling semantic concepts to implementation details.

**Date:** 2026-08-05

## Purpose

This document captures the architectural vision behind the ongoing Home Assistant migration and the longer-term evolution of the home automation environment.

The migration from Raspberry Pi based Home Assistant installations towards Home Assistant Green is not only a hardware migration. It is an opportunity to evolve towards a more modular, model-driven architecture.

The central principle is:

> The house is modelled, not programmed.

Home Assistant, FHEM, MQTT, Z-Wave, Zigbee, Matter and other technologies are implementation platforms. The primary source of truth should be the semantic model of the house.

---

# Architecture Overview

The architecture consists of multiple conceptual layers.

```
                    Conceptual House Model

                            |

                    Technological Model

                            |

              +-------------+-------------+

              |             |             |

        Home Assistant    FHEM        Coordinators

              |             |             |

          Automations    Legacy       Radio/cloud
                         systems      adapters
```

The conceptual model describes what the house means.

The technological model describes how those concepts are implemented.

---

# Z-Wave Migration

## Current Setup

The new Z-Wave environment consists of:

- Home Assistant Green
- Raspberry Pi running Docker
- Z-Wave JS UI
- Aeotec ZWA-2 controller
- Aeotec ZW117 Range Extenders

The Raspberry Pi currently acts as a dedicated radio services host.

```
Basement

    ZWA-2 controller

        |

1st floor

    ZW117 repeater

        |

2nd floor

    ZW117 repeater

        |

    Fibaro test device
```

---

# Z-Wave Validation

The migration approach was tested using a Fibaro module on the second floor.

The following sequence was successful:

1. Exclude from new network
2. Include into new network
3. Exclude again
4. Include again

Observed:

```
First inclusion  -> Node ID 4
Second inclusion -> Node ID 5
```

The new network therefore appears capable of migrating devices remotely without physically moving them close to the controller.

---

# Migration Strategy

The planned migration approach:

1. Build a temporary new mesh using existing repeaters.
2. Migrate remote devices first.
3. Allow migrated mains-powered devices to strengthen the new mesh.
4. Migrate controller-near devices later.

Advantages:

- New network becomes stronger during migration.
- Old network remains functional longer.
- Migration does not require all devices to be physically accessible.

---

# Node ID Considerations

Node IDs are not treated as purely internal values because they are also used in administration.

Observed behaviours suggest that Node ID allocation may not simply reuse the first available gap.

Potential strategies:

- First unused Node ID
- Highest known Node ID + 1
- Highest ever assigned Node ID + 1

Further testing is required.

The migration plan therefore avoids depending on Node ID preservation.

# Technological Model

## Purpose

The Technological Model describes the concrete implementation of the house.

It answers:

- What devices exist?
- Which technology is used?
- Which platform owns the device?
- How can it be controlled?
- Where is it physically located?

Examples of technologies:

- Z-Wave
- Zigbee
- FS20
- EMS
- Netatmo
- MQTT devices
- Matter
- ESPHome

The Technological Model is not intended to describe how inhabitants perceive the house.

---

## Example

A technological description:

```yaml
device:
  name: living_room_wall_switch

  technology:
    protocol: zwave
    platform: z_wave_js

  address:
    node_id: 79

  location:
    room: living_room

  capability:
    - switch
```

This describes implementation.

It does not yet describe what the switch means.

---

# Conceptual House Model

## Purpose

The Conceptual House Model describes the house as inhabitants perceive it.

It contains:

- rooms
- functions
- aggregates
- virtual devices
- comfort concepts
- automation concepts

It deliberately avoids technology details.

---

## Example

A conceptual description:

```yaml
room:
  living_room:

    lighting:
      main:
        components:
          - ceiling_light
          - indirect_light

    climate:
      temperature:
        aggregation:
          method: average
          sources:
            - wall_sensor
            - window_sensor

    air_quality:
      co2:
        source:
          - co2_sensor
```

The model describes meaning, not implementation.

---

# Relationship Between Models

The relationship between the two models is a mapping.

Example:

```
Conceptual:

Living Room Main Light


        maps to


Technological:

Z-Wave wall switch
        +
Zigbee LED dimmer
        +
automation
```

The inhabitant sees one light.

The implementation may involve several physical devices.

---

# Three Semantic Namespaces

Home Assistant entities are organised into three semantic namespaces.

## 1. Social Namespace

Entities directly relevant to inhabitants.

Examples:

```
social/house/living_room/co2

social/house/living_room/light

social/house/bedroom/temperature
```

These represent concepts users interact with.

They should remain stable even when technology changes.

---

## 2. Physical Namespace

Entities representing implementation details.

Examples:

```
physical/zwave/living_room_switch

physical/zigbee/living_room_led_driver

physical/netatmo/outdoor_sensor
```

These entities exist because the system requires them.

They normally should not be exposed as user-facing entities.

---

## Example: Combined Light Device

A real-world example:

```
Physical devices:

Z-Wave momentary wall switch

        |

Zigbee dimming transformer

        |

LED lights
```

The user concept is:

```
social/house/living_room/main_light
```

The conceptual model defines:

- which physical devices are involved
- default brightness
- behaviour after switching on

Example:

```yaml
light:
  living_room_main:

    components:
      - wall_switch
      - led_dimmer

    default_level:
      40_percent
```

The generator creates:

- Home Assistant template light
- required automations
- physical mappings

---

## 3. Infrastructure Namespace

Entities required to operate and maintain the automation infrastructure.

Examples:

```
infrastructure/pi4/status

infrastructure/fhem/status

infrastructure/mqtt/status

infrastructure/network/router
```

These entities are not part of the house functionality.

They exist to manage the platform itself.

---

# Infrastructure Self-Healing

Infrastructure entities enable automatic recovery.

Example:

```
Pi crashes

    |

Detection

    |

Power cycle smart plug

    |

Wait

    |

Verify recovery

```

This keeps infrastructure problems separate from house semantics.

---

# MQTT as Semantic Bus

MQTT is not only a transport mechanism.

It is the integration layer between systems.

The principle:

```
Below MQTT:

protocol-specific world

FS20
Z-Wave
Zigbee
EMS
Netatmo


Above MQTT:

semantic house world

lights
temperature
heating
security
energy
```

---

# Coordinator Concept

The coordinator provides the abstraction layer between technology and semantics.

Responsibilities:

- collect sensor reports
- remove duplicate reports
- maintain desired state
- dispatch commands
- publish semantic MQTT topics
- generate Home Assistant discovery

---

# FS20 Example

FS20 devices are mostly one-way radio devices.

They do not confirm their state.

Therefore:

```
Command:

Turn light ON


Coordinator:

desired_state = ON


MQTT:

publish ON


Radio:

send FS20 command periodically
```

The coordinator provides a stable abstraction despite unreliable underlying hardware.

Home Assistant receives:

```
switch:
  optimistic: true
```

because that accurately represents the technology.

---

# Multiple Radio Nodes

The coordinator also allows multiple Raspberry Pis to participate.

Example:

```
              MQTT

                |

        FS20 Coordinator

          /      |      \

        Pi1     Pi2     Pi3

        FS20    FS20    FS20
        TX      RX      TX

```

The radio infrastructure becomes distributed.

The semantic layer remains unchanged.

---

# Semantic Normalisation

Different technologies are mapped to common concepts.

Examples:

Raw:

```
ems/register/0x17/value
```

becomes:

```
house/heating/boiler/flow_temperature
```

Raw:

```
netatmo/module/12/outdoorTemperature
```

becomes:

```
house/weather/outdoor/temperature
```

The consumer does not need to know the original technology.

# FHEM Role

## Purpose

FHEM remains valuable because of its broad support for older and specialised systems.

The role of FHEM changes from being the central automation platform towards being an integration and adaptation layer.

Responsibilities:

- FS20
- legacy hardware
- cloud services
- experimental integrations
- protocol adaptation
- publishing MQTT information

Conceptually:

```
Hardware / Cloud Service

          |

        FHEM

          |

        MQTT

          |

    Semantic Layer
```

---

# FHEM and the Coordinator

Although FHEM can provide access to many protocols, the semantic coordinator should remain conceptually separate.

Possible architecture:

```
             FS20
              |
              |
            FHEM
              |
              |
             MQTT
              |
              |
        Semantic Coordinator
              |
              |
             MQTT
              |
              |
       Home Assistant
```

The coordinator owns:

- semantic mapping
- state handling
- aggregation
- Home Assistant discovery

FHEM owns:

- protocol communication
- device drivers
- legacy support

---

# Docker Strategy

Docker is not considered mandatory everywhere.

The decision depends on whether a service is an appliance or a workbench.

---

## Appliance Services

These are stable components that should be easy to replace.

Examples:

- Z-Wave JS UI
- Zigbee2MQTT
- MQTT bridge services
- other radio services

Recommended deployment:

```
Docker
```

Advantages:

- reproducible deployment
- easy migration
- isolated dependencies
- simple upgrades

---

## Workbench Services

These are systems that are actively extended and experimented with.

Example:

```
FHEM
```

Recommended deployment:

```
Native Linux installation
```

Advantages:

- easy module installation
- easy debugging
- natural integration with Linux tools

---

# Current Physical Deployment Vision

The logical architecture is separated from the physical machines.

Possible deployment:

```
Home Assistant Green

    |
    |
    +-- Home Assistant
    +-- MQTT broker


Radio Pi

    |
    |
    +-- Z-Wave JS UI
    +-- Zigbee2MQTT
    +-- other radio services


FHEM Pi

    |
    |
    +-- FHEM
    +-- FS20 support
    +-- cloud integrations
    +-- coordinator


Other Raspberry Pis

    |
    |
    +-- power meter reader
    +-- photo frame
    +-- specialised local services

```

---

# Vienna and Junglinster

The architecture supports multiple houses.

Each house remains autonomous.

Shared information is exchanged through MQTT.

```
                 MQTT Bridge

Junglinster  <---------------->  Vienna

```

The bridge should exchange semantic information.

Not:

```
Netatmo API details
Z-Wave node information
device-specific state
```

But:

```
house/junglinster/weather/outdoor_temperature

house/junglinster/security/status

house/junglinster/energy/consumption
```

---

# Netatmo Example

Problem:

- One Netatmo account
- Multiple houses
- Multiple logins cause problems

Solution:

One location becomes the owner of the cloud integration.

Example:

```
Netatmo

    |

Junglinster Gateway

    |

MQTT Bridge

    |

Vienna Home Assistant
```

The second house does not authenticate independently.

---

# Vienna Architecture

Potential future setup:

```
Home Assistant Green

    |
    |
Automation
Dashboards


Pi4

    |
    |
Photo frame
MQTT bridge
Possible Zigbee coordinator
Other lightweight services

```

The Pi4 becomes a general infrastructure host.

---

# Matter Position

Matter differs from Z-Wave and Zigbee.

It is not only a radio protocol.

It provides:

- device models
- commissioning
- fabrics
- application-level interoperability

The expectation is that Matter remains closer to Home Assistant.

Conceptually:

```
Matter Devices

       |

Home Assistant

```

rather than:

```
Matter

       |

MQTT

       |

Home Assistant
```

---

# Final Architecture Vision

The complete architecture:

```

                 Conceptual House Model

                         |

                 Technological Model

                         |

          +--------------+--------------+

          |              |              |

   Home Assistant      FHEM       Coordinator

          |              |              |

     Automation     Legacy/cloud    Semantic MQTT

                         |

                    MQTT Backbone

                         |

              Radio and cloud adapters


```

---

# Architectural Principles

## 1. Model first

The house model is the source of truth.

Platforms consume generated configurations.

---

## 2. Separate meaning from implementation

"Living room light" is a concept.

"Z-Wave node 79" is an implementation detail.

They should not be mixed.

---

## 3. Use native APIs when they provide value

Examples:

- Z-Wave JS WebSocket
- Matter native integration
- ESPHome API

Do not replace richer APIs with MQTT unnecessarily.

---

## 4. Use MQTT for decoupling

MQTT is appropriate for:

- semantic events
- integration between systems
- gateways
- multi-house communication

---

## 5. Let each platform do what it does best

Home Assistant:

- automation
- dashboards
- user interaction

FHEM:

- legacy systems
- unusual integrations

Radio services:

- protocol handling

MQTT:

- communication backbone

---

# Final Concept

The goal is not a collection of configured devices.

The goal is a model of the house.

Devices, protocols and platforms are replaceable implementations behind that model.

```
The house is the model.
The platforms are runtimes.

