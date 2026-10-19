---
name: project-entity-existence-inquiry-design
description: "PROJECT.md item 1.1: three-state entity-existence tracking + paced inquiry loop + optimistic generation. BUILT, DEPLOYED, and confirmed running live 2026-08-28 (both main and protocols-server-2). Old manifest/entities_detailed mechanism removed entirely, superseded by device_entities()-based discovery. Command-automation half deferred to PROJECT.md 1.6. Known gap: can't bootstrap an entirely new, undeclared device -- see body. 1.1b (full Spaces.def Netatmo migration to the device/for-block pattern) DONE and deployed 2026-08-30."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-08-29T22:43:17.837Z
---

Full replacement for the 2026-08-27 statestream-based redesign — not a refinement of it. Both
`mqtt_statestream` and `event_stream` turned out to be classified **legacy** in current Home
Assistant, ruling out passive stream-based observation as a design basis at all. If a future
session finds the old statestream write-up (git history, prior PROJECT.md revisions, earlier
memory drafts), it is superseded — do not resurrect or re-propose it.

Original prompt for this whole line of work still holds: forcing the old "report entities
detailed" bootstrap automation on "main" made the entire instance unresponsive
(`entitiesDetailedPayloadTemplate`'s `{% for s in states %}` loop, synchronous, per-entity
`device_id()`/`device_attr()`/`area_name()` calls, over every state on the instance) —
`automation.coordinator_bootstrap.yaml` generation is disabled (`remote_instance_automations.go`,
2026-08-27) until this design replaces it.

## Model: three-state entity existence, tracked per integration and per device

The coordinator tracks, per integration (grouped per device where the entity is device-linked), a
status per entity:

- **known-to-exist**
- **not-known-to-exist** — unresolved, not yet inquired about
- **known-not-to-exist** — inquired and confirmed absent

Status is derived from the coordinator's own inquiry plus the involved integration's own report.

`.def` files (Physical.def/Spaces.def) reference entities the generator cannot itself confirm
exist on a live MQTT-based integration at generate time — these are **"assumed to exist"**
references, and are exactly the seed list handed to the coordinator (see below).

## Generation rule: optimistic by default, error only on confirmed absence

Generate code/YAML **as if the entity exists** in every case *except* known-not-to-exist. Only a
*confirmed*-absent entity (known-not-to-exist) is a generate-time **error** — not-known-to-exist is
*not* an error, it still generates optimistically. This is a deliberate shift from "don't generate
until confirmed" to "generate unless disproven."

What "as if it exists" produces:
- the needed MQTT discovery message, same as today
- for entities sourced from a `home_assistant` bridge instance: the automations needed to
  report/respond to state at HA startup time, state changes, and received commands

## File/topic naming for per-entity home_assistant-instance automations

- **State reporting**: one trigger/action pair per entity, in **one YAML file per entity** (not
  one shared file for the whole instance, unlike the old bootstrap automation):
  - filename: `reporting_<fully_qualified_entity_name>.yaml`
  - friendly name: `reporting/<fully_qualified_entity_name_with_slashes>`
- **Commands** (e.g. `light_on`): one file per entity *per command*:
  - filename: `command_<fully_qualified_entity_name>_<command>.yaml`
  - friendly name: `command/<fully_qualified_entity_name_with_slashes>/<command>`

"Friendly name" is HA's own notion of friendly name, confirmed by the user (2026-08-28): here it is
the generated automation's own friendly name -- its `alias:` field in the YAML -- not an MQTT topic
segment. So the slash-joined forms above (`reporting/<...>`, `command/<...>/<command>`) are literal
`alias:` values; the underscore-joined forms are the filenames (and, following this generator's
existing convention elsewhere, presumably also feed the automation's `id:` -- not yet explicitly
confirmed, check when implementing).

## Generator ↔ coordinator protocol

- **Generator → coordinator**: publishes its "assumed to exist" entity list (everything referenced
  in `.def` files) — this is the seed/starting point for the coordinator's own inquiries, replacing
  the old statestream-based "cold-start seed automation" idea entirely.
- **Coordinator inquiry cadence, per integration, at most one inquiry per minute**:
  1. Entities with not-known-to-exist status first (the backlog of unresolved/newly-seeded ones).
  2. Once every not-known-to-exist entity has been inquired about: sequentially re-check every
     *known* device — does it still exist, what entities does it currently provide (a refresh
     pass, not just first-discovery).
  3. Then any entities with no associated device (orphans).
- **Coordinator → generator**: the resulting status list (known-to-exist / not-known-to-exist /
  known-not-to-exist), grouped per device where the entities are device-linked, is published to an
  MQTT topic for the generator to pick up and refresh its own local cache — same
  fetch-when-online/fall-back-to-cache pattern the manifest-cache design already established
  ([[project_entity_catalogue_and_device_positioning]]), just carrying three-state status per
  entity now instead of a flat entity list. Exact topic name not yet decided — likely reuses or
  extends `meta/manifests/<installation>/<instance>/state`.

## Implementation status (2026-08-28)

**Built and verified live**: per-entity state-reporting automation generation
(`remote_instance_entity_reporting.go`) — `fullyQualifiedEntityNameUnderscored`/`...Slashed` built
from `TEntityIdentity` (added an `Identity` field to `TDeviceAttributeLink` for this, since the
flattened entity_id can't be un-flattened losslessly), `generateHassBridgeEntityReportingAutomations`
writing one `reporting_<name>.yaml` per positioned capability, wired into
`generateInstanceAutomationTrees` replacing the old shared per-instance automation.

**Real bug found and fixed while building this** (pre-existing, not introduced by 1.1's work):
`device.Capabilities` and `link.AttributeEntityIDs` used different map keys (bare capability name
vs. resolved entity_id) for anything positioned via `entity ... from <device-id> entity
<capability>;` — silently dropped those entities from both `coordinator/homeassistant_bridge.yaml`
and the new reporting generator. Confirmed live: `hass.davids_bedroom`'s entire Netatmo capability
set was missing from the bridge file before the fix, present after. Fixed via a `capabilityKey`
parameter on `registerDeviceSourceEntityLink` — see Conceptual_DeviceSourceEntities.go's own doc
comment for the full mechanism.

**Built and verified live, same day, continued**: the actual three-state inquiry mechanism —
- `remote_instance_entity_existence.go` (generator): `entityExistenceInquiryAutomationBody` — one
  automation per *instance* (not per entity), a single `states[trigger.payload] is not none` dict
  lookup answering about whatever entity_id arrives as the inquiry command's payload. Safe at any
  instance size — the opposite shape from the disabled all-states-scan automation that hung main.
  Written for every declared instance unconditionally (main included, no hassbridge devices
  required yet).
- "Assumed to exist" seed needed no new generator output — `coordinator/homeassistant_bridge.yaml`
  already *is* that list; the coordinator seeds from what it already loads.
- `house_event_bus_coordinator/entity_existence.go` (new file): `TEntityExistenceTracker` — seeds
  from `THassBridgeFile`, paced per-instance (≤1/minute) inquiry loop with the two-tier priority
  (unknown first, then round-robin refresh of resolved ones — the orphan tier is a no-op today,
  every hassbridge capability is device-linked), publishes grouped-per-device status to
  `homeassistant_instances/<name>/existence/state` (main broker only, never cloud — unlike the
  older manifest topic). Wired into `main.go`.
- `mqtt_entity_existence.go` (generator, new file): `fetchEntityExistence` — same
  fetch-then-cache-fallback pattern as the older manifest mechanism, but a separate topic/cache,
  not shared code. `checkKnownNotToExistErrors`, wired into `Physical_Generator.go`: only a
  confirmed known-not-to-exist verdict blocks generation; unresolved and confirmed-existing both
  still generate optimistically.

Verified against real Junglinster: full pipeline runs clean end-to-end, both Go modules build/vet/
test/gofmt clean. Not yet deployed to any live instance — this was all built and verified locally
in one continuous push after the user pointed out (correctly, and with some frustration) that only
the peripheral reporting-automation piece had been built while the actual inquiry mechanism — the
point of the whole design — was still entirely missing. Lesson: when a design has a "core
mechanism" and a "peripheral piece," build the core first, or at least don't imply more progress
than the core mechanism actually has.

**Correction, same day**: the first pass of the inquiry loop wrongly made existence status
local-broker-only — inventing a new policy instead of following the already-established, discussed
one: cloud broker first for generator reads, fall back to local; coordinator relays to both
unconditionally. This is exactly what `subscribeEntityCatalogue`/`fetchManifest` already do for the
older manifest topic ([[project_entity_catalogue_and_device_positioning]]) — should have been
mirrored immediately, not reinvented. Fixed same day: `coordinatorOnlyBrokerSecrets` (shared "find
the cloud profile" helper, was `manifestBrokerSecrets`), both `fetchManifest` and
`fetchEntityExistence` now try cloud→local→cache, coordinator's `publishStatus` now relays to both
brokers unconditionally (inquiry commands/replies stay local-only, only the aggregated status needs
cloud reach). Worth remembering generally: when adding a new MQTT-read/publish mechanism to this
codebase, check the existing manifest-topic mechanism's broker policy first and mirror it, rather
than deciding a policy from scratch per feature.

**Command-automation generation deliberately deferred, not designed yet** — there's no existing
notion anywhere in this codebase of which commands a capability/domain supports or how one maps to
a remote service call on the bridged instance, so building it now would be guessing. The user
placed this explicitly at **PROJECT.md 1.6** (Overkiz-based SOMFY cover control) — the first
commandable domain (open/close/stop) actually coming up; 1.5 (Volvo) may bring buttons/locks even
earlier if it lands first. Resolve the command semantics there, not before, and not by inventing a
shape speculatively now.

**Still open**: inquiry-state persistence across coordinator restarts (today every restart re-seeds
and re-learns from scratch — PROJECT.md 1.1 itself still lists this as open, not a regression).

**Old manifest mechanism removed entirely, same day, after a second sharp correction.** The user
initially asked (after seeing main's manifest fetch fail forever) whether the mechanism should be
retired or rebuilt safely later — I offered that as a multiple-choice question. Wrong move: the
user had already told me the answer earlier in the session ("the txt file is based on the
'known-to-exist' status of the three-valued approach") — I just hadn't connected it. The design was
never "manifest OR existence-tracking" as two competing options; it was always "existence-tracking
subsumes what the manifest mechanism did," via the design's own "checks all known devices ... and
which entities they provide" clause, which I had built only half of (confirm-one-entity, not
discover-what-a-device-has). Corrected same session, no more questions asked:
- `remote_instance_entity_existence.go`'s inquiry automation reply now also carries `device_id` and
  `sibling_entities` (`device_entities(did)`) -- one extra cheap registry lookup per inquiry, not a
  second automation or a second inquiry type.
- `entity_existence.go` gained `DiscoverSiblings`: any sibling not already tracked is folded in as
  known-to-exist, attributed to the same DSL device as the anchor entity that revealed it.
- `mqtt_entity_catalogue.go` (generator) and `entity_catalogue.go` (coordinator) — the entire old
  manifest/`entities_detailed` mechanism — deleted outright, not deprecated-in-place.
  `generateEntityCatalogueSuggestions` rewritten on top of `TEntityExistenceStatusPayload`; device
  groupings are now always real `hass.<id>` Physical.def ids (no more guessing a name from HA's own
  live device-name report), and the suggestion file now correctly **deletes** a stale file when a
  fresh, successful fetch finds nothing left to suggest, rather than silently leaving old content
  behind looking current (a real bug caught live: an old manifest-era suggestions file sat
  unchanged through a `./generate` run that had already made it inaccurate).
- **Real, permanent scope gap, not a bug**: `DiscoverSiblings` can only surface new entities on a
  device the tracker already has *some* anchor entity for. It can never bootstrap an entirely new,
  never-declared device (no anchor exists to discover from) -- something the old full-instance-scan
  manifest mechanism could do. Confirmed live: both instances' suggestions files went empty after
  this rewrite, since everything currently tracked is already declared. The "migrate the other
  Netatmo rooms" workflow this session used the suggestions file for has no working bootstrap path
  right now -- needs a new, deliberately one-time/manual mechanism (not automatic, not
  automation-triggered on every restart) for discovering a device from scratch. Not designed.

Confirmed deployed and running live the same day: coordinator redeployed, both instances'
automation trees deployed, inquiry loop confirmed running against both main and protocols-server-2
without incident (the whole point, after 2026-08-27's outage on exactly this kind of automation).

**Discovered-device grouping fix, same day.** The "Discover entity" bootstrap (§ above) seeds a
manually-typed entity with `DeviceID == ""` (no owning DSL device known yet). `DiscoverSiblings` was
grouping any siblings it found under that same `""`, so a genuinely new device (tested live:
`office_storage`, 5 Netatmo entities) always landed in the suggestion report's "no known device
grouping" section rather than a clean `device hass.<id> with: ...; end;` block — user flagged this
directly ("these should have a device grouping"). Fixed:
- `remote_instance_entity_existence.go`'s inquiry reply now also carries `device_name`
  (`device_attr(did, 'name')`) alongside the existing `device_id`/`sibling_entities`.
- `entity_existence.go`'s `DiscoverSiblings` gained a `deviceName` parameter: when the anchor's own
  `DeviceID` is `""` and it just found siblings, it mints `hass.discovered_<slugify(deviceName or
  anchorEntity)>` and assigns it to the anchor's own entry too, so it sticks for every later
  discovery round on the same anchor (not re-minted each time).
- New `slugify` helper (coordinator-side, minimal — lowercase, non `[a-z0-9]` runs collapsed to a
  single `_`, trimmed).
Unit-tested both sides; confirmed live 2026-08-28 against `office_garden`: journal showed
`binary_sensor.office_garden_connectivity has no known device, grouping it and its siblings under
synthetic device "hass.discovered_office_garden"`, then 4 siblings folded in under that same id.

**Follow-on, same day: Physical.def overrides a synthetic name.** Once `office_garden` showed up in
the suggestion file grouped under `hass.discovered_office_garden`, the user declared 3 of its 5
entities under a real name (`device hass.office_garden with: ...; end;`) and expected the remaining
2 to suggest under `hass.office_garden` too — not the synthetic placeholder. Explicit framing: "It
is OK for the coordinator to use the 'discovered' name, but Physical.def should provide the
overriding name for a device." Fixed generator-side (coordinator untouched — this is purely a
suggestion-report rendering concern, no MQTT/coordinator change needed): new
`declaredDeviceIDByEntity(hassBridgeDevicesByID, instanceName) map[string]string` (mirrors
`usedHassBridgeEntityIDs`, but keeps the real device id instead of collapsing to a bool);
`buildSuggestionReportFromExistence` now checks, per coordinator-side status group, whether any
entity in it has a declared real id, and if so renders the whole group's still-undeclared entities
under that real id. `usedHassBridgeEntityIDs` refactored to build on `declaredDeviceIDByEntity`
rather than duplicating the same iteration.

**Debugging note, worth remembering generally**: while chasing why used-filtering "wasn't working"
in the user's live output, an isolated repro test showed the filtering logic was actually already
correct — the real cause was a stale installed `homeassistant` binary (hadn't been `go install`ed
since the `device_name`/`rf_strength` changes earlier the same session). Re-confirms
[[feedback_terse_chat_and_rebuild_before_test]]: always suspect a stale binary before assuming new
logic is broken, especially when an isolated unit test of the exact same logic passes.

**Also same day**: `rf_strength` added to `recognizedCapabilityKeywords` (→ `sensor.radio`, same
target as the pre-existing `wi_fi_strength` keyword) — a real gap, not a copy of an existing case.

**Scope correction, 2026-08-29: kind-2 needs the same tracking too, just not active inquiry.**
Architecture.md §6.10 originally concluded kind-1 *and* kind-2 both need zero existence-tracking
machinery, since both "self-announce." The user corrected this with a precise layering principle:
conceptual-layer entities are either defined via other conceptual entities, assumed native on main,
or defined via the physical (or later logical) layer — and existence-checking only ever concerns
that third case, and only ever concerns the physical layer's own declared *source* specification
(a capability line's right-hand side, e.g. `sensor.outdoor_temperature: sensor.boiler_outdoortemp;`'s
`sensor.boiler_outdoortemp`), never the conceptual-layer name on the left. Reframed per-kind:
- **Kind 1 (hosts)**: still correctly needs nothing — direct link, the coordinator is the source.
- **Kind 2 (discovery)**: was wrongly lumped with kind-1. It doesn't need *active inquiry* (a
  gateway self-announces via native discovery, unlike a remote HA instance) — but it still needs
  the coordinator to *track and report* its declared sources' three-state status, exactly like
  kind-3, just populated passively instead of by paced inquiry. The generator currently has **zero**
  existence signal for kind-2 sources — a real, previously-unnoticed blind spot, not a "nothing to
  track" case.
- **Kind 3**: unchanged — the only kind needing active inquiry, since a remote HA instance never
  tells the coordinator anything unprompted.

Scoped as **PROJECT.md 1.8**, not folded back into 1.1: per-gateway tracker keyed by
`(gatewayID, TDiscoveryEntityLink.Leaf)`, seeded not-known-to-exist from `discovery.yaml`'s
`EntityLinks`, populated by `discoverybridge.go`'s *existing* discovery-payload subscription
handler (arrival → known-to-exist, no new subscription needed) — status topic + generator-side
fetch/cache mirroring kind-3's own mechanism. Retraction (known-not-to-exist) explicitly deferred:
an empty retained discovery payload carries no `unique_id` to decode, so recognising which
`(gatewayID, leaf)` a retraction belongs to needs a topic→identity map that doesn't exist yet —
build arrival-only tracking first. Architecture.md §6.10 rewritten to match, PROJECT.md 1.1's own
"Scope confirmation" note corrected to point here.

**Built same day, first phase.** Coordinator: `discovery_existence.go` (new file) --
`TDiscoveryExistenceTracker` reuses `entity_existence.go`'s own `TEntityExistenceStatus`/`Status*`
constants (same package, no redefinition). `MarkKnown` is called directly from
`discoverybridge.go`'s existing handler (signature gained `cloudClient mqtt.Client,
existenceTracker *TDiscoveryExistenceTracker`, both accepting nil for tests/no-op). Status topic
`discovery_gateways/<gatewayID>/existence/state`; persisted to `discovery_existence.json`
(mirrors `entity_existence.json`'s pattern exactly, including the `path == ""` disables-persistence
convention for tests). Generator: `mqtt_discovery_existence.go` (new file) --
`fetchDiscoveryExistence`/`checkDiscoveryKnownNotToExistErrors` mirror
`fetchEntityExistence`/`checkKnownNotToExistErrors` almost line for line; wired into
`Physical_Generator.go` right after the kind-3 check, using `admin.DiscoveryEntityLinks` (already
populated by parsing, no new generator output needed to seed it -- same "already have what we
need" pattern as kind-3's `homeassistant_bridge.yaml`). Confirmed via a real `./generate` against
Junglinster: soft-fails gracefully offline (no broker, no cache) exactly like kind-3. Unit-tested
both sides (`discovery_existence_test.go`, `discoverybridge_test.go`'s two new tests,
`mqtt_discovery_existence_test.go`). Deployed live same day, confirmed clean (no errors on
deploy/generate) -- not yet exercised against a real gateway discovery payload.

**Retraction built same day too, after a correction.** I initially wrote off retraction
(known-not-to-exist) as needing "a topic→identity map the coordinator doesn't keep" — the user
pushed back, correctly: the discovery-payload handler already has both the topic and the decoded
`(gatewayID, leaf)` together at the moment any real payload arrives, so remembering
`topic -> (gatewayID, leaf)` as it goes costs nothing (`RecordTopicIdentity`) — no new subscription
needed. And since discovery config topics are retained, the map doesn't even need to survive a
restart on disk: a coordinator restart's own subscribe naturally replays every gateway's current
config first, so the map self-heals in memory alone. An empty payload (HA's own MQTT discovery
removal convention) resolves via that map (`MarkRetracted`) and moves the leaf to
known-not-to-exist. `checkDiscoveryKnownNotToExistErrors` needed zero changes — already written
expecting all three states. Lesson: don't declare something "needs new infrastructure" without
checking whether the data is already sitting right there in the handler's own locals.

**"Discover entity" now accepts a semicolon-separated batch, 2026-08-29.** The user wanted to
paste a whole list of entities (one per line, each ending in ";", some bare, some
"<instance>: "-prefixed) into the single text-field control in one submission, rather than typing
one at a time. `discover_entity_input.go` gained `splitDiscoverEntityRequests` (splits on ";",
trims, drops blanks) and `subscribeDiscoverEntityRequests`'s handler now loops over the split
requests, applying the existing per-request `parseDiscoverEntityRequest`/seed/`InquireNow` logic
to each -- one bad entry (unrecognised instance) is logged and dropped without aborting the rest
of the batch. Fully backward compatible: a payload with no ";" at all still produces exactly one
request. Unit-tested (`TestSplitDiscoverEntityRequests`,
`TestSubscribeDiscoverEntityRequestsHandlesSemicolonSeparatedBatch`,
`TestSubscribeDiscoverEntityRequestsBatchContinuesPastUnknownInstance`); Architecture.md §6.9
updated to match. Not yet deployed/exercised live.

**Real, previously-mysterious bug found and fixed live 2026-08-29: deploy was wiping the
coordinator's own persisted runtime state on every deploy.** The user submitted the semicolon-batch
"Discover entity" request live before that feature was actually deployed (see the "root cause"
investigation below), which surfaced this: `Junglinster/deploy.d/04_deploy.coordinator` pushes the
local `coordinator/` dir (generator-authored YAML only) to the coordinator's own working directory
on the remote host (`/home/erikp/coordinator`, confirmed via `coordinator.service`'s `ExecStart` —
the *same* directory the running coordinator persists `entity_existence.json`/
`discovery_existence.json`/`discovery_topics.json` into) using `rsync --delete`. Since those three
files are remote-only (never present in the local source tree), `--delete` silently erased all
three on *every* deploy, right before restarting the coordinator — defeating the whole point of
building persistence-across-restarts, since a deploy is exactly the restart it needs to survive.
This retroactively explains an earlier live mystery: `hass.office_garden`'s confirmed
`known-to-exist` entry had inexplicably vanished from the coordinator's published status after a
later deploy — this bug is why, not a coordinator logic bug. Fixed:
`--exclude entity_existence.json --exclude discovery_existence.json --exclude discovery_topics.json`
added to that rsync line. Documented as Architecture.md §6.11 — any future coordinator-persisted
runtime file must be added to that exclude list too, flagged explicitly in the doc so it isn't
missed next time.

**Real bug, same day: synthetic device naming collapsed two different remote devices sharing a
display name.** After the multi-entity batch resolved, the suggestion output showed
`hass.discovered_vienna_terrace` with 10 entities that were actually two separate Netatmo modules
(an "Outdoor Module" — connectivity/battery/humidity/rf_strength/temperature — and a "Wind Gauge" —
connectivity_2/battery_2/wind_speed/rf_strength_2/wind_direction) merged into one device block. Root
cause: `syntheticDeviceID` minted purely from `device_attr(did, 'name')`, and both modules are
auto-named "Vienna Terrace" by Netatmo/HA — a non-unique key. Fixed: `DiscoverSiblings` now takes
`remoteDeviceID` (the inquiry reply's own `device_id`, already fetched for
`device_entities()`/`device_attr()` but never threaded through before) and a new
`resolveSyntheticDeviceID` reuses an existing synthetic id for the *same* remote device_id, or
disambiguates with a `_2`/`_3` suffix when a *different* remote device_id would otherwise collide
on the same name-derived slug — mirroring how the remote instance's own entity naming already
disambiguates same-named siblings. New `RemoteDeviceID` field on `TEntityExistenceEntry`
(unexported bookkeeping, not published to the generator) — no separate tracker-level map needed,
the existing entries map already carries enough to detect a collision. Unit-tested
(`TestResolveSyntheticDeviceID`,
`TestEntityExistenceTrackerDiscoverSiblingsDisambiguatesSameNameDifferentRemoteDevice`).
Architecture.md §6.9 updated. Not yet deployed; the already-poisoned `hass.discovered_vienna_terrace`
entry on the live coordinator needs clearing (same `entity_existence.json` deletion approach as the
other incidents this session) once redeployed.

**Root-cause investigation, same incident**: the user reported "not much seems to have been added"
after submitting the batch request, and expected suggestions that never appeared. Before finding
the deploy bug above, I confirmed via `discover_entity_input.go`'s own mtime (11:01 WEST) vs. the
journal timestamps showing the *old* single-string behavior (06:08-06:13, hours earlier) that the
running coordinator simply predated the semicolon-split feature — no bug in that feature itself,
just a deploy that hadn't happened yet relative to when the code was finished. Separately, the
first (pre-split) submission's entire multi-line payload got treated as one literal (unresolvable)
entity_id, which — per `nextToInquire`'s "first not-known-to-exist wins" priority order — then
permanently starved every other not-known-to-exist entity from ever being inquired on both `main`
and `protocols-server-2`, the same starvation shape as the earlier `" is available"` sugar bug. That
garbage entry needs clearing (via the now-safe-to-lose `entity_existence.json`, or once the deploy
fix lands, a fresh restart) before resubmitting the batch.

**Real trap found live-verifying the override fix, same day.** The user's first attempt to declare
`hass.office_garden` in Physical.def placed the `device hass.office_garden with: ...; end;` block
right *after* the enclosing `integration home_assistant protocols-server-2 with:` block's own
closing `end;`, not inside it — an easy mistake, since the suggestion `.txt` file only ever shows
the bare `device ... end;` block, never its wrapping `integration home_assistant <qualifier> with:`
context, so nothing hints that placement matters. `collectHassBridgeDevicesByID` only ever looks
for device declarations *inside* a matched integration block; outside one, the whole declaration
silently vanished — no warning, no error — which is exactly why both the used-filtering and the
name-override appeared broken on the first live check, even though an isolated unit test of the
exact same logic already passed (a second instance of
[[feedback_terse_chat_and_rebuild_before_test]]'s lesson: suspect environment/input before
assuming new logic is broken). Fixed two ways: (1) relocated the block in the real Physical.def;
(2) added `warnAboutOrphanedHassBridgeDeviceBlocks` (`integration_hassbridge_parser.go`) so this
never silently vanishes again — it scans the whole file for any `device hass.<id> with:` header
whose id never resolved into `hassBridgeDevicesByID`, and warns with a line number.

**Remaining three "beyond today" items closed out the same day**:
- **Persistence across restarts** — `entity_existence.json` (mirrors `discoverycleanup.go`'s
  `discovery_topics.json` pattern exactly: `newEntityExistenceTracker(path)` loads it up front,
  every mutating method (`Seed`/`Record`/`DiscoverSiblings`/`SeedManualEntity`) re-persists while
  already holding `t.mu`). `path == ""` disables persistence outright — used by every test, only
  `main.go` passes a real path (`coordinatorDir/entity_existence.json`). Not yet exercised via a
  real coordinator restart in production, only unit-tested.
- **Scope confirmation** (existence tracking is kind-3-only) — written up as Architecture.md §6.10.
  The framing that stuck: kind-1/kind-2 sources are self-announcing by construction (a host's own
  script, a gateway's own native discovery payload/retraction), so the coordinator trusts that
  "advertiser" outright with no separate verification round; kind-3 is the only case where a source
  entity is a bare, never-confirmed assumption about another HA instance's own state machine, which
  is exactly the gap the whole entity-existence design exists to close.
- **"Orphan entities with no owning device" tier** — turned out to be a documentation artifact, not
  real dead code: `nextToInquire` never had a separate third pass for device-less entities in the
  first place (its flat priority order already treats every tracked entity, `DeviceID` or not,
  identically). Nothing to delete; corrected the stale doc comments in `entity_existence.go`
  instead of removing code that was never there.

**Reporting automations now also fire on a plain automations reload, 2026-08-29.** Diagnosing why
`binary_sensor.infrastructural_house_corridor_netatmo_node` stayed "unknown" after a deploy
surfaced a real, generally-applicable gap: `entityReportingAutomationBody`'s only "republish
current state proactively" trigger was `homeassistant: start`, which fires *only* on a full HA
restart -- a plain "reload automations" (far more common day-to-day, and the only thing deploying
this very automation itself triggers) left the entity stuck on stale/unknown until either a real
restart or the next incidental source state-change. Home Assistant's `homeassistant` trigger
platform has no "reload" event at all (only `start`/`shutdown`) -- but the automation integration
separately fires an `automation_reloaded` *event* on the generic event bus whenever automations.yaml
reloads, triggerable via the plain `event` platform:
```yaml
- platform: event
  event_type: automation_reloaded
```
Added as a third trigger to every generated reporting automation (`remote_instance_entity_reporting.go`).
Deliberately NOT added to `hassBridgeReportingAutomationBody` (the separate device-info reporting
automation) in the same pass -- that one has a different, pre-existing gap (no restart trigger at
all, not just no reload trigger) that wasn't asked about and wasn't touched. Unit-tested; confirmed
via a real `./generate` that the new trigger appears in the generated YAML. Needs the normal
deploy + per-instance "reload automations" action to take effect live (this is an actual HA
automation content change, not a coordinator-only one).

**PROJECT.md 1.1b DONE, 2026-08-29/30: full Spaces.def Netatmo migration, deployed live.** Every
remaining old-style Netatmo block in Spaces.def (bare `entity sensor.physical:netatmo/co2;` +
`entity binary_sensor.infrastructural:netatmo/node with: condition ...; end;` +
`entity device.infrastructural:netatmo from host.<X> with: all entities;`) migrated to the
`device infrastructural:netatmo from hass.<device>;` (auto-registers node) + `for hass.<device>:
entity <spec> from entity <capability>; ...; end;` pattern, using exactly the capability set each
`hass.*` device actually declares in Physical.def (dropping e.g. `radio`/`noise`/`pressure` lines
for devices that don't have them, rather than carrying over the old space's full field list
unconditionally). Final mapping, all 11 spaces:
`front`→`hass.office_garden`, `living_room`→`hass.living_room`, `kitchen`→`hass.living_room_kitchen`,
`first_floor`→`hass.living_room_hallway_first_floor`, `sarah_bedroom`→`hass.sarahs_bedroom`,
`david_bedroom`→`hass.davids_bedroom` (done earlier), `master_bedroom`→`hass.master_bedroom`,
`shower_room`→`hass.living_room_shower_room`, `corridor`→`hass.office_corridor` (done earlier),
`storage_room`→`hass.office_storage`, `bathroom`→`hass.office_bathroom`. Three of these devices
(`living_room_kitchen`, `living_room_hallway_first_floor`, `living_room_shower_room`) didn't exist
yet at the start of this migration — the user triggered a live coordinator "Discover entity" inquiry
per missing room mid-session and reported back the resulting `hass.*` id once discovery completed,
rather than these being pre-known. Also fixed in passing: `sensor.heatlh` → `sensor.health` typo in
`hass.sarahs_bedroom`'s Physical.def block (caught because the migrated Spaces.def `for` block needed
the exact capability key to reference). `./generate` run clean after every single space's edit (not
batched at the end) — confirmed via live `./generate` throughout, not just at the finish. Deployed +
both instances' YAML reloaded same session; user confirmed "All seems to be functioning."

**Node-liveness source: literal connectivity sensor, not `... is available`, decided and reverted
same session.** Mid-session, all 14 (then-)Netatmo devices' `binary_sensor.node:` capability was
switched from the literal `binary_sensor.<x>_connectivity;` source to `sensor.<x>_temperature is
available;` (mirroring `host.junglinster`'s own node pattern), reasoning it would update more
often. The user later reconsidered after confirming via the suggestions file that the literal
`_connectivity` sensors are genuinely real/`known-to-exist` (not stale placeholders) — and that the
actual root cause of an earlier "stuck on unknown" node entity was the missing `automation_reloaded`
trigger (see above), not sensor unreliability. Reverted all of them back to the literal connectivity
sensor, "to keep things explicit" (the user's own phrasing) — this is the standing preference for any
*future* Netatmo device's `node` capability too: prefer the literal `binary_sensor.<x>_connectivity`
source over an `... is available` derivation, unless a device genuinely has no connectivity sensor of
its own (e.g. `hass.vienna_terrace_wind`, which shares its physical hub's connectivity sensor via a
`_2`-suffixed disambiguated name, `binary_sensor.vienna_terrace_connectivity_2`).

## Why this needed capturing carefully

The user explicitly said "capture this, this time" when dictating this design — a direct callback
to [[feedback_check_for_prior_design_specs]] (the 2026-08-27 incident where a design given once was
re-derived incorrectly the next time it was needed, costing a full rebuild and visible
frustration). Treat every clause above as load-bearing and precise, not paraphrase-safe — especially
the exact filename/topic naming patterns, the per-integration (not global) one-inquiry-per-minute
rate limit, and the three-tier inquiry priority order.
