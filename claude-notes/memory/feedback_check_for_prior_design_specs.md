---
name: feedback-check-for-prior-design-specs
description: "Before designing a new coordinator/generator feature from scratch, check memory and PROJECT.md for a design the user may have already given -- don't reinvent."
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-08-27T11:21:23.119Z
---

When the user asks for a feature that sounds like infrastructure/architecture work (a new MQTT
topic scheme, a caching strategy, a cross-installation mechanism), search memory and PROJECT.md
for a prior design *before* proposing one from scratch — the user has a habit of specifying
designs well ahead of when they're buildable ("for now, keep this as a note, until we have X
online"), then expecting the note to be found and followed once the trigger condition is met,
rather than re-explaining it.

**Why:** built the entity-manifest/suggestion-tool feature twice in one session (2026-08-27). The
first version (generator connects directly per HA instance, aggregates into one topic, picks one
preferred broker, no local persistence) was my own invention. The user then quoted back, nearly
verbatim, a design they'd given earlier — per-instance topics keyed by installation
(`meta/manifests/<installation>/<instance>/state`), publish to *both* local and cloud
unconditionally, coordinator (not generator) does the aggregation/relay, and the generator keeps
a *persisted local cache* (fresh when online, stale fallback when offline) — explicitly deferred
earlier with "for now, we can keep this as a note, until we have the cloud broker online as part
of the Netatmo integration." That trigger condition was met mid-session (cloud broker built
earlier the same session, Netatmo integration actively in progress), but I didn't reconnect the
two. Cost a full rebuild plus visible frustration ("How many times do I need to write this!!!
READ!!").

**How to apply:** when a request touches cross-cutting infrastructure (broker topology, caching,
deploy/reload triggers, multi-installation federation), check `MEMORY.md`'s project-note index
and PROJECT.md's "Smaller pending items"/deferred notes for a matching prior design before writing
code. A deferred note with an explicit trigger condition ("until X is online/built/deployed") is
a live spec waiting for that condition, not a stale idea to re-derive independently.
