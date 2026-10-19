# Project Memory — Home Assistant DSL

## User

- [Identity](user_identity.md) — Erik (Henderik A. Proper); owns Vienna and Junglinster.

## Reference

- [Main HA SSH access](reference_main_ha_ssh_access.md) — `~/bin/ha.vienna`/`ha.junglinster`, hassio@<house>.homelinux.org -p 1967.

## Feedback Notes

- [EU English spelling standard](feedback_eu_english.md) — British/EU spelling everywhere.
- [Keep node-identity blocks sorted](feedback_keep_node_identity_blocks_sorted.md) — Junglinster's per-space Z-Wave node-identity blocks stay numeric-ID-sorted as they grow.
- [Port old bash generator, don't patch .def files](feedback_generator_must_match_old_bash.md) — missing "implied" entities go into the Go generator, never hand-declared in Entities.def.
- [Terse chat + rebuild before testing](feedback_terse_chat_and_rebuild_before_test.md) — short chat during live debugging; `go install .` before trusting a `./configure` run.
- [Generator/coordinator terminology](feedback_generator_coordinator_terminology.md) — never "compiler"/"transformer".
- [Check for prior design specs](feedback_check_for_prior_design_specs.md) — search memory/PROJECT.md for a deferred design before inventing a new one.
- [PROJECT.md stays a TODO](feedback_project_md_stays_a_todo.md) — completed work's detail goes in README.md/Architecture.md, not there.
- [Ask one at a time](feedback_ask_one_at_a_time.md) — one clarifying question at a time, not batched.
- [No new regexp parsers](feedback_no_new_regexp_parsers.md) — GO_CONVENTIONS §7/§8's three-layer CDL1 architecture only.
- [No permission needed for doc updates](feedback_no_permission_needed_for_doc_updates.md) — don't pause before PROJECT.md/Architecture.md bookkeeping.
- [macOS Local Network permission blocks LaunchAgents](feedback_macos_local_network_permission_launchagents.md) — silent NECP block; diagnose via `log show`.
- [Two-phase deploy for template→coordinator migrations](feedback_two_phase_deploy_for_template_to_derived_migrations.md) — retiring a generator-YAML entity for a coordinator one needs two deploy passes or HA makes a `_2` duplicate.
- [Budget-conscious session management](feedback_budget_conscious_session_management.md) — user tracks weekly usage %, freezes non-urgent work proactively.
- [Verify Zigbee group history before deleting](feedback_verify_zigbee_group_before_deleting.md) — real mistake 2026-09-14, deleted renamed-not-orphaned groups; check history first.
- [Fedora SELinux deploy gotchas](feedback_fedora_selinux_deploy_gotchas.md) — port-labeling blocks ssh -L silently; `mv` from /tmp keeps wrong exec context.
- [Keep discovery_passthrough mechanism](feedback_keep_discovery_passthrough_mechanism.md) — don't remove it during cleanup even once unused by both houses; it's reusable architecture, not dead code.
- [Device naming avoids integration names](feedback_device_naming_avoid_integration_names.md) — device ids name the device's functional category, never the backing mechanism.
- [Explain semantic meaning of live values](feedback_explain_semantic_meaning_of_live_values.md) — state what a live-reported value means in plain terms, not just how it's wired.
- [Verification trigger, not hard blocker](feedback_verification_trigger_not_hard_blocker.md) — "should be X, of course..." means verify and report honestly, not "block unless literally true".
- [Native integrations belong on ha2mqtt](feedback_native_integrations_belong_on_ha2mqtt.md) — e.g. "sun" integration should run on ha_mqtt/ha2mqtt, not main; found as root cause of a naming collision on both houses.
- [Prefer real fix over workaround](feedback_prefer_real_fix_over_workaround.md) — don't ship a hosts-file hack or a tarball-instead-of-package shortcut; find the real root-cause fix first, even if slower.

## Project Notes

- [Junglinster p-s-1 hardware failure, 2026-10-13](project_junglinster_ps1_hardware_failure_2026_10_13.md) — Pi4+external-SSD bus error mid-Fedora-update; wife's power reboot didn't fix it; physical check Thursday 2026-10-15.
- [Junglinster p-s-1/p-s-2 consolidation plan](project_junglinster_ps1_ps2_consolidation_plan.md) — decided 2026-10-13, NOT yet executed: both move onto a new refurbished HP EliteDesk mini PC (Ubuntu); Green stays frontend-only; supersedes the 2026-09-21 "keep p-s-2 separate" call.
- [p-s-1@Junglinster DNS search-domain reboot bug](project_ps1_junglinster_dns_search_domain_reboot_bug.md) — bare hostname resolution broke after reboot, crashed the coordinator; fixed with a static /etc/hosts entry, root cause (netplan search-domain) not yet fixed.
- [ping/report IPv6-fallback bug](project_ping_report_ipv6_fallback_bug.md) — fixed on Vienna's p-s-1 2026-10-14 (added -4); caused the picture-frame switch to show unavailable. SAME latent bug on Junglinster's p-s-1, blocked until it's back up.
- [Living-room bt-proxy plan](project_living_room_bt_proxy_plan.md) — DONE 2026-10-15: denvera/bt-proxy podman quadlet on frame@Junglinster, paired with ha2mqtt's bluetooth integration.
- [p-s-2@Junglinster retired, kept for FS20](project_ps2_junglinster_retired_kept_for_fs20.md) — idle (0 containers), safe to power off, but kept (not decommissioned) + DSL declarations stay, earmarked for a future FS20 experiment.
- [frame OOM/no-swap fix, DONE 2026-10-15](project_frame_oom_no_swap_fix_2026_10_15.md) — both frame.vienna/frame.junglinster had zero swap at all; feh got OOM-killed on both; fixed with a 4G swapfile each.
- [Rule grammar: no ";" between when/if/then, DONE 2026-10-15](project_rule_grammar_no_semicolon_between_clauses_2026_10_15.md) — Pascal-style clause boundaries; TWO separate line-joiners needed fixing, not one; both houses regenerate byte-identical.
- [Named-trigger abstraction, Steps 1+2 DONE+LIVE 2026-10-15](project_named_trigger_abstraction_step1_2026_10_15.md) — "trigger ... and if trigger.<attr> is '<val>';" + "dim"/"brighten"/"[default] brightness %" verbs; verified live on Vienna's Signify dimmer (4 rules) + hallway door (3 rules).
- [Xanadu repurposing idea](project_xanadu_repurposing_idea_2026_10_13.md) — floated 2026-10-13, NOT decided: music onto the retired Junglinster SSD via SMB/Plex; xanadu powers off except for video/backups; container backups need a new home (mqtt.erikproper.eu suggested).
- ["Clean xanadu" TODO definition](project_clean_xanadu_todo_definition.md) — means (1) apt-package hygiene audit, (2) pare down to just Plex-video + RAID-backup role; sequenced after the repurposing plan lands.
- [Samba/Plex Xanadu→p-s-1 test, DONE 2026-10-16](project_samba_plex_xanadu_to_ps1_migration_plan.md) — both native (not containerized) on p-s-1, working; real cutover from xanadu + container-backups-to-mqtt.erikproper.eu item still not started.
- [p-s-1 backup hardware plan, both houses](project_ps1_backup_hardware_plan_both_houses.md) — Vienna gets a spare Pi4+SSD after Sun 2026-10-18; Junglinster's fallback is xanadu/"backups" Mac Mini.
- [Vienna p-s-1 Fedora→Ubuntu migration, DONE 2026-10-18](project_vienna_ps1_fedora_to_ubuntu_migration_done_2026_10_18.md) — containers/coordinator/smtpproxy/Samba/backup-offbox-target(both houses) verified end-to-end; several gotchas reusable for Junglinster's own move.
- [Vienna frame X-server systemd supervision, DONE 2026-10-19](project_frame_vienna_xserver_systemd_supervision_2026_10_19.md) — picture-frame X/feh now auto-respawns (Restart=always) instead of silently dying for hours; PROJECT.md item 17 = maybe same for Junglinster.
- [Manual automation migration, DONE 2026-10-13](project_manual_automation_migration_done_2026_10_13.md) — item 13 complete & removed from PROJECT.md: new CDL1 rule parser, both houses' 85 (84 migrated+1 excluded) automations moved into Conceptual.def; detail now in Architecture.md §9.3/README.md.
- [Device-level "as"/"::" grammar removal, DONE 2026-10-11](project_device_leaf_fold_removal_2026_10_11.md) — item 10 complete & removed from PROJECT.md: Phase A+B both done, ~51 test fixtures fixed, both houses byte-identical.
- [Friendly names / implied node positioning, DONE+DEPLOYED 2026-10-07](project_implied_node_positioning_2026_10_06.md) — item 8 complete & removed from PROJECT.md: DeviceNodeEntityID choke point, new "colocated" construct, commandline's own physical-layer "dependency on", sphere-agnostic heating-detection fix. Both houses deployed, live-verified.
- [Final orphan check, 2026-10-06](project_final_orphan_check_2026_10_06.md) — item 7b: Vienna clean, Junglinster 2 new ghosts found+cleaned (unrelated device rename); sun_solar_elevation collision still unresolved despite earlier "Fixed" note.
- [Stable entity identity, DONE+BOOTSTRAPPED 2026-10-05](project_stable_entity_identity_implementation_2026_10_05.md) — item 7 full switch, both houses: "as <id>" mechanism built, bootstrapped (Vienna 158/164, Junglinster 290/290, 100% verified), caught a real 3rd-construction-site propagation bug.
- [Dimmer device naming fix, 2026-10-05](project_dimmer_device_naming_2026_10_05.md) — Junglinster: all 7 mis-prefixed sensors.* devices renamed (4 buttons.*, 3 lights.* via item 7's new mechanism), zero registry churn.
- [Logical-layer node capability rule, 2026-10-05](project_logical_node_capability_rule_2026_10_05.md) — new validateLogicalNodeCapabilityRequired catches a brand-new logical device missing "node" with no physical counterpart to inherit from; fixed Junglinster's fan.house_wc.
- [Registry orphan audit + cleanup, 2026-10-05](project_registry_orphan_audit_2026_10_05.md) — item 6: both houses' orphan buckets + the full `_2`/`_3` collision bucket emptied (10 deletes + 2 config-entry-removal fixes); 44 live-but-unpositioned entries left for item 20b; recorder-DB liveness beats registry metadata.
- [MQTT migration plan](project_mqtt_migration_plan.md) — coordinator discovery + live metadata + "as area" spaces live on Junglinster; raw-path cleanup still open.
- [EMS-ESP MQTT-direct architecture](project_ems_esp_mqtt_direct_architecture.md) — flows straight to p-s-1's broker, bypasses Junglinster's p-s-2 entirely; not a future load factor there.
- [Vienna p-s-2 HA migration + Junglinster decision](project_vienna_ps2_ha_migration_and_junglinster_decision.md) — Vienna's HA moved p-s-2→p-s-1 (RAM crisis, DONE); Junglinster's own p-s-2 explicitly KEEPS its HA instance.
- [Junglinster physical-presence timeline](project_junglinster_physical_presence_timeline.md) — Erik back on-site ~2026-10-19; physical tasks wait, remote DSL/migration work can proceed now.
- [Meta-reload insufficient after new automations](project_meta_reload_insufficient_after_new_automations.md) — found 2026-09-21: automation.reload didn't republish bridged entities after a deploy. Always verify live MQTT, not just deploy success.
- [Occupation automation sphere bug](project_occupation_automation_sphere_bug.md) — hardcodes "physical" sphere; would collide with a 2nd sphere. Known, not fixed.
- [Deployed house locations](project_deployed_house_locations.md) — both houses live at /Users/erikproper/SmartLiving/<House>/hass/<house>.
- [New architecture plan](project_new_architecture_plan.md) — future: reshape DSL/generator via experiments.
- [Discovery over YAML templates](project_discovery_over_yaml_templates.md) — required (not just preferred) for device metadata; discovery also advertises undealt-with devices.
- [Coordinator deployment](project_coordinator_deployment.md) — Junglinster's coordinator live on protocols-server-1.
- [devices.yaml EventEndpoint debt](project_devices_yaml_eventendpoint_debt.md) — ad hoc scaffolding, not the real EventEndpoint abstraction yet.
- [Roaming devices bridging broker](project_roaming_devices_bridging_broker.md) — multi-broker relay + liveness-timeout built 2026-08-26; deploy verification incomplete.
- [Hosts device metadata future work](project_hosts_device_metadata_future_work.md) — per-device overrides DONE; per-integration default-icon grammar still open.
- [Logical layer device combining](project_logical_layer_device_combining.md) — "absorb" DONE+DEPLOYED 2026-09-18 (Vienna washing_machine); "aggregate" still future.
- [Cloud manifest and reload strategy](project_cloud_manifest_and_reload_strategy.md) — manifest half superseded; reload-trigger half (`meta/reload/...`) accurate.
- [Envoy power/energy naming cleanup](project_envoy_power_energy_naming_cleanup.md) — deferred: Envoy's "total" sensor is energy not power.
- [Entity existence inquiry design](project_entity_existence_inquiry_design.md) — three-state entity tracking live since 2026-08-28; command half (1.6) still deferred.
- [Hassbridge command routing](project_hassbridge_command_routing.md) — Phase 1 DONE+LIVE 2026-09-09 (switch/vacuum); cross-house Phase 2 deferred.
- [HA entity registry collision lessons](project_ha_entity_registry_collision_lessons.md) — a disabled legacy integration reserves its entity_id forever; safe fix procedure; HAOS SSH is restricted.
- [p-s-2 swap fix procedure](project_ps2_swap_fix_procedure.md) — fallocate/mkswap commands that fixed Vienna's p-s-2 OOM (zram-only).
- [HAOS encrypted backup decrypt procedure](project_haos_encrypted_backup_decrypt_procedure.md) — decrypting a password-protected HAOS Supervisor backup without `ha` CLI.
- [Undeclared entity audit technique](project_undeclared_entity_audit_technique.md) — reusable scan for live entities never declared in Spaces.def.
- [Junglinster item 7/8 ordering](project_junglinster_item_7_8_ordering.md) — item 8 (Zigbee passthrough) may run before item 7 (antenna swap, needs on-site).
- [Logical layer media_player_device migration](project_logical_layer_media_player_device.md) — DONE+DEPLOYED 2026-09-16: logical device can declare "is available" + media_player→switch coercion, positioned like a physical device. Junglinster's no_collect/no_play_input queued.
- [Logical layer "dependency on"](project_logical_layer_dependency_on.md) — DONE+DEPLOYED 2026-09-16: `dependency on <device-id>;` ANDs another device's liveness into availability, transitive + cycle-detected. Two more target forms (raw entity/availability deps on "main") queued.
- [Discovery migration PLAYBOOK](project_discovery_device_block_and_node_capability.md) — THE reusable how-to for the next raw Zigbee2MQTT device migration. Read FIRST before any such migration.
- [Meta-reload ServiceNotFound/packages-reload saga](project_meta_reload_servicenotfound_fix.md) — superseded by [[project_homeassistant_packages_reload_limitation_fix]]'s root cause; keep for historical incident detail only.
- [discovery.yaml truncated-deploy incident](project_discovery_yaml_truncated_deploy_incident.md) — real incident: a near-empty deploy dropped Vienna to raw passthrough for ~8min. Always verify the REMOTE file.
- [suggestions/physical vs suggestions/conceptual split](project_suggestions_physical_conceptual_split.md) — queued 2026-09-17, not started: split "as-is" suggestions from a new declared-but-unpositioned-capability report.
- [Meta-reload simplified to reload_all](project_meta_reload_all_simplification.md) — DONE+DEPLOYED 2026-09-17 both houses; real lesson: a restart does NOT clean up orphaned registry entities (see also the 2026-10-05 orphan audit).
- [Device leaf naming/conceptual mismatch gap](project_device_leaf_naming_conceptual_mismatch_gap.md) — PARTIALLY DONE 2026-09-19: "sphere::path" form fixes several Vienna cases, not deployed; explicit-replacement form still deferred.
- [Logical.def literal-reference ordering bug](project_logical_literal_reference_ordering_bug.md) — a device's own "node" line must come AFTER the entity line positioning any entity it references literally, or a false duplicate-id warning fires.
- [Flat entity-form ban](project_flat_entity_form_ban.md) — queued 2026-09-18: Vienna done, Junglinster 4/5 done, 3 bedroom "health" lines frozen pending Junglinster's own migration.
- [Logical layer "defined"/"derived" capabilities](project_logical_layer_defined_and_derived_capabilities.md) — DONE+VERIFIED 2026-09-18/19: windy/sunny/battery_alert; `over:` resolves siblings + literal entity_ids; cross-device deferred pending ontological design.
- [Passthrough suggestion tracking decoupling](project_passthrough_suggestion_tracking_decoupling.md) — DONE 2026-09-19, NOT YET DEPLOYED: suggestion coupling + stale discovery_passthrough.yaml fixes; coordinator needs redeploy (both houses).
- [Junglinster coordinator crash loop, 2026-09-22](project_junglinster_coordinator_crash_loop_2026_09_22.md) — DONE+FIXED: root cause was synchronous full-manifest JSON persist() on every retirement/relay; needed async-queue routing + debounced persist().
- [HA packages reload limitation fix](project_homeassistant_packages_reload_limitation_fix.md) — DONE+DEPLOYED 2026-09-23: dropped `homeassistant: packages:` (HA never reprocesses packages on reload, upstream wontfix); root cause of several earlier reload incidents.
- [Junglinster stale template entity collision, 2026-09-23](project_junglinster_stale_template_entity_collision_2026_09_23.md) — DONE+FIXED: 21 entities stuck as dead "_2" duplicates behind orphaned pre-migration template entries; registry-surgery precedent.
- [Event Bus operational vs coordinative roles](project_event_bus_operational_vs_coordinative_roles.md) — future architecture: FHEM state re-enforcement needs its own higher-reliability role; 4-way Logical.def situation breakdown.
- [Taps are energy-harvesting, not battery](project_taps_energy_harvesting_unavailable.md) — "unavailable" after disuse is normal; may need repeated presses to recharge and report.
- [Stable entity identity design](project_stable_entity_identity_design.md) — designed 2026-09-29, NOT implemented: Physical.def `as <id>` annotation freezes coordinator<->HA unique_id across hardware swaps/renames/repositions; plan after "do a /clean"; read before any registry-impact-sensitive rename work.
