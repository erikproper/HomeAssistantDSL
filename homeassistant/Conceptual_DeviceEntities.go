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
	// A multi-sphere spec (2026-09-24, see isMultiSphereDeviceNamePath's own doc comment) is
	// preserved VERBATIM here -- deviceDisplayName never sees this value directly any more for
	// such a spec (registerDevicePositioning resolves the "infrastructural" entry separately for
	// display purposes instead), and every per-entity naming call
	// (registerXXXCapabilityEntityLink -> namingSpacePath) needs the full, unresolved text so it
	// can pick the entry matching THAT entity's own sphere.
	if isMultiSphereDeviceNamePath(spec) {
		return spec
	}
	colonIdx := strings.Index(spec, ":")
	if colonIdx < 0 {
		// No colon at all -- spec IS the leaf path outright. Still trim a leading '/' (the
		// "absolute, no space-prefix" leaf shape, e.g. "as /smarty") the same as the colon branch
		// below always has, so deviceDisplayName's own "spaceName + \"/\" + path" join never
		// doubles up the separator.
		return strings.TrimPrefix(spec, "/")
	}
	// Reached only for a spec built internally with a single "infrastructural:" prefix (e.g.
	// Conceptual_DeviceExportRegistration.go's own "infrastructural:/"+bareLocalName construction)
	// -- isMultiSphereDeviceNamePath deliberately excludes a lone "infrastructural:" entry (a
	// user-authored "as infrastructural:X" is rejected outright by registerDevicePositioning
	// instead, forcing the bare "as X" form), so this old single-strip behaviour still applies to
	// that internal caller.
	return strings.TrimPrefix(spec[colonIdx+1:], "/")
}

// isMultiSphereDeviceNamePath reports whether s is a sphere-qualified "as" clause -- one or more
// space-separated "<sphere>:<path>" tokens, each naming a KNOWN sphere (2026-09-24, confirmed with
// the user: unlike a device's own identity, which has no sphere of its own, a device's LEAF can
// legitimately differ per sphere -- e.g. a brand-qualifying suffix like "frient" is useful
// "infrastructural" grouping information but has no business leaking into a "physical"/"social"
// reading's own name: "device sensors.X as physical:front infrastructural:front/frient with: ...
// entity binary_sensor.physical:motion from core; entity binary_sensor.infrastructural:node; ...
// end;" gives the motion entity the plain "front" leaf while the node entity still gets
// "front/frient"). A SINGLE lone "infrastructural:<path>" entry does NOT count (returns false) --
// that's the redundant single-sphere form registerDevicePositioning rejects outright (a device has
// no sphere of its own in the single-sphere case, so it must be written as a bare "as <path>"
// instead); this function only recognises the genuinely multi-sphere-relevant shapes: 2+ tokens, or
// a single token naming a sphere OTHER than "infrastructural".
func isMultiSphereDeviceNamePath(s string) bool {
	tokens := strings.Fields(s)
	if len(tokens) == 0 {
		return false
	}
	if len(tokens) == 1 {
		colonIdx := strings.Index(tokens[0], ":")
		if colonIdx <= 0 {
			return false
		}
		sphere := tokens[0][:colonIdx]
		return isKnownSphere(sphere) && sphere != "infrastructural"
	}
	for _, tok := range tokens {
		colonIdx := strings.Index(tok, ":")
		if colonIdx <= 0 || !isKnownSphere(tok[:colonIdx]) {
			return false
		}
	}
	return true
}

// parseMultiSphereDeviceNamePath splits a multi-sphere "as" clause (isMultiSphereDeviceNamePath
// must already be true for s) into its own sphere->path entries, plus the order they were written
// in (order matters only as the deterministic fallback in resolveDeviceNamePathForSphere, when
// neither the requested sphere nor "infrastructural" has an entry).
func parseMultiSphereDeviceNamePath(s string) (entries map[string]string, order []string) {
	entries = map[string]string{}
	for _, tok := range strings.Fields(s) {
		colonIdx := strings.Index(tok, ":")
		sphere, path := tok[:colonIdx], tok[colonIdx+1:]
		if _, exists := entries[sphere]; !exists {
			order = append(order, sphere)
		}
		entries[sphere] = path
	}
	return entries, order
}

// resolveDeviceNamePathForSphere resolves deviceNamePath -- either a single bare leaf path (the
// common case, used unchanged for every entity regardless of its own sphere) or a multi-sphere "as"
// clause (isMultiSphereDeviceNamePath) -- to the ONE leaf path that applies to an entity of the
// given sphere. Falls back to the "infrastructural" entry, then to whichever entry was written
// first, if the requested sphere has no entry of its own -- a device positioning every capability
// under one or two spheres has no reason to also enumerate the rare third one.
func resolveDeviceNamePathForSphere(deviceNamePath, sphere string) string {
	if !isMultiSphereDeviceNamePath(deviceNamePath) {
		return deviceNamePath
	}
	entries, order := parseMultiSphereDeviceNamePath(deviceNamePath)
	if path, ok := entries[sphere]; ok {
		return path
	}
	if path, ok := entries["infrastructural"]; ok {
		return path
	}
	if len(order) > 0 {
		return entries[order[0]]
	}
	return ""
}

// explicitSphereOfSpec extracts an entity spec's own sphere (e.g. "physical" from
// "binary_sensor.physical:motion", "social" from the bare-known-sphere form "vacuum.social") --
// mirrors the sphere half of normalizeEntityFullName's own no-colon/colon parsing, kept separate
// since namingSpacePath needs the sphere ALONE, before any space-context folding, to pick the right
// entry out of a multi-sphere deviceNamePath (resolveDeviceNamePathForSphere). Returns "" for a
// spec with no explicit/known sphere at all (the domain-default-sphere bare form, e.g.
// "sensor.status") -- resolveDeviceNamePathForSphere's own "infrastructural" fallback applies then.
func explicitSphereOfSpec(spec string) string {
	dotIdx := strings.Index(spec, ".")
	if dotIdx <= 0 || dotIdx >= len(spec)-1 {
		return ""
	}
	remainder := spec[dotIdx+1:]
	colonIdx := strings.Index(remainder, ":")
	if colonIdx < 0 {
		if isKnownSphere(remainder) {
			return remainder
		}
		return ""
	}
	return remainder[:colonIdx]
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
	// Resolve a multi-sphere deviceNamePath (isMultiSphereDeviceNamePath) down to the one entry
	// matching THIS entity's own sphere before anything else below -- a no-op for the ordinary
	// single-leaf case (resolveDeviceNamePathForSphere returns deviceNamePath unchanged then).
	deviceNamePath = resolveDeviceNamePathForSphere(deviceNamePath, explicitSphereOfSpec(spec))
	if deviceNamePath == "" || !specHasExplicitSpherePath(spec) {
		return spacePath
	}
	if hasDeviceLeafOverride(spec) {
		return spacePath
	}
	if dotIdx := strings.Index(spec, "."); dotIdx > 0 {
		domain := spec[:dotIdx]
		// clipped is deviceNamePath with a trailing segment matching domain removed ("" if
		// deviceNamePath IS the domain outright, e.g. "vacuum" for a vacuum.* spec -- nothing
		// remains after clipping, same as the no-slash case immediately below).
		clipped := ""
		tailMatchesDomain := domain == deviceNamePath
		if !tailMatchesDomain {
			if lastSlash := strings.LastIndex(deviceNamePath, "/"); lastSlash >= 0 && domain == deviceNamePath[lastSlash+1:] {
				tailMatchesDomain = true
				clipped = deviceNamePath[:lastSlash]
			}
		}
		if tailMatchesDomain {
			// Real bug found live 2026-09-24 (Junglinster's "backups/switch" device, "entity
			// switch.social: from core;"): when spec's own leaf is non-empty ("light.social:main",
			// "switch.social:imac"), that leaf already supplies the entity's whole distinguishing
			// identity, so suppressing deviceNamePath entirely here is correct -- it would
			// otherwise duplicate ("main/light" + spec leaf "main" -> "..._main_light_main"). But
			// when spec's own leaf is EMPTY (a bare "switch.social:" relying entirely on the
			// device's own compound name for identity), full suppression leaves NO identity
			// source at all -- confirmed live: "backups/switch" resolved to the bare, collapsed
			// "switch.social_house_storage_room", indistinguishable from (and colliding with) any
			// other bare switch in that space. Fold in the CLIPPED deviceNamePath instead (the
			// domain-matching tail segment removed, same as "vacuum.social:"'s own domain-only
			// case already did before this fix -- clipped is "" there too, so behaviour for that
			// precedent is unchanged).
			if deviceSpecLeafPath(spec) != "" || clipped == "" {
				return spacePath
			}
			extended := make([]string, len(spacePath), len(spacePath)+1)
			copy(extended, spacePath)
			return append(extended, clipped)
		}
	}
	extended := make([]string, len(spacePath), len(spacePath)+1)
	copy(extended, spacePath)
	return append(extended, deviceNamePath)
}

// resolveDeviceEntityFullName is normalizeEntityFullName(spec, namingSpacePath(spec, spacePath,
// deviceNamePath))'s own shared wrapper, adding ONE more piece of domain-suppression on top: an
// entity's OWN declared path is stripped of a trailing segment that equals its own domain (e.g.
// "entity light.social:light;" reads as ".../main/light" once the device's own "social:main" leaf
// folds in, then drops the redundant trailing "light" the same way a domain-matching DEVICE leaf
// already does) -- confirmed with the user 2026-09-24: "it *must* be possible to use entity
// light.social:light with the clear intuition that... at the end the light at the end is stripped
// again," independent of namingSpacePath's own separate suppression for a domain-matching tail on
// the DEVICE's own leaf (a different source of the same redundancy).
//
// Deliberately scoped to deviceNamePath != "" (i.e. only when this entity is positioned inside a
// "device ... with:" block) rather than folded into normalizeEntityFullName itself as a blanket
// rule for every entity spec everywhere -- a real regression caught live: "entity
// vacuum.physical:vacuum from hass.roomba vacuum.roomba;" (remote_instance_entity_reporting_test.go)
// is a bare hassbridge reference with NO device leaf of its own, where "vacuum" is a genuine,
// meaningful qualifier that only coincidentally matches its own domain word -- stripping it there
// would be wrong, not redundant.
func resolveDeviceEntityFullName(spec string, spacePath []string, deviceNamePath string) string {
	extendedSpacePath := namingSpacePath(spec, spacePath, deviceNamePath)
	if deviceNamePath != "" {
		spec = stripEntityOwnTrailingDomainSegment(spec)
	}
	return normalizeEntityFullName(spec, extendedSpacePath)
}

// stripEntityOwnTrailingDomainSegment removes spec's own trailing path segment (the part after the
// sphere's ':') when it exactly equals spec's own domain -- "light.social:light" ->
// "light.social:", "light.social:main/light" -> "light.social:main". See
// resolveDeviceEntityFullName's own doc comment for why this is a separate, narrowly-scoped step
// rather than a change to normalizeEntityFullName's own general parsing.
func stripEntityOwnTrailingDomainSegment(spec string) string {
	dotIdx := strings.Index(spec, ".")
	if dotIdx <= 0 {
		return spec
	}
	domain := spec[:dotIdx]
	colonIdx := strings.Index(spec, ":")
	if colonIdx < 0 {
		return spec
	}
	pathPart := spec[colonIdx+1:]
	if lastSlash := strings.LastIndex(pathPart, "/"); lastSlash >= 0 {
		if pathPart[lastSlash+1:] == domain {
			return spec[:colonIdx+1] + pathPart[:lastSlash]
		}
		return spec
	}
	if pathPart == domain {
		return spec[:colonIdx+1]
	}
	return spec
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

// hasDeviceLeafOverride reports whether spec's sphere-prefixed remainder carries a SECOND colon
// beyond the one separating the sphere itself -- covering two related forms that both mean
// "don't fold the enclosing device's own leaf name into this entity's path," differing only in
// what (if anything) replaces it:
//
//   - "sphere::path" (double colon, empty leaf-override segment, 2026-09-19): the enclosing
//     device's own leaf is dropped entirely, e.g. "sensor.social::wind_speed" under a "device
//     infrastructural:netatmo_windmeter with: ...;" block resolves to
//     "sensor.social_terrace_wind_speed" (space context "terrace" only), not the usual
//     "..._terrace_netatmo_windmeter_wind_speed" namingSpacePath would otherwise produce.
//   - "sphere:leaf:path" (explicit replacement leaf, 2026-09-21): the enclosing device's own leaf
//     is replaced with an author-chosen alternative, e.g. "sensor.social:terrace:pressure" under
//     "device environment.weather with: ...;" (a ROOT-level device, no enclosing space of its own)
//     resolves to "sensor.social_terrace_pressure" -- "terrace" standing in for the (here
//     nonexistent/inapplicable) space context, not "weather" (the device's own internal name, which
//     the DSL author never wants exposed as a location word this outdoor reading didn't come from).
//
// Both forms need no special handling in normalizeEntityFullName itself: once namingSpacePath
// (this file) stops folding deviceNamePath in, that function's own existing "every remaining colon
// in the path portion becomes a '/' separator" logic already produces the right result from spec's
// own remainder alone -- an empty leading segment for the double-colon form (trimmed away), or the
// literal "leaf/path" segments for the explicit-override form. See
// project_device_leaf_naming_conceptual_mismatch_gap.md for the design history; both forms it
// describes are now implemented (the explicit-override one was deferred until this session's real
// environment.weather case needed it).
func hasDeviceLeafOverride(spec string) bool {
	dotIdx := strings.Index(spec, ".")
	if dotIdx <= 0 || dotIdx >= len(spec)-1 {
		return false
	}
	remainder := spec[dotIdx+1:]
	colonIdx := strings.Index(remainder, ":")
	if colonIdx < 0 {
		return false
	}
	return strings.Contains(remainder[colonIdx+1:], ":")
}

// deviceDisplayName has no sphere of its own to prepend: a device isn't an entity, so its "as
// SS:NN" clause's SS (needed only to compute deviceIdentity via the entity-identity machinery,
// extractEntityIdentity/normalizeEntityFullName) never shows up in the device's own display text
// -- confirmed by the user 2026-09-24: dropped even for the one non-"infrastructural" precedent
// (the 11 solar_panels devices, "as social:solar_panels..."), which used to read
// "social/house/.../solar_panels" and now reads "house/.../solar_panels" like everything else.
// Spaces/areas keep their own real sphere in their own display text -- see spaceAreaDisplayName,
// OpenSpace's own caller, which builds on the same suffix computed here.
func deviceDisplayName(spaceName, path string) string {
	if spaceName == "" || spaceName == "root" {
		return path
	}
	segments := strings.Split(spaceName, "/")
	if len(segments) > 1 {
		segments = segments[1:] // drop the space's own leading sphere token
	} else {
		segments = nil
	}
	if len(segments) == 0 {
		return path
	}
	return strings.Join(segments, "/") + "/" + path
}

// spaceAreaDisplayName is deviceDisplayName's own counterpart for a "space <spec> as area with:"
// block's HA Area name (OpenSpace, administration.go) -- unlike a device, a SPACE's own sphere is
// real, meaningful location semantics (almost always "social" for an "as area" space), so it keeps
// prefixing sphere when non-"infrastructural", exactly as deviceDisplayName itself used to before
// the user's 2026-09-24 clarification that devices (unlike spaces) don't have a sphere of their own.
func spaceAreaDisplayName(spaceName, sphere, path string) string {
	suffix := deviceDisplayName(spaceName, path)
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

// registerHostDeviceIdentity builds the base TDeviceConceptualLink for a "hosts" device:
// DisplayName/ConstantAttributes (mergedConstantAttributes, including the "as area" suggested_area
// default) and HostIdentity -- no node entity (2026-09-24: "node" is now an ordinary explicit
// capability reference for every kind, including hosts, never auto-registered at positioning time --
// see registerHostCapabilityEntityLink's own "node" branch, Conceptual_DeviceCapabilityEntities.go).
// Used by registerHostDevicePositioning so every hosts device's identity/constant-attribute handling
// goes through one place. Returns the link plus any warnings (mergedConstantAttributes' own); the
// caller still owns writing the link into administration.DeviceConceptualLinks.
func registerHostDeviceIdentity(administration *TAdministrationState, mat THostsEntityMaterialization, device THostDevice, deviceIdentity TEntityIdentity, displayName string) (TDeviceConceptualLink, []string) {
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
		DisplayName:        displayName,
		ConstantAttributes: linkConstantAttrs,
		AttributeEntityIDs: map[string]TDeviceAttributeLink{},
		HostIdentity:       deviceIdentity,
	}, warnings
}

// registerHostNodeAttribute resolves a "hosts" device's own "node" entity from an already-resolved
// fullName (its sphere/path) -- shared by registerHostCapabilityEntityLink's own explicit "node"
// reference (Conceptual_DeviceCapabilityEntities.go) and autoMaterializePhysicalCapability's
// on-demand synthetic materialization (Conceptual_LogicalEntities.go), so the two paths can never
// drift. Returns the resolved entityID plus typing metadata; the caller decides where to store them
// (a real DeviceConceptualLink's NodeEntityID/NodeDeviceClass/NodeIcon vs a synthetic one's).
func registerHostNodeAttribute(administration *TAdministrationState, mat THostsEntityMaterialization, device THostDevice, fullName, spaceName, provenance string) (entityID, deviceClass, icon string) {
	administration.RegisterDiscoveryImpliedEntity(spaceName, fullName, provenance, device.DeviceID+"!node", false)
	deviceClass, _, _, icon = resolveCapabilityDefaults(administration.CapabilityDefaults, mat.NodeDomain, mat.NodeSuffix)
	return toHomeAssistantEntityID(fullName), deviceClass, icon
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
