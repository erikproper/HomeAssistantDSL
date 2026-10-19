---
name: reference-main-ha-ssh-access
description: "SSH access to both houses' main HA instance (Advanced SSH & Web Terminal add-on), for pulling .storage files or running ha CLI commands."
metadata:
  node_type: memory
  type: reference
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-05T17:37:01.638Z
---

Both houses' main HA instance exposes the "Advanced SSH & Web Terminal" add-on's own SSH server
(a real filesystem shell with `ha` CLI installed, survives `ha core stop` — see
[[project_ha_entity_registry_collision_lessons]]), wrapped in local shell scripts:

- `~/bin/ha.vienna` → `ssh hassio@vienna.homelinux.org -p 1967 $*`
- `~/bin/ha.junglinster` → `ssh hassio@junglinster.homelinux.org -p 1967 $*`

Both use user `hassio`, port 1967, hostname `<house>.homelinux.org`. `SUPERVISOR_TOKEN` isn't in
this login shell's environment — read it from `/run/s6/container_environment/SUPERVISOR_TOKEN`
when a command needs Supervisor auth (e.g. `ha core stop`/`start`).

**How to apply:** use `~/bin/ha.<house> <command>` instead of re-deriving the SSH target each
session — these credentials were previously undocumented in memory despite being used in several
past incidents.
