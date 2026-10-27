/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: LogicalSwitchedBy
 *
 * Implements Logical.def's "<domain>.<label> is switched by <domain>.<label> from <device-id>;"
 * device-level annotation (2026-09-27, replacing the old bash-era "definition as switched_device
 * <device> <main>;" Conceptual.def construct -- see the plan file/PROJECT.md for the full
 * design discussion). Written on a device XX that ALREADY declares <label> at the physical layer
 * (Physical.def/local-kind) -- e.g.
 *
 *   device sensors.server_room_main_spots with:
 *     light.core is switched by light.light_2 from switches.house_server_room_fuse_box_zwave_048;
 *   end;
 *
 * -- annotating XX's OWN "core" capability (a Zigbee2MQTT light group, the armature) as being
 * combined with switches.house_server_room_fuse_box_zwave_048's own "light_2" capability (the
 * in-wall relay gating its power, historically mis-assigned domain "light" too -- see the plan's
 * own "domain declared light, not the raw switch" note). This is metadata, NOT a new capability
 * definition: XX's "core" still registers exactly as it always would (an ordinary discovery-kind
 * capability); this file only ADDITIONALLY reacts once that registration succeeds.
 *
 * Reuses the existing "definition as switched_device" generation wholesale
 * (generateSwitchedDeviceEntities/generateSwitchedDeviceAutomations, generator.go) -- only how
 * administration.SwitchedDeviceRelations gets POPULATED changes. Both "Device" (XX's own raw
 * entity) and "MainEntity" (YY's referenced capability) are resolved via resolveSwitchedByEndpoint
 * below, which ALWAYS prefers an already-registered EXPLICIT Conceptual.def positioning of the same
 * (deviceID, label) over auto-materializing a fresh synthetic one -- the user's own correction
 * (2026-09-28, after a live collision on Junglinster's corridor/server_room pilot): an explicit
 * name, once given to a capability, always takes precedence over the "hidden" name generated when
 * that SAME capability is ALSO needed purely internally by this mechanism, so there must never be
 * two competing entities/MQTT discovery topics for one physical leaf. An EARLIER version of this
 * file always auto-materialized unconditionally, on the theory that reuse was "sphere-blind" and
 * risked self-reference for XX's own capability specifically (matching the very "social" entity
 * that triggers this whole mechanism) -- that risk is real for a NAIVE sphere-blind scan
 * (resolveLogicalSiblingSourceEntityID's tiers 2/3), but doesn't apply to
 * administration.ConceptualUseBySource: switchedBySuppressesEntity (below) already stops THIS
 * positioning's own raw entity from ever being registered under decl.DeviceID+"!"+bareName, so that
 * exact key can never resolve back to itself here -- it can only ever match a GENUINELY separate,
 * independently-authored physical-sphere positioning, exactly the case that must win.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 28.09.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
)

// resolveSwitchedByEndpoint resolves (deviceID, label)'s own physical-sphere entity for the "is
// switched by" mechanism (either XX's own annotated capability or YY's referenced source
// capability), preferring an EXISTING explicit Conceptual.def positioning over auto-materializing a
// fresh synthetic one -- see this file's own header comment for why that's always safe here.
// administration.ConceptualUseBySource is populated by RegisterDiscoveryImpliedEntity, called by
// EVERY ordinary capability-registration path in the codebase with the identical sourceKey
// convention (deviceID+"!"+label) used here, so this is a simple, direct lookup -- not a new
// resolution mechanism.
//
// finalAttempt gates the auto-materialize fallback -- NOT just whether one exists anywhere, but
// whether the parser has finished reading the WHOLE file yet. A real bug found live 2026-09-28: an
// explicit positioning elsewhere in the file (e.g. Conceptual.def line 1092, AFTER XX's own "is
// switched by"-triggering line 1084 in file order) hasn't been parsed yet on a first pass, so
// ConceptualUseBySource wouldn't have it -- but autoMaterializePhysicalCapability itself only needs
// the RAW Physical.def capability to exist (a static fact, true from the start), so it would
// SUCCEED immediately regardless of file order, permanently locking in the wrong (auto-materialized)
// entity before the explicit positioning ever got a chance to register. Deferring (ok=false) on
// every non-final attempt when no explicit positioning has appeared YET -- exactly like every other
// forward-reference in this codebase -- lets the standard single deferred-retry pass (which runs
// only after the ENTIRE file has been read once) see it. Falls back to auto-materialization (its own
// "!physical"-discriminated shadow entity) only once finalAttempt confirms no explicit positioning
// exists anywhere in the file. Returns the resolved entity's own FULLNAME (pre-sanitisation identity
// string, e.g. "light.physical:house/corridor/main" or the auto-materialized
// "light.physical/switches.zwave_048/2") -- required by generateSwitchedDeviceEntities
// (generator.go), which parses this via extractEntityIdentity to build the helper group's own name;
// a plain sanitised entity_id would not survive that round-trip (no colon/slash left to split on).
func resolveSwitchedByEndpoint(administration *TAdministrationState, deviceID, label, domain, role string, finalAttempt bool, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, hostDevicesByID map[string]THostDevice, localDevicesByID map[string]TLocalDevice, provenance string) (fullName string, ok bool) {
	if existing, found := administration.ConceptualUseBySource[deviceID+"!"+label]; found {
		return existing.EntityName, true
	}
	if !finalAttempt {
		return "", false
	}
	if _, materialized := autoMaterializePhysicalCapability(administration, deviceID, label, role, TEntityIdentity{}, discoveryGatewaysByID, hassBridgeDevicesByID, hostDevicesByID, localDevicesByID, provenance); !materialized {
		return "", false
	}
	return autoMaterializedFullName(coercedDomain(domain, role), domain, TEntityIdentity{Sphere: "physical", Path: deviceID}, label), true
}

// switchedBySuppressesEntity reports whether decl's own capability (already resolved to bareName)
// is annotated in Logical.def as the "is switched by" SelfEntity trigger for a non-physical-sphere
// positioning -- in which case the capability's own raw discovery/local entity generation must be
// SKIPPED by registerDeviceCapabilityEntityLinkAllowingHiddenCore, since the "is switched by"
// mechanism independently generates the FINAL combined group entity under this exact same target
// name (generateSwitchedDeviceEntities, generator.go). Letting both paths run doubly registers the
// same entity_id -- confirmed live 2026-09-28 (Junglinster's server_room/corridor pilot deploy):
// the raw discovery entity won the name, bumping the generator's own group to a "_2" suffix and
// never actually creating the intended switched-light entity under its real name at all. Mirrors
// maybeRegisterSwitchedByRelation's own "physical sphere is exempt" rule below (an explicit
// physical-sphere positioning is a user's own diagnostic entity, never the SelfEntity trigger) --
// duplicated rather than shared because this check must run BEFORE Core's own registration
// (registerDeviceCapabilityEntityLinkAllowingHidden, Conceptual_DeviceCapabilityEntities.go),
// whereas maybeRegisterSwitchedByRelation's fuller version runs AFTER it succeeds.
func switchedBySuppressesEntity(administration *TAdministrationState, decl TDeviceCapabilityEntityDeclaration, bareName, deviceNamePath string, logicalDevicesByID map[string]TLogicalDevice) bool {
	logicalDevice, hasLogical := logicalDevicesByID[decl.DeviceID]
	if !hasLogical {
		return false
	}
	if _, hasFact := logicalDevice.SwitchedBy[bareName]; !hasFact {
		return false
	}
	fullName := resolveDeviceEntityFullName(decl.LocalSpec, administration.SpacePath, deviceNamePath)
	return extractEntityIdentity(fullName).Sphere != "physical"
}

// maybeRegisterSwitchedByRelation is registerDeviceCapabilityEntityLinkAllowingHidden's own
// post-registration hook (see that function's own doc comment). No-ops immediately (nil, false)
// unless decl.DeviceID has a Logical.def TLogicalDevice with a SwitchedBy fact keyed by bareName --
// the overwhelmingly common case, so this stays cheap on every other capability registration in
// the file.
//
// identity.Sphere == "physical" means THIS registration IS the annotated capability's own raw
// physical-sphere entity (a user still free to hand-write one, e.g. for their own diagnostics) --
// nothing to combine yet, so this returns immediately; the relation is only ever built once a
// NON-physical-sphere positioning of the SAME (deviceID, bareName) capability is registered (the
// "social" sphere entity Conceptual.def actually positions for users), which becomes
// TSwitchedDeviceRelation.SelfEntity. By the time this runs, switchedBySuppressesEntity has already
// prevented Core from generating that positioning's own raw entity, so there is no competing
// registration left to collide with.
func maybeRegisterSwitchedByRelation(administration *TAdministrationState, decl TDeviceCapabilityEntityDeclaration, bareName, deviceNamePath string, logicalDevicesByID map[string]TLogicalDevice, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, hostDevicesByID map[string]THostDevice, localDevicesByID map[string]TLocalDevice, finalAttempt bool, entitiesPath string, lineNum int) (warnings []string, deferred bool) {
	logicalDevice, hasLogical := logicalDevicesByID[decl.DeviceID]
	if !hasLogical {
		return nil, false
	}
	fact, hasFact := logicalDevice.SwitchedBy[bareName]
	if !hasFact {
		return nil, false
	}

	fullName := resolveDeviceEntityFullName(decl.LocalSpec, administration.SpacePath, deviceNamePath)
	identity := extractEntityIdentity(fullName)
	if identity.Sphere == "physical" {
		return nil, false
	}

	provenance := fmt.Sprintf("%s:%d → %q (\"is switched by\")", filepath.Base(entitiesPath), lineNum, decl.LocalSpec)

	// switchedBySuppressesEntity (called from registerDeviceCapabilityEntityLinkAllowingHidden,
	// before Core ran) skipped Core's own registerDiscoveryEntityLink/registerLocalCapabilityEntityLink
	// call for THIS positioning -- which is also what would have declared fullName into
	// EntityRecordsBySpace (via RegisterDiscoveryImpliedEntity). Registered here instead --
	// deliberately BEFORE the Device/MainEntity resolution below, and unconditionally on every
	// attempt (not just once the relation itself fully resolves): a real regression found live
	// 2026-09-28 (kitchen workplace, the first switched-by site with no pre-existing explicit
	// "light on:" override to mask it) -- a space's own "apply light-on/off defaults" snapshot
	// (HandleEndToken, administration.go) runs SYNCHRONOUSLY the moment that space's own "end;" is
	// parsed, during the ordinary first pass; Device/MainEntity resolution below routinely defers
	// to the LATER finalAttempt-only retry pass (resolveSwitchedByEndpoint's own deferral, added
	// earlier this same session) whenever no explicit physical positioning exists yet for either
	// side -- the overwhelmingly common case. Registering fullName's own EXISTENCE only after that
	// later resolution succeeds meant the space's defaults snapshot had already run with this light
	// entirely missing, silently dropping the room's own generated aggregate light entity
	// (generateTemplateLights only writes one when SpaceOnByName ends up non-empty) for any space
	// that relied on the auto-collect fallback rather than an explicit override. The entity's own
	// NAME is fully known immediately (resolveDeviceEntityFullName, no deferral needed for that
	// part) even when its Device/MainEntity partners aren't resolved yet -- decoupling "declare I
	// exist as a light entity in this space" from "resolve my switched-by relation" fixes this.
	// Idempotent (RegisterDiscoveryImpliedEntity's own EntityRecordSeenBySpace check) if this ends
	// up called again on a later attempt for the same declaration.
	//
	// sourceKey carries a "!selfentity" discriminator (2026-09-28, node 15's own living/dining
	// migration -- a real bug found live) distinguishing THIS bookkeeping-only registration from an
	// ORDINARY explicit positioning of the exact same (deviceID, capability) elsewhere -- a
	// genuinely new scenario the server_room/corridor pilot never exercised: XX's own dimmer
	// capability gets BOTH the switched-by SelfEntity trigger (light.social:main) AND, separately, a
	// deliberate explicit "physical" sphere positioning (light.physical:main, kept stable for two
	// pre-existing non-DSL "dim up/down" automations). Without the discriminator, both registrations
	// share the bare "deviceID!capability" sourceKey and collide, firing a false-positive "already
	// used as a different conceptual entity" warning -- exactly the same false-positive class
	// autoMaterializePhysicalCapability's own "!physical" suffix (Conceptual_LogicalEntities.go)
	// already exists to prevent, just for a different pair of legitimate dual uses.
	administration.RegisterDiscoveryImpliedEntity(administration.CurrentSpaceName(), fullName, provenance, decl.DeviceID+"!"+bareName+"!selfentity", decl.NoCollect)

	deviceFullName, deviceOK := resolveSwitchedByEndpoint(administration, decl.DeviceID, bareName, fact.OwnDomain, "", finalAttempt, discoveryGatewaysByID, hassBridgeDevicesByID, hostDevicesByID, localDevicesByID, provenance)
	if !deviceOK {
		if !finalAttempt {
			return nil, true
		}
		return []string{fmt.Sprintf("%s: %q's own %q capability could not be materialized at the physical layer for \"is switched by\" (declare it in Physical.def first)", provenance, decl.DeviceID, bareName)}, true
	}
	mainFullName, mainOK := resolveSwitchedByEndpoint(administration, fact.SourceDeviceID, fact.SourceLabel, fact.SourceDomain, fact.OwnDomain, finalAttempt, discoveryGatewaysByID, hassBridgeDevicesByID, hostDevicesByID, localDevicesByID, provenance)
	if !mainOK {
		if !finalAttempt {
			return nil, true
		}
		return []string{fmt.Sprintf("%s: switch source %s.%s from %q could not be materialized at the physical layer (declare it in Physical.def first)", provenance, fact.SourceDomain, fact.SourceLabel, fact.SourceDeviceID)}, true
	}

	administration.SwitchedDeviceRelations = append(administration.SwitchedDeviceRelations, TSwitchedDeviceRelation{
		SpaceName:  administration.CurrentSpaceName(),
		DeviceID:   decl.DeviceID,
		SelfEntity: fullName,
		Device:     deviceFullName,
		MainEntity: mainFullName,
	})
	return nil, false
}
