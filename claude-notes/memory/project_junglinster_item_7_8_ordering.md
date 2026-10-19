---
name: project-junglinster-item-7-8-ordering
description: Junglinster PROJECT.md items 7 (Zigbee antenna/hardware swap) and 8 (Zigbee2MQTT passthrough migration) may run out of numeric order.
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-14T19:45:05.271Z
---

Item 8 (Zigbee2MQTT legacy-to-conceptual passthrough migration of devices, the Junglinster
counterpart to Vienna's [[project_derived_capability_mechanism|item 4]] work) may actually
happen BEFORE item 7 (physical Sonoff -> Nabu Casa ZBT-2 antenna swap on Junglinster's p-s-1).

**Why:** item 7 needs the user physically on-site at Junglinster for several days to handle any
hardware-swap glitches (re-pairing devices, IEEE address transfer issues, etc.) -- item 8 doesn't
have that constraint and can be done remotely, same as Vienna's own item 4 work was done today
(2026-09-14) while the user was away from Junglinster.

**How to apply:** don't assume PROJECT.md's numeric ordering reflects actual execution order for
these two items. When picking up Junglinster Zigbee work, check with the user which one is
actually next rather than defaulting to numeric order. See [[project_zigbee_mqtt_relocation_2026_09]]
for the original antenna-swap plan this item 7 note extends.
