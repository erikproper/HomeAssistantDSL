# Machine setup — Home Assistant DSL project

What this Mac has that a new machine would need to reproduce to keep working on this project.
Written 2026-10-19 as part of a machine handover (this Mac going in for service). No keys,
tokens, or passwords are included anywhere below.

## SSH: real config vs. the house's own wrapper scripts

`~/.ssh/config` itself is minimal:

```
Host *
  HostKeyAlgorithms=+ssh-rsa

Host junglinster.homelinux.org
  ProxyJump erikp@mqtt.erikproper.eu:80
```

The `ProxyJump` block is flagged **TEMPORARY** by its own comment (added 2026-09-25, to route
around a restrictive WiFi that blocked Junglinster's non-standard SSH ports but allowed outbound
80) — it worked fine being left in place this whole session via direct SSH, so it may already be
safe to remove; not touched here, just flagging it as possibly stale.

The REAL per-house shortcuts live as small wrapper scripts in `~/bin/` (not SSH `Host` aliases).
The ones this project's own work actually uses:

| Script | Target | Notes |
|---|---|---|
| `frame.vienna` | `erikp@vienna.homelinux.org -p 1968` | Picture-frame Pi (`frame.vienna`) |
| `frame.junglinster` | `erikp@junglinster.homelinux.org -p 1968` | Picture-frame Pi (Junglinster) |
| `ha.vienna` | `hassio@vienna.homelinux.org -p 1967` | Main HAOS instance, Vienna |
| `ha.junglinster` | `hassio@junglinster.homelinux.org -p 1967` | Main HAOS instance, Junglinster |
| `protocols-server-1.vienna` | `erikp@vienna.homelinux.org -p 1970` | p-s-1 (Pi4, now Ubuntu); also opens local-port forwards `8124→:8123` (HA), `8100→:8099` (zigbee2mqtt) |
| `protocols-server-1.junglinster` | `erikp@junglinster.homelinux.org -p 1970` | p-s-1; local-port forwards `8123→protocols-server-1:8123`, `8080→junglinster:8099`, `8091→protocols-server-1:8091` (Z-Wave JS) |
| `protocols-server-2.junglinster` | `erikp@junglinster.homelinux.org -p 1971` | p-s-2 (retired, kept for FS20) |
| `xanadu` | `erikp@junglinster.homelinux.org -p 1969` | Xanadu (backup/Plex box), reached via Junglinster's own port-forward — it's physically there |

Also used this session, not house-specific wrappers but worth knowing: `mqtt.erikproper.eu`
(plain `ssh erikp@mqtt.erikproper.eu`, default port 22) — off-box backup target for both houses'
`backup-home-automation` script, and the ProxyJump host above.

Every house box (`erikp` user) has **full passwordless `sudo`** (`NOPASSWD: ALL`) — confirmed on
Vienna's p-s-1 and its frame; almost certainly the same convention everywhere else. The main HAOS
instances (`hassio` user, port 1967) are a separate, more restricted account — see
`claude-notes/memory/project_ha_entity_registry_collision_lessons.md`'s "HAOS SSH is restricted"
note; some operations there need the HA Supervisor CLI (`ha ...`) rather than plain file access.

## Tools installed outside Homebrew

- **Go toolchain**: `/usr/local/go/bin/go` (go1.25.0, darwin/arm64) — the **official** Go
  install, not Homebrew's. `GOPATH=/Users/erikproper/go`, so `go install .` from
  `homeassistant/` puts the `homeassistant` binary at `~/go/bin/homeassistant` — both
  `/usr/local/go/bin` and `~/go/bin` must be on `PATH` for `./generate` (which just runs
  `homeassistant Definitions/Main.def`) to find the right binary at all.
- `rsync`/`ssh` are the system-provided ones (`/usr/bin`), from Xcode Command Line Tools
  (`xcode-select -p` → `/Library/Developer/CommandLineTools`) — present on any Mac with CLT
  installed, nothing extra to do.
- `mosquitto_pub`/`mosquitto_sub` (used by `./deploy`'s `PublishMetaReload` and by live
  debugging this session) ARE from Homebrew (`/opt/homebrew/bin/mosquitto_pub`) — a plain
  `brew install mosquitto` covers it.
- **PyYAML** for the system `python3` (Apple's Command Line Tools Python 3.9): `python3 -m pip
  install --user pyyaml` (installed 2026-10-21 on the new MacBook Pro, `pro1`) -- Claude's own ad hoc
  analysis scripts read the generated `coordinator/*.yaml` files with it.
- This Mac also has MacPorts (`/opt/local/bin`), XQuartz (`/opt/X11/bin`), and MacTeX
  (`/Library/TeX/texbin`) on `PATH` — confirmed NOT needed for anything in this project's own
  generate/deploy/build pipeline; general personal-machine tooling, not reproduced here.

## Absolute paths the project's own scripts depend on

- `/Users/erikproper/Nextcloud/Systems/go/src/homeassistant` — this repo itself (the
  `homeassistant` Go module + `house_event_bus_coordinator`), Nextcloud-synced.
- `/Users/erikproper/SmartLiving` is a **symlink** into
  `/Users/erikproper/Nextcloud/Systems/SmartLiving` — confirmed via `file`. Both houses'
  `Definitions/`, generated `hass/<house>/` output, and the `generate`/`deploy`/`deploy.d/*`
  scripts live under here (`SmartLiving/Vienna/...`, `SmartLiving/Junglinster/...`), fully
  Nextcloud-synced along with the repo above.
- `04_deploy.coordinator` (in each house's own `deploy.d/`) hardcodes
  `COORD_SRC=/Users/erikproper/Nextcloud/Systems/go/src/homeassistant` — the whole repo gets
  rsynced from that exact local path to the remote house's own `~/go/src/homeassistant`, then
  built there with whatever Go toolchain that remote box has (its own `/usr/bin/go` on
  Fedora, the official `golang-go` apt package on the new Ubuntu boxes — not cross-compiled
  and shipped as a binary).
- `~/go/bin/homeassistant` — see "Go toolchain" above; `./generate` assumes this exact binary
  name is on `PATH`, built fresh via `go install .` from `homeassistant/` before trusting any
  real `./generate` run (this session's own repeated convention, not optional).

## What this file deliberately does NOT cover

Full per-house remote-side setup (podman quadlets, coordinator config, Samba, smtpproxy, etc.)
is documented in `claude-notes/memory/project_vienna_ps1_fedora_to_ubuntu_migration_done_2026_10_18.md`
and `README.md`'s Operational notes — this file is specifically about reproducing THIS Mac's own
local development environment, not the remote house infrastructure.
