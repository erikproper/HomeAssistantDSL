---
name: feedback-no-permission-needed-for-doc-updates
description: "Don't pause to ask permission before updating PROJECT.md/Architecture.md/plan files that this session is already actively maintaining -- just edit them directly as part of the work."
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-10T13:59:57.694Z
---

Editing PROJECT.md, Architecture.md, or an active `/Users/erikproper/.claude/plans/*.md` file to
record progress is not a decision that needs a check-in -- these are already being edited
continuously through the session as a normal part of doing the work (see
[[feedback_project_md_stays_a_todo]]), not a discrete action to seek approval for.

**Why:** the user pointed out (2026-09-10) that asking "should I update this doc" reads as
unnecessary when the same doc has already been edited repeatedly earlier in the same session with
no objection -- asking again about something already established as fine adds friction with no
value. Asking about *next steps in the actual engineering work* (which increment to build next,
which design fork to take) is still genuinely useful and should continue.

**How to apply:** don't phrase progress-recording doc edits (PROJECT.md status updates, plan-file
"Progress" notes, Architecture.md target-design writeups) as something awaiting permission, and
don't frame turn-ending questions around them. Reserve check-ins for real forks in the engineering
work itself -- which runtime path to build, which device to test against first, whether to proceed
to a riskier step -- not for whether it's okay to write down what was just done.
