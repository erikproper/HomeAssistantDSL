/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Tokeniser
 *
 * Provides a rune-by-rune character stream and a token-level stream for parsing .def files.
 * Comments begin with '#' and extend to end-of-line; newlines are treated as whitespace.
 * Architecture mirrors the BiBTeX tokenizer in bibtex_check (TCharacterStream / TBibTeXStream).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 25.03.2026
 *
 */

package main

import (
	"bufio"
	"fmt"
	"os"
)

// --- TByteSet: compact bit-vector character set ---
//
// Represents a set of bytes (ASCII characters) as four 64-bit words.
// Membership tests and union operations run in O(1) constant time.

type TByteSet struct {
	words [4]uint64
}

// Add adds one or more bytes to the set.
func (s *TByteSet) Add(elements ...byte) *TByteSet {
	for _, b := range elements {
		s.words[b/64] |= 1 << (b % 64)
	}
	return s
}

// AddString adds every byte in str to the set.
func (s *TByteSet) AddString(str string) *TByteSet {
	return s.Add([]byte(str)...)
}

// Unite adds all elements of t into s.
func (s *TByteSet) Unite(t TByteSet) *TByteSet {
	s.words[0] |= t.words[0]
	s.words[1] |= t.words[1]
	s.words[2] |= t.words[2]
	s.words[3] |= t.words[3]
	return s
}

// Contains returns true when b is a member of the set.
func (s TByteSet) Contains(b byte) bool {
	return (s.words[b/64] & (1 << (b % 64))) != 0
}

// --- Named character constants ---

const (
	charNewline   byte = '\n'
	charCR        byte = '\r'
	charTab       byte = '\t'
	charSpace     byte = ' '
	charHash      byte = '#'
	charDollar    byte = '$'
	charLBrace    byte = '{'
	charRBrace    byte = '}'
	charLParen    byte = '('
	charRParen    byte = ')'
	charSemicolon byte = ';'
	charComma     byte = ','
	charColon     byte = ':'
	charEquals    byte = '='
	charAt        byte = '@'
	charExclam    byte = '!'
	charAsterisk  byte = '*'
	charDot       byte = '.'
	charSlash     byte = '/'
	charLBracket  byte = '['
	charRBracket  byte = ']'
)

// --- Token kinds ---

type TTokenKind int

const (
	TokenKindNone      TTokenKind = iota
	TokenKindWord                 // letters, digits, _ only
	TokenKindPath                 // word chars + . / : ! [ ] + embedded ${...}
	TokenKindVariable             // ${name} or ${name.accessor} standing alone
	TokenKindComment              // # rest-of-line (standalone at logical line start)
	TokenKindSemicolon            // ;
	TokenKindComma                // ,
	TokenKindLParen               // (
	TokenKindRParen               // )
	TokenKindLBrace               // { (bare, not preceded by $)
	TokenKindRBrace               // }
	TokenKindColon                // : (standalone delimiter)
	TokenKindEquals               // =
	TokenKindEOS                  // end of stream
)

// --- Character sets (initialised in init) ---

var (
	DefHSpaceChars  TByteSet // horizontal whitespace: space, tab, CR
	DefWordStarters TByteSet // letters + _
	DefWordChars    TByteSet // letters + digits + _
	DefPathChars    TByteSet // word chars + . / [ ] (: handled with one-step lookahead)
	DefVarNameChars TByteSet // word chars + .  (for ${name.accessor} notation)
)

// --- TDefCharacterStream: rune-level character stream ---
//
// Reads characters from a .def file one rune at a time via bufio.Scanner.
// Tracks line number and column position for error messages.

type TDefCharacterStream struct {
	filePath     string
	file         *os.File
	scanner      *bufio.Scanner
	fileIsOpen   bool
	runes        []rune
	runePos      int
	endOfStream  bool
	currentChar  byte
	linePosition int // current line (1-based)
	runePosition int // current column (1-based)
}

// initStream sets the character stream to its initial state.
func (c *TDefCharacterStream) initStream() {
	c.endOfStream = false
	c.runes = nil
	c.runePos = 0
	c.currentChar = charSpace
	c.linePosition = 1
	c.runePosition = 0
}

// OpenFile opens path as the character source and primes the first character.
// Returns false when the file cannot be opened.
func (c *TDefCharacterStream) OpenFile(path string) bool {
	var err error
	c.filePath = path
	c.file, err = os.Open(path)
	if err != nil {
		c.endOfStream = true
		return false
	}
	c.fileIsOpen = true
	c.initStream()
	c.scanner = bufio.NewScanner(c.file)
	return c.NextCharacter()
}

// CloseFile closes the underlying file if it is open.
func (c *TDefCharacterStream) CloseFile() {
	if c.fileIsOpen {
		c.file.Close()
		c.fileIsOpen = false
	}
	c.endOfStream = true
}

// EndOfStream returns true when no further characters are available.
func (c *TDefCharacterStream) EndOfStream() bool {
	return c.endOfStream
}

// ThisCharacter returns the current character.
func (c *TDefCharacterStream) ThisCharacter() byte {
	return c.currentChar
}

// NextCharacter advances to the next character in the stream.
// Returns true when a character was successfully read.
func (c *TDefCharacterStream) NextCharacter() bool {
	if c.endOfStream {
		return false
	}
	if c.runePos < len(c.runes) {
		// Take the next rune from the already-loaded buffer.
		r := c.runes[c.runePos]
		c.runePos++
		c.currentChar = byte(r) // .def files use only ASCII
		c.runePosition++
		if c.currentChar == charNewline {
			c.linePosition++
			c.runePosition = 0
		}
		return true
	}
	if c.fileIsOpen && c.scanner.Scan() {
		// Load the next line from the file; append '\n' so newlines pass through.
		c.runes = []rune(c.scanner.Text() + "\n")
		c.runePos = 0
		return c.NextCharacter()
	}
	c.endOfStream = true
	return false
}

// ThisCharacterIs returns true when the current character equals ch.
func (c *TDefCharacterStream) ThisCharacterIs(ch byte) bool {
	return !c.endOfStream && c.currentChar == ch
}

// ThisCharacterIsIn returns true when the current character is in set s.
func (c *TDefCharacterStream) ThisCharacterIsIn(s TByteSet) bool {
	return !c.endOfStream && s.Contains(c.currentChar)
}

// CollectCharacterThatWasIn appends the current character to out when it is in s, then advances.
func (c *TDefCharacterStream) CollectCharacterThatWasIn(s TByteSet, out *string) bool {
	if c.ThisCharacterIsIn(s) {
		*out += string(c.currentChar)
		return c.NextCharacter()
	}
	return false
}

// PositionReport returns a position annotation for use in error messages.
func (c *TDefCharacterStream) PositionReport() string {
	if c.endOfStream {
		return fmt.Sprintf(" (in %s, at end)", c.filePath)
	}
	return fmt.Sprintf(" (in %s, L:%d, C:%d)", c.filePath, c.linePosition, c.runePosition)
}

// --- TDefTokeniser: token-level stream built on TDefCharacterStream ---
//
// Produces typed tokens by calling NextToken(expectedKind).
// Comments at the logical start of a line are emitted as TokenKindComment tokens;
// inline comments (after other tokens on the same line) are silently skipped.
// ':' in entity paths is resolved with one-step lookahead via pushToken.
//
// CDL1 convention (same as bibtex_check):
//   A() && B()  — A then B (sequence)
//   A() || B()  — A or else B (alternative)
//   ForcedX()   — X is required; reports error on failure
//   Xety()      — X or empty (optional)

type TDefTokeniser struct {
	TDefCharacterStream
	tokenValue  string     // value of the current (buffered) token
	tokenKind   TTokenKind // kind of the current (buffered) token
	pushedValue string     // lookahead pushed-back token value
	pushedKind  TTokenKind // lookahead pushed-back token kind
	hasPushed   bool       // whether a pushed-back token is waiting
	atLineStart bool       // true when only whitespace has been seen since the last newline
	errors      []string   // accumulated tokeniser errors
}

// ReportTokenError records an error message with source position.
func (t *TDefTokeniser) ReportTokenError(format string, args ...any) bool {
	t.errors = append(t.errors, fmt.Sprintf(format, args...)+t.PositionReport())
	return false
}

// ThisToken returns the current token's string value.
func (t *TDefTokeniser) ThisToken() string { return t.tokenValue }

// ThisTokenIs returns true when the current token has the given kind.
func (t *TDefTokeniser) ThisTokenIs(kind TTokenKind) bool { return t.tokenKind == kind }

// pushToken stores a single token to be returned by the next NextToken call.
// Used to resolve the ':' lookahead in collectPathToken without backtracking.
func (t *TDefTokeniser) pushToken(value string, kind TTokenKind) {
	t.pushedValue = value
	t.pushedKind = kind
	t.hasPushed = true
}

// NextToken fetches the next token from the stream.
// expectedKind controls whether a word-like token is collected as TokenKindWord (letters+digits+_)
// or TokenKindPath (greedy, includes . / [ ] and embedded ${...}).
// Delimiter tokens (;  ,  (  )  {  }  :) are always returned regardless of expectedKind.
// Returns true when a non-EOS token was produced.
func (t *TDefTokeniser) NextToken(expectedKind TTokenKind) bool {
	// Return a pushed-back token first (produced by the ':' lookahead resolution).
	if t.hasPushed {
		t.tokenValue = t.pushedValue
		t.tokenKind = t.pushedKind
		t.hasPushed = false
		return t.tokenKind != TokenKindEOS
	}

	t.tokenValue = ""
	t.tokenKind = TokenKindNone

	// Skip horizontal whitespace and newlines, tracking whether we are at the start of a
	// logical line.  A standalone '#' (only whitespace before it on this line) produces a
	// TokenKindComment token.  An inline '#' (something else on this line already) is
	// silently consumed up to the end of the line.
	for {
		// Skip horizontal whitespace (space, tab, CR) without changing atLineStart.
		for t.ThisCharacterIsIn(DefHSpaceChars) {
			t.NextCharacter()
		}

		if t.EndOfStream() {
			t.tokenKind = TokenKindEOS
			return false
		}

		if t.ThisCharacterIs(charNewline) {
			t.NextCharacter()
			t.atLineStart = true
			continue
		}

		if t.ThisCharacterIs(charHash) {
			if t.atLineStart {
				// Standalone comment: collect '#' through end-of-line as one token.
				text := ""
				for !t.EndOfStream() && !t.ThisCharacterIs(charNewline) {
					text += string(t.ThisCharacter())
					t.NextCharacter()
				}
				t.tokenValue = text
				t.tokenKind = TokenKindComment
				// atLineStart stays true: the next real token is on a new line.
				return true
			}
			// Inline comment: skip to end-of-line and continue the whitespace loop.
			for !t.EndOfStream() && !t.ThisCharacterIs(charNewline) {
				t.NextCharacter()
			}
			t.atLineStart = true
			continue
		}

		// Non-whitespace, non-comment character: fall through to token collection.
		break
	}

	// Any real (non-comment) token clears the line-start flag.
	t.atLineStart = false

	ch := t.ThisCharacter()

	// Single-character delimiter tokens — returned regardless of expectedKind.
	switch ch {
	case charSemicolon:
		t.tokenValue, t.tokenKind = ";", TokenKindSemicolon
		t.NextCharacter()
		return true
	case charComma:
		t.tokenValue, t.tokenKind = ",", TokenKindComma
		t.NextCharacter()
		return true
	case charLParen:
		t.tokenValue, t.tokenKind = "(", TokenKindLParen
		t.NextCharacter()
		return true
	case charRParen:
		t.tokenValue, t.tokenKind = ")", TokenKindRParen
		t.NextCharacter()
		return true
	case charLBrace:
		// Bare '{' (not preceded by '$') is a block delimiter.
		t.tokenValue, t.tokenKind = "{", TokenKindLBrace
		t.NextCharacter()
		return true
	case charRBrace:
		t.tokenValue, t.tokenKind = "}", TokenKindRBrace
		t.NextCharacter()
		return true
	case charColon:
		// Standalone ':' delimiter (see collectPathToken for embedded ':' handling).
		t.tokenValue, t.tokenKind = ":", TokenKindColon
		t.NextCharacter()
		return true
	case charEquals:
		t.tokenValue, t.tokenKind = "=", TokenKindEquals
		t.NextCharacter()
		return true
	}

	// Quoted string: "..." — collected as one opaque token (kind TokenKindPath).
	if ch == '"' {
		return t.collectQuotedStringToken()
	}

	// In a path context, '$' starts an inline variable reference that may be followed by more
	// path characters (e.g. ${entity}/${node}); route to collectPathToken which handles this.
	// In a word context, '$' starts a standalone variable token.
	if ch == charDollar {
		if expectedKind == TokenKindPath {
			return t.collectPathToken()
		}
		return t.collectVariableToken()
	}

	// Word or path token.
	// Paths may start with any DefPathChars character (includes '.', '/', '!'),
	// not just word-starters, to handle relative paths like ../../foo.
	if t.ThisCharacterIsIn(DefWordStarters) || (expectedKind == TokenKindPath && t.ThisCharacterIsIn(DefPathChars)) {
		if expectedKind == TokenKindPath {
			return t.collectPathToken()
		}
		return t.collectWordToken()
	}

	t.ReportTokenError("unexpected character %q", string(ch))
	t.NextCharacter()
	return false
}

// collectWordToken collects a sequence of word characters (letters, digits, _)
// and returns it as a TokenKindWord token.
func (t *TDefTokeniser) collectWordToken() bool {
	value := ""
	for t.CollectCharacterThatWasIn(DefWordChars, &value) {
	}
	t.tokenValue = value
	t.tokenKind = TokenKindWord
	return value != ""
}

// collectPathToken greedily collects a path token (word chars + . / [ ] + embedded ${...}).
// ':' is included only when immediately followed by another path character; otherwise it is
// pushed back as a standalone TokenKindColon for the next NextToken call.
func (t *TDefTokeniser) collectPathToken() bool {
	value := ""
	for {
		// Word/path characters (excluding ':').
		if t.CollectCharacterThatWasIn(DefPathChars, &value) {
			continue
		}

		// ':' with one-step lookahead: include in path only when followed by a path character,
		// so that 'social:apartment' is one token but 'macro name:' ends at the word.
		if t.ThisCharacterIs(charColon) {
			t.NextCharacter() // advance past ':'
			if !t.EndOfStream() && (t.ThisCharacterIsIn(DefWordStarters) ||
				t.ThisCharacterIsIn(DefPathChars) ||
				t.ThisCharacterIs(charDollar)) {
				value += ":"
				// Current character is the first char of the path segment after ':'; loop continues.
				continue
			}
			// ':' is not followed by a path character — treat it as a standalone delimiter.
			// Push it back so the next NextToken call returns it.
			t.pushToken(":", TokenKindColon)
			break
		}

		// Embedded variable reference ${...} within a path template.
		if t.ThisCharacterIs(charDollar) {
			varValue := ""
			if t.collectVariableInline(&varValue) {
				value += varValue
				continue
			}
			break
		}

		break
	}
	t.tokenValue = value
	t.tokenKind = TokenKindPath
	return value != ""
}

// collectVariableToken collects a standalone ${name} or ${name.accessor} token.
// On entry the current character must be '$'.
func (t *TDefTokeniser) collectVariableToken() bool {
	t.NextCharacter() // advance past '$'
	if t.EndOfStream() || !t.ThisCharacterIs(charLBrace) {
		t.ReportTokenError("expected '{' after '$'")
		return false
	}
	t.NextCharacter() // advance past '{'
	name := ""
	for t.CollectCharacterThatWasIn(DefVarNameChars, &name) {
	}
	if !t.ThisCharacterIs(charRBrace) {
		t.ReportTokenError("expected '}' to close variable ${%s", name)
		return false
	}
	t.NextCharacter() // advance past '}'
	t.tokenValue = "${" + name + "}"
	t.tokenKind = TokenKindVariable
	return true
}

// collectVariableInline collects a ${...} reference inline within a path token,
// appending the full "${name}" string to out without setting tokenValue/tokenKind.
// On entry the current character must be '$'.
func (t *TDefTokeniser) collectVariableInline(out *string) bool {
	t.NextCharacter() // advance past '$'
	if t.EndOfStream() || !t.ThisCharacterIs(charLBrace) {
		// Bare '$' with no '{': append as literal and continue.
		*out += "$"
		return true
	}
	t.NextCharacter() // advance past '{'
	name := ""
	for t.CollectCharacterThatWasIn(DefVarNameChars, &name) {
	}
	if !t.ThisCharacterIs(charRBrace) {
		t.ReportTokenError("expected '}' in inline variable reference")
		return false
	}
	t.NextCharacter() // advance past '}'
	*out += "${" + name + "}"
	return true
}

// collectQuotedStringToken collects a double-quoted string as one token of kind TokenKindPath.
// On entry the current character must be '"'.
// Collecting stops at the closing '"' or at end-of-line (whichever comes first).
func (t *TDefTokeniser) collectQuotedStringToken() bool {
	value := "\""
	t.NextCharacter() // advance past opening '"'
	for !t.EndOfStream() && !t.ThisCharacterIs('"') && !t.ThisCharacterIs(charNewline) {
		value += string(t.ThisCharacter())
		t.NextCharacter()
	}
	if t.ThisCharacterIs('"') {
		value += "\""
		t.NextCharacter() // advance past closing '"'
	}
	t.tokenValue = value
	t.tokenKind = TokenKindPath
	return true
}

// --- Forced variants: report error and skip on failure, returning false ---

// --- Initialisation ---

func init() {
	// Horizontal whitespace: space, tab, carriage return (not newline — handled separately).
	DefHSpaceChars.AddString(" \t\r")

	// Word starters: letters and underscore.
	DefWordStarters.AddString("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_")

	// Word characters: starters plus digits.
	DefWordChars.Unite(DefWordStarters).AddString("0123456789")

	// Path characters: word chars plus '.', '/', '!', '*', '@', '[', ']'.
	// ':' is intentionally excluded and handled with one-step lookahead in collectPathToken.
	// '!' is included for entity attribute access notation (entity!attribute).
	// '*' is included for wildcard entity patterns used in list definitions.
	// '@' is included for aggregation tokens (@light, @all, @media) in space-off/light-on lines.
	DefPathChars.Unite(DefWordChars).Add(charDot, charSlash, charExclam, charAsterisk, charAt, charLBracket, charRBracket)

	// Variable name characters: word chars plus '.' for accessor notation (${x.domain}).
	DefVarNameChars.Unite(DefWordChars).Add(charDot)
}
