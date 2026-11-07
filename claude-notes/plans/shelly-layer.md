# The Shelly layer of "integration mqtt" (PROJECT.md 39)

Agreed 2026-11-07 (Erik): this shape ("A"), a statement inside a device of `integration mqtt`, not a
separate `integration mqtt shelly with:` block -- a Shelly stays a raw device with a shortcut, and
can be mixed with hand-written capabilities. FHEM (PROJECT.md 37) gets a layer of the same shape.
An update entity only signals an update; updating stays manual (PROJECT.md 39).
Built 2026-11-07; Junglinster's test plug rewritten in this form, byte-identical output. Generate
writes the implied manufacturer and model into the device as captured metadata, as for others.

## Aim

A Shelly device publishes a fixed, documented MQTT tree, but no discovery. Declaring one by hand in
`integration mqtt` (as Junglinster's test plug is) repeats the device's id in every line and
spells out what is the same for every Shelly of its family. The Shelly layer says it once:

```
integration mqtt with:
  device switches.test_shelly_plug with:
    shelly "shellyplug-6CC3D8";
    is retained;
    capability binary_sensor.node;
    capability switch.switch;
    capability sensor.power;
    capability sensor.energy;
    derived capability binary_sensor.consumes from sensor.power
      via jinja ${int_more_than}(1);
  end;
end;
```

This means exactly what the hand-written declaration means now: the generator expands it into
the same raw capabilities (`TRawTopicCapability`), so positioning, frozen IDs, YAML entities,
retain and shadows stay the generic machinery's. Nothing is Shelly-specific beyond the expansion.

## What `shelly "<id>";` implies

- **The family**, from the id's prefix (`shellyplug-6CC3D8`: `shellyplug`): which capability
  labels exist and the topic each reads (table below). A label the family doesn't have is an
  error at the word.
- **The capabilities stay listed** -- the author chooses which ones the house uses, as for every
  other kind -- but without their topics: `capability switch.switch;`. Generate writes their
  `as unique_id` onto these lines, as it already does for an instance's implied node.
- **Metadata**: `manufacturer: "Shelly"`; `model_id` and `sw_version` read from
  `shellies/<id>/announce` (`model`, `fw_ver`), captured by generate as now. `model` (the product
  name) from a small code table where known (`SHPLG2-1`: "Plug E"), else left to the author.
- **Liveness**: the block the test plug has, with `<id>` filled in -- quiet 3 x 30 s, ask
  `announce` on `shellies/<id>/command`, answer `info` with `wifi_sta.connected` within 30 s,
  verdict on `shellies/<id>/online` (`true`/`false`), ignoring the command topics and `#/retain`.
  A written `liveness with:` block replaces it (e.g. for a longer update period).
- **Not implied: retain.** It is a setting of the device, so the author says `is retained;` (or
  not), as for any raw device.

## The family table (only what a real device has shown)

Gen1, from Shelly's API docs; `<n>` is the channel. A single-channel family uses the plain labels,
a multi-channel one `_1`, `_2`, ... (channel `<n>` is `_<n+1>`), as Zigbee devices already do
(`power_1`, `consumes_1`).

| Family (id prefix) | Label | Domain | Topic (under `shellies/<id>/`) | Details |
|---|---|---|---|---|
| all | `node` | binary_sensor | `online` | on `"true"` |
| `shellyplug`, `shellyplug-s` | `switch` | switch | `relay/0` | command `relay/0/command` |
| | `power` | sensor | `relay/0/power` | unit W |
| | `energy` | sensor | `relay/0/energy` | unit Wh, divided by 60 (watt-minutes) |
| `shellyplug-s` only | `temperature` | sensor | `temperature` | unit °C |
| | `overheated` | binary_sensor | `overtemperature` | on `"1"` |

Other Gen1 families (`shellyswitch25`, `shelly1pm`, `shellydimmer`, ...: `relay/<n>`,
`roller/<n>`, `light/<n>`, `input/<n>`) are added when such a device is in a house and its tree
has been looked at -- not from the docs alone. **Gen2/Plus devices** have another tree
(`<id>/status/switch:0` as JSON, commands over RPC, `<id>/online`); they would be a second table,
and need the value template for JSON that item 39 already lists.

## Code

- The `shelly` statement and implied capability lines in the grammar of
  `parser_mqtt_capability.go`.
- The family table and the expansion in a new `integration_mqtt_shelly.go`; the default liveness
  through the existing `TLivenessDecl`.
- The frozen-ID search already knows implied capability lines (`impliedRawTopic`).
- Junglinster's test plug is rewritten in the new form; generate must give byte-identical output
  (the same unique_ids, topics, liveness and metadata files).
