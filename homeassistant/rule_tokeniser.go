/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: RuleTokeniser
 *
 * The middle layer of this DSL frontend's three-layer parser architecture (GO_CONVENTIONS.md §7),
 * embedding TDefCharacterStream (rule_character_stream.go). Classifies the rule grammar's byte
 * stream into three kinds of token: a run of identifier characters (TRuleTokenWord -- covers both
 * grammar keywords like "when"/"turn" and DSL identifier segments like entity leaves/sphere
 * names, which are lexically identical here; the parser layer, not this one, decides which
 * keyword a given word means), a single punctuation byte (TRuleTokenPunct: '.', ':', '/', ';',
 * ','), and a double-quoted string (TRuleTokenString -- the "jinja \"...\"" raw-template
 * pass-through convention Logical.def's own "defined"/"derived" capabilities already use is the
 * only real need for this). See rule_character_stream.go's own doc comment for why this layer
 * exists.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 12.10.2026
 *
 */

package main

// TRuleTokenKind classifies one TRuleToken.
type TRuleTokenKind int

const (
	TRuleTokenEOF TRuleTokenKind = iota
	TRuleTokenWord
	TRuleTokenPunct
	TRuleTokenString
)

// TRuleToken is one classified token: Kind plus its raw text (a single byte for TRuleTokenPunct,
// empty for TRuleTokenEOF). PrecededByWhitespace is true unless this token immediately follows
// the previous one with no space/newline in between -- the only way EntityRef (rule_parser.go)
// can tell "<sphere>:<path>" (a non-empty path starting right after the colon) apart from
// "<sphere>:" (an empty-path reference, e.g. "vacuum.social:") followed by an ordinary word
// token like a predicate keyword. A real bug, found live 2026-10-13: without this, "vacuum.social:
// turns error;" silently swallowed "turns" as if it were the (non-existent) path.
type TRuleToken struct {
	Kind                 TRuleTokenKind
	Value                string
	PrecededByWhitespace bool
}

// ruleIdentifierCharacters are the bytes TDefTokeniser folds into one TRuleTokenWord -- this
// DSL's own entity-leaf/sphere-name character set elsewhere (letters, digits, underscore,
// hyphen).
const ruleIdentifierCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-"

// TDefTokeniser embeds TDefCharacterStream (chaining the layers, per GO_CONVENTIONS.md §7) and
// classifies its byte stream into TRuleToken values.
type TDefTokeniser struct {
	TDefCharacterStream
	current TRuleToken
}

// Initialise sets up the tokeniser over source and classifies the first token.
func (t *TDefTokeniser) Initialise(source string) {
	t.TDefCharacterStream.Initialise(source)
	t.NextToken()
}

// ThisToken returns the currently selected token.
func (t *TDefTokeniser) ThisToken() TRuleToken {
	return t.current
}

// NextToken skips whitespace, then classifies and selects the next token. Always returns true --
// end-of-stream is its own token kind (TRuleTokenEOF), not a failure to be propagated.
func (t *TDefTokeniser) NextToken() bool {
	// Captured before skipping: the character sitting right after the PREVIOUS token ended is
	// whitespace if and only if at least one space/newline separates it from this next one --
	// see TRuleToken.PrecededByWhitespace's own doc comment for why this matters.
	precededByWhitespace := t.ThisCharacterIsIn(" \t\n\r")
	t.SkipWhiteSpace()

	if t.EndOfStream() {
		t.current = TRuleToken{Kind: TRuleTokenEOF, PrecededByWhitespace: precededByWhitespace}
		return true
	}

	if t.ThisCharacterIsIn(ruleIdentifierCharacters) {
		word := ""
		for t.CollectCharacterThatWasIn(ruleIdentifierCharacters, &word) {
		}
		t.current = TRuleToken{Kind: TRuleTokenWord, Value: word, PrecededByWhitespace: precededByWhitespace}
		return true
	}

	if t.ThisCharacterIs('"') {
		t.NextCharacter()
		text := ""
		for !t.ThisCharacterIs('"') && !t.EndOfStream() {
			text += string(t.ThisCharacter())
			t.NextCharacter()
		}
		t.NextCharacter() // past the closing '"' -- an unterminated string just runs to EOF, same
		// "no error state in this layer" convention end-of-stream itself already follows.
		t.current = TRuleToken{Kind: TRuleTokenString, Value: text, PrecededByWhitespace: precededByWhitespace}
		return true
	}

	ch := t.ThisCharacter()
	t.NextCharacter()
	t.current = TRuleToken{Kind: TRuleTokenPunct, Value: string(ch), PrecededByWhitespace: precededByWhitespace}
	return true
}

// AtEndOfTokens reports whether the current token is the end-of-stream marker.
func (t *TDefTokeniser) AtEndOfTokens() bool {
	return t.current.Kind == TRuleTokenEOF
}

// ThisWordIs reports whether the current token is a word with exactly this value. Every rule-
// grammar keyword and DSL identifier segment is already lower-case, so this is a plain
// case-sensitive comparison.
func (t *TDefTokeniser) ThisWordIs(word string) bool {
	return t.current.Kind == TRuleTokenWord && t.current.Value == word
}

// ThisWordWas reports ThisWordIs, and if so, advances to the next token.
func (t *TDefTokeniser) ThisWordWas(word string) bool {
	return t.ThisWordIs(word) && t.NextToken()
}

// ThisWord reports whether the current token is any word, copies its value into *target, and
// advances to the next token.
func (t *TDefTokeniser) ThisWord(target *string) bool {
	if t.current.Kind != TRuleTokenWord {
		return false
	}
	*target = t.current.Value
	return t.NextToken()
}

// ThisString reports whether the current token is a string literal, copies its (unquoted)
// content into *target, and advances to the next token.
func (t *TDefTokeniser) ThisString(target *string) bool {
	if t.current.Kind != TRuleTokenString {
		return false
	}
	*target = t.current.Value
	return t.NextToken()
}

// ThisPunctuationIs reports whether the current token is exactly this single punctuation byte.
func (t *TDefTokeniser) ThisPunctuationIs(ch byte) bool {
	return t.current.Kind == TRuleTokenPunct && t.current.Value == string(ch)
}

// ThisPunctuationWas reports ThisPunctuationIs, and if so, advances to the next token.
func (t *TDefTokeniser) ThisPunctuationWas(ch byte) bool {
	return t.ThisPunctuationIs(ch) && t.NextToken()
}
