# Plan: Logical-layer "enabler"/"dependency" sugar + relocate switch.media to "local"

## Progress (2026-09-27)

**DONE, tested, deployed nowhere yet:**
- Part A (switch.media relocated to Physical.def's "local" kind, with the `no_play_input`
  refinement) -- `integration_local_storage.go`/`integration_local_parser.go`/
  `Conceptual_LocalEntities.go` (new `registerLocalMediaSwitchCapability`).
- Part B+C (`enabler <id>;`/`dependency on <id>;` device-level sugar, kind-aware) -- new
  `logical_enabler_dependency_sugar.go` (`applyLogicalEnablerDependencySugar`), wired into
  `generator.go` right after `collectLogicalDevicesByID`. New `TLogicalDevice.Enabler` field +
  parser recognition (both block and one-liner forms).
- Pilot device migrated: Junglinster's `appliance.tv_living_room` (Physical.def + Logical.def)
  now reads exactly the worked example in this plan. Regenerated, byte-diffed against a
  pre-migration snapshot -- clean except two UNRELATED things (see below).
- **Real pre-existing bug found and fixed along the way**: `registerLogicalMediaSwitchCapability`'s
  own sibling "node" reverse lookup broke out of a `range` over `LogicalEntityLinks` on the first
  match -- Go map iteration order is randomised per process, so two consecutive `./generate` runs
  with zero changes produced DIFFERENT output for `appliance.sonos_playbar_livingroom`'s own media
  switch. Fixed (both the new local-kind path and the original logical one) by collecting all
  matches and picking the lexicographically-first, deterministically. Regression test added
  (20-iteration loop, `TestRegisterLogicalMediaSwitchCapabilityDeterministicWithMultipleNodeMatches`).
- **Root cause of why there even WERE two matches for sonos_playbar, found while diagnosing the
  above**: `appliance.sonos_playbar_livingroom` is positioned TWICE in Junglinster's Conceptual.def
  with two different `as` leaves -- line ~301 (`as sonos/playbar`, bare `entity
  binary_sensor.infrastructural:node;`) and line ~328 (`as sonos`, the fuller block with
  `media_player`/`switch.media` too). This is the user's own file, a genuine duplicate
  positioning -- flagged to them, not touched (line 301 looks like the stale leftover, but this is
  their call).
- Unrelated drift noticed in the same byte-diff, NOT caused by this work: a `bloom_left` light
  migration the user made concurrently elsewhere in Conceptual.def, regenerating for the first
  time in this same run. Confirmed unrelated by content (nothing bloom_left-shaped in this plan's
  own edits).

**NOT done yet (next steps, in order):**
1. Confirm with the user whether to remove the stale `sonos_playbar` duplicate positioning.
2. Roll out the same `enabler`/`dependency on` + `switch.media`-relocation migration to the
   remaining 7 sites (Junglinster: `sonos_garage`, `appletv_living_room`, `sonos_playbar_livingroom`;
   Vienna: `apple_tv`, `tv`, `sonos`, `sonos_roam`) -- one at a time, byte-diff each, per this
   plan's own migration discipline.
3. Vienna's `appliance.washing_machine` reconciliation (`or not enabler` -> strict jinja form) --
   confirmed decision above, not yet applied.
4. Not deployed anywhere -- deploy only after explicit confirmation, as always.

## Context

Working through `appliance.tv_living_room`'s Logical.def block (Junglinster) surfaced two
things the user wants fixed before any further migration work continues:

1. **`switch.media <entity>;` doesn't belong at the Logical layer at all.** It's a pure,
   single-source media_player→switch domain coercion (no cross-device combination, no
   availability computation) — architecturally the same kind of thing Physical.def's discovery
   devices already do for a Z-Wave leaf (e.g. node 67's `light.core_2` coercing a switch-domain
   leaf into a light capability). The user's own principle: **the Logical layer should only ever
   define genuinely *derived* entities** (things computed by combining multiple devices/
   conditions) — a bare domain relabel of one already-existing local entity isn't that, and
   should move to Physical.def's `local` integration kind instead, where
   `appliance.tv_living_room` is already declared.
2. **The "enabler" pattern is hand-written in full every time** (absorb the power switch's own
   `core` as `switch.enabler`, then OR its "off" state into the node condition) and so is the
   "extra AND'd availability dependency" pattern — both recur across devices in both houses and
   deserve real sugar: `enabler EEE;` / `dependency FFF;`.

This applies (once built) to all 8 `switch.media` declarations across both houses'
Logical.def, and to every device already hand-writing the enabler/dependency longhand
(confirmed: Junglinster's `appliance.tv_living_room`, Vienna's `appliance.tv`; Vienna's
`appliance.washing_machine` has a related-but-distinct `... or not enabler` form — see open
question below).

## Investigated facts

- `appliance.tv_living_room` is ALREADY a Physical.def `local`-kind device (Physical.def:611,
  inside `integration local with:`, started line 562) with exactly one capability today:
  ```
  device appliance.tv_living_room with:
    derived binary_sensor.node with condition media_player.core is available;
    media_player.core;
  end;
  ```
  `TLocalCapability` (integration_local_storage.go) today supports ONLY a bare `<domain>.<label>;`
  (no source at all — Conceptual.def's own positioning IS the entity's real name, self-
  referential) or the availability-tracking `<domain>.<label> <sibling> is available;` shape. It
  has NO way to declare "this capability's value is domain-coerced FROM this OTHER literal
  already-existing entity" — that's exactly what needs adding to host `switch.media` here.
- The actual media→switch coercion logic already exists and is proven:
  `registerLogicalMediaSwitchCoercion`-family code in `Conceptual_LogicalEntities.go` (~line 160-233,
  `MediaSwitchNoPlayInput` etc.) — reuse this generation logic via a new call site, don't
  reimplement it.
- `switch.media` sites to migrate (both houses' Logical.def):
  Junglinster: `sonos_garage`, `appletv_living_room`, `tv_living_room`, `sonos_playbar_livingroom`
  (this last one has a `with: no_play_input "TV"; end:` block — the new local-kind grammar must
  support that same refinement). Vienna: `apple_tv`, `tv`, `sonos`, `sonos_roam`.
- Enabler pattern, full current longhand (Junglinster `tv_living_room`, Logical.def):
  ```
  device appliance.tv_living_room with:
    capabilities from switch.house_living_room_tv:
      switch.enabler core;
    end;

    derived binary_sensor.node with:
      condition
        (physical appliance.tv_living_room and utility.sonytv) or
        jinja "$1 == 'off'" { enabler };
    end;
  end;
  ```
  Vienna's `appliance.tv` is the same shape plus a `delay_off`. Vienna's `appliance.washing_machine`
  uses a related but textually different idiom: `condition (physical ... and node.candy) or not
  enabler;` (boolean `not` on the sibling directly, no jinja string-equality clause) — confirmed:
  `enabler`'s own desugaring always produces the strict jinja form, so washing_machine's own
  semantics tighten to match once it's migrated (see Decisions below).
- Device-level `dependency on <device-id>;` ALREADY exists (integration_logical_storage.go's
  `resolveDependencyAvailabilityTopics`/`resolveTransitiveDependencies`) but ONLY resolves into
  `depends_on_availability` (MQTT-level), and ONLY for hassbridge/import-kind *dependents* whose
  target is hosts-kind. A `local`-kind device (like `appliance.tv_living_room`) has no MQTT
  discovery topic of its own at all, so this mechanism doesn't reach it today — confirmed by
  reading the function, it's never called for local-kind devices. The condition-tree's own
  `physical <device-id>` leaf (logical_condition_expr.go's `ref` atom) is already exactly the
  right primitive to AND a dependency into a template condition instead — used today by hand in
  every one of the examples above.

## Recommended approach

**A. Extend `local` kind to support a domain-coerced capability with a literal source.**
Add a new capability shape to `TLocalCapability`/integration_local_parser.go:
```
device appliance.tv_living_room with:
  ...
  switch.media media_player.social_house_living_room_tv;
end;
```
and the `with: no_play_input "..."; end;` refinement (mirrors Logical.def's existing
`switch.media <entity> with: no_play_input "<value>"; end;` shape verbatim — same field name,
same grammar, just a new home). Registration reuses the EXISTING
`registerLogicalMediaSwitchCoercion`-family generation code (Conceptual_LogicalEntities.go)
via a new call site triggered from local-kind registration instead of logical-kind — the
generated YAML must come out byte-identical to today's Logical.def-sourced version.

**B. New device-level `enabler <device-id>;` keyword (Logical.def).** Desugars, applied LAST
(after any `dependency` clauses below are folded into the base condition):
```
device DDD with:
  enabler EEE;
end;
```
→
```
device DDD with:
  capabilities from EEE:
    switch.enabler core;
  end;

  derived binary_sensor.node with:
    condition
      <<base-condition>> or
      jinja "$1 == 'off'" { enabler };
  end;
end;
```
where `<<base-condition>>` defaults to `physical DDD` when nothing else applies, ANDed with
every `dependency` target (see C). Implementation: a small desugaring pass in the Logical.def
parser/registration path that synthesizes the same `TLogicalCapability`/absorb/derived-condition
values the longhand already produces — not a new runtime mechanism, just declarative sugar over
the existing one.

**C. Extend the existing `dependency on <device-id>;` keyword (Logical.def, repeatable) to be
kind-aware** (keyword wording UNCHANGED — confirmed: "with dependency on"/"with enabler" both
need to read naturally, so "on" stays):
- If DDD is hassbridge/import-kind (has its own MQTT discovery topic): resolve exactly like
  today's mechanism, into `depends_on_availability`.
- Otherwise (DDD is `local`-kind, or any other kind whose availability is a `derived
  binary_sensor.node with: condition ...;` tree): AND a `physical <device-id>` leaf onto that
  tree instead, alongside `physical DDD` itself.
Both routes already exist independently (MQTT depends_on_availability, and hand-written
`physical X and Y` conditions) — this only needs a dispatch check on DDD's own kind before
picking one, plus letting `enabler`'s own desugaring (B) see the ALREADY-collected dependency
set so `<<base-condition>>` includes it.

**Resulting worked example** (Junglinster, `appliance.tv_living_room`):

Physical.def (local kind, extended):
```
device appliance.tv_living_room with:
  derived binary_sensor.node with condition media_player.core is available;
  media_player.core;
  switch.media media_player.social_house_living_room_tv;
end;
```
Logical.def (fully sugared):
```
device appliance.tv_living_room with:
  dependency on utility.sonytv;
  enabler switch.house_living_room_tv;
end;
```
Both must regenerate byte-identical YAML to what's live today.

## Decisions (confirmed)

1. **Keyword spelling**: `enabler EEE;` and `dependency on FFF;` — "on" stays (reads naturally as
   "device DDD with: dependency on FFF; enabler EEE; end;").
2. **Enabler semantics**: `enabler`'s own desugaring ALWAYS produces the strict jinja form
   (`jinja "$1 == 'off'" { enabler }`), matching the TV precedent. Vienna's `appliance.washing_machine`
   (currently `... or not enabler`, a looser boolean negation true for off/unavailable/unknown
   alike) will have its own semantics tightened to match once it's migrated to the new sugar —
   not a blocker, just noted so the behavioural change is deliberate, not accidental.

## Migration/verification discipline (same as every prior migration this session)

- Build the Physical.def `local`-kind extension (A) first in isolation: `go build/vet/test`
  clean before any `.def` edit.
- Build the `enabler`/`dependency` desugaring (B, C) next, with unit tests covering: the
  worked TV example above producing the identical `TLogicalCapability`/condition-tree values the
  current longhand produces; a device with `dependency` but no `enabler`; a device with neither.
- Migrate ONE device first (`appliance.tv_living_room`), snapshot `hass/`+`coordinator/*.yaml`
  before, regenerate, byte-diff after — must be empty. Only then roll out to the other 7
  `switch.media` sites and Vienna's `appliance.tv`.
- No deploy without explicit confirmation, as always.

## Active (2026-09-27 evening): "switched_device" → Logical.def "switched by" construct

Promoted out of Parked — user has now given the design spec directly. **Planning only, per
explicit instruction — no code written, nothing deployed.** Supersedes/corrects the parked
investigation below where noted.

### Context (from the user, cleaned up)

Conceptual.def still has 7 "definition as switched_device <device> <main>;" declarations (first
one line ~393, `space social:living`) — an old bash-era construct combining, into one conceptual
light: an in-wall/fuse-box switch (the power gate, sometimes mis-assigned domain "light" even
though it's really a switch) and the actual dimmable/colour armature light it powers. Two
follower automations force the armature light to match the switch whenever the switch's own
state changes (already built, see parked facts below).

The user's own architectural framing (new, supersedes the parked plan's "give switched_device a
Logical.def home, keyword TBD"):

- Let `XX` = the actual light (the armature — dimmable/colour bulb or dimmer).
- Let `YY` = the in-wall/fuse-box switch that gates power to it (may be mis-assigned domain
  "light" today, same situation as a plain `enabler`).
- New Logical.def device-level construct:
  ```
  device XX with:
    switched by light YY;
  end;
  ```
  The `by light` part is deliberately domain-parametrized (not hardcoded) so other domains could
  be "switched by" in future — only `light` needs implementing now.
- This desugars to something like:
  ```
  device XX with:
    capabilities from YY:
      light.enabler core;
    end;
  end;
  ```
  Note the absorbed label's domain is `light`, not `switch` — because YY's own `core` capability
  is ALREADY domain `light` at the physical layer here (the historical mis-assignment). This is
  the same absorb machinery [[project_logical_layer_device_combining]]/the existing `enabler`
  sugar already builds (`applyLogicalEnablerDependencySugar`,
  `logical_enabler_dependency_sugar.go`) but that code hardcodes `switch.enabler core;` today — it
  needs a domain parameter to cover this case, OR `switched by light YY;` is really a distinct
  sibling construct to plain `enabler EEE;` (absorbing under a different capability domain) rather
  than a rename of it. **Open design question, not yet resolved — see Open questions below.**
- Generated YAML — the user's own verbal spec used a `<<XX>>_switched`/`<<YY>>_switcher` naming
  placeholder here; **superseded by reading the actual generator code** (see the resolved naming-
  mechanics question further down this document) — the real shape is simpler: a helper group named
  off YY's own path (not a synthetic `_switched`/`_switcher` suffix pair at all), reusing
  `generateSwitchedDeviceEntities`/`generateSwitchedDeviceAutomations` (generator.go:4020-4185)
  unchanged — only how `TSwitchedDeviceRelation` gets POPULATED changes (from the new Logical.def
  construct instead of Conceptual.def's `definition as switched_device X Y;` line).
- The two follower automations (`..._switched_on`/`..._switched_off`) are unchanged in kind —
  already built, already correct in shape.
- **The old `light.physical:main`/`light.physical:main_dimmer` "no_collect" entity lines in
  Conceptual.def can be DROPPED entirely** once XX/YY are properly positioned — their entity_ids
  will instead fall out of real device+capability naming, not a hand-written literal.

### Correction to the parked investigation's Physical.def naming

- Line 393's `entity light.physical:main no_collect with call providing :hallway_door_switch_box/fibaro;`
  is WRONG — the user says this should read `hallway_door_switch_box/zwave_15` (Zwave ID 015), not
  `/fibaro`. Confirmed by research this session: `providing` (Macros.def:10-22,
  `# Should go` — i.e. legacy/deprecated) is a plain macro producing a liveness
  `binary_sensor.infrastructural:<path>/node` derived from the declaring entity's OWN
  availability — NOT an MQTT topic path. Given the user wants both `social:living`/`social:dining`
  fully migrated to proper Physical.def/Logical.def positioning in one round (not just a label
  patch), this `providing` macro line goes away entirely for both — the naming correction just
  confirms the target device stem is `hallway_door_switch_box_zwave_15`, e.g.
  `switches.house_living_room_hallway_door_switch_box_zwave_15` at the Physical.def level, mirroring
  node 67's own shape:
  ```
  device switches.house_living_room_hallway_door_switch_box_zwave_15 with:
    identifiers "zwavejs2mqtt_0xd0204fb2_node15";
    light.light_1 zwavejs2mqtt_0xd0204fb2_15-37-1-currentValue;
    light.light_2 zwavejs2mqtt_0xd0204fb2_15-37-2-currentValue;
    derived binary_sensor.node with condition light.light_1 is available;
  end;
  ```
  **Domain point revisited once more**: the RAW hardware/zwave_js entity is genuinely domain
  `switch` (confirmed via `core.entity_registry`: `switch.physical_house_living_room_living_main`
  / `switch.physical_house_living_room_dining_main`, unique_ids `3491778482.15-37-1-currentValue`/
  `...-37-2-...` — `3491778482` decimal == `0xd0204fb2` hex, confirming the SAME home_id every
  other node in this file uses) — but the user's own follow-up call is to declare it as `light`
  anyway, directly at the Physical.def kind-2 discovery layer, matching node 67/42/64's own
  existing precedent (which ALSO freely relabels a raw switch-shaped leaf as `light.core_2` when
  that's how it'll be consumed). Rationale: a kind-2 discovery declaration's domain is a
  DECLARATION CHOICE (how HA should platform this raw MQTT value), not dictated by the "native" HA
  domain — declaring it `light` directly here is free, whereas declaring it `switch` and then
  needing a separate switch→light coercion template downstream (like the `local`-kind
  `switch.media`/`registerLocalMediaSwitchCapability` mechanism built earlier tonight) would be
  pure waste for something that's going straight into a light group either way. **Capability label
  also updated per the user's further preference**: not the generic numbered `core_1`/`core_2`,
  but a purpose-legible `light_1`/`light_2` (domain doubling as part of the label itself, rather
  than a placeholder word) — so: `light.light_1`/`light.light_2` stands as the actual plan, back
  to the original domain reading (with the label refined once more) — the `switch_as_x`
  helper this replaces was ALSO relabeling switch→light, just at runtime via a config-entry
  instead of for free at declaration time. Endpoint→room mapping: `15-37-1-` = living,
  `15-37-2-` = dining (matches the parked plan's own already-correct read).

### Real, separate bug found this session: wrong automation references

Confirmed live in `/config/automations.yaml` (Junglinster): 4 dim-up/dim-down automations call
`light.turn_on` with `brightness_step_pct` directly on:
```
light.physical_house_living_room_dining_main_dimmer   (x2, dim_up/dim_down)
light.physical_house_living_room_living_main_dimmer   (x2, dim_up/dim_down)
```
The user confirms this is their own naming error — these should read `light.physical_house_living_room_{dining,living}_main` (no `_dimmer` suffix), matching the intended post-migration name for XX (the armature light). Fix these 4 references as PART of this same migration round (their correct value depends on however Physical.def ends up naming XX, so fix after, not before, the positioning is finalised) — not a pre-existing separate task.

### Pilot pair SWITCHED to server_room/corridor (node 48), fully confirmed live — no toggle-testing needed

The user redirected the pilot pair here (their wife will soon be home in Junglinster and could
help toggle switches to identify the zwave sub_id) — but a live `core.entity_registry`/
`core.device_registry` lookup (same technique as node 15) already answers it completely, no
physical toggling required at all:

- **Node 48 confirmed**: Fibaro FGS-223 (`model: "FGS-223"`, same as nodes 15/67), device name
  `house/server_room/fuse_box/zwave/048` (matches the user's own added comment hints exactly),
  home_id `3491778482` (== `0xd0204fb2`, same network as every other node), device_id
  `2145213ea34fe18f70e3c513b247f129`.
- **Endpoint→room mapping, confirmed via unique_id**:
  - `switch.physical_house_corridor_main` — `3491778482.48-37-1-currentValue` → **endpoint 1 = corridor**
  - `switch.physical_house_server_room_main` — `3491778482.48-37-2-currentValue` → **endpoint 2 = server_room**
  (both genuinely domain `switch` at the raw zwave_js layer, confirming the light-domain
  appearance is purely a `switch_as_x` wrapper artifact here too, same as node 15).
- **XX (the armature light) is NOT a single bulb here — it's a Zigbee2MQTT GROUP device**
  (unlike living/dining's single dimmer): `light.physical_house_server_room_main_spots` (grouping
  4 individual ceiling spot lights, `main_spot_1..4`) and `light.physical_house_corridor_main_spots`
  (grouping 3, `main_spot_1..3`). Their own MQTT device identifiers (from `core.device_registry`):
  - server_room: `identifiers: zigbee2mqtt_1221051039810110150109113116116_1`
  - corridor: `identifiers: zigbee2mqtt_1221051039810110150109113116116_4`
  Neither is in Physical.def today (grep confirms zero `main_spots`/`spots` hits) — same
  undeclared state as living/dining's dimmers. Architecturally this doesn't change the plan at
  all: a Zigbee2MQTT group is still just one MQTT device from Physical.def's own perspective,
  same kind-2 discovery shape as any other `light.core` device — it just means XX's own Physical.def
  declaration here is a plain kind-2 group light, not a single-bulb device.
- Proposed Physical.def declarations (server_room/corridor pair), pending confirmation of the
  entity-naming mechanics (still open) before actually writing them:
  ```
  device switches.house_server_room_fuse_box_zwave_048 with:
    identifiers "zwavejs2mqtt_0xd0204fb2_node48";
    light.light_1 zwavejs2mqtt_0xd0204fb2_48-37-1-currentValue;
    light.light_2 zwavejs2mqtt_0xd0204fb2_48-37-2-currentValue;
    derived binary_sensor.node with condition light.light_1 is available;
  end;
  (domain declared `light`, not the raw `switch` — same rationale as node 15 above: free at
  declaration time, avoids a wasted switch→light coercion template downstream.)

  device sensors.server_room_main_spots with:
    identifiers "zigbee2mqtt_1221051039810110150109113116116_1";
    light.core;
  end;

  device sensors.corridor_main_spots with:
    identifiers "zigbee2mqtt_1221051039810110150109113116116_4";
    light.core;
  end;
  ```
  (`zwavejs2mqtt_0x........_node48`'s exact HOME_ID string form — `0xd0204fb2` vs some other
  hex spelling — should still be spot-checked against the live MQTT discovery topic the same way
  nodes 42/54/55/64/67 were, since this is inferred from the zwave_js integration's own decimal
  home_id matching, not read directly off the zwavejs2mqtt bridge's own topic tree — cheap
  confirmation, not a blocker, just not literally proven byte-for-byte yet.)
- This also means node 15 (living/dining) no longer needs to be the first pilot — server_room/
  corridor is now the better first pair precisely because both pieces (the switch AND the
  Zigbee group light) are already fully identified live, with zero remaining unknowns beyond the
  two still-open design questions (entity-naming mechanics, and the Q1/Q2 grammar resolved above).

### Q1 resolved (2026-09-27, later same evening): one unified domain-qualified grammar

The user's answer: NOT a variant relationship — `switched by light YY` and `enabler EEE` both
generalize into ONE construct, because HA's own light `platform: group` template is what buys the
switched-light case its real benefit (luminance/colour-temperature propagation from the armature
light through the group "for free" — not something the plain condition-derived `enabler` approach
gives you). The user then generalized the SYNTAX itself, in two refinement passes (their Note
1/Note 2, reconciled with their Note 3 below into one consistent reading):

```
device XX with:
  <target-domain>.<target-label> is <verb> by <source-domain>.<source-label> from YY;
end;
```

- `<target-domain>.<target-label>` names one of XX's OWN existing capabilities being gated/combined
  (its real domain — `media_player`, `light`, whatever XX actually is; NOT a fixed keyword, despite
  the user's own illustrative examples using `switch.core` as a placeholder in one spot — their own
  clarifying line, "the switch.core of the enabled device does not have to be a switch", confirms
  this domain is descriptive/real, not a literal required token).
- `<verb>` varies by role: `switched` (drives the two `platform: group` blocks + on/off follower
  automations — today's `switched_device`/`TSwitchedDeviceRelation` shape) vs `enabled` (drives the
  `derived binary_sensor.node with: condition ... or jinja "$1=='off'" {source}; end;` availability
  gating — today's plain `enabler` sugar, `applyLogicalEnablerDependencySugar`).
- `<source-domain>.<source-label> from YY` names YY's own capability supplying the signal — **Note
  3: domain is ALWAYS spelt out explicitly here**, no shorthand — and must genuinely match YY's own
  capability's real domain (`switch` for a proper switch device, `light` for a mis-assigned in-wall
  relay like this Fibaro case).

Worked example 1 — tv_living_room (the `enabler` case, fully domain-qualified per Note 3):
```
device appliance.tv_living_room with:
  dependency on utility.sonytv;
  media_player.core is enabled by switch.core from switch.house_living_room_tv;
end;
```
(LHS is `media_player.core` — tv_living_room's own real capability — not `switch.core`; the user's
own draft used `switch.core` as a loose placeholder there, per their own caveat quoted above.)

Worked example 2 — now using the confirmed server_room/corridor pilot pair (the `switched by`
case, still Note-3-qualified), attached directly onto XX's own EXISTING Physical.def positioning
(no new logical-only device needed — same pattern already used for `appliance.tv_living_room`'s
`local`-kind device getting a Logical.def block bolted on tonight earlier). **RHS domain settled
back to `light`, and capability label refined to `light_1`/`light_2`** — per the user's follow-up:
declare YY's endpoints as `light` at the Physical.def layer directly (free), rather than `switch`
(which the raw hardware genuinely is) plus a wasted downstream coercion template, AND give the
capability itself a purpose-legible label rather than the generic numbered `core_1`/`core_2`:
```
device sensors.server_room_main_spots with:
  light.core is switched by light.light_2 from switches.house_server_room_fuse_box_zwave_048;
end;

device sensors.corridor_main_spots with:
  light.core is switched by light.light_1 from switches.house_server_room_fuse_box_zwave_048;
end;
```
(YY's endpoint label `light_1`/`light_2` still pending live confirmation of the underlying
`-37-N-` mapping's stability — see question 3 below; dining's own sibling device off node 15 would
read `light.light_2` from that YY.)

**One thing worth flagging explicitly**: this generalization would REPLACE the plain `enabler EEE;`
one-liner keyword already built and tested earlier tonight (`TLogicalDevice.Enabler`,
`logical_enabler_dependency_sugar.go`) with this fuller, always-domain-qualified two-sided form —
a grammar-surface change on already-tested (not yet deployed) code, not just new work. Worth
confirming that's intended before reworking it, rather than assuming.

### Open questions (need the user's confirmation before implementation — NOT before travel, no rush)

1. ~~Is `switched by light YY;` a variant of the existing `enabler` sugar, or separate?~~ **Resolved
   above** — one unified `<domain>.<label> is <verb> by <domain>.<label> from YY;` grammar, `verb`
   picks the generation path.
2. ~~Remaining ambiguity on the "enabled" verb's LHS target~~ **Resolved (2026-09-27, later
   still): LHS is load-bearing.** The generator must genuinely look up XX's own named capability
   (e.g. `media_player.core` must actually exist on XX) — this is a real change from today's
   already-built `enabler` sugar, whose base condition is the whole-device `physical DDD` atom
   regardless of any specific capability. The new `enabled by` form's base condition should instead
   be built from THAT SPECIFIC capability's own availability (the same "is available" condition-
   building trick already used elsewhere, e.g. `registerDiscoveryAvailabilityEntityLink`'s
   `parseConditionDirective("condition "+sibling+" is available", nil)`), not a blanket `physical
   DDD`. Implementation implication: `applyLogicalEnablerDependencySugar`'s condition-tree
   construction needs revising to look up and validate the named LHS capability, not just assume
   `physical <device-id>` as the base — a real, not cosmetic, change to already-tested code.
   **Confirmed further**: the LHS must name a capability ALREADY declared at the physical level
   (Physical.def, including `local`-kind) — this is a validation, not just a lookup: if XX has no
   such capability declared physically, that's a hard error (or the standard
   defer-once-then-error-on-final-attempt pattern already used everywhere else in this codebase for
   forward-reference resolution), never a silent fall-through to the old `physical DDD` blanket
   condition.
3. ~~Exact entity-id mechanics for `<<XX>>_switched`/`<<YY>>_switcher`/`physical_YY`~~ **Resolved,
   by reading the actual generator code** (`generateSwitchedDeviceEntities`/
   `generateSwitchedDeviceAutomations`, generator.go:4020-4185) instead of reconstructing it from
   the user's own paraphrase — much simpler than that paraphrase suggested, and needs NO new
   naming convention at all:
   - `TSwitchedDeviceRelation{SelfEntity, Device, MainEntity}` — `Device` is XX (the armature
     light, first arg historically), `MainEntity` is YY (the switch, second arg). Both are
     resolved the ordinary way: whatever Conceptual.def positions them as, in whichever sphere.
   - Helper group (all:true) name = `light.physical/<MainEntity's own resolved path>/helper`
     (i.e. built off YY's OWN path, suffixed `_helper` — NOT off XX at all, and NOT the
     `<<XX>>_switched`/`<<YY>>_switcher` shape from the user's paraphrase). Members = `[deviceID,
     mainID]` = `[XX's own entity id, YY's own entity id]`, verbatim.
   - Self group (the FINAL user-facing entity, any-on) = `SelfEntity` (wherever Conceptual.def
     positions the outer combined entity, e.g. `entity light.social:main from core;`). Members =
     `[helperID, mainID]` — note XX itself is NOT a direct member of the self group, only via the
     helper.
   - The two follower automations are named off `MainEntity`'s own path + `_switched_on`/
     `_switched_off`, trigger on `MainEntity`'s state, act on `Device` (XX).
   - **This means NO new Logical.def-only entity is needed at all** — XX (the armature) keeps its
     ordinary Physical.def positioning (e.g. a second `entity light.physical:main_spots from
     core;` line alongside the `entity light.social:main from core;` one, exactly the
     already-established "position the same capability into two spheres" pattern used everywhere
     else in this file), and YY similarly gets an ordinary positioning of its `light.light_N`
     capability (e.g. `entity light.physical:main from light_2;`) under the SAME space. The
     Logical.def `is switched by` clause doesn't invent any new entity_id — it just tells the
     generator "when populating `TSwitchedDeviceRelation`, use XX's already-positioned entity as
     `Device` and YY's already-positioned entity as `MainEntity`, and self-entity is wherever THIS
     capability (`light.core`) got positioned in Conceptual.def."
   - **Full worked example, server_room** (corridor is the same shape, `light_1` instead of
     `light_2`):
     ```
     # Physical.def (already covered above)

     # Logical.def
     device sensors.server_room_main_spots with:
       light.core is switched by light.light_2 from switches.house_server_room_fuse_box_zwave_048;
     end;

     # Conceptual.def, space social:server_room
     device switches.house_server_room_fuse_box_zwave_048 with entity light.physical:main from light_2;
     device sensors.server_room_main_spots with:
       entity light.physical:main_spots from core;
       entity light.social:main from core;
     end;
     ```
     Resulting entity_ids (matching the living/dining byte-level ground truth shape exactly, just
     server_room-named):
     - `light.physical_house_server_room_main_spots` (XX/Device)
     - `light.physical_house_server_room_main` (YY/MainEntity)
     - `light.physical_house_server_room_main_helper` (all:true helper, members: the two above)
     - `light.social_house_server_room_main` (SelfEntity, the FINAL user-facing entity, any-on
       group of `[helper, light.physical_house_server_room_main]`)
     - `automation.house_server_room_main_switched_on`/`_switched_off`
   - No naming-error risk here the way living/dining had (that bug was specific to the "_dimmer"
     suffix the user hand-wrote for the dimmer's own name; server_room/corridor's XX is naturally
     `main_spots`, already correctly referenced everywhere including any future automations).
4. **zwavejs2mqtt node 15/48's exact topic spelling** — the home_id match (`0xd0204fb2` /
   `3491778482`) is now cross-confirmed via TWO different nodes' entity registries (15 and 48), so
   this is low-risk, but the literal `zwavejs2mqtt_0x........_node<N>` topic string itself (rather
   than the zwave_js-integration unique_id form used to infer it) is still worth a cheap live
   spot-check before writing Physical.def, same technique as nodes 42/54/55/64/67.

### Hiding the plumbing entities (2026-09-27, later still) — reuse existing mechanism, don't invent one

The user's further point: the OLD bash-based generator forced Conceptual.def to hand-declare
things that are pure implementation plumbing, never meant to be picked individually by a user —
specifically `entity light.physical:main no_collect with call providing ...;` (YY's own "physical"
sphere positioning) and `entity light.physical:main_dimmer no_collect;` (XX's own "physical" sphere
positioning). Only the FINAL combined `light.social:main` entity is genuinely conceptual and
deserves a hand-written line. **Clarified: not asking for a new mechanism — asking to confirm and
reuse whatever the codebase already has for this**, which sent me to check
`autoMaterializePhysicalCapability` before proposing anything.

**What already exists** (`autoMaterializePhysicalCapability`, `Conceptual_LogicalEntities.go:335-456`,
invoked from `resolveLogicalSiblingSourceEntityID`'s tier-4 fallback,
`Conceptual_LogicalEntities.go:458-476/510`, gated by `allowAutoMaterialize`): when a condition
tree or cross-device `"<label> from [physical] <device-id>"` reference points at a capability that
was NEVER explicitly positioned via a Conceptual.def `entity ...: from ...;` line, the generator
already synthesizes a stable `<domain>.physical_<device-id>_<label>` id for it on the spot —
exactly the "physical" + device-name + capability naming scheme the user described, already built,
already in production use (e.g. the `appliance.tv_living_room`/sonos_playbar sibling-node lookups
fixed earlier tonight fall back to exactly this). **This already covers the bulk of the ask** — XX
and YY's own "physical" sphere entities don't need hand-written Conceptual.def lines at all IF
their capability is only ever reached via this fallback path — this needs wiring into the NEW
`is switched by`/`is enabled by` construct's own `TSwitchedDeviceRelation.Device`/`.MainEntity`
population (today only wired for condition-tree/cross-device-reference resolution, not for
switched_device population) — an extension of an existing call site, not new machinery.

**The helper group needs nothing at all, today, already** — re-reading `generateSwitchedDeviceEntities`
(generator.go:4037-4038) shows `helperName := "light.physical/" + mainIdentity.Path + "/helper"` is
built as a plain string, with NO Conceptual.def declaration required for it even today, in the
current Go generator. The `light.physical_..._helper` line the user remembered needing was a
bash-era requirement only — already solved by the Go rewrite, nothing further to do here.

**Two real gaps found, confirmed NOT built anywhere today** (checked `bareCapabilityLabel`,
`namingSpacePath`'s compound-leaf suppression, `stripEntityOwnTrailingDomainSegment` — none of
these apply to a capability LABEL's own tail, only to a device's leaf/name path or an entity spec's
own path text):
1. `autoMaterializePhysicalCapability` always appends the label verbatim — a bare `"core"` label
   gets a trailing `..._core` in the auto-materialized id, unlike an explicit positioning (which
   omits `"core"` from the final id). Needs a `label == "core"` special case added, to keep
   auto-materialized ids consistent with what an explicit positioning would have produced.
2. No existing logic shortens a label's own tail when it duplicates the domain (e.g. label
   `light_2` under domain `light` → should shorten to just `_2`) — genuinely new code, though
   `namingSpacePath`'s own 2026-09-15 compound-leaf suppression (`Conceptual_DeviceEntities.go:241-290`)
   is a close PRECEDENT for the same spirit of rule (drop a domain-duplicating tail), applied to a
   different string (device leaf path there, capability label here) — same idea, new call site,
   not a wholly novel design.
- **Net effect once both are built**: Conceptual.def for server_room collapses from the OLD 3-line
  form (`main_spots` no_collect + `main` no_collect/providing + the switched_device block) down to
  exactly ONE line — `entity light.social:main from core;` under `sensors.server_room_main_spots`
  — with XX's own physical id and YY's `light.light_2`→auto-shortened-to-`_2` physical id both
  materialized automatically, never hand-declared.

### Migration/verification discipline

- This migration will NOT regenerate byte-identical output (unlike every prior migration this
  session) — it deliberately renames `light.physical_house_living_room_{living,dining}_main_dimmer`
  to `..._main`. Verification instead means: confirm the NEW generated entity_ids match what the
  (separately fixed) automations.yaml now expects, and that the two group YAMLs + two automation
  YAMLs per space still match `generateSwitchedDeviceEntities`/`Automations`'s existing shape,
  just via the new naming.
- Build the new Logical.def construct + generator wiring in isolation first: `go build/vet/test`
  clean before any `.def` edit, with unit tests covering the fully-resolved living-room worked
  example once question 2 above is settled.
- Migrate `social:living`+`social:dining` together (same physical device, per the user's own
  instruction — "best to migrate both of these switched_devices in one round"). Snapshot
  `hass/`+`coordinator/*.yaml` before, regenerate after, review the diff (not byte-identical, see
  above, but every OTHER file must be untouched).
- Fix the 4 wrong automation references in the same deploy pass, after the new names are settled.
- Retire HA's own `switch_as_x` helper for both endpoints only after the new entities are
  confirmed live and correct (manual, user-driven step).
- **Status correction (2026-09-27, later still) — this whole living/dining section was written
  BEFORE the pilot pair changed to server_room/corridor; it was stale.** Actual state of all 7
  `switched_device` sites:
  - **DONE** (regenerated, byte-diffed clean, NOT deployed): server_room (Physical.def ~1016),
    corridor (~1074) — node 48, the mechanism itself (`is switched by`, `autoMaterializedFullName`
    core-omission/domain-tail-shortening), and node 48's own liveness positioning (fixed after the
    user caught it missing).
  - **STILL PENDING** (5 remaining, none started): living room (~393) + dining (~436) — node 15,
    same physical device, migrate together per the user's own instruction "best to migrate both of
    these switched_devices in one round" — including the 4 wrong automation reference fixes
    (`light.physical_house_living_room_{dining,living}_main_dimmer` → `..._main`); workplace
    (~528/532-536); bathroom (~1190); a TV-room one (~1316).
- No deploy without explicit confirmation, as always.

<details>
<summary>Original parked investigation (2026-09-25), largely superseded above but kept for the
byte-level ground-truth facts (exact file names/shapes) not repeated above</summary>

- **The switch is one physical device, not two**: Fibaro FGS-223 "Double Switch 2" (device name
  "house/living_room/hallway_door_switch_box/fibaro", Z-Wave JS node 15, home_id 3491778482) has
  TWO endpoints — endpoint 1 drives `social:living`, endpoint 2 drives `social:dining`. Confirmed
  via HA main's own `core.entity_registry`/`core.device_registry`: both
  `switch.physical_house_living_room_living_main` (zwave_js unique_id
  `3491778482.15-37-1-currentValue`) and `switch.physical_house_living_room_dining_main`
  (`3491778482.15-37-2-currentValue`) share device_id `125c0a8f677a34b6668b0335b444e117`. (NOTE:
  this `3491778482` home_id was read from the zwave_js integration's OWN unique_id, not
  necessarily the same string as the `zwavejs2mqtt_0x........` topic-prefix form nodes
  33/42/54/55/64/67 use in Physical.def — still needs live MQTT confirmation, per Open questions
  above.)
- Today each endpoint is ALSO wrapped by HA's own "Switch as X" helper integration (platform
  `switch_as_x`, config-entry-based, entirely outside this DSL) into
  `light.physical_house_living_room_{living,dining}_main` — this is what "definition as
  switched_device" currently points at as its own "main" argument.
- **The dimmers are two ordinary, separate Zigbee2MQTT lights** — living:
  `zigbee2mqtt_0x0c2a6ffffe5bf9c2` (HA device name "house/living_room/living/main_dimmer"),
  dining: `zigbee2mqtt_0x18690afffe5982c0` ("house/living_room/dining/main_dimmer"). No domain
  coercion needed — a plain `light.core` discovery-kind device each.
- Byte-level ground truth for `social:living` (confirmed by reading the actual files):
  - `entities/light/social/light.social_house_living_room_living_main.yaml` — `platform: group`,
    NOT all (any-on), members `[light.physical_..._main_helper, light.physical_..._main]`.
  - `entities/light/physical/light.physical_house_living_room_living_main_helper.yaml` —
    `platform: group`, `all: true`, members `[..._main_dimmer, ..._main]`.
  - `customization/light/social/light.social_house_living_room_living_main.yaml` — icon +
    friendly_name only.
  - `automation/physical/automation.physical_house_living_room_living_main_switched_{on,off}.yaml`
    — on switch→'on'/'off', if dimmer is currently off/on, call `light.turn_on`/`light.turn_off`
    on the dimmer.
  `social:dining`'s own five files mirror this exactly, dining-named.
- `TSwitchedDeviceRelation` (administration.go:391-397): `SpaceName`/`SelfEntity`/`Device`
  (first arg)/`MainEntity` (second arg), populated purely by string normalization in
  parser.go:576-593 (`normalizeEntityFullName`, no capability/device-registry lookup at all
  today) — confirms the parked plan's read that today's mechanism is name-based, not
  capability-based; the new Logical.def construct needs to go through real capability resolution
  instead (via the absorb machinery), which is the actual architectural upgrade here.

</details>

## New, separate initiatives raised 2026-09-27 (late evening) -- NOT implemented, planning only

Raised while reviewing the "switched by" work just shipped for server_room/corridor. All four are
real, independent, and each has a wide blast radius across BOTH houses' existing `.def` files --
none should be implemented ad hoc mid-migration; each needs its own design/rollout pass.

### 1. Generic with-clause parser mechanism (the user's own recalled prior discussion)

Confirmed via research (not invented): there IS a real paper trail --
`Background/Arch2.md:4-5`/`Architecture.md:1268-1276` sketch a generic `WithClause(f)`/
`GroupClause(f)` grammar rule; `defined.go`'s `scanGroupClauseBlocks` explicitly formalizes that
sketch, but ONLY for wrapper *headers* (`integration X with:`, `<layer> layer with:`, `mqtt X:`,
`defaults:`/`for X:` -- 5 consumers, all list-bodied). The single-statement-body collapsing rule
("all with clauses should fold this way") was stated as an INSTRUCTION on 2026-09-25
(`integration_logical_parser.go:49`, `integration_hassbridge_parser.go:242`) and then
independently hand-implemented, file by file, ever since -- never lifted into one shared,
parameterized mechanism.

**Full inventory (13 hand-rolled with-clause consumers across 6 files)**:
- `integration_logical_parser.go` (5): device-level (`logicalDevicePattern`/OneLiner -- has
  one-liner), capability-level (`logicalCapabilityWithBlockPattern`/OneLiner -- has one-liner,
  added tonight), absorb block (no one-liner, inherently list-bodied), defined block (no
  one-liner, 5 shapes), derived block (no one-liner, 5 shapes).
- `integration_local_parser.go` (2): device-level `localWithDevicePattern` (no one-liner, 3
  shapes), `localCapabilityWithPattern`/switch.media (no one-liner, 1 shape).
- `Conceptual_DevicePositioning.go` (1): `deviceWithBlockHeaderPattern`/OneLiner -- has one-liner.
- `integration_hassbridge_parser.go` (2, inside its own already-generic `integration home_assistant
  X with:` wrapper): device-level `withDevicePattern` (no one-liner, ~10 shapes),
  `capabilityWithPattern`/OneLiner (has one-liner, AND already factors block+one-liner dispatch
  through one shared local closure `applyCapabilityWithBodyLine` -- the closest existing precedent
  for the generic mechanism, but scoped to this one file only).
- `integration_commandline_parser.go`/`integration_discovery_parser.go`/`integration_hosts_parser.go`/
  `integration_import_parser.go` (4): all list-bodied device-level with:-openers, no one-liner
  (arguably a different sub-case -- real multi-line lists, not single statements).
- `Physical_DerivedCapability.go`'s `derivedConditionOneLinerPattern` uses a THIRD, different
  strategy entirely: a pure textual pre-rewrite unwrapping the one-liner into the plain shape
  BEFORE any Physical.def scanner runs, rather than a block/one-liner regex pair.

4 of the 13 already have a one-liner; 9 don't. **This is a real, substantial refactor** (6 files,
13 call sites) if unified into one generic, parameterized mechanism (the user's own recalled
design: pass the "local" allowed-entries table/function into a shared with-clause parser) --
worth doing, but needs its own dedicated pass, not a drive-by fix.

### 2. Mandatory domain-qualification on EVERY capability reference (generalizing Note 3)

The user's own Note 3 ("always include the domain when referring to a capability in a 'CC from
YY' clause") should generalize into a HARD RULE, enforced as a parse ERROR when omitted, applying
to (at least) these places, none of which enforce it today:
- Conceptual.def's `entity <spec> from <device-id> <capability>;` third token -- TODAY
  domain-optional (`Conceptual_DeviceCapabilityEntities.go`'s own doc comment: "may carry a domain
  prefix... stripped before lookup"). Concrete example flagged live: this session's own new
  `device sensors.server_room_main_spots with entity light.social:main from core;` should have been
  REJECTED (or required to read `... from light.core;`) under this rule -- it wasn't, because the
  rule doesn't exist yet. This one line is illustrative of a LARGE population of existing lines in
  both houses that would need auditing/fixing once this becomes mandatory (e.g. the button/tap
  declarations below).
- The button/tap declaration shape, e.g. (Junglinster, `space social:living`):
  ```
  device buttons.house_living_room_living_table_tap as table/tap with:
    entity event.physical: from core;
    entity sensor.physical: from core_sensor;
  end;
  ```
  -- domain missing on `core`/`core_sensor` here too (should read `event.core`/`event.core_sensor`
  once mandatory, or renamed per item 3 below).
- A jinja template call's own `{ <source> }` argument, e.g.:
  ```
  derived binary_sensor.battery_alert with:
    condition jinja ${int_less_then}(20) { battery_level };
  end;
  ```
  should become `{ sensor.battery_level }` -- domain-qualifying the jinja source token too.
- Logical.def's new `is switched by`/`is enabled by` clauses ALREADY enforce this (the grammar
  requires `<domain>.<label>` on both sides by construction) -- the one place this rule is already
  real, everywhere else it's still optional/silently accepted.

**Scope warning**: making this a hard error would immediately flag a large number of ALREADY-LIVE,
already-deployed `.def` lines in both houses that omit the domain today -- this needs an audit
pass (how many lines would start erroring) before flipping it from optional to mandatory, not a
silent grammar tightening.

### 3. Capability renaming for tap/button-style devices: "core"/"core_sensor" -> "event.pressed"/
"event.pressed_as_sensor"

Applies to taps, Moes buttons, Signify dimmers, and Philips dimmers, across BOTH houses -- renaming
the generic `core`/`core_sensor` capability labels to the more semantically descriptive
`pressed`/`pressed_as_sensor` (domain `event`), e.g. the table/tap example above would become:
```
device buttons.house_living_room_living_table_tap as table/tap with:
  entity event.physical: from event.pressed;
  entity sensor.physical: from event.pressed_as_sensor;
end;
```
(assuming item 2's mandatory domain-qualification lands too). This is a real, multi-device,
both-houses renaming migration -- Physical.def's own capability LABEL changes for every such
device, plus every Conceptual.def positioning referencing it by the old bare name. Needs its own
inventory pass (how many devices/lines are affected) before touching anything.

### Benchmark snapshot taken (2026-09-27, late evening) -- before any of the 3 items above start

No deploy possible right now (Erik doesn't want to disturb his wife with a restart), so the user's
own plan: use the CURRENT generated YAML (both houses, as of right after the node 48 liveness fix
above) as the "before" reference for items 1-3, and separately cross-check against the user's own
collated chat notes (pasted separately) on what was actually agreed/worked on, specifically to
catch anything dropped across a context compression.

Snapshotted to
`/private/tmp/claude-501/-Users-erikproper-Nextcloud-Systems-go-src-homeassistant/02e0cbb6-56e4-4de4-b032-b8a76cfdca1f/scratchpad/benchmark_2026-09-27/`:
`junglinster_hass/`, `junglinster_coordinator/`, `junglinster_definitions/`, `vienna_hass/`,
`vienna_coordinator/`, `vienna_definitions/` -- both houses' full generated output AND source
`.def` files, as they stood at that moment (Junglinster already includes tonight's switched_by
work + the node 48 liveness fix; Vienna untouched all session). Use this as the diff baseline once
items 1-3 start landing -- and note the user's own instruction: the collated chat notes describe
what should have been WORKED ON, which may differ from what should exist today (things may have
been deliberately decided differently mid-conversation) -- don't treat the notes as a literal
target to match, cross-reference them against actual decisions made in this conversation.

### Item 2 (mandatory domain-qualification) -- IMPLEMENTED 2026-09-27, later still, both houses

The user's own explicit instruction ("stop logging, start implementing") — pushed through despite
the scope warning above, and the measured blast radius WAS as large as warned (see below), but it
is now fully done and verified, not just designed:

- **Grammar change** (`Conceptual_DeviceCapabilityEntities.go`,
  `registerDeviceCapabilityEntityLinkAllowingHiddenCore`): an explicit `entity X from Y Z;`
  capability reference with no domain prefix on `Z` is now a hard parse error (rejected, not a
  silent strip-and-continue) — scoped to the explicit form only, never the no-rename shorthand
  (`capabilityIsInferred`), matching Note 3's own "CC from YY clause" framing. The error message
  itself looks up the capability's OWN real declared domain (`lookupCapabilityDomain`, new helper,
  mirrors `deviceHasCapability`'s kind-by-kind scan) and suggests the exact fix, e.g. `write
  "sensor.cpu/load"` — turned a blind "add a domain prefix" into something mechanically fixable.
- **Real bug found and fixed along the way**: the Logical.def absorb mechanism's own recursive
  dispatch (`registerLogicalAbsorbedCapability`) passed `capability.AbsorbedFromCapability` bare
  by design (the parser's own `bareCapabilityName` strips any domain the RHS text happens to
  carry) — this now trips the new mandatory check on its own internal plumbing. Fixed by
  domain-qualifying the recursive call using `capability.Domain` (already captured from the
  absorb block's own `<domain>.<label>: <bare-capability>;` line) — no new field needed, no
  behaviour change for any absorbed capability.
- **Measured blast radius, exactly as scope-warned**: 177 real violations in Junglinster's
  Conceptual.def, 79 in Vienna's (both fixed, see below), plus 35 existing Go tests broke (all
  fixed, full suite green).
- **Both houses fixed and verified byte-identical to the pre-change benchmark**
  (`benchmark_2026-09-27/`, see above): every flagged line's capability reference now carries its
  real, correct domain prefix (looked up per-line from the actual `lookupCapabilityDomain`
  suggestion, applied by exact file:line, not guessed). `diff -rq` against the benchmark's
  `junglinster_hass`/`vienna_hass` — **zero differences**, confirming this is a pure syntax
  tightening with no semantic/output change, exactly as expected.
- **A second, related cleanup landed in the same pass** (the user's own "fritz_box" example,
  raised while this was in flight): collapsed every `entity X from Y;` line, in both houses, where
  Y (domain-stripped) already exactly equals X's own path tail — 99 lines in Junglinster, 2 in
  Vienna — to the bare no-rename shorthand `entity X;` (`capabilityIsInferred` already handles
  these correctly, no grammar change needed, purely a `.def`-text cleanup). Also byte-identical to
  benchmark. (One real scripting mistake happened here: an over-eager first regex pass corrupted
  several unrelated lines before this was caught via the generator's own "undeclared entity"
  warnings and fixed properly — a reminder that even a "mechanical" `.def`-text script needs the
  same byte-diff verification discipline as everything else.)
- All work is committed to the SmartLiving git repos (both houses) but NOT deployed to either
  live coordinator/HA instance — deploy still requires explicit confirmation, unchanged.

### Item 3 (tap/dimmer capability rename) -- DONE 2026-09-27, later still, both houses

`core`/`core_sensor` -> `pressed`/`pressed_as_sensor` (domain unchanged: `event`/`sensor`), plus
unified Vienna's two OTHER naming variants for the same thing found along the way
(`action_as_event`/`action_as_sensor` on `hallway_bedroom_door_moes`, and bare `action` with no
sensor counterpart on `living_room_signify_dimmer`) — all now read `event.pressed`/
`sensor.pressed_as_sensor` consistently. Applied to both Physical.def (capability declarations)
and Conceptual.def (references) in both houses. Regenerated + byte-diffed clean against the
benchmark (a capability LABEL rename never affects the generated entity_id, which comes from the
entity's own position/spec, not the source capability's name — confirmed, not just assumed).

**Real, separate finding surfaced along the way, NOT silently folded into this rename**:
Junglinster's 4 Philips dimmer devices (`sensors.*_philips_dimmer`) declare NO `action`/`core`
capability in Physical.def at all -- the live `event.physical_house_*_philips_dimmer_action`
entities this session's own earlier tap-migration work touched are undeclared passthrough
entities (existing on the MQTT bus/HA registry without any DSL involvement), not DSL-generated
ones. Bringing them into the DSL properly is a genuinely different task from this rename (adding
a whole new capability declaration, not renaming an existing one) -- flagged, not started.

### Item 1 (generic with-clause mechanism) -- CORE BUILT + FIRST MIGRATION DONE, 12 of 13 remain

Confirmed via research (not invented): `defined.go`'s `scanGroupClauseBlocks` already generalizes
Arch2.md's `WithClause(f)` sketch, but ONLY at the wrapper-HEADER level (block open/close +
raw body extraction) -- it does NOT help with what happens to each extracted body STATEMENT next,
which is where every one of the 13 consumers still hand-rolls its own "try each recognised
single-statement shape" dispatch, once for the block form and again (duplicated) for the
one-liner fold. `integration_hassbridge_parser.go`'s own `applyCapabilityWithBodyLine` closure was
the closest real precedent (already shares block+one-liner dispatch, just scoped to one file/one
accumulator type).

**Built**: `with_clause.go` (NEW file) -- `TWithClauseStatement[T]`/`ApplyWithClauseBodyLine[T]`,
a small Go generic (this codebase's FIRST use of one, kept deliberately minimal) generalizing
`applyCapabilityWithBodyLine`'s exact idea across every accumulator type in the DSL: define the
list of recognised single-statement shapes ONCE, call the same dispatch from both the block-body
loop and the one-liner fold. Doesn't touch `scanGroupClauseBlocks` at all (still the right tool
for header+block-body extraction) -- purely about what happens to a body line/one-liner statement
next.

**Migrated (5 of 13)**, each its own `go build/vet/test` + regenerate-both-houses-and-byte-diff
pass, all clean against the benchmark:

1. `integration_logical_parser.go`'s capability-level `with:` block (enabler/delay_off/
   no_play_input -- the exact dispatch I'd hand-duplicated between block and one-liner forms
   earlier tonight) now uses `logicalCapabilityBodyStatements []TWithClauseStatement[TLogicalCapability]`
   + `ApplyWithClauseBodyLine`, called identically from both forms. `logicalCapabilityBodyPattern`
   (the old combined enabler/delay_off regex) removed as dead code once both call sites stopped
   using it. "dependency on" (repeatable) stays a separate explicit check at each call site --
   doesn't fit the generic dispatcher's "first match wins, once" model cleanly.
2. `integration_logical_parser.go`'s `defined` block (minimum/maximum/step/icon/units) -- same
   treatment, `logicalDefinedFieldStatements`, PLUS a genuinely new one-liner
   (`logicalDefinedOneLinerPattern`) this block never had before (rarely useful given every real
   case needs all five fields, but free and consistent with every other consumer). Replaced 5 loose
   `currentDefinedXxx` string variables with one `currentDefinedCap TLogicalCapability` accumulator.
3. `integration_local_parser.go`'s device-level block (bare/is-available/media-switch-literal
   capability declarations) -- extracted the existing single-pattern-with-internal-branching logic
   into a shared `applyLocalCapabilityLine` function (not the generic `TWithClauseStatement[T]`
   machinery itself, which doesn't fit a "one complex pattern" shape as cleanly as a "list of simple
   patterns" shape -- but the SAME "block form and one-liner call the same logic" principle), and
   gave it a genuinely NEW one-liner (`localWithDeviceOneLinerPattern`) it never had before, e.g.
   `device appliance.x with camera.core;`.
4. `integration_local_parser.go`'s `switch.media` capability's own `with:` block (`no_play_input`)
   -- `localMediaSwitchCapabilityStatements` + a genuinely new one-liner
   (`localCapabilityWithOneLinerPattern`), e.g. `switch.media media_player.X with no_play_input
   "TV";` instead of requiring the full block.
5. `integration_hassbridge_parser.go`'s capability-level `with:` block -- retired the file's own
   original `applyCapabilityWithBodyLine` closure (the precedent the whole mechanism generalizes
   from) in favour of a locally-built `capabilityWithBodyStatements []TWithClauseStatement[THassBridgeCapability]`
   (local, not package-level, since two entries close over `instance`, this function's own
   parameter) + `ApplyWithClauseBodyLine`. Zero behaviour change -- this file's own extensive
   existing hassbridge test coverage passed unmodified, confirming the retrofit is byte-for-byte
   equivalent to the closure it replaced.

**Real, unrelated bug found and fixed while regenerating to verify this migration** (a THIRD
instance of the exact non-determinism class found twice earlier tonight -- map iteration order
randomised per Go process, break-on-first-match instead of collect-all-sort-pick-first): both
`registerDiscoveryAvailabilityEntityLink` and `registerDiscoveryDerivedEntityLink`
(Conceptual_DiscoveryEntities.go) had the identical bug in their own sibling reverse-lookup.
Confirmed live: Junglinster's server_room/corridor "switched by" pilot (this session's own earlier
work) put node 48's own `derived binary_sensor.node with condition light.light_1 is available;`
into exactly this non-deterministic path (two genuine positionings of the same "light_1" leaf --
the switched-by mechanism's own auto-materialized entity, and the explicit corridor positioning
kept for an unrelated `follows` reference) -- caught it by re-running `./generate` 6 times with
zero `.def` changes between runs and seeing 2 different outputs. Fixed identically to the earlier
two instances; regression tests added
(`TestResolveLogicalSiblingSourceEntityIDDeterministicWithMultipleDiscoveryMatches`,
`TestDiscoveryAvailabilityEntityLinkDeterministicWithMultipleSiblingPositionings` -- the latter
end-to-end, 20 iterations). Confirmed deterministic across 6+ fresh `./generate` runs afterward.
Both houses regenerated and byte-diffed clean against the benchmark.

**Migrated (6 of 13) -- #6, 2026-09-28 (Phase 2's first item)**: `integration_logical_parser.go`
device-level (`enabler`/`switched by` via the new `logicalDeviceBodyStatements` +
`ApplyWithClauseBodyLine`, matching `logicalCapabilityBodyStatements`' own precedent one level up;
`dependency on` kept as its own explicit repeatable check, same reasoning as the capability-level
consumer). One real subtlety caught before it became a bug: the one-liner dispatch's own original
`switch` checked `switched-by` BEFORE the permissive bare-capability pattern (which would otherwise
swallow a switched-by line as a literal nonsensical capability value) -- a first-draft reordering
of this refactor accidentally flipped that precedence; caught by re-reading the original code's own
comment before regenerating, fixed before it ever reached a `go build`. Regenerated both houses,
zero warnings; full test suite green; Vienna byte-diffed BYTE-IDENTICAL against the original
2026-09-27 benchmark (Vienna's own `dependency on`/`enabler` usage is extensive, so this is a real,
meaningful confirmation of zero behaviour change, not a no-op check).

**Remaining (7 of 13, not started)** -- roughly ordered easiest/safest first:
- `Conceptual_DevicePositioning.go` device positioning (1 shape) -- already has a one-liner via
  its own dedicated regex pair. Lowest priority to touch at all.
- `integration_logical_parser.go` absorb block (2 shapes, inherently list-bodied) -- may not
  need/benefit from a one-liner at all; worth a deliberate "no" rather than forcing one.
- `integration_logical_parser.go` `derived` block (5 shapes, currently NO one-liner) -- a real
  gap, but its `condition`/`value` fields accumulate across multiple physical lines BEFORE parsing
  (`derivedExprBuffer`/`tryCloseDerivedExpr`), a genuinely different shape than a plain
  single-statement list. Needs its own design pass, not a drop-in swap like the five already done.
- `integration_hassbridge_parser.go` device-level (~10 shapes, no one-liner) -- **stopped here
  deliberately, 2026-09-28 (past midnight)**: this is the biggest potential win, but several of
  its shapes (metadata/value-map/attribute for an ALREADY-DECLARED capability) can't sensibly
  stand alone as a one-liner's sole statement -- there'd be no capability yet for them to attach
  to on a fresh one-liner device. Reducing this to a clean one-liner needs real thought about
  WHICH subset of the ~10 shapes a device-level one-liner should even accept, not a mechanical
  swap like the five already done -- exactly the kind of rushed-mechanical-change risk this
  session's own earlier collapse-script mistake was a lesson in. Left for a fresh pass.
- 4 list-bodied device-level headers (commandline/discovery/hosts/import) -- no one-liner concept
  applies to these at all (genuinely multi-line lists). Lowest priority, possibly out of scope
  entirely.

Each remaining migration needs the same discipline as the five already done: migrate, `go
build/vet/test`, regenerate both houses, byte-diff against the benchmark, before moving to the
next one -- not a single big-bang sweep.

## Post-mortem + real fixes, server_room/corridor pilot deploy (2026-09-28)

Deploying and live-testing the server_room/corridor pilot (the first real "is switched by" site to
actually go live) surfaced two real generator bugs, both now fixed, tested, and deployed:

1. **Discovery-entity collision**: the SelfEntity positioning (e.g. `entity light.social:main from
   light.core;`) was ALSO independently registering an ordinary raw discovery entity under the same
   target name as the switched-by mechanism's own generated group -- two things claiming the same
   entity_id/MQTT topic. Fixed: `switchedBySuppressesEntity`
   (Conceptual_DeviceCapabilityEntities.go/logical_switched_by.go) suppresses the raw registration
   for exactly this positioning, while still declaring the name into the ordinary
   "declared entity" bookkeeping (`RegisterDiscoveryImpliedEntity`) so `follows`/`light on:`
   directive checks don't flag it as undeclared.
2. **Auto-materialization ignored an existing explicit positioning**: the user's own correction --
   an explicit Conceptual.def positioning of a capability ALWAYS takes precedence over the "hidden"
   name auto-materialized when that same capability is ALSO needed purely internally (as YY, the
   switch source, e.g. corridor's kept `light.physical:main from light.light_1` -- now renamed, see
   below). Fixed: `resolveSwitchedByEndpoint` (logical_switched_by.go) checks
   `administration.ConceptualUseBySource` (keyed identically, `deviceID+"!"+label`, to every other
   capability-registration call site) FIRST, only auto-materializing as a fallback once the
   deferred-retry pass (`finalAttempt`) confirms no explicit positioning exists anywhere in the
   file. (First cut of this fix didn't gate on `finalAttempt` and silently locked in the
   auto-materialized choice before the explicit positioning -- appearing LATER in the file -- had
   even been parsed; the codebase's file is read whole before interpretation, so this was a bug in
   not participating in the established order-independent deferred-retry protocol, not a "line
   order" issue as such.)
3. **Naming convention applied**: "physical" sphere is reserved for purely-internal/auto-materialized
   entities now -- an entity that's genuinely used at the conceptual level (like corridor's kept
   switch positioning, referenced by `follows`) belongs in "social" instead. Renamed
   `light.physical:main` (corridor's switch) to `light.social:main/switcher` in Conceptual.def, and
   the `follows` line referencing it.

All three fixed, tested (new regression tests: `TestSwitchedBySuppressesCompetingDiscoveryEntity`,
`TestSwitchedByReusesExplicitPhysicalPositioning`), regenerated + byte-diffed clean against Vienna
(zero side effects there), redeployed to Junglinster (HA + coordinator), HA Core restarted to pick
up the corrected group YAML.

**CONFIRMED LIVE 2026-09-28, user-tested via the `_old` raw zwave switch entities (which flip the
Z-Wave modules directly) for both endpoints**: `light.social_house_server_room_main` and
`light.social_house_corridor_main` both correctly follow the physical switch on/off in both
directions (UI toggle -> physical relay; physical relay -> UI state), matching the pre-migration
`switched_device` behaviour exactly. The server_room/corridor pilot is DONE end-to-end: mechanism
built, generator bugs found+fixed, deployed, and now live-verified -- the template for the 5
remaining sites (living room+dining/node 15, workplace, bathroom, TV-room).

One extra, corridor-specific snag worth remembering for the remaining sites: corridor's switch had
a PRE-EXISTING sticky MQTT entity_id from before this rename (`light.physical_house_corridor_main_2`,
predating today's work) -- fixing the generator alone wasn't enough to free it. Needed, in order:
(1) delete the live registry entry via the `config/entity_registry/remove` websocket call, (2)
discover HA moves it to a `deleted_entities` tombstone that ALSO blocks recreation, requiring the
established stop-core/backup/`jq`-edit/verify/start-core registry-surgery procedure to remove the
tombstone too, (3) discover that even after that, the coordinator itself won't re-publish a topic
it believes is unchanged -- needed a direct, hand-published retained MQTT message using the exact
payload from the coordinator's own `discovery_topics.json` manifest (first attempt accidentally
double-JSON-encoded the payload, since that manifest stores it as a JSON-encoded STRING value, not
a nested object -- fixed by writing the string's raw content, not `json.dump`-ing it again). None of
the OTHER remaining switched_device sites are known to have a similarly pre-existing sticky
entity_id (server_room's own endpoint had none, went live cleanly on the first restart) -- but this
whole sequence is now a known playbook if one comes up again.

## Deferred: "follows" + "is switched by" interplay (raised 2026-09-28, after Junglinster's migration)

Right now, a switched-by device needs TWO separate `follows` lines when something else needs to
track it -- one for the combined "social" entity (`light.social:main`) and one for the raw switch
("main/switcher") -- because `follows` needs to react to whichever underlying entity actually
changes state first/authoritatively. The user wants this made agnostic: a single `follows` rule
should work correctly regardless of whether its target is a plain entity or a switched-by combined
one, without the DSL author needing to know about or duplicate the internal plumbing. Not designed
yet -- revisit once the rest of the Junglinster switched_by migration (living/dining, workplace,
bathroom, TV-room) is done, since by then every real shape of this interplay will be known.

## Full work-queue ordering, as confirmed by the user 2026-09-28 -- SUPERSEDES the stale 7-site
## `switched_device` list above and the ad hoc "everything else queued" ordering from earlier notes

The user gave an explicit end-to-end sequencing for all remaining Junglinster/Vienna DSL work.
Phases run in order; don't start a later phase before the earlier one is genuinely done.

### Phase 1 -- Per-Fibaro-(double-)switch sweep (current phase)

Work grouped by PHYSICAL Fibaro node, not by room -- since a double-switch Fibaro (FGS-223) always
drives two rooms/endpoints at once, migrating "per Fibaro" naturally covers the old room-based
"living/dining + workplace + bathroom + TV-room" list in one pass. For each node: migrate every
switched_device/`providing`-macro/other zwave-sourced entity that shares it, using the node
48/corridor playbook (Physical.def declaration -> Logical.def `is switched by` where applicable ->
Conceptual.def positioning -> byte-diff -> deploy -> restart -> live-verify, watching for the
sticky-entity-id trap if the node already has a live pre-existing entity under an old name).

**Per-device workflow, formalized by the user 2026-09-28 (from the corridor sticky-entity-id
lesson)** -- FOUR steps, per node, in this exact order, not skipped or reordered:
1. **Pre-migration (user)**: Claude asks the user to rename the raw zwave entities to `_old` and
   remove the `switch_as_x` "Show as light"/"Show as fan" alias config-entries for THIS node,
   before any DSL work starts. (Node 15: confirmed done by the user 2026-09-28.)
2. **Migrate (Claude)**: Physical.def/Logical.def/Conceptual.def changes, `go build/vet/test`,
   regenerate both houses, byte-diff against the benchmark. ALSO (added 2026-09-28, per the user):
   before deploying, grep the LIVE, hand-maintained `/config/automations.yaml`/`scripts.yaml` (never
   DSL-generated, so invisible to any of the generator's own checks) for every entity_id this node's
   migration touches or renames -- catches broken dependencies the DSL can't see on its own. Node
   15's own check found exactly the 4 already-known `_main_dimmer`-suffixed references (needing the
   suffix dropped) and nothing else broken. Then deploy, restart if needed.
3. **Test (together)**: live verification, the user toggling the `_old` entities to confirm the
   new switched entity follows correctly in both directions.
4. **Clean unneeded zwave discovery items (Claude)**: once testing confirms the node works,
   disable/remove the zwave_js integration's own auto-discovered-but-unused entities for that node
   (parameter/configuration controls nobody wants exposed, unused endpoints) -- the concrete
   per-device execution of this phase's own cross-check #2 (excluding unused Fibaro
   capabilities/endpoints from HA discovery), done once per node rather than as one big sweep at
   the end.

Corridor's own sticky-entity-id problem (this session) came from exactly a step-1/step-2
ordering mismatch -- a pre-existing live entity under the old name, not yet cleaned up, blocked the
new one from registering cleanly. Following this order strictly should mean corridor's own
registry-surgery detour is the exception, not the rule -- nodes where step 1 happens cleanly before
step 2 should go live on the first restart, like server_room did.

**Live zwave_js inventory** (pulled 2026-09-28 from Junglinster's own `core.device_registry`,
home_id `3491778482`/`0xd0204fb2` unless noted -- this is the authoritative source, not the stale
notes in the "Original parked investigation" section above):

- **Excluded, per the user's own instruction**: node 1 (`NC-ZWA-9734`, the Z-Wave JS UI's own
  antenna/controller, not a switch at all), node 9 (`FGS-213`, spare, still default-named "09"),
  node 14 (`FGS-223`, spare, still default-named "14").
- **FGS-223 (double switch, 2 endpoints) -- the "per Fibaro double switch" sweep**. Cross-checked
  against the actual current Conceptual.def content (2026-09-28, corrected from an earlier,
  overly-pessimistic first pass through this file) -- "modern" below means a plain `entity X from
  Y capability;`/`with entity X from Y;` positioning (no `providing`/`switched_device` macro), i.e.
  already off the legacy bash-era mechanism, though NOT necessarily yet checked for whether it
  actually needs a switched-by light+relay combo at all:
  - Node 48 (server_room/corridor) -- **DONE, confirmed live 2026-09-28** (this session's pilot,
    the actual `is switched by` combo mechanism).
  - Node 44 -- (FGR-223 not FGS-223, see below) **DONE**.
  - Node 42 (`garage/fuse_box/zwave/042`) -- **already modern** (Conceptual.def:217/625, `entity
    light.social:main from light.core_1`/`light.core_2`, two positionings in two spaces) -- no
    `providing`, no pending combo need spotted; low-priority double-check only, not a real
    migration task.
  - Node 54 (`house/server_room/fuse_box/zwave/054`) -- **already modern** (~77/101, `light.core_1`
    front / `light.core_2` rear) -- same low-priority double-check as 42.
  - Node 55 (`house/server_room/fuse_box/zwave/055`) -- **already modern** (~102, `switch.core_1`
    only, single capability used) -- a genuine switched outlet, not a light armature; no combo
    applicable at all.
  - Node 64 (`house/server_room/fuse_box/zwave/064`) -- **already modern** (~37/62, `light.core_1`
    carport / `light.core_2` main) -- same low-priority double-check as 42/54.
  - Node 67 (`garage/fuse_box/zwave/067`) -- **already modern** (~32-35, `switch.core_1` sockets /
    `light.core_2` front_door) -- same low-priority double-check.
  - Node 15 (`house/living_room/hallway_door_switch_box/zwave_015`) -- **DONE, confirmed live
    2026-09-28** (living+dining, all 4 workflow steps complete): Physical.def declares
    `switches.house_living_room_hallway_door_switch_box_zwave_015` (light_1=living, light_2=dining)
    plus two new dimmer devices (`sensors.living_room_{living,dining}_main_dimmer`); Logical.def
    "is switched by" for both; XX (the dimmer) ALSO kept an explicit "physical:main" positioning
    (no "_dimmer" suffix) for 2 pre-existing non-DSL automations, YY's light_1 similarly kept for
    the node's own liveness condition -- a genuinely NEW scenario (a device's capability serving as
    BOTH the SelfEntity trigger and a separate explicit physical positioning) that exposed a new
    generator bug (sourceKey collision, fixed with a "!selfentity" discriminator, mirroring
    autoMaterializePhysicalCapability's own "!physical" one -- see logical_switched_by.go). The 4
    manual automations.yaml dim-up/dim-down references fixed live (dropped "_dimmer" suffix).
    10 unneeded zwave_js entities disabled (protection state selects, power/energy monitoring,
    scene-press events) after user confirmation; node status/ping/firmware/alarm sensors kept.
  - Node 57 (`house/kitchen/hallway_door/power_socket/fibaro`, NOTE different home_id
    `3615616288` -- a SEPARATE Z-Wave network/controller, confirm before assuming the same
    `zwavejs2mqtt_0xd0204fb2_` topic prefix convention applies) -- **NOT migrated**, still
    `providing`-sourced (~572 fan, ~578 light), both off this one node.
  - Node 71 (`house/server_room/fuse_box/zwave/071`) -- **NOT migrated**, still `providing`/
    `switched_device` (~1201/1204) -- AND currently mis-wired: the code references node 029, but
    Conceptual.def's own comment says "Should be 071" -- fix the node number as part of migrating.
  - Node 52 (`house/server_room/fuse_box/zwave/052`) -- **NOT migrated**, still `providing`/
    `switched_device` (~1331/1334) -- AND mis-wired the same way: code references node 023,
    comment says "Should be 052" -- fix as part of migrating.
  - Node 58 (registry name `garage/fuse_box/zwave/058`, but per the user 2026-09-28 its own two
    endpoints actually drive: kitchen WORKPLACE lights + WC fan -- the registry name is a stale/
    misleading naming-convention artifact, not its real function) -- **genuinely undeclared**: zero
    hits anywhere in Conceptual.def -- needs a fresh from-scratch Physical.def+Conceptual.def
    declaration, not a migration of something existing. This is very likely the SAME site as
    Physical.def's own 2026-09-22 comment (~1140) for kitchen's mystery `light.social:main`
    `light_device` macro, flagged back then as "NOT migrated... device doesn't currently appear in
    the live Zigbee2MQTT passthrough list" -- that check was for a ZIGBEE device, but this is a
    Z-WAVE node, so of course it never showed up there; the WC fan half likely explains kitchen's
    own separate `fan.social` `providing` site too (~572, currently attributed to node 57 instead --
    worth double-checking node 57 vs 58 aren't being conflated once this one is actually declared).
    This is the plan's own "workplace" site from the original 4-item room list.
- **FGS-213 (single switch, 1 endpoint) -- the "remaining zwave devices" sweep, after all FGS-223s
  above**:
  - Node 33 (garage) -- **already modern** (~110/153, `light.core` single capability) -- low-priority
    double-check only.
  - Node 10 (shower_room switch_box, ~946), node 11 (master_bedroom door_switch_box, ~886), node 12
    (sarah_bedroom door_switch_box, ~765 -- located, per the user 2026-09-28: `entity
    light.social:main with call providing :door_switch_box/fibaro;`), node 13 (david_bedroom_door
    switch_box, ~813), node 17 (bathroom/bathtub/relais_box, ~1210) -- all **NOT migrated**, still
    on `providing`.
- **FGR-223 (roller shutter, different domain entirely)**: node 44 (`house/server_room/fuse_box/
  zwave/044`) -- **already migrated (confirmed by the user 2026-09-28)**, Conceptual.def line 96:
  `device switches.server_room_zwave_044 as cover with: entity binary_sensor.infrastructural:node;
  entity cover.social:awnings from cover.core; end;` -- a real kind-2 discovery positioning, `cover`
  domain, single capability, no `light`/`switch` pairing needed at all (a roller shutter has no
  separate "armature" to combine). Nothing left to do here -- don't re-touch it.

**Two cross-checks the user asked for, to run once the sweep above is otherwise complete**:
1. Confirm EVERY real zwave_js device (per `core.device_registry`, the list above) has SOME
   Physical.def/Conceptual.def representation -- except the 3 explicitly excluded (antenna, spares
   09/14). Catches anything undeclared that this inventory pass missed.
2. Confirm every Fibaro's UNUSED endpoint/capability (a double switch where only one endpoint is
   actually wired to something real) is excluded from HA discovery -- i.e., Physical.def should
   never declare a capability for an endpoint nothing actually uses, so the coordinator never
   publishes discovery for it in the first place. Needs a per-node check against what's physically
   wired (can't be inferred from the DSL alone).

**Cross-check #2 applied retroactively 2026-09-28** to all 8 already-migrated nodes besides 15
(33, 42, 44, 48, 54, 55, 64, 67) plus the 2 spares (9, 14), per the user's own request (not waiting
for the sweep to finish) -- pulled `core.device_registry`/`core.entity_registry` directly, per-node
active (non-`disabled_by`) entity lists, classified by keep-pattern (node_status/ping/overheat/
over_current/system_hardware_failure/unknown/firmware/`_old` switches) vs disable-pattern
(everything else: scene-press events, protection-state selects, power/energy monitoring
sensors+reset buttons). **76 entities disabled** across the 8 nodes via the `config/
entity_registry/update` websocket call (same mechanism as node 15's own cleanup) -- zero left
active beyond the keep-list. One genuine "unused endpoint" catch: node 55's own second endpoint
(`switch.double_switch_2_2_2`, never declared in Physical.def -- only `switch.core_1` sockets is)
was included and disabled, the first live example of cross-check #2's own stated purpose. The 2
spares (9, 14) needed NO action -- already minimal (only node_status/ping active), confirming
they're genuinely untouched/unpaired-for-real-use, matching their "spare" status. This cross-check
is now DONE for every node migrated so far; re-run it (same script shape) after each future node in
Phase 1's own sweep, as part of that node's own step 4, rather than batching it again at the end.

### Phase 2 -- "Everything else queued" (from the earlier "New, separate initiatives" section above)

In order: the remaining 8 with-clause consumers (item 1), the `is enabled by` grammar + mechanism
(design already confirmed above, not yet implemented), jinja source domain-qualification, Vienna's
washing_machine reconciliation, Junglinster's 4 undeclared Philips dimmer `action` capabilities.

### Phase 3 -- Resume the file's OWN embedded migration queue

Conceptual.def itself carries the user's own TODO, written directly in the file at the "#### HERE"
marker (line 391, right after living/dining's own switched_device block -- covered by Phase 1
above): "Then all remaining light_devices" / "Then all remaining enablers" / "Then all remaining
the media_players" / "Then continue the (remaining) migration sequentially from here." In order:

1. **Remaining `call light_device` sites** (legacy bash macro, already retired from the generator --
   PROJECT.md 1b/`project_macro_cleanup_after_junglinster` -- these are the last live call sites
   blocking its removal): `light.social:bloom_left` (~409) and kitchen's `light.social:main` (~522)
   are BOTH already flagged in Physical.def's own 2026-09-22 comments as blocked -- neither device
   currently appears in the live Zigbee2MQTT passthrough list at all, needs the USER's own check
   (possibly offline/unpaired/renamed) before either can be migrated -- don't attempt to guess.
   `light.social:left`/`light.social:right` (~871-872, master_bedroom/bed) -- not yet investigated
   this session, check live passthrough status first.
2. **Remaining plain `enabler`/`dependency on` sites** (once Phase 2's `is enabled by` mechanism is
   built): Junglinster's `sonos_garage`, `appletv_living_room`, `sonos_playbar_livingroom` (the
   stale duplicate positioning flagged earlier in this plan, never resolved -- confirm with the user
   before touching); Vienna's `apple_tv`, `tv`, `sonos`, `sonos_roam`.
3. **Remaining media_player sites** (the `switch.media` local-kind relocation, part A of this
   plan's very first section) -- whichever of the original 8 `switch.media` sites aren't already
   covered by item 2's enabler work.
4. **Sequential continuation from line 391 onward** -- once 1-3 above are done, read Conceptual.def
   top-to-bottom from that point and migrate whatever legacy constructs remain (more `providing`
   macros, more `switched_device` blocks not already caught by Phase 1, anything else obsolete-DSL-
   shaped) in file order, not by hunting for a specific pattern.

### Phase 4 -- Back to PROJECT.md's own task list

Once Phases 1-3 are done, resume whatever PROJECT.md's own backlog has queued next (its own DSL
obsolescence inventory item 1b -- `zwave_node`/legacy macro removal -- likely becomes unblockable
exactly at that point, once Phase 3's `light_device` sites are gone).

## Nodes 57/58 migrated + a real generator regression found and fixed, 2026-09-28

Nodes 57/58 (a cross-wired pair: node 57 endpoints -> WC light + kitchen "main" light, node 58
endpoints -> WC fan + kitchen "workplace" light) migrated together, confirmed live by the user
(all four entities tested working). WC light/fan and kitchen main are plain domain relabels (no
armature); kitchen workplace is a real switched-by combo with its own separate Zigbee dimmer
(Physical.def's new `sensors.house_kitchen_workplace_dimmer`). Domain "fan" used for node 58's own
endpoint 1 -- confirmed working as a free declaration choice, same as "light" elsewhere.

**Real regression found and fixed during this migration**: kitchen workplace's own room-level
light aggregate (`light.social_house_kitchen_workplace`, no "_main" suffix -- a `generateTemplateLights`-
produced entity, distinct from the switched-by SelfEntity itself) stopped generating. Root cause:
`resolveSwitchedByEndpoint`'s own deferral (added earlier this session, the "prefer explicit
positioning" fix) routinely pushes Device/MainEntity resolution to the LATER finalAttempt-only
retry pass -- but a space's own "apply light-on/off defaults" snapshot (`HandleEndToken`,
administration.go) runs synchronously the moment that space's "end;" is parsed, on the FIRST pass.
The switched-by SelfEntity's own bookkeeping registration (into `EntityRecordsBySpace`/
`SpaceLightsByName`) used to happen only AFTER Device/MainEntity resolution succeeded -- missing
that already-run snapshot for any space relying on the auto-collect fallback (no explicit "light
on:"/"light off:" override). Every switched-by site migrated before this one (server_room/corridor,
living/dining) already had an explicit "light on:" in its own space, masking the bug -- kitchen
workplace was the first one without it. **Fixed**: the SelfEntity's own registration now happens
EAGERLY, decoupled from Device/MainEntity resolution (the entity's own NAME is known immediately;
only ITS combo partners need deferral) -- `logical_switched_by.go`'s own header comment and the
call site have the full writeup. New regression test:
`TestSwitchedBySelfEntityFeedsSpaceLightAggregate`. Verified live: removing the (now-redundant, not
removed, the user's own file) `light on:` workaround from kitchen workplace and regenerating still
produces the aggregate correctly. Redeployed (HA files only, no coordinator/Logical.def change) +
Core restarted.

**Action item for later**: every switched-by site migrated so far HAS an explicit "light on:" in
its space (masking this bug, now moot since it's fixed) -- no retroactive fix needed for
server_room/corridor/living/dining. Going forward, a site WITHOUT an explicit "light on:" no longer
needs one added just to work around this (the fix handles it) -- only add one if the space
genuinely needs non-default on-list behaviour.

## Nodes 57/58 renamed + repositioned, and a second cosmetic generator fix, 2026-09-28

Nodes 57/58's own device-ids renamed to reflect their real function/location (per the user):
`switches.house_kitchen_wc_fuse_box_zwave_057` -> `switches.house_kitchen_hallway_door_power_socket_zwave_057`,
`..._058` -> `switches.house_kitchen_hallway_door_switch_box_zwave_058`. Both nodes' own
node-liveness positioning moved OUT of `space social:wc` into a new `space
infrastructural:fuse_cabinet` sub-space inside `space social:kitchen` (their real physical
location, matching garage/server_room's own established sub-space convention) -- node 57 also gets
a custom leaf (`binary_sensor.infrastructural:hallway_door/socket/node`, not bare `:node`), node 58
keeps the bare form. Step 4 cleanup applied (20 entities disabled, same categories as before).
**Cross-checked node-liveness positioning for all 11 migrated nodes** (15, 33, 42, 44, 48, 54, 55,
57, 58, 64, 67) -- all confirmed present (44's own is a block-form `entity
binary_sensor.infrastructural:node;` inside a `with: ... end;` body, not the one-liner form the
other 10 use, both are equally valid). Apply this same node-liveness cross-check to every future
node this session migrates, not just the ones done so far.

**Second real generator fix, found by the user (2026-09-28, same session)**: `buildPlatformGroupYAML`
(generateSwitchedDeviceEntities's own group-entity builder) was writing the ALREADY-SANITISED,
underscore-joined object_id as its own "name:" field (e.g. "social_house_kitchen_workplace_main")
-- inconsistent with EVERY other group-style light entity in this codebase, which use
`buildLightGroupYAML`'s own established convention: a slash-separated conceptual path as "name"
(e.g. "social/house/kitchen/workplace/main", purely the HA-displayed friendly_name) plus an
EXPLICIT "unique_id:" field carrying the real entity_id (decoupling the two, so slugified-name
guessing never comes into play). Fixed: `buildPlatformGroupYAML`'s signature now takes both
`entityID` (for `unique_id:`) and `displayName` (for `name:`, derived from the already-computed
pre-sanitisation fullName strings, which are already slash-separated -- no new string-building
logic needed). Verified: applies uniformly to every switched-by site generated so far
(server_room/corridor/living/dining/kitchen-workplace all confirmed with the new format); Vienna
has ZERO `switched_device`/`is switched by` sites at all this session, so the fix is inert there
(confirmed, not just assumed -- no live Vienna file uses this code path). **Not yet deployed** --
the user asked to hold this one until the next scheduled deploy rather than redeploy immediately.

## Nodes 71/52 (bathroom + office/storage_room), migrated locally 2026-09-28 -- NOT YET DEPLOYED

Per the user: "combine with next node" -- this migration, the pending node 57/58 rename, and the
buildPlatformGroupYAML name/unique_id fix are all queued together for one combined deploy, not yet
pushed. Both nodes physically sit in the bathroom's own fuse cabinet (confirmed by the user) despite
node 52 driving office+storage_room, not the bathroom -- same cross-wired-location pattern as nodes
57/58. Node 71: endpoint 1 = bathroom main (combined with its own separate disc-light armature,
`sensors.house_bathroom_main_disc`, kept an explicit `physical:main_disc` positioning since 2
pre-existing manual automations target it directly), endpoint 2 = bathroom mirror (plain relabel).
Node 52: endpoint 1 = office main (combined with its own separate ceiling-light armature,
`sensors.house_office_ceiling`, no kept explicit positioning needed -- nothing external references
it), endpoint 2 = storage_room main (plain relabel, was on the bare/fragile no-rename-shorthand
`entity light.social:main;` form before, no explicit device reference at all). Both nodes'
node-liveness now in a shared `space infrastructural:fuse_cabinet` sub-space inside `space
social:bathroom`, referencing light_2 (not light_1) in each node's own Physical.def liveness
condition -- light_1 (bathroom main / office ceiling) has no explicit positioning of its own (no
external dependency needs it stable), so light_2 (mirror / storage_room, both explicitly
positioned) is used instead, avoiding yet another instance of the same "no positioning yet for the
liveness sibling" warning node 15/57/58 already hit. Pre-migration cleanup (aliases removed, `_old`
renames) confirmed done by the user for both nodes before this work started. Manual-automations
cross-check done: only the stable `light.social_house_bathroom_main` SelfEntity name is referenced
anywhere live, already unaffected. Regenerated + tested + Vienna byte-diffed clean; node-liveness
cross-check passed for both new nodes.

**Remaining in the Fibaro-double-switch sweep after this combined deploy lands**: none -- 71/52
were the last two `providing`/`switched_device` FGS-223 sites in the original inventory. Only the
FGS-213 singles (10, 11, 12, 13, 17) and confirming node 58's own step-4 zwave-discovery-cleanup
(already done) remain before Phase 1's sweep is fully complete, plus the two Phase-1 cross-checks
(all-zwave-devices-declared, all-unused-endpoints-excluded) as a final pass.

## Combined deploy CONFIRMED LIVE, 2026-09-28

Bathroom's 2 pre-existing dim-up/down automations were changed (per the user) to target the
combined `light.social_house_bathroom_main` entity directly instead of the raw disc light --
simpler than living/dining's own "kept explicit physical positioning" approach, and means
`sensors.house_bathroom_main_disc` no longer needs one either (reverted to the single-positioning
form, XX now fully auto-materializes). Live automations.yaml fixed + reloaded to match.

Deployed (HA files + coordinator, since nodes 71/52 changed discovery config) + Core restarted.
User confirmed all working after a brief restart-settling window (several entities/aggregates
showed transient "unavailable" right after restart, including the whole-house `light.social_house`
aggregate itself and `light.social_house_bathroom` specifically -- both cleared on their own within
~1-2 minutes, consistent with every other restart this session, not a new bug). Verified live:
light.social_house_wc_main, fan.social_house_wc, light.social_house_kitchen_main,
light.social_house_kitchen_workplace_main, light.social_house_bathroom_main,
light.social_house_bathroom_mirror, light.social_house_office_main,
light.social_house_storage_room_main, and the full aggregation hierarchy up to light.social_house
itself -- all correct.

**This closes out the entire FGS-223 (double switch) portion of Phase 1's sweep**: nodes 15, 48,
57, 58, 71, 52 all migrated, deployed, tested, and step-4-cleaned this session, on top of the
already-modern 33/42/44/54/55/64/67. Next: the FGS-213 singles (10, 11, 12, 13, 17), then the two
Phase-1 cross-checks as a final pass over the whole zwave inventory.

## FGS-213 singles (10, 11, 12, 13, 17) migrated locally 2026-09-28 -- NOT YET DEPLOYED

All 5 done in one pass, "one by one" per the user (local migration for each, step-1 cleanup
confirmed per-node before implementing). All share the SAME simple shape: single used endpoint
(the OTHER endpoint on every one of these FGS-213s is genuinely unused hardware -- confirmed live
via each node's own `switch.single_switch_2_2_N` entity already being `disabled_by: user`, matching
this phase's own "exclude unused capabilities from discovery" cross-check automatically, since only
the used endpoint's capability was ever declared in Physical.def), no armature, no manual
automation dependencies (cross-checked per node, all clean). Naming: `switches.house_<location>_
<function>_zwave_0NN` (e.g. `switches.house_shower_room_switch_box_zwave_010`), matching the
FGS-223 sites' own established convention. Node-liveness positioned as a plain top-level one-liner
in each node's own space, EXCEPT node 17 (bathroom's own whirlpool/bathtub relay), added to the
SAME shared `infrastructural:fuse_cabinet` sub-space as nodes 71/52 (same physical location).
Node 17 is the one exception to "declare everything as light" -- it's a genuine switch (a whirlpool
pump, `switch.switch_1`), correctly kept domain `switch`, not relabeled to `light`.

- Node 10 (shower_room switch_box): plain relabel, `light.social_house_shower_room_main`.
- Node 11 (master_bedroom door switch_box): plain relabel, `light.social_house_master_bedroom_main`.
- Node 12 (sarah_bedroom door switch_box): plain relabel, `light.social_house_sarah_bedroom_main`.
- Node 13 (david_bedroom door switch_box): plain relabel, `light.social_house_david_bedroom_main`.
- Node 17 (bathroom bathtub/relais_box, whirlpool pump): plain relabel, domain `switch`,
  `switch.social_house_bathroom_whirlpool`.

Regenerated clean (zero warnings on the first attempt for all 5 -- no liveness-sibling conflicts
this time, since each node's own single used endpoint IS what its liveness condition already
references). Full test suite green, Vienna byte-diffed clean throughout. Node-liveness cross-check
passed for all 5. **Not yet deployed** -- awaiting the user's go-ahead, same as the previous round.

**This completes the ENTIRE Fibaro zwave sweep** (both FGS-223 doubles and FGS-213 singles) --
every node originally inventoried is now migrated, on the modern `is switched by`/plain-relabel
mechanism, off every legacy `providing`/`switched_device` construct. Remaining before Phase 1 is
fully closed: (1) deploy + test this round, (2) the two Phase-1 cross-checks as a final formal pass
(all-zwave-devices-declared -- almost certainly already true given this sweep, but worth a clean
final check; all-unused-endpoints-excluded -- also already true in practice, confirmed per-node
throughout this sweep, but worth the same final formal pass). Then Phase 2 ("everything else
queued") begins.

**Correction (2026-09-28, same session)**: nodes 71/52 were WRONGLY assumed to physically sit in
the bathroom's own fuse cabinet (my own earlier inference from "same location as 71", which I
misread). Corrected by the user: 71/52 physically sit in the SERVER ROOM's own fuse cabinet
instead -- renamed `switches.house_bathroom_fuse_cabinet_zwave_{071,052}` ->
`switches.house_server_room_fuse_cabinet_zwave_{071,052}` across all 3 `.def` files, and moved
their node-liveness lines out of a (now-removed) bathroom `infrastructural:fuse_cabinet` sub-space
into `space social:server_room`'s own PRE-EXISTING one (alongside nodes 54/55/64/48). Node 17's own
relais_box, by contrast, IS genuinely in the bathroom (confirmed) -- kept there, but as its own
standalone positioning (not a shared "fuse_cabinet" grouping, since a relais_box is a different kind
of enclosure). Regenerated + tested + Vienna byte-diffed clean after the fix; node-liveness
cross-check re-confirmed for all 3 renamed/repositioned nodes. Lesson: don't infer physical
location from which rooms a device's OWN capabilities happen to drive -- ask, or wait for the user
to state it directly, especially for cross-wired devices where the function and the physical
mounting location can differ.

## ENTIRE Fibaro zwave sweep CONFIRMED LIVE, 2026-09-28

Deployed (HA files + coordinator) + Core restarted + user-tested + step-4-cleaned (35 more entities
disabled for nodes 10/11/12/13/17). Minor observation, not actioned: node 12's own status/ping
entities carry a stray "_2" suffix (a pre-existing naming collision, unrelated to tonight's work,
not a step-4 candidate) -- flagged for the user, left alone.

**Phase 1's Fibaro sweep is now fully DONE, deployed, and confirmed working end to end**: every
zwave node in the original inventory (33, 42, 44, 48, 54, 55, 57, 58, 64, 67 -- already modern or
migrated this session -- plus 15, 71, 52, 10, 11, 12, 13, 17, all migrated this session) is on the
modern mechanism. Remaining for Phase 1: the two formal cross-checks (all-zwave-devices-declared,
all-unused-endpoints-excluded) as a final pass -- almost certainly both already true in practice
(every node was checked individually throughout this sweep), but worth a clean confirming pass
before calling Phase 1 fully closed. Then Phase 2 ("everything else queued": with-clause consumers,
`is enabled by` mechanism, jinja domain-qualification, Vienna washing_machine, Philips dimmers).

## Phase 1 formal cross-checks + `is enabled by` mechanism (Phase 2 item 2), 2026-09-28

**Phase 1 CLOSED**: both formal cross-checks run clean -- all-zwave-devices-declared (18/18 exact
match against a fresh live registry pull), all-unused-endpoints-excluded (one cosmetic-only
exception, node 55's declared-but-never-positioned `switch.core_2`, confirmed harmless since a bare
Physical.def declaration alone never triggers a discovery publish). Native Z-Wave JS integration
subsequently deleted from HA main by the user; zero leftover `_old` entities confirmed.

**`is enabled by` mechanism (Phase 2 item 2) DONE, REPLACING the plain `enabler <device-id>;`
keyword entirely** (confirmed via AskUserQuestion -- "Replace", not "Coexist"):
- Grammar: `<domain>.<label> is enabled by <domain>.<label> from <device-id>;` -- LHS names a
  capability already declared on THIS device at the physical layer (validated implicitly via the
  normal same-device sibling-ref resolution machinery, no bespoke pre-check).
- Storage: `TLogicalDevice.EnabledBy *TLogicalEnabledByFact` (pointer, nil = none) replaces the old
  `Enabler string` field. `logicalEnabledByPattern` replaces `logicalEnablerPattern` in
  `integration_logical_parser.go`, wired into `logicalDeviceBodyStatements` (the generic
  with-clause dispatcher) alongside `is switched by`.
- Desugaring (`logical_enabler_dependency_sugar.go`): the base "node" self-check condition changed
  from the whole device's own `physical <device-id>` liveness to the NAMED LHS capability's own
  `<label> is available` state -- more precise for a device with several capabilities where only
  ONE actually signals "the appliance is genuinely on".
- All 5 files touched (`integration_logical_storage.go`, `integration_logical_parser.go`,
  `logical_enabler_dependency_sugar.go`, `integration_logical_parser_test.go`,
  `logical_enabler_dependency_sugar_test.go`) -- full test suite green, `go build/vet/test` clean.
- Migrated the one live site, Junglinster's `appliance.tv_living_room`
  (`enabler switch.house_living_room_tv;` -> `media_player.core is enabled by switch.core from
  switch.house_living_room_tv;`). Regenerated: **not byte-identical**, but confirmed functionally
  identical and the difference is benign -- explained to and explicitly accepted by the user
  ("No. Go ahead. Was only intended as a 'trigger'."): the old sugar's base condition
  (`physical appliance.tv_living_room`) auto-materialized an unused indirection entity
  (`binary_sensor.physical_appliance_tv_living_room_node`, never referenced anywhere else, whose
  own template body was textually IDENTICAL to what's now inlined directly); the new sugar's base
  condition (`core is available`) references `media_player.social_house_living_room_tv` directly,
  so that indirection entity (plus its customization file) is simply no longer generated. Net
  diff: 2 files deleted (the unused template + its customization), 1 file's template text changed
  to the equivalent inlined form (Junglinster `hass/` tree only) -- coordinator output untouched.
  Vienna byte-diffed **fully clean** (it never used the plain `enabler` sugar, so this change is
  inert there -- a real, meaningful confirmation given Vienna's own extensive `dependency
  on`/`enabler` usage).
- Regenerated, not yet deployed -- awaiting explicit deploy go-ahead, as always.

**Remaining Phase 2 items, in order**: 7 of 13 with-clause consumers left (item 1, see the
inventory above), jinja source domain-qualification, Vienna's `washing_machine` reconciliation (its
own `... or not enabler` form, now itself needing migration to `is enabled by` too), Junglinster's
4 undeclared Philips dimmer `action` capabilities, then the remaining plain-`enabler`-sugar
migration sites now that `is enabled by` exists (Junglinster: `sonos_garage`,
`appletv_living_room`, `sonos_playbar_livingroom`; Vienna: `apple_tv`, `tv`, `sonos`,
`sonos_roam`).

## With-clause consumers (item 1) reviewed, jinja domain-qualification + washing_machine done, 2026-09-28

**With-clause consumers (item 1) -- reviewed, no further action needed today.** Of the remaining 7:
`Conceptual_DevicePositioning.go`'s device-positioning one-liner already delegates its body to
parser.go's own shared per-line dispatch (not a local statement list at all) -- confirmed it
doesn't fit/need the generic mechanism. The other 6 (absorb block, derived block, hassbridge
device-level, and the 4 list-bodied integration headers) all share one property: NONE of them
currently has a one-liner form, so there is no DUPLICATED block-vs-one-liner dispatch to collapse
(the actual problem this mechanism solves) -- unifying them would mean ADDING new one-liner grammar
surface, a real scope expansion the user asked to defer ("the other two will likely lead to 'new
things'"). Left untouched; revisit as its own deliberate pass, not a mechanical sweep.

**Jinja source domain-qualification -- DONE, both houses, byte-identical.** Generalizes the
2026-09-27 mandatory-domain-qualification rule (`entity X from Y Z;`) to a jinja clause's own
`{ ... }` source list: `{ battery_level }` -> `{ sensor.battery_level }`. Implementation
(`Conceptual_LogicalEntities.go`):
- `resolveLogicalSiblingSourceEntityID` now strips a leading domain (`bareCapabilityLabel`) before
  every sibling-lookup tier (previously only the "from"-qualified path did this) -- a NEW
  `mustBeSibling bool` parameter suppresses the tier-4 literal-entity-id fallback when the caller
  has already confirmed the token names a real capability (see below), so a declared-but-not-yet-
  LINKED sibling still defers/warns correctly instead of being silently misread as literal.
- `render`'s own `CondJinja` case: a bare (non-dotted) source is now a hard warning (mirroring the
  2026-09-27 rule's own actionable-suggestion style, via `lookupCapabilityDomain`). A dotted token
  is genuinely ambiguous on its own (`sensor.battery_level` could be "the sibling capability
  battery_level, really domain sensor" OR a literal already-existing entity_id, e.g.
  `sensor.infrastructural_apartment_bedroom_sonos_battery_level` for a media_player attribute never
  modeled as a capability at all -- a real, tested, still-needed feature, confirmed via
  `TestRegisterLogicalDerivedConditionCapabilityResolvesLiteralEntityInOver`) -- disambiguated by
  checking `lookupCapabilityDomain` first: a REAL declared capability of this device resolves via
  the normal sibling path (`resolveJinjaSource`, `mustBeSibling=true`); everything else is used
  verbatim as a literal entity reference, exactly matching the pre-existing tier-4 fallback.
- `logical_enabler_dependency_sugar.go`'s own generated jinja text updated to
  `{ <SourceDomain>.enabler }` (was bare `{ enabler }`) -- domain-qualified consistently with
  everything else now.
- 5 unit tests fixed to populate the declaration-level maps (`discoveryGatewaysByID`/
  `logicalDevicesByID`) the new `lookupCapabilityDomain` gate needs -- these tests previously only
  populated the resolved-entity-LINK level (`LogicalEntityLinks`/`DiscoveryEntityLinks`), which was
  enough under the old (non-domain-aware) resolution but isn't enough to exercise the new
  declaration-aware gate correctly.
- Both houses' 7 real jinja-source usages migrated (Junglinster: `front_door_ring`/`ring_camera`'s
  own `battery_alert`, x2; Vienna: `appliance.tv`'s own `enabler`, `sonos_roam`'s own
  `battery_alert`, `netatmo_terrace_wind`'s own `windy` (`wind_speed`+`windy_threshold`),
  `terrace_motion`'s own `sunny` (`illuminance`+`sunny_threshold`)). Regenerated + byte-diffed --
  **fully clean, both houses, hass/ and coordinator/ output alike** -- confirming this is a pure
  syntax tightening with zero behavioural change, exactly as expected ("Probably again byte
  equivalent").

**Vienna's `washing_machine` reconciliation -- DONE, NOT byte-identical (deliberately).** The
device's own `derived binary_sensor.node` condition changed from `... or not enabler` (a loose
boolean negation, true for off/unavailable/unknown alike) to the strict jinja form
`... or jinja "$1 == 'off'" { switch.enabler }` -- matching the `enabler`/`is enabled by` sugar's
own ALWAYS-strict semantics, a tightening the user explicitly confirmed as deliberate back when the
sugar was first designed (2026-09-27: "Vienna's `appliance.washing_machine` ... will have its own
semantics tightened to match once it's migrated to the new sugar -- not a blocker"). **Not folded
into the `is enabled by` one-liner sugar itself** -- this device absorbs several OTHER capabilities
beyond "enabler" (power/energy/current/consumes) and its own liveness genuinely combines two
independent conditions (`node.candy` AND (`physical` OR switched-off)), which doesn't fit the
sugar's single-capability "OwnLabel is available" base condition -- kept as an explicit hand-written
`derived binary_sensor.node` block, just with the one line's own condition text tightened. Verified
via byte-diff: exactly ONE file changed (`binary_sensor.infrastructural_apartment_shower_room_
washing_machine_node.yaml`), the diff being precisely `not is_state(..., 'on')` ->
`states(...) == 'off'` -- nothing else touched, coordinator output untouched. Regenerated clean,
zero warnings, full test suite green.

**Deliberately deferred to later today (session-limit-conscious, per the user's own instruction)**:
Junglinster's 4 undeclared Philips dimmer `action` capabilities, and the remaining plain-`enabler`-
sugar migration sites (Junglinster: `sonos_garage`, `appletv_living_room`,
`sonos_playbar_livingroom`; Vienna: `apple_tv`, `tv`, `sonos`, `sonos_roam`) -- both of these are
expected to produce genuinely NEW generated YAML/coordinator config (new capability declarations,
not pure refactors), unlike everything done in this round. **Nothing deployed this round** -- all
changes are local/regenerated only, awaiting explicit deploy go-ahead as always.

## Session limit reset, 2026-09-28 (continued) -- both deferred items done

**Junglinster's 4 undeclared Philips dimmer `action` capabilities -- DONE.** Added
`event.pressed event:<mac>_action_zigbee2mqtt;` to all 4 devices' Physical.def declarations
(`living_room_dining_philips_dimmer`, `living_room_living_philips_dimmer`,
`house_kitchen_terrace_door_philips_dimmer`, `bathroom_philips_dimmer`) -- matching Vienna's own
`buttons.living_room_signify_dimmer` precedent exactly (bare `event.pressed`, no
`sensor.pressed_as_sensor` counterpart -- these are discrete named actions, not a value worth
polling as a sensor, unlike some of the taps). Positioned each into Conceptual.def's existing
`as philips_dimmer with:` block as `entity event.physical: from event.pressed;` (bare leaf, same
convention as every tap device). Regenerated clean, zero warnings -- 4 new
`event.physical_house_*_philips_dimmer` entities now appear in `discovery.yaml`, confirmed present
with the right `gateway`/`leaf`/`source_domain`. Vienna untouched, regenerated clean too. **This was
genuinely NEW output** (new capability, new entities) as expected -- not deployed, awaiting the
user's go-ahead.

**Remaining plain-media-relocation sites -- DONE, byte-identical (turned out simpler than the
"enabler-sugar" framing suggested).** Re-reading these 7 devices' actual current Logical.def/
Physical.def content revealed NONE of them (except Vienna's `appliance.tv`) uses `enabler`/`is
enabled by` at all -- they only had a plain hand-written `derived binary_sensor.node with:
condition physical <id> and node.<x> [and ...];` (a genuine cross-device combination, several
referencing OTHER devices' own nodes/media_players by name -- e.g. the Sonos playbar ANDing in 3
satellite speakers' own liveness -- which can never collapse into the single-capability `is enabled
by` sugar's "OwnLabel is available" base condition, so correctly stays hand-written in Logical.def
forever). The ACTUAL remaining task was Part A of this plan's very first section: relocate each
device's own `switch.media <entity> [with no_play_input "...";];` line from Logical.def to
Physical.def's already-existing `local`-kind block (the pilot, `appliance.tv_living_room`, was done
2026-09-27; these 7 were the "remaining sites" note left unresolved). Done for:
- Junglinster: `sonos_garage`, `appletv_living_room`, `sonos_playbar_livingroom` (the latter kept
  its own `with no_play_input "TV";` refinement, using the local-kind one-liner sugar).
- Vienna: `apple_tv`, `sonos`, `sonos_roam`, and `appliance.tv` (switch.media relocated; its own
  `capabilities from appliance.living_room_tv: switch.enabler core; end;` absorb block and hand-
  written `derived binary_sensor.node` stay exactly as they are -- see below).
- Regenerated both houses -- **fully byte-identical**, `hass/` and `coordinator/` alike, confirming
  the relocation is a pure refactor with zero behavioural change (same underlying registration
  machinery either layer).

**`appliance.tv` (Vienna) deliberately NOT migrated to `is enabled by`, even though it superficially
looks like a candidate.** Investigated closely: it uses `delay_off 00:01:00;` on its own `derived
binary_sensor.node` block -- a real, live, deployed 1-minute debounce before the TV's own liveness
flips off. `applyLogicalEnablerDependencySugar`'s synthesized "node" capability has NO way to carry
a `DelayOn`/`DelayOff` value at all (confirmed by reading the sugar's own code) -- migrating this
device would SILENTLY DROP that debounce, a real behavioural regression, not a cosmetic one. Since
`is enabled by`/`enabler` were only ever meant to REPLACE the plain `enabler <device-id>;`
shorthand (confirmed earlier, "Replace" not "Coexist"), not to retire the general-purpose
absorb-block + hand-written-derived-condition grammar entirely, leaving `appliance.tv` on its own
explicit longhand is correct, not a leftover -- flagged here so a future pass doesn't "helpfully"
migrate it without noticing the same gap.

Both houses' full test suite green throughout. **Nothing deployed** -- all changes local/regenerated
only, awaiting explicit deploy go-ahead. Phase 2 is now fully closed (with-clause consumers
reviewed/deferred as a deliberate "no" for today, `is enabled by` done, jinja domain-qualification
done, washing_machine reconciled, Philips dimmers declared, switch.media relocation complete for
all 8 sites). Next: Phase 3 (Conceptual.def's own embedded migration queue -- remaining
`light_device` macro sites needing the user's own live-device check, remaining media_player sites
already covered by the work above, sequential continuation from line 391) and Phase 4 (PROJECT.md's
backlog).

## Vienna's `appliance.tv` migrated to `is enabled by` too, 2026-09-28 (same day, fresh session limit)

The user's own instruction: copy Junglinster's `appliance.tv_living_room` pattern to Vienna's
`appliance.tv`. Investigating first (per this session's own discipline) surfaced the real blocker
flagged earlier: `appliance.tv`'s hand-written `derived binary_sensor.node` carries a
`delay_off 00:01:00;` -- a real, deployed 1-minute debounce -- which the `is enabled by` sugar had
NO way to express (`applyLogicalEnablerDependencySugar`'s synthesized "node" capability never set
`DelayOff` at all). Migrating naively would have SILENTLY DROPPED that debounce -- a real
behavioural regression, not cosmetic -- so this was fixed properly rather than worked around:

- **New device-level `delay_off <time>;` grammar** (Logical.def, `integration_logical_parser.go`):
  `logicalDeviceDelayOffPattern`, added to `logicalDeviceBodyStatements` (the generic with-clause
  dispatcher) -- works in both the block form and the one-liner fold, matching every other
  with-clause statement's own precedent.
- **New `TLogicalDevice.NodeDelayOff string` field** (`integration_logical_storage.go`), carried
  into the sugar's own synthesized `Capabilities["node"]` (`logical_enabler_dependency_sugar.go`:
  `DelayOff: d.NodeDelayOff` -- confirmed this reuses the SAME `TLogicalCapability.DelayOff` field
  `registerLogicalDerivedConditionCapability` already reads for every hand-written derived
  condition, not a new plumbing path).
- Also fixed the one-liner rejection warning's own stale text (still said `"enabler <id>;"`, the
  RETIRED keyword, instead of `"is enabled by"` -- caught while touching this code).
- 2 new tests: `TestParseLogicalLayerBodyDeviceLevelDelayOff` (parser),
  `TestApplyLogicalEnablerDependencySugarCarriesNodeDelayOff` (sugar). Full suite green.
- Vienna's `appliance.tv` migrated:
  ```
  device appliance.tv with:
    dependency on node.tv;
    delay_off 00:01:00;
    media_player.core is enabled by switch.core from appliance.living_room_tv;
  end;
  ```
  (source device is `appliance.living_room_tv`, a discovery-kind device whose own `switch.core` is
  genuinely domain "switch" -- confirmed by reading Physical.def directly, not assumed.)
- Regenerated: **not byte-identical**, but the exact same class of benign diff already accepted for
  `tv_living_room`/`washing_machine` -- the old base condition's own `physical appliance.tv`
  auto-materialized an unused indirection entity (`binary_sensor.physical_appliance_tv_node`,
  confirmed referenced nowhere else), now gone; the real node entity
  (`binary_sensor.infrastructural_apartment_living_room_tv_node`) inlines the equivalent check
  directly. **Confirmed `delay_off: 00:01:00` survived intact** in the regenerated YAML -- the one
  thing this whole detour existed to protect. Junglinster regenerated too (byte-identical, zero
  warnings) confirming the new grammar is additive-only, no effect on existing devices.
- **Not deployed** -- awaiting the user's go-ahead, same as every change this round.

## Two real fixes from reviewing this work with the user, 2026-09-28 (same day, still fresh)

**1. Real architectural gap fixed: `enabled by` now works for hassbridge/import-kind devices
too.** The user pushed back on my own explanation of why Vienna's `appliance.tv` needed special
handling -- rightly so: their ORIGINAL design spec (predating "is enabled by") explicitly
anticipated `dependency` working for MQTT-based devices via the discovery-based
`depends_on_availability` condition. That part was never in question. What I'd conflated: an
`enabler` is an OR/exception ("unless the switch reports literally off, that's not a fault"), which
HA's own MQTT `availability:` topic list can only ever AND, never express an OR/exception for -- so
an enabler *always* needs a template-based "node" condition, regardless of the device's own kind.
The OLD code's `applyLogicalEnablerDependencySugar` skipped node-template synthesis for EVERY
hassbridge/import-kind device unconditionally -- meaning a hassbridge-kind device declaring
`enabled by` would have its "enabler" capability silently absorbed but NEVER actually wired into
the device's own liveness at all (a latent bug, never hit live since the one hassbridge-kind device
needing an enabler, Vienna's `appliance.washing_machine`, still uses the full hand-written
longhand). **Fixed**: the kind-based skip now applies ONLY to the pure dependency-only case
(`EnabledBy == nil`) -- exactly the case `depends_on_availability` already covers correctly; a
device WITH `EnabledBy` always gets the template now, regardless of kind. New regression test:
`TestApplyLogicalEnablerDependencySugarSynthesizesNodeForHassbridgeDeviceWithEnabledBy`.
**Side discovery while fixing this**: Vienna's `appliance.washing_machine` could now ALSO be
migrated to `enabled by` (its own base condition, `status is available and node.candy`, is
equivalent to today's `physical appliance.washing_machine and node.candy`, same reasoning as
`appliance.tv`'s own equivalence) -- the OTHER absorbed capabilities (power/energy/current/
consumes) would just move to their own separate `capabilities from switch.washing_machine: ...;
end;` block, no longer including the `switch.enabler` line itself. Documented as a real, available
option in washing_machine's own comment; NOT done -- the user only asked for the gap fix + rename,
not this further migration.

**2. Grammar simplified: "is enabled by" -> "enabled by".** The user's own call, purely readability
-- "enabled by" reads better directly after "with:" than "is enabled by" does. `logicalEnabledByPattern`'s
regex updated (dropped `is\s+`), every comment/doc-string updated across
`integration_logical_parser.go`/`integration_logical_storage.go`/`logical_enabler_dependency_sugar.go`
and their test files, both live `.def` sites (Junglinster's `appliance.tv_living_room`, Vienna's
`appliance.tv`) updated to the new spelling. "is switched by" (the sibling construct) was
DELIBERATELY left untouched -- the user's instruction was scoped to "enabled by" specifically.

Both fixes: full test suite green (2 new tests total), both houses regenerated -- **fully
byte-identical**, `hass/` and `coordinator/` alike, confirming the grammar rename is pure syntax and
the hassbridge/enabler gap fix has zero effect on any currently-migrated device in either house (as
expected -- neither `tv_living_room`/`appliance.tv` is hassbridge-kind, so they never exercised the
bug). **Not deployed** -- awaiting the user's go-ahead.

## Everything above DEPLOYED + VERIFIED LIVE, 2026-09-28 (later, "Let's deploy")

Full deploy pipeline run on both houses (HA files + coordinator rebuild/restart + hosts +
commandline), both coordinators confirmed on fresh builds, both HA Cores restarted. Two real,
pre-existing entity-registry bugs surfaced during verification (stale `deleted_entities` tombstones
squatting clean names, forcing today's newly-active entities to `_2` -- same class as the
2026-09-23 stale-template-entity incident, confirmed unrelated to today's own edits via byte-diff)
-- both fixed via the established registry-surgery playbook:
- Vienna: `binary_sensor.infrastructural_apartment_shower_room_washing_machine_node` (2 stale
  tombstones from an old hassbridge relay unique_id).
- Junglinster: `binary_sensor.infrastructural_garage_sonos_node` and
  `binary_sensor.infrastructural_house_living_room_sonos_playbar_node` (2 tombstones each, old
  hyphenated-vs-underscore unique_id variants) -- these were also causing their own `switch.media`
  coercion switches to read `unavailable` (fixed as a direct consequence).

A broader scan found ~46 more `_2` collisions on Junglinster and 12 on Vienna, ALL confirmed
unrelated to this session's own work (macbook_pro mobile_app re-registration, Samsung TV re-pairing,
aqara thermostat switches, philips_motion illuminance sensors) -- left untouched, flagged to the
user as a separate, optional housekeeping item, not touched without being asked.

Every entity from today's work spot-checked live and correct afterward: 4 Philips dimmer
`event.pressed` entities, `tv_living_room`/`appliance.tv`'s migrated node conditions
(`delay_off: 00:01:00` confirmed preserved on the latter), every relocated `switch.media` entity on
both houses. `appliance.tv`'s own media switch legitimately reads `unavailable` right now (its own
node is genuinely `off` -- TV disconnected/off in real life) -- confirmed this is correct, pre-
existing behaviour via byte-diff, not a bug.

## Phase 3 started, 2026-09-28 (same session) -- bloom_left migrated

First item of Phase 3 (Conceptual.def's own embedded "#### HERE" migration queue): of the 4
remaining `call light_device` macro sites (`bloom_left`, master_bedroom's `left`/`right`, and a
newly-surfaced 4th site, server_room's `corb` -- referenced by two `follows` lines tied to the
switched-by fuse-box light group, not previously tracked in this plan), the user picked `bloom_left`
first.

**Confirmed live** (2026-09-22's own prior check had found it NOT in the passthrough list --
correct at the time, but it's back now): `zigbee2mqtt_0x001788010923befb`, an active Zigbee light
with entities including `light.social_house_living_room_living_bloom_left`, confirmed via direct
device-registry lookup on Junglinster's main HA instance (not guessed). Migrated to the modern form,
identical shape to its own sibling `bloom_right` (Physical.def `light.core` + `derived
binary_sensor.node`; Conceptual.def `entity binary_sensor.infrastructural:node;`/`entity
light.social: from light.core;`). Regenerated clean, zero warnings -- the generated discovery
entity_id matches the ALREADY-LIVE one exactly
(`light.social_house_living_room_living_bloom_left`). Full test suite green. **Not deployed yet**.

**master_bedroom's `left`/`right` migrated too, 2026-09-28.** Confirmed live via device-registry
lookup: `left_table_lamp`/`right_table_lamp` (house/master_bedroom/ namespace) -- exactly the guess
flagged-but-unconfirmed on 2026-09-22, now confirmed precisely (their own already-live entities are
literally named `light.social_house_master_bedroom_bed_left`/`_right`, not just plausibly the same
device). Real naming-mechanics lesson hit while migrating: giving the device an "as left_table_lamp"
leaf AND a non-empty explicit entity leaf override ("entity light.social:left from light.core;")
does NOT replace the device's own path -- it APPENDS, producing
`light.social_house_master_bedroom_bed_left_table_lamp_left` (wrong, didn't match live). Fixed by
dropping the device's own "as" clause entirely and giving EVERY one of its entities (both "node" and
the light) a fully explicit leaf instead (`entity binary_sensor.infrastructural:left_table_lamp/
node;` / `entity light.social:left from light.core;`) -- confirmed the regenerated entity_id then
matches the live one exactly. Regenerated clean, zero warnings, full test suite green.

**`corb` migrated too, 2026-09-28 -- and a location correction from the user.** I'd wrongly
attributed `corb` to `space social:server_room` (misreading nearby content about node 48's own
fuse-box, physically IN the server room, being POSITIONED under `space social:corridor` because its
own corridor-side endpoint drives corridor's own switched lights) -- the user corrected this: `corb`
itself is genuinely in `space social:corridor` (confirmed directly by re-checking the actual
enclosing `space` line, 1080, not by inference). Confirmed live via device-registry lookup:
`house/corridor/corb`, `zigbee2mqtt_0x0017880104a1e1f7`, already publishing
`light.social_house_corridor_corb` live -- matching the "bloom_left/bloom_right" shape exactly (bare
device identity, no leaf-naming complication like the table lamps needed, since the device's own
natural name already equals the intended entity leaf). Migrated, regenerated clean, zero warnings
(confirming the two `follows` lines referencing `light.social:corb` still resolve correctly), the
generated `light.social_house_corridor_corb` byte-matches the live entity_id. Full test suite green.

**This closes out ALL 4 `call light_device` sites** (bloom_left, master_bedroom's left/right,
corb) -- the last live call sites blocking `PROJECT.md` item 1b's removal of the retired
`light_device` macro mechanism from the generator entirely (that removal itself is a separate,
not-yet-started cleanup task).

**Real process correction, 2026-09-28**: asked "what's the next loose entity", the user caught a
real methodology error -- my own "sequential scan" for the rest of Phase 3 was actually a targeted
`grep` for known legacy KEYWORDS (`call providing`/`definition as switched_device`/`call
light_device`/`call media_player_device`/`call battery_level_device`/`no_collect`), which skipped
an entirely different, unlisted legacy shape: Conceptual.def's own OLD flat "entity X with:
condition <source> \"<jinja-with-bare-$>\"; end;" / "definition as has_state ...;" macros (a still-
actively-parsed mechanism, `expander.go`'s `scanConditionBody`/`parseConditionDirective`/
`generateHasStateEntities` -- NOT dead code, just an OLDER, pre-Logical-layer way of declaring the
exact same kind of derived condition). Confirmed via grep: exactly ONE surviving instance in
Junglinster (`binary_sensor.social:nespresso/brews`), plus its own dependent
`binary_sensor.social:nespresso/in_use`. **Lesson: Phase 3's own "read sequentially, not by
pattern-hunting" instruction means an ACTUAL line-by-line read from the marker, not a grep for a
pre-enumerated keyword list -- any construct not already on that list gets silently skipped
otherwise.**

**Kitchen Nespresso's `brews`/`in_use` migrated, per the user's own direction ("added as a derived
capability of switch.house_kitchen_nespresso", "both actually").** New Logical.def device block:
```
device switch.house_kitchen_nespresso with:
  derived binary_sensor.brews with:
    condition jinja "( $1 | int(0) > 200 )" { sensor.power };
  end;
  derived binary_sensor.in_use with:
    condition brews;
    delay_off 00:10:00;
  end;
end;
```
(`in_use`'s old "definition as has_state brews \"on\" \"\" \"00:10:00\";" macro is EXACTLY
`condition brews; delay_off 00:10:00;` once expressed as an ordinary derived capability --
confirmed by reading `generateHasStateEntities`'s own generation code, same `is_state(...)` +
`delay_off` shape either way.) Conceptual.def's own two macro blocks replaced with bare no-rename
positionings INSIDE the device's existing block (`entity binary_sensor.social:brews;`/`entity
binary_sensor.social:in_use;`, matching the sibling `consumes` capability's own precedent) --
**one real naming bug caught before it shipped**: my first attempt used compound leaves
(`binary_sensor.social:nespresso/brews`) matching the OLD macro's own path text, but since the
device is ALREADY positioned "as nespresso", this DOUBLED the segment
(`..._nespresso_nespresso_brews`, confirmed wrong against the live entity_id
`binary_sensor.social_house_kitchen_nespresso_brews`) -- fixed by dropping the redundant
"nespresso/" prefix (bare `brews`/`in_use` leaves), regenerated entity_ids then matched the
already-live ones exactly. Regenerated clean, zero warnings, full test suite green. Not
byte-diffed against a pre-snapshot (no snapshot taken before this specific edit), but the generated
template text was read directly and confirmed functionally identical to what `scanConditionBody`/
`generateHasStateEntities` would have produced for the old macro forms.

**Kitchen Sonos migrated (media_player_device migration #5), 2026-09-28** -- the user's own
direction: "similar to the roam in Vienna, a sonos with battery... should end up as 1 device."
Confirmed via Vienna's own `appliance.sonos_roam` precedent (a SEPARATE `node.sonos_roam` ping
device exists in Physical.def but is NEVER explicitly positioned in Conceptual.def at all -- it
auto-materializes internally; only the LOCAL-kind `appliance.sonos_roam`'s own combined
Logical.def-derived "node" gets the real "infrastructural:node" positioning). Same shape applied to
Junglinster's kitchen Sonos:
- Physical.def: renamed the existing hosts-kind ping device `appliance.sonos_kitchen` ->
  `node.sonos_kitchen` (freeing the id, same pattern as garage/appletv_living_room/
  sonos_playbar_livingroom before it), added a NEW local-kind `appliance.sonos_kitchen` (
  `media_player.core`/`sensor.battery_level`/`switch.media media_player.social_house_kitchen_sonos`).
- Logical.def: `derived binary_sensor.node with: condition physical appliance.sonos_kitchen and
  node.sonos_kitchen; end;` + `derived binary_sensor.battery_alert with: condition jinja
  ${int_less_then}(10) { sensor.battery_level }; end;` (threshold "10" matches the OLD macro's own
  `call battery_alert :sonos with alert_level 10;` exactly).
- Conceptual.def: replaced the OLD macro-driven block (`call media_player_device :sonos;`/`call
  battery_alert :sonos with alert_level 10;`, confirmed via the SHARED `Macros.def`'s own
  `media_player_device` macro body that these produced exactly `media_player.social:sonos`/
  `switch.social:sonos/media` -- not raw/native entities, genuinely DSL-managed all along) with the
  modern single-device form, matching Vienna's sonos_roam shape verbatim.
- Regenerated clean, zero warnings -- all 5 resulting entity_ids
  (`media_player.social_house_kitchen_sonos`, `switch.social_house_kitchen_sonos_media`,
  `sensor.infrastructural_house_kitchen_sonos_battery_level`,
  `binary_sensor.infrastructural_house_kitchen_sonos_battery_alert`,
  `binary_sensor.infrastructural_house_kitchen_sonos_node`) confirmed to match the ALREADY-LIVE
  ones exactly (checked against the live registry pulled earlier today). Full test suite green.
  **Known likely follow-up once deployed**: kitchen sonos's own "node" entity already had a
  pre-existing "_2"-suffixed template-platform orphan sitting at its clean unique_id BEFORE this
  migration (found in the same registry pull used for the garage_sonos/playbar fixes) -- once this
  migration's own new "node" derivation tries to claim that same clean name, the same
  stale-tombstone-collision playbook (already used twice today) will likely be needed again after
  deploy; not pre-emptively fixed, since the exact collision shape can only be confirmed live.

**Kitchen/workplace furnace switch migrated, 2026-09-28.** The old bare `entity switch.physical:
furnace;` + `entity switch.social:furnace with definition as flipped switch.physical:furnace;`
pair was genuinely undeclared (zero Physical.def presence) -- "furnace" here is a SEPARATE switch
device (`house/laundry_kitchen/heating/furnace_switch` in the live registry, despite its current
positioning being kitchen/workplace -- the user confirmed this positioning is correct, just a
stale device-registry area label), NOT the nearby `sensors.house_heating_power_meter` (a pure CT-
clamp power sensor). The user's own explanation: this switch drives a normally-closed triple relay
for the furnace's three phases -- raw "off" means the furnace IS powered, hence the "flipped"
wrapper. Confirmed "flipped" (`definition as flipped <source>;`, Conceptual.def) is itself a
STILL-CURRENT, non-deprecated mechanism (`generateFlippedSwitches`/`buildFlippedSwitchYAML`,
generator.go) -- not something needing its own migration, just needed a properly-declared source.
Also confirmed by reading `parser.go`'s own handling: a flipped switch's source is resolved PURELY
TEXTUALLY (`normalizeEntityFullName`, no registered-entity lookup at all) -- meaning the bare
`entity switch.physical:furnace;` positioning line was never load-bearing for "flipped" itself, but
IS still needed to trigger the raw switch's own MQTT discovery publish in the first place (a bare
Physical.def declaration alone does NOT get published -- confirmed via this session's own earlier
Phase-1 cross-check finding) -- so it stays, now properly sourced from a real Physical.def
declaration instead of an unverified bare placeholder. Migrated:
- Physical.def: `device sensors.kitchen_workplace_furnace_switch with: identifiers
  "zigbee2mqtt_0xf4b3b1fffed648f5"; switch.core 0xf4b3b1fffed648f5_switch_zigbee2mqtt; end;` -- no
  "node" capability added (nothing referenced this device's own liveness before, matching the
  original construct's own minimal scope, not adding unrequested behaviour).
- Conceptual.def: `device sensors.kitchen_workplace_furnace_switch with entity switch.physical:
  furnace from switch.core;` (one-liner) + the SAME, unchanged "flipped" line.
- Regenerated clean, zero warnings. Both resulting entity_ids
  (`switch.physical_house_kitchen_workplace_furnace`, `switch.social_house_kitchen_workplace_furnace`)
  confirmed to match the already-live raw entity and the flipped template's own generated logic
  read directly (`is_state(..., 'off')` / turn_on->turn_off/turn_off->turn_on on the source) --
  matches the user's own description exactly. Full test suite green. **Not deployed.**

**Real feature added: physical-level payload inversion ("inverted" qualifier), 2026-09-28.** The
user corrected the furnace switch's own architecture (see above): rather than a raw "physical"
entity + a Conceptual.def "flipped" template wrapper (two entities), they wanted ONE entity,
positioned directly at `switch.social:furnace`, inverted AT THE PHYSICAL/DISCOVERY level if at all
feasible -- "otherwise we need to do it on the logical level." Investigated and confirmed feasible:
HA's own MQTT switch/binary_sensor schema uses `payload_on`/`payload_off` for BOTH the command sent
on turn_on/turn_off AND (absent a `state_on`/`state_off` override) how state_topic's payload is
interpreted -- swapping these two strings in the relayed discovery config is a complete,
zero-template physical-level inversion. Implemented as a real feature spanning both Go modules:
- **`homeassistant` (generator)**: new `Inverted bool` field on `TDiscoveryCapability`
  (integration_discovery_storage.go); new `"inverted "` leading qualifier on a capability line
  (integration_discovery_parser.go), stackable with the existing `"hidden "` qualifier in EITHER
  order; threaded through `TDiscoveryEntityLink.Invert` (administration.go) ->
  `registerDiscoveryEntityLink` (Conceptual_DiscoveryEntities.go) -> `discovery.yaml`'s own
  `invert: true` manifest line (integration_discovery_generator.go). 5 new tests (2 parser-level
  including the stacking-order case, matching this session's own "hidden" precedent).
- **`house_event_bus_coordinator`**: new `Invert bool` field on `TDiscoveryEntityLink` (main.go);
  `buildRelayedDiscoveryConfig` (discoverybridge.go) swaps `payload_on`/`payload_off` when set --
  ALWAYS (defaulting to HA's own documented "ON"/"OFF" when the native Zigbee2MQTT payload leaves
  them implicit, so the common case still inverts correctly rather than silently no-op'ing) -- and
  swaps `state_on`/`state_off` too, but ONLY when the native payload genuinely uses that override
  (their absence already means "same as payload_on/off", already correctly handled). 3 new tests
  covering explicit native payload_on/off, implicit/defaulted, and the state_on/off-present case.
- Both modules: `go build`/`go vet`/`go test` all green.

**Furnace switch re-migrated to the new mechanism**: Physical.def's `switch.core` capability now
reads `inverted switch.core 0xf4b3b1fffed648f5_switch_zigbee2mqtt;`; Conceptual.def collapsed to
ONE line, `device sensors.kitchen_workplace_furnace_switch with entity switch.social:furnace from
switch.core;` -- no separate "physical" indirection entity, no "flipped" wrapper at all. Regenerated
clean, zero warnings -- `discovery.yaml`'s own entry now reads `gateway: sensors.
kitchen_workplace_furnace_switch` / `leaf: 0xf4b3b1fffed648f5_switch_zigbee2mqtt` / `invert: true`,
positioned directly as `switch.social_house_kitchen_workplace_furnace`. Vienna regenerated
**byte-identical** (confirms the new feature is purely additive, zero effect where unused). Full
test suite green on both modules. **Not deployed** -- this one needs BOTH a generator regenerate
AND a coordinator rebuild+restart to take effect (the swap logic itself lives in the coordinator
binary), so it should wait for the next full deploy pass alongside everything else already queued.

## Real fix: tabs/CR now treated as "just another space" everywhere, 2026-09-28 (late)

The user's own long-standing report (raised more than once): a literal TAB character, or a non-LF
line ending, wasn't being treated as "just another space" anywhere in this DSL. Root cause found:
this codebase has DOZENS of hand-rolled literal `strings.Index(line, " with X ")`-style single-
ASCII-space substring searches scattered across parser.go/expander.go/every integration_*_parser.go
-- each one only ever matched a genuine space character, so a tab (common from an auto-indenting
editor, or pasted mixed-whitespace text) silently broke the match with no warning at all, rather
than "just working" the way any other run of whitespace already did. Fixing each call site
individually would have been a much larger, riskier change than fixing it once at the true single
source: **`normalizeDefinitionWhitespace`** (new function, defined.go), applied inside
`readOptionalFile` (the shared low-level read every `.def`-family file goes through --
`collectLayerContent`/Settings.def/Defaults.def all funnel through it) and, separately, at
Macros.def's own direct `os.ReadFile` call site (expander.go's `loadMacroContext`, which bypasses
readOptionalFile on purpose -- a missing Macros.def is a real error, not "silently empty"). Every
`.def`-family file is now normalised the INSTANT it's read, before any layer/macro/parser code ever
sees it: CRLF and lone-CR line endings both become a plain "\n", and every literal TAB becomes a
single ASCII space -- so every existing single-space-literal search just works, with zero changes
needed at any of those dozens of call sites. Deliberately does NOT collapse repeated real spaces
(never broken) and does NOT special-case quoted-string content (a tab/stray-CR inside a quoted
jinja template or icon string is exactly as unlikely to be meaningful as one outside it -- doing
that properly would need the same quote-tracking machinery logical_condition_expr.go's own
tokenizer already has, for no realistic benefit here). 6 new unit tests (defined_test.go, a new
file) plus an end-to-end one proving a real on-disk file with a tab and CRLF comes back from
`readOptionalFile` already clean. Full test suite green; both houses regenerated **byte-identical**
-- confirms this is a pure latent-bug fix with zero effect on either house's current, already-clean
`.def` files. **Not deployed** -- purely a generator-side fix, no coordinator rebuild needed for
this one, but still queued for the same morning deploy as everything else.

## Two more real bugs found+fixed: domain-tail-duplicate naming + a WIDESPREAD area-name bug, 2026-09-28 (late)

The user reported `cover.social_house_kitchen` misnamed as `..._cover` ("patterns like DDD.path/DDD
should always be renamed to DDD.path") -- investigated, root-caused, and while checking the fix's
own side effects, the user ALSO reported an odd friendly name (`house/kitchen//cover`, double
slash) and an even stranger suggested area (`social/house/social:kitchen`) on the same device.
Turned into THREE separate, real findings:

1. **The immediate `.def` bug**: Junglinster's `utility.somfy_kitchen` wrote `entity
   cover.social:cover from cover.cover;` -- EVERY other cover in the same file (8+ sites) correctly
   uses the established empty-leaf convention, `entity cover.social:     from cover.cover;`. Fixed
   to match. This alone isn't a generator bug -- `resolveDeviceEntityFullName`'s own domain-tail
   stripping (`stripEntityOwnTrailingDomainSegment`, confirmed with the user back on 2026-09-24 for
   exactly this "light.social:light" shape) is deliberately gated on `deviceNamePath != ""`
   (skipped for a device with no "as" leaf, like `utility.somfy_kitchen`) -- ungating it risks
   reintroducing a real, once-hit regression (`remote_instance_entity_reporting_test.go`'s own
   roomba vacuum case, where an apparently-identical domain-matching tail is NOT redundant).
   Investigated the exact distinguishing signal needed to fix this generally without that risk;
   didn't find a safe one within the time available -- flagged as a genuine, still-open generator
   gap (not resolved tonight), matching the .def-level fix for the four+ sites that need it.
2. **Real generator bug: `deviceDisplayName`'s own trailing-slash** (Conceptual_DeviceEntities.go):
   for a device with NO "as" leaf, `strings.Join(segments, "/") + "/" + path` appended `"/" + ""`
   unconditionally, producing `"house/kitchen/"` -- which the hassbridge relay's own display-name
   combiner then doubled into the reported `"house/kitchen//cover"`. Fixed: only append when `path
   != ""`.
3. **Real generator bug, WIDESPREAD: `OpenSpace`'s own area-name computation** (administration.go):
   passed the WHOLE "sphere:leaf" spec (e.g. "social:kitchen") into `deviceSpecLeafPath`, whose own
   "multi-sphere" verbatim-preserving branch (`isMultiSphereDeviceNamePath`) is a DEVICE-"as"-clause-
   specific heuristic (a single non-"infrastructural" sphere token is deliberately a distinct shape
   THERE) -- reused here for an ordinary SPACE spec (which has no such special case), it returned
   the leaf argument UNCHANGED, colon and all, producing `suggested_area: "social/house/social:kitchen"`
   instead of `"social/house/kitchen"`. Fixed: use the already-computed `spaceName[colonIdx+1:]`
   directly, bypassing the device-specific helper entirely. **This turned out to affect ESSENTIALLY
   EVERY "as area" space in BOTH houses** whose sphere isn't "infrastructural" ("social" being the
   overwhelming common case) -- confirmed via byte-diff: 30 lines changed in Junglinster's own
   devices.yaml, 39 in homeassistant_bridge.yaml (every room's own suggested_area/nested-space
   device_info corrected), similar counts in Vienna. `hass/` itself untouched on both houses (no
   entity_id ever depended on this -- purely display_name/suggested_area metadata), confirming this
   was a real, long-standing, but purely cosmetic (device-grouping-hint) bug, not a functional one.

4 new tests (`Conceptual_DeviceEntities_test.go`, a new file: 3 for `deviceDisplayName`, 1 for
`OpenSpace`'s own area-name computation via `CurrentArea()`). Full test suite green. Both houses
regenerated -- zero warnings, `hass/` byte-identical, only `devices.yaml`/`homeassistant_bridge.yaml`
(Junglinster) and `devices.yaml`/`homeassistant_bridge.yaml`/`imported.yaml` (Vienna) changed, all
confirmed to be exactly the expected fix (trailing slashes gone, colons gone). **Not deployed** --
queued for the same morning deploy as everything else; this one only affects coordinator-published
metadata (device names/areas in the HA UI), no entity_id/behaviour change, so it's safe to bundle in.

**Resolved properly, 2026-09-29 (same session, continued): the "still open" gap above was a wrong
call, corrected by the user.** The gating on `deviceNamePath != ""` was WRONG -- the user's own
explicit instruction: the domain-tail strip "should always [be] attempt[ed]... based on the
original name with '/'", independent of whether the enclosing device has an "as" leaf at all (the
user is separately considering dropping "as" from the grammar entirely, making this the NORMAL
case going forward, not an edge case). Fixed: `resolveDeviceEntityFullName`
(Conceptual_DeviceEntities.go) now calls `stripEntityOwnTrailingDomainSegment` UNCONDITIONALLY, no
gate at all.

The once-protected roomba regression (`TestGenerateHassBridgeEntityReportingAutomationsWritesAttributesFile`,
"vacuum.physical:vacuum from hass.roomba vacuum.roomba;") DID break as expected -- its own
expectation was updated to match the new, correct behaviour (entity_id is now
"vacuum.physical_living_room", not "..._vacuum"; automation alias/filename updated to match) rather
than kept as a reason to avoid the fix. The strip only ever removes a TRAILING segment that exactly
equals the domain (confirmed via a new test: "number.social:cover/my_position" is completely
unaffected, no genuine multi-segment path is ever touched) -- so this isn't a blanket "ignore
domain-looking words" rule, just closing the one real gap.

2 new direct regression tests added (`Conceptual_DeviceEntities_test.go`): the exact
`utility.somfy_kitchen`-shaped case (no "as" leaf, bare domain-matching leaf, confirms
`cover.social_house_kitchen` not `..._cover`), and a negative case confirming genuine multi-segment
paths are never touched. Full test suite green (after updating the 2 roomba-test assertions to the
new, correct expected values). Regenerated both houses: **Vienna byte-identical** (this exact
shape never occurs there); **Junglinster** cascades a real, deliberate entity RENAME through every
script/automation/helper built around the kitchen cover's own entity_id (input_number/input_select/
script/automation files, all consistently dropping the redundant "cover" suffix) -- a genuine fix
propagating everywhere it should, not a stray side effect. **Not deployed** -- this one IS a real
entity rename (unlike the purely-cosmetic area-name fix above), so it needs the same registry-
collision watchfulness as every other rename this session once it does go out.

Next in Phase 3: continue a GENUINE line-by-line read from Conceptual.def's own "#### HERE" marker
(line ~391) onward -- not a keyword grep -- for whatever legacy constructs remain (more `call
providing`/`call media_player_device`/`call battery_level_device` sites already spotted further
down, but also anything else not yet enumerated, per this session's own fresh lesson). **Nothing
from this Phase 3 work deployed yet** -- bloom_left, the table lamps, corb, nespresso's
brews/in_use, kitchen sonos, and the furnace switch (now using the new "inverted" mechanism) are
all local-only, regenerated + verified but not pushed.

## Everything above DEPLOYED + VERIFIED LIVE, 2026-09-29 (morning) -- "OK. Now we can deploy to
## test our earlier changes."

Full deploy pipeline run on both houses (HA files + coordinator rebuild/restart). Two real registry
collisions surfaced during verification and fixed via the established registry-surgery playbook
(both a fresh occurrence of the same stale-tombstone/orphan class hit repeatedly this session, not
new bug classes):
- **Furnace switch**: retiring the OLD Conceptual.def "flipped" wrapper (a template-platform
  switch) in favour of the new single-entity `inverted` mechanism left the OLD template entity as a
  genuine (non-tombstoned) LIVE orphan still occupying the clean
  `switch.social_house_kitchen_workplace_furnace` name, bumping the new relayed entity to `_2`.
  Fixed by deleting the orphaned live template entry outright (not just a tombstone this time), then
  renaming `_2` back to clean.
- **Kitchen sonos node**: the exact same stale-`deleted_entities`-tombstone pattern as
  garage_sonos/playbar_livingroom the prior night -- fixed identically (tombstones removed, `_2`
  renamed to clean).

**User confirmed live: "Furnace works."** -- the `inverted` mechanism's on/off direction is
correct (raw "off" on the normally-closed relay genuinely means the furnace is powered, and the
relayed `switch.social_house_kitchen_workplace_furnace` now reports/commands the INTENDED sense).

## Hassbridge/discovery relay friendly-name bug FIXED, 2026-09-29 -- "Let's fix this."

Verification during the deploy above surfaced a garbled live `friendly_name` on two unrelated
entities -- `cover.social_house_kitchen` ("house/kitchen house/kitchen/cover") and
`switch.social_house_kitchen_workplace_furnace` ("house/laundry_kitchen/heating/furnace_switch
social_house_kitchen_workplace_furnace") -- flagged as "purely cosmetic, worth a proper look
later," then explicitly assigned: "Furnace works. ... Let's fix this."

Root-caused to TWO separate code sites in `house_event_bus_coordinator`, both the same class of
mistake `TBinarySensorDiscoveryPayload`'s own doc comment (discovery.go) already fixed once before:
HA combines a non-null MQTT discovery "name" with device.name for any device-grouped entity, so a
"name" that already duplicates (or embeds) the device's own name doubles up in the UI. Neither site
had adopted that established `Name *string`-nil-means-"device name alone" pattern:

1. **`discoverybridge.go`'s `buildRelayedDiscoveryConfig`** (raw Zigbee2MQTT/EMS-ESP-style
   discovery-kind relay -- the furnace switch's own path): `body["name"] = objectID` unconditionally
   -- the entity's own full sanitised object id (e.g. "social_house_kitchen_workplace_furnace"),
   never null, always combined by HA with device.name. Fixed with a NEW distinction,
   `soleGatewayCapability` (computed once per subscribe via the new
   `discoveryEntityLinksPerGateway` helper, counting entity_links per gateway id -- confirmed via a
   live `discovery.yaml` pull that many gateways (Aqara multi-sensors, EMS-ESP) genuinely relay
   SEVERAL capabilities from one physical device, so a blanket null would have flattened every one
   of THEIR display names identically, an equally real regression): when true (the furnace switch's
   own gateway relays exactly one capability), "name" is now a literal JSON null. When false, "name"
   is left as whatever the gateway's own native payload already set (Zigbee2MQTT/EMS-ESP already
   publish a short, genuinely distinguishing per-entity label there -- "Occupancy"/"Battery"/"Boiler
   Temperature" -- unrelated to this DSL's own entity_id positioning, so there was never a real
   "stale name after a reposition" risk the old override was actually guarding against).
2. **`discoveryhassbridge.go`'s `buildHassBridgeEntityDiscoveryBody`** (the cover's own path, a
   bridged remote-HA-instance capability): `name := devBlock.Name; if DisplaySuffix != "" { name =
   devBlock.Name + "/" + DisplaySuffix }` -- for the cover (DisplaySuffix "cover"), this produced
   "house/kitchen/cover", a non-null string HA then combined with device.name ("house/kitchen")
   into the reported doubled string. Fixed: DisplaySuffix empty -> literal JSON null; DisplaySuffix
   non-empty -> DisplaySuffix ALONE (e.g. just "cover"), letting HA's own combination logic do what
   it was always going to do anyway, matching `TSensorDiscoveryPayload`'s own established
   leaf-label convention.

4 new/updated tests (`discoverybridge_test.go`:
`TestBuildRelayedDiscoveryConfigNameNullForSoleGatewayCapability`,
`TestBuildRelayedDiscoveryConfigNamePassesThroughForSharedGatewayCapability`;
`discoveryhassbridge_test.go`: `TestSubscribeHassBridgePublishesDiscoveryAndState`'s own assertion
corrected from the old buggy expected value to "status" (the bare DisplaySuffix), plus a new
`TestSubscribeHassBridgePublishesNullNameWhenDisplaySuffixEmpty` built directly off the
`utility.somfy_kitchen` shape). Full coordinator test suite green; generator module unaffected
(confirmed still green, no source in `homeassistant/` touched). **Deployed and confirmed live**
(coordinator rebuilt + restarted on both houses' protocols-server-1) -- `light.social_house_kitchen`'s
own friendly_name confirmed clean afterward.

## Real, pre-existing registry collision found live during that same deploy: corb

Not caused by today's coordinator deploy -- confirmed pre-existing since corb's own 2026-09-28
migration, whose own verification pass checked 3 other devices (washing_machine, garage_sonos,
sonos_playbar) but never checked corb. The user reported it directly: "z2m agrees [corb] IS
available" while HA showed the entity `unavailable`/`restored: true`. Root cause: `light.
social_house_corridor_corb` (the clean name) was still squatted by an old, non-tombstoned *raw*
Zigbee2MQTT registration (unique_id `0x0017880104a1e1f7_light_zigbee2mqtt`, predating corb's own
move to the coordinator relay), leaving the real, live, relayed entity (unique_id
`sensors_corridor_corb_0x0017880104a1e1f7_light_zigbee2mqtt`) stuck at `_2`. Fixed via the
established registry-surgery playbook (backup, `ha core stop`, remove the orphan + rename `_2` to
clean, `ha core start`) -- confirmed live afterward: `light.social_house_corridor_corb` reports
`on`/`off` correctly, `_2` no longer exists.

## Real parser bug found+fixed: a wrapped one-liner "with" clause, 2026-09-29

The user's own long-standing complaint ("we still don't treat tabs... and returns as 'just another
space'") had one further real gap beyond the earlier tab/CRLF fix: an editor-wrapped one-liner
"device X with <statement>;", broken right after the bare word "with" (found live: Junglinster's
own Conceptual.def:517, `switches.house_kitchen_hallway_door_power_socket_zwave_057`'s own kitchen-
main positioning) -- neither `deviceWithBlockHeaderPattern` (needs "with:") nor
`deviceWithOneLinerPattern` (needs the whole statement on the same physical line) recognised it,
silently dropping the positioning from any FUTURE regeneration (the live-deployed entity, from
before the wrap happened, kept working the whole time -- confirmed: "the lights go on/off when I
use it from the UI" -- which is why nothing looked broken).

Fixed: `joinWrappedStatementContinuations` (`defined.go`, folded into `normalizeDefinitionWhitespace`)
-- a line ending in the standalone word "with" (never "with:") is joined, single-space, to whatever
follows, until a real terminator (`;`/`:`) or a blank line. **Real regression caught before
shipping** (own doc comment has the full story): the first version didn't exempt comment lines, and
a `# ... with` comment two lines above a REAL `device ... with:` header (Physical.def's own
aqara_magnet device) got swallowed into it, corrupting that header off its own line and breaking
the whole block (5 "unrecognised line" warnings, caught by re-running `./generate` and reading the
warnings, not assumed clean). Fixed by excluding comment lines (`isCommentLine`) from ever
triggering OR being swallowed by the join. Both houses regenerate clean, zero warnings, full test
suite green (`TestJoinWrappedStatementContinuations`, several cases including the comment-line
regression). **Confirmed NOT a live bug**: byte-diffed clean against Junglinster's currently-
deployed state -- the one affected line was already correctly deployed before whatever edit wrapped
it, so nothing needed redeploying for this fix alone.

Also investigated and **ruled out** a suspected gap while digging into this: `light.
social_house_kitchen_main`/`light.social_house_wc_main` (node 057's two endpoints) looked "missing"
purely because the check searched the wrong output tree (`hass/`, for YAML-template entities) --
they're discovery-kind (MQTT relay) entities, correctly present in `coordinator/discovery.yaml`,
confirmed live and matching the registry exactly. No bug; own investigation mistake, corrected to
the user directly.

**`CHECKPOINT.md` rewritten** (2026-09-29) to reflect current state, ahead of a possible `/clear` --
see that file directly for a short-pointer summary of everything above; this plan file remains the
deep detail log.

## New "when" rule construct + two real migrations, 2026-09-29

The user's next migration target (`entity timer.social:removing_smell with definition as timer
"00:03:00"; limits timer.social:removing_smell fan.social: off on;`) needed several genuinely new
things, all now built:

**1. New space-level DSL construct**: `when <event-entity> turns on|off then turn <action-entity>
on|off;` (`TWhenRule`, administration.go; parsed inline in parser.go's existing space-body loop,
same place `follows`/`limits` already live). **Deliberately minimal first cut** (confirmed via the
user's own "Later" framing for the fuller design): no "if <gate-condition>" clause, no and/or/not
composition of multiple event atoms, no "<event> received" atoms, no multi-action block form --
none of that is needed by either real site this was built for; extend when a real site needs more,
per this session's own "don't build beyond what's needed" discipline. Malformed lines are silently
skipped, matching `follows`/`limits`' own local precedent (this function has no warnings-collection
path otherwise).

**2. `follows` confirmed as literal permanent sugar for two `when` rules** -- `follows <follower>
<leader>;` conceptually desugars to "leader turns on -> follower on; leader turns off -> follower
off". Kept its OWN dedicated relation type/generator (unchanged automation entity_ids on an
already-live device -- corb's own automations didn't need to churn) rather than being rerouted
through `TWhenRule` internally, but `generateFollowsAutomations` and the new
`generateWhenRuleAutomations` now share their actual per-automation YAML-building
(`buildStateTriggerAutomationYAML`, generator.go) -- a real refactor, not just a comment, proven by
`TestFollowsStillProducesTwoAutomationsViaSharedBuilder` (follows' own output unchanged: still
exactly 2 automations, same alias/trigger/action shape).

**3. Corb's own two `follows` lines collapsed to one**, per the user's own explicit instruction:
`light.social:main` is corb's switched-by combo's own SelfEntity, whose state already follows the
raw switcher -- the switcher-targeted follows line was redundant. Regenerated: byte-diff confirms
exactly the 2 "switcher" automations disappear, nothing else changes.

**4. `timer.social:removing_smell` fully migrated off the old `limits`/`TTimerLimitsRelation`
mechanism** (a hardcoded, single-purpose start/cancel/finish/watchdog timer dance --
`generateTimerLimitsAutomations`, 4 automations including an easily-missed per-minute "was" watchdog
that restarts the timer if the fan is on but the timer isn't running -- found while byte-diffing, not
initially expected). New mechanism, all via already-EXISTING machinery (no new capability grammar
needed at all -- a bare `condition <label>;` already meant "is on" all along, confirmed by reading
the renderer before assuming a new "is on" atom was needed):
- **New device** `sensors.house_wc_fan` (Logical.def) absorbs node 058's own `fan_1` endpoint as
  `fan.core` (`capabilities from switches.house_kitchen_hallway_door_switch_box_zwave_058: fan.core
  fan_1; end;` -- caught and fixed one real syntax slip while writing this: absorb-block capability
  lines are whitespace-only now, no colon, per the session's own earlier "capability colon dropped
  from grammar" work -- `fan.core fan_1;` not `fan.core: fan_1;`).
- **New derived capability** `binary_sensor.done_removing` on that same device: `condition core;
  delay_on 00:03:00;` -- becomes "on" once the fan has been continuously on for 3 minutes.
- **Conceptual.def**: `sensors.house_wc_fan` positioned with `entity binary_sensor.social:done_removing;`
  + `entity fan.social from fan.core;` (replacing the old bare positioning directly on node 058's
  own device id, which mixed an unrelated functional capability onto a purely-physical device id --
  matching `sensors.house_kitchen_workplace_dimmer`'s own established precedent of a dedicated
  wrapper device per functional capability).
- **The `when` rule**: `when binary_sensor.social:done_removing turns on then turn fan.social off;`
- **The old `entity timer.social:removing_smell with definition as timer "00:03:00";` declaration
  dropped entirely** -- the new mechanism needs no `timer.` helper at all (HA's own template
  binary_sensor `delay_on` handles the debounce natively, and turns out to be MORE restart-robust
  than the old timer dance, which needed that per-minute "was" watchdog specifically to recover from
  timer-entity restart quirks). Confirmed safe to drop: grepped the LIVE `/config/automations.yaml`/
  `scripts.yaml` for "removing_smell" first -- zero hits, nothing else references the timer entity.

Both houses regenerated: Junglinster's diff is EXACTLY the intended change set (corb's 2 switcher
automations gone, removing_smell's old 4 automations + timer entity/customization gone, the new
done_removing entity + its one `when`-rule automation added) -- confirmed via `diff -rq` against a
pre-migration snapshot; **coordinator/discovery.yaml is BYTE-IDENTICAL** (fan.social_house_wc still
resolves to the exact same gateway/leaf pair as before, just via the absorb path instead of a direct
device positioning -- zero live MQTT disruption). Vienna regenerated fully byte-identical (inert,
as expected -- new construct, unused there). 4 new tests (`logical_when_rule_test.go`), full
generator test suite green. **Not deployed** -- local + regenerated + verified only, awaiting the
user's go-ahead as always.

Next: continue Phase 3's sequential read of Conceptual.def from wherever this session's own earlier
"#### HERE" marker work left off, watching for more legacy constructs the same way this one was
found (not by keyword-grepping, per the session's own established lesson).

## Newline-as-space generalized + mandatory domain-qualification for condition refs, 2026-09-29

The `when`-rule work above immediately hit a live parse failure: a wrapped `"is switched by"`
one-liner (`sensors.house_kitchen_workplace_dimmer`), a DIFFERENT wrapped shape than the
`"with"`-only fix handled. **The user's own explicit, forceful correction**: the narrow "ends in
bare 'with'" trigger was the wrong FIX SHAPE entirely, not just incomplete -- "newlines should be
treated like spaces... All over the grammar!!!!" Given via `AskUserQuestion` the choice between a
general reflow fix now vs. a full character-stream/token-stream rewrite of the whole .def parser
(GO_CONVENTIONS §7/§8's three-layer CDL1 architecture, matching `logical_condition_expr.go`'s own
design) -- the user picked the general fix now, with the full rewrite explicitly deferred to a
later "code cleaning" pass (memory `feedback_no_new_regexp_parsers` updated with this exact
exchange, and PROJECT.md item 15 got a dated note recording today's saga as concrete evidence for
that future pass).

**`joinWrappedStatementContinuations` generalized** (`defined.go`): any physical line not ending,
OUTSIDE any quoted string, in a real terminator (`;` for a statement, `:` for a block header) is
now joined into the next line, single-space, until one does. Comment lines and blank lines are
exempt in both directions (never trigger, never swallowed). Two real bugs found and fixed WHILE
building this, each caught by actually regenerating and reading the result rather than trusting
the tests alone:
1. A blank line has no trailing terminator of its own, so without an explicit exemption it was
   misread as "incomplete" and swallowed the paragraph after it -- fixed (`isCommentOrBlankLine`).
2. Far more serious: quote-tracking didn't exempt trailing inline comments, and this codebase's
   comments are free-form English prose that routinely quotes phrases with an ODD count of `"`
   (typos, or just informal quoting) -- one such comment anywhere in the file left quote-tracking
   stuck "open" for the rest of the scan, since the function re-scans the whole growing
   accumulation each time a line is appended, swallowing everything after it. Confirmed live: this
   ate the REST OF Junglinster's Conceptual.def, losing the outermost `end;` entirely ("block
   starting 'conceptual layer with:' is missing its closing 'end;'"). Fixed: quote-tracking now
   stops at an outside-quote `#`, exactly matching how every other per-line consumer in this
   codebase already strips trailing comments.
3. A third, genuinely pre-existing issue surfaced once both of the above were fixed and the file
   STILL failed the same way: `space off: @light` (Conceptual.def:536, no trailing `;`) -- a real,
   previously-silent typo the old lenient per-line parsers tolerated (they just no-op'd a
   `TrimSuffix(x, ";")` that found nothing to trim) but the new, stricter general model correctly
   flags as incomplete. Fixed as a `.def` edit (added the missing `;`), not a parser change -- this
   is exactly the kind of latent inconsistency the general model is SUPPOSED to expose.

8 tests total (`defined_test.go`), including the live "is switched by" wrap, a multi-line boolean
condition (already-legitimate DSL author layout, confirmed inert), a genuine mid-line `;` inside a
quoted jinja string, and the inline-comment-quote regression. Both houses regenerate clean, zero
warnings.

**Mandatory domain-qualification extended to plain `condition`/`value` sibling refs** (the user's
own second, simultaneous correction: "that 'core' without domain should be illegal" -- they'd
already renamed their own new fan device's capability from `core`/`fan_1` to bare `fan.fan`,
domain-qualifying the condition ref themselves, ahead of the grammar catching up). Generalizes the
SAME rule already enforced for jinja `{ ... }` sources and `"entity X from Y Z;"`
(`Conceptual_LogicalEntities.go`'s `resolveLeaf`, same actionable-suggestion style via
`lookupCapabilityDomain`). Blast radius, both houses: 2 sites (Junglinster's nespresso
`condition brews;` -> `condition binary_sensor.brews;`, and a GENERATOR-side gap --
`logical_enabler_dependency_sugar.go`'s own synthesized `"<label> is available"` text was itself
bare, fixed to `d.EnabledBy.OwnDomain + "." + d.EnabledBy.OwnLabel`). Vienna: fully clean, zero
`.def` changes needed.

**A second, more serious real bug found WHILE fixing the first**, caught by byte-diffing rather
than trusting "zero warnings, tests green": once `resolveLeaf` started passing a genuinely dotted
token (`"media_player.core"`) into `resolveTargeted` for the first time ever (previously this path
only ever saw bare, undotted tokens, so this code was never exercised), a PRE-EXISTING latent bug
in `resolveLogicalSiblingSourceEntityID`'s own tier-4 literal-entity-id fallback
(`!mustBeSibling && strings.Contains(token, ".")`) became reachable: tier-4 has no notion of
`r.finalAttempt` at all, so it "succeeds" by treating a genuinely-real-but-not-yet-LINKED capability
as a literal entity_id on the very FIRST resolution attempt -- permanently locking in the wrong
answer instead of deferring to a later pass once the real Conceptual.def positioning is parsed.
Confirmed live: `appliance.tv_living_room`'s own "enabled by" sugar rendered
`states('media_player.core')` verbatim (a nonsense literal) instead of resolving to the real
`media_player.social_house_living_room_tv`; the new fan device's own `done_removing` binary_sensor
hit the identical bug (`is_state('fan.fan', 'on')` instead of `fan.social_house_wc`), and a
degenerate `customization/fan/fan/fan.fan.yaml` file appeared from the same root cause. The
JINJA-source code already solved this exact problem correctly (`mustBeSibling=true`, checked via
`lookupCapabilityDomain` first) -- `resolveLeaf` now mirrors that identical disambiguation:
domain-qualified AND a real declared capability -> `mustBeSibling=true` (forces proper
deferred/retried sibling resolution, tier-4 suppressed); domain-qualified but NOT a recognised
capability (a genuine literal, e.g. an `"over:"`-style external entity_id) -> `mustBeSibling=false`
(original permissive literal-fallback behaviour, unchanged). New regression test
(`TestResolveLeafDefersRatherThanLiteralFallbackForRealCapabilityNotYetPositioned`) drives the
exact two-pass (not-yet-positioned -> positioned) scenario directly. Full test suite green (2 new
tests total for this fix, on top of the 8 for the whitespace generalization and 1 for the plain
mandatory-domain rule itself).

**Both houses regenerated and verified correct after all of the above, together**: Junglinster's
diff is exactly the expected set (tv_node/tv_media/done_removing content corrected to reference
real entities instead of literals, the broken `fan/fan/fan.fan.yaml` artifact gone,
`main_entities.yaml`/`devices.yaml` reflecting the corrected dependency chain); **Vienna is fully
byte-identical, zero diff** -- meaningful confirmation given Vienna's own `appliance.tv` exercises
this exact "enabled by" code path too. **Nothing deployed** -- all local, regenerated, and verified
only, awaiting the user's go-ahead as always.

Real lesson reinforced, not new: every one of today's fixes that looked "obviously correct" in
isolation (the domain-qualification rule, the enabler-sugar text change) had a real, live-breaking
consequence one layer down that only surfaced by actually regenerating both houses and reading the
diff -- "tests green" was never sufficient on its own for any of these.

## Two more Phase 3 migrations + two real compound-"node"-leaf gaps found+fixed, 2026-09-29

**Three small migrations, each following the by-now-established "find the live device, position it
properly" playbook**: Junglinster's `entity light.social:main;` (dangling, no device at all) at
Conceptual.def:657 turned out to have a twin at :686 (same text, second_floor instead of
first_floor) plus one more shape-alike at :436 (`light.social:red_highboard`) -- all three
confirmed live as `switch_as_x`-wrapped plain Zigbee2MQTT relay switches (matching bloom_left/corb's
own precedent), migrated to real `light.core` discovery-kind declarations. Several OTHER
superficially-similar dangling entities (`switch.physical:heating_raw`, various `call
battery_level_device`/`call media_player_device` pairs) were found but correctly left alone -- each
paired with an existing macro call, a DIFFERENT, already-tracked legacy-macro category, not this
"bare, unexplained, assumed-to-exist" shape.

**ring_chime_hallway/ring_chime_corridor unified** into one consistent shape (the user's own
explicit target): dropped their `as ring-chime-X` leaves, added compound leaves
(`ring_chime/node`/`ring_chime/volume`) directly. This surfaced TWO real, narrow gaps in shared
resolution code, both now fixed and regression-tested, both confirmed inert everywhere else
(Vienna byte-identical after each):

1. **`deviceHasCapability`'s hosts-kind branch never recognised "node"** (`Conceptual_DeviceCapabilityEntities.go`).
   `resolveLongestSuffixCapability` (built 2026-09-25) already strips a compound leaf's leading
   segments looking for a real capability match, and is already wired uniformly into the dispatcher
   for every integration kind -- but hosts-kind's own "node" is never a real
   `mat.AttributeNames()` entry (it's synthesized from a ping check), so the one candidate the
   stripping was aiming for always came back "no," silently defeating the already-general mechanism
   for hosts specifically. Fixed: `capName == "node" || slices.Contains(...)`. One new test
   (`TestHostCapabilityEntityLinkResolvesCompoundNodeLeafViaNoRenameShorthand`).
2. **Absorb's own recursive delegate call required the SOURCE device to already have its own
   Conceptual.def positioning**, purely for the "node" case (`registerHostCapabilityEntityLink`).
   The user's own explicit push-back, confirmed as the desired semantics: a hosts device referenced
   ONLY indirectly (Junglinster's `node.ring_chime_hallway`, never positioned anywhere on its own,
   reached only via absorb/`condition node.X;`) should still resolve, not require an artificial
   direct reference first. Traced to two genuinely separate resolution subsystems in this codebase
   (Logical.def's own condition-expr resolver, which already falls back to
   `autoMaterializePhysicalCapability` with no such requirement -- vs. absorb's delegate call, which
   went through the SAME strict dispatcher an ordinary `entity X from Y Z;` line uses). Fixed by
   dropping the deferred-retry/error branch for "node" specifically and proceeding with Go's own
   zero-value `TDeviceConceptualLink{}` when no prior link exists -- exactly the same tolerance
   `autoMaterializePhysicalCapability` already proved safe. One new test
   (`TestLogicalAbsorbResolvesHostsNodeWithNoPriorPositioning`), full suite green, both houses
   byte-diffed clean (Vienna fully identical both times).

**One known, narrower gap surfaced but NOT hit today, worth remembering**: the "node" branch's own
fix doesn't set `link.HostIdentity` the way `autoMaterializePhysicalCapability`'s parallel hosts
branch does as a fallback. If a hosts device that's ONLY ever reached indirectly (no direct
Conceptual.def positioning, like `node.ring_chime_hallway`) ever ALSO needs a non-node attribute
(e.g. `cpu/load`-style), `registerHostAttributeEntity`'s own use of `link.HostIdentity` would find
it empty/zero-valued. Neither ring_chime device needs a non-node attribute, so this wasn't
exercised -- flagged here rather than fixed speculatively, in case a future indirectly-referenced
hosts device needs one too.

Junglinster's own Logical.def absorb line for the hallway (`capabilities from
node.ring_chime_hallway: binary_sensor.node; end;`, with the original working `derived
binary_sensor.node with: condition node.ring_chime_hallway; end;` form left commented out
alongside it) was the user's own live edit while this was being diagnosed -- now resolves clean
with the fix in place. Corridor still uses the original `derived`/`condition` form; both are
confirmed correct, just via different (now both working) mechanisms. **Nothing deployed** -- all
local, regenerated, and verified only.
