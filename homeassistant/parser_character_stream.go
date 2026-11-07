/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ParserCharacterStream
 *
 * The character layer of the definition parsers (GO_CONVENTIONS.md §7): streams over a source
 * text one byte at a time, keeping the line and column for messages, with the This…Is/This…Was/
 * Collect… primitives over characters and character sets. The definitions are plain ASCII.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Vienna, Austria, in collaboration with Claude.ai
 *
 * Version of: 07.11.2026
 *
 */

package main

// TByteSet is a set of characters, built once (parser_tokeniser.go's init).
type TByteSet [256]bool

// TCharacterStream streams over source; line is 1-based, column 0-based.
type TCharacterStream struct {
	source      []byte
	position    int
	endOfStream bool
	current     byte
	line        int
	column      int
}

// --- sets ---

// newByteSet returns the set of the characters in characters.
func newByteSet(characters string) TByteSet {
	var set TByteSet
	for i := 0; i < len(characters); i++ {
		set[characters[i]] = true
	}
	return set
}

// Contains reports whether ch is in s.
func (s *TByteSet) Contains(ch byte) bool {
	return s[ch]
}

// --- streaming ---

// Initialise sets s up over source, at its first character.
func (s *TCharacterStream) Initialise(source string) {
	s.source = []byte(source)
	s.position = 0
	s.line = 1
	s.column = 0
	s.endOfStream = len(s.source) == 0
	if !s.endOfStream {
		s.current = s.source[0]
	}
}

// EndOfStream reports whether every character has been read.
func (s *TCharacterStream) EndOfStream() bool {
	return s.endOfStream
}

// ThisCharacter returns the current character; undefined at the end of the stream.
func (s *TCharacterStream) ThisCharacter() byte {
	return s.current
}

// Position returns the current line and column.
func (s *TCharacterStream) Position() (line, column int) {
	return s.line, s.column
}

// NextCharacter moves to the next character, and reports whether there is one.
func (s *TCharacterStream) NextCharacter() bool {
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

// --- primitives ---

// ThisCharacterIs reports whether the current character is ch.
func (s *TCharacterStream) ThisCharacterIs(ch byte) bool {
	return !s.endOfStream && s.current == ch
}

// ThisCharacterWas consumes the current character if it is ch. At the last character it still
// reports the match.
func (s *TCharacterStream) ThisCharacterWas(ch byte) bool {
	return s.ThisCharacterIs(ch) && (s.NextCharacter() || true)
}

// ThisCharacterIsIn reports whether the current character is in set.
func (s *TCharacterStream) ThisCharacterIsIn(set TByteSet) bool {
	return !s.endOfStream && set.Contains(s.current)
}

// ThisCharacterWasIn consumes the current character if it is in set.
func (s *TCharacterStream) ThisCharacterWasIn(set TByteSet) bool {
	return s.ThisCharacterIsIn(set) && (s.NextCharacter() || true)
}

// CollectCharacterThatWasIn appends the current character to *into and consumes it, if it is in
// set.
func (s *TCharacterStream) CollectCharacterThatWasIn(set TByteSet, into *string) bool {
	return s.ThisCharacterIsIn(set) && s.collectCharacter(into)
}

// CollectCharacterThatWasNot appends the current character to *into and consumes it, unless it
// is ch or the stream has ended.
func (s *TCharacterStream) CollectCharacterThatWasNot(ch byte, into *string) bool {
	return !s.endOfStream && s.current != ch && s.collectCharacter(into)
}

// collectCharacter appends the current character to *into and consumes it.
func (s *TCharacterStream) collectCharacter(into *string) bool {
	*into += string(s.current)
	s.NextCharacter()
	return true
}
