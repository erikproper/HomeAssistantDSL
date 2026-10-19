---
name: project-stable-discovery-identity-architecture
description: "Queued 2026-09-20, NOT started: architectural principle for coordinator<->HA identity, agreed after the Junglinster xanadu/disks_1 passthrough collision incident. Sequenced AFTER Junglinster fixes + open Vienna questions are resolved."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-20T14:56:52.209Z
---

**The principle** (Erik's own framing, 2026-09-20, after the passthrough name-collision incident --
see [[project_passthrough_name_collision_guard]]):

1. The generator defines the ID for a given entity/device -- it must be a function of the entity's
   own POSITIONING (conceptual/logical placement, which may itself be for "internal"/physical-only
   use), and of nothing else. Never derived from the physical source's own naming/topic content.
   Already true today for DECLARED devices (discovery/hassbridge/etc. positioned via
   Conceptual.def) -- the gap is specifically raw PASSTHROUGH, which deliberately skips positioning
   entirely for zero-effort visibility, and inherits Zigbee2MQTT's own naming instability as a
   result (exactly what broke live this session).
2. If the coordinator discovers that a previously-discovered source entity/device (seen via
   `homeassistant.physical`) no longer exists, this must be REPORTED, not silently acted on: a new
   meta binary_sensor ("something's missing" style aggregate, live in HA) plus itemized detail
   surfaced during `generate` (extending the existing suggestion-report mechanisms,
   discovery_existence.go / mqtt_entity_existence.go / discovery_passthrough_suggestions.go).
3. Unless the CONCEPTUAL/LOGICAL reference itself is deleted by the user, the shared
   config/identity between coordinator and HA's main instance must PERSIST -- a vanished or changed
   physical source must never retract or rename the HA-facing entity on its own.
4. The ID must persist even when the physical backing of a conceptual-layer entity is swapped for a
   different one (e.g. replacing which discovery device backs `switch.social:xanadu`) -- already
   true today as long as the capability's own key name is unchanged across the swap, since identity
   derives from the Conceptual.def path, not the physical device.

**Why this matters / the real tension**: this is a deliberate REVERSAL of several retire behaviors
already live today, all of which currently treat "physical source changed or vanished" as a reason
to actively retire the HA-facing discovery config:
- `discoverycleanup.go`'s `TDiscoveryPublisher.RetireMissing` (startup sweep against an expected
  topic set -- already excludes passthrough topics specifically, per a 2026-09-14 incident, but
  would still retire a DECLARED entity whose source vanished).
- `discoverybridge.go`'s empty-payload handler (treats an empty retained payload as a genuine
  retraction and retires immediately).
- `TDiscoveryPublisher.Publish`'s own retire-on-content-change (`discoverycleanup.go` line ~331,
  "content changed -- force a fresh HA registration").

Under the new principle, none of these should retire a DECLARED entity's config just because its
physical source changed/vanished -- only report it. Needs real design work: how to distinguish
"temporarily offline" (normal, don't touch) from "genuinely gone forever" (still don't retire, per
the principle, but the missing-indicator and generate-time report need to say so clearly); what the
entity's own HA-visible state should do while its source is missing (presumably just goes
"unavailable" naturally via its own availability topic, without the discovery CONFIG itself being
touched).

**Sequencing** (Erik's own explicit ordering, 2026-09-20): 
1. First finish fixing the CURRENT Junglinster issues (the passthrough collision incident and its
   fallout -- see [[project_passthrough_name_collision_guard]], still being manually repaired
   device-by-device as of this note).
2. Then resolve open Vienna questions (unspecified as of this note -- ask Erik what these are before
   starting anything Vienna-side).
3. THEN take on this architectural work -- balancing discovery's convenience, the explicit
   Physical.def/Conceptual.def link, and a genuinely stable coordinator<->HA relation.

**How to apply**: do NOT start implementing any part of this until Erik explicitly asks -- it's
queued behind two other pieces of work per his own sequencing. When picked up, re-read
[[project_passthrough_name_collision_guard]] and [[project_discovery_device_block_and_node_capability]]
(the migration playbook) first, since both are directly relevant precedent.
