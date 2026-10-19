---
name: project-entity-catalogue-and-device-positioning
description: "DONE (2026-08-27): generator reads live entity lists over MQTT and suggests unused-entity device blocks; new lighter device-positioning + per-capability entity grammar; old coordinator-side assumed-entities discrepancy check removed entirely as redundant."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-08-27T11:21:06.582Z
---

Three linked pieces, all landed the same day, PROJECT.md item 1.4.

**Live entity manifests** -- rebuilt twice this session; the *second* version is the one that
stands, matching a design the user had already given in an earlier session and repeated
emphatically when my first attempt didn't match it (see
[[feedback_check_for_prior_design_specs]]). Final shape: the **coordinator**
(`house_event_bus_coordinator/entity_catalogue.go`) relays each declared "home_assistant"
instance's own "report entities detailed" bootstrap automation output
(`entity_id`+`device_id`+`device_name`+`area_name`, `remote_instance_automations.go`'s
`entitiesDetailedPayloadTemplate`) onto `meta/manifests/<installation>/<instance>/state`,
retained, on **both** the local and cloud broker unconditionally whenever a cloud one is
configured -- not "prefer one." `<installation>` is `devicesFile.Installation`
(`${installation}`, already resolved elsewhere this session). A new
`coordinator/home_assistant_instances.yaml` (generator output,
`generateHomeAssistantInstancesFile`) tells the coordinator which instances to watch,
independent of whether any bridge device is declared for them yet.
The **generator** (`homeassistant/mqtt_entity_catalogue.go`) reads one instance's own manifest
topic at a time, preferring the `coordinator_only`-marked broker profile for *this read only*
(reachable from anywhere, unlike the local broker) -- and keeps a **persisted local cache** per
instance at `Definitions/.cache/entity_manifest_<name>.json` (deliberately outside any
generated-output tree the generator ever wipes): a fresh successful read updates the cache; any
failure falls back to the cache's last contents. This was the user's own explicit framing --
"fetch when online, otherwise use the old list" -- and is a deliberate, standing exception to the
"MQTT retained messages already are the cache, no local persistence needed" reasoning used
everywhere else this session; don't generalize the "no local cache" pattern from other MQTT
features onto this one.
Writes `suggestions/home_assistant_<name>.txt` -- copy-paste-ready `device <id> with: ...; end;`
blocks grouped by device, with each capability's local domain/path guessed from a keyword in the
entity's own name (recognizeCapability's table: pressure/co2/humidity/noise/temperature/
connectivity->node/wi_fi_strength->radio/battery), left blank+flagged when nothing matches.
Excludes entities a declared `home_assistant` bridge device already claims. Soft-fails per
instance (a warning, not an aborted run) when neither a fresh read nor a cache is available --
confirmed live twice: once from this dev machine (no route to the local `junglinster` broker,
clean soft-fail) and once against the real cloud broker (connects fine, correctly times out
since the *deployed* coordinator on protocols-server-1 hadn't been rebuilt with this relay yet).
**Deferred, explicitly noted, not built**: a `meta/reload/<installation>/request` topic the
deploy script posts to (cloud or local, a command-line choice) which the coordinator relays
cloud->local for its own installation, triggering each HA instance's own YAML reload via its
bootstrap automation.

**New DSL grammar, two pieces**:
- `entity <local-spec> from <device-id> entity <capability>;`
  (`Conceptual_DeviceCapabilityEntities.go`) positions one entity by its device's own already-
  declared capability label (e.g. `sensor.co2` from a `device hass.X with: sensor.co2: ...; end;`
  line) -- works for both "discovery" gateway devices (dispatches to the existing
  `registerDiscoveryEntityLink`) and "home_assistant" bridge devices (dispatches to
  `registerDeviceSourceEntityLink`), unifying what used to be two separate, differently-shaped
  constructs. **Real bug hit and fixed**: the first regex used literal single spaces between
  tokens, but Spaces.def uses multiple spaces for column alignment (`netatmo/co2                 from`)
  -- silently matched almost nothing, no warning, looked like missing positioning instead of a
  parser bug. Fixed to `\s+`; regression tests added for both padded and single-space forms.
- `device <sphere>:<path> from <device-id>;` (`Conceptual_DevicePositioning.go`) -- a *lighter*
  alternative to the bulk `entity device.<spec> from <device-id> with: all entities;` form: just
  establishes the device's shared identity (DisplayName/ConstantAttributes for HA's shared
  "device:" block) and auto-registers its "node" entity if declared, nothing else. Collected
  **file-wide, before any other interpretation** (`collectDevicePositioningDeclarations`, a
  full walk using a *throwaway* `TAdministrationState` purely to replicate the real parser's own
  space-nesting state machine -- `OpenSpace`/`OpenOtherBlock`/`HandleEndToken`, `administration.go`
  -- so a positioning statement's `DisplayName` gets the right space context regardless of where
  in the file it appears relative to what references it). This was an explicit, corrected design
  choice: the user pushed back hard on an initial "just auto-create on first single-entity
  reference" proposal, invoking an already-agreed "read everything, then interpret" principle --
  worth remembering as a standing preference for this codebase, not just this feature.
  Real naming collision hit: the bare keyword "device" alone (no "entity" prefix) isn't itself
  problematic, but a user-authored line with a stray trailing "entity" token
  (`device X from Y entity;`) gets mis-parsed by the macro-invocation fallback into a confusing
  "strict macro validation failed" *error* (not a warning) -- worth recognizing this exact
  error shape as "probably a stray token on a device-positioning line," not a real macro problem.
  Both bulk and lighter forms coexist deliberately -- the bulk form is *not* being removed, only
  deprecated informally as the newer forms get adopted (the user's own framing).

**Removed entirely, not just simplified** (the user was emphatic about this -- see
[[feedback_generator_coordinator_terminology]]-adjacent standing preference: "the transformer
does this based on the cached list" made the old mechanism "completely redundant"):
`house_event_bus_coordinator/discoveryassumed.go` (+ test), `homeassistant/assumed_entities.go`
(+ test), `collectAssumedEntityIDs` (`defined.go`), the coordinator's own
`sensor.infrastructural_meta_<name>_error`/`binary_sensor.infrastructural_meta_<name>_error`
meta entities, `coordinator/assumed_entities.yaml` (generator output), and the bare
`report_entities`/`homeassistant_instances/<name>/entities` automation+topic (its only consumer
was the mechanism just removed). **Caution for next time**: deleting `assumed_entities_test.go`
wholesale also deleted two tests for `collectHomeAssistantInstances`/
`parseHomeAssistantMainDirective` that had nothing to do with the removed mechanism -- they just
happened to live in that file. Had to reconstruct them from conversation history into a new
`defined_home_assistant_instances_test.go`. Check a test file's *full* contents before deleting
it by name/association, not just its filename.
