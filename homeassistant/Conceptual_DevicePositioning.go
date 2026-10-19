/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualDevicePositioning
 *
 * Parses and registers the conceptual layer's "device <device-id> with: ...; end;" construct (the
 * only way to position a device), plus a "with <entity-statement>;" one-liner sugar for the common
 * single-capability case, skipping the "end;" wrapper.
 *
 * PROJECT.md item 10 (2026-10-11): the construct used to also accept an optional "as <leaf-path>"
 * clause (and, via it, an optional "<sphere1>:<path1> <sphere2>:<path2> ..." multi-sphere form),
 * letting the DSL author give a device its own compound leaf. Once both real houses' Conceptual.def
 * no longer used it at all, the clause was dropped from the grammar outright -- the regexes below no
 * longer capture anything past "with"/"with:", so "as" is no longer valid syntax here. decl.Spec
 * itself stays (not removed) purely because applyColocatedPositioning (device_friendly_name.go)
 * still feeds it a COMPUTED leaf for a "colocated" Logical.def target -- never author-written text,
 * always a bare path with no sphere prefix at all, so registerDevicePositioning's own sphere-prefix
 * validation/multi-sphere resolution (which only ever existed for the "as" grammar) was removed
 * alongside it. deviceIdentity.Sphere still exists internally (normalizeEntityFullName's own
 * "device" taxonomy default, SphereOf["device"] == "infrastructural") purely because
 * DeviceIdentityOwner's collision key and registerHostDeviceIdentity's own signature still carry a
 * TEntityIdentity, but it's now a constant, never author-controlled, and never shown in a device's
 * own display text (deviceDisplayName takes no sphere argument at all any more,
 * Conceptual_DeviceEntities.go).
 *
 * The SAME device-id may appear in as many of these blocks as needed, in as many spaces as needed,
 * with no first/subsequent distinction in the grammar -- this is now the norm (a Z-Wave module
 * contributing entities to two different rooms plus its own infrastructural identity is a real,
 * common case, not a rare one). Registration is register-once-merge-repeatedly: the FIRST occurrence
 * of a device-id establishes its shared conceptual identity (DisplayName/ConstantAttributes/
 * suggested_area, from THAT occurrence's own enclosing space); every later occurrence just merges
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

// TDevicePositioningDeclaration is one parsed "device <device-id> with:" header (block or
// one-liner form).
type TDevicePositioningDeclaration struct {
	DeviceID string
	// Spec is a bare leaf path for this device's own identity, "" when there is none. The DSL
	// grammar itself no longer has any way to author a non-empty Spec (PROJECT.md item 10,
	// 2026-10-11: the "device <id> as <leaf> with:" clause was removed outright once both real
	// houses' Conceptual.def no longer used it) -- the only remaining source is
	// applyColocatedPositioning (device_friendly_name.go), which positions a "colocated"
	// Logical.def target under a COMPUTED leaf (the anchor's own leaf + the fact's own suffix),
	// not author-written text. Fed directly into the same normalizeEntityFullName("device."+Spec,
	// ...) machinery an entity reference uses; an empty Spec resolves to an empty leaf/path, same
	// as a device with no compound name at all.
	Spec string
}

// TDevicePositioningOccurrence is one recorded "device <id> [as ...] with:" occurrence --
// administration.DevicePositioningOccurrences's own element type. SpacePath is a defensive COPY of
// administration.SpacePath at the moment this occurrence was seen (the live slice is mutated by
// every later OpenSpace/CloseSpace as parsing continues, so the occurrence must not alias it).
// DeviceNamePath is decl.Spec run through deviceSpecLeafPath -- the exact same value parser.go
// itself threads into every one of this block's own body-line entity dispatches (forDeviceNamePath/
// oneLinerDeviceNamePath) -- so replaying an implied "node" registration against this occurrence
// resolves to precisely the same position a manually-written "node" entity would have.
type TDevicePositioningOccurrence struct {
	SpacePath      []string
	DeviceNamePath string
}

// deviceWithBlockHeaderPattern is "device <device-id> with:" -- a block header, parser.go's main
// loop tracks "currently inside this block, for which device-id".
var deviceWithBlockHeaderPattern = regexp.MustCompile(`^device\s+(\S+)\s+with:$`)

// extractDeviceWithBlockHeader recognises a "device <device-id> with:" block header.
func extractDeviceWithBlockHeader(line string) (*TDevicePositioningDeclaration, bool) {
	matches := deviceWithBlockHeaderPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil, false
	}
	return &TDevicePositioningDeclaration{DeviceID: matches[1]}, true
}

// deviceAtLeafWithBlockHeaderPattern is "device <device-id> at <leaf> with:" (2026-10-15) -- a
// NARROWLY new, separate mechanism from the "device <id> as <leaf> with:" grammar removed
// outright 2026-10-11 (PROJECT.md item 10): that one implicitly folded its leaf into EVERY
// entity line in the block with an explicit sphere-path, with no way to opt out line by line --
// exactly the blanket behaviour the user had just finished removing. This one is opt-in per
// line, via "::" (expandDeviceAtLeafDoubleColon, parser.go) -- a plain single ":" spec inside
// such a block is completely untouched, only an explicit "::" folds in <leaf>. Motivating case:
// a device-level "trigger <name> when <capability> is '<value>';" declaration
// (Conceptual_NamedTriggerEntities.go) implies a hidden raw entity with no author-given local
// path at all -- <leaf> is what that hidden entity's own path uses, matching whatever the
// pre-existing live entity's own path already was, so migrating a device to "trigger" form
// doesn't change the HA entity_id this generator's own files reference.
var deviceAtLeafWithBlockHeaderPattern = regexp.MustCompile(`^device\s+(\S+)\s+at\s+(\S+)\s+with:$`)

// extractDeviceAtLeafWithBlockHeader recognises a "device <device-id> at <leaf> with:" block
// header, returning the device id and the leaf separately (deviceAtLeafWithBlockHeaderPattern's
// own doc comment).
func extractDeviceAtLeafWithBlockHeader(line string) (deviceID, leaf string, ok bool) {
	matches := deviceAtLeafWithBlockHeaderPattern.FindStringSubmatch(line)
	if matches == nil {
		return "", "", false
	}
	return matches[1], matches[2], true
}

// expandDeviceAtLeafDoubleColon folds atLeaf into line's FIRST "::" occurrence, leaving every
// other colon (including a line with no "::" at all) completely untouched -- the opt-in
// distinction deviceAtLeafWithBlockHeaderPattern's own doc comment describes. "X::Y" becomes
// "X:<leaf>/Y"; "X::;" (nothing between "::" and the statement's own trailing punctuation)
// becomes "X:<leaf>;", not "X:<leaf>/;" -- no stray trailing slash.
func expandDeviceAtLeafDoubleColon(line, atLeaf string) string {
	idx := strings.Index(line, "::")
	if idx < 0 {
		return line
	}
	before := line[:idx+1] // up to and including the FIRST ':' of the pair
	after := line[idx+2:]
	if after == "" || after[0] == ';' || after[0] == ' ' {
		return before + atLeaf + after
	}
	return before + atLeaf + "/" + after
}

// deviceWithOneLinerPattern is "device <device-id> with <entity-statement>;" -- sugar for a
// single-capability block, skipping the "end;" wrapper. <entity-statement> is captured whole
// (group 2) and handed to the same bare "entity <spec> [with no_collect];"
// (forDeviceBareEntityPattern) / "entity <spec> from <capability> [with no_collect];"
// (forDeviceShorthandPattern) expansion a block's own body line already goes through --
// Conceptual_DeviceCapabilityEntities.go, unchanged.
var deviceWithOneLinerPattern = regexp.MustCompile(`^device\s+(\S+)\s+with\s+(entity\s+.+;)$`)

// extractDeviceWithOneLiner recognises the one-liner form, returning the positioning declaration
// plus the single embedded entity statement text (still ending in ";").
func extractDeviceWithOneLiner(line string) (*TDevicePositioningDeclaration, string, bool) {
	matches := deviceWithOneLinerPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil, "", false
	}
	return &TDevicePositioningDeclaration{DeviceID: matches[1]}, matches[2], true
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
	// Recorded for EVERY occurrence, including a later merge below -- this is a count of how many
	// different spaces decl.DeviceID is positioned in, which applyImpliedNodePositioning
	// (device_friendly_name.go, PROJECT.md item 8) needs regardless of the identity-level
	// register-once guard right below.
	administration.DevicePositioningOccurrences[decl.DeviceID] = append(administration.DevicePositioningOccurrences[decl.DeviceID], TDevicePositioningOccurrence{
		SpacePath:      append([]string{}, administration.SpacePath...),
		DeviceNamePath: deviceSpecLeafPath(decl.Spec),
	})

	// Already established (by an earlier occurrence, anywhere in the file) -- pure re-entry, this
	// block's own entities are registered by the ordinary per-line dispatch, nothing more to do here.
	if _, already := administration.DeviceConceptualLinks[decl.DeviceID]; already {
		return nil
	}

	provenance := fmt.Sprintf("%s:%d → device %s as %q", filepath.Base(entitiesPath), lineNum, decl.DeviceID, decl.Spec)

	// "infrastructural:" is re-inserted here, internally, purely to route through
	// normalizeEntityFullName's own well-tested EXPLICIT-sphere branch -- a colon-less
	// "device.<leaf-with-slashes>" (e.g. "device.radiator/aqara_thermostat", every compound leaf)
	// is textually indistinguishable from isExtensionalEntityReference's "already-resolved
	// domain.sphere/path" shape, and was being misdetected as one, returned verbatim with no
	// sphere-defaulting AND no space-context folding at all -- a real bug found live 2026-09-24
	// (silently flagged dozens of same-leaf-different-room devices, e.g. every
	// "radiator/aqara_thermostat", as colliding identities, and would have collapsed their own
	// "node" entities onto the same name too). decl.Spec is used directly (not deviceSpecLeafPath's
	// OWN output) -- normalizeEntityFullName's own absolute-path detection needs a leading '/'
	// preserved when present (e.g. a colocated target's own computed leaf), which deviceSpecLeafPath
	// deliberately trims for DISPLAY purposes only (a second real bug found live 2026-09-24 fixing
	// the one above: feeding the ALREADY-trimmed leaf in here lost that marker, silently folding the
	// enclosing space's own context into an "absolute" device's identity when it shouldn't be).
	identityLeaf := decl.Spec
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
