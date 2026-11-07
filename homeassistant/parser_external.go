/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ParserExternal
 *
 * The grammar of External.def, its lists (generator.go writes a list.* card for each):
 *
 *   ExternalDefinition: ( "external" "layer" "with" ":" )? ListStatement* ( "end" ";" )?
 *   ListStatement:      "list" String "all"? Pattern+ AsCardsety ListBody
 *   Pattern:            Text Underety                -- a domain, "." and a path ("*" any)
 *   Underety:           ( "under" Text )?            -- a path segment, "*" allowed
 *   AsCardsety:         ( "as" "cards" )?
 *   ListBody:           "with" ":" ListOption* "end" ";" | ";"
 *   ListOption:         ( "clean_prefix" | "clean_postfix" ) Text ";" | "detail_level" Number ";"
 *
 * A Text is written without spaces: a run of adjacent words and punctuation, up to a space or ";".
 * Comments ("#" to the end of the line) are taken out first.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 07.11.2026
 *
 */

package main

import (
	"math"
	"strings"
)

// TExternalParser embeds TDefParser.
type TExternalParser struct {
	TDefParser   // External.def
	Declarations []TListDeclaration
}

// --- top level ---

// parseExternalDefinition parses External.def's source into its lists; warnings say what is wrong.
func parseExternalDefinition(src string) ([]TListDeclaration, []string) {
	var lines []string
	for _, line := range strings.Split(src, "\n") {
		lines = append(lines, stripLineComment(line))
	}
	p := &TExternalParser{}
	p.Initialise(strings.Join(lines, "\n"), OriginExternal)
	p.ExternalDefinition()
	return p.Declarations, p.Warnings
}

// parseListDeclarations is parseExternalDefinition without its warnings.
func parseListDeclarations(src string) []TListDeclaration {
	declarations, _ := parseExternalDefinition(src)
	return declarations
}

// --- grammar ---

// ExternalDefinition: ( "external" "layer" "with" ":" )? ListStatement* ( "end" ";" )?
func (p *TExternalParser) ExternalDefinition() bool {
	if p.ThisWordWas("external") &&
		!(p.ForcedThisWordWas("layer") && p.ForcedThisWordWas("with") && p.ForcedThisPunctuationWas(':')) {
		return false
	}
	for !p.AtEndOfTokens() && !p.ThisWordIs("end") {
		p.skipping = false
		if !p.ListStatement() {
			p.SkipToNextList()
		}
	}
	return !p.ThisWordWas("end") ||
		/**/ p.ForcedThisPunctuationWas(';') &&
			/**/ p.ForcedEndOfStatement()
}

// ListStatement: "list" String "all"? Pattern+ AsCardsety ListBody
func (p *TExternalParser) ListStatement() bool {
	declaration := TListDeclaration{detailLevel: 2}
	return p.ForcedThisWordWas("list") &&
		/**/ p.ForcedThisString(&declaration.title, MessageListTitle) &&
		/**/ (p.ThisWordWas("all") || true) &&
		/**/ p.Patterns(&declaration) &&
		/**/ p.AsCardsety(&declaration) &&
		/**/ p.ListBody(&declaration) &&
		/**/ p.AppendDeclaration(declaration)
}

// Patterns: Pattern+, each a Text with its Underety.
func (p *TExternalParser) Patterns(declaration *TListDeclaration) bool {
	for p.ThisToken().Kind != TokEOF && !p.ThisWordIs("as") && !p.ThisWordIs("with") && !p.ThisPunctuationIs(';') {
		var text, under string
		if !(p.ForcedText(&text, MessageListPattern) && p.Underety(&under)) {
			return false
		}
		patterns := parseListPatterns(text)
		if len(patterns) != 1 {
			return p.Failed(ErrorNotA, text, MessageListPattern)
		}
		patterns[0].under = under
		declaration.patterns = append(declaration.patterns, patterns[0])
	}
	return len(declaration.patterns) > 0 ||
		p.Failed(ErrorExpectedGot, MessageListPattern, p.ThisTokenText())
}

// Underety: ( "under" Text )?
func (p *TExternalParser) Underety(under *string) bool {
	return p.ThisWordWas("under") &&
		/**/ p.ForcedText(under, MessagePathSegment) ||
		true
}

// AsCardsety: ( "as" "cards" )?
func (p *TExternalParser) AsCardsety(declaration *TListDeclaration) bool {
	return p.ThisWordWas("as") &&
		/**/ p.ForcedThisWordWas("cards") &&
		/**/ p.SetAsCards(declaration) ||
		true
}

// ListBody: "with" ":" ListOption* "end" ";" | ";"
func (p *TExternalParser) ListBody(declaration *TListDeclaration) bool {
	switch {
	// Its options.
	case p.ThisWordWas("with"):
		if !p.ForcedThisPunctuationWas(':') {
			return false
		}
		for !p.ThisWordIs("end") {
			if !p.ListOption(declaration) {
				return false
			}
		}
		return p.ForcedThisWordWas("end") &&
			/**/ p.ForcedThisPunctuationWas(';')

	// None.
	default:
		return p.ForcedThisPunctuationWas(';')
	}
}

// ListOption: ( "clean_prefix" | "clean_postfix" ) Text ";" | "detail_level" Number ";"
func (p *TExternalParser) ListOption(declaration *TListDeclaration) bool {
	var value string
	switch {
	// A part taken off the start of every name.
	case p.ThisWordWas("clean_prefix"):
		return p.ForcedText(&value, MessageNamePart) &&
			/**/ p.ForcedThisPunctuationWas(';') &&
			/**/ p.AppendCleanOp(declaration, "prefix", value)

	// A part taken off the end of every name.
	case p.ThisWordWas("clean_postfix"):
		return p.ForcedText(&value, MessageNamePart) &&
			/**/ p.ForcedThisPunctuationWas(';') &&
			/**/ p.AppendCleanOp(declaration, "postfix", value)

	// How much a card's graph shows.
	case p.ThisWordWas("detail_level"):
		return p.ForcedNumber(&declaration.detailLevel, 0, math.MaxInt, MessageDetailLevel) &&
			/**/ p.ForcedThisPunctuationWas(';')

	default:
		return p.Failed(ErrorExpectedGot, MessageListOption, p.ThisTokenText())
	}
}

// ForcedText: a run of adjacent tokens, up to a space or ";", into *target.
func (p *TExternalParser) ForcedText(target *string, what string) bool {
	token := p.ThisToken()
	if token.Kind != TokWord && !(token.Kind == TokPunctuation && token.Value != ";") {
		return p.Failed(ErrorExpectedGot, what, p.ThisTokenText())
	}
	*target = token.Value
	p.NextToken()
	for token = p.ThisToken(); !token.PrecededByWhitespace && (token.Kind == TokWord || token.Kind == TokPunctuation && token.Value != ";"); token = p.ThisToken() {
		*target += token.Value
		p.NextToken()
	}
	return true
}

// --- small helpers ---

// SkipToNextList skips what is left of a list that failed, its options and "end;" included.
func (p *TExternalParser) SkipToNextList() bool {
	for !p.AtEndOfTokens() && !p.ThisWordIs("list") {
		p.NextToken()
	}
	return true
}

// SetAsCards marks declaration as rendered as cards.
func (p *TExternalParser) SetAsCards(declaration *TListDeclaration) bool {
	declaration.asCards = true
	return true
}

// AppendCleanOp adds a clean operation to declaration.
func (p *TExternalParser) AppendCleanOp(declaration *TListDeclaration, kind, value string) bool {
	declaration.cleanOps = append(declaration.cleanOps, TListCleanOp{kind, value})
	return true
}

// AppendDeclaration adds a parsed list.
func (p *TExternalParser) AppendDeclaration(declaration TListDeclaration) bool {
	p.Declarations = append(p.Declarations, declaration)
	return true
}
