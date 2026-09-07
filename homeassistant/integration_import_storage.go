/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationImportStorage
 *
 * The data model for Physical.def's unified device-import grammar (2026-09-05 unification):
 * "device <local-id> from <remote-installation> <remote-device-id> with: <local-capability>:
 * <domain>.<remote-entity-local-part>; ... end;" -- pulling in one device another installation
 * exposed on the shared cloud broker, deliberately agnostic to which integration kind ("hosts" or
 * "home_assistant"/hassbridge) the exporting installation used to declare it. This used to be two
 * structurally unrelated forms (a bare positional "hosts" form reusing THostDevice, and a
 * "hassbridge"-only "with:" block) -- unified because both kinds already publish an
 * HA-MQTT-discovery-shaped "cloud catalogue" entry per capability to the cloud broker today
 * (house_event_bus_coordinator/mqtt.go's publishDeviceDiscovery), so the importing side never
 * actually needed to know which kind it was consuming: house_event_bus_coordinator/discoveryimport.go
 * resolves everything (topic qualification, JSON-blob extraction) from fields already present on
 * that one shared payload shape.
 *
 * Per Architecture.md's own "Physical.def stays the naming authority" principle (the
 * "intensional device->space linkage" plan's design point D), an import still declares its exact
 * capability set up front -- RemoteEntityRef is matched against each incoming exported discovery
 * payload's own "default_entity_id" field to recognise which capability arrived, the same "no
 * auto-adopting anything undeclared" discipline kind-2's own leaf declarations already follow. This
 * is also what makes the resulting local discovery topics fully knowable at generate time
 * (house_event_bus_coordinator/discoveryimport.go's expectedImportedTopics), avoiding the exact
 * class of orphan-topic-cleanup bug PROJECT.md 1.2b's own export side hit live 2026-08-30 (see
 * memory: project_native_export_cloud_crosspost.md) from trying to publish something the
 * coordinator's ambient "what belongs here" bookkeeping didn't know about ahead of time.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 05.09.2026
 *
 */

package main

// TImportedCapability is one declared capability of an imported device, in one of two forms:
//
//   - Explicit ("<local-capability>: <domain>.<remote-entity-local-part>;"): RemoteEntityRef is
//     the EXPORTING installation's own already-positioned local entity_id for it (e.g.
//     "sensor.junglinster_smarty_cpu_load"), matched against each incoming exported discovery
//     payload's "default_entity_id" field to recognise which capability a message belongs to. Not
//     a Home Assistant entity_id on *this* instance -- purely a foreign identifier used for
//     matching. Kind-agnostic: this same field works whether the exporting installation declared
//     the capability via "integration hosts" or "integration home_assistant" -- both give their
//     own entities a real "domain.entity_id" through their own Spaces.def positioning, and both
//     publish it verbatim as their cloud discovery payload's "default_entity_id".
//   - Shorthand ("<domain>.<capability>;", added 2026-09-07): RemoteEntityRef is left "" --
//     house_event_bus_coordinator/discoveryimport.go derives the remote stable id itself from
//     (RemoteInstallation, RemoteDeviceID, this capability's own map key) instead, matching
//     against the exporting coordinator's own EXPORT-side stable id
//     (house_event_bus_coordinator/discoveryhassbridge.go's exportStableID) rather than the
//     exporting installation's own local entity naming, which the DSL author would otherwise have
//     to already know and copy by hand.
type TImportedCapability struct {
	RemoteEntityRef string
}

// TImportedDevice is one "device <local-id> from <remote-installation> <remote-device-id> with:
// ... end;" declaration inside Physical.def's "integration import with: ... end;" block
// (integration_import_parser.go). RemoteDeviceID is the exporting installation's own DeviceID for
// the device (the key its own cloud-published discovery payloads' "device.identifiers[0]" carry)
// -- not necessarily equal to DeviceID, though keeping them equal (as Junglinster's own
// Vienna-device declarations do) makes cross-referencing the two installations' Physical.def files
// easier for a human, and isn't required by anything here.
type TImportedDevice struct {
	DeviceID           string
	RemoteInstallation string
	RemoteDeviceID     string
	Capabilities       map[string]TImportedCapability
}
