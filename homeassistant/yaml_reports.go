/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: YAMLReports
 *
 * Generate's output for the YAML "mqtt:" entity writers (2026-10-28, Erik: "rather noisy"): each
 * writer reports one line per device ("<device>: <n> entities via YAML[; <note>...]"); printed are
 * only the lines that need attention (printYAMLReports).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 28.10.2026
 *
 */

package main

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	yamlCountReportPattern  = regexp.MustCompile(`^(\S+): (\d+) entities via YAML(.*)$`)
	yamlNothingPositioned   = ": no positioned entities -- nothing to write"
	yamlDroppedNotePrefix   = "dropped, unknown to Home Assistant: "
	yamlOwnAvailabilityNote = "own availability ignored: "
)

// printYAMLReports prints, prefixed with prefix (e.g. "[physical]"), only what needs attention among
// a writer's reports: a device's ERROR or WARNING note, or a report that isn't a plain count. Counts
// and the informational notes (dropped fields, ignored own availability, nothing positioned) are
// the same every run, and not printed (Erik, 2026-10-28).
func printYAMLReports(prefix string, reports []string) {
	for _, report := range reports {
		if strings.HasSuffix(report, yamlNothingPositioned) {
			continue
		}
		m := yamlCountReportPattern.FindStringSubmatch(report)
		if m == nil {
			fmt.Printf("%s %s\n", prefix, report)
			continue
		}
		var attention []string
		for _, note := range strings.Split(strings.TrimPrefix(m[3], "; "), "; ") {
			if note != "" && !strings.HasPrefix(note, yamlDroppedNotePrefix) && !strings.HasPrefix(note, yamlOwnAvailabilityNote) {
				attention = append(attention, note)
			}
		}
		if len(attention) > 0 {
			fmt.Printf("%s %s: %s\n", prefix, m[1], strings.Join(attention, "; "))
		}
	}
}
