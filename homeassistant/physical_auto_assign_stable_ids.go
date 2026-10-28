/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: PhysicalAutoAssignStableIDs
 *
 * PROJECT.md item 7's "full switch" (memory/project_stable_entity_identity_design.md): once a
 * discovery-kind capability's "as <id>;" has been assigned (by the one-time bootstrap script for
 * everything already live, or by this file for anything born after), the GENERATOR itself keeps
 * it frozen and never revisits it. This file is the "born after" half -- the one place this whole
 * codebase deliberately rewrites a `.def` file as a side effect of an ordinary run, rather than
 * only ever reading one. It's narrow and purely additive: the only possible edit is appending
 * " as <id>" before an existing capability line's own trailing ";", never anything else, and only
 * for a capability that genuinely has none yet.
 *
 * Runs once Conceptual.def has been fully parsed (administration.DiscoveryEntityLinks reflects
 * every capability actually positioned right now), from parseAdministrationFromPaths
 * (generator.go), right before it returns.
 *
 * Fresh-id format, per the user's own explicit choice (2026-10-05): the capability's own resolved
 * entity_id at the moment it's first positioned ("position at birth"), frozen forever after --
 * e.g. "light.physical_house_wc_main". On a collision against an id already assigned anywhere
 * (including ones assigned earlier in this SAME run), disambiguated with a plain "_n" suffix
 * (_2, _3, ...), per the user's own correction of the original "__n" sketch.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 05.10.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// tStableIDAssignment is one capability that needs a fresh "as <id>;" written back into
// Physical.def -- everything buildPhysicalDefRewritePattern/appendAssignedIDToLine need to find
// and edit the exact right line, nothing more.
type tStableIDAssignment struct {
	gatewayID    string
	domain       string
	label        string
	sourceDomain string
	leaf         string
	newID        string
	// rawTopic: an "integration mqtt" capability's topic (PROJECT.md 39) -- its line names that,
	// not a leaf.
	rawTopic string
}

// autoAssignDiscoveryStableIDs walks every discovery gateway's every plain raw-leaf capability
// (Leaf != "" -- AvailabilityOf/DerivedFromCapability capabilities never reach
// relayedDiscoveryUniqueID, so they never need one): a capability with no AssignedID yet that IS
// currently positioned (a live entry in admin.DiscoveryEntityLinks keyed on the same
// gatewayID+leaf) gets a freshly minted id, written back into Physical.def; a capability that HAS
// an AssignedID but is no LONGER positioned gets a warning instead (the "orphaned id" complement
// to the existing "undeclared entity" checks, PROJECT.md item 6's own "unused ids we may want to
// re-use later" question).
func autoAssignDiscoveryStableIDs(definitionDir string, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, admin *TAdministrationState) []string {
	var warnings []string

	usedIDs := map[string]bool{}
	for _, gw := range discoveryGatewaysByID {
		for _, cap := range gw.Capabilities {
			if cap.AssignedID != "" {
				usedIDs[cap.AssignedID] = true
			}
		}
	}

	// (gatewayID, leaf) -> entityID for every capability that's actually live right now --
	// built from admin.DiscoveryEntityLinks rather than re-deriving positioning ourselves, so this
	// stays correct for the indirect "derived from hidden raw leaf" case too (the hidden
	// sibling's own gatewayID+leaf, but the DERIVING capability's own resolved entityID --
	// exactly what registerDiscoveryDerivedFromRawLeafEntityLink already built the link from).
	liveEntityIDByGatewayLeaf := map[string]string{}
	entityIDs := make([]string, 0, len(admin.DiscoveryEntityLinks))
	for entityID := range admin.DiscoveryEntityLinks {
		entityIDs = append(entityIDs, entityID)
	}
	sort.Strings(entityIDs) // deterministic "first one wins" on the (extremely unlikely) collision below
	for _, entityID := range entityIDs {
		link := admin.DiscoveryEntityLinks[entityID]
		key := link.GatewayDeviceID + "\x00" + link.Leaf
		if _, exists := liveEntityIDByGatewayLeaf[key]; !exists {
			liveEntityIDByGatewayLeaf[key] = entityID
		}
	}

	gatewayIDs := make([]string, 0, len(discoveryGatewaysByID))
	for id := range discoveryGatewaysByID {
		gatewayIDs = append(gatewayIDs, id)
	}
	sort.Strings(gatewayIDs)

	var toAssign []tStableIDAssignment
	for _, gatewayID := range gatewayIDs {
		gw := discoveryGatewaysByID[gatewayID]
		labels := make([]string, 0, len(gw.Capabilities))
		for label := range gw.Capabilities {
			labels = append(labels, label)
		}
		sort.Strings(labels)
		for _, label := range labels {
			cap := gw.Capabilities[label]
			if cap.Leaf == "" {
				continue
			}
			entityID, isLive := liveEntityIDByGatewayLeaf[gatewayID+"\x00"+cap.Leaf]
			if cap.AssignedID != "" {
				if !isLive {
					warnings = append(warnings, fmt.Sprintf("device %q's %q capability has a frozen \"as %s\" but nothing positions it in Conceptual.def anymore -- a real candidate for reuse (PROJECT.md item 6's own \"unused ids\" question)", gatewayID, label, cap.AssignedID))
				}
				continue
			}
			if !isLive {
				continue
			}
			newID := entityID
			for n := 2; usedIDs[newID]; n++ {
				newID = fmt.Sprintf("%s_%d", entityID, n)
			}
			usedIDs[newID] = true
			toAssign = append(toAssign, tStableIDAssignment{
				gatewayID: gatewayID, domain: cap.Domain, label: label,
				sourceDomain: cap.SourceDomain, leaf: cap.Leaf, newID: newID, rawTopic: rawTopicForLineSearch(gw.RawTopics[label]),
			})
		}
	}

	if len(toAssign) == 0 {
		return warnings
	}

	assigned, rewriteWarnings := rewritePhysicalDefWithAssignedIDs(definitionDir, toAssign)
	warnings = append(warnings, rewriteWarnings...)
	for _, a := range assigned {
		warnings = append(warnings, fmt.Sprintf("assigned stable id %q to device %q's %q capability", a.newID, a.gatewayID, a.label))
	}
	return warnings
}

// discoveryCapabilityLinePattern matches the one Physical.def line a.domain/a.label/
// a.sourceDomain/a.leaf were parsed from -- built entirely from already-known, already-trusted
// fields (never user input at this point), so regexp.QuoteMeta on each piece is enough; no new
// grammar is being introduced (memory/feedback_no_new_regexp_parsers.md's own concern doesn't
// apply -- this locates a line for a mechanical rewrite, it doesn't parse one). Deliberately
// excludes a line that already has its own trailing "as ...", by construction (the pattern ends
// right after the leaf) -- an already-assigned capability's line simply won't match, which is
// exactly the behaviour wanted (never assign twice).
//
// The "capability" keyword (PROJECT.md item 11b) is allowed in front; an "integration mqtt"
// capability (raw, PROJECT.md 39) is matched by its topic form instead of a leaf.
func discoveryCapabilityLinePattern(domain, label, sourceDomain, leaf, rawTopic string) *regexp.Regexp {
	source := regexp.QuoteMeta(leaf)
	if sourceDomain != "" {
		source = regexp.QuoteMeta(sourceDomain) + ":" + source
	}
	if rawTopic == impliedRawTopic {
		return regexp.MustCompile(`^\s*capability\s+` + regexp.QuoteMeta(domain) + `\.` + regexp.QuoteMeta(label) + `\s*;\s*$`)
	}
	if rawTopic != "" {
		source = `topic\s+"` + regexp.QuoteMeta(rawTopic) + `"\s+on\s+"[^"]*"`
	}
	return regexp.MustCompile(`^\s*(?:capability\s+)?(?:hidden\s+|inverted\s+){0,2}` + regexp.QuoteMeta(domain) + `\.` + regexp.QuoteMeta(label) + `\s+` + source + `\s*;\s*$`)
}

// rewritePhysicalDefWithAssignedIDs locates each assignment's own capability line in the real,
// on-disk Physical.def (never the parser's own preprocessed/joined-line copy, which may not match
// the file's real layout 1:1 after one-liner-wrapping normalisation) and appends " as <id>"
// before its trailing ";". All-or-nothing: if ANY assignment's pattern doesn't match exactly one
// line, NOTHING is written -- a partial write would leave Physical.def in a half-assigned state
// that's harder to reason about than just trying again next run once the ambiguity (almost
// certainly a hand-edit mid-flight) is resolved.
func rewritePhysicalDefWithAssignedIDs(definitionDir string, assignments []tStableIDAssignment) (assigned []tStableIDAssignment, warnings []string) {
	path := filepath.Join(definitionDir, "Physical.def")
	lines, err := readDefStatementLines(path) // one line per statement (PROJECT.md 22c)
	if err != nil {
		return nil, []string{fmt.Sprintf("could not read %s to assign stable ids: %v", path, err)}
	}

	type pendingEdit struct {
		lineIdx int
		a       tStableIDAssignment
	}
	var pending []pendingEdit
	for _, a := range assignments {
		pattern := discoveryCapabilityLinePattern(a.domain, a.label, a.sourceDomain, a.leaf, a.rawTopic)
		matchIdx, matchCount := -1, 0
		for i, line := range lines {
			if pattern.MatchString(line) {
				matchCount++
				matchIdx = i
			}
		}
		if matchCount != 1 {
			warnings = append(warnings, fmt.Sprintf("found %d matching Physical.def lines for device %q's %q capability (want exactly 1) -- skipping this run's stable-id assignment entirely rather than guessing", matchCount, a.gatewayID, a.label))
			return nil, warnings
		}
		pending = append(pending, pendingEdit{lineIdx: matchIdx, a: a})
	}

	for _, edit := range pending {
		lines[edit.lineIdx] = appendAssignedIDToLine(lines[edit.lineIdx], edit.a.newID)
		assigned = append(assigned, edit.a)
	}

	if err := writeDefFormatted(path, lines); err != nil {
		return nil, []string{fmt.Sprintf("could not write %s with assigned stable ids: %v", path, err)}
	}
	return assigned, warnings
}

// bootstrapDiscoveryStableIDs (PROJECT.md item 7's ONE-TIME full bootstrap, run once per house via
// "-bootstrap_stable_ids") assigns "as <id>;" to every currently-live, not-yet-assigned discovery
// capability using its REAL, currently-assigned HA registry unique_id (registryUniqueIDs, keyed
// by entity_id) -- never a freshly-minted value, preserving exactly what's already live, per the
// design memo's own explicit rule ("never freshly minted for something already live"). A live
// capability with no corresponding entry in registryUniqueIDs is reported, not silently skipped
// or guessed -- that gap means something unexpected (post item-6-cleanup, expected to be rare)
// and deserves a human look.
func bootstrapDiscoveryStableIDs(definitionDir string, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, admin *TAdministrationState, registryUniqueIDs map[string]string) []string {
	var warnings []string

	liveEntityIDByGatewayLeaf := map[string]string{}
	entityIDs := make([]string, 0, len(admin.DiscoveryEntityLinks))
	for entityID := range admin.DiscoveryEntityLinks {
		entityIDs = append(entityIDs, entityID)
	}
	sort.Strings(entityIDs)
	for _, entityID := range entityIDs {
		link := admin.DiscoveryEntityLinks[entityID]
		key := link.GatewayDeviceID + "\x00" + link.Leaf
		if _, exists := liveEntityIDByGatewayLeaf[key]; !exists {
			liveEntityIDByGatewayLeaf[key] = entityID
		}
	}

	gatewayIDs := make([]string, 0, len(discoveryGatewaysByID))
	for id := range discoveryGatewaysByID {
		gatewayIDs = append(gatewayIDs, id)
	}
	sort.Strings(gatewayIDs)

	var toAssign []tStableIDAssignment
	for _, gatewayID := range gatewayIDs {
		gw := discoveryGatewaysByID[gatewayID]
		labels := make([]string, 0, len(gw.Capabilities))
		for label := range gw.Capabilities {
			labels = append(labels, label)
		}
		sort.Strings(labels)
		for _, label := range labels {
			cap := gw.Capabilities[label]
			if cap.Leaf == "" || cap.AssignedID != "" {
				continue
			}
			entityID, isLive := liveEntityIDByGatewayLeaf[gatewayID+"\x00"+cap.Leaf]
			if !isLive {
				continue
			}
			uniqueID, known := registryUniqueIDs[entityID]
			if !known {
				warnings = append(warnings, fmt.Sprintf("device %q's %q capability (entity %q) is live but has no matching entry in the supplied registry mapping -- skipping, not assigning", gatewayID, label, entityID))
				continue
			}
			toAssign = append(toAssign, tStableIDAssignment{
				gatewayID: gatewayID, domain: cap.Domain, label: label,
				sourceDomain: cap.SourceDomain, leaf: cap.Leaf, newID: uniqueID,
			})
		}
	}

	if len(toAssign) == 0 {
		return warnings
	}

	assigned, rewriteWarnings := rewritePhysicalDefWithAssignedIDs(definitionDir, toAssign)
	warnings = append(warnings, rewriteWarnings...)
	for _, a := range assigned {
		warnings = append(warnings, fmt.Sprintf("bootstrapped stable id %q for device %q's %q capability", a.newID, a.gatewayID, a.label))
	}
	return warnings
}

// appendAssignedIDToLine replaces line's own trailing ";" with " as <id>;" -- the line's own text
// up to that point is left untouched, matching this file's own header comment that this is a
// purely additive edit.
func appendAssignedIDToLine(line, id string) string {
	trimmed := strings.TrimRight(line, " \t")
	if !strings.HasSuffix(trimmed, ";") {
		return line
	}
	return strings.TrimSuffix(trimmed, ";") + " as " + id + ";"
}

// rawTopicForLineSearch is what discoveryCapabilityLinePattern looks for in a raw capability's line.
func rawTopicForLineSearch(raw TRawTopicCapability) string {
	if raw.Implied {
		return impliedRawTopic
	}
	return raw.Topic
}
