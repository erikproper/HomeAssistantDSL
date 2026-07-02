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
| `Settings.def` | Per-house variable overrides |
| `Server.def` | Server/bridge conditional definitions |
| `Bridges.def` | REST bridge declarations |
| `Entities.def` | Space and entity declarations |
| `Lists.def` | Lovelace list declarations |

## Entity specification model

Entities are named with a domain, sphere, and path.

**Extensional** (absolute):
```
type.sphere/path          e.g.  sensor.physical/apartment/hallway/temperature
type.[raw-name]           e.g.  sensor.[some_integration_entity]
```

**Intensional** (space-relative, resolved against the current space context `x`):
```
type.sphere:path          →  type.sphere/x/path
type.sphere:/path         →  type.sphere/path
type.sphere:path:sub      →  type.sphere/x/path/sub
```

**Entity spheres:**

| Sphere | Meaning |
|---|---|
| `physical` | Entities without an immediate social role |
| `social` | Entities with a direct social/usage role |
| `infrastructural` | IoT infrastructure entities (nodes, battery sensors, …) |

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

### Entities

```
entity light.social:main;                            # assumed (provided by HA integration)
entity sensor.physical:temperature with:             # explicitly defined
  value climate.physical:radiator!current_temperature;
end;
entity sensor.physical:pressure with adjustment 20 1;  # adjustment sensor wrapping a _raw source
```

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
list "Title" all <pattern> [<pattern> ...] [with:
  clean_prefix  <segment>;
  clean_postfix <segment>;
end;]
```

### Patterns

Each pattern selects a subset of declared entities:

| Pattern form | Matches |
|---|---|
| `domain.*` | All entities in the domain |
| `domain.*/suffix` | Entities whose path ends with `/suffix` |
| `domain.sphere/*/suffix` | Entities in a specific sphere whose path ends with `/suffix` |

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

## Post-generation checks

After YAML generation three checks run automatically:

1. **Referential integrity** (always, offline): every entity ID referenced in the generated YAML must be in the declared set (DSL-declared entities + generator-implied entities from output filenames).

2. **Online availability** (when HA is reachable): entities assumed to be provided by integrations (no definition or import in the DSL) are verified against the live HA instance.

3. **Bridge entity availability** (when each bridge is reachable): remote entity IDs referenced by `import rest` directives are verified against the bridge's live HA instance.

All three checks are advisory: warnings are printed but generation still completes.
