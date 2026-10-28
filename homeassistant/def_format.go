/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: DefFormat
 *
 * PROJECT.md 22c (2026-10-28): the pretty printer for the .def files. One uniform layout:
 *
 *   - indentation by nesting, 2 spaces a level: a statement ending in ":" (a "with:" block, a
 *     layer) opens one, "end;" closes it;
 *   - a finished statement ("...;") longer than defLineWidth is broken before its clause keywords
 *     ("as", "from", "via", "topic", "with" -- never "with:"), each continuation defContinuation
 *     deeper -- which the DSL already reads back as one statement (joinWrappedStatementContinuations);
 *   - comment lines, blank lines and trailing comments stay as written; a "when"/"if" rule clause,
 *     a "${...}" setting and a statement with a comment inside are never broken.
 *
 * Formatting is idempotent, and changes no meaning: the generated output is the same before and
 * after. The generator's own writers into the defs (frozen IDs, metadata, import identities)
 * read a file with its statements joined (readDefStatementLines) and write it back through this
 * formatter (writeDefFormatted), so they keep working on one line per statement.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 28.10.2026
 *
 */

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	defLineWidth    = 80
	defIndent       = 2
	defContinuation = 4
)

// defBreakKeywords are the clause keywords a long statement may be broken before.
var defBreakKeywords = map[string]bool{"as": true, "from": true, "via": true, "topic": true, "with": true}

// formatDefContent returns content in the uniform layout (this file's header comment).
func formatDefContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	var out []string
	depth := 0
	for i := 0; i < len(lines); i++ {
		line := strings.ReplaceAll(strings.TrimRight(lines[i], " \t"), "\t", " ")
		if isCommentOrBlankLine(line) {
			out = append(out, line)
			continue
		}
		// One statement, possibly over several lines (the same rule the parser joins by).
		group := []string{line}
		joined := strings.TrimSpace(line)
		for !lineIsCompleteStatement(joined) && i+1 < len(lines) {
			next := strings.TrimRight(lines[i+1], " \t")
			if isCommentOrBlankLine(next) {
				break
			}
			i++
			group = append(group, next)
			joined += " " + strings.TrimSpace(next)
		}
		code, comment := splitTrailingComment(joined)
		if strings.HasPrefix(code, "end;") && depth > 0 {
			depth--
		}
		indent := strings.Repeat(" ", depth*defIndent)
		if len(group) > 1 && (hasInternalComment(group) || !breakableStatement(code)) {
			// A comment inside a statement, or one written over lines that may not be broken (a
			// rule clause): keep its lines, only re-indented.
			for k, g := range group {
				extra := ""
				if k > 0 {
					extra = strings.Repeat(" ", defContinuation)
				}
				out = append(out, indent+extra+strings.TrimSpace(g))
			}
		} else {
			out = append(out, layoutStatement(indent, code, comment)...)
		}
		if strings.HasSuffix(code, ":") {
			depth++
		}
	}
	return strings.Join(alignSettings(out), "\n")
}

// defSettingLinePattern matches a setting line, "${<name>} = <value>;" (with its indentation).
var defSettingLinePattern = regexp.MustCompile(`^(\s*)(\$\{[^}]+\})\s*=\s*(.*)$`)

// alignSettings aligns the "=" of each run of consecutive setting lines (a blank line, a comment or
// any other statement ends a run) -- the table layout Settings.def has always had (Erik, 2026-10-28).
func alignSettings(lines []string) []string {
	for start := 0; start < len(lines); {
		if !defSettingLinePattern.MatchString(lines[start]) {
			start++
			continue
		}
		end, width := start, 0
		for ; end < len(lines); end++ {
			m := defSettingLinePattern.FindStringSubmatch(lines[end])
			if m == nil {
				break
			}
			width = max(width, len(m[2]))
		}
		for i := start; i < end; i++ {
			m := defSettingLinePattern.FindStringSubmatch(lines[i])
			lines[i] = m[1] + fmt.Sprintf("%-*s", width, m[2]) + " = " + m[3]
		}
		start = end
	}
	return lines
}

// defMetadataLinePattern matches a metadata line ("<field>: <value>[ forced];"), whose colons the
// generator aligns within its block -- the one alignment the layout keeps.
var defMetadataLinePattern = regexp.MustCompile(`^[a-z_]+:\s`)

// layoutStatement lays out one statement at indent: single spaces between its words (outside quoted
// strings; a metadata line keeps its alignment), on one line when it fits (or can't be broken),
// else broken before its clause keywords, packed greedily.
func layoutStatement(indent, code, comment string) []string {
	if !defMetadataLinePattern.MatchString(code) {
		code = strings.Join(splitOutsideQuotes(code), " ")
	}
	single := indent + code + comment
	if len(indent)+len(code) <= defLineWidth || !breakableStatement(code) {
		return []string{single}
	}
	segments := clauseSegments(code)
	if len(segments) < 2 {
		return []string{single}
	}
	continuation := indent + strings.Repeat(" ", defContinuation)
	var lines []string
	current := indent + segments[0]
	for _, segment := range segments[1:] {
		// Never break after a ":" or ";": that line would read as a statement of its own (a block
		// opener, e.g. "entity light.social:" in the old one-liner form).
		if len(current)+1+len(segment) <= defLineWidth || strings.HasSuffix(current, ":") || strings.HasSuffix(current, ";") {
			current += " " + segment
			continue
		}
		lines = append(lines, current)
		current = continuation + segment
	}
	lines = append(lines, current+comment)
	return lines
}

// breakableStatement reports whether a statement may be broken over lines: a finished one, not a
// rule clause, not a setting.
func breakableStatement(code string) bool {
	return strings.HasSuffix(code, ";") && !strings.HasPrefix(code, "when ") && !strings.HasPrefix(code, "if ") && !strings.HasPrefix(code, "${")
}

// clauseSegments splits code before each clause keyword outside a quoted string; a keyword whose
// line would end in ":" or ";" (e.g. "with:") isn't one.
func clauseSegments(code string) []string {
	words := splitOutsideQuotes(code)
	var segments []string
	current := ""
	for _, word := range words {
		if current != "" && defBreakKeywords[word] {
			segments = append(segments, current)
			current = word
			continue
		}
		if current == "" {
			current = word
		} else {
			current += " " + word
		}
	}
	return append(segments, current)
}

// splitOutsideQuotes splits s at spaces outside double-quoted strings.
func splitOutsideQuotes(s string) []string {
	var words []string
	var word strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' && (i == 0 || s[i-1] != '\\') {
			inQuote = !inQuote
		}
		if c == ' ' && !inQuote {
			if word.Len() > 0 {
				words = append(words, word.String())
				word.Reset()
			}
			continue
		}
		word.WriteByte(c)
	}
	if word.Len() > 0 {
		words = append(words, word.String())
	}
	return words
}

// splitTrailingComment splits a statement into its code and its trailing comment (with the spacing
// before it, "" when none).
func splitTrailingComment(statement string) (code, comment string) {
	statement = strings.TrimSpace(statement)
	code = stripLineComment(statement)
	idx := strings.IndexByte(statement[len(code):], '#')
	if idx < 0 {
		return code, ""
	}
	return code, statement[len(code):len(code)+idx] + statement[len(code)+idx:]
}

// hasInternalComment reports whether any but the last line of a statement's group carries a comment.
func hasInternalComment(group []string) bool {
	for _, line := range group[:len(group)-1] {
		if stripLineComment(line) != strings.TrimSpace(line) {
			return true
		}
	}
	return false
}

// readDefStatementLines reads a def file with each statement on one line (comments and blank
// lines as they are) -- for the generator's own line-based writers.
func readDefStatementLines(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return strings.Split(joinWrappedStatementContinuations(strings.ReplaceAll(string(raw), "\r\n", "\n")), "\n"), nil
}

// writeDefFormatted writes lines to path in the uniform layout.
func writeDefFormatted(path string, lines []string) error {
	return os.WriteFile(path, []byte(formatDefContent(strings.Join(lines, "\n"))), 0o644)
}

// runFormatDefs formats every .def file in dir, reporting each one it changed.
func runFormatDefs(dir string) error {
	paths, err := filepath.Glob(filepath.Join(dir, "*.def"))
	if err != nil {
		return err
	}
	sort.Strings(paths)
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		formatted := formatDefContent(string(raw))
		if formatted == string(raw) {
			continue
		}
		if err := os.WriteFile(path, []byte(formatted), 0o644); err != nil {
			return err
		}
		fmt.Printf("formatted %s\n", path)
	}
	return nil
}
