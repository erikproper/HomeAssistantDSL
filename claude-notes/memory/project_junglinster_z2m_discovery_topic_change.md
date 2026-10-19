---
name: project-junglinster-z2m-discovery-topic-change
description: "Planned 2026-09-18: change Zigbee2MQTT's own MQTT discovery publish topic in Junglinster from 'homeassistant' to 'homeassistant.physical', gated on Vienna's washing_machine absorb deploy checking out live. Needs a rollback plan before executing; user on-site in Junglinster that weekend for hardware recovery if needed."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-18T06:27:57.904Z
---

Stated by the user 2026-09-18, as a same-day sequenced plan (weekly usage was at 80%, resets
Sunday):

1. User (EP) checks Vienna remotely — does the just-deployed [[project_logical_layer_device_combining]]
   "absorb" migration (washing_machine/ROBB plug) actually work live.
2. If yes: Claude deploys the CURRENT generator version to Junglinster too (no Junglinster-specific
   `.def` changes needed for this — `./generate` there was already confirmed byte-identical before/
   after the absorb code changes, since Junglinster has no Logical.def).
3. Then: investigate whether changing Zigbee2MQTT's own MQTT discovery publish topic in Junglinster
   from "homeassistant" (HA's default discovery_prefix) to "homeassistant.physical" would now work
   properly — not yet researched which Z2M config file/setting this is (its own `configuration.yaml`
   `mqtt.discovery_topic` most likely, wherever Z2M's now-containerized config lives on
   protocols-server-1 post-relocation, see [[project_zigbee_mqtt_relocation_2026_09]]), nor exactly
   why "now" specifically (something about the current generator/coordinator state makes this
   newly viable — worth asking the user directly when this step is reached rather than guessing).
4. Document a rollback plan the user can execute themselves (they're physically in Junglinster that
   weekend, so can also intervene with the hardware directly if needed).
5. Execute the topic change.
6. Depending on remaining weekly budget, continue migrating Vienna afterward.

**Why:** presumably continues the "Discovery over YAML templates" direction
([[project_discovery_over_yaml_templates]]) — fully separating Z2M's own raw MQTT discovery
(currently under the shared "homeassistant" prefix, HA's default) from the coordinator's own
relayed/stable discovery configs, so HA never picks up a Z2M-native discovery config directly. Not
confirmed with the user yet — verify this reasoning before acting on it.

**How to apply:** don't start step 3's investigation until the user confirms step 1 passed (their
own check, not something to do on their behalf) and step 2's Junglinster deploy has actually run.
When step 3 starts: locate Z2M's current discovery_topic setting on Junglinster's
protocols-server-1 container, understand what "would now work properly" refers to (ask if unclear
rather than assume), then write the rollback plan as a literal, ready-to-run set of steps (revert
the config value + restart the container/service) BEFORE touching anything live, per the explicit
request. This is real infrastructure work on a running Zigbee mesh — same "follow their lead,
don't pre-empt" discipline as [[project_zigbee_mqtt_relocation_2026_09]].
