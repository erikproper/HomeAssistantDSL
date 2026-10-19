---
name: project-meta-reload-insufficient-after-new-automations
description: "Found live 2026-09-21: PublishMetaReload's automation.reload didn't cause frame@Vienna's bridged entities to republish after a deploy added/changed automation files -- only a full container restart did. All bridged entities sat on stale retained MQTT data for hours, invisible because HA's frontend still showed the old values."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-21T14:07:21.053Z
---

After deploying Physical.def changes (the frame rename, then the new vacuum attribute-exposure
feature) to frame@Vienna's HA instance (now on protocols-server-1) and triggering
`PublishMetaReload frame` as part of the normal `./deploy` pipeline, NOTHING published fresh MQTT
state for ANY bridged capability on that instance for hours -- not just the newly-added
attributes automation, but the pre-existing fritz_box/washing_machine/sun/vacuum-state ones too.
Confirmed via direct MQTT inspection: `homeassistant_instances/frame/bridge/+/state` was
completely silent, while the entity registry showed the automations as loaded/"on" (i.e. enabled),
and the HA log had no errors at all -- silence looked like health.

The user's own main HA frontend never flagged anything wrong, because it just kept showing the
LAST RETAINED (stale, pre-deploy) values, which looked plausible. Only became visible because this
session was actively checking the underlying MQTT topics for a specific reason (verifying the new
attribute feature).

**Fix that worked**: `sudo podman restart homeassistant` (a full container restart, not just an
automation reload) immediately caused every bridged entity to republish fresh, correct data.

**Root cause, not fully diagnosed**: automation.reload (triggered via the meta-reload mechanism,
[[project_meta_reload_all_simplification]]) is supposed to fire each automation's own
"event: automation_reloaded" trigger, which every reporting automation in this codebase includes
specifically so a plain reload republishes current state without needing a full restart (see
homeassistant/remote_instance_entity_reporting.go's entityReportingAutomationBody doc comment,
2026-08-29 regression test). That mechanism apparently didn't fire reliably this time -- not
investigated further this session (out of scope for the attribute-exposure feature being verified),
but worth real investigation if it recurs.

**Why:** two back-to-back deploys that day (the frame rename, then the attribute feature) both
relied on the meta-reload path to make the change visible -- neither actually took effect live
until a manual restart, several hours apart from either deploy.

**How to apply:** after deploying a change to a `home_assistant`/hassbridge-kind instance's own
automations, don't trust the meta-reload/`PublishMetaReload` trigger alone to confirm the deploy
actually took effect -- directly check the relevant `homeassistant_instances/<qualifier>/bridge/...`
MQTT topics for fresh (not stale-retained) data before declaring success. If stale, a full
`podman restart <container>` on that instance reliably fixes it. Worth a proper root-cause dig
(is `automation_reloaded` genuinely not firing, or is something else swallowing it) if this
happens again -- currently just a known workaround, not a understood-and-fixed bug.
