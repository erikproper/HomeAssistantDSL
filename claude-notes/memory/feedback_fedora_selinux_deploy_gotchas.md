---
name: feedback-fedora-selinux-deploy-gotchas
description: "Two distinct SELinux gotchas found live on the Fedora-based p-s-1/p-s-2 hosts — port labeling for SSH port-forwards, and exec context for manually-deployed binaries."
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-14T19:46:33.141Z
---

Both found live 2026-09-14 on Vienna's protocols-server-1 (Fedora), both silent-failure modes with
no useful error unless you know to look for SELinux specifically.

**1. Port labeling blocks `ssh -L` forwards, not just server-side listeners.** sshd's own confined
SELinux domain can only forward TCP connections to ports labeled with a type it's permitted to
reach (`http_port_t`, `http_cache_port_t`, ...). An unlabeled/wrongly-labeled port (Zigbee2MQTT's
8099 defaulted to `preupgrade_port_t`) makes `ssh -L` forwards to it silently fail with "channel N:
open failed: connect failed: open failed" — while a direct shell-initiated connection (`curl`) to
the same port, on the same host, succeeds fine, since it runs in an unconfined domain. No AVC denial
necessarily shows up in `ausearch` depending on audit config, so don't rule this out just because
the audit log is quiet. Fix: `sudo semanage port -m -t http_port_t -p tcp <port>`; confirm with
`semanage port -l -C` (shows local overrides).

**2. A binary copied via `sudo mv` from `/tmp` keeps `/tmp`'s SELinux context, not the destination
directory's.** Deploying a rebuilt coordinator binary by `scp`-ing it to `/tmp` then `sudo mv` to
`/usr/local/bin/coordinator` left it labeled `user_tmp_t` instead of `bin_t` — `systemctl restart`
then failed with `status=203/EXEC` (nothing more specific). `mv`/`rename` preserves the source
file's own label; only genuine *file creation* at the destination (via `cp`, `install`, or a fresh
write) picks up the destination directory's default label through SELinux's own type-transition
rules. This is exactly why the project's own `coordinator/deploy.junglinster` script uses `sudo
install -m 755 coordinator /usr/local/bin/coordinator` rather than a move — its own header comment
already documents the reasoning. Fix once already labeled wrong: `sudo restorecon -v <path>`.

**How to apply:** when manually deploying a binary outside the project's own `deploy.d/`/
`deploy.<house>` scripts (e.g. an emergency hotfix before a full deploy makes sense), prefer `cp`/
`install` over `mv` from a scratch location, or `restorecon` immediately after. `status=203/EXEC` or
a "channel N: open failed" with no other explanation on these Fedora hosts is a strong SELinux
context/labeling signal, not necessarily a real connectivity or permissions problem.
