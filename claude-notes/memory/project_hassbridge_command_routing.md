---
name: project-hassbridge-command-routing
description: "Generic hassbridge command routing (Phase 1) built + verified live 2026-09-09; Vienna's MQTT migration now complete; Phase 2 (cross-house) still deferred. Derived-capability work (item 4) has since UNFROZEN and progressed -- see [[project_derived_capability_mechanism]]."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-10T14:44:56.325Z
---

**Generic command routing for hassbridge entities — Phase 1 DONE + VERIFIED LIVE (2026-09-09).**
`homeassistant/hassbridge_commands.go` (domain -> commands/discovery_extra tables, switch+vacuum
today), `homeassistant/remote_instance_entity_commands.go` (per-entity-per-command automations,
PROJECT.md item 11's naming convention), `house_event_bus_coordinator/discoveryhassbridge.go`
(command_topic + payload_* wiring). Same-broker only; coordinator stays fully domain-agnostic.
Also added: `domainStatePayloadTemplate` (per-domain state JSON wrapping — vacuum needs
`{"state": ...}`, most domains stay atomic) and `THassBridgeCapability.ValueMap` (per-DEVICE, not
domain-generic, native-vocabulary-to-HA translation, e.g. Roomba's "home" -> "docked"). Documented
in Architecture.md §6.13 and PROJECT.md item 11 (resolved) / item 23 (future generalization —
reversible topic/JSON tree rewriting, MSc thesis reference:
https://repositum.tuwien.at/handle/20.500.12708/220349).

**Why:** the user's Roomba vacuum, newly relayed via Vienna's `protocols-server-2` secondary HA
instance, needed real command control (start/pause/stop/return_to_base/locate/clean_spot), not
just read-only state — hassbridge had been one-way (state only) until this.

**How to apply:** a hassbridge capability's domain automatically gets commands the moment
`domainCommands` has an entry for it — no new Physical.def syntax needed to enable this. Adding a
future domain (e.g. `cover` for Junglinster's Overkiz/SOMFY covers, PROJECT.md item 11) is one new
table entry. Phase 2 (routing a command across the `export`/import cloud-broker link) is
explicitly NOT built — deferred, same-house was the forcing case.

**Vienna's MQTT migration is now complete** (2026-09-09, per the user): the vacuum was the last
physical device needing migration to the MQTT-based architecture. Junglinster is untouched by any
of today's work.

**Two follow-on generator fixes, same day (2026-09-10), triggered by splitting the picture frame
off host.frame's shared identity into its own `appliance.picture_frame` commandline device:**
1. `registerDevicePositioning` (Conceptual_DevicePositioning.go) never recognised a standalone
   "commandline" device as a valid `device <spec> from <device-id> with: ...;` target — only
   hosts/hassbridge/import. Fixed via a new, deliberately minimal `registerCommandlineDevicePositioning`
   (no ConstantAttributes, no node-capability concept — a commandline device's liveness is always
   the coordinator's own unconditional `binary_sensor.<host>_commandline_node`, untouched by
   positioning).
2. `namingSpacePath`'s "device declaration acts like a space" sugar (item 5) doubles the device's
   own leaf path whenever an entity's local label equals it — e.g. `vacuum.social:vacuum` under
   `device ... from appliance.vacuum` silently produced `..._vacuum_vacuum`. Confirmed the DSL
   convention to avoid this, not a code fix: write the local spec with an EMPTY path after the
   colon (`entity vacuum.social: from roomba;` / `entity switch.social: from slideshow;`) —
   `normalizeEntityFullName` already resolves an empty path to exactly the enclosing space context,
   with no doubling. Applies to any future device whose "primary" entity would otherwise repeat
   the device's own name.

**Superseded (2026-09-10)**: the "frozen for budget" note below no longer applies — item 4 was
unfrozen and fully built out the same day. See [[project_derived_capability_mechanism]] for current
status; this file's own scope is command routing only from here on.
