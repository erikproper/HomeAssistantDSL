# Checkpoint — Home Assistant DSL Generator
**Frozen:** 2026-03-26
**Last commits pushed to GitHub:** `38f7e00`, `3d24a40`
**Status:** all tests passing (`go test ./...` → ok), no warnings on expand/generate.

---

## What the project is

A Pascal/fil-style DSL that generates Home Assistant YAML. Two houses: **Vienna** and **Junglinster**. The Go generator (`go run . generate <house>`) replaces an old bash system. Goal is 1:1 output parity (Phase 1), then Phase 2 for entity body properties.

---

## Phase 1 — complete

All generator steps implemented in `generator.go`:

| Step | Output path |
|------|-------------|
| `configuration.yaml` | fixed-content |
| Integration aggregator files | `integrations/*.yaml` |
| Template lights | `entities/template/light/<sphere>/<id>.yaml` |
| Template switches | `entities/template/switch/<sphere>/<id>.yaml` |
| Heating leakage evidence groups | `entities/binary_sensor/physical/<id>.yaml` |
| Binary sensor subdomain groups | `entities/binary_sensor/social/<id>.yaml` — door, window, motion, water + windoor |
| Sensor subdomain mean groups | `entities/sensor/social/<id>.yaml` — temperature, humidity, co2, noise, pressure, illuminance |
| Zigbee light groups | `entities/light/<sphere>/<id>.yaml` |
| Customisation files | `customization/<domain>/<sphere>/<id>.yaml` |

**Key semantics:**
- Binary sensor groups: hierarchical bottom-up (reverse SpaceOrder); sphere-level + windoor as post-loop
- Sensor groups: flat subtree — all matching `sensor.*` entities in the space and descendants; `type: mean`, `ignore_non_numeric: true`, `state_class: measurement`
- `SpaceHasExplicitOff`: explicit `space off:` → turn_off = only listed items; implicit → also include lights
- Template switch `turn_on`: only items ending `/space`
- Template light `turn_off`: union of `SpaceOnByName` + `SpaceLightsByName`
- All items sorted alphabetically to match old bash output
- `NoCollect: true` on all derived aggregates

**`DeriveBinarySensorSubdomainAggregates()`** (administration.go) runs before `RunPostParseChecks` to register derived `binary_sensor.social/<space>/<subdomain>` entities, so `heating leak:` references to them (e.g. `binary_sensor.social:door` in a space with a physical door sensor) pass validation without false warnings.

**Remaining Vienna diffs vs Old (13 files, all understood/accepted):**
- `light.social` — old system self-references (old bug); new correct
- `light.social_apartment_living_room` — legacy `physical_main_space` sub-space not in new DSL
- `switch.social_apartment_bedroom_space` — `sonos_media` Phase 2
- `switch.social_apartment_living_room_space` — legacy `physical/rack` sub-spaces
- `switch.social_apartment_living_room_vidja_space` — legacy `physical_vidja_left/right`
- `switch.social_apartment_space` — child media switches (Phase 2)
- 7 customisation files — 5 need `icon:` body parsing (Phase 2); 2 bitron naming deliberate

---

## Recent session changes

- `availability.go` → `defined.go`: all references renamed (`runDefinedCheck`, `checkHouseDefined`, `parseServerAssignmentsWithDefined`, `DebugReportDefined = "Defined"`, report header `=== ASSUMED ENTITY DEFINITION CHECK ===`, offline string, test names in `main_test.go`)
- `defined.go`: "Assumed external entities checked:" → "Assumed defined entities checked:"
- `administration.go`: `DeriveBinarySensorSubdomainAggregates()` + `registerDerivedBinarySensorAggregate()`
- `parser.go`: calls `DeriveBinarySensorSubdomainAggregates()` before `RunPostParseChecks`
- `generator.go`: added `generateBinarySensorSubdomainGroups`, `generateSensorSubdomainGroups`, `directChildSpaces`, `entityNamesToIDs`, `buildSensorGroupYAML`; two new steps in `generateHouseYAML`
- `CLAUDE.md`: checkpoint location updated from `~/.claude/checkpoints/` to `CHECKPOINT.md` in project root

---

## Phase 2 — pending

1. **REST sensor import** (`imported rest`) → `entities/sensor/<sphere>/<id>.yaml`. Vienna ~130 files, Junglinster ~300. Conditional variant adds Jinja2 `value_template`. Once done, sensor subdomain groups auto-populate.
2. **`providing` macro** → `binary_sensor.infrastructural:<path>/node` with reachability condition template. Needs Phase 2 condition body.
3. **CLI sensors** (`cli_sensor`) → `entities/command_line/sensor/<sphere>/<id>.yaml`
4. **CLI switches** (`cli_switch`) → `entities/command_line/switch/<sphere>/<id>.yaml`
5. **Template sensors** with `adjustment <offset> <scale>` → `entities/template/sensor/<sphere>/<id>.yaml`
6. **`condition` / `value` directives** → template binary_sensor or attribute-pull sensor
7. **Entity properties**: `device_class`, `delay_on/off`, `available`, `enabler`
8. **`input_number`** → `entities/input_number/<sphere>/<id>.yaml`
9. **`input_boolean`** → `entities/input_boolean/<sphere>/<id>.yaml`
10. **`input_datetime`** → `entities/input_datetime/<sphere>/<id>.yaml`
11. **Cover cascade scripts** (`open_stop_close`) — two-level cascade; Vienna 36 files, Junglinster ~117
12. **Heating preset scripts** (`set_heating_to_around/away/day/night`)
13. **Automations** — `lights_motion_guarded`, covers, heating; Vienna 21, Junglinster 83
14. **`windy` / `sunny` macros** → `input_number` threshold + template binary_sensor with `delay_on/off`
15. **Timer entities** (Junglinster only) → `entities/timer/social/<id>.yaml`
16. **`list` construct** → plain-text list files like `Old/Vienna/list.battery_alerts`

---

## Key files

| File | Role |
|------|------|
| `generator.go` | All Phase 1 YAML generation steps |
| `administration.go` | Runtime state; space/entity registry; derived aggregate derivation |
| `parser.go` | CDL1-style recursive-descent parser; calls admin + checks |
| `checks.go` | `RunPostParseChecks` — post-parse subset validation |
| `expander.go` | Macro expansion; `normalizeEntityFullName`; entity identity |
| `defined.go` | HA REST API entity definition check (renamed from availability.go) |
| `debug.go` | Debug report infrastructure; `DebugReportDefined = "Defined"` |
| `main.go` | CLI: interpret, expand, check, generate |
| `New/<House>/Definitions/` | DSL source files |
| `Old/<House>/hass/` | Old bash-generated YAML (comparison reference) |
| `New/<House>/hass/` | New Go-generated YAML output |

## Commands

```bash
go run . generate vienna          # regenerate Vienna YAML
go run . generate junglinster     # regenerate Junglinster YAML
go run . expand vienna            # macro expansion report
go run . check vienna             # entity definition check (needs HA running)
go test ./...                     # all tests
diff -rq New/Vienna/hass/vienna Old/Vienna/hass/vienna   # compare outputs
```
