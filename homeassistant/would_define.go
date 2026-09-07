/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: WouldDefine
 *
 * Implements the "-would_define <entity_id>" CLI flag: checks whether a given Home Assistant
 * entity_id (e.g. "sensor.physical_entrance_garage_door_temperature") would be defined -- or
 * assumed to exist -- by this DSL, without generating any output or making any network calls.
 * Runs only the generation pipeline's read-and-interpret phase (parseAdministrationFromPaths,
 * generator.go), then looks the entity id up against the resulting entity registry.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 23.08.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
)

// describeWouldDefine is the pure lookup at runWouldDefineCheck's core, kept separate so it's
// testable against a hand-built TAdministrationState without any file I/O. Returns the message to
// print and whether entityID was found at all (see runWouldDefineCheck's doc comment for what
// each outcome means).
func describeWouldDefine(admin *TAdministrationState, entityID string) (message string, found bool) {
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if toHomeAssistantEntityID(rec.Name) != entityID {
				continue
			}
			switch {
			case rec.DiscoveryImplied:
				return fmt.Sprintf("%s: yes -- discovery-implied (expected from the MQTT coordinator, not compiler-generated YAML)\n  %s", entityID, rec.Provenance), true
			case rec.HasDefinitionOrImport:
				return fmt.Sprintf("%s: yes -- compiler-generated\n  %s", entityID, rec.Provenance), true
			default:
				return fmt.Sprintf("%s: referenced but not defined or imported\n  %s", entityID, rec.Provenance), true
			}
		}
	}
	return fmt.Sprintf("%s: no -- not referenced anywhere in this DSL", entityID), false
}

// runWouldDefineCheck reports how (if at all) entityID would come to exist per this DSL:
//   - compiler-generated: an ordinary entity this generator writes YAML for itself.
//   - discovery-implied: expected to be created later by the MQTT coordinator
//     (house_event_bus_coordinator), not by generator-authored YAML (TEntityRecord.DiscoveryImplied).
//   - referenced but not defined/imported: known to the DSL (e.g. referenced by a directive) but
//     without its own definition or import -- unusual, worth flagging as-is rather than folding
//     into "unknown".
//   - unknown: entityID doesn't correspond to anything this DSL's Spaces.def registers at all.
//
// Returns found=true for every case except "unknown", so callers can map it to a shell-friendly
// exit code without re-parsing the printed message.
func runWouldDefineCheck(cwd, defPath, entityID string) (found bool, err error) {
	fullDefPath := filepath.Join(cwd, defPath)
	definitionDir := filepath.Dir(fullDefPath)
	sharedDefinitionDir := filepath.Clean(filepath.Join(definitionDir, "..", "..", "Shared", "Definitions"))
	label := filepath.Base(cwd)

	admin, err := parseAdministrationFromPaths(definitionDir, sharedDefinitionDir, label)
	if err != nil {
		return false, err
	}

	message, found := describeWouldDefine(admin, entityID)
	fmt.Println(message)
	return found, nil
}
