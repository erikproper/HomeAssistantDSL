---
name: project-logical-layer-device-combining
description: "Logical layer's two ways to combine physical-layer devices: aggregate (form something new, not yet built) vs. absorb (subordinate into an existing master device — DONE+DEPLOYED 2026-09-18, Vienna's washing_machine/ROBB plug) — plus a follow-on note that combining devices will need a more refined, chained view of availability (compute-host liveness, Netatmo base-module IP link) than the physical layer's own two-factor node+self AND."
metadata:
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-18T06:01:51.185Z
---

Noted 2026-08-22, once Z-Wave and Zigbee2MQTT both speak MQTT (PROJECT.md Phase 3 steps 3-4 —
neither started yet; this is explicitly future work, not something to build now). Refines
Architecture.md §2's existing "devices may be aggregated from other devices" sentence into two
named operations (now reflected in Architecture.md §2 directly):

- **Aggregate**: combine multiple devices into a genuinely *new* logical device that isn't really
  either constituent alone. Architecture.md's own pre-existing example: an external temperature
  sensor merged into a smart plug to form one logical "heater" device. Third worked example, noted
  2026-08-24: a HASSOS-based compute host (e.g. junglinster itself). Its basic ping-based liveness
  works like any host, but CPU load/temp is ONLY obtainable via HA's own "Home Assistant Operating
  System" integration running inside that HA instance (HASSOS is a locked-down appliance OS — no
  running a separate `cpu/report` script on it, unlike a normal Linux host) — i.e. a
  `home_assistant`-kind device (see the "intensional device→space linkage" plan,
  splendid-zooming-lighthouse.md, its D section on the unified device grammar), not a `hosts`-kind
  one. So "the compute host" for a HASSOS box must logically aggregate a `host.<id>` (ping) device
  with a `home_assistant`-kind device providing load/temp — neither alone represents "is this
  physical machine alive and how loaded is it" on its own. Directly relates to the still-unbuilt
  §2e/"stacked dependence" liveness reminder in that same plan (device → integration/HA-instance →
  compute host propagation).
- **Absorb**: one device becomes subordinate to, and extends, an existing "master" device, rather
  than forming something new. Two worked examples from the user:
  1. `switch.infrastructural:garage/smarty` (a Zigbee-controlled power switch for the `smarty`
     host) should become logically merged into `host.smarty` as its on/off switch capability,
     rather than existing as its own separate HA device.
  2. The common Z-Wave/Zigbee pairing: a Z-Wave-based main wall switch (linked to a physical wall
     switch) absorbing the Zigbee-based dimmer light(s) it actually controls, forming one logical
     light — a very common pattern across this house's real device inventory, not a one-off case.

**Why:** part of the same "logical layer" gap Architecture.md §2 already flagged as *(planned —
see §10.4)* with no current DSL file backing it (`Entities.def`/conceptual and
`Integrations.def`/physical both exist in some form; there's no logical-layer DSL file yet). The
`smarty` case is a direct, concrete near-term motivator once `switch.infrastructural:garage/smarty`
exists as a real MQTT-discovered entity (see [[project-mqtt-migration-plan]]) sitting awkwardly
next to `host.smarty`'s own discovery-implied node/cpu entities as an unrelated separate HA device.

**How to apply:** don't design or build either operation speculatively. When Z-Wave/Zigbee2MQTT
migration actually starts (Phase 3 steps 3-4), this note is the starting point for that
conversation — bring back both named operations and the two worked examples. No DSL grammar,
generator shape, or coordinator behavior has been decided for either "aggregate" or "absorb" yet.

**Absorb: DONE+DEPLOYED 2026-09-18, first real case.** Vienna's shower_room washing machine:
`appliance.washing_machine` (hassbridge, cross-posted from protocols-server-2's native
integration) absorbed its ROBB smart plug's (`discovery.washing_machine_switch`)
switch/power/energy/current/consumes as its OWN Logical.def capabilities. New grammar: a
Logical.def device's own `capabilities from <device-id>: <domain>.<label>: <bare-capability>; ...
end;` block (`integration_logical_parser.go`'s `logicalAbsorbBlockPattern`,
`TLogicalCapability.AbsorbedFromDeviceID`/`AbsorbedFromCapability`,
`registerLogicalAbsorbedCapability` in `Conceptual_LogicalEntities.go` delegates straight to the
absorbed device's own normal registration path, attributed under the HOST device's own leaf). Two
real implementation gaps found and fixed along the way:
1. Hassbridge's own capability dispatch (`registerDeviceCapabilityEntityLinkAllowingHidden`,
   `Conceptual_DeviceCapabilityEntities.go`) errored immediately on a capability miss instead of
   falling through to the SAME device id's Logical.def overlay — fixed with a `dispatchLogical
   Capability` helper shared by both the pure-logical branch and this new hassbridge fallback.
2. `registerDevicePositioning`'s "does this device id have real Logical.def capabilities" guard
   (`Conceptual_DevicePositioning.go`) would have intercepted the device's own positioning line
   entirely once its absorb block gave it non-empty Capabilities — silently dropping its real
   hassbridge constantAttrs/suggested_area/node registration. Fixed by requiring NO physical
   presence (hassbridge/discovery/hosts/imported/commandline) before the pure-logical branch may
   claim a device id.
Enabler design note (genuinely new constraint, not anticipated in the original plan): a hassbridge
device's own "node" is a Jinja expression evaluated on the REMOTE source instance and published to
MQTT already-computed — MAIN never evaluates it locally, so a local Zigbee2MQTT enabler switch
can't be folded into that same expression the way a purely-local Logical.def enabler (appliance.tv)
can. Solved by leaving the raw node exactly as-is (unconditional, unchanged entity_id) and adding a
SEPARATE wrapping `binary_sensor.<label>: <raw-node-entity> is available with: enabler <literal
entity_id>; end;` capability — no new grammar needed, just an ordinary IsAvailable capability whose
Entity is the host's own already-known raw node entity_id. Full writeup: Vienna's
`MigrationNotes.def`, "appliance.washing_machine (shower_room), 2026-09-18" entry.

**Availability composition, added 2026-08-31.** Once the physical-layer coordinator work landed
node-gated + self-check availability for hassbridge/import entities (`buildAvailabilityFields`,
`house_event_bus_coordinator/discovery.go` — an entity is available iff its own last-reported
value isn't literally "unavailable" AND its device's node/connectivity entity says so), the user
flagged that this two-factor AND is only right for a single physical-layer device — aggregate/
absorb will need a more refined view once they combine devices whose availability chains are
deeper than that. Two concrete cases raised:
- A compute host running an integration (e.g. protocols-server-2) has its OWN liveness, separate
  from any device it reports on — exactly the still-unbuilt "stacked/dependent availability"
  reminder already flagged in `splendid-zooming-lighthouse.md`'s out-of-scope section (device →
  integration/HA-instance → compute host propagation, three independent signals today with no
  propagation between them).
- Netatmo's IP-connected base modules add a layer of their own: an outdoor/rain/wind module's true
  availability depends on ITS radio link to the base module AND the base module's own IP/WiFi link
  to Netatmo's cloud AND the compute-host case above — a genuine chain, not a flat set, and
  today's per-entity "node" capability only ever captures the nearest link in it.
Not designed — PROJECT.md's step 6 (Logical layer) is where this gets picked back up, alongside
aggregate/absorb themselves.
