---
name: feedback-budget-conscious-session-management
description: "User actively tracks weekly usage-limit percentage and proactively freezes larger/speculative work when it runs low, reserving the remainder for emergency fixes."
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-09T20:30:43.260Z
---

When the user mentions remaining weekly budget/usage-limit percentage (e.g. "75% left, resets in
3 days"), treat it as a real constraint, not idle chatter: they will proactively ask to pause or
"freeze" larger, non-urgent design/build work (e.g. PROJECT.md item 4's `derived`-capability
mechanism, frozen 2026-09-09) so remaining budget is available for genuine emergency fixes,
especially before travel (a conference trip, in this case) when they won't be able to babysit a
long session.

**Why:** the user is directly billed/limited by usage, and correctly distinguishes "planned,
resumable work" (safe to pause with a clear plan-file pointer) from "live incident, needs budget
now" (never pause).

**How to apply:** when told to freeze/pause a work stream for budget reasons, update PROJECT.md
(or the relevant plan file) to record the freeze explicitly — state, reason, and that the design
itself is unaffected/complete, not just silently stop working on it. Don't propose resuming large
speculative work unprompted while a freeze is in effect. Live incidents and small, well-diagnosed
fixes (e.g. today's picture-frame slideshow bugs) are NOT what "freeze" applies to — those get
fixed regardless, since leaving a real bug half-fixed is worse than the token cost of finishing it.
