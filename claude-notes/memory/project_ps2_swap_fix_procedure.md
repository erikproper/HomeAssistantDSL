---
name: project-ps2-swap-fix-procedure
description: "Exact procedure used to fix Vienna's protocols-server-2 OOM-killing the picture-frame slideshow (2026-09-09) — zram-only swap with no disk-backed overflow; the model to follow for frame.junglinster's own eventual Fedora migration."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-10T09:39:00.969Z
---

**Root cause (2026-09-09)**: Vienna's `protocols-server-2` (a resource-constrained Raspberry Pi,
~916Mi RAM) ran HA container + mqtt_commandline + X11/feh on zram-only swap (916M, sized to match
RAM 1:1, no real disk-backed overflow at all). Under load, the kernel OOM-killed `feh` outright
(confirmed via `dmesg`), which is what caused the picture-frame slideshow's on/off asymmetry
investigation to spiral before the actual cause was found.

**Fix applied, safe and reversible (doesn't touch the existing zram)**:
```
fallocate -l 4G /swapfile
chmod 600 /swapfile
mkswap /swapfile
swapon /swapfile
grep -q '^/swapfile ' /etc/fstab || echo '/swapfile none swap sw 0 0' >> /etc/fstab
```
Result: `swapon --show` shows `/dev/zram0` at priority 100 (used first, fast) and `/swapfile` at
the kernel's default priority -1 (only used once zram is full) — zram absorbs the common case,
the disk-backed file is genuine overflow capacity for real memory pressure.

**How to apply elsewhere**: frame.junglinster (PROJECT.md item 2, still RaspberryOS/external SSD as
of 2026-09-10) is the same class of hardware and will need the same fix once it's migrated to
Fedora on a USB stick — fold it into that migration's own fresh-install step rather than
retrofitting it live. Junglinster's OWN existing "protocols-server-2" (a different, better-resourced
machine hosting laserjet/colourjet/envoy) already has an equivalent real disk swap partition
(2.4G) — not something added by this session, just confirmed already adequate when checked
2026-09-10.
