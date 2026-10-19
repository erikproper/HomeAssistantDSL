---
name: project-junglinster-migration-starting-checklist
description: "Queued 2026-09-19: next session's entry point for Junglinster's own item-4-style migration. Consolidates several loose threads from Vienna's work that should be applied to/watched for on Junglinster too."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-19T08:21:17.887Z
---

Session paused 2026-09-19 pending the user's weekly usage limit reset
([[feedback_budget_conscious_session_management]]); Junglinster's migration is the next task.
Loose threads from this Vienna session that apply there too:

1. **Flat entity-form ban** ([[project_flat_entity_form_ban]]) — Junglinster has 3 frozen bedroom
   "health" lines deliberately kept outside their device's `with:` block; decide how to resolve
   (accept the rename, or design an escape hatch) before the parser-level ban can be implemented in
   either house.

2. **No-rename shorthand retrofit** ([[project_no_rename_entity_shorthand]]) — Vienna's
   Conceptual.def/Logical.def were mechanically rewritten to use `entity <spec>;` (no `from`) and
   `<domain>.<label>;` (absorb blocks) wherever no rename is needed (186 + 5 lines, verified
   byte-identical regenerate). User explicitly deferred Junglinster's own retrofit to ride along
   with its migration rather than doing it standalone now.

3. **"sphere::path" double-colon naming gap fix**
   ([[project_device_leaf_naming_conceptual_mismatch_gap]]) — new grammar form for "space context,
   skip device leaf," deployed on Vienna (wind_speed/wind_direction/windy/sunny/daylight). Watch
   for the same conceptual-name-vs-infrastructural-name mismatch pattern when positioning
   Junglinster's own devices — likely to recur given it's the same underlying "device block acts
   like a space" convention. A third form (`sphere:leaf:path`, explicit replacement leaf) is still
   deferred/unimplemented if a case needs it.

4. **Import rename remote_device_id gotcha**
   ([[project_import_rename_remote_device_id_bug]]) — CRITICAL to check before renaming ANY
   import-kind device's local id on either house: the 2-token `device <id> from <installation>
   with:` shorthand silently ties the remote lookup to the same string. Must switch to the explicit
   3-token form (`device <id> from <installation> <remote-id> with:`) in the SAME edit as any such
   rename, or the device goes dark with no warning. Grep for 2-token vs 3-token import headers
   before starting.

5. **input_number/helper values don't survive an entity rename** (no dedicated memory yet, minor)
   — renaming a YAML-defined helper's entity_id (via Conceptual.def positioning changes) resets its
   live value to default; HA's restore_state/recorder keys by exact entity_id. If Junglinster's
   migration renames any input_number/input_boolean/etc. helpers, capture their current values from
   the recorder DB (`sqlite3 home-assistant_v2.db`) or restore_state cache BEFORE deploying the
   rename, and be ready to re-set them afterward (found live 2026-09-19, Vienna's windy_threshold/
   sunny_threshold both reset to 0 across their renames; recovered via recorder DB history showing
   the stable long-term values: windy=25 km/h, sunny=7000 lux, unchanged since 2026-09-09).

**How to apply:** read [[project_discovery_device_block_and_node_capability]] (the general
migration playbook) first as always, then work through Junglinster's own remaining devices one at
a time per that playbook, applying the above four points as they become relevant rather than
front-loading all of them.
