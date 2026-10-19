---
name: project-logical-layer-dependency-on
description: "DONE + DEPLOYED (2026-09-16): logical-layer 'device <id> with: dependency on <other-id>; end;' in Logical.def makes a device's own availability AND in another device's own liveness, transitively, with cycle detection. Now supports BOTH import-kind and hassbridge-kind dependents (target must still be hosts-kind). First deployed case: Vienna's fritz_box merge, see the file's own 'Second deployed case' section. Read this before extending the logical layer further (e.g. the queued 'dependency on entity <id>;' forms)."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-16T13:31:38.780Z
---

## What this is

Vienna's first logical-layer construct, built from a concrete case the user found scanning
Conceptual.def: `node.vienna_livingroom` (an `import`-kind device relaying Junglinster's own
Netatmo readings) should go unavailable whenever `host.netatmo` (a Vienna-local `hosts`/`ping`-kind
device pinging the Netatmo base station's own LAN address) is unreachable. Previously this was
faked with a separate `infrastructural:netatmo_module from host.netatmo;` placeholder device whose
own node entity showed the liveness independently, with no real link to `node.vienna_livingroom`'s
own entities at all.

New grammar (`Logical.def`, no outer "integration X with:" wrapper -- the logical layer isn't tied
to a protocol):

```
logical layer with:
  device <device-id> with:
    dependency on <device-id>;   # repeatable
  end;
end;
```

A logical device sharing its id with an existing physical-layer device (any kind) is an
EXTENSION, not a second competing device -- Conceptual.def positions it exactly as before, this
just adds extra availability wiring on top. `LayerLogical = "logical"` already existed as a
recognised layer keyword in `layers.go` (added when `Logical.def` was first stubbed in, see
[[project_logical_layer_introduction]]) but nothing ever collected it until now.

## How it resolves (generator-side, [[project_meta_reload_servicenotfound_fix]]-adjacent files)

- `homeassistant/integration_logical_parser.go` / `integration_logical_storage.go` (new):
  `TLogicalDevice{DeviceID, DependsOn []string}`, `resolveTransitiveDependencies` (DFS with cycle
  detection -- A→B→C flattens to {B,C}; a genuine cycle returns the offending path and the caller
  discards the whole chain rather than partially applying it), `resolveDependencyAvailabilityTopics`
  (dispatches each resolved dependency id by kind).
- **Scope today: `hosts`-kind dependency targets only** (all three IntegrationTypes --
  ping/cpu/home_assistant -- share `THostsEntityMaterialization.NodeTopic(hostName)`, e.g.
  `"hosts/netatmo/node/state"`, publishing literal `"true"`/`"false"`). A dependency naming a
  hassbridge/discovery/import device warns clearly and is skipped, not silently ignored or
  guessed at -- extend `resolveDependencyAvailabilityTopics` the next time a real case needs one
  of those kinds.
- Resolved ONCE in `generator.go`'s `parseAdministrationFromPaths` (right after `admin` exists,
  since it needs both `logicalByID` and `hostDevicesByID`), stored on
  `TAdministrationState.DependsOnAvailabilityTopics map[string][]string` (device id → topics).
  `Physical_Generator.go`'s own independently-re-parsed imported-device slice (the one that
  actually writes `coordinator/imported.yaml`) just CONSULTS that map
  (`applyResolvedDependencyTopics`) rather than re-resolving/re-walking the graph a second time.
- `coordinator/imported.yaml` gained a new optional per-device field,
  `depends_on_availability: ["<topic>", ...]`.

## How it applies (coordinator-side)

- `buildAvailabilityFields(body, stateTopic string, nodeTopics ...string)` (`discovery.go`) --
  generalized from a single `nodeTopic string` to a variadic list; every existing single-topic
  call site (hassbridge, import) is source-compatible unchanged (a bare string argument still
  satisfies a variadic `...string` parameter). Every topic gets the SAME existing on/true
  normalization template -- no new "what payload means available" concept needed, the existing
  dual on/true convention already covers every kind found so far.
- `discoveryimport.go`: `TImportedDevice.DependsOnAvailability []string` (decoded from the new
  YAML field), threaded through the renamed `importedAvailabilityTopics` (was
  `importedAvailabilityTopic`, singular) into both discovery-body builders. The device's own
  dependency topics apply to EVERY capability including its own "node" (unlike its self-referential
  own node topic, which correctly excludes itself to avoid circularity) -- a dependency going down
  must mark the dependent device's own node false too, per the user's explicit requirement.

Verified live-equivalent via full regenerate+diff (not yet deployed): Vienna's `imported.yaml`
gained exactly the expected `depends_on_availability` block on `node.vienna_livingroom`;
`devices.yaml` lost `netatmo_module`'s own conceptual entry (the intended, expected placeholder
retirement); the generated `hass/` tree changed in exactly one file (the aggregate
`binary_sensor.infrastructural_nodes.yaml` list, dropping the retired node). Junglinster
regenerated byte-identical (no Logical.def, no dependencies declared there at all).

## Queued follow-up (user's own next request, not yet built)

Rename was itself a mid-implementation revision: "depends on" → **"dependency on"** (reads more
naturally after "with:") -- already fully applied throughout.

Two more dependency-target forms are explicitly wanted next, once a real case needs them (same
"on demand" discipline as the rest of this layer):
```
device X with:
  dependency on entity binary_sensor.KKK;                    # raw state
end;
device X with:
  dependency on availability of entity DDD.KKK;              # raw availability, not state
end;
```
Both name an already-existing, assumed-to-exist HA entity on "main" directly (not another
Physical.def device id) -- needs (a) adding the referenced entity to kind-5's existence-check list
(`main_entities.go`), and (b) a NEW generator-authored automation on "main" that watches that
entity and publishes its state (or, for the "availability of" form, whether it's NOT
unavailable/unknown) to a fresh dedicated MQTT topic, since main doesn't otherwise publish
arbitrary entity states to MQTT at all -- that new topic then plugs into
`resolveDependencyAvailabilityTopics` as a third dispatch case, same shape as the hosts-kind one.
**The user's own explicit refinement**: that publishing automation must only be generated when
device X is actually positioned/used at the conceptual layer (or exported) -- mirror the existing
`hasLink`/`DeviceConceptualLinks` gating already used everywhere else
(`generateHassBridgeFile`/`generateImportedDeviceFile`'s own "skip if never positioned" checks),
not generated unconditionally for every Logical.def declaration.

Currently `logicalDependencyOnPattern`'s regex deliberately requires the dependency target to be
ONE bare token, so `"dependency on entity binary_sensor.KKK;"` (two tokens) and `"dependency on
availability of entity DDD.KKK;"` (four tokens) both currently fall through to a generic
"unrecognised line" warning rather than being misparsed as a device-id of "entity"/"availability"
-- safe to extend without any risk of silently misreading either new form once built.

First deployed (2026-09-16): Vienna's `node.vienna_livingroom depends on host.netatmo`, verified
live via MQTT/coordinator inspection.

## Second deployed case (2026-09-16): extended to hassbridge-kind dependents, fritz_box merge

Real bug found live: [[project_no_duplicate_final_entity_ids_rule]] -- Vienna's Physical.def
independently declared the router as TWO devices, `node.fritz_box` (a `home_assistant` bridge
device, rich capabilities) and `node.fritz.box` (a `hosts`/ping device, bare liveness only), both
independently positioned in Conceptual.def, sanitizing to the identical final entity_id
(`binary_sensor.infrastructural_apartment_living_room_fritz_box_node`) -- HA silently gave the
second one a `_2`-suffixed duplicate. The user's own fix, mirroring the netatmo precedent: fold the
ping device into the bridge device via `dependency on`, rather than just deleting the redundant
positioning.

This required extending "dependency on" -- previously wired ONLY for `import`-kind dependents -- to
ALSO support `hassbridge`-kind dependents (the dependency-resolution algorithm itself,
`resolveTransitiveDependencies`/`resolveDependencyAvailabilityTopics`, needed zero changes; it was
already kind-agnostic on the dependent side and already supported hosts-kind targets). Mirrored the
proven import-kind pattern exactly:
- `THassBridgeDevice.DependsOnAvailabilityTopics []string` (generator,
  `integration_hassbridge_storage.go`) / `THassBridgeDevice.DependsOnAvailability []string`
  (coordinator, `discoveryhassbridge.go`, yaml `depends_on_availability`).
- `applyResolvedDependencyTopicsToHassBridge` (`integration_logical_storage.go`) -- map-based
  mirror of `applyResolvedDependencyTopics` (hassbridge devices are collected as a map, never
  independently re-parsed as a slice the way imported devices are); wired into
  `Physical_Generator.go` between `collectHassBridgeDevicesByID` and `generateHassBridgeFile`.
- `integration_hassbridge_generator.go` gained the same `depends_on_availability:` YAML emission
  `integration_import_generator.go` already had.
- Coordinator: `hassBridgeAvailabilityTopic` (singular) renamed to **`hassBridgeAvailabilityTopics`**
  (`[]string`, appending `device.DependsOnAvailability` -- exact mirror of
  `importedAvailabilityTopics`), `buildHassBridgeEntityDiscoveryBody`'s `nodeAvailabilityTopic
  string` param became `[]string` threaded via `...` into `buildAvailabilityFields`. Both of
  `subscribeHassBridge`'s own local+cloud-export call sites, and
  `republishHassBridgeDeviceCapabilities`'s own local+cloud-export call sites, updated (the
  cloud-export halves now map EVERY topic in the list through `canonicalizeRoamingBridgeTopic`,
  not just one).

**User also renamed the two devices during this same fix** (a naming-convention correction, not
required by the mechanism itself): `node.fritz_box` (the hassbridge/rich device) → **`utility.
fritz_box`**, `node.fritz.box` (the hosts/ping device) → **`node.fritz_box`** -- reserving the
`node.*` prefix for genuine ping/liveness-only devices, freeing it up from the richer bridge
device. Conceptual.def's positioning now reads `device infrastructural:fritz_box from
utility.fritz_box with: ...;` (the redundant second positioning, `... from node.fritz.box;`,
removed outright); Logical.def: `device utility.fritz_box with: dependency on node.fritz_box; end;`.

Verified live: every fritz_box capability's entity_id preserved byte-for-byte; `hass/` tree diffed
byte-identical (this is a purely coordinator/MQTT-side device, no main-instance YAML involved);
`./generate` runs with zero warnings post-fix; deployed coordinator to protocols-server-1; journal
showed the expected stale-topic retirement (every fritz_box capability's discovery payload
legitimately changed shape, gaining the extra ANDed availability entry) with no errors; live MQTT
inspection of `.../fritz_box_node/config` confirmed `availability_mode: "all"` ANDing the bridge's
own state topic AND `hosts/fritz.box/node/state`, both currently true. User manually cleaned up the
resulting stale `_2`-suffixed HA registry entity afterwards.

Also documented as a new Architecture.md §6.7 paragraph (2026-09-16): this mechanism only works for
MQTT-published entities; domains hard-wired straight into the main instance via a local integration
(weather, media_player, ...) have no MQTT availability topic to depend on, and need a Jinja
`availability:` template on the generator-authored YAML entity instead (the mechanism
[[project_logical_layer_media_player_device]]'s own `is available [with: enabler ...;]` capability
shape already uses).
