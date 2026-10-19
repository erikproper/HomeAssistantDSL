---
name: eu-english-spelling-standard
description: "All code comments, documentation, and communication must use EU (British) English spelling"
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-14T20:19:49.148Z
---

Use EU English spelling throughout — in code comments, commit messages, documentation, and all communication. Examples: organise (not organize), standardise, normalise, colour, behaviour, recognise, analyse, licence (noun).

**Why:** User explicitly requested this as the project standard — they speak English, not American.
**How to apply:** Any time writing comments, variable names where English words are spelled out, commit messages, or documentation in this project.

**Exception, real slip 2026-09-14**: Home Assistant's own MQTT discovery schema uses American
spelling in its literal field names (`color_temp`, `supported_color_modes`, `rgb_color`, ...) —
those field names must stay exactly as HA defines them, never respelled. The rule is for prose:
when writing ABOUT those fields in comments/chat ("colour support", "colour-mode handling"), use
the EU spelling even though the field name itself, quoted verbatim, stays American. Caught after
writing "color modes"/"color-mode-shaped fields" in chat prose while the literal field name
`supported_color_modes` was also in scope — don't let discussing an API field's name drag its
spelling into surrounding prose.
