/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: BatteryEmulation
 *
 * PROJECT.md 31b (2026-11-06, Erik): a battery level emulated from the days since the batteries
 * were replaced, for rechargeable (Li-ion) batteries, which keep their voltage until almost empty,
 * so that the level a device reports says little (Junglinster's dining Philips dimmer reported ~1 %
 * for six months while working fine). A physical capability: an emulation one Home Assistant
 * instance runs (normally ha2mqtt), bridged to main as MQTT entities of the device itself. In
 * Physical.def, once:
 *
 *   battery level emulation on ha2mqtt;
 *
 * and in a device's block (any integration):
 *
 *   battery level emulation as unique_id <root> lasting 90 days since 31.10.2026 with alert at 5%;
 *
 * which gives the device its battery_level and battery_alert (positioned in Conceptual.def as
 * usual), plus three entities next to the level: the replacement date ("batteries_replaced"), the
 * duration a charge is assumed to last ("battery_duration") and a button setting the date to today
 * ("batteries_replaced_today"). "lasting" and "since" are the starting values, set once by the
 * emulating instance (a hidden "seeded" flag); after that the date and duration are edited on main.
 * The level is 100 * (X - D) / X percent (X the duration, D the days since replacement, kept between
 * 0 and 100), the alert on below the given percentage. The device's own reading, wherever it comes
 * from, is only declared when wanted -- as battery_level_raw / battery_alert_raw; an OR with it is a
 * Logical.def battery_alert.
 *
 * On the emulating instance: the helpers, the level and alert templates, and automations seeding
 * the helpers, reporting the five states on the bridge's topics (homeassistant.instances/<instance>/
 * bridge/<entity>/state) and carrying out main's commands. On main: MQTT entities of the device,
 * built as every bridged entity is (hamqtt.BridgeEntityConfig). The unique_ids are rooted in the
 * statement's frozen root.
 *
 * The statements are parsed by a CDL1-style parser (GO_CONVENTIONS.md §8) on the rule grammar's
 * tokeniser (rule_tokeniser.go), and removed from Physical.def's content before its integration
 * parsers see it (resolveDerivedConditionOneLiners), as the frozen IDs are.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 06.11.2026
 *
 */

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"homeassistant/hamqtt"

	"gopkg.in/yaml.v3"
)

// TBatteryEmulation is one device's "battery level emulation ...;": RootID roots its entities'
// unique_ids, Days and Since (yyyy-mm-dd, "" for the day it is first seeded) are the starting
// values, AlertPercent the level below which the alert is on.
type TBatteryEmulation struct {
	DeviceID     string
	RootID       string
	Days         int
	Since        string
	AlertPercent int
}

// TBatteryEmulationEntities are an emulation's entities on main, as positioned ("" until then).
type TBatteryEmulationEntities struct {
	Level, Alert, Date, Duration, Reset string
}

// batteryEmulationDefaultDays is the duration a charge is assumed to last without "lasting";
// batteryResetPayload is what main's button sends (MQTT button's own default).
const (
	batteryEmulationDefaultDays = 90
	batteryResetPayload         = "PRESS"
)

// The segments of the entities next to the level.
const (
	batteryReplacedSegment = "batteries_replaced"
	batteryDurationSegment = "battery_duration"
	batteryResetSegment    = "batteries_replaced_today"
)

// declaredBatteryEmulationInstance and declaredBatteryEmulations are Physical.def's emulation
// statements, read before Conceptual.def is positioned (collectBatteryEmulations), as
// generatedUniqueIDs is a generation's own bookkeeping.
var (
	declaredBatteryEmulationInstance string
	declaredBatteryEmulations        = map[string]TBatteryEmulation{}
)

// --- parser ---

// TBatteryEmulationParser embeds TDefTokeniser (chaining the layers, GO_CONVENTIONS.md §7).
type TBatteryEmulationParser struct {
	TDefTokeniser
	Instance  string
	Emulation TBatteryEmulation
	Warnings  []string
}

// parseBatteryEmulationLine parses line as an emulation statement: instance is set for "... on
// <instance>;", else emulation. ok is false when it isn't one; warnings say what is wrong with one
// that is.
func parseBatteryEmulationLine(line string) (instance string, emulation TBatteryEmulation, ok bool, warnings []string) {
	p := &TBatteryEmulationParser{Emulation: TBatteryEmulation{Days: batteryEmulationDefaultDays}}
	p.Initialise(line)
	ok = p.EmulationStatement()
	return p.Instance, p.Emulation, ok, p.Warnings
}

// ReportParsingError records a warning and returns true, for the right of a "||" chain.
func (p *TBatteryEmulationParser) ReportParsingError(format string, args ...any) bool {
	p.Warnings = append(p.Warnings, "battery level emulation: "+fmt.Sprintf(format, args...))
	return true
}

// Fail always returns false: the end of a "||" error chain.
func (p *TBatteryEmulationParser) Fail() bool {
	return false
}

// ForcedThisWordWas requires the current token to be exactly word, consuming it.
func (p *TBatteryEmulationParser) ForcedThisWordWas(word string) bool {
	return p.ThisWordWas(word) ||
		p.ReportParsingError("expected %q", word) && p.Fail()
}

// ForcedThisPunctuationWas requires the current token to be exactly ch, consuming it.
func (p *TBatteryEmulationParser) ForcedThisPunctuationWas(ch byte) bool {
	return p.ThisPunctuationWas(ch) ||
		p.ReportParsingError("expected %q", string(ch)) && p.Fail()
}

// ForcedEnd requires ";" and nothing after it.
func (p *TBatteryEmulationParser) ForcedEnd() bool {
	return p.ForcedThisPunctuationWas(';') &&
		(p.AtEndOfTokens() || p.ReportParsingError("unexpected text after \";\"") && p.Fail())
}

// EmulationStatement := "battery" "level" "emulation" ( "on" Word ";" | Details ).
func (p *TBatteryEmulationParser) EmulationStatement() bool {
	if !(p.ThisWordWas("battery") && p.ThisWordWas("level") && p.ThisWordWas("emulation")) {
		return false
	}
	if p.ThisWordWas("on") {
		return (p.ThisWord(&p.Instance) || p.ReportParsingError("expected the emulating Home Assistant instance") && p.Fail()) &&
			p.ForcedEnd()
	}
	return p.Details()
}

// Details := [ "as" "unique_id" Id ] [ "lasting" Days "days" ] [ "since" Date ] "with" "alert" "at"
// Percent "%" ";".
func (p *TBatteryEmulationParser) Details() bool {
	return p.AssignedID() && p.Lasting() && p.Since() &&
		p.ForcedThisWordWas("with") && p.ForcedThisWordWas("alert") && p.ForcedThisWordWas("at") &&
		p.Percent() && p.ForcedThisPunctuationWas('%') && p.ForcedEnd()
}

// AssignedID := [ "as" "unique_id" Word { "." Word } ].
func (p *TBatteryEmulationParser) AssignedID() bool {
	if !p.ThisWordWas("as") {
		return true
	}
	if !p.ForcedThisWordWas("unique_id") || !(p.ThisWord(&p.Emulation.RootID) || p.ReportParsingError("expected the frozen unique_id root") && p.Fail()) {
		return false
	}
	var segment string
	for p.ThisPunctuationWas('.') {
		if !p.ThisWord(&segment) {
			return p.ReportParsingError("expected the rest of the unique_id") && p.Fail()
		}
		p.Emulation.RootID += "." + segment
	}
	return true
}

// Lasting := [ "lasting" Number "days" ].
func (p *TBatteryEmulationParser) Lasting() bool {
	if !p.ThisWordWas("lasting") {
		return true
	}
	return p.Number(&p.Emulation.Days, 1, "the number of days a charge lasts") && p.ForcedThisWordWas("days")
}

// Since := [ "since" Day "." Month "." Year ], a date that exists.
func (p *TBatteryEmulationParser) Since() bool {
	if !p.ThisWordWas("since") {
		return true
	}
	var day, month, year string
	if !(p.ThisWord(&day) && p.ForcedThisPunctuationWas('.') && p.ThisWord(&month) && p.ForcedThisPunctuationWas('.') && p.ThisWord(&year)) {
		return p.ReportParsingError("expected a date, dd.mm.yyyy") && p.Fail()
	}
	date, err := time.Parse("2.1.2006", day+"."+month+"."+year)
	if err != nil {
		return p.ReportParsingError("%s.%s.%s is no date", day, month, year) && p.Fail()
	}
	p.Emulation.Since = date.Format("2006-01-02")
	return true
}

// Percent := a whole number from 1 to 99.
func (p *TBatteryEmulationParser) Percent() bool {
	return p.Number(&p.Emulation.AlertPercent, 1, "the alert's percentage") &&
		(p.Emulation.AlertPercent < 100 || p.ReportParsingError("%d%% is no alert level", p.Emulation.AlertPercent) && p.Fail())
}

// Number reads a whole number of at least minimum into *target; what names it in an error.
func (p *TBatteryEmulationParser) Number(target *int, minimum int, what string) bool {
	var word string
	if !p.ThisWord(&word) {
		return p.ReportParsingError("expected %s", what) && p.Fail()
	}
	number, err := strconv.Atoi(word)
	if err != nil || number < minimum {
		return p.ReportParsingError("%q is no valid %s", word, what) && p.Fail()
	}
	*target = number
	return true
}

// --- Physical.def ---

// isBatteryEmulationStatement reports whether line is one, well-formed or not.
func isBatteryEmulationStatement(line string) bool {
	_, _, ok, warnings := parseBatteryEmulationLine(strings.TrimSpace(line))
	return ok || len(warnings) > 0
}

// stripBatteryEmulationStatements blanks every emulation statement in content, one line for one
// line, so none of Physical.def's integration parsers sees it.
func stripBatteryEmulationStatements(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if isBatteryEmulationStatement(line) {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

// collectBatteryEmulations reads definitionDir's Physical.def for the emulating instance and every
// device's emulation.
func collectBatteryEmulations(definitionDir string) (instance string, emulations map[string]TBatteryEmulation, warnings []string) {
	content, _, _ := collectLayerContent(definitionDir, []string{"Physical.def"}, LayerPhysical)
	emulations = map[string]TBatteryEmulation{}
	deviceID := ""
	for _, line := range strings.Split(content, "\n") {
		if m := physicalDeviceHeaderPattern.FindStringSubmatch(line); m != nil {
			deviceID = m[1]
			continue
		}
		on, emulation, ok, w := parseBatteryEmulationLine(strings.TrimSpace(line))
		warnings = append(warnings, w...)
		switch {
		case !ok:
		case on != "":
			if instance != "" && instance != on {
				warnings = append(warnings, fmt.Sprintf("battery level emulation on both %q and %q -- keeping %q", instance, on, instance))
				continue
			}
			instance = on
		case deviceID == "":
			warnings = append(warnings, "battery level emulation outside a device's block -- ignored")
		default:
			emulation.DeviceID = deviceID
			emulations[deviceID] = emulation
		}
	}
	if len(emulations) > 0 && instance == "" {
		warnings = append(warnings, "devices have a battery level emulation, but nothing runs it: add \"battery level emulation on <instance>;\" (normally ha2mqtt)")
	}
	return instance, emulations, warnings
}

// hasBatteryEmulationCapability reports whether label is a capability deviceID's emulation gives.
func hasBatteryEmulationCapability(deviceID, label string) bool {
	_, emulated := declaredBatteryEmulations[deviceID]
	return emulated && (label == "battery_level" || label == "battery_alert")
}

// --- positioning ---

// batteryEmulationEntitiesOf returns deviceID's emulation entities, created on first use.
func (state *TAdministrationState) batteryEmulationEntitiesOf(deviceID string) *TBatteryEmulationEntities {
	if state.BatteryEmulationEntities == nil {
		state.BatteryEmulationEntities = map[string]*TBatteryEmulationEntities{}
	}
	if state.BatteryEmulationEntities[deviceID] == nil {
		state.BatteryEmulationEntities[deviceID] = &TBatteryEmulationEntities{}
	}
	return state.BatteryEmulationEntities[deviceID]
}

// registerBatteryEmulationEntity positions an emulation's level or alert; the level brings the
// date, duration and button, next to it.
func registerBatteryEmulationEntity(administration *TAdministrationState, decl TDeviceCapabilityEntityDeclaration, label, deviceNamePath, entitiesPath string, lineNum int) []string {
	provenance := fmt.Sprintf("%s:%d → %s from %s %s", filepath.Base(entitiesPath), lineNum, decl.LocalSpec, decl.DeviceID, decl.Capability)
	fullName := resolveDeviceEntityFullName(decl.LocalSpec, administration.SpacePath, deviceNamePath)
	identity := extractEntityIdentity(fullName)
	entityID := toHomeAssistantEntityID(fullName)
	want := map[string]string{"battery_level": "sensor", "battery_alert": "binary_sensor"}[label]
	if identity.Domain != want || entityID == "" {
		return []string{fmt.Sprintf("%s: an emulated %s is a %s, not %q", provenance, label, want, decl.LocalSpec)}
	}
	spaceName := administration.CurrentSpaceName()
	record := func(name, note string) string {
		administration.AppendEntityRecord(spaceName, TEntityRecord{
			Name: name, Identity: extractEntityIdentity(name), Provenance: provenance + note, HasDefinitionOrImport: true,
		})
		return toHomeAssistantEntityID(name)
	}
	entities := administration.batteryEmulationEntitiesOf(decl.DeviceID)
	if label == "battery_alert" {
		entities.Alert = record(fullName, "")
		return nil
	}
	entities.Level = record(fullName, "")
	parent := stripLastPathSegment(identity).Path
	sibling := func(domain, segment string) string {
		if parent == "" {
			return domain + "." + identity.Sphere + "/" + segment
		}
		return domain + "." + identity.Sphere + "/" + parent + "/" + segment
	}
	entities.Date = record(sibling("date", batteryReplacedSegment), " (its replacement date)")
	entities.Duration = record(sibling("number", batteryDurationSegment), " (the days a charge lasts)")
	entities.Reset = record(sibling("button", batteryResetSegment), " (sets its replacement date to today)")
	return nil
}

// --- main: the device's MQTT entities ---

// batteryEmulationUniqueIDs are the unique_ids of an emulation's entities on main, by entity_id.
func batteryEmulationUniqueIDs(emulation TBatteryEmulation, entities *TBatteryEmulationEntities) map[string]string {
	root := emulation.RootID
	if root == "" {
		root = strings.ReplaceAll(emulation.DeviceID, ".", "_") + "_battery_emulation"
	}
	ids := map[string]string{}
	for entityID, suffix := range map[string]string{entities.Level: "", entities.Alert: "_alert", entities.Date: "_replaced", entities.Duration: "_duration", entities.Reset: "_reset"} {
		if entityID != "" {
			ids[entityID] = root + suffix
		}
	}
	return ids
}

// writeBatteryEmulationMainEntities writes, under haOutputDir, every emulation's entities on main
// as YAML "mqtt:" entities of the device itself -- its device block taken from one of the device's
// other YAML "mqtt:" entities -- and records their unique_ids for the registry manifest. Run after
// every other writer, before the naming pass (nameYAMLMQTTEntities).
func writeBatteryEmulationMainEntities(haOutputDir string, admin *TAdministrationState) []string {
	if haOutputDir == "" || len(admin.BatteryEmulationEntities) == 0 {
		return nil
	}
	instance := declaredBatteryEmulationInstance
	devices := batteryEmulationDeviceBlocks(haOutputDir, admin)
	admin.BatteryEmulationUniqueIDs = map[string]string{}
	var reports []string
	for _, deviceID := range sortedMapKeys(admin.BatteryEmulationEntities) {
		entities := admin.BatteryEmulationEntities[deviceID]
		emulation := declaredBatteryEmulations[deviceID]
		device, found := devices[deviceID]
		if !found {
			reports = append(reports, fmt.Sprintf("%s: battery level emulation, but the device has no YAML mqtt entity to take its device block from -- left out", deviceID))
			continue
		}
		if entities.Level == "" {
			reports = append(reports, fmt.Sprintf("%s: battery level emulation, but its battery_level isn't positioned -- left out", deviceID))
			continue
		}
		uniqueIDs := batteryEmulationUniqueIDs(emulation, entities)
		node := []string{instanceStatusTopic(instance)}
		bodies := map[string]hamqtt.BridgeEntity{
			entities.Level: {Capability: "battery_level"},
			entities.Date: {Capability: batteryReplacedSegment, CommandsEnabled: true,
				Commands: map[string]hamqtt.Command{"set_value": {ValueTrigger: true}}},
			entities.Duration: {Capability: batteryDurationSegment, DeviceClass: "duration", Unit: "d", CommandsEnabled: true,
				Commands:       map[string]hamqtt.Command{"set_value": {ValueTrigger: true}},
				DiscoveryExtra: map[string]interface{}{"min": 1, "max": 3650, "step": 1, "mode": "box"}},
			entities.Reset: {Capability: batteryResetSegment, Icon: "mdi:battery-sync", CommandsEnabled: true,
				Commands: map[string]hamqtt.Command{"press": {Payload: batteryResetPayload, DiscoveryKey: "payload_press"}}},
		}
		if entities.Alert != "" {
			bodies[entities.Alert] = hamqtt.BridgeEntity{Capability: "battery_alert"}
		}
		for _, entityID := range sortedMapKeys(bodies) {
			entity := bodies[entityID]
			domain := entityID[:strings.Index(entityID, ".")]
			identity, _ := entityIdentityByID(admin, entityID)
			if entity.DeviceClass == "" && entity.Unit == "" {
				entity.DeviceClass, entity.Unit, entity.StateClass, _ = resolveCapabilityDefaults(admin.CapabilityDefaults, domain, identity.Path)
			}
			entity.UniqueID, entity.LocalEntity, entity.DisplaySuffix = uniqueIDs[entityID], entityID, entity.Capability
			entity.Instance, entity.StateTopic, entity.NodeAvailabilityTopics = instance, hamqtt.BridgeStateTopic(instance, entityID), node
			entity.Device = copyDeviceBlock(device)
			body := hamqtt.BridgeEntityConfig(entity)
			data, err := yaml.Marshal([]map[string]interface{}{{domain: body}})
			if err == nil {
				err = writeYAMLFile(filepath.Join(haOutputDir, "entities", "mqtt", domain, entityID+".yaml"), generatorHeader+string(data))
			}
			if err != nil {
				reports = append(reports, fmt.Sprintf("%s: %v", entityID, err))
				continue
			}
			admin.YAMLEntities[entityID] = body
			admin.BatteryEmulationUniqueIDs[entityID] = uniqueIDs[entityID]
		}
	}
	return reports
}

// batteryEmulationDeviceBlocks finds, for every emulated device, the device block of one of its
// YAML "mqtt:" entities under haOutputDir.
func batteryEmulationDeviceBlocks(haOutputDir string, admin *TAdministrationState) map[string]map[string]interface{} {
	entityDevice := entityDeviceIDs(admin)
	files, _ := filepath.Glob(filepath.Join(haOutputDir, "entities", "mqtt", "*", "*.yaml"))
	sort.Strings(files)
	blocks := map[string]map[string]interface{}{}
	for _, file := range files {
		deviceID := entityDevice[strings.TrimSuffix(filepath.Base(file), ".yaml")]
		if _, emulated := admin.BatteryEmulationEntities[deviceID]; !emulated || blocks[deviceID] != nil {
			continue
		}
		data, err := os.ReadFile(file)
		var parsed []map[string]map[string]interface{}
		if err != nil || yaml.Unmarshal(data, &parsed) != nil || len(parsed) != 1 {
			continue
		}
		for _, body := range parsed[0] {
			if device, isMap := body["device"].(map[string]interface{}); isMap {
				blocks[deviceID] = device
			}
		}
	}
	return blocks
}

// copyDeviceBlock is a shallow copy of device: every entity of it gets its own, which the naming
// pass may rewrite.
func copyDeviceBlock(device map[string]interface{}) map[string]interface{} {
	copied := make(map[string]interface{}, len(device))
	for key, value := range device {
		copied[key] = value
	}
	return copied
}

// --- the emulating instance ---

// batteryEmulationInstanceDomains are the integrations the emulating instance's configuration
// includes for it.
var batteryEmulationInstanceDomains = []string{"input_boolean", "input_datetime", "input_number", "template"}

// batteryEmulationBackingID is the entity on the emulating instance behind mainEntityID -- its own
// id, so that main can run the emulation itself too; its unique_id is main's with "_source".
func batteryEmulationBackingID(domain, mainEntityID string) string {
	return domain + "." + mainEntityID[strings.Index(mainEntityID, ".")+1:] + "_emulation"
}

// writeBatteryEmulationInstanceFiles writes, under instanceOutputDir, every emulation's helpers,
// templates and automations for the emulating instance (see the file comment).
func writeBatteryEmulationInstanceFiles(instanceOutputDir, instance string, admin *TAdministrationState) error {
	for _, deviceID := range sortedMapKeys(admin.BatteryEmulationEntities) {
		entities := admin.BatteryEmulationEntities[deviceID]
		if entities.Level == "" {
			continue
		}
		emulation := declaredBatteryEmulations[deviceID]
		uniqueIDs := batteryEmulationUniqueIDs(emulation, entities)
		level := batteryEmulationBackingID("sensor", entities.Level)
		date := batteryEmulationBackingID("input_datetime", entities.Date)
		duration := batteryEmulationBackingID("input_number", entities.Duration)
		seeded := batteryEmulationBackingID("input_boolean", entities.Level) + "_seeded"
		levelIdentity, _ := entityIdentityByID(admin, entities.Level)
		name := func(entityID string) string {
			identity, _ := entityIdentityByID(admin, entityID)
			return friendlyEntityName(identity) + " (emulation)"
		}
		object := func(entityID string) string { return entityID[strings.Index(entityID, ".")+1:] }

		files := map[string]string{
			filepath.Join("input_datetime", "infrastructural", date+".yaml"): object(date) + ":\n  name: " + name(entities.Date) + "\n  has_date: true\n  has_time: false\n",
			filepath.Join("input_number", "infrastructural", duration+".yaml"): object(duration) + ":\n  name: " + name(entities.Duration) +
				"\n  min: 1\n  max: 3650\n  step: 1\n  mode: box\n  unit_of_measurement: d\n",
			filepath.Join("input_boolean", "infrastructural", seeded+".yaml"):     object(seeded) + ":\n  name: " + name(entities.Level) + " seeded\n",
			filepath.Join("template", "sensor", "infrastructural", level+".yaml"): batteryEmulationLevelYAML(level, name(entities.Level), uniqueIDs[entities.Level]+"_source", date, duration),
		}
		alert := ""
		if entities.Alert != "" {
			alert = batteryEmulationBackingID("binary_sensor", entities.Alert)
			files[filepath.Join("template", "binary_sensor", "infrastructural", alert+".yaml")] = batteryEmulationAlertYAML(alert, name(entities.Alert), uniqueIDs[entities.Alert]+"_source", level, emulation.AlertPercent)
		}
		for path, content := range files {
			if err := writeYAMLFile(filepath.Join(instanceOutputDir, "entities", path), generatorHeader+content); err != nil {
				return err
			}
		}

		var sb strings.Builder
		sb.WriteString(generatorHeader)
		sb.WriteString(batteryEmulationSeedAutomation(levelIdentity, emulation, seeded, date, duration))
		reports := []struct{ entityID, source, suffix string }{
			{entities.Level, level, "/battery_level"}, {entities.Date, date, "/" + batteryReplacedSegment},
			{entities.Duration, duration, "/" + batteryDurationSegment}, {entities.Reset, date, "/" + batteryResetSegment},
		}
		if alert != "" {
			reports = append(reports, struct{ entityID, source, suffix string }{entities.Alert, alert, "/battery_alert"})
		}
		for _, report := range reports {
			sb.WriteString(entityReportingAutomationBody(levelIdentity, []string{report.source}, hamqtt.BridgeStateTopic(instance, report.entityID), "states('"+report.source+"')", report.suffix))
		}
		setValue := hamqtt.Command{ValueTrigger: true}
		dateIdentity, _ := entityIdentityByID(admin, entities.Date)
		durationIdentity, _ := entityIdentityByID(admin, entities.Duration)
		sb.WriteString(entityCommandAutomationBody(dateIdentity, commandSpec{Name: "set_value", Service: "input_datetime.set_datetime", ValueTrigger: true, DataKey: "date"}, date, hamqtt.BridgeCommandTopicFor(instance, entities.Date, "set_value", setValue)))
		sb.WriteString(entityCommandAutomationBody(durationIdentity, commandSpec{Name: "set_value", Service: "input_number.set_value", ValueTrigger: true}, duration, hamqtt.BridgeCommandTopicFor(instance, entities.Duration, "set_value", setValue)))
		sb.WriteString(batteryEmulationResetAutomation(levelIdentity, date, hamqtt.BridgeCommandTopic(instance, entities.Reset)))
		path := filepath.Join(instanceOutputDir, "automation", "infrastructural", "automation.battery_emulation_"+object(entities.Level)+".yaml")
		if err := writeYAMLFile(path, sb.String()); err != nil {
			return err
		}
	}
	return nil
}

// batteryEmulationLevelExpression is the level: 100 * (X - D) / X percent, X duration's days (at
// least 1), D the days since date's date, kept between 0 and 100.
func batteryEmulationLevelExpression(date, duration string) string {
	days := fmt.Sprintf("([states('%s') | int(0), 1] | max)", duration)
	elapsed := fmt.Sprintf("(now().date() - strptime(states('%s'), '%%Y-%%m-%%d').date()).days", date)
	return fmt.Sprintf("[100, [0, (100 * (%s - %s) / %s) | round(0) | int] | max] | min", days, elapsed, days)
}

// batteryEmulationLevelYAML is the level's template sensor, unavailable until date and duration
// have a value.
func batteryEmulationLevelYAML(id, name, uniqueID, date, duration string) string {
	return "sensor:\n" +
		"- name: " + name + "\n" +
		"  unique_id: " + uniqueID + "\n" +
		"  default_entity_id: " + id + "\n" +
		"  unit_of_measurement: \"%\"\n" +
		"  device_class: battery\n" +
		"  state_class: measurement\n" +
		"  availability: \"{{ has_value('" + date + "') and has_value('" + duration + "') }}\"\n" +
		"  state: \"{{ " + batteryEmulationLevelExpression(date, duration) + " }}\"\n"
}

// batteryEmulationAlertYAML is the alert's template binary_sensor: on below percent.
func batteryEmulationAlertYAML(id, name, uniqueID, level string, percent int) string {
	return "binary_sensor:\n" +
		"- name: " + name + "\n" +
		"  unique_id: " + uniqueID + "\n" +
		"  default_entity_id: " + id + "\n" +
		"  device_class: battery\n" +
		"  availability: \"{{ has_value('" + level + "') }}\"\n" +
		fmt.Sprintf("  state: \"{{ (states('%s') | int(0)) < %d }}\"\n", level, percent)
}

// batteryEmulationSeedAutomation sets the date and duration to their starting values once, when
// the helpers are new (seeded off): the date in Physical.def, else today.
func batteryEmulationSeedAutomation(identity TEntityIdentity, emulation TBatteryEmulation, seeded, date, duration string) string {
	since := "{{ now().date() }}"
	if emulation.Since != "" {
		since = emulation.Since
	}
	var sb strings.Builder
	sb.WriteString("- alias: \"" + entityAliasBase(identity) + "/emulation/seed\"\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: homeassistant\n")
	sb.WriteString("    event: start\n")
	sb.WriteString("  - platform: event\n")
	sb.WriteString("    event_type: automation_reloaded\n")
	sb.WriteString("  condition:\n")
	sb.WriteString("  - condition: state\n")
	sb.WriteString("    entity_id: " + seeded + "\n")
	sb.WriteString("    state: \"off\"\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: input_datetime.set_datetime\n")
	sb.WriteString("    target:\n")
	sb.WriteString("      entity_id: " + date + "\n")
	sb.WriteString("    data:\n")
	sb.WriteString("      date: \"" + since + "\"\n")
	sb.WriteString("  - service: input_number.set_value\n")
	sb.WriteString("    target:\n")
	sb.WriteString("      entity_id: " + duration + "\n")
	sb.WriteString("    data:\n")
	sb.WriteString(fmt.Sprintf("      value: %d\n", emulation.Days))
	sb.WriteString("  - service: input_boolean.turn_on\n")
	sb.WriteString("    target:\n")
	sb.WriteString("      entity_id: " + seeded + "\n")
	return sb.String()
}

// batteryEmulationResetAutomation sets date to today when main's button is pressed.
func batteryEmulationResetAutomation(identity TEntityIdentity, date, topic string) string {
	var sb strings.Builder
	sb.WriteString("- alias: \"" + entityAliasBase(identity) + "/command/" + batteryResetSegment + "\"\n")
	sb.WriteString("  trigger:\n")
	sb.WriteString("  - platform: mqtt\n")
	sb.WriteString("    topic: \"" + topic + "\"\n")
	sb.WriteString("    payload: \"" + batteryResetPayload + "\"\n")
	sb.WriteString("  action:\n")
	sb.WriteString("  - service: input_datetime.set_datetime\n")
	sb.WriteString("    target:\n")
	sb.WriteString("      entity_id: " + date + "\n")
	sb.WriteString("    data:\n")
	sb.WriteString("      date: \"{{ now().date() }}\"\n")
	return sb.String()
}
