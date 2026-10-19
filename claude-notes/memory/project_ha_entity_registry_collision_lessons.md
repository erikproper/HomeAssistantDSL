---
name: project-ha-entity-registry-collision-lessons
description: "Real HA entity_id collision found live 2026-09-10 (Vienna's vacuum) — root cause was a disabled (not deleted) native integration reserving the name, invisible in the normal UI; documents the safe direct-registry-edit procedure, the web-terminal-dies-on-core-stop gotcha, and (2026-10-13) that the SSH add-on has no SFTP subsystem so scp/sftp fail -- use a cat|ssh pipe instead."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-10-13T15:50:39.227Z
---

**Root cause of a persistent `entity_id` collision (`vacuum.social_apartment_living_room` -> `_2`),
found live 2026-09-10**: not an orphaned MQTT ghost from repeated renames (those go to
`core.entity_registry`'s `data.deleted_entities` list and are harmless), but a **disabled** (not
deleted) native `roomba` integration on Green itself, from before the vacuum was migrated to the
MQTT-bridge architecture. A disabled integration's entities stay in `data.entities` (the LIVE list,
not `deleted_entities`) and still reserve their `entity_id`/`unique_id` — completely invisible in
the normal Settings > Entities UI (which only shows entities backed by an active/visible
integration), and blocking any other integration from claiming the same name. Renaming via the UI
fails with no useful error; the real error ("Entity with this ID is already registered") only shows
up in Settings > Logs when something ELSE (e.g. a fresh MQTT discovery) tries to take that name.

**Why:** the user had migrated the vacuum from a native local-network `roomba` integration
(`host: 192.168.178.42`, disabled but never removed) to the new coordinator/MQTT-bridge
architecture (see [[project_hassbridge_command_routing]]) — a common pattern worth checking for
whenever migrating ANY device this way: a merely-*disabled* legacy integration is not equivalent to
removing it, and will silently block the new one from getting a clean name.

**How to apply:**
1. **Diagnosis**: `jq '{live: [.data.entities[] | select(.entity_id | startswith("<prefix>"))],
   deleted: [.data.deleted_entities[] | select(...)]}' core.entity_registry` — split live vs.
   soft-deleted explicitly; don't trust a plain `grep` across the file, it conflates both lists.
   Check each live match's own `platform`/`config_entry_id` — a collision from a genuinely
   different integration (not just an old iteration of the same one) will have a different
   `platform` value and a `unique_id` that doesn't match anything you're managing.
2. Cross-check the suspect `config_entry_id` against `core.config_entries` — `disabled_by: "user"`
   means disabled-not-deleted (the exact trap here), distinct from the entry being genuinely absent.
3. **Direct registry edit procedure** (only when the UI can't do it): `ha core stop` (via SSH, see
   below) -> back up both files -> build edited versions with `jq` into `/tmp` -> verify the `/tmp`
   output looks exactly right -> only then `cp` over the real files -> `ha core start`. Never edit
   in place without this stop/backup/verify/copy discipline on a live production instance.
4. **HAOS access gotcha**: Home Assistant Green's *default* SSH endpoint (typically
   `hassio@<host> -p 1967` or similar) is the Supervisor's own restricted CLI shell (the `ha`
   command), not a real filesystem shell — `login` to escalate is blocked (`must be suid`). The
   "Terminal & SSH"/"Advanced SSH & Web Terminal" **add-on's own web-embedded terminal panel**
   (inside HA's own UI) DOES give real `/config` access, but is served by HA Core itself — so it
   goes dark the instant `ha core stop` takes effect, mid-command. For any procedure involving
   stopping Core, a *separate*, direct SSH channel to the Supervisor (not the in-UI terminal) is
   the only one that survives the stop to let you finish the job and restart it.

   Update (2026-09-23, second live use of this procedure — see
   [[project_junglinster_stale_template_entity_collision_2026_09_23]]): this project's own
   `hassio@junglinster.homelinux.org -p 1967` channel is actually the opposite of the above —
   it's the "Advanced SSH & Web Terminal" ADD-ON's own persistent SSH server (a separate process
   from Core), giving a real filesystem shell with `ha` CLI installed, AND it survives `ha core
   stop` fine. Its own gap: `ha core stop`/`start` fail with "unauthorized: missing or invalid API
   token" by default, since SUPERVISOR_TOKEN isn't in this login shell's environment. Fix: read it
   from the s6-overlay container-environment file directly — `SUPERVISOR_TOKEN=$(cat
   /run/s6/container_environment/SUPERVISOR_TOKEN) ha core stop` (same for `start`/`info`/etc.).
   Both houses' own SSH add-on setups behave this way, not the restricted-Supervisor-shell case
   originally described above.

   Update (2026-10-13, third live use, Junglinster Phase 4 of PROJECT.md's manual-automation
   migration): this SSH add-on channel does NOT run an SFTP subsystem, so `scp`/`sftp` both fail
   silently with "subsystem request failed on channel 0" — including when used for exactly this
   registry-edit procedure's own step 3 (pushing the edited `/tmp` file). Use a plain pipe instead:
   `cat local_file | ssh hassio@<house>.homelinux.org -p 1967 "cat > /tmp/remote_file"`. Over a
   slow/firewalled connection a multi-MB registry file can take minutes this way — don't assume a
   `scp` timeout or a stalled `cat | ssh` pipe means something is broken; check the remote file's
   growing byte count instead of killing and retrying blindly.
