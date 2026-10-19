---
name: project-device-display-name-cleanup
description: "2026-08-29 -- dropped the \"infrastructural\" sphere segment from coordinator-published device display names; safe, purely cosmetic, one-time house-wide discovery republish."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-08-29T19:51:36.883Z
---

The user asked me to assess, then implement, dropping "infrastructural" from the front of device
display names as the coordinator publishes them on MQTT discovery topics -- it's the sphere nearly
every device resolves to, so spelling it out on every device name added visual noise without
conveying anything (a "battery"/"connectivity"/"node" device is *always* infrastructural; the
sphere only becomes informative for the rarer social/physical device).

**Why safe, verified before touching anything**: HA's device registry keys identity strictly off
`device.identifiers` (the DSL's own internal device id, e.g. `host.laserjet`), never off
`device.name` -- confirmed via `house_event_bus_coordinator/discovery.go`'s `buildDiscoveryConfigs`,
where `Identifiers`/`Name` are already fully separate fields and `Identifiers` never contains a
sphere segment at all. Entity identity/entity_id are likewise independent of device display name
(this codebase deliberately never uses HA's `has_entity_name` device-name templating -- every
entity_id is explicit via `default_entity_id`). So no new devices, no orphaned entities, no broken
automations/dashboards/history -- all of those are keyed by entity_id or `unique_id`, never by
device display name.

**One real, harmless consequence**: `DisplayName` is embedded in *every* entity's own discovery
payload for that device (MQTT discovery has no separate device-only topic). Changing it triggers a
one-time, house-wide retire+republish of nearly every entity's discovery config the next time the
coordinator processes it (`TDiscoveryPublisher.Publish`'s content-aware retire-then-republish) --
transient, cosmetic, not data loss. A device the user manually renamed in the HA UI keeps that
override regardless (HA lets a user override win).

**Implementation**: `Conceptual_DeviceEntities.go`'s `deviceDisplayName(spaceName, sphere, path)`
now omits the sphere segment entirely when `sphere == "infrastructural"`, returning just
`<space-path>/<device-path>` (e.g. `"garage/smarty"` instead of `"infrastructural/garage/smarty"`);
every other sphere (`social`, `physical`) is unaffected. Same function also drives `suggested_area`
(`administration.go`'s `CurrentArea()`), so an *auto-derived* suggested_area
(from an "as area" space's own sphere/path) loses the prefix too -- an *explicit* literal
`suggested_area` override declared in Physical.def is untouched (never goes through
`deviceDisplayName`). A separate, older mechanism in `generator.go` (customization YAML
friendly-names for generator-authored template entities, not the coordinator's MQTT discovery
device blocks) also uses `"infrastructural/"`-prefixed strings but was deliberately left alone --
out of scope, a different naming path entirely.

**Verified live**: `devices.yaml`/`homeassistant_bridge.yaml` display names now read e.g.
`house/storage_room/backups`, `house/office/eriks-mac-studio` -- other spheres
(`social/roof/solar_panels`) untouched. Four existing tests updated to match the new expected
values (`TestDeviceEntityImpliesDiscoveryEntitiesWithoutSpacePrefix`,
`TestDeviceEntityBareSpecDefaultsToInfrastructuralSphereAndSpaceRelativePath`,
`TestSpaceAsAreaAssignsSuggestedAreaToDevices`, `TestSpaceAsAreaShadowingAndExplicitOverride`).
Generator reinstalled, real `./generate` against Junglinster confirmed clean.

**Not yet done**: deploy + the one-time house-wide discovery republish this will trigger, plus the
user's own planned manual pass to fix up any device names HA doesn't auto-update the way expected
(their own words: "will then test the resulting renames and manually correct where needed").
