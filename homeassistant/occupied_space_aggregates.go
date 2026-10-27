/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: OccupiedSpaceAggregates
 *
 * A space aggregate (the sensor/binary_sensor groups, the space lights, the space and media
 * switches) isn't generated when an entity is positioned at its own name (2026-10-24, Erik): a
 * space's only sensor renamed to the aggregate's name (e.g. "entity sensor.social:humidity;")
 * then stands for the space itself, and the entity_id the UI uses stays. When other members would
 * be aggregated too ("binary_sensor.social:door" next to "binary_sensor.social:left/door"), the
 * aggregation can't be generated at all: generation warns, and the positioned entity needs a
 * qualifier of its own ("right/door"). An aggregate of one single member gets a mild warning: that
 * member could take the aggregate's name instead (2026-10-26).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 26.10.2026
 *
 */

package main

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// spaceAggregateMembers maps each space aggregate's entity_id to its members' entity_ids, filled by
// the aggregate builders during generation; positionedEntityIDs holds the entity_ids positioned in
// Conceptual.def, positionedSpaceOf the space each is positioned in. All are reset per run by
// generateFromPaths.
var (
	spaceAggregateMembers map[string][]string
	positionedEntityIDs   map[string]bool
	positionedSpaceOf     map[string]string
)

// recordSpaceAggregate registers entityID as a space aggregate of memberIDs, and reports whether
// the builder may generate it: not when an entity is positioned at the aggregate's own name.
func recordSpaceAggregate(admin *TAdministrationState, entityID string, memberIDs []string) bool {
	if spaceAggregateMembers == nil {
		spaceAggregateMembers = map[string][]string{}
	}
	spaceAggregateMembers[entityID] = append(spaceAggregateMembers[entityID], memberIDs...)
	if positionedEntityIDs == nil {
		positionedEntityIDs = map[string]bool{}
		positionedSpaceOf = map[string]string{}
		for space, records := range admin.EntityRecordsBySpace {
			for _, rec := range records {
				// The generator's own registrations of these aggregates are no positioning.
				if !strings.HasPrefix(rec.Provenance, "derived") && !strings.HasPrefix(rec.Provenance, "heating_leak:") {
					positionedEntityIDs[toHomeAssistantEntityID(rec.Name)] = true
					positionedSpaceOf[toHomeAssistantEntityID(rec.Name)] = space
				}
			}
		}
	}
	return !positionedEntityIDs[entityID]
}

// dropShadowedDerivedAggregates removes every space aggregate the parser derived ("derived: ..."
// provenance) whose entity_id a positioned entity already has: that entity stands for the space,
// and the aggregate's builder skips it anyway (recordSpaceAggregate). A space's aggregate is
// derived when the space closes, so it only yields by itself to an entity registered before that
// -- a media player's wrapper switch is registered later, which made it a second declaration of
// the space's own media switch (PROJECT.md 17g, 2026-10-26).
func dropShadowedDerivedAggregates(admin *TAdministrationState) {
	positioned := map[string]bool{}
	for _, records := range admin.EntityRecordsBySpace {
		for _, rec := range records {
			if !strings.HasPrefix(rec.Provenance, "derived") {
				positioned[toHomeAssistantEntityID(rec.Name)] = true
			}
		}
	}
	for space, records := range admin.EntityRecordsBySpace {
		kept := records[:0]
		for _, rec := range records {
			if strings.HasPrefix(rec.Provenance, "derived:") && positioned[toHomeAssistantEntityID(rec.Name)] {
				continue
			}
			kept = append(kept, rec)
		}
		admin.EntityRecordsBySpace[space] = kept
	}
}

// occupiedSpaceAggregates returns the aggregates whose entity_id is taken by a positioned entity,
// each with its other members.
func occupiedSpaceAggregates() map[string][]string {
	occupied := map[string][]string{}
	for aggregate, members := range spaceAggregateMembers {
		if !positionedEntityIDs[aggregate] {
			continue
		}
		others := []string{}
		for _, member := range members {
			if member != aggregate && !slices.Contains(others, member) {
				others = append(others, member)
			}
		}
		sort.Strings(others)
		occupied[aggregate] = others
	}
	return occupied
}

// singletonSpaceAggregates returns the generated aggregates with exactly one member of their own
// domain, positioned in the aggregate's own space, each with that member: the member could carry
// the aggregate's name and stand for the space itself (2026-10-26, Erik). A member positioned in a
// deeper space can only stand for that one, so the enclosing spaces' aggregates are left out. The
// heating leakage evidence groups (sphere physical) always stay: a leakage cause needs its
// binary_sensor wrapper. So does a space switch ("<space>/space"): its members are the space's
// "space off" items, not sources of its own kind. A media switch ("<space>/media") does count: its
// members are the players' own media switches, whichever name each is positioned at (PROJECT.md
// 17g, 2026-10-26).
func singletonSpaceAggregates() map[string]string {
	singletons := map[string]string{}
	for aggregate, members := range spaceAggregateMembers {
		domain, object, _ := strings.Cut(aggregate, ".")
		if positionedEntityIDs[aggregate] || strings.HasPrefix(object, "physical_") || (domain == "switch" && strings.HasSuffix(object, "_space")) {
			continue
		}
		distinct := slices.Compact(slices.Sorted(slices.Values(members)))
		if len(distinct) != 1 || !strings.HasPrefix(distinct[0], domain+".") {
			continue
		}
		space := toHomeAssistantEntityID(domain + "." + positionedSpaceOf[distinct[0]])
		if aggregate == space || strings.HasPrefix(aggregate, space+"_") {
			singletons[aggregate] = distinct[0]
		}
	}
	return singletons
}

// warnOccupiedSpaceAggregates reports each aggregation that a positioned entity blocks, and (mildly)
// each aggregate of one single member.
func warnOccupiedSpaceAggregates() {
	occupied := occupiedSpaceAggregates()
	for _, aggregate := range sortedKeys(occupied) {
		if others := occupied[aggregate]; len(others) > 0 {
			fmt.Printf("[conceptual] %s is positioned directly, so the space can't aggregate %s into it -- give the positioned entity a qualifier of its own (e.g. .../right/<name>)\n", aggregate, strings.Join(others, ", "))
		}
	}
	singletons := singletonSpaceAggregates()
	for _, aggregate := range sortedMapKeys(singletons) {
		fmt.Printf("[conceptual] note: %s aggregates only %s -- consider positioning that entity at the aggregate's name, so it stands for the space\n", aggregate, singletons[aggregate])
	}
}

// noteNodesAtSpaceNames notes (mildly) every node positioned at its own space's name in a space
// that holds more than one device, e.g. "binary_sensor.infrastructural:node;" next to a light that
// stands for a landing that also has a tap and a radiator: there the node doesn't say which
// device it is the node of -- "light/node" does (2026-10-26, Erik). A space that is the device
// itself (a car, "cars/xc40/node") is no such case.
func noteNodesAtSpaceNames(admin *TAdministrationState) {
	deviceOf := regexp.MustCompile(` from (\S+)`)
	var names []string
	for spaceName, records := range admin.EntityRecordsBySpace {
		path := spaceContextPath(spaceName)
		if path == "" {
			continue
		}
		var devices, nodes []string
		for _, rec := range records {
			if m := deviceOf.FindStringSubmatch(rec.Provenance); m != nil && !strings.HasPrefix(rec.Provenance, "derived") {
				devices = addIfAbsent(devices, m[1])
			}
			if rec.Identity.Domain == "binary_sensor" && rec.Identity.Path == path+"/node" {
				nodes = append(nodes, toHomeAssistantEntityID(rec.Name))
			}
		}
		if len(devices) > 1 {
			names = append(names, nodes...)
		}
	}
	for _, name := range slices.Compact(slices.Sorted(slices.Values(names))) {
		fmt.Printf("[conceptual] note: %s sits at its space's own name, in a space with several devices, so it doesn't say which one it is the node of -- e.g. \".../light/node\"\n", name)
	}
}
