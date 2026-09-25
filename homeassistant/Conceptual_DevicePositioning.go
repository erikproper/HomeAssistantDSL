/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualDevicePositioning
 *
 * Parses and registers the conceptual layer's "device <device-id> [as <leaf-path>] with: ...;
 * end;" construct (2026-09-24 redesign -- see PROJECT.md item 11's own note for the full
 * rationale) -- the only way to position a device. Replaces three earlier, separately-keyworded
 * shapes ("device <spec> from <device-id>;", "device <spec> from <device-id> with: ...; end;",
 * "device from <device-id> with: ...; end;") with one: the device-id comes first (no more "from"),
 * "as <leaf-path>" is a direct stand-in for the old <spec> argument's own path half, and is itself
 * optional. Also new: a "with <entity-statement>;" one-liner sugar for the common single-capability
 * case, skipping the "end;" wrapper.
 *
 * A device has no sphere of its own (confirmed with the user 2026-09-24: spheres are an ENTITY
 * concept -- a device's "as" clause used to accept an optional "<sphere>:" prefix, defaulting to
 * infrastructural when omitted, but that was never more than an artifact of feeding decl.Spec
 * through the same normalizeEntityFullName("device."+spec, ...) machinery an entity reference
 * uses; nothing downstream ever needed a REAL, non-default sphere there). "as <sphere>:<leaf-path>"
 * is now rejected outright (registerDevicePositioning's own strings.Contains(decl.Spec, ":")
 * check) -- no dual-syntax transition, same discipline as every other grammar retirement in this
 * codebase. deviceIdentity.Sphere still exists internally (normalizeEntityFullName's own "device"
 * taxonomy default, SphereOf["device"] == "infrastructural") purely because DeviceIdentityOwner's
 * collision key and registerHostDeviceIdentity's own signature still carry a TEntityIdentity, but
 * it's now a constant, never author-controlled, and never shown in a device's own display text
 * (deviceDisplayName takes no sphere argument at all any more, Conceptual_DeviceEntities.go).
 *
 * The SAME device-id may appear in as many of these blocks as needed, in as many spaces as needed,
 * with no first/subsequent distinction in the grammar -- this is now the norm (a Z-Wave module
 * contributing entities to two different rooms plus its own infrastructural identity is a real,
 * common case, not a rare one). Registration is register-once-merge-repeatedly: the FIRST occurrence
 * of a device-id establishes its shared conceptual identity (DisplayName/ConstantAttributes/
 * suggested_area, using THAT occurrence's own "as" leaf, if any); every later occurrence just merges
 * its own entities into the same DeviceConceptualLinks[id].AttributeEntityIDs map, exactly like the
 * old "device from <id> with:" reuse form already did. There is no more "already positioned,
 * ignoring duplicate" guard for this construct -- a real double-declaration mistake is already
 * caught at the entity level (administration.ConceptualUseBySource's "already used as a different
 * conceptual entity" warning, validateNoDuplicateFinalEntityIDs), which is what actually matters;
 * the device-id-level guard was only ever a proxy for that.
 *
 * "node" is no longer auto-registered by ANY kind (including "hosts", historically unconditional --
 * a deliberate tradeoff, confirmed with the user: more verbose for the simple ping/cpu case, but the
 * mechanism doesn't have to know which specific integration a device happens to be). Every kind's
 * own node is now an ordinary explicit "entity binary_sensor.<sphere>:node;"-shaped capability
 * reference, dispatched through registerDeviceCapabilityEntityLink exactly like any other capability
 * -- including the Logical.def "node override" precedence check, which now lives at the top of
 * registerDeviceCapabilityEntityLinkAllowingHidden itself (Conceptual_DeviceCapabilityEntities.go),
 * not here.
 *
 * Registered the moment ParseEntitiesAndFillAdministration's single main loop (parser.go) reaches
 * this line -- no separate pre-pass or second scan of the file. A positioning line appearing *after*
 * something that references it still works: the ordinary deferred-retry mechanism
 * (registerDeviceCapabilityEntityLink's own "not positioned yet" -> deferred=true) already covers
 * every entity reference, this file needs no retry machinery of its own any more (no more auto-node
 * dispatch to defer).
 *
 * Scope: "home_assistant" bridge devices (native + hassbridge-import), "hosts" devices
 * (registerHostDevicePositioning, reusing registerHostDeviceIdentity -- Conceptual_DeviceEntities.go's
 * own materialization machinery), "discovery"/"local"/"commandline"/"import"-kind devices, and a
 * pure Logical.def device with no physical presence of its own.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 24.09.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// TDevicePositioningDeclaration is one parsed "device <device-id> [as <leaf-path>] with:" header
// (block or one-liner form).
type TDevicePositioningDeclaration struct {
	DeviceID string
	// Spec is the "as" clause's own text, verbatim -- "" if "as" was omitted entirely. A bare leaf
	// path only, no "<sphere>:" prefix (registerDevicePositioning rejects one outright, a device
	// has no sphere of its own). Fed directly into the same normalizeEntityFullName("device."+Spec,
	// ...) machinery the old <spec> argument always used; an empty Spec resolves to an empty
	// leaf/path, same as a device with no compound name ever had.
	Spec string
}

// deviceWithBlockHeaderPattern is "device <device-id> [as <leaf-path>] with:" -- a block header,
// parser.go's main loop tracks "currently inside this block, for which device-id/deviceNamePath".
// Group 2 is non-greedy and captures EVERYTHING between "as" and the trailing "with:" as one
// string, not just one token -- the multi-sphere "as <sphere1>:<path1> <sphere2>:<path2> ..." form
// (2026-09-24, see deviceSpecLeafPath's own doc comment) is several space-separated tokens.
var deviceWithBlockHeaderPattern = regexp.MustCompile(`^device\s+(\S+)(?:\s+as\s+(.+?))?\s+with:$`)

// extractDeviceWithBlockHeader recognises a "device <device-id> [as <leaf-path>] with:" block
// header.
func extractDeviceWithBlockHeader(line string) (*TDevicePositioningDeclaration, bool) {
	matches := deviceWithBlockHeaderPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil, false
	}
	return &TDevicePositioningDeclaration{DeviceID: matches[1], Spec: matches[2]}, true
}

// deviceWithOneLinerPattern is "device <device-id> [as <leaf-path>] with <entity-statement>;" --
// sugar for a single-capability block, skipping the "end;" wrapper. <entity-statement> is captured
// whole (group 3) and handed to the same bare "entity <spec> [with no_collect];"
// (forDeviceBareEntityPattern) / "entity <spec> from <capability> [with no_collect];"
// (forDeviceShorthandPattern) expansion a block's own body line already goes through --
// Conceptual_DeviceCapabilityEntities.go, unchanged.
var deviceWithOneLinerPattern = regexp.MustCompile(`^device\s+(\S+)(?:\s+as\s+(.+?))?\s+with\s+(entity\s+.+;)$`)

// extractDeviceWithOneLiner recognises the one-liner form, returning the positioning declaration
// plus the single embedded entity statement text (still ending in ";").
func extractDeviceWithOneLiner(line string) (*TDevicePositioningDeclaration, string, bool) {
	matches := deviceWithOneLinerPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil, "", false
	}
	return &TDevicePositioningDeclaration{DeviceID: matches[1], Spec: matches[2]}, matches[3], true
}

// registerDevicePositioning resolves decl into administration.DeviceConceptualLinks[decl.DeviceID]:
// on the FIRST occurrence of decl.DeviceID, establishes its shared conceptual identity
// (DisplayName/ConstantAttributes/suggested_area, from decl.Spec if given); on every later
// occurrence, this is a no-op (the entities this block's own body lines contribute are registered
// separately, by parser.go's ordinary per-line dispatch -- this function's only job is identity).
// See this file's own header comment for the full register-once-merge-repeatedly rationale.
// deviceNamePath is decl.Spec's own leaf path (computed once here, returned so the caller can
// thread it through this block's own body-line dispatch -- deviceSpecLeafPath("") is "", the "no
// compound name" case).
func registerDevicePositioning(administration *TAdministrationState, decl TDevicePositioningDeclaration, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hostDevicesByID map[string]THostDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, commandlineDevicesByID map[string]TCommandlineDevice, localDevicesByID map[string]TLocalDevice, logicalDevicesByID map[string]TLogicalDevice, entitiesPath string, lineNum int) []string {
	// Already established (by an earlier occurrence, anywhere in the file) -- pure re-entry, this
	// block's own entities are registered by the ordinary per-line dispatch, nothing more to do here.
	if _, already := administration.DeviceConceptualLinks[decl.DeviceID]; already {
		return nil
	}

	provenance := fmt.Sprintf("%s:%d → device %s as %q", filepath.Base(entitiesPath), lineNum, decl.DeviceID, decl.Spec)

	// A device has no sphere of its own (confirmed with the user 2026-09-24: entities have spheres,
	// devices don't) -- a SINGLE lone "as infrastructural:<leaf-path>" is still rejected outright
	// (redundant: the bare "as <leaf-path>" form already means exactly that), same no-dual-syntax
	// discipline as every other grammar retirement in this codebase. But a device's LEAF can
	// legitimately differ PER SPHERE (2026-09-24, a later, different reason than the one above --
	// see isMultiSphereDeviceNamePath's own doc comment): "as <sphere1>:<path1> <sphere2>:<path2>
	// ..." (2+ tokens, or a single token naming a sphere other than "infrastructural") IS accepted.
	for _, tok := range strings.Fields(decl.Spec) {
		colonIdx := strings.Index(tok, ":")
		if colonIdx < 0 {
			continue
		}
		sphere := tok[:colonIdx]
		if !isKnownSphere(sphere) {
			return []string{fmt.Sprintf("%s: malformed \"as\" clause -- %q names an unknown sphere (must be infrastructural/social/physical)", provenance, tok)}
		}
	}
	if !isMultiSphereDeviceNamePath(decl.Spec) && strings.Contains(decl.Spec, ":") {
		return []string{fmt.Sprintf("%s: \"as\" no longer accepts a sphere prefix on its own (a device has no sphere of its own) -- use a bare leaf path, e.g. \"as %s\", or pair it with another sphere's own entry if the leaf genuinely differs per sphere", provenance, strings.SplitN(decl.Spec, ":", 2)[1])}
	}

	// "infrastructural:" is re-inserted here, internally, purely to route through
	// normalizeEntityFullName's own well-tested EXPLICIT-sphere branch -- a colon-less
	// "device.<leaf-with-slashes>" (e.g. "device.radiator/aqara_thermostat", every compound leaf)
	// is textually indistinguishable from isExtensionalEntityReference's "already-resolved
	// domain.sphere/path" shape, and was being misdetected as one, returned verbatim with no
	// sphere-defaulting AND no space-context folding at all -- a real bug found live 2026-09-24
	// migrating both houses off the sphere-prefixed "as" grammar (silently flagged dozens of
	// same-leaf-different-room devices, e.g. every "radiator/aqara_thermostat", as colliding
	// identities, and would have collapsed their own "node" entities onto the same name too).
	// For a multi-sphere spec, the device's own IDENTITY (DisplayName/suggested_area/collision key)
	// is always the "infrastructural" entry (falling back to whichever entry was written first) --
	// resolveDeviceNamePathForSphere does exactly this resolution, and is a no-op for the ordinary
	// single-leaf case. Resolved from decl.Spec directly (not deviceSpecLeafPath's OWN output) --
	// normalizeEntityFullName's own absolute-path detection needs a leading '/' preserved when
	// present (e.g. "as /smarty"), which deviceSpecLeafPath deliberately trims for DISPLAY purposes
	// only (a second real bug found live 2026-09-24 fixing the one above: feeding the ALREADY-
	// trimmed leaf in here lost that marker, silently folding the enclosing space's own context
	// into an "absolute" device's identity when it shouldn't be).
	identityLeaf := resolveDeviceNamePathForSphere(decl.Spec, "infrastructural")
	deviceSpecForIdentity := identityLeaf
	if deviceSpecForIdentity != "" {
		deviceSpecForIdentity = "infrastructural:" + deviceSpecForIdentity
	}
	deviceIdentity := extractEntityIdentity(normalizeEntityFullName("device."+deviceSpecForIdentity, administration.SpacePath))
	// displayLeaf trims that same identityLeaf for deviceDisplayName's own simple text
	// concatenation, which always folds the enclosing space in regardless of an absolute marker
	// (a device's user-facing display name still wants its own location, even when its underlying
	// raw entity_id, for legacy-naming reasons, doesn't) -- deviceSpecLeafPath does exactly that
	// trim, and is a no-op (returns it unchanged) for a leaf that never had a leading '/' anyway.
	displayLeaf := deviceSpecLeafPath(identityLeaf)
	spaceName := administration.CurrentSpaceName()
	displayName := deviceDisplayName(spaceName, displayLeaf)

	// Two different device ids sharing the same conceptual identity ("weaving" two devices onto one
	// position, e.g. two Aqara sensors both "as infrastructural:apartment/hallway/door") is legal on
	// its own -- it's only a real mistake when it actually collides two entities onto the same final
	// HA entity_id, which RegisterDiscoveryImpliedEntity now catches and reports precisely, naming
	// the actual colliding entity (2026-09-25: this identity-level check used to hard-fail on the
	// shared position alone, which was a false positive whenever the two devices' own entity sets
	// didn't actually overlap -- see the user's own correction, Vienna's
	// "sensors.hallway_door_aqara_multi"/"sensors.hallway_aqara_windoor" both "as
	// infrastructural:apartment/hallway/door", which never collided at the entity level at all).

	if hostDevice, found := hostDevicesByID[decl.DeviceID]; found {
		mat, known := MaterializationForIntegrationType(hostDevice.IntegrationType)
		if !known {
			return []string{fmt.Sprintf("%s: integration type %q has no known entity materialization; skipping", provenance, hostDevice.IntegrationType)}
		}
		link, warnings := registerHostDeviceIdentity(administration, mat, hostDevice, deviceIdentity, displayName)
		administration.DeviceConceptualLinks[decl.DeviceID] = link
		return warnings
	}

	if _, found := importedDevicesByID[decl.DeviceID]; found {
		link := TDeviceConceptualLink{DisplayName: displayName, AttributeEntityIDs: map[string]TDeviceAttributeLink{}}
		if area := administration.CurrentArea(); area != "" {
			link.ConstantAttributes = map[string]TDeviceAttributeConstant{"suggested_area": {Value: area}}
		}
		administration.DeviceConceptualLinks[decl.DeviceID] = link
		return nil
	}

	if _, found := commandlineDevicesByID[decl.DeviceID]; found {
		administration.DeviceConceptualLinks[decl.DeviceID] = TDeviceConceptualLink{DisplayName: displayName, AttributeEntityIDs: map[string]TDeviceAttributeLink{}}
		return nil
	}

	if _, found := discoveryGatewaysByID[decl.DeviceID]; found {
		administration.DeviceConceptualLinks[decl.DeviceID] = TDeviceConceptualLink{DisplayName: displayName, AttributeEntityIDs: map[string]TDeviceAttributeLink{}}
		return nil
	}

	if _, found := localDevicesByID[decl.DeviceID]; found {
		administration.DeviceConceptualLinks[decl.DeviceID] = TDeviceConceptualLink{DisplayName: displayName, AttributeEntityIDs: map[string]TDeviceAttributeLink{}}
		return nil
	}

	if device, found := hassBridgeDevicesByID[decl.DeviceID]; found {
		constantAttrs := make(map[string]TDeviceAttributeConstant, len(device.ConstantAttributes))
		for name, attr := range device.ConstantAttributes {
			constantAttrs[name] = TDeviceAttributeConstant{Value: attr.Value, Forced: attr.Forced}
		}
		if _, hasExplicit := constantAttrs["suggested_area"]; !hasExplicit {
			if area := administration.CurrentArea(); area != "" {
				constantAttrs["suggested_area"] = TDeviceAttributeConstant{Value: area}
			}
		}
		administration.DeviceConceptualLinks[decl.DeviceID] = TDeviceConceptualLink{
			DisplayName:        displayName,
			ConstantAttributes: constantAttrs,
			AttributeEntityIDs: map[string]TDeviceAttributeLink{},
		}
		return nil
	}

	// A pure Logical.def device (no physical presence at all -- an "absorb"/dual-presence device
	// always matches one of the physical-kind branches above instead, whichever kind it physically
	// is) still gets a conceptual identity of its own, exactly like every physical kind above.
	if logical, found := logicalDevicesByID[decl.DeviceID]; found && len(logical.Capabilities) > 0 {
		administration.DeviceConceptualLinks[decl.DeviceID] = TDeviceConceptualLink{DisplayName: displayName, AttributeEntityIDs: map[string]TDeviceAttributeLink{}}
		return nil
	}

	return []string{fmt.Sprintf("%s: device %q not found in Physical.def's \"hosts\"/\"home_assistant\"/\"discovery\"/\"local\"/\"commandline\" integrations, Logical.def, or as a \"hassbridge\"-form import", provenance, decl.DeviceID)}
}
