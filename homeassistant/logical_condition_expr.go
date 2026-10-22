/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: LogicalConditionExpr
 *
 * Tokeniser and recursive-descent parser for Logical.def's "derived <domain>.<label> with:
 * condition <bool-expr>; ... end;" / "... value <atom>; ... end;" grammar (2026-09-24 redesign,
 * replacing the older single-Jinja-string-plus-flat-"over:"-list shape). See PROJECT plan
 * "Logical.def condition/value boolean+jinja grammar redesign" for the full rationale.
 *
 * Grammar:
 *
 *   bool-expr  := bool-expr "and" bool-expr | bool-expr "or" bool-expr | "not" bool-expr
 *               | "(" bool-expr ")" | atom
 *   bool-expr  := ... | bool-expr "and" "if" bool-expr   ("if" optional, reads as a filter)
 *   atom       := ref ["from" ["physical"] device-id] ["is" ("available" | STRING | "in" "[" STRING
 *                 {"," STRING} "]")] | "physical" device-id ["is" "available"] | "jinja" jinja-head
 *   ref        := bare sibling capability label, or a literal "domain.path[!attribute]" entity
 *                 reference -- exactly logicalBareCapabilityRefPattern's own charset/meaning,
 *                 resolved later (registerLogicalDerivedConditionCapability) via the same
 *                 three-tier sibling lookup that already existed for the old "over:" list.
 *   jinja-head := STRING | "(" ref ")" "${" IDENT "}" "(" args ")"
 *
 * A literal-string head (2026-10-05, PROJECT.md 1h, the SECOND and final revision that same day --
 * a brief "(ref, ref, ...) head" reordering was tried first, then replaced by this) embeds its own
 * sources directly inside the quoted text, as "$[domain.label]" placeholders -- e.g.
 * jinja "($[sensor.wind_speed] in ['unknown', 'unavailable']) or (($[sensor.wind_speed] | int) >
 * ($[input_number.windy_threshold] | int))". Each placeholder is resolved independently at render
 * time (resolveJinjaToken/the CondJinja render case, Conceptual_LogicalEntities.go), the same
 * sibling-vs-literal disambiguation every other reference in this grammar already uses -- reading
 * a template no longer requires holding un-explained $1/$2 positions in mind while scrolling to a
 * separate sources list to find out what they mean, and reordering/adding/removing a reference
 * never requires renumbering anything else in the template. No dual syntax kept at any point in
 * this redesign -- every real .def usage (5 sites, both houses' Logical.def) and the one Go call
 * site that synthesizes jinja-clause text (logical_enabler_dependency_sugar.go's own "enabler"
 * OR-exception) were migrated together, twice, in the same day.
 *
 * A named-template-call head ("${name}(args)", Settings.def's int_less_then/int_more_then/...,
 * folded in here instead of the old separate resolveJinjaTemplateCallsInLogicalConditions
 * text-rewrite pass) still needs an explicit SINGLE source, kept parenthesized -- its own template
 * text lives in Settings.def, author-blind to any particular device, so it always resolves against
 * exactly one bare "$"/"$1" placeholder; there is nothing for it to embed a "$[...]" placeholder
 * INTO. "(ref)" here reads as "this template, applied to this one source," not a general sources
 * list -- e.g. jinja (sensor.battery_level) ${int_less_then}(20).
 *
 * "not" binds tighter than "and", which binds tighter than "or"; parens override. Two independent
 * jinja clauses coexist in one condition with no index-collision risk at all now, by construction
 * (each clause's own placeholders/source name themselves, there is no shared numbering to collide).
 *
 * This is genuine new recursive grammar (nested parens, quoted strings that must not be split on
 * their own embedded "and"/"or"/";") -- a line-based regexp state machine (this DSL's usual style,
 * see integration_logical_parser.go's own header comment) cannot express it correctly, and per
 * GO_CONVENTIONS.md Sec.7/8 (three-layer character-stream -> tokeniser -> recursive-descent
 * CDL1-style parser) plus prior explicit feedback against ad hoc regexp grammars
 * (memory/feedback_no_new_regexp_parsers.md), this one piece is built properly instead. The three
 * layers are embedding-chained in this one file (the grammar is small enough that splitting them
 * into three separate files, as the bibtex_check exemplar does for a much larger grammar, would
 * only hurt readability here).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 24.09.2026
 *
 */

package main

import (
	"fmt"
	"strings"
)

// TConditionExprKind discriminates TConditionExpr's node shapes.
type TConditionExprKind int

const (
	CondRef TConditionExprKind = iota
	CondJinja
	CondNot
	CondAnd
	CondOr
)

// TConditionExpr is one node of a parsed "condition"/"value" boolean+jinja expression tree.
// Leaves (CondRef/CondJinja) carry unresolved raw tokens -- resolution against a specific logical
// device's own siblings happens later, in Conceptual_LogicalEntities.go's
// resolveLogicalSiblingSourceEntityID, once administration state is available.
type TConditionExpr struct {
	Kind TConditionExprKind

	// CondRef only. Ref is either a bare sibling capability label of the OWNING device (no
	// FromDeviceID), or a label targeting an EXPLICITLY OTHER device's own capability
	// (FromDeviceID set, 2026-09-24 cross-device references -- "<label> from [physical]
	// <device-id>"). ForcePhysical (only meaningful with FromDeviceID set) forces resolution
	// against <device-id>'s own raw Physical.def capability, bypassing any Logical.def override
	// for that same label -- see resolveLogicalSiblingSourceEntityID's own doc comment.
	Ref           string
	IsAvailable   bool
	FromDeviceID  string
	ForcePhysical bool
	// Values (2026-10-22) is set by "<ref> is '<value>'" / "<ref> is in ['<value>', ...]": the
	// ref's state must be one of them. In an "event" capability's condition (PROJECT.md item 49)
	// an "event.<label>" ref matches its event types this way, and a "trigger.<attribute>" ref
	// filters on the firing event's own attribute.
	Values []string

	// CondJinja only. Two shapes (2026-10-05, PROJECT.md 1h): a literal-string head has
	// JinjaSources == nil and embeds its own "$[domain.label]" placeholders directly inside
	// JinjaExpr, each resolved independently at render time; a named-template-call head
	// ("${name}(args)") has exactly one JinjaSources entry, substituted positionally into
	// JinjaExpr's own bare "$"/"$1" (that template's text lives in Settings.def, so it can't embed
	// a placeholder of its own).
	JinjaExpr    string
	JinjaSources []string

	// CondNot only.
	Child *TConditionExpr
	// CondAnd/CondOr only.
	Left, Right *TConditionExpr
}

// --- Layer 1: character stream ---

// TConditionExprStream is a minimal rune-position cursor over the raw text captured between a
// "condition"/"value" keyword and its closing top-level ";" -- mirrors GO_CONVENTIONS.md Sec.7's
// TDefCharacterStream role, kept minimal since the source is always a short in-memory string, not
// a file.
type TConditionExprStream struct {
	runes []rune
	pos   int
}

func newConditionExprStream(source string) TConditionExprStream {
	return TConditionExprStream{runes: []rune(source)}
}

func (s *TConditionExprStream) peekRune() (rune, bool) {
	if s.pos >= len(s.runes) {
		return 0, false
	}
	return s.runes[s.pos], true
}

func (s *TConditionExprStream) nextRune() (rune, bool) {
	r, ok := s.peekRune()
	if ok {
		s.pos++
	}
	return r, ok
}

// --- Layer 2: tokeniser ---

type TConditionExprTokenKind int

const (
	TokIdent TConditionExprTokenKind = iota // identifiers AND keywords (and/or/not/is/available/jinja) -- see GO_CONVENTIONS.md Sec.10
	TokString
	TokLParen
	TokRParen
	TokLBrace
	TokRBrace
	TokComma
	TokLBracket
	TokRBracket
	TokDollarBrace // the two-char "${" sequence introducing a named jinja template call
	TokSemicolon
	TokNumber // a "${name}(args)" argument literal, e.g. "10" -- never a valid ref (refs always start with a letter/underscore)
	TokEOF
	TokError
)

type TConditionExprToken struct {
	Kind  TConditionExprTokenKind
	Value string
}

// conditionExprIdentRune reports whether r may appear inside a bare ref token -- mirrors
// logicalBareCapabilityRefPattern's own charset ("[A-Za-z_][A-Za-z0-9_.!]*") exactly, so a bare
// ref's resolution meaning (sibling label first, literal "domain.path[!attribute]" fallback) stays
// identical to the pre-2026-09-24 "over:" list entry it replaces.
func conditionExprIdentRune(r rune, first bool) bool {
	if r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
		return true
	}
	if first {
		return false
	}
	return (r >= '0' && r <= '9') || r == '.' || r == '!'
}

// TConditionExprTokeniser embeds the character stream and classifies rune sequences into typed
// tokens -- GO_CONVENTIONS.md Sec.7's tokeniser layer.
type TConditionExprTokeniser struct {
	TConditionExprStream
	current TConditionExprToken
	err     string
}

func newConditionExprTokeniser(source string) *TConditionExprTokeniser {
	t := &TConditionExprTokeniser{TConditionExprStream: newConditionExprStream(source)}
	t.advance()
	return t
}

func (t *TConditionExprTokeniser) advance() {
	for {
		r, ok := t.peekRune()
		if !ok {
			t.current = TConditionExprToken{Kind: TokEOF}
			return
		}
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			t.nextRune()
			continue
		}
		break
	}

	r, _ := t.nextRune()
	switch {
	case r == '(':
		t.current = TConditionExprToken{Kind: TokLParen, Value: "("}
	case r == ')':
		t.current = TConditionExprToken{Kind: TokRParen, Value: ")"}
	case r == '{':
		t.current = TConditionExprToken{Kind: TokLBrace, Value: "{"}
	case r == '}':
		t.current = TConditionExprToken{Kind: TokRBrace, Value: "}"}
	case r == ',':
		t.current = TConditionExprToken{Kind: TokComma, Value: ","}
	case r == '[':
		t.current = TConditionExprToken{Kind: TokLBracket, Value: "["}
	case r == ']':
		t.current = TConditionExprToken{Kind: TokRBracket, Value: "]"}
	case r == ';':
		t.current = TConditionExprToken{Kind: TokSemicolon, Value: ";"}
	case r == '$':
		if next, ok := t.peekRune(); ok && next == '{' {
			t.nextRune()
			t.current = TConditionExprToken{Kind: TokDollarBrace, Value: "${"}
		} else {
			t.current = TConditionExprToken{Kind: TokError, Value: "$"}
		}
	case r == '"' || r == '\'':
		var sb strings.Builder
		closed := false
		for {
			c, ok := t.nextRune()
			if !ok {
				break
			}
			if c == r {
				closed = true
				break
			}
			sb.WriteRune(c)
		}
		if !closed {
			t.current = TConditionExprToken{Kind: TokError, Value: `unterminated string literal`}
			return
		}
		t.current = TConditionExprToken{Kind: TokString, Value: sb.String()}
	case conditionExprIdentRune(r, true):
		var sb strings.Builder
		sb.WriteRune(r)
		for {
			c, ok := t.peekRune()
			if !ok || !conditionExprIdentRune(c, false) {
				break
			}
			sb.WriteRune(c)
			t.nextRune()
		}
		t.current = TConditionExprToken{Kind: TokIdent, Value: sb.String()}
	case r >= '0' && r <= '9':
		var sb strings.Builder
		sb.WriteRune(r)
		for {
			c, ok := t.peekRune()
			if !ok || !((c >= '0' && c <= '9') || c == '.') {
				break
			}
			sb.WriteRune(c)
			t.nextRune()
		}
		t.current = TConditionExprToken{Kind: TokNumber, Value: sb.String()}
	default:
		t.current = TConditionExprToken{Kind: TokError, Value: fmt.Sprintf("unexpected character %q", r)}
	}
}

// --- Layer 3: recursive-descent parser (CDL1-style, GO_CONVENTIONS.md Sec.8) ---

// TConditionExprParser embeds the tokeniser and implements the grammar rules above. Parse
// functions never return `error` (GO_CONVENTIONS.md Sec.8); a rule that fails records into
// warnings and returns false, aborting the current "&&" chain. jinjaTemplates resolves a
// "${name}(args)" jinja-head at parse time (Settings.def's own named templates).
type TConditionExprParser struct {
	*TConditionExprTokeniser
	jinjaTemplates map[string]TJinjaTemplateDefinition
	warnings       []string
	sourceLabel    string // for warning messages, e.g. "Logical.def device %q label %q"
}

func newConditionExprParser(source string, jinjaTemplates map[string]TJinjaTemplateDefinition, sourceLabel string) *TConditionExprParser {
	return &TConditionExprParser{
		TConditionExprTokeniser: newConditionExprTokeniser(source),
		jinjaTemplates:          jinjaTemplates,
		sourceLabel:             sourceLabel,
	}
}

func (p *TConditionExprParser) reportError(format string, args ...any) bool {
	p.warnings = append(p.warnings, fmt.Sprintf("%s: %s", p.sourceLabel, fmt.Sprintf(format, args...)))
	return false
}

// ThisIdentKeywordWas reports (and consumes) whether the current token is the identifier keyword
// kw -- "and"/"or"/"not"/"is"/"available"/"jinja" are all plain TokIdent values, matching
// GO_CONVENTIONS.md Sec.10's "TokIdent // keywords and identifiers" convention.
func (p *TConditionExprParser) ThisIdentKeywordWas(kw string) bool {
	if p.current.Kind == TokIdent && p.current.Value == kw {
		p.advance()
		return true
	}
	return false
}

func (p *TConditionExprParser) ForcedThisTokenWas(kind TConditionExprTokenKind, label string) bool {
	if p.current.Kind == kind {
		p.advance()
		return true
	}
	return p.reportError("expected %s, got %q", label, p.current.Value)
}

// ParseOrExpr := ParseAndExpr ("or" ParseAndExpr)*
func (p *TConditionExprParser) ParseOrExpr(out **TConditionExpr) bool {
	if !p.ParseAndExpr(out) {
		return false
	}
	for p.ThisIdentKeywordWas("or") {
		left := *out
		var right *TConditionExpr
		if !p.ParseAndExpr(&right) {
			return false
		}
		*out = &TConditionExpr{Kind: CondOr, Left: left, Right: right}
	}
	return true
}

// ParseAndExpr := ParseNotExpr ("and" ["if"] ParseNotExpr)*
func (p *TConditionExprParser) ParseAndExpr(out **TConditionExpr) bool {
	if !p.ParseNotExpr(out) {
		return false
	}
	for p.ThisIdentKeywordWas("and") {
		p.ThisIdentKeywordWas("if")
		left := *out
		var right *TConditionExpr
		if !p.ParseNotExpr(&right) {
			return false
		}
		*out = &TConditionExpr{Kind: CondAnd, Left: left, Right: right}
	}
	return true
}

// ParseNotExpr := "not" ParseNotExpr | ParseParenOrAtom
func (p *TConditionExprParser) ParseNotExpr(out **TConditionExpr) bool {
	if p.ThisIdentKeywordWas("not") {
		var child *TConditionExpr
		if !p.ParseNotExpr(&child) {
			return false
		}
		*out = &TConditionExpr{Kind: CondNot, Child: child}
		return true
	}
	return p.ParseParenOrAtom(out)
}

// ParseParenOrAtom := "(" ParseOrExpr ")" | ParseAtom
func (p *TConditionExprParser) ParseParenOrAtom(out **TConditionExpr) bool {
	if p.current.Kind == TokLParen {
		p.advance()
		if !p.ParseOrExpr(out) {
			return false
		}
		return p.ForcedThisTokenWas(TokRParen, `")"`)
	}
	return p.ParseAtom(out)
}

// ParseAtom := "jinja" ParseJinjaClause | "physical" IDENT ["is" "available"] | ParseRef
func (p *TConditionExprParser) ParseAtom(out **TConditionExpr) bool {
	if p.ThisIdentKeywordWas("jinja") {
		return p.ParseJinjaClause(out)
	}
	if p.ThisIdentKeywordWas("physical") {
		return p.ParsePhysicalShorthand(out)
	}
	return p.ParseRef(out)
}

// ParsePhysicalShorthand := IDENT ["is" "available"]  (the "physical" keyword itself already
// consumed by the caller)
//
// Bare "physical <device-id>" (2026-09-24) is sugar for "node from physical <device-id>" -- the
// single most common shape of the cross-device "from" clause, letting a Logical.def override
// reach its own device's raw un-overridden "node" capability without spelling out "node from"
// (see ParseRef's own doc comment for what "physical" forces).
func (p *TConditionExprParser) ParsePhysicalShorthand(out **TConditionExpr) bool {
	if p.current.Kind != TokIdent {
		return p.reportError("expected a device id after %q, got %q", "physical", p.current.Value)
	}
	deviceID := p.current.Value
	p.advance()
	isAvailable := false
	if p.ThisIdentKeywordWas("is") {
		if !p.ForcedThisIdentKeywordWas("available") {
			return false
		}
		isAvailable = true
	}
	*out = &TConditionExpr{Kind: CondRef, Ref: "node", IsAvailable: isAvailable, FromDeviceID: deviceID, ForcePhysical: true}
	return true
}

// ParseRef := IDENT ["from" ["physical"] IDENT] ["is" ("available" | ParseValues)]
//
// The optional "from [physical] <device-id>" clause (2026-09-24) targets an EXPLICITLY OTHER
// device's own capability instead of a same-device sibling label -- "physical" forces the lookup
// to bypass that device's own Logical.def override (if any) for the same label, reaching its raw
// Physical.def-declared value instead (see resolveLogicalSiblingSourceEntityID's own doc comment
// for exactly how this resolves and, when the target was never explicitly positioned, how it gets
// auto-materialized).
func (p *TConditionExprParser) ParseRef(out **TConditionExpr) bool {
	if p.current.Kind != TokIdent {
		return p.reportError("expected a reference (sibling capability label or entity_id) or %q, got %q", "jinja", p.current.Value)
	}
	ref := p.current.Value
	p.advance()
	fromDeviceID := ""
	forcePhysical := false
	if p.ThisIdentKeywordWas("from") {
		forcePhysical = p.ThisIdentKeywordWas("physical")
		if p.current.Kind != TokIdent {
			return p.reportError("expected a device id after %q, got %q", "from", p.current.Value)
		}
		fromDeviceID = p.current.Value
		p.advance()
	}
	isAvailable := false
	var values []string
	if p.ThisIdentKeywordWas("is") {
		if !p.ThisIdentKeywordWas("available") && !p.ParseValues(&values) {
			return false
		}
		isAvailable = values == nil
	}
	*out = &TConditionExpr{Kind: CondRef, Ref: ref, IsAvailable: isAvailable, FromDeviceID: fromDeviceID, ForcePhysical: forcePhysical, Values: values}
	return true
}

// ParseValues := STRING | "in" "[" STRING {"," STRING} "]"
func (p *TConditionExprParser) ParseValues(out *[]string) bool {
	if p.current.Kind == TokString {
		*out = []string{p.current.Value}
		p.advance()
		return true
	}
	if !p.ThisIdentKeywordWas("in") {
		return p.reportError("expected %q, a quoted value or %q after %q, got %q", "available", "in [...]", "is", p.current.Value)
	}
	if !p.ForcedThisTokenWas(TokLBracket, `"["`) {
		return false
	}
	for {
		if p.current.Kind != TokString {
			return p.reportError("expected a quoted value in the %q list, got %q", "in", p.current.Value)
		}
		*out = append(*out, p.current.Value)
		p.advance()
		if p.current.Kind != TokComma {
			break
		}
		p.advance()
	}
	return p.ForcedThisTokenWas(TokRBracket, `"]"`)
}

func (p *TConditionExprParser) ForcedThisIdentKeywordWas(kw string) bool {
	return p.ThisIdentKeywordWas(kw) || p.reportError("expected %q, got %q", kw, p.current.Value)
}

// ParseJinjaClause := STRING | "(" IDENT ")" "${" IDENT "}" "(" args ")"
//
// A literal-string head (2026-10-05, PROJECT.md 1h, replacing the brief 2026-10-04 "( sources )
// head" reorder in the same day) embeds its own sources directly, as "$[domain.label]"
// placeholders inside the quoted text itself -- each one resolved independently, the same
// disambiguation render's own CondJinja case already applies to every other reference in this
// grammar (sibling lookup vs literal entity_id). No separate sources list is parsed or accepted
// for this head shape -- retired in the same change as the brief parenthesized-list form (no
// dual syntax kept; the 5 real .def sites and the one Go call site synthesizing jinja-clause text,
// logical_enabler_dependency_sugar.go, were migrated together).
//
// A named-template-call head ("${name}(args)") still needs an explicit SINGLE source, kept
// parenthesized: its own template text lives in Settings.def, author-blind to any particular
// device, so it always resolves against exactly one bare "$" placeholder
// (TJinjaTemplateDefinition's own doc comment) -- there is nothing for it to embed a "$[...]"
// placeholder INTO. "(ref)" here reads as "this template, applied to this one source," not a
// general sources list.
func (p *TConditionExprParser) ParseJinjaClause(out **TConditionExpr) bool {
	if p.current.Kind == TokString {
		expr := p.current.Value
		p.advance()
		*out = &TConditionExpr{Kind: CondJinja, JinjaExpr: expr}
		return true
	}
	if !p.ForcedThisTokenWas(TokLParen, `"("`) {
		return false
	}
	if p.current.Kind != TokIdent {
		return p.reportError("expected a source reference inside \"( ... )\", got %q", p.current.Value)
	}
	source := p.current.Value
	p.advance()
	if !p.ForcedThisTokenWas(TokRParen, `")"`) {
		return false
	}
	expr, ok := p.parseJinjaNamedTemplateCall()
	if !ok {
		return false
	}
	*out = &TConditionExpr{Kind: CondJinja, JinjaExpr: expr, JinjaSources: []string{source}}
	return true
}

// parseJinjaNamedTemplateCall := "${" IDENT "}" "(" <raw-args-until-")"> ")" -- the caller
// (ParseJinjaClause) has already consumed this head's own single "(ref)" source. Resolves against
// p.jinjaTemplates immediately (Settings.def's own "jinja ${name}(params) = "template";"
// declarations, same mechanism resolveJinjaTemplateCallsInDerivedLines/the pre-2026-09-24
// resolveJinjaTemplateCallsInLogicalConditions already used).
func (p *TConditionExprParser) parseJinjaNamedTemplateCall() (string, bool) {
	if p.current.Kind != TokDollarBrace {
		return "", p.reportError(`expected a quoted jinja expression or "${", got %q`, p.current.Value)
	}
	p.advance()
	if p.current.Kind != TokIdent {
		return "", p.reportError("expected a jinja template name after \"${\", got %q", p.current.Value)
	}
	name := p.current.Value
	p.advance()
	if !p.ForcedThisTokenWas(TokRBrace, `"}"`) {
		return "", false
	}
	if !p.ForcedThisTokenWas(TokLParen, `"("`) {
		return "", false
	}
	var argTokens []string
	for p.current.Kind != TokRParen {
		if p.current.Kind == TokEOF || p.current.Kind == TokError {
			return "", p.reportError("unterminated \"${%s}(...)\" argument list", name)
		}
		argTokens = append(argTokens, p.current.Value)
		p.advance()
		if p.current.Kind == TokComma {
			p.advance()
		}
	}
	p.advance() // consume ")"

	def, found := p.jinjaTemplates[name]
	if !found {
		return "", p.reportError("references undeclared \"jinja ${%s}(...)\" -- add a \"jinja ${%s}(...) = \"...\";\" line to Settings.def", name, name)
	}
	if len(argTokens) != len(def.Params) {
		return "", p.reportError("calls \"jinja ${%s}(...)\" with %d argument(s), want %d", name, len(argTokens), len(def.Params))
	}
	resolved := def.Template
	for i, param := range def.Params {
		resolved = strings.ReplaceAll(resolved, "${"+param+"}", argTokens[i])
	}
	resolved = strings.ReplaceAll(resolved, "$", "$1")
	return resolved, true
}

// parseConditionExpr parses a "condition <bool-expr>" directive's raw expression text (everything
// after the "condition" keyword, up to but excluding the terminating ";") into a boolean
// TConditionExpr tree. sourceLabel is used to prefix any warnings.
func parseConditionExpr(raw string, jinjaTemplates map[string]TJinjaTemplateDefinition, sourceLabel string) (*TConditionExpr, []string) {
	p := newConditionExprParser(raw, jinjaTemplates, sourceLabel)
	var tree *TConditionExpr
	if !p.ParseOrExpr(&tree) {
		return nil, p.warnings
	}
	if p.current.Kind != TokEOF {
		p.reportError("unexpected trailing input %q after the expression", p.current.Value)
		return nil, p.warnings
	}
	return tree, p.warnings
}

// parseValueExpr parses a "value <atom>" directive's raw expression text into a single
// (non-boolean-composable) TConditionExpr leaf -- a bare ref or one jinja clause, never
// and/or/not/parens (a sensor value isn't boolean-composable).
func parseValueExpr(raw string, jinjaTemplates map[string]TJinjaTemplateDefinition, sourceLabel string) (*TConditionExpr, []string) {
	p := newConditionExprParser(raw, jinjaTemplates, sourceLabel)
	var tree *TConditionExpr
	if !p.ParseAtom(&tree) {
		return nil, p.warnings
	}
	if p.current.Kind != TokEOF {
		p.reportError("unexpected trailing input %q after the value expression", p.current.Value)
		return nil, p.warnings
	}
	return tree, p.warnings
}
