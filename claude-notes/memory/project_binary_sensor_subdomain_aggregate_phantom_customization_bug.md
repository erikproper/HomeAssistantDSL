---
name: project-binary-sensor-subdomain-aggregate-phantom-customization-bug
description: "DONE+DEPLOYED 2026-09-18 (both houses): DeriveBinarySensorSubdomainAggregates derived a placeholder for every binary_sensor subdomain, but only door/motion/water/window ever get backed by a real entity -- every other subdomain (windy/sunny/battery_alert/node/consumes/...) got a phantom customization file for an entity_id nothing creates. Fixed, verified live on Vienna and Junglinster."
metadata:
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-18T21:18:56.348Z
---

User reported live: "In Vienna, social_terrace_windy is now not reported as existing" -- right
after the windy/sunny migration ([[project_logical_layer_defined_and_derived_capabilities]]).
Traced with a temporary debug print in `AppendEntityRecord` (administration.go) dumping a stack
trace for any record named "windy" -- found TWO records: the real one
(`registerLogicalDerivedConditionCapability`, long name `binary_sensor.social_terrace_netatmo_windmeter_windy`,
correctly backed by a real template) and a second one from
`DeriveBinarySensorSubdomainAggregates` -> `registerDerivedBinarySensorAggregate`, short name
`binary_sensor.social_terrace_windy`, `Provenance: "derived binary_sensor subdomain aggregate"`.

Root cause: `DeriveBinarySensorSubdomainAggregates` (administration.go) scans every "social/..."
space for binary_sensor entities and registers a synthetic aggregate EntityRecord per subdomain
(any entity's own last path segment) it sees -- with NO filter. But
`generateBinarySensorSubdomainGroups` (generator.go), the function that actually WRITES a real
group entity for such an aggregate, only ever does so for `binarySensorSubdomains = []string{"door",
"motion", "water", "window"}` (generator.go:1422) plus the derived "windoor" union. Every OTHER
subdomain's placeholder record still flows into `generateCustomizationFiles` (which iterates every
`EntityRecord` unconditionally, with no check for whether a real backing entity exists) -- so it
got a `customization/binary_sensor/social/*.yaml` friendly_name/icon stanza for an entity_id no
generation step ever creates. This is NOT a bug introduced this session -- confirmed via a clean
before/after regenerate diff that it ALSO affected "battery_alert"/"node"/"consumes"/"charging"/
"connection"/"link"/"evidence"/"bin_full"/"available" subdomains at every space level, latent since
whenever those subdomain names first started appearing in the DSL. "windy"/"sunny" simply gave the
user a fresh, easy-to-notice symptom (checking a brand-new entity right after migrating it).

**Fix**: `DeriveBinarySensorSubdomainAggregates` now builds a `trackedSubdomains` set from the same
`binarySensorSubdomains` list generator.go's own writer uses, and skips any subdomain not in it
during its collection pass -- one-line filter, no change to the writer side. New test
`TestDeriveBinarySensorSubdomainAggregatesOnlyTracksBackedSubdomains` (administration_test.go).

**How verified isolated**: regenerated Vienna twice (fix reverted vs applied) into snapshotted
copies, `diff -rq` between the two trees -- the ONLY difference is 34 removed phantom
`customization/binary_sensor/social/*.yaml` files; every real, correctly-backed entity
(the windy/sunny long names, the door/motion/water/window/windoor groups, everything else) is
byte-identical. This mattered because `git status` on the SmartLiving repo showed ~822 unrelated
pending changes (this whole session's accumulated, uncommitted work across both DSL and generator
output) -- far too much noise to eyeball a normal `git diff`, so isolate-by-double-regenerate was
the only reliable way to confirm the fix's blast radius.

**How to apply**: full `go build`/`vet`/`test` clean. Deployed to both houses same day: Vienna
first (user confirmed), then Junglinster (also had the same latent phantom-customization files for
battery_alert/node/etc. subdomains -- confirmed removed post-regenerate, its 5 remaining
"*_consumes.yaml" customizations checked and are real template-backed entities, not phantoms).
Both coordinators restarted cleanly, no errors in either journalctl.
