/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ParserBatteryEmulation
 *
 * The grammar of Physical.def's battery level emulation statements (battery_emulation.go):
 *
 *   BatteryEmulation: "battery" "level" "emulation" ( "on" Word ";" | EmulationDetails )
 *   EmulationDetails: AssignedIDety Lastingety Sinceety "with" "alert" "at" Number "%" ";"
 *   AssignedIDety:    ( "as" "unique_id" Word ( "." Word )* )?
 *   Lastingety:       ( "lasting" Number "days" )?
 *   Sinceety:         ( "since" Word "." Word "." Word )?
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Vienna, Austria, in collaboration with Claude.ai
 *
 * Version of: 07.11.2026
 *
 */

package main

import (
	"math"
	"time"
)

// TBatteryEmulationParser embeds TDefParser.
type TBatteryEmulationParser struct {
	TDefParser // the statement being parsed
	Instance   string
	Emulation  TBatteryEmulation
}

// --- top level ---

// parseBatteryEmulationLine parses line as an emulation statement: instance is set for "... on
// <instance>;", else emulation. ok is false when it isn't one; warnings say what is wrong with one
// that is.
func parseBatteryEmulationLine(line string) (instance string, emulation TBatteryEmulation, ok bool, warnings []string) {
	p := &TBatteryEmulationParser{Emulation: TBatteryEmulation{Days: batteryEmulationDefaultDays}}
	p.Initialise(line, OriginBatteryEmulation)
	ok = p.BatteryEmulation()
	return p.Instance, p.Emulation, ok, p.Warnings
}

// --- grammar ---

// BatteryEmulation: "battery" "level" "emulation" ( "on" Word ";" | EmulationDetails )
func (p *TBatteryEmulationParser) BatteryEmulation() bool {
	return p.ThisWordWas("battery") &&
		/**/ p.ForcedThisWordWas("level") &&
		/**/ p.ForcedThisWordWas("emulation") &&
		/*  */ (p.ThisWordWas("on") &&
			/*    */ p.ForcedThisWord(&p.Instance, MessageEmulatingInstance) &&
			/*    */ p.ForcedThisPunctuationWas(';') &&
			/*    */ p.ForcedEndOfStatement() ||
			/*  */ p.EmulationDetails())
}

// EmulationDetails: AssignedIDety Lastingety Sinceety "with" "alert" "at" Number "%" ";"
func (p *TBatteryEmulationParser) EmulationDetails() bool {
	return p.AssignedIDety() &&
		/**/ p.Lastingety() &&
		/**/ p.Sinceety() &&
		/**/ p.ForcedThisWordWas("with") &&
		/**/ p.ForcedThisWordWas("alert") &&
		/**/ p.ForcedThisWordWas("at") &&
		/**/ p.ForcedNumber(&p.Emulation.AlertPercent, 1, 99, MessageAlertPercentage) &&
		/**/ p.ForcedThisPunctuationWas('%') &&
		/**/ p.ForcedThisPunctuationWas(';') &&
		/**/ p.ForcedEndOfStatement()
}

// AssignedIDety: ( "as" "unique_id" Word RootSegmentsety )?
func (p *TBatteryEmulationParser) AssignedIDety() bool {
	return p.ThisWordWas("as") &&
		/**/ p.ForcedThisWordWas("unique_id") &&
		/**/ p.ForcedThisWord(&p.Emulation.RootID, MessageFrozenRoot) &&
		/**/ p.RootSegmentsety() ||
		true
}

// RootSegmentsety: ( "." Word )*
func (p *TBatteryEmulationParser) RootSegmentsety() bool {
	var segment string
	for p.ThisPunctuationWas('.') &&
		/**/ p.ForcedThisWord(&segment, MessageFrozenRoot) {
		p.Emulation.RootID += "." + segment
	}
	return true
}

// Lastingety: ( "lasting" Number "days" )?
func (p *TBatteryEmulationParser) Lastingety() bool {
	return p.ThisWordWas("lasting") &&
		/**/ p.ForcedNumber(&p.Emulation.Days, 1, math.MaxInt, MessageDaysACharge) &&
		/**/ p.ForcedThisWordWas("days") ||
		true
}

// Sinceety: ( "since" Word "." Word "." Word )?, a date that exists (dd.mm.yyyy).
func (p *TBatteryEmulationParser) Sinceety() bool {
	var day, month, year string
	return p.ThisWordWas("since") &&
		/**/ p.ForcedThisWord(&day, MessageDate) &&
		/**/ p.ForcedThisPunctuationWas('.') &&
		/**/ p.ForcedThisWord(&month, MessageDate) &&
		/**/ p.ForcedThisPunctuationWas('.') &&
		/**/ p.ForcedThisWord(&year, MessageDate) &&
		/**/ p.SinceDate(day+"."+month+"."+year) ||
		true
}

// --- small helpers ---

// SinceDate stores text, a dd.mm.yyyy date, as the emulation's starting date.
func (p *TBatteryEmulationParser) SinceDate(text string) bool {
	date, err := time.Parse("2.1.2006", text)
	switch {
	case err != nil:
		return p.Failed(ErrorNoDate, text)
	default:
		p.Emulation.Since = date.Format("2006-01-02")
		return true
	}
}
