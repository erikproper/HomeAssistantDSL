---
name: feedback-ask-one-at-a-time
description: "When several genuinely ambiguous items need clarification, ask about them one at a time and wait for each answer, not as one batched multi-question prompt."
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-08-29T21:12:33.647Z
---

When migrating/mapping several ambiguous items (e.g. multiple Spaces.def rooms needing the right
`hass.*` device chosen), do **not** batch all the open questions into a single multi-question
prompt. Ask about one item, wait for the answer, then ask about the next.

**Why:** the user said explicitly (2026-08-29, mid Netatmo Spaces.def migration): "I answered you I
would give you info on which device to choose, and then you went immediately to the next[, that's
wrong]. That won't work. One by one please." They rejected a 4-question batched
`AskUserQuestion` call for exactly this reason.

**How to apply**: this project specifically. When several independent facts need clarification from
the user before proceeding, resist the urge to bundle them into one efficient batch (even though
the tooling supports up to 4 questions at once) — ask the single most immediately-relevant one, let
the user answer, then move to the next. This applies to `AskUserQuestion` calls and to plain
conversational clarifying questions alike.
