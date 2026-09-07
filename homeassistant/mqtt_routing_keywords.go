/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: MQTTRoutingKeywords
 *
 * parseRoutingKeywords strips the trailing "cloud"/"import" routing keywords a device or entity
 * declaration can carry anywhere in the physical layer (today: "integration hosts" device
 * lines -- integration_hosts_parser.go; other integrations' capability lines are meant to grow
 * the same trailing keywords over time, reusing this one helper rather than each inventing its
 * own). Semantics (the user's own framing): "cloud" means this declaration's communication
 * (reporting, discovery) happens over the house's cloud broker profiles
 * ("cloud_client"/"cloud_coordinator" in Physical.def) instead of "main", and by default is
 * *not* mirrored onto the local/main broker or its Home Assistant instance at all -- it only
 * becomes visible on the cloud broker's own (installation-qualified) namespace, for another
 * installation's coordinator to pick up via "integration import". Adding "import" alongside
 * "cloud" additionally mirrors it onto the local/main broker too, today's "fully local" default
 * behaviour just layered on top rather than replaced -- it means "and also self-import this
 * device, as if THIS installation had written an ordinary 'integration import' block pointing at
 * itself" (see selfImportInstallation, integration_hosts_storage.go/integration_hosts_generator.go,
 * which resolve the returned selfImport bool into THostDevice.ImportedFrom). "import" alone
 * (without "cloud") is meaningless and left unrecognised -- there's nothing to additionally mirror.
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
 * Version of: 02.09.2026
 *
 */

package main

import "strings"

// parseRoutingKeywords strips trailing " cloud" and/or " import" tokens (in either order) from
// line, immediately before its final suffix (";" or "with:"), returning the line with those
// tokens removed (suffix intact) alongside the flags found. line is returned unchanged (cloud
// and selfImport both false) if it doesn't end in one of the two recognised suffixes, or carries
// neither keyword.
func parseRoutingKeywords(line string) (rest string, cloud, selfImport bool) {
	suffix := ""
	switch {
	case strings.HasSuffix(line, ";"):
		suffix = ";"
	case strings.HasSuffix(line, "with:"):
		suffix = "with:"
	default:
		return line, false, false
	}

	body := strings.TrimSpace(strings.TrimSuffix(line, suffix))
	tokens := strings.Fields(body)
	for len(tokens) > 0 {
		switch tokens[len(tokens)-1] {
		case "cloud":
			cloud = true
			tokens = tokens[:len(tokens)-1]
		case "import":
			selfImport = true
			tokens = tokens[:len(tokens)-1]
		default:
			return strings.Join(tokens, " ") + " " + suffix, cloud, selfImport
		}
	}
	return strings.Join(tokens, " ") + " " + suffix, cloud, selfImport
}
