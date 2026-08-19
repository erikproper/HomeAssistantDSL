---
name: BibTeX project — known over-engineering
description: When resuming the bibtex project, review for over-engineering per convention §14; string map functions are a known candidate.
type: project
---

The `../bibtex/bibtex_check` project was the user's first large Go project. When we work on it together, plan a GO_REVISIT pass that specifically looks for violations of convention §14 (Avoiding Over-Engineering).

**Known candidate:** the string map functions defined within that project are likely over-engineered.

**Why:** It was the first large Go project; abstractions were introduced speculatively that would not be introduced today under convention §14.

**How to apply:** When starting bibtex work, read GO_REVISIT.md and include a targeted review of string map utilities and any other multi-method abstractions that serve only one call site.
