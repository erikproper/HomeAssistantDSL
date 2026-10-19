---
name: project-cloud-manifest-and-reload-strategy
description: "SUPERSEDED (partially): the manifest half was built 2026-08-27 (see project_entity_catalogue_and_device_positioning) and is itself now being replaced by project_entity_existence_inquiry_design (2026-08-28); the reload-trigger half is still an open TODO (PROJECT.md 1.4), unbuilt."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-08-27T23:04:01.773Z
---

Historical design (2026-08-25), kept for the reload-trigger half only. The manifest half below was
built 2026-08-27 ([[project_entity_catalogue_and_device_positioning]]) and is now itself superseded
by [[project_entity_existence_inquiry_design]] (2026-08-28, PROJECT.md 1.1) — do not read the
manifest section below as current design; it is history, not a spec to build from. The
reload-trigger section (`meta/reload/<installation>/request`) is still accurate and still unbuilt
— see PROJECT.md 1.4.

**Manifests**: the coordinator publishes, per installation/instance, an up-to-date list of
available devices+entities to `meta/manifests/<installation>/<instance>/state` (`<installation>`
= e.g. "vienna"/"junglinster", `<instance>` = the local kind-2/kind-3 host — zigbee2mqtt, zwave,
or a home_assistant incarnation). Published on both the local MQTT bus and the cloud bus. Payload
should be a "transformer-friendly" JSON shape (avoid unnecessary parsing on the consuming side).

**Transformer (generator) consumption**: the transformer keeps a persisted, per-kind-2/3-instance
list of known entities/devices. At the start of a transform run, if online and the (cloud) MQTT
broker is reachable, it fetches the fresh list from MQTT and updates its local copy; otherwise it
falls back to the previously-persisted list. This is the mechanism meant to eventually replace/
extend today's purely static, generate-time-only knowledge of what a kind-2/3 device exposes —
see [[project_new_architecture_plan]] and the plan file `splendid-zooming-lighthouse.md`'s
"interrogation"/"observation" sections, which this note refines specifically for missing-entity
detection, instance listing, and reload triggering.

**Reload triggering**: to trigger a reload, the deploy script posts a
`meta/reload/<installation>/request` message onto whichever bus (cloud or local) is named via a
command-line parameter to the `homeassistant` command; bus secrets are looked up from the .def
files. If a per-installation coordinator receives such a request via the cloud broker, it
relays/forwards it onto that installation's own local MQTT bus. Every HA instance within that
installation then triggers its own YAML reload via its existing bootstrapped automation (the
already-built `homeassistant_instances/<name>/command` = "reload" mechanism).

**Why deferred**: explicitly gated on the cloud MQTT broker existing — the user's own words: "For
now, we can keep this as a note, until we have the cloud broker on-line as part of the Netatmo
integration." Not to be started before then.

**How to apply**: when the cloud broker work begins, use this note (not the older, more
speculative §2b/§2e sketches in the plan file) as the concrete spec for manifest topics, payload
shape, and reload-trigger flow.
