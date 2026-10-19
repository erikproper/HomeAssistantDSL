---
name: project-new-architecture-plan
description: "User's planned next phase after current bash-parity debugging: shape a new DSL/generator architecture via experiments, streamlining the DSL itself."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-08-10T15:12:20.771Z
---

Alongside continuing to debug the current Go generator against the two live houses, the user's
next phase of work is to move toward a new architecture for this project. The stated approach is
to first shape the architecture itself and run initial experiments, rather than jumping straight
to a full rewrite. This is expected to lead to a streamlined DSL — not just backend/generator
changes, but simplification of the DSL language surface itself.

**Why:** stated directly by the user on 2026-08-10 as the direction after the current debugging
work. Connects to [[project-mqtt-migration-plan]] (MQTT-as-integrator plan, expected to simplify
node/device-availability config) — the new architecture work likely subsumes or extends that plan,
though the user described it more broadly here ("new architecture" + "streamlining the DSL"), not
only MQTT-specific.

**How to apply:** don't propose or default to large speculative refactors of the current
bash-parity Go generator/DSL beyond what's needed to finish debugging it — that's still active,
separate work. When the user raises "the new architecture" or DSL redesign explicitly, expect it
to start as architecture-shaping and small experiments, not a wholesale rewrite request; match
effort accordingly (design discussion and prototypes over large committed implementations) until
they indicate otherwise.
