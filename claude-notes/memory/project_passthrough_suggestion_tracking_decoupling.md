---
name: project-passthrough-suggestion-tracking-decoupling
description: "DONE 2026-09-19, needs a coordinator redeploy to take live effect: fixed a real bug where suggestions/discovery.txt's 'undeclared device' feed was wrongly coupled to whether a discovery_passthrough rule existed. ALSO fixed: removing that rule from Physical.def didn't actually turn passthrough off, since the generator only skipped writing discovery_passthrough.yaml on empty, never deleted a stale one. Both fixed, Vienna's own passthrough rule removed; not yet deployed."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-18T20:36:52.831Z
---

User's own framing (2026-09-19), verbatim because it's the right way to think about this: "since
the 'homeassistant' discovery topic tree is 'controlled' by the coordinator to 'feed' HA with the
proper configs, it is actually the 'homeassistant.physical' tree that should feed
suggestions/discovery.txt with potential things to add to Physical.def." Corrected on my own first
instinct, which was to propose building a whole NEW raw-MQTT scanner in the generator -- the user
was right that this was unnecessary: `house_event_bus_coordinator/discoverybridge.go`'s
`subscribeDiscoveryBridge` was ALREADY the scanner, already subscribed unconditionally to
`<physical_prefix>/+/+/+/config` (sees every device regardless of passthrough rules) -- the actual
bug was two conflated conditions downstream of that already-complete subscription:

1. **Coordinator side**: `passthroughDeviceTracker.Record(...)` was nested INSIDE the
   `matchesAnyPassthroughPrefix(...)` check in the `!matched` branch -- so an undeclared device was
   only ever tracked (for suggestion purposes) if it ALSO happened to match an actively-configured
   `discovery_passthrough` relay prefix. Fixed: `Record` now fires unconditionally for ANY
   undeclared device; only the RELAY decision (publish it into HA so it stays visible while
   unmigrated) stays gated on `matchesAnyPassthroughPrefix` -- tracking-for-suggestions and
   relaying-into-HA are genuinely separate concerns that were wrongly conflated.
2. **Generator side**: `generateDiscoverySuggestions` gated its `fetchPassthroughDevices` call on
   `len(passthroughRules) > 0` -- so even a correctly-tracked device would never surface in
   `suggestions/discovery.txt` once zero `discovery_passthrough` rules were declared (a completely
   normal state; passthrough is an opt-in gradual-migration aid, not a prerequisite). Fixed: the
   fetch now always runs once the function's own top-of-function guard passes (which already
   guarantees `${mqtt_discovery_physical}` is genuinely in use).

Both sides now unit-tested (`TestSubscribeDiscoveryBridgeRecordsUndeclaredDeviceWithoutPassthroughRule`,
`TestGenerateDiscoverySuggestionsIncludesUndeclaredDevicesWithoutPassthroughRules`). Full
`go build`/`vet`/`test` clean across all three packages (`homeassistant`, `house_event_bus_coordinator`,
`mqtt_commandline`).

**How to apply:** the coordinator half of this fix needs a rebuild+redeploy on protocols-server-1
(both houses) before it takes effect live -- purely local code changes don't help until that
happens. Not yet deployed as of 2026-09-19 (this was found and fixed WHILE Vienna's own HA
core/OS update was in progress, so no deploy was attempted this session at all). Check with the
user before deploying, per the usual discipline -- this is exactly the kind of "found a bug, fixed
it, verify it live once it's safe to deploy" situation [[project_meta_reload_all_simplification]]
and others this session already established a pattern for.

Found while investigating Vienna's own item-4 migration completeness (3 real devices + the Z2M
bridge device itself still undeclared, discovered via the coordinator's own
`discovery_passthrough_devices.json` cache) -- see PROJECT.md item 4's own tail-end note for the
full writeup.

**Second, related bug found the SAME day, immediately after**: once the user decided to actually
remove Vienna's `discovery_passthrough "zigbee2mqtt/" from "${mqtt_discovery_physical}";` line
(accepting those 3 devices + the bridge going dark), `generateDiscoveryPassthroughFile`
(`discovery_passthrough.go`) turned out to only ever SKIP writing `coordinator/discovery_passthrough.yaml`
when rules are empty -- never REMOVE a file an earlier run had already written. So removing the
DSL line alone would NOT have actually turned passthrough off; the coordinator would have kept
relaying from the stale file forever, silently. Fixed to `os.Remove` the file when rules go empty
(mirrors `generateDiscoverySuggestions`' own identical "empty report removes the stale file"
convention, found and fixed earlier this same session for `suggestions/discovery.txt`). New test
`TestGenerateDiscoveryPassthroughFileRemovesStaleFileWhenRulesGoEmpty`. Applied + verified live on
Vienna: `coordinator/discovery_passthrough.yaml` confirmed genuinely absent after regenerate;
`coordinator/discovery.yaml`'s own `physical_prefix` line correctly stays (still needed for the
declared gateways' own relay, unrelated to the passthrough rule list). **This part is applied to
Vienna's own `.def` files already** (the `discovery_passthrough` line is gone) -- still not
deployed, same reasoning as above.
