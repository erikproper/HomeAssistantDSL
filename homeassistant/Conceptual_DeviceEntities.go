/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualDeviceEntities
 *
 * Shared attribute/node registration helpers for "hosts" and "home_assistant" bridge devices --
 * registerHostAttributeEntity/registerHostNodeEntity/registerHassBridgeAttributeEntity, used by
 * Conceptual_DevicePositioning.go (device positioning, including a hosts device's unconditional
 * node) and Conceptual_DeviceCapabilityEntities.go (the "entity ... from <device-id> entity
 * <capability>;" construct and its "for"/merged-block shorthands). None of the entities these
 * register are generator-authored YAML -- they're expected to be created later by the MQTT
 * discovery coordinator (house_event_bus_coordinator), so they're registered as discovery-implied
 * (TEntityRecord.DiscoveryImplied), not generator-defined.
 *
 * This file owns none of the "what does a device's entity become" decision -- which domain (a
 * binary_sensor, a sensor, or something else), which path suffix ("/node" for hosts devices),
 * which attribute names -- that's entirely the linked device's own integration to decide
 * (integration_hosts_storage.go's THostsEntityMaterialization/MaterializationForIntegrationType
 * for "hosts" devices, Physical.def's own declared Capabilities for "home_assistant" bridge
 * devices). This file only resolves the DSL grammar's naming/typing bookkeeping.
 *
 * "device" is a genuine sphere-bearing domain like any other -- a device's own <spec> follows the
 * exact same "<sphere>:<path>" grammar as every other entity (normalizeEntityFullName), including
 * the space-relative-by-default / leading-"/"-for-absolute convention already used elsewhere
 * (e.g. "call providing /Y;"). "device" defaults to the infrastructural sphere (taxonomy.go's
 * SphereOf) when <spec> omits one, since a device is infrastructural by nature -- but the DSL
 * author still controls path placement (relative to the enclosing space, or absolute) exactly
 * as they would for any other entity. The materialised entities reuse that *resolved*
 * sphere/path verbatim -- so e.g. "device.infrastructural:/smarty" (absolute) yields
 * "binary_sensor.infrastructural:smarty/node", while a bare "device.smarty" declared inside
 * "space social:garage" would (like any other entity) yield ".../garage/smarty/node".
 *
 * Historical note (this file's original scope, retired 2026-09-01): this file used to own the
 * "entity device.<spec> from <device-id> with: <flag>[, <flag>...];" construct itself (bulk "all
 * entities" or a single explicit attribute-name flag) end to end -- parsing, dispatch, and
 * registration all in one. That construct, and the standalone "for <device-id>: ... end;"
 * shorthand it coexisted with, are both gone now, replaced everywhere (Junglinster and Vienna's
 * real Spaces.def included) by the merged "device <spec> from <device-id> with: <entity-spec>;
 * ...; end;" form (Conceptual_DevicePositioning.go's extractDeviceWithBlockHeader +
 * Conceptual_DeviceCapabilityEntities.go's expandForDeviceShorthandLine) -- see PROJECT.md's
 * unification plan. The registration *engine* below survived the retirement unchanged; only the
 * single-line "with: <flag>;" parsing/dispatch shell around it is gone.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 01.09.2026
 *
 */

package main

import (
	"fmt"
	"strings"
)

// deviceDisplayName builds a location-aware human-readable name for a device.<spec>'s shared
// HA "device:" block: "<resolved sphere>/<enclosing space's own local path>/<resolved path>",
// EXCEPT sphere "infrastructural" is omitted entirely -- it's the sphere nearly every device
// resolves to, so spelling it out on every single device name added visual noise without
// conveying anything (a "battery" or "connectivity"/"node" device is *always* infrastructural;
// the sphere only becomes informative for the rarer social/physical device). Purely cosmetic:
// the discovery "device:" block's Identifiers (deviceID, the DSL's own internal device id) never
// contain a sphere segment at all and are untouched by this, so this can never affect HA's own
// device-registry identity -- see Architecture.md §6.9's own "we trust identifiers, not names"
// framing. The entity ids themselves stay flat/absolute (no space context, by design -- see the
// file header), but the *display* name should still say where the thing conceptually lives: e.g.
// "device.social:/smarty" declared inside "space social:garage" resolves to sphere "social" /
// path "smarty", and spaceName "social/garage" -- dropping "social" (the space's own leading
// sphere token, meaningless once the device's own sphere is prepended) gives "garage", so the
// result is "social/garage/smarty". A device.<spec> declared at the top level (spaceName "root")
// has no space segment to insert. An "infrastructural" device in that same "garage" space instead
// yields the bare "garage/smarty", with no sphere segment at all.
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

// namingSpacePath returns the space path to resolve spec against: spacePath itself, unless
// deviceNamePath is non-empty AND spec names its sphere explicitly via ':' (PROJECT.md item 5,
// "device declaration acts like a space") -- in which case deviceNamePath is appended as one extra
// segment. See registerDeviceSourceEntityLink's own doc comment for why this must stay separate
// from administration.SpacePath itself (used unmodified for space bucketing). Always returns a
// fresh slice when it extends anything; never mutates spacePath's backing array.
//
// The explicit-sphere gate matters: a bare spec like "sensor.status" (no ':', domain's default
// sphere, the WHOLE remainder taken as path -- normalizeEntityFullName's own no-colon branch)
// carries no location semantics the DSL author asked for at all, unlike "sensor.physical:co2"'s
// explicit sphere + relative path. Real, deployed precedent for the bare form: Junglinster's own
// "device infrastructural:laserjet from hass.laserjet with: entity sensor.status from
// sensor.status; ...; end;" resolves to the flat "sensor.social_status", not
// "sensor.social_laserjet_status" -- auto-prepending deviceNamePath there would silently rename a
// real, already-onboarded HA entity. Confirmed via TestDeviceWithBlockMatchesTwoStatementFormForHassBridge.
//
// Also skipped when deviceNamePath equals spec's own domain (e.g. "device sphere:vacuum with:
// entity vacuum.social: from ...; end;") -- confirmed by the user 2026-09-10: a device literally
// named after its own domain (its leaf path just restates "this is a vacuum," not a distinguishing
// name the way "washing_machine"/"picture_frame" are) shouldn't have that domain repeated a
// second time in the entity's own path, purely redundant with the domain prefix already there --
// "vacuum.social_apartment_living_room_vacuum" should read "vacuum.social_apartment_living_room".
//
// The same suppression also fires when the domain matches only deviceNamePath's own LAST "/"
// segment (2026-09-15, generalising the exact rule above to a compound deviceNamePath) -- lets a
// device leaf like "main/light" carry the "light" discriminator ONLY where something else needs
// it to disambiguate (e.g. the auto-implied "node" capability's own path, which has no domain of
// its own to fall back on: "infrastructural/.../main/light/node"), while the light entity itself
// -- whose domain word already says "light" -- doesn't repeat it: "light.social:main" (explicit
// path "main", suppression makes deviceNamePath contribute nothing) resolves to plain ".../main",
// not the redundant ".../main/light". Real precedent: Vienna's hallway ceiling light, migrated off
// the old "call light_device light.social:main;" macro (whose own "providing ${entity.path}/light;"
// node-naming had exactly this same intent, by hand, before this rule existed to do it for a
// device-block positioning).
func namingSpacePath(spec string, spacePath []string, deviceNamePath string) []string {
	if deviceNamePath == "" || !specHasExplicitSpherePath(spec) {
		return spacePath
	}
	if hasEmptyDeviceLeafOverride(spec) {
		return spacePath
	}
	if dotIdx := strings.Index(spec, "."); dotIdx > 0 {
		domain := spec[:dotIdx]
		if domain == deviceNamePath {
			return spacePath
		}
		if lastSlash := strings.LastIndex(deviceNamePath, "/"); lastSlash >= 0 && domain == deviceNamePath[lastSlash+1:] {
			return spacePath
		}
	}
	extended := make([]string, len(spacePath), len(spacePath)+1)
	copy(extended, spacePath)
	return append(extended, deviceNamePath)
}

// specHasExplicitSpherePath reports whether spec (e.g. "sensor.physical:co2") names its sphere
// explicitly via a ':' after the domain, as opposed to a bare "sensor.status" form that resolves
// through lookupDefaultSphere's fallback with no location semantics of its own -- see
// namingSpacePath's own doc comment for why only the former is eligible for deviceNamePath's
// implicit path injection.
func specHasExplicitSpherePath(spec string) bool {
	dotIdx := strings.Index(spec, ".")
	if dotIdx <= 0 || dotIdx >= len(spec)-1 {
		return false
	}
	return strings.Contains(spec[dotIdx+1:], ":")
}

// hasEmptyDeviceLeafOverride reports whether spec uses the "sphere::path" double-colon form
// (2026-09-19) -- an explicit request to attach to the enclosing space's own context while
// skipping the enclosing device's own leaf entirely, e.g. "sensor.social::wind_speed" under a
// "device infrastructural:netatmo_windmeter with: ...;" block resolving to
// "sensor.social_terrace_wind_speed" (space context "terrace" only), not the usual
// "..._terrace_netatmo_windmeter_wind_speed" namingSpacePath would otherwise produce. Needs no
// change in normalizeEntityFullName itself: once namingSpacePath (this file) stops folding
// deviceNamePath in, that function's own existing "any leftover colon becomes a path separator,
// then a leading empty segment is trimmed" handling already resolves the second colon to nothing
// on its own. See project_device_leaf_naming_conceptual_mismatch_gap.md for the design context
// and the deferred "sphere:leaf:path" explicit-override form this doesn't yet cover.
func hasEmptyDeviceLeafOverride(spec string) bool {
	dotIdx := strings.Index(spec, ".")
	if dotIdx <= 0 || dotIdx >= len(spec)-1 {
		return false
	}
	remainder := spec[dotIdx+1:]
	colonIdx := strings.Index(remainder, ":")
	if colonIdx < 0 || colonIdx+1 >= len(remainder) {
		return false
	}
	return remainder[colonIdx+1] == ':'
}

func deviceDisplayName(spaceName, sphere, path string) string {
	var suffix string
	if spaceName == "" || spaceName == "root" {
		suffix = path
	} else {
		segments := strings.Split(spaceName, "/")
		if len(segments) > 1 {
			segments = segments[1:] // drop the space's own leading sphere token
		} else {
			segments = nil
		}
		if len(segments) == 0 {
			suffix = path
		} else {
			suffix = strings.Join(segments, "/") + "/" + path
		}
	}
	if sphere == "infrastructural" {
		return suffix
	}
	return sphere + "/" + suffix
}

// registerHostAttributeEntity registers one named attribute (e.g. "cpu/load", full "<group>/<leaf>"
// form -- see AttributeNames) of a "hosts" device's integration-type materialization into link --
// shared by "all entities" (which calls it once per mat.AttributeNames(device)) and a single
// explicit attribute-name flag (PROJECT.md 1.3b), so the two paths can never drift on
// typing/naming.
//
// attr's group half only ever drives the generated entity's own path segment (identical output
// whether the caller passes the group-prefixed or bare form, since it's reconstructed from the
// split either way); its leaf half is what actually keys link.AttributeEntityIDs and, downstream
// (integration_hosts_generator.go's devices.yaml "attribute_entities" map), the coordinator's own
// `{{ value_json.<leaf> }}` MQTT value_template -- that field name is fixed by the real
// hosts/<host>/cpu/state JSON payload's own key (posted by Integrations/cpu/report, entirely
// outside this DSL's naming choices), so it must stay the bare leaf regardless of how the
// capability is referenced in Spaces.def.
func registerHostAttributeEntity(administration *TAdministrationState, mat THostsEntityMaterialization, device THostDevice, deviceIdentity TEntityIdentity, spaceName, provenance string, link TDeviceConceptualLink, attr string) {
	spec := mat.AttributeSpec(attr)
	group, leaf := splitCapabilityName(attr)
	if group == "" {
		group = mat.AttributeSuffix
	}
	attrFullName := fmt.Sprintf("%s.%s/%s/%s/%s", mat.AttributeDomain, deviceIdentity.Sphere, deviceIdentity.Path, group, leaf)
	administration.RegisterDiscoveryImpliedEntity(spaceName, attrFullName, provenance, device.DeviceID+"!"+attr, false)
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
	link.AttributeEntityIDs[leaf] = TDeviceAttributeLink{
		EntityID:    toHomeAssistantEntityID(attrFullName),
		DeviceClass: deviceClass,
		Unit:        unit,
		StateClass:  stateClass,
		Icon:        icon,
		Identity:    extractEntityIdentity(attrFullName),
	}
}

// registerHostNodeEntity builds the base TDeviceConceptualLink for a "hosts" device: its node
// entity (unconditional -- every hosts device gets one, regardless of what attributes it declares
// or requests, unlike hassbridge's optional "node" capability) plus DisplayName/ConstantAttributes
// (mergedConstantAttributes, including the "as area" suggested_area default). Used by
// registerHostDevicePositioning (Conceptual_DevicePositioning.go, the "device <spec> from
// <device-id>;" light positioning form) so every hosts device's node/constant-attribute handling
// goes through one place. Returns the link plus any warnings
// (mergedConstantAttributes' own); the caller still owns registering per-attribute entities (if
// any) and writing the link into administration.DeviceConceptualLinks.
func registerHostNodeEntity(administration *TAdministrationState, mat THostsEntityMaterialization, device THostDevice, deviceIdentity TEntityIdentity, spaceName, displayName, provenance string) (TDeviceConceptualLink, []string) {
	nodeFullName := fmt.Sprintf("%s.%s/%s/%s", mat.NodeDomain, deviceIdentity.Sphere, deviceIdentity.Path, mat.NodeSuffix)
	administration.RegisterDiscoveryImpliedEntity(spaceName, nodeFullName, provenance, device.DeviceID+"!node", false)
	nodeDeviceClass, _, _, nodeIcon := resolveCapabilityDefaults(administration.CapabilityDefaults, mat.NodeDomain, mat.NodeSuffix)

	constantAttrs, warnings := mergedConstantAttributes(mat, device)
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

	return TDeviceConceptualLink{
		NodeEntityID:       toHomeAssistantEntityID(nodeFullName),
		NodeDeviceClass:    nodeDeviceClass,
		NodeIcon:           nodeIcon,
		DisplayName:        displayName,
		ConstantAttributes: linkConstantAttrs,
		AttributeEntityIDs: map[string]TDeviceAttributeLink{},
		HostIdentity:       deviceIdentity,
	}, warnings
}

// registerHassBridgeAttributeEntity registers one named capability (e.g. "production/current/power")
// of a "home_assistant" bridge device into link -- shared by "all entities" (which calls it once
// per declared capability) and a single explicit capability-name flag (PROJECT.md 1.3b), so the
// two paths can never drift on typing/naming.
func registerHassBridgeAttributeEntity(administration *TAdministrationState, device THassBridgeDevice, deviceIdentity TEntityIdentity, spaceName, provenance string, link TDeviceConceptualLink, attr string) {
	cap := device.Capabilities[attr]
	attrFullName := fmt.Sprintf("%s.%s/%s/%s", cap.Domain, deviceIdentity.Sphere, deviceIdentity.Path, attr)
	administration.RegisterDiscoveryImpliedEntity(spaceName, attrFullName, provenance, device.DeviceID+"!"+attr, false)
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
