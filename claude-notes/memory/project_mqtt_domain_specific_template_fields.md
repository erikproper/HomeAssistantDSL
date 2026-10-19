---
name: project-mqtt-domain-specific-template-fields
description: "HA's MQTT discovery schemas use different field names for conceptually-equivalent templates across domains (e.g. fan's state_value_template vs the generic value_template) — a real bug found migrating a switch-shaped payload to domain \"fan\"."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-14T21:01:19.941Z
---

Real bug, 2026-09-14, found live migrating Vienna's cycling fan (PROJECT.md item 4): once
`buildRelayedDiscoveryConfig` started cloning a gateway's entire raw discovery payload and only
overriding naming fields (fixing an earlier bug where brightness/colour-mode/effect fields got
dropped entirely), a NEW class of bug appeared — relaying a Zigbee2MQTT switch's own payload
verbatim under Spaces.def's `fan.social:cycling` declaration left the resulting fan entity stuck
on state "unknown" forever, even though live MQTT messages were arriving correctly and the
raw device itself worked fine (confirmed via Zigbee2MQTT's own web UI).

**Root cause**: HA's MQTT Fan integration reads `state_value_template` to extract a value from a
JSON state payload — not the generic `value_template` key that Z2M's own switch-shaped payload
carries (correct for switch/sensor/binary_sensor, which DO use `value_template`). The field name
isn't universal across domains; Fan just silently ignores the key it doesn't recognise, with no
validation error anywhere — the entity accepts the discovery config fine, just never has a
successfully-parsed state.

**Fix**: `buildRelayedDiscoveryConfig` (house_event_bus_coordinator/discoverybridge.go) now sets
both `value_template` and `state_value_template` to the same value whenever a value_template
exists, regardless of target domain — confirmed harmless for domains that only recognise the
generic key (HA's discovery schemas ignore extra keys, no validation warning appeared for any
already-working entity carrying both).

**How to apply**: this is a standing risk for ANY future migration that relays a device's payload
under a domain different from what it was natively shaped for (PROJECT.md item 4's "declare the
leaf as the wrapper's own domain directly" pattern, replacing switch_as_x helpers). Cover, climate,
and other richer domains likely have their own similarly-named-but-different template fields
(HA's own per-domain MQTT docs are the source of truth) — don't assume a payload relays cleanly
across domains just because the wire format looks compatible; check the TARGET domain's own schema
docs for its actual field names before trusting a cross-domain relay silently. See
[[project_retiremissing_passthrough_wipe_incident]] for the earlier, related bug (dropped
command_topic/brightness fields) this same relay mechanism had.
