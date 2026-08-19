/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationHostsStorage
 *
 * The data model and analysis helpers for the "hosts" integration's parsed devices:
 * THostDevice itself, plus the collection-level operations (deduplication, duplicate
 * detection) that generation (integration_hosts_generator.go) relies on. Parsing
 * (integration_hosts_parser.go) produces THostDevice values; this file doesn't parse or
 * generate anything itself.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 19.08.2026
 *
 */

package main

import (
	"fmt"
	"sort"
)

// THostDevice is one "device <id> <host> <type> [with: <capability>: <entity>; ... end;]"
// declaration from the "integration hosts" block.
type THostDevice struct {
	DeviceID        string
	HostName        string
	IntegrationType string            // "home_assistant", "cpu", or "ping"
	Capabilities    map[string]string // capability name -> Home Assistant entity id (home_assistant type only)
}

// dedupedHostNames returns every device's HostName, deduplicated and sorted, regardless
// of integration type -- home_assistant, cpu, and ping devices are all pingable hosts.
func dedupedHostNames(devices []THostDevice) []string {
	seen := map[string]bool{}
	var names []string
	for _, d := range devices {
		if d.HostName == "" || seen[d.HostName] {
			continue
		}
		seen[d.HostName] = true
		names = append(names, d.HostName)
	}
	sort.Strings(names)
	return names
}

// warnDuplicateDeviceIDs reports device ids declared more than once (e.g. copy-paste
// leftovers when Physical.def's device list was assembled from Integrations.def's
// earlier draft) -- the first declaration wins for the coordinator devices file, but the
// duplication itself is worth surfacing rather than silently resolving.
func warnDuplicateDeviceIDs(devices []THostDevice) []string {
	seen := map[string]bool{}
	var warnings []string
	for _, d := range devices {
		if seen[d.DeviceID] {
			warnings = append(warnings, fmt.Sprintf("device id %q is declared more than once; keeping the first declaration", d.DeviceID))
			continue
		}
		seen[d.DeviceID] = true
	}
	return warnings
}
