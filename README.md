# Home Assistant DSL

A Pascal/fil-style DSL that generates Home Assistant YAML (automations, scripts, template sensors/switches, Lovelace entity-card lists) from structured definition files.

## Building

```
go build -o homeassistant .
```

## Usage

Run from inside a per-house directory:

```
cd New/Vienna
homeassistant Definitions/Main.def
```

Output lands in `hass/` and `list.*` files alongside it. The generated `hass/` tree is rsynced to the relevant Home Assistant instance by the `Update` script.

Debug reports can be collected by passing `--debug` before the `.def` path.

## Repository layout

```
New/
  Vienna/
    Definitions/       per-house definition files
    hass/              generated YAML (target: rsync to HA)
    list.*             Lovelace entity-card lists
  Junglinster/
    ...
  Shared/
    Definitions/
      Macros.def       macro definitions shared across all houses
      Settings.def     global variable defaults
```

### Per-house definition files

| File | Purpose |
|---|---|
| `Main.def` | Include-order entry point |
| `Settings.def` | Per-house variable overrides (also holds real secrets — gitignored) |
| `Physical.def` | Physical layer: MQTT/Home Assistant main target, integration device declarations |
| `Spaces.def` | Conceptual layer: space and entity declarations |
| `Lists.def` | Lovelace list declarations |

## Entity specification model

Entities are named with a domain, sphere, and path.

**Extensional** (absolute):
```
type.sphere/path          e.g.  sensor.social/apartment/hallway/temperature
type.[raw-name]           e.g.  sensor.[some_integration_entity]
```

**Intensional** (space-relative, resolved against the current space context `x`):
```
type.sphere:path          →  type.sphere/x/path
type.sphere:/path         →  type.sphere/path
type.sphere:path:sub      →  type.sphere/x/path/sub
```

**Entity spheres** (conceptual-layer namespaces — see Architecture.md §2 for how spheres relate to
the separate conceptual/logical/physical *layer* split; "physical" names a layer there, not a
sphere):

| Sphere | Meaning |
|---|---|
| `social` | Entities with a direct social/usage role |
| `infrastructural` | Entities reporting on the state of the automation platform's own IoT infrastructure (nodes, battery sensors, radio/signal strength, …) |
| `meta` | Entities that operate the DSL/generator/coordinator system itself, rather than the house or its infrastructure (e.g. discover-entity/reload/restart controls) |

`physical` also appears as a sphere in both real houses' current Spaces.def files (2026-09-06 and
earlier) — a legacy naming choice from before the conceptual/logical/physical layer split was
established, not part of the current three-sphere standard above. Left as-is deliberately: retiring
it would mean renaming ~200 real, already-deployed HA entity_ids across both houses (dashboards,
history, automations), a separate migration of its own, not a documentation-only change.

## DSL syntax

### Spaces

```
space social:living_room with:
  entity light.social:main;
  entity switch.social:media;
  ...
end;
```

Spaces can be nested. Virtual spaces aggregate multiple physical spaces:

```
virtual-space social:whole_apartment with:
  member switch.social:/apartment/living_room/space;
  member switch.social:/apartment/kitchen/space;
end;
```

A space may declare itself a Home Assistant Area with `as area`. Devices positioned directly in
it, or in a sub-space that doesn't declare its own area, get it as their `suggested_area`; a
nested `as area` space shadows its enclosing one for its own subtree:

```
space infrastructural:rack as area with:
  ...
end;
```

### Entities

```
entity light.social:main;                            # assumed (provided by HA integration)
entity sensor.physical:temperature with:             # explicitly defined
  value climate.physical:radiator!current_temperature;
end;
entity sensor.physical:pressure with adjustment 20 1;  # adjustment sensor wrapping a _raw source
```

`!attribute` (as used above) reads one of an entity's attributes instead of its bare state.

### Devices (the `hosts` integration)

`Physical.def`'s `integration hosts with: ... end;` block declares each pingable/monitorable host
and how it materializes into Home Assistant entities. Three integration types:

```
integration hosts with:
  device host.smarty smarty cpu;                       # bare: hardwired load/temperature capabilities

  device host.someswitch someswitch ping;               # liveness-only, no capabilities

  device host.eriks-mac-studio eriks-mac-studio cpu cloud import;  # reports via the shared cloud
                                                                    # broker, relayed back locally

  device host.junglinster junglinster home_assistant with:
    cpu/load:        sensor.processor_use;               # grouped -> its own sensor entity, path suffix "cpu"
    cpu/temperature: sensor.processor_temperature;
    sw_version: "Home Assistant Operating System " update.home_assistant_operating_system_update!installed_version;
    model:       "Home Assistant Green";                 # constant device-map field
    hw_version:  "1928930" forced;                        # "forced": never overridden by live-reported data
  end;
end;
```

A capability name is either `<leaf>` (a device-info string — manufacturer, model, sw_version,
... — feeds the HA discovery `device:` block only) or `<group>/<leaf>` (a variable attribute —
gets its own sensor entity at `.../<group>/<leaf>`, posted as numeric JSON on the device's MQTT
state topic). A capability's entity reference may use the same `entity!attribute` syntax as
regular entity bodies, and an optional leading `"<literal>"` string prefix concatenated onto it.

The trailing `cloud`/`import` keywords route a `cpu`/`ping`-type device's traffic through the
shared cloud broker instead of (or in addition to) this house's own local one, relayed back by the
coordinator — see `Architecture.md` §12 for the full mechanism. **Every macOS `cpu`-reporting host
must use `cloud import`, never a plain local declaration**: `mosquitto_pub` under `launchd`
reliably fails to deliver (`Error: Bad file descriptor`, silent) when the destination is a LAN
address, but works cleanly against the cloud broker every time — a launchd/libmosquitto quirk found
live 2026-09-05 (`Architecture.md` §12 has the full incident writeup), not a bug in this DSL or the
report script. Linux `cpu`-reporting hosts are unaffected and may use either form.

`Spaces.def` then positions a declared host in the conceptual tree and pulls in its implied
entities:

```
space infrastructural:rack as area with:
  entity device.infrastructural:junglinster from host.junglinster with: variable_device_attributes;
end;
```

This implies a node (availability) entity plus one entity per grouped capability (`cpu/load`,
`cpu/temperature` above) — none of them generator-authored YAML; they're created by the
`house_event_bus_coordinator`'s own Home Assistant MQTT Discovery publish instead. See
`Architecture.md` §6 for the coordinator's role and §6.6 for the constant/variable attribute
distinction.

### Devices (the `commandline` integration)

`Physical.def`'s `integration commandline with: ... end;` block declares script-backed entities on
a host running the `mqtt_commandline` daemon (`mqtt_commandline/`) — a third long-running Go
service, alongside the generator and coordinator, for devices with no MQTT integration of their
own (e.g. a picture frame's slideshow control):

```
integration commandline with:
  device host.frame frame with:
    switch.slideshow: "/home/pi/bin/check_slideshow" "/home/pi/bin/start_slideshow" "/home/pi/bin/stop_slideshow";
    button.reboot:    "/home/pi/bin/reboot_frame";
  end;
end;
```

Three capability kinds, each capturing one quoted command-line string per script (path + args —
the daemon does ordinary shell word-splitting at invocation time, so a single generic script can
be reused parametrically):

| Kind | Scripts |
|---|---|
| `switch.<name>` | `status_script` `on_script` `off_script` |
| `sensor.<name>` | `status_script` |
| `button.<name>` | `press_script` |

Use absolute script paths — the daemon runs as a systemd service, which doesn't get an interactive
shell's `PATH`. A device commonly also carries a `hosts`-kind declaration for the same physical
machine (e.g. `host.frame` also reporting cpu/ping) — both share one HA device entry (identical
`identifiers`), each declaring only the capabilities its own kind actually has.

Wire shape (mirrors `hosts/<host>/...`): `commandline/<host>/node/state` (the daemon's own MQTT
Last Will — `"true"` once connected, `"false"` on any disconnect, clean or not — gates every
entity's availability); `commandline/<host>/<entity>/state` (retained, a `status_script`'s trimmed
stdout verbatim, polled every 60s and republished immediately after any switch command);
`commandline/<host>/<entity>/set` (switch command, `"1"`/`"0"`); `commandline/<host>/<entity>/press`
(button command, no state). The coordinator only ever authors HA MQTT Discovery for these — a
switch/button's `command_topic` points directly at the daemon's own topic, HA publishes there
straight from the UI, no coordinator-side relay needed.

`Spaces.def` can position a specific capability under a chosen conceptual entity, the same way a
`hosts` device's own attributes are:

```
entity switch.social:picture_frame from host.frame entity slideshow;
```

### Devices (cross-house import)

`Physical.def`'s `integration import with: ... end;` block pulls in a device another installation
already exposes on the shared cloud broker — kind-agnostic: the DSL author never states whether the
exporting installation declared it via `hosts` or `home_assistant`:

```
integration import with:
  device import.pro-1 from junglinster host.pro-1 with:
    binary_sensor.node;
    sensor.cpu/load;
    sensor.cpu/temperature;
  end;

  device import.vienna_livingroom from junglinster hass.vienna_livingroom with:
    binary_sensor.node;
    sensor.co2;
    sensor.humidity;
  end;
end;
```

`<remote-device-id>` is the exporting installation's own `DeviceID`. Each `<domain>.<capability>;`
line names one capability to pull in — `<domain>` is for readability only (never stored); the local
discovery config's actual domain always comes from wherever `Spaces.def` positions it, coercing
across any domain mismatch with the remote's own declaration silently, by design. A capability's
name must match the DSL-wide convention the exporting installation's own declarations use for it
(e.g. `cpu/load`, group-prefixed, for a `hosts`-kind capability — see the `hosts` integration
section above) — the coordinator recomputes the exporter's own stable id from
`(remote-installation, remote-device-id, capability-name)` and matches on that alone, never on the
incoming payload's own content.

`Spaces.def` positions an imported device exactly like a native one:

```
device infrastructural:pro-1 from import.pro-1 with:
  entity sensor.infrastructural:pro-1/cpu/load        from entity cpu/load;
  entity sensor.infrastructural:pro-1/cpu/temperature from entity cpu/temperature;
end;
```

Only capabilities Spaces.def actually references end up in `coordinator/imported.yaml` — an
imported-but-unused capability is silently skipped, same as every other device kind's own
positioning gate. See `Architecture.md` §12 for the underlying cloud-broker federation mechanism.

### Macros

Macros expand into one or more entity declarations. They are defined in `Macros.def` and invoked from `Entities.def`:

```
call battery_level_device :aqara_sensor with:
  alert_level 10;
end;
```

## Macro system

### Parameter types

| Type | Description |
|---|---|
| *(default)* | Entity specification (`type.sphere:path` form) |
| `string` | Any text value |
| `int` | Numeric value |
| `boolean` | `true`/`false` |
| `entityReference` | Concrete reference to an entity |
| `entity_name` | Fully qualified entity name (`domain.sphere/path`) |
| `entity_path` | Entity path including sphere (`sphere/path`) |
| `entity_space_path` | Path without domain or sphere |
| `path` | Home Assistant entity or node path |
| `option` | Optional flag (implicitly optional, defaults to `false`) |
| `set<string>` | Comma-separated string list |
| `set<int>` | Comma-separated integer list |
| `set<entityReference>` | Comma-separated entity reference list |
| `time` | `HH:MM:SS` |

Parameters may be marked optional with `op`. `option` parameters are always optional.

### Implied parameters

Every macro invocation automatically provides, regardless of the macro header:

| Variable | Value |
|---|---|
| `${domain}` | HA domain (e.g. `switch`) |
| `${sphere}` | Sphere (e.g. `social`) |
| `${entity}` | Full entity path (space path + subdomain) |

### Macro definition syntax

```
macro name [no_raw] [space_level] ( $positional type, ... ) { $named type [op], ... }:
  <body>
end;
```

## Lists.def syntax

`Lists.def` declares Lovelace entity-card lists. Each declaration produces a `list.<name>` file in the house directory.

```
list "Title" all <pattern> [<pattern> ...] [as cards] [with:
  clean_prefix  <segment>;
  clean_postfix <segment>;
  detail_level  <n>;    # "as cards" only, default 2
end;]
```

### Patterns

Each pattern selects a subset of declared entities:

| Pattern form | Matches |
|---|---|
| `domain.*` | All entities in the domain |
| `domain.*/suffix` | Entities whose path ends with `/suffix` |
| `domain.sphere/*/suffix` | Entities in a specific sphere whose path ends with `/suffix` |
| `domain.*/prefix/*` | Entities whose path has `prefix` as the second-to-last segment, with any one trailing leaf segment (e.g. `sensor.*/cpu/*` matches `.../cpu/load`, `.../cpu/temperature`, or any future `cpu`-suffix attribute — but not bare `.../cpu`) |

The `all` keyword is optional and silently ignored.

When a sphere is specified (e.g. `binary_sensor.social/*/door`), the pattern matches only entities
in that sphere. It also promotes derived sphere-level group entities for spaces where the
group directly wraps a declared sensor — so the social group (e.g. `social_apartment_bedroom_door`)
appears instead of the underlying physical entity.

Derived aggregate groups at parent levels (e.g. `social_apartment_door`) are never included.

### Clean operations

`clean_prefix <segment>` strips `<segment>/` from the start of the display name.  
`clean_postfix <segment>` strips `/<segment>` from the end of the display name.

Multiple operations are applied in order.

### Example

```
list "Windoors" all binary_sensor.social/*/door binary_sensor.social/*/window with:
  clean_prefix  social;
  clean_prefix  apartment;
end;
```

This selects all social-sphere door and window binary sensors, strips the `social/` and
`apartment/` prefixes from their display names, and writes the result to `list.windoors`.

### Card types

By default a list renders as a flat Lovelace `entities:` card. Adding `as cards` to the header
instead renders a `vertical-stack` of one built-in `type: sensor` mini-graph card per entity
(`detail`, `graph: line`, `name`), useful for numeric time-series attributes:

```
list "Compute nodes" all sensor.*/cpu/load sensor.*/cpu/temperature as cards with:
  clean_prefix infrastructural;
  detail_level 2;
end;
```

## Post-generation checks

After YAML generation three checks run automatically:

1. **Referential integrity** (always, offline): every entity ID referenced in the generated YAML must be in the declared set (DSL-declared entities + generator-implied entities from output filenames).

2. **Online availability** (when HA is reachable): entities assumed to be provided by integrations (no definition or import in the DSL) are verified against the live HA instance.

3. **Bridge entity availability** (when each bridge is reachable): remote entity IDs referenced by `import rest` directives are verified against the bridge's live HA instance.

All three checks are advisory: warnings are printed but generation still completes.
