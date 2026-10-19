---
name: project-retiremissing-passthrough-wipe-incident
description: "RetireMissing was deleting every passthrough-relayed Zigbee2MQTT entity (and manual renames) on every coordinator restart, until fixed 2026-09-14 — root cause and fix, relevant to any future discovery_passthrough migration (Junglinster item 8)."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-14T19:46:15.888Z
---

**Real incident, 2026-09-14, found during Vienna's live test of PROJECT.md item 4's
discovery_passthrough mechanism.** `RetireMissing` (`house_event_bus_coordinator/discoverycleanup.go`)
retires any topic recorded in the coordinator's persisted manifest that isn't in the current run's
generator-computed `expectedTopics` set. Passthrough-relayed topics (item 4's whole point: relay a
not-yet-declared Zigbee2MQTT device byte-for-byte) can never be in that static set — no Physical.def
declaration exists for them yet — but the plain `Publish` call still recorded them as "known".
Result: **every single coordinator restart deleted every currently passthrough-relayed entity**
(and any manual HA rename on it, since an MQTT-discovery delete removes the whole registry row),
only for Zigbee2MQTT to recreate a fresh, unqualified one moments later. This repeated on every
restart during a long live debugging session, compounding into ~180 entities needing registry
restoration from a same-morning HAOS backup (see [[project_haos_encrypted_backup_decrypt_procedure]]).

**Fix** (committed 7b0aba3): `TDiscoveryPublisher.PublishPassthrough` marks a topic exempt from
`RetireMissing`'s sweep (persisted alongside the existing manifest); the passthrough relay path
routes through it via a `TDiscoveryRelayJob.Passthrough` flag instead of plain `Publish`.
`RetireOne` forgets the exemption too, so it doesn't outlive the topic's own real retirement.

**How to apply**: this fix is now committed and will be picked up automatically the next time
Junglinster's coordinator gets deployed (a normal `./deploy` run rebuilds+restarts it from current
source unconditionally — see [[project_coordinator_deployment]]) — but *verify* that's actually
happened before ever flipping Junglinster's own Z2M discovery prefix to
`${mqtt_discovery_physical}` (item 8), don't assume the binary already there is new enough. See
PROJECT.md item 8's own notes (added 2026-09-14) for the fuller checklist this incident produced.
