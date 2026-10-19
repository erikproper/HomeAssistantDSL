---
name: project-stable-entity-identity-design
description: "SUPERSEDED 2026-10-05 -- now DONE+BOOTSTRAPPED, see project_stable_entity_identity_implementation_2026_10_05.md for what was actually built (both open questions below resolved: position-at-birth with a plain _n disambiguator, full two-house scope). Keep this file for the original design rationale/history only."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-10-06T01:34:00.792Z
---

Full design for a real architectural gap: today the coordinator's own relayed-entity unique_id is
`relayedDiscoveryUniqueID(gatewayID, leaf)` — a pure function of Physical.def's raw declaration,
recomputed fresh on every discovery message, never persisted. So a hardware swap that keeps the
exact same conceptual DSL position (replacing a Z-Wave switch with a Zigbee one, a leaf/topic
renumbering, etc.) still changes the unique_id, and HA treats it as a brand-new entity — the same
`_2` registry-collision dance this session repeatedly fixed by hand (furnace switch, kitchen sonos,
corb) keeps recurring for exactly this reason.

**The design** (arrived at through several iterations with the user — an earlier
coordinator-owned-database framing was explicitly rejected in favour of this one, see "Why" below):

- The **generator** assigns a permanent id the moment a capability is first declared bare (no `as`
  suffix), device-name + capability-name derived, and writes it back into Physical.def itself:
  `light.light_1 zwavejs2mqtt_0xd0204fb2_71-37-1-currentValue;` becomes, after the generator's next
  run, `light.light_1 zwavejs2mqtt_0xd0204fb2_71-37-1-currentValue as <assigned-id>;` — once an
  `as ...` is present, it is authoritative and the generator never touches it again, regardless of
  later raw-leaf changes, conceptual repositions, or renames of the device id or capability label
  itself. This is the key property: the annotation survives strictly more kinds of change than a
  name-derived key would, because once frozen it no longer depends on anything that could later
  change.
- The **coordinator** makes a small, contained change: prefer this carried-through id (threaded via
  `discovery.yaml`) over its own computed `gatewayID+leaf` when present. It does NOT become
  authoritative for identity — it stays a faithful executor of whatever Physical.def says, exactly
  matching the rest of this architecture's own division of labour (generator decides, coordinator
  executes). No new coordinator-owned database.
- **One-time bootstrap, required before this can ship**: every capability that's ALREADY live today
  needs its `as <id>` seeded from its CURRENTLY assigned unique_id (pulled once from the
  coordinator/HA registry) — never freshly minted for something already live, or every existing
  entity would collide with itself on the very first run. Scripted, byte-diffed, dated backups —
  same discipline as every other `.def` rewrite this session.
- **Side benefit, nearly free once built**: an "orphaned id" warning at generate time — an
  `as <id>`-carrying capability that nothing in Conceptual.def positions anymore — the natural
  complement of the already-existing "undeclared entity" checks (which catch the opposite
  direction).

**Second live motivating incident, 2026-10-05**: fixing 4 heating relays (discovered mid-passthrough-
audit to be raw Zigbee2MQTT devices wrongly declared as native `home_assistant`-bridge entities)
required moving their entity_id from one HA platform (`template`) to another (`mqtt`,
coordinator-published) — same intended entity_id, but a full stop-core/edit-registry/start-core pass
was needed to free the name first, since HA keys identity off `(platform, unique_id)` not the bare
entity_id. Documented in Architecture.md §6.4 (new paragraph, same section as the 2026-09-09
passthrough-naming incident) as a second concrete illustration of why this design matters, at the
user's own explicit request ("it adds to the motivation of the whole setup").

**Open design question added 2026-10-05, not yet resolved**: what should a NEWLY-assigned `as <id>`
actually look like, once this design is implemented? The user's own explicit instruction: an
**existing** id must never change (this design is about freezing identity, not renaming anything) —
the question is only about what a *fresh* assignment (a capability that has no `as ...` yet) should
be. Two options raised: (a) an opaque unique number, or (b) something semantically meaningful. The
user's own stated inclination is (b), specifically: use the entity's own **conceptual position at
the moment it was first positioned** — its "position of birth" in the conceptual layer — as the id
itself, rather than a number or a device-name+capability-name derivation (the tentative placeholder
the original design sketch above used). Not yet designed further (how exactly a conceptual position
string becomes a valid `as <id>` token, what happens if that SAME position is later vacated and
reused by a different entity, etc.) — flagged for the next design session on this item, due after
PROJECT.md's own "/clean" pass per the sequencing note below.

**Why the coordinator-owned-database framing was rejected**: the user's own reasoning, and it's
right — Physical.def is already the moment a human "selects" a raw suggestion into the conceptual
model; that's the natural place to freeze an identity, not a separate runtime table the generator
would have to trust and back up independently. Keeping identity git-diffable and human-reviewable
in the `.def` file fits everything else about this project's own philosophy (the `.def` files ARE
the source of truth, not a side database).

**Sequencing, per the user's own explicit instruction (2026-09-29)**: PROJECT.md item 1 lists this
right after its own "do a /clean" bullet — implement this AFTER that cleaning pass, not before.
Separately, PROJECT.md's own "moes related renames" item is flagged as registry-impact-sensitive
and should be sequenced AFTER this design lands, if avoidable, since renames are exactly the class
of change this mechanism is meant to make free of registry churn.

**How to apply**: before starting any Junglinster/Vienna rename or hardware-swap migration work
that's likely to cause a registry collision, check whether this design has landed yet — if not, the
collision is expected and the established registry-surgery playbook
([[project_ha_entity_registry_collision_lessons]]) still applies; if it HAS landed, that class of
churn should no longer occur at all, and a collision would indicate a real bug in this mechanism
rather than an expected cost of the migration.
