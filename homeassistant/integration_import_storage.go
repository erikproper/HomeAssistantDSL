/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationImportStorage
 *
 * The data model for Physical.def's unified device-import grammar: "device <local-id> from
 * <remote-installation> <remote-device-id> with: <domain>.<capability>; ... end;" -- pulling in
 * one device another installation exposed on the shared cloud broker, deliberately agnostic to
 * which integration kind ("hosts" or "home_assistant"/hassbridge) the exporting installation used
 * to declare it. This used to be two structurally unrelated forms (a bare positional "hosts" form
 * reusing THostDevice, and a "hassbridge"-only "with:" block), unified 2026-09-05 -- both kinds
 * already publish an HA-MQTT-discovery-shaped "cloud catalogue" entry per capability to the cloud
 * broker today (house_event_bus_coordinator/mqtt.go's publishDeviceDiscovery), so the importing
 * side never actually needed to know which kind it was consuming:
 * house_event_bus_coordinator/discoveryimport.go resolves everything (topic qualification,
 * JSON-blob extraction) from fields already present on that one shared payload shape.
 *
 * Per Architecture.md's own "Physical.def stays the naming authority" principle (the
 * "intensional device->space linkage" plan's design point D), an import still declares its exact
 * capability set up front -- each declared capability's name is combined with (RemoteInstallation,
 * RemoteDeviceID) to compute the exporter's own stable id (exportStableID,
 * house_event_bus_coordinator/discoveryhassbridge.go), which is what recognises which incoming
 * cloud discovery payload it corresponds to, the same "no auto-adopting anything undeclared"
 * discipline kind-2's own leaf declarations already follow. This is also what makes the resulting
 * local discovery topics fully knowable at generate time
 * (house_event_bus_coordinator/discoveryimport.go's expectedImportedTopics), avoiding the exact
 * class of orphan-topic-cleanup bug PROJECT.md 1.2b's own export side hit live 2026-08-30 (see
 * memory: project_native_export_cloud_crosspost.md) from trying to publish something the
 * coordinator's ambient "what belongs here" bookkeeping didn't know about ahead of time.
 *
 * An earlier, more verbose explicit form ("<local-capability>: <domain>.<remote-entity-local-
 * part>;", requiring the DSL author to look up and copy the exporter's own local entity_id by
 * hand) was removed outright on 2026-09-07 once no real Physical.def still used it -- see
 * integration_import_parser.go's own header comment.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 07.09.2026
 *
 */

package main

// TImportedCapability is one declared capability of an imported device -- "<domain>.<capability>;"
// inside a "device <local-id> from <remote-installation> <remote-device-id> with: ...; end;" block
// (integration_import_parser.go). Empty: the capability's own map key (its name) plus the owning
// TImportedDevice's (RemoteInstallation, RemoteDeviceID) are everything
// house_event_bus_coordinator/discoveryimport.go needs to derive the exporter's own stable id
// (exportStableID, discoveryhassbridge.go) and resolve which incoming cloud discovery payload it
// corresponds to -- there is nothing left to declare per capability beyond its presence.
//
// Replaced an earlier, more verbose explicit form ("<local-capability>: <domain>.<remote-entity-
// local-part>;", carrying the exporter's own already-positioned local entity_id as a
// RemoteEntityRef string field, matched against each incoming payload's "default_entity_id") on
// 2026-09-07, once no real Physical.def still used it -- see integration_import_parser.go's own
// header comment.
//
// Domain/DerivedFromCapability/DerivedViaTemplate (added 2026-09-10, plans/
// derived-capability-mechanism.md Phase 2) are only ever set for a "derived DDD.NNN from
// EEE.MMM via TTT;" capability -- a LOCALLY-synthesized capability with no upstream cloud
// discovery payload to match at all (unlike an ordinary imported capability, which has nothing
// left to declare beyond its own presence, per this type's own header comment above). Domain
// (DDD) has to be stored here, unlike an ordinary imported capability's domain (which always
// comes from wherever Spaces.def positions it): a derived capability has no exporter payload to
// inherit a domain from. DerivedFromCapability holds the sibling capability's own map key (MMM);
// DerivedViaTemplate is TTT, "$" standing for the sibling's own resolved value. All three empty
// for an ordinary imported capability -- unchanged behaviour from this type's previous empty-
// struct shape.
type TImportedCapability struct {
	Domain                string
	DerivedFromCapability string
	DerivedViaTemplate    string
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
	// DependsOnAvailabilityTopics is resolved generator-side (integration_logical_storage.go's
	// resolveDependencyAvailabilityTopics) from a Logical.def declaration sharing this same
	// DeviceID's own "dependency on <device-id>;" lines -- the raw MQTT topics the coordinator must
	// additionally AND into every one of this device's own capabilities' availability (including
	// its own "node"), already flattened across the full transitive dependency chain and
	// cycle-checked. Empty for a device with no matching Logical.def declaration -- unchanged
	// behaviour from before this field existed.
	DependsOnAvailabilityTopics []string
}
