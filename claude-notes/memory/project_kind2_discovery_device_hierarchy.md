---
name: project-kind2-discovery-device-hierarchy
description: "Kind-2 \"discovery\" integration grammar redesigned — nested device hierarchy + stable local capability names, replacing the old flat \"identifiers\" list. Implemented and verified live 2026-08-26."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-08-25T23:24:38.362Z
---

Flagged 2026-08-26 as needing discussion; the user then sketched a concrete design in the same
session and asked for it to be implemented immediately so it could be tested the next morning
alongside EMS/Enphase. Implemented and verified live against real Junglinster the same day.

**What changed.** The old flat declaration:
```
integration discovery with:
  device discovery.ems_esp including:
    identifiers "ems-esp-thermostat";
    identifiers "ems-esp-boiler";
    identifiers "ems-esp-mixer";
  end;
end;
```
became a nested device hierarchy with per-device capability lines, reusing the same `with: ...
end;` device grammar the `home_assistant`/`hosts` integrations already use:
```
integration discovery with:
  device discovery.ems_esp with:
    device discovery.ems_esp_boiler with:
      sensor.outdoor_temperature: sensor.boiler_outdoortemp;
    end;
    device discovery.ems_esp_thermostat with:
    end;
    device discovery.ems_esp_mixer with:
    end;
  end;
end;
```
And Spaces.def's link now names the *stable local capability name*, not the gateway's raw leaf
id, and targets the specific sub-device that declares it:
```
entity sensor.physical:garage_door/temperature from discovery.ems_esp_boiler.outdoor_temperature;
```

**How it resolved each original problem:**
- **Leaf-name stability** — `sensor.outdoor_temperature: sensor.boiler_outdoortemp;`'s LHS
  (`outdoor_temperature`) is the DSL-authored, permanent name; the RHS is only a *source
  reference*. A capability-line source written domain-prefixed (`sensor.boiler_outdoortemp`, the
  same shape as the gateway's own `def_ent_id`) has that prefix stripped down to the bare
  `uniq_id` the coordinator actually matches on (`bareDiscoveryLeaf`,
  `integration_discovery_parser.go`) — confirmed against the real captured EMS-ESP payload, where
  `def_ent_id` and `uniq_id` correspond exactly (`sensor.boiler_outdoortemp` / `boiler_outdoortemp`).
- **Device hierarchy** — `ParentDeviceID` on `TDiscoveryGatewayDevice` records the real
  parent/child relationship directly from the nested declaration. Turned out the coordinator's
  existing `matchingGateway` (via `via_device`, one hop) already treats a child device's payload
  as belonging to its own declared identifier regardless of hierarchy modelling — so **no
  coordinator-side code changed at all**; this was purely a generator-side (Physical.def parsing
  + `discovery.yaml` shape) redesign. Every device (parent and every nested child) still gets its
  own flat entry in `discovery.yaml`'s `gateways:` block, each with an *inferred* default HA
  device identifier (`inferredDiscoveryIdentifier`: drop the `discovery.` prefix, `_` → `-`, e.g.
  `discovery.ems_esp_boiler` → `ems-esp-boiler`) alongside any explicit `identifiers "...";`
  override lines — so a device whose real identifier matches the naming convention needs no
  explicit declaration at all.
- **`identifiers` predating the newer per-entity-named strategy** — capability lines are now the
  primary way entities get declared, using the exact same `<domain>.<local-name>: <source>;`
  shape as `home_assistant`/`hosts` devices; `identifiers` survives only as an optional override
  for devices whose real HA identifier doesn't follow the inferred naming convention.

**Parser implementation note**: unlike every other `device ... with: ... end;` body elsewhere in
this codebase (hand-rolled flat state machines, since they never nest devices within devices),
kind-2 devices can nest to arbitrary depth, so `integration_discovery_parser.go` uses genuine
recursive descent (`parseDiscoveryDeviceBody` recursing on `TLineCursor`, `defined.go`'s existing
primitive) rather than a flat loop.

**Still open, not built**: a per-leaf *explicit* metadata override clause (the `hass.envoy`-style
`with: unit: "kW"; end;` pattern already available on `home_assistant`/`hosts` capability lines)
doesn't exist for kind-2 capability lines yet — only the source-reference mapping and
[[project_new_architecture_plan]]'s `Defaults.def` gap-fill apply. Flagged in Architecture.md
§6.8, not scoped as a task.

Related: [[project_envoy_power_energy_naming_cleanup]] (a different, kind-3-adjacent
naming-reliability issue) and [[project_new_architecture_plan]] (the `Defaults.def`
gap-fill work this redesign's typing metadata still relies on).
