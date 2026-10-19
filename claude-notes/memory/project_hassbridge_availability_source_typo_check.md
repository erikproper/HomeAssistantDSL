---
name: project-hassbridge-availability-source-typo-check
description: "DONE (2026-09-15): new validateHassBridgeAvailabilitySources generator rule catches a typo'd literal entity_id in a hassbridge '... is available;' capability source. Read this if a hassbridge node/availability condition silently never comes online."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-15T16:57:08.228Z
---

User noticed a real gap while reviewing a hand-edited Physical.def "is available" line: the
"discovery" gateway mechanism (item 4) already validates its own same-device sibling references
(`registerDiscoveryAvailabilityEntityLink`/`registerDiscoveryDerivedEntityLink`,
Conceptual_DiscoveryEntities.go) — a typo'd capability label errors clearly at generate time. But
the OLDER `home_assistant`/hassbridge mechanism's own `"... is available;"` sugar
(`node.fritz_box`, `node.vienna`, etc.) takes a raw, hand-typed literal entity_id with ZERO
validation anywhere — it flows straight through `sourceToJinja2`/`registerDeviceSourceEntityLink`
into a Jinja `states('...')` call. A typo there produces no build/generate error or warning at
all, just a condition that's permanently `unavailable` in HA.

Fixed: `validateHassBridgeAvailabilitySources`
(`homeassistant/physical_rule_hassbridge_availability_sources.go`), wired into generator.go's
validation pipeline right after `validateNodeCapabilityRequired`. Checks every hassbridge device's
`"... is available;"` capability source's bare entity_id against that SAME device's own other
(plain, non-"is available") capability sources, across every declaring instance — a match is
required, or it's flagged as a likely typo. Mirrors the discovery mechanism's own
same-device-sibling discipline, adapted to raw-string comparison since hassbridge capabilities
have no label registry to look sibling references up in. Confirmed zero false positives against
every real "is available" line in both Vienna's and Junglinster's actual Physical.def.

Scope: hassbridge (`home_assistant` integration) only — the only place this sugar is used in
either house's `.def` files today, and the only kind (`hosts`/`import`/`discovery` all already
validated some other way) that interpreted a raw literal "is available" entity-id reference with
no validation.

Test: `physical_rule_hassbridge_availability_sources_test.go`,
`TestValidateHassBridgeAvailabilitySources` — reproduces the exact
`fritz_box_7590_ax_gb_recieved`-style transposition typo that prompted this.

See also [[project_meta_reload_servicenotfound_fix]] — the SAME debugging thread that led here
also surfaced a real, unrelated live bug in fritz_box's own "is available" source (its underlying
entity_id had actually changed upstream, not a typo) and, downstream of chasing that, the systemic
`continue_on_error`/`ServiceNotFound` reload bug.
