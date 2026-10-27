/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: FrozenUniqueIDs
 *
 * PROJECT.md item 48 (2026-10-21): every derived capability gets a frozen universal ID -- the
 * unique_id its generated template entity is registered under in Home Assistant -- written into
 * Physical.def/Logical.def as "as <id>", item 7's shape for raw discovery capabilities. Its value
 * is the entity's own entity_id at the moment it is first assigned ("name at birth"), which is
 * exactly the unique_id it already had (generated unique_ids used to be the entity_id), so
 * freezing changes nothing in Home Assistant; from then on it never changes, surviving later
 * conceptual AND capability renames. The other half -- renaming the entity_id in Home Assistant's
 * registry when the name changes -- belongs to the deploy.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 21.10.2026
 *
 */

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// loadFrozenUniqueIDs fills admin.FrozenUniqueIDs from every derived capability's frozen "as <id>":
// Physical.def's (keyed "physical:<device-id>!<label>") and Logical.def's (keyed
// "logical:<device-id>!<label>") -- the same keys TEntityRecord.SourceKey uses.
func loadFrozenUniqueIDs(admin *TAdministrationState, definitionDir string, logicalByID map[string]TLogicalDevice) {
	physicalContent, _, _ := collectLayerContent(definitionDir, []string{"Physical.def"}, LayerPhysical)
	for key, id := range collectDerivedFrozenIDs(physicalContent) {
		admin.FrozenUniqueIDs["physical:"+key] = id
	}
	for deviceID, device := range logicalByID {
		for label, capability := range device.Capabilities {
			if capability.AssignedID != "" {
				admin.FrozenUniqueIDs["logical:"+deviceID+"!"+label] = capability.AssignedID
			}
		}
	}
}

// autoAssignFrozenUniqueIDs gives every generated entity whose capability has no frozen ID yet its
// current entity_id as one, written into that capability's own derived line in Physical.def or
// Logical.def. Only derived capability lines carry an ID; an entity whose capability isn't one (a
// plain local capability, a logical "defined" one, ...) keeps its entity_id as unique_id, as before.
// Reports what it assigned, and every line it couldn't locate unambiguously (skipped, never guessed).
func autoAssignFrozenUniqueIDs(definitionDir string, admin *TAdministrationState) []string {
	byFile := map[string]map[string]string{} // file -> "<device-id>!<label>" -> id
	var reports []string
	reportedShared := map[string]bool{}
	entityIDs := make([]string, 0, len(admin.SourceKeyByEntityID))
	for entityID := range admin.SourceKeyByEntityID {
		entityIDs = append(entityIDs, entityID)
	}
	sort.Strings(entityIDs)
	for _, entityID := range entityIDs {
		key := admin.SourceKeyByEntityID[entityID]
		if admin.SharedSourceKeys[key] {
			if !reportedShared[key] {
				reportedShared[key] = true
				var users []string
				for other, otherKey := range admin.SourceKeyByEntityID {
					if otherKey == key {
						users = append(users, other)
					}
				}
				sort.Strings(users)
				reports = append(reports, fmt.Sprintf("capability %q feeds %d entities (%s) -- no frozen unique_id; position it only once (e.g. not also directly when a logical device absorbs it)", key, len(users), strings.Join(users, ", ")))
			}
			continue
		}
		if _, frozen := admin.FrozenUniqueIDs[key]; frozen {
			continue
		}
		layer, deviceLabel, ok := strings.Cut(key, ":")
		if !ok {
			continue
		}
		file := map[string]string{"physical": "Physical.def", "logical": "Logical.def"}[layer]
		if file == "" {
			continue
		}
		if byFile[file] == nil {
			byFile[file] = map[string]string{}
		}
		byFile[file][deviceLabel] = entityID
	}

	// Imported capabilities (PROJECT.md item 48): frozen as their CURRENT unique_id -- the
	// coordinator's name-based formula for their current local entity -- so nothing changes in HA.
	for _, deviceID := range admin.ImportedDeviceIDs {
		for label, attribute := range admin.DeviceConceptualLinks[deviceID].AttributeEntityIDs {
			if _, frozen := admin.FrozenUniqueIDs["import:"+deviceID+"!"+label]; frozen || attribute.EntityID == "" {
				continue
			}
			if byFile["Physical.def"] == nil {
				byFile["Physical.def"] = map[string]string{}
			}
			byFile["Physical.def"][importAssignmentPrefix+deviceID+"!"+label] = importedUniqueIDFor(admin, deviceID, label, attribute.EntityID)
		}
	}

	for _, file := range []string{"Physical.def", "Logical.def"} {
		if len(byFile[file]) > 0 {
			reports = append(reports, writeFrozenUniqueIDs(filepath.Join(definitionDir, file), byFile[file])...)
		}
	}
	return reports
}

// importAssignmentPrefix marks an assignment key for an imported capability's plain
// "capability <domain>.<label>;" line, rather than a derived capability's line.
const importAssignmentPrefix = "import|"

// commandlineAssignmentPrefix marks an assignment key for a commandline capability's
// "capability <kind>.<name> <scripts>;" line.
const commandlineAssignmentPrefix = "commandline|"

// autoAssignCommandlineFrozenUniqueIDs freezes every commandline capability that has no frozen ID
// yet as its CURRENT unique_id, "<device-id>_<name>" (the coordinator's formula,
// commandline_yaml_entities.go), written into its own Physical.def line -- so nothing changes in HA, and
// renaming the capability later keeps its registry entry.
func autoAssignCommandlineFrozenUniqueIDs(definitionDir string) []string {
	devices, _ := collectCommandlineDevicesByID(definitionDir)
	assignments := map[string]string{}
	for deviceID, device := range devices {
		for name, capability := range device.Capabilities {
			if capability.AssignedID == "" {
				assignments[commandlineAssignmentPrefix+deviceID+"!"+name] = deviceID + "_" + name
			}
		}
	}
	if len(assignments) == 0 {
		return nil
	}
	return writeFrozenUniqueIDs(filepath.Join(definitionDir, "Physical.def"), assignments)
}

// bridgedAssignmentPrefix marks an assignment key for a bridged ("home_assistant") capability's
// plain or derived capability line.
const bridgedAssignmentPrefix = "bridged|"

// frozenIDDeviceHeaderPattern matches the start of a device declaration in Physical.def or
// Logical.def, capturing its device id.
// enabledByLineOf returns the index of deviceID's "enabled by" line in lines (Logical.def) that
// has no "as <id>" yet, if it has exactly one -- in a "device ... with:" block, or a one-line
// "device ... with enabled by ...;".
func enabledByLineOf(lines []string, deviceID string) (int, bool) {
	found, count, inDevice := -1, 0, false
	for i, line := range lines {
		if m := frozenIDDeviceHeaderPattern.FindStringSubmatch(line); m != nil {
			inDevice = m[1] == deviceID
			if inDevice && strings.Contains(line, " with enabled by ") {
				found, count = i, count+1
			}
			continue
		}
		if inDevice && regexp.MustCompile(`^\s*enabled\s+by\s`).MatchString(line) {
			found, count = i, count+1
		}
	}
	if count != 1 || regexp.MustCompile(`\sas\s`).MatchString(lines[found]) {
		return -1, false
	}
	return found, true
}

var frozenIDDeviceHeaderPattern =regexp.MustCompile(`^\s*device\s+([A-Za-z_][A-Za-z0-9_]*\.[A-Za-z_][A-Za-z0-9_]*)\b`)

// assignedIDHeadPattern matches the part of a capability statement its frozen "as <id>" follows
// (2026-10-22: always before the statement's "with"/"from" part): a derived capability's name, or a
// bridged capability's name plus its source entity.
var assignedIDHeadPattern = regexp.MustCompile(`^\s*(?:hidden\s+)?(?:derived\s+capability\s+[A-Za-z_][A-Za-z0-9_]*\.[A-Za-z_][A-Za-z0-9_/]*|capability\s+[A-Za-z_][A-Za-z0-9_]*\.[A-Za-z_][A-Za-z0-9_/]*\s+[^\s;]+)`)

// commandlineAssignedIDHeadPattern is a commandline capability line up to its name, which its
// frozen "as <id>" follows (before the quoted scripts).
var commandlineAssignedIDHeadPattern = regexp.MustCompile(`^\s*capability\s+(?:switch|sensor|button)\.[A-Za-z_][A-Za-z0-9_]*`)

// writeFrozenUniqueIDs adds " as <id>" to each assignment's capability statement in path (the real
// on-disk file, keeping its own layout): right after a derived capability's name or a bridged
// capability's source, before a block header's "with:", or else before the statement's own ";". The statement is looked up within its
// own device's declaration; one that isn't found exactly once there is reported and left alone.
func writeFrozenUniqueIDs(path string, assignments map[string]string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return []string{fmt.Sprintf("could not read %s to assign frozen unique_ids: %v", path, err)}
	}
	lines := strings.Split(string(raw), "\n")
	var reports []string
	changed := false

	keys := make([]string, 0, len(assignments))
	for key := range assignments {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		id := assignments[key]
		plainKey, isImport := strings.CutPrefix(key, importAssignmentPrefix)
		plainKey, isBridged := strings.CutPrefix(plainKey, bridgedAssignmentPrefix)
		plainKey, isCommandline := strings.CutPrefix(plainKey, commandlineAssignmentPrefix)
		deviceID, label, _ := strings.Cut(plainKey, "!")
		start, count := -1, 0
		statement := regexp.MustCompile(`^\s*(?:hidden\s+)?derived\s+capability\s+[A-Za-z_][A-Za-z0-9_]*\.` + regexp.QuoteMeta(label) + `\s`)
		if filepath.Base(path) == "Logical.def" {
			// A Logical.def capability is either derived or absorbed ("capability <domain>.<label>[
			// <source>];" inside an "absorb from" block).
			statement = regexp.MustCompile(`^\s*(?:derived\s+capability\s+[A-Za-z_][A-Za-z0-9_]*\.` + regexp.QuoteMeta(label) + `\s|capability\s+[A-Za-z_][A-Za-z0-9_]*\.` + regexp.QuoteMeta(label) + `(?:\s[^;]*)?;)`)
		}
		if isImport {
			statement = regexp.MustCompile(`^\s*capability\s+[A-Za-z_][A-Za-z0-9_]*\.` + regexp.QuoteMeta(label) + `\s*;`)
		}
		if isBridged {
			statement = regexp.MustCompile(`^\s*(?:hidden\s+)?(?:derived\s+)?(?:capability\s+)?[A-Za-z_][A-Za-z0-9_]*\.` + regexp.QuoteMeta(label) + `\s`)
		}
		if isCommandline {
			statement = regexp.MustCompile(`^\s*capability\s+(?:switch|sensor|button)\.` + regexp.QuoteMeta(label) + `\s`)
		}
		inDevice := false
		for i, line := range lines {
			if m := frozenIDDeviceHeaderPattern.FindStringSubmatch(line); m != nil {
				inDevice = m[1] == deviceID
				continue
			}
			if inDevice && statement.MatchString(line) {
				start = i
				count++
			}
		}
		if count == 0 && label == "node" && filepath.Base(path) == "Logical.def" {
			// An "enabled by" device's implied node (logical_enabled_by.go) carries its ID on the
			// "enabled by" line itself, before its "with delay_off", if any.
			if line, found := enabledByLineOf(lines, deviceID); found {
				if head, tail, hasWith := strings.Cut(lines[line], " with "); hasWith {
					lines[line] = head + " as " + id + " with " + tail
				} else {
					lines[line] = appendAssignedIDToLine(lines[line], id)
				}
				changed = true
				reports = append(reports, fmt.Sprintf("assigned frozen unique_id %q to device %q's implied node (%s)", id, deviceID, filepath.Base(path)))
				continue
			}
		}
		if count == 0 {
			continue // not a derived capability line: keeps its entity_id as unique_id
		}
		if count > 1 {
			reports = append(reports, fmt.Sprintf("%s: %d derived %q lines for device %q (want exactly 1) -- frozen unique_id not assigned", filepath.Base(path), count, label, deviceID))
			continue
		}
		if isCommandline {
			head := commandlineAssignedIDHeadPattern.FindString(lines[start])
			lines[start] = head + " as " + id + lines[start][len(head):]
		} else if head := assignedIDHeadPattern.FindString(lines[start]); head != "" && (isBridged || strings.Contains(head, "derived")) {
			lines[start] = head + " as " + id + lines[start][len(head):]
		} else if strings.HasSuffix(strings.TrimSpace(lines[start]), " with:") {
			trimmed := strings.TrimRight(lines[start], " \t")
			lines[start] = strings.TrimSuffix(trimmed, " with:") + " as " + id + " with:"
		} else {
			end := start
			for end < len(lines) && !strings.HasSuffix(strings.TrimRight(lines[end], " \t"), ";") {
				end++
			}
			if end == len(lines) {
				reports = append(reports, fmt.Sprintf("%s: derived %q line for device %q has no closing \";\" -- frozen unique_id not assigned", filepath.Base(path), label, deviceID))
				continue
			}
			lines[end] = appendAssignedIDToLine(lines[end], id)
		}
		changed = true
		reports = append(reports, fmt.Sprintf("assigned frozen unique_id %q to device %q's %q capability (%s)", id, deviceID, label, filepath.Base(path)))
	}

	if changed {
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			return []string{fmt.Sprintf("could not write %s with frozen unique_ids: %v", path, err)}
		}
	}
	return reports
}

// autoAssignBridgedFrozenUniqueIDs freezes every bridged ("home_assistant") capability that has no
// frozen ID yet as its CURRENT unique_id (the coordinator's name-based formula), written into its
// own capability line in Physical.def -- so nothing changes in HA. Runs only once every bridged
// device's local entities are known: after registerExportedHassBridgeDevices, which positions the
// devices that are only exported (never positioned in this house's own Conceptual.def).
func autoAssignBridgedFrozenUniqueIDs(definitionDir string, admin *TAdministrationState) []string {
	assignments := map[string]string{}
	for _, deviceID := range admin.BridgedDeviceIDs {
		for label, attribute := range admin.DeviceConceptualLinks[deviceID].AttributeEntityIDs {
			if _, frozen := admin.FrozenUniqueIDs["physical:"+deviceID+"!"+label]; frozen || attribute.EntityID == "" {
				continue
			}
			assignments[bridgedAssignmentPrefix+deviceID+"!"+label] = bridgedUniqueIDFor(admin, deviceID, label, attribute.EntityID)
		}
	}
	if len(assignments) == 0 {
		return nil
	}
	return writeFrozenUniqueIDs(filepath.Join(definitionDir, "Physical.def"), assignments)
}

// attributeAbsorbedEntities gives every entity a Logical.def device absorbs its own source key,
// "logical:<device-id>!<label>", instead of the absorbed physical capability's -- so a capability
// that is both positioned directly and absorbed yields two entities with two universal IDs (one on
// its Physical.def line, one on the absorb line) rather than two entities sharing one, which no
// def line could freeze. Recomputes SharedSourceKeys afterwards.
func attributeAbsorbedEntities(admin *TAdministrationState, logicalByID map[string]TLogicalDevice) {
	for entityID, link := range admin.LogicalEntityLinks {
		capability, known := logicalByID[link.DeviceID].Capabilities[link.Label]
		if !known || capability.AbsorbedFromDeviceID == "" {
			continue
		}
		if _, tracked := admin.SourceKeyByEntityID[entityID]; tracked {
			admin.SourceKeyByEntityID[entityID] = "logical:" + link.DeviceID + "!" + link.Label
		}
	}
	count := map[string]int{}
	for _, key := range admin.SourceKeyByEntityID {
		count[key]++
	}
	admin.SharedSourceKeys = map[string]bool{}
	for key, n := range count {
		if n > 1 {
			admin.SharedSourceKeys[key] = true
		}
	}
}
