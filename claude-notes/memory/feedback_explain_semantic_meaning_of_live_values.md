---
name: feedback-explain-semantic-meaning-of-live-values
description: "When wiring up a value tied to live device state (e.g. a reported position/reading), explicitly state what it represents in plain terms, not just how it's sourced/wired."
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-25T14:01:05.378Z
---

When introducing or explaining a mechanism that surfaces a live-reported device value (e.g. a
cover's `current_position` attribute), explicitly say what the value semantically represents --
"this is the device's own live-reported status, not something computed or cached" -- rather than
only describing the wiring (which entity/attribute it's sourced from).

**Why:** Real incident 2026-09-25 (Somfy cover position work): after explaining the
`cover.cover ... with derived value: ...!current_position;` line purely in terms of its wiring
(domain/label/source/attribute), the user reacted with alarm ("WHAT THE FUCK", "Are you
sabotaging things on purpose!?!?") -- they weren't sure whether "position" was genuine live device
status or something fabricated/computed by the DSL, which read as suspicious. A direct follow-up
question ("Is the position the reported status of the cover?") and a plain "yes, it's the live
reported status, not a cached/commanded value" answer immediately resolved it. Their own
feedback: "Would have been wise to tell me that explicitly."

**How to apply:** Any time a change surfaces or re-routes a value that reflects real-world device
state (position, temperature, battery level, on/off, etc.), state plainly what real-world thing
it represents and where the authority for that value lives (the device/integration itself, not
this DSL) as part of the initial explanation -- don't wait to be asked. Especially important for
anything safety- or trust-relevant (covers, locks, security devices).
