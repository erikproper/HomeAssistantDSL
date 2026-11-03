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
 *   - a finished statement ("...;") or a "... with:" block header longer than defLineWidth is
 *     broken before its clause keywords ("as", "from", "via", "topic", "with" -- never "with:"),
 *     each continuation defContinuation deeper -- which the DSL already reads back as one
 *     statement (joinWrappedStatementContinuations); a continuation that still doesn't fit has its
 *     value on a line of its own, one level deeper (breakLongClause);
 *   - comment lines, blank lines and trailing comments stay as written; a "when"/"if" rule clause,
 *     a "${...}" setting and a statement with a comment inside are never broken.
 *
 * A block holding a single statement is written in its one-line form (foldSingleMemberBlocks,
 * PROJECT.md 30), where the grammar has one.
 *
 * Formatting is idempotent, and changes no meaning: the generated output is the same before and
 * after. The generator's own writers into the defs (frozen IDs, metadata, import identities)
 * read a file with its statements joined (readDefStatementLines) and write it back through this
 * formatter (writeDefFormatted), so they keep working on one line per statement.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 03.11.2026
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
	defContinuation = 2 // Erik (PROJECT.md 30, 2026-11-03): 2 is the standard indentation, also here
)

// defBreakKeywords are the clause keywords a long statement may be broken before.
var defBreakKeywords = map[string]bool{"as": true, "from": true, "via": true, "topic": true, "with": true}

// formatDefContent returns content in the uniform layout (this file's header comment).
func formatDefContent(content string) string {
	statements := foldSingleMemberBlocks(splitDefStatements(content))
	var out []string
	depth := 0
	for _, statement := range statements {
		if statement.code == "" {
			out = append(out, statement.group[0])
			continue
		}
		code, comment, group := statement.code, statement.comment, statement.group
		if strings.HasPrefix(code, "end;") && depth > 0 {
			depth--
		}
		indent := strings.Repeat(" ", depth*defIndent)
		if isElseClause(code) && depth > 0 {
			// "else:"/"else <action>;" belongs to its "then if ...:" and shares one "end;" with it:
			// written at the "then if"'s own level, its branch below it as deep as the "then" one.
			indent = strings.Repeat(" ", (depth-1)*defIndent)
		}
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
		if strings.HasSuffix(code, ":") && !isElseClause(code) {
			depth++
		}
	}
	return strings.Join(alignSettings(out), "\n")
}

// isElseClause reports whether code is a "then if" block's else clause, "else:" or "else <action>;".
func isElseClause(code string) bool {
	return code == "else:" || strings.HasPrefix(code, "else ")
}

// tDefStatement is one statement of a def file as written: its lines, and its code and trailing
// comment joined onto one line. A comment or blank line is a statement with no code.
type tDefStatement struct {
	group         []string
	code, comment string
}

// splitDefStatements splits content into its statements, a statement possibly written over several
// lines (the same rule the parser joins by).
func splitDefStatements(content string) []tDefStatement {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	var statements []tDefStatement
	for i := 0; i < len(lines); i++ {
		line := strings.ReplaceAll(strings.TrimRight(lines[i], " \t"), "\t", " ")
		if isCommentOrBlankLine(line) {
			statements = append(statements, tDefStatement{group: []string{line}})
			continue
		}
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
		statements = append(statements, tDefStatement{group: group, code: code, comment: comment})
	}
	return statements
}

// foldSingleMemberBlocks writes a block holding a single statement as its one-line form (PROJECT.md
// 30, Erik: "if a ': ...; end;' clause only has one member, then we should have used the version
// without the ': ...; end;' option" -- the formatter enforces it). Only where that one-line form is
// part of the grammar (foldedBlockHeader), and never when a comment sits on any of the three lines.
func foldSingleMemberBlocks(statements []tDefStatement) []tDefStatement {
	var folded []tDefStatement
	for i := 0; i < len(statements); i++ {
		// "else:" with one action before its "then if"'s "end;": "else <action>;", the "end;" kept.
		if i+2 < len(statements) && statements[i].code == "else:" && statements[i].comment == "" {
			member, end := statements[i+1], statements[i+2]
			if member.comment == "" && end.code == "end;" && member.code != "" && !strings.HasSuffix(member.code, ":") && !hasInternalComment(member.group) {
				code := "else " + member.code
				folded = append(folded, tDefStatement{group: []string{code}, code: code})
				i++
				continue
			}
		}
		if i+2 < len(statements) {
			header, member, end := statements[i], statements[i+1], statements[i+2]
			if header.comment == "" && member.comment == "" && end.comment == "" && end.code == "end;" &&
				member.code != "" && !strings.HasSuffix(member.code, ":") && !hasInternalComment(member.group) {
				if head, ok := foldedBlockHeader(header.code, member.code); ok {
					code := head + " " + member.code
					folded = append(folded, tDefStatement{group: []string{code}, code: code})
					i += 2
					continue
				}
			}
		}
		folded = append(folded, statements[i])
	}
	return folded
}

// foldedBlockHeader returns header without its ":" when "<header> <member>" is the one-line form of
// that block in the grammar:
//
//   - "device <id> [as leaf <leaf>] with entity ...;" (Conceptual.def),
//   - "derived capability ... with condition ...;" and "... with value ...;" (Physical.def, Logical.def),
//   - "then <action>;" (a rule; "else <action>;" is foldSingleMemberBlocks' own case),
//   - "for <targets> with <field>: <value>;" (a "defaults:" rule),
//   - "device <id> host <hostname> <type> [via cloud] with <statement>;" (integration hosts),
//   - "absorb from <device-id> capability ...;" (Logical.def).
//
// A space or a device holding one block of its own stays a block: its one-liner would need a
// second "with" (Erik, 2026-11-02).
func foldedBlockHeader(header, member string) (string, bool) {
	switch {
	case defDeviceBlockHeaderPattern.MatchString(header):
		return strings.TrimSuffix(header, ":"), strings.HasPrefix(member, "entity ")
	case defDefaultsBlockHeaderPattern.MatchString(header):
		return strings.TrimSuffix(header, ":") + " with", defMetadataLinePattern.MatchString(member)
	case defHostsDeviceBlockHeaderPattern.MatchString(header):
		return strings.TrimSuffix(header, ":"), true
	case defAbsorbBlockHeaderPattern.MatchString(header):
		return strings.TrimSuffix(header, ":"), strings.HasPrefix(member, "capability ")
	case defDerivedBlockHeaderPattern.MatchString(header):
		return strings.TrimSuffix(header, ":"), strings.HasPrefix(member, "condition ") || strings.HasPrefix(member, "value ")
	case header == "then:":
		return "then", true
	}
	return "", false
}

var (
	defDeviceBlockHeaderPattern      = regexp.MustCompile(`^device\s+\S+(?:\s+as\s+leaf\s+\S+)?\s+with:$`)
	defDefaultsBlockHeaderPattern    = regexp.MustCompile(`^for\s+\S.*:$`)
	defHostsDeviceBlockHeaderPattern = regexp.MustCompile(`^device\s+\S+\s+host\s+\S+\s+\S+(?:\s+via\s+cloud)?\s+with:$`)
	defAbsorbBlockHeaderPattern      = regexp.MustCompile(`^absorb\s+from\s+\S+:$`)
	defDerivedBlockHeaderPattern     = regexp.MustCompile(`^(?:internal\s+)?derived\s+capability\s+\S+(?:\s+as\s+unique_id\s+\S+)?\s+with:$`)
)

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
		if lines := breakCondition(indent+code, indent); lines != nil {
			lines[len(lines)-1] += comment
			return lines
		}
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
		lines = append(lines, breakLongLine(current, indent, continuation)...)
		current = continuation + segment
	}
	last := breakLongLine(current, indent, continuation)
	if header, ok := strings.CutSuffix(last[len(last)-1], " with:"); ok && len(lines)+len(last) > 1 {
		// A broken block header: its "with:" on a line of its own at the header's depth, so that
		// "with:" and "end;" frame the block (Erik, PROJECT.md 30).
		last = append(last[:len(last)-1], header, indent+"with:")
	}
	last[len(last)-1] += comment
	return append(lines, last...)
}

// breakLongLine breaks a laid-out line that is still too long: a condition over its operands
// (breakCondition), else a continuation's value onto a line of its own (breakLongClause).
func breakLongLine(line, indent, continuation string) []string {
	lineIndent := indent
	if strings.HasPrefix(line, continuation) && line != indent+strings.TrimSpace(line) {
		lineIndent = continuation
	}
	if lines := breakCondition(line, lineIndent); lines != nil {
		return lines
	}
	if lineIndent != continuation {
		return []string{line}
	}
	return breakLongClause(line, continuation)
}

// breakCondition returns a too-long line holding a condition ("... condition <expr>;", e.g. "with
// condition a and b;") broken after "condition", each operand of the expression's top-level "and"/
// "or" on a line of its own, defIndent deeper than the line, ending in its operator (Erik, PROJECT.md
// 30). Parentheses and quoted text are never broken into. nil when line holds no condition, or it
// has a single operand.
func breakCondition(line, lineIndent string) []string {
	if len(line) <= defLineWidth {
		return nil
	}
	words := splitOutsideQuotes(strings.TrimSpace(line))
	at := -1
	for i, word := range words {
		if word == "condition" && i+1 < len(words) {
			at = i
			break
		}
	}
	if at < 0 {
		return nil
	}
	operandIndent := lineIndent + strings.Repeat(" ", defIndent)
	var lines []string
	depth := 0
	var current []string
	for _, word := range words[at+1:] {
		current = append(current, word)
		depth += strings.Count(word, "(") - strings.Count(word, ")")
		if depth == 0 && (word == "and" || word == "or") {
			lines = append(lines, operandIndent+strings.Join(current, " "))
			current = nil
		}
	}
	if len(lines) == 0 {
		return nil
	}
	lines = append(lines, operandIndent+strings.Join(current, " "))
	return append([]string{lineIndent + strings.Join(words[:at+1], " ")}, lines...)
}

// breakLongClause returns a continuation line (at continuation) that is still too long with its
// last word -- the clause's value, e.g. a long frozen ID -- on a line of its own, one level deeper
// (Erik, PROJECT.md 30: "as unique_id" / "<id>;"). Not when that would leave a line ending in ":"
// or ";" (it would read as a statement of its own), nor for a block header's "with:".
func breakLongClause(line, continuation string) []string {
	if !strings.HasPrefix(line, continuation) || len(line) <= defLineWidth {
		return []string{line}
	}
	words := splitOutsideQuotes(strings.TrimPrefix(line, continuation))
	n := len(words)
	if n < 2 || words[n-1] == "with:" || strings.HasSuffix(words[n-2], ":") || strings.HasSuffix(words[n-2], ";") {
		return []string{line}
	}
	return []string{continuation + strings.Join(words[:n-1], " "), continuation + strings.Repeat(" ", defIndent) + words[n-1]}
}

// breakableStatement reports whether a statement may be broken over lines: a finished one or a
// "... with:" block header (PROJECT.md 30, Erik: a long header is split too; the "with:" stays on
// its last line), not a rule clause, not a setting.
func breakableStatement(code string) bool {
	return (strings.HasSuffix(code, ";") || strings.HasSuffix(code, " with:")) && !strings.HasPrefix(code, "when ") && !strings.HasPrefix(code, "if ") && !strings.HasPrefix(code, "then ") && !strings.HasPrefix(code, "${")
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
// lines as they are) -- for the generator's own line-based writers. A hosts device's or an absorb
// block's one-liner comes back as its block (expandOneLinerBlocks), the form those writers edit;
// writeDefFormatted folds it again.
func readDefStatementLines(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(joinWrappedStatementContinuations(strings.ReplaceAll(string(raw), "\r\n", "\n")), "\n")
	lines = expandOneLinerBlocks(lines, hostsDeviceOneLinerPattern, " with:")
	return expandOneLinerBlocks(lines, absorbOneLinerPattern, ":"), nil
}

// The one-liners of blocks whose parsers read the block form (PROJECT.md 30, 2026-11-03: every
// block with one member has a one-line form): group 1 is the header without its opener, group 2
// the single statement.
var (
	// "for <targets> with <field>: <value>;" (Defaults.def, a "defaults:" block)
	defaultsOneLinerPattern = regexp.MustCompile(`^(for\s+.+?)\s+with\s+([a-z_]+:\s.*;)$`)
	// "device <id> host <hostname> <type> [via cloud] with <statement>;" (integration hosts)
	hostsDeviceOneLinerPattern = regexp.MustCompile(`^(\s*device\s+\S+\s+host\s+\S+\s+\S+(?:\s+via\s+cloud)?)\s+with\s+(\S.*;)$`)
	// "absorb from <device-id> capability ...;" (Logical.def)
	absorbOneLinerPattern = regexp.MustCompile(`^(\s*absorb\s+from\s+\S+)\s+(capability\s.*;)$`)
)

// expandOneLinerBlocks returns lines with every match of pattern (see its groups above) written as
// its block: the header plus opener, the statement one level deeper, and "end;". A trailing
// comment stays on the header.
func expandOneLinerBlocks(lines []string, pattern *regexp.Regexp, opener string) []string {
	var out []string
	for _, line := range lines {
		code, comment := splitTrailingComment(line)
		m := pattern.FindStringSubmatch(code)
		if m == nil {
			out = append(out, line)
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
		out = append(out, indent+strings.TrimSpace(m[1])+opener+comment, indent+"  "+m[2], indent+"end;")
	}
	return out
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
