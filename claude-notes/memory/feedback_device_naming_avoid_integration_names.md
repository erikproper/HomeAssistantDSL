---
name: feedback-device-naming-avoid-integration-names
description: "Physical.def device ids should be named after the device's functional category (light, buttons, sensors, switch, cover, appliance, ...), never after the integration/mechanism that provides it (discovery, hassbridge, zigbee2mqtt, ...). Apply when migrating Junglinster's own devices."
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-20T16:11:45.633Z
---

Erik's own instruction (2026-09-20), given right after renaming ~73 of Vienna's device ids away
from the generic `discovery.<name>` prefix to functional ones (`light.<name>`, `buttons.<name>`,
`sensors.<name>`, `switch.<name>`, `cover.<name>`/`covers.<name>`, `appliance.<name>`): device ids
should describe *what the device is/does*, not *which integration mechanism backs it*. `discovery`
names the HA-MQTT-discovery mechanism, not the device -- every device in Physical.def happens to be
discovery-kind under the hood regardless of its id prefix, so using "discovery" as the id prefix
told the reader nothing useful and gave zero organizational value across a large file.

**Why:** purely a readability/organization improvement for the human author scanning Physical.def --
device ids are free-form strings with no functional effect on parsing or on the entity ids
generated downstream (confirmed live 2026-09-20: renaming all 73 Vienna devices produced a
byte-identical regenerate, since final entity_id depends on the Conceptual.def POSITIONING path,
never on the device id's own string). The mechanism (discovery/hassbridge/import/commandline/hosts)
is an internal implementation detail; the id should read like a category label instead.

**How to apply**: when migrating Junglinster's own devices (or writing NEW Physical.def
declarations on either house going forward), choose a device id kind prefix based on what the
device fundamentally IS -- `light`, `buttons` (any button/scene-switch/remote), `switch`,
`sensors` (any sensor-only device, including multi-sensor combos), `cover`/`covers`, `appliance`
(anything cross-posted or complex enough not to fit the above) -- never `discovery`, `mqtt`,
`zigbee2mqtt`, `z2m`, or similar mechanism/integration names. `node`/`utility`/`environment` remain
legitimate EXISTING prefixes for their own established meanings (hosts-kind liveness, EMS-ESP-style
utility gateways, Netatmo-style environment sensors respectively) -- don't rename those away
without a specific reason, this heuristic is about NOT introducing a mechanism name as an id
prefix, not about forcing every id into exactly these five buckets.
