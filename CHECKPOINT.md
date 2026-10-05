# Checkpoint — Home Assistant DSL Generator + Coordinator

**Frozen:** 2026-09-29 (mid-session, ahead of a possible `/clear`)
**Status:** both Go modules (`homeassistant` generator, `house_event_bus_coordinator`) build/vet/
test clean. Both houses (Vienna, Junglinster) regenerate clean. **Coordinator is deployed and live
on both houses' protocols-server-1 as of this session**, carrying today's two coordinator-side name
fixes (see below). The generator-side whitespace/continuation-join fix is built, tested, and
regenerates clean on both houses, but **has not been deployed** (no `.def`/generated-YAML change
resulted from it on either house, so there's nothing new to push — see below for why).

This file is a short pointer, not a duplicate. The full, extremely detailed build log for
everything below (and for the whole multi-week Junglinster migration this session is deep inside)
lives in the plan file: **`/Users/erikproper/.claude/plans/splendid-zooming-lighthouse.md`** — read
that first for real continuation, this file only orients you to where things stand *right now* and
what's still open.

**Git note**: both Go modules and both houses' `Definitions/` repos currently carry a large
uncommitted changeset (this whole session's work, plus earlier accumulated work never committed).
Don't assume a clean git state, and don't discard any of it — `git status`/`git diff` are the
authoritative record of what's changed; nothing here has been committed yet.

---

## What landed THIS session (2026-09-29), in order

1. **Hassbridge/discovery-relay MQTT "name" field bug — found, fixed, deployed, live-verified.**
   Two entities (`cover.social_house_kitchen`, `switch.social_house_kitchen_workplace_furnace`)
   showed garbled doubled `friendly_name`s (e.g. "house/kitchen house/kitchen/cover"). Root cause:
   neither `discoverybridge.go`'s `buildRelayedDiscoveryConfig` (raw Zigbee2MQTT/EMS-ESP relay) nor
   `discoveryhassbridge.go`'s `buildHassBridgeEntityDiscoveryBody` (hassbridge relay) used the
   `Name *string`-nil-means-"device name alone" pattern `TBinarySensorDiscoveryPayload` already
   established (discovery.go) — both wrote a non-null "name" HA then combined with device.name.
   Fixed in both files: a new `soleGatewayCapability` distinction (raw relay — null only when the
   gateway relays exactly one capability, otherwise the gateway's own native per-entity label is
   left untouched, since many gateways — Aqara multi-sensors, EMS-ESP — genuinely relay several
   capabilities from one device) and a DisplaySuffix-empty→null / DisplaySuffix-non-empty→suffix-
   alone fix (hassbridge). 4 new/updated tests, both modules green. **Deployed to both houses'
   coordinators and confirmed live.**
2. **Two live registry collisions found and fixed** (same class hit repeatedly all session — a
   stale/orphaned registry entry squatting a clean entity_id, forcing the real one to `_2`):
   - Furnace switch: an old, non-tombstoned template-platform orphan (from before this session's
     `inverted` mechanism replaced the old two-entity "flipped" wrapper) — deleted, `_2` renamed.
   - **Kitchen corridor's `corb` light** (`light.social_house_corridor_corb`) — found live TODAY,
     reported by the user ("z2m agrees it IS available" while HA showed unavailable/restored). Root
     cause: an old *raw* native Zigbee2MQTT registration (predating corb's 2026-09-28 migration to
     the coordinator relay) never got cleaned up at the time, leaving the real relayed entity stuck
     at `_2`. Fixed via the established registry-surgery playbook (backup, `ha core stop`, remove
     orphan + rename `_2`→clean, `ha core start`, verified live: now reports `on`/`off` correctly).
     **Confirmed this was a pre-existing gap from 2026-09-28's own deploy verification (which
     checked 3 other devices but missed corb), not caused by anything this session did.**
3. **DSL parser fix: a wrapped one-liner "with" clause is now joined back into one logical line**
   (`defined.go`, `joinWrappedStatementContinuations` + `normalizeDefinitionWhitespace`). Real bug:
   an editor auto-wrapped `device X with <statement>;` right after the bare word "with" (Junglinster
   Conceptual.def:517), which neither the block-header nor one-liner regex recognised — the
   device's positioning silently vanished from any *future* regeneration (the live-deployed entity
   from before the wrap kept working, which is why nothing looked broken in HA). Fixed narrowly: a
   line ending in the standalone word "with" (not "with:") gets joined, single-space, to whatever
   follows, until a real terminator (`;`/`:`) or a blank line. **Real regression caught before
   shipping**: the first version didn't exempt comment lines, and a `# ... with` comment two lines
   above a real `device ... with:` block header swallowed that header into itself, breaking the
   block (Physical.def's aqara_magnet device, 5 "unrecognised line" warnings) — fixed by excluding
   comment lines from ever triggering or being swallowed by the join. Both houses regenerate clean
   with zero warnings now. **Confirmed NOT a live bug** — byte-diffed clean against the currently-
   deployed state on Junglinster (the one affected line was already correctly deployed back on
   2026-09-28, before whatever edit wrapped it — so nothing to redeploy).
4. Investigated and **ruled out** a suspected gap: `light.social_house_kitchen_main`/
   `light.social_house_wc_main` (node 057's two endpoints) looked "missing" because I searched the
   wrong output tree (`hass/`, for YAML-template entities) — they're discovery-kind (MQTT relay)
   entities, correctly present in `coordinator/discovery.yaml`, confirmed live and matching the
   registry exactly. No bug; my own investigation mistake, corrected.
5. **Coordinator deployed to both houses** (Junglinster + Vienna protocols-server-1), carrying item
   1's fix. Verified via `systemctl is-active coordinator` + fresh binary timestamp on both.

## Immediate next steps queued

1. **Update the plan file** (`splendid-zooming-lighthouse.md`) with everything in this checkpoint —
   not yet done as of this freeze (the plan file's own last entry is 2026-09-28).
2. Resume **Phase 3** of the plan: a genuine line-by-line read of Junglinster's Conceptual.def from
   the "#### HERE" marker (~line 391) onward — not a keyword grep, per this session's own lesson
   about missing the old flat "has_state" macro shape that way once already.
3. `timer.social:removing_smell` (`definition as timer "00:03:00"; limits ... fan.social: off on;`)
   — flagged by the user as the likely next migration target, explicitly needing its own planning
   pass before starting (not begun).
4. PROJECT.md lines ~13-19 (surfaced via an IDE selection, not yet actioned): a note about "turn
   XXX" switcher on/off ordering and a possible `follows` simplification into two when-if-then
   rules for the `corb`-style switched-light case. Relevance to current work unconfirmed.
5. The plan's own still-open items: 7 of 13 with-clause consumers left unmigrated (deliberately
   deferred, low priority); Phase 1's two cross-checks were already closed out; Phase 4 (PROJECT.md's
   backlog) waits behind Phase 3.

## Key files (unchanged from before, still accurate)

| File | Role |
|------|------|
| `homeassistant/generator.go` | Bulk of YAML generation steps |
| `homeassistant/administration.go` | Runtime state; space/entity registry; derived aggregate derivation |
| `homeassistant/parser.go` | Single-pass parser |
| `homeassistant/expander.go` | Macro expansion; `normalizeEntityFullName`; entity identity |
| `homeassistant/defined.go` | Installation/instance resolution; `toHomeAssistantEntityID`; **new: `normalizeDefinitionWhitespace`/`joinWrappedStatementContinuations`** |
| `homeassistant/Conceptual_Device*.go` | The various "position/reference a device's entity" DSL constructs |
| `homeassistant/integration_logical_*.go` | Logical-layer `enabled by`/`is switched by`/`dependency on` mechanisms |
| `homeassistant/logical_enabler_dependency_sugar.go` | `enabled by`/`dependency on` desugaring |
| `homeassistant/logical_switched_by.go` | `is switched by` desugaring |
| `house_event_bus_coordinator/discoverybridge.go` | Raw Zigbee2MQTT/EMS-ESP discovery relay; **today's `soleGatewayCapability`/name fix** |
| `house_event_bus_coordinator/discoveryhassbridge.go` | Hassbridge (remote-HA) relay; **today's DisplaySuffix/name fix** |
| `house_event_bus_coordinator/discovery.go` | `TBinarySensorDiscoveryPayload`/`TSensorDiscoveryPayload` — the original `Name *string`-null pattern both of today's fixes now match |

## Commands

```bash
cd homeassistant && go build ./... && go vet ./... && go test ./...              # generator module
cd house_event_bus_coordinator && go build ./... && go vet ./... && go test ./...  # coordinator module
go install .                                                                      # ALWAYS before trusting a real ./generate run (from homeassistant/)
cd /Users/erikproper/SmartLiving/Junglinster && ./generate                       # real regenerate, real house
cd /Users/erikproper/SmartLiving/Vienna && ./generate                           # real regenerate, real house
```

Deploy (both houses): `./deploy` sources `deploy.d/01..05` in order; `04_deploy.coordinator`
routinely exceeds a foreground timeout — run it in background and verify via
`systemctl is-active coordinator` + `/usr/local/bin/coordinator` timestamp on protocols-server-1
(port 1970 SSH, `erikp` user). HA Core restart/stop for registry surgery:
`SUPERVISOR_TOKEN=$(cat /run/s6/container_environment/SUPERVISOR_TOKEN) ha core start|stop|restart`
(port 1967 SSH, `hassio` user — that user's own `sudo -n` works for `/config/.storage/*` edits,
plain `cp` does not).
