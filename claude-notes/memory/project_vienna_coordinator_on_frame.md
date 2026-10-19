---
name: project-vienna-coordinator-on-frame
description: "PROJECT.md 1.2c+1.2d DONE and FULLY VERIFIED LIVE (2026-08-30) -- Vienna's coordinator on frame imports all 5 Netatmo devices via real Spaces.def positioning (device <spec> from <device-id>; + for <device-id>: ...), no coordinator-invented naming, windy/battery_alert macros intact. Spaces.def itself already migrated off the old REST-import lines. Next: drop the now-dead REST-import Go code."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-08-30T15:07:25.817Z
---

## Status: coordinator running live on frame, zero devices declared yet

Confirmed live 2026-08-30: `systemctl status coordinator` on frame shows active/running, connected
to Vienna's main MQTT broker, "0 devices total" / "published 0 discovery config(s)" (correct --
Vienna's Physical.def has no `integration hosts`/`home_assistant`/`discovery` device declarations
at all yet, just the bare `home_assistant main: vienna ...;` + the new `mqtt main: ...;` block).
**Next real step for 1.2c, not started**: actually declare Vienna's own devices in Physical.def/
Spaces.def (mirroring the Netatmo-migration pattern already done for Junglinster this session, see
[[project_entity_existence_inquiry_design]]) so the coordinator has something real to discover/
report -- standing the service up was infrastructure, not the actual test.

## "frame" -- the temporary host

- Physical: a Pi3 running RaspberryOS (Debian 12/"Raspbian"), armv7l (32-bit ARM), ~900MB RAM,
  already swapping under light load -- explicitly temporary, per the user, until a planned Vienna
  hardware migration "next month" (unspecified further). Also already runs an unrelated
  `smtpproxy` Go service (`/home/pi/go/src/smtpproxy`) the user built previously -- confirms the
  Go-binary-as-systemd-service pattern was already proven on this host before this session.
- Reachability: locally within Vienna's LAN as `frame`/`frame.fritz.box`. From outside (this
  session's own access path): `ssh pi@vienna.homelinux.org -p 1968` (a port-forward through
  Vienna's router, same shape as Junglinster's own `ssh -p 1970 erikp@junglinster.homelinux.org`
  convention) -- also saved as the user's own shell alias `$HOME/bin/frame.vienna` locally.
  Passwordless sudo confirmed for the `pi` user.
- **Go 1.27.0 installed by hand** at `/usr/local/go` (symlinked into `/usr/local/bin`), since the
  Debian apt package only offers Go 1.19.8 ("oldstable"), too old for this repo's `go.mod` (`go
  1.24.0`). Downloaded+verified via go.dev's own JSON API + published sha256 before installing
  with sudo. This does NOT touch/conflict with the pre-existing `golang-go` apt package (still
  installed, just shadowed by `/usr/local/bin` being earlier in PATH) -- confirmed both interactive
  and non-interactive SSH resolve `go` to 1.27.0 correctly (the latter matters for the deploy
  script's own non-interactive `ssh ... go build` step).
- Build approach: **on-target build**, matching Junglinster's own `deploy.junglinster` convention
  exactly (not cross-compiled) -- the user's explicit choice, confirmed via
  [[feedback_ask_one_at_a_time]]-style single-question check, even though frame's limited RAM made
  cross-compiling-and-shipping-a-binary tempting; on-target build took ~3 minutes real time (`time
  go build`: 3m2s real, 7m46s user -- multi-core Pi3), acceptable for a deploy cadence.
- **MQTT broker: deliberately NOT frame's own local Mosquitto.** frame does run a Mosquitto
  instance (port 1883, one existing user `modelling_bus`, presumably for the smtpproxy or an
  unrelated purpose) -- the user explicitly redirected away from using it: "we should use the MQTT
  server on the main local HA instance" instead. Vienna's actual main broker is the Mosquitto addon
  on the main HA instance itself, `vienna.fritz.box:1883` (renamed as-declared bare `${installation}`
  hostname "vienna", which frame's local DNS resolves to `vienna.fritz.box`/192.168.178.22 --
  mirrors Junglinster's own `${main_mqtt_server} = "junglinster"` convention exactly). Login/
  password: **same as Junglinster's main broker** (`carvalhoproper`/`PaulaErik1967`) -- the user's
  own explicit, acknowledged-temporary choice ("I know ... will change this after things are
  stable"), not something to "fix" unprompted.
- **Future architecture note, the user's own framing**: after the planned hardware migration, the
  local MQTT brokers in both Vienna and Junglinster will move onto the same machines that run the
  coordinator (in containers), alongside Zigbee2MQTT (described as generating the bulk of the
  traffic) -- i.e. broker + coordinator + Z2M co-located per house, superseding frame's
  temporary/separate role and the "main HA instance hosts the broker" arrangement described above.
  Not built, just the stated direction -- relevant context for any future coordinator/broker
  topology decision, not an immediate action item.

## Two real gaps found and fixed getting Vienna's `./configure` + coordinator working

**Gap 1: `${main_mqtt_server}`/etc. in Settings.def alone is not enough -- Physical.def also needs
a `mqtt main: ... end;` block.** Adding the four `${main_mqtt_*}` variables to
`Vienna/Definitions/Settings.def` did NOT make `./configure` pick them up (still printed "no MQTT
broker settings found") -- `resolveMQTTBrokerProfiles`/`resolveMQTTBrokerSecrets`
(`homeassistant/defined.go`) only ever read a broker profile from an actual
`mqtt <profile-name>: server ...; login ...; password ...; port ...; end;` block inside
`physical layer with: ... end;` in Physical.def, where the `${...}` tokens are merely
variable-substituted -- Settings.def alone never wires anything up on its own. Vienna's
Physical.def had NO `mqtt main: ...;` block at all (just the bare `home_assistant main: ...;`
line) -- added one, mirroring Junglinster's exactly. Fixed, confirmed via `coordinator/secrets.yaml`
being generated correctly afterward.

**Gap 2, coordinator-side, real bug**: `loadDevicesFile` (`house_event_bus_coordinator/main.go`)
was the one config-file loader that did NOT tolerate a missing file -- unlike
`loadHassBridgeFile`/`loadDiscoveryFile`, which both already return a zero value (not an error) for
a missing file, since "this house has none of this integration kind yet" is a perfectly normal
state. Since the generator only ever writes `coordinator/devices.yaml` when at least one "hosts"
device is declared (`generateHostsIntegrationOutputs` bails early at zero devices,
`integration_hosts_generator.go`), and Vienna's Physical.def has zero "hosts" devices declared so
far, the file never existed -- and the coordinator hard-failed at startup with `error: cannot read
.../devices.yaml: ... no such file or directory`, crash-looping under systemd's `Restart=on-failure`.
Fixed: `loadDevicesFile` now mirrors the other two loaders' `os.IsNotExist` -> zero-value pattern.
Regression test added (`main_test.go`, new file for this package --
`TestLoadDevicesFileMissingReturnsZeroValue`). Confirmed live: redeployed, coordinator came up
clean, "0 devices total" / "published 0 discovery config(s)", no crash loop.

## Deploy tooling created (Vienna-specific, mirrors Junglinster's shape at a smaller scale)

Vienna's deploy tooling predates this session's Junglinster `deploy.d/`-directory restructure --
still the older single-flat-script style (`configure`, `update`, `deploy.coordinator`) -- this work
did NOT restructure it to match, just fixed/extended what already existed:
- `Vienna/Definitions/Settings.def` -- added `${installation}`/`${main_mqtt_*}` (see Gap 1 above).
- `Vienna/Definitions/Physical.def` -- added the `mqtt main: ...;` block (see Gap 1 above).
- `Vienna/coordinator/coordinator.service` -- new, mirrors Junglinster's `Integrations/coordinator/
  coordinator.service` (`User=pi`, `WorkingDirectory=/home/pi/coordinator`,
  `ExecStart=/usr/local/bin/coordinator /home/pi/coordinator`).
- `Vienna/coordinator/deploy.frame` -- new, mirrors Junglinster's `deploy.junglinster` (on-target
  build + `sudo install` to `/usr/local/bin` + systemd `daemon-reload`/`enable`/`restart`) --
  simplified slightly since frame (plain Debian/RaspberryOS) has no SELinux-context concern the
  way Junglinster's Fedora host does, but installed to `/usr/local/bin` anyway for consistency.
- `Vienna/deploy.coordinator` -- rewritten (previously a placeholder pointing at
  `coordinator.vienna.fritz.box`, a host that was never actually provisioned) to rsync the go
  source + `coordinator/` dir (with the same `entity_existence.json`/`discovery_existence.json`/
  `discovery_topics.json` excludes as Junglinster's own, [[project_entity_existence_inquiry_design]]'s
  §6.11 bug) to frame via the external port-forwarded route, then invoke `deploy.frame` remotely.
  Confirmed working end-to-end, twice (once pre-fix crash-looping, once post-fix clean).

Not yet done: `Vienna/update` (the separate script pushing HA's own YAML config, not the
coordinator) hasn't been touched or exercised this session -- Vienna's main HA instance itself
still has whatever config it had before; only the coordinator side was stood up.

## PROJECT.md 1.2d built same day: the "import" side, and Vienna's real Netatmo devices

Immediately after 1.2c's infra above, the user clarified the actual goal wasn't "reconnect
Vienna's own Netatmo integration" (it's genuinely disconnected/unconfigured on Vienna's own HA,
confirmed via a live REST API check -- no per-room weather-station entities exist there, only
`device_tracker.netatmo`/`switch.netatmo_internet_access`) but rather: replace Vienna's old
`imported rest junglinster <entity> <poll_seconds>;` polling shim (Spaces.def) with the new
MQTT export/import machinery -- "we will, once this is done, drop all 'rest' related code"
(not done yet -- explicitly deferred until import is verified working).

**Built**: PROJECT.md 1.2d, a new "hassbridge" import kind inside Physical.def's existing
`integration import with: ... end;` block:
```
device <local-id> from <remote-installation> <remote-device-id> hassbridge with:
  <capability>: <remote-local-entity>;
  ...
end;
```
Declares capabilities explicitly (RemoteLocalEntity = the exporting installation's own
`local_entity`, matched against each incoming exported discovery payload's `default_entity_id`)
rather than auto-discovering everything a remote device happens to export -- deliberately mirrors
kind-2's own "declare the leaf, don't auto-adopt" discipline, specifically so the resulting local
discovery topics are fully computable from Physical.def alone (avoiding a repeat of the SAME
orphan-topic-cleanup bug class the export side hit live earlier the same day -- see
[[project_native_export_cloud_crosspost]]).

New files: `homeassistant/integration_import_hassbridge_storage.go` (TImportedHassBridgeDevice/
Capability), `integration_import_parser.go` REWRITTEN (was a single-pass hosts-only parser; now
one unified pass recognising both the old bare hosts-import line AND the new hassbridge-form
block in the SAME body scan -- an earlier two-pass draft had each parser warn about the other's
lines, a real bug caught before deploy), `integration_import_hassbridge_generator.go` (writes
`coordinator/imported_hassbridge.yaml`), `house_event_bus_coordinator/discoveryimport.go`
(coordinator-side consumer -- deliberately reuses `discoverybridge.go`'s own shape: subscribe to
a `<remote-installation>/<prefix>/+/+/+/config` filter on the CLOUD broker, decode, match, publish
a local discovery config on "main" whose `state_topic` points at a new local relay topic
`imports/<local-id>/<capability>/state`, and -- once per remote `state_topic` -- subscribe to
relay the raw value from cloud to that local topic). `expectedImportedHassBridgeTopics` merged
into `main.go`'s ordinary "main"-broker expected-topics set (not cloud -- these are locally-owned
relay entities). Both modules build/vet/gofmt/test clean; new tests in
`discoveryimport_test.go`/`integration_import_parser_test.go`/
`integration_import_hassbridge_generator_test.go`/`Physical_Generator_test.go`.

**Real bug #3 of the day, found live deploying this**: even with the import subscription code
correct, nothing happened on Vienna's coordinator -- `subscribeImportedHassBridgeDevices`'s own
`cloudClient == nil` no-op guard was firing, because Vienna had no cloud broker profile at all
yet (deliberately deferred as "follow-on work" when 1.2c was first stood up). Fixed: added
`${cloud_mqtt_server}`/`${cloud_coordinator_mqtt_login}`/`${cloud_coordinator_mqtt_password}` to
Vienna's Settings.def (same shared broker + "coordinator" login Junglinster's own coordinator
profile already uses) and a `mqtt cloud_coordinator: ... coordinator_only true; end;` block to
Vienna's Physical.def, mirroring Junglinster's exactly.

**Real bug #4, found immediately after fixing #3**: coordinator then logged "a cloud broker is
configured but devices.yaml has no installation set" and refused to publish/clean up cloud-side
topics. Root cause: `coordinator/devices.yaml` (the ONLY file carrying `Installation`/
`MQTTDiscoveryConceptualPrefix` to the coordinator) was ENTIRELY gated behind
`generateHostsIntegrationOutputs`, which never runs at all when Physical.def has no
`integration hosts with: ... end;` block -- Vienna's exact situation (zero local hosts devices
declared, by the user's own explicit choice -- "will add other things like host/cpu myself
later"). Fixed in `Physical_Generator.go`: `generateCoordinatorDevicesFile` now also gets called
unconditionally, right after `ImportedDevices` is collected, as a baseline write -- a house WITH
an `integration hosts` block still gets that block's own later, fuller (hosts+imports) write
overwriting this baseline in the same run, so existing houses are unaffected; a house WITHOUT one
(Vienna today) now gets a minimal-but-correct devices.yaml (empty `devices:`, but
`installation`/`mqtt_discovery_conceptual_prefix` both present) instead of no file at all.
Regression test: `TestGeneratePhysicalIntegrationOutputsWritesDevicesFileWithNoHostsBlock`.
Confirmed live: regenerated, `coordinator/devices.yaml` now reads
`installation: "vienna"` correctly.

## PROJECT.md 1.2d FULLY VERIFIED LIVE, 2026-08-30 (after a "freeze"/resume mid-session)

Redeployed with the devices.yaml baseline-write fix, then hit a 6th real bug that day: still
nothing showed up locally, even though a temporary debug print confirmed the coordinator WAS
receiving Junglinster's exported discovery payloads correctly over the cloud subscription. Root
cause: the raw-value relay's own `cloudClient.Subscribe(payload.StateTopic, ...)` call was made
**synchronously, from within the discovery-config message handler itself** -- a classic, well-known
paho gotcha: that handler runs on `cloudClient`'s own receive goroutine, which is also the goroutine
responsible for reading the broker's SUBACK response the blocking `Subscribe(...).WaitTimeout(...)`
call was waiting on -- self-deadlock (or, since `WaitTimeout` gives up after 10s, a silent timeout
that never showed up in logs at all: `token.Error()` is nil on a timeout, and the original code only
printed on `err != nil`, so the failure was completely invisible). Confirmed by first adding a blunt
debug print at the top of the discovery-config handler -- proved messages WERE arriving and matching
correctly, isolating the bug to specifically the nested Subscribe call. Fixed: wrapped that Subscribe
call in `go func() { ... }()`, and fixed the swallowed-timeout logging bug alongside it (now logs
"timed out" explicitly rather than silently doing nothing when `Error()` is nil).

Test-side consequence: `discoveryimport_test.go`'s end-to-end test now has a genuine
producer/consumer race between the test goroutine and the newly-spawned relay-subscribe goroutine.
Fixed properly rather than papered over: added a `sync.Mutex` to the shared `fakeClient` test
double (`discoverycleanup_test.go`, used by many test files) guarding `published`/
`subscribedHandlers`, with `publishedSnapshot()`/`subscribedHandlersSnapshot()` thread-safe
accessors -- existing tests keep reading the fields directly (no concurrent access of their own,
so no risk), only the new async test uses the snapshot methods plus a short poll-with-timeout for
the relay subscription to register before simulating a value arriving. Full suite, including
`go test -race ./...`, passes clean on both modules.

**Confirmed live, real values, all 5 devices, every declared capability** (`mosquitto_sub -h
vienna.fritz.box -p 1883 -t 'imports/#' -v`, run via a jump through frame since Vienna's own LAN
isn't reachable from outside it -- also installed `mosquitto-clients` on frame itself for this,
previously absent):
```
imports/vienna_bedroom/{node,co2,battery_level,humidity,temperature,radio}/state
imports/vienna_livingroom/{node,co2,humidity,noise,pressure,radio,temperature}/state
imports/vienna_shower_room/{node,battery_level,humidity,co2,temperature,radio}/state
imports/vienna_terrace/{node,humidity,battery_level,temperature,radio}/state
imports/vienna_terrace_wind/{node,battery_level,radio,wind_direction,wind_speed}/state
```
Real Netatmo readings, matching what's currently on the cloud broker. Discovery configs for the
same set already confirmed separately (`homeassistant/+/coordinator/imported_+/config`, correct
`state_topic`/`device_class`/`unit_of_measurement`/device grouping throughout).

**PROJECT.md 1.2c and 1.2d are both done and live-verified.** Next, explicitly gated on this by the
user's own words ("once this is done, we will drop all 'rest' related code"): remove the old
`imported rest junglinster <entity> <poll_seconds>;` lines from Vienna's Spaces.def (the 5
Netatmo-room blocks) and any now-dead REST-import parser/generator code in the Go codebase (not
yet located/scoped -- search for "imported rest"/"ImportedRest" across `administration.go`/
`generator.go`/`parser.go` first, per this file's own earlier research notes, before removing
anything). Not started.

Minor known cosmetic issue, not fixed (didn't block correctness): the imported local `unique_id`
doubles up "imported_" (e.g. `imported_sensor_imported_vienna_bedroom_battery_level`) since
`importedUniqueID` prefixes an already-`imported_`-containing local entity id. Harmless (still
unique), but worth a quick cleanup pass if touching this code again.

## Redesign, same day: imports get real Spaces.def positioning, not coordinator-invented naming

Immediately after the above, the user pushed back hard on the "imported_<device>_<capability>"
naming scheme the first working version used: real Vienna Spaces.def entities like
`sensor.social:wind_speed` (referenced by `call windy sensor.social:wind_speed ...;`) and the
`battery_alert :netatmo`-style macro calls depend on the OLD Netatmo entities living at very
specific, already-established conceptual paths -- a coordinator-invented generic name would have
silently orphaned those references (and dropped device positioning/area continuity entirely). The
user's own explicit framing, unprompted: "we do NOT need to extend the DSL syntax (for spaces!) to
deal with import. It (the definitions used in spaces) should only refer to the device name. That
does not depend on a specific integration (such as import)" -- i.e. Spaces.def must stay
completely integration-agnostic; the fix belongs in the *lookup/dispatch* layer, not new grammar.

**Redesigned and rebuilt same day**: the existing `device <spec> from <device-id>;` +
`for <device-id>: entity <spec> from entity <capability>;` constructs (already used for native
hassbridge devices) now *also* resolve against imported devices -- zero new Spaces.def syntax.
Generator-side: `registerImportedDevicePositioning`/`registerImportedDeviceSourceEntityLink` (new,
`Conceptual_DevicePositioning.go`/`Conceptual_DeviceSourceEntities.go`) mirror the native path's
shape closely, but resolve typing from nothing (an import has no local typing metadata to seed --
the coordinator supplies it live from the remote's own payload) and validate against the imported
device's own declared `Capabilities` map instead of mutating a `THassBridgeCapability`. The bare
"entity ... as <source> from ...;" U1 form is explicitly rejected for imports (no meaningful local
"source" to give) -- only the "for <device-id>: ..." shorthand referencing an already-declared
Physical.def capability makes sense. `collectImportedHassBridgeDevicesByID` (new,
`integration_import_parser.go`) collects imports the same independent-reparse way
`collectHassBridgeDevicesByID`/`collectHostsDevicesByID` already do, threaded through
`ParseEntitiesAndFillAdministration`/`parser.go` as one more `map[string]TImportedHassBridgeDevice`
parameter alongside the existing device-kind maps.

`generateImportedHassBridgeFile` rewritten to gate on `admin.DeviceConceptualLinks`/
`AttributeEntityIDs` exactly like `generateHassBridgeFile` (export) already does -- only a
capability Spaces.def actually positioned gets written, now carrying BOTH `remote_local_entity`
(Physical.def's own declared reference) and `local_entity` (Spaces.def's resolved naming).
Coordinator-side (`discoveryimport.go`) simplified accordingly: `TImportedHassBridgeCapability`
gained a `LocalEntity` field read straight from the YAML; the coordinator's own
`importedLocalEntityID`/generic-naming machinery was deleted outright, `buildImportedDiscoveryBody`/
`matchImportedCapability`/`expectedImportedHassBridgeTopics` all key off `cap.LocalEntity` now,
never inventing anything.

**Verified live, full redeploy cycle**: real bug hit AGAIN during this -- ran `./configure`
against Vienna's real Physical.def/Spaces.def and got "device not found" warnings for every single
imported device, despite a standalone unit test proving `collectImportedHassBridgeDevicesByID`
found all 5 correctly. Root cause: stale installed `homeassistant` binary -- `go install .` hadn't
been re-run since the positioning changes landed (yet another instance of
[[feedback_terse_chat_and_rebuild_before_test]]'s lesson; a passing isolated test was the giveaway
that logic was fine and environment was stale). Reinstalled, `./configure` came back completely
clean, and `coordinator/imported_hassbridge.yaml` showed exactly the expected
Spaces.def-resolved names (e.g. `sensor.physical_apartment_bedroom_netatmo_co2`,
`sensor.social_terrace_wind_speed`). Redeployed to frame -- the coordinator's own orphan-topic
watcher correctly retired the OLD generic-named discovery configs on restart (the mechanism
working exactly as designed), and `mosquitto_sub` against Vienna's local broker confirmed the new,
correctly-named discovery configs + real relayed values for all 5 devices' every capability.
Manually cleared the handful of stale RAW-value retained topics under the old naming (not
auto-cleaned -- only discovery configs go through the orphan-topic mechanism, raw relay topics
don't have an equivalent lifecycle tracker) via `mosquitto_pub -r -n`. Confirmed final state clean:
only correctly-named topics remain.

Both Go modules build/vet/gofmt/test clean throughout, including `go test -race ./...` on both
(the async relay-subscribe goroutine surfaced one more real data race in the test itself, from an
unprotected direct `fakeClient.published` field read racing the background goroutine's own
publish -- fixed by routing that read through the same `publishedSnapshot()` accessor used
elsewhere, not by touching production code).
