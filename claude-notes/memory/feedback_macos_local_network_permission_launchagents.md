---
name: feedback-macos-local-network-permission-launchagents
description: "A CLI binary launched via a macOS LaunchAgent to connect to a LAN/local-broker address can be silently blocked by macOS's Local Network privacy permission (NECP) after its first success or two -- no error output, just dropped connections. Diagnostic: log show for NECP drops."
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-11T09:52:32.786Z
---

**Real finding (2026-09-11, eriks-mac-studio)**: switched a macOS host's `cpu-report` LaunchAgent
from reporting directly to the cloud broker (`mqtt.erikproper.eu`, external) to reporting to the
house's own local broker (a bare local hostname, e.g. "junglinster") — mirroring how Linux/Pi hosts
already work. The FIRST one or two runs succeeded (real TCP connections completing), then every
subsequent `mosquitto_pub` invocation started failing with exit code 1 and **completely empty**
stdout/stderr log files — no error text anywhere, `launchctl print` just showed `last exit code = 1`.

**Root cause**: macOS's Local Network privacy permission subsystem (NECP). A bare CLI binary
(`mosquitto_pub`, or `bash` calling it) launched via a LaunchAgent has no real `.app` bundle
identity for macOS to associate a granted permission with, so it never gets shown the interactive
"Allow this app to find and connect to devices on your local network?" prompt — it defaults to
silently deny going forward. This only gates connections to LAN/RFC1918 addresses; a connection to
an external host (the cloud broker) is completely unaffected, which is exactly why the OLD,
cloud-targeting job never hit this.

**Why this is hard to diagnose**: the failure is completely silent at the application layer — no
stderr, no distinctive exit code, and (confusingly) the FIRST attempt or two can succeed before
macOS's TCC/NECP subsystem "notices" the new synthetic process identity and starts enforcing the
policy, so a quick manual test right after making the change can look like it worked.

**The diagnostic that cracked it**: `log show --last <N>m --predicate 'eventMessage CONTAINS[c]
"<process-name>" OR eventMessage CONTAINS[c] "local network"' --style compact` on the Mac itself —
look for `tcp drop outgoing ... reason: NECP` lines and a `LocalNetwork: found bundle id
<binary>-<hash> by PID` / `nehelper: Could not find bundle ID or display name for app` pair right
before the drops start. This is unambiguous confirmation, much faster than guessing from empty
logs.

**No remote fix exists**: granting this permission requires an interactive System Settings action
(Privacy & Security → Local Network) on the Mac's own screen — cannot be done over SSH. If a
similarly-affected Mac needs local-broker delivery, it needs someone physically there (or
screen-sharing in) at least once.

**Resolution taken here**: the user decided ALL macOS hosts should report to the cloud broker
directly, not through a house's own local broker — sidesteps the whole class of problem. Applied to
eriks-mac-studio (fresh revert) and eriks-macbook-pro-2 (found running a genuinely-working local
job left dangling from an incomplete 2026-09-08 migration — its own LaunchAgents plist file had
already been deleted, but launchd kept running the job since `launchctl bootout` was never called;
`bootout` + confirm cloud-client-only fixed it). eriks-mac-mini was, per the user, already reporting to the cloud correctly the whole time — the
"no retained data" finding here was just the machine being off, not a real problem (don't assume a
"no retained data" finding means a permission failure without first checking the machine is even
reachable). Still worth checking it for the SAME dangling-dual-job issue macbook-pro-2 had, queued
for whenever it's next reachable. See PROJECT.md item 6 and
[[project_conflicting_device_name_and_cursor_persistence_fixes]]'s sibling notes for the full
writeup.
