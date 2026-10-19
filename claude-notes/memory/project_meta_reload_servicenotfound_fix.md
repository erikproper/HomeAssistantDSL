---
name: project-meta-reload-servicenotfound-fix
description: "DONE + DEPLOYED (2026-09-15): fixed a real, systemic bug where continue_on_error never actually protected the coordinator's per-instance 'reload' automation from an optional domain's ServiceNotFound, silently truncating the whole reload before automation.reload ever ran. Read this before touching remote_instance_meta_control.go or debugging a deploy whose reload trigger doesn't seem to take effect."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-15T16:55:33.427Z
---

## What was found

Vienna's `node.fritz_box` (hassbridge device on protocols-server-2) had two independent issues
surface in the same debugging session:

1. Two capabilities (`fritz_os`, `reconnect`) referenced stale AVM Fritz!Box entity_ids that no
   longer existed on protocols-server-2 (a real upstream entity rename) — this blocked
   `./generate` outright via the coordinator's own confirmed-not-to-exist check. User fixed the
   entity names directly in Physical.def.
2. Separately, `sensor.infrastructural_apartment_living_room_fritz_box_gb_received` showed
   "unavailable" in HA while its own value clearly kept updating. Root cause: a hassbridge
   capability's own state_topic and its device's shared availability gate
   (`hassBridgeAvailabilityTopic`/`buildAvailabilityFields`, discoveryhassbridge.go) are two
   INDEPENDENT MQTT topics — the state can keep refreshing fine while the device's own `node`
   topic is stuck on a stale value, showing every sibling capability unavailable regardless.

Chasing why `node`'s own stale value never refreshed after a deploy+reload led to the real,
systemic bug: **HA's script engine explicitly RE-RAISES `ServiceNotFound` (along with
`vol.Invalid`, `TemplateError`, `InvalidEntityFormatError`, `NoEntitySpecifiedError`,
`ConditionError`) regardless of `continue_on_error`** — confirmed via HA's own `script.py` source
(these exceptions are treated as authoring mistakes, not runtime failures continue_on_error is
meant to shrug off). The coordinator's own `metaReloadAutomationBody`
(`homeassistant/remote_instance_meta_control.go`) called every optional `<domain>.reload` service
as sequential steps in ONE script with `automation.reload` last — the templating trick from an
earlier fix (2026-09-06) only avoided HA's LOAD-time static validation, but a genuinely-missing
optional domain (e.g. protocols-server-2 has no "template" integration configured) still aborted
the WHOLE script at that step, RUN-time, every single deploy, silently never reaching
`automation.reload` at the end. The freshly-deployed, corrected automation files were therefore
never actually loaded — only a full `homeassistant.restart` (which reads all YAML fresh,
bypassing "reload" entirely) picks up a fix on an instance missing any optional domain.

No Jinja-level "does this service exist" check exists in HA to guard against this instead
(confirmed via web search — no `has_service`/`is_service_available` template function).

## The fix

`metaReloadAutomationBody` now generates SEVERAL independent automations sharing one MQTT trigger
topic, instead of one big sequential script:
1. One "core" automation: `metaReloadServices` (currently just `script.reload`, always safe) +
   `homeassistant.reload_core_config`.
2. One SEPARATE automation per `metaOptionalReloadServices` entry — isolates each domain's own
   possible `ServiceNotFound` to itself alone; nothing else triggered by the same topic is
   affected.
3. One "reload automations" automation: a 5s delay, then `automation.reload` — kept last in the
   generated file purely for readability now (not a correctness requirement, since each automation
   is independent); the delay is what protects sibling automations from `automation.reload`'s own
   well-known self-cancellation (real bug, 2026-09-08, still documented in the file) reaching them
   mid-execution.

Covered by new tests in `remote_instance_meta_control_test.go`
(`TestMetaReloadAutomationBodyIsolatesEachOptionalDomain`,
`TestMetaReloadAutomationBodyDelaysAutomationReload`); all pre-existing tests still pass unchanged.

Verified live on Vienna's protocols-server-2, 2026-09-15: a post-fix reload trigger produced THREE
independent, isolated `ServiceNotFound` errors (group/command_line/template, each domain missing
there) with no monolithic abort — `scene`/`input_*` reloaded silently and successfully, and
`node.fritz_box`'s stale availability topic recovered to `on` once a full restart picked up both
this fix and the corrected entity names together.

## Operational note

`homeassistant.restart` (triggered via the coordinator's `meta/restart/<name>/request` topic,
cloud-relayed) took roughly **8 minutes** between the MQTT publish and the actual process exit
(`[HH:MM:SS] INFO: Home Assistant Core finish process exit code 100` in journalctl) on
protocols-server-2 during this session — don't assume a restart failed just because nothing
happened in the first minute or two; wait for that exact log line before concluding it didn't
fire. `systemctl status`/`ActiveEnterTimestamp` does NOT change on this kind of restart (it's an
in-process HA Core restart under a supervisor, not a systemd-level one) — check journalctl content
(the exit-code-100 line, or fresh "custom integration ... not tested" boot warnings) instead.

Deploying to Vienna's protocols-server-2 uses SSH port 1971 (`root@vienna.homelinux.org`); "main"
uses port 1967 as user `hassio` (a restricted HAOS shell — `ha` CLI needs Supervisor auth not
available there, so journalctl/status checks don't work for main the way they do for
protocols-server-2; verify main's health indirectly via the coordinator's own journal activity for
`[hosts:main]`/`[existence] main:` lines instead).

This same fix applies to Junglinster too (shared Go code) but was not deployed there today — out
of scope, no known live symptom there yet.
