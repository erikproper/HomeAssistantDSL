---
name: feedback-verify-zigbee-group-before-deleting
description: "Never delete a Zigbee2MQTT native \"Group\" (or any entity) just because it looks like an unqualified/orphaned duplicate — check its entity_id history first."
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-14T19:45:46.113Z
---

Real mistake, 2026-09-14 (Vienna): found 3 Zigbee2MQTT-native "Group" discovery configs
(`model: "Group"`) publishing under raw, unqualified friendly names (e.g.
`apartment/living_room/lights/main`) that didn't match any Physical.def declaration, and deleted
all 3 via `zigbee2mqtt/bridge/request/group/remove`, assuming they were legacy orphaned duplicates
unrelated to the DSL. They weren't — they were the user's real, actively-used aggregation lights
(`light.social_apartment_living_room_main` etc.), manually renamed in HA's own entity registry away
from their raw Z2M default at some point in the past. The raw/unqualified name visible in the live
MQTT discovery **config** only reflects Z2M's own default naming; it says nothing about what the
entity is actually registered as in HA, since a manual entity_id rename lives only in
`core.entity_registry`, never in the discovery payload itself.

**Why:** I judged "orphaned duplicate" purely from the discovery config's own default naming
looking unlike the DSL's own conventions, without checking whether that entity_id (or its
unique_id) had any history — a HAOS backup's `core.entity_registry` would have shown immediately
that it was renamed and in active use.

**How to apply:** before deleting/retiring ANY entity (a Zigbee2MQTT group, a device, a discovery
topic) on the grounds that it "looks like" an orphan or duplicate, check its actual entity_id
history first — either against a recent HAOS backup's `core.entity_registry` (see
[[project_haos_encrypted_backup_decrypt_procedure]] for how to decrypt one), or by asking the user
directly. "Doesn't match the DSL's naming convention" is not the same as "unused" — a live, renamed,
actively-relied-on entity can look identical to a genuine orphan from the discovery config alone.
If it does turn out to need recreating, Zigbee2MQTT's `bridge/request/group/add` accepts an
explicit `"id"` field — reusing the original numeric group id keeps the resulting unique_id
identical, avoiding an extra unnecessary rename on top of the recovery.
