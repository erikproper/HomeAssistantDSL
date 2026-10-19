---
name: discovery-yaml-truncated-deploy-incident
description: "real incident (2026-09-15) -- a deploy shipped a near-empty coordinator/discovery.yaml, silently dropping every discovery gateway back to raw Zigbee passthrough house-wide"
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-15T09:42:23.940Z
---

During Vienna's hallway aqara_windoor/door_aqara_multi migration (renaming their discovery
gateway ids to `hallway_aqara_windoor`/`hallway_door_aqara_multi`), one `./deploy` run shipped a
`coordinator/discovery.yaml` to protocols-server-1 that held only the 2-line
`generateDiscoveryPrefixBaselineFile` header (100 bytes) instead of the full 162-line file with
all 23 gateways/entity_links. The locally-generated file was verified correct via `diff`
immediately before that same deploy ran, and a fresh `./generate` afterwards reproduced the
correct 162-line file again -- ruling out a generator code bug (traced `checkDiscoveryKnownNotToExistErrors`/`generateDiscoverySuggestions` in mqtt_discovery_existence.go: both already
soft-fail on a fetch error with no cache, `continue` rather than aborting the caller, confirmed by
reading the code).

Effect: `matchingGateway` (discoverybridge.go) had an empty `Gateways` map, so *every* Zigbee leaf
across the whole house fell back to raw, unqualified passthrough relay for ~8 minutes -- the exact
same "greyed out" failure mode this whole migration project started from. Discovered by noticing
`[discovery-bridge] queued passthrough relay ...` log lines for leaves that should have logged
`queued relay of <entity>` instead, then confirming `wc -l ~/coordinator/discovery.yaml` on the
remote showed 100 bytes.

Root cause not conclusively pinned down (the failure didn't reproduce on a clean regenerate +
redeploy done immediately after). Leading theory: a stray backgrounded `./generate` invocation
from much earlier in the same session (one that had timed out waiting on unreachable local-network
DNS lookups and was moved to background) was possibly still alive and raced a later foreground
`./generate`, overwriting the file with its own stale/early output right as the later `./deploy`'s
rsync read it. Not proven (no stray process was still running when checked afterward).

**Why:** whatever the exact trigger, deploy scripts that rsync generator output currently trust
"the local file is correct because generate just ran" with zero verification on the remote side.

**How to apply:** after any `./deploy` that touches `coordinator/discovery.yaml` (or likely any
other coordinator/*.yaml), verify the REMOTE file's line count / gateway count directly
(`ssh ... wc -l ~/coordinator/discovery.yaml`, `grep -c 'gateway:' ...`) before trusting the
"queued relay of ..." log lines -- don't stop at "coordinator restarted with no errors," since a
near-empty config file loads and runs cleanly, it just silently does the wrong thing. If a stray
background `./generate` is ever intentionally left running again, kill it before running further
foreground generate/deploy cycles in the same session.

See also [[project_retiremissing_passthrough_wipe_incident]] (a different but related
passthrough-adjacent incident from the same migration effort) and
[[project_discovery_device_block_and_node_capability]].
