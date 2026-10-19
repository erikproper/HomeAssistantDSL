---
name: feedback-generator-coordinator-terminology
description: "Always call the two system components \"generator\" and \"coordinator\" in docs/prose -- never \"compiler\", \"transformer\", \"processor\", etc."
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-08-26T16:34:58.087Z
---

Use exactly "generator" and "coordinator" as the names of this project's two components, in
README.md, Architecture.md, commit messages, and chat -- never a synonym like "compiler",
"transformer", "processor", "translator", or "engine", even when it feels natural in a given
sentence (e.g. "the generator is a batch compiler").

**Why:** the user explicitly asked to standardize on these two names (2026-08-26). Architecture.md
§8 ("System components") already defines them canonically, but §9 had drifted into calling the
generator a "compiler" throughout its own section title and body -- found and fixed same session.
"Event bus" (MQTT itself) and "hub" (Home Assistant, in the "not a single hub" argument) are
different concepts and are fine to keep -- this rule is specifically about naming the two Go
components ([[project_roaming_devices_bridging_broker]] documents recent work on the coordinator
side).

**How to apply:** when writing or editing README.md, Architecture.md, PROJECT.md, or any other
prose describing this system, grep for stray synonyms before considering the writing done, the
same way this session did (`grep -n "compiler\|transformer\|processor\|engine"`). In code comments
this project already reliably uses "generator"/"coordinator", so this is really a docs/prose-only
reminder.
