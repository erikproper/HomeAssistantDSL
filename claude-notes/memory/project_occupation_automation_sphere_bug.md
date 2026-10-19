---
name: project-occupation-automation-sphere-bug
description: "Latent bug in generateOccupationAutomations: the four selected_occupation automations are hardcoded to the \"physical\" sphere and would silently collide across spheres."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-08-10T13:26:50.836Z
---

`generateOccupationAutomations` in `generator.go` (~line 2701) generates four mutual-exclusion
automations per sphere that has all three `input_boolean.<sphere>/occupation/{home,around,away}`
booleans: `physical_selected_occupation_home/around/away/equalize`. The alias/id/filename of all
four are hardcoded to the literal string `"physical"` regardless of which sphere actually
triggered generation — the loop computes `sphere` but immediately discards it (`_ = sphere`).

**Why:** found during a 2026-08-10 code-review question ("what generates
selected_occupation_around?"). Not fixed yet — user explicitly asked to just note it rather than
fix immediately, since it's currently harmless: only the `social` sphere has all three occupation
booleans in both Junglinster and Vienna, so there's only ever one sphere in the loop today.

**How to apply:** if a future change gives a second sphere (e.g. `physical`, or a new sphere
entirely) its own `occupation/home|around|away` triple, this bug will manifest as silently
missing/overwritten automations — each sphere's iteration overwrites the same four files instead
of emitting per-sphere ones. Fix (not yet done): use the actual `sphere` value in the
alias/id/filename instead of the hardcoded `"physical"` string, in `buildOccupationMutexAutomation`
and `buildOccupationEqualizeAutomation`'s call sites and bodies.
