---
name: project-clean-xanadu-todo-definition
description: "Defines what PROJECT.md's own \"[ ] Clean xanadu\" checklist item actually means, clarified 2026-10-16: (1) audit xanadu for anything installed outside apt's own package management and bring it back in line, (2) align what's running there with its narrowed future role (Plex video + the RAID backup array only)."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-16T11:39:54.578Z
---

**What "Clean xanadu" means** (the user's own words, clarifying an otherwise-terse PROJECT.md
checklist line): two distinct parts, not one vague tidy-up --
1. **Package hygiene**: audit what's actually installed on xanadu and make sure it's "in line
   with the official apt-get structures" -- i.e. anything installed outside apt's own package
   management (manually-built binaries, tarball installs, etc.) should be identified and, where a
   proper apt package exists, replaced with it. Same underlying principle as
   [[feedback_prefer_real_fix_over_workaround]] (real/official tooling over ad hoc shortcuts),
   just applied as a one-time audit rather than a live incident.
2. **Role alignment**: once [[project_xanadu_repurposing_idea_2026_10_13]]'s plan actually lands
   (music + container backups moving off xanadu onto protocols-server-1@Junglinster and
   mqtt.erikproper.eu respectively), xanadu's own installed services/config should be pared down
   to match its narrowed future role -- Plex (video collection only, music having moved
   elsewhere) + the RAID-based backup array (`/mnt/yotta*`) -- rather than still carrying
   config/services for roles it no longer serves.

**Third step, added right after**: once the role is actually reduced to Plex-video + RAID-backup
only, the user will ALSO do a full OS release upgrade on xanadu -- the narrower role makes that
safer/simpler to do than attempting it while xanadu still carries everything else. Sequenced
AFTER the package-hygiene audit and role pare-down above, not concurrent with them.

**How to apply**: this is a cleanup/audit task, sequenced AFTER the repurposing plan it depends
on actually happens (not yet -- see that memory's own "how to apply" for the dependency chain).
Don't start auditing/removing anything on xanadu, or suggest the OS release upgrade, until the
music+backups migration is confirmed done.
