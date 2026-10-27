# Checkpoint — Home Assistant DSL Generator + Coordinator

**Frozen:** 2026-10-27, evening (pause of a few days: session and weekly limits).
**Status:** all three Go packages (generator, coordinator, shared `hamqtt`) build/vet/test clean.
Both houses deployed 2026-10-27 with item 19 complete (discovery to the main HA ends; every
entity from the generator's YAML `mqtt:` config; its lasting content is in Architecture.md
§6.16). Durable detail lives in `PROJECT.md` (open items), `README.md`/`Architecture.md` (how
things work) and the Claude memory directory (synced via Nextcloud). This file is only the
pointer to where to pick up.

**Git:** a large uncommitted changeset spans both modules (`git status`). Don't discard any of it.

## Next

Pick up with:

1. Item 20 (retained state via "<topic>/retain" shadow topics): the design is agreed and written
   down there; one choice is open for Erik (wildcard on topics or on identifiers), then steps
   (a)-(e).

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
