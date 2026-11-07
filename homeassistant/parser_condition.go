/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ParserCondition
 *
 * The grammar of the conditions and values of derived capabilities (Logical.def, and Physical.def's
 * "with condition"): a boolean expression over references to capabilities and Jinja clauses.
 *
 *   OrExpression:      AndExpression ( "or" AndExpression )*
 *   AndExpression:     NotExpression ( "and" "if"? NotExpression )*
 *   NotExpression:     "not" NotExpression | Operand
 *   Operand:           "(" OrExpression ")" | Atom
 *   Atom:              "jinja" JinjaClause | "physical" Word Availableety | Reference
 *   Reference:         Name ( "from" "physical"? Name )? ( "is" ( "available" | Values ) )?
 *   Values:            String | "in" "[" String ( "," String )* "]"
 *   JinjaClause:       String | "(" Name ")" NamedTemplateCall
 *   NamedTemplateCall: "$" "{" Word "}" "(" ( Argument ( "," Argument )* )? ")"
 *   Name:              Word ( ( "." | "!" ) Word )*, written without spaces
 *
 * A Name is a capability label of the same device, or an entity_id ("domain.path", with
 * "!attribute" for an attribute); they are resolved later (resolveLogicalSiblingSourceEntityID).
 * "from <device>" refers to another device's capability; "physical" to the device's capability as
 * Physical.def declares it, without Logical.def's override. "physical <device>" is short for
 * "node from physical <device>". "and if" reads as a filter and means "and".
 *
 * A Jinja string refers to its sources as "$[domain.label]". A named template ("${name}(args)",
 * Settings.def) is applied to the one source in parentheses before it, which takes the template's
 * "$"; it is expanded here.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 07.11.2026
 *
 */

package main

import "strings"

// TConditionExprKind discriminates TConditionExpr's nodes.
type TConditionExprKind int

const (
	CondRef TConditionExprKind = iota
	CondJinja
	CondNot
	CondAnd
	CondOr
)

// TConditionExpr is one node of a parsed condition or value. The leaves hold the references as
// written; resolveLogicalSiblingSourceEntityID resolves them against the owning device.
type TConditionExpr struct {
	Kind TConditionExprKind

	// CondRef: Ref, of FromDeviceID if set (as Physical.def declares it if ForcePhysical).
	// IsAvailable for "is available"; Values for "is '<value>'" and "is in [...]".
	Ref           string
	IsAvailable   bool
	FromDeviceID  string
	ForcePhysical bool
	Values        []string

	// CondJinja: JinjaExpr, with its "$[...]" placeholders; for a named template, the template
	// expanded, its "$1" taking JinjaSources' one source.
	JinjaExpr    string
	JinjaSources []string

	Child       *TConditionExpr // CondNot
	Left, Right *TConditionExpr // CondAnd, CondOr
}

// TConditionParser embeds TDefParser. jinjaTemplates are Settings.def's named templates.
type TConditionParser struct {
	TDefParser     // the expression being parsed
	jinjaTemplates map[string]TJinjaTemplateDefinition
}

// --- top level ---

// newConditionParser returns a parser for source; origin names it in messages.
func newConditionParser(source string, jinjaTemplates map[string]TJinjaTemplateDefinition, origin string) *TConditionParser {
	p := &TConditionParser{jinjaTemplates: jinjaTemplates}
	p.Initialise(source, origin)
	return p
}

// parseConditionExpr parses a condition (the text after "condition", without its ";").
func parseConditionExpr(raw string, jinjaTemplates map[string]TJinjaTemplateDefinition, origin string) (*TConditionExpr, []string) {
	var tree *TConditionExpr
	p := newConditionParser(raw, jinjaTemplates, origin)
	if !(p.OrExpression(&tree) && p.ForcedEndOfStatement()) {
		return nil, p.Warnings
	}
	return tree, p.Warnings
}

// parseValueExpr parses a value (the text after "value", without its ";"): one Atom, as a value is
// not boolean.
func parseValueExpr(raw string, jinjaTemplates map[string]TJinjaTemplateDefinition, origin string) (*TConditionExpr, []string) {
	var tree *TConditionExpr
	p := newConditionParser(raw, jinjaTemplates, origin)
	if !(p.Atom(&tree) && p.ForcedEndOfStatement()) {
		return nil, p.Warnings
	}
	return tree, p.Warnings
}

// --- small helpers ---

// SetExpression sets *out to expression.
func (p *TConditionParser) SetExpression(out **TConditionExpr, expression *TConditionExpr) bool {
	*out = expression
	return true
}

// --- names ---

// Name: Word ( ( "." | "!" ) Word )*, written without spaces, into *target.
func (p *TConditionParser) Name(target *string) bool {
	return p.ThisWord(target) &&
		/**/ p.NameRestety(target)
}

// ForcedName requires a Name; what names it in the message.
func (p *TConditionParser) ForcedName(target *string, what string) bool {
	return p.Name(target) ||
		p.Failed(ErrorExpectedGot, what, p.ThisTokenText())
}

// NameRestety: ( ( "." | "!" ) Word )*, written without spaces.
func (p *TConditionParser) NameRestety(target *string) bool {
	var segment string
	separator := p.ThisToken().Value
	for (p.ThisAdjacentPunctuationWas('.') || p.ThisAdjacentPunctuationWas('!')) &&
		/**/ (p.ThisAdjacentWord(&segment) || p.Failed(ErrorExpectedGot, MessageNameRest, p.ThisTokenText())) {
		*target += separator + segment
		separator = p.ThisToken().Value
	}
	return true
}

// --- expressions ---

// OrExpression: AndExpression ( "or" AndExpression )*
func (p *TConditionParser) OrExpression(out **TConditionExpr) bool {
	var right *TConditionExpr
	if !p.AndExpression(out) {
		return false
	}
	for p.ThisWordWas("or") {
		if !p.AndExpression(&right) {
			return false
		}
		*out = &TConditionExpr{Kind: CondOr, Left: *out, Right: right}
	}
	return true
}

// AndExpression: NotExpression ( "and" "if"? NotExpression )*
func (p *TConditionParser) AndExpression(out **TConditionExpr) bool {
	var right *TConditionExpr
	if !p.NotExpression(out) {
		return false
	}
	for p.ThisWordWas("and") {
		if !((p.ThisWordWas("if") || true) && p.NotExpression(&right)) {
			return false
		}
		*out = &TConditionExpr{Kind: CondAnd, Left: *out, Right: right}
	}
	return true
}

// NotExpression: "not" NotExpression | Operand
func (p *TConditionParser) NotExpression(out **TConditionExpr) bool {
	var child *TConditionExpr
	switch {
	case p.ThisWordWas("not"):
		return p.NotExpression(&child) &&
			/**/ p.SetExpression(out, &TConditionExpr{Kind: CondNot, Child: child})
	default:
		return p.Operand(out)
	}
}

// Operand: "(" OrExpression ")" | Atom
func (p *TConditionParser) Operand(out **TConditionExpr) bool {
	switch {
	case p.ThisPunctuationWas('('):
		return p.OrExpression(out) &&
			/**/ p.ForcedThisPunctuationWas(')')
	default:
		return p.Atom(out)
	}
}

// Atom: "jinja" JinjaClause | "physical" Word Availableety | Reference
func (p *TConditionParser) Atom(out **TConditionExpr) bool {
	var deviceID string
	switch {
	// A Jinja clause.
	case p.ThisWordWas("jinja"):
		return p.JinjaClause(out)

	// The node of a device, as Physical.def declares it.
	case p.ThisWordWas("physical"):
		*out = &TConditionExpr{Kind: CondRef, Ref: "node", ForcePhysical: true}
		return p.ForcedName(&deviceID, MessageDeviceID) &&
			/**/ p.SetFromDevice(*out, deviceID) &&
			/**/ p.Availableety(*out)

	// A reference to a capability or entity.
	default:
		return p.Reference(out)
	}
}

// Availableety: ( "is" "available" )?
func (p *TConditionParser) Availableety(expression *TConditionExpr) bool {
	return p.ThisWordWas("is") &&
		/**/ p.ForcedThisWordWas("available") &&
		/**/ p.SetAvailable(expression) ||
		true
}

// Reference: Name ( "from" "physical"? Name )? ( "is" ( "available" | Values ) )?
func (p *TConditionParser) Reference(out **TConditionExpr) bool {
	*out = &TConditionExpr{Kind: CondRef}
	return p.ForcedName(&(*out).Ref, MessageReference) &&
		/**/ p.FromDeviceety(*out) &&
		/**/ p.Stateety(*out)
}

// FromDeviceety: ( "from" "physical"? Name )?
func (p *TConditionParser) FromDeviceety(expression *TConditionExpr) bool {
	var deviceID string
	return p.ThisWordWas("from") &&
		/**/ (p.ThisWordWas("physical") && p.SetForcePhysical(expression) || true) &&
		/**/ p.ForcedName(&deviceID, MessageDeviceID) &&
		/**/ p.SetFromDevice(expression, deviceID) ||
		true
}

// Stateety: ( "is" ( "available" | Values ) )?
func (p *TConditionParser) Stateety(expression *TConditionExpr) bool {
	return p.ThisWordWas("is") &&
		/**/ (p.ThisWordWas("available") && p.SetAvailable(expression) ||
			/**/ p.Values(&expression.Values)) ||
		true
}

// Values: String | "in" "[" String ( "," String )* "]"
func (p *TConditionParser) Values(out *[]string) bool {
	var value string
	switch {
	// One value.
	case p.ThisString(&value):
		*out = []string{value}
		return true

	// A list of values.
	case p.ThisWordWas("in"):
		if !(p.ForcedThisPunctuationWas('[') && p.ForcedThisString(&value, MessageQuotedValue)) {
			return false
		}
		*out = append(*out, value)
		for p.ThisPunctuationWas(',') && p.ForcedThisString(&value, MessageQuotedValue) {
			*out = append(*out, value)
		}
		return p.ForcedThisPunctuationWas(']')

	default:
		return p.Failed(ErrorExpectedGot, MessageStateValues, p.ThisTokenText())
	}
}

// SetFromDevice makes expression refer to deviceID's capability.
func (p *TConditionParser) SetFromDevice(expression *TConditionExpr, deviceID string) bool {
	expression.FromDeviceID = deviceID
	return true
}

// SetForcePhysical makes expression refer to the capability as Physical.def declares it.
func (p *TConditionParser) SetForcePhysical(expression *TConditionExpr) bool {
	expression.ForcePhysical = true
	return true
}

// SetAvailable makes expression test availability.
func (p *TConditionParser) SetAvailable(expression *TConditionExpr) bool {
	expression.IsAvailable = true
	return true
}

// --- Jinja ---

// JinjaClause: String | "(" Name ")" NamedTemplateCall
func (p *TConditionParser) JinjaClause(out **TConditionExpr) bool {
	var text, source string
	switch {
	// A Jinja expression with "$[...]" placeholders.
	case p.ThisString(&text):
		*out = &TConditionExpr{Kind: CondJinja, JinjaExpr: text}
		return true

	// A named template, applied to one source.
	case p.ThisPunctuationWas('('):
		return p.ForcedName(&source, MessageJinjaSource) &&
			/**/ p.ForcedThisPunctuationWas(')') &&
			/**/ p.NamedTemplateCall(&text) &&
			/**/ p.SetExpression(out, &TConditionExpr{Kind: CondJinja, JinjaExpr: text, JinjaSources: []string{source}})

	default:
		return p.Failed(ErrorExpectedGot, MessageJinjaClause, p.ThisTokenText())
	}
}

// NamedTemplateCall: "$" "{" Word "}" "(" ( Argument ( "," Argument )* )? ")", expanded into
// *expanded.
func (p *TConditionParser) NamedTemplateCall(expanded *string) bool {
	var name string
	var arguments []string
	return p.ForcedThisPunctuationWas('$') &&
		/**/ p.ForcedThisPunctuationWas('{') &&
		/**/ p.ForcedThisWord(&name, MessageTemplateName) &&
		/**/ p.ForcedThisPunctuationWas('}') &&
		/**/ p.ForcedThisPunctuationWas('(') &&
		/**/ p.Argumentsety(&arguments) &&
		/**/ p.ForcedThisPunctuationWas(')') &&
		/**/ p.ExpandedTemplate(name, arguments, expanded)
}

// Argumentsety: ( Argument ( "," Argument )* )?
func (p *TConditionParser) Argumentsety(arguments *[]string) bool {
	if p.Argument(arguments) {
		for p.ThisPunctuationWas(',') && p.ForcedArgument(arguments) {
			// Skip
		}
	}
	return true
}

// Argument: String | Name, appended to *arguments.
func (p *TConditionParser) Argument(arguments *[]string) bool {
	var argument string
	return (p.ThisString(&argument) || p.Name(&argument)) &&
		/**/ p.AppendArgument(arguments, argument)
}

// ForcedArgument requires an Argument.
func (p *TConditionParser) ForcedArgument(arguments *[]string) bool {
	return p.Argument(arguments) ||
		p.Failed(ErrorExpectedGot, MessageArgument, p.ThisTokenText())
}

// AppendArgument appends argument to *arguments.
func (p *TConditionParser) AppendArgument(arguments *[]string, argument string) bool {
	*arguments = append(*arguments, argument)
	return true
}

// ExpandedTemplate expands Settings.def's template name with arguments into *expanded: each
// "${param}" becomes its argument, and "$" the source's "$1".
func (p *TConditionParser) ExpandedTemplate(name string, arguments []string, expanded *string) bool {
	definition, found := p.jinjaTemplates[name]
	switch {
	case !found:
		return p.Failed(ErrorUndeclaredTemplate, name, name)
	case len(arguments) != len(definition.Params):
		return p.Failed(ErrorTemplateArguments, name, len(arguments), len(definition.Params))
	}
	*expanded = definition.Template
	for i, param := range definition.Params {
		*expanded = strings.ReplaceAll(*expanded, "${"+param+"}", arguments[i])
	}
	*expanded = strings.ReplaceAll(*expanded, "$", "$1")
	return true
}
