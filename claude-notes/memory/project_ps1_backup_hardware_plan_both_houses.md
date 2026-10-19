---
name: project-ps1-backup-hardware-plan-both-houses
description: "Disaster-recovery hardware plan for both houses' p-s-1 role, as of 2026-10-16: a 3-week SSD relay (Vienna Fedora->Ubuntu 2TB swap, then that freed 2TB goes to Junglinster as the real music disk, then the freed 0.5TB test disk goes to Vienna as a backup combo) -- NOT finished after the first Vienna trip alone."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-16T07:51:41.982Z
---

**Corrected timeline (the user explicitly flagged an earlier, too-fast version of this as
wrong)** -- this is a 3-week relay of SSDs between the two houses, not something that completes
in one Vienna trip:

1. **Week of Sun 2026-10-18, in Vienna**: migrate Vienna's own p-s-1 from Fedora to Ubuntu --
   onto a 2TB SSD (a DIFFERENT, fresh 2TB disk, not the 0.5TB one from the Junglinster Samba/Plex
   test). Vienna's Pi4 reboots on this new Ubuntu 2TB SSD; the OLD Fedora 2TB SSD is now free.
   (The spare 2nd Pi4 also gets left in Vienna this same trip, per the earlier note below -- but
   NOT yet paired with a backup SSD; that pairing happens in week 3.)
2. **Right after, back in Junglinster**: bring the now-freed Fedora 2TB SSD back to JLI and
   connect IT to protocols-server-1 instead of the 0.5TB disk currently serving
   [[project_samba_plex_xanadu_to_ps1_migration_plan]]'s own test -- the 2TB becomes the real,
   permanent music-collection disk there (bigger capacity than the 0.5TB test disk). The 0.5TB
   stays connected and in active use until THIS swap actually happens -- explicitly "until next
   weekend" from 2026-10-16, i.e. do not treat the 0.5TB as already free.
3. **The week after THAT** (roughly 2026-10-25+): once the 2TB has taken over at Junglinster, the
   NOW-freed 0.5TB SSD travels to Vienna and pairs with the spare 2nd Pi4 already left there in
   step 1 -- THAT is when the backup Pi4+SSD combo for Vienna's p-s-1 actually exists, not right
   after the first Sunday trip.

**Junglinster's own existing fallback** (unchanged, separate from this relay): if
protocols-server-1@Junglinster (the new HP EliteDesk) fails, xanadu or the old Mac Mini server
("backups") can stand in -- already-existing hardware, nothing new being prepared for this.

**How to apply**: don't say the Vienna backup Pi4+SSD combo exists until the WEEK 3 step above has
actually happened -- there are two intermediate SSD swaps first. If asked "is the Vienna backup
ready yet," check which week it is relative to 2026-10-18 before answering.
