---
name: feedback-two-phase-deploy-for-template-to-derived-migrations
description: "Migrating ANY entity off a generator-authored YAML template mechanism onto a new coordinator/MQTT-discovery-based one needs a two-phase deploy, or HA registers a _2-suffixed duplicate instead of reusing the clean name. General rule, not scoped to the derived mechanism specifically."
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-11T10:54:12.324Z
---

**The rule, stated by the user (2026-09-11), and explicitly generalized by them the same day**:
this applies to retiring ANY generator-authored YAML-template entity in favour of a new
coordinator/MQTT-discovery-based one — not just today's `derived`/`condition`/`battery_alert` case.
The same two-phase discipline applies to every future migration of this shape, most notably the
still-upcoming Zigbee2MQTT legacy-to-conceptual passthrough work (PROJECT.md items 7/10) and
anything else that moves an entity off generator-authored `template:` YAML onto coordinator-owned
discovery. When migrating an entity currently defined by a generator-authored YAML template (the
old `condition`/`adjustment`/`battery_alert` mechanism, `generateConditionEntities`/
`generateTemplateBinarySensors` et al., or any other template-YAML-authoring code path) onto a new
coordinator/MQTT-discovery mechanism (`derived` being the first concrete case,
[[project_derived_capability_mechanism]]), do it in two separate deploy passes, never one combined
edit+deploy:

1. **Remove** the old macro/condition usage from Spaces.def ONLY (don't add the new `derived`
   construct yet). Deploy, then reload (HA **main** instance only). This lets HA actually release
   the old template entity's registry slot before anything new tries to claim the same name.
2. **Then** add the new `derived entity ...;` Physical.def declaration + its Spaces.def
   positioning. Deploy the coordinator (so it picks up the new manifest and can publish discovery
   once state arrives). Reload the automations of the "source" HA instance too, if needed (the one
   actually running the reporting automation, e.g. p-s-2) — this is what actually gets the state
   flowing so discovery follows.

**Why**: the old template entity and the new MQTT-discovered one are different platforms with
different `unique_id`s, even though they target the same `entity_id`. HA's registry keys identity
off `(platform, unique_id)`, not the bare entity_id — so removing the old declaration and adding the
new one in the SAME deploy doesn't give HA a chance to tear down the old registration first; the new
one gets registered as a genuinely different entity, and since the old one is still occupying the
clean name, the new one gets bumped to a `_2` suffix instead.

**Confirmed real incident**: this happened live 2026-09-11 with `daylight` on Junglinster's main HA
instance — a `_2`-suffixed duplicate appeared because the Spaces.def/Physical.def edit and deploy
were done in one combined pass, exactly the sequence this rule says to avoid. (Vienna's own earlier
sun/daylight migration didn't hit this, likely differing in some Vienna-specific circumstance not
fully understood — but the two-phase sequence is the safe default regardless, don't rely on getting
lucky.)

**How to apply going forward**: treat this as the default sequence for ANY "retire a
generator-authored template entity, replace with something coordinator/MQTT-discovery-based"
migration, not a `derived`-specific checklist item. Concretely due next: Junglinster's 7 real
`battery_alert` candidates, Junglinster's own sun/daylight case — each needs "remove old usage +
deploy + reload main" confirmed clean, THEN "add the new construct + deploy coordinator + reload
source instance" as a distinct second step. Further out, the same discipline applies to the
Zigbee2MQTT legacy-to-conceptual passthrough migration (items 7/10) once that work starts moving
raw Z2M entities off any template-YAML wrapping they currently have. Don't collapse either phase
into one edit+deploy pass even when it looks safe.
