/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: RemoteInstanceAutomations
 *
 * Writes the coordinator's MQTT-driven "bootstrap" automations -- report_entities_detailed/reload
 * (mqtt_entity_catalogue.go's contract, the generator's own live entity fetch) and, for a bridge
 * source instance, capability-value reporting (house_event_bus_coordinator/discoveryhassbridge.go's
 * contract) -- for every "home_assistant <qualifier>: <name>;" instance Physical.def declares.
 * The plain "report_entities" (bare entity_id list) automation and its consumer (the coordinator's
 * assumed-entities discrepancy check, discoveryassumed.go) were removed 2026-08-27 -- redundant
 * once the generator itself could read the live, richer catalogue directly at ./generate time.
 * "main" is the
 * one instance this generator already produces and deploys a full hass/<incarnation>/ tree for, so
 * its automations land straight into that tree via writeAutomationFile, same as every other
 * generated automation. Every other named instance (e.g. protocols-server-2) is a genuinely
 * separate HA installation this generator has no deploy path to -- but it still gets its own
 * hass/<qualifier>/ tree with the skeleton around the automations that main's tree has
 * (configuration.yaml + integrations/automation.yaml, both needed for HA to actually pick up
 * anything under automation/ via !include_dir_merge_list), scoped down to just what this
 * generator actually produces for that instance -- e.g. no "customize:" line in
 * configuration.yaml, since there's no customization/ directory to go with it. The only manual
 * step left is copying that tree onto the remote instance's own config and reloading, not
 * figuring out what belongs in it or how it's wired together.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 23.08.2026
 *
 */

package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// entitiesDetailedPayloadTemplate is the Jinja expression the "report entities detailed"
// automation publishes -- a JSON array of {entity_id, device_id, device_name, area_name} per
// entity, read by homeassistant/mqtt_entity_catalogue.go's device-grouping suggestion tool. Built
// with a namespace-accumulator for-loop (HA's own documented pattern for
// building a list of dicts in a template) rather than a Python-style list comprehension --
// deliberately the more verbose but unambiguously-supported form, since this can't be live-tested
// against a real instance before being generated. All tags use "-%}"/"{%-" whitespace control and
// the whole thing is written on one line with no inter-tag whitespace at all, so the rendered
// payload is exactly the trailing {{ }} expression's JSON, nothing else, regardless of the
// instance's own trim_blocks/lstrip_blocks Jinja settings. device_id(...) returns none for an
// entity with no owning device, guarded before calling device_attr on it.
const entitiesDetailedPayloadTemplate = `{% set ns = namespace(items = []) %}{% for s in states %}{% set did = device_id(s.entity_id) %}{% set ns.items = ns.items + [{'entity_id': s.entity_id, 'device_id': did, 'device_name': device_attr(did, 'name') if did else none, 'area_name': area_name(s.entity_id)}] %}{% endfor %}{{ ns.items | tojson }}`

// bootstrapAutomationBody returns the "report entities detailed on command" / "reload on
// command" automation pair every named instance needs, fully substituted for name -- no header,
// callers prepend generatorHeader themselves (writeAutomationFile writes whatever content it's
// given verbatim). The report automation also fires on the "reload" command and on HA's own
// startup event -- otherwise a fresh report only ever happens when someone manually publishes
// the command, leaving stale data across an instance's own reload/restart even though that's
// exactly when its entity set is most likely to have changed.
func bootstrapAutomationBody(name string) string {
	var sb strings.Builder
	sb.WriteString("- alias: \"Coordinator bridge: report entities detailed\"\n")
	sb.WriteString("  id: automation.coordinator_bootstrap_report_detailed_" + name + "\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: mqtt\n")
	sb.WriteString("    topic: \"homeassistant_instances/" + name + "/command\"\n")
	sb.WriteString("    payload: \"report_entities_detailed\"\n")
	sb.WriteString("  - platform: mqtt\n")
	sb.WriteString("    topic: \"homeassistant_instances/" + name + "/command\"\n")
	sb.WriteString("    payload: \"reload\"\n")
	sb.WriteString("  - platform: homeassistant\n")
	sb.WriteString("    event: start\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: mqtt.publish\n")
	sb.WriteString("    data:\n")
	sb.WriteString("      topic: \"homeassistant_instances/" + name + "/entities_detailed\"\n")
	sb.WriteString("      retain: true\n")
	sb.WriteString("      payload: \"" + entitiesDetailedPayloadTemplate + "\"\n")
	sb.WriteString("- alias: \"Coordinator bridge: reload\"\n")
	sb.WriteString("  id: automation.coordinator_bootstrap_reload_" + name + "\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: mqtt\n")
	sb.WriteString("    topic: \"homeassistant_instances/" + name + "/command\"\n")
	sb.WriteString("    payload: \"reload\"\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: automation.reload\n")
	return sb.String()
}

// bareEntityFromSource strips a source specification's "!attribute" or " is available" suffix
// (sourceToJinja2's own two sugars), leaving the plain HA entity_id a state trigger can watch --
// entity_id lists (unlike sourceToJinja2's Jinja output) have no room for a compound expression.
func bareEntityFromSource(src string) string {
	if entityID, ok := strings.CutSuffix(src, " is available"); ok {
		return entityID
	}
	if bangIdx := strings.Index(src, "!"); bangIdx > 0 {
		return src[:bangIdx]
	}
	return src
}

// THassBridgeDeviceInfoReport is one device's dynamic device-info fields (device.DeviceInfoCapabilities,
// integration_hassbridge_storage.go) ready for the reporting automation -- unlike capability
// values, these all publish together as one JSON object per device (mirroring hosts devices' own
// DeviceInfoTopic convention), not one message per field.
type THassBridgeDeviceInfoReport struct {
	DeviceID        string
	Fields          map[string]string // name -> raw source ("<entity>[!<attribute>]")
	LiteralPrefixes map[string]string // name -> literal prefix, if declared
}

// deviceInfoFieldExpr builds one device-info field's Jinja value expression, prepending its
// declared literal prefix (if any) as a Jinja string concatenation -- same convention
// integration_hosts_generator.go's own capabilityExpr already uses for hosts device-info fields.
func deviceInfoFieldExpr(report THassBridgeDeviceInfoReport, name string) string {
	expr := sourceToJinja2(report.Fields[name])
	if prefix, hasPrefix := report.LiteralPrefixes[name]; hasPrefix {
		expr = jinjaStringLiteral(prefix) + " ~ " + expr
	}
	return expr
}

// hassBridgeReportingAutomationBody returns the "publish each device's dynamic device-info fields
// as one combined JSON object" automation for a bridge source instance. Per-capability (entity)
// reporting moved to its own per-entity automation (generateHassBridgeEntityReportingAutomations,
// remote_instance_entity_reporting.go, PROJECT.md 1.1) -- device-info fields stay combined here
// since they're not individually positioned DSL entities, just device-map metadata. Triggers on
// the deduplicated set of *bare* underlying entities every device-info field's source depends on
// (a state trigger can't watch a compound "entity!attribute"/"entity is available" expression
// directly, only the plain entity whose changes should re-evaluate it). deviceInfoReports must be
// pre-sorted by the caller for deterministic output.
func hassBridgeReportingAutomationBody(name string, deviceInfoReports []THassBridgeDeviceInfoReport) string {
	var sb strings.Builder
	sb.WriteString("- alias: \"Coordinator bridge: report device info\"\n")
	sb.WriteString("  id: automation.coordinator_bridge_report_" + name + "\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: state\n")
	sb.WriteString("    entity_id:\n")
	seenBare := map[string]bool{}
	var bareEntities []string
	addBare := func(source string) {
		bare := bareEntityFromSource(source)
		if !seenBare[bare] {
			seenBare[bare] = true
			bareEntities = append(bareEntities, bare)
		}
	}
	for _, report := range deviceInfoReports {
		fieldNames := make([]string, 0, len(report.Fields))
		for fieldName := range report.Fields {
			fieldNames = append(fieldNames, fieldName)
		}
		sort.Strings(fieldNames)
		for _, fieldName := range fieldNames {
			addBare(report.Fields[fieldName])
		}
	}
	sort.Strings(bareEntities)
	for _, entity := range bareEntities {
		sb.WriteString("    - " + entity + "\n")
	}
	sb.WriteString("  action:\n")
	for _, report := range deviceInfoReports {
		fieldNames := make([]string, 0, len(report.Fields))
		for fieldName := range report.Fields {
			fieldNames = append(fieldNames, fieldName)
		}
		sort.Strings(fieldNames)
		sb.WriteString("  - service: mqtt.publish\n")
		sb.WriteString("    data:\n")
		sb.WriteString("      topic: \"homeassistant_instances/" + name + "/bridge/device/" + report.DeviceID + "/state\"\n")
		sb.WriteString("      retain: true\n")
		sb.WriteString("      payload: \"{{ {")
		for i, fieldName := range fieldNames {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("'" + fieldName + "': " + deviceInfoFieldExpr(report, fieldName))
		}
		sb.WriteString("} | tojson }}\"\n")
	}
	return sb.String()
}

// collectHassBridgeDeviceInfoByInstance groups every positioned hassbridge device's dynamic
// device-info fields by the named instance they're bridged from -- unpositioned devices (no
// DeviceConceptualLinks entry) and devices with no DeviceInfoCapabilities declared contribute
// nothing.
func collectHassBridgeDeviceInfoByInstance(hassBridgeDevicesByID map[string]THassBridgeDevice, admin *TAdministrationState) map[string][]THassBridgeDeviceInfoReport {
	byInstance := map[string][]THassBridgeDeviceInfoReport{}
	for deviceID, device := range hassBridgeDevicesByID {
		if _, hasLink := admin.DeviceConceptualLinks[deviceID]; !hasLink {
			continue
		}
		if len(device.DeviceInfoCapabilities) == 0 {
			continue
		}
		for _, instance := range device.Instances {
			byInstance[instance] = append(byInstance[instance], THassBridgeDeviceInfoReport{
				DeviceID:        deviceID,
				Fields:          device.DeviceInfoCapabilities,
				LiteralPrefixes: device.DeviceInfoLiteralPrefixes,
			})
		}
	}
	for name := range byInstance {
		sort.Slice(byInstance[name], func(i, j int) bool {
			return byInstance[name][i].DeviceID < byInstance[name][j].DeviceID
		})
	}
	return byInstance
}

// automationIntegrationDef is integrationDefs' own "automation.yaml" entry (generator.go) --
// looked up rather than duplicated, so a remote instance's skeleton can't drift from main's.
func automationIntegrationDef() (file, content string) {
	for _, def := range integrationDefs {
		if def.file == "automation.yaml" {
			return def.file, def.content
		}
	}
	panic("integrationDefs has no \"automation.yaml\" entry")
}

// remoteInstanceConfigurationYAMLBody is configurationYAMLBody (generator.go) minus the
// "customize: !include_dir_merge_named customization" line -- unlike main's tree, a remote
// instance's tree has no customization/ directory at all (this generator produces no
// customization content for it), so the reference would point at a directory that never exists.
//
// Deliberately never carries an "http:" block, even for an instance behind a reverse proxy
// (trusted_proxies/use_x_forwarded_for) -- tried that (2026-08-26) and found Home Assistant has
// migrated the http integration's config to a UI-managed config entry; any "http:" block in
// configuration.yaml is silently ignored (removed outright from 2027.2.0). That setting has to be
// made via Settings > System > Network in the UI, not generated -- not something this generator
// can own.
const remoteInstanceConfigurationYAMLBody = "default_config:\n\nhomeassistant:\n  packages: !include_dir_named integrations\n"

// writeRemoteInstanceSkeleton writes the minimal skeleton a remote instance's tree needs around
// its automation/ content to actually be includable by HA: configuration.yaml (scoped to what
// this generator actually produces for a remote instance -- packages only, no customize line) and
// integrations/automation.yaml alone (not the full integrationDefs set main's tree has, since
// this generator produces no other domain's content for a remote instance).
func writeRemoteInstanceSkeleton(instanceOutputDir string) error {
	if err := writeYAMLFile(filepath.Join(instanceOutputDir, "configuration.yaml"), generatorHeader+remoteInstanceConfigurationYAMLBody); err != nil {
		return err
	}
	file, content := automationIntegrationDef()
	return writeYAMLFile(filepath.Join(instanceOutputDir, "integrations", file), generatorHeader+content+"\n")
}

// generateInstanceAutomationTrees writes every declared instance's coordinator-bootstrap
// automations. "main" writes into haOutputDir (the tree already deployed by the rest of this
// generation run, whose configuration.yaml/integrations/ skeleton is already generated
// separately). Every other instance gets a sibling hass/<qualifier>/ tree, with its own
// automation-scoped skeleton (writeRemoteInstanceSkeleton) around it -- wiped and rewritten each
// run the same way the main tree is (nothing else writes there today, so a stale file from a
// removed capability/instance would otherwise linger). No-op when haOutputDir is empty (no
// incarnation resolved) or no instance is declared at all.
func generateInstanceAutomationTrees(haOutputDir string, instances map[string]THomeAssistantInstance, hassBridgeDevicesByID map[string]THassBridgeDevice, admin *TAdministrationState) error {
	if haOutputDir == "" || len(instances) == 0 {
		return nil
	}
	hassRootDir := filepath.Dir(haOutputDir)
	deviceInfoByInstance := collectHassBridgeDeviceInfoByInstance(hassBridgeDevicesByID, admin)

	names := make([]string, 0, len(instances))
	for name := range instances {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		instanceHAOutputDir := haOutputDir
		if name != "main" {
			instanceHAOutputDir = filepath.Join(hassRootDir, name)
			if err := os.RemoveAll(instanceHAOutputDir); err != nil {
				return err
			}
			if err := writeRemoteInstanceSkeleton(instanceHAOutputDir); err != nil {
				return err
			}
		}

		// coordinator_bootstrap generation is disabled for now (2026-08-27): its "report entities
		// detailed" automation (entitiesDetailedPayloadTemplate, a synchronous {% for s in states
		// %} loop over every entity, calling device_id()/device_attr()/area_name() per entity)
		// hung "main" outright when forced -- fine at protocols-server-2's scale, not at main's.
		// PROJECT.md 1.1 (entity-existence inquiry & optimistic generation, 2026-08-28 -- see
		// memory: project_entity_existence_inquiry_design) replaces this; re-enable (or delete)
		// bootstrapAutomationBody once the coordinator side of 1.1 lands, not before. A fresh
		// ./generate run wipes this file from any previously-deployed tree, so simply not writing
		// it here is enough to retire it on next deploy -- no separate cleanup needed.
		if err := generateHassBridgeEntityReportingAutomations(instanceHAOutputDir, name, hassBridgeDevicesByID, admin); err != nil {
			return err
		}
		if deviceInfoReports, hasDeviceInfo := deviceInfoByInstance[name]; hasDeviceInfo {
			if err := writeAutomationFile(instanceHAOutputDir, "infrastructural", "coordinator_bridge_report", generatorHeader+hassBridgeReportingAutomationBody(name, deviceInfoReports)); err != nil {
				return err
			}
		}
		// Written unconditionally, for every declared instance, regardless of whether it has
		// hassbridge devices positioned yet -- entity-existence inquiry (PROJECT.md 1.1) applies
		// to any HA incarnation, including "main" itself, not just remote bridge sources. Safe at
		// any instance size (a single states[...] lookup per inquiry, never a full scan) -- see
		// this automation's own doc comment for why that's the point.
		if err := writeAutomationFile(instanceHAOutputDir, "infrastructural", "coordinator_bridge_inquire", generatorHeader+entityExistenceInquiryAutomationBody(name)); err != nil {
			return err
		}
		// PROJECT.md item 2: reload/restart meta-commands, unconditional for every declared
		// instance exactly like the inquiry automation above -- no Physical.def grammar needed.
		if err := writeAutomationFile(instanceHAOutputDir, "infrastructural", "coordinator_meta_reload", generatorHeader+metaReloadAutomationBody(name)); err != nil {
			return err
		}
		if err := writeAutomationFile(instanceHAOutputDir, "infrastructural", "coordinator_meta_restart", generatorHeader+metaRestartAutomationBody(name)); err != nil {
			return err
		}
	}
	return nil
}
