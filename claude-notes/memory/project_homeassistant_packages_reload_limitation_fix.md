---
name: homeassistant-packages-reload-limitation-fix
description: "DONE+DEPLOYED 2026-09-23 - removed HA \"packages:\" wrapper from generated configuration.yaml on both houses; packages are never reprocessed by any HA reload service (upstream wontfix), which made homeassistant.reload_all a no-op for this project's entire generated config"
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-23T10:27:55.557Z
---

Real, high-impact finding, 2026-09-22/23: both houses' generated `configuration.yaml` routed EVERY
domain (template, automation, binary_sensor, sensor, switch, script, scene, the input_* helpers,
tts) through `homeassistant: packages: !include_dir_named integrations`. Home Assistant has a
long-standing, upstream **wontfix** limitation — packages are never reprocessed by any reload
service: not `<domain>.reload`, not `homeassistant.reload_all` (just a loop over those same
per-domain hooks), not even `homeassistant.reload_core_config`
(github.com/home-assistant/core#12069, closed wontfix). Only a full restart re-runs the package
merge. This meant this project's whole meta-reload mechanism (`PublishMetaReload` ->
`homeassistant.reload_all`, built specifically so a deploy doesn't need a restart) was
architecturally a no-op for the ENTIRE generated native-instance config — not a narrow "misses new
files" bug. This reframed three separate incidents (2026-09-10, 2026-09-11, 2026-09-22) as the same
root cause. See also [[project_junglinster_coordinator_crash_loop_2026_09_22]] for the unrelated
coordinator crash loop found the same day (different component, same session).

**Fix**: generator.go's `integrationDefs`/`generateConfigurationFile`, plus
`remote_instance_automations.go`'s remote-instance skeleton, reworked to drop `packages:` entirely.
Confirmed via direct code read (an Explore agent audit) that every per-domain
`integrations/<domain>.yaml` file was ALWAYS a single `!include*` line for exactly one domain — the
package-grouping capability (multiple domains merged from one file) was never actually exercised
anywhere in either house's DSL or generated output, so nothing was lost by splitting. Each domain is
now a plain top-level key in `configuration.yaml` (`<domain>: !include integrations/<domain>.yaml`)
— standard, well-supported HA config-splitting that DOES get properly reprocessed by
`<domain>.reload`/`reload_all`. Per-domain `integrations/*.yaml` files changed shape too (content is
now just the bare `!include_dir_*` tag, no `<domain>: ` prefix — the domain key itself now lives in
configuration.yaml, which nests the file's own value under it via `!include`).

**Deploy note, important for next time**: this specific structural change to `configuration.yaml`
itself needs a ONE-TIME full restart to take effect (the same packages limitation being fixed —
ironic but unavoidable, since restructuring *which* keys exist at all is itself a config reload
can't discover). Deployed+restarted all four live instances 2026-09-23 (Vienna main, Vienna
ha2mqtt, Junglinster main, Junglinster ha2mqtt) via the existing `meta/restart/<name>/request`
cloud-relay topic (no `PublishMetaRestart` helper existed in the deploy scripts yet — triggered
manually via `mosquitto_pub` mirroring `PublishMetaReload`'s own cloud-broker pattern). Each
verified healthy after: `curl localhost:8123/` returns HTTP 200, zero config/setup errors in the
log, entity registry count intact. **Verification lesson**: don't poll for new log lines as a
"did it finish starting" signal — a healthy, fully-started instance can go quiet for a long stretch
during normal steady-state operation (a live incident during this exact deploy: Vienna's ha2mqtt
looked "stuck" for ~10 minutes because nothing new was being logged, causing real concern, when it
had actually finished starting within ~30s and was just idling normally). Check the HTTP frontend
directly instead.

**Unrelated bug found and fixed the same session, while investigating this**: several Fedora-on-a-
stick hosts (protocols-server-1 @ Vienna, frame @ Vienna, protocols-server-1 @ Junglinster) were set
to `America/New_York` instead of their real timezone (`Europe/Vienna`/`Europe/Luxembourg`) — traced
to the stick-cloning workflow (PROJECT.md item 2) carrying over the source image's timezone rather
than each clone getting its own. Fixed live via `timedatectl set-timezone`; also flagged in
PROJECT.md item 2 that the two not-yet-deployed cloned sticks need the same fix before going live.
