/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualDeviceEntities
 *
 * Parses and registers the conceptual layer's "entity device.<spec> from <device-id> with:
 * <flag>[, <flag>...];" construct (Architecture.md §6.6): a link from a Spaces.def-declared
 * device to a Physical.def "hosts" integration device, implying a node availability entity
 * and, when "all entities" is given, one entity per variable attribute the
 * device's integration type materializes -- either hardwired per type (e.g. "cpu") or, when
 * the type hardwires none, per the device's own declared capabilities (e.g. "home_assistant";
 * see THostsEntityMaterialization.AttributeNames). None of these implied entities are
 * generator-authored YAML -- they're expected to be created later by the MQTT discovery
 * coordinator (house_event_bus_coordinator), so they're registered as discovery-implied
 * (TEntityRecord.DiscoveryImplied), not generator-defined.
 *
 * This file owns none of the "what does device.<spec> become" decision -- which domain (a
 * binary_sensor, a sensor, or something else), which path suffix ("/node" for hosts devices,
 * possibly nothing for a future device kind), which attribute names -- that's entirely the
 * linked device's own integration to decide (integration_hosts_storage.go's
 * THostsEntityMaterialization/MaterializationForIntegrationType for "hosts" devices). This
 * file only resolves the DSL grammar and the device's *position* (sphere/path).
 *
 * "device" is a genuine sphere-bearing domain like any other -- <spec> follows the exact same
 * "<sphere>:<path>" grammar as every other entity (normalizeEntityFullName), including the
 * space-relative-by-default / leading-"/"-for-absolute convention already used elsewhere (e.g.
 * "call providing /Y;"). "device" defaults to the infrastructural sphere (taxonomy.go's
 * SphereOf) when <spec> omits one, since a device is infrastructural by nature -- but the DSL
 * author still controls path placement (relative to the enclosing space, or absolute) exactly
 * as they would for any other entity. The materialised entities reuse that *resolved*
 * sphere/path verbatim -- so e.g. "device.infrastructural:/smarty" (absolute) yields
 * "binary_sensor.infrastructural:smarty/node", while a bare "device.smarty" declared inside
 * "space social:garage" would (like any other entity) yield ".../garage/smarty/node".
 *
 * This is the first cross-reference between Spaces.def-parsed entities and Physical.def's
 * THostDevice/DeviceID -- previously fully independent parse pipelines.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 20.08.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// TDeviceEntityDeclaration is one parsed "entity device.<spec> from <device-id> with:
// <flag>[, <flag>...];" line.
type TDeviceEntityDeclaration struct {
	DeviceSpec string   // "infrastructural:/smarty" from "device.infrastructural:/smarty" -- an ordinary <sphere>:<path> spec, resolved the same way any other entity's is
	DeviceID   string   // "host.smarty" from "from host.smarty"
	Flags      []string // e.g. ["all entities"]
}

var deviceEntityPattern = regexp.MustCompile(`^entity device\.(\S+)\s+from\s+(\S+)\s+with:\s*(.+);$`)

// extractDeviceEntityDeclaration recognises the "entity device.<spec> from <device-id>
// with: <flag>[, <flag>...];" shape. This is a single-line form with a colon but no
// matching "end;" -- neither of the two existing entity-body shapes
// (extractEntityDeclaration's " with " inline clause, or the multi-line "with:" ... "end;"
// block) matches it, so it must be recognised before either in the parse loop.
func extractDeviceEntityDeclaration(line string) (*TDeviceEntityDeclaration, bool) {
	matches := deviceEntityPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil, false
	}
	var flags []string
	for _, f := range strings.Split(matches[3], ",") {
		f = strings.TrimSpace(f)
		if f != "" {
			flags = append(flags, f)
		}
	}
	return &TDeviceEntityDeclaration{
		DeviceSpec: matches[1],
		DeviceID:   matches[2],
		Flags:      flags,
	}, true
}

// deviceDisplayName builds a location-aware human-readable name for a device.<spec>'s shared
// HA "device:" block: "<resolved sphere>/<enclosing space's own local path>/<resolved path>".
// The entity ids themselves stay flat/absolute (no space context, by design -- see the file
// header), but the *display* name should still say where the thing conceptually lives: e.g.
// "device.infrastructural:/smarty" declared inside "space social:garage" resolves to sphere
// "infrastructural" / path "smarty", and spaceName "social/garage" -- dropping "social" (the
// space's own leading sphere token, meaningless once the device's own sphere is prepended)
// gives "garage", so the result is "infrastructural/garage/smarty". A device.<spec> declared
// at the top level (spaceName "root") has no space segment to insert.
// deviceSpecLeafPath returns spec's own path portion, independent of the current space
// context: everything after the sphere's ":" separator, with a leading "/" (the "absolute,
// not space-relative" marker -- see normalizeEntityFullName) stripped. deviceDisplayName
// combines this with the current space's own location segments; deviceIdentity.Path must NOT
// be used for that instead, since normalizeEntityFullName already merges the space context
// into it for a space-relative spec -- doing so again in deviceDisplayName would double the
// location segments (e.g. ".../house/laundry_kitchen/rack/house/laundry_kitchen/rack/...").
func deviceSpecLeafPath(spec string) string {
	colonIdx := strings.Index(spec, ":")
	if colonIdx < 0 {
		return spec
	}
	return strings.TrimPrefix(spec[colonIdx+1:], "/")
}

func deviceDisplayName(spaceName, sphere, path string) string {
	if spaceName == "" || spaceName == "root" {
		return sphere + "/" + path
	}
	segments := strings.Split(spaceName, "/")
	if len(segments) > 1 {
		segments = segments[1:] // drop the space's own leading sphere token
	} else {
		segments = nil
	}
	if len(segments) == 0 {
		return sphere + "/" + path
	}
	return sphere + "/" + strings.Join(segments, "/") + "/" + path
}

// registerDeviceImpliedEntities resolves decl.DeviceSpec through the normal entity-naming
// machinery (domain "device", defaulting to the infrastructural sphere -- see taxonomy.go's
// SphereOf), then dispatches to whichever device kind decl.DeviceID resolves against --
// hostDevicesByID ("hosts" integration devices) or hassBridgeDevicesByID ("home_assistant"
// integration devices bridging entities from a named remote HA instance,
// integration_hassbridge_storage.go) -- each owning its own materialization logic
// (registerHostDeviceImpliedEntities / registerHassBridgeDeviceImpliedEntities below). This file
// decides none of that shape itself; that's entirely the linked device's own integration's call.
// Returns warnings; never aborts parsing.
func registerDeviceImpliedEntities(administration *TAdministrationState, decl TDeviceEntityDeclaration, hostDevicesByID map[string]THostDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, entitiesPath string, lineNum int) []string {
	provenance := fmt.Sprintf("%s:%d → device.%s from %s", filepath.Base(entitiesPath), lineNum, decl.DeviceSpec, decl.DeviceID)

	deviceIdentity := extractEntityIdentity(normalizeEntityFullName("device."+decl.DeviceSpec, administration.SpacePath))
	if deviceIdentity.Sphere == "" || deviceIdentity.Path == "" {
		return []string{fmt.Sprintf("%s: could not resolve a sphere/path from %q; skipping", provenance, decl.DeviceSpec)}
	}
	spaceName := administration.CurrentSpaceName()
	// displayName drives both the shared HA device block's name AND each of this device's
	// entities' own frontend names -- built consistently for both absolute and space-relative
	// device.<spec> forms, unlike deviceIdentity.Path (which only carries space context for a
	// relative spec; see deviceSpecLeafPath's doc comment). Deriving entity names from
	// deviceIdentity.Path instead would silently drop the space context for an absolute spec's
	// entities while the device block's own name kept it -- an inconsistency a user wouldn't
	// expect.
	displayName := deviceDisplayName(spaceName, deviceIdentity.Sphere, deviceSpecLeafPath(decl.DeviceSpec))

	if hostDevice, found := hostDevicesByID[decl.DeviceID]; found {
		return registerHostDeviceImpliedEntities(administration, decl, hostDevice, deviceIdentity, spaceName, displayName, provenance)
	}
	if hassDevice, found := hassBridgeDevicesByID[decl.DeviceID]; found {
		return registerHassBridgeDeviceImpliedEntities(administration, decl, hassDevice, deviceIdentity, spaceName, displayName, provenance)
	}
	return []string{fmt.Sprintf("%s: device %q not found in Physical.def's \"hosts\" or \"home_assistant\" integrations", provenance, decl.DeviceID)}
}

// registerHostDeviceImpliedEntities registers the node and (if "all entities" is
// present) attribute entities a "hosts" integration device's integration type materializes
// (integration_hosts_storage.go's MaterializationForIntegrationType) -- reusing the device's
// already-resolved sphere/path/displayName for all of them, plus the naming/typing metadata that
// materialization hardwires (manufacturer/model defaults, per-attribute device_class/unit/
// state_class).
func registerHostDeviceImpliedEntities(administration *TAdministrationState, decl TDeviceEntityDeclaration, device THostDevice, deviceIdentity TEntityIdentity, spaceName, displayName, provenance string) []string {
	var warnings []string

	mat, known := MaterializationForIntegrationType(device.IntegrationType)
	if !known {
		return []string{fmt.Sprintf("%s: integration type %q has no known entity materialization; skipping", provenance, device.IntegrationType)}
	}

	nodeFullName := fmt.Sprintf("%s.%s/%s/%s", mat.NodeDomain, deviceIdentity.Sphere, deviceIdentity.Path, mat.NodeSuffix)
	administration.RegisterDiscoveryImpliedEntity(spaceName, nodeFullName, provenance)
	nodeDeviceClass, _, _, nodeIcon := resolveCapabilityDefaults(administration.CapabilityDefaults, mat.NodeDomain, mat.NodeSuffix)

	constantAttrs, constantAttrWarnings := mergedConstantAttributes(mat, device)
	warnings = append(warnings, constantAttrWarnings...)
	// A device positioned inside an "as area" space gets that area as its suggested_area
	// default -- unless the device already has its own explicit override (device always wins,
	// same precedence mergedConstantAttributes itself already applies to every other field).
	if _, hasExplicit := constantAttrs["suggested_area"]; !hasExplicit {
		if area := administration.CurrentArea(); area != "" {
			constantAttrs["suggested_area"] = THostConstantAttribute{Value: area}
		}
	}
	linkConstantAttrs := make(map[string]TDeviceAttributeConstant, len(constantAttrs))
	for name, attr := range constantAttrs {
		linkConstantAttrs[name] = TDeviceAttributeConstant{Value: attr.Value, Forced: attr.Forced}
	}

	link := TDeviceConceptualLink{
		NodeEntityID:       toHomeAssistantEntityID(nodeFullName),
		NodeDeviceClass:    nodeDeviceClass,
		NodeIcon:           nodeIcon,
		DisplayName:        displayName,
		ConstantAttributes: linkConstantAttrs,
		AttributeEntityIDs: map[string]TDeviceAttributeLink{},
	}

	for _, flag := range decl.Flags {
		switch flag {
		case "all entities":
			attrNames := mat.AttributeNames(device)
			if len(attrNames) == 0 {
				// Only worth a warning when this integration type actually has the structural
				// capacity for variable attributes (AttributeDomain set, e.g. "home_assistant",
				// where an empty set usually means the device forgot to declare capabilities).
				// A type like "ping" has no such capacity at all -- liveness-only by design,
				// registering the node entity only is the expected, permanent, unremarkable
				// outcome, not a warning-worthy edge case.
				if mat.AttributeDomain != "" {
					warnings = append(warnings, fmt.Sprintf("%s: integration type %q has no hardwired variable device attributes and device declares no capabilities; registering the node entity only", provenance, device.IntegrationType))
				}
				continue
			}
			for _, attr := range attrNames {
				spec := mat.AttributeSpec(attr)
				group := mat.AttributeGroup(device, attr)
				attrFullName := fmt.Sprintf("%s.%s/%s/%s/%s", mat.AttributeDomain, deviceIdentity.Sphere, deviceIdentity.Path, group, attr)
				administration.RegisterDiscoveryImpliedEntity(spaceName, attrFullName, provenance)
				// Seed any field the hardwired spec (cpuAttributeSpecs etc.) leaves unset from
				// a "defaults: for ...;" rule or the code-level postfix table -- same
				// resolveCapabilityDefaults precedence as hassbridge capabilities.
				deviceClass, unit, stateClass, icon := spec.DeviceClass, spec.Unit, spec.StateClass, spec.Icon
				defaultDeviceClass, defaultUnit, defaultStateClass, defaultIcon := resolveCapabilityDefaults(administration.CapabilityDefaults, mat.AttributeDomain, attr)
				if deviceClass == "" {
					deviceClass = defaultDeviceClass
				}
				if unit == "" {
					unit = defaultUnit
				}
				if stateClass == "" {
					stateClass = defaultStateClass
				}
				if icon == "" {
					icon = defaultIcon
				}
				link.AttributeEntityIDs[attr] = TDeviceAttributeLink{
					EntityID:    toHomeAssistantEntityID(attrFullName),
					DeviceClass: deviceClass,
					Unit:        unit,
					StateClass:  stateClass,
					Icon:        icon,
					Identity:    extractEntityIdentity(attrFullName),
				}
			}
		default:
			warnings = append(warnings, fmt.Sprintf("%s: unrecognised flag %q; ignored", provenance, flag))
		}
	}

	administration.DeviceConceptualLinks[device.DeviceID] = link
	return warnings
}

// registerHassBridgeDeviceImpliedEntities registers one variable attribute entity per capability
// a "home_assistant" integration device declares (integration_hassbridge_storage.go) -- entities
// bridged live from a named remote HA instance, not generator-authored YAML, same
// discovery-implied treatment as every other device kind here. No node/liveness entity (this
// device kind doesn't monitor availability -- deliberately independent from any "hosts" ping
// device that might share the same conceptual position, per Architecture.md's deferred
// aggregate/absorb logical-layer work -- combining them is not this function's job). Constant
// device attributes (device.ConstantAttributes/DeviceInfoCapabilities, both validated against
// knownConstantDeviceAttributes at parse time already) DO carry through to link.ConstantAttributes
// now -- generateHassBridgeFile picks them up for the coordinator's device: block, the same
// mechanism hosts devices already have (mergedConstantAttributes), generalized to this kind too.
func registerHassBridgeDeviceImpliedEntities(administration *TAdministrationState, decl TDeviceEntityDeclaration, device THassBridgeDevice, deviceIdentity TEntityIdentity, spaceName, displayName, provenance string) []string {
	var warnings []string

	constantAttrs := make(map[string]TDeviceAttributeConstant, len(device.ConstantAttributes)+len(device.DeviceInfoCapabilities))
	for name, attr := range device.ConstantAttributes {
		constantAttrs[name] = TDeviceAttributeConstant{Value: attr.Value, Forced: attr.Forced}
	}
	// DeviceInfoCapabilities are dynamic (read live, reported by the automation) -- their DSL-known
	// half is just the name's *existence*, no static value to carry here; the coordinator learns
	// the actual value from the device-info report topic instead (see remote_instance_automations.go).

	link := TDeviceConceptualLink{
		DisplayName:        displayName,
		ConstantAttributes: constantAttrs,
		AttributeEntityIDs: map[string]TDeviceAttributeLink{},
	}

	for _, flag := range decl.Flags {
		switch flag {
		case "all entities":
			if len(device.Capabilities) == 0 {
				warnings = append(warnings, fmt.Sprintf("%s: device %q declares no capabilities; nothing to register", provenance, decl.DeviceID))
				continue
			}
			attrNames := make([]string, 0, len(device.Capabilities))
			for attr := range device.Capabilities {
				attrNames = append(attrNames, attr)
			}
			sort.Strings(attrNames)
			for _, attr := range attrNames {
				cap := device.Capabilities[attr]
				attrFullName := fmt.Sprintf("%s.%s/%s/%s", cap.Domain, deviceIdentity.Sphere, deviceIdentity.Path, attr)
				administration.RegisterDiscoveryImpliedEntity(spaceName, attrFullName, provenance)
				// Seed any of device_class/unit/state_class/icon the DSL left unset -- first from
				// Physical.def's/Defaults.def's own "defaults: for <domain>.<pattern>: ...;
				// end;" rules (user-declared), then from the capability path's code-level
				// postfix (e.g. "node" -> connectivity, "cpu/temperature" -> temperature) --
				// resolveCapabilityDefaults (capability_defaults.go). An explicit metadata line
				// on the capability itself always wins over both; this only fills gaps.
				deviceClass, unit, stateClass, icon := cap.DeviceClass, cap.Unit, cap.StateClass, cap.Icon
				defaultDeviceClass, defaultUnit, defaultStateClass, defaultIcon := resolveCapabilityDefaults(administration.CapabilityDefaults, cap.Domain, attr)
				if deviceClass == "" {
					deviceClass = defaultDeviceClass
				}
				if unit == "" {
					unit = defaultUnit
				}
				if stateClass == "" {
					stateClass = defaultStateClass
				}
				if icon == "" {
					icon = defaultIcon
				}
				link.AttributeEntityIDs[attr] = TDeviceAttributeLink{
					EntityID:    toHomeAssistantEntityID(attrFullName),
					DeviceClass: deviceClass,
					Unit:        unit,
					StateClass:  stateClass,
					Icon:        icon,
					Identity:    extractEntityIdentity(attrFullName),
				}
			}
		default:
			warnings = append(warnings, fmt.Sprintf("%s: unrecognised flag %q; ignored", provenance, flag))
		}
	}

	administration.DeviceConceptualLinks[decl.DeviceID] = link
	return warnings
}
