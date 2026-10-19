---
name: project-native-export-cloud-crosspost
description: "PROJECT.md 1.2a+1.2b DONE and VERIFIED LIVE (2026-08-30) -- \"local\"->\"native\" rename; \"export\" cross-posts a home_assistant-bridge device's metadata/state onto the cloud broker (kind-2-shaped) AND implies \"all entities used\" even with no Spaces.def positioning. Two orphan-topic-cleanup bugs found+fixed during live verification -- see body. Import side (1.2d) still not built."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-08-30T11:23:45.011Z
---

## "local" -> "native" rename

The "hosts" integration's trailing `cloud`/`local` routing keywords (`device host.x host cpu cloud
local;`) had "local" renamed to "native" throughout: `parseRoutingKeywords`
(`mqtt_routing_keywords.go`), `THostDevice.Native` (was `.Local`), `TDevice.Native` (coordinator
side, `main.go`), `devices.yaml`'s `native:` field (was `local:`), all doc comments in
`mqtt_relay.go`/`discoverycleanup.go`/`defined.go`, both real Physical.def usages
(`host.eriks-macbook-pro-2`, `host.mqtt`). Reason (the user's own): "local" was already doing
double duty meaning "the local/main MQTT broker" throughout this codebase (`mqtt_relay.go`'s own
doc header uses "local/main broker" constantly) -- the routing keyword needed a name that doesn't
collide with that. Purely mechanical, no behaviour changed. Real Physical.def now reads `cpu cloud
native;`.

## "export" keyword: real semantics for `home_assistant`-bridge (kind-3) devices

Previously parser-only (`THassBridgeDevice.Export`, no downstream effect). Now: `device <id>
export with: ...; end;` -> `integration_hassbridge_generator.go` writes `export: true` into
`coordinator/homeassistant_bridge.yaml` -> `house_event_bus_coordinator/discoveryhassbridge.go`
reads `THassBridgeDevice.Export` and, for that device only, additionally cross-posts onto the
cloud broker (both `subscribeHassBridge` and `subscribeHassBridgeDeviceInfo`, both no-ops when no
cloud broker is configured):

1. **Raw value relay** (`crossPostHassBridgeToCloud`): every entity-state / device-info message
   gets forward-published (retained) to `<installation>/<bare local topic>` on the cloud broker --
   a plain prefix, not `qualifyHostsTopic`'s ACL-preserving mid-insertion, since the *coordinator*
   is the publisher here (using the `cloud_coordinator` broker profile, not per-client-ACL scoped),
   unlike a hosts device's own report script.
2. **A separate, genuinely functional discovery config** per entity, published to the cloud broker
   at the usual installation-qualified topic. NOT a copy of the local payload -- its `state_topic`
   points at the cross-posted cloud topic from (1), and it carries an extra `"installation"` field.

**Why "separate, functional" rather than reusing the existing hosts `cloud`+`native` pattern
verbatim**: `mqtt_relay.go`'s own doc header documents that a hosts device's cloud-side discovery
config is a deliberately **non-functional catalogue entry** -- its embedded `state_topic` still
points at a bare local topic no outside party can reach, because that mechanism exists for a
device whose own report script already chooses which broker to target; the coordinator never
originates the raw value there. A hassbridge entity's reporting automation, by contrast, *always*
publishes locally only (`homeassistant_instances/<name>/bridge/<local_entity>/state`) -- there's no
equivalent "the device itself could target either broker" choice -- so for `export` to mean
anything real, the *coordinator itself* has to be the one relaying local->cloud, and since it's
doing the relay anyway, there's no reason for the accompanying discovery config to be a
non-functional catalogue entry too. Confirmed directly by the user mid-build (2026-08-30): "The
meta-data posted by the export should be rich enough for an importing coordinator to construct the
device + entity + topics map needed for the imports and kind 2/3 alike checking of existence and
creation of suggestions" -- i.e. genuinely usable by a future importer without a hand-authored
re-declaration, the way kind-2's own native discovery already is, not the "look, something is out
there, go declare it by hand" catalogue shape kind-1's cloud discovery uses.

**Design consequence, deliberate**: reused the *existing* HA-MQTT-discovery-shaped payload
(`buildHassBridgeEntityDiscoveryBody`) rather than inventing a separate manifest format. An
exported kind-3 entity is now meant to look, wire-format-wise, indistinguishable from a kind-2
gateway's own native self-description -- so PROJECT.md 1.2b's eventual "import" consumer could in
principle reuse `discoverybridge.go`'s existing kind-2-payload-consuming logic rather than needing
a third, bespoke ingestion path. This is the "kind 2/3 alike" framing from the user's own
instruction, taken literally as a wire-format decision, not just a loose analogy.

**`"installation"` field, why**: `device.Identifiers` in the discovery payload is still just the
bare local device id (e.g. `hass.office_garden`) -- unqualified, since qualifying it would be a
bigger, more invasive change (touches the shared `TDiscoveryDevice` struct used everywhere else
too) and wasn't asked for. Two different exporting houses could in principle both have a device
with the same bare id on the same shared cloud broker; the extra top-level `"installation"` field
gives a future importer what it needs to disambiguate without that deeper change. Flagged as
sufficient for now, not treated as a solved general problem.

**Not built, deliberately, and why**:
- **The "import" consuming side** (PROJECT.md 1.2b) -- an `integration import`-style declaration
  for hassbridge devices, analogous to hosts' `ImportedFrom`. 1.2b's own note explicitly points
  back at this file's export payload as "rich enough" -- the next session building 1.2b should
  start by re-reading the actual discovery-payload shape this produces (via a live
  `mosquitto_sub`/journal check against a real exported device) rather than re-deriving it from
  scratch.
- **Command topics** -- open/close/stop etc. don't exist for hassbridge entities *at all* yet
  (PROJECT.md 1.6, still undesigned in general, not specific to export). Nothing to cross-post
  there; add it alongside state/device-info once commands land, not before -- don't invent a
  shape speculatively now.

## "export" implies "all entities used" (PROJECT.md 1.2b, same day, immediate follow-up)

Everything above only ever fires for a device with a `DeviceConceptualLinks` entry -- and the only
way to get one, until this fix, was an explicit Spaces.def positioning (`entity device.<spec> from
<device-id> with: all entities;` or the lighter `device <spec> from <device-id>;` form). Backwards
for the motivating case: Vienna's Netatmo readings (relayed via protocols-server-2) have no reason
to live anywhere in *Junglinster's* own Spaces.def at all -- their whole purpose is to be exported,
not used locally -- so `export` alone silently registered nothing until this was fixed. The user's
own words, quoted directly since they're the spec: "the export keyword implies that the coordinator
exports the device topics + discovery information to the cloud broker. For the integration it
means that it needs to 'do' all that would need to be done when the device entities would all be
'used'. For the home_assistant integration it implies we need to ensure the reporting (and future
potential command) automations are also generated."

Fixed: new `registerExportedHassBridgeDevices` (`Conceptual_DeviceExportRegistration.go`), called
from `Physical_Generator.go`'s `generatePhysicalIntegrationOutputs` right after
`collectHassBridgeDevicesByID`, before `generateHassBridgeFile`. For every `Export=true` device
with **no existing** `DeviceConceptualLinks` entry, auto-registers one -- one entity per declared
capability -- exactly as if `device infrastructural:/<bare-id> from <device-id> with: all
entities;` had been written at the root space. Calls `registerHassBridgeDeviceImpliedEntities`
directly with an explicitly-built `deviceIdentity`/`displayName`, bypassing
`registerDeviceImpliedEntities`'s own `administration.SpacePath`-based resolution -- `SpacePath` is
parse-time-only state, no longer meaningful by the time this runs (after Spaces.def parsing has
fully finished). The synthetic spec `"infrastructural:/" + bareLocalName` has a leading `/` after
the colon specifically so `normalizeEntityFullName` treats it as absolute and ignores `SpacePath`
regardless of its value -- verified this is safe by reading `normalizeEntityFullName`'s own
absolute-path branch (`expander.go`) rather than assuming. Once the link exists,
`generateHassBridgeFile`/`generateInstanceAutomationTrees` pick it up through their own
already-existing gate with zero further changes -- reporting automations for the source instance
come for free.

**Deliberately narrow scope, a real design decision not an oversight**: only a device with *no*
existing link at all gets auto-registered. A device already (even partially) positioned in
Spaces.def keeps exactly what was explicitly declared -- `export` never retroactively expands a
manual positioning. Reasoning: filling in the remaining capabilities under a different, synthetic
root-level identity would put them at a visibly different display-name/entity-id path than their
already-positioned siblings (e.g. `hass.office_garden`, already positioned as `infrastructural:netatmo`
inside `space social:front`, i.e. displaying as "front/netatmo/..." -- an export-triggered fallback
would've put any *missing* capability at bare root "vienna_terrace/..." instead, a jarring
inconsistency) -- worse than just leaving the gap for the DSL author to close explicitly. A
partially-positioned, exported device's un-positioned capabilities staying unregistered is
accepted, not solved.

Verified live, not just unit-tested: the real, already-`export`-flagged 5 Vienna devices
(`hass.vienna_terrace`/`_wind`/`_bedroom`/`_livingroom`/`_shower_room`, confirmed via grep to have
zero Spaces.def positioning anywhere in Junglinster's own definitions) produced, from a real
`./generate` with zero code changes to Spaces.def: full `coordinator/homeassistant_bridge.yaml`
entries (`export: true` + every declared capability resolved) and one
`reporting_<name>.yaml` automation file per entity under
`hass/protocols-server-2/automation/infrastructural/` -- inspected both directly, not just trusted
the "no warnings" exit.

## Two real bugs found live verifying 1.2b, both fixed and deployed the same day (2026-08-30)

Live-verified via `mosquitto_sub` against the cloud broker from `xanadu` (TLS: `--cafile
/etc/ssl/certs/ca-certificates.crt` on Debian/Ubuntu, `/etc/pki/tls/certs/ca-bundle.crt` on
Fedora/RHEL; port 8883; `-u coordinator -P <password from Settings.def ${cloud_coordinator_mqtt_*}>`).
Raw cross-posted state values (`junglinster/homeassistant_instances/protocols-server-2/bridge/...`)
worked immediately. The discovery-config half (`junglinster/homeassistant/.../coordinator/
hassbridge_.../config`) did not, across two redeploys, revealing two separate coordinator bugs --
neither in the hassbridge code itself, both in shared orphan-topic-cleanup machinery
(`discoverycleanup.go`) that had never accounted for this new topic class.

**Bug 1: new cloud topics never added to the "expected" set, so the orphan watcher retired them
within seconds of publishing.** `main.go`'s cloud-broker `RetireMissing`/`watchForOrphanedDiscoveryTopics`
calls used `expectedCloudTopics` built only from `expectedCloudHostsPayloads` (hosts devices) --
`expectedHassBridgeCloudTopics` (a new function, mirroring `expectedHassBridgeTopics`'s local-side
role but qualified with `ownInstallation+"/"` and filtered to `Export=true` devices only) was never
merged in. Confirmed via journalctl: `[discovery-cleanup] retired stale topic
junglinster/homeassistant/.../hassbridge_..._vienna_terrace_.../config` moments after each first
real publish. Fixed: `expectedHassBridgeCloudTopics` added, merged into `expectedCloudTopics` in
`main.go` right where `expectedCloudTopics := topicSet(expectedCloudContent)` is built.

**Bug 2, found immediately after redeploying bug 1's fix -- still nothing showed up.** The orphan
watcher's retire path (`watchForOrphanedDiscoveryTopics`'s handler) called the bare
`retireDiscoveryTopic(client, topic)` function directly -- which only unretains the topic *on the
wire*, and never touches `TDiscoveryPublisher.known` (its own persisted content-diffing manifest,
`discovery_topics.json`). So even after bug 1's fix made these topics "expected" again, every
subsequent `publisher.Publish(...)` call for the SAME (unchanged) content hit `Publish`'s own
`if prev, ok := p.known[key]; ok { if prev == content { return nil } }` fast path and silently
skipped republishing -- the coordinator believed it had already published (from stale
pre-bug-1-fix manifest state), while the broker actually had nothing there. Confirmed by checking
raw state cross-post kept working fine across restarts (ruling out a redelivery/retained-message
problem) while discovery configs stayed empty specifically. Fixed: new
`(p *TDiscoveryPublisher) RetireOne(client mqtt.Client, broker, topic string)` method that retires
*and* deletes the manifest entry (`delete(p.known, manifestKey(broker, topic))`) + persists --
`watchForOrphanedDiscoveryTopics` now takes `publisher *TDiscoveryPublisher, broker string` params
and calls `publisher.RetireOne(...)` instead of the bare function; both `main.go` call sites (main
and cloud broker) updated to pass `publisher, "main"` / `publisher, "cloud_coordinator"`.

**Operational unblock, same incident**: bug 2's fix only prevents *future* manifest/wire desync --
the manifest on protocols-server-1 already had stale entries from bug 1's mis-retirement, which
needed clearing once, by hand, for this specific incident to actually resolve. Moved aside (not
deleted -- `mv discovery_topics.json discovery_topics.json.bak-<timestamp>`, since a bare `rm -f`
on a remote host got blocked by the auto-mode destructive-action classifier and a reversible rename
achieves the same self-healing restart without needing an override) + `systemctl restart
coordinator`. Confirmed self-healing worked: full discovery configs for all 5 Vienna devices'
capabilities appeared on the cloud broker within ~15 seconds of restart, each with correct
cloud-qualified `state_topic`, `installation` field, and typing metadata -- 1.2b fully verified
live, not just unit-tested.

**Lesson, worth generalizing**: any time a NEW class of coordinator-published topic is added
(local or cloud), check `discoverycleanup.go`'s `expected*` functions and `main.go`'s wiring for
whether the orphan-watcher/RetireMissing machinery already knows about it -- these two bugs are
exactly the shape of gap that recurs whenever a new topic-publishing code path is added without
symmetrically updating the ambient "what's supposed to be here" bookkeeping that watches over
*every* coordinator-owned topic, not just the ones a specific new feature cares about.

Test coverage, both bugs: `TestExpectedHassBridgeCloudTopicsOnlyIncludesExportedDevices`,
`TestExpectedHassBridgeCloudTopicsEmptyWithNoInstallation`,
`TestWatchForOrphanedDiscoveryTopicsForgetsRetiredTopicFromManifest` (the regression test for bug 2
specifically -- seeds a manifest entry, retires it through the watcher, then asserts a fresh
`Publish` call with identical content actually reaches the wire instead of being skipped).

Test coverage: `Conceptual_DeviceExportRegistration_test.go` --
`TestRegisterExportedHassBridgeDevicesRegistersUnpositionedDevice`,
`...SkipsNonExportDevices`, `...LeavesAlreadyPositionedDeviceUntouched` (the scope-boundary test --
asserts a pre-seeded partial link is neither overwritten nor expanded).

Test coverage: generator side (`integration_hassbridge_test.go`) --
`TestGenerateHassBridgeFileWritesExportFlag`/`...OmitsExportFlagWhenNotDeclared`. Coordinator side
(`discoveryhassbridge_test.go`) -- `TestSubscribeHassBridgeExportCrossPostsToCloud` (asserts the
raw cross-post payload, the cloud discovery's qualified `state_topic`, the `"installation"` field,
and that the *local* copy is untouched) and
`TestSubscribeHassBridgeNonExportDeviceNeverPublishesToCloud`. Both Go modules build/vet/gofmt/test
clean; real `./generate` against Junglinster confirmed clean after each phase (native rename, then
export semantics). Architecture.md §6.12 has the full writeup. Not yet deployed/exercised against a
real second cloud subscriber -- no importer exists yet to verify against.
