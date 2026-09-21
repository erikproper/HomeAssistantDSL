/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: MainEntities
 *
 * Kind-5 (PROJECT.md item 1, 2026-09-07): collects Spaces.def's "bare" entity declarations --
 * "entity <domain>.<path>;" with no `value`/`condition`/`from ... entity ...`/anything else at all
 * -- and writes them to <outputRoot>/coordinator/main_entities.yaml, for
 * house_event_bus_coordinator/entity_existence.go's own TEntityExistenceTracker to seed instance
 * "main" with (SeedMainEntities). These are entities the DSL author trusts to already exist as
 * real, native entities on the house's own main Home Assistant instance -- predating all the
 * device/MQTT machinery ("like all entities used to be," the user's own words) -- and today have
 * zero existence checking at all.
 *
 * Deliberately reads TAdministrationState.EntityRecordsBySpace (already-parsed data), filtered on
 * TEntityRecord.HasDefinitionOrImport == false AND DiscoveryImplied == false -- NOT
 * ExternalEntitiesBySpace, which is decorated diagnostic text ("name (line N) [config options:
 * ...]"), has no reader anywhere else in the codebase, and isn't a usable entity-id list. The
 * DiscoveryImplied exclusion matters: RegisterDiscoveryImpliedEntity (administration.go) ALSO sets
 * HasDefinitionOrImport=false for entities the coordinator itself will materialise via MQTT
 * discovery (kind-1/2/3/4/commandline) -- those are not "assumed to exist on main" at all, and
 * must not be swept into this list.
 *
 * No new Physical.def/Spaces.def grammar is introduced by this file at all -- it operates entirely
 * on data the existing parser already produces.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 07.09.2026
 *
 */

package main

import (
	"path/filepath"
	"sort"
	"strings"
)

// collectMainEntityIDs returns the deduplicated, sorted set of real HA entity ids for every
// "bare" Spaces.def entity declaration (no definition, no import, not discovery-implied) --
// see this file's own header comment for why those two exclusions both matter.
func collectMainEntityIDs(admin *TAdministrationState) []string {
	seen := map[string]bool{}
	for _, records := range admin.EntityRecordsBySpace {
		for _, rec := range records {
			if rec.HasDefinitionOrImport || rec.DiscoveryImplied {
				continue
			}
			id := toHomeAssistantEntityID(rec.Name)
			if id == "" {
				continue
			}
			seen[id] = true
		}
	}
	entityIDs := make([]string, 0, len(seen))
	for id := range seen {
		entityIDs = append(entityIDs, id)
	}
	sort.Strings(entityIDs)
	return entityIDs
}

// collectDiscoveryImpliedEntityIDs returns the deduplicated, sorted set of real HA entity ids for
// every entity record RegisterDiscoveryImpliedEntity (administration.go) marked DiscoveryImplied --
// i.e. every entity kind-1/2/3/4/commandline's own discovery relay will materialise on "main" all
// by itself, with no hassbridge/kind-5 declaration involved at all. This is the exact inverse
// selection of collectMainEntityIDs' own DiscoveryImplied exclusion (see that function's own header
// comment): kind-5's file must NOT list these (they're not "assumed to exist," the coordinator
// relays them), but generateEntityCatalogueSuggestions' own "what's still unclaimed on main" report
// must NOT suggest them either -- they're already fully claimed, just via a different mechanism
// than a hassbridge cross-post. Real bug found live 2026-09-20: every discovery-declared device's
// own entities (e.g. bedroom_bed_moes' event/battery_level) kept reappearing in
// suggestions/home_assistant_main.txt as "hass.discovered_..." blocks, since neither
// usedHassBridgeEntityIDs nor mainEntityIDs ever covered discovery-kind entities at all.
func collectDiscoveryImpliedEntityIDs(admin *TAdministrationState) []string {
	seen := map[string]bool{}
	for _, records := range admin.EntityRecordsBySpace {
		for _, rec := range records {
			if !rec.DiscoveryImplied {
				continue
			}
			id := toHomeAssistantEntityID(rec.Name)
			if id == "" {
				continue
			}
			seen[id] = true
		}
	}
	entityIDs := make([]string, 0, len(seen))
	for id := range seen {
		entityIDs = append(entityIDs, id)
	}
	sort.Strings(entityIDs)
	return entityIDs
}

// generateMainEntitiesFile writes <outputRoot>/coordinator/main_entities.yaml -- mirrors
// generateHomeAssistantInstancesFile's own flat-list shape exactly. Unconditional (even an empty
// list is written, same "baseline always exists" convention generateCoordinatorDevicesFile
// follows), so the coordinator never has to distinguish "no file yet" from "genuinely nothing
// declared".
func generateMainEntitiesFile(outputRoot string, entityIDs []string) error {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("entities:\n")
	for _, id := range entityIDs {
		sb.WriteString("  - " + id + "\n")
	}
	dir := filepath.Join(outputRoot, "coordinator")
	return writeYAMLFile(filepath.Join(dir, "main_entities.yaml"), sb.String())
}
