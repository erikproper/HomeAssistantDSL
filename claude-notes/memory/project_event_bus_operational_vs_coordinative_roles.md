---
name: event-bus-operational-vs-coordinative-roles
description: "Future architecture decision (2026-09-22) - state-enforcement for report-less devices (FHEM) is a separate, higher-reliability \"operational\" Event Bus role; also indexes the 4-way Logical.def situation breakdown (item 11) and which situations are coordinator-vs-separate-component candidates"
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-22T10:57:15.778Z
---

Decided 2026-09-22 (PROJECT.md item 19): when integrations are added for devices that accept
commands but never report state back (FHEM is the motivating case), the "remember the expected
state and periodically re-broadcast/re-enforce it" responsibility must NOT be folded into the
existing coordinator process, even though it looks like a natural extension at first glance.

**Why**: the coordinator's job is COORDINATIVE — "what is available", discovery/existence
bookkeeping. State re-enforcement for report-less devices is OPERATIONAL — it's materially part of
actually keeping devices in their expected state, not just tracking what exists. The user's own
distinction: a per-message integration-level component (e.g. FHEM2MQTT) should just relay on/off
commands; a *separate* component maintains the expected state and re-enforces it via regular
broadcasts.

**How to apply**: this operational role should be:
- Its own separate "role" on the Event Bus, deployed in its own container — not merged into the
  coordinator.
- Held to a much higher reliability bar than the coordinator. Coordinator crashes are relatively
  low-stakes because the coordinator does not (should not) play an operational role in exchanging
  actual state/command messages — see [[project_junglinster_coordinator_crash_loop_2026_09_22]] for
  a concrete case where the coordinator crash-looped for ~3 hours with zero impact on live
  state/command traffic between devices and Home Assistant. An operational-role crash, by contrast,
  would mean devices silently drift from their expected state — a real user-facing failure.

Not yet designed: how this role subscribes/publishes relative to the coordinator and per-
integration relays, how "expected state" gets declared (DSL-level?), re-enforcement cadence, FHEM/
FS20 protocol specifics. Read PROJECT.md item 19 before starting the FHEM integration.

**Where this fits in the broader Logical.def design** (added 2026-09-22, PROJECT.md item 11): the
user broke Logical.def work down into (at least) four distinct situations —
1. simply clustering existing capabilities,
2. derived entities that require TIMING (delay_on/delay_off, ...),
3. derived entities that rely on OTHER entities from other devices (or main HA),
4. ABSTRACTED entities that accommodate for device shortcomings (no state report-back,
   multi-broadcast radio, ...) — this FHEM case is specifically situation 4.

Of these, (2) and (4) are the two the user flagged as candidates for living either IN the
coordinator or in a separate component — genuinely open, not decided. (1) and (3) are more clearly
coordinative (composition/derivation over already-known state), while (4) leans operational per
this memory's own reasoning above. Read PROJECT.md item 11's own 2026-09-22 note alongside item 19
before designing either.

**"Guarding automations"** (PROJECT.md item 12(3), added 2026-09-22): a related but distinct idea —
reload-on-unavailable for quirky HA integrations (Netatmo is the concrete case), rate-limited (e.g.
4/hour), implemented as a generator-created automation living ON the affected instance itself, not
a separate component. Refined same day: guard rules are declared in Physical.def's own INTEGRATION
sections (per integration type, alongside that integration's other declarations), with
INTEGRATION-SPECIFIC parsing — matching this project's existing per-integration parser architecture
(each integration type owns its own grammar file, e.g. integration_hassbridge_parser.go), not a
generic cross-integration "meta" syntax.
