---
name: feedback-terse-chat-and-rebuild-before-test
description: "Keep chat messages terse during live debugging (user finds long explanatory messages clutter their terminal); always `go install .` (or equivalent rebuild) after generator/coordinator code changes before testing live against a real house, not just `go build`."
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-10-15T10:36:53.476Z
---

Two corrections from the 2026-08-23/24 session building the `home_assistant` bridge integration
and coordinator-based assumed-entity warnings:

**Terse chat messages.** User said explicitly: "You seem to forget that your chat messages
clutter my window." During live/interactive debugging especially, prefer short, direct messages —
the next command(s) to run, or a one-line status — over explanatory paragraphs. Save longer
explanations for when the user asks "why" directly, or for durable write-ups (PROJECT.md, code
comments), not for routine back-and-forth.

**Rebuild + reinstall before live testing.** After editing the `homeassistant` generator package,
`go build ./...` only proves it compiles — it does NOT update the installed `homeassistant` binary
on `$GOPATH/bin` that each house's own run script actually invokes. Ran a live run against real
Junglinster data after a generator change once this session and got stale (pre-change) output,
which briefly looked like a real bug. Fix: always `go install .` from the `homeassistant/`
directory before treating a live run as a real verification, not just `go build`.

**Why:** both wasted real debugging time this session by pointing at the wrong signal (a stale
binary, or a wall of text mid-crisis) instead of the actual next useful action.

**How to apply:** in this project specifically — after any change to `homeassistant/*.go`, run `go
install .` (from that directory) before the next `./generate`/`./deploy` test. Note: each house's
top-level script was renamed from `configure` to `generate` on 2026-08-26 (per
[[feedback_generator_coordinator_terminology]]'s naming cleanup) — use `./generate`, not
`./configure`, when running live verification from now on. During any live debugging thread
(especially production HA instances), default to short, action-oriented messages; expand only
when asked.

**Reinforced 2026-10-15, sharper framing**: user said "You keep 'dumping' text here, without
giving me a chance to respond/act." This isn't only about message LENGTH (the original lesson
above) — it's about PACING/turn-taking: don't chain multiple unprompted things into one turn (a
status recap + a memory-save confirmation + a "let me know when..." closer + a new question) when
there's exactly ONE pending action. End the turn right after stating that one thing, don't pad it
with summary/wrap-up text the user didn't ask for. If there's truly nothing to add beyond "waiting
on you," say that alone and stop — don't manufacture closing paragraphs. This matters especially
mid-live-debugging/mid-deploy, where the user is actively switching between their own terminal and
this chat and needs each turn to be scannable in one glance.
