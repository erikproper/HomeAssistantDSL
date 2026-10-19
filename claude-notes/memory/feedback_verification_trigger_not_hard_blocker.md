---
name: feedback-verification-trigger-not-hard-blocker
description: "an offhand \"should be X, of course...\" is a prompt to verify rigorously and report honestly, not a hard requirement that blocks progress if reality deviates slightly"
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-28T12:34:14.708Z
---

When the user says something like "Should be byte equivallent, of course ..." in passing, treat it
as an instruction to verify carefully and report the truth -- not as a hard constraint that must
literally hold before proceeding. If analysis/diffing reveals a small, well-understood, benign
deviation (e.g. one unused auto-materialized entity stops being generated), explain exactly what
changed and why it's safe, then ask whether to proceed -- don't assume the deviation is a blocking
regression, and don't silently force byte-identity by adding a special case just to satisfy the
letter of the offhand remark.

**Why:** confirmed 2026-09-28 during the Logical.def `is enabled by` migration
(`appliance.tv_living_room`, Junglinster) -- the regenerated output wasn't byte-identical (a
previously-auto-materialized indirection entity, never referenced anywhere else, stopped being
generated). Flagged it precisely (which files, why, functional-equivalence proof). The user's
response: "No. Go ahead. Was only intended as a 'trigger'." -- confirming the phrase was meant to
make me check thoroughly and surface anything I found, not to gate the change on literal
byte-identity.

**How to apply:** whenever a similar "should be X, of course" aside precedes a verification step,
do the verification, present the honest result (including any deviation and why it's benign or
not), and let the user decide -- rather than either (a) skipping the check because it seems
obviously true, or (b) treating a minor discovered deviation as a hard stop.
