---
name: project-logical-node-capability-rule-2026-10-05
description: "DONE+DEPLOYED 2026-10-05: Rule 1 (every device needs a 'node' capability) extended to the logical layer -- found live via Junglinster's fan.house_wc absorbing a capability from another device without that device's own node, generating silently with no warning. New validateLogicalNodeCapabilityRequired, logical_rule_node_capability.go."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-05T19:49:24.669Z
---

**Found live** (the user, reading Junglinster's own Logical.def): `fan.house_wc` absorbed `fan.fan`
from `switches.garage_fuse_cabinet_zwave_058` but declared no `node` capability of its own, and
generation succeeded completely silently — no warning anywhere, despite the device having no
liveness/availability entity at all. The user's own ask, in order: "I first want to see an error
message" (before fixing it), then the fix itself: "the logical solution is binary_sensor.node as
one of the inherited capabilities from switches...".

**Root cause**: [[project_stable_entity_identity_design]]'s neighbour,
`physical_rule_node_capability.go`'s `validateNodeCapabilityRequired` (PROJECT.md item 7's "Rule
1"), only ever scans `hassBridgeDevicesByID`/`importedDevicesByID` — the two PHYSICAL-layer kinds
that must declare their own `node`. It never looks at Logical.def devices at all, so a genuinely
NEW logical-only device identity (one with no physical-layer counterpart under the same id) had
nothing checking it.

**Fix, two parts**:
1. **The actual DSL fix** (Junglinster's `Logical.def`/`Conceptual.def`, `fan.house_wc`): confirmed
   the absorb mechanism already supported this with zero code changes — adding `binary_sensor.
   node;` to the existing `capabilities from switches.garage_fuse_cabinet_zwave_058: fan.fan; ...`
   block, plus `entity binary_sensor.infrastructural:node;` in Conceptual.def's positioning of the
   device, correctly produces a working `binary_sensor.infrastructural_house_wc_node` template
   sensor checking `fan.social_house_wc`'s own availability. (First debugging detour: checked
   `discovery.yaml` for the new entity and found nothing — wrong file. A `derived binary_sensor.
   node` capability becomes a TEMPLATE-platform HA entity under `hass/<house>/entities/template/`,
   never an MQTT `discovery.yaml` block, same as every other `derived`/`node` capability in this
   codebase. Used `homeassistant -would_define=<entity_id> Definitions/Main.def` to confirm the
   generator's own internal bookkeeping believed it was defined, which is what pointed at the
   right output location.)
2. **The new validation rule**: `logical_rule_node_capability.go`'s
   `validateLogicalNodeCapabilityRequired` — fires when a Logical.def device (a) declares at least
   one capability of its own, (b) has none named "node", and (c) names an id that NO
   physical-layer collection (hosts/discovery/hassbridge/import/commandline/local) already
   declares. Deliberately NOT "every logical device needs its own node" — audited both houses'
   real Logical.def content first (this codebase's own established discipline before adding a hard
   rule, matching Rule 1's own "confirmed zero real violations" precedent): every OTHER
   capability-bearing logical device in both houses shares its id with an existing physical-layer
   device (an EXTENSION, per `integration_logical_storage.go`'s own header comment) that already
   owns its own liveness story — `fan.house_wc` was the only genuine, brand-new-identity violation
   in either house. A logical device with an empty `Capabilities` map (the plain "device X with
   dependency on Y;" sugar, nothing else of its own) is also skipped — nothing of its own for
   "node" to gate. Warning-only (not a hard abort), matching every other hard-enforcement rule in
   this codebase; printed under `[logical]`, not `[physical]`, so the DSL author is pointed at the
   right file. Wired into `generator.go` right after Rule 1's own existing check. Unit test:
   `logical_rule_node_capability_test.go`, covering the missing/compliant/extends-physical/
   dependency-only-sugar cases. Confirmed the warning fires correctly on the ORIGINAL (unfixed)
   Logical.def text before restoring the real fix, and that both houses generate completely clean
   with the fix + rule both in place (zero regressions, full test suite green).

**A live mid-session hazard, worth remembering**: while testing (temporarily reverting
`Logical.def`/`Conceptual.def` to reproduce the original bug, then restoring), the user mentioned
mid-turn they were ALSO editing `Conceptual.def` concurrently via their own editor. Checked via
`diff` against a pre-test backup immediately — confirmed no concurrent edit was clobbered (the
only difference was my own intentional test revert) — then restored via fresh `Read` + minimal
`Edit` (not a blind full-file overwrite) to minimize the window for a real collision next time.
General lesson: when doing a destructive temporary revert-then-restore on a `.def` file the user
might have open, re-`Read` immediately before restoring rather than trusting an in-memory/backup
copy, and prefer small `Edit` calls over whole-file rewrites whenever the user might be editing the
same file concurrently.

**How to apply**: if a future "why didn't this error" report turns up for a Logical.def construct,
check first whether it's a genuinely new device identity (no physical-layer counterpart) — Rule 1
itself still only covers hassbridge/import PHYSICAL devices directly; this logical-layer rule is
the one place a brand-new Logical.def-only device gets checked for "node" at all.
