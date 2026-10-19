---
name: project-ems-esp-mqtt-direct-architecture
description: "EMS-ESP (Junglinster's heater/boiler integration, not yet started) will flow purely via MQTT to p-s-1's broker, bypassing protocols-server-2 entirely."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-21T12:01:52.776Z
---

EMS-ESP → MQTT broker @ protocols-server-1 → HA main. Confirmed by the user 2026-09-21, correcting
an assumption I made while discussing Junglinster's protocols-server-2 memory headroom (its own
local HA instance is comparatively tight, similar percentage-wise to what tipped Vienna's own p-s-2
into a real crisis, addressed that day via [[project_vienna_frame_protocols_server_2_identity]]'s
migration off protocols-server-2).

**Why:** EMS-ESP is a standalone MQTT-speaking device/gateway, not something that needs to run
inside or be proxied through p-s-2's own local HA container the way Envoy/printer/Bermuda currently
do there.

**How to apply:** when EMS-ESP eventually gets built (PROJECT.md: "design/syntax not yet worked
out"), don't assume it adds load to Junglinster's protocols-server-2 the way its other integrations
do -- it's architecturally more like a Zigbee2MQTT device (arrives via the coordinator/discovery
path) than like a native HA integration hosted on that box. When reassessing that host's future
capacity, EMS-ESP is not a load factor to weigh in.
