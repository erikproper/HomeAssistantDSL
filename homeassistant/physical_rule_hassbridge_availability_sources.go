/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Physical-layer hard-enforcement rules
 *
 * A hassbridge capability's source may carry the " is available" sugar (sourceToJinja2/
 * bareEntityFromSource, remote_instance_automations.go) to declare a condition-based entity
 * (almost always the device's own "node" liveness capability) instead of a plain relayed value --
 * e.g. "binary_sensor.node: sensor.fritz_box_7590_ax_gb_received is available;". The referenced
 * entity_id is a raw, hand-typed literal -- unlike the "discovery" gateway mechanism's own
 * "<domain>.<label> is available;" sibling reference (registerDiscoveryAvailabilityEntityLink,
 * Conceptual_DiscoveryEntities.go), which resolves against a same-device capability-label lookup
 * and errors clearly on a typo, nothing anywhere checks this literal string against reality: it
 * flows straight through registerDeviceSourceEntityLink/sourceToJinja2 into a Jinja
 * states('...') call with no validation at all. A typo there produces no build/generate error or
 * warning -- just a condition that's permanently 'unavailable', discovered only by noticing the
 * entity never comes online. Real incident, 2026-09-15.
 *
 * validateHassBridgeAvailabilitySources closes the gap for the one case that's staticly checkable
 * without live HA/MQTT access: every real "... is available;" usage in both houses' Physical.def
 * today points at an entity_id ALSO declared as a plain (non-"is available") source of one of that
 * SAME device's other capabilities -- the device's own "node" tracks its own "core" reading, or
 * whichever sibling capability makes it meaningfully "available" (see e.g. node.vienna: "binary_
 * sensor.node: sensor.processor_use is available;" alongside "sensor.cpu/load: sensor.processor_
 * use;" on the same device). This mirrors the discovery mechanism's own same-device-sibling
 * discipline, adapted to raw-string comparison since hassbridge capabilities have no label
 * registry to look sibling references up in.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 15.09.2026
 *
 */

package main

import (
	"fmt"
	"sort"
	"strings"
)

// resolveHassBridgeAvailabilityLabelReferences rewrites a hassbridge capability's "<label> is
// available;" source -- where <label> is not a raw entity_id but this SAME device's own sibling
// capability label -- into the fully-resolved "<raw-entity-id> is available;" shape every
// downstream consumer (sourceToJinja2, bareEntityFromSource, validateHassBridgeAvailabilitySources
// below) already expects (2026-09-25, the user's own explicit request: "utility.somfy_living_room_
// front"'s own "cover.cover" capability's OWN label, "cover", used directly in
// "binary_sensor.node cover is available;" instead of spelling out its raw entity_id a second
// time). Runs once, right after hassbridge devices are collected (generator.go, immediately before
// validateHassBridgeAvailabilitySources), so nothing downstream needs to know this shorthand
// exists at all.
//
// A source whose pre-suffix text already looks like a raw entity_id (contains a ".", which a bare
// capability label never does in this grammar) is left untouched -- label resolution only ever
// applies to a genuinely bare token. An unknown label, a label naming itself, or a label whose OWN source is itself
// another "is available" reference (matching validateHassBridgeAvailabilitySources' own "an
// is-available source contributes nothing here" rule) is left unresolved, surfacing as that
// validator's ordinary "likely a typo" warning unchanged.
func resolveHassBridgeAvailabilityLabelReferences(hassBridgeDevicesByID map[string]THassBridgeDevice) {
	for _, device := range hassBridgeDevicesByID {
		for label, capability := range device.Capabilities {
			token, isAvailability := strings.CutSuffix(capability.Source, discoveryAvailabilitySuffix)
			if !isAvailability || strings.Contains(token, ".") || token == label {
				continue
			}
			sibling, found := device.Capabilities[token]
			if !found || sibling.Source == "" {
				continue
			}
			if _, siblingIsAvailability := strings.CutSuffix(sibling.Source, discoveryAvailabilitySuffix); siblingIsAvailability {
				continue
			}
			capability.Source = sibling.Source + discoveryAvailabilitySuffix
			device.Capabilities[label] = capability
		}
	}
}

// validateHassBridgeAvailabilitySources checks every hassbridge device's "... is available;"
// capability sources against that SAME device's own other, plain (non-"is available") capability
// sources -- the referenced entity_id must match at least one of them, or it's flagged as a likely typo. Warning-only, like validateDerivedCapabilities (the node rule,
// validateEveryDeviceHasNode, is an error since 2026-11-02).
func validateHassBridgeAvailabilitySources(hassBridgeDevicesByID map[string]THassBridgeDevice) []string {
	var warnings []string

	deviceIDs := make([]string, 0, len(hassBridgeDevicesByID))
	for deviceID := range hassBridgeDevicesByID {
		deviceIDs = append(deviceIDs, deviceID)
	}
	sort.Strings(deviceIDs)

	for _, deviceID := range deviceIDs {
		device := hassBridgeDevicesByID[deviceID]

		capabilityLabels := make([]string, 0, len(device.Capabilities))
		for label := range device.Capabilities {
			capabilityLabels = append(capabilityLabels, label)
		}
		sort.Strings(capabilityLabels)

		// The pool of entity_ids an "is available" reference is expected to match one of --
		// every PLAIN (non-"is available") source this device declares. An "is available" source contributes nothing here, so a capability can never validate
		// against its own (or another capability's) availability reference, only a real value.
		knownEntities := map[string]bool{}
		for _, label := range capabilityLabels {
			source := device.Capabilities[label].Source
			if _, isAvailability := strings.CutSuffix(source, discoveryAvailabilitySuffix); source == "" || isAvailability {
				continue
			}
			knownEntities[bareEntityFromSource(source)] = true
		}

		for _, label := range capabilityLabels {
			entityID, isAvailability := strings.CutSuffix(device.Capabilities[label].Source, discoveryAvailabilitySuffix)
			if !isAvailability || knownEntities[entityID] || strings.HasPrefix(entityID, integrationSourcePrefix) {
				continue
			}
			warnings = append(warnings, fmt.Sprintf("device %q's %q capability's \"%s is available;\" doesn't match any of this device's OWN other capability sources -- likely a typo in the entity_id", deviceID, label, entityID))
		}
	}

	return warnings
}
