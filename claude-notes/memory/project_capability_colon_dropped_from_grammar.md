---
name: project-capability-colon-dropped-from-grammar
description: "DONE 2026-09-19: the ':' between a capability's own domain.label name and its value/source (Physical.def discovery/hassbridge/commandline, Logical.def) is no longer accepted at all -- whitespace-separated only. Both houses' .def files rewritten, 6 regexes changed, all tests updated. Not yet deployed."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-19T13:19:34.942Z
---

User's own framing (2026-09-19): a capability's own declared name (e.g. `sensor.power`,
`switch.core`) reads exactly like a Conceptual.def entity spec, so a colon immediately after it
visually clashes with that OTHER, unrelated "sphere:path" colon
(normalizeEntityFullName/namingSpacePath). Traced back to Vienna's vacuum device block having a
genuinely inert trailing colon (`sensor.infrastructural:battery_level:  from battery_level;`) --
confirmed via direct test that the trailing colon changes nothing (silently absorbed by
`normalizeEntityFullName`'s own path-trimming), which prompted the user to ask whether the colon
is needed at all for capability declarations generally.

**Three-phase execution, exactly as the user specified** ("make it optional → remove from files →
test → if all works, exclude from grammar"):

1. **Made optional**: 6 regexes changed to `(?::\s*|\s+)` (colon-or-whitespace) --
   `discoveryCapabilityPattern` (integration_discovery_parser.go), `capabilityPattern`/
   `capabilityWithPattern` (integration_hassbridge_parser.go), `commandlineCapabilityPattern`
   (integration_commandline_parser.go), `logicalCapabilityPattern`/
   `logicalCapabilityWithBlockPattern` (integration_logical_parser.go). 4 new regression tests
   proved the colon-less form parsed identically to the colon-having one.
2. **Removed from files**: 687 lines mechanically rewritten across Vienna's Physical.def/
   Logical.def and Junglinster's Physical.def (no Logical.def there yet). Verified byte-identical
   regenerate for both houses -- Junglinster's own diff was initially noisy from the user's
   concurrent unrelated edits (two new tuya devices added mid-task); isolated cleanly by
   reconstructing a "same content, colons reinserted" comparison rather than relying on a stale
   snapshot.
3. **Tested**: full go build/vet/test clean at every step.
4. **Excluded from grammar**: all 6 regexes changed to `\s+` (whitespace only, colon rejected
   outright). Went pattern-by-pattern, `go install` + full test suite after EACH one (per the
   user's own request to stay resumable against a weekly-limit interruption) -- discovery (0
   remaining failures needed fixing beyond the 6 direct ones), hassbridge (19 lines across
   integration_hassbridge_test.go + 1 in the valuemap test), commandline (11 lines +
   1 test), logical (16 lines, quote-prefixed Go string literals needed a variant sed pattern).
   Every "ColonOptional" test from phase 1 was repurposed into a "RejectsOldColonForm" negative
   test (asserts the old colon form now produces an "unrecognised line" warning) rather than left
   as dead/misleading coverage.

**Two real bugs caught only by this final step**, both would have produced syntactically broken
copy-paste suggestions or a silently-dropped capability if missed:

- Three **suggestion-generating functions** (`discovery_passthrough_suggestions.go`,
  `mqtt_discovery_existence.go`'s `buildDiscoverySuggestionReport`, `mqtt_entity_existence.go`'s
  `buildSuggestionReportFromExistence`) all independently built their own "paste this into
  Physical.def" text using the OLD `domain + "." + suffix + ":"` shape -- if left unfixed, every
  future discovery suggestion would have been unparseable the moment a user copy-pasted it. Fixed
  by dropping the `+ ":"` in all three; several test assertions checking for the literal colon
  substring updated to match.
- Vienna's own Physical.def had ONE remaining colon-form line the initial mechanical strip missed:
  `hidden sensor.pressure_raw: 0x00158d0003f0d585_pressure_zigbee2mqtt;` -- the `hidden ` keyword
  PREFIX before `domain.label` meant the strip script's line-start anchor never matched it. Caught
  only by the final full regenerate-diff (a real, previously-declared hallway pressure sensor
  briefly disappeared from generated output before the fix). Fixed directly; confirmed no other
  keyword-prefixed capability lines exist in either house via a broader grep sweep afterward.

**How to apply**: fully done and tested (go build/vet/test clean, `./generate` clean with zero
warnings for both houses, zero remaining colon-form capability declarations anywhere per a final
grep sweep). **NOT yet deployed** -- same "confirm before deploying" discipline as every other
change this session. Deploying needs a coordinator rebuild+redeploy on both houses' own
protocols-server-1 (the parser changes are in the shared `homeassistant` Go module the coordinator
also vendors/builds from) -- check whether the coordinator's own binary actually embeds this
parser code before assuming a redeploy is even necessary; if it doesn't, only the generator
(`./generate` locally) needs the new binary, already confirmed via `go install`.
