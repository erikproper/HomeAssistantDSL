/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: MQTTRoutingKeywords
 *
 * parseRoutingKeywords strips the trailing "via cloud" routing keyword of an "integration hosts"
 * device line (integration_hosts_parser.go). Semantics (Erik's own framing): the device's
 * communication (reporting, discovery) happens over the house's cloud broker profiles
 * ("cloud_client"/"cloud_coordinator" in Physical.def) instead of "main", and this installation
 * also self-imports it, as if it had written an ordinary "integration import" block pointing at
 * itself (see selfImportInstallation, integration_hosts_storage.go/integration_hosts_generator.go,
 * which resolve it into THostDevice.ImportedFrom).
 *
 * Written "cloud import" until PROJECT.md 30 (2026-11-03, Erik): "cloud" and "import" always came
 * together, so one "via cloud" says both. "cloud" alone -- visible only on the cloud broker, for
 * another installation to import -- was never used, and can no longer be written.
 *
 * Retired 2026-09-02: this file used to recognise "native" instead of "import" -- a bare boolean
 * with no declared identity, which silently assumed the declaring installation was always the
 * device's real owner. That assumption broke the moment the same device (e.g. host.mqtt) got
 * declared "cloud native" independently in a second house too -- only the house matching the
 * device's real reporting identity ever received anything (confirmed live, host.mqtt/
 * host.eriks-macbook-pro-2 permanently "unavailable" in Vienna). "import" replaces it with an
 * explicit, declared installation name (this installation's own, auto-resolved) everywhere,
 * closing that gap structurally rather than by convention/discipline alone.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 03.11.2026
 *
 */

package main

import "strings"

// parseRoutingKeywords strips a trailing " via cloud" from line, immediately before its final
// suffix (";" or "with:"), returning the line with it removed (suffix intact) and whether it was
// there. A line with neither suffix, or without the keyword, is returned unchanged.
func parseRoutingKeywords(line string) (rest string, viaCloud bool) {
	for _, suffix := range []string{";", "with:"} {
		body, ok := strings.CutSuffix(line, suffix)
		if !ok {
			continue
		}
		if body, ok = strings.CutSuffix(strings.TrimSpace(body), " via cloud"); ok {
			return strings.TrimSpace(body) + " " + suffix, true
		}
		return line, false
	}
	return line, false
}
