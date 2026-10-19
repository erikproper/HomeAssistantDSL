---
name: feedback-prefer-real-fix-over-workaround
description: "User strongly rejects quick workarounds/hacks in favor of root-cause fixes, even when the workaround is faster or the hack is already proven to work. Applies broadly, not just to DNS."
metadata:
  node_type: memory
  type: feedback
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-16T11:27:44.503Z
---

**Rule**: when something is broken and I reach for a quick, working-but-superficial fix (a static
`/etc/hosts` override instead of fixing the real DNS config; a manually-downloaded tool tarball
instead of the distro's own maintained package), surface that it's a workaround and offer the
real fix as the default, rather than quietly shipping the workaround as "done."

**Why**: real incident, 2026-10-16. I added a static `/etc/hosts` entry (`192.168.178.34 xanadu`)
on Vienna's new p-s-1 SSD to work around a DNS search-domain issue -- the SAME class of
workaround I'd already used once before (Junglinster's p-s-1, 2026-10-15) without the user
objecting at the time. This time the user pushed back hard: "That is not a fix, but a hack!
Revert this immediately!" -- and separately, in the same message, also rejected installing Go
from the official tarball ("Use the official Ubuntu version!") purely because it sidestepped an
apt lock rather than being the right tool for the job. Both reverted; the real DNS fix turned out
to be a one-line netplan `dhcp4-overrides`/`dhcp6-overrides: use-domains: true` drop-in --
genuinely not much harder than the hack, just required diagnosing instead of papering over.

**How to apply**: before reaching for a workaround (hosts-file override, a manually-fetched
binary instead of a package, a sleep/retry loop instead of fixing a race, etc.), ask: is there a
proper fix at the actual config/root-cause level, even if it takes a bit more investigation?
Default to finding and proposing that first. If a workaround is genuinely the only practical
option (e.g. truly no real fix exists, or it's explicitly temporary pending something out of my
control), say so explicitly and flag it as a workaround -- don't present it as equivalent to a
real fix. This applies well beyond DNS/networking -- it's a general stance on quality over
expedience.
