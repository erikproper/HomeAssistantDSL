/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ParserRule
 *
 * The grammar of the statements in a Conceptual.def "rule <name> with: ... end;" block. parser.go
 * hands it one statement at a time: a "when" clause, an "if" clause, a "then" action, an action
 * inside a "then: ... end;" block, or the header of a "then if ...:" block or of one of its
 * "else if ...:" branches. A clause ends where the next one begins, so "when" and "if" have no
 * ";"; an action does.
 *
 *   WhenStatement:     "when" TriggerClause ( "or" TriggerClause )*
 *   TriggerClause:     "time" "is" TimeOfDay | "every" "minute"
 *                      | EntityRef ( "turns" Word ForDurationety | "changes" ForDurationety
 *                                  | "is" Word )
 *   IfStatement:       "if" ConditionOrExpr
 *   ConditionOrExpr:   ConditionAndExpr ( "or" ConditionAndExpr )*
 *   ConditionAndExpr:  ConditionNotExpr ( "and" ConditionNotExpr )*
 *   ConditionNotExpr:  "not" ConditionNotExpr | ConditionAtom
 *   ConditionAtom:     "(" ConditionOrExpr ")" | "trigger" "." Word "is" String
 *                      | "time" Afterety Beforeety Weekdaysety | EntityRef "is" Word ForDurationety
 *   ThenStatement:     "then" ActionStatement
 *   ActionStatement:   "turn" EntityRef ( "on" | "off" ) DataFieldsety ";"
 *                      | "toggle" EntityRef DataFieldsety ";"
 *                      | ( "dim" | "brighten" ) EntityRef "with" Number "%"? ";"
 *                      | ( "press" | "stop" | "close" | "open" | "start" | "pause"
 *                        | "return_to_base" ) EntityRef ";"
 *                      | "set" EntityRef "to" Value ";" | "delay" Duration ";"
 *   IfActionHeader:    "then" "if" ConditionOrExpr
 *   ElseIfHeader:      "else" "if" ConditionOrExpr
 *   EntityRef:         Word "." Word ( ":" ( "/"? Word ( "/" Word )* )? )?
 *   DataFieldsety:     ( "with" DataField ( "," DataField )* )?
 *   DataField:         "default"? Word Value "%"?
 *   Value:             "jinja" String | Word
 *
 * The path of an EntityRef is written right after its ":"; "vacuum.social: turns ..." has an
 * empty path. "dim"/"brighten" turn a light on with a step in brightness that never goes below
 * dimBrightnessFloorPct, so that dimming never switches it off.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 07.11.2026
 *
 */

package main

import (
	"fmt"
	"strconv"
	"strings"
)

// TRuleParser embeds TDefParser.
type TRuleParser struct {
	TDefParser // the statement being parsed
}

// brightnessBoundedStepField is the Data field "dim"/"brighten" record their signed step in;
// dimBrightnessFloorPct is the lowest brightness either ever sets.
const (
	brightnessBoundedStepField = "brightness_bounded_step_pct"
	dimBrightnessFloorPct      = 5
)

// simpleActionVerbs are the verbs that take only an entity.
var simpleActionVerbs = map[string]bool{
	"press": true, "stop": true, "close": true, "open": true, "start": true, "pause": true, "return_to_base": true,
}

// newRuleParser returns a parser for one statement of rule ruleName.
func newRuleParser(source, ruleName string) *TRuleParser {
	p := &TRuleParser{}
	p.Initialise(source, fmt.Sprintf(OriginRule, ruleName))
	return p
}

// --- small helpers ---

// SetText sets *target to text.
func (p *TRuleParser) SetText(target *string, text string) bool {
	*target = text
	return true
}

// AppendText appends text to *target.
func (p *TRuleParser) AppendText(target *string, text string) bool {
	*target += text
	return true
}

// SetTriggerKind sets the kind of trigger.
func (p *TRuleParser) SetTriggerKind(trigger *TRuleTrigger, kind string) bool {
	trigger.Kind = kind
	return true
}

// AppendTrigger appends trigger to *triggers.
func (p *TRuleParser) AppendTrigger(triggers *[]TRuleTrigger, trigger TRuleTrigger) bool {
	*triggers = append(*triggers, trigger)
	return true
}

// NamedTriggerReference turns "event.<name> is raised" into a reference to the request <name>
// (TAdministrationState.NamedTriggers).
func (p *TRuleParser) NamedTriggerReference(trigger *TRuleTrigger) bool {
	if name, isEvent := strings.CutPrefix(trigger.EntitySpec, "event."); isEvent && trigger.Kind == "event" && trigger.EventType == "raised" {
		*trigger = TRuleTrigger{Kind: "named_trigger", Name: name}
	}
	return true
}

// SetConditionKind sets the kind of cond.
func (p *TRuleParser) SetConditionKind(cond *TRuleCondition, kind string) bool {
	cond.Kind = kind
	return true
}

// AppendCondition appends cond to *operands.
func (p *TRuleParser) AppendCondition(operands *[]TRuleCondition, cond TRuleCondition) bool {
	*operands = append(*operands, cond)
	return true
}

// CombinedCondition sets *cond to operands combined by kind ("and"/"or"), or to the one operand.
func (p *TRuleParser) CombinedCondition(cond *TRuleCondition, kind string, operands []TRuleCondition) bool {
	switch {
	case len(operands) == 1:
		*cond = operands[0]
	default:
		cond.Kind = kind
		cond.Operands = operands
	}
	return true
}

// NegatedCondition sets *cond to "not inner".
func (p *TRuleParser) NegatedCondition(cond *TRuleCondition, inner TRuleCondition) bool {
	cond.Kind = "not"
	cond.Operands = []TRuleCondition{inner}
	return true
}

// TimeConditionGiven requires at least one of "after", "before" and "weekday".
func (p *TRuleParser) TimeConditionGiven(cond *TRuleCondition) bool {
	return cond.After != "" || cond.Before != "" || len(cond.Weekdays) > 0 ||
		p.Failed(ErrorExpectedGot, MessageTimeCondition, p.ThisTokenText())
}

// SetVerb sets the verb of action.
func (p *TRuleParser) SetVerb(action *TRuleAction, verb string) bool {
	action.Verb = verb
	return true
}

// AppendDataField appends field to *fields; "brightness" is Home Assistant's "brightness_pct".
func (p *TRuleParser) AppendDataField(fields *[]TRuleDataField, field TRuleDataField) bool {
	if field.Name == "brightness" {
		field.Name = "brightness_pct"
	}
	*fields = append(*fields, field)
	return true
}

// AppendBrightnessStep makes action turn its light on with a bounded step of step percent.
func (p *TRuleParser) AppendBrightnessStep(action *TRuleAction, step int) bool {
	action.Verb = "turn"
	action.State = "on"
	action.Data = append(action.Data, TRuleDataField{Name: brightnessBoundedStepField, Value: strconv.Itoa(step)})
	return true
}

// --- entities and values ---

// EntityRef: Word "." Word ( ":" ( "/"? Word ( "/" Word )* )? )?, into *target as written. The
// caller resolves it against the space (normalizeEntityFullName).
func (p *TRuleParser) EntityRef(target *string) bool {
	var domain, sphere string
	return p.ForcedThisWord(&domain, MessageDomain) &&
		/**/ p.ForcedThisPunctuationWas('.') &&
		/**/ p.ForcedThisWord(&sphere, MessageSphere) &&
		/**/ p.SetText(target, domain+"."+sphere) &&
		/**/ p.EntityPathety(target)
}

// EntityPathety: ( ":" AdjacentPathety )?
func (p *TRuleParser) EntityPathety(target *string) bool {
	return p.ThisPunctuationWas(':') &&
		/**/ p.AppendText(target, ":") &&
		/**/ p.AdjacentPathety(target) ||
		true
}

// AdjacentPathety: ( "/"? Word PathSegmentsety )?, written right after the ":". A "/" makes the
// path absolute: from the sphere's root, not from the current space.
func (p *TRuleParser) AdjacentPathety(target *string) bool {
	var segment string
	return !p.ThisToken().PrecededByWhitespace &&
		/**/ (p.ThisPunctuationWas('/') && p.AppendText(target, "/") || true) &&
		/**/ p.ThisWord(&segment) &&
		/**/ p.AppendText(target, segment) &&
		/**/ p.PathSegmentsety(target) ||
		true
}

// PathSegmentsety: ( "/" Word )*
func (p *TRuleParser) PathSegmentsety(target *string) bool {
	var segment string
	for p.ThisPunctuationWas('/') &&
		/**/ p.ForcedThisWord(&segment, MessagePathSegment) &&
		/**/ p.AppendText(target, "/"+segment) {
		// Skip
	}
	return true
}

// ForcedTurnState: "on" | "off"
func (p *TRuleParser) ForcedTurnState(target *string) bool {
	return (p.ThisWordIs("on") || p.ThisWordIs("off")) && p.ThisWord(target) ||
		p.Failed(ErrorExpectedGot, MessageOnOrOff, p.ThisTokenText())
}

// Duration: Word ":" Word ":" Word, into *target as written (HH:MM:SS).
func (p *TRuleParser) Duration(target *string) bool {
	var hours, minutes, seconds string
	return p.ForcedThisWord(&hours, MessageDuration) &&
		/**/ p.ForcedThisPunctuationWas(':') &&
		/**/ p.ForcedThisWord(&minutes, MessageDuration) &&
		/**/ p.ForcedThisPunctuationWas(':') &&
		/**/ p.ForcedThisWord(&seconds, MessageDuration) &&
		/**/ p.SetText(target, hours+":"+minutes+":"+seconds)
}

// ForDurationety: ( "for" Duration )?
func (p *TRuleParser) ForDurationety(target *string) bool {
	return p.ThisWordWas("for") &&
		/**/ p.Duration(target) ||
		true
}

// TimeOfDay: Word ":" Word Secondsety, into *target as written (HH:MM[:SS]).
func (p *TRuleParser) TimeOfDay(target *string) bool {
	var hours, minutes string
	return p.ForcedThisWord(&hours, MessageTimeOfDay) &&
		/**/ p.ForcedThisPunctuationWas(':') &&
		/**/ p.ForcedThisWord(&minutes, MessageTimeOfDay) &&
		/**/ p.SetText(target, hours+":"+minutes) &&
		/**/ p.Secondsety(target)
}

// Secondsety: ( ":" Word )?
func (p *TRuleParser) Secondsety(target *string) bool {
	var seconds string
	return p.ThisPunctuationWas(':') &&
		/**/ p.ForcedThisWord(&seconds, MessageTimeOfDay) &&
		/**/ p.AppendText(target, ":"+seconds) ||
		true
}

// Value: "jinja" String | Word, setting *isJinja.
func (p *TRuleParser) Value(value *string, isJinja *bool) bool {
	*isJinja = p.ThisWordWas("jinja")
	switch {
	case *isJinja:
		return p.ForcedThisString(value, MessageJinjaTemplate)
	default:
		return p.ForcedThisWord(value, MessageValue)
	}
}

// DataFieldsety: ( "with" DataField ( "," DataField )* )?
func (p *TRuleParser) DataFieldsety(fields *[]TRuleDataField) bool {
	if p.ThisWordWas("with") && p.DataField(fields) {
		for p.ThisPunctuationWas(',') && p.DataField(fields) {
			// Skip
		}
	}
	return true
}

// DataField: "default"? Word Value "%"?. "default" and "%" only read well: "with default
// brightness 70%".
func (p *TRuleParser) DataField(fields *[]TRuleDataField) bool {
	var field TRuleDataField
	return (p.ThisWordWas("default") || true) &&
		/**/ p.ForcedThisWord(&field.Name, MessageDataField) &&
		/**/ p.Value(&field.Value, &field.IsJinja) &&
		/**/ (!field.IsJinja && p.ThisPunctuationWas('%') || true) &&
		/**/ p.AppendDataField(fields, field)
}

// --- triggers ---

// WhenStatement: "when" TriggerClause ( "or" TriggerClause )*
func (p *TRuleParser) WhenStatement(triggers *[]TRuleTrigger) bool {
	if !(p.ThisWordWas("when") && p.TriggerClause(triggers)) {
		return false
	}
	for p.ThisWordWas("or") && p.TriggerClause(triggers) {
		// Skip
	}
	return p.ForcedEndOfStatement()
}

// TriggerClause: "time" "is" TimeOfDay | "every" "minute" | EntityAnchoredClause, appended to
// *triggers.
func (p *TRuleParser) TriggerClause(triggers *[]TRuleTrigger) bool {
	var trigger TRuleTrigger
	return p.TriggerClauseKind(&trigger) &&
		/**/ p.AppendTrigger(triggers, trigger)
}

// TriggerClauseKind parses one trigger clause into *trigger.
func (p *TRuleParser) TriggerClauseKind(trigger *TRuleTrigger) bool {
	switch {
	// A time of day; EventType holds it.
	case p.ThisWordWas("time"):
		return p.SetTriggerKind(trigger, "time") &&
			/**/ p.ForcedThisWordWas("is") &&
			/**/ p.TimeOfDay(&trigger.EventType)

	// Every minute.
	case p.ThisWordWas("every"):
		return p.SetTriggerKind(trigger, "time_pattern") &&
			/**/ p.ForcedThisWordWas("minute")

	// An entity changing state, or raising an event.
	default:
		return p.EntityRef(&trigger.EntitySpec) &&
			/**/ p.TriggerPredicate(trigger) &&
			/**/ p.NamedTriggerReference(trigger)
	}
}

// TriggerPredicate: "turns" Word ForDurationety | "changes" ForDurationety | "is" Word
func (p *TRuleParser) TriggerPredicate(trigger *TRuleTrigger) bool {
	switch {
	// A change to a given state (any value, e.g. a vacuum's "error").
	case p.ThisWordWas("turns"):
		return p.SetTriggerKind(trigger, "state") &&
			/**/ p.ForcedThisWord(&trigger.State, MessageState) &&
			/**/ p.ForDurationety(&trigger.For)

	// Any change of state: no State.
	case p.ThisWordWas("changes"):
		return p.SetTriggerKind(trigger, "state") &&
			/**/ p.ForDurationety(&trigger.For)

	// An event of the given type.
	case p.ThisWordWas("is"):
		return p.SetTriggerKind(trigger, "event") &&
			/**/ p.ForcedThisWord(&trigger.EventType, MessageEventType)

	default:
		return p.Failed(ErrorExpectedGot, MessageTriggerPredicate, p.ThisTokenText())
	}
}

// --- conditions ---

// IfStatement: "if" ConditionOrExpr. "not" binds tighter than "and", "and" tighter than "or".
func (p *TRuleParser) IfStatement(cond *TRuleCondition) bool {
	return p.ThisWordWas("if") &&
		/**/ p.ConditionOrExpr(cond) &&
		/**/ p.ForcedEndOfStatement()
}

// ConditionOrExpr: ConditionAndExpr ( "or" ConditionAndExpr )*
func (p *TRuleParser) ConditionOrExpr(cond *TRuleCondition) bool {
	return p.combinedConditions(cond, "or", p.ConditionAndExpr)
}

// ConditionAndExpr: ConditionNotExpr ( "and" ConditionNotExpr )*
func (p *TRuleParser) ConditionAndExpr(cond *TRuleCondition) bool {
	return p.combinedConditions(cond, "and", p.ConditionNotExpr)
}

// combinedConditions: Operand ( keyword Operand )*, combined by keyword into *cond.
func (p *TRuleParser) combinedConditions(cond *TRuleCondition, keyword string, operand func(*TRuleCondition) bool) bool {
	var operands []TRuleCondition
	var next TRuleCondition
	if !(operand(&next) && p.AppendCondition(&operands, next)) {
		return false
	}
	for p.ThisWordWas(keyword) {
		next = TRuleCondition{}
		if !(operand(&next) && p.AppendCondition(&operands, next)) {
			return false
		}
	}
	return p.CombinedCondition(cond, keyword, operands)
}

// ConditionNotExpr: "not" ConditionNotExpr | ConditionAtom
func (p *TRuleParser) ConditionNotExpr(cond *TRuleCondition) bool {
	var inner TRuleCondition
	switch {
	case p.ThisWordWas("not"):
		return p.ConditionNotExpr(&inner) &&
			/**/ p.NegatedCondition(cond, inner)
	default:
		return p.ConditionAtom(cond)
	}
}

// ConditionAtom: "(" ConditionOrExpr ")" | TemplateCondition | TimeCondition | StateCondition
func (p *TRuleParser) ConditionAtom(cond *TRuleCondition) bool {
	switch {
	// A condition in parentheses.
	case p.ThisPunctuationWas('('):
		return p.ConditionOrExpr(cond) &&
			/**/ p.ForcedThisPunctuationWas(')')

	// An attribute of the trigger: "trigger" "." Word "is" String.
	case p.ThisWordWas("trigger"):
		return p.SetConditionKind(cond, "template") &&
			/**/ p.ForcedThisPunctuationWas('.') &&
			/**/ p.ForcedThisWord(&cond.TriggerAttr, MessageTriggerAttribute) &&
			/**/ p.ForcedThisWordWas("is") &&
			/**/ p.ForcedThisString(&cond.Value, MessageQuotedValue)

	// A time window: "time" Afterety Beforeety Weekdaysety, at least one of them.
	case p.ThisWordWas("time"):
		return p.SetConditionKind(cond, "time") &&
			/**/ p.Afterety(cond) &&
			/**/ p.Beforeety(cond) &&
			/**/ p.Weekdaysety(cond) &&
			/**/ p.TimeConditionGiven(cond)

	// An entity's state, held for at least the duration: EntityRef "is" Word ForDurationety.
	case p.ThisToken().Kind == TokWord:
		return p.SetConditionKind(cond, "state") &&
			/**/ p.EntityRef(&cond.EntitySpec) &&
			/**/ p.ForcedThisWordWas("is") &&
			/**/ p.ForcedThisWord(&cond.State, MessageState) &&
			/**/ p.ForDurationety(&cond.For)

	default:
		return p.Failed(ErrorExpectedGot, MessageCondition, p.ThisTokenText())
	}
}

// Afterety: ( "after" TimeOfDay )?
func (p *TRuleParser) Afterety(cond *TRuleCondition) bool {
	return p.ThisWordWas("after") &&
		/**/ p.TimeOfDay(&cond.After) ||
		true
}

// Beforeety: ( "before" TimeOfDay )?
func (p *TRuleParser) Beforeety(cond *TRuleCondition) bool {
	return p.ThisWordWas("before") &&
		/**/ p.TimeOfDay(&cond.Before) ||
		true
}

// Weekdaysety: ( "weekday" Word ( "," Word )* )?
func (p *TRuleParser) Weekdaysety(cond *TRuleCondition) bool {
	var day string
	if p.ThisWordWas("weekday") && p.ForcedThisWord(&day, MessageWeekday) {
		cond.Weekdays = append(cond.Weekdays, day)
		for p.ThisPunctuationWas(',') && p.ForcedThisWord(&day, MessageWeekday) {
			cond.Weekdays = append(cond.Weekdays, day)
		}
	}
	return true
}

// --- actions ---

// ThenStatement: "then" ActionStatement
func (p *TRuleParser) ThenStatement(action *TRuleAction) bool {
	return p.ThisWordWas("then") &&
		/**/ p.ActionStatement(action)
}

// ActionStatement: one action, as in a "then" statement or a "then: ... end;" block.
func (p *TRuleParser) ActionStatement(action *TRuleAction) bool {
	var magnitude int
	switch {
	// Switch on or off, with data for the service.
	case p.ThisWordWas("turn"):
		return p.SetVerb(action, "turn") &&
			/**/ p.EntityRef(&action.EntitySpec) &&
			/**/ p.ForcedTurnState(&action.State) &&
			/**/ p.DataFieldsety(&action.Data) &&
			/**/ p.ForcedThisPunctuationWas(';')

	// Toggle, with data for the service.
	case p.ThisWordWas("toggle"):
		return p.SetVerb(action, "toggle") &&
			/**/ p.EntityRef(&action.EntitySpec) &&
			/**/ p.DataFieldsety(&action.Data) &&
			/**/ p.ForcedThisPunctuationWas(';')

	// A step down in brightness.
	case p.ThisWordWas("dim"):
		return p.EntityRef(&action.EntitySpec) &&
			/**/ p.ForcedThisWordWas("with") &&
			/**/ p.ForcedNumber(&magnitude, 0, 100, MessageBrightnessStep) &&
			/**/ (p.ThisPunctuationWas('%') || true) &&
			/**/ p.AppendBrightnessStep(action, -magnitude) &&
			/**/ p.ForcedThisPunctuationWas(';')

	// A step up in brightness.
	case p.ThisWordWas("brighten"):
		return p.EntityRef(&action.EntitySpec) &&
			/**/ p.ForcedThisWordWas("with") &&
			/**/ p.ForcedNumber(&magnitude, 0, 100, MessageBrightnessStep) &&
			/**/ (p.ThisPunctuationWas('%') || true) &&
			/**/ p.AppendBrightnessStep(action, magnitude) &&
			/**/ p.ForcedThisPunctuationWas(';')

	// A service of the entity's domain without data ("<domain>.<verb>").
	case simpleActionVerbs[p.ThisToken().Value] && p.ThisToken().Kind == TokWord:
		return p.ThisWord(&action.Verb) &&
			/**/ p.EntityRef(&action.EntitySpec) &&
			/**/ p.ForcedThisPunctuationWas(';')

	// Set a value.
	case p.ThisWordWas("set"):
		return p.SetVerb(action, "set") &&
			/**/ p.EntityRef(&action.EntitySpec) &&
			/**/ p.ForcedThisWordWas("to") &&
			/**/ p.Value(&action.Value, &action.ValueIsJinja) &&
			/**/ p.ForcedThisPunctuationWas(';')

	// Wait.
	case p.ThisWordWas("delay"):
		return p.SetVerb(action, "delay") &&
			/**/ p.Duration(&action.Duration) &&
			/**/ p.ForcedThisPunctuationWas(';')

	default:
		return p.Failed(ErrorExpectedGot, MessageActionVerb, p.ThisTokenText())
	}
}

// IfActionHeader: "then" "if" ConditionOrExpr, of a "then if ...:" block (the ":" removed by the
// caller).
func (p *TRuleParser) IfActionHeader(action *TRuleAction) bool {
	return p.SetVerb(action, "if") &&
		/**/ p.ThisWordWas("then") &&
		/**/ p.ThisWordWas("if") &&
		/**/ p.ConditionOrExpr(&action.Condition) &&
		/**/ p.ForcedEndOfStatement()
}

// ElseIfHeader: "else" "if" ConditionOrExpr, of an "else if ...:" branch of a "then if ...:" block
// (the ":" removed by the caller).
func (p *TRuleParser) ElseIfHeader(action *TRuleAction) bool {
	return p.SetVerb(action, "if") &&
		/**/ p.ThisWordWas("else") &&
		/**/ p.ThisWordWas("if") &&
		/**/ p.ConditionOrExpr(&action.Condition) &&
		/**/ p.ForcedEndOfStatement()
}
