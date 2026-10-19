---
name: project-named-trigger-abstraction-step1-2026-10-15
description: "Mechanism DONE+DEPLOYED LIVE (Vienna, 2026-10-18): device-level \"trigger <name> when <capability> is '<value>';\" + rule-level \"when trigger.<name> is triggered\" + \"device <id> at <leaf> with:\"/\"::\" leaf-reuse mechanism. 25+ Vienna rules migrated and live. NEXT: shower_room's door_moes_1/2 family not yet migrated; Junglinster untouched."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-18T22:42:14.560Z
---

**Final syntax** (went through two real revisions this session, both from live migration/user
feedback, not guesswork):
1. Device side: `trigger <sphere:path> when <capability> is '<value>';` / `... is in ['<v1>',
   '<v2>', ...];` inside a `device <id> [at <leaf>] with: ... end;` block.
2. Rule side: `when trigger.<sphere:path> is triggered` -- NOT `when [a] <name> is triggered`
   (an earlier, now-superseded form). Reuses the existing `trigger.<attr>` template-condition
   prefix deliberately (same idea: a derived value from the firing context) -- parses for free
   via the ordinary entity-ref grammar (domain "trigger"), swapped to Kind "named_trigger" after
   the fact (`resolveNamedTriggerReference`, rule_parser.go). No special lookahead/backtracking
   needed -- simpler than the original implementation.
3. The named trigger's own Name is resolved space-relative (`normalizeEntityFullName`), exactly
   like any other entity reference -- NOT taken literally. This matters: the same relative name
   written in two different spaces resolves to two different keys, so no collision. A device and
   a rule in DIFFERENT spaces referencing the SAME named trigger must use the absolute `/` form on
   both sides (or they resolve to different strings and never connect) -- real case: Vienna's
   `buttons.hallway_bedroom_door_moes` (declared in hallway's own space) feeding
   `social:main/light/toggle` (positioned in bedroom's space) via
   `social:/apartment/bedroom/main/light/toggle_request`.
4. `device <id> at <leaf> with:` (new, deliberately narrower than the `device <id> as <leaf>
   with:` grammar removed outright 2026-10-11, item 10 -- confirmed OK with the user, that removal
   stands, this is a genuinely different, opt-in-per-line mechanism): sets a per-block anchor
   leaf. A `::` (double colon) in ANY line inside that block folds the anchor in
   (`expandDeviceAtLeafDoubleColon`, pure text substitution, does NOT touch the shared
   `namingSpacePath`/`forDeviceNamePath` machinery other live features still depend on); a plain
   single `:` is completely unaffected. A `trigger ...;` line's own hidden entity uses the anchor
   leaf directly when present, else falls back to deriving one from the device id.

**Why `at <leaf>` exists at all**: migrating an EXISTING device's raw `entity event.physical:door
from event.pressed;` line to the bare-capability `trigger` form, with NO leaf override, computes
a BRAND NEW local path from the device id (e.g. `hallway_door_moes` instead of `door`) -- the live
HA entity's own `unique_id` stays frozen (Physical.def's `as <id>` stable-identity mechanism,
confirmed real via code trace: `StableID` -> MQTT discovery's `unique_id` only), but THIS
GENERATOR's own `entity_id:` string embedded in automation YAML is computed independently from
the Conceptual.def local path, with zero connection to `StableID` -- so a careless rename breaks
the generated automation's own trigger target, even though the registry entity itself is safe.
`at <leaf>` lets the author pin the SAME leaf the live entity already uses.

**First real migration, DONE+VERIFIED**: Vienna's `buttons.hallway_door_moes` (`at door`) +
`buttons.hallway_bedroom_door_moes` (`at bedroom/door`), 3 rules (`main/light/toggle` ×2 across
two different spaces, `/leaving`). Verified via `./generate` (zero warnings) and a direct diff of
the generated automation YAML against a pre-migration snapshot -- identical entity_ids,
event_types and actions (one harmless trigger-list ordering difference, irrelevant to HA's own
"any one trigger fires it" semantics).

**4 real bugs found and fixed during this migration** (all in the mechanism itself, not the DSL
content): (1) two `trigger` lines sharing one device+capability falsely flagged as a copy-paste
duplicate -- fixed via `NamedTriggerEntityIDByDeviceCapability` reuse cache. (2) the rule-side
grammar didn't accept the `/`-absolute-path escape hatch (now moot -- superseded by reusing
EntityRef's own grammar, which already had it). (3) named-trigger names were originally taken
literally (not space-relative) -- real collision risk, fixed per above. (4) the ORIGINAL
device-id-derived auto-leaf broke a real live automation's entity reference -- fixed by `at
<leaf>`.

**Step 2 + action-verb cleanup, DONE+LIVE 2026-10-15** (same day, same session): the trigger
declaration's optional `and if trigger.<attr> is '<value>';` suffix folds into the generated
automation's own condition at GENERATION time (`namedTriggerImpliedCondition`, generator.go) --
scoped narrowly: only applies when every trigger on the rule is a named trigger and all sources
agree on one (attr,value) pair. `dim`/`brighten <entity-ref> with <N>%;` (rule_parser.go's
`DimAction`/`BrightenAction`) are sugar for `turn ... on with brightness_step_pct -N/+N;`. `with
[default] brightness <N>%;` (`DataFields`) is sugar for `with brightness_pct <N>;` -- both
`default`/`%` purely cosmetic, discarded at parse time. Verified live on Vienna's whole Signify
dimmer (4 rules: turn_on/turn_off/dim_up/dim_down) -- generated automations confirmed
identical/equivalent (dim_up/dim_down's trigger list structurally simplified: 2 same-entity
trigger entries -> 1 with a 2-item `event_type` list, same HA schema, not a regression).

**DEPLOYED LIVE 2026-10-18**: physically in Vienna, ran the real `./generate` and diffed the
output against a pre-migration snapshot of `hass/vienna/` -- byte-identical, zero behaviour
change across the whole migration (hallway door family, Signify dimmer, living_room/kitchen_bj
family, terrace/light/toggle cross-room rule, bedroom's own door family -- all of it). Then ran
the real `./deploy` end-to-end (main HA instance + protocols-server-1's own HA + coordinator +
hosts/cpu/ping + commandline), clean except one unrelated known_hosts gap on the freshly-migrated
Ubuntu box (see [[project_vienna_ps1_fedora_to_ubuntu_migration_done_2026_10_18]]), now fixed. The
5 flagged multi-value-event_type automations are live; no further verification blocker remains.

**Same Sunday trip, Vienna's own p-s-1 hardware plan** (refines PROJECT.md's "Hardware in
Junglinster" item -- NOT a whole-unit swap after all): keep Vienna's OWN existing Pi4 enclosure;
just swap in a prepared Ubuntu SSD as the new boot disk, reboot Vienna's Pi4 on it, then remount
the OLD Fedora SSD as a secondary (non-boot) disk to migrate the container data (zigbee2mqtt/
zwave-js/mosquitto state etc.) across -- the exact same "mount old rootfs as secondary, pull
container data over" procedure already proven working at Junglinster's own p-s-1 HP migration
earlier this session (2026-10-15). Separate from (but same trip as) the pulled music
library/named-trigger deploy verification above.

**Prep done remotely 2026-10-16 (before the physical trip), on the fresh Ubuntu SSD reachable at
192.168.178.53**: Samba (`[music]` share matching Vienna's own Fedora smb.conf convention
exactly -- `valid users = erikp`, NOT a dedicated service account like the Junglinster test used
-- `erikp`'s own Samba password still needs `smbpasswd -a erikp` run interactively), a 4G
swapfile (none existed), `cpu`+`ping` cron jobs + mosquitto-clients, the REAL DNS search-domain
fix (netplan `dhcp4-overrides`/`dhcp6-overrides: use-domains: true` -- see
[[project_ps1_junglinster_dns_search_domain_reboot_bug]], NOT the `/etc/hosts` hack that was
tried and explicitly rejected first), podman, and the official `golang-go` apt package (go1.26.8
-- NOT a manually-fetched tarball, also explicitly rejected first). The user is independently
copying the real music library across to `/srv/samba/music` themselves.

**Deliberately NOT done yet, confirmed to wait for the physical Vienna trip**: the podman
quadlets (homeassistant/mosquitto/zigbee containers), the coordinator (Go binary build +
systemd service + config), and the backup-home-automation script/timer/conf. The user's own
plan: once physically in Vienna, reboot the Pi4 on this Ubuntu SSD, connect the OLD Fedora SSD
alongside it, migrate the container data across, get the coordinator running, and bring up the
backup scripts -- and AT THAT SAME MOMENT (not before, not separately), also do the "move
home-automation backups to mqtt.erikproper.eu" item for BOTH houses at once (Vienna AND
Junglinster together), not as a separate later step.

**Migration progress, Vienna** (same day): hallway door/bedroom-door family (3 rules), the whole
Signify dimmer (4 rules), the living_room door/moes + kitchen_bj family (11 rules), and
terrace/light/toggle (3 cross-room devices: living_room's kitchen_door_moes_1/living_door_moes_1
+ bedroom_door_moes_1) -- all verified byte/semantically identical to pre-migration. Recurring
real gotcha across this whole batch: a device's raw entity is sometimes consumed by MORE THAN
textually-literally-greppable consumers -- always grep for the device's OWN underlying
leaf/entity substring across the WHOLE file (both relative AND absolute "/"-prefixed spellings)
before retiring its explicit "entity ... from <capability>;" line, not just the rules you already
know about. Caught 3 times live this session (bedroom's cross-space toggle_request, then
terrace/light/toggle's 3-device cross-room rule).

Bedroom space's own device family (bed_moes, bed_shortcut_left/right, door_moes_1/2,
hallway_door_moes_1/2) ALSO fully migrated same session: main/light/toggle (now a single named
trigger folding in the earlier cross-space clause too), blinds/down/stop/up,
bed/left/right/light/toggle, bed/light/toggle -- 7 more rules, all verified byte-identical except
the same harmless multi-value-bundling cosmetic diff as elsewhere.

Vienna device/rule migration is now substantial: hallway door family, Signify dimmer, living_room
door/kitchen_bj family, terrace/light/toggle (cross-room), bedroom's own door family -- 21+ rules
total. Not yet touched: shower_room's own door_moes_1/2 (same pattern, seen in passing at
Conceptual.def lines ~764+ -- not yet surveyed for consumers), and likely more elsewhere in the
file/Junglinster not yet looked at.

**Explicitly deferred, not started**:
- A LOGICAL-layer generalisation floated by the user, NOT designed: `derived capability event.YYY
  with: trigger DDD.XXX turns ZZZ; [device_class ...;] [delay_on ...;] [delay_off ...;] end;` --
  a derived capability defined by another device's own capability transition + HA-style debounce.
- The rest of both houses' real rules, migrated incrementally -- not all at once.

**How to apply**: read /Users/erikproper/.claude/plans/binary-weaving-iverson.md for full design
history before continuing. Before migrating any further device, check whether it needs `at
<leaf>` (does any OTHER rule/device reference its entity_id by a specific existing name?) --
don't assume bare device-id-derivation is safe without checking first, per the real incident
above. **Concrete next step (flagged by the user 2026-10-18 for the next session)**: start with
Vienna's shower_room `door_moes_1`/`door_moes_2` family, then do a fresh grep sweep of the rest of
Vienna's Conceptual.def for any other remaining raw event-code `when` clauses before assuming the
file is fully migrated -- PROJECT.md's own "Deal with the 'physical' sphere" item has been updated
to list these same next steps. **CORRECTION 2026-10-19**: shower_room's door_moes_1/door_moes_2
turned out to ALREADY be migrated (found while fixing the regression below) -- the 2026-10-18 note
above was wrong, there was no un-migrated shower_room family left.

**REAL REGRESSION found + fixed live, 2026-10-19: every Moes multi-gang button in Vienna broke.**
Full detail in README.md's own Operational notes (same incident, Nextcloud-synced, read that for
the complete root-cause writeup) -- summary: the coordinator's pre-existing discovery-layer
value_template normalizes a multi-gang Moes button's raw action ("1_single") into event_type
"single" + a separate "button" attribute, which the migrated rules never accounted for (they
copied the OLD raw values straight into the new construct). Fixed by (1) extending the trigger
declaration's own embedded "and if trigger.<attr> is '<value>'" suffix to ALSO accept a bracketed
list (`is in ['<v1>', ...]`, mirroring the base capability-value shape), and (2) a new
`namedTriggerSourceCondition` (replacing `namedTriggerImpliedCondition`, generator.go) that
assigns each contributing trigger entry its own "id" and builds a per-source OR-of-(id, attr)
condition when sources DISAGREE on their own button filter -- the real case here, since 4
different physical devices fed Vienna's own `bedroom/main/light/toggle`, each needing a
different button number. Falls back to the ORIGINAL single-shared-condition shape (no id
machinery) when sources agree, preserving the Signify dimmer's output byte-identical -- verified
via the full test suite plus a new dedicated regression test
(`TestNamedTriggerDisagreeingSourcesGetPerSourceScopedConditions`). 43 `.def` lines across
Vienna's whole file needed the same mechanical fix -- every migrated Moes device had this bug,
not an isolated one. Deployed live 2026-10-19; user's first manual test confirmed working, full
verification deferred to the next day (machine switch risk flagged same session -- see
[[project_vienna_ps1_fedora_to_ubuntu_migration_done_2026_10_18]] and the README.md writeup,
both Nextcloud-synced, for continuity).
