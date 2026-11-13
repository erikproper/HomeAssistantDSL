/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ParserBase
 *
 * The part of the definition parsers every grammar shares (GO_CONVENTIONS.md §8), embedding
 * TTokeniser: error reporting and recovery, the Forced variants of the token primitives, and
 * numbers. Each grammar (parser_rule.go, parser_condition.go, parser_battery_emulation.go)
 * embeds TDefParser.
 *
 * A parser is given one statement at a time, so a position is the line and column within that
 * statement, and recovery skips to the end of it.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Vienna, Austria, in collaboration with Claude.ai
 *
 * Version of: 07.11.2026
 *
 */

package main

import (
	"fmt"
	"strconv"
)

// TDefParser embeds TTokeniser. origin names what is parsed, at the start of each message.
type TDefParser struct {
	TTokeniser // the tokens being parsed
	origin     string
	skipping   bool // after an error: the rest of the statement is skipped silently
	Warnings   []string
}

// Initialise sets p up over source.
func (p *TDefParser) Initialise(source, origin string) {
	p.TTokeniser.Initialise(source)
	p.origin = origin
}

// --- error reporting and recovery ---

// ReportParsingError records message, at the current token, unless an earlier error is being
// skipped. It returns true, for the left of a "&& SkipToEndOfStatement()".
func (p *TDefParser) ReportParsingError(message string, args ...any) bool {
	if !p.skipping {
		p.Warnings = append(p.Warnings, fmt.Sprintf("%s: %s (L:%d, C:%d)", p.origin, fmt.Sprintf(message, args...), p.ThisToken().Line, p.ThisToken().Column))
		p.skipping = true
	}
	return true
}

// SkipToEndOfStatement skips past the next ";", or to the end. It returns false, ending the
// current "&&" chain.
func (p *TDefParser) SkipToEndOfStatement() bool {
	for !p.AtEndOfTokens() && !p.ThisPunctuationWas(';') {
		p.NextToken()
	}
	return false
}

// Failed reports the error message and skips; it returns false.
func (p *TDefParser) Failed(message string, args ...any) bool {
	return p.ReportParsingError(message, args...) && p.SkipToEndOfStatement()
}

// ThisTokenText describes the current token in a message.
func (p *TDefParser) ThisTokenText() string {
	switch p.ThisToken().Kind {
	case TokEOF:
		return MessageEndOfStatement
	case TokUnterminated:
		return MessageUnterminatedString
	default:
		return strconv.Quote(p.ThisToken().Value)
	}
}

// --- Forced variants ---

// ForcedThisWordWas requires the word word.
func (p *TDefParser) ForcedThisWordWas(word string) bool {
	return p.ThisWordWas(word) ||
		p.Failed(ErrorExpectedGot, strconv.Quote(word), p.ThisTokenText())
}

// ForcedThisPunctuationWas requires the punctuation ch.
func (p *TDefParser) ForcedThisPunctuationWas(ch byte) bool {
	return p.ThisPunctuationWas(ch) ||
		p.Failed(ErrorExpectedGot, strconv.Quote(string(ch)), p.ThisTokenText())
}

// ForcedThisWord requires a word, copied into *target; what names it in the message.
func (p *TDefParser) ForcedThisWord(target *string, what string) bool {
	return p.ThisWord(target) ||
		p.Failed(ErrorExpectedGot, what, p.ThisTokenText())
}

// ForcedThisString requires a string, copied into *target; what names it in the message.
func (p *TDefParser) ForcedThisString(target *string, what string) bool {
	return p.ThisString(target) ||
		p.Failed(ErrorExpectedGot, what, p.ThisTokenText())
}

// ForcedEndOfStatement requires that nothing follows. After an error it fails: the statement
// did not parse, even if the error was in an optional part.
func (p *TDefParser) ForcedEndOfStatement() bool {
	return !p.skipping &&
		/**/ (p.AtEndOfTokens() ||
			/**/ p.Failed(ErrorTrailingText, p.ThisTokenText()))
}

// --- numbers ---

// ForcedNumber requires a whole number from minimum to maximum, stored in *target; what names it
// in the message.
func (p *TDefParser) ForcedNumber(target *int, minimum, maximum int, what string) bool {
	number, err := strconv.Atoi(p.ThisToken().Value)
	switch {
	case p.ThisToken().Kind != TokWord:
		return p.Failed(ErrorExpectedGot, what, p.ThisTokenText())
	case err != nil || number < minimum || number > maximum:
		return p.Failed(ErrorNotA, p.ThisToken().Value, what)
	default:
		*target = number
		return p.NextToken()
	}
}

// AssignedIDety: ( "as" "unique_id" Word ( "." Word )* )? -- a capability's frozen ID, shared by
// the integration grammars that take one (governed, fhem).
func (p *TDefParser) AssignedIDety(target *string) bool {
	var segment string
	if p.ThisWordWas("as") &&
		/**/ p.ForcedThisWordWas("unique_id") &&
		/**/ p.ForcedThisWord(target, MessageFrozenRoot) {
		for p.ThisPunctuationWas('.') &&
			/**/ p.ForcedThisWord(&segment, MessageFrozenRoot) {
			*target += "." + segment
		}
	}
	return true
}
