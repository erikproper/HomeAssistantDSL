---
name: project-manual-automation-migration-done-2026-10-13
description: "DONE 2026-10-13: PROJECT.md item 13 (migrate manual automations) complete & removed from PROJECT.md. Both houses' hand-authored HA automations.yaml entries moved into Conceptual.def as a new 'rule' construct, via a real three-layer CDL1-style parser. Full writeup now in Architecture.md §9.3 (design/bugs) and README.md's DSL syntax 'Rules' section (reference)."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-13T15:51:09.978Z
---

**What shipped**: a real three-layer CDL1-style parser (`rule_character_stream.go`/
`rule_tokeniser.go`/`rule_parser.go`) for a new Conceptual.def `rule <spec> with: when ...; if
...; then ...; end;` construct, replacing the old ad hoc `TWhenRule` one-liner entirely. Vienna's
31 and Junglinster's 53 (of 54 — one confirmed-dead duplicate excluded) hand-authored HA
automations, previously living only in each house's own UI-managed `automations.yaml` and never
seen by this project, are now DSL rules that regenerate through the normal pipeline. Both houses'
`automations.yaml` are now backed up and emptied.

**Where the detail lives now** (PROJECT.md stays a TODO list, per
[[feedback_project_md_stays_a_todo]] — this item's full phase-by-phase log was moved out, not kept
there):
- **Architecture.md §9.3** — the design rationale (why a fresh parser rather than extending
  `TWhenRule`), the grammar's structural rules (exactly-one-`when`/at-most-one-`if`/exactly-one-
  `then`), the positioning convention (lowest-common-ancestor space, relative-vs-absolute paths),
  and four real bugs found live along the way (two `EntityRef` parsing bugs, a `genericBodyElement`
  block-boundary miscount on a rule's own `else:` marker, and a jinja single-quote YAML-escaping
  break that put Junglinster's HA core into recovery mode on deploy).
- **README.md's "DSL syntax" → "Rules"** — the actual syntax reference (when/if/then clauses,
  trigger/condition/action verb vocabulary, absolute-path escape hatch).

**Three more dead-target entities found and fixed during migration verification** (same
"redirect to the real entity, confirmed with the user" resolution as each other): Vienna's
`switch.social_apartment_living_room_tv` (→ `..._tv_media`), and on Junglinster:
`switch.social_house_sarah_bedroom_media_media` (→ `switch.social_house_sarah_bedroom_media`) and
`switch.physical_house_storage_room_backup` (→ `switch.social_house_storage_room_backups`, note
the sphere AND pluralisation both wrong in the original).

**How to apply**: if a future automation-migration-shaped task comes up (a new house, or more
automations appear on an existing one), the grammar is already built — read Architecture.md §9.3
and README.md's "Rules" section first rather than re-deriving the design. The registry-collision-
after-deploy pattern this hit on both houses is the same one
[[project_ha_entity_registry_collision_lessons]] already documents — follow that procedure, don't
re-discover it.
