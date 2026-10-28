# Checkpoint — Home Assistant DSL Generator + Coordinator

**Frozen:** 2026-10-28, midday.
**Status:** all three Go packages build/vet/test clean. Both houses deployed 2026-10-28 with item
20 complete (retained shadows "<topic>/retain" for state topics that can't be trusted as retained;
Architecture.md §6.16), on top of item 19 (every entity from the generator's YAML). Durable detail
lives in `PROJECT.md` (open items), `README.md`/`Architecture.md` (how things work) and the Claude
memory directory (synced via Nextcloud). This file is only the pointer to where to pick up.

**Git:** uncommitted changes since Erik's commit of 2026-10-27 (item 20: generator, coordinator,
the quote-aware comment rule). New files to add: homeassistant/retain_shadows*.go,
house_event_bus_coordinator/retain_shadows*.go.

## Next

Pick up with:

1. The next open item in PROJECT.md (21 onward), as Erik chooses.

Notes from 2026-10-28:
- Deploy step 06 is now `06_check.ha_reload` in both houses: it waits for HA to finish its reload
  and reports reload errors; it no longer restarts Zigbee2MQTT (the shadows keep the values).
- Fixed in Zigbee2MQTT's UI: negative "debounce" values (Vienna's washing-machine plug, two
  Junglinster devices) made it drop reports ("`wait` must not be negative").

Notes from 2026-10-27:
- Deploy order and checks: Vienna first; check the defs against the last verified generate right
  before deploying; check only after HA's MQTT reload has finished (~45 s); step 06 reports HA's
  reload errors.
- Vienna's deploy step 00 now rsyncs the source with `--delete` (as Junglinster's already did):
  without it, a file deleted locally stayed on the server and broke the coordinator build.
- After a deploy, Zigbee2MQTT's restart leaves some battery devices offline until they check in
  (Vienna's Signify dimmer, Junglinster's kitchen Go); not caused by the YAML.

Xanadu is powered down until needed (Plex videos, backups); power cycle procedure in memory.

## Commands

```bash
cd homeassistant && go vet . && go test . && go install .     # generator (install before generating)
cd house_event_bus_coordinator && go vet . && go test .
cd ~/SmartLiving/<House> && bash generate                      # Nextcloud scripts: always via bash
cd ~/SmartLiving/<House> && bash deploy                        # deploy.d/01..05, incl. registry sync
```
