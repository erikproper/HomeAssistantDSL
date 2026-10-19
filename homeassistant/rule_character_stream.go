/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: RuleCharacterStream
 *
 * The innermost layer of this DSL frontend's three-layer parser architecture (GO_CONVENTIONS.md
 * §7: character stream -> tokeniser -> recursive-descent parser). TDefCharacterStream reads a
 * source string one byte at a time, tracking line/column for error reporting. PROJECT.md item 13
 * (migrating manual HA automations into a real "rule" grammar) is the first user of this layer —
 * a deliberate choice, not a rule-specific one: `feedback_no_new_regexp_parsers` flags every
 * ad hoc regexp/strings.Fields-based Conceptual.def/Physical.def/Logical.def parser this codebase
 * has accumulated as debt the user wants fixed by a dedicated future pass (PROJECT.md item 18),
 * not patched piecemeal — so brand new grammar surface (this item's own rich trigger/condition/
 * action grammar, far beyond what the old ad hoc "rule ... when ... then ...;" one-liner covered)
 * gets built on the real architecture from the start instead of adding to that pile. Nothing here
 * is specific to rules; item 18's eventual rewrite of the other integration parsers can embed this
 * same type.
 *
 * Deliberately narrower than ../bibtex/bibtex_check's own TCharacterStream (character_stream.go),
 * which also supports streaming from a file and remapping non-ASCII runes to TeX escapes: a rule
 * block's body text is already fully read into memory by Conceptual.def's own outer line-based
 * scanner (parser.go) before being handed here, so file-backed streaming is never needed, and this
 * DSL's own grammar is plain ASCII throughout.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 12.10.2026
 *
 */

package main

// TDefCharacterStream streams over source one byte at a time, tracking line/column (1-based line,
// 0-based column within that line) for error reporting by layers built on top of it.
type TDefCharacterStream struct {
	source      []byte
	position    int
	endOfStream bool
	current     byte
	line        int
	column      int
}

// Initialise sets s up to stream over source, positioned at its first byte -- or immediately at
// end-of-stream, for an empty source.
func (s *TDefCharacterStream) Initialise(source string) {
	s.source = []byte(source)
	s.position = 0
	s.line = 1
	s.column = 0
	s.endOfStream = len(s.source) == 0
	if !s.endOfStream {
		s.current = s.source[0]
	}
}

// EndOfStream reports whether every byte of source has been consumed.
func (s *TDefCharacterStream) EndOfStream() bool {
	return s.endOfStream
}

// ThisCharacter returns the currently selected byte. Its value is undefined at end-of-stream.
func (s *TDefCharacterStream) ThisCharacter() byte {
	return s.current
}

// Position reports the current line (1-based) and column (0-based) in source, for error messages.
func (s *TDefCharacterStream) Position() (line, column int) {
	return s.line, s.column
}

// NextCharacter advances to the next byte, tracking line/column, and reports whether a next byte
// was actually found (false at end-of-stream).
func (s *TDefCharacterStream) NextCharacter() bool {
	if s.endOfStream {
		return false
	}
	if s.current == '\n' {
		s.line++
		s.column = 0
	} else {
		s.column++
	}
	s.position++
	if s.position >= len(s.source) {
		s.endOfStream = true
		return false
	}
	s.current = s.source[s.position]
	return true
}

// ThisCharacterIs reports whether the current byte equals ch.
func (s *TDefCharacterStream) ThisCharacterIs(ch byte) bool {
	return !s.endOfStream && s.current == ch
}

// ThisCharacterIsIn reports whether the current byte occurs anywhere in set.
func (s *TDefCharacterStream) ThisCharacterIsIn(set string) bool {
	if s.endOfStream {
		return false
	}
	for i := 0; i < len(set); i++ {
		if set[i] == s.current {
			return true
		}
	}
	return false
}

// CollectCharacterThatWasIn appends the current byte to *into and advances past it, if that byte
// occurs in set.
func (s *TDefCharacterStream) CollectCharacterThatWasIn(set string, into *string) bool {
	if !s.ThisCharacterIsIn(set) {
		return false
	}
	*into += string(s.current)
	return s.NextCharacter()
}

// SkipWhiteSpace advances past every space/tab/newline/carriage-return at the current position.
// Always returns true -- "zero or more" never fails.
func (s *TDefCharacterStream) SkipWhiteSpace() bool {
	for s.ThisCharacterIsIn(" \t\n\r") {
		s.NextCharacter()
	}
	return true
}
