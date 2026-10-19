---
name: project-frame-oom-no-swap-fix-2026-10-15
description: "DONE 2026-10-15: both frame.vienna and frame.junglinster (Ubuntu, ~894Mi RAM, cloned image) had zero swap configured at all (no zram, no swapfile) -- feh got OOM-killed on both. Fixed with a 4G disk-backed /swapfile on each, persisted in /etc/fstab."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-15T12:14:58.373Z
---

**Root cause**: `dmesg -T` on both frame hosts showed `feh invoked oom-killer` / `Out of memory:
Killed process ... (feh)` (frame.junglinster 2026-10-15, frame.vienna 2026-10-14 -- a day apart,
same underlying image). `swapon --show` was completely empty on both -- no zram, no swapfile, no
fstab swap entry at all, on a ~894Mi-RAM board with feh's own RSS climbing to ~480-500MB over a
long-running slideshow. This is the SAME class of bug [[project_ps2_swap_fix_procedure]] fixed on
Vienna's protocols-server-2 back on 2026-09-09 (zram-only there, no swap at all here) -- the fix
never got carried into the Ubuntu image both frame hosts were cloned from.

**Fix applied (both hosts)**:
```
sudo fallocate -l 4G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
```
No zram present on either host (unlike p-s-2's case) -- just a plain disk-backed swapfile, plenty
of headroom (102G/118G free on frame.junglinster's root). Confirmed live: slideshow restarted
clean on frame.junglinster afterwards.

**How to apply**: if ANY other host cloned from this same frame/Ubuntu image shows up (e.g. a
future third picture frame), check `swapon --show` and add this same swapfile proactively, don't
wait for the OOM to recur. The clone image itself still has no swap baked in -- worth fixing at
the image level if more clones are made, rather than patching each one after the fact again.
