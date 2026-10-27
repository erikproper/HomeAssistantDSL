# Checkpoint — Home Assistant DSL Generator + Coordinator

**Frozen:** 2026-10-24 (Erik in Junglinster).
**Status:** both Go modules build/vet/test clean. Both houses deployed 2026-10-24; Junglinster's
last deploys did the physical->social renames (item 15, done). Durable detail lives in
`PROJECT.md` (open items), `README.md`/`Architecture.md` (how things work) and the Claude memory
directory (synced via Nextcloud). This file is only the pointer to where to pick up.

**Git:** a large uncommitted changeset spans both modules (`git status`). Don't discard any of it.

## Next

Pick up with:

1. Item 19 (plan in PROJECT.md).

Done 2026-10-26, all deployed: no node in the physical/social sphere (inputs inlined or named
"<node>/ping|cpu|controller/node", generate warns otherwise), enabler nodes, switched lights' nodes
up while their relay is off, ping reporter sends 3 pings, Junglinster cpu distribution moved from
xanadu to protocols-server-1, deploy skips reload/restart after a failed copy, ssh ControlMaster
for both houses (local ~/.ssh/config, each Mac needs it), "enabled by" (17k, incl. the iMac as
server.imac), coordinator renews its MQTT subscriptions after a reconnect (it went deaf to the
cloud broker after a DNS outage), 17g complete (single-player spaces' media switches are the
player's own switch; note for an ambiguous node at a space's name; derived-node leaf fallback),
17h (relayed Zigbee2MQTT/EMS-ESP entities carry their conceptual name as discovery "name", so HA's
UI shows it; the other kinds moved to item 19), 17i (firmware updates follow the nodes, written
into the defs, shown without install; firmware_updates summary; list.updates for an update page),
17j (the deploy's registry step is a repair driven only by the generator's table, documented
with its HA dependency in Architecture.md; native entities in the table as "native", keyed by
source, renamed with their position, never removed -- recorded 2026-10-26, renames from the next
deploy on), 17l (device ids whose prefix misstated the device renamed: 31 in Junglinster, 2 in
Vienna; frozen unique_ids unchanged, 3 hidden switched-light entities renamed by the repair),
item 18 ("meta" controls are infrastructural now; unique_ids kept, renamed by the table; MQTT
topics unchanged; Junglinster's dashboard updated). Capability label "core" renamed to the
domain in both houses (generated output identical, so nothing to deploy).

Xanadu is powered down until needed (Plex videos, backups); power cycle procedure in memory.

## Commands

```bash
cd homeassistant && go vet . && go test . && go install .     # generator (install before generating)
cd house_event_bus_coordinator && go vet . && go test .
cd ~/SmartLiving/<House> && bash generate                      # Nextcloud scripts: always via bash
cd ~/SmartLiving/<House> && bash deploy                        # deploy.d/01..05, incl. registry sync
```
