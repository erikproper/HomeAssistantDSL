---
name: junglinster-coordinator-crash-loop-2026-09-22
description: "DONE+FIXED 2026-09-22 - real incident, ~3hr Junglinster coordinator crash loop from synchronous full-manifest JSON persist on every retirement/relay; fixed via debouncing, not the initial async-queue fix alone"
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-22T09:48:40.977Z
---

Real incident, 2026-09-22: Junglinster's coordinator crash-looped roughly every 23s from ~02:03 EDT
until fixed ~05:33 EDT (~3.5 hours), restarting hundreds of times. Found while the user asked to
deploy a small, unrelated button-device addition (2 HUE-tap-style buttons) — deploying triggered a
`sudo systemctl restart coordinator` that exposed the pre-existing loop (it had already started
hours earlier, unrelated to that deploy).

**Symptom**: `error: subscribing to homeassistant_instances/ha2mqtt/inquire/reply: timed out` (or
`.../+/bridge/device/+/state`) — a `client.Subscribe(...)` call's `WaitTimeout(10*time.Second)`
never got its SUBACK, and `main()` treats any subscribe failure as fatal (`os.Exit(1)`).
Vienna's own coordinator was unaffected (stable, 2 restarts in 9+ hours) — the difference is scale:
Junglinster relays ~983 Zigbee2MQTT passthrough devices vs. Vienna's much smaller set.

**Root cause, found via CPU profiling (not guessing)**: `TDiscoveryPublisher.persist()`
(discoverycleanup.go) and `TPassthroughDeviceTracker.persist()` (discovery_passthrough_devices.go)
each `json.MarshalIndent` the WHOLE growing manifest and write it to disk — and were being called
**synchronously on nearly every individual topic retirement/relay** during the startup retained-
backlog replay (`RetireOne`, `RetireMissing`'s sweep, `PublishPassthrough`, `Publish`,
`TPassthroughDeviceTracker.Record`). With ~1000+ retirements/relays in a single startup (backlog
built up from this whole session's renames), a CPU profile showed `json.MarshalIndent` as 76% of a
20s window's CPU — enough to starve the process's OTHER goroutines (including the one waiting on
the crashing Subscribe's SUBACK) via Go's own scheduler, even though none of this work was on the
paho MQTT client's own message-dispatch goroutine.

**Two rounds of fix were needed, not one**:
1. First fix (necessary but insufficient alone): routed `watchForOrphanedDiscoveryTopics`'s
   retirements and `RetireMissing`'s startup sweep through the existing throttled `discoveryRelayJobs`
   queue (the same mechanism [[project_junglinster_z2m_discovery_topic_change]]'s own 2026-09-14
   incident introduced for `discoverybridge.go`'s handler) instead of calling `publisher.RetireOne`
   synchronously inline. This stopped the RETIRE/RELAY work from blocking the paho dispatch
   goroutine directly, but the coordinator kept crashing at essentially the same ~23s mark —
   confirmed via CPU profiling that the volume of full-manifest JSON remarshals was itself the
   dominant cost, not just where it ran.
2. Second fix (the one that actually stopped the crash loop): debounced `persist()` itself in both
   `TDiscoveryPublisher` (500ms) and `TPassthroughDeviceTracker` (reused the existing 30s
   `PassthroughStatusDebounceDelay`, added a parallel `SchedulePersist`/`persistTimer`) — coalescing
   a burst of many calls into one actual disk write. `Record()` no longer persists synchronously at
   all; callers (`discoverybridge.go`) schedule it explicitly alongside the existing
   `ScheduleStatusPublish` call. Confirmed via a second CPU profile: total CPU dropped from 15.28s to
   4.14s over the same 20s window, and the coordinator has stayed up with 0 restarts since.

**Diagnostic technique worth reusing**: when a first evidence-based fix improves but doesn't fully
resolve a crash loop, don't keep guessing from log timestamps — add a temporary `runtime/pprof`
CPU profile (`pprof.StartCPUProfile` + a `time.AfterFunc` to stop+flush it) and, critically, replace
every fatal `os.Exit(1)` in the affected file with a `fatalExit()` wrapper that calls
`pprof.StopCPUProfile()` first — a bare `os.Exit()` skips deferred functions AND `time.AfterFunc`
callbacks entirely, so the profile is otherwise lost on exactly the crash you're trying to capture.
Revert the wrapper/profiling once the real hot path is found (`sed`-reversible if applied via a
blanket find/replace).

**Real-world consequence check**: the coordinator being down for ~3.5 hours did NOT affect live
device state/command exchange (Zigbee2MQTT, HA automations) at all — only discovery/coordination
bookkeeping was stalled. This directly informed
[[project_event_bus_operational_vs_coordinative_roles]]'s reasoning about why a future FHEM-style
state-enforcement component needs a much higher reliability bar than the coordinator itself.
