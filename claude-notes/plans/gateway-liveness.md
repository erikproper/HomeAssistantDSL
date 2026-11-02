# Gateway liveness: one generic strategy (PROJECT.md 25c) -- draft 2026-11-01

## What we have today

| Shape | Who | Detects |
|---|---|---|
| Last Will | Zigbee2MQTT (`zigbee2mqtt/bridge/state`), Z-Wave JS UI (`zwave/_CLIENTS/ZWAVE_GATEWAY-zwave-js-ui/status`), the coordinator | the process stopping or losing its connection -- not a hang |
| Our ping-pong | ha2mqtt (`instance_liveness.go`) | a hang too: after 5 min silence the coordinator sends `ping`, no `pong` within 30 s means offline |
| Staleness | hosts (`cpu/report`, `mqtt_relay.go`) | no report for 3 min means unavailable |
| External ping | hosts (`ping/report`, cron) | the host not answering on the network |

Tested 2026-11-01 (Vienna): stopping Zigbee2MQTT makes all its entities unavailable, and they come
back with their values. The Last Will works; only a hang is not covered.

## The model

Every liveness check is the same five things; only the values differ per gateway:

1. **Signs of life**: topics whose traffic shows the gateway is working.
2. **Quiet period**: how long without traffic before we ask.
3. **Question** (optional): a request topic and payload.
4. **Answer**: a topic, what counts as healthy, and within how long.
5. **Verdict**: the topic (and online/offline payloads) that the entities' availability already
   reads.

A gateway with a Last Will writes the verdict itself when it stops; we add the question for when
it hangs, and write "offline" to that same topic. When the gateway recovers it overwrites it with
"online" itself, which is what we want. ha2mqtt's ping-pong is this model with `ping`/`pong` and
`homeassistant_instances/ha2mqtt/status`. A host's staleness is the model without a question.

## In the defs

On the gateway's device in Physical.def (example values; to verify live, see below):

```
device node.zigbee2mqtt with:
  liveness with:
    traffic "zigbee2mqtt/#" quiet for 00:05:00;
    ignore "zigbee2mqtt/#/set", "zigbee2mqtt/#/get", "zigbee2mqtt/#/retain", "zigbee2mqtt/bridge/request/#";
    ask "zigbee2mqtt/bridge/request/health_check" with "";
    answer on "zigbee2mqtt/bridge/response/health_check" has data.healthy within 00:00:30;
    verdict on "zigbee2mqtt/bridge/state" online "{\"state\":\"online\"}" offline "{\"state\":\"offline\"}";
  end;
end;

device node.zwave2mqtt with:
  liveness with:
    traffic "zwave/#" quiet for 00:05:00;
    ignore "zwave/#/set";
    ask "zwave/_CLIENTS/ZWAVE_GATEWAY-zwave-js-ui/api/getInfo/set" with "{}";
    answer on "zwave/_CLIENTS/ZWAVE_GATEWAY-zwave-js-ui/api/getInfo" has success within 00:00:30;
    verdict on "zwave/_CLIENTS/ZWAVE_GATEWAY-zwave-js-ui/status" online "{\"value\":true}" offline "{\"value\":false}";
  end;
end;
```

`answer ... is "<payload>"` for an exact answer (ha2mqtt's `pong`), `answer ... has <field>` for a
JSON field that must be true (no Jinja: the coordinator evaluates it). `ignore`: what others send
under the traffic topics -- commands from main and the coordinator's own shadows (the broker
forwards those without the retain flag), which would otherwise count as life while the gateway
hangs. In `ignore`, `#` may also stand in the middle, for any number of levels: device names have
slashes, so `zigbee2mqtt/#/set` is needed for every command.

The generator turns this into a list in the coordinator's config; the coordinator has no
gateway-specific code. ha2mqtt's hard-coded ping-pong becomes one entry of the same list (its
"pong" answer automation stays generated).

## Who asks: the coordinator (decided by Erik, 2026-11-01)

The 2026-10-30 sketch had the coordinator only detect silence ("suspicion_of_hang") and rules on
main do the asking. Instead the coordinator does both, by generalizing `instance_liveness.go`:

- It already works this way for ha2mqtt, and it keeps working while main is down or reloading.
- Nothing is sent while a gateway is busy: a question goes out only after a quiet period, so this
  stays within "the coordinator does no polling" as already accepted for ha2mqtt.
- The rule route would need new DSL for MQTT requests and answers with timeouts, only for this.

## Details (agreed with Erik, 2026-11-01)

- **False offline**: an answer lost once must not make 300 entities unavailable. So: on no
  answer, ask once more after 30 s; only a second miss writes "offline".
- **Retained verdict topics**: we write "offline" retained, like the Last Will. If the gateway stays
  hung, it stays offline until it recovers; the coordinator keeps asking every quiet period and
  writes "online" itself on a healthy answer (for a gateway that recovers without republishing).
- **Quiet nights**: Zigbee2MQTT may be silent for longer at night; then it gets one question per
  quiet period -- a few messages an hour.
- **Coordinator down**: no hang detection then; its own Last Will already shows that.
- **Verified live** (Junglinster, 2026-11-01): Zigbee2MQTT answers `{"data":{"healthy":true},
  "status":"ok"}` on `zigbee2mqtt/bridge/response/health_check`; Z-Wave JS UI answers
  `{"success":true,"message":"Success zwave api call","result":{...,"status":"scan done",...}}` on
  `zwave/_CLIENTS/ZWAVE_GATEWAY-zwave-js-ui/api/getInfo` (its MQTT API is enabled). Both within a
  second.

## Steps

1. Done 2026-11-01: `liveness.go` (generic, `TLivenessSpec`), `instance_liveness.go` now one spec
   of it. Found on the way: ha2mqtt's ping-pong counted the coordinator's own existence inquiry
   (every minute) as a sign of life, so ha2mqtt never got quiet and a hang would have gone
   unnoticed; its spec now ignores that, its existence report and main's commands. Not deployed.
2. Done 2026-11-01: `homeassistant/physical_liveness.go` parses the block (inside a discovery
   gateway device) and writes `coordinator/liveness.yaml`; the coordinator loads it at start
   (`startGatewayLiveness`). Both houses generate an empty list until a block is added.
3. Done 2026-11-01/02: Zigbee2MQTT (both houses) and Z-Wave JS UI (Junglinster) declared and
   deployed; all checks online at start.
4. Tested 2026-11-01 in Vienna: `podman pause` of Zigbee2MQTT stops its keep-alives too, so the
   broker sent the Last Will after 90 s, before our check; on `unpause` the coordinator wrote online
   within a second. The connected-hang path itself stays untested live (unit test only); Erik left
   that to a real hang (Architecture.md §6.16).
