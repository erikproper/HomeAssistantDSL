/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Administration
 *
 * This component stores administrative runtime state collected from definition files, including global variable bindings.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 26.03.2026
 *
 */

package main

import (
	"fmt"
	"os"
	"strings"
)

type TPendingEntityCollection struct {
	SpaceName      string
	Entry          string
	ExternalEntry  string
	Record         TEntityRecord
	ExpectedDepth  int
	HasExternalRef bool
}

type TAdministrationState struct {
	SpacePath                []string
	OpenBlocks               []string
	PendingEntityCollections []TPendingEntityCollection

	SpaceOrder              []string
	SpaceKindByName         map[string]string
	EntitiesBySpace         map[string][]string
	EntityRecordsBySpace    map[string][]TEntityRecord
	EntityRecordSeenBySpace map[string]map[string]string // spaceName → entityKey → provenance of first registration
	ExternalEntitiesBySpace map[string][]string
	SpaceDepthByName        map[string]int
	SpaceOffByName          map[string][]string // Entities/aggregations to turn off
	SpaceOnByName           map[string][]string // Full accumulated lights-on list (explicit + propagated from children)
	SpaceOnExplicitByName   map[string][]string // Lights named in an explicit "light on:" directive only

	// Domain-specific collections, auto-populated on entity registration (no_collect excluded).
	// Used for expanding @light / @media / @all tokens and for default on/off derivation.
	SpaceLightsByName map[string][]string // light-domain entities per space
	SpaceMediaByName  map[string][]string // media_player-domain entities per space

	SpaceLightOffByName map[string][]string // explicit "light off:" list per space (used by generator)

	HeatingLeaksByName map[string][]string // entities listed in "heating leak:" directives per space

	// Follows/switched_device/timer-limits relations, populated by ParseEntitiesAndFillAdministration.
	FollowsRelations        []TFollowsRelation
	SwitchedDeviceRelations []TSwitchedDeviceRelation
	TimerLimitsRelations    []TTimerLimitsRelation

	TrustedProxies []string // from "http proxies ...;" in Settings.def

	// Maps DSL node entity name → representative HA entity ID for top-level providing calls.
	// Populated by ParseEntitiesAndFillAdministration; used by the template binary sensor generator.
	NodeRepresentativeByEntityID map[string]string

	// REST import records, one per "imported rest" directive, in source order.
	// Populated by ParseEntitiesAndFillAdministration; used by the REST sensor/binary_sensor generator.
	RestImports []TRestImportRecord

	// CLI sensor/switch records, populated by ParseEntitiesAndFillAdministration.
	CliSensors []TCliSensorRecord
	CliSwitches []TCliSwitchRecord

	// Bridge targets (name → resolved URL+token), populated by generateHouseYAML.
	BridgeTargets map[string]THomeAssistantTarget

	// Track whether an explicit light on: / space off: directive was given for a space.
	// When false, defaults are derived from the domain collections at space-close time.
	SpaceHasExplicitOn  map[string]bool
	SpaceHasExplicitOff map[string]bool

	// Per-space no-motion light-off delay in minutes, set by "lights_motion_guarded with delay N".
	// Defaults to 15 when absent.
	SpaceNoMotionDelayByName map[string]int

	// Per-virtual-space member space list, populated by "member <spaceName>;" directives.
	// Used to expand @light / @media / @all tokens in virtual-space contexts.
	SpaceMembersByName map[string][]string

	// Per-space explicit switch turn-on items, set by "space on: <items>;" directives.
	// When set, overrides the default lights-on list in the template switch turn_on block.
	SpaceSwitchOnByName map[string][]string
}

// TFollowsRelation records a "follows <follower> <leader>;" space-level directive.
type TFollowsRelation struct {
	SpaceName string
	Follower  string // fully qualified DSL entity name, e.g. "light.social/house/corridor/corb"
	Leader    string // fully qualified DSL entity name, e.g. "light.social/house/corridor/main"
}

// TSwitchedDeviceRelation records a "definition as switched_device <device> <main>" entity directive.
type TSwitchedDeviceRelation struct {
	SpaceName  string
	Device     string // fully qualified DSL entity name (first arg), e.g. "light.physical/house/bathroom/main_disc"
	MainEntity string // fully qualified DSL entity name (second arg), e.g. "light.physical/house/bathroom/main"
}

// TTimerLimitsRelation records a "limits <timer> <entity>: off on;" space-level directive.
type TTimerLimitsRelation struct {
	SpaceName    string
	TimerEntity  string // fully qualified DSL entity name, e.g. "timer.social/house/wc/removing_smell"
	BoundEntity  string // fully qualified DSL entity name, e.g. "fan.social/house/wc"
}

// TRestImportRecord holds the parameters of a single "imported rest" directive.
type TRestImportRecord struct {
	LocalEntityName string // fully qualified local DSL entity name
	BridgeName      string // bridge name (e.g. "junglinster")
	RemoteEntityID  string // remote HA entity ID (e.g. "sensor.physical_vienna_living_room_co2")
	ScanInterval    int    // scan interval in seconds
	ValueExpr       string // optional value expression with "$" as state placeholder (e.g. "$ == 'True'")
}

// TCliSensorRecord holds parameters of a single "cli_sensor <alias> <fqdn> <script>" directive.
type TCliSensorRecord struct {
	LocalEntityName string
	UserAlias       string
	HostFQDN        string
	ScriptPath      string
}

// TCliSwitchRecord holds parameters of a single "cli_switch <alias> <fqdn> <on> <off> <state>" directive.
type TCliSwitchRecord struct {
	LocalEntityName string
	UserAlias       string
	HostFQDN        string
	OnScript        string
	OffScript       string
	StateScript     string
}

type TEntityIdentity struct {
	Domain  string
	IsRaw   bool
	RawName string
	Sphere  string
	Path    string
}

type TEntityRecord struct {
	Name                  string
	Identity              TEntityIdentity
	NoCollect             bool
	HasDefinitionOrImport bool
	OpenStopClose         bool
	Provenance            string // call chain that produced this record, e.g. "Entities.def:65 → battery_alert :roborock"

	// Fields populated during macro expansion for template binary sensor generation.
	NodeRepresentativeEntityID string // HA entity ID that the node monitors (e.g. sensor.infrastructural_..._battery_level)
	NodeEnablerEntityID        string // optional enabler HA entity ID (condition becomes: node OR enabler is off)
	NodeDelayOff               string // optional delay_off value (e.g. "00:01:00")
	BatteryAlertLevel          int    // alert threshold for battery_alert entities (0 = not set)

	// Fields populated during macro expansion for input_number generation.
	InputNumberMin  string
	InputNumberMax  string
	InputNumberStep string
	InputNumberUnit string
	InputNumberIcon string

	// Fields for template sensor generation.
	AdjustmentOffset string // numeric offset for adjustment sensors (e.g. "20")
	AdjustmentScale  string // numeric scale multiplier (e.g. "1")
	ValueExpr        string // normalised entity reference for value-directive sensors (e.g. "climate.physical/.../bitron_thermostat!current_temperature")

	// Fields for media switch generation.
	MediaSwitchPlayerName  string // HA entity name of the controlled media_player (e.g. "media_player.social/apartment/living_room/sonos")
	MediaSwitchNoPlayInput string // optional source name that should NOT count as "playing" (e.g. "TV")

	// Fields for generic condition-based template entity generation.
	ConditionSources  []string // normalised HA entity IDs for $/$1/$2/... in the condition expression
	ConditionExpr     string   // Jinja2 template expression with $/$1/$2 as placeholders
	ConditionDevClass string   // device_class for the generated template entity
	ConditionDelayOn  string   // delay_on for binary sensors
	ConditionDelayOff string   // delay_off for binary sensors

	// General entity icon from an inline "with icon" or body "icon" directive.
	EntityIcon string

	// Fields for input_boolean generation.
	InputBooleanIcon string // mdi:... icon for the input_boolean entity

	// Fields for input_datetime generation.
	InputDatetimeHasDate bool // whether the input_datetime includes a date component
	InputDatetimeHasTime bool // whether the input_datetime includes a time component
}

const (
	SpaceKindRegular = "space"
	SpaceKindVirtual = "virtual-space"
)

func newAdministrationState() *TAdministrationState {
	state := &TAdministrationState{
		SpacePath:                []string{},
		OpenBlocks:               []string{},
		PendingEntityCollections: []TPendingEntityCollection{},
		SpaceOrder:               []string{},
		SpaceKindByName:          map[string]string{"root": SpaceKindRegular},
		EntitiesBySpace:          map[string][]string{},
		EntityRecordsBySpace:     map[string][]TEntityRecord{},
		EntityRecordSeenBySpace:  map[string]map[string]string{},
		ExternalEntitiesBySpace:  map[string][]string{},
		SpaceDepthByName:         map[string]int{},
		SpaceOffByName:           map[string][]string{},
		SpaceOnByName:            map[string][]string{},
		SpaceOnExplicitByName:    map[string][]string{},
		SpaceLightsByName:        map[string][]string{},
		SpaceMediaByName:         map[string][]string{},
		SpaceLightOffByName:      map[string][]string{},
		HeatingLeaksByName:       map[string][]string{},
		SpaceHasExplicitOn:          map[string]bool{},
		SpaceHasExplicitOff:         map[string]bool{},
		SpaceNoMotionDelayByName:     map[string]int{},
		SpaceMembersByName:           map[string][]string{},
		SpaceSwitchOnByName:          map[string][]string{},
		NodeRepresentativeByEntityID: map[string]string{},
	}

	state.EnsureSpaceRegistered(nil, SpaceKindRegular)
	return state
}

func (state *TAdministrationState) CurrentSpaceName() string {
	return formatNestedSpaceName(state.SpacePath)
}

func (state *TAdministrationState) EnsureSpaceRegistered(path []string, spaceKind string) {
	if len(path) == 0 {
		if _, seen := state.EntitiesBySpace["root"]; !seen {
			state.SpaceOrder = append(state.SpaceOrder, "root")
			state.SpaceDepthByName["root"] = 0
			state.EntitiesBySpace["root"] = []string{}
			state.EntityRecordsBySpace["root"] = []TEntityRecord{}
			state.EntityRecordSeenBySpace["root"] = map[string]string{}
		}
		if _, exists := state.SpaceKindByName["root"]; !exists {
			state.SpaceKindByName["root"] = SpaceKindRegular
		}
		return
	}

	for level := 1; level <= len(path); level++ {
		prefix := append([]string{}, path[:level]...)
		name := formatNestedSpaceName(prefix)
		if _, seen := state.EntitiesBySpace[name]; !seen {
			state.SpaceOrder = append(state.SpaceOrder, name)
			state.SpaceDepthByName[name] = nestedSpaceDepth(prefix)
			state.EntitiesBySpace[name] = []string{}
			state.EntityRecordsBySpace[name] = []TEntityRecord{}
			state.EntityRecordSeenBySpace[name] = map[string]string{}
		}
		if level == len(path) {
			state.SpaceKindByName[name] = spaceKind
		} else if _, exists := state.SpaceKindByName[name]; !exists {
			state.SpaceKindByName[name] = SpaceKindRegular
		}
	}
}

func (state *TAdministrationState) AppendEntityRecord(spaceName string, record TEntityRecord) {
	if _, exists := state.EntityRecordSeenBySpace[spaceName]; !exists {
		state.EntityRecordSeenBySpace[spaceName] = map[string]string{}
	}
	key := record.Name
	if record.NoCollect {
		key += "|no_collect"
	}
	if firstProvenance, seen := state.EntityRecordSeenBySpace[spaceName][key]; seen {
		fmt.Fprintf(os.Stderr, "[WARNING] Entity %q defined more than once in space %q\n  first:  %s\n  second: %s\n",
			record.Name, spaceName, provenanceLabel(firstProvenance), provenanceLabel(record.Provenance))
		return
	}
	state.EntityRecordSeenBySpace[spaceName][key] = record.Provenance
	state.EntityRecordsBySpace[spaceName] = append(state.EntityRecordsBySpace[spaceName], record)

	// Auto-collect into domain-specific collections for @light / @media / @all expansion.
	if !record.NoCollect {
		switch record.Identity.Domain {
		case "light":
			state.SpaceLightsByName[spaceName] = append(state.SpaceLightsByName[spaceName], record.Name)
		case "media_player":
			state.SpaceMediaByName[spaceName] = append(state.SpaceMediaByName[spaceName], record.Name)
		}
	}
}

func provenanceLabel(p string) string {
	if p == "" {
		return "(unknown)"
	}
	return p
}

func (state *TAdministrationState) RegisterEntityClosure(pending TPendingEntityCollection) {
	// Only store the plain entity name (strip any annotation like line numbers)
	plainName := pending.Entry
	if idx := strings.Index(plainName, " "); idx > 0 {
		plainName = plainName[:idx]
	}
	state.EntitiesBySpace[pending.SpaceName] = append(state.EntitiesBySpace[pending.SpaceName], plainName)
	if pending.HasExternalRef {
		state.ExternalEntitiesBySpace[pending.SpaceName] = append(state.ExternalEntitiesBySpace[pending.SpaceName], pending.ExternalEntry)
	}
	state.AppendEntityRecord(pending.SpaceName, pending.Record)
}

func (state *TAdministrationState) OpenSpace(spaceKind, spaceName string) {
	state.OpenBlocks = append(state.OpenBlocks, spaceKind)
	if spaceName == "" {
		state.SpacePath = append(state.SpacePath, "?")
	} else {
		state.SpacePath = append(state.SpacePath, spaceName)
	}
	state.EnsureSpaceRegistered(state.SpacePath, spaceKind)
}

func (state *TAdministrationState) OpenOtherBlock() {
	state.OpenBlocks = append(state.OpenBlocks, "other")
}

func (state *TAdministrationState) HandleEndToken(onSpaceClosed func(string)) {
	if len(state.PendingEntityCollections) > 0 {
		lastPending := state.PendingEntityCollections[len(state.PendingEntityCollections)-1]
		if len(state.OpenBlocks) == lastPending.ExpectedDepth {
			state.RegisterEntityClosure(lastPending)
			state.PendingEntityCollections = state.PendingEntityCollections[:len(state.PendingEntityCollections)-1]
		}
	}

	if len(state.OpenBlocks) == 0 {
		return
	}

	last := state.OpenBlocks[len(state.OpenBlocks)-1]
	state.OpenBlocks = state.OpenBlocks[:len(state.OpenBlocks)-1]
	if (last == SpaceKindRegular || last == SpaceKindVirtual) && len(state.SpacePath) > 0 {
		spaceName := formatNestedSpaceName(state.SpacePath)
		// --- Validation for virtual spaces: check member existence ---
		if last == SpaceKindVirtual {
			for _, entry := range state.EntitiesBySpace[spaceName] {
				entryTrim := strings.TrimSpace(entry)
				if strings.HasPrefix(entryTrim, "member ") {
					// member switch.social:/house/foo/space; or similar
					memberRef := strings.TrimPrefix(entryTrim, "member ")
					memberRef = strings.TrimSuffix(memberRef, ";")
					memberRef = strings.TrimSpace(memberRef)
					found := false
					// Check if this entity exists anywhere
					for _, allEntities := range state.EntitiesBySpace {
						for _, e := range allEntities {
							if strings.HasPrefix(e, memberRef) {
								found = true
								break
							}
						}
						if found {
							break
						}
					}
					if !found {
						Reporter.Warn("Virtual space %q: member reference %q does not exist as an entity", spaceName, memberRef)
					}
				}
			}
		}
		// --- Expand @light / @media / @all tokens and apply defaults ---
		// Expand explicit directives first.
		if state.SpaceHasExplicitOn[spaceName] {
			state.SpaceOnByName[spaceName] = state.expandAggregationTokens(spaceName, state.SpaceOnByName[spaceName], true)
		}
		if state.SpaceHasExplicitOff[spaceName] {
			state.SpaceOffByName[spaceName] = state.expandAggregationTokens(spaceName, state.SpaceOffByName[spaceName], false)
		}
		// Apply defaults when no explicit directive was given and no child space has already
		// contributed entries via propagation (matches bash: light_on @all / space_off @all defaults).
		if len(state.SpaceOnByName[spaceName]) == 0 {
			state.SpaceOnByName[spaceName] = append([]string{}, state.SpaceLightsByName[spaceName]...)
		}
		if len(state.SpaceOffByName[spaceName]) == 0 {
			offDefault := append([]string{}, state.SpaceLightsByName[spaceName]...)
			offDefault = append(offDefault, state.SpaceMediaByName[spaceName]...)
			state.SpaceOffByName[spaceName] = offDefault
		}

		// --- Derive aggregate entities from lights-on and space-off lists ---
		parentPath := state.SpacePath[:len(state.SpacePath)-1]
		parentSpaceName := formatNestedSpaceName(parentPath)

		// addIfAbsent is defined at package level; used here for propagation.

		// deriveEntity registers a derived aggregate entity in the current space if not already
		// declared.  Derived aggregates are marked no_collect so they are not added to
		// SpaceLightsByName / SpaceMediaByName (which would cause self-referencing templates).
		deriveEntity := func(entityName, provenance string) {
			if _, alreadySeen := state.EntityRecordSeenBySpace[spaceName][entityName]; !alreadySeen {
				state.RegisterEntityClosure(TPendingEntityCollection{
					SpaceName: spaceName,
					Entry:     entityName,
					Record: TEntityRecord{
						Name:                  entityName,
						Identity:              extractEntityIdentity(entityName),
						NoCollect:             true,
						HasDefinitionOrImport: true,
						Provenance:            provenance,
					},
				})
			}
		}

		// If this space has a lights-on list, derive light.<spaceName> and propagate to parent.
		// Always propagate — even to root — so sphere-level aggregates can be generated.
		if len(state.SpaceOnByName[spaceName]) > 0 {
			lightEntity := "light." + spaceName
			deriveEntity(lightEntity, "derived: light on "+spaceName)
			state.SpaceOnByName[parentSpaceName] = addIfAbsent(state.SpaceOnByName[parentSpaceName], lightEntity)
		}

		// If this space has a space-off list, derive switch.<spaceName>/space and propagate to parent.
		// Always propagate — even to root — so top-level space switches are trackable.
		if len(state.SpaceOffByName[spaceName]) > 0 {
			switchEntity := "switch." + spaceName + "/space"
			deriveEntity(switchEntity, "derived: space off "+spaceName)
			state.SpaceOffByName[parentSpaceName] = addIfAbsent(state.SpaceOffByName[parentSpaceName], switchEntity)
		}

		// If this space has direct media players OR received media aggregates from child spaces,
		// derive switch.<spaceName>/media and propagate it to the parent so parent space switches
		// and parent media aggregates include it in their turn_off/state.
		hasMedia := len(state.SpaceMediaByName[spaceName]) > 0
		if !hasMedia {
			for _, item := range state.SpaceOffByName[spaceName] {
				if strings.HasPrefix(item, "switch.") && strings.HasSuffix(item, "/media") {
					hasMedia = true
					break
				}
			}
		}
		if hasMedia {
			mediaEntity := "switch." + spaceName + "/media"
			deriveEntity(mediaEntity, "derived: media "+spaceName)
			state.SpaceOffByName[parentSpaceName] = addIfAbsent(state.SpaceOffByName[parentSpaceName], mediaEntity)
		}

		onSpaceClosed(spaceName)
		state.SpacePath = state.SpacePath[:len(state.SpacePath)-1]
	}
}

// GlobalVariables keeps the current value of global variables, seeded from the code defaults.
var GlobalVariables = cloneStringMap(DefaultGlobalVariables)

func cloneStringMap(source map[string]string) map[string]string {
	clone := map[string]string{}
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func ResetGlobalVariables() {
	GlobalVariables = cloneStringMap(DefaultGlobalVariables)
}

func lookupGlobalVariable(name string) (string, bool) {
	value, exists := GlobalVariables[name]
	if exists {
		return value, true
	}
	value, exists = DefaultGlobalVariables[name]
	return value, exists
}

// RecordSpaceOff accumulates entities/aggregations to turn off for a space (from an explicit directive).
// Multiple "space off:" directives in the same space are merged; later calls do not replace earlier ones.
func (state *TAdministrationState) RecordSpaceOff(spaceName string, items []string) {
	for _, item := range items {
		state.SpaceOffByName[spaceName] = addIfAbsent(state.SpaceOffByName[spaceName], item)
	}
	state.SpaceHasExplicitOff[spaceName] = true
}

// RecordSpaceOn records lights to turn on for a space (from an explicit "light on:" directive).
// The explicit list is stored in SpaceOnExplicitByName for use by the generator as the turn_on
// sequence.  SpaceOnByName accumulates both the explicit items and any already-propagated child
// lights, so the generator can use it for the turn_off / state expression.
func (state *TAdministrationState) RecordSpaceOn(spaceName string, lights []string) {
	state.SpaceOnExplicitByName[spaceName] = lights
	for _, light := range lights {
		state.SpaceOnByName[spaceName] = addIfAbsent(state.SpaceOnByName[spaceName], light)
	}
	state.SpaceHasExplicitOn[spaceName] = true
}

// RecordSpaceSwitchOn records the explicit items that the space switch should turn on (from "space on:" directives).
// When set, this overrides the default lights-on list used in the template switch turn_on block.
func (state *TAdministrationState) RecordSpaceSwitchOn(spaceName string, items []string) {
	for _, item := range items {
		state.SpaceSwitchOnByName[spaceName] = addIfAbsent(state.SpaceSwitchOnByName[spaceName], item)
	}
}

// RecordLightOff records the explicit lights-off list for a space (from a "light off:" directive).
// Used by the generator when building the aggregate light entity's turn-off automation.
func (state *TAdministrationState) RecordLightOff(spaceName string, lights []string) {
	state.SpaceLightOffByName[spaceName] = lights
}

// RecordHeatingLeak appends normalised entity references from a "heating leak:" directive.
// All entities in a space's heating-leak list must be open (off) for the radiator to be on safely.
func (state *TAdministrationState) RecordHeatingLeak(spaceName string, entities []string) {
	state.HeatingLeaksByName[spaceName] = append(state.HeatingLeaksByName[spaceName], entities...)
}

// DeriveBinarySensorSubdomainAggregates registers synthetic aggregate entities of the form
// binary_sensor.social/<space>/<subdomain> for every social-sphere space that contains at
// least one binary_sensor entity whose path ends with /<subdomain>.  Parent spaces also get
// an aggregate if any direct child space already has one (bottom-up propagation).
//
// These entities are generated as YAML group files by generateBinarySensorSubdomainGroups;
// registering them here ensures that heating leak: directives that reference them (e.g.
// "heating leak: binary_sensor.social:door" in a space that has a physical door sensor)
// pass RunPostParseChecks without a false undeclared-entity warning.
func (state *TAdministrationState) DeriveBinarySensorSubdomainAggregates() {
	// derivedSubs[spaceName] = set of subdomains for which an aggregate was derived.
	derivedSubs := map[string]map[string]bool{}

	// Process in reverse SpaceOrder so children are handled before parents.
	for i := len(state.SpaceOrder) - 1; i >= 0; i-- {
		spaceName := state.SpaceOrder[i]
		if !strings.HasPrefix(spaceName, "social/") {
			continue
		}

		// Collect subdomains present via direct entities.
		subdomains := map[string]bool{}
		for _, rec := range state.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain != "binary_sensor" || rec.Identity.Path == "" {
				continue
			}
			parts := strings.Split(rec.Identity.Path, "/")
			subdomains[parts[len(parts)-1]] = true
		}

		// Propagate subdomains from direct child spaces.
		parentDepth := strings.Count(spaceName, "/")
		prefix := spaceName + "/"
		for childSpace, childSubs := range derivedSubs {
			if strings.HasPrefix(childSpace, prefix) && strings.Count(childSpace, "/") == parentDepth+1 {
				for sub := range childSubs {
					subdomains[sub] = true
				}
			}
		}

		if len(subdomains) == 0 {
			continue
		}
		derivedSubs[spaceName] = map[string]bool{}
		for sub := range subdomains {
			entityName := "binary_sensor." + spaceName + "/" + sub
			state.registerDerivedBinarySensorAggregate(spaceName, entityName)
			derivedSubs[spaceName][sub] = true
		}
	}
}

func (state *TAdministrationState) registerDerivedBinarySensorAggregate(spaceName, entityName string) {
	if state.EntityRecordSeenBySpace[spaceName] == nil {
		state.EntityRecordSeenBySpace[spaceName] = map[string]string{}
	}
	if _, seen := state.EntityRecordSeenBySpace[spaceName][entityName]; seen {
		return
	}
	state.RegisterEntityClosure(TPendingEntityCollection{
		SpaceName: spaceName,
		Entry:     entityName,
		Record: TEntityRecord{
			Name:                  entityName,
			Identity:              extractEntityIdentity(entityName),
			NoCollect:             true,
			HasDefinitionOrImport: true,
			Provenance:            "derived binary_sensor subdomain aggregate",
		},
	})
}

// heatingPresetNames lists the four heating presets that get their own preset-target entities.
var heatingPresetNames = []string{"around", "away", "day", "night"}

// heatingCapableSpaces returns, keyed by "social/"-stripped physical path, every DSL space that
// contains a climate entity and/or a physical heating switch (switch.physical:.../heating). A
// space with a climate entity qualifies for the full temperature/preset entity set; a space with
// only a heating switch qualifies for the shared leakage/enable control entities.
func heatingCapableSpaces(state *TAdministrationState) (climateSpaces map[string]bool, switchSpaces map[string]bool) {
	climateSpaces = map[string]bool{}
	switchSpaces = map[string]bool{}
	for _, spaceName := range state.SpaceOrder {
		physPath := strings.TrimPrefix(spaceName, "social/")
		for _, rec := range state.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain == "climate" && !rec.Identity.IsRaw {
				climateSpaces[physPath] = true
			}
			if rec.Identity.Domain == "switch" && rec.Identity.Sphere == "physical" {
				p := rec.Identity.Path
				if strings.HasSuffix(p, "/heating") || p == "heating" {
					switchSpaces[physPath] = true
				}
			}
		}
	}
	return climateSpaces, switchSpaces
}

// heatingCapableSocialSpaceNames returns the sorted "social/..." space names for every space
// with a climate entity or a physical heating switch — the spaces that get the shared heating
// leakage/should_be_off control chain regardless of whether an explicit "heating leak:" was
// declared (see generateHeatingLeakageSensors / generateHeatingShouldBeOffSensors).
func heatingCapableSocialSpaceNames(state *TAdministrationState) []string {
	climateSpaces, switchSpaces := heatingCapableSpaces(state)
	union := map[string]bool{}
	for p := range climateSpaces {
		union["social/"+p] = true
	}
	for p := range switchSpaces {
		union["social/"+p] = true
	}
	return sortedStringSlice(union)
}

// hasCoverCloseSpace reports whether any space declares an open/stop/close cover.
func hasCoverCloseSpace(state *TAdministrationState) bool {
	for _, spaceName := range state.SpaceOrder {
		for _, rec := range state.EntityRecordsBySpace[spaceName] {
			if rec.Identity.Domain == "cover" && rec.OpenStopClose {
				return true
			}
		}
	}
	return false
}

// deriveEntityIfAbsent registers a synthetic entity record for an "implied" entity — one the
// legacy bash generator always created for a qualifying space — unless an entity of the same
// name already exists in that space (e.g. a lingering manual declaration takes precedence).
func (state *TAdministrationState) deriveEntityIfAbsent(spaceName, entityName, provenance string, mutate func(*TEntityRecord)) {
	if state.EntityRecordSeenBySpace[spaceName] != nil {
		if _, seen := state.EntityRecordSeenBySpace[spaceName][entityName]; seen {
			return
		}
	}
	record := TEntityRecord{
		Name:                  entityName,
		Identity:              extractEntityIdentity(entityName),
		NoCollect:             true,
		HasDefinitionOrImport: true,
		Provenance:            provenance,
	}
	mutate(&record)
	state.RegisterEntityClosure(TPendingEntityCollection{
		SpaceName: spaceName,
		Entry:     entityName + " (implied)",
		Record:    record,
	})
}

// deriveTemperatureInputNumber registers an implied temperature input_number (10-30°C, step 1,
// mdi:thermometer) — the shared parameters used for temperature_target, temperature_desired and
// every heating_target_preset_for_* entity.
func (state *TAdministrationState) deriveTemperatureInputNumber(spaceName, entityName string) {
	state.deriveEntityIfAbsent(spaceName, entityName, "implied: heating temperature control", func(rec *TEntityRecord) {
		rec.InputNumberMin = "10"
		rec.InputNumberMax = "30"
		rec.InputNumberStep = "1"
		rec.InputNumberUnit = "°C"
		rec.InputNumberIcon = "mdi:thermometer"
	})
}

// deriveInputBoolean registers an implied input_boolean with the given icon.
func (state *TAdministrationState) deriveInputBoolean(spaceName, entityName, icon, provenance string) {
	state.deriveEntityIfAbsent(spaceName, entityName, provenance, func(rec *TEntityRecord) {
		rec.InputBooleanIcon = icon
	})
}

// deriveTimeInputDatetime registers an implied input_datetime with only a time component.
func (state *TAdministrationState) deriveTimeInputDatetime(spaceName, entityName string) {
	state.deriveEntityIfAbsent(spaceName, entityName, "implied: time window", func(rec *TEntityRecord) {
		rec.InputDatetimeHasTime = true
	})
}

// deriveConstantBinarySensor registers an implied binary_sensor whose state is a fixed 'on' or
// 'off' literal, reusing the generic condition-entity generator (empty ConditionSources means the
// expression is emitted verbatim).
func (state *TAdministrationState) deriveConstantBinarySensor(spaceName, entityName, constant string) {
	state.deriveEntityIfAbsent(spaceName, entityName, "implied: constant", func(rec *TEntityRecord) {
		rec.ConditionExpr = "'" + constant + "'"
	})
}

// deriveTimeWindowDatetimePair registers the four input_datetime entities (workday/holiday ×
// start_day/start_night) that back an is_<domain>_day_time / is_<domain>_night_time sensor pair.
func (state *TAdministrationState) deriveTimeWindowDatetimePair(workdayPrefix, holidayPrefix string) {
	for _, prefix := range []string{workdayPrefix, holidayPrefix} {
		state.deriveTimeInputDatetime("root", "input_datetime.social/"+prefix+"/start_day")
		state.deriveTimeInputDatetime("root", "input_datetime.social/"+prefix+"/start_night")
	}
}

// DeriveImpliedHeatingEntities registers every "implied" entity that the legacy bash generator
// created automatically — never requiring a per-house declaration in Entities.def — for the
// occupation tracker, any space with a climate device or physical heating switch, and the
// cover/vacuum time-window controls. See Old/<House>/Modules/Generate.1.hass:
// GenerateOccupationControl, GenerateLocalHeatingControls, GenerateLocalClimateDeviceControl,
// MaybeGenerateHeatingPresetings, MaybeGenerateGlobalHeatingControl, MaybeGenerateCoverControl,
// MaybeGenerateVacuumingControl.
func (state *TAdministrationState) DeriveImpliedHeatingEntities() {
	state.deriveInputBoolean("root", "input_boolean.social/occupation/home", "mdi:home", "implied: occupation")
	state.deriveInputBoolean("root", "input_boolean.social/occupation/around", "mdi:city", "implied: occupation")
	state.deriveInputBoolean("root", "input_boolean.social/occupation/away", "mdi:airplane", "implied: occupation")

	state.deriveConstantBinarySensor("root", "binary_sensor.infrastructural/on", "on")
	state.deriveConstantBinarySensor("root", "binary_sensor.infrastructural/off", "off")

	climateSpaces, switchSpaces := heatingCapableSpaces(state)
	heatingSpaces := map[string]bool{}
	for p := range climateSpaces {
		heatingSpaces[p] = true
	}
	for p := range switchSpaces {
		heatingSpaces[p] = true
	}

	for _, physPath := range sortedStringSlice(heatingSpaces) {
		socialSpace := "social/" + physPath
		state.deriveInputBoolean(socialSpace, "input_boolean.social/"+physPath+"/heating/enable", "mdi:radiator", "implied: heating control")
		state.deriveInputBoolean(socialSpace, "input_boolean.social/"+physPath+"/heating/leakage/ignore", "mdi:radiator", "implied: heating control")

		if climateSpaces[physPath] {
			state.deriveTemperatureInputNumber(socialSpace, "input_number.social/"+physPath+"/temperature_desired")
			state.deriveTemperatureInputNumber(socialSpace, "input_number.social/"+physPath+"/heating_target_preset_for_leakage")
			for _, preset := range heatingPresetNames {
				state.deriveTemperatureInputNumber(socialSpace, "input_number.social/"+physPath+"/heating_target_preset_for_"+preset)
			}
		}
	}

	if len(heatingSpaces) > 0 {
		state.deriveTimeWindowDatetimePair("heating_workday", "heating_holiday")
	}

	if hasCoverCloseSpace(state) {
		state.deriveTimeWindowDatetimePair("coverage_workday", "coverage_holiday")
		state.deriveInputBoolean("root", "input_boolean.social/covers/auto_control", "mdi:dots-vertical", "implied: cover control")
	}

	if len(allEntityRecordsByDomain(state, "vacuum")) > 0 {
		state.deriveTimeWindowDatetimePair("vacuuming_workday", "vacuuming_holiday")
		state.deriveInputBoolean("root", "input_boolean.social/vacuum_should_be_active", "mdi:robot-vacuum", "implied: vacuum control")
		state.deriveInputBoolean("root", "input_boolean.social/vacuuming_requested", "mdi:robot-vacuum", "implied: vacuum control")
	}
}

// expandAggregationTokens resolves @light, @media, and @all in a list of entity refs.
// @light  → all light-domain entities in the space (no_collect excluded)
// @media  → all media_player-domain entities in the space (no_collect excluded)
// @all    → @light + @media
// In the context of "light on:", @all is treated as @light (matches bash light_on @all behaviour).
// For virtual spaces, @light / @media / @all also include the corresponding derived entities
// (light.<member>, switch.<member>/media) from each declared member space.
func (state *TAdministrationState) expandAggregationTokens(spaceName string, items []string, lightOnContext bool) []string {
	var result []string
	for _, item := range items {
		switch item {
		case "@light":
			result = append(result, state.SpaceLightsByName[spaceName]...)
			result = append(result, state.virtualMemberLights(spaceName)...)
		case "@media":
			if !lightOnContext {
				result = append(result, state.SpaceMediaByName[spaceName]...)
				result = append(result, state.virtualMemberMedia(spaceName)...)
			}
		case "@all":
			result = append(result, state.SpaceLightsByName[spaceName]...)
			result = append(result, state.virtualMemberLights(spaceName)...)
			if !lightOnContext {
				result = append(result, state.SpaceMediaByName[spaceName]...)
				result = append(result, state.virtualMemberMedia(spaceName)...)
				result = append(result, state.virtualMemberSpaceSwitches(spaceName)...)
			}
		default:
			result = append(result, item)
		}
	}
	return result
}

// virtualMemberLights returns the derived light entities for each member space of a virtual space.
// Returns nil for non-virtual spaces or spaces with no members.
func (state *TAdministrationState) virtualMemberLights(spaceName string) []string {
	members := state.SpaceMembersByName[spaceName]
	if len(members) == 0 {
		return nil
	}
	var result []string
	for _, member := range members {
		if len(state.SpaceOnByName[member]) > 0 {
			result = addIfAbsent(result, "light."+member)
		}
	}
	return result
}

// virtualMemberMedia returns the derived media switch entities for each member space of a virtual space.
// Returns nil for non-virtual spaces or spaces with no members.
func (state *TAdministrationState) virtualMemberMedia(spaceName string) []string {
	members := state.SpaceMembersByName[spaceName]
	if len(members) == 0 {
		return nil
	}
	var result []string
	for _, member := range members {
		mediaSwitch := "switch." + member + "/media"
		for _, off := range state.SpaceOffByName[member] {
			if off == mediaSwitch {
				result = addIfAbsent(result, mediaSwitch)
				break
			}
		}
	}
	return result
}

// virtualMemberSpaceSwitches returns the space switch entities for each member space of a virtual space.
// Included in @all so that turning off a virtual space also turns off its constituent sub-space switches.
func (state *TAdministrationState) virtualMemberSpaceSwitches(spaceName string) []string {
	members := state.SpaceMembersByName[spaceName]
	if len(members) == 0 {
		return nil
	}
	var result []string
	for _, member := range members {
		if len(state.SpaceOffByName[member]) > 0 {
			result = addIfAbsent(result, "switch."+member+"/space")
		}
	}
	return result
}

// addIfAbsent appends item to slice only if it is not already present.
func addIfAbsent(slice []string, item string) []string {
	for _, existing := range slice {
		if existing == item {
			return slice
		}
	}
	return append(slice, item)
}
