# Device-level "trigger" abstraction (Step 1 of the raw-event-code cleanup)

## Context

Now that every manual automation lives in Conceptual.def as a real `rule`, the user has spotted a
genuine abstraction breach in the grammar that rule migration itself just finished building: a
`when` clause routinely contains raw, device-level protocol codes --

```
when event.physical:door/moes/1 is 2_single or
     event.physical:bed/moes is 1_single or
     ...
```

`2_single`/`1_single` are Zigbee button click-count codes -- a device-level implementation detail,
not something a conceptual-layer rule should ever need to know. This is also a concrete,
previously-missing prerequisite for an already-tracked future goal: PROJECT.md's "Deal with the
'physical' sphere. Should be empty..." item. The user wants to fix this architecturally, "step by
step," not as one giant rewrite. This plan covers **only the first step**: build the new mechanism
and prove it with tests. It deliberately defers:

- The embedded `and if trigger.<attr> is "<value>";` form (the Signify dimmer's press/hold +
  button-direction case) -- a real case, but its own, later step (confirmed with the user).
- New action-verb cleanup (`dim`/`brighten`, `toggle ... with [default] brightness <N>%`) --
  orthogonal to triggers, its own later step.
- Migrating any of either house's REAL Conceptual.def content to the new construct -- a careful,
  rule-by-rule migration once the mechanism exists and is proven, not part of building it.
- Actually removing "physical" as a sphere -- the long-term goal this is one prerequisite for.

## The new grammar (Step 1 scope)

Inside a `device ... with:` block (Conceptual.def), a new statement form alongside `entity ...;`:

```
device buttons.living_room_hallway_door_moes_1 with:
  trigger social:main/light/toggle_request when event.pressed is '2_single';
  entity binary_sensor.infrastructural:hallway_door/moes/1/node;
  entity sensor.physical:hallway_door/moes/1 from sensor.pressed_as_sensor;
  entity sensor.infrastructural:hallway_door/moes/1/battery_level;
  entity binary_sensor.infrastructural:hallway_door/moes/1/battery_alert;
end;

device buttons.bedroom_bed_moes with:
  trigger social:main/light/toggle_request when event.pressed is in ['1_single', '2_single'];
  ...
end;
```

- `trigger <name> when <capability> is '<value>';` / `... is in ['<v1>', '<v2>', ...];` -- `<name>`
  is a bare `sphere:path` token (no `domain.` prefix -- it names a virtual request, not a real HA
  entity). `<capability>` is resolved exactly like today's `entity ... from <capability>;` (same
  `extractDeviceCapabilityEntityDeclaration`/`registerDeviceCapabilityEntityLink` machinery,
  Conceptual_DeviceCapabilityEntities.go) -- reused, not reinvented.
- The explicit `entity event.physical:X from event.pressed;` line this replaces goes away
  entirely: the underlying entity's own local path is now auto-derived from the device's own
  identity (confirmed with the user) -- same precedent as the existing `DeviceNodeEntityID`
  choke point (administration.go:139, used by device_friendly_name.go/
  Conceptual_DeviceSourceEntities.go for implied/friendly-name resolution), not a new naming
  mechanism.
- Multiple DIFFERENT devices may declare the SAME `<name>` -- the generator ORs all of them
  together when building the real HA automation trigger list, exactly replacing what a hand-written
  multi-clause `when A is X or B is Y or ...;` chain does today, just sourced from the device
  declarations instead of the rule text.
- Rule-level reference (new `WhenStatement` alternative, rule_parser.go): `when [a]
  <name> is triggered` -- the optional leading `a` is purely cosmetic, parsed and discarded.

## Data model & generator

- `TAdministrationState` (administration.go) gains a new registry, e.g. `NamedTriggers
  map[string][]TNamedTriggerSource` -- keyed by the bare `sphere:path` name, each source carrying
  the resolved entity_id and its own `[]string` of matched raw values. Populated by the new
  device-block line handler (parallel to today's `entity ... from <capability>;` handling,
  Conceptual_DeviceCapabilityEntities.go) during the same parse pass.
- `TRuleTrigger` (administration.go:~450) gains `Kind: "named_trigger"` with just a `Name` field
  (reuse the existing struct, don't add a parallel one).
- `rule_parser.go`'s `EntityAnchoredClause`/`TriggerClause` gets a new alternative recognizing a
  bare `sphere:path` (no domain) followed by `is triggered` -- distinguish from today's
  domain-qualified `EntityRef` grammar (which requires `<domain>.<sphere>...`).
  `checks.go`'s `RunPostParseChecks` needs a new check: a rule's `named_trigger` reference must
  exist in `NamedTriggers` (warn + skip otherwise, same convention as every other "undeclared
  entity" warning this session already added).
- `generator.go`'s trigger-rendering (the code added for `renderRuleTriggerYAML` this session)
  gets a `named_trigger` case: expand into one `event.received` trigger per contributing device,
  each with that device's own `event_type` value list -- structurally identical YAML to what an
  explicit multi-clause `when` chain already renders, just built from `NamedTriggers` instead of
  `rule.Triggers` directly.
- Validate: warn (don't crash) if a named trigger is declared twice by the exact same device (a
  real authoring mistake), and if a rule references a name with zero declaring devices.

## Verification

- New `_test.go` coverage (mirroring this session's own `rule_parser_phase2_test.go` precedent):
  one device declaring a named trigger + a rule referencing it with `is triggered`, asserting the
  generated automation's YAML trigger list matches byte-for-byte what the equivalent explicit
  `when <entity-ref> is '<value>'` rule produces today. A second test with TWO devices declaring
  the SAME name, asserting both end up OR'd into one trigger list.
- `go build ./... && go test ./...` green.
- Explicitly NOT verified against either house's real Conceptual.def in this step -- no real file
  uses the new construct yet; that migration is a separate, later step.
