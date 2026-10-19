# Checkpoint — Home Assistant DSL Generator + Coordinator

**Frozen:** 2026-10-19 (machine handover — this Mac is going in for service; the new machine
picks up from here, reading only what's synced via Nextcloud).
**Status:** both Go modules (`homeassistant` generator, `house_event_bus_coordinator`) build/vet/
test clean. Vienna's p-s-1 (Pi4) has been fully migrated from Fedora to Ubuntu and is live
(containers, coordinator, smtpproxy, Samba, off-box backups all verified). Vienna's own rule
migration (named-trigger abstraction) is deployed live — including a same-day regression fix
(Moes buttons) — but **full live verification of that fix is NOT done yet**, see below.
Junglinster's Conceptual.def is untouched by any of this session's rule-migration work.

This file is a short pointer, not a duplicate. Durable project-level detail lives in
`PROJECT.md` (open TODOs), `README.md`'s Operational notes (incident write-ups), and
`claude-notes/memory/` (this session's Claude memory, copied out of `~/.claude` so it survives
a machine switch — that directory is local to a machine, not Nextcloud-synced).

**Git note**: a large uncommitted changeset (83 files as of this freeze) spans both Go modules —
this whole multi-day session's work, never committed. Don't assume a clean git state, and don't
discard any of it — `git status`/`git diff` are the authoritative record of what's changed.

---

## What's genuinely IN FLIGHT (needs picking up, not just background context)

1. **Moes button fix — deployed, NOT fully verified.** Real regression found live 2026-10-19:
   every Moes multi-gang button in Vienna stopped working after the 2026-10-15/18 named-trigger
   rule migration. Root cause: the coordinator's own pre-existing discovery-layer
   `state_value_template` normalizes a multi-gang button's raw action (`1_single`) into
   `event_type: "single"` + a separate `button: "1"` attribute — the migrated rules had copied
   the old RAW values straight into the new construct, so they could never match. Fixed by
   extending the trigger declaration's embedded attr-condition to accept a list of values
   (`and if trigger.<attr> is in [...]`), and a new `namedTriggerSourceCondition`
   (`generator.go`, replacing `namedTriggerImpliedCondition`) that per-source "id"-scopes the
   condition when several different devices feed the same named trigger with DIFFERING
   button-number requirements — falls back to the original simple shared-condition shape when
   sources agree, so the Signify dimmer's already-proven output stays byte-identical (full test
   suite + a new dedicated regression test both pass). 43 `Conceptual.def` lines rewritten,
   `./generate` + `./deploy` both run clean, user's **first manual test confirmed working** —
   but they went to sleep before testing the rest. **Next session: confirm the fix across ALL
   affected rules** (full list: see README.md's Operational notes entry, same incident, for the
   complete rundown of which rules/devices are affected).
2. **This `claude-notes/` transfer itself, in progress.** Triggered by this Mac going in for
   service. Once this checkpoint and `MACHINE.md` are both written and reviewed, the transfer
   is done — nothing to resume there unless something was missed (see the transfer's own report
   for what was/wasn't found).
3. **Vienna's old Fedora SSD**: migration off it is fully done and verified (see
   `claude-notes/memory/project_vienna_ps1_fedora_to_ubuntu_migration_done_2026_10_18.md`), but
   the disk itself still needs reformatting + the music collection copying onto it (user's own
   task, using a separate Ubuntu Mac — ext4 isn't natively writable from macOS). Not yet done as
   of this freeze. Destined for Junglinster's p-s-1 as its music disk, target: this coming
   Friday.

## Queued but not started

- **PROJECT.md item 17**: maybe apply the same X-server systemd supervision
  (`xinit-slideshow.service`, `Restart=always`) to Junglinster's own picture frame that Vienna's
  frame got 2026-10-19 — not yet even confirmed Junglinster's frame shows the same symptom.
- **Vienna's shower_room door_moes family** — turned out to already be migrated (a 2026-10-18
  note claiming otherwise was wrong, corrected in PROJECT.md).
- A fresh grep sweep of the rest of Vienna's Conceptual.def for any other remaining raw
  event-code `when` clauses the earlier migration batches might have missed.
- Junglinster's Conceptual.def: zero rules migrated to the named-trigger construct — entirely
  separate, not started. Given Vienna's OWN Moes devices all had the latent normalization bug,
  check whether Junglinster has Moes-style multi-gang buttons before migrating there, and build
  the trigger declarations with the normalized `event_type` + `button`-attribute form from the
  start.
- Certbot/Let's Encrypt renewal duty still needs moving from Vienna's retired Fedora box
  (`certbot-renew.timer`) to the new Ubuntu one before the current cert expires 2026-12-06.
- Items 48/49 in PROJECT.md (template-entity stable-id gap; named-trigger-as-a-real-HA-event-
  entity + discovery-vs-templating reflection) — both still just ideas, nothing designed.

## Key files (generator-side work touched this session)

| File | Role |
|------|------|
| `homeassistant/generator.go` | Bulk of YAML generation; **`namedTriggerSourceCondition`/`attrValueCondition`/`namedTriggerSourceID` (2026-10-19, replaces `namedTriggerImpliedCondition`)** |
| `homeassistant/administration.go` | Runtime state; `TNamedTriggerSource` (**`TriggerAttrValues []string`, 2026-10-19, was a single string**) |
| `homeassistant/Conceptual_NamedTriggerEntities.go` | Device-level `trigger ...;` construct; **attr-condition suffix now accepts a bracketed list too (2026-10-19)** |
| `homeassistant/Conceptual_NamedTriggerEntities_test.go` | **New test: `TestNamedTriggerDisagreeingSourcesGetPerSourceScopedConditions`** |
| `homeassistant/rule_parser.go` | Rule grammar incl. `when trigger.<name> is triggered` |
| `house_event_bus_coordinator/` | Unchanged this session — the Moes bug was purely a generator/`.def` mismatch with this module's OWN pre-existing (unchanged) discovery normalization |

## Commands

```bash
cd homeassistant && go build ./... && go vet ./... && go test ./...              # generator module
go install .                                                                      # ALWAYS before trusting a real ./generate run (from homeassistant/)
cd /Users/erikproper/SmartLiving/Vienna && ./generate && diff -rq hass/vienna <snapshot>  # real regenerate, real house -- diff before deploying
cd /Users/erikproper/SmartLiving/Vienna && ./deploy                               # sources deploy.d/01..05 in order
```

SSH access, sudo conventions, and absolute paths this project's scripts depend on: see
`claude-notes/MACHINE.md` (written alongside this file, same transfer).

## Machine-handover transfer, 2026-10-19: what's missing and why

`claude-notes/` (this file's sibling) carries everything from `~/.claude` that this project's
own docs/memory reference, copied out because `~/.claude` itself is local to a machine, not
Nextcloud-synced. Checked every `[[...]]` link and `project_*`/`feedback_*` mention across
`PROJECT.md`, `README.md`, `Architecture.md`, and this file — all resolved to a real file now
under `claude-notes/memory/`, none missing.

Two things referenced but genuinely absent, not renamed or merged:
- `~/.claude/plans/derived-capability-mechanism.md` and `~/.claude/plans/via-device-inference.md`
  — both referenced from memory/PROJECT.md as the design plan for now-FINISHED work (the
  "derived" capability mechanism; `via_device`, PROJECT.md's own words: "finished, formerly item
  3"). Likely cleaned up once that work completed, while the corresponding memory files
  (`claude-notes/memory/project_derived_capability_mechanism.md`, present) kept the durable
  summary. Only `binary-weaving-iverson.md` and `splendid-zooming-lighthouse.md` actually exist
  in `~/.claude/plans/` and were copied to `claude-notes/plans/`.
- `~/.claude/CLAUDE.md` (user-level instructions) doesn't exist on the old machine at all —
  nothing to copy, `claude-notes/user-CLAUDE.md` was never created.
