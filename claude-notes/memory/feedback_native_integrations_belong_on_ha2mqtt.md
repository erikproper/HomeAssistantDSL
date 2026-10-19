---
name: feedback-native-integrations-belong-on-ha2mqtt
description: "Native/built-in HA integrations (e.g. the 'sun' integration) should run on the ha_mqtt/ha2mqtt secondary instance, not main — found live 2026-10-05 as the root cause of a sun_solar_elevation naming collision on both houses."
metadata:
  node_type: memory
  type: feedback
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-05T19:26:03.074Z
---

Found during the 2026-10-05 registry orphan audit ([[project_registry_orphan_audit_2026_10_05]]):
`sensor.social_sun_solar_elevation` collided with its own hassbridge-imported echo
(`..._2`) on Junglinster because the native `sun` integration was still active on **main** HA.
Same latent issue found dormant on Vienna's main instance too (not yet fixed there as of this
session). The user's own stated rule, given while fixing Junglinster's case: **such
[native/built-in] integrations should be on `ha_mqtt`/`ha2mqtt`**, not main.

**Why**: main HA's job is to be the thin, generator-authored, mostly-MQTT-driven instance; native
integrations (sun, native Zigbee/Z-Wave hubs when not yet migrated, etc.) belong on the secondary
`ha2mqtt` instance, which then re-publishes their values back via hassbridge for main (and other
houses) to import under the project's own conceptual naming. A native integration left running on
main duplicates/collides with that same value's own hassbridge-imported echo under a name both
want to claim.

**How to apply**: when auditing or debugging an entity-naming collision between a `_2`-suffixed
hassbridge import and its clean-named sibling, check whether the sibling's actual source is a
native integration still enabled on **main** instead of `ha2mqtt` — that's the structural fix
(disable/remove the integration from main, let the `ha2mqtt`-sourced import take the name), not a
registry-row deletion. Worth a standing check across both houses' main instances for other native
integrations that should have been moved to `ha2mqtt` and weren't.
