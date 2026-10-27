/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualDeviceCapabilityEntities
 *
 * Parses and registers the conceptual layer's "entity <local-spec> from <device-id> <capability>;"
 * construct -- a unified alternative to the discovery integration's dot-joined "entity <spec> from
 * <gateway-id>.<leaf>;" (Conceptual_DiscoveryEntities.go) and the home_assistant bridge's "entity
 * <spec> as <source> from <device-id>;" (Conceptual_DeviceSourceEntities.go) forms, referencing a
 * device's own already-declared capability by its local label (the LHS of its "<type>.<label>:
 * <source>;" Physical.def line, e.g. "co2" from "sensor.co2: sensor.davids_bedroom_carbon_dioxide;")
 * regardless of which integration kind the device belongs to -- one syntax for "position this
 * specific entity of a device I've already declared," instead of a different keyword per device
 * kind. <capability> may carry a domain prefix ("sensor.co2") for readability at the call site;
 * it's stripped before lookup since neither the discovery gateway's nor the hassbridge device's own
 * Capabilities map is keyed by domain-prefixed names (both are keyed by the bare local label alone).
 *
 * PROJECT.md item 7 (2026-09-09): the RHS used to require a redundant literal "entity" keyword
 * before <capability> ("entity <spec> from <device-id> <capability>;") -- dropped outright
 * (no dual-syntax transition period, matching every other grammar retirement this project has done)
 * once it became clear nothing else could ever occupy that slot: the only other "entity ... from
 * ...;" shape with a single token after "from" is the discovery gateway's dot-joined form, which is
 * unambiguous against this construct's two-or-three-token shape regardless of the "entity" keyword.
 *
 * This file owns only the parsing + dispatch; the actual registration work is delegated entirely
 * to each device kind's own existing, unchanged function (registerDiscoveryEntityLink /
 * registerDeviceSourceEntityLink) once the device id resolves and the bare capability name is
 * looked up -- so this construct can never drift from what those two already-relied-upon paths do.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 27.08.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// TDeviceCapabilityEntityDeclaration is one parsed "entity <local-spec> from <device-id>
// <capability>;" line.
type TDeviceCapabilityEntityDeclaration struct {
	LocalSpec  string
	DeviceID   string
	Capability string // e.g. "sensor.co2" -- domain prefix optional, stripped before lookup (bareCapabilityName)
	// NoCollect excludes this entity from its space's domain-specific aggregate collections
	// (mirrors the plain-entity "no_collect" suffix, TEntityRecord.NoCollect) -- e.g. a smart
	// plug's own PCB temperature reading, which shares a "/temperature" subdomain with real room
	// climate sensors but must stay out of the room's aggregated group sensor. Written as a
	// trailing "with no_collect;" -- the same single-property "with <X>;" shorthand a bare "entity
	// <spec> with <X>;" declaration already uses (analyzeEntityDefinitionContext) -- rather than a
	// bare trailing word, so "no_collect" is never mistaken for a second capability token.
	NoCollect bool
}

var deviceCapabilityEntityPattern = regexp.MustCompile(`^entity\s+(\S+)\s+from\s+(\S+)\s+(\S+?)(?:\s+with\s+no_collect)?;$`)

// extractDeviceCapabilityEntityDeclaration recognises the "entity <spec> from <device-id>
// <capability> [with no_collect];" shape -- distinguished from extractDiscoveryEntityDeclaration's
// dot-joined "entity <spec> from <gateway-id>.<leaf>;" (exactly one token after "from") by the
// extra capability token.
func extractDeviceCapabilityEntityDeclaration(line string) (*TDeviceCapabilityEntityDeclaration, bool) {
	matches := deviceCapabilityEntityPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil, false
	}
	return &TDeviceCapabilityEntityDeclaration{
		LocalSpec:  matches[1],
		DeviceID:   matches[2],
		Capability: domainNamedCapability(matches[1], matches[3]),
		NoCollect:  strings.HasSuffix(line, "with no_collect;"),
	}, true
}

// domainNamedCapability expands "from DD" -- a bare capability equal to the entity's own domain --
// to the capability DD.DD (2026-10-25, Erik): "entity cover.social:front from cover;" keeps the
// entity's name what rules use, instead of a trailing "/cover" the name drops again. Any other bare
// capability stays bare, for the domain-qualification check to reject.
func domainNamedCapability(localSpec, capability string) string {
	domain, _, _ := strings.Cut(localSpec, ".")
	if capability == domain {
		return domain + "." + domain
	}
	return capability
}

// forDeviceShorthandPattern is the body-line shape inside a "device <spec> from <device-id>
// with: ... end;" block (Conceptual_DevicePositioning.go's deviceWithBlockHeaderPattern) -- sugar
// for a run of "entity <spec> from <device-id> <capability>;" lines that all repeat the same
// device-id. Inside the block, each line drops the repeated device-id ("entity <spec> from
// <capability>;"); parser.go's own main loop tracks "currently inside a with-block, for which
// device-id" and expands each such line back to deviceCapabilityEntityPattern's full shape before
// handing it to the ordinary dispatch chain -- so this is purely textual sugar, expanded at the
// point of parsing, with no new registration path of its own to drift from
// registerDeviceCapabilityEntityLink. (Formerly also the body shape of a standalone "for
// <device-id>: ... end;" block; that standalone form and its header recognizer were retired
// 2026-09-01 in favour of the merged construct -- this expansion function is the only part that
// survived, now reused by the merged construct's own body-line handling.)
//
// No collision risk against the discovery gateway's own dot-joined "entity <spec> from
// <gateway-id>.<leaf>;" (PROJECT.md item 7, 2026-09-09, dropping this shape's own former "entity"
// keyword before <capability>): a "device ... with:" block can only ever be opened for a
// hosts/imported/hassbridge target (registerDevicePositioning, Conceptual_DevicePositioning.go),
// never a "discovery" gateway one, and parser.go's main loop only ever attempts this expansion
// while inside such a block (tracked via forDeviceID) -- the discovery pattern is never even
// consulted for a line reached this way.
var forDeviceShorthandPattern = regexp.MustCompile(`^entity\s+(\S+)\s+from\s+(\S+?)(?:\s+with\s+no_collect)?;$`)

// expandForDeviceShorthandLine rewrites one "entity <spec> from <capability> [with no_collect];"
// line (found inside a "device <spec> from <deviceID> with: ... end;" block) back to the full
// "entity <spec> from <deviceID> <capability> [with no_collect];" shape
// deviceCapabilityEntityPattern expects.
func expandForDeviceShorthandLine(line, deviceID string) (string, bool) {
	matches := forDeviceShorthandPattern.FindStringSubmatch(line)
	if matches == nil {
		return "", false
	}
	suffix := ""
	if strings.HasSuffix(line, "with no_collect;") {
		suffix = " with no_collect"
	}
	return fmt.Sprintf("entity %s from %s %s%s;", matches[1], deviceID, matches[2], suffix), true
}

// forDeviceBareEntityPattern is a SECOND, even shorter body-line shape inside the same "device
// <spec> from <device-id> with: ... end;" block (2026-09-19) -- "entity <spec> [with no_collect];"
// with no "from <capability>" clause at all. Only a shorthand for the ALREADY-shorthand
// forDeviceShorthandPattern above, not a separate mechanism: expandForDeviceBareEntityLine
// auto-infers the capability as <spec>'s own explicit path (the text after the sphere's ":",
// deviceSpecLeafPath) and re-expands through the exact same full "entity <spec> from <device-id>
// <capability> [with no_collect];" shape -- so this can only ever fire when the desired local path
// and the device's own capability name are IDENTICAL text. A rename (local path differs from the
// capability, e.g. "main" positioned from capability "core") still requires the explicit
// "entity <spec> from <capability>;" form -- this pattern doesn't even try to match a line that
// already has a "from" clause, so there's no ambiguity between the two shapes.
var forDeviceBareEntityPattern = regexp.MustCompile(`^entity\s+(\S+?)(?:\s+with\s+no_collect)?;$`)

// expandForDeviceBareEntityLine rewrites one "entity <spec> [with no_collect];" line (no "from"
// clause) into the full "entity <spec> from <deviceID> <capability> [with no_collect];" shape,
// inferring <capability> as deviceSpecLeafPath(spec) -- see forDeviceBareEntityPattern's own doc
// comment. Returns false (no match) when spec has no explicit path at all (e.g. "switch.social:",
// deviceSpecLeafPath returns "") -- there is nothing to infer a capability from in that case, so
// the caller must keep using the explicit "from <capability>;" form.
//
// Only the LAST ":"-separated segment of the leaf path is used as the inferred capability
// (2026-09-19 fix, generalised 2026-09-21): deviceSpecLeafPath only ever strips up to the FIRST
// colon, so a multi-segment colon path (normalizeEntityFullName's general "every remaining colon
// becomes a '/' separator" sugar, expander.go) -- e.g. "sensor.social:terrace:pressure", meaning
// path "terrace/pressure" -- would otherwise infer a capability name still carrying the leftover
// colon-separated segment ("terrace:pressure"), which can never match a real Physical.def/
// Logical.def capability name (a bare label, never colon- or slash-containing). The reconstructed
// "entity <spec> from ...;" line still passes the ORIGINAL spec (with its colons intact) through
// unchanged, so the naming resolution itself is unaffected -- only the inferred capability name
// needed the fix. Real bugs found live: 2026-09-19 (Vienna's daylight entity), 2026-09-21
// (environment.weather's pressure capability).
func expandForDeviceBareEntityLine(line, deviceID string) (string, bool) {
	matches := forDeviceBareEntityPattern.FindStringSubmatch(line)
	if matches == nil {
		return "", false
	}
	spec := matches[1]
	leaf := deviceSpecLeafPath(spec)
	leafSegments := strings.Split(leaf, ":")
	capability := leafSegments[len(leafSegments)-1]
	if capability == "" {
		return "", false
	}
	suffix := ""
	if strings.HasSuffix(line, "with no_collect;") {
		suffix = " with no_collect"
	}
	return fmt.Sprintf("entity %s from %s %s%s;", spec, deviceID, capability, suffix), true
}

// bareCapabilityName strips an optional "<domain>." prefix from ref (e.g. "sensor.co2" -> "co2"),
// matching the bare-name convention both discoveryGatewaysByID's and hassBridgeDevicesByID's own
// Capabilities maps are keyed by (each map key is a device's own declared local label, never
// domain-prefixed -- see integration_discovery_parser.go / integration_hassbridge_parser.go).
func bareCapabilityName(ref string) string {
	if idx := strings.Index(ref, "."); idx >= 0 {
		return ref[idx+1:]
	}
	return ref
}

// resolveLongestSuffixCapability tries name as-is first (the ordinary, single-segment case, a no-op
// for every existing declaration), then progressively shorter "/"-delimited suffixes, returning the
// LONGEST one exists reports true for -- confirmed with the user 2026-09-25: "kk/ll/mm" must match a
// capability "mm" over one named "l/mm" (not a real suffix, "l" isn't a whole path segment of
// "kk/ll/mm"), and must prefer a capability "ll/mm" over a shorter "mm" when a device happens to
// declare both (see deviceHasCapability's own sibling validation note on why that dual-declaration
// itself should be rejected outright). Returns name unchanged if nothing at all matches, so the
// caller's existing "no such capability" warning still names the ORIGINAL text the author wrote.
func resolveLongestSuffixCapability(name string, exists func(string) bool) string {
	if exists(name) {
		return name
	}
	segments := strings.Split(name, "/")
	for i := 1; i < len(segments); i++ {
		candidate := strings.Join(segments[i:], "/")
		if exists(candidate) {
			return candidate
		}
	}
	return name
}

// deviceHasCapability reports whether deviceID -- whichever integration kind it actually turns out
// to be -- declares a capability named exactly capName. Used only by
// resolveLongestSuffixCapability's own exists callback (2026-09-25), which doesn't know or care
// which kind deviceID is; checking all of them is harmless since a device-id only ever belongs to
// one.
func deviceHasCapability(capName, deviceID string, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, hostDevicesByID map[string]THostDevice, commandlineDevicesByID map[string]TCommandlineDevice, localDevicesByID map[string]TLocalDevice, logicalDevicesByID map[string]TLogicalDevice) bool {
	if gateway, ok := discoveryGatewaysByID[deviceID]; ok {
		if _, ok := gateway.Capabilities[capName]; ok {
			return true
		}
	}
	if device, ok := hassBridgeDevicesByID[deviceID]; ok {
		if _, ok := device.Capabilities[capName]; ok {
			return true
		}
	}
	if device, ok := importedDevicesByID[deviceID]; ok {
		if _, ok := device.Capabilities[capName]; ok {
			return true
		}
	}
	if device, ok := hostDevicesByID[deviceID]; ok {
		if mat, known := MaterializationForIntegrationType(device.IntegrationType); known {
			// "node" is real bug found live 2026-09-29 (Junglinster's ring_chime_hallway/corridor,
			// migrating a bare "entity binary_sensor.infrastructural:ring_chime/node;" -- no "as"
			// leaf, no "from" clause, relying purely on resolveLongestSuffixCapability's own
			// progressive-stripping to find "node" as a suffix of "ring_chime/node"): a "hosts"
			// device's own "node" capability is NEVER a real map entry in mat.AttributeNames() (it's
			// synthesized on the fly from a ping check, exactly like registerHostCapabilityEntityLink's
			// own "if bareName == \"node\"" special case handles separately) -- so this function's own
			// hosts-kind branch always said "no" for "node", the ONE thing resolveLongestSuffixCapability
			// was stripping down TO, silently defeating the whole general mechanism for this one kind.
			// Every OTHER kind's own "node" is a genuine Physical.def-declared map entry already
			// covered by that kind's own generic Capabilities[capName] check below -- hosts is the only
			// kind needing this explicit exception, matching registerHostCapabilityEntityLink's own.
			if capName == "node" || slices.Contains(mat.AttributeNames(device), capName) {
				return true
			}
		}
	}
	if device, ok := commandlineDevicesByID[deviceID]; ok {
		// "node" mirrors the "hosts" branch's own exact exception above: a commandline device's
		// node is LWT-based, synthesized by the coordinator (house_event_bus_coordinator/
		// discoverycommandline.go's own unconditional "_commandline_node" entity), never a literal
		// Physical.def-declared Capabilities map entry (2026-10-12, PROJECT.md item 11c: giving
		// commandline devices a real, positionable "node" capability, replacing the Logical.def-only
		// "derived capability binary_sensor.node with: condition node.X; end;" indirection that used
		// to merely borrow another device's own node position instead of exposing this one).
		if capName == "node" {
			return true
		}
		if _, ok := device.Capabilities[capName]; ok {
			return true
		}
	}
	if device, ok := localDevicesByID[deviceID]; ok {
		if _, ok := device.Capabilities[capName]; ok {
			return true
		}
	}
	if device, ok := logicalDevicesByID[deviceID]; ok {
		if _, ok := device.Capabilities[capName]; ok {
			return true
		}
	}
	return false
}

// lookupCapabilityDomain reports capName's own REAL declared domain on deviceID (whichever
// integration kind it turns out to be), for the mandatory domain-qualification error message
// (registerDeviceCapabilityEntityLinkAllowingHiddenCore, 2026-09-27) -- turns "add a domain
// prefix" into an ACTIONABLE "write \"<real-domain>.<capability>\"" by mirroring
// deviceHasCapability's own kind-by-kind scan, returning the domain instead of a bare bool.
// "node" is special-cased for the "hosts" kind (mat.NodeDomain, not mat.AttributeDomain) --
// exactly the same split registerHostCapabilityEntityLink's own dispatch already makes.
func lookupCapabilityDomain(capName, deviceID string, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, hostDevicesByID map[string]THostDevice, commandlineDevicesByID map[string]TCommandlineDevice, localDevicesByID map[string]TLocalDevice, logicalDevicesByID map[string]TLogicalDevice) (string, bool) {
	if gateway, ok := discoveryGatewaysByID[deviceID]; ok {
		if cap, ok := gateway.Capabilities[capName]; ok {
			return cap.Domain, true
		}
	}
	if device, ok := hassBridgeDevicesByID[deviceID]; ok {
		if cap, ok := device.Capabilities[capName]; ok {
			return cap.Domain, true
		}
	}
	if device, ok := importedDevicesByID[deviceID]; ok {
		if cap, ok := device.Capabilities[capName]; ok {
			return cap.Domain, true
		}
	}
	if device, ok := hostDevicesByID[deviceID]; ok {
		if mat, known := MaterializationForIntegrationType(device.IntegrationType); known {
			if capName == "node" {
				return mat.NodeDomain, true
			}
			if slices.Contains(mat.AttributeNames(device), capName) {
				return mat.AttributeDomain, true
			}
		}
	}
	if device, ok := commandlineDevicesByID[deviceID]; ok {
		// "node" mirrors the "hosts" branch's own mat.NodeDomain special case above -- see
		// deviceHasCapability's identical commandline "node" case for the full rationale.
		if capName == "node" {
			return "binary_sensor", true
		}
		if cap, ok := device.Capabilities[capName]; ok {
			return cap.Kind, true
		}
	}
	if device, ok := localDevicesByID[deviceID]; ok {
		if cap, ok := device.Capabilities[capName]; ok {
			return cap.Domain, true
		}
	}
	if device, ok := logicalDevicesByID[deviceID]; ok {
		if cap, ok := device.Capabilities[capName]; ok {
			return cap.Domain, true
		}
	}
	return "", false
}

// registerDeviceCapabilityEntityLink dispatches decl.DeviceID to whichever integration kind it
// resolves against -- a "discovery" gateway device (delegates to registerDiscoveryEntityLink,
// Conceptual_DiscoveryEntities.go, entirely unchanged), a "home_assistant" bridge device (looks up
// the named capability's own Source, then delegates to registerDeviceSourceEntityLink,
// Conceptual_DeviceSourceEntities.go, entirely unchanged), or -- since 2026-09-01 -- a "hosts"
// device (delegates to registerHostCapabilityEntityLink, below; structurally different from the
// hassbridge/import branches since a hosts capability has no per-attribute Source to resolve, see
// that function's own doc comment).
//
// finalAttempt is threaded straight through to registerDeviceSourceEntityLink for the
// "home_assistant" branch -- see that function's own doc comment: the parser reads the file once,
// so a U1 line can arrive before its device's U2 positioning does; false lets that branch defer
// silently (deferred=true, no warning) rather than report a false "not positioned" failure, true
// is the one retry after the whole file has been read, where "still not positioned" is final. The
// discovery/hosts/unknown-device branches never depend on positioning, so they always return
// deferred=false regardless of finalAttempt.
//
// deviceNamePath is non-empty only when decl came from inside a "device <spec> from <device-id>
// with: ... end;" block's body (parser.go tracks this alongside forDeviceID) -- PROJECT.md item 5,
// see registerDeviceSourceEntityLink's doc comment for the full rationale. Threaded through to
// whichever branch below resolves decl.LocalSpec via a free-form spec (commandline, hassbridge,
// imported); the discovery/hosts branches ignore it, since neither derives its naming from
// decl.LocalSpec's own text (a discovery gateway's naming comes from the dot-joined gateway
// reference itself, and a "hosts" device's from its own positioning-time deviceIdentity, already
// fully resolved when it was positioned).
func registerDeviceCapabilityEntityLink(administration *TAdministrationState, decl TDeviceCapabilityEntityDeclaration, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, hostDevicesByID map[string]THostDevice, commandlineDevicesByID map[string]TCommandlineDevice, localDevicesByID map[string]TLocalDevice, logicalDevicesByID map[string]TLogicalDevice, entitiesPath string, lineNum int, finalAttempt bool, deviceNamePath string, capabilityIsInferred bool) (warnings []string, deferred bool) {
	return registerDeviceCapabilityEntityLinkAllowingHidden(administration, decl, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID, entitiesPath, lineNum, finalAttempt, deviceNamePath, false, capabilityIsInferred)
}

// registerDeviceCapabilityEntityLinkAllowingHidden is registerDeviceCapabilityEntityLink's real
// entry point, with one extra parameter: allowHidden. Every ordinary DSL-authored "entity ... from
// <device-id> <capability>;" line (parser.go's main dispatch loop and its deferred retry pass) goes
// through the public wrapper above with allowHidden always false -- a hidden discovery capability
// (integration_discovery_storage.go) may never be positioned directly by Spaces.def. The one
// exception is registerDevicePositioning's own auto-implied registration of a hidden capability's
// underlying entity (Conceptual_DevicePositioning.go) -- that registration is what MAKES the
// capability's value readable at all (a "derived" sibling's own reverse lookup needs a real
// DiscoveryEntityLinks entry to find), so it calls this allowing-hidden entry point directly.
//
// A thin wrapper (2026-09-27, the "switched by" migration) around
// registerDeviceCapabilityEntityLinkAllowingHiddenCore (the unchanged original body, every kind's
// own dispatch): after a successful, non-deferred registration, ALSO checks whether this specific
// (decl.DeviceID, capability-label) pair carries a Logical.def "is switched by" annotation
// (logical_switched_by.go's maybeRegisterSwitchedByRelation) -- reusing this single choke point
// (rather than each kind's own branch individually) because every kind funnels through here
// uniformly, discovery/local (the physical light-armature/relay case this migration is FOR)
// included, unlike administration.DeviceConceptualLinks (populated by some kinds, not others -- see
// that file's own header comment for why this dispatcher, not that map, is the only reliable
// universal hook). Applies identically regardless of which of the 3+ call sites reached here
// (including registerLogicalAbsorbedCapability's own recursive call targeting a DIFFERENT device
// id) -- each invocation only ever checks its OWN decl.DeviceID/label pair, so recursion is safe.
//
// bareName is resolved here, BEFORE Core runs (duplicating Core's own identical resolution,
// deterministic given the same inputs -- capabilityIsInferred's resolveLongestSuffixCapability
// never depends on anything Core itself mutates), so switchedBySuppressesEntity
// (logical_switched_by.go) can tell Core whether THIS positioning is the "is switched by"
// SelfEntity trigger -- if so, Core must skip generating its own raw entity for it entirely (a real
// bug found live 2026-09-28: without this, Core's raw discovery/local entity and the switched-by
// mechanism's own generated group both tried to claim the SAME entity_id, and the raw one won,
// leaving the intended switched-light entity never actually created under its real name).
func registerDeviceCapabilityEntityLinkAllowingHidden(administration *TAdministrationState, decl TDeviceCapabilityEntityDeclaration, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, hostDevicesByID map[string]THostDevice, commandlineDevicesByID map[string]TCommandlineDevice, localDevicesByID map[string]TLocalDevice, logicalDevicesByID map[string]TLogicalDevice, entitiesPath string, lineNum int, finalAttempt bool, deviceNamePath string, allowHidden bool, capabilityIsInferred bool) (warnings []string, deferred bool) {
	bareName := bareCapabilityName(decl.Capability)
	if capabilityIsInferred {
		bareName = resolveLongestSuffixCapability(bareName, func(candidate string) bool {
			return deviceHasCapability(candidate, decl.DeviceID, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID)
		})
	}
	suppressEntity := switchedBySuppressesEntity(administration, decl, bareName, deviceNamePath, logicalDevicesByID)

	warnings, deferred = registerDeviceCapabilityEntityLinkAllowingHiddenCore(administration, decl, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID, entitiesPath, lineNum, finalAttempt, deviceNamePath, allowHidden, capabilityIsInferred, suppressEntity)
	if deferred || len(warnings) > 0 {
		return warnings, deferred
	}

	// Record the device's own "node" positioning uniformly, regardless of which of Core's several
	// branches actually handled it (see TAdministrationState.DeviceNodeEntityID's own doc comment
	// for why this single choke point, not each branch's own storage shape, is the only reliable
	// place to do this). fullName is recomputed the exact same way every one of those branches
	// already computes it internally (resolveDeviceEntityFullName(decl.LocalSpec,
	// administration.SpacePath, deviceNamePath) -> toHomeAssistantEntityID), so this always agrees
	// with whatever entity_id Core itself just registered.
	if bareName == "node" {
		if entityID := toHomeAssistantEntityID(resolveDeviceEntityFullName(decl.LocalSpec, administration.SpacePath, deviceNamePath)); entityID != "" {
			administration.DeviceNodeEntityID[decl.DeviceID] = entityID
		}
	}

	extraWarnings, extraDeferred := maybeRegisterSwitchedByRelation(administration, decl, bareName, deviceNamePath, logicalDevicesByID, discoveryGatewaysByID, hassBridgeDevicesByID, hostDevicesByID, localDevicesByID, finalAttempt, entitiesPath, lineNum)
	return append(warnings, extraWarnings...), extraDeferred
}

// registerDeviceCapabilityEntityLinkAllowingHiddenCore is the original dispatcher body, structurally
// unchanged (see registerDeviceCapabilityEntityLinkAllowingHidden's own doc comment above for why
// it's now wrapped rather than called directly) except for one new parameter, suppressEntityGeneration
// -- when true, the discovery and local branches below skip their own final entity-generating call
// (the capability's real entity_id is about to come from the "is switched by" group instead) while
// still running every validation step first (domain-qualification, "does this device actually
// declare this capability" -- an author error there must still surface as a warning regardless of
// suppression).
func registerDeviceCapabilityEntityLinkAllowingHiddenCore(administration *TAdministrationState, decl TDeviceCapabilityEntityDeclaration, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, hostDevicesByID map[string]THostDevice, commandlineDevicesByID map[string]TCommandlineDevice, localDevicesByID map[string]TLocalDevice, logicalDevicesByID map[string]TLogicalDevice, entitiesPath string, lineNum int, finalAttempt bool, deviceNamePath string, allowHidden bool, capabilityIsInferred bool, suppressEntityGeneration bool) (warnings []string, deferred bool) {
	provenance := fmt.Sprintf("%s:%d → %s from %s %s", filepath.Base(entitiesPath), lineNum, decl.LocalSpec, decl.DeviceID, decl.Capability)
	bareName := bareCapabilityName(decl.Capability)

	// Mandatory domain-qualification (2026-09-27, the user's own "Note 3": "always include the
	// domain when referring to a capability in a 'CC from YY' clause" -- generalized from a
	// preference into a hard rule, since a bare, domain-less capability reference (e.g. "core"
	// instead of "light.core") silently hid which domain the author actually meant, right up until
	// this rule existed to catch it. Scoped to an EXPLICIT "entity X from Y Z;" reference only
	// (capabilityIsInferred's own no-rename shorthand has no separate capability token to qualify
	// at all -- Note 3 was about "CC from YY" clauses specifically, not that different shape).
	if !capabilityIsInferred && !strings.Contains(decl.Capability, ".") {
		if realDomain, found := lookupCapabilityDomain(decl.Capability, decl.DeviceID, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID); found {
			return []string{fmt.Sprintf("%s: capability reference %q has no domain prefix -- write %q", provenance, decl.Capability, realDomain+"."+decl.Capability)}, false
		}
		return []string{fmt.Sprintf("%s: capability reference %q has no domain prefix -- write the domain explicitly (e.g. %q), matching whatever domain %s's own Physical.def declaration actually gives it", provenance, decl.Capability, "<domain>."+decl.Capability, decl.DeviceID)}, false
	}

	// The no-rename shorthand's own inferred capability name is the entity's WHOLE compound leaf
	// path (e.g. "aqara/node" from "entity binary_sensor.infrastructural:aqara/node;", no "from" at
	// all) -- confirmed with the user 2026-09-25: this should match the LONGEST available
	// "/"-delimited SUFFIX of that path against the device's own real capabilities ("node" here),
	// not require the device to declare a capability under the exact compound text. Scoped to
	// capabilityIsInferred ONLY (never for an explicit "entity X from Y;", even when Y itself
	// contains a "/", e.g. a hosts device's own "cpu/load") -- a real, deliberately-written
	// capability name that's simply wrong must still fail outright, never silently fall back to a
	// shorter guess.
	if capabilityIsInferred {
		bareName = resolveLongestSuffixCapability(bareName, func(candidate string) bool {
			return deviceHasCapability(candidate, decl.DeviceID, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID)
		})
	}

	// "node" capability lending/override (2026-09-24, PROJECT.md item 11): a Logical.def device
	// sharing this SAME device id can claim "node" for itself even when the device also has real
	// physical presence -- e.g. appliance.front_door_ring's own combined-availability override,
	// replacing its raw "local"-kind "node" (Physical.def's own "binary_sensor.node camera.core is
	// available;") with a "derived binary_sensor.node with: condition ...; end;" one that also ANDs
	// in a ping check. Checked here, before any kind-specific branch below, because "node" is no
	// longer auto-registered at positioning time (every kind's own node is now an ordinary explicit
	// "entity binary_sensor.X:node;" reference like any other capability) -- this is now the ONLY
	// place the override-vs-physical choice needs to be made, so it has to run on every dispatch of
	// "node" specifically, including retries (this same check re-runs then too, since the ordinary
	// deferred-retry path re-invokes this whole function -- no separate "via override" retry flag
	// needed any more, unlike the pre-2026-09-24 positioning-triggered version of this check).
	//
	// Dispatches DIRECTLY via dispatchLogicalCapability rather than falling through to a kind-specific
	// branch below: those branches resolve via each kind's own PHYSICAL Capabilities map first, which
	// -- for the entire override use case, where the physical capability genuinely still exists --
	// would always win on capFound alone and the override would never be reached (the exact bug this
	// check's own predecessor, registerLogicalNodeOverride, was built to fix on 2026-09-24).
	if bareName == "node" {
		if logicalDevice, hasLogical := logicalDevicesByID[decl.DeviceID]; hasLogical {
			if _, hasOverride := logicalDevice.Capabilities["node"]; hasOverride {
				return dispatchLogicalCapability(administration, decl, logicalDevice, "node", deviceNamePath, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID, entitiesPath, lineNum, finalAttempt, provenance)
			}
		}
	}

	// Checked first, but only actually claims the declaration when this SPECIFIC capability name is
	// one of this device's own commandline capabilities -- a commandline device commonly shares its
	// DeviceID with a "hosts"-kind declaration of the same physical machine (e.g. host.frame is both
	// a "hosts" cpu device and a "commandline" device, deliberately sharing one HA device identity,
	// discoverycommandline.go's own doc comment), so falling through to the other kinds below when
	// the capability name isn't recognised here (rather than erroring immediately, unlike every
	// other branch) is required for that dual-kind case to keep working for its own non-commandline
	// capabilities (e.g. "load"/"temperature").
	if device, found := commandlineDevicesByID[decl.DeviceID]; found {
		// "node" (2026-10-12, PROJECT.md item 11c): synthetic, never a literal Capabilities map
		// entry -- see deviceHasCapability's commandline "node" case for why (mirrors the "hosts"
		// branch's own identical treatment of its ping-based node, just above in this same
		// dispatcher). Kind is hardcoded "binary_sensor" since that's the only domain the
		// coordinator's own auto-published "_commandline_node" entity (discoverycommandline.go) is
		// ever published as.
		//
		// Skipped when this SAME DeviceID is ALSO a "hosts"-kind device
		// (TestCommandlineCapabilityEntityLinkResolvesOnDualKindDevice's own real precedent, e.g.
		// host.frame sharing one id across both kinds): the hosts branch below already owns "node"
		// for that id (its ping check, a semantically different and longer-standing liveness
		// signal than this device's own LWT) -- falling through here lets it keep winning, exactly
		// as it did before this synthetic case existed.
		if bareName == "node" {
			if _, alsoHost := hostDevicesByID[decl.DeviceID]; !alsoHost {
				return registerCommandlineCapabilityEntityLink(administration, decl.LocalSpec, decl.DeviceID, "node", TCommandlineCapability{Kind: "binary_sensor"}, provenance, finalAttempt, deviceNamePath)
			}
		} else if capability, capFound := device.Capabilities[bareName]; capFound {
			return registerCommandlineCapabilityEntityLink(administration, decl.LocalSpec, decl.DeviceID, bareName, capability, provenance, finalAttempt, deviceNamePath)
		}
	}

	if gateway, found := discoveryGatewaysByID[decl.DeviceID]; found {
		discoveryDecl := TDiscoveryEntityDeclaration{
			EntitySpec:      decl.LocalSpec,
			GatewayDeviceID: decl.DeviceID,
			Leaf:            bareName,
			NoCollect:       decl.NoCollect,
		}
		// Fall through to this SAME device id's Logical.def overlay before erroring -- mirrors the
		// hassbridge/import branches' own identical fallback (2026-09-18: a discovery-kind device
		// id, e.g. sensors.terrace_motion, can now ALSO carry real Logical.def capabilities --
		// first real case: "sunny_threshold"/"sunny" alongside the plain discovery-relayed
		// illuminance/temperature/motion/battery_level capabilities).
		if _, capFound := gateway.Capabilities[bareName]; !capFound {
			if logicalDevice, logicalFound := logicalDevicesByID[decl.DeviceID]; logicalFound {
				if _, logicalCapFound := logicalDevice.Capabilities[bareName]; logicalCapFound {
					return dispatchLogicalCapability(administration, decl, logicalDevice, bareName, deviceNamePath, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID, entitiesPath, lineNum, finalAttempt, provenance)
				}
			}
		}
		// A capability whose own source is "... is available;" (capability.AvailabilityOf set)
		// tracks a SIBLING capability's availability rather than a raw gateway leaf -- needs
		// registerDiscoveryAvailabilityEntityLink's own deferred-capable resolution (the sibling
		// may not be positioned yet), not registerDiscoveryEntityLink's raw-leaf path. Unknown
		// capability names fall through to registerDiscoveryEntityLink unchanged, which already
		// reports the "declares no ... capability" warning itself.
		if capability, capFound := gateway.Capabilities[bareName]; capFound && capability.AvailabilityOf != "" {
			return registerDiscoveryAvailabilityEntityLink(administration, discoveryDecl, capability, discoveryGatewaysByID, deviceNamePath, entitiesPath, lineNum, finalAttempt)
		}
		// A "derived DDD.NNN from EEE.MMM via TTT;" capability (DerivedFromCapability set) computes
		// its own value from a SIBLING capability rather than a raw gateway leaf. Two shapes:
		// (1) the sibling is a "hidden" plain raw leaf of the SAME gateway -- relay it directly
		// under this capability's own entity, TTT folded into the MQTT value_template
		// (registerDiscoveryDerivedFromRawLeafEntityLink), no deferral possible or needed.
		// (2) anything else (a real, independently-positioned/materialized capability, or the
		// sibling is itself AvailabilityOf/Derived) -- the sibling's own resolved HA entity_id has
		// to be read via states(...), needing registerDiscoveryDerivedEntityLink's own
		// deferred-capable reverse lookup instead.
		if capability, capFound := gateway.Capabilities[bareName]; capFound && capability.DerivedFromCapability != "" {
			siblingCapability, siblingFound := gateway.Capabilities[capability.DerivedFromCapability]
			if siblingFound && siblingCapability.Hidden && siblingCapability.Leaf != "" && siblingCapability.AvailabilityOf == "" && siblingCapability.DerivedFromCapability == "" {
				return registerDiscoveryDerivedFromRawLeafEntityLink(administration, discoveryDecl, capability, siblingCapability, deviceNamePath, entitiesPath, lineNum), false
			}
			return registerDiscoveryDerivedEntityLink(administration, discoveryDecl, capability, discoveryGatewaysByID, deviceNamePath, entitiesPath, lineNum, finalAttempt)
		}
		if suppressEntityGeneration {
			return nil, false
		}
		return registerDiscoveryEntityLink(administration, discoveryDecl, discoveryGatewaysByID, entitiesPath, lineNum, deviceNamePath, allowHidden), false
	}

	if device, found := hassBridgeDevicesByID[decl.DeviceID]; found {
		capability, capFound := device.Capabilities[bareName]
		if !capFound {
			// Fall through to this SAME device id's Logical.def overlay before erroring -- since
			// the "absorb" operation (2026-09-18) a hassbridge device id can ALSO carry its own
			// Logical.def entry (e.g. appliance.washing_machine's own absorbed "power"/"energy"/
			// "consumes"/"switch.core" from a separate ROBB plug's discovery gateway, plus a
			// wrapping enabler-aware "available" capability referencing this SAME device's own
			// already-positioned raw node) -- dispatchLogicalCapability (below) is the exact same
			// switch the pure-logical branch uses. The imported-device and discovery-gateway
			// branches below have the identical fallback (sensors.vienna_terrace_wind's own
			// "windy_threshold"/"windy", sensors.terrace_motion's own "sunny_threshold"/"sunny",
			// both 2026-09-18); hosts doesn't need it yet (no real case), matching commandline's
			// own pre-existing fall-through above for its own dual-kind case.
			if logicalDevice, logicalFound := logicalDevicesByID[decl.DeviceID]; logicalFound {
				if _, logicalCapFound := logicalDevice.Capabilities[bareName]; logicalCapFound {
					return dispatchLogicalCapability(administration, decl, logicalDevice, bareName, deviceNamePath, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID, entitiesPath, lineNum, finalAttempt, provenance)
				}
			}
			return []string{fmt.Sprintf("%s: device %q declares no %q capability; add a \"<type>.%s: <source>;\" line to its Physical.def declaration", provenance, decl.DeviceID, bareName, bareName)}, false
		}
		warnings, deferred := registerDeviceSourceEntityLink(administration, TDeviceSourceEntityDeclaration{
			LocalSpec: decl.LocalSpec,
			// capabilityKey (bareName) is non-empty below, so registerDeviceSourceEntityLink
			// preserves capability's own already-per-instance Sources map unchanged -- this Source
			// value is display-only (the provenance string on a rare error path).
			Source:   representativeSource(capability.Sources),
			DeviceID: decl.DeviceID,
		}, hassBridgeDevicesByID, importedDevicesByID, entitiesPath, lineNum, finalAttempt, bareName, deviceNamePath)
		// A capability with a slow-speed shadow ("slow speed ...;", 2026-10-23) brings it along:
		// "<domain>.<sphere>:<path>" implies "<domain>.<sphere>:slow/<path>", no line of its own.
		slowLabel := slowSpeedCapabilityLabel(bareName)
		if slow, hasSlow := device.Capabilities[slowLabel]; hasSlow && !deferred && len(warnings) == 0 {
			warnings, deferred = registerDeviceSourceEntityLink(administration, TDeviceSourceEntityDeclaration{
				LocalSpec: slowSpeedLocalSpec(decl.LocalSpec),
				Source:    representativeSource(slow.Sources),
				DeviceID:  decl.DeviceID,
			}, hassBridgeDevicesByID, importedDevicesByID, entitiesPath, lineNum, finalAttempt, slowLabel, deviceNamePath)
			if !deferred && len(warnings) == 0 {
				attributes := administration.DeviceConceptualLinks[decl.DeviceID].AttributeEntityIDs
				administration.SlowSpeedShadows[attributes[bareName].EntityID] = attributes[slowLabel].EntityID
			}
		}
		return warnings, deferred
	}

	if importedDevice, found := importedDevicesByID[decl.DeviceID]; found {
		if _, capFound := importedDevice.Capabilities[bareName]; !capFound {
			// Fall through to this SAME device id's Logical.def overlay before erroring -- mirrors
			// the hassbridge branch's own identical fallback above (2026-09-18: an imported device
			// id, e.g. sensors.vienna_terrace_wind, can now ALSO carry real Logical.def capabilities
			// -- first real case: "windy_threshold"/"windy" alongside the plain imported radio/
			// battery_level/wind_direction/wind_speed capabilities).
			if logicalDevice, logicalFound := logicalDevicesByID[decl.DeviceID]; logicalFound {
				if _, logicalCapFound := logicalDevice.Capabilities[bareName]; logicalCapFound {
					return dispatchLogicalCapability(administration, decl, logicalDevice, bareName, deviceNamePath, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID, entitiesPath, lineNum, finalAttempt, provenance)
				}
			}
			return []string{fmt.Sprintf("%s: imported device %q declares no %q capability; add a \"%s: <remote-local-entity>;\" line to its Physical.def import declaration", provenance, decl.DeviceID, bareName, bareName)}, false
		}
		return registerDeviceSourceEntityLink(administration, TDeviceSourceEntityDeclaration{
			LocalSpec: decl.LocalSpec,
			DeviceID:  decl.DeviceID,
		}, hassBridgeDevicesByID, importedDevicesByID, entitiesPath, lineNum, finalAttempt, bareName, deviceNamePath)
	}

	if device, found := hostDevicesByID[decl.DeviceID]; found {
		return registerHostCapabilityEntityLink(administration, device, decl.DeviceID, bareName, decl.LocalSpec, deviceNamePath, provenance, finalAttempt)
	}

	if device, found := localDevicesByID[decl.DeviceID]; found {
		if capability, capFound := device.Capabilities[bareName]; capFound {
			if suppressEntityGeneration {
				return nil, false
			}
			_, hasNodeCapability := device.Capabilities["node"]
			return registerLocalCapabilityEntityLink(administration, decl, capability, bareName, deviceNamePath, entitiesPath, lineNum, hasNodeCapability, finalAttempt)
		}
		// Fall through to this SAME device id's Logical.def overlay before erroring -- mirrors the
		// hassbridge/discovery/imported branches' own identical fallback (a "local" device's own id
		// can ALSO carry a real Logical.def capability with no physical counterpart at all -- the
		// Ring doorbell's own "battery_alert", a pure Logical.def derivation, alongside its plain
		// "local" camera/ding/motion/volume/battery_level/node capabilities).
		if logicalDevice, logicalFound := logicalDevicesByID[decl.DeviceID]; logicalFound {
			if _, logicalCapFound := logicalDevice.Capabilities[bareName]; logicalCapFound {
				return dispatchLogicalCapability(administration, decl, logicalDevice, bareName, deviceNamePath, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID, entitiesPath, lineNum, finalAttempt, provenance)
			}
		}
		return []string{fmt.Sprintf("%s: local device %q declares no %q capability; add a \"<type>.%s;\" line to its Physical.def declaration", provenance, decl.DeviceID, bareName, bareName)}, false
	}

	if device, found := logicalDevicesByID[decl.DeviceID]; found {
		return dispatchLogicalCapability(administration, decl, device, bareName, deviceNamePath, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID, entitiesPath, lineNum, finalAttempt, provenance)
	}

	return []string{fmt.Sprintf("%s: device %q not found in Physical.def's \"discovery\" or \"home_assistant\" integrations, Logical.def, or as a \"hassbridge\"-form import", provenance, decl.DeviceID)}, false
}

// dispatchLogicalCapability is the Logical.def device's own capability-shape switch, shared by two
// callers: registerDeviceCapabilityEntityLinkAllowingHidden's own "logicalDevicesByID" branch above
// (a purely logical device id, e.g. appliance.tv), and the hassbridge branch's fallback just above
// (a hassbridge device id that ALSO carries a Logical.def entry -- the "absorb" operation,
// 2026-09-18). Factored out rather than duplicated so both entry points stay byte-identical in
// behaviour as new capability shapes get added here.
func dispatchLogicalCapability(administration *TAdministrationState, decl TDeviceCapabilityEntityDeclaration, device TLogicalDevice, bareName, deviceNamePath string, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, hostDevicesByID map[string]THostDevice, commandlineDevicesByID map[string]TCommandlineDevice, localDevicesByID map[string]TLocalDevice, logicalDevicesByID map[string]TLogicalDevice, entitiesPath string, lineNum int, finalAttempt bool, provenance string) (warnings []string, deferred bool) {
	capability, capFound := device.Capabilities[bareName]
	if !capFound {
		return []string{fmt.Sprintf("%s: logical device %q declares no %q capability; add a \"<type>.%s: <value>;\" line to its Logical.def declaration", provenance, decl.DeviceID, bareName, bareName)}, false
	}
	switch {
	case capability.AbsorbedFromDeviceID != "":
		return registerLogicalAbsorbedCapability(administration, decl, capability, bareName, deviceNamePath, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID, entitiesPath, lineNum, finalAttempt)
	case capability.IsDefinedInputNumber:
		return registerLogicalDefinedInputNumberCapability(administration, decl, capability, bareName, deviceNamePath, entitiesPath, lineNum), false
	case capability.IsDerivedCondition, capability.IsDerivedValue:
		return registerLogicalDerivedConditionCapability(administration, decl, capability, bareName, deviceNamePath, entitiesPath, lineNum, finalAttempt, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, localDevicesByID, logicalDevicesByID)
	default:
		return []string{fmt.Sprintf("%s: logical device %q's %q capability isn't a supported shape yet (only absorbed capabilities, defined input_numbers, and derived conditions are built)", provenance, decl.DeviceID, bareName)}, false
	}
}

// registerHostCapabilityEntityLink is registerDeviceCapabilityEntityLink's "hosts"-kind branch
// (PROJECT.md, unification plan 2026-09-01) -- added so `entity <spec> from <device-id> entity
// <capability>;` (and its `for <device-id>: ... end;` shorthand) work for "hosts" devices exactly
// as they already do for "home_assistant" bridge/import devices, letting a device that doesn't
// actually have every attribute its integration type hardwires (e.g. host.mqtt, a cloud VM with
// no real "temperature" sensor) name only the ones it genuinely has, instead of the "with: all
// entities;" bulk form pulling in the full hardwired set.
//
// Structurally different from the hassbridge/import branches above: a hosts capability has no
// per-attribute `Source` string to resolve (integration_hosts_storage.go's
// MaterializationForIntegrationType hardwires domain/naming from the device's own position plus
// its integration type, never a free-form reference), so this reuses registerHostAttributeEntity
// (Conceptual_DeviceEntities.go) -- the same function "with: all entities;"/its own single-flag
// form already call -- rather than registerDeviceSourceEntityLink. "node" (2026-09-24: no longer
// auto-registered at positioning time, deliberately, even though a hosts device's own liveness is
// unconditional -- confirmed with the user as a tradeoff, so the mechanism doesn't have to know
// which specific integration a device happens to be) is handled by its own branch below, resolving
// via localSpec/deviceNamePath through the ordinary naming machinery exactly like every other
// kind's own explicit "node" reference already does -- see registerHostNodeAttribute's own doc
// comment (Conceptual_DeviceEntities.go).
//
// finalAttempt/deferred follow registerDeviceSourceEntityLink's own established convention: the
// parser reads the file once, so a capability-link line can arrive before its device's positioning
// does; deferred=true with no warning lets the single retry (after the whole file is read) resolve
// it once positioning has actually landed, wherever in the file it appears.
func registerHostCapabilityEntityLink(administration *TAdministrationState, device THostDevice, deviceID, bareName, localSpec, deviceNamePath, provenance string, finalAttempt bool) (warnings []string, deferred bool) {
	mat, known := MaterializationForIntegrationType(device.IntegrationType)
	if !known {
		return []string{fmt.Sprintf("%s: integration type %q has no known entity materialization; skipping", provenance, device.IntegrationType)}, false
	}

	if bareName == "node" {
		// Real gap found live 2026-09-29 (Junglinster's ring_chime_hallway, absorbing
		// node.ring_chime_hallway's own "node" -- a hosts/ping device NEVER otherwise directly
		// referenced anywhere in Conceptual.def, relying entirely on Logical.def's own
		// "condition node.ring_chime_hallway;" bare-device-id sugar): this used to require
		// administration.DeviceConceptualLinks[deviceID] to ALREADY exist -- i.e. deviceID must
		// have at least one OTHER direct Conceptual.def reference somewhere, deferring/erroring
		// otherwise. That requirement was never actually load-bearing: Go's own zero-value for a
		// missing map entry is already a perfectly usable empty TDeviceConceptualLink, and
		// autoMaterializePhysicalCapability's own hosts-kind "node" branch (Conceptual_LogicalEntities.go)
		// already proves creating a hosts device's own node entity with NO pre-existing link is
		// safe -- it's exactly how "condition node.X;" resolves one today. Dropped the
		// deferred-retry/error branch entirely: proceeding with the zero-value link (Go's own
		// `administration.DeviceConceptualLinks[deviceID]`, present or not) now works uniformly
		// for both an ordinary direct "entity X from Y node;" reference AND an absorb block's own
		// recursive delegate call, matching the user's own explicit confirmation that a device
		// referenced ONLY indirectly (never positioned on its own) should still resolve.
		link := administration.DeviceConceptualLinks[deviceID]
		fullName := resolveDeviceEntityFullName(localSpec, administration.SpacePath, deviceNamePath)
		if identity := extractEntityIdentity(fullName); identity.Domain == "" {
			return []string{fmt.Sprintf("%s: could not resolve a domain from %q; skipping", provenance, localSpec)}, false
		}
		spaceName := administration.CurrentSpaceName()
		link.NodeEntityID, link.NodeDeviceClass, link.NodeIcon = registerHostNodeAttribute(administration, mat, device, fullName, spaceName, provenance)
		administration.DeviceConceptualLinks[deviceID] = link
		return nil, false
	}

	if !slices.Contains(mat.AttributeNames(device), bareName) {
		return []string{fmt.Sprintf("%s: device %q (integration type %q) has no %q attribute", provenance, deviceID, device.IntegrationType, bareName)}, false
	}

	link, hasLink := administration.DeviceConceptualLinks[deviceID]
	if !hasLink {
		if !finalAttempt {
			return nil, true
		}
		return []string{fmt.Sprintf("%s: device %q has no \"device %s [as ...] with: ...;\" positioning yet -- add one (any space) before referencing one of its entities directly", provenance, deviceID, deviceID)}, true
	}

	// Resolved the exact same way the "node" branch above already does -- fold localSpec through
	// the general, "as"-independent mechanism every other entity kind uses, rather than through a
	// device-identity reconstruction keyed off the device header's own raw "as <leaf>" text. This
	// also sidesteps a real, now-moot bug (found live 2026-10-04, migrating server.xanadu/
	// server.backups): the OLD version trusted link.HostIdentity, correct ONLY for a device's own
	// DIRECT positioning, and had to specially reconstruct identity from deviceNamePath/SpacePath
	// for an "absorb" block's recursive delegate call (deviceID being the ABSORBED SOURCE device,
	// which may never be directly positioned at all) to avoid producing a wrong, shadow-named
	// entity. localSpec/deviceNamePath are already the correct, contextually-resolved values for
	// WHICHEVER device's block this entity line actually lives under (absorbing or direct) --
	// confirmed by the "node" branch's own unconditional use of them succeeding in that exact same
	// absorb scenario, with no special-casing of its own at all.
	fullName := resolveDeviceEntityFullName(localSpec, administration.SpacePath, deviceNamePath)
	if identity := extractEntityIdentity(fullName); identity.Domain == "" {
		return []string{fmt.Sprintf("%s: could not resolve a domain from %q; skipping", provenance, localSpec)}, false
	}

	spaceName := administration.CurrentSpaceName()
	registerHostAttributeEntity(administration, mat, device, fullName, spaceName, provenance, link, bareName)
	administration.DeviceConceptualLinks[deviceID] = link
	return nil, false
}

// slowSpeedLocalSpec is the positioning implied for a capability's slow-speed shadow:
// "cover.social:front" -> "cover.social:slow/front", "cover.social:" -> "cover.social:slow".
func slowSpeedLocalSpec(spec string) string {
	colon := strings.Index(spec, ":")
	if colon < 0 {
		return spec
	}
	if path := spec[colon+1:]; path != "" {
		return spec[:colon+1] + "slow/" + path
	}
	return spec + "slow"
}
