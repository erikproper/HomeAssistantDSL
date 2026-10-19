---
name: project-discovery-over-yaml-templates
description: "User prefers MQTT Discovery (via the coordinator) over generator-authored YAML MQTT template sensors for representing bus-sourced entities in Home Assistant — refined 2026-10-17: not just preference, structurally required for device metadata."
metadata:
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-10-17T09:35:44.384Z
---

For turning MQTT-published data (device telemetry, Z2M/Z-Wave state, etc.) into Home Assistant
entities, the user prefers MQTT Discovery — driven by the coordinator (see
[[project-new-architecture-plan]], Architecture.md §6.1/§6.2) — over the generator statically
authoring YAML `mqtt: sensor:`-style template sensors.

**Why (refined 2026-10-17, verified precisely via WebFetch against HA's own docs):** two distinct
reasons, confirmed with the user, not to conflate —
1. **Metadata is structurally undiscoverable any other way.** Both discovery and template-style
   `value_template`/`value_json.*` extraction can pull *state* out of a live payload — that part is
   not special to discovery. But an entity's `device:` metadata block (manufacturer, model, `device`
   grouping, sw/hw version) is never templated from a live payload in *either* mechanism — it's only
   ever set once, at config-publish time. A static/template sensor therefore has no mechanism to
   carry that metadata at all (not just a staleness risk) unless the generator hand-reconstructs and
   republishes it itself. Discovery sidesteps this because Zigbee2MQTT/Z-Wave JS already know and
   supply it automatically from their own device database; the coordinator's refinement role just
   corrects/repositions it (Architecture.md §6.1).
2. **Discovery surfaces devices before anyone's dealt with them.** A device publishing raw discovery
   becomes visible to the model (and the "undeclared entity"/suggestion-list reports,
   [[project_undeclared_entity_audit_technique]]) the moment it exists on the bus — a hand-authored
   template sensor only exists once someone already decided to write it, so it can't play this
   advertising role.

Architecture.md §6.1 documents both reasons explicitly (updated 2026-08-19, refined 2026-10-17).

**How to apply:** when a new physical-layer/coordinator feature needs to expose MQTT-sourced state
as an HA entity, default to the discovery path (coordinator-published discovery config) rather than
proposing or generating a static YAML MQTT template sensor — even as a "quick" stopgap — unless the
user explicitly asks for one. If the entity genuinely has no useful device-identity metadata to
carry (a pure derived/computed value, e.g. a template `binary_sensor` built from another entity's
state — see [[project_entity_existence_inquiry_design]]-adjacent cases), a template is fine; the
hard requirement is specifically about carrying *device* metadata, not a blanket ban on templates.
Note the coordinator doesn't speak MQTT yet in every integration (scaffold only in places, per
[[project-new-architecture-plan]]), so this is a design preference to hold to once each integration
is built, not something to force through everywhere today.
