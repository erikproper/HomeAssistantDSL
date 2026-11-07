# Code review: Go files

Erik's review of the Go code (PROJECT.md item 33), following ../GO_REVIEWING.md. Files are listed
in four states; Claude keeps the list up to date in the same commit as each change.

- **Not ready**: still to be refactored; the step (PROJECT.md 33's order) that makes it ready is
  given per group.
- **Ready for review**: refactored, waiting for Erik.
- **Re-review**: reviewed before, then changed again; the reason says what changed.
- **Reviewed**: done, with the date.

A change to a reviewed file -- also one caused by work on another file -- moves it back to
Re-review, with the reason. A file's test file is reviewed together with it. Erik's findings on a
file are noted under its entry; once they are dealt with, it goes to Re-review.

## Re-review

(none)

## Ready for review

Step 2, the shared parser stack (Architecture.md §9.4). The stack's lower layers will change again
in step 3, when the definition files are read whole (positions in the file, comments); the
grammars on top show the style the other grammars will follow.

- `homeassistant/parser_character_stream.go`
- `homeassistant/parser_tokeniser.go`
- `homeassistant/parser_base.go` (+ parser_base_test.go)
- `homeassistant/parser_messages.go`
- `homeassistant/parser_battery_emulation.go` (tests in battery_emulation_test.go)
- `homeassistant/parser_mqtt_capability.go` (tests in integration_mqtt_test.go)
- `homeassistant/parser_rule.go` (+ parser_rule_test.go; its comments are cleaned in step 6)
- `homeassistant/parser_condition.go` (+ parser_condition_test.go; its comments are cleaned in
  step 6)

## Reviewed

(none)

## Not ready

### Parser front end (steps 2-4: shared parser stack, one grammar per .def file)

- `homeassistant/Physical_DerivedCapability.go` (+ Physical_DerivedCapability_test.go)
- `homeassistant/battery_emulation.go` (+ battery_emulation_test.go)
- `homeassistant/capability_defaults.go` (+ capability_defaults_test.go)
- `homeassistant/def_format.go` (+ def_format_test.go)
- `homeassistant/defined.go` (+ defined_test.go)
- `homeassistant/expander.go` (+ expander_test.go)
- `homeassistant/frozen_unique_ids.go` (+ frozen_unique_ids_test.go)
- `homeassistant/integration_commandline_parser.go`
- `homeassistant/integration_commandline_storage.go`
- `homeassistant/integration_discovery_parser.go` (+ integration_discovery_parser_test.go)
- `homeassistant/integration_discovery_storage.go`
- `homeassistant/integration_hassbridge_parser.go`
- `homeassistant/integration_hassbridge_storage.go`
- `homeassistant/integration_hosts_parser.go`
- `homeassistant/integration_hosts_storage.go`
- `homeassistant/integration_import_parser.go` (+ integration_import_parser_test.go)
- `homeassistant/integration_import_storage.go`
- `homeassistant/integration_local_parser.go`
- `homeassistant/integration_local_storage.go`
- `homeassistant/integration_logical_parser.go` (+ integration_logical_parser_test.go)
- `homeassistant/integration_logical_storage.go` (+ integration_logical_storage_test.go)
- `homeassistant/integration_mqtt.go` (+ integration_mqtt_test.go)
- `homeassistant/layers.go` (+ layers_test.go)
- `homeassistant/parser.go`
- `homeassistant/physical_liveness.go` (+ physical_liveness_test.go)
- `homeassistant/presence.go`
- `homeassistant/with_clause.go`

### Everything else (step 5: mechanical fixes, generator.go split; step 6: comments)

- `homeassistant/Conceptual_CommandlineEntities.go` (+ Conceptual_CommandlineEntities_test.go)
- `homeassistant/Conceptual_DeviceCapabilityEntities.go` (+ Conceptual_DeviceCapabilityEntities_test.go)
- `homeassistant/Conceptual_DeviceEntities.go` (+ Conceptual_DeviceEntities_test.go)
- `homeassistant/Conceptual_DeviceExportRegistration.go` (+ Conceptual_DeviceExportRegistration_test.go)
- `homeassistant/Conceptual_DevicePositioning.go` (+ Conceptual_DevicePositioning_test.go)
- `homeassistant/Conceptual_DeviceSourceEntities.go` (+ Conceptual_DeviceSourceEntities_test.go)
- `homeassistant/Conceptual_DiscoveryEntities.go`
- `homeassistant/Conceptual_LocalEntities.go` (+ Conceptual_LocalEntities_test.go)
- `homeassistant/Conceptual_LogicalEntities.go` (+ Conceptual_LogicalEntities_test.go)
- `homeassistant/Physical_Generator.go` (+ Physical_Generator_test.go)
- `homeassistant/Physical_Parser.go`
- `homeassistant/Physical_Storage.go`
- `homeassistant/administration.go` (+ administration_test.go)
- `homeassistant/bootstrap_stable_ids.go`
- `homeassistant/capability_icon.go` (+ capability_icon_test.go)
- `homeassistant/checks.go` (+ checks_test.go)
- `homeassistant/commandline_yaml_entities.go` (+ commandline_yaml_entities_test.go)
- `homeassistant/conceptual_rule_no_duplicate_final_entity_ids.go` (+ conceptual_rule_no_duplicate_final_entity_ids_test.go)
- `homeassistant/conceptual_rule_no_overlapping_capability_suffixes.go` (+ conceptual_rule_no_overlapping_capability_suffixes_test.go)
- `homeassistant/conceptual_unpositioned_report.go` (+ conceptual_unpositioned_report_test.go)
- `homeassistant/conceptual_volume_sync.go` (+ conceptual_volume_sync_test.go)
- `homeassistant/containers_deploy.go`
- `homeassistant/coordinator_deploy.go`
- `homeassistant/coordinator_yaml_entities.go` (+ coordinator_yaml_entities_test.go)
- `homeassistant/deploy_plan.go` (+ deploy_plan_test.go)
- `homeassistant/deploy_run.go`
- `homeassistant/derived_migration_helper.go` (+ derived_migration_helper_test.go)
- `homeassistant/device_friendly_name.go` (+ device_friendly_name_test.go)
- `homeassistant/discovery_passthrough.go` (+ discovery_passthrough_test.go)
- `homeassistant/discovery_passthrough_suggestions.go` (+ discovery_passthrough_suggestions_test.go)
- `homeassistant/entity_naming.go` (+ entity_naming_test.go)
- `homeassistant/firmware_updates.go` (+ firmware_updates_test.go)
- `homeassistant/generator.go`
- `homeassistant/hassbridge_commands.go` (+ hassbridge_commands_test.go)
- `homeassistant/hassbridge_yaml_entities.go`
- `homeassistant/home_assistant_deploy.go`
- `homeassistant/home_assistant_registry.go` (+ home_assistant_registry_test.go)
- `homeassistant/hosts_yaml_entities.go`
- `homeassistant/imported_yaml_entities.go` (+ imported_yaml_entities_test.go)
- `homeassistant/integration_commandline_deploy.go`
- `homeassistant/integration_commandline_generator.go`
- `homeassistant/integration_discovery_generator.go`
- `homeassistant/integration_hassbridge_generator.go`
- `homeassistant/integration_hosts_deploy.go`
- `homeassistant/integration_hosts_generator.go` (+ integration_hosts_generator_test.go)
- `homeassistant/integration_import_generator.go` (+ integration_import_generator_test.go)
- `homeassistant/logical_enabled_by.go` (+ logical_enabled_by_test.go)
- `homeassistant/logical_enabler_nodes.go` (+ logical_enabler_nodes_test.go)
- `homeassistant/logical_rule_battery_alert_biconditional.go` (+ logical_rule_battery_alert_biconditional_test.go)
- `homeassistant/logical_switched_by.go` (+ logical_switched_by_test.go)
- `homeassistant/main.go` (+ main_test.go)
- `homeassistant/main_entities.go` (+ main_entities_test.go)
- `homeassistant/missing_entities_report.go` (+ missing_entities_report_test.go)
- `homeassistant/mqtt_broker_reachability.go` (+ mqtt_broker_reachability_test.go)
- `homeassistant/mqtt_discovery_cleanup.go` (+ mqtt_discovery_cleanup_test.go)
- `homeassistant/mqtt_discovery_device_info.go` (+ mqtt_discovery_device_info_test.go)
- `homeassistant/mqtt_discovery_existence.go` (+ mqtt_discovery_existence_test.go)
- `homeassistant/mqtt_entity_existence.go` (+ mqtt_entity_existence_test.go)
- `homeassistant/mqtt_import_existence.go` (+ mqtt_import_existence_test.go)
- `homeassistant/mqtt_offline_notice.go`
- `homeassistant/mqtt_routing_keywords.go` (+ mqtt_routing_keywords_test.go)
- `homeassistant/mqtt_yaml_entities.go` (+ mqtt_yaml_entities_test.go)
- `homeassistant/occupied_space_aggregates.go` (+ occupied_space_aggregates_test.go)
- `homeassistant/physical_auto_assign_stable_ids.go` (+ physical_auto_assign_stable_ids_test.go)
- `homeassistant/physical_rule_battery_alert_biconditional.go` (+ physical_rule_battery_alert_biconditional_test.go)
- `homeassistant/physical_rule_derived_capabilities.go` (+ physical_rule_derived_capabilities_test.go)
- `homeassistant/physical_rule_hassbridge_availability_sources.go` (+ physical_rule_hassbridge_availability_sources_test.go)
- `homeassistant/physical_rule_no_conflicting_device_names.go` (+ physical_rule_no_conflicting_device_names_test.go)
- `homeassistant/physical_rule_power_implies_consumes.go` (+ physical_rule_power_implies_consumes_test.go)
- `homeassistant/registry_manifest.go` (+ registry_manifest_test.go)
- `homeassistant/remote_instance_automations.go` (+ remote_instance_automations_test.go)
- `homeassistant/remote_instance_entity_commands.go` (+ remote_instance_entity_commands_test.go)
- `homeassistant/remote_instance_entity_existence.go` (+ remote_instance_entity_existence_test.go)
- `homeassistant/remote_instance_entity_reporting.go` (+ remote_instance_entity_reporting_test.go)
- `homeassistant/remote_instance_integration_health.go` (+ remote_instance_integration_health_test.go)
- `homeassistant/remote_instance_meta_control.go` (+ remote_instance_meta_control_test.go)
- `homeassistant/remote_instance_reload_guard.go` (+ remote_instance_reload_guard_test.go)
- `homeassistant/reporter.go`
- `homeassistant/retain_shadows.go` (+ retain_shadows_test.go)
- `homeassistant/rule_node_capability.go` (+ rule_node_capability_test.go)
- `homeassistant/shared_availability.go` (+ shared_availability_test.go)
- `homeassistant/taxonomy.go` (+ taxonomy_test.go)
- `homeassistant/template_availability.go` (+ template_availability_test.go)
- `homeassistant/would_define.go` (+ would_define_test.go)
- `homeassistant/yaml_reports.go`
- `hamqtt/bridge.go` (+ bridge_test.go)
- `hamqtt/fields.go` (+ fields_test.go)
- `hamqtt/imported.go`
- `hamqtt/relay.go`
- `house_event_bus_coordinator/discover_entity_input.go` (+ discover_entity_input_test.go)
- `house_event_bus_coordinator/discovery.go` (+ discovery_test.go)
- `house_event_bus_coordinator/discovery_device_info.go` (+ discovery_device_info_test.go)
- `house_event_bus_coordinator/discovery_existence.go` (+ discovery_existence_test.go)
- `house_event_bus_coordinator/discovery_passthrough_devices.go` (+ discovery_passthrough_devices_test.go)
- `house_event_bus_coordinator/discovery_relay_queue.go` (+ discovery_relay_queue_test.go)
- `house_event_bus_coordinator/discoverybridge.go` (+ discoverybridge_test.go)
- `house_event_bus_coordinator/discoverycleanup.go` (+ discoverycleanup_test.go)
- `house_event_bus_coordinator/discoveryhassbridge.go` (+ discoveryhassbridge_test.go)
- `house_event_bus_coordinator/discoveryimport.go` (+ discoveryimport_test.go)
- `house_event_bus_coordinator/entity_existence.go` (+ entity_existence_test.go)
- `house_event_bus_coordinator/home_assistant_instances.go`
- `house_event_bus_coordinator/import_existence.go` (+ import_existence_test.go)
- `house_event_bus_coordinator/instance_liveness.go` (+ instance_liveness_test.go)
- `house_event_bus_coordinator/liveinfo.go` (+ liveinfo_test.go)
- `house_event_bus_coordinator/liveness.go`
- `house_event_bus_coordinator/main.go` (+ main_test.go)
- `house_event_bus_coordinator/main_entities.go`
- `house_event_bus_coordinator/meta_reload_restart.go` (+ meta_reload_restart_test.go)
- `house_event_bus_coordinator/metadata_topics.go` (+ metadata_topics_test.go)
- `house_event_bus_coordinator/missing_declared_entities.go` (+ missing_declared_entities_test.go)
- `house_event_bus_coordinator/mqtt.go`
- `house_event_bus_coordinator/mqtt_relay.go` (+ mqtt_relay_test.go)
- `house_event_bus_coordinator/retain_shadows.go` (+ retain_shadows_test.go)
- `house_event_bus_coordinator/secrets.go`
- `house_event_bus_coordinator/systemd_notify.go`
- `mqtt_commandline/main.go` (+ main_test.go)

### Test files without a file of the same name

- `homeassistant/Conceptual_DerivedEvents_test.go`
- `homeassistant/Conceptual_ImportedDevicePositioning_test.go`
- `homeassistant/defined_home_assistant_instances_test.go`
- `homeassistant/generator_cover_away_test.go`
- `homeassistant/generator_media_switch_test.go`
- `homeassistant/hosts_metadata_test.go`
- `homeassistant/integration_commandline_test.go`
- `homeassistant/integration_hassbridge_attribute_test.go`
- `homeassistant/integration_hassbridge_test.go`
- `homeassistant/integration_hassbridge_valuemap_test.go`
- `homeassistant/integration_hassbridge_with_oneliner_test.go`
- `homeassistant/integration_local_test.go`
- `homeassistant/logical_when_rule_test.go`
- `homeassistant/mqtt_broker_profiles_test.go`
- `homeassistant/occupied_space_aggregates_media_test.go`
- `homeassistant/remote_instance_integration_node_test.go`
- `homeassistant/rule_parser_phase2_test.go`
- `homeassistant/sensor_subdomain_groups_test.go`
- `house_event_bus_coordinator/mqtt_cloud_suggested_area_test.go`
- `house_event_bus_coordinator/mqtt_reconnect_hook_test.go`
- `house_event_bus_coordinator/mqtt_resubscribe_test.go`
- `house_event_bus_coordinator/testhelpers_test.go`
