/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: MissingEntitiesReport
 *
 * PROJECT.md item 1a (2026-09-21, "stable ID-based link" architecture): the shared, non-blocking
 * report every "known-not-to-exist" existence check (kind-2 discovery, kind-3 hassbridge, kind-4
 * import, kind-5 main-instance) feeds into, replacing the hard `./generate`-aborting errors those
 * four checks used to return. See memory: project_stable_discovery_identity_architecture.md --
 * principle 2: a vanished physical source must be REPORTED, not silently acted on, and never block
 * unrelated work. A single dead Zigbee battery or a rebooting router must not stop the operator
 * from regenerating the rest of the house.
 *
 * <outputRoot>/suggestions/missing.txt is the itemized half of that principle; the live half (a
 * "something's missing" binary_sensor in HA) is coordinator-side, see
 * house_event_bus_coordinator/missing_declared_entities.go.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 21.09.2026
 *
 */

package main

import (
	"os"
	"path/filepath"
	"strings"
)

// TMissingEntitiesSection is one existence-check kind's own confirmed-missing items (already
// formatted, human-readable strings -- each check function builds these exactly as it did when
// they were folded into a hard error message, unchanged).
type TMissingEntitiesSection struct {
	Kind     string
	Problems []string
}

// TMissingEntitiesReport collects every existence check's own confirmed-missing items across one
// `./generate` run, in whatever order Add is called (Physical_Generator.go calls it once per kind,
// in a fixed order, so output is deterministic run to run without needing its own sort here).
type TMissingEntitiesReport struct {
	Sections []TMissingEntitiesSection
}

// Add appends one kind's own problems, skipped entirely when empty so Render/Empty never have to
// special-case a zero-length section.
func (r *TMissingEntitiesReport) Add(kind string, problems []string) {
	if len(problems) == 0 {
		return
	}
	r.Sections = append(r.Sections, TMissingEntitiesSection{Kind: kind, Problems: problems})
}

// Empty reports whether every check kind came back clean this run.
func (r *TMissingEntitiesReport) Empty() bool {
	return len(r.Sections) == 0
}

// Render formats the report as plain text (not YAML -- matches suggestions/discovery.txt and
// suggestions/home_assistant_<name>.txt, which are copy-paste-Physical.def-text and plain warning
// text respectively, never real YAML either).
func (r *TMissingEntitiesReport) Render() string {
	var sb strings.Builder
	sb.WriteString("# Declared entities the coordinator has confirmed known-not-to-exist.\n")
	sb.WriteString("# This is a report only -- generation is never blocked by it (PROJECT.md item 1a).\n")
	sb.WriteString("# A source that's only temporarily offline will clear on its own once it reports again;\n")
	sb.WriteString("# a source that's genuinely gone needs its own Physical.def/Conceptual.def declaration\n")
	sb.WriteString("# fixed or removed by hand -- the coordinator will never do that for you.\n\n")
	for _, section := range r.Sections {
		sb.WriteString("## " + section.Kind + "\n")
		for _, problem := range section.Problems {
			sb.WriteString("- " + problem + "\n")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// generateMissingEntitiesReport writes <outputRoot>/suggestions/missing.txt, or removes any stale
// file from an earlier run when report is now empty -- the same "authoritative absence" convention
// every other suggestions file already follows (e.g. generateDiscoverySuggestions).
func generateMissingEntitiesReport(outputRoot string, report *TMissingEntitiesReport) error {
	path := filepath.Join(outputRoot, "suggestions", "missing.txt")
	if report.Empty() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return writeYAMLFile(path, report.Render())
}
