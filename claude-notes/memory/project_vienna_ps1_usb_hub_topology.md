---
name: project-vienna-ps1-usb-hub-topology
description: "Vienna's p-s-1 (Pi4) now has an active USB hub between the Zigbee2MQTT antenna and the Pi4; the SSD is plugged directly into the Pi4, not through the hub. Junglinster's p-s-1 will get the same setup, but with BOTH the Z-Wave and Zigbee antennas sharing the hub."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-21T17:19:26.985Z
---

Stated by the user 2026-09-21, mid PROJECT.md item 1a ("powerboard + USB hub in place"):
- Vienna's p-s-1: Zigbee2MQTT antenna -> active USB hub -> Pi4. SSD -> directly into the Pi4 (not
  through the hub).
- Junglinster's p-s-1 (planned, not yet done): same active-hub setup, but with BOTH the Z-Wave
  antenna AND the Zigbee antenna sharing the one hub (Vienna only has Zigbee on p-s-1; Junglinster's
  own migration plan, PROJECT.md items 5/6, adds Z-Wave there too).

**Why:** relevant context for troubleshooting radio dropouts/enumeration issues later, and directly
touches PROJECT.md item 6's own checklist step ("Confirm the new adapter's `/dev/serial/by-id/...`
path on p-s-1") -- a hub in the path can affect device enumeration order/stability differently than
a direct connection, and the SSD being direct-attached (not through the hub) matters for its own
I/O performance/reliability.

**How to apply:** when Junglinster's own Zigbee2MQTT/Z-Wave antenna migration (item 6) is actually
being done, expect the hub to carry TWO radios at once there (unlike Vienna's one) -- worth keeping
an eye on whether a single active hub has enough power/bandwidth headroom for both simultaneously.
