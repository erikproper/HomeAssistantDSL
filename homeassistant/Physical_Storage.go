/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: PhysicalStorage
 *
 * The registry of known physical-layer integrations: which "integration <name>" names the
 * generator understands, and which function turns that integration's parsed body into
 * output. Adding a future integration (netatmo, ems, ...) means adding its own
 * integration_<name>_{parser,storage,generator}.go and registering it here; nothing else
 * in the physical-layer dispatch (Physical_Parser.go, Physical_Generator.go)
 * should need to change.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 19.08.2026
 *
 */

package main

// integrationBodyParsers maps an "integration <name>" name to the function that turns its
// body lines into generated output. Each integration owns its own local body syntax; only
// the generic "integration <name> [on <host>] with: ... end;" wrapper is parsed elsewhere
// (Physical_Parser.go).
var integrationBodyParsers = map[string]func(bodyLines []string, outputRoot string) error{
	"hosts": generateHostsIntegrationOutputs,
}
