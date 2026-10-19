---
name: project-deployed-house-locations
description: "Where the live/deployed Home Assistant YAML for both houses currently lives, for byte-for-byte verification against generator output."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-08-16T21:15:48.958Z
---

As of 2026-08-16, `/Users/erikproper/SmartLiving/<House>` (e.g. `/Users/erikproper/SmartLiving/Junglinster`, `/Users/erikproper/SmartLiving/Vienna`) is the **sole, canonical location** for both houses — both the DSL source (`Definitions/`, `configure` running `homeassistant Definitions/Main.def`, i.e. this repo's Go binary) and the deployed/live generated YAML (`hass/<house>`). This repo's `New/Junglinster` and `New/Vienna` (source `Definitions/`, `configure`, `list.*`) and `New/Shared/Definitions/Macros.def`/`Settings.def` were deliberately deleted by the user on/around 2026-08-10 — this repo now only holds the Go generator source code (`*.go`), not house-specific DSL definitions.

**Why:** the two-house `New/<House>` structure in this dev repo became redundant once both houses were fully migrated to the Go generator and deployed live — the user consolidated to editing `/Users/erikproper/SmartLiving/<House>/Definitions/` directly rather than maintaining a separate copy here. (Earlier, before this consolidation, Vienna's SmartLiving directory was briefly found near-empty because it was deployed from a different on-disk checkout of this repo — that transitional state no longer applies.)

**How to apply:** to read/edit DSL source (Entities.def, Macros.def, Settings.def, etc.) for either house, go to `/Users/erikproper/SmartLiving/<House>/Definitions/` — not `New/<House>` in this repo, which no longer exists. To regenerate, `cd /Users/erikproper/SmartLiving/<House> && bash configure` (uses the `homeassistant` binary built from this repo via `go install .`). To verify generator changes, diff `/Users/erikproper/SmartLiving/<House>/hass/<house>` before/after rebuilding+regenerating. Don't assume `New/<House>` exists in this repo going forward.
