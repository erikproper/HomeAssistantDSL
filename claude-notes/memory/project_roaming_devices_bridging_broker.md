---
name: project-roaming-devices-bridging-broker
description: "Cloud MQTT broker (mqtt.erikproper.eu): broker itself DONE 2026-08-26 (PROJECT.md 1.2). Coordinator-side multi-broker relay + \"cloud\"/\"local\"/\"import\" DSL routing built + unit-tested same day (code not yet deployed) -- see the 2026-08-26 (continued) update below for the full design."
metadata:
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-08-26T16:35:43.529Z
---

Stated 2026-08-21, as a forward-looking note (not a request to build). Laptops
(`eriks-macbook-pro-2`, `paulas-air-m1` today) physically move between the two houses. Reporting
`cpu` data to whichever house's local MQTT broker they happen to be near is fragile — it would
need reconfiguring, or arbitrarily picking one house, every time the laptop moves.

**The proposed fix:** these devices should report to a shared **bridging MQTT broker** set up for
exactly this purpose, with each house's own coordinator/instance importing the relevant topics
from there instead of owning them locally. Recorded in Architecture.md §12 (Multi-house
federation) as the same federation shape as the existing Netatmo cross-home example (§6.5) — a
device whose "home" isn't fixed needs its data to flow through a broker neither house directly
owns — just for infrastructural monitoring data instead of cloud-sourced semantic data.

**Refined same session:** this also covers laptops away from *both* houses entirely, not just
switching between them — the bridging broker must be reachable from anywhere the laptop has
outbound network access to it (unlike each house's own local broker today, which is LAN-only).
And, half-jokingly but genuinely: whatever host ends up running the bridging broker itself
(presumably cloud-based, given the reachability requirement) is just another `cpu`-type "hosts"
integration device once it exists — no special-casing needed, the same self-monitoring pattern
applies to it too.

**Why:** part of the broader [[project-mqtt-migration-plan]] federation story, but distinct from
it — this is about *infrastructure* devices with no fixed house, not about one house owning a
cloud integration on another's behalf.

**How to apply:** nothing to build yet — no bridging broker exists, and today's laptops report to
Junglinster's local broker only (per Physical.def's current `hosts` integration devices). Don't
design the bridging-broker mechanism speculatively; wait for the user to raise it as active work.
If Vienna's own MQTT broker gets set up before this is designed, flag that roaming devices will
need a decision then (report to Junglinster's broker, Vienna's, or wait for the bridging broker).

**Update (2026-08-23):** the user now says cross-site MQTT bridging should become testable "soon"
via this same cloud-based broker, precisely because they also want mobile devices (e.g. their own
laptop) connected to it. Still not designed, but this is the trigger to watch for — when the user
raises it next, it's likely genuinely ready to design/build, not still speculative. Note the
distinct, still-blocked piece: actually getting Netatmo data flowing through to Vienna specifically
still waits on Vienna's separate local hardware (bridge/router) issue — the cloud broker unblocks
general cross-site testing, not that specific data path.

**Update (2026-08-24):** the trigger fired — user named this "next plumbing," promoted to
PROJECT.md's "Current focus" (superseding Enphase Envoy bridging as the immediate next thing,
though that can proceed in parallel/independently). Design/build starts next session. No design
decisions made yet as of this note — provider, auth, topic namespace, and how each house's
coordinator subscribes/bridges to it are all still open.

**Update (2026-08-26): host provisioned and broker installed, PROJECT.md's step 1.2 now DONE.**
`mqtt.erikproper.eu` is the host (already online when this session started). Design decisions
made and verified live:
- **Broker**: Mosquitto, matching every local house broker already in use — no reason to diverge.
- **TLS**: Let's Encrypt via `certbot certonly --standalone` (port 80 free, no other web server on
  the box). Real gotcha hit and fixed: the `mosquitto` system user can't read
  `/etc/letsencrypt/live/` (root-only, 0700), so a certbot renewal deploy-hook
  (`/etc/letsencrypt/renewal-hooks/deploy/mosquitto.sh`) copies `fullchain.pem`/`privkey.pem` into
  `/etc/mosquitto/certs/`, chowns them to `mosquitto:mosquitto`, and reloads the service — needed
  on every renewal (~90 days), not just the initial cert.
- **Auth**: originally sketched as one login per client (house coordinator + one per leaf device);
  simplified to exactly **two shared accounts** per the user's explicit "don't want to manage too
  many logins" — `coordinator` (broad `topic readwrite #`) and `client` (all leaf devices share
  this one login). Isolation between leaf devices without needing more logins: Mosquitto's ACL
  `pattern readwrite hosts/%c/#` scopes by **client ID**, not username — every device still needs
  its own fixed, unique client ID (`-i <device-name>`) matching its topic prefix, but they can all
  authenticate as the same `client` user. Verified both directions: a device publishing under its
  own client-ID-matching topic prefix is delivered; publishing under a mismatched prefix is
  silently dropped (Mosquitto doesn't error on an ACL-denied QoS 0 publish — worth remembering,
  it looks identical to "nothing happened" from the publisher's own debug log, which cost real
  debugging time before the mismatch was spotted).
- **Relay design confirmed**: coordinator-mediated relay, not native Mosquitto broker-to-broker
  bridging — the user's own reasoning: "we also may need to do some low level (json) renames,"
  which a dumb broker bridge can't do but the coordinator (already doing exactly this kind of
  JSON reshape for discovery refinement elsewhere) naturally can.
- Verified end-to-end (TLS handshake via `openssl s_client`, auth, publish/subscribe, and the
  per-client-ID isolation both allowed and rejected cases) from three places: the server itself,
  `xanadu` (a Junglinster machine), and a MacBook.
- **Real gotcha hit, worth remembering**: `mosquitto_pub`/`mosquitto_sub` need an explicit
  `--cafile` to use TLS at all (silently connects plaintext to 8883 otherwise, which a TLS-only
  listener just won't complete) — and the CA bundle path is OS-specific: Debian/Ubuntu has one at
  `/etc/ssl/certs/ca-certificates.crt`, but macOS has no equivalent file at all (needs one
  exported from the system Keychain via `security find-certificate -a -p
  /System/Library/Keychains/SystemRootCertificates.keychain`, since Homebrew-installed CLI tools
  don't consult the Keychain automatically). This same per-OS handling will need to be built into
  `Integrations/cpu/report` before it can report to this broker over TLS — flagged in PROJECT.md
  under step 1.3 (mobile laptop reporting), not yet built.
- **Also worth remembering**: a real password (`coordinator` account) got pasted in plaintext into
  the chat during testing and had to be treated as burned/rotated immediately — a live reminder to
  watch for this when walking through credential-setup steps interactively.
- **Not yet done** (as of this note, superseded below): the actual coordinator-side relay code, and
  wiring real per-house/per-laptop credentials into Settings.def -- credentials were wired in by
  the user shortly after this note (real values in Settings.def, `mqtt cloud_client:`/`mqtt
  cloud_coordinator:` profiles in Physical.def with `coordinator_only true;` on the latter).

**Update (2026-08-26, continued): coordinator-side multi-broker relay + DSL routing designed and
built, per the user's explicit "we need to fix this properly."** Full design, all code
build/vet/test clean, generator side verified live against real Junglinster (`./configure` clean
with the macbook's real `cloud local` device line) -- **coordinator side not yet deployed/restarted
on protocols-server-1 as of this note** (user said "will transform and deploy now").

- **Trigger**: user added `device host.eriks-macbook-pro-2 eriks-macbook-pro-2 cpu cloud;` to real
  Physical.def, exposing that the coordinator had zero multi-broker MQTT support at all (one
  `TMQTTBrokerSecrets`, one client, `mqtt.go`'s own header comment explicitly said "the coordinator
  never bridges or republishes device state itself"). Scoped via AskUserQuestion; user chose full
  proper support, not a documentation-only stopgap.
- **Semantics (from the user directly, mid-build)**: "cloud" means this declaration's
  communication happens over the cloud broker instead of main, and by default is **not** mirrored
  onto the local/main bus at all -- only "cloud local" together also mirrors it locally. And,
  separately: **"cloud"/"local" should be usable on any entity specification in any of the
  integrations"** -- tonight's build only wires this into "integration hosts" device lines (the
  real, blocking need); generalizing to per-capability-lines in `home_assistant`/`discovery`
  integrations is an explicit, scoped-down follow-up, not done. Also: **"the cloud based MQTT
  broker has always been about precisely that cross-installation device sharing... we're just
  going there in small steps"** -- i.e. this is the long-standing federation goal
  ([[project-mqtt-migration-plan]]), not scope creep.
- **New DSL pieces** (`homeassistant/` generator):
  - `mqtt_routing_keywords.go`: `parseRoutingKeywords(line)` strips trailing `cloud`/`local`
    tokens from any `;`- or `with:`-terminated line -- a small, reusable, integration-agnostic
    helper (deliberately factored out so extending routing keywords to other integrations later
    doesn't need new parsing machinery, just a call site). Wired into `integration hosts`'s device
    line grammar (`THostDevice.Cloud`/`.Local` fields).
  - `integration_import_parser.go`: new `integration import with: device <local-id>
    <remote-installation> <remote-host> <type>; end;` grammar -- pulls in a device another
    installation published as "cloud" under its own name. Reuses `THostDevice` (new
    `ImportedFrom` field) and the *entire* existing hosts materialization/discovery pipeline
    rather than inventing a separate model -- an import is functionally "a device whose data
    arrives via cloud-relay instead of a local report script." Pre-collected into
    `TPhysicalGenerationContext.ImportedDevices` before the generic per-block dispatch loop runs
    (`generatePhysicalIntegrationOutputs`), mirroring how hassBridge/instances are already
    pre-collected, so "hosts" can merge them into its own `devices.yaml` output regardless of
    block order. Excluded from `ping/hosts` and reporting automations (never on this house's own
    LAN, carries no local capabilities) -- naturally, by construction, not special-cased.
  - `${installation}` (new `resolveInstallationName`, defined.go) threads through
    `TPhysicalGenerationContext.Installation` into `coordinator/devices.yaml` (top-level
    `installation:` field) and `cpu/secrets.<cloud-profile>` (`installation=` line, omitted for
    "main" -- cpu/report's own presence-check, not just an empty value, decides whether to qualify
    its topics).
  - `generateCoordinatorSecretsFile` now writes every `coordinator_only true;` broker profile
    (e.g. `cloud_coordinator`) under a new `brokers:` map in `coordinator/secrets.yaml`, alongside
    the existing `mqtt:` (main) block.
- **Topic-qualification design (the one real bug caught mid-build, worth remembering)**: a
  cloud-routed device's state/device-info topic is qualified as `hosts/<host>/<installation>/...`
  -- installation inserted as a segment **after** the hostname, not as an overall prefix. An
  overall `<installation>/hosts/...` prefix would silently fall outside the existing Mosquitto ACL
  (`pattern readwrite hosts/%c/#`, %c = the report script's own `-i $hostname` client ID) and get
  dropped with zero error, exactly the "silent ACL drop" gotcha from the initial broker setup
  above. Both `Integrations/cpu/report` (script-side topic construction) and the coordinator's
  `qualifyHostsTopic` (`mqtt_relay.go`) implement this identically -- they must never drift apart.
  Discovery-topic qualification (a *different* namespace, `homeassistant/...`, published by the
  broad-access `cloud_coordinator` account, not the restricted `client` account) has no such ACL
  constraint and stays a plain `<installation>/` prefix.
- **Coordinator routing table** (`house_event_bus_coordinator/mqtt_relay.go`,
  `deviceRoutingPlan`/`needsCloudRelay`): plain local -> main only (unchanged). `cloud` only ->
  discovery to cloud only, no relay, never visible locally. `cloud`+`local` -> discovery to both;
  `relayCloudDevice` bridges its cloud-qualified state/node/device-info traffic onto main's own
  bare topics, so local HA sees it exactly like a normal local report (this is effectively "import
  from yourself"). `ImportedFrom` -> discovery to main only; same relay mechanism, qualifier =
  the remote installation's name instead of this house's own.
- **Second broker connection**: `secrets.Brokers` (new map in the coordinator's own
  `TMQTTSecretsFile`) is connected as one additional TLS-aware `mqtt.Client` in `main.go`. Only
  ONE secondary broker is supported today (warns and connects to neither if more than one
  `coordinator_only` profile exists) -- matches tonight's real need, not a hard architectural
  limit.
- **Cloud-broker discovery cleanup** (added after the user explicitly asked for it): mirrors
  main's own `RetireMissing`/`watchForOrphanedDiscoveryTopics`, scoped to exactly this
  installation's own cloud-side topics via a compound `<installation>/<conceptualPrefix>`
  discovery-topic prefix -- required generalizing `topicNodeID` from "first path segment must
  equal prefix" to "topic must start with prefix as a literal string," which also fixes it for a
  multi-segment prefix without changing behaviour for the existing single-segment case. Verified
  (new test) that a sibling installation's own qualified topics on the same shared broker never
  match this coordinator's own subscribe filter -- each installation's coordinator only ever
  cleans up after itself. Also fixed an adjacent latent bug caught while doing this:
  `expectedHostsPayloads` (main broker) was counting a `cloud`-only (not `local`) device's topic as
  "expected" even though it's never published to main, which would have permanently suppressed
  cleanup of a stale main-broker entry for any device later flipped from local to cloud-only.
- **`TDiscoveryPublisher`'s on-disk manifest is now broker-scoped**
  (`manifestKey(broker, topic)`, `discoverycleanup.go`) -- otherwise a `cloud`+`local` device's
  discovery config, legitimately published to both brokers under the same topic string, would
  look like an already-done repeat on the second broker and get silently skipped there.
- **Real Physical.def**: `device host.eriks-macbook-pro-2 eriks-macbook-pro-2 cpu cloud local;`
  (the earlier bare `cloud` token described in an earlier summary turned out to not actually be in
  the live file -- re-added, with `local` too, since the whole point of the laptop's cpu-reporting
  work was getting its data into Junglinster's own HA).
- **Not yet done**: nothing deployed (coordinator binary not rebuilt/restarted on
  protocols-server-1, `cpu/report` not redeployed to real hosts via `deploy.d/03_deploy.hosts`);
  "cloud"/"local" not yet generalized beyond hosts-integration device lines; no real
  cross-installation `import` scenario exists yet to verify end-to-end (Vienna declares nothing);
  Netatmo-in-Vienna specifically still needs its own design once Vienna actually has something to
  export. User said "will transform and deploy now" -- next session should check whether that
  landed cleanly before assuming any of this is live.

**Update (2026-08-26, live debugging after first deploy): availability inversion -- ping-based
liveness replaced by a coordinator-owned timeout, for "cloud"-tagged hosts devices.** Found live:
`sensor.infrastructural_eriks_macbook_pro_2_cpu_temperature` never showed in HA despite real data
flowing correctly through the relay (confirmed via `mosquitto_sub` on xanadu) -- root cause was
`availability_topic` pointing at `hosts/<host>/node/state`, which only `Integrations/ping/report`
(a centrally-run LAN ping, `getent hosts` + ICMP) ever wrote, and which can't reach a roaming
device. The user's own framing: **"the entire logic should be inverted... cpu + temperature get
their (un)availability via liveness + timeout, node/state follows that topic as well"** -- i.e.
traffic arrival becomes the primary signal and node/state becomes a computed reflection of it,
not an independent ping-derived signal others merely depend on.
- **`TCloudLivenessTracker`** (new, `house_event_bus_coordinator/mqtt_relay.go`): `Touch()` marks
  a device alive and publishes `"true"` to its `node/state` immediately, called on every relayed
  cpu/device-info message. A background sweep (30s interval) publishes `"false"` once
  3 minutes (`cloudDeviceStaleAfter`) pass with no Touch -- published once per staleness episode,
  not repeated. `relayCloudDevice` no longer subscribes to the cloud-side `NodeTopic` at all
  (nothing publishes there any more) -- `node/state` is computed, not relayed.
  `generatePingHostsFile` (homeassistant generator) excludes every `Cloud`-tagged device from
  `ping/hosts` entirely, so nothing else can fight the timeout on the same retained topic.
  Deliberate, accepted limitation: this is self-reported-arrival-based, not LWT -- a `mosquitto_pub`
  short-lived script can't use MQTT's LWT, so if a device truly dies mid-report the topic just
  goes stale at whatever it last was until the 3-minute sweep catches it (same "goes stale, no
  active offline declaration" character as `cpu/state` itself already has).
- Also found + fixed live: `host.mqtt`'s (the broker's own self-monitoring device) Physical.def
  `HostName` was the FQDN `mqtt.erikproper.eu` (needed for ping reachability) but `cpu/report`
  names its own topic from `hostname -s` (`mqtt`) -- a real, pre-existing conflict between "ping
  target address" and "report-topic host key" sharing one field, only consequential now that the
  relay actually needs the two to match. Fixed by changing the DSL declaration to the short name
  (`device host.mqtt mqtt cpu cloud local;`) per the user's own choice -- worth remembering if
  another host ever has a FQDN-vs-short-hostname mismatch like this.
- Also found + cleared live: four stale **retained** MQTT messages
  (`hosts/mqtt/cpu|device/state`, `hosts/eriks-macbook-pro-2/cpu|device/state` -- the bare,
  unqualified forms) sitting on the cloud broker from before installation-qualification existed.
  Retained messages persist until explicitly cleared (empty payload + `-r`) or overwritten --
  worth remembering as a recurring gotcha class whenever a topic-naming convention changes on a
  broker that's been in use.
- Both `host.eriks-macbook-pro-2` and `host.mqtt` now carry `cloud local;` in real Physical.def.
