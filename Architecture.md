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
- **Logical**: devices that provide entities. A device is not yet a "real" entity until it is positioned in a space. Devices may themselves be combined from other devices/entities, via two distinct operations (noted 2026-08-22, not yet designed at the DSL/generator level — gated on Z-Wave and Zigbee2MQTT both being MQTT-based, Phase 3 steps 3-4 in PROJECT.md):
  - **Aggregate**: combine devices into a genuinely *new* logical device that isn't really either constituent alone — e.g. an external temperature sensor merged into a smart plug to form one logical "heater" device.
  - **Absorb**: one device becomes subordinate to, and extends, an existing "master" device rather than forming something new — e.g. `switch.infrastructural:garage/smarty` (a Zigbee-controlled power switch for the `smarty` host) absorbed into `host.smarty` as its on/off switch capability; or the common Z-Wave/Zigbee pairing of a Z-Wave wall switch (linked to a physical wall switch) absorbing the Zigbee dimmer light(s) it actually controls, forming one logical light.
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
design note (PROJECT.md) for why this calls for a conceptual-to-conceptual mechanism (state
mirroring, closer to HA's own `mqtt_statestream` pattern) rather than the physical-layer discovery
path "hosts"/"discovery" use.

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
binary sensors, REST/CLI-imported sensors, "hosts" integration attribute typing, and — the one
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

  Known gap against both principles in the current implementation: it is thoroughly filename-driven. `generateFromPaths` reads `Spaces.def`/`Settings.def` by hardcoded name; `resolveBridgeTargets` reads `Settings.def`/`Bridges.def` by name; `resolveHomeAssistantTarget` and `resolveMainIncarnationName` both read `Physical.def`/`Settings.def` by name. Each is also its own separate read pass, interleaved with (and in the incarnation-name case, *before*) entity parsing, rather than one unified read-everything-then-interpret pass. Don't grow more filename-keyed side-channel readers in the meantime — this is the concrete shape the semantic-model layer above needs to dissolve.

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

**Roaming infrastructure devices** (noted 2026-08-21): laptops physically move between Junglinster and Vienna — and beyond, since they also travel away from both houses entirely. Rather than each laptop's `cpu` report script targeting whichever house's local broker it happens to be near (fragile — needs reconfiguring, or picking one house arbitrarily, every time it moves; and simply unreachable while away from both houses), it should report to a shared **bridging MQTT broker** set up for exactly this purpose, with each house's own coordinator/instance importing the relevant topics from there instead of owning them locally. This means the bridging broker must itself be reachable from anywhere the laptop's local network allows outbound access to it — not just LAN-local to either house, unlike each house's own local broker today — so a laptop keeps reporting its CPU data even while away from both houses, whenever it has network access. Same federation shape as the Netatmo cross-home example (§6.5) — a device whose "home" isn't fixed needs its data to flow through a broker neither house directly owns — just for infrastructural monitoring data instead of cloud-sourced semantic data. Not yet designed further than this (no bridging broker exists yet; today's laptops — `eriks-macbook-pro-2`, `paulas-air-m1` — report to Junglinster's local broker only, per Physical.def's current `hosts` integration declarations). Whatever host ends up running the bridging broker itself is just another `cpu`-type "hosts" integration device once it exists — no special-casing needed, the same self-monitoring applies to it too.

---

## 13. Open design questions

- Should **provider** become a first-class DSL concept (as opposed to today's `providing` macro pattern)?
- How should **projections** (to MQTT, to HA YAML, to future backends) and **transformations** be represented in the semantic model — and how should reusable patterns across them work?
- Cross-check **HA areas vs. DSL spaces** — are they the same concept, and if not, how do they relate?
- `Integrations.def` grammar is still being worked out by direct experimentation (§9.2) rather than designed up front — expect it to keep changing shape for a while before the physical/logical file split (§9.1) is worth actually implementing in the parser.
- How a `space` definition refers to a device attribute (constant/variable, device/world — §6.6) rather than only a device, and how that then materializes as a generated entity, is still open.
- ~~Per-integration default icons (noted 2026-08-21)~~ — **done, see §6.8** (2026-08-26): built as the `defaults: for <domain>.<pattern>: ...; end;` grammar plus `Shared/Definitions/Defaults.def`, generalized well beyond icons to device_class/unit/state_class too, and applying to every integration kind, not just "hosts".
- **Raw/extensional entity references should become unnecessary post-MQTT-migration** (noted 2026-08-22): `domain.[raw_entity_id]` declarations in Spaces.def (e.g. `entity sensor.[ems_esp_boiler_boiler_outside_temperature];`) duplicate what a `device.<spec> from <device-id> with: ...;` link already identifies, for any entity whose underlying device has (or could have) such a link. Once the MQTT migration is complete, audit remaining `.[...]`-style declarations and see which can be replaced or removed. Not yet scoped as an implementation task.

### Two-level parser sketch (from early notes, unimplemented)

```
node             ::= NodeForPlatform
WithClause(f)    ::= "with", (":", f | GroupClause(f))
NodeForPlatform  ::= <name> WithClause(MyWithClause)
```

Kept here as a placeholder for whenever the `Integrations.def`/aggregation-file grammar gets formalized — not a commitment to this exact shape.
