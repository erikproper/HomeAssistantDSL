---
name: feedback-project-md-stays-a-todo
description: "PROJECT.md is a shrinking backlog, not a changelog — completed work's lasting detail belongs in README.md/Architecture.md, not accumulated there."
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-10-04T21:01:33.912Z
---

PROJECT.md should trend toward *empty* as work completes, not grow. A growing PROJECT.md is a bad
sign. It's a TODO/backlog, not a journal of everything that was built, when, or how it was verified.

**Why:** the user said it directly, 2026-08-29, after a stretch of sessions where I kept appending
long "Status (done)" / "Test plan" / "Also caught and fixed" narratives directly into PROJECT.md
item entries (e.g. 1.1's entity-existence work, 1.8's kind-2 tracking) — each confirmed-done item
grew its own multi-paragraph build log in place rather than shrinking or disappearing. The user was
manually trimming PROJECT.md back down around the same time this feedback was given.

**How to apply**: once an item is genuinely done:
- Remove it from PROJECT.md, or shrink it to a bare pointer, rather than leaving a "Status: DONE"
  essay in place.
- If the *outcome* has lasting relevance to a reader/user/developer (what the system does now, why
  it's shaped that way, what to know before touching it again) — write that into Architecture.md
  (design/behavior reference) or README.md (user-facing), not PROJECT.md.
- Session-scoped build narrative (what broke, what the fix was, which commit/test covers it) is
  what memory (this file store) and git history are for — it doesn't need a home in the repo's own
  docs at all, in PROJECT.md or elsewhere.
- PROJECT.md entries should stay terse: what's left to do, and why, not a log of what already
  happened. When in doubt, cut instead of appending.

This reframes how I should treat "update the docs when something is confirmed done" instructions
[[feedback_terse_chat_and_rebuild_before_test]] gave earlier this project: "update the docs" means
*trim PROJECT.md and enrich Architecture.md/README.md*, not append a done-status paragraph to
PROJECT.md and call it updated.

**Repeated relapse, 2026-09-07**: despite this memory existing since 2026-08-29, I did the exact
same thing again — wrote a 32-line "DONE + DEPLOYED" essay into PROJECT.md for the cross-house
import shorthand work instead of documenting it in README.md/Architecture.md and leaving a short
pointer. The user had to call it out directly a second time ("I asked you several times to do so,
but you keep only just adding 'Done'"). Fixed live that session: added a real README.md section
("Devices (cross-house import)") documenting the grammar, trimmed the PROJECT.md bullet to 5 lines
pointing at it.

**Third relapse, 2026-10-04**: wrote a long "ANSWERED 2026-10-04" explanation directly into a
PROJECT.md item (1e, the "dependency on" three-way-dispatch question), then even after moving the
detail to Architecture.md §6.7, left a one-line "ANSWERED -- see Architecture.md §6.7" pointer
stub behind in PROJECT.md. The user: "As said, over and over again. Project.MD should end up as an
empty file in the END." This OVERRIDES the (a)/(b) choice below — a pointer stub is ALSO clutter,
not an acceptable middle ground. Fixed live: deleted the pointer line entirely, keeping only the
still-open follow-up question (renumbered into the same slot) with a "(see Architecture.md §6.7)"
inline reference for context, not a dedicated stub line.

**Concrete trigger going forward**: the moment a body of work is confirmed done (tests pass, live
deploy verified, etc.) and I'm about to touch PROJECT.md to note it, that SAME edit must write any
durable design/behavior detail into README.md or Architecture.md first, then DELETE the PROJECT.md
entry outright — no "Status: DONE" essay, and no one-line "see Architecture.md" pointer stub either.
A finished item leaves zero trace in PROJECT.md; if a reference to the now-documented behavior is
still needed (e.g. a related open follow-up question), fold it inline into the open item's own
text rather than giving the finished item its own line. The file should visibly shrink toward empty
as work completes — never write the done-narrative directly into PROJECT.md as the first draft "for
now," and never leave a pointer behind "just in case" — that's exactly the pattern that keeps
recurring.
