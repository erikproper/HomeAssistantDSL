/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationCommandlineStorage
 *
 * The data model for the "commandline" integration's parsed devices: hosts running the
 * mqtt_commandline daemon (Integrations/mqtt_commandline/), each exposing one or more
 * script-backed entities -- switch (status/on/off scripts), sensor (status script only), or
 * button (press script only). Unlike "hosts"/"home_assistant" capabilities, a commandline
 * capability's source is a local SCRIPT, not another entity's own state -- closer in shape to
 * hassbridge's own "bespoke per-kind data model" than to hosts' "reference another entity" one
 * (PROJECT.md item 1's own build-plan rationale for the file-per-concern split chosen here).
 *
 * Parsing (integration_commandline_parser.go) produces TCommandlineDevice values; this file
 * doesn't parse anything itself.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 06.09.2026
 *
 */

package main

// TCommandlineCapability is one "<kind>.<name>: <quoted-scripts>;" declaration inside a
// commandline device's "with: ... end;" block. Which of StatusScript/OnScript/OffScript/
// PressScript are populated depends on Kind:
//   - "sensor": StatusScript only.
//   - "switch": StatusScript, OnScript, OffScript (all three, in that order).
//   - "button": PressScript only.
// Each script is captured as one opaque quoted command-line string (path + args) -- the daemon
// does ordinary shell-word-splitting at invocation time, letting a single generic script be
// reused parametrically rather than needing one bespoke script per entity.
type TCommandlineCapability struct {
	Kind         string // "switch", "sensor", or "button"
	StatusScript string
	OnScript     string
	OffScript    string
	PressScript  string
}

// TCommandlineDevice is one "device <id> <host> with: <kind>.<name>: <quoted-scripts>; ...;
// end;" declaration inside Physical.def's "integration commandline with: ... end;" block --
// Host is the machine the mqtt_commandline daemon runs on (and the MQTT topic segment its wire
// shape uses, commandline/<host>/<entity>/...), Capabilities keyed by the declared local name
// (e.g. "slideshow", "reboot").
type TCommandlineDevice struct {
	DeviceID     string
	Host         string
	Capabilities map[string]TCommandlineCapability
}

// collectCommandlineDevicesByID re-parses Physical.def's "commandline" integration block and
// returns every declared device keyed by DeviceID, for callers that need lookup outside the
// Physical.def-native generation pipeline. First declaration wins for a duplicated id, matching
// collectHostsDevicesByID's own policy.
func collectCommandlineDevicesByID(definitionDir string) (map[string]TCommandlineDevice, []string) {
	physicalContent, mergedLineNos, warnings := collectLayerContent(definitionDir, []string{"Physical.def"}, LayerPhysical)
	blocks, blockWarnings := parseIntegrationBlocks(physicalContent, mergedLineNos)
	warnings = append(warnings, blockWarnings...)

	byID := map[string]TCommandlineDevice{}
	for _, block := range blocks {
		if block.Name != "commandline" {
			continue
		}
		devices, bodyWarnings := parseCommandlineIntegrationBody(block.BodyLines)
		warnings = append(warnings, bodyWarnings...)
		for _, d := range devices {
			if _, exists := byID[d.DeviceID]; !exists {
				byID[d.DeviceID] = d
			}
		}
	}
	return byID, warnings
}
