# Checkpoint — Home Assistant DSL Generator
**Frozen:** 2026-08-29 (end of session)
**Status:** generator + coordinator both build/vet/test clean, gofmt clean. Real `./generate`
against Junglinster runs clean. **PROJECT.md 1.1's core mechanism (kind-3) is deployed and
confirmed running live** — coordinator on protocols-server-1, both instances' automation trees
deployed, inquiry loop running against both main and protocols-server-2 without incident.
**PROJECT.md 1.8's kind-2 mechanism (new 2026-08-29) is built and unit-tested, NOT yet deployed to
the live coordinator.** See PROJECT.md items 1.1/1.8 and memory
`project_entity_existence_inquiry_design` for the full build log — this file is a short pointer,
not a duplicate of that detail.

**Immediate next steps queued**:
1. Deploy 1.8 (kind-2 discovery existence tracking) to protocols-server-1's coordinator, then
   confirm live against a real gateway (EMS-ESP) — trigger a discovery payload, confirm
   `[discovery-existence]` log lines and a `discovery_gateways/<gatewayID>/existence/state` publish,
   same deploy-then-verify step 1.1's kind-3 mechanism already went through.
2. Clean up PROJECT.md — item 1.1 in particular has accumulated a lot of "Status" / "Test plan" /
   "Also caught and fixed" / "Remaining" prose across a long live-debugging session and is due for
   a tidy-up pass (consolidate, drop what's now redundant, make sure DONE/remaining is accurate and
   easy to scan). Queued by the user 2026-08-28, not started yet.

---

## What landed this session (2026-08-28), in order

1. **PROJECT.md 1.1 core mechanism** — three-state entity-existence tracking (known-to-exist /
   not-known-to-exist / known-not-to-exist), paced (≤1/min/instance) coordinator inquiry loop,
   optimistic generation, device-level sibling discovery (`DiscoverSiblings`), "Discover entity"
   manual bootstrap. Built, deployed, confirmed live against both `main` and `protocols-server-2`.
   Old manifest/`entities_detailed` mechanism deleted entirely (both sides).
2. **Synthetic device grouping** — a `DiscoverSiblings`-discovered anchor with no owning DSL device
   now gets a stable synthetic `hass.discovered_<slug>` id (from the remote instance's own
   `device_attr(did, 'name')`) instead of landing ungrouped. Confirmed live (`office_garden`).
3. **Inquiry-state persistence across coordinator restarts** — `entity_existence.json`
   (coordinator-owned runtime state, mirrors `discovery_topics.json`'s pattern). Unit-tested, not
   yet exercised via a real coordinator restart in production.
4. **Physical.def overrides a synthetic name** — once any sibling of a synthetically-grouped device
   gets declared for real in Physical.def, the suggestion report's remaining entities render under
   that real device id instead of the synthetic one (`declaredDeviceIDByEntity`,
   `buildSuggestionReportFromExistence`). Confirmed live (`office_garden` → `hass.office_garden`).
5. **New warning**: a `device hass.<id> with: ...; end;` block sitting outside any
   `integration home_assistant <qualifier> with:` block used to silently vanish with zero
   feedback — now warns with a line number (`warnAboutOrphanedHassBridgeDeviceBlocks`). Found and
   fixed live (the user's own `office_garden` Physical.def placement mistake).
6. **Two real, pre-existing Spaces.def typos** found and fixed live: `hass.laserjet`'s
   capability-key lines used the raw remote entity_id instead of the local capability label, and
   its device-positioning line used `device.` (dot) instead of `device ` (space) — both silently
   broke, now fixed in the real file.
7. **New DSL sugar**: `for <device-id>: entity <spec> from entity <capability>; ...; end;`
   abbreviates repeated `entity <spec> from <device-id> entity <capability>;` lines. Purely textual,
   expanded inline in parser.go's existing single-pass loop
   (`Conceptual_DeviceCapabilityEntities.go`'s `expandForDeviceShorthandLine`). Confirmed
   byte-identical generated output vs. the old explicit form (A/B `./generate` checksum). Real
   Spaces.def converted at its three repeated-block sites (`hass.laserjet`, `hass.davids_bedroom`,
   `hass.office_corridor`).
8. Small keyword additions to `recognizedCapabilityKeywords`: `rf_strength`, `uptime` → both map to
   existing domains (`sensor.radio`, `sensor.uptime`).
9. Suggestion-report indentation fixed from 3 spaces to 2, matching the house's own Physical.def
   convention.
10. Investigated (not a bug): `host.laserjet`'s implied `_node` entity has no static
   customization/template YAML by design (`DiscoveryImplied: true` — the coordinator creates it
   live via MQTT discovery, per memory `project_discovery_over_yaml_templates`); confirmed
   `coordinator/devices.yaml` has the correct entry and the coordinator's `discovery.go` consumes it
   generically. Also confirmed the generator fully wipes+rebuilds `hass/` every run
   (`os.RemoveAll`), so stale customization files can't survive a `./generate` regardless.

All of the above: `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l` clean on both
modules; generator reinstalled (`go install .`) after every source change; real `./generate`
against Junglinster confirmed clean multiple times over the session.

New memory files this session: `project_entity_existence_inquiry_design.md` (heavily extended —
now the canonical build log for all of items 1-5 above), `project_for_device_shorthand.md` (item 6).

---

## What landed 2026-08-29

**PROJECT.md 1.8: kind-2 (discovery) passive existence tracking.** The user identified a real gap
in the previous day's Architecture.md §6.10: it correctly concluded kind-2 gateways don't need
*active inquiry* (unlike kind-3, a gateway self-announces via native HA MQTT discovery), but wrongly
generalized that into "needs no existence tracking at all" — leaving the generator with zero
existence signal for kind-2 sources, not even kind-3's old bare-assumption gap. The user's own
framing, precisely: existence checks always anchor on the physical layer's own declared *source*
specification (a capability line's right-hand side), never the conceptual-layer name; kind-1 needs
nothing (direct link), kind-2 and kind-3 both need the coordinator to report source-entity status,
kind-3 by active inquiry, kind-2 by passive observation.

Built first-phase (arrival-only, retraction deferred — see PROJECT.md 1.8's own note on why):
- Coordinator: `discovery_existence.go` (new) — `TDiscoveryExistenceTracker`, wired into
  `discoverybridge.go`'s existing discovery-payload handler (no new subscription) and `main.go`.
- Generator: `mqtt_discovery_existence.go` (new) — `fetchDiscoveryExistence`/
  `checkDiscoveryKnownNotToExistErrors`, wired into `Physical_Generator.go` right after the kind-3
  check.
- Architecture.md §6.10 corrected/rewritten; PROJECT.md 1.1's "Scope confirmation" note corrected to
  point at 1.8; memory `project_entity_existence_inquiry_design.md` extended with the full build
  log.

`go build/vet/test`, gofmt clean on both modules. Confirmed via a real `./generate` against
Junglinster: soft-fails gracefully offline (no broker, no cache yet), generation still succeeds —
same behavior kind-3 already has. **Not yet deployed to the live coordinator or exercised against a
real gateway payload** — see this file's own "Immediate next steps" at the top.

---

## Earlier sessions (2026-08-27 and before) — see PROJECT.md for the full numbered TODO list

Everything below "Phase 1" in this file's git history is stale bash-parity-era content, already
superseded. Trust `PROJECT.md` and `Architecture.md` over any further historical detail here.

## Key files

| File | Role |
|------|------|
| `generator.go` | Bulk of YAML generation steps |
| `administration.go` | Runtime state; space/entity registry; derived aggregate derivation |
| `parser.go` | Single-pass parser (since 2026-08-27); now also handles the `for` shorthand's inline expansion |
| `expander.go` | Macro expansion; `normalizeEntityFullName`; entity identity |
| `defined.go` | Installation/instance resolution; `toHomeAssistantEntityID` |
| `Conceptual_Device*.go` | The various "position/reference a device's entity" DSL constructs |
| `mqtt_entity_existence.go` | Generator-side existence status cache, suggestion-report builder |
| `remote_instance_entity_existence.go` | Generator-side inquiry automation body |
| `house_event_bus_coordinator/entity_existence.go` | Coordinator-side kind-3 three-state tracker, paced inquiry loop, persistence |
| `house_event_bus_coordinator/discover_entity_input.go` | Manual "Discover entity" bootstrap |
| `mqtt_discovery_existence.go` | Generator-side kind-2 existence status cache/check |
| `house_event_bus_coordinator/discovery_existence.go` | Coordinator-side kind-2 three-state tracker, passive (no inquiry), persistence |

## Commands

```bash
cd homeassistant && go build ./... && go vet ./... && go test ./...   # generator package
cd house_event_bus_coordinator && go build ./... && go test ./...     # coordinator package
go install .                                                          # ALWAYS before treating a real ./generate run as verification (from homeassistant/)
cd /Users/erikproper/Nextcloud/Systems/SmartLiving/Junglinster && ./generate   # real regenerate, real house
```
