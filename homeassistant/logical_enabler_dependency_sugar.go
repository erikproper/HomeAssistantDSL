/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: LogicalEnablerDependencySugar
 *
 * Desugars a Logical.def device's own "<domain>.<label> enabled by <domain>.<label> from
 * <device-id>;"/"dependency on <device-id>;" convenience lines (integration_logical_storage.go's
 * TLogicalDevice.EnabledBy/DependsOn) into the exact same Capabilities["enabler"] (absorb)/
 * Capabilities["node"] (derived condition) entries the old hand-written longhand already
 * produced -- e.g. Junglinster's own "appliance.tv_living_room", pre-migration:
 *
 *   device appliance.tv_living_room with:
 *     capabilities from switch.house_living_room_tv:
 *       switch.enabler core;
 *     end;
 *
 *     derived binary_sensor.node with:
 *       condition
 *         (physical appliance.tv_living_room and utility.sonytv) or
 *         jinja "$1 == 'off'" { enabler };
 *     end;
 *   end;
 *
 * becomes, post-migration:
 *
 *   device appliance.tv_living_room with:
 *     dependency on utility.sonytv;
 *     media_player.core enabled by switch.core from switch.house_living_room_tv;
 *   end;
 *
 * (2026-09-28: this REPLACES an earlier, simpler "enabler <device-id>;" one-liner form -- built and
 * live-tested for this same device just one day earlier -- with the fuller, domain-qualified LHS
 * form above; confirmed with the user this is a full replacement, not a coexisting alternative.
 * Grammar note, same day: "is enabled by" was trimmed to "enabled by" -- it reads better directly
 * after "with:", the user's own call.
 * The base self-check changes accordingly: instead of the whole device's own "physical <id>"
 * liveness, it's now built from the NAMED LHS capability's own "is available" state -- more
 * precise for a device with several capabilities where only ONE of them actually signals "the
 * appliance is genuinely on". For appliance.tv_living_room specifically the two are actually
 * IDENTICAL in effect -- its own Physical.def "node" capability is ITSELF defined as "derived
 * binary_sensor.node with condition media_player.core is available;", so "physical <id>" and
 * "core is available" already resolved to the exact same thing; byte-diffed clean confirming this.
 * The win is for a FUTURE device whose own "node" isn't simply "this one capability is available".)
 *
 * "enabler" is applied LAST (2026-09-27, the user's own explicit design): it wraps whatever base
 * condition "dependency on" (plus the self-check above) already built, as an "... or jinja
 * \"$1 == 'off'\" { enabler }" OR-clause -- a deliberately powered-down device is not a fault. Each
 * "dependency on" target is referenced bare (no "physical" needed --
 * resolveLogicalSiblingSourceEntityID's own isKnownDeviceID check already resolves a bare
 * device-id reference to that OTHER device's own availability with no ambiguity, exactly matching
 * every hand-written precedent in both houses).
 *
 * Deliberately conservative about WHEN it applies:
 *   - Only when the device declares NEITHER of these lines at all (EnabledBy == nil &&
 *     len(DependsOn) == 0) is this a no-op -- most Logical.def devices never touch it.
 *   - An already-present explicit "node" capability always wins (a device mid-migration, still on
 *     the old hand-written longhand, is left completely untouched) -- checked by map key, not by
 *     content, so a partially-migrated device can't have its own explicit declaration silently
 *     clobbered.
 *   - A hassbridge/import-kind device's own availability is real MQTT-reported and already wired
 *     via depends_on_availability (resolveDependencyAvailabilityTopics, integration_logical_storage.go)
 *     -- "dependency on" ALONE (no "enabled by"), for such a device, must keep working exactly as
 *     it does today: no synthesized condition-tree "node" override nobody asked for, since a pure
 *     AND-of-availability is already expressed natively via MQTT's own "availability:" topic list.
 *     Confirmed via isHassbridgeOrImport, callable by the collector (generator.go) since
 *     hassBridgeDevicesByID/importedDevicesByID are already collected before Logical.def's own pass
 *     runs.
 *   - **A hassbridge/import-kind device WITH "enabled by" is DIFFERENT (fixed 2026-09-28, a real
 *     gap found while reviewing this file with the user)**: an enabler is an OR/exception ("unless
 *     the switch reports literally off, that's not a fault"), not a plain AND -- and HA's own MQTT
 *     "availability:" topic list can only ever AND multiple topics together, with no way to express
 *     an OR/exception at that level. So a template-based "node" condition is UNAVOIDABLE whenever
 *     "enabled by" is declared, even for a hassbridge/import-kind device -- the earlier code
 *     unconditionally skipped node synthesis for that whole kind, which meant an "enabler"
 *     capability would get silently absorbed but NEVER ACTUALLY WIRED into the device's own
 *     liveness at all (a latent bug, never hit live since the one hassbridge-kind device that needs
 *     an enabler, Vienna's own appliance.washing_machine, still uses the full hand-written longhand
 *     instead of this sugar). The kind-based skip now applies ONLY to the pure dependency-only case
 *     (EnabledBy == nil) -- exactly the case depends_on_availability already covers correctly.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 28.09.2026
 *
 */

package main

import "fmt"

// applyLogicalEnablerDependencySugar mutates devices in place, desugaring every device with a
// non-nil EnabledBy and/or non-empty DependsOn (and no pre-existing explicit "node" capability of
// its own) into the fully-expanded Capabilities entries. isHassbridgeOrImport reports whether
// deviceID is declared under Physical.def's "home_assistant"/"import" integrations (real MQTT
// availability of its own) -- such a device is skipped for the "node" synthesis ONLY when it has no
// "enabled by" of its own (see this file's own header comment for why an enabler always needs a
// template, kind-independent); "enabler" itself, once EnabledBy is set, always gets its own absorb
// entry regardless of kind.
//
// The LHS capability named by EnabledBy is validated implicitly, not via a separate pre-check: its
// bare label is resolved as an ordinary same-device sibling ref through the exact same machinery
// every other condition-tree ref already goes through (resolveLogicalSiblingSourceEntityID, via
// parseConditionExpr below) -- if the device genuinely has no such capability declared at the
// physical layer, that resolution fails the normal way (a clear, already-established warning, or a
// deferred retry if the file just hasn't reached that declaration yet), exactly the "hard error"
// the design called for, with no bespoke validation logic needed here.
func applyLogicalEnablerDependencySugar(devices map[string]TLogicalDevice, isHassbridgeOrImport func(deviceID string) bool, jinjaTemplates map[string]TJinjaTemplateDefinition) []string {
	var warnings []string
	for id, d := range devices {
		if d.EnabledBy == nil && len(d.DependsOn) == 0 {
			continue
		}

		// A hassbridge/import-kind device with NO "enabled by" is left to depends_on_availability
		// entirely (a pure AND-of-availability, natively expressible via MQTT's own topic list) --
		// but one WITH "enabled by" always needs this template (an OR/exception can't be expressed
		// any other way), so the kind-based skip is scoped to the dependency-only case alone.
		needsTemplate := d.EnabledBy != nil || !isHassbridgeOrImport(id)
		if _, hasNode := d.Capabilities["node"]; !hasNode && needsTemplate {
			var conditionText string
			if d.EnabledBy != nil {
				// Domain-qualified (2026-09-29, mandatory-domain-qualification generalized to a plain
				// condition/value ref, Conceptual_LogicalEntities.go's own resolveLeaf) -- OwnDomain is
				// already captured on TLogicalEnabledByFact for exactly this, so this was a real gap
				// (found live: Junglinster's appliance.tv_living_room), not a hardcoded guess.
				conditionText = d.EnabledBy.OwnDomain + "." + d.EnabledBy.OwnLabel + " is available"
			} else {
				conditionText = "physical " + id
			}
			for _, dep := range d.DependsOn {
				conditionText += " and " + dep
			}
			if d.EnabledBy != nil {
				// The jinja source is domain-qualified (2026-09-28, mandatory-domain-qualification
				// generalized to jinja clauses, Conceptual_LogicalEntities.go's own render) --
				// "enabler"'s own declared domain is always d.EnabledBy.SourceDomain (set two lines
				// below), so that's what's written here too, not a hardcoded "switch". Named
				// "$[domain.label]" placeholder, embedded directly in the template text (2026-10-05,
				// PROJECT.md 1h) -- see logical_condition_expr.go's own grammar comment.
				conditionText = "(" + conditionText + ") or jinja \"$[" + d.EnabledBy.SourceDomain + ".enabler] == 'off'\""
			}
			sourceLabel := fmt.Sprintf("Logical.def: device %q's auto-derived \"node\" (enabler/dependency sugar)", id)
			tree, exprWarnings := parseConditionExpr(conditionText, jinjaTemplates, sourceLabel)
			warnings = append(warnings, exprWarnings...)
			if d.Capabilities == nil {
				d.Capabilities = map[string]TLogicalCapability{}
			}
			d.Capabilities["node"] = TLogicalCapability{Domain: "binary_sensor", IsDerivedCondition: true, DerivedCondition: tree, DelayOff: d.NodeDelayOff}
		}

		if d.EnabledBy != nil {
			if _, hasEnabler := d.Capabilities["enabler"]; !hasEnabler {
				if d.Capabilities == nil {
					d.Capabilities = map[string]TLogicalCapability{}
				}
				d.Capabilities["enabler"] = TLogicalCapability{Domain: d.EnabledBy.SourceDomain, AbsorbedFromDeviceID: d.EnabledBy.SourceDeviceID, AbsorbedFromCapability: d.EnabledBy.SourceLabel}
			}
		}

		devices[id] = d
	}
	return warnings
}
