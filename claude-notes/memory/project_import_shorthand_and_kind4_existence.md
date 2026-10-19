---
name: project-import-shorthand-and-kind4-existence
description: "Cross-house import naming decoupling + shorthand grammar + kind-4 existence tracking: built+tested 2026-09-07, pushed to GitHub, and deployed live to both houses' coordinators (Junglinster + Vienna/frame) the same day -- no real declaration migrated to shorthand yet."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-07T17:36:00.211Z
---

## What changed and why

The user was unhappy having to hand-spell a remote entity's exact local entity_id in every cross-
house import declaration (analogous to how `cloud import` on a hosts device already implies its
capabilities). Design worked out iteratively, then user said **"Do both together now"** when asked
whether to build kind-4 existence tracking alongside the naming/shorthand/coercion work or as a
separate follow-up -- direct instruction overriding my own suggested "naming first, tracking later"
default.

Three parts, all landed:

1. **Export stable ID, decoupled from local naming**: `exportStableID(qualifier, deviceID,
   capability)` = `"<qualifier>_<deviceID-with-dots-as-underscores>_<capability>"`
   (`house_event_bus_coordinator/discoveryhassbridge.go`) is now the cloud `unique_id` for every
   exported hassbridge capability. Previously the cloud `unique_id` was derived from the exporter's
   own local entity_id -- fragile, since repositioning the device locally on the exporting side
   silently broke every importer with no warning.
2. **Import shorthand grammar**: `<domain>.<capability>;` (domain read for readability, discarded)
   alongside the existing explicit `<capability>: <domain>.<remote-entity-ref>;` form
   (`homeassistant/integration_import_parser.go`). Coordinator resolves a shorthand capability from
   the topic's own stable-id segment alone (`matchImportedCapabilityByStableID`,
   `discoveryimport.go`) -- **never inspects the payload's actual domain**, and always republishes
   under the *declared* domain. Domain mismatches are silently coerced, by explicit user
   requirement -- never warned about.
3. **Kind-4 existence tracking**, mirroring kind-2's `TDiscoveryExistenceTracker` exactly: only
   shorthand-declared capabilities are tracked (an explicit-ref capability predates this mechanism
   and is matched by payload content, not a stable id, so it's never tracked at all).
   `house_event_bus_coordinator/import_existence.go` (coordinator) publishes to
   `import_existence/<remoteInstallation>/existence/state`;
   `homeassistant/mqtt_import_existence.go` (generator) fetches it at generate time and hard-fails
   generation on a confirmed known-not-to-exist verdict -- wired into `Physical_Generator.go` right
   after `generateImportedDeviceFile`, gated on `hasMQTTSecrets` same as kind-2/kind-3. Three-state,
   but only two reachable today (retraction/known-not-to-exist is a coordinator-side gap not yet
   built, matching kind-2's own).

`exportStableID`'s formula is duplicated in the generator as `exportStableIDGen` (separate Go
module from the coordinator, no shared internal package) -- keep the two in sync if the formula
ever changes.

Both houses' `deploy.d/0{2,4}_deploy.coordinator` already have `--exclude import_existence.json`
added to their `rsync --delete` calls, alongside the pre-existing `entity_existence.json`/
`discovery_existence.json`/`discovery_topics.json` excludes -- this is coordinator-only runtime
state, never present in the local source tree, and every prior existence tracker got wiped by a
deploy at least once before this exclude was added for it.

## Status as of 2026-09-07 -- DEPLOYED + VERIFIED LIVE

Code-complete, `go vet`/`go test -race` clean across all three packages. Committed (86 files) and
pushed to `origin/main` on GitHub. Both real houses regenerated (`./generate`) and diffed
byte-identical against pre-change output (expected -- no real declaration uses the shorthand form
yet, so nothing changes for today's explicit-ref-only imports). Coordinator redeployed and
restarted cleanly on both:
- **Junglinster** (`protocol-server-1.junglinster`, port 1970): `deploy.d/04_deploy.coordinator`
  run directly (not the full `deploy` pipeline -- only coordinator code changed, hass/hosts/
  commandline output was already confirmed unchanged). `systemctl status coordinator` confirmed
  `active (running)`, journal showed a clean restart with no errors and normal host/cpu state
  traffic resuming immediately.
- **Vienna** (`frame`/`protocol-server-2.vienna`, port 1971 -- see
  [[project_vienna_frame_protocols_server_2_identity]]): `deploy.d/02_deploy.coordinator` run
  directly, same clean-restart verification.

**Incidental finding during the Junglinster deploy**: its source rsync (`~/go/src/homeassistant`,
`--delete`) deleted a stale `homeassistant/New/Vienna/hass/...` tree and the 16 old root-level `.go`
monolith files from the REMOTE go/src checkout -- these had never been cleaned up there even though
both were already removed locally (the `New/` directory earlier the same day, the root `.go` files
in the "clean things after a final github push" step). Also removed one stray build artifact,
`house_event_bus_coordinator/coordinator` (an old `go build -o coordinator .` binary sitting in the
source dir). All three were confirmed harmless dead weight, not live-referenced by anything -- but
worth remembering that Junglinster's `04_deploy.coordinator` mirrors the ENTIRE local repo tree with
`--delete`, so any future local cleanup will similarly scrub the remote checkout on the next deploy.
Vienna's own `02_deploy.coordinator` doesn't have this surprise potential -- its source rsync has no
`--delete` at all, only the small `coordinator/` config rsync does.

**Not yet done, deliberately**: no real Physical.def import declaration has been migrated to the
shorthand form -- propose it to the user (e.g. Junglinster's own real Vienna Netatmo imports) rather
than doing it unilaterally. PROJECT.md's own current item 1 ("Existence checking again... entities
provided on the main instance...") remains a **separate**, structurally different (kind-3-based, not
kind-2-based) future item -- not folded into this work.
