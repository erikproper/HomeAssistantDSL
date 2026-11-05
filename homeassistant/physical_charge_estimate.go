/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: PhysicalChargeEstimate
 *
 * PROJECT.md 31a (2026-11-05, Erik): a battery level estimated from the days since the batteries
 * were replaced, for rechargeable (Li-ion) batteries, which keep their voltage until almost empty,
 * so that the level a device reports says little (Junglinster's dining Philips dimmer reported ~1 %
 * for six months while working fine). In Physical.def, a discovery device's capability
 *
 *   derived capability sensor.battery_level as unique_id <id>
 *     by charge lasting 90 days since 31.10.2026;
 *
 * is a template sensor at 100 * (X - D) / X percent (X the days a charge is assumed to last, D the
 * days since the batteries were replaced, kept between 0 and 100), next to two entities of its own:
 * an input_datetime holding the replacement date ("batteries_replaced", the date above until it is
 * first set) and a button setting it to today. A "derived ... from sensor.battery_level via ..."
 * alert reads it like any other sibling.
 *
 * The line is parsed by a CDL1-style parser (GO_CONVENTIONS.md §8) on the rule grammar's
 * tokeniser (rule_tokeniser.go).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 05.11.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// --- parser ---

// TChargeEstimateDeclaration is one "derived capability <domain>.<label> by charge lasting <days>
// days since <dd.mm.yyyy>;" line; Since is the date as yyyy-mm-dd.
type TChargeEstimateDeclaration struct {
	Domain string
	Label  string
	Days   int
	Since  string
}

// TChargeEstimateParser embeds TDefTokeniser (chaining the layers, GO_CONVENTIONS.md §7).
type TChargeEstimateParser struct {
	TDefTokeniser
	Declaration TChargeEstimateDeclaration
	Warnings    []string
}

// parseChargeEstimateLine parses line as a charge estimate. ok is false when it isn't one -- the
// other capability forms then get it -- and warnings say what is wrong with one that is.
func parseChargeEstimateLine(line string) (declaration TChargeEstimateDeclaration, ok bool, warnings []string) {
	p := &TChargeEstimateParser{}
	p.Initialise(line)
	ok = p.ChargeEstimateStatement()
	return p.Declaration, ok, p.Warnings
}

// ReportParsingError records a warning and returns true, for the right of a "||" chain.
func (p *TChargeEstimateParser) ReportParsingError(format string, args ...any) bool {
	p.Warnings = append(p.Warnings, fmt.Sprintf("charge estimate of %q: %s", p.Declaration.Domain+"."+p.Declaration.Label, fmt.Sprintf(format, args...)))
	return true
}

// Fail always returns false: the end of a "||" error chain.
func (p *TChargeEstimateParser) Fail() bool {
	return false
}

// ForcedThisWordWas requires the current token to be exactly word, consuming it.
func (p *TChargeEstimateParser) ForcedThisWordWas(word string) bool {
	return p.ThisWordWas(word) ||
		p.ReportParsingError("expected %q", word) && p.Fail()
}

// ForcedThisPunctuationWas requires the current token to be exactly ch, consuming it.
func (p *TChargeEstimateParser) ForcedThisPunctuationWas(ch byte) bool {
	return p.ThisPunctuationWas(ch) ||
		p.ReportParsingError("expected %q", string(ch)) && p.Fail()
}

// ChargeEstimateStatement := "derived" "capability" CapabilityName "by" ChargeEstimate.
// Up to "by" it may be another derived form, which then isn't ours.
func (p *TChargeEstimateParser) ChargeEstimateStatement() bool {
	return p.ThisWordWas("derived") && p.ThisWordWas("capability") && p.CapabilityName() &&
		p.ThisWordWas("by") && p.ChargeEstimate()
}

// CapabilityName := Word "." Word { "/" Word }.
func (p *TChargeEstimateParser) CapabilityName() bool {
	if !p.ThisWord(&p.Declaration.Domain) || !p.ThisPunctuationWas('.') || !p.ThisWord(&p.Declaration.Label) {
		return false
	}
	var segment string
	for p.ThisPunctuationWas('/') {
		if !p.ThisWord(&segment) {
			return false
		}
		p.Declaration.Label += "/" + segment
	}
	return true
}

// ChargeEstimate := "charge" "lasting" Days "days" "since" Date ";".
func (p *TChargeEstimateParser) ChargeEstimate() bool {
	return p.ForcedThisWordWas("charge") && p.ForcedThisWordWas("lasting") && p.Days() &&
		p.ForcedThisWordWas("days") && p.ForcedThisWordWas("since") && p.Date() &&
		p.ForcedThisPunctuationWas(';') &&
		(p.AtEndOfTokens() || p.ReportParsingError("unexpected text after \";\"") && p.Fail())
}

// Days := a whole number of days, at least 1.
func (p *TChargeEstimateParser) Days() bool {
	var word string
	if !p.ThisWord(&word) {
		return p.ReportParsingError("expected the number of days a charge lasts") && p.Fail()
	}
	days, err := strconv.Atoi(word)
	if err != nil || days < 1 {
		return p.ReportParsingError("%q is no number of days", word) && p.Fail()
	}
	p.Declaration.Days = days
	return true
}

// Date := Day "." Month "." Year, a date that exists.
func (p *TChargeEstimateParser) Date() bool {
	var day, month, year string
	if !(p.ThisWord(&day) && p.ForcedThisPunctuationWas('.') && p.ThisWord(&month) && p.ForcedThisPunctuationWas('.') && p.ThisWord(&year)) {
		return p.ReportParsingError("expected a date, dd.mm.yyyy") && p.Fail()
	}
	date, err := time.Parse("2.1.2006", day+"."+month+"."+year)
	if err != nil {
		return p.ReportParsingError("%s.%s.%s is no date", day, month, year) && p.Fail()
	}
	p.Declaration.Since = date.Format("2006-01-02")
	return true
}

// --- positioning ---

// chargeReplacedSegment names the replacement date and its button, next to the level.
// chargeButtonKeyPrefix roots the button's source key ("charge:<device-id>!<label>") in its level's.
const (
	chargeReplacedSegment = "batteries_replaced"
	chargeButtonKeyPrefix = "charge:"
)

// registerDiscoveryChargeEntityLink positions a charge estimate (see the file comment): the level,
// the replacement date and the button setting it.
func registerDiscoveryChargeEntityLink(administration *TAdministrationState, decl TDiscoveryEntityDeclaration, capability TDiscoveryCapability, deviceNamePath, entitiesPath string, lineNum int) []string {
	provenance := fmt.Sprintf("%s:%d → %s from %s with %s", filepath.Base(entitiesPath), lineNum, decl.EntitySpec, decl.GatewayDeviceID, decl.Leaf)
	fullName := resolveDeviceEntityFullName(decl.EntitySpec, administration.SpacePath, deviceNamePath)
	identity := extractEntityIdentity(fullName)
	entityID := toHomeAssistantEntityID(fullName)
	if identity.Domain != "sensor" || entityID == "" {
		return []string{fmt.Sprintf("%s: a charge estimate is a sensor, not %q", provenance, decl.EntitySpec)}
	}
	path := chargeReplacedSegment
	if parent := stripLastPathSegment(identity).Path; parent != "" {
		path = parent + "/" + chargeReplacedSegment
	}
	dateName := "input_datetime." + identity.Sphere + "/" + path
	buttonName := "button." + identity.Sphere + "/" + path
	dateID := toHomeAssistantEntityID(dateName)
	spaceName := administration.CurrentSpaceName()
	administration.AppendEntityRecord(spaceName, TEntityRecord{
		Name: fullName, SourceKey: "physical:" + decl.GatewayDeviceID + "!" + decl.Leaf, Identity: identity,
		Provenance: provenance, HasDefinitionOrImport: true,
		ChargeDays: capability.ChargeDays, ChargeSince: capability.ChargeSince, ChargeDateEntityID: dateID,
	})
	administration.AppendEntityRecord(spaceName, TEntityRecord{
		Name: dateName, Identity: extractEntityIdentity(dateName), Provenance: provenance + " (its replacement date)",
		HasDefinitionOrImport: true, InputDatetimeHasDate: true,
	})
	administration.AppendEntityRecord(spaceName, TEntityRecord{
		Name: buttonName, SourceKey: chargeButtonKeyPrefix + decl.GatewayDeviceID + "!" + decl.Leaf, Identity: extractEntityIdentity(buttonName),
		Provenance: provenance + " (sets its replacement date to today)", HasDefinitionOrImport: true,
		ChargeDateEntityID: dateID,
	})
	return nil
}

// generatedSiblingEntityID returns the entity a generated capability of deviceID ("<device-id>!
// <label>") is positioned as -- for a sibling that is no raw discovery leaf (a charge estimate).
func generatedSiblingEntityID(administration *TAdministrationState, deviceID, label string) string {
	var candidates []string
	for entityID, key := range administration.SourceKeyByEntityID {
		if key == "physical:"+deviceID+"!"+label {
			candidates = append(candidates, entityID)
		}
	}
	sort.Strings(candidates)
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

// --- generation ---

// chargeLevelExpression is the level of a charge lasting days, replaced on dateID's date, else on
// since: 100 * (days - D) / days percent, D the days since then, kept between 0 and 100.
func chargeLevelExpression(dateID string, days int, since string) string {
	replaced := fmt.Sprintf("(states('%s') if has_value('%s') else '%s')", dateID, dateID, since)
	elapsed := fmt.Sprintf("(now().date() - strptime(%s, '%%Y-%%m-%%d').date()).days", replaced)
	return fmt.Sprintf("[100, [0, (100 * (%d - %s) / %d) | round(0) | int] | max] | min", days, elapsed, days)
}

// generateChargeEstimates writes every charge estimate's level (a template sensor) and button (a
// template button setting the replacement date to today); generateInputDatetimes writes the date.
func generateChargeEstimates(outputDir string, admin *TAdministrationState) error {
	for _, spaceName := range admin.SpaceOrder {
		for _, rec := range admin.EntityRecordsBySpace[spaceName] {
			if rec.ChargeDateEntityID == "" {
				continue
			}
			id := toHomeAssistantEntityID(rec.Name)
			displayName := friendlyEntityName(rec.Identity)
			var content string
			switch rec.Identity.Domain {
			case "sensor":
				deviceClass, unit, stateClass, icon := resolveCapabilityDefaults(admin.CapabilityDefaults, "sensor", rec.Identity.Path)
				content = buildTemplateSensorYAML(id, displayName, unit, deviceClass, stateClass, icon, chargeLevelExpression(rec.ChargeDateEntityID, rec.ChargeDays, rec.ChargeSince), "")
			case "button":
				// Rooted in the level's frozen unique_id, so it survives renames as the level does
				// (the frozen IDs are read after positioning, so only now).
				levelKey := "physical:" + strings.TrimPrefix(rec.SourceKey, chargeButtonKeyPrefix)
				if levelID, frozen := admin.FrozenUniqueIDs[levelKey]; frozen {
					admin.FrozenUniqueIDs[rec.SourceKey] = levelID + "_" + chargeReplacedSegment
				}
				content = buildChargeReplacedButtonYAML(id, displayName, rec.ChargeDateEntityID)
			default:
				continue
			}
			dir := filepath.Join(outputDir, "entities", "template", rec.Identity.Domain, rec.Identity.Sphere)
			if err := writeYAMLFile(filepath.Join(dir, id+".yaml"), content); err != nil {
				return err
			}
		}
	}
	return nil
}

// buildChargeReplacedButtonYAML is a template button setting dateID to today.
func buildChargeReplacedButtonYAML(id, displayName, dateID string) string {
	var sb strings.Builder
	sb.WriteString(generatorHeader)
	sb.WriteString("button:\n")
	sb.WriteString("- name: " + displayName + "\n")
	sb.WriteString("  unique_id: " + generatedUniqueID(id) + "\n")
	sb.WriteString("  default_entity_id: " + id + "\n")
	sb.WriteString("  icon: mdi:battery-sync\n")
	sb.WriteString("  press:\n")
	sb.WriteString("  - action: input_datetime.set_datetime\n")
	sb.WriteString("    target:\n")
	sb.WriteString("      entity_id: " + dateID + "\n")
	sb.WriteString("    data:\n")
	sb.WriteString("      date: \"{{ now().date() }}\"\n")
	return sb.String()
}
