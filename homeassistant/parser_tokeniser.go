/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ParserTokeniser
 *
 * The token layer of the definition parsers (GO_CONVENTIONS.md §7), embedding TCharacterStream.
 * A token is a word (a run of letters, digits, "_" and "-": keywords, identifiers and numbers
 * alike, the grammar decides which), a string ("…" or '…'), or a single punctuation character.
 * Each token records whether whitespace preceded it, so that the grammar can join names written
 * without spaces, such as "sensor.wind_speed" (word, ".", word).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Vienna, Austria, in collaboration with Claude.ai
 *
 * Version of: 07.11.2026
 *
 */

package main

// TTokenKind classifies a TToken.
type TTokenKind int

const (
	TokEOF          TTokenKind = iota
	TokWord                    // keywords, identifiers and numbers
	TokString                  // the text between the quotes
	TokPunctuation             // one character
	TokUnterminated            // a string without its closing quote, running to the end
)

// TToken is one token, starting at Line and Column. Value is empty for TokEOF.
type TToken struct {
	Kind                 TTokenKind
	Value                string
	PrecededByWhitespace bool
	Line, Column         int
}

// TTokeniser embeds TCharacterStream and classifies one token ahead.
type TTokeniser struct {
	TCharacterStream // the characters being tokenised
	current          TToken
}

// The character classes of the definitions.
var (
	WordCharacters,
	WhiteSpaceCharacters,
	QuoteCharacters TByteSet
)

func init() {
	WordCharacters = newByteSet("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-")
	WhiteSpaceCharacters = newByteSet(" \t\n\r")
	QuoteCharacters = newByteSet(`"'`)
}

// Initialise sets t up over source, at its first token.
func (t *TTokeniser) Initialise(source string) {
	t.TCharacterStream.Initialise(source)
	t.NextToken()
}

// ThisToken returns the current token.
func (t *TTokeniser) ThisToken() TToken {
	return t.current
}

// NextToken skips whitespace and classifies the next token. It always returns true: the end of
// the source is a token of its own.
func (t *TTokeniser) NextToken() bool {
	precededByWhitespace := t.ThisCharacterIsIn(WhiteSpaceCharacters)
	for t.ThisCharacterWasIn(WhiteSpaceCharacters) {
		// Skip
	}
	t.current = TToken{PrecededByWhitespace: precededByWhitespace}
	t.current.Line, t.current.Column = t.Position()

	switch {
	// The end of the source.
	case t.EndOfStream():
		t.current.Kind = TokEOF

	// A word.
	case t.ThisCharacterIsIn(WordCharacters):
		t.current.Kind = TokWord
		for t.CollectCharacterThatWasIn(WordCharacters, &t.current.Value) {
			// Skip
		}

	// A string, up to the same quote it opened with.
	case t.ThisCharacterIsIn(QuoteCharacters):
		quote := t.ThisCharacter()
		t.NextCharacter()
		for t.CollectCharacterThatWasNot(quote, &t.current.Value) {
			// Skip
		}
		if t.ThisCharacterWas(quote) {
			t.current.Kind = TokString
		} else {
			t.current.Kind = TokUnterminated
		}

	// Any other character.
	default:
		t.current.Kind = TokPunctuation
		t.current.Value = string(t.ThisCharacter())
		t.NextCharacter()
	}
	return true
}

// --- primitives ---

// AtEndOfTokens reports whether the current token is the end of the source.
func (t *TTokeniser) AtEndOfTokens() bool {
	return t.current.Kind == TokEOF
}

// ThisWordIs reports whether the current token is the word word.
func (t *TTokeniser) ThisWordIs(word string) bool {
	return t.current.Kind == TokWord && t.current.Value == word
}

// ThisWordWas consumes the current token if it is the word word.
func (t *TTokeniser) ThisWordWas(word string) bool {
	return t.ThisWordIs(word) && t.NextToken()
}

// ThisWord consumes the current token if it is a word, copying it into *target.
func (t *TTokeniser) ThisWord(target *string) bool {
	return t.current.Kind == TokWord && t.copyToken(target)
}

// ThisAdjacentWord is ThisWord for a word written right after the previous token.
func (t *TTokeniser) ThisAdjacentWord(target *string) bool {
	return !t.current.PrecededByWhitespace && t.ThisWord(target)
}

// ThisString consumes the current token if it is a string, copying its text into *target.
func (t *TTokeniser) ThisString(target *string) bool {
	return t.current.Kind == TokString && t.copyToken(target)
}

// ThisPunctuationIs reports whether the current token is the punctuation ch.
func (t *TTokeniser) ThisPunctuationIs(ch byte) bool {
	return t.current.Kind == TokPunctuation && t.current.Value[0] == ch
}

// ThisPunctuationWas consumes the current token if it is the punctuation ch.
func (t *TTokeniser) ThisPunctuationWas(ch byte) bool {
	return t.ThisPunctuationIs(ch) && t.NextToken()
}

// ThisAdjacentPunctuationWas is ThisPunctuationWas for punctuation written right after the
// previous token.
func (t *TTokeniser) ThisAdjacentPunctuationWas(ch byte) bool {
	return !t.current.PrecededByWhitespace && t.ThisPunctuationWas(ch)
}

// copyToken copies the current token's value into *target and moves to the next token.
func (t *TTokeniser) copyToken(target *string) bool {
	*target = t.current.Value
	return t.NextToken()
}
