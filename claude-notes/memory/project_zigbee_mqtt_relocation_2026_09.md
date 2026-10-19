---
name: project-zigbee-mqtt-relocation-2026-09
description: "Planned infra relocation (2026-09-08 evening, Vienna; following weekend, Junglinster): Zigbee antenna + MQTT broker move off each house's own main HA instance onto protocols-server-1 containers. Junglinster also swaps its Zigbee antenna, requiring an IEEE address transfer."
metadata:
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-08T13:19:46.391Z
---

**This evening, 2026-09-08, Vienna** (stated ~15:13 local Vienna time):
1. Move the Zigbee antenna from "Green" (Vienna's main HA instance, an HA Green appliance) to
   protocols-server-1, running Zigbee2MQTT in a container there.
2. Move the MQTT broker off Green (its own built-in broker) to a containerized one on
   protocols-server-1 too.

**Deliberately out of scope for this move**: the DSL/generator's own Zigbee2MQTT entity modeling
(PROJECT.md item 9, "kind-2 discovery" migration) is scheduled separately, later. This is pure
infrastructure relocation -- existing devices keep working exactly as they do now, just on
relocated hardware/containers, no Physical.def/Spaces.def changes expected from this step alone.

**This weekend, contingent on the Vienna move working, Junglinster**: same relocation pattern
(antenna + broker off Green-equivalent onto protocols-server-1 containers), PLUS a hardware swap:
replacing the current Sonoff Zigbee antenna with a new Nabu Casa one.

**Real constraint flagged by the user for the Junglinster antenna swap**: the Zigbee coordinator's
own IEEE address is the mesh network's trust-center identity -- swapping the physical
antenna/dongle without transferring the OLD IEEE address onto the NEW one means every paired
Zigbee device sees a different network identity and needs re-pairing from scratch. The antenna
swap therefore needs an explicit "transfer" step (writing the old IEEE address onto the new
adapter's firmware/EEPROM, where the hardware/tooling supports it) to avoid re-pairing the whole
mesh. Not yet researched which specific tool/flashing method applies to the Nabu Casa
adapter -- worth checking before the weekend work starts.

**How to apply**: don't start drafting Zigbee2MQTT/Mosquitto container configs (Quadlet units,
matching the existing Fedora/Podman convention on protocols-server-1) until the user says they're
starting -- this is live infrastructure work on a running home automation system, follow their
lead rather than pre-empting. When the Junglinster antenna swap comes up, raise the IEEE-address
transfer step explicitly if it hasn't already been handled.
