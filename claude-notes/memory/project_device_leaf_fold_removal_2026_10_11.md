---
name: project-device-leaf-fold-removal-2026-10-11
description: "PROJECT.md item 10, DONE 2026-10-11 both phases -- device-level \"as <leaf>\"/\"::\" grammar fully removed from Go generator, both houses verified byte-identical, item 10 deleted from PROJECT.md."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-11T18:32:05.038Z
---

PROJECT.md item 10 ("remove the '.. as ..' clause for devices... take care of the '::'") is fully
done and removed from PROJECT.md as of 2026-10-11.

**Phase A** (earlier in the same session): mechanically rewrote both houses' real Conceptual.def so
every entity spells out its own full explicit path, instead of relying on the device header's
implicit `device <id> as <leaf> with:` leaf-fold. Verified 0 "as"/"::" occurrences left in either
house's Conceptual.def. Found and fixed several real generator bugs along the way, including a
previously-silent duplicate-Conceptual.def-line bug (two copy-pasted `entity ...;` lines for the
same capability were silently merged with no warning at all) -- fixed with a new real-line-number
comparison check in `RegisterDiscoveryImpliedEntity` (administration.go).

**Phase B** (this continuation): deleted the dead grammar capture and its supporting code from the
Go generator -- `Conceptual_DevicePositioning.go`'s "as <spec>" regex capture groups,
`Conceptual_DeviceEntities.go`'s `isMultiSphereDeviceNamePath`/`parseMultiSphereDeviceNamePath`/
`resolveDeviceNamePathForSphere`/`explicitSphereOfSpec`/`hasDeviceLeafOverride`, main.go's two
one-off migration-tool CLI flags, and the `flatten_conceptual_positions.go`/
`backfill_explicit_node_positioning.go` migration tool files themselves (fully spent).
Deliberately did NOT delete `TDevicePositioningDeclaration.Spec`/`TDevicePositioningOccurrence.
DeviceNamePath`/`deviceSpecLeafPath`/`namingSpacePath`'s core fold -- those are still live,
repurposed by this session's own `applyColocatedPositioning`/`triggerLogicalOwnNodeCapabilities`
mechanisms, which feed a COMPUTED (non-grammar-text) leaf through the exact same machinery.

**The ~51 test failures this surfaced** were almost all mechanical: test DSL fixtures across ~15
files used `device X as Y with:` purely as boilerplate (no longer parses at all once the "as"
capture is gone, since extractDeviceWithBlockHeader's regex now requires `device <id> with:`
verbatim) -- fixed by stripping the dead `as <leaf>` text. A smaller set of tests genuinely
exercised the fold's real behavior (entities relying on the device's own leaf being injected into
their path) and needed their DSL fixtures rewritten to spell out the leaf explicitly inline instead
(e.g. `entity light.social:main/light;` instead of `device X as main/light with: entity
light.social:light;`). A few tests (`TestExtractDeviceWithBlockHeader`'s "as" assertion,
`TestRegisterDevicePositioningRejectsLoneInfrastructuralSpherePrefix`, `TestRegisterDevicePositioningMultiSphereLeafPerEntitySphere`,
`TestDeviceCapabilityStripsEntityOwnTrailingDomainSegment`, the two
`TestExpandForDeviceBareEntityLine{DoubleColonForm,ExplicitLeafOverrideForm}` cases) tested the dead
feature directly and were deleted outright. `Conceptual_DeviceNamePathSugar_test.go` was deleted
wholesale (8 of 9 tests existed solely for the dead fold); its one test of an unrelated live
mechanism (`TestDeferredCapabilityLinkResolvesAgainstItsOwnDeclaredSpace`) was migrated into
`Conceptual_DeviceCapabilityEntities_test.go` first.

**One corrected assumption from an earlier survey**: the survey predicted
`expandForDeviceBareEntityLine`'s colon-splitting logic (Conceptual_DeviceCapabilityEntities.go)
could be simplified now that "::" is dead. That's wrong -- `normalizeEntityFullName` (expander.go)
has a GENERIC "every remaining colon in the path becomes a '/' separator" rule, completely
independent of the device-leaf-fold feature (e.g. `sensor.social:terrace:pressure` still
legitimately means path `terrace/pressure` today, with no device leaf involved at all), so that
colon-splitting logic is still live and was left alone -- only its doc comment (which wrongly tied
it to the removed `hasDeviceLeafOverride` mechanism) was corrected.

Final verification: both houses regenerated via `go install .` + `./generate`, `diff -rq` against
pre-change `hass/`/`coordinator/`/`list.*` snapshots -- byte-identical for both Vienna and
Junglinster.

**Known pre-existing staleness NOT touched**: Architecture.md's "Implied node positioning" section
(~line 1033) still describes `applyImpliedNodePositioning`, a function already fully deleted
*before* this session segment started (not part of this item's own cleanup) -- flagged to the user,
not fixed, since it's a separate, older staleness issue.
