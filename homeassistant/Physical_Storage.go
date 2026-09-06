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

// TPhysicalGenerationContext bundles the per-run inputs an "integration ... with: ...;"
// handler may need, so the shared dispatch signature doesn't keep growing positional
// parameters as physical-layer output gains new cross-cutting concerns (this is already its
// fourth: outputRoot, then haOutputDir, then admin, now MQTT secrets).
type TPhysicalGenerationContext struct {
	OutputRoot     string                // house root (ping/hosts, coordinator/devices.yaml, cpu/secrets live there)
	HAOutputDir    string                // main instance's HA YAML tree (hass/<incarnation>), for output HA itself must consume (e.g. reporting automations)
	Admin          *TAdministrationState // conceptual layer's parsed state (Spaces.def), for integrations that look up conceptual-layer linkage (e.g. "hosts" devices via Conceptual_DeviceEntities.go)
	MQTTSecrets    TMQTTBrokerSecrets    // main MQTT broker connection secrets, resolveMQTTBrokerSecrets (defined.go) -- zero value when HasMQTTSecrets is false
	HasMQTTSecrets bool
	// MQTTBrokerProfiles holds every named "mqtt <name>: ...; end;" broker declared in
	// Physical.def (resolveMQTTBrokerProfiles, defined.go), keyed by name -- e.g. "main", "cloud".
	// Lets a "hosts" device opt into a specific broker (e.g. a laptop reporting to a cloud broker
	// instead of the house's local one) rather than always using MQTTSecrets ("main") implicitly.
	MQTTBrokerProfiles map[string]TMQTTBrokerSecrets
	// MQTTDiscoveryPhysicalPrefix is ${mqtt_discovery_physical} (resolveMQTTDiscoveryPhysicalPrefix,
	// defined.go) -- "" if not configured (no "discovery" integration in use yet).
	MQTTDiscoveryPhysicalPrefix string
	// MQTTDiscoveryConceptualPrefix is ${mqtt_discovery_conceptual} (resolveMQTTDiscoveryConceptualPrefix,
	// defined.go) -- the prefix the coordinator itself publishes conceptual-layer discovery
	// under; always set (defaults to "homeassistant", HA's own default), unlike
	// MQTTDiscoveryPhysicalPrefix.
	MQTTDiscoveryConceptualPrefix string
	// Installation is ${installation} (resolveInstallationName, defined.go) -- this house's own
	// short name (e.g. "junglinster"), used to qualify state/discovery topics this coordinator
	// publishes onto a shared cloud broker (mqtt_relay.go, coordinator side), so two
	// installations sharing one cloud broker's namespace never collide.
	Installation string
	// ImportedDevices are every "integration import with: device <local-id> from
	// <remote-installation> <remote-device-id> with: ...; end;" declaration
	// (integration_import_parser.go), pre-collected before the generic per-block dispatch loop
	// runs (mirroring how hassBridgeDevicesByID and instances are already pre-collected in
	// generatePhysicalIntegrationOutputs) since generateImportedDeviceFile needs them regardless
	// of "import"/"hosts"/"home_assistant" block order in Physical.def.
	ImportedDevices []TImportedDevice
}

// integrationBodyParsers maps an "integration <name>" name to the function that turns its
// body lines into generated output. Each integration owns its own local body syntax; only
// the generic "integration <name> [on <host>] with: ... end;" wrapper is parsed elsewhere
// (Physical_Parser.go).
var integrationBodyParsers = map[string]func(bodyLines []string, ctx TPhysicalGenerationContext) error{
	"hosts":       generateHostsIntegrationOutputs,
	"discovery":   generateDiscoveryIntegrationOutputs,
	"commandline": generateCommandlineIntegrationOutputs,
	// "import" is a no-op here deliberately -- generatePhysicalIntegrationOutputs pre-collects
	// every "import" block into ctx.ImportedDevices before this generic dispatch loop runs (see
	// TPhysicalGenerationContext.ImportedDevices' own doc comment) and writes
	// coordinator/imported.yaml from that list directly (generateImportedDeviceFile), independent
	// of "hosts"/"home_assistant" block order. Registered here only so the loop doesn't warn "no
	// parser registered" for it.
	"import": func(bodyLines []string, ctx TPhysicalGenerationContext) error { return nil },
}
