---
name: fedora-migration
description: "smtpproxy moved from a Raspberry Pi (Raspbian) to a Fedora host (protocols-server-1 / vienna.homelinux.org); Fedora's SELinux blocks executing binaries built into $HOME"
metadata: 
  node_type: memory
  type: project
  originSessionId: e843d204-3f45-4981-bcb1-5cbdfebdfdde
  modified: 2026-09-07T20:07:11.707Z
---

smtpproxy was migrated (2026-09-07) from a Raspberry Pi running Raspbian to a Fedora
server, reachable externally as `vienna.homelinux.org` (port 1970 for SSH/rsync — see
[deploy](deploy)) and internally as `protocols-server-1`. The Fedora box is on a
different network than Erik's Mac, so only the external hostname/port resolves from
outside; the internal short hostname does not.

Runs as system user `erikp` (was `User=pi` on the Pi), with paths under
`/home/erikp/...` (binary at `/home/erikp/go/bin/smtpproxy`, certs at
`/home/erikp/ssl/`).

**Why:** Old Pi hardware was being retired; deploy tooling (`deploy`/`do.deploy`,
`smtpproxy.service`) was written years earlier for Raspbian and had no Fedora-specific
handling.

**Key gotcha — SELinux blocks the binary:** Fedora's SELinux (Enforcing by default)
denies systemd (`init_t`) permission to *execute* a binary living in a user's home
directory, because `go install` labels it `user_home_t`. Symptom: systemd reports
`Unable to locate executable '...': Permission denied` / `status=203/EXEC`, even
though the file exists with correct Unix permissions. Confirmed via
`sudo journalctl -b | grep -i avc` showing `avc: denied { execute } ... tcontext=...user_home_t...`.

Fix applied: `sudo semanage fcontext -a -t bin_t '/home/erikp/go/bin/smtpproxy'` once
(persists the rule), plus `sudo restorecon $HOME/go/bin/smtpproxy` added into
[do.deploy](do.deploy) after every `go install`, since each rebuild recreates the file
with the default `user_home_t` label and would otherwise re-break the service on the
next deploy.

**How to apply:** Any future Go project deployed to this Fedora host (or any
SELinux-enforcing target) that installs its binary under `$HOME` needs the same
`restorecon`-after-build step in its deploy script — this is not a one-off, it recurs
on every rebuild.

Cert renewal now runs locally on the box via `certbot` + `certbot-renew.timer`
(HTTP-01, standalone mode, port 80 forwarded on demand) with a deploy-hook
([certbot-deploy-hook.sh](certbot-deploy-hook.sh), installed at
`/etc/letsencrypt/renewal-hooks/deploy/smtpproxy.sh`) that copies the renewed cert into
`/home/erikp/ssl/` and restarts `smtpproxy`. This replaces a prior setup where the
Let's Encrypt cert was actually being issued/renewed by a HomeAssistant instance and
manually copied to the Pi — that HomeAssistant Let's Encrypt config and the old Pi are
both candidates for decommissioning now.
