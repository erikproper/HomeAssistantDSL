---
name: project-logical-layer-defined-and-derived-capabilities
description: "DONE+VERIFIED 2026-09-18/19: Logical.def's 3rd/4th capability shapes -- 'defined <domain>.<label>' (an HA input_number helper) and 'derived <domain>.<label>' (a multi-source $1/$2/... condition over sibling capability labels or literal entity_ids). Three real cases on Vienna: 'windy', 'sunny', and sonos_roam_complement's battery_alert (jinja-template-call condition form). Media-player 'shadow' devices renamed to appliance.*_complement."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-18T19:51:31.666Z
---

Built 2026-09-18, same day as the "absorb" operation ([[project_logical_layer_device_combining]]).
Two new Logical.def capability shapes, both parsed as new nested block kinds inside a
`device <id> with: ... end;` declaration (`integration_logical_parser.go`):

```
device sensors.vienna_terrace_wind with:
  defined input_number.windy_threshold with:
    minimum ${wind_speed_min};
    maximum ${wind_speed_max};
    step ${wind_speed_step};
    icon ${wind_speed_icon};
    units ${wind_speed_unit};
  end;
  derived binary_sensor.windy with:
    condition "($1 in ['unknown', 'unavailable']) or (($1 | int) > ($2 | int))" over:
      wind_speed;
      windy_threshold;
    end;
    device_class ${windy_device_class};
    delay_on  00:01:00;
    delay_off 00:10:00;
  end;
end;
```

- **`defined`**: declares a pure HA `input_number` helper (no device/MQTT backing at all).
  `registerLogicalDefinedInputNumberCapability` (`Conceptual_LogicalEntities.go`) just seeds
  `TEntityRecord.InputNumberMin/Max/Step/Unit/Icon` directly and calls `AppendEntityRecord` --
  reuses the SAME rendering the plain Conceptual-layer `entity input_number.<spec> with: minimum
  ...; end;` construct already has (`expander.go`'s `scanInputNumberBody`), no new YAML writer.
- **`derived`**: a multi-source condition generalizing the existing `IsAvailable` capability's
  fixed single-`$1` shape into an author-supplied Jinja expression over an ORDERED `over:` list of
  sibling capability *labels* (not raw entity_ids) -- `$1`, `$2`, ... in list order.
  `registerLogicalDerivedConditionCapability` resolves each label by reverse lookup, checked in
  TWO places so it sees the complete device, not just its Logical.def overlay (explicit user
  design point): first `administration.LogicalEntityLinks` (DeviceID+Label -- covers
  IsAvailable/switch-coercion/another `defined`/an absorbed capability), then, if not found,
  `administration.DeviceConceptualLinks[deviceID].AttributeEntityIDs[label]` (covers a PLAIN
  capability of the device's own real physical presence, e.g. this device's own imported
  `wind_speed` -- registered via the ordinary import path, which never touches
  `LogicalEntityLinks` at all). A sibling not yet positioned in EITHER place defers exactly like
  `registerDiscoveryAvailabilityEntityLink`'s own sibling lookup -- real bug caught live during
  this exact build (see below).
- **Same-device only** for `over:` -- no `from <device-id>` qualifier per entry. Deliberately
  deferred: the user flagged mid-build that inter-device derivation "needs some foundational
  ontological reflection" and might actually belong at the Conceptual layer instead -- don't build
  cross-device `over:` without that conversation happening first.
- `registerLogicalAbsorbedCapability` was extended (real gap, same session) to ALSO record a
  `LogicalEntityLinks` entry under the HOST device's own id/label (it only used to register the
  absorbed entity under the ABSORBED device's own id via the delegate call) -- otherwise a
  `derived ... over:` capability on the same host device could never find an absorbed sibling by
  reverse lookup.
- **Real gap found live while wiring up Vienna's own `windy`**: Logical.def has no `${name}`
  Settings.def-variable resolution of its own -- unlike Conceptual.def (a handful of targeted
  `resolveSettingsVar` call sites, e.g. icon/input_number fields in `parser.go`) or Macros.def
  (resolved as ordinary macro-argument substitution during expansion). `${windy_device_class}`
  etc. came out completely unsubstituted, literally, in the first generate. Fixed by adding
  `substituteSettingsVariables` (`expander.go`) and running it once over Logical.def's whole raw
  body in `collectLogicalDevicesByID`, before parsing -- `ctx.Settings` threaded in from
  `generator.go`'s own call site (the one and only caller).
- Device rename `node.vienna_terrace_wind` -> `sensors.vienna_terrace_wind` (Physical.def's own
  import-kind declaration, `home_assistant junglinster` qualifier) -- user's own explicit choice,
  a new naming convention marking "this device's own interesting behaviour now lives at the
  Logical layer." `registerDevicePositioning`'s `hasPhysicalPresence` guard (built earlier the
  same day for "absorb") already covered import-kind devices, so the device's own physical
  positioning (constantAttrs/node/radio/battery_level/wind_direction/wind_speed) needed zero
  further changes to keep working once it also gained Logical.def capabilities.
- Verified via diff on Vienna: every VALUE (device_class, delay_on/off, min/max/step/icon/unit,
  the condition's own two `states()` reads) is byte-for-byte identical to the old `windy` macro's
  own rendered output -- only the entity_id changed (now positioned under the
  `netatmo_windmeter` device leaf instead of bare), an accepted rename matching the user's own
  "yes" given for this whole batch of flat-entity-form migrations
  ([[project_flat_entity_form_ban]]). Full writeup: Vienna's own `MigrationNotes.def`.
- Junglinster: confirmed zero-diff regenerate (no Logical.def there at all, `collectLogicalDevicesByID`'s
  new `settings` parameter is a no-op when the file doesn't exist).

**Second real case, 2026-09-19: `sensors.terrace_motion` / "sunny"** (Vienna's terrace Philips Hue
motion sensor, replacing the "sunny" macro — structurally identical to "windy," just its own
simpler `($1|int) > ($2|int)` condition, no unavailable/unknown check). Found and fixed a THIRD
reverse-lookup gap in `registerLogicalDerivedConditionCapability`'s "over:" resolution: it only
checked `LogicalEntityLinks` (Logical.def-declared capabilities) and
`DeviceConceptualLinks[...].AttributeEntityIDs` (import/hassbridge-kind plain capabilities) — it
had NO path to resolve a PLAIN **discovery**-kind sibling (this device's own `illuminance`).
`registerDiscoveryEntityLink`'s own bookkeeping (`DiscoveryEntityLinks`) is keyed by entity_id with
`(GatewayDeviceID, Leaf)`, never touching either of the other two maps. Fixed by adding a third
lookup: `discoveryGatewaysByID[decl.DeviceID].Capabilities[overLabel].Leaf`, then scanning
`DiscoveryEntityLinks` for that `(GatewayDeviceID, Leaf)` pair — mirrors
`registerDiscoveryAvailabilityEntityLink`'s own sibling resolution exactly. Also added the
discovery branch's own capability-level fallback to Logical.def on a miss
(`registerDeviceCapabilityEntityLinkAllowingHidden`) — previously only hassbridge/import had this
fallback (from the "absorb"/"windy" work); discovery needed it too the moment a discovery-kind
device (not just import-kind) gained real Logical.def capabilities.

Also confirmed: a discovery gateway's own device-id does NOT need the "discovery." prefix at all —
it's purely a naming convention (`sensors.terrace_motion` works exactly like
`discovery.terrace_motion` would, registered the same way in `discoveryGatewaysByID`). This is
how the "sensors." device-id convention (started with `sensors.vienna_terrace_wind`, an
import-kind device) extends cleanly to discovery-kind devices too.

**Third real case, 2026-09-19: `appliance.sonos_roam_complement` / battery_alert.** Added a second
form of the `derived` condition head: `condition jinja ${name}(args) over: <one-entry>; end;` as an
alternative to `condition "<expr>" over: ...; end;`, reusing Settings.def's own named jinja
templates (`resolveJinjaTemplateCallsInLogicalConditions`, a text-level rewrite mirroring
`resolveJinjaTemplateCallsInDerivedLines` exactly, run before `parseLogicalLayerBody`) — so
Logical.def's own battery_alert can reuse `${int_less_then}(10)`, the SAME template Physical.def's
battery_alert declarations use everywhere else, instead of hand-writing the expression again.
Restricted to exactly one `over:` entry (a named template's own "$" placeholder is inherently
single-variable).

Also extended `over:` to resolve a **literal, already-qualified entity_id** (contains a `.`) when
nothing matches as a sibling capability label in any of the three lookups — used verbatim, no
deferral. Real motivating case: `sensor.infrastructural_apartment_bedroom_sonos_battery_level` is
an attribute of the underlying media_player, not a capability of `appliance.sonos_roam_complement`
at all, and — this is the important part — **it must stay a bare kind-5 entity, never become a
Logical.def capability of its own.** It's a real, natively-named HA entity (Sonos's own
integration); wrapping it as a capability and positioning it through the device block would make
the generator author a NEW template entity under that SAME already-existing name — a genuine
collision (the same `_2`-suffix registry-zombie class from the Netatmo import cleanup earlier this
session), not a clean consolidation. The literal-entity `over:` fallback is what lets
`battery_alert` read `battery_level`'s value without `battery_level` itself needing to move at
all. The same reasoning applies to the four bare `entity media_player.social:*;` declarations
(living_room's sonos/apple_tv/tv, bedroom's sonos) found during a "which entities in Vienna's
Conceptual.def are NOT inside a device block" audit — they're natively-named too, and stay bare
for the same reason. Renamed the four Logical.def "shadow" devices to `appliance.*_complement`
(user's own naming: they only ever complement an already-existing raw media_player entity, never
standing alone) — `apple_tv`/`tv`/`sonos`/`sonos_roam` all became `*_complement`, both in
Logical.def and every Conceptual.def `from appliance.X` reference.

**Open question, not yet resolved**: `over:` resolution now has to check THREE separate
bookkeeping maps (`LogicalEntityLinks`, `DeviceConceptualLinks.AttributeEntityIDs`,
`DiscoveryEntityLinks` via a gateway-capability lookup) to see "the complete device," and a 4th
kind (hosts) would need a 4th path if a real case ever needs it. Worth considering, if a 4th case
comes up, whether to unify these into one generic "which entity_id did device D's capability C
resolve to" lookup instead of growing the list further — not worth doing speculatively now (only
3 real cases exist, on-demand discipline), but flag this if a 4th one appears.
