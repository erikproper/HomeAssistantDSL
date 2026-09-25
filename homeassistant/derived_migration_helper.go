/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: DerivedMigrationHelper
 *
 * Detect-and-report tool for plans/derived-capability-mechanism.md's "Confirmed rollout sequence"
 * step 2: finds real, currently-live "call battery_alert ...;"/"call battery_level_device ...;"
 * macro usages (Macros.def) whose battery_level source traces back to a real Physical.def
 * home_assistant/hassbridge or import device -- genuine candidates for migrating to a "derived
 * entity binary_sensor.battery_alert from sensor.battery_level via ...;" Physical.def declaration.
 * Report-only, by design (decided with the user 2026-09-10): this file never edits a real .def
 * file. Once the full Zigbee2MQTT migration needs the same detection for its own bare-entity
 * "condition"/"adjustment" usages, an auto-rewrite mode can be layered on top of the same
 * detection logic here, worked through per-device -- not built yet, deliberately deferred until
 * that migration actually needs it.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 10.09.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// findBatteryLevelWithoutAlertDevices finds every home_assistant/hassbridge or import device that
// declares a "battery_level" capability but no "battery_alert" capability at all (atomic or
// derived) -- Rule 2's own violation set (plans/derived-capability-mechanism.md's "Two
// hard-enforcement rules"), reused here to scope the migration helper's report to real candidates;
// Rule 2 itself is not enabled as a live check yet (still gated on this migration completing in
// both houses, per the plan's "Confirmed rollout sequence").
func findBatteryLevelWithoutAlertDevices(hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice) []string {
	var violators []string
	for deviceID, device := range hassBridgeDevicesByID {
		if _, hasLevel := device.Capabilities["battery_level"]; hasLevel {
			if _, hasAlert := device.Capabilities["battery_alert"]; !hasAlert {
				violators = append(violators, deviceID)
			}
		}
	}
	for deviceID, device := range importedDevicesByID {
		if _, hasLevel := device.Capabilities["battery_level"]; hasLevel {
			if _, hasAlert := device.Capabilities["battery_alert"]; !hasAlert {
				violators = append(violators, deviceID)
			}
		}
	}
	sort.Strings(violators)
	return violators
}

// TBatteryAlertMigrationCandidate is one real, currently-live macro-driven battery_alert usage
// whose battery_level source traces back to a real Physical.def device -- see this file's own
// header comment.
type TBatteryAlertMigrationCandidate struct {
	DeviceID             string
	BatteryLevelEntityID string
	BatteryAlertEntityID string
	AlertLevel           int
	Provenance           string // e.g. "Spaces.def:203 → battery_alert :netatmo"
}

// findBatteryAlertMigrationCandidates cross-references findBatteryLevelWithoutAlertDevices' own
// violation set against admin's already-macro-EXPANDED "battery_alert"-suffixed entities, rather
// than re-parsing Spaces.def's raw macro call syntax. Deliberately does NOT use
// TEntityRecord.ConditionSources -- a "/battery_alert"-suffixed record's ConditionSources/
// ConditionExpr are never populated at all (expander.go's own expansion loop explicitly skips the
// generic condition scan for battery_alert entities, scanning only BatteryAlertLevel via
// scanBatteryAlertBody instead). Its battery_level source is instead a pure NAMING CONVENTION,
// confirmed against generateTemplateBinarySensors' own derivation (generator.go): always
// "sensor.infrastructural" + the same path hierarchy with "/battery_alert" swapped for
// "/battery_level" -- reproduced here exactly, so this stays correct if that convention is ever
// revisited, since both read from the one macro (Macros.def) that actually produces this shape.
// This works regardless of which exact macro (battery_alert, battery_level_device, or any future
// equivalent) produced the entity, as long as it followed that one shared naming convention. A
// violating device whose battery_level source doesn't resolve to any positioned battery_alert
// entity this way is simply absent from the result -- not every Rule 2 violator necessarily has a
// live macro usage to migrate from; a device whose battery_level itself traces back to a bare
// (non-Physical.def-backed) entity, e.g. a raw Zigbee2MQTT passthrough one, never appears in the
// violation set in the first place (it has no Physical.def battery_level capability at all), so
// it's automatically excluded without any separate Zigbee-specific filtering here.
func findBatteryAlertMigrationCandidates(admin *TAdministrationState, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice) []TBatteryAlertMigrationCandidate {
	deviceIDByBatteryLevelEntity := map[string]string{}
	for _, deviceID := range findBatteryLevelWithoutAlertDevices(hassBridgeDevicesByID, importedDevicesByID) {
		link, ok := admin.DeviceConceptualLinks[deviceID]
		if !ok {
			continue
		}
		attr, ok := link.AttributeEntityIDs["battery_level"]
		if !ok {
			continue
		}
		deviceIDByBatteryLevelEntity[attr.EntityID] = deviceID
	}

	seen := map[string]bool{}
	var candidates []TBatteryAlertMigrationCandidate
	for _, records := range admin.EntityRecordsBySpace {
		for _, rec := range records {
			if rec.BatteryAlertLevel == 0 || rec.Identity.Domain != "binary_sensor" || !strings.HasSuffix(rec.Name, "/battery_alert") {
				continue
			}
			if seen[rec.Name] {
				continue
			}
			seen[rec.Name] = true

			batteryLevelEntityID := toHomeAssistantEntityID("sensor.infrastructural/" + strings.TrimSuffix(rec.Identity.Path, "/battery_alert") + "/battery_level")
			deviceID, known := deviceIDByBatteryLevelEntity[batteryLevelEntityID]
			if !known {
				continue
			}
			candidates = append(candidates, TBatteryAlertMigrationCandidate{
				DeviceID:             deviceID,
				BatteryLevelEntityID: batteryLevelEntityID,
				BatteryAlertEntityID: toHomeAssistantEntityID(rec.Name),
				AlertLevel:           rec.BatteryAlertLevel,
				Provenance:           rec.Provenance,
			})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].DeviceID < candidates[j].DeviceID })
	return candidates
}

// recommendedDerivedDeclaration renders the exact Physical.def line to add for one candidate --
// "'on' if (...) else 'off'", NOT a bare Jinja boolean, per this session's own convention note
// (plans/derived-capability-mechanism.md): a "derived" template must render the literal state
// string the target domain expects (matching sourceToJinja2's own "is available" sugar), which
// applyProxiedBinarySensorPayload/HA's own binary_sensor default payload_on/payload_off both
// assume.
func recommendedDerivedDeclaration(c TBatteryAlertMigrationCandidate) string {
	return "derived binary_sensor.battery_alert from sensor.battery_level via " +
		"\"'on' if (($ | int(0)) < " + strconv.Itoa(c.AlertLevel) + ") else 'off'\";"
}

// runBatteryAlertMigrationReport implements the "-derived_migration_candidates" CLI flag: runs
// the ordinary read-and-interpret phase (parseAdministrationFromPaths, same as
// runWouldDefineCheck), then prints every real battery_alert migration candidate found, plus the
// exact Physical.def line to add and the Spaces.def provenance of the macro call it would replace.
// Report-only -- never touches a real .def file (see this file's own header comment for why).
func runBatteryAlertMigrationReport(cwd, defPath string) error {
	fullDefPath := filepath.Join(cwd, defPath)
	definitionDir := filepath.Dir(fullDefPath)
	sharedDefinitionDir := filepath.Clean(filepath.Join(definitionDir, "..", "..", "Shared", "Definitions"))
	label := filepath.Base(cwd)

	admin, err := parseAdministrationFromPaths(definitionDir, sharedDefinitionDir, label)
	if err != nil {
		return err
	}
	hassBridgeDevicesByID, _ := collectHassBridgeDevicesByID(definitionDir)
	importedDevicesByID, _ := collectImportedDevicesByID(definitionDir)

	candidates := findBatteryAlertMigrationCandidates(admin, hassBridgeDevicesByID, importedDevicesByID)
	if len(candidates) == 0 {
		fmt.Println("no battery_alert migration candidates found")
		return nil
	}
	for _, c := range candidates {
		fmt.Printf("device %q:\n", c.DeviceID)
		fmt.Printf("  currently: %s\n", c.Provenance)
		fmt.Printf("  add to Physical.def (inside this device's own block): %s\n", recommendedDerivedDeclaration(c))
		fmt.Printf("  then remove the Spaces.def macro call above, and replace it with an ordinary\n")
		fmt.Printf("  positioning line for the new \"battery_alert\" capability (same shape any other\n")
		fmt.Printf("  capability already uses for this device).\n")
	}
	return nil
}
