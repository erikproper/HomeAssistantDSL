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

**Where physical-layer derivation ends and logical-layer combination begins** (2026-09-09, design
in progress, not yet built — see PROJECT.md item 4 and `splendid-zooming-lighthouse.md`): a
capability's value can be *derived* from exactly one sibling capability of the same physical
device (e.g. a `battery_alert` binary_sensor thresholding that same device's own `battery_level`)
entirely within the physical layer — the transformation is expressed once, in Physical.def, and
realized as an MQTT discovery `value_template`, so it is the *receiving* HA instance that evaluates
it on ingest, never the coordinator or the source integration itself (resolving the "who does this
derivation work" question this project has carried since the MQTT migration: not the source
platform, since not every integration — Zigbee2MQTT, for instance — can define a derived entity of
its own; not the coordinator, since that would mean re-implementing HA's own templating engine for
no benefit). This gives a precise, mechanical line between the two layers: **a transformation over
the value of exactly one base entity is physical-layer work; anything needing more than one base
entity's value is logical-layer work by construction** — aggregate/absorb above, and multi-device
choreography (turning on a smart dimmer whose mains power is switched by a separate device,
sequencing "switch relay on, wait, then set brightness") both fall on the logical side of that line
for exactly this reason, not merely by convention.

One deliberate exception: availability/liveness ("is available") stays a physical-layer primitive
even though it is itself a kind of derived value — it can require a synthetic, time-out-based check
(an entity considered unavailable after N minutes of silence) that is not a pure function of one
payload value at all, so it can never fit the single-base-entity substitution model above, however
many base entities it might otherwise reference.

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
- **Meta** (2026-09-06 to 2026-10-26, now part of infrastructural) — entities that operate the DSL/generator/coordinator system itself: the "Discover entity" bootstrap control (§6.9) and the reload/restart buttons. Asking whether "meta" is not simply "infrastructural" answered it (PROJECT.md item 18): they are `text.infrastructural_discover_entity` and `button.infrastructural_<reload|restart>_<instance|installation|all>` now, named `infrastructural/...`. Their unique_ids keep the `meta_` root (`meta_discover_entity`, `meta_reload_main`, ...), so they stayed the same entities, and the generator's registry table moves them; their MQTT command topics (`meta/discover_entity/set`, `meta/<action>/<target>/request`) are internal plumbing and kept their names.

**Legacy note**: both real houses' current Spaces.def files also use `physical:` as a sphere
(206 occurrences combined, confirmed live 2026-09-06) — a naming choice from before the
conceptual/logical/physical layer split above was established, when "physical" hadn't yet been
claimed as a layer name and so its reuse as a sphere name wasn't yet a collision. Not part of the
sphere standard above, and deliberately not migrated: retiring it would mean renaming ~200
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

This is also why discovery is preferred over generator-authored YAML MQTT template sensors wherever a producer already speaks it — and not merely preferred for convenience, but structurally required for anything that needs device metadata at all. A template-based entity (static `mqtt: sensor:` YAML, or HA's template platforms) can only ever extract *state* from a payload via `value_template`/`value_json.*` — the exact same mechanism discovery's own `value_template` field uses internally. Neither path offers a way to populate an entity's `device:` metadata block (manufacturer, model, `device` grouping, sw/hw version) from a live MQTT payload; that block is only ever set once, at config-publish time, never templated per message, in *either* mechanism. So a template sensor doesn't just risk the metadata going stale — it has no mechanism to carry that metadata at all, short of the generator hand-authoring and continually re-publishing it itself. Discovery sidesteps the problem entirely because Z2M and Z-Wave JS already know and supply that metadata (manufacturer, model, `device` grouping, availability, unit/class hints, ...) as part of their raw discovery payload, sourced from their own device database — refinement just lets the coordinator correct and reposition it. Discovery also, independently, makes a device's existence known to the model before it's been consciously dealt with at all — e.g. surfacing it in the "undeclared entity"/suggestion-list reports (§6.9, `project_undeclared_entity_audit_technique` in memory) — which a hand-authored template sensor, declared only once someone already decided to write it, cannot do.

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

**Design rationale: passthrough naming is a robustness liability, not just a cosmetic gap.** A live incident on Vienna (2026-09-09, see README.md's "Operational notes") showed why finishing this migration matters beyond naming consistency. Home Assistant's MQTT integration treats a missing/empty retained discovery config as an instruction to *delete* the entity outright, not merely mark it unavailable — unlike most other integrations, which leave an orphaned entity behind when a device stops reporting. For an entity still on raw Zigbee2MQTT passthrough, the `entity_id` HA shows is whatever a human manually renamed it to at some point after creation, entirely outside this codebase's control; if the registry row is ever deleted and recreated (broker losing retained state, a bad reload, or any other event that makes the config genuinely disappear for a while), the entity comes back under Zigbee2MQTT's own raw `friendly_name`-based naming, and the manual rename is lost with no record of what it should be. An entity that has been moved to true model-driven refinement doesn't have this problem: the coordinator itself authors the discovery config, including its `object_id`/name, so it can set that name to already match the DSL's naming convention. Even in the worst case — the registry row deleted and the entity recreated completely from scratch — the freshly-created `entity_id` comes back correct immediately, because there is no longer a human-only, unrecorded naming step to lose. This is a concrete, incident-tested reason (not just tidiness) to keep moving entities off passthrough and onto real model-driven refinement over time.

**Junglinster switched its own `discovery_passthrough` off, 2026-10-05 (PROJECT.md item 2), once every real Zigbee2MQTT device was confirmed migrated to proper discovery-kind declarations.** The mechanism itself stays in the generator/coordinator unchanged — it's deliberately reusable infrastructure for *any future* legacy-to-conceptual migration (a different house adopting this architecture from scratch, a future protocol gateway, re-onboarding after a broker rebuild), not something scoped to Junglinster's own one-time migration. Turning it off for a specific house is just commenting out (or never writing) that house's own `discovery_passthrough "<prefix>" from "<source>";` line in its Physical.def — the coordinator code path, `discovery_passthrough_suggestions.go`'s own "what's still unclaimed" report, and this section's whole design rationale remain exactly as useful the next time they're needed.

**A second incident, the same day, makes the deeper motivation explicit.** Fixing 4 heating relays (discovered mid-migration to be raw Zigbee2MQTT devices wrongly declared as native `home_assistant`-bridge entities) meant moving their entity_id from one HA platform (`template`) to another (`mqtt`, coordinator-published). Same conceptual device, same intended `entity_id`, but HA's registry keys identity off `(platform, unique_id)`, not the bare `entity_id` — so the switch required a full stop-core/edit-registry/start-core pass to free the name before the new platform could claim it cleanly (§6.11-style registry surgery, not just a file deploy). This is exactly the class of friction the whole MQTT-mediated design already exists to reduce — routing every device through one coordinator-owned bus, rather than binding a conceptual entity directly to whichever native integration happens to back it today, is what makes it *possible* to swap the backing mechanism at all without the entity's own history (automations, dashboards, the user's own mental model of "this switch") breaking. The passthrough-off mechanism above and the still-unimplemented "stable entity identity" design ([[project_stable_entity_identity_design]] in memory; PROJECT.md's own "stable coordinator<->HA entity identity" item) are two instances of the same goal taken further: an entity's *identity* should survive changes to its own backing mechanism, not merely its physical implementation. Passthrough already does this for "which integration first discovers the device"; stable identity would do it for "which unique_id HA remembers it by," eliminating today's manual registry-surgery step entirely once built.

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

**Availability composition is a further consequence of this split (2026-09-16).** Ideally every
atomic capability (a physical device's own reading/control) reaches the main instance over MQTT —
which is what makes it possible to refine one device's availability by ANDing in another's: Logical.def's
`dependency on <device-id>;` (resolved generator-side into a flattened, cycle-checked
`depends_on_availability:` topic list, ANDed together by the coordinator's `buildAvailabilityFields`).
That mechanism only works for MQTT-published entities (`hosts`/`discovery`/`hassbridge`/`import`).
An entity domain from this section's own exception list (`weather`, `media_player`, ...) is instead
hard-wired straight into the main instance via a local HA integration — never routed through the
coordinator, so there is no MQTT availability topic for `dependency on` to attach to. For those,
availability has to be expressed the other way round: as a Jinja `availability:` template on the
generator-authored YAML entity itself — the mechanism Logical.def's own `derived binary_sensor.node
with: condition ...; end;` capability shape already uses (e.g. Vienna's `apple_tv`/`living_room_tv`
`node` entities, reusing `buildConditionBinarySensorYAML`'s existing condition-entity rendering).
The two mechanisms don't share one code path — they're deliberately separate, matching which side
of the MQTT boundary the entity in question lives on.

**Generated template entities get their availability from their sources (2026-10-25, PROJECT.md
item 17b, `template_availability.go`).** HA's own groups already turn unavailable with their
members; a template entity would otherwise keep showing a value (a derived sensor's `| float(0)`
turning a missing meter into `0`). The rules, per kind:

- a derived value (template sensor, or a binary_sensor whose condition reads values, e.g. `brews`
  from power > 200): available only while **all** sources have a value -- `has_value(...)` for
  every entity_id the rendered state expression reads (`templateSourceIDs`);
- a template aggregate (space light, space switch, space media switch): available while **any**
  member is, like a group; `availability all;` in a space (`SpaceAvailabilityAll`) makes that
  space's own aggregates need every member -- not its child spaces, and never the sphere-wide
  aggregates;
- no `availability:` for an entity that tests availability itself -- a node, a battery alert, an
  "is available" condition (an expression containing `'unavailable'`, `'unknown'` or
  `has_value(`): it has to say `off` exactly when a source is gone, not become unavailable too;
- none for the request events (they only fire) and, deliberately, none yet for the generator's
  internal heating/cover logic (`should_be_off`, `leakage_identified`, ...), whose automations
  would behave differently with an unavailable input.

**`dependency on <id>;` is actually two separate compilation paths, not one (2026-10-04, revised
2026-10-11).** `integration_logical_storage.go`:

- Device-level, on a hassbridge/import-kind device (`resolveDependencyAvailabilityTopics`): walks
  the full transitive dependency graph (`resolveTransitiveDependencies`, cycle-detected), resolves
  every `hosts`-kind dependency to its own raw MQTT node/liveness topic, and feeds the flattened
  list straight into the coordinator's native MQTT `availability:` topic list
  (`depends_on_availability`) — this is the path described just above, ANDed by MQTT/HA itself, no
  generator-authored template at all.
- Capability-level, inside a capability's own `is available with: ...; end;` block
  (`TLogicalCapability.DependsOn`, resolved by `resolveLogicalDependencyNodeEntities`): a flat,
  non-transitive, one-hop resolution — each `hosts`-kind target gets its own entity_id (reusing an
  existing positioning, or auto-materializing a synthetic "physical/&lt;id&gt;/node" entity if none
  exists yet) and those are folded as literal, already-resolved `is_state(...)` strings directly
  into the capability's own ConditionExpr.

**No node in the "physical" sphere** (2026-10-26, Erik): a node is always infrastructural. When a
logical node's own condition refers to a node that has no positioning of its own, the generator
never materializes it as "physical/&lt;id&gt;/node" (`resolveNodeInput`,
`Conceptual_LogicalEntities.go`). A device's own "&lt;capability&gt; is available" node is inlined
into the condition; any other node becomes an entity next to the node it feeds,
"infrastructural/&lt;node path&gt;/&lt;qualifier&gt;/node": the host's integration (`ping`, `cpu`,
plus the host's own id leaf when the condition pings several, e.g. `sonos/ping/sonos_left/node`),
else `controller` (the device's own link to its controller, e.g. Vienna's ha2mqtt-bridged washing
machine). Generate warns about any node outside the infrastructural sphere (`checkNodeSpheres`).

**"enabled by"** (PROJECT.md 17k, 2026-10-26, Erik): a Logical.def device statement
`enabled by <domain>.<label> from <device> [as <id>] [with delay_off <time>];` says the device is
powered by that device's switch (`logical_enabled_by.go`). It absorbs the switch as the device's
`enabler` and implies its node: (its own Physical.def node, if any) and (the node of every device
it is built from -- absorbed, colocated or `dependency on`) or (the enabler is off); the `as` and
`with delay_off` belong to that node, and the generator writes the `as` back like any other frozen
unique_id. A device that declares its own node keeps it, with "or the enabler is off" added --
never a rebuilt base (the lesson of the earlier sugar removed in item 11a).

**Enabler nodes** (2026-10-26, Erik): a device enabled by another device's switch ("is switched
by ... from &lt;device&gt;", or `enabled by`) also gets "&lt;its node path&gt;/
enabler/node", a template following the enabling device's own node, which stays where it is
positioned (e.g. a relay in the fuse cabinet). Its unique_id is the device node's frozen one plus
"_enabler", so it needs no line of its own in Logical.def (`logical_enabler_nodes.go`). A
switched device's own node also counts as up while what switches it is off (a light behind a relay
is unreachable exactly then) -- added by the generator, the same rule a logical device with an
absorbed enabler spells out itself as `or jinja "$[switch.enabler] == 'off'"`.

Two `dependency on` lines on one device: the device-level path is repeatable and fully transitive —
if A depends on B and C, and B itself depends on D, A's own resolved chain is B+C+D, deduped, with a
cycle reported (and the whole dependency list ignored) if the graph loops. The capability-level path
is repeatable too, but each target is resolved as its own flat one-hop lookup (no further walk of
*that* target's own dependencies) — a documented simplification, not a hard limit; extend it the day
a real case needs more than one hop.

**A third path -- a device-level sugar for any non-hassbridge/import kind, or a hassbridge/import
device with "enabled by" -- existed from 2026-09-28 to 2026-10-11, then was removed (PROJECT.md item
11a).** `applyLogicalEnablerDependencySugar` folded each dependency into a literal "and
&lt;dep-id&gt;" clause, but built its OWN base self-check from the named `enabled by` capability's
"is available" state (e.g. `media_player.core is available`) instead of referencing the device's
real Physical.def `node` condition via `physical &lt;id&gt;`. For every real device that used it,
that base self-check happened to be byte-for-byte identical to the real `node` condition — but only
by convention, never enforced: had a device's own `node` ever been declared as anything richer than
"this one capability is available," the sugar would have silently discarded the extra part instead
of ANDing onto it, rather than reflecting the real condition the way the hassbridge/import native-
MQTT path above always correctly does. Found live 2026-10-11 reviewing Vienna's own
`appliance.living_room_tv` (`dependency on node.tv; delay_off 00:01:00; ... media_player.core
enabled by switch.core from switch.living_room_tv;`) with the user. Fixed not by patching the
sugar's own self-check, but by removing it entirely: every device that used to rely on it now spells
the exact same thing out explicitly —

```
device appliance.living_room_tv with:
  colocated switch.living_room_tv as switch;

  absorb from switch.living_room_tv:
    capability switch.enabler core;
  end;

  derived capability binary_sensor.node with:
    condition (physical appliance.living_room_tv and node.tv) or jinja "$[switch.enabler] == 'off'";
    delay_off 00:01:00;
  end;
end;
```

— the same style every hand-written case in both houses (`appliance.apple_tv`, `appliance.sonos`,
`server.xanadu`, `server.backups`, ...) already used. `physical &lt;id&gt;` (`ParsePhysicalShorthand`,
`logical_condition_expr.go`) forces resolution of the device's own real, raw Physical.def `node`
capability (auto-materializing it as its own entity if it wasn't already one), so the dependency is
genuinely ANDed onto whatever that capability really says, not an independently reconstructed
approximation of it. Deployed live to both houses (5 devices total: Vienna's `appliance.living_room_tv`;
Junglinster's `appliance.tv_living_room`/`appletv_bedroom_sarah`/`appletv_office`/`samsungtv`,
plus `sonos_bathroom`/`sonos_shower_room` which needed no "enabled by" and so already matched),
each gaining one new intermediate "physical node" entity as a result — a genuine behavior
fix, not a no-op refactor, confirmed healthy against live state after redeploy/restart. (The
`capability`/`absorb from` keywords shown in the block above were themselves added a day later,
PROJECT.md item 11b, 2026-10-11 -- see this file's own note on that item, just below the
"dependency on" writeup, for why.)

(A derived capability holding nothing but its `condition`/`value` may also be written as a
one-liner, `derived capability binary_sensor.node as <id> with condition <expr>;` — 2026-10-21, the
same shape Physical.def's own derived one-liner has. The `condition`/`value` may also start on the
next line.)

**Derived events (PROJECT.md item 49, 2026-10-22).** A request raised by several buttons is a
logical device of its own, with one derived `event` capability:

```
device request.shower_room_light_toggle with:
  derived capability event.toggle as event.social_apartment_shower_room_light_main_toggle with
    condition
      event.pressed from buttons.shower_room_door_moes_1 is 'single' and if
      trigger.button from buttons.shower_room_door_moes_1 is '1'
    or
      event.pressed from buttons.shower_room_mirror_moes is 'single' and if
      trigger.button from buttons.shower_room_mirror_moes is '1';
end;
```

Its condition is an `or` of terms, each an `event.<label> from <device> is <value(s)>` match,
optionally `and if trigger.<attribute> from <device> is <value(s)>` (filtering on the firing event's
own attribute). Conceptual.def positions it (`device request.… with: entity event.social:light/main/
toggle; end;`); the buttons' raw events never appear there -- like the stacked lights' plumbing,
each source is auto-materialized as a `physical` entity (`event.physical_<device-id>_<label>`,
under its frozen Physical.def ID), on the final resolution pass so an explicit positioning, if any,
is found first. The generator writes one trigger-based
template `event` entity (generateNamedTriggerEvents) raising `raised`; rules use `when
event.social:light/main/toggle is raised`. Such an event-only device has no state, so it needs no
`node` (Rule 1 and item 8's checks skip it). The condition grammar's `is '<value>'` / `is in [...]`
works in binary_sensor conditions too (`is_state(...)`).

**Frozen universal IDs (PROJECT.md item 48, 2026-10-21).** Every derived capability, in Physical.def
or Logical.def, carries its universal ID right after its name, before its `with`/`from` part
(2026-10-22): `derived capability <domain>.<label> as <id> with …;` / `… as <id> from …;` (one-liner)
or `derived capability … as <id> with:` (block). A bridged capability carries it after its source:
`capability <domain>.<label> <source> as <id> [with …];`. A commandline capability carries it
after its name, before its scripts (`capability switch.picture_frame as <id> "<status>" …;`,
2026-10-22); it is the capability's unique_id in its YAML entity (commandline_yaml_entities.go),
so the capability can be renamed. Its MQTT topics still follow the label. It is the `unique_id` of the template entity generated from
it. The generator writes it itself the first time it sees the capability: its entity_id at that
moment, which is exactly the unique_id it had before IDs were frozen, so freezing changed nothing
in Home Assistant. It is never recomputed, so renaming the entity or the capability keeps the same
Home Assistant registry entry (and its history). A capability feeding more than one entity (e.g.
positioned directly and also absorbed by a logical device) gets no ID; generate reports it.

**Home Assistant's entity identities, and the registry repair at deploy (2026-10-26, Erik).**
Home Assistant keeps four separate identities per entity, and only one of them follows our defs by
itself:

| Identity | What it is | Set by | Changes later? |
|---|---|---|---|
| unique_id | the entity's permanent identity, the registry's key (per platform and domain) | the integration: our frozen `as <id>` | never |
| entity_id | the handle automations, templates and our generated YAML refer to | Home Assistant, once, when the unique_id first appears (from the suggested `default_entity_id`/name) | only by a registry rename |
| name | the entity name the UI shows (registry name / original_name) | the integration (discovery/YAML `name`, PROJECT.md 17h), or the user | when the integration sends another one |
| friendly_name | a state attribute | Home Assistant, or our customization | with the customization |

Once a unique_id is in the registry, Home Assistant keeps its entity_id: a template or discovery
config suggesting another one is ignored. That is deliberate on Home Assistant's side (a user's
rename in the UI must survive the integration), but for a setup whose source is the defs it means
a moved or renamed entity keeps its old entity_id, while the generator already refers to the new
one. Reusing the unique_id elsewhere doesn't help (the registry keys on it per platform; a new
platform is a new entity), and deleting the entry loses its history.

So the deploy repairs the registry, driven only by the generator's own table: every instance's
`registry_manifest.json` lists, per generator-owned entity, its unique_id and the entity_id it
should have. `tools/ha_registry_sync.py` (`deploy.d/01_deploy.hass.*`, before Home Assistant
reloads the new YAML) compares that with the registry and renames each entry whose entity_id isn't
current -- the same registry entry, so its history and settings stay. It only touches entries it
can prove are the generator's own (a YAML-platform entry whose unique_id is in the current or the
previous manifest, or an MQTT entry whose unique_id is a frozen stable_id), and skips and reports a
rename whose target entity_id is held by anything else. A unique_id that disappears from the
table goes from Home Assistant too (Erik, 2026-10-26): the repair removes the generator-owned
entries the generator no longer produces (e.g. replaced wrapper switches). Entities a native
integration provides (Ring, Sonos, Apple TV, ...: Physical.def "integration local" capabilities
the generator only refers to) are in the table too, as "native": their unique_id is the
integration's, unknown to the generator, so each is keyed by its source in the defs
("<device-id>!<capability>"); when the entity_id expected for a source changes from the previous
deploy, the entry still holding the previous one is renamed (PROJECT.md 17j). A native entity is
never removed -- it belongs to its integration. Nothing else edits the registry: no ad-hoc
or hand-made changes; any other registry change is an explicit, one-off migration.

**This is a real dependency on Home Assistant's internals.** The repair uses Home Assistant's
websocket API (`config/entity_registry/list`, `…/update` with `new_entity_id`, `…/remove`) -- the
interface its own UI uses, so more stable than the `.storage/core.entity_registry` file, which
the repair never reads or writes. But should Home Assistant change how the registry keys entities,
when it fixes an entity_id, or that API itself, renames in the defs stop reaching Home Assistant
(entities then keep their old entity_id, and the generated YAML refers to ids that don't exist).
Not liked, but accepted: there is no other way to move an existing entity's entity_id. PROJECT.md
item 19 is where this could be avoided altogether -- by freezing entity_ids at birth and letting
only the name follow the position.

**Every capability-declaring line, in every Physical.def/Logical.def integration kind, now
carries an explicit `capability` keyword (PROJECT.md item 11b, 2026-10-11).** Before this, a
capability was declared bare -- `media_player.core;`, `sensor.battery_level 0x...;`, `derived
binary_sensor.node with condition ...;` -- visually indistinguishable from a device-level FACT of
the same general shape (`<domain>.<label> is switched by ...;`, a refinement-block body line) on a
quick read. The keyword goes at the very front of a bare line (`capability media_player.core;`),
or right after `derived`/`defined` for those two block-opening forms (`derived capability
binary_sensor.node with: ...;`, `defined capability input_number.x with: ...;`) -- block openers
that DON'T start with `derived`/`defined` (hassbridge/local's own `<domain>.<label> <value> with:`
capability-refinement block, which nests further modifier lines rather than declaring a second
capability) stay unprefixed, same as any other device-level fact. Logical.def's absorb-block
opener was renamed from `capabilities from <id>:` to `absorb from <id>:` in the same change --
"absorb" already being this mechanism's own internal name everywhere else in the codebase
(`TLogicalCapability.AbsorbedFromDeviceID`/`AbsorbedFromCapability`) -- with its own body lines
getting the `capability` keyword too. Purely a syntax change (a one-off migration tool, `capability_keyword_migration.go` -- deleted once
spent, same precedent as item 10's own migration tools -- rewrote ~1600 real lines across both
houses' entire Physical.def/Logical.def in one pass, via a single kind-agnostic classifier
covering every integration's own capability-line shape, verified by hand against a dry run before
touching the real files) -- both houses regenerate byte-identical, so no deploy was needed.

**A non-bridgeable entity pulls its whole device onto the same side of the boundary (2026-09-24).**
The workaround above is per-*domain*: `weather`/`media_player`/etc. themselves can't be MQTT-
discovered. But a device can mix a non-bridgeable entity with otherwise perfectly MQTT-capable
ones — a Ring doorbell's own camera (native-only) alongside its ding/motion/battery `binary_sensor`/
`sensor` entities (both MQTT-discoverable domains in isolation). Splitting such a device across the
boundary — camera hard-wired locally, the rest relayed over MQTT — would fragment one physical
device's entities across two completely different registration mechanisms for no benefit; the whole
device is instead declared through Physical.def's `local` integration kind (`camera.core;`,
`binary_sensor.ding;`, ... all as bare "possibly locally defined" acknowledgements, confirmed to
really exist by Conceptual.def's own positioning, same existence-tracking discipline a plain bare
`entity <spec>;` declaration already has — `integration_local_parser.go`/`Conceptual_LocalEntities.go`).
So the practical rule is: one non-bridgeable entity anywhere on a device is enough to pull every
entity of that device onto the `local` side, even the ones that individually could have gone via MQTT.
This is also why the media-player devices from this section's own `apple_tv`/`tv`/`sonos` examples
were later folded onto the exact same `local` mechanism (2026-09-24) — a `media_player`'s own
combined-availability "node" and `switch.media` coercion aren't separately bridgeable either, so the
whole device belongs there, not split.

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


**Display precision and display unit (PROJECT.md 24, 2026-10-28).** A defaults rule may also give
`precision: <decimals>;`. Precision and unit are not written into any YAML, but set as Home
Assistant's own per-entity display options -- the same as the UI's "Display precision" and "Unit of
measurement" -- by the deploy's registry repair: the generator lists them per positioned sensor in
the registry manifest (`display`), and the repair sets them where they differ, keeping every other
option key. That reaches every kind of sensor alike -- discovered, bridged, imported, native, and the
generated templates (aggregates, averages), which have no `suggested_display_precision` in YAML at
all. Rounding is for display only: state, history and statistics keep full precision. A display
unit is only set for a sensor with a device class whose shown unit differs; Home Assistant converts
the value itself (pressure: `hPa`, the SI form -- mbar is the same value under another name). The
defs win: a precision set by hand in the UI is overwritten wherever a default applies. A failed
update is a warning, never a failed deploy. Defaults (Shared/Definitions/Defaults.def): temperature
and wind speed 1 decimal, energy 2, humidity, pressure, CO2, illuminance, noise, load, power and
battery 0.
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
(The numbering below is this section's own; §6.16 has the five integration kinds as they stand
since item 19, with a mapping.)

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

**Kind 5 (bare "main instance" entities) built and deployed live, 2026-09-07/08** (PROJECT.md
items 1/1b) — the last of the three conceptual-layer cases from this section's own layering
principle: a `Spaces.def` entity declared bare (`entity sensor.physical:door/aqara_multi/temperature;`
— no `value`/`condition`/`from ... entity ...`), assumed to already exist as a native entity on the
house's own main HA instance directly, predating all device/MQTT machinery. Reuses kind-3's own
`TEntityExistenceTracker` engine (active inquiry, sibling discovery, retained status publish)
almost entirely as-is, seeded under instance `"main"` — no new coordinator machinery, no new
DSL/Spaces.def grammar. Generator side: `collectMainEntityIDs` (`main_entities.go`) filters
`EntityRecordsBySpace` on `!HasDefinitionOrImport && !DiscoveryImplied` and converts each via the
same `toHomeAssistantEntityID` conversion used everywhere else in the generator — honouring this
section's own "always the declared source, never the conceptual name" rule even though, for a bare
entity, the two happen to be spelled the same way. `checkMainEntityKnownNotToExistErrors`
(`mqtt_entity_existence.go`) mirrors `checkKnownNotToExistErrors`'s hard-fail-on-confirmed-absence
shape; `generateEntityCatalogueSuggestions` folds `mainEntityIDs` into its own `used` set for
instance `"main"` so an already-declared bare entity is never suggested back as unclaimed.
Confirmed live: Junglinster's first hard-fail run under kind-5 flagged 21 genuinely-absent bare
entities.

**Starvation bug found and fixed live, 2026-09-07/08**: `nextToInquire` always rescanned the
not-known-to-exist backlog from index 0, so a single entry whose reply never arrives (confirmed
live case: a typo'd entity name, `sensor.social_terra_sun_solar_elevation` for the real
`sensor.social_terrace_sun_solar_elevation`) permanently starved every entity registered after it
— no false hard-fail was ever raised for it across multiple `./generate` runs over several hours,
which is what surfaced the bug. Not specific to kind-5 (the same tracker backs kind-3 too), just
first exposed by kind-5's one-time bulk seed of ~144/~28 bare entities per house. Fixed via a
`backlogCursor map[string]int` that rotates the scan's starting point on each call instead of
always restarting from index 0. Deployed to both houses; confirmed live that each house's full
backlog drained to completion (Vienna 395/395, Junglinster 1242/1242, 22 confirmed-absent) with no
entries left stuck.

**"Confirmed known-not-to-exist" downgraded from a hard build failure to a report, 2026-09-21
(PROJECT.md item 1a, "stable ID-based link" architecture)** — this section's own hard-fail checks
(`checkDiscoveryKnownNotToExistErrors`/`checkKnownNotToExistErrors`/
`checkMainEntityKnownNotToExistErrors`, plus kind-4's `checkImportKnownNotToExistErrors`) had grown
disproportionate: a single declared entity's physical source going away (a dead Zigbee battery, a
rebooting router) aborted `./generate` for the *entire house*, blocking every other unrelated
change until the operator fixed or removed the one declaration. Agreed principle (memory:
`project_stable_discovery_identity_architecture.md`, written after the Junglinster xanadu/disks_1
passthrough name-collision incident): a vanished physical source must be *reported, not acted on*.

Before touching anything, the four candidate "acting on it" mechanisms elsewhere in the coordinator
were re-read and confirmed to already satisfy the principle as-is, so none were changed:
`TDiscoveryPublisher.RetireMissing`/`watchForOrphanedDiscoveryTopics` (`discoverycleanup.go`) only
ever retire a topic when the *operator's own* Physical.def/devices.yaml declaration stops producing
it, never because a physical source vanished at runtime; `Publish`'s content-changed
retire-then-republish re-registers under the *same* topic (same `unique_id`/`entity_id`), a
same-identity refresh, not a rename or deletion; and `discoverybridge.go`'s empty-payload handler
already tracks retraction correctly (`existenceTracker.MarkRetracted`) without ever actively
retiring a *declared* entity's own relayed HA topic (only genuine raw-passthrough devices get
retired there).

The four check functions themselves changed from returning a hard `error` to returning
`[]string` problems, collected by `Physical_Generator.go` into one `TMissingEntitiesReport`
(new file, `missing_entities_report.go`) and written to `suggestions/missing.txt` — same
non-blocking/stale-file-cleanup convention every other suggestions report already follows.

The other half of the principle — a live indicator, not just a generate-time report — is a new
coordinator-authored `binary_sensor.missing_declared_entities` (`device_class: problem`,
`missing_declared_entities.go`), discovery-published directly by the coordinator (mirroring
`meta_reload_restart.go`'s own self-authored-entity style, local broker only). It aggregates all
three existence trackers (`TDiscoveryExistenceTracker`/`TEntityExistenceTracker`/
`TImportExistenceTracker`, each gaining a `KnownNotToExistItems()` snapshot and an `onChange`
callback fired only on a genuine transition into or out of known-not-to-exist), debounced/
periodic-published exactly like every other tracker's own status mechanism in this file (30s
debounce, 10min safety net). Deployed and confirmed live on both houses.

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
scenario this was built ahead of; 1.2d is the import mechanism itself. Command topics now exist
for the same-broker case (§6.13) but not yet across this `export`/import cloud-broker link —
that's §6.13's own "Phase 2" note, not built yet either.

**An export's persistent identity (PROJECT.md 22, 2026-10-28).** The cloud config topic of an
exported capability is keyed by a name-based id (`exportStableID`: installation, device id,
capability), so renaming the exporter's device or capability used to break the import until the
importer's declaration followed (2026-10-27: Junglinster's Vienna Netatmos renamed, five Vienna
import lines adjusted by hand). Now every cloud config also carries an `export_id`, which no
rename changes:
- a bridged capability: its frozen universal ID (`hassbridge_sensor_infrastructural_netatmo_bedroom_co2`);
- a host: `host_<hostname>_<attribute>` (`host_pro1_cpu_load`) -- the machine's own name, already
  in all its topics and independent of its device id; hosts declare no capability `as` IDs.

The importer names it per capability -- `capability sensor.co2 from <export-id> as <id>;` -- and
its coordinator matches on the payload's `export_id` (a capability without `from` still matches
the old way). Nobody types it: generate writes the `from` from the import catalogue (which
carries each entry's `export_id` and the exporter's current device id), and when the exporting
house renamed the device it updates the importer's `device <id> from <house> <remote-id> with:`
line and reports it. The config topics themselves didn't change: only the other house's
coordinator reads them, so the identity travels in the message. An import of something the other
house doesn't export is an ERROR in generate's output ("<house> doesn't export these"). That holds
when an export stops too (PROJECT.md 22g, 2026-10-28): the exporter clears its retained cloud
config, and the importing coordinator -- which remembers which topic fed which capability, since
an empty message carries no `export_id` -- drops the catalogue entry and marks the capability
known-not-to-exist ("missing declared entities" turns on). A rename on the exporting side clears
the old topic too; the entry stays when the new topic already feeds the same capability.

### 6.13 Command routing for hassbridge entities (PROJECT.md item 11)

**hassbridge was one-way (state only) until 2026-09-09.** Every mechanism §6/§6.12 document —
reporting automations, discovery relay, `export`'s cloud cross-post — only ever moves a value from
the instance that owns an entity *outward*. Nothing let a command travel the other way, so a
bridged entity in a domain HA itself treats as controllable (`switch`, `vacuum`, ...) showed up
read-only on the consuming instance regardless. Forcing case: a Roomba vacuum bridged from a
secondary HA instance needs `vacuum.start`/`pause`/`return_to_base` to actually reach the physical
device, not just report its status.

**Design principle: a domain's command set is generator-owned, table-driven, opt-in-via-
declaration.** `homeassistant/hassbridge_commands.go` maps a capability's domain (the part of its
`<domain>.<label>` key before the dot) to the HA services it supports (`domainCommands`) and any
fixed discovery fields those need (`domainDiscoveryExtras`, e.g. vacuum's `supported_features` —
MQTT vacuum's own default omits `pause`/`locate` unless explicitly listed). A capability
automatically gets command generation the moment its domain has a table entry — no new Physical.def
syntax, the same principle already used to reject a `forced` override keyword for the
battery_level/battery_alert biconditional (item 14): declaring a capability in a commandable
domain already *is* the opt-in. `house_event_bus_coordinator` never learns a domain name or an HA
service string — `integration_hassbridge_generator.go` bakes each capability's resolved
`commands`/`discovery_extra` straight into `coordinator/homeassistant_bridge.yaml`, so the
coordinator only ever merges already-resolved data into a discovery config
(`discoveryhassbridge.go`'s `buildHassBridgeEntityDiscoveryBody`), keeping it domain-agnostic
everywhere else already applies.

**All of one entity's commands share a single MQTT topic**
(`homeassistant_instances/<instance>/bridge/<local_entity>/command`), discriminated purely by
matching the incoming payload against a literal string — confirmed against the live MQTT
switch/vacuum component source (not assumed from memory): both schemas take exactly one
`command_topic`, with each command recognised via its own configured `payload_*` field (`payload_
on`/`payload_off`, `payload_start`/`payload_pause`/...). `homeassistant/
remote_instance_entity_commands.go` generates one automation file per entity per command (matching
PROJECT.md item 11's own naming, `command_<fully_qualified_entity_name>_<command>.yaml`, alias
`command/<fully_qualified_entity_name_with_slashes>/<command>`), each triggering on that shared
topic with an exact-payload filter and calling one hardcoded service on the remote entity_id — no
Jinja dispatch, no argument parsing, because every command this table carries is a bare
service-call trigger.

**Same-broker case needs zero coordinator runtime involvement**, exactly like state reporting's own
shape (§6, "author the discovery config once, then get out of the way") and the `commandline`
daemon's (`command_topic` pointed at directly, coordinator never subscribes to it at runtime,
§8): once `buildHassBridgeEntityDiscoveryBody` has set `command_topic` and each `payload_*` field,
HA's own MQTT integration on the consuming instance publishes straight to that shared topic, and
the remote instance's own already-subscribed automation picks it up — the message never touches
the coordinator process at all.

**Not every domain's state is a bare value — vacuum needed a second table.** Confirmed against the
live MQTT vacuum component's own `_state_message_received`: its `state_topic` payload must already
be a JSON object carrying a `"state"` key, with no `value_template` hook to reshape a bare string
into that at the discovery-config level. `hassbridge_commands.go`'s `domainStatePayloadTemplate`
covers this — a domain with no entry keeps today's atomic (bare-string) reporting unchanged;
vacuum's entry wraps the resolved value as `{'state': ($)} | tojson` before
`entityReportingAutomationBody` publishes it. See PROJECT.md item 23 for the generalisation of this
one-off wrapper into a proper reversible topic/JSON rewriting mechanism, not yet designed.

**Phase 2, not built yet**: routing a command across the `export`/import cloud-broker link
(§6.12) — the mirror image of state's own direction (state flows owning-house → cloud →
consuming-house; a command must flow consuming-house → cloud → owning-house over the same link).
Reuses `Export`/`SelfImportFrom`, no new Physical.def flag: those already mean "this device's
traffic crosses the house boundary," and a command is just the reverse-direction half of that
meaning once its domain is commandable. Deliberately deferred — same-broker was the forcing case
(a same-house secondary HA instance), and nothing today needs a command to cross a house boundary
yet.

### 6.14 Stable coordinator↔HA entity identity: Physical.def `as <id>` (PROJECT.md item 7)

**The problem this closes**: a discovery-kind (`discovery` integration — Zigbee2MQTT/Z-Wave-JS/
EMS-ESP-style) capability's own HA `unique_id` was, until this landed, always computed fresh by
the coordinator as `relayedDiscoveryUniqueID(gatewayID, leaf)` —
`sanitizeTopicSegment(gatewayID) + "_" + leaf` (`discoverybridge.go`), a pure function of
Physical.def's own raw declaration. Renaming a device's DSL id, or swapping its underlying
hardware at the same conceptual position, changes `gatewayID` or `leaf` and therefore the
`unique_id` — HA then treats the result as a brand-new entity: the old registration orphans, the
new one registers fresh with no history, and the two can collide into a `_2` suffix if anything
else still references the old name. §6.11's own persistence-across-restarts fix solved the
*coordinator's* half of "don't lose state you shouldn't lose" — this closes the same problem for
the *HA registry's* own idea of identity.

**Mechanism**: a plain raw-leaf capability line in Physical.def (never an `is available`/
`derived` capability — those are local condition/template entities, never relayed, so they never
reach `relayedDiscoveryUniqueID` at all) can carry a trailing `as <id>;` —
`light.light_1 zwavejs2mqtt_0xd0204fb2_71-37-1-currentValue as sensors_X_...;`
(`TDiscoveryCapability.AssignedID`, `integration_discovery_parser.go`). Once present, it is
authoritative forever: the generator carries it through to `discovery.yaml`'s `entity_links:`
block as a new `stable_id:` field (`TDiscoveryEntityLink.StableID`), and the coordinator prefers
it over its own `gatewayID+leaf` computation in **both** places that computation happens —
`buildRelayedDiscoveryConfig` (the actual relay, drives both `unique_id` and the retained
discovery topic) and `expectedDiscoveryBridgeTopics` (the orphan-cleanup pass's own "what should
exist" calculation). Both needed the same preference, or cleanup would compute the *old*
gatewayID-based topic as "expected" and wrongly retire the real, stable-ID'd one.

**Three construction sites, not two.** `TDiscoveryEntityLink` is built independently in three
places, and all three must copy `AssignedID` into `StableID` or the id silently stops reaching
`discovery.yaml` for whatever path was missed:
`registerDiscoveryEntityLink`/`registerDiscoveryDerivedFromRawLeafEntityLink`
(`Conceptual_DiscoveryEntities.go`, the two "obvious" ones) and, less obviously,
`autoMaterializePhysicalCapability`'s own discovery-kind branch
(`Conceptual_LogicalEntities.go`) — the generic "any physical capability referenced via a
cross-device condition/value reference" path a Logical.def `... is switched by ...;`/
`dependency on ...;` one-liner resolves through, named nothing to suggest it touches discovery at
all. Found live by checking a bootstrapped capability's own `discovery.yaml` output and finding
`stable_id` simply absent, not by code review. **Before extending any struct built in more than
one place, `grep -rn "<TypeName>{" --include="*.go" .` for every construction site rather than
trusting the ones found by reading the obviously-named registration functions.**

**A switch takes its role from its use (2026-10-25).** A relay channel is declared as what the
device does, `capability switch.switch_2 ...`, whatever it switches. Positioned, its conceptual
entity decides the domain (`entity light.social:main from switch.switch_1;` is relayed as a
light). Unpositioned, the role comes from the use: in `light.core is switched by switch.switch_2
from <device>;` the hidden entity `autoMaterializePhysicalCapability` creates is a light already
(`coercedDomain`, roles light and fan), so it can be grouped with the light it powers; its name
and capability defaults are those of the role, and `switch_2` still shortens to `2`.

**Fresh ids, for a capability with no registry history to preserve**, are minted automatically as
part of ordinary generation (`autoAssignDiscoveryStableIDs`,
`physical_auto_assign_stable_ids.go`) — the one deliberate exception to this codebase's otherwise
universal "`.def` files are only ever read, never written" rule, and purely additive (it only ever
appends `as <id>` before an existing line's own trailing `;`). The id itself is the capability's
own resolved entity_id at the moment it's first positioned ("position at birth", frozen from then
on regardless of later repositioning), `_n`-disambiguated against any id already assigned anywhere
if it collides. All-or-nothing per run: if even one assignment's own capability line can't be
located in the real, on-disk Physical.def by reconstructing its exact domain/label/leaf text
(deliberately NOT by tracking a parsed line's line number — the one-liner-wrapping normalisation
pass, `joinWrappedStatementContinuations`, means a parsed line's position doesn't reliably map
back to the file's own real layout), nothing is written that run rather than a half-assigned file.

This auto-assignment call must never run during a read-only diagnostic pass (`-would_define`,
`-derived_migration_candidates`) — both share `parseAdministrationFromPaths` with the real
generator and are explicitly documented as never editing a file. Gated by a `writeBack bool`
parameter on that function, true only from `generateFromPaths`.

**One-time bootstrap** (`homeassistant -bootstrap_stable_ids <mapping.json> Definitions/Main.def`,
`bootstrap_stable_ids.go`) seeds `as <id>` for every *already-live* capability from its currently
real HA registry `unique_id` — never a freshly-minted value, preserving exactly what's already
live. `mapping.json` is a flat `{entity_id: unique_id}` dump of the live registry's own
mqtt-platform entities (no pre-filtering needed; only entries matching a live discovery capability
are ever consulted). A live capability with no corresponding registry entry is reported, not
guessed. Run once per house, before the first `as`-id-aware deploy; never again afterwards.

**Net effect on a rename, confirmed live**: HA updates the entity's own `entity_id` in place from
the new `default_entity_id` a fresh discovery payload carries (e.g.
`light.physical_sensors_X` → `light.physical_lights_X` after a device-id rename) while the
`unique_id` — and therefore the entity's actual identity/history — stays exactly as it was. A
device-id rename is now a cosmetic, zero-churn operation for any capability that's been
bootstrapped.

### 6.15 Device friendly names, implied node positioning, and `colocated` (PROJECT.md item 8)

**Friendly names derived from conceptual position, not the gateway.** A raw integration's own
native discovery payload (zwavejs2mqtt's Location+Name, Zigbee2MQTT's device nickname, ...)
describes where the *relay module* physically sits, not what it controls or where it conceptually
belongs — a Fibaro double-relay switching two different rooms' lights has no single "room" of its
own anyway. The fix, applied uniformly: an entity's own friendly name is always
`identity.Sphere + "/" + identity.Path` (`friendlyEntityName`, `device_friendly_name.go`); a
device's own friendly name is its *positioned* `node` entity's identity with the trailing `/node`
segment dropped (`resolveDeviceFriendlyName`) — e.g. a Z-Wave relay's own node positioned at
`infrastructural:garage/fuse_cabinet/zwave/042` gives the device itself that same path, correctly
describing the relay's real install location regardless of which room(s) it switches.

**Finding a device's own `node` entity needed one universal choke point, not per-kind lookups.**
Four different internal shapes can back a device's "node" capability (a raw discovery leaf; a
discovery-level `derived ... with condition ... is available;`; a Logical.def `node` override via
`dispatchLogicalCapability`; hosts/import/commandline's own `AttributeEntityIDs["node"]`) — a
per-kind lookup missed one shape, silently flagged dozens of genuinely-positioned devices as
unpositioned, then tried to register a second, colliding node for them. Fixed once:
`administration.DeviceNodeEntityID[deviceID] -> entityID`, populated inside
`registerDeviceCapabilityEntityLinkAllowingHidden` itself (the one function every kind's "node"
dispatch already funnels through) right after a successful registration where the resolved
capability name is `"node"`. `resolveDeviceFriendlyName` now has exactly two cases: a hosts-kind
device's own `HostIdentity` (its whole identity *is* its position, no separate node entity), and
this map for everything else.

**Implied node positioning**: a device used in *exactly one* conceptual space has only one
possible answer for "where is this device" — `applyImpliedNodePositioning`
(`device_friendly_name.go`, driven by `administration.DevicePositioningOccurrences`, one entry per
`device <id> with:` occurrence `registerDevicePositioning` ever saw, recorded
unconditionally regardless of that function's own register-once-merge-repeatedly identity guard)
auto-positions `node` there with no author action needed. A device positioned in 0 or 2+ spaces is
a genuine gap/ambiguity and gets a warning instead of a guess. Must run before
`validateDevicesHaveFriendlyNames`.

**`findSharedDevicePositions`** catches the one real hazard this creates: two *different* devices
weaving onto the same conceptual position (a real, legitimate pattern — e.g. two Zigbee devices
bundled in one enclosure), each independently lacking its own node, would both default to the same
bare `.../node` entity and collide. Reports only when 2+ members of a shared-position group
currently lack a positioned node (not merely when they share a position) — each then needs its own
explicit, distinguishing leaf (e.g. `entity binary_sensor.infrastructural:main_spots/node;` instead
of the bare form).

**`colocated <device-id> as <leaf>;`** (Logical.def, repeatable, on the *anchor* device's own
block): positions a device that has no conceptual location of its own but obviously lives wherever
another, already-positioned device lives — e.g. Vienna's `switch.living_room_tv` (a bare Z-Wave
relay switching the TV's own power) has no position of its own, but is `colocated
switch.living_room_tv as switch;` on `appliance.living_room_tv` (positioned `as tv`), resolving to
`tv/switch`. `applyColocatedPositioning` resolves the anchor's own single-space position (same
single-occurrence rule as implied node positioning — an anchor positioned 0 or 2+ times is
ambiguous) and calls `registerDevicePositioning` for the colocated device with that position plus
the declared leaf; the colocated device's own `node` is then auto-implied for free by the
mechanism above, since register-once also records its own occurrence.

**Commandline's own `dependency on <id>;`** is declared directly inside Physical.def's
`integration commandline with: device <id> <host> with: ...; end;` block, *not* via a separate
Logical.def entry like hassbridge/import — confirmed with the user 2026-10-07: commandline is a
physical-layer integration, so a dependency on another device's liveness belongs declared at the
physical layer directly. Resolved single-hop only, directly against `hostDevicesByID`
(`generateCommandlineIntegrationOutputs`) — no transitive chain walk, since commandline devices
aren't part of Logical.def's own dependency graph. ANDed (via the shared `nodeAvailabilityEntries`
helper, `house_event_bus_coordinator/discovery.go`) into every one of that device's own entities,
including its own liveness entity — matching hassbridge's identical precedent (a device's own node
must also reflect a dependency it declares, not just its siblings).

**`heatingCapableSpaces` (`administration.go`) is sphere-agnostic**, triggered purely by a switch
entity's own path ending in `heating` — *not* a specific sphere. Found live while renaming
Junglinster's heating-relay devices: this was hardcoded to `Sphere == "physical"` only, which
silently dropped the entire leakage-detection/mode-preset/`set_heating_to_*` subsystem the moment a
heating switch was repositioned to `social`. `generateSwitchControlledHeatingContent`'s own,
separate detection loop had the identical hardcoding, plus a second bug one level deeper: the
`switch.turn_on`/`switch.turn_off` automations it generates had the target entity_id *hardcoded* to
`switch.physical_<context>_heating` regardless of the switch's real sphere — fixed by resolving the
real entity_id from the same scan (`heatingSwitchEntityIDs`) instead of guessing it from a fixed
sphere prefix.

### 6.16 The five integration kinds, and the end state of item 19 (2026-10-27, Erik)

The main Home Assistant instance gets the states of almost every integration from the MQTT bus,
and sends commands back over it. Since item 19 it no longer takes those entities from MQTT
discovery but from the generator's own YAML `mqtt:` config: which entities exist, and what they
are called, follows the defs alone. What differs per integration is how much of the MQTT side is
ours. That gives five kinds, by name rather than number (§6.10's older "kind 1..5" numbering
predates this and is a different cut -- see the mapping at the end):

| Kind | Integrations | Entities known from | MQTT topics and retain | Coordinator's role |
|---|---|---|---|---|
| **controlled** | `hosts`, `commandline`; later `fhem2mqtt` | the defs alone | ours: we choose the topics and set retain | relays/aggregates; nothing to discover |
| **bridged** | `home_assistant ha2mqtt`, `home_assistant main` | the defs, checked against the hosting HA instance | ours: the generator's reporting automations publish them | checks that exported entities exist there, and suggests unused ones |
| **local** | `local` (media_player, camera, weather, ...: §6.7) | the defs, checked by positioning | none: no MQTT at all | none; names held by the universal ID and the registry repair |
| **raw** | future: devices with MQTT but no discovery (e.g. Shelly) | hand-written definitions in Physical.def | theirs: we don't control retain | cross-posts state retained (below) |
| **discovered** | `discovery`: Zigbee2MQTT, Z-Wave JS UI (zwave2mqtt), EMS-ESP | the gateway's own discovery, collected by the coordinator | theirs: we don't control retain | collects configs, checks assumed entities, writes `suggestions/discovery.txt`; cross-posts state retained |

**Not kinds of their own.** Templates the generator writes itself (derived capabilities, logical
devices, conceptual aggregates, nodes) are local Home Assistant template entities, built from
entities of the five kinds. `import` (another house's entities over the cloud broker, PROJECT.md
19h) is transport rather than a kind: the entity keeps the kind it has in its own house, and the
transport is the coordinators', so ours -- topics and retain included.

**`home_assistant main` is the odd one out**: the main instance speaks MQTT to itself (a reporting
automation publishes an entity's state, and the same instance reads it back as an MQTT entity).
Where an entity is only used by this instance, making it `local` instead removes that round trip
and the duplicate entity. What it loses is the entity's place on the bus: anything that reads the
bus (the other house via `export`, other consumers, §7) then no longer sees it. So the choice is
per entity: on the bus because something else needs it, otherwise local.

Checked and done for both houses 2026-10-28 (PROJECT.md 21): their `integration home_assistant
main` held only the instance's own host (`node.vienna`/`node.junglinster`: CPU load and
temperature from System Monitor, a node) and the iPhone (battery from the Mobile App, a node, a
battery alert). Nothing else on the bus needed them -- not exported, not imported, no "dependency
on" -- and a bus copy of the main instance's own host is useless anyway: when it is down, nothing
works (Erik). Both moved to `integration local`; the block is gone. What made that possible:

- A local capability may name its native entity: `capability sensor.cpu/load
  sensor.processor_use;` -- a source of the capability's own domain (labels may hold `/`). The
  generator records it in the registry manifest's native entry as `seed`; the repair renames that
  entity to the positioned id the first time no previous deploy knows the source, and afterwards
  as any native entry. A derived node names its sibling (`with condition sensor.cpu/load is
  available`), not the native entity, whose name changes.
- The switch takes two deploys, because the repair runs before HA's reload: in the first the
  bridge entities are still live, so they stay (as orphans after the reload) and the native
  renames wait; the node templates come up as `..._2`. The second removes the orphans and does the
  renames. A stale mqtt entry still live at the sync is kept as `pending_removal` in the saved
  state -- before, it was forgotten and stayed an orphan for good.
- The positioned ids kept their history (HA's recorder doesn't merge into an id that has history
  already); the native entities' older history stays under their former names. The device with
  captured metadata (e.g. "Home Assistant Green", its OS version) is gone: the entities belong to
  their native integrations' devices.

**Retain, for raw and discovered.** After a Home Assistant restart, a reload, or the switch of an
entity to YAML, Home Assistant only has a value for a state topic the broker retained. Controlled
and bridged integrations retain because we tell them to; Zigbee2MQTT, Z-Wave JS UI and EMS-ESP each
have their own option for it, and we don't want integration-specific code (or config) to set it.
Mosquitto has no "force retain" (a broker with a rule engine, e.g. EMQX, could republish with
retain, but that would tie us to that broker). So the coordinator steps in: for every state topic of
a raw or discovered entity that isn't declared retained (below), it cross-posts the latest message,
unchanged, retained, onto a topic of its own -- a shadow topic, "<original topic>/retain" (Erik,
2026-10-27) -- and the entity's YAML `state_topic` points there. The generator decides which topics
get one and gives the coordinator the list; the coordinator keeps a manifest of its shadows and
clears the retained message of every one no longer listed (at deploy and at every start), so
switching a device either way only changes its entities' `state_topic` (unique_id and entity_id
stay). A device's retain status can't be observed (a retained message at subscribe proves the
publisher retains, its absence proves nothing) and a discovery config doesn't say it, so the worst
is assumed unless the defs say otherwise -- no gateway-specific code (Erik, 2026-10-27/28):

- **Default: not retained.** A device or topic no line mentions gets a shadow -- for every topic its
  entities read state from (`state_topic` and the per-feature ones, e.g. a climate device's
  `mode_state_topic`, `current_temperature_topic`), never for command or availability topics. A
  wrong "not retained" only costs a redundant shadow; a wrong "retained" loses values on every
  reload.
- **Wildcard, per installation**: `retained topics "<filter>", ...;` in the `integration discovery
  with:` block, in MQTT's own filter syntax (`+`, `#`), e.g. Junglinster's
  `retained topics "zwave/#", "ems-esp/#";`. Topics matching it get no shadow. The prefixes are the
  installation's own gateway settings (Z-Wave JS UI's and EMS-ESP's base topics and their own
  retain settings), so they live in its defs, never in the code; the claim must hold for every
  state topic under the filter.
- **Per device**, in its block (like `ignore other capabilities;`), always winning over the
  wildcard: `is retained;` -- no shadow, the entities read the device's own topics (e.g. a
  Zigbee2MQTT device with Zigbee2MQTT's own retain option on; for raw devices, item 39, the normal
  way to say it); `is not retained;` -- a shadow after all: the way to back out of a wildcard that
  claims more than is true (e.g. a device whose own retain setting differs from its gateway's). It
  never stops a device from retaining; a redundant shadow is harmless.
- So each device line only does something against the default around it: `is retained;` where no
  wildcard covers the device, `is not retained;` where one does. Generate reports a line without
  effect, and a filter that matches none of the installation's state topics (a typo).
- Zigbee2MQTT gets no wildcard by default: its state message carries `action`, so a retained
  button message could fire again after each reload, and its retain setting and the defs would
  have to be kept in sync by hand.
- Commands keep going straight to the integration's own command topic.
- Only *state* is cross-posted. Events are never retained: an `event` entity, a button or a
  remote's `action` would fire again on every reload. Where one topic carries both (Zigbee2MQTT
  puts `action` into the device's state JSON), the event entity keeps reading the original topic,
  and only the state entities read the retained copy -- a choice per entity, which the generator
  can make since it writes each entity's YAML.
- The coordinator then sits on the data path for these states. If it stops, the copies stop
  changing while the devices go on: values freeze instead of going unavailable. So every
  cross-posted entity's availability also includes the coordinator's own (its Last Will).
- A retained value of a device that is really gone stays visible only as long as availability
  doesn't say otherwise; for discovered gateways their availability topics cover this, for raw
  devices each definition needs its own availability (Erik: "we also need to detect if they are
  really down"; PROJECT.md 39).
- Built 2026-10-28 (PROJECT.md 20): generator `homeassistant/retain_shadows.go` (the decision,
  the grammar's filters; `applyRetainShadows` in the discovery YAML writer, which also writes
  `coordinator/retain_shadows.yaml`), coordinator `house_event_bus_coordinator/retain_shadows.go`
  (the copies, the retained manifest `coordinator/retain_shadows/state`, the Last Will
  `coordinator/availability`). A `#` inside a quoted string is no comment (`stripLineComment`),
  for the filters. Why it matters: HA's MQTT reload re-creates every YAML mqtt entity, and before
  the shadows a deploy left Vienna with 99 of 162 Zigbee entities "unknown" (2026-10-27). Tested
  live 2026-10-28 with a full reload and no other refill: none of Vienna's 129 or Junglinster's
  289 shadowed entities lost its value. Zigbee2MQTT ignores the `/retain` topics inside its own
  namespace (tested). Still expected after a reload: a template with `delay_on`/`delay_off` shows
  "unknown" until its delay has passed, an event entity until its next event.

**Home Assistant's MQTT schema is a code dependency, isolated in one place.** Item 19 depends on
how Home Assistant structures MQTT entities in two ways: reading the discovery a gateway publishes
(abbreviations, `~`, device/origin blocks), and writing YAML entities (which fields each domain and
light schema accepts). A real change of either needs a code change, so this knowledge belongs in
one module, shared by generator and coordinator, not spread over both: the package `hamqtt`
(`homeassistant/hamqtt`, its own Go package in the same module). `fields.go` holds the field table
(`ha_mqtt_fields.json`, embedded, with the Home Assistant version it was checked against), the
abbreviations, `DiscoveredConfig` (what Home Assistant's discovery keeps of a payload: abbreviations
expanded, `~` substituted, unknown fields dropped) and `Accepts`; `relay.go`, `bridge.go` and
`imported.go` the builders of each entity kind's config. The coordinator's device-block keys come
from the same table. `tools/ha_mqtt_fields.py` (reading the table out of Home Assistant's own code)
is a migration aid: empirical data next to the documentation and the sources, not a step any build
or deploy depends on. Structurally, what keeps the table honest is the data itself: generate
reports, per device, every field its YAML leaves out because the domain doesn't know it ("dropped,
unknown to Home Assistant: light value_template (2)") -- `origin`, `object_id` and `~` excepted, which go by design. Our own builders
only write fields the domain accepts, so what shows up there comes from a source: a new field from a
gateway, or a schema change, and then the table and the code change together.

**How the YAML entities are built (item 19, done 2026-10-27).** One file per entity,
`entities/mqtt/<domain>/<entity_id>.yaml`, included with `!include_dir_merge_list`. Every entity
keeps the unique_id (its frozen universal ID, §6.7) and entity_id it had under discovery, so
registry entries, devices and history stay; its `name` is its full conceptual name ("sphere/path",
the UI shows the registry name, not the friendly_name).
- *Discovered*: the coordinator collects every discovery config of every declared gateway, used or
  not, and publishes them retained per gateway (`discovery_raw_configs/<gateway>/state`, also
  under `<installation>/` on the cloud broker). Generate fetches them and caches them in
  `Definitions/.cache/discovery_raw_configs.json`; a gateway the broker doesn't deliver (offline,
  or a slow link cutting the fetch short) keeps its cached configs. The relay's transformation
  (`hamqtt.RelayedConfig`) then `DiscoveredConfig` make the entity. A capability declared for the
  first time gets its YAML on that same generate; one without a raw config is an ERROR in
  generate's output and stays out -- never a fallback to discovery.
- *Controlled* (hosts, commandline) and the coordinator's own entities (meta controls, missing
  declared entities): from the defs alone.
- *Bridged*: from the defs, plus the source entities' typing as the coordinator learns it
  (`hassbridge_typing/state`, cached); a value nothing reports stays out.
- *Imports*: what the exporting house says about an entity (device metadata, via_device, typing,
  payloads) belongs to that house: it comes from the coordinator's `import_catalogue/state`
  (cached), never from the importer's defs. The coordinator relays the state onto
  `imports/<entity>/state`.
- *Device metadata* (manufacturer, model, versions, serial, via_device) changes rarely, so it is
  captured into Physical.def at generate time, from what discovery or the device itself reports
  (`discovery_device_info/state`, cached). A value marked `forced` keeps its listed value; where
  the report differs, it appears next to it as a comment (`model: "Roomba" forced;   # discovery:
  ...`). A value nothing reports any more stays. `via_device: utility.envoy;` is a device
  reference, unquoted; the YAML gets the referenced device's Home Assistant identifier (a
  discovery device's first identifier, every other device's declared id).
- *Discovery to Home Assistant ends*: no coordinator publishes discovery to its main instance. Two
  exceptions remain: the cloud catalogue between houses (another house's import reads it), and
  the passthrough of an undeclared discovery device, for migrating an installation device by
  device. That is also why the discovery prefix stays `homeassistant.physical` (next to an existing
  installation's `homeassistant/`), and why `${mqtt_discovery_conceptual}` stays in the code
  (commented out in both houses' Settings.def; its default is the same).
- *Suggestions* (`suggestions/discovery.txt`) are copy-paste-ready capability lines
  (`capability <domain>.<label> <leaf>;`).

**A device's own availability versus its gateway's (PROJECT.md 22b, 2026-10-28).** A discovered
device's entities take their availability from its gateway's discovery configs, which mix two
kinds of entries: the gateway's own, shared by all its devices (Zigbee2MQTT's
`zigbee2mqtt/bridge/state`; Z-Wave JS UI's `zwave/driver/status` and
`zwave/_CLIENTS/<client>/status`; EMS-ESP's `ems-esp/status`), and the device's own (Zigbee2MQTT's
`zigbee2mqtt/<device>/availability`, a Z-Wave node's status). Which is which is read from the
data, never from gateway names: a topic the configs of more than one device carry is shared, one
only this device carries is its own (`homeassistant/shared_availability.go`). There is no single
"bridge/state" per gateway and no reliable way to name a device's gateway (only Zigbee2MQTT
reports via_device), and neither is needed.

A device that only sends when used -- Junglinster's 19 battery-less Hue Taps -- gets marked
offline by its gateway after a while (Zigbee2MQTT: 25 h without a message), though it works when
pressed. Its declaration says `ignore own availability;` (in its device block, like `ignore other
capabilities;`): its entities keep only the shared entries, so they are available exactly as long
as the gateway is. Generate reports what it drops, and warns -- leaving the availability as it was
-- when nothing shared would remain. The Taps' nodes were already that: Logical.def derived nodes with
"condition node from node.zigbee2mqtt_bridge", positioned implicitly. A device flag may sit
among its metadata lines, with a trailing comment; generate's metadata writer keeps it (it once
left a second metadata block behind when a flag came before the identifiers). This is a first step towards integration devices (PROJECT.md
26): integration.zigbee2mqtt and the like, with a node from that shared topic and a dependency on
their compute host, which the devices' availability will then follow; the same topic sharing
suggests (as a comment, never a value) which devices belong to which integration, where the gateway
reports no via_device -- a "via_device: ... forced;" line overrides.

**Integration devices, and `integration mqtt` (PROJECT.md 26/39, 2026-10-28).** Each gateway and
bridged instance is a device of its own, positioned next to the compute host it runs on, with a
node that says whether it is up (Erik): `integration.zigbee2mqtt` (renamed from
`node.zigbee2mqtt_bridge`; its node is Zigbee2MQTT's own discovered connection state),
`integration.zwave2mqtt`, `integration.ems2mqtt` (realised by the EMS-ESP board, hosted by
`node.ems_esp`) and `integration.ha2mqtt`. A Logical.def "dependency on" its host is ANDed into the
node's availability -- the discovery YAML writer now applies dependencies like every other kind's.
Where the node's topic comes from depends on who controls it:
- *Topics we don't control, without discovery* -- Z-Wave JS UI's `zwave/driver/status`, EMS-ESP's
  `ems-esp/status` -- are declared in `integration mqtt`, the start of the raw MQTT integration:
  `capability binary_sensor.node topic "zwave/driver/status" on "true" [as <id>];` ("on" for that
  value, "off" for any other). Such a device rides on the discovery machinery: the generator writes
  its "raw config" itself (`homeassistant/integration_mqtt.go`), so positioning, frozen IDs, YAML
  entity and retain decision are a discovered device's; the coordinator gets nothing. Retain: the
  same default as for discovery (not retained, so a shadow), the installation's `retained topics`
  filters (written in either block) and `is retained;`/`is not retained;` apply -- Junglinster's
  `zwave/#`/`ems-esp/#` cover these two.
- *Topics we control* -- a bridged Home Assistant instance, later fhem2mqtt -- declare the node in
  their own integration: `integration home_assistant ha2mqtt with: device integration.ha2mqtt
  with: capability binary_sensor.node; end; ...`. Its topic is `homeassistant_instances/<instance>/
  status`, kept by the coordinator's ping-pong (`house_event_bus_coordinator/
  instance_liveness.go`, Erik's design): every message from the instance is a sign of life; only
  after 5 minutes of quiet does the coordinator send `ping` on `homeassistant_instances/<instance>/
  ping`, which a generated automation on the instance answers with `pong` on the same topic; no
  pong within 30 s is `offline`. The status is retained and the node's availability includes the
  coordinator's own (without it, nobody keeps the status).

Device metadata from the integrations (PROJECT.md 22d, 2026-10-28). The coordinator knows no
integration's peculiarities; the generator tells it, per device, where that device's metadata is.
For topics we control, by convention: a hosts device's `device_info_topic` (devices.yaml) -- the
coordinator now subscribes to exactly those, no longer to a hardcoded `hosts/+/device/state` -- and
a bridged device's reporting automation. For topics we don't control, the device says it in
`integration mqtt`: `metadata sw_version from topic "ems-esp/info" value "version";` (the JSON
field; without `value`, the whole message). The generator lists them in
`coordinator/metadata_topics.yaml`; the coordinator subscribes, takes the field and records it like
discovered metadata (`discovery_device_info/state`), which generate captures into Physical.def --
`forced` and the `# discovery: ...` comment work as for discovered devices.

Reload guards (PROJECT.md 23, 2026-10-28). An integration on a bridged instance that loses its
connection now and then (Junglinster's Netatmo on ha2mqtt) comes back with a reload. The bridged
device declares it in its own integration section (PROJECT.md 29 (3)): `reload integration when
sensor.co2 is unavailable for 00:05:00;` -- a capability of the device, whose source entity on the
instance the generator knows. One generated automation per instance
(`homeassistant/remote_instance_reload_guard.go`) triggers on any indicator staying unavailable that
long, notes it in the logbook, reloads that entity's integration (`homeassistant.reload_config_entry`)
and then waits 15 minutes (`mode: single`) -- at most 4 reloads an hour, never a reload storm when
the integration is really down. A local device's guard (`integration local`) runs on main, the
same way. `while <device> is up` makes a guard reload only while that device's node is on -- a
portable speaker switched off must not reload the whole Sonos integration every 15 minutes. After a
start or an automation reload (a deploy), when an entity may already be unavailable without a
change to trigger on, the guard waits the longest guard time once and reloads for every guarded
entity still unavailable (with its while-condition). In Junglinster: the three Netatmo Smart Indoor
Air Quality Monitors' CO2 (on ha2mqtt), the two Ring cameras and the Sonos headphones (on main,
each while its ping node is up).

Node positioning, a fix (2026-10-28): a derived node whose condition refers to another device's
node ("condition node from integration.zigbee2mqtt") materializes that node itself when it isn't
positioned -- but now only when nothing positions it explicitly further down Conceptual.def
(`ExplicitNodeDevices`, from a scan of the device blocks before resolving); otherwise it waits.
Before, a Hue Tap's node positioned explicitly before the bridge made the bridge's node the Tap's
"controller" node, and every other Tap's node pointed at it.

**Deploying YAML entities: what the live rollout taught.**
- One field Home Assistant doesn't know makes the WHOLE MQTT reload fail, while the deploy still
  reports success (found live: a relay `state_topic` on a bridge button; 64 entities missing for
  ~5 minutes). So generate validates every YAML file against the field table (an unknown field
  stops it), and deploy step 06 reports Home Assistant's reload errors
  (`tools/ha_reload_errors.py`). `ha core check` does not validate MQTT YAML entities; only a
  reload does.
- The MQTT reload takes ~45 s and recreates every YAML entity: check only after it. Values of
  topics that aren't retained would be lost (Zigbee2MQTT's state topics); the retained shadows
  above keep them. Until 2026-10-28 deploy step 06 restarted Zigbee2MQTT instead, whose
  `cache_state_send_on_startup` republishes every state -- with side effects (the bridge offline
  ~10-30 s, plugs and battery devices "offline" in its availability for minutes, until their next
  check-in). The first deploy that introduces shadows still needs one such refill: the shadows
  start empty.
- The coordinator runs as systemd `Type=notify`, sending `READY=1` once every subscription is in
  place, so a reload request the deploy sends right after a restart isn't lost.
- A retained cloud message is no sign of life: a cloud host's liveness only counts live messages
  (switched-off Macs showed "on" for ~1.5 minutes after every coordinator restart).

**Mapping from §6.10's older numbering**: its kind 1 (hosts) is *controlled*; kind 2 (discovery)
is *discovered*; kind 3 (`home_assistant` bridge) is *bridged*; kind 4 (imports) is the transport
above; kind 5 (bare "main instance" entities, used directly, without MQTT) is *local* in all but
its grammar -- a candidate to fold into `local`.

---

### 6.17 One layout for the .def files (PROJECT.md 22c, 2026-10-28)

`homeassistant -format <path/to/any.def>` formats every `.def` file in that folder
(`homeassistant/def_format.go`); generate formats the files it writes into (Physical.def,
Logical.def) as it writes them. The layout, uniform for all files:
- indentation by nesting, 2 spaces a level -- a statement ending in `:` opens a level, `end;`
  closes it;
- single spaces between a statement's words (outside quoted strings) -- except a metadata line
  (`model:        "Bridge";`), whose colons the generator aligns within its block;
- a finished statement (`...;`) longer than 80 characters is broken before its clause keywords
  (`as`, `from`, `via`, `topic`, `with` -- never `with:`), each continuation 4 deeper, packed
  greedily; never after a `:` or `;` (that line would read as a statement of its own);
- comment lines, blank lines and trailing comments stay as written; a `when`/`if` rule clause, a
  `${...}` setting and a statement with a comment inside are never broken.

The DSL already reads a broken statement as one (`joinWrappedStatementContinuations`), so
formatting changes no meaning: both houses generate byte-identical output before and after, and a
second pass changes nothing. The generator's own writers into the defs (frozen IDs, stable IDs,
metadata, import identities) read a file with each statement joined onto one line
(`readDefStatementLines`) and write it back through the formatter (`writeDefFormatted`), so they
work whatever the layout. What stays long is what can't be broken: comments (by choice), and an
`as <id>` or `entity ...` line whose ID alone is long. Per-integration layouts (the old item 46's
idea) only if one turns out to be wanted.

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

## 8. System components

- **coordinator** — the runtime component described in §6: connects to the local MQTT broker and any bridged external brokers, performs refinement/aggregation/federation continuously.
- **generator** — compiles the high-level DSL specification into:
  - YAML for the main Home Assistant instance(s)
  - definitions (YAML) for the coordinator
  - definitions/specifications per integration (e.g. FHEM's MQTT connection config, a host-ping table, a second HA instance used purely to bridge a cloud service)
- **mqtt_commandline** — a third long-running Go daemon (`mqtt_commandline/`, PROJECT.md item 1), deployed onto any host that needs local script-backed entities with no MQTT integration of their own (e.g. a picture frame's slideshow control). Reads a generator-authored per-host YAML config (script paths only), connects with its own MQTT Last Will on `commandline/<host>/node/state` (detects the *service* dying, not just the host — the coordinator's own class of liveness mechanism, not `hosts`' ping-based one, since this daemon holds its own persistent broker connection the way `hosts`' fire-and-exit `cpu/report` never does), and relays raw state/command topics for switch/sensor/button capabilities. Never publishes HA MQTT discovery itself — the coordinator stays the sole discovery author, same as every other integration kind; a switch/button's `command_topic` points directly at this daemon's own topic, HA publishes there straight from the UI, no coordinator-side relay needed. See README.md's "Devices (the `commandline` integration)" for the DSL grammar and wire shape.

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

### 9.3 Rule grammar: a CDL1-style three-layer parser (PROJECT.md item 13)

**The problem this closes**: both houses carried a batch of hand-authored HA automations (31
Vienna, 54 Junglinster) living entirely in HA's own UI-managed `automations.yaml` — symlinked into
the generator's own `automation/` output directory so they still ran, but never generated,
verified, or even visible to this project. A much smaller existing construct
(`TWhenRule`, "`rule ... when ... then ...;`") already let Conceptual.def declare a *single*
trigger/action rule, built with the same ad hoc line/`strings.Fields` matching GO_CONVENTIONS.md
§7/§8 explicitly discourages for anything beyond the simplest one-liner. Extending that
construct's own matching with more special cases for every new trigger/condition/action shape the
85 real automations needed would have piled more of exactly that debt on top of itself.

**Decision**: build a real three-layer CDL1-style parser from scratch for the rule grammar —
`rule_character_stream.go` (`TDefCharacterStream`, byte-level, line/column tracked),
`rule_tokeniser.go` (`TDefTokeniser`, keyword/identifier/number/string/punctuation
classification), `rule_parser.go` (`TRuleParser`, CDL1 bool-returning recursive-descent grammar
functions) — the same architecture pattern `../bibtex/bibtex_check` already exemplifies, adapted
to this codebase's own warnings-slice error convention rather than importing a second reporting
mechanism. `TWhenRule` was replaced outright, not kept alongside the new construct; Junglinster's
one real pre-existing usage (`done_removing_smells`) migrated to the new block syntax in the same
pass, with both houses regenerating byte-identical.

**Grammar shape** (full syntax reference: README.md's "DSL syntax" → "Rules"). A rule requires
exactly one `when` clause (trigger, inline-`or`-chainable), allows at most one `if` clause
(gating condition — a single recursive and/or/not/parenthesised boolean expression, mirroring
HA's own nested `condition: and/or/not` + `conditions: [...]` YAML shape directly, rather than a
separate block-composition mechanism), and requires exactly one `then` clause (one action, or a
`then: ...; end;` block for several). This exactly-one/at-most-one/exactly-one structure is
enforced at parse time by a per-rule `ruleClauseTracker` (`parser.go`) that warns and discards any
extra clause rather than silently merging or hard-erroring — a deliberate, user-requested
constraint (not implied by HA's own automation schema, which tolerates an arbitrary trigger/
condition/action list) chosen because a rule migrated from a real automation should read as one
coherent if-this-then-that statement, not an accumulation of independently-added clauses.

**Positioning convention**: a rule is written inside whichever space is the lowest common ancestor
of all the entities its own trigger/condition/action clauses touch — not always the action's own
leaf space. A toggle whose two triggers live in sibling subspaces and whose action lives in a
third sibling is positioned at their shared parent, using *relative* paths that simply descend
into each subspace from there (relative resolution is plain path concatenation against the
current space context, so it works for a child/descendant entity exactly as well as a same-space
one — not restricted to "the entity must be in this exact space"). Only entities genuinely outside
that subtree — true siblings of the rule's own position, or reached only through a virtual
space's `member` reference (which is a logical grouping for aggregation, never a parse-time
nesting relationship) — fall back to the `/`-prefixed absolute form
(`normalizeEntityFullName`'s existing escape hatch, reused verbatim, not redesigned for this).
Positioning a rule near the entities it actually touches, rather than always at its action's own
home, is also what makes the relative-path form usable at all for the common "triggers and action
scattered across sibling spaces" shape — the reason this convention was adopted over simply
always positioning at the action's own leaf space.

**Four real bugs found live, not anticipated up front** — each fixed with a regression test:

- `EntityRef` never consumed the leading `/` of the absolute-path escape hatch, so every absolute
  entity reference failed immediately after the colon (found migrating Vienna's first
  cross-space rule).
- `EntityRef` couldn't tell `<sphere>:<path>` (path continues right after the colon) apart from
  `<sphere>:` (empty path, e.g. `vacuum.social:` immediately before a predicate keyword) followed
  by an ordinary word — it silently swallowed the next keyword as if it were the path. Fixed via a
  new `TRuleToken.PrecededByWhitespace` field (`rule_tokeniser.go`): a path only continues when the
  next token touches the colon with no whitespace in between.
- `genericBodyElement` (`defined.go`, the generic top-level block-boundary scanner
  `scanGroupClauseBlocks`/`layers.go` use to find each `"<layer> layer with: ... end;"` block's own
  extent) treated *any* `":"`-terminated line as an independent nested block needing its own
  matching `"end;"`, with no knowledge that a rule body's `"then if <entity-ref> is <state>: ...;
  else: ...; end;"` branching action is a single flat construct sharing **one** closing `"end;"`
  between its `"then if ...:"` header and `"else:"` marker. The first real if/else rule ever
  written into a live Conceptual.def (Junglinster's sarah_bedroom cover rules — Vienna's own 31
  never needed one) hit this: `"else:"` was mistaken for its own opener demanding a third `"end;"`
  that doesn't exist, starving the enclosing `"rule ...:"`/`"conceptual layer with:"` blocks of
  their own close and failing the *whole file* with a misleading "missing its closing end;" error
  anchored at the outermost block — while the real per-line rule-body parser
  (`ParseEntitiesAndFillAdministration`), and every existing unit test, parsed the exact same rule
  text perfectly fine, since `parseIfActionBlock` already understands `"else:"` correctly. The bug
  was purely in the separate, generic, rule-grammar-agnostic layer-boundary scanner. Fixed by
  special-casing the literal line `"else:"` to never open its own group.
- `renderRuleValue`'s jinja branch (`generator.go`) wrapped raw jinja pass-through text verbatim in
  a single-quoted YAML scalar without escaping any embedded `'` — a real automation's own
  `jinja "100 - (states('number...') | int)"` is exactly this shape. Deployed to Junglinster, this
  broke the generated automation's own YAML block mapping and put the live house's HA core into
  recovery mode on restart. The test covering this exact rule (`TestParseRuleIfThenElseAction`)
  had passed throughout, because it only string-matched the generated text rather than confirming
  it was valid YAML at all — fixed by doubling embedded `'` (the same rule `jinjaQuotedLiteral`
  already applied for a different call site) and by adding a real `yaml.Unmarshal` assertion to
  that test, since string-matching alone cannot catch a YAML-structure break.

**Net effect, both houses**: Vienna's 31 and Junglinster's 53 (of 54 — one confirmed-dead
duplicate excluded, not migrated) real automations are now DSL `rule` blocks in Conceptual.def,
regenerating through the normal pipeline, with each house's own `automations.yaml` backed up and
emptied. Three more dead-target automations (referencing entities that no longer exist) were found
and fixed during migration verification against each house's live entity registry, the same
"confirm with the user, then redirect to the real entity" resolution as an already-known Vienna
case. Both houses' live entity registries were confirmed to have zero remaining `"_2"` collisions
from this migration after registry surgery (the same stop-core/edit-registry/restart-core pattern
§6.14 already established) resolved the ones found.

---

## 10. Platform-specific integration notes

No single integration mechanism suits everything — richness of the native API is weighed against MQTT's decoupling benefits per platform:

| Platform | Mechanism | Why |
|---|---|---|
| **Z-Wave** | Z-Wave JS UI, MQTT discovery (zwave2mqtt; *discovered*, §6.16) | What runs in Junglinster. The original plan was the native WebSocket to Z-Wave JS UI, for network management, inclusion/exclusion, interviews, firmware updates, configuration parameters and associations; those stay in Z-Wave JS UI's own UI. |
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

As of 2026-10-23, the same shape in both houses:

```
Home Assistant Green ("main", per house)
    +-- Home Assistant -- Consumer role only: automation, dashboards, UI.
        No MQTT broker and no Zigbee2MQTT add-on any more; its MQTT integration points at
        protocols-server-1.

protocols-server-1 (Ubuntu 26.04, Podman containers + native services) -- the house's hub
    +-- mosquitto       the house's MQTT broker (every client: HA main, coordinator, Z2M,
                        Z-Wave JS UI, ha2mqtt, EMS-ESP, cpu/ping reporters, mqtt_commandline)
    +-- zigbee2mqtt     Zigbee radio (JLI: Nabu Casa ZBT-2; Vienna: Sonoff dongle)
    +-- zwave           Z-Wave JS UI (JLI only: Nabu Casa ZWA-2)
    +-- homeassistant   "ha2mqtt" -- Home Assistant in its Integration/adapter role only
                        (Netatmo, Envoy, Overkiz/Somfy, ...), bridged by the coordinator
    +-- caddy           JLI only: public HTTPS endpoint for Netatmo's webhook
    +-- coordinator     native systemd service (house_event_bus_coordinator)
    +-- ping/report     cron: liveness of the house's hosts (hosts/<host>/node/state)
    +-- Samba (+ Plex)  the music collection for the Sonos (JLI: also Plex); Vienna: smtpproxy

Other hosts
    +-- frame           picture frame (mqtt_commandline); JLI also runs the BT proxy there
    +-- xanadu (JLI)    Plex (video) + the RAID backup pool; backups = backup of last resort
    +-- protocols-server-2 (JLI)  retired, kept for FS20 until FHEM moves
    +-- protocols-server-3 (JLI)  Raspberry Pi, server-only; planned FHEM/FS20 (FHEM2MQTT)
    +-- every host      cpu/report (load, temperature, device info) to the house's broker

Cloud
    +-- mqtt.erikproper.eu  shared bridging broker: roaming laptops' cpu reports and the houses'
                            export/import of devices (§6.12, §12) -- never something a house
                            depends on to function
```

`protocols-server-*` naming reflects architectural *role*, not hardware -- the next protocol to
migrate (or the next cloud service to isolate) has an obvious destination without renaming
anything.

**How it got here.** Junglinster moved its broker from the Green to protocols-server-1 on
2026-10-21/22 (clients moved one by one, a temporary two-way bridge carried retained state, then
the bridge and the Green's Mosquitto add-on were removed), and its Zigbee network on 2026-10-23
(from the Green's Z2M add-on with a Sonoff dongle to Z2M on protocols-server-1 with the ZBT-2 --
README.md's Operational notes has the adapter-change procedure). Vienna had made the same moves
earlier (Zigbee 2026-09-08).

### 11.1 Why compute nodes are split, not consolidated

The topology above is deliberately not "one powerful box running everything": `protocols-server-1`
carries the Zigbee/Z-Wave radios, `protocols-server-2` hosts the Netatmo-adapter HA instance, Home
Assistant Green runs the main automation/dashboard instance, and so on — each its own machine, not
a role inside a shared process. Four independent reasons converge on that answer, not just one:

1. **Hardware constraints are real, not a design choice.** A USB Zigbee coordinator, a Z-Wave
   controller, and a picture frame's own HDMI output are each physically attached to one specific
   machine; they cannot float to wherever would be architecturally tidiest. Some placement
   decisions are made before any preference gets a say.
2. **Blast radius.** Zigbee2MQTT crashing, needing a firmware update, or being restarted for a
   config change should never take down the main HA instance's own automations, and vice versa.
   Each compute node's own failure mode stays contained to whatever it alone is responsible for.
3. **Independent maintenance and upgrade cadence.** `protocols-server-2`'s own HA instance (whose
   only job is bridging Netatmo) can be rebooted, backed up, or reinstalled entirely on its own
   schedule, with zero coordination needed against the main instance's own release cycle.
4. **Incremental migration.** §6.4's legacy-to-conceptual passthrough — and PROJECT.md's own Phase
   3 roadmap, one integration migrated onto the new architecture at a time (cpu/ping, then Netatmo,
   then Zigbee2MQTT, then Z-Wave) — is only possible because each integration already lives on its
   own compute node. Moving one integration onto the new architecture never risks the others, since
   they were never sharing a process or a failure domain to begin with.

None of these four reasons is really about home automation specifically — they're the ordinary
reasons any distributed system decomposes into independently-deployable units instead of one
monolith. The event bus (§5) is what lets that decomposition stay invisible to the conceptual
layer: a `social/apartment/living_room/temperature` entity looks identical to Home Assistant
regardless of whether it's reported by a Zigbee radio on `protocols-server-1`, a Netatmo cloud
bridge on `protocols-server-2`, or a picture frame's own `mqtt_commandline` daemon (PROJECT.md item
1) — the compute-node boundary is a physical-layer/deployment concern, never a conceptual-layer one.

### 11.2 Container strategy: appliance vs workbench

Not everything needs Docker/Podman — the decision is whether a service is an *appliance* (stable, should be trivially replaceable) or a *workbench* (actively extended/experimented with):

- **Appliance** (→ containerize, e.g. Podman/Quadlets or Docker): Z-Wave JS UI, Zigbee2MQTT, MQTT bridge services, other radio services. Reproducible deployment, easy migration between hosts (copy the compose/quadlet directory + persistent storage, plug in the hardware, start), isolated dependencies, simple upgrades.
- **Workbench** (→ native install): FHEM. Easier module installation, easier debugging, natural integration with host Linux tooling — containerizing something under active, exploratory development mostly gets in the way.

### 11.3 Backup strategy

Every containerized appliance (§11.2) keeps its persistent state under one uniform path,
`/var/lib/home-automation/<container-name>/` (mosquitto, zigbee, zwave, bt-proxy, homeassistant,
caddy, ...) — a convention that predates this backup mechanism but is exactly what makes it work
with zero per-container or per-host configuration: `backup-home-automation`
(`/usr/local/bin/backup-home-automation`, one daily `systemd` timer per host) simply iterates
whatever subdirectories exist under that root, tars each into a dated archive under
`/var/backups/home-automation/` (14-day local retention), and best-effort `rsync --delete`s the
whole thing to `erikp@xanadu:/mnt/yotta7/HomeAutomation/<installation>/<hostname>/` — xanadu being
a "generally on-line" RAID box, so this step logs and skips gracefully rather than failing the
local backup when it isn't reachable.

On `protocols-server-1` specifically (not `-2`), the same script also mirrors the local **main**
Home Assistant instance's own self-managed `/backup` folder — HAOS's own automatic snapshots,
already self-cleaning on that side — via its "Advanced SSH & Web Terminal" add-on, staged locally
first (rsync can't hop directly between two remote hosts in one invocation) and then replicated to
`.../HomeAutomation/<installation>/main/` the same way. Deployed live 2026-09-08 on both houses'
`protocols-server-1` and Junglinster's `protocols-server-2`; all per-host specifics (which
`INSTALLATION` name, how xanadu and the main instance are each reached — this differs by house,
since xanadu isn't reachable the same way from Vienna as it is from Junglinster's own LAN) live in
`/etc/backup-home-automation.conf`, keeping the script itself identical everywhere.

Two real, worth-remembering gotchas from that rollout: GNU `tar` exits 1 (not a hard failure) when
a file changes mid-archive — expected for an actively-written database like Home Assistant's own
`home-assistant_v2.db`, tolerated explicitly rather than treated as fatal; `rsync` similarly exits
23/24 ("partial transfer") when some source files are permission-restricted (hit against a
subtree inside HAOS's own `/backup/config/` that even the SSH add-on's own account can't read),
tolerated the same way. Both are narrowly scoped tolerances — any other exit code still aborts the
script loudly, since the point is graceful degradation of *known* edge cases, not swallowing real
failures.

**Update (2026-09-21)**: Vienna's `protocols-server-2` (`frame`) picture-frame `~/bin` folder — the
hand-tuned, monitor/location-specific `*_slideshow` scripts plus its other utility scripts — is now
backed up too, without any script change: `/var/lib/home-automation/frame-bin` is a plain symlink
to `~/erikp/bin`, so the existing per-host iteration over `/var/lib/home-automation/*/` already
picks it up. This surfaced a real, previously-latent bug: GNU `tar` does **not** dereference a
symlink given as an archive-member argument by default, so the very first test backup captured
only the bare `frame-bin` symlink entry, not the real files behind it. Fixed by adding `-h`
(`--dereference`) to the script's `tar czf` invocation (now `tar czhf`) — deployed identically to
all four hosts that run this script (Vienna's and Junglinster's `protocols-server-1`/`-2`), keeping
the "identical everywhere" invariant the script's own header comment already declares. Re-verified
live: the tarball now contains all of `~/bin`'s real files. The equivalent action for
Frame@Junglinster (still RaspberryOS, no `~/bin`-worthy state yet) is tracked as PROJECT.md item 3,
to be done once that machine is migrated to Fedora.

Only Vienna's `protocols-server-2` (as it was still called at the time) needed this treatment —
it's the one house/host combination where "frame" and a `/var/lib/home-automation/`-backed host
were the same physical machine (see memory: Vienna frame/protocols-server-2 identity). The rest of
that host's own state (X11/slideshow config beyond `~/bin`, OS-level state) remained out of scope,
unchanged from before.

**Update (2026-09-21, same day):** that combination no longer exists. Following the p-s-2→p-s-1 HA
migration above, this host's role is now purely the picture frame, so its "protocols-server-2"
identity was retired entirely — Physical.def/Conceptual.def renamed it to `frame` across all four
places that string played an identity role (the `home_assistant` hassbridge qualifier, the `hosts`
DeviceID token, the `commandline`/picture-frame routing host, and the Conceptual.def position
label), matching Junglinster's own naming convention. The `hosts`-kind DeviceID token rename was the
one genuinely consequential piece (unlike the other three, which are decoupled from `unique_id`) —
it deliberately orphaned the rack CPU/liveness sensor's old entity history rather than preserving it
under the new name, a considered choice (the box's role genuinely changed, not just its label) that
the coordinator's own `RetireMissing` mechanism handled cleanly: the old entities were fully removed
from HA's registry (not left as disabled stubs) once Physical.def stopped declaring the old
DeviceID, and the new `frame`-named entities came up live in their place. Vienna's SSH port-forward
and OS hostname were also realigned to `frame`/1968 that same day, matching Junglinster's own
port-per-host convention. Deploy scripts (`deploy.d/02_deploy.hass.protocols-server-2`,
`05_deploy.commandline`) keep their old filenames (a rename churns less than it's worth) but their
internal targets/variables now point at `frame`.

---

## 12. Multi-house federation

§11.1's reasons for splitting compute nodes *within* a house — hardware locality, blast radius,
independent maintenance, incremental migration — apply again, unchanged, one level up. From the
other house's point of view, a whole house is just a much bigger compute node: its own physical
failure domain (a power cut, a router reboot, a WAN outage), its own maintenance schedule, its own
hardware, its own incremental adoption of whatever this project builds next. Treating "Junglinster"
and "Vienna" as two independent installations that *cooperate*, rather than two wings of one
bigger system under a single point of control, is the same instinct as §11.1's compute-node split —
just applied recursively at a larger scale, which is exactly what "**Federated** Home Assistant"
(this document's own title, §1) names.

The concrete consequence: the shared cloud broker must never become a hub either house *depends on*
to function. Each house's own MQTT broker, coordinator, and HA instance keep working exactly as
well with the cloud broker completely unreachable as with it up — the cloud broker exists to *share*
selected information between houses, never to coordinate either house's own core operation. A house
that stopped working the moment its WAN link dropped wouldn't be a federation member, it would be a
thin client of the other house (or of the cloud broker itself) — precisely the hub-and-spoke shape
this architecture deliberately avoids.

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

**Retired 2026-09-08: the `hosts`-kind cloud topic's installation-qualified segment.** Every
cloud-reporting `hosts`-kind device (`cpu`/report script, this section's own worked examples)
originally published under `hosts/<host>/<installation>/<suffix>/state` — the installation segment
existed solely to disambiguate two installations reporting the same bare hostname on the shared
cloud broker. Found live the same day: a single shared, Nextcloud-synced `secrets.cloud_client`
file is what every Mac in *both* houses reads, and each house's own `./generate`+`./deploy` wrote
its own installation's value into that one shared file — whichever house deployed most recently
silently overwrote the other's, breaking `eriks-macbook-pro-2`'s reporting the moment Vienna's own
`eriks-mac-mini` rollout deployed after it. The qualifier bought disambiguation that was never
actually needed: every cloud-reporting host (Mac or Unix — `studio`, `mini`, `pro-1`, `air-4`,
`mqtt`, ...) already has a globally unique hostname across the whole fleet, the same invariant a
plain local report already relies on. Retired the segment entirely rather than working around the
collision — `cpu/report` always publishes bare `hosts/<host>/...` now, `secrets.cloud_client` is
byte-identical regardless of which house generates it (no more per-installation
`secrets.<installation>` alias file either), and `mqtt_relay.go`'s `qualifyHostsTopic`/
`discoveryimport.go`'s equivalent cross-house-import handling were removed outright — both the
self-import relay and a real cross-house import (Vienna importing Junglinster's `pro-1`/`air-4`/
`mqtt`) now subscribe/publish the identical bare topic on both the cloud and local broker. The
*other*, whole-prefix installation qualification (`<installation>/homeassistant/...` discovery
topics, `<installation>/discovery_gateways/...` etc. — see the cloud existence-status entry below)
is a separate mechanism for a separate reason (two houses' own "main" instances colliding on their
own discovery topics) and is unaffected by this change.

**Roaming `home_assistant`-bridge devices** (an HA companion-app device, e.g. a phone, reachable via more than one local HA instance — `hass.eriks_iphone`, built 2026-09-02): `THassBridgeDevice.Instances []string` lets the same device be declared identically under more than one `integration home_assistant <qualifier> with:` block in the same house's Physical.def, merged (not colliding) generator-side. `ExportAs` (the `roaming` keyword, replacing `export`) qualifies the cloud cross-post with a shared virtual installation name ("roaming") instead of this house's own real name, so several real installations can each export their own local copy of the device under one stable cloud identity; `SelfImportFrom` (the `import` keyword) additionally relays that shared cloud data back onto this coordinator's own local bridge topic, feeding the one Spaces.def-positioned entity rather than creating a second one.

Four bugs found and fixed live 2026-09-05, the first day this was exercised for a real device (`hass.eriks_iphone`, roaming across `main`/`protocols-server-2` at Junglinster and `main` at Vienna):

- **Self-import feedback loop**: `subscribeHassBridgeSelfImport`'s relay republishes onto a *synthetic* local topic keyed by this house's own installation name (`homeassistant_instances/<installation>/bridge/...`) — never a real declared instance. Before the fix, `subscribeHassBridge`'s own local wildcard treated that synthetic topic as fresh genuine traffic and cross-posted it straight back to the cloud under `roaming`, which the self-import relay was itself subscribed to — an unbounded republish loop that flooded the shared cloud broker within seconds of first use. Fixed by gating cross-posting on the reporting topic's instance segment genuinely being one of the device's own declared `Instances` (`containsString`) — a synthetic echo never is, so it's relayed locally (the local broker copy still needs it) but never re-exported.
- **Non-canonical roaming topic**: a roaming device's cloud-published bridge topics (entity-state, device-info, and the availability reference inside each discovery config) embedded whichever *real* local instance happened to report most recently (`homeassistant_instances/main/bridge/.../state` vs. `.../protocols-server-2/bridge/.../state`), forcing any importer to watch both topics, or wildcard, to see the device's current value. Fixed by `canonicalizeRoamingBridgeTopic`: for a roaming device only (`ExportAs != ""`), every cloud-bound topic's instance segment is rewritten to one fixed literal (`"main"`) regardless of which real instance actually published — so an importer only ever needs to know one topic per capability.
- **Cross-house existence check false positive**: `checkKnownNotToExistErrors` (§6.9/§6.10, generator side) treats a capability as a real problem only when *every* declared instance confirms it not-to-exist — but `hassBridgeDevicesByID` is built from this house's own Physical.def alone, so a roaming device's `Instances` there is only ever this house's own local subset. Vienna's own `hass.eriks_iphone` declaration (`Instances: ["main"]`, Vienna's own main) failed generation outright the moment Vienna's own existence status became reliably fresh (see the immediate-publish fix below) — Vienna's own main instance genuinely has no local pairing for this capability, but the same roaming device is confirmed known-to-exist on Junglinster's own instances, information Vienna's generator has no way to see. A roaming device's existence can never be authoritatively judged from one house's own local instance(s) alone. Fixed by skipping the hard-fail check entirely for any device with `ExportAs != ""` — generated optimistically, same as an unresolved (`not-known-to-exist`) source.

**Cloud existence-status topic collision, found and fixed live 2026-09-05**: `discovery_gateways/<gatewayID>/existence/state` and `homeassistant_instances/<name>/existence/state` (§6.10) are published to both the local and cloud broker unconditionally when a cloud broker is configured — but, before this fix, under the *same bare topic* on both. Harmless locally (each house only ever reads its own), but wrong on the shared cloud broker: every house's own primary instance is conventionally named "main", and Junglinster's and Vienna's own retained existence status were confirmed live to be silently overwriting each other. Fixed by qualifying the CLOUD copy only with the owning installation's own name (`<installation>/discovery_gateways/...`, `<installation>/homeassistant_instances/...`), the same whole-topic-prefix convention `export`'s own cloud cross-post already uses (§6.12) — not the `hosts` kind's different "insert after first segment" convention, which exists solely for that kind's client-ID-scoped cloud ACL and doesn't apply here. Generator-side readers construct the matching qualified topic (using `ctx.Installation`, this house's own name) only when reading via the cloud broker; the local-broker read path is untouched.

**Follow-up gap, same day**: `publishStatus` (both kind-2 and kind-3) only ever fires on an actual known/retracted transition — a coordinator restart re-seeds already-known status in memory (from persisted JSON) without re-publishing it, so a topic-naming migration like the qualification fix above left the newly-qualified cloud topic with *no* retained value at all until the next real transition, which a stable gateway or instance might not produce again for a long time; every `./generate` cloud fetch timed out in the meantime (confirmed live for Junglinster's discovery gateways). Fixed by publishing every declared instance's/gateway's current status immediately on startup — `TEntityExistenceTracker.StartEntityExistenceInquiries` and the new `TDiscoveryExistenceTracker.PublishAll`, both called right after `Seed`.

**The exporting side owns all device-specific knowledge; an import device may declare neither derivations nor constant attributes** (2026-09-12): the same principle behind moving Netatmo `battery_alert` derivations off Vienna's imports and onto Junglinster's own exports (§2's "derived" note) generalizes to a hard rule, enforced in the parser rather than left as a convention. An importing house's Physical.def `device <local-id> from <house> <remote-id> with: ... end;` block may only ever list bare `<domain>.<capability>;` capability references (or the shared `for <device-id>:` shorthand) — it may not declare:

- a `derived ... via ...;` capability of its own (`integration_import_parser.go` recognizes the shape on sight and rejects it with a specific warning, rather than silently registering it as it briefly did during an earlier, temporary phase of the derived-capability rollout — see that file's own header comment), or
- a constant device attribute (`manufacturer:`/`model:`/etc.) — `TImportedDevice` structurally has no field for one, and the parser has never accepted the `hosts`/`hassbridge` `<name>: "<value>" [forced];` shape here either.

The reasoning is symmetric for both: an importing house should never need to know *how* a remote device's property came to be — whether a value is a raw sensor reading or something the exporting side computed via `derived`, and whether "Nabu Casa" is a hardcoded literal or something the exporting installation itself reports live — that is exactly the kind of "device-specific knowledge" this project consistently keeps on the side that actually owns the device. An importer only ever gets to say *which* of the exporter's already-fully-formed capabilities to relay locally, never to add device-level knowledge of its own on top. (Constant attributes reported live by the exporter and threaded through to the importer's own discovery config is the natural long-term shape here, but isn't built yet — today an imported device's `manufacturer`/`model` fields stay empty, per `registerImportedDevicePositioning`'s own doc comment.)

---

## 13. Open design questions

- Should **provider** become a first-class DSL concept (as opposed to today's `providing` macro pattern)?
- How should **projections** (to MQTT, to HA YAML, to future backends) and **transformations** be represented in the semantic model — and how should reusable patterns across them work? Partially
  answered 2026-09-09 for the physical-layer, single-base-entity case (see §2's own "Where
  physical-layer derivation ends" note and `splendid-zooming-lighthouse.md`): a `derived` capability
  declared in Physical.def, realized as an MQTT discovery `value_template` composed via two
  coordinator-resident tables (a per-capability value_template table, and a template-recipe table
  keyed by base/derived capability) rather than a separate generator-authored HA `template:` entity —
  designed, not yet built. Still fully open: the equivalent story for logical-layer, multi-device
  transformations and command choreography (PROJECT.md item 14's inverse-command-routing question).
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
