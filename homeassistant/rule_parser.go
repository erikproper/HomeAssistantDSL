/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: RuleParser
 *
 * The outermost layer of this DSL frontend's three-layer parser architecture (GO_CONVENTIONS.md
 * §7/§8), embedding TDefTokeniser (rule_tokeniser.go). Recursive-descent, bool-returning, CDL1-
 * style grammar for the body of a "rule <spec> with: ...; end;" block (Conceptual.def) -- see
 * rule_character_stream.go's own doc comment for why this exists as real tokenizer-based grammar
 * rather than more ad hoc line/strings.Fields matching.
 *
 * A rule allows exactly one "when" clause, at most one "if" clause, and exactly one "then"
 * clause (ruleClauseTracker, parser.go, enforces this) -- "when"/"if"/"then" is the whole phrase
 * a rule body reads as. No ";" ever separates these clauses from each other (2026-10-15
 * correction, Pascal-style -- a clause ends wherever the next clause's own keyword begins, not
 * at a written ";"; only the final "then" action(s) keep their own ";", since there is no
 * further clause after them). "when <clause> [or <clause>]*" inline-ORs multiple triggers into
 * that one clause; "if <expr>" is a full and/or/not/parenthesised boolean expression (state, or
 * a template check against the firing trigger's own context, or a time-window check); "then
 * <action>;" or "then: <action>; <action>; ...; end;" holds one or more ordered actions
 * (turn/toggle/press/stop/set, each with optional "with <field> <value>" data, plus a bare
 * "delay <duration>" step). "then if ...: ...; else: ...; end;" branching actions are multi-line
 * sub-blocks parsed by parser.go's own outer line scanner (which buffers each sub-block's body
 * text, the same "surgical insertion" shape as the rule body itself), calling back into this
 * file's ActionStatement for their own lines.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 15.10.2026
 *
 */

package main

import (
	"fmt"
	"strconv"
	"strings"
)

// TRuleParser embeds TDefTokeniser (chaining the layers, per GO_CONVENTIONS.md §7). RuleSpec is
// the rule's own already-resolved name (parsed by parser.go's own header-line handling, exactly
// like every other "<spec> with:" block header elsewhere in Conceptual.def), carried here only
// for error messages.
type TRuleParser struct {
	TDefTokeniser
	RuleSpec string
	Warnings []string
}

// newRuleParser creates a TRuleParser for one statement line's worth of source text.
func newRuleParser(source, ruleSpec string) *TRuleParser {
	p := &TRuleParser{RuleSpec: ruleSpec}
	p.Initialise(source)
	return p
}

// --- error reporting ---

// ReportParsingError records a warning against this rule's own body, in the same "[WARNING]
// Conceptual.def: ..." style every other Conceptual.def warning already uses (the caller prints
// p.Warnings with that prefix) -- not bibtex_check's own TInteraction object, matching this
// project's existing warnings-slice convention rather than importing a second one. Returns true
// so it can appear on the right of a CDL1 "||" chain (GO_CONVENTIONS.md §8's own
// ForcedThisKeywordWas exemplar).
func (p *TRuleParser) ReportParsingError(format string, args ...any) bool {
	line, column := p.Position()
	message := fmt.Sprintf(format, args...)
	p.Warnings = append(p.Warnings, fmt.Sprintf("rule %q (L:%d,C:%d): %s", p.RuleSpec, line, column, message))
	return true
}

// Fail always returns false -- the terminal step of a "||" error-recovery chain once
// ReportParsingError has already recorded the warning (this grammar's statements are always one
// line, so there is no enclosing block to skip to, unlike GO_CONVENTIONS.md §8's own
// SkipToEndOrNextBlock exemplar).
func (p *TRuleParser) Fail() bool {
	return false
}

// --- primitive token-matching functions ---

// ForcedThisWordWas requires the current token to be exactly this word, consuming it; records an
// error and fails otherwise.
func (p *TRuleParser) ForcedThisWordWas(word string) bool {
	return p.ThisWordWas(word) ||
		p.ReportParsingError("expected %q", word) && p.Fail()
}

// ForcedThisPunctuationWas requires the current token to be exactly this punctuation byte,
// consuming it; records an error and fails otherwise.
func (p *TRuleParser) ForcedThisPunctuationWas(ch byte) bool {
	return p.ThisPunctuationWas(ch) ||
		p.ReportParsingError("expected %q", string(ch)) && p.Fail()
}

// ForcedThisWord requires the current token to be any word, copying its value into *target;
// records an error and fails otherwise.
func (p *TRuleParser) ForcedThisWord(target *string) bool {
	return p.ThisWord(target) ||
		p.ReportParsingError("expected an identifier") && p.Fail()
}

// ForcedEndOfInput requires nothing but whitespace/EOF remains -- a "when"/"if" clause's own
// terminator is now the next clause's own keyword ("if"/"then"), not a written ";" (2026-10-15,
// the user's own correction: Pascal-style "when <chain> then <action>;" needs no ";" between the
// when/if/then parts, only at the very end of the whole statement). The outer line-joiner
// (parser.go's own rule-body continuation loop) guarantees a when/if clause's own joined line
// never has trailing garbage beyond the clause itself -- this is the belt-and-suspenders check
// that joiner's own "ends with or/and" continuation logic did its job correctly.
func (p *TRuleParser) ForcedEndOfInput() bool {
	return p.AtEndOfTokens() ||
		p.ReportParsingError("unexpected trailing text after the clause") && p.Fail()
}

// --- entity/value grammar ---

// EntityRef parses "<domain>.<sphere>[:[<path>[/<path-segment>]*]]" into *target as the raw spec
// text, reassembled verbatim (e.g. "binary_sensor.social:done_removing"). This grammar function
// only reassembles the raw text; space-context resolution stays exactly where every other
// construct already does it (the caller passes the result through normalizeEntityFullName), so
// a rule's trigger/action entities get the SAME "/"-prefixed absolute-path escape hatch every
// other Conceptual.def entity reference already has, with no new mechanism needed.
func (p *TRuleParser) EntityRef(target *string) bool {
	var domain, sphere string
	if !(p.ForcedThisWord(&domain) && p.ForcedThisPunctuationWas('.') && p.ForcedThisWord(&sphere)) {
		return false
	}
	spec := domain + "." + sphere
	if p.ThisPunctuationWas(':') {
		spec += ":"
		// A path only continues directly from the colon with no whitespace in between --
		// "vacuum.social:" (empty path, e.g. this project's own "vacuum.social: from
		// vacuum.core;" convention, or immediately followed by a predicate keyword like
		// "turns") must not have the next ordinary token mistaken for part of the path just
		// because it happens to be a valid word or a '/'. PrecededByWhitespace
		// (rule_tokeniser.go) is the only way to tell "<sphere>:<path>" apart from "<sphere>:"
		// followed by unrelated grammar -- a real bug, found live 2026-10-13: without this
		// check, "vacuum.social: turns error;" silently swallowed "turns" as the path.
		if !p.ThisToken().PrecededByWhitespace {
			// A leading "/" right after the colon is the absolute-path escape hatch
			// (normalizeEntityFullName, expander.go) -- "ignore the current space context,
			// resolve from the sphere root".
			if p.ThisPunctuationWas('/') {
				spec += "/"
			}
			var path string
			if p.ThisWord(&path) {
				spec += path
				for p.ThisPunctuationIs('/') {
					p.NextToken()
					var segment string
					if !p.ForcedThisWord(&segment) {
						return false
					}
					spec += "/" + segment
				}
			}
		}
	}
	*target = spec
	return true
}

// TurnState parses "on" or "off" into *target.
func (p *TRuleParser) TurnState(target *string) bool {
	if p.ThisWordWas("on") {
		*target = "on"
		return true
	}
	if p.ThisWordWas("off") {
		*target = "off"
		return true
	}
	return false
}

// Duration parses "<HH>:<MM>:<SS>" into *target as that exact string -- the same literal format
// Logical.def's own "delay_on"/"delay_off" durations already use elsewhere
// (Physical_DerivedCapability.go); generateRuleAutomations splits it into hours/minutes/seconds
// fields when building YAML.
func (p *TRuleParser) Duration(target *string) bool {
	var h, m, s string
	if !(p.ForcedThisWord(&h) && p.ForcedThisPunctuationWas(':') && p.ForcedThisWord(&m) && p.ForcedThisPunctuationWas(':') && p.ForcedThisWord(&s)) {
		return false
	}
	*target = h + ":" + m + ":" + s
	return true
}

// Value parses either a Jinja-template string ("jinja \"...\"" -- the same raw pass-through
// convention Logical.def's own "defined"/"derived" capabilities already use,
// Physical_DerivedCapability.go) or a plain word (a literal number/token) into *value, setting
// *isJinja accordingly.
func (p *TRuleParser) Value(value *string, isJinja *bool) bool {
	if p.ThisWordWas("jinja") {
		*isJinja = true
		return p.ThisString(value) ||
			p.ReportParsingError("expected a quoted jinja template") && p.Fail()
	}
	*isJinja = false
	return p.ForcedThisWord(value)
}

// DataFields parses "<field> <value>[, <field> <value>]*" into *fields. A leading "default"
// before a field name is optional and purely cosmetic (discarded) -- "with default brightness
// 70%" reads better than "with brightness 70" for the HA light-domain concept of "the brightness
// used when switching from off to on" (confirmed with the user 2026-10-15: not a second kind of
// data field, the DSL-level "brightness" field name IS "brightness_pct", renamed here for the
// same reason). A trailing "%" on any value is likewise optional and cosmetic (HA's own
// brightness_pct/brightness_step_pct fields are already plain percentages, so it adds nothing a
// bare number didn't already mean -- just reads better for a human author).
func (p *TRuleParser) DataFields(fields *[]TRuleDataField) bool {
	for {
		var field TRuleDataField
		p.ThisWordWas("default")
		if !p.ForcedThisWord(&field.Name) {
			return false
		}
		if field.Name == "brightness" {
			field.Name = "brightness_pct"
		}
		if !p.Value(&field.Value, &field.IsJinja) {
			return false
		}
		if !field.IsJinja {
			p.ThisPunctuationWas('%')
		}
		*fields = append(*fields, field)
		if !p.ThisPunctuationWas(',') {
			return true
		}
	}
}

// --- "Xety" (optional) variants ---

// DataFieldsEty parses an optional "with <field> <value>[, <field> <value>]*" clause into
// *fields.
func (p *TRuleParser) DataFieldsEty(fields *[]TRuleDataField) bool {
	if !p.ThisWordWas("with") {
		return true
	}
	return p.DataFields(fields)
}

// ForDurationEty parses an optional "for <duration>" clause into *target.
func (p *TRuleParser) ForDurationEty(target *string) bool {
	if !p.ThisWordWas("for") {
		return true
	}
	return p.Duration(target)
}

// --- trigger grammar ---

// WhenStatement parses one "when <clause> [or <clause>]*" statement into *triggers -- one or
// more trigger clauses, inline-OR'd (PROJECT.md item 13, 2026-10-13: lets several triggers that
// share one action read as a single statement instead of repeating "when" once per clause; HA's
// own trigger semantics are already "any one firing runs the automation" regardless of how many
// separate list entries that becomes, so this is a pure readability surface, not a new
// mechanism). Each clause is one of: "time is <HH:MM[:SS]>" (a daily time-of-day trigger, Kind
// "time"), "every minute" (the one real polling-trigger case, Kind "time_pattern"), or an
// entity-anchored clause -- "<entity-ref> turns <state> [for <duration>]" (Kind "state"),
// "<entity-ref> changes [for <duration>]" (Kind "state" with an empty State, i.e. ANY transition
// -- the real "entity_id: X, trigger: state" form with no "to:" at all), or "<entity-ref> is
// <event-type>" (Kind "event"). No trailing ";" -- the clause ends wherever the next keyword
// ("if"/"then") begins (2026-10-15 correction); ForcedEndOfInput just confirms the outer
// line-joiner handed this parser exactly the clause and nothing past it.
func (p *TRuleParser) WhenStatement(triggers *[]TRuleTrigger) bool {
	if !p.ThisWordWas("when") {
		return false
	}
	for {
		var trigger TRuleTrigger
		if !p.TriggerClause(&trigger) {
			return false
		}
		*triggers = append(*triggers, trigger)
		if !p.ThisWordWas("or") {
			break
		}
	}
	return p.ForcedEndOfInput()
}

// TriggerClause parses one trigger clause (no leading "when"/"or", no trailing ";"/"or" --
// WhenStatement owns the statement's own punctuation and OR-chaining).
func (p *TRuleParser) TriggerClause(trigger *TRuleTrigger) bool {
	return p.TimeOfDayClause(trigger) || p.EveryMinuteClause(trigger) || p.EntityAnchoredClause(trigger)
}

// TimeOfDayClause parses "time is <HH:MM[:SS]>" into *trigger (Kind "time"; EventType doubles as
// the time-of-day string here, same field the "is <event-type>" clause uses for Kind "event").
func (p *TRuleParser) TimeOfDayClause(trigger *TRuleTrigger) bool {
	if !p.ThisWordWas("time") {
		return false
	}
	trigger.Kind = "time"
	return p.ForcedThisWordWas("is") && p.TimeOfDay(&trigger.EventType)
}

// EveryMinuteClause parses "every minute" into *trigger (Kind "time_pattern") -- the one real
// polling-trigger case among the manual automations being migrated (PROJECT.md item 13), same
// "time_pattern, minutes: /1" shape generateNoMotionAutomations already uses elsewhere.
func (p *TRuleParser) EveryMinuteClause(trigger *TRuleTrigger) bool {
	if !p.ThisWordWas("every") {
		return false
	}
	trigger.Kind = "time_pattern"
	return p.ForcedThisWordWas("minute")
}

// EntityAnchoredClause parses an entity-ref followed by "turns"/"changes"/"is".
func (p *TRuleParser) EntityAnchoredClause(trigger *TRuleTrigger) bool {
	if !(p.EntityRef(&trigger.EntitySpec) &&
		(p.StateClause(trigger) || p.ChangesClause(trigger) || p.EventClause(trigger) ||
			p.ReportParsingError("expected \"turns\", \"changes\", or \"is\"") && p.Fail())) {
		return false
	}
	return resolveNamedTriggerReference(trigger, p)
}

// resolveNamedTriggerReference reinterprets a "trigger.<sphere>:<path> is triggered" clause
// (2026-10-15, superseding the earlier "[a] <sphere>:<path> is triggered" form) -- EntityRef
// above already parses "trigger.social:main/light/toggle_request" as an ordinary entity-ref
// (domain "trigger"), and EventClause already parses "is triggered" as an ordinary event-type
// match, so no separate grammar branch is needed at all: this just notices the domain was
// "trigger" and swaps Kind "event"/EntitySpec for "named_trigger"/Name, naming a device-level
// "trigger <name> when ...;" declaration (Conceptual_NamedTriggerEntities.go) rather than a real
// HA entity -- see TAdministrationState.NamedTriggers' own doc comment (administration.go). The
// "trigger." prefix deliberately echoes "trigger.<attr>" (ConditionTemplateClause's own template
// condition against the firing trigger's context) -- conceptually the same idea, just at a
// different level: a named trigger IS a derived trigger, built from the conjunction of every
// device's own "trigger <name> when ...;" declaration for that name.
func resolveNamedTriggerReference(trigger *TRuleTrigger, p *TRuleParser) bool {
	// "event.<name> is raised" (2026-10-22) is the same reference in the notation matching the
	// declaration "aggregated event.<name> is raised when ...;" (Conceptual_NamedTriggerEntities.go).
	if name, isEvent := strings.CutPrefix(trigger.EntitySpec, "event."); isEvent && trigger.Kind == "event" && trigger.EventType == "raised" {
		trigger.Kind = "named_trigger"
		trigger.EntitySpec = ""
		trigger.EventType = ""
		trigger.Name = name
		return true
	}
	name, ok := strings.CutPrefix(trigger.EntitySpec, "trigger.")
	if !ok {
		return true
	}
	if trigger.Kind != "event" || trigger.EventType != "triggered" {
		return p.ReportParsingError("a named trigger reference must read \"trigger.<name> is triggered\"") && p.Fail()
	}
	trigger.Kind = "named_trigger"
	trigger.EntitySpec = ""
	trigger.EventType = ""
	trigger.Name = name
	return true
}

// StateClause parses "turns <state> [for <duration>]" into *trigger (Kind "state"). <state> is
// any word, not just "on"/"off" -- a trigger describes an observed HA state VALUE, which spans
// far more than binary domains (e.g. a vacuum's own "error"/"cleaning"/"docked" states, the real
// case this generality is for), unlike a "turn" ACTION (TurnAction below), which really is
// restricted to on/off since it maps straight onto HA's own turn_on/turn_off services.
func (p *TRuleParser) StateClause(trigger *TRuleTrigger) bool {
	if !p.ThisWordWas("turns") {
		return false
	}
	trigger.Kind = "state"
	return p.ForcedThisWord(&trigger.State) && p.ForDurationEty(&trigger.For)
}

// ChangesClause parses "changes [for <duration>]" into *trigger (Kind "state", State "" -- the
// generator omits the YAML "to:" line for this, meaning "any transition").
func (p *TRuleParser) ChangesClause(trigger *TRuleTrigger) bool {
	if !p.ThisWordWas("changes") {
		return false
	}
	trigger.Kind = "state"
	trigger.State = ""
	return p.ForDurationEty(&trigger.For)
}

// EventClause parses "is <event-type>" into *trigger (Kind "event").
func (p *TRuleParser) EventClause(trigger *TRuleTrigger) bool {
	if !p.ThisWordWas("is") {
		return false
	}
	trigger.Kind = "event"
	return p.ForcedThisWord(&trigger.EventType)
}

// TimeOfDay parses "<HH>:<MM>[:<SS>]" into *target as that exact string.
func (p *TRuleParser) TimeOfDay(target *string) bool {
	var hours, minutes string
	if !(p.ForcedThisWord(&hours) && p.ForcedThisPunctuationWas(':') && p.ForcedThisWord(&minutes)) {
		return false
	}
	*target = hours + ":" + minutes
	if p.ThisPunctuationWas(':') {
		var seconds string
		if !p.ForcedThisWord(&seconds) {
			return false
		}
		*target += ":" + seconds
	}
	return true
}

// --- "if" clause grammar (the rule's own single gating condition) ---

// IfStatement parses one "if <expr>" clause into *cond -- a rule's own single, optional gating
// condition (PROJECT.md item 13, 2026-10-13: a rule allows at most one; "when"/"if"/"then" is
// the whole phrase a rule now reads as). <expr> is a full boolean expression over simple clauses
// (state/template/time checks) with "and"/"or"/"not" composition and parenthesised grouping,
// exactly like any other boolean expression -- standard precedence (not binds tightest, then
// and, then or) -- mirroring HA's own nested "condition: and/or/not" + "conditions: [...]" YAML
// shape directly. Replaces the earlier, narrower design (a flat AND-implied list of top-level
// "condition ...;" statements, plus a separate "condition or:"/"condition and:" block for actual
// composition) -- the user's own correction, the same day that design shipped: multiple
// conditions that need to combine belong INSIDE the one allowed clause, not spread across a
// second top-level statement or a bespoke block construct. No trailing ";" -- same 2026-10-15
// correction as WhenStatement: the clause ends wherever "then" begins.
func (p *TRuleParser) IfStatement(cond *TRuleCondition) bool {
	return p.ThisWordWas("if") &&
		p.ConditionOrExpr(cond) &&
		p.ForcedEndOfInput()
}

// ConditionOrExpr parses "<and-expr> (or <and-expr>)*" into *cond -- Kind "or" with 2+ Operands
// if there's more than one, otherwise just the single and-expr, unwrapped.
func (p *TRuleParser) ConditionOrExpr(cond *TRuleCondition) bool {
	var first TRuleCondition
	if !p.ConditionAndExpr(&first) {
		return false
	}
	operands := []TRuleCondition{first}
	for p.ThisWordWas("or") {
		var next TRuleCondition
		if !p.ConditionAndExpr(&next) {
			return false
		}
		operands = append(operands, next)
	}
	if len(operands) == 1 {
		*cond = operands[0]
		return true
	}
	cond.Kind = "or"
	cond.Operands = operands
	return true
}

// ConditionAndExpr parses "<not-expr> (and <not-expr>)*" into *cond -- Kind "and" with 2+
// Operands if there's more than one, otherwise just the single not-expr, unwrapped.
func (p *TRuleParser) ConditionAndExpr(cond *TRuleCondition) bool {
	var first TRuleCondition
	if !p.ConditionNotExpr(&first) {
		return false
	}
	operands := []TRuleCondition{first}
	for p.ThisWordWas("and") {
		var next TRuleCondition
		if !p.ConditionNotExpr(&next) {
			return false
		}
		operands = append(operands, next)
	}
	if len(operands) == 1 {
		*cond = operands[0]
		return true
	}
	cond.Kind = "and"
	cond.Operands = operands
	return true
}

// ConditionNotExpr parses "not <not-expr>" (Kind "not", exactly one Operand) or a bare atom into
// *cond.
func (p *TRuleParser) ConditionNotExpr(cond *TRuleCondition) bool {
	if p.ThisWordWas("not") {
		var inner TRuleCondition
		if !p.ConditionNotExpr(&inner) {
			return false
		}
		cond.Kind = "not"
		cond.Operands = []TRuleCondition{inner}
		return true
	}
	return p.ConditionAtom(cond)
}

// ConditionAtom parses "( <or-expr> )" or a simple clause into *cond.
func (p *TRuleParser) ConditionAtom(cond *TRuleCondition) bool {
	if p.ThisPunctuationWas('(') {
		return p.ConditionOrExpr(cond) && p.ForcedThisPunctuationWas(')')
	}
	return p.TemplateConditionClause(cond) || p.TimeConditionClause(cond) || p.StateConditionClause(cond) ||
		p.ReportParsingError("expected a condition") && p.Fail()
}

// TimeConditionClause parses "time [after <HH:MM[:SS]>] [before <HH:MM[:SS]>] [weekday <day>[,
// <day>]*]" into *cond (Kind "time") -- at least one of "after"/"before"/"weekday" must be
// present.
func (p *TRuleParser) TimeConditionClause(cond *TRuleCondition) bool {
	if !p.ThisWordWas("time") {
		return false
	}
	cond.Kind = "time"
	if p.ThisWordWas("after") {
		if !p.TimeOfDay(&cond.After) {
			return false
		}
	}
	if p.ThisWordWas("before") {
		if !p.TimeOfDay(&cond.Before) {
			return false
		}
	}
	if p.ThisWordWas("weekday") {
		if !p.WeekdayList(&cond.Weekdays) {
			return false
		}
	}
	if cond.After == "" && cond.Before == "" && len(cond.Weekdays) == 0 {
		return p.ReportParsingError("expected \"after\", \"before\", and/or \"weekday\"") && p.Fail()
	}
	return true
}

// WeekdayList parses "<day>[, <day>]*" (e.g. "sat, sun") into *days.
func (p *TRuleParser) WeekdayList(days *[]string) bool {
	for {
		var day string
		if !p.ForcedThisWord(&day) {
			return false
		}
		*days = append(*days, day)
		if !p.ThisPunctuationWas(',') {
			return true
		}
	}
}

// TemplateConditionClause parses "trigger.<attr> is \"<value>\"" into *cond (Kind "template").
func (p *TRuleParser) TemplateConditionClause(cond *TRuleCondition) bool {
	if !p.ThisWordWas("trigger") {
		return false
	}
	cond.Kind = "template"
	return p.ForcedThisPunctuationWas('.') &&
		p.ForcedThisWord(&cond.TriggerAttr) &&
		p.ForcedThisWordWas("is") &&
		(p.ThisString(&cond.Value) || p.ReportParsingError("expected a quoted value") && p.Fail())
}

// StateConditionClause parses "<entity-ref> is <state> [for <duration>]" into *cond (Kind
// "state") -- the optional "for" debounces the check against how long the entity has already
// held that state, same duration syntax as a trigger's own "for" (ForDurationEty).
func (p *TRuleParser) StateConditionClause(cond *TRuleCondition) bool {
	cond.Kind = "state"
	return p.EntityRef(&cond.EntitySpec) &&
		p.ForcedThisWordWas("is") &&
		p.ForcedThisWord(&cond.State) &&
		p.ForDurationEty(&cond.For)
}

// --- action grammar ---

// ThenStatement parses one top-level "then <action>;" statement into *action.
func (p *TRuleParser) ThenStatement(action *TRuleAction) bool {
	return p.ThisWordWas("then") && p.ActionStatement(action)
}

// ActionStatement parses one action (without a leading "then" -- used both for a rule body's own
// "then <action>;" lines, via ThenStatement above, and for each bare action line inside a "then
// if ...: ...; else: ...; end;" branch, parser.go's own parseIfActionBlock) into *action.
func (p *TRuleParser) ActionStatement(action *TRuleAction) bool {
	return p.TurnAction(action) ||
		p.ToggleAction(action) ||
		p.DimAction(action) ||
		p.BrightenAction(action) ||
		p.PressAction(action) ||
		p.StopAction(action) ||
		p.CloseAction(action) ||
		p.OpenAction(action) ||
		p.SetAction(action) ||
		p.DelayAction(action) ||
		p.StartAction(action) ||
		p.PauseAction(action) ||
		p.ReturnToBaseAction(action) ||
		p.ReportParsingError("expected an action verb (turn/toggle/dim/brighten/press/stop/close/open/set/delay/start/pause/return_to_base)") && p.Fail()
}

// TurnAction parses "turn <entity-ref> on|off [with <field> <value>[, <field> <value>]*];".
func (p *TRuleParser) TurnAction(action *TRuleAction) bool {
	if !p.ThisWordWas("turn") {
		return false
	}
	action.Verb = "turn"
	return p.EntityRef(&action.EntitySpec) &&
		(p.TurnState(&action.State) || p.ReportParsingError("expected \"on\" or \"off\"") && p.Fail()) &&
		p.DataFieldsEty(&action.Data) &&
		p.ForcedThisPunctuationWas(';')
}

// ToggleAction parses "toggle <entity-ref> [with <field> <value>[, <field> <value>]*];".
func (p *TRuleParser) ToggleAction(action *TRuleAction) bool {
	if !p.ThisWordWas("toggle") {
		return false
	}
	action.Verb = "toggle"
	return p.EntityRef(&action.EntitySpec) &&
		p.DataFieldsEty(&action.Data) &&
		p.ForcedThisPunctuationWas(';')
}

// DimAction parses "dim <entity-ref> with <N>%;" -- sugar for "turn <entity-ref> on with
// brightness_step_pct -<N>;", avoiding a bare negative number (which reads as the raw HA
// "brightness_step_pct" field, not a DSL concept of its own) in rule text. BrightenAction below
// is the positive-direction counterpart (confirmed with the user 2026-10-15).
func (p *TRuleParser) DimAction(action *TRuleAction) bool {
	return p.brightnessStepAction(action, "dim", -1)
}

// BrightenAction parses "brighten <entity-ref> with <N>%;" -- DimAction's own doc comment.
func (p *TRuleParser) BrightenAction(action *TRuleAction) bool {
	return p.brightnessStepAction(action, "brighten", 1)
}

// brightnessStepAction is DimAction/BrightenAction's shared body -- verb is "dim"/"brighten",
// sign is -1/1. Produces an ordinary Verb "turn"/State "on" action with one "brightness_step_pct"
// Data field, so generator.go's existing "turn" rendering needs no changes at all for this sugar.
func (p *TRuleParser) brightnessStepAction(action *TRuleAction, verb string, sign int) bool {
	if !p.ThisWordWas(verb) {
		return false
	}
	action.Verb = "turn"
	action.State = "on"
	if !(p.EntityRef(&action.EntitySpec) && p.ForcedThisWordWas("with")) {
		return false
	}
	var magnitude string
	if !p.ForcedThisWord(&magnitude) {
		return false
	}
	p.ThisPunctuationWas('%')
	n, err := strconv.Atoi(magnitude)
	if err != nil {
		return p.ReportParsingError("expected a plain number, got %q", magnitude) && p.Fail()
	}
	action.Data = append(action.Data, TRuleDataField{Name: "brightness_step_pct", Value: strconv.Itoa(sign * n)})
	return p.ForcedThisPunctuationWas(';')
}

// PressAction parses "press <entity-ref>;".
func (p *TRuleParser) PressAction(action *TRuleAction) bool {
	if !p.ThisWordWas("press") {
		return false
	}
	action.Verb = "press"
	return p.EntityRef(&action.EntitySpec) && p.ForcedThisPunctuationWas(';')
}

// StopAction parses "stop <entity-ref>;".
func (p *TRuleParser) StopAction(action *TRuleAction) bool {
	if !p.ThisWordWas("stop") {
		return false
	}
	action.Verb = "stop"
	return p.EntityRef(&action.EntitySpec) && p.ForcedThisPunctuationWas(';')
}

// CloseAction parses "close <entity-ref>;" -- "cover.close_cover".
func (p *TRuleParser) CloseAction(action *TRuleAction) bool {
	if !p.ThisWordWas("close") {
		return false
	}
	action.Verb = "close"
	return p.EntityRef(&action.EntitySpec) && p.ForcedThisPunctuationWas(';')
}

// OpenAction parses "open <entity-ref>;" -- "cover.open_cover".
func (p *TRuleParser) OpenAction(action *TRuleAction) bool {
	if !p.ThisWordWas("open") {
		return false
	}
	action.Verb = "open"
	return p.EntityRef(&action.EntitySpec) && p.ForcedThisPunctuationWas(';')
}

// SetAction parses "set <entity-ref> to <value>|jinja \"...\";".
func (p *TRuleParser) SetAction(action *TRuleAction) bool {
	if !p.ThisWordWas("set") {
		return false
	}
	action.Verb = "set"
	return p.EntityRef(&action.EntitySpec) &&
		p.ForcedThisWordWas("to") &&
		p.Value(&action.Value, &action.ValueIsJinja) &&
		p.ForcedThisPunctuationWas(';')
}

// DelayAction parses "delay <duration>;".
func (p *TRuleParser) DelayAction(action *TRuleAction) bool {
	if !p.ThisWordWas("delay") {
		return false
	}
	action.Verb = "delay"
	return p.Duration(&action.Duration) && p.ForcedThisPunctuationWas(';')
}

// StartAction parses "start <entity-ref>;" -- e.g. "vacuum.start", the real vacuum-automation
// case; maps to "<domain>.start" generically, not vacuum-specific in itself.
func (p *TRuleParser) StartAction(action *TRuleAction) bool {
	if !p.ThisWordWas("start") {
		return false
	}
	action.Verb = "start"
	return p.EntityRef(&action.EntitySpec) && p.ForcedThisPunctuationWas(';')
}

// PauseAction parses "pause <entity-ref>;" -- maps to "<domain>.pause" generically.
func (p *TRuleParser) PauseAction(action *TRuleAction) bool {
	if !p.ThisWordWas("pause") {
		return false
	}
	action.Verb = "pause"
	return p.EntityRef(&action.EntitySpec) && p.ForcedThisPunctuationWas(';')
}

// ReturnToBaseAction parses "return_to_base <entity-ref>;" -- vacuum-specific, no generic
// cross-domain equivalent (the only real case among the manual automations being migrated).
func (p *TRuleParser) ReturnToBaseAction(action *TRuleAction) bool {
	if !p.ThisWordWas("return_to_base") {
		return false
	}
	action.Verb = "return_to_base"
	return p.EntityRef(&action.EntitySpec) && p.ForcedThisPunctuationWas(';')
}

// --- if-action header (parser.go's own parseIfActionBlock parses the rest of the block) ---

// IfActionHeader parses "then if <entity-ref> is <state>" (the ":" at the end of the real source
// line is stripped by the caller before constructing this parser, since it marks a multi-line
// block boundary the OUTER line scanner already handled -- see parser.go's own
// parseIfActionBlock) into *action's CondEntitySpec/CondState.
func (p *TRuleParser) IfActionHeader(action *TRuleAction) bool {
	action.Verb = "if"
	return p.ThisWordWas("then") &&
		p.ThisWordWas("if") &&
		p.EntityRef(&action.CondEntitySpec) &&
		p.ForcedThisWordWas("is") &&
		p.ForcedThisWord(&action.CondState)
}
