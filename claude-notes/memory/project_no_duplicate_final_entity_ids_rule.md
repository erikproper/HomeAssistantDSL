---
name: project-no-duplicate-final-entity-ids-rule
description: "DONE + DEPLOYED (2026-09-16): validateNoDuplicateFinalEntityIDs catches two DSL declarations that sanitize to the identical final HA entity_id, even when their own DSL-level names differ (e.g. 'fritz_box' vs 'fritz.box') or live in different spaces -- a gap no earlier check covered. Read before touching entity-name/device-id validation rules."
metadata:
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-16T13:31:53.029Z
---

## What this is

Real bug found live 2026-09-16 (Vienna): `binary_sensor.infrastructural_apartment_living_room_fritz_box_node`
reported "unavailable" while visibly updating -- traced to Vienna's Physical.def declaring the
Fritz!Box as two entirely separate devices (`node.fritz_box`, a `home_assistant` bridge device, and
`node.fritz.box`, a `hosts`/ping device), each independently positioned in Conceptual.def. Their own
DSL-level names differ by one character (`fritz_box` vs `fritz.box`), but `toHomeAssistantEntityID`
sanitizes both `.` and `_` down to the same `_`, so the two resolve to the IDENTICAL final HA
entity_id -- HA silently gave the second one a `_2`-suffixed duplicate, forever, with no error from
the generator at all. The user's own words: **"That should have resulted in an error/warning."**

Confirmed as a genuine, general gap: `validateNoConflictingDeviceNames`
(`physical_rule_no_conflicting_device_names.go`) only ever compares DEVICE ids, never their
downstream entity names; `AppendEntityRecord`'s own dedup (`administration.go`) only refuses a
second identical NAME within one space -- neither can see "different names, same sanitized id"
(this case) or even "same name, different spaces" (a second, equally silent gap it also closes).

## The fix (`homeassistant/conceptual_rule_no_duplicate_final_entity_ids.go`, new file)

`validateNoDuplicateFinalEntityIDs(admin *TAdministrationState) []string` runs ONCE, at the very
end of `parseAdministrationFromPaths` (`generator.go`), after every kind's own
positioning/capability registration has fully completed. Scans `admin.EntityRecordsBySpace` (every
entity record this run produced, across every space), groups by `toHomeAssistantEntityID(rec.Name)`,
and warns whenever 2+ records collide on that sanitized id -- deliberately keyed on the SANITIZED
id, not the raw name, since that's the one property no earlier, kind-specific check compares. Each
warning names every colliding declaration's own provenance for direct actionability.

Covers BOTH halves of the gap in one pass: different-name-same-id (the fritz_box/fritz.box case)
AND identical-name-different-space (an entity's own full name is meant to be globally unique across
the whole house, so two occurrences anywhere is always a mistake, even in two unrelated spaces).

3 unit tests (`conceptual_rule_no_duplicate_final_entity_ids_test.go`), including a direct
regression test reproducing the exact fritz_box/fritz.box collision. Confirmed firing correctly
against Vienna's real content before the fix ([[project_logical_layer_dependency_on]]'s own
"Second deployed case" section), confirmed silent afterwards.
